package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"strings"
	"testing"
)

// "Export / import site" is a core system tool since phase 7 (the former
// backup plugin): same zip format, plus the installed packages list (no
// code) and the Site Code draft.
func TestToolsExportImport(t *testing.T) {
	e := newPluginAPI(t)
	admin, _ := setupSite(t, e.c)
	_, ed := setupUser(t, e, admin, "editor")

	// two package plugins, one with a secret-looking setting
	mk := func(id string) map[string]string {
		m, _ := json.Marshal(map[string]any{"id": id, "name": "P " + id, "version": "1.2.3", "permissions": []any{"settings"},
			"settings": []any{map[string]any{"key": "color", "type": "text", "default": "red"},
				map[string]any{"key": "apiKey", "type": "text", "default": ""}}})
		return map[string]string{"manifest.json": string(m)}
	}
	mustUpload(t, admin, mk("colors"))
	mustUpload(t, admin, mk("shapes"))
	if code, b := admin.do("PUT", "/api/v1/package-plugins/colors/settings", map[string]any{"values": map[string]any{"color": "blue", "apiKey": "sk-123"}}); code != 200 {
		t.Fatalf("settings: %d %v", code, b)
	}
	activate(t, admin, "colors")
	if _, err := e.code.DB.PutFile("index.ts", "site.route('GET /hi', () => 'hi');", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.code.DB.PutFile("lib/util.ts", "export const x = 1;", 1); err != nil {
		t.Fatal(err)
	}

	for _, c := range []*client{ed, e.c} {
		if rec := c.send("GET", "/api/v1/tools/export", nil, nil); rec.Code != 403 && rec.Code != 401 {
			t.Fatalf("export without manage_options: %d", rec.Code)
		}
	}
	// the plugin route is gone
	if rec := admin.send("GET", "/api/v1/backup/export", nil, nil); rec.Code == 200 && strings.Contains(rec.Header().Get("Content-Type"), "zip") {
		t.Fatal("the old backup plugin route still answers")
	}

	rec := admin.send("GET", "/api/v1/tools/export", nil, nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/zip" || !strings.Contains(rec.Header().Get("Content-Disposition"), "anasaki-backup-") {
		t.Fatalf("export: %d %v", rec.Code, rec.Header())
	}
	zipData := rec.Body.Bytes()
	files := unzip(t, zipData)
	var d struct {
		Format   string            `json:"format"`
		Posts    []any             `json:"posts"`
		Packages []map[string]any  `json:"packages"`
		SiteCode map[string]string `json:"siteCode"`
	}
	if err := json.Unmarshal([]byte(files["data.json"]), &d); err != nil {
		t.Fatal(err)
	}
	if d.Format != "anasaki-backup" || len(d.Posts) == 0 || len(d.Packages) != 2 || d.SiteCode["index.ts"] == "" || d.SiteCode["lib/util.ts"] == "" {
		t.Fatalf("data.json = %+v", d)
	}
	for _, p := range d.Packages {
		s := p["settings"].(map[string]any)
		if _, leak := s["apiKey"]; leak {
			t.Fatalf("secret setting exported: %v", p)
		}
		if p["id"] == "colors" && (s["color"] != "blue" || p["active"] != true || p["version"] != "1.2.3") {
			t.Fatalf("colors = %v", p)
		}
	}
	if strings.Contains(files["data.json"], "sk-123") {
		t.Fatal("secret value in data.json")
	}

	// import into the same site after removing a package and a Site Code file
	if code, _ := admin.do("DELETE", "/api/v1/package-plugins/colors", nil); code != 204 {
		t.Fatalf("delete colors: %d", code)
	}
	if err := e.code.DB.DeleteFiles([]string{"lib/util.ts"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.code.DB.PutFile("index.ts", "// changed here", 1); err != nil {
		t.Fatal(err)
	}
	importZip := func(c *client, data []byte) (int, map[string]any) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "site.zip")
		_, _ = fw.Write(data)
		_ = mw.Close()
		rec := c.send("POST", "/api/v1/tools/import", &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, _ := importZip(ed, zipData); code != 403 {
		t.Fatalf("editor import: %d", code)
	}
	if code, b := importZip(admin, []byte("nope")); code != 400 {
		t.Fatalf("garbage import: %d %v", code, b)
	}
	code, b := importZip(admin, zipData)
	if code != 200 {
		t.Fatalf("import: %d %v", code, b)
	}
	if b["imported"].(map[string]any)["posts"].(float64) == 0 {
		t.Fatalf("imported = %v", b["imported"])
	}
	pk := map[string]map[string]any{}
	for _, x := range b["packages"].([]any) {
		m := x.(map[string]any)
		pk[m["id"].(string)] = m
	}
	if pk["colors"]["status"] != "missing" || pk["colors"]["settings"].(map[string]any)["color"] != "blue" || pk["colors"]["version"] != "1.2.3" ||
		pk["shapes"]["status"] != "installed" || pk["shapes"]["installedVersion"] != "1.2.3" {
		t.Fatalf("packages = %v", pk)
	}
	if pkgOf(t, admin, "colors") != nil {
		t.Fatal("import installed a package")
	}
	sc := b["siteCode"].(map[string]any)
	if strs(sc["added"]) != "lib/util.ts" || sc["skipped"].(float64) != 1 {
		t.Fatalf("siteCode = %v", sc)
	}
	draft, _ := e.code.DB.Draft()
	if draft["index.ts"] != "// changed here" || draft["lib/util.ts"] != "export const x = 1;" {
		t.Fatalf("draft = %v", draft)
	}
	// without edit_scripts no Site Code is imported (the content still is)
	if _, err := e.st.DB.Exec(`UPDATE roles SET caps = JSON_ARRAY_APPEND(caps, '$', 'manage_options') WHERE slug = 'editor'`); err != nil {
		t.Skip("roles table shape: ", err)
	}
	_ = e.api.reloadRoles()
	_ = e.code.DB.DeleteFiles([]string{"lib/util.ts"})
	code, b = importZip(ed, zipData)
	if code != 200 {
		t.Fatalf("editor+manage_options import: %d %v", code, b)
	}
	if sc := b["siteCode"].(map[string]any); len(sc["added"].([]any)) != 0 || sc["skipped"].(float64) != 2 {
		t.Fatalf("siteCode without edit_scripts = %v", sc)
	}
}
