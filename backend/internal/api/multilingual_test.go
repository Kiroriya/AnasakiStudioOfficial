package api

import (
	"fmt"
	"strings"
	"testing"
)

// 2.5.0 multilingual content: an extra language, translations made from the original, /<lang>/ addresses with
// redirects to the right version, lists of the page's language, hreflang, the language switch block and a
// site header per language.
func TestMultilingualContent(t *testing.T) {
	e := newPluginAPI(t)
	admin, _ := setupSite(t, e.c)
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"content_languages": "en, xx_bad"}); code != 400 {
		t.Fatalf("bad language code accepted: %d %v", code, b)
	}
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"content_languages": "en"}); code != 200 || b["content_languages"] != "en" {
		t.Fatalf("languages: %d %v", code, b)
	}
	// two header components: the site's and the English one
	hdr := func(text string) string {
		_, c := admin.do("POST", "/api/v1/components", map[string]any{"name": text, "node": map[string]any{"id": "h", "type": "text", "props": map[string]any{"text": text}}})
		return id(c)
	}
	th, en := hdr("เมนูไทย"), hdr("English menu")
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"site_header_component": th, "site_header_component_en": en}); code != 200 {
		t.Fatalf("headers: %d %v", code, b)
	}
	design := func(text string) map[string]any {
		return map[string]any{"id": "root", "type": "frame", "children": []any{
			map[string]any{"id": "hd", "type": "instance", "props": map[string]any{"componentId": numOf(th)}},
			map[string]any{"id": "t", "type": "text", "props": map[string]any{"text": text}},
			map[string]any{"id": "ls", "type": "langSwitch"},
			map[string]any{"id": "pl", "type": "postList", "props": map[string]any{"query": map[string]any{"postType": "post", "limit": 10}},
				"children": []any{map[string]any{"id": "pt", "type": "text", "props": map[string]any{"text": "item:{{post.title}}"}}}}}}
	}
	_, about := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "เกี่ยวกับเรา", "slug": "about", "status": "publish", "design": design("สวัสดี")})
	_, post := admin.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "ข่าวไทย", "slug": "news", "status": "publish", "content": "<p>ไทย</p>"})

	// translate: a draft copy linked to the original; asking again opens it; unknown languages are refused
	if code, _ := admin.do("POST", "/api/v1/posts/"+id(about)+"/translate", map[string]any{"lang": "fr"}); code != 400 {
		t.Fatalf("language not on: %d", code)
	}
	code, tr := admin.do("POST", "/api/v1/posts/"+id(about)+"/translate", map[string]any{"lang": "en"})
	if code != 201 || tr["lang"] != "en" || fmt.Sprint(tr["translationOf"]) != id(about) || tr["status"] != "draft" || tr["slug"] != "about-en" {
		t.Fatalf("translate: %d %v", code, tr)
	}
	if code, again := admin.do("POST", "/api/v1/posts/"+id(tr)+"/translate", map[string]any{"lang": "en"}); code != 200 || id(again) != id(tr) {
		t.Fatalf("translate again: %d %v", code, again)
	}
	admin.do("PUT", "/api/v1/posts/"+id(tr), map[string]any{"title": "About us", "slug": "about-us", "status": "publish", "design": design("Hello")})
	_, ptr := admin.do("POST", "/api/v1/posts/"+id(post)+"/translate", map[string]any{"lang": "en"})
	admin.do("PUT", "/api/v1/posts/"+id(ptr), map[string]any{"title": "English news", "status": "publish"})
	_, list := admin.doList("GET", "/api/v1/posts/"+id(about)+"/translations")
	if len(list) != 2 || list[1]["url"] != "/en/about-us" {
		t.Fatalf("translations: %v", list)
	}

	get := func(path string) (int, string, string) {
		rec := e.c.send("GET", path, nil, nil)
		return rec.Code, rec.Body.String(), rec.Header().Get("Location")
	}
	code, body, _ := get("/en/about-us")
	if code != 200 || !strings.Contains(body, `<html lang="en"`) || !strings.Contains(body, "Hello") || !strings.Contains(body, "English menu") ||
		strings.Contains(body, "เมนูไทย") || !strings.Contains(body, "item:English news") || strings.Contains(body, "item:ข่าวไทย") {
		t.Fatalf("/en/about-us: %d %.3000s", code, body)
	}
	for _, want := range []string{`<link rel="alternate" hreflang="th" href="`, `/about"`, `hreflang="en" href="`, `/en/about-us"`,
		`class="` /* the switch */, `rea-langs`, `<a href="/about" hreflang="th" lang="th">ไทย</a>`, `aria-current="true">English</a>`} {
		if !strings.Contains(body, want) {
			i := strings.Index(body, "rea-langs\"><")
			t.Fatalf("/en/about-us lacks %s: %s", want, body[max(0, i-20):min(len(body), i+400)])
		}
	}
	code, body, _ = get("/about")
	if code != 200 || !strings.Contains(body, `<html lang="th"`) || !strings.Contains(body, "item:ข่าวไทย") || strings.Contains(body, "item:English news") || !strings.Contains(body, "เมนูไทย") {
		t.Fatalf("/about: %d", code)
	}
	// wrong addresses go to the right version
	for path, to := range map[string]string{"/about-us": "/en/about-us", "/en/about": "/en/about-us", "/blog/news-en": "/en/blog/news-en"} {
		if code, _, loc := get(path); code != 301 || loc != to {
			t.Errorf("%s → %d %s (want %s)", path, code, loc, to)
		}
	}
	if code, body, _ := get("/en/blog"); code != 200 || !strings.Contains(body, "English news") || strings.Contains(body, "ข่าวไทย") {
		t.Fatalf("/en/blog: %d", code)
	}
	if code, _, _ := get("/en/blog/news-en"); code != 200 {
		t.Fatalf("/en/blog/news-en: %d", code)
	}
	// /en: the home page's translation (404 until there is one)
	admin.do("PUT", "/api/v1/settings", map[string]any{"home_page_id": id(about)})
	if code, _, _ := get("/en"); code != 200 {
		t.Fatalf("/en with the home page translated: %d", code)
	}
	// lists in the admin: every language, or one
	if _, l := admin.do("GET", "/api/v1/posts?type=page&lang=en", nil); fmt.Sprint(l["total"]) != "1" {
		t.Fatalf("?lang=en: %v", l["total"])
	}
}

func numOf(s string) float64 {
	var n float64
	fmt.Sscan(s, &n)
	return n
}
