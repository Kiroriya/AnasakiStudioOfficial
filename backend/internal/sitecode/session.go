package sitecode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// Session is Site Code bound to one public request (design.CodeSession):
// the visitor (ctx.user), their cookies, one VM reused for every call of
// the request (collections, page data, formatters, the action), and the
// logs written when it closes.
type Session struct {
	e       *Engine
	prog    *Program
	lp      *liveProg // nil for a draft preview
	version int64
	r       *http.Request
	user    *User
	draft   bool

	v       *vm
	release func()
	vmErr   error

	// phase 4b: the other units of the request
	set   *unitSet
	setOK bool
	pvs   map[string]*vm // VMs of JS plugins
	held  bool           // the caller holds the VM slot (route, event, job)
	lazy  bool           // prog / lp resolved on first use (attachSession)
	depth int            // nesting of cross-unit calls

	logs    []LogEntry
	cookies []string
	pages   map[string]map[string]any
	cols    map[string]map[string]bool
	vis     *visitorState     // the visitor (site.visitor), shared by the session's VMs
	aliases map[string]string // enabled formatter aliases (loaded on first use)
	locale  string            // ctx.locale of admin page calls ("th" | "en", phase 6)
	admin   bool              // an admin page session (site.saveSettings, phase 7)
	system  bool              // a system call (CallExport): VMs run with db.asSystem rights
	// posts read ahead by core for the render-path hooks (PreloadPost): site.meta
	// answers from them, so the 50 ms of page.meta / site.head is not spent on a
	// database round trip; dropped when the session writes the post.
	posts map[int64]*model.Post
	// LastError is the last load error (preview API).
	LastError string
}

var errBusy = errors.New("ระบบไม่ว่าง กรุณาลองใหม่อีกครั้ง")

// Session returns the live code's session for a public request; nil when
// nothing is published or Site Code is off (code:/table: sources then
// render empty).
func (e *Engine) Session(r *http.Request, opts map[string]string) design.CodeSession {
	s := e.LiveSession(r, opts)
	if s == nil {
		return nil
	}
	return s
}

// LiveSession is Session with the concrete type.
func (e *Engine) LiveSession(r *http.Request, opts map[string]string) *Session {
	if e.pluginsOff(opts) {
		return nil
	}
	set := e.units()
	var lp *liveProg
	if opts[OptVersion] != "" && !e.siteOff(opts) {
		if lp = e.current(opts); lp != nil && lp.prog == nil {
			lp = nil
		}
	}
	if lp == nil && set.empty() {
		return nil
	}
	s := &Session{e: e, r: r, user: e.userFrom(r), set: set, setOK: true}
	if lp != nil {
		s.prog, s.lp, s.version = lp.prog, lp, lp.id
	} else {
		s.prog = e.emptyProgram()
	}
	return s
}

// emptyProgram stands in for Site Code when nothing is published (plugins
// may still run).
func (e *Engine) emptyProgram() *Program {
	e.emptyOnce.Do(func() {
		prog, _ := goja.Compile("bundle.js", "", false)
		e.empty = &Program{unit: e.siteU, prog: prog, byKey: map[string]int{}, events: map[string]bool{},
			collections: map[string]*CollectionInfo{}, actions: map[string]*ActionInfo{}, formatters: map[string]bool{},
			filterNames: map[string]bool{}, jobs: map[string]*JobInfo{}, adminPages: map[string]*AdminPageInfo{}, exports: map[string]bool{}}
		e.empty.Manifest.normalize()
	})
	return e.empty
}

// sp is the session's Site Code program (never nil).
func (s *Session) sp() *Program {
	if s.lazy && s.prog == nil {
		if lp := s.e.live.Load(); lp != nil && lp.prog != nil {
			if opts, err := s.e.st.Options(); err == nil && liveID(opts) == lp.id && !s.e.siteOff(opts) {
				s.prog, s.lp, s.version = lp.prog, lp, lp.id
			}
		}
		if s.prog == nil {
			s.prog = s.e.emptyProgram()
		}
	}
	if s.prog == nil {
		return s.e.emptyProgram()
	}
	return s.prog
}

// plugins is the session's plugin units.
func (s *Session) plugins() *unitSet {
	if !s.setOK {
		s.set, s.setOK = s.e.units(), true
	}
	return s.set
}

