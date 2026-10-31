package api

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/legacy"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Demo mode (DEMO_MODE=true): JS plugins installed from the Store run (the
// former built-ins are Store plugins since phase 7); Site Code and JS
// plugins from an uploaded zip stay off; mail.send / http.webhook stay off;
// safe mode and SITE_CODE_DISABLED still switch everything off.

const demoStoreCode = `
site.route('GET /st/hi', { access: ['*'] }, () => 'store hi')
site.route('GET /st/mail', { access: ['*'] }, () => { try { mail.send({ to: 'a@b.co', subject: 'x', text: 'y' }); return 'sent' } catch (e) { return 'mail: ' + e.message } })
site.route('GET /st/hook', { access: ['*'] }, () => { try { http.webhook('https://example.com/h', 'st.ping', {}); return 'queued' } catch (e) { return 'hook: ' + e.message } })
site.route('GET /st/ticks', { access: ['*'] }, () => String(db.docs('ticks').find({}).total))
site.middleware((ctx) => ctx.request.path === '/st-mw' ? 'store mw' : undefined)
site.head(() => '<meta name="x-store" content="yes">')
site.cron('* * * * *', () => { db.docs('ticks').insert({ at: 1 }) })
site.adminPage('pg', { label: 'Store page', render: () => '<p>store page body</p>' })
`

func demoStorePkg(t *testing.T, id string) map[string]string {
	return jsPlugin(t, map[string]any{"id": id, "code": map[string]any{"entry": "code/main.ts",
		"scopes": []any{"routes", "middleware", "head-scripts", "jobs", "db", "admin-pages", "mail", "webhooks"}, "routes": []any{"/st"}}},
		map[string]string{"code/main.ts": demoStoreCode})
}

func demoUploadPkg(t *testing.T, id, prefix string) map[string]string {
	return jsPlugin(t, map[string]any{"id": id, "code": map[string]any{"entry": "code/main.ts",
		"scopes": []any{"routes", "middleware", "head-scripts"}, "routes": []any{prefix}}},
		map[string]string{"code/main.ts": fmt.Sprintf(`
site.route('GET %[1]s/hi', { access: ['*'] }, () => 'uploaded hi')
site.middleware((ctx) => ctx.request.path === '%[1]s-mw' ? 'uploaded mw' : undefined)
site.head(() => '<meta name="x-uploaded" content="yes">')
`, prefix)})
}

func installStore(t *testing.T, e *pluginEnv, files map[string]string) {
	t.Helper()
	if _, err := e.pm.InstallPackage(zipBytes(t, files), plugin.SourceStore, ""); err != nil {
		t.Fatalf("store install: %v", err)
	}
}

