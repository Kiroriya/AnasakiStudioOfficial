package api

import (
	"strings"
	"testing"
)

// plugins-official/cookie-consent: the built-in banner (internal/plugins/cookie-consent) as the package's
// bodyEnd template — same settings keys and defaults, escaping, colours, position, policy link, optional
// decline button, localStorage + anasaki:consent script.

func TestOfficialCookieConsent(t *testing.T) {
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBPage(t, admin, "about", "About")
	offBInstall(t, admin, "cookie-consent")
	page := func() string {
		t.Helper()
		rec := e.offBSend(offBReq{path: "/about", hdr: map[string]string{"User-Agent": offBHuman}})
		if rec.Code != 200 {
			t.Fatalf("GET /about: %d", rec.Code)
		}
		return rec.Body.String()
	}
	html := page()
	offBContains(t, "defaults", html, `<div id="anasaki-cc" role="dialog" aria-live="polite" aria-label="Cookie consent" hidden data-pos="bottom" data-bg="#111827"`,
		"เว็บไซต์นี้ใช้คุกกี้เพื่อมอบประสบการณ์ที่ดีที่สุดให้กับคุณ", `data-consent="accepted"`, ">ยอมรับ</button>", ">ปฏิเสธ</button>",
		`data-cc-policy href=""`, "localStorage.setItem(k,v)", "anasaki:consent", "window.anasakiConsent=c", "#anasaki-cc[hidden]{display:none!important}")
	if i, j := strings.Index(html, `id="anasaki-cc"`), strings.Index(html, "</body>"); i < 0 || j < i {
		t.Fatal("banner must be before </body>")
	}

	// settings of the Plugins page (same keys as the built-in plugin)
	offBPkgSettings(t, admin, "cookie-consent", map[string]any{"message": `We use <b>cookies</b> & "stuff"`, "acceptText": "OK", "declineText": "",
		"policyUrl": "/privacy", "policyText": "Policy", "position": "top", "background": "#ff0000", "textColor": "white",
		"buttonColor": "rgb(1,2,3)", "buttonTextColor": "#000"})
	html = page()
	offBContains(t, "custom", html, `data-pos="top" data-bg="#ff0000" data-fg="white" data-btn="rgb(1,2,3)" data-btn-fg="#000"`,
		"We use &lt;b&gt;cookies&lt;/b&gt; &amp; &#34;stuff&#34;", `href="/privacy"`, ">Policy</a>", ">OK</button>",
		`data-consent="declined" style="border:0;border-radius:8px;padding:10px 18px;font:inherit;font-weight:600;cursor:pointer;background:transparent;color:inherit;box-shadow:inset 0 0 0 1px currentColor"></button>`)
	if strings.Contains(html, "<b>cookies") {
		t.Fatal("message not escaped")
	}
	// invalid values are refused like the built-in settings form
	for k, v := range map[string]any{"background": "red;position:absolute", "policyUrl": "javascript:alert(1)", "position": "middle"} {
		if code, b := admin.do("PUT", "/api/v1/package-plugins/cookie-consent/settings", map[string]any{"values": map[string]any{k: v}}); code != 400 {
			t.Fatalf("%s=%v accepted: %d %v", k, v, code, b)
		}
	}
	// legacy import: no tables
	if out := offBExport(t, e, admin, "cookie-consent", "legacyImport", map[string]any{"kind": "rows", "table": "x", "rows": []any{}, "last": true}); out["imported"] != float64(0) {
		t.Fatalf("legacyImport: %v", out)
	}
	// switched off → no banner
	if code, b := admin.do("POST", "/api/v1/package-plugins/cookie-consent/deactivate", nil); code != 200 {
		t.Fatalf("deactivate: %d %v", code, b)
	}
	if strings.Contains(page(), "anasaki-cc") {
		t.Fatal("banner shown while inactive")
	}
}