// progOf is the program of unit (nil when not running).
func (s *Session) progOf(unit string) *Program {
	if unit == SiteUnit || unit == "" {
		return s.sp()
	}
	if lp := s.plugins().get(unit); lp != nil {
		return lp.prog
	}
	return nil
}

// own registers v as the session's VM of its unit.
func (s *Session) own(v *vm) {
	if v.u.plugin() {
		if s.pvs == nil {
			s.pvs = map[string]*vm{}
		}
		s.pvs[v.u.ID] = v
	} else {
		s.v = v
	}
}

func (s *Session) bind(v *vm) {
	if s.vis == nil {
		s.vis = &visitorState{}
	}
	v.vis, v.req, v.sess = s.vis, s.request(), s
	s.own(v)
}

// slot takes the VM slot of the session (once).
func (s *Session) slot() error {
	if s.held || s.release != nil {
		return nil
	}
	release, ok := s.e.acquire(s.e.QueueWait)
	if !ok {
		s.vmErr = errBusy
		return s.vmErr
	}
	s.release = release
	return nil
}

// reset drops the VM of unit (interrupted: the next call gets a fresh one).
func (s *Session) reset(unit string) {
	if unit == SiteUnit || unit == "" {
		s.v = nil
	} else {
		delete(s.pvs, unit)
	}
}

// unitVM returns the session's VM of unit, loading it on first use.
func (s *Session) unitVM(unit string) (*vm, error) {
	if unit == SiteUnit || unit == "" {
		return s.vm()
	}
	if v := s.pvs[unit]; v != nil {
		return v, nil
	}
	if s.vmErr != nil {
		return nil, s.vmErr
	}
	lp := s.plugins().get(unit)
	if lp == nil {
		return nil, &RunError{Message: "ปลั๊กอิน " + unit + " ไม่ได้ทำงานอยู่ (ยังไม่ได้เปิดใช้งาน หรือโหลดไม่สำเร็จ)"}
	}
	if err := s.slot(); err != nil {
		return nil, err
	}
	v, err := s.e.liveVM(lp)
	if err != nil {
		return nil, err
	}
	s.bind(v)
	return v, nil
}

// PreviewSession runs the live version (or, with draft, the draft) as user
// (nil = an anonymous visitor) for the editor's preview API.
func (e *Engine) PreviewSession(user *User, draft bool) (*Session, []Problem, error) {
	req, _ := http.NewRequest(http.MethodGet, "http://site.local/", nil)
	if draft {
		out, _, err := e.BuildDraft()
		if err != nil {
			return nil, nil, err
		}
		if !out.OK {
			return nil, out.Errors, nil
		}
		return &Session{e: e, prog: out.prog, r: req, user: user, draft: true, set: e.units(), setOK: true}, nil, nil
	}
	opts, err := e.st.Options()
	if err != nil {
		return nil, nil, err
	}
	s := e.LiveSession(req, opts)
	if s == nil {
		return nil, nil, nil
	}
	s.user = user
	return s, nil, nil
}

// Program returns the session's program (bindings API).
func (s *Session) Program() *Program { return s.prog }

// Close releases the VM slot and writes the logs.
func (s *Session) Close() {
	if s.release != nil {
		s.release()
		s.release = nil
	}
	s.v, s.pvs = nil, nil
	if len(s.logs) > 0 {
		s.e.persist(s.logs, s.version)
		s.logs = nil
	}
}

// PreloadPost reads post id ahead of the render-path hooks (page.meta,
// site.head / bodyEnd), outside their time limit; site.meta(id) in those
// hooks is then answered from memory. Host calls are not interruptible, so a
// slow database read inside a 50 ms hook did not make the page faster: it
// only made core throw the hook's result away (SEO title, canonical,
// noindex) after the page had already waited for the read.
func (s *Session) PreloadPost(id int64) {
	if s == nil || id <= 0 || s.e == nil || s.e.h == nil || s.e.h.posts == nil {
		return
	}
	if _, ok := s.posts[id]; ok {
		return
	}
	p, err := s.e.h.posts.GetPost(id)
	if err != nil || p == nil {
		return
	}
	if s.posts == nil {
		s.posts = map[int64]*model.Post{}
	}
	s.posts[id] = p
}

// preloaded is a post read by PreloadPost (nil when it was not read ahead).
func (s *Session) preloaded(id int64) *model.Post {
	if s == nil {
		return nil
	}
	return s.posts[id]
}

