package site

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
)

// Finding content (2.5.0): an RSS feed of the latest posts (/feed, /rss.xml),
// a search page for every public type (/search?q=), the posts of an author
// (/author/<id>), and /…/page/N addresses that open page N of a list.

var pagedPathRe = regexp.MustCompile(`^(.*)/page/([0-9]{1,6})$`)

// serveDiscover handles those addresses; false = not one of them.
func (s *Site) serveDiscover(w http.ResponseWriter, r *http.Request, opts map[string]string, path string, types []PostTypeRoute) bool {
	if m := pagedPathRe.FindStringSubmatch(path); m != nil { // /blog/page/2 → /blog?page=2
		q := r.URL.Query()
		q.Set("page", m[2])
		w.Header().Set("Location", "/"+m[1]+"?"+q.Encode())
		w.WriteHeader(http.StatusMovedPermanently)
		return true
	}
	switch {
	case path == "feed" || path == "rss.xml" || path == "feed.xml" || path == getBases().blog+"/feed":
		s.feed(w, r, opts)
	case path == "search":
		if p, err := s.st.PostBySlug(model.TypePage, "search", true); err == nil {
			s.renderPost(w, opts, p, nil, "", http.StatusOK) // the owner's own search page wins
			return true
		}
		s.searchPage(w, r, opts, types)
	case strings.HasPrefix(path, "author/") && !strings.Contains(path[len("author/"):], "/"):
		s.authorArchive(w, opts, path[len("author/"):])
	default:
		return false
	}
	return true
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	PubDate     string   `xml:"pubDate"`
	Description string   `xml:"description"`
	Categories  []string `xml:"category"`
}

// feed answers the RSS 2.0 feed of the 20 latest published posts.
func (s *Site) feed(w http.ResponseWriter, r *http.Request, opts map[string]string) {
	base := BaseURL(opts, r)
	posts, _, err := s.st.ListPosts(model.PostFilter{Type: model.TypePost, Status: model.StatusPublish, Limit: 20, Order: "newest", Lang: pageLang(opts)})
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	type channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Language    string    `xml:"language"`
		Generator   string    `xml:"generator"`
		Items       []rssItem `xml:"item"`
	}
	ch := channel{Title: opts["site_title"], Link: base + "/", Description: opts["tagline"], Language: orStr(opts["language"], "th"), Generator: "Anasaki Studio"}
	for _, p := range posts {
		ctx := PostContext(p)
		date := p.CreatedAt
		if p.PublishedAt != nil {
			date = *p.PublishedAt
		}
		it := rssItem{Title: p.Title, Link: base + ctx["url"], GUID: base + ctx["url"], PubDate: date.UTC().Format(time.RFC1123Z), Description: ctx["excerpt"]}
		for _, c := range p.Categories {
			it.Categories = append(it.Categories, c.Name)
		}
		ch.Items = append(ch.Items, it)
	}
	out, err := xml.MarshalIndent(struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Channel channel  `xml:"channel"`
	}{Version: "2.0", Channel: ch}, "", " ")
	if err != nil {
		http.Error(w, "feed error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(out)
}

func orStr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// searchPage lists the published posts, pages and public custom posts matching ?q= in the site's layout: a
// search box and the results (a post list the box filters), 10 a page.
func (s *Site) searchPage(w http.ResponseWriter, r *http.Request, opts map[string]string, types []PostTypeRoute) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if r := []rune(q); len(r) > 100 {
		q = string(r[:100])
	}
	en := opts["language"] == "en"
	tr := func(th, e string) string {
		if en {
			return e
		}
		return th
	}
	searchable := []string{model.TypePost, model.TypePage}
	for _, t := range types {
		if t.Public {
			searchable = append(searchable, t.Type)
		}
	}
	title := tr("ค้นหา", "Search")
	sub := tr("พิมพ์คำที่ต้องการค้นหา", "Type what you are looking for")
	if q != "" {
		title = fmt.Sprintf(tr("ผลการค้นหา “%s”", "Results for “%s”"), q)
		sub = ""
	}
	root := s.searchDesign(tr("ค้นหาในเว็บนี้…", "Search this site…"), tr("ค้นหา", "Search"), tr("ไม่พบรายการที่ตรงกับคำค้น", "Nothing matches your search"))
	raw, err := json.Marshal(root)
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	tpl := &model.Post{Type: model.TypeTemplate, Design: raw}
	ctx := design.PostCtx{"title": title, "excerpt": sub, "url": "/search", "archiveSearch": q, "archiveTypes": strings.Join(searchable, ",")}
	w.Header().Set("X-Robots-Tag", "noindex") // result pages are not for search engines
	s.renderPost(w, opts, tpl, ctx, "", http.StatusOK, meta{title: title, postType: "archive"})
}

