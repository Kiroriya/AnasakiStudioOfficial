package plugin

// Service plugins: a package plugin whose manifest has a "service" section
// ships a server program that runs next to the CMS (usually its own Docker
// container). The CMS stays in charge and talks to it over HTTP:
//
//	CMS → service (every request signed with the plugin's shared secret)
//	  GET  /health                      connection test → {"name","version"}
//	  ANY  /public/<path>               public site paths listed in service.public
//	  ANY  /admin/<path>                the plugin's admin screens (shown in an iframe)
//	  POST /blocks/<type>               designer blocks with "render":"service" → {"html","css"}
//	  POST /collections/<source>        collection data → {"items":[{...}]}
//	  POST /events                      core events listed in service.events (async)
//
//	service → CMS (signed the same way): /api/service/... (see api/service.go)
//
// Signature v1: X-Anasaki-Signature = hex(HMAC-SHA256(secret,
// "<unix ts>\n<METHOD>\n<request URI>\n<hex sha256(body)>")), with
// X-Anasaki-Timestamp and X-Anasaki-Plugin. Requests older than 5 minutes are
// refused.
//
// Signature v2 (sent alongside v1, so older services keep working):
// X-Anasaki-Nonce (random hex) and X-Anasaki-Signature-V2 = hex(HMAC-SHA256(
// secret, "v2\n<ts>\n<nonce>\n<METHOD>\n<request URI>\n<hex sha256(body)>\n
// <X-Anasaki-Plugin>\n<X-Anasaki-User>\n<X-Anasaki-Admin-Base>")). A verifier
// that sees v2 checks it instead of v1 and refuses a nonce it has already
// seen within the 5-minute window; once a peer has sent v2, v1-only requests
// from it are refused (no downgrade). The SDK in sdk/go does all of this.

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
)

// Headers of the service protocol.
const (
	HeaderPlugin    = "X-Anasaki-Plugin"
	HeaderTimestamp = "X-Anasaki-Timestamp"
	HeaderSignature = "X-Anasaki-Signature"
	HeaderSigV2     = "X-Anasaki-Signature-V2" // v2: also covers the nonce and the user headers
	HeaderNonce     = "X-Anasaki-Nonce"        // v2: random per request (replay protection)
	HeaderUser      = "X-Anasaki-User"         // admin screens: base64(JSON ServiceUser)
	HeaderSiteURL   = "X-Anasaki-Site-Url"     // public base URL of the site
	HeaderLayout    = "X-Anasaki-Layout"       // response: "page" = wrap with the site's header/footer
	HeaderTitle     = "X-Anasaki-Title"        // response: page title (URL-encoded UTF-8)
	HeaderAdminBase = "X-Anasaki-Admin-Base"   // admin screens: URL prefix of the screen ("/ext-ui/<id>/<ticket>/")
)

// ServiceAPIScopes are what a service may ask for in service.api.
var ServiceAPIScopes = map[string]bool{"site": true, "mail": true, "content": true, "users": true, "events": true,
	"storage": true, "http": true}

// ManifestService is the "service" section of a manifest.
type ManifestService struct {
	// Public path prefixes the service answers on the public site ("/shop").
	Public []string `json:"public"`
	// Core events POSTed to /events ("post.published", … or "*").
	Events []string `json:"events"`
	// Data sources for the designer's collection block.
	Collections []ServiceCollection `json:"collections"`
	// Admin screens, added to the sidebar while the plugin is active.
	Admin []ServiceAdminPage `json:"admin"`
	// CMS service API scopes the service may call (site, mail, content, users,
	// events, storage, http).
	API []string `json:"api"`
	// Runtime "wasm": the program is Module (a .wasm file in the package) and
	// runs inside the CMS — nothing to set up. "" = a separate service.
	Runtime string `json:"runtime,omitempty"`
	Module  string `json:"module,omitempty"`
	// Hosts the plugin may call with the "http" scope (https only; "*.x.com" allowed).
	Hosts []string `json:"hosts,omitempty"`
	// Functions code can call with plugins.call(id, fn, input): the CMS
	// POSTs {input, user, visitor} to /functions/<fn> (signed).
	Functions []string `json:"functions,omitempty"`
}

// ServiceCollection is a collection source served by the service.
type ServiceCollection struct {
	Source string `json:"source"`
	Label  string `json:"label"`
}

// ServiceAdminPage is an admin screen served by the service.
type ServiceAdminPage struct {
	Path    string `json:"path"`
	Label   string `json:"label"`
	LabelEn string `json:"labelEn"`
	Icon    string `json:"icon"`
	Cap     string `json:"cap"` // who may open it ("" = manage_options; "<own id>.<capability>" since 2.0.6)
}

