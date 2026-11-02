package sitecode

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

// db.terms (CMS 2.1.0, P1-1): read (scope "content") and write (scope
// "terms-write") the terms of "category" and the site.taxonomy taxonomies.
// A JS plugin writes only its own taxonomies and "category"; Site Code writes
// its own and "category". The caller needs the right to manage the
// taxonomy's terms (manage_categories for "category", the taxonomy's
// access.manage otherwise) or db.asSystem.

// TermStore is what db.terms uses (store.Store).
type TermStore interface {
	FindTerms(f model.TermFilter) ([]model.Term, error)
	GetTerm(id int64) (*model.Term, error)
	TermBySlug(taxonomy, slug string) (*model.Term, error)
	TermAncestorOf(a, b int64) (bool, error)
	CreateTerm(t *model.Term) error
	UpdateTerm(t *model.Term) error
	DeleteTerm(id int64) error
}

var _ TermStore = (*store.Store)(nil)

func (v *vm) termStore() TermStore {
	ts, ok := v.h.posts.(TermStore)
	if !ok || ts == nil {
		v.throw("db.terms ไม่พร้อมใช้งานในสภาพแวดล้อมนี้")
	}
	if st, ok := ts.(*store.Store); ok {
		return st.As(v.actor()) // term.* events carry this code as the actor
	}
	return ts
}

// ownTaxonomy is a taxonomy the running code registered (nil otherwise).
func (v *vm) ownTaxonomy(id string) *TaxonomyInfo {
	for _, t := range v.taxes {
		if t.info.ID == id {
			info := t.info
			return &info
		}
	}
	return nil
}

// readableTaxonomy: "category" (nil info, true), the code's own or any
// running unit's taxonomy.
func (v *vm) readableTaxonomy(id string) (*TaxonomyInfo, bool) {
	if id == CategoryTaxonomy {
		return nil, true
	}
	if t := v.ownTaxonomy(id); t != nil {
		return t, true
	}
	if v.h.e != nil {
		if t := v.h.e.PostTypes().Taxonomy(id); t != nil {
			return t, true
		}
	}
	return nil, false
}

// mayManageTerms reports whether the caller may write terms of taxonomy id
// (info nil = "category").
func (v *vm) mayManageTerms(info *TaxonomyInfo) bool {
	if v.system {
		return true
	}
	if v.user == nil {
		return false
	}
	if info == nil {
		return v.user.Can(auth.CapManageTerms)
	}
	return info.CanManage(v.user.Role)
}

func (v *vm) taxonomyArg(o *goja.Object, what string) string {
	tax := optString(o, "taxonomy")
	if tax == "" {
		tax = CategoryTaxonomy
	}
	if _, ok := v.readableTaxonomy(tax); !ok {
		v.throw("%s: ไม่มี taxonomy %q (ใช้ 'category' หรือที่ลงทะเบียนด้วย site.taxonomy)", what, tax)
	}
	return tax
}

func termJSON(t model.Term) map[string]any {
	return map[string]any{"id": t.ID, "taxonomy": t.Taxonomy, "name": t.Name, "slug": t.Slug, "parent": t.Parent,
		"description": t.Description, "count": t.Count}
}

func (v *vm) idList(x goja.Value, what, key string) []int64 {
	list, ok := x.Export().([]any)
	if !ok || len(list) > 200 {
		v.throw("%s: %s ต้องเป็น array (ไม่เกิน 200 รายการ)", what, key)
	}
	out := make([]int64, 0, len(list))
	for _, e := range list {
		id := anyID(e)
		if id < 1 {
			v.throw("%s: %s ต้องเป็น id (จำนวนเต็มบวก)", what, key)
		}
		out = append(out, id)
	}
	return out
}

func anyID(e any) int64 {
	switch n := e.(type) {
	case int64:
		return n
	case float64:
		if n == float64(int64(n)) {
			return int64(n)
		}
	case string:
		id, _ := strconv.ParseInt(n, 10, 64)
		return id
	}
	return 0
}

