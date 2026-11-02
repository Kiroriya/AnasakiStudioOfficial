package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/store"
)

// OptionActive is the options key holding the active plugin ids.
const OptionActive = "active_plugins"

// ErrUnknown is returned for an id that is not compiled in.
var ErrUnknown = errors.New("unknown plugin")

type entry struct {
	p    Plugin
	info Info
	reg  *Registry
}

// Manager owns every compiled-in plugin and their activation state.
type Manager struct {
	host    *Host
	entries []*entry
	byID    map[string]*entry

	mu       sync.Mutex
	active   map[string]bool
	loadedAt time.Time

	setMu    sync.Mutex
	settings map[string]cachedValues

	pkgs *packages

	serviceAPI ServiceAPIFunc // core service-API routes (set by the api package)

	listeners []func(event string, payload map[string]any) // core subscribers of Emit (Site Code)

	code CodeRuntime // the Site Code engine (JS plugins), nil until attached
}

// Status is the API representation of a plugin.
type Status struct {
	Info
	Active      bool         `json:"active"`
	Menu        []MenuItem   `json:"menu"`
	Collections []Collection `json:"collections"`
	// HasSettings: the plugin declares a settings schema
	// (GET/PUT /api/v1/plugin-settings/<id>).
	HasSettings bool `json:"hasSettings"`
	// DefaultActive: switched on automatically at first start.
	DefaultActive bool `json:"defaultActive"`
	// Kind is "service" for an active service plugin (a package), "" for built-ins.
	Kind string `json:"kind,omitempty"`
}

// NewManager migrates and registers every plugin.
// SetMailer lets plugins send email (wired to the core mail settings).
func (m *Manager) SetMailer(fn func(to []string, subject, html, replyTo string) error) {
	m.host.mail = fn
}

func NewManager(st *store.Store, uploadDir string, plugins ...Plugin) (*Manager, error) {
	m := &Manager{host: &Host{Store: st, UploadDir: uploadDir}, byID: map[string]*entry{},
		settings: map[string]cachedValues{}}
	m.host.mgr = m
	for _, p := range plugins {
		info := p.Info()
		if _, dup := m.byID[info.ID]; dup || info.ID == "" {
			return nil, fmt.Errorf("plugin: duplicate or empty id %q", info.ID)
		}
		if err := p.Migrate(st.DB); err != nil {
			return nil, fmt.Errorf("plugin %s: migrate: %w", info.ID, err)
		}
		reg := &Registry{id: info.ID, host: m.host, mgr: m, public: http.NewServeMux()}
		p.Register(reg)
		e := &entry{p: p, info: info, reg: reg}
		m.entries = append(m.entries, e)
		m.byID[info.ID] = e
	}
	pk, err := newPackages(m)
	if err != nil {
		return nil, err
	}
	m.pkgs = pk
	m.syncAllCaps()
	auth.SetPluginCapSource(pk.capCatalog)
	m.applyDefaults()
	go pk.warmUp()
	return m, nil
}

// SetPages attaches the public page renderer (the site).
func (m *Manager) SetPages(p Pages) { m.host.Pages = p }

// activeSet returns the active ids, re-reading the option at most every two
// seconds so several server processes sharing a database stay in sync.
func (m *Manager) activeSet() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && time.Since(m.loadedAt) < 2*time.Second {
		return m.active
	}
	set := map[string]bool{}
	var raw string
	err := m.host.Store.DB.QueryRow(`SELECT value FROM options WHERE name = ?`, OptionActive).Scan(&raw)
	if err == nil {
		var ids []string
		_ = json.Unmarshal([]byte(raw), &ids)
		for _, id := range ids {
			set[id] = true
		}
	}
	m.active, m.loadedAt = set, time.Now()
	return set
}

// Active reports whether plugin id is switched on.
func (m *Manager) Active(id string) bool { return m.activeSet()[id] }

func (m *Manager) status(e *entry) Status {
	s := Status{Info: e.info, Active: m.Active(e.info.ID), Menu: e.reg.menu, Collections: e.reg.collections,
		HasSettings: len(e.reg.settings) > 0, DefaultActive: e.reg.defaultActive}
	if s.Menu == nil {
		s.Menu = []MenuItem{}
	}
	if s.Collections == nil {
		s.Collections = []Collection{}
	}
	return s
}

// List returns every compiled-in plugin.
func (m *Manager) List() []Status {
	out := make([]Status, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, m.status(e))
	}
	return out
}

