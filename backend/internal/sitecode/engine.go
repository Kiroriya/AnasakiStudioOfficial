package sitecode

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/store"
)

// Config wires the engine to the rest of the server.
type Config struct {
	Disabled bool       // env SITE_CODE_DISABLED=true
	Demo     bool       // DEMO_MODE: Site Code (and uploaded JS plugins) are off on the playground; Store plugins run
	Auth     *auth.Auth // to recognise an admin bearer token on public routes (ctx.user)
	// Services returns the public path prefixes of active service plugins.
	Services func() []string
	// PostURL builds a post's public path (site.PostURL).
	PostURL func(p *model.Post) string
	// DSN is the CMS database DSN (the CMS user also manages the code
	// database); CodeDSN is CODE_DB_DSN ("" = fallback mode).
	DSN     string
	CodeDSN string

	// phase 4a
	// Emit delivers a core event (post.saved, …) to plugins and Site Code
	// (plugin.Manager.Emit); nil = Site Code handlers only.
	Emit func(event string, payload map[string]any)
	// Mailer sends mail.send messages (SMTP from the site settings); nil =
	// mail.send throws "not configured".
	Mailer func(m MailMessage) error
	// SecretKey is SITE_CODE_SECRET_KEY; KeyDir holds site-code.key when it
	// is empty (PLUGINS_DIR).
	SecretKey string
	KeyDir    string
	// UploadDir is the media library's folder (media.*, public files.*; 2.1.0).
	UploadDir string
	// TypeSlugs receives the public URL prefixes of custom post types
	// (site.SetTypeSlugs) whenever the live version changes.
	TypeSlugs func(map[string]string)

	// phase 4b
	// Plugins are the installed JS plugins and the WASM/service plugin bridge
	// (plugin.Manager); nil = Site Code only.
	Plugins PluginHost
}

// PluginHost is what the engine needs from the plugin manager.
type PluginHost interface {
	CodeUnits() []plugin.CodeUnitRef
	CodeFiles(id string) (map[string]string, error)
	CallFunction(id, fn string, body []byte, timeout time.Duration) (int, []byte, error)
	HasFunction(id, fn string) bool
	DeliverEvent(event string, payload map[string]any)
}

// Engine owns the live Site Code: it loads the published version, serves
// its routes on the public site, delivers core events and runs the admin
// operations (build, publish, restore, test runs).
type Engine struct {
	st   *store.Store
	DB   *DB
	cfg  Config
	h    *host
	data *Data

	// Limits (tests shorten them).
	RouteTimeout time.Duration // 2 s
	EventTimeout time.Duration // 10 s
	QueueWait    time.Duration // 2 s for a free VM slot, then 503
	// phase 4a limits
	FilterTimeout       time.Duration // 2 s (post.beforeSave, action.*, mail.beforeSend, custom)
	FilterRenderTimeout time.Duration // 50 ms (post.content, page.head, collection.items)
	JobPoll             time.Duration // worker poll interval (1 s)
	JobBackoff          time.Duration // first retry delay (30 s, doubled each attempt)
	JobWorkers          int           // worker goroutines (2)
	CronEvery           time.Duration // cron evaluation interval (1 min)
	// phase 6 limits (0 = defaults: 20 ms per site.middleware, 500 ms per site.afterResponse)
	MiddlewareTimeout time.Duration
	AfterTimeout      time.Duration
	Now               func() time.Time

	sem       chan struct{} // at most 8 invocations at once
	breaker   *Breaker
	cache     *Cache
	collCache *Cache                      // site.collection results (cache: seconds), keyed by role
	access    atomic.Pointer[accessCache] // script_access overrides
	limiter   *rateLimiter                // POST /_a: 30 / minute / IP per action

	loadMu  sync.Mutex // loading a version
	pubMu   sync.Mutex // publish / restore
	live    atomic.Pointer[liveProg]
	lastErr atomic.Pointer[string]
	logN    atomic.Int64
	events  sync.WaitGroup // running event handlers (tests wait on it)

	mailLimit    *rateLimiter // mail.send per hour (setting)
	routeLimit   *rateLimiter // public routes per IP per minute (setting)
	secrets      secretBox
	settings     atomic.Pointer[settingsCache]
	manifests    sync.Map // version id → Manifest (post types for the admin / site)
	worker       *worker
	fetchHook    *fetchHook             // tests: dial override for http.fetch
	typeSlugsKey atomic.Pointer[string] // version + unit set whose post type slugs were pushed

	// phase 4b: code units
	siteU      *unitInfo // Site Code
	unitSetP   atomic.Pointer[unitSet]
	unitsMu    sync.Mutex
	unitsDirty atomic.Bool
	routeTab   atomic.Pointer[routeTable]
	unitErr    sync.Map // unit id → last runtime error (status)
	emptyOnce  sync.Once
	empty      *Program

	after afterQueue // site.afterResponse (phase 6)

	// phase 7 joints
	gens        sync.Map     // unit id → *int64Counter: data written (cache fingerprints)
	settingsGen atomic.Int64 // script_settings / manifest settings saves
	// WebhookClient sends http.webhook requests (nil = public addresses
	// only, 5 s); tests point it at a local server.
	WebhookClient *http.Client
	// BlockTimeout limits a site.block render (0 = 200 ms).
	BlockTimeout time.Duration
	blockCache   *Cache // site.block results with cache: seconds
}

type int64Counter = atomic.Int64

// liveProg is a loaded unit: the published Site Code version, or an active
// JS plugin (id 0).
type liveProg struct {
	id    int64
	unit  *unitInfo
	prog  *Program // nil when the version failed to load
	mux   *http.ServeMux
	ready chan *vm // pre-warmed VMs (each used once)
	files map[string]string
}

// maxConcurrent is the size of the VM pool.
const maxConcurrent = 8

