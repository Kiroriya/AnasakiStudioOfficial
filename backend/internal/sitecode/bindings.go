package sitecode

import (
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// Phase 3: what the designer binds to — site.collection (data for
// collection nodes), site.action (buttons / forms), site.page ({{page.*}})
// and site.formatter (| name in bindings).

// CollectionInfo is a registered site.collection (manifest).
type CollectionInfo struct {
	Name    string            `json:"name"`
	Label   any               `json:"label"`
	Fields  map[string]string `json:"fields"`
	Params  map[string]string `json:"params"`
	PerPage PerPageInfo       `json:"perPage"`
	Cache   int               `json:"cache"`
	Access  []string          `json:"access"`
}

// PerPageInfo is a collection's page size: default and maximum.
type PerPageInfo struct {
	Default int `json:"default"`
	Max     int `json:"max"`
}

// ActionInfo is a registered site.action.
type ActionInfo struct {
	Name   string            `json:"name"`
	Input  map[string]string `json:"input"`
	Access []string          `json:"access"`
}

// PageInfo is a registered site.page.
type PageInfo struct {
	Path   string   `json:"path"`
	Access []string `json:"access"`
}

type regCollection struct {
	info CollectionInfo
	fn   goja.Callable
	at   Problem
}

type regAction struct {
	info ActionInfo
	fn   goja.Callable
	at   Problem
}

type regPage struct {
	info PageInfo
	spec *routeSpec
	fn   goja.Callable
	at   Problem
}

type regFormatter struct {
	name string
	fn   goja.Callable
	at   Problem
}

// Limits of phase 3.
const (
	maxBindings       = 200
	hardPerPage       = 100
	FormatterTimeout  = 50 * time.Millisecond
	maxActionInputLen = 10000
)

var (
	bindingNameRe = design.ActionNameRe // ^[a-z][a-zA-Z0-9_-]{0,62}$
	fieldNameRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
	fieldTypes    = map[string]bool{"text": true, "number": true, "date": true, "datetime": true, "image": true, "url": true, "html": true, "bool": true, "json": true}
	paramTypes    = map[string]bool{"text": true, "number": true, "bool": true}
	inputTypes    = map[string]bool{"text": true, "text?": true, "number": true, "number?": true, "bool": true, "email": true, "email?": true, "json": true, "json?": true}
	cookieNameRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// CookiePrefix namespaces the cookies scripts read and set.
const CookiePrefix = "pl_site_"

// ---------------------------------------------------------------- registration

func (v *vm) regName(kind string, c goja.FunctionCall) (string, *goja.Object) {
	if v.mode == modeHandler {
		v.throw("site.%s ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler", kind)
	}
	name, _ := c.Argument(0).Export().(string)
	if !bindingNameRe.MatchString(name) {
		v.throw("site.%s: ชื่อ %q ไม่ถูกต้อง (a-z ตัวแรก ตามด้วย a-z A-Z 0-9 _ - ไม่เกิน 63 ตัว)", kind, c.Argument(0).String())
	}
	if len(v.colls)+len(v.acts)+len(v.pages)+len(v.fmts) >= maxBindings {
		v.throw("ลงทะเบียน collection/action/page/formatter รวมได้ไม่เกิน %d รายการ", maxBindings)
	}
	return name, optObject(v.rt, c.Argument(1))
}

func (v *vm) accessOpt(o *goja.Object, what string, def []string) []string {
	if o == nil {
		return def
	}
	a := o.Get("access")
	if a == nil || goja.IsUndefined(a) || goja.IsNull(a) {
		return def
	}
	var list []string
	if err := v.rt.ExportTo(a, &list); err != nil {
		v.throw("%s: access ต้องเป็น array ของข้อความ เช่น ['*'] หรือ ['editor']", what)
	}
	if len(list) == 0 {
		return def
	}
	acc, err := checkAccess(list)
	if err != nil {
		v.throw("%s: %s", what, err.Error())
	}
	return acc
}

// typeMap reads {name: 'type'} with allowed types.
func (v *vm) typeMap(o *goja.Object, key, what string, allowed map[string]bool) map[string]string {
	out := map[string]string{}
	if o == nil {
		return out
	}
	x := o.Get(key)
	if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
		return out
	}
	m := x.ToObject(v.rt)
	for _, k := range m.Keys() {
		t, _ := m.Get(k).Export().(string)
		if !fieldNameRe.MatchString(k) {
			v.throw("%s: ชื่อ %s %q ไม่ถูกต้อง", what, key, k)
		}
		if !allowed[t] {
			names := make([]string, 0, len(allowed))
			for a := range allowed {
				names = append(names, "'"+a+"'")
			}
			sort.Strings(names)
			v.throw("%s: %s.%s ต้องเป็น %s", what, key, k, strings.Join(names, ", "))
		}
		out[k] = t
	}
	return out
}

func (v *vm) fnOpt(o *goja.Object, key, what string) goja.Callable {
	if o == nil {
		v.throw("%s: ต้องมี %s เป็นฟังก์ชัน", what, key)
	}
	fn, ok := goja.AssertFunction(o.Get(key))
	if !ok {
		v.throw("%s: ต้องมี %s เป็นฟังก์ชัน", what, key)
	}
	return fn
}

func (v *vm) siteCollection(c goja.FunctionCall) goja.Value {
	name, o := v.regName("collection", c)
	what := fmt.Sprintf("site.collection(%q)", name)
	info := CollectionInfo{Name: name, Label: name, PerPage: PerPageInfo{Default: 10, Max: 50}}
	if o != nil {
		if l := o.Get("label"); l != nil && !goja.IsUndefined(l) && !goja.IsNull(l) {
			switch x := l.Export().(type) {
			case string:
				info.Label = x
			case map[string]any:
				lm := map[string]string{}
				for _, k := range []string{"th", "en"} {
					if s, ok := x[k].(string); ok {
						lm[k] = s
					}
				}
				info.Label = lm
			default:
				v.throw("%s: label ต้องเป็นข้อความหรือ {th, en}", what)
			}
		}
		if pp := optObject(v.rt, o.Get("perPage")); pp != nil {
			if d := pp.Get("default"); d != nil && !goja.IsUndefined(d) {
				info.PerPage.Default = int(d.ToInteger())
			}
			if m := pp.Get("max"); m != nil && !goja.IsUndefined(m) {
				info.PerPage.Max = int(m.ToInteger())
			}
		}
		if ca := o.Get("cache"); ca != nil && !goja.IsUndefined(ca) && !goja.IsNull(ca) {
			info.Cache = int(ca.ToInteger())
			if info.Cache < 0 || info.Cache > 86400 {
				v.throw("%s: cache ต้องอยู่ระหว่าง 0 ถึง 86400 วินาที", what)
			}
		}
	}
	if info.PerPage.Max < 1 || info.PerPage.Max > hardPerPage {
		v.throw("%s: perPage.max ต้องอยู่ระหว่าง 1 ถึง %d", what, hardPerPage)
	}
	if info.PerPage.Default < 1 || info.PerPage.Default > info.PerPage.Max {
		v.throw("%s: perPage.default ต้องอยู่ระหว่าง 1 ถึง perPage.max", what)
	}
	info.Fields = v.typeMap(o, "fields", what, fieldTypes)
	info.Params = v.typeMap(o, "params", what, paramTypes)
	info.Access = v.accessOpt(o, what, []string{"administrator"})
	fn := v.fnOpt(o, "load", what)
	v.colls = append(v.colls, &regCollection{info: info, fn: fn, at: v.caller()})
	return goja.Undefined()
}

func (v *vm) siteAction(c goja.FunctionCall) goja.Value {
	name, o := v.regName("action", c)
	what := fmt.Sprintf("site.action(%q)", name)
	info := ActionInfo{Name: name, Input: v.typeMap(o, "input", what, inputTypes)}
	info.Access = v.accessOpt(o, what, []string{"administrator"})
	fn := v.fnOpt(o, "run", what)
	v.acts = append(v.acts, &regAction{info: info, fn: fn, at: v.caller()})
	return goja.Undefined()
}

func (v *vm) sitePage(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.page ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	path, _ := c.Argument(0).Export().(string)
	what := fmt.Sprintf("site.page(%q)", path)
	if len(v.colls)+len(v.acts)+len(v.pages)+len(v.fmts) >= maxBindings {
		v.throw("ลงทะเบียน collection/action/page/formatter รวมได้ไม่เกิน %d รายการ", maxBindings)
	}
	rs, err := parseSpec("GET " + strings.TrimSpace(path))
	if err != nil || strings.Contains(path, " ") {
		msg := "path ต้องขึ้นต้นด้วย / เช่น \"/checkout\" หรือ \"/events/{slug}\""
		if err != nil {
			msg = strings.Replace(err.Error(), "route ", "page ", 1)
		}
		v.throw("%s: %s", what, msg)
	}
	o := optObject(v.rt, c.Argument(1))
	info := PageInfo{Path: rs.Path, Access: v.accessOpt(o, what, []string{"*"})}
	fn := v.fnOpt(o, "load", what)
	v.pages = append(v.pages, &regPage{info: info, spec: rs, fn: fn, at: v.caller()})
	return goja.Undefined()
}

func (v *vm) siteFormatter(c goja.FunctionCall) goja.Value {
	// phase 6: a JS plugin's formatter is "<id>:<name>" in expressions
	// ({{ price | thai:baht }}); the administrator may enable an unprefixed alias.
	name, _ := v.regName("formatter", c)
	fn, ok := goja.AssertFunction(c.Argument(1))
	if !ok {
		v.throw("site.formatter(%q): ต้องมีฟังก์ชัน (value, ...args) => string", name)
	}
	v.fmts = append(v.fmts, &regFormatter{name: name, fn: fn, at: v.caller()})
	return goja.Undefined()
}

// registerBindings validates the phase-3 registrations of a registration
// run and adds them to the program and its manifest.
func (p *Program) registerBindings(v *vm) []Problem {
	var probs []Problem
	seen := map[string]Problem{}
	dup := func(kind, name string, at Problem) bool {
		k := kind + "\x00" + name
		if prev, ok := seen[k]; ok {
			probs = append(probs, Problem{File: at.File, Line: at.Line, Column: at.Column,
				Message: fmt.Sprintf("site.%s %q ซ้ำกับที่ลงทะเบียนไว้แล้ว (%s:%d)", kind, name, prev.File, prev.Line)})
			return true
		}
		seen[k] = at
		return false
	}
	p.collections, p.actions, p.formatters = map[string]*CollectionInfo{}, map[string]*ActionInfo{}, map[string]bool{}
	for _, c := range v.colls {
		if dup("collection", c.info.Name, c.at) {
			continue
		}
		info := c.info
		p.collections[info.Name] = &info
		p.Manifest.Collections = append(p.Manifest.Collections, info)
	}
	for _, a := range v.acts {
		if dup("action", a.info.Name, a.at) {
			continue
		}
		info := a.info
		p.actions[info.Name] = &info
		p.Manifest.Actions = append(p.Manifest.Actions, info)
	}
	var specs []*routeSpec
	for _, pg := range v.pages {
		if dup("page", pg.spec.Shape, pg.at) {
			continue
		}
		p.pageInfo = append(p.pageInfo, pg.info)
		specs = append(specs, pg.spec)
		p.Manifest.Pages = append(p.Manifest.Pages, pg.info)
	}
	if len(specs) > 0 {
		mux, err := newMux(specs, func(int) http.Handler { return http.NotFoundHandler() })
		if err != nil {
			probs = append(probs, Problem{Message: strings.Replace(err.Error(), "route", "page", 1)})
		}
		p.pageMux, p.pageSpecs = mux, specs
	}
	for _, f := range v.fmts {
		if dup("formatter", f.name, f.at) {
			continue
		}
		p.formatters[f.name] = true
		p.Manifest.Formatters = append(p.Manifest.Formatters, f.name)
	}
	sort.Strings(p.Manifest.Formatters)
	return probs
}

// matchPage finds the site.page pattern matching path (-1 when none).
func (p *Program) matchPage(path string) (int, map[string]string) {
	if p == nil || p.pageMux == nil {
		return -1, nil
	}
	req, err := http.NewRequest(http.MethodGet, "http://site.local"+path, nil)
	if err != nil {
		return -1, nil
	}
	_, pattern := p.pageMux.Handler(req)
	if pattern == "" {
		return -1, nil
	}
	for i, rs := range p.pageSpecs {
		if rs.Pattern == pattern {
			out := map[string]string{}
			mux := http.NewServeMux()
			mux.HandleFunc(rs.Pattern, func(_ http.ResponseWriter, r *http.Request) { out = params(rs, r) })
			mux.ServeHTTP(&nopWriter{h: http.Header{}}, req)
			return i, out
		}
	}
	return -1, nil
}

// ---------------------------------------------------------------- values in and out

// coerceParams keeps the declared params with their types.
func coerceParams(decl map[string]string, in map[string]any) map[string]any {
	out := map[string]any{}
	for k, t := range decl {
		v, ok := in[k]
		if !ok || v == nil {
			continue
		}
		switch t {
		case "number":
			s := design.Stringify(v)
			if strings.TrimSpace(s) == "" {
				continue
			}
			f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
			if err != nil {
				continue
			}
			out[k] = f
		case "bool":
			out[k] = truthyInput(v)
		default:
			s := design.Stringify(v)
			if len(s) > 1000 {
				s = clip(s, 1000)
			}
			out[k] = s
		}
	}
	return out
}

func truthyInput(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "on", "yes":
			return true
		}
		return false
	}
	return design.Truthy(v)
}

