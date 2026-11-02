package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

// Site Code phase 4b: JavaScript plugins as code units, end to end through
// the package API (install, requires, activation, migrations, uninstall),
// interop with Site Code and other plugins, scopes, plugins.call to service
// and WASM plugins, the event bridge, hooks and export-as-plugin.

type pluginEnv struct {
	c    *client
	code *sitecode.Engine
	pm   *plugin.Manager
	st   *store.Store
	api  *API // set by newPluginAPI
}

// newPluginAPI is newCodeAPI with the plugin manager wired to the engine
// (JS plugins) like cmd/server.
func newPluginAPI(t *testing.T) *pluginEnv { t.Helper(); return newPluginAPIWith(t, nil) }

// newPluginAPIWith is newPluginAPI with the engine config tweaked (demo mode, …).
func newPluginAPIWith(t *testing.T, tweak func(*sitecode.Config)) *pluginEnv {
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
	t.Setenv("PLUGINS_DIR", t.TempDir())
	st := store.New(conn)
	up := t.TempDir()
	// clear first: the manager loads plugin_packages when it is created (and creates the plugin
	// tables on a fresh database, so a missing table here just means there is nothing to clear)
	for _, tbl := range []string{"users", "posts", "post_terms", "terms", "revisions", "media", "menus", "components", "options", "roles",
		"script_files", "script_versions", "script_logs", "script_audit", "script_access",
		"post_meta", "script_settings", "script_jobs", "script_cron", "plugin_packages", "plugin_kv"} {
		if _, err := conn.Exec("DELETE FROM " + tbl); err != nil && !strings.Contains(err.Error(), "1146") {
			t.Fatal(err)
		}
	}
	pm, err := plugin.NewManager(st, up)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Roles(st); err != nil {
		t.Fatal(err)
	}
	web := site.New(st, pm)
	pm.SetPages(web) // as cmd/server: route results {page} use the site's layout
	authn := auth.New("test-secret")
	resetCodeDB(t, conn, os.Getenv("TEST_CODE_DB_DSN"))
	cfg := sitecode.Config{Auth: authn, Services: pm.ServicePublicPaths, PostURL: site.PostURL,
		DSN: dsn, CodeDSN: os.Getenv("TEST_CODE_DB_DSN"), Emit: pm.Emit, TypeSlugs: site.SetTypeSlugs,
		SecretKey: "test-site-code-key", KeyDir: t.TempDir(), Plugins: pm}
	if tweak != nil {
		tweak(&cfg)
	}
	code := sitecode.New(st, cfg)
	t.Cleanup(code.Close)
	pm.SetCodeRuntime(code)
	code.RouteTimeout, code.EventTimeout = 700*time.Millisecond, 3*time.Second
	web.SetCode(code.ServePublic)
	web.SetCodeSessions(code.Session)
	web.SetCodeHooks(code)
	web.SetPostTypes(func(opts map[string]string) []site.PostTypeRoute { // as cmd/server: Site Code + plugin post types
		var out []site.PostTypeRoute
		for _, pt := range code.ContentFor(opts).PostTypes {
			out = append(out, site.PostTypeRoute{Type: pt.Type, Slug: pt.Slug, Public: pt.Public, Label: pt.Label})
		}
		return out
	})
	pm.Listen(code.OnEvent)
	mux := http.NewServeMux()
	a := New(st, authn, up, pm, web)
	a.SetSiteCode(code)
	a.Routes(mux)
	mux.Handle("/", pm.Middleware(web))
	t.Cleanup(code.WaitEvents)
	return &pluginEnv{c: &client{t: t, h: mux}, code: code, pm: pm, st: st, api: a}
}

// dirFiles reads an example plugin folder.
func dirFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func examplePlugin(t *testing.T, name string) map[string]string {
	return dirFiles(t, filepath.Join("..", "..", "..", "examples", "js-plugins", name))
}

