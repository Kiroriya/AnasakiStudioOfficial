package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/media"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/store"
)

// ---------------------------------------------------------------- terms

func (a *API) listTerms(w http.ResponseWriter, r *http.Request) {
	tax := r.URL.Query().Get("taxonomy")
	if tax == "" {
		tax = "category"
	}
	if !a.knownTaxonomy(tax) {
		writeErr(w, http.StatusBadRequest, "ไม่มีชุดหมวดนี้")
		return
	}
	ts, err := a.st.ListTerms(tax)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

func (a *API) createTerm(w http.ResponseWriter, r *http.Request) {
	var t model.Term
	if !readJSON(w, r, &t) {
		return
	}
	if t.Taxonomy == "" {
		t.Taxonomy = "category"
	}
	a.saveTerm(w, r, &t, http.StatusCreated)
}

func (a *API) updateTerm(w http.ResponseWriter, r *http.Request) {
	old, err := a.st.GetTerm(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	t := *old
	if !readJSON(w, r, &t) {
		return
	}
	t.ID, t.Taxonomy = old.ID, old.Taxonomy // a term never changes taxonomy
	a.saveTerm(w, r, &t, http.StatusOK)
}

// termWriteDenied answers 400/403 unless the caller may write terms of tax.
func (a *API) termWriteDenied(w http.ResponseWriter, r *http.Request, tax string) bool {
	if !a.knownTaxonomy(tax) {
		writeErr(w, http.StatusBadRequest, "ไม่มีชุดหมวดนี้")
		return true
	}
	if !a.canManageTerms(auth.From(r), tax) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์จัดการหมวดหมู่ชุดนี้")
		return true
	}
	return false
}

func (a *API) saveTerm(w http.ResponseWriter, r *http.Request, t *model.Term, status int) {
	if a.termWriteDenied(w, r, t.Taxonomy) {
		return
	}
	if t.Parent != 0 {
		tx := a.taxonomy(t.Taxonomy)
		if tx == nil || !tx.Hierarchical {
			writeErr(w, http.StatusBadRequest, "ชุดหมวดนี้ไม่มีลำดับชั้น")
			return
		}
		if p, err := a.st.GetTerm(t.Parent); err != nil || p.Taxonomy != t.Taxonomy {
			writeErr(w, http.StatusBadRequest, "ไม่พบหมวดแม่")
			return
		}
		if t.ID != 0 {
			if loop, _ := a.st.TermAncestorOf(t.ID, t.Parent); loop {
				writeErr(w, http.StatusBadRequest, "ตั้งหมวดแม่เป็นตัวเองหรือหมวดลูกของตัวเองไม่ได้")
				return
			}
		}
	}
	if len([]rune(t.Description)) > 500 {
		t.Description = string([]rune(t.Description)[:500])
	}
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		writeErr(w, http.StatusBadRequest, "กรุณาใส่ชื่อ")
		return
	}
	if t.Slug == "" {
		t.Slug = t.Name
	}
	t.Slug = store.Slugify(t.Slug)
	var err error
	if t.ID == 0 {
		err = a.db(r).CreateTerm(t)
	} else {
		err = a.db(r).UpdateTerm(t)
	}
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeErr(w, http.StatusConflict, "มีหมวดหมู่ที่ใช้ชื่อในลิงก์นี้แล้ว")
			return
		}
		serverErr(w, err)
		return
	}
	writeJSON(w, status, t)
}

