package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/legacy"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Phase 7: migration of the former built-in plugins to their official JS
// plugins, tested with small fake official packages served by a fake Store.

// fakeStore serves official package zips (download + detail) and can go down.
type fakeStore struct {
	srv       *httptest.Server
	mu        sync.Mutex
	zips      map[string][]byte
	details   map[string]map[string]any
	down      atomic.Bool
	downloads atomic.Int32
}

func newFakeStore(t *testing.T) *fakeStore {
	t.Helper()
	fs := &fakeStore{zips: map[string][]byte{}, details: map[string]map[string]any{}}
	fs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fs.down.Load() {
			panic(http.ErrAbortHandler) // connection dropped: "store unreachable"
		}
		if r.URL.Path == "/api/v1/plugins" { // catalogue (only the recommended filter is faked)
			items := []any{}
			fs.mu.Lock()
			for _, d := range fs.details {
				if r.URL.Query().Get("recommended") != "1" || d["recommended"] == true {
					sum := map[string]any{}
					for k, v := range d {
						if k != "permissions" { // like the real Store: no permissions in summaries
							sum[k] = v
						}
					}
					items = append(items, sum)
				}
			}
			fs.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "total": len(items), "page": 1})
			return
		}
		id, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/v1/plugins/"), "/")
		fs.mu.Lock()
		z, d := fs.zips[id], fs.details[id]
		fs.mu.Unlock()
		switch {
		case rest == "download" && z != nil:
			fs.downloads.Add(1)
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(z)
		case rest == "" && d != nil:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(d)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	t.Cleanup(fs.srv.Close)
	t.Setenv("STORE_URL", fs.srv.URL)
	return fs
}

func (fs *fakeStore) put(t *testing.T, files map[string]string) {
	t.Helper()
	var mf map[string]any
	_ = json.Unmarshal([]byte(files["manifest.json"]), &mf)
	id := mf["id"].(string)
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.zips[id] = zipBytes(t, files)
	fs.details[id] = map[string]any{"id": id, "name": mf["name"], "version": mf["version"], "official": true,
		"permissions": mf["permissions"], "code": mf["code"]}
}

// recommend marks id as recommended by the Store (detail-only when the id
// has no package: listed but not downloadable).
func (fs *fakeStore) recommend(id string, checked bool, order int, extra map[string]any) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	d := fs.details[id]
	if d == nil {
		d = map[string]any{"id": id, "name": id, "version": "1.0.0", "official": true}
		fs.details[id] = d
	}
	d["recommended"], d["recommendedChecked"], d["recommendedOrder"] = true, checked, order
	for k, v := range extra {
		d[k] = v
	}
}

