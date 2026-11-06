package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/auth"
)

// Site Code phase 3: code connected to the designer, end to end through the
// public site handler.

const shopCode = `
site.collection('items', { label: { th: 'สินค้า', en: 'Items' }, fields: { name: 'text', price: 'number' },
  params: { q: 'text', cat: 'text' }, perPage: { default: 2, max: 10 }, access: ['*'],
  load: ({ page, perPage, params }) => {
    const q = db.table('products').orderBy('id', 'asc')
    if (params.q) q.whereLike('name', '%' + params.q + '%')
    if (params.cat) q.where('category', params.cat)
    return q.paginate(page, perPage)
  } })
site.action('addToCart', { input: { id: 'number' }, access: ['*'], run: ({ input, response }) => {
  response.setCookie('cart', String(input.id))
  return { message: 'เพิ่มแล้ว ' + input.id, refresh: ['items'], update: ['cartBadge'], page: { cartCount: 7 } }
} })
site.action('signup', { input: { email: 'email', name: 'text' }, access: ['*'], run: ({ input }) =>
  input.name === 'taken' ? { errors: { name: 'ชื่อนี้ถูกใช้แล้ว' } } : { message: 'ok', redirect: '/thanks', data: { id: 1 } } })
site.action('evilRedirect', { access: ['*'], run: () => ({ redirect: '//evil.example/x' }) })
site.action('ping', { access: ['*'], run: () => ({ message: 'pong' }) })
site.action('adminOnly', { run: () => ({ message: 'hi admin' }) })
site.page('/shop', { load: ({ request, user }) => ({ title: 'ร้านค้า', cartCount: request.cookies.cart ? 1 : 0, who: user ? user.role : 'guest' }) })
site.formatter('baht', (v) => '฿' + Number(v).toFixed(2))
site.route('GET /hello', { access: ['*'] }, () => 'hello')
`

const productsMigration = `export default migration({
  up(schema) {
    schema.create('products', (t) => {
      t.id()
      t.string('name', 100)
      t.string('category', 50)
      t.decimal('price', 10, 2)
      t.string('status', 20).default('active')
    })
  },
  down(schema) { schema.drop('products') },
})
`

func shopDesign() map[string]any {
	text := func(id, s string, props ...map[string]any) map[string]any {
		p := map[string]any{"text": s}
		for _, x := range props {
			for k, v := range x {
				p[k] = v
			}
		}
		return map[string]any{"id": id, "type": "text", "width": map[string]any{"mode": "hug"}, "height": map[string]any{"mode": "hug"}, "props": p}
	}
	return map[string]any{"id": "root", "type": "frame", "width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"},
		"layout": map[string]any{"mode": "vertical"}, "children": []any{
			text("title", "{{page.title}} / {{post.title}}"),
			map[string]any{"id": "search", "type": "input", "props": map[string]any{"kind": "search", "placeholder": "ค้นหา",
				"filter": map[string]any{"collection": "list", "param": "q"}}},
			map[string]any{"id": "list", "type": "collection", "props": map[string]any{"source": "code:items", "perPage": 2, "key": "shop",
				"params": map[string]any{"cat": "query.cat"}}, "children": []any{
				map[string]any{"id": "card", "type": "frame", "children": []any{
					text("pname", "{{ $index + 1 }}. {{item.name}} {{item.price | baht}} {{ item.price * 2 | number:2 }}"),
					map[string]any{"id": "buy", "type": "button", "props": map[string]any{"text": "ใส่ตะกร้า",
						"action": map[string]any{"type": "code", "name": "addToCart", "input": map[string]any{"id": "item.id"}, "update": []any{"cartBadge"}}}},
				}},
				text("empty", "ไม่พบสินค้า", map[string]any{"showWhen": "$empty"}),
			}},
			map[string]any{"id": "pager", "type": "pagination", "props": map[string]any{"collection": "list", "style": "numbers"}},
			map[string]any{"id": "tbl", "type": "collection", "props": map[string]any{"source": "table:products", "perPage": 3,
				"table": map[string]any{"columns": []any{"name", "status"}, "where": []any{map[string]any{"column": "status", "op": "=", "value": "'active'"}},
					"orderBy": []any{map[string]any{"column": "id", "dir": "desc"}}}},
				"children": []any{text("tname", "T:{{item.name}}")}},
			map[string]any{"id": "tpager", "type": "pagination", "props": map[string]any{"collection": "tbl", "style": "loadmore"}},
			map[string]any{"id": "cartBadge", "name": "cartBadge", "type": "text", "props": map[string]any{"text": "Cart: {{page.cartCount}}"}},
			map[string]any{"id": "adminOnly", "type": "frame", "props": map[string]any{"showWhen": "query.admin == '1'"}, "children": []any{text("secret", "SECRET-BOX")}},
			map[string]any{"id": "form", "type": "frame", "props": map[string]any{"form": true}, "children": []any{
				map[string]any{"id": "em", "type": "input", "props": map[string]any{"kind": "email", "name": "email", "required": true}},
				map[string]any{"id": "nm", "type": "input", "props": map[string]any{"kind": "text", "name": "name"}},
				map[string]any{"id": "send", "type": "button", "props": map[string]any{"text": "Send", "action": map[string]any{"type": "code", "name": "signup"}}},
			}},
		}}
}

