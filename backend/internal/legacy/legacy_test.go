package legacy

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/db"
)

func TestConvert(t *testing.T) {
	tm := time.Date(2024, 5, 1, 10, 30, 0, 0, time.UTC)
	cases := []struct {
		in   any
		kind string
		want any
	}{
		{int64(7), KindInt, int64(7)},
		{[]byte("42"), KindInt, int64(42)},
		{nil, KindInt, int64(0)},
		{int64(1), KindBool, true},
		{int64(0), KindBool, false},
		{[]byte("1"), KindBool, true},
		{[]byte("abc"), KindStr, "abc"},
		{nil, KindNullStr, nil},
		{[]byte(""), KindNullStr, ""},
		{tm, KindTime, "2024-05-01T10:30:00Z"},
		{[]byte("2024-05-01 10:30:00"), KindTime, "2024-05-01T10:30:00Z"},
		{tm, KindDate, "2024-05-01"},
		{[]byte("2024-05-01"), KindDate, "2024-05-01"},
		{[]byte(`not json`), KindJSON, "not json"},
		{nil, KindJSON, nil},
	}
	for _, c := range cases {
		if got := convert(c.in, c.kind); fmt.Sprint(got) != fmt.Sprint(c.want) || fmt.Sprintf("%T", got) != fmt.Sprintf("%T", c.want) {
			t.Errorf("convert(%v, %s) = %#v, want %#v", c.in, c.kind, got, c.want)
		}
	}
	j := convert([]byte(`[{"name":"email","type":"email"}]`), KindJSON).([]any)
	if j[0].(map[string]any)["type"] != "email" {
		t.Errorf("json = %v", j)
	}
}

func TestBuiltinsShape(t *testing.T) {
	want := "analytics,seo,forms,redirects,cookie-consent,maintenance,custom-code"
	var ids []string
	for _, b := range Builtins {
		ids = append(ids, b.ID)
		for _, tb := range b.Tables {
			if len(tb.Key) == 0 || len(tb.Columns) == 0 {
				t.Errorf("%s.%s: no key/columns", b.ID, tb.Name)
			}
			if len(OldSchema[b.ID]) != len(b.Tables) {
				t.Errorf("%s: OldSchema has %d tables, Builtin %d", b.ID, len(OldSchema[b.ID]), len(b.Tables))
			}
		}
		if Get(b.ID) == nil {
			t.Errorf("Get(%s) = nil", b.ID)
		}
	}
	if strings.Join(ids, ",") != want {
		t.Errorf("ids = %v", ids)
	}
	if Get("backup") != nil {
		t.Error("backup is core, not a legacy plugin")
	}
}

