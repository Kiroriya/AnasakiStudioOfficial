package sitecode

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/design"
)

func phase3Engine() *Engine {
	e := testEngine()
	e.collCache = NewCache()
	e.limiter = newRateLimiter(30, time.Minute)
	return e
}

func TestRegisterBindings(t *testing.T) {
	p := mustCompile(t, map[string]string{"main.ts": `
site.collection('events', { label: { th: 'อีเวนต์', en: 'Events' }, fields: { title: 'text', date: 'date', body: 'html' },
  params: { category: 'text', n: 'number' }, perPage: { default: 9, max: 30 }, cache: 60, access: ['*'],
  load: () => [] })
site.collection('plain', { load: () => [] })
site.action('register', { input: { eventId: 'number', note: 'text?', email: 'email' }, access: ['subscriber'], run: () => {} })
site.page('/events/{slug}', { load: () => ({}) })
site.page('/checkout', { access: ['editor'], load: () => ({}) })
site.formatter('baht', (v) => '฿' + v)
`})
	m := p.Manifest
	if len(m.Collections) != 2 || m.Collections[0].Name != "events" || m.Collections[0].PerPage.Default != 9 || m.Collections[0].PerPage.Max != 30 ||
		m.Collections[0].Cache != 60 || m.Collections[0].Access[0] != "*" || m.Collections[0].Fields["body"] != "html" || m.Collections[0].Params["n"] != "number" {
		t.Fatalf("collections: %+v", m.Collections)
	}
	if l, ok := m.Collections[0].Label.(map[string]string); !ok || l["en"] != "Events" {
		t.Fatalf("label: %#v", m.Collections[0].Label)
	}
	if c := m.Collections[1]; c.Label != "plain" || c.PerPage.Default != 10 || c.PerPage.Max != 50 || c.Access[0] != "administrator" {
		t.Fatalf("collection defaults: %+v", c)
	}
	if len(m.Actions) != 1 || m.Actions[0].Input["note"] != "text?" || m.Actions[0].Access[0] != "subscriber" {
		t.Fatalf("actions: %+v", m.Actions)
	}
	if len(m.Pages) != 2 || m.Pages[0].Path != "/events/{slug}" || m.Pages[0].Access[0] != "*" || m.Pages[1].Access[0] != "editor" {
		t.Fatalf("pages: %+v", m.Pages)
	}
	if strings.Join(m.Formatters, ",") != "baht" {
		t.Fatalf("formatters: %v", m.Formatters)
	}
	if i, ps := p.matchPage("/events/songkran"); i != 0 || ps["slug"] != "songkran" {
		t.Fatalf("matchPage: %d %v", i, ps)
	}
	if i, _ := p.matchPage("/nope"); i != -1 {
		t.Fatal("matchPage matched /nope")
	}

	bad := map[string]string{
		"bad name":         "site.collection('Events', { load: () => [] })\n",
		"dup collection":   "site.collection('a', { load: () => [] })\nsite.collection('a', { load: () => [] })\n",
		"dup action":       "site.action('a', { run: () => 1 })\nsite.action('a', { run: () => 1 })\n",
		"dup formatter":    "site.formatter('a', (v) => v)\nsite.formatter('a', (v) => v)\n",
		"dup page":         "site.page('/a/{x}', { load: () => ({}) })\nsite.page('/a/{y}', { load: () => ({}) })\n",
		"no load":          "site.collection('a', {})\n",
		"bad field":        "site.collection('a', { fields: { x: 'money' }, load: () => [] })\n",
		"bad param":        "site.collection('a', { params: { x: 'date' }, load: () => [] })\n",
		"perPage max":      "site.collection('a', { perPage: { max: 500 }, load: () => [] })\n",
		"perPage default":  "site.collection('a', { perPage: { default: 60 }, load: () => [] })\n",
		"bad input":        "site.action('a', { input: { x: 'date' }, run: () => 1 })\n",
		"bad page path":    "site.page('checkout', { load: () => ({}) })\n",
		"bad access":       "site.action('a', { access: ['Bad Role'], run: () => 1 })\n",
		"formatter no fn":  "site.formatter('a')\n",
		"in handler":       "site.route('GET /x', { access: ['*'] }, () => { site.collection('a', { load: () => [] }) })\n",
		"cache too long":   "site.collection('a', { cache: 999999, load: () => [] })\n",
		"name with spaces": "site.action('a b', { run: () => 1 })\n",
	}
	for name, src := range bad {
		if name == "in handler" {
			continue // only fails when the route runs
		}
		if _, probs := compile(t, map[string]string{"main.ts": src}); len(probs) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
	_, probs := compile(t, map[string]string{"main.ts": "site.collection('a', { load: () => [] })\n\nsite.collection('a', { load: () => [] })\n"})
	if len(probs) != 1 || probs[0].Line != 3 || !strings.Contains(probs[0].Message, "ซ้ำ") {
		t.Fatalf("duplicate location: %+v", probs)
	}
}

func TestValidateInputAndParams(t *testing.T) {
	decl := map[string]string{"id": "number", "note": "text?", "email": "email", "agree": "bool", "meta": "json?", "name": "text"}
	out, errs := validateInput(decl, map[string]any{"id": "12", "email": "a@b.co", "agree": "on", "name": "Ann", "extra": "dropped"})
	if len(errs) != 0 || out["id"] != 12.0 || out["note"] != nil || out["agree"] != true || out["name"] != "Ann" || out["meta"] != nil {
		t.Fatalf("valid input: %v %v", out, errs)
	}
	if _, ok := out["extra"]; ok {
		t.Fatal("unknown key kept")
	}
	_, errs = validateInput(decl, map[string]any{"id": "x", "email": "nope", "name": ""})
	if errs["id"] == "" || errs["email"] == "" || errs["name"] == "" || len(errs) != 3 {
		t.Fatalf("errors: %v", errs)
	}
	_, errs = validateInput(map[string]string{"t": "text"}, map[string]any{"t": strings.Repeat("x", maxActionInputLen+1)})
	if errs["t"] == "" {
		t.Fatal("over-long text accepted")
	}
	p := coerceParams(map[string]string{"q": "text", "n": "number", "b": "bool"}, map[string]any{"q": 5.0, "n": "7", "b": "1", "x": "drop"})
	if p["q"] != "5" || p["n"] != 7.0 || p["b"] != true || len(p) != 3 {
		t.Fatalf("params: %v", p)
	}
	if p := coerceParams(map[string]string{"n": "number"}, map[string]any{"n": "abc"}); len(p) != 0 {
		t.Fatalf("bad number kept: %v", p)
	}
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(3, time.Minute)
	now := time.Unix(1000, 0)
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if !l.Allow("ip|a") {
			t.Fatal("blocked too early")
		}
	}
	if l.Allow("ip|a") || !l.Allow("ip|b") || !l.Allow("ip2|a") {
		t.Fatal("limit per key")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("ip|a") {
		t.Fatal("window did not reset")
	}
}

func TestSessionCollectionsFormattersActions(t *testing.T) {
	e := phase3Engine()
	p := mustCompile(t, map[string]string{"main.ts": `
let calls = 0
site.collection('items', { fields: { name: 'text', body: 'html' }, params: { q: 'text' }, perPage: { default: 2, max: 3 }, cache: 60, access: ['*'],
  load: ({ page, perPage, params, user, request }) => {
    calls++
    const n = (cache.get('n') || 0) + 1; cache.set('n', n)
    const all = ['a', 'b', 'c', 'd', 'e'].filter((x) => !params.q || x === params.q)
    return { items: all.slice((page - 1) * perPage, page * perPage).map((x) => ({ name: x, n, role: user ? user.role : 'anon', body: '<b>ok</b><script>x</script>', ck: request.cookies.cart || '' })), total: all.length }
  } })
site.collection('list', { access: ['*'], load: () => [1, 2, 3, 4, 5].map((i) => ({ i })) })
site.collection('boom', { access: ['*'], load: () => { throw new Error('kaput') } })
site.collection('admins', { load: () => [{ x: 1 }] })
site.formatter('slow', (v) => { while (true) {} })
site.formatter('star', (v, a) => '*' + v + (a || '') + '*')
site.page('/shop/{cat}', { load: ({ params, response }) => { response.setCookie('seen', params.cat, { maxAge: 60 }); return { cat: params.cat } } })
site.action('add', { input: { id: 'number', note: 'text?' }, access: ['*'], run: ({ input, response }) => {
  response.setCookie('cart', String(input.id), { httpOnly: false })
  return { message: 'added ' + input.id, refresh: ['items'], update: ['cartBox'], page: { count: 1 }, redirect: 'https://evil.example/x', data: { ok: true }, errors: input.note === 'bad' ? { note: 'no' } : undefined }
} })
site.action('fails', { access: ['*'], run: () => { throw new Error('secret stack') } })
site.action('mine', { run: () => ({}) })
`})
	req := httptest.NewRequest("GET", "https://example.com/shop/x", nil)
	req.AddCookie(&http.Cookie{Name: "pl_site_cart", Value: "7"})
	req.AddCookie(&http.Cookie{Name: "other", Value: "z"})
	sess := &Session{e: e, prog: p, r: req}
	defer sess.Close()

	res := sess.LoadCollection(design.CollectionReq{Source: "code:items", Page: 2, PerPage: 50, Params: map[string]any{"zzz": "drop"}})
	if res.Error != "" || res.Total != 5 || res.PerPage != 3 || len(res.Items) != 2 || res.Items[0]["name"] != "d" || res.Items[0]["role"] != "anon" {
		t.Fatalf("page 2 (perPage capped at max 3): %+v", res)
	}
	if b, _ := res.Items[0]["body"].(string); !strings.HasPrefix(b, "<b>ok</b>") || strings.Contains(b, "<script") || res.Items[0]["ck"] != "7" {
		t.Fatalf("html field sanitised / request.cookies (prefix stripped): %+v", res.Items[0])
	}
	// cached for the same role/params; a different role gets its own entry
	n1 := res.Items[0]["n"]
	if again := sess.LoadCollection(design.CollectionReq{Source: "code:items", Page: 2, PerPage: 50}); again.Items[0]["n"] != n1 {
		t.Fatalf("cache: %v vs %v", again.Items[0]["n"], n1)
	}
	ed := &Session{e: e, prog: p, r: req, user: &User{ID: 2, Role: "editor"}}
	defer ed.Close()
	r2 := ed.LoadCollection(design.CollectionReq{Source: "code:items", Page: 2})
	if r2.Items[0]["role"] != "editor" || r2.Items[0]["n"] == n1 || r2.PerPage != 2 {
		t.Fatalf("cache keyed by role: %+v", r2)
	}
	if r3 := ed.LoadCollection(design.CollectionReq{Source: "code:items", Page: 2}); r3.Items[0]["n"] != r2.Items[0]["n"] {
		t.Fatal("editor entry not cached")
	}
	// a plain list longer than a page is the whole collection
	if l := sess.LoadCollection(design.CollectionReq{Source: "code:list", Page: 3, PerPage: 2}); l.Total != 5 || len(l.Items) != 1 || l.Items[0]["i"] != 5.0 {
		t.Fatalf("plain list: %+v", l)
	}
	if b := sess.LoadCollection(design.CollectionReq{Source: "code:boom", Page: 1}); b.Error == "" || len(b.Items) != 0 {
		t.Fatalf("failing load: %+v", b)
	}
	if d := sess.LoadCollection(design.CollectionReq{Source: "code:admins", Page: 1}); !d.Denied || len(d.Items) != 0 {
		t.Fatalf("default access is administrator only: %+v", d)
	}
	admin := &Session{e: e, prog: p, r: req, user: &User{ID: 1, Role: "administrator"}}
	defer admin.Close()
	if d := admin.LoadCollection(design.CollectionReq{Source: "code:admins", Page: 1}); d.Denied || len(d.Items) != 1 {
		t.Fatalf("administrator: %+v", d)
	}
	if u := sess.LoadCollection(design.CollectionReq{Source: "code:nope"}); u.Error == "" {
		t.Fatal("unknown collection")
	}

	// formatters: 50 ms limit → raw value; unknown → not handled
	start := time.Now()
	if s, ok := sess.Format("slow", "raw", nil); !ok || s != "raw" {
		t.Fatalf("slow formatter: %q %v", s, ok)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("formatter timeout took %v", time.Since(start))
	}
	if s, ok := sess.Format("star", 5.0, []any{"!"}); !ok || s != "*5!*" {
		t.Fatalf("formatter after a timeout: %q %v", s, ok)
	}
	if _, ok := sess.Format("nope", 1, nil); ok {
		t.Fatal("unknown formatter handled")
	}

	// page data + cookies (prefixed, Secure on https)
	if d := sess.PageData("/shop/shoes"); d["cat"] != "shoes" {
		t.Fatalf("page data: %v %s", d, sess.LastError)
	}
	if d := sess.PageData("/other"); d != nil {
		t.Fatalf("no page: %v", d)
	}

	// actions
	out := sess.RunAction("add", map[string]any{"id": "3"})
	if out.Status != 200 || out.Message != "added 3" || out.Redirect != "" || out.Refresh[0] != "items" || out.Update[0] != "cartBox" ||
		out.Page["count"] != 1.0 || out.Data.(map[string]any)["ok"] != true || out.Errors != nil {
		t.Fatalf("action: %+v", out)
	}
	cookies := strings.Join(sess.Cookies(), "\n")
	if !strings.Contains(cookies, "pl_site_seen=shoes; Path=/; Max-Age=60; HttpOnly; Secure; SameSite=Lax") ||
		!strings.Contains(cookies, "pl_site_cart=3; Path=/; Secure; SameSite=Lax") || strings.Contains(cookies, "pl_site_cart=3; Path=/; HttpOnly") {
		t.Fatalf("cookies: %s", cookies)
	}
	if out := sess.RunAction("add", map[string]any{"id": "x"}); out.Status != 400 || out.Errors["id"] == "" {
		t.Fatalf("validation: %+v", out)
	}
	if out := sess.RunAction("add", map[string]any{"id": 1.0, "note": "bad"}); out.Status != 200 || out.Errors["note"] != "no" {
		t.Fatalf("errors from run: %+v", out)
	}
	if out := sess.RunAction("fails", nil); out.Status != 500 || out.Error != "ทำรายการไม่สำเร็จ" || strings.Contains(out.Error, "secret") {
		t.Fatalf("failing action: %+v", out)
	}
	if out := sess.RunAction("mine", nil); out.Status != 401 {
		t.Fatalf("anonymous on an admin action: %+v", out)
	}
	if out := ed.RunAction("mine", nil); out.Status != 403 {
		t.Fatalf("editor on an admin action: %+v", out)
	}
	if out := sess.RunAction("ghost", nil); out.Status != 404 {
		t.Fatalf("unknown action: %+v", out)
	}
}
