package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// starterKit reads a kit file the Store serves (store/backend/starters, made by cmd/kits).
func starterKit(t *testing.T, id string) map[string]any {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "store", "backend", "starters", id+".json"))
	if err != nil {
		t.Skip("store kits not next to the CMS: ", err)
	}
	var k map[string]any
	if err := json.Unmarshal(b, &k); err != nil {
		t.Fatal(err)
	}
	return k
}

// 2.5.0 + store 1.6.0: the setup wizard lists the store's starter kits (only before setup) and builds the site
// from the chosen one — pages in the site layout, menu, content type, sample posts with fields, the type's own
// archive; HTML embeds in a kit are dropped; the kit's plugins are offered (ticked) in the next step.
func TestSetupFromStarterKit(t *testing.T) {
	fs := newFakeStore(t)
	cafe := starterKit(t, "cafe")
	// a kit may not smuggle raw HTML in
	home := cafe["pages"].([]any)[0].(map[string]any)
	home["sections"] = append(home["sections"].([]any), map[string]any{"id": "evil", "type": "embed", "props": map[string]any{"html": "<script>alert(1)</script>"}})
	fs.kits = map[string]map[string]any{"cafe": cafe}
	fs.put(t, fakeOfficial("booking", false, nil))
	e := newPluginAPI(t)

	code, b := e.c.do("GET", "/api/v1/setup/kits", nil)
	if code != 200 || b["storeAvailable"] != true || !strings.Contains(fmt.Sprint(b["items"]), "id:cafe") {
		t.Fatalf("kits: %d %v", code, b)
	}
	code, body := e.c.do("POST", "/api/v1/setup", map[string]any{"siteTitle": "Ban Kafe", "adminName": "Owner",
		"adminEmail": "owner@x.io", "adminPassword": "secret123", "locale": "th", "language": "th", "demo": true, "kit": "cafe"})
	if code != 201 {
		t.Fatalf("setup: %d %v", code, body)
	}
	admin := e.c.as(body["token"].(string))
	if code, _ := e.c.do("GET", "/api/v1/setup/kits", nil); code != 404 {
		t.Fatalf("kits after setup: %d", code)
	}
	page := func(path string) string {
		t.Helper()
		rec := e.c.send("GET", path, nil, nil)
		if rec.Code != 200 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	h := page("/")
	for _, want := range []string{"กาแฟคั่วเอง ขนมอบสดทุกเช้า", `href="/menu"`, "เมนูแนะนำ", "เอสเปรสโซ่", "<dd>55</dd>"} {
		if !strings.Contains(h, want) {
			t.Errorf("home lacks %q", want)
		}
	}
	if strings.Contains(h, "alert(1)") || strings.Contains(h, "ครัวซองต์เนยสด") {
		t.Error("an embed got in, or a non-featured item is on the home page")
	}
	// the type's own archive: every item, sorted by price
	m := page("/menu")
	if i, j := strings.Index(m, "เอสเปรสโซ่"), strings.Index(m, "ครัวซองต์เนยสด"); i < 0 || j < 0 || i > j {
		t.Errorf("menu archive order: espresso %d, croissant %d", i, j)
	}
	if s := page("/menu/espresso"); !strings.Contains(s, "<dt>ราคา (บาท)</dt><dd>55</dd>") {
		t.Error("item page without its price")
	}
	// the kit's plugin is offered and ticked
	_, rec := admin.do("GET", "/api/v1/setup/recommended", nil)
	var booking map[string]any
	for _, x := range rec["items"].([]any) {
		if m := x.(map[string]any); m["id"] == "booking" {
			booking = m
		}
	}
	if booking == nil || booking["checked"] != true {
		t.Fatalf("booking not offered: %v", rec["items"])
	}
	_, res := admin.do("POST", "/api/v1/setup/recommended/install", map[string]any{"ids": []any{"booking"}})
	if fmt.Sprint(res["results"]) != "[map[id:booking ok:true version:2.0.0]]" {
		t.Fatalf("install: %v", res)
	}
}
