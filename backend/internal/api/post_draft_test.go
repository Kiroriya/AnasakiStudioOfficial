package api

import (
	"strconv"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/model"
)

// 2.3.0 (Qraft feedback 26): the designer's Save on a published page keeps a draft; visitors see the
// published version until Publish, and the draft can be thrown away.
func TestPublishedPageDraft(t *testing.T) {
	c, st := newTestAPI(t)
	admin, _ := setupSite(t, c)
	design := func(text string) map[string]any {
		return map[string]any{"id": "root", "type": "frame", "children": []any{
			map[string]any{"id": "h", "type": "text", "props": map[string]any{"text": text}}}}
	}
	_, page := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Tools", "slug": "tools",
		"status": "publish", "design": design("Live heading")})
	pid := id(page)
	live := func() string {
		t.Helper()
		p, err := st.PostBySlug("page", "tools", true)
		if err != nil {
			t.Fatal(err)
		}
		return p.Title + " | " + string(p.Design)
	}

	// Save (asDraft): stored as the draft, the site is unchanged
	code, b := admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"title": "Tools v2", "slug": "tools",
		"design": design("Work in progress"), "asDraft": true, "baseUpdatedAt": page["updatedAt"]})
	if code != 200 || b["draft"] == nil {
		t.Fatalf("draft save: %d %v", code, b)
	}
	if s := live(); !strings.Contains(s, "Live heading") || strings.Contains(s, "Work in progress") || !strings.HasPrefix(s, "Tools |") {
		t.Fatalf("the draft reached the site: %s", s)
	}
	_, got := admin.do("GET", "/api/v1/posts/"+pid, nil)
	draft, _ := got["draft"].(map[string]any)
	if draft == nil || draft["title"] != "Tools v2" || !strings.Contains(mustJSON(draft["design"]), "Work in progress") ||
		!strings.Contains(mustJSON(got["design"]), "Live heading") {
		t.Fatalf("GET after the draft: %v", got)
	}
	// a stale tab (loaded before the draft) gets 409, as for a normal save
	if code, _ := admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"design": design("Stale"), "asDraft": true,
		"baseUpdatedAt": page["updatedAt"]}); code != 409 {
		t.Fatalf("stale draft save: %d", code)
	}
	// a broken design is refused like a real save
	if code, _ := admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"design": map[string]any{"id": "root", "type": "frame",
		"children": []any{map[string]any{"id": "x", "type": "text"}, map[string]any{"id": "x", "type": "text"}}}, "asDraft": true}); code != 400 {
		t.Fatalf("draft with repeated ids: %d", code)
	}

	// Discard: the draft goes, the site never changed
	if code, b := admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"discardDraft": true}); code != 200 || b["draft"] != nil {
		t.Fatalf("discard: %d %v", code, b)
	}

	// Save again, then Publish: the draft goes live and is cleared
	admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"title": "Tools v3", "design": design("Second draft"), "asDraft": true})
	code, b = admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"title": "Tools v3", "slug": "tools", "design": design("Second draft"), "status": "publish"})
	if code != 200 || b["draft"] != nil {
		t.Fatalf("publish: %d %v", code, b)
	}
	if s := live(); !strings.Contains(s, "Second draft") || !strings.HasPrefix(s, "Tools v3 |") {
		t.Fatalf("publish did not reach the site: %s", s)
	}

	// a draft page has nothing to protect: asDraft saves it as usual
	_, d := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Hidden", "status": "draft", "design": design("A")})
	code, b = admin.do("PUT", "/api/v1/posts/"+id(d), map[string]any{"design": design("B"), "asDraft": true})
	if code != 200 || b["draft"] != nil || !strings.Contains(mustJSON(b["design"]), `"B"`) {
		t.Fatalf("asDraft on a draft page: %d %v", code, b)
	}
}

// 2.3.0 (Qraft feedback 28): status=any lists every status; an unknown status is a 400, not an empty list.
func TestListPostsStatusAny(t *testing.T) {
	c, _ := newTestAPI(t)
	admin, _ := setupSite(t, c)
	admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Live", "status": "publish"})
	admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Hidden", "status": "draft"})
	count := func(q string) (int, float64) {
		code, b := admin.do("GET", "/api/v1/posts?type=page&perPage=50"+q, nil)
		total, _ := b["total"].(float64)
		return code, total
	}
	_, all := count("")
	for _, q := range []string{"&status=any", "&status=all"} {
		if code, n := count(q); code != 200 || n != all || n < 2 {
			t.Fatalf("%s: %d %v (all %v)", q, code, n, all)
		}
	}
	if code, n := count("&status=draft"); code != 200 || n < 1 || n >= all {
		t.Fatalf("draft: %d %v", code, n)
	}
	if code, _ := count("&status=trash"); code != 400 {
		t.Fatalf("unknown status: %d", code)
	}
}

// 2.3.0 (Qraft feedback 32): a list's category filter takes a term's slug, id or name, and a custom type's
// list matches its own taxonomy (e.g. tool_cat), not only "category".
func TestCategoryFilterAnyTaxonomy(t *testing.T) {
	c, st := newTestAPI(t)
	admin, _ := setupSite(t, c)
	_, p := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "QR maker", "status": "publish"})
	pid := int64(p["id"].(float64))
	res, err := st.DB.Exec(`INSERT INTO terms (taxonomy, name, slug) VALUES ('tool_cat', 'QR & ลิงก์', 'qr-link')`)
	if err != nil {
		t.Fatal(err)
	}
	tid, _ := res.LastInsertId()
	if _, err := st.DB.Exec(`INSERT INTO post_terms (post_id, term_id) VALUES (?, ?)`, pid, tid); err != nil {
		t.Fatal(err)
	}
	list := func(cat string, any bool) int {
		t.Helper()
		posts, _, err := st.ListPosts(model.PostFilter{Type: "page", Category: cat, AnyTaxonomy: any, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		return len(posts)
	}
	for _, cat := range []string{"qr-link", "QR & ลิงก์", strconv.FormatInt(tid, 10)} {
		if n := list(cat, true); n != 1 {
			t.Errorf("%q in any taxonomy: %d", cat, n)
		}
		if n := list(cat, false); n != 0 {
			t.Errorf("%q matched outside category: %d", cat, n)
		}
	}
	if n := list("nope", true); n != 0 {
		t.Errorf("unknown term: %d", n)
	}
}