// New creates the engine.
func New(st *store.Store, cfg Config) *Engine {
	e := &Engine{st: st, DB: &DB{db: st.DB}, cfg: cfg,
		RouteTimeout: 2 * time.Second, EventTimeout: 10 * time.Second, QueueWait: 2 * time.Second,
		sem: make(chan struct{}, maxConcurrent), breaker: NewBreaker(), cache: NewCache(), collCache: NewCache(), blockCache: NewCache(),
		limiter: newRateLimiter(30, time.Minute), mailLimit: newRateLimiter(100, time.Hour), routeLimit: newRateLimiter(120, time.Minute),
		FilterTimeout: 2 * time.Second, FilterRenderTimeout: 50 * time.Millisecond, JobPoll: time.Second, JobBackoff: 30 * time.Second,
		JobWorkers: 2, CronEvery: time.Minute, Now: time.Now}
	e.data = OpenData(st.DB, cfg.DSN, cfg.CodeDSN)
	e.h = &host{cache: e.cache, posts: st, postURL: cfg.PostURL, data: e.data, e: e}
	e.siteU = &unitInfo{ID: SiteUnit, n: e.data.N}
	return e
}

// Data is the site's own database.
func (e *Engine) Data() *Data { return e.data }

// Close releases the code database connections.
func (e *Engine) Close() { e.data.Close() }

// EnvDisabled reports SITE_CODE_DISABLED.
func (e *Engine) EnvDisabled() bool { return e.cfg.Disabled }

// Demo reports demo mode.
func (e *Engine) Demo() bool { return e.cfg.Demo }

// MsgDemoStoreOnly refuses a JS plugin that is not from the Store in demo mode.
const MsgDemoStoreOnly = "ปิดในเว็บทดลอง — เฉพาะปลั๊กอินจาก Store (disabled on the demo site — only plugins from the Store run)"

// msgDemoSite marks Site Code (the site unit) as off in demo mode.
const msgDemoSite = "ปิดในเว็บทดลอง"

// pluginsOff: no code runs at all — no JS plugin and no Site Code (safe
// mode or SITE_CODE_DISABLED).
func (e *Engine) pluginsOff(opts map[string]string) bool {
	return e.cfg.Disabled || opts[OptSafeMode] == "1"
}

// siteOff: the site unit (the site's own code) does not run: pluginsOff,
// or demo mode (JS plugins from the Store still run there).
func (e *Engine) siteOff(opts map[string]string) bool {
	return e.pluginsOff(opts) || e.cfg.Demo
}

// unitAllowed reports whether a JS plugin may be activated and run: in demo
// mode only packages installed from the Store.
func (e *Engine) unitAllowed(ref plugin.CodeUnitRef) bool {
	return !e.cfg.Demo || ref.Source == plugin.SourceStore
}

func (e *Engine) services() []string {
	if e.cfg.Services == nil {
		return nil
	}
	return e.cfg.Services()
}

// reserved returns the reserved first path segments under opts.
func (e *Engine) reserved(opts map[string]string) map[string]bool {
	return Reserved(opts, e.services())
}

func liveID(opts map[string]string) int64 {
	id, _ := strconv.ParseInt(opts[OptVersion], 10, 64)
	return id
}

// current returns the loaded live version, loading it on a change (another
// server process may have published). nil while the site unit is off
// (siteOff: safe mode, SITE_CODE_DISABLED, demo mode).
func (e *Engine) current(opts map[string]string) *liveProg {
	id := liveID(opts)
	if id == 0 || e.siteOff(opts) {
		return nil
	}
	if lp := e.live.Load(); lp != nil && lp.id == id {
		return lp
	}
	e.loadMu.Lock()
	defer e.loadMu.Unlock()
	if lp := e.live.Load(); lp != nil && lp.id == id {
		return lp
	}
	lp := &liveProg{id: id, unit: e.siteU, ready: make(chan *vm, 4)}
	row, err := e.DB.GetVersion(id, id, true)
	if err == nil {
		lp.files = row.FileMap // public/ files (/site-assets/, site.asset) — 2.2.0
		prog, probs := RegisterUnit(row.Bundle, map[string]bool{}, e.h, e.RouteTimeout, e.siteU)
		if len(probs) > 0 {
			err = fmt.Errorf("%s", probs[0].Message)
		} else {
			lp.prog = prog
			lp.mux, err = newMux(prog.specs, func(i int) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { e.serveRoute(lp, i, w, r) })
			})
		}
	}
	if err != nil {
		lp.prog, lp.mux = nil, nil
		e.setLastErr(fmt.Sprintf("โหลดเวอร์ชัน #%d ไม่สำเร็จ: %v", id, err))
		e.persist([]LogEntry{{Level: "error", Source: "load", Message: fmt.Sprintf("โหลดเวอร์ชัน #%d ไม่สำเร็จ: %v", id, err)}}, id)
	}
	e.live.Store(lp)
	return lp
}

// ServePublic answers a public request with a Site Code route; false when
// none matches (or Site Code is off), so normal site routing continues.
// opts are the options the site already loaded for this request.
func (e *Engine) ServePublic(w http.ResponseWriter, r *http.Request, opts map[string]string) bool {
	if opts[OptVersion] != "" && e.servePublicFile(w, r, opts) { // public/ files (2.2.0)
		return true
	}
	if e.pluginsOff(opts) {
		return false
	}
	set := e.units()
	if (opts[OptVersion] == "" || e.siteOff(opts)) && set.empty() {
		return false // fast path: nothing published (or Site Code off), no JS plugin
	}
	e.applyLimits(opts)
	var site *liveProg
	if opts[OptVersion] != "" && !e.siteOff(opts) {
		if lp := e.current(opts); lp != nil && lp.mux != nil && lp.prog != nil {
			site = lp
		}
	}
	rt := e.routes(site, set)
	if rt == nil || rt.n == 0 {
		return false
	}
	if _, pattern := rt.mux.Handler(r); pattern == "" {
		return false
	}
	if requestReserved(r.URL.Path, e.reserved(opts)) {
		return false
	}
	rt.mux.ServeHTTP(w, r) // via the mux so r.PathValue works
	return true
}

