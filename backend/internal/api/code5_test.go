package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Site Code phase 5: personal access tokens, tree/sync, Frontend code
// block save rules.

func newToken(t *testing.T, c *client, body map[string]any) (string, map[string]any) {
	t.Helper()
	code, b := c.do("POST", "/api/v1/auth/tokens", body)
	if code != 201 {
		t.Fatalf("create token: %d %v", code, b)
	}
	tok, _ := b["token"].(string)
	item, _ := b["item"].(map[string]any)
	return tok, item
}

func TestAPITokens(t *testing.T) {
	c, _, st := newCodeAPI(t, nil)
	_, _ = st.DB.Exec("DELETE FROM api_tokens")
	tokenFails.Lock()
	tokenFails.hits = map[string][]time.Time{}
	tokenFails.Unlock()
	admin, user := setupSite(t, c)
	admin.put("main.ts", "site.route('GET /hi', { access: ['*'] }, () => 'hi')\n", "")

	tok, item := newToken(t, admin, map[string]any{"name": "laptop", "scopes": []string{"code"}, "expiresInDays": 30})
	if !strings.HasPrefix(tok, "ast_") || len(tok) != 44 || !validTokenShape(tok) {
		t.Fatalf("token format: %q", tok)
	}
	if item["prefix"] != tok[:8] || item["name"] != "laptop" || item["expiresAt"] == nil || item["lastUsedAt"] != nil ||
		strs(item["scopes"]) != "code" {
		t.Fatalf("item: %v", item)
	}
	if _, has := item["token"]; has {
		t.Fatal("the item must not carry the secret")
	}
	code, list := admin.do("GET", "/api/v1/auth/tokens", nil)
	if items, _ := list["items"].([]any); code != 200 || len(items) != 1 {
		t.Fatalf("list: %d %v", code, list)
	}
	// bad input
	for _, b := range []map[string]any{{"name": ""}, {"name": "x", "scopes": []string{"admin"}}, {"name": "x", "expiresInDays": 7}} {
		if code, _ := admin.do("POST", "/api/v1/auth/tokens", b); code != 400 {
			t.Errorf("create %v: %d", b, code)
		}
	}

	// the token works for /code/** and GET /auth/me only
	cli := c.as(tok)
	if code, me := cli.do("GET", "/api/v1/auth/me", nil); code != 200 || me["email"] != "owner@x.io" {
		t.Fatalf("me with token: %d %v", code, me)
	}
	if code, tree := cli.do("GET", "/api/v1/code/tree", nil); code != 200 || len(tree["files"].([]any)) != 1 {
		t.Fatalf("tree with token: %d %v", code, tree)
	}
	if code := cli.send("GET", "/api/v1/code/types", nil, nil).Code; code != 200 {
		t.Fatalf("types with token: %d", code)
	}
	for _, rq := range [][2]string{{"GET", "/api/v1/posts"}, {"PUT", "/api/v1/auth/me"}, {"POST", "/api/v1/auth/tokens"},
		{"GET", "/api/v1/auth/tokens"}, {"GET", "/api/v1/users"}, {"GET", "/api/v1/settings"}} {
		if code, b := cli.do(rq[0], rq[1], map[string]any{"name": "x"}); code != 403 || b["error"] != "token scope" {
			t.Errorf("token %s %s: %d %v", rq[0], rq[1], code, b)
		}
	}
	_, list = admin.do("GET", "/api/v1/auth/tokens", nil)
	if it := list["items"].([]any)[0].(map[string]any); it["lastUsedAt"] == nil {
		t.Errorf("last use not recorded: %v", it)
	}
	// role capabilities still apply to a token
	editor := user("Ed", "editor")
	edTok, edItem := newToken(t, editor, map[string]any{"name": "ed"})
	if code, _ := c.as(edTok).do("GET", "/api/v1/code/tree", nil); code != 403 {
		t.Fatalf("editor token without view_scripts: %d", code)
	}
	// someone else's token: hidden from a non-manager, revocable by an admin
	if code, _ := editor.do("DELETE", "/api/v1/auth/tokens/"+id(item), nil); code != 404 {
		t.Fatalf("editor revoked the owner's token: %d", code)
	}
	if code, _ := editor.do("GET", "/api/v1/auth/tokens?user="+jsonNum(item["userId"].(float64)), nil); code != 403 {
		t.Fatalf("editor listed the owner's tokens: %d", code)
	}
	if code, l := admin.do("GET", "/api/v1/auth/tokens?user="+jsonNum(edItem["userId"].(float64)), nil); code != 200 || len(l["items"].([]any)) != 1 {
		t.Fatalf("admin ?user=: %d %v", code, l)
	}
	if code, _ := admin.do("DELETE", "/api/v1/auth/tokens/"+id(edItem), nil); code != 204 {
		t.Fatalf("admin revoke: %d", code)
	}
	if code, _ := c.as(edTok).do("GET", "/api/v1/auth/me", nil); code != 401 {
		t.Fatalf("revoked token: %d", code)
	}
	// expired → 401
	expTok, expItem := newToken(t, admin, map[string]any{"name": "old", "expiresInDays": 90})
	if _, err := st.DB.Exec("UPDATE api_tokens SET expires_at = ? WHERE id = ?", time.Now().UTC().Add(-time.Hour), int64(expItem["id"].(float64))); err != nil {
		t.Fatal(err)
	}
	if code, _ := c.as(expTok).do("GET", "/api/v1/auth/me", nil); code != 401 {
		t.Fatalf("expired token: %d", code)
	}
	// unknown / malformed → 401
	for _, bad := range []string{"ast_nope", "ast_" + strings.Repeat("A", 40)} {
		if code, _ := c.as(bad).do("GET", "/api/v1/auth/me", nil); code != 401 {
			t.Fatalf("bad token %q: %d", bad, code)
		}
	}
	// a password change revokes the user's tokens
	if code, b := admin.do("PUT", "/api/v1/auth/me", map[string]any{"password": "newsecret1", "currentPassword": "secret123"}); code != 200 {
		t.Fatalf("change password: %d %v", code, b)
	}
	if code, _ := cli.do("GET", "/api/v1/code/tree", nil); code != 401 {
		t.Fatalf("token after password change: %d", code)
	}
	var n int
	_ = st.DB.QueryRow("SELECT COUNT(*) FROM api_tokens").Scan(&n)
	if n != 0 {
		t.Fatalf("%d tokens left after password change / revocations", n)
	}
	// failures are rate limited per address
	for i := 0; i < 60; i++ {
		c.as("ast_"+strings.Repeat("b", 40)).do("GET", "/api/v1/auth/me", nil)
	}
	if code, _ := c.as("ast_x").do("GET", "/api/v1/auth/me", nil); code != 429 {
		t.Fatalf("token failures not limited: %d", code)
	}
	tokenFails.Lock()
	tokenFails.hits = map[string][]time.Time{}
	tokenFails.Unlock()
}

