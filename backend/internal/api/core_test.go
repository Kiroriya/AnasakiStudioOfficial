package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/db"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/seed"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/store"
)

// ---------------------------------------------------------------- pure rules

func TestBulkGuard(t *testing.T) {
	const me = 7
	cases := []struct {
		ids          []int64
		action, role string
		blocked      bool
	}{
		{[]int64{1, me}, "suspend", "", true},
		{[]int64{me}, "delete", "", true},
		{[]int64{2, me}, "role", "subscriber", true},
		{[]int64{me}, "role", "administrator", false}, // same role: no demotion
		{[]int64{1, 2}, "suspend", "", false},
		{[]int64{me}, "activate", "", false},
	}
	for _, c := range cases {
		if got := bulkGuard(me, c.ids, c.action, c.role, "administrator") != ""; got != c.blocked {
			t.Errorf("bulkGuard(%v,%s,%s) blocked=%v want %v", c.ids, c.action, c.role, got, c.blocked)
		}
	}
}

func TestUploadRules(t *testing.T) {
	allowed, msg := parseUploadTypes(DefaultUploadTypes)
	if msg != "" {
		t.Fatal(msg)
	}
	for _, ext := range []string{"pdf", "docx", "zip", "mp3", "png", "csv"} {
		if m := uploadAllowed(ext, allowed, false); m != "" {
			t.Errorf("%s should be allowed: %s", ext, m)
		}
	}
	for _, ext := range []string{"html", "htm", "js", "php", "exe", "bat", "sh", "dll", "jar", "", "psd"} {
		if uploadAllowed(ext, allowed, true) == "" {
			t.Errorf("%q should be denied", ext)
		}
	}
	if uploadAllowed("svg", allowed, false) == "" || uploadAllowed("svg", allowed, true) != "" {
		t.Error("svg needs unfiltered_html")
	}
	if _, msg := parseUploadTypes("pdf, .PHP"); msg == "" {
		t.Error("allowed_upload_types must refuse always-denied extensions")
	}
	if got, _ := parseUploadTypes(" .PDF ,zip,pdf"); strings.Join(got, ",") != "pdf,zip" {
		t.Errorf("normalised list = %v", got)
	}
	if contentMatches("jpg", "text/html; charset=utf-8") || !contentMatches("pdf", "application/pdf") {
		t.Error("content sniffing rule")
	}
	if cd := ContentDisposition("รายงาน ประจำปี.pdf"); !strings.Contains(cd, "filename*=UTF-8''%E0%B8%A3") || !strings.HasPrefix(cd, "attachment; filename=\"download.pdf\";") {
		t.Errorf("content disposition: %s", cd)
	}
}

func TestAdminPathValidation(t *testing.T) {
	for _, v := range []string{"admin", "backoffice", "my-panel-2"} {
		if msg := validatePathOption(v, nil); msg != "" {
			t.Errorf("%q: %s", v, msg)
		}
	}
	for _, v := range []string{"ab", "Admin", "api", "uploads", "download", "plugin-assets", "a/b", "sitemap.xml", strings.Repeat("a", 33)} {
		if validatePathOption(v, nil) == "" {
			t.Errorf("%q should be rejected", v)
		}
	}
	merged := withDefaults(map[string]string{"admin_path": "blog"})
	if validateSettings(map[string]string{"admin_path": "blog"}, merged, nil) == "" {
		t.Error("admin path must not equal blog base")
	}
	merged = withDefaults(map[string]string{"admin_path": "secret-door"})
	if msg := validateSettings(map[string]string{"admin_path": "secret-door", "max_upload_mb": "64", "timezone": "Asia/Tokyo"}, merged, nil); msg != "" {
		t.Error(msg)
	}
	if validateSettings(map[string]string{"max_upload_mb": "1000"}, withDefaults(nil), nil) == "" {
		t.Error("max_upload_mb is capped at 512")
	}
	if validateSettings(map[string]string{"timezone": "Mars/Base"}, withDefaults(nil), nil) == "" {
		t.Error("invalid timezone accepted")
	}
}

// ---------------------------------------------------------------- HTTP flow (needs TEST_DB_DSN)

type client struct {
	t     *testing.T
	h     http.Handler
	token string
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (c *client) as(token string) *client { return &client{t: c.t, h: c.h, token: token} }

func newTestAPI(t *testing.T) (*client, *store.Store) {
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
	for _, tbl := range []string{"users", "posts", "post_terms", "terms", "revisions", "media", "menus", "components", "options", "roles"} {
		if _, err := conn.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatal(err)
		}
	}
	st := store.New(conn)
	if err := seed.Roles(st); err != nil {
		t.Fatal(err)
	}
	up := t.TempDir()
	pm, err := plugin.NewManager(st, up)
	if err != nil {
		t.Fatal(err)
	}
	web := site.New(st, pm)
	mux := http.NewServeMux()
	New(st, auth.New("test-secret"), up, pm, web).Routes(mux)
	return &client{t: t, h: mux}, st
}

