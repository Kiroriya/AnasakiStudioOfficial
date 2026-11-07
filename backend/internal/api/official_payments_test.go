package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/payments with the Shop and Donations: the card method appears once the plugin has its key,
// a payment page that cannot open leaves the order waiting with a "pay online" button, and Stripe's signed
// webhook marks the order paid / confirms the gift once (bad signatures and other amounts are refused).
func TestOfficialPayments(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "eshop", "donations")
	offBPkgSettings(t, admin, "eshop", map[string]any{"promptpayId": "0812345678"})
	code, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "eshop_product", "title": "Mug", "slug": "mug", "status": "publish", "meta": map[string]any{"price": 250}})
	if code != 201 {
		t.Fatalf("product: %d %v", code, p)
	}
	var cookie string
	send := func(method, path, body string) *httpRec {
		t.Helper()
		hdr := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
		if cookie != "" {
			hdr["Cookie"] = cookie
		}
		r := e.offBSend(offBReq{method: method, path: path, ip: "203.0.113.200", body: strings.NewReader(body), hdr: hdr})
		for _, c := range r.Result().Cookies() {
			if strings.HasSuffix(c.Name, "cart") {
				cookie = c.Name + "=" + c.Value
			}
		}
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	send("POST", "/cart/add", "id="+id(p)+"&qty=2")
	if strings.Contains(send("GET", "/checkout", "").body, `value="card"`) {
		t.Fatal("card offered without the Payments plugin")
	}
	offBInstall(t, admin, "payments")
	offBPkgSettings(t, admin, "payments", map[string]any{"secretKey": "sk_test_abc123", "webhookSecret": "whsec_test"})
	offBContains(t, "checkout", send("GET", "/checkout", "").body, `value="card"`)
	r := send("POST", "/checkout", url.Values{"name": {"Mali"}, "phone": {"0812345678"}, "email": {"m@x.io"}, "address": {"1 Long enough road, Bangkok"}, "payment": {"card"}}.Encode())
	loc := r.hdr.Get("Location")
	// api.stripe.com is not in this site's allowed hosts: the order waits with the button
	if r.code != 303 || !strings.HasSuffix(loc, "?card=failed") {
		t.Fatalf("card checkout: %d %s", r.code, loc)
	}
	orderCode := strings.TrimSuffix(strings.TrimPrefix(loc, "/shop/order/"), "?card=failed")
	offBContains(t, "order page", send("GET", loc, "").body, `id="shop-pay-card"`, `action="/shop/order/`+orderCode+`/pay"`)

	hook := func(ref string, amount int, secret string) int {
		body := fmt.Sprintf(`{"type":"checkout.session.completed","data":{"object":{"id":"cs_test_1","client_reference_id":%q,"amount_total":%d,"payment_status":"paid"}}}`, ref, amount)
		ts := fmt.Sprint(time.Now().Unix())
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte(ts + "." + body))
		rec := e.offBSend(offBReq{method: "POST", path: "/payments/stripe/webhook", body: strings.NewReader(body),
			hdr: map[string]string{"Content-Type": "application/json", "Stripe-Signature": "t=" + ts + ",v1=" + hex.EncodeToString(m.Sum(nil))}})
		e.code.ProcessJobs(context.Background(), 20)
		return rec.Code
	}
	if c := hook("eshop:"+orderCode, 50000, "whsec_wrong"); c != 400 {
		t.Fatalf("bad signature: %d", c)
	}
	if c := hook("eshop:"+orderCode, 1000, "whsec_test"); c != 200 {
		t.Fatalf("webhook: %d", c)
	}
	if b := send("GET", "/shop/order/"+orderCode, "").body; strings.Contains(b, "shop-status-paid") {
		t.Fatal("a payment of another amount paid the order")
	}
	// a fresh order (the reference above is used): the right amount pays it, once
	send("POST", "/cart/add", "id="+id(p)+"&qty=1")
	r = send("POST", "/checkout", url.Values{"name": {"Nok"}, "phone": {"0812345678"}, "address": {"2 Long enough road, Bangkok"}, "payment": {"card"}}.Encode())
	code2 := strings.TrimSuffix(strings.TrimPrefix(r.hdr.Get("Location"), "/shop/order/"), "?card=failed")
	if c := hook("eshop:"+code2, 25000, "whsec_test"); c != 200 {
		t.Fatalf("webhook 2: %d", c)
	}
	hook("eshop:"+code2, 25000, "whsec_test")
	if b := send("GET", "/shop/order/"+code2, "").body; !strings.Contains(b, "shop-status-paid") {
		t.Fatalf("order not paid: %.400s %s", b[strings.Index(b, "shop-page"):], logsOf(t, admin, "eshop"))
	}
	html, _ := adminPage(t, e.toSample(), admin, "payments:list", "")
	if strings.Count(html, "eshop:"+code2) != 1 {
		t.Fatalf("payments list: %s", html)
	}

	// donations: the card button and a confirmed gift with its receipt number
	g := e.offBSend(offBReq{method: "POST", path: "/donate", ip: "203.0.113.201", body: strings.NewReader("amount=500&name=Mali&receipt=1&tax_id=1234567890123&address=99+Sukhumvit+Rd+Bangkok"),
		hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}})
	giftCode := strings.TrimSuffix(strings.TrimPrefix(g.Header().Get("Location"), "/donate/d/"), "?new=1")
	offBContains(t, "gift page", e.offBSend(offBReq{path: "/donate/d/" + giftCode}).Body.String(), `id="dn-pay-card"`)
	hook("donations:"+giftCode, 50000, "whsec_test")
	if rec := e.offBSend(offBReq{path: "/donate/d/" + giftCode + "/receipt"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "D20") {
		t.Fatalf("gift receipt after card payment: %d", rec.Code)
	}
}
