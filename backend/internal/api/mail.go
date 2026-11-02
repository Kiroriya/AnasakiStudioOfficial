package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/config"
	"github.com/anasaki/studio/backend/internal/demo"
	"github.com/anasaki/studio/backend/internal/mail"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// secretMask is what GET /settings shows instead of the SMTP password;
// sending it back unchanged keeps the stored password.
const secretMask = "••••••••"

// resetTTL is how long a reset / set-password link works.
const resetTTL = 2 * time.Hour

// welcomeTTL is how long the "set your password" link in a welcome email works.
const welcomeTTL = 72 * time.Hour

func (a *API) mailRoutes(h func(pattern, cap string, fn http.HandlerFunc), mux *http.ServeMux) {
	const p = "/api/v1"
	mux.HandleFunc("POST "+p+"/auth/forgot", a.forgotPassword)
	mux.HandleFunc("POST "+p+"/auth/reset", a.resetPassword)
	// the users screen offers welcome e-mails when mail works (manage_users); settings test it
	h("GET "+p+"/mail/status", "", needAny(a.mailStatus, auth.CapManageUsers, auth.CapManageOptions))
	h("POST "+p+"/mail/test", auth.CapManageOptions, a.testMail)
}

// mailConfig reads the SMTP settings.
func (a *API) mailConfig() (mail.Config, map[string]string, error) {
	opts, err := a.st.Options()
	if err != nil {
		return mail.Config{}, nil, err
	}
	o := withDefaults(opts)
	c, err := mail.FromOptions(o)
	return c, o, err
}

// SendMail sends a branded email; plugins use it through the host.
func (a *API) SendMail(to []string, subject, html, replyTo string) error {
	c, _, err := a.mailConfig()
	if err != nil {
		return err
	}
	return mail.Send(c, mail.Message{To: to, Subject: subject, HTML: html, ReplyTo: replyTo})
}

func (a *API) mailStatus(w http.ResponseWriter, r *http.Request) {
	_, _, err := a.mailConfig()
	writeJSON(w, http.StatusOK, map[string]bool{"configured": err == nil})
}

