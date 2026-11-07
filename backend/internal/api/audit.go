package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/store"
)

// Admin activity log (2.4.0): every change made through the signed-in API
// (POST / PUT / PATCH / DELETE) and every sign-in is recorded with who, when,
// from where and the answer's status — never the request body, so passwords,
// keys and content stay out of it. GET /api/v1/audit lists it (manage_users).

// auditSkip are routes that change nothing worth a line, or run all the
// time while someone works (presence beats, session renewal, previews).
var auditSkip = []string{"/design/presence", "/auth/refresh", "/blocks/render", "/preview", "/ticket", "/ui-ticket", "/code/build"}

func auditable(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	for _, s := range auditSkip {
		if strings.HasSuffix(r.URL.Path, s) {
			return false
		}
	}
	return true
}

// audited wraps a protected handler: changes are written to the log after
// the handler answered (the principal is set by then).
func (a *API) audited(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auditable(r) {
			fn(w, r)
			return
		}
		target, label := auditTarget(r), a.auditLabel(r)
		rec := &plugin.StatusRecorder{ResponseWriter: w}
		if target == "" && r.Method == http.MethodPost {
			// a create: the new item's id and name come back in the answer
			cr := &createdRecorder{StatusRecorder: rec}
			fn(cr, r)
			target, label = cr.created()
		} else {
			fn(rec, r)
		}
		p := auth.From(r)
		if p == nil {
			return
		}
		via := "session"
		if p.TokenID > 0 {
			via = "token"
		}
		a.audit(r, store.AuditEntry{UserID: p.ID, UserName: p.Name, Target: target, Label: label, Status: max(rec.Status, http.StatusOK), Via: via})
	}
}

// createdRecorder keeps the start of a create's answer, to name what was made.
type createdRecorder struct {
	*plugin.StatusRecorder
	buf []byte
}

func (c *createdRecorder) Write(b []byte) (int, error) {
	if room := 1<<20 - len(c.buf); room > 0 { // a page answer carries its design
		c.buf = append(c.buf, b[:min(len(b), room)]...)
	}
	return c.StatusRecorder.Write(b)
}

// created reads {id, title|name|filename} of a 2xx JSON answer.
func (c *createdRecorder) created() (target, label string) {
	if c.Status >= 300 || len(c.buf) == 0 || c.buf[0] != '{' {
		return "", ""
	}
	var v struct {
		ID       json.RawMessage `json:"id"`
		Title    string          `json:"title"`
		Name     string          `json:"name"`
		Filename string          `json:"filename"`
	}
	if json.Unmarshal(c.buf, &v) != nil {
		return "", ""
	}
	label = v.Title
	if label == "" {
		label = v.Name
	}
	if label == "" {
		label = v.Filename
	}
	return strings.Trim(string(v.ID), `"`), label
}

// audit writes one line for r (method, route and IP are filled in here).
func (a *API) audit(r *http.Request, e store.AuditEntry) {
	if e.Method == "" {
		e.Method = r.Method
	}
	if e.Route == "" {
		e.Route = r.Pattern
		if e.Route == "" {
			e.Route = r.Method + " " + r.URL.Path
		}
	}
	e.IP = plugin.ClientIP(r)
	if err := a.st.AddAudit(e); err != nil {
		log.Printf("admin audit: %v", err)
	}
}

// auditTarget is what the route named: its {id}, {slug}, {name} or {path}.
func auditTarget(r *http.Request) string {
	for _, k := range []string{"id", "slug", "name", "path", "t"} {
		if v := r.PathValue(k); v != "" {
			if k == "t" { // a table of the site's database, with its row
				if pk := r.PathValue("pk"); pk != "" {
					return v + "/" + pk
				}
			}
			return v
		}
	}
	return ""
}

// auditLabel is a readable name of the target, read before the change so a
// deleted page keeps its title in the log ("" when there is none).
func (a *API) auditLabel(r *http.Request) string {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return ""
	}
	switch strings.SplitN(path, "/", 2)[0] {
	case "posts":
		if p, err := a.st.GetPost(id); err == nil {
			return p.Title
		}
	case "users":
		if u, err := a.st.UserByID(id); err == nil {
			return u.Name + " <" + u.Email + ">"
		}
	case "media":
		if m, err := a.st.GetMedia(id); err == nil {
			return m.Filename
		}
	case "menus":
		if m, err := a.st.GetMenu(id); err == nil {
			return m.Name
		}
	case "components":
		if c, err := a.st.GetComponent(id); err == nil {
			return c.Name
		}
	case "terms":
		if t, err := a.st.GetTerm(id); err == nil {
			return t.Name
		}
	}
	return ""
}

// listAudit answers GET /api/v1/audit?page&perPage&user&q&kind&failed.
func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	per := queryInt(r, "perPage", 50)
	page := max(1, queryInt(r, "page", 1))
	uid, _ := strconv.ParseInt(q.Get("user"), 10, 64)
	items, total, err := a.st.ListAudit(store.AuditFilter{UserID: uid, Search: q.Get("q"), Kind: q.Get("kind"),
		Failed: q.Get("failed") == "1" || q.Get("failed") == "true", Limit: per, Offset: (page - 1) * per})
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page})
}