// db.terms.list({taxonomy, parent?, search?, postId?, ids?, slugs?, limit?})
func (v *vm) termsList(c goja.FunctionCall) goja.Value {
	const what = "db.terms.list"
	v.handlerOnly("db")
	v.need("content", what)
	ts := v.termStore()
	o := optObject(v.rt, c.Argument(0))
	f := model.TermFilter{Taxonomy: v.taxonomyArg(o, what), Search: strings.TrimSpace(optString(o, "search")), Limit: 200}
	if o != nil {
		if p := o.Get("parent"); p != nil && !goja.IsUndefined(p) && !goja.IsNull(p) {
			n := anyID(p.Export())
			if n < 0 || (n == 0 && p.ToFloat() != 0) {
				v.throw("%s: parent ต้องเป็น id ของหมวด (0 = ระดับบนสุด)", what)
			}
			f.Parent = &n
		}
		if p := optString(o, "postId"); p != "" {
			f.PostID, _ = strconv.ParseInt(p, 10, 64)
			if f.PostID < 1 {
				v.throw("%s: postId ต้องเป็น id ของโพสต์", what)
			}
		}
		if x := o.Get("ids"); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
			f.IDs = v.idList(x, what, "ids")
		}
		if x := o.Get("slugs"); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
			if err := v.rt.ExportTo(x, &f.Slugs); err != nil || len(f.Slugs) > 200 {
				v.throw("%s: slugs ต้องเป็น array ของข้อความ (ไม่เกิน 200)", what)
			}
			if f.Slugs == nil {
				f.Slugs = []string{}
			}
		}
		if l := optString(o, "limit"); l != "" {
			n, err := strconv.Atoi(l)
			if err != nil || n < 1 || n > 1000 {
				v.throw("%s: limit ต้องอยู่ระหว่าง 1 ถึง 1000", what)
			}
			f.Limit = n
		}
	}
	terms, err := ts.FindTerms(f)
	if err != nil {
		v.throw("%s: อ่านข้อมูลไม่สำเร็จ", what)
	}
	out := make([]map[string]any, 0, len(terms))
	for _, t := range terms {
		out = append(out, termJSON(t))
	}
	return v.toJS(out)
}

// db.terms.get(idOrSlug, {taxonomy?}) → term | null
func (v *vm) termsGet(c goja.FunctionCall) goja.Value {
	const what = "db.terms.get"
	v.handlerOnly("db")
	v.need("content", what)
	ts := v.termStore()
	tax := v.taxonomyArg(optObject(v.rt, c.Argument(1)), what)
	var t *model.Term
	var err error
	if id := anyID(c.Argument(0).Export()); id > 0 {
		t, err = ts.GetTerm(id)
		if err == nil {
			if _, ok := v.readableTaxonomy(t.Taxonomy); !ok {
				return goja.Null()
			}
		}
	} else if s, ok := c.Argument(0).Export().(string); ok && s != "" {
		t, err = ts.TermBySlug(tax, s)
	} else {
		v.throw("%s: ต้องระบุ id หรือ slug", what)
	}
	if err != nil || t == nil {
		return goja.Null()
	}
	return v.toJS(termJSON(*t))
}

// writableTaxonomy checks scope, ownership and rights for a write.
func (v *vm) writableTaxonomy(tax, what string) {
	v.handlerOnly("db.terms")
	v.need("terms-write", what)
	var info *TaxonomyInfo
	if tax != CategoryTaxonomy {
		if info = v.ownTaxonomy(tax); info == nil {
			v.fail("%s: เขียนได้เฉพาะ 'category' และ taxonomy ที่โค้ดนี้ลงทะเบียนเอง (ไม่ใช่ %q)", what, tax)
		}
	}
	if !v.mayManageTerms(info) {
		v.fail("%s: ไม่มีสิทธิ์จัดการหมวดของ %q (ผู้เยี่ยมชมเขียนไม่ได้ — ใช้ db.asSystem เมื่อโค้ดทำแทนระบบ)", what, tax)
	}
}

// readTermInput applies {name, slug, parent, description} onto t.
func (v *vm) readTermInput(ts TermStore, o *goja.Object, t *model.Term, what string) {
	if s := optString(o, "name"); s != "" || t.ID == 0 {
		t.Name = clip(strings.TrimSpace(s), 191)
	}
	if t.Name == "" {
		v.throw("%s: ต้องมี name", what)
	}
	if x := o.Get("slug"); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
		t.Slug = x.String()
	}
	if t.Slug == "" {
		t.Slug = t.Name
	}
	t.Slug = store.Slugify(t.Slug)
	if t.Slug == "" {
		v.throw("%s: slug ว่าง", what)
	}
	if x := o.Get("description"); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
		t.Description = clip(x.String(), 500)
	}
	if x := o.Get("parent"); x != nil && !goja.IsUndefined(x) {
		p := int64(0)
		if !goja.IsNull(x) {
			p = anyID(x.Export())
		}
		if p != 0 {
			info, _ := v.readableTaxonomy(t.Taxonomy)
			if info == nil || !info.Hierarchical {
				v.throw("%s: taxonomy %q ไม่ใช่แบบลำดับชั้น (hierarchical) จึงตั้ง parent ไม่ได้", what, t.Taxonomy)
			}
			pt, err := ts.GetTerm(p)
			if err != nil || pt.Taxonomy != t.Taxonomy {
				v.fail("%s: ไม่พบหมวดแม่ #%d ใน %q", what, p, t.Taxonomy)
			}
			if t.ID != 0 {
				if loop, _ := ts.TermAncestorOf(t.ID, p); loop {
					v.fail("%s: ตั้ง parent เป็นตัวเองหรือหมวดลูกของตัวเองไม่ได้", what)
				}
			}
		}
		t.Parent = p
	}
}

