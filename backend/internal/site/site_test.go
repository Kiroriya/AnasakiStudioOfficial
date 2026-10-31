package site

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
)

type fakePlugins struct{ seen plugin.PageInfo }

func (f *fakePlugins) ServePublic(http.ResponseWriter, *http.Request) bool { return false }
func (f *fakePlugins) Collection(string, map[string]any) []design.PostCtx  { return nil }
func (f *fakePlugins) RenderBlock(design.BlockCall) (string, string, bool) { return "", "", false }
func (f *fakePlugins) FilterPage(p *plugin.PageInfo)                       { p.FullTitle = "SEO: " + p.Title }
func (f *fakePlugins) HeadHTML(p plugin.PageInfo) string                   { f.seen = p; return `<meta name="x-hook">` }
func (f *fakePlugins) BodyEndHTML(p plugin.PageInfo) string                { return `<div id="body-end"></div>` }

func TestPermalinkBases(t *testing.T) {
	defer SetBases(map[string]string{})
	post := &model.Post{Type: model.TypePost, Slug: "hello"}
	page := &model.Post{Type: model.TypePage, Slug: "about"}
	SetBases(map[string]string{})
	if PostURL(post) != "/blog/hello" || PostURL(page) != "/about" || CategoryURL("news") != "/category/news" {
		t.Errorf("defaults: %s %s %s", PostURL(post), PostURL(page), CategoryURL("news"))
	}
	SetBases(map[string]string{"blog_base": "/News/", "category_base": "topics"})
	if PostURL(post) != "/news/hello" || CategoryURL("go") != "/topics/go" {
		t.Errorf("custom: %s %s", PostURL(post), CategoryURL("go"))
	}
	SetBases(map[string]string{"blog_base": "../x", "category_base": "a b"})
	if BlogBase() != "blog" || CategoryBase() != "category" {
		t.Errorf("invalid bases must fall back: %s %s", BlogBase(), CategoryBase())
	}
}

func TestDocumentHead(t *testing.T) {
	fp := &fakePlugins{}
	s := &Site{plugins: fp}
	opts := map[string]string{"site_title": "My Site", "site_icon": "/uploads/icon.png", "search_visibility": "0"}
	page := s.document(Doc{Title: "About", Options: opts, Path: "/about", BaseURL: "https://x.io", PostID: 5,
		PostType: "page"}, "<main>hi</main>", "", nil)
	for _, want := range []string{
		`<title>SEO: About</title>`, `<meta name="robots" content="noindex,nofollow">`,
		`<meta name="generator" content="Anasaki Studio">`, `<link rel="icon" href="/uploads/icon.png">`,
		`<link rel="apple-touch-icon" href="/uploads/icon.png">`, `<meta name="x-hook"></head>`,
		`<main>hi</main><div id="body-end"></div></body>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q in\n%s", want, page)
		}
	}
	if fp.seen.Path != "/about" || fp.seen.PostID != 5 || fp.seen.Status != 200 || fp.seen.Site["title"] != "My Site" ||
		fp.seen.AbsURL("/about") != "https://x.io/about" {
		t.Errorf("page info: %+v", fp.seen)
	}
	// visible site, unsafe icon, no plugins
	page = (&Site{}).document(Doc{Title: "My Site", Options: map[string]string{"site_title": "My Site",
		"site_icon": "javascript:alert(1)"}}, "", "", nil)
	if strings.Contains(page, "robots") || strings.Contains(page, "javascript:") || !strings.Contains(page, "<title>My Site</title>") {
		t.Errorf("plain page:\n%s", page)
	}
}

func TestFormatDate(t *testing.T) {
	defer SetBases(map[string]string{})
	d := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	SetBases(map[string]string{"language": "th"})
	if got := FormatDate(d); got != "17 ต.ค. 2569" {
		t.Errorf("thai: %q", got)
	}
	SetBases(map[string]string{"language": "en", "date_format": "January 2, 2006"})
	if got := FormatDate(d); got != "October 17, 2026" {
		t.Errorf("english: %q", got)
	}
	SetBases(map[string]string{"language": "th", "date_format": "2 January 2006", "timezone": "Asia/Bangkok"})
	if got := FormatDate(time.Date(2026, 12, 31, 20, 0, 0, 0, time.UTC)); got != "1 มกราคม 2570" {
		t.Errorf("timezone: %q", got)
	}
}
