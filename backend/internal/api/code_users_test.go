package api

import (
	"strings"
	"testing"
)

// CMS 2.2.2: db.users (scope "users") gives code read-only public profiles — never the email; suspended users
// are left out; a plugin without the scope is refused.
func TestCodeDBUsers(t *testing.T) {
	e := newPluginAPI(t)
	admin, user := setupSite(t, e.c)
	ed := user("Ed Writer", "editor")
	_ = ed
	_, me := admin.do("GET", "/api/v1/auth/me", nil)
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "team",
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"routes", "users"}, "routes": []any{"/team"}}},
		map[string]string{"code/main.ts": `
site.route('GET /team', { access: ['*'] }, () => {
  const all = db.users.list({ perPage: 50 });
  const one = db.users.get(all.items[0].id);
  return { total: all.total, items: all.items, one, missing: db.users.get(999999) };
});
`}))
	activate(t, admin, "team")
	rec := e.c.send("GET", "/team", nil, nil)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `"name":"Ed Writer"`) || !strings.Contains(body, `"missing":null`) || !strings.Contains(body, `"postCount"`) {
		t.Fatalf("GET /team: %d %s", rec.Code, body)
	}
	if email, _ := me["email"].(string); email == "" || strings.Contains(body, email) || strings.Contains(body, "@") {
		t.Fatalf("an email leaked: %s", body)
	}
	// without the scope
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "noscope",
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"routes"}, "routes": []any{"/noscope"}}},
		map[string]string{"code/main.ts": `site.route('GET /noscope', { access: ['*'] }, () => db.users.list());`}))
	activate(t, admin, "noscope")
	if rec := e.c.send("GET", "/noscope", nil, nil); rec.Code == 200 {
		t.Fatalf("db.users without the scope: %d %s", rec.Code, rec.Body.String())
	}
}