func TestTokenScopeAllows(t *testing.T) {
	for _, tc := range []struct {
		m, p string
		ok   bool
	}{
		{"GET", "/api/v1/code/tree", true}, {"POST", "/api/v1/code/sync", true}, {"GET", "/api/v1/code", true},
		{"GET", "/api/v1/auth/me", true}, {"PUT", "/api/v1/auth/me", false}, {"GET", "/api/v1/codex", false},
		{"GET", "/api/v1/code/../users", false}, {"GET", "/api/v1/posts", false},
	} {
		if got := tokenScopeAllows([]string{"code"}, tc.m, tc.p); got != tc.ok {
			t.Errorf("%s %s = %v", tc.m, tc.p, got)
		}
	}
	if tokenScopeAllows(nil, "GET", "/api/v1/code/tree") {
		t.Error("no scope allowed something")
	}
}

func TestCodeTreeSync(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	main := "site.route('GET /a', { access: ['*'] }, () => 'a')\n"
	admin.put("main.ts", main, "")
	code, tree := admin.do("GET", "/api/v1/code/tree", nil)
	files := tree["files"].([]any)
	f0 := files[0].(map[string]any)
	if code != 200 || len(files) != 1 || f0["path"] != "main.ts" || f0["sha256"] != sitecode.ContentHash(main) ||
		f0["size"].(float64) != float64(len(main)) || tree["draftChanged"] != true || tree["liveVersion"] != nil {
		t.Fatalf("tree: %d %v", code, tree)
	}
	h := sitecode.ContentHash(main)
	empty := ""
	// put a new file (base "" = must not exist) + change main with its base
	code, b := admin.do("POST", "/api/v1/code/sync", map[string]any{"put": []any{
		map[string]any{"path": "lib/x.ts", "content": "export const x = 1\n", "baseSha256": empty},
		map[string]any{"path": "main.ts", "content": "import { x } from './lib/x'\n" + main, "baseSha256": h},
		map[string]any{"path": "anasaki.d.ts", "content": "// generated"},
	}})
	if code != 200 || b["ok"] != true || len(b["files"].([]any)) != 2 || strs(b["written"]) != "lib/x.ts,main.ts" ||
		strs(b["skipped"]) != "anasaki.d.ts" || len(b["problems"].([]any)) != 0 {
		t.Fatalf("sync: %d %v", code, b)
	}
	// stale base → 409, nothing applied
	code, b = admin.do("POST", "/api/v1/code/sync", map[string]any{
		"put":    []any{map[string]any{"path": "new.ts", "content": "x"}},
		"delete": []any{map[string]any{"path": "main.ts", "baseSha256": h}},
	})
	if code != 409 || !strings.Contains(strs(b["conflicts"]), "main.ts") {
		t.Fatalf("conflict: %d %v", code, b)
	}
	if _, tr := admin.do("GET", "/api/v1/code/tree", nil); len(tr["files"].([]any)) != 2 {
		t.Fatalf("conflicting sync applied something: %v", tr)
	}
	// a new file that already exists on the server conflicts too
	if code, b := admin.do("POST", "/api/v1/code/sync", map[string]any{"put": []any{
		map[string]any{"path": "lib/x.ts", "content": "y", "baseSha256": ""}}}); code != 409 {
		t.Fatalf("new-file conflict: %d %v", code, b)
	}
	// dry run reports problems without writing
	code, b = admin.do("POST", "/api/v1/code/sync", map[string]any{"dryRun": true, "put": []any{
		map[string]any{"path": "main.ts", "content": "site.route('GET /a', () => {\n"}}})
	if code != 200 || b["ok"] != false || len(b["problems"].([]any)) == 0 || b["dryRun"] != true {
		t.Fatalf("dry run: %d %v", code, b)
	}
	p0 := b["problems"].([]any)[0].(map[string]any)
	if p0["file"] != "main.ts" || p0["line"] == nil {
		t.Fatalf("problem shape: %v", p0)
	}
	if _, tr := admin.do("GET", "/api/v1/code/tree", nil); strings.Contains(strs(tr["files"]), sitecode.ContentHash("site.route('GET /a', () => {\n")) {
		t.Fatal("dry run wrote")
	}
	// delete + validation
	if code, b := admin.do("POST", "/api/v1/code/sync", map[string]any{"delete": []any{map[string]any{"path": "lib/x.ts"}},
		"put": []any{map[string]any{"path": "main.ts", "content": main}}}); code != 200 || strs(b["deleted"]) != "lib/x.ts" {
		t.Fatalf("delete: %d %v", code, b)
	}
	for _, bad := range []map[string]any{
		{"put": []any{map[string]any{"path": "../x.ts", "content": "x"}}},
		{"put": []any{map[string]any{"path": "x.exe", "content": "x"}}},
		{"put": []any{map[string]any{"path": "x.ts"}}},
		{"put": []any{map[string]any{"path": "big.ts", "content": strings.Repeat("a", sitecode.MaxFileBytes+1)}}},
		{"put": []any{map[string]any{"path": "a.ts", "content": "x"}}, "delete": []any{map[string]any{"path": "a.ts"}}},
	} {
		if code, b := admin.do("POST", "/api/v1/code/sync", bad); code != 400 {
			t.Errorf("sync %v: %d %v", bad, code, b)
		}
	}
}

