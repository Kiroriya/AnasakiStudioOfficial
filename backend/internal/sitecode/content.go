package sitecode

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/kses"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

// Phase 4a: writing content from code (db.posts.create/update/delete,
// db.asSystem) and custom post types / meta filters in db.posts.list.

// ContentStore is what db.posts writes (store.Store).
type ContentStore interface {
	PostStore
	CreatePost(p *model.Post, termIDs []int64) error
	UpdatePost(p *model.Post, termIDs []int64, editorID int64) error
	DeletePost(id int64) error
	UniqueSlug(typ, slug string, exceptID int64) (string, error)
	SetPostMeta(id int64, meta map[string]any) error
}

var _ ContentStore = (*store.Store)(nil)

// customType returns the running code's post type typ (nil for builtins / unknown).
func (v *vm) customType(typ string) *PostTypeInfo {
	for _, t := range v.ptypes {
		if t.info.Type == typ {
			info := t.info
			return &info
		}
	}
	return nil
}

// fieldsFor is the custom fields of typ in the running code.
func (v *vm) fieldsFor(typ string) []FieldInfo {
	var out []FieldInfo
	if t := v.customType(typ); t != nil {
		out = append(out, t.Fields...)
	}
	for _, f := range v.pfields {
		if f.typ == typ || (f.typ == AllTypes && typ != "") {
			out = append(out, f.fields...)
		}
	}
	return out
}

// knownType: post, page or a registered custom type.
func (v *vm) knownType(typ string) bool {
	return typ == model.TypePost || typ == model.TypePage || v.customType(typ) != nil
}

// mayEdit reports whether the caller may write posts of typ (authorID: the
// post's author, 0 for a new post).
func (v *vm) mayEdit(typ string, authorID int64) bool {
	if v.system {
		return true
	}
	if v.user == nil {
		return false
	}
	switch typ {
	case model.TypePage:
		return v.user.Can(auth.CapEditPages)
	case model.TypePost:
		if authorID == 0 || authorID == v.user.ID {
			return v.user.Can(auth.CapEditOwnPosts)
		}
		return v.user.Can(auth.CapEditOthersPosts)
	}
	if t := v.customType(typ); t != nil {
		return t.CanEditType(v.user.Role)
	}
	return false
}

// mayPublish reports whether the caller may publish posts of typ.
func (v *vm) mayPublish(typ string) bool {
	if v.system {
		return true
	}
	if typ == model.TypePost || typ == model.TypePage {
		return v.user.Can(auth.CapPublishPosts)
	}
	if t := v.customType(typ); t != nil && t.Caps.Publish != "" { // 2.0.6
		return v.user != nil && t.CanPublishType(v.user.Role)
	}
	return v.mayEdit(typ, 0)
}

func (v *vm) contentStore() ContentStore {
	v.handlerOnly("db.posts")
	v.need("content-write", "db.posts (เขียน)")
	cs, ok := v.h.posts.(ContentStore)
	if !ok || cs == nil {
		v.throw("db.posts: เขียนเนื้อหาไม่ได้ในสภาพแวดล้อมนี้")
	}
	return cs
}

// postFields is what code may set on a post.
type postFields struct {
	typ, title, slug, status, excerpt, content *string
	meta                                       map[string]any
}

func (v *vm) readPostFields(x goja.Value, what string) postFields {
	o := optObject(v.rt, x)
	if o == nil {
		v.throw("%s: ต้องระบุ object ของโพสต์", what)
	}
	var pf postFields
	str := func(k string) *string {
		val := o.Get(k)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			return nil
		}
		s := val.String()
		return &s
	}
	pf.typ, pf.title, pf.slug, pf.status = str("type"), str("title"), str("slug"), str("status")
	pf.excerpt, pf.content = str("excerpt"), str("content")
	if m := o.Get("meta"); m != nil && !goja.IsUndefined(m) && !goja.IsNull(m) {
		s, ok, err := v.toJSON(m)
		if err != nil || !ok || jsonUnmarshal(s, &pf.meta) != nil {
			v.throw("%s: meta ต้องเป็น object", what)
		}
	}
	return pf
}

// postValueMap is the post.beforeSave value of p.
func postValueMap(p *model.Post, meta map[string]any) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	return map[string]any{"type": p.Type, "title": p.Title, "slug": p.Slug, "status": p.Status,
		"excerpt": p.Excerpt, "content": p.Content, "meta": meta}
}

