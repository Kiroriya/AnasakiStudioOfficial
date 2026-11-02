package api

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/store"
)

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	email := strings.TrimSpace(strings.ToLower(in.Email))
	ip := limiterIP(plugin.ClientIP(r))
	// guessing one account from one address locks that pair only; the
	// address alone and the account alone (from many addresses) have
	// higher limits, so a stranger can't lock the owner out from elsewhere
	keys := []string{"ipemail:" + ip + "|" + email, "ip:" + ip, "email:" + email}
	if !loginFails.allow(keys...) {
		writeErr(w, http.StatusTooManyRequests, "เข้าสู่ระบบผิดหลายครั้งเกินไป กรุณารอ 15 นาทีแล้วลองใหม่")
		return
	}
	u, hash, err := a.st.UserByEmail(email)
	var ok bool
	if err != nil {
		ok = auth.CheckNoUser(in.Password)
	} else {
		ok = auth.CheckPassword(hash, in.Password)
	}
	if !ok {
		loginFails.fail(keys...)
		writeErr(w, http.StatusUnauthorized, "อีเมลหรือรหัสผ่านไม่ถูกต้อง")
		return
	}
	loginFails.clear(keys[0])
	loginFails.clear(keys[2])
	if u.Status == model.UserSuspended {
		writeErr(w, http.StatusForbidden, "บัญชีนี้ถูกระงับ")
		return
	}
	a.issueToken(w, u, http.StatusOK)
}

