package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// eshop 1.1: products with options (own price and stock), more photos on the product page, discount codes at
// checkout, the parcel's tracking number for the customer, and the price on cards only (hideOnSite 'page').
func TestOfficialShopOptionsCouponsTracking(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "eshop")
	offBPkgSettings(t, admin, "eshop", map[string]any{"promptpayId": "0812345678", "shippingFee": 50, "freeShippingOver": 1000,
		"coupons": "WELCOME | 10%\nBIG | 100 | 600\nOLD | 50 | 0 | 2020-01-01\nbad line"})

	photo := &model.Media{Filename: "side.jpg", URL: "/uploads/side.jpg", Mime: "image/jpeg", Width: 800, Height: 800}
	if err := e.st.CreateMedia(photo); err != nil {
		t.Fatal(err)
	}
	code, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "eshop_product", "title": "Beans", "slug": "beans", "status": "publish",
		"content": "<p>Roasted</p>", "meta": map[string]any{"price": 300, "sku": "B-1", "optionLabel": "Size", "gallery": []any{photo.ID},
			"variants": []any{
				map[string]any{"name": "250 g", "stock": 2},
				map[string]any{"name": "1 kg", "price": 900, "stock": 0},
				map[string]any{"name": "500 g", "price": 550},
			}}})
	if code != 201 {
		t.Fatalf("product: %d %v", code, p)
	}
	beans := int(p["id"].(float64))

	// the product page: more photos, the option select (sold-out one disabled), "from" the lowest price; the
	// Fields block leaves out the hidden fields and the price (cards only)
	page := e.offBSend(offBReq{path: "/product/beans"}).Body.String()
	offBContains(t, "product page", page, `data-shop-gallery`, `/uploads/side.jpg`, `name="v"`, `>Size<`, "250 g", `value="1" data-price="฿900.00" disabled`,
		"฿300.00")
	for _, bad := range []string{`data-field="sku"`, `data-field="price"`, `data-field="variants"`, "B-1"} {
		if strings.Contains(page, bad) {
			t.Fatalf("product page shows %s", bad)
		}
	}
	// a products archive with cards (a list of the type with a one-line Fields block)
	card := map[string]any{"id": "card", "type": "frame", "children": []any{
		map[string]any{"id": "t", "type": "text", "props": map[string]any{"text": "{{post.title}}"}},
		map[string]any{"id": "f", "type": "postFields", "props": map[string]any{"layout": "inline", "max": 3}}}}
	archDesign := map[string]any{"id": "root", "type": "frame", "children": []any{
		map[string]any{"id": "l", "type": "postList", "props": map[string]any{"query": map[string]any{"postType": "current", "limit": 10}}, "children": []any{card}}}}
	code, tpl := admin.do("POST", "/api/v1/posts", map[string]any{"type": "template", "title": "Products", "design": archDesign})
	if code != 201 {
		t.Fatalf("template: %d %v", code, tpl)
	}
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"archive_template_eshop_product": id(tpl)}); code != 200 {
		t.Fatalf("archive template: %d %v", code, b)
	}
	arch := e.offBSend(offBReq{path: "/product"}).Body.String()
	if !strings.Contains(arch, `data-field="price"`) || strings.Contains(arch, `data-field="sku"`) || strings.Contains(arch, `data-field="gallery"`) {
		t.Fatalf("product cards: price on cards only, sku never: %.3000s", arch[strings.Index(arch, "<body"):])
	}

	var cart *http.Cookie
	send := func(method, path, body string) *httpRec {
		t.Helper()
		hdr := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
		if strings.HasPrefix(body, "{") {
			hdr["Content-Type"], hdr["Accept"] = "application/json", "application/json"
		}
		if cart != nil {
			hdr["Cookie"] = cart.Name + "=" + cart.Value
		}
		r := e.offBSend(offBReq{method: method, path: path, ip: "203.0.113.7", body: strings.NewReader(body), hdr: hdr})
		for _, c := range r.Result().Cookies() {
			if strings.HasSuffix(c.Name, "cart") {
				cc := *c
				cart = &cc
			}
		}
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	// an option is needed, a sold-out one is refused, then two options of one product
	if r := send("POST", "/cart/add", fmt.Sprintf(`{"id":%d,"qty":1}`, beans)); r.code != 400 {
		t.Fatalf("no option: %d %s", r.code, r.body)
	}
	if r := send("POST", "/cart/add", fmt.Sprintf(`{"id":%d,"qty":1,"v":"1"}`, beans)); r.code != 400 {
		t.Fatalf("sold-out option: %d %s", r.code, r.body)
	}
	send("POST", "/cart/add", url.Values{"id": {fmt.Sprint(beans)}, "v": {"0"}, "qty": {"2"}}.Encode())
	if r := send("POST", "/cart/add", fmt.Sprintf(`{"id":%d,"qty":1,"v":"2"}`, beans)); r.code != 200 || !strings.Contains(r.body, `"count":3`) {
		t.Fatalf("second option: %d %s", r.code, r.body)
	}
	offBContains(t, "cart", send("GET", "/cart", "").body, "Beans — 250 g", "Beans — 500 g", "฿600.00", "฿550.00", "฿1,150.00", `name="v" value="2"`)

	// checkout: the code box, an unknown / expired / too-small code, then 10% off (free shipping is on the
	// discounted amount: 1,150 − 115 = 1,035 ≥ 1,000)
	offBContains(t, "checkout", send("GET", "/checkout", "").body, `name="coupon"`)
	form := func(coupon string) string {
		return url.Values{"name": {"Mali"}, "phone": {"0812345678"}, "email": {"mali@x.io"}, "address": {"1 Long enough road, Chiang Mai"},
			"payment": {"promptpay"}, "coupon": {coupon}}.Encode()
	}
	for c, want := range map[string]string{"NOPE": "ไม่พบโค้ดนี้|This code does not exist", "old": "หมดอายุ|expired"} {
		r := send("POST", "/checkout", form(c))
		if r.code != 422 || !regexpAny(r.body, want) {
			t.Fatalf("coupon %s: %d", c, r.code)
		}
	}
	r := send("POST", "/checkout", form("welcome"))
	loc := r.hdr.Get("Location")
	if r.code != 303 || !strings.HasPrefix(loc, "/shop/order/") {
		t.Fatalf("checkout: %d %s", r.code, r.body)
	}
	orderCode := strings.TrimSuffix(strings.TrimPrefix(loc, "/shop/order/"), "?new=1")
	offBContains(t, "order page", send("GET", loc, "").body, "WELCOME", "-฿115.00", "฿1,035.00", "Beans — 500 g")

	// stock of the counted option went down (2 → 0); the uncounted one stays uncounted
	_, got := admin.do("GET", fmt.Sprintf("/api/v1/posts/%d", beans), nil)
	vs := fmt.Sprint(got["meta"].(map[string]any)["variants"])
	if !strings.Contains(vs, "name:250 g stock:0") || strings.Contains(vs, "name:500 g stock") {
		t.Fatalf("variant stock: %s", vs)
	}

	// tracking: saved and marked shipped from the Orders page; the customer sees it with a carrier link and is mailed
	_, call := adminPage(t, e.toSample(), admin, "eshop:orders", "")
	call("eshop:setStatus", map[string]any{"id": 1, "status": "paid"})
	html, _ := adminPage(t, e.toSample(), admin, "eshop:orders", "order=1")
	offBContains(t, "order detail", html, `id="shop-tracking"`, `data-ship="1"`, "WELCOME", "Beans — 250 g")
	if c, out := call("eshop:setTracking", map[string]any{"id": 1, "carrier": "Kerry Express", "tracking": "KEX123", "ship": true}); c != 200 ||
		fmt.Sprint(out["data"]) != "map[id:1 status:shipped tracking:KEX123]" {
		t.Fatalf("tracking: %d %v", c, out)
	}
	offBContains(t, "tracked order", send("GET", "/shop/order/"+orderCode, "").body, "KEX123", "kex-express.com/th/track/?track=KEX123")
	e.code.ProcessJobs(context.Background(), 20)
	if m := fmt.Sprint(e.offBMailbox()); !strings.Contains(m, "KEX123") {
		t.Fatalf("shipped mail has no tracking: %s", m)
	}
	// cancelling returns the option's stock
	call("eshop:setStatus", map[string]any{"id": 1, "status": "cancelled"})
	_, got = admin.do("GET", fmt.Sprintf("/api/v1/posts/%d", beans), nil)
	if vs := fmt.Sprint(got["meta"].(map[string]any)["variants"]); !strings.Contains(vs, "name:250 g stock:2") {
		t.Fatalf("variant stock after cancel: %s", vs)
	}
	if d := adminDownload(t, e.toSample(), admin, "eshop:orders", "csv=1"); !strings.Contains(d.Body.String(), "KEX123") || !strings.Contains(d.Body.String(), "WELCOME") {
		t.Fatalf("csv: %s", d.Body.String())
	}
	// the export for other code prices an option
	offASiteCode(t, admin, "")
	if out := offACall(t, admin, "eshop", "orderTotal", []any{map[string]any{"id": beans, "qty": 2, "variant": 2}}); fmt.Sprint(out) != "1100" {
		t.Fatalf("orderTotal with an option: %v", out)
	}
}

