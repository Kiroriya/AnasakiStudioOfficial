// Package api exposes the REST API used by the Angular admin + designer
// (the equivalent of WordPress' /wp-json/wp/v2 namespace).
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/demo"
	"github.com/anasaki/studio/backend/internal/legacy"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/sitecode"
	"github.com/anasaki/studio/backend/internal/store"
)

type API struct {
	st        *store.Store
	auth      *auth.Auth
	uploadDir string
	plugins   *plugin.Manager
	site      *site.Site

	demo *demo.Demo // nil unless DEMO_MODE

	code   *sitecode.Engine // Site Code (nil = endpoints not mounted)
	codeMu sync.Mutex       // serialises draft writes (limits, conflicts)

	setupMu    sync.Mutex // serialises the first-run setup
	setupToken string     // SETUP_TOKEN: required by POST /setup when set (audit M-5)
	adminPath  pathCache

	legacy *legacy.Migrator // phase 7: migration of the former built-ins (nil = none)
}

// New creates the API and loads the role capabilities from the database
// (seed.Roles must have run).
// db is the store acting for the request's user: the events of its writes
// carry the actor (source "admin", or "api" for a personal access token).
func (a *API) db(r *http.Request) *store.Store {
	p := auth.From(r)
	if p == nil {
		return a.st.As(store.Actor{Source: "admin"})
	}
	src := "admin"
	if p.TokenID != 0 {
		src = "api"
	}
	return a.st.As(store.Actor{ID: p.ID, Source: src})
}

func New(st *store.Store, a *auth.Auth, uploadDir string, plugins *plugin.Manager, web *site.Site) *API {
	api := &API{st: st, auth: a, uploadDir: uploadDir, plugins: plugins, site: web}
	if err := api.reloadRoles(); err != nil {
		log.Printf("load roles: %v", err)
	}
	return api
}

