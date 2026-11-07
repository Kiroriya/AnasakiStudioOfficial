package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/donations: any amount by PromptPay QR, a receipt asked with a tax ID, the slip, confirming
// numbers the receipt and mails the donor, the printable receipt, the goal bar, the block and the CSV.
func TestOfficialDonations(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "donations")
	offBPkgSettings(t, admin, "donations", map[string]any{"orgName": "มูลนิธิรักษ์น้ำ", "orgTaxId": "0993000123456", "promptpayId": "0812345678",
		"amounts": "100, 500", "goal": 10000, "notifyEmail": "org@x.io"})

	offBContains(t, "donate page", e.offBSend(offBReq{path: "/donate?amount=500"}).Body.String(), `name="amount" value="500" checked`, `role="progressbar"`, "มูลนิธิรักษ์น้ำ")
	give := func(kv ...string) *httpRec {
		t.Helper()
		f := url.Values{}
		for i := 0; i+1 < len(kv); i += 2 {
			f.Set(kv[i], kv[i+1])
		}
		r := e.offBSend(offBReq{method: "POST", path: "/donate", ip: fmt.Sprintf("203.0.113.%d", 80+len(kv)), body: strings.NewReader(f.Encode()),
			hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}})
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	if r := give("amount", "0"); r.code != 422 {
		t.Fatalf("no amount: %d", r.code)
	}
	if r := give("other", "250", "receipt", "1", "name", "Mali", "tax_id", "123"); r.code != 422 {
		t.Fatalf("receipt without a tax ID: %d", r.code)
	}
	r := give("amount", "100", "other", "1250", "name", "Mali", "email", "mali@x.io", "receipt", "1", "tax_id", "1-2345-67890-12-3",
		"address", "99 Sukhumvit Rd, Bangkok 10110", "campaign", "น้ำสะอาด")
	if r.code != 303 {
		t.Fatalf("gift: %d %s", r.code, r.body)
	}
	code := strings.TrimSuffix(strings.TrimPrefix(r.hdr.Get("Location"), "/donate/d/"), "?new=1")
	offBContains(t, "gift page", e.offBSend(offBReq{path: "/donate/d/" + code}).Body.String(), "฿1,250", "/donate/d/"+code+"/qr.svg", `data-dn-slip="`+code+`"`)
	if q := e.offBSend(offBReq{path: "/donate/d/" + code + "/qr.svg"}); q.Code != 200 || !strings.HasPrefix(q.Body.String(), "<svg") {
		t.Fatalf("qr: %d", q.Code)
	}
	if q := e.offBSend(offBReq{path: "/donate/d/" + code + "/receipt"}); q.Code != 404 {
		t.Fatalf("receipt before confirmation: %d", q.Code)
	}
	jpeg := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1})
	if s := e.offBSend(offBReq{method: "POST", path: "/donate/d/" + code + "/slip", body: strings.NewReader(`{"image":"` + jpeg + `"}`),
		hdr: map[string]string{"Content-Type": "application/json"}}); s.Code != 200 {
		t.Fatalf("slip: %d %s", s.Code, s.Body.String())
	}

	html, call := adminPage(t, e.toSample(), admin, "donations:gifts", "")
	offBContains(t, "admin", html, "Mali", "฿1,250", `data-dn="confirmed"`, `data-ak-download="slip=`)
	c, out := call("donations:setStatus", map[string]any{"id": 1, "status": "confirmed"})
	if c != 200 || !strings.HasPrefix(fmt.Sprint(out["data"]), "map[id:1 receiptNo:D") {
		t.Fatalf("confirm: %d %v", c, out)
	}
	rec := e.offBSend(offBReq{path: "/donate/d/" + code + "/receipt"}).Body.String()
	offBContains(t, "receipt", rec, "ใบอนุโมทนาบัตร", "1234567890123", "฿1,250", "0993000123456", "น้ำสะอาด")
	e.code.ProcessJobs(context.Background(), 20)
	if m := fmt.Sprint(e.offBMailbox()); !strings.Contains(m, "mali@x.io") || !strings.Contains(m, "org@x.io") {
		t.Fatalf("mails: %s", m)
	}
	// the block: amounts linking to the form and the goal bar with the confirmed total
	_, blocks := admin.doAny("GET", "/api/v1/blocks", nil)
	if !strings.Contains(fmt.Sprint(blocks), "type:donate") {
		t.Fatalf("block: %v", blocks)
	}
	if code, pv := admin.do("POST", "/api/v1/blocks/render", map[string]any{"plugin": "donations", "block": "donate", "values": map[string]any{}}); code != 200 ||
		!strings.Contains(fmt.Sprint(pv), "/donate?amount=100") || !strings.Contains(fmt.Sprint(pv), "width: 13%") {
		t.Fatalf("block render: %d %v", code, pv)
	}
	if d := adminDownload(t, e.toSample(), admin, "donations:gifts", "csv=1"); !strings.Contains(d.Body.String(), "1234567890123") {
		t.Fatalf("csv: %s", d.Body.String())
	}
}
