package sitecode

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"golang.org/x/crypto/bcrypt"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/kses"
	"github.com/anasaki/studio/backend/internal/model"
)

// PostStore is what db.posts reads (store.Store).
type PostStore interface {
	ListPosts(f model.PostFilter) ([]*model.Post, int, error)
	GetPost(id int64) (*model.Post, error)
	PostBySlug(typ, slug string, publishedOnly bool) (*model.Post, error)
}

// User is the CMS user a script sees as ctx.user.
type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
	// Locale is the user's admin language (ctx.locale of admin pages).
	Locale string `json:"-"`
}

// Can reports whether the user's role has cap.
func (u *User) Can(cap string) bool { return u != nil && auth.RoleHas(u.Role, cap) }

// LogLine is a log.* / console.* call made during one invocation.
type LogLine struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Per-invocation limits on logging.
const (
	maxLogLines   = 100
	maxLogMessage = 4000
	maxBody       = 1 << 20
	maxRouteBody  = 5 << 20
)

// host is what scripts reach outside the VM.
type host struct {
	cache *Cache
	data  *Data // the site's own database (nil in unit tests)
	posts PostStore
	// URL builds a post's public path (site.PostURL).
	postURL func(p *model.Post) string
	// e is the engine (nil in unit tests): content writes, mail, http.fetch,
	// settings, jobs, events, audit.
	e *Engine
}

// mode of a VM run: registration (only site.route/site.on/log), the top
// level of a live VM (same) or inside a handler (everything).
type vmMode int

const (
	modeRegister vmMode = iota
	modeTop
	modeHandler
)

// regRoute is a site.route call.
type regRoute struct {
	spec    *routeSpec
	access  []string
	fn      goja.Callable
	at      Problem // where it was registered (file:line)
	rawBody bool
	maxBody int
}

type regEvent struct {
	name string
	fn   goja.Callable
	at   Problem
}

// vm is one goja runtime with the Site Code globals. A vm serves a single
// invocation and is then discarded, so nothing leaks between requests
// except what the script puts in cache.
type vm struct {
	rt     *goja.Runtime
	h      *host
	u      *unitInfo // the code unit (Site Code or a JS plugin)
	sess   *Session  // the request session (cross-unit calls), nil at registration
	mode   vmMode
	routes []*regRoute
	events []*regEvent
	errs   []Problem // bad site.route / site.on calls (registration)
	colls  []*regCollection
	acts   []*regAction
	pages  []*regPage
	fmts   []*regFormatter

	// phase 4a registrations
	filters  []*regFilter
	visitors []*regVisitor
	vgroups  []string
	ptypes   []*regPostType
	taxes    []*regTaxonomy // site.taxonomy (2.1.0)
	// request.files() / form(): the parsed multipart body of formReq (2.1.0)
	form    *multipart.Form
	formReq *http.Request
	pfields []*regFields
	jobDefs []*regJob
	crons   []*regCron
	apages  []*regAdminPage
	exps    []*regExport // site.export (phase 4b)
	// phase 6: site.middleware / afterResponse / head / bodyEnd
	mws, afters, heads, bodyEnds []*regHook
	xdepth                       int // nesting of cross-unit calls into this VM
	// phase 7: site.adminNotice / dashboardWidget / block, http.webhook calls
	notices  []*regNotice
	widgets  []*regWidget
	blocks   []*regBlock
	webhooks int

	// per invocation (phase 4a)
	req     *http.Request // the request being served (site.visitor ctx)
	vis     *visitorState // the request's visitor (resolved once)
	system  bool          // inside db.asSystem
	inCall  bool          // a handler is running (nested time limits)
	rawBody bool          // the route asked for request.rawBody
	maxBody int           // body limit of the route (0 = 1 MB)
	bodyReq *http.Request
	body    []byte
	job     *jobRun // the running job (progress)
	source  string  // the running handler ("route GET /x", "job send"): mail.beforeSend ctx

	cookies []string // Set-Cookie values from response.setCookie

	user *User
	logs []LogLine

	deadline time.Time // end of the running handler (db calls respect it)
	inTx     bool
	// curTx is the open db.transaction: inside its function the global db
	// runs on it too (2.5.4), so a script that writes through db.table(…)
	// rather than the tx argument is rolled back with the rest on an error.
	curTx *sql.Tx

	parse, stringify goja.Callable // the original JSON functions
}

var errTimeout = errors.New("timeout")

func newVM(h *host) *vm { return newUnitVM(h, nil) }