func TestCodeBlockSaveRules(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	editor := user("Ed", "editor") // edit_pages + unfiltered_html, no edit_scripts
	block := func(js string) map[string]any {
		return map[string]any{"id": "root", "type": "frame", "children": []any{
			map[string]any{"id": "cb1", "type": "codeblock", "props": map[string]any{"html": "<p>x</p>", "css": "p{color:red}", "js": js}}}}
	}
	if code, b := editor.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "E", "design": block("a()")}); code != 403 ||
		strs(b["blocks"]) != "cb1" || !strings.Contains(b["error"].(string), "edit_scripts") {
		t.Fatalf("editor added a code block: %d %v", code, b)
	}
	// an empty block is harmless
	if code, b := editor.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "E2", "design": map[string]any{"id": "root", "type": "frame",
		"children": []any{map[string]any{"id": "e", "type": "codeblock", "props": map[string]any{}}}}}); code != 201 {
		t.Fatalf("empty block: %d %v", code, b)
	}
	code, page := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "P", "design": block("a()")})
	if code != 201 {
		t.Fatalf("admin save: %d %v", code, page)
	}
	pid := id(page)
	// the editor changes something else: the admin's block stays byte for byte
	d := block("a()")
	d["name"] = "renamed"
	if code, b := editor.do("PUT", "/api/v1/posts/"+pid, map[string]any{"title": "P2", "design": d}); code != 200 {
		t.Fatalf("editor kept block: %d %v", code, b)
	}
	_, got := admin.do("GET", "/api/v1/posts/"+pid, nil)
	raw, _ := json.Marshal(got["design"])
	if !strings.Contains(string(raw), `"js":"a()"`) || !strings.Contains(string(raw), `"css":"p{color:red}"`) {
		t.Fatalf("block not kept: %s", raw)
	}
	// changing the code → 403
	if code, _ := editor.do("PUT", "/api/v1/posts/"+pid, map[string]any{"design": block("evil()")}); code != 403 {
		t.Fatalf("editor changed the block: %d", code)
	}
	// @import is refused (even for an admin)
	bad := block("a()")
	bad["children"].([]any)[0].(map[string]any)["props"].(map[string]any)["css"] = "@import url(x.css); p{}"
	if code, _ := admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"design": bad}); code != 400 {
		t.Fatalf("@import: %d", code)
	}
	// components follow the same rules
	if code, _ := editor.do("POST", "/api/v1/components", map[string]any{"name": "C", "node": block("c()")}); code != 403 {
		t.Fatalf("editor component with code: %d", code)
	}
	code, comp := admin.do("POST", "/api/v1/components", map[string]any{"name": "C", "node": block("c()")})
	if code != 200 && code != 201 {
		t.Fatalf("admin component: %d %v", code, comp)
	}
	if code, b := editor.do("PUT", "/api/v1/components/"+id(comp), map[string]any{"name": "C2", "node": block("c()")}); code != 200 {
		t.Fatalf("editor renamed component: %d %v", code, b)
	}
	if code, _ := editor.do("PUT", "/api/v1/components/"+id(comp), map[string]any{"name": "C2", "node": block("d()")}); code != 403 {
		t.Fatalf("editor changed component code: %d", code)
	}
	// a preview by the editor runs only the stored block
	code, html := editor.raw("POST", "/api/v1/preview", map[string]any{"design": block("evil()"), "postId": page["id"], "title": "P"})
	if code != 200 || strings.Contains(html, "evil()") {
		t.Fatalf("editor preview ran new code: %d", code)
	}
	code, html = editor.raw("POST", "/api/v1/preview", map[string]any{"design": block("a()"), "postId": page["id"], "title": "P"})
	if code != 200 || !strings.Contains(html, "a()") || !strings.Contains(html, `data-rea-code-block="cb1"`) || !strings.Contains(html, "window.anasaki") {
		t.Fatalf("stored block missing from preview: %d", code)
	}
}