// acquire takes a VM slot, waiting at most d.
func (e *Engine) acquire(d time.Duration) (func(), bool) {
	select {
	case e.sem <- struct{}{}:
		return func() { <-e.sem }, true
	default:
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case e.sem <- struct{}{}:
		return func() { <-e.sem }, true
	case <-t.C:
		return nil, false
	}
}

// liveVM returns a fresh VM with the live bundle loaded (pre-warmed when
// possible) and starts warming its replacement.
func (e *Engine) liveVM(lp *liveProg) (*vm, error) {
	defer func() { go e.warm(lp) }()
	select {
	case v := <-lp.ready:
		return v, nil
	default:
	}
	return e.newLoaded(lp.prog)
}

func (e *Engine) newLoaded(p *Program) (*vm, error) {
	v := newUnitVM(e.h, p.unit)
	if err := v.load(p.prog, modeTop, e.RouteTimeout); err != nil {
		return nil, err
	}
	v.logs = nil // top-level logs were recorded at registration
	return v, nil
}

func (e *Engine) warm(lp *liveProg) {
	if len(lp.ready) == cap(lp.ready) || e.live.Load() != lp {
		return
	}
	select { // warming also takes one of the 8 VM slots; skipped when busy
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	default:
		return
	}
	v, err := e.newLoaded(lp.prog)
	if err != nil {
		return
	}
	select {
	case lp.ready <- v:
	default:
	}
}

// ---------------------------------------------------------------- routes

func routeKey(rs *routeSpec) string { return "route " + rs.Method + " " + rs.Path }

// params collects the path parameters ServeMux matched.
func params(rs *routeSpec, r *http.Request) map[string]string {
	out := map[string]string{}
	for _, name := range rs.Params {
		if name == "*" {
			out["*"] = r.PathValue("_rest")
		} else {
			out[name] = r.PathValue(name)
		}
	}
	return out
}

func (e *Engine) serveRoute(lp *liveProg, i int, w http.ResponseWriter, r *http.Request) {
	rs := lp.prog.specs[i]
	u := lp.unit
	key := u.src(routeKey(rs))
	if !e.breaker.Allow(key) {
		routeError(w, r, http.StatusServiceUnavailable, "ระบบปิดส่วนนี้ชั่วคราวเนื่องจากเกิดข้อผิดพลาดซ้ำ ๆ กรุณาลองใหม่ภายหลัง")
		return
	}
	user := e.userFrom(r)
	access := e.Effective(KindRoute, u.qualify(rs.Method+" "+rs.Path), lp.prog.access[i])
	if allowed(access, nil) && !e.routeLimit.Allow(plugin.ClientIP(r)) { // public routes: per-IP limit
		w.Header().Set("Retry-After", "60")
		routeError(w, r, http.StatusTooManyRequests, "เรียกใช้บ่อยเกินไป กรุณารอสักครู่แล้วลองใหม่")
		return
	}
	ok := allowed(access, user)
	needVisitor := !ok && user == nil && hasGroupEntries(access) && e.visitorUnit(nil) != ""
	deny := func() {
		if user == nil {
			routeError(w, r, http.StatusUnauthorized, "กรุณาเข้าสู่ระบบ")
		} else {
			routeError(w, r, http.StatusForbidden, "คุณไม่มีสิทธิ์เข้าถึงหน้านี้")
		}
	}
	if !ok && !needVisitor {
		deny()
		return
	}
	limit := maxBody
	if i < len(lp.prog.routeOpts) && lp.prog.routeOpts[i].maxBody > 0 {
		limit = lp.prog.routeOpts[i].maxBody
	}
	var body []byte
	if r.Body != nil {
		if r.ContentLength > int64(limit) {
			routeError(w, r, http.StatusRequestEntityTooLarge, "ข้อมูลที่ส่งมาใหญ่เกินกำหนด")
			return
		}
		body, _ = io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
		if len(body) > limit {
			routeError(w, r, http.StatusRequestEntityTooLarge, "ข้อมูลที่ส่งมาใหญ่เกินกำหนด")
			return
		}
	}
	release, okSlot := e.acquire(e.QueueWait)
	if !okSlot {
		w.Header().Set("Retry-After", "2")
		routeError(w, r, http.StatusServiceUnavailable, "ระบบไม่ว่าง กรุณาลองใหม่อีกครั้ง")
		return
	}
	defer release()
	var res *Response
	var logs []LogLine
	v, err := e.liveVM(lp)
	if err == nil {
		v.req, v.vis, v.bodyReq, v.body = r, &visitorState{}, r, body
		sess := e.attachSession(v, r, user)
		defer sess.Close()
		if needVisitor {
			vis := v.resolveVisitor()
			pre := unitEntries(logEntries(v.logs, key), u)
			if !allowedFor(access, nil, func() []string { return groupsOf(vis) }) {
				e.persist(pre, lp.id)
				deny()
				return
			}
			e.persist(pre, lp.id)
		}
		res, logs, err = e.execRoute(v, rs.Pattern, r, params(rs, r), user)
	}
	entries := unitEntries(logEntries(logs, key), u)
	if err != nil {
		re := toRunError(err)
		entries = append(entries, unitEntry(errorEntry(key, re), u))
		e.persist(entries, lp.id)
		e.failedIn(u, key, re, lp.id)
		if re.Timeout {
			routeError(w, r, http.StatusGatewayTimeout, "หมดเวลาในการประมวลผล กรุณาลองใหม่อีกครั้ง")
		} else {
			routeError(w, r, http.StatusInternalServerError, "เกิดข้อผิดพลาดในระบบ กรุณาลองใหม่อีกครั้ง")
		}
		return
	}
	e.persist(entries, lp.id)
	e.writeResponse(w, r, res)
}

