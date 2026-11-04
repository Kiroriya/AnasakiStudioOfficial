package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Site Code phase 6: the generic joints the sample plugins needed (plugin
// formatters + aliases, binary bodies, admin page downloads, package
// assets, plugins.call to JS exports, optionalRequires, admin page access
// overrides and ctx.locale) and the joints for moving the built-in plugins
// out of core (site.middleware, site.afterResponse, site.head / bodyEnd,
// page.meta, sitemap.urls / robots.txt, the admin UI kit).

func liveFormat(t *testing.T, e *pluginEnv, name string, v any) (string, bool) {
	t.Helper()
	opts, err := e.st.Options()
	if err != nil {
		t.Fatal(err)
	}
	s := e.code.LiveSession(httptest.NewRequest("GET", "/", nil), opts)
	if s == nil {
		return "", false
	}
	defer s.Close()
	return s.Format(name, v, nil)
}

func TestPhase6Formatters(t *testing.T) {
	e := newPluginAPI(t)
	admin, user := setupSite(t, e.c)
	editor := user("Ed", "editor")
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "thai", "code": map[string]any{"entry": "code/main.ts", "scopes": []any{}}},
		map[string]string{"code/main.ts": `
site.formatter('baht', (v) => '฿' + Number(v).toFixed(2))
site.formatter('num', (v) => 'n' + v)
site.formatter('upper', (v) => 'never')
`}))
	activate(t, admin, "thai")
	if s, ok := liveFormat(t, e, "thai:baht", 5); !ok || s != "฿5.00" {
		t.Fatalf("thai:baht = %q %v", s, ok)
	}
	if _, ok := liveFormat(t, e, "baht", 5); ok {
		t.Fatal("unprefixed name resolved without an alias")
	}
	if code, _ := editor.do("PUT", "/api/v1/code/formatters/aliases", map[string]any{"aliases": map[string]any{}}); code != 403 {
		t.Fatalf("editor may set aliases: %d", code)
	}
	code, l := admin.doAny("GET", "/api/v1/code/formatters", nil)
	if code != 200 || fmt.Sprint(l) != "[map[name:thai:baht unit:thai] map[name:thai:num unit:thai] map[conflict:core name:thai:upper unit:thai]]" {
		t.Fatalf("formatters: %d %v", code, l)
	}
	for _, bad := range []map[string]any{{"x": "thai:baht"}, {"upper": "thai:upper"}, {"baht": "baht"}, {"Bad name": "thai:Bad name"}} {
		if code, b := admin.do("PUT", "/api/v1/code/formatters/aliases", map[string]any{"aliases": bad}); code != 400 {
			t.Fatalf("bad aliases %v accepted: %d %v", bad, code, b)
		}
	}
	code, l = admin.doAny("PUT", "/api/v1/code/formatters/aliases", map[string]any{"aliases": map[string]any{"baht": "thai:baht"}})
	if code != 200 || !strings.Contains(fmt.Sprint(l), "map[alias:baht name:thai:baht unit:thai]") {
		t.Fatalf("set aliases: %d %v", code, l)
	}
	if s, ok := liveFormat(t, e, "baht", 2); !ok || s != "฿2.00" {
		t.Fatalf("alias baht = %q %v", s, ok)
	}
	if _, b := admin.do("GET", "/api/v1/code/bindings", nil); !strings.Contains(strs(b["formatters"]), "thai:baht") || !strings.HasSuffix(fmt.Sprint(b["formatters"]), " baht]") {
		t.Fatalf("bindings formatters: %v", b["formatters"])
	}
	// a second plugin with the same name: the first enabled alias keeps it
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "thai2", "code": map[string]any{"entry": "code/main.ts", "scopes": []any{}}},
		map[string]string{"code/main.ts": `site.formatter('baht', (v) => 'B' + v)`}))
	activate(t, admin, "thai2")
	if _, l := admin.doAny("GET", "/api/v1/code/formatters", nil); !strings.Contains(fmt.Sprint(l), "map[conflict:thai:baht name:thai2:baht unit:thai2]") {
		t.Fatalf("plugin conflict: %v", l)
	}
	if s, _ := liveFormat(t, e, "thai2:baht", 2); s != "B2" {
		t.Fatalf("thai2:baht = %q", s)
	}
	// Site Code's own formatter wins over the alias
	publishCode(t, admin, map[string]string{"main.ts": `site.formatter('baht', (v) => 'site ' + v)`})
	if s, _ := liveFormat(t, e, "baht", 3); s != "site 3" {
		t.Fatalf("site formatter must win: %q", s)
	}
	if _, l := admin.doAny("GET", "/api/v1/code/formatters", nil); !strings.Contains(fmt.Sprint(l), "map[alias:baht conflict:site name:thai:baht unit:thai]") ||
		!strings.Contains(fmt.Sprint(l), "map[name:baht unit:site]") {
		t.Fatalf("site conflict: %v", l)
	}
	if s, _ := liveFormat(t, e, "thai:baht", 3); s != "฿3.00" {
		t.Fatalf("prefixed name still works: %q", s)
	}
}

