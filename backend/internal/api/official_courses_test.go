package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// plugins-official/courses with Members: a paid course whose lessons are locked (a preview is open), the
// enrolment waiting for payment, access granted on the Students page, lessons marked done, the progress on the
// course page, "next lesson", the certificate at 100 %, and a members-only course for any signed-in member.
func TestOfficialCourses(t *testing.T) {
	old := plugin.CoreVersion
	plugin.CoreVersion = "2.5.0"
	t.Cleanup(func() { plugin.CoreVersion = old })
	offBMailReady = func() bool { return false } // members sign in at once without mail
	defer func() { offBMailReady = nil }()
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "members", "courses")
	offBPkgSettings(t, admin, "courses", map[string]any{"paymentNote": "KBank 123-4-56789-0", "issuer": "Anasaki Academy"})

	post := func(typ, title, slug string, meta map[string]any) string {
		t.Helper()
		code, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": typ, "title": title, "slug": slug, "status": "publish",
			"content": "<p>SECRET " + title + "</p>", "excerpt": title, "meta": meta})
		if code != 201 {
			t.Fatalf("%s: %d %v", slug, code, p)
		}
		return id(p)
	}
	course := post("courses_course", "Thai cooking", "thai-cooking", map[string]any{"access": "paid", "price": 990})
	l1 := post("courses_lesson", "Curry paste", "curry-paste", map[string]any{"course": numOf(course), "order": 1, "preview": true, "video": "https://youtu.be/dQw4w9WgXcQ"})
	l2 := post("courses_lesson", "Green curry", "green-curry", map[string]any{"course": numOf(course), "order": 2})
	club := post("courses_course", "Members club", "club", map[string]any{"access": "members"})
	post("courses_lesson", "Welcome", "welcome", map[string]any{"course": numOf(club), "order": 1})

	var sid string
	get := func(path string) (int, string) {
		hdr := map[string]string{}
		if sid != "" {
			hdr["Cookie"] = sid
		}
		r := e.offBSend(offBReq{path: path, hdr: hdr})
		return r.Code, r.Body.String()
	}
	postForm := func(path string) (int, string) {
		hdr := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
		if sid != "" {
			hdr["Cookie"] = sid
		}
		r := e.offBSend(offBReq{method: "POST", path: path, ip: "203.0.113.40", body: strings.NewReader(""), hdr: hdr})
		return r.Code, r.Header().Get("Location")
	}

	// a visitor: the course page lists the lessons (locked but the preview), the paid lesson is locked
	_, page := get("/course/thai-cooking")
	offBContains(t, "course page", page, "฿990", `href="/members/login?next=`, "Green curry 🔒", `href="/lesson/curry-paste"`)
	_, lesson := get("/lesson/green-curry")
	if !strings.Contains(lesson, "lesson-locked") || strings.Contains(lesson, "SECRET Green curry") {
		t.Fatalf("locked lesson shows its content: %.400s", lesson)
	}
	if _, pv := get("/lesson/curry-paste"); !strings.Contains(pv, "SECRET Curry paste") || !strings.Contains(pv, "youtube-nocookie.com/embed/dQw4w9WgXcQ") {
		t.Fatal("the preview lesson is not open")
	}

	// a member signs up and buys: without Payments the enrolment waits with the payment note
	r := e.offBSend(offBReq{method: "POST", path: "/members/register", ip: "203.0.113.41", body: strings.NewReader(`{"email":"cook@x.io","password":"password1","name":"Somsri"}`),
		hdr: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}})
	for _, c := range r.Result().Cookies() {
		if strings.HasSuffix(c.Name, "sid") {
			sid = (&http.Cookie{Name: c.Name, Value: c.Value}).String()
		}
	}
	if sid == "" {
		t.Fatalf("no member session: %d %s", r.Code, r.Body.String())
	}
	if _, page = get("/course/thai-cooking"); !strings.Contains(page, `id="course-buy"`) {
		t.Fatal("no buy button for a signed-in member")
	}
	if code, loc := postForm("/learn/enroll/" + course); code != 303 || loc != "/course/thai-cooking" {
		t.Fatalf("enrol: %d %s", code, loc)
	}
	if _, page = get("/course/thai-cooking"); !strings.Contains(page, "KBank 123-4-56789-0") {
		t.Fatal("no payment note while waiting")
	}
	if _, lesson = get("/lesson/green-curry"); strings.Contains(lesson, "SECRET Green curry") {
		t.Fatal("an unpaid enrolment opened the lesson")
	}
	// the owner grants access on the Students page
	html, call := adminPage(t, e.toSample(), admin, "courses:students", "course="+course)
	offBContains(t, "students", html, "Somsri", `data-cs="active"`)
	eid := regexp.MustCompile(`data-cs="active" data-id="(\d+)"`).FindStringSubmatch(html)[1]
	if c, out := call("courses:setAccess", map[string]any{"id": eid, "status": "active"}); c != 200 || out["data"] == nil {
		t.Fatalf("grant: %d %v", c, out)
	}
	if _, lesson = get("/lesson/green-curry"); !strings.Contains(lesson, "SECRET Green curry") || !strings.Contains(lesson, `id="lesson-done"`) {
		t.Fatal("granted lesson still locked")
	}
	// lessons done → progress → certificate
	if code, loc := postForm("/learn/done/" + l1); code != 303 || loc != "/lesson/green-curry" {
		t.Fatalf("done 1: %d %s", code, loc)
	}
	_, page = get("/course/thai-cooking")
	offBContains(t, "progress", page, `aria-valuenow="50"`, "✅ ")
	if code, _ := get("/learn/certificate/" + course); code != 404 {
		t.Fatalf("certificate before the end: %d", code)
	}
	postForm("/learn/done/" + l2)
	_, page = get("/course/thai-cooking")
	offBContains(t, "complete", page, `aria-valuenow="100"`, `/learn/certificate/`+course)
	code, cert := get("/learn/certificate/" + course)
	if code != 200 || !strings.Contains(cert, "Somsri") || !strings.Contains(cert, "Thai cooking") || !strings.Contains(cert, "Anasaki Academy") {
		t.Fatalf("certificate: %d", code)
	}
	// a members-only course is open to any signed-in member, closed to visitors
	if _, w := get("/lesson/welcome"); !strings.Contains(w, "SECRET Welcome") {
		t.Fatal("members course closed to a member")
	}
	sid = ""
	if _, w := get("/lesson/welcome"); strings.Contains(w, "SECRET Welcome") {
		t.Fatal("members course open to a visitor")
	}
	_ = fmt.Sprint
}
