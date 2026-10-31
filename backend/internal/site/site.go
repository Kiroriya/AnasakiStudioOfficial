// Package site serves the public website: it resolves URLs the way WordPress'
// template hierarchy does (front page → page → single post → category
// archive → 404) and renders the matching design with live CMS data.
package site

import (
	"encoding/json"
	"errors"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/store"
)

// Generator is the <meta name="generator"> value.
const Generator = "Anasaki Studio"

// Plugins is what the site needs from the plugin manager (active plugins
// only): public routes, page hooks, blocks and collection data.
type Plugins interface {
	ServePublic(w http.ResponseWriter, r *http.Request) bool
	Collection(source string, query map[string]any) []design.PostCtx
	FilterPage(p *plugin.PageInfo)
	HeadHTML(p plugin.PageInfo) string
	BodyEndHTML(p plugin.PageInfo) string
	RenderBlock(c design.BlockCall) (html, css string, ok bool)
}

type Site struct {
	st      *store.Store
	plugins Plugins
	demo    bool // demo mode: show a ribbon inviting visitors into the admin

	// code serves Site Code routes (after plugin public routes, before page
	// routing); false = not handled. nil = none.
	code func(w http.ResponseWriter, r *http.Request, opts map[string]string) bool
	// sessions binds Site Code to a request (collections, page data,
	// formatters, actions); nil or a nil result = no Site Code data.
	sessions func(r *http.Request, opts map[string]string) design.CodeSession
	// postTypes lists Site Code's custom post types (nil = none).
	postTypes func(opts map[string]string) []PostTypeRoute
	// hooks are Site Code's request hooks (site.middleware,
	// site.afterResponse; phase 6). nil = none.
	hooks CodeHooks
	// sitemap is the last rendered /sitemap.xml (phase 7 cache).
	sitemap sitemapStore
}

// CodeHooks are Site Code's per-request hooks (sitecode.Engine).
type CodeHooks interface {
	// Middleware runs site.middleware; true = it answered.
	Middleware(w http.ResponseWriter, r *http.Request, opts map[string]string) bool
	// AfterResponse queues site.afterResponse (never blocks).
	AfterResponse(r *http.Request, status int, ctype string, dur time.Duration, title string, opts map[string]string)
}

// SetCodeHooks installs Site Code's request hooks (phase 6).
func (s *Site) SetCodeHooks(h CodeHooks) { s.hooks = h }

// SetCodeSessions installs the Site Code data provider (sitecode.Engine.Session).
func (s *Site) SetCodeSessions(fn func(r *http.Request, opts map[string]string) design.CodeSession) {
	s.sessions = fn
}

// session returns the request's Site Code session or nil.
func (s *Site) session(r *http.Request, opts map[string]string) design.CodeSession {
	if s.sessions == nil || r == nil {
		return nil
	}
	if cs := s.sessions(r, opts); cs != nil {
		return cs
	}
	return nil
}

// SetCode installs the Site Code route handler (sitecode.Engine.ServePublic).
func (s *Site) SetCode(fn func(w http.ResponseWriter, r *http.Request, opts map[string]string) bool) {
	s.code = fn
}

// SetDemo turns the public "this is a demo" ribbon on or off.
func (s *Site) SetDemo(on bool) { s.demo = on }

// New creates the public site; plugins may be nil.
func New(st *store.Store, plugins Plugins) *Site {
	s := &Site{st: st, plugins: plugins}
	if st != nil {
		if opts, err := st.Options(); err == nil {
			SetBases(opts)
		}
	}
	return s
}

// ---------------------------------------------------------------- permalinks

type bases struct {
	blog, category string
	dateFormat     string         // Go layout from the date_format option
	thai           bool           // site language th: Thai month names, Buddhist year
	loc            *time.Location // timezone option
}

var current atomic.Pointer[bases]

var baseRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

// SetBases reads the permalink bases from options (blog_base, default "blog";
// category_base, default "category"). The site calls it whenever it loads
// the options, so PostURL follows setting changes.
func SetBases(opts map[string]string) {
	b := &bases{blog: "blog", category: "category"}
	if v := strings.Trim(strings.ToLower(opts["blog_base"]), "/ "); baseRe.MatchString(v) {
		b.blog = v
	}
	if v := strings.Trim(strings.ToLower(opts["category_base"]), "/ "); baseRe.MatchString(v) && v != b.blog {
		b.category = v
	}
	b.dateFormat = opts["date_format"]
	b.thai = opts["language"] == "" || opts["language"] == "th"
	if l, err := time.LoadLocation(opts["timezone"]); err == nil && opts["timezone"] != "" {
		b.loc = l
	}
	current.Store(b)
}

func getBases() *bases {
	if b := current.Load(); b != nil {
		return b
	}
	return &bases{blog: "blog", category: "category"}
}

// BlogBase is the URL prefix of blog posts ("blog").
func BlogBase() string { return getBases().blog }

