package sitecode

// Phase 7 joints (generic, found while porting the built-in plugins to JS):
//
//   - site.settings(key) in a plugin also reads its manifest settings
//     (Plugins → plugin → Settings, plugin_packages.settings); precedence:
//     1. the saved manifest value (a text value only when not empty),
//     2. Site code → Settings, key "<id>.<key>" (script_settings),
//     3. the manifest default.
//     site.settings() without a key returns every value as an object.
//   - site.saveSettings(values) updates the calling plugin's manifest
//     settings from an admin page (render, action, download) as a user with
//     manage_options; validated against the manifest, audited.
//   - site.info() → {title, tagline, url, language, timezone, adminEmail?}
//     (adminEmail for Site Code and plugins with the "mail" scope).
//   - site.fields(type, fields) in a plugin (scope "content-fields"): post
//     meta keys "<id>.<key>", shown by the editor in a panel per plugin;
//     site.meta(postId) / site.setMeta(postId, values) read and write the
//     plugin's own fields.
//   - site.adminNotice(fn, {access}) and site.dashboardWidget(id, spec)
//     (scope "admin-pages"): admin-wide notices and dashboard cards.
//   - http.webhook(url, event, data, {secret}) (scope "webhooks").
//   - code.eventAliases: a plugin's own event also delivered under another
//     name (form.submitted for forms.submitted).
//   - route / middleware result {page: {title, html}} (response.page(...)):
//     the HTML inside the site's header and footer.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// packageHost is the plugin manager's settings joint (plugin.Manager).
type packageHost interface {
	PackageSettings(id string) ([]plugin.Field, plugin.Values, bool)
	MergePackageSettings(id string, patch map[string]any) (plugin.Values, error)
}

// pageHost renders HTML inside the site's layout (plugin.Manager → site).
type pageHost interface {
	RenderPage(w http.ResponseWriter, status int, title, body string) bool
}

func (e *Engine) pkgHost() packageHost {
	if e == nil || e.cfg.Plugins == nil {
		return nil
	}
	if ph, ok := e.cfg.Plugins.(packageHost); ok {
		return ph
	}
	return nil
}

// ---------------------------------------------------------------- site.settings

// manifestSetting returns the manifest field key of the unit's package and
// its saved value (nil field: not a manifest setting).
func (e *Engine) manifestSetting(u *unitInfo, key string) (*plugin.Field, any) {
	if !u.plugin() {
		return nil, nil
	}
	ph := e.pkgHost()
	if ph == nil {
		return nil, nil
	}
	schema, vals, ok := ph.PackageSettings(u.ID)
	if !ok {
		return nil, nil
	}
	for i := range schema {
		if schema[i].Key == key {
			return &schema[i], vals[key]
		}
	}
	return nil, nil
}

// savedValue: a saved manifest value counts unless it is an empty text.
func savedValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	}
	return true
}

// settingValue resolves site.settings(key) for v's unit (see the file
// comment for the precedence).
func (v *vm) settingValue(key string) (any, bool) {
	e := v.h.e
	if e == nil || e.st == nil {
		return nil, false
	}
	if !v.u.plugin() && e.pluginKey(key) != "" {
		return nil, false // a JS plugin's setting: not readable by Site Code
	}
	field, saved := e.manifestSetting(v.u, key)
	if field != nil && savedValue(saved) {
		return saved, true
	}
	vals, err := e.settingValues()
	if err != nil {
		v.fail("site.settings: อ่านค่าไม่สำเร็จ: %s", err.Error())
	}
	if s, ok := vals[v.u.settingKey(key)]; ok {
		return s, true
	}
	if field != nil {
		return plugin.Defaults([]plugin.Field{*field})[key], true
	}
	return nil, false
}

// allSettings is site.settings() without a key.
func (v *vm) allSettings() map[string]any {
	out := map[string]any{}
	e := v.h.e
	if e == nil || e.st == nil {
		return out
	}
	vals, err := e.settingValues()
	if err != nil {
		v.fail("site.settings: อ่านค่าไม่สำเร็จ: %s", err.Error())
	}
	for k, s := range vals {
		if v.u.plugin() {
			if local, ok := strings.CutPrefix(k, v.u.ID+"."); ok {
				out[local] = s
			}
		} else if e.pluginKey(k) == "" {
			out[k] = s
		}
	}
	if ph := e.pkgHost(); ph != nil && v.u.plugin() {
		if schema, saved, ok := ph.PackageSettings(v.u.ID); ok {
			for _, f := range schema {
				if savedValue(saved[f.Key]) {
					out[f.Key] = saved[f.Key]
				} else if _, ok := out[f.Key]; !ok {
					out[f.Key] = plugin.Defaults([]plugin.Field{f})[f.Key]
				}
			}
		}
	}
	return out
}

