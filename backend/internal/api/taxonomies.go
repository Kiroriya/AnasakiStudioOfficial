package api

import (
	"net/http"
	"slices"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Taxonomies (CMS 2.1.0): "category" plus those registered with
// site.taxonomy by Site Code and JS plugins.
//
//	GET /api/v1/taxonomies   any signed-in user (admin sidebar and post editor)
//	  → [{id, label, singular, hierarchical, postTypes, slug, public, builtin, canManage, plugin?, pluginName?}]

type taxonomyOut struct {
	sitecode.TaxonomyInfo
	Builtin   bool `json:"builtin"`
	CanManage bool `json:"canManage"`
}

func (a *API) manifest() sitecode.Manifest {
	if a.code == nil {
		return sitecode.EmptyManifest()
	}
	return a.code.PostTypes()
}

// taxonomy returns a registered taxonomy (nil for "category" and unknown ids).
func (a *API) taxonomy(id string) *sitecode.TaxonomyInfo {
	if id == sitecode.CategoryTaxonomy {
		return nil
	}
	return a.manifest().Taxonomy(id)
}

func (a *API) knownTaxonomy(id string) bool {
	return id == sitecode.CategoryTaxonomy || a.taxonomy(id) != nil
}

// canManageTerms: manage_categories for "category", the taxonomy's
// access.manage otherwise.
func (a *API) canManageTerms(me *auth.Principal, tax string) bool {
	if tax == sitecode.CategoryTaxonomy {
		return me.Can(auth.CapManageTerms)
	}
	t := a.taxonomy(tax)
	return t != nil && t.CanManage(me.Role)
}

func (a *API) listTaxonomies(w http.ResponseWriter, r *http.Request) {
	me := auth.From(r)
	m := a.manifest()
	cat := sitecode.TaxonomyInfo{ID: sitecode.CategoryTaxonomy, Label: map[string]string{"th": "หมวดหมู่", "en": "Categories"},
		Singular: map[string]string{"th": "หมวดหมู่", "en": "Category"}, PostTypes: []string{model.TypePost},
		Slug: site.CategoryBase(), Public: true, Access: sitecode.TaxonomyAccess{Manage: []string{}}}
	for _, t := range m.PostTypes {
		if slices.Contains(t.Supports, "categories") {
			cat.PostTypes = append(cat.PostTypes, t.Type)
		}
	}
	out := []taxonomyOut{{TaxonomyInfo: cat, Builtin: true, CanManage: me.Can(auth.CapManageTerms)}}
	for _, t := range m.Taxonomies {
		out = append(out, taxonomyOut{TaxonomyInfo: t, CanManage: t.CanManage(me.Role)})
	}
	writeJSON(w, http.StatusOK, out)
}

// postTermsInput checks PostInput.terms against the post type: every
// taxonomy must apply to typ ("" = ok).
func (a *API) postTermsInput(typ string, terms map[string][]int64) string {
	if len(terms) == 0 {
		return ""
	}
	allowed := a.manifest().TaxonomiesOf(typ)
	for tax, ids := range terms {
		if !slices.Contains(allowed, tax) {
			return "ประเภทเนื้อหานี้ไม่ได้ใช้ชุดหมวด " + tax
		}
		if len(ids) > 100 {
			return "เลือกหมวดได้ไม่เกิน 100 รายการต่อชุด"
		}
	}
	return ""
}