// jsPlugin builds a JS plugin package from a manifest (as a map) and files.
func jsPlugin(t *testing.T, manifest map[string]any, files map[string]string) map[string]string {
	t.Helper()
	m := map[string]any{"name": manifest["id"], "version": "1.0.0", "runtime": "js", "permissions": []any{"code"}}
	for k, v := range manifest {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	out := map[string]string{"manifest.json": string(b)}
	for k, v := range files {
		out[k] = v
	}
	return out
}

func uploadPkg(t *testing.T, c *client, query string, files map[string]string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "p.zip")
	_, _ = fw.Write(zipBytes(t, files))
	_ = mw.Close()
	rec := c.send("POST", "/api/v1/package-plugins/upload"+query, &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func mustUpload(t *testing.T, c *client, files map[string]string) {
	t.Helper()
	if code, b := uploadPkg(t, c, "", files); code != 200 {
		t.Fatalf("upload: %d %v", code, b)
	}
}

func activate(t *testing.T, c *client, id string) {
	t.Helper()
	if code, b := c.do("POST", "/api/v1/package-plugins/"+id+"/activate", nil); code != 200 || b["active"] != true {
		t.Fatalf("activate %s: %d %v", id, code, b)
	}
}

func pkgOf(t *testing.T, c *client, id string) map[string]any {
	t.Helper()
	_, list := c.doAny("GET", "/api/v1/package-plugins", nil)
	for _, x := range list.([]any) {
		if m := x.(map[string]any); m["id"] == id {
			return m
		}
	}
	return nil
}

// codeTable reports whether a physical table exists in the code database.
func codeTable(t *testing.T, e *pluginEnv, name string) bool {
	t.Helper()
	var n int
	if err := e.st.DB.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`,
		e.code.Data().Database, e.code.Data().N.Prefix+name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func logsOf(t *testing.T, c *client, unit string) string {
	t.Helper()
	_, b := c.do("GET", "/api/v1/code/logs?perPage=200&unit="+unit, nil)
	var sb strings.Builder
	for _, x := range b["items"].([]any) {
		m := x.(map[string]any)
		if m["unit"] != unit {
			c.t.Fatalf("log of unit %v in ?unit=%s", m["unit"], unit)
		}
		fmt.Fprintf(&sb, "%s | %s\n", m["source"], m["message"])
	}
	return sb.String()
}

func getJSON(t *testing.T, c *client, method, path, body string, hdr map[string]string) (int, map[string]any) {
	t.Helper()
	h := map[string]string{"Accept": "application/json", "Content-Type": "application/json"}
	for k, v := range hdr {
		h[k] = v
	}
	rec := c.send(method, path, strings.NewReader(body), h)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// ---------------------------------------------------------------- the example plugins + Site Code

func TestJSPluginLifecycle(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	admin, _ := setupSite(t, c)

	// requires: shop needs money
	code, b := uploadPkg(t, admin, "", examplePlugin(t, "shop"))
	if code != 400 || b["missing"] == nil {
		t.Fatalf("shop without money: %d %v", code, b)
	}
	miss := b["missing"].([]any)[0].(map[string]any)
	if miss["id"] != "money" || miss["range"] != "^1.0.0" {
		t.Fatalf("missing = %v", miss)
	}
	mustUpload(t, admin, examplePlugin(t, "money"))
	mustUpload(t, admin, examplePlugin(t, "shop"))
	shop := pkgOf(t, admin, "shop")
	if shop["runtime"] != "js" || shop["requires"].(map[string]any)["money"] != "^1.0.0" ||
		strs(shop["code"].(map[string]any)["scopes"]) != "db,jobs,routes" || shop["active"] != false {
		t.Fatalf("shop listing = %v", shop)
	}
	if strs(pkgOf(t, admin, "money")["dependents"]) != "shop" {
		t.Fatalf("money dependents = %v", pkgOf(t, admin, "money")["dependents"])
	}
	// the dependency must be active first
	code, b = admin.do("POST", "/api/v1/package-plugins/shop/activate", nil)
	if code != 400 || b["missing"].([]any)[0].(map[string]any)["active"] != false {
		t.Fatalf("activate shop before money: %d %v", code, b)
	}
	activate(t, admin, "money")
	activate(t, admin, "shop")
	if !codeTable(t, e, "p_shop_orders") {
		t.Fatal("migration did not create p_shop_orders")
	}
	code, b = getJSON(t, c, "GET", "/shop", "", nil)
	if code != 200 || b["orders"].(float64) != 0 {
		t.Fatalf("GET /shop: %d %v", code, b)
	}
	p0 := b["products"].([]any)[0].(map[string]any)
	if p0["label"] != "฿250.00" || p0["usd"].(float64) != 7.14 {
		t.Fatalf("product = %v", p0)
	}
	// money's own setting, read by its export (runs in money's context)
	if code, b := admin.do("PUT", "/api/v1/code/settings", map[string]any{"unit": "money",
		"values": []any{map[string]any{"key": "usdRate", "value": "25", "secret": false}}}); code != 200 {
		t.Fatalf("settings: %d %v", code, b)
	}
	_, sv := admin.do("GET", "/api/v1/code/settings?unit=money", nil)
	if v := sv["values"].([]any); len(v) != 1 || v[0].(map[string]any)["key"] != "usdRate" || v[0].(map[string]any)["unit"] != "money" {
		t.Fatalf("money settings = %v", sv)
	}
	if _, sv := admin.do("GET", "/api/v1/code/settings?unit=site", nil); len(sv["values"].([]any)) != 0 {
		t.Fatalf("site settings show plugin keys: %v", sv)
	}
	if _, b := getJSON(t, c, "GET", "/shop", "", nil); b["products"].([]any)[0].(map[string]any)["usd"].(float64) != 10 {
		t.Fatalf("rate after setting: %v", b)
	}
	if code, b := getJSON(t, c, "POST", "/shop/order", `{"id":1}`, nil); code != 200 || b["id"] == nil {
		t.Fatalf("order: %d %v", code, b)
	}

	// Site Code: imports the money library and shop's export, filters
	// shop.price, listens to shop.order.paid, cannot touch p_shop_orders
	publishCode(t, admin, map[string]string{"main.ts": `
import { baht } from 'money';
import { cartTotal } from 'shop';
site.filter('shop.price', (p) => p * 2);
site.on('shop.order.paid', (o) => log.info('paid ' + o.item + ' ' + o.total));
site.route('GET /total', { access: ['*'] }, () => ({ total: cartTotal(), label: baht(cartTotal()) }));
site.route('GET /steal', { access: ['*'] }, () => { try { return db.table('p_shop_orders').count() } catch (e) { return { err: e.message } } });
site.route('GET /secret', { access: ['*'] }, () => ({ v: site.settings('money.usdRate') }));
`})
	if _, b := getJSON(t, c, "GET", "/shop", "", nil); b["products"].([]any)[0].(map[string]any)["price"].(float64) != 500 {
		t.Fatalf("site filter on shop.price: %v", b)
	}
	if code, _ := getJSON(t, c, "POST", "/shop/order", `{"id":1}`, nil); code != 200 {
		t.Fatal("order 2")
	}
	e.code.WaitEvents()
	if l := logsOf(t, admin, "site"); !strings.Contains(l, "paid Mug 500") {
		t.Fatalf("site.on(shop.order.paid) not delivered:\n%s", l)
	}
	if _, b := getJSON(t, c, "GET", "/total", "", nil); b["total"].(float64) != 750 || b["label"] != "฿750.00" {
		t.Fatalf("/total = %v", b)
	}
	if _, b := getJSON(t, c, "GET", "/steal", "", nil); !strings.Contains(fmt.Sprint(b["err"]), "ปลั๊กอินอื่น") {
		t.Fatalf("site read a plugin table: %v", b)
	}
	if _, b := getJSON(t, c, "GET", "/secret", "", nil); b["v"] != nil {
		t.Fatalf("site read a plugin setting: %v", b)
	}

	// admin views per unit
	_, st := admin.do("GET", "/api/v1/code/status?unit=shop", nil)
	if st["unit"] != "shop" || len(st["registered"].(map[string]any)["routes"].([]any)) != 2 {
		t.Fatalf("status shop = %v", st)
	}
	_, st = admin.do("GET", "/api/v1/code/status", nil)
	units := st["units"].([]any)
	if units[0].(map[string]any)["id"] != "site" || len(units) != 3 {
		t.Fatalf("units = %v", units)
	}
	_, acc := admin.doAny("GET", "/api/v1/code/access?unit=shop", nil)
	names := []string{}
	for _, x := range acc.([]any) {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "shop:GET /shop,shop:POST /shop/order,shop:products,shop:order" {
		t.Fatalf("access rows = %v", names)
	}
	_, bind := admin.do("GET", "/api/v1/code/bindings", nil)
	found := false
	for _, x := range bind["collections"].([]any) {
		found = found || x.(map[string]any)["name"] == "shop:products"
	}
	if !found {
		t.Fatalf("bindings = %v", bind["collections"])
	}
	_, pv := admin.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "code:shop:products", "viewAs": "anonymous"})
	if it := pv["items"].([]any); len(it) != 2 || it[0].(map[string]any)["label"] != "฿500.00" {
		t.Fatalf("preview = %v", pv)
	}
	_, hooks := admin.doAny("GET", "/api/v1/code/hooks", nil)
	var price map[string]any
	for _, x := range hooks.([]any) {
		if m := x.(map[string]any); m["name"] == "shop.price" {
			price = m
		}
	}
	if price == nil || price["declaredBy"] != "shop" || price["kind"] != "filter" ||
		price["subscribers"].([]any)[0].(map[string]any)["unit"] != "site" {
		t.Fatalf("hooks = %v", hooks)
	}
	if l := logsOf(t, admin, "money"); !strings.Contains(l, "activate") {
		t.Fatalf("money logs:\n%s", l)
	}

	// dependents: money cannot go while shop needs it
	if code, b := admin.do("POST", "/api/v1/package-plugins/money/deactivate", nil); code != 409 || strs(b["dependents"]) != "shop" {
		t.Fatalf("deactivate money: %d %v", code, b)
	}
	if code, b := admin.do("DELETE", "/api/v1/package-plugins/money", nil); code != 409 || strs(b["dependents"]) != "shop" {
		t.Fatalf("delete money: %d %v", code, b)
	}
	if code, _ := admin.do("POST", "/api/v1/package-plugins/money/deactivate?force=1", nil); code != 200 {
		t.Fatal("force deactivate")
	}
	if pkgOf(t, admin, "shop")["active"] != false {
		t.Fatal("dependent stayed active")
	}
	if code, _ := getJSON(t, c, "GET", "/shop", "", nil); code != 404 {
		t.Fatalf("inactive plugin route answered %d", code)
	}
	activate(t, admin, "money")
	activate(t, admin, "shop")

	// upgrade with a destructive migration: confirmation, then a backup
	v2 := examplePlugin(t, "shop")
	v2["manifest.json"] = strings.Replace(v2["manifest.json"], `"version": "1.0.0"`, `"version": "1.1.0"`, 1)
	v2["migrations/002_drop_item.ts"] = `export default migration({ up(schema) { schema.table('orders', (t) => { t.dropColumn('item'); }); } })`
	v2["code/main.ts"] = strings.ReplaceAll(v2["code/main.ts"], "item: p.name, ", "")
	code, b = uploadPkg(t, admin, "", v2)
	if code != 400 || b["destructive"] == nil {
		t.Fatalf("destructive upgrade: %d %v", code, b)
	}
	if d := b["destructive"].([]any)[0].(map[string]any); d["op"] != "drop_column" || d["migration"] != "shop/002_drop_item" {
		t.Fatalf("destructive = %v", d)
	}
	if pkgOf(t, admin, "shop")["version"] != "1.0.0" {
		t.Fatal("refused upgrade changed the version")
	}
	if code, b := uploadPkg(t, admin, "?confirmDestructive=1", v2); code != 200 || b["version"] != "1.1.0" || b["active"] != true {
		t.Fatalf("confirmed upgrade: %d %v", code, b)
	}
	_, bk := admin.doAny("GET", "/api/v1/code/db/backups", nil)
	if !strings.Contains(fmt.Sprint(bk), "bak_p_shop_orders") {
		t.Fatalf("backups = %v", bk)
	}

	// uninstall with its data (dependents first)
	if code, _ := admin.do("DELETE", "/api/v1/package-plugins/shop?deleteData=1", nil); code != 204 {
		t.Fatal("delete shop")
	}
	if codeTable(t, e, "p_shop_orders") || codeTable(t, e, "p_shop_kv_docs") {
		t.Fatal("plugin tables left after deleteData")
	}
	var n int
	_ = e.code.Data().Database
	if err := e.st.DB.QueryRow(`SELECT COUNT(*) FROM script_access WHERE name LIKE 'shop:%'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("access rows left", err)
	}
	if code, _ := admin.do("DELETE", "/api/v1/package-plugins/money", nil); code != 204 {
		t.Fatal("delete money")
	}
	// Site Code that imports an uninstalled plugin no longer builds
	_, out := admin.do("POST", "/api/v1/code/build", nil)
	if out["ok"] != false || !strings.Contains(fmt.Sprint(out["errors"]), "money") {
		t.Fatalf("build after uninstall = %v", out)
	}
}

// ---------------------------------------------------------------- interop between units

func TestJSPluginInterop(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	admin, _ := setupSite(t, c)
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "alpha",
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"db"}, "exports": []any{"echo", "boom", "slow", "count", "who"}}},
		map[string]string{
			"migrations/001_items.ts": `export default migration({ up(s) { s.create('items', (t) => { t.id(); t.string('name'); }); } })`,
			"code/main.ts": `
site.export('echo', (x) => x);
site.export('boom', () => { throw new Error('alpha broke') });
site.export('slow', () => { for (;;) {} });
site.export('count', () => { db.table('items').insert({ name: 'x' }); return db.table('items').count() });
site.export('who', () => ({ user: ctxUser() }));
function ctxUser() { return null }
site.filter('beta.price', (v) => v + 1);
site.on('beta.done', (d) => log.info('alpha heard ' + d.n));
`}))
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "beta", "requires": map[string]any{"alpha": "^1.0.0"},
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"routes"}, "routes": []any{"/beta"},
			"hooks": map[string]any{"filters": []any{"beta.price"}, "events": []any{"beta.done"}}}},
		map[string]string{"code/main.ts": `
import { echo, boom, slow, count } from 'alpha';
site.filter('beta.price', (v) => v * 10, { priority: 5 });
const j = (f) => { try { return f() } catch (e) { return { err: String(e.message) } } };
site.route('GET /beta/price', { access: ['*'] }, () => ({ v: site.applyFilter('beta.price', 100) }));
site.route('GET /beta/echo', { access: ['*'] }, () => echo({ a: 1, f() { return 1 }, d: new Date(0), u: undefined, nested: [1, { b: 2 }] }));
site.route('GET /beta/boom', { access: ['*'] }, () => j(() => boom()));
site.route('GET /beta/slow', { access: ['*'] }, () => slow());
site.route('GET /beta/count', { access: ['*'] }, () => ({ viaExport: count(), direct: j(() => db.table('items').count()) }));
site.route('GET /beta/emit', { access: ['*'] }, () => { site.emit('beta.done', { n: 7 }); return 'ok' });
site.route('GET /beta/cookie', { access: ['*'] }, ({ request, response }) => { response.setCookie('cart', 'c1'); return request.cookies });
site.route('GET /beta/bad-emit', { access: ['*'] }, () => j(() => site.emit('alpha.x', {})));
`}))
	activate(t, admin, "alpha")
	activate(t, admin, "beta")
	publishCode(t, admin, map[string]string{"main.ts": `site.filter('beta.price', (v) => v * 2);`})

	// filter order: priority, then load order (alpha before beta: requires), Site Code last
	if _, b := getJSON(t, c, "GET", "/beta/price", "", nil); b["v"].(float64) != 2002 {
		t.Fatalf("beta.price = %v (want (100*10+1)*2)", b["v"])
	}
	_, hooks := admin.doAny("GET", "/api/v1/code/hooks", nil)
	var order []string
	for _, x := range hooks.([]any) {
		if m := x.(map[string]any); m["name"] == "beta.price" {
			for _, s := range m["subscribers"].([]any) {
				sm := s.(map[string]any)
				order = append(order, fmt.Sprintf("%s:%v", sm["unit"], sm["priority"]))
			}
		}
	}
	if strings.Join(order, ",") != "beta:5,alpha:10,site:10" {
		t.Fatalf("hook order = %v", order)
	}
	// JSON copy: functions and undefined dropped, Date → ISO string
	if _, b := getJSON(t, c, "GET", "/beta/echo", "", nil); fmt.Sprint(b) != "map[a:1 d:1970-01-01T00:00:00.000Z nested:[1 map[b:2]]]" {
		t.Fatalf("echo = %v", b)
	}
	// errors carry the target's file:line
	if _, b := getJSON(t, c, "GET", "/beta/boom", "", nil); !strings.Contains(fmt.Sprint(b["err"]), "alpha broke") ||
		!strings.Contains(fmt.Sprint(b["err"]), "(alpha: code/main.ts:3:") {
		t.Fatalf("boom = %v", b)
	}
	// the export runs with alpha's scopes and tables; beta has no db scope
	_, b := getJSON(t, c, "GET", "/beta/count", "", nil)
	if b["viaExport"].(float64) != 1 || !strings.Contains(fmt.Sprint(b["direct"]), "ไม่มีสิทธิ์ใช้ db") {
		t.Fatalf("count = %v", b)
	}
	// time: the export uses the caller's remaining time
	start := time.Now()
	if code, _ := getJSON(t, c, "GET", "/beta/slow", "", nil); code != 504 || time.Since(start) > 2*time.Second {
		t.Fatalf("slow export: %d after %s", code, time.Since(start))
	}
	// events from a plugin reach other units
	_, _ = getJSON(t, c, "GET", "/beta/emit", "", nil)
	e.code.WaitEvents()
	if l := logsOf(t, admin, "alpha"); !strings.Contains(l, "alpha heard 7") || !strings.Contains(l, "alpha: event beta.done") {
		t.Fatalf("alpha logs:\n%s", l)
	}
	if _, b := getJSON(t, c, "GET", "/beta/bad-emit", "", nil); !strings.Contains(fmt.Sprint(b["err"]), "beta.") {
		t.Fatalf("emit of another unit's name = %v", b)
	}
	// cookies: own namespace only
	rec := c.send("GET", "/beta/cookie", nil, map[string]string{"Accept": "application/json", "Cookie": "pl_beta_cart=x; pl_site_a=1; pl_alpha_b=2"})
	if strings.TrimSpace(rec.Body.String()) != `{"cart":"x"}` || !strings.HasPrefix(rec.Header().Get("Set-Cookie"), "pl_beta_cart=c1") {
		t.Fatalf("cookie: %s %v", rec.Body.String(), rec.Header())
	}
	// a plugin may not import what it does not require
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "gamma", "code": map[string]any{"entry": "code/main.ts", "scopes": []any{}}},
		map[string]string{"code/main.ts": `import { echo } from 'alpha'; site.on('x.y', () => echo(1));`}))
	code, b := admin.do("POST", "/api/v1/package-plugins/gamma/activate", nil)
	if code != 400 || !strings.Contains(fmt.Sprint(b["activationError"]), "requires") {
		t.Fatalf("gamma: %d %v", code, b)
	}
	if g := pkgOf(t, admin, "gamma"); g["active"] != false || !strings.Contains(fmt.Sprint(g["activationError"]), "requires") {
		t.Fatalf("gamma listing = %v", g)
	}
}

