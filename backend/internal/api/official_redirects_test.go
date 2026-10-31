package api

import (
	"fmt"
	"strings"
	"testing"
)

// The official "redirects" JS plugin (plugins-official/redirects) — the port of the built-in Go plugin:
// rule validation and loop check (TestValidate / TestFindLoop), the middleware cases (TestMiddleware),
// hit counters and the 404 log via site.afterResponse, the admin page and legacyImport.
func TestOfficialRedirects(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	t.Cleanup(e.code.WaitAfterResponse)
	admin, user := setupSite(t, c)
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "About", "slug": "about", "status": "publish", "content": "<p>x</p>"}); code != 201 {
		t.Fatalf("page: %d %v", code, b)
	}
	offAInstall(t, admin, "redirects")
	offASiteCode(t, admin, "")

	if code, _ := user("Ed", "editor").do("POST", "/api/v1/code/admin-pages/redirects:redirects/ticket", map[string]any{}); code != 404 {
		t.Fatalf("editor ticket: %d", code)
	}
	html, call := offAAdminPage(t, e, admin, "redirects:redirects", "")
	if !strings.Contains(html, `data-rd="form"`) || !strings.Contains(html, "No redirects yet") || !strings.Contains(html, "No broken links") ||
		!strings.Contains(html, "301 · Permanent") || !strings.Contains(html, "308 · Permanent (keep method)") {
		t.Fatalf("empty admin page:\n%s", html)
	}
	save := func(in map[string]any) (int, map[string]any) { return call("redirects:save", in) }

	// TestValidate: invalid rules (English messages: the owner's admin language)
	for _, r := range []map[string]any{
		{"from": "old", "to": "/x"}, {"from": "/", "to": "/x"}, {"from": "/a", "to": "javascript:alert(1)"},
		{"from": "/api/v1/posts", "to": "/x"}, {"from": "/a", "to": "/a/"}, {"from": "/a", "to": "/b", "code": 303},
		{"from": "//evil", "to": "/x"}, {"from": "/a", "to": ""}, {"from": "/uploads", "to": "/x"},
	} {
		if code, out := save(r); out["errors"] == nil || (code == 200 && offAErr(out) == "") {
			t.Errorf("rule %v should be invalid: %d %v", r, code, out)
		}
	}
	if _, out := save(map[string]any{"from": "/a", "to": "/b", "code": 303}); offAErr(out) != "The redirect type must be 301, 302, 307 or 308" {
		t.Fatalf("message: %v", out)
	}
	// a valid rule is normalised (spaces, trailing slash, query of the source dropped), default 301
	code, out := save(map[string]any{"from": " /old/?x=1 ", "to": "/new"})
	if code != 200 || offAErr(out) != "" || out["data"].(map[string]any)["from"] != "/old" || out["data"].(map[string]any)["code"] != float64(301) {
		t.Fatalf("valid rule: %d %v", code, out)
	}
	for _, r := range []map[string]any{
		{"from": "/promo", "to": "https://shop.example.com/sale#top", "code": 302},
		{"from": "/old/deep", "to": "/x", "code": 308},
		{"from": "/Case", "to": "/upper"}, // case-sensitive: /case is a different path
		{"from": "/case", "to": "/lower"},
	} {
		if code, out := save(r); code != 200 || offAErr(out) != "" {
			t.Fatalf("rule %v: %d %v", r, code, out)
		}
	}
	if _, out := save(map[string]any{"from": "/old/", "to": "/elsewhere"}); offAErr(out) != "A redirect for this path already exists" {
		t.Fatalf("duplicate: %v", out)
	}
	// TestFindLoop through the API: A→B, B→C?x=1, then C→A is refused with the chain
	save(map[string]any{"from": "/la", "to": "/lb"})
	_, lb := save(map[string]any{"from": "/lb", "to": "/lc?x=1"})
	if _, out := save(map[string]any{"from": "/lc", "to": "/la"}); !strings.Contains(offAErr(out), "/lc → /la → /lb → /lc") {
		t.Fatalf("loop: %v", out)
	}
	lbID := lb["data"].(map[string]any)["id"]
	if _, out := save(map[string]any{"id": lbID, "from": "/lb", "to": "/la#top"}); !strings.Contains(offAErr(out), "/lb → /la → /lb") {
		t.Fatalf("editing into a loop: %v", out)
	}
	if _, out := save(map[string]any{"id": lbID, "from": "/lb", "to": "https://other.example/x"}); offAErr(out) != "" {
		t.Fatalf("edit: %v", out)
	}
	if _, out := save(map[string]any{"id": 999999, "from": "/zz", "to": "/x"}); offAErr(out) != "Redirect not found" {
		t.Fatalf("edit missing: %v", out)
	}
	// a self-redirect cannot be saved, but an imported one is ignored by the middleware
	offACall(t, admin, "redirects", "legacyImport", map[string]any{"kind": "rows", "table": "redirects_rules", "last": false,
		"rows": []any{map[string]any{"id": 500, "from_path": "/loop", "to_url": "/loop/", "code": 301, "hits": 0, "created_at": "2026-01-01 00:00:00"}}})

	// TestMiddleware
	cases := []struct {
		method, url string
		code        int
		loc         string
	}{
		{"GET", "/old", 301, "/new"},
		{"GET", "/old/", 301, "/new"},
		{"GET", "/old?utm=1", 301, "/new?utm=1"},
		{"HEAD", "/promo?a=b", 302, "https://shop.example.com/sale?a=b#top"},
		{"GET", "/old/deep/", 308, "/x"},
		{"GET", "/Case", 301, "/upper"},
		{"GET", "/case", 301, "/lower"},
		{"GET", "/OLD", 404, ""},
		{"GET", "/loop", 404, ""},
		{"GET", "/about", 200, ""},
	}
	for _, cs := range cases {
		r := c.send(cs.method, cs.url, nil, nil)
		if r.Code != cs.code || r.Header().Get("Location") != cs.loc {
			t.Errorf("%s %s: got %d %q, want %d %q", cs.method, cs.url, r.Code, r.Header().Get("Location"), cs.code, cs.loc)
		}
		if cs.loc != "" && r.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s %s: Cache-Control %q", cs.method, cs.url, r.Header().Get("Cache-Control"))
		}
	}
	if r := c.send("POST", "/old", nil, nil); r.Code == 301 || r.Header().Get("Location") != "" {
		t.Errorf("POST must not redirect: %d %q", r.Code, r.Header().Get("Location"))
	}
	c.send("GET", "/missing", nil, nil)
	c.send("GET", "/missing", nil, nil)

	// hits and the 404 log (written by site.afterResponse): 3 × /old, 1 promo, 1 deep; GET 404s only
	e.code.WaitAfterResponse()
	_, out = call("redirects:list", map[string]any{})
	data := out["data"].(map[string]any)
	hits := map[string]float64{}
	for _, r := range data["rules"].([]any) {
		m := r.(map[string]any)
		hits[m["from"].(string)] = m["hits"].(float64)
	}
	if hits["/old"] != 3 || hits["/promo"] != 1 || hits["/old/deep"] != 1 || hits["/loop"] != 0 || hits["/Case"] != 1 {
		t.Fatalf("hits: %v", hits)
	}
	nf := map[string]float64{}
	for _, r := range data["notFound"].([]any) {
		m := r.(map[string]any)
		nf[m["path"].(string)] = m["hits"].(float64)
	}
	if nf["/missing"] != 2 || nf["/OLD"] != 1 || nf["/loop"] != 1 || len(nf) != 3 {
		t.Fatalf("404 log: %v", nf)
	}
	if first := data["notFound"].([]any)[0].(map[string]any); first["path"] != "/missing" {
		t.Fatalf("most hit first: %v", first)
	}
	html, _ = offAAdminPage(t, e, admin, "redirects:redirects", "")
	if !strings.Contains(html, "<code>/old</code>") || !strings.Contains(html, `data-path="/missing"`) || !strings.Contains(html, "×2") ||
		!strings.Contains(html, `href="?from=%2Fmissing&rp=1&mp=1">Fix</a>`) || !strings.Contains(html, `data-rd="clear"`) {
		t.Fatalf("admin page with data:\n%s", html)
	}
	// "Fix" prefills the form
	if html, _ := offAAdminPage(t, e, admin, "redirects:redirects", "from=%2Fmissing"); !strings.Contains(html, `name="from" value="/missing"`) || !strings.Contains(html, `name="to" value="/"`) {
		t.Fatalf("fix prefill:\n%s", html)
	}
	// edit view + delete (an edit or delete is effective at once: the rules cache is dropped)
	var oldID float64
	for _, r := range data["rules"].([]any) {
		if m := r.(map[string]any); m["from"] == "/old" {
			oldID = m["id"].(float64)
		}
	}
	if html, _ := offAAdminPage(t, e, admin, "redirects:redirects", fmt.Sprintf("edit=%d", int(oldID))); !strings.Contains(html, `name="to" value="/new"`) || !strings.Contains(html, "💾 Save") {
		t.Fatalf("edit view:\n%s", html)
	}
	if _, out := save(map[string]any{"id": oldID, "from": "/old", "to": "/newer", "code": 307}); offAErr(out) != "" {
		t.Fatalf("update: %v", out)
	}
	if r := c.send("GET", "/old", nil, nil); r.Code != 307 || r.Header().Get("Location") != "/newer" {
		t.Fatalf("after update: %d %q", r.Code, r.Header().Get("Location"))
	}
	if _, out := call("redirects:delete", map[string]any{"id": oldID}); out["data"].(map[string]any)["deleted"] != float64(1) {
		t.Fatalf("delete: %v", out)
	}
	if r := c.send("GET", "/old", nil, nil); r.Code != 404 {
		t.Fatalf("after delete: %d", r.Code)
	}
	e.code.WaitAfterResponse()
	call("redirects:clear404", map[string]any{})
	if _, out := call("redirects:list", map[string]any{}); len(out["data"].(map[string]any)["notFound"].([]any)) != 0 {
		t.Fatalf("after clear: %v", out)
	}

	// Thai admin language
	admin.do("PUT", "/api/v1/auth/me", map[string]any{"locale": "th"})
	html, call = offAAdminPage(t, e, admin, "redirects:redirects", "")
	if !strings.Contains(html, "เปลี่ยนเส้นทางลิงก์") || !strings.Contains(html, "ยังไม่พบลิงก์เสีย") {
		t.Fatalf("thai admin page:\n%s", html)
	}
	if _, out := call("redirects:save", map[string]any{"from": "old", "to": "/x"}); offAErr(out) != "เส้นทางต้นทางต้องขึ้นต้นด้วย / เช่น /old-page" {
		t.Fatalf("thai message: %v", out)
	}

	// legacy import: rules (old ids kept when free, upsert by exact source path) and the 404 log (no doubled counts)
	rules := map[string]any{"kind": "rows", "table": "redirects_rules", "last": false, "rows": []any{
		map[string]any{"id": 900, "from_path": "/legacy", "to_url": "/new-home", "code": 302, "hits": 7, "created_at": "2025-05-06 07:08:09"},
		map[string]any{"id": 901, "from_path": "/Legacy", "to_url": "https://example.org/", "code": 308, "hits": 1, "created_at": "2025-05-06T07:08:09Z"},
		map[string]any{"id": 902, "from_path": "/promo", "to_url": "/promo-2026", "code": 301, "hits": 40, "created_at": nil},
	}}
	nfRows := map[string]any{"kind": "rows", "table": "redirects_404", "last": true, "rows": []any{
		map[string]any{"path": "/gone", "hits": 12, "last_seen": "2026-02-03 04:05:06"},
		map[string]any{"path": "/Gone", "hits": 3, "last_seen": "2026-02-03 04:05:06"},
	}}
	before := offACount(t, e, "p_redirects_rules")
	for i := 0; i < 2; i++ {
		if got := offACall(t, admin, "redirects", "legacyImport", rules).(map[string]any); got["imported"] != float64(3) {
			t.Fatalf("rules import #%d: %v", i, got)
		}
		if got := offACall(t, admin, "redirects", "legacyImport", nfRows).(map[string]any); got["imported"] != float64(2) {
			t.Fatalf("404 import #%d: %v", i, got)
		}
	}
	if n := offACount(t, e, "p_redirects_rules"); n != before+2 { // /promo was updated, not added
		t.Fatalf("rules after two imports: %d (before %d)", n, before)
	}
	if r := c.send("GET", "/legacy?a=1", nil, nil); r.Code != 302 || r.Header().Get("Location") != "/new-home?a=1" {
		t.Fatalf("imported rule: %d %q", r.Code, r.Header().Get("Location"))
	}
	if r := c.send("GET", "/Legacy", nil, nil); r.Code != 308 || r.Header().Get("Location") != "https://example.org/" {
		t.Fatalf("imported case-sensitive rule: %d %q", r.Code, r.Header().Get("Location"))
	}
	if r := c.send("GET", "/promo", nil, nil); r.Code != 301 || r.Header().Get("Location") != "/promo-2026" {
		t.Fatalf("upserted rule: %d %q", r.Code, r.Header().Get("Location"))
	}
	e.code.WaitAfterResponse()
	_, out = call("redirects:list", map[string]any{})
	data = out["data"].(map[string]any)
	ids := map[string]float64{}
	hits = map[string]float64{}
	for _, r := range data["rules"].([]any) {
		m := r.(map[string]any)
		ids[m["from"].(string)], hits[m["from"].(string)] = m["id"].(float64), m["hits"].(float64)
	}
	if ids["/legacy"] != 900 || ids["/Legacy"] != 901 || hits["/legacy"] != 8 || hits["/promo"] != 41 {
		t.Fatalf("imported rules: ids %v hits %v", ids, hits)
	}
	nf = map[string]float64{}
	for _, r := range data["notFound"].([]any) {
		m := r.(map[string]any)
		nf[m["path"].(string)] = m["hits"].(float64)
	}
	if nf["/gone"] != 12 || nf["/Gone"] != 3 {
		t.Fatalf("imported 404s: %v", nf)
	}
}

// The 404 log keeps the top 200 paths (trimmed after a new path, at most once a minute).
func TestOfficialRedirects404Trim(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	t.Cleanup(e.code.WaitAfterResponse)
	admin, _ := setupSite(t, c)
	offAInstall(t, admin, "redirects")
	offASiteCode(t, admin, "")
	var rows []any
	for i := 0; i < 250; i++ {
		rows = append(rows, map[string]any{"path": fmt.Sprintf("/scan/%d", i), "hits": 1 + i%5, "last_seen": "2026-01-01 00:00:00"})
	}
	if got := offACall(t, admin, "redirects", "legacyImport", map[string]any{"kind": "rows", "table": "redirects_404", "rows": rows, "last": true}).(map[string]any); got["imported"] != float64(250) {
		t.Fatalf("import: %v", got)
	}
	c.send("GET", "/brand-new-missing", nil, nil)
	e.code.WaitAfterResponse()
	if n := offACount(t, e, "p_redirects_notfound"); n != 200 {
		t.Fatalf("404 rows after trim: %d", n)
	}
}