var (
	servicePathRe   = regexp.MustCompile(`^/[a-z0-9][a-z0-9_-]*(/[a-z0-9_-]+)*$`)
	serviceSourceRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,39}$`)
	adminPageRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	capNameRe       = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)
)

// reservedPublic are first path segments a service can never take over.
var reservedPublic = map[string]bool{"api": true, "uploads": true, "download": true, "plugin-assets": true, "site-assets": true,
	"ext-ui": true, "assets": true, "static": true, "favicon.ico": true, "favicon.svg": true, "sitemap.xml": true,
	"robots.txt": true, "admin": true}

func (s *ManifestService) validate() error {
	if len(s.Public) > 20 || len(s.Events) > 20 || len(s.Collections) > 10 || len(s.Admin) > 10 {
		return pkgErr("manifest: service: too many public paths, events, collections or admin pages")
	}
	for _, p := range s.Public {
		first, _, _ := strings.Cut(strings.TrimPrefix(p, "/"), "/")
		if len(p) > 60 || !servicePathRe.MatchString(p) || reservedPublic[first] {
			return pkgErr("manifest: service: public path %q is invalid or reserved", p)
		}
	}
	for _, e := range s.Events {
		if e != "*" && !eventRe.MatchString(e) {
			return pkgErr("manifest: service: invalid event %q", e)
		}
	}
	for _, c := range s.Collections {
		if !serviceSourceRe.MatchString(c.Source) || strings.TrimSpace(c.Label) == "" || len(c.Label) > 80 {
			return pkgErr("manifest: service: collection %q needs a valid source and a label", c.Source)
		}
	}
	for _, a := range s.Admin {
		if !adminPageRe.MatchString(a.Path) || strings.TrimSpace(a.Label) == "" || len(a.Label) > 60 {
			return pkgErr("manifest: service: admin page %q needs a valid path and a label", a.Path)
		}
		if a.Cap != "" && !capNameRe.MatchString(a.Cap) && !auth.IsPluginCap(a.Cap) {
			return pkgErr("manifest: service: admin page %q: invalid cap", a.Path)
		}
	}
	for _, a := range s.API {
		if !ServiceAPIScopes[a] {
			return pkgErr("manifest: service: unknown api scope %q (site, mail, content, users, events, storage, http)", a)
		}
	}
	if len(s.Hosts) > 20 {
		return pkgErr("manifest: service: too many hosts")
	}
	if len(s.Functions) > 50 {
		return pkgErr("manifest: service: too many functions")
	}
	for _, f := range s.Functions {
		if !fnNameRe.MatchString(f) {
			return pkgErr("manifest: service: invalid function name %q", f)
		}
	}
	for _, h := range s.Hosts {
		if !hostRe.MatchString(h) {
			return pkgErr("manifest: service: invalid host %q", h)
		}
	}
	switch s.Runtime {
	case "":
		if s.Module != "" {
			return pkgErr(`manifest: service: module needs "runtime": "wasm"`)
		}
	case "wasm":
		if !strings.HasSuffix(s.Module, ".wasm") {
			return pkgErr(`manifest: service: a wasm runtime needs "module": "<file>.wasm"`)
		}
	default:
		return pkgErr(`manifest: service: unknown runtime %q (use "wasm" or leave it out)`, s.Runtime)
	}
	return nil
}

var hostRe = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// ServiceView is the service part of an installed package (admin only).
type ServiceView struct {
	URL         string              `json:"url"`
	Secret      string              `json:"secret"`
	Public      []string            `json:"public"`
	Events      []string            `json:"events"`
	Collections []ServiceCollection `json:"collections"`
	Admin       []ServiceAdminPage  `json:"admin"`
	API         []string            `json:"api"`
	Runtime     string              `json:"runtime"` // "wasm" = runs inside the CMS
	Hosts       []string            `json:"hosts"`
}

// ServiceUser is who opened an admin screen (sent base64-JSON in X-Anasaki-User).
type ServiceUser struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Role   string   `json:"role"`
	Caps   []string `json:"caps"`
	Locale string   `json:"locale"`
}

// ServiceHealth is the result of a connection test.
type ServiceHealth struct {
	OK      bool   `json:"ok"`
	Status  int    `json:"status"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Error   string `json:"error"`
	Ms      int64  `json:"ms"`
}

// ---------------------------------------------------------------- signing

// ServiceSignature computes the request signature (see the package comment).
func ServiceSignature(secret string, ts int64, method, requestURI string, body []byte) string {
	sum := sha256.Sum256(body)
	h := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(h, "%d\n%s\n%s\n%x", ts, strings.ToUpper(method), requestURI, sum)
	return hex.EncodeToString(h.Sum(nil))
}

