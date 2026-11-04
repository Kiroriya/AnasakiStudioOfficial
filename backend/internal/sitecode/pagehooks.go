package sitecode

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"golang.org/x/net/html"

	"github.com/anasaki/studio/backend/internal/kses"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// Phase 6 (G7): the joints the built-in plugins need to move out of core.
//
//   - site.middleware(fn, {priority}) runs before routing for public requests
//     (scope "middleware"); fn(ctx) → undefined (continue) | RouteResult
//     (respond now). Strict time budget (MiddlewareTimeout, 20 ms) per
//     middleware; failures count against the circuit breaker, an open breaker
//     skips the middleware.
//   - site.afterResponse(fn) is fire-and-forget after the page was sent
//     (scope "middleware"): a queue + one worker, never delays a response.
//   - site.head(fn(page)) / site.bodyEnd(fn(page)) add HTML to every public
//     page; the output is sanitised unless the unit has scope "head-scripts"
//     (Site Code is trusted).

// regHook is a site.middleware / afterResponse / head / bodyEnd handler.
type regHook struct {
	fn   goja.Callable
	prio int
	at   Problem
}

const maxPageHooks = 20

// Page hook limits (Engine fields override them).
const (
	defaultMiddlewareTimeout = 20 * time.Millisecond
	defaultAfterTimeout      = 2 * time.Second // 500 ms before 2.2.0: too short for a first hit on a slow database
	afterQueueSize           = 1000
)

func (v *vm) regPageHook(name, scope string, list *[]*regHook, c goja.FunctionCall, withOpts bool) {
	if v.mode == modeHandler {
		v.throw("site.%s ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler", name)
	}
	if scope != "" {
		v.need(scope, "site."+name)
	}
	fn, ok := goja.AssertFunction(c.Argument(0))
	if !ok {
		v.throw("site.%s: ต้องส่งฟังก์ชัน", name)
	}
	if len(*list) >= maxPageHooks {
		v.throw("ลงทะเบียน site.%s ได้ไม่เกิน %d รายการ", name, maxPageHooks)
	}
	prio := defaultPriority
	if withOpts {
		if o := optObject(v.rt, c.Argument(1)); o != nil {
			if p := o.Get("priority"); p != nil && !goja.IsUndefined(p) && !goja.IsNull(p) {
				f := p.ToFloat()
				if f != float64(int(f)) || f < -10000 || f > 10000 {
					v.throw("site.%s: priority ต้องเป็นจำนวนเต็ม -10000 ถึง 10000", name)
				}
				prio = int(f)
			}
		}
	}
	*list = append(*list, &regHook{fn: fn, prio: prio, at: v.caller()})
}

func (v *vm) siteMiddleware(c goja.FunctionCall) goja.Value {
	v.regPageHook("middleware", "middleware", &v.mws, c, true)
	return goja.Undefined()
}

func (v *vm) siteAfterResponse(c goja.FunctionCall) goja.Value {
	v.regPageHook("afterResponse", "middleware", &v.afters, c, false)
	return goja.Undefined()
}

func (v *vm) siteHead(c goja.FunctionCall) goja.Value {
	v.regPageHook("head", "", &v.heads, c, false)
	return goja.Undefined()
}

func (v *vm) siteBodyEnd(c goja.FunctionCall) goja.Value {
	v.regPageHook("bodyEnd", "", &v.bodyEnds, c, false)
	return goja.Undefined()
}

func (p *Program) registerPageHooks(v *vm) {
	for _, m := range v.mws {
		p.middlewares = append(p.middlewares, filterMeta{Name: "middleware", Priority: m.prio, At: m.at})
	}
	p.afterN, p.headN, p.bodyEndN = len(v.afters), len(v.heads), len(v.bodyEnds)
}

// ---------------------------------------------------------------- middleware

type mwRef struct {
	unit *unitInfo
	idx  int
	prio int
}