func (v *vm) siteSettings(c goja.FunctionCall) goja.Value {
	v.handlerOnly("site.settings")
	arg := c.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		return v.toJS(v.allSettings())
	}
	key, _ := arg.Export().(string)
	if !settingKeyRe.MatchString(key) {
		v.throw("site.settings: ชื่อค่าไม่ถูกต้อง")
	}
	val, ok := v.settingValue(key)
	if !ok {
		return goja.Null()
	}
	if s, isStr := val.(string); isStr {
		return v.rt.ToValue(s)
	}
	return v.toJS(val)
}

// ---------------------------------------------------------------- site.saveSettings

func (v *vm) siteSaveSettings(c goja.FunctionCall) goja.Value {
	const what = "site.saveSettings"
	v.handlerOnly(what)
	if !v.u.plugin() {
		v.throw("%s ใช้ได้เฉพาะปลั๊กอิน (ค่าของ Site Code แก้ที่หน้า Site code → ค่าลับและการตั้งค่า)", what)
	}
	if v.sess == nil || !v.sess.admin {
		v.fail("%s ใช้ได้เฉพาะในหน้าแอดมินของปลั๊กอิน (site.adminPage: render, action ที่หน้าเรียก หรือ download)", what)
	}
	if v.user == nil || !v.user.Can(auth.CapManageOptions) {
		v.fail("%s: ต้องเป็นผู้ที่แก้การตั้งค่าปลั๊กอินได้ (manage_options)", what)
	}
	ph := v.h.e.pkgHost()
	if ph == nil {
		v.fail("%s: ไม่พร้อมใช้งาน", what)
	}
	s, ok, err := v.toJSON(c.Argument(0))
	var patch map[string]any
	if err != nil || !ok || json.Unmarshal([]byte(s), &patch) != nil || patch == nil {
		v.throw("%s: ต้องส่ง object เช่น { titleTemplate: '%%title%% | %%site%%' }", what)
	}
	vals, err := ph.MergePackageSettings(v.u.ID, patch)
	if err != nil {
		var ve plugin.ValidationError
		if errors.As(err, &ve) {
			keys := make([]string, 0, len(ve))
			for k, m := range ve {
				keys = append(keys, k+": "+m)
			}
			sort.Strings(keys)
			v.throw("%s: ค่าไม่ถูกต้อง — %s", what, strings.Join(keys, "; "))
		}
		v.fail("%s: บันทึกไม่สำเร็จ: %s", what, err.Error())
	}
	keys := make([]string, 0, len(patch))
	for k := range patch {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	_ = v.h.e.DB.Audit(v.user.ID, "plugin_settings", clip(v.u.ID+": "+strings.Join(keys, ", "), 500))
	v.h.e.bumpGen(v.u.ID)
	return v.toJS(vals)
}

// ---------------------------------------------------------------- site.info

func (v *vm) siteInfo(goja.FunctionCall) goja.Value {
	v.handlerOnly("site.info")
	out := map[string]any{"title": "", "tagline": "", "url": "", "language": "th", "timezone": "Asia/Bangkok"}
	e := v.h.e
	if e == nil || e.st == nil {
		return v.toJS(out)
	}
	opts, err := e.st.Options()
	if err != nil {
		v.fail("site.info: อ่านค่าไม่สำเร็จ")
	}
	out["title"], out["tagline"] = opts["site_title"], opts["tagline"]
	if l := strings.TrimSpace(opts["language"]); l != "" {
		out["language"] = l
	}
	if tz := strings.TrimSpace(opts["timezone"]); tz != "" {
		out["timezone"] = tz
	}
	base := strings.TrimRight(strings.TrimSpace(opts["site_url"]), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = ""
		if r := v.req; r != nil && r.Host != "" && r.Host != "site.local" {
			base = plugin.RequestScheme(r) + "://" + plugin.RequestHost(r)
		}
	}
	out["url"] = base
	if v.u.can("mail") {
		out["adminEmail"] = strings.TrimSpace(opts["admin_email"])
	}
	return v.toJS(out)
}

// ---------------------------------------------------------------- plugin fields: site.meta / site.setMeta

// ownFields are the unit's custom fields of typ (plugin keys "<id>.<key>").
func (v *vm) ownFields(typ string) []FieldInfo {
	var out []FieldInfo
	for _, f := range v.pfields {
		if f.typ == typ {
			out = append(out, f.fields...)
		}
	}
	return out
}

// localKey strips the unit's "<id>." prefix from a field key.
func (v *vm) localKey(k string) string {
	if v.u.plugin() {
		return strings.TrimPrefix(k, v.u.ID+".")
	}
	return k
}

// metaPost reads the post of site.meta / site.setMeta; read (site.meta) may
// use the post core read ahead for the render (Session.PreloadPost).
func (v *vm) metaPost(c goja.FunctionCall, what string, read bool) (int64, []FieldInfo, map[string]any, string, int64) {
	v.handlerOnly(what)
	v.need("content-fields", what)
	id := c.Argument(0).ToInteger()
	if id <= 0 {
		v.throw("%s: ต้องระบุรหัสโพสต์", what)
	}
	if v.h.posts == nil {
		v.fail("%s: ไม่พร้อมใช้งาน", what)
	}
	var p *model.Post
	if read {
		p = v.sess.preloaded(id)
	}
	if p == nil {
		var err error
		if p, err = v.h.posts.GetPost(id); err != nil || p == nil {
			return 0, nil, nil, "", 0
		}
	}
	return p.ID, v.ownFields(p.Type), p.Meta, p.Type, p.AuthorID
}

func (v *vm) siteMeta(c goja.FunctionCall) goja.Value {
	id, fields, meta, _, _ := v.metaPost(c, "site.meta", true)
	if id == 0 {
		return goja.Null()
	}
	out := map[string]any{}
	for _, f := range fields {
		if x, ok := meta[f.Key]; ok {
			out[v.localKey(f.Key)] = x
		}
	}
	return v.toJS(out)
}

func (v *vm) siteSetMeta(c goja.FunctionCall) goja.Value {
	const what = "site.setMeta"
	id, fields, meta, typ, author := v.metaPost(c, what, false)
	if id == 0 {
		v.fail("%s: ไม่พบโพสต์นี้", what)
	}
	if len(fields) == 0 {
		v.fail("%s: โค้ดนี้ไม่ได้ลงทะเบียนฟิลด์ของประเภท %q (site.fields)", what, typ)
	}
	if !v.system && !v.mayEdit(typ, author) {
		v.fail("%s: ผู้ใช้นี้ไม่มีสิทธิ์แก้โพสต์ #%d", what, id)
	}
	s, ok, err := v.toJSON(c.Argument(1))
	var in map[string]any
	if err != nil || !ok || json.Unmarshal([]byte(s), &in) != nil || in == nil {
		v.throw("%s: ต้องส่ง object ของฟิลด์ เช่น { title: '…' }", what)
	}
	full := map[string]any{}
	for k, x := range in {
		if v.u.plugin() && !strings.HasPrefix(k, v.u.ID+".") {
			k = v.u.ID + "." + k
		}
		full[k] = x
	}
	clean, err := ValidateMeta(fields, full, meta)
	if err != nil {
		v.fail("%s: %s", what, err.Error())
	}
	cs, ok := v.h.posts.(ContentStore)
	if !ok || cs == nil {
		v.fail("%s: เขียนข้อมูลไม่ได้ในสภาพแวดล้อมนี้", what)
	}
	v.sess.forgetPost(id)
	if err := cs.SetPostMeta(id, clean); err != nil {
		v.fail("%s: บันทึกไม่สำเร็จ: %s", what, err.Error())
	}
	v.h.e.bumpGen(v.u.ID)
	out := map[string]any{}
	for _, f := range fields {
		x, changed := clean[f.Key]
		if !changed {
			x = meta[f.Key]
		}
		if x != nil {
			out[v.localKey(f.Key)] = x
		}
	}
	return v.toJS(out)
}

// WithUnitFields adds what running JS plugins register for content to
// manifest m: their post types (site.postType, scope "post-types", since
// 2.0.5) and their custom fields (site.fields, keys "<id>.<key>") of post,
// page and every custom type; m itself is not changed. A plugin type whose
// id or slug is already taken in m is skipped (activation refuses it, but m
// may be a draft).
func (e *Engine) WithUnitFields(m Manifest) Manifest {
	return withUnits(m, e.units().loaded())
}

func withUnits(m Manifest, units []*liveProg) Manifest {
	var withTypes, withFields []*liveProg
	for _, lp := range units {
		if len(lp.prog.Manifest.PostTypes) > 0 {
			withTypes = append(withTypes, lp)
		}
		if len(lp.prog.fields) > 0 {
			withFields = append(withFields, lp)
		}
	}
	if len(withTypes) == 0 && len(withFields) == 0 {
		return m
	}
	if len(withTypes) > 0 {
		types := append([]PostTypeInfo(nil), m.PostTypes...)
		for _, lp := range withTypes {
			for _, t := range lp.prog.Manifest.PostTypes {
				if typeTaken(types, t.Type, t.Slug) == "" {
					types = append(types, t)
				}
			}
		}
		m.PostTypes = types
	}
	fields := map[string][]FieldInfo{}
	for k, list := range m.Fields {
		fields[k] = append([]FieldInfo(nil), list...)
	}
	for _, lp := range withFields {
		types := make([]string, 0, len(lp.prog.fields))
		for t := range lp.prog.fields {
			types = append(types, t)
		}
		sort.Strings(types)
		for _, t := range types {
			if t != "post" && t != "page" && m.PostType(t) == nil {
				continue
			}
			fields[t] = append(fields[t], lp.prog.fields[t]...)
		}
	}
	m.Fields = fields
	return m
}

// typeTaken returns the type of list that has id or slug ("" = free).
func typeTaken(list []PostTypeInfo, id, slug string) string {
	for _, t := range list {
		if t.Type == id || t.Slug == slug {
			return t.Type
		}
	}
	return ""
}

func typeOwner(t PostTypeInfo) string {
	if t.Plugin != "" {
		return "ปลั๊กอิน " + t.Plugin
	}
	return "Site Code"
}

// unitTypeConflicts checks the post types of plugin unit lp against Site
// Code's live types (site), the other loaded plugins (others) and the URL
// prefixes other installed plugins declare (code.routes / service public).
func (e *Engine) unitTypeConflicts(lp *liveProg, site Manifest, others []*liveProg) []Problem {
	types := lp.prog.Manifest.PostTypes
	if len(types) == 0 {
		return nil
	}
	var probs []Problem
	taken := append([]PostTypeInfo(nil), site.PostTypes...)
	for _, o := range others {
		if o.unit.ID != lp.unit.ID {
			taken = append(taken, o.prog.Manifest.PostTypes...)
		}
	}
	declared := map[string]string{}
	if e.cfg.Plugins != nil {
		for _, r := range e.cfg.Plugins.CodeUnits() {
			if r.ID == lp.unit.ID {
				continue
			}
			for _, p := range r.Code.Routes {
				first, _, _ := strings.Cut(strings.TrimPrefix(p, "/"), "/")
				if first = strings.ToLower(first); first != "" && declared[first] == "" {
					declared[first] = r.ID
				}
			}
		}
	}
	for _, t := range types {
		for _, o := range taken {
			switch {
			case o.Type == t.Type:
				probs = append(probs, Problem{Message: fmt.Sprintf("site.postType %q: มีประเภทนี้แล้ว (ลงทะเบียนโดย %s)", t.Type, typeOwner(o))})
			case o.Slug == t.Slug:
				probs = append(probs, Problem{Message: fmt.Sprintf("site.postType %q: slug /%s ซ้ำกับประเภท %q ของ %s", t.Type, t.Slug, o.Type, typeOwner(o))})
			default:
				continue
			}
			break
		}
		if id := declared[t.Slug]; id != "" {
			probs = append(probs, Problem{Message: fmt.Sprintf("site.postType %q: slug /%s เป็น URL ของปลั๊กอิน %s", t.Type, t.Slug, id)})
		}
	}
	return probs
}

// siteTypeConflicts checks Site Code's post types (m) against the types of
// the loaded plugins (publish / restore).
func siteTypeConflicts(m Manifest, units []*liveProg) []Problem {
	var probs []Problem
	for _, lp := range units {
		for _, pt := range lp.prog.Manifest.PostTypes {
			for _, t := range m.PostTypes {
				switch {
				case t.Type == pt.Type:
					probs = append(probs, Problem{Message: fmt.Sprintf("site.postType %q: ปลั๊กอิน %s ลงทะเบียนประเภทนี้ไว้แล้ว", t.Type, lp.unit.ID)})
				case t.Slug == pt.Slug:
					probs = append(probs, Problem{Message: fmt.Sprintf("site.postType %q: slug /%s ซ้ำกับประเภท %q ของปลั๊กอิน %s", t.Type, t.Slug, pt.Type, lp.unit.ID)})
				}
			}
		}
	}
	return probs
}

// ---------------------------------------------------------------- admin notices

// NoticeInfo is one admin-wide notice (GET /api/v1/code/admin-notices).
type NoticeInfo struct {
	ID    string      `json:"id"`
	Unit  string      `json:"unit"`
	Level string      `json:"level"`
	Text  string      `json:"text"`
	Link  *NoticeLink `json:"link,omitempty"`
}

// NoticeLink is the optional button of a notice.
type NoticeLink struct {
	Label    string `json:"label"`
	Href     string `json:"href"`
	External bool   `json:"external"`
}

type regNotice struct {
	fn     goja.Callable
	access []string
	at     Problem
}

const (
	maxNotices    = 5
	maxWidgets    = 10
	noticeTimeout = 300 * time.Millisecond
)

var noticeLevels = map[string]bool{"info": true, "warning": true, "danger": true, "success": true}

// accessList reads {access: [...]} (role slugs, "cap:<capability>") with default def.
func (v *vm) accessList(o *goja.Object, def []string, what string) []string {
	if o == nil {
		return def
	}
	a := o.Get("access")
	if a == nil || goja.IsUndefined(a) || goja.IsNull(a) {
		return def
	}
	var list []string
	if err := v.rt.ExportTo(a, &list); err != nil || len(list) == 0 {
		v.throw("%s: access ต้องเป็น array ของ role เช่น ['editor']", what)
	}
	for _, r := range list {
		if !adminEntry(r) {
			v.throw("%s: access: %q ไม่ใช่ role ที่ถูกต้อง (หรือ 'cap:<สิทธิ์>')", what, r)
		}
	}
	v.vetCaps(list, what)
	return list
}

func (v *vm) siteAdminNotice(c goja.FunctionCall) goja.Value {
	const what = "site.adminNotice"
	if v.mode == modeHandler {
		v.throw("%s ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler", what)
	}
	v.need("admin-pages", what)
	fn, ok := goja.AssertFunction(c.Argument(0))
	if !ok {
		v.throw("%s: ต้องส่งฟังก์ชัน (ctx) => ({ level, text, link? }) | null", what)
	}
	if len(v.notices) >= maxNotices {
		v.throw("ลงทะเบียน %s ได้ไม่เกิน %d รายการ", what, maxNotices)
	}
	acc := v.accessList(optObject(v.rt, c.Argument(1)), []string{"administrator"}, what)
	v.notices = append(v.notices, &regNotice{fn: fn, access: acc, at: v.caller()})
	return goja.Undefined()
}

// localized picks the user's language from a string or {th, en}.
func localized(x any, locale string) string {
	switch t := x.(type) {
	case string:
		return t
	case map[string]any:
		if s, ok := t[locale].(string); ok && s != "" {
			return s
		}
		for _, k := range []string{"th", "en"} {
			if s, ok := t[k].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// noticeHref accepts a site path ("/x", not "//") or an http(s) URL.
func noticeHref(h string) (string, bool, bool) {
	h = strings.TrimSpace(h)
	if len(h) > 2000 || strings.ContainsAny(h, "\r\n\"<> ") {
		return "", false, false
	}
	if strings.HasPrefix(h, "/") && !strings.HasPrefix(h, "//") && !strings.HasPrefix(h, "/\\") {
		return h, false, true
	}
	u, err := url.Parse(h)
	if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil {
		return h, true, true
	}
	return "", false, false
}

// AdminNotices runs every unit's site.adminNotice handlers for user (each
// 300 ms; a failing or empty one adds nothing).
func (e *Engine) AdminNotices(user *User) []NoticeInfo {
	out := []NoticeInfo{}
	s := e.adminSession(nil, user)
	if s == nil {
		return out
	}
	defer s.Close()
	units := []string{}
	for _, lp := range s.plugins().loaded() {
		units = append(units, lp.unit.ID)
	}
	units = append(units, SiteUnit)
	for _, unit := range units {
		p := s.progOf(unit)
		if p == nil {
			continue
		}
		for i, acc := range p.noticeAccess {
			if !allowed(acc, user) {
				continue
			}
			idx := i
			res, err := s.callIn(unit, fmt.Sprintf("adminNotice #%d", idx+1), noticeTimeout, func(v *vm) goja.Callable {
				if idx < len(v.notices) {
					return v.notices[idx].fn
				}
				return nil
			}, func(v *vm) []goja.Value {
				ctx := v.toJS(map[string]any{"locale": s.locale}).ToObject(v.rt)
				_ = ctx.Set("user", v.userValue())
				return []goja.Value{ctx}
			})
			m, ok := res.(map[string]any)
			if err != nil || !ok {
				continue
			}
			text := clip(strings.TrimSpace(localized(m["text"], s.locale)), 500)
			if text == "" {
				continue
			}
			level, _ := m["level"].(string)
			if !noticeLevels[level] {
				level = "info"
			}
			n := NoticeInfo{ID: fmt.Sprintf("%s:%d", unit, idx+1), Unit: unit, Level: level, Text: text}
			if lk, ok := m["link"].(map[string]any); ok {
				href, ext, okh := noticeHref(fmt.Sprint(lk["href"]))
				label := clip(strings.TrimSpace(localized(lk["label"], s.locale)), 80)
				if okh && label != "" {
					n.Link = &NoticeLink{Label: label, Href: href, External: ext}
				}
			}
			out = append(out, n)
		}
	}
	return out
}

// ---------------------------------------------------------------- dashboard widgets

// WidgetInfo is a site.dashboardWidget (GET /api/v1/code/dashboard-widgets).
type WidgetInfo struct {
	ID     string            `json:"id"`
	Label  map[string]string `json:"label"`
	Icon   string            `json:"icon"`
	Access []string          `json:"access"`
	Height int               `json:"height"`
}

type regWidget struct {
	info WidgetInfo
	fn   goja.Callable
	at   Problem
}

func (v *vm) siteDashboardWidget(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.dashboardWidget ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	v.need("admin-pages", "site.dashboardWidget")
	id, _ := c.Argument(0).Export().(string)
	what := fmt.Sprintf("site.dashboardWidget(%q)", id)
	if !bindingNameRe.MatchString(id) {
		v.throw("site.dashboardWidget: id %q ไม่ถูกต้อง (a-z ตัวแรก ตามด้วย a-z A-Z 0-9 _ - ไม่เกิน 63 ตัว)", c.Argument(0).String())
	}
	if len(v.widgets) >= maxWidgets {
		v.throw("ลงทะเบียน site.dashboardWidget ได้ไม่เกิน %d รายการ", maxWidgets)
	}
	o := optObject(v.rt, c.Argument(1))
	if o == nil {
		v.throw("%s: ต้องระบุ { label, render }", what)
	}
	info := WidgetInfo{ID: id, Label: v.i18nLabel(o.Get("label"), id, what), Icon: clip(optString(o, "icon"), 16), Height: 220}
	info.Access = v.accessList(o, []string{"administrator"}, what)
	if h := o.Get("height"); h != nil && !goja.IsUndefined(h) && !goja.IsNull(h) {
		n := int(h.ToInteger())
		if n < 60 || n > 2000 {
			v.throw("%s: height ต้องอยู่ระหว่าง 60 ถึง 2000 (px)", what)
		}
		info.Height = n
	}
	fn := v.fnOpt(o, "render", what)
	v.widgets = append(v.widgets, &regWidget{info: info, fn: fn, at: v.caller()})
	return goja.Undefined()
}

// DashboardWidgets lists the widgets user may see (qualified ids).
func (e *Engine) DashboardWidgets(user *User) []WidgetInfo {
	out := []WidgetInfo{}
	s := e.adminSession(nil, user)
	if s == nil {
		return out
	}
	defer s.Close()
	add := func(u *unitInfo, p *Program) {
		for _, w := range p.widgetList {
			if allowed(w.Access, user) {
				w.ID = u.qualify(w.ID)
				out = append(out, w)
			}
		}
	}
	for _, lp := range s.plugins().loaded() {
		add(lp.unit, lp.prog)
	}
	add(e.siteU, s.sp())
	return out
}

// RenderWidget runs dashboard widget id's render as user (HTML, title).
func (e *Engine) RenderWidget(id string, user *User, r *http.Request) (body, title string, err error) {
	s := e.adminSession(r, user)
	if s == nil {
		return "", "", ErrNoAdminPage
	}
	defer s.Close()
	unit, local := SplitUnit(id)
	p := s.progOf(unit)
	var info *WidgetInfo
	if p != nil {
		for i := range p.widgetList {
			if p.widgetList[i].ID == local {
				info = &p.widgetList[i]
			}
		}
	}
	if info == nil {
		return "", "", ErrNoAdminPage
	}
	if !allowed(info.Access, user) {
		return "", "", ErrAdminPageDenied
	}
	out, err := s.callIn(unit, "dashboardWidget "+local, e.RouteTimeout, func(v *vm) goja.Callable {
		for _, w := range v.widgets {
			if w.info.ID == local {
				return w.fn
			}
		}
		return nil
	}, func(v *vm) []goja.Value {
		return []goja.Value{s.ctxObject(v, map[string]any{"query": queryMap(s.request()), "widget": id}, nil)}
	})
	if err != nil {
		return "", "", err
	}
	title = localized(map[string]any{"th": info.Label["th"], "en": info.Label["en"]}, s.locale)
	switch x := out.(type) {
	case string:
		body = x
	case map[string]any:
		body, _ = x["html"].(string)
	}
	return body, title, nil
}

// ---------------------------------------------------------------- http.webhook

var webhookEventRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,99}$`)

const maxWebhooksPerRun = 10

func (e *Engine) webhookHTTP() *http.Client {
	if e.WebhookClient != nil {
		return e.WebhookClient
	}
	return plugin.NewWebhookClient(5 * time.Second)
}

func (v *vm) httpWebhook(c goja.FunctionCall) goja.Value {
	const what = "http.webhook"
	v.handlerOnly(what)
	v.need("webhooks", what)
	e := v.h.e
	if e == nil || e.cfg.Demo {
		v.fail("%s: ปิดในเว็บทดลอง", what)
	}
	raw, _ := c.Argument(0).Export().(string)
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(raw) > 2000 {
		v.fail("%s: URL ต้องขึ้นต้นด้วย http:// หรือ https:// (ไม่รับ user:password@)", what)
	}
	event, _ := c.Argument(1).Export().(string)
	if !webhookEventRe.MatchString(event) {
		v.throw("%s: ชื่อเหตุการณ์ไม่ถูกต้อง (a-z 0-9 . _ : - ไม่เกิน 100 ตัว)", what)
	}
	body := []byte("null")
	if x := c.Argument(2); !goja.IsUndefined(x) {
		s, ok, err := v.toJSON(x)
		if err != nil || !ok {
			v.throw("%s: data ต้องแปลงเป็น JSON ได้", what)
		}
		body = []byte(s)
	}
	if len(body) > 1<<20 {
		v.fail("%s: data ใหญ่เกิน 1 MB", what)
	}
	secret := ""
	if o := optObject(v.rt, c.Argument(3)); o != nil {
		secret = optString(o, "secret")
	}
	if v.webhooks >= maxWebhooksPerRun {
		v.fail("%s: ส่งได้ไม่เกิน %d ครั้งต่อการทำงานหนึ่งครั้ง (ใช้ jobs สำหรับจำนวนมาก)", what, maxWebhooksPerRun)
	}
	v.webhooks++
	unit, target := v.u.id(), u.String()
	e.events.Add(1)
	go func() {
		defer e.events.Done()
		if err := postWebhook(e.webhookHTTP(), target, event, body, secret); err != nil {
			e.logUnit(unit, "warn", "http.webhook", fmt.Sprintf("ส่ง webhook %s ไปยัง %s ไม่สำเร็จ: %v", event, u.Host, err))
		}
	}()
	v.addLogLine("info", fmt.Sprintf("http.webhook %s → %s", event, u.Host))
	return goja.Undefined()
}

// postWebhook POSTs body like core's package webhooks (X-Anasaki-Event),
// signed with X-Anasaki-Signature: sha256=<hex HMAC-SHA256(secret, body)>
// when a secret is given.
func postWebhook(client *http.Client, u, event string, body []byte, secret string) error {
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AnasakiStudio-Webhook/1.0")
	req.Header.Set("X-Anasaki-Event", event)
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-Anasaki-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := client.Do(req)
	if err != nil {
		if plugin.IsPrivateAddrErr(err) {
			return errors.New("ที่อยู่ภายใน (private / loopback) — ไม่อนุญาต")
		}
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------- event aliases

// emitAlias delivers a plugin's event under its code.eventAliases name too,
// unless another running plugin owns that name's prefix.
func (v *vm) emitAlias(name string, data any) {
	alias := v.u.aliases[name]
	if alias == "" || v.h == nil || v.h.e == nil {
		return
	}
	pre, _, _ := strings.Cut(alias, ".")
	if pre != v.u.ID && v.h.e.units().get(pre) != nil {
		v.addLogLine("warn", fmt.Sprintf("site.emit(%q): ไม่ส่งชื่อแทน %q เพราะเป็นชื่อของปลั๊กอิน %s", name, alias, pre))
		return
	}
	v.h.e.emitCustom(alias, data)
}

// ---------------------------------------------------------------- {page: {title, html}}

// PageResult is a route / middleware answer rendered in the site's layout.
type PageResult struct {
	Title string `json:"title"`
	HTML  string `json:"html"`
}

// isPageShape: {page: {...}} with optional status / headers only.
func isPageShape(o *goja.Object) bool {
	pg := o.Get("page")
	if pg == nil || goja.IsUndefined(pg) || goja.IsNull(pg) {
		return false
	}
	if po, ok := pg.(*goja.Object); !ok || po.ClassName() != "Object" {
		return false
	}
	for _, k := range o.Keys() {
		if k != "page" && k != "status" && k != "headers" {
			return false
		}
	}
	return true
}

// writeResponse answers with res; a page result is wrapped in the site's
// header and footer (a plain document when no renderer is attached).
func (e *Engine) writeResponse(w http.ResponseWriter, r *http.Request, res *Response) {
	if res.Page == nil {
		writeResponse(w, r, res)
		return
	}
	for k, v := range res.Headers {
		if !strings.EqualFold(k, "Content-Type") {
			w.Header().Set(k, v)
		}
	}
	for _, c := range res.Cookies {
		w.Header().Add("Set-Cookie", c)
	}
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	if e.cfg.Plugins != nil {
		if ph, ok := e.cfg.Plugins.(pageHost); ok && ph.RenderPage(w, res.Status, res.Page.Title, res.Page.HTML) {
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(res.Status)
	if r.Method != http.MethodHead {
		_, _ = fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title></head><body>%s</body></html>`,
			htmlEscape(res.Page.Title), res.Page.HTML)
	}
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;").Replace(s)
}

