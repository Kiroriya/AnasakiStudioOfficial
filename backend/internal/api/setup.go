package api

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anasaki/studio/backend/internal/config"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/seed"
	"github.com/anasaki/studio/backend/internal/store"
)

// SetSetupToken makes POST /setup require token (SETUP_TOKEN; "" = none).
func (a *API) SetSetupToken(token string) { a.setupToken = strings.TrimSpace(token) }

// getSetup answers GET /api/v1/setup (no auth): whether the first-run
// wizard must be shown and whether it needs the setup token. The version is
// only told while the site is not set up yet (audit L-4).
func (a *API) getSetup(w http.ResponseWriter, r *http.Request) {
	n, err := a.st.CountUsers()
	if err != nil {
		serverErr(w, err)
		return
	}
	out := map[string]any{"needsSetup": n == 0, "product": config.Product}
	if n == 0 {
		out["version"] = config.Version
		out["tokenRequired"] = a.setupToken != ""
	}
	writeJSON(w, http.StatusOK, out)
}

type setupInput struct {
	SiteTitle     string `json:"siteTitle"`
	Tagline       string `json:"tagline"`
	Language      string `json:"language"`
	AdminName     string `json:"adminName"`
	AdminEmail    string `json:"adminEmail"`
	AdminPassword string `json:"adminPassword"`
	Locale        string `json:"locale"`
	Demo          bool   `json:"demo"`
	// SetupToken must match SETUP_TOKEN when the server sets one (audit M-5).
	SetupToken string `json:"setupToken"`
}

func (in *setupInput) validate() string {
	in.SiteTitle = strings.TrimSpace(in.SiteTitle)
	in.Tagline = strings.TrimSpace(in.Tagline)
	in.AdminName = strings.TrimSpace(in.AdminName)
	in.AdminEmail = strings.ToLower(strings.TrimSpace(in.AdminEmail))
	if in.Language == "" {
		in.Language = model.LocaleTH
	}
	if in.Locale == "" {
		in.Locale = in.Language
	}
	switch {
	case in.SiteTitle == "":
		return "กรุณาใส่ชื่อเว็บไซต์"
	case in.AdminName == "":
		return "กรุณาใส่ชื่อผู้ดูแล"
	case !strings.Contains(in.AdminEmail, "@"):
		return "อีเมลไม่ถูกต้อง"
	case len(in.AdminPassword) < 8:
		return "รหัสผ่านต้องมีอย่างน้อย 8 ตัวอักษร"
	case !model.ValidSiteLanguage(in.Language) || !model.ValidLocale(in.Locale):
		return "ภาษาของเว็บต้องเป็น th หรือ en และภาษาของหน้าจัดการต้องเป็น th, en, ja, zh หรือ ko"
	}
	return ""
}

// runSetup answers POST /api/v1/setup (no auth, only while there is no
// user): creates the administrator, the site options and the starter content.
func (a *API) runSetup(w http.ResponseWriter, r *http.Request) {
	var in setupInput
	if !readJSON(w, r, &in) {
		return
	}
	a.setupMu.Lock()
	defer a.setupMu.Unlock()
	n, err := a.st.CountUsers()
	if err != nil {
		serverErr(w, err)
		return
	}
	if n > 0 {
		writeErr(w, http.StatusConflict, "ติดตั้งระบบเรียบร้อยแล้ว")
		return
	}
	if a.setupToken != "" {
		key := "setup:" + limiterIP(plugin.ClientIP(r))
		if !loginFails.allow(key) {
			writeErr(w, http.StatusTooManyRequests, "ใส่ setup token ผิดหลายครั้งเกินไป กรุณารอ 15 นาทีแล้วลองใหม่")
			return
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(in.SetupToken)), []byte(a.setupToken)) != 1 {
			loginFails.fail(key)
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "setup token ไม่ถูกต้อง — ดูค่า SETUP_TOKEN ที่ตั้งไว้ตอนเปิดเซิร์ฟเวอร์ / Wrong setup token (the SETUP_TOKEN the server was started with)", "setupToken": true})
			return
		}
	}
	if msg := in.validate(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := seed.Roles(a.st); err != nil {
		serverErr(w, err)
		return
	}
	_ = a.reloadRoles()
	// the steps below are not one transaction (the seeders use the store):
	// on a failure everything they added is removed again, so the wizard
	// can simply be run once more instead of finding an admin but no site
	mark, err := a.st.MarkSetup()
	if err != nil {
		serverErr(w, err)
		return
	}
	admin, err := seed.Admin(a.st.As(store.Actor{Source: "setup"}), in.AdminName, in.AdminEmail, in.AdminPassword, in.Locale)
	if err == nil {
		if in.Demo {
			err = seed.Demo(a.st.Quiet(), a.uploadDir, admin.ID) // sample content: no events
		} else {
			err = seed.Blank(a.st.Quiet(), admin.ID)
		}
	}
	if err == nil {
		err = a.st.SetOptions(map[string]string{"site_title": in.SiteTitle, "tagline": in.Tagline,
			"language": in.Language, "admin_email": in.AdminEmail})
	}
	if err != nil {
		a.rollbackSetup(mark)
		serverErr(w, err)
		return
	}
	a.issueToken(w, r, admin, http.StatusCreated)
}

// rollbackSetup undoes a failed setup: its rows and uploaded files.
func (a *API) rollbackSetup(mark store.SetupMark) {
	urls, err := a.st.RollbackSetup(mark)
	if err != nil {
		log.Printf("setup: rollback: %v", err)
		return
	}
	for _, u := range urls {
		if rel, ok := strings.CutPrefix(u, "/uploads/"); ok && !strings.Contains(rel, "..") {
			_ = os.Remove(filepath.Join(a.uploadDir, filepath.FromSlash(rel)))
		}
	}
}