// newUnitVM is a VM of unit u (nil = Site Code).
func newUnitVM(h *host, u *unitInfo) *vm {
	if u == nil {
		u = siteUnitInfo
		if h != nil && h.e != nil && h.e.siteU != nil {
			u = h.e.siteU
		}
	}
	rt := goja.New()
	rt.SetMaxCallStackSize(2000)
	v := &vm{rt: rt, h: h, u: u}
	j := rt.Get("JSON").ToObject(rt)
	v.parse, _ = goja.AssertFunction(j.Get("parse"))
	v.stringify, _ = goja.AssertFunction(j.Get("stringify"))
	v.install()
	return v
}

// unit is the VM's code unit.
func (v *vm) unit() *unitInfo { return v.u }

// need checks a code scope of the VM's unit: a JS plugin calling an API
// outside its approved scopes gets "ไม่มีสิทธิ์ใช้ <scope>" (logged).
func (v *vm) need(scope, what string) {
	if v.u.can(scope) {
		return
	}
	msg := fmt.Sprintf("ไม่มีสิทธิ์ใช้ %s (%s — ปลั๊กอิน %s ไม่ได้ขอ scope นี้ใน code.scopes)", scope, what, v.u.ID)
	v.addLogLine("warn", msg)
	v.fail("%s", msg)
}

// throw raises a JS TypeError from a host function.
func (v *vm) throw(format string, a ...any) {
	panic(v.rt.NewTypeError(fmt.Sprintf(format, a...)))
}

// handlerOnly guards APIs that need a running handler.
func (v *vm) handlerOnly(name string) {
	if v.mode != modeHandler {
		v.throw("%s ใช้ได้เฉพาะภายใน handler ของ site.route / site.on (ไม่ใช่ตอนลงทะเบียน)", name)
	}
}

// toJS turns a Go value into a plain JS value (via JSON).
func (v *vm) toJS(x any) goja.Value {
	b, err := json.Marshal(x)
	if err != nil {
		return goja.Null()
	}
	out, err := v.parse(goja.Undefined(), v.rt.ToValue(string(b)))
	if err != nil {
		return goja.Null()
	}
	return out
}

// toJSON serialises a JS value; ok=false for undefined/functions.
func (v *vm) toJSON(x goja.Value) (string, bool, error) {
	out, err := v.stringify(goja.Undefined(), x)
	if err != nil {
		return "", false, err
	}
	if out == nil || goja.IsUndefined(out) {
		return "", false, nil
	}
	return out.String(), true, nil
}

// caller returns where the current host function was called from.
func (v *vm) caller() Problem {
	for _, f := range v.rt.CaptureCallStack(10, nil) {
		p := f.Position()
		if p.Filename != "" && p.Filename != "bundle.js" {
			return Problem{File: p.Filename, Line: p.Line, Column: p.Column}
		}
	}
	return Problem{}
}