// execRoute calls the route registered under pattern in v.
func (e *Engine) execRoute(v *vm, pattern string, r *http.Request, ps map[string]string, user *User) (*Response, []LogLine, error) {
	var reg *regRoute
	for _, rr := range v.routes {
		if rr.spec.Pattern == pattern {
			reg = rr
		}
	}
	if reg == nil {
		return nil, nil, &RunError{Message: "route นี้ไม่ได้ลงทะเบียนในรอบนี้ (site.route ต้องเรียกทุกครั้งแบบเดียวกัน)"}
	}
	v.rawBody, v.maxBody = reg.rawBody, reg.maxBody
	v.source = v.unit().src(routeKey(reg.spec))
	v.user, v.logs = user, nil
	v.mode = modeHandler
	ctx := v.ctxValue(r, ps)
	val, err := v.call(reg.fn, e.RouteTimeout, ctx)
	if err != nil {
		return nil, v.logs, err
	}
	var res *Response
	_, err = v.guarded(e.RouteTimeout, func() (goja.Value, error) {
		var err error
		res, err = v.response(val)
		return nil, err
	})
	if err != nil {
		return nil, v.logs, v.runErr(err)
	}
	res.Cookies = v.cookies
	return res, v.logs, nil
}

func toRunError(err error) *RunError {
	var re *RunError
	if errors.As(err, &re) {
		return re
	}
	return &RunError{Message: err.Error()}
}

// failed records an invocation failure with the circuit breaker.
func (e *Engine) failed(key string, re *RunError, version int64) {
	e.failedIn(e.siteU, key, re, version)
}

// failedIn is failed for a unit (a plugin's last error is kept apart from
// Site Code's).
func (e *Engine) failedIn(u *unitInfo, key string, re *RunError, version int64) {
	set := e.setLastErr
	if u.plugin() {
		set = func(s string) { e.unitErr.Store(u.ID, s) }
	}
	set(key + ": " + re.Error())
	if e.breaker.Fail(key) {
		msg := fmt.Sprintf("%s ล้มเหลว %d ครั้งใน 1 นาที — ปิดชั่วคราว 5 นาที", key, e.breaker.Threshold)
		set(msg)
		e.persist([]LogEntry{{Level: "error", Source: key, Message: msg, Unit: u.id()}}, version)
	}
}

func (e *Engine) setLastErr(s string) { e.lastErr.Store(&s) }

// userFrom resolves ctx.user from an admin bearer token (nil = anonymous).
func (e *Engine) userFrom(r *http.Request) *User {
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || e.cfg.Auth == nil {
		return nil
	}
	p, err := e.cfg.Auth.Parse(tok)
	if err != nil {
		return nil
	}
	u, hash, revoked, err := e.st.SessionCheck(p.ID, p.JTI)
	if err != nil || revoked || !e.cfg.Auth.Current(p, hash) || u.Status == model.UserSuspended {
		return nil
	}
	return &User{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role}
}

// routeError answers a failed route without details: JSON when the client
// asks for it, else a small HTML page.
func routeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.Header().Set("Cache-Control", "no-store")
	if strings.Contains(r.Header.Get("Accept"), "application/json") || strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "{\"error\":%q}\n", msg)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>%d</title><body style=\"font-family:sans-serif;padding:3rem;text-align:center\"><h1>%d</h1><p>%s</p></body>",
		status, status, html.EscapeString(msg))
}

func writeResponse(w http.ResponseWriter, r *http.Request, res *Response) {
	for k, v := range res.Headers {
		w.Header().Set(k, v)
	}
	for _, c := range res.Cookies {
		w.Header().Add("Set-Cookie", c)
	}
	w.WriteHeader(res.Status)
	if r.Method != http.MethodHead && res.Status != http.StatusNoContent && res.Status != http.StatusNotModified {
		_, _ = w.Write([]byte(res.Body))
	}
}

// ---------------------------------------------------------------- events

// OnEvent delivers a core event to the live code's site.on handlers
// (asynchronously; it never blocks the caller).
func (e *Engine) OnEvent(event string, payload map[string]any) { e.dispatch(event, payload) }

// dispatch delivers an event with any JSON payload (site.emit data).
func (e *Engine) dispatch(event string, payload any) {
	opts, err := e.st.Options()
	if err != nil || e.pluginsOff(opts) {
		return
	}
	var targets []*liveProg
	for _, lp := range e.units().loaded() {
		if lp.prog.events[event] {
			targets = append(targets, lp)
		}
	}
	if opts[OptVersion] != "" && !e.siteOff(opts) {
		if lp := e.current(opts); lp != nil && lp.prog != nil && lp.prog.events[event] {
			targets = append(targets, lp)
		}
	}
	for _, lp := range targets {
		lp := lp
		e.events.Add(1)
		go func() {
			defer e.events.Done()
			e.runEvent(lp, event, payload)
		}()
	}
}

// WaitEvents blocks until running event handlers finish (tests, shutdown).
func (e *Engine) WaitEvents() { e.events.Wait() }

