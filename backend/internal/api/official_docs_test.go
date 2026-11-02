package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/docs: the documentation hub — its post type (site.postType in a plugin, CMS 2.0.5), the
// bundle import (admin page action, upsert by slug), the public pages (hub, product landing, article with
// sidebar / breadcrumb / TOC / prev-next, 404), search.json, page.meta, sitemap.urls and the legacy redirects.

// offDocsBundle is a small bundle in the import format (two products, sections out of order, one draft).
func offDocsBundle() []any {
	return []any{
		map[string]any{"slug": "studio-install", "title": "ติดตั้งด้วย Docker", "excerpt": "เริ่มต้นใช้งานด้วย docker compose", "product": "studio",
			"section": "เริ่มต้น", "order": 1, "sectionOrder": 1,
			"content": `<p>ขั้นตอนการติดตั้ง</p><h2>สิ่งที่ต้องมี</h2><p>Docker</p><h3>ตัวแปร .env</h3><pre><code class="language-bash">docker compose up -d</code></pre>` +
				`<h2>สิ่งที่ต้องมี</h2><table><tr><th>ชื่อ</th></tr><tr><td>CODE_DB_PASSWORD</td></tr></table><a href="https://example.com/x">ext</a>`},
		map[string]any{"slug": "studio-setup", "title": "Setup wizard", "excerpt": "ตั้งค่าครั้งแรก", "product": "studio", "section": "เริ่มต้น", "order": 2,
			"content": `<h2>Run the wizard</h2><p>Open <code>/setup</code>.</p>`},
		map[string]any{"slug": "studio-designer", "title": "ออกแบบหน้าเว็บ", "excerpt": "Designer", "product": "studio", "section": "ออกแบบ", "order": 1, "sectionOrder": 2,
			"content": `<h2>Frames</h2><p>…</p>`},
		map[string]any{"slug": "studio-trouble", "title": "แก้ปัญหา", "excerpt": "", "product": "studio", "section": "อื่น ๆ", "order": 1, "sectionOrder": 9,
			"content": `<p>FAQ</p>`},
		map[string]any{"slug": "dev-cli", "title": "CLI anasaki-code", "excerpt": "Sync files", "product": "developers", "section": "Site Code", "order": 1,
			"content": `<h2>Install the CLI</h2><p>npm i -g</p>`},
		map[string]any{"slug": "studio-secret", "title": "Secret draft doc", "product": "studio", "section": "เริ่มต้น", "order": 3, "status": "draft",
			"content": `<p>hidden</p>`},
	}
}

// offDocsCore makes the test CMS a 2.0.5 release (the package's minCore; development builds report 1.0.0).
func offDocsCore(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.0.5"
	t.Cleanup(func() { plugin.CoreVersion = old })
}

