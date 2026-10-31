package sitecode

import (
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/model"
)

func TestSelectAggregateSQL(t *testing.T) {
	q, err := NewQuery(Naming{Prefix: "sc_"}, "hits")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Select("path"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		fn    string
		col   any
		alias string
	}{{"SUM", "views", "v"}, {"COUNT", "*", ""}, {"MAX", "views", "top"}, {"COUNT DISTINCT", "path", "paths"}, {"AVG", "views", ""}} {
		if err := q.SelectAggregate(c.fn, c.col, c.alias); err != nil {
			t.Fatalf("%s: %v", c.fn, err)
		}
	}
	sql, _ := q.SelectSQL(0)
	for _, want := range []string{"(SUM(`views`) + 0e0) AS `v`", "COUNT(*) AS `count`", "MAX(`views`) AS `top`", "COUNT(DISTINCT `path`) AS `paths`", "(AVG(`views`) + 0e0) AS `avg`"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in %s", want, sql)
		}
	}
	for _, bad := range []struct {
		fn, col, alias string
	}{{"SUM", "*", "x"}, {"SUM", "views", "bad alias"}, {"COUNT DISTINCT", "*", "x"}, {"SUM", "views; DROP", "x"}, {"MEDIAN", "views", "x"}} {
		if err := q.SelectAggregate(bad.fn, bad.col, bad.alias); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}

func TestColumnCollation(t *testing.T) {
	c := &ColumnDef{Name: "path", Kind: "string", Length: 255, Collation: "utf8mb4_bin"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if d := c.Definition(); d != "`path` VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL" {
		t.Fatalf("definition: %s", d)
	}
	if scol(c).Collation != "utf8mb4_bin" {
		t.Fatal("snapshot collation")
	}
	if err := (&ColumnDef{Name: "n", Kind: "integer", Collation: "utf8mb4_bin"}).Check(); err == nil {
		t.Fatal("collation on an integer")
	}
	if err := (&ColumnDef{Name: "s", Kind: "string", Length: 10, Collation: "latin1_swedish_ci"}).Check(); err == nil {
		t.Fatal("unknown collation")
	}
}

func TestNoticeHelpers(t *testing.T) {
	for in, want := range map[string]bool{"/admin/x": true, "/?bypass=1": true, "https://x.io/a": true, "//evil.io": false,
		"javascript:alert(1)": false, `/\evil`: false, "http://a b": false, "": false} {
		if _, _, ok := noticeHref(in); ok != want {
			t.Errorf("noticeHref(%q) = %v", in, ok)
		}
	}
	if _, ext, _ := noticeHref("https://x.io"); !ext {
		t.Error("external")
	}
	if localized(map[string]any{"th": "ไทย", "en": "Eng"}, "en") != "Eng" || localized(map[string]any{"th": "ไทย"}, "en") != "ไทย" || localized("x", "th") != "x" {
		t.Error("localized")
	}
	if !metaKeyOK("seo.title") || !metaKeyOK("title") || metaKeyOK("a.title") || metaKeyOK("seo.ti-tle") || metaKeyOK("seo.x.y") {
		t.Error("metaKeyOK")
	}
}

// slowPosts is a post store whose reads take longer than a render-path hook may run.
type slowPosts struct {
	delay time.Duration
	reads atomic.Int32
	post  model.Post
}

func (s *slowPosts) ListPosts(model.PostFilter) ([]*model.Post, int, error) { return nil, 0, nil }
func (s *slowPosts) PostBySlug(string, string, bool) (*model.Post, error)   { return nil, nil }
func (s *slowPosts) GetPost(id int64) (*model.Post, error) {
	s.reads.Add(1)
	time.Sleep(s.delay)
	if id != s.post.ID {
		return nil, nil
	}
	p := s.post
	return &p, nil
}

// A page.meta filter that reads the page's own fields with site.meta(page.id) (what the official
// seo plugin does) must not lose its result to the 50 ms render limit because the database is slow:
// core reads the page's post ahead (Session.PreloadPost), outside the limit. Before, a read that took
// longer than 50 ms made the filter be skipped and the SEO title, canonical and noindex vanish.
func TestPreloadPostForRenderHooks(t *testing.T) {
	e := phase3Engine()
	e.FilterTimeout, e.FilterRenderTimeout = 2*time.Second, 50*time.Millisecond
	posts := &slowPosts{delay: 120 * time.Millisecond, post: model.Post{ID: 7, Type: "page", Meta: map[string]any{"seo_title": "Custom"}}}
	e.h.posts = posts
	p := mustCompile(t, map[string]string{"main.ts": `
site.fields('page', { seo_title: { type: 'text' } })
site.filter('page.meta', (m, { page }) => {
  const own = site.meta(page.id)
  return own && own.seo_title ? { ...m, title: own.seo_title, noindex: true } : m
})
`})
	val := func() map[string]any {
		return map[string]any{"title": "About", "fullTitle": "About – S", "noindex": false, "og": map[string]any{}}
	}
	ctx := map[string]any{"page": map[string]any{"id": 7, "path": "/about"}, "path": "/about"}
	newSess := func() *Session {
		return &Session{e: e, prog: p, r: httptest.NewRequest("GET", "https://example.com/about", nil)}
	}

	// without the read-ahead the slow read inside the hook still hits the limit (the limit is kept)
	s := newSess()
	if out := s.ApplyFilter("page.meta", val(), ctx).(map[string]any); out["title"] != "About" {
		t.Fatalf("un-preloaded slow read: %v", out)
	}
	s.Close()

	// read ahead: the hook gets the post from memory, no database read inside the limit
	s = newSess()
	defer s.Close()
	s.PreloadPost(7)
	s.PreloadPost(7) // once per request
	s.PreloadPost(99999)
	posts.reads.Store(0)
	out := s.ApplyFilter("page.meta", val(), ctx).(map[string]any)
	if out["title"] != "Custom" || out["noindex"] != true {
		t.Fatalf("preloaded page.meta: %v (logs %v)", out, s.logs)
	}
	if n := posts.reads.Load(); n != 0 {
		t.Fatalf("site.meta read the database %d times inside the hook", n)
	}
	// a write by the session drops the read-ahead copy (the next site.meta reads the store)
	s.forgetPost(7)
	if s.preloaded(7) != nil {
		t.Fatal("forgetPost kept the post")
	}
}
