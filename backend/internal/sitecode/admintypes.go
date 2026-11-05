package sitecode

// Content types made in the admin without code (CMS 2.2.2, Settings → Content types). They are stored as JSON in
// the option content_types and join the manifest exactly like site.postType types: the admin menu and editor
// with their fields, /<slug> and /<slug>/<post> on the site, {{post.meta.x}} in the designer, db.posts in code.
// A type Site Code or a plugin registers under the same id wins; the admin one is then left out.

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// OptContentTypes holds the admin-defined content types ([]TypeSpec as JSON).
const OptContentTypes = "content_types"

// MaxAdminTypes is how many content types the admin may define.
const MaxAdminTypes = 30

// TypeSpec is a content type as the admin form sends and stores it.
type TypeSpec struct {
	Type     string            `json:"type"`
	Label    map[string]string `json:"label"`
	Singular map[string]string `json:"singular,omitempty"`
	Icon     string            `json:"icon,omitempty"`
	Slug     string            `json:"slug"`
	Public   bool              `json:"public"`
	Supports []string          `json:"supports"`
	Fields   []FieldSpec       `json:"fields"`
}

// FieldSpec is one field of a TypeSpec (a repeater holds sub-fields, one level).
type FieldSpec struct {
	Key       string            `json:"key"`
	Type      string            `json:"type"`
	Label     map[string]string `json:"label"`
	Required  bool              `json:"required,omitempty"`
	Min       *float64          `json:"min,omitempty"`
	Max       *float64          `json:"max,omitempty"`
	Options   []FieldOption     `json:"options,omitempty"`
	PostTypes []string          `json:"postTypes,omitempty"`
	Multiple  bool              `json:"multiple,omitempty"`
	Taxonomy  string            `json:"taxonomy,omitempty"`
	Fields    []FieldSpec       `json:"fields,omitempty"`
}

// SpecOf turns a type of the manifest back into the form's shape (editing an admin type).
func SpecOf(t PostTypeInfo) TypeSpec {
	s := TypeSpec{Type: t.Type, Label: t.Label, Singular: t.Singular, Icon: t.Icon, Slug: t.Slug, Public: t.Public, Supports: t.Supports}
	s.Fields = fieldSpecs(t.Fields)
	return s
}

func fieldSpecs(list []FieldInfo) []FieldSpec {
	out := []FieldSpec{}
	for _, f := range list {
		out = append(out, FieldSpec{Key: f.Key, Type: f.Type, Label: f.Label, Required: f.Required, Min: f.Min, Max: f.Max,
			Options: f.Options, PostTypes: f.PostTypes, Multiple: f.Multiple, Taxonomy: f.Taxonomy, Fields: fieldSpecs(f.Fields)})
	}
	return out
}

// label2 cleans a {th, en} label; def fills a missing language.
func label2(in map[string]string, def string, max int) map[string]string {
	th, en := strings.TrimSpace(in["th"]), strings.TrimSpace(in["en"])
	if th == "" {
		th = en
	}
	if en == "" {
		en = th
	}
	if th == "" {
		th, en = def, def
	}
	return map[string]string{"th": clip(th, max), "en": clip(en, max)}
}

// ValidateTypeSpec checks a spec with the rules of site.postType and returns the manifest entry, or an error
// message in Thai for the form.
func ValidateTypeSpec(s TypeSpec) (PostTypeInfo, error) {
	bad := func(f string, a ...any) (PostTypeInfo, error) { return PostTypeInfo{}, fmt.Errorf(f, a...) }
	s.Type = strings.TrimSpace(s.Type)
	if !postTypeRe.MatchString(s.Type) {
		return bad("รหัสประเภทต้องเป็น a-z ตัวแรก ตามด้วย a-z 0-9 _ (2–20 ตัว)")
	}
	if builtinTypes[s.Type] {
		return bad("%q เป็นประเภทของระบบ ใช้รหัสอื่น", s.Type)
	}
	info := PostTypeInfo{Type: s.Type, Icon: clip(strings.TrimSpace(s.Icon), 16), Public: s.Public, Admin: true,
		Access: PostTypeAccess{Edit: []string{"editor", "administrator"}}}
	info.Label = label2(s.Label, s.Type, 60)
	info.Singular = label2(s.Singular, "", 60)
	if s.Singular == nil || (strings.TrimSpace(s.Singular["th"]) == "" && strings.TrimSpace(s.Singular["en"]) == "") {
		info.Singular = map[string]string{"th": info.Label["th"], "en": info.Label["en"]}
	}
	info.Slug = strings.Trim(strings.ToLower(strings.TrimSpace(s.Slug)), "/")
	if info.Slug == "" {
		info.Slug = strings.ReplaceAll(s.Type, "_", "-")
	}
	if !typeSlugRe.MatchString(info.Slug) {
		return bad("ที่อยู่ (slug) ต้องเป็น a-z 0-9 - (ไม่เกิน 41 ตัว)")
	}
	seen := map[string]bool{}
	info.Supports = []string{}
	for _, x := range s.Supports {
		if !supportsAll[x] {
			return bad("ส่วนของหน้าแก้ไข %q ไม่รองรับ (title, content, excerpt, image, categories)", x)
		}
		if !seen[x] {
			seen[x] = true
			info.Supports = append(info.Supports, x)
		}
	}
	if !seen["title"] {
		info.Supports = append([]string{"title"}, info.Supports...) // every post has a title (lists, links, search)
	}
	fields, err := validateFieldSpecs(s.Fields, "", false)
	if err != nil {
		return PostTypeInfo{}, err
	}
	info.Fields = fields
	return info, nil
}

