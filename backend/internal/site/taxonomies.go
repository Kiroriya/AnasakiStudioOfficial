package site

import (
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
)

// Public archives of the taxonomies registered with site.taxonomy (CMS
// 2.1.0): /<slug>/<term-slug> lists the published posts having the term or
// one of its children ("category" keeps its own /category/<slug>).

// TaxonomyRoute is a taxonomy's public archive.
type TaxonomyRoute struct {
	ID        string
	Slug      string
	Public    bool
	Label     map[string]string
	PostTypes []string
}

// SetTaxonomies sets where the registered taxonomies come from (Site Code).
func (s *Site) SetTaxonomies(fn func(opts map[string]string) []TaxonomyRoute) { s.taxes = fn }

func (s *Site) taxonomies(opts map[string]string) []TaxonomyRoute {
	if s.taxes == nil {
		return nil
	}
	return s.taxes(opts)
}

func taxBySlug(list []TaxonomyRoute, slug string) *TaxonomyRoute {
	for i := range list {
		if list[i].Public && list[i].Slug == slug {
			return &list[i]
		}
	}
	return nil
}

// TermURL is the public path of a term of a public taxonomy.
func TermURL(taxSlug, termSlug string) string { return "/" + taxSlug + "/" + url.PathEscape(termSlug) }

// termFamily is the term and its descendants (5 levels).
func (s *Site) termFamily(t *model.Term) []int64 {
	ids := []int64{t.ID}
	level := []int64{t.ID}
	for depth := 0; depth < 5 && len(level) > 0; depth++ {
		var next []int64
		for _, id := range level {
			parent := id
			kids, err := s.st.FindTerms(model.TermFilter{Taxonomy: t.Taxonomy, Parent: &parent, Limit: 1000})
			if err != nil {
				break
			}
			for _, k := range kids {
				next = append(next, k.ID)
			}
		}
		ids = append(ids, next...)
		level = next
	}
	return ids
}

func (s *Site) termArchive(w http.ResponseWriter, opts map[string]string, tax *TaxonomyRoute, slug string) {
	t, err := s.st.TermBySlug(tax.ID, slug)
	if err != nil {
		s.notFound(w, opts)
		return
	}
	m := meta{title: t.Name, description: t.Description, postType: "archive"}
	family := s.termFamily(t)
	if tpl := s.template(opts["archive_template_id"]); tpl != nil {
		ids := make([]string, len(family))
		for i, id := range family {
			ids[i] = strconv.FormatInt(id, 10)
		}
		// the template's "this archive's posts" list shows the term's posts (design.Query.TermIDs)
		ctx := design.PostCtx{"title": t.Name, "slug": t.Slug, "url": TermURL(tax.Slug, t.Slug), "taxonomy": tax.ID,
			"excerpt": t.Description, "archiveTerms": strings.Join(ids, ","), "archiveTypes": strings.Join(tax.PostTypes, ",")}
		s.renderPost(w, opts, tpl, ctx, "", http.StatusOK, m)
		return
	}
	posts, _, _ := s.st.ListPosts(model.PostFilter{Types: tax.PostTypes, Status: model.StatusPublish, TermIDs: family,
		Limit: 50, NoContent: true, PinnedFirst: true, Lang: pageLang(opts)})
	b := `<h1>` + html.EscapeString(t.Name) + `</h1>`
	if t.Description != "" {
		b += `<p>` + html.EscapeString(t.Description) + `</p>`
	}
	b += `<ul>`
	for _, p := range posts {
		b += `<li><a href="` + html.EscapeString(PostURL(p)) + `">` + html.EscapeString(p.Title) + `</a></li>`
	}
	b += `</ul>`
	s.writeFallback(w, opts, m, b, http.StatusOK)
}