// validateInput checks an action's input against its declaration: unknown
// keys are dropped, required ones must be present.
func validateInput(decl map[string]string, in map[string]any) (map[string]any, map[string]string) {
	out, errs := map[string]any{}, map[string]string{}
	for k, t := range decl {
		optional := strings.HasSuffix(t, "?")
		t = strings.TrimSuffix(t, "?")
		v, present := in[k]
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" && t != "text" && t != "json" {
			present, v = false, nil
		}
		if s, ok := v.(string); ok && t == "text" && s == "" {
			present, v = false, nil
		}
		if !present || v == nil {
			switch {
			case t == "bool":
				out[k] = false
			case optional:
				out[k] = nil
			default:
				errs[k] = "กรุณากรอกข้อมูลช่องนี้"
			}
			continue
		}
		switch t {
		case "text":
			var s string
			switch x := v.(type) {
			case string:
				s = x
			case float64, bool:
				s = design.Stringify(x)
			default:
				errs[k] = "ต้องเป็นข้อความ"
				continue
			}
			if len(s) > maxActionInputLen {
				errs[k] = fmt.Sprintf("ยาวเกิน %d ตัวอักษร", maxActionInputLen)
				continue
			}
			out[k] = s
		case "number":
			switch x := v.(type) {
			case float64:
				out[k] = x
			case string:
				f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
				if err != nil {
					errs[k] = "ต้องเป็นตัวเลข"
					continue
				}
				out[k] = f
			default:
				errs[k] = "ต้องเป็นตัวเลข"
			}
		case "bool":
			out[k] = truthyInput(v)
		case "email":
			s, _ := v.(string)
			s = strings.TrimSpace(s)
			if a, err := mail.ParseAddress(s); err != nil || a.Address != s || !strings.Contains(s[strings.LastIndex(s, "@")+1:], ".") || len(s) > 254 {
				errs[k] = "อีเมลไม่ถูกต้อง"
				continue
			}
			out[k] = s
		case "json":
			out[k] = v
		}
	}
	return out, errs
}