// middlewareChain is every unit's middleware: ascending priority, then unit
// load order (plugins, Site Code last), then registration order.
func middlewareChain(set *unitSet, site *liveProg) []mwRef {
	var out []mwRef
	add := func(lp *liveProg) {
		if lp == nil || lp.prog == nil {
			return
		}
		for i, m := range lp.prog.middlewares {
			out = append(out, mwRef{unit: lp.unit, idx: i, prio: m.Priority})
		}
	}
	for _, lp := range set.loaded() {
		add(lp)
	}
	add(site)
	sort.SliceStable(out, func(i, j int) bool { return out[i].prio < out[j].prio })
	return out
}

func (e *Engine) middlewareTimeout() time.Duration {
	if e.MiddlewareTimeout > 0 {
		return e.MiddlewareTimeout
	}
	return defaultMiddlewareTimeout
}

// Middleware runs the site.middleware chain for a public request; true when
// a middleware answered (the response is written).
func (e *Engine) Middleware(w http.ResponseWriter, r *http.Request, opts map[string]string) bool {
	if e.pluginsOff(opts) || r.URL.Path == "/favicon.ico" || r.URL.Path == "/favicon.svg" {
		return false
	}
	set := e.units()
	var site *liveProg
	if opts[OptVersion] != "" && !e.siteOff(opts) {
		if lp := e.current(opts); lp != nil && lp.prog != nil {
			site = lp
		}
	}
	chain := middlewareChain(set, site)
	if len(chain) == 0 {
		return false
	}
	release, ok := e.acquire(100 * time.Millisecond)
	if !ok {
		e.logUnit(SiteUnit, "warn", "middleware", "ระบบไม่ว่าง: ข้าม site.middleware ของคำขอนี้")
		return false
	}
	defer release()
	user := e.userFrom(r)
	s := &Session{e: e, r: r, user: user, held: true, set: set, setOK: true, vis: &visitorState{}}
	if site != nil {
		s.prog, s.lp, s.version = site.prog, site, site.id
	} else {
		s.prog = e.emptyProgram()
	}
	defer s.Close()
	limit := e.middlewareTimeout()
	for _, m := range chain {
		u := m.unit
		key := u.src(fmt.Sprintf("middleware #%d", m.idx+1))
		if !e.breaker.Allow(key) {
			continue // open breaker: skipped (logged when it opened)
		}
		v, err := s.unitVM(u.id())
		if err != nil || m.idx >= len(v.mws) {
			continue
		}
		v.rt.ClearInterrupt()
		v.user, v.logs, v.cookies, v.source = user, nil, nil, key
		v.req, v.bodyReq, v.body, v.rawBody = r, r, nil, false // the body stays for the route
		ctx := v.middlewareCtx(r, user)
		val, err := v.call(v.mws[m.idx].fn, limit, ctx)
		var res *Response
		if err == nil && val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) && continueHeaders(w, v, val) {
			val = nil // {headers} only: continue with these headers (phase 7)
		}
		if err == nil && val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
			_, err = v.guarded(limit+50*time.Millisecond, func() (goja.Value, error) {
				var rerr error
				res, rerr = v.response(val)
				return nil, rerr
			})
			if err != nil {
				err = v.runErr(err)
			}
		}
		s.logs = append(s.logs, unitEntries(logEntries(v.logs, key), u)...)
		if err != nil {
			re := toRunError(err)
			en := unitEntry(errorEntry(key, re), u)
			en.Level, en.Message = "warn", "middleware ถูกข้าม: "+re.Error()
			s.logs = append(s.logs, en)
			e.failedIn(u, key, re, s.version)
			if re.Timeout {
				s.reset(u.id())
			}
			continue
		}
		if res == nil {
			continue
		}
		res.Cookies = v.cookies
		if _, ok := res.Headers["Cache-Control"]; !ok {
			res.Headers["Cache-Control"] = "no-store"
		}
		e.writeResponse(w, r, res)
		return true
	}
	return false
}