func (v *vm) install() {
	rt := v.rt
	obj := func(fns map[string]func(goja.FunctionCall) goja.Value) *goja.Object {
		o := rt.NewObject()
		for k, fn := range fns {
			_ = o.Set(k, fn)
		}
		return o
	}

	_ = rt.Set("site", obj(map[string]func(goja.FunctionCall) goja.Value{
		"route":         v.siteRoute,
		"on":            v.siteOn,
		"collection":    v.siteCollection,
		"action":        v.siteAction,
		"page":          v.sitePage,
		"formatter":     v.siteFormatter,
		"filter":        v.siteFilter,
		"applyFilter":   v.siteApplyFilter,
		"emit":          v.siteEmit,
		"visitor":       v.siteVisitor,
		"visitorGroups": v.siteVisitorGroups,
		"postType":      v.sitePostType,
		"taxonomy":      v.siteTaxonomy,
		"fields":        v.siteFields,
		"job":           v.siteJob,
		"cron":          v.siteCron,
		"adminPage":     v.siteAdminPage,
		"settings":      v.siteSettings,
		"export":        v.siteExport,
		// phase 6
		"asset":         v.siteAsset,
		"middleware":    v.siteMiddleware,
		"afterResponse": v.siteAfterResponse,
		"head":          v.siteHead,
		"bodyEnd":       v.siteBodyEnd,
		// phase 7
		"saveSettings":    v.siteSaveSettings,
		"info":            v.siteInfo,
		"meta":            v.siteMeta,
		"setMeta":         v.siteSetMeta,
		"adminNotice":     v.siteAdminNotice,
		"dashboardWidget": v.siteDashboardWidget,
		"block":           v.siteBlock,
		// 2.0.6: plugin capabilities
		"can": v.siteCan,
	}))
	_ = rt.Set("plugins", obj(map[string]func(goja.FunctionCall) goja.Value{"call": v.pluginsCall, "has": v.pluginsHas, "version": v.pluginsVersion}))
	_ = rt.Set("__anasaki_export", v.exportProxy)

	logFn := func(level string) func(goja.FunctionCall) goja.Value {
		return func(c goja.FunctionCall) goja.Value {
			v.addLog(level, c.Arguments)
			return goja.Undefined()
		}
	}
	_ = rt.Set("log", obj(map[string]func(goja.FunctionCall) goja.Value{
		"debug": logFn("debug"), "info": logFn("info"), "warn": logFn("warn"), "error": logFn("error"),
	}))
	_ = rt.Set("console", obj(map[string]func(goja.FunctionCall) goja.Value{
		"log": logFn("info"), "info": logFn("info"), "warn": logFn("warn"), "error": logFn("error"), "debug": logFn("debug"),
	}))

	_ = rt.Set("cache", obj(map[string]func(goja.FunctionCall) goja.Value{
		"get":    v.cacheGet,
		"set":    v.cacheSet,
		"delete": v.cacheDelete,
	}))

	cr := obj(map[string]func(goja.FunctionCall) goja.Value{
		"random":          v.cryptoRandom,
		"uuid":            v.cryptoUUID,
		"hash":            v.cryptoHash,
		"hmac":            v.cryptoHmac,
		"timingSafeEqual": v.cryptoEqual,
	})
	_ = cr.Set("password", obj(map[string]func(goja.FunctionCall) goja.Value{
		"hash":   v.passwordHash,
		"verify": v.passwordVerify,
	}))
	_ = rt.Set("crypto", cr)

	db := rt.NewObject()
	_ = db.Set("posts", obj(map[string]func(goja.FunctionCall) goja.Value{
		"list":   v.postsList,
		"get":    v.postsGet,
		"create": v.postsCreate,
		"update": v.postsUpdate,
		"delete": v.postsDelete,
	}))
	_ = db.Set("asSystem", v.dbAsSystem)
	_ = db.Set("terms", obj(map[string]func(goja.FunctionCall) goja.Value{ // 2.1.0
		"list":   v.termsList,
		"get":    v.termsGet,
		"create": v.termsCreate,
		"update": v.termsUpdate,
		"delete": v.termsDelete,
	}))
	_ = db.Set("users", obj(map[string]func(goja.FunctionCall) goja.Value{ // 2.2.2, scope "users"
		"list": v.usersList,
		"get":  v.usersGet,
	}))
	v.installDB(db)
	_ = rt.Set("db", db)
	_ = rt.Set("migration", v.migrationGlobal)

	_ = rt.Set("media", obj(map[string]func(goja.FunctionCall) goja.Value{ // 2.1.0
		"list": v.mediaList, "get": v.mediaGet, "read": v.mediaRead, "upload": v.mediaUpload,
		"update": v.mediaUpdate, "replace": v.mediaReplace, "delete": v.mediaDelete, "resize": v.mediaResize,
	}))
	_ = rt.Set("files", obj(map[string]func(goja.FunctionCall) goja.Value{ // 2.1.0
		"put": v.filesPut, "get": v.filesGet, "text": v.filesText, "list": v.filesList, "delete": v.filesDelete, "url": v.filesURL,
	}))
	_ = rt.Set("mail", obj(map[string]func(goja.FunctionCall) goja.Value{"send": v.mailSend, "ready": v.mailReady}))
	_ = rt.Set("http", obj(map[string]func(goja.FunctionCall) goja.Value{"fetch": v.httpFetch, "webhook": v.httpWebhook}))
	_ = rt.Set("jobs", obj(map[string]func(goja.FunctionCall) goja.Value{"enqueue": v.jobsEnqueue}))

	v.installLocale() // 2.1.1: toLocaleString without Intl

	// no timers or modules; network only through http.fetch
	for _, name := range []string{"setTimeout", "setInterval", "fetch", "require"} {
		n := name
		_ = rt.Set(n, func(goja.FunctionCall) goja.Value {
			v.throw("%s ยังไม่รองรับใน Site Code", n)
			return nil
		})
	}
}

// ---------------------------------------------------------------- site