// testDB opens TEST_DB_DSN and removes every old table and legacy option.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN not set (use a throw-away database)")
	}
	conn, err := db.Open(dsn)
	if err != nil {
		t.Skip("test database unreachable: ", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	clean := func() {
		for _, b := range Builtins {
			for _, tb := range b.Tables {
				_, _ = conn.Exec("DROP TABLE IF EXISTS `" + tb.Name + "`")
			}
		}
		_, _ = conn.Exec(`DELETE FROM options WHERE name = 'active_plugins' OR name LIKE 'plugin\_settings.%' OR name = 'analytics_salt'`)
	}
	clean()
	t.Cleanup(clean)
	return conn
}

func setOption(t *testing.T, conn *sql.DB, name, value string) {
	t.Helper()
	if _, err := conn.Exec(`INSERT INTO options (name, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)`, name, value); err != nil {
		t.Fatal(err)
	}
}

func TestReadersOldSchema(t *testing.T) {
	conn := testDB(t)
	for _, id := range []string{"analytics", "seo", "forms", "redirects"} {
		if err := CreateOldTables(conn, id); err != nil {
			t.Fatal(err)
		}
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO forms_forms (id, name, fields, submit_label, success_message, redirect_url, webhook_url, created_at)
		VALUES (3, 'Contact', '[{"name":"email","type":"email","required":true}]', 'Send', 'Thanks', '', 'https://hook.example/x', '2024-01-02 03:04:05'),
		       (1, 'Quote', '[]', '', '', '/thanks', '', '2023-12-31 23:59:59')`)
	mustExec(`INSERT INTO forms_submissions (id, form_id, data, is_read, created_at) VALUES
		(10, 3, '{"email":"a@b.c","msg":"สวัสดี"}', TRUE, '2024-02-01 00:00:00'), (11, 3, '{}', FALSE, '2024-02-02 00:00:00')`)
	mustExec(`INSERT INTO seo_meta (post_id, title, description, image, noindex, updated_at) VALUES
		(5, 'T', NULL, '/uploads/a.png', TRUE, '2024-03-03 03:03:03'), (6, '', 'desc', '', FALSE, '2024-03-04 00:00:00')`)
	mustExec(`INSERT INTO redirects_rules (id, from_path, to_url, code, hits, created_at) VALUES (1, '/old', '/new', 302, 9, '2024-01-01 00:00:00')`)
	mustExec(`INSERT INTO redirects_404 (path, hits, last_seen) VALUES ('/missing', 4, '2024-04-04 04:04:04')`)
	mustExec(`INSERT INTO analytics_daily (day, path, title, views) VALUES ('2024-05-01', '/', 'Home', 12), ('2024-05-01', '/a', 'A', 3)`)
	mustExec(`INSERT INTO analytics_visitors (day, hash) VALUES ('2024-05-01', 'abcdef0123456789')`)

	tableOf := func(id, name string) Table {
		for _, tb := range Get(id).Tables {
			if tb.Name == name {
				return tb
			}
		}
		t.Fatalf("no table %s", name)
		return Table{}
	}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }

	forms, err := ReadRows(conn, tableOf("forms", "forms_forms"), 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(forms) != 2 || forms[0]["id"] != int64(1) || forms[1]["id"] != int64(3) { // key order
		t.Fatalf("forms = %v", forms)
	}
	if js(forms[1]) != `{"created_at":"2024-01-02T03:04:05Z","fields":[{"name":"email","required":true,"type":"email"}],"id":3,"name":"Contact","redirect_url":"","submit_label":"Send","success_message":"Thanks","webhook_url":"https://hook.example/x"}` {
		t.Errorf("form row = %s", js(forms[1]))
	}
	subs, _ := ReadRows(conn, tableOf("forms", "forms_submissions"), 0, 500)
	if js(subs[0]) != `{"created_at":"2024-02-01T00:00:00Z","data":{"email":"a@b.c","msg":"สวัสดี"},"form_id":3,"id":10,"is_read":true}` || subs[1]["is_read"] != false {
		t.Errorf("subs = %s", js(subs))
	}
	seo, _ := ReadRows(conn, tableOf("seo", "seo_meta"), 0, 500)
	if seo[0]["description"] != nil || seo[0]["noindex"] != true || seo[1]["description"] != "desc" || seo[0]["updated_at"] != "2024-03-03T03:03:03Z" {
		t.Errorf("seo = %s", js(seo))
	}
	rr, _ := ReadRows(conn, tableOf("redirects", "redirects_rules"), 0, 500)
	if js(rr) != `[{"code":302,"created_at":"2024-01-01T00:00:00Z","from_path":"/old","hits":9,"id":1,"to_url":"/new"}]` {
		t.Errorf("rules = %s", js(rr))
	}
	r4, _ := ReadRows(conn, tableOf("redirects", "redirects_404"), 0, 500)
	if js(r4) != `[{"hits":4,"last_seen":"2024-04-04T04:04:04Z","path":"/missing"}]` {
		t.Errorf("404 = %s", js(r4))
	}
	ad, _ := ReadRows(conn, tableOf("analytics", "analytics_daily"), 0, 500)
	if js(ad) != `[{"day":"2024-05-01","path":"/","title":"Home","views":12},{"day":"2024-05-01","path":"/a","title":"A","views":3}]` {
		t.Errorf("daily = %s", js(ad))
	}
	// paging
	p1, _ := ReadRows(conn, tableOf("analytics", "analytics_daily"), 1, 1)
	if len(p1) != 1 || p1[0]["path"] != "/a" {
		t.Errorf("page 2 = %v", p1)
	}
	// a missing table reads as empty
	_, _ = conn.Exec("DROP TABLE IF EXISTS analytics_referrers")
	if rows, err := ReadRows(conn, tableOf("analytics", "analytics_referrers"), 0, 10); err != nil || len(rows) != 0 {
		t.Errorf("missing table = %v %v", rows, err)
	}

	// detection
	setOption(t, conn, OptionActive, `["analytics","seo","backup"]`)
	setOption(t, conn, OptionSettingsPrefix+"cookie-consent", `{"message":"Hi"}`)
	setOption(t, conn, "analytics_salt", "2024-05-01:aa")
	d, err := Detect(conn, "forms")
	if err != nil || d.Active || !d.HasData() || d.Rows["forms_forms"] != 2 || d.Rows["forms_submissions"] != 2 {
		t.Errorf("forms detection = %+v %v", d, err)
	}
	d, _ = Detect(conn, "seo")
	if !d.Active || !d.HasData() {
		t.Errorf("seo detection = %+v", d)
	}
	d, _ = Detect(conn, "cookie-consent")
	if d.Active || !d.HasData() || !d.HasSettings {
		t.Errorf("cookie detection = %+v", d)
	}
	d, _ = Detect(conn, "maintenance")
	if d.Active || d.HasData() || d.Leftovers() {
		t.Errorf("maintenance detection = %+v", d)
	}
	d, _ = Detect(conn, "analytics")
	if !d.Active || !d.HasOptions || len(d.Rows) != 3 {
		t.Errorf("analytics detection = %+v", d)
	}
	// empty tables: leftovers but no data
	_, _ = conn.Exec("DELETE FROM redirects_rules")
	_, _ = conn.Exec("DELETE FROM redirects_404")
	d, _ = Detect(conn, "redirects")
	if d.HasData() || !d.Leftovers() {
		t.Errorf("empty redirects = %+v", d)
	}
	if s, ok, _ := Settings(conn, "cookie-consent"); !ok || s["message"] != "Hi" {
		t.Errorf("settings = %v %v", s, ok)
	}
	if _, ok, _ := Settings(conn, "custom-code"); ok {
		t.Error("custom-code has no settings")
	}

	// clean-up: tables, options, active entry — and only those of that plugin
	if err := DropOld(conn, "analytics"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := TableExists(conn, "analytics_daily"); ok {
		t.Error("analytics_daily still exists")
	}
	if ok, _ := TableExists(conn, "seo_meta"); !ok {
		t.Error("seo_meta was dropped")
	}
	var n int
	_ = conn.QueryRow(`SELECT COUNT(*) FROM options WHERE name = 'analytics_salt'`).Scan(&n)
	if n != 0 {
		t.Error("analytics_salt kept")
	}
	act, _ := ActiveIDs(conn)
	if act["analytics"] || !act["seo"] || !act["backup"] {
		t.Errorf("active after clean-up = %v", act)
	}
	if err := DropOld(conn, "nope"); err == nil {
		t.Error("unknown id accepted")
	}
}

func TestDefaultBackoff(t *testing.T) {
	want := []time.Duration{time.Minute, time.Minute, 2 * time.Minute, 4 * time.Minute, 32 * time.Minute, time.Hour, time.Hour, time.Hour}
	for i, n := range []int{0, 1, 2, 3, 6, 7, 8, 100} {
		if got := DefaultBackoff(n); got != want[i] {
			t.Errorf("DefaultBackoff(%d) = %v, want %v", n, got, want[i])
		}
	}
}
