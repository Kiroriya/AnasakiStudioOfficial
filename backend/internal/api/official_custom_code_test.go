package api

import (
	"strings"
	"testing"
)

// plugins-official/custom-code: raw HTML from the settings headCode / bodyCode in <head> and before
// </body> of every public page, unsanitised (scope head-scripts), like internal/plugins/custom-code.

func TestOfficialCustomCode(t *testing.T) {
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBPage(t, admin, "about", "About")
	offBInstall(t, admin, "custom-code")
	page := func(path string) string {
		t.Helper()
		rec := e.offBSend(offBReq{path: path, hdr: map[string]string{"User-Agent": offBHuman}})
		return rec.Body.String()
	}
	// nothing set: nothing added
	if html := page("/about"); strings.Contains(html, "gtag") {
		t.Fatal("unexpected code")
	}
	head := `<script async src="https://www.googletagmanager.com/gtag/js?id=G-1"></script><script>window.dataLayer=[];function gtag(){dataLayer.push(arguments)}</script>`
	body := `<div id="chat-widget" onclick="openChat()"></div><script>window.chat=1</script>`
	// Plugins → Custom code → Settings, longer than the 20,000 characters of Site code → Settings (built-in: 100,000)
	head += "<!--" + strings.Repeat("x", 30000) + "-->"
	offBPkgSettings(t, admin, "custom-code", map[string]any{"headCode": head, "bodyCode": body})
	for _, p := range []string{"/about", "/no-such-page"} { // designed pages and the 404 fallback
		html := page(p)
		hi, he := strings.Index(html, head), strings.Index(html, "</head>")
		bi, be := strings.Index(html, body), strings.Index(html, "</body>")
		if hi < 0 || he < hi {
			t.Fatalf("%s: head code missing or outside <head>:\n%s", p, html)
		}
		if bi < 0 || be < bi || bi < he {
			t.Fatalf("%s: body code missing or misplaced:\n%s", p, html)
		}
	}
	if out := offBExport(t, e, admin, "custom-code", "legacyImport", map[string]any{"kind": "rows", "table": "x", "rows": []any{}, "last": true}); out["imported"] != float64(0) {
		t.Fatalf("legacyImport: %v", out)
	}
	offBQuiet(t, admin, "custom-code")
}
