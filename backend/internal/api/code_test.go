package api

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/db"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/seed"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/sitecode"
	"github.com/anasaki/studio/backend/internal/store"
)

// newCodeAPI is newTestAPI with Site Code wired like cmd/server: the API
// under /api/v1 and the public site (plugins → Site Code → pages) on "/".
func newCodeAPI(t *testing.T, tweak func(*sitecode.Config)) (*client, *sitecode.Engine, *store.Store) {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN not set (use a throw-away database)")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Skip("test database unreachable: ", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"users", "posts", "post_terms", "terms", "revisions", "media", "menus", "components", "options", "roles",
		"script_files", "script_versions", "script_logs", "script_audit", "script_access",
		"post_meta", "script_settings", "script_jobs", "script_cron"} {
		if _, err := conn.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatal(err)
		}
	}
	st := store.New(conn)
	if err := seed.Roles(st); err != nil {
		t.Fatal(err)
	}
	up := t.TempDir()
	pm, err := plugin.NewManager(st, up)
	if err != nil {
		t.Fatal(err)
	}
	web := site.New(st, pm)
	authn := auth.New("test-secret")
	cfg := sitecode.Config{Auth: authn, Services: pm.ServicePublicPaths, PostURL: site.PostURL,
		DSN: dsn, CodeDSN: os.Getenv("TEST_CODE_DB_DSN"), Emit: pm.Emit, TypeSlugs: site.SetTypeSlugs,
		SecretKey: "test-site-code-key", KeyDir: t.TempDir(), UploadDir: up}
	if tweak != nil {
		tweak(&cfg)
	}
	resetCodeDB(t, conn, os.Getenv("TEST_CODE_DB_DSN"))
	code := sitecode.New(st, cfg)
	t.Cleanup(code.Close)
	code.RouteTimeout, code.EventTimeout = 500*time.Millisecond, 2*time.Second
	web.SetCode(code.ServePublic)
	web.SetCodeSessions(code.Session)
	wireSiteContent(web, code)
	pm.Listen(code.OnEvent)
	mux := http.NewServeMux()
	a := New(st, authn, up, pm, web)
	a.SetSiteCode(code)
	a.Routes(mux)
	mux.Handle("/", pm.Middleware(web))
	t.Cleanup(code.WaitEvents)
	return &client{t: t, h: mux, api: a}, code, st
}

// wireSiteContent gives the public site the custom post types and
// taxonomies of Site Code and the JS plugins, as cmd/server does.
func wireSiteContent(web *site.Site, code *sitecode.Engine) {
	web.SetPostTypes(func(opts map[string]string) []site.PostTypeRoute {
		var out []site.PostTypeRoute
		for _, pt := range code.ContentFor(opts).PostTypes {
			out = append(out, site.PostTypeRoute{Type: pt.Type, Slug: pt.Slug, Public: pt.Public, Label: pt.Label})
		}
		return out
	})
	web.SetTaxonomies(func(opts map[string]string) []site.TaxonomyRoute {
		var out []site.TaxonomyRoute
		for _, t := range code.ContentFor(opts).Taxonomies {
			out = append(out, site.TaxonomyRoute{ID: t.ID, Slug: t.Slug, Public: t.Public, Label: t.Label, PostTypes: t.PostTypes})
		}
		return out
	})
}

// resetCodeDB drops what earlier tests left in the code database (separate
// mode) and the sc_ / bookkeeping tables of fallback mode in the CMS database.
func resetCodeDB(t *testing.T, cms *sql.DB, codeDSN string) {
	t.Helper()
	drop := func(db *sql.DB, extra string) {
		ctx := context.Background()
		c, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		rows, err := c.QueryContext(ctx, `SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()`+extra)
		if err != nil {
			t.Fatal(err)
		}
		var names, types []string
		for rows.Next() {
			var n, ty string
			_ = rows.Scan(&n, &ty)
			names, types = append(names, n), append(types, ty)
		}
		rows.Close()
		_, _ = c.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0")
		for i, n := range names {
			kw := "TABLE"
			if types[i] == "VIEW" {
				kw = "VIEW"
			}
			if _, err := c.ExecContext(ctx, "DROP "+kw+" IF EXISTS `"+n+"`"); err != nil {
				t.Fatalf("drop %s: %v", n, err)
			}
		}
		_, _ = c.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1")
	}
	drop(cms, ` AND (TABLE_NAME LIKE 'sc\_%' OR TABLE_NAME IN ('script_migrations', 'script_table_backups', 'script_relations'))`)
	if codeDSN != "" {
		db, err := sql.Open("mysql", codeDSN)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		drop(db, "")
	}
}

// send makes a request with headers and returns the recorder.
func (c *client) send(method, path string, body io.Reader, hdr map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	if body == nil {
		body = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, body)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	return rec
}

