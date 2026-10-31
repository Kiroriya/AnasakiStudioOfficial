package sitecode

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/design"
)

// ---------------------------------------------------------------- registration

func TestPhase4Registration(t *testing.T) {
	p := mustCompile(t, map[string]string{"main.ts": `
site.filter('post.content', (h) => h + '!', { priority: 5 })
site.filter('site.price', (v) => v * 2)
site.visitor(({ request }) => null)
site.visitorGroups(['member', 'gold'])
site.postType('event', { label: { th: 'อีเวนต์', en: 'Events' }, icon: '📅', slug: 'events',
  fields: { date: { type: 'datetime', required: true, label: { th: 'วันที่', en: 'Date' } }, place: 'text', seats: { type: 'number', min: 0 },
    kind: { type: 'select', options: ['talk', { value: 'ws', label: 'Workshop' }] } },
  access: { edit: ['editor', 'author'] } })
site.fields('post', { readingTime: 'number' })
site.fields('event', { extra: 'bool' })
site.job('send', { timeout: 30, retries: 2, concurrency: 3 }, ({ data }) => data)
site.job('plain', () => 1)
site.cron('0 8 * * *', 'send')
site.cron('*/5 * * * *', 'send')
site.cron('@daily', () => 1)
site.adminPage('orders', { label: 'Orders', icon: '🧾', access: ['editor'], render: () => '<p>x</p>' })
site.route('POST /hook', { access: ['*'], rawBody: true, maxBody: 2048 }, ({ request }) => request.rawBody)
site.route('GET /club', { access: ['group:gold', 'editor'] }, () => 'ok')
`})
	m := p.Manifest
	if len(m.Filters) != 2 || m.Filters[0].Priority != 5 || m.Filters[1].Priority != 10 || !m.Visitor || strings.Join(m.VisitorGroups, ",") != "member,gold" {
		t.Fatalf("hooks: %+v %v %v", m.Filters, m.Visitor, m.VisitorGroups)
	}
	pt := m.PostType("event")
	if pt == nil || pt.Slug != "events" || pt.Label["en"] != "Events" || len(pt.Fields) != 4 || pt.Fields[0].Key != "date" || !pt.Fields[0].Required ||
		pt.Fields[3].Options[1].Label["en"] != "Workshop" || strings.Join(pt.Access.Edit, ",") != "editor,author" || strings.Join(pt.Supports, ",") != "title,content,excerpt,image" {
		t.Fatalf("post type: %+v", pt)
	}
	if f := m.FieldsFor("event"); len(f) != 5 || f[4].Key != "extra" {
		t.Fatalf("fields for event: %+v", f)
	}
	if f := m.FieldsFor("post"); len(f) != 1 || f[0].Type != "number" {
		t.Fatalf("site.fields post: %+v", f)
	}
	if string(FieldsObject(m.FieldsFor("post"))) != `{"readingTime":{"type":"number","label":{"en":"readingTime","th":"readingTime"}}}` {
		t.Fatalf("fields object: %s", FieldsObject(m.FieldsFor("post")))
	}
	if len(m.Jobs) != 2 || m.Jobs[0].Retries != 2 || m.Jobs[1].Timeout != 60 || m.Jobs[1].Retries != 3 {
		t.Fatalf("jobs: %+v", m.Jobs)
	}
	if len(m.Crons) != 3 || m.Crons[0].Name != "send" || m.Crons[1].Name != "send@2" || m.Crons[2].Name != "@cron-1" || m.Crons[2].Job != "@cron-1" {
		t.Fatalf("crons: %+v", m.Crons)
	}
	if p.jobs["@cron-1"] == nil || p.jobs["@cron-1"].Retries != 0 {
		t.Fatal("function cron has no internal job")
	}
	if len(m.AdminPages) != 1 || m.AdminPages[0].Access[0] != "editor" {
		t.Fatalf("admin pages: %+v", m.AdminPages)
	}
	if !m.Routes[0].RawBody || m.Routes[0].MaxBody != 2048 || m.Routes[1].Access[0] != "group:gold" {
		t.Fatalf("routes: %+v", m.Routes)
	}

	bad := map[string]string{
		"bad filter":      "site.filter('nope', (v) => v)\n",
		"filter no fn":    "site.filter('post.content')\n",
		"two visitors":    "site.visitor(() => null)\nsite.visitor(() => null)\n",
		"bad group":       "site.visitorGroups(['Gold!'])\n",
		"bad access grp":  "site.route('GET /x', { access: ['group:Bad Group'] }, () => 1)\n",
		"builtin type":    "site.postType('page', {})\n",
		"bad type id":     "site.postType('E', {})\n",
		"reserved slug":   "site.postType('news', { slug: 'api' })\n",
		"blog slug":       "site.postType('stories', { slug: 'news' })\n",
		"dup slug":        "site.postType('aa', { slug: 'x' })\nsite.postType('bb', { slug: 'x' })\n",
		"dup type":        "site.postType('aa', {})\nsite.postType('aa', {})\n",
		"select no opts":  "site.postType('aa', { fields: { k: { type: 'select' } } })\n",
		"bad field type":  "site.postType('aa', { fields: { k: 'money' } })\n",
		"fields unknown":  "site.fields('ghost', { k: 'text' })\n",
		"dup field":       "site.fields('post', { k: 'text' })\nsite.fields('post', { k: 'number' })\n",
		"bad edit role":   "site.postType('aa', { access: { edit: ['*'] } })\n",
		"dup job":         "site.job('a', () => 1)\nsite.job('a', () => 1)\n",
		"job timeout":     "site.job('a', { timeout: 999 }, () => 1)\n",
		"bad cron":        "site.cron('61 * * * *', () => 1)\n",
		"cron 4 fields":   "site.cron('* * * *', () => 1)\n",
		"cron no job":     "site.cron('* * * * *', 'ghost')\n",
		"dup admin page":  "site.adminPage('a', { render: () => '' })\nsite.adminPage('a', { render: () => '' })\n",
		"admin no render": "site.adminPage('a', { label: 'x' })\n",
		"admin group":     "site.adminPage('a', { access: ['group:x'], render: () => '' })\n",
		"maxBody":         "site.route('POST /h', { maxBody: 6000000 }, () => 1)\n",
	}
	for name, src := range bad {
		if _, probs := compile(t, map[string]string{"main.ts": src}); len(probs) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
	_, probs := compile(t, map[string]string{"main.ts": "site.visitor(() => null)\n\nsite.visitor(() => null)\n"})
	if len(probs) != 1 || probs[0].Line != 3 {
		t.Fatalf("second visitor location: %+v", probs)
	}
}

// ---------------------------------------------------------------- filters

func TestPhase4Filters(t *testing.T) {
	e := phase3Engine()
	e.FilterTimeout, e.FilterRenderTimeout = 300*time.Millisecond, 50*time.Millisecond
	p := mustCompile(t, map[string]string{"main.ts": `
site.filter('post.content', (h) => h + '[b]', { priority: 20 })
site.filter('post.content', (h) => h + '[a]', { priority: 5 })
site.filter('post.content', (h, { post }) => h + '[c:' + post.title + ']', { priority: 20 })
site.filter('post.content', () => { throw new Error('broken filter') })
site.filter('page.head', (head, { path }) => ({ ...head, title: head.title + ' | ' + path }))
site.filter('page.head', () => { while (true) {} })
site.filter('page.head', (head) => { head.noindex = true })
site.filter('post.beforeSave', (p, { user, isNew }) => isNew && !p.excerpt ? { ...p, excerpt: 'auto by ' + (user ? user.role : 'none') } : p)
site.filter('post.beforeSave', (p) => { const s = Date.now(); while (Date.now() - s < 1000) {} return { error: 'never' } })
site.filter('mail.beforeSend', (m) => m.to.includes('blocked@x.io') ? null : m)
site.filter('collection.items', (items, { name, page }) => items.filter((i) => i.n % 2 === 1).map((i) => ({ ...i, tag: name + '#' + page })))
site.filter('action.input', (input) => ({ ...input, n: input.n * 10 }))
site.filter('action.result', (r, { name }) => ({ ...r, message: r.message + ' via ' + name }))
site.collection('nums', { access: ['*'], load: () => [1, 2, 3, 4].map((n) => ({ n })) })
site.action('calc', { input: { n: 'number' }, access: ['*'], run: ({ input }) => ({ message: 'n=' + input.n }) })
site.route('GET /custom', { access: ['*'] }, () => site.applyFilter('site.price', 10, { cur: 'THB' }))
site.filter('site.price', (v, { cur }) => v * 2 + (cur === 'THB' ? 1 : 0))
site.filter('site.price', () => { throw new Error('skip me') })
`})
	req := httptest.NewRequest("GET", "https://example.com/", nil)
	s := &Session{e: e, prog: p, r: req, user: &User{ID: 3, Role: "editor"}}
	defer s.Close()

	if out := s.ApplyFilter("post.content", "<p>x</p>", map[string]any{"post": map[string]any{"title": "T"}}); out != "<p>x</p>[a][b][c:T]" {
		t.Fatalf("order / priority / skip on error: %v", out)
	}
	start := time.Now()
	head := s.ApplyFilter("page.head", map[string]any{"title": "Home", "description": "", "image": "", "noindex": false}, map[string]any{"path": "/a"})
	hm, _ := head.(map[string]any)
	if hm["title"] != "Home | /a" || hm["noindex"] != true {
		t.Fatalf("page.head (timeout skipped, in-place mutation kept): %v", head)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("render filter time limit not applied: %v", time.Since(start))
	}
	bs := s.ApplyFilter("post.beforeSave", map[string]any{"title": "A", "excerpt": ""}, map[string]any{"isNew": true})
	if bm, _ := bs.(map[string]any); bm["excerpt"] != "auto by editor" || bm["error"] != nil {
		t.Fatalf("post.beforeSave (2 s limit skips the slow one): %v", bs)
	}
	if out := s.ApplyFilter("mail.beforeSend", map[string]any{"to": []any{"blocked@x.io"}}, nil); out != nil {
		t.Fatalf("mail.beforeSend cancel: %v", out)
	}
	if out := s.ApplyFilter("nothing.here", 5, nil); out != 5 {
		t.Fatal("no filter: value unchanged")
	}
	res := s.LoadCollection(design.CollectionReq{Source: "code:nums", Page: 1, PerPage: 10})
	if len(res.Items) != 2 || res.Items[1]["n"] != 3.0 || res.Items[0]["tag"] != "nums#1" {
		t.Fatalf("collection.items: %+v", res.Items)
	}
	if out := s.RunAction("calc", map[string]any{"n": 2.0}); out.Status != 200 || out.Message != "n=20 via calc" {
		t.Fatalf("action.input / action.result: %+v", out)
	}
	logs := ""
	for _, l := range s.logs {
		logs += l.Message + "\n"
	}
	if !strings.Contains(logs, "broken filter") || !strings.Contains(logs, "main.ts:") || !strings.Contains(logs, "timeout") {
		t.Fatalf("skipped filters are logged with file:line:\n%s", logs)
	}
	// site.applyFilter inside a handler
	r, _, err := call(t, e, p, "GET", "/custom", nil)
	if err != nil || r.Body != "21" {
		t.Fatalf("applyFilter: %v %v", r, err)
	}
	// a filter that is not 'site.*' cannot be applied from code
	p2 := mustCompile(t, map[string]string{"main.ts": "site.route('GET /x', { access: ['*'] }, () => site.applyFilter('post.content', 'x'))\n"})
	if _, _, err := call(t, e, p2, "GET", "/x", nil); err == nil {
		t.Fatal("applyFilter on a core point accepted")
	}
}

// ---------------------------------------------------------------- visitor

func TestPhase4Visitor(t *testing.T) {
	e := phase3Engine()
	p := mustCompile(t, map[string]string{"main.ts": `
site.visitor(({ request }) => {
  cache.set('calls', (cache.get('calls') || 0) + 1)
  const m = request.cookies.member
  if (m === 'boom') throw new Error('resolver broke')
  if (m === 'slow') { while (true) {} }
  return m ? { id: m, name: 'Member ' + m, groups: m === 'g1' ? ['gold'] : ['member'], plan: 'pro' } : null
})
site.collection('club', { access: ['group:gold'], load: ({ visitor }) => [{ who: visitor.name }] })
site.page('/p', { load: ({ visitor }) => ({ who: visitor ? visitor.id : 'anon' }) })
site.route('GET /me', { access: ['*'] }, ({ visitor }) => ({ v: visitor }))
site.route('GET /gold', { access: ['group:gold'] }, ({ visitor }) => visitor.name)
`})
	withCookie := func(v string) *http.Request {
		r := httptest.NewRequest("GET", "https://example.com/p", nil)
		if v != "" {
			r.AddCookie(&http.Cookie{Name: "pl_site_member", Value: v})
		}
		return r
	}
	calls := func() float64 {
		s, _ := e.h.cache.Get("calls")
		var n float64
		_ = jsonUnmarshal(s, &n)
		return n
	}
	s := &Session{e: e, prog: p, r: withCookie("g1")}
	if d := s.PageData("/p"); d["who"] != "g1" {
		t.Fatalf("ctx.visitor in page: %v", d)
	}
	if v := s.Visitor(); v["name"] != "Member g1" || len(groupsOf(v)) != 1 {
		t.Fatalf("visitor: %v", v)
	}
	if r := s.LoadCollection(design.CollectionReq{Source: "code:club", Page: 1}); r.Denied || len(r.Items) != 1 || r.Items[0]["who"] != "Member g1" {
		t.Fatalf("group access: %+v", r)
	}
	s.Close()
	if n := calls(); n != 1 {
		t.Fatalf("resolver ran %v times in one request (want 1)", n)
	}
	s = &Session{e: e, prog: p, r: withCookie("m2")}
	if r := s.LoadCollection(design.CollectionReq{Source: "code:club", Page: 1}); !r.Denied {
		t.Fatalf("member not in gold: %+v", r)
	}
	s.Close()
	// lazy: nothing asks for the visitor → the resolver does not run
	before := calls()
	s = &Session{e: e, prog: p, r: withCookie("g1")}
	s.LoadCollection(design.CollectionReq{Source: "code:nope"})
	s.Close()
	if calls() != before {
		t.Fatal("resolver ran without being needed")
	}
	// errors and timeouts → null (logged)
	for _, c := range []string{"boom", "slow"} {
		s = &Session{e: e, prog: p, r: withCookie(c)}
		start := time.Now()
		if v := s.Visitor(); v != nil {
			t.Fatalf("%s: visitor %v", c, v)
		}
		if time.Since(start) > time.Second {
			t.Fatalf("%s: 200 ms limit not applied", c)
		}
		if d := s.PageData("/p"); d["who"] != "anon" {
			t.Fatalf("%s: page after failed resolver: %v %s", c, d, s.LastError)
		}
		if len(s.logs) == 0 {
			t.Fatalf("%s: not logged", c)
		}
		s.Close()
	}
	// a CMS user is checked by role, never by groups
	s = &Session{e: e, prog: p, r: withCookie("g1"), user: &User{ID: 2, Role: "author"}}
	if r := s.LoadCollection(design.CollectionReq{Source: "code:club", Page: 1}); !r.Denied {
		t.Fatal("CMS user matched a group entry")
	}
	s.Close()
	// preview: fixed visitor
	s = &Session{e: e, prog: p, r: withCookie("")}
	s.SetVisitor(map[string]any{"id": "preview", "groups": []any{"gold"}})
	if r := s.LoadCollection(design.CollectionReq{Source: "code:club", Page: 1}); r.Denied {
		t.Fatal("preview visitor in gold denied")
	}
	s.Close()
	// routes: ctx.visitor (lazy getter, inside the handler)
	res, _, err := call(t, e, p, "GET", "/me", nil)
	if err != nil || res.Body != `{"v":null}` {
		t.Fatalf("route without cookie: %v %v", res, err)
	}
	if !allowedFor([]string{"group:gold"}, nil, func() []string { return []string{"gold"} }) ||
		allowedFor([]string{"group:gold"}, nil, func() []string { return []string{"member"} }) ||
		allowedFor([]string{"group:gold"}, &User{Role: "editor"}, func() []string { return []string{"gold"} }) ||
		!allowedFor([]string{"group:gold"}, &User{Role: "administrator"}, nil) {
		t.Fatal("allowedFor rules")
	}
}

// ---------------------------------------------------------------- mail

func TestPhase4Mail(t *testing.T) {
	e := phase3Engine()
	e.h.e = e
	e.mailLimit = newRateLimiter(2, time.Hour)
	var sent []MailMessage
	e.cfg.Mailer = func(m MailMessage) error { sent = append(sent, m); return nil }
	p := mustCompile(t, map[string]string{"main.ts": `
site.filter('mail.beforeSend', (m, { source }) => m.to[0] === 'stop@x.io' ? null : { ...m, subject: '[site] ' + m.subject + ' (' + source + ')' })
site.route('GET /send', { access: ['*'] }, ({ request }) => mail.send({ to: request.query.to, subject: 'Hi', text: 'hello' }))
site.route('GET /many', { access: ['*'] }, () => mail.send({ to: Array.from({ length: 21 }, (_, i) => 'u' + i + '@x.io'), subject: 's', html: '<b>x</b>' }))
site.route('GET /bad', { access: ['*'] }, () => mail.send({ to: 'not-an-email', subject: 's', html: 'x' }))
`})
	r, logs, err := call(t, e, p, "GET", "/send?to=a@x.io", nil)
	if err != nil || r.Body != `{"ok":true}` || len(sent) != 1 || sent[0].Subject != "[site] Hi (route GET /send)" || sent[0].Text != "hello" {
		t.Fatalf("send: %v %v %+v", r, err, sent)
	}
	if len(logs) == 0 || strings.Contains(logs[len(logs)-1].Message, "hello") {
		t.Fatalf("logged without the body: %+v", logs)
	}
	if r, _, _ := call(t, e, p, "GET", "/send?to=stop@x.io", nil); r.Body != `{"cancelled":true,"ok":false}` || len(sent) != 1 {
		t.Fatalf("cancelled by filter: %v", r)
	}
	if _, _, err := call(t, e, p, "GET", "/many", nil); err == nil || !strings.Contains(err.Error(), "20") {
		t.Fatalf("21 recipients: %v", err)
	}
	if _, _, err := call(t, e, p, "GET", "/bad", nil); err == nil {
		t.Fatal("bad address accepted")
	}
	call(t, e, p, "GET", "/send?to=b@x.io", nil)
	if _, _, err := call(t, e, p, "GET", "/send?to=c@x.io", nil); err == nil || !strings.Contains(err.Error(), "โควตา") {
		t.Fatalf("rate limit: %v (sent %d)", err, len(sent))
	}
	e.cfg.Demo = true
	if _, _, err := call(t, e, p, "GET", "/send?to=a@x.io", nil); err == nil {
		t.Fatal("mail in demo mode")
	}
	e.cfg.Demo, e.cfg.Mailer = false, nil
	e.mailLimit = newRateLimiter(10, time.Hour)
	if _, _, err := call(t, e, p, "GET", "/send?to=a@x.io", nil); err == nil || !strings.Contains(err.Error(), "ตั้งค่าระบบอีเมล") {
		t.Fatalf("unconfigured SMTP: %v", err)
	}
}

// ---------------------------------------------------------------- http.fetch

func TestPhase4Fetch(t *testing.T) {
	big := strings.Repeat("x", maxFetchBytes+10)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Echo", r.Header.Get("X-Key"))
			_, _ = w.Write([]byte(`{"ok":true,"method":"` + r.Method + `"}`))
		case "/big":
			_, _ = w.Write([]byte(big))
		case "/slow":
			time.Sleep(1500 * time.Millisecond)
		case "/away":
			http.Redirect(w, r, "https://elsewhere.example/x", http.StatusFound)
		case "/here":
			http.Redirect(w, r, "/json", http.StatusFound)
		}
	}))
	defer srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	e := phase3Engine()
	e.h.e = e
	e.RouteTimeout = 5 * time.Second
	e.fetchHook = &fetchHook{addrs: map[string]string{"api.example.test:443": srv.Listener.Addr().String()},
		rootCAs: &tls.Config{RootCAs: pool, ServerName: "example.com"}, hosts: []string{"*.example.test", "localhost"}}
	p := mustCompile(t, map[string]string{"main.ts": `
site.route('GET /f', { access: ['*'] }, ({ request }) => {
  const r = http.fetch(request.query.u, { method: request.query.m || 'GET', headers: { 'X-Key': 'k1' }, timeout: Number(request.query.t || 5000) })
  return { status: r.status, ok: r.ok, echo: r.headers['x-echo'] || null, json: r.headers['content-type'] === 'application/json' ? r.json() : null, len: r.body.length }
})
`})
	get := func(q string) (*Response, error) {
		r, _, err := call(t, e, p, "GET", "/f?"+q, nil)
		return r, err
	}
	r, err := get("u=https://api.example.test/json&m=POST")
	if err != nil || r.Body != `{"status":200,"ok":true,"echo":"k1","json":{"ok":true,"method":"POST"},"len":27}` {
		t.Fatalf("allowlisted fetch: %v %v", r, err)
	}
	if r, err := get("u=https://api.example.test/here"); err != nil || !strings.Contains(r.Body, `"method":"GET"`) {
		t.Fatalf("same-host redirect: %v %v", r, err)
	}
	cases := map[string]string{
		"u=http://api.example.test/json":          "https",
		"u=https://other.test/json":               "ไม่อยู่ในรายการ",
		"u=https://127.0.0.1/json":                "IP",
		"u=https://localhost/json":                "ภายใน",
		"u=https://api.example.test/big":          "5 MB",
		"u=https://api.example.test/slow&t=300":   "หมดเวลา",
		"u=https://api.example.test/away":         "redirect",
		"u=https://api.example.test/json&t=0":     "timeout",
		"u=https://api.example.test/json&t=20000": "timeout",
	}
	for q, want := range cases {
		start := time.Now()
		_, err := get(q)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v (want %q)", q, err, want)
		}
		if time.Since(start) > 3*time.Second {
			t.Errorf("%s: took %v", q, time.Since(start))
		}
	}
	if !hostAllowed("a.b.example.com", []string{"*.example.com"}) || hostAllowed("example.com", []string{"*.example.com"}) ||
		hostAllowed("evil-example.com", []string{"*.example.com"}) || !hostAllowed("API.example.com", []string{"api.example.com"}) {
		t.Fatal("hostAllowed")
	}
	if _, err := cleanHosts([]string{"10.0.0.1"}); err == nil {
		t.Fatal("IP accepted in the allowlist")
	}
	if h, err := cleanHosts([]string{" API.Example.com ", "*.x.io", "api.example.com"}); err != nil || strings.Join(h, ",") != "*.x.io,api.example.com" {
		t.Fatalf("cleanHosts: %v %v", h, err)
	}
	_ = net.IPv4len
}

