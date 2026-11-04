package api

import (
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // IANA zones for timezone validation on any OS

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/config"
	"github.com/anasaki/studio/backend/internal/demo"
	"github.com/anasaki/studio/backend/internal/media"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/site"
)

// DefaultUploadTypes is the default allowed_upload_types option.
const DefaultUploadTypes = media.DefaultUploadTypes

// settingDefaults lists every whitelisted setting with its default value.
var settingDefaults = map[string]string{
	"site_title":          "",
	"tagline":             "",
	"language":            "th",
	"home_page_id":        "",
	"post_template_id":    "",
	"archive_template_id": "",
	// the site header / footer chosen with "Set as header/footer" (site.OptionHeaderComponent)
	"site_header_component": "",
	"site_footer_component": "",
	"site_icon":             "",
	"site_logo":             "",
	"admin_email":           "",
	"timezone":              "Asia/Bangkok",
	"date_format":           "2 Jan 2006",
	"posts_per_page":        "10",
	"blog_base":             "blog",
	"category_base":         "category",
	"search_visibility":     "1",
	"admin_path":            "admin",
	"max_upload_mb":         "32",
	"smtp_host":             "",
	"smtp_port":             "",
	"smtp_user":             "",
	"smtp_password":         "",
	"smtp_encryption":       "starttls",
	"mail_from_email":       "",
	"mail_from_name":        "",
	"allowed_upload_types":  DefaultUploadTypes,
	"theme":                 "", // design tokens (JSON), see site.Theme
}

// corePaths cannot be used as admin path or URL bases: the server's own
// top-level routes (cmd/server/main.go, Routes, site.Site). Only the core's
// paths are listed here: the paths of plugins come from what the installed
// packages declare (plugin.Manager.DeclaredPaths, see reservedPaths).
var corePaths = map[string]bool{
	"api": true, "uploads": true, "download": true, "plugin-assets": true, "site-assets": true, "ext-ui": true, "setup": true,
	"_a": true, "_c": true, "sitemap.xml": true, "robots.txt": true, "favicon.ico": true, "favicon.svg": true,
	"assets": true, "static": true,
}

// reservedPaths returns every reserved first path segment → its owner: ""
// for the core, else the id of the installed plugin that declares it.
func (a *API) reservedPaths() map[string]string {
	out := map[string]string{}
	if a.plugins != nil {
		for seg, id := range a.plugins.DeclaredPaths() {
			out[seg] = id
		}
	}
	for seg := range corePaths {
		out[seg] = ""
	}
	return out
}

// ReservedPath is an entry of GET /api/v1/settings/reserved-paths.
type ReservedPath struct {
	Path   string `json:"path"`
	Plugin string `json:"plugin,omitempty"` // "" = the core
}