// middlewareCtx is the ctx of a middleware: request (without the body),
// visitor, user / isAdminUser and response (setCookie).
func (v *vm) middlewareCtx(r *http.Request, user *User) goja.Value {
	ctx := v.rt.NewObject()
	req := v.requestValueP(r, map[string]string{}).ToObject(v.rt)
	_ = req.Set("ip", plugin.ClientIP(r))
	_ = req.Set("userAgent", r.UserAgent())
	_ = req.Set("referrer", r.Referer())
	_ = req.Set("url", r.URL.RequestURI())
	_ = req.Set("host", plugin.RequestHost(r))
	_ = ctx.Set("request", req)
	_ = ctx.Set("response", v.responseValue(r))
	_ = ctx.Set("user", v.userValue())
	_ = ctx.Set("isAdminUser", user != nil)
	v.defineVisitor(ctx)
	return ctx
}

// ---------------------------------------------------------------- afterResponse

// ResponseInfo is what site.afterResponse receives.
type ResponseInfo struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Query       string `json:"query"`
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	Referrer    string `json:"referrer"`
	UserAgent   string `json:"userAgent"`
	IP          string `json:"ip"`
	DurationMs  int64  `json:"durationMs"`
	Title       string `json:"title"` // the page title when the site rendered a page
	IsAdminUser bool   `json:"isAdminUser"`
	At          string `json:"at"` // RFC 3339 UTC
	// phase 7: the request's host and a few request headers (lower-case
	// names: purpose, sec-purpose, x-moz, x-purpose, accept-language, dnt,
	// sec-fetch-dest, sec-fetch-mode)
	Host    string            `json:"host"`
	Headers map[string]string `json:"headers"`
}

// afterHeaders are the request headers ResponseInfo.headers carries.
var afterHeaders = []string{"purpose", "sec-purpose", "x-moz", "x-purpose", "accept-language", "dnt", "sec-fetch-dest", "sec-fetch-mode"}

// continueHeaders reports a middleware result {headers: {...}} (no other
// keys): its allow-listed headers are set on the response and the request
// continues.
func continueHeaders(w http.ResponseWriter, v *vm, val goja.Value) bool {
	o, ok := val.(*goja.Object)
	if !ok || o.ClassName() != "Object" {
		return false
	}
	keys := o.Keys()
	if len(keys) != 1 || keys[0] != "headers" {
		return false
	}
	h := optObject(v.rt, o.Get("headers"))
	if h == nil {
		return true
	}
	for _, k := range h.Keys() {
		name := http.CanonicalHeaderKey(strings.TrimSpace(k))
		hv := h.Get(k).String()
		if !continueHeaderOK[name] || strings.ContainsAny(hv, "\r\n") || len(hv) > 2000 {
			continue
		}
		if name == "Vary" {
			w.Header().Add(name, hv)
		} else {
			w.Header().Set(name, hv)
		}
	}
	return true
}

// continueHeaderOK are the headers a middleware may add to a request it lets
// through (the page handler may still set its own Cache-Control).
var continueHeaderOK = map[string]bool{"Cache-Control": true, "Vary": true, "X-Robots-Tag": true, "Content-Language": true,
	"Link": true, "Referrer-Policy": true, "X-Frame-Options": true, "X-Content-Type-Options": true, "Permissions-Policy": true}

type afterQueue struct {
	once    sync.Once
	ch      chan ResponseInfo
	pending sync.WaitGroup
	dropped atomic.Int64
}

func (e *Engine) afterTimeout() time.Duration {
	if e.AfterTimeout > 0 {
		return e.AfterTimeout
	}
	return defaultAfterTimeout
}

// hasAfter reports whether a running unit registered site.afterResponse.
func (e *Engine) hasAfter(opts map[string]string) bool {
	for _, lp := range e.units().loaded() {
		if lp.prog.afterN > 0 {
			return true
		}
	}
	if opts[OptVersion] != "" && !e.siteOff(opts) {
		if lp := e.current(opts); lp != nil && lp.prog != nil && lp.prog.afterN > 0 {
			return true
		}
	}
	return false
}

