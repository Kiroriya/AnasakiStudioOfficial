package sitecode

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/plugin"
)

// Program is a compiled bundle plus what its registration run declared.
type Program struct {
	unit     *unitInfo
	prog     *goja.Program
	Manifest Manifest
	specs    []*routeSpec
	byKey    map[string]int // route pattern → index in specs/access
	access   [][]string
	events   map[string]bool

	// phase 3: site.collection / site.action / site.page / site.formatter
	collections map[string]*CollectionInfo
	actions     map[string]*ActionInfo
	formatters  map[string]bool
	pageInfo    []PageInfo
	pageSpecs   []*routeSpec
	pageMux     *http.ServeMux

	// phase 4a
	routeOpts   []routeOpt
	filterNames map[string]bool
	hasVisitor  bool
	postTypes   map[string]*PostTypeInfo
	fields      map[string][]FieldInfo
	jobs        map[string]*JobInfo
	adminPages  map[string]*AdminPageInfo

	// phase 4b
	exports    map[string]bool
	filterList []filterMeta // site.filter in registration order
	eventList  []filterMeta // site.on in registration order

	// phase 6: page hooks (registration order; middleware by priority)
	middlewares []filterMeta
	afterN      int
	headN       int
	bodyEndN    int

	// phase 7: admin notices (access per handler), dashboard widgets, blocks
	noticeAccess [][]string
	widgetList   []WidgetInfo
	blocks       map[string]*CodeBlockInfo
	blockList    []*CodeBlockInfo
}

// filterMeta is a registered filter / event handler (hooks screen, order).
type filterMeta struct {
	Name     string
	Priority int
	At       Problem
}

// routeOpt are the body options of a route.
type routeOpt struct {
	rawBody bool
	maxBody int
}

// Register compiles a bundle and runs it once in a fresh VM where only
// site.route / site.on / log work, then validates what it registered
// (duplicate routes, reserved paths, conflicting patterns). Problems carry
// the original file:line.
func Register(bundle string, reserved map[string]bool, h *host, timeout time.Duration) (*Program, []Problem) {
	return RegisterUnit(bundle, reserved, h, timeout, nil)
}

// RegisterUnit is Register for a code unit (nil = Site Code).
func RegisterUnit(bundle string, reserved map[string]bool, h *host, timeout time.Duration, u *unitInfo) (*Program, []Problem) {
	prog, err := goja.Compile("bundle.js", bundle, false)
	if err != nil {
		return nil, []Problem{{Message: "คอมไพล์โค้ดไม่สำเร็จ: " + err.Error()}}
	}
	v := newUnitVM(h, u)
	if err := v.load(prog, modeRegister, timeout); err != nil {
		re := v.runErr(err)
		return nil, []Problem{{File: re.At.File, Line: re.At.Line, Column: re.At.Column, Message: re.Message}}
	}
	p := &Program{unit: v.u, prog: prog, byKey: map[string]int{}, events: map[string]bool{}}
	var probs []Problem
	shapes := map[string]Problem{}
	for _, r := range v.routes {
		key := r.spec.Method + " " + r.spec.Shape
		if prev, dup := shapes[key]; dup {
			probs = append(probs, at(r.at, fmt.Sprintf("route \"%s %s\" ซ้ำกับที่ลงทะเบียนไว้แล้ว (%s:%d)", r.spec.Method, r.spec.Path, prev.File, prev.Line)))
			continue
		}
		shapes[key] = r.at
		if seg := r.spec.firstSegment(); reserved[seg] {
			probs = append(probs, at(r.at, fmt.Sprintf("route \"%s %s\": path /%s ถูกสงวนไว้โดยระบบ", r.spec.Method, r.spec.Path, seg)))
			continue
		}
		p.byKey[r.spec.Pattern] = len(p.specs)
		p.specs = append(p.specs, r.spec)
		p.access = append(p.access, r.access)
		p.routeOpts = append(p.routeOpts, routeOpt{rawBody: r.rawBody, maxBody: r.maxBody})
		p.Manifest.Routes = append(p.Manifest.Routes, RouteInfo{Method: r.spec.Method, Path: r.spec.Path, Access: r.access, RawBody: r.rawBody, MaxBody: r.maxBody})
	}
	if _, err := newMux(p.specs, func(int) http.Handler { return http.NotFoundHandler() }); err != nil {
		probs = append(probs, Problem{Message: err.Error()})
	}
	for _, e := range v.events {
		p.eventList = append(p.eventList, filterMeta{Name: e.name, Priority: defaultPriority, At: e.at})
		if !p.events[e.name] {
			p.events[e.name] = true
			p.Manifest.Events = append(p.Manifest.Events, e.name)
		}
	}
	sort.Strings(p.Manifest.Events)
	probs = append(probs, p.registerBindings(v)...)
	probs = append(probs, p.registerHooks(v)...)
	probs = append(probs, p.registerContent(v, reserved)...)
	probs = append(probs, p.registerTaxonomies(v, reserved)...)
	probs = append(probs, p.registerJobs(v)...)
	probs = append(probs, p.registerAdminPages(v)...)
	probs = append(probs, p.registerExports(v)...)
	p.registerPageHooks(v)
	probs = append(probs, p.registerJoints7(v)...)
	p.Manifest.normalize()
	if len(probs) > 0 {
		return nil, probs
	}
	return p, nil
}

