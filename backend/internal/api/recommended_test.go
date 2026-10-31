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
	fs.put(t, fakeOfficial("seo", false, nil))
	fs.put(t, fakeOfficial("forms", true, nil))
	fs.put(t, fakeOfficial("cookie-consent", false, nil))
	// a package in the store under an official id but not official
	fake := fakeOfficial("analytics", false, nil)
	fake["manifest.json"] = strings.Replace(fake["manifest.json"], `"author":"Anasaki Studio"`, `"author":"Mallory"`, 1)
	fs.put(t, fake)

	_, ed := setupUser(t, e, admin, "editor")
	if code, _ := ed.do("GET", "/api/v1/setup/recommended", nil); code != 403 {
		t.Fatalf("editor list: %d", code)
	}
	if code, _ := e.c.do("POST", "/api/v1/setup/recommended/install", map[string]any{"ids": []any{"seo"}}); code != 401 {
		t.Fatalf("anonymous install: %d", code)
	}

	code, b := admin.do("GET", "/api/v1/setup/recommended", nil)
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
	if strings.Join(ids, ",") != "seo,redirects,forms,cookie-consent,analytics,maintenance,custom-code" ||
		strings.Join(checked, ",") != "seo,redirects,forms,cookie-consent" {
		t.Fatalf("items = %v checked = %v", ids, checked)
	}
	seo := byID["seo"]
	if seo["available"] != true || seo["version"] != "2.0.0" || strs(seo["permissions"]) != "code" || strs(seo["scopes"]) != "db" ||
		seo["name"].(map[string]any)["en"] != "SEO" || seo["description"].(map[string]any)["th"] == "" || seo["installed"] != false {
		t.Fatalf("seo item = %v", seo)
	}
	if byID["redirects"]["available"] != false {
		t.Fatalf("redirects is not in the store: %v", byID["redirects"])
	}

	code, b = admin.do("POST", "/api/v1/setup/recommended/install", map[string]any{"ids": []any{"seo", "redirects", "forms", "analytics", "not-a-plugin"}})
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
	if pkgOf(t, admin, "analytics") != nil {
		t.Fatal("a non-official package was installed")
	}
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
	if code != 200 || b["storeAvailable"] != false || len(b["items"].([]any)) != 7 {
		t.Fatalf("list with store down: %d %v", code, b)
	}
	if it := b["items"].([]any)[2].(map[string]any); it["available"] != false || it["checked"] != true {
		t.Fatalf("item with store down = %v", it)
	}
	code, b = admin.do("POST", "/api/v1/setup/recommended/install", map[string]any{"ids": []any{"redirects", "cookie-consent"}})
	if code != 200 || b["storeAvailable"] != false {
		t.Fatalf("install with store down: %d %v", code, b)
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
