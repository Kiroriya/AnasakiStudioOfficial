package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/kses"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/site"
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
	return a.canEdit(p, post) && (post.Status != model.StatusPublish || a.canPublish(p, post.Type))
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
	AuthorID        *int64          `json:"authorId"`
	Pinned          *bool           `json:"pinned"`
	BaseUpdatedAt   *time.Time      `json:"baseUpdatedAt"` // designer: the version it loaded (409 when stale)
	// Meta sets custom fields (site.postType / site.fields); "" or null removes one.
	Meta map[string]any `json:"meta"`
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
		if *in.Status != model.StatusDraft && *in.Status != model.StatusPublish {
			return "สถานะไม่ถูกต้อง"
		}
		p.Status = *in.Status
		if p.Status == model.StatusPublish && !canPublish {
			p.Status = model.StatusDraft // contributors submit drafts for review
		}
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
		if _, err := design.Parse(in.Design); err != nil {
			return "ข้อมูลดีไซน์ไม่ถูกต้อง: " + err.Error()
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
	if err := a.st.CreatePost(p, in.CategoryIDs); err != nil {
		serverErr(w, err)
		return
	}
	if err := a.st.SetPostMeta(p.ID, meta); err != nil {
		serverErr(w, err)
		return
	}
	a.emitSaved(p)
	a.emitPublished(p, "")
	a.respondPost(w, p.ID, http.StatusCreated)
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
	if a.saveConflict(w, "post", p.ID, in.BaseUpdatedAt, p.UpdatedAt) {
		return
	}
	was, storedDesign := p.Status, p.Design
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
	if err := a.st.UpdatePost(p, in.CategoryIDs, me.ID); err != nil {
		serverErr(w, err)
		return
	}
	if err := a.st.SetPostMeta(p.ID, meta); err != nil {
		serverErr(w, err)
		return
	}
	a.noteSaved("post", p.ID, me, time.Now())
	a.emitSaved(p)
	a.emitPublished(p, was)
	a.respondPost(w, p.ID, http.StatusOK)
}

// emitSaved fires post.saved after a post/page was created or saved.
func (a *API) emitSaved(p *model.Post) {
	a.plugins.Emit("post.saved", map[string]any{"id": p.ID, "type": p.Type, "title": p.Title,
		"slug": p.Slug, "status": p.Status, "url": site.PostURL(p)})
}

// emitPublished fires post.published when p has just become published.
func (a *API) emitPublished(p *model.Post, previous string) {
	if p.Status != model.StatusPublish || previous == model.StatusPublish {
		return
	}
	a.plugins.Emit("post.published", map[string]any{"id": p.ID, "type": p.Type, "title": p.Title,
		"slug": p.Slug, "url": site.PostURL(p)})
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
	if err := a.st.DeletePost(p.ID); err != nil {
		serverErr(w, err)
		return
	}
	a.plugins.Emit("post.deleted", map[string]any{"id": p.ID, "type": p.Type, "title": p.Title, "slug": p.Slug})
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
	cp.Title = src.Title + " (copy)"
	if cp.Slug, err = a.st.UniqueSlug(cp.Type, src.Slug+"-copy", 0); err != nil {
		serverErr(w, err)
		return
	}
	ids := make([]int64, 0, len(src.Categories))
	for _, t := range src.Categories {
		ids = append(ids, t.ID)
	}
	if err := a.st.CreatePost(&cp, ids); err != nil {
		serverErr(w, err)
		return
	}
	if err := a.st.SetPostMeta(cp.ID, src.Meta); err != nil {
		serverErr(w, err)
		return
	}
	a.emitSaved(&cp)
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
	if err := a.st.UpdatePost(p, nil, me.ID); err != nil {
		serverErr(w, err)
		return
	}
	a.emitSaved(p)
	a.respondPost(w, p.ID, http.StatusOK)
}

// sanitizeHTML is the analogue of wp_kses for users without unfiltered_html.
func sanitizeHTML(s string) string { return kses.SanitizeHTML(s) }

// embedHostRe lists the video and map players allowed in iframes.
var embedHostRe = kses.EmbedHostRe
