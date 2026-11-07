package sitecode

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/auth"
)

// Taxonomies (CMS 2.1.0, P1-1): site.taxonomy registers a kind of term
// ("product categories", "tags", "genres") for one or more post types. The
// admin gets a terms screen and a panel in the post editor, db.terms reads
// and writes the terms, PostInput.terms assigns them, and a public taxonomy
// has archive pages /<slug>/<term-slug>. "category" (posts) is the core one.

// CategoryTaxonomy is the core taxonomy of posts (and of custom types that
// support "categories").
const CategoryTaxonomy = "category"

// TaxonomyAccess lists who manages a taxonomy's terms: role slugs and
// "cap:<capability>" entries.
type TaxonomyAccess struct {
	Manage []string `json:"manage"`
}

// TaxonomyInfo is a registered site.taxonomy.
type TaxonomyInfo struct {
	ID           string            `json:"id"`
	Label        map[string]string `json:"label"`
	Singular     map[string]string `json:"singular"`
	Hierarchical bool              `json:"hierarchical"`
	// PostTypes are the content types whose posts take these terms.
	PostTypes []string `json:"postTypes"`
	// Slug is the public archive base: /<slug>/<term-slug> when Public.
	Slug   string         `json:"slug"`
	Public bool           `json:"public"`
	Access TaxonomyAccess `json:"access"`
	// Plugin / PluginName: the JS plugin that registered it ("" = Site Code).
	Plugin     string `json:"plugin,omitempty"`
	PluginName string `json:"pluginName,omitempty"`
}

type regTaxonomy struct {
	info TaxonomyInfo
	at   Problem
}

const maxTaxonomies = 30

// builtinTaxonomies cannot be registered again.
var builtinTaxonomies = map[string]bool{"category": true, "tag": true, "post_tag": true, "nav_menu": true, "term": true, "taxonomy": true}

// CanManage reports whether role may create, rename and delete the terms.
func (t *TaxonomyInfo) CanManage(role string) bool {
	if role == "administrator" {
		return true
	}
	if role == "" {
		return false
	}
	for _, a := range t.Access.Manage {
		if a == role {
			return true
		}
		if c, ok := capEntry(a); ok && auth.RoleHas(role, c) {
			return true
		}
	}
	return false
}

// For reports whether the taxonomy applies to posts of typ.
func (t *TaxonomyInfo) For(typ string) bool { return slices.Contains(t.PostTypes, typ) }