// CategoryBase is the URL prefix of category archives ("category").
func CategoryBase() string { return getBases().category }

// PostURL returns a post's public path.
func PostURL(p *model.Post) string {
	if u, ok := customURL(p); ok {
		return u
	}
	if p.Type == model.TypePost {
		return "/" + BlogBase() + "/" + url.PathEscape(p.Slug)
	}
	return "/" + url.PathEscape(p.Slug)
}

// CategoryURL returns a category archive's public path.
func CategoryURL(slug string) string { return "/" + CategoryBase() + "/" + url.PathEscape(slug) }

// BaseURL returns scheme://host of the site: option site_url when set, else
// derived from the request.
func BaseURL(opts map[string]string, r *http.Request) string {
	if u := strings.TrimRight(strings.TrimSpace(opts["site_url"]), "/"); strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if r == nil {
		return ""
	}
	return plugin.RequestScheme(r) + "://" + plugin.RequestHost(r)
}

// ---------------------------------------------------------------- data source

// Source implements design.DataSource on top of the store, caching per render.
type Source struct {
	st      *store.Store
	plugins Plugins
	comps   map[int64]*design.Node
	menus   map[int64][]design.MenuItem
	media   map[int64]*design.MediaInfo
	code    design.CodeSession // Site Code (nil = code:/table: sources stay empty)
}

// LoadCollection serves code:/table: sources through Site Code (design.Dynamic).
func (s *Source) LoadCollection(req design.CollectionReq) design.CollectionRes {
	if s.code == nil {
		return design.CollectionRes{}
	}
	return s.code.LoadCollection(req)
}

// Format runs a site.formatter (design.Dynamic).
func (s *Source) Format(name string, v any, args []any) (string, bool) {
	if s.code == nil {
		return "", false
	}
	return s.code.Format(name, v, args)
}

func NewSource(st *store.Store, plugins Plugins) *Source {
	return &Source{st: st, plugins: plugins, comps: map[int64]*design.Node{}, menus: map[int64][]design.MenuItem{},
		media: map[int64]*design.MediaInfo{}}
}

// Collection delegates to the active plugins' collection providers.
func (s *Source) Collection(source string, query map[string]any) []design.PostCtx {
	if s.plugins == nil {
		return nil
	}
	return s.plugins.Collection(source, query)
}

// codeBlocker renders a JS plugin's site.block in the request's code session.
type codeBlocker interface {
	CodeBlock(c design.BlockCall) (string, string, bool)
}

// PluginBlock renders a JS plugin's code block in the page's code session,
// else delegates to the plugin block registry.
func (s *Source) PluginBlock(c design.BlockCall) (string, string, bool) {
	if cb, ok := s.code.(codeBlocker); ok && cb != nil {
		if html, css, ok := cb.CodeBlock(c); ok {
			return html, css, true
		}
	}
	if s.plugins == nil {
		return "", "", false
	}
	return s.plugins.RenderBlock(c)
}

// Media looks up an uploaded file (download nodes).
func (s *Source) Media(id int64) *design.MediaInfo {
	if m, ok := s.media[id]; ok {
		return m
	}
	var out *design.MediaInfo
	if m, err := s.st.GetMedia(id); err == nil {
		out = &design.MediaInfo{ID: m.ID, URL: m.URL, Filename: m.Filename, Mime: m.Mime, Size: m.Size}
	}
	s.media[id] = out
	return out
}

func (s *Source) QueryPosts(q design.Query) []design.PostCtx {
	posts, _, err := s.st.ListPosts(model.PostFilter{Type: q.PostType, Status: model.StatusPublish,
		Category: q.Category, Limit: q.Limit, Order: q.Order, PinnedFirst: true})
	if err != nil {
		log.Printf("query posts: %v", err)
		return nil
	}
	out := make([]design.PostCtx, 0, len(posts))
	for _, p := range posts {
		out = append(out, PostContext(p))
	}
	return out
}

func (s *Source) MenuItems(id int64) []design.MenuItem {
	if items, ok := s.menus[id]; ok {
		return items
	}
	var items []design.MenuItem
	if m, err := s.st.GetMenu(id); err == nil {
		_ = json.Unmarshal(m.Items, &items)
	}
	s.menus[id] = items
	return items
}

