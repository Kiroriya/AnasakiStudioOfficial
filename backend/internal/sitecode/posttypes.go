package sitecode

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/kses"
)

// Phase 4a: custom post types (site.postType) and custom fields
// (site.fields), stored in post_meta and edited in the admin.

// FieldOption is one choice of a select field.
type FieldOption struct {
	Value string            `json:"value"`
	Label map[string]string `json:"label"`
}

// FieldInfo is one custom field (the admin's schema form renders it).
type FieldInfo struct {
	Key      string            `json:"key"`
	Type     string            `json:"type"`
	Label    map[string]string `json:"label"`
	Required bool              `json:"required,omitempty"`
	// HideOnSite (2.5.0): the Fields block on the site leaves it out (stock,
	// cost, internal notes); the admin and code still see it. HideOnPage:
	// listed on cards in lists only, not on the post's own page.
	HideOnSite bool          `json:"hideOnSite,omitempty"`
	HideOnPage bool          `json:"hideOnPage,omitempty"`
	Min        *float64      `json:"min,omitempty"`
	Max        *float64      `json:"max,omitempty"`
	Options    []FieldOption `json:"options,omitempty"`
	// 2.1.0 field types: relation (PostTypes, Multiple), user / file
	// (Multiple), term (Taxonomy, Multiple), gallery, repeater (Fields: one
	// level of simple fields; Max = rows), richtext, color.
	PostTypes []string    `json:"postTypes,omitempty"`
	Multiple  bool        `json:"multiple,omitempty"`
	Taxonomy  string      `json:"taxonomy,omitempty"`
	Fields    []FieldInfo `json:"fields,omitempty"`
	// Plugin / PluginName: a JS plugin's field (site.fields, phase 7); its
	// key is "<plugin>.<key>" and the editor shows it in that plugin's panel.
	Plugin     string `json:"plugin,omitempty"`
	PluginName string `json:"pluginName,omitempty"`
}

// PostTypeAccess lists who edits a post type: role slugs and, since 2.0.6,
// "cap:<capability>" entries.
type PostTypeAccess struct {
	Edit []string `json:"edit"`
}

// PostTypeCaps are the capabilities that edit / publish a post type (since
// 2.0.6; a JS plugin names its own, e.g. {edit: "docs.edit", publish:
// "docs.publish"}). Empty = the access list decides (publish = edit).
type PostTypeCaps struct {
	Edit    string `json:"edit,omitempty"`
	Publish string `json:"publish,omitempty"`
}

// PostTypeInfo is a registered site.postType.
type PostTypeInfo struct {
	Type     string            `json:"type"`
	Label    map[string]string `json:"label"`
	Singular map[string]string `json:"singular"`
	Icon     string            `json:"icon"`
	Slug     string            `json:"slug"`
	Public   bool              `json:"public"`
	Supports []string          `json:"supports"`
	Fields   []FieldInfo       `json:"fields"`
	Access   PostTypeAccess    `json:"access"`
	Caps     PostTypeCaps      `json:"caps"`
	// Plugin / PluginName: the JS plugin that registered the type (since
	// 2.0.5, scope "post-types"); "" for Site Code.
	Plugin     string `json:"plugin,omitempty"`
	PluginName string `json:"pluginName,omitempty"`
	// Admin: made in the admin without code (2.2.2, Settings → Content types).
	Admin bool `json:"admin,omitempty"`
}

type regPostType struct {
	info PostTypeInfo
	at   Problem
}

type regFields struct {
	typ    string
	fields []FieldInfo
	at     Problem
}

// AllTypes: site.fields('*', …) adds the fields to every content type — post, page and each type
// registered by Site Code or a plugin, also later ones (2.0.8; e.g. an SEO panel on every type).
const AllTypes = "*"

var (
	postTypeRe    = regexp.MustCompile(`^[a-z][a-z0-9_]{1,19}$`)
	typeSlugRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)
	metaKeyRe     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,39}$`)
	metaFieldType = map[string]bool{"text": true, "textarea": true, "number": true, "bool": true, "date": true, "datetime": true,
		"select": true, "url": true, "image": true, "email": true, "json": true,
		// 2.1.0
		"relation": true, "user": true, "term": true, "file": true, "gallery": true, "repeater": true, "richtext": true, "color": true}
	// repeaterSubType: what a repeater row may hold (no nesting)
	repeaterSubType = map[string]bool{"text": true, "textarea": true, "number": true, "bool": true, "date": true, "datetime": true,
		"select": true, "url": true, "image": true, "email": true, "file": true, "color": true, "relation": true}
	supportsAll = map[string]bool{"title": true, "content": true, "excerpt": true, "image": true, "categories": true, "tags": true}
	// builtinTypes cannot be redefined by site.postType.
	builtinTypes = map[string]bool{"post": true, "page": true, "template": true, "revision": true, "attachment": true,
		"media": true, "menu": true, "nav_menu": true, "component": true, "user": true, "users": true, "category": true,
		"tag": true, "term": true, "comment": true, "option": true, "plugin": true, "setting": true, "settings": true,
		"archive": true, "search": true, "feed": true}
	dateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	dateTimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d{1,9})?)?(Z|[+-]\d{2}:\d{2})?$`)
)

