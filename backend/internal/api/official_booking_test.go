package api

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/booking: a service made on the admin page, the no-JavaScript booking flow (service → day →
// time → details), no double booking, the customer's page with cancel, closed days, and the admin actions.
func TestOfficialBooking(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.2.2"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "booking")
	offBPkgSettings(t, admin, "booking", map[string]any{"notifyEmail": "shop@x.io", "minNoticeHours": 0})

	_, call := adminPage(t, e.toSample(), admin, "booking:bookings", "tab=svc")
	if c, out := call("booking:saveService", map[string]any{"name": "Haircut", "minutes": 60, "price": 300, "capacity": 1, "active": "on"}); c != 200 || out["data"] == nil {
		t.Fatalf("service: %d %v", c, out)
	}
	get := func(path string) string {
		t.Helper()
		r := e.offBSend(offBReq{path: path})
		if r.Code != 200 {
			t.Fatalf("%s: %d", path, r.Code)
		}
		return r.Body.String()
	}
	// one service: the page goes straight to its days; Sunday is closed by default
	html := get("/booking")
	days := regexp.MustCompile(`<a role="listitem" class="bk-day" href="/booking\?service=1&date=(\d{4}-\d{2}-\d{2})">`).FindAllStringSubmatch(html, -1)
	if len(days) < 2 {
		i := strings.Index(html, `class="bk`)
		t.Fatalf("no open days: %s %s", html[max(0, i):min(len(html), i+1500)], logsOf(t, admin, "booking"))
	}
	day := days[1][1] // not today: every slot of it may be past
	html = get("/booking?service=1&date=" + day)
	times := regexp.MustCompile(`date=`+day+`&time=(\d+)">(\d\d:\d\d)</a>`).FindAllStringSubmatch(html, -1)
	if len(times) != 17 || times[0][2] != "09:00" || times[16][2] != "17:00" || times[1][2] != "09:30" {
		t.Fatalf("slots of %s: %v", day, times)
	}
	tm := times[0][1]
	html = get(fmt.Sprintf("/booking?service=1&date=%s&time=%s", day, tm))
	offBContains(t, "details", html, `action="/booking"`, "09:00–10:00", "Haircut")

	book := func(name string) *httpRec {
		form := url.Values{"service": {"1"}, "date": {day}, "time": {tm}, "name": {name}, "phone": {"0812345678"}, "email": {"c@x.io"}}
		r := e.offBSend(offBReq{method: "POST", path: "/booking", ip: "203.0.113.40", body: strings.NewReader(form.Encode()),
			hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}})
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	r := book("Mali")
	loc := r.hdr.Get("Location")
	if r.code != 303 || !strings.HasPrefix(loc, "/booking/b/") {
		t.Fatalf("book: %d %s", r.code, r.body)
	}
	if r := book("Second"); r.code != 409 {
		t.Fatalf("double booking: %d", r.code)
	}
	if strings.Contains(get("/booking?service=1&date="+day), "time="+tm+`">09:00`) {
		t.Fatal("a booked slot is still offered")
	}
	offBContains(t, "customer page", get(loc), "Mali", "09:00–10:00", "/cancel")
	e.code.ProcessJobs(context.Background(), 20)
	if m := fmt.Sprint(e.offBMailbox()); !strings.Contains(m, "c@x.io") || !strings.Contains(m, "shop@x.io") {
		t.Fatalf("mails: %s", m)
	}

	// the admin list and a status change
	html, call = adminPage(t, e.toSample(), admin, "booking:bookings", "")
	offBContains(t, "bookings", html, "Mali", `data-status="done"`)
	// cancelled by the customer: the slot is free again
	code := strings.TrimSuffix(strings.TrimPrefix(loc, "/booking/b/"), "?new=1")
	if c := e.offBSend(offBReq{method: "POST", path: "/booking/b/" + code + "/cancel"}); c.Code != 200 {
		t.Fatalf("cancel: %d", c.Code)
	}
	if !strings.Contains(get("/booking?service=1&date="+day), "time="+tm+`">09:00`) {
		t.Fatal("a cancelled slot is not offered again")
	}
	// a closed day is not bookable
	if c, out := call("booking:addClosedDay", map[string]any{"day": day, "note": "holiday"}); c != 200 {
		t.Fatalf("closed day: %d %v", c, out)
	}
	if strings.Contains(get("/booking?service=1&date="+day), "&time=") {
		t.Fatal("slots on a closed day")
	}
	if c, out := call("booking:saveHours", map[string]any{"values": map[string]any{"open1": "18:00", "close1": "09:00"}}); c != 200 || out["errors"] == nil {
		t.Fatalf("bad hours accepted: %d %v", c, out)
	}
}