// fakeOfficial builds an official package of built-in id. withImport adds a
// table of imported rows (upsert by table + key) and the legacyImport /
// stats exports; settings is the manifest settings schema.
func fakeOfficial(id string, withImport bool, settings []any) map[string]string {
	code := map[string]any{"entry": "code/main.ts", "scopes": []any{"db"}, "exports": []any{"stats"}}
	files := map[string]string{
		"code/main.ts": `site.export('stats', () => ({ ok: true }));`,
	}
	if withImport {
		code["exports"] = []any{"legacyImport", "stats"}
		files["migrations/001_init.ts"] = `export default migration({
  up(schema) {
    schema.create('rows', (t) => { t.id(); t.string('tbl', 64); t.string('k', 191); t.json('data'); t.unique(['tbl', 'k']); });
    schema.create('calls', (t) => { t.id(); t.string('tbl', 64); t.integer('cnt'); t.boolean('last'); });
  },
  down(schema) { schema.drop('calls'); schema.drop('rows'); },
});`
		files["code/main.ts"] = `
const keyOf = (r: any) => String(r.id ?? r.post_id ?? (r.day ? r.day + '|' + (r.path ?? r.host ?? '') + '|' + (r.hash ?? '') : r.path));
site.export('legacyImport', (p: any) => {
  if (p.kind !== 'rows') throw new Error('unexpected payload ' + p.kind);
  let n = 0;
  const keys = p.rows.map(keyOf);
  const have = new Map<string, number>();
  if (keys.length) for (const x of db.table('rows').where('tbl', p.table).whereIn('k', keys).get() as any[]) have.set(x.k, x.id);
  p.rows.forEach((r: any, i: number) => {
    const id = have.get(keys[i]);
    if (id) db.table('rows').where('id', id).update({ data: r });
    else db.table('rows').insert({ tbl: p.table, k: keys[i], data: r });
    n++;
  });
  db.table('calls').insert({ tbl: p.table, cnt: p.rows.length, last: !!p.last });
  return { imported: n };
});
site.export('stats', (table?: string) => {
  const out: any = { rows: db.table('rows').count(), calls: db.table('calls').orderBy('id').get().map((c: any) => c.tbl + ':' + c.cnt + ':' + (c.last ? 1 : 0)).join(',') };
  if (table) out.sample = db.table('rows').where('tbl', table).orderBy('id').first();
  return out;
});`
	}
	mf := map[string]any{"id": id, "name": "Official " + id, "version": "2.0.0", "author": "Anasaki Studio", "runtime": "js",
		"permissions": []any{"code"}, "legacy": id, "code": code}
	if settings != nil {
		mf["settings"] = settings
		mf["permissions"] = []any{"code", "settings"}
	}
	b, _ := json.Marshal(mf)
	files["manifest.json"] = string(b)
	return files
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *testClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type legacyEnv struct {
	*pluginEnv
	admin *client
	mig   *legacy.Migrator
	clock *testClock
	store *fakeStore
}

// newLegacyEnv is a site "from before the upgrade": old tables dropped and
// recreated by the test, a fake store, and a Migrator whose removed
// built-ins are the given ids.
func newLegacyEnv(t *testing.T, removed ...string) *legacyEnv {
	t.Helper()
	return newLegacyEnvWith(t, nil, removed...)
}

// newLegacyEnvWith is newLegacyEnv with the engine config tweaked (demo mode).
func newLegacyEnvWith(t *testing.T, tweak func(*sitecode.Config), removed ...string) *legacyEnv {
	t.Helper()
	fs := newFakeStore(t) // before the manager: STORE_URL
	e := newPluginAPIWith(t, tweak)
	admin, _ := setupSite(t, e.c)
	clean := func() {
		for _, b := range legacy.Builtins {
			for _, tb := range b.Tables {
				_, _ = e.st.DB.Exec("DROP TABLE IF EXISTS `" + tb.Name + "`")
			}
		}
		_, _ = e.st.DB.Exec("DROP TABLE IF EXISTS legacy_migrations")
	}
	clean()
	t.Cleanup(clean)
	gone := map[string]bool{}
	for _, id := range removed {
		gone[id] = true
	}
	clock := &testClock{t: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	mig, err := legacy.New(e.st.DB, e.pm, e.code, legacy.Options{Removed: func(id string) bool { return gone[id] },
		Now: clock.now, InboxDir: t.TempDir(), Every: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	// the API was built by newPluginAPI; attach the migrator (routes check it per request)
	e.api.SetLegacy(mig)
	old := legacyWait
	legacyWait = 3 * time.Minute // Retry / upload answer when their run is over (a slow CI database)
	t.Cleanup(func() { legacyWait = old })
	return &legacyEnv{pluginEnv: e, admin: admin, mig: mig, clock: clock, store: fs}
}

func (e *legacyEnv) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.st.DB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func (e *legacyEnv) option(t *testing.T, name, value string) {
	t.Helper()
	e.exec(t, `INSERT INTO options (name, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)`, name, value)
}

func (e *legacyEnv) items(t *testing.T) (int, map[string]map[string]any) {
	t.Helper()
	code, b := e.admin.do("GET", "/api/v1/legacy-migrations", nil)
	if code != 200 {
		t.Fatalf("list: %d %v", code, b)
	}
	out := map[string]map[string]any{}
	for _, x := range b["items"].([]any) {
		m := x.(map[string]any)
		out[m["id"].(string)] = m
	}
	return int(b["waiting"].(float64)), out
}

func (e *legacyEnv) stats(t *testing.T, id string, table string) map[string]any {
	t.Helper()
	var arg any
	if table != "" {
		arg = table
	}
	v, err := e.code.CallExport(id, "stats", arg, 5*time.Second)
	if err != nil {
		t.Fatalf("stats %s: %v", id, err)
	}
	return v.(map[string]any)
}

func optionValue(t *testing.T, e *pluginEnv, name string) (string, bool) {
	t.Helper()
	var v string
	err := e.st.DB.QueryRow(`SELECT value FROM options WHERE name = ?`, name).Scan(&v)
	return v, err == nil
}

func tableExists(t *testing.T, e *pluginEnv, name string) bool {
	t.Helper()
	ok, err := legacy.TableExists(e.st.DB, name)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestLegacyMigration(t *testing.T) {
	e := newLegacyEnv(t, "forms", "cookie-consent", "maintenance", "analytics")
	ctx := context.Background()

	// ---- the site before the upgrade
	if err := legacy.CreateOldTables(e.st.DB, "forms"); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO forms_forms (id, name, fields, created_at) VALUES (1, 'Contact', '[{"name":"email","type":"email"}]', '2024-01-01 00:00:00'),
		(2, 'Quote', '[]', '2024-01-02 00:00:00')`)
	var vals []string
	for i := 1; i <= 1003; i++ {
		vals = append(vals, fmt.Sprintf("(%d, %d, '{\"n\":%d}', %v, '2024-02-01 00:00:00')", i, 1+i%2, i, i%3 == 0))
	}
	e.exec(t, `INSERT INTO forms_submissions (id, form_id, data, is_read, created_at) VALUES `+strings.Join(vals, ","))
	e.option(t, "active_plugins", `["analytics","forms","cookie-consent","backup"]`)
	e.option(t, "plugin_settings.cookie-consent", `{"message":"เราใช้คุกกี้","acceptText":"OK","position":"top","bogus":1,"background":"not-a-color"}`)
	e.option(t, "plugin_settings.maintenance", `{"enabled":true,"title":"ปิดปรับปรุง","bypassKey":"k1"}`)
	// analytics: active but the store has no official package for it in this test → stays waiting

	cookieSettings := []any{
		map[string]any{"key": "message", "type": "textarea", "default": "default message"},
		map[string]any{"key": "acceptText", "type": "text", "default": "ยอมรับ"},
		map[string]any{"key": "position", "type": "select", "default": "bottom", "options": []any{map[string]any{"value": "bottom", "label": "Bottom"}, map[string]any{"value": "top", "label": "Top"}}},
		map[string]any{"key": "background", "type": "color", "default": "#111827"},
	}
	maintSettings := []any{
		map[string]any{"key": "enabled", "type": "boolean", "default": false},
		map[string]any{"key": "title", "type": "text", "default": "Maintenance"},
		map[string]any{"key": "bypassKey", "type": "text", "default": ""},
	}
	e.store.put(t, fakeOfficial("forms", true, nil))
	e.store.put(t, fakeOfficial("cookie-consent", false, cookieSettings))
	e.store.put(t, fakeOfficial("maintenance", false, maintSettings))

	// ---- nothing happens for built-ins still compiled in
	other, err := legacy.New(e.st.DB, e.pm, e.code, legacy.Options{Removed: func(string) bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	if added, err := other.Detect(); err != nil || len(added) != 0 {
		t.Fatalf("detect with nothing removed = %v %v", added, err)
	}

	// ---- store down: everything waits, with backoff
	e.store.down.Store(true)
	e.mig.RunOnce(ctx)
	waiting, it := e.items(t)
	if waiting != 4 || len(it) != 4 {
		t.Fatalf("waiting = %d %v", waiting, it)
	}
	for _, id := range []string{"forms", "cookie-consent", "maintenance", "analytics"} {
		m := it[id]
		if m["status"] != "failed" || m["attempts"].(float64) != 1 || !strings.Contains(m["error"].(string), "store unreachable") || m["nextAttemptAt"] == nil {
			t.Fatalf("%s while store down = %v", id, m)
		}
	}
	if it["forms"]["wasActive"] != true || it["forms"]["hadData"] != true || it["maintenance"]["wasActive"] != false || it["maintenance"]["hadData"] != true {
		t.Fatalf("detection = %v / %v", it["forms"], it["maintenance"])
	}
	e.mig.RunOnce(ctx) // not due yet
	if _, it = e.items(t); it["forms"]["attempts"].(float64) != 1 {
		t.Fatalf("retried before the backoff: %v", it["forms"])
	}
	e.clock.add(61 * time.Second)
	e.mig.RunOnce(ctx) // due: second attempt, next one 2 minutes later
	_, it = e.items(t)
	if it["forms"]["attempts"].(float64) != 2 {
		t.Fatalf("second attempt: %v", it["forms"])
	}
	next, _ := time.Parse(time.RFC3339, it["forms"]["nextAttemptAt"].(string))
	if want := e.clock.now().Add(2 * time.Minute); !next.Equal(want) {
		t.Fatalf("next attempt %v, want %v", next, want)
	}
	if e.pm.InstalledVersion("forms") != "" {
		t.Fatal("installed although the store is down")
	}

	// ---- store back, admin presses Retry (the background loop runs it)
	e.store.down.Store(false)
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	e.mig.Start(runCtx)
	code, b := e.admin.do("POST", "/api/v1/legacy-migrations/retry", map[string]any{})
	if code != 200 {
		t.Fatalf("retry: %d %v", code, b)
	}
	waiting, it = e.items(t)
	if waiting != 1 || it["analytics"]["status"] != "failed" || !strings.Contains(it["analytics"]["error"].(string), "404") {
		t.Fatalf("after retry: %d %v", waiting, it["analytics"])
	}
	for _, id := range []string{"forms", "cookie-consent", "maintenance"} {
		m := it[id]
		if m["status"] != "done" || m["version"] != "2.0.0" || m["source"] != "store" || m["approvedBy"] != "system migration" ||
			m["approvedAt"] == nil || m["error"] != "" || m["doneAt"] == nil {
			t.Fatalf("%s = %v", id, m)
		}
		if strs(m["approved"].(map[string]any)["scopes"]) != "db" || !strings.HasPrefix(strs(m["approved"].(map[string]any)["permissions"]), "code") {
			t.Fatalf("%s approved = %v", id, m["approved"])
		}
	}
	if it["forms"]["imported"].(float64) != 1005 || it["forms"]["cleanable"] != true || it["cookie-consent"]["cleanable"] != true {
		t.Fatalf("forms = %v", it["forms"])
	}
	// active built-ins end up active; the inactive one (maintenance) is installed but off
	if p := pkgOf(t, e.admin, "forms"); p["active"] != true || p["legacy"] != "forms" {
		t.Fatalf("forms package = %v", p)
	}
	if p := pkgOf(t, e.admin, "maintenance"); p["active"] != false {
		t.Fatalf("maintenance package should stay off: %v", p)
	}
	// settings mapped 1:1 (same keys), invalid / unknown values dropped
	cs := pkgOf(t, e.admin, "cookie-consent")["settings"].(map[string]any)
	if cs["message"] != "เราใช้คุกกี้" || cs["acceptText"] != "OK" || cs["position"] != "top" || cs["background"] != "#111827" || cs["bogus"] != nil {
		t.Fatalf("cookie settings = %v", cs)
	}
	ms := pkgOf(t, e.admin, "maintenance")["settings"].(map[string]any)
	if ms["enabled"] != true || ms["title"] != "ปิดปรับปรุง" || ms["bypassKey"] != "k1" {
		t.Fatalf("maintenance settings = %v", ms)
	}
	// rows streamed in batches of ≤ 500, tables in dependency order, last flags
	st := e.stats(t, "forms", "forms_submissions")
	if st["rows"].(float64) != 1005 || st["calls"] != "forms_forms:2:1,forms_submissions:500:0,forms_submissions:500:0,forms_submissions:3:1" {
		t.Fatalf("forms stats = %v", st)
	}
	if s := st["sample"].(map[string]any)["data"].(map[string]any); s["form_id"].(float64) != 2 || s["is_read"] != false || s["created_at"] != "2024-02-01T00:00:00Z" ||
		s["data"].(map[string]any)["n"].(float64) != 1 {
		t.Fatalf("sample row = %v", s)
	}
	// old data is never deleted automatically
	if !tableExists(t, e.pluginEnv, "forms_forms") || !tableExists(t, e.pluginEnv, "forms_submissions") {
		t.Fatal("old forms tables were dropped")
	}
	if _, ok := optionValue(t, e.pluginEnv, "plugin_settings.cookie-consent"); !ok {
		t.Fatal("old cookie-consent settings were deleted")
	}
	if v, _ := optionValue(t, e.pluginEnv, "active_plugins"); !strings.Contains(v, `"forms"`) {
		t.Fatalf("active_plugins changed: %s", v)
	}

	// ---- idempotency: running again changes nothing; a full re-import upserts
	downloads := e.store.downloads.Load()
	e.mig.RunOnce(ctx)
	if e.store.downloads.Load() != downloads {
		t.Fatal("a done migration downloaded again")
	}
	e.exec(t, `UPDATE legacy_migrations SET status = 'failed', next_attempt_at = NULL, cursor_table = 0, cursor_offset = 0 WHERE id = 'forms'`)
	e.mig.RunOnce(ctx)
	if st := e.stats(t, "forms", ""); st["rows"].(float64) != 1005 {
		t.Fatalf("re-import duplicated rows: %v", st)
	}
	// resume from a saved position: only what is left is sent
	e.exec(t, `UPDATE legacy_migrations SET status = 'importing', cursor_table = 1, cursor_offset = 1000 WHERE id = 'forms'`)
	e.mig.RunOnce(ctx)
	st = e.stats(t, "forms", "")
	if !strings.HasSuffix(st["calls"].(string), ",forms_submissions:500:0,forms_submissions:500:0,forms_submissions:3:1,forms_submissions:3:1") {
		t.Fatalf("resume calls = %v", st["calls"])
	}
	if _, it = e.items(t); it["forms"]["status"] != "done" {
		t.Fatalf("forms after resume = %v", it["forms"])
	}

	// ---- clean up old data (explicit, confirmed, done migrations only)
	user := func(role string) *client {
		_, mk := setupUser(t, e.pluginEnv, e.admin, role)
		return mk
	}
	if code, _ := user("editor").do("POST", "/api/v1/legacy-migrations/forms/cleanup", map[string]any{"confirm": true}); code != 403 {
		t.Fatalf("editor cleanup: %d", code)
	}
	if code, _ := e.admin.do("POST", "/api/v1/legacy-migrations/forms/cleanup", map[string]any{}); code != 400 {
		t.Fatalf("cleanup without confirm: %d", code)
	}
	if code, _ := e.admin.do("POST", "/api/v1/legacy-migrations/analytics/cleanup", map[string]any{"confirm": true}); code != 409 {
		t.Fatalf("cleanup of a waiting migration: %d", code)
	}
	if code, _ := e.admin.do("POST", "/api/v1/legacy-migrations/seo/cleanup", map[string]any{"confirm": true}); code != 409 {
		t.Fatalf("cleanup without a migration: %d", code)
	}
	code, b = e.admin.do("POST", "/api/v1/legacy-migrations/forms/cleanup", map[string]any{"confirm": true})
	if code != 200 {
		t.Fatalf("cleanup: %d %v", code, b)
	}
	if tableExists(t, e.pluginEnv, "forms_forms") || tableExists(t, e.pluginEnv, "forms_submissions") {
		t.Fatal("old tables still there after clean-up")
	}
	if v, _ := optionValue(t, e.pluginEnv, "active_plugins"); strings.Contains(v, `"forms"`) || !strings.Contains(v, `"analytics"`) {
		t.Fatalf("active_plugins after clean-up: %s", v)
	}
	if _, it = e.items(t); it["forms"]["cleanedAt"] == nil || it["forms"]["cleanable"] != false || it["forms"]["status"] != "done" {
		t.Fatalf("forms after clean-up = %v", it["forms"])
	}
	// the plugin's own data is untouched
	if st := e.stats(t, "forms", ""); st["rows"].(float64) != 1005 {
		t.Fatalf("plugin rows after clean-up: %v", st)
	}
	code, _ = e.admin.do("POST", "/api/v1/legacy-migrations/cookie-consent/cleanup", map[string]any{"confirm": true})
	if _, ok := optionValue(t, e.pluginEnv, "plugin_settings.cookie-consent"); code != 200 || ok {
		t.Fatalf("cookie clean-up: %d (option kept: %v)", code, ok)
	}
}

// setupUser creates a user with role and returns it and a client for it.
func setupUser(t *testing.T, e *pluginEnv, admin *client, role string) (map[string]any, *client) {
	t.Helper()
	email := role + fmt.Sprint(time.Now().UnixNano()) + "@x.io"
	code, u := admin.do("POST", "/api/v1/users", map[string]any{"name": role, "email": email, "password": "password1", "role": role})
	if code != 201 {
		t.Fatalf("create %s: %d %v", role, code, u)
	}
	code, l := e.c.do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": "password1"})
	if code != 200 {
		t.Fatalf("login %s: %d %v", role, code, l)
	}
	return u, e.c.as(l["token"].(string))
}

func uploadLegacyZip(t *testing.T, c *client, data []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "official.zip")
	_, _ = fw.Write(data)
	_ = mw.Close()
	rec := c.send("POST", "/api/v1/legacy-migrations/upload", &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestLegacyManualZip(t *testing.T) {
	e := newLegacyEnv(t, "redirects", "seo")
	ctx := context.Background()
	if err := legacy.CreateOldTables(e.st.DB, "redirects"); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO redirects_rules (id, from_path, to_url, code, hits) VALUES (1, '/a', '/b', 301, 3), (2, '/c', 'https://x.io/', 302, 0)`)
	e.exec(t, `INSERT INTO redirects_404 (path, hits) VALUES ('/gone', 7)`)
	e.option(t, "active_plugins", `["redirects"]`)

	e.store.down.Store(true) // an air-gapped site
	e.mig.RunOnce(ctx)
	waiting, it := e.items(t)
	if waiting != 1 || it["redirects"]["status"] != "failed" || it["seo"] != nil {
		t.Fatalf("before upload: %d %v", waiting, it)
	}

	// not an official package / not waiting / third-party → refused
	notOfficial := fakeOfficial("redirects", true, nil)
	notOfficial["manifest.json"] = strings.Replace(notOfficial["manifest.json"], `"author":"Anasaki Studio"`, `"author":"Someone"`, 1)
	if code, b := uploadLegacyZip(t, e.admin, zipBytes(t, notOfficial)); code != 400 || !strings.Contains(b["error"].(string), "official") {
		t.Fatalf("non-official upload: %d %v", code, b)
	}
	noLegacy := fakeOfficial("redirects", true, nil)
	noLegacy["manifest.json"] = strings.Replace(noLegacy["manifest.json"], `"legacy":"redirects",`, ``, 1)
	if code, _ := uploadLegacyZip(t, e.admin, zipBytes(t, noLegacy)); code != 400 {
		t.Fatalf("upload without legacy: %d", code)
	}
	if code, _ := uploadLegacyZip(t, e.admin, zipBytes(t, fakeOfficial("seo", true, nil))); code != 400 {
		t.Fatalf("upload of a plugin that is not waiting: %d", code)
	}
	if code, _ := uploadLegacyZip(t, e.admin, []byte("not a zip")); code != 400 {
		t.Fatalf("garbage upload: %d", code)
	}

	// the official zip is picked up by the same task
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	e.mig.Start(runCtx)
	code, b := uploadLegacyZip(t, e.admin, zipBytes(t, fakeOfficial("redirects", true, nil)))
	if code != 200 {
		t.Fatalf("upload: %d %v", code, b)
	}
	waiting, it = e.items(t)
	if waiting != 0 || it["redirects"]["status"] != "done" || it["redirects"]["source"] != "upload" || it["redirects"]["imported"].(float64) != 3 {
		t.Fatalf("after upload: %d %v", waiting, it["redirects"])
	}
	if st := e.stats(t, "redirects", "redirects_404"); st["calls"] != "redirects_rules:2:1,redirects_404:1:1" ||
		st["sample"].(map[string]any)["data"].(map[string]any)["hits"].(float64) != 7 {
		t.Fatalf("redirects stats = %v", st)
	}
	if p := pkgOf(t, e.admin, "redirects"); p["active"] != true || p["source"] != "upload" {
		t.Fatalf("redirects package = %v", p)
	}
	if e.store.downloads.Load() != 0 {
		t.Fatal("the store was used")
	}
}

func TestLegacyRefusesThirdPartyPackage(t *testing.T) {
	e := newLegacyEnv(t, "custom-code")
	e.option(t, "plugin_settings.custom-code", `{"headCode":"<meta name=x>"}`)
	// someone installed a package with the same id that is not the official one
	third := fakeOfficial("custom-code", false, nil)
	third["manifest.json"] = strings.Replace(third["manifest.json"], `"legacy":"custom-code",`, ``, 1)
	mustUpload(t, e.admin, third)
	e.mig.RunOnce(context.Background())
	_, it := e.items(t)
	if it["custom-code"]["status"] != "failed" || !strings.Contains(it["custom-code"]["error"].(string), "not an official") {
		t.Fatalf("third-party package = %v", it["custom-code"])
	}
	if pkgOf(t, e.admin, "custom-code")["active"] != false {
		t.Fatal("third-party package was activated")
	}
	if e.store.downloads.Load() != 0 {
		t.Fatal("downloaded over an installed package")
	}
}

func TestLegacyRoutesNeedManageOptions(t *testing.T) {
	e := newLegacyEnv(t)
	_, ed := setupUser(t, e.pluginEnv, e.admin, "editor")
	for _, rt := range [][2]string{{"GET", "/api/v1/legacy-migrations"}, {"POST", "/api/v1/legacy-migrations/retry"}, {"POST", "/api/v1/legacy-migrations/upload"}} {
		if code, _ := ed.do(rt[0], rt[1], nil); code != 403 {
			t.Errorf("editor %s %s: %d", rt[0], rt[1], code)
		}
		if code, _ := e.c.do(rt[0], rt[1], nil); code != 401 {
			t.Errorf("anonymous %s %s: %d", rt[0], rt[1], code)
		}
	}
	waiting, it := e.items(t)
	if waiting != 0 || len(it) != 0 {
		t.Fatalf("fresh site = %d %v", waiting, it)
	}
}

// The real official packages (plugins-official/*) through the same migration:
// a site from before the upgrade with data in every old table.
func TestLegacyMigrationRealPackages(t *testing.T) {
	ids := []string{"analytics", "seo", "forms", "redirects", "cookie-consent", "maintenance", "custom-code"}
	e := newLegacyEnv(t, ids...)
	version := map[string]string{} // each official package's own manifest version (they are released separately)
	for _, id := range ids {
		files := dirFiles(t, filepath.Join("..", "..", "..", "plugins-official", id))
		var mf struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal([]byte(files["manifest.json"]), &mf); err != nil || mf.Version == "" {
			t.Fatalf("%s manifest version: %v", id, err)
		}
		version[id] = mf.Version
		e.store.put(t, files)
		if err := legacy.CreateOldTables(e.st.DB, id); err != nil {
			t.Fatal(err)
		}
	}
	var postID int64
	if err := e.st.DB.QueryRow(`SELECT id FROM posts ORDER BY id LIMIT 1`).Scan(&postID); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO analytics_daily (day, path, title, views) VALUES ('2024-05-01', '/', 'Home', 12), ('2024-05-02', '/a', 'A', 3)`)
	e.exec(t, `INSERT INTO analytics_visitors (day, hash) VALUES ('2024-05-01', 'abcdef0123456789')`)
	e.exec(t, `INSERT INTO analytics_page_visitors (day, path, hash) VALUES ('2024-05-01', '/', 'abcdef0123456789')`)
	e.exec(t, `INSERT INTO analytics_referrers (day, host, views) VALUES ('2024-05-01', 'google.com', 5)`)
	e.exec(t, `INSERT INTO seo_meta (post_id, title, description, image, noindex) VALUES (?, 'Custom title', NULL, '', TRUE)`, postID)
	e.exec(t, `INSERT INTO forms_forms (id, name, fields, submit_label, success_message) VALUES
		(7, 'Contact', '[{"name":"email","label":"Email","type":"email","required":true},{"name":"msg","label":"Message","type":"textarea"}]', 'Send', 'Thanks')`)
	e.exec(t, `INSERT INTO forms_submissions (id, form_id, data, is_read) VALUES (1, 7, '{"email":"a@b.co","msg":"hi"}', FALSE), (2, 7, '{"email":"c@d.co","msg":"yo"}', TRUE)`)
	e.exec(t, `INSERT INTO redirects_rules (id, from_path, to_url, code, hits) VALUES (1, '/old', '/new', 301, 4), (2, '/x', 'https://example.com/', 302, 0)`)
	e.exec(t, `INSERT INTO redirects_404 (path, hits) VALUES ('/gone', 9)`)
	e.option(t, "active_plugins", `["analytics","seo","forms","redirects","cookie-consent","custom-code"]`)
	e.option(t, "plugin_settings.seo", `{"titleTemplate":"%title% | %site%","twitterHandle":"@anasaki"}`)
	e.option(t, "plugin_settings.cookie-consent", `{"message":"เราใช้คุกกี้","position":"top"}`)
	e.option(t, "plugin_settings.custom-code", `{"headCode":"<meta name=\"x\" content=\"1\">"}`)
	e.option(t, "plugin_settings.maintenance", `{"enabled":true,"title":"ปิดปรับปรุง","bypassKey":"k-123"}`)

	e.mig.RunOnce(context.Background())
	waiting, it := e.items(t)
	if waiting != 0 {
		for id, m := range it {
			t.Logf("%s: %v %v", id, m["status"], m["error"])
		}
		t.Fatalf("waiting = %d", waiting)
	}
	want := map[string]float64{"analytics": 5, "seo": 1, "forms": 3, "redirects": 3}
	for _, id := range ids {
		m := it[id]
		if m["status"] != "done" || m["version"] != version[id] || m["imported"].(float64) != want[id] {
			t.Errorf("%s = %v", id, m)
		}
		p := pkgOf(t, e.admin, id)
		if p == nil || p["active"] != (id != "maintenance") || p["legacy"] != id {
			t.Errorf("%s package = %v", id, p)
		}
	}
	if s := pkgOf(t, e.admin, "seo")["settings"].(map[string]any); s["titleTemplate"] != "%title% | %site%" || s["twitterHandle"] != "@anasaki" {
		t.Errorf("seo settings = %v", s)
	}
	if s := pkgOf(t, e.admin, "cookie-consent")["settings"].(map[string]any); s["message"] != "เราใช้คุกกี้" || s["position"] != "top" || s["acceptText"] == "" {
		t.Errorf("cookie settings = %v", s)
	}
	if s := pkgOf(t, e.admin, "maintenance")["settings"].(map[string]any); s["enabled"] != true || s["bypassKey"] != "k-123" {
		t.Errorf("maintenance settings = %v", s)
	}
	if s := pkgOf(t, e.admin, "custom-code")["settings"].(map[string]any); !strings.Contains(fmt.Sprint(s["headCode"]), `name="x"`) {
		t.Errorf("custom-code settings = %v", s)
	}
	// seo keeps per-post values as post meta "seo.*" (no tables of its own)
	var title string
	if err := e.st.DB.QueryRow(`SELECT JSON_UNQUOTE(value) FROM post_meta WHERE post_id = ? AND meta_key = 'seo.title'`, postID).Scan(&title); err != nil || title != "Custom title" {
		t.Errorf("seo.title post meta = %q %v", title, err)
	}
	// behaviour after the move: the redirect rule works, the old tables are still there
	rec := e.c.send("GET", "/old", nil, nil)
	if rec.Code != 301 || rec.Header().Get("Location") != "/new" {
		t.Errorf("GET /old after migration = %d %v", rec.Code, rec.Header())
	}
	if !tableExists(t, e.pluginEnv, "redirects_rules") || !tableExists(t, e.pluginEnv, "forms_submissions") {
		t.Error("old tables dropped")
	}
	// a second run is a no-op, a forced full re-import does not duplicate (seen via imported rows in the plugin)
	e.exec(t, `UPDATE legacy_migrations SET status = 'failed', next_attempt_at = NULL, cursor_table = 0, cursor_offset = 0`)
	e.mig.RunOnce(context.Background())
	if waiting, it = e.items(t); waiting != 0 {
		t.Fatalf("re-run waiting = %d %v", waiting, it)
	}
	rec = e.c.send("GET", "/old", nil, nil)
	if rec.Code != 301 {
		t.Errorf("GET /old after re-import = %d", rec.Code)
	}
}