// ServiceSignatureV2 computes the v2 signature (see the package comment).
func ServiceSignatureV2(secret string, ts int64, nonce, method, requestURI string, body []byte,
	plugin, user, adminBase string) string {
	sum := sha256.Sum256(body)
	h := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(h, "v2\n%d\n%s\n%s\n%s\n%x\n%s\n%s\n%s", ts, nonce, strings.ToUpper(method), requestURI, sum,
		plugin, user, adminBase)
	return hex.EncodeToString(h.Sum(nil))
}

// signServiceRequest sets the timestamp, nonce and both signatures on req.
// Call it after every header the v2 signature covers (plugin, user, admin
// base) is set.
func signServiceRequest(secret string, req *http.Request, body []byte) {
	ts := time.Now().Unix()
	nb := make([]byte, 16)
	_, _ = rand.Read(nb)
	nonce := hex.EncodeToString(nb)
	uri := req.URL.RequestURI()
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSignature, ServiceSignature(secret, ts, req.Method, uri, body))
	req.Header.Set(HeaderSigV2, ServiceSignatureV2(secret, ts, nonce, req.Method, uri, body,
		req.Header.Get(HeaderPlugin), req.Header.Get(HeaderUser), req.Header.Get(HeaderAdminBase)))
}

// ErrBadSignature is returned for an unsigned, stale, replayed or forged request.
var ErrBadSignature = errors.New("bad service signature")

// signatureWindow is how far a request's timestamp may be from now.
const signatureWindow = 5 * time.Minute

var nonceRe = regexp.MustCompile(`^[0-9a-f]{16,64}$`)

// VerifyServiceSignature checks a signed request (body already read): the v2
// signature (and its nonce, once) when the request carries one, else v1.
func VerifyServiceSignature(secret string, r *http.Request, body []byte) error {
	ts, err := strconv.ParseInt(r.Header.Get(HeaderTimestamp), 10, 64)
	if err != nil || secret == "" {
		return ErrBadSignature
	}
	if d := time.Since(time.Unix(ts, 0)); d > signatureWindow || d < -signatureWindow {
		return ErrBadSignature
	}
	if v2 := r.Header.Get(HeaderSigV2); v2 != "" {
		nonce := r.Header.Get(HeaderNonce)
		if !nonceRe.MatchString(nonce) {
			return ErrBadSignature
		}
		want := ServiceSignatureV2(secret, ts, nonce, r.Method, r.URL.RequestURI(), body,
			r.Header.Get(HeaderPlugin), r.Header.Get(HeaderUser), r.Header.Get(HeaderAdminBase))
		if !hmac.Equal([]byte(want), []byte(v2)) {
			return ErrBadSignature
		}
		if !serviceNonces.add(r.Header.Get(HeaderPlugin)+":"+nonce, time.Now()) {
			return ErrBadSignature // replayed
		}
		return nil
	}
	want := ServiceSignature(secret, ts, r.Method, r.URL.RequestURI(), body)
	if !hmac.Equal([]byte(want), []byte(r.Header.Get(HeaderSignature))) {
		return ErrBadSignature
	}
	return nil
}

// nonceCache remembers the nonces of verified requests long enough to cover
// the signature window (bounded: past max entries the oldest are dropped).
type nonceCache struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	order []nonceEntry // insertion order = expiry order
	max   int
}

type nonceEntry struct {
	nonce string
	exp   time.Time
}

func newNonceCache(max int) *nonceCache { return &nonceCache{seen: map[string]struct{}{}, max: max} }

// add records nonce; false when it was already seen.
func (c *nonceCache) add(nonce string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.order) > 0 && (now.After(c.order[0].exp) || len(c.order) >= c.max) {
		delete(c.seen, c.order[0].nonce)
		c.order = c.order[1:]
	}
	if _, dup := c.seen[nonce]; dup {
		return false
	}
	c.seen[nonce] = struct{}{}
	// a timestamp is accepted up to signatureWindow after now, and stays
	// acceptable for another signatureWindow: keep the nonce that long
	c.order = append(c.order, nonceEntry{nonce, now.Add(2 * signatureWindow)})
	return true
}

var (
	serviceNonces = newNonceCache(1 << 18)
	// v2Plugins are the plugins whose service has signed with v2: a v1-only
	// request claiming to be from them is a downgrade and is refused.
	v2Plugins sync.Map
)

// NewServiceSecret returns a random shared secret.
func NewServiceSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------- storage

