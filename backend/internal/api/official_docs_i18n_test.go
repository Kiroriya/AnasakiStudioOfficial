package api

import (
	"strings"
	"testing"
)

// docs 1.2.0: articles in a second language at /docs/<lang>/… — translations share the original's URL slug
// (meta base), pages link to the other language (and say so in hreflang), each language has its own hub,
// search index, llms.txt and sitemap entries; the site's language stays at /docs.
func TestOfficialDocsTranslations(t *testing.T) {
	offDocsCore(t)
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	admin.do("PUT", "/api/v1/settings", map[string]any{"language": "th", "timezone": "Asia/Bangkok"})
	offBInstall(t, admin, "docs")
	_, call := adminPage(t, e.toSample(), admin, "docs:docs", "")
	if code, out := call("docs:saveSettings", map[string]any{"products": []any{
		map[string]any{"id": "studio", "name": map[string]any{"th": "สตูดิโอ", "en": "Studio"}, "icon": "🎨"}}}); code != 200 || offAErr(out) != "" {
		t.Fatalf("products: %d %v", code, out)
	}
	code, out := call("docs:import", map[string]any{"docs": []any{
		map[string]any{"slug": "install", "title": "ติดตั้ง", "excerpt": "ติดตั้งด้วย Docker", "product": "studio", "section": "เริ่มต้น", "order": 1, "content": `<h2>ขั้นตอน</h2><p>ไทย</p>`},
		map[string]any{"slug": "only-thai", "title": "เฉพาะภาษาไทย", "product": "studio", "section": "เริ่มต้น", "order": 2, "content": `<p>ไทย</p>`},
		map[string]any{"slug": "install-en", "lang": "en", "base": "install", "title": "Install", "excerpt": "Install with Docker", "product": "studio",
			"section": "Getting started", "order": 1, "content": `<h2>Steps</h2><p>English</p>`},
	}})
	if code != 200 || out["data"].(map[string]any)["created"] != float64(3) {
		t.Fatalf("import: %d %v", code, out)
	}
	page := func(path string, want int) string {
		t.Helper()
		rec := e.offBSend(offBReq{path: path})
		if rec.Code != want {
			t.Fatalf("%s: %d (want %d)\n%s", path, rec.Code, want, rec.Body.String())
		}
		return rec.Body.String()
	}

	// the site's language at /docs: Thai only, with a link to the English docs
	h := page("/docs", 200)
	offBContains(t, "thai hub", h, "ติดตั้ง", "เฉพาะภาษาไทย", `<nav class="dx-langs"`, `href="/docs/en" hreflang="en"`,
		`<link rel="alternate" href="http://example.com/docs/en" hreflang="en">`)
	if strings.Contains(h, ">Install<") {
		t.Fatal("English doc on the Thai hub")
	}
	// English: hub, product page and article under /docs/en, English strings
	h = page("/docs/en", 200)
	offBContains(t, "english hub", h, "Documentation", ">Install<", `href="/docs/en/studio/install"`, `href="/docs" hreflang="th"`, `"search":"/docs/en/search.json"`)
	if strings.Contains(h, "เฉพาะภาษาไทย") {
		t.Fatal("Thai doc on the English hub")
	}
	offBContains(t, "english product", page("/docs/en/studio", 200), "Getting started", ">Studio<")
	a := page("/docs/en/studio/install", 200)
	offBContains(t, "english article", a, "<h1>Install</h1>", "English", `href="/docs/studio/install" hreflang="th"`, `data-dx-copy-md="/docs/en/studio/install.md"`,
		`<link rel="alternate" href="http://example.com/docs/studio/install" hreflang="th">`, `<link rel="alternate" href="http://example.com/docs/en/studio/install" hreflang="en">`)
	// the Thai article links to its translation; a Thai-only article links to the English product page
	offBContains(t, "thai article", page("/docs/studio/install", 200), "<h1>ติดตั้ง</h1>", `href="/docs/en/studio/install" hreflang="en"`)
	offBContains(t, "thai only", page("/docs/studio/only-thai", 200), `href="/docs/en/studio" hreflang="en"`)
	// Markdown, search and llms.txt per language
	offBContains(t, "english md", page("/docs/en/studio/install.md", 200), "# Install", "/docs/en/studio/install")
	s := page("/docs/en/search.json", 200)
	if !strings.Contains(s, `"url":"/docs/en/studio/install"`) || strings.Contains(s, "only-thai") {
		t.Fatalf("english search: %s", s)
	}
	offBContains(t, "english llms", page("/docs/en/llms.txt", 200), "/docs/en/studio/install.md", "/docs/llms.txt")
	// unknown language / article; the post slug of a translation is not a URL
	page("/docs/fr/studio/install", 404)
	page("/docs/en/studio/only-thai", 404)
	page("/docs/studio/install-en", 404)
	// sitemap: both languages
	sm := page("/sitemap.xml", 200)
	for _, want := range []string{"/docs/en</loc>", "/docs/en/studio</loc>", "/docs/en/studio/install</loc>", "/docs/studio/install</loc>"} {
		if !strings.Contains(sm, want) {
			t.Errorf("sitemap lacks %s", want)
		}
	}

	// the same bundle on a site whose own language is English: the Thai originals say "th" (the bundle build
	// marks them), so English is at /docs and Thai at /docs/th
	code, out = call("docs:import", map[string]any{"docs": []any{
		map[string]any{"slug": "install", "lang": "th", "title": "ติดตั้ง", "product": "studio", "section": "เริ่มต้น", "order": 1, "content": `<p>ไทย</p>`},
		map[string]any{"slug": "only-thai", "lang": "th", "title": "เฉพาะภาษาไทย", "product": "studio", "section": "เริ่มต้น", "order": 2, "content": `<p>ไทย</p>`},
	}})
	if code != 200 || out["data"].(map[string]any)["updated"] != float64(2) {
		t.Fatalf("re-import: %d %v", code, out)
	}
	offBContains(t, "thai hub with lang th", page("/docs", 200), "เฉพาะภาษาไทย", `href="/docs/en" hreflang="en"`)
	admin.do("PUT", "/api/v1/settings", map[string]any{"language": "en"})
	h = page("/docs", 200)
	offBContains(t, "english site hub", h, ">Install<", `href="/docs/th" hreflang="th"`)
	if strings.Contains(h, "เฉพาะภาษาไทย") {
		t.Fatal("Thai doc on the English site's hub")
	}
	offBContains(t, "english site article", page("/docs/studio/install", 200), "<h1>Install</h1>", `href="/docs/th/studio/install" hreflang="th"`)
	offBContains(t, "thai on english site", page("/docs/th/studio/install", 200), "<h1>ติดตั้ง</h1>", `href="/docs/studio/install" hreflang="en"`)
	page("/docs/en/studio/install", 404) // the site's language has no prefix
}
