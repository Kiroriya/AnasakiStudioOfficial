package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Service plugins (see plugin/service.go): connection settings, admin screens
// in a sandboxed iframe (/ext-ui/...), and the API a service calls back
// (/api/service/...).

func (a *API) serviceRoutes(h func(pattern, cap string, fn http.HandlerFunc), mux *http.ServeMux) {
	const p = "/api/v1"
	h("PUT "+p+"/package-plugins/{id}/service", auth.CapManageOptions, a.setServiceURL)
	h("POST "+p+"/package-plugins/{id}/service/secret", auth.CapManageOptions, a.rotateServiceSecret)
	h("GET "+p+"/package-plugins/{id}/service/health", auth.CapManageOptions, a.serviceHealth)
	h("POST "+p+"/package-plugins/{id}/ui-ticket", "", a.serviceTicket)
	a.plugins.SetServiceAPI(a.serviceDispatch)
	mux.HandleFunc("/ext-ui/", a.extUI)
	mux.HandleFunc("/api/service/", a.serviceAPI)
}

func (a *API) setServiceURL(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	inst, err := a.plugins.SetServiceURL(r.PathValue("id"), in.URL)
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inst)
}

func (a *API) rotateServiceSecret(w http.ResponseWriter, r *http.Request) {
	inst, err := a.plugins.RotateServiceSecret(r.PathValue("id"))
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inst)
}

func (a *API) serviceHealth(w http.ResponseWriter, r *http.Request) {
	h, err := a.plugins.ServiceHealth(r.PathValue("id"))
	if err != nil {
		platformErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h)
}

const uiTicketTTL = 8 * time.Hour