// doAny is do for any JSON shape (arrays too).
func (c *client) doAny(method, path string, body any) (int, any) {
	c.t.Helper()
	b, _ := json.Marshal(body)
	rec := c.send(method, path, bytes.NewReader(b), map[string]string{"Content-Type": "application/json"})
	var out any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// put saves a draft file (base "" = new) and returns its updatedAt.
func (c *client) put(path, content, base string) string {
	c.t.Helper()
	code, b := c.do("PUT", "/api/v1/code/files/"+path, map[string]any{"content": content, "baseUpdatedAt": base})
	if code != 200 {
		c.t.Fatalf("PUT %s: %d %v", path, code, b)
	}
	return b["updatedAt"].(string)
}

// publish publishes the draft based on version base (nil = none).
func (c *client) publish(base any) int {
	c.t.Helper()
	code, b := c.do("POST", "/api/v1/code/publish", map[string]any{"message": "test", "baseVersion": base})
	if code != 200 {
		c.t.Fatalf("publish: %d %v", code, b)
	}
	return int(b["version"].(map[string]any)["id"].(float64))
}

// public requests a public URL (Accept: application/json).
func (c *client) public(method, path string) (int, string) {
	c.t.Helper()
	rec := c.send(method, path, nil, map[string]string{"Accept": "application/json"})
	return rec.Code, rec.Body.String()
}

func uploadReq(t *testing.T, c *client, query, name string, data []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	rec := c.send("POST", "/api/v1/code/upload"+query, &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		w, _ := zw.Create(n)
		_, _ = w.Write([]byte(c))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func unzip(t *testing.T, b []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		c, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(c)
	}
	return out
}

func strs(v any) string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, fmt.Sprint(x))
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// ---------------------------------------------------------------- tests

func TestCodeCaps(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	editor := user("Ed", "editor")
	if code, _ := editor.do("GET", "/api/v1/code/files", nil); code != 403 {
		t.Fatalf("editor without caps read files: %d", code)
	}
	if code, _ := editor.do("GET", "/api/v1/code/status", nil); code != 403 {
		t.Fatalf("editor without caps read status: %d", code)
	}
	if code, _ := c.do("GET", "/api/v1/code/files", nil); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
	admin.put("main.ts", "site.route('GET /hello', { access: ['*'] }, () => 'hi')\n", "")

	viewer := roleUser(t, c, admin, "code-viewer", auth.CapRead, auth.CapViewScripts)
	if code, files := viewer.doAny("GET", "/api/v1/code/files", nil); code != 200 || len(files.([]any)) != 1 {
		t.Fatalf("viewer list: %d %v", code, files)
	}
	if code, f := viewer.do("GET", "/api/v1/code/files/main.ts", nil); code != 200 || !strings.Contains(f["content"].(string), "GET /hello") {
		t.Fatalf("viewer get: %d %v", code, f)
	}
	if code := viewer.send("GET", "/api/v1/code/types", nil, nil).Code; code != 200 {
		t.Fatalf("viewer types: %d", code)
	}
	for _, rq := range [][2]string{{"PUT", "/api/v1/code/files/x.ts"}, {"POST", "/api/v1/code/build"}, {"POST", "/api/v1/code/run"},
		{"POST", "/api/v1/code/publish"}, {"DELETE", "/api/v1/code/files/main.ts"}, {"POST", "/api/v1/code/safe-mode"}, {"DELETE", "/api/v1/code/logs"}} {
		if code, _ := viewer.do(rq[0], rq[1], map[string]any{"content": "x"}); code != 403 {
			t.Errorf("viewer %s %s: %d", rq[0], rq[1], code)
		}
	}

	dev := roleUser(t, c, admin, "code-dev", auth.CapRead, auth.CapViewScripts, auth.CapEditScripts)
	dev.put("lib.ts", "export const x = 1\n", "")
	if code, b := dev.do("POST", "/api/v1/code/build", nil); code != 200 || b["ok"] != true {
		t.Fatalf("dev build: %d %v", code, b)
	}
	if code, b := dev.do("POST", "/api/v1/code/run", map[string]any{"method": "GET", "path": "/hello"}); code != 200 || b["body"] != "hi" || b["status"].(float64) != 200 {
		t.Fatalf("dev run: %d %v", code, b)
	}
	if code, _ := dev.do("POST", "/api/v1/code/publish", map[string]any{"message": "x"}); code != 403 {
		t.Fatalf("edit_scripts must not publish: %d", code)
	}
	if code, _ := dev.do("POST", "/api/v1/code/versions/1/restore", map[string]any{}); code != 403 {
		t.Fatalf("edit_scripts must not restore: %d", code)
	}
	if code, _ := dev.do("POST", "/api/v1/code/safe-mode", map[string]any{"enabled": true}); code != 403 {
		t.Fatalf("edit_scripts must not use safe mode: %d", code)
	}
	// the escalation guard: a role manager without publish_scripts cannot hand it out
	mgr := roleUser(t, c, admin, "role-mgr", auth.CapRead, auth.CapManageUsers, auth.CapViewScripts)
	if code, _ := mgr.do("POST", "/api/v1/roles", map[string]any{"slug": "sneaky", "name": "S", "caps": []string{"read", "publish_scripts"}}); code != 403 {
		t.Fatalf("granted publish_scripts without holding it: %d", code)
	}
	// catalog lists the new caps in the "code" group; built-in roles don't have them
	_, roles := admin.do("GET", "/api/v1/roles", nil)
	found := 0
	for _, ci := range roles["caps"].([]any) {
		m := ci.(map[string]any)
		if strings.HasSuffix(m["key"].(string), "_scripts") && m["group"] == "code" {
			found++
		}
	}
	if found != 4 {
		t.Fatalf("code caps in catalog: %d", found)
	}
	if auth.RoleHas("editor", auth.CapViewScripts) || !auth.RoleHas("administrator", auth.CapPublishScripts) {
		t.Fatal("default grants")
	}
}

func TestCodePublishFlow(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)

	// a syntax error: build reports file:line, publish refuses
	admin.put("main.ts", "import { greet } from './lib/greet'\nsite.route('GET /hello/{name}', { access: ['*'] }, ({ request }) => ({ hi: greet(request.params.name) }))\n", "")
	base := admin.put("lib/greet.ts", "export function greet(n: string) {\n  return n +\n}\n", "")
	code, b := admin.do("POST", "/api/v1/code/build", nil)
	errs, _ := b["errors"].([]any)
	if code != 200 || b["ok"] != false || len(errs) == 0 || errs[0].(map[string]any)["file"] != "lib/greet.ts" || errs[0].(map[string]any)["line"].(float64) != 3 {
		t.Fatalf("build errors: %d %v", code, b)
	}
	code, b = admin.do("POST", "/api/v1/code/publish", map[string]any{"message": "broken", "baseVersion": nil})
	if code != 400 || b["errors"] == nil {
		t.Fatalf("broken publish: %d %v", code, b)
	}
	if _, st := admin.do("GET", "/api/v1/code/status", nil); st["liveVersion"] != nil || st["draftChanged"] != true {
		t.Fatalf("status after refused publish: %v", st)
	}

	// file conflicts
	admin.put("lib/greet.ts", "export function greet(n: string) {\n  return 'x'\n}\n", base)
	code, b = admin.do("PUT", "/api/v1/code/files/lib/greet.ts", map[string]any{"content": "stale", "baseUpdatedAt": base})
	if code != 409 || b["current"].(map[string]any)["content"] != "export function greet(n: string) {\n  return 'x'\n}\n" {
		t.Fatalf("stale save: %d %v", code, b)
	}
	if code, _ := admin.do("PUT", "/api/v1/code/files/lib/greet.ts", map[string]any{"content": "new", "baseUpdatedAt": ""}); code != 409 {
		t.Fatalf("creating an existing file: %d", code)
	}
	_, cur := admin.do("GET", "/api/v1/code/files/lib/greet.ts", nil)
	admin.put("lib/greet.ts", "export function greet(n: string): string {\n  return 'hello ' + n\n}\n", cur["updatedAt"].(string))
	admin.put("admin.ts", "site.route('GET /secret', (c) => ({ who: c.user ? c.user.name : null, caps: c.user.can('manage_options') }))\n"+
		"site.route('GET /team', { access: ['editor'] }, (c) => c.user.role)\n"+
		"site.route('POST /echo', { access: ['*'] }, (c) => ({ got: c.request.json(), q: c.request.query.x, h: c.request.headers['x-hi'] ?? null, auth: c.request.headers['authorization'] ?? null }))\n", "")
	_, cur = admin.do("GET", "/api/v1/code/files/main.ts", nil)
	admin.put("main.ts", "import './admin'\nimport { greet } from './lib/greet'\nsite.route('GET /hello/{name}', { access: ['*'] }, ({ request }) => ({ hi: greet(request.params.name) }))\n", cur["updatedAt"].(string))

	code, b = admin.do("POST", "/api/v1/code/build", nil)
	if code != 200 || b["ok"] != true || len(b["registered"].(map[string]any)["routes"].([]any)) != 4 {
		t.Fatalf("fixed build: %d %v", code, b)
	}
	// nothing is live before publishing
	if code, _ := c.public("GET", "/hello/bob"); code != 404 {
		t.Fatalf("draft must not be live: %d", code)
	}
	v1 := admin.publish(nil)

	code, body := c.public("GET", "/hello/bob")
	if code != 200 || strings.TrimSpace(body) != `{"hi":"hello bob"}` {
		t.Fatalf("public route: %d %s", code, body)
	}
	// access: default administrator only
	if code, _ := c.public("GET", "/secret"); code != 401 {
		t.Fatalf("anonymous on admin-only route: %d", code)
	}
	if code, body := admin.public("GET", "/secret"); code != 200 || strings.TrimSpace(body) != `{"who":"Owner","caps":true}` {
		t.Fatalf("admin on admin-only route: %d %s", code, body)
	}
	editor, author := user("Ed", "editor"), user("Au", "author")
	if code, _ := editor.public("GET", "/secret"); code != 403 {
		t.Fatalf("editor on admin-only route: %d", code)
	}
	if code, body := editor.public("GET", "/team"); code != 200 || body != "editor" {
		t.Fatalf("editor on editor route: %d %s", code, body)
	}
	if code, _ := author.public("GET", "/team"); code != 403 {
		t.Fatalf("author on editor route: %d", code)
	}
	rec := admin.send("POST", "/echo?x=1", strings.NewReader(`{"a":[1]}`), map[string]string{"Content-Type": "application/json", "X-Hi": "yo"})
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"got":{"a":[1]},"q":"1","h":"yo","auth":null}` {
		t.Fatalf("echo: %d %s", rec.Code, rec.Body.String())
	}
	// normal pages still work; method mismatch falls through to the site
	if code, _ := c.public("GET", "/no-such-page"); code != 404 {
		t.Fatalf("page routing: %d", code)
	}
	if code, _ := c.public("GET", "/echo"); code != 404 {
		t.Fatalf("GET on a POST route: %d", code)
	}

	_, st := admin.do("GET", "/api/v1/code/status", nil)
	if st["liveVersion"].(float64) != float64(v1) || st["draftChanged"] != false || len(st["registered"].(map[string]any)["routes"].([]any)) != 4 {
		t.Fatalf("status after publish: %v", st)
	}
	// publish conflicts
	if code, b := admin.do("POST", "/api/v1/code/publish", map[string]any{"message": "again", "baseVersion": nil}); code != 409 || b["liveVersion"].(float64) != float64(v1) {
		t.Fatalf("stale publish: %d %v", code, b)
	}
	// reserved paths are refused and the live version keeps running
	_, cur = admin.do("GET", "/api/v1/code/files/main.ts", nil)
	admin.put("main.ts", cur["content"].(string)+"site.route('GET /uploads/x', () => 1)\n", cur["updatedAt"].(string))
	code, b = admin.do("POST", "/api/v1/code/publish", map[string]any{"message": "reserved", "baseVersion": v1})
	if code != 400 || !strings.Contains(fmt.Sprint(b["errors"]), "สงวน") {
		t.Fatalf("reserved publish: %d %v", code, b)
	}
	if code, _ := c.public("GET", "/hello/bob"); code != 200 {
		t.Fatalf("live version stopped after a refused publish: %d", code)
	}
	if _, st := admin.do("GET", "/api/v1/code/status", nil); st["draftChanged"] != true {
		t.Fatalf("draftChanged: %v", st)
	}
}

func TestCodeRuntimeErrorsAndLogs(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	admin.put("main.ts", "import './routes'\n", "")
	admin.put("routes.ts", "// routes\nsite.route('GET /throw', { access: ['*'] }, () => {\n  log.warn('about to fail')\n  throw new Error('bad thing')\n})\n"+
		"site.route('GET /loop', { access: ['*'] }, () => { for (;;) {} })\n"+
		"site.route('GET /ok', { access: ['*'] }, () => { log.info('fine', 42); return 'ok' })\n", "")
	admin.publish(nil)

	code, body := c.public("GET", "/throw")
	if code != 500 || strings.Contains(body, "bad thing") || strings.Contains(body, "routes.ts") || !strings.Contains(body, `"error"`) {
		t.Fatalf("error response leaks details: %d %s", code, body)
	}
	if rec := c.send("GET", "/throw", nil, nil); rec.Code != 500 || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("HTML error page: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	start := time.Now()
	if code, _ := c.public("GET", "/loop"); code != 504 {
		t.Fatalf("infinite loop: %d", code)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout too slow")
	}
	c.public("GET", "/ok")

	_, b := admin.do("GET", "/api/v1/code/logs?level=error&source=route%20GET%20/throw", nil)
	items := b["items"].([]any)
	if b["total"].(float64) != 2 || len(items) != 2 {
		t.Fatalf("error logs: %v", b)
	}
	first := items[0].(map[string]any)
	if !strings.Contains(first["message"].(string), "bad thing") || !strings.Contains(first["message"].(string), "routes.ts:4") ||
		!strings.Contains(fmt.Sprint(first["trace"]), "routes.ts:4") || first["version"] == nil {
		t.Fatalf("source-mapped error log: %v", first)
	}
	if _, b := admin.do("GET", "/api/v1/code/logs?q=fine", nil); b["total"].(float64) != 1 || items0(b)["message"] != "fine 42" || items0(b)["level"] != "info" {
		t.Fatalf("search logs: %v", b)
	}
	if _, b := admin.do("GET", "/api/v1/code/logs?level=warn", nil); b["total"].(float64) != 2 { // about to fail ×2
		t.Fatalf("warn logs: %v", b)
	}
	if _, b := admin.do("GET", "/api/v1/code/logs?source=route%20GET%20/loop", nil); !strings.Contains(fmt.Sprint(b["items"]), "timeout") {
		t.Fatalf("timeout log: %v", b)
	}
	if _, b := admin.do("GET", "/api/v1/code/logs?perPage=2&page=1", nil); len(b["items"].([]any)) != 2 || b["total"].(float64) < 5 {
		t.Fatalf("paging: %v", b)
	}
	if code, _ := admin.do("GET", "/api/v1/code/logs?level=fatal", nil); code != 400 {
		t.Fatalf("bad level: %d", code)
	}
	if _, st := admin.do("GET", "/api/v1/code/status", nil); st["lastError"] == nil {
		t.Fatalf("lastError: %v", st)
	}
	rec := admin.send("GET", "/api/v1/code/logs/download?level=error", nil, nil)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") || !strings.Contains(rec.Body.String(), "[error] route GET /throw") {
		t.Fatalf("logs download: %d %s", rec.Code, rec.Body.String())
	}
	viewer := roleUser(t, c, admin, "log-viewer", auth.CapRead, auth.CapViewScripts)
	if code, _ := viewer.do("DELETE", "/api/v1/code/logs", nil); code != 403 {
		t.Fatalf("clear logs without manage_scripts: %d", code)
	}
	if code, _ := admin.do("DELETE", "/api/v1/code/logs", nil); code != 204 {
		t.Fatalf("clear logs: %d", code)
	}
	if _, b := admin.do("GET", "/api/v1/code/logs", nil); b["total"].(float64) != 0 {
		t.Fatalf("logs after clear: %v", b)
	}

	// test runs report the error with the source location
	code, b = admin.do("POST", "/api/v1/code/run", map[string]any{"method": "GET", "path": "/throw"})
	if code != 200 || !strings.Contains(fmt.Sprint(b["error"]), "routes.ts:4") || len(b["logs"].([]any)) != 1 {
		t.Fatalf("run error: %d %v", code, b)
	}
	if _, b = admin.do("POST", "/api/v1/code/run", map[string]any{"method": "GET", "path": "/nope"}); b["error"] == nil {
		t.Fatalf("run unknown route: %v", b)
	}
	if _, b = admin.do("POST", "/api/v1/code/run", map[string]any{"method": "GET", "path": "/ok"}); b["body"] != "ok" ||
		items0(map[string]any{"items": b["logs"]})["message"] != "fine 42" {
		t.Fatalf("run ok: %v", b)
	}
}

func items0(b map[string]any) map[string]any {
	items, _ := b["items"].([]any)
	if len(items) == 0 {
		return map[string]any{}
	}
	return items[0].(map[string]any)
}

func TestCodeUploadDownload(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	project := map[string]string{"site/main.ts": "import './lib/a'\n", "site/lib/a.ts": "export const a = 1\n", "site/README.md": "# hi\n",
		"site/anasaki.d.ts": "ignored", "__MACOSX/site/._main.ts": "junk"}
	code, d := uploadReq(t, admin, "?dryRun=1&mode=merge", "site.zip", zipBytes(t, project))
	if code != 200 || strs(d["added"]) != "README.md,lib/a.ts,main.ts" || len(d["changed"].([]any)) != 0 {
		t.Fatalf("dry run: %d %v", code, d)
	}
	if _, files := admin.doAny("GET", "/api/v1/code/files", nil); len(files.([]any)) != 0 {
		t.Fatalf("dry run wrote files: %v", files)
	}
	if code, d = uploadReq(t, admin, "?mode=merge", "site.zip", zipBytes(t, project)); code != 200 || len(d["added"].([]any)) != 3 {
		t.Fatalf("merge upload: %d %v", code, d)
	}
	// download → upload (replace) → download gives the same files
	rec := admin.send("GET", "/api/v1/code/download", nil, nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/zip" || !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %d %v", rec.Code, rec.Header())
	}
	first := rec.Body.Bytes()
	got := unzip(t, first)
	if len(got) != 4 || got["main.ts"] != "import './lib/a'\n" || !strings.Contains(got["anasaki.d.ts"], "declare const site") {
		t.Fatalf("zip contents: %v", got)
	}
	code, d = uploadReq(t, admin, "?mode=replace", "site-code.zip", first)
	if code != 200 || len(d["added"].([]any))+len(d["changed"].([]any))+len(d["removed"].([]any)) != 0 || d["unchanged"].(float64) != 3 {
		t.Fatalf("round-trip upload: %d %v", code, d)
	}
	second := admin.send("GET", "/api/v1/code/download", nil, nil).Body.Bytes()
	if !bytes.Equal(first, second) {
		t.Fatalf("round trip changed the zip: %v vs %v", unzip(t, first), unzip(t, second))
	}
	// replace removes files missing from the zip
	code, d = uploadReq(t, admin, "?mode=replace", "p.zip", zipBytes(t, map[string]string{"main.ts": "// new\n", "extra.js": "1"}))
	if code != 200 || strs(d["removed"]) != "README.md,lib/a.ts" || strs(d["changed"]) != "main.ts" || strs(d["added"]) != "extra.js" {
		t.Fatalf("replace: %d %v", code, d)
	}
	// a single file goes into dir
	if code, d = uploadReq(t, admin, "?dir=lib", "util.ts", []byte("export const u = 2\n")); code != 200 || strs(d["added"]) != "lib/util.ts" {
		t.Fatalf("single file: %d %v", code, d)
	}
	// refused uploads
	for name, z := range map[string][]byte{
		"zip-slip": zipBytes(t, map[string]string{"../evil.ts": "x"}),
		"bad ext":  zipBytes(t, map[string]string{"main.ts": "", "x.php": "<?php"}),
		"garbage":  []byte("PK not really"),
	} {
		if code, _ := uploadReq(t, admin, "", "x.zip", z); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := uploadReq(t, admin, "", "evil.exe", []byte("MZ")); code != 400 {
		t.Error("single file with a bad extension accepted")
	}
	if code, _ := uploadReq(t, admin, "?dir=../x", "a.ts", []byte("1")); code != 400 {
		t.Error("bad dir accepted")
	}
	// file / folder downloads
	rec = admin.send("GET", "/api/v1/code/download?path=lib/util.ts", nil, nil)
	if rec.Code != 200 || rec.Body.String() != "export const u = 2\n" || !strings.Contains(rec.Header().Get("Content-Disposition"), "util.ts") {
		t.Fatalf("file download: %d %s", rec.Code, rec.Body.String())
	}
	admin.put("lib/deep/x.ts", "x", "")
	rec = admin.send("GET", "/api/v1/code/download?path=lib", nil, nil)
	if z := unzip(t, rec.Body.Bytes()); len(z) != 2 || z["lib/util.ts"] == "" || z["lib/deep/x.ts"] != "x" {
		t.Fatalf("folder download: %v", z)
	}
	if code := admin.send("GET", "/api/v1/code/download?path=nope", nil, nil).Code; code != 404 {
		t.Fatalf("missing download: %d", code)
	}
	// move and delete folders
	if code, b := admin.do("POST", "/api/v1/code/files/move", map[string]any{"from": "lib", "to": "src/lib"}); code != 204 {
		t.Fatalf("move folder: %d %v", code, b)
	}
	if code, _ := admin.do("POST", "/api/v1/code/files/move", map[string]any{"from": "main.ts", "to": "extra.js"}); code != 409 {
		t.Fatalf("move onto existing: %d", code)
	}
	if code, _ := admin.do("POST", "/api/v1/code/files/move", map[string]any{"from": "main.ts", "to": "index.ts"}); code != 204 {
		t.Fatalf("rename file: %d", code)
	}
	_, files := admin.doAny("GET", "/api/v1/code/files", nil)
	var paths []string
	for _, f := range files.([]any) {
		paths = append(paths, f.(map[string]any)["path"].(string))
	}
	if strings.Join(paths, ",") != "extra.js,index.ts,src/lib/deep/x.ts,src/lib/util.ts" {
		t.Fatalf("after moves: %v", paths)
	}
	if code := admin.send("DELETE", "/api/v1/code/files/src/?recursive=1", nil, nil).Code; code != 204 {
		t.Fatalf("delete folder: %d", code)
	}
	if code := admin.send("DELETE", "/api/v1/code/files/extra.js", nil, nil).Code; code != 204 {
		t.Fatalf("delete file: %d", code)
	}
	if code := admin.send("DELETE", "/api/v1/code/files/extra.js", nil, nil).Code; code != 404 {
		t.Fatalf("delete missing: %d", code)
	}
	if _, files := admin.doAny("GET", "/api/v1/code/files", nil); len(files.([]any)) != 1 {
		t.Fatalf("after deletes: %v", files)
	}
	if code, _ := admin.do("PUT", "/api/v1/code/files/../x.ts", map[string]any{"content": "x"}); code != 400 && code != 404 && code != 301 && code != 307 {
		t.Fatalf("path traversal save: %d", code)
	}
	if code, _ := admin.do("PUT", "/api/v1/code/files/x.html", map[string]any{"content": "x"}); code != 400 {
		t.Fatalf("bad extension save: %d", code)
	}
	if code, _ := admin.do("PUT", "/api/v1/code/files/big.ts", map[string]any{"content": strings.Repeat("x", 600<<10)}); code != 400 {
		t.Fatalf("oversized save: %d", code)
	}
}

func TestCodeVersionsRestore(t *testing.T) {
	c, _, st := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	admin.put("main.ts", "site.route('GET /which', { access: ['*'] }, () => 'one')\n", "")
	v1 := admin.publish(nil)
	_, f := admin.do("GET", "/api/v1/code/files/main.ts", nil)
	admin.put("main.ts", "site.route('GET /which', { access: ['*'] }, () => 'two')\n", f["updatedAt"].(string))
	admin.put("extra.ts", "export {}\n", "")
	v2 := admin.publish(v1)
	if _, body := c.public("GET", "/which"); body != "two" {
		t.Fatalf("v2 live: %s", body)
	}
	_, list := admin.doAny("GET", "/api/v1/code/versions", nil)
	vs := list.([]any)
	if len(vs) != 2 || vs[0].(map[string]any)["id"].(float64) != float64(v2) || vs[0].(map[string]any)["live"] != true ||
		vs[1].(map[string]any)["live"] != false || vs[0].(map[string]any)["files"].(float64) != 2 || vs[0].(map[string]any)["createdBy"] != "Owner" {
		t.Fatalf("versions: %v", vs)
	}
	_, one := admin.do("GET", fmt.Sprintf("/api/v1/code/versions/%d", v1), nil)
	if files := one["files"].([]any); len(files) != 1 || files[0].(map[string]any)["path"] != "main.ts" || one["live"] != false {
		t.Fatalf("version detail: %v", one)
	}
	if _, vf := admin.do("GET", fmt.Sprintf("/api/v1/code/versions/%d/files/main.ts", v1), nil); !strings.Contains(fmt.Sprint(vf["content"]), "'one'") {
		t.Fatalf("version file: %v", vf)
	}
	if code, _ := admin.do("GET", fmt.Sprintf("/api/v1/code/versions/%d/files/nope.ts", v1), nil); code != 404 {
		t.Fatalf("missing version file: %d", code)
	}
	if code, _ := admin.do("GET", "/api/v1/code/versions/99999", nil); code != 404 {
		t.Fatalf("missing version: %d", code)
	}
	rec := admin.send("GET", fmt.Sprintf("/api/v1/code/versions/%d/download", v1), nil, nil)
	if z := unzip(t, rec.Body.Bytes()); !strings.Contains(z["main.ts"], "'one'") || len(z) != 2 {
		t.Fatalf("version zip: %v", z)
	}

	code, b := admin.do("POST", fmt.Sprintf("/api/v1/code/versions/%d/restore", v1), map[string]any{"alsoDraft": true})
	if code != 200 {
		t.Fatalf("restore: %d %v", code, b)
	}
	v3 := b["version"].(map[string]any)
	if v3["message"] != fmt.Sprintf("Restore #%d", v1) || v3["live"] != true || int(v3["id"].(float64)) <= v2 {
		t.Fatalf("restore version: %v", v3)
	}
	if _, body := c.public("GET", "/which"); body != "one" {
		t.Fatalf("after restore: %s", body)
	}
	if _, files := admin.doAny("GET", "/api/v1/code/files", nil); len(files.([]any)) != 1 {
		t.Fatalf("alsoDraft did not replace the draft: %v", files)
	}
	if _, st := admin.do("GET", "/api/v1/code/status", nil); st["draftChanged"] != false {
		t.Fatalf("draft after restore: %v", st)
	}
	// restore without alsoDraft keeps the draft
	admin.put("keep.ts", "export {}\n", "")
	if code, _ := admin.do("POST", fmt.Sprintf("/api/v1/code/versions/%d/restore", v2), map[string]any{}); code != 200 {
		t.Fatalf("restore v2: %d", code)
	}
	if _, body := c.public("GET", "/which"); body != "two" {
		t.Fatalf("after restore v2: %s", body)
	}
	if _, files := admin.doAny("GET", "/api/v1/code/files", nil); len(files.([]any)) != 2 {
		t.Fatalf("draft changed by a plain restore: %v", files)
	}
	// every mutation is audited
	var n int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM script_audit WHERE action IN ('save','publish','restore')`).Scan(&n); err != nil || n < 7 {
		t.Fatalf("audit rows: %d %v", n, err)
	}
}