// Routes registers every endpoint under /api/v1.
func (a *API) Routes(mux *http.ServeMux) {
	const p = "/api/v1"
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST "+p+"/auth/login", a.login)
	mux.Handle("POST "+p+"/auth/logout", a.protectSession("", http.HandlerFunc(a.logout)))
	mux.Handle("POST "+p+"/auth/refresh", a.protectSession("", http.HandlerFunc(a.refreshSession)))
	mux.HandleFunc("GET "+p+"/setup", a.getSetup)
	mux.HandleFunc("POST "+p+"/setup", a.runSetup)
	mux.HandleFunc("GET /api/public/site", a.publicSite)
	mux.HandleFunc("GET /download/{id}", a.download)

	h := func(pattern, cap string, fn http.HandlerFunc) { mux.Handle(pattern, a.protect(cap, fn)) }
	a.mailRoutes(h, mux)

	h("GET "+p+"/auth/me", "", a.me)
	h("PUT "+p+"/auth/me", "", a.updateMe)
	h("GET "+p+"/auth/tokens", "", a.listTokens) // own tokens; ?user= with manage_users
	h("POST "+p+"/auth/tokens", "", a.createToken)
	h("DELETE "+p+"/auth/tokens/{id}", "", a.deleteToken)
	h("GET "+p+"/stats", "", a.stats) // every role opens the dashboard; counts are filtered by capability

	h("GET "+p+"/posts", "", a.listPosts)
	h("POST "+p+"/posts", "", a.createPost)
	h("PUT "+p+"/posts/order", "", a.orderPosts)
	h("GET "+p+"/posts/{id}", "", a.getPost)
	h("PUT "+p+"/posts/{id}", "", a.updatePost)
	h("DELETE "+p+"/posts/{id}", "", a.deletePost)
	h("POST "+p+"/posts/{id}/duplicate", "", a.duplicatePost)
	h("GET "+p+"/posts/{id}/revisions", "", a.listRevisions)
	h("GET "+p+"/posts/{id}/revisions/{rid}", "", a.getRevision)
	h("POST "+p+"/posts/{id}/revisions/{rid}/restore", "", a.restoreRevision)
	a.postTypeRoutes(h)

	// categories: the post editor (edit_own_posts), the designer (edit_pages) and the categories screen
	h("GET "+p+"/terms", "", needAny(a.listTerms, auth.CapEditOwnPosts, auth.CapEditPages, auth.CapManageTerms))
	// writes: manage_categories for "category", the taxonomy's access.manage otherwise (2.1.0)
	h("POST "+p+"/terms", "", a.createTerm)
	h("PUT "+p+"/terms/{id}", "", a.updateTerm)
	h("DELETE "+p+"/terms/{id}", "", a.deleteTerm)
	h("GET "+p+"/taxonomies", "", a.listTaxonomies)

	h("GET "+p+"/media", auth.CapUploadFiles, a.listMedia)
	h("POST "+p+"/media", auth.CapUploadFiles, a.uploadMedia)
	h("POST "+p+"/media/import", auth.CapUploadFiles, a.importMedia)
	h("PUT "+p+"/media/{id}", auth.CapUploadFiles, a.updateMedia)
	h("DELETE "+p+"/media/{id}", auth.CapUploadFiles, a.deleteMedia)

	h("GET "+p+"/menus", auth.CapEditPages, a.listMenus) // menus screen + designer
	h("POST "+p+"/menus", auth.CapEditPages, a.saveMenu)
	h("PUT "+p+"/menus/{id}", auth.CapEditPages, a.saveMenu)
	h("DELETE "+p+"/menus/{id}", auth.CapEditPages, a.deleteMenu)

	h("GET "+p+"/components", auth.CapEditPages, a.listComponents) // components screen + designer
	h("POST "+p+"/components", auth.CapEditPages, a.saveComponent)
	h("PUT "+p+"/components/{id}", auth.CapEditPages, a.saveComponent)
	h("DELETE "+p+"/components/{id}", auth.CapEditPages, a.deleteComponent)
	h("GET "+p+"/components/{id}/revisions", auth.CapEditPages, a.listComponentRevisions)
	h("GET "+p+"/components/{id}/usage", auth.CapEditPages, a.componentUsage)
	h("POST "+p+"/components/{id}/variants/rename", auth.CapEditPages, a.renameVariant)
	h("POST "+p+"/components/{id}/site-role", auth.CapEditPages, a.setComponentSiteRole)
	h("POST "+p+"/links/replace", auth.CapEditPages, a.replaceLinks)
	h("GET "+p+"/components/{id}/revisions/{rid}", auth.CapEditPages, a.getComponentRevision)

	h("GET "+p+"/settings", "", a.getSettings)
	h("PUT "+p+"/settings", "", a.updateSettings) // manage_options, or a demo visitor for a few harmless keys
	h("GET "+p+"/settings/reserved-paths", auth.CapManageOptions, a.reservedPathList)
	h("POST "+p+"/demo/snapshot", auth.CapManageOptions, a.demoSnapshot)
	h("POST "+p+"/demo/reset", auth.CapManageOptions, a.demoReset)

	h("GET "+p+"/users", auth.CapManageUsers, a.listUsers)
	// id + display name only, for "user" fields of the post editor (2.1.0)
	h("GET "+p+"/users/options", auth.CapEditOthersPosts, a.userOptions)
	h("POST "+p+"/users", auth.CapManageUsers, a.createUser)
	h("PUT "+p+"/users/{id}", auth.CapManageUsers, a.updateUser)
	h("DELETE "+p+"/users/{id}", auth.CapManageUsers, a.deleteUser)
	h("POST "+p+"/users/bulk", auth.CapManageUsers, a.bulkUsers)

	h("GET "+p+"/roles", auth.CapManageUsers, a.listRoles)
	h("POST "+p+"/roles", auth.CapManageUsers, a.createRole)
	h("PUT "+p+"/roles/{slug}", auth.CapManageUsers, a.updateRole)
	h("DELETE "+p+"/roles/{slug}", auth.CapManageUsers, a.deleteRole)
	h("POST "+p+"/roles/restore", auth.CapManageUsers, a.restoreRoles)

	h("POST "+p+"/preview", auth.CapEditPages, a.preview)

	h("GET "+p+"/plugins", "", a.listPlugins) // every role builds its menu from it; trimmed without manage_options
	h("POST "+p+"/plugins/{id}/activate", auth.CapManageOptions, a.activatePlugin)
	h("POST "+p+"/plugins/{id}/deactivate", auth.CapManageOptions, a.deactivatePlugin)
	h("GET "+p+"/collections/{source}", auth.CapEditPages, a.collectionItems)
	a.platformRoutes(h)
	a.toolRoutes(h)        // Export / import site (phase 7, tools.go)
	a.legacyRoutes(h)      // migration of the former built-ins (legacy.go)
	a.recommendedRoutes(h) // setup wizard: recommended plugins (recommended.go)
	a.collabRoutes(h)      // designer comments, presence (comments.go)
	a.serviceRoutes(h, mux)
	if a.code != nil {
		a.codeRoutes(h)
	}
	for _, rt := range a.plugins.Routes() {
		mux.Handle(rt.Pattern, a.pluginRoute(rt))
	}
}

