package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestOfficialDocsServe is a manual helper for looking at the docs plugin in a browser (screenshots): it installs
// plugins-official/docs, imports a bundle and serves the site until DOCS_SERVE_SECONDS pass. Skipped unless
// DOCS_SERVE_SECONDS is set.
//
//	DOCS_SERVE_SECONDS=300 DOCS_SERVE_BUNDLE=/path/docs-bundle.json DOCS_SERVE_URL_FILE=/tmp/url.txt \
//	  go test -run OfficialDocsServe ./internal/api/
func TestOfficialDocsServe(t *testing.T) {
	secs, _ := strconv.Atoi(os.Getenv("DOCS_SERVE_SECONDS"))
	if secs <= 0 {
		t.Skip("DOCS_SERVE_SECONDS not set (manual screenshot helper)")
	}
	offDocsCore(t)
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	admin.do("PUT", "/api/v1/settings", map[string]any{"language": "th", "timezone": "Asia/Bangkok", "site_title": "Anasaki"})
	offBInstall(t, admin, "docs")

	// a plain site header and footer, like a real site's layout
	comp := func(name, role, html string) {
		_, c := admin.do("POST", "/api/v1/components", map[string]any{"name": name, "node": map[string]any{
			"id": "root", "type": "frame", "width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"},
			"layout": map[string]any{"mode": "vertical"}, "children": []any{map[string]any{"id": "h", "type": "embed", "width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"},
				"props": map[string]any{"html": html}}}}})
		if code, b := admin.do("POST", "/api/v1/components/"+id(c)+"/site-role", map[string]any{"role": role}); code != 200 {
			t.Fatalf("%s: %d %v", role, code, b)
		}
	}
	comp("Header", "header", `<header style="display:flex;align-items:center;justify-content:space-between;height:64px;padding:0 32px;border-bottom:1px solid #e5e7eb;background:#fff;font-family:system-ui,'Noto Sans Thai',sans-serif">`+
		`<a href="/" style="font-weight:800;font-size:18px;color:#111827;text-decoration:none">◆ Anasaki</a>`+
		`<nav style="display:flex;gap:24px;font-size:15px"><a href="/" style="color:#4b5563;text-decoration:none">หน้าแรก</a><a href="/docs" style="color:#4f46e5;font-weight:600;text-decoration:none">เอกสาร</a><a href="/blog" style="color:#4b5563;text-decoration:none">บล็อก</a></nav></header>`)
	comp("Footer", "footer", `<footer style="padding:32px;background:#0f172a;color:#94a3b8;font:14px system-ui,'Noto Sans Thai',sans-serif;text-align:center">© 2026 Anasaki Studio</footer>`)

	if p := os.Getenv("DOCS_SERVE_BUNDLE"); p != "" {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var bundle struct {
			Products        []any  `json:"products"`
			HubIntro        string `json:"hubIntro"`
			Docs            []any  `json:"docs"`
			LegacyRedirects []any  `json:"legacyRedirects"`
		}
		if err := json.Unmarshal(raw, &bundle); err != nil {
			t.Fatal(err)
		}
		_, call := adminPage(t, e.toSample(), admin, "docs:docs", "")
		if len(bundle.Products) > 0 {
			call("docs:saveSettings", map[string]any{"products": bundle.Products, "hubIntro": bundle.HubIntro})
		}
		for i := 0; i < len(bundle.Docs); i += 20 {
			j := min(i+20, len(bundle.Docs))
			code, out := call("docs:import", map[string]any{"docs": bundle.Docs[i:j]})
			t.Logf("import %d-%d: %d %v", i, j, code, out["data"])
		}
		if len(bundle.LegacyRedirects) > 0 {
			call("docs:saveSettings", map[string]any{"legacyRedirects": bundle.LegacyRedirects})
		}
	}
	srv := httptest.NewServer(e.c.h)
	defer srv.Close()
	t.Logf("serving %s for %ds", srv.URL, secs)
	if f := os.Getenv("DOCS_SERVE_URL_FILE"); f != "" {
		// line 2: the admin page "docs:docs" (a ticket URL, opened without signing in)
		_, tk := admin.do("POST", "/api/v1/code/admin-pages/docs:docs/ticket", map[string]any{"query": ""})
		ticket, _ := tk["url"].(string)
		_ = os.WriteFile(f, []byte(srv.URL+"\n"+ticket), 0o644)
	}
	time.Sleep(time.Duration(secs) * time.Second)
}
