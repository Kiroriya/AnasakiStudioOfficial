package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/kses"
	"github.com/anasaki/studio/backend/internal/mail"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/sitecode"
	"github.com/anasaki/studio/backend/internal/store"
)

// validType: post, page, template, or a custom post type of the live Site
// Code (site.postType).
func (a *API) validType(t string) bool {
	return t == model.TypePost || t == model.TypePage || t == model.TypeTemplate || a.postType(t) != nil
}

// postType returns the live custom post type t (nil for core types and
// types no longer registered).
func (a *API) postType(t string) *sitecode.PostTypeInfo {
	if a.code == nil || t == model.TypePost || t == model.TypePage || t == model.TypeTemplate {
		return nil
	}
	return a.code.PostTypes().PostType(t)
}

// metaFields is the custom fields of type t (site.postType / site.fields).
func (a *API) metaFields(t string) []sitecode.FieldInfo {
	if a.code == nil {
		return nil
	}
	return a.code.PostTypes().FieldsFor(t)
}

// canEdit implements the WordPress edit_post meta-capability. Custom post
// types: the roles in the type's access.edit (the administrator always).
func (a *API) canEdit(p *auth.Principal, post *model.Post) bool {
	if pt := a.postType(post.Type); pt != nil {
		return pt.CanEditType(p.Role)
	}
	if post.Type != model.TypePost {
		return p.Can(auth.CapEditPages)
	}
	if post.AuthorID == p.ID {
		return p.Can(auth.CapEditOwnPosts)
	}
	return p.Can(auth.CapEditOthersPosts)
}

// canPublish: publish_posts, or for a custom type its caps.publish (2.0.6)
// or else its editors.
func (a *API) canPublish(p *auth.Principal, typ string) bool {
	if pt := a.postType(typ); pt != nil {
		return pt.CanPublishType(p.Role)
	}
	return p.Can(auth.CapPublishPosts)
}

// canModify is canEdit for changes (save, delete, restore): like WordPress'
// edit_published_posts, a published post or page stays as the publisher
// left it unless the user may publish too (a contributor can't rewrite or
// remove it after an editor approved it).
func (a *API) canModify(p *auth.Principal, post *model.Post) bool {
	return a.canEdit(p, post) && (post.Status == model.StatusDraft || post.Status == model.StatusPending || a.canPublish(p, post.Type))
}

const msgPublishedLocked = "เนื้อหานี้เผยแพร่แล้ว ต้องมีสิทธิ์เผยแพร่จึงจะแก้ไขหรือลบได้"

// denyModify writes 403 when me may not change post; true = denied.
func (a *API) denyModify(w http.ResponseWriter, me *auth.Principal, post *model.Post) bool {
	switch {
	case !a.canEdit(me, post):
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
	case !a.canModify(me, post):
		writeErr(w, http.StatusForbidden, msgPublishedLocked)
	default:
		return false
	}
	return true
}

func (a *API) listPosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	me := auth.From(r)
	f := model.PostFilter{
		Type:      q.Get("type"),
		Status:    q.Get("status"),
		Search:    strings.TrimSpace(q.Get("search")),
		Category:  q.Get("category"),
		Order:     q.Get("order"),
		Limit:     queryInt(r, "perPage", 20),
		NoContent: true, // lists stay light; fetch a single post for its body
	}
	if f.Limit < 1 || f.Limit > 200 { // same bounds as the store (a negative offset is a SQL error)
		f.Limit = 20
	}
	if q.Has("lang") { // 2.5.0: one content language ("" = the site's); absent = every language
		l := q.Get("lang")
		f.Lang = &l
	}
	// status: one of draft / publish / scheduled; "any" (or "all", "") = every status (2.3.0:
	// before, an unknown value silently matched nothing — Qraft feedback 28)
	switch f.Status {
	case "any", "all":
		f.Status = ""
	case "":
	default:
		if !model.ValidStatus(f.Status) {
			writeErr(w, http.StatusBadRequest, "status ต้องเป็น draft, pending, publish, scheduled หรือ any")
			return
		}
	}
	if f.Type == "" {
		f.Type = model.TypePost
	}
	if !a.validType(f.Type) {
		writeErr(w, http.StatusBadRequest, "ประเภทเนื้อหาไม่ถูกต้อง")
		return
	}
	if pt := a.postType(f.Type); pt != nil {
		if !pt.CanEditType(me.Role) {
			writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ดูรายการนี้")
			return
		}
	} else if f.Type != model.TypePost && !me.Can(auth.CapEditPages) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ดูรายการนี้")
		return
	}
	if f.Type == model.TypePost && !me.Can(auth.CapEditOthersPosts) {
		f.AuthorID = me.ID
	}
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	f.Offset = (page - 1) * f.Limit
	posts, total, err := a.st.ListPosts(f)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": posts, "total": total, "page": page})
}

