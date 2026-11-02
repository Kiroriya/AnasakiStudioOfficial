package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/db"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/seed"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/sitecode"
	"github.com/anasaki/studio/backend/internal/store"
)

// Helpers of the official plugin smoke tests for analytics, forms, cookie-consent and custom-code
// (plugins-official/<id>). The packages are installed from their folders through the package API
// exactly as an administrator (or the core migration) would. The Go built-ins are not registered in
// this environment (plugin.NewManager without built-ins), so the ids are free for the packages.

type offBEnv struct {
	*pluginEnv
	mu    sync.Mutex
	mails []sitecode.MailMessage
}

func (e *offBEnv) offBMailbox() []sitecode.MailMessage {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sitecode.MailMessage(nil), e.mails...)
}

// offBNew is newPluginAPI (page hooks: site.middleware / afterResponse / head / bodyEnd) with a mail sink.
func offBNew(t *testing.T) *offBEnv {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN not set (use a throw-away database)")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Skip("test database unreachable: ", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLUGINS_DIR", t.TempDir())
	st := store.New(conn)
	up := t.TempDir()
	clear := func() {
		for _, tbl := range []string{"plugin_packages", "plugin_kv", "script_settings", "script_jobs", "script_cron"} {
			_, _ = conn.Exec("DELETE FROM " + tbl)
		}
	}
	clear()
	t.Cleanup(clear)
	for _, tbl := range []string{"users", "posts", "post_terms", "terms", "revisions", "media", "menus", "components", "options", "roles",
		"script_files", "script_versions", "script_logs", "script_audit", "script_access", "post_meta"} {
		if _, err := conn.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatal(err)
		}
	}
	pm, err := plugin.NewManager(st, up)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Roles(st); err != nil {
		t.Fatal(err)
	}
	e := &offBEnv{}
	web := site.New(st, pm)
	authn := auth.New("test-secret")
	resetCodeDB(t, conn, os.Getenv("TEST_CODE_DB_DSN"))
	code := sitecode.New(st, sitecode.Config{Auth: authn, Services: pm.ServicePublicPaths, PostURL: site.PostURL,
		DSN: dsn, CodeDSN: os.Getenv("TEST_CODE_DB_DSN"), Emit: pm.Emit, TypeSlugs: site.SetTypeSlugs,
		SecretKey: "test-site-code-key", KeyDir: t.TempDir(), Plugins: pm,
		Mailer: func(m sitecode.MailMessage) error {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.mails = append(e.mails, m)
			return nil
		}})
	t.Cleanup(code.Close)
	pm.SetCodeRuntime(code)
	pm.SetPages(web) // route results {page} and core pages use the site's layout
	// generous limits on a busy machine (bundling, first VM warm-up); the joints keep their own budgets
	code.RouteTimeout, code.EventTimeout = 3*time.Second, 5*time.Second
	code.MiddlewareTimeout, code.AfterTimeout = 500*time.Millisecond, 3*time.Second
	web.SetCode(code.ServePublic)
	web.SetCodeSessions(code.Session)
	web.SetCodeHooks(code)
	web.SetPostTypes(func(opts map[string]string) []site.PostTypeRoute { // as cmd/server: Site Code + plugin post types
		var out []site.PostTypeRoute
		for _, pt := range code.ContentFor(opts).PostTypes {
			out = append(out, site.PostTypeRoute{Type: pt.Type, Slug: pt.Slug, Public: pt.Public, Label: pt.Label})
		}
		return out
	})
	pm.Listen(code.OnEvent)
	mux := http.NewServeMux()
	a := New(st, authn, up, pm, web)
	a.SetSiteCode(code)
	a.Routes(mux)
	mux.Handle("/", pm.Middleware(web))
	t.Cleanup(code.WaitEvents)
	t.Cleanup(code.WaitAfterResponse)
	e.pluginEnv = &pluginEnv{c: &client{t: t, h: mux}, code: code, pm: pm, st: st}
	return e
}

// toSample adapts the env to the samples_test.go helpers (adminPage, adminDownload).
func (e *offBEnv) toSample() *sampleEnv { return &sampleEnv{pluginEnv: e.pluginEnv} }

