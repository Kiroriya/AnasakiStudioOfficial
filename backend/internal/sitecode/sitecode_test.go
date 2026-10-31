package sitecode

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- files

func TestValidPath(t *testing.T) {
	for _, p := range []string{"main.ts", "lib/format.ts", "a/b/c.json", "README.md", "x.mjs", "A-b_c.1.js"} {
		if err := ValidPath(p); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for _, p := range []string{"", "/main.ts", "../main.ts", "a/../b.ts", `a\b.ts`, "a//b.ts", "./main.ts",
		"main.exe", "main", "main.TSX", "lib/ไทย.ts", "a b.ts", strings.Repeat("a", 260) + ".ts", "lib/.", "x.php"} {
		if ValidPath(p) == nil {
			t.Errorf("%q should be refused", p)
		}
	}
	if d, err := CleanDir("lib/"); err != nil || d != "lib" {
		t.Errorf("CleanDir = %q %v", d, err)
	}
	if _, err := CleanDir("../x"); err == nil {
		t.Error("CleanDir must refuse ..")
	}
}

func TestCheckSet(t *testing.T) {
	if err := CheckSet(map[string]string{"main.ts": "", "Main.ts": ""}); err == nil {
		t.Error("case-insensitive duplicate accepted")
	}
	if err := CheckSet(map[string]string{"lib.ts": "", "lib.ts/x.ts": ""}); err == nil {
		t.Error("file used as folder accepted")
	}
	many := map[string]string{}
	for i := 0; i <= MaxFiles; i++ {
		many["f"+itoa(i)+".ts"] = ""
	}
	if err := CheckSet(many); err == nil {
		t.Error("more than 200 files accepted")
	}
	if err := CheckSet(map[string]string{"big.ts": strings.Repeat("x", MaxFileBytes+1)}); err == nil {
		t.Error("file over 512 KB accepted")
	}
	total := map[string]string{}
	for i := 0; i < 5; i++ {
		total["f"+itoa(i)+".ts"] = strings.Repeat("x", 500<<10)
	}
	if err := CheckSet(total); err == nil {
		t.Error("over 2 MB accepted")
	}
	if err := CheckSet(map[string]string{"main.ts": "x", "lib/a.ts": "y"}); err != nil {
		t.Error(err)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, c := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(c))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadZip(t *testing.T) {
	files, err := ReadZip(makeZip(t, map[string]string{
		"proj/main.ts": "a", "proj/lib/x.ts": "b", "proj/anasaki.d.ts": "types", "__MACOSX/proj/._main.ts": "junk", "proj/.DS_Store": "j",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files["main.ts"] != "a" || files["lib/x.ts"] != "b" {
		t.Fatalf("top folder not stripped / junk kept: %v", files)
	}
	if files, err := ReadZip(makeZip(t, map[string]string{"main.ts": "a", "lib/x.ts": "b"})); err != nil || len(files) != 2 {
		t.Fatalf("flat zip: %v %v", files, err)
	}
	for name, entries := range map[string]map[string]string{
		"zip-slip":  {"../evil.ts": "x", "main.ts": ""},
		"zip-slip2": {"proj/../../evil.ts": "x"},
		"absolute":  {"/etc/x.ts": "x"},
		"backslash": {`a\..\..\x.ts`: "x"},
		"bad ext":   {"main.ts": "", "run.exe": "MZ"},
		"html":      {"index.html": "<script>"},
		"too big":   {"main.ts": strings.Repeat("x", MaxFileBytes+1)},
		"case dup":  {"main.ts": "", "MAIN.ts": ""},
	} {
		if _, err := ReadZip(makeZip(t, entries)); err == nil {
			t.Errorf("%s: zip accepted", name)
		}
	}
	if _, err := ReadZip([]byte("not a zip")); err == nil {
		t.Error("garbage accepted as zip")
	}
	// round trip
	src := map[string]string{"main.ts": "x", "lib/a.ts": "y", "README.md": "z"}
	var buf bytes.Buffer
	if err := WriteZip(&buf, src, ""); err != nil {
		t.Fatal(err)
	}
	back, err := ReadZip(buf.Bytes())
	if err != nil || !sameFiles(src, back) {
		t.Fatalf("round trip: %v %v", back, err)
	}
	var buf2 bytes.Buffer
	_ = WriteZip(&buf2, src, "")
	if !bytes.Equal(buf.Bytes(), buf2.Bytes()) {
		t.Error("zip output is not deterministic")
	}
}

func TestComputeDiff(t *testing.T) {
	draft := map[string]string{"main.ts": "1", "lib/a.ts": "a", "lib/b.ts": "b", "other.ts": "o"}
	in := map[string]string{"main.ts": "2", "lib/a.ts": "a", "new.ts": "n"}
	d, res := ComputeDiff(draft, in, false, "")
	if strings.Join(d.Added, ",") != "new.ts" || strings.Join(d.Changed, ",") != "main.ts" || len(d.Removed) != 0 || d.Unchanged != 1 || len(res) != 5 {
		t.Fatalf("merge diff: %+v %v", d, res)
	}
	d, res = ComputeDiff(draft, in, true, "")
	if strings.Join(d.Removed, ",") != "lib/b.ts,other.ts" || len(res) != 3 {
		t.Fatalf("replace diff: %+v %v", d, res)
	}
	d, _ = ComputeDiff(draft, map[string]string{"lib/a.ts": "a"}, true, "lib")
	if strings.Join(d.Removed, ",") != "lib/b.ts" {
		t.Fatalf("scoped replace: %+v", d)
	}
}

// ---------------------------------------------------------------- build

func TestBuildTypeScriptImports(t *testing.T) {
	res := Build(map[string]string{
		"main.ts":       "import { greet } from './lib/greet'\nimport data from './data.json'\nexport const out: string = greet(data.name);\n(globalThis as any).out = out\n",
		"lib/greet.ts":  "import { upper } from '../util'\nexport function greet(n: string): string { return 'hi ' + upper(n) }\n",
		"util/index.ts": "export const upper = (s: string) => s.toUpperCase()\n",
		"data.json":     `{"name":"bob"}`,
	})
	if !res.OK() {
		t.Fatalf("build errors: %+v", res.Errors)
	}
	if !strings.Contains(res.Bundle, "sourceMappingURL=data:application/json;base64,") || !strings.Contains(res.SourceMap, `"lib/greet.ts"`) {
		t.Fatalf("source map: %s", res.SourceMap)
	}
	v := newVM(&host{cache: NewCache()})
	if _, err := v.rt.RunString(res.Bundle); err != nil {
		t.Fatal(err)
	}
	if got := v.rt.Get("out").String(); got != "hi BOB" {
		t.Fatalf("out = %q", got)
	}
}

func TestBuildErrors(t *testing.T) {
	res := Build(map[string]string{"main.ts": "import { f } from './lib/x'\nf()\n", "lib/x.ts": "export function f() {\n  const a = = 1\n}\n"})
	if res.OK() || res.Errors[0].File != "lib/x.ts" || res.Errors[0].Line != 2 || res.Errors[0].Column < 1 {
		t.Fatalf("syntax error location: %+v", res.Errors)
	}
	if res := Build(map[string]string{"lib.ts": "x"}); res.OK() || !strings.Contains(res.Errors[0].Message, "main.ts") {
		t.Fatalf("missing entry: %+v", res.Errors)
	}
	res = Build(map[string]string{"main.js": "import x from 'lodash'\nx()\n"})
	if res.OK() || res.Errors[0].File != "main.js" || res.Errors[0].Line != 1 {
		t.Fatalf("package import: %+v", res.Errors)
	}
	res = Build(map[string]string{"main.js": "import y from './nope'\ny()\n"})
	if res.OK() || !strings.Contains(res.Errors[0].Message, "./nope") {
		t.Fatalf("missing file: %+v", res.Errors)
	}
	if res := Build(map[string]string{"index.js": "1"}); !res.OK() {
		t.Fatalf("index.js entry: %+v", res.Errors)
	}
	// async generators are lowered (goja lacks them)
	res = Build(map[string]string{"main.ts": "async function* g(){ yield 1 }\n(globalThis as any).ok = typeof g\n"})
	if !res.OK() {
		t.Fatal(res.Errors)
	}
	if _, err := newVM(&host{}).rt.RunString(res.Bundle); err != nil {
		t.Fatalf("lowered async generator: %v", err)
	}
}

// ---------------------------------------------------------------- registration

var testReserved = Reserved(map[string]string{"blog_base": "news"}, []string{"/shop"})

func compile(t *testing.T, files map[string]string) (*Program, []Problem) {
	t.Helper()
	res := Build(files)
	if !res.OK() {
		t.Fatalf("build: %+v", res.Errors)
	}
	return Register(res.Bundle, testReserved, &host{cache: NewCache()}, time.Second)
}

func mustCompile(t *testing.T, files map[string]string) *Program {
	t.Helper()
	p, probs := compile(t, files)
	if len(probs) > 0 {
		t.Fatalf("registration: %+v", probs)
	}
	return p
}

func TestRegistration(t *testing.T) {
	p := mustCompile(t, map[string]string{"main.ts": `
site.route('GET /hello/{name}', { access: ['*'] }, (c) => 'hi')
site.route('POST /hello/{name}', (c) => 'x')
site.route('GET /files/*', { access: ['editor'] }, () => 1)
site.route('GET /', { access: ['*'] }, () => 'home')
site.on('post.published', () => {})
site.on('post.published', () => {})
site.on('user.created', () => {})
log.info('registering')
`})
	m := p.Manifest
	if len(m.Routes) != 4 || m.Routes[0].Path != "/hello/{name}" || m.Routes[0].Access[0] != "*" ||
		m.Routes[1].Access[0] != "administrator" || m.Routes[2].Access[0] != "editor" {
		t.Fatalf("manifest routes: %+v", m.Routes)
	}
	if strings.Join(m.Events, ",") != "post.published,user.created" {
		t.Fatalf("events: %v", m.Events)
	}

	cases := map[string]string{
		"duplicate":        "site.route('GET /a/{x}', () => 1)\nsite.route('GET /a/{y}', () => 2)\n",
		"reserved api":     "site.route('GET /api/x', () => 1)\n",
		"reserved admin":   "site.route('GET /admin', () => 1)\n",
		"reserved uploads": "site.route('GET /uploads/{f}', () => 1)\n",
		"reserved blog":    "site.route('GET /news/x', () => 1)\n",
		"reserved service": "site.route('GET /shop/cart', () => 1)\n",
		"reserved robots":  "site.route('GET /robots.txt', () => 1)\n",
		"bad method":       "site.route('FETCH /x', () => 1)\n",
		"no slash":         "site.route('GET x', () => 1)\n",
		"trailing slash":   "site.route('GET /x/', () => 1)\n",
		"no handler":       "site.route('GET /x')\n",
		"bad access":       "site.route('GET /x', { access: ['Bad Role'] }, () => 1)\n",
		"db at top level":  "db.posts.list()\n",
		"crypto at top":    "crypto.uuid()\n",
		"throw":            "throw new Error('nope')\n",
		"bad event":        "site.on('', () => 1)\n",
		"conflict":         "site.route('GET /a/{x}/b', () => 1)\nsite.route('GET /a/b/{y}', () => 1)\n",
	}
	for name, src := range cases {
		if _, probs := compile(t, map[string]string{"main.ts": src}); len(probs) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
	// problems point at the original file:line
	_, probs := compile(t, map[string]string{"main.ts": "import './routes'\n", "routes.ts": "\n\nsite.route('GET /api/x', () => 1)\n"})
	if len(probs) != 1 || probs[0].File != "routes.ts" || probs[0].Line != 3 || !strings.Contains(probs[0].Message, "สงวน") {
		t.Fatalf("reserved location: %+v", probs)
	}
	_, probs = compile(t, map[string]string{"main.ts": "const x: number = 1\n\ndb.posts.list()\n"})
	if len(probs) != 1 || probs[0].File != "main.ts" || probs[0].Line != 3 {
		t.Fatalf("db during registration: %+v", probs)
	}
	_, probs = compile(t, map[string]string{"main.ts": "site.route('GET /a', () => 1)\nsite.route('GET /a', () => 2)\n"})
	if len(probs) != 1 || probs[0].Line != 2 || !strings.Contains(probs[0].Message, "ซ้ำ") {
		t.Fatalf("duplicate location: %+v", probs)
	}
	// an infinite loop at the top level is stopped
	start := time.Now()
	if _, probs := compile(t, map[string]string{"main.ts": "while (true) {}\n"}); len(probs) == 0 || !strings.Contains(probs[0].Message, "timeout") {
		t.Fatalf("top-level loop: %+v", probs)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout took too long")
	}
}

// ---------------------------------------------------------------- runtime

func testEngine() *Engine {
	return &Engine{RouteTimeout: 300 * time.Millisecond, EventTimeout: time.Second, QueueWait: time.Second,
		sem: make(chan struct{}, maxConcurrent), breaker: NewBreaker(), h: &host{cache: NewCache()}}
}

// call runs route `spec` of p in a fresh VM like a live request.
func call(t *testing.T, e *Engine, p *Program, method, target string, user *User) (*Response, []LogLine, error) {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	idx, ps := -1, map[string]string{}
	mux, err := newMux(p.specs, func(i int) http.Handler {
		return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { idx, ps = i, params(p.specs[i], r) })
	})
	if err != nil {
		t.Fatal(err)
	}
	mux.ServeHTTP(httptest.NewRecorder(), req)
	if idx < 0 {
		t.Fatalf("no route for %s %s", method, target)
	}
	v, err := e.newLoaded(p)
	if err != nil {
		t.Fatal(err)
	}
	return e.execRoute(v, p.specs[idx].Pattern, req, ps, user)
}

func TestRuntimeResponsesAndIsolation(t *testing.T) {
	e := testEngine()
	p := mustCompile(t, map[string]string{"main.ts": `
let n = 0
site.route('GET /count', { access: ['*'] }, () => { n++; (globalThis as any).leak = ((globalThis as any).leak || 0) + 1; return { n, leak: (globalThis as any).leak } })
site.route('GET /hello/{name}', { access: ['*'] }, ({ request }) => '<b>' + request.params.name + '</b>')
site.route('GET /raw', { access: ['*'] }, () => ({ status: 201, headers: { 'X-Test': 'yes', 'Content-Type': 'text/plain' }, body: 'made' }))
site.route('GET /obj', { access: ['*'] }, () => ({ status: 'ok', items: [1, 2] }))
site.route('GET /none', { access: ['*'] }, () => {})
site.route('GET /async', { access: ['*'] }, async ({ request }) => { const x = await Promise.resolve(2); return { x, q: request.query.a } })
site.route('GET /files/*', { access: ['*'] }, ({ request }) => request.params['*'])
site.route('GET /me', (c) => ({ user: c.user && c.user.name, can: c.user && c.user.can('edit_pages') }))
site.route('GET /log', { access: ['*'] }, () => { console.log('hello', { a: 1 }); log.error(new Error('bad')); return 'ok' })
`})
	for i := 0; i < 2; i++ {
		res, _, err := call(t, e, p, "GET", "/count", nil)
		if err != nil || res.Body != `{"n":1,"leak":1}` {
			t.Fatalf("call %d: state leaked between requests: %v %v", i, res, err)
		}
	}
	res, _, _ := call(t, e, p, "GET", "/hello/bob", nil)
	if res.Status != 200 || res.Body != "<b>bob</b>" || !strings.HasPrefix(res.Headers["Content-Type"], "text/html") {
		t.Fatalf("string response: %+v", res)
	}
	res, _, _ = call(t, e, p, "GET", "/raw", nil)
	if res.Status != 201 || res.Body != "made" || res.Headers["X-Test"] != "yes" || res.Headers["Content-Type"] != "text/plain" {
		t.Fatalf("full response: %+v", res)
	}
	res, _, _ = call(t, e, p, "GET", "/obj", nil)
	if res.Status != 200 || res.Body != `{"status":"ok","items":[1,2]}` || !strings.HasPrefix(res.Headers["Content-Type"], "application/json") {
		t.Fatalf("object response: %+v", res)
	}
	if res, _, _ = call(t, e, p, "GET", "/none", nil); res.Status != 204 {
		t.Fatalf("undefined → %d", res.Status)
	}
	if res, _, _ = call(t, e, p, "GET", "/async?a=z", nil); res.Body != `{"x":2,"q":"z"}` {
		t.Fatalf("async: %+v", res)
	}
	if res, _, _ = call(t, e, p, "GET", "/files/a/b/c.txt", nil); res.Body != "a/b/c.txt" {
		t.Fatalf("wildcard: %+v", res)
	}
	if res, _, _ = call(t, e, p, "GET", "/me", &User{ID: 1, Name: "Ann", Role: "administrator"}); res.Body != `{"user":"Ann","can":true}` {
		t.Fatalf("user: %+v", res)
	}
	_, logs, _ := call(t, e, p, "GET", "/log", nil)
	if len(logs) != 2 || logs[0].Level != "info" || logs[0].Message != `hello {"a":1}` || logs[1].Level != "error" || logs[1].Message != "Error: bad" {
		t.Fatalf("logs: %+v", logs)
	}
	if !allowed([]string{"administrator"}, &User{Role: "administrator"}) || allowed([]string{"administrator"}, nil) ||
		allowed([]string{"editor"}, &User{Role: "author"}) || !allowed([]string{"editor"}, &User{Role: "editor"}) || !allowed([]string{"*"}, nil) {
		t.Error("access rules")
	}
}

func TestRuntimeTimeoutAndErrors(t *testing.T) {
	e := testEngine()
	p := mustCompile(t, map[string]string{
		"main.ts": "import { boom } from './lib/boom'\n" +
			"site.route('GET /loop', { access: ['*'] }, () => { while (true) {} })\n" +
			"site.route('GET /loop2', { access: ['*'] }, async () => { await null; for (;;) {} })\n" +
			"site.route('GET /boom', { access: ['*'] }, () => boom())\n" +
			"site.route('GET /reject', { access: ['*'] }, async () => { await null; boom() })\n" +
			"site.route('GET /deep', { access: ['*'] }, () => { const f = (): number => f(); return f() })\n" +
			"site.route('GET /pending', { access: ['*'] }, () => new Promise(() => {}))\n",
		"lib/boom.ts": "// helper\nexport function boom(): never {\n  throw new Error('kaboom')\n}\n",
	})
	for _, path := range []string{"/loop", "/loop2"} {
		start := time.Now()
		_, _, err := call(t, e, p, "GET", path, nil)
		re := toRunError(err)
		if err == nil || !re.Timeout {
			t.Fatalf("%s: infinite loop not stopped: %v", path, err)
		}
		if time.Since(start) > 3*time.Second {
			t.Fatalf("%s: timeout too slow", path)
		}
	}
	_, _, err := call(t, e, p, "GET", "/boom", nil)
	re := toRunError(err)
	if re.At.File != "lib/boom.ts" || re.At.Line != 3 || !strings.Contains(re.Message, "kaboom") ||
		!strings.Contains(re.Trace, "lib/boom.ts:3") || !strings.Contains(re.Trace, "main.ts:4") || strings.Contains(re.Trace, "bundle.js") {
		t.Fatalf("source-mapped stack: %+v", re)
	}
	_, _, err = call(t, e, p, "GET", "/reject", nil)
	if re := toRunError(err); re.At.File != "lib/boom.ts" || re.At.Line != 3 || !strings.Contains(re.Message, "kaboom") {
		t.Fatalf("rejected promise: %+v", re)
	}
	if _, _, err = call(t, e, p, "GET", "/deep", nil); err == nil {
		t.Fatal("stack overflow not reported")
	}
	if _, _, err = call(t, e, p, "GET", "/pending", nil); err == nil || !strings.Contains(err.Error(), "Promise") {
		t.Fatalf("pending promise: %v", err)
	}
}

func TestRuntimeCryptoAndCache(t *testing.T) {
	e := testEngine()
	e.RouteTimeout = 5 * time.Second // bcrypt
	clock := time.Unix(1000, 0)
	e.h.cache.Now = func() time.Time { return clock }
	p := mustCompile(t, map[string]string{"main.ts": `
site.route('GET /crypto', { access: ['*'] }, () => {
  const pw = crypto.password.hash('secret')
  return {
    sha256: crypto.hash('sha256', 'abc'),
    sha1: crypto.hash('sha1', 'abc'),
    md5: crypto.hash('md5', 'abc'),
    hmac: crypto.hmac('sha256', 'key', 'The quick brown fox jumps over the lazy dog'),
    rnd: crypto.random(8).length,
    uuid: crypto.uuid(),
    eq: crypto.timingSafeEqual('a', 'a') && !crypto.timingSafeEqual('a', 'b'),
    pw: crypto.password.verify('secret', pw) && !crypto.password.verify('wrong', pw),
  }
})
site.route('GET /badalg', { access: ['*'] }, () => crypto.hash('sha3', 'x'))
site.route('GET /set', { access: ['*'] }, () => { cache.set('k', { v: [1, 'a'] }, 10); cache.set('forever', 1); return 'ok' })
site.route('GET /get', { access: ['*'] }, () => ({ k: cache.get('k') ?? null, f: cache.get('forever') ?? null }))
site.route('GET /del', { access: ['*'] }, () => { cache.delete('forever'); return 'ok' })
`})
	res, _, err := call(t, e, p, "GET", "/crypto", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"sha256":"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"`,
		`"sha1":"a9993e364706816aba3e25717850c26c9cd0d89d"`,
		`"md5":"900150983cd24fb0d6963f7d28e17f72"`,
		`"hmac":"f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"`,
		`"rnd":16`, `"eq":true`, `"pw":true`,
	} {
		if !strings.Contains(res.Body, want) {
			t.Errorf("crypto: missing %s in %s", want, res.Body)
		}
	}
	if !regexp.MustCompile(`"uuid":"[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}"`).MatchString(res.Body) {
		t.Errorf("uuid: %s", res.Body)
	}
	if _, _, err := call(t, e, p, "GET", "/badalg", nil); err == nil {
		t.Error("unknown algorithm accepted")
	}
	call(t, e, p, "GET", "/set", nil)
	if res, _, _ := call(t, e, p, "GET", "/get", nil); res.Body != `{"k":{"v":[1,"a"]},"f":1}` {
		t.Fatalf("cache shared between requests: %s", res.Body)
	}
	clock = clock.Add(11 * time.Second)
	if res, _, _ := call(t, e, p, "GET", "/get", nil); res.Body != `{"k":null,"f":1}` {
		t.Fatalf("cache TTL: %s", res.Body)
	}
	call(t, e, p, "GET", "/del", nil)
	if res, _, _ := call(t, e, p, "GET", "/get", nil); res.Body != `{"k":null,"f":null}` {
		t.Fatalf("cache delete: %s", res.Body)
	}
}

func TestCacheCap(t *testing.T) {
	c := NewCache()
	c.Cap = 100
	_ = c.Set("a", strings.Repeat("x", 40), 0)
	_ = c.Set("b", strings.Repeat("x", 40), 0)
	_ = c.Set("c", strings.Repeat("x", 40), 0) // evicts a
	if _, ok := c.Get("a"); ok {
		t.Error("oldest entry not evicted")
	}
	if _, ok := c.Get("c"); !ok {
		t.Error("new entry missing")
	}
	if err := c.Set("big", strings.Repeat("x", 200), 0); err == nil {
		t.Error("value over the cap accepted")
	}
}

func TestBreaker(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewBreaker()
	b.Now = func() time.Time { return now }
	for i := 0; i < 19; i++ {
		if b.Fail("k") {
			t.Fatal("opened too early")
		}
		now = now.Add(time.Second)
	}
	if !b.Allow("k") {
		t.Fatal("closed breaker refused")
	}
	now = now.Add(45 * time.Second) // the first failures leave the 1-minute window
	if b.Fail("k") {
		t.Fatal("failures outside the window counted")
	}
	for i := 0; i < 19 && !b.Fail("k"); i++ {
	}
	if b.Allow("k") {
		t.Fatal("breaker did not open after 20 failures in a minute")
	}
	if !b.Allow("other") {
		t.Fatal("breaker is per handler")
	}
	now = now.Add(5*time.Minute + time.Second)
	if !b.Allow("k") {
		t.Fatal("breaker did not close after 5 minutes")
	}
	b.Fail("k")
	b.Reset()
	if !b.Allow("k") {
		t.Fatal("reset")
	}
}

func TestReservedRequest(t *testing.T) {
	r := Reserved(map[string]string{"admin_path": "secret", "blog_base": "", "category_base": "topics"}, []string{"/shop/x"})
	for _, seg := range []string{"api", "admin", "secret", "blog", "topics", "shop", "uploads", "ext-ui", "plugin-assets", "sitemap.xml"} {
		if !r[seg] {
			t.Errorf("%s not reserved", seg)
		}
	}
	if !requestReserved("/API/v1/x", r) || requestReserved("/hello", r) {
		t.Error("requestReserved")
	}
}

func TestPoolLimit(t *testing.T) {
	e := testEngine()
	var releases []func()
	for i := 0; i < maxConcurrent; i++ {
		rel, ok := e.acquire(10 * time.Millisecond)
		if !ok {
			t.Fatalf("slot %d refused", i)
		}
		releases = append(releases, rel)
	}
	start := time.Now()
	if _, ok := e.acquire(50 * time.Millisecond); ok {
		t.Fatal("9th concurrent invocation admitted")
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("did not wait for a slot")
	}
	go func() { time.Sleep(20 * time.Millisecond); releases[0]() }()
	if rel, ok := e.acquire(time.Second); !ok {
		t.Fatal("waiting caller did not get the freed slot")
	} else {
		rel()
	}
}
