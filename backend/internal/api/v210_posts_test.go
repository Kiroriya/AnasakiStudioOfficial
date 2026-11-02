package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// CMS 2.1.0 (P1-3): scheduled posts in core and the full post input of
// db.posts (authorId, publishedAt, imageId, pinned, sortOrder, design) plus
// db.posts.list({status, authorId, ids}).

func TestScheduledPosts(t *testing.T) {
	c, code, _ := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": `
site.on('post.published', (p) => { cache.set('pub', p.title) })
site.route('GET /pub', { access: ['*'] }, () => cache.get('pub') ?? null)
site.route('GET /visible/{id}', { access: ['*'] }, ({ request }) => db.posts.get(Number(request.params.id)))
`})
	future := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	// "publish" with a future date → scheduled; hidden from the public site
	code1, p := admin.do("POST", "/api/v1/posts", map[string]any{"title": "Later", "slug": "later", "status": "publish", "publishedAt": future})
	if code1 != 201 || p["status"] != "scheduled" || !strings.HasPrefix(p["publishedAt"].(string), future[:16]) {
		t.Fatalf("schedule: %d %v", code1, p)
	}
	if code, _ := c.public("GET", "/blog/later"); code != 404 {
		t.Fatalf("scheduled post is public: %d", code)
	}
	if _, b := c.public("GET", "/visible/"+id(p)); strings.Contains(b, "Later") {
		t.Fatalf("db.posts.get shows a scheduled post to a visitor: %s", b)
	}
	// scheduled without a date is refused; a contributor's schedule becomes a draft
	if code, _ := admin.do("POST", "/api/v1/posts", map[string]any{"title": "x", "status": "scheduled"}); code != 400 {
		t.Fatalf("scheduled without publishedAt: %d", code)
	}
	con := user("Co", "contributor")
	if code, b := con.do("POST", "/api/v1/posts", map[string]any{"title": "c", "status": "scheduled", "publishedAt": future}); code != 201 || b["status"] != "draft" {
		t.Fatalf("contributor schedules: %d %v", code, b)
	}
	// the list filter
	if _, l := admin.do("GET", "/api/v1/posts?status=scheduled", nil); l["total"] != 1.0 {
		t.Fatalf("status=scheduled list: %v", l)
	}
	// nothing is due yet
	if n := c.api.PublishScheduled(time.Now()); n != 0 {
		t.Fatalf("published early: %d", n)
	}
	// the time comes: published once, events fire, the page is public
	if n := c.api.PublishScheduled(time.Now().Add(3 * time.Hour)); n != 1 {
		t.Fatalf("due: %d", n)
	}
	if n := c.api.PublishScheduled(time.Now().Add(3 * time.Hour)); n != 0 {
		t.Fatalf("published twice: %d", n)
	}
	code.WaitEvents()
	if _, b := c.public("GET", "/pub"); strings.Trim(strings.TrimSpace(b), `"`) != "Later" {
		t.Fatalf("post.published from the scheduler: %s", b)
	}
	if _, g := admin.do("GET", "/api/v1/posts/"+id(p), nil); g["status"] != "publish" || !strings.HasPrefix(g["publishedAt"].(string), future[:16]) {
		t.Fatalf("after the scheduler: %v", g)
	}
	// back-dating a published post keeps it published
	past := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC).Format(time.RFC3339)
	if code, b := admin.do("PUT", "/api/v1/posts/"+id(p), map[string]any{"status": "publish", "publishedAt": past}); code != 200 || b["status"] != "publish" || b["publishedAt"] != past {
		t.Fatalf("back-date: %d %v", code, b)
	}
	// a contributor cannot touch a scheduled post (same rule as a published one)
	_, s2 := admin.do("POST", "/api/v1/posts", map[string]any{"title": "S2", "status": "scheduled", "publishedAt": future})
	if code, _ := con.do("PUT", "/api/v1/posts/"+id(s2), map[string]any{"title": "hijack"}); code != 403 {
		t.Fatalf("contributor edits a scheduled post: %d", code)
	}
}