func (e *Engine) runEvent(lp *liveProg, event string, payload any) {
	u := lp.unit
	source := u.src("event " + event)
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("site code: %s: panic: %v", source, rec)
		}
	}()
	release, ok := e.acquire(e.EventTimeout)
	if !ok {
		e.persist([]LogEntry{{Level: "warn", Source: source, Message: "ระบบไม่ว่าง: ข้ามเหตุการณ์นี้", Unit: u.id()}}, lp.id)
		return
	}
	defer release()
	// each handler of the event runs in its own fresh VM
	for idx := 0; ; idx++ {
		v, err := e.liveVM(lp)
		if err != nil {
			e.persist([]LogEntry{unitEntry(errorEntry(source, toRunError(err)), u)}, lp.id)
			return
		}
		var handlers []*regEvent
		for _, ev := range v.events {
			if ev.name == event {
				handlers = append(handlers, ev)
			}
		}
		if idx >= len(handlers) {
			return
		}
		key := source
		if len(handlers) > 1 {
			key = fmt.Sprintf("%s #%d", source, idx+1)
		}
		if !e.breaker.Allow(key) {
			continue
		}
		v.user, v.logs, v.source = nil, nil, source
		v.vis = &visitorState{done: true}
		sess := e.attachSession(v, nil, nil)
		_, err = v.call(handlers[idx].fn, e.EventTimeout, v.toJS(payload))
		sess.Close()
		entries := unitEntries(logEntries(v.logs, source), u)
		if err != nil {
			re := toRunError(err)
			entries = append(entries, unitEntry(errorEntry(source, re), u))
			e.persist(entries, lp.id)
			e.failedIn(u, key, re, lp.id)
		} else {
			e.persist(entries, lp.id)
		}
	}
}

// ---------------------------------------------------------------- logs

func logEntries(lines []LogLine, source string) []LogEntry {
	out := make([]LogEntry, 0, len(lines)+1)
	for _, l := range lines {
		out = append(out, LogEntry{Level: l.Level, Source: source, Message: l.Message})
	}
	return out
}

func errorEntry(source string, re *RunError) LogEntry {
	e := LogEntry{Level: "error", Source: source, Message: re.Error()}
	if re.Trace != "" {
		t := re.Trace
		e.Trace = &t
	}
	return e
}

// persist writes log rows for version (0 = none) and prunes now and then.
func (e *Engine) persist(entries []LogEntry, version int64) {
	if len(entries) == 0 || e.DB == nil || e.DB.db == nil {
		return
	}
	at := time.Now().UTC()
	for i := range entries {
		entries[i].At = at
		if entries[i].Unit == "" {
			entries[i].Unit = SiteUnit
		}
		if version > 0 && entries[i].Unit == SiteUnit {
			v := version
			entries[i].Version = &v
		}
	}
	if err := e.DB.InsertLogs(entries); err != nil {
		log.Printf("site code: write log: %v", err)
	}
	if e.logN.Add(int64(len(entries)))%500 < int64(len(entries)) {
		go func() { _ = e.DB.PruneLogs() }()
	}
}

// Log writes one row (used by the admin operations, e.g. publish).
func (e *Engine) Log(level, source, msg string, version int64) {
	e.persist([]LogEntry{{Level: level, Source: source, Message: msg}}, version)
}

// ---------------------------------------------------------------- status

// Status is GET /status.
type Status struct {
	SafeMode     bool     `json:"safeMode"`
	EnvDisabled  bool     `json:"envDisabled"`
	Demo         bool     `json:"demo"`
	LiveVersion  *int64   `json:"liveVersion"`
	DraftChanged bool     `json:"draftChanged"`
	Registered   Manifest `json:"registered"`
	LastError    *string  `json:"lastError"`
	DB           DBState  `json:"db"`
	// phase 4b: the unit shown ("site" or a JS plugin id) and every unit.
	Unit            string     `json:"unit"`
	ActivationError string     `json:"activationError,omitempty"`
	Units           []UnitInfo `json:"units"`
}

// UnitInfo is one code unit of GET /status (the unit switcher).
type UnitInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Plugin  bool   `json:"plugin"`
	Active  bool   `json:"active"`
	Loaded  bool   `json:"loaded"`
	Error   string `json:"error,omitempty"`
}

// Units lists Site Code first, then the installed JS plugins (load order).
func (e *Engine) Units() []UnitInfo {
	out := []UnitInfo{}
	set := e.units()
	seen := map[string]bool{}
	for _, lp := range set.loaded() {
		seen[lp.unit.ID] = true
		out = append(out, UnitInfo{ID: lp.unit.ID, Name: lp.unit.Name, Version: lp.unit.Version, Plugin: true, Active: true, Loaded: true})
	}
	if set != nil {
		var rest []UnitInfo
		for id, r := range set.refs {
			if !seen[id] {
				rest = append(rest, UnitInfo{ID: id, Name: r.Name, Version: r.Version, Plugin: true, Active: r.Active, Error: set.errs[id]})
			}
		}
		sort.Slice(rest, func(i, j int) bool { return rest[i].ID < rest[j].ID })
		out = append(out, rest...)
	}
	site := UnitInfo{ID: SiteUnit, Name: "Site Code", Active: true, Loaded: true}
	if e.cfg.Demo {
		site = UnitInfo{ID: SiteUnit, Name: "Site Code", Error: msgDemoSite}
	}
	return append([]UnitInfo{site}, out...)
}

// DBState is the "db" part of GET /status.
type DBState struct {
	Mode              string `json:"mode"`
	PendingMigrations int    `json:"pendingMigrations"`
}

// Status reports the runtime state.
func (e *Engine) Status() (*Status, error) {
	opts, err := e.st.Options()
	if err != nil {
		return nil, err
	}
	s := &Status{SafeMode: opts[OptSafeMode] == "1", EnvDisabled: e.cfg.Disabled, Demo: e.cfg.Demo, LastError: e.lastErr.Load(),
		Unit: SiteUnit, Units: []UnitInfo{}}
	s.Registered.normalize()
	s.Units = e.Units() // demo: the Store plugins that run, Site Code off
	if e.data != nil {
		s.DB.Mode = e.data.Mode
	}
	if e.cfg.Demo {
		return s, nil // the site unit is off: no draft, version or manifest
	}
	draft, err := e.DB.Draft()
	if err != nil {
		return nil, err
	}
	s.DB.PendingMigrations = e.pendingCount(draft)
	if id := liveID(opts); id > 0 {
		s.LiveVersion = &id
		row, err := e.DB.GetVersion(id, id, false)
		switch {
		case err == nil:
			s.Registered = row.Manifest
			s.DraftChanged = !sameFiles(draft, row.FileMap)
		case errors.Is(err, ErrNotFound):
			s.DraftChanged = len(draft) > 0
		default:
			return nil, err
		}
	} else {
		s.DraftChanged = len(draft) > 0
	}
	return s, nil
}