// forgetPost drops a read-ahead post the session has written.
func (s *Session) forgetPost(id int64) {
	if s != nil {
		delete(s.posts, id)
	}
}

// Cookies are the Set-Cookie values the code set (response.setCookie).
func (s *Session) Cookies() []string { return s.cookies }

func (s *Session) vm() (*vm, error) {
	if s.v != nil {
		return s.v, nil
	}
	if s.vmErr != nil {
		return nil, s.vmErr
	}
	if err := s.slot(); err != nil {
		return nil, err
	}
	prog := s.sp()
	var v *vm
	var err error
	if s.lp != nil {
		v, err = s.e.liveVM(s.lp)
	} else {
		v, err = s.e.newLoaded(prog)
	}
	if err != nil {
		s.vmErr = err
		return nil, err
	}
	s.bind(v)
	return v, nil
}

func (s *Session) request() *http.Request {
	if s.r != nil {
		return s.r
	}
	req, _ := http.NewRequest(http.MethodGet, "http://site.local/", nil)
	return req
}

// call runs the handler pick finds in the session's Site Code VM.
func (s *Session) call(source string, timeout time.Duration, pick func(v *vm) goja.Callable, args func(v *vm) []goja.Value) (any, error) {
	return s.callIn(SiteUnit, source, timeout, pick, args)
}

// callIn runs the handler pick finds in the session's VM of unit.
func (s *Session) callIn(unit, source string, timeout time.Duration, pick func(v *vm) goja.Callable, args func(v *vm) []goja.Value) (any, error) {
	return s.callInWith(unit, source, timeout, pick, args, nil)
}

// callInWith is callIn with conv turning the handler's return value into
// the result inside the VM (nil = JSON copy, max 10 MB).
func (s *Session) callInWith(unit, source string, timeout time.Duration, pick func(v *vm) goja.Callable, args func(v *vm) []goja.Value,
	conv func(v *vm, val goja.Value) (any, error)) (any, error) {
	u := s.e.siteU
	if lp := s.plugins().get(unit); lp != nil && unit != SiteUnit {
		u = lp.unit
	}
	source = u.src(source)
	if !s.draft && !s.e.breaker.Allow(source) {
		return nil, &RunError{Message: "ปิดชั่วคราวเนื่องจากเกิดข้อผิดพลาดซ้ำ ๆ"}
	}
	v, err := s.unitVM(unit)
	if err != nil {
		return nil, err
	}
	v.rt.ClearInterrupt()
	v.user, v.logs, v.cookies, v.source = s.user, nil, nil, source
	if s.system {
		prevSys := v.system
		v.system = true
		defer func() { v.system = prevSys }()
	}
	fn := pick(v)
	if fn == nil {
		return nil, &RunError{Message: source + " ไม่ได้ลงทะเบียนในรอบนี้ (ต้องเรียกทุกครั้งแบบเดียวกัน)"}
	}
	v.mode = modeHandler
	val, err := v.call(fn, timeout, args(v)...)
	var out any
	if err == nil {
		_, err = v.guarded(timeout, func() (goja.Value, error) {
			if conv != nil {
				var err error
				out, err = conv(v, val)
				return nil, err
			}
			str, ok, err := v.toJSON(val)
			if err != nil || !ok {
				return nil, err
			}
			if len(str) > MaxResultBytes {
				return nil, &RunError{Message: "ผลลัพธ์ใหญ่เกิน 10 MB"}
			}
			return nil, json.Unmarshal([]byte(str), &out)
		})
		if err != nil {
			err = v.runErr(err)
		}
	}
	src := source
	if s.draft {
		src = "preview " + source
	}
	s.logs = append(s.logs, unitEntries(logEntries(v.logs, src), u)...)
	s.cookies = append(s.cookies, v.cookies...)
	if err != nil {
		re := toRunError(err)
		s.logs = append(s.logs, unitEntry(errorEntry(src, re), u))
		if !s.draft {
			s.e.failedIn(u, source, re, s.version)
		}
		if re.Timeout {
			s.reset(unit) // interrupted: the next call gets a fresh VM
		}
		s.LastError = re.Error()
		return nil, re
	}
	return out, nil
}