func TestCodePostInputFull(t *testing.T) {
	c, _, st := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	if _, err := st.DB.Exec(`INSERT INTO media (id, filename, url, mime, size, author_id) VALUES (901, 'a.png', '/uploads/a.png', 'image/png', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('POST /mk', { access: ['*'] }, ({ request }) => db.posts.create(request.json()))
site.route('POST /upd/{id}', { access: ['*'] }, ({ request }) => db.posts.update(Number(request.params.id), request.json()))
site.route('POST /list', { access: ['*'] }, ({ request }) => db.posts.list(request.json()))
`})
	post := func(cl *client, path string, body any) (int, map[string]any) {
		b, _ := json.Marshal(body)
		rec := cl.send("POST", path, strings.NewReader(string(b)), map[string]string{"Accept": "application/json", "Content-Type": "application/json"})
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	_, me := admin.do("GET", "/api/v1/auth/me", nil)
	adminID := me["id"]
	if adminID == nil {
		adminID = me["user"].(map[string]any)["id"]
	}
	ed := user("Ed", "editor")
	_, edMe := ed.do("GET", "/api/v1/auth/me", nil)
	edID := edMe["id"]
	if edID == nil {
		edID = edMe["user"].(map[string]any)["id"]
	}
	when := "2021-05-06T07:08:09Z"
	code, p := post(admin, "/mk", map[string]any{"title": "Full", "status": "publish", "authorId": edID, "publishedAt": when,
		"imageId": 901, "pinned": true, "sortOrder": 4})
	if code != 200 || p["authorId"] != edID || p["publishedAt"] != when || p["imageId"] != 901.0 || p["pinned"] != true || p["sortOrder"] != 4.0 || p["status"] != "publish" {
		t.Fatalf("create with every field: %d %v", code, p)
	}
	// a future publishedAt from code schedules; imageId null clears
	fut := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if code, u := post(admin, "/upd/"+id(p), map[string]any{"publishedAt": fut, "imageId": nil}); code != 200 || u["status"] != "scheduled" || u["imageId"] != nil {
		t.Fatalf("update schedule: %d %v", code, u)
	}
	// rights: an author may not reassign or pin; an unknown image is refused
	au := user("Au", "author")
	for name, body := range map[string]map[string]any{
		"authorId": {"title": "a", "authorId": adminID},
		"pinned":   {"title": "a", "pinned": true},
		"image":    {"title": "a", "imageId": 777},
		"date":     {"title": "a", "publishedAt": "not a date"},
	} {
		if code, b := post(au, "/mk", body); code != 500 {
			t.Errorf("author %s: %d %v", name, code, b)
		}
	}
	// design: an editor may set a page design; an author may not
	des := map[string]any{"type": "page", "title": "D", "design": map[string]any{"id": "root", "type": "section", "children": []any{}}}
	if code, b := post(ed, "/mk", des); code != 200 {
		t.Fatalf("editor design: %d %v", code, b)
	}
	if code, _ := post(au, "/mk", map[string]any{"title": "D2", "design": des["design"]}); code != 500 {
		t.Fatalf("author design: %d", code)
	}
	// list filters: status, authorId, ids
	_, d := post(admin, "/mk", map[string]any{"title": "Draft one"})
	if code, l := post(admin, "/list", map[string]any{"status": "scheduled"}); code != 200 || l["total"] != 1.0 {
		t.Fatalf("list status: %d %v", code, l)
	}
	if _, l := post(admin, "/list", map[string]any{"status": "any", "authorId": edID}); l["total"] != 1.0 {
		t.Fatalf("list authorId: %v", l)
	}
	if _, l := post(admin, "/list", map[string]any{"status": "any", "ids": []any{p["id"], d["id"], 99999}}); l["total"] != 2.0 {
		t.Fatalf("list ids: %v", l)
	}
	if _, l := post(admin, "/list", map[string]any{"ids": []any{}}); l["total"] != 0.0 {
		t.Fatalf("list empty ids: %v", l)
	}
	// a visitor asking for drafts gets nothing (not an error, not the drafts)
	if code, l := post(c, "/list", map[string]any{"status": "draft"}); code != 200 || l["total"] != 0.0 {
		t.Fatalf("visitor drafts: %d %v", code, l)
	}
	if code, _ := post(admin, "/list", map[string]any{"status": "bogus"}); code != 500 {
		t.Fatalf("bad status: %d", code)
	}
}

// A published post saved again within the same second stays published: the
// database rounds published_at to whole seconds (it used to turn into a
// scheduled post — gone from the site — when the rounding went up).
func TestResaveStaysPublished(t *testing.T) {
	c, _ := newTestAPI(t)
	admin, _ := setupSite(t, c)
	for i := 0; i < 10; i++ {
		_, p := admin.do("POST", "/api/v1/posts", map[string]any{"title": fmt.Sprintf("Quick %d", i), "status": "publish"})
		for j := 0; j < 3; j++ {
			if code, b := admin.do("PUT", "/api/v1/posts/"+id(p), map[string]any{"title": fmt.Sprintf("Quick %d.%d", i, j)}); code != 200 || b["status"] != "publish" {
				t.Fatalf("resave %d.%d: %d status %v", i, j, code, b["status"])
			}
		}
	}
}
