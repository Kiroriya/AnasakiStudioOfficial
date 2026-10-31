package sitecode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// Phase 4b interop between units: site.export and the export proxies
// imported by other units, filters of every unit in one chain, the shared
// visitor, plugins.call to WASM / service plugins.

// maxCrossDepth limits nested calls between units (A → B → A …).
const maxCrossDepth = 16

var exportNameRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)

type regExport struct {
	name string
	fn   goja.Callable
	at   Problem
}

// siteExport is site.export(name, fn): a function other units import; it
// runs in this unit's context (scopes, tables, settings).
func (v *vm) siteExport(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.export ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	name, _ := c.Argument(0).Export().(string)
	if !exportNameRe.MatchString(name) {
		v.throw("site.export: ชื่อ %q ต้องเป็นชื่อฟังก์ชัน JavaScript", c.Argument(0).String())
	}
	fn, ok := goja.AssertFunction(c.Argument(1))
	if !ok {
		v.throw("site.export(%q): ต้องส่งฟังก์ชัน", name)
	}
	if v.u.plugin() && !v.u.exports[name] {
		v.throw("site.export(%q): ต้องประกาศชื่อนี้ใน code.exports ของ manifest", name)
	}
	v.exps = append(v.exps, &regExport{name: name, fn: fn, at: v.caller()})
	return goja.Undefined()
}

func (p *Program) registerExports(v *vm) []Problem {
	var probs []Problem
	p.exports = map[string]bool{}
	for _, x := range v.exps {
		if p.exports[x.name] {
			probs = append(probs, at(x.at, fmt.Sprintf("site.export %q ซ้ำกับที่ลงทะเบียนไว้แล้ว", x.name)))
			continue
		}
		p.exports[x.name] = true
		p.Manifest.Exports = append(p.Manifest.Exports, x.name)
	}
	sort.Strings(p.Manifest.Exports)
	return probs
}

func (v *vm) exportFn(name string) goja.Callable {
	for _, x := range v.exps {
		if x.name == name {
			return x.fn
		}
	}
	return nil
}

// exportProxy is __anasaki_export(unit, name): the function the build puts
// in place of an imported export.
func (v *vm) exportProxy(c goja.FunctionCall) goja.Value {
	target, name := c.Argument(0).String(), c.Argument(1).String()
	return v.rt.ToValue(func(call goja.FunctionCall) goja.Value { return v.callExport(target, name, call.Arguments) })
}

// remaining is what is left of the running handler's time.
func (v *vm) remaining() time.Duration {
	if v.deadline.IsZero() {
		if v.h != nil && v.h.e != nil {
			return v.h.e.RouteTimeout
		}
		return 2 * time.Second
	}
	return time.Until(v.deadline)
}

// callExport runs export name of unit target in that unit's VM of the same
// request session: arguments and result are copied as JSON, the time counts
// against the caller, errors come back with the target's file:line.
func (v *vm) callExport(target, name string, args []goja.Value) goja.Value {
	return v.callExportOpt(target, name, args, true)
}