func TestSetupUsersRolesFlow(t *testing.T) {
	c, _ := newTestAPI(t)

	code, body := c.do("GET", "/api/v1/setup", nil)
	if code != 200 || body["needsSetup"] != true || body["product"] != "Anasaki Studio" {
		t.Fatalf("GET setup: %d %v", code, body)
	}
	if code, _ := c.do("POST", "/api/v1/setup", map[string]any{"siteTitle": "X", "adminName": "A",
		"adminEmail": "a@x.io", "adminPassword": "short"}); code != 400 {
		t.Fatalf("short password accepted: %d", code)
	}
	setup := map[string]any{"siteTitle": "My Site", "tagline": "hi", "language": "en", "adminName": "Owner",
		"adminEmail": "Owner@Example.com", "adminPassword": "secret123", "locale": "en", "demo": false}
	code, body = c.do("POST", "/api/v1/setup", setup)
	if code != 201 || body["token"] == nil {
		t.Fatalf("POST setup: %d %v", code, body)
	}
	admin := c.as(body["token"].(string))
	me := body["user"].(map[string]any)
	if me["role"] != "administrator" || me["email"] != "owner@example.com" || me["lastLoginAt"] == nil {
		t.Fatalf("setup user: %v", me)
	}
	if code, _ := c.do("POST", "/api/v1/setup", setup); code != 409 {
		t.Fatalf("second setup must be 409, got %d", code)
	}
	if _, body := c.do("GET", "/api/public/site", nil); body["siteTitle"] != "My Site" || body["adminPath"] != "admin" {
		t.Fatalf("public site: %v", body)
	}
	if _, body := admin.do("GET", "/api/v1/settings", nil); body["home_page_id"] == "" || body["timezone"] != "Asia/Bangkok" {
		t.Fatalf("settings after blank setup: %v", body)
	}

	// contributor: can write, cannot publish
	code, body = admin.do("POST", "/api/v1/users", map[string]any{"name": "Con", "email": "con@x.io",
		"password": "password1", "role": "contributor", "firstName": "Con", "bio": "hello"})
	if code != 201 || body["firstName"] != "Con" || body["status"] != "active" {
		t.Fatalf("create contributor: %d %v", code, body)
	}
	conID := body["id"].(float64)
	_, login := c.do("POST", "/api/v1/auth/login", map[string]any{"email": "con@x.io", "password": "password1"})
	con := c.as(login["token"].(string))
	code, body = con.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "Draft please", "status": "publish"})
	if code != 201 || body["status"] != "draft" {
		t.Fatalf("contributor publish must be forced to draft: %d %v", code, body)
	}

	// custom role
	code, body = admin.do("POST", "/api/v1/roles", map[string]any{"slug": "media-team", "name": "Media", "caps": []string{"read", "upload_files"}})
	if code != 201 {
		t.Fatalf("create role: %d %v", code, body)
	}
	if code, _ := admin.do("PUT", "/api/v1/roles/administrator", map[string]any{"caps": []string{"read"}}); code != 400 {
		t.Fatalf("administrator must be immutable, got %d", code)
	}
	if code, _ := admin.do("POST", "/api/v1/users/bulk", map[string]any{"ids": []float64{conID}, "action": "role", "role": "media-team"}); code != 200 {
		t.Fatalf("bulk role: %d", code)
	}
	if code, _ := con.do("GET", "/api/v1/media", nil); code != 200 {
		t.Fatalf("custom role cap upload_files not applied: %d", code)
	}
	code, body = admin.do("GET", "/api/v1/roles", nil)
	if code != 200 || len(body["roles"].([]any)) != 6 || len(body["caps"].([]any)) < 11 {
		t.Fatalf("list roles: %d %v", code, body)
	}
	if code, _ := admin.do("DELETE", "/api/v1/roles/media-team", nil); code != 204 {
		t.Fatalf("delete role: %d", code)
	}
	// built-in roles: deletable (except administrator/subscriber), and restorable
	for slug, want := range map[string]int{"administrator": 400, "subscriber": 400, "contributor": 204} {
		if code, _ := admin.do("DELETE", "/api/v1/roles/"+slug, nil); code != want {
			t.Fatalf("delete built-in %s: got %d, want %d", slug, code, want)
		}
	}
	if code, body := admin.do("POST", "/api/v1/roles/restore", nil); code != 200 || len(body["roles"].([]any)) != 5 {
		t.Fatalf("restore built-in roles: %d %v", code, body)
	}
	if _, body := admin.do("GET", "/api/v1/users?role=subscriber", nil); body["total"].(float64) != 1 {
		t.Fatalf("users of a deleted role must become subscribers: %v", body)
	}

	// guards and suspension
	if code, _ := admin.do("POST", "/api/v1/users/bulk", map[string]any{"ids": []any{me["id"], conID}, "action": "suspend"}); code != 400 {
		t.Fatalf("suspending yourself must fail, got %d", code)
	}
	if code, body := admin.do("POST", "/api/v1/users/bulk", map[string]any{"ids": []float64{conID}, "action": "suspend"}); code != 200 || body["updated"].(float64) != 1 {
		t.Fatalf("bulk suspend: %d %v", code, body)
	}
	if code, _ := con.do("GET", "/api/v1/auth/me", nil); code != 403 {
		t.Fatalf("suspended token must be rejected, got %d", code)
	}
	if code, body := c.do("POST", "/api/v1/auth/login", map[string]any{"email": "con@x.io", "password": "password1"}); code != 403 || body["error"] != "บัญชีนี้ถูกระงับ" {
		t.Fatalf("suspended login: %d %v", code, body)
	}
	_, body = admin.do("GET", "/api/v1/users?status=suspended", nil)
	counts := body["counts"].(map[string]any)
	if body["total"].(float64) != 1 || counts["all"].(float64) != 2 || counts["suspended"].(float64) != 1 || counts["administrator"].(float64) != 1 {
		t.Fatalf("users list: %v", body)
	}
	if code, _ := admin.do("DELETE", "/api/v1/users/"+jsonNum(conID), nil); code != 204 {
		t.Fatalf("delete user: %d", code)
	}

	// settings: admin path
	if code, _ := admin.do("PUT", "/api/v1/settings", map[string]any{"allowed_upload_types": "pdf,exe"}); code != 400 {
		t.Fatalf("denied extension accepted in allowed_upload_types: %d", code)
	}
	if code, _ := admin.do("PUT", "/api/v1/settings", map[string]any{"admin_path": "api"}); code != 400 {
		t.Fatalf("reserved admin path accepted: %d", code)
	}
	if code, body := admin.do("PUT", "/api/v1/settings", map[string]any{"admin_path": "backstage", "max_upload_mb": "1"}); code != 200 || body["admin_path"] != "backstage" {
		t.Fatalf("set admin path: %d %v", code, body)
	}
}