func TestOfficialDocs(t *testing.T) {
	offDocsCore(t)
	e := offBNew(t)
	admin, user := setupSite(t, e.c)
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"language": "th", "timezone": "Asia/Bangkok"}); code != 200 {
		t.Fatalf("settings: %d %v", code, b)
	}
	files := offBPackage(t, "docs")
	var mf map[string]any
	if err := json.Unmarshal([]byte(files["manifest.json"]), &mf); err != nil {
		t.Fatal(err)
	}
	if mf["id"] != "docs" || mf["version"] != "1.1.0" || mf["author"] != "Anasaki Studio" || mf["runtime"] != "js" || mf["minCore"] != "2.0.5" {
		t.Fatalf("manifest: %v", mf)
	}
	if s := files["README.md"]; !strings.Contains(s, "## ภาษาไทย") || !strings.Contains(s, "## English") {
		t.Fatal("README must have Thai and English sections")
	}
	offBInstall(t, admin, "docs")

	// the post type: listed for the admin with the plugin, own fields, not routed by core
	_, pts := admin.do("GET", "/api/v1/post-types", nil)
	var docType map[string]any
	for _, x := range pts["postTypes"].([]any) {
		if m := x.(map[string]any); m["type"] == "docs_doc" {
			docType = m
		}
	}
	if docType == nil || docType["plugin"] != "docs" || docType["public"] != false || docType["slug"] != "docs" ||
		!strings.Contains(fmt.Sprint(docType["fields"]), "sectionOrder") || !strings.Contains(fmt.Sprint(docType["label"]), "เอกสาร") {
		t.Fatalf("post type: %v", docType)
	}

	// admin page (editors and administrators) + import
	html, call := adminPage(t, e.toSample(), admin, "docs:docs", "")
	offBContains(t, "admin page", html, `id="docs-data"`, `src="/plugin-assets/docs/assets/admin.js?v=1.1.0" defer`, `data-tab="import"`, `"canSettings":true`)
	if code, _ := user("Au", "author").do("POST", "/api/v1/code/admin-pages/docs:docs/ticket", map[string]any{}); code == 200 {
		t.Fatal("author may open the docs admin page")
	}
	imp := func(call func(string, any) (int, map[string]any), docs []any) map[string]any {
		t.Helper()
		code, out := call("docs:import", map[string]any{"docs": docs})
		if code != 200 || out["data"] == nil {
			t.Fatalf("import: %d %v", code, out)
		}
		return out["data"].(map[string]any)
	}
	// a fresh install carries nothing of any site: one generic group, other product ids are refused
	if rec := e.offBSend(offBReq{path: "/docs"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `href="/docs/general"`) ||
		strings.Contains(rec.Body.String(), "Plugin Store") || strings.Contains(rec.Body.String(), "สำหรับนักพัฒนา") {
		t.Fatalf("fresh hub: %d %s", rec.Code, rec.Body.String())
	}
	if res := imp(call, offDocsBundle()[:1]); res["created"] != float64(0) || !strings.Contains(fmt.Sprint(res["errors"]), `ไม่รู้จักผลิตภัณฑ์ "studio"`) {
		t.Fatalf("import before the site's products: %v", res)
	}
	// the site's own products (a bundle's "products"; the admin page saves them before importing)
	if code, out := call("docs:saveSettings", map[string]any{"products": []any{
		map[string]any{"id": "studio", "name": "Anasaki Studio", "icon": "🎨"},
		map[string]any{"id": "store", "name": "Plugin Store", "icon": "🛍"},
		map[string]any{"id": "developers", "name": map[string]any{"th": "สำหรับนักพัฒนา", "en": "Developers"}, "icon": "⌨️"},
	}}); code != 200 || offAErr(out) != "" {
		t.Fatalf("site products: %d %v", code, out)
	}
	bad := []any{
		map[string]any{"slug": "Bad Slug", "title": "x", "product": "studio"},
		map[string]any{"slug": "no-title", "title": "", "product": "studio"},
		map[string]any{"slug": "nope", "title": "x", "product": "unknown"},
	}
	res := imp(call, append(offDocsBundle(), bad...))
	if res["created"] != float64(6) || res["updated"] != float64(0) || len(res["errors"].([]any)) != 3 ||
		!strings.Contains(fmt.Sprint(res["errors"]), `ไม่รู้จักผลิตภัณฑ์ "unknown"`) {
		t.Fatalf("import result: %v", res)
	}
	if code, out := call("docs:import", map[string]any{"docs": make([]any, 26)}); code != 200 || !strings.Contains(offAErr(out), "25") {
		t.Fatalf("batch limit: %d %v", code, out)
	}
	// editors import too; the same slug is updated, never duplicated
	_, edCall := adminPage(t, e.toSample(), user("Ed", "editor"), "docs:docs", "")
	res = imp(edCall, []any{map[string]any{"slug": "studio-setup", "title": "Setup wizard", "excerpt": "ตั้งค่าครั้งแรกหลังติดตั้ง", "product": "studio",
		"section": "เริ่มต้น", "order": 2, "content": `<h2>Run the wizard</h2><p>Open <code>/setup</code>.</p><h2>Recommended plugins</h2>`}})
	if res["created"] != float64(0) || res["updated"] != float64(1) {
		t.Fatalf("upsert: %v", res)
	}
	if _, list := admin.do("GET", "/api/v1/posts?type=docs_doc&perPage=50", nil); fmt.Sprint(list["total"]) != "6" {
		t.Fatalf("docs in the admin: %v", list["total"])
	}
	// the admin list groups by product, the draft shows there
	html, _ = adminPage(t, e.toSample(), admin, "docs:docs", "")
	offBContains(t, "admin list", html, `data-product="studio"`, `data-product="developers"`, "Secret draft doc", `/admin/content/docs_doc/`,
		`<div class="docs-sec" data-section="เริ่มต้น">`, `<tr data-id="`, `"counts":{`)

	page := func(path string, want int) string {
		t.Helper()
		rec := e.offBSend(offBReq{path: path})
		if rec.Code != want {
			t.Fatalf("%s: %d (want %d)\n%s", path, rec.Code, want, rec.Body.String())
		}
		return rec.Body.String()
	}

	// hub
	h := page("/docs", 200)
	offBContains(t, "hub", h, `<link rel="stylesheet" href="/plugin-assets/docs/assets/docs.css?v=1.1.0">`, `<script defer src="/plugin-assets/docs/assets/docs.js?v=1.1.0">`,
		`class="dx dx-hub"`, "ศูนย์รวมเอกสาร", `data-dx-search`, `<a class="dx-stretch" href="/docs/studio">Anasaki Studio</a>`, "4 บทความ", "1 บทความ", "0 บทความ",
		`<a class="dx-pcard-cta" href="/docs/studio/studio-install">เริ่มอ่าน`, "เริ่มต้นอย่างรวดเร็ว", `href="/docs/developers/dev-cli"`, "<title>ศูนย์รวมเอกสาร")
	if strings.Contains(h, "Secret draft doc") {
		t.Fatal("draft on the hub")
	}

	// product landing: sections by sectionOrder, docs by order
	pl := page("/docs/studio", 200)
	offBContains(t, "product", pl, `class="dx dx-product"`, `<span aria-current="page">Anasaki Studio</span>`, `<details class="dx-nav-group" open data-dx-group="studio/เริ่มต้น"><summary class="dx-nav-title">เริ่มต้น</summary>`,
		`class="dx-card dx-dcard" href="/docs/studio/studio-setup"`, "ตั้งค่าครั้งแรกหลังติดตั้ง", `aria-controls="dx-sidebar"`, `data-dx-switch`)
	if i, j, k := strings.Index(pl, `<h2 id="sec-1">เริ่มต้น`), strings.Index(pl, `<h2 id="sec-2">ออกแบบ`), strings.Index(pl, `<h2 id="sec-3">อื่น ๆ`); i < 0 || j < i || k < j {
		t.Fatalf("section order: %d %d %d", i, j, k)
	}

	// article
	a := page("/docs/studio/studio-setup", 200)
	offBContains(t, "article", a, `class="dx dx-doc"`, `<title>Setup wizard – S</title>`,
		`<a href="/docs/studio/studio-setup" class="is-active" aria-current="page">Setup wizard</a>`,
		`<ol><li><a href="/docs">เอกสาร</a></li><li><a href="/docs/studio">Anasaki Studio</a></li><li><span aria-current="page">เริ่มต้น</span></li></ol>`,
		`<h1>Setup wizard</h1>`, `<p class="dx-lead">ตั้งค่าครั้งแรกหลังติดตั้ง</p>`, `<h2 id="run-the-wizard">Run the wizard<a class="dx-anchor" href="#run-the-wizard"`,
		`<a href="#recommended-plugins" data-dx-toc="recommended-plugins">Recommended plugins</a>`, `<p class="dx-toc-title">ในหน้านี้</p>`,
		`<a class="dx-pager-prev" href="/docs/studio/studio-install" rel="prev">`, `<a class="dx-pager-next" href="/docs/studio/studio-designer" rel="next">`,
		"อัปเดตล่าสุด", `<time datetime="`, `<meta name="description" content="ตั้งค่าครั้งแรกหลังติดตั้ง">`,
		`<link rel="canonical" href="http://example.com/docs/studio/studio-setup">`, `<meta property="og:type" content="article">`, `<details class="dx-toc-m">`,
		`data-dx-copy-md="/docs/studio/studio-setup.md"`, `<a class="dx-chip" href="/docs/studio/studio-setup.md" type="text/markdown">`)
	if strings.Contains(a, "data-dx-edit") {
		t.Fatal("edit link without the editUrl setting")
	}
	if !regexp.MustCompile(`<time datetime="[^"]+">\d{1,2} [ก-๙.]+ 25\d\d</time>`).MatchString(a) {
		t.Fatal("Thai date missing")
	}
	// first article: heading ids unique, tables wrapped, external links new tab, code kept; no prev
	a = page("/docs/studio/studio-install", 200)
	offBContains(t, "first article", a, `<h2 id="สิ่งที่ต้องมี">`, `<h2 id="สิ่งที่ต้องมี-2">`, `<h3 id="ตัวแปร-env">`, `<li class="dx-toc-l3">`,
		`<div class="dx-table"><table>`, `target="_blank" rel="noopener"`, `<code class="language-bash">docker compose up -d</code>`)
	if strings.Contains(a, "dx-pager-prev") || !strings.Contains(a, "dx-pager-next") {
		t.Fatal("first article pager")
	}
	// last article of the product: prev only
	if a = page("/docs/studio/studio-trouble", 200); !strings.Contains(a, `rel="prev"`) || strings.Contains(a, `rel="next"`) {
		t.Fatal("last article pager")
	}

	// core's post URL /docs/<slug> and a wrong product → the canonical article URL; unknown pages → 404 in the layout
	for from, to := range map[string]string{"/docs/studio-setup": "/docs/studio/studio-setup", "/docs/developers/studio-setup": "/docs/studio/studio-setup"} {
		if rec := e.offBSend(offBReq{path: from}); rec.Code != 301 || rec.Header().Get("Location") != to {
			t.Fatalf("%s: %d %q", from, rec.Code, rec.Header().Get("Location"))
		}
	}
	for _, p := range []string{"/docs/nope", "/docs/studio/nope", "/docs/studio/studio-secret", "/docs/studio/Bad%20Slug"} {
		nf := page(p, 404)
		offBContains(t, p, nf, `class="dx dx-nf"`, "ไม่พบหน้าเอกสารนี้", `href="/docs"`, "<html")
	}
	// the type is not routed by core
	if rec := e.offBSend(offBReq{path: "/docs-doc/studio-setup"}); rec.Code != 404 {
		t.Fatalf("core route of the type: %d", rec.Code)
	}

	// search index: published docs of known products only
	rec := e.offBSend(offBReq{path: "/docs/search.json"})
	var idx struct {
		Items []struct {
			Title, URL, Product, ProductName, Section, Excerpt string
			Headings                                           []map[string]string
		}
	}
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || json.Unmarshal(rec.Body.Bytes(), &idx) != nil {
		t.Fatalf("search.json: %d %s", rec.Code, rec.Body.String())
	}
	if len(idx.Items) != 5 || strings.Contains(rec.Body.String(), "Secret") {
		t.Fatalf("search items: %+v", idx.Items)
	}
	found := false
	for _, it := range idx.Items {
		if it.URL == "/docs/studio/studio-setup" && it.ProductName == "Anasaki Studio" && it.Section == "เริ่มต้น" &&
			len(it.Headings) == 2 && it.Headings[1]["id"] == "recommended-plugins" && it.Headings[1]["text"] == "Recommended plugins" {
			found = true
		}
	}
	if !found {
		t.Fatalf("search item: %+v", idx.Items)
	}

	// Markdown of an article, llms.txt and llms-full.txt (published docs of known products only)
	md := e.offBSend(offBReq{path: "/docs/studio/studio-install.md"})
	if md.Code != 200 || !strings.HasPrefix(md.Header().Get("Content-Type"), "text/markdown") {
		t.Fatalf("markdown: %d %s", md.Code, md.Body.String())
	}
	offBContains(t, "markdown", md.Body.String(), "# ติดตั้งด้วย Docker\n", "> เริ่มต้นใช้งานด้วย docker compose", "/docs/studio/studio-install\n",
		"## สิ่งที่ต้องมี\n", "### ตัวแปร .env\n", "```bash\ndocker compose up -d\n```", "| ชื่อ |\n| --- |\n| CODE_DB_PASSWORD |", "[ext](https://example.com/x)")
	if strings.Contains(md.Body.String(), "<") {
		t.Fatalf("HTML left in the Markdown:\n%s", md.Body.String())
	}
	for _, p := range []string{"/docs/developers/studio-install.md", "/docs/studio/studio-secret.md", "/docs/studio/nope.md"} {
		if r := e.offBSend(offBReq{path: p}); r.Code != 404 {
			t.Fatalf("%s: %d", p, r.Code)
		}
	}
	llms := page("/docs/llms.txt", 200)
	offBContains(t, "llms.txt", llms, "# ศูนย์รวมเอกสาร\n", "## Anasaki Studio\n", "/docs/studio/studio-setup.md): ตั้งค่าครั้งแรกหลังติดตั้ง", "/docs/developers/dev-cli.md)", "/docs/llms-full.txt")
	full := page("/docs/llms-full.txt", 200)
	offBContains(t, "llms-full.txt", full, "# Setup wizard\n", "# CLI anasaki-code\n", "## Install the CLI\n")
	if strings.Contains(llms+full, "Secret") || strings.Contains(full, "hidden") {
		t.Fatal("draft in llms.txt")
	}

	// assets
	for _, p := range []string{"/plugin-assets/docs/assets/docs.css?v=1.1.0", "/plugin-assets/docs/assets/docs.js?v=1.1.0"} {
		if r := e.offBSend(offBReq{path: p}); r.Code != 200 || r.Body.Len() < 2000 {
			t.Fatalf("%s: %d", p, r.Code)
		}
	}

	// sitemap: hub, products, every published doc
	sm := e.offBSend(offBReq{path: "/sitemap.xml"}).Body.String()
	offBContains(t, "sitemap", sm, "/docs</loc>", "/docs/studio</loc>", "/docs/developers/dev-cli</loc>", "/docs/studio/studio-setup</loc>", "<lastmod>")
	if strings.Contains(sm, "studio-secret") {
		t.Fatal("draft in the sitemap")
	}

	// an edit in the post editor (REST) moves the doc: the cache is dropped by the post.saved event
	_, list := admin.do("GET", "/api/v1/posts?type=docs_doc&perPage=50&search=CLI", nil)
	cli := list["items"].([]any)[0].(map[string]any)
	if code, b := admin.do("PUT", "/api/v1/posts/"+id(cli), map[string]any{"title": "CLI (anasaki-code)", "meta": map[string]any{"section": "เครื่องมือ"}}); code != 200 {
		t.Fatalf("edit: %d %v", code, b)
	}
	e.code.WaitEvents()
	offBContains(t, "after edit", page("/docs/developers", 200), "CLI (anasaki-code)", `<summary class="dx-nav-title">เครื่องมือ</summary>`)

	// order from the admin page (drag and drop): the troubleshooting doc moves to the top of the first section
	_, list = admin.do("GET", "/api/v1/posts?type=docs_doc&perPage=50&search="+url.QueryEscape("แก้ปัญหา"), nil)
	trouble := list["items"].([]any)[0].(map[string]any)
	code, out := edCall("docs:reorder", map[string]any{"items": []any{
		map[string]any{"id": trouble["id"], "section": "เริ่มต้น", "order": 0, "sectionOrder": 1},
		map[string]any{"id": 999999, "section": "x", "order": 1, "sectionOrder": 1},
	}})
	if code != 200 || out["data"] == nil {
		t.Fatalf("reorder: %d %v", code, out)
	}
	if res := out["data"].(map[string]any); res["updated"] != float64(1) || !strings.Contains(fmt.Sprint(res["errors"]), "ไม่พบบทความ") {
		t.Fatalf("reorder result: %v", res)
	}
	pl = page("/docs/studio", 200)
	if i, j := strings.Index(pl, `href="/docs/studio/studio-trouble"`), strings.Index(pl, `href="/docs/studio/studio-install"`); i < 0 || j < i || strings.Contains(pl, "อื่น ๆ") {
		t.Fatalf("order after reorder: %d %d", i, j)
	}
	if code, out := call("docs:reorder", map[string]any{"items": []any{}}); code != 200 || offAErr(out) == "" {
		t.Fatalf("empty reorder: %d %v", code, out)
	}

	// settings: products, hub texts, legacy redirects (administrators only)
	if code, out := edCall("docs:saveSettings", map[string]any{"hubTitle": "x"}); code != 200 || !strings.Contains(offAErr(out), "manage_options") {
		t.Fatalf("editor saves settings: %d %v", code, out)
	}
	if code, out := call("docs:saveSettings", map[string]any{"products": []any{map[string]any{"id": "Bad id", "name": "x"}}}); code != 200 || offAErr(out) == "" {
		t.Fatalf("bad products: %d %v", code, out)
	}
	if code, out := call("docs:saveSettings", map[string]any{"legacyRedirects": []any{map[string]any{"from": "javascript:x", "to": "/docs"}}}); code != 200 || offAErr(out) == "" {
		t.Fatalf("bad redirects: %d %v", code, out)
	}
	if code, out := call("docs:saveSettings", map[string]any{"editUrl": "javascript:alert(1)"}); code != 200 || offAErr(out) == "" {
		t.Fatalf("bad editUrl: %d %v", code, out)
	}
	code, out = call("docs:saveSettings", map[string]any{
		"hubTitle": "Docs ของเรา", "hubIntro": "คู่มือทั้งหมด <ทดสอบ>", "editUrl": "https://github.com/o/r/edit/main/{product}/{slug}.md",
		"products": []any{
			map[string]any{"id": "studio", "name": map[string]any{"th": "สตูดิโอ", "en": "Studio"}, "desc": "CMS", "icon": "🎨"},
			map[string]any{"id": "developers", "name": "Developers", "desc": "API", "icon": "⌨️"},
		},
		"legacyRedirects": []any{map[string]any{"from": "/blog/studio-getting-started/", "to": "/docs/studio/studio-install"},
			map[string]any{"from": "/blog/studio-designer", "to": "/docs/studio/studio-designer"}},
	})
	if code != 200 || offAErr(out) != "" {
		t.Fatalf("save settings: %d %v", code, out)
	}
	h = page("/docs", 200)
	offBContains(t, "hub after settings", h, "Docs ของเรา", "คู่มือทั้งหมด &lt;ทดสอบ&gt;", ">สตูดิโอ</a>", `<meta name="description" content="คู่มือทั้งหมด &lt;ทดสอบ&gt;">`)
	if strings.Contains(h, "Plugin Store") {
		t.Fatal("removed product still on the hub")
	}
	page("/docs/store", 404)
	offBContains(t, "edit link", page("/docs/studio/studio-install", 200),
		`<a class="dx-chip" href="https://github.com/o/r/edit/main/studio/studio-install.md" data-dx-edit target="_blank" rel="noopener">`)
	for from, to := range map[string]string{
		"/blog/studio-getting-started":       "/docs/studio/studio-install",
		"/blog/studio-getting-started/":      "/docs/studio/studio-install",
		"/blog/studio-designer?utm_source=x": "/docs/studio/studio-designer?utm_source=x",
	} {
		if rec := e.offBSend(offBReq{path: from}); rec.Code != 301 || rec.Header().Get("Location") != to {
			t.Fatalf("redirect %s: %d %q", from, rec.Code, rec.Header().Get("Location"))
		}
	}
	if rec := e.offBSend(offBReq{method: "POST", path: "/blog/studio-designer"}); rec.Code == 301 {
		t.Fatal("POST redirected")
	}
	offBQuiet(t, admin, "docs")
}
