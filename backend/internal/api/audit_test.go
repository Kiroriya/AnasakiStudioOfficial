package api

import (
	"strings"
	"testing"
)

// 2.4.0: the admin activity log records changes and sign-ins (who, what, status) but never request bodies,
// keeps the title of a deleted page, and only administrators read it.
func TestAdminAuditLog(t *testing.T) {
	c, st := newTestAPI(t)
	if _, err := st.DB.Exec(`DELETE FROM admin_audit`); err != nil {
		t.Fatal(err)
	}
	admin, user := setupSite(t, c)
	c.do("POST", "/api/v1/auth/login", map[string]any{"email": "owner@x.io", "password": "wrong-secret-xyz"})
	c.do("POST", "/api/v1/auth/login", map[string]any{"email": "owner@x.io", "password": "secret123"})
	_, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Audit page", "content": "body-secret-text"})
	pid := id(p)
	admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"title": "Audit page 2"})
	admin.do("DELETE", "/api/v1/posts/"+pid, nil)
	admin.do("GET", "/api/v1/posts?type=page", nil) // reads are not logged
	admin.do("POST", "/api/v1/auth/refresh", nil)   // nor session renewal

	code, b := admin.do("GET", "/api/v1/audit?perPage=100", nil)
	if code != 200 {
		t.Fatalf("list: %d %v", code, b)
	}
	items := b["items"].([]any)
	var lines []string
	for _, it := range items {
		m := it.(map[string]any)
		lines = append(lines, m["via"].(string)+" "+m["route"].(string)+" "+m["target"].(string)+" "+m["label"].(string)+" "+jsonNum(m["status"].(float64)))
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{
		"login POST /api/v1/auth/login  owner@x.io 401",
		"login POST /api/v1/auth/login  owner@x.io 200",
		"session POST /api/v1/posts " + pid + " Audit page 2", // a create is named from its answer
		"session PUT /api/v1/posts/{id} " + pid + " Audit page 200",
		"session DELETE /api/v1/posts/{id} " + pid + " Audit page 2 204",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	for _, not := range []string{"GET /api/v1/posts", "/auth/refresh", "body-secret-text", "wrong-secret-xyz", "secret123"} {
		if strings.Contains(all, not) {
			t.Errorf("logged %q:\n%s", not, all)
		}
	}
	// filters
	if _, b := admin.do("GET", "/api/v1/audit?kind=posts", nil); b["total"].(float64) != 3 {
		t.Errorf("kind=posts: %v", b["total"])
	}
	if _, b := admin.do("GET", "/api/v1/audit?failed=1", nil); b["total"].(float64) != 1 {
		t.Errorf("failed=1: %v", b["total"])
	}
	// administrators only
	if code, _ := user("Ed", "editor").do("GET", "/api/v1/audit", nil); code != 403 {
		t.Errorf("editor reads the log: %d", code)
	}
}