func validateFieldSpecs(list []FieldSpec, parent string, inRepeater bool) ([]FieldInfo, error) {
	out := []FieldInfo{}
	if len(list) > maxFields {
		return nil, fmt.Errorf("มีช่องข้อมูลได้ไม่เกิน %d ช่อง", maxFields)
	}
	seen := map[string]bool{}
	for _, fs := range list {
		k := strings.TrimSpace(fs.Key)
		name := parent + k
		if !metaKeyRe.MatchString(k) {
			return nil, fmt.Errorf("ชื่อช่อง %q ไม่ถูกต้อง (a-z A-Z ตัวแรก ตามด้วย a-z A-Z 0-9 _ ไม่เกิน 40 ตัว)", name)
		}
		if seen[k] {
			return nil, fmt.Errorf("ชื่อช่อง %q ซ้ำกัน", name)
		}
		seen[k] = true
		allowed := metaFieldType
		if inRepeater {
			allowed = repeaterSubType
		}
		if !allowed[fs.Type] {
			return nil, fmt.Errorf("ช่อง %s: ชนิด %q ใช้ไม่ได้", name, fs.Type)
		}
		f := FieldInfo{Key: k, Type: fs.Type, Label: label2(fs.Label, k, 100), Required: fs.Required, Min: fs.Min, Max: fs.Max,
			Multiple: fs.Multiple}
		if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			return nil, fmt.Errorf("ช่อง %s: ค่าต่ำสุดต้องไม่เกินค่าสูงสุด", name)
		}
		for _, o := range fs.Options {
			v := strings.TrimSpace(o.Value)
			if v == "" || len(v) > 200 {
				return nil, fmt.Errorf("ช่อง %s: ตัวเลือกต้องมีค่า (ไม่เกิน 200 ตัวอักษร)", name)
			}
			f.Options = append(f.Options, FieldOption{Value: v, Label: label2(o.Label, v, 100)})
		}
		if len(f.Options) > 100 {
			return nil, fmt.Errorf("ช่อง %s: ตัวเลือกได้ไม่เกิน 100 รายการ", name)
		}
		switch f.Type {
		case "select":
			if len(f.Options) == 0 {
				return nil, fmt.Errorf("ช่อง %s เป็นตัวเลือก ต้องมีตัวเลือกอย่างน้อย 1 รายการ", name)
			}
		case "relation":
			f.PostTypes = fs.PostTypes
			if len(f.PostTypes) == 0 {
				f.PostTypes = []string{"post"}
			}
			for _, t := range f.PostTypes {
				if t != "post" && t != "page" && !postTypeRe.MatchString(t) {
					return nil, fmt.Errorf("ช่อง %s: %q ไม่ใช่ประเภทเนื้อหา", name, t)
				}
			}
		case "term":
			f.Taxonomy = strings.TrimSpace(fs.Taxonomy)
			if f.Taxonomy == "" {
				f.Taxonomy = CategoryTaxonomy
			}
			if f.Taxonomy != CategoryTaxonomy && !postTypeRe.MatchString(f.Taxonomy) {
				return nil, fmt.Errorf("ช่อง %s: ชุดหมวด %q ไม่ถูกต้อง", name, f.Taxonomy)
			}
		case "gallery":
			f.Multiple = true
		case "repeater":
			if inRepeater {
				return nil, fmt.Errorf("ช่อง %s: ซ้อนรายการในรายการไม่ได้", name)
			}
			sub, err := validateFieldSpecs(fs.Fields, name+".", true)
			if err != nil {
				return nil, err
			}
			if len(sub) == 0 || len(sub) > 20 {
				return nil, fmt.Errorf("ช่อง %s เป็นรายการ ต้องมีช่องย่อย 1–20 ช่อง", name)
			}
			f.Fields = sub
		}
		out = append(out, f)
	}
	return out, nil
}

// ParseTypeSpecs reads the option (invalid entries are skipped: the admin form never stores them).
func ParseTypeSpecs(raw string) []TypeSpec {
	var list []TypeSpec
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &list) != nil {
		return nil
	}
	return list
}

var adminTypesCache struct {
	sync.Mutex
	raw   string
	types []PostTypeInfo
}

// AdminTypes returns the validated admin-defined types of the option value (cached by its text).
func AdminTypes(raw string) []PostTypeInfo {
	adminTypesCache.Lock()
	defer adminTypesCache.Unlock()
	if raw == adminTypesCache.raw && adminTypesCache.types != nil {
		return adminTypesCache.types
	}
	out := []PostTypeInfo{}
	for _, s := range ParseTypeSpecs(raw) {
		if t, err := ValidateTypeSpec(s); err == nil {
			out = append(out, t)
		}
	}
	adminTypesCache.raw, adminTypesCache.types = raw, out
	return out
}

// withAdminTypes adds the admin-defined types a code type does not already take (same id or slug).
func withAdminTypes(m Manifest, raw string) Manifest {
	admin := AdminTypes(raw)
	if len(admin) == 0 {
		return m
	}
	types := append([]PostTypeInfo(nil), m.PostTypes...)
	for _, t := range admin {
		if typeTaken(types, t.Type, t.Slug) == "" {
			types = append(types, t)
		}
	}
	m.PostTypes = types
	return m
}
