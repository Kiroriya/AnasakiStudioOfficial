package api

// Designer collaboration: Figma-style comments pinned to layers, presence
// (who else has a page/component open) and optimistic-concurrency checks for
// designer saves.

import (
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

const (
	maxCommentRunes  = 5000
	presenceTTL      = 45 * time.Second
	commentDenyMsg   = "คุณไม่มีสิทธิ์ทำรายการนี้"
	commentTargetMsg = "ไม่พบหน้าหรือคอมโพเนนต์นี้"
)

var nodeIDRe = regexp.MustCompile(`^[A-Za-z0-9_\-:.]{1,64}$`)

func (a *API) collabRoutes(h func(string, string, http.HandlerFunc)) {
	const p = "/api/v1"
	h("GET "+p+"/design/comments", "", a.listDesignComments)
	h("POST "+p+"/design/comments", "", a.createDesignComment)
	h("PUT "+p+"/design/comments/{id}", "", a.updateDesignComment)
	h("DELETE "+p+"/design/comments/{id}", "", a.deleteDesignComment)
	h("POST "+p+"/design/comments/{id}/resolve", "", a.resolveDesignComment)
	h("POST "+p+"/design/presence", "", a.presenceHeartbeat)
	h("DELETE "+p+"/design/presence", "", a.presenceLeave)
}

// targetAccess checks that the user may edit the target (the same rule as
// saving it) and returns its stored updated_at. On failure it writes the
// response and returns ok=false.
func (a *API) targetAccess(w http.ResponseWriter, r *http.Request, kind string, id int64) (time.Time, bool) {
	me := auth.From(r)
	switch kind {
	case "post":
		p, err := a.st.GetPost(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, commentTargetMsg)
			return time.Time{}, false
		}
		if !a.canEdit(me, p) {
			writeErr(w, http.StatusForbidden, commentDenyMsg)
			return time.Time{}, false
		}
		return p.UpdatedAt, true
	case "component":
		if !me.Can(auth.CapEditPages) {
			writeErr(w, http.StatusForbidden, commentDenyMsg)
			return time.Time{}, false
		}
		t, err := a.st.TargetUpdatedAt("component", id)
		if err != nil {
			writeErr(w, http.StatusNotFound, commentTargetMsg)
			return time.Time{}, false
		}
		return t, true
	}
	writeErr(w, http.StatusBadRequest, "kind ต้องเป็น post หรือ component")
	return time.Time{}, false
}

// ---------------------------------------------------------------- comments

func (a *API) listDesignComments(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if _, ok := a.targetAccess(w, r, kind, id); !ok {
		return
	}
	cs, err := a.st.ListDesignComments(kind, id)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func cleanCommentBody(s string) (string, string) {
	s = strings.TrimSpace(strings.ToValidUTF8(s, ""))
	if s == "" {
		return "", "กรุณาพิมพ์ความคิดเห็น"
	}
	if utf8.RuneCountInString(s) > maxCommentRunes {
		return "", "ความคิดเห็นยาวเกินไป (สูงสุด 5000 ตัวอักษร)"
	}
	return s, ""
}

func cleanCoord(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Max(-100000, math.Min(100000, math.Round(v*100)/100))
}

func (a *API) createDesignComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind     string  `json:"kind"`
		ID       int64   `json:"id"`
		NodeID   *string `json:"nodeId"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		Body     string  `json:"body"`
		ParentID *int64  `json:"parentId"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	me := auth.From(r)
	c := &store.DesignComment{TargetKind: in.Kind, TargetID: in.ID, AuthorID: me.ID}
	if in.ParentID != nil && *in.ParentID > 0 {
		parent, err := a.st.GetDesignComment(*in.ParentID)
		if err != nil {
			serverErr(w, err)
			return
		}
		if parent.ParentID != nil { // replies always hang off the thread root
			if parent, err = a.st.GetDesignComment(*parent.ParentID); err != nil {
				serverErr(w, err)
				return
			}
		}
		c.TargetKind, c.TargetID, c.ParentID = parent.TargetKind, parent.TargetID, &parent.ID
		c.NodeID, c.X, c.Y = parent.NodeID, parent.X, parent.Y
	} else {
		if in.NodeID != nil && *in.NodeID != "" {
			if !nodeIDRe.MatchString(*in.NodeID) {
				writeErr(w, http.StatusBadRequest, "nodeId ไม่ถูกต้อง")
				return
			}
			c.NodeID = in.NodeID
		}
		c.X, c.Y = cleanCoord(in.X), cleanCoord(in.Y)
	}
	if _, ok := a.targetAccess(w, r, c.TargetKind, c.TargetID); !ok {
		return
	}
	var msg string
	if c.Body, msg = cleanCommentBody(in.Body); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := a.st.CreateDesignComment(c); err != nil {
		serverErr(w, err)
		return
	}
	a.respondComment(w, c.ID, http.StatusCreated)
}