// AfterResponse queues the site.afterResponse handlers for a request that
// was answered (never blocks; a full queue drops the item).
func (e *Engine) AfterResponse(r *http.Request, status int, ctype string, dur time.Duration, title string, opts map[string]string) {
	if e.pluginsOff(opts) || !e.hasAfter(opts) {
		return
	}
	info := ResponseInfo{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Status: status, ContentType: ctype,
		Referrer: r.Referer(), UserAgent: r.UserAgent(), IP: plugin.ClientIP(r), DurationMs: dur.Milliseconds(), Title: title,
		IsAdminUser: strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && e.userFrom(r) != nil,
		At:          e.Now().UTC().Format(time.RFC3339), Host: plugin.RequestHost(r), Headers: map[string]string{}}
	for _, h := range afterHeaders {
		if x := r.Header.Get(h); x != "" {
			info.Headers[h] = clip(x, 200)
		}
	}
	q := &e.after
	q.once.Do(func() {
		q.ch = make(chan ResponseInfo, afterQueueSize)
		go e.afterWorker()
	})
	q.pending.Add(1)
	select {
	case q.ch <- info:
	default:
		q.pending.Done()
		if q.dropped.Add(1)%100 == 1 {
			e.logUnit(SiteUnit, "warn", "afterResponse", "คิว site.afterResponse เต็ม — ข้ามบางคำขอ")
		}
	}
}

// WaitAfterResponse blocks until the queued afterResponse items ran (tests).
func (e *Engine) WaitAfterResponse() { e.after.pending.Wait() }

func (e *Engine) afterWorker() {
	for info := range e.after.ch {
		func() {
			defer e.after.pending.Done()
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("site code: afterResponse: panic: %v", rec)
				}
			}()
			e.runAfter(info)
		}()
	}
}

func (e *Engine) runAfter(info ResponseInfo) {
	opts, err := e.st.Options()
	if err != nil || e.pluginsOff(opts) {
		return
	}
	var units []*liveProg
	for _, lp := range e.units().loaded() {
		if lp.prog.afterN > 0 {
			units = append(units, lp)
		}
	}
	if opts[OptVersion] != "" && !e.siteOff(opts) {
		if lp := e.current(opts); lp != nil && lp.prog != nil && lp.prog.afterN > 0 {
			units = append(units, lp)
		}
	}
	if len(units) == 0 {
		return
	}
	release, ok := e.acquire(2 * time.Second)
	if !ok {
		return
	}
	defer release()
	limit := e.afterTimeout()
	for _, lp := range units {
		u := lp.unit
		var v *vm
		for idx := 0; idx < lp.prog.afterN; idx++ {
			key := u.src(fmt.Sprintf("afterResponse #%d", idx+1))
			if !e.breaker.Allow(key) {
				continue
			}
			if v == nil {
				if v, err = e.liveVM(lp); err != nil {
					e.persist([]LogEntry{unitEntry(errorEntry(key, toRunError(err)), u)}, lp.id)
					break
				}
			}
			if idx >= len(v.afters) {
				break
			}
			v.rt.ClearInterrupt()
			v.user, v.logs, v.source = nil, nil, key
			v.vis = &visitorState{done: true}
			sess := e.attachSession(v, nil, nil)
			_, err := v.call(v.afters[idx].fn, limit, v.toJS(info))
			sess.Close()
			entries := unitEntries(logEntries(v.logs, key), u)
			if err != nil {
				re := toRunError(err)
				entries = append(entries, unitEntry(errorEntry(key, re), u))
				e.failedIn(u, key, re, lp.id)
				if re.Timeout {
					v = nil // interrupted: a fresh VM for the next handler
				}
			}
			e.persist(entries, lp.id)
		}
	}
}

// ---------------------------------------------------------------- head / bodyEnd

// HasPageHTML reports a site.head ("head") / site.bodyEnd ("bodyEnd")
// handler in any unit of the session.
func (s *Session) HasPageHTML(kind string) bool {
	n := func(p *Program) int {
		if p == nil {
			return 0
		}
		if kind == "head" {
			return p.headN
		}
		return p.bodyEndN
	}
	if n(s.sp()) > 0 {
		return true
	}
	for _, lp := range s.plugins().loaded() {
		if n(lp.prog) > 0 {
			return true
		}
	}
	return false
}