// ---------------------------------------------------------------- scopes

func TestJSPluginScopes(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	admin, _ := setupSite(t, c)
	// runtime APIs outside the granted scopes
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "nosco",
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"routes", "http"}, "routes": []any{"/nosco"}, "hosts": []any{"api.example.com"}}},
		map[string]string{"code/main.ts": `
const j = (f) => { try { f(); return { ok: true } } catch (e) { return { err: String(e.message) } } };
site.route('GET /nosco/{what}', { access: ['*'] }, ({ request }) => j(() => ({
  db: () => db.table('x').count(),
  docs: () => db.docs('x').find({}),
  content: () => db.posts.list({}),
  write: () => db.posts.create({ title: 'x' }),
  system: () => db.asSystem(() => 1),
  mail: () => mail.send({ to: 'a@b.co', subject: 'x', text: 'y' }),
  jobs: () => jobs.enqueue('x'),
  host: () => http.fetch('https://other.example.org/'),
}[request.params.what])()));
`}))
	activate(t, admin, "nosco")
	for what, scope := range map[string]string{"db": "db", "docs": "db", "content": "content", "write": "content-write",
		"system": "content-write", "mail": "mail", "jobs": "jobs"} {
		if _, b := getJSON(t, c, "GET", "/nosco/"+what, "", nil); !strings.Contains(fmt.Sprint(b["err"]), "ไม่มีสิทธิ์ใช้ "+scope) {
			t.Errorf("%s: %v", what, b)
		}
	}
	admin.do("PUT", "/api/v1/code/settings", map[string]any{"fetchHosts": []any{"other.example.org", "api.example.com"}})
	if _, b := getJSON(t, c, "GET", "/nosco/host", "", nil); !strings.Contains(fmt.Sprint(b["err"]), "code.hosts") {
		t.Errorf("host outside code.hosts: %v", b)
	}
	if l := logsOf(t, admin, "nosco"); !strings.Contains(l, "ไม่มีสิทธิ์ใช้ mail") {
		t.Errorf("denials are not logged:\n%s", l)
	}
	// registration-time scopes: the plugin does not activate
	reg := map[string]string{
		"routes":       `site.route('GET /r1', () => 1)`,
		"visitor":      `site.visitor(() => null)`,
		"admin-pages":  `site.adminPage('p', { label: 'P', render: () => 'x' })`,
		"jobs":         `site.job('j', () => 1)`,
		"filters-core": `site.filter('post.content', (h) => h)`,
	}
	i := 0
	for scope, src := range reg {
		i++
		id := "reg" + strconv.Itoa(i) + "-" + strings.ReplaceAll(scope, "-", "")
		mustUpload(t, admin, jsPlugin(t, map[string]any{"id": id, "code": map[string]any{"entry": "code/main.ts", "scopes": []any{}}},
			map[string]string{"code/main.ts": src}))
		code, b := admin.do("POST", "/api/v1/package-plugins/"+id+"/activate", nil)
		if code != 400 || !strings.Contains(fmt.Sprint(b["activationError"]), "ไม่มีสิทธิ์ใช้ "+scope) {
			t.Errorf("%s: %d %v", scope, code, b)
		}
	}
	// with the scopes it works; route outside code.routes is refused
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "withsc", "code": map[string]any{"entry": "code/main.ts",
		"scopes": []any{"routes", "visitor", "admin-pages", "jobs", "filters-core"}, "routes": []any{"/withsc"}}},
		map[string]string{"code/main.ts": `site.route('GET /withsc/a', { access: ['*'] }, () => ({ ok: 1 }));
site.visitor(() => null); site.adminPage('p', { label: 'P', render: () => 'x' }); site.job('j', ({ data }) => data.n * 2);
site.route('GET /withsc/job', { access: ['*'] }, () => ({ id: jobs.enqueue('j', { n: 21 }) }));
site.filter('post.content', (h) => h);`}))
	activate(t, admin, "withsc")
	if code, _ := getJSON(t, c, "GET", "/withsc/a", "", nil); code != 200 {
		t.Fatal("withsc route")
	}
	// a plugin's jobs run in the plugin ("withsc:j"), its admin pages are listed as "withsc:p"
	if _, b := getJSON(t, c, "GET", "/withsc/job", "", nil); b["id"] == nil {
		t.Fatalf("enqueue: %v", b)
	}
	if n := e.code.ProcessJobs(context.Background(), 5); n != 1 {
		t.Fatalf("processed %d jobs", n)
	}
	_, jl := admin.do("GET", "/api/v1/code/jobs?unit=withsc", nil)
	if it := jl["items"].([]any); len(it) != 1 || it[0].(map[string]any)["name"] != "withsc:j" ||
		it[0].(map[string]any)["status"] != "done" || it[0].(map[string]any)["result"].(float64) != 42 || it[0].(map[string]any)["unit"] != "withsc" {
		t.Fatalf("plugin jobs = %v", jl)
	}
	if _, jl := admin.do("GET", "/api/v1/code/jobs?unit=site", nil); len(jl["items"].([]any)) != 0 {
		t.Fatalf("site jobs = %v", jl)
	}
	_, pages := admin.doAny("GET", "/api/v1/code/admin-pages", nil)
	if !strings.Contains(fmt.Sprint(pages), "withsc:p") {
		t.Fatalf("admin pages = %v", pages)
	}
	if code, tk := admin.do("POST", "/api/v1/code/admin-pages/withsc:p/ticket", map[string]any{}); code != 200 {
		t.Fatalf("ticket: %d %v", code, tk)
	} else if rec := c.send("GET", tk["url"].(string), nil, nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "x") {
		t.Fatalf("admin page: %d %s", rec.Code, rec.Body.String())
	}
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "outside", "code": map[string]any{"entry": "code/main.ts",
		"scopes": []any{"routes"}, "routes": []any{"/outside"}}},
		map[string]string{"code/main.ts": `site.route('GET /elsewhere', () => 1)`}))
	if code, b := admin.do("POST", "/api/v1/package-plugins/outside/activate", nil); code != 400 || !strings.Contains(fmt.Sprint(b["error"]), "code.routes") {
		t.Fatalf("route outside prefix: %d %v", code, b)
	}
	// a second site.visitor anywhere is refused
	code, out := admin.do("PUT", "/api/v1/code/files/main.ts", map[string]any{"content": "site.visitor(() => null)", "baseUpdatedAt": ""})
	if code != 200 {
		t.Fatal(out)
	}
	if _, b := admin.do("POST", "/api/v1/code/build", nil); b["ok"] != false || !strings.Contains(fmt.Sprint(b["errors"]), "withsc") {
		t.Fatalf("second visitor: %v", b)
	}
}