func sameFiles(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// SetSafeMode switches every Site Code handler off (or back on).
func (e *Engine) SetSafeMode(on bool) error {
	v := ""
	if on {
		v = "1"
	}
	return e.st.SetOptions(map[string]string{OptSafeMode: v})
}

// ---------------------------------------------------------------- build / publish

// BuildOutcome is POST /build.
type BuildOutcome struct {
	OK         bool      `json:"ok"`
	Errors     []Problem `json:"errors"`
	Warnings   []Problem `json:"warnings"`
	Registered Manifest  `json:"registered"`
	// Migrations lists what publishing would run (dry run).
	Migrations *MigrationsDry `json:"migrations"`

	result *BuildResult
	prog   *Program
}

// BuildFiles bundles files and runs the registration.
func (e *Engine) BuildFiles(files map[string]string) (*BuildOutcome, error) {
	opts, err := e.st.Options()
	if err != nil {
		return nil, err
	}
	out := &BuildOutcome{Errors: []Problem{}, Warnings: []Problem{}}
	out.Registered.normalize()
	migs, mprobs := e.planMigrations(files)
	out.Migrations = migs
	var res *BuildResult
	set := e.units()
	if entry := EntryPoint(files); entry != "" {
		res = buildEntryWith(files, entry, "", e.importerFor(e.siteU, set))
	} else {
		res = Build(files)
	}
	out.Warnings = res.Warnings
	if !res.OK() {
		out.Errors = append(res.Errors, mprobs...)
		return out, nil
	}
	prog, probs := RegisterUnit(res.Bundle, e.reserved(opts), e.h, e.RouteTimeout, e.siteU)
	if len(probs) == 0 {
		probs = siteTypeConflicts(prog.Manifest, set.loaded())
	}
	if len(probs) == 0 && prog.hasVisitor {
		for _, lp := range set.loaded() {
			if lp.prog.hasVisitor {
				probs = append(probs, Problem{Message: "site.visitor มีได้ที่เดียว: ปลั๊กอิน " + lp.unit.ID + " ลงทะเบียน site.visitor ไว้แล้ว"})
			}
		}
	}
	if len(probs) > 0 || len(mprobs) > 0 {
		out.Errors = append(probs, mprobs...)
		return out, nil
	}
	out.OK, out.Registered, out.result, out.prog = true, prog.Manifest, res, prog
	return out, nil
}

// BuildDraft builds the current draft.
func (e *Engine) BuildDraft() (*BuildOutcome, map[string]string, error) {
	draft, err := e.DB.Draft()
	if err != nil {
		return nil, nil, err
	}
	out, err := e.BuildFiles(draft)
	return out, draft, err
}

// ErrConflict: the live version is not the one the client based its work on.
var ErrConflict = errors.New("conflict")

// BuildFailed carries the problems of a refused publish/restore.
type BuildFailed struct{ Errors []Problem }

func (b *BuildFailed) Error() string { return "build failed" }

// LiveVersion returns the live version id (0 = none).
func (e *Engine) LiveVersion() (int64, error) {
	opts, err := e.st.Options()
	if err != nil {
		return 0, err
	}
	return liveID(opts), nil
}

// Publish builds the draft, runs its pending migrations and makes it live
// as a new version. Destructive migrations need confirmDestructive.
func (e *Engine) Publish(message string, baseVersion *int64, userID int64, confirmDestructive bool) (*Version, error) {
	e.pubMu.Lock()
	defer e.pubMu.Unlock()
	live, err := e.LiveVersion()
	if err != nil {
		return nil, err
	}
	base := int64(0)
	if baseVersion != nil {
		base = *baseVersion
	}
	if base != live {
		return nil, ErrConflict
	}
	out, draft, err := e.BuildDraft()
	if err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, &BuildFailed{Errors: out.Errors}
	}
	if d := out.Migrations.HasDestructive(); len(d) > 0 && !confirmDestructive {
		return nil, &DestructiveRefused{List: d}
	}
	if strings.TrimSpace(message) == "" {
		message = "เผยแพร่"
	}
	user := ""
	if u, err := e.st.UserByID(userID); err == nil && u != nil {
		user = u.Name
	}
	if _, err := e.runMigrations(draft, user); err != nil {
		return nil, err
	}
	return e.goLive(message, draft, out.result, out.prog.Manifest, userID)
}

func (e *Engine) goLive(message string, files map[string]string, res *BuildResult, m Manifest, userID int64) (*Version, error) {
	id, err := e.DB.InsertVersion(message, files, res, m, userID)
	if err != nil {
		return nil, err
	}
	if err := e.st.SetOptions(map[string]string{OptVersion: strconv.FormatInt(id, 10)}); err != nil {
		return nil, err
	}
	e.breaker.Reset()
	e.lastErr.Store(nil)
	e.Log("info", "publish", fmt.Sprintf("เผยแพร่เวอร์ชัน #%d: %s", id, message), id)
	if err := e.data.RefreshViews(); err != nil {
		e.Log("error", "publish", "สร้าง view core_* ใหม่ไม่สำเร็จ: "+err.Error(), id)
	}
	if opts, err := e.st.Options(); err == nil {
		e.current(opts) // load now, not on the next visitor
	}
	row, err := e.DB.GetVersion(id, id, false)
	if err != nil {
		return nil, err
	}
	return &row.Version, nil
}

