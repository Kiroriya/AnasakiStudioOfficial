// Command server runs Anasaki Studio: REST API, admin SPA and the public site.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anasaki/studio/backend/internal/api"
	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/config"
	"github.com/anasaki/studio/backend/internal/db"
	"github.com/anasaki/studio/backend/internal/demo"
	"github.com/anasaki/studio/backend/internal/legacy"
	"github.com/anasaki/studio/backend/internal/mail"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/seed"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/sitecode"
	"github.com/anasaki/studio/backend/internal/store"
)

func main() {
	cfg := config.Load()
	if warn, err := config.CheckJWTSecret(cfg.JWTSecret); err != nil {
		log.Fatal(err)
	} else if warn != "" {
		log.Print(warn)
	}
	plugin.SetTrustProxy(cfg.TrustProxy)
	plugin.SetCoreVersion(config.Version) // plugins' minCore is checked against this release

	conn, err := db.Open(cfg.DSN)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(conn); err != nil {
		log.Fatal(err)
	}
	st := store.New(conn)
	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := seed.Roles(st); err != nil {
		log.Fatalf("seed roles: %v", err)
	}
	if err := seed.Run(st, cfg.AdminEmail, cfg.AdminPassword, cfg.UploadDir); err != nil {
		log.Fatalf("seed: %v", err)
	}

	// The plugin platform: package / service / WASM / JS plugins. No feature
	// plugins are compiled in any more (phase 7: they are official JS plugins
	// from the Store; backup is the core tool "Export / import site").
	plugins, err := plugin.NewManager(st, cfg.UploadDir)
	if err != nil {
		log.Fatal(err)
	}
	web := site.New(st, plugins)
	plugins.SetPages(web)

	// Demo mode: shared visitor account, guard rails and periodic content reset.
	playground, err := demo.New(demo.Config{Enabled: cfg.DemoMode, Email: cfg.DemoEmail, Password: cfg.DemoPassword,
		Every: cfg.DemoReset}, st, cfg.UploadDir)
	if err != nil {
		log.Fatal(err)
	}
	web.SetDemo(playground != nil)

	mux := http.NewServeMux()
	authn := auth.New(cfg.JWTSecret)
	core := api.New(st, authn, cfg.UploadDir, plugins, web)

	// Site Code: the site's own JS/TS, served after plugin public routes and
	// before page routing, and subscribed to core events.
	var code *sitecode.Engine
	mailer := func(m sitecode.MailMessage) error {
		if cfg.SiteCodeMailSink == "log" { // development: no SMTP needed
			code.Log("info", "mail (dev sink)", fmt.Sprintf("to=%s subject=%q html=%d bytes text=%d bytes", strings.Join(m.To, ", "), m.Subject, len(m.HTML), len(m.Text)), 0)
			return nil
		}
		return core.SendMailMessage(mail.Message{To: m.To, Subject: m.Subject, HTML: m.HTML, Text: m.Text, ReplyTo: m.ReplyTo})
	}
	code = sitecode.New(st, sitecode.Config{Disabled: cfg.SiteCodeDisabled, Demo: playground != nil, Auth: authn,
		Services: plugins.ServicePublicPaths, PostURL: site.PostURL, DSN: cfg.DSN, CodeDSN: cfg.CodeDSN,
		Emit: plugins.Emit, Mailer: mailer, SecretKey: cfg.SiteCodeSecretKey, KeyDir: cfg.PluginsDir, TypeSlugs: site.SetTypeSlugs,
		Plugins: plugins})
	defer code.Close()
	plugins.SetCodeRuntime(code) // JS plugins ("runtime": "js") run as Site Code units
	web.SetPostTypes(func(opts map[string]string) []site.PostTypeRoute {
		var out []site.PostTypeRoute
		for _, t := range code.ManifestFor(opts).PostTypes {
			out = append(out, site.PostTypeRoute{Type: t.Type, Slug: t.Slug, Public: t.Public, Label: t.Label})
		}
		return out
	})
	log.Printf("site code: database mode %s (%s)", code.Data().Mode, code.Data().Database)
	web.SetCode(code.ServePublic)
	web.SetCodeSessions(code.Session)
	web.SetCodeHooks(code) // site.middleware / site.afterResponse (phase 6)
	plugins.Listen(code.OnEvent)
	core.SetSiteCode(code)
	if cfg.SiteCodeDisabled {
		log.Printf("site code: disabled by SITE_CODE_DISABLED")
	}
	core.SetDemo(playground)
	plugins.SetMailer(core.SendMail)
	// Phase 7: the former built-in plugins (analytics, seo, forms, …) are
	// official JS plugins now; a site upgraded from a version that had them
	// gets them installed from the Store and its old data imported, in the
	// background (old tables/options are never deleted automatically).
	migrator, err := legacy.New(st.DB, plugins, code, legacy.Options{})
	if err != nil {
		log.Fatal(err)
	}
	core.SetLegacy(migrator)
	core.Routes(mux)
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", noListing(nosniff(sandboxUploads(http.FileServer(http.Dir(cfg.UploadDir)))))))
	// Catch-all: the admin SPA under its configurable path, then the public site.
	mux.Handle("/", adminOrSite(core.AdminPath, spa(cfg.AdminDir), plugins.Middleware(web)))

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           logRequests(core.AdminPath, cors(cfg.CORSOrigin, secureHeaders(core.AdminPath, mux))),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Minute, // large uploads
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("%s %s listening on %s  (site: /, admin: /%s/, api: /api/v1)", config.Product, config.Version, cfg.Addr, core.AdminPath())
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		if n, err := st.CountUsers(); err == nil && n == 0 {
			log.Printf("first run: open the admin to finish the setup wizard")
		}
	}
	// background jobs and cron of Site Code; stopped cleanly on SIGINT / SIGTERM
	code.StartWorker()
	migCtx, stopMigration := context.WithCancel(context.Background())
	defer stopMigration()
	migrator.Start(migCtx)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		log.Printf("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	code.StopWorker()
	code.WaitEvents()
}