// logout answers POST /api/v1/auth/logout {all?}: the session token stops
// working at once (revoked until it expires); all=true also ends every other
// session and ticket of the account ("sign out everywhere").
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		All bool `json:"all"`
	}
	if r.ContentLength != 0 && !readJSON(w, r, &in) {
		return
	}
	p := auth.From(r)
	if err := a.st.RevokeSession(p.JTI, p.Expires); err != nil {
		serverErr(w, err)
		return
	}
	if in.All {
		if a.visitor(r) { // the shared demo account: don't sign the other visitors out
			demoDenied(w)
			return
		}
		if err := a.st.BumpSessionEpoch(p.ID); err != nil {
			serverErr(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// refreshSession answers POST /api/v1/auth/refresh: a fresh session token
// (sliding expiry; the admin app renews while it is open).
func (a *API) refreshSession(w http.ResponseWriter, r *http.Request) {
	u, tok, err := a.session(auth.From(r).ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "user": u})
}

// limiterIP is the address a login limit applies to: IPv6 clients get a
// whole /64 each, so hopping between their own addresses doesn't help.
func limiterIP(s string) string {
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() != nil {
		return s
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// failLimiter counts failed logins per key (client IP, email…) in a sliding
// window. A key "<kind>:…" allows limits[kind] failures, else max.
type failLimiter struct {
	sync.Mutex
	max    int
	limits map[string]int
	window time.Duration
	hits   map[string][]time.Time
}

var loginFails = &failLimiter{max: 10, window: 15 * time.Minute, hits: map[string][]time.Time{},
	limits: map[string]int{"ip": 50, "email": 100}}

// limit is the number of failures key k may have in the window.
func (l *failLimiter) limit(k string) int {
	if kind, _, ok := strings.Cut(k, ":"); ok && l.limits[kind] > 0 {
		return l.limits[kind]
	}
	return l.max
}

// recent drops the expired entries of k; the lock must be held.
func (l *failLimiter) recent(k string, now time.Time) []time.Time {
	kept := l.hits[k][:0]
	for _, t := range l.hits[k] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.hits, k)
		return nil
	}
	l.hits[k] = kept
	return kept
}

func (l *failLimiter) allow(keys ...string) bool {
	l.Lock()
	defer l.Unlock()
	now := time.Now()
	for _, k := range keys {
		if len(l.recent(k, now)) >= l.limit(k) {
			return false
		}
	}
	return true
}

func (l *failLimiter) fail(keys ...string) {
	l.Lock()
	defer l.Unlock()
	now := time.Now()
	if len(l.hits) > 10000 { // forget stale keys now and then
		for k := range l.hits {
			l.recent(k, now)
		}
	}
	for _, k := range keys {
		l.hits[k] = append(l.recent(k, now), now)
	}
}

func (l *failLimiter) clear(k string) {
	l.Lock()
	delete(l.hits, k)
	l.Unlock()
}

// issueToken records the login and answers {token, user}.
func (a *API) issueToken(w http.ResponseWriter, u *model.User, status int) {
	if err := a.st.TouchLogin(u.ID); err != nil {
		serverErr(w, err)
		return
	}
	u, tok, err := a.session(u.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, status, map[string]any{"token": tok, "user": u})
}

// session reloads user id and signs a session for its current password.
func (a *API) session(id int64) (*model.User, string, error) {
	u, hash, err := a.st.UserAuthByID(id)
	if err != nil {
		return nil, "", err
	}
	tok, err := a.auth.Issue(u, hash)
	if err != nil {
		return nil, "", err
	}
	u.Caps = auth.Caps(u.Role)
	return u, tok, nil
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u, err := a.st.UserByID(auth.From(r).ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	u.Caps = auth.Caps(u.Role)
	writeJSON(w, http.StatusOK, u)
}

// profileInput holds the fields a user may change on their own profile.
type profileInput struct {
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	Password string  `json:"password"`
	// CurrentPassword must accompany a new password or email (PUT /auth/me),
	// so a stolen session token alone can't take the account over.
	CurrentPassword string  `json:"currentPassword"`
	Locale          *string `json:"locale"`
	FirstName       *string `json:"firstName"`
	LastName        *string `json:"lastName"`
	Bio             *string `json:"bio"`
	Website         *string `json:"website"`
	Avatar          *string `json:"avatar"`
}

func (in *profileInput) apply(u *model.User) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	set(&u.Name, in.Name)
	if in.Email != nil {
		u.Email = strings.ToLower(strings.TrimSpace(*in.Email))
	}
	set(&u.Locale, in.Locale)
	set(&u.FirstName, in.FirstName)
	set(&u.LastName, in.LastName)
	set(&u.Bio, in.Bio)
	set(&u.Website, in.Website)
	set(&u.Avatar, in.Avatar)
	if u.Name == "" {
		u.Name = strings.TrimSpace(u.FirstName + " " + u.LastName)
	}
}

func (a *API) updateMe(w http.ResponseWriter, r *http.Request) {
	var in profileInput
	if !readJSON(w, r, &in) {
		return
	}
	u, err := a.st.UserByID(auth.From(r).ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	if a.visitor(r) {
		// the account is shared: only the UI language may change
		if in.Password != "" || (in.Email != nil && *in.Email != u.Email) {
			demoDenied(w)
			return
		}
		in = profileInput{Locale: in.Locale}
	}
	emailChange := in.Email != nil && !strings.EqualFold(strings.TrimSpace(*in.Email), u.Email)
	if in.Password != "" || emailChange {
		key := "me:" + strconv.FormatInt(u.ID, 10)
		if !loginFails.allow(key) {
			writeErr(w, http.StatusTooManyRequests, "ใส่รหัสผ่านผิดหลายครั้งเกินไป กรุณารอ 15 นาทีแล้วลองใหม่")
			return
		}
		_, hash, err := a.st.UserByEmail(u.Email)
		if err != nil {
			serverErr(w, err)
			return
		}
		// 400, not 401/403: the session itself is fine
		if !auth.CheckPassword(hash, in.CurrentPassword) {
			loginFails.fail(key)
			writeErr(w, http.StatusBadRequest, "รหัสผ่านปัจจุบันไม่ถูกต้อง")
			return
		}
		loginFails.clear(key)
	}
	in.apply(u)
	if in.Password == "" {
		a.saveUser(w, r, u, "", http.StatusOK)
		return
	}
	// a new password signs out every other session; this one gets a fresh token
	if !a.storeUser(w, r, u, in.Password) {
		return
	}
	fresh, tok, err := a.session(u.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		*model.User
		Token string `json:"token"`
	}{fresh, tok})
}

// canManageRole reports whether p may give role to someone, or manage an
// account that has it: only administrators reach the administrator role or
// capabilities they do not hold themselves.
func canManageRole(p *auth.Principal, role string) bool {
	if p.Role == model.RoleAdmin {
		return true
	}
	return role != model.RoleAdmin && holdsAll(p, auth.Caps(role))
}

// holdsAll reports whether p has every capability in caps.
func holdsAll(p *auth.Principal, caps []string) bool {
	for _, c := range caps {
		if !p.Can(c) {
			return false
		}
	}
	return true
}

const (
	errRoleTooHigh = "คุณไม่สามารถกำหนดสิทธิ์ที่สูงกว่าสิทธิ์ของตัวเองได้"
	errUserTooHigh = "คุณไม่มีสิทธิ์จัดการบัญชีนี้"
)

// userOptions lists users as {id, name} (ids given in ?ids=1,2 or by
// ?search=, at most 50): the picker of "user" custom fields (CMS 2.1.0).
// No e-mail addresses or roles.
func (a *API) userOptions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out := []map[string]any{}
	if ids := q.Get("ids"); ids != "" {
		for i, s := range strings.Split(ids, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err != nil || i >= 100 {
				continue
			}
			if u, err := a.st.UserByID(id); err == nil {
				out = append(out, map[string]any{"id": u.ID, "name": u.Name})
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	us, _, err := a.st.ListUsers(model.UserFilter{Search: q.Get("search"), Status: model.UserActive, Limit: 50})
	if err != nil {
		serverErr(w, err)
		return
	}
	for _, u := range us {
		out = append(out, map[string]any{"id": u.ID, "name": u.Name})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	per := queryInt(r, "perPage", 20)
	if per < 1 || per > 200 {
		per = 20
	}
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	us, total, err := a.st.ListUsers(model.UserFilter{Search: q.Get("search"), Role: q.Get("role"),
		Status: q.Get("status"), Limit: per, Offset: (page - 1) * per})
	if err != nil {
		serverErr(w, err)
		return
	}
	counts, err := a.st.UserCounts()
	if err != nil {
		serverErr(w, err)
		return
	}
	for i := range us {
		us[i].Caps = auth.Caps(us[i].Role)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": us, "total": total, "counts": counts})
}

// userInput is what an administrator may set on any account.
type userInput struct {
	profileInput
	Role   string `json:"role"`
	Status string `json:"status"`
	// SendWelcome emails the new user a link to choose their password (create only).
	SendWelcome bool `json:"sendWelcome"`
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var in userInput
	if !readJSON(w, r, &in) {
		return
	}
	if in.SendWelcome && in.Password == "" {
		// The new user chooses their own password from the welcome email.
		if _, _, err := a.mailConfig(); err != nil {
			writeErr(w, http.StatusBadRequest, "ส่งอีเมลต้อนรับไม่ได้: "+err.Error()+" — หรือกำหนดรหัสผ่านเอง")
			return
		}
		in.Password = randomPassword()
	}
	if len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "รหัสผ่านต้องมีอย่างน้อย 8 ตัวอักษร")
		return
	}
	u := &model.User{Role: in.Role, Status: in.Status}
	if u.Role == "" {
		u.Role = model.RoleSubscriber
	}
	if !canManageRole(auth.From(r), u.Role) {
		writeErr(w, http.StatusForbidden, errRoleTooHigh)
		return
	}
	in.apply(u)
	if !a.saveUser(w, r, u, in.Password, http.StatusCreated) {
		return
	}
	if in.SendWelcome {
		a.sendWelcome(r, u)
	}
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	var in userInput
	if !readJSON(w, r, &in) {
		return
	}
	u, err := a.st.UserByID(pathID(r, "id"))
	if err != nil {
		serverErr(w, err)
		return
	}
	me := auth.From(r)
	if !canManageRole(me, u.Role) {
		writeErr(w, http.StatusForbidden, errUserTooHigh)
		return
	}
	if in.Role != "" && !canManageRole(me, in.Role) {
		writeErr(w, http.StatusForbidden, errRoleTooHigh)
		return
	}
	if u.ID == me.ID {
		if in.Role != "" && in.Role != u.Role {
			writeErr(w, http.StatusBadRequest, "ไม่สามารถเปลี่ยนสิทธิ์ของตัวเองได้")
			return
		}
		if in.Status != "" && in.Status != u.Status {
			writeErr(w, http.StatusBadRequest, "ไม่สามารถระงับบัญชีของตัวเองได้")
			return
		}
	}
	in.apply(u)
	if in.Role != "" {
		u.Role = in.Role
	}
	if in.Status != "" {
		u.Status = in.Status
	}
	a.saveUser(w, r, u, in.Password, http.StatusOK)
}

// saveUser validates and stores u, then answers with it; false on failure.
func (a *API) saveUser(w http.ResponseWriter, r *http.Request, u *model.User, password string, status int) bool {
	if !a.storeUser(w, r, u, password) {
		return false
	}
	if fresh, err := a.st.UserByID(u.ID); err == nil {
		*u = *fresh
	}
	u.Caps = auth.Caps(u.Role)
	writeJSON(w, status, u)
	return true
}

// storeUser validates and stores u; on failure it answers and returns false.
func (a *API) storeUser(w http.ResponseWriter, r *http.Request, u *model.User, password string) bool {
	if u.Name == "" || !strings.Contains(u.Email, "@") {
		writeErr(w, http.StatusBadRequest, "กรุณากรอกชื่อและอีเมลให้ถูกต้อง")
		return false
	}
	if !auth.ValidRole(u.Role) {
		writeErr(w, http.StatusBadRequest, "สิทธิ์การใช้งานไม่ถูกต้อง")
		return false
	}
	if u.Status == "" {
		u.Status = model.UserActive
	}
	if u.Status != model.UserActive && u.Status != model.UserSuspended {
		writeErr(w, http.StatusBadRequest, "สถานะผู้ใช้ไม่ถูกต้อง")
		return false
	}
	if u.Locale == "" {
		u.Locale = model.LocaleTH
	}
	if !model.ValidLocale(u.Locale) {
		writeErr(w, http.StatusBadRequest, "ภาษาต้องเป็น th หรือ en")
		return false
	}
	if !safeURL(u.Website) || !safeURL(u.Avatar) {
		writeErr(w, http.StatusBadRequest, "ลิงก์เว็บไซต์หรือรูปโปรไฟล์ไม่ถูกต้อง")
		return false
	}
	if password != "" && len(password) < 8 {
		writeErr(w, http.StatusBadRequest, "รหัสผ่านต้องมีอย่างน้อย 8 ตัวอักษร")
		return false
	}
	hash := ""
	if password != "" {
		var err error
		if hash, err = auth.HashPassword(password); err != nil {
			serverErr(w, err)
			return false
		}
	}
	var err error
	if u.ID == 0 {
		err = a.db(r).CreateUser(u, hash)
	} else {
		err = a.db(r).UpdateUser(u, hash)
	}
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeErr(w, http.StatusConflict, "อีเมลนี้มีผู้ใช้แล้ว")
			return false
		}
		serverErr(w, err)
		return false
	}
	return true
}

// safeURL accepts empty, http(s) and site-relative URLs.
func safeURL(s string) bool {
	if s == "" {
		return true
	}
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") ||
		(strings.HasPrefix(l, "/") && !strings.HasPrefix(l, "//"))
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	me := auth.From(r).ID
	target, err := a.st.UserByID(id)
	if err != nil {
		serverErr(w, err)
		return
	}
	if !canManageRole(auth.From(r), target.Role) {
		writeErr(w, http.StatusForbidden, errUserTooHigh)
		return
	}
	if id == me {
		writeErr(w, http.StatusBadRequest, "ไม่สามารถลบบัญชีของตัวเองได้")
		return
	}
	to := int64(queryInt(r, "reassign", int(me)))
	if to == id {
		writeErr(w, http.StatusBadRequest, "ต้องโอนเนื้อหาให้ผู้ใช้คนอื่น")
		return
	}
	if _, err := a.st.UserByID(to); err != nil {
		writeErr(w, http.StatusBadRequest, "ไม่พบผู้ใช้ที่จะรับโอนเนื้อหา")
		return
	}
	if err := a.db(r).DeleteUser(id, to); err != nil {
		serverErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// bulkGuard rejects bulk actions that would lock the caller out; "" = ok.
func bulkGuard(me int64, ids []int64, action, role, myRole string) string {
	for _, id := range ids {
		if id != me {
			continue
		}
		switch action {
		case "suspend":
			return "ไม่สามารถระงับบัญชีของตัวเองได้"
		case "delete":
			return "ไม่สามารถลบบัญชีของตัวเองได้"
		case "role":
			if role != myRole {
				return "ไม่สามารถเปลี่ยนสิทธิ์ของตัวเองได้"
			}
		}
	}
	return ""
}

func (a *API) bulkUsers(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs    []int64 `json:"ids"`
		Action string  `json:"action"`
		Role   string  `json:"role"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	me := auth.From(r)
	if len(in.IDs) == 0 {
		writeErr(w, http.StatusBadRequest, "กรุณาเลือกผู้ใช้")
		return
	}
	if msg := bulkGuard(me.ID, in.IDs, in.Action, in.Role, me.Role); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if me.Role != model.RoleAdmin {
		if in.Action == "role" && !canManageRole(me, in.Role) {
			writeErr(w, http.StatusForbidden, errRoleTooHigh)
			return
		}
		for _, id := range in.IDs {
			if u, err := a.st.UserByID(id); err == nil && !canManageRole(me, u.Role) {
				writeErr(w, http.StatusForbidden, errUserTooHigh)
				return
			}
		}
	}
	var n int64
	var err error
	switch in.Action {
	case "role":
		if !auth.ValidRole(in.Role) {
			writeErr(w, http.StatusBadRequest, "สิทธิ์การใช้งานไม่ถูกต้อง")
			return
		}
		n, err = a.db(r).SetUsersRole(in.IDs, in.Role)
	case "suspend":
		n, err = a.db(r).SetUsersStatus(in.IDs, model.UserSuspended)
	case "activate":
		n, err = a.db(r).SetUsersStatus(in.IDs, model.UserActive)
	case "delete":
		for _, id := range in.IDs {
			e := a.db(r).DeleteUser(id, me.ID)
			if e == nil {
				n++
			} else if !errors.Is(e, store.ErrNotFound) {
				err = e
				break
			}
		}
	default:
		writeErr(w, http.StatusBadRequest, "คำสั่งไม่ถูกต้อง")
		return
	}
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"updated": n})
}