func (v *vm) siteRoute(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.route ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	if len(v.routes) >= maxRoutes {
		v.throw("ลงทะเบียน route ได้ไม่เกิน %d รายการ", maxRoutes)
	}
	v.need("routes", "site.route")
	spec := c.Argument(0)
	if _, ok := spec.Export().(string); !ok {
		v.throw("site.route: อาร์กิวเมนต์แรกต้องเป็นข้อความ เช่น \"GET /hello\"")
	}
	rs, err := parseSpec(spec.String())
	if err != nil {
		v.throw("%s", err.Error())
	}
	if !v.u.routeAllowed(rs.Path) {
		v.throw("site.route(%q): path ต้องอยู่ใต้ code.routes ของปลั๊กอิน (%s)", spec.String(), strings.Join(v.u.routes, ", "))
	}
	var opts goja.Value
	fnv := c.Argument(1)
	if _, ok := goja.AssertFunction(fnv); !ok {
		opts, fnv = fnv, c.Argument(2)
	}
	fn, ok := goja.AssertFunction(fnv)
	if !ok {
		v.throw("site.route(%q): ต้องมี handler เป็นฟังก์ชัน", spec.String())
	}
	var access []string
	rawBody, maxB := false, 0
	if opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
		o := opts.ToObject(v.rt)
		if rb := o.Get("rawBody"); rb != nil && !goja.IsUndefined(rb) {
			rawBody = rb.ToBoolean()
		}
		if mb := o.Get("maxBody"); mb != nil && !goja.IsUndefined(mb) && !goja.IsNull(mb) {
			f := mb.ToFloat()
			if f < 1 || f > maxRouteBody || f != float64(int64(f)) {
				v.throw("site.route(%q): maxBody ต้องเป็นจำนวน byte 1 ถึง %d (5 MB)", spec.String(), maxRouteBody)
			}
			maxB = int(f)
		}
		if a := o.Get("access"); a != nil && !goja.IsUndefined(a) {
			var list []string
			if err := v.rt.ExportTo(a, &list); err != nil {
				v.throw("site.route(%q): access ต้องเป็น array ของข้อความ เช่น ['*'] หรือ ['editor']", spec.String())
			}
			access = list
		}
	}
	acc, err := checkAccess(access)
	if err != nil {
		v.throw("site.route(%q): %s", spec.String(), err.Error())
	}
	v.vetCaps(acc, fmt.Sprintf("site.route(%q)", spec.String()))
	v.routes = append(v.routes, &regRoute{spec: rs, access: acc, fn: fn, at: v.caller(), rawBody: rawBody, maxBody: maxB})
	return goja.Undefined()
}

func (v *vm) siteOn(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.on ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	if len(v.events) >= maxHandlers {
		v.throw("ลงทะเบียน site.on ได้ไม่เกิน %d รายการ", maxHandlers)
	}
	name, _ := c.Argument(0).Export().(string)
	if !eventNameRe.MatchString(name) {
		v.throw("site.on: ชื่อเหตุการณ์ไม่ถูกต้อง: %q", c.Argument(0).String())
	}
	fn, ok := goja.AssertFunction(c.Argument(1))
	if !ok {
		v.throw("site.on(%q): ต้องมี handler เป็นฟังก์ชัน", name)
	}
	v.events = append(v.events, &regEvent{name: name, fn: fn, at: v.caller()})
	return goja.Undefined()
}

// ---------------------------------------------------------------- log

func (v *vm) addLog(level string, args []goja.Value) {
	if len(v.logs) >= maxLogLines {
		return
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, v.logString(a))
	}
	msg := strings.Join(parts, " ")
	if len(msg) > maxLogMessage {
		msg = clip(msg, maxLogMessage) + "…"
	}
	v.logs = append(v.logs, LogLine{Level: level, Message: msg})
	if len(v.logs) == maxLogLines {
		v.logs = append(v.logs, LogLine{Level: "warn", Message: fmt.Sprintf("log เกิน %d บรรทัดต่อครั้ง — ส่วนที่เหลือถูกตัด", maxLogLines)})
	}
}

func (v *vm) logString(a goja.Value) string {
	if a == nil || goja.IsUndefined(a) {
		return "undefined"
	}
	if s, ok := a.Export().(string); ok {
		return s
	}
	if o, ok := a.(*goja.Object); ok {
		if o.ClassName() == "Error" {
			return a.String()
		}
		if _, isFn := goja.AssertFunction(a); isFn {
			return "[function]"
		}
	}
	if s, ok, err := v.toJSON(a); err == nil && ok {
		return s
	}
	return a.String()
}

// ---------------------------------------------------------------- cache

func (v *vm) cacheKey(c goja.FunctionCall) string {
	k, ok := c.Argument(0).Export().(string)
	if !ok || k == "" || len(k) > 250 {
		v.throw("cache: key ต้องเป็นข้อความ 1-250 ตัวอักษร")
	}
	return k
}

func (v *vm) cacheGet(c goja.FunctionCall) goja.Value {
	v.handlerOnly("cache")
	s, ok := v.h.cache.Get(v.cacheKey(c))
	if !ok {
		return goja.Undefined()
	}
	out, err := v.parse(goja.Undefined(), v.rt.ToValue(s))
	if err != nil {
		return goja.Undefined()
	}
	return out
}

