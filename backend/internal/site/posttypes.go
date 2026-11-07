package site

import (
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
)

// Site Code phase 4a on the public site: custom post types (site.postType)
// at /<slug>/<post-slug> with an archive at /<slug>, the visitor in
// bindings, and the post.content / page.head filters.

// PostTypeRoute is a custom post type as the public site needs it.
type PostTypeRoute struct {
	Type   string
	Slug   string
	Public bool
	Label  map[string]string // {th, en}
	Fields []RouteField      // its custom fields (2.5.0: the Fields block, the default page)
}

var typeSlugs atomic.Pointer[map[string]string]

// SetTypeSlugs sets the URL prefixes of custom post types (type → slug),
// used by PostURL. Site Code calls it whenever the live version changes.
func SetTypeSlugs(m map[string]string) {
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	typeSlugs.Store(&cp)
}

func typeSlug(typ string) (string, bool) {
	if m := typeSlugs.Load(); m != nil {
		s, ok := (*m)[typ]
		return s, ok
	}
	return "", false
}

// SetPostTypes installs the provider of custom post types (Site Code).
func (s *Site) SetPostTypes(fn func(opts map[string]string) []PostTypeRoute) { s.postTypes = fn }

// customTypes returns the custom post types for this request.
func (s *Site) customTypes(opts map[string]string) []PostTypeRoute {
	if s.postTypes == nil {
		return nil
	}
	return s.postTypes(opts)
}

func typeBySlug(types []PostTypeRoute, slug string) *PostTypeRoute {
	for i := range types {
		if types[i].Slug == slug && types[i].Public {
			return &types[i]
		}
	}
	return nil
}

func typeByID(types []PostTypeRoute, id string) *PostTypeRoute {
	for i := range types {
		if types[i].Type == id {
			return &types[i]
		}
	}
	return nil
}

// typeTemplate is the template of a custom type's single posts: option
// post_template_<type>, else none (the post's own design or the default).
func (s *Site) typeTemplate(opts map[string]string, typ string) *model.Post {
	return s.template(opts["post_template_"+typ])
}

// singleCustom renders /<slug>/<post-slug> of a custom post type.
func (s *Site) singleCustom(w http.ResponseWriter, opts map[string]string, t *PostTypeRoute, slug string) {
	p, err := s.st.PostBySlug(t.Type, slug, true)
	if err != nil {
		s.notFound(w, opts)
		return
	}
	tpl := s.typeTemplate(opts, t.Type)
	if s.langGate(w, opts, p) {
		return
	}
	if tpl == nil && len(p.Design) == 0 {
		tpl = s.defaultTemplateFor(p)
	}
	if tpl == nil {
		s.renderPost(w, opts, p, nil, "", http.StatusOK)
		return
	}
	ctx := PostContext(p)
	s.renderPost(w, opts, tpl, ctx, "", http.StatusOK, meta{title: p.Title, description: ctx["excerpt"],
		image: p.FeaturedImage, postID: p.ID, postType: p.Type})
}

// typeArchive lists a custom type's published posts at /<slug> (the archive
// template when one is set).
func (s *Site) typeArchive(w http.ResponseWriter, opts map[string]string, t *PostTypeRoute) {
	title := t.Label["th"]
	if opts["language"] == "en" && t.Label["en"] != "" {
		title = t.Label["en"]
	}
	if title == "" {
		title = t.Type
	}
	m := meta{title: title, postType: "archive"}
	tpl := s.template(opts["archive_template_"+t.Type]) // 2.5.0: the type's own archive (sorted, filtered)
	if tpl == nil {
		tpl = s.template(opts["archive_template_id"])
	}
	if tpl != nil {
		s.renderPost(w, opts, tpl, design.PostCtx{"title": title, "url": "/" + t.Slug, "type": t.Type, "archiveType": t.Type}, "", http.StatusOK, m)
		return
	}
	posts, _, _ := s.st.ListPosts(model.PostFilter{Type: t.Type, Status: model.StatusPublish, Limit: 50, NoContent: true, Lang: pageLang(opts)})
	b := `<h1>` + html.EscapeString(title) + `</h1><ul>`
	for _, p := range posts {
		b += `<li><a href="` + html.EscapeString(PostURL(p)) + `">` + html.EscapeString(p.Title) + `</a></li>`
	}
	b += `</ul>`
	s.writeFallback(w, opts, m, b, http.StatusOK)
}

// customURL is PostURL for custom post types.
func customURL(p *model.Post) (string, bool) {
	slug, ok := typeSlug(p.Type)
	if !ok {
		return "", false
	}
	return "/" + slug + "/" + url.PathEscape(p.Slug), true
}

// metaJSON carries custom fields into PostCtx (decoded by the renderer for
// {{post.meta.<key>}}).
func metaJSON(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// visitorFunc returns the lazy {{visitor}} of a code session.
func visitorFunc(sess design.CodeSession) func() map[string]any {
	vs, ok := sess.(design.VisitorSource)
	if !ok || vs == nil {
		return nil
	}
	return vs.Visitor
}

// FilterContent runs Site Code's post.content filters (design.ContentFilter).
// The code is trusted (publishing it is administrator-equivalent): what a
// filter returns is inserted as is.
func (s *Source) FilterContent(h string, post design.PostCtx) string {
	f, ok := s.code.(design.Filterer)
	if !ok || f == nil || !f.HasFilter("post.content") {
		return h
	}
	pm := make(map[string]any, len(post))
	for k, v := range post {
		pm[k] = v
	}
	if pv, ok := postMapOf(post); ok {
		pm = pv
	}
	ctx := map[string]any{"post": pm}
	if vf := visitorFunc(s.code); vf != nil { // 2.5.0: who reads it (members-only content, course lessons)
		if v := vf(); v != nil {
			ctx["visitor"] = v
		}
	}
	if out, ok := f.ApplyFilter("post.content", h, ctx).(string); ok {
		return out
	}
	return h
}

func postMapOf(post design.PostCtx) (map[string]any, bool) {
	raw, ok := post["meta"]
	if !ok || raw == "" {
		return nil, false
	}
	var meta map[string]any
	if json.Unmarshal([]byte(raw), &meta) != nil {
		return nil, false
	}
	out := make(map[string]any, len(post))
	for k, v := range post {
		out[k] = v
	}
	out["meta"] = meta
	return out, true
}

// filterHead runs page.head over the document's head values.
func filterHead(sess design.CodeSession, d Doc, title, description, image *string, noindex *bool) {
	f, ok := sess.(design.Filterer)
	if !ok || f == nil || !f.HasFilter("page.head") {
		return
	}
	post := map[string]any{}
	for k, v := range d.Post {
		post[k] = v
	}
	if pm, ok := postMapOf(d.Post); ok {
		post = pm
	}
	val := map[string]any{"title": *title, "description": *description, "image": *image, "noindex": *noindex}
	out, ok := f.ApplyFilter("page.head", val, map[string]any{"post": post, "path": d.Path}).(map[string]any)
	if !ok {
		return
	}
	if v, ok := out["title"].(string); ok {
		*title = v
	}
	if v, ok := out["description"].(string); ok {
		*description = v
	}
	if v, ok := out["image"].(string); ok {
		*image = v
	}
	if v, ok := out["noindex"].(bool); ok {
		*noindex = v
	}
}
