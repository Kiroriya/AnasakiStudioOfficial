package api

import (
	"fmt"
	"strings"
	"testing"
)

// 2.5.3: a user's own admin settings (dashboard layout): own only, any JSON up to 16 KB, null removes a key.
func TestUserPrefs(t *testing.T) {
	c, _ := newTestAPI(t)
	admin, _ := setupSite(t, c)
	if code, b := admin.doAny("GET", "/api/v1/auth/me/prefs", nil); code != 200 || len(b.(map[string]any)) != 0 {
		t.Fatalf("empty prefs: %d %v", code, b)
	}
	layout := map[string]any{"order": []any{"analytics:visitors", "primepos:waiting"}, "hidden": []any{"eshop:summary"}, "size": map[string]any{"analytics:visitors": "wide"}}
	if code, b := admin.doAny("PUT", "/api/v1/auth/me/prefs/dashboard", layout); code != 200 || !strings.Contains(strings.ToLower(fmt.Sprint(b)), "eshop:summary") {
		t.Fatalf("save: %d %v", code, b)
	}
	if code, b := admin.doAny("GET", "/api/v1/auth/me/prefs", nil); code != 200 || !strings.Contains(fmt.Sprint(b), "analytics:visitors") {
		t.Fatalf("read back: %d %v", code, b)
	}
	if code, _ := admin.doAny("PUT", "/api/v1/auth/me/prefs/Bad%21Key", layout); code != 400 && code != 404 {
		t.Fatalf("bad key: %d", code)
	}
	if code, _ := admin.doAny("PUT", "/api/v1/auth/me/prefs/dashboard", strings.Repeat("x", 20000)); code != 400 {
		t.Fatalf("too big: %d", code)
	}
	if code, b := admin.doAny("PUT", "/api/v1/auth/me/prefs/dashboard", nil); code != 200 || strings.Contains(fmt.Sprint(b), "dashboard") {
		t.Fatalf("null removes: %d %v", code, b)
	}
	// an empty body (Angular's HttpClient sends one for null) removes it too
	admin.doAny("PUT", "/api/v1/auth/me/prefs/dashboard", layout)
	if rec := admin.send("PUT", "/api/v1/auth/me/prefs/dashboard", strings.NewReader(""), nil); rec.Code != 200 || strings.Contains(rec.Body.String(), "dashboard") {
		t.Fatalf("empty body removes: %d %s", rec.Code, rec.Body.String())
	}
	if code, _ := c.doAny("GET", "/api/v1/auth/me/prefs", nil); code != 401 {
		t.Fatalf("without a session: %d", code)
	}
}