func (v *vm) cacheSet(c goja.FunctionCall) goja.Value {
	v.handlerOnly("cache")
	k := v.cacheKey(c)
	s, ok, err := v.toJSON(c.Argument(1))
	if err != nil || !ok {
		v.throw("cache.set: ค่าต้องแปลงเป็น JSON ได้")
	}
	var ttl time.Duration
	if t := c.Argument(2); !goja.IsUndefined(t) && !goja.IsNull(t) {
		sec := t.ToFloat()
		if sec < 0 || sec > 30*24*3600 {
			v.throw("cache.set: ttlSeconds ต้องอยู่ระหว่าง 0 ถึง 2592000")
		}
		ttl = time.Duration(sec * float64(time.Second))
	}
	if err := v.h.cache.Set(k, s, ttl); err != nil {
		v.throw("cache.set: %s", err.Error())
	}
	return goja.Undefined()
}

func (v *vm) cacheDelete(c goja.FunctionCall) goja.Value {
	v.handlerOnly("cache")
	v.h.cache.Delete(v.cacheKey(c))
	return goja.Undefined()
}

// ---------------------------------------------------------------- crypto

func (v *vm) str(c goja.FunctionCall, i int, what string) string {
	s, ok := c.Argument(i).Export().(string)
	if !ok {
		v.throw("%s ต้องเป็นข้อความ", what)
	}
	return s
}

func (v *vm) cryptoRandom(c goja.FunctionCall) goja.Value {
	v.handlerOnly("crypto")
	n := int(c.Argument(0).ToInteger())
	if goja.IsUndefined(c.Argument(0)) {
		n = 16
	}
	if n < 1 || n > 1024 {
		v.throw("crypto.random: จำนวน byte ต้องอยู่ระหว่าง 1 ถึง 1024")
	}
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return v.rt.ToValue(hex.EncodeToString(b))
}

func (v *vm) cryptoUUID(goja.FunctionCall) goja.Value {
	v.handlerOnly("crypto")
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return v.rt.ToValue(h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:])
}

func (v *vm) hasher(alg string, md5ok bool) func() hash.Hash {
	switch strings.ToLower(alg) {
	case "sha256":
		return sha256.New
	case "sha1":
		return sha1.New
	case "sha512":
		return sha512.New
	case "md5":
		if md5ok {
			return md5.New
		}
	}
	v.throw("ไม่รองรับ algorithm %q", alg)
	return nil
}

func (v *vm) cryptoHash(c goja.FunctionCall) goja.Value {
	v.handlerOnly("crypto")
	h := v.hasher(v.str(c, 0, "crypto.hash: algorithm"), true)()
	h.Write([]byte(v.str(c, 1, "crypto.hash: data")))
	return v.rt.ToValue(hex.EncodeToString(h.Sum(nil)))
}

func (v *vm) cryptoHmac(c goja.FunctionCall) goja.Value {
	v.handlerOnly("crypto")
	newH := v.hasher(v.str(c, 0, "crypto.hmac: algorithm"), false)
	m := hmac.New(newH, []byte(v.str(c, 1, "crypto.hmac: key")))
	m.Write([]byte(v.str(c, 2, "crypto.hmac: data")))
	return v.rt.ToValue(hex.EncodeToString(m.Sum(nil)))
}

func (v *vm) cryptoEqual(c goja.FunctionCall) goja.Value {
	v.handlerOnly("crypto")
	a, b := v.str(c, 0, "crypto.timingSafeEqual: a"), v.str(c, 1, "crypto.timingSafeEqual: b")
	return v.rt.ToValue(subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1)
}

func (v *vm) passwordHash(c goja.FunctionCall) goja.Value {
	v.handlerOnly("crypto")
	pw := v.str(c, 0, "crypto.password.hash: password")
	if len(pw) > 72 {
		v.throw("crypto.password.hash: รหัสผ่านยาวเกิน 72 byte")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		v.throw("crypto.password.hash: %s", err.Error())
	}
	return v.rt.ToValue(string(h))
}

func (v *vm) passwordVerify(c goja.FunctionCall) goja.Value {
	v.handlerOnly("crypto")
	pw, h := v.str(c, 0, "crypto.password.verify: password"), v.str(c, 1, "crypto.password.verify: hash")
	return v.rt.ToValue(bcrypt.CompareHashAndPassword([]byte(h), []byte(pw)) == nil)
}

// ---------------------------------------------------------------- db.posts

// canSeeDrafts: drafts are visible only to users who may edit that type.
func (v *vm) canSeeDrafts(typ string) bool {
	if v.system {
		return true
	}
	switch typ {
	case model.TypePage:
		return v.user.Can(auth.CapEditPages)
	case model.TypePost:
		return v.user.Can(auth.CapEditOthersPosts)
	}
	if t := v.readableType(typ); t != nil && v.user != nil {
		return t.CanEditType(v.user.Role)
	}
	return false
}