func (a *API) deleteTerm(w http.ResponseWriter, r *http.Request) {
	t, err := a.st.GetTerm(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if a.termWriteDenied(w, r, t.Taxonomy) {
		return
	}
	if err := a.db(r).DeleteTerm(t.ID); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- media

func (a *API) mediaOwnedOrEditor(w http.ResponseWriter, r *http.Request) *model.Media {
	m, err := a.st.GetMedia(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return nil
	}
	me := auth.From(r)
	if m.AuthorID != me.ID && !me.Can(auth.CapEditOthersPosts) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
		return nil
	}
	return m
}

func (a *API) updateMedia(w http.ResponseWriter, r *http.Request) {
	var in struct{ Alt string }
	if !readJSON(w, r, &in) {
		return
	}
	m := a.mediaOwnedOrEditor(w, r)
	if m == nil {
		return
	}
	if err := a.db(r).UpdateMediaAlt(m.ID, in.Alt); err != nil {
		serverErr(w, err)
		return
	}
	m.Alt = in.Alt
	writeJSON(w, http.StatusOK, m)
}

func (a *API) deleteMedia(w http.ResponseWriter, r *http.Request) {
	m := a.mediaOwnedOrEditor(w, r)
	if m == nil {
		return
	}
	if err := media.Delete(a.db(r), a.uploadDir, m); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- menus

func (a *API) listMenus(w http.ResponseWriter, r *http.Request) {
	ms, err := a.st.ListMenus()
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

func (a *API) saveMenu(w http.ResponseWriter, r *http.Request) {
	var m model.Menu
	if !readJSON(w, r, &m) {
		return
	}
	m.ID = pathID(r, "id")
	var items []design.MenuItem
	if len(m.Items) > 0 {
		if err := json.Unmarshal(m.Items, &items); err != nil {
			writeErr(w, http.StatusBadRequest, "รายการเมนูไม่ถูกต้อง")
			return
		}
	}
	if strings.TrimSpace(m.Name) == "" {
		writeErr(w, http.StatusBadRequest, "กรุณาใส่ชื่อ")
		return
	}
	if m.ID != 0 {
		if _, err := a.st.GetMenu(m.ID); err != nil {
			serverErr(w, err) // 404 once deleted: autosaves must not resurrect it
			return
		}
	}
	if err := a.st.SaveMenu(&m); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (a *API) deleteMenu(w http.ResponseWriter, r *http.Request) {
	if err := a.st.DeleteMenu(pathID(r, "id")); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- components

func (a *API) listComponents(w http.ResponseWriter, r *http.Request) {
	cs, err := a.st.ListComponents()
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (a *API) saveComponent(w http.ResponseWriter, r *http.Request) {
	var in componentInput // model.Component + baseUpdatedAt (comments.go)
	if !readJSON(w, r, &in) {
		return
	}
	c := in.Component
	c.ID = pathID(r, "id")
	if a.componentConflict(w, c.ID, in.BaseUpdatedAt) {
		return
	}
	if strings.TrimSpace(c.Name) == "" {
		writeErr(w, http.StatusBadRequest, "กรุณาใส่ชื่อ")
		return
	}
	if _, err := design.ParseForSave(c.Node); err != nil {
		writeErr(w, http.StatusBadRequest, "ข้อมูลดีไซน์ไม่ถูกต้อง: "+jsonErrText(err))
		return
	}
	var stored json.RawMessage
	if c.ID > 0 {
		if old, err := a.st.GetComponent(c.ID); err == nil {
			stored = old.Node
		}
	}
	if a.denyCodeBlocks(w, r, c.Node, stored) {
		return
	}
	if !auth.From(r).Can(auth.CapUnfilteredHTML) {
		c.Node = design.StripRawHTML(c.Node)
	}
	if err := a.st.SaveComponentKeepingRevision(&c, auth.From(r).ID); err != nil {
		serverErr(w, err)
		return
	}
	a.componentSaved(&c, auth.From(r))
	writeJSON(w, http.StatusOK, c)
}

func (a *API) listComponentRevisions(w http.ResponseWriter, r *http.Request) {
	revs, err := a.st.ListComponentRevisions(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, revs)
}

func (a *API) getComponentRevision(w http.ResponseWriter, r *http.Request) {
	rev, err := a.st.GetComponentRevision(pathID(r, "id"), pathID(r, "rid"))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rev)
}

// deleteComponent deletes a component and takes its placements out of every
// design (the admin warns "those places will no longer show it" and lists
// them from /usage first), so no instance keeps pointing at a component that
// no longer exists. The changed designs keep revisions.
func (a *API) deleteComponent(w http.ResponseWriter, r *http.Request) {
	if _, err := a.st.DeleteComponent(pathID(r, "id"), auth.From(r).ID); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) componentUsage(w http.ResponseWriter, r *http.Request) {
	uses, err := a.st.ComponentUsage(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, uses)
}

// renameVariant follows a variant rename: instances pick a variant by name
// (props.variant), so every instance of the component that showed variant
// "from" is pointed at "to" in all designs (revisions kept, updated_at
// bumped). The editor renames the variant in the component itself.
func (a *API) renameVariant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.From, in.To = strings.TrimSpace(in.From), strings.TrimSpace(in.To)
	if in.From == "" || in.To == "" {
		writeErr(w, http.StatusBadRequest, "กรุณาใส่ชื่อเดิมและชื่อใหม่ของรูปแบบ")
		return
	}
	if len(in.From) > 191 || len(in.To) > 191 {
		writeErr(w, http.StatusBadRequest, "ชื่อรูปแบบยาวเกินไป")
		return
	}
	id := pathID(r, "id")
	if _, err := a.st.GetComponent(id); err != nil {
		writeErr(w, http.StatusNotFound, "ไม่พบส่วนนี้")
		return
	}
	n := 0
	if in.From != in.To {
		var err error
		if n, err = a.st.RenameVariant(id, in.From, in.To, auth.From(r).ID); err != nil {
			serverErr(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": n})
}

// setComponentSiteRole makes a component the site header or footer: every
// placement of the current one now shows it (or, with none yet, it is added
// to every page and template), and the names are swapped so plugin pages
// and new pages pick it up too.
func (a *API) setComponentSiteRole(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role string `json:"role"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Role != "header" && in.Role != "footer" {
		writeErr(w, http.StatusBadRequest, "role ต้องเป็น header หรือ footer")
		return
	}
	c, err := a.st.GetComponent(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "ไม่พบส่วนนี้")
		return
	}
	header, footer := site.HeaderFooter(a.st)
	cur, name, oldSuffix := header, "ส่วนหัวเว็บ", " (เดิม)"
	if in.Role == "footer" {
		cur, name = footer, "ส่วนท้ายเว็บ"
	}
	out := struct {
		Replaced int `json:"replaced"`
		Added    int `json:"added"`
	}{}
	if cur == c.ID { // already it (maybe found by name only): remember the role
		if err := a.st.SetOptions(map[string]string{site.RoleOption(in.Role): strconv.FormatInt(c.ID, 10)}); err != nil {
			serverErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	editor := auth.From(r).ID
	if cur > 0 {
		old, err := a.st.GetComponent(cur)
		if err != nil {
			serverErr(w, err)
			return
		}
		if strings.EqualFold(strings.TrimSpace(old.Name), in.Role) {
			name, oldSuffix = old.Name, " (old)"
		}
		if out.Replaced, err = a.st.ReplaceComponentRefs(cur, c.ID, editor); err != nil {
			serverErr(w, err)
			return
		}
		old.Name = strings.TrimSpace(old.Name) + oldSuffix
		if err := a.st.SaveComponentKeepingRevision(old, editor); err != nil {
			serverErr(w, err)
			return
		}
	} else if out.Added, err = a.st.PlaceComponentOnPages(c.ID, name, in.Role, in.Role == "footer", editor); err != nil {
		serverErr(w, err)
		return
	}
	// the renderer finds the header/footer by this option (the names are only
	// the fallback for sites saved before it existed)
	id := strconv.FormatInt(c.ID, 10)
	roles := map[string]string{site.RoleOption(in.Role): id}
	other := site.RoleOption(map[string]string{"header": "footer", "footer": "header"}[in.Role])
	if opts, err := a.st.Options(); err == nil && opts[other] == id {
		roles[other] = "" // a header turned footer is no longer the header
	}
	if err := a.st.SetOptions(roles); err != nil {
		serverErr(w, err)
		return
	}
	// the other role must not keep this component (e.g. a header renamed to footer)
	c.Name = name
	if err := a.st.SaveComponentKeepingRevision(c, editor); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// replaceLinks updates every link to a page's old address after it moved
// (designs, components and menus).
func (a *API) replaceLinks(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !sitePath(in.From) || !sitePath(in.To) {
		writeErr(w, http.StatusBadRequest, "ที่อยู่ต้องเป็นลิงก์ภายในเว็บที่ขึ้นต้นด้วย / (ห้ามขึ้นต้นด้วย // หรือมี \\)")
		return
	}
	n, err := a.st.ReplaceLinks(in.From, in.To, auth.From(r).ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"changed": n})
}

// sitePath reports whether s is a link inside this site ("/about"): not
// "//host" or "/\host", which browsers open as another site, and no
// control characters.
func sitePath(s string) bool {
	if len(s) > 300 || !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.ContainsRune(s, '\\') {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- stats

// stats answers the dashboard's counters. Every signed-in user opens the
// dashboard, so each number is only given to roles that may see those items
// (0 otherwise): posts — everyone's with edit_others_posts, else the user's
// own; pages, templates and components need edit_pages; media upload_files.
func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	me := auth.From(r)
	out := map[string]int{"posts": 0, "pages": 0, "templates": 0, "media": 0, "components": 0}
	for _, t := range []string{model.TypePost, model.TypePage, model.TypeTemplate} {
		f := model.PostFilter{Type: t, Limit: 1, NoContent: true}
		switch {
		case t != model.TypePost && !me.Can(auth.CapEditPages):
			continue
		case t == model.TypePost && !me.Can(auth.CapEditOthersPosts):
			if !me.Can(auth.CapEditOwnPosts) {
				continue
			}
			f.AuthorID = me.ID
		}
		_, n, err := a.st.ListPosts(f)
		if err != nil {
			serverErr(w, err)
			return
		}
		out[t+"s"] = n
	}
	count := func(key, table, cap string) error {
		if !me.Can(cap) {
			return nil
		}
		var n int
		err := a.st.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
		out[key] = n
		return err
	}
	if err := count("media", "media", auth.CapUploadFiles); err != nil {
		serverErr(w, err)
		return
	}
	if err := count("components", "components", auth.CapEditPages); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------------------------------------------------------------- preview

// preview renders an unsaved design exactly like the public site would.
func (a *API) preview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Design json.RawMessage `json:"design"`
		PostID int64           `json:"postId"`
		Title  string          `json:"title"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	// the same rules as a save: raw HTML (embed blocks) needs unfiltered_html,
	// or a preview link would run markup its author could never publish
	if !auth.From(r).Can(auth.CapUnfilteredHTML) {
		in.Design = design.StripRawHTML(in.Design)
	}
	// Frontend code blocks: only the stored ones run in a preview by
	// someone without edit_scripts
	if !auth.From(r).Can(codeBlockCap) {
		var stored json.RawMessage
		if in.PostID > 0 {
			if p, err := a.st.GetPost(in.PostID); err == nil {
				stored = p.Design
			}
		}
		in.Design = design.StripCodeBlocks(in.Design, stored)
	}
	root, err := design.Parse(in.Design)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ข้อมูลดีไซน์ไม่ถูกต้อง")
		return
	}
	opts, err := a.st.Options()
	if err != nil {
		serverErr(w, err)
		return
	}
	var ctx design.PostCtx
	if in.PostID > 0 {
		if p, err := a.st.GetPost(in.PostID); err == nil {
			if p.Type == model.TypeTemplate {
				// a template previews with the latest post as sample data
				if ps, _, _ := a.st.ListPosts(model.PostFilter{Type: model.TypePost, Limit: 1, NoContent: true}); len(ps) > 0 {
					if full, err := a.st.GetPost(ps[0].ID); err == nil {
						ctx = site.PostContext(full)
					}
				}
			} else {
				ctx = site.PostContext(p)
			}
		}
	}
	page, err := a.site.RenderDocument(root, site.Doc{Title: in.Title, Options: opts, Post: ctx})
	if err != nil {
		serverErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}