// callExportOpt is callExport; checkRequires=false for plugins.call (a JS
// plugin's declared exports are callable by any unit, phase 6).
func (v *vm) callExportOpt(target, name string, args []goja.Value, checkRequires bool) goja.Value {
	what := target + "." + name
	v.handlerOnly(what)
	if v.u.plugin() && checkRequires {
		if _, ok := v.u.requires[target]; !ok {
			v.fail("%s: ปลั๊กอิน %s ไม่ได้อยู่ใน requires", what, target)
		}
	}
	if v.sess == nil {
		v.fail("%s: เรียกได้เฉพาะภายใน handler", what)
	}
	tv, err := v.sess.unitVM(target)
	if err != nil {
		v.fail("%s: %s", what, toRunError(err).Message)
	}
	fn := tv.exportFn(name)
	if fn == nil {
		v.fail("%s: ปลั๊กอิน %s ไม่ได้ site.export(%q)", what, target, name)
	}
	argJSON, ok, jerr := v.toJSON(v.rt.NewArray(toAny(args)...))
	if jerr != nil || !ok {
		v.fail("%s: อาร์กิวเมนต์ต้องแปลงเป็น JSON ได้", what)
	}
	rem := v.remaining()
	if rem <= 0 {
		v.fail("%s: หมดเวลาของ handler แล้ว", what)
	}
	var out goja.Value
	cerr := v.sess.crossCall(v, tv, "export "+name, rem, func() (goja.Value, error) {
		parsed, err := tv.parse(goja.Undefined(), tv.rt.ToValue(argJSON))
		if err != nil {
			return nil, err
		}
		var list []goja.Value
		if o, ok := parsed.(*goja.Object); ok {
			for _, k := range o.Keys() {
				list = append(list, o.Get(k))
			}
		}
		r, err := fn(goja.Undefined(), list...)
		if err != nil {
			return nil, err
		}
		r, err = tv.settle(r)
		if err != nil {
			return nil, err
		}
		s, ok, err := tv.toJSON(r)
		if err != nil {
			return nil, &RunError{Message: "ผลลัพธ์ต้องแปลงเป็น JSON ได้"}
		}
		if ok {
			out = v.rt.ToValue(s)
		}
		return nil, nil
	})
	if cerr != nil {
		re := toRunError(cerr)
		where := ""
		if re.At.File != "" {
			where = fmt.Sprintf(" (%s: %s:%d:%d)", target, re.At.File, re.At.Line, re.At.Column)
		}
		if re.Timeout { // the caller is out of time too: end it as a timeout
			v.rt.Interrupt(errTimeout)
			return goja.Undefined()
		}
		v.fail("%s: %s%s", what, re.Message, where)
	}
	if out == nil {
		return goja.Undefined()
	}
	res, err := v.parse(goja.Undefined(), out)
	if err != nil {
		return goja.Undefined()
	}
	return res
}

func toAny(vals []goja.Value) []any {
	out := make([]any, len(vals))
	for i, x := range vals {
		out[i] = x
	}
	return out
}

// crossCall runs fn in tv (another unit's VM of the session) on behalf of
// caller, within limit; tv's logs go to the session, its cookies to caller.
func (s *Session) crossCall(caller, tv *vm, source string, limit time.Duration, fn func() (goja.Value, error)) (err error) {
	if s.depth >= maxCrossDepth {
		return &RunError{Message: "เรียกข้ามปลั๊กอินซ้อนกันลึกเกินไป (วนเรียกกันเอง?)"}
	}
	s.depth++
	defer func() { s.depth-- }()
	reentrant := tv.inCall || tv.xdepth > 0
	if !reentrant {
		tv.rt.ClearInterrupt()
		tv.logs, tv.cookies = nil, nil
		tv.mode, tv.user, tv.deadline, tv.system = modeHandler, caller.user, caller.deadline, false
		tv.source = tv.u.src(source)
	}
	tv.xdepth++
	defer func() {
		tv.xdepth--
		if !reentrant {
			s.logs = append(s.logs, unitEntries(logEntries(tv.logs, tv.u.src(source)), tv.u)...)
			caller.cookies = append(caller.cookies, tv.cookies...)
			tv.logs, tv.cookies = nil, nil
		}
	}()
	_, err = tv.nestedCall(goja.Callable(func(goja.Value, ...goja.Value) (goja.Value, error) { return fn() }), limit)
	if err != nil {
		if isInterrupt(err) {
			if !reentrant {
				s.reset(tv.u.id())
			}
		}
		return tv.runErr(err)
	}
	return nil
}

// ---------------------------------------------------------------- plugins.call

