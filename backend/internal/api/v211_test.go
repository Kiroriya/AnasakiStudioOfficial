package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// CMS 2.1.1: fixes from the Qraft build (anasaki-studio-feedback.md).

func TestDesignDuplicateIDs(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	dup := map[string]any{"id": "root", "type": "frame", "width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"},
		"children": []any{
			map[string]any{"id": "imin", "type": "frame", "width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"}},
			map[string]any{"id": "imin", "type": "text", "width": map[string]any{"mode": "hug"}, "height": map[string]any{"mode": "hug"}, "props": map[string]any{"text": "x"}},
		}}
	code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Dup", "design": dup})
	if code != 400 || !strings.Contains(b["error"].(string), `"imin"`) {
		t.Fatalf("duplicate layer id accepted: %d %v", code, b)
	}
	if code, b := admin.do("POST", "/api/v1/components", map[string]any{"name": "C", "node": dup}); code != 400 {
		t.Fatalf("component with a duplicate id: %d %v", code, b)
	}
	dup["children"].([]any)[1].(map[string]any)["id"] = "imin2"
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Ok", "design": dup}); code != 201 {
		t.Fatalf("unique ids: %d %v", code, b)
	}
}

func TestPostTermsBySlugAndJSONErrors(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	_, news := admin.do("POST", "/api/v1/terms", map[string]any{"name": "News", "slug": "news", "taxonomy": "category"})
	code, p := admin.do("POST", "/api/v1/posts", map[string]any{"title": "T", "terms": map[string]any{"category": []any{"news"}}})
	if code != 201 {
		t.Fatalf("terms by slug: %d %v", code, p)
	}
	cats, _ := p["categories"].([]any)
	if len(cats) != 1 || cats[0].(map[string]any)["id"] != news["id"] {
		t.Fatalf("slug not resolved: %v", p["categories"])
	}
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"title": "T", "terms": map[string]any{"category": []any{"nope"}}}); code != 400 || !strings.Contains(b["error"].(string), `"nope"`) {
		t.Fatalf("unknown slug: %d %v", code, b)
	}
	// decode errors name the field instead of Go's struct names
	raw := func(body string) string {
		req := httptest.NewRequest("POST", "/api/v1/posts", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+admin.token)
		rec := httptest.NewRecorder()
		admin.h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, rec.Code)
		}
		return rec.Body.String()
	}
	if b := raw(`{"title": 5}`); !strings.Contains(b, "title") || strings.Contains(b, "Go struct") || strings.Contains(b, "postInput") {
		t.Fatalf("type error: %s", b)
	}
	if b := raw(`{"terms": {"category": [true]}}`); strings.Contains(b, "Go struct") || !strings.Contains(b, "slug") {
		t.Fatalf("terms entry error: %s", b)
	}
	if b := raw(`{"title": `); strings.Contains(b, "unexpected EOF") {
		t.Fatalf("truncated JSON: %s", b)
	}
}

func TestDeletedPageLeavesMenus(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	_, about := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "เกี่ยวกับเรา", "slug": "เกี่ยวกับ", "status": "publish"})
	_, keep := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Contact", "slug": "contact", "status": "publish"})
	_, m := admin.do("POST", "/api/v1/menus", map[string]any{"name": "Main", "items": []any{
		map[string]any{"label": "Home", "url": "/"},
		map[string]any{"label": "About", "url": "/เกี่ยวกับ"},
		map[string]any{"label": "Contact", "url": "/contact"},
		map[string]any{"label": "About again", "url": "/%E0%B9%80%E0%B8%81%E0%B8%B5%E0%B9%88%E0%B8%A2%E0%B8%A7%E0%B8%81%E0%B8%B1%E0%B8%9A/"},
	}})
	if code, _ := admin.do("DELETE", "/api/v1/posts/"+id(about), nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	_, list := admin.doAny("GET", "/api/v1/menus", nil)
	var items []any
	for _, x := range list.([]any) {
		if x.(map[string]any)["id"] == m["id"] {
			items = x.(map[string]any)["items"].([]any)
		}
	}
	if len(items) != 2 || items[0].(map[string]any)["url"] != "/" || items[1].(map[string]any)["url"] != "/contact" {
		t.Fatalf("menu after delete: %v", items)
	}
	_ = keep
}
