package api

import (
	"strings"
	"testing"
)

// Setup wizard step "Recommended plugins" (phase 7) against a fake Store.
func TestSetupRecommendedPlugins(t *testing.T) {
	fs := newFakeStore(t)
	e := newPluginAPI(t)
	admin, _ := setupSite(t, e.c)

	// nothing recommended yet: an empty list, the wizard just finishes
	fs.put(t, fakeOfficial("seo", false, nil))
	code, b := admin.do("GET", "/api/v1/setup/recommended", nil)
	if code != 200 || b["storeAvailable"] != true || len(b["items"].([]any)) != 0 {
		t.Fatalf("list without recommendations: %d %v", code, b)
	}

	fs.put(t, fakeOfficial("forms", true, nil))
	fs.put(t, fakeOfficial("cookie-consent", false, nil))
	// a package the Store calls official, but not by the official author
	fake := fakeOfficial("analytics", false, nil)
	fake["manifest.json"] = strings.Replace(fake["manifest.json"], `"author":"Anasaki Studio"`, `"author":"Mallory"`, 1)
	fs.put(t, fake)
	// a recommended package that is not official: never offered nor installed
	fs.put(t, fakeOfficial("third-party", false, nil))
	// any id works (the core knows no plugin), sorted by order then name
	fs.recommend("seo", true, 1, nil)
	fs.recommend("forms", true, 2, nil)
	fs.recommend("redirects", true, 2, map[string]any{"name": "Redirects"}) // listed, no package
	fs.recommend("cookie-consent", false, 5, map[string]any{"name": map[string]any{"th": "คุกกี้", "en": "Cookie consent"}})
	fs.recommend("analytics", false, 5, nil)
	fs.recommend("third-party", true, 0, map[string]any{"official": false})

	_, ed := setupUser(t, e, admin, "editor")
	if code, _ := ed.do("GET", "/api/v1/setup/recommended", nil); code != 403 {
		t.Fatalf("editor list: %d", code)
	}
	if code, _ := e.c.do("POST", "/api/v1/setup/recommended/install", map[string]any{"ids": []any{"seo"}}); code != 401 {
		t.Fatalf("anonymous install: %d", code)
	}

	code, b = admin.do("GET", "/api/v1/setup/recommended", nil)
	if code != 200 || b["storeAvailable"] != true {
		t.Fatalf("list: %d %v", code, b)
	}
	items := b["items"].([]any)
	var ids, checked []string
	byID := map[string]map[string]any{}
	for _, x := range items {
		m := x.(map[string]any)
		ids = append(ids, m["id"].(string))
		if m["checked"] == true {
			checked = append(checked, m["id"].(string))
		}
		byID[m["id"].(string)] = m
	}
	if strings.Join(ids, ",") != "seo,forms,redirects,cookie-consent,analytics" ||
		strings.Join(checked, ",") != "seo,forms,redirects" {
		t.Fatalf("items = %v checked = %v", ids, checked)
	}
	seo := byID["seo"]
	// permissions come from the Store detail (summaries have none)
	if seo["available"] != true || seo["version"] != "2.0.0" || strs(seo["permissions"]) != "code" || strs(seo["scopes"]) != "db" ||
		seo["name"] != "Official seo" || seo["installed"] != false || seo["order"] != 1.0 {
		t.Fatalf("seo item = %v", seo)
	}
	if n, ok := byID["cookie-consent"]["name"].(map[string]any); !ok || n["th"] != "คุกกี้" {
		t.Fatalf("an {th,en} name is kept: %v", byID["cookie-consent"])
	}

	code, b = admin.do("POST", "/api/v1/setup/recommended/install", map[string]any{"ids": []any{"seo", "redirects", "forms", "analytics", "not-a-plugin", "third-party"}})
	if code != 200 || b["storeAvailable"] != true {
		t.Fatalf("install: %d %v", code, b)
	}
	res := map[string]map[string]any{}
	for _, x := range b["results"].([]any) {
		m := x.(map[string]any)
		res[m["id"].(string)] = m
	}
	if len(res) != 4 || res["seo"]["ok"] != true || res["seo"]["version"] != "2.0.0" || res["forms"]["ok"] != true ||
		res["redirects"]["ok"] != false || !strings.Contains(res["redirects"]["error"].(string), "not in the Store") ||
		res["analytics"]["ok"] != false || !strings.Contains(res["analytics"]["error"].(string), "not an official") {
		t.Fatalf("results = %v", res)
	}
	for _, id := range []string{"seo", "forms"} {
		if p := pkgOf(t, admin, id); p == nil || p["active"] != true || p["source"] != "store" {
			t.Fatalf("%s package = %v", id, p)
		}
	}
	if pkgOf(t, admin, "analytics") != nil || pkgOf(t, admin, "third-party") != nil {
		t.Fatal("a non-official package was installed")
	}
	// the Store stops recommending a plugin: it cannot be installed
	// through the wizard any more
	fs.put(t, fakeOfficial("cookie-consent", false, nil)) // drops the recommended flag
	if _, b := admin.do("POST", "/api/v1/setup/recommended/install", map[string]any{"ids": []any{"cookie-consent"}}); len(b["results"].([]any)) != 0 ||
		pkgOf(t, admin, "cookie-consent") != nil {
		t.Fatalf("a plugin that is not recommended was installed: %v", b)
	}
	fs.recommend("cookie-consent", false, 5, nil)
	// installing again is harmless
	if code, b := admin.do("POST", "/api/v1/setup/recommended/install", map[string]any{"ids": []any{"seo"}}); code != 200 ||
		b["results"].([]any)[0].(map[string]any)["ok"] != true {
		t.Fatalf("reinstall: %d %v", code, b)
	}
	_, b = admin.do("GET", "/api/v1/setup/recommended", nil)
	if b["items"].([]any)[0].(map[string]any)["installed"] != true {
		t.Fatalf("seo should be installed: %v", b["items"].([]any)[0])
	}

	// store unreachable: setup is not blocked, the wizard says "install later"
	fs.down.Store(true)
	code, b = admin.do("GET", "/api/v1/setup/recommended", nil)
	if code != 200 || b["storeAvailable"] != false || len(b["items"].([]any)) != 0 {
		t.Fatalf("list with store down: %d %v", code, b)
	}
	code, b = admin.do("POST", "/api/v1/setup/recommended/install", map[string]any{"ids": []any{"redirects", "cookie-consent"}})
	if code != 200 || b["storeAvailable"] != false {
		t.Fatalf("install with store down: %d %v", code, b)
	}
	if len(b["results"].([]any)) != 2 {
		t.Fatalf("results with store down = %v", b)
	}
	for _, x := range b["results"].([]any) {
		if m := x.(map[string]any); m["ok"] != false || m["error"] != "store unavailable" {
			t.Fatalf("result with store down = %v", m)
		}
	}
	if pkgOf(t, admin, "cookie-consent") != nil {
		t.Fatal("installed with the store down")
	}
	// the site itself works (setup already finished)
	if code, _ := admin.do("GET", "/api/v1/auth/me", nil); code != 200 {
		t.Fatalf("me: %d", code)
	}
}
