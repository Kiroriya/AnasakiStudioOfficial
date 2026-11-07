package api

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/events: the sign-up form under an event of a no-code content type, the seat limit, the
// attendee's page and cancel, the month calendar, the .ics file, mails and the admin page (check-in, CSV).
func TestOfficialEvents(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	if code, b := admin.do("PUT", "/api/v1/content-types/event", map[string]any{"type": "event", "label": map[string]any{"th": "กิจกรรม", "en": "Events"},
		"slug": "events", "public": true, "supports": []any{"title", "content"}, "fields": []any{
			map[string]any{"key": "when", "type": "datetime", "label": map[string]any{"th": "วันเวลา"}},
			map[string]any{"key": "place", "type": "text", "label": map[string]any{"th": "สถานที่"}},
			map[string]any{"key": "seats", "type": "number", "label": map[string]any{"th": "ที่นั่ง"}}}}); code != 200 {
		t.Fatalf("content type: %d %v", code, b)
	}
	offBInstall(t, admin, "events")
	offBPkgSettings(t, admin, "events", map[string]any{"notifyEmail": "org@x.io"})
	soon := time.Now().AddDate(0, 0, 10)
	when := soon.Format("2006-01-02") + "T09:30"
	code, ev := admin.do("POST", "/api/v1/posts", map[string]any{"type": "event", "title": "Canal clean-up", "slug": "canal", "status": "publish",
		"content": "<p>Bring gloves</p>", "meta": map[string]any{"when": when, "place": "Pier 3, Bangkok", "seats": 3}})
	if code != 201 {
		t.Fatalf("event: %d %v", code, ev)
	}
	evID := id(ev)
	admin.do("POST", "/api/v1/posts", map[string]any{"type": "event", "title": "Last year", "slug": "old", "status": "publish",
		"meta": map[string]any{"when": "2020-01-01T09:00", "seats": 10}})

	pg := e.offBSend(offBReq{path: "/events/canal"}).Body.String()
	offBContains(t, "event page", pg, "Bring gloves", `action="/calendar/rsvp/`+evID+`"`, "3 ", `href="/calendar/`+evID+`.ics"`, "calendar.google.com", "Pier 3")
	if old := e.offBSend(offBReq{path: "/events/old"}).Body.String(); strings.Contains(old, `/calendar/rsvp/`) {
		t.Fatal("a past event takes sign-ups")
	}
	n := 0
	signUp := func(name, seats string) *httpRec {
		n++
		f := url.Values{"name": {name}, "email": {strings.ToLower(name) + "@x.io"}, "seats": {seats}}
		r := e.offBSend(offBReq{method: "POST", path: "/calendar/rsvp/" + evID, ip: fmt.Sprintf("203.0.113.%d", 120+n), body: strings.NewReader(f.Encode()),
			hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}})
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	r := signUp("Mali", "2")
	if r.code != 303 {
		t.Fatalf("sign-up: %d %s", r.code, r.body)
	}
	loc := r.hdr.Get("Location")
	offBContains(t, "registration page", e.offBSend(offBReq{path: loc}).Body.String(), "Canal clean-up", "Pier 3", "/cancel")
	if r := signUp("Somchai", "2"); r.code != 409 {
		t.Fatalf("over the seats: %d", r.code)
	}
	if r := signUp("Nok", "1"); r.code != 303 {
		t.Fatalf("last seat: %d", r.code)
	}
	if pg := e.offBSend(offBReq{path: "/events/canal"}).Body.String(); strings.Contains(pg, `/calendar/rsvp/`) {
		t.Fatal("a full event takes sign-ups")
	}
	// the attendee cancels: the seats come back
	e.offBSend(offBReq{method: "POST", path: loc[:strings.Index(loc, "?")] + "/cancel"})
	if pg := e.offBSend(offBReq{path: "/events/canal"}).Body.String(); !strings.Contains(pg, `/calendar/rsvp/`) {
		t.Fatal("cancelled seats did not come back")
	}

	cal := e.offBSend(offBReq{path: "/calendar?month=" + soon.Format("2006-01")}).Body.String()
	offBContains(t, "calendar", cal, `id="ev-grid"`, `href="/events/canal"`, "09:30 Canal clean-up")
	ics := e.offBSend(offBReq{path: "/calendar/" + evID + ".ics"})
	if ics.Code != 200 || !strings.Contains(ics.Body.String(), "DTSTART:"+soon.Format("20060102")+"T093000") || !strings.Contains(ics.Body.String(), `LOCATION:Pier 3\, Bangkok`) {
		t.Fatalf("ics: %d %s", ics.Code, ics.Body.String())
	}
	e.code.ProcessJobs(context.Background(), 20)
	if m := fmt.Sprint(e.offBMailbox()); !strings.Contains(m, "mali@x.io") || !strings.Contains(m, "org@x.io") {
		t.Fatalf("mails: %s", m)
	}
	html, call := adminPage(t, e.toSample(), admin, "events:registrations", "")
	offBContains(t, "admin events", html, `id="ev-events"`, "Canal clean-up")
	html, _ = adminPage(t, e.toSample(), admin, "events:registrations", "event="+evID)
	offBContains(t, "attendees", html, "Nok", `data-ev="checkIn"`)
	rid := regexp.MustCompile(`data-ev="checkIn" data-id="(\d+)"`).FindStringSubmatch(html)[1]
	if c, out := call("events:checkIn", map[string]any{"id": rid}); c != 200 || out["data"] == nil {
		t.Fatalf("check-in: %d %v", c, out)
	}
	if d := adminDownload(t, e.toSample(), admin, "events:registrations", "csv="+evID); !strings.Contains(d.Body.String(), "nok@x.io") {
		t.Fatalf("csv: %s", d.Body.String())
	}
}