func TestCodeSafeModeAndPosts(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	admin.put("main.ts", `
site.route('GET /posts', { access: ['*'] }, ({ request }) => {
  const r = db.posts.list({ perPage: 50, search: request.query.s })
  return { total: r.total, titles: r.items.map(p => p.title).sort(), first: r.items[0] ?? null }
})
site.route('GET /post/{slug}', { access: ['*'] }, ({ request }) => db.posts.get(request.params.slug))
`, "")
	admin.publish(nil)
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "Pub", "slug": "pub", "status": "publish", "content": "<p>x</p><script>alert(1)</script>"}); code != 201 {
		t.Fatalf("post: %d %v", code, b)
	}
	if code, _ := admin.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "Draft", "slug": "draft", "status": "draft"}); code != 201 {
		t.Fatal("draft")
	}
	code, body := c.public("GET", "/posts")
	var res map[string]any
	_ = json.Unmarshal([]byte(body), &res)
	if code != 200 || res["total"].(float64) != 1 || strs(res["titles"]) != "Pub" {
		t.Fatalf("anonymous sees drafts: %d %s", code, body)
	}
	p := res["first"].(map[string]any)
	if p["url"] != "/blog/pub" || strings.Contains(p["content"].(string), "<script") || p["author"] != "Owner" || p["publishedAt"] == nil {
		t.Fatalf("post shape: %v", p)
	}
	if _, body := admin.public("GET", "/posts"); !strings.Contains(body, `"total":2`) {
		t.Fatalf("admin sees drafts: %s", body)
	}
	if _, body := c.public("GET", "/post/draft"); body != "" && strings.TrimSpace(body) != "null" {
		t.Fatalf("anonymous get draft: %q", body)
	}
	if code, body := c.public("GET", "/post/draft"); code != 204 {
		t.Fatalf("null → 204: %d %s", code, body)
	}
	if _, body := c.public("GET", "/post/pub"); !strings.Contains(body, `"slug":"pub"`) {
		t.Fatalf("get published: %s", body)
	}

	// safe mode stops routes (normal routing answers), and back
	code, st := admin.do("POST", "/api/v1/code/safe-mode", map[string]any{"enabled": true})
	if code != 200 || st["safeMode"] != true {
		t.Fatalf("safe mode: %d %v", code, st)
	}
	if code, _ := c.public("GET", "/posts"); code != 404 {
		t.Fatalf("route served in safe mode: %d", code)
	}
	admin.do("POST", "/api/v1/code/safe-mode", map[string]any{"enabled": false})
	if code, _ := c.public("GET", "/posts"); code != 200 {
		t.Fatalf("route after safe mode: %d", code)
	}
}