// adminOrSite routes /<admin path>/… to the admin SPA and everything else to
// the public site. The legacy /admin/ answers 404 once the path is changed
// (like WordPress "hide login" plugins).
func adminOrSite(adminPath func() string, admin func(prefix string) http.Handler, site http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := adminPath()
		switch {
		case r.URL.Path == "/"+p:
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				http.Redirect(w, r, "/"+p+"/", http.StatusMovedPermanently)
				return
			}
		case strings.HasPrefix(r.URL.Path, "/"+p+"/"):
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			admin("/"+p).ServeHTTP(w, r)
			return
		case p != "admin" && (r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/")):
			http.NotFound(w, r)
			return
		}
		site.ServeHTTP(w, r)
	})
}

// spa serves the built Angular app under prefix, falling back to index.html
// for client routes. index.html's <base href> is rewritten to the prefix.
func spa(dir string) func(prefix string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return func(prefix string) http.Handler {
		return http.StripPrefix(prefix, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := filepath.Join(dir, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() && filepath.Base(p) != "index.html" {
				fs.ServeHTTP(w, r)
				return
			}
			index, err := os.ReadFile(filepath.Join(dir, "index.html"))
			if err != nil {
				http.Error(w, "Admin app not built. Run `npm run build` in frontend/ or use `ng serve` on :4200.", http.StatusNotFound)
				return
			}
			index = bytes.Replace(index, []byte(`<base href="/admin/">`), []byte(`<base href="`+prefix+`/">`), 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(index)
		}))
	}
}

func noListing(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") || r.URL.Path == "" {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// nosniff stops browsers from guessing a content type for uploaded files.
func nosniff(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		h.ServeHTTP(w, r)
	})
}

// sandboxUploads serves uploaded files as sandboxed documents, so an uploaded
// SVG or similar cannot run script on the site's origin. PDFs are left out:
// browsers refuse to show a sandboxed PDF.
func sandboxUploads(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(path.Ext(r.URL.Path), ".pdf") {
			w.Header().Set("Content-Security-Policy", "sandbox")
		}
		h.ServeHTTP(w, r)
	})
}

// secureHeaders adds anti-framing / sniffing / referrer headers to the admin
// app, the API and plugin admin screens (the public site is left alone so it
// can still be embedded elsewhere).
func secureHeaders(adminPath func() string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/ext-ui/") ||
			p == "/"+adminPath() || strings.HasPrefix(p, "/"+adminPath()+"/") {
			h := w.Header()
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("Content-Security-Policy", "frame-ancestors 'self'")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		}
		next.ServeHTTP(w, r)
	})
}

func cors(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin != "" && r.Header.Get("Origin") == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(adminPath func() string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/"+adminPath()+"/") {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Microsecond))
		}
	})
}