// Restore makes a snapshot live again as a new version ("Restore #id") and,
// with alsoDraft, replaces the draft with its files.
func (e *Engine) Restore(id int64, alsoDraft bool, userID int64) (*Version, error) {
	e.pubMu.Lock()
	defer e.pubMu.Unlock()
	live, err := e.LiveVersion()
	if err != nil {
		return nil, err
	}
	row, err := e.DB.GetVersion(id, live, true)
	if err != nil {
		return nil, err
	}
	opts, err := e.st.Options()
	if err != nil {
		return nil, err
	}
	prog, probs := RegisterUnit(row.Bundle, e.reserved(opts), e.h, e.RouteTimeout, e.siteU)
	if len(probs) == 0 {
		probs = siteTypeConflicts(prog.Manifest, e.units().loaded())
	}
	if len(probs) > 0 {
		return nil, &BuildFailed{Errors: probs}
	}
	v, err := e.goLive(fmt.Sprintf("Restore #%d", id), row.FileMap,
		&BuildResult{Bundle: row.Bundle, SourceMap: row.SourceMap}, prog.Manifest, userID)
	if err != nil {
		return nil, err
	}
	if alsoDraft {
		draft, err := e.DB.Draft()
		if err != nil {
			return nil, err
		}
		var remove []string
		for p := range draft {
			if _, ok := row.FileMap[p]; !ok {
				remove = append(remove, p)
			}
		}
		if err := e.DB.ApplyDraft(row.FileMap, remove, userID); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// ---------------------------------------------------------------- test runs

// RunRequest is POST /run.
type RunRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   map[string]any    `json:"query"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// RunResult is the outcome of a test run.
type RunResult struct {
	Status     int               `json:"status,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Cookies    []string          `json:"cookies,omitempty"` // Set-Cookie values
	Body       *string           `json:"body,omitempty"`
	BodyBase64 *string           `json:"bodyBase64,omitempty"` // a binary body (phase 6)
	Logs       []LogLine         `json:"logs"`
	DurationMs int64             `json:"durationMs"`
	Error      string            `json:"error,omitempty"`
	Trace      string            `json:"trace,omitempty"`
	Errors     []Problem         `json:"errors,omitempty"` // build problems
}

type nopWriter struct{ h http.Header }

func (n *nopWriter) Header() http.Header         { return n.h }
func (n *nopWriter) Write(b []byte) (int, error) { return len(b), nil }
func (n *nopWriter) WriteHeader(int)             {}

// Run executes a route of the DRAFT in the sandbox for the "Test route" panel.
func (e *Engine) Run(in RunRequest, user *User) (*RunResult, error) {
	out, _, err := e.BuildDraft()
	if err != nil {
		return nil, err
	}
	res := &RunResult{Logs: []LogLine{}}
	if !out.OK {
		res.Error, res.Errors = "build ไม่ผ่าน — แก้ข้อผิดพลาดก่อนทดสอบ", out.Errors
		return res, nil
	}
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !methods[method] || !strings.HasPrefix(in.Path, "/") {
		res.Error = "ระบุ method (GET, POST, PUT, PATCH, DELETE) และ path ที่ขึ้นต้นด้วย /"
		return res, nil
	}
	q := url.Values{}
	for k, v := range in.Query {
		switch x := v.(type) {
		case []any:
			for _, s := range x {
				q.Add(k, fmt.Sprint(s))
			}
		default:
			q.Set(k, fmt.Sprint(x))
		}
	}
	u := in.Path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest(method, "http://site.local"+u, strings.NewReader(in.Body))
	if err != nil {
		res.Error = "path ไม่ถูกต้อง"
		return res, nil
	}
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}
	matched, ps := -1, map[string]string{}
	mux, err := newMux(out.prog.specs, func(i int) http.Handler {
		return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			matched, ps = i, params(out.prog.specs[i], r)
		})
	})
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	if _, pattern := mux.Handler(req); pattern == "" {
		res.Error = fmt.Sprintf("ไม่พบ route ที่ตรงกับ %s %s", method, in.Path)
		return res, nil
	}
	mux.ServeHTTP(&nopWriter{h: http.Header{}}, req)
	if matched < 0 {
		res.Error = fmt.Sprintf("ไม่พบ route ที่ตรงกับ %s %s", method, in.Path)
		return res, nil
	}
	rs := out.prog.specs[matched]
	if !allowed(e.Effective(KindRoute, rs.Method+" "+rs.Path, out.prog.access[matched]), user) {
		body := `{"error":"คุณไม่มีสิทธิ์เข้าถึงหน้านี้"}`
		res.Status, res.Headers, res.Body = http.StatusForbidden, map[string]string{"Content-Type": "application/json; charset=utf-8"}, &body
		return res, nil
	}
	release, ok := e.acquire(e.QueueWait)
	if !ok {
		res.Error = "ระบบไม่ว่าง กรุณาลองใหม่อีกครั้ง"
		return res, nil
	}
	defer release()
	start := time.Now()
	v, err := e.newLoaded(out.prog)
	var r *Response
	var logs []LogLine
	if err == nil {
		r, logs, err = e.execRoute(v, rs.Pattern, req, ps, user)
	}
	res.DurationMs = time.Since(start).Milliseconds()
	if logs != nil {
		res.Logs = logs
	}
	if err != nil {
		re := toRunError(err)
		res.Error, res.Trace = re.Error(), re.Trace
		return res, nil
	}
	res.Status, res.Headers, res.Body, res.Cookies = r.Status, r.Headers, &r.Body, r.Cookies
	if r.Binary {
		b64 := base64.StdEncoding.EncodeToString([]byte(r.Body))
		res.Body, res.BodyBase64 = nil, &b64
	}
	return res, nil
}