// Get returns one plugin's status.
func (m *Manager) Get(id string) (Status, error) {
	e := m.byID[id]
	if e == nil {
		return Status{}, ErrUnknown
	}
	return m.status(e), nil
}

// Activate runs the plugin's first-time setup and switches it on.
func (m *Manager) Activate(id string, userID int64) (Status, error) {
	e := m.byID[id]
	if e == nil {
		return Status{}, ErrUnknown
	}
	if err := e.p.Activate(ActivateContext{Store: m.host.Store, UploadDir: m.host.UploadDir, UserID: userID}); err != nil {
		return Status{}, fmt.Errorf("plugin %s: activate: %w", id, err)
	}
	if err := m.setActive(id, true); err != nil {
		return Status{}, err
	}
	return m.status(e), nil
}

// Deactivate switches a plugin off; its data stays in place.
func (m *Manager) Deactivate(id string) (Status, error) {
	e := m.byID[id]
	if e == nil {
		return Status{}, ErrUnknown
	}
	if err := m.setActive(id, false); err != nil {
		return Status{}, err
	}
	return m.status(e), nil
}

func (m *Manager) setActive(id string, on bool) error {
	m.mu.Lock()
	m.active = nil // force a fresh read
	m.mu.Unlock()
	set := m.activeSet()
	ids := []string{}
	for _, e := range m.entries { // keep registration order, drop unknown ids
		if (set[e.info.ID] && e.info.ID != id) || (e.info.ID == id && on) {
			ids = append(ids, e.info.ID)
		}
	}
	b, _ := json.Marshal(ids)
	if err := m.host.Store.SetOptions(map[string]string{OptionActive: string(b)}); err != nil {
		return err
	}
	m.mu.Lock()
	m.active = nil
	m.mu.Unlock()
	return nil
}

// Routes returns every plugin API route (active or not; callers must guard
// with Active so routes can be mounted once at startup).
func (m *Manager) Routes() []Route {
	var out []Route
	for _, e := range m.entries {
		out = append(out, e.reg.routes...)
	}
	return out
}

// ServePublic lets active plugins handle a public site request. It reports
// whether a plugin route matched.
func (m *Manager) ServePublic(w http.ResponseWriter, r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/plugin-assets/") {
		m.pkgs.serveAsset(w, r)
		return true
	}
	if m.pkgs.servePublic(w, r, ClientIP(r)) {
		return true
	}
	set := m.activeSet()
	for _, e := range m.entries {
		if !e.reg.hasPublic || !set[e.info.ID] {
			continue
		}
		if _, pattern := e.reg.public.Handler(r); pattern != "" {
			e.reg.public.ServeHTTP(w, r) // via the mux so r.PathValue works
			return true
		}
	}
	return false
}

// HeadTags returns the <head> HTML of every active plugin.
func (m *Manager) HeadTags() string {
	set := m.activeSet()
	var b strings.Builder
	for _, e := range m.entries {
		if set[e.info.ID] {
			for _, h := range e.reg.head {
				b.WriteString(h)
			}
		}
	}
	return b.String()
}

// Collection returns the items of an active plugin's collection source.
func (m *Manager) Collection(source string, query map[string]any) []design.PostCtx {
	set := m.activeSet()
	for _, e := range m.entries {
		if !set[e.info.ID] {
			continue
		}
		for _, c := range e.reg.collections {
			if c.Source == source && c.Items != nil {
				return c.Items(query)
			}
		}
	}
	items, _ := m.pkgs.serviceCollection(source, query)
	return items
}

// ---------------------------------------------------------------- helpers
// JSON helpers mirroring internal/api's so plugin handlers answer alike.

// WriteJSON writes v as JSON.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteErr writes {"error": msg}.
func WriteErr(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// ServerErr maps store.ErrNotFound to 404 and logs anything else as a 500.
func ServerErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		WriteErr(w, http.StatusNotFound, "ไม่พบข้อมูลที่ต้องการ (อาจถูกลบไปแล้ว)")
		return
	}
	log.Printf("plugin api error: %v", err)
	WriteErr(w, http.StatusInternalServerError, "เกิดข้อผิดพลาดในระบบ กรุณาลองใหม่อีกครั้ง")
}

// ReadJSON decodes the body (max limit bytes) and answers 400 on failure.
func ReadJSON(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		WriteErr(w, http.StatusBadRequest, "ข้อมูลที่ส่งมาไม่ถูกต้อง")
		return false
	}
	return true
}