// readableType is a custom type db.posts.list / get may read: the running
// code's own, or one another unit registered (Site Code's types, a JS
// plugin's types — since 2.0.5). Writes stay limited to knownType.
func (v *vm) readableType(typ string) *PostTypeInfo {
	if t := v.customType(typ); t != nil {
		return t
	}
	if v.h.e == nil || typ == model.TypePost || typ == model.TypePage {
		return nil
	}
	return v.h.e.PostTypes().PostType(typ)
}

func (v *vm) postType(o *goja.Object) string {
	typ := model.TypePost
	if o != nil {
		if t := o.Get("type"); t != nil && !goja.IsUndefined(t) && !goja.IsNull(t) {
			typ = t.String()
		}
	}
	if !v.knownType(typ) && v.readableType(typ) == nil {
		v.throw("db.posts: type ต้องเป็น 'post', 'page' หรือประเภทที่ลงทะเบียนด้วย site.postType (ไม่ใช่ %q)", typ)
	}
	return typ
}

func optObject(rt *goja.Runtime, x goja.Value) *goja.Object {
	if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
		return nil
	}
	return x.ToObject(rt)
}

func optString(o *goja.Object, k string) string {
	if o == nil {
		return ""
	}
	if x := o.Get(k); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
		return x.String()
	}
	return ""
}

func (v *vm) postsList(c goja.FunctionCall) goja.Value {
	v.handlerOnly("db")
	v.need("content", "db.posts.list")
	if v.h.posts == nil {
		v.throw("db ไม่พร้อมใช้งาน")
	}
	o := optObject(v.rt, c.Argument(0))
	typ := v.postType(o)
	f := model.PostFilter{Type: typ, Category: optString(o, "category"), Search: strings.TrimSpace(optString(o, "search")),
		Limit: 10, NoContent: false}
	switch ord := optString(o, "order"); ord {
	case "", "newest":
	case "oldest", "title":
		f.Order = ord
	default:
		v.throw("db.posts.list: order ต้องเป็น 'newest', 'oldest' หรือ 'title'")
	}
	if pp := optString(o, "perPage"); pp != "" {
		n, err := strconv.Atoi(pp)
		if err != nil || n < 1 || n > 100 {
			v.throw("db.posts.list: perPage ต้องอยู่ระหว่าง 1 ถึง 100")
		}
		f.Limit = n
	}
	page := 1
	if pg := optString(o, "page"); pg != "" {
		n, err := strconv.Atoi(pg)
		if err != nil || n < 1 || n > 100000 {
			v.throw("db.posts.list: page ต้องเป็นจำนวนเต็มตั้งแต่ 1")
		}
		page = n
	}
	f.Offset = (page - 1) * f.Limit
	v.metaFilter(o, &f)
	v.listFilter(o, &f)
	if !v.canSeeDrafts(typ) {
		if f.Status != "" && f.Status != model.StatusPublish {
			return v.toJS(map[string]any{"items": []any{}, "total": 0})
		}
		f.Status = model.StatusPublish
	}
	posts, total, err := v.h.posts.ListPosts(f)
	if err != nil {
		v.throw("db.posts.list: อ่านข้อมูลไม่สำเร็จ")
	}
	items := make([]map[string]any, 0, len(posts))
	for _, p := range posts {
		items = append(items, v.postJSON(p))
	}
	return v.toJS(map[string]any{"items": items, "total": total})
}

// listFilter reads db.posts.list({status, authorId, ids}) (2.1.0).
func (v *vm) listFilter(o *goja.Object, f *model.PostFilter) {
	if o == nil {
		return
	}
	if st := optString(o, "status"); st != "" && st != "any" {
		if !model.ValidStatus(st) {
			v.throw("db.posts.list: status ต้องเป็น 'draft', 'publish', 'scheduled' หรือ 'any'")
		}
		f.Status = st
	}
	if a := optString(o, "authorId"); a != "" {
		n, err := strconv.ParseInt(a, 10, 64)
		if err != nil || n < 1 {
			v.throw("db.posts.list: authorId ต้องเป็นจำนวนเต็มบวก")
		}
		f.AuthorID = n
	}
	if x := o.Get("ids"); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
		list, ok := x.Export().([]any)
		if !ok || len(list) > 200 {
			v.throw("db.posts.list: ids ต้องเป็น array ของ id (ไม่เกิน 200)")
		}
		f.IDs = make([]int64, 0, len(list))
		for _, e := range list {
			var id int64
			switch n := e.(type) {
			case int64:
				id = n
			case float64:
				id = int64(n)
			case string:
				id, _ = strconv.ParseInt(n, 10, 64)
			}
			if id < 1 {
				v.throw("db.posts.list: ids ต้องเป็น id ของโพสต์ (จำนวนเต็มบวก)")
			}
			f.IDs = append(f.IDs, id)
		}
	}
}

