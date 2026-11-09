package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/members: the ready-made pages, register → confirmation mail → verify → sign in (cookie) →
// My account → profile → sign out, the member menu block, and the admin page.
func TestOfficialMembers(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.2.2"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "members")

	var sid *http.Cookie
	send := func(method, path, body string) *httpRec {
		t.Helper()
		hdr := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
		if sid != nil {
			hdr["Cookie"] = sid.Name + "=" + sid.Value
		}
		r := e.offBSend(offBReq{method: method, path: path, ip: "203.0.113.20", body: strings.NewReader(body), hdr: hdr})
		for _, c := range r.Result().Cookies() {
			if strings.HasSuffix(c.Name, "sid") {
				cc := *c
				sid = &cc
			}
		}
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}

	// 1.4.1: a reset link that cannot be used says so on opening, with the way to a new one
	if r := send("GET", "/members/reset?token=x", ""); r.code != 400 || !strings.Contains(r.body, `href="/members/forgot"`) || strings.Contains(r.body, `data-members="reset"`) {
		t.Fatalf("unusable reset link: %d %.300s", r.code, r.body)
	}
	for _, p := range []string{"/members/login", "/members/register", "/members/forgot"} {
		r := send("GET", p, "")
		if r.code != 200 || !strings.Contains(r.body, `class="mb-form"`) || !strings.Contains(r.body, "/plugin-assets/members/assets/members.js") {
			t.Fatalf("%s: %d", p, r.code)
		}
	}
	if r := send("GET", "/members/account", ""); r.code != 303 || !strings.HasPrefix(r.hdr.Get("Location"), "/members/login?next=") {
		t.Fatalf("account without a session: %d %v", r.code, r.hdr)
	}
	// a cross-site HTML form cannot post
	if r := e.offBSend(offBReq{method: "POST", path: "/members/register", body: strings.NewReader("email=a@x.io&password=password1"),
		hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}}); r.Code != 415 {
		t.Fatalf("form post: %d", r.Code)
	}

	if r := send("POST", "/members/register", `{"email":"Fah@X.io","password":"password1","name":"Fah"}`); r.code != 200 || !strings.Contains(r.body, `"ok":true`) {
		t.Fatalf("register: %d %s", r.code, r.body)
	}
	// 1.4.0: the confirmation mail goes through the job queue — the answer did not wait for it
	if len(e.offBMailbox()) != 0 {
		t.Fatalf("the mail was sent inside the request: %v", e.offBMailbox())
	}
	if r := send("POST", "/members/login", `{"email":"fah@x.io","password":"password1"}`); r.code == 200 || !strings.Contains(r.body, `"unverified":true`) || !strings.Contains(r.body, "fah@x.io") {
		t.Fatalf("signed in before confirming the email (or no hint): %d %s", r.code, r.body)
	}
	if r := send("POST", "/members/resend", `{"email":"nobody@x.io"}`); r.code != 200 {
		t.Fatalf("resend for an unknown address: %d", r.code)
	}
	e.code.ProcessJobs(context.Background(), 20)
	if n := len(e.offBMailbox()); n != 1 {
		t.Fatalf("confirmation mails after the queue ran: %d (one, the resend waits a minute)", n)
	}
	mails := fmt.Sprint(e.offBMailbox())
	m := regexp.MustCompile(`/members/verify\?token=([0-9a-f]{64})`).FindStringSubmatch(mails)
	if m == nil {
		t.Fatalf("no confirmation link: %s", mails)
	}
	if r := send("GET", "/members/verify?token="+m[1], ""); r.code != 200 || !strings.Contains(r.body, "/members/login?verified=1") {
		t.Fatalf("verify: %d", r.code)
	}
	// 1.4.1: the confirmation link signs the member in
	if r := send("GET", "/members/me", ""); !strings.Contains(r.body, "fah@x.io") {
		t.Fatalf("not signed in after confirming: %s", r.body)
	}
	if r := send("POST", "/members/logout", `{}`); r.code != 200 {
		t.Fatalf("logout after confirming: %d", r.code)
	}
	if r := send("POST", "/members/login", `{"email":"fah@x.io","password":"wrong-pass"}`); r.code != 401 {
		t.Fatalf("wrong password: %d", r.code)
	}
	if r := send("POST", "/members/login", `{"email":"fah@x.io","password":"password1"}`); r.code != 200 || sid == nil {
		t.Fatalf("login: %d %s", r.code, r.body)
	}
	r := send("GET", "/members/account", "")
	offBContains(t, "account", r.body, "fah@x.io", `value="Fah"`, ">member<", ">verified<", `data-members="logout"`)
	if r := send("GET", "/members/login", ""); r.code != 303 {
		t.Fatalf("login page while signed in: %d", r.code)
	}
	if r := send("POST", "/members/profile", `{"name":"Fah K."}`); r.code != 200 {
		t.Fatalf("profile: %d %s", r.code, r.body)
	}
	if r := send("GET", "/members/me", ""); !strings.Contains(r.body, `"name":"Fah K."`) || !strings.Contains(r.body, `"verified"`) {
		t.Fatalf("me: %s", r.body)
	}
	// 1.3.0: change the password from the account page (the current one first); other sessions are signed out
	if r := send("GET", "/members/account?next=/pos", ""); !strings.Contains(r.body, `data-members="password"`) || !strings.Contains(r.body, `href="/pos"`) {
		t.Fatalf("account page: password form / way back: %.400s", r.body)
	}
	if r := send("GET", "/members/account?next=//evil.example", ""); strings.Contains(r.body, "evil.example") {
		t.Fatal("account page links to another site")
	}
	other := e.offBSend(offBReq{method: "POST", path: "/members/login", ip: "203.0.113.21", body: strings.NewReader(`{"email":"fah@x.io","password":"password1"}`),
		hdr: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}})
	var otherSid string
	for _, c := range other.Result().Cookies() {
		if strings.HasSuffix(c.Name, "sid") {
			otherSid = c.Name + "=" + c.Value
		}
	}
	if r := send("POST", "/members/password", `{"current":"wrong-pass","password":"newpass123","password2":"newpass123"}`); r.code == 200 {
		t.Fatal("password changed with a wrong current password")
	}
	if r := send("POST", "/members/password", `{"current":"password1","password":"newpass123","password2":"other12345"}`); r.code == 200 {
		t.Fatal("password changed with two different new passwords")
	}
	if r := send("POST", "/members/password", `{"current":"password1","password":"short","password2":"short"}`); r.code == 200 {
		t.Fatal("a short password was accepted")
	}
	if r := send("POST", "/members/password", `{"current":"password1","password":"newpass123","password2":"newpass123"}`); r.code != 200 {
		t.Fatalf("change password: %d %s", r.code, r.body)
	}
	if r := send("GET", "/members/me", ""); !strings.Contains(r.body, `"name":"Fah K."`) {
		t.Fatalf("this session was signed out: %s", r.body)
	}
	if r := e.offBSend(offBReq{path: "/members/me", hdr: map[string]string{"Cookie": otherSid}}); !strings.Contains(r.Body.String(), `"visitor":null`) {
		t.Fatalf("the other session is still signed in: %s", r.Body.String())
	}
	if r := e.offBSend(offBReq{method: "POST", path: "/members/password", body: strings.NewReader(`{"current":"x","password":"newpass123"}`),
		hdr: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}}); r.Code != 401 {
		t.Fatalf("password change without a session: %d", r.Code)
	}
	if r := send("POST", "/members/password", `{"current":"newpass123","password":"password1","password2":"password1"}`); r.code != 200 {
		t.Fatalf("change back: %d %s", r.code, r.body)
	}
	if r := send("POST", "/members/logout", `{}`); r.code != 200 {
		t.Fatalf("logout: %d", r.code)
	}
	if r := send("GET", "/members/me", ""); !strings.Contains(r.body, `"visitor":null`) {
		t.Fatalf("still signed in: %s", r.body)
	}

	// the designer block and the admin page
	_, blocks := admin.doAny("GET", "/api/v1/blocks", nil)
	if !strings.Contains(fmt.Sprint(blocks), "type:menu") {
		t.Fatalf("blocks: %v", blocks)
	}
	html, call := adminPage(t, e.toSample(), admin, "members:list", "")
	offBContains(t, "admin", html, "fah@x.io", `data-member-status="disabled"`, "ak-table")
	if c, out := call("members:setStatus", map[string]any{"id": 1, "status": "disabled"}); c != 200 {
		t.Fatalf("disable: %d %v", c, out)
	}
	if r := send("POST", "/members/login", `{"email":"fah@x.io","password":"password1"}`); r.code == 200 {
		t.Fatal("a disabled member signed in")
	}
}

