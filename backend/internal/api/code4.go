package api

import (
	"encoding/json"
	"errors"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Site Code phase 4a admin API:
//
//	GET  /api/v1/code/settings                  manage_scripts  {values:[{key,secret,set,value?}], fetchHosts, limits:{mailPerHour,routePerMinute}}
//	PUT  /api/v1/code/settings                  manage_scripts  {values:[{key,secret,value}], fetchHosts?, limits?, remove?:[keys]} → same as GET
//	GET  /api/v1/code/jobs?status=&name=&page=  view_scripts    {items, total, page, perPage}
//	POST /api/v1/code/jobs/{id}/cancel|retry    publish_scripts {ok:true}
//	GET  /api/v1/code/cron                      view_scripts    [{name, expr, job, nextRun, lastRun, lastStatus}]
//	POST /api/v1/code/cron/{name}/run           publish_scripts {jobId}
//	GET  /api/v1/code/admin-pages               signed in       [{id, label, icon}] the pages the user's role may open
//	POST /api/v1/code/admin-pages/{id}/ticket   signed in       {query?} → {url: "/ext-ui/code/<ticket>/<id>?<query>"}
//
// and the iframe host /ext-ui/code/<ticket>/<id> (+ POST …/_a/<action>).

func (a *API) codePhase4Routes(h, c func(pattern, cap string, fn http.HandlerFunc)) {
	const p = "/api/v1/code"
	c("GET "+p+"/settings", auth.CapManageScripts, a.codeSettings)
	c("PUT "+p+"/settings", auth.CapManageScripts, a.codeSaveSettings)
	c("GET "+p+"/jobs", auth.CapViewScripts, a.codeJobs)
	c("POST "+p+"/jobs/{id}/cancel", auth.CapPublishScripts, a.codeJobCancel)
	c("POST "+p+"/jobs/{id}/retry", auth.CapPublishScripts, a.codeJobRetry)
	c("GET "+p+"/cron", auth.CapViewScripts, a.codeCron)
	c("POST "+p+"/cron/{name}/run", auth.CapPublishScripts, a.codeCronRun)
	h("GET "+p+"/admin-pages", "", a.codeAdminPages)
	h("POST "+p+"/admin-pages/{id}/ticket", "", a.codeAdminTicket)
}

// ---------------------------------------------------------------- settings

func (a *API) codeSettings(w http.ResponseWriter, r *http.Request) {
	s, err := a.code.SettingsOf(r.URL.Query().Get("unit"))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *API) codeSaveSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		sitecode.SettingsInput
		Unit string `json:"unit"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	unit := in.Unit
	if unit == "" {
		unit = r.URL.Query().Get("unit")
	}
	if err := a.code.SaveSettingsOf(in.SettingsInput, auth.From(r).ID, unit); err != nil {
		var bad *sitecode.ErrBadSettings
		if errors.As(err, &bad) {
			writeErr(w, http.StatusBadRequest, bad.Msg)
			return
		}
		serverErr(w, err)
		return
	}
	keys := []string{}
	for _, v := range in.Values {
		keys = append(keys, v.Key)
	}
	detail := map[string]any{"keys": keys, "remove": in.Remove, "unit": unit}
	if in.FetchHosts != nil {
		detail["fetchHosts"] = *in.FetchHosts
	}
	if in.Limits != nil {
		detail["limits"] = in.Limits
	}
	a.codeAudit(r, "settings", detail) // never the values
	s, err := a.code.SettingsOf(unit)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// ---------------------------------------------------------------- jobs and cron

func (a *API) codeJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status != "" && !sitecode.IsJobStatus(status) {
		writeErr(w, http.StatusBadRequest, "status ต้องเป็น pending, running, done, failed หรือ cancelled")
		return
	}
	page := max(queryInt(r, "page", 1), 1)
	perPage := queryInt(r, "perPage", 50)
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}
	items, total, err := a.code.JobsOf(q.Get("unit"), status, q.Get("name"), page, perPage)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "perPage": perPage})
}

func (a *API) jobChange(w http.ResponseWriter, r *http.Request, action string, fn func(int64) error) {
	id := pathID(r, "id")
	if err := fn(id); err != nil {
		switch {
		case sitecode.IsNotFound(err):
			writeErr(w, http.StatusNotFound, "ไม่พบงานนี้")
		case errors.Is(err, sitecode.ErrJobState):
			writeErr(w, http.StatusConflict, "สถานะของงานนี้เปลี่ยนไม่ได้ (ยกเลิกได้เฉพาะงานที่รอ/กำลังทำ, ลองใหม่ได้เฉพาะงานที่ล้มเหลว/ถูกยกเลิก)")
		default:
			serverErr(w, err)
		}
		return
	}
	a.codeAudit(r, action, map[string]any{"id": id})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *API) codeJobCancel(w http.ResponseWriter, r *http.Request) {
	a.jobChange(w, r, "job_cancel", a.code.CancelJob)
}

func (a *API) codeJobRetry(w http.ResponseWriter, r *http.Request) {
	a.jobChange(w, r, "job_retry", a.code.RetryJob)
}

func (a *API) codeCron(w http.ResponseWriter, r *http.Request) {
	rows, err := a.code.CronsOf(r.URL.Query().Get("unit"))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *API) codeCronRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id, err := a.code.RunCronNow(name)
	if err != nil {
		var bad *sitecode.ErrBadSettings
		switch {
		case errors.As(err, &bad):
			writeErr(w, http.StatusConflict, bad.Msg)
		case sitecode.IsNotFound(err):
			writeErr(w, http.StatusNotFound, "ไม่พบตารางเวลานี้ในโค้ดที่เผยแพร่")
		default:
			serverErr(w, err)
		}
		return
	}
	a.codeAudit(r, "cron_run", map[string]any{"name": name, "jobId": id})
	writeJSON(w, http.StatusOK, map[string]any{"jobId": id})
}

// ---------------------------------------------------------------- admin pages

type adminPageOut struct {
	ID    string            `json:"id"`
	Label map[string]string `json:"label"`
	Icon  string            `json:"icon"`
}

func principalUser(p *auth.Principal) *sitecode.User {
	return &sitecode.User{ID: p.ID, Name: p.Name, Role: p.Role}
}

func (a *API) codeAdminPages(w http.ResponseWriter, r *http.Request) {
	out := []adminPageOut{}
	for _, pg := range a.code.AdminPages(principalUser(auth.From(r))) {
		out = append(out, adminPageOut{ID: pg.ID, Label: pg.Label, Icon: pg.Icon})
	}
	writeJSON(w, http.StatusOK, out)
}

// codeTicketTTL: the iframe URL of a code admin page.
const codeTicketTTL = 8 * time.Hour

func (a *API) codeAdminTicket(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query any `json:"query"`
	}
	if r.ContentLength != 0 && !readJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	me := auth.From(r)
	if !a.code.AdminPageAllowed(id, principalUser(me)) {
		writeErr(w, http.StatusNotFound, "ไม่พบหน้านี้ หรือคุณไม่มีสิทธิ์เปิด")
		return
	}
	_, hash, err := a.st.UserAuthByID(me.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	tok, err := a.auth.IssueTicket(me.ID, hash, "ext-ui:code", "", codeTicketTTL)
	if err != nil {
		serverErr(w, err)
		return
	}
	u := "/ext-ui/code/" + tok + "/" + id
	switch q := in.Query.(type) {
	case string:
		if q = strings.TrimPrefix(strings.TrimSpace(q), "?"); q != "" {
			if v, err := url.ParseQuery(q); err == nil {
				u += "?" + v.Encode()
			}
		}
	case map[string]any:
		v := url.Values{}
		for k, x := range q {
			if s, ok := x.(string); ok {
				v.Set(k, s)
			}
		}
		if len(v) > 0 {
			u += "?" + v.Encode()
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": u})
}

// codeExtUI serves /ext-ui/code/<ticket>/<page> (the rendered admin page,
// sandboxed) and POST /ext-ui/code/<ticket>/_a/<action> (anasaki.call). The
// ticket's user is re-checked against the database on every request.
func (a *API) codeExtUI(w http.ResponseWriter, r *http.Request, tok, rest string) {
	cors := func() {
		w.Header().Set("Access-Control-Allow-Origin", "*") // the sandboxed page has an opaque origin; the ticket authenticates
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Cache-Control", "no-store")
	}
	isAction := strings.HasPrefix(rest, "_a/")
	if isAction && r.Method == http.MethodOptions {
		cors()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	t, err := a.auth.ParseTicket(tok, "ext-ui:code")
	var u *model.User
	var hash string
	if err == nil {
		u, hash, err = a.st.UserAuthByID(t.UserID)
	}
	if err != nil || !a.auth.TicketCurrent(t, hash) || u.Status == model.UserSuspended {
		if isAction {
			cors()
			writeErr(w, http.StatusUnauthorized, "ลิงก์หมดอายุ — เปิดหน้านี้ใหม่จากเมนูของระบบจัดการ")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `<p style="font:15px system-ui;padding:32px">ลิงก์หมดอายุ — เปิดหน้านี้ใหม่จากเมนูของระบบจัดการ<br>This link expired; open the page again from the admin menu.</p>`)
		return
	}
	user := &sitecode.User{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role, Locale: u.Locale}
	if isAction {
		cors()
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		name := strings.TrimPrefix(rest, "_a/")
		var body struct {
			Input map[string]any `json:"input"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeErr(w, http.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
			return
		}
		res := a.code.AdminAction(name, body.Input, user, r)
		out := map[string]any{}
		if res.Status != http.StatusOK {
			out["error"] = res.Error
		}
		if len(res.Errors) > 0 {
			out["errors"] = res.Errors
		}
		if res.Message != "" {
			out["message"] = res.Message
		}
		if res.Redirect != "" {
			out["redirect"] = res.Redirect
		}
		if res.Data != nil {
			out["data"] = res.Data
		}
		writeJSON(w, res.Status, out)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(rest, "/")
	if wid, ok := strings.CutPrefix(id, "widget/"); ok { // phase 7: site.dashboardWidget
		a.codeWidgetPage(w, r, tok, wid, user)
		return
	}
	if pageID, ok := strings.CutSuffix(id, "/download"); ok { // phase 6: download(ctx) of the page (top-level navigation)
		a.codeAdminDownload(w, r, pageID, user)
		return
	}
	page := func(status int, title, body string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		// allow-popups-to-escape-sandbox: a tab the page opens (target=_blank, e.g. the plugin's own public page) is a
		// normal page of its URL; without it the tab inherits the sandbox (origin "null") and its forms all fail
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads; frame-ancestors 'self'")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, sitecode.AdminPageDocumentFor(title, body, "/ext-ui/code/"+tok+"/", id, sitecode.AdminLocale(u.Locale)))
	}
	body, title, err := a.code.RenderAdminPage(id, user, r)
	switch {
	case errors.Is(err, sitecode.ErrNoAdminPage):
		page(http.StatusNotFound, "404", `<p>ไม่พบหน้านี้ (อาจถูกลบออกจากโค้ด) / Page not found.</p>`)
	case errors.Is(err, sitecode.ErrAdminPageDenied):
		page(http.StatusForbidden, "403", `<p>คุณไม่มีสิทธิ์เปิดหน้านี้ / Forbidden.</p>`)
	case err != nil:
		log.Printf("site code: admin page %s: %v", id, err)
		page(http.StatusInternalServerError, "500", `<p>เกิดข้อผิดพลาดในโค้ดของหน้านี้ — ดูรายละเอียดในบันทึกการทำงาน (Site code → Logs)</p><p style="color:#6b7280">`+
			html.EscapeString("adminPage "+id)+`</p>`)
	default:
		page(http.StatusOK, title, body)
	}
}