// searchDesign is the search page: the site header, a heading, a search box (a GET form, so it works without
// JavaScript) and the result list with page links, the site footer.
func (s *Site) searchDesign(placeholder, button, empty string) *design.Node {
	header, footer := HeaderFooter(s.st)
	fill, hug := design.Size{Mode: "fill"}, design.Size{Mode: "hug"}
	card := &design.Node{ID: "sr-card", Type: design.TypeFrame, Width: fill, Height: hug,
		Layout: map[string]any{"mode": "vertical", "gap": 6.0, "padding": []any{18.0, 20.0, 18.0, 20.0}},
		Style:  map[string]any{"fill": "#ffffff", "radius": 14.0, "stroke": "#e5e7eb", "strokeWidth": 1.0},
		Children: []*design.Node{
			{ID: "sr-title", Type: design.TypeText, Width: fill, Height: hug, Style: map[string]any{"fontSize": 19.0, "fontWeight": 700.0},
				Props: map[string]any{"text": "{{post.title}}", "tag": "h2", "href": "{{post.url}}"}},
			{ID: "sr-excerpt", Type: design.TypeText, Width: fill, Height: hug, Style: map[string]any{"fontSize": 15.0, "color": "#6b7280"},
				Props: map[string]any{"text": "{{post.excerpt}}"}},
		}}
	list := &design.Node{ID: "results", Type: design.TypePostList, Width: fill, Height: hug,
		Layout: map[string]any{"mode": "vertical", "gap": 12.0}, Style: map[string]any{"maxWidth": 760.0},
		Props: map[string]any{"query": map[string]any{"postType": "current", "limit": 10.0}, "emptyText": empty}, Children: []*design.Node{card}}
	box := &design.Node{ID: "sr-box", Type: design.TypeInput, Width: fill, Height: hug, Style: map[string]any{"maxWidth": 760.0, "fontSize": 17.0},
		Props: map[string]any{"kind": "search", "name": "q", "placeholder": placeholder, "a11yLabel": button, "value": "query.q"}}
	section := &design.Node{ID: "sr-section", Type: design.TypeFrame, Width: fill, Height: hug,
		Layout: map[string]any{"mode": "vertical", "align": "center", "gap": 20.0, "padding": []any{64.0, 24.0, 24.0, 24.0}},
		Children: []*design.Node{
			{ID: "sr-h1", Type: design.TypeText, Width: fill, Height: hug, Style: map[string]any{"fontSize": 38.0, "fontWeight": 800.0, "textAlign": "center", "maxWidth": 760.0},
				Props: map[string]any{"text": "{{post.title}}", "tag": "h1"}},
			{ID: "sr-sub", Type: design.TypeText, Width: fill, Height: hug, Style: map[string]any{"fontSize": 16.0, "color": "#6b7280", "textAlign": "center"},
				Props: map[string]any{"text": "{{post.excerpt}}"}},
			{ID: "sr-form", Type: design.TypeFrame, Width: fill, Height: hug, Layout: map[string]any{"mode": "horizontal", "gap": 8.0, "justify": "center"},
				Props: map[string]any{"searchForm": true}, Children: []*design.Node{box}},
			list,
			{ID: "sr-pages", Type: design.TypePagination, Width: fill, Height: hug, Props: map[string]any{"collection": "results", "style": "numbers"}},
		}}
	root := &design.Node{ID: "root", Type: design.TypeFrame, Width: fill, Height: hug, Layout: map[string]any{"mode": "vertical"}, Style: map[string]any{"fill": "#ffffff"}}
	if header > 0 {
		root.Children = append(root.Children, &design.Node{ID: "sr-header", Type: design.TypeInstance, Width: fill, Height: hug, Props: map[string]any{"componentId": float64(header)}})
	}
	root.Children = append(root.Children, section)
	if footer > 0 {
		root.Children = append(root.Children, &design.Node{ID: "sr-footer", Type: design.TypeInstance, Width: fill, Height: hug, Props: map[string]any{"componentId": float64(footer)}})
	}
	return root
}

// authorArchive lists the published posts of user id at /author/<id> (404 without any).
func (s *Site) authorArchive(w http.ResponseWriter, opts map[string]string, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		s.notFound(w, opts)
		return
	}
	u, err := s.st.UserByID(id)
	if err != nil {
		s.notFound(w, opts)
		return
	}
	posts, total, err := s.st.ListPosts(model.PostFilter{Type: model.TypePost, Status: model.StatusPublish, AuthorID: id, Limit: 1, NoContent: true, Lang: pageLang(opts)})
	if err != nil || total == 0 || len(posts) == 0 {
		s.notFound(w, opts) // no public trace of accounts without published posts
		return
	}
	name := u.Name
	m := meta{title: name, description: u.Bio, postType: "archive", image: u.Avatar}
	ctx := design.PostCtx{"title": name, "excerpt": u.Bio, "url": "/author/" + idStr, "image": u.Avatar, "archiveAuthor": idStr}
	if tpl := s.template(opts["archive_template_id"]); tpl != nil {
		s.renderPost(w, opts, tpl, ctx, "", http.StatusOK, m)
		return
	}
	list, _, _ := s.st.ListPosts(model.PostFilter{Type: model.TypePost, Status: model.StatusPublish, AuthorID: id, Limit: 50, NoContent: true, Lang: pageLang(opts)})
	b := `<h1>` + html.EscapeString(name) + `</h1><ul>`
	for _, p := range list {
		b += `<li><a href="` + html.EscapeString(PostURL(p)) + `">` + html.EscapeString(p.Title) + `</a></li>`
	}
	s.writeFallback(w, opts, m, b+`</ul>`, http.StatusOK)
}

// AuthorURL is the address of a user's posts.
func AuthorURL(id int64) string { return "/author/" + url.PathEscape(strconv.FormatInt(id, 10)) }
