package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Helpers of the official plugin tests (plugins-official/<id>, phase 7) for seo, redirects and
// maintenance. Prefixed offA so they never clash with the other official plugin tests.

// offAPlugin reads an official plugin folder as package files.
func offAPlugin(t *testing.T, id string) map[string]string {
	t.Helper()
	return dirFiles(t, filepath.Join("..", "..", "..", "plugins-official", id))
}

// offAInstall uploads official plugins from their folders and activates them, like an administrator.
func offAInstall(t *testing.T, admin *client, ids ...string) {
	t.Helper()
	for _, id := range ids {
		files := offAPlugin(t, id)
		var mf map[string]any
		if err := json.Unmarshal([]byte(files["manifest.json"]), &mf); err != nil {
			t.Fatalf("%s manifest: %v", id, err)
		}
		// official packages start at 2.0.0 (the built-ins were 1.x) and are released separately afterwards
		if v, _ := mf["version"].(string); mf["id"] != id || !regexp.MustCompile(`^([2-9]|[1-9][0-9]+)\.[0-9]+\.[0-9]+$`).MatchString(v) ||
			mf["author"] != "Anasaki Studio" || mf["runtime"] != "js" || mf["legacy"] != id {
			t.Fatalf("%s manifest fields: %v", id, mf)
		}
		if ex, _ := mf["code"].(map[string]any)["exports"].([]any); !strings.Contains(fmt.Sprint(ex), "legacyImport") {
			t.Fatalf("%s must export legacyImport: %v", id, ex)
		}
		for _, readme := range []string{"README.md"} {
			if s := files[readme]; !strings.Contains(s, "## ภาษาไทย") || !strings.Contains(s, "## English") {
				t.Fatalf("%s README must have Thai and English sections", id)
			}
		}
		mustUpload(t, admin, files)
		activate(t, admin, id)
	}
}

// offASettings sets plugin settings (keys of the manifest, same as the built-in plugin) through the
// Site Code settings API of the plugin unit — what site.settings(key) reads.
func offASettings(t *testing.T, admin *client, unit string, kv map[string]string) {
	t.Helper()
	var vals []any
	for k, v := range kv {
		vals = append(vals, map[string]any{"key": k, "secret": false, "value": v})
	}
	if code, b := admin.do("PUT", "/api/v1/code/settings", map[string]any{"unit": unit, "values": vals}); code != 200 {
		t.Fatalf("settings %s: %d %v", unit, code, b)
	}
}

// offAPkgSettings merges kv into the plugin's manifest settings (Plugins → plugin → Settings) — what
// site.settings(key) reads first.
func offAPkgSettings(t *testing.T, admin *client, id string, kv map[string]any) {
	t.Helper()
	cur, _ := pkgOf(t, admin, id)["settings"].(map[string]any)
	vals := map[string]any{}
	for k, v := range cur {
		vals[k] = v
	}
	for k, v := range kv {
		vals[k] = v
	}
	if code, b := admin.do("PUT", "/api/v1/package-plugins/"+id+"/settings", map[string]any{"values": vals}); code != 200 {
		t.Fatalf("package settings %s: %d %v", id, code, b)
	}
}

// offAAdminPage opens a site.adminPage through its ticket; the returned function runs
// anasaki.call(name, input) like the page's helper does.
func offAAdminPage(t *testing.T, e *pluginEnv, admin *client, id, query string) (string, func(name string, input any) (int, map[string]any)) {
	t.Helper()
	code, tk := admin.do("POST", "/api/v1/code/admin-pages/"+id+"/ticket", map[string]any{"query": query})
	if code != 200 {
		t.Fatalf("ticket %s: %d %v", id, code, tk)
	}
	u := tk["url"].(string)
	rec := e.c.send("GET", u, nil, nil)
	if rec.Code != 200 {
		t.Fatalf("admin page %s: %d %s", u, rec.Code, rec.Body.String())
	}
	tok := strings.Split(strings.TrimPrefix(u, "/ext-ui/code/"), "/")[0]
	call := func(name string, input any) (int, map[string]any) {
		b, _ := json.Marshal(map[string]any{"input": input})
		r := e.c.send("POST", "/ext-ui/code/"+tok+"/_a/"+name, bytes.NewReader(b), map[string]string{"Content-Type": "text/plain;charset=utf-8", "Origin": "null"})
		out := map[string]any{}
		_ = json.Unmarshal(r.Body.Bytes(), &out)
		return r.Code, out
	}
	return rec.Body.String(), call
}

// offAErr is the form error of an action answer ("" when none).
func offAErr(out map[string]any) string {
	errs, _ := out["errors"].(map[string]any)
	s, _ := errs["form"].(string)
	return s
}

// offACount counts the rows of a plugin table (physical name in the code database or fallback).
func offACount(t *testing.T, e *pluginEnv, name string) int {
	t.Helper()
	var n int
	q := "SELECT COUNT(*) FROM `" + e.code.Data().Database + "`.`" + e.code.Data().N.Prefix + name + "`"
	if err := e.st.DB.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

// offASiteCode publishes a Site Code route GET /offa-call?p=<plugin>&f=<export>&a=<JSON args> that calls a
// plugin export with plugins.call (as the core migration calls legacyImport), plus GET /offa-lib for the
// seo library (applyTemplate vectors).
func offASiteCode(t *testing.T, admin *client, extra string) {
	t.Helper()
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('POST /offa-call', { access: ['*'] }, ({ request }) => {
  const b = request.json();
  return { result: plugins.call(b.p, b.f, ...(b.a || [])) };
});
` + extra})
}

// offACall calls a plugin export through the Site Code route of offASiteCode, as the signed-in
// administrator (maintenance mode never blocks a CMS user).
func offACall(t *testing.T, admin *client, plugin, fn string, args ...any) any {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"p": plugin, "f": fn, "a": args})
	rec := admin.send("POST", "/offa-call", bytes.NewReader(b), map[string]string{"Content-Type": "application/json", "Accept": "application/json"})
	if rec.Code != 200 {
		t.Fatalf("%s.%s: %d %s", plugin, fn, rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out["result"]
}