// members 1.1.0 + CMS 2.5.0: on a site that cannot send email yet (mail.ready() false) a new member is signed in
// right away instead of waiting for a confirmation mail that never comes; administrators see a notice.
func TestOfficialMembersWithoutEmail(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	mailOn := false // switched on later in the test
	offBMailReady = func() bool { return mailOn }
	defer func() { offBMailReady = nil }()
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "members")
	r := e.offBSend(offBReq{method: "POST", path: "/members/register", ip: "203.0.113.30",
		body: strings.NewReader(`{"email":"new@x.io","password":"password1","name":"New"}`),
		hdr:  map[string]string{"Content-Type": "application/json", "Accept": "application/json"}})
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"signedIn":true`) {
		t.Fatalf("register: %d %s", r.Code, r.Body.String())
	}
	var sid *http.Cookie
	for _, c := range r.Result().Cookies() {
		if strings.HasSuffix(c.Name, "sid") {
			sid = c
		}
	}
	if sid == nil {
		t.Fatal("no session cookie after registering")
	}
	if len(e.mails) != 0 {
		t.Fatalf("a confirmation mail was sent: %v", e.mails)
	}
	// 1.4.0: e-mail is switched on later — this member joined without a confirmation step and still gets in
	mailOn = true
	if r := e.offBSend(offBReq{method: "POST", path: "/members/login", ip: "203.0.113.32", body: strings.NewReader(`{"email":"new@x.io","password":"password1"}`),
		hdr: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}}); r.Code != 200 {
		t.Fatalf("an earlier member locked out once e-mail works: %d %s", r.Code, r.Body.String())
	}
	// a member who joins now must confirm; the administrator can confirm by hand
	if r := e.offBSend(offBReq{method: "POST", path: "/members/register", ip: "203.0.113.33", body: strings.NewReader(`{"email":"late@x.io","password":"password1","name":"Late"}`),
		hdr: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}}); r.Code != 200 || strings.Contains(r.Body.String(), `"signedIn":true`) {
		t.Fatalf("register with e-mail on: %d %s", r.Code, r.Body.String())
	}
	late := func() int {
		return e.offBSend(offBReq{method: "POST", path: "/members/login", ip: "203.0.113.34", body: strings.NewReader(`{"email":"late@x.io","password":"password1"}`),
			hdr: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}}).Code
	}
	if c := late(); c == 200 {
		t.Fatal("signed in without confirming")
	}
	html, call := adminPage(t, e.toSample(), admin, "members:list", "")
	id := regexp.MustCompile(`data-member-verify data-id="(\d+)"`).FindStringSubmatch(html)
	if id == nil {
		t.Fatalf("no confirm button for the unconfirmed member: %.300s", html)
	}
	if c, out := call("members:verify", map[string]any{"id": id[1]}); c != 200 {
		t.Fatalf("confirm by hand: %d %v", c, out)
	}
	if c := late(); c != 200 {
		t.Fatalf("login after the administrator confirmed: %d", c)
	}
	mailOn = false
	// 1.3.0: "forgot password" says the site cannot send mail, instead of pretending a link went out
	if r := e.offBSend(offBReq{method: "POST", path: "/members/forgot", ip: "203.0.113.31", body: strings.NewReader(`{"email":"new@x.io"}`),
		hdr: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}}); r.Code == 200 || !strings.Contains(r.Body.String(), "ส่งอีเมลไม่ได้") && !strings.Contains(r.Body.String(), "cannot send e-mail") {
		t.Fatalf("forgot without mail: %d %s", r.Code, r.Body.String())
	}
	_, n := admin.doAny("GET", "/api/v1/code/admin-notices", nil)
	if s := fmt.Sprint(n); !strings.Contains(s, "cannot send email yet") && !strings.Contains(s, "เว็บยังส่งอีเมลไม่ได้") {
		t.Fatalf("no admin notice: %v", n)
	}
}
