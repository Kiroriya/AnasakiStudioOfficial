package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// CMS 2.1.0 (P2-4): manifest settings of type "secret" are stored sealed,
// never come back to the admin (secretsSet instead), keep their value when
// the form sends "", reach the plugin's code through site.settings and stay
// out of public templates; block props may not be secret.

func secPackage(extra string) map[string]string {
	m := map[string]any{"id": "sec-kit", "name": "Sec Kit", "version": "1.0.0", "runtime": "js", "permissions": []any{"code", "settings", "bodyEnd"},
		"bodyEnd": "end.html",
		"settings": []any{
			map[string]any{"key": "apiKey", "type": "secret", "label": "API key"},
			map[string]any{"key": "mode", "type": "text", "label": "Mode", "default": "test"},
		},
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"routes"}, "routes": []any{"/seckit"}}}
	if extra != "" {
		m["permissions"] = []any{"code", "settings", "bodyEnd", "blocks"}
		m["blocks"] = []any{map[string]any{"type": "card", "name": "Card", "template": "card.html", "props": []any{map[string]any{"key": "token", "type": "secret", "label": "T"}}}}
	}
	b, _ := json.Marshal(m)
	files := map[string]string{"manifest.json": string(b), "end.html": `<i id="end">{{settings.apiKey}}|{{settings.mode}}</i>`,
		"code/main.ts": `site.route('GET /seckit/key', { access: ['*'] }, () => site.settings('apiKey') + '|' + site.settings('mode'))`}
	if extra != "" {
		files["card.html"] = "<b>x</b>"
	}
	return files
}

func TestSecretSettings(t *testing.T) {
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	if code, b := uploadPkg(t, admin, "", secPackage("block")); code == 200 || !strings.Contains(strings.ToLower(jsonString(b)), "secret") {
		t.Fatalf("secret block prop accepted: %d %v", code, b)
	}
	mustUpload(t, admin, secPackage(""))
	activate(t, admin, "sec-kit")
	put := func(vals map[string]any) map[string]any {
		t.Helper()
		code, b := admin.do("PUT", "/api/v1/package-plugins/sec-kit/settings", map[string]any{"values": vals})
		if code != 200 {
			t.Fatalf("PUT settings: %d %v", code, b)
		}
		return b
	}
	b := put(map[string]any{"apiKey": "sk_live_42", "mode": "live"})
	if b["settings"].(map[string]any)["apiKey"] != "" || strs(b["secretsSet"]) != "apiKey" || strings.Contains(jsonString(b), "sk_live_42") {
		t.Fatalf("admin view: %v", b)
	}
	var raw string
	_ = e.st.DB.QueryRow(`SELECT settings FROM plugin_packages WHERE id = 'sec-kit'`).Scan(&raw)
	if strings.Contains(raw, "sk_live_42") || !strings.Contains(raw, "enc:v1:") {
		t.Fatalf("not sealed at rest: %s", raw)
	}
	key := func() string {
		_, s := e.c.public("GET", "/seckit/key")
		return strings.Trim(strings.TrimSpace(s), `"`)
	}
	if got := key(); got != "sk_live_42|live" {
		t.Fatalf("site.settings: %q", got)
	}
	// public template: the secret is not printed
	if _, h := e.c.public("GET", "/"); !strings.Contains(h, `<i id="end">|live</i>`) {
		t.Fatalf("bodyEnd template: %.300s", h[strings.Index(h, "</main>")+1:])
	}
	// "" keeps the value, __clear clears it
	put(map[string]any{"apiKey": "", "mode": "live"})
	if got := key(); got != "sk_live_42|live" {
		t.Fatalf("after an empty secret: %q", got)
	}
	b = put(map[string]any{"apiKey": "", "apiKey__clear": true, "mode": "live"})
	if got := key(); got != "|live" || b["secretsSet"] != nil {
		t.Fatalf("after clear: %q %v", got, b["secretsSet"])
	}
}