func (a *API) getPost(w http.ResponseWriter, r *http.Request) {
	p, err := a.st.GetPost(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if !a.canEdit(auth.From(r), p) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type postInput struct {
	Type            string          `json:"type"`
	Title           *string         `json:"title"`
	Slug            *string         `json:"slug"`
	Status          *string         `json:"status"`
	Excerpt         *string         `json:"excerpt"`
	Content         *string         `json:"content"`
	Design          json.RawMessage `json:"design"`
	FeaturedMediaID *int64          `json:"featuredMediaId"`
	ClearFeatured   bool            `json:"clearFeatured"`
	CategoryIDs     []int64         `json:"categoryIds"`
	// Terms sets the post's terms per taxonomy (2.1.0): {"<taxonomy>": [ids]};
	// a taxonomy left out is unchanged. 2.1.1: an entry may also be a slug
	// (a JSON string), like db.posts in Site Code.
	Terms    map[string][]termRef `json:"terms"`
	AuthorID *int64               `json:"authorId"`
	Pinned   *bool                `json:"pinned"`
	// PublishedAt sets the publish date (2.1.0): in the future with status
	// "publish" or "scheduled" the post waits for it; in the past it back-dates.
	PublishedAt   *time.Time `json:"publishedAt"`
	BaseUpdatedAt *time.Time `json:"baseUpdatedAt"` // designer: the version it loaded (409 when stale)
	// Meta sets custom fields (site.postType / site.fields); "" or null removes one.
	Meta map[string]any `json:"meta"`
	// AsDraft (2.3.0, the designer's Save): on a published page, title / slug /
	// design are kept as its draft and visitors keep seeing the published
	// version until a save without it (Publish). DiscardDraft drops the draft.
	AsDraft      bool `json:"asDraft"`
	DiscardDraft bool `json:"discardDraft"`
}

// apply copies the provided fields onto p and validates them.
func (in *postInput) apply(p *model.Post, me *auth.Principal, canPublish bool) string {
	if in.Title != nil {
		p.Title = strings.TrimSpace(*in.Title)
	}
	if in.Slug != nil {
		p.Slug = *in.Slug
	}
	if in.Status != nil {
		if !model.ValidStatus(*in.Status) {
			return "สถานะไม่ถูกต้อง"
		}
		p.Status = *in.Status
		if p.Status != model.StatusDraft && !canPublish {
			p.Status = model.StatusPending // contributors send posts for review (2.5.0; before: a draft)
		}
	}
	if in.PublishedAt != nil && canPublish {
		t := in.PublishedAt.UTC()
		p.PublishedAt = &t
	}
	if p.Status == model.StatusScheduled && p.PublishedAt == nil {
		return "ตั้งเวลาเผยแพร่ต้องระบุวันเวลา (publishedAt)"
	}
	if in.Excerpt != nil {
		p.Excerpt = *in.Excerpt
	}
	if in.Content != nil {
		p.Content = *in.Content
		if !me.Can(auth.CapUnfilteredHTML) {
			p.Content = sanitizeHTML(p.Content)
		}
	}
	if len(in.Design) > 0 && string(in.Design) != "null" {
		if !me.Can(auth.CapEditPages) {
			return "คุณไม่มีสิทธิ์แก้ไขหน้าตาเว็บ"
		}
		if _, err := design.ParseForSave(in.Design); err != nil {
			return "ข้อมูลดีไซน์ไม่ถูกต้อง: " + jsonErrText(err)
		}
		p.Design = in.Design
		if !me.Can(auth.CapUnfilteredHTML) {
			p.Design = design.StripRawHTML(p.Design)
		}
	}
	if in.FeaturedMediaID != nil {
		p.FeaturedMediaID = in.FeaturedMediaID
	}
	if in.ClearFeatured {
		p.FeaturedMediaID = nil
	}
	if in.AuthorID != nil && me.Can(auth.CapEditOthersPosts) {
		p.AuthorID = *in.AuthorID
	}
	if in.Pinned != nil && me.Can(auth.CapEditOthersPosts) { // pinning reorders everyone's lists
		p.Pinned = *in.Pinned
	}
	if p.Title == "" {
		p.Title = "(ไม่มีชื่อ)"
	}
	return ""
}

func (a *API) createPost(w http.ResponseWriter, r *http.Request) {
	var in postInput
	if !readJSON(w, r, &in) {
		return
	}
	me := auth.From(r)
	if in.Type == "" {
		in.Type = model.TypePost
	}
	if !a.validType(in.Type) {
		writeErr(w, http.StatusBadRequest, "ประเภทเนื้อหาไม่ถูกต้อง")
		return
	}
	p := &model.Post{Type: in.Type, Status: model.StatusDraft, AuthorID: me.ID}
	if !a.canEdit(me, p) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
		return
	}
	if msg := in.apply(p, me, a.canPublish(me, p.Type)); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if a.denyCodeBlocks(w, r, p.Design) {
		return
	}
	meta, ok := a.prepareSave(w, me, p, in.Meta, nil, true)
	if !ok {
		return
	}
	if p.Slug == "" {
		p.Slug = p.Title
	}
	var err error
	if p.Slug, err = a.st.UniqueSlug(p.Type, p.Slug, 0); err != nil {
		serverErr(w, err)
		return
	}
	terms, msg := a.termSets(p.Type, &in)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := a.db(r).SavePost(p, true, store.PostSave{Terms: terms, Meta: meta}); err != nil {
		serverErr(w, err)
		return
	}
	if p.Status == model.StatusPending {
		a.notifyPending(p, me)
	}
	a.respondPost(w, p.ID, http.StatusCreated)
}

// notifyPending tells the site's administrator email that a post waits for review (2.5.0; best effort, in
// the background — a site without email set up just shows it in the list).
func (a *API) notifyPending(p *model.Post, me *auth.Principal) {
	opts, err := a.st.Options()
	if err != nil || strings.TrimSpace(opts["admin_email"]) == "" {
		return
	}
	to, title, by := strings.TrimSpace(opts["admin_email"]), p.Title, me.Name
	link := strings.TrimRight(opts["site_url"], "/") + "/" + orDefault(opts["admin_path"], "admin") + "/posts/" + strconv.FormatInt(p.ID, 10)
	go func() {
		err := a.SendMailMessage(mail.Message{To: []string{to},
			Subject: "มีบทความรอตรวจ: " + title + " / A post waits for review",
			Text:    by + " ส่งบทความ \"" + title + "\" ให้ตรวจ / sent \"" + title + "\" for review\n\n" + link + "\n"})
		if err != nil && !errors.Is(err, mail.ErrNotConfigured) {
			log.Printf("pending post mail: %v", err)
		}
	}()
}

func (a *API) updatePost(w http.ResponseWriter, r *http.Request) {
	var in postInput
	if !readJSON(w, r, &in) {
		return
	}
	me := auth.From(r)
	p, err := a.st.GetPost(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if a.denyModify(w, me, p) {
		return
	}
	wasStatus := p.Status
	if a.saveConflict(w, "post", p.ID, in.BaseUpdatedAt, p.UpdatedAt) {
		return
	}
	if in.DiscardDraft {
		if _, err := a.st.SetPostDraft(p.ID, nil); err != nil {
			serverErr(w, err)
			return
		}
		a.respondPost(w, p.ID, http.StatusOK)
		return
	}
	if in.AsDraft && p.Status == model.StatusPublish && in.Design != nil {
		a.saveDraft(w, r, me, p, &in)
		return
	}
	storedDesign := p.Design
	if msg := in.apply(p, me, a.canPublish(me, p.Type)); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if a.denyCodeBlocks(w, r, p.Design, storedDesign) {
		return
	}
	slugBefore := p.Slug
	meta, ok := a.prepareSave(w, me, p, in.Meta, p.Meta, false)
	if !ok {
		return
	}
	if p.Slug != slugBefore && in.Slug == nil {
		in.Slug = &p.Slug // changed by post.beforeSave: keep it unique
	}
	if in.Slug != nil {
		if p.Slug, err = a.st.UniqueSlug(p.Type, p.Slug, p.ID); err != nil {
			serverErr(w, err)
			return
		}
	}
	terms, msg := a.termSets(p.Type, &in)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := a.db(r).SavePost(p, false, store.PostSave{Terms: terms, Meta: meta, EditorID: me.ID}); err != nil {
		serverErr(w, err)
		return
	}
	if p.Status == model.StatusPending && wasStatus != model.StatusPending {
		a.notifyPending(p, me)
	}
	if in.Design != nil { // the designer published: its draft is now live
		if _, err := a.st.SetPostDraft(p.ID, nil); err != nil {
			serverErr(w, err)
			return
		}
	}
	a.noteSaved("post", p.ID, me, time.Now())
	a.respondPost(w, p.ID, http.StatusOK)
}

// saveDraft keeps a published page's designer changes as its draft
// (postInput.AsDraft), checked like a real save but not applied.
func (a *API) saveDraft(w http.ResponseWriter, r *http.Request, me *auth.Principal, p *model.Post, in *postInput) {
	next := *p
	in.Status = nil
	if msg := in.apply(&next, me, a.canPublish(me, p.Type)); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if a.denyCodeBlocks(w, r, next.Design, p.Design) {
		return
	}
	if _, err := a.st.SetPostDraft(p.ID, &model.PostDraft{Title: next.Title, Slug: next.Slug, Design: next.Design}); err != nil {
		serverErr(w, err)
		return
	}
	a.noteSaved("post", p.ID, me, time.Now())
	a.respondPost(w, p.ID, http.StatusOK)
}

func (a *API) respondPost(w http.ResponseWriter, id int64, status int) {
	p, err := a.st.GetPost(id)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, status, p)
}

func (a *API) deletePost(w http.ResponseWriter, r *http.Request) {
	p, err := a.st.GetPost(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if a.denyModify(w, auth.From(r), p) {
		return
	}
	if err := a.db(r).DeletePost(p.ID); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) duplicatePost(w http.ResponseWriter, r *http.Request) {
	me := auth.From(r)
	src, err := a.st.GetPost(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if !a.canEdit(me, src) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
		return
	}
	cp := *src
	cp.ID, cp.Status, cp.AuthorID, cp.PublishedAt = 0, model.StatusDraft, me.ID, nil
	cp.TranslationOf = 0 // a copy keeps the language but is not one of the original's translations
	cp.Title = src.Title + " (copy)"
	if cp.Slug, err = a.st.UniqueSlug(cp.Type, src.Slug+"-copy", 0); err != nil {
		serverErr(w, err)
		return
	}
	ids := []int64{}
	for _, list := range src.Terms {
		for _, t := range list {
			ids = append(ids, t.ID)
		}
	}
	if err := a.db(r).SavePost(&cp, true, store.PostSave{TermIDs: ids, Meta: src.Meta}); err != nil {
		serverErr(w, err)
		return
	}
	a.respondPost(w, cp.ID, http.StatusCreated)
}

func (a *API) listRevisions(w http.ResponseWriter, r *http.Request) {
	p, err := a.st.GetPost(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if !a.canEdit(auth.From(r), p) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
		return
	}
	revs, err := a.st.ListRevisions(p.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, revs)
}

// getRevision returns one revision with its content and design (preview
// before restoring, or loading it into the designer).
func (a *API) getRevision(w http.ResponseWriter, r *http.Request) {
	p, err := a.st.GetPost(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if !a.canEdit(auth.From(r), p) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
		return
	}
	rev, err := a.st.GetRevision(p.ID, pathID(r, "rid"))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rev)
}

// orderPosts saves a hand-made order: PUT /posts/order {type, ids}.
func (a *API) orderPosts(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Type string  `json:"type"`
		IDs  []int64 `json:"ids"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Type == "" {
		in.Type = model.TypePost
	}
	me := auth.From(r)
	if !a.validType(in.Type) || (in.Type == model.TypePost && !me.Can(auth.CapEditOthersPosts)) ||
		(in.Type != model.TypePost && !me.Can(auth.CapEditPages)) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
		return
	}
	if len(in.IDs) > 1000 {
		writeErr(w, http.StatusBadRequest, "รายการมากเกินไป")
		return
	}
	if err := a.st.SetPostOrder(in.Type, in.IDs); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) restoreRevision(w http.ResponseWriter, r *http.Request) {
	me := auth.From(r)
	p, err := a.st.GetPost(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if a.denyModify(w, me, p) {
		return
	}
	rev, err := a.st.GetRevision(p.ID, pathID(r, "rid"))
	if err != nil {
		serverErr(w, err)
		return
	}
	// the same rules as a save (postInput.apply): the design needs
	// edit_pages, and markup is filtered without unfiltered_html
	if !store.JSONEqual(rev.Design, p.Design) && !me.Can(auth.CapEditPages) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์แก้ไขหน้าตาเว็บ")
		return
	}
	if a.denyCodeBlocks(w, r, rev.Design, p.Design) {
		return
	}
	p.Title, p.Content, p.Design = rev.Title, rev.Content, rev.Design
	if !me.Can(auth.CapUnfilteredHTML) {
		p.Content = sanitizeHTML(p.Content)
		if len(p.Design) > 0 {
			p.Design = design.StripRawHTML(p.Design)
		}
	}
	if err := a.db(r).UpdatePost(p, nil, me.ID); err != nil {
		serverErr(w, err)
		return
	}
	a.respondPost(w, p.ID, http.StatusOK)
}

// termSets is the terms to store per taxonomy: in.terms (each taxonomy must
// apply to the type) plus categoryIds, kept for the types that use categories.
// termRef is one entry of postInput.terms: a term id (JSON number) or slug
// (JSON string).
type termRef struct {
	ID   int64
	Slug string
}

func (r *termRef) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &r.Slug); err != nil {
			return err
		}
		if r.Slug = strings.TrimSpace(r.Slug); r.Slug == "" {
			return errors.New("terms: slug ว่าง")
		}
		return nil
	}
	if err := json.Unmarshal(b, &r.ID); err != nil {
		return errors.New("terms: ต้องเป็น id (ตัวเลข) หรือ slug (ข้อความ)")
	}
	return nil
}

func (a *API) termSets(typ string, in *postInput) (map[string][]int64, string) {
	if msg := a.postTermsInput(typ, in.Terms); msg != "" {
		return nil, msg
	}
	out := map[string][]int64{}
	for tax, refs := range in.Terms {
		ids := make([]int64, 0, len(refs))
		for _, ref := range refs {
			if ref.Slug == "" {
				ids = append(ids, ref.ID)
				continue
			}
			t, err := a.st.TermBySlug(tax, ref.Slug)
			if err != nil {
				return nil, "ไม่พบหมวด " + strconv.Quote(ref.Slug) + " ในชุดหมวด " + tax
			}
			ids = append(ids, t.ID)
		}
		out[tax] = ids
	}
	if in.CategoryIDs != nil && slices.Contains(a.manifest().TaxonomiesOf(typ), "category") {
		if _, set := out["category"]; !set {
			out["category"] = in.CategoryIDs
		}
	}
	return out, ""
}

// sanitizeHTML is the analogue of wp_kses for users without unfiltered_html.
func sanitizeHTML(s string) string { return kses.SanitizeHTML(s) }

// embedHostRe lists the video and map players allowed in iframes.
var embedHostRe = kses.EmbedHostRe
