package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/seed"
)

// 2.5.0: a content type made in the admin gets a useful page without a template (photo, its fields with
// labels and formatted values, no date), a page's post list filters / sorts by those fields, and the URL
// parameters of filter inputs narrow it further.
func TestContentTypePagesAndFieldLists(t *testing.T) {
	e := newPluginAPI(t)
	admin, _ := setupSite(t, e.c)
	spec := map[string]any{"type": "listing", "label": map[string]any{"th": "ประกาศ", "en": "Listings"}, "slug": "property", "public": true,
		"supports": []any{"title", "content", "image"},
		"fields": []any{
			map[string]any{"key": "price", "type": "number", "label": map[string]any{"th": "ราคา (บาท)"}},
			map[string]any{"key": "beds", "type": "number", "label": map[string]any{"th": "ห้องนอน"}},
			map[string]any{"key": "district", "type": "select", "label": map[string]any{"th": "เขต"}, "options": []any{
				map[string]any{"value": "bangna", "label": map[string]any{"th": "บางนา"}}, map[string]any{"value": "ari", "label": map[string]any{"th": "อารีย์"}}}},
			map[string]any{"key": "pool", "type": "bool", "label": map[string]any{"th": "สระว่ายน้ำ"}},
			map[string]any{"key": "web", "type": "url", "label": map[string]any{"th": "เว็บโครงการ"}},
		}}
	if code, b := admin.do("PUT", "/api/v1/content-types/listing", spec); code != 200 {
		t.Fatalf("type: %d %v", code, b)
	}
	img := &model.Media{Filename: "house.jpg", URL: "/uploads/house.jpg", Mime: "image/jpeg", Width: 1200, Height: 800}
	if err := e.st.CreateMedia(img); err != nil {
		t.Fatal(err)
	}
	homes := []map[string]any{
		{"title": "คอนโดอารีย์", "slug": "ari-condo", "meta": map[string]any{"price": 3200000, "beds": 1, "district": "ari"}},
		{"title": "บ้านบางนา <หลังใหญ่>", "slug": "bangna-house", "featuredMediaId": img.ID,
			"meta": map[string]any{"price": 6900000, "beds": 3, "district": "bangna", "pool": true, "web": "https://example.com/bangna?a=1&b=2"}},
		{"title": "ทาวน์โฮมบางนา", "slug": "bangna-town", "meta": map[string]any{"price": 4500000, "beds": 3, "district": "bangna"}},
	}
	for _, h := range homes {
		h["type"], h["status"] = "listing", "publish"
		if code, b := admin.do("POST", "/api/v1/posts", h); code != 201 {
			t.Fatalf("post %v: %d %v", h["title"], code, b)
		}
	}
	page := func(path string) string {
		t.Helper()
		rec := e.c.send("GET", path, nil, nil)
		if rec.Code != 200 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	// the single page without a template: title, photo, fields with labels (formatted, escaped), no date line
	h := page("/property/bangna-house")
	for _, want := range []string{"บ้านบางนา &lt;หลังใหญ่&gt;", `src="/uploads/house.jpg"`, `<dl class="rea-fields">`,
		`<dt>ราคา (บาท)</dt><dd>6,900,000</dd>`, `<dt>เขต</dt><dd>บางนา</dd>`, `<dt>สระว่ายน้ำ</dt><dd>✓ ใช่</dd>`,
		`<a href="https://example.com/bangna?a=1&amp;b=2" rel="nofollow noopener" target="_blank">example.com/bangna</a>`} {
		if !strings.Contains(h, want) {
			t.Errorf("single page lacks %q", want)
		}
	}
	if strings.Contains(h, "n-post-date") {
		t.Error("a listing's page shows a publish date")
	}
	// a field without a value is left out
	if a := page("/property/ari-condo"); strings.Contains(a, "สระว่ายน้ำ") || strings.Contains(a, "เว็บโครงการ") {
		t.Error("empty fields listed")
	}

	// a page with a list: 3 beds or more, most expensive first; inputs narrow it by URL
	design := map[string]any{"id": "root", "type": "frame", "children": []any{
		map[string]any{"id": "homes", "type": "postList", "props": map[string]any{"query": map[string]any{"postType": "listing",
			"orderBy": "price", "orderDesc": true, "where": []any{map[string]any{"field": "beds", "op": ">=", "value": "3"}}}},
			"children": []any{map[string]any{"id": "t", "type": "text", "props": map[string]any{"text": "[{{post.title}}]"}},
				map[string]any{"id": "pf", "type": "postFields", "props": map[string]any{"max": 1, "layout": "inline"}}}}}}
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Find", "slug": "find", "status": "publish", "design": design}); code != 201 {
		t.Fatalf("page: %d %v", code, b)
	}
	order := func(path string) string {
		h := page(path)
		h = h[strings.Index(h, "<body"):] // the cards' [title] markers, not CSS selectors
		var out []string
		for _, part := range strings.Split(h, "[")[1:] {
			out = append(out, part[:strings.Index(part, "]")])
		}
		return strings.Join(out, " | ")
	}
	if got := order("/find"); got != "บ้านบางนา &lt;หลังใหญ่&gt; | ทาวน์โฮมบางนา" {
		t.Errorf("list: %s", got)
	}
	if got := order("/find?homes_max_price=5000000"); got != "ทาวน์โฮมบางนา" {
		t.Errorf("max price: %s", got)
	}
	if got := order("/find?homes_f_district=ari"); got != "" {
		t.Errorf("district ari with 3 beds: %s", got)
	}
	if f := page("/find"); !strings.Contains(f, `<div class="n-pf rea-fields-inline"><dl class="rea-fields"><div class="rea-field" data-field="price"><dt>ราคา (บาท)</dt><dd>6,900,000</dd></div></dl></div>`) {
		t.Errorf("card fields (max 1, inline) missing")
	}

	// the archive: a type's archive has no "all posts in this category" subtitle; a category's has
	if a := page("/property"); strings.Contains(a, "บทความทั้งหมดในหมวดหมู่นี้") {
		t.Error("type archive says all posts in this category")
	}
}