func TestCodeEnvDisabledAndDemo(t *testing.T) {
	c, _, _ := newCodeAPI(t, func(cfg *sitecode.Config) { cfg.Disabled = true })
	admin, _ := setupSite(t, c)
	admin.put("main.ts", "site.route('GET /x', { access: ['*'] }, () => 'x')\n", "")
	admin.publish(nil)
	if code, _ := c.public("GET", "/x"); code != 404 {
		t.Fatalf("SITE_CODE_DISABLED: route served: %d", code)
	}
	if _, st := admin.do("GET", "/api/v1/code/status", nil); st["envDisabled"] != true {
		t.Fatalf("status: %v", st)
	}

	c, _, _ = newCodeAPI(t, func(cfg *sitecode.Config) { cfg.Demo = true })
	admin, _ = setupSite(t, c)
	if code, st := admin.do("GET", "/api/v1/code/status", nil); code != 200 || st["demo"] != true {
		t.Fatalf("demo status: %d %v", code, st)
	}
	for _, rq := range [][2]string{{"GET", "/api/v1/code/files"}, {"PUT", "/api/v1/code/files/main.ts"}, {"POST", "/api/v1/code/publish"},
		{"GET", "/api/v1/code/download"}, {"GET", "/api/v1/code/types"}, {"GET", "/api/v1/code/logs"}} {
		if code, b := admin.do(rq[0], rq[1], map[string]any{"content": "x"}); code != 403 || b["error"] != "ปิดในเว็บทดลอง" {
			t.Errorf("demo %s %s: %d %v", rq[0], rq[1], code, b)
		}
	}
}