// serviceTicket returns the iframe URL of a service admin page:
// POST /package-plugins/{id}/ui-ticket {page} → {url}.
func (a *API) serviceTicket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Page string `json:"page"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	pg, ok := a.plugins.AdminPage(id, in.Page)
	if !ok {
		writeErr(w, http.StatusNotFound, "ไม่พบหน้านี้ หรือปลั๊กอินยังไม่ได้เปิดใช้งาน/เชื่อมต่อ")
		return
	}
	cp := pg.Cap
	if cp == "" {
		cp = auth.CapManageOptions
	}
	me := auth.From(r)
	if !me.Can(cp) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์เปิดหน้านี้")
		return
	}
	_, hash, err := a.st.UserAuthByID(me.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	tok, err := a.auth.IssueTicket(me.ID, hash, "ext-ui:"+id, cp, uiTicketTTL)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": "/ext-ui/" + id + "/" + tok + "/" + in.Page})
}

// extUI serves /ext-ui/<plugin>/<ticket>/<rest> by proxying to the service's
// /admin/<rest> as the ticket's user, re-checked against the database on
// every request: a new password ends the ticket like a session, and a
// suspension or a role that lost the page's capability closes the page.
func (a *API) extUI(w http.ResponseWriter, r *http.Request) {
	if sitecode.ServeKit(w, r) { // /ext-ui/kit.css, /ext-ui/kit.js (phase 6 admin UI kit)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/ext-ui/"), "/", 3)
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	id, tok, rest := parts[0], parts[1], ""
	if len(parts) == 3 {
		rest = parts[2]
	}
	if id == "code" && a.code != nil { // Site Code admin pages (site.adminPage)
		a.codeExtUI(w, r, tok, rest)
		return
	}
	t, err := a.auth.ParseTicket(tok, "ext-ui:"+id)
	var u *model.User
	var hash string
	if err == nil {
		u, hash, err = a.st.UserAuthByID(t.UserID)
	}
	if err != nil || !a.auth.TicketCurrent(t, hash) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `<p style="font:15px system-ui;padding:32px">ลิงก์หมดอายุ — เปิดหน้านี้ใหม่จากเมนูของระบบจัดการ<br>This link expired; open the page again from the admin menu.</p>`)
		return
	}
	cp := t.Cap
	if cp == "" {
		cp = auth.CapManageOptions
	}
	if u.Status == model.UserSuspended || !(&auth.Principal{ID: u.ID, Role: u.Role}).Can(cp) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	a.plugins.ServeAdminUI(w, r, id, rest, "/ext-ui/"+id+"/"+tok+"/", plugin.ServiceUser{ID: u.ID, Name: u.Name, Email: u.Email,
		Role: u.Role, Caps: auth.CapsFor(u.Role, id), Locale: u.Locale})
}

// serviceAPI is called by plugin services (signed requests):
//
//	GET  /api/service/site            scope site     → site title, url, logo, theme…
//	POST /api/service/mail            scope mail     {to, subject, html, replyTo}
//	POST /api/service/events          scope events   {event, data} → emitted as "<plugin>.<event>"
//	GET  /api/service/posts?type=&category=&limit=   scope content  → published posts/pages
//	GET  /api/service/users/{id}      scope users    → {id, name, email, role, avatar}
func (a *API) serviceAPI(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		writeErr(w, http.StatusRequestEntityTooLarge, "request too large")
		return
	}
	id, _, err := a.plugins.VerifyServiceRequest(r, body)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid plugin signature")
		return
	}
	// storage and fetch are answered by the plugin manager, the rest by serviceRoutes
	status, out := a.plugins.ServiceCall(id, r.Method, r.URL.RequestURI(), body)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(out)
}

// serviceDispatch answers a core service-API route for plugin id (wired into
// the plugin manager, which also serves WASM plugins through it).
func (a *API) serviceDispatch(id string, scopes map[string]bool, method, uri string, body []byte) (int, []byte) {
	r := httptest.NewRequest(method, uri, bytes.NewReader(body))
	if opts, err := a.st.Options(); err == nil {
		if u, err := url.Parse(site.BaseURL(opts, nil)); err == nil && u.Host != "" {
			r.Host = u.Host
		}
	}
	w := httptest.NewRecorder()
	a.serviceRoute(w, r, id, scopes, body)
	if w.Body.Len() == 0 {
		return w.Code, []byte(`{}`)
	}
	return w.Code, w.Body.Bytes()
}

func (a *API) serviceRoute(w http.ResponseWriter, r *http.Request, id string, scopes map[string]bool, body []byte) {
	route := strings.Trim(r.URL.Path, "/")
	need := func(scope string) bool {
		if !scopes[scope] {
			writeErr(w, http.StatusForbidden, "the plugin manifest does not request the \""+scope+"\" api scope")
			return false
		}
		return true
	}
	switch {
	case route == "site" && r.Method == http.MethodGet:
		if !need("site") {
			return
		}
		opts, err := a.st.Options()
		if err != nil {
			serverErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"title": opts["site_title"], "tagline": opts["tagline"],
			"url": site.BaseURL(opts, r), "logo": opts["site_logo"], "icon": opts["site_icon"],
			"language": orDefault(opts["language"], "th"), "timezone": orDefault(opts["timezone"], "Asia/Bangkok"),
			"theme": site.ParseTheme(opts["theme"])})
	case route == "mail" && r.Method == http.MethodPost:
		if !need("mail") {
			return
		}
		var in struct {
			To      []string `json:"to"`
			Subject string   `json:"subject"`
			HTML    string   `json:"html"`
			ReplyTo string   `json:"replyTo"`
		}
		if !decodeBody(w, body, &in) {
			return
		}
		if len(in.To) == 0 || len(in.To) > 50 || strings.TrimSpace(in.Subject) == "" {
			writeErr(w, http.StatusBadRequest, "to (1-50 addresses) and subject are required")
			return
		}
		if err := a.SendMail(in.To, in.Subject, in.HTML, in.ReplyTo); err != nil {
			writeErr(w, http.StatusBadGateway, "send failed: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case route == "events" && r.Method == http.MethodPost:
		if !need("events") {
			return
		}
		var in struct {
			Event string         `json:"event"`
			Data  map[string]any `json:"data"`
		}
		if !decodeBody(w, body, &in) {
			return
		}
		if in.Event == "" || len(in.Event) > 60 || strings.ContainsAny(in.Event, " /") {
			writeErr(w, http.StatusBadRequest, "invalid event name")
			return
		}
		a.plugins.Emit(id+"."+in.Event, in.Data)
		w.WriteHeader(http.StatusAccepted)
	case route == "posts" && r.Method == http.MethodGet:
		if !need("content") {
			return
		}
		q := r.URL.Query()
		typ := q.Get("type")
		if typ != model.TypePage {
			typ = model.TypePost
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit < 1 || limit > 100 {
			limit = 20
		}
		posts, _, err := a.st.ListPosts(model.PostFilter{Type: typ, Status: model.StatusPublish, Category: q.Get("category"),
			Search: q.Get("search"), Limit: limit, Order: q.Get("order"), PinnedFirst: true})
		if err != nil {
			serverErr(w, err)
			return
		}
		out := make([]map[string]string, 0, len(posts))
		for _, p := range posts {
			c := site.PostContext(p)
			delete(c, "content")
			out = append(out, c)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	case strings.HasPrefix(route, "users/") && r.Method == http.MethodGet:
		if !need("users") {
			return
		}
		uid, _ := strconv.ParseInt(strings.TrimPrefix(route, "users/"), 10, 64)
		u, err := a.st.UserByID(uid)
		if err != nil {
			writeErr(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": u.ID, "name": u.Name, "email": u.Email, "role": u.Role,
			"avatar": u.Avatar, "status": u.Status})
	default:
		writeErr(w, http.StatusNotFound, "unknown service api route")
	}
}

func decodeBody(w http.ResponseWriter, body []byte, v any) bool {
	if err := json.Unmarshal(body, v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