// 2.5.0: the archive template of an older site is upgraded once — the fixed subtitle becomes {{post.excerpt}}
// and its cards hide the date of content type posts, followed by their first fields.
func TestUpgradeArchiveTemplate(t *testing.T) {
	old := `{"id":"root","type":"frame","children":[
	 {"id":"h","type":"frame","children":[{"id":"st","type":"text","props":{"text":"บทความทั้งหมดในหมวดหมู่นี้","tag":"p"}}]},
	 {"id":"pl","type":"postList","children":[{"id":"card","type":"frame","children":[
	   {"id":"tt","type":"text","props":{"text":"{{post.title}}"}},
	   {"id":"dt","type":"text","props":{"text":"{{post.date}}"}}]}]},
	 {"id":"other","type":"text","props":{"text":"{{post.date}}"}}]}`
	out, changed := seed.UpgradeArchiveDesign([]byte(old))
	if !changed {
		t.Fatal("not upgraded")
	}
	s := string(out)
	for _, want := range []string{`"text":"{{post.excerpt}}"`, `"showWhen":"post.type == 'post'"`, `"type":"postFields"`, `"showWhen":"post.type != 'post'"`} {
		if !strings.Contains(s, want) {
			t.Errorf("upgraded design lacks %s:\n%s", want, s)
		}
	}
	var root map[string]any
	_ = json.Unmarshal(out, &root)
	if other := fmt.Sprint(root["children"].([]any)[2]); strings.Contains(other, "showWhen") {
		t.Error("a date outside the post list was changed")
	}
	if _, again := seed.UpgradeArchiveDesign(out); again {
		t.Error("the upgrade is not idempotent")
	}
}

