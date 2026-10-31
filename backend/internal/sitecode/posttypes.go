package sitecode

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
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
	Min      *float64          `json:"min,omitempty"`
	Max      *float64          `json:"max,omitempty"`
	Options  []FieldOption     `json:"options,omitempty"`
	// Plugin / PluginName: a JS plugin's field (site.fields, phase 7); its
	// key is "<plugin>.<key>" and the editor shows it in that plugin's panel.
	Plugin     string `json:"plugin,omitempty"`
	PluginName string `json:"pluginName,omitempty"`
}

// PostTypeAccess lists the roles that edit a post type.
type PostTypeAccess struct {
	Edit []string `json:"edit"`
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

var (
	postTypeRe    = regexp.MustCompile(`^[a-z][a-z0-9_]{1,19}$`)
	typeSlugRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)
	metaKeyRe     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,39}$`)
	metaFieldType = map[string]bool{"text": true, "textarea": true, "number": true, "bool": true, "date": true, "datetime": true,
		"select": true, "url": true, "image": true, "email": true, "json": true}
	supportsAll = map[string]bool{"title": true, "content": true, "excerpt": true, "image": true, "categories": true}
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
			v.throw("%s: ฟิลด์ %s ต้องมี type เป็น text, textarea, number, bool, date, datetime, select, url, image, email หรือ json", what, k)
		}
		f.Label = map[string]string{"th": k, "en": k}
		if spec != nil {
			f.Label = v.i18nLabel(spec.Get("label"), k, what+"."+k)
			if r := spec.Get("required"); r != nil && !goja.IsUndefined(r) {
				f.Required = r.ToBoolean()
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
		out = append(out, f)
		if len(out) > maxFields {
			v.throw("%s: มีฟิลด์ได้ไม่เกิน %d ฟิลด์", what, maxFields)
		}
	}
	return out
}

func (v *vm) sitePostType(c goja.FunctionCall) goja.Value {
	if v.u.plugin() {
		v.throw("site.postType ใช้ได้เฉพาะ Site Code (ปลั๊กอิน JavaScript ยังลงทะเบียนประเภทเนื้อหาไม่ได้)")
	}
	if v.mode == modeHandler {
		v.throw("site.postType ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	id, _ := c.Argument(0).Export().(string)
	what := fmt.Sprintf("site.postType(%q)", id)
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
		if a := optObject(v.rt, o.Get("access")); a != nil {
			if e := a.Get("edit"); e != nil && !goja.IsUndefined(e) && !goja.IsNull(e) {
				var list []string
				if err := v.rt.ExportTo(e, &list); err != nil || len(list) == 0 {
					v.throw("%s: access.edit ต้องเป็น array ของ role เช่น ['editor']", what)
				}
				for _, r := range list {
					if !accessRe.MatchString(r) {
						v.throw("%s: access.edit: %q ไม่ใช่ role ที่ถูกต้อง", what, r)
					}
				}
				info.Access.Edit = list
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
	if typ != "post" && typ != "page" && !postTypeRe.MatchString(typ) {
		v.throw("%s: ประเภทต้องเป็น 'post', 'page' หรือประเภทที่ลงทะเบียนด้วย site.postType", what)
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
		if f.typ != "post" && f.typ != "page" && p.postTypes[f.typ] == nil && !v.u.plugin() {
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

// CanEditType reports whether role may edit posts of a custom type.
func (t *PostTypeInfo) CanEditType(role string) bool {
	if role == "administrator" {
		return true
	}
	for _, r := range t.Access.Edit {
		if r == role {
			return true
		}
	}
	return false
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
	}
	return nil, bad("ชนิดฟิลด์ไม่รู้จัก")
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
		}{f.Type, f.Label, f.Required, f.Min, f.Max, f.Options, f.Plugin, f.PluginName}
		v, _ := json.Marshal(spec)
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return json.RawMessage(b.String())
}
