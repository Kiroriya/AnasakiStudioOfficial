package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// booking 1.1: the page's own name, party sizes counted as seats, staff (each takes one booking at a time,
// "anyone" picks a free person), the shop's questions, a deposit with its PromptPay QR and slip, and the
// reminder mail.
func TestOfficialBookingPartyStaffDeposit(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "booking")
	offBPkgSettings(t, admin, "booking", map[string]any{"notifyEmail": "shop@x.io", "minNoticeHours": 0, "pageTitle": "จองโต๊ะ",
		"questions": "แพ้อาหารอะไรไหม *\nโอกาสพิเศษ", "promptpayId": "0812345678", "remindHours": 168})

	_, call := adminPage(t, e.toSample(), admin, "booking:bookings", "tab=svc")
	must := func(name string, in map[string]any) map[string]any {
		t.Helper()
		c, out := call("booking:"+name, in)
		if c != 200 || out["data"] == nil {
			t.Fatalf("%s: %d %v", name, c, out)
		}
		return out["data"].(map[string]any)
	}
	// a table service: 6 seats at once, parties up to 4; a massage with two people; a deposit
	must("saveService", map[string]any{"name": "Table", "minutes": 90, "capacity": 6, "max_party": 4, "active": "on"})
	must("saveService", map[string]any{"name": "Massage", "minutes": 60, "capacity": 1, "active": "on"})
	must("saveService", map[string]any{"name": "Party room", "minutes": 120, "capacity": 1, "deposit": 500, "active": "on"})
	must("saveStaff", map[string]any{"name": "Nok", "active": "on", "services": "2"})
	must("saveStaff", map[string]any{"name": "Joy", "active": "on", "services": "2"})
	html, _ := adminPage(t, e.toSample(), admin, "booking:bookings", "tab=staff")
	offBContains(t, "staff tab", html, `value="Nok"`, `value="Joy"`, `data-bk-form="saveStaff"`)

	get := func(path string) string {
		t.Helper()
		r := e.offBSend(offBReq{path: path})
		if r.Code != 200 {
			t.Fatalf("%s: %d", path, r.Code)
		}
		return r.Body.String()
	}
	html = get("/booking")
	offBContains(t, "own title", html, "<h1>จองโต๊ะ</h1>")
	days := regexp.MustCompile(`date=(\d{4}-\d{2}-\d{2})">`).FindAllStringSubmatch(get("/booking?service=1"), -1)
	day := days[1][1]
	offBContains(t, "party select", get("/booking?service=1&date="+day), `name="party"`, `<option value="4">4</option>`)
	if strings.Contains(get("/booking?service=1&date="+day), `<option value="5">`) {
		t.Fatal("party above max_party")
	}

	n := 0
	book := func(kv ...string) *httpRec {
		t.Helper()
		n++
		f := url.Values{"date": {day}, "name": {"Guest"}, "phone": {"0812345678"}, "email": {"g@x.io"}}
		for i := 0; i+1 < len(kv); i += 2 {
			f.Set(kv[i], kv[i+1])
		}
		r := e.offBSend(offBReq{method: "POST", path: "/booking", ip: fmt.Sprintf("203.0.113.%d", 50+n), body: strings.NewReader(f.Encode()),
			hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}})
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	// questions: the required one must be answered
	if r := book("service", "1", "time", "540", "party", "4"); r.code != 422 {
		t.Fatalf("unanswered question: %d", r.code)
	}
	// seats: 4 + 2 fill the 6 seats at 09:00; a third party no longer fits, a party of 4 sees no 09:00
	if r := book("service", "1", "time", "540", "party", "4", "q0", "ถั่ว"); r.code != 303 {
		t.Fatalf("party of 4: %d %s", r.code, r.body)
	}
	if !strings.Contains(get("/booking?service=1&party=2&date="+day), "&time=540\"") {
		t.Fatal("2 seats left at 09:00 are not offered")
	}
	if strings.Contains(get("/booking?service=1&party=4&date="+day), "&time=540\"") {
		t.Fatal("a party of 4 is offered 09:00 with 2 seats left")
	}
	r := book("service", "1", "time", "540", "party", "2", "q0", "-")
	if r.code != 303 {
		t.Fatalf("party of 2: %d", r.code)
	}
	if r := book("service", "1", "time", "540", "party", "1", "q0", "-"); r.code != 409 {
		t.Fatalf("over the seats: %d", r.code)
	}
	page := get(r.hdr.Get("Location"))
	offBContains(t, "booking page", page, "2 คน", "แพ้อาหารอะไรไหม: -")

	// staff: anyone → Nok, anyone again → Joy, a third at the same time is refused; choosing Nok hides 09:00
	for i, want := range []string{"Nok", "Joy"} {
		r := book("service", "2", "time", "540", "staff", "0", "q0", "-")
		if r.code != 303 || !strings.Contains(get(r.hdr.Get("Location")), "👤 "+want) {
			t.Fatalf("staff booking %d: %d", i, r.code)
		}
	}
	if r := book("service", "2", "time", "540", "staff", "0", "q0", "-"); r.code != 409 {
		t.Fatalf("no one free: %d", r.code)
	}
	if strings.Contains(get("/booking?service=2&staff=1&date="+day), "&time=540\"") {
		t.Fatal("Nok's busy time is offered")
	}

	// a deposit: the booking waits for it, with a QR of the amount and a slip upload
	r = book("service", "3", "time", "600", "q0", "-")
	if r.code != 303 {
		t.Fatalf("deposit booking: %d", r.code)
	}
	loc := r.hdr.Get("Location")
	code := strings.TrimSuffix(strings.TrimPrefix(loc, "/booking/b/"), "?new=1")
	offBContains(t, "deposit page", get(loc), "รอชำระมัดจำ", "/booking/b/"+code+"/qr.svg", `data-bk-slip="`+code+`"`, "฿500.00")
	if q := e.offBSend(offBReq{path: "/booking/b/" + code + "/qr.svg"}); q.Code != 200 || !strings.HasPrefix(q.Body.String(), "<svg") {
		t.Fatalf("qr: %d", q.Code)
	}
	jpeg := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0, 1, 2, 3})
	if s := e.offBSend(offBReq{method: "POST", path: "/booking/b/" + code + "/slip", body: strings.NewReader(`{"image":"` + jpeg + `"}`),
		hdr: map[string]string{"Content-Type": "application/json"}}); s.Code != 200 {
		t.Fatalf("slip: %d %s", s.Code, s.Body.String())
	}
	offBContains(t, "slip sent", get("/booking/b/"+code), `id="bk-slip-done"`)
	html, _ = adminPage(t, e.toSample(), admin, "booking:bookings", "tab=list")
	offBContains(t, "admin list", html, "👥 4", "👤 Nok", `data-ak-download="slip=`, "ถั่ว")
	slipQ := regexp.MustCompile(`data-ak-download="(slip=\d+)"`).FindStringSubmatch(html)
	if d := adminDownload(t, e.toSample(), admin, "booking:bookings", slipQ[1]); d.Code != 200 {
		t.Fatalf("slip download: %d", d.Code)
	}

	// reminders: confirmed bookings with an email within remindHours get one, once
	e.code.ProcessJobs(context.Background(), 50)
	e.mu.Lock()
	e.mails = nil
	e.mu.Unlock()
	// the schedule runs every 15 minutes: two ticks at quarter-hours from now
	tick := time.Now().UTC().Truncate(15 * time.Minute).Add(-30 * time.Minute) // in the past: the queued job is due at once
	runCron := func() {
		t.Helper()
		e.code.CronTick(tick.Add(-15 * time.Minute)) // the schedule's first sight: its next run is set
		if n := e.code.CronTick(tick); n < 1 {
			t.Fatal("the reminder schedule did not run")
		}
		tick = tick.Add(15 * time.Minute)
	}
	runCron()
	e.code.ProcessJobs(context.Background(), 50)
	reminders := 0
	for _, m := range e.offBMailbox() {
		if strings.Contains(m.Subject, "⏰") {
			reminders++
		}
	}
	if reminders != 4 { // the two tables and two massages (the deposit booking is not confirmed yet)
		t.Fatalf("reminders: %d %v %s", reminders, e.offBMailbox(), logsOf(t, admin, "booking"))
	}
	runCron()
	e.code.ProcessJobs(context.Background(), 50)
	again := 0
	for _, m := range e.offBMailbox() {
		if strings.Contains(m.Subject, "⏰") {
			again++
		}
	}
	if again != 4 {
		t.Fatalf("reminders sent twice: %d", again)
	}
}
