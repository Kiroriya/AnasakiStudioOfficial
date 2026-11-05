package api

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/newsletter: sign-up (form and JSON) with double opt-in, confirm and unsubscribe links, the
// admin page (import, CSV), and a campaign sent by the background job with the unsubscribe link in every mail.
func TestOfficialNewsletter(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.2.2"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "newsletter")

	post := func(ip, body, ct string) *httpRec {
		hdr := map[string]string{"Content-Type": ct}
		if strings.Contains(ct, "json") {
			hdr["Accept"] = "application/json"
		}
		r := e.offBSend(offBReq{method: "POST", path: "/newsletter/subscribe", ip: ip, body: strings.NewReader(body), hdr: hdr})
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	if r := post("203.0.113.30", url.Values{"email": {"Fon@X.io"}, "name": {"Fon"}}.Encode(), "application/x-www-form-urlencoded"); r.code != 200 {
		t.Fatalf("form sign-up: %d %s", r.code, r.body)
	}
	if r := post("203.0.113.31", `{"email":"not-an-email"}`, "application/json"); r.code != 400 {
		t.Fatalf("bad email: %d", r.code)
	}
	if r := post("203.0.113.32", `{"email":"bot@x.io","website":"http://spam"}`, "application/json"); r.code != 200 {
		t.Fatalf("honeypot: %d", r.code)
	}
	e.code.ProcessJobs(context.Background(), 20)
	mails := fmt.Sprint(e.offBMailbox())
	m := regexp.MustCompile(`/newsletter/confirm\?t=([0-9a-f]{48})`).FindStringSubmatch(mails)
	if m == nil || !strings.Contains(mails, "fon@x.io") || strings.Contains(mails, "bot@x.io") {
		t.Fatalf("confirmation mail: %s", mails)
	}
	token := m[1]
	if r := e.offBSend(offBReq{path: "/newsletter/confirm?t=" + token}); r.Code != 200 {
		t.Fatalf("confirm: %d", r.Code)
	}

	// admin: import two more (one invalid), list and CSV
	html, call := adminPage(t, e.toSample(), admin, "newsletter:newsletter", "")
	offBContains(t, "subscribers", html, "fon@x.io", `id="nl-subscribers"`)
	if c, out := call("newsletter:addSubscribers", map[string]any{"lines": "Pim@X.io,Pim\nwrong\nkai@x.io"}); c != 200 || fmt.Sprint(out["data"]) != "map[added:2 skipped:1]" {
		t.Fatalf("import: %d %v", c, out)
	}
	if d := adminDownload(t, e.toSample(), admin, "newsletter:newsletter", "status=active"); d.Code != 200 || !strings.Contains(d.Body.String(), "pim@x.io") {
		t.Fatalf("csv: %d %s", d.Code, d.Body.String())
	}

	// a campaign to the three active subscribers
	c, out := call("newsletter:saveCampaign", map[string]any{"subject": "November news", "body": "Hello!\n\nSee https://example.com/sale"})
	if c != 200 {
		t.Fatalf("save: %d %v", c, out)
	}
	id := out["data"].(map[string]any)["id"]
	before := len(e.offBMailbox())
	if c, out := call("newsletter:send", map[string]any{"id": id}); c != 200 || fmt.Sprint(out["data"].(map[string]any)["total"]) != "3" {
		t.Fatalf("send: %d %v", c, out)
	}
	if c, _ := call("newsletter:send", map[string]any{"id": id}); c == 200 {
		if _, o := call("newsletter:send", map[string]any{"id": id}); o["errors"] == nil {
			t.Fatal("sent twice")
		}
	}
	e.code.ProcessJobs(context.Background(), 20)
	sent := e.offBMailbox()[before:]
	if len(sent) != 3 {
		t.Fatalf("campaign mails: %d %v", len(sent), sent)
	}
	all := fmt.Sprint(sent)
	if !strings.Contains(all, "/newsletter/unsubscribe?t=") || !strings.Contains(all, `<a href="https://example.com/sale">`) {
		t.Fatalf("campaign mail: %s", all)
	}
	html, _ = adminPage(t, e.toSample(), admin, "newsletter:newsletter", "tab=camps")
	offBContains(t, "campaigns", html, "November news", "3 / 3")

	// unsubscribe: GET asks, POST does it
	if r := e.offBSend(offBReq{path: "/newsletter/unsubscribe?t=" + token}); r.Code != 200 || !strings.Contains(r.Body.String(), `method="post"`) {
		t.Fatalf("unsubscribe page: %d", r.Code)
	}
	if r := e.offBSend(offBReq{method: "POST", path: "/newsletter/unsubscribe?t=" + token}); r.Code != 200 {
		t.Fatalf("unsubscribe: %d", r.Code)
	}
	html, _ = adminPage(t, e.toSample(), admin, "newsletter:newsletter", "tab=subs&status=unsubscribed")
	offBContains(t, "unsubscribed", html, "fon@x.io")
	// the designer block
	_, blocks := admin.doAny("GET", "/api/v1/blocks", nil)
	if !strings.Contains(fmt.Sprint(blocks), "plugin:newsletter") {
		t.Fatalf("blocks: %v", blocks)
	}
}