// ---------------------------------------------------------------- helpers for the API

// UserFromModel converts a CMS user.
func UserFromModel(u *model.User) *User {
	if u == nil {
		return nil
	}
	return &User{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role}
}

// IsNotFound reports a missing file/version (or store row).
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, store.ErrNotFound) || errors.Is(err, sql.ErrNoRows)
}

// ---------------------------------------------------------------- phase 4a helpers

// emitCore fires a core event (post.saved, …) from code: through the plugin
// manager when wired (plugins see it too), else to Site Code only.
func (e *Engine) emitCore(event string, payload map[string]any) {
	if e.cfg.Emit != nil {
		e.cfg.Emit(event, payload)
		return
	}
	e.OnEvent(event, payload)
}

// emitCustom delivers site.emit(name, data) to site.on(name) handlers of
// every unit and to the service / WASM plugins subscribed to it.
func (e *Engine) emitCustom(name string, data any) {
	e.dispatch(name, data)
	if e.cfg.Plugins != nil {
		payload, ok := data.(map[string]any)
		if !ok {
			payload = map[string]any{"value": data}
		}
		e.cfg.Plugins.DeliverEvent(name, payload)
	}
}

// systemAuthor is the author of posts created with db.asSystem: the first
// administrator.
func (e *Engine) systemAuthor() int64 {
	if e == nil || e.st == nil {
		return 0
	}
	var id int64
	_ = e.st.DB.QueryRow(`SELECT id FROM users WHERE role = 'administrator' ORDER BY id LIMIT 1`).Scan(&id)
	return id
}

// Option keys of the Site Code limits (Settings screen of Site Code).
const (
	OptMailPerHour    = "site_code_mail_per_hour"    // mail.send per hour (100)
	OptRoutePerMinute = "site_code_route_per_minute" // public route calls per IP per minute (120)
	OptFetchHosts     = "site_code_fetch_hosts"      // JSON array of http.fetch hosts
)

// Limits are the adjustable Site Code limits.
type Limits struct {
	MailPerHour    int `json:"mailPerHour"`
	RoutePerMinute int `json:"routePerMinute"`
}

// LimitsFrom reads the limits from options (defaults 100 / 120).
func LimitsFrom(opts map[string]string) Limits {
	l := Limits{MailPerHour: 100, RoutePerMinute: 120}
	if n, err := strconv.Atoi(opts[OptMailPerHour]); err == nil && n >= 0 {
		l.MailPerHour = n
	}
	if n, err := strconv.Atoi(opts[OptRoutePerMinute]); err == nil && n > 0 {
		l.RoutePerMinute = n
	}
	return l
}

func (e *Engine) applyLimits(opts map[string]string) {
	l := LimitsFrom(opts)
	e.mailLimit.SetLimit(l.MailPerHour)
	e.routeLimit.SetLimit(l.RoutePerMinute)
}

// liveProgram returns the loaded live program (nil when none, or Site Code
// is off).
func (e *Engine) liveProgram() (*liveProg, map[string]string) {
	opts, err := e.st.Options()
	if err != nil || opts[OptVersion] == "" || e.siteOff(opts) {
		return nil, opts
	}
	lp := e.current(opts)
	if lp == nil || lp.prog == nil {
		return nil, opts
	}
	return lp, opts
}

// ManifestFor returns the live version's manifest under opts (cached per
// version; declarative data such as post types stays available in safe
// mode). Empty without a live version or in demo mode.
func (e *Engine) ManifestFor(opts map[string]string) Manifest {
	var m Manifest
	m.normalize()
	id := liveID(opts)
	if id == 0 || e.cfg.Demo {
		e.pushTypeSlugs(0, m, opts[OptContentTypes])
		return m
	}
	if c, ok := e.manifests.Load(id); ok {
		m = c.(Manifest)
	} else {
		row, err := e.DB.GetVersion(id, id, false)
		if err != nil {
			return m
		}
		e.manifests.Store(id, row.Manifest)
		m = row.Manifest
	}
	e.pushTypeSlugs(id, m, opts[OptContentTypes])
	return m
}

// pushTypeSlugs hands the URL prefixes of the custom post types (Site
// Code's live types and the loaded plugins' types) to the site (PostURL)
// when the live version or the set of plugin units changed. It reads the
// current unit set without reloading it (ManifestFor runs while the units
// load).
func (e *Engine) pushTypeSlugs(id int64, m Manifest, adminTypes string) {
	if e.cfg.TypeSlugs == nil {
		return
	}
	set := e.unitSetP.Load()
	key := strconv.FormatInt(id, 10) + "|" + adminTypes // 2.2.2: the admin-defined types are part of the set
	if set != nil {
		key += "|" + set.fp
	}
	if old := e.typeSlugsKey.Swap(&key); old != nil && *old == key {
		return
	}
	slugs := map[string]string{}
	for _, t := range withAdminTypes(withUnits(m, set.loaded()), adminTypes).PostTypes {
		slugs[t.Type] = t.Slug
	}
	e.cfg.TypeSlugs(slugs)
}

// PostTypes returns the custom post types of the live version and the
// running JS plugins, with every custom field (site.fields).
func (e *Engine) PostTypes() Manifest {
	opts, err := e.st.Options()
	if err != nil {
		var m Manifest
		m.normalize()
		return m
	}
	return e.ContentFor(opts)
}

// ContentFor is ManifestFor(opts) with the post types and custom fields of
// the running JS plugins (WithUnitFields) and, since 2.2.2, the content types
// made in the admin (option content_types; code and plugin types win a clash).
func (e *Engine) ContentFor(opts map[string]string) Manifest {
	m := e.ManifestFor(opts)
	return expandAllTypes(withAdminTypes(withUnits(m, e.units().loaded()), opts[OptContentTypes]))
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }
