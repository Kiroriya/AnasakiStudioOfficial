package api

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/legacy"
)

// plugins-official/analytics: history taken over from the built-in Go plugin. A site "from before the upgrade"
// (the old analytics_* tables, ten days of traffic — more than one 500-row batch per table) is streamed to
// legacyImport exactly like the core migration does (legacy.ReadRows → {kind, table, rows, last}), and the
// plugin's summary(7) must give the numbers the old plugin's GET /api/v1/analytics/summary gave on the same
// tables (its SQL is replayed below): totals, every day of the chart, the top pages with their visitors.

// oldSummary is the built-in plugin's summary (internal/plugins/analytics/summary.go before the move),
// read from the old tables. Days are server-local, like the old plugin's time.Now().Format.
func oldSummary(t *testing.T, db *sql.DB, days int) (totals [4]int, daily []map[string]int, top []map[string]any) {
	t.Helper()
	now := time.Now()
	today := now.Format("2006-01-02")
	since := now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	views, visitors := map[string]int{}, map[string]int{}
	scan := func(q string, into map[string]int) {
		rows, err := db.Query(q, since)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var d time.Time
			var n int
			if err := rows.Scan(&d, &n); err != nil {
				t.Fatal(err)
			}
			into[d.Format("2006-01-02")] = n
		}
	}
	scan(`SELECT day, SUM(views) FROM analytics_daily WHERE day >= ? GROUP BY day`, views)
	scan(`SELECT day, COUNT(*) FROM analytics_visitors WHERE day >= ? GROUP BY day`, visitors)
	for i := days - 1; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		daily = append(daily, map[string]int{d: 0})
		daily[len(daily)-1][d] = views[d]*1_000_000 + visitors[d] // packed: compared as one number per day
		totals[0] += views[d]
		totals[1] += visitors[d]
		if d == today {
			totals[2], totals[3] = views[d], visitors[d]
		}
	}
	rows, err := db.Query(`SELECT path, SUM(views) v, MAX(title) FROM analytics_daily WHERE day >= ? GROUP BY path ORDER BY v DESC, path LIMIT 10`, since)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p, title string
		var v int
		if err := rows.Scan(&p, &v, &title); err != nil {
			t.Fatal(err)
		}
		top = append(top, map[string]any{"path": p, "views": v, "title": title})
	}
	rows.Close()
	for _, p := range top {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM analytics_page_visitors WHERE day >= ? AND path = ?`, since, p["path"]).Scan(&n); err != nil {
			t.Fatal(err)
		}
		p["visitors"] = n
	}
	return
}

func TestOfficialAnalyticsHistory(t *testing.T) {
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "analytics")
	db := e.st.DB
	b := legacy.Get("analytics")
	drop := func() {
		for _, tb := range b.Tables {
			_, _ = db.Exec("DROP TABLE IF EXISTS `" + tb.Name + "`")
		}
	}
	drop()
	t.Cleanup(drop)
	if err := legacy.CreateOldTables(db, "analytics"); err != nil {
		t.Fatal(err)
	}

	// ten days of traffic written the way the old plugin wrote it (server-local days, one row per day + key)
	paths := []string{"/about", "/contact", "/blog/hello", "/services", "/News", "/news", "/pricing", "/faq"}
	type dk struct{ day, key string }
	dailyViews, refViews := map[dk]int{}, map[dk]int{}
	visitors, pageVisitors := map[dk]bool{}, map[[3]string]bool{}
	wantViews, wantVisitors := map[string]int{}, map[string]int{}
	for ago := 9; ago >= 0; ago-- {
		day := time.Now().AddDate(0, 0, -ago).Format("2006-01-02")
		n := 70 + (ago*37)%41 // visitors that day
		for v := 0; v < n; v++ {
			hash := fmt.Sprintf("%08x%08x", ago, v)
			visitors[dk{day, hash}] = true
			wantVisitors[day]++
			for k := 0; k <= (v+ago)%3; k++ { // 1–3 pages, some twice
				p := paths[(v*3+k*5+ago)%len(paths)]
				hits := 1 + (v+k)%2
				dailyViews[dk{day, p}] += hits
				wantViews[day] += hits
				pageVisitors[[3]string{day, p, hash}] = true
				if (v+k)%4 == 0 {
					refViews[dk{day, []string{"google.com", "facebook.com", "line.me"}[(v+k)%3]}] += hits
				}
			}
		}
	}
	insert := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for k, n := range dailyViews {
		insert(`INSERT INTO analytics_daily (day, path, title, views) VALUES (?,?,?,?)`, k.day, k.key, "Title of "+k.key, n)
	}
	for k := range visitors {
		insert(`INSERT INTO analytics_visitors (day, hash) VALUES (?,?)`, k.day, k.key)
	}
	for k := range pageVisitors {
		insert(`INSERT INTO analytics_page_visitors (day, path, hash) VALUES (?,?,?)`, k[0], k[1], k[2])
	}
	for k, n := range refViews {
		insert(`INSERT INTO analytics_referrers (day, host, views) VALUES (?,?,?)`, k.day, k.key, n)
	}
	if len(visitors) <= legacy.MaxBatch || len(pageVisitors) <= 2*legacy.MaxBatch {
		t.Fatalf("test data too small for batching: %d visitors, %d page visitors", len(visitors), len(pageVisitors))
	}

	// what the old plugin showed (and a cross-check of its SQL against the generator)
	oldTot, oldDaily, oldTop := oldSummary(t, db, 7)
	gen := [2]int{}
	for ago := 6; ago >= 0; ago-- {
		d := time.Now().AddDate(0, 0, -ago).Format("2006-01-02")
		gen[0] += wantViews[d]
		gen[1] += wantVisitors[d]
	}
	if oldTot[0] != gen[0] || oldTot[1] != gen[1] {
		t.Fatalf("old summary %v, generated %v", oldTot, gen)
	}

	// the core migration's loop (internal/legacy/migrate.go importRows): batches of 500, last flag by look-ahead
	type batch struct {
		table string
		n     int
		last  bool
	}
	var sent []batch
	imported := 0.0
	for _, tb := range b.Tables {
		for offset := int64(0); ; {
			rows, err := legacy.ReadRows(db, tb, offset, legacy.MaxBatch)
			if err != nil {
				t.Fatal(err)
			}
			last := len(rows) < legacy.MaxBatch
			if !last {
				more, err := legacy.ReadRows(db, tb, offset+legacy.MaxBatch, 1)
				if err != nil {
					t.Fatal(err)
				}
				last = len(more) == 0
			}
			for _, r := range rows {
				if d, _ := r["day"].(string); len(d) != 10 {
					t.Fatalf("legacy reader: day %v (want YYYY-MM-DD)", r["day"])
				}
			}
			out := offBExport(t, e, admin, "analytics", "legacyImport", map[string]any{"kind": "rows", "table": tb.Name, "rows": rows, "last": last})
			imported += out["imported"].(float64)
			sent = append(sent, batch{tb.Name, len(rows), last})
			offset += int64(len(rows))
			if last {
				break
			}
		}
	}
	total := len(dailyViews) + len(visitors) + len(pageVisitors) + len(refViews)
	if int(imported) != total {
		t.Fatalf("imported %v of %d rows (batches %v)", imported, total, sent)
	}

	check := func(when string) {
		t.Helper()
		sum := offBExport(t, e, admin, "analytics", "summary", 7)
		tot := sum["totals"].(map[string]any)
		got := [4]int{int(tot["views"].(float64)), int(tot["visitors"].(float64)), int(tot["todayViews"].(float64)), int(tot["todayVisitors"].(float64))}
		if got != oldTot {
			t.Fatalf("%s: totals %v, old plugin %v", when, got, oldTot)
		}
		d := sum["daily"].([]any)
		if len(d) != 7 {
			t.Fatalf("%s: %d days", when, len(d))
		}
		for i, x := range d {
			m := x.(map[string]any)
			day := m["day"].(string)
			want, ok := oldDaily[i][day]
			if !ok {
				t.Fatalf("%s: day %d is %s, old plugin %v (time zone?)", when, i, day, oldDaily[i])
			}
			if g := int(m["views"].(float64))*1_000_000 + int(m["visitors"].(float64)); g != want {
				t.Fatalf("%s: %s views/visitors %v/%v, old plugin %d/%d", when, day, m["views"], m["visitors"], want/1_000_000, want%1_000_000)
			}
		}
		if d[6].(map[string]any)["day"] != time.Now().Format("2006-01-02") {
			t.Fatalf("%s: last day %v is not today (server-local)", when, d[6])
		}
		top := sum["topPages"].([]any)
		if len(top) != len(oldTop) {
			t.Fatalf("%s: top pages %v, old plugin %v", when, top, oldTop)
		}
		for i, x := range top {
			m, o := x.(map[string]any), oldTop[i]
			if m["path"] != o["path"] || int(m["views"].(float64)) != o["views"] || int(m["visitors"].(float64)) != o["visitors"] || m["title"] != o["title"] {
				t.Fatalf("%s: top page %d %v, old plugin %v", when, i, m, o)
			}
		}
		// referrers of the 7 days
		since := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
		wantRef := map[string]int{}
		for k, n := range refViews {
			if k.day >= since {
				wantRef[k.key] += n
			}
		}
		gotRef := map[string]int{}
		for _, x := range sum["topReferrers"].([]any) {
			gotRef[x.(map[string]any)["host"].(string)] = int(x.(map[string]any)["views"].(float64))
		}
		if fmt.Sprint(gotRef) != fmt.Sprint(wantRef) {
			t.Fatalf("%s: referrers %v, want %v", when, gotRef, wantRef)
		}
	}
	check("after import")

	// the per-day totals table equals the raw rows for every imported day (also the 3 days outside the window)
	days := offBRows(t, e, "p_analytics_days", "")
	if len(days) != 10 {
		t.Fatalf("days rows: %v", days)
	}
	var bad []string
	for _, r := range days {
		d := r["day"].(string)
		if int(r["views"].(int64)) != wantViews[d] || int(r["visitors"].(int64)) != wantVisitors[d] {
			bad = append(bad, fmt.Sprintf("%s %v/%v want %d/%d", d, r["views"], r["visitors"], wantViews[d], wantVisitors[d]))
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("p_analytics_days:\n%s", strings.Join(bad, "\n"))
	}

	// the migration sent again (a retry after a crash) changes nothing
	for _, tb := range b.Tables {
		rows, err := legacy.ReadRows(db, tb, 0, legacy.MaxBatch)
		if err != nil {
			t.Fatal(err)
		}
		offBExport(t, e, admin, "analytics", "legacyImport", map[string]any{"kind": "rows", "table": tb.Name, "rows": rows, "last": false})
	}
	check("after a repeated batch")
	offBQuiet(t, admin, "analytics")
}
