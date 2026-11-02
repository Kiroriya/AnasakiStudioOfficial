package site

import (
	"database/sql"
	"encoding/xml"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// Phase 6 (G7): Site Code / JS plugin joints on the public pages —
// site.head / site.bodyEnd HTML, the page.meta filter (title, description,
// canonical, robots, og:*), and /sitemap.xml + /robots.txt served by core
// when a unit filters sitemap.urls / robots.txt (the official seo plugin
// does).

// pageHTMLer is implemented by a code session with site.head / site.bodyEnd.
type pageHTMLer interface {
	HasPageHTML(kind string) bool
	PageHTML(kind string, page map[string]any) string
}

// postPreloader is a code session that reads the page's post ahead of the
// render-path hooks, outside their 50 ms limit, so site.meta(page.id) inside
// page.meta / site.head is answered from memory (sitecode.Session).
type postPreloader interface{ PreloadPost(id int64) }

func preloadPost(sess design.CodeSession, id int64) {
	if pp, ok := sess.(postPreloader); ok && pp != nil && id > 0 {
		pp.PreloadPost(id)
	}
}

// headExtra is what page.meta adds to <head>.
type headExtra struct {
	Canonical string
	Robots    string
	OG        map[string]string // "og:title" → content; "twitter:*" use name=
}

var ogKeyRe = regexp.MustCompile(`^(og|twitter|article|profile|book|fb|music|video):[a-z][a-z0-9_:]{0,60}$`)

func (x headExtra) html() string {
	var b strings.Builder
	if x.Canonical != "" {
		b.WriteString(`<link rel="canonical" href="` + html.EscapeString(x.Canonical) + `">`)
	}
	keys := make([]string, 0, len(x.OG))
	for k := range x.OG {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		attr := "property"
		if strings.HasPrefix(k, "twitter:") {
			attr = "name"
		}
		b.WriteString(`<meta ` + attr + `="` + k + `" content="` + html.EscapeString(x.OG[k]) + `">`)
	}
	return b.String()
}

func safeMetaURL(u string) bool {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") {
		return true
	}
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "https" || p.Scheme == "http") && p.Host != ""
}

// pageObject is the page a site.head / site.bodyEnd handler and the
// page.meta filter see.
func pageObject(d Doc, pi plugin.PageInfo) map[string]any {
	return pageView(d, pi.Title, pi.Description, pi.Image, pi.Status)
}

// pageView builds the page object from the document and its hook info.
func pageView(d Doc, title, description, image string, status int) map[string]any {
	archive := d.Post["archiveType"]
	if archive == "" && d.PostType == "archive" {
		archive = "category"
		if d.Category == "" {
			archive = "blog"
		}
	}
	abs := func(u string) string {
		if u == "" || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			return u
		}
		if !strings.HasPrefix(u, "/") {
			u = "/" + u
		}
		return strings.TrimRight(d.BaseURL, "/") + u
	}
	typ := d.PostType
	if typ == "" {
		typ = "plugin"
	}
	if status == 0 {
		status = http.StatusOK
	}
	return map[string]any{
		"type": typ, "id": d.PostID, "slug": d.Post["slug"], "title": title, "description": description,
		"url": abs(d.Path), "path": d.Path, "image": abs(image), "lang": orDefault(d.Options["language"], "th"),
		"isHome": d.Post["isHome"] == "1" || d.Path == "/", "archiveType": archive, "status": status,
		"siteTitle": d.Options["site_title"], "siteTagline": d.Options["tagline"],
	}
}