func (v *vm) postsGet(c goja.FunctionCall) goja.Value {
	v.handlerOnly("db")
	v.need("content", "db.posts.get")
	if v.h.posts == nil {
		v.throw("db ไม่พร้อมใช้งาน")
	}
	o := optObject(v.rt, c.Argument(1))
	typ := v.postType(o)
	key := c.Argument(0)
	var p *model.Post
	var err error
	switch x := key.Export().(type) {
	case int64:
		p, err = v.h.posts.GetPost(x)
	case float64:
		p, err = v.h.posts.GetPost(int64(x))
	case string:
		if id, perr := strconv.ParseInt(x, 10, 64); perr == nil && id > 0 {
			p, err = v.h.posts.GetPost(id)
		} else {
			p, err = v.h.posts.PostBySlug(typ, x, !v.canSeeDrafts(typ))
		}
	default:
		v.throw("db.posts.get: ต้องระบุ id หรือ slug")
	}
	if err != nil || p == nil || p.Type != typ {
		return goja.Null()
	}
	if p.Status != model.StatusPublish && !v.canSeeDrafts(typ) {
		return goja.Null()
	}
	return v.toJS(v.postJSON(p))
}

// termsJSON is a post's terms by taxonomy: {category: [{id, slug, name}], …}.
func termsJSON(terms map[string][]model.Term) map[string]any {
	out := map[string]any{}
	for tax, list := range terms {
		l := make([]map[string]any, 0, len(list))
		for _, t := range list {
			l = append(l, map[string]any{"id": t.ID, "slug": t.Slug, "name": t.Name, "parent": t.Parent})
		}
		out[tax] = l
	}
	return out
}