// ---------------------------------------------------------------- service / WASM functions and the event bridge

func TestPluginsCallAndEventBridge(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	admin, _ := setupSite(t, c)
	var mu sync.Mutex
	var events []string
	var secret string
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		sec := secret
		mu.Unlock()
		if err := plugin.VerifyServiceSignature(sec, r, body); err != nil {
			http.Error(w, "bad signature", 401)
			return
		}
		switch r.URL.Path {
		case "/functions/monthly":
			var in struct {
				Input   map[string]float64 `json:"input"`
				User    map[string]any     `json:"user"`
				Visitor any                `json:"visitor"`
			}
			_ = json.Unmarshal(body, &in)
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"double": in.Input["n"] * 2, "user": in.User != nil}})
		case "/functions/fail":
			w.WriteHeader(422)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "bad input"})
		case "/events":
			mu.Lock()
			events = append(events, string(body))
			mu.Unlock()
		default:
			w.WriteHeader(404)
		}
	}))
	defer svc.Close()
	b, _ := json.Marshal(map[string]any{"id": "svc", "name": "Svc", "version": "1.0.0", "permissions": []any{"service"},
		"service": map[string]any{"functions": []any{"monthly", "fail"}, "events": []any{"jsp.ping"}, "api": []any{"events"}}})
	mustUpload(t, admin, map[string]string{"manifest.json": string(b)})
	_, inst := admin.do("PUT", "/api/v1/package-plugins/svc/service", map[string]any{"url": svc.URL})
	mu.Lock()
	secret = inst["service"].(map[string]any)["secret"].(string)
	mu.Unlock()
	if pkgOf(t, admin, "svc")["runtime"] != "service" || strs(pkgOf(t, admin, "svc")["functions"]) != "fail,monthly" {
		t.Fatalf("svc listing = %v", pkgOf(t, admin, "svc"))
	}
	activate(t, admin, "svc")
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "jsp", "requires": map[string]any{"svc": "*"},
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"routes"}, "routes": []any{"/jsp"}, "hooks": map[string]any{"events": []any{"jsp.ping"}}}},
		map[string]string{"code/main.ts": `
const j = (f) => { try { return f() } catch (e) { return { err: String(e.message) } } };
site.route('GET /jsp/call', { access: ['*'] }, () => plugins.call('svc', 'monthly', { n: 21 }));
site.route('GET /jsp/fail', { access: ['*'] }, () => j(() => plugins.call('svc', 'fail', {})));
site.route('GET /jsp/none', { access: ['*'] }, () => j(() => plugins.call('svc', 'nope', {})));
site.route('GET /jsp/other', { access: ['*'] }, () => j(() => plugins.call('reports', 'monthly', {})));
site.route('GET /jsp/ping', { access: ['*'] }, () => { site.emit('jsp.ping', { n: 1 }); return 'ok' });
site.on('svc.hello', (d) => log.info('from service ' + d.x));
`}))
	activate(t, admin, "jsp")
	if _, b := getJSON(t, c, "GET", "/jsp/call", "", nil); b["double"].(float64) != 42 || b["user"] != false {
		t.Fatalf("plugins.call = %v", b)
	}
	if _, b := getJSON(t, c, "GET", "/jsp/fail", "", nil); !strings.Contains(fmt.Sprint(b["err"]), "bad input") {
		t.Fatalf("fail = %v", b)
	}
	if _, b := getJSON(t, c, "GET", "/jsp/none", "", nil); !strings.Contains(fmt.Sprint(b["err"]), "service.functions") {
		t.Fatalf("none = %v", b)
	}
	if _, b := getJSON(t, c, "GET", "/jsp/other", "", nil); !strings.Contains(fmt.Sprint(b["err"]), "PluginUnavailable") { // phase 6: not installed
		t.Fatalf("not required = %v", b)
	}
	// Site Code may call any function plugin
	publishCode(t, admin, map[string]string{"main.ts": `site.route('GET /sc-call', { access: ['*'] }, () => plugins.call('svc', 'monthly', { n: 1 }));`})
	if _, b := getJSON(t, c, "GET", "/sc-call", "", nil); b["double"].(float64) != 2 {
		t.Fatalf("site call = %v", b)
	}
	// JS → service: site.emit reaches the subscribed service
	_, _ = getJSON(t, c, "GET", "/jsp/ping", "", nil)
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		got := strings.Join(events, "\n")
		mu.Unlock()
		if strings.Contains(got, `"event":"jsp.ping"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("service did not get jsp.ping: %s", got)
		}
		time.Sleep(50 * time.Millisecond)
	}
	// service → JS: /api/service/events emits svc.hello to site.on in the plugin
	body := []byte(`{"event":"hello","data":{"x":"hi"}}`)
	req := httptest.NewRequest("POST", "/api/service/events", bytes.NewReader(body))
	ts := time.Now().Unix()
	req.Header.Set(plugin.HeaderPlugin, "svc")
	req.Header.Set(plugin.HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(plugin.HeaderSignature, plugin.ServiceSignature(secret, ts, "POST", "/api/service/events", body))
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	if rec.Code/100 != 2 {
		t.Fatalf("service event: %d %s", rec.Code, rec.Body.String())
	}
	e.code.WaitEvents()
	time.Sleep(100 * time.Millisecond)
	e.code.WaitEvents()
	if l := logsOf(t, admin, "jsp"); !strings.Contains(l, "from service hi") {
		t.Fatalf("jsp logs:\n%s", l)
	}
}

func TestPluginsCallWasm(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	admin, _ := setupSite(t, c)
	src := filepath.Join("..", "..", "..", "examples", "service-plugins", "functions-wasm")
	out := filepath.Join(t.TempDir(), "plugin.wasm")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build the WASM example: %v %s", err, b)
	}
	wasmBytes, _ := os.ReadFile(out)
	files := dirFiles(t, filepath.Join(src, "package"))
	files["plugin.wasm"] = string(wasmBytes)
	mustUpload(t, admin, files)
	activate(t, admin, "reports")
	// compile now (the first run would otherwise eat the route's time)
	if code, h := admin.do("GET", "/api/v1/package-plugins/reports/service/health", nil); code != 200 || h["ok"] != true {
		t.Fatalf("health: %d %v", code, h)
	}
	e.code.RouteTimeout = 5 * time.Second
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('GET /report', { access: ['*'] }, () => plugins.call('reports', 'monthly', { sales: [100, 250, 30] }));`})
	code, b := getJSON(t, c, "GET", "/report", "", nil)
	if code != 200 || b["total"].(float64) != 380 || b["count"].(float64) != 3 {
		t.Fatalf("wasm function: %d %v", code, b)
	}
}