// ctxObject is the ctx of a collection / page / action handler.
func (s *Session) ctxObject(v *vm, extra map[string]any, params map[string]string) goja.Value {
	if params == nil {
		params = map[string]string{}
	}
	ctx := v.ctxValue(s.request(), params).ToObject(v.rt)
	if s.locale != "" {
		_ = ctx.Set("locale", s.locale)
	}
	for k, x := range extra {
		_ = ctx.Set(k, v.toJS(x))
	}
	return ctx
}

func roleKey(u *User) string {
	if u == nil {
		return "*"
	}
	return u.Role
}

// ---------------------------------------------------------------- collections

// LoadCollection serves code:<name> and table:<name> sources.
func (s *Session) LoadCollection(req design.CollectionReq) design.CollectionRes {
	if name, ok := strings.CutPrefix(req.Source, "code:"); ok {
		return s.filterItems(name, req, s.loadCode(name, req))
	} // name is "<collection>" (Site Code) or "<plugin id>:<collection>"
	if name, ok := strings.CutPrefix(req.Source, "table:"); ok {
		return s.filterItems("table:"+name, req, s.loadTable(name, req))
	}
	return design.CollectionRes{Items: []map[string]any{}, Error: "unknown source"}
}

func (s *Session) loadCode(qname string, req design.CollectionReq) design.CollectionRes {
	res := design.CollectionRes{Items: []map[string]any{}, Page: max(1, req.Page)}
	unit, name := SplitUnit(qname)
	var info *CollectionInfo
	if p := s.progOf(unit); p != nil {
		info = p.collections[name]
	}
	if info == nil {
		res.Error = fmt.Sprintf("ไม่มี site.collection %q ในโค้ดที่เผยแพร่", qname)
		s.LastError = res.Error
		return res
	}
	if !s.permit(s.e.Effective(KindCollection, qname, info.Access)) {
		res.Denied = true
		return res
	}
	perPage := req.PerPage
	if perPage <= 0 {
		perPage = info.PerPage.Default
	}
	perPage = min(perPage, info.PerPage.Max, hardPerPage)
	res.PerPage = perPage
	params := coerceParams(info.Params, req.Params)
	key := ""
	if info.Cache > 0 && !s.draft {
		pj, _ := json.Marshal(params)
		key = fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%d\x00%s", s.version, qname, pj, res.Page, perPage, roleKey(s.user))
		if c, ok := s.e.collCache.Get(key); ok {
			var cached design.CollectionRes
			if json.Unmarshal([]byte(c), &cached) == nil {
				return cached
			}
		}
	}
	out, err := s.callIn(unit, "collection "+name, s.e.RouteTimeout, func(v *vm) goja.Callable {
		for _, c := range v.colls {
			if c.info.Name == name {
				return c.fn
			}
		}
		return nil
	}, func(v *vm) []goja.Value {
		return []goja.Value{s.ctxObject(v, map[string]any{"page": res.Page, "perPage": perPage, "params": params}, nil)}
	})
	if err != nil {
		res.Error = toRunError(err).Error()
		return res
	}
	var list []any
	total := -1
	switch x := out.(type) {
	case []any:
		list = x
	case map[string]any:
		list, _ = x["items"].([]any)
		if t, ok := x["total"].(float64); ok && t >= 0 {
			total = int(t)
		}
	}
	if total < 0 {
		// a plain list (or no total): it is the whole collection when longer than a page
		if len(list) > perPage {
			total = len(list)
			lo := min((res.Page-1)*perPage, len(list))
			list = list[lo:min(lo+perPage, len(list))]
		} else {
			total = (res.Page-1)*perPage + len(list)
		}
	} else if len(list) > perPage {
		list = list[:perPage]
	}
	res.Total = total
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			m = map[string]any{"value": it}
		}
		for f, t := range info.Fields {
			if t == "html" {
				if sv, ok := m[f]; ok && sv != nil {
					m[f] = design.SanitizeRich(design.Stringify(sv))
				}
			}
		}
		res.Items = append(res.Items, m)
	}
	if key != "" {
		if b, err := json.Marshal(res); err == nil {
			_ = s.e.collCache.Set(key, string(b), time.Duration(info.Cache)*time.Second)
		}
	}
	return res
}