func TestCodeEvents(t *testing.T) {
	c, code, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	admin.put("main.ts", `
site.on('post.published', (p) => log.info('published', p.title, p.url))
site.on('post.saved', (p) => log.info('saved', p.title, p.status))
site.on('post.deleted', (p) => log.info('deleted', p.title))
site.on('user.created', async (u) => { await null; log.info('user', u.email) })
site.on('post.published', () => { throw new Error('second handler fails') })
`, "")
	admin.publish(nil)
	_, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "Hello", "slug": "hello", "status": "publish"})
	admin.do("POST", "/api/v1/users", map[string]any{"name": "N", "email": "n@x.io", "password": "password1", "role": "author"})
	admin.do("DELETE", fmt.Sprintf("/api/v1/posts/%v", p["id"]), nil)
	code.WaitEvents()
	logs := func(q string) []string {
		_, b := admin.do("GET", "/api/v1/code/logs?"+q, nil)
		var out []string
		for _, it := range b["items"].([]any) {
			out = append(out, it.(map[string]any)["message"].(string))
		}
		sort.Strings(out)
		return out
	}
	if got := logs("source=event%20post.published&level=info"); len(got) != 1 || got[0] != "published Hello /blog/hello" {
		t.Fatalf("post.published: %v", got)
	}
	if got := logs("source=event%20post.published&level=error"); len(got) != 1 || !strings.Contains(got[0], "second handler fails") || !strings.Contains(got[0], "main.ts:6") {
		t.Fatalf("failing event handler: %v", got)
	}
	if got := logs("source=event%20post.saved"); len(got) != 1 || got[0] != "saved Hello publish" {
		t.Fatalf("post.saved: %v", got)
	}
	if got := logs("source=event%20post.deleted"); len(got) != 1 || got[0] != "deleted Hello" {
		t.Fatalf("post.deleted: %v", got)
	}
	if got := logs("source=event%20user.created"); len(got) != 1 || got[0] != "user n@x.io" {
		t.Fatalf("user.created: %v", got)
	}
	// safe mode: events are ignored
	admin.do("POST", "/api/v1/code/safe-mode", map[string]any{"enabled": true})
	admin.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "Quiet", "slug": "quiet", "status": "publish"})
	code.WaitEvents()
	if got := logs("q=Quiet"); len(got) != 0 {
		t.Fatalf("events ran in safe mode: %v", got)
	}
}
