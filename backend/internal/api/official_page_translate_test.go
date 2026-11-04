package api

import (
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/page-translate: the floating button (head assets + bodyEnd template) on public pages; the
// Google API key reaches the page only in API mode, as <meta name="ptw-api-key"> in <head>; the admin page
// "แปลภาษา (ตั้งค่า)" validates and saves the settings and the key.

func TestOfficialPageTranslate(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.0.6" // minCore 2.0.0 (tests run as a dev build); 2.0.6+ for the plugin's capabilities
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBPage(t, admin, "about", "About")
	offBInstall(t, admin, "page-translate")
	page := func() string {
		t.Helper()
		rec := e.offBSend(offBReq{path: "/about", hdr: map[string]string{"User-Agent": offBHuman}})
		if rec.Code != 200 {
			t.Fatalf("GET /about: %d", rec.Code)
		}
		return rec.Body.String()
	}
	html := page()
	offBContains(t, "defaults", html, `class="ptw-root notranslate" translate="no"`, `data-mode="free"`, `data-site-lang="th"`,
		`--ptw-color: #4f46e5`, `aria-expanded="false"`, `data-lang="__restore"`, "translate.js", "translate.css")
	if i, j := strings.Index(html, `class="ptw-root`), strings.Index(html, "</body>"); i < 0 || j < i {
		t.Fatal("button must be before </body>")
	}
	if strings.Contains(html, "ptw-api-key") || strings.Contains(html, "data-api-key") {
		t.Fatal("no key set, but a key element is in the page")
	}

	// a key kept while the free mode is on stays out of the page
	const key = "AIzaTestKey_123-abc"
	offBPkgSettings(t, admin, "page-translate", map[string]any{"mode": "free", "apiKey": key, "siteLang": "en", "buttonColor": "#112233"})
	html = page()
	if strings.Contains(html, key) {
		t.Fatal("free mode: the API key is in the page")
	}
	offBContains(t, "free", html, `data-site-lang="en"`, `--ptw-color: #112233`)

	// API mode: the key as a meta tag in <head>
	offBPkgSettings(t, admin, "page-translate", map[string]any{"mode": "api", "apiKey": key, "siteLang": "en", "buttonColor": "#112233"})
	html = page()
	meta := `<meta name="ptw-api-key" content="` + key + `">`
	if i, j := strings.Index(html, meta), strings.Index(html, "</head>"); i < 0 || j < i {
		t.Fatalf("api mode: %s missing from <head>", meta)
	}
	if strings.Count(html, key) != 1 {
		t.Fatal("api mode: the key must appear once (the meta tag only)")
	}

	// admin page: masked key, validated actions
	body, call := offAAdminPage(t, e.pluginEnv, admin, "page-translate:settings", "")
	offBContains(t, "admin page", body, `id="ptw-general"`, `id="ptw-key-form"`, "••••••••-abc", `value="api" selected`)
	if strings.Contains(body, key) {
		t.Fatal("admin page shows the whole key")
	}
	for _, in := range []map[string]any{
		{"mode": "paid", "siteLang": "th", "buttonColor": "#000000"},
		{"mode": "free", "siteLang": "fr", "buttonColor": "#000000"},
		{"mode": "free", "siteLang": "th", "buttonColor": "red;x:1"},
	} {
		if _, out := call("page-translate:save", in); offAErr(out) == "" {
			t.Fatalf("save %v accepted: %v", in, out)
		}
	}
	if _, out := call("page-translate:saveApiKey", map[string]any{"apiKey": `bad"key<`}); offAErr(out) == "" {
		t.Fatalf("bad key accepted: %v", out)
	}
	if code, out := call("page-translate:save", map[string]any{"mode": "api", "siteLang": "ja", "buttonColor": "#abcdef"}); code != 200 || offAErr(out) != "" {
		t.Fatalf("save: %v", out)
	}
	if code, out := call("page-translate:saveApiKey", map[string]any{"apiKey": ""}); code != 200 || offAErr(out) != "" {
		t.Fatalf("clear key: %v", out)
	}
	html = page()
	offBContains(t, "saved", html, `data-mode="api"`, `data-site-lang="ja"`, `--ptw-color: #abcdef`)
	if strings.Contains(html, "ptw-api-key") {
		t.Fatal("key cleared, but the meta tag is still there")
	}

	// switched off → no button
	if code, b := admin.do("POST", "/api/v1/package-plugins/page-translate/deactivate", nil); code != 200 {
		t.Fatalf("deactivate: %d %v", code, b)
	}
	if strings.Contains(page(), "ptw-root") {
		t.Fatal("button shown while inactive")
	}
}