func dupErr(err error) bool { return err != nil && strings.Contains(err.Error(), "Duplicate") }

// db.terms.create({taxonomy, name, slug?, parent?, description?})
func (v *vm) termsCreate(c goja.FunctionCall) goja.Value {
	const what = "db.terms.create"
	o := optObject(v.rt, c.Argument(0))
	if o == nil {
		v.throw("%s: ต้องระบุ { taxonomy, name }", what)
	}
	tax := v.taxonomyArg(o, what)
	v.writableTaxonomy(tax, what)
	ts := v.termStore()
	t := &model.Term{Taxonomy: tax}
	v.readTermInput(ts, o, t, what)
	if err := ts.CreateTerm(t); err != nil {
		if dupErr(err) {
			v.fail("%s: มี slug %q ใน %q แล้ว", what, t.Slug, tax)
		}
		v.fail("%s: บันทึกไม่สำเร็จ", what)
	}
	return v.toJS(termJSON(*t))
}

func (v *vm) termByArg(ts TermStore, x goja.Value, what string) *model.Term {
	id := anyID(x.Export())
	if id < 1 {
		v.throw("%s: ต้องระบุ id ของหมวด", what)
	}
	t, err := ts.GetTerm(id)
	if err != nil {
		v.fail("%s: ไม่พบหมวด #%d", what, id)
	}
	return t
}

// db.terms.update(id, {name?, slug?, parent?, description?})
func (v *vm) termsUpdate(c goja.FunctionCall) goja.Value {
	const what = "db.terms.update"
	v.handlerOnly("db.terms")
	ts := v.termStore()
	t := v.termByArg(ts, c.Argument(0), what)
	v.writableTaxonomy(t.Taxonomy, what)
	o := optObject(v.rt, c.Argument(1))
	if o == nil {
		v.throw("%s: ต้องระบุสิ่งที่แก้ เช่น { name }", what)
	}
	v.readTermInput(ts, o, t, what)
	if err := ts.UpdateTerm(t); err != nil {
		if dupErr(err) {
			v.fail("%s: มี slug %q ใน %q แล้ว", what, t.Slug, t.Taxonomy)
		}
		v.fail("%s: บันทึกไม่สำเร็จ", what)
	}
	return v.toJS(termJSON(*t))
}

// db.terms.delete(id) — the term's children move up to its parent.
func (v *vm) termsDelete(c goja.FunctionCall) goja.Value {
	const what = "db.terms.delete"
	v.handlerOnly("db.terms")
	ts := v.termStore()
	t := v.termByArg(ts, c.Argument(0), what)
	v.writableTaxonomy(t.Taxonomy, what)
	if err := ts.DeleteTerm(t.ID); err != nil {
		v.fail("%s: ลบไม่สำเร็จ", what)
	}
	return v.rt.ToValue(true)
}

// postTerms resolves PostInput.terms {taxonomy: (id | slug)[]} for a post of
// typ: each taxonomy must apply to the type and be "category" or the code's
// own; unknown terms are an error.
func (v *vm) postTerms(x goja.Value, typ, what string) map[string][]int64 {
	o := optObject(v.rt, x)
	if o == nil {
		return nil
	}
	ts := v.termStore()
	out := map[string][]int64{}
	for _, tax := range o.Keys() {
		var info *TaxonomyInfo
		if tax == CategoryTaxonomy {
			if typ != model.TypePost {
				if t := v.customType(typ); t == nil || !containsStr(t.Supports, "categories") {
					v.throw("%s: ประเภท %q ไม่ได้ใช้ 'category' (supports: ['categories'])", what, typ)
				}
			}
		} else if info = v.ownTaxonomy(tax); info == nil || !info.For(typ) {
			v.throw("%s: terms.%s: taxonomy นี้ไม่ได้ลงทะเบียนในโค้ดนี้สำหรับประเภท %q (postTypes)", what, tax, typ)
		}
		list, ok := o.Get(tax).Export().([]any)
		if !ok || len(list) > 100 {
			v.throw("%s: terms.%s ต้องเป็น array ของ id หรือ slug (ไม่เกิน 100)", what, tax)
		}
		ids := []int64{}
		for _, e := range list {
			var t *model.Term
			var err error
			if id := anyID(e); id > 0 {
				if t, err = ts.GetTerm(id); err == nil && t.Taxonomy != tax {
					err = store.ErrNotFound
				}
			} else if s, ok := e.(string); ok && s != "" {
				t, err = ts.TermBySlug(tax, s)
			} else {
				err = store.ErrNotFound
			}
			if err != nil || t == nil {
				v.fail("%s: terms.%s: ไม่พบหมวด %v", what, tax, e)
			}
			ids = append(ids, t.ID)
		}
		out[tax] = ids
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
