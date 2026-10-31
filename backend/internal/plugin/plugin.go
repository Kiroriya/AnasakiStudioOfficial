// Package plugin is Anasaki Studio's take on WordPress plugins: features
// compiled into the binary (built-in plugins) plus safe, data-only "package
// plugins" installed at runtime from a zip or the plugin store.
//
// Every registered built-in plugin is migrated and registered at startup; its
// routes, public pages, hooks, blocks, collections and menu entries only take
// effect while it is active. Active built-in ids live in the options table
// under "active_plugins" as a JSON array; package plugins live in the
// plugin_packages table.
package plugin

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/store"
)

// Info describes a plugin in the admin plugin list.
type Info struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Icon        string `json:"icon"` // an emoji
}

// MenuItem is an admin sidebar entry the frontend shows while the plugin is
// active (and the user has Cap; "" means any logged-in user).
type MenuItem struct {
	Path    string `json:"path"`
	Label   string `json:"label"`
	LabelEn string `json:"labelEn"`
	Icon    string `json:"icon"`
	Cap     string `json:"cap"`
}

// Plugin is implemented by every compiled-in plugin.
type Plugin interface {
	Info() Info
	// Migrate creates the plugin's tables; idempotent, run at every startup.
	Migrate(db *sql.DB) error
	// Register declares routes, public routes, hooks, blocks, menu…
	Register(r *Registry)
	// Activate performs first-time setup (sample data…); must be idempotent.
	Activate(ctx ActivateContext) error
}

// ActivateContext is handed to Plugin.Activate.
type ActivateContext struct {
	Store     *store.Store
	UploadDir string
	UserID    int64 // the administrator switching the plugin on (0 for defaults)
}

// Pages lets plugins render public pages with the site's look (implemented
// by the site package and attached with Manager.SetPages).
type Pages interface {
	// Page renders the published page with slug; false when there is none.
	Page(w http.ResponseWriter, slug string) bool
	// Template renders template post id with ctx as the {{post.*}} binding
	// context; false when the template does not exist.
	Template(w http.ResponseWriter, id int64, ctx design.PostCtx, title, description string) bool
	// HTML wraps body with the site's header and footer components.
	HTML(w http.ResponseWriter, status int, title, body string)
}

// Host is the runtime environment the core provides to plugin handlers.
type Host struct {
	Store     *store.Store
	UploadDir string
	Pages     Pages // set before the server starts serving

	mgr  *Manager
	mail func(to []string, subject, html, replyTo string) error
}

// SendMail sends an email through the SMTP server configured in Settings → Email.
// It fails with mail.ErrNotConfigured when email is not set up yet.
func (h *Host) SendMail(to []string, subject, html, replyTo string) error {
	if h.mail == nil {
		return errors.New("mail is not available")
	}
	return h.mail(to, subject, html, replyTo)
}

// Active reports whether a built-in plugin is switched on.
func (h *Host) Active(id string) bool { return h.mgr != nil && h.mgr.Active(id) }

// Emit publishes an event to every active plugin and package webhook.
func (h *Host) Emit(event string, payload map[string]any) {
	if h.mgr != nil {
		h.mgr.Emit(event, payload)
	}
}

// Route is an authenticated API route of a plugin.
type Route struct {
	PluginID string
	Pattern  string   // full ServeMux pattern, e.g. "GET /api/v1/shop/products"
	Caps     []string // caller needs any of these (none: any logged-in user)
	Handler  http.HandlerFunc
}

// Collection is a data source for the designer's collection node.
type Collection struct {
	Source string                                      `json:"source"` // e.g. "products"
	Label  string                                      `json:"label"`
	Items  func(query map[string]any) []design.PostCtx `json:"-"`
}

// Registry collects what a plugin contributes. It is filled once, at startup.
type Registry struct {
	id            string
	host          *Host
	mgr           *Manager
	routes        []Route
	public        *http.ServeMux
	hasPublic     bool
	head          []string
	collections   []Collection
	menu          []MenuItem
	middleware    []func(http.Handler) http.Handler
	events        map[string][]func(map[string]any)
	heads         []func(PageInfo) string
	bodyEnds      []func(PageInfo) string
	filters       []func(*PageInfo)
	settings      []Field
	settingsFix   func(Values) Values
	blocks        []*Block
	defaultActive bool
}