// publishSiteCode publishes Site Code with a second, non-demo engine on the
// same database (the editor API is off in demo mode).
func publishSiteCode(t *testing.T, e *pluginEnv, src string) {
	t.Helper()
	e2 := sitecode.New(e.st, sitecode.Config{DSN: os.Getenv("TEST_DB_DSN"), CodeDSN: os.Getenv("TEST_CODE_DB_DSN")})
	defer e2.Close()
	if _, err := e2.DB.PutFile("main.ts", src, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e2.Publish("site", nil, 1, false); err != nil {
		t.Fatalf("publish site code: %v", err)
	}
}

func unitsOf(t *testing.T, admin *client) map[string]map[string]any {
	t.Helper()
	code, st := admin.do("GET", "/api/v1/code/status", nil)
	if code != 200 {
		t.Fatalf("status: %d %v", code, st)
	}
	out := map[string]map[string]any{}
	for _, x := range st["units"].([]any) {
		m := x.(map[string]any)
		out[m["id"].(string)] = m
	}
	return out
}

func TestDemoStorePlugins(t *testing.T) {
	e := newPluginAPIWith(t, func(c *sitecode.Config) { c.Demo = true })
	c := e.c
	t.Cleanup(e.code.WaitAfterResponse)
	admin, _ := setupSite(t, c)
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "About", "slug": "about", "status": "publish", "content": "<p>hi</p>"}); code != 201 {
		t.Fatalf("page: %d %v", code, b)
	}
	publishSiteCode(t, e, `
site.route('GET /sitex', { access: ['*'] }, () => 'site code ran')
site.middleware((ctx) => ctx.request.path === '/site-mw' ? 'site mw' : undefined)
site.head(() => '<meta name="x-site" content="yes">')
`)

	// ---- a Store plugin activates and runs
	installStore(t, e, demoStorePkg(t, "stp"))
	activate(t, admin, "stp")
	if code, body := c.public("GET", "/st/hi"); code != 200 || body != "store hi" {
		t.Fatalf("store route: %d %q", code, body)
	}
	if code, body := c.public("GET", "/st-mw"); code != 200 || body != "store mw" {
		t.Fatalf("store middleware: %d %q", code, body)
	}
	h := c.send("GET", "/about", nil, nil).Body.String()
	if !strings.Contains(h, `<meta name="x-store" content="yes">`) {
		t.Fatalf("store head hook missing:\n%s", h)
	}
	// mail.send and http.webhook stay off on the demo site
	if _, body := c.public("GET", "/st/mail"); !strings.Contains(body, "ปิดในเว็บทดลอง") {
		t.Fatalf("mail in demo: %q", body)
	}
	if _, body := c.public("GET", "/st/hook"); !strings.Contains(body, "ปิดในเว็บทดลอง") {
		t.Fatalf("webhook in demo: %q", body)
	}
	// cron + jobs
	if n := e.code.CronTick(time.Now()); n != 1 {
		t.Fatalf("cron tick started %d", n)
	}
	if n := e.code.ProcessJobs(context.Background(), 10); n != 1 {
		t.Fatalf("processed %d jobs", n)
	}
	if _, body := c.public("GET", "/st/ticks"); body != "1" {
		t.Fatalf("cron job did not run: %q", body)
	}
	// admin page
	code, tk := admin.do("POST", "/api/v1/code/admin-pages/stp:pg/ticket", map[string]any{})
	u, _ := tk["url"].(string)
	if code != 200 || u == "" {
		t.Fatalf("admin page ticket: %d %v", code, tk)
	}
	if rec := c.send("GET", u, nil, nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "store page body") {
		t.Fatalf("admin page: %d %s", rec.Code, rec.Body.String())
	}
	if _, b := admin.do("GET", "/api/v1/code/bindings", nil); len(b["adminPages"].([]any)) != 1 {
		t.Fatalf("bindings in demo: %v", b)
	}
	if code, _ := admin.do("GET", "/api/v1/code/bindings?draft=1", nil); code != 403 {
		t.Fatalf("draft bindings in demo: %d", code)
	}

	// ---- Site Code does not run
	if code, _ := c.public("GET", "/sitex"); code != 404 {
		t.Fatalf("site code route ran in demo: %d", code)
	}
	if _, body := c.public("GET", "/site-mw"); body == "site mw" {
		t.Fatal("site code middleware ran in demo")
	}
	if strings.Contains(h, "x-site") {
		t.Fatal("site code head hook ran in demo")
	}
	// the editor / publish API stays off
	if code, b := admin.do("PUT", "/api/v1/code/files/main.ts", map[string]any{"content": "x"}); code != 403 || b["error"] != "ปิดในเว็บทดลอง" {
		t.Fatalf("editor in demo: %d %v", code, b)
	}
	if code, _ := admin.do("POST", "/api/v1/code/publish", map[string]any{"message": "m"}); code != 403 {
		t.Fatalf("publish in demo: %d", code)
	}

	// ---- an uploaded plugin is refused
	mustUpload(t, admin, demoUploadPkg(t, "upp", "/up"))
	code, b := admin.do("POST", "/api/v1/package-plugins/upp/activate", nil)
	if code == 200 || !strings.Contains(fmt.Sprint(b), "เฉพาะปลั๊กอินจาก Store") {
		t.Fatalf("uploaded plugin activated in demo: %d %v", code, b)
	}
	if p := pkgOf(t, admin, "upp"); p["active"] == true {
		t.Fatalf("uploaded plugin active: %v", p)
	}
	// an uploaded plugin that is already active (e.g. switched on before the
	// site became a demo) does not run either
	installStore(t, e, demoUploadPkg(t, "old", "/old"))
	activate(t, admin, "old")
	if code, body := c.public("GET", "/old/hi"); code != 200 || body != "uploaded hi" {
		t.Fatalf("old as a store plugin: %d %q", code, body)
	}
	if _, err := e.st.DB.Exec(`UPDATE plugin_packages SET source = 'upload' WHERE id = 'old'`); err != nil {
		t.Fatal(err)
	}
	mustUpload(t, admin, demoUploadPkg(t, "upq", "/upq")) // reloads the package cache
	e.code.UnitsChanged()
	if code, _ := c.public("GET", "/old/hi"); code != 404 {
		t.Fatalf("active uploaded plugin ran in demo: %d", code)
	}
	if _, body := c.public("GET", "/old-mw"); body == "uploaded mw" {
		t.Fatal("active uploaded middleware ran in demo")
	}
	if h := c.send("GET", "/about", nil, nil).Body.String(); strings.Contains(h, "x-uploaded") || !strings.Contains(h, "x-store") {
		t.Fatalf("head hooks in demo:\n%s", h)
	}
	// status reflects what runs
	units := unitsOf(t, admin)
	if s := units["site"]; s["loaded"] != false || s["error"] != "ปิดในเว็บทดลอง" {
		t.Fatalf("site unit status: %v", s)
	}
	if s := units["stp"]; s["loaded"] != true {
		t.Fatalf("store unit status: %v", s)
	}
	if s := units["old"]; s["loaded"] != false || !strings.Contains(fmt.Sprint(s["error"]), "เฉพาะปลั๊กอินจาก Store") {
		t.Fatalf("uploaded unit status: %v", s)
	}
	if code, st := admin.do("GET", "/api/v1/code/status?unit=stp", nil); code != 200 || st["unit"] != "stp" ||
		len(st["registered"].(map[string]any)["routes"].([]any)) != 4 {
		t.Fatalf("store unit status: %d %v", code, st)
	}

	// ---- safe mode stops everything
	if err := e.code.SetSafeMode(true); err != nil {
		t.Fatal(err)
	}
	if code, _ := c.public("GET", "/st/hi"); code != 404 {
		t.Fatalf("store route in safe mode: %d", code)
	}
	if _, body := c.public("GET", "/st-mw"); body == "store mw" {
		t.Fatal("store middleware in safe mode")
	}
	if strings.Contains(c.send("GET", "/about", nil, nil).Body.String(), "x-store") {
		t.Fatal("store head hook in safe mode")
	}
	if n := e.code.CronTick(time.Now().Add(time.Minute)); n != 0 {
		t.Fatalf("cron in safe mode: %d", n)
	}
	if _, l := admin.doAny("GET", "/api/v1/code/admin-pages", nil); len(l.([]any)) != 0 {
		t.Fatalf("admin pages in safe mode: %v", l)
	}
	if err := e.code.SetSafeMode(false); err != nil {
		t.Fatal(err)
	}
	if code, _ := c.public("GET", "/st/hi"); code != 200 {
		t.Fatalf("store route after safe mode: %d", code)
	}
}