// IsBuiltinType reports a core content type (post, page, template, …).
func IsBuiltinType(t string) bool { return builtinTypes[t] }

const (
	maxPostTypes = 50
	maxFields    = 60
)

// i18nLabel reads a label: a string or {th, en}.
func (v *vm) i18nLabel(x goja.Value, def, what string) map[string]string {
	out := map[string]string{"th": def, "en": def}
	if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
		return out
	}
	switch l := x.Export().(type) {
	case string:
		if strings.TrimSpace(l) != "" {
			out["th"], out["en"] = l, l
		}
	case map[string]any:
		th, _ := l["th"].(string)
		en, _ := l["en"].(string)
		if th == "" {
			th = en
		}
		if en == "" {
			en = th
		}
		if th != "" {
			out["th"], out["en"] = th, en
		}
	default:
		v.throw("%s: label ต้องเป็นข้อความหรือ {th, en}", what)
	}
	for k, s := range out {
		out[k] = clip(s, 100)
	}
	return out
}

// fieldMap reads {key: 'type' | {type, required, label, min, max, options}}.
func (v *vm) fieldMap(x goja.Value, what string) []FieldInfo {
	var out []FieldInfo
	if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
		return out
	}
	o := x.ToObject(v.rt)
	for _, k := range o.Keys() {
		if !metaKeyRe.MatchString(k) {
			v.throw("%s: ชื่อฟิลด์ %q ไม่ถูกต้อง (a-z A-Z ตัวแรก ตามด้วย a-z A-Z 0-9 _ ไม่เกิน 40 ตัว)", what, k)
		}
		f := FieldInfo{Key: k}
		fv := o.Get(k)
		var spec *goja.Object
		if s, ok := fv.Export().(string); ok {
			f.Type = s
		} else if fo := optObject(v.rt, fv); fo != nil {
			spec = fo
			f.Type = optString(fo, "type")
		}
		if !metaFieldType[f.Type] {
			v.throw("%s: ฟิลด์ %s ต้องมี type เป็น text, textarea, richtext, number, bool, date, datetime, select, url, image, email, color, json, relation, user, term, file, gallery หรือ repeater", what, k)
		}
		f.Label = map[string]string{"th": k, "en": k}
		if spec != nil {
			f.Label = v.i18nLabel(spec.Get("label"), k, what+"."+k)
			if r := spec.Get("required"); r != nil && !goja.IsUndefined(r) {
				f.Required = r.ToBoolean()
			}
			if h := spec.Get("hideOnSite"); h != nil && !goja.IsUndefined(h) { // true, or "page" (cards only)
				if s, ok := h.Export().(string); ok && s == "page" {
					f.HideOnPage = true
				} else {
					f.HideOnSite = h.ToBoolean()
				}
			}
			num := func(name string) *float64 {
				x := spec.Get(name)
				if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
					return nil
				}
				n := x.ToFloat()
				return &n
			}
			f.Min, f.Max = num("min"), num("max")
			if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
				v.throw("%s: ฟิลด์ %s: min ต้องไม่เกิน max", what, k)
			}
			if opts := spec.Get("options"); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
				list, ok := v.array(opts)
				if !ok {
					v.throw("%s: ฟิลด์ %s: options ต้องเป็น array", what, k)
				}
				for _, it := range list {
					var opt FieldOption
					if s, ok := it.Export().(string); ok {
						opt = FieldOption{Value: s, Label: map[string]string{"th": s, "en": s}}
					} else if io := optObject(v.rt, it); io != nil {
						opt.Value = optString(io, "value")
						opt.Label = v.i18nLabel(io.Get("label"), opt.Value, what+"."+k)
					}
					if opt.Value == "" || len(opt.Value) > 200 {
						v.throw("%s: ฟิลด์ %s: ตัวเลือกต้องมี value (ไม่เกิน 200 ตัวอักษร)", what, k)
					}
					f.Options = append(f.Options, opt)
				}
			}
		}
		if f.Type == "select" && len(f.Options) == 0 {
			v.throw("%s: ฟิลด์ %s เป็น select ต้องมี options", what, k)
		}
		v.fieldExtras(&f, spec, what)
		out = append(out, f)
		if len(out) > maxFields {
			v.throw("%s: มีฟิลด์ได้ไม่เกิน %d ฟิลด์", what, maxFields)
		}
	}
	return out
}

