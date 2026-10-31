package site

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

// The methods below implement plugin.Pages so plugins can render public
// pages that share the site's look.

// Page renders the published page with slug; false when there is none.
func (s *Site) Page(w http.ResponseWriter, slug string) bool {
	opts, err := s.st.Options()
	if err != nil {
		return false
	}
	p, err := s.st.PostBySlug(model.TypePage, slug, true)
	if err != nil {
		return false
	}
	s.renderPost(w, opts, p, nil, "", http.StatusOK)
	return true
}

// Template renders template id with ctx as the {{post.*}} context.
func (s *Site) Template(w http.ResponseWriter, id int64, ctx design.PostCtx, title, description string) bool {
	opts, err := s.st.Options()
	if err != nil {
		return false
	}
	tpl := s.template(strconv.FormatInt(id, 10))
	if tpl == nil {
		return false
	}
	s.renderPost(w, opts, tpl, ctx, "", http.StatusOK, meta{title: title, description: description,
		image: ctx["image"], postType: "plugin"})
	return true
}

// HTML wraps body with the site's header and footer components (when they
// exist) using the design renderer, so plugin pages look like the rest.
func (s *Site) HTML(w http.ResponseWriter, status int, title, body string) {
	opts, err := s.st.Options()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	header, footer := HeaderFooter(s.st)
	inst := func(id string, comp int64) *design.Node {
		return &design.Node{ID: id, Type: design.TypeInstance, Width: design.Size{Mode: "fill"},
			Height: design.Size{Mode: "hug"}, Props: map[string]any{"componentId": float64(comp)}}
	}
	root := &design.Node{ID: "root", Type: design.TypeFrame, Width: design.Size{Mode: "fill"},
		Height: design.Size{Mode: "hug"}, Layout: map[string]any{"mode": "vertical"},
		Style: map[string]any{"fill": "#ffffff"}}
	if header > 0 {
		root.Children = append(root.Children, inst("plugin-header", header))
	}
	root.Children = append(root.Children, &design.Node{ID: "plugin-body", Type: design.TypeEmbed,
		Width: design.Size{Mode: "fill"}, Height: design.Size{Mode: "hug"}, Props: map[string]any{"html": body}})
	if footer > 0 {
		root.Children = append(root.Children, inst("plugin-footer", footer))
	}
	path, base := reqInfo(w)
	page, err := s.RenderDocument(root, Doc{Title: title, Options: opts, Path: path, BaseURL: base,
		PostType: "plugin", Status: status, Request: reqOf(w)})
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	writeHTML(w, status, page)
}

// Options naming the components chosen as the site header and footer
// (POST /api/v1/components/{id}/site-role).
const (
	OptionHeaderComponent = "site_header_component"
	OptionFooterComponent = "site_footer_component"
)

// RoleOption is the option holding the component of a site role ("header"
// or "footer"); "" for anything else.
func RoleOption(role string) string {
	switch role {
	case "header":
		return OptionHeaderComponent
	case "footer":
		return OptionFooterComponent
	}
	return ""
}

// HeaderFooter finds the site's header and footer components: the ones
// chosen with the site-role action (options site_header_component /
// site_footer_component) and, for sites saved before that existed, by name
// ("ส่วนหัวเว็บ"/"Header", "ส่วนท้ายเว็บ"/"Footer"); 0 when missing. Only
// ids and names are read (it runs on every page view).
func HeaderFooter(st *store.Store) (header, footer int64) {
	cs, err := st.ComponentNames()
	if err != nil {
		return 0, 0
	}
	exists := make(map[int64]bool, len(cs))
	for _, c := range cs {
		exists[c.ID] = true
	}
	if opts, err := st.Options(); err == nil {
		if id, _ := strconv.ParseInt(opts[OptionHeaderComponent], 10, 64); exists[id] {
			header = id
		}
		if id, _ := strconv.ParseInt(opts[OptionFooterComponent], 10, 64); exists[id] && id != header {
			footer = id
		}
	}
	for _, c := range cs {
		n := strings.ToLower(strings.TrimSpace(c.Name))
		switch {
		case header == 0 && c.ID != footer && (n == "ส่วนหัวเว็บ" || n == "header"):
			header = c.ID
		case footer == 0 && c.ID != header && (n == "ส่วนท้ายเว็บ" || n == "footer"):
			footer = c.ID
		}
	}
	return header, footer
}

// defaultPostTemplate shows a post (without a design of its own) between the
// site header and footer until the owner makes a post template.
func (s *Site) defaultPostTemplate() *model.Post {
	header, footer := HeaderFooter(s.st)
	inst := func(id string, comp int64) *design.Node {
		return &design.Node{ID: id, Type: design.TypeInstance, Width: design.Size{Mode: "fill"},
			Height: design.Size{Mode: "hug"}, Props: map[string]any{"componentId": float64(comp)}}
	}
	text := func(id, t, tag string, style map[string]any) *design.Node {
		return &design.Node{ID: id, Type: design.TypeText, Width: design.Size{Mode: "fill"}, Height: design.Size{Mode: "hug"},
			Style: style, Props: map[string]any{"text": t, "tag": tag}}
	}
	article := &design.Node{ID: "post-article", Type: design.TypeFrame, Width: design.Size{Mode: "fill"}, Height: design.Size{Mode: "hug"},
		Layout: map[string]any{"mode": "vertical", "gap": 16.0, "padding": []any{56.0, 24.0, 80.0, 24.0}},
		Style:  map[string]any{"maxWidth": 760.0},
		Children: []*design.Node{
			text("post-title", "{{post.title}}", "h1", map[string]any{"fontSize": 40.0, "fontWeight": 800.0, "lineHeight": 1.2}),
			text("post-date", "{{post.date}}", "p", map[string]any{"fontSize": 14.0, "color": "var(--c-muted)"}),
			{ID: "post-content", Type: design.TypePostContent, Width: design.Size{Mode: "fill"}, Height: design.Size{Mode: "hug"}, Style: map[string]any{"fontSize": 18.0}},
		}}
	root := &design.Node{ID: "root", Type: design.TypeFrame, Width: design.Size{Mode: "fill"}, Height: design.Size{Mode: "hug"},
		Layout: map[string]any{"mode": "vertical", "align": "center"}, Style: map[string]any{"fill": "#ffffff"}}
	if header > 0 {
		root.Children = append(root.Children, inst("post-header", header))
	}
	root.Children = append(root.Children, article)
	if footer > 0 {
		root.Children = append(root.Children, inst("post-footer", footer))
	}
	raw, err := json.Marshal(root)
	if err != nil {
		return nil
	}
	return &model.Post{Type: model.TypeTemplate, Title: "", Design: raw}
}
