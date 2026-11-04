package api

import (
	"strings"
	"testing"
)

// CMS 2.2.0: Site Code static files (public/ → /site-assets/), site.asset in
// Site Code, and the theme's breakpoints on the public page.

func TestSiteCodePublicFiles(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	if code, b := admin.do("PUT", "/api/v1/code/files/public/page.html", map[string]any{"content": "<p>x</p>", "baseUpdatedAt": ""}); code != 400 {
		t.Fatalf("public/*.html accepted: %d %v", code, b)
	}
	if code, b := admin.do("PUT", "/api/v1/code/files/style.css", map[string]any{"content": "a{}", "baseUpdatedAt": ""}); code != 400 {
		t.Fatalf(".css outside public/ accepted: %d %v", code, b)
	}
	publishCode(t, admin, map[string]string{
		"public/qr.js":    "window.QR = 1;",
		"public/base.css": ".x{color:red}",
		"main.ts": `
site.route('GET /asset-url', { access: ['*'] }, () => {
  const a = site.asset('public/base.css');
  return { url: a.url, text: a.text(), size: a.size, type: a.type };
})
site.route('GET /asset-bad', { access: ['*'] }, () => site.asset('main.ts'))
`})
	code, body := c.public("GET", "/asset-url")
	if code != 200 || !strings.Contains(body, `"url":"/site-assets/base.css?v=`) || !strings.Contains(body, `"text":".x{color:red}"`) || !strings.Contains(body, "text/css") {
		_, logs := admin.doAny("GET", "/api/v1/code/logs", nil)
		t.Fatalf("site.asset: %d %s %v", code, body, logs)
	}
	if code, _ := c.public("GET", "/asset-bad"); code != 500 {
		t.Fatalf("site.asset outside public/: %d", code)
	}
	rec := c.send("GET", "/site-assets/qr.js", nil, nil)
	if rec.Code != 200 || rec.Body.String() != "window.QR = 1;" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") ||
		!strings.Contains(rec.Header().Get("Cache-Control"), "must-revalidate") || rec.Header().Get("ETag") == "" {
		t.Fatalf("public file: %d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
	if again := c.send("GET", "/site-assets/qr.js", nil, map[string]string{"If-None-Match": rec.Header().Get("ETag")}); again.Code != 304 {
		t.Fatalf("etag: %d", again.Code)
	}
	v := body[strings.Index(body, "?v=")+3:]
	v = v[:strings.IndexAny(v, `"&`)]
	if r := c.send("GET", "/site-assets/base.css?v="+v, nil, nil); !strings.Contains(r.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("versioned url not cached: %v", r.Header())
	}
	for _, p := range []string{"/site-assets/nope.js", "/site-assets/main.ts", "/site-assets/x/../../main.ts"} {
		// a 404 — or the router's redirect of a ".." path to the cleaned page path, never the source
		if r := c.send("GET", p, nil, nil); (r.Code != 404 && r.Code != 307 && r.Code != 301) || strings.Contains(r.Body.String(), "site.route") {
			t.Errorf("%s: %d %q", p, r.Code, r.Body.String())
		}
	}
}

func TestThemeBreakpointsOnPage(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	design := map[string]any{"id": "root", "type": "frame", "width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"},
		"children": []any{map[string]any{"id": "row", "type": "frame", "width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"},
			"layout": map[string]any{"mode": "horizontal"}, "responsive": map[string]any{"mobile": map[string]any{"layout": map[string]any{"mode": "vertical"}}}}}}
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "BP", "slug": "bp", "status": "publish", "design": design}); code != 201 {
		t.Fatalf("page: %d %v", code, b)
	}
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"theme": `{"breakpoints":{"mobile":767}}`}); code != 200 {
		t.Fatalf("theme: %d %v", code, b)
	}
	code, html := c.public("GET", "/bp")
	if code != 200 || !strings.Contains(html, "@media (max-width:767px){\n.n-row") || !strings.Contains(html, "@media (max-width:767px){.rea-burger") {
		t.Fatalf("page at 767: %d %s", code, html[:min(len(html), 400)])
	}
	admin.do("PUT", "/api/v1/settings", map[string]any{"theme": `{"breakpoints":{"mobile":5}}`}) // out of range → default
	if _, html := c.public("GET", "/bp"); !strings.Contains(html, "@media (max-width:640px){\n.n-row") {
		t.Fatal("invalid breakpoint not ignored")
	}
}
