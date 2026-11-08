package api

import (
	"errors"
	"html"
	"io"
	"log"
	"net/http"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Phase 7 joints in the admin (code units: Site Code and JS plugins):
//
//	GET /api/v1/code/admin-notices      signed in  [{id, unit, level, text, link?: {label, href, external}}]
//	GET /api/v1/code/dashboard-widgets  signed in  [{id, label, icon, url, height}]
//
// A widget's url is "/ext-ui/code/<ticket>/widget/<id>": the sandboxed
// document its render(ctx) returns (same ticket, kit and anasaki.call as
// admin pages).

func (a *API) codePhase7Routes(h func(pattern, cap string, fn http.HandlerFunc)) {
	const p = "/api/v1/code"
	h("GET "+p+"/admin-notices", "", a.codeAdminNotices)
	h("GET "+p+"/dashboard-widgets", "", a.codeDashboardWidgets)
}

// adminUser is the signed-in user with their admin language (ctx.locale).
func (a *API) adminUser(r *http.Request) *sitecode.User {
	me := auth.From(r)
	u := principalUser(me)
	if mu, _, err := a.st.UserAuthByID(me.ID); err == nil && mu != nil {
		u.Email, u.Locale = mu.Email, mu.Locale
	}
	return u
}

func (a *API) codeAdminNotices(w http.ResponseWriter, r *http.Request) {
	if a.code == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, a.code.AdminNotices(a.adminUser(r)))
}

type widgetOut struct {
	ID     string            `json:"id"`
	Label  map[string]string `json:"label"`
	Icon   string            `json:"icon"`
	URL    string            `json:"url"`
	Height int               `json:"height"`
}

func (a *API) codeDashboardWidgets(w http.ResponseWriter, r *http.Request) {
	out := []widgetOut{}
	if a.code == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	user := a.adminUser(r)
	list := a.code.DashboardWidgets(user)
	if len(list) == 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	_, hash, err := a.st.UserAuthByID(user.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	tok, err := a.auth.IssueTicket(user.ID, hash, "ext-ui:code", "", codeTicketTTL)
	if err != nil {
		serverErr(w, err)
		return
	}
	for _, wd := range list {
		out = append(out, widgetOut{ID: wd.ID, Label: wd.Label, Icon: wd.Icon, URL: "/ext-ui/code/" + tok + "/widget/" + wd.ID, Height: wd.Height})
	}
	writeJSON(w, http.StatusOK, out)
}

// codeWidgetPage serves /ext-ui/code/<ticket>/widget/<id> (sandboxed like an
// admin page).
func (a *API) codeWidgetPage(w http.ResponseWriter, r *http.Request, tok, id string, user *sitecode.User) {
	page := func(status int, title, body string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads; frame-ancestors 'self'")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, sitecode.AdminPageDocumentFor(title, `<div class="ak-widget">`+body+`</div>`, "/ext-ui/code/"+tok+"/", "", sitecode.AdminLocale(user.Locale)))
	}
	body, title, err := a.code.RenderWidget(id, user, r)
	switch {
	case errors.Is(err, sitecode.ErrNoAdminPage):
		page(http.StatusNotFound, "404", `<p>ไม่พบวิดเจ็ตนี้ / Widget not found.</p>`)
	case errors.Is(err, sitecode.ErrAdminPageDenied):
		page(http.StatusForbidden, "403", `<p>คุณไม่มีสิทธิ์ / Forbidden.</p>`)
	case err != nil:
		log.Printf("site code: dashboard widget %s: %v", id, err)
		page(http.StatusInternalServerError, "500", `<p class="ak-muted">เกิดข้อผิดพลาดในวิดเจ็ตนี้ — ดูบันทึกการทำงาน / The widget failed.</p><p class="ak-muted ak-small">`+
			html.EscapeString(id)+`</p>`)
	default:
		page(http.StatusOK, title, body)
	}
}