// 2.5.0: finding content — tags (/tag/<slug> + the post's tag links), the RSS feed, the search page, an author's
// page, /blog/page/N, and contributors sending posts for review.
func TestFindingContent(t *testing.T) {
	e := newPluginAPI(t)
	admin, user := setupSite(t, e.c)
	code, tg := admin.do("POST", "/api/v1/terms", map[string]any{"name": "กาแฟ", "slug": "coffee", "taxonomy": "tag"})
	if code != 201 {
		t.Fatalf("tag: %d %v", code, tg)
	}
	for i := 1; i <= 13; i++ {
		body := map[string]any{"type": "post", "title": fmt.Sprintf("Story %02d", i), "slug": fmt.Sprintf("story-%02d", i), "status": "publish",
			"excerpt": "about things", "content": "<p>body</p>"}
		if i == 3 {
			body["title"], body["excerpt"] = "Latte art basics", "milk & coffee <b>"
			body["terms"] = map[string]any{"tag": []any{tg["id"]}}
		}
		if code, b := admin.do("POST", "/api/v1/posts", body); code != 201 {
			t.Fatalf("post %d: %d %v", i, code, b)
		}
	}
	page := func(path string, want int) string {
		t.Helper()
		rec := e.c.send("GET", path, nil, nil)
		if rec.Code != want {
			t.Fatalf("%s: %d (want %d)", path, rec.Code, want)
		}
		return rec.Body.String()
	}
	// tags
	if h := page("/tag/coffee", 200); !strings.Contains(h, "Latte art basics") || strings.Contains(h, "Story 05") {
		t.Error("tag archive")
	}
	if h := page("/blog/story-03", 200); !strings.Contains(h, `<a href="/tag/coffee" rel="tag">#กาแฟ</a>`) {
		t.Error("the post does not link its tag")
	}
	// feed
	f := page("/feed", 200)
	if !strings.Contains(f, `<rss version="2.0">`) || !strings.Contains(f, "<title>Latte art basics</title>") || !strings.Contains(f, "milk &amp; coffee") {
		t.Errorf("feed:\n%s", f[:min(len(f), 600)])
	}
	if !strings.Contains(page("/blog/story-01", 200), `type="application/rss+xml"`) {
		t.Error("pages do not announce the feed")
	}
	// search
	s := page("/search?q=latte", 200)
	if !strings.Contains(s, "Latte art basics") || strings.Contains(s, "Story 05") || !strings.Contains(s, `<form`) || !strings.Contains(s, `name="q"`) {
		t.Error("search results")
	}
	if strings.Contains(page("/search", 200), "Story 05") {
		t.Error("an empty search lists everything")
	}
	// author
	_, me := admin.do("GET", "/api/v1/auth/me", nil)
	uid := jsonNum(me["id"].(float64))
	if h := page("/author/"+uid, 200); !strings.Contains(h, "Story 13") {
		t.Error("author page")
	}
	page("/author/99999", 404)
	// /blog/page/2 → ?page=2
	if rec := e.c.send("GET", "/blog/page/2", nil, nil); rec.Code != 301 || rec.Header().Get("Location") != "/blog?page=2" {
		t.Errorf("paged address: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	// contributors send for review; the post is not public and can still be edited by them
	con := user("Con", "contributor")
	code, p := con.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "Guest story", "status": "publish"})
	if code != 201 || p["status"] != "pending" {
		t.Fatalf("contributor publish: %d %v", code, p)
	}
	if code, b := con.do("PUT", "/api/v1/posts/"+id(p), map[string]any{"title": "Guest story v2", "status": "publish"}); code != 200 || b["status"] != "pending" {
		t.Fatalf("contributor edit: %d %v", code, b)
	}
	if _, l := admin.do("GET", "/api/v1/posts?status=pending", nil); l["total"] != float64(1) {
		t.Fatalf("pending list: %v", l["total"])
	}
	if code, b := admin.do("PUT", "/api/v1/posts/"+id(p), map[string]any{"status": "publish"}); code != 200 || b["status"] != "publish" {
		t.Fatalf("approve: %d %v", code, b)
	}
}