// filterMeta runs the page.meta filters over the head values.
// Value: {title, fullTitle, description, image, canonical, robots, noindex,
// og: {"og:type": …, "twitter:card": …}}; ctx: {page, path}.
func filterMeta(sess design.CodeSession, d Doc, pi *plugin.PageInfo) headExtra {
	var x headExtra
	f, ok := sess.(design.Filterer)
	if !ok || f == nil || !f.HasFilter("page.meta") {
		return x
	}
	preloadPost(sess, d.PostID)
	val := map[string]any{"title": pi.Title, "fullTitle": pi.FullTitle, "description": pi.Description, "image": pi.Image,
		"canonical": "", "robots": "", "noindex": pi.NoIndex, "og": map[string]any{}}
	out, ok := f.ApplyFilter("page.meta", val, map[string]any{"page": pageObject(d, *pi), "path": d.Path}).(map[string]any)
	if !ok {
		return x
	}
	str := func(k string, max int) (string, bool) {
		s, ok := out[k].(string)
		if !ok || len(s) > max || strings.ContainsAny(s, "\r\n") {
			return "", false
		}
		return s, true
	}
	if v, ok := str("title", 500); ok {
		pi.Title = v
	}
	if v, ok := str("fullTitle", 500); ok {
		pi.FullTitle = v
	}
	if v, ok := out["description"].(string); ok && len(v) <= 2000 {
		pi.Description = v
	}
	if v, ok := str("image", 2000); ok && (v == "" || safeMetaURL(v)) {
		pi.Image = v
	}
	if v, ok := out["noindex"].(bool); ok {
		pi.NoIndex = v
	}
	if v, ok := str("canonical", 2000); ok && v != "" && safeMetaURL(v) {
		if strings.HasPrefix(v, "/") {
			v = strings.TrimRight(d.BaseURL, "/") + v
		}
		x.Canonical = v
	}
	if v, ok := str("robots", 200); ok {
		x.Robots = v
	}
	if og, ok := out["og"].(map[string]any); ok {
		x.OG = map[string]string{}
		for k, v := range og {
			s := design.Stringify(v)
			if ogKeyRe.MatchString(k) && s != "" && len(s) <= 2000 && len(x.OG) < 50 {
				x.OG[k] = s
			}
		}
	}
	return x
}

// serveCodeSEO answers /sitemap.xml and /robots.txt when a code unit filters
// sitemap.urls / robots.txt (false otherwise: normal routing continues).
func (s *Site) serveCodeSEO(w http.ResponseWriter, r *http.Request, opts map[string]string, path string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	cs := s.session(r, opts)
	if cs == nil {
		return false
	}
	defer cs.Close()
	f, ok := cs.(design.Filterer)
	if !ok || f == nil || !(f.HasFilter("sitemap.urls") || f.HasFilter("robots.txt")) {
		return false
	}
	base := BaseURL(opts, r)
	visible := opts["search_visibility"] != "0"
	if path == "robots.txt" {
		txt := "User-agent: *\nDisallow: /\n"
		if visible {
			txt = "User-agent: *\nDisallow: /api/\n\nSitemap: " + strings.TrimRight(base, "/") + "/sitemap.xml\n"
		}
		if f.HasFilter("robots.txt") {
			if out, ok := f.ApplyFilter("robots.txt", txt, map[string]any{"baseUrl": base, "visible": visible}).(string); ok {
				txt = out
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=600")
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(txt))
		}
		return true
	}
	if !f.HasFilter("sitemap.urls") {
		return false
	}
	ckey := s.sitemapKey(cs, base, visible, opts)
	if b, ok := s.sitemap.get(ckey); ok {
		writeSitemap(w, r, b)
		return true
	}
	var entries []any
	if visible {
		for _, e := range s.sitemapEntries(opts) {
			entries = append(entries, e)
		}
	}
	if entries == nil {
		entries = []any{}
	}
	out := f.ApplyFilter("sitemap.urls", entries, map[string]any{"baseUrl": base, "visible": visible})
	list, _ := out.([]any)
	if !visible {
		list = nil
	}
	b, err := SitemapXML(base, list)
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return true
	}
	s.sitemap.put(ckey, b)
	writeSitemap(w, r, b)
	return true
}

func writeSitemap(w http.ResponseWriter, r *http.Request, b []byte) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	if r.Method != http.MethodHead {
		_, _ = w.Write(b)
	}
}

// ---------------------------------------------------------------- sitemap cache (phase 7)

// SitemapTTL is how long a rendered /sitemap.xml is reused while nothing
// it depends on changed (tests shorten it).
var SitemapTTL = 5 * time.Minute

type sitemapEntry struct {
	key string
	xml []byte
	at  time.Time
}

type sitemapStore struct {
	mu sync.Mutex
	e  *sitemapEntry
}

func (c *sitemapStore) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.e == nil || c.e.key != key || time.Since(c.e.at) > SitemapTTL {
		return nil, false
	}
	return c.e.xml, true
}

func (c *sitemapStore) put(key string, b []byte) {
	c.mu.Lock()
	c.e = &sitemapEntry{key: key, xml: b, at: time.Now()}
	c.mu.Unlock()
}

// fingerprinter is implemented by a code session (sitecode.Session).
type fingerprinter interface {
	FilterFingerprint(name string) string
}

