package api

import (
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

	for _, p := range []string{"/members/login", "/members/register", "/members/forgot", "/members/reset?token=x"} {
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
	if r := send("POST", "/members/login", `{"email":"fah@x.io","password":"password1"}`); r.code == 200 {
		t.Fatal("signed in before confirming the email")
	}
	mails := fmt.Sprint(e.offBMailbox())
	m := regexp.MustCompile(`/members/verify\?token=([0-9a-f]{64})`).FindStringSubmatch(mails)
	if m == nil {
		t.Fatalf("no confirmation link: %s", mails)
	}
	if r := send("GET", "/members/verify?token="+m[1], ""); r.code != 200 || !strings.Contains(r.body, "/members/login?verified=1") {
		t.Fatalf("verify: %d", r.code)
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