func TestPhase6BinaryDownloadsAccessLocale(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	admin, user := setupSite(t, c)
	editor := user("Ed", "editor")
	author := user("Au", "author")
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "bin", "code": map[string]any{"entry": "code/main.ts",
		"scopes": []any{"routes", "admin-pages"}, "routes": []any{"/bin"}}}, map[string]string{"code/main.ts": `
site.route('GET /bin/u8', { access: ['*'] }, () => ({ status: 200, headers: { 'Content-Disposition': 'attachment; filename="x.bin"' }, body: new Uint8Array([0, 255, 128, 10]) }))
site.route('GET /bin/b64', { access: ['*'] }, () => ({ status: 201, bodyBase64: 'AP+A' }))
site.route('GET /bin/buf', { access: ['*'] }, () => new Uint8Array([1, 2, 3]).buffer)
site.route('GET /bin/typed', { access: ['*'] }, () => ({ status: 200, headers: { 'Content-Type': 'image/png' }, body: new Uint8Array([137, 80]) }))
site.route('GET /bin/both', { access: ['*'] }, () => ({ status: 200, body: 'x', bodyBase64: 'AA==' }))
site.route('GET /bin/bad64', { access: ['*'] }, () => ({ status: 200, bodyBase64: '***' }))
site.route('GET /bin/big', { access: ['*'] }, () => new Uint8Array(21 * 1024 * 1024))
site.adminPage('exp', { label: { th: 'ส่งออก', en: 'Export' }, access: ['editor'],
  render: (ctx) => '<p id="loc">' + ctx.locale + '</p>',
  download: (ctx) => ({ status: 200, headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': 'inline; filename="e.csv"' }, body: 'a,b\n' + ctx.query.n + ',' + ctx.locale + ',' + ctx.user.role }) })
site.adminPage('nodl', { label: 'No download', render: () => 'x' })
`}))
	activate(t, admin, "bin")
	get := func(p string) *httptest.ResponseRecorder { return c.send("GET", p, nil, nil) }
	if r := get("/bin/u8"); r.Code != 200 || r.Body.String() != "\x00\xff\x80\n" || r.Header().Get("Content-Type") != "application/octet-stream" ||
		r.Header().Get("Content-Disposition") != `attachment; filename="x.bin"` {
		t.Fatalf("u8: %d %q %v", r.Code, r.Body.String(), r.Header())
	}
	if r := get("/bin/b64"); r.Code != 201 || r.Body.String() != "\x00\xff\x80" {
		t.Fatalf("b64: %d %q", r.Code, r.Body.String())
	}
	if r := get("/bin/buf"); r.Code != 200 || r.Body.String() != "\x01\x02\x03" {
		t.Fatalf("ArrayBuffer: %d %q", r.Code, r.Body.String())
	}
	if r := get("/bin/typed"); r.Header().Get("Content-Type") != "image/png" || r.Body.String() != "\x89P" {
		t.Fatalf("typed: %v %q", r.Header(), r.Body.String())
	}
	for _, p := range []string{"/bin/both", "/bin/bad64", "/bin/big"} {
		if r := get(p); r.Code != 500 {
			t.Fatalf("%s: %d", p, r.Code)
		}
	}
	if l := logsOf(t, admin, "bin"); !strings.Contains(l, "body ใหญ่เกิน 20 MB") || !strings.Contains(l, "bodyBase64 ไม่ใช่ base64") {
		t.Fatalf("logs:\n%s", l)
	}
	// the test-run API returns a binary body as base64
	publishCode(t, admin, map[string]string{"main.ts": `site.route('GET /sc-bin', { access: ['*'] }, () => new Uint8Array([255]))`})
	if _, b := admin.do("POST", "/api/v1/code/run", map[string]any{"method": "GET", "path": "/sc-bin"}); b["bodyBase64"] != base64.StdEncoding.EncodeToString([]byte{255}) || b["body"] != nil {
		t.Fatalf("run: %v", b)
	}

	// admin page download: same ticket and access as the page
	ticket := func(cl *client, id string) (int, string) {
		code, b := cl.do("POST", "/api/v1/code/admin-pages/"+id+"/ticket", nil)
		u, _ := b["url"].(string)
		return code, u
	}
	_, u := ticket(admin, "bin:exp")
	page := get(u)
	h := page.Body.String()
	if page.Code != 200 || !strings.Contains(h, `<p id="loc">en</p>`) || !strings.Contains(h, "<title>Export</title>") || !strings.Contains(h, "download:function") ||
		!strings.Contains(h, `/ext-ui/kit.css?v=`) || !strings.Contains(h, `/ext-ui/kit.js?v=`) || !strings.Contains(page.Header().Get("Content-Security-Policy"), "allow-downloads") {
		t.Fatalf("page: %d %v\n%s", page.Code, page.Header(), h)
	}
	dl := get(u + "/download?n=7")
	if dl.Code != 200 || dl.Body.String() != "a,b\n7,en,administrator" || dl.Header().Get("Content-Disposition") != `attachment; filename="e.csv"` ||
		dl.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("download: %d %q %v", dl.Code, dl.Body.String(), dl.Header())
	}
	_, eu := ticket(editor, "bin:exp")
	if dl := get(eu + "/download?n=1"); dl.Code != 200 || dl.Body.String() != "a,b\n1,th,editor" {
		t.Fatalf("editor download: %d %q", dl.Code, dl.Body.String())
	}
	if code, _ := ticket(author, "bin:exp"); code != 404 {
		t.Fatalf("author ticket: %d", code)
	}
	tok := strings.Split(strings.TrimPrefix(u, "/ext-ui/code/"), "/")[0]
	if r := get("/ext-ui/code/" + tok + "/bin:nodl/download"); r.Code != 404 {
		t.Fatalf("page without download: %d", r.Code)
	}
	if r := get("/ext-ui/code/garbage/bin:exp/download"); r.Code != 401 {
		t.Fatalf("bad ticket download: %d", r.Code)
	}
	// Access screen override for admin pages (admin roles only)
	if code, b := admin.do("PUT", "/api/v1/code/access", map[string]any{"kind": "adminPage", "name": "bin:exp", "roles": []any{"*"}}); code != 400 {
		t.Fatalf("'*' for an admin page: %d %v", code, b)
	}
	if code, b := admin.do("PUT", "/api/v1/code/access", map[string]any{"kind": "adminPage", "name": "exp", "unit": "bin", "roles": []any{"author"}}); code != 200 ||
		strs(b["effective"]) != "author" {
		t.Fatalf("override: %d %v", code, b)
	}
	if code, _ := ticket(author, "bin:exp"); code != 200 {
		t.Fatalf("author after override: %d", code)
	}
	if r := get(eu + "/download"); r.Code != 403 {
		t.Fatalf("editor after override: %d", r.Code)
	}
	_, rows := admin.doAny("GET", "/api/v1/code/access?unit=bin", nil)
	if !strings.Contains(fmt.Sprint(rows), "kind:adminPage") {
		t.Fatalf("access rows: %v", rows)
	}
	// the admin UI kit
	for p, ct := range map[string]string{"/ext-ui/kit.css": "text/css", "/ext-ui/kit.js": "text/javascript"} {
		r := get(p)
		if r.Code != 200 || !strings.HasPrefix(r.Header().Get("Content-Type"), ct) || r.Body.Len() < 500 || r.Header().Get("ETag") == "" {
			t.Fatalf("%s: %d %v", p, r.Code, r.Header())
		}
	}
	if r := get("/ext-ui/kit.js"); !strings.Contains(r.Body.String(), "AK") || !strings.Contains(r.Body.String(), "pickMedia") || !strings.Contains(r.Body.String(), "ak:pick-media:result") {
		t.Fatal("kit.js has no AK global")
	}
}

