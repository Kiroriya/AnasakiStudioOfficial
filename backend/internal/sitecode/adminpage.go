package sitecode

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/design"
)

// Phase 4a: admin pages from code (site.adminPage). The admin shows each in
// the sandboxed iframe host of service plugins: /ext-ui/code/<ticket>/<id>
// serves the HTML render() returns, and anasaki.call(name, input) inside
// posts to /ext-ui/code/<ticket>/_a/<name> to run a site.action as the
// signed-in CMS user.
//
// Phase 6: download(ctx) → RouteResult is served at
// /ext-ui/code/<ticket>/<id>/download?… (anasaki.download(query)), the
// Access screen can override who may open a page (kind "adminPage"), and
// ctx.locale is the signed-in user's admin language.

// AdminPageInfo is a registered site.adminPage.
type AdminPageInfo struct {
	ID     string            `json:"id"`
	Label  map[string]string `json:"label"`
	Icon   string            `json:"icon"`
	Access []string          `json:"access"`
	// Download: the page has a download(ctx) handler (phase 6).
	Download bool `json:"download,omitempty"`
}

type regAdminPage struct {
	info AdminPageInfo
	fn   goja.Callable
	dl   goja.Callable // download (optional)
	at   Problem
}

const maxAdminPages = 50

func (v *vm) siteAdminPage(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.adminPage ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	v.need("admin-pages", "site.adminPage")
	id, _ := c.Argument(0).Export().(string)
	what := fmt.Sprintf("site.adminPage(%q)", id)
	if !bindingNameRe.MatchString(id) {
		v.throw("site.adminPage: id %q ไม่ถูกต้อง (a-z ตัวแรก ตามด้วย a-z A-Z 0-9 _ - ไม่เกิน 63 ตัว)", c.Argument(0).String())
	}
	if len(v.apages) >= maxAdminPages {
		v.throw("ลงทะเบียน site.adminPage ได้ไม่เกิน %d หน้า", maxAdminPages)
	}
	o := optObject(v.rt, c.Argument(1))
	if o == nil {
		v.throw("%s: ต้องระบุ { label, render }", what)
	}
	info := AdminPageInfo{ID: id, Label: v.i18nLabel(o.Get("label"), id, what), Icon: clip(optString(o, "icon"), 16)}
	info.Access = []string{"administrator"}
	if a := o.Get("access"); a != nil && !goja.IsUndefined(a) && !goja.IsNull(a) {
		var list []string
		if err := v.rt.ExportTo(a, &list); err != nil || len(list) == 0 {
			v.throw("%s: access ต้องเป็น array ของ role เช่น ['editor']", what)
		}
		for _, r := range list {
			if !adminEntry(r) {
				v.throw("%s: access: %q ไม่ใช่ role ที่ถูกต้อง (หน้าแอดมินใช้ได้เฉพาะ role ของผู้ดูแล หรือ 'cap:<สิทธิ์>')", what, r)
			}
		}
		v.vetCaps(list, what)
		info.Access = list
	}
	fn := v.fnOpt(o, "render", what)
	var dl goja.Callable
	if d := o.Get("download"); d != nil && !goja.IsUndefined(d) && !goja.IsNull(d) {
		f, ok := goja.AssertFunction(d)
		if !ok {
			v.throw("%s: download ต้องเป็นฟังก์ชัน (ctx) => RouteResult", what)
		}
		dl, info.Download = f, true
	}
	v.apages = append(v.apages, &regAdminPage{info: info, fn: fn, dl: dl, at: v.caller()})
	return goja.Undefined()
}

func (p *Program) registerAdminPages(v *vm) []Problem {
	var probs []Problem
	p.adminPages = map[string]*AdminPageInfo{}
	for _, a := range v.apages {
		if _, dup := p.adminPages[a.info.ID]; dup {
			probs = append(probs, at(a.at, fmt.Sprintf("site.adminPage %q ซ้ำกับที่ลงทะเบียนไว้แล้ว", a.info.ID)))
			continue
		}
		info := a.info
		p.adminPages[info.ID] = &info
		p.Manifest.AdminPages = append(p.Manifest.AdminPages, info)
	}
	return probs
}

// adminAccess is the effective access of admin page qid (the Access screen
// override, else the code's list).
func (e *Engine) adminAccess(qid string, info *AdminPageInfo) []string {
	return e.Effective(KindAdminPage, qid, info.Access)
}

// AdminPages lists the live admin pages user may open (none while Site
// Code is off).
func (e *Engine) AdminPages(user *User) []AdminPageInfo {
	out := []AdminPageInfo{}
	s := e.adminSession(nil, user)
	if s == nil {
		return out
	}
	defer s.Close()
	for _, lp := range s.plugins().loaded() {
		for _, a := range lp.prog.Manifest.AdminPages {
			a.ID = lp.unit.qualify(a.ID)
			if allowed(e.adminAccess(a.ID, &a), user) {
				out = append(out, a)
			}
		}
	}
	for _, a := range s.sp().Manifest.AdminPages {
		if allowed(e.adminAccess(a.ID, &a), user) {
			out = append(out, a)
		}
	}
	return out
}