func (a *API) respondComment(w http.ResponseWriter, id int64, status int) {
	c, err := a.st.GetDesignComment(id)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, status, c)
}

// commentForChange loads the comment in the path and checks target access.
func (a *API) commentForChange(w http.ResponseWriter, r *http.Request) *store.DesignComment {
	c, err := a.st.GetDesignComment(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return nil
	}
	if _, ok := a.targetAccess(w, r, c.TargetKind, c.TargetID); !ok {
		return nil
	}
	return c
}

func (a *API) updateDesignComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body string `json:"body"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	c := a.commentForChange(w, r)
	if c == nil {
		return
	}
	if c.AuthorID != auth.From(r).ID {
		writeErr(w, http.StatusForbidden, "แก้ไขได้เฉพาะความคิดเห็นของคุณเอง")
		return
	}
	body, msg := cleanCommentBody(in.Body)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := a.st.UpdateDesignCommentBody(c.ID, body); err != nil {
		serverErr(w, err)
		return
	}
	a.respondComment(w, c.ID, http.StatusOK)
}

func (a *API) deleteDesignComment(w http.ResponseWriter, r *http.Request) {
	c := a.commentForChange(w, r)
	if c == nil {
		return
	}
	me := auth.From(r)
	if c.AuthorID != me.ID && !me.Can(auth.CapManageOptions) {
		writeErr(w, http.StatusForbidden, "ลบได้เฉพาะความคิดเห็นของคุณเอง")
		return
	}
	if err := a.st.DeleteDesignComment(c.ID); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) resolveDesignComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Resolved bool `json:"resolved"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	c := a.commentForChange(w, r)
	if c == nil {
		return
	}
	if c.ParentID != nil {
		writeErr(w, http.StatusBadRequest, "ปิดได้เฉพาะทั้งเธรด")
		return
	}
	if err := a.st.SetDesignCommentResolved(c.ID, in.Resolved); err != nil {
		serverErr(w, err)
		return
	}
	a.respondComment(w, c.ID, http.StatusOK)
}

// ---------------------------------------------------------------- presence

type presenceUser struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Avatar     string `json:"avatar"`
	SelectedID string `json:"selectedId,omitempty"`
	session    string
	seen       time.Time
}

type savedBy struct {
	userID int64
	name   string
	at     time.Time
}

// collabHub keeps who has which design open (in memory: presence is
// ephemeral, a restart just empties it) and who saved each target last.
type collabHub struct {
	mu        sync.Mutex
	open      map[string]map[string]*presenceUser // target → session → user
	lastSaved map[string]savedBy
}

var collab = &collabHub{open: map[string]map[string]*presenceUser{}, lastSaved: map[string]savedBy{}}

func targetKey(kind string, id int64) string { return kind + ":" + strconv.FormatInt(id, 10) }

func (h *collabHub) beat(key string, u presenceUser) []presenceUser {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	// sweep every target (cheap: only open designers are tracked)
	for k, sessions := range h.open {
		for s, e := range sessions {
			if now.Sub(e.seen) > presenceTTL {
				delete(sessions, s)
			}
		}
		if len(sessions) == 0 {
			delete(h.open, k)
		}
	}
	sessions := h.open[key]
	if sessions == nil {
		sessions = map[string]*presenceUser{}
		h.open[key] = sessions
	}
	u.seen = now
	sessions[u.session] = &u
	out := []presenceUser{}
	seen := map[int64]bool{u.ID: true}
	for _, e := range sessions {
		if seen[e.ID] {
			continue // yourself (any tab), or a user's second tab
		}
		seen[e.ID] = true
		out = append(out, *e)
	}
	return out
}