// setupShop publishes the code + migration, fills the table and creates the
// /shop page; it returns the page id.
func setupShop(t *testing.T, c, admin *client) int {
	t.Helper()
	admin.put("main.ts", shopCode, "")
	admin.put("migrations/001_products.ts", productsMigration, "")
	if code, b := publishWith(admin, nil, false); code != 200 {
		t.Fatalf("publish: %d %v", code, b)
	}
	rows := []map[string]any{
		{"name": "Apple", "category": "fruit", "price": "10.50", "status": "active"},
		{"name": "Banana", "category": "fruit", "price": "5", "status": "active"},
		{"name": "Carrot", "category": "veg", "price": "3.25", "status": "active"},
		{"name": "Durian <b>", "category": "fruit", "price": "250", "status": "active"},
		{"name": "Eggplant", "category": "veg", "price": "8", "status": "hidden"},
	}
	for _, r := range rows {
		if code, b := admin.do("POST", "/api/v1/code/db/tables/products/rows", r); code != 201 && code != 200 {
			t.Fatalf("insert row: %d %v", code, b)
		}
	}
	code, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Shop", "slug": "shop", "status": "publish", "design": shopDesign()})
	if code != 201 {
		t.Fatalf("create page: %d %v", code, p)
	}
	return int(p["id"].(float64))
}

func page(c *client, path string, hdr map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	return c.send("GET", path, nil, hdr)
}

var csrfRe = regexp.MustCompile(`anasaki_csrf=([0-9a-f]+)`)

// action posts to /_a/name like the client runtime.
func action(c *client, name, csrf string, body any, hdr map[string]string) (int, map[string]any, *httptest.ResponseRecorder) {
	c.t.Helper()
	b, _ := json.Marshal(body)
	h := map[string]string{"Content-Type": "application/json", "Origin": "http://example.com"}
	if csrf != "" {
		h["X-CSRF-Token"] = csrf
		h["Cookie"] = "anasaki_csrf=" + csrf
	}
	for k, v := range hdr {
		if v == "" {
			delete(h, k)
		} else {
			h[k] = v
		}
	}
	rec := c.send("POST", "/_a/"+name, bytes.NewReader(b), h)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec
}