// ---------------------------------------------------------------- export Site Code as a plugin

func TestExportSiteCodeAsPlugin(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	admin, _ := setupSite(t, c)
	if code, _ := admin.do("POST", "/api/v1/code/package?dryRun=1", map[string]any{}); code != 400 {
		t.Fatal("export without a live version")
	}
	publishCode(t, admin, map[string]string{
		"main.ts": `import { label } from './lib/label';
site.filter('site.greet', (s) => s + '!');
site.route('GET /xp/hello', { access: ['*'] }, ({ request }) => ({ g: site.applyFilter('site.greet', label('hi')), cookies: request.cookies,
  notes: db.table('notes').count() }));
site.export('shout', (s) => String(s).toUpperCase());`,
		"lib/label.ts":            `export const label = (s: string) => '<' + s + '>';`,
		"migrations/001_notes.ts": `export default migration({ up(s) { s.create('notes', (t) => { t.id(); t.text('body'); }); } })`,
	})
	code, dry := admin.do("POST", "/api/v1/code/package?dryRun=1", map[string]any{})
	if code != 200 {
		t.Fatalf("dry run: %d %v", code, dry)
	}
	m := dry["manifest"].(map[string]any)
	cd := m["code"].(map[string]any)
	if m["runtime"] != "js" || cd["entry"] != "code/main.ts" || strs(cd["scopes"]) != "db,routes" || strs(cd["routes"]) != "/xp" ||
		strs(cd["exports"]) != "shout" {
		t.Fatalf("prefilled manifest = %v", m)
	}
	req := map[string]any{"id": "xpkg", "name": "Exported", "version": "1.0.0", "description": "from site code"}
	_, dry = admin.do("POST", "/api/v1/code/package?dryRun=1", req)
	cd = dry["manifest"].(map[string]any)["code"].(map[string]any)
	if strs(cd["hooks"].(map[string]any)["filters"]) != "xpkg.greet" || !strings.Contains(fmt.Sprint(dry["warnings"]), "xpkg.greet") {
		t.Fatalf("hooks = %v warnings %v", cd, dry["warnings"])
	}
	if code, b := admin.do("POST", "/api/v1/code/package?dryRun=1", map[string]any{"id": "site"}); code != 400 {
		t.Fatalf("id site: %d %v", code, b)
	}
	b, _ := json.Marshal(req)
	rec := admin.send("POST", "/api/v1/code/package", bytes.NewReader(b), map[string]string{"Content-Type": "application/json"})
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Disposition"), "xpkg-1.0.0.zip") {
		t.Fatalf("package: %d %v", rec.Code, rec.Header())
	}
	files := unzip(t, rec.Body.Bytes())
	if files["code/lib/label.ts"] == "" || files["migrations/001_notes.ts"] == "" || !strings.Contains(files["code/main.ts"], "'xpkg.greet'") {
		t.Fatalf("zip files = %v", files)
	}
	if _, _, err := plugin.ParsePackage(rec.Body.Bytes()); err != nil {
		t.Fatalf("validator: %v", err)
	}
	// round trip: install + activate the exported package
	mustUpload(t, admin, files)
	activate(t, admin, "xpkg")
	if !codeTable(t, e, "p_xpkg_notes") {
		t.Fatal("plugin migration did not run")
	}
	// the plugin (loaded before Site Code) owns /xp/hello now; Site Code's route is refused
	rec = c.send("GET", "/xp/hello", nil, map[string]string{"Accept": "application/json", "Cookie": "pl_xpkg_c=1; pl_site_c=2"})
	var hello map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &hello)
	if fmt.Sprint(hello) != "map[cookies:map[c:1] g:<hi>! notes:0]" {
		t.Fatalf("plugin route: %d %s", rec.Code, rec.Body.String())
	}
	if l := logsOf(t, admin, "site"); !strings.Contains(l, "ถูกปฏิเสธ") {
		t.Fatalf("conflict not logged:\n%s", l)
	}
}