func TestPhase6AssetsCallsOptionalRequires(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	admin, _ := setupSite(t, c)
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "lib", "code": map[string]any{"entry": "code/main.ts",
		"scopes": []any{"routes"}, "routes": []any{"/lib"}, "exports": []any{"hello"}}}, map[string]string{
		"data/hello.txt": "สวัสดี",
		"fonts/x.ttf":    "\x00\x01\xff",
		"code/main.ts": `
site.export('hello', (n) => ({ hi: 'hello ' + n }))
site.route('GET /lib/asset', { access: ['*'] }, () => {
  const a = site.asset('data/hello.txt'), b = site.asset('fonts/x.ttf')
  const errs = ['../manifest.json', 'manifest.json', 'code/main.ts', 'migrations/x.ts', '/etc/passwd', 'data/../manifest.json', 'missing.txt', 'data\\hello.txt']
    .map((p) => { try { site.asset(p); return 'ok' } catch (e) { return 'err' } })
  return { text: a.text(), size: a.size, type: a.type, url: a.url, bytes: Array.from(b.bytes()), burl: b.url, btype: b.type, errs }
})
`}))
	activate(t, admin, "lib")
	_, b := getJSON(t, c, "GET", "/lib/asset", "", nil)
	if b["text"] != "สวัสดี" || b["size"] != float64(len("สวัสดี")) || !strings.HasPrefix(fmt.Sprint(b["type"]), "text/plain") ||
		b["url"] != "/plugin-assets/lib/data/hello.txt?v=1.0.0" || fmt.Sprint(b["bytes"]) != "[0 1 255]" || b["burl"] != "/plugin-assets/lib/fonts/x.ttf?v=1.0.0" || b["btype"] != "font/ttf" ||
		strs(b["errs"]) != "err,err,err,err,err,err,err,err" {
		t.Fatalf("asset: %v", b)
	}
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('GET /sc-asset', { access: ['*'] }, () => { try { site.asset('x.txt'); return 'ok' } catch (e) { return { err: String(e.message) } } })
site.route('GET /sc-call', { access: ['*'] }, () => plugins.call('lib', 'hello', 'site'))
`})
	if _, b := getJSON(t, c, "GET", "/sc-asset", "", nil); !strings.Contains(fmt.Sprint(b["err"]), "public/") { // 2.2.0: Site Code reads its public/ files only
		t.Fatalf("site.asset in Site Code: %v", b)
	}
	if _, b := getJSON(t, c, "GET", "/sc-call", "", nil); b["hi"] != "hello site" {
		t.Fatalf("Site Code plugins.call: %v", b)
	}

	// optionalRequires: same syntax as requires, never blocks the install
	for _, bad := range []map[string]any{{"optionalRequires": map[string]any{"lib": "not a range"}},
		{"optionalRequires": map[string]any{"lib": "*"}, "requires": map[string]any{"lib": "*"}}} {
		m := map[string]any{"id": "bad", "code": map[string]any{"entry": "code/main.ts", "scopes": []any{}}}
		for k, v := range bad {
			m[k] = v
		}
		if code, _ := uploadPkg(t, admin, "", jsPlugin(t, m, map[string]string{"code/main.ts": "1"})); code != 400 {
			t.Fatalf("bad optionalRequires %v: %d", bad, code)
		}
	}
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "caller", "optionalRequires": map[string]any{"lib": "^2.0.0", "absent": "^1.0.0"},
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"routes"}, "routes": []any{"/caller"}}}, map[string]string{"code/main.ts": `
const j = (f) => { try { return f() } catch (e) { return { err: String(e.message), code: e.code || null, name: e.name } } }
const top = plugins.has('lib')
site.route('GET /caller/x', { access: ['*'] }, () => ({
  call: j(() => plugins.call('lib', 'hello', 'bob')),
  has: plugins.has('lib'), top, version: plugins.version('lib'), none: plugins.has('nope'), noneV: plugins.version('nope'),
  missing: j(() => plugins.call('nope', 'x')),
  secret: j(() => plugins.call('lib', 'nothere')),
}))
`}))
	activate(t, admin, "caller")
	p := pkgOf(t, admin, "caller")
	if fmt.Sprint(p["optionalRequires"]) != "map[absent:^1.0.0 lib:^2.0.0]" ||
		fmt.Sprint(p["optionalRequiresStatus"]) != "[map[active:false id:absent installed: ok:false range:^1.0.0] map[active:true id:lib installed:1.0.0 ok:false range:^2.0.0]]" {
		t.Fatalf("optionalRequires listing: %v %v", p["optionalRequires"], p["optionalRequiresStatus"])
	}
	_, b = getJSON(t, c, "GET", "/caller/x", "", nil)
	if fmt.Sprint(b["call"]) != "map[hi:hello bob]" || b["has"] != true || b["top"] != true || b["version"] != "1.0.0" || b["none"] != false || b["noneV"] != nil {
		t.Fatalf("calls: %v", b)
	}
	if m := b["missing"].(map[string]any); m["code"] != "unavailable" || m["name"] != "PluginUnavailable" {
		t.Fatalf("unavailable: %v", m)
	}
	if !strings.Contains(fmt.Sprint(b["secret"]), "code.exports") {
		t.Fatalf("undeclared export: %v", b["secret"])
	}
	if l := logsOf(t, admin, "caller"); !strings.Contains(l, "ใช้ร่วมกับปลั๊กอิน lib ^2.0.0") {
		t.Fatalf("optional version warning missing:\n%s", l)
	}
	// target switched off → PluginUnavailable; plugins.has false
	if code, _ := admin.do("POST", "/api/v1/package-plugins/lib/deactivate", nil); code != 200 {
		t.Fatalf("deactivate lib: %d", code)
	}
	e.code.UnitsChanged()
	_, b = getJSON(t, c, "GET", "/caller/x", "", nil)
	if m, _ := b["call"].(map[string]any); m["code"] != "unavailable" || b["has"] != false || b["version"] != nil {
		t.Fatalf("after deactivation: %v", b)
	}
}

func TestPhase6PageHooks(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	t.Cleanup(e.code.WaitAfterResponse)
	e.code.MiddlewareTimeout = 60 * time.Millisecond
	admin, _ := setupSite(t, c)
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "About", "slug": "about", "status": "publish", "content": "<p>hi</p>"}); code != 201 {
		t.Fatalf("page: %d %v", code, b)
	}
	// middleware without the scope → activation fails
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "noscope", "code": map[string]any{"entry": "code/main.ts", "scopes": []any{}}},
		map[string]string{"code/main.ts": `site.middleware(() => undefined)`}))
	if code, b := admin.do("POST", "/api/v1/package-plugins/noscope/activate", nil); code == 200 || !strings.Contains(fmt.Sprint(b), "middleware") {
		t.Fatalf("middleware without scope: %d %v", code, b)
	}
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "mwx", "code": map[string]any{"entry": "code/main.ts", "scopes": []any{"middleware"}}},
		map[string]string{"code/main.ts": `