// offBPackage reads plugins-official/<id>.
func offBPackage(t *testing.T, id string) map[string]string {
	return dirFiles(t, filepath.Join("..", "..", "..", "plugins-official", id))
}

// offBInstall uploads and activates official packages.
func offBInstall(t *testing.T, admin *client, ids ...string) {
	t.Helper()
	for _, id := range ids {
		mustUpload(t, admin, offBPackage(t, id))
		activate(t, admin, id)
	}
}

// offBSetting sets unit settings (Site code → Settings → <unit>), read by the plugin with site.settings.
func offBSetting(t *testing.T, admin *client, unit string, kv map[string]string) {
	t.Helper()
	vals := []any{}
	for k, v := range kv {
		vals = append(vals, map[string]any{"key": k, "secret": false, "value": v})
	}
	if code, b := admin.do("PUT", "/api/v1/code/settings", map[string]any{"unit": unit, "values": vals}); code != 200 {
		t.Fatalf("settings %s: %d %v", unit, code, b)
	}
}

// offBPkgSettings saves manifest settings (Plugins page form).
func offBPkgSettings(t *testing.T, admin *client, id string, values map[string]any) {
	t.Helper()
	if code, b := admin.do("PUT", "/api/v1/package-plugins/"+id+"/settings", map[string]any{"values": values}); code != 200 {
		t.Fatalf("package settings %s: %d %v", id, code, b)
	}
}

// offBReq is one public request with a client address and headers.
type offBReq struct {
	method, path, ip string
	body             io.Reader
	hdr              map[string]string
}

func (e *offBEnv) offBSend(r offBReq) *httptest.ResponseRecorder {
	e.c.t.Helper()
	if r.method == "" {
		r.method = "GET"
	}
	if r.body == nil {
		r.body = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(r.method, r.path, r.body)
	if r.ip != "" {
		req.RemoteAddr = r.ip + ":4321"
	}
	for k, v := range r.hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.c.h.ServeHTTP(rec, req)
	return rec
}

const offBHuman = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// offBPage creates a published page.
func offBPage(t *testing.T, admin *client, slug, title string) map[string]any {
	t.Helper()
	code, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": title, "slug": slug, "status": "publish",
		"content": "<p>" + title + "</p>"})
	if code != 201 {
		t.Fatalf("page %s: %d %v", slug, code, p)
	}
	return p
}

// offBExport calls a JS plugin export like the core migration does (plugins.call from Site Code).
func offBExport(t *testing.T, e *offBEnv, admin *client, plugin, name string, arg any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(arg)
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('POST /offb-call', { access: ['*'] }, ({ request }) => plugins.call(` + jsQuote(plugin) + `, ` + jsQuote(name) + `, request.json()))`})
	code, out := getJSON(t, e.c, "POST", "/offb-call", string(b), nil)
	if code != 200 {
		t.Fatalf("%s.%s: %d %v", plugin, name, code, out)
	}
	return out
}

func jsQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// offBRows reads a plugin table (p_<id>_<name>, fallback sc_p_…) of the code database.
func offBRows(t *testing.T, e *offBEnv, table, where string, args ...any) []map[string]any {
	t.Helper()
	d := e.code.Data()
	q := "SELECT * FROM `" + d.Database + "`.`" + d.N.Prefix + table + "`"
	if where != "" {
		q += " WHERE " + where
	}
	rows, err := e.st.DB.Query(q, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				m[c] = string(v)
			case time.Time:
				m[c] = v.Format("2006-01-02")
			default:
				m[c] = v
			}
		}
		out = append(out, m)
	}
	return out
}

func offBContains(t *testing.T, what, s string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Fatalf("%s: missing %q in\n%s", what, w, s)
		}
	}
}

// offBQuiet fails when the unit logged warnings or errors (skipped middleware, failed handlers…).
func offBQuiet(t *testing.T, admin *client, unit string) {
	t.Helper()
	_, b := admin.do("GET", "/api/v1/code/logs?perPage=200&unit="+unit, nil)
	for _, x := range b["items"].([]any) {
		m := x.(map[string]any)
		if m["level"] == "warn" || m["level"] == "error" {
			t.Fatalf("%s logged %v: %v | %v", unit, m["level"], m["source"], m["message"])
		}
	}
}
