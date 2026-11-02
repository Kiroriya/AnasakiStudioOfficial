package sitecode

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/media"
	"github.com/anasaki/studio/backend/internal/model"
)

// Phase 4a: filters (site.filter / site.applyFilter), custom events
// (site.emit) and the visitor identity hook (site.visitor).

// Core filter points and their time limits.
var coreFilters = map[string]bool{
	"post.content": true, "post.beforeSave": true, "page.head": true, "collection.items": true,
	"action.input": true, "action.result": true, "mail.beforeSend": true,
	// phase 6: head meta of every page, and /sitemap.xml + /robots.txt (served by core when filtered)
	"page.meta": true, "sitemap.urls": true, "robots.txt": true,
	// 2.1.0: before a file joins the media library (rename, alt text, refuse)
	"media.beforeSave": true,
}

// renderFilters run while a page renders: 50 ms per filter.
var renderFilters = map[string]bool{"post.content": true, "page.head": true, "collection.items": true, "page.meta": true}

var (
	customHookRe = regexp.MustCompile(`^site\.[a-zA-Z0-9][a-zA-Z0-9._:-]{0,90}$`)
	// anyHookRe is a hook of any unit: "site.*" or "<plugin id>.*".
	anyHookRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,39}\.[a-zA-Z0-9][a-zA-Z0-9._:-]{0,90}$`)
	groupNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
)

const (
	maxFilters      = 200
	visitorTimeout  = 200 * time.Millisecond
	defaultPriority = 10
)

// FilterInfo is a registered site.filter (manifest).
type FilterInfo struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

type regFilter struct {
	info FilterInfo
	fn   goja.Callable
	at   Problem
}

type regVisitor struct {
	fn goja.Callable
	at Problem
}

// visitorState caches the visitor of one request (resolved at most once).
type visitorState struct {
	done bool
	val  map[string]any
}

// ---------------------------------------------------------------- registration

func (v *vm) siteFilter(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.filter ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	name, _ := c.Argument(0).Export().(string)
	if !coreFilters[name] && !customHookRe.MatchString(name) && !anyHookRe.MatchString(name) {
		v.throw("site.filter: ไม่มีจุด filter %q (ใช้ post.content, post.beforeSave, page.head, page.meta, sitemap.urls, robots.txt, collection.items, action.input, action.result, mail.beforeSend หรือชื่อของคุณเองที่ขึ้นต้นด้วย 'site.' / '<id ปลั๊กอิน>.')", c.Argument(0).String())
	}
	if coreFilters[name] {
		v.need("filters-core", "site.filter('"+name+"')")
	}
	fn, ok := goja.AssertFunction(c.Argument(1))
	if !ok {
		v.throw("site.filter(%q): ต้องมีฟังก์ชัน (value, ctx) => value", name)
	}
	if len(v.filters) >= maxFilters {
		v.throw("ลงทะเบียน site.filter ได้ไม่เกิน %d รายการ", maxFilters)
	}
	prio := defaultPriority
	if o := optObject(v.rt, c.Argument(2)); o != nil {
		if p := o.Get("priority"); p != nil && !goja.IsUndefined(p) && !goja.IsNull(p) {
			f := p.ToFloat()
			if f != float64(int(f)) || f < -10000 || f > 10000 {
				v.throw("site.filter(%q): priority ต้องเป็นจำนวนเต็ม -10000 ถึง 10000", name)
			}
			prio = int(f)
		}
	}
	v.filters = append(v.filters, &regFilter{info: FilterInfo{Name: name, Priority: prio}, fn: fn, at: v.caller()})
	return goja.Undefined()
}

func (v *vm) siteVisitor(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.visitor ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	v.need("visitor", "site.visitor")
	fn, ok := goja.AssertFunction(c.Argument(0))
	if !ok {
		v.throw("site.visitor: ต้องมีฟังก์ชัน ({ request }) => ({ id, name, groups }) | null")
	}
	v.visitors = append(v.visitors, &regVisitor{fn: fn, at: v.caller()})
	return goja.Undefined()
}

func (v *vm) siteVisitorGroups(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.visitorGroups ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	var list []string
	if err := v.rt.ExportTo(c.Argument(0), &list); err != nil {
		v.throw("site.visitorGroups: ต้องเป็น array ของชื่อกลุ่ม เช่น ['member', 'gold']")
	}
	for _, g := range list {
		if !groupNameRe.MatchString(g) {
			v.throw("site.visitorGroups: ชื่อกลุ่ม %q ไม่ถูกต้อง (a-z ตัวแรก ตามด้วย a-z 0-9 _ - ไม่เกิน 32 ตัว)", g)
		}
		dup := false
		for _, x := range v.vgroups {
			dup = dup || x == g
		}
		if !dup {
			v.vgroups = append(v.vgroups, g)
		}
	}
	if len(v.vgroups) > 50 {
		v.throw("site.visitorGroups: ประกาศได้ไม่เกิน 50 กลุ่ม")
	}
	return goja.Undefined()
}

// registerHooks validates filters and the visitor hook of a registration run.
func (p *Program) registerHooks(v *vm) []Problem {
	var probs []Problem
	p.filterNames = map[string]bool{}
	for _, f := range v.filters {
		p.filterNames[f.info.Name] = true
		p.Manifest.Filters = append(p.Manifest.Filters, f.info)
		p.filterList = append(p.filterList, filterMeta{Name: f.info.Name, Priority: f.info.Priority, At: f.at})
	}
	if len(v.visitors) > 1 {
		first, second := v.visitors[0].at, v.visitors[1].at
		probs = append(probs, at(second, fmt.Sprintf("site.visitor ลงทะเบียนได้ครั้งเดียว (มีแล้วที่ %s:%d)", first.File, first.Line)))
	}
	p.hasVisitor = len(v.visitors) > 0
	p.Manifest.Visitor = p.hasVisitor
	p.Manifest.VisitorGroups = append([]string{}, v.vgroups...)
	return probs
}

// HasFilter reports whether the program registered a filter under name.
func (p *Program) HasFilter(name string) bool { return p != nil && p.filterNames[name] }

// filtersFor returns the filters of name in run order: ascending priority,
// then registration order.
func (v *vm) filtersFor(name string) []*regFilter {
	var out []*regFilter
	for _, f := range v.filters {
		if f.info.Name == name {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].info.Priority < out[j].info.Priority })
	return out
}

func filterTimeout(e *Engine, name string) time.Duration {
	if renderFilters[name] {
		if e != nil && e.FilterRenderTimeout > 0 {
			return e.FilterRenderTimeout
		}
		return 50 * time.Millisecond
	}
	if e != nil && e.FilterTimeout > 0 {
		return e.FilterTimeout
	}
	return 2 * time.Second
}

// ---------------------------------------------------------------- running filters inside a handler

// isInterrupt reports a vm.Interrupt (handler timeout) error.
func isInterrupt(err error) bool {
	var ie *goja.InterruptedError
	return errors.As(err, &ie)
}

// applyNested runs the filters of name on value inside the running handler
// (site.applyFilter, db.posts writes, mail.send). A filter that throws is
// skipped (logged with its file:line); its time counts against the handler.
func (v *vm) applyNested(name string, value goja.Value, ctx goja.Value) goja.Value {
	if v.sess != nil && v.sess.multiUnit(name, v.u.id()) {
		own := v.filtersFor(name)
		for _, h := range filterChain(v.sess.plugins(), v.sess.sp(), name) {
			if h.unit != v.u.id() {
				value = v.crossFilter(h, name, value, ctx)
				continue
			}
			if h.idx < len(own) {
				var stop bool
				value, stop = v.localFilter(own[h.idx], name, value, ctx)
				if stop {
					return value
				}
			}
		}
		return value
	}
	for _, f := range v.filtersFor(name) {
		var stop bool
		value, stop = v.localFilter(f, name, value, ctx)
		if stop {
			return value
		}
	}
	return value
}

// localFilter runs one filter of the VM's own unit (stop: the handler ran
// out of time).
func (v *vm) localFilter(f *regFilter, name string, value, ctx goja.Value) (goja.Value, bool) {
	{
		out, err := f.fn(goja.Undefined(), value, ctx)
		if err != nil {
			if isInterrupt(err) {
				return value, true // the handler's own time limit: unwinds right after
			}
			re := v.runErr(err)
			v.addLogLine("warn", fmt.Sprintf("filter %s (%s:%d) ถูกข้าม: %s", name, f.at.File, f.at.Line, re.Error()))
			return value, false
		}
		if out == nil || goja.IsUndefined(out) {
			return value, false // mutated in place (or nothing returned): keep the argument
		}
		return out, false
	}
}

func (v *vm) addLogLine(level, msg string) {
	if len(v.logs) < maxLogLines {
		v.logs = append(v.logs, LogLine{Level: level, Message: clip(msg, maxLogMessage)})
	}
}

func (v *vm) siteApplyFilter(c goja.FunctionCall) goja.Value {
	v.handlerOnly("site.applyFilter")
	name, _ := c.Argument(0).Export().(string)
	if !anyHookRe.MatchString(name) || !strings.HasPrefix(name, v.u.hookPrefix()) {
		v.throw("site.applyFilter: ชื่อต้องขึ้นต้นด้วย '%s' (จุด filter ของระบบหรือของหน่วยโค้ดอื่นเรียกเองไม่ได้)", v.u.hookPrefix())
	}
	if v.u.plugin() && !v.u.declared[name] {
		v.addLogLine("warn", fmt.Sprintf("site.applyFilter(%q): ไม่ได้ประกาศใน code.hooks.filters ของ manifest", name))
	}
	ctx := c.Argument(2)
	if goja.IsUndefined(ctx) || goja.IsNull(ctx) {
		ctx = v.rt.NewObject()
	}
	return v.applyNested(name, c.Argument(1), ctx)
}

func (v *vm) siteEmit(c goja.FunctionCall) goja.Value {
	v.handlerOnly("site.emit")
	name, _ := c.Argument(0).Export().(string)
	if !anyHookRe.MatchString(name) || !strings.HasPrefix(name, v.u.hookPrefix()) {
		v.throw("site.emit: ชื่อเหตุการณ์ต้องขึ้นต้นด้วย '%s' เช่น '%sorderPaid'", v.u.hookPrefix(), v.u.hookPrefix())
	}
	if v.u.plugin() && !v.u.declared[name] {
		v.addLogLine("warn", fmt.Sprintf("site.emit(%q): ไม่ได้ประกาศใน code.hooks.events ของ manifest", name))
	}
	var data any
	if x := c.Argument(1); !goja.IsUndefined(x) {
		s, ok, err := v.toJSON(x)
		if err != nil || (!ok && !goja.IsUndefined(x)) {
			v.throw("site.emit: data ต้องแปลงเป็น JSON ได้")
		}
		if ok {
			if err := jsonUnmarshal(s, &data); err != nil {
				v.throw("site.emit: data ต้องแปลงเป็น JSON ได้")
			}
		}
	}
	if v.h != nil && v.h.e != nil {
		v.h.e.emitCustom(name, data)
		v.emitAlias(name, data)
	}
	return goja.Undefined()
}

// ---------------------------------------------------------------- filters from Go (session)

// filterCtx builds the ctx object of a filter: the given values plus user
// and a lazy visitor.
func (v *vm) filterCtx(ctx map[string]any) goja.Value {
	o := v.toJS(ctx).ToObject(v.rt)
	if _, ok := ctx["user"]; !ok {
		_ = o.Set("user", v.userValue())
	}
	v.defineVisitor(o)
	return o
}

// HasFilter reports a filter registered under name in any unit of the session.
func (s *Session) HasFilter(name string) bool {
	if s.sp().HasFilter(name) {
		return true
	}
	for _, lp := range s.plugins().loaded() {
		if lp.prog.HasFilter(name) {
			return true
		}
	}
	return false
}

// ApplyFilter runs the filters of name on value (design.Filterer): each in
// the session VM with its time limit (50 ms on the render path, 2 s
// otherwise); a filter that throws or times out is skipped and logged.
func (s *Session) ApplyFilter(name string, value any, ctx map[string]any) any {
	if !s.HasFilter(name) {
		return value
	}
	if ctx == nil {
		ctx = map[string]any{}
	}
	limit := filterTimeout(s.e, name)
	for _, h := range filterChain(s.plugins(), s.sp(), name) {
		v, err := s.unitVM(h.unit)
		if err != nil {
			continue
		}
		list := v.filtersFor(name)
		if h.idx >= len(list) {
			continue
		}
		f := list[h.idx]
		u := v.u
		src := u.src("filter " + name)
		if s.draft {
			src = "preview " + src
		}
		v.rt.ClearInterrupt()
		v.user, v.logs, v.cookies = s.user, nil, nil
		arg := v.toJS(value)
		val, err := v.call(f.fn, limit, arg, v.filterCtx(ctx))
		var out any
		if err == nil {
			if val == nil || goja.IsUndefined(val) {
				val = arg
			}
			_, err = v.guarded(limit, func() (goja.Value, error) {
				str, ok, err := v.toJSON(val)
				if err != nil {
					return nil, err
				}
				if !ok {
					out = nil
					return nil, nil
				}
				return nil, jsonUnmarshal(str, &out)
			})
			if err != nil {
				err = v.runErr(err)
			}
		}
		s.logs = append(s.logs, unitEntries(logEntries(v.logs, src), u)...)
		if err != nil {
			re := toRunError(err)
			e := unitEntry(errorEntry(src, re), u)
			e.Level = "warn"
			e.Message = fmt.Sprintf("filter %s (%s:%d) ถูกข้าม: %s", name, f.at.File, f.at.Line, re.Error())
			s.logs = append(s.logs, e)
			if re.Timeout {
				s.reset(h.unit)
			}
			continue
		}
		value = out
	}
	return value
}

// ---------------------------------------------------------------- visitor

// defineVisitor adds the lazy ctx.visitor property.
func (v *vm) defineVisitor(o *goja.Object) {
	var cached goja.Value
	getter := v.rt.ToValue(func(goja.FunctionCall) goja.Value {
		if cached == nil {
			cached = v.toJS(v.resolveVisitor())
		}
		return cached
	})
	_ = o.DefineAccessorProperty("visitor", getter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
}

// resolveVisitor runs site.visitor at most once per request (200 ms; an
// error or a bad result → null, logged).
func (v *vm) resolveVisitor() map[string]any {
	if v.vis == nil {
		v.vis = &visitorState{}
	}
	if v.vis.done {
		return v.vis.val
	}
	if len(v.visitors) == 0 {
		if v.sess != nil && v.sess.vis == v.vis && v.h != nil && v.h.e != nil && v.h.e.visitorUnit(v.sess) != "" {
			return v.sess.Visitor() // another unit resolves the visitor
		}
		v.vis.done = true
		return nil
	}
	v.vis.done = true
	fn := v.visitors[0].fn
	req := v.req
	if req == nil {
		req, _ = http.NewRequest(http.MethodGet, "http://site.local/", nil)
	}
	prevMode := v.mode
	v.mode = modeHandler
	defer func() { v.mode = prevMode }()
	ctx := v.rt.NewObject()
	_ = ctx.Set("request", v.requestValue(req))
	out, err := v.nestedCall(fn, visitorTimeout, ctx)
	if err != nil {
		v.addLogLine("error", "site.visitor ล้มเหลว (ถือว่าไม่ใช่สมาชิก): "+v.runErr(err).Error())
		return nil
	}
	if p, ok := out.Export().(*goja.Promise); ok {
		if p.State() != goja.PromiseStateFulfilled {
			v.addLogLine("error", "site.visitor ล้มเหลว: Promise ไม่สำเร็จ")
			return nil
		}
		out = p.Result()
	}
	if out == nil || goja.IsUndefined(out) || goja.IsNull(out) {
		return nil
	}
	s, ok, err := v.toJSON(out)
	var m map[string]any
	if err != nil || !ok || jsonUnmarshal(s, &m) != nil || m == nil {
		v.addLogLine("warn", "site.visitor ต้องคืน object { id, name?, groups } หรือ null")
		return nil
	}
	switch id := m["id"].(type) {
	case string:
		if id == "" {
			v.addLogLine("warn", "site.visitor: id ว่าง — ถือว่าไม่ใช่สมาชิก")
			return nil
		}
	case float64:
	default:
		v.addLogLine("warn", "site.visitor: id ต้องเป็นข้อความหรือตัวเลข")
		return nil
	}
	groups := []any{}
	if l, ok := m["groups"].([]any); ok {
		for _, g := range l {
			if gs, ok := g.(string); ok && len(groups) < 100 {
				groups = append(groups, gs)
			}
		}
	}
	m["groups"] = groups
	v.vis.val = m
	return m
}

// nestedCall calls fn inside (or outside) a running handler with its own
// time limit: the watchdog's interrupt is cleared afterwards unless the
// outer handler's deadline passed too.
func (v *vm) nestedCall(fn goja.Callable, limit time.Duration, args ...goja.Value) (out goja.Value, err error) {
	fired := make(chan struct{})
	t := time.AfterFunc(limit, func() {
		close(fired)
		v.rt.Interrupt(errTimeout)
	})
	defer func() {
		t.Stop()
		select {
		case <-fired:
			v.rt.ClearInterrupt()
			if v.inCall && !v.deadline.IsZero() && time.Now().After(v.deadline) {
				v.rt.Interrupt(errTimeout) // the handler is out of time as well
			}
		default:
		}
		if rec := recover(); rec != nil {
			out, err = nil, &RunError{Message: fmt.Sprintf("internal error: %v", rec)}
		}
	}()
	return fn(goja.Undefined(), args...)
}

// groupsOf returns visitor.groups.
func groupsOf(vis map[string]any) []string {
	var out []string
	if l, ok := vis["groups"].([]any); ok {
		for _, g := range l {
			if s, ok := g.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// allowedFor checks an access list: '*', role slugs for CMS users, and
// 'group:<name>' entries for anonymous visitors (groups is called only when
// needed, so the visitor hook runs lazily).
func allowedFor(access []string, u *User, groups func() []string) bool {
	if allowed(access, u) {
		return true
	}
	if u != nil || groups == nil {
		return false
	}
	need := false
	for _, a := range access {
		if strings.HasPrefix(a, "group:") {
			need = true
		}
	}
	if !need {
		return false
	}
	for _, g := range groups() {
		for _, a := range access {
			if a == "group:"+g {
				return true
			}
		}
	}
	return false
}

// hasGroupEntries reports 'group:' entries in an access list.
func hasGroupEntries(access []string) bool {
	for _, a := range access {
		if strings.HasPrefix(a, "group:") {
			return true
		}
	}
	return false
}

// Visitor resolves the request's visitor (design.VisitorSource).
func (s *Session) Visitor() map[string]any {
	if s.vis != nil && s.vis.done {
		return s.vis.val
	}
	if s.vis == nil {
		s.vis = &visitorState{}
	}
	unit := s.e.visitorUnit(s)
	if unit == "" {
		s.vis.done = true
		return nil
	}
	v, err := s.unitVM(unit)
	if err != nil || len(v.visitors) == 0 {
		s.vis.done = true
		return nil
	}
	busy := v.inCall || v.xdepth > 0
	var saved []LogLine
	if !busy {
		v.rt.ClearInterrupt()
		v.logs = nil
	} else {
		saved = v.logs
	}
	val := v.resolveVisitor()
	if !busy {
		src := v.u.src("visitor")
		if s.draft {
			src = "preview " + src
		}
		s.logs = append(s.logs, unitEntries(logEntries(v.logs, src), v.u)...)
	} else {
		_ = saved
	}
	return val
}

// permit checks access for the session's caller.
func (s *Session) permit(access []string) bool {
	return allowedFor(access, s.user, func() []string { return groupsOf(s.Visitor()) })
}

// SetVisitor fixes the session's visitor (the editor's "view as visitor in
// group…" preview); nil = anonymous.
func (s *Session) SetVisitor(vis map[string]any) {
	s.vis = &visitorState{done: true, val: vis}
	if s.v != nil {
		s.v.vis = s.vis
	}
}

// MediaBeforeSave runs the media.beforeSave filters for a file uploaded in
// the admin (2.1.0): value {filename, alt}, ctx {mime, size, kind, source,
// user}; a filter may change filename (same extension) and alt, or return
// {error: 'message'} to refuse the file.
func (e *Engine) MediaBeforeSave(m *media.Meta, user *User) error {
	opts, err := e.st.Options()
	if err != nil {
		return nil
	}
	req, _ := http.NewRequest(http.MethodPost, "http://site.local/", nil)
	s := e.LiveSession(req, opts)
	if s == nil || !s.HasFilter("media.beforeSave") {
		return nil
	}
	s.user = user
	defer s.Close()
	res := s.ApplyFilter("media.beforeSave", map[string]any{"filename": m.Filename, "alt": m.Alt}, mediaCtx(m))
	applyMediaMeta(m, res)
	if out, ok := res.(map[string]any); ok {
		if msg, ok := out["error"].(string); ok && msg != "" {
			return errors.New(msg)
		}
	}
	return nil
}

func mediaCtx(m *media.Meta) map[string]any {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(m.Filename)), ".")
	return map[string]any{"mime": m.Mime, "size": m.Size, "kind": model.FileKind(ext), "source": m.Source}
}

// applyMediaMeta copies filename / alt of a media.beforeSave result.
func applyMediaMeta(m *media.Meta, res any) {
	out, ok := res.(map[string]any)
	if !ok {
		return
	}
	if s, ok := out["filename"].(string); ok && strings.TrimSpace(s) != "" {
		m.Filename = clip(strings.TrimSpace(s), 200)
	}
	if s, ok := out["alt"].(string); ok {
		m.Alt = clip(s, 255)
	}
}

// BeforeSave runs the live code's post.beforeSave filters for a save made
// in the admin (value: {type, title, slug, status, excerpt, content, meta}).
// A filter returning {error: 'message'} refuses the save: msg is set.
func (e *Engine) BeforeSave(value map[string]any, user *User, isNew bool) (out map[string]any, msg string) {
	opts, err := e.st.Options()
	if err != nil {
		return value, ""
	}
	req, _ := http.NewRequest(http.MethodPost, "http://site.local/", nil)
	s := e.LiveSession(req, opts)
	if s == nil || !s.HasFilter("post.beforeSave") {
		return value, ""
	}
	s.user = user
	defer s.Close()
	res := s.ApplyFilter("post.beforeSave", value, map[string]any{"isNew": isNew})
	m, ok := res.(map[string]any)
	if !ok || m == nil {
		s.logs = append(s.logs, LogEntry{Level: "warn", Source: "filter post.beforeSave", Message: "post.beforeSave ต้องคืน object ของโพสต์ — ใช้ค่าเดิม"})
		return value, ""
	}
	if e, ok := m["error"].(string); ok && e != "" {
		return nil, e
	}
	return m, ""
}