func (s *Session) tableColumns(ctx context.Context, name string) (map[string]bool, error) {
	if c, ok := s.cols[name]; ok {
		return c, nil
	}
	d := s.e.data
	cols, err := d.columns(ctx, d.N.Phys(name))
	if err != nil {
		return nil, err
	}
	m := map[string]bool{}
	for _, c := range cols {
		m[c.Name] = true
	}
	if s.cols == nil {
		s.cols = map[string]map[string]bool{}
	}
	s.cols[name] = m
	return m, nil
}

var tableOps = map[string]string{"=": "=", "!=": "!=", "<": "<", "<=": "<=", ">": ">", ">=": ">=", "like": "like"}

func (s *Session) loadTable(name string, req design.CollectionReq) design.CollectionRes {
	res := design.CollectionRes{Items: []map[string]any{}, Page: max(1, req.Page)}
	fail := func(msg string) design.CollectionRes {
		res.Error = "table:" + name + ": " + msg
		s.LastError = res.Error
		s.logs = append(s.logs, LogEntry{Level: "error", Source: "collection table:" + name, Message: msg})
		return res
	}
	if !refIdentRe.MatchString(name) {
		return fail("ชื่อตารางไม่ถูกต้อง")
	}
	if !s.permit(s.e.Effective(KindTable, name, nil)) {
		res.Denied = true
		return res
	}
	d := s.e.data
	if d == nil {
		return fail("ฐานข้อมูลของโค้ดไม่พร้อมใช้งาน")
	}
	perPage := req.PerPage
	if perPage <= 0 {
		perPage = 10
	}
	perPage = min(perPage, hardPerPage)
	res.PerPage = perPage
	ctx, cancel := context.WithTimeout(context.Background(), s.e.RouteTimeout)
	defer cancel()
	q, err := NewQuery(d.N, name)
	if err != nil {
		return fail(err.Error())
	}
	cols, err := s.tableColumns(ctx, name)
	if err != nil {
		return fail(err.Error())
	}
	if len(cols) == 0 {
		return fail("ไม่พบตารางนี้")
	}
	col := func(c string) error {
		if !cols[c] {
			return fmt.Errorf("ไม่มีคอลัมน์ %q", c)
		}
		return nil
	}
	if t := req.Table; t != nil {
		for _, c := range t.Columns {
			if err := col(c); err != nil {
				return fail(err.Error())
			}
			if err := q.Select(c); err != nil {
				return fail(err.Error())
			}
		}
		for _, w := range t.Where {
			if err := col(w.Column); err != nil {
				return fail(err.Error())
			}
			op := strings.ToLower(strings.TrimSpace(w.Op))
			switch op {
			case "null", "notnull":
				_ = q.WhereNull(false, w.Column, op == "notnull")
				continue
			case "in":
				var list []any
				switch x := w.Value.(type) {
				case []any:
					list = x
				case nil:
				default:
					for _, p := range strings.Split(design.Stringify(x), ",") {
						if p = strings.TrimSpace(p); p != "" {
							list = append(list, p)
						}
					}
				}
				if len(list) == 0 {
					continue // no value: no filter
				}
				_ = q.WhereIn(false, w.Column, list, false)
				continue
			}
			o, ok := tableOps[op]
			if !ok {
				return fail(fmt.Sprintf("operator %q ไม่รองรับ", w.Op))
			}
			if w.Value == nil || design.Stringify(w.Value) == "" {
				continue // an empty filter value means "no filter"
			}
			c, err := Compare(w.Column, o, BindValue(scalar(w.Value)))
			if err != nil {
				return fail(err.Error())
			}
			q.Where(false, c)
		}
		for _, ob := range t.OrderBy {
			if err := col(ob.Column); err != nil {
				return fail(err.Error())
			}
			if err := q.OrderBy(ob.Column, ob.Dir); err != nil {
				return fail(err.Error())
			}
		}
	}
	// filter inputs: params named like a column filter it by equality
	keys := make([]string, 0, len(req.Params))
	for k := range req.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := req.Params[k]
		if !cols[k] || v == nil || design.Stringify(v) == "" {
			continue
		}
		c, err := Compare(k, "=", BindValue(scalar(v)))
		if err != nil {
			continue
		}
		q.Where(false, c)
	}
	ms := int(s.e.RouteTimeout / time.Millisecond)
	cs, cargs := q.CountSQL(ms)
	if err := d.code.QueryRowContext(ctx, cs, cargs...).Scan(&res.Total); err != nil {
		return fail(err.Error())
	}
	ps, pargs := q.PageSQL(ms, res.Page, perPage)
	rows, err := d.code.QueryContext(ctx, ps, pargs...)
	if err != nil {
		return fail(err.Error())
	}
	_, list, _, err := ReadRows(rows, perPage, true)
	if err != nil {
		return fail(err.Error())
	}
	for _, r := range list {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			res.Items = append(res.Items, m)
		}
	}
	return res
}

