package api

import (
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/maps: the map block from its own address, from each post's field in a content type's
// template (so every listing has its own map), OpenStreetMap for coordinates, and the settings' default.
func TestOfficialMaps(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "maps")
	offBPkgSettings(t, admin, "maps", map[string]any{"address": "Siam Paragon, Bangkok"})

	page := func(slug string, values map[string]any) string {
		t.Helper()
		code, pg := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": slug, "slug": slug, "status": "publish",
			"design": map[string]any{"id": "root", "type": "frame", "children": []any{
				map[string]any{"id": "m", "type": "pluginBlock", "props": map[string]any{"plugin": "maps", "block": "map", "values": values}}}}})
		if code != 201 {
			t.Fatalf("page: %d %v", code, pg)
		}
		return e.offBSend(offBReq{path: "/" + slug}).Body.String()
	}
	out := page("contact", map[string]any{})
	offBContains(t, "default address", out, `<iframe src="https://www.google.com/maps?q=Siam%20Paragon%2C%20Bangkok&amp;z=15&amp;output=embed"`,
		`height="360"`, `href="https://www.google.com/maps/dir/?api=1&amp;destination=Siam%20Paragon%2C%20Bangkok"`)
	out = page("osm", map[string]any{"address": "13.7466, 100.5393", "provider": "osm", "zoom": 16, "directions": false, "height": 500})
	offBContains(t, "osm", out, `src="https://www.openstreetmap.org/export/embed.html?bbox=`, `marker=13.7466,100.5393`, `height="500"`)
	if strings.Contains(out, `class="amap-dir"`) {
		t.Fatal("directions shown though turned off")
	}
	// a content type whose template reads the "address" field
	code, typ := admin.do("PUT", "/api/v1/content-types/listing", map[string]any{"type": "listing", "label": map[string]any{"th": "ประกาศ", "en": "Listings"},
		"slug": "listings", "public": true, "supports": []any{"title"},
		"fields": []any{map[string]any{"key": "address", "type": "text", "label": map[string]any{"th": "ที่ตั้ง"}}}})
	if code != 200 {
		t.Fatalf("content type: %d %v", code, typ)
	}
	tpl := map[string]any{"id": "root", "type": "frame", "children": []any{
		map[string]any{"id": "m", "type": "pluginBlock", "props": map[string]any{"plugin": "maps", "block": "map", "values": map[string]any{"field": "address"}}}}}
	_, tp := admin.do("POST", "/api/v1/posts", map[string]any{"type": "template", "title": "Listing", "design": tpl})
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"post_template_listing": id(tp)}); code != 200 {
		t.Fatalf("template: %d %v", code, b)
	}
	admin.do("POST", "/api/v1/posts", map[string]any{"type": "listing", "title": "Condo", "slug": "condo", "status": "publish", "meta": map[string]any{"address": "Chiang Mai Old City"}})
	out = e.offBSend(offBReq{path: "/listings/condo"}).Body.String()
	offBContains(t, "listing map", out, `q=Chiang%20Mai%20Old%20City`)
}
