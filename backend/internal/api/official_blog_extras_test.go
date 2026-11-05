package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/blog-extras: reading time, table of contents, share buttons, related posts and comments added to
// posts through post.content; posting (form and JSON), moderation, replies, spam guards and the admin page.
func TestOfficialBlogExtras(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.2.2"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, user := setupSite(t, e.c)
	offBInstall(t, admin, "blog-extras")
	if !codeTable(t, e.pluginEnv, "p_blog_extras_comments") {
		t.Fatal("migration did not create the table")
	}
	offBPkgSettings(t, admin, "blog-extras", map[string]any{"notifyEmail": "editor@x.io"})

	code, cat := admin.do("POST", "/api/v1/terms", map[string]any{"name": "Coffee", "slug": "coffee"})
	if code != 201 {
		t.Fatalf("category: %d %v", code, cat)
	}
	post := func(title, slug, content string) map[string]any {
		t.Helper()
		code, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": title, "slug": slug, "status": "publish",
			"content": content, "categoryIds": []any{cat["id"]}})
		if code != 201 {
			t.Fatalf("post %s: %d %v", slug, code, p)
		}
		return p
	}
	body := "<h2>Beans</h2><p>" + strings.Repeat("word ", 600) + "</p><h3>Roast</h3><p>x</p><h2>Brew &amp; serve</h2><p>y</p>"
	p1 := post("Coffee guide", "coffee-guide", body)
	post("Latte art", "latte-art", "<p>milk</p>")
	pid := int(p1["id"].(float64))
	more := func() string {
		t.Helper()
		rec := e.offBSend(offBReq{path: fmt.Sprintf("/blog-extras/post/%d/more", pid)})
		if rec.Code != 200 {
			t.Fatalf("more: %d", rec.Code)
		}
		return rec.Body.String()
	}
	page := func() string {
		t.Helper()
		more() // the part below the post is cached by its route (post.content reads only the cache)
		rec := e.offBSend(offBReq{path: "/blog/coffee-guide"})
		if rec.Code != 200 {
			t.Fatalf("post page: %d", rec.Code)
		}
		return rec.Body.String()
	}
	// first view: the part below the post comes later from its route (a placeholder the script fills)
	if first := e.offBSend(offBReq{path: "/blog/coffee-guide"}).Body.String(); !strings.Contains(first, fmt.Sprintf(`data-bx-more="/blog-extras/post/%d/more"`, pid)) {
		t.Fatalf("no placeholder: %s", logsOf(t, admin, "blog-extras"))
	}
	html := page()
	offBContains(t, "extras", html, "⏱", `<details class="bx-toc" open>`, `<h2 id="beans">Beans</h2>`, `href="#brew-serve"`,
		"facebook.com/sharer", "social-plugins.line.me", `class="bx-related"`, "Latte art", `id="bx-comment-form"`, `name="post" value="`+fmt.Sprint(pid)+`"`)
	if strings.Contains(html, ">Coffee guide</a></li>") {
		t.Fatal("a post is related to itself")
	}

	send := func(ip, body, ct string) *httpRec {
		hdr := map[string]string{"Content-Type": ct}
		if strings.Contains(ct, "json") {
			hdr["Accept"] = "application/json"
		}
		r := e.offBSend(offBReq{method: "POST", path: "/blog-extras/comment", ip: ip, body: strings.NewReader(body), hdr: hdr})
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	form := url.Values{"post": {fmt.Sprint(pid)}, "name": {"Nok"}, "email": {"nok@x.io"}, "body": {"Great guide <b>!</b>"}, "back": {"//evil.example"}}
	if r := send("198.51.100.1", form.Encode(), "application/x-www-form-urlencoded"); r.code != 201 || !strings.Contains(r.body, `href="/blog/coffee-guide#comments"`) && !strings.Contains(r.body, "#comments") {
		t.Fatalf("form post: %d %s", r.code, r.body)
	}
	if strings.Contains(page(), "Great guide") {
		t.Fatal("a held comment is shown before approval")
	}
	// validation, honeypot, a draft / unknown post
	if r := send("198.51.100.2", fmt.Sprintf(`{"post":%d,"name":"","body":"x"}`, pid), "application/json"); r.code != 422 || !strings.Contains(r.body, `"errors"`) {
		t.Fatalf("invalid: %d %s", r.code, r.body)
	}
	if r := send("198.51.100.3", fmt.Sprintf(`{"post":%d,"name":"Bot","body":"buy now","website":"http://spam"}`, pid), "application/json"); r.code != 200 {
		t.Fatalf("honeypot: %d", r.code)
	}
	if r := send("198.51.100.4", `{"post":999999,"name":"A","body":"hello"}`, "application/json"); r.code != 404 {
		t.Fatalf("unknown post: %d", r.code)
	}
	// rate limit: 3 a minute from one address
	for i := 0; i < 3; i++ {
		send("198.51.100.9", fmt.Sprintf(`{"post":%d,"name":"R","body":"comment %d"}`, pid, i), "application/json")
	}
	if r := send("198.51.100.9", fmt.Sprintf(`{"post":%d,"name":"R","body":"again"}`, pid), "application/json"); r.code != 429 {
		t.Fatalf("rate limit: %d", r.code)
	}

	// moderation page: approve Nok's comment, then a reply shows under it
	html2, call := adminPage(t, e.toSample(), admin, "blog-extras:comments", "")
	offBContains(t, "moderation", html2, "Great guide &lt;b&gt;!&lt;/b&gt;", "nok@x.io", `data-bx-action="approved"`)
	if c, _ := user("Au", "author").do("POST", "/api/v1/code/admin-pages/blog-extras:comments/ticket", map[string]any{}); c == 200 {
		t.Fatal("author may moderate")
	}
	if c, out := call("blog-extras:moderate", map[string]any{"id": 1, "action": "approved"}); c != 200 || fmt.Sprint(out["data"]) != "map[id:1 status:approved]" {
		t.Fatalf("approve: %d %v", c, out)
	}
	offBPkgSettings(t, admin, "blog-extras", map[string]any{"moderation": "auto", "notifyEmail": "editor@x.io"})
	if r := send("198.51.100.5", fmt.Sprintf(`{"post":%d,"parent":1,"name":"Owner","body":"Thank you!"}`, pid), "application/json"); r.code != 201 || !strings.Contains(r.body, `"posted":true`) {
		t.Fatalf("auto reply: %d %s", r.code, r.body)
	}
	html = page()
	offBContains(t, "comments", html, "Great guide &lt;b&gt;!&lt;/b&gt;", `<div class="bx-replies">`, "Thank you!", `id="comment-1"`)
	if strings.Contains(html, "nok@x.io") {
		t.Fatal("an email address reached the page")
	}
	// spam / delete
	call("blog-extras:moderate", map[string]any{"id": 1, "action": "spam"})
	if strings.Contains(page(), "Great guide") {
		t.Fatal("spam is shown")
	}
	if c, out := call("blog-extras:moderate", map[string]any{"id": 1, "action": "delete"}); c != 200 || out["data"] == nil {
		t.Fatalf("delete: %d %v", c, out)
	}
	e.code.ProcessJobs(context.Background(), 20)
	if !strings.Contains(fmt.Sprint(e.offBMailbox()), "editor@x.io") {
		t.Fatalf("no notice mail: %v", e.offBMailbox())
	}

	// settings switch parts off
	offBPkgSettings(t, admin, "blog-extras", map[string]any{"comments": false, "toc": false, "share": false, "related": 0, "readingTime": false})
	html = page()
	for _, gone := range []string{`<details class="bx-toc"`, `<div class="bx-share"`, `<section class="bx-related"`, "bx-comment-form", "⏱"} {
		if strings.Contains(html, gone) {
			t.Errorf("%s still shown when switched off", gone)
		}
	}
	if r := send("198.51.100.6", fmt.Sprintf(`{"post":%d,"name":"A","body":"hello"}`, pid), "application/json"); r.code != 403 {
		t.Fatalf("closed comments: %d", r.code)
	}
}