// scalar turns an evaluated binding value into a bind parameter.
func scalar(v any) any {
	switch x := v.(type) {
	case string, float64, bool, nil:
		return x
	}
	return design.Stringify(v)
}

// ---------------------------------------------------------------- formatters, pages

// PageData runs the site.page handler matching path ({{page.*}}).
func (s *Session) PageData(path string) map[string]any {
	if d, ok := s.pages[path]; ok {
		return d
	}
	if s.pages == nil {
		s.pages = map[string]map[string]any{}
	}
	s.pages[path] = nil
	// plugins in load order, then Site Code: later units' keys win
	var merged map[string]any
	units := []string{}
	for _, lp := range s.plugins().loaded() {
		units = append(units, lp.unit.ID)
	}
	units = append(units, SiteUnit)
	for _, unit := range units {
		if m := s.pageDataOf(unit, path); m != nil {
			if merged == nil {
				merged = map[string]any{}
			}
			for k, x := range m {
				merged[k] = x
			}
		}
	}
	s.pages[path] = merged
	return merged
}

func (s *Session) pageDataOf(unit, path string) map[string]any {
	prog := s.progOf(unit)
	i, params := prog.matchPage(path)
	if i < 0 {
		return nil
	}
	info, pattern := prog.pageInfo[i], prog.pageSpecs[i].Pattern
	name := info.Path
	if unit != SiteUnit {
		name = unit + ":" + info.Path
	}
	if !s.permit(s.e.Effective(KindPage, name, info.Access)) {
		return nil
	}
	out, err := s.callIn(unit, "page "+info.Path, s.e.RouteTimeout, func(v *vm) goja.Callable {
		for _, p := range v.pages {
			if p.spec.Pattern == pattern {
				return p.fn
			}
		}
		return nil
	}, func(v *vm) []goja.Value {
		return []goja.Value{s.ctxObject(v, map[string]any{"params": params}, params)}
	})
	if err != nil {
		return nil
	}
	m, _ := out.(map[string]any)
	return m
}

// ---------------------------------------------------------------- actions