// reservedPathList answers GET /api/v1/settings/reserved-paths: the first
// path segments the admin path and the URL bases may not use (since 2.0.4).
func (a *API) reservedPathList(w http.ResponseWriter, r *http.Request) {
	items := []ReservedPath{}
	for seg, id := range a.reservedPaths() {
		items = append(items, ReservedPath{Path: seg, Plugin: id})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

var pathOptRe = regexp.MustCompile(`^[a-z0-9-]{3,32}$`)

// postTemplateKeyRe: the template of a Site Code post type's single posts.
var postTemplateKeyRe = regexp.MustCompile(`^post_template_[a-z][a-z0-9_]{1,19}$`)

// withDefaults returns every setting, defaults filled in for missing/empty values.
func withDefaults(opts map[string]string) map[string]string {
	out := make(map[string]string, len(settingDefaults))
	for k, def := range settingDefaults {
		v := opts[k]
		if v == "" {
			v = def
		}
		out[k] = v
	}
	return out
}

// validatePathOption checks an admin path / URL base value against the core
// paths and reserved (segment → plugin id, may be nil); "" = ok.
func validatePathOption(v string, reserved map[string]string) string {
	if !pathOptRe.MatchString(v) {
		return "ใช้ได้เฉพาะ a-z, 0-9 และ - (3-32 ตัวอักษร)"
	}
	if corePaths[v] {
		return "คำนี้ถูกสงวนไว้โดยระบบ: " + v
	}
	if id, ok := reserved[v]; ok {
		if id == "" {
			return "คำนี้ถูกสงวนไว้โดยระบบ: " + v
		}
		return "คำนี้ถูกใช้โดยปลั๊กอิน " + id + ": " + v
	}
	return ""
}

func isDigits(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func intIn(s string, lo, hi int) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= lo && n <= hi
}

// validateSettings validates the incoming values merged over current ones
// (merged must already contain both; reserved: see validatePathOption);
// returns a user-facing error or "".
func validateSettings(in, merged, reserved map[string]string) string {
	for k, v := range in {
		if postTemplateKeyRe.MatchString(k) && v != "" && !isDigits(v) {
			return "รหัสหน้าไม่ถูกต้อง: " + k
		}
		switch k {
		case "language":
			if v != "th" && v != "en" {
				return "ภาษาต้องเป็น th หรือ en"
			}
		case "home_page_id", "post_template_id", "archive_template_id", "site_header_component", "site_footer_component":
			if v != "" && !isDigits(v) {
				return "รหัสหน้าไม่ถูกต้อง: " + k
			}
		case "site_icon", "site_logo":
			if !safeURL(v) {
				return "ลิงก์รูปภาพไม่ถูกต้อง"
			}
		case "admin_email":
			if v != "" && !strings.Contains(v, "@") {
				return "อีเมลผู้ดูแลไม่ถูกต้อง"
			}
		case "timezone":
			if _, err := time.LoadLocation(v); err != nil || v == "" || strings.EqualFold(v, "local") {
				return "เขตเวลาไม่ถูกต้อง (เช่น Asia/Bangkok)"
			}
		case "date_format":
			if strings.TrimSpace(v) == "" || len(v) > 64 {
				return "รูปแบบวันที่ไม่ถูกต้อง"
			}
		case "posts_per_page":
			if !intIn(v, 1, 100) {
				return "จำนวนบทความต่อหน้าต้องอยู่ระหว่าง 1-100"
			}
		case "search_visibility":
			if v != "0" && v != "1" {
				return "search_visibility ต้องเป็น 0 หรือ 1"
			}
		case "max_upload_mb":
			if !intIn(v, 1, 512) {
				return "ขนาดอัปโหลดสูงสุดต้องอยู่ระหว่าง 1-512 MB"
			}
		case "allowed_upload_types":
			if _, msg := parseUploadTypes(v); msg != "" {
				return msg
			}
		case "smtp_port":
			if !intIn(v, 1, 65535) {
				return "พอร์ต SMTP ต้องเป็นตัวเลข 1-65535"
			}
		case "smtp_encryption":
			if v != "starttls" && v != "ssl" && v != "none" {
				return "การเข้ารหัสต้องเป็น starttls, ssl หรือ none"
			}
		case "mail_from_email":
			if !strings.Contains(v, "@") {
				return "อีเมลผู้ส่งไม่ถูกต้อง"
			}
		case "admin_path", "blog_base", "category_base":
			if msg := validatePathOption(v, reserved); msg != "" {
				return msg
			}
		case "theme":
			if msg := site.ValidateTheme(v); msg != "" {
				return msg
			}
		}
	}
	a, b, c := merged["admin_path"], merged["blog_base"], merged["category_base"]
	if a == b || a == c || b == c {
		return "admin path, blog base และ category base ต้องไม่ซ้ำกัน"
	}
	return ""
}

func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	opts, err := a.st.Options()
	if err != nil {
		serverErr(w, err)
		return
	}
	out := withDefaults(opts)
	for k, v := range opts { // per-type post templates of Site Code post types (post_template_<type>)
		if postTemplateKeyRe.MatchString(k) && k != "post_template_id" {
			out[k] = v
		}
	}
	if out["smtp_password"] != "" {
		out["smtp_password"] = secretMask
	}
	if !auth.From(r).Can(auth.CapManageOptions) {
		for k := range out {
			if !sharedSettings[k] && !postTemplateKeyRe.MatchString(k) {
				delete(out, k)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// sharedSettings are the settings every signed-in user may read (the admin
// app and the designer need them); mail and contact settings stay with manage_options.
var sharedSettings = map[string]bool{
	"site_title": true, "tagline": true, "language": true, "home_page_id": true, "post_template_id": true,
	"archive_template_id": true, "site_icon": true, "site_logo": true, "timezone": true, "date_format": true,
	"posts_per_page": true, "blog_base": true, "category_base": true, "search_visibility": true,
	"admin_path": true, "max_upload_mb": true, "allowed_upload_types": true, "theme": true,
	"site_header_component": true, "site_footer_component": true,
}

func (a *API) updateSettings(w http.ResponseWriter, r *http.Request) {
	var in map[string]string
	if !readJSON(w, r, &in) {
		return
	}
	if !auth.From(r).Can(auth.CapManageOptions) {
		ok := a.visitor(r)
		for k := range in {
			ok = ok && demo.VisitorSettings[k]
		}
		if !ok {
			writeErr(w, http.StatusForbidden, "คุณไม่มีสิทธิ์ทำรายการนี้")
			return
		}
		// a visitor's logo/icon is shown to everyone: only images uploaded
		// here (removed again by the next reset), never another site's file
		for _, k := range []string{"site_icon", "site_logo"} {
			if v, set := in[k]; set && strings.TrimSpace(v) != "" && !demo.UploadedFile(strings.TrimSpace(v)) {
				writeErr(w, http.StatusBadRequest, "ในเว็บทดลองใช้ได้เฉพาะรูปที่อัปโหลดในคลังสื่อ (/uploads/...)")
				return
			}
		}
	}
	for k, v := range in {
		if _, ok := settingDefaults[k]; !ok && !postTemplateKeyRe.MatchString(k) {
			delete(in, k)
			continue
		}
		in[k] = strings.TrimSpace(v)
	}
	if in["smtp_password"] == secretMask {
		delete(in, "smtp_password") // unchanged
	}
	if v, ok := in["allowed_upload_types"]; ok && v != "" {
		types, msg := parseUploadTypes(v)
		if msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		in["allowed_upload_types"] = strings.Join(types, ",")
	}
	opts, err := a.st.Options()
	if err != nil {
		serverErr(w, err)
		return
	}
	merged := withDefaults(opts)
	for k, v := range in {
		if v == "" {
			v = settingDefaults[k]
		}
		merged[k] = v
	}
	check := map[string]string{}
	for k, v := range in {
		if v != "" { // empty = reset to default
			check[k] = v
		}
	}
	if msg := validateSettings(check, merged, a.reservedPaths()); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := a.db(r).SaveSettings(in); err != nil {
		serverErr(w, err)
		return
	}
	a.adminPath.reset()
	a.getSettings(w, r)
}

// publicSite answers GET /api/public/site (no auth): what the login screen
// and the setup wizard need to brand themselves. It never tells the admin
// path (the admin knows it from its own URL — a hidden login page stays
// hidden) and tells the version only to a signed-in session (audit L-4).
func (a *API) publicSite(w http.ResponseWriter, r *http.Request) {
	opts, err := a.st.Options()
	if err != nil {
		serverErr(w, err)
		return
	}
	o := withDefaults(opts)
	out := map[string]any{
		"demo":      a.demo.Status(),
		"siteTitle": o["site_title"], "tagline": o["tagline"], "siteIcon": o["site_icon"], "siteLogo": o["site_logo"],
		"product": config.Product,
	}
	if a.signedIn(r) {
		out["version"] = config.Version
	}
	writeJSON(w, http.StatusOK, out)
}

// signedIn reports whether r carries a valid, current session token.
func (a *API) signedIn(r *http.Request) bool {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	p, err := a.auth.Parse(tok)
	if err != nil {
		return false
	}
	u, hash, revoked, err := a.st.SessionCheck(p.ID, p.JTI)
	return err == nil && !revoked && a.auth.Current(p, hash) && u.Status != model.UserSuspended
}

// pathCache caches the admin_path option for a few seconds, since it is
// consulted on every non-API request.
type pathCache struct {
	mu    sync.Mutex
	value string
	at    time.Time
}

func (c *pathCache) reset() {
	c.mu.Lock()
	c.at = time.Time{}
	c.mu.Unlock()
}

// AdminPath returns the URL segment the admin SPA is served under ("admin"
// unless changed in the settings). The ADMIN_PATH environment variable wins,
// so an owner who forgot a custom path can always get back in.
func (a *API) AdminPath() string {
	if v := strings.Trim(os.Getenv("ADMIN_PATH"), "/"); v != "" {
		return v
	}
	c := &a.adminPath
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value != "" && time.Since(c.at) < 5*time.Second {
		return c.value
	}
	v := settingDefaults["admin_path"]
	if opts, err := a.st.Options(); err == nil && opts["admin_path"] != "" {
		v = opts["admin_path"]
	} else if err != nil && c.value != "" {
		v = c.value // keep the last known value while the database hiccups
	}
	c.value, c.at = v, time.Now()
	return v
}
