package plugin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
)

// ErrUnknownPackage is returned for a package id that is not installed.
var ErrUnknownPackage = errors.New("unknown package plugin")

const packagesSchema = `CREATE TABLE IF NOT EXISTS plugin_packages (
  id           VARCHAR(64)  NOT NULL PRIMARY KEY,
  version      VARCHAR(64)  NOT NULL,
  manifest     JSON         NOT NULL,
  active       BOOLEAN      NOT NULL DEFAULT FALSE,
  source       VARCHAR(10)  NOT NULL DEFAULT 'upload',
  settings     JSON         NULL,
  installed_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// Installed is the API representation of an installed package plugin.
type Installed struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Icon        string   `json:"icon"`
	IconURL     string   `json:"iconUrl"`
	Category    string   `json:"category"`
	Permissions []string `json:"permissions"`
	Active      bool     `json:"active"`
	Source      string   `json:"source"` // upload | store
	// Publisher is the Store publisher id the package is bound to (set by
	// the first install from the Store; "" = unknown). Updates from another
	// publisher are refused (audit H-2).
	Publisher      string           `json:"publisher,omitempty"`
	InstalledAt    time.Time        `json:"installedAt"`
	SettingsSchema []Field          `json:"settingsSchema"`
	Settings       Values           `json:"settings"`
	Blocks         []InstalledBlock `json:"blocks"`
	// Service is set when the plugin ships a server program (service plugin).
	Service *ServiceView `json:"service,omitempty"`
	// Reconsent is set when an upgrade asked for permissions, service API
	// scopes or hosts the administrator hasn't approved: the package was
	// switched off and activating it again approves them.
	Reconsent *Reconsent `json:"reconsent,omitempty"`

	// JavaScript plugins (runtime "js") and dependencies (phase 4b).
	// Runtime: "js", "wasm" (WASM service), "service" (separate program) or
	// "" (a plain package).
	Runtime        string            `json:"runtime"`
	Requires       map[string]string `json:"requires"`
	RequiresStatus []RequireStatus   `json:"requiresStatus"`
	// OptionalRequires: plugins it works with when installed ("works with",
	// phase 6); the status has the same shape as RequiresStatus (OK false with
	// an installed version = incompatible: a warning, never a failure).
	OptionalRequires       map[string]string `json:"optionalRequires"`
	OptionalRequiresStatus []RequireStatus   `json:"optionalRequiresStatus"`
	Dependents             []string          `json:"dependents"` // installed plugins that require this one
	Code                   *CodeView         `json:"code,omitempty"`
	// ActivationError: the code did not build / migrate / register at
	// activation (the plugin stays off), or failed to load since.
	ActivationError string `json:"activationError,omitempty"`
	// Functions: service.functions callable with plugins.call.
	Functions []string `json:"functions,omitempty"`
	// Legacy: the former built-in plugin this official package replaces
	// (manifest "legacy", phase 7).
	Legacy string `json:"legacy,omitempty"`
	// Capabilities the plugin declares (since 2.0.6): "<id>.<capability>",
	// ticked per role in Users → Roles.
	Capabilities []CapabilityView `json:"capabilities"`
}

// Reconsent lists what upgrades added that still awaits approval.
type Reconsent struct {
	FromVersion string   `json:"fromVersion"` // the version that was approved
	Permissions []string `json:"permissions"`
	API         []string `json:"api"`    // service.api scopes
	Hosts       []string `json:"hosts"`  // service.hosts and code.hosts
	Scopes      []string `json:"scopes"` // code.scopes (JS plugins)
}

func (rc *Reconsent) empty() bool {
	return rc == nil || len(rc.Permissions)+len(rc.API)+len(rc.Hosts)+len(rc.Scopes) == 0
}

// manifestGrants are what a manifest asks the administrator to allow.
func manifestGrants(mf *Manifest) (perms, api, hosts, scopes []string) {
	perms = mf.Permissions
	if mf.Service != nil {
		api, hosts = mf.Service.API, mf.Service.Hosts
	}
	if mf.Code != nil {
		scopes = mf.Code.Scopes
		hosts = append(append([]string{}, hosts...), mf.Code.Hosts...)
	}
	return
}

// reconsentFor returns what the upgrade from old (with its own still pending
// grants) to mf asks for beyond what the administrator approved; nil when
// nothing.
func reconsentFor(old *pkg, mf *Manifest) *Reconsent {
	op, oa, oh, oc := manifestGrants(old.mf)
	pending := old.reconsent
	if pending == nil {
		pending = &Reconsent{}
	}
	approved := func(list, notYet []string) map[string]bool {
		out := map[string]bool{}
		for _, v := range list {
			out[v] = true
		}
		for _, v := range notYet {
			delete(out, v)
		}
		return out
	}
	ap, aa, ah := approved(op, pending.Permissions), approved(oa, pending.API), approved(oh, pending.Hosts)
	as := approved(oc, pending.Scopes)
	np, na, nh, ns := manifestGrants(mf)
	rc := &Reconsent{FromVersion: old.version, Permissions: []string{}, API: []string{}, Hosts: []string{}, Scopes: []string{}}
	if old.reconsent != nil && old.reconsent.FromVersion != "" {
		rc.FromVersion = old.reconsent.FromVersion
	}
	for _, v := range np {
		if !ap[v] {
			rc.Permissions = append(rc.Permissions, v)
		}
	}
	for _, v := range na {
		if !aa[v] {
			rc.API = append(rc.API, v)
		}
	}
	for _, v := range nh {
		if !hostCovered(v, ah) {
			rc.Hosts = append(rc.Hosts, v)
		}
	}
	for _, v := range ns {
		if !as[v] {
			rc.Scopes = append(rc.Scopes, v)
		}
	}
	if rc.empty() {
		return nil
	}
	return rc
}

// hostCovered reports whether host (maybe "*.x.com") is within the approved hosts.
func hostCovered(host string, approved map[string]bool) bool {
	if approved[host] {
		return true
	}
	for a := range approved {
		if suf, ok := strings.CutPrefix(a, "*"); ok && strings.HasSuffix(host, suf) && !strings.HasPrefix(host, "*.") {
			return true
		}
	}
	return false
}

// InstalledBlock is a block of an installed package.
type InstalledBlock struct {
	Type  string  `json:"type"`
	Name  I18n    `json:"name"`
	Icon  string  `json:"icon"`
	Props []Field `json:"props"`
}

type pkg struct {
	mf          *Manifest
	version     string
	active      bool
	source      string
	settings    Values
	installedAt time.Time
	dir         string
	serviceURL  string // where the plugin's service listens ("" = not connected)
	secret      string // shared HMAC secret with the service
	reconsent   *Reconsent
	codeError   string // activation error of a JS plugin
	publisher   string // Store publisher id the package is bound to ("" = unknown)
}

type packages struct {
	m    *Manager
	db   *sql.DB
	root string // PLUGINS_DIR

	mu       sync.Mutex
	list     []*pkg
	loadedAt time.Time
	files    map[string]string // dir|path → file contents (templates, css)

	client *http.Client // webhooks

	installMu sync.Mutex // one install / uninstall at a time

	segMu  sync.Mutex
	segs   map[string]bool // first path segments of the site (siteSegments)
	segsAt time.Time

	// capCatalog cache (guarded by mu): valid for the list loaded at capsAt
	caps   []auth.CapInfo
	capsAt time.Time
	capsOK bool
}

func newPackages(m *Manager) (*packages, error) {
	db := m.host.Store.DB
	if _, err := db.Exec(packagesSchema); err != nil {
		return nil, fmt.Errorf("plugin_packages: %w", err)
	}
	if err := migrateService(db); err != nil {
		return nil, err
	}
	if _, err := db.Exec(kvSchema); err != nil {
		return nil, fmt.Errorf("plugin_kv: %w", err)
	}
	root := os.Getenv("PLUGINS_DIR")
	if root == "" {
		root = "./plugins-data"
	}
	return &packages{m: m, db: db, root: root, files: map[string]string{},
		client: NewWebhookClient(5 * time.Second)}, nil
}

func (p *packages) invalidate() {
	p.mu.Lock()
	p.loadedAt = time.Time{}
	p.mu.Unlock()
}

// all returns the installed packages (sorted by name), reloaded at most
// every two seconds.
func (p *packages) all() []*pkg {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db == nil || (!p.loadedAt.IsZero() && time.Since(p.loadedAt) < 2*time.Second) {
		return p.list
	}
	rows, err := p.db.Query(`SELECT id, version, manifest, active, source, COALESCE(settings, '{}'), installed_at,
		service_url, service_secret, COALESCE(consent_pending, ''), COALESCE(code_error, ''), publisher FROM plugin_packages`)
	if err != nil {
		log.Printf("plugin packages: %v", err)
		return p.list
	}
	defer rows.Close()
	var list []*pkg
	for rows.Next() {
		var id, ver, src, svcURL, secret, consent, codeErr, publisher string
		var mfRaw, setRaw []byte
		var active bool
		var at time.Time
		if err := rows.Scan(&id, &ver, &mfRaw, &active, &src, &setRaw, &at, &svcURL, &secret, &consent, &codeErr, &publisher); err != nil {
			log.Printf("plugin packages: %v", err)
			continue
		}
		var mf Manifest
		if err := json.Unmarshal(mfRaw, &mf); err != nil {
			log.Printf("plugin package %s: bad manifest: %v", id, err)
			continue
		}
		stored := map[string]any{}
		_ = json.Unmarshal(setRaw, &stored)
		var rc *Reconsent
		if consent != "" {
			rc = &Reconsent{}
			if json.Unmarshal([]byte(consent), rc) != nil || rc.empty() {
				rc = nil
			}
		}
		list = append(list, &pkg{mf: &mf, version: ver, active: active, source: src, installedAt: at,
			settings: Sanitize(mf.Settings, stored), dir: filepath.Join(p.root, id, ver), serviceURL: svcURL, secret: secret,
			reconsent: rc, codeError: codeErr, publisher: publisher})
	}
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].mf.Name) < strings.ToLower(list[j].mf.Name) })
	p.list, p.loadedAt = list, time.Now()
	return list
}

func (p *packages) get(id string) *pkg {
	for _, k := range p.all() {
		if k.mf.ID == id {
			return k
		}
	}
	return nil
}

func (p *packages) active() []*pkg {
	var out []*pkg
	for _, k := range p.all() {
		if k.active {
			out = append(out, k)
		}
	}
	return out
}

// file returns a package file's contents (cached; packages are immutable
// per version directory).
func (p *packages) file(k *pkg, rel string) string {
	key := k.dir + "|" + rel
	p.mu.Lock()
	s, ok := p.files[key]
	p.mu.Unlock()
	if ok {
		return s
	}
	if checkZipPath(rel) != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(k.dir, filepath.FromSlash(rel)))
	if err != nil {
		log.Printf("plugin package %s: %v", k.mf.ID, err)
		return ""
	}
	p.mu.Lock()
	if len(p.files) > 2000 {
		p.files = map[string]string{}
	}
	p.files[key] = string(b)
	p.mu.Unlock()
	return string(b)
}

func (k *pkg) view() Installed {
	mf := k.mf
	in := Installed{ID: mf.ID, Name: mf.Name, Version: k.version, Description: mf.Description, Author: mf.Author,
		Icon: mf.Icon, Category: mf.Category, Permissions: mf.Permissions, Active: k.active, Source: k.source, Publisher: k.publisher,
		InstalledAt: k.installedAt, SettingsSchema: mf.Settings, Settings: k.settings, Blocks: []InstalledBlock{}}
	if ip := mf.IconPath(); ip != "" {
		in.Icon = ""
		in.IconURL = "/plugin-assets/" + mf.ID + "/" + ip + "?v=" + k.version
	}
	if in.Permissions == nil {
		in.Permissions = []string{}
	}
	if in.SettingsSchema == nil {
		in.SettingsSchema = []Field{}
	}
	if in.Settings == nil {
		in.Settings = Values{}
	}
	for _, b := range mf.Blocks {
		props := b.Props
		if props == nil {
			props = []Field{}
		}
		in.Blocks = append(in.Blocks, InstalledBlock{Type: b.Type, Name: b.Name, Icon: b.Icon, Props: props})
	}
	in.Service = k.serviceView()
	in.Reconsent = k.reconsent
	in.Requires = mf.Requires
	if in.Requires == nil {
		in.Requires = map[string]string{}
	}
	in.OptionalRequires = mf.OptionalRequires
	if in.OptionalRequires == nil {
		in.OptionalRequires = map[string]string{}
	}
	in.Dependents, in.RequiresStatus, in.OptionalRequiresStatus = []string{}, []RequireStatus{}, []RequireStatus{}
	switch {
	case k.isJS():
		in.Runtime = RuntimeJS
		in.Code = mf.Code.view()
		in.ActivationError = k.codeError
	case k.isWasm():
		in.Runtime = "wasm"
	case k.hasService():
		in.Runtime = "service"
	}
	if k.hasService() {
		in.Functions = mf.Service.Functions
	}
	in.Legacy = mf.Legacy
	in.Capabilities = mf.capabilityViews()
	return in
}

// decorate adds what needs the other packages (dependencies, dependents)
// and the engine (load errors).
func (m *Manager) decorate(in *Installed, k *pkg) {
	in.RequiresStatus = m.requireStatus(k.mf)
	in.OptionalRequiresStatus = m.depStatus(k.mf.OptionalRequires)
	in.Dependents = nonNil(m.dependents(k.mf.ID, false))
	if k.isJS() && in.ActivationError == "" && k.active && m.code != nil {
		in.ActivationError = m.code.UnitError(k.mf.ID)
	}
}

// forgetFiles drops the cached files of every version of package id (a
// reinstall of the same version must not serve the old contents).
func (p *packages) forgetFiles(id string) {
	pre := filepath.Join(p.root, id) + string(filepath.Separator)
	p.mu.Lock()
	for key := range p.files {
		if strings.HasPrefix(key, pre) {
			delete(p.files, key)
		}
	}
	p.mu.Unlock()
}

// ---------------------------------------------------------------- manager API

// ListPackages returns every installed package plugin.
func (m *Manager) ListPackages() []Installed {
	out := []Installed{}
	for _, k := range m.pkgs.all() {
		in := k.view()
		m.decorate(&in, k)
		out = append(out, in)
	}
	return out
}

// GetPackage returns one installed package.
func (m *Manager) GetPackage(id string) (Installed, error) {
	k := m.pkgs.get(id)
	if k == nil {
		return Installed{}, ErrUnknownPackage
	}
	in := k.view()
	m.decorate(&in, k)
	return in, nil
}

// InstalledVersion returns the installed version of a package ("" if none).
func (m *Manager) InstalledVersion(id string) string {
	if k := m.pkgs.get(id); k != nil {
		return k.version
	}
	return ""
}

// InstalledPublisher returns the Store publisher id installed package id is
// bound to ("" when it is not installed or the publisher is unknown).
func (m *Manager) InstalledPublisher(id string) string {
	if k := m.pkgs.get(id); k != nil {
		return k.publisher
	}
	return ""
}

// InstallOptions are the administrator's answers for an install / upgrade.
type InstallOptions struct {
	// ConfirmDestructive allows migrations of a JS plugin upgrade that drop
	// or rename data (backups are made first).
	ConfirmDestructive bool
	// Publisher is the Store publisher id of the downloaded package (source
	// "store"; X-Anasaki-Publisher). The first install binds the package to
	// it; an update from another publisher is a *PublisherError unless
	// AcceptPublisher (the administrator trusts the new publisher).
	Publisher       string
	AcceptPublisher bool
}

// PublisherError: an update from the Store comes from another publisher than
// the one the installed plugin is bound to — the plugin id may have changed
// hands (audit H-2). Nothing is installed (HTTP 409).
type PublisherError struct {
	ID, Installed, Offered string
}

func (e *PublisherError) Error() string {
	offered := e.Offered
	if offered == "" {
		offered = "unknown"
	}
	return fmt.Sprintf("อัปเดตนี้มาจากผู้เผยแพร่คนละรายกับปลั๊กอิน %s ที่ติดตั้งไว้ (เดิม %s, ใหม่ %s) — ปลั๊กอินอาจเปลี่ยนมือ จึงไม่ได้ติดตั้งให้ ตรวจสอบกับร้านก่อน / "+
		"This update of %s comes from a different publisher (installed: %s, offered: %s) — the plugin may have changed hands; it was not installed",
		e.ID, e.Installed, offered, e.ID, e.Installed, offered)
}

// InstallPackage validates a zip and installs (or upgrades) it. source is
// "upload" or "store"; wantID, when set, must match the manifest id.
// Validation failures are *PackageError.
func (m *Manager) InstallPackage(data []byte, source, wantID string) (Installed, error) {
	return m.InstallPackageWith(data, source, wantID, InstallOptions{})
}

// InstallPackageWith is InstallPackage with options. Missing dependencies
// are *RequiresError; an active JS plugin's upgrade whose code does not build
// is *ActivationError and one with destructive migrations needs
// ConfirmDestructive (*DestructiveError) - in both cases nothing changes.
func (m *Manager) InstallPackageWith(data []byte, source, wantID string, opts InstallOptions) (Installed, error) {
	mf, files, err := ParsePackage(data)
	if err != nil {
		return Installed{}, err
	}
	if wantID != "" && mf.ID != wantID {
		return Installed{}, pkgErr("the downloaded package is %q, expected %q", mf.ID, wantID)
	}
	if _, builtin := m.byID[mf.ID]; builtin || builtinIDs[mf.ID] {
		return Installed{}, pkgErr("id %q is reserved by a built-in plugin", mf.ID)
	}
	p := m.pkgs
	if c := p.publicConflicts(mf.Service); len(c) > 0 {
		return Installed{}, pkgErr("manifest: service: public path %q is used by the site (admin path, blog or category base)", c[0])
	}
	if mf.Code != nil {
		if c := p.publicConflicts(&ManifestService{Public: mf.Code.Routes}); len(c) > 0 {
			return Installed{}, pkgErr("manifest: code: route prefix %q is used by the site (admin path, blog or category base)", c[0])
		}
	}
	if missing := m.checkRequires(mf, false); len(missing) > 0 {
		return Installed{}, &RequiresError{Missing: missing}
	}
	for _, d := range m.dependents(mf.ID, false) {
		if dk := p.get(d); dk != nil && !SatisfiesRange(mf.Version, dk.mf.Requires[mf.ID]) {
			return Installed{}, pkgErr("version %s does not satisfy plugin %s, which requires %s %s", mf.Version, d, mf.ID, dk.mf.Requires[mf.ID])
		}
	}
	// one install at a time: the directory swap and the cleanup below must
	// not interleave with another install of the same plugin
	p.installMu.Lock()
	defer p.installMu.Unlock()
	old := p.get(mf.ID)
	if source == "store" && old != nil && old.publisher != "" && opts.Publisher != old.publisher && !opts.AcceptPublisher {
		return Installed{}, &PublisherError{ID: mf.ID, Installed: old.publisher, Offered: opts.Publisher}
	}
	reactivate := old != nil && old.active && mf.Runtime == RuntimeJS && mf.Code != nil && reconsentFor(old, mf) == nil
	if reactivate {
		if m.code == nil {
			return Installed{}, &ActivationError{Msg: "ระบบ Site Code ไม่พร้อมใช้งาน — อัปเกรดปลั๊กอิน JavaScript ไม่ได้"}
		}
		ref := CodeUnitRef{ID: mf.ID, Name: mf.Name, Version: mf.Version, InstalledAt: old.installedAt, Active: true,
			Code: *mf.Code, Requires: mf.Requires, OptionalRequires: mf.OptionalRequires, Source: source, Capabilities: mf.CapKeys()}
		plan, err := m.code.PlanUnit(ref, codeFilesOf(files))
		if err != nil {
			return Installed{}, err
		}
		if len(plan.Errors) > 0 {
			return Installed{}, &ActivationError{Msg: "โค้ดของเวอร์ชันใหม่ build ไม่ผ่าน — ยังใช้เวอร์ชันเดิมอยู่", Problems: plan.Errors}
		}
		if len(plan.Destructive) > 0 && !opts.ConfirmDestructive {
			return Installed{}, &DestructiveError{List: plan.Destructive}
		}
	}
	idDir := filepath.Join(p.root, mf.ID)
	dir := filepath.Join(idDir, mf.Version)
	if err := os.MkdirAll(idDir, 0o755); err != nil {
		return Installed{}, err
	}
	tmp, err := os.MkdirTemp(idDir, ".install-")
	if err != nil {
		return Installed{}, err
	}
	defer os.RemoveAll(tmp)
	for name, b := range files {
		dst := filepath.Join(tmp, filepath.FromSlash(name))
		if !strings.HasPrefix(dst, tmp+string(filepath.Separator)) { // belt and braces against zip-slip
			return Installed{}, pkgErr("unsafe file path %q", name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return Installed{}, err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return Installed{}, err
		}
	}
	// reinstall of the same version: move the old copy aside first (a
	// directory can't be renamed over a non-empty one) and put it back if
	// the swap fails
	var aside string
	if _, err := os.Stat(dir); err == nil {
		aside = filepath.Join(idDir, ".old-"+filepath.Base(tmp))
		if err := os.Rename(dir, aside); err != nil {
			return Installed{}, err
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		if aside != "" {
			_ = os.Rename(aside, dir)
		}
		return Installed{}, err
	}
	if aside != "" {
		_ = os.RemoveAll(aside)
	}
	p.forgetFiles(mf.ID)
	wasm.drop(mf.ID)

	// keep settings across upgrades (re-validated), and activation unless the
	// upgrade asks for more than the administrator approved
	settings := Values{}
	var rc *Reconsent
	if old := p.get(mf.ID); old != nil {
		settings = Sanitize(mf.Settings, old.settings)
		rc = reconsentFor(old, mf)
	} else {
		settings = Defaults(mf.Settings)
	}
	mfJSON, _ := json.Marshal(mf)
	setJSON, _ := json.Marshal(settings)
	consent := ""
	if rc != nil {
		b, _ := json.Marshal(rc)
		consent = string(b)
	}
	now := time.Now().UTC()
	secret := ""
	if mf.Service != nil {
		secret = NewServiceSecret() // kept on upgrade when one exists
	}
	publisher := "" // only the Store vouches for a publisher; an upload keeps the binding
	if source == "store" {
		publisher = opts.Publisher
	}
	if _, err := p.db.Exec(`INSERT INTO plugin_packages (id, version, manifest, active, source, settings, installed_at, updated_at, service_secret, consent_pending, code_error, publisher)
		VALUES (?,?,?,FALSE,?,?,?,?,?,'','',?)
		ON DUPLICATE KEY UPDATE version=VALUES(version), manifest=VALUES(manifest), source=VALUES(source),
		settings=VALUES(settings), updated_at=VALUES(updated_at),
		service_secret=IF(service_secret='', VALUES(service_secret), service_secret),
		publisher=IF(VALUES(publisher)<>'', VALUES(publisher), publisher),
		active=IF(?<>'', FALSE, active), consent_pending=?, code_error=''`,
		mf.ID, mf.Version, mfJSON, source, setJSON, now, now, secret, publisher, consent, consent); err != nil {
		return Installed{}, err
	}
	p.invalidate()
	// drop other versions' files, and install leftovers older than an hour
	// (a crashed install); never another install's temp dir in progress
	if ents, err := os.ReadDir(idDir); err == nil {
		for _, e := range ents {
			name := e.Name()
			if name == mf.Version {
				continue
			}
			if strings.HasPrefix(name, ".install-") || strings.HasPrefix(name, ".old-") {
				if fi, err := e.Info(); err != nil || time.Since(fi.ModTime()) < time.Hour {
					continue
				}
			}
			_ = os.RemoveAll(filepath.Join(idDir, name))
		}
	}
	p.invalidate()
	// capabilities before the code runs: its access lists may name them
	if err := m.syncCaps(mf); err != nil {
		log.Printf("%v", err)
	}
	if mf.Runtime == RuntimeJS && m.code != nil {
		if reactivate {
			if err := m.activateCode(mf.ID, opts.ConfirmDestructive); err != nil {
				return Installed{}, err
			}
		}
		m.code.UnitsChanged()
	}
	return m.GetPackage(mf.ID)
}

// ActiveOptions are the administrator's answers when switching a package.
type ActiveOptions struct {
	ConfirmDestructive bool // activation: allow destructive migrations
	Force              bool // deactivation: also switch off the plugins that require it
}

// SetPackageActive switches a package on or off. Switching it on approves
// what an upgrade added (Installed.Reconsent).
func (m *Manager) SetPackageActive(id string, on bool) (Installed, error) {
	return m.SetPackageActiveWith(id, on, ActiveOptions{})
}

// SetPackageActiveWith is SetPackageActive with options. Activating needs
// the dependencies active (*RequiresError) and, for a JS plugin, code that
// builds and migrates (*ActivationError / *DestructiveError). Deactivating a
// plugin others require is *DependentsError unless Force.
func (m *Manager) SetPackageActiveWith(id string, on bool, opts ActiveOptions) (Installed, error) {
	k := m.pkgs.get(id)
	if k == nil {
		return Installed{}, ErrUnknownPackage
	}
	jsChanged := k.isJS()
	if on {
		if missing := m.checkRequires(k.mf, true); len(missing) > 0 {
			return Installed{}, &RequiresError{Missing: missing}
		}
		if k.isJS() {
			if err := m.activateCode(id, opts.ConfirmDestructive); err != nil {
				return Installed{}, err
			}
		}
	} else if deps := m.dependents(id, true); len(deps) > 0 {
		if !opts.Force {
			return Installed{}, &DependentsError{ID: id, Dependents: deps}
		}
		jsChanged = m.switchOffDependents(id) || jsChanged
	}
	if _, err := m.pkgs.db.Exec(`UPDATE plugin_packages SET active=?, consent_pending=IF(?, '', consent_pending), code_error=IF(?, '', code_error) WHERE id=?`,
		on, on, on, id); err != nil {
		return Installed{}, err
	}
	m.pkgs.invalidate()
	if jsChanged && m.code != nil {
		m.code.UnitsChanged()
	}
	if k := m.pkgs.get(id); on && k != nil && k.isWasm() {
		go func() { // compile now so the first visitor doesn't wait
			if _, err := wasm.module(m.pkgs, k); err != nil {
				log.Printf("plugin %s: %v", id, err)
			}
		}()
	}
	return m.GetPackage(id)
}

// switchOffDependents deactivates every active package that requires id,
// directly or through another one; it reports whether a JS plugin changed.
func (m *Manager) switchOffDependents(id string) bool {
	js := false
	seen := map[string]bool{id: true}
	queue := []string{id}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range m.dependents(cur, true) {
			if seen[d] {
				continue
			}
			seen[d] = true
			queue = append(queue, d)
			if _, err := m.pkgs.db.Exec(`UPDATE plugin_packages SET active=FALSE WHERE id=?`, d); err != nil {
				log.Printf("plugin %s: deactivate dependent: %v", d, err)
			}
			if dk := m.pkgs.get(d); dk != nil && dk.isJS() {
				js = true
			}
		}
		m.pkgs.invalidate()
	}
	return js
}

// DeleteOptions are the administrator's answers when uninstalling.
type DeleteOptions struct {
	Force      bool // uninstall although other plugins require it (they are switched off)
	DeleteData bool // JS plugin: back up and drop its tables, settings, jobs (else kept)
}

// DeletePackage uninstalls a package: its row, stored data (plugin_kv),
// files, cached files and compiled WASM module.
func (m *Manager) DeletePackage(id string) error { return m.DeletePackageWith(id, DeleteOptions{}) }

// DeletePackageWith is DeletePackage with options (*DependentsError when
// other installed plugins require it, unless Force).
func (m *Manager) DeletePackageWith(id string, opts DeleteOptions) error {
	p := m.pkgs
	k := p.get(id)
	if k == nil {
		return ErrUnknownPackage
	}
	if deps := m.dependents(id, false); len(deps) > 0 {
		if !opts.Force {
			return &DependentsError{ID: id, Dependents: deps}
		}
		m.switchOffDependents(id)
	}
	if opts.DeleteData && k.isJS() && m.code != nil {
		if err := m.code.DropUnitData(id); err != nil {
			return err
		}
	}
	p.installMu.Lock()
	defer p.installMu.Unlock()
	if p.get(id) == nil {
		return ErrUnknownPackage
	}
	if _, err := p.db.Exec(`DELETE FROM plugin_packages WHERE id=?`, id); err != nil {
		return err
	}
	if _, err := p.db.Exec(`DELETE FROM plugin_kv WHERE plugin_id=?`, id); err != nil {
		log.Printf("plugin package %s: kv cleanup: %v", id, err)
	}
	p.invalidate()
	if err := m.dropCaps(id); err != nil {
		log.Printf("plugin package %s: capabilities cleanup: %v", id, err)
	}
	if pkgIDRe.MatchString(id) {
		_ = os.RemoveAll(filepath.Join(p.root, id))
	}
	p.forgetFiles(id)
	wasm.drop(id)
	v2Plugins.Delete(id)
	if k.isJS() && m.code != nil {
		m.code.UnitsChanged()
	}
	return nil
}

// ImportPackageSettings merges values taken over from elsewhere (a former
// built-in plugin's settings, phase 7) into package id's settings: keys of the
// manifest schema only, invalid values fall back to the current ones.
func (m *Manager) ImportPackageSettings(id string, in map[string]any) (Installed, error) {
	k := m.pkgs.get(id)
	if k == nil {
		return Installed{}, ErrUnknownPackage
	}
	merged := map[string]any{}
	for key, v := range k.settings {
		merged[key] = v
	}
	cur := Sanitize(k.mf.Settings, merged)
	for _, f := range k.mf.Settings {
		raw, ok := in[f.Key]
		if !ok {
			continue
		}
		if v, err := Validate([]Field{f}, map[string]any{f.Key: raw}); err == nil {
			cur[f.Key] = v[f.Key]
		}
	}
	b, _ := json.Marshal(cur)
	if _, err := m.pkgs.db.Exec(`UPDATE plugin_packages SET settings=? WHERE id=?`, b, id); err != nil {
		return Installed{}, err
	}
	m.pkgs.invalidate()
	return m.GetPackage(id)
}

// IsBuiltin reports whether id is a plugin compiled into this build.
func (m *Manager) IsBuiltin(id string) bool { _, ok := m.byID[id]; return ok }

// SetPackageSettings validates values against the manifest schema and saves
// them (ValidationError on bad input).
func (m *Manager) SetPackageSettings(id string, in map[string]any) (Installed, error) {
	k := m.pkgs.get(id)
	if k == nil {
		return Installed{}, ErrUnknownPackage
	}
	v, err := Validate(k.mf.Settings, in)
	if err != nil {
		return Installed{}, err
	}
	b, _ := json.Marshal(v)
	if _, err := m.pkgs.db.Exec(`UPDATE plugin_packages SET settings=? WHERE id=?`, b, id); err != nil {
		return Installed{}, err
	}
	m.pkgs.invalidate()
	return m.GetPackage(id)
}

// ---------------------------------------------------------------- runtime

func (p *packages) headHTML() string {
	var b strings.Builder
	for _, k := range p.active() {
		if !k.mf.HasPermission("head") {
			continue
		}
		for _, h := range k.mf.Head {
			u := Esc("/plugin-assets/" + k.mf.ID + "/" + h + "?v=" + k.version)
			if strings.HasSuffix(strings.ToLower(h), ".css") {
				b.WriteString(`<link rel="stylesheet" href="` + u + `">`)
			} else {
				b.WriteString(`<script defer src="` + u + `"></script>`)
			}
		}
	}
	return b.String()
}

func pagePost(pi PageInfo) map[string]string {
	return map[string]string{"id": strconv.FormatInt(pi.PostID, 10), "title": pi.Title, "url": pi.Path,
		"type": pi.PostType, "description": pi.Description, "image": pi.Image}
}

func (p *packages) bodyEndHTML(pi PageInfo) string {
	var b strings.Builder
	for _, k := range p.active() {
		if k.mf.BodyEnd == "" || !k.mf.HasPermission("bodyEnd") {
			continue
		}
		b.WriteString(RenderTemplate(p.file(k, k.mf.BodyEnd), TemplateData{Settings: k.settings,
			Site: pi.Site, Post: pagePost(pi)}))
	}
	return b.String()
}

func (p *packages) blockInfos() []BlockInfo {
	var out []BlockInfo
	for _, k := range p.active() {
		if !k.mf.HasPermission("blocks") {
			continue
		}
		for _, b := range k.mf.Blocks {
			props := b.Props
			if props == nil {
				props = []Field{}
			}
			icon := b.Icon
			if icon == "" {
				icon = k.mf.Icon
			}
			out = append(out, BlockInfo{Plugin: k.mf.ID, PluginName: k.mf.Name, Type: b.Type, Name: b.Name,
				Icon: icon, Props: props})
		}
	}
	return out
}

func (p *packages) renderBlock(c design.BlockCall) (string, string, bool) {
	k := p.get(c.Plugin)
	if k == nil || !k.active || !k.mf.HasPermission("blocks") {
		return "", "", false
	}
	for _, b := range k.mf.Blocks {
		if b.Type != c.Block {
			continue
		}
		if b.Render == "service" {
			html, css := p.renderServiceBlock(k, b, c)
			return html, css, true
		}
		html := RenderTemplate(p.file(k, b.Template), TemplateData{Settings: k.settings,
			Props: Sanitize(b.Props, c.Values), Site: c.Site, Post: c.Post, Item: c.Item})
		css := ""
		if b.CSS != "" {
			css = ScopeCSS(p.file(k, b.CSS), "."+BlockClass(k.mf.ID, b.Type))
		}
		return html, css, true
	}
	return "", "", false
}

// webhooks POSTs {event, plugin, data, sentAt} to every active package
// subscribed to event (async, 5 s timeout, http/https only).
func (p *packages) webhooks(event string, payload map[string]any) {
	p.serviceEvents(event, payload)
	for _, k := range p.active() {
		if !k.mf.HasPermission("webhooks") {
			continue
		}
		for _, h := range k.mf.Webhooks {
			if h.Event != event {
				continue
			}
			u := strings.TrimSpace(RenderText(h.URL, TemplateData{Settings: k.settings}))
			if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
				continue // unset setting, or unsafe scheme
			}
			body, _ := json.Marshal(map[string]any{"event": event, "plugin": k.mf.ID, "data": payload,
				"sentAt": time.Now().UTC().Format(time.RFC3339)})
			go p.post(k.mf.ID, u, event, body)
		}
	}
}

func (p *packages) post(id, u, event string, body []byte) {
	if err := PostJSON(p.client, u, event, body); err != nil {
		log.Printf("plugin package %s: webhook %s: %v", id, event, err)
	}
}

// PostJSON sends a webhook request (shared with built-in plugins).
func PostJSON(client *http.Client, u, event string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AnasakiStudio-Webhook/1.0")
	req.Header.Set("X-Anasaki-Event", event)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// assetTypes are the files served at /plugin-assets/ (templates, markdown
// and JSON stay server-side).
var assetTypes = map[string]string{
	".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".txt": "text/plain; charset=utf-8",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".svg": "image/svg+xml", ".woff": "font/woff", ".woff2": "font/woff2", ".ttf": "font/ttf", ".otf": "font/otf",
}

// AssetURL is the public URL of a package file served at /plugin-assets/
// ("" when that file type is not served publicly).
func AssetURL(id, version, rel string) string {
	if _, ok := assetTypes[strings.ToLower(path.Ext(rel))]; !ok {
		return ""
	}
	return "/plugin-assets/" + id + "/" + rel + "?v=" + version
}

// serveAsset serves /plugin-assets/<id>/<path> for active packages (the
// manifest icon is served for inactive ones too, for the admin list).
func (p *packages) serveAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/plugin-assets/")
	id, rel, ok := strings.Cut(rest, "/")
	k := p.get(id)
	if !ok || k == nil || checkZipPath(rel) != nil || (!k.active && rel != k.mf.IconPath()) {
		http.NotFound(w, r)
		return
	}
	ctype, ok := assetTypes[strings.ToLower(path.Ext(rel))]
	if !ok {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(k.dir, filepath.FromSlash(rel))
	if !strings.HasPrefix(full, filepath.Clean(k.dir)+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	fi, err := os.Lstat(full)
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	if strings.HasSuffix(ctype, "svg+xml") {
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	}
	if r.URL.Query().Get("v") == k.version {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "public, max-age=300")
	}
	http.ServeContent(w, r, "", fi.ModTime(), f)
}
