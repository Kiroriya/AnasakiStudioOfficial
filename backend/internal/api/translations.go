package api

import (
	"net/http"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/site"
	"github.com/anasaki/studio/backend/internal/store"
)

// Translations (2.5.0, multilingual content):
//
//	GET  /api/v1/posts/{id}/translations   → [{id, lang, title, status, url}] of the post's group (the original first)
//	POST /api/v1/posts/{id}/translate      {lang} → the new draft: a copy of the original in lang (201), or the
//	                                       existing translation (200)

type translationRow struct {
	ID     int64  `json:"id"`
	Lang   string `json:"lang"`
	Title  string `json:"title"`
	Status string `json:"status"`
	URL    string `json:"url"`
	Type   string `json:"type"`
}

func (a *API) listTranslations(w http.ResponseWriter, r *http.Request) {
	p, err := a.st.GetPost(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if !a.canEdit(auth.From(r), p) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
		return
	}
	list, err := a.st.Translations(p)
	if err != nil {
		serverErr(w, err)
		return
	}
	out := make([]translationRow, 0, len(list))
	for _, q := range list {
		out = append(out, translationRow{ID: q.ID, Lang: q.Lang, Title: q.Title, Status: q.Status, URL: site.PostURL(q), Type: q.Type})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) translatePost(w http.ResponseWriter, r *http.Request) {
	me := auth.From(r)
	var in struct {
		Lang string `json:"lang"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	src, err := a.st.GetPost(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if !a.canEdit(me, src) {
		writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
		return
	}
	if src.Type == model.TypeTemplate {
		writeErr(w, http.StatusBadRequest, "แม่แบบไม่ต้องแปล — ข้อความในแม่แบบมาจากเนื้อหาของแต่ละหน้า")
		return
	}
	opts, err := a.st.Options()
	if err != nil {
		serverErr(w, err)
		return
	}
	langs := site.ContentLanguages(opts)
	known := in.Lang == ""
	for _, l := range langs {
		known = known || l == in.Lang
	}
	if !known {
		writeErr(w, http.StatusBadRequest, "ยังไม่ได้เปิดภาษานี้ (ตั้งค่า → ทั่วไป → ภาษาเพิ่มเติม)")
		return
	}
	// translate from the original
	orig := src
	if src.TranslationOf > 0 {
		if o, err := a.st.GetPost(src.TranslationOf); err == nil {
			orig = o
		}
	}
	list, err := a.st.Translations(orig)
	if err != nil {
		serverErr(w, err)
		return
	}
	for _, q := range list {
		if q.Lang == in.Lang {
			a.respondPost(w, q.ID, http.StatusOK) // already translated: open it
			return
		}
	}
	cp := *orig
	cp.ID, cp.Status, cp.AuthorID, cp.PublishedAt, cp.Pinned = 0, model.StatusDraft, me.ID, nil, false
	cp.Lang, cp.TranslationOf = in.Lang, orig.ID
	if cp.Slug, err = a.st.UniqueSlug(cp.Type, orig.Slug+"-"+in.Lang, 0); err != nil {
		serverErr(w, err)
		return
	}
	ids := []int64{}
	for _, terms := range orig.Terms {
		for _, t := range terms {
			ids = append(ids, t.ID)
		}
	}
	if err := a.db(r).SavePost(&cp, true, store.PostSave{TermIDs: ids, Meta: orig.Meta}); err != nil {
		serverErr(w, err)
		return
	}
	a.respondPost(w, cp.ID, http.StatusCreated)
}