// pluginsCall is plugins.call(pluginId, fn, input): a service.functions
// function of a WASM / service plugin (signed POST /functions/<fn>).
func (v *vm) pluginsCall(c goja.FunctionCall) goja.Value {
	const what = "plugins.call"
	v.handlerOnly(what)
	id, _ := c.Argument(0).Export().(string)
	fn, _ := c.Argument(1).Export().(string)
	if !unitIDRe.MatchString(id) || fn == "" {
		v.throw("%s(pluginId, fn, input): ต้องระบุ id ปลั๊กอินและชื่อฟังก์ชัน", what)
	}
	e := v.h.e
	if e == nil || e.cfg.Plugins == nil {
		v.fail("%s: ไม่พร้อมใช้งาน", what)
	}
	// phase 6: a JS plugin's code.exports — same semantics as an import proxy
	if ref, ok := e.jsRef(id); ok {
		var lp *liveProg
		if v.sess != nil {
			lp = v.sess.plugins().get(id)
		}
		if !ref.Active || lp == nil {
			v.unavailable(id)
		}
		if !lp.unit.exports[fn] || !lp.prog.exports[fn] {
			v.fail("%s(%q, %q): ปลั๊กอิน %s ไม่ได้ประกาศ %q ใน code.exports", what, id, fn, id, fn)
		}
		var args []goja.Value
		if len(c.Arguments) > 2 {
			args = c.Arguments[2:]
		}
		return v.callExportOpt(id, fn, args, false)
	}
	if !e.pluginActive(id) {
		v.unavailable(id)
	}
	if v.u.plugin() { // a service / WASM function: the caller lists the plugin
		_, req := v.u.requires[id]
		_, opt := v.u.optional[id]
		if !req && !opt {
			v.fail("%s: ปลั๊กอิน %s ไม่ได้อยู่ใน requires (หรือ optionalRequires)", what, id)
		}
	}
	if !e.cfg.Plugins.HasFunction(id, fn) {
		v.fail("%s: ปลั๊กอิน %s ไม่มีฟังก์ชัน %q (service.functions)", what, id, fn)
	}
	input := json.RawMessage("null")
	if x := c.Argument(2); !goja.IsUndefined(x) {
		s, ok, err := v.toJSON(x)
		if err != nil {
			v.fail("%s: input ต้องแปลงเป็น JSON ได้", what)
		}
		if ok {
			input = json.RawMessage(s)
		}
	}
	var user any
	if v.user != nil {
		user = map[string]any{"id": v.user.ID, "name": v.user.Name, "email": v.user.Email, "role": v.user.Role}
	}
	var visitor any
	if vis := v.resolveVisitor(); vis != nil {
		visitor = vis
	}
	body, _ := json.Marshal(map[string]any{"input": input, "user": user, "visitor": visitor})
	rem := v.remaining()
	if rem <= 10*time.Millisecond {
		v.fail("%s: หมดเวลาของ handler แล้ว", what)
	}
	status, resp, err := e.cfg.Plugins.CallFunction(id, fn, body, min(rem, 10*time.Second))
	if err != nil {
		v.fail("%s(%q, %q): %s", what, id, fn, err.Error())
	}
	var out any
	if len(resp) > 0 && json.Unmarshal(resp, &out) != nil {
		out = string(resp)
	}
	if status >= 300 {
		msg := fmt.Sprintf("HTTP %d", status)
		if m, ok := out.(map[string]any); ok {
			if s, ok := m["error"].(string); ok && s != "" {
				msg = s
			}
		}
		v.fail("%s(%q, %q): %s", what, id, fn, msg)
	}
	if m, ok := out.(map[string]any); ok {
		if r, ok := m["result"]; ok {
			return v.toJS(r)
		}
	}
	return v.toJS(out)
}

// ---------------------------------------------------------------- the filter chain of all units

// hookRef is one filter of the chain: its unit and position among that
// unit's filters of the name (vm.filtersFor order).
type hookRef struct {
	unit string
	idx  int
	prio int
	at   Problem
}