func at(p Problem, msg string) Problem { p.Message = msg; return p }

// ---------------------------------------------------------------- request → JS

// ctxValue builds the ctx argument of a route handler.
func (v *vm) ctxValue(r *http.Request, params map[string]string) goja.Value {
	v.req = r
	ctx := v.rt.NewObject()
	_ = ctx.Set("request", v.requestValueP(r, params))
	_ = ctx.Set("response", v.responseValue(r))
	_ = ctx.Set("user", v.userValue())
	v.defineVisitor(ctx)
	return ctx
}

// requestValue is ctx.request without path parameters (site.visitor).
func (v *vm) requestValue(r *http.Request) goja.Value { return v.requestValueP(r, map[string]string{}) }

// readBody reads the request body once (up to the route's maxBody).
func (v *vm) readBody(r *http.Request) []byte {
	if v.bodyReq == r {
		return v.body
	}
	v.bodyReq, v.body = r, nil
	if r.Body != nil {
		limit := v.maxBody
		if limit <= 0 {
			limit = maxBody
		}
		v.body, _ = io.ReadAll(io.LimitReader(r.Body, int64(limit)))
	}
	return v.body
}

func (v *vm) requestValueP(r *http.Request, params map[string]string) goja.Value {
	q := map[string]any{}
	for k, vals := range r.URL.Query() {
		if len(vals) == 1 {
			q[k] = vals[0]
		} else {
			q[k] = vals
		}
	}
	hdr := map[string]string{}
	for k, vals := range r.Header {
		lk := strings.ToLower(k)
		if lk == "authorization" { // the admin's session token stays out of scripts
			continue
		}
		hdr[lk] = strings.Join(vals, ", ")
	}
	cookies := map[string]string{} // only the unit's own cookies (prefix pl_site_ / pl_<id>_, stripped)
	pre := v.u.cookiePrefix()
	for _, c := range r.Cookies() {
		if n, ok := strings.CutPrefix(c.Name, pre); ok && n != "" {
			cookies[n] = c.Value
		}
	}
	raw := v.readBody(r)
	body := string(raw)
	m := map[string]any{"method": r.Method, "path": r.URL.Path, "params": params, "query": q,
		"headers": hdr, "cookies": cookies, "body": body,
		// phase 7: client address (same rules as middleware) and host
		"ip": plugin.ClientIP(r), "host": plugin.RequestHost(r)}
	if v.rawBody {
		m["rawBody"] = body
		m["rawBodyBase64"] = base64.StdEncoding.EncodeToString(raw)
	}
	req := v.toJS(m).ToObject(v.rt)
	_ = req.Set("files", v.requestFiles(r)) // multipart bodies (2.1.0)
	_ = req.Set("form", v.requestForm(r))
	_ = req.Set("json", func(goja.FunctionCall) goja.Value {
		out, err := v.parse(goja.Undefined(), v.rt.ToValue(body))
		if err != nil {
			v.throw("request.json(): body ไม่ใช่ JSON ที่ถูกต้อง")
		}
		return out
	})
	return req
}

// Response is what a route produced.
type Response struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Cookies []string          `json:"cookies,omitempty"` // Set-Cookie values (response.setCookie)
	// Page: render Page.HTML inside the site's layout (result {page}, phase 7).
	Page *PageResult `json:"page,omitempty"`
	// Binary: Body holds raw bytes (a Uint8Array / ArrayBuffer body or
	// bodyBase64, phase 6), written verbatim.
	Binary bool `json:"binary,omitempty"`
}

// MaxBinaryBody caps a binary response body (route, webhook reply, admin
// page download).
const MaxBinaryBody = 20 << 20

// binaryOf returns the bytes of a Uint8Array / ArrayBuffer (a copy).
func binaryOf(x goja.Value) ([]byte, bool) {
	if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
		return nil, false
	}
	o, ok := x.(*goja.Object)
	if !ok {
		return nil, false
	}
	switch b := o.Export().(type) {
	case goja.ArrayBuffer:
		return append([]byte(nil), b.Bytes()...), true
	case []byte:
		// a Uint8Array (other typed arrays export other slice types)
		return append([]byte(nil), b...), true
	}
	return nil, false
}

// setBinary stores b as the response body (default content type
// application/octet-stream).
func (res *Response) setBinary(b []byte) error {
	if len(b) > MaxBinaryBody {
		return &RunError{Message: fmt.Sprintf("body ใหญ่เกิน %d MB", MaxBinaryBody>>20)}
	}
	res.Body, res.Binary = string(b), true
	if _, ok := res.Headers["Content-Type"]; !ok {
		res.Headers["Content-Type"] = "application/octet-stream"
	}
	return nil
}