// PageHTML runs every unit's site.head / site.bodyEnd handlers for page
// (plugins in load order, then Site Code) and joins their HTML: sanitised
// unless the unit has scope "head-scripts". A handler that fails or runs
// out of time (the render limit, 50 ms) adds nothing.
func (s *Session) PageHTML(kind string, page map[string]any) string {
	if !s.HasPageHTML(kind) {
		return ""
	}
	units := []string{}
	for _, lp := range s.plugins().loaded() {
		units = append(units, lp.unit.ID)
	}
	units = append(units, SiteUnit)
	var b strings.Builder
	for _, unit := range units {
		p := s.progOf(unit)
		if p == nil {
			continue
		}
		n := p.headN
		if kind == "bodyEnd" {
			n = p.bodyEndN
		}
		trusted := unit == SiteUnit
		if lp := s.plugins().get(unit); lp != nil && unit != SiteUnit {
			trusted = lp.unit.can("head-scripts")
		}
		for i := 0; i < n; i++ {
			idx := i
			out, err := s.callIn(unit, fmt.Sprintf("%s #%d", kind, idx+1), s.e.FilterRenderTimeout, func(v *vm) goja.Callable {
				list := v.heads
				if kind == "bodyEnd" {
					list = v.bodyEnds
				}
				if idx < len(list) {
					return list[idx].fn
				}
				return nil
			}, func(v *vm) []goja.Value { return []goja.Value{v.toJS(page)} })
			str, _ := out.(string)
			if err != nil || str == "" {
				continue
			}
			if !trusted {
				if kind == "head" {
					str = SanitizeHead(str)
				} else {
					str = kses.SanitizeHTML(str)
				}
			}
			b.WriteString(str)
		}
	}
	return b.String()
}

// SanitizeHead keeps what search and social tags need from untrusted <head>
// HTML: <meta name|property|content|itemprop>, <link> with a safe href, and
// <script type="application/ld+json"> (its text re-escaped). Everything else
// is dropped.
func SanitizeHead(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	inLD := false
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return b.String()
		case html.TextToken:
			if inLD {
				t := strings.NewReplacer("<", `\u003c`, ">", `\u003e`).Replace(string(z.Text()))
				b.WriteString(t)
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "script" && inLD {
				b.WriteString("</script>")
				inLD = false
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			attrs := map[string]string{}
			for _, a := range tok.Attr {
				attrs[strings.ToLower(a.Key)] = a.Val
			}
			switch tok.Data {
			case "meta":
				b.WriteString("<meta")
				for _, k := range []string{"name", "property", "itemprop", "content"} {
					if v, ok := attrs[k]; ok {
						b.WriteString(" " + k + `="` + html.EscapeString(v) + `"`)
					}
				}
				b.WriteString(">")
			case "link":
				rel := strings.ToLower(attrs["rel"])
				href := attrs["href"]
				if rel == "" || strings.Contains(rel, "import") || strings.Contains(rel, "stylesheet") || strings.Contains(rel, "preload") ||
					strings.Contains(rel, "modulepreload") || !safeHeadURL(href) {
					continue
				}
				b.WriteString("<link")
				for _, k := range []string{"rel", "href", "hreflang", "type", "sizes", "media", "title"} {
					if v, ok := attrs[k]; ok {
						b.WriteString(" " + k + `="` + html.EscapeString(v) + `"`)
					}
				}
				b.WriteString(">")
			case "script":
				if strings.EqualFold(strings.TrimSpace(attrs["type"]), "application/ld+json") && tt == html.StartTagToken {
					b.WriteString(`<script type="application/ld+json">`)
					inLD = true
				}
			}
		}
	}
}

func safeHeadURL(u string) bool {
	u = strings.TrimSpace(u)
	if u == "" {
		return false
	}
	if strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") {
		return true
	}
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "https" || p.Scheme == "http") && p.Host != ""
}
