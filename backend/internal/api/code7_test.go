package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/site"
)

// Phase 7 joints (found while porting the built-in plugins): manifest settings in site.settings,
// site.saveSettings, site.info, plugin fields, admin notices, dashboard widgets, middleware headers,
// {page} results, request ip/host, event aliases, http.webhook, code blocks, aggregates, binary columns,
// the sitemap cache.

const jt7Main = `
site.route('GET /jt7/settings', { access: ['*'] }, () => ({ a: site.settings('a'), b: site.settings('b'), n: site.settings('n'),
  c: site.settings('c'), extra: site.settings('extra'), none: site.settings('none'), all: site.settings() }))
site.route('GET /jt7/save', { access: ['*'] }, () => { try { site.saveSettings({ a: 'x' }); return 'saved' } catch (e) { return { error: String(e.message) } } })
site.route('GET /jt7/info', { access: ['*'] }, () => site.info())
site.route('GET /jt7/req', { access: ['*'] }, ({ request }) => ({ ip: request.ip, host: request.host }))
site.route('GET /jt7/page', { access: ['*'] }, ({ response }) => response.page({ title: 'Plugin page', html: '<p id="pp">inside</p>' }, 409))
site.route('GET /jt7/page2', { access: ['*'] }, () => ({ page: { title: 'Two', html: '<p>two</p>' } }))
site.route('GET /jt7/emit', { access: ['*'] }, () => { site.emit('jt7.done', { n: 1 }); return 'ok' })
site.route('GET /jt7/hook', { access: ['*'] }, () => { http.webhook(String(site.settings('hook')), 'jt7.ping', { hello: 'world' }, { secret: 'k3y' }); return 'queued' })
site.route('GET /jt7/agg', { access: ['*'] }, () => db.table('hits').select('path').selectSum('views', 'v').selectCount('n').selectMax('views', 'top')
  .groupBy('path').orderBy('v', 'desc').get())
site.route('GET /jt7/agg2', { access: ['*'] }, () => db.table('hits').selectCountDistinct('path', 'paths').selectAvg('views', 'avg').first())
site.route('GET /jt7/hit', { access: ['*'] }, ({ request }) => { db.table('hits').insert({ path: request.query.p, views: Number(request.query.v) }); return 'ok' })
site.route('GET /jt7/meta', { access: ['*'] }, ({ request }) => site.meta(Number(request.query.id)))
site.middleware((ctx) => {
  if (ctx.request.path === '/jt7/req' || ctx.request.path === '/') return { headers: { 'X-Robots-Tag': 'noindex', 'Cache-Control': 'no-store', 'Set-Cookie': 'x=1', Vary: 'Cookie' } }
})
site.afterResponse((info) => { if (info.path === '/jt7/req') cache.set('after', { host: info.host, purpose: info.headers.purpose ?? '' }) })
site.route('GET /jt7/after', { access: ['*'] }, () => cache.get('after') ?? null)
site.fields('post', { title: { type: 'text', label: { th: 'ชื่อ SEO', en: 'SEO title' }, max: 60 }, noindex: 'bool' })
site.adminNotice((ctx) => site.settings('b') ? { level: 'warning', text: { th: 'เปิดอยู่', en: 'It is on' },
  link: { label: { th: 'ดู', en: 'View' }, href: '/?bypass=1' } } : null)
site.adminNotice(() => ({ level: 'nope', text: 'plain', link: { label: 'bad', href: 'javascript:alert(1)' } }), { access: ['editor'] })
site.dashboardWidget('visits', { label: { th: 'ผู้เข้าชม', en: 'Visitors' }, icon: '📈', height: 180, access: ['editor'],
  render: (ctx) => '<p id="w">' + ctx.locale + ':' + ctx.user.role + '</p>' })
site.adminPage('cfg', { label: 'Config', access: ['editor'], render: () => 'cfg' })
site.action('save', { input: { a: 'text?', n: 'number?' }, access: ['editor'], run: ({ input }) => {
  try { return { data: site.saveSettings(input.n == null ? { a: input.a } : { n: input.n }) } } catch (e) { return { errors: { form: String(e.message) } } } } })
site.action('setmeta', { input: { id: 'number', title: 'text' }, access: ['editor'], run: ({ input }) => {
  try { return { data: site.setMeta(input.id, { title: input.title }) } } catch (e) { return { errors: { form: String(e.message) } } } } })
site.block('card', {
  label: { th: 'การ์ด', en: 'Card' }, icon: '🃏',
  props: [{ key: 'pick', type: 'select', label: 'Pick' }, { key: 'title', type: 'text', label: 'Title', default: 'Hi' }],
  options: () => ({ pick: [{ value: 7, label: { th: 'เจ็ด', en: 'Seven' } }, 'plain'] }),
  script: 'assets/card.js',
  render: (ctx, props) => ({ html: '<form action="/jt7/x" method="post" onsubmit="evil()"><input name="q" value="' + props.pick + '"><b>' + props.title +
    '</b><script>alert(1)</script></form><i>' + (ctx.post?.title ?? '') + '</i>', css: '.box{color:red}' }),
})
site.filter('sitemap.urls', (urls) => { cache.set('smcount', (cache.get('smcount') ?? 0) + 1); return urls.concat(db.table('hits').pluck('path')) })
`