// adminSession is a live session for the admin pages (nil when Site Code is
// off or nothing runs).
func (e *Engine) adminSession(r *http.Request, user *User) *Session {
	opts, err := e.st.Options()
	if err != nil {
		return nil
	}
	if r == nil {
		r, _ = http.NewRequest(http.MethodGet, "http://site.local/", nil)
	}
	s := e.LiveSession(r, opts)
	if s != nil {
		s.user, s.admin = user, true
		if user != nil {
			s.locale, s.lang = AdminLocale(user.Locale), user.Locale
		}
	}
	return s
}

// AdminLocale is ctx.locale for a user's admin language: "en" or "th".
func AdminLocale(l string) string {
	// ctx.locale stays th | en for plugins written for two languages; the other admin languages (2.2.2) read
	// English, and the full code is ctx.lang
	l = strings.ToLower(strings.TrimSpace(l))
	if l == "" || strings.HasPrefix(l, "th") {
		return "th"
	}
	return "en"
}

// adminPageInfo finds admin page id ("<plugin>:<page>" for a JS plugin).
func (s *Session) adminPageInfo(id string) (string, string, *AdminPageInfo) {
	unit, local := SplitUnit(id)
	p := s.progOf(unit)
	if p == nil {
		return unit, local, nil
	}
	return unit, local, p.adminPages[local]
}

// AdminPageAllowed reports whether user may open admin page id (false when
// it does not exist or Site Code is off).
func (e *Engine) AdminPageAllowed(id string, user *User) bool {
	s := e.adminSession(nil, user)
	if s == nil {
		return false
	}
	defer s.Close()
	_, _, info := s.adminPageInfo(id)
	return info != nil && allowed(e.adminAccess(id, info), user)
}

// Errors of RenderAdminPage.
var (
	ErrNoAdminPage     = errors.New("admin page not found")
	ErrAdminPageDenied = errors.New("admin page denied")
)

func queryMap(r *http.Request) map[string]any {
	q := map[string]any{}
	for k, vals := range r.URL.Query() {
		if len(vals) == 1 {
			q[k] = vals[0]
		} else {
			q[k] = vals
		}
	}
	return q
}

// RenderAdminPage runs site.adminPage id's render as user: HTML and title.
func (e *Engine) RenderAdminPage(id string, user *User, r *http.Request) (body, title string, err error) {
	s := e.adminSession(r, user)
	if s == nil {
		return "", "", ErrNoAdminPage
	}
	defer s.Close()
	unit, local, info := s.adminPageInfo(id)
	if info == nil {
		return "", "", ErrNoAdminPage
	}
	if !allowed(e.adminAccess(id, info), user) {
		return "", "", ErrAdminPageDenied
	}
	q := queryMap(r)
	out, err := s.callIn(unit, "adminPage "+local, e.RouteTimeout, func(v *vm) goja.Callable {
		for _, a := range v.apages {
			if a.info.ID == local {
				return a.fn
			}
		}
		return nil
	}, func(v *vm) []goja.Value {
		return []goja.Value{s.ctxObject(v, map[string]any{"query": q, "page": id}, nil)}
	})
	if err != nil {
		return "", "", err
	}
	title = info.Label["th"]
	if s.locale == "en" && info.Label["en"] != "" {
		title = info.Label["en"]
	}
	switch x := out.(type) {
	case string:
		body = x
	case map[string]any:
		body, _ = x["html"].(string)
		if t, ok := x["title"].(string); ok && t != "" {
			title = t
		}
	case nil:
	default:
		body = design.Stringify(x)
	}
	return body, title, nil
}