func (s *Source) Component(id int64) *design.Node {
	if n, ok := s.comps[id]; ok {
		return n
	}
	var n *design.Node
	if c, err := s.st.GetComponent(id); err == nil {
		n, _ = design.Parse(c.Node)
	}
	s.comps[id] = n
	return n
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// PostContext exposes a post to bindings ({{post.title}}, {{post.url}}, …).
func PostContext(p *model.Post) design.PostCtx {
	excerpt := p.Excerpt
	if excerpt == "" {
		plain := strings.Join(strings.Fields(html.UnescapeString(tagRe.ReplaceAllString(p.Content, " "))), " ")
		r := []rune(plain)
		if len(r) > 160 {
			plain = string(r[:160]) + "…"
		}
		excerpt = plain
	}
	date := p.CreatedAt
	if p.PublishedAt != nil {
		date = *p.PublishedAt
	}
	cat, catURL := "", ""
	if len(p.Categories) > 0 {
		cat, catURL = p.Categories[0].Name, CategoryURL(p.Categories[0].Slug)
	}
	pinned := ""
	if p.Pinned {
		pinned = "1"
	}
	ctx := design.PostCtx{
		"id":          strconv.FormatInt(p.ID, 10),
		"title":       p.Title,
		"excerpt":     excerpt,
		"content":     p.Content,
		"date":        FormatDate(date),
		"author":      p.AuthorName,
		"image":       p.FeaturedImage,
		"category":    cat,
		"categoryUrl": catURL,
		"url":         PostURL(p),
		"slug":        p.Slug,
		"type":        p.Type,
		"pinned":      pinned,
	}
	if m := metaJSON(p.Meta); m != "" {
		ctx["meta"] = m // {{post.meta.<key>}}
	}
	return ctx
}

// ---------------------------------------------------------------- handlers

// pageWriter carries request details down to the renderers (also through
// plugin public handlers that call Pages.HTML/Page/Template).
type pageWriter struct {
	http.ResponseWriter
	path  string
	base  string
	req   *http.Request
	title string // the rendered page's title (site.afterResponse)
}

// setTitle remembers the page title of the request (site.afterResponse).
func setTitle(w http.ResponseWriter, title string) {
	if pw := findPageWriter(w); pw != nil {
		pw.title = title
	}
}

func (p *pageWriter) Unwrap() http.ResponseWriter { return p.ResponseWriter }

func findPageWriter(w http.ResponseWriter) *pageWriter {
	for i := 0; i < 8 && w != nil; i++ {
		if pw, ok := w.(*pageWriter); ok {
			return pw
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		w = u.Unwrap()
	}
	return nil
}

func reqInfo(w http.ResponseWriter) (path, base string) {
	if pw := findPageWriter(w); pw != nil {
		return pw.path, pw.base
	}
	return "", ""
}

// reqOf is the public request being answered (nil outside the site handler).
func reqOf(w http.ResponseWriter) *http.Request {
	if pw := findPageWriter(w); pw != nil {
		return pw.req
	}
	return nil
}

func (s *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	opts, err := s.st.Options()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	SetBases(opts)
	if s.hooks != nil { // phase 6: site.afterResponse sees the status; site.middleware runs first
		start := time.Now()
		rec := &plugin.StatusRecorder{ResponseWriter: w}
		w = rec
		pw := &pageWriter{ResponseWriter: w, path: r.URL.Path, base: BaseURL(opts, r), req: r}
		defer func() {
			s.hooks.AfterResponse(r, rec.Code(), rec.Header().Get("Content-Type"), time.Since(start), pw.title, opts)
		}()
		if s.hooks.Middleware(pw, r, opts) {
			return
		}
		w = pw
	}
	pw, ok := w.(*pageWriter)
	if !ok {
		pw = &pageWriter{ResponseWriter: w, path: r.URL.Path, base: BaseURL(opts, r), req: r}
	}
	if s.plugins != nil && s.plugins.ServePublic(pw, r) {
		return
	}
	if s.serveDynamic(pw, r, opts) { // /_c (partial re-render) and /_a (actions)
		return
	}
	if s.code != nil && s.code(pw, r, opts) {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.Trim(r.URL.Path, "/")
	if (path == "sitemap.xml" || path == "robots.txt") && s.serveCodeSEO(pw, r, opts, path) {
		return
	}
	parts := strings.Split(path, "/")
	b := getBases()
	types := s.customTypes(opts)

	switch {
	case len(parts) == 2 && typeBySlug(types, parts[0]) != nil:
		s.singleCustom(pw, opts, typeBySlug(types, parts[0]), parts[1])
	case len(parts) == 1 && typeBySlug(types, parts[0]) != nil:
		if p, err := s.st.PostBySlug(model.TypePage, parts[0], true); err == nil {
			s.renderPost(pw, opts, p, nil, "", http.StatusOK) // a page at /<slug> wins over the archive
		} else {
			s.typeArchive(pw, opts, typeBySlug(types, parts[0]))
		}
	case path == "favicon.ico" || path == "favicon.svg":
		s.favicon(w, r, opts)
	case path == "":
		s.front(pw, opts)
	case len(parts) == 2 && parts[0] == b.blog:
		s.single(pw, opts, parts[1])
	case len(parts) == 2 && parts[0] == b.category:
		s.archive(pw, opts, parts[1])
	case len(parts) == 1 && parts[0] == b.blog:
		if p, err := s.st.PostBySlug(model.TypePage, parts[0], true); err == nil {
			s.renderPost(pw, opts, p, nil, "", http.StatusOK)
		} else {
			s.blogIndex(pw, opts) // no page at /blog yet: list the posts
		}
	case len(parts) == 1:
		s.page(pw, opts, parts[0])
	default:
		s.notFound(pw, opts)
	}
}

func (s *Site) front(w http.ResponseWriter, opts map[string]string) {
	home := func(p *model.Post) {
		ctx := PostContext(p)
		ctx["url"], ctx["isHome"] = "/", "1"
		s.renderPost(w, opts, p, ctx, "", http.StatusOK)
	}
	if id, _ := strconv.ParseInt(opts["home_page_id"], 10, 64); id > 0 {
		if p, err := s.st.GetPost(id); err == nil && p.Status == model.StatusPublish {
			home(p)
			return
		}
	}
	if p, err := s.st.PostBySlug(model.TypePage, "home", true); err == nil {
		home(p)
		return
	}
	// Fresh install: send the owner to the first-run setup instead of a 404.
	if n, err := s.st.CountUsers(); err == nil && n == 0 {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Location", "/"+adminPath(opts)+"/")
		w.WriteHeader(http.StatusFound)
		return
	}
	// Installed but no home page yet: a neutral placeholder (no admin link).
	s.comingSoon(w, opts)
}

func (s *Site) page(w http.ResponseWriter, opts map[string]string, slug string) {
	p, err := s.st.PostBySlug(model.TypePage, slug, true)
	if err != nil {
		s.notFound(w, opts)
		return
	}
	s.renderPost(w, opts, p, nil, "", http.StatusOK)
}

func (s *Site) single(w http.ResponseWriter, opts map[string]string, slug string) {
	p, err := s.st.PostBySlug(model.TypePost, slug, true)
	if err != nil {
		s.notFound(w, opts)
		return
	}
	tpl := s.template(opts["post_template_id"])
	if tpl == nil && len(p.Design) == 0 {
		tpl = s.defaultPostTemplate() // no template yet: header + article + footer
	}
	if tpl == nil {
		s.renderPost(w, opts, p, nil, "", http.StatusOK)
		return
	}
	ctx := PostContext(p)
	s.renderPost(w, opts, tpl, ctx, "", http.StatusOK, meta{title: p.Title, description: ctx["excerpt"],
		image: p.FeaturedImage, postID: p.ID, postType: p.Type})
}

func (s *Site) archive(w http.ResponseWriter, opts map[string]string, slug string) {
	t, err := s.st.TermBySlug("category", slug)
	if err != nil {
		s.notFound(w, opts)
		return
	}
	ctx := design.PostCtx{"title": t.Name, "slug": t.Slug, "url": CategoryURL(t.Slug)}
	m := meta{title: t.Name, postType: "archive"}
	if tpl := s.template(opts["archive_template_id"]); tpl != nil {
		s.renderPost(w, opts, tpl, ctx, slug, http.StatusOK, m)
		return
	}
	posts, _, _ := s.st.ListPosts(model.PostFilter{Type: model.TypePost, Status: model.StatusPublish, Category: slug, Limit: 50, NoContent: true})
	var b strings.Builder
	b.WriteString("<h1>" + html.EscapeString(t.Name) + "</h1><ul>")
	for _, p := range posts {
		b.WriteString(`<li><a href="` + html.EscapeString(PostURL(p)) + `">` + html.EscapeString(p.Title) + `</a></li>`)
	}
	b.WriteString("</ul>")
	s.writeFallback(w, opts, m, b.String(), http.StatusOK)
}

// blogIndex lists published posts at /blog when no page uses that address:
// the archive template if there is one, else a simple list inside the site's
// header and footer.
func (s *Site) blogIndex(w http.ResponseWriter, opts map[string]string) {
	title := "บทความ"
	if opts["language"] == "en" {
		title = "Blog"
	}
	if tpl := s.template(opts["archive_template_id"]); tpl != nil {
		s.renderPost(w, opts, tpl, design.PostCtx{"title": title, "url": "/" + getBases().blog}, "", http.StatusOK, meta{title: title, postType: "archive"})
		return
	}
	posts, _, _ := s.st.ListPosts(model.PostFilter{Type: model.TypePost, Status: model.StatusPublish, Limit: 50, NoContent: true})
	var b strings.Builder
	b.WriteString(`<div style="max-width:880px;margin:0 auto;padding:64px 24px"><h1 style="font-size:40px;font-weight:800;margin-bottom:32px">` + html.EscapeString(title) + `</h1>`)
	if len(posts) == 0 {
		empty := "ยังไม่มีบทความ"
		if opts["language"] == "en" {
			empty = "No posts yet"
		}
		b.WriteString(`<p style="opacity:.6">` + empty + `</p>`)
	}
	for _, p := range posts {
		b.WriteString(`<a href="` + html.EscapeString(PostURL(p)) + `" style="display:block;padding:20px 0;border-bottom:1px solid #e5e7eb;color:inherit;text-decoration:none">` +
			`<h2 style="font-size:22px;font-weight:700">` + html.EscapeString(p.Title) + `</h2>`)
		if p.Excerpt != "" {
			b.WriteString(`<p style="margin-top:6px;color:#6b7280">` + html.EscapeString(p.Excerpt) + `</p>`)
		}
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
	s.HTML(w, http.StatusOK, title, b.String())
}

func (s *Site) notFound(w http.ResponseWriter, opts map[string]string) {
	if p, err := s.st.PostBySlug(model.TypePage, "404", true); err == nil {
		s.renderPost(w, opts, p, nil, "", http.StatusNotFound, meta{title: p.Title, postType: "404"})
		return
	}
	s.writeFallback(w, opts, meta{title: "Not found", postType: "404"},
		"<h1>404</h1><p>ไม่พบหน้าที่คุณต้องการ / Page not found.</p>", http.StatusNotFound)
}

func (s *Site) template(idStr string) *model.Post {
	id, _ := strconv.ParseInt(idStr, 10, 64)
	if id == 0 {
		return nil
	}
	p, err := s.st.GetPost(id)
	if err != nil || len(p.Design) == 0 {
		return nil
	}
	return p
}

// meta describes the page for <head> and plugin hooks.
type meta struct {
	title, description, image string
	postID                    int64
	postType                  string
}

// renderPost renders p's design. ctxPost overrides the binding context (used
// when p is a template rendering another post or an archive).
func (s *Site) renderPost(w http.ResponseWriter, opts map[string]string, p *model.Post, ctxPost design.PostCtx,
	category string, status int, m ...meta) {
	mt := meta{title: p.Title, description: p.Excerpt, image: p.FeaturedImage, postID: p.ID, postType: p.Type}
	if len(m) > 0 {
		mt = m[0]
	}
	if ctxPost == nil {
		ctxPost = PostContext(p)
	}
	root, err := design.Parse(p.Design)
	if err != nil {
		s.writeFallback(w, opts, mt, `<article class="rea-content" style="max-width:760px;margin:48px auto;padding:0 20px"><h1>`+
			html.EscapeString(p.Title)+`</h1>`+p.Content+`</article>`, status)
		return
	}
	path, base := reqInfo(w)
	setTitle(w, mt.title)
	page, cookies, err := s.renderDocument(root, Doc{Title: mt.title, Description: mt.description, Options: opts,
		Post: ctxPost, Category: category, Path: path, BaseURL: base, Image: mt.image, PostID: mt.postID,
		PostType: mt.postType, Status: status, Request: reqOf(w), DesignID: p.ID})
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	for _, c := range cookies {
		w.Header().Add("Set-Cookie", c)
	}
	writeHTML(w, status, page)
}

func (s *Site) writeFallback(w http.ResponseWriter, opts map[string]string, m meta, body string, status int) {
	path, base := reqInfo(w)
	setTitle(w, m.title)
	d := Doc{Title: m.title, Description: m.description, Options: opts, Path: path, BaseURL: base,
		Image: m.image, PostID: m.postID, PostType: m.postType, Status: status, Request: reqOf(w)}
	writeHTML(w, status, s.document(d, `<main style="max-width:760px;margin:48px auto;padding:0 20px">`+body+`</main>`, "", nil))
}

func writeHTML(w http.ResponseWriter, status int, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(page))
}

// ---------------------------------------------------------------- documents

// Doc describes a full HTML page to render.
type Doc struct {
	Title       string
	Description string
	Options     map[string]string
	Post        design.PostCtx
	Category    string
	Head        string // extra <head> HTML; filled from the plugin hooks
	BodyEnd     string // HTML before </body>; filled from the plugin hooks

	// Page details for plugin hooks (all optional).
	Path     string
	BaseURL  string
	Image    string
	PostID   int64
	PostType string
	Status   int

	// Site Code: the request (query, visitor, cookies) and the id of the
	// post holding the design (templates), for partial re-renders.
	Request  *http.Request
	DesignID int64

	code  design.CodeSession // the render's Site Code session (page.head filters)
	extra headExtra          // page.meta results: canonical, robots, og:* (phase 6)
}

// RenderDocument renders a design tree into a complete HTML document.
func (s *Site) RenderDocument(root *design.Node, d Doc) (string, error) {
	page, _, err := s.renderDocument(root, d)
	return page, err
}

// renderDocument also returns the Set-Cookie values the page needs (the
// CSRF cookie of the client runtime, cookies set by site.page).
func (s *Site) renderDocument(root *design.Node, d Doc) (page string, cookies []string, err error) {
	if root == nil {
		return "", nil, errors.New("nil design")
	}
	// short pages: the page fills the screen and the site footer sits at the bottom
	if n := len(root.Children); n > 0 && root.Type == design.TypeFrame && root.Layout["mode"] == "vertical" {
		if last := root.Children[n-1]; last.Type == design.TypeInstance {
			if _, footer := HeaderFooter(s.st); footer > 0 && last.Props["componentId"] == float64(footer) {
				d.Head += "<style>." + design.ClassName(root.ID) + "{min-height:100vh}." + design.ClassName(last.ID) + "{margin-top:auto}</style>"
			}
		}
	}
	src := NewSource(s.st, s.plugins)
	var query url.Values
	if d.Request != nil {
		query = d.Request.URL.Query()
	}
	var data map[string]any
	var visitor func() map[string]any
	if sess := s.session(d.Request, d.Options); sess != nil {
		src.code = sess
		d.code = sess
		data = sess.PageData(d.Path)
		visitor = visitorFunc(sess)
		defer func() {
			cookies = append(cookies, sess.Cookies()...)
			sess.Close()
		}()
	}
	dc := designContext(d.Options, d.Post, d.Category, d.Path, query, data)
	dc.Visitor = visitor
	out := design.Render(root, dc, src)
	d.Head += out.Head
	if out.Runtime { // Site Code client runtime: actions, filters, load-more
		id := d.PostID
		if id == 0 {
			id = d.DesignID
		}
		d.BodyEnd += "<script>window.__reaP=" + strconv.FormatInt(id, 10) + "</script><script>" + design.RuntimeScript() + "</script>"
		if c := csrfCookie(d.Request); c != "" {
			cookies = append(cookies, c)
		}
	}
	if out.Anim { // scroll-in animations: runs in <head> so hidden-until-visible applies before the first paint
		d.Head += "<script>" + design.AnimScript + "</script>"
	}
	if out.Interact { // popups, tabs, accordions, sliders (json.Marshal escapes < > &)
		acts, _ := json.Marshal(out.Actions)
		d.BodyEnd += "<script>window.__reaAct=" + string(acts) + "</script><script>" + design.InteractScript + "</script>"
	}
	return s.document(d, out.HTML, out.CSS, out.Fonts), cookies, nil
}

// designContext is the binding context of a page render.
func designContext(opts map[string]string, post design.PostCtx, category, path string, query url.Values, page map[string]any) design.Context {
	locale := "th"
	if opts["language"] == "en" {
		locale = "en"
	}
	return design.Context{
		Site:       siteBindings(opts),
		Post:       post,
		Category:   category,
		TextStyles: ParseTheme(opts["theme"]).DesignTextStyles(),
		Page:       page,
		Query:      query,
		Path:       path,
		Locale:     locale,
		Loc:        bindingLoc(),
	}
}

var (
	bangkok     *time.Location
	bangkokOnce sync.Once
)

// bindingLoc is the timezone of | date in bindings: the site timezone,
// default Asia/Bangkok (the editor's evaluator does the same).
func bindingLoc() *time.Location {
	if l := getBases().loc; l != nil {
		return l
	}
	bangkokOnce.Do(func() {
		if l, err := time.LoadLocation("Asia/Bangkok"); err == nil {
			bangkok = l
		} else {
			bangkok = time.FixedZone("ICT", 7*3600)
		}
	})
	return bangkok
}

func siteBindings(opts map[string]string) map[string]string {
	return map[string]string{"title": opts["site_title"], "tagline": opts["tagline"], "url": "/",
		"logo": opts["site_logo"], "icon": opts["site_icon"], "language": orDefault(opts["language"], "th")}
}

// pageInfo builds the plugin hook view of a document.
func pageInfo(d Doc) plugin.PageInfo {
	site := d.Options["site_title"]
	full := d.Title
	if site != "" && full != site {
		if full == "" {
			full = site
		} else {
			full += " – " + site
		}
	}
	status := d.Status
	if status == 0 {
		status = http.StatusOK
	}
	pt := d.PostType
	if pt == "" {
		pt = "plugin"
	}
	return plugin.PageInfo{Path: d.Path, Title: d.Title, Description: d.Description, Image: d.Image,
		PostID: d.PostID, PostType: pt, Status: status, FullTitle: full, BaseURL: d.BaseURL,
		Site: siteBindings(d.Options)}
}

// document runs the plugin page hooks and wraps body into a full page.
func (s *Site) document(d Doc, body, css string, fonts []string) string {
	pi := pageInfo(d)
	if s.plugins != nil {
		s.plugins.FilterPage(&pi)
	}
	sess := d.code
	if sess == nil && d.Request != nil { // fallback pages: Site Code hooks still apply
		if cs := s.session(d.Request, d.Options); cs != nil {
			sess = cs
			defer cs.Close()
		}
	}
	if sess != nil {
		filterHead(sess, d, &pi.FullTitle, &pi.Description, &pi.Image, &pi.NoIndex)
		d.extra = filterMeta(sess, d, &pi)
	}
	if s.plugins != nil {
		d.Head += s.plugins.HeadHTML(pi)
		d.BodyEnd += s.plugins.BodyEndHTML(pi)
	}
	if ph, ok := sess.(pageHTMLer); ok && ph != nil {
		if ph.HasPageHTML("head") || ph.HasPageHTML("bodyEnd") {
			preloadPost(sess, d.PostID)
			page := pageObject(d, pi)
			d.Head += ph.PageHTML("head", page)
			d.BodyEnd += ph.PageHTML("bodyEnd", page)
		}
	}
	icon := s.siteIcon(d.Options)
	if icon == "" {
		icon = "/favicon.svg" // same default icon as the admin
	}
	if s.demo {
		d.BodyEnd += demoRibbon(d.Options)
	}
	return wrap(d, pi, icon, body, css, fonts)
}

// siteIcon resolves option site_icon (a media id or a URL).
func (s *Site) siteIcon(opts map[string]string) string {
	v := strings.TrimSpace(opts["site_icon"])
	if v == "" {
		return ""
	}
	if id, err := strconv.ParseInt(v, 10, 64); err == nil {
		if s.st == nil {
			return ""
		}
		if m, err := s.st.GetMedia(id); err == nil {
			return m.URL
		}
		return ""
	}
	if plugin.SafeURL(v) && !strings.HasPrefix(v, "#") {
		return v
	}
	return ""
}

// BaseCSS is shared by the public site and (mirrored) by the editor canvas.
const BaseCSS = `*,*::before,*::after{box-sizing:border-box}
html,body{margin:0;padding:0}
body{font-family:'Inter','Noto Sans Thai',system-ui,sans-serif;color:#111827;-webkit-font-smoothing:antialiased;line-height:1.5}
img{max-width:100%}
h1,h2,h3,h4,h5,h6,p,blockquote{margin:0;font-size:inherit;font-weight:inherit}
.rea-menu a{color:inherit;text-decoration:none}
.rea-menu a:hover{opacity:.7}
.rea-bt{position:absolute;opacity:0;width:1px;height:1px;margin:0;pointer-events:none}
.rea-bb{display:none;cursor:pointer;padding:6px;margin:-6px;line-height:0;border-radius:8px}
.rea-bl{display:contents}
@media (max-width:640px){.rea-burger .rea-bb{display:block}
.rea-burger .rea-bt:focus-visible+.rea-bb{outline:2px solid currentColor}
.rea-burger .rea-bl{display:none;position:absolute;right:0;top:100%;z-index:60;flex-direction:column;align-items:stretch;gap:2px;min-width:220px;max-width:calc(100vw - 32px);padding:8px;margin-top:10px;background:#fff;color:#111827;border-radius:12px;box-shadow:0 12px 32px rgba(17,24,39,.18);text-align:left}
.rea-burger .rea-bl a{padding:10px 12px;border-radius:8px}
.rea-burger .rea-bl a:hover{background:#f3f4f6;opacity:1}
.rea-burger .rea-bt:checked~.rea-bl{display:flex}}
.rea-btn{transition:filter .15s}.rea-btn:hover{filter:brightness(.92)}
.rea-download{gap:.5em}
.rea-file-meta{font-size:.8em;opacity:.75;font-weight:400;white-space:nowrap}
.rea-content{line-height:1.75;overflow-wrap:break-word}
.rea-content>*+*{margin-top:1em}
.rea-content::after{content:"";display:block;clear:both}
.rea-content h1{font-size:2em;font-weight:800;line-height:1.25}
.rea-content h2{font-size:1.6em;font-weight:700;line-height:1.3;margin-top:1.4em}
.rea-content h3{font-size:1.3em;font-weight:600;line-height:1.35;margin-top:1.2em}
.rea-content h4{font-size:1.1em;font-weight:600;margin-top:1em}
.rea-content a{color:#4f46e5;text-decoration:underline;text-underline-offset:2px}
.rea-content strong,.rea-content b{font-weight:700}
.rea-content ul,.rea-content ol{padding-left:1.5em}
.rea-content li+li{margin-top:.35em}
.rea-content blockquote{margin-left:0;margin-right:0;padding:.2em 0 .2em 1.1em;border-left:4px solid #c7d2fe;color:#374151;font-style:italic}
.rea-content blockquote.callout{font-style:normal;border-left:0;padding:1em 1.2em;border-radius:12px;background:#eef2ff;color:#1e1b4b}
.rea-content pre{background:#0f172a;color:#e2e8f0;padding:1em 1.2em;border-radius:10px;overflow-x:auto;font-size:.88em;line-height:1.6;white-space:pre}
.rea-content code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:.9em;background:#f1f5f9;color:#be185d;padding:.1em .35em;border-radius:5px}
.rea-content pre code{background:none;color:inherit;padding:0}
.rea-content mark{background:#fef08a;padding:0 .15em;border-radius:3px}
.rea-content hr{border:0;border-top:1px solid #e5e7eb;margin:2em 0}
.rea-content img{max-width:100%;height:auto;border-radius:8px}
.rea-content figure{margin-left:0;margin-right:0}
.rea-content figure img{display:block;width:100%}
.rea-content figure.aligncenter{margin-left:auto;margin-right:auto}
.rea-content figure.alignleft{float:left;margin:.3em 1.5em 1em 0;max-width:60%}
.rea-content figure.alignright{float:right;margin:.3em 0 1em 1.5em;max-width:60%}
.rea-content figcaption{margin-top:.5em;font-size:.85em;color:#6b7280;text-align:center}
.rea-content figure.video{position:relative;aspect-ratio:16/9;width:100%}
.rea-content figure.video iframe,.rea-content iframe{width:100%;height:100%;border:0;border-radius:10px}
.rea-content iframe{aspect-ratio:16/9;height:auto}
.rea-content table{width:100%;border-collapse:collapse;font-size:.95em}
.rea-content th,.rea-content td{border:1px solid #e5e7eb;padding:.55em .75em;text-align:left;vertical-align:top}
.rea-content th{background:#f9fafb;font-weight:600}
.rea-content .file-download{display:inline-flex;align-items:center;gap:.4em;padding:.55em 1em;border:1px solid #e5e7eb;border-radius:10px;text-decoration:none;font-weight:600}
@media (max-width:640px){.rea-content figure.alignleft,.rea-content figure.alignright{float:none;margin:1em 0;max-width:100%;width:100%!important}.rea-content table{display:block;overflow-x:auto}}
.rea-empty{opacity:.6}
.rea-breadcrumb{display:flex;flex-wrap:wrap;align-items:center;gap:.5em}
.rea-breadcrumb a{color:inherit;text-decoration:none}.rea-breadcrumb a:hover{text-decoration:underline}
.rea-bc-sep{opacity:.5}
`

func wrap(d Doc, pi plugin.PageInfo, icon, body, css string, fonts []string) string {
	theme := ParseTheme(d.Options["theme"])
	families := []string{"Inter:wght@300;400;500;600;700;800", "Noto+Sans+Thai:wght@300;400;500;600;700"}
	seen := map[string]bool{"Inter": true, "Noto Sans Thai": true}
	for _, f := range append(theme.Fonts(), fonts...) {
		if seen[f] {
			continue
		}
		seen[f] = true
		families = append(families, strings.ReplaceAll(url.QueryEscape(f), "%20", "+")+":wght@300;400;500;600;700;800")
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="` + html.EscapeString(orDefault(d.Options["language"], "th")) + `"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<title>` + html.EscapeString(pi.FullTitle) + `</title>`)
	if pi.Description != "" {
		b.WriteString(`<meta name="description" content="` + html.EscapeString(pi.Description) + `">`)
	}
	switch {
	case d.Options["search_visibility"] == "0" || pi.NoIndex:
		b.WriteString(`<meta name="robots" content="noindex,nofollow">`)
	case d.extra.Robots != "":
		b.WriteString(`<meta name="robots" content="` + html.EscapeString(d.extra.Robots) + `">`)
	}
	b.WriteString(d.extra.html())
	b.WriteString(`<meta name="generator" content="` + Generator + `">`)
	if icon != "" {
		b.WriteString(`<link rel="icon" href="` + html.EscapeString(icon) + `">`)
		b.WriteString(`<link rel="apple-touch-icon" href="` + html.EscapeString(icon) + `">`)
	}
	b.WriteString(`<link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>`)
	b.WriteString(`<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=` + strings.Join(families, "&family=") + `&display=swap">`)
	b.WriteString("<style>\n" + BaseCSS + theme.CSS() + css + "</style>" + d.Head + "</head><body>")
	b.WriteString(body)
	b.WriteString(d.BodyEnd)
	b.WriteString(`</body></html>`)
	return b.String()
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

var thaiMonths = strings.NewReplacer(
	"January", "มกราคม", "February", "กุมภาพันธ์", "March", "มีนาคม", "April", "เมษายน", "May", "พฤษภาคม", "June", "มิถุนายน",
	"July", "กรกฎาคม", "August", "สิงหาคม", "September", "กันยายน", "October", "ตุลาคม", "November", "พฤศจิกายน", "December", "ธันวาคม",
	"Jan", "ม.ค.", "Feb", "ก.พ.", "Mar", "มี.ค.", "Apr", "เม.ย.", "Jun", "มิ.ย.", "Jul", "ก.ค.", "Aug", "ส.ค.", "Sep", "ก.ย.",
	"Oct", "ต.ค.", "Nov", "พ.ย.", "Dec", "ธ.ค.")

// FormatDate formats a post date with the site's date format, timezone and
// language (Thai sites: Thai month names and the Buddhist year).
func FormatDate(t time.Time) string {
	b := getBases()
	if b.loc != nil {
		t = t.In(b.loc)
	}
	f := b.dateFormat
	if f == "" || !strings.ContainsAny(f, "0123456789") {
		f = "2 Jan 2006"
	}
	out := t.Format(f)
	if !b.thai {
		return out
	}
	if strings.Contains(f, "2006") {
		out = strings.Replace(out, strconv.Itoa(t.Year()), strconv.Itoa(t.Year()+543), 1)
	}
	return thaiMonths.Replace(out)
}