// responsePage is response.page({title, html}) → {page: {...}} (returned as is).
func (v *vm) responsePage(c goja.FunctionCall) goja.Value {
	o := optObject(v.rt, c.Argument(0))
	if o == nil {
		v.throw("response.page: ต้องระบุ { title, html }")
	}
	out := v.rt.NewObject()
	_ = out.Set("page", o)
	if st := c.Argument(1); !goja.IsUndefined(st) && !goja.IsNull(st) {
		_ = out.Set("status", st)
	}
	return out
}

// ---------------------------------------------------------------- cache generations

// bumpGen marks unit's data as changed (sitemap cache, block cache).
func (e *Engine) bumpGen(unit string) {
	if e == nil {
		return
	}
	g, ok := e.gens.Load(unit)
	if !ok {
		g, _ = e.gens.LoadOrStore(unit, new(int64Counter))
	}
	g.(*int64Counter).Add(1)
}

func (e *Engine) gen(unit string) int64 {
	if g, ok := e.gens.Load(unit); ok {
		return g.(*int64Counter).Load()
	}
	return 0
}

// FilterFingerprint changes whenever the output of filter name may change
// for reasons other than core content: the units filtering it (versions,
// manifest settings) or the data they wrote. Core caches (the sitemap) add
// their own content fingerprint.
func (s *Session) FilterFingerprint(name string) string {
	var b strings.Builder
	sp := s.sp()
	fmt.Fprintf(&b, "v%d;", s.version)
	if sp != nil && sp.filterNames[name] {
		fmt.Fprintf(&b, "site:%d:%d;", s.e.gen(SiteUnit), s.e.settingsGen.Load())
	}
	for _, lp := range s.plugins().loaded() {
		if !lp.prog.filterNames[name] {
			continue
		}
		fmt.Fprintf(&b, "%s@%s:%d;", lp.unit.ID, lp.unit.Version, s.e.gen(lp.unit.ID))
		if ph := s.e.pkgHost(); ph != nil {
			if _, vals, ok := ph.PackageSettings(lp.unit.ID); ok {
				j, _ := json.Marshal(vals)
				sum := sha256.Sum256(j)
				b.WriteString(hex.EncodeToString(sum[:6]))
			}
		}
		fmt.Fprintf(&b, ":%d;", s.e.settingsGen.Load())
	}
	return b.String()
}