// sitemapKey changes whenever the sitemap may change: published content and
// categories, the options it reads, and the units filtering sitemap.urls
// (their versions, settings and the data they wrote).
func (s *Site) sitemapKey(cs design.CodeSession, base string, visible bool, opts map[string]string) string {
	var b strings.Builder
	b.WriteString(base + "|" + strconv.FormatBool(visible) + "|" + opts["home_page_id"] + "|" + opts["blog_base"] + "|" + opts["category_base"] + "|")
	if s.st != nil {
		var n, terms int64
		var last sql.NullString
		if err := s.st.DB.QueryRow(`SELECT COUNT(*), CAST(MAX(updated_at) AS CHAR) FROM posts WHERE status = 'publish'`).Scan(&n, &last); err == nil {
			b.WriteString(strconv.FormatInt(n, 10) + "@" + last.String + "|")
		} else {
			b.WriteString(strconv.FormatInt(time.Now().UnixNano(), 10)) // no fingerprint: no reuse
		}
		if err := s.st.DB.QueryRow(`SELECT COUNT(*) FROM post_terms`).Scan(&terms); err == nil {
			b.WriteString(strconv.FormatInt(terms, 10) + "|")
		}
	}
	if fp, ok := cs.(fingerprinter); ok {
		b.WriteString(fp.FilterFingerprint("sitemap.urls"))
	}
	return b.String()
}

// sitemapURL is one <url> of a sitemap.
type sitemapURL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq,omitempty"`
	Priority   string `xml:"priority,omitempty"`
}

type sitemapSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

var changeFreqs = map[string]bool{"always": true, "hourly": true, "daily": true, "weekly": true, "monthly": true, "yearly": true, "never": true}

// SitemapXML writes sitemap entries ({loc, lastmod?, changefreq?, priority?}
// objects or plain path / URL strings): relative locations get base, other
// hosts and bad values are dropped; at most 50000 URLs.
func SitemapXML(base string, entries []any) ([]byte, error) {
	set := sitemapSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: []sitemapURL{}}
	base = strings.TrimRight(base, "/")
	for _, e := range entries {
		var u sitemapURL
		switch x := e.(type) {
		case string:
			u.Loc = x
		case map[string]any:
			u.Loc, _ = x["loc"].(string)
			u.LastMod, _ = x["lastmod"].(string)
			if cf, _ := x["changefreq"].(string); changeFreqs[cf] {
				u.ChangeFreq = cf
			}
			switch p := x["priority"].(type) {
			case float64:
				if p >= 0 && p <= 1 {
					u.Priority = strconv.FormatFloat(p, 'f', 1, 64)
				}
			case string:
				if f, err := strconv.ParseFloat(p, 64); err == nil && f >= 0 && f <= 1 {
					u.Priority = strconv.FormatFloat(f, 'f', 1, 64)
				}
			}
		default:
			continue
		}
		u.Loc = strings.TrimSpace(u.Loc)
		switch {
		case strings.HasPrefix(u.Loc, "/") && !strings.HasPrefix(u.Loc, "//"):
			u.Loc = base + u.Loc
		case strings.HasPrefix(u.Loc, base+"/") || u.Loc == base:
		default:
			continue // another host or not a URL
		}
		if len(u.LastMod) > 40 {
			u.LastMod = ""
		}
		set.URLs = append(set.URLs, u)
		if len(set.URLs) >= 50000 {
			break
		}
	}
	b, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), b...), nil
}

// sitemapEntries is core's default list for sitemap.urls: the home page,
// published pages and posts (oldest first), categories with posts.
func (s *Site) sitemapEntries(opts map[string]string) []map[string]any {
	homeID, _ := strconv.ParseInt(opts["home_page_id"], 10, 64)
	out := []map[string]any{{"loc": "/"}}
	types := []string{model.TypePage, model.TypePost}
	for _, t := range s.customTypes(opts) { // 2.0.8: public custom types (Site Code / plugins): archive + posts
		if t.Public && t.Slug != "" {
			out = append(out, map[string]any{"loc": "/" + t.Slug, "type": "archive"})
			types = append(types, t.Type)
		}
	}
	for _, typ := range types {
		for off := 0; off < 20000; off += 200 {
			posts, _, err := s.st.ListPosts(model.PostFilter{Type: typ, Status: model.StatusPublish, Order: "oldest", NoContent: true,
				Limit: 200, Offset: off})
			if err != nil {
				return out
			}
			for _, p := range posts {
				if typ == model.TypePage && (p.Slug == "404" || p.ID == homeID || (homeID == 0 && p.Slug == "home")) {
					continue
				}
				out = append(out, map[string]any{"loc": PostURL(p), "lastmod": p.UpdatedAt.UTC().Format("2006-01-02"),
					"id": p.ID, "type": p.Type})
			}
			if len(posts) < 200 {
				break
			}
		}
	}
	if terms, err := s.st.ListTerms("category"); err == nil {
		for _, t := range terms {
			if t.Count > 0 {
				out = append(out, map[string]any{"loc": CategoryURL(t.Slug), "type": "category"})
			}
		}
	}
	return out
}
