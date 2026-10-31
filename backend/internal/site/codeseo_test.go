package site

import (
	"strconv"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// metaSession is a code session with a page.meta filter that records the
// read-ahead of the page's post.
type metaSession struct {
	design.CodeSession
	log []string
}

func (m *metaSession) PreloadPost(id int64) {
	m.log = append(m.log, "preload "+strconv.FormatInt(id, 10))
}
func (m *metaSession) HasFilter(name string) bool { return name == "page.meta" }
func (m *metaSession) ApplyFilter(name string, value any, _ map[string]any) any {
	m.log = append(m.log, "filter "+name)
	return value
}

// The page's post is read ahead before page.meta runs (outside the hook's 50 ms), so a plugin's
// site.meta(page.id) does not spend the limit on a database round trip.
func TestFilterMetaPreloadsPagePost(t *testing.T) {
	sess := &metaSession{}
	pi := plugin.PageInfo{Title: "About", FullTitle: "About – S"}
	filterMeta(sess, Doc{Path: "/about", PostID: 42}, &pi)
	if strings.Join(sess.log, ",") != "preload 42,filter page.meta" {
		t.Fatalf("calls: %v", sess.log)
	}
}

func TestSitemapXML(t *testing.T) {
	b, err := SitemapXML("https://x.io/", []any{"/a", map[string]any{"loc": "/b", "lastmod": "2026-01-02", "changefreq": "daily", "priority": 0.8},
		"https://x.io/c", "https://evil.io/d", "//evil.io/e", "javascript:alert(1)", map[string]any{"loc": "/f", "changefreq": "sometimes", "priority": 3.0}, 42})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"<loc>https://x.io/a</loc>", "<loc>https://x.io/b</loc>", "<lastmod>2026-01-02</lastmod>", "<changefreq>daily</changefreq>",
		"<priority>0.8</priority>", "<loc>https://x.io/c</loc>", "<loc>https://x.io/f</loc>"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s:\n%s", want, s)
		}
	}
	for _, bad := range []string{"evil", "javascript", "sometimes", "<priority>3"} {
		if strings.Contains(s, bad) {
			t.Fatalf("has %s:\n%s", bad, s)
		}
	}
	if strings.Count(s, "<url>") != 4 {
		t.Fatalf("urls:\n%s", s)
	}
}

func TestHeadExtraHTML(t *testing.T) {
	x := headExtra{Canonical: "https://x.io/a?b=1&c=2", OG: map[string]string{"og:title": `A "B"`, "twitter:card": "summary"}}
	if got := x.html(); got != `<link rel="canonical" href="https://x.io/a?b=1&amp;c=2"><meta property="og:title" content="A &#34;B&#34;"><meta name="twitter:card" content="summary">` {
		t.Fatal(got)
	}
}
