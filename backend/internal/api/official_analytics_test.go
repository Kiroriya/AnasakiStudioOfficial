package api

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// plugins-official/analytics: the behaviour of the built-in Go plugin (internal/plugins/analytics and its
// tests) through public joints only — afterResponse counting, bots / prefetches / non-pages skipped,
// daily salted visitor hashes, referrers, the summary, the admin page and the legacy import.

func TestOfficialAnalytics(t *testing.T) {
	e := offBNew(t)
	admin, user := setupSite(t, e.c)
	offBInstall(t, admin, "analytics")
	for _, tb := range []string{"p_analytics_daily", "p_analytics_visitors", "p_analytics_page_visitors", "p_analytics_referrers", "p_analytics_days", "p_analytics_salts"} {
		if !codeTable(t, e.pluginEnv, tb) {
			t.Fatalf("migration did not create %s", tb)
		}
	}
	offBPage(t, admin, "about", "About us")
	offBPage(t, admin, "contact", "Contact")
	hit := func(r offBReq) int {
		t.Helper()
		if r.hdr == nil {
			r.hdr = map[string]string{}
		}
		if _, ok := r.hdr["User-Agent"]; !ok {
			r.hdr["User-Agent"] = offBHuman
		}
		if r.ip == "" {
			r.ip = "203.0.113.9"
		}
		rec := e.offBSend(r)
		e.code.WaitAfterResponse()
		return rec.Code
	}
	views := func(path string) int {
		n := 0
		for _, r := range offBRows(t, e, "p_analytics_daily", "path = ?", path) {
			n += int(r["views"].(int64))
		}
		return n
	}

	// people: two views by the same visitor, one by another
	for i := 0; i < 2; i++ {
		if c := hit(offBReq{path: "/about"}); c != 200 {
			t.Fatalf("GET /about: %d", c)
		}
	}
	hit(offBReq{path: "/about", ip: "198.51.100.7", hdr: map[string]string{"Referer": "https://www.google.com/search?q=x"}})
	hit(offBReq{path: "/contact", hdr: map[string]string{"Referer": "http://example.com/about"}}) // internal referrer
	if views("/about") != 2+1 || views("/contact") != 1 {
		t.Fatalf("views: about=%d contact=%d", views("/about"), views("/contact"))
	}
	daily := offBRows(t, e, "p_analytics_daily", "path = ?", "/about")
	if daily[0]["title"] != "About us" {
		t.Fatalf("title: %v", daily)
	}

	// not counted: bots / tools, prefetches, POST, 404, files, API
	bots := []string{"", "Googlebot/2.1 (+http://www.google.com/bot.html)", "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"curl/8.4.0", "python-requests/2.31", "facebookexternalhit/1.1",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 HeadlessChrome/120.0 Safari/537.36", "Mozilla/5.0 (compatible; AhrefsBot/7.0)",
		"Go-http-client/1.1", "Wget/1.21", "short ua"}
	for _, ua := range bots {
		hit(offBReq{path: "/about", hdr: map[string]string{"User-Agent": ua}})
	}
	hit(offBReq{path: "/about", hdr: map[string]string{"Sec-Purpose": "prefetch"}})
	hit(offBReq{path: "/about", hdr: map[string]string{"Purpose": "prefetch"}})
	hit(offBReq{path: "/about", method: "POST"})
	hit(offBReq{path: "/nope-404"})
	hit(offBReq{path: "/robots.txt"})
	hit(offBReq{path: "/sitemap.xml"})
	hit(offBReq{path: "/uploads/a.png"})
	if v := views("/about"); v != 3 {
		t.Fatalf("bots / prefetch / POST counted: %d", v)
	}
	if n := len(offBRows(t, e, "p_analytics_daily", "path NOT IN ('/about','/contact')")); n != 0 {
		t.Fatalf("other paths counted: %v", offBRows(t, e, "p_analytics_daily", ""))
	}
	// an iPhone is a person; .html pages count
	hit(offBReq{path: "/contact", hdr: map[string]string{"User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"}})
	if views("/contact") != 2 {
		t.Fatalf("iPhone not counted: %d", views("/contact"))
	}

	// visitors: daily salted hash (16 hex), never the IP; one salt row (today)
	vis := offBRows(t, e, "p_analytics_visitors", "")
	if len(vis) != 3 { // 203.0.113.9 desktop, 198.51.100.7, 203.0.113.9 iPhone
		t.Fatalf("visitors: %v", vis)
	}
	for _, v := range vis {
		h := v["hash"].(string)
		if len(h) != 16 || strings.Trim(h, "0123456789abcdef") != "" {
			t.Fatalf("hash: %q", h)
		}
	}
	for _, tb := range []string{"p_analytics_daily", "p_analytics_visitors", "p_analytics_page_visitors", "p_analytics_referrers", "p_analytics_days", "p_analytics_salts"} {
		if s := fmt.Sprint(offBRows(t, e, tb, "")); strings.Contains(s, "203.0.113.9") || strings.Contains(s, "198.51.100.7") {
			t.Fatalf("raw IP stored in %s: %s", tb, s)
		}
	}
	if s := offBRows(t, e, "p_analytics_salts", ""); len(s) != 1 || len(s[0]["salt"].(string)) != 32 {
		t.Fatalf("salts: %v", s)
	}
	if pv := offBRows(t, e, "p_analytics_page_visitors", "path = ?", "/about"); len(pv) != 2 {
		t.Fatalf("page visitors: %v", pv)
	}
	refs := offBRows(t, e, "p_analytics_referrers", "")
	if len(refs) != 1 || refs[0]["host"] != "google.com" || refs[0]["views"].(int64) != 1 {
		t.Fatalf("referrers (www. stripped, internal skipped): %v", refs)
	}

	// summary export (same JSON as GET /api/v1/analytics/summary)
	sum := offBExport(t, e, admin, "analytics", "summary", 7)
	tot := sum["totals"].(map[string]any)
	if tot["views"].(float64) != 5 || tot["todayViews"].(float64) != 5 || tot["visitors"].(float64) != 3 || tot["todayVisitors"].(float64) != 3 {
		t.Fatalf("totals: %v", tot)
	}
	if d := sum["daily"].([]any); len(d) != 7 || d[6].(map[string]any)["day"] != time.Now().Format("2006-01-02") {
		t.Fatalf("daily: %v", d)
	}
	top := sum["topPages"].([]any)
	if len(top) != 2 || top[0].(map[string]any)["path"] != "/about" || top[0].(map[string]any)["views"].(float64) != 3 ||
		top[0].(map[string]any)["visitors"].(float64) != 2 || top[0].(map[string]any)["title"] != "About us" {
		t.Fatalf("top pages: %v", top)
	}
	if r := sum["topReferrers"].([]any); len(r) != 1 || r[0].(map[string]any)["host"] != "google.com" {
		t.Fatalf("top referrers: %v", r)
	}
	if d := offBExport(t, e, admin, "analytics", "summary", 30)["daily"].([]any); len(d) != 30 {
		t.Fatalf("30 days: %d", len(d))
	}
	if d := offBExport(t, e, admin, "analytics", "summary", 12)["daily"].([]any); len(d) != 7 {
		t.Fatalf("unknown range → 7: %d", len(d))
	}

	// admin page: administrators and editors (was view_stats), English for the owner
	html, _ := adminPage(t, e.toSample(), admin, "analytics:stats", "days=7")
	offBContains(t, "admin page", html, "<title>Visitor statistics</title>", `data-stat="todayViews"><span>👁 Views today</span><b>5</b>`,
		`data-path="/about"`, "About us", `data-host="google.com"`, "No cookies, no stored IPs", `data-ak-bar=`, `class="on" data-days="7"`)
	ed := user("Ed", "editor")
	if code, _ := ed.do("POST", "/api/v1/code/admin-pages/analytics:stats/ticket", map[string]any{}); code != 200 {
		t.Fatalf("editor ticket: %d", code)
	}
	if code, _ := user("Au", "author").do("POST", "/api/v1/code/admin-pages/analytics:stats/ticket", map[string]any{}); code == 200 {
		t.Fatal("author may open analytics")
	}
	if code, out := admin.do("PUT", "/api/v1/auth/me", map[string]any{"locale": "th"}); code != 200 {
		t.Fatalf("locale: %d %v", code, out)
	}
	html, _ = adminPage(t, e.toSample(), admin, "analytics:stats", "")
	offBContains(t, "admin page (th)", html, "<title>สถิติผู้เข้าชม</title>", "การเข้าชม 30 วัน", `class="on" data-days="30"`, "ไม่ใช้คุกกี้")

	// legacy import (rows shaped like the old analytics_* tables), idempotent
	yday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")
	imp := func(table string, rows []map[string]any) float64 {
		out := offBExport(t, e, admin, "analytics", "legacyImport", map[string]any{"kind": "rows", "table": table, "rows": rows, "last": true})
		return out["imported"].(float64)
	}
	for round := 0; round < 2; round++ {
		if n := imp("analytics_daily", []map[string]any{
			{"day": yday + "T00:00:00Z", "path": "/old", "title": "Old page", "views": 40},
			{"day": yday, "path": "/about", "title": "", "views": 4},
			{"day": today, "path": "/about", "title": "About (old)", "views": 1}, // smaller than the live count: kept
		}); n != 3 {
			t.Fatalf("daily imported: %v", n)
		}
		imp("analytics_visitors", []map[string]any{{"day": yday, "hash": "aaaaaaaaaaaaaaaa"}, {"day": yday, "hash": "bbbbbbbbbbbbbbbb"}})
		imp("analytics_page_visitors", []map[string]any{{"day": yday, "path": "/old", "hash": "aaaaaaaaaaaaaaaa"}})
		imp("analytics_referrers", []map[string]any{{"day": yday, "host": "facebook.com", "views": 9}})
		if n := imp("unknown_table", []map[string]any{{"x": 1}}); n != 0 {
			t.Fatalf("unknown table: %v", n)
		}
	}
	sum = offBExport(t, e, admin, "analytics", "summary", 7)
	tot = sum["totals"].(map[string]any)
	if tot["views"].(float64) != 5+44 || tot["visitors"].(float64) != 3+2 || tot["todayViews"].(float64) != 5 {
		t.Fatalf("totals after import: %v", tot)
	}
	top = sum["topPages"].([]any)
	if top[0].(map[string]any)["path"] != "/old" || top[0].(map[string]any)["views"].(float64) != 40 || top[0].(map[string]any)["visitors"].(float64) != 1 ||
		top[0].(map[string]any)["title"] != "Old page" {
		t.Fatalf("top after import: %v", top)
	}
	if fmt.Sprint(sum["topReferrers"]) != "[map[host:facebook.com views:9] map[host:google.com views:1]]" {
		t.Fatalf("referrers after import: %v", sum["topReferrers"])
	}
	if v := views("/about"); v != 3+4 {
		t.Fatalf("about after import: %d", v)
	}
	// counting goes on after the import
	hit(offBReq{path: "/about"})
	if v := views("/about"); v != 8 {
		t.Fatalf("about after a new view: %d", v)
	}

	// the dashboard widget "Visitors, last 7 days" (administrators and editors)
	if _, wl := user("Au2", "author").doAny("GET", "/api/v1/code/dashboard-widgets", nil); len(wl.([]any)) != 0 {
		t.Fatalf("author widgets: %v", wl)
	}
	_, wl := ed.doAny("GET", "/api/v1/code/dashboard-widgets", nil)
	if l := wl.([]any); len(l) != 1 || l[0].(map[string]any)["id"] != "analytics:visitors" || l[0].(map[string]any)["label"].(map[string]any)["en"] != "Visitors, last 7 days" {
		t.Fatalf("widgets: %v", wl)
	}
	rec := e.c.send("GET", wl.([]any)[0].(map[string]any)["url"].(string), nil, nil)
	offBContains(t, "widget", rec.Body.String(), `id="an-widget"`, `data-stat="views"`, `<b>50</b>`, `data-ak-bar=`, "/ext-ui/kit.js")

	// case-sensitive paths (utf8mb4_bin, like the built-in tables): /CaseTest and /casetest are two pages
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('GET /CaseTest', { access: ['*'] }, () => '<!doctype html><title>Upper</title><p>U</p>')
site.route('GET /casetest', { access: ['*'] }, () => '<!doctype html><title>lower</title><p>l</p>')`})
	hit(offBReq{path: "/CaseTest"})
	hit(offBReq{path: "/casetest"})
	hit(offBReq{path: "/casetest"})
	if views("/CaseTest") != 1 || views("/casetest") != 2 {
		t.Fatalf("case-sensitive paths: %v", offBRows(t, e, "p_analytics_daily", "LOWER(path) = 'casetest' OR path LIKE '/%asetest'"))
	}
	offBQuiet(t, admin, "analytics")
}