// filterOrder lists p's filters of name in run order (ascending priority,
// then registration order), like vm.filtersFor.
func (p *Program) filterOrder(name string) []filterMeta {
	var out []filterMeta
	if p == nil {
		return out
	}
	for _, f := range p.filterList {
		if f.Name == name {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out
}

// filterChain is the filters of name across units: ascending priority,
// then unit load order (plugins by requires / install time, Site Code
// last), then registration order.
func filterChain(set *unitSet, site *Program, name string) []hookRef {
	var out []hookRef
	add := func(unit string, p *Program) {
		if !p.HasFilter(name) {
			return
		}
		for i, f := range p.filterOrder(name) {
			out = append(out, hookRef{unit: unit, idx: i, prio: f.Priority, at: f.At})
		}
	}
	for _, lp := range set.loaded() {
		add(lp.unit.ID, lp.prog)
	}
	add(SiteUnit, site)
	sort.SliceStable(out, func(i, j int) bool { return out[i].prio < out[j].prio })
	return out
}

// multiUnit reports whether a filter of name exists outside unit.
func (s *Session) multiUnit(name, unit string) bool {
	for _, lp := range s.plugins().loaded() {
		if lp.unit.ID != unit && lp.prog.HasFilter(name) {
			return true
		}
	}
	return unit != SiteUnit && s.sp().HasFilter(name)
}

// crossFilter runs filter h (of another unit) on value for caller v.
func (v *vm) crossFilter(h hookRef, name string, value, ctx goja.Value) goja.Value {
	tv, err := v.sess.unitVM(h.unit)
	if err != nil {
		return value
	}
	list := tv.filtersFor(name)
	if h.idx >= len(list) {
		return value
	}
	f := list[h.idx]
	valJSON, ok, err := v.toJSON(value)
	if err != nil || !ok {
		valJSON = "null"
	}
	ctxJSON, ok, err := v.toJSON(ctx)
	if err != nil || !ok {
		ctxJSON = "{}"
	}
	var out string
	var hasOut bool
	cerr := v.sess.crossCall(v, tv, "filter "+name, v.remaining(), func() (goja.Value, error) {
		arg, err := tv.parse(goja.Undefined(), tv.rt.ToValue(valJSON))
		if err != nil {
			return nil, err
		}
		cv, err := tv.parse(goja.Undefined(), tv.rt.ToValue(ctxJSON))
		if err != nil {
			return nil, err
		}
		co := cv.ToObject(tv.rt)
		if co.Get("user") == nil {
			_ = co.Set("user", tv.userValue())
		}
		tv.defineVisitor(co)
		r, err := f.fn(goja.Undefined(), arg, co)
		if err != nil {
			return nil, err
		}
		if r == nil || goja.IsUndefined(r) {
			r = arg
		}
		out, hasOut, err = tv.toJSON(r)
		return nil, err
	})
	if cerr != nil {
		re := toRunError(cerr)
		v.addLogLine("warn", fmt.Sprintf("filter %s (%s: %s:%d) ถูกข้าม: %s", name, h.unit, f.at.File, f.at.Line, re.Error()))
		return value
	}
	if !hasOut {
		return goja.Null()
	}
	res, err := v.parse(goja.Undefined(), v.rt.ToValue(out))
	if err != nil {
		return value
	}
	return res
}

// ---------------------------------------------------------------- the visitor across units

// visitorUnit is the unit that registered site.visitor ("" = none): Site
// Code (s's program) or an active plugin.
func (e *Engine) visitorUnit(s *Session) string {
	var site *Program
	var set *unitSet
	if s != nil {
		site, set = s.sp(), s.plugins()
	} else {
		if lp := e.live.Load(); lp != nil {
			site = lp.prog
		}
		set = e.units()
	}
	if site != nil && site.hasVisitor {
		return SiteUnit
	}
	for _, lp := range set.loaded() {
		if lp.prog.hasVisitor {
			return lp.unit.ID
		}
	}
	return ""
}

// ---------------------------------------------------------------- helpers

// attachSession gives a VM serving a route / event / job a request session
// (other units' VMs for exports, filters and the visitor). The caller holds
// the VM slot.
func (e *Engine) attachSession(v *vm, r *http.Request, user *User) *Session {
	s := &Session{e: e, r: r, user: user, held: true, lazy: true, vis: v.vis}
	if s.vis == nil {
		s.vis = &visitorState{}
		v.vis = s.vis
	}
	s.own(v)
	v.sess = s
	return s
}

func unitEntries(list []LogEntry, u *unitInfo) []LogEntry {
	for i := range list {
		list[i].Unit = u.id()
	}
	return list
}

func unitEntry(l LogEntry, u *unitInfo) LogEntry {
	l.Unit = u.id()
	return l
}

// pluginKey returns the JS plugin a script_settings key belongs to
// ("<id>.<key>"), "" for a Site Code key.
func (e *Engine) pluginKey(k string) string {
	id, _, ok := strings.Cut(k, ".")
	if !ok {
		return ""
	}
	if set := e.units(); set != nil {
		if _, ok := set.refs[id]; ok {
			return id
		}
	}
	return ""
}

// ---------------------------------------------------------------- plugins.has / plugins.version (phase 6)

// pluginState is the plugin manager's view of installed packages (optional
// on PluginHost).
type pluginState interface {
	PluginState(id string) (version string, active, ok bool)
}

// jsRef finds an installed JS plugin.
func (e *Engine) jsRef(id string) (plugin.CodeUnitRef, bool) {
	if e.cfg.Plugins == nil {
		return plugin.CodeUnitRef{}, false
	}
	for _, r := range e.cfg.Plugins.CodeUnits() {
		if r.ID == id {
			return r, true
		}
	}
	return plugin.CodeUnitRef{}, false
}

// pluginVersion is the version of an installed, active plugin that runs
// ("" otherwise): a JS plugin must also have loaded without errors. It
// never rebuilds the units (safe during registration).
func (e *Engine) pluginVersion(id string) string {
	if e == nil || e.cfg.Plugins == nil || !unitIDRe.MatchString(id) {
		return ""
	}
	if r, ok := e.jsRef(id); ok {
		if !r.Active {
			return ""
		}
		if s := e.unitSetP.Load(); s != nil && s.errs[id] != "" {
			return ""
		}
		return r.Version
	}
	ps, ok := e.cfg.Plugins.(pluginState)
	if !ok {
		return ""
	}
	ver, active, ok := ps.PluginState(id)
	if !ok || !active {
		return ""
	}
	return ver
}

func (e *Engine) pluginActive(id string) bool {
	if _, ok := e.cfg.Plugins.(pluginState); !ok {
		return true // a host without package state (tests): let the call decide
	}
	return e.pluginVersion(id) != ""
}

// unavailable throws PluginUnavailable (error.code === 'unavailable').
func (v *vm) unavailable(id string) {
	msg := fmt.Sprintf("PluginUnavailable: ปลั๊กอิน %s ไม่ได้ติดตั้งหรือไม่ได้เปิดใช้งาน", id)
	ctor, _ := goja.AssertConstructor(v.rt.Get("Error"))
	if ctor != nil {
		if obj, err := ctor(nil, v.rt.ToValue(msg)); err == nil {
			_ = obj.Set("name", "PluginUnavailable")
			_ = obj.Set("code", "unavailable")
			_ = obj.Set("plugin", id)
			panic(obj)
		}
	}
	v.throw("%s", msg)
}

// pluginsHas is plugins.has(id): the plugin is installed, active and runs.
func (v *vm) pluginsHas(c goja.FunctionCall) goja.Value {
	id, _ := c.Argument(0).Export().(string)
	if v.h == nil || v.h.e == nil {
		return v.rt.ToValue(false)
	}
	return v.rt.ToValue(v.h.e.pluginVersion(id) != "")
}

// pluginsVersion is plugins.version(id): its version, or null.
func (v *vm) pluginsVersion(c goja.FunctionCall) goja.Value {
	id, _ := c.Argument(0).Export().(string)
	if v.h == nil || v.h.e == nil {
		return goja.Null()
	}
	if ver := v.h.e.pluginVersion(id); ver != "" {
		return v.rt.ToValue(ver)
	}
	return goja.Null()
}