site.middleware((ctx) => {
  if (ctx.request.query.slow) { while (true) {} }
  if (ctx.request.path === '/blocked') return { status: 403, body: 'no' }
}, { priority: 5 })
site.middleware((ctx) => {
  if (ctx.request.path === '/blocked') return 'second'
  if (ctx.request.path === '/m2') { ctx.response.setCookie('seen', '1'); return { status: 200, body: 'm2 ' + ctx.request.ip + ' ' + ctx.isAdminUser + ' ' + (ctx.request.body === '') } }
})
site.afterResponse((info) => { log.info('after ' + info.method + ' ' + info.path + ' ' + info.status + ' ' + info.title + ' ' + (info.durationMs >= 0)) })
site.head((page) => '<meta name="x-test" content="' + page.type + ' ' + page.path + ' ' + page.isHome + '"><script>alert(1)</script><style>x</style><link rel="alternate" hreflang="en" href="/en/about"><script type="application/ld+json">{"a":"<b>"}</script>')
site.bodyEnd(() => '<b>end</b><script>bad()</script>')
`}))
	activate(t, admin, "mwx")
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "hsx", "code": map[string]any{"entry": "code/main.ts", "scopes": []any{"head-scripts", "filters-core"}}},
		map[string]string{"code/main.ts": `