// dropped response headers (the server manages them).
var hopHeaders = map[string]bool{"content-length": true, "transfer-encoding": true, "connection": true,
	"keep-alive": true, "upgrade": true, "trailer": true}

// response converts a handler's return value: object/array → JSON,
// string → HTML, {status, headers, body} → as given, nothing → 204.
func (v *vm) response(val goja.Value) (*Response, error) {
	res := &Response{Status: http.StatusOK, Headers: map[string]string{}}
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		res.Status = http.StatusNoContent
		return res, nil
	}
	if s, ok := val.Export().(string); ok {
		res.Headers["Content-Type"] = "text/html; charset=utf-8"
		res.Body = s
		return res, nil
	}
	if b, ok := binaryOf(val); ok {
		return res, res.setBinary(b)
	}
	if o, ok := val.(*goja.Object); ok && o.ClassName() == "Object" && isPageShape(o) {
		return v.pageResponse(o)
	}
	if o, ok := val.(*goja.Object); ok && o.ClassName() == "Object" && isResponseShape(o) {
		st := o.Get("status").ToInteger()
		if st < 100 || st > 599 {
			return nil, &RunError{Message: fmt.Sprintf("status %d ไม่ถูกต้อง (ต้องอยู่ระหว่าง 100-599)", st)}
		}
		res.Status = int(st)
		if h := o.Get("headers"); h != nil && !goja.IsUndefined(h) && !goja.IsNull(h) {
			ho := h.ToObject(v.rt)
			for _, k := range ho.Keys() {
				if hopHeaders[strings.ToLower(k)] || strings.ContainsAny(k, "\r\n:") {
					continue
				}
				hv := ho.Get(k).String()
				if strings.ContainsAny(hv, "\r\n") {
					continue
				}
				res.Headers[http.CanonicalHeaderKey(k)] = hv
			}
		}
		body := o.Get("body")
		if b64 := o.Get("bodyBase64"); b64 != nil && !goja.IsUndefined(b64) && !goja.IsNull(b64) {
			if body != nil && !goja.IsUndefined(body) && !goja.IsNull(body) {
				return nil, &RunError{Message: "ใช้ body หรือ bodyBase64 อย่างใดอย่างหนึ่ง"}
			}
			s, _ := b64.Export().(string)
			if len(s) > MaxBinaryBody/3*4+8 {
				return nil, &RunError{Message: fmt.Sprintf("body ใหญ่เกิน %d MB", MaxBinaryBody>>20)}
			}
			raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
			if err != nil {
				return nil, &RunError{Message: "bodyBase64 ไม่ใช่ base64 ที่ถูกต้อง"}
			}
			return res, res.setBinary(raw)
		}
		if b, ok := binaryOf(body); ok {
			return res, res.setBinary(b)
		}
		switch {
		case body == nil || goja.IsUndefined(body) || goja.IsNull(body):
		case isString(body):
			res.Body = body.String()
			if _, ok := res.Headers["Content-Type"]; !ok {
				res.Headers["Content-Type"] = "text/html; charset=utf-8"
			}
		default:
			s, ok, err := v.toJSON(body)
			if err != nil {
				return nil, v.runErr(err)
			}
			if ok {
				res.Body = s
			}
			if _, ok := res.Headers["Content-Type"]; !ok {
				res.Headers["Content-Type"] = "application/json; charset=utf-8"
			}
		}
		return res, nil
	}
	s, ok, err := v.toJSON(val)
	if err != nil {
		return nil, v.runErr(err)
	}
	if !ok {
		res.Status = http.StatusNoContent
		return res, nil
	}
	res.Headers["Content-Type"] = "application/json; charset=utf-8"
	res.Body = s
	return res, nil
}

func isString(x goja.Value) bool {
	_, ok := x.Export().(string)
	return ok
}

// isResponseShape: a numeric status and no keys besides status/headers/body.
func isResponseShape(o *goja.Object) bool {
	st := o.Get("status")
	if st == nil {
		return false
	}
	switch st.Export().(type) {
	case int64, float64:
	default:
		return false
	}
	for _, k := range o.Keys() {
		if k != "status" && k != "headers" && k != "body" && k != "bodyBase64" {
			return false
		}
	}
	return true
}

// allowed checks an access list against the caller: '*', role slugs and
// "cap:<capability>" entries (the administrator always passes).
func allowed(access []string, u *User) bool {
	for _, a := range access {
		if a == "*" {
			return true
		}
	}
	if u == nil {
		return false
	}
	if u.Role == "administrator" {
		return true
	}
	for _, a := range access {
		if a == u.Role {
			return true
		}
		// "cap:<capability>" (2.0.6): any role holding it, as set in Users → Roles
		if c, ok := capEntry(a); ok && auth.RoleHas(u.Role, c) {
			return true
		}
	}
	return false
}