// codeAdminDownload serves GET /ext-ui/code/<ticket>/<page>/download?…: the
// page's download(ctx) result as an attachment (same ticket and access as
// the page).
func (a *API) codeAdminDownload(w http.ResponseWriter, r *http.Request, id string, user *sitecode.User) {
	fail := func(status int, msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `<p style="font:15px system-ui;padding:32px">`+html.EscapeString(msg)+`</p>`)
	}
	res, err := a.code.AdminDownload(id, user, r)
	switch {
	case errors.Is(err, sitecode.ErrNoAdminPage):
		fail(http.StatusNotFound, "ไม่พบไฟล์ดาวน์โหลดของหน้านี้ / Not found.")
		return
	case errors.Is(err, sitecode.ErrAdminPageDenied):
		fail(http.StatusForbidden, "คุณไม่มีสิทธิ์ / Forbidden.")
		return
	case err != nil:
		log.Printf("site code: admin page %s download: %v", id, err)
		fail(http.StatusInternalServerError, "สร้างไฟล์ไม่สำเร็จ — ดูบันทึกการทำงาน (Site code → Logs) / The download failed.")
		return
	}
	h := w.Header()
	for k, v := range res.Headers {
		h.Set(k, v)
	}
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.WriteHeader(res.Status)
	if r.Method != http.MethodHead && res.Status != http.StatusNoContent {
		_, _ = w.Write([]byte(res.Body))
	}
}