site.head(() => '<script>trusted()</script>')
site.bodyEnd(() => '<script>alsoTrusted()</script>')
site.filter('page.meta', (m, ctx) => ({ ...m, fullTitle: 'T: ' + m.title, canonical: '/canon' + ctx.page.path, robots: 'noarchive', og: { 'og:title': m.title, 'twitter:card': 'summary', 'bad key': 'x' } }))
site.filter('sitemap.urls', (list) => [...list, '/extra', { loc: '/pri', priority: 0.5, changefreq: 'daily' }, 'https://evil.example/x'])
`}))
	activate(t, admin, "hsx")
	publishCode(t, admin, map[string]string{"main.ts": `site.middleware((ctx) => ctx.request.path === '/first' ? 'site first' : undefined, { priority: 1 })`})

	if code, body := c.public("GET", "/blocked"); code != 403 || body != "no" {
		t.Fatalf("blocked: %d %q", code, body)
	}
	if code, body := c.public("GET", "/first"); code != 200 || body != "site first" {
		t.Fatalf("priority order: %d %q", code, body)
	}
	r := c.send("POST", "/m2", strings.NewReader("payload"), nil)
	if r.Code != 200 || r.Body.String() != "m2 192.0.2.1 false true" || !strings.Contains(r.Header().Get("Set-Cookie"), "pl_mwx_seen=1") {
		t.Fatalf("m2: %d %q %v", r.Code, r.Body.String(), r.Header())
	}
	if r := admin.send("GET", "/m2", nil, nil); !strings.Contains(r.Body.String(), " true ") {
		t.Fatalf("isAdminUser: %q", r.Body.String())
	}
	// a slow middleware is skipped (time budget); the next one answers
	start := time.Now()
	if code, body := c.public("GET", "/blocked?slow=1"); code != 200 || body != "second" {
		t.Fatalf("slow middleware: %d %q", code, body)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("slow middleware took %v", d)
	}
	// circuit breaker: 20 failures → the middleware is skipped without running
	for i := 0; i < 20; i++ {
		_, _ = c.public("GET", "/x?slow=1")
	}
	l := logsOf(t, admin, "mwx")
	if !strings.Contains(l, "middleware ถูกข้าม") || !strings.Contains(l, "ปิดชั่วคราว") {
		t.Fatalf("mw logs:\n%s", l)
	}
	skipped := strings.Count(l, "middleware ถูกข้าม")
	if code, body := c.public("GET", "/blocked?slow=1"); body != "second" || code != 200 {
		t.Fatalf("breaker: %d %q", code, body)
	}
	if n := strings.Count(logsOf(t, admin, "mwx"), "middleware ถูกข้าม"); n != skipped {
		t.Fatalf("open breaker still ran the middleware: %d → %d", skipped, n)
	}

	// head / bodyEnd / page.meta on a fallback page
	r = c.send("GET", "/about", nil, nil)
	h := r.Body.String()
	for _, want := range []string{`<meta name="x-test" content="page /about false">`, `<link rel="alternate" href="/en/about" hreflang="en">`,
		`<script type="application/ld+json">{"a":"\u003cb\u003e"}</script>`, "<b>end</b>", "<script>trusted()</script>", "<script>alsoTrusted()</script>",
		"<title>T: About</title>", `<link rel="canonical" href="http://example.com/canon/about">`, `<meta name="robots" content="noarchive">`,
		`<meta property="og:title" content="About">`, `<meta name="twitter:card" content="summary">`} {
		if !strings.Contains(h, want) {
			t.Fatalf("page lacks %s:\n%s", want, h)
		}
	}
	for _, bad := range []string{"alert(1)", "bad()", "<style>x</style>", "bad key"} {
		if strings.Contains(h, bad) {
			t.Fatalf("page has %s:\n%s", bad, h)
		}
	}
	// sitemap.xml + robots.txt served by core when filtered
	r = c.send("GET", "/sitemap.xml", nil, nil)
	sm := r.Body.String()
	if r.Code != 200 || !strings.HasPrefix(r.Header().Get("Content-Type"), "application/xml") || !strings.Contains(sm, "<loc>http://example.com/about</loc>") ||
		!strings.Contains(sm, "<loc>http://example.com/extra</loc>") || !strings.Contains(sm, "<priority>0.5</priority>") || strings.Contains(sm, "evil") {
		t.Fatalf("sitemap: %d\n%s", r.Code, sm)
	}
	if code, body := c.public("GET", "/robots.txt"); code != 200 || !strings.Contains(body, "Sitemap: http://example.com/sitemap.xml") {
		t.Fatalf("robots: %d %q", code, body)
	}
	// afterResponse ran for the requests above, after they were answered
	e.code.WaitAfterResponse()
	if l := logsOf(t, admin, "mwx"); !strings.Contains(l, "after GET /about 200 About true") || !strings.Contains(l, "after GET /blocked 403") {
		t.Fatalf("afterResponse logs:\n%s", l)
	}
}

// a slow afterResponse handler never delays the response
func TestPhase6AfterResponseNonBlocking(t *testing.T) {
	e := newPluginAPI(t)
	c := e.c
	t.Cleanup(e.code.WaitAfterResponse)
	admin, _ := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": `
site.afterResponse(() => { const t = Date.now(); while (Date.now() - t < 300) {} log.info('slow after done') })
site.route('GET /fast', { access: ['*'] }, () => 'fast')
`})
	start := time.Now()
	for i := 0; i < 3; i++ {
		if code, body := c.public("GET", "/fast"); code != 200 || body != "fast" {
			t.Fatalf("fast: %d %q", code, body)
		}
	}
	if d := time.Since(start); d > 250*time.Millisecond {
		t.Fatalf("responses waited for afterResponse: %v", d)
	}
	e.code.WaitAfterResponse()
	if l := logsOf(t, admin, "site"); strings.Count(l, "slow after done") != 3 {
		t.Fatalf("logs:\n%s", l)
	}
	_ = http.StatusOK
}