// ID returns the plugin id the registry belongs to.
func (r *Registry) ID() string { return r.id }

// Host returns the runtime environment (its Pages field is set later, so
// keep the pointer rather than copying the struct).
func (r *Registry) Host() *Host { return r.host }

// Handle adds an authenticated API route. pattern is "METHOD /path" relative
// to /api/v1/<plugin id>, e.g. "GET /products/{id}".
func (r *Registry) Handle(pattern string, h http.HandlerFunc, caps ...string) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		method, path = "", pattern
	}
	full := "/api/v1/" + r.id + path
	if method != "" {
		full = method + " " + full
	}
	r.routes = append(r.routes, Route{PluginID: r.id, Pattern: full, Caps: caps, Handler: h})
}

// HandlePublic adds a public site route (a ServeMux pattern such as
// "GET /product/{slug}"). The site delegates to it before its own routing.
func (r *Registry) HandlePublic(pattern string, h http.HandlerFunc) {
	r.public.HandleFunc(pattern, h)
	r.hasPublic = true
}

// HeadTag injects static raw HTML into <head> of every public page.
func (r *Registry) HeadTag(html string) { r.head = append(r.head, html) }

// Head adds a per-page <head> hook (meta tags, analytics snippets…).
func (r *Registry) Head(fn func(PageInfo) string) { r.heads = append(r.heads, fn) }

// BodyEnd adds a per-page hook whose HTML is appended before </body>.
func (r *Registry) BodyEnd(fn func(PageInfo) string) { r.bodyEnds = append(r.bodyEnds, fn) }

// PageFilter adds a hook that may change a page's title, description, image
// or robots flag before the document head is written (SEO overrides).
func (r *Registry) PageFilter(fn func(*PageInfo)) { r.filters = append(r.filters, fn) }

// Middleware wraps the public site while the plugin is active (redirects,
// maintenance mode, analytics…). Middlewares run in plugin order, the first
// registered plugin being the outermost.
func (r *Registry) Middleware(mw func(http.Handler) http.Handler) {
	r.middleware = append(r.middleware, mw)
}

// On subscribes to a core event ("post.published", "user.created",
// "media.uploaded", "form.submitted"…). Handlers run asynchronously.
func (r *Registry) On(event string, fn func(payload map[string]any)) {
	if r.events == nil {
		r.events = map[string][]func(map[string]any){}
	}
	r.events[event] = append(r.events[event], fn)
}

// Settings declares the plugin's settings schema (rendered generically by the
// admin; values via GET/PUT /api/v1/plugin-settings/<id>).
func (r *Registry) Settings(fields ...Field) { r.settings = append(r.settings, fields...) }

// SettingsFilter registers fn to complete settings before they are stored
// (e.g. generate a secret left empty). fn gets a copy and returns the values
// to keep; it also runs when the settings are read, and its result is then
// stored if it changed anything (values saved before the filter existed).
func (r *Registry) SettingsFilter(fn func(Values) Values) { r.settingsFix = fn }

// Values returns the plugin's current settings (defaults applied).
func (r *Registry) Values() Values { return r.mgr.settingsValues(r.id, r.settings) }

// Block registers a designer block rendered by Go code.
func (r *Registry) Block(b Block) {
	b.Plugin = r.id
	r.blocks = append(r.blocks, &b)
}

// DefaultActive marks the plugin as switched on at first start (applied once;
// a later deactivation is respected).
func (r *Registry) DefaultActive() { r.defaultActive = true }

// Collection registers a data source for collection nodes.
func (r *Registry) Collection(c Collection) { r.collections = append(r.collections, c) }

// Menu adds admin sidebar entries.
func (r *Registry) Menu(items ...MenuItem) { r.menu = append(r.menu, items...) }

// Grant adds capabilities to a role (e.g. a plugin-specific role).
func (r *Registry) Grant(role string, caps ...string) { auth.Grant(role, caps...) }