func (v *vm) siteTaxonomy(c goja.FunctionCall) goja.Value {
	if v.u.plugin() {
		v.need("taxonomies", "site.taxonomy")
	}
	if v.mode == modeHandler {
		v.throw("site.taxonomy ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	id, _ := c.Argument(0).Export().(string)
	what := fmt.Sprintf("site.taxonomy(%q)", id)
	if v.u.plugin() {
		v.pluginTypeID(id, what) // the same "<plugin>_<name>" rule as post types
	}
	if !postTypeRe.MatchString(id) {
		v.throw("%s: ชื่อ taxonomy ต้องเป็น a-z ตัวแรก ตามด้วย a-z 0-9 _ (2-20 ตัว)", what)
	}
	if builtinTaxonomies[id] || builtinTypes[id] {
		v.throw("%s: %q เป็นชื่อของระบบ ใช้ชื่ออื่น", what, id)
	}
	if len(v.taxes) >= maxTaxonomies {
		v.throw("ลงทะเบียน site.taxonomy ได้ไม่เกิน %d ชุด", maxTaxonomies)
	}
	o := optObject(v.rt, c.Argument(1))
	if o == nil {
		v.throw("%s: ต้องระบุ { label, postTypes }", what)
	}
	// slugs have no "_": shop_brand → /shop-brand
	info := TaxonomyInfo{ID: id, Slug: strings.ReplaceAll(id, "_", "-"), Public: true, Access: TaxonomyAccess{Manage: []string{"editor", "administrator"}}}
	if v.u.plugin() {
		info.Plugin, info.PluginName = v.u.ID, orStr(v.u.Name, v.u.ID)
	}
	info.Label = v.i18nLabel(o.Get("label"), id, what)
	info.Singular = v.i18nLabel(o.Get("singular"), info.Label["en"], what)
	info.Singular["th"] = orStr(labelOf(o, "singular", "th"), info.Label["th"])
	if h := o.Get("hierarchical"); h != nil && !goja.IsUndefined(h) {
		info.Hierarchical = h.ToBoolean()
	}
	if p := o.Get("public"); p != nil && !goja.IsUndefined(p) {
		info.Public = p.ToBoolean()
	}
	if s := optString(o, "slug"); s != "" {
		info.Slug = strings.Trim(strings.ToLower(s), "/")
	}
	if !typeSlugRe.MatchString(info.Slug) {
		v.throw("%s: slug ต้องเป็น a-z 0-9 - (ไม่เกิน 41 ตัว)", what)
	}
	var types []string
	if pt := o.Get("postTypes"); pt == nil || goja.IsUndefined(pt) || v.rt.ExportTo(pt, &types) != nil || len(types) == 0 {
		v.throw("%s: postTypes ต้องเป็น array ของประเภทเนื้อหา เช่น ['post'] หรือ ['%s']", what, PluginTypePrefix(orStr(v.u.ID, "shop"))+"product")
	}
	for _, t := range types {
		if t != "post" && t != "page" && !postTypeRe.MatchString(t) {
			v.throw("%s: postTypes: %q ไม่ใช่ชื่อประเภทเนื้อหา", what, t)
		}
		if !slices.Contains(info.PostTypes, t) {
			info.PostTypes = append(info.PostTypes, t)
		}
	}
	if a := optObject(v.rt, o.Get("access")); a != nil {
		if m := a.Get("manage"); m != nil && !goja.IsUndefined(m) && !goja.IsNull(m) {
			var list []string
			if err := v.rt.ExportTo(m, &list); err != nil || len(list) == 0 {
				v.throw("%s: access.manage ต้องเป็น array ของ role เช่น ['editor'] หรือ 'cap:<สิทธิ์>'", what)
			}
			for _, r := range list {
				if !adminEntry(r) {
					v.throw("%s: access.manage: %q ไม่ใช่ role ที่ถูกต้อง (หรือ 'cap:<สิทธิ์>')", what, r)
				}
			}
			v.vetCaps(list, what+": access.manage")
			info.Access.Manage = list
		}
	}
	v.taxes = append(v.taxes, &regTaxonomy{info: info, at: v.caller()})
	return goja.Undefined()
}

// registerTaxonomies validates the taxonomies of a registration run: no
// duplicate ids, no archive slug that is reserved or taken by a post type.
func (p *Program) registerTaxonomies(v *vm, reserved map[string]bool) []Problem {
	var probs []Problem
	seen := map[string]bool{}
	slugs := map[string]string{}
	for _, t := range p.Manifest.PostTypes {
		slugs[t.Slug] = "post type " + t.Type
	}
	for _, t := range v.taxes {
		info := t.info
		switch {
		case seen[info.ID]:
			probs = append(probs, at(t.at, fmt.Sprintf("site.taxonomy %q ซ้ำกับที่ลงทะเบียนไว้แล้ว", info.ID)))
			continue
		case p.postTypes[info.ID] != nil:
			probs = append(probs, at(t.at, fmt.Sprintf("site.taxonomy %q: ชื่อซ้ำกับประเภทเนื้อหา", info.ID)))
			continue
		case info.Public && reserved[info.Slug]:
			probs = append(probs, at(t.at, fmt.Sprintf("site.taxonomy %q: slug /%s ถูกสงวนไว้โดยระบบ", info.ID, info.Slug)))
			continue
		}
		if other, dup := slugs[info.Slug]; dup && info.Public {
			probs = append(probs, at(t.at, fmt.Sprintf("site.taxonomy %q: slug /%s ซ้ำกับ %s", info.ID, info.Slug, other)))
			continue
		}
		seen[info.ID] = true
		slugs[info.Slug] = "taxonomy " + info.ID
		p.Manifest.Taxonomies = append(p.Manifest.Taxonomies, info)
	}
	return probs
}

// Taxonomy returns a registered taxonomy of the manifest (nil for
// "category" and unknown ids).
func (m Manifest) Taxonomy(id string) *TaxonomyInfo {
	for i := range m.Taxonomies {
		if m.Taxonomies[i].ID == id {
			return &m.Taxonomies[i]
		}
	}
	return nil
}

// TaxonomiesOf lists the taxonomy ids that apply to posts of typ: "category"
// for posts (and custom types supporting "categories"), then the registered
// ones naming typ.
func (m Manifest) TaxonomiesOf(typ string) []string {
	var out []string
	if typ == "post" {
		out = append(out, CategoryTaxonomy)
	} else if t := m.PostType(typ); t != nil && slices.Contains(t.Supports, "categories") {
		out = append(out, CategoryTaxonomy)
	}
	for _, t := range m.Taxonomies {
		if t.For(typ) {
			out = append(out, t.ID)
		}
	}
	return out
}

// KnownTaxonomy reports "category" or a registered taxonomy.
func (m Manifest) KnownTaxonomy(id string) bool {
	return id == CategoryTaxonomy || m.Taxonomy(id) != nil
}

// taxSlugFree: a plugin's public taxonomy may not take the archive slug of
// another taxonomy or a post type (the first registered keeps it).
func taxSlugFree(taxes []TaxonomyInfo, types []PostTypeInfo, t TaxonomyInfo) bool {
	if !t.Public {
		return true
	}
	for _, x := range taxes {
		if x.Public && x.Slug == t.Slug {
			return false
		}
	}
	for _, x := range types {
		if x.Slug == t.Slug {
			return false
		}
	}
	return true
}

// TagTaxonomy is the core "tag" taxonomy (2.5.0): flat keywords of posts (and of custom types supporting
// "tags") with archives at /tag/<slug>. Anyone who writes posts may add tags. A taxonomy with the same id or
// slug from Site Code or a plugin keeps it instead.
const TagTaxonomy = "tag"

func withCoreTaxonomies(m Manifest) Manifest {
	for _, t := range m.Taxonomies {
		if t.ID == TagTaxonomy || t.Slug == TagTaxonomy {
			return m
		}
	}
	tag := TaxonomyInfo{ID: TagTaxonomy, Label: map[string]string{"th": "แท็ก", "en": "Tags"},
		Singular: map[string]string{"th": "แท็ก", "en": "Tag"}, PostTypes: []string{"post"}, Slug: TagTaxonomy, Public: true,
		Access: TaxonomyAccess{Manage: []string{CapPrefix + auth.CapEditOwnPosts}}}
	for _, t := range m.PostTypes {
		if slices.Contains(t.Supports, "tags") {
			tag.PostTypes = append(tag.PostTypes, t.Type)
		}
	}
	m.Taxonomies = append(append([]TaxonomyInfo(nil), m.Taxonomies...), tag)
	return m
}