func TestCodeDesignerPublicPage(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	pid := setupShop(t, c, admin)
	if code, b := admin.do("PUT", "/api/v1/code/access", map[string]any{"kind": "table", "name": "products", "roles": []string{"*"}}); code != 200 {
		t.Fatalf("open the table: %d %v", code, b)
	}

	// a page without Site Code features stays as before (no runtime, no cookie)
	if code, _ := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Plain", "slug": "plain", "status": "publish",
		"design": map[string]any{"id": "root", "type": "frame", "children": []any{map[string]any{"id": "t", "type": "text", "props": map[string]any{"text": "{{post.title}}"}}}}}); code != 201 {
		t.Fatal("plain page")
	}
	rec := page(c, "/plain", nil)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "__reaP") || rec.Header().Get("Set-Cookie") != "" || !strings.Contains(rec.Body.String(), `<p class="n-t">Plain</p>`) {
		t.Fatalf("plain page: %d %v\n%s", rec.Code, rec.Header(), rec.Body.String())
	}

	rec = page(c, "/shop?shop_page=2&x=1", nil)
	h := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("shop: %d\n%s", rec.Code, h)
	}
	must := func(h string, subs ...string) {
		t.Helper()
		for _, s := range subs {
			if !strings.Contains(h, s) {
				t.Errorf("expected %q in:\n%s", s, h)
			}
		}
	}
	mustNot := func(h string, subs ...string) {
		t.Helper()
		for _, s := range subs {
			if strings.Contains(h, s) {
				t.Errorf("did not expect %q", s)
			}
		}
	}
	must(h,
		`<p class="n-title">ร้านค้า / Shop</p>`,
		`<div class="n-list" data-rea-collection="list" data-rea-key="shop" data-rea-source="code:items">`,
		`1. Carrot ฿3.25 6.50`, `2. Durian &lt;b&gt; ฿250.00 500.00`,
		`data-rea-action="addToCart" data-rea-input="{&#34;id&#34;:3}" data-rea-update="cartBadge" class="n-buy rea-btn"`,
		`<a class="rea-pg-item rea-pg-prev" href="/shop?x=1" rel="prev">`,
		`<a class="rea-pg-item rea-pg-active" href="/shop?shop_page=2&amp;x=1" aria-current="page">2</a>`,
		`<a class="rea-pg-item rea-pg-next" href="/shop?shop_page=3&amp;x=1" rel="next">`,
		`<link rel="prev" href="/shop?x=1"><link rel="next" href="/shop?shop_page=3&amp;x=1">`,
		// table source: active rows only, newest first, 3 per page, load more
		`<div class="n-tbl" data-rea-collection="tbl" data-rea-key="tbl" data-rea-source="table:products">`, `T:Durian &lt;b&gt;`, `T:Carrot`, `T:Banana`,
		`<a class="rea-pg-item rea-pg-more" href="/shop?shop_page=2&amp;tbl_page=2&amp;x=1" rel="next" data-rea-more>`,
		`<div class="n-search rea-field"><input class="rea-field-el" type="search" name="shop_q" data-rea-filter="list" data-rea-reset="shop_page" aria-label="ค้นหา" placeholder="ค้นหา">`,
		`<form class="n-form" data-rea-form onsubmit="return false">`, `name="email" required`, `data-rea-error-for="email"`,
		`<p class="n-cartBadge">Cart: 0</p>`,
		`window.__reaP=`+fmt.Sprint(pid)+`</script><script>`, "X-CSRF-Token")
	mustNot(h, "1. Apple", "T:Apple", "T:Eggplant", "SECRET-BOX", `class="n-adminOnly"`, "ไม่พบสินค้า")
	m := csrfRe.FindStringSubmatch(rec.Header().Get("Set-Cookie"))
	if m == nil || !strings.Contains(rec.Header().Get("Set-Cookie"), "SameSite=Lax") || strings.Contains(rec.Header().Get("Set-Cookie"), "HttpOnly") {
		t.Fatalf("csrf cookie: %v", rec.Header().Values("Set-Cookie"))
	}
	csrf := m[1]
	// the cookie is not set again once the browser has it
	if rec := page(c, "/shop", map[string]string{"Cookie": "anasaki_csrf=" + csrf}); strings.Contains(rec.Header().Get("Set-Cookie"), "anasaki_csrf") {
		t.Fatal("csrf cookie re-issued")
	}

	// showWhen from the query; filter by URL (?shop_q) and by a mapped param (?cat); empty state
	h = page(c, "/shop?admin=1&shop_q=an", nil).Body.String()
	must(h, "SECRET-BOX", "1. Banana", `value="an"`)
	mustNot(h, "Carrot ฿", "ไม่พบสินค้า")
	h = page(c, "/shop?cat=veg", nil).Body.String()
	must(h, "1. Carrot", "2. Eggplant")
	mustNot(h, "Apple ฿")
	h = page(c, "/shop?shop_q=zzz", nil).Body.String()
	must(h, `<p class="n-empty">ไม่พบสินค้า</p>`, `data-rea-pagination="list" data-rea-style="numbers" aria-label="เลขหน้า" hidden></nav>`)
	// ?page= is not used: two collections are paginated on this page
	h = page(c, "/shop?page=3", nil).Body.String()
	must(h, "1. Apple")

	// partial re-render: the collection and its pagination only
	rec = c.send("GET", fmt.Sprintf("/_c/%d/list?shop_page=3", pid), nil, map[string]string{"X-Rea-Path": "/shop"})
	var part struct {
		Nodes map[string]string `json:"nodes"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &part)
	if rec.Code != 200 || len(part.Nodes) != 2 || !strings.Contains(part.Nodes["list"], "1. Eggplant") ||
		!strings.Contains(part.Nodes["pager"], `aria-current="page">3</a>`) || !strings.HasPrefix(part.Nodes["list"], `<div class="n-list"`) {
		t.Fatalf("partial: %d %s", rec.Code, rec.Body.String())
	}
	rec = c.send("GET", fmt.Sprintf("/_c/%d/tbl?tbl_page=2", pid), nil, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &part)
	if !strings.Contains(part.Nodes["tbl"], "T:Apple") || !strings.Contains(part.Nodes["tpager"], "hidden") {
		t.Fatalf("partial table page 2: %s", rec.Body.String())
	}
	if rec := c.send("GET", "/_c/999999/list", nil, nil); rec.Code != 404 {
		t.Fatalf("partial of a missing page: %d", rec.Code)
	}
	if rec := c.send("POST", fmt.Sprintf("/_c/%d/list", pid), nil, nil); rec.Code != 405 {
		t.Fatalf("partial POST: %d", rec.Code)
	}

	// ---- actions
	body := map[string]any{"input": map[string]any{"id": 3}, "ctx": map[string]any{"postId": pid, "path": "/shop", "query": map[string]any{"shop_page": "2"}},
		"update": []any{"cartBadge"}}
	if code, b, _ := action(c, "addToCart", "", body, nil); code != 403 {
		t.Fatalf("no CSRF: %d %v", code, b)
	}
	if code, _, _ := action(c, "addToCart", csrf, body, map[string]string{"X-CSRF-Token": "wrong"}); code != 403 {
		t.Fatalf("CSRF mismatch: %d", code)
	}
	if code, _, _ := action(c, "addToCart", csrf, body, map[string]string{"Origin": "https://evil.example"}); code != 403 {
		t.Fatalf("cross-origin: %d", code)
	}
	if code, _, _ := action(c, "addToCart", csrf, body, map[string]string{"Origin": "", "Referer": "http://example.com/shop"}); code != 200 {
		t.Fatalf("same-site referer: %d", code)
	}
	if code, _, _ := action(c, "addToCart", csrf, body, map[string]string{"Origin": ""}); code != 403 {
		t.Fatalf("no origin nor referer: %d", code)
	}
	if code, b, _ := action(c, "addToCart", csrf, map[string]any{"input": map[string]any{"id": "abc"}}, nil); code != 400 || b["errors"].(map[string]any)["id"] == nil {
		t.Fatalf("validation: %d %v", code, b)
	}
	code, b, rec := action(c, "addToCart", csrf, body, nil)
	nodes, _ := b["nodes"].(map[string]any)
	if code != 200 || b["message"] != "เพิ่มแล้ว 3" || nodes["cartBadge"] != `<p class="n-cartBadge">Cart: 7</p>` ||
		!strings.Contains(fmt.Sprint(nodes["list"]), "1. Carrot") || nodes["pager"] == nil {
		t.Fatalf("action refresh/update: %d %v", code, b)
	}
	if sc := strings.Join(rec.Header().Values("Set-Cookie"), "\n"); !strings.Contains(sc, "pl_site_cart=3; Path=/; HttpOnly; SameSite=Lax") {
		t.Fatalf("setCookie prefix: %s", sc)
	}
	code, b, _ = action(c, "signup", csrf, map[string]any{"input": map[string]any{"email": "a@b.co", "name": "taken"}}, nil)
	if code != 200 || b["errors"].(map[string]any)["name"] != "ชื่อนี้ถูกใช้แล้ว" {
		t.Fatalf("errors from run: %d %v", code, b)
	}
	code, b, _ = action(c, "signup", csrf, map[string]any{"input": map[string]any{"email": "a@b.co", "name": "Ann"}}, nil)
	if code != 200 || b["redirect"] != "/thanks" || b["data"].(map[string]any)["id"] != 1.0 {
		t.Fatalf("redirect/data: %d %v", code, b)
	}
	if code, b, _ = action(c, "evilRedirect", csrf, map[string]any{}, nil); code != 200 || b["redirect"] != nil {
		t.Fatalf("off-site redirect must be dropped: %d %v", code, b)
	}
	if code, b, _ = action(c, "adminOnly", csrf, map[string]any{}, nil); code != 401 {
		t.Fatalf("admin action anonymous: %d %v", code, b)
	}
	if code, b, _ = action(admin, "adminOnly", csrf, map[string]any{}, nil); code != 200 || b["message"] != "hi admin" {
		t.Fatalf("admin action as admin: %d %v", code, b)
	}
	if code, _, _ = action(c, "nope", csrf, map[string]any{}, nil); code != 404 {
		t.Fatalf("unknown action: %d", code)
	}
	// rate limit: 30 / minute / IP per action
	for i := 1; i <= 31; i++ {
		code, _, _ := action(c, "ping", csrf, map[string]any{}, nil)
		if i <= 30 && code != 200 {
			t.Fatalf("ping %d: %d", i, code)
		}
		if i == 31 && code != 429 {
			t.Fatalf("31st call: %d", code)
		}
	}
	if code, _, _ := action(c, "signup", csrf, map[string]any{"input": map[string]any{"email": "a@b.co", "name": "x"}}, nil); code != 200 {
		t.Fatalf("other actions are not limited by ping: %d", code)
	}
	if rec := c.send("GET", "/_a/ping", nil, nil); rec.Code != 405 {
		t.Fatalf("GET /_a: %d", rec.Code)
	}

	// the page's own cookie: site.page sees it (prefix stripped)
	if h := page(c, "/shop", map[string]string{"Cookie": "pl_site_cart=3; anasaki_csrf=" + csrf}).Body.String(); !strings.Contains(h, "Cart: 1") {
		t.Fatal("request.cookies in site.page")
	}
}

func TestCodeAccessOverrides(t *testing.T) {
	c, _, st := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	setupShop(t, c, admin)
	editor := user("Ed", "editor")
	pub := func(cl *client, path string) (int, string) {
		rec := cl.send("GET", path, nil, nil)
		return rec.Code, rec.Body.String()
	}
	set := func(kind, name string, roles any) map[string]any {
		t.Helper()
		code, b := admin.do("PUT", "/api/v1/code/access", map[string]any{"kind": kind, "name": name, "roles": roles})
		if code != 200 {
			t.Fatalf("PUT access %s %s: %d %v", kind, name, code, b)
		}
		return b
	}

	// the list: everything registered + developer tables
	code, list := admin.doAny("GET", "/api/v1/code/access", nil)
	if code != 200 {
		t.Fatalf("GET access: %d", code)
	}
	rows := map[string]map[string]any{}
	for _, x := range list.([]any) {
		r := x.(map[string]any)
		rows[r["kind"].(string)+" "+r["name"].(string)] = r
	}
	for k, want := range map[string]string{"route GET /hello": "*", "collection items": "*", "action adminOnly": "administrator", "page /shop": "*", "table products": ""} {
		r := rows[k]
		if r == nil {
			t.Fatalf("missing %s in %v", k, rows)
		}
		if got := strs(r["effective"]); got != want || r["override"] != nil {
			t.Errorf("%s effective %q override %v", k, got, r["override"])
		}
	}
	if rows["table products"]["codeDefault"] != nil || strs(rows["collection items"]["codeDefault"]) != "*" {
		t.Errorf("codeDefault: %v / %v", rows["table products"]["codeDefault"], rows["collection items"]["codeDefault"])
	}
	if l, ok := rows["collection items"]["label"].(map[string]any); !ok || l["en"] != "Items" {
		t.Errorf("label: %v", rows["collection items"]["label"])
	}
	if code, _ := editor.do("GET", "/api/v1/code/access", nil); code != 403 {
		t.Fatalf("editor GET access: %d", code)
	}
	if code, _ := admin.do("PUT", "/api/v1/code/access", map[string]any{"kind": "thing", "name": "x", "roles": []string{"*"}}); code != 400 {
		t.Fatalf("bad kind: %d", code)
	}
	if code, _ := admin.do("PUT", "/api/v1/code/access", map[string]any{"kind": "route", "name": "GET /hello", "roles": []string{"Bad Role"}}); code != 400 {
		t.Fatalf("bad role: %d", code)
	}

	// route: override beats the code's ['*']
	if code, _ := pub(c, "/hello"); code != 200 {
		t.Fatalf("route default: %d", code)
	}
	if r := set("route", "GET /hello", []string{"editor"}); strs(r["effective"]) != "editor" || strs(r["override"]) != "editor" {
		t.Fatalf("PUT answer: %v", r)
	}
	if code, _ := pub(c, "/hello"); code != 401 {
		t.Fatalf("route anonymous after override: %d", code)
	}
	if code, _ := pub(editor, "/hello"); code != 200 {
		t.Fatalf("route editor: %d", code)
	}
	if code, _ := pub(admin, "/hello"); code != 200 {
		t.Fatalf("route admin always: %d", code)
	}
	if r := set("route", "GET /hello", nil); r["override"] != nil || strs(r["effective"]) != "*" {
		t.Fatalf("reset: %v", r)
	}
	if code, _ := pub(c, "/hello"); code != 200 {
		t.Fatalf("route after reset: %d", code)
	}

	// collection: [] = nobody but the administrator
	set("collection", "items", []string{})
	if _, h := pub(c, "/shop"); strings.Contains(h, "1. Apple") {
		t.Fatal("collection visible to anonymous after override")
	}
	if _, h := pub(admin, "/shop"); !strings.Contains(h, "1. Apple") {
		t.Fatal("administrator always sees the collection")
	}
	set("collection", "items", []string{"editor"})
	if _, h := pub(editor, "/shop"); !strings.Contains(h, "1. Apple") {
		t.Fatal("editor allowed by override")
	}

	// table: nobody by default, '*' opens it
	if _, h := pub(c, "/shop"); strings.Contains(h, "T:Durian") {
		t.Fatal("table visible by default")
	}
	set("table", "products", []string{"*"})
	if _, h := pub(c, "/shop"); !strings.Contains(h, "T:Durian") {
		t.Fatal("table after '*' override")
	}

	// page data: override to editors only
	set("page", "/shop", []string{"editor"})
	if _, h := pub(c, "/shop"); strings.Contains(h, "ร้านค้า") {
		t.Fatal("page data for anonymous after override")
	}
	if _, h := pub(editor, "/shop"); !strings.Contains(h, "ร้านค้า") {
		t.Fatal("page data for editor")
	}

	// action: administrator default → open to everyone
	rec := c.send("GET", "/shop", nil, nil)
	csrf := csrfRe.FindStringSubmatch(rec.Header().Get("Set-Cookie"))[1]
	if code, _, _ := action(c, "adminOnly", csrf, map[string]any{}, nil); code != 401 {
		t.Fatalf("action default: %d", code)
	}
	if code, _, _ := action(editor, "adminOnly", csrf, map[string]any{}, nil); code != 403 {
		t.Fatalf("action editor: %d", code)
	}
	set("action", "adminOnly", []string{"*"})
	if code, _, _ := action(c, "adminOnly", csrf, map[string]any{}, nil); code != 200 {
		t.Fatalf("action after override: %d", code)
	}
	// audited
	var n int
	if err := st.DB.QueryRow("SELECT COUNT(*) FROM script_audit WHERE action = 'access'").Scan(&n); err != nil || n < 7 {
		t.Fatalf("audit rows: %d %v", n, err)
	}
}

func TestCodeBindingsAndPreview(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	setupShop(t, c, admin)
	designer := roleUser(t, c, admin, "designer", auth.CapRead, auth.CapEditPages)
	dev := roleUser(t, c, admin, "pagedev", auth.CapRead, auth.CapEditPages, auth.CapViewScripts, auth.CapEditScripts)

	code, b := designer.do("GET", "/api/v1/code/bindings", nil)
	if code != 200 {
		t.Fatalf("bindings: %d %v", code, b)
	}
	cols := b["collections"].([]any)
	if len(cols) != 1 || cols[0].(map[string]any)["name"] != "items" || cols[0].(map[string]any)["fields"].(map[string]any)["price"] != "number" ||
		cols[0].(map[string]any)["perPage"].(map[string]any)["default"] != 2.0 {
		t.Fatalf("collections: %v", cols)
	}
	if strs(b["formatters"]) != "baht" || len(b["actions"].([]any)) != 5 || b["pages"].([]any)[0].(map[string]any)["path"] != "/shop" {
		t.Fatalf("bindings: %v", b)
	}
	tables := b["tables"].([]any)
	if len(tables) != 1 || tables[0].(map[string]any)["name"] != "products" || len(tables[0].(map[string]any)["columns"].([]any)) != 5 {
		t.Fatalf("tables: %v", tables)
	}
	// draft: only with edit_scripts
	if code, _ := designer.do("GET", "/api/v1/code/bindings?draft=1", nil); code != 403 {
		t.Fatalf("designer draft bindings: %d", code)
	}
	dev.put("extra.ts", "site.collection('draftOnly', { access: ['*'], load: () => [{ x: 1 }] })\n", "")
	_, mf := admin.do("GET", "/api/v1/code/files/main.ts", nil)
	admin.put("main.ts", "import './extra'\n"+shopCode, mf["updatedAt"].(string))
	code, b = dev.do("GET", "/api/v1/code/bindings?draft=1", nil)
	if code != 200 || len(b["collections"].([]any)) != 2 {
		t.Fatalf("draft bindings: %d %v", code, b)
	}
	if _, b = designer.do("GET", "/api/v1/code/bindings", nil); len(b["collections"].([]any)) != 1 {
		t.Fatal("live bindings changed by the draft")
	}
	if code, _ := c.do("GET", "/api/v1/code/bindings", nil); code != 401 {
		t.Fatalf("anonymous bindings: %d", code)
	}

	// preview
	code, b = designer.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "code:items", "page": 1, "perPage": 3, "params": map[string]any{"cat": "veg"}})
	if code != 200 || b["total"] != 2.0 || len(b["items"].([]any)) != 2 || b["items"].([]any)[0].(map[string]any)["name"] != "Carrot" {
		t.Fatalf("preview collection: %d %v", code, b)
	}
	code, b = designer.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "table:products", "page": 1, "viewAs": "anonymous"})
	if code != 200 || b["denied"] != true || len(b["items"].([]any)) != 0 {
		t.Fatalf("preview table as visitor: %d %v", code, b)
	}
	code, b = designer.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "table:products", "page": 1, "viewAs": "me",
		"table": map[string]any{"where": []any{map[string]any{"column": "category", "op": "=", "value": "veg"}}}})
	if code != 200 || b["denied"] == true || b["total"] != 0.0 {
		// a designer (not admin) is not allowed either: tables default to nobody
		if b["denied"] != true {
			t.Fatalf("preview table as designer: %d %v", code, b)
		}
	}
	code, b = admin.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "table:products", "viewAs": "me",
		"table": map[string]any{"where": []any{map[string]any{"column": "category", "op": "=", "value": "veg"}}}})
	if code != 200 || b["total"] != 2.0 {
		t.Fatalf("preview table as admin: %d %v", code, b)
	}
	code, b = designer.do("POST", "/api/v1/code/preview", map[string]any{"kind": "page", "path": "/shop", "viewAs": "me"})
	if code != 200 || b["data"].(map[string]any)["title"] != "ร้านค้า" || b["data"].(map[string]any)["who"] != "designer" {
		t.Fatalf("preview page: %d %v", code, b)
	}
	if code, _ := designer.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "code:draftOnly", "draft": true}); code != 403 {
		t.Fatalf("designer draft preview: %d", code)
	}
	code, b = dev.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "code:draftOnly", "draft": true})
	if code != 200 || b["total"] != 1.0 {
		t.Fatalf("draft preview: %d %v", code, b)
	}
	code, b = dev.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "code:draftOnly"})
	if code != 200 || b["error"] == nil {
		t.Fatalf("draft-only source on live: %d %v", code, b)
	}
	if code, _ := designer.do("POST", "/api/v1/code/preview", map[string]any{"kind": "bogus"}); code != 400 {
		t.Fatalf("bad kind: %d", code)
	}
	if code, _ := roleUser(t, c, admin, "reader", auth.CapRead).do("POST", "/api/v1/code/preview", map[string]any{"kind": "page", "path": "/"}); code != 403 {
		t.Fatalf("preview without edit_pages: %d", code)
	}
	_ = http.StatusOK
}