// protect authenticates the request, refreshes the role from the database
// (so demotions, suspensions, deletions and password changes apply
// immediately) and enforces a capability.
func (a *API) protect(cap string, fn http.HandlerFunc) http.Handler {
	session := a.protectSession(cap, fn)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// a personal access token (tokens.go) instead of a session
		if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && strings.HasPrefix(tok, tokenPrefix) {
			a.tokenAuth(w, r, tok, cap, fn)
			return
		}
		session.ServeHTTP(w, r)
	})
}

// protectSession is protect for a session (JWT) bearer.
func (a *API) protectSession(cap string, fn http.HandlerFunc) http.Handler {
	return a.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := auth.From(r)
		u, hash, revoked, err := a.st.SessionCheck(p.ID, p.JTI) // cheap: runs on every request
		if err != nil || revoked || !a.auth.Current(p, hash) {
			writeErr(w, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบใหม่")
			return
		}
		if u.Status == model.UserSuspended {
			writeErr(w, http.StatusForbidden, "บัญชีนี้ถูกระงับ")
			return
		}
		p.Role, p.Name = u.Role, u.Name
		if cap != "" && !p.Can(cap) {
			writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
			return
		}
		fn(w, r)
	}))
}

// ---------------------------------------------------------------- helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func serverErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "ไม่พบข้อมูลที่ต้องการ (อาจถูกลบไปแล้ว)")
		return
	}
	log.Printf("api error: %v", err)
	writeErr(w, http.StatusInternalServerError, "เกิดข้อผิดพลาดในระบบ กรุณาลองใหม่อีกครั้ง")
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง: "+jsonErrText(err))
		return false
	}
	return true
}

// jsonErrText turns a decode error into a message that names the field and the
// expected kind of value, instead of Go's "cannot unmarshal … into Go struct
// field postInput.terms of type int64".
func jsonErrText(err error) string {
	var te *json.UnmarshalTypeError
	var se *json.SyntaxError
	var mbe *http.MaxBytesError
	switch {
	case errors.As(err, &te):
		want := te.Type.Kind().String()
		switch want {
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
			want = "ตัวเลข"
		case "string":
			want = "ข้อความ"
		case "bool":
			want = "true / false"
		case "slice", "array":
			want = "array"
		case "map", "struct":
			want = "object"
		}
		if te.Field == "" {
			return "ต้องเป็น " + want + " แต่ได้ " + te.Value
		}
		return "ฟิลด์ " + te.Field + " ต้องเป็น " + want + " แต่ได้ " + te.Value
	case errors.As(err, &se):
		return "JSON ไม่ถูกต้อง (ตำแหน่ง " + strconv.FormatInt(se.Offset, 10) + ")"
	case errors.As(err, &mbe):
		return "ข้อมูลใหญ่เกิน " + strconv.FormatInt(mbe.Limit>>20, 10) + " MB"
	case errors.Is(err, io.EOF):
		return "ไม่มีข้อมูล (body ว่าง)"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "JSON ไม่ครบ"
	}
	return err.Error()
}

func pathID(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id
}

func queryInt(r *http.Request, name string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return v
	}
	return def
}

// needAny wraps fn so it runs only for users with at least one of caps (for
// endpoints several screens with different capabilities share).
func needAny(fn http.HandlerFunc, caps ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.From(r).CanAny(caps...) {
			writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
			return
		}
		fn(w, r)
	}
}