// fieldExtras reads the options of the 2.1.0 field types.
func (v *vm) fieldExtras(f *FieldInfo, spec *goja.Object, what string) {
	k := f.Key
	if spec != nil {
		if m := spec.Get("multiple"); m != nil && !goja.IsUndefined(m) {
			f.Multiple = m.ToBoolean()
		}
	}
	switch f.Type {
	case "relation":
		if spec != nil {
			if pt := spec.Get("postType"); pt != nil && !goja.IsUndefined(pt) && !goja.IsNull(pt) {
				if one, ok := pt.Export().(string); ok {
					f.PostTypes = []string{one}
				} else if err := v.rt.ExportTo(pt, &f.PostTypes); err != nil {
					v.throw("%s: ฟิลด์ %s: postType ต้องเป็นชื่อประเภทหรือ array", what, k)
				}
			}
		}
		if len(f.PostTypes) == 0 {
			f.PostTypes = []string{"post"}
		}
		for _, t := range f.PostTypes {
			if t != "post" && t != "page" && !postTypeRe.MatchString(t) {
				v.throw("%s: ฟิลด์ %s: postType %q ไม่ใช่ชื่อประเภทเนื้อหา", what, k, t)
			}
		}
	case "term":
		f.Taxonomy = CategoryTaxonomy
		if spec != nil {
			if t := optString(spec, "taxonomy"); t != "" {
				f.Taxonomy = t
			}
		}
		if f.Taxonomy != CategoryTaxonomy && !postTypeRe.MatchString(f.Taxonomy) {
			v.throw("%s: ฟิลด์ %s: taxonomy %q ไม่ถูกต้อง", what, k, f.Taxonomy)
		}
	case "gallery":
		f.Multiple = true
	case "repeater":
		var sub goja.Value
		if spec != nil {
			sub = spec.Get("fields")
		}
		if sub == nil || goja.IsUndefined(sub) || goja.IsNull(sub) {
			v.throw("%s: ฟิลด์ %s เป็น repeater ต้องมี fields เช่น { name: 'text', qty: 'number' }", what, k)
		}
		f.Fields = v.fieldMap(sub, what+"."+k)
		if len(f.Fields) == 0 || len(f.Fields) > 20 {
			v.throw("%s: ฟิลด์ %s: repeater มีได้ 1-20 ฟิลด์ย่อย", what, k)
		}
		for _, s := range f.Fields {
			if !repeaterSubType[s.Type] {
				v.throw("%s: ฟิลด์ %s.%s: ใน repeater ใช้ type %s ไม่ได้", what, k, s.Key, s.Type)
			}
		}
	}
}