// RunAction validates input and runs site.action name (POST /_a/{name}).
func (s *Session) RunAction(name string, input map[string]any) *design.ActionOutcome {
	unit, local := SplitUnit(name)
	var info *ActionInfo
	if p := s.progOf(unit); p != nil {
		info = p.actions[local]
	}
	if info == nil {
		return &design.ActionOutcome{Status: http.StatusNotFound, Error: "ไม่พบ action นี้"}
	}
	if s.r != nil && !s.e.limiter.Allow(plugin.ClientIP(s.r)+"\x00"+name) {
		return &design.ActionOutcome{Status: http.StatusTooManyRequests, Error: "เรียกใช้บ่อยเกินไป กรุณารอสักครู่แล้วลองใหม่"}
	}
	if !s.permit(s.e.Effective(KindAction, name, info.Access)) {
		if s.user == nil {
			return &design.ActionOutcome{Status: http.StatusUnauthorized, Error: "กรุณาเข้าสู่ระบบ"}
		}
		return &design.ActionOutcome{Status: http.StatusForbidden, Error: "คุณไม่มีสิทธิ์ทำรายการนี้"}
	}
	clean, errs := validateInput(info.Input, input)
	if len(errs) > 0 {
		return &design.ActionOutcome{Status: http.StatusBadRequest, Error: "ข้อมูลไม่ถูกต้อง", Errors: errs}
	}
	if s.HasFilter("action.input") {
		if m, ok := s.ApplyFilter("action.input", clean, map[string]any{"name": name}).(map[string]any); ok {
			clean = m
		}
	}
	out, err := s.callIn(unit, "action "+local, s.e.RouteTimeout, func(v *vm) goja.Callable {
		for _, a := range v.acts {
			if a.info.Name == local {
				return a.fn
			}
		}
		return nil
	}, func(v *vm) []goja.Value {
		return []goja.Value{s.ctxObject(v, map[string]any{"input": clean}, nil)}
	})
	if err != nil {
		if errors.Is(err, errBusy) {
			return &design.ActionOutcome{Status: http.StatusServiceUnavailable, Error: errBusy.Error()}
		}
		return &design.ActionOutcome{Status: http.StatusInternalServerError, Error: "ทำรายการไม่สำเร็จ"}
	}
	if s.HasFilter("action.result") {
		if out == nil {
			out = map[string]any{}
		}
		out = s.ApplyFilter("action.result", out, map[string]any{"name": name})
	}
	res := &design.ActionOutcome{Status: http.StatusOK, SetCookies: s.cookies}
	m, _ := out.(map[string]any)
	if m == nil {
		return res
	}
	res.Message, _ = m["message"].(string)
	if red, ok := m["redirect"].(string); ok && red != "" {
		if design.SameSitePath(red) {
			res.Redirect = red
		} else {
			s.logs = append(s.logs, LogEntry{Level: "warn", Source: "action " + name,
				Message: fmt.Sprintf("redirect %q ถูกละไว้: ใช้ได้เฉพาะ path ในเว็บนี้ (ขึ้นต้นด้วย /)", clip(red, 200))})
		}
	}
	if e, ok := m["errors"].(map[string]any); ok && len(e) > 0 {
		res.Errors = map[string]string{}
		for k, v := range e {
			res.Errors[k] = design.Stringify(v)
		}
	}
	strList := func(x any) []string {
		var out []string
		if l, ok := x.([]any); ok {
			for _, it := range l {
				if s, ok := it.(string); ok && s != "" && len(out) < 50 {
					out = append(out, s)
				}
			}
		}
		return out
	}
	res.Refresh, res.Update = strList(m["refresh"]), strList(m["update"])
	if unit != SiteUnit { // a plugin's own collections: code:<id>:<name>
		for i, n := range res.Refresh {
			if !strings.Contains(n, ":") {
				res.Refresh[i] = unit + ":" + n
			}
		}
	}
	res.Page, _ = m["page"].(map[string]any)
	res.Data = m["data"]
	return res
}

// ---------------------------------------------------------------- rate limit

// rateLimiter allows n calls per window per key (fixed windows).
type rateLimiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	hits   map[string]*rateHit
	now    func() time.Time
	sweep  time.Time
}

type rateHit struct {
	start time.Time
	count int
}

func newRateLimiter(n int, window time.Duration) *rateLimiter {
	return &rateLimiter{n: n, window: window, hits: map[string]*rateHit{}, now: time.Now}
}

// Allow counts one call of key.
func (l *rateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.n <= 0 {
		return false
	}
	now := l.now()
	if now.Sub(l.sweep) > l.window {
		for k, h := range l.hits {
			if now.Sub(h.start) >= l.window {
				delete(l.hits, k)
			}
		}
		l.sweep = now
	}
	h := l.hits[key]
	if h == nil || now.Sub(h.start) >= l.window {
		l.hits[key] = &rateHit{start: now, count: 1}
		return true
	}
	if h.count >= l.n {
		return false
	}
	h.count++
	return true
}

// SetLimit changes the allowed calls per window (0 = none allowed).
func (l *rateLimiter) SetLimit(n int) {
	l.mu.Lock()
	l.n = n
	l.mu.Unlock()
}

// filterItems runs the collection.items filters on a loaded page (after
// the cache: filters may depend on the visitor).
func (s *Session) filterItems(name string, req design.CollectionReq, res design.CollectionRes) design.CollectionRes {
	if res.Error != "" || res.Denied || !s.HasFilter("collection.items") {
		return res
	}
	items := make([]any, len(res.Items))
	for i, it := range res.Items {
		items[i] = it
	}
	params := req.Params
	if params == nil {
		params = map[string]any{}
	}
	out := s.ApplyFilter("collection.items", items, map[string]any{"name": name, "params": params, "page": res.Page})
	list, ok := out.([]any)
	if !ok {
		s.logs = append(s.logs, LogEntry{Level: "warn", Source: "filter collection.items", Message: "collection.items ต้องคืน array — ใช้รายการเดิม"})
		return res
	}
	res.Items = make([]map[string]any, 0, len(list))
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			m = map[string]any{"value": it}
		}
		res.Items = append(res.Items, m)
	}
	return res
}
