package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/eshop: products (post type shop_product), the cart cookie, checkout with stock, the order
// page with its PromptPay QR, the slip upload, the Orders admin page and the status mails.
func TestOfficialShop(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.2.2" // minCore 2.2.2 (tests run as a dev build)
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, user := setupSite(t, e.c)
	offBInstall(t, admin, "eshop")
	if !codeTable(t, e.pluginEnv, "p_eshop_orders") || !codeTable(t, e.pluginEnv, "p_eshop_items") {
		t.Fatal("migration did not create the tables")
	}
	offBPkgSettings(t, admin, "eshop", map[string]any{"promptpayId": "0812345678", "promptpayName": "Anasaki Shop",
		"bankInfo": "KBank 123-4-56789-0", "allowCod": true, "shippingFee": 50, "freeShippingOver": 1000, "notifyEmail": "shop@x.io"})

	product := func(title, slug string, meta map[string]any, status string) int {
		t.Helper()
		code, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "eshop_product", "title": title, "slug": slug, "status": status,
			"content": "<p>" + title + "</p>", "meta": meta})
		if code != 201 {
			t.Fatalf("product %s: %d %v", slug, code, p)
		}
		return int(p["id"].(float64))
	}
	mug := product("Mug", "mug", map[string]any{"price": 250, "stock": 3, "sku": "M-1"}, "publish")
	tee := product("Tee", "tee", map[string]any{"price": 500, "salePrice": 400}, "publish")
	hidden := product("Hidden", "hidden", map[string]any{"price": 10}, "draft")

	// the product page: price + add-to-cart under the description (setting buyBox)
	rec := e.offBSend(offBReq{path: "/product/tee"})
	offBContains(t, "product page", rec.Body.String(), "฿500.00</del>", "<b>฿400.00</b>", `action="/cart/add"`, `name="id" value="`+fmt.Sprint(tee)+`"`)

	// the designer: blocks and the products collection
	_, blocks := admin.doAny("GET", "/api/v1/blocks", nil)
	for _, want := range []string{"type:add-to-cart", "type:cart-link", "type:price"} {
		if !strings.Contains(fmt.Sprint(blocks), want) {
			t.Fatalf("blocks lack %s: %v", want, blocks)
		}
	}

	var cart *http.Cookie
	send := func(method, path, body, ct string) *httpRec {
		t.Helper()
		hdr := map[string]string{}
		if ct != "" {
			hdr["Content-Type"] = ct
		}
		if ct == "application/json" {
			hdr["Accept"] = "application/json"
		}
		if cart != nil {
			hdr["Cookie"] = cart.Name + "=" + cart.Value
		}
		r := e.offBSend(offBReq{method: method, path: path, ip: "203.0.113.9", body: strings.NewReader(body), hdr: hdr})
		for _, c := range r.Result().Cookies() {
			if strings.HasSuffix(c.Name, "cart") {
				cc := *c
				cart = &cc
			}
		}
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	form := func(kv ...string) string {
		v := url.Values{}
		for i := 0; i+1 < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v.Encode()
	}

	// cart: a classic form post, then JSON (the block script)
	if r := send("POST", "/cart/add", form("id", fmt.Sprint(mug), "qty", "2"), "application/x-www-form-urlencoded"); r.code != 303 || r.hdr.Get("Location") != "/cart" || cart == nil {
		t.Fatalf("add: %d %v %v", r.code, r.hdr, cart)
	}
	if r := send("POST", "/cart/add", fmt.Sprintf(`{"id":%d,"qty":1}`, tee), "application/json"); r.code != 200 || !strings.Contains(r.body, `"count":3`) {
		t.Fatalf("json add: %d %s", r.code, r.body)
	}
	if r := send("POST", "/cart/add", fmt.Sprintf(`{"id":%d,"qty":1}`, hidden), "application/json"); r.code != 400 {
		t.Fatalf("a draft product reached the cart: %d %s", r.code, r.body)
	}
	r := send("GET", "/cart", "", "")
	offBContains(t, "cart", r.body, "Mug", "Tee", "฿500.00", "฿400.00", "฿900.00", "฿50.00", "฿950.00", `href="/checkout"`)

	// checkout: field errors keep the input, then an order
	r = send("POST", "/checkout", form("name", "Somchai", "phone", "abc", "address", "short", "payment", "promptpay"), "application/x-www-form-urlencoded")
	if r.code != 422 || !strings.Contains(r.body, `value="Somchai"`) || !strings.Contains(r.body, "is-invalid") {
		t.Fatalf("invalid checkout: %d", r.code)
	}
	r = send("POST", "/checkout", form("name", "Somchai", "phone", "081-234-5678", "email", "buyer@x.io",
		"address", "99/1 Sukhumvit Rd, Bangkok 10110", "payment", "promptpay"), "application/x-www-form-urlencoded")
	loc := r.hdr.Get("Location")
	if r.code != 303 || !strings.HasPrefix(loc, "/shop/order/") {
		t.Fatalf("checkout: %d %s %s", r.code, loc, logsOf(t, admin, "eshop"))
	}
	if cart == nil || cart.MaxAge >= 0 && cart.Value != "" {
		t.Fatalf("cart not cleared: %+v", cart)
	}
	code := strings.TrimSuffix(strings.TrimPrefix(loc, "/shop/order/"), "?new=1")
	r = send("GET", loc, "", "")
	offBContains(t, "order page", r.body, `/shop/order/`+code+`/qr.svg`, "฿950.00", "Anasaki Shop", `data-shop-slip="`+code+`"`)
	if q := send("GET", "/shop/order/"+code+"/qr.svg", "", ""); q.code != 200 || q.hdr.Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(q.body, "<svg") {
		t.Fatalf("qr: %d %s", q.code, q.hdr.Get("Content-Type"))
	}
	if q := send("GET", "/shop/order/0123456789abcdef01234567", "", ""); q.code != 404 {
		t.Fatalf("unknown order: %d", q.code)
	}

	// stock went down (3 → 1); two more mugs no longer fit
	_, p := admin.do("GET", fmt.Sprintf("/api/v1/posts/%d", mug), nil)
	if m, _ := p["meta"].(map[string]any); fmt.Sprint(m["stock"]) != "1" || fmt.Sprint(m["sku"]) != "M-1" {
		t.Fatalf("stock: %v", p["meta"])
	}
	send("POST", "/cart/add", form("id", fmt.Sprint(mug), "qty", "2"), "application/x-www-form-urlencoded")
	if r := send("POST", "/checkout", form("name", "B", "phone", "0812345678", "address", "1 Long enough street", "payment", "bank"), "application/x-www-form-urlencoded"); r.code != 409 {
		t.Fatalf("over-stock checkout: %d", r.code)
	}

	// the slip: a JPEG data URL, once
	jpeg := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3})
	if s := send("POST", "/shop/order/"+code+"/slip", `{"image":"data:image/png;base64,AAAA"}`, "application/json"); s.code != 400 {
		t.Fatalf("bad slip: %d", s.code)
	}
	if s := send("POST", "/shop/order/"+code+"/slip", `{"image":"`+jpeg+`"}`, "application/json"); s.code != 200 {
		t.Fatalf("slip: %d %s", s.code, s.body)
	}
	if s := send("POST", "/shop/order/"+code+"/slip", `{"image":"`+jpeg+`"}`, "application/json"); s.code != 409 {
		t.Fatalf("second slip: %d", s.code)
	}
	if !strings.Contains(send("GET", "/shop/order/"+code, "", "").body, `id="shop-slip-done"`) {
		t.Fatal("order page does not show the slip")
	}

	// mails: the customer and the shop
	e.code.ProcessJobs(context.Background(), 20)
	mails := fmt.Sprint(e.offBMailbox())
	if !strings.Contains(mails, "buyer@x.io") || !strings.Contains(mails, "shop@x.io") {
		t.Fatalf("mails: %s", mails)
	}

	// admin page: list, detail with the slip, status → paid mails the customer; authors may not open it
	html, call := adminPage(t, e.toSample(), admin, "eshop:orders", "")
	offBContains(t, "orders", html, `id="shop-orders"`, "Somchai", "฿950.00")
	html, _ = adminPage(t, e.toSample(), admin, "eshop:orders", "order=1")
	offBContains(t, "order detail", html, "data:image/jpeg;base64,", `data-shop-status="paid"`)
	if code, _ := user("Au", "author").do("POST", "/api/v1/code/admin-pages/shop:orders/ticket", map[string]any{}); code == 200 {
		t.Fatal("author may open orders")
	}
	if c, out := call("eshop:setStatus", map[string]any{"id": 1, "status": "paid"}); c != 200 || fmt.Sprint(out["data"]) != "map[id:1 status:paid]" {
		t.Fatalf("set paid: %d %v", c, out)
	}
	e.code.ProcessJobs(context.Background(), 20)
	if !strings.Contains(fmt.Sprint(e.offBMailbox()), "Payment confirmed") && !strings.Contains(fmt.Sprint(e.offBMailbox()), "ยืนยันการชำระเงิน") {
		t.Fatalf("no paid mail: %v", e.offBMailbox())
	}
	// cancelling gives the stock back
	call("eshop:setStatus", map[string]any{"id": 1, "status": "cancelled"})
	_, p = admin.do("GET", fmt.Sprintf("/api/v1/posts/%d", mug), nil)
	if m, _ := p["meta"].(map[string]any); fmt.Sprint(m["stock"]) != "3" {
		t.Fatalf("stock after cancel: %v", p["meta"])
	}
	if d := adminDownload(t, e.toSample(), admin, "eshop:orders", "csv=1"); d.Code != 200 || !strings.Contains(d.Body.String(), "Somchai") {
		t.Fatalf("csv: %d %s", d.Code, d.Body.String())
	}
}

type httpRec struct {
	code int
	body string
	hdr  http.Header
}