const jt7Migration = `export default migration({ up(schema) { schema.create('hits', (t) => { t.id(); t.string('path', 100).binary().unique(); t.integer('views').default(0) }) } })`

func jt7Package(scopes []any, extra map[string]any) map[string]string {
	m := map[string]any{"id": "jt7", "name": "Joint test", "version": "1.0.0", "runtime": "js", "permissions": []any{"code", "settings"},
		"settings": []any{
			map[string]any{"key": "a", "type": "textarea", "default": "da", "maxLength": 100000},
			map[string]any{"key": "b", "type": "boolean"},
			map[string]any{"key": "n", "type": "number", "default": 3, "min": 0, "max": 10},
			map[string]any{"key": "c", "type": "text"},
			map[string]any{"key": "hook", "type": "url"},
		},
		"code": map[string]any{"entry": "code/main.ts", "scopes": scopes, "routes": []any{"/jt7"},
			"hooks": map[string]any{"events": []any{"jt7.done"}}, "eventAliases": map[string]any{"jt7.done": "legacy.done"}}}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return map[string]string{"manifest.json": string(b), "code/main.ts": jt7Main, "migrations/001_hits.ts": jt7Migration,
		"assets/card.js": "window.jt7=1"}
}

func TestPhase7Joints(t *testing.T) {
	e := offBNew(t)
	admin, user := setupSite(t, e.c)
	editor := user("Ed", "editor")
	all := []any{"routes", "db", "middleware", "admin-pages", "content-fields", "webhooks", "filters-core", "mail"}

	// manifest validation: event aliases may not claim CMS event names, unknown scopes are refused
	bad := jt7Package(all, nil)
	bad["manifest.json"] = strings.Replace(bad["manifest.json"], `"legacy.done"`, `"post.saved"`, 1)
	if code, out := uploadPkg(t, admin, "", bad); code == 200 || code == 201 || !strings.Contains(fmt.Sprint(out), "CMS event") {
		t.Fatalf("alias post.saved accepted: %d %v", code, out)
	}
	// site.fields without the content-fields scope: the plugin does not activate
	noFields := jt7Package([]any{"routes", "db", "middleware", "admin-pages", "webhooks", "filters-core"}, nil)
	mustUpload(t, admin, noFields)
	if code, out := admin.do("POST", "/api/v1/package-plugins/jt7/activate", map[string]any{}); code == 200 || !strings.Contains(fmt.Sprint(out), "content-fields") {
		t.Fatalf("site.fields without scope: %d %v", code, out)
	}
	admin.do("DELETE", "/api/v1/package-plugins/jt7", nil)
	mustUpload(t, admin, jt7Package(all, nil))
	activate(t, admin, "jt7")

	get := func(p string) map[string]any {
		t.Helper()
		code, out := getJSON(t, e.c, "GET", p, "", nil)
		if code != 200 {
			t.Fatalf("GET %s: %d %v", p, code, out)
		}
		return out
	}

	// A1: manifest value → Site code → Settings "<id>.<key>" → manifest default
	s := get("/jt7/settings")
	if s["a"] != "da" || s["b"] != false || s["n"] != float64(3) || s["c"] != "" || s["extra"] != nil || s["none"] != nil {
		t.Fatalf("defaults: %v", s)
	}
	offBSetting(t, admin, "jt7", map[string]string{"c": "from-script", "extra": "x1", "a": "script-a"})
	offBPkgSettings(t, admin, "jt7", map[string]any{"a": "pkg-a", "b": true, "n": 7, "c": ""})
	s = get("/jt7/settings")
	if s["a"] != "pkg-a" || s["b"] != true || s["n"] != float64(7) || s["c"] != "from-script" || s["extra"] != "x1" {
		t.Fatalf("precedence: %v", s)
	}
	if a := s["all"].(map[string]any); a["a"] != "pkg-a" || a["c"] != "from-script" || a["extra"] != "x1" || a["n"] != float64(7) {
		t.Fatalf("site.settings(): %v", a)
	}
	// B9: manifest text values are not limited to the 20,000 characters of Site code → Settings
	big := strings.Repeat("x", 30000)
	offBPkgSettings(t, admin, "jt7", map[string]any{"a": big, "b": true, "n": 7})
	if s = get("/jt7/settings"); s["a"] != big {
		t.Fatalf("long value: %d", len(fmt.Sprint(s["a"])))
	}

	// A2: site.saveSettings only from admin page code, for manage_options
	if out := get("/jt7/save"); !strings.Contains(fmt.Sprint(out["error"]), "หน้าแอดมิน") {
		t.Fatalf("saveSettings from a public route: %v", out)
	}
	_, edCall := adminPage(t, &sampleEnv{pluginEnv: e.pluginEnv}, editor, "jt7:cfg", "")
	if _, out := edCall("jt7:save", map[string]any{"a": "ed"}); !strings.Contains(fmt.Sprint(out["errors"]), "manage_options") {
		t.Fatalf("editor saved: %v", out)
	}
	_, adCall := adminPage(t, &sampleEnv{pluginEnv: e.pluginEnv}, admin, "jt7:cfg", "")
	if _, out := adCall("jt7:save", map[string]any{"n": 99}); !strings.Contains(fmt.Sprint(out["errors"]), "n:") {
		t.Fatalf("invalid value saved: %v", out)
	}
	if _, out := adCall("jt7:save", map[string]any{"a": "admin-a"}); out["data"].(map[string]any)["a"] != "admin-a" || out["data"].(map[string]any)["n"] != float64(7) {
		t.Fatalf("admin save: %v", out)
	}
	if st := pkgOf(t, admin, "jt7")["settings"].(map[string]any); st["a"] != "admin-a" || st["b"] != true {
		t.Fatalf("saved settings: %v", st)
	}

	// A3: site.info (+ adminEmail with scope mail)
	admin.do("PUT", "/api/v1/settings", map[string]any{"site_title": "My Site", "tagline": "Just a test", "admin_email": "boss@x.io", "language": "en"})
	info := get("/jt7/info")
	if info["title"] != "My Site" || info["tagline"] != "Just a test" || info["language"] != "en" || info["adminEmail"] != "boss@x.io" ||
		!strings.HasPrefix(fmt.Sprint(info["url"]), "http") {
		t.Fatalf("info: %v", info)
	}

	// B2: request.ip / host; A6: {headers} continues with allow-listed headers; ResponseInfo host / headers
	rec := e.offBSend(offBReq{path: "/jt7/req", ip: "203.0.113.9", hdr: map[string]string{"Purpose": "prefetch"}})
	var rq map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &rq)
	if rq["ip"] != "203.0.113.9" || rq["host"] != "example.com" {
		t.Fatalf("request: %v", rq)
	}
	if rec.Header().Get("X-Robots-Tag") != "noindex" || rec.Header().Get("Set-Cookie") != "" || rec.Header().Get("Vary") != "Cookie" {
		t.Fatalf("continue headers: %v", rec.Header())
	}
	e.code.WaitAfterResponse()
	if a := get("/jt7/after"); a["host"] != "example.com" || a["purpose"] != "prefetch" {
		t.Fatalf("afterResponse: %v", a)
	}

	// B6: {page} inside the site's layout
	rec = e.offBSend(offBReq{path: "/jt7/page"})
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), `<p id="pp">inside</p>`) || !strings.Contains(rec.Body.String(), "<title>Plugin page") ||
		!strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("page: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	if rec = e.offBSend(offBReq{path: "/jt7/page2"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "<p>two</p>") || !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("page2: %d %s", rec.Code, rec.Body.String())
	}

	// B3: code.eventAliases deliver jt7.done as legacy.done too
	publishCode(t, admin, map[string]string{"main.ts": `
site.on('legacy.done', (d) => cache.set('alias', d))
site.route('GET /alias', { access: ['*'] }, () => cache.get('alias') ?? null)`})
	get("/jt7/emit")
	e.code.WaitEvents()
	if a := get("/alias"); a["n"] != float64(1) {
		t.Fatalf("alias: %v", a)
	}

	// B4: http.webhook (signed)
	var mu sync.Mutex
	var gotHdr http.Header
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotHdr, gotBody = r.Header.Clone(), b
		mu.Unlock()
	}))
	defer srv.Close()
	e.code.WebhookClient = srv.Client()
	offBPkgSettings(t, admin, "jt7", map[string]any{"a": "admin-a", "b": true, "n": 7, "hook": srv.URL + "/in"})
	rec = e.offBSend(offBReq{path: "/jt7/hook"})
	if rec.Code != 200 {
		t.Fatalf("webhook route: %d %s", rec.Code, rec.Body.String())
	}
	e.code.WaitEvents()
	mu.Lock()
	mac := hmac.New(sha256.New, []byte("k3y"))
	mac.Write(gotBody)
	if string(gotBody) != `{"hello":"world"}` || gotHdr.Get("X-Anasaki-Event") != "jt7.ping" || gotHdr.Get("X-Anasaki-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("webhook: %s %v", gotBody, gotHdr)
	}
	mu.Unlock()

	// A4: plugin fields in the editor catalog, saved as "<id>.<key>", read with site.meta, written with site.setMeta
	_, cat := admin.do("GET", "/api/v1/post-types", nil)
	pf := cat["postFields"].(map[string]any)["post"].(map[string]any)
	if f, ok := pf["jt7.title"].(map[string]any); !ok || f["plugin"] != "jt7" || f["pluginName"] != "Joint test" {
		t.Fatalf("post fields: %v", pf)
	}
	code, post := admin.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "Hello", "status": "publish", "meta": map[string]any{"jt7.title": "SEO hello", "jt7.noindex": true}})
	if code != 201 {
		t.Fatalf("post: %d %v", code, post)
	}
	pid := int(post["id"].(float64))
	if code, out := admin.do("PUT", fmt.Sprintf("/api/v1/posts/%d", pid), map[string]any{"meta": map[string]any{"jt7.title": strings.Repeat("y", 61)}}); code != 400 {
		t.Fatalf("too long accepted: %d %v", code, out)
	}
	if m := get(fmt.Sprintf("/jt7/meta?id=%d", pid)); m["title"] != "SEO hello" || m["noindex"] != true {
		t.Fatalf("site.meta: %v", m)
	}
	if _, out := adCall("jt7:setmeta", map[string]any{"id": pid, "title": "Changed"}); out["data"].(map[string]any)["title"] != "Changed" {
		t.Fatalf("setMeta: %v", out)
	}
	author := user("Au", "author")
	_ = author
	if m := get(fmt.Sprintf("/jt7/meta?id=%d", pid)); m["title"] != "Changed" {
		t.Fatalf("after setMeta: %v", m)
	}

	// A5: admin notices (access, locale, link checks)
	_, nl := admin.doAny("GET", "/api/v1/code/admin-notices", nil)
	list := nl.([]any)
	if len(list) != 2 {
		t.Fatalf("notices (admin): %v", list)
	}
	n0 := list[0].(map[string]any)
	if n0["level"] != "warning" || n0["unit"] != "jt7" || n0["link"].(map[string]any)["href"] != "/?bypass=1" || n0["link"].(map[string]any)["external"] != false {
		t.Fatalf("notice: %v", n0)
	}
	if n1 := list[1].(map[string]any); n1["level"] != "info" || n1["text"] != "plain" || n1["link"] != nil {
		t.Fatalf("bad link kept: %v", n1)
	}
	if _, nl := editor.doAny("GET", "/api/v1/code/admin-notices", nil); len(nl.([]any)) != 1 {
		t.Fatalf("notices (editor): %v", nl)
	}
	if _, nl := author.doAny("GET", "/api/v1/code/admin-notices", nil); len(nl.([]any)) != 0 {
		t.Fatalf("notices (author): %v", nl)
	}

	// B5: dashboard widgets
	if _, wl := author.doAny("GET", "/api/v1/code/dashboard-widgets", nil); len(wl.([]any)) != 0 {
		t.Fatalf("author widgets: %v", wl)
	}
	_, wl := editor.doAny("GET", "/api/v1/code/dashboard-widgets", nil)
	w0 := wl.([]any)[0].(map[string]any)
	if w0["id"] != "jt7:visits" || w0["height"] != float64(180) || !strings.HasPrefix(fmt.Sprint(w0["url"]), "/ext-ui/code/") {
		t.Fatalf("widgets: %v", wl)
	}
	rec = e.c.send("GET", w0["url"].(string), nil, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `<p id="w">th:editor</p>`) || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("widget page: %d %s", rec.Code, rec.Body.String())
	}

	// B1: code blocks in the designer (dynamic options), previews and public pages (sanitised, scoped CSS, script)
	_, blocks := admin.doAny("GET", "/api/v1/blocks", nil)
	var card map[string]any
	for _, b := range blocks.([]any) {
		if m := b.(map[string]any); m["plugin"] == "jt7" && m["type"] == "card" {
			card = m
		}
	}
	if card == nil || card["pluginName"] != "Joint test" {
		t.Fatalf("blocks: %v", blocks)
	}
	opts := card["props"].([]any)[0].(map[string]any)["options"].([]any)
	if len(opts) != 2 || opts[0].(map[string]any)["value"] != "7" || opts[0].(map[string]any)["label"].(map[string]any)["en"] != "Seven" ||
		opts[1].(map[string]any)["value"] != "plain" {
		t.Fatalf("dynamic options: %v", opts)
	}
	code, pv := admin.do("POST", "/api/v1/blocks/render", map[string]any{"plugin": "jt7", "block": "card", "values": map[string]any{"pick": "7", "title": "<u>T</u>"}})
	h := fmt.Sprint(pv["html"])
	if code != 200 || !strings.Contains(h, `<input name="q" value="7">`) || !strings.Contains(h, `<form action="/jt7/x" method="post">`) ||
		strings.Contains(h, "alert(1)") || strings.Contains(h, "onsubmit") || !strings.Contains(h, `src="/plugin-assets/jt7/assets/card.js?v=1.0.0"`) ||
		!strings.Contains(fmt.Sprint(pv["css"]), ".pb-jt7-card .box") {
		t.Fatalf("preview: %d %v", code, pv)
	}
	_, pg := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Blocky", "slug": "blocky", "status": "publish",
		"design": map[string]any{"id": "root", "type": "frame", "children": []any{
			map[string]any{"id": "pb", "type": "pluginBlock", "props": map[string]any{"plugin": "jt7", "block": "card", "values": map[string]any{"pick": "anything"}}}}}})
	if pg["id"] == nil {
		t.Fatalf("page: %v", pg)
	}
	out := e.offBSend(offBReq{path: "/blocky"}).Body.String()
	offBContains(t, "public block", out, `pb-jt7-card`, `<input name="q" value="anything">`, `<b>Hi</b>`, `.pb-jt7-card .box`)
	if strings.Contains(out, "alert(1)") {
		t.Fatal("block script kept")
	}

	// B7: aggregates in fallback (and code-database) mode; B8: case-sensitive unique paths
	for _, h := range [][2]string{{"/About", "3"}, {"/about", "4"}, {"/x", "10"}} {
		if rec := e.offBSend(offBReq{path: "/jt7/hit?p=" + h[0] + "&v=" + h[1]}); rec.Code != 200 {
			t.Fatalf("hit %s: %d %s", h[0], rec.Code, rec.Body.String())
		}
	}
	rec = e.offBSend(offBReq{path: "/jt7/agg"})
	var rows []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &rows)
	if len(rows) != 3 || rows[0]["path"] != "/x" || rows[0]["v"] != float64(10) || rows[0]["n"] != float64(1) || rows[0]["top"] != float64(10) {
		t.Fatalf("aggregates: %d %s", rec.Code, rec.Body.String())
	}
	if a := get("/jt7/agg2"); a["paths"] != float64(3) || fmt.Sprintf("%.4f", a["avg"]) != "5.6667" {
		t.Fatalf("aggregates 2: %v", a)
	}

	// A7: sitemap cache — reused until content, plugin data or settings change
	site.SitemapTTL = time.Minute
	sm := func() string { return e.offBSend(offBReq{path: "/sitemap.xml"}).Body.String() }
	smCount := func() float64 {
		publishCode(t, admin, map[string]string{"main.ts": `site.route('GET /smc', { access: ['*'] }, () => ({ n: plugins.call('jt7', 'smc') }))`})
		return 0
	}
	_ = smCount
	first := sm()
	if !strings.Contains(first, "/About") || !strings.Contains(first, "/about") {
		t.Fatalf("sitemap: %s", first)
	}
	if second := sm(); second != first {
		t.Fatal("sitemap changed without a change")
	}
	e.offBSend(offBReq{path: "/jt7/hit?p=/new&v=1"}) // plugin data changed
	if third := sm(); !strings.Contains(third, "/new") {
		t.Fatalf("sitemap not refreshed after a plugin write: %s", third)
	}
	admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Fresh", "slug": "fresh-page", "status": "publish"})
	if fourth := sm(); !strings.Contains(fourth, "/fresh-page") {
		t.Fatalf("sitemap not refreshed after a new page: %s", fourth)
	}
}