// testMail sends a test message (to the given address or the admin email).
func (a *API) testMail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		To string `json:"to"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	c, o, err := a.mailConfig()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	to := strings.TrimSpace(in.To)
	if to == "" {
		to = o["admin_email"]
	}
	if to == "" {
		writeErr(w, http.StatusBadRequest, "กรุณาระบุอีเมลผู้รับ")
		return
	}
	body := mail.Email{Site: o["site_title"], Heading: "ทดสอบระบบอีเมล ✅",
		Paragraphs: []string{"ถ้าคุณเห็นข้อความนี้ แปลว่าเว็บไซต์ส่งอีเมลได้แล้ว — ระบบจะใช้ส่งการแจ้งเตือนแบบฟอร์ม ลิงก์ตั้งรหัสผ่านใหม่ และอีเมลต้อนรับผู้ใช้ใหม่",
			"This is a test email from your website. Email delivery works."}}.Render()
	if err := mail.Send(c, mail.Message{To: []string{to}, Subject: "ทดสอบอีเมลจาก " + o["site_title"], HTML: body}); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sentTo": to})
}

// ---------------------------------------------------------------- password reset

func hashToken(t string) string {
	s := sha256.Sum256([]byte(t))
	return hex.EncodeToString(s[:])
}

// newResetToken stores a one-time token for u and returns it.
func (a *API) newResetToken(userID int64, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(b)
	_, _ = a.st.DB.Exec(`DELETE FROM password_resets WHERE user_id = ? OR expires_at < ?`, userID, time.Now().UTC())
	_, err := a.st.DB.Exec(`INSERT INTO password_resets (token_hash, user_id, expires_at) VALUES (?,?,?)`,
		hashToken(tok), userID, time.Now().UTC().Add(ttl))
	return tok, err
}

// adminURL is the absolute URL of an admin screen. Emailed links must not
// follow a forged Host header, so the base comes from the site_url option or
// the SITE_URL environment variable; the request host is only a fallback.
func (a *API) adminURL(r *http.Request, path string) string {
	return a.siteBase(r) + "/" + a.AdminPath() + "/" + strings.TrimLeft(path, "/")
}

func (a *API) siteBase(r *http.Request) string {
	clean := func(u string) string {
		u = strings.TrimRight(strings.TrimSpace(u), "/")
		if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			return u
		}
		return ""
	}
	if opts, err := a.st.Options(); err == nil {
		if u := clean(opts["site_url"]); u != "" {
			return u
		}
	}
	if u := clean(config.Env("SITE_URL", "")); u != "" {
		return u
	}
	scheme := plugin.RequestScheme(r)
	log.Printf("warning: SITE_URL is not set — email links use the request host %q (set SITE_URL=https://your-domain)", r.Host)
	return scheme + "://" + r.Host
}

// forgotPassword emails a reset link. It always answers the same way, so it
// cannot be used to find out which emails have accounts.
func (a *API) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	c, o, err := a.mailConfig()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "เว็บไซต์นี้ยังไม่ได้ตั้งค่าระบบอีเมล กรุณาติดต่อผู้ดูแลเว็บ")
		return
	}
	if !a.allowReset(r) {
		writeErr(w, http.StatusTooManyRequests, "ขอรีเซ็ตบ่อยเกินไป กรุณารอสักครู่")
		return
	}
	ok := map[string]string{"message": "ถ้าอีเมลนี้มีบัญชีอยู่ เราได้ส่งลิงก์ตั้งรหัสผ่านใหม่ไปให้แล้ว"}
	email := strings.TrimSpace(strings.ToLower(in.Email))
	// a few mails per address and hour, whatever the IP (audit L-2); the
	// answer stays the same, so it tells nothing about the address
	if !allowResetEmail(email) {
		writeJSON(w, http.StatusOK, ok)
		return
	}
	u, _, err := a.st.UserByEmail(email)
	if err != nil || u.Status == model.UserSuspended || (a.demo != nil && u.Role == demo.Role) {
		writeJSON(w, http.StatusOK, ok)
		return
	}
	tok, err := a.newResetToken(u.ID, resetTTL)
	if err != nil {
		serverErr(w, err)
		return
	}
	link := a.adminURL(r, "reset?token="+tok)
	body := mail.Email{Site: o["site_title"], Heading: "ตั้งรหัสผ่านใหม่ / Reset your password",
		Paragraphs: []string{"สวัสดี " + u.Name + ",\nมีคำขอตั้งรหัสผ่านใหม่สำหรับบัญชีของคุณ กดปุ่มด้านล่างภายใน 2 ชั่วโมง",
			"ถ้าคุณไม่ได้ขอ ไม่ต้องทำอะไร รหัสผ่านเดิมยังใช้ได้ตามปกติ",
			"Someone asked to reset your password. Use the button within 2 hours, or ignore this email."},
		Button: "ตั้งรหัสผ่านใหม่", ButtonURL: link}.Render()
	go func() {
		if err := mail.Send(c, mail.Message{To: []string{u.Email}, Subject: "ตั้งรหัสผ่านใหม่ — " + o["site_title"], HTML: body}); err != nil {
			log.Printf("mail: reset for %s: %v", u.Email, err)
		}
	}()
	writeJSON(w, http.StatusOK, ok)
}

// resetPassword sets a new password with a token from the email.
func (a *API) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "รหัสผ่านต้องมีอย่างน้อย 8 ตัวอักษร")
		return
	}
	var userID int64
	err := a.st.DB.QueryRow(`SELECT user_id FROM password_resets WHERE token_hash = ? AND expires_at > ?`,
		hashToken(strings.TrimSpace(in.Token)), time.Now().UTC()).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusBadRequest, "ลิงก์นี้หมดอายุหรือถูกใช้ไปแล้ว กรุณาขอลิงก์ใหม่")
		return
	}
	if err != nil {
		serverErr(w, err)
		return
	}
	u, err := a.st.UserByID(userID)
	if err != nil {
		serverErr(w, err)
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		serverErr(w, err)
		return
	}
	if err := a.st.UpdateUser(u, hash); err != nil {
		serverErr(w, err)
		return
	}
	_, _ = a.st.DB.Exec(`DELETE FROM password_resets WHERE user_id = ?`, userID)
	if u.Status == model.UserSuspended {
		writeErr(w, http.StatusForbidden, "บัญชีนี้ถูกระงับ")
		return
	}
	a.issueToken(w, u, http.StatusOK)
}

// sendWelcome emails a new user their login address and a link to choose
// their own password (the admin never has to share a password).
func (a *API) sendWelcome(r *http.Request, u *model.User) {
	c, o, err := a.mailConfig()
	if err != nil {
		return
	}
	tok, err := a.newResetToken(u.ID, welcomeTTL)
	if err != nil {
		log.Printf("mail: welcome token: %v", err)
		return
	}
	body := mail.Email{Site: o["site_title"], Heading: "ยินดีต้อนรับสู่ " + o["site_title"],
		Paragraphs: []string{"สวัสดี " + u.Name + ",\nคุณได้รับเชิญให้เข้าร่วมจัดการเว็บไซต์ " + o["site_title"] + " — ตั้งรหัสผ่านของคุณได้จากปุ่มด้านล่าง (ลิงก์ใช้ได้ 3 วัน)",
			"You have been invited to help manage this website. Choose your password with the button below."},
		Rows:   [][2]string{{"อีเมลเข้าสู่ระบบ", u.Email}, {"หน้าเข้าสู่ระบบ", a.adminURL(r, "login")}},
		Button: "ตั้งรหัสผ่านและเริ่มใช้งาน", ButtonURL: a.adminURL(r, "reset?token="+tok+"&welcome=1")}.Render()
	go func() {
		if err := mail.Send(c, mail.Message{To: []string{u.Email}, Subject: "ยินดีต้อนรับสู่ " + o["site_title"], HTML: body}); err != nil {
			log.Printf("mail: welcome %s: %v", u.Email, err)
		}
	}()
}

// windowLimiter allows max events per key in a sliding window; keys whose
// events are all older than the window are swept regularly, so the map can't
// grow without bound.
type windowLimiter struct {
	sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
	swept  time.Time
}

func newWindowLimiter(max int, window time.Duration) *windowLimiter {
	return &windowLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *windowLimiter) allow(key string) bool {
	now := time.Now()
	l.Lock()
	defer l.Unlock()
	if now.Sub(l.swept) > l.window || len(l.hits) > 10000 {
		for k, ts := range l.hits {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) >= l.window {
				delete(l.hits, k)
			}
		}
		l.swept = now
	}
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

// resetIPs: reset requests per client IP; resetEmails: per address (any IP).
var (
	resetIPs    = newWindowLimiter(5, 15*time.Minute)
	resetEmails = newWindowLimiter(3, time.Hour)
)

func (a *API) allowReset(r *http.Request) bool {
	return resetIPs.allow(limiterIP(plugin.ClientIP(r)))
}

func allowResetEmail(email string) bool { return resetEmails.allow(email) }

// randomPassword is an unguessable placeholder for accounts whose owner sets
// the real password from the welcome email.
func randomPassword() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SendMailMessage sends a message (Site Code's mail.send) through the site's SMTP settings.
func (a *API) SendMailMessage(m mail.Message) error {
	c, _, err := a.mailConfig()
	if err != nil {
		return err
	}
	return mail.Send(c, m)
}
