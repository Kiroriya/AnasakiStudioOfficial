package sitecode

import (
	"strings"
	"testing"
)

func TestPhase6BinaryResponses(t *testing.T) {
	p := mustCompile(t, map[string]string{"main.ts": `
site.route('GET /u8', () => new Uint8Array([0, 200, 255]))
site.route('GET /ab', () => new Uint8Array([7]).buffer)
site.route('GET /shape', () => ({ status: 202, headers: { 'Content-Type': 'application/pdf' }, body: new Uint8Array([37, 80]) }))
site.route('GET /b64', () => ({ status: 200, bodyBase64: 'AAEC' }))
site.route('GET /i16', () => ({ status: 200, body: new Int16Array([1]) }))
`})
	e := testEngine()
	cases := map[string]struct {
		status int
		body   string
		ctype  string
	}{
		"/u8":    {200, "\x00\xc8\xff", "application/octet-stream"},
		"/ab":    {200, "\x07", "application/octet-stream"},
		"/shape": {202, "%P", "application/pdf"},
		"/b64":   {200, "\x00\x01\x02", "application/octet-stream"},
		"/i16":   {200, `{"0":1}`, "application/json; charset=utf-8"}, // other typed arrays stay JSON
	}
	for path, want := range cases {
		res, _, err := call(t, e, p, "GET", path, nil)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if res.Status != want.status || res.Body != want.body || res.Headers["Content-Type"] != want.ctype || res.Binary != (want.ctype != "application/json; charset=utf-8") {
			t.Fatalf("%s: %+v", path, res)
		}
	}
}

func TestPhase6PageHookRegistration(t *testing.T) {
	p := mustCompile(t, map[string]string{"main.ts": `
site.middleware(() => undefined, { priority: 3 })
site.middleware(() => undefined)
site.afterResponse(() => {})
site.head(() => '')
site.head(() => '')
site.bodyEnd(() => '')
`})
	if len(p.middlewares) != 2 || p.middlewares[0].Priority != 3 || p.middlewares[1].Priority != 10 || p.afterN != 1 || p.headN != 2 || p.bodyEndN != 1 {
		t.Fatalf("hooks: %+v %d %d %d", p.middlewares, p.afterN, p.headN, p.bodyEndN)
	}
	for name, src := range map[string]string{
		"no fn":        "site.middleware(1)\n",
		"bad priority": "site.middleware(() => 1, { priority: 1.5 })\n",
		"in handler":   "site.route('GET /x', () => { site.head(() => '') })\n",
	} {
		if name == "in handler" {
			continue // registration inside a handler is checked at run time
		}
		if _, probs := compile(t, map[string]string{"main.ts": src}); len(probs) == 0 {
			t.Fatalf("%s: accepted", name)
		}
	}
	// a plugin without the "middleware" scope cannot register middleware / afterResponse
	u := &unitInfo{ID: "abc", Plugin: true, scopes: map[string]bool{}}
	for _, src := range []string{"site.middleware(() => 1)", "site.afterResponse(() => 1)"} {
		res := Build(map[string]string{"main.ts": src})
		if _, probs := RegisterUnit(res.Bundle, nil, &host{cache: NewCache()}, 300_000_000, u); len(probs) == 0 || !strings.Contains(probs[0].Message, "middleware") {
			t.Fatalf("%s without scope: %v", src, probs)
		}
	}
	// head / bodyEnd need no scope (the output is sanitised instead)
	res := Build(map[string]string{"main.ts": "site.head(() => '<meta name=a content=b>')\nsite.formatter('baht', (v) => v)"})
	if p, probs := RegisterUnit(res.Bundle, nil, &host{cache: NewCache()}, 300_000_000, u); len(probs) > 0 || p.headN != 1 || !p.formatters["baht"] {
		t.Fatalf("plugin head / formatter: %v", probs)
	}
}

func TestPhase6SanitizeHead(t *testing.T) {
	in := `<meta name="description" content="a&quot;b"><meta http-equiv="refresh" content="0;url=//evil"><script>alert(1)</script>` +
		`<link rel="canonical" href="https://x.io/a"><link rel="stylesheet" href="/x.css"><link rel="icon" href="javascript:alert(1)">` +
		`<style>body{}</style><script type="application/ld+json">{"@type":"Thing","n":"<b>"}</script><title>t</title><img src=x onerror=alert(1)>text`
	out := SanitizeHead(in)
	want := `<meta name="description" content="a&#34;b"><meta content="0;url=//evil"><link rel="canonical" href="https://x.io/a">` +
		`<script type="application/ld+json">{"@type":"Thing","n":"\u003cb\u003e"}</script>`
	if out != want {
		t.Fatalf("SanitizeHead:\n got %s\nwant %s", out, want)
	}
}

func TestPhase6Aliases(t *testing.T) {
	if m := AliasesFrom(map[string]string{OptFormatterAliases: `{"baht":"thai:baht"}`}); m["baht"] != "thai:baht" {
		t.Fatal(m)
	}
	if m := AliasesFrom(map[string]string{OptFormatterAliases: `nope`}); len(m) != 0 {
		t.Fatal(m)
	}
	if AdminLocale("en-US") != "en" || AdminLocale("") != "th" || AdminLocale("th") != "th" {
		t.Fatal("AdminLocale")
	}
}