func idOrNil(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func (v *vm) postJSON(p *model.Post) map[string]any {
	cats := make([]map[string]string, 0, len(p.Categories))
	category := ""
	for _, t := range p.Categories {
		if t.Taxonomy != "category" {
			continue
		}
		if category == "" {
			category = t.Name
		}
		cats = append(cats, map[string]string{"slug": t.Slug, "name": t.Name})
	}
	url := ""
	if v.h.postURL != nil {
		url = v.h.postURL(p)
	}
	return map[string]any{
		"id": p.ID, "type": p.Type, "title": p.Title, "slug": p.Slug, "status": p.Status, "url": url,
		"excerpt": p.Excerpt, "content": kses.SanitizeHTML(p.Content), "image": p.FeaturedImage,
		"category": category, "categories": cats, "author": p.AuthorName,
		"publishedAt": timeOrNil(p.PublishedAt), "updatedAt": p.UpdatedAt.UTC().Format(time.RFC3339), "meta": metaOrEmpty(p.Meta),
		// 2.1.0
		"terms":    termsJSON(p.Terms),
		"authorId": p.AuthorID, "imageId": idOrNil(p.FeaturedMediaID), "pinned": p.Pinned, "sortOrder": p.SortOrder,
		"createdAt": p.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ---------------------------------------------------------------- running

// RunError is a failed invocation: the message and source-mapped trace.
type RunError struct {
	Message string
	Trace   string
	At      Problem // first frame in the project files
	Timeout bool
}

func (e *RunError) Error() string {
	if e.At.File != "" {
		return fmt.Sprintf("%s (%s:%d:%d)", e.Message, e.At.File, e.At.Line, e.At.Column)
	}
	return e.Message
}

var pcSuffix = regexp.MustCompile(`\(\d+\)`)

// frames formats goja stack frames that belong to the project files.
func frames(st []goja.StackFrame) (string, Problem) {
	var b strings.Builder
	var first Problem
	for _, f := range st {
		p := f.Position()
		if p.Filename == "" || p.Filename == "bundle.js" {
			continue
		}
		if first.File == "" {
			first = Problem{File: p.Filename, Line: p.Line, Column: p.Column}
		}
		fn := f.FuncName()
		if fn == "" || fn == "<anonymous>" {
			fmt.Fprintf(&b, "    at %s:%d:%d\n", p.Filename, p.Line, p.Column)
		} else {
			fmt.Fprintf(&b, "    at %s (%s:%d:%d)\n", fn, p.Filename, p.Line, p.Column)
		}
	}
	return strings.TrimRight(b.String(), "\n"), first
}

// stackOf cleans an Error's "stack" property (used for rejected promises).
var stackLine = regexp.MustCompile(`^\s*at (?:(.+?) \()?([^():]+):(\d+):(\d+)\)?$`)

func stackOf(s string) (string, Problem) {
	var b strings.Builder
	var first Problem
	for _, line := range strings.Split(pcSuffix.ReplaceAllString(s, ""), "\n") {
		m := stackLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil || m[2] == "bundle.js" {
			continue
		}
		if first.File == "" {
			first.File = m[2]
			first.Line, _ = strconv.Atoi(m[3])
			first.Column, _ = strconv.Atoi(m[4])
		}
		b.WriteString("    " + strings.TrimSpace(line) + "\n")
	}
	return strings.TrimRight(b.String(), "\n"), first
}

// runErr converts what goja returned into a RunError.
func (v *vm) runErr(err error) *RunError {
	var ie *goja.InterruptedError
	if errors.As(err, &ie) {
		tr, at := frames(ie.Stack())
		return &RunError{Message: "หมดเวลา (timeout): โค้ดทำงานนานเกินกำหนด", Trace: tr, At: at, Timeout: true}
	}
	var se *goja.StackOverflowError
	if errors.As(err, &se) {
		tr, at := frames(se.Stack())
		return &RunError{Message: "RangeError: call stack ลึกเกินไป (recursion ไม่รู้จบ?)", Trace: tr, At: at}
	}
	var ex *goja.Exception
	if errors.As(err, &ex) {
		msg := "Error"
		if ex.Value() != nil {
			msg = ex.Value().String()
		}
		tr, at := frames(ex.Stack())
		return &RunError{Message: msg, Trace: tr, At: at}
	}
	var re *RunError
	if errors.As(err, &re) {
		return re
	}
	return &RunError{Message: err.Error()}
}

// guarded runs fn with a timeout (vm.Interrupt) and turns panics into errors.
func (v *vm) guarded(timeout time.Duration, fn func() (goja.Value, error)) (val goja.Value, err error) {
	t := time.AfterFunc(timeout, func() { v.rt.Interrupt(errTimeout) })
	defer t.Stop()
	defer func() {
		if rec := recover(); rec != nil {
			val, err = nil, &RunError{Message: fmt.Sprintf("internal error: %v", rec)}
		}
	}()
	return fn()
}

// settle unwraps a returned promise (there is no event loop: every job has
// run when the call returned, so a pending promise never settles).
func (v *vm) settle(val goja.Value) (goja.Value, error) {
	if val == nil {
		return goja.Undefined(), nil
	}
	p, ok := val.Export().(*goja.Promise)
	if !ok {
		return val, nil
	}
	switch p.State() {
	case goja.PromiseStateFulfilled:
		return p.Result(), nil
	case goja.PromiseStateRejected:
		r := p.Result()
		re := &RunError{Message: "Error"}
		if r != nil {
			re.Message = r.String()
			if o, ok := r.(*goja.Object); ok {
				if st := o.Get("stack"); st != nil && !goja.IsUndefined(st) {
					re.Trace, re.At = stackOf(st.String())
				}
			}
		}
		return nil, re
	}
	return nil, &RunError{Message: "handler คืน Promise ที่ไม่มีวันเสร็จ (ยังไม่รองรับ timer / fetch)"}
}

// load runs the bundle's top level (registration or a live VM).
func (v *vm) load(prog *goja.Program, mode vmMode, timeout time.Duration) error {
	v.mode = mode
	_, err := v.guarded(timeout, func() (goja.Value, error) { return v.rt.RunProgram(prog) })
	if err != nil {
		return v.runErr(err)
	}
	return nil
}

// call invokes a handler with args inside the time limit.
func (v *vm) call(fn goja.Callable, timeout time.Duration, args ...goja.Value) (goja.Value, error) {
	v.mode = modeHandler
	v.deadline = time.Now().Add(timeout)
	v.inCall = true
	defer func() { v.inCall = false }()
	val, err := v.guarded(timeout, func() (goja.Value, error) {
		r, err := fn(goja.Undefined(), args...)
		if err != nil {
			return nil, err
		}
		return v.settle(r)
	})
	if err != nil {
		return nil, v.runErr(err)
	}
	return val, nil
}

// userValue is ctx.user (null without a CMS session).
func (v *vm) userValue() goja.Value {
	if v.user == nil {
		return goja.Null()
	}
	u := v.toJS(map[string]any{"id": v.user.ID, "name": v.user.Name, "email": v.user.Email, "role": v.user.Role,
		"caps": v.userCaps(v.user.Role)}).ToObject(v.rt)
	role := v.user.Role
	_ = u.Set("can", func(c goja.FunctionCall) goja.Value {
		name := c.Argument(0).String()
		v.checkCanArg(name, "ctx.user.can") // a plugin: only its own capabilities (2.0.6)
		return v.rt.ToValue(auth.RoleHas(role, name))
	})
	return u
}

func metaOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