// beforeSaveNested runs post.beforeSave inside the handler; it throws when
// a filter refuses the save ({error}).
func (v *vm) beforeSaveNested(p *model.Post, meta map[string]any, isNew bool, what string) map[string]any {
	if len(v.filtersFor("post.beforeSave")) == 0 {
		return meta
	}
	ctx := v.toJS(map[string]any{"isNew": isNew}).ToObject(v.rt)
	_ = ctx.Set("user", v.userValue())
	v.defineVisitor(ctx)
	out := v.applyNested("post.beforeSave", v.toJS(postValueMap(p, meta)), ctx)
	s, ok, err := v.toJSON(out)
	var m map[string]any
	if err != nil || !ok || jsonUnmarshal(s, &m) != nil || m == nil {
		v.fail("%s: post.beforeSave ต้องคืน object ของโพสต์", what)
	}
	if e, ok := m["error"].(string); ok && e != "" {
		v.fail("%s", e)
	}
	applyPostMap(p, m)
	if mm, ok := m["meta"].(map[string]any); ok {
		return mm
	}
	return meta
}

// applyPostMap copies title/slug/status/excerpt/content from a filter result.
func applyPostMap(p *model.Post, m map[string]any) {
	if s, ok := m["title"].(string); ok {
		p.Title = strings.TrimSpace(s)
	}
	if s, ok := m["slug"].(string); ok {
		p.Slug = s
	}
	if s, ok := m["status"].(string); ok && (s == model.StatusDraft || s == model.StatusPublish) {
		p.Status = s
	}
	if s, ok := m["excerpt"].(string); ok {
		p.Excerpt = s
	}
	if s, ok := m["content"].(string); ok {
		p.Content = s
	}
}

func (v *vm) sanitizeFor(p *model.Post) {
	if !v.system && !v.user.Can(auth.CapUnfilteredHTML) {
		p.Content = kses.SanitizeHTML(p.Content)
	}
}

func (v *vm) emitPost(event string, p *model.Post) {
	if v.h.e == nil {
		return
	}
	url := ""
	if v.h.postURL != nil {
		url = v.h.postURL(p)
	}
	payload := map[string]any{"id": p.ID, "type": p.Type, "title": p.Title, "slug": p.Slug, "status": p.Status, "url": url}
	v.h.e.emitCore(event, payload)
}

func (v *vm) postsCreate(c goja.FunctionCall) goja.Value {
	const what = "db.posts.create"
	cs := v.contentStore()
	pf := v.readPostFields(c.Argument(0), what)
	typ := model.TypePost
	if pf.typ != nil {
		typ = *pf.typ
	}
	if !v.knownType(typ) {
		v.throw("%s: ไม่มีประเภท %q", what, typ)
	}
	if !v.mayEdit(typ, 0) {
		v.fail("%s: ไม่มีสิทธิ์สร้างเนื้อหาประเภทนี้ (ผู้เยี่ยมชมเขียนเนื้อหาไม่ได้ — ใช้ db.asSystem เมื่อโค้ดทำแทนระบบ)", what)
	}
	p := &model.Post{Type: typ, Status: model.StatusDraft}
	if v.system {
		p.AuthorID = v.h.e.systemAuthor()
	} else {
		p.AuthorID = v.user.ID
	}
	v.applyFields(p, pf)
	meta := v.writeMeta(p, pf.meta, nil, true, what)
	return v.savePost(cs, p, meta, true, "", what)
}

func (v *vm) applyFields(p *model.Post, pf postFields) {
	if pf.title != nil {
		p.Title = strings.TrimSpace(*pf.title)
	}
	if pf.slug != nil {
		p.Slug = *pf.slug
	}
	if pf.status != nil {
		if *pf.status != model.StatusDraft && *pf.status != model.StatusPublish {
			v.throw("db.posts: status ต้องเป็น 'draft' หรือ 'publish'")
		}
		p.Status = *pf.status
	}
	if pf.excerpt != nil {
		p.Excerpt = *pf.excerpt
	}
	if pf.content != nil {
		p.Content = *pf.content
	}
}

// writeMeta validates the meta patch against the type's fields.
func (v *vm) writeMeta(p *model.Post, in, existing map[string]any, isNew bool, what string) map[string]any {
	fields := v.fieldsFor(p.Type)
	if in == nil {
		in = map[string]any{}
	}
	if len(fields) == 0 {
		return map[string]any{}
	}
	clean, err := ValidateMeta(fields, in, existing)
	if err != nil {
		v.fail("%s: %s", what, err.Error())
	}
	return clean
}