func (v *vm) sitePostType(c goja.FunctionCall) goja.Value {
	if v.u.plugin() {
		v.need("post-types", "site.postType")
	}
	if v.mode == modeHandler {
		v.throw("site.postType ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	id, _ := c.Argument(0).Export().(string)
	what := fmt.Sprintf("site.postType(%q)", id)
	if v.u.plugin() {
		v.pluginTypeID(id, what)
	}
	if !postTypeRe.MatchString(id) {
		v.throw("%s: ชื่อประเภทต้องเป็น a-z ตัวแรก ตามด้วย a-z 0-9 _ (2-20 ตัว)", what)
	}
	if builtinTypes[id] {
		v.throw("%s: %q เป็นประเภทของระบบ ใช้ชื่ออื่น", what, id)
	}
	if len(v.ptypes) >= maxPostTypes {
		v.throw("ลงทะเบียน site.postType ได้ไม่เกิน %d ประเภท", maxPostTypes)
	}
	o := optObject(v.rt, c.Argument(1))
	info := PostTypeInfo{Type: id, Slug: id, Public: true, Supports: []string{"title", "content", "excerpt", "image"},
		Access: PostTypeAccess{Edit: []string{"editor", "administrator"}}}
	info.Label = map[string]string{"th": id, "en": id}
	if v.u.plugin() { // slugs have no "_": docs_doc → /docs-doc
		info.Slug = strings.ReplaceAll(id, "_", "-")
		info.Plugin, info.PluginName = v.u.ID, orStr(v.u.Name, v.u.ID)
	}
	if o != nil {
		info.Label = v.i18nLabel(o.Get("label"), id, what)
		info.Singular = v.i18nLabel(o.Get("singular"), info.Label["en"], what)
		info.Singular["th"] = orStr(labelOf(o, "singular", "th"), info.Label["th"])
		info.Icon = clip(optString(o, "icon"), 16)
		if s := optString(o, "slug"); s != "" {
			info.Slug = strings.Trim(strings.ToLower(s), "/")
		}
		if p := o.Get("public"); p != nil && !goja.IsUndefined(p) {
			info.Public = p.ToBoolean()
		}
		if s := o.Get("supports"); s != nil && !goja.IsUndefined(s) && !goja.IsNull(s) {
			var list []string
			if err := v.rt.ExportTo(s, &list); err != nil {
				v.throw("%s: supports ต้องเป็น array เช่น ['title', 'content']", what)
			}
			seen := map[string]bool{}
			info.Supports = []string{}
			for _, x := range list {
				if !supportsAll[x] {
					v.throw("%s: supports %q ไม่รองรับ (title, content, excerpt, image, categories)", what, x)
				}
				if !seen[x] {
					seen[x] = true
					info.Supports = append(info.Supports, x)
				}
			}
		}
		info.Fields = v.fieldMap(o.Get("fields"), what)
		explicit := false
		if a := optObject(v.rt, o.Get("access")); a != nil {
			if e := a.Get("edit"); e != nil && !goja.IsUndefined(e) && !goja.IsNull(e) {
				var list []string
				if err := v.rt.ExportTo(e, &list); err != nil || len(list) == 0 {
					v.throw("%s: access.edit ต้องเป็น array ของ role เช่น ['editor']", what)
				}
				for _, r := range list {
					if !adminEntry(r) {
						v.throw("%s: access.edit: %q ไม่ใช่ role ที่ถูกต้อง (หรือ 'cap:<สิทธิ์>')", what, r)
					}
				}
				v.vetCaps(list, what+": access.edit")
				info.Access.Edit = list
				explicit = true
			}
		}
		// caps: {edit, publish} (2.0.6): the capabilities that edit / publish the type
		if cs := optObject(v.rt, o.Get("caps")); cs != nil {
			for _, k := range cs.Keys() {
				if k != "edit" && k != "publish" {
					v.throw("%s: caps.%s ไม่รองรับ (ใช้ edit, publish)", what, k)
				}
			}
			info.Caps.Edit = v.capOpt(cs, "edit", what)
			info.Caps.Publish = v.capOpt(cs, "publish", what)
			if info.Caps.Edit != "" {
				entry := CapPrefix + info.Caps.Edit
				if !explicit {
					info.Access.Edit = []string{entry} // the roles holding the capability, not a fixed list
				} else if !slices.Contains(info.Access.Edit, entry) {
					info.Access.Edit = append(info.Access.Edit, entry)
				}
			}
		}
	}
	if info.Singular == nil {
		info.Singular = map[string]string{"th": info.Label["th"], "en": info.Label["en"]}
	}
	if !typeSlugRe.MatchString(info.Slug) {
		v.throw("%s: slug ต้องเป็น a-z 0-9 - (ไม่เกิน 41 ตัว)", what)
	}
	if info.Fields == nil {
		info.Fields = []FieldInfo{}
	}
	v.ptypes = append(v.ptypes, &regPostType{info: info, at: v.caller()})
	return goja.Undefined()
}

// PluginTypePrefix is what a JS plugin's post type ids start with: the
// plugin id with "-" as "_", then "_" (plugin "my-shop" → "my_shop_").
func PluginTypePrefix(pluginID string) string {
	return strings.ReplaceAll(pluginID, "-", "_") + "_"
}

// pluginTypeID enforces the type id rule of JS plugins: "<id>_<name>"
// within the 20 characters of a type id.
func (v *vm) pluginTypeID(id, what string) {
	pre := PluginTypePrefix(v.u.ID)
	switch {
	case pre[0] < 'a' || pre[0] > 'z':
		v.throw("%s: ปลั๊กอินที่รหัสขึ้นต้นด้วยตัวเลข (%s) ลงทะเบียนประเภทเนื้อหาไม่ได้ — ชื่อประเภทต้องขึ้นต้นด้วย a-z", what, v.u.ID)
	case len(pre)+1 > 20:
		v.throw("%s: รหัสปลั๊กอิน %q ยาวเกินไปสำหรับชื่อประเภท (ต้องเป็น %s<ชื่อ> รวมไม่เกิน 20 ตัว)", what, v.u.ID, pre)
	case !strings.HasPrefix(id, pre) || len(id) == len(pre):
		v.throw("%s: ชื่อประเภทของปลั๊กอินต้องขึ้นต้นด้วย %q ตามด้วยชื่อ เช่น %s", what, pre, pre+"item")
	case len(id) > 20:
		v.throw("%s: ชื่อประเภทยาวเกิน 20 ตัว (%s + ชื่อ รวมไม่เกิน 20 ตัว)", what, pre)
	}
}

func labelOf(o *goja.Object, key, lang string) string {
	x := o.Get(key)
	if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
		return ""
	}
	switch l := x.Export().(type) {
	case string:
		return l
	case map[string]any:
		s, _ := l[lang].(string)
		return s
	}
	return ""
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (v *vm) siteFields(c goja.FunctionCall) goja.Value {
	if v.u.plugin() {
		v.need("content-fields", "site.fields")
	}
	if v.mode == modeHandler {
		v.throw("site.fields ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	typ, _ := c.Argument(0).Export().(string)
	what := fmt.Sprintf("site.fields(%q)", typ)
	if typ != "post" && typ != "page" && typ != AllTypes && !postTypeRe.MatchString(typ) {
		v.throw("%s: ประเภทต้องเป็น 'post', 'page', '*' (ทุกประเภท) หรือประเภทที่ลงทะเบียนด้วย site.postType", what)
	}
	fields := v.fieldMap(c.Argument(1), what)
	if len(fields) == 0 {
		v.throw("%s: ต้องมีอย่างน้อยหนึ่งฟิลด์", what)
	}
	if v.u.plugin() { // phase 7: the plugin's own namespace in post_meta
		for i := range fields {
			fields[i].Key = v.u.ID + "." + fields[i].Key
			if len(fields[i].Key) > 64 {
				v.throw("%s: ชื่อฟิลด์ %q ยาวเกินไป (รวมรหัสปลั๊กอินแล้วต้องไม่เกิน 64 ตัว)", what, fields[i].Key)
			}
			fields[i].Plugin, fields[i].PluginName = v.u.ID, orStr(v.u.Name, v.u.ID)
		}
	}
	v.pfields = append(v.pfields, &regFields{typ: typ, fields: fields, at: v.caller()})
	return goja.Undefined()
}

// registerContent validates post types and fields of a registration run.
func (p *Program) registerContent(v *vm, reserved map[string]bool) []Problem {
	var probs []Problem
	p.postTypes, p.fields = map[string]*PostTypeInfo{}, map[string][]FieldInfo{}
	slugs := map[string]string{}
	for _, t := range v.ptypes {
		info := t.info
		if _, dup := p.postTypes[info.Type]; dup {
			probs = append(probs, at(t.at, fmt.Sprintf("site.postType %q ซ้ำกับที่ลงทะเบียนไว้แล้ว", info.Type)))
			continue
		}
		if reserved[info.Slug] {
			probs = append(probs, at(t.at, fmt.Sprintf("site.postType %q: slug /%s ถูกสงวนไว้โดยระบบ", info.Type, info.Slug)))
			continue
		}
		if other, dup := slugs[info.Slug]; dup {
			probs = append(probs, at(t.at, fmt.Sprintf("site.postType %q: slug /%s ซ้ำกับประเภท %q", info.Type, info.Slug, other)))
			continue
		}
		slugs[info.Slug] = info.Type
		p.postTypes[info.Type] = &info
	}
	for _, f := range v.pfields {
		// a plugin's fields may target Site Code's custom types (kept when the type exists)
		if f.typ != "post" && f.typ != "page" && f.typ != AllTypes && p.postTypes[f.typ] == nil && !v.u.plugin() {
			probs = append(probs, at(f.at, fmt.Sprintf("site.fields(%q): ไม่มีประเภทนี้ (ลงทะเบียนด้วย site.postType ก่อน)", f.typ)))
			continue
		}
		have := map[string]bool{}
		for _, x := range p.fieldsFor(f.typ) {
			have[x.Key] = true
		}
		for _, x := range f.fields {
			if have[x.Key] {
				probs = append(probs, at(f.at, fmt.Sprintf("site.fields(%q): ฟิลด์ %q ซ้ำ", f.typ, x.Key)))
				continue
			}
			have[x.Key] = true
			p.fields[f.typ] = append(p.fields[f.typ], x)
		}
	}
	for _, t := range v.ptypes {
		if info := p.postTypes[t.info.Type]; info != nil {
			p.Manifest.PostTypes = append(p.Manifest.PostTypes, *info)
		}
	}
	p.Manifest.Fields = map[string][]FieldInfo{}
	for k, list := range p.fields {
		p.Manifest.Fields[k] = list
	}
	return probs
}

// fieldsFor is the custom fields of a type: the post type's own plus
// site.fields additions.
func (p *Program) fieldsFor(typ string) []FieldInfo {
	var out []FieldInfo
	if t := p.postTypes[typ]; t != nil {
		out = append(out, t.Fields...)
	}
	return append(out, p.fields[typ]...)
}

// PostType returns a registered custom post type of the manifest.
func (m Manifest) PostType(typ string) *PostTypeInfo {
	for i := range m.PostTypes {
		if m.PostTypes[i].Type == typ {
			return &m.PostTypes[i]
		}
	}
	return nil
}

// FieldsFor is every custom field of typ (post type fields + site.fields).
func (m Manifest) FieldsFor(typ string) []FieldInfo {
	var out []FieldInfo
	if t := m.PostType(typ); t != nil {
		out = append(out, t.Fields...)
	}
	return append(out, m.Fields[typ]...)
}

// CanEditType reports whether role may edit posts of a custom type: a role
// listed in access.edit, or one holding a "cap:" entry's capability (2.0.6).
func (t *PostTypeInfo) CanEditType(role string) bool {
	if role == "administrator" {
		return true
	}
	if role == "" {
		return false
	}
	for _, a := range t.Access.Edit {
		if a == role {
			return true
		}
		if c, ok := capEntry(a); ok && auth.RoleHas(role, c) {
			return true
		}
	}
	return false
}

// CanPublishType reports whether role may publish posts of a custom type:
// the holders of caps.publish when set (2.0.6), else whoever edits it.
func (t *PostTypeInfo) CanPublishType(role string) bool {
	if t.Caps.Publish == "" {
		return t.CanEditType(role)
	}
	return role == "administrator" || (role != "" && auth.RoleHas(role, t.Caps.Publish))
}

// ---------------------------------------------------------------- validation

// MetaError is a refused custom field value (400 with a Thai message).
type MetaError struct{ Key, Msg string }

func (e *MetaError) Error() string { return e.Msg }

func fieldLabel(f FieldInfo) string {
	if l := f.Label["th"]; l != "" {
		return l
	}
	return f.Key
}

// ValidateMeta checks custom field values against fields: unknown keys are
// dropped, "" / null remove a value, required fields must have a value
// after merging with existing. The result holds the values to write (nil =
// remove).
func ValidateMeta(fields []FieldInfo, in, existing map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for _, f := range fields {
		raw, present := in[f.Key]
		if !present {
			if f.Required {
				if ev, ok := existing[f.Key]; !ok || ev == nil {
					return nil, &MetaError{Key: f.Key, Msg: fmt.Sprintf("กรุณากรอก %s", fieldLabel(f))}
				}
			}
			continue
		}
		val, err := metaValue(f, raw)
		if err != nil {
			return nil, err
		}
		if val == nil && f.Required {
			return nil, &MetaError{Key: f.Key, Msg: fmt.Sprintf("กรุณากรอก %s", fieldLabel(f))}
		}
		out[f.Key] = val
	}
	return out, nil
}

func metaValue(f FieldInfo, raw any) (any, error) {
	raw = normScalar(raw)
	bad := func(msg string) error { return &MetaError{Key: f.Key, Msg: fieldLabel(f) + ": " + msg} }
	if raw == nil {
		return nil, nil
	}
	if s, ok := raw.(string); ok && strings.TrimSpace(s) == "" && f.Type != "json" {
		return nil, nil
	}
	str := func() (string, bool) {
		switch x := raw.(type) {
		case string:
			return x, true
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64), true
		case bool:
			return strconv.FormatBool(x), true
		}
		return "", false
	}
	switch f.Type {
	case "text", "textarea", "email", "url", "date", "datetime", "select":
		s, ok := str()
		if !ok {
			return nil, bad("ต้องเป็นข้อความ")
		}
		if f.Type != "textarea" {
			s = strings.TrimSpace(s)
		}
		limit := 2000
		if f.Type == "textarea" {
			limit = 20000
		}
		n := len([]rune(s))
		if n > limit {
			return nil, bad(fmt.Sprintf("ยาวเกิน %d ตัวอักษร", limit))
		}
		if f.Type == "text" || f.Type == "textarea" {
			if f.Min != nil && float64(n) < *f.Min {
				return nil, bad(fmt.Sprintf("ต้องยาวอย่างน้อย %v ตัวอักษร", *f.Min))
			}
			if f.Max != nil && float64(n) > *f.Max {
				return nil, bad(fmt.Sprintf("ต้องยาวไม่เกิน %v ตัวอักษร", *f.Max))
			}
		}
		switch f.Type {
		case "email":
			if a, err := mail.ParseAddress(s); err != nil || a.Address != s {
				return nil, bad("อีเมลไม่ถูกต้อง")
			}
		case "url":
			u, err := url.Parse(s)
			if err != nil || !((u.Scheme == "http" || u.Scheme == "https") && u.Host != "" || (u.Scheme == "" && strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//"))) {
				return nil, bad("ลิงก์ต้องขึ้นต้นด้วย https:// หรือ /")
			}
		case "date":
			if _, err := time.Parse("2006-01-02", s); err != nil || !dateRe.MatchString(s) {
				return nil, bad("วันที่ต้องอยู่ในรูปแบบ YYYY-MM-DD")
			}
		case "datetime":
			if !dateTimeRe.MatchString(s) {
				return nil, bad("วันเวลาต้องอยู่ในรูปแบบ YYYY-MM-DDTHH:MM")
			}
			if _, err := time.Parse("2006-01-02", s[:10]); err != nil {
				return nil, bad("วันที่ไม่ถูกต้อง")
			}
		case "select":
			found := false
			for _, o := range f.Options {
				found = found || o.Value == s
			}
			if !found {
				return nil, bad("ไม่มีตัวเลือกนี้")
			}
		}
		return s, nil
	case "number":
		var n float64
		switch x := raw.(type) {
		case float64:
			n = x
		case string:
			var err error
			if n, err = strconv.ParseFloat(strings.TrimSpace(x), 64); err != nil {
				return nil, bad("ต้องเป็นตัวเลข")
			}
		default:
			return nil, bad("ต้องเป็นตัวเลข")
		}
		if f.Min != nil && n < *f.Min {
			return nil, bad(fmt.Sprintf("ต้องไม่น้อยกว่า %v", *f.Min))
		}
		if f.Max != nil && n > *f.Max {
			return nil, bad(fmt.Sprintf("ต้องไม่มากกว่า %v", *f.Max))
		}
		return n, nil
	case "bool":
		switch x := raw.(type) {
		case bool:
			return x, nil
		case string:
			return truthyInput(x), nil
		case float64:
			return x != 0, nil
		}
		return nil, bad("ต้องเป็นจริง/เท็จ")
	case "image":
		switch x := raw.(type) {
		case float64:
			if x <= 0 || x != float64(int64(x)) {
				return nil, bad("รหัสรูปไม่ถูกต้อง")
			}
			return x, nil
		case string:
			s := strings.TrimSpace(x)
			u, err := url.Parse(s)
			if err != nil || len(s) > 1000 || !((u.Scheme == "http" || u.Scheme == "https") && u.Host != "" || (u.Scheme == "" && strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//"))) {
				return nil, bad("ลิงก์รูปไม่ถูกต้อง")
			}
			return s, nil
		}
		return nil, bad("ต้องเป็นลิงก์รูปหรือรหัสสื่อ")
	case "json":
		return raw, nil
	case "richtext":
		s, ok := str()
		if !ok {
			return nil, bad("ต้องเป็นข้อความ")
		}
		if len(s) > 100000 {
			return nil, bad("ยาวเกิน 100000 ตัวอักษร")
		}
		return kses.SanitizeHTML(s), nil
	case "color":
		s, ok := str()
		if !ok || !colorRe.MatchString(strings.TrimSpace(s)) {
			return nil, bad("สีต้องเป็น #rrggbb, rgb(), hsl() หรือชื่อสี")
		}
		return strings.TrimSpace(s), nil
	case "relation", "user", "term", "file", "gallery":
		return idsValue(f, raw, bad)
	case "repeater":
		return repeaterValue(f, raw, bad)
	}
	return nil, bad("ชนิดฟิลด์ไม่รู้จัก")
}

var colorRe = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|(rgb|rgba|hsl|hsla)\([0-9.,%\s/]+\)|[a-zA-Z]{3,20}|transparent)$`)

// idsValue: one id (a number) or, for multiple / gallery, a list of ids (≤ 100,
// duplicates dropped, order kept). An empty list removes the value.
func idsValue(f FieldInfo, raw any, bad func(string) error) (any, error) {
	one := func(x any) (float64, bool) {
		switch n := normScalar(x).(type) {
		case float64:
			return n, n > 0 && n == float64(int64(n))
		case string:
			v, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
			return float64(v), err == nil && v > 0
		}
		return 0, false
	}
	if !f.Multiple {
		if list, ok := raw.([]any); ok && len(list) == 1 {
			raw = list[0]
		}
		n, ok := one(raw)
		if !ok {
			return nil, bad("ต้องเป็นรหัส (จำนวนเต็มบวก)")
		}
		return n, nil
	}
	list, ok := raw.([]any)
	if !ok {
		if n, ok := one(raw); ok {
			list = []any{n}
		} else {
			return nil, bad("ต้องเป็นรายการรหัส")
		}
	}
	if len(list) > 100 {
		return nil, bad("เลือกได้ไม่เกิน 100 รายการ")
	}
	out, seen := []any{}, map[float64]bool{}
	for _, x := range list {
		n, ok := one(x)
		if !ok {
			return nil, bad("ต้องเป็นรายการรหัส (จำนวนเต็มบวก)")
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	if f.Max != nil && float64(len(out)) > *f.Max {
		return nil, bad(fmt.Sprintf("เลือกได้ไม่เกิน %v รายการ", *f.Max))
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// repeaterValue: a list of rows (≤ Max, default 100); each row's values are
// checked against the sub-fields (required ones included); empty rows drop.
func repeaterValue(f FieldInfo, raw any, bad func(string) error) (any, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, bad("ต้องเป็นรายการแถว")
	}
	limit := 100.0
	if f.Max != nil {
		limit = *f.Max
	}
	out := []any{}
	for i, r := range list {
		row, ok := r.(map[string]any)
		if !ok {
			return nil, bad(fmt.Sprintf("แถว %d ต้องเป็น object", i+1))
		}
		clean := map[string]any{}
		for _, s := range f.Fields {
			v, err := metaValue(s, row[s.Key])
			if err != nil {
				return nil, bad(fmt.Sprintf("แถว %d: %s", i+1, err.(*MetaError).Msg))
			}
			if v != nil {
				clean[s.Key] = v
			}
		}
		if len(clean) == 0 {
			continue
		}
		for _, s := range f.Fields {
			if _, has := clean[s.Key]; s.Required && !has {
				return nil, bad(fmt.Sprintf("แถว %d: กรุณากรอก %s", i+1, fieldLabel(s)))
			}
		}
		out = append(out, clean)
	}
	if float64(len(out)) > limit {
		return nil, bad(fmt.Sprintf("มีได้ไม่เกิน %v แถว", limit))
	}
	if f.Min != nil && float64(len(out)) < *f.Min {
		return nil, bad(fmt.Sprintf("ต้องมีอย่างน้อย %v แถว", *f.Min))
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// FieldsObject renders fields as the JSON object the admin's schema form
// reads: {"<key>": {type, label, required?, min?, max?, options?}, …} in
// declaration order.
func FieldsObject(list []FieldInfo) json.RawMessage {
	var b strings.Builder
	b.WriteByte('{')
	for i, f := range list {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(f.Key)
		spec := struct {
			Type       string            `json:"type"`
			Label      map[string]string `json:"label"`
			Required   bool              `json:"required,omitempty"`
			Min        *float64          `json:"min,omitempty"`
			Max        *float64          `json:"max,omitempty"`
			Options    []FieldOption     `json:"options,omitempty"`
			Plugin     string            `json:"plugin,omitempty"`
			PluginName string            `json:"pluginName,omitempty"`
			PostTypes  []string          `json:"postTypes,omitempty"`
			Multiple   bool              `json:"multiple,omitempty"`
			Taxonomy   string            `json:"taxonomy,omitempty"`
			Fields     json.RawMessage   `json:"fields,omitempty"`
		}{f.Type, f.Label, f.Required, f.Min, f.Max, f.Options, f.Plugin, f.PluginName, f.PostTypes, f.Multiple, f.Taxonomy, nil}
		if len(f.Fields) > 0 {
			spec.Fields = FieldsObject(f.Fields)
		}
		v, _ := json.Marshal(spec)
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return json.RawMessage(b.String())
}