func (h *collabHub) leave(key, session string, userID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e := h.open[key][session]; e != nil && e.ID == userID {
		delete(h.open[key], session)
	}
}

func (h *collabHub) noteSaved(key string, userID int64, name string, at time.Time) {
	h.mu.Lock()
	h.lastSaved[key] = savedBy{userID, name, at}
	h.mu.Unlock()
}

// lastEditor names who saved the version stored at `stored`.
func (a *API) lastEditor(kind string, id int64, stored time.Time) (int64, string) {
	collab.mu.Lock()
	s, ok := collab.lastSaved[targetKey(kind, id)]
	collab.mu.Unlock()
	if ok && math.Abs(s.at.Sub(stored).Seconds()) <= 1.5 {
		return s.userID, s.name
	}
	return 0, a.st.LastEditorName(kind, id)
}

func cleanSession(s string) string {
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func (a *API) presenceHeartbeat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind       string `json:"kind"`
		ID         int64  `json:"id"`
		Session    string `json:"session"`
		SelectedID string `json:"selectedId"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	updated, ok := a.targetAccess(w, r, in.Kind, in.ID)
	if !ok {
		return
	}
	me := auth.From(r)
	name, avatar := a.st.UserBrief(me.ID)
	if name == "" {
		name = me.Name
	}
	sel := in.SelectedID
	if sel != "" && !nodeIDRe.MatchString(sel) {
		sel = ""
	}
	session := cleanSession(in.Session)
	if session == "" {
		session = "u" + strconv.FormatInt(me.ID, 10)
	}
	others := collab.beat(targetKey(in.Kind, in.ID), presenceUser{ID: me.ID, Name: name, Avatar: avatar, SelectedID: sel, session: session})
	byID, byName := a.lastEditor(in.Kind, in.ID, updated)
	stamp, err := a.st.DesignCommentsStamp(in.Kind, in.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users": others, "updatedAt": updated, "updatedById": byID, "updatedBy": byName, "commentsStamp": stamp,
	})
}

func (a *API) presenceLeave(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, _ := strconv.ParseInt(q.Get("id"), 10, 64)
	collab.leave(targetKey(q.Get("kind"), id), cleanSession(q.Get("session")), auth.From(r).ID)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- save conflicts

// componentInput is a component save with the optional optimistic-concurrency base.
type componentInput struct {
	model.Component
	BaseUpdatedAt *time.Time `json:"baseUpdatedAt"`
}

// saveConflict writes 409 when the client loaded an older version than the
// one stored (base is only sent by the designer; nil = no check).
func (a *API) saveConflict(w http.ResponseWriter, kind string, id int64, base *time.Time, stored time.Time) bool {
	if base == nil || base.IsZero() || id == 0 {
		return false
	}
	if !stored.Truncate(time.Second).After(base.Truncate(time.Second)) {
		return false
	}
	byID, byName := a.lastEditor(kind, id, stored)
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":    "มีผู้อื่นบันทึกเวอร์ชันที่ใหม่กว่าแล้ว",
		"conflict": map[string]any{"updatedAt": stored, "updatedBy": byName, "updatedById": byID},
	})
	return true
}

// componentConflict checks a component save against its stored version.
func (a *API) componentConflict(w http.ResponseWriter, id int64, base *time.Time) bool {
	if base == nil || id == 0 {
		return false
	}
	stored, err := a.st.TargetUpdatedAt("component", id)
	if err != nil {
		return false // saving reports the missing component
	}
	return a.saveConflict(w, "component", id, base, stored)
}

// noteSaved records who saved a target, for conflict and presence messages.
func (a *API) noteSaved(kind string, id int64, me *auth.Principal, at time.Time) {
	collab.noteSaved(targetKey(kind, id), me.ID, me.Name, at)
}

// componentSaved returns the stored updated_at (second precision, like the
// list endpoint) so the designer's next base matches the database.
func (a *API) componentSaved(c *model.Component, me *auth.Principal) {
	if t, err := a.st.TargetUpdatedAt("component", c.ID); err == nil {
		c.UpdatedAt = t
	}
	a.noteSaved("component", c.ID, me, c.UpdatedAt)
}