// AdminDownload runs admin page id's download(ctx) as user (phase 6): the
// result is a route response (string, JSON, binary body or bodyBase64),
// served as an attachment.
func (e *Engine) AdminDownload(id string, user *User, r *http.Request) (*Response, error) {
	s := e.adminSession(r, user)
	if s == nil {
		return nil, ErrNoAdminPage
	}
	defer s.Close()
	unit, local, info := s.adminPageInfo(id)
	if info == nil || !info.Download {
		return nil, ErrNoAdminPage
	}
	if !allowed(e.adminAccess(id, info), user) {
		return nil, ErrAdminPageDenied
	}
	q := queryMap(r)
	out, err := s.callInWith(unit, "adminPage "+local+" download", e.RouteTimeout, func(v *vm) goja.Callable {
		for _, a := range v.apages {
			if a.info.ID == local {
				return a.dl
			}
		}
		return nil
	}, func(v *vm) []goja.Value {
		return []goja.Value{s.ctxObject(v, map[string]any{"query": q, "page": id}, nil)}
	}, func(v *vm, val goja.Value) (any, error) {
		return v.response(val)
	})
	if err != nil {
		return nil, err
	}
	res, _ := out.(*Response)
	if res == nil {
		return nil, &RunError{Message: "download ไม่ได้คืนผลลัพธ์"}
	}
	res.Cookies = nil // an admin download sets no site cookies
	cd := strings.TrimSpace(res.Headers["Content-Disposition"])
	switch {
	case cd == "":
		res.Headers["Content-Disposition"] = "attachment"
	case !strings.HasPrefix(strings.ToLower(cd), "attachment"):
		if i := strings.Index(cd, ";"); i >= 0 {
			res.Headers["Content-Disposition"] = "attachment" + cd[i:]
		} else {
			res.Headers["Content-Disposition"] = "attachment"
		}
	}
	return res, nil
}

// AdminAction runs site.action name for an admin page (anasaki.call): the
// caller is the signed-in CMS user of the ticket.
func (e *Engine) AdminAction(name string, input map[string]any, user *User, r *http.Request) *design.ActionOutcome {
	s := e.adminSession(r, user)
	if s == nil {
		return &design.ActionOutcome{Status: http.StatusNotFound, Error: "ไม่พบ action นี้"}
	}
	defer s.Close()
	if input == nil {
		input = map[string]any{}
	}
	return s.RunAction(name, input)
}

// AdminPageDocument wraps a rendered admin page: a small stylesheet and the
// anasaki.call helper (base is "/ext-ui/code/<ticket>/").
func AdminPageDocument(title, body, base string) string {
	return AdminPageDocumentFor(title, body, base, "", "th")
}

// AdminPageDocumentFor is AdminPageDocument for page id (anasaki.download)
// in locale ("th" | "en"): it also links the admin UI kit
// (/ext-ui/kit.css, /ext-ui/kit.js).
func AdminPageDocumentFor(title, body, base, page, locale string) string {
	if locale != "en" {
		locale = "th"
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="` + locale + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<title>` + html.EscapeString(title) + `</title>`)
	b.WriteString(`<link rel="stylesheet" href="/ext-ui/kit.css?v=` + KitVersion + `">`)
	b.WriteString(`<style>body{font:15px/1.5 system-ui,-apple-system,"Segoe UI","Noto Sans Thai",sans-serif;margin:0;padding:24px}` +
		`table{border-collapse:collapse;width:100%}th,td{border-bottom:1px solid var(--ak-border,#e5e7eb);padding:8px;text-align:left}` +
		`button{font:inherit;padding:6px 14px;border-radius:8px;border:1px solid var(--ak-border,#d1d5db);background:var(--ak-surface,#fff);color:inherit;cursor:pointer}` +
		`button.primary{background:#4f46e5;color:#fff;border-color:#4f46e5}input,select,textarea{font:inherit;padding:6px 10px;border:1px solid var(--ak-border,#d1d5db);border-radius:8px}</style>`)
	b.WriteString(`<script>window.anasaki={base:` + jsString(base) + `,page:` + jsString(page) + `,locale:` + jsString(locale) +
		`,call:function(name,input){return fetch(this.base+"_a/"+encodeURIComponent(name),` +
		`{method:"POST",headers:{"Content-Type":"text/plain;charset=utf-8"},body:JSON.stringify({input:input||{}})}).then(function(r){` +
		`return r.json().catch(function(){return {}}).then(function(d){if(!r.ok){var e=new Error(d.error||("HTTP "+r.status));e.status=r.status;e.errors=d.errors;throw e}return d})})},` +
		`downloadUrl:function(query){var q="";if(typeof query==="string"){q=query.replace(/^\?/,"")}else if(query){q=new URLSearchParams(query).toString()}` +
		`return this.base+encodeURIComponent(this.page)+"/download"+(q?"?"+q:"")},` +
		`download:function(query){var a=document.createElement("a");a.href=this.downloadUrl(query);a.download="";a.rel="noopener";a.style.display="none";` +
		`document.body.appendChild(a);a.click();setTimeout(function(){a.remove()},1000)}};</script>`)
	b.WriteString(`<script src="/ext-ui/kit.js?v=` + KitVersion + `" defer></script>`)
	b.WriteString(`</head><body class="ak">`)
	b.WriteString(body)
	b.WriteString(`</body></html>`)
	return b.String()
}

// jsString quotes s as a JS string literal safe inside <script>.
func jsString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "<", `\u003c`, ">", `\u003e`, "\n", `\n`, "\r", `\r`, "\u2028", `\u2028`, "\u2029", `\u2029`)
	return `"` + r.Replace(s) + `"`
}