// cookieHeader builds the Set-Cookie value of response.setCookie.
func cookieHeader(r *http.Request, prefix, name, value string, maxAge int, httpOnly bool, sameSite, path string) string {
	c := &http.Cookie{Name: prefix + name, Value: value, Path: path, HttpOnly: httpOnly, MaxAge: maxAge}
	switch strings.ToLower(sameSite) {
	case "strict":
		c.SameSite = http.SameSiteStrictMode
	case "none":
		c.SameSite = http.SameSiteNoneMode
		c.Secure = true
	default:
		c.SameSite = http.SameSiteLaxMode
	}
	if r != nil && plugin.RequestScheme(r) == "https" {
		c.Secure = true
	}
	return c.String()
}

// responseValue is ctx.response: setCookie(name, value, opts).
func (v *vm) responseValue(r *http.Request) goja.Value {
	o := v.rt.NewObject()
	_ = o.Set("setCookie", func(c goja.FunctionCall) goja.Value {
		name, _ := c.Argument(0).Export().(string)
		if !cookieNameRe.MatchString(name) {
			v.throw("response.setCookie: ชื่อ cookie ต้องเป็น A-Z a-z 0-9 _ - (ไม่เกิน 64 ตัว)")
		}
		val := ""
		if x := c.Argument(1); !goja.IsUndefined(x) && !goja.IsNull(x) {
			val = x.String()
		}
		if len(val) > 4000 {
			v.throw("response.setCookie: ค่ายาวเกิน 4000 ตัวอักษร")
		}
		maxAge, httpOnly, sameSite, path := 0, true, "Lax", "/"
		if o := optObject(v.rt, c.Argument(2)); o != nil {
			if m := o.Get("maxAge"); m != nil && !goja.IsUndefined(m) && !goja.IsNull(m) {
				maxAge = int(m.ToInteger())
				if maxAge == 0 {
					maxAge = -1 // maxAge: 0 deletes the cookie
				}
			}
			if h := o.Get("httpOnly"); h != nil && !goja.IsUndefined(h) {
				httpOnly = h.ToBoolean()
			}
			if s := optString(o, "sameSite"); s != "" {
				sameSite = s
			}
			if p := optString(o, "path"); p != "" {
				if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, ";\r\n ") {
					v.throw("response.setCookie: path ต้องขึ้นต้นด้วย /")
				}
				path = p
			}
		}
		if len(v.cookies) >= 20 {
			v.throw("response.setCookie: ตั้ง cookie ได้ไม่เกิน 20 ตัวต่อครั้ง")
		}
		v.cookies = append(v.cookies, cookieHeader(r, v.u.cookiePrefix(), name, val, maxAge, httpOnly, sameSite, path))
		return goja.Undefined()
	})
	_ = o.Set("page", v.responsePage) // phase 7: {page: {title, html}} in the site's layout
	return o
}