func TestDemoEnvDisabledStopsStorePlugins(t *testing.T) {
	e := newPluginAPIWith(t, func(c *sitecode.Config) { c.Demo, c.Disabled = true, true })
	admin, _ := setupSite(t, e.c)
	installStore(t, e, demoStorePkg(t, "stp"))
	_, _ = e.pm.SetPackageActive("stp", true) // may or may not activate; nothing must run either way
	e.code.UnitsChanged()
	if code, _ := e.c.public("GET", "/st/hi"); code != 404 {
		t.Fatalf("store route with SITE_CODE_DISABLED: %d", code)
	}
	if _, body := e.c.public("GET", "/st-mw"); body == "store mw" {
		t.Fatal("store middleware with SITE_CODE_DISABLED")
	}
	if n := e.code.CronTick(time.Now()); n != 0 {
		t.Fatalf("cron with SITE_CODE_DISABLED: %d", n)
	}
	if code, b := admin.do("POST", "/api/v1/code/admin-pages/stp:pg/ticket", map[string]any{}); code != 404 {
		t.Fatalf("admin page with SITE_CODE_DISABLED: %d %v", code, b)
	}
}

// the legacy migration completes on a demo site for official Store packages
func TestDemoLegacyMigration(t *testing.T) {
	e := newLegacyEnvWith(t, func(c *sitecode.Config) { c.Demo = true }, "forms")
	if err := legacy.CreateOldTables(e.st.DB, "forms"); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO forms_forms (id, name, fields, created_at) VALUES (1, 'Contact', '[]', '2024-01-01 00:00:00')`)
	e.exec(t, `INSERT INTO forms_submissions (id, form_id, data, is_read, created_at) VALUES (1, 1, '{"n":1}', false, '2024-02-01 00:00:00'),
		(2, 1, '{"n":2}', true, '2024-02-01 00:00:00')`)
	e.option(t, "active_plugins", `["forms"]`)
	e.store.put(t, fakeOfficial("forms", true, nil))
	e.mig.RunOnce(context.Background())
	_, it := e.items(t)
	if m := it["forms"]; m["status"] != "done" || m["source"] != "store" || m["error"] != "" || m["imported"].(float64) != 3 {
		t.Fatalf("demo legacy migration = %v", m)
	}
	if p := pkgOf(t, e.admin, "forms"); p["active"] != true {
		t.Fatalf("forms package = %v", p)
	}
	if st := e.stats(t, "forms", ""); st["rows"].(float64) != 3 {
		t.Fatalf("forms stats = %v", st)
	}
}
