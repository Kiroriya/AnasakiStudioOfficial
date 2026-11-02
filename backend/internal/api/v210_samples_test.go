package api

import (
	"bytes"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// The CMS 2.1.0 proof plugins in examples/js-plugins: catalog (post type with
// the new field types, two taxonomies, db.terms, PostInput.terms, a secret
// setting) and image-optimizer (media.beforeSave, media.uploaded → read /
// resize / replace).

func TestSampleCatalog(t *testing.T) {
	coreAt210(t)
	e := newSampleAPI(t)
	admin, _ := setupSite(t, e.c)
	installSample(t, admin, "catalog")
	body := `{"products":[{"title":"Anvil","price":990,"sku":"A-1","brand":"Acme","category":"Tools","parentCategory":"Hardware",
		"specs":[{"name":"Weight","value":"50 kg"}]},{"title":"Rocket","price":5000,"brand":"Acme","category":"Hardware"}]}`
	if code, b := getJSON(t, admin, "POST", "/catalog/import", body, nil); code != 200 || b["imported"] != 2.0 {
		t.Fatalf("import: %d %v", code, b)
	}
	if code, _ := getJSON(t, e.c, "POST", "/catalog/import", body, nil); code == 200 {
		t.Fatal("a visitor imports")
	}
	brands := sampleRouteList(t, e.c, "/catalog/brands")
	if len(brands) != 1 || brands[0]["slug"] != "acme" || brands[0]["count"] != 2.0 {
		t.Fatalf("brands: %v", brands)
	}
	p := sampleRoute(t, e.c, "/catalog/product/anvil")
	if p["price"] != 990.0 || jsonString(p["categories"]) != `["tools"]` || jsonString(p["specs"]) != `[{"name":"Weight","value":"50 kg"}]` {
		t.Fatalf("product: %v", p)
	}
	// hierarchical archive: /product-category/hardware lists both (Tools is its child)
	if code, h := e.c.public("GET", "/product-category/hardware"); code != 200 || !strings.Contains(h, "Anvil") || !strings.Contains(h, "Rocket") {
		t.Fatalf("category archive: %d %.300s", code, h)
	}
	if code, h := e.c.public("GET", "/brands/acme"); code != 200 || !strings.Contains(h, "Rocket") {
		t.Fatalf("brand archive: %d", code)
	}
	// the admin: two taxonomies with terms screens
	_, txs := admin.doList("GET", "/api/v1/taxonomies")
	if len(txs) != 3 || txs[1]["id"] != "catalog_category" || txs[1]["hierarchical"] != true || txs[2]["slug"] != "brands" {
		t.Fatalf("taxonomies: %v", txs)
	}
	// the feed: off without a key, then guarded by the secret
	if code, _ := e.c.public("GET", "/catalog/feed.json"); code != 404 {
		t.Fatalf("feed without a key: %d", code)
	}
	if code, b := admin.do("PUT", "/api/v1/package-plugins/catalog/settings", map[string]any{"values": map[string]any{"feedKey": "k-123", "currency": "THB"}}); code != 200 || strs(b["secretsSet"]) != "feedKey" {
		t.Fatalf("set the key: %d %v", code, b)
	}
	if code, _ := getJSON(t, e.c, "GET", "/catalog/feed.json", "", map[string]string{"X-Api-Key": "wrong"}); code != 401 {
		t.Fatalf("wrong key: %d", code)
	}
	if code, f := getJSON(t, e.c, "GET", "/catalog/feed.json", "", map[string]string{"X-Api-Key": "k-123"}); code != 200 || f["total"] != 2.0 {
		t.Fatalf("feed: %d %v", code, f)
	}
}

func TestSampleImageOptimizer(t *testing.T) {
	coreAt210(t)
	e := newSampleAPI(t)
	admin, _ := setupSite(t, e.c)
	installSample(t, admin, "image-optimizer")
	if code, _ := admin.do("PUT", "/api/v1/package-plugins/image-optimizer/settings", map[string]any{"values": map[string]any{"maxWidth": 400, "quality": 80, "maxUploadMB": 0}}); code != 200 {
		t.Fatal("settings")
	}
	upload := func(name string, data []byte) (int, map[string]any) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write(data)
		_ = mw.Close()
		rec := admin.send("POST", "/api/v1/media", &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
		out := map[string]any{}
		_ = jsonUnmarshalTest(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	code, m := upload("wide.png", bigPNG(1200, 300))
	if code != 201 || m["width"] != 1200.0 {
		t.Fatalf("upload: %d %v", code, m)
	}
	e.code.WaitEvents()
	got, err := e.st.GetMedia(int64(m["id"].(float64)))
	if err != nil || got.Width != 400 || got.Height != 100 || got.Size >= int64(m["size"].(float64)) {
		t.Fatalf("not optimized: %+v %v", got, err)
	}
	// a small image is left alone
	code, small := upload("dot.png", tinyPNG)
	e.code.WaitEvents()
	if s, _ := e.st.GetMedia(int64(small["id"].(float64))); code != 201 || s.Size != int64(len(tinyPNG)) {
		t.Fatalf("small image changed: %d %+v", code, s)
	}
	// maxUploadMB refuses before storing
	admin.do("PUT", "/api/v1/package-plugins/image-optimizer/settings", map[string]any{"values": map[string]any{"maxWidth": 400, "quality": 80, "maxUploadMB": 1}})
	big := bigPNG(1500, 1500)
	for len(big) <= 1<<20 { // make sure it is over 1 MB
		big = append(big, big...)
	}
	if code, b := upload("huge.png", big); code != 400 || !strings.Contains(jsonString(b), "1 MB") {
		t.Fatalf("over the limit: %d %v", code, b)
	}
}

// coreAt210: the proof plugins need minCore 2.1.0 (a test build says "dev").
func coreAt210(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.1.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
}

// sampleRouteList calls a public route answering a JSON array.
func sampleRouteList(t *testing.T, c *client, path string) []map[string]any {
	t.Helper()
	code, l := c.doList("GET", path)
	if code != 200 {
		t.Fatalf("GET %s: %d", path, code)
	}
	return l
}