// ---------------------------------------------------------------- secrets

func TestPhase4SecretBox(t *testing.T) {
	dir := t.TempDir()
	k1, err := loadSecretKey("", dir)
	if err != nil || len(k1) != 32 {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, secretKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode %v", fi.Mode().Perm())
	}
	k2, _ := loadSecretKey("", dir)
	if string(k1) != string(k2) {
		t.Fatal("key file not reused")
	}
	sealed, err := sealSecret(k1, "s3cret")
	if err != nil || !strings.HasPrefix(sealed, "enc:v1:") || strings.Contains(sealed, "s3cret") {
		t.Fatalf("seal: %q %v", sealed, err)
	}
	if v, err := openSecret(k1, sealed); err != nil || v != "s3cret" {
		t.Fatalf("open: %q %v", v, err)
	}
	other, _ := loadSecretKey("env-key", "")
	if _, err := openSecret(other, sealed); err == nil {
		t.Fatal("opened with another key")
	}
}

// ---------------------------------------------------------------- cron

func TestPhase4Cron(t *testing.T) {
	bkk, _ := time.LoadLocation("Asia/Bangkok")
	at := func(s string) time.Time { x, _ := time.Parse(time.RFC3339, s); return x }
	cases := []struct{ expr, after, want string }{
		{"0 8 * * *", "2026-10-24T00:30:00Z", "2026-10-24T01:00:00Z"}, // 08:00 Bangkok = 01:00 UTC
		{"0 8 * * *", "2026-10-24T01:00:00Z", "2026-10-25T01:00:00Z"}, // strictly after
		{"*/15 * * * *", "2026-10-24T00:07:00Z", "2026-10-24T00:15:00Z"},
		{"30 9 1 * *", "2026-10-24T00:00:00Z", "2026-11-01T02:30:00Z"},
		{"0 0 * * MON", "2026-10-24T00:00:00Z", "2026-10-25T17:00:00Z"}, // Mon 26 Oct 00:00 BKK
		{"0 12 13 * 5", "2026-10-24T00:00:00Z", "2026-10-30T05:00:00Z"}, // dom OR dow: Fri 30 Oct
		{"@hourly", "2026-10-24T00:59:00Z", "2026-10-24T01:00:00Z"},
		{"0 0 29 2 *", "2026-10-24T00:00:00Z", "2028-02-28T17:00:00Z"},
		{"0-10/5 1-2 * jan-mar *", "2026-10-24T00:00:00Z", "2026-12-31T18:00:00Z"},
	}
	for _, c := range cases {
		s, err := ParseCron(c.expr)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if got := s.Next(at(c.after), bkk); !got.Equal(at(c.want)) {
			t.Errorf("%s after %s = %s, want %s", c.expr, c.after, got.Format(time.RFC3339), c.want)
		}
	}
	for _, bad := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8", "*/0 * * * *", "5-1 * * * *", "a b c d e"} {
		if _, err := ParseCron(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if n, ok := cronOf("cron:send@2:1761264000"); !ok || n != "send@2" {
		t.Fatal("cronOf")
	}
}

// ---------------------------------------------------------------- meta validation

func TestPhase4ValidateMeta(t *testing.T) {
	min0 := 0.0
	fields := []FieldInfo{
		{Key: "date", Type: "datetime", Required: true, Label: map[string]string{"th": "วันที่"}},
		{Key: "seats", Type: "number", Min: &min0},
		{Key: "kind", Type: "select", Options: []FieldOption{{Value: "talk"}}},
		{Key: "site", Type: "url"}, {Key: "mail", Type: "email"}, {Key: "ok", Type: "bool"}, {Key: "day", Type: "date"},
		{Key: "img", Type: "image"}, {Key: "data", Type: "json"}, {Key: "note", Type: "textarea"},
	}
	out, err := ValidateMeta(fields, map[string]any{"date": "2026-11-01T10:00:00", "seats": "12", "kind": "talk", "site": "https://x.io",
		"mail": "a@b.co", "ok": "on", "day": "2026-02-28", "img": "/uploads/a.png", "data": map[string]any{"a": 1.0}, "unknown": 1, "note": ""}, nil)
	if err != nil || out["seats"] != 12.0 || out["ok"] != true || out["note"] != nil || len(out) != 10 {
		t.Fatalf("valid: %v %v", out, err)
	}
	if _, ok := out["unknown"]; ok {
		t.Fatal("unknown key kept")
	}
	bad := []map[string]any{
		{"seats": -1}, {"date": "x"}, {"date": "2026-11-01T10:00", "kind": "nope"}, {"date": "2026-11-01T10:00", "site": "javascript:alert(1)"},
		{"date": "2026-11-01T10:00", "mail": "x"}, {"date": "2026-11-01T10:00", "day": "2026-02-30"}, {"date": ""},
	}
	for _, in := range bad {
		if _, err := ValidateMeta(fields, in, map[string]any{"date": "2026-01-01T00:00"}); err == nil {
			t.Errorf("%v accepted", in)
		}
	}
	if _, err := ValidateMeta(fields, map[string]any{"seats": 1}, nil); err == nil || err.(*MetaError).Key != "date" {
		t.Fatalf("required on create: %v", err)
	}
	if _, err := ValidateMeta(fields, map[string]any{"seats": 1}, map[string]any{"date": "2026-01-01T00:00"}); err != nil {
		t.Fatalf("required satisfied by stored value: %v", err)
	}
}