func (v *vm) savePost(cs ContentStore, p *model.Post, meta map[string]any, isNew bool, was, what string) goja.Value {
	meta = v.beforeSaveNested(p, meta, isNew, what)
	if p.Status == model.StatusPublish && !v.mayPublish(p.Type) {
		p.Status = model.StatusDraft
	}
	if p.Title == "" {
		p.Title = "(ไม่มีชื่อ)"
	}
	v.sanitizeFor(p)
	slug := p.Slug
	if slug == "" {
		slug = p.Title
	}
	var err error
	except := int64(0)
	if !isNew {
		except = p.ID
	}
	if p.Slug, err = cs.UniqueSlug(p.Type, slug, except); err != nil {
		v.fail("%s: บันทึกไม่สำเร็จ", what)
	}
	if isNew {
		err = cs.CreatePost(p, nil)
	} else {
		v.sess.forgetPost(p.ID)
		editor := int64(0)
		if v.user != nil {
			editor = v.user.ID
		} else if v.h.e != nil {
			editor = v.h.e.systemAuthor()
		}
		err = cs.UpdatePost(p, nil, editor)
	}
	if err != nil {
		v.fail("%s: บันทึกไม่สำเร็จ: %s", what, err.Error())
	}
	if len(meta) > 0 {
		if err := cs.SetPostMeta(p.ID, meta); err != nil {
			v.fail("%s: บันทึกฟิลด์ไม่สำเร็จ", what)
		}
	}
	saved, err := cs.GetPost(p.ID)
	if err != nil {
		v.fail("%s: อ่านโพสต์ที่บันทึกไม่สำเร็จ", what)
	}
	v.emitPost("post.saved", saved)
	if saved.Status == model.StatusPublish && was != model.StatusPublish {
		v.emitPost("post.published", saved)
	}
	return v.toJS(v.postJSON(saved))
}

func (v *vm) postByArg(cs ContentStore, x goja.Value, what string) *model.Post {
	var id int64
	switch k := x.Export().(type) {
	case int64:
		id = k
	case float64:
		id = int64(k)
	case string:
		id, _ = strconv.ParseInt(k, 10, 64)
	}
	if id <= 0 {
		v.throw("%s: ต้องระบุ id ของโพสต์", what)
	}
	p, err := cs.GetPost(id)
	if err != nil || p == nil {
		v.fail("%s: ไม่พบโพสต์ #%d", what, id)
	}
	if !v.knownType(p.Type) {
		v.fail("%s: โพสต์ #%d เป็นประเภทที่โค้ดนี้ไม่ได้ลงทะเบียน", what, id)
	}
	return p
}

func (v *vm) postsUpdate(c goja.FunctionCall) goja.Value {
	const what = "db.posts.update"
	cs := v.contentStore()
	p := v.postByArg(cs, c.Argument(0), what)
	if !v.mayEdit(p.Type, p.AuthorID) || (p.Status == model.StatusPublish && !v.mayPublish(p.Type)) {
		v.fail("%s: ไม่มีสิทธิ์แก้ไขโพสต์นี้", what)
	}
	pf := v.readPostFields(c.Argument(1), what)
	if pf.typ != nil && *pf.typ != p.Type {
		v.throw("%s: เปลี่ยนประเภทของโพสต์ไม่ได้", what)
	}
	was := p.Status
	if pf.slug == nil {
		pf.slug = &p.Slug
	}
	v.applyFields(p, pf)
	meta := v.writeMeta(p, pf.meta, p.Meta, false, what)
	return v.savePost(cs, p, meta, false, was, what)
}

func (v *vm) postsDelete(c goja.FunctionCall) goja.Value {
	const what = "db.posts.delete"
	cs := v.contentStore()
	p := v.postByArg(cs, c.Argument(0), what)
	if !v.mayEdit(p.Type, p.AuthorID) || (p.Status == model.StatusPublish && !v.mayPublish(p.Type)) {
		v.fail("%s: ไม่มีสิทธิ์ลบโพสต์นี้", what)
	}
	v.sess.forgetPost(p.ID)
	if err := cs.DeletePost(p.ID); err != nil {
		v.fail("%s: ลบไม่สำเร็จ", what)
	}
	if v.h.e != nil {
		v.h.e.emitCore("post.deleted", map[string]any{"id": p.ID, "type": p.Type, "title": p.Title, "slug": p.Slug})
	}
	return v.rt.ToValue(true)
}

