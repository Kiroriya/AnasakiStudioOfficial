package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// plugins-official/forms: the built-in form builder (internal/plugins/forms and its tests) through public
// joints — admin actions (validation), the designer block, the form markup, submissions (urlencoded /
// multipart / JSON, honeypot, rate limit, validation, redirects), the event, the mail notification, the
// inbox, the CSV export and the legacy import.

func offBTestForm() map[string]any {
	return map[string]any{"name": "Contact", "submitLabel": "Send <now>", "fields": []any{
		map[string]any{"key": "name", "label": "Name <b>", "type": "text", "required": true, "placeholder": `"quoted"`},
		map[string]any{"key": "email", "label": "Email", "type": "email", "required": true},
		map[string]any{"key": "topic", "label": "Topic", "type": "select", "options": []any{"Sales", "Support <x>"}},
		map[string]any{"key": "agree", "label": "Agree", "type": "checkbox"},
		map[string]any{"key": "msg", "label": "Message", "type": "textarea"},
	}}
}

func TestOfficialForms(t *testing.T) {
	e := offBNew(t)
	admin, user := setupSite(t, e.c)
	offBInstall(t, admin, "forms")
	if !codeTable(t, e.pluginEnv, "p_forms_forms") || !codeTable(t, e.pluginEnv, "p_forms_submissions") {
		t.Fatal("migration did not create the tables")
	}
	// the designer block: same node as the built-in ({plugin: forms, block: form, values: {formId}})
	_, blocks := admin.doAny("GET", "/api/v1/blocks", nil)
	if !strings.Contains(fmt.Sprint(blocks), "plugin:forms") || !strings.Contains(fmt.Sprint(blocks), "key:formId") {
		t.Fatalf("blocks: %v", blocks)
	}

	// admin page (editors and administrators) and its actions
	html, call := adminPage(t, e.toSample(), admin, "forms:forms", "")
	offBContains(t, "admin page", html, "<title>Forms</title>", `<script type="application/json" id="forms-data">{"locale":"en","forms":[]`,
		`src="/plugin-assets/forms/assets/admin.js?v=2.2.0" defer`)
	if rec := e.offBSend(offBReq{path: "/plugin-assets/forms/assets/admin.js?v=2.2.0"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "anasaki.call('forms:'") {
		t.Fatalf("admin script: %d", rec.Code)
	}
	if code, _ := user("Au", "author").do("POST", "/api/v1/code/admin-pages/forms:forms/ticket", map[string]any{}); code == 200 {
		t.Fatal("author may open forms")
	}
	_, edCall := adminPage(t, e.toSample(), user("Ed", "editor"), "forms:forms", "")
	if code, out := edCall("forms:list", map[string]any{}); code != 200 || out["data"] == nil {
		t.Fatalf("editor list: %d %v", code, out)
	}

	save := func(form map[string]any) (map[string]any, string) {
		t.Helper()
		code, out := call("forms:save", map[string]any{"form": form})
		if code != 200 {
			t.Fatalf("save: %d %v", code, out)
		}
		if errs, ok := out["errors"].(map[string]any); ok && len(errs) > 0 {
			return nil, fmt.Sprint(errs["form"])
		}
		return out["data"].(map[string]any), ""
	}
	one := []any{map[string]any{"key": "a"}}
	bad := []map[string]any{
		{"name": "", "fields": offBTestForm()["fields"]},
		{"name": "x"},
		{"name": "x", "fields": []any{map[string]any{"key": "_hp", "type": "text"}}},
		{"name": "x", "fields": []any{map[string]any{"key": "a", "type": "video"}}},
		{"name": "x", "fields": []any{map[string]any{"key": "a", "type": "select"}}},
		{"name": "x", "fields": []any{map[string]any{"key": "a"}, map[string]any{"key": "a"}}},
		{"name": "x", "fields": one, "webhookUrl": "ftp://x"},
		{"name": "x", "fields": one, "redirectUrl": "javascript:alert(1)"},
		{"name": "x", "fields": one, "redirectUrl": "https://evil.example/phish"},
		{"name": "x", "fields": one, "redirectUrl": "//evil.example"},
		{"name": "x", "fields": one, "redirectUrl": `/\evil.example`},
		{"name": "x", "fields": one, "redirectUrl": "thank-you"},
	}
	for i, f := range bad {
		if _, msg := save(f); msg == "" {
			t.Errorf("case %d should be invalid", i)
		}
	}
	if _, msg := save(bad[0]); msg != "กรุณาตั้งชื่อฟอร์ม" {
		t.Fatalf("message: %q", msg)
	}
	if _, msg := save(bad[2]); msg != `คีย์ของช่อง "_hp" ไม่ถูกต้องหรือซ้ำ (ใช้ a-z, 0-9, _)` {
		t.Fatalf("message: %q", msg)
	}
	local := map[string]any{"name": "Local", "fields": one, "redirectUrl": " /thank-you?from=form#top "}
	lf, msg := save(local)
	if msg != "" || lf["redirectUrl"] != "/thank-you?from=form#top" || lf["fields"].([]any)[0].(map[string]any)["label"] != "a" {
		t.Fatalf("local redirect: %q %v", msg, lf)
	}
	fm, msg := save(offBTestForm())
	if msg != "" {
		t.Fatal(msg)
	}
	id := int(fm["id"].(float64))
	sid := fmt.Sprint(id)

	// the form markup (RenderForm)
	rec := e.offBSend(offBReq{path: "/forms/" + sid + "/embed"})
	out := rec.Body.String()
	offBContains(t, "embed", out, `action="/forms/`+sid+`/submit"`, `data-rea-form="`+sid+`" data-success="ขอบคุณ! เราได้รับข้อความของคุณแล้ว"`,
		`<label for="rea-f`+sid+`-name">Name &lt;b&gt;<span class="rea-form-req" aria-hidden="true"> *</span></label>`,
		`placeholder="&#34;quoted&#34;" required`, `type="email" name="email"`, `<option value="Support &lt;x&gt;">`,
		`name="_hp" tabindex="-1"`, `>Send &lt;now&gt;</button>`, `role="status"`, `<textarea name="msg"`,
		`<input type="checkbox" name="agree" id="rea-f`+sid+`-agree" value="yes"> Agree</label>`)
	if strings.Contains(out, "<b>") {
		t.Fatal("label not escaped")
	}
	for p, want := range map[string]string{"/forms/0/embed": "Choose a form", "/forms/abc/embed": "Choose a form", "/forms/999999/embed": "Form not found"} {
		if b := e.offBSend(offBReq{path: p}).Body.String(); !strings.Contains(b, want) {
			t.Fatalf("%s: %s", p, b)
		}
	}
	// the designer block: a select of the existing forms (dynamic options)
	_, blocks = admin.doAny("GET", "/api/v1/blocks", nil)
	if s := fmt.Sprint(blocks); !strings.Contains(s, "value:"+sid) || !strings.Contains(s, "#"+sid+" — Contact") {
		t.Fatalf("form select: %v", blocks)
	}
	// the block on a designed page: rendered on the server (works without JavaScript), the client script loaded with it
	_, pg := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Contact us", "slug": "contact-us", "status": "publish",
		"design": map[string]any{"id": "root", "type": "frame", "children": []any{
			map[string]any{"id": "pb", "type": "pluginBlock", "props": map[string]any{"plugin": "forms", "block": "form", "values": map[string]any{"formId": sid}}}}}})
	if pg["id"] == nil {
		t.Fatalf("page: %v", pg)
	}
	out = e.offBSend(offBReq{path: "/contact-us"}).Body.String()
	offBContains(t, "page with the block", out, `pb-forms-form`, `<form class="rea-form" id="rea-form-`+sid+`" method="post" action="/forms/`+sid+`/submit"`,
		`name="_hp" tabindex="-1"`, `<label for="rea-f`+sid+`-name">Name &lt;b&gt;`, `.pb-forms-form .rea-form{`,
		`<script defer src="/plugin-assets/forms/assets/form.js?v=2.2.0"></script>`, `role="status"`)
	if strings.Contains(out, `<p class="rea-form-msg" role="status" aria-live="polite">`) {
		t.Fatal("success message shown without ?sent")
	}
	// back from a no-JavaScript submit (?sent=<id>): the server shows the success message itself
	if out := e.offBSend(offBReq{path: "/contact-us?sent=" + sid}).Body.String(); !strings.Contains(out, `<p class="rea-form-msg" role="status" aria-live="polite">`) {
		t.Fatalf("no success message after ?sent=%s:\n%s", sid, out)
	}
	if out := e.offBSend(offBReq{path: "/contact-us?sent=999"}).Body.String(); strings.Contains(out, `<p class="rea-form-msg" role="status" aria-live="polite">`) {
		t.Fatal("success message shown for another form's ?sent")
	}
	if js := e.offBSend(offBReq{path: "/plugin-assets/forms/assets/form.js?v=2.2.0"}).Body.String(); !strings.Contains(js, "window.__reaForms") {
		t.Fatalf("client script: %s", js)
	}
	// the designer preview of the block
	if code, pv := admin.do("POST", "/api/v1/blocks/render", map[string]any{"plugin": "forms", "block": "form", "values": map[string]any{}}); code != 200 ||
		!strings.Contains(fmt.Sprint(pv["html"]), "Choose a form") {
		t.Fatalf("preview: %d %v", code, pv)
	}
	// 2.0.3: on the live page a block without a form (or with a deleted one) shows visitors nothing
	_, empty := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "No form", "slug": "no-form", "status": "publish",
		"design": map[string]any{"id": "root", "type": "frame", "children": []any{
			map[string]any{"id": "pb1", "type": "pluginBlock", "props": map[string]any{"plugin": "forms", "block": "form", "values": map[string]any{}}},
			map[string]any{"id": "pb2", "type": "pluginBlock", "props": map[string]any{"plugin": "forms", "block": "form", "values": map[string]any{"formId": "999999"}}}}}})
	if empty["id"] == nil {
		t.Fatalf("page: %v", empty)
	}
	if out := e.offBSend(offBReq{path: "/no-form"}).Body.String(); strings.Contains(out, "Choose a form") || strings.Contains(out, "Form not found") {
		t.Fatalf("designer hint shown to visitors:\n%s", out)
	}
	// the standalone page, inside the site's header and footer
	out = e.offBSend(offBReq{path: "/forms/" + sid}).Body.String()
	offBContains(t, "standalone page", out, "<title>Contact", `action="/forms/`+sid+`/submit"`, "assets/form.js", ".pb-forms-form .rea-form{", "rea-page")

	// a Site Code listener for the event
	publishCode(t, admin, map[string]string{"main.ts": `
site.on('forms.submitted', (d) => { cache.set('got', d) })
site.on('form.submitted', (d) => { cache.set('legacy', d) })
site.route('GET /got', { access: ['*'] }, () => cache.get('got') ?? null)
site.route('GET /got-legacy', { access: ['*'] }, () => cache.get('legacy') ?? null)`})
	offBPkgSettings(t, admin, "forms", map[string]any{"notifyEmail": "owner@x.io", "siteTitle": "My Site"})

	post := func(ip, ctype, body string, hdr map[string]string) (int, map[string]any, string) {
		t.Helper()
		h := map[string]string{"Content-Type": ctype}
		for k, v := range hdr {
			h[k] = v
		}
		rec := e.offBSend(offBReq{method: "POST", path: "/forms/" + sid + "/submit", ip: ip, body: strings.NewReader(body), hdr: h})
		m := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		loc := rec.Header().Get("Location")
		return rec.Code, m, loc + rec.Body.String()
	}
	jsonH := map[string]string{"Accept": "application/json"}
	form := url.Values{"name": {" Tom "}, "email": {"tom@example.com"}, "topic": {"Sales"}, "agree": {"on"}, "x": {"dropped"}, "msg": {"=cmd()"}}
	code, res, _ := post("10.0.0.1", "application/x-www-form-urlencoded", form.Encode(), jsonH)
	if code != 200 || res["ok"] != true || res["message"] != "ขอบคุณ! เราได้รับข้อความของคุณแล้ว" || res["redirect"] != nil {
		t.Fatalf("submit: %d %v", code, res)
	}
	subs := offBRows(t, e, "p_forms_submissions", "form_id = ?", id)
	if len(subs) != 1 {
		t.Fatalf("stored: %v", subs)
	}
	var data map[string]string
	_ = json.Unmarshal([]byte(subs[0]["data"].(string)), &data)
	if data["name"] != "Tom" || data["agree"] != "yes" || data["topic"] != "Sales" || data["msg"] != "=cmd()" || data["x"] != "" {
		t.Fatalf("data: %v", data)
	}
	if _, ok := data["x"]; ok {
		t.Fatal("unknown field kept")
	}
	e.code.WaitEvents()
	_, got := getJSON(t, e.c, "GET", "/got", "", nil)
	if got["formId"] != float64(id) || got["formName"] != "Contact" || got["submissionId"] == nil || got["submittedAt"] == nil ||
		got["data"].(map[string]any)["email"] != "tom@example.com" {
		t.Fatalf("event: %v", got)
	}
	// the former core event name still reaches listeners (code.eventAliases)
	if _, lg := getJSON(t, e.c, "GET", "/got-legacy", "", nil); lg["submissionId"] != got["submissionId"] || lg["formName"] != "Contact" {
		t.Fatalf("form.submitted: %v", lg)
	}
	mails := e.offBMailbox()
	if len(mails) != 1 || mails[0].To[0] != "owner@x.io" || mails[0].Subject != "[My Site] Contact: ข้อความใหม่" || mails[0].ReplyTo != "tom@example.com" ||
		!strings.Contains(mails[0].HTML, "มีข้อความใหม่จากฟอร์ม “Contact”") || !strings.Contains(mails[0].HTML, "Name &lt;b&gt;") ||
		!strings.Contains(mails[0].HTML, ">Tom</td>") {
		t.Fatalf("mail: %+v", mails)
	}

	// validation (the Go plugin's Clean cases), as JSON answers
	for i, in := range []url.Values{
		{"email": {"tom@example.com"}},
		{"name": {"T"}, "email": {"not-an-email"}},
		{"name": {"T"}, "email": {"Tom <tom@example.com>"}},
		{"name": {"T"}, "email": {"t@x.io"}, "topic": {"Hacking"}},
		{"name": {strings.Repeat("a", 5001)}, "email": {"t@x.io"}},
	} {
		if code, res, _ := post(fmt.Sprintf("10.0.1.%d", i), "application/x-www-form-urlencoded", in.Encode(), jsonH); code != 400 || res["ok"] != false || res["message"] == "" {
			t.Errorf("case %d accepted: %d %v", i, code, res)
		}
	}
	if _, res, _ := post("10.0.2.1", "application/x-www-form-urlencoded", "email=t%40x.io", jsonH); res["message"] != `กรุณากรอก "Name <b>"` {
		t.Fatalf("required message: %v", res)
	}
	if code, res, _ := post("10.0.2.2", "application/x-www-form-urlencoded", "name=%zz", jsonH); code != 400 || res["message"] != "ข้อมูลที่ส่งมาไม่ถูกต้อง" {
		t.Fatalf("bad body: %d %v", code, res)
	}
	// multipart (FormData) and JSON bodies
	var mp bytes.Buffer
	mw := multipart.NewWriter(&mp)
	_ = mw.WriteField("name", "Multi")
	_ = mw.WriteField("email", "m@example.com")
	_ = mw.Close()
	if code, res, _ := post("10.0.3.1", mw.FormDataContentType(), mp.String(), jsonH); code != 200 || res["ok"] != true {
		t.Fatalf("multipart: %d %v", code, res)
	}
	if code, res, _ := post("10.0.3.2", "application/json", `{"name":"Jay","email":"j@example.com","agree":true}`, nil); code != 200 || res["ok"] != true {
		t.Fatalf("json: %d %v", code, res)
	}
	// honeypot: answered like a success, not stored
	before := len(offBRows(t, e, "p_forms_submissions", ""))
	if code, res, _ := post("10.0.4.1", "application/x-www-form-urlencoded", "_hp=spam&name=bot", jsonH); code != 200 || res["ok"] != true {
		t.Fatalf("honeypot: %d %v", code, res)
	}
	if n := len(offBRows(t, e, "p_forms_submissions", "")); n != before {
		t.Fatalf("honeypot stored: %d → %d", before, n)
	}
	// classic (no-JS) posts: back to the page with ?sent=<id>, failures as a page with a back link
	ok := url.Values{"name": {"C"}, "email": {"c@example.com"}}.Encode()
	code, _, loc := post("10.0.5.1", "application/x-www-form-urlencoded", ok, map[string]string{"Referer": "http://example.com/contact-us?x=1&sent=9", "Origin": "http://example.com"})
	if code != 303 || !strings.HasPrefix(loc, "/contact-us?sent="+sid+"&x=1#rea-form-"+sid) {
		t.Fatalf("classic: %d %s", code, loc)
	}
	if code, _, loc := post("10.0.5.2", "application/x-www-form-urlencoded", ok, map[string]string{"Referer": "https://evil.example/x", "Origin": "http://example.com"}); code != 303 ||
		!strings.HasPrefix(loc, "/?sent="+sid) {
		t.Fatalf("foreign referrer: %d %s", code, loc)
	}
	code, _, body := post("10.0.5.3", "application/x-www-form-urlencoded", "email=c%40example.com", map[string]string{"Referer": "http://example.com/contact-us"})
	if code != 400 || !strings.Contains(body, "ส่งฟอร์มไม่สำเร็จ") || !strings.Contains(body, `href="/contact-us"`) || !strings.Contains(body, "กรุณากรอก &#34;Name &lt;b&gt;&#34;") {
		t.Fatalf("classic failure: %d %s", code, body)
	}
	if code, res, _ := getJSONFrom(e, "10.0.5.4", "/forms/999999/submit"); code != 404 || res["message"] != "ไม่พบฟอร์มนี้" {
		t.Fatalf("unknown form: %d %v", code, res)
	}
	// redirect after submitting (this site only)
	fm["redirectUrl"] = "/thank-you"
	if _, msg := save(fm); msg != "" {
		t.Fatal(msg)
	}
	if code, res, _ := post("10.0.6.1", "application/x-www-form-urlencoded", ok, jsonH); code != 200 || res["redirect"] != "/thank-you" {
		t.Fatalf("redirect (json): %d %v", code, res)
	}
	if code, _, loc := post("10.0.6.2", "application/x-www-form-urlencoded", ok, nil); code != 303 || !strings.HasPrefix(loc, "/thank-you") {
		t.Fatalf("redirect (classic): %d %s", code, loc)
	}
	fm["redirectUrl"] = ""
	save(fm)

	// 5 submissions per minute per IP; honeypot posts are not counted (the built-in limiter ran after it)
	for i := 0; i < 3; i++ {
		post("10.9.9.9", "application/x-www-form-urlencoded", "_hp=spam&name=bot", jsonH)
	}
	for i := 0; i < 5; i++ {
		if code, res, _ := post("10.9.9.9", "application/x-www-form-urlencoded", ok, jsonH); code != 200 {
			t.Fatalf("attempt %d: %d %v", i+1, code, res)
		}
	}
	if code, res, _ := post("10.9.9.9", "application/x-www-form-urlencoded", ok, jsonH); code != 429 || res["message"] != "ส่งบ่อยเกินไป กรุณารอสักครู่แล้วลองใหม่" {
		t.Fatalf("6th attempt: %d %v", code, res)
	}
	if code, _, body := post("10.9.9.9", "application/x-www-form-urlencoded", ok, nil); code != 429 || !strings.Contains(body, "ส่งบ่อยเกินไป") {
		t.Fatalf("6th attempt (classic): %d", code)
	}
	if code, _, _ := post("10.9.9.10", "application/x-www-form-urlencoded", ok, jsonH); code != 200 {
		t.Fatalf("other IP blocked: %d", code)
	}

	// the form's own webhook (any public http(s) URL; the built-in body)
	var mu sync.Mutex
	var hookHdr http.Header
	var hookBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		hookHdr = r.Header.Clone()
		_ = json.Unmarshal(b, &hookBody)
		mu.Unlock()
	}))
	defer srv.Close()
	e.code.WebhookClient = srv.Client()
	fm["webhookUrl"] = srv.URL + "/hook"
	if _, msg := save(fm); msg != "" {
		t.Fatal(msg)
	}
	if code, res, _ := post("10.8.8.8", "application/x-www-form-urlencoded", ok, jsonH); code != 200 {
		t.Fatalf("submit with webhook: %d %v", code, res)
	}
	e.code.WaitEvents()
	mu.Lock()
	if hookHdr.Get("X-Anasaki-Event") != "form.submitted" || hookBody["event"] != "form.submitted" || hookBody["submissionId"] == nil ||
		hookBody["form"].(map[string]any)["name"] != "Contact" || hookBody["data"].(map[string]any)["email"] != "c@example.com" || hookBody["submittedAt"] == nil {
		t.Fatalf("per-form webhook: %v %v", hookHdr, hookBody)
	}
	mu.Unlock()
	fm["webhookUrl"] = ""
	save(fm)
	// the admin email (Settings → General) when the plugin has no notification address
	offBPkgSettings(t, admin, "forms", map[string]any{"notifyEmail": "", "siteTitle": ""})
	admin.do("PUT", "/api/v1/settings", map[string]any{"admin_email": "boss@x.io", "site_title": "Site Name"})
	before = len(e.offBMailbox())
	post("10.8.8.9", "application/x-www-form-urlencoded", ok, jsonH)
	e.code.WaitEvents()
	if m := e.offBMailbox(); len(m) != before+1 || m[len(m)-1].To[0] != "boss@x.io" || m[len(m)-1].Subject != "[Site Name] Contact: ข้อความใหม่" {
		t.Fatalf("admin email fallback: %+v", m)
	}

	// inbox: counts, read / unread, delete, CSV
	total := len(offBRows(t, e, "p_forms_submissions", "form_id = ?", id))
	_, lst := call("forms:list", map[string]any{})
	var item map[string]any
	for _, x := range lst["data"].([]any) {
		if m := x.(map[string]any); m["id"] == float64(id) {
			item = m
		}
	}
	if item == nil || item["submissionCount"] != float64(total) || item["unreadCount"] != float64(total) {
		t.Fatalf("list counts (%d): %v", total, item)
	}
	_, sl := call("forms:submissions", map[string]any{"formId": id})
	list := sl["data"].([]any)
	if len(list) != total || list[len(list)-1].(map[string]any)["data"].(map[string]any)["name"] != "Tom" {
		t.Fatalf("submissions (newest first): %v", list)
	}
	firstID := list[len(list)-1].(map[string]any)["id"]
	if _, r := call("forms:markRead", map[string]any{"id": firstID, "read": true}); r["data"].(map[string]any)["read"] != true {
		t.Fatalf("mark read: %v", r)
	}
	_, lst = call("forms:list", map[string]any{})
	for _, x := range lst["data"].([]any) {
		if m := x.(map[string]any); m["id"] == float64(id) && m["unreadCount"] != float64(total-1) {
			t.Fatalf("unread after mark: %v", m)
		}
	}
	rec = adminDownload(t, e.toSample(), admin, "forms:forms", "form="+sid)
	csv := rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), `attachment; filename="form-`+sid+`.csv"`) ||
		!strings.HasPrefix(csv, "\ufeffid,createdAt,Name <b>,Email,Topic,Agree,Message\n") || strings.Count(csv, "\n") != total+1 ||
		!strings.Contains(csv, ",Tom,tom@example.com,Sales,yes,'=cmd()\n") {
		t.Fatalf("csv: %d %v\n%s", rec.Code, rec.Header(), csv)
	}
	if rec := adminDownload(t, e.toSample(), admin, "forms:forms", "form=999999"); rec.Code != 404 {
		t.Fatalf("csv of an unknown form: %d", rec.Code)
	}
	if _, r := call("forms:deleteSubmission", map[string]any{"id": firstID}); r["data"] == nil {
		t.Fatalf("delete submission: %v", r)
	}
	if n := len(offBRows(t, e, "p_forms_submissions", "form_id = ?", id)); n != total-1 {
		t.Fatalf("after delete: %d", n)
	}
	// admin page data for a selected form
	html, _ = adminPage(t, e.toSample(), admin, "forms:forms", "form="+sid)
	offBContains(t, "admin page with forms", html, `"selected":`+sid, `"name":"Contact"`, `"label":"Name \u003cb\u003e"`)
	// deleting a form deletes its submissions
	call("forms:delete", map[string]any{"id": id})
	if n := len(offBRows(t, e, "p_forms_submissions", "form_id = ?", id)); n != 0 {
		t.Fatalf("submissions left: %d", n)
	}
	if b := e.offBSend(offBReq{path: "/forms/" + sid + "/embed"}).Body.String(); !strings.Contains(b, "Form not found") {
		t.Fatalf("deleted form still renders: %s", b)
	}

	// legacy import (rows shaped like forms_forms / forms_submissions), idempotent, ids kept
	imp := func(table string, rows []map[string]any) float64 {
		out := offBExport(t, e, admin, "forms", "legacyImport", map[string]any{"kind": "rows", "table": table, "rows": rows, "last": true})
		return out["imported"].(float64)
	}
	fieldsJSON := `[{"key":"name","label":"Name","type":"text","required":true,"options":[],"placeholder":""},{"key":"email","label":"Email","type":"email","required":false,"options":[],"placeholder":""}]`
	for round := 0; round < 2; round++ {
		if n := imp("forms_forms", []map[string]any{{"id": 41, "name": "Old contact", "fields": fieldsJSON, "submit_label": "Go", "success_message": "Thx",
			"redirect_url": "", "webhook_url": "https://hooks.example/x", "created_at": "2025-01-02T03:04:05Z"}}); n != 1 {
			t.Fatalf("forms imported: %v", n)
		}
		if n := imp("forms_submissions", []map[string]any{
			{"id": 900, "form_id": 41, "data": `{"name":"Ann","email":"ann@example.com"}`, "is_read": 1, "created_at": "2025-01-03 10:00:00"},
			{"id": 901, "form_id": 41, "data": map[string]any{"name": "Ben"}, "is_read": false, "created_at": "2025-01-04T10:00:00Z"},
		}); n != 2 {
			t.Fatalf("submissions imported: %v", n)
		}
	}
	if n := imp("unknown", []map[string]any{{"id": 1}}); n != 0 {
		t.Fatalf("unknown table: %v", n)
	}
	if r := offBRows(t, e, "p_forms_forms", "id = 41"); len(r) != 1 || r[0]["name"] != "Old contact" || r[0]["webhook_url"] != "https://hooks.example/x" {
		t.Fatalf("legacy form: %v", r)
	}
	_, sl = call("forms:submissions", map[string]any{"formId": 41})
	if l := sl["data"].([]any); len(l) != 2 || l[0].(map[string]any)["id"] != float64(901) || l[1].(map[string]any)["read"] != true ||
		l[1].(map[string]any)["createdAt"] != "2025-01-03T10:00:00Z" || l[0].(map[string]any)["data"].(map[string]any)["name"] != "Ben" {
		t.Fatalf("legacy submissions: %v", l)
	}
	if b := e.offBSend(offBReq{path: "/forms/41/embed"}).Body.String(); !strings.Contains(b, `action="/forms/41/submit"`) || !strings.Contains(b, ">Go</button>") {
		t.Fatalf("legacy form markup: %s", b)
	}
	code, res, _ = func() (int, map[string]any, string) {
		rec := e.offBSend(offBReq{method: "POST", path: "/forms/41/submit", ip: "10.7.7.7", body: strings.NewReader("name=New"),
			hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"}})
		m := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m, ""
	}()
	if code != 200 || res["message"] != "Thx" {
		t.Fatalf("submit to a legacy form: %d %v", code, res)
	}
	if r := offBRows(t, e, "p_forms_submissions", "form_id = 41 AND id > 901"); len(r) != 1 {
		t.Fatalf("new submission after the legacy ids: %v", r)
	}
	offBQuiet(t, admin, "forms")
}

// getJSONFrom posts an empty JSON body from a client address.
func getJSONFrom(e *offBEnv, ip, path string) (int, map[string]any, string) {
	rec := e.offBSend(offBReq{method: "POST", path: path, ip: ip, body: strings.NewReader("{}"), hdr: map[string]string{"Content-Type": "application/json"}})
	m := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec.Code, m, rec.Body.String()
}
