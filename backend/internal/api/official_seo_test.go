package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The official "seo" JS plugin (plugins-official/seo) — the port of the built-in Go plugin: page.meta
// (title template, defaults, per-page overrides, canonical, Open Graph, Twitter), sitemap.urls (noindex
// pages left out; core serves /sitemap.xml and /robots.txt), the admin page and legacyImport.
func TestOfficialSEO(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	admin, user := setupSite(t, c)
	mk := func(typ, title, slug, status string) float64 {
		code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": typ, "title": title, "slug": slug, "status": status, "content": "<p>x</p>"})
		if code != 201 {
			t.Fatalf("%s: %d %v", slug, code, b)
		}
		return b["id"].(float64)
	}
	about := mk("page", "About", "about", "publish")
	mk("page", "Secret draft", "secret-draft", "draft")
	hello := mk("post", "Hello", "hello", "publish")
	offAInstall(t, admin, "seo")
	offASiteCode(t, admin, `
import { applyTemplate } from 'seo';
site.route('POST /offa-tpl', { access: ['*'] }, ({ request }) => request.json().map((c) => applyTemplate(c[0], c[1], 'Site', 'Tag')));
`)

	page := func(path string) string {
		t.Helper()
		r := c.send("GET", path, nil, nil)
		if r.Code != 200 {
			t.Fatalf("GET %s: %d", path, r.Code)
		}
		return r.Body.String()
	}
	has := func(h string, want ...string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(h, w) {
				t.Fatalf("page lacks %s:\n%s", w, h)
			}
		}
	}
	lacks := func(h string, bad ...string) {
		t.Helper()
		for _, w := range bad {
			if strings.Contains(h, w) {
				t.Fatalf("page has %s:\n%s", w, h)
			}
		}
	}

	// defaults: the built-in template "%title% – %site%", canonical, og:*, twitter:card summary
	h := page("/about")
	has(h, "<title>About – S</title>", `<link rel="canonical" href="http://example.com/about">`, `<meta property="og:url" content="http://example.com/about">`,
		`<meta property="og:type" content="website">`, `<meta property="og:site_name" content="S">`, `<meta property="og:title" content="About">`,
		`<meta name="twitter:card" content="summary">`, `<meta name="twitter:title" content="About">`)
	lacks(h, "og:image", "twitter:site", `name="robots"`)
	has(page("/blog/hello"), `<meta property="og:type" content="article">`, "<title>Hello – S</title>")

	// settings (same keys as the built-in plugin), saved on Plugins → SEO → Settings
	offBPkgSettings(t, admin, "seo", map[string]any{"titleTemplate": "%site% | %title%", "defaultDescription": "Default <desc>",
		"defaultImage": "/uploads/share.png", "twitterHandle": "brand"})
	h = page("/about")
	has(h, "<title>S | About</title>", `<meta name="description" content="Default &lt;desc&gt;">`,
		`<meta property="og:description" content="Default &lt;desc&gt;">`, `<meta property="og:image" content="http://example.com/uploads/share.png">`,
		`<meta name="twitter:card" content="summary_large_image">`, `<meta name="twitter:site" content="@brand">`,
		`<meta name="twitter:image" content="http://example.com/uploads/share.png">`)

	// applyTemplate: the built-in plugin's TestApplyTemplate vectors (through the plugin's library)
	vec := [][3]string{{"%title% – %site%", "About", "About – Site"}, {"%site% | %title%", "About", "Site | About"}, {"%title% – %site%", "", "Site"},
		{"%title% – %site%", "Site", "Site"}, {"", "About", "About – Site"}, {"%title% · %tagline%", "About", "About · Tag"},
		{"%title% %site%", "%site%", "%site% Site"}, {"| %title% |", "About", "About"}}
	var in [][2]string
	for _, v := range vec {
		in = append(in, [2]string{v[0], v[1]})
	}
	b, _ := json.Marshal(in)
	r := c.send("POST", "/offa-tpl", strings.NewReader(string(b)), map[string]string{"Content-Type": "application/json", "Accept": "application/json"})
	var got []string
	if err := json.Unmarshal(r.Body.Bytes(), &got); err != nil || len(got) != len(vec) {
		t.Fatalf("applyTemplate: %d %s", r.Code, r.Body.String())
	}
	for i, v := range vec {
		if got[i] != v[2] {
			t.Errorf("applyTemplate(%q, %q) = %q, want %q", v[0], v[1], got[i], v[2])
		}
	}

	// admin page (administrator only; the owner's admin language is English)
	if code, _ := user("Ed", "editor").do("POST", "/api/v1/code/admin-pages/seo:seo/ticket", map[string]any{}); code != 404 {
		t.Fatalf("editor ticket: %d", code)
	}
	html, call := offAAdminPage(t, e, admin, "seo:seo", "")
	if !strings.Contains(html, `data-seo="list"`) || !strings.Contains(html, "Search result previews") || !strings.Contains(html, "S | About") ||
		!strings.Contains(html, "Hello") || strings.Contains(html, "Secret draft") || !strings.Contains(html, `href="/sitemap.xml"`) ||
		!strings.Contains(html, `data-seo-setting="titleTemplate" maxlength="2000" value="%site% | %title%"`) || !strings.Contains(html, "/ext-ui/kit.js") {
		t.Fatalf("seo admin page:\n%s", html)
	}
	html, _ = offAAdminPage(t, e, admin, "seo:seo", fmt.Sprintf("edit=%d", int(about)))
	if !strings.Contains(html, `data-seo="form"`) || !strings.Contains(html, `placeholder="About"`) || !strings.Contains(html, "noindex") {
		t.Fatalf("seo edit view:\n%s", html)
	}
	// validation (same rules as PUT /api/v1/seo/meta/{postId})
	if code, out := call("seo:saveMeta", map[string]any{"postId": about, "image": "javascript:alert(1)", "noindex": false}); code != 200 || offAErr(out) != "Invalid image link" {
		t.Fatalf("bad image: %d %v", code, out)
	}
	if code, out := call("seo:saveMeta", map[string]any{"postId": about, "title": strings.Repeat("ก", 86), "noindex": false}); code != 200 || offAErr(out) != "Text is too long" {
		t.Fatalf("long title (bytes): %d %v", code, out)
	}
	if code, out := call("seo:saveMeta", map[string]any{"postId": 999999, "noindex": false}); code != 200 || offAErr(out) == "" {
		t.Fatalf("missing post: %d %v", code, out)
	}
	if code, out := call("seo:saveMeta", map[string]any{"postId": about, "title": "  Custom  ", "description": "Own description", "image": "https://cdn.example.com/a.png", "noindex": true}); code != 200 ||
		out["data"].(map[string]any)["title"] != "Custom" {
		t.Fatalf("save: %d %v", code, out)
	}
	h = page("/about")
	has(h, "<title>S | Custom</title>", `<meta name="description" content="Own description">`, `<meta name="robots" content="noindex,nofollow">`,
		`<meta property="og:image" content="https://cdn.example.com/a.png">`, `<meta property="og:title" content="Custom">`)
	if got := offACall(t, admin, "seo", "metaOf", about).(map[string]any); got["noindex"] != true || got["description"] != "Own description" {
		t.Fatalf("metaOf: %v", got)
	}
	html, _ = offAAdminPage(t, e, admin, "seo:seo", "")
	if !strings.Contains(html, "S | Custom") || !strings.Contains(html, "noindex</span>") {
		t.Fatalf("list after save:\n%s", html)
	}
	// the site-wide defaults form saves the plugin's settings (site.saveSettings)
	if code, out := call("seo:saveDefaults", map[string]any{"titleTemplate": "%title% · %tagline%", "defaultDescription": "Default <desc>",
		"defaultImage": "javascript:x", "twitterHandle": "brand"}); code != 200 || offAErr(out) != "Invalid image link" {
		t.Fatalf("bad default image: %d %v", code, out)
	}
	if code, out := call("seo:saveDefaults", map[string]any{"titleTemplate": "%title% · %tagline%", "defaultDescription": "Default <desc>",
		"defaultImage": "/uploads/share.png", "twitterHandle": "brand"}); code != 200 || out["data"].(map[string]any)["titleTemplate"] != "%title% · %tagline%" {
		t.Fatalf("save defaults: %d %v", code, out)
	}
	if st := pkgOf(t, admin, "seo")["settings"].(map[string]any); st["titleTemplate"] != "%title% · %tagline%" {
		t.Fatalf("saved settings: %v", st)
	}
	// %tagline% (page.siteTagline)
	admin.do("PUT", "/api/v1/settings", map[string]any{"tagline": "Best <site>"})
	has(page("/about"), "<title>Custom · Best &lt;site&gt;</title>")
	html, _ = offAAdminPage(t, e, admin, "seo:seo", "")
	if !strings.Contains(html, "Custom · Best &lt;site&gt;") {
		t.Fatalf("admin preview with tagline:\n%s", html)
	}
	call("seo:saveDefaults", map[string]any{"titleTemplate": "%site% | %title%", "defaultDescription": "Default <desc>", "defaultImage": "/uploads/share.png", "twitterHandle": "brand"})

	// the post editor's SEO panel: plugin fields "seo.*" (authors on their own posts)
	_, cat := admin.do("GET", "/api/v1/post-types", nil)
	for _, typ := range []string{"post", "page"} {
		pf := cat["postFields"].(map[string]any)[typ].(map[string]any)
		if f, ok := pf["seo.title"].(map[string]any); !ok || f["plugin"] != "seo" || f["pluginName"] != "SEO" || pf["seo.noindex"] == nil || pf["seo.image"] == nil {
			t.Fatalf("%s fields: %v", typ, pf)
		}
	}
	author := user("Au", "author")
	code, own := author.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "Mine", "slug": "mine", "status": "draft",
		"meta": map[string]any{"seo.title": "Author SEO", "seo.description": "Written by the author"}})
	if code != 201 {
		t.Fatalf("author post: %d %v", code, own)
	}
	admin.do("PUT", fmt.Sprintf("/api/v1/posts/%d", int(own["id"].(float64))), map[string]any{"status": "publish"})
	has(page("/blog/mine"), "<title>S | Author SEO</title>", `<meta name="description" content="Written by the author">`)
	if got := offACall(t, admin, "seo", "metaOf", own["id"]).(map[string]any); got["title"] != "Author SEO" {
		t.Fatalf("metaOf (editor values): %v", got)
	}

	// Thai admin language → Thai screen
	admin.do("PUT", "/api/v1/auth/me", map[string]any{"locale": "th"})
	if html, _ := offAAdminPage(t, e, admin, "seo:seo", ""); !strings.Contains(html, "หน้าตาในผลการค้นหา") || !strings.Contains(html, "ค่าเริ่มต้นทั้งเว็บ") {
		t.Fatalf("thai admin page:\n%s", html)
	}

	// sitemap.xml / robots.txt (served by core because the plugin filters sitemap.urls)
	r = c.send("GET", "/sitemap.xml", nil, nil)
	sm := r.Body.String()
	if r.Code != 200 || !strings.HasPrefix(r.Header().Get("Content-Type"), "application/xml") || !strings.Contains(sm, "<loc>http://example.com/blog/hello</loc>") ||
		!strings.Contains(sm, "<loc>http://example.com/</loc>") || strings.Contains(sm, "/about") || strings.Contains(sm, "secret-draft") || !strings.Contains(sm, "<lastmod>") {
		t.Fatalf("sitemap: %d\n%s", r.Code, sm)
	}
	if code, body := c.public("GET", "/robots.txt"); code != 200 || body != "User-agent: *\nDisallow: /api/\n\nSitemap: http://example.com/sitemap.xml\n" {
		t.Fatalf("robots: %d %q", code, body)
	}
	// search engines discouraged (Settings → Reading)
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"search_visibility": "0"}); code != 200 {
		t.Fatalf("visibility: %d %v", code, b)
	}
	if _, body := c.public("GET", "/robots.txt"); body != "User-agent: *\nDisallow: /\n" {
		t.Fatalf("hidden robots: %q", body)
	}
	if sm := c.send("GET", "/sitemap.xml", nil, nil).Body.String(); strings.Contains(sm, "<url>") || !strings.Contains(sm, "<urlset") {
		t.Fatalf("hidden sitemap:\n%s", sm)
	}
	admin.do("PUT", "/api/v1/settings", map[string]any{"search_visibility": "1"})

	// legacy import: rows of the built-in seo_meta table, idempotent (upsert by post_id)
	payload := map[string]any{"kind": "rows", "table": "seo_meta", "last": true, "rows": []any{
		map[string]any{"post_id": hello, "title": "Old SEO title", "description": "Old description", "image": "", "noindex": 1, "updated_at": "2026-01-02 03:04:05"},
		map[string]any{"post_id": about, "title": "", "description": nil, "image": "", "noindex": false, "updated_at": "2026-01-02T03:04:05Z"},
	}}
	for i := 0; i < 2; i++ {
		if got := offACall(t, admin, "seo", "legacyImport", payload).(map[string]any); got["imported"] != float64(2) {
			t.Fatalf("legacyImport #%d: %v", i, got)
		}
	}
	if got := offACall(t, admin, "seo", "metaOf", hello).(map[string]any); got["title"] != "Old SEO title" || got["noindex"] != true {
		t.Fatalf("hello after import: %v", got)
	}
	if got := offACall(t, admin, "seo", "metaOf", about).(map[string]any); got["title"] != "" || got["noindex"] != false || got["description"] != "" {
		t.Fatalf("about after import: %v", got)
	}
	// rows of deleted posts are skipped
	if got := offACall(t, admin, "seo", "legacyImport", map[string]any{"kind": "rows", "table": "seo_meta", "rows": []any{
		map[string]any{"post_id": 999999, "title": "Gone"}}}).(map[string]any); got["imported"] != float64(0) {
		t.Fatalf("deleted post: %v", got)
	}
	if got := offACall(t, admin, "seo", "legacyImport", map[string]any{"kind": "rows", "table": "other", "rows": []any{}}).(map[string]any); got["imported"] != float64(0) {
		t.Fatalf("unknown table: %v", got)
	}
	has(page("/blog/hello"), "<title>S | Old SEO title</title>", `<meta name="description" content="Old description">`, `<meta name="robots" content="noindex,nofollow">`)
	h = page("/about")
	has(h, "<title>S | About</title>", `<meta name="description" content="Default &lt;desc&gt;">`)
	lacks(h, `name="robots"`)
	sm = c.send("GET", "/sitemap.xml", nil, nil).Body.String()
	if strings.Contains(sm, "/blog/hello") || !strings.Contains(sm, "<loc>http://example.com/about</loc>") {
		t.Fatalf("sitemap after import:\n%s", sm)
	}
}