func regexpAny(s, alts string) bool {
	for _, a := range strings.Split(alts, "|") {
		if strings.Contains(s, a) {
			return true
		}
	}
	return false
}

// eshop 1.1: a full tax invoice asked for at checkout, the receipt number given when the order is paid, the
// printable receipt (with VAT for a registered seller), and a member's order history at /shop/orders.
func TestOfficialShopReceiptsAndHistory(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	offBMailReady = func() bool { return false }
	defer func() { offBMailReady = nil }()
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "members", "eshop")
	offBPkgSettings(t, admin, "eshop", map[string]any{"bankInfo": "KBank 1", "sellerName": "Beans Co., Ltd.", "sellerTaxId": "0105561234567", "vatRegistered": true})
	_, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "eshop_product", "title": "Beans", "slug": "beans", "status": "publish", "meta": map[string]any{"price": 1070}})

	// a member orders, asking for a tax invoice
	r := e.offBSend(offBReq{method: "POST", path: "/members/register", ip: "203.0.113.70", body: strings.NewReader(`{"email":"buyer@x.io","password":"password1","name":"Buyer"}`),
		hdr: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}})
	cookies := []string{}
	for _, c := range r.Result().Cookies() {
		cookies = append(cookies, c.Name+"="+c.Value)
	}
	send := func(method, path, body string) *httpRec {
		t.Helper()
		rec := e.offBSend(offBReq{method: method, path: path, ip: "203.0.113.71", body: strings.NewReader(body),
			hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Cookie": strings.Join(cookies, "; ")}})
		for _, c := range rec.Result().Cookies() {
			if strings.HasSuffix(c.Name, "cart") {
				cookies = append(cookies, c.Name+"="+c.Value)
			}
		}
		return &httpRec{code: rec.Code, body: rec.Body.String(), hdr: rec.Header()}
	}
	send("POST", "/cart/add", "id="+id(p)+"&qty=1")
	f := url.Values{"name": {"Buyer"}, "phone": {"0812345678"}, "address": {"1 Long enough road, Bangkok"}, "payment": {"bank"}, "wantTax": {"1"},
		"taxName": {"Buyer Ltd."}, "taxId": {"123"}, "taxAddress": {"9 Silom Rd, Bangkok 10500"}}
	if r := send("POST", "/checkout", f.Encode()); r.code != 422 || !strings.Contains(r.body, "13") {
		t.Fatalf("bad tax ID accepted: %d", r.code)
	}
	f.Set("taxId", "0-1055-61234-56-7")
	r2 := send("POST", "/checkout", f.Encode())
	loc := r2.hdr.Get("Location")
	if r2.code != 303 {
		t.Fatalf("checkout: %d %s", r2.code, r2.body)
	}
	code := strings.TrimSuffix(strings.TrimPrefix(loc, "/shop/order/"), "?new=1")
	if q := send("GET", "/shop/order/"+code+"/receipt", ""); q.code != 404 {
		t.Fatalf("receipt before payment: %d", q.code)
	}
	_, call := adminPage(t, e.toSample(), admin, "eshop:orders", "")
	call("eshop:setStatus", map[string]any{"id": 1, "status": "paid"})
	offBContains(t, "order page", send("GET", "/shop/order/"+code, "").body, `id="shop-receipt"`)
	rc := send("GET", "/shop/order/"+code+"/receipt", "")
	offBContains(t, "receipt", rc.body, "R20", "Beans Co., Ltd.", "0105561234567", "Buyer Ltd.", "0105561234567", "฿1,000.00", "฿70.00", "฿1,070.00")
	// the member's order history; a visitor is sent to sign in
	offBContains(t, "my orders", send("GET", "/shop/orders", "").body, "#1", "/shop/order/"+code+"/receipt")
	if v := e.offBSend(offBReq{path: "/shop/orders"}); v.Code != 303 || !strings.HasPrefix(v.Header().Get("Location"), "/members/login") {
		t.Fatalf("visitor history: %d %s", v.Code, v.Header().Get("Location"))
	}
}