// migrateService adds the service columns (and consent_pending, the JSON
// Reconsent of an upgrade awaiting approval) to plugin_packages.
func migrateService(db *sql.DB) error {
	for _, c := range [][2]string{
		{"service_url", "VARCHAR(500) NOT NULL DEFAULT ''"},
		{"service_secret", "VARCHAR(100) NOT NULL DEFAULT ''"},
		{"consent_pending", "TEXT NULL"},
		{"code_error", "TEXT NULL"},
		// the Store publisher an installed package is bound to (audit H-2)
		{"publisher", "VARCHAR(64) NOT NULL DEFAULT ''"},
	} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'plugin_packages' AND COLUMN_NAME = ?`, c[0]).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := db.Exec("ALTER TABLE plugin_packages ADD COLUMN `" + c[0] + "` " + c[1]); err != nil {
				return fmt.Errorf("plugin_packages.%s: %w", c[0], err)
			}
		}
	}
	return nil
}

// hasService reports whether the package declares a service.
func (k *pkg) hasService() bool { return k.mf.Service != nil && k.mf.HasPermission("service") }

// isWasm reports whether the service runs as a WASM module inside the CMS.
func (k *pkg) isWasm() bool { return k.hasService() && k.mf.Service.Runtime == "wasm" }

// serviceReady: declared, active and runnable (a WASM module, or a connected
// service with URL + secret).
func (k *pkg) serviceReady() bool {
	return k.active && k.hasService() && (k.isWasm() || (k.serviceURL != "" && k.secret != ""))
}

// do sends a request to the plugin's program: the WASM module or the service.
func (p *packages) do(k *pkg, req *http.Request) (*http.Response, error) {
	if k.isWasm() {
		return wasm.roundTrip(p, k, req)
	}
	return serviceClient.Do(req)
}

func (k *pkg) serviceView() *ServiceView {
	if !k.hasService() {
		return nil
	}
	s := k.mf.Service
	v := &ServiceView{URL: k.serviceURL, Secret: k.secret, Public: s.Public, Events: s.Events,
		Collections: s.Collections, Admin: s.Admin, API: s.API, Runtime: s.Runtime, Hosts: s.Hosts}
	if k.isWasm() {
		v.URL, v.Secret = "", "" // runs inside the CMS: nothing to connect
	}
	if v.Hosts == nil {
		v.Hosts = []string{}
	}
	if v.Public == nil {
		v.Public = []string{}
	}
	if v.Events == nil {
		v.Events = []string{}
	}
	if v.Collections == nil {
		v.Collections = []ServiceCollection{}
	}
	if v.Admin == nil {
		v.Admin = []ServiceAdminPage{}
	}
	if v.API == nil {
		v.API = []string{}
	}
	return v
}

// SetServiceURL saves where the plugin's service listens (http/https).
func (m *Manager) SetServiceURL(id, raw string) (Installed, error) {
	k := m.pkgs.get(id)
	if k == nil {
		return Installed{}, ErrUnknownPackage
	}
	if !k.hasService() {
		return Installed{}, pkgErr("this plugin has no service")
	}
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return Installed{}, pkgErr("the service URL must look like http://shop:9000 or https://plugin.example.com")
		}
	}
	if _, err := m.pkgs.db.Exec(`UPDATE plugin_packages SET service_url=? WHERE id=?`, raw, id); err != nil {
		return Installed{}, err
	}
	v2Plugins.Delete(id) // a new address may be another build of the service
	m.pkgs.invalidate()
	return m.GetPackage(id)
}

// RotateServiceSecret issues a new shared secret (the service must be given it).
func (m *Manager) RotateServiceSecret(id string) (Installed, error) {
	k := m.pkgs.get(id)
	if k == nil {
		return Installed{}, ErrUnknownPackage
	}
	if !k.hasService() {
		return Installed{}, pkgErr("this plugin has no service")
	}
	if _, err := m.pkgs.db.Exec(`UPDATE plugin_packages SET service_secret=? WHERE id=?`, NewServiceSecret(), id); err != nil {
		return Installed{}, err
	}
	v2Plugins.Delete(id) // the service is being reconfigured: it may be another build
	m.pkgs.invalidate()
	return m.GetPackage(id)
}

// ---------------------------------------------------------------- calls

// serviceClient talks to services on the private network (no SSRF filter on
// purpose: the administrator chose the URL) and never follows redirects.
var serviceClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Transport:     &http.Transport{MaxIdleConnsPerHost: 16, IdleConnTimeout: 90 * time.Second, ResponseHeaderTimeout: 30 * time.Second},
}

// newServiceRequest builds a signed request to the service of k.
func newServiceRequest(k *pkg, method, uri string, body []byte, timeout time.Duration) (*http.Request, func(), error) {
	ctx, cancel := contextWithTimeout(timeout)
	req, err := http.NewRequestWithContext(ctx, method, k.serviceURL+uri, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set(HeaderPlugin, k.mf.ID)
	req.Header.Set("User-Agent", "AnasakiStudio-Service/1.0")
	signServiceRequest(k.secret, req, body)
	return req, cancel, nil
}

// callJSON POSTs v and decodes the JSON answer into out.
func (p *packages) callJSON(k *pkg, uri string, v, out any, timeout time.Duration) error {
	body, _ := json.Marshal(v)
	req, cancel, err := newServiceRequest(k, http.MethodPost, uri, body, timeout)
	if err != nil {
		return err
	}
	defer cancel()
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.do(k, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

// ServiceHealth tests the connection (works before activation too).
func (m *Manager) ServiceHealth(id string) (ServiceHealth, error) {
	k := m.pkgs.get(id)
	if k == nil {
		return ServiceHealth{}, ErrUnknownPackage
	}
	if !k.hasService() {
		return ServiceHealth{}, pkgErr("this plugin has no service")
	}
	if k.serviceURL == "" && !k.isWasm() {
		return ServiceHealth{Error: "ยังไม่ได้ใส่ที่อยู่ของโปรแกรมปลั๊กอิน"}, nil
	}
	start := time.Now()
	req, cancel, err := newServiceRequest(k, http.MethodGet, "/health", nil, 5*time.Second)
	if err != nil {
		return ServiceHealth{Error: err.Error()}, nil
	}
	defer cancel()
	resp, err := m.pkgs.do(k, req)
	h := ServiceHealth{Ms: time.Since(start).Milliseconds()}
	if err != nil {
		h.Error = "เชื่อมต่อไม่ได้: " + err.Error()
		return h, nil
	}
	defer resp.Body.Close()
	h.Status = resp.StatusCode
	var info struct{ Name, Version string }
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&info)
	h.Name, h.Version = info.Name, info.Version
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		h.Error = "โปรแกรมปลั๊กอินปฏิเสธลายเซ็น — รหัสลับ (secret) ไม่ตรงกัน"
	case resp.StatusCode >= 300:
		h.Error = fmt.Sprintf("โปรแกรมปลั๊กอินตอบกลับ HTTP %d", resp.StatusCode)
	default:
		h.OK = true
	}
	return h, nil
}

// ---------------------------------------------------------------- public pages

var hopHeaders = map[string]bool{"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true,
	"Proxy-Authorization": true, "Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
	"Content-Length": true}

// forwarded request headers (public pages); cookies are filtered separately
var publicReqHeaders = []string{"Accept", "Accept-Language", "Content-Type", "User-Agent", "Referer",
	"Origin", "If-None-Match", "If-Modified-Since", "Range", "X-Requested-With"}

// CookiePrefix is the cookie namespace of a service plugin: on public pages
// only cookies whose name starts with it are forwarded to the service, and
// only those may be set by it, so a plugin never sees or overwrites the
// cookies of the CMS or of other plugins.
func CookiePrefix(id string) string { return "pl_" + id + "_" }

// pluginCookies returns the request's Cookie header reduced to the plugin's own cookies.
func pluginCookies(r *http.Request, id string) string {
	pre := CookiePrefix(id)
	var out []string
	for _, c := range r.Cookies() {
		if strings.HasPrefix(c.Name, pre) {
			out = append(out, c.Name+"="+c.Value)
		}
	}
	return strings.Join(out, "; ")
}

// filterSetCookies drops the Set-Cookie headers outside the plugin's namespace.
func filterSetCookies(h http.Header, id string) {
	pre := CookiePrefix(id)
	var keep []string
	for _, v := range h.Values("Set-Cookie") {
		name, _, _ := strings.Cut(strings.TrimSpace(v), "=")
		if strings.HasPrefix(name, pre) {
			keep = append(keep, v)
		}
	}
	h.Del("Set-Cookie")
	for _, v := range keep {
		h.Add("Set-Cookie", v)
	}
}

// sitePathOptions are the options naming a first path segment of the site
// itself, with their defaults.
var sitePathOptions = map[string]string{"admin_path": "admin", "blog_base": "blog", "category_base": "category"}

// siteSegments returns the first path segments the site uses under the
// current options (custom admin path, blog and category bases), cached for
// a few seconds. A service never takes these over, even if the options
// change after it was installed.
func (p *packages) siteSegments() map[string]bool {
	p.segMu.Lock()
	defer p.segMu.Unlock()
	if p.segs != nil && time.Since(p.segsAt) < 5*time.Second {
		return p.segs
	}
	vals := map[string]string{}
	if p.db != nil {
		if rows, err := p.db.Query(`SELECT name, value FROM options WHERE name IN ('admin_path','blog_base','category_base')`); err == nil {
			for rows.Next() {
				var k, v string
				if rows.Scan(&k, &v) == nil {
					vals[k] = v
				}
			}
			rows.Close()
		}
	}
	segs := map[string]bool{}
	for name, def := range sitePathOptions {
		v := strings.ToLower(strings.Trim(vals[name], "/ "))
		if v == "" {
			v = def
		}
		segs[v] = true
	}
	p.segs, p.segsAt = segs, time.Now()
	return segs
}

// publicConflicts returns the public paths of s that the site itself uses.
func (p *packages) publicConflicts(s *ManifestService) []string {
	if s == nil {
		return nil
	}
	segs := p.siteSegments()
	var out []string
	for _, pre := range s.Public {
		first, _, _ := strings.Cut(strings.TrimPrefix(pre, "/"), "/")
		if segs[first] || reservedPublic[first] {
			out = append(out, pre)
		}
	}
	return out
}

// publicService finds the active service that owns path (never one of the
// site's own sections: see siteSegments).
func (p *packages) publicService(path string) *pkg {
	var segs map[string]bool
	for _, k := range p.active() {
		if !k.serviceReady() {
			continue
		}
		for _, pre := range k.mf.Service.Public {
			if path == pre || strings.HasPrefix(path, pre+"/") {
				if segs == nil {
					segs = p.siteSegments()
				}
				first, _, _ := strings.Cut(strings.TrimPrefix(pre, "/"), "/")
				if segs[first] {
					continue
				}
				return k
			}
		}
	}
	return nil
}

// servePublic proxies a public request to the owning service; false when no
// service claims the path.
func (p *packages) servePublic(w http.ResponseWriter, r *http.Request, clientIP string) bool {
	k := p.publicService(r.URL.Path)
	if k == nil {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20+1))
	if err != nil || len(body) > 10<<20 {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return true
	}
	req, cancel, err := newServiceRequest(k, r.Method, "/public"+r.URL.RequestURI(), body, 30*time.Second)
	if err != nil {
		p.serviceDown(w, k, err)
		return true
	}
	defer cancel()
	for _, h := range publicReqHeaders {
		if v := r.Header.Values(h); len(v) > 0 {
			req.Header[h] = v
		}
	}
	if c := pluginCookies(r, k.mf.ID); c != "" {
		req.Header.Set("Cookie", c)
	}
	req.Header.Set("X-Forwarded-For", clientIP)
	host, proto := RequestHost(r), RequestScheme(r)
	req.Header.Set("X-Forwarded-Host", host)
	req.Header.Set("X-Forwarded-Proto", proto)
	req.Header.Set(HeaderSiteURL, proto+"://"+host)
	resp, err := p.do(k, req)
	if err != nil {
		p.serviceDown(w, k, err)
		return true
	}
	defer resp.Body.Close()
	filterSetCookies(resp.Header, k.mf.ID)
	if strings.EqualFold(resp.Header.Get(HeaderLayout), "page") && p.m.host.Pages != nil {
		html, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
		title, _ := url.QueryUnescape(resp.Header.Get(HeaderTitle))
		for _, c := range resp.Header.Values("Set-Cookie") {
			w.Header().Add("Set-Cookie", c)
		}
		p.m.host.Pages.HTML(w, resp.StatusCode, title, string(html))
		return true
	}
	copyResponse(w, resp, nil)
	return true
}

func (p *packages) serviceDown(w http.ResponseWriter, k *pkg, err error) {
	log.Printf("plugin service %s: %v", k.mf.ID, err)
	msg := `<section style="max-width:640px;margin:96px auto;padding:0 24px;text-align:center"><h1 style="font-size:28px;font-weight:800">ขออภัย ส่วนนี้ใช้งานไม่ได้ชั่วคราว</h1><p style="opacity:.7">This section is temporarily unavailable. Please try again later.</p></section>`
	if p.m.host.Pages != nil {
		p.m.host.Pages.HTML(w, http.StatusBadGateway, "Unavailable", msg)
		return
	}
	http.Error(w, "plugin service unavailable", http.StatusBadGateway)
}

// copyResponse writes the service's answer (minus hop-by-hop and protocol
// headers); extra headers are set last.
func copyResponse(w http.ResponseWriter, resp *http.Response, extra http.Header) {
	h := w.Header()
	for k, vs := range resp.Header {
		k = http.CanonicalHeaderKey(k)
		if hopHeaders[k] || strings.HasPrefix(k, "X-Anasaki-") {
			continue
		}
		h[k] = vs
	}
	for k, vs := range extra {
		h[k] = vs
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// ---------------------------------------------------------------- admin screens

// AdminPage returns a service admin page declared by an active plugin.
func (m *Manager) AdminPage(id, page string) (ServiceAdminPage, bool) {
	k := m.pkgs.get(id)
	if k == nil || !k.serviceReady() {
		return ServiceAdminPage{}, false
	}
	for _, a := range k.mf.Service.Admin {
		if a.Path == page {
			return a, true
		}
	}
	return ServiceAdminPage{}, false
}

// ServeAdminUI proxies an admin-screen request (already authenticated by the
// caller) to the service at /admin/<rest>. The answer is sandboxed (opaque
// origin: it can never read the admin's session) and may be fetched
// cross-origin by that sandboxed page (CORS *; the ticket is in the URL).
func (m *Manager) ServeAdminUI(w http.ResponseWriter, r *http.Request, id, rest, base string, user ServiceUser) {
	k := m.pkgs.get(id)
	if k == nil || !k.serviceReady() {
		http.NotFound(w, r)
		return
	}
	cors := http.Header{
		"Access-Control-Allow-Origin":  {"*"},
		"Access-Control-Allow-Methods": {"GET, POST, PUT, PATCH, DELETE, OPTIONS"},
		"Access-Control-Allow-Headers": {"Content-Type, Accept"},
		"Access-Control-Max-Age":       {"600"},
	}
	if r.Method == http.MethodOptions {
		for k, v := range cors {
			w.Header()[k] = v
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 20<<20+1))
	if err != nil || len(body) > 20<<20 {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	uri := "/admin/" + rest
	if r.URL.RawQuery != "" {
		uri += "?" + r.URL.RawQuery
	}
	req, cancel, err := newServiceRequest(k, r.Method, uri, body, 60*time.Second)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer cancel()
	for _, h := range []string{"Accept", "Accept-Language", "Content-Type", "Range"} {
		if v := r.Header.Values(h); len(v) > 0 {
			req.Header[h] = v
		}
	}
	uj, _ := json.Marshal(user)
	req.Header.Set(HeaderUser, base64.StdEncoding.EncodeToString(uj))
	req.Header.Set(HeaderAdminBase, base)
	signServiceRequest(k.secret, req, body) // again: v2 covers the user and the base
	resp, err := m.pkgs.do(k, req)
	if err != nil {
		log.Printf("plugin service %s: admin: %v", id, err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `<p style="font:15px system-ui;padding:32px;color:#b91c1c">โปรแกรมปลั๊กอินไม่ตอบสนอง — ตรวจการเชื่อมต่อที่หน้าตั้งค่าปลั๊กอิน<br>The plugin service is not responding.</p>`)
		return
	}
	defer resp.Body.Close()
	resp.Header.Del("Set-Cookie") // never let a plugin set cookies on the CMS origin
	extra := cors.Clone()
	extra.Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups allow-downloads allow-modals")
	extra.Set("Referrer-Policy", "no-referrer")
	extra.Set("X-Content-Type-Options", "nosniff")
	copyResponse(w, resp, extra)
}

// ---------------------------------------------------------------- blocks, collections, events

type blockCacheEntry struct {
	html, css string
	at        time.Time
}

var (
	blockCacheMu sync.Mutex
	blockCache   = map[string]blockCacheEntry{}
)

// renderServiceBlock asks the service for a block (cached for 10 s per
// identical input; failures render nothing).
func (p *packages) renderServiceBlock(k *pkg, b ManifestBlock, c design.BlockCall) (string, string) {
	if !k.serviceReady() {
		return "", ""
	}
	in := map[string]any{"values": Sanitize(b.Props, c.Values), "site": c.Site, "post": c.Post, "item": c.Item,
		"settings": k.settings}
	key, _ := json.Marshal([]any{k.mf.ID, k.version, b.Type, in})
	blockCacheMu.Lock()
	if e, ok := blockCache[string(key)]; ok && time.Since(e.at) < 10*time.Second {
		blockCacheMu.Unlock()
		return e.html, e.css
	}
	blockCacheMu.Unlock()
	var out struct{ HTML, CSS string }
	if err := p.callJSON(k, "/blocks/"+url.PathEscape(b.Type), in, &out, 3*time.Second); err != nil {
		log.Printf("plugin service %s: block %s: %v", k.mf.ID, b.Type, err)
		return "", ""
	}
	css := ""
	if out.CSS != "" {
		css = ScopeCSS(out.CSS, "."+BlockClass(k.mf.ID, b.Type))
	}
	blockCacheMu.Lock()
	if len(blockCache) > 500 {
		blockCache = map[string]blockCacheEntry{}
	}
	blockCache[string(key)] = blockCacheEntry{out.HTML, css, time.Now()}
	blockCacheMu.Unlock()
	return out.HTML, css
}

// serviceCollection returns items from the service owning source.
func (p *packages) serviceCollection(source string, query map[string]any) ([]design.PostCtx, bool) {
	for _, k := range p.active() {
		if !k.serviceReady() {
			continue
		}
		for _, c := range k.mf.Service.Collections {
			if c.Source != source {
				continue
			}
			var out struct {
				Items []map[string]any `json:"items"`
			}
			if err := p.callJSON(k, "/collections/"+url.PathEscape(source), map[string]any{"query": query}, &out, 3*time.Second); err != nil {
				log.Printf("plugin service %s: collection %s: %v", k.mf.ID, source, err)
				return nil, true
			}
			items := make([]design.PostCtx, 0, len(out.Items))
			for _, it := range out.Items {
				ctx := design.PostCtx{}
				for key, v := range it {
					switch t := v.(type) {
					case string:
						ctx[key] = t
					case float64:
						ctx[key] = strconv.FormatFloat(t, 'f', -1, 64)
					case bool:
						ctx[key] = strconv.FormatBool(t)
					}
				}
				items = append(items, ctx)
			}
			return items, true
		}
	}
	return nil, false
}

// serviceCollections lists the collection sources of active services.
func (p *packages) serviceCollections() []Collection {
	var out []Collection
	for _, k := range p.active() {
		if k.serviceReady() {
			for _, c := range k.mf.Service.Collections {
				out = append(out, Collection{Source: c.Source, Label: c.Label})
			}
		}
	}
	return out
}

// serviceEvents POSTs an event to every active service subscribed to it.
func (p *packages) serviceEvents(event string, payload map[string]any) {
	for _, k := range p.active() {
		if !k.serviceReady() {
			continue
		}
		for _, e := range k.mf.Service.Events {
			if e != event && e != "*" {
				continue
			}
			body := map[string]any{"event": event, "data": payload, "sentAt": time.Now().UTC().Format(time.RFC3339)}
			go func(k *pkg) {
				if err := p.callJSON(k, "/events", body, nil, 10*time.Second); err != nil {
					log.Printf("plugin service %s: event %s: %v", k.mf.ID, event, err)
				}
			}(k)
			break
		}
	}
}

// ServiceStatuses describes active, connected services for the admin menu
// and the designer (sidebar entries and collection sources).
func (m *Manager) ServiceStatuses() []Status {
	var out []Status
	for _, k := range m.pkgs.active() {
		if !k.serviceReady() {
			continue
		}
		s := Status{Info: Info{ID: k.mf.ID, Name: k.mf.Name, Description: k.mf.Description, Version: k.version,
			Author: k.mf.Author, Icon: k.mf.Icon}, Active: true, Kind: "service", Menu: []MenuItem{}, Collections: []Collection{}}
		for _, a := range k.mf.Service.Admin {
			cp := a.Cap
			if cp == "" {
				cp = "manage_options"
			}
			icon := a.Icon
			if icon == "" {
				icon = k.mf.Icon
			}
			s.Menu = append(s.Menu, MenuItem{Path: "/x/" + k.mf.ID + "/" + a.Path, Label: a.Label, LabelEn: a.LabelEn, Icon: icon, Cap: cp})
		}
		for _, c := range k.mf.Service.Collections {
			s.Collections = append(s.Collections, Collection{Source: c.Source, Label: c.Label})
		}
		out = append(out, s)
	}
	return out
}

// VerifyServiceRequest authenticates a call from a service to the CMS
// (/api/service/...): it returns the plugin id and its API scopes.
func (m *Manager) VerifyServiceRequest(r *http.Request, body []byte) (string, map[string]bool, error) {
	id := r.Header.Get(HeaderPlugin)
	k := m.pkgs.get(id)
	if k == nil || !k.serviceReady() {
		return "", nil, ErrBadSignature
	}
	v2 := r.Header.Get(HeaderSigV2) != ""
	if _, upgraded := v2Plugins.Load(id); upgraded && !v2 {
		return "", nil, ErrBadSignature
	}
	if err := VerifyServiceSignature(k.secret, r, body); err != nil {
		return "", nil, err
	}
	if v2 {
		v2Plugins.Store(id, true)
	}
	scopes := map[string]bool{}
	for _, s := range k.mf.Service.API {
		scopes[s] = true
	}
	return id, scopes, nil
}

// DeclaredPaths returns the first path segments the installed packages
// (active or not) declare as their public URLs — service "public" prefixes
// and JS "code.routes" — mapped to the package id. The site's admin path and
// URL bases may not use them.
func (m *Manager) DeclaredPaths() map[string]string {
	out := map[string]string{}
	add := func(id string, prefixes []string) {
		for _, p := range prefixes {
			first, _, _ := strings.Cut(strings.TrimPrefix(p, "/"), "/")
			if first = strings.ToLower(first); first != "" && out[first] == "" {
				out[first] = id
			}
		}
	}
	for _, k := range m.pkgs.all() {
		if k.mf.Service != nil {
			add(k.mf.ID, k.mf.Service.Public)
		}
		if k.mf.Code != nil {
			add(k.mf.ID, k.mf.Code.Routes)
		}
	}
	return out
}

// ServicePublicPaths returns the public path prefixes of the active service
// plugins (Site Code refuses routes under them).
func (m *Manager) ServicePublicPaths() []string {
	var out []string
	for _, k := range m.pkgs.active() {
		if k.hasService() && k.mf.Service != nil {
			out = append(out, k.mf.Service.Public...)
		}
	}
	return out
}