// dbAsSystem runs fn with system rights (content writes), audited with the
// caller's file:line.
func (v *vm) dbAsSystem(c goja.FunctionCall) goja.Value {
	v.handlerOnly("db.asSystem")
	v.need("content-write", "db.asSystem")
	fn, ok := goja.AssertFunction(c.Argument(0))
	if !ok {
		v.throw("db.asSystem: ต้องส่งฟังก์ชัน เช่น db.asSystem(() => db.posts.create({...}))")
	}
	at := v.caller()
	if v.h.e != nil {
		uid := int64(0)
		if v.user != nil {
			uid = v.user.ID
		}
		if err := v.h.e.DB.Audit(uid, "as_system", fmt.Sprintf("%s:%d", at.File, at.Line)); err != nil {
			v.addLogLine("warn", "db.asSystem: บันทึก audit ไม่สำเร็จ")
		}
	}
	prev := v.system
	v.system = true
	defer func() { v.system = prev }()
	out, err := fn(goja.Undefined())
	if err != nil {
		panic(err)
	}
	return out
}

// ---------------------------------------------------------------- reading custom types and meta

var metaWhereOps = map[string]string{"=": "=", "eq": "=", "!=": "!=", "ne": "!=", "<": "<", "lt": "<", "<=": "<=", "lte": "<=",
	">": ">", "gt": ">", ">=": ">=", "gte": ">=", "in": "in"}

// metaFilter reads db.posts.list({ where: {'meta.x': v | {gte: v}}, orderBy: 'meta.x' | '-meta.x' }).
func (v *vm) metaFilter(o *goja.Object, f *model.PostFilter) {
	if o == nil {
		return
	}
	if w := optObject(v.rt, o.Get("where")); w != nil {
		for _, k := range w.Keys() {
			key, ok := strings.CutPrefix(k, "meta.")
			if !ok || !metaKeyOK(key) {
				v.throw("db.posts.list: where รองรับเฉพาะ 'meta.<ฟิลด์>' (ไม่ใช่ %q)", k)
			}
			val := w.Get(k)
			if co := optObject(v.rt, val); co != nil && co.ClassName() == "Object" {
				for _, op := range co.Keys() {
					sqlOp, ok := metaWhereOps[op]
					if !ok {
						v.throw("db.posts.list: operator %q ไม่รองรับ (=, !=, <, <=, >, >=, in)", op)
					}
					x := co.Get(op).Export()
					if sqlOp == "in" {
						list, ok := x.([]any)
						if !ok {
							v.throw("db.posts.list: in ต้องเป็น array")
						}
						f.Meta = append(f.Meta, model.MetaCond{Key: key, Op: "in", Value: normList(list)})
						continue
					}
					f.Meta = append(f.Meta, model.MetaCond{Key: key, Op: sqlOp, Value: normScalar(x)})
				}
				continue
			}
			f.Meta = append(f.Meta, model.MetaCond{Key: key, Op: "=", Value: normScalar(val.Export())})
		}
	}
	if ob := optString(o, "orderBy"); ob != "" {
		desc := false
		if strings.HasPrefix(ob, "-") {
			desc, ob = true, ob[1:]
		}
		if f2 := strings.Fields(ob); len(f2) == 2 {
			ob, desc = f2[0], strings.EqualFold(f2[1], "desc")
		}
		key, ok := strings.CutPrefix(ob, "meta.")
		if !ok || !metaKeyOK(key) {
			v.throw("db.posts.list: orderBy รองรับ 'meta.<ฟิลด์>' หรือ '-meta.<ฟิลด์>' (ใช้ order สำหรับ newest/oldest/title)")
		}
		f.MetaOrder, f.MetaDesc = key, desc
	}
}

func normScalar(x any) any {
	switch n := x.(type) {
	case int64:
		return float64(n)
	case int:
		return float64(n)
	}
	return x
}

func normList(l []any) []any {
	out := make([]any, len(l))
	for i, x := range l {
		out[i] = normScalar(x)
	}
	return out
}

// metaKeyOK: a custom field key, or a JS plugin's "<id>.<key>" (phase 7).
func metaKeyOK(k string) bool {
	if metaKeyRe.MatchString(k) {
		return true
	}
	id, key, ok := strings.Cut(k, ".")
	return ok && len(k) <= 64 && unitIDRe.MatchString(id) && metaKeyRe.MatchString(key)
}
