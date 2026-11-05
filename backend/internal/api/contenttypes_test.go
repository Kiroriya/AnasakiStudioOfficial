package api

import (
	"fmt"
	"strings"
	"testing"
)

// CMS 2.2.2: a content type made in the admin (no code) behaves like a site.postType type — listed for the
// admin menu with its fields, posts with validated fields, public archive and single pages; clashes are refused.
func TestContentTypesNoCode(t *testing.T) {
	e := newPluginAPI(t)
	admin, user := setupSite(t, e.c)
	spec := map[string]any{"type": "property", "label": map[string]any{"th": "ประกาศขายบ้าน", "en": "Properties"},
		"singular": map[string]any{"th": "ประกาศ", "en": "Property"}, "icon": "🏠", "slug": "homes", "public": true,
		"supports": []any{"title", "content", "image"},
		"fields": []any{
			map[string]any{"key": "price", "type": "number", "label": map[string]any{"th": "ราคา"}, "required": true, "min": 0},
			map[string]any{"key": "kind", "type": "select", "label": map[string]any{"th": "ประเภท"}, "options": []any{
				map[string]any{"value": "house", "label": map[string]any{"th": "บ้านเดี่ยว"}}, map[string]any{"value": "condo"}}},
			map[string]any{"key": "rooms", "type": "repeater", "label": map[string]any{"th": "ห้อง"}, "fields": []any{
				map[string]any{"key": "name", "type": "text"}, map[string]any{"key": "size", "type": "number"}}},
		}}
	if code, b := admin.do("PUT", "/api/v1/content-types/property", spec); code != 200 || b["slug"] != "homes" {
		t.Fatalf("create: %d %v", code, b)
	}
	// refused: bad field, a select without options, reserved address, a clash, an editor
	for name, bad := range map[string]map[string]any{
		"bad key":   {"type": "thing", "label": map[string]any{"th": "x"}, "slug": "things", "fields": []any{map[string]any{"key": "1x", "type": "text"}}},
		"no option": {"type": "thing", "label": map[string]any{"th": "x"}, "slug": "things", "fields": []any{map[string]any{"key": "k", "type": "select"}}},
		"reserved":  {"type": "thing", "label": map[string]any{"th": "x"}, "slug": "api"},
		"blog base": {"type": "thing", "label": map[string]any{"th": "x"}, "slug": "blog"},
		"same slug": {"type": "thing", "label": map[string]any{"th": "x"}, "slug": "homes"},
		"builtin":   {"type": "page", "label": map[string]any{"th": "x"}, "slug": "pagesx"},
	} {
		id, _ := bad["type"].(string)
		if code, b := admin.do("PUT", "/api/v1/content-types/"+id, bad); code != 400 {
			t.Errorf("%s accepted: %d %v", name, code, b)
		}
	}
	if code, _ := user("Ed", "editor").do("PUT", "/api/v1/content-types/property", spec); code != 403 {
		t.Fatalf("editor may change content types: %d", code)
	}
	// the admin menu / editor catalog
	_, pt := admin.do("GET", "/api/v1/post-types", nil)
	s := fmt.Sprint(pt["postTypes"])
	if !strings.Contains(s, "type:property") || !strings.Contains(s, "admin:true") || !strings.Contains(s, "slug:homes") || !strings.Contains(s, "price") {
		t.Fatalf("post-types: %v", pt["postTypes"])
	}
	// the designer / admin menu catalog (what the sidebar reads first) has it too
	_, bind := admin.do("GET", "/api/v1/code/bindings", nil)
	if s := fmt.Sprint(bind["postTypes"]); !strings.Contains(s, "type:property") {
		t.Fatalf("bindings: %v", bind["postTypes"])
	}
	// a post of the type: the fields are validated like code fields
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "property", "title": "บ้านริมน้ำ", "slug": "riverside", "status": "publish",
		"meta": map[string]any{"price": "abc"}}); code != 400 {
		t.Fatalf("bad price accepted: %d %v", code, b)
	}
	code, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "property", "title": "บ้านริมน้ำ", "slug": "riverside", "status": "publish",
		"content": "<p>ติดแม่น้ำเจ้าพระยา</p>", "meta": map[string]any{"price": 4500000, "kind": "house", "rooms": []any{map[string]any{"name": "นอน", "size": 16}}}})
	if code != 201 {
		t.Fatalf("post: %d %v", code, p)
	}
	if rec := e.c.send("GET", "/homes/riverside", nil, nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "บ้านริมน้ำ") {
		t.Fatalf("single page: %d", rec.Code)
	}
	if rec := e.c.send("GET", "/homes", nil, nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "บ้านริมน้ำ") {
		t.Fatalf("archive: %d", rec.Code)
	}
	// the list for the settings page
	_, l := admin.do("GET", "/api/v1/content-types", nil)
	if s := fmt.Sprint(l["types"]); !strings.Contains(s, "postCount:1") || !strings.Contains(s, "active:true") {
		t.Fatalf("list: %v", l)
	}
	// delete: refused while a post exists
	if code, _ := admin.do("DELETE", "/api/v1/content-types/property", nil); code != 409 {
		t.Fatalf("deleted with a post: %d", code)
	}
	if code, b := admin.do("DELETE", fmt.Sprintf("/api/v1/posts/%v?force=1", p["id"]), nil); code != 204 && code != 200 {
		t.Fatalf("delete post: %d %v", code, b)
	}
	if code, b := admin.do("DELETE", "/api/v1/content-types/property", nil); code != 200 {
		t.Fatalf("delete type: %d %v", code, b)
	}
	if rec := e.c.send("GET", "/homes", nil, nil); rec.Code != 404 {
		t.Fatalf("archive after delete: %d", rec.Code)
	}
}