func TestUploadAndDownload(t *testing.T) {
	c, st := newTestAPI(t)
	admin, err := seed.Admin(st, "Admin", "admin@x.io", "password1", "th")
	if err != nil {
		t.Fatal(err)
	}
	_, hash, _ := st.UserByEmail("admin@x.io")
	tok, _ := auth.New("test-secret").Issue(admin, hash)
	upload := func(name, content string) (int, map[string]any) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write([]byte(content))
		mw.Close()
		req := httptest.NewRequest("POST", "/api/v1/media", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		c.h.ServeHTTP(rec, req)
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, _ := upload("evil.html", "<script>alert(1)</script>"); code != 400 {
		t.Fatalf("html upload: %d", code)
	}
	if code, _ := upload("fake.jpg", "<html><body>hi</body></html>"); code != 400 {
		t.Fatalf("html disguised as jpg: %d", code)
	}
	code, m := upload("รายงาน.pdf", "%PDF-1.4 test")
	if code != 201 || m["kind"] != "document" || m["ext"] != "pdf" {
		t.Fatalf("pdf upload: %d %v", code, m)
	}
	req := httptest.NewRequest("GET", "/download/"+jsonNum(m["id"].(float64)), nil)
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment;") ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Body.String() != "%PDF-1.4 test" {
		t.Fatalf("download: %d %v", rec.Code, rec.Header())
	}
	got, _ := st.GetMedia(int64(m["id"].(float64)))
	if got.Downloads != 1 {
		t.Fatalf("downloads = %d", got.Downloads)
	}
	cl := c.as(tok)
	if _, body := cl.do("GET", "/api/v1/media?kind=image", nil); len(body) != 0 {
		// body decodes into a map only for objects; an array leaves it empty
		t.Fatalf("media filter: %v", body)
	}
}

func jsonNum(f float64) string { b, _ := json.Marshal(f); return string(b) }

func TestSanitizeKeepsTrustedEmbeds(t *testing.T) {
	in := `<p>a</p><iframe src="https://www.youtube.com/embed/abc" onload="x()"></iframe><iframe src="https://evil.example/x"></iframe><script>x</script>`
	out := sanitizeHTML(in)
	if !strings.Contains(out, `src="https://www.youtube.com/embed/abc"`) || strings.Contains(out, "evil") || strings.Contains(out, "script") || strings.Contains(out, "onload") {
		t.Fatalf("sanitize: %s", out)
	}
	forged := sanitizeHTML("\x00embed:https://evil.example\x00/embed\x00<rea-embed src=\"https://evil.example\"></rea-embed>")
	if strings.Contains(forged, "<iframe") {
		t.Fatalf("forged embed marker must not produce an iframe: %s", forged)
	}
}
