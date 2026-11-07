package api

import (
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/seed"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Starter kits (2.5.0, store 1.6.0): the setup wizard offers the kits the
// Plugin Store lists (a café, a shop, a clinic …) and builds the new site from
// the chosen one: its pages (design sections inside the site header and
// footer), menu, categories, content types, sample posts, and the official
// plugins it works with ticked in the next step. A kit is data from the store:
// the core names no plugin and no business, and a kit's designs keep only
// plain layout nodes (no HTML embeds, code blocks or references to records
// of another site).
//
//	GET /api/v1/setup/kits   (before setup, or manage_options) → {storeAvailable, items:[{id, name, description, icon, plugins}]}

// optKitPlugins remembers the chosen kit's plugins for the "Recommended plugins" step.
const optKitPlugins = "setup_kit_plugins"

type kitDoc struct {
	ID           string                        `json:"id"`
	Plugins      []string                      `json:"plugins"`
	Menu         []map[string]any              `json:"menu"`
	Categories   []struct{ Name, Slug string } `json:"categories"`
	ContentTypes []sitecode.TypeSpec           `json:"contentTypes"`
	Pages        []kitPage                     `json:"pages"`
	Posts        []kitPost                     `json:"posts"`
}

type kitPage struct {
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	Home       bool   `json:"home"`
	ArchiveFor string `json:"archiveFor"`
	Sections   []any  `json:"sections"`
}

type kitPost struct {
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Slug     string         `json:"slug"`
	Excerpt  string         `json:"excerpt"`
	Content  string         `json:"content"`
	Category string         `json:"category"`
	Meta     map[string]any `json:"meta"`
	Image    []string       `json:"image"` // two hex colours of a generated cover
}

var (
	kitIDRe   = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)
	kitSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,60}$`)
)

// kitClip cuts s to at most n characters.
func kitClip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// kitNodeTypes are the design nodes a kit may use.
var kitNodeTypes = map[string]bool{"frame": true, "text": true, "rect": true, "image": true, "button": true, "postList": true,
	"postContent": true, "postFields": true, "breadcrumb": true, "icon": true, "shape": true, "pagination": true, "input": true}

// cleanKitNode keeps a kit's design to plain layout: unknown node types (HTML embeds, code blocks, components
// and menus of another site …) are dropped and so are raw-HTML props. n counts nodes (a kit page has at most 3000).
func cleanKitNode(v any, depth int, n *int) (map[string]any, bool) {
	node, ok := v.(map[string]any)
	if !ok || depth > 30 || *n >= 3000 {
		return nil, false
	}
	t, _ := node["type"].(string)
	if !kitNodeTypes[t] {
		return nil, false
	}
	*n++
	if props, ok := node["props"].(map[string]any); ok {
		for _, k := range []string{"html", "css", "js", "rich"} {
			delete(props, k)
		}
	}
	if kids, ok := node["children"].([]any); ok {
		out := make([]any, 0, len(kids))
		for _, k := range kids {
			if c, ok := cleanKitNode(k, depth+1, n); ok {
				out = append(out, c)
			}
		}
		node["children"] = out
	}
	return node, true
}

func hexRGBA(s string) color.RGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.RGBA{99, 102, 241, 255}
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{99, 102, 241, 255}
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

// kitsList answers GET /api/v1/setup/kits.
func (a *API) kitsList(w http.ResponseWriter, r *http.Request) {
	v, err := a.plugins.StoreJSON(http.MethodGet, "/api/v1/starters", nil, nil, plugin.StoreRaw, nil)
	items := []map[string]any{}
	if m, ok := v.(map[string]any); ok && err == nil {
		list, _ := m["items"].([]any)
		for _, x := range list {
			o, ok := x.(map[string]any)
			id, _ := o["id"].(string)
			if !ok || !kitIDRe.MatchString(id) {
				continue
			}
			items = append(items, map[string]any{"id": id, "name": textOf(o["name"]), "description": textOf(o["description"]),
				"icon": o["icon"], "plugins": strList(o["plugins"])})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"storeAvailable": err == nil, "items": items})
}

// fetchKit downloads kit id from the store.
func (a *API) fetchKit(id string) (*kitDoc, error) {
	if !kitIDRe.MatchString(id) {
		return nil, fmt.Errorf("bad kit id")
	}
	v, err := a.plugins.StoreJSON(http.MethodGet, "/api/v1/starters/"+url.PathEscape(id), nil, nil, plugin.StoreRaw, nil)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var k kitDoc
	if err := json.Unmarshal(raw, &k); err != nil {
		return nil, err
	}
	if k.ID != id || len(k.Pages) == 0 || len(k.Pages) > 20 || len(k.Posts) > 50 || len(k.ContentTypes) > 10 {
		return nil, fmt.Errorf("kit %q is not usable", id)
	}
	return &k, nil
}

// applyKit builds the new site from k (setup, before the first sign-in).
func (a *API) applyKit(k *kitDoc, authorID int64) error {
	st := a.st.Quiet()
	frame, err := seed.KitBase(st, authorID, k.Menu)
	if err != nil {
		return err
	}
	opts := map[string]string{}
	// content types (validated like the admin form's)
	if len(k.ContentTypes) > 0 {
		cur, err := st.Options()
		if err != nil {
			return err
		}
		var specs []sitecode.TypeSpec
		for _, s := range k.ContentTypes {
			info, err := sitecode.ValidateTypeSpec(s)
			if err != nil {
				log.Printf("starter kit %s: content type %s: %v", k.ID, s.Type, err)
				continue
			}
			if msg := a.typeSlugProblem(info.Slug, cur); msg != "" {
				log.Printf("starter kit %s: content type %s: %s", k.ID, s.Type, msg)
				continue
			}
			specs = append(specs, sitecode.SpecOf(info))
		}
		b, _ := json.Marshal(specs)
		if err := st.SetOptions(map[string]string{sitecode.OptContentTypes: string(b)}); err != nil {
			return err
		}
	}
	// categories
	cats := map[string]int64{}
	for _, c := range k.Categories {
		slug := strings.TrimSpace(c.Slug)
		if c.Name == "" || !kitSlugRe.MatchString(slug) {
			continue
		}
		t := &model.Term{Taxonomy: "category", Name: kitClip(c.Name, 100), Slug: slug}
		if err := st.CreateTerm(t); err == nil {
			cats[slug] = t.ID
		}
	}
	// pages and per-type archives
	for _, pg := range k.Pages {
		var sections []any
		n := 0
		for _, s := range pg.Sections {
			if c, ok := cleanKitNode(s, 0, &n); ok {
				sections = append(sections, c)
			}
		}
		design := frame.Page(sections)
		title := kitClip(strings.TrimSpace(pg.Title), 200)
		if pg.ArchiveFor != "" {
			id, err := frame.Template(st, authorID, title, pg.Slug, design)
			if err != nil {
				return err
			}
			opts["archive_template_"+pg.ArchiveFor] = strconv.FormatInt(id, 10)
			continue
		}
		slug, err := st.UniqueSlug(model.TypePage, pg.Slug, 0)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(design)
		p := &model.Post{Type: model.TypePage, Title: title, Slug: slug, Status: model.StatusPublish, Design: raw, AuthorID: authorID}
		if err := st.CreatePost(p, nil); err != nil {
			return err
		}
		if pg.Home {
			opts["home_page_id"] = strconv.FormatInt(p.ID, 10)
		}
	}
	// sample posts (with a generated cover and their fields)
	for i, kp := range k.Posts {
		typ := kp.Type
		if typ == "" {
			typ = model.TypePost
		}
		slug, err := st.UniqueSlug(typ, kp.Slug, 0)
		if err != nil {
			return err
		}
		p := &model.Post{Type: typ, Title: kitClip(kp.Title, 200), Slug: slug, Status: model.StatusPublish,
			Excerpt: kitClip(kp.Excerpt, 500), Content: sanitizeHTML(kp.Content), AuthorID: authorID}
		if len(kp.Image) == 2 {
			if m, err := seed.GradientMedia(st, a.uploadDir, fmt.Sprintf("kit-%s-%d", k.ID, i), hexRGBA(kp.Image[0]), hexRGBA(kp.Image[1]), authorID); err == nil {
				p.FeaturedMediaID = &m.ID
			}
		}
		var terms []int64
		if id, ok := cats[kp.Category]; ok {
			terms = append(terms, id)
		}
		if err := st.CreatePost(p, terms); err != nil {
			return err
		}
		meta := map[string]any{}
		for key, v := range kp.Meta {
			switch v.(type) {
			case string, float64, bool:
				if len(key) <= 40 && !strings.Contains(key, ".") {
					meta[key] = v
				}
			}
		}
		if len(meta) > 0 {
			if err := st.SetPostMeta(p.ID, meta); err != nil {
				return err
			}
		}
	}
	plugins := []string{}
	for _, id := range k.Plugins {
		if plugin.ValidPackageID(id) {
			plugins = append(plugins, id)
		}
	}
	opts[optKitPlugins] = strings.Join(plugins, ",")
	return st.SetOptions(opts)
}

// kitPlugins are the chosen kit's plugins (the "Recommended plugins" step offers and ticks them).
func (a *API) kitPlugins() []string {
	opts, err := a.st.Options()
	if err != nil || opts[optKitPlugins] == "" {
		return nil
	}
	var out []string
	for _, id := range strings.Split(opts[optKitPlugins], ",") {
		if plugin.ValidPackageID(id) {
			out = append(out, id)
		}
	}
	return out
}
