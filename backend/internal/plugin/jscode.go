package plugin

// JavaScript plugins (Site Code phase 4b): a package with "runtime": "js"
// ships server-side TypeScript/JavaScript (code/**, migrations/**) that runs
// in the CMS's Site Code engine as its own "unit" — with its own tables
// (p_<id>_*), settings, cookies, hooks and routes, limited to the code
// scopes the administrator approved. The engine (internal/sitecode) builds
// and runs the code; this file holds the package side: manifest rules,
// dependencies between plugins (requires), and the bridge to WASM/service
// plugins (service.functions, events).

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RuntimeJS is the manifest "runtime" of a JavaScript plugin.
const RuntimeJS = "js"

// Code limits (the same as Site Code).
const (
	MaxCodeFiles    = 200
	MaxCodeFileSize = 512 << 10
	MaxCodeTotal    = 2 << 20
)

// CodeScopes are what a JS plugin may ask for in code.scopes.
var CodeScopes = map[string]bool{"db": true, "content": true, "content-write": true, "mail": true, "http": true,
	"visitor": true, "admin-pages": true, "jobs": true, "routes": true, "filters-core": true,
	// phase 6: site.middleware / site.afterResponse, and unsanitised site.head / site.bodyEnd output
	"middleware": true, "head-scripts": true,
	// phase 7 joints: site.fields in a plugin (post meta "<id>.<key>"), http.webhook to any public URL
	"content-fields": true, "webhooks": true,
	// 2.0.5: site.postType in a plugin (type ids "<id>_<name>")
	"post-types": true,
	// 2.2.2: db.users — read-only public profiles (author pages, team lists)
	"users": true,
	// 2.1.0: site.taxonomy, db.terms writes, the media library from code, the plugin's own files
	"taxonomies": true, "terms-write": true, "media": true, "media-write": true, "files": true}

// codeExt are the file types of code/** and migrations/**.
var codeExt = map[string]bool{".ts": true, ".js": true, ".mjs": true, ".json": true, ".md": true}

// ManifestCode is the "code" section of a JS plugin's manifest.
type ManifestCode struct {
	Entry   string     `json:"entry"`
	Scopes  []string   `json:"scopes"`
	Hosts   []string   `json:"hosts,omitempty"`
	Routes  []string   `json:"routes,omitempty"`
	Library string     `json:"library,omitempty"`
	Exports []string   `json:"exports,omitempty"`
	Hooks   *CodeHooks `json:"hooks,omitempty"`
	// EventAliases: events this plugin emits (its own "<id>.*" names) that are
	// also delivered under another name, e.g. {"forms.submitted":
	// "form.submitted"} for a plugin that took over a former core event.
	EventAliases map[string]string `json:"eventAliases,omitempty"`
}

// CodeHooks documents the plugin's own hooks (shown in the docs and the
// editor; applyFilter/emit of other names only logs a warning).
type CodeHooks struct {
	Filters []string `json:"filters"`
	Events  []string `json:"events"`
}

// CodeView is the "code" part of an installed package (API).
type CodeView struct {
	Entry   string    `json:"entry"`
	Scopes  []string  `json:"scopes"`
	Hosts   []string  `json:"hosts"`
	Routes  []string  `json:"routes"`
	Exports []string  `json:"exports"`
	Library string    `json:"library"`
	Hooks   CodeHooks `json:"hooks"`
	// EventAliases: "<id>.x" → the other name it is also delivered as.
	EventAliases map[string]string `json:"eventAliases"`
}

func (c *ManifestCode) view() *CodeView {
	if c == nil {
		return nil
	}
	v := &CodeView{Entry: c.Entry, Scopes: nonNil(c.Scopes), Hosts: nonNil(c.Hosts), Routes: nonNil(c.Routes),
		Exports: nonNil(c.Exports), Library: c.Library, Hooks: CodeHooks{Filters: []string{}, Events: []string{}},
		EventAliases: map[string]string{}}
	for k, a := range c.EventAliases {
		v.EventAliases[k] = a
	}
	if c.Hooks != nil {
		v.Hooks.Filters, v.Hooks.Events = nonNil(c.Hooks.Filters), nonNil(c.Hooks.Events)
	}
	return v
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// HasScope reports a code scope of the manifest.
func (c *ManifestCode) HasScope(s string) bool {
	if c == nil {
		return false
	}
	for _, x := range c.Scopes {
		if x == s {
			return true
		}
	}
	return false
}

var (
	exportNameRe = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)
	fnNameRe     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,62}$`)
)

// isCodeFile reports a path of the code area of a JS plugin.
func isCodeFile(name string) bool {
	return strings.HasPrefix(name, "code/") || strings.HasPrefix(name, "migrations/")
}

// Rules shared with the Plugin Store's validator (pkgfmt/jscode.go): a
// package the store accepts installs here and vice versa.
const (
	maxCodeList   = 50 // exports, hook filters, hook events
	maxCodeHosts  = 20
	maxCodeRoutes = 20
	maxRequires   = 20
	maxRangeLen   = 100
)

var (
	codeSegRe   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	codeHostRe  = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	hookTailRe  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,90}$`)
	migrationRe = regexp.MustCompile(`^migrations/[A-Za-z0-9][A-Za-z0-9._-]*\.(ts|js)$`)
)

// reservedCodeRoute are first path segments a JS plugin route prefix can
// never take (the install also checks the site's admin path / bases).
// codeRouteRe: a path prefix of a JS plugin; segments may hold a dot (2.2.2: /feed.xml, /ads.txt,
// /.well-known is still refused — a segment starts with a letter or digit)
var codeRouteRe = regexp.MustCompile(`^/[a-z0-9][a-z0-9_.-]*(/[a-z0-9][a-z0-9_.-]*)*$`)

var reservedCodeRoute = map[string]bool{"api": true, "admin": true, "uploads": true, "download": true, "plugin-assets": true, "site-assets": true,
	"_a": true, "_c": true, "ext-ui": true, "sitemap.xml": true, "robots.txt": true, "favicon.ico": true, "favicon.svg": true,
	"assets": true, "static": true, "blog": true, "category": true}

// validateCode checks the runtime "js" rules (docs/plugins.md §4).
func (mf *Manifest) validateCode(files map[string][]byte) error {
	switch mf.Runtime {
	case "", RuntimeJS:
	default:
		return pkgErr(`manifest: unknown runtime %q (use "js", or "runtime" inside "service" for wasm)`, mf.Runtime)
	}
	if mf.Runtime != RuntimeJS {
		switch {
		case mf.Code != nil:
			return pkgErr(`manifest: a "code" section needs "runtime": "js"`)
		case mf.HasPermission("code"):
			return pkgErr(`manifest: the "code" permission needs "runtime": "js"`)
		}
		for name := range files {
			if ext := strings.ToLower(path.Ext(name)); ext == ".ts" || ext == ".mjs" {
				return pkgErr(`%s: .ts / .mjs files need "runtime": "js"`, name)
			}
		}
		return nil
	}
	c := mf.Code
	if c == nil {
		return pkgErr(`manifest: "runtime": "js" needs "code": {"entry": "code/main.ts"}`)
	}
	if !mf.HasPermission("code") {
		return pkgErr(`manifest: "runtime": "js" requires the "code" permission`)
	}
	if mf.Service != nil {
		return pkgErr(`manifest: a JS plugin cannot also declare a "service" (make a separate service/WASM plugin and use plugins.call)`)
	}
	n, total := 0, 0
	lower := map[string]string{}
	migrations := 0
	for name, b := range files {
		if !isCodeFile(name) {
			continue
		}
		n++
		total += len(b)
		if !codeExt[strings.ToLower(path.Ext(name))] {
			return pkgErr("%s: files under code/ and migrations/ must be .ts .js .mjs .json or .md", name)
		}
		for _, seg := range strings.Split(name, "/") {
			if !codeSegRe.MatchString(seg) {
				return pkgErr("%s: code file names may only use A-Z a-z 0-9 . _ -", name)
			}
		}
		if len(b) > MaxCodeFileSize {
			return pkgErr("%s: code file is larger than 512 KB", name)
		}
		if prev, dup := lower[strings.ToLower(name)]; dup {
			return pkgErr("%s: same name as %s (case only)", name, prev)
		}
		lower[strings.ToLower(name)] = name
		if migrationRe.MatchString(name) {
			migrations++
		}
	}
	if n > MaxCodeFiles {
		return pkgErr("too many code files (max %d under code/ and migrations/)", MaxCodeFiles)
	}
	if total > MaxCodeTotal {
		return pkgErr("code is larger than 2 MB")
	}
	codePath := func(p, what string) error {
		ext := strings.ToLower(path.Ext(p))
		if !strings.HasPrefix(p, "code/") || (ext != ".ts" && ext != ".js" && ext != ".mjs") {
			return pkgErr("manifest: code.%s must be a .ts, .js or .mjs file under code/: %q", what, p)
		}
		if _, ok := files[p]; !ok {
			return pkgErr("manifest: code.%s file %q not found in the package", what, p)
		}
		return nil
	}
	if strings.TrimSpace(c.Entry) == "" {
		return pkgErr(`manifest: code.entry is required (e.g. "code/main.ts")`)
	}
	if err := codePath(c.Entry, "entry"); err != nil {
		return err
	}
	if c.Library != "" {
		if err := codePath(c.Library, "library"); err != nil {
			return err
		}
	}
	if err := uniqueList("code.scopes", c.Scopes, len(CodeScopes), func(s string) bool { return CodeScopes[s] },
		"unknown scope (db, content, content-write, mail, http, visitor, admin-pages, jobs, routes, filters-core, middleware, head-scripts, content-fields, webhooks, post-types, users, taxonomies, terms-write, media, media-write, files)"); err != nil {
		return err
	}
	if err := uniqueList("code.hosts", c.Hosts, maxCodeHosts, func(h string) bool { return len(h) <= 253 && codeHostRe.MatchString(h) },
		"invalid host (api.example.com or *.example.com)"); err != nil {
		return err
	}
	if len(c.Hosts) > 0 && !c.HasScope("http") {
		return pkgErr(`manifest: code.hosts needs the "http" scope`)
	}
	if err := uniqueList("code.routes", c.Routes, maxCodeRoutes, func(p string) bool { return len(p) <= 60 && codeRouteRe.MatchString(p) && !strings.Contains(p, "..") },
		"a route must be a path prefix such as /shop or /feed.xml (a-z 0-9 _ - .)"); err != nil {
		return err
	}
	for _, p := range c.Routes {
		first, _, _ := strings.Cut(strings.TrimPrefix(p, "/"), "/")
		if reservedCodeRoute[first] {
			return pkgErr("manifest: code.routes: %q is reserved by the CMS", p)
		}
	}
	if len(c.Routes) > 0 && !c.HasScope("routes") {
		return pkgErr(`manifest: code.routes needs the "routes" scope`)
	}
	if err := uniqueList("code.exports", c.Exports, maxCodeList, exportNameRe.MatchString,
		"an export must be a JavaScript function name (A-Z a-z 0-9 _ $)"); err != nil {
		return err
	}
	hookOK := func(h string) bool {
		tail, ok := strings.CutPrefix(h, mf.ID+".")
		return ok && hookTailRe.MatchString(tail)
	}
	if h := c.Hooks; h != nil {
		if err := uniqueList("code.hooks.filters", h.Filters, maxCodeList, hookOK, "a hook name must start with \""+mf.ID+".\""); err != nil {
			return err
		}
		if err := uniqueList("code.hooks.events", h.Events, maxCodeList, hookOK, "a hook name must start with \""+mf.ID+".\""); err != nil {
			return err
		}
	}
	if migrations > 0 && !c.HasScope("db") {
		return pkgErr(`manifest: migrations/ needs the "db" scope`)
	}
	if err := validateEventAliases(mf.ID, c.EventAliases); err != nil {
		return err
	}
	return nil
}

// eventAliasRe is the other name of an aliased event ("form.submitted").
var eventAliasRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}\.[a-zA-Z0-9][a-zA-Z0-9._:-]{0,90}$`)

// CoreEventPrefixes are event namespaces only the CMS emits: an event alias
// can never claim them.
var CoreEventPrefixes = []string{"post.", "user.", "media.", "comment.", "site.", "plugin.", "page.", "mail.", "action.", "collection.", "sitemap.", "robots."}

// validateEventAliases checks code.eventAliases: a source is one of the
// plugin's own events ("<id>.x"), the alias is another dotted name outside
// the CMS's own namespaces and the plugin's own prefix.
func validateEventAliases(id string, aliases map[string]string) error {
	if len(aliases) > 10 {
		return pkgErr("manifest: code.eventAliases: at most 10 entries")
	}
	for src, alias := range aliases {
		tail, ok := strings.CutPrefix(src, id+".")
		if !ok || !hookTailRe.MatchString(tail) {
			return pkgErr("manifest: code.eventAliases: %q must be one of the plugin's own events (\"%s.…\")", src, id)
		}
		if !eventAliasRe.MatchString(alias) || strings.HasPrefix(alias, id+".") {
			return pkgErr("manifest: code.eventAliases: %q is not a valid alias (e.g. \"form.submitted\")", alias)
		}
		for _, p := range CoreEventPrefixes {
			if strings.HasPrefix(alias, p) {
				return pkgErr("manifest: code.eventAliases: %q is a CMS event name", alias)
			}
		}
	}
	return nil
}

func uniqueList(field string, list []string, max int, ok func(string) bool, why string) error {
	if len(list) > max {
		return pkgErr("manifest: %s: at most %d entries", field, max)
	}
	seen := map[string]bool{}
	for _, v := range list {
		if seen[v] {
			return pkgErr("manifest: %s: %q is listed twice", field, v)
		}
		seen[v] = true
		if !ok(v) {
			return pkgErr("manifest: %s: %q — %s", field, v, why)
		}
	}
	return nil
}

// builtinIDs are ids "requires" cannot name (same list as the Plugin Store's
// reserved ids): "backup" is the core tool "Export / import site". The former
// built-ins (seo, forms, …) are official packages since phase 7 and may be
// required like any plugin.
var builtinIDs = map[string]bool{"backup": true}

// validateRequires checks the "requires" map (ids and range syntax); every
// package may declare it.
func (mf *Manifest) validateRequires() error {
	if err := mf.validateDeps("requires", mf.Requires); err != nil {
		return err
	}
	if err := mf.validateDeps("optionalRequires", mf.OptionalRequires); err != nil {
		return err
	}
	for id := range mf.OptionalRequires {
		if _, both := mf.Requires[id]; both {
			return pkgErr("manifest: %q is listed in both requires and optionalRequires", id)
		}
	}
	return nil
}

// validateDeps checks a requires / optionalRequires map (same syntax rules).
func (mf *Manifest) validateDeps(field string, deps map[string]string) error {
	if len(deps) > maxRequires {
		return pkgErr("manifest: %s: at most %d entries", field, maxRequires)
	}
	for id, rng := range deps {
		if !pkgIDRe.MatchString(id) || id == mf.ID {
			return pkgErr("manifest: %s: invalid plugin id %q", field, id)
		}
		if builtinIDs[id] {
			return pkgErr("manifest: %s: %q is a built-in plugin (no need to require it)", field, id)
		}
		if _, err := ParseRange(rng); err != nil {
			return pkgErr("manifest: %s %s: %v", field, id, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- semver ranges

// cmp is one comparator of a range.
type cmp struct {
	op string // >= > < <= =
	v  string
}

var partialRe = regexp.MustCompile(`^(\d{1,6}|[xX*])(?:\.(\d{1,6}|[xX*]))?(?:\.(\d{1,6}|[xX*]))?(-[0-9A-Za-z.-]+)?$`)

// ParseRange parses a semver range (npm style): alternatives joined with
// "||", each a space-separated set of comparators that must all match:
// exact (1.2.3, =1.2.3), caret (^1.2.3, ^0.2, ^1), tilde (~1.2.3, ~1.2),
// x-ranges (1.x, 1.2.*, *, 1, 1.2) and >=, >, <, <= ("> = 1.0" with a space
// after the operator is accepted). Hyphen ranges, a leading "v", build
// metadata and a pre-release on a partial version are refused.
func ParseRange(s string) ([][]cmp, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("a version range is required (e.g. ^1.0.0)")
	}
	if len(s) > maxRangeLen {
		return nil, fmt.Errorf("version range longer than %d characters", maxRangeLen)
	}
	var out [][]cmp
	for _, alt := range strings.Split(s, "||") {
		fields := strings.Fields(alt)
		if len(fields) == 0 {
			return nil, fmt.Errorf("invalid version range %q", s)
		}
		var set []cmp
		for i := 0; i < len(fields); i++ {
			f := fields[i]
			if (f == ">=" || f == "<=" || f == ">" || f == "<" || f == "=" || f == "^" || f == "~") && i+1 < len(fields) {
				f += fields[i+1]
				i++
			}
			cs, err := parseComparator(f)
			if err != nil {
				return nil, fmt.Errorf("invalid version range %q (use ^1.2.0, ~1.2.0, 1.x, >=1.0.0 <2.0.0)", s)
			}
			set = append(set, cs...)
		}
		out = append(out, set)
	}
	return out, nil
}

func parseComparator(s string) ([]cmp, error) {
	op := ""
	for _, p := range []string{">=", "<=", ">", "<", "=", "^", "~"} {
		if strings.HasPrefix(s, p) {
			op, s = p, s[len(p):]
			break
		}
	}
	m := partialRe.FindStringSubmatch(s)
	if m == nil {
		return nil, errors.New("bad comparator")
	}
	parts := [3]int{-1, -1, -1}
	for i := 0; i < 3; i++ {
		if m[i+1] == "" || strings.ContainsAny(m[i+1], "xX*") {
			break
		}
		parts[i], _ = strconv.Atoi(m[i+1])
	}
	pre := m[4]
	known := 0
	for known < 3 && parts[known] >= 0 {
		known++
	}
	if pre != "" && known < 3 {
		return nil, errors.New("pre-release needs a full version")
	}
	ver := func(a, b, c int) string { return fmt.Sprintf("%d.%d.%d", a, b, c) }
	full := func() string { return ver(parts[0], parts[1], parts[2]) + pre }
	lo := func() string {
		p := parts
		for i := range p {
			if p[i] < 0 {
				p[i] = 0
			}
		}
		return ver(p[0], p[1], p[2]) + pre
	}
	next := func(k int) string {
		switch k {
		case 1:
			return ver(parts[0]+1, 0, 0)
		case 2:
			return ver(parts[0], parts[1]+1, 0)
		}
		return ""
	}
	if known == 0 {
		if op == "<" || op == ">" {
			return []cmp{{"<", "0.0.0"}}, nil // matches nothing, like npm
		}
		return []cmp{{">=", "0.0.0"}}, nil
	}
	switch op {
	case "", "=":
		if known == 3 {
			return []cmp{{"=", full()}}, nil
		}
		return []cmp{{">=", lo()}, {"<", next(known)}}, nil
	case "^":
		var hi string
		switch {
		case parts[0] > 0 || known == 1:
			hi = ver(parts[0]+1, 0, 0)
		case parts[1] > 0 || known == 2:
			hi = ver(0, parts[1]+1, 0)
		default:
			hi = ver(0, 0, parts[2]+1)
		}
		return []cmp{{">=", lo()}, {"<", hi}}, nil
	case "~":
		if known == 1 {
			return []cmp{{">=", lo()}, {"<", next(1)}}, nil
		}
		return []cmp{{">=", lo()}, {"<", ver(parts[0], parts[1]+1, 0)}}, nil
	case ">=", "<":
		return []cmp{{op, lo()}}, nil
	case ">":
		if known == 3 {
			return []cmp{{">", full()}}, nil
		}
		return []cmp{{">=", next(known)}}, nil
	case "<=":
		if known == 3 {
			return []cmp{{"<=", full()}}, nil
		}
		return []cmp{{"<", next(known)}}, nil
	}
	return nil, errors.New("bad operator")
}

// SatisfiesRange reports whether version is within rng. A pre-release only
// matches a comparator set with a pre-release on the same MAJOR.MINOR.PATCH
// (npm semantics).
func SatisfiesRange(version, rng string) bool {
	alts, err := ParseRange(rng)
	pv := semverRe.FindStringSubmatch(version)
	if err != nil || pv == nil {
		return false
	}
	for _, set := range alts {
		ok := true
		for _, c := range set {
			d := CompareVersions(version, c.v)
			switch c.op {
			case ">=":
				ok = ok && d >= 0
			case ">":
				ok = ok && d > 0
			case "<":
				ok = ok && d < 0
			case "<=":
				ok = ok && d <= 0
			case "=":
				ok = ok && d == 0
			}
		}
		if !ok {
			continue
		}
		if pv[4] == "" {
			return true
		}
		core := pv[1] + "." + pv[2] + "." + pv[3]
		for _, c := range set {
			if pc := semverRe.FindStringSubmatch(c.v); pc != nil && pc[4] != "" && pc[1]+"."+pc[2]+"."+pc[3] == core {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------- dependencies

// MissingDep is a dependency that is not installed, has an incompatible
// version, or (when activating) is not active.
type MissingDep struct {
	ID        string `json:"id"`
	Range     string `json:"range"`
	Installed string `json:"installed,omitempty"` // the installed version (incompatible or inactive)
	Active    *bool  `json:"active,omitempty"`    // set when the dependency is installed but switched off
	// StoreVersion is the newest version in the Plugin Store (store installs)
	// and Compatible whether it satisfies Range.
	StoreVersion string `json:"storeVersion,omitempty"`
	Latest       string `json:"latest,omitempty"` // same as StoreVersion (admin UI)
	Compatible   *bool  `json:"compatible,omitempty"`
}

// RequiresError lists missing dependencies (HTTP 400 {error, missing}).
type RequiresError struct{ Missing []MissingDep }

func (e *RequiresError) Error() string {
	ids := make([]string, 0, len(e.Missing))
	for _, m := range e.Missing {
		ids = append(ids, m.ID+" "+m.Range)
	}
	return "ต้องติดตั้งปลั๊กอินที่จำเป็นก่อน: " + strings.Join(ids, ", ")
}

// DependentsError: other plugins require this one (HTTP 409 {error, dependents}).
type DependentsError struct {
	ID         string
	Dependents []string
}

func (e *DependentsError) Error() string {
	return fmt.Sprintf("ปลั๊กอิน %s ถูกใช้โดย %s — ปิด/ถอนปลั๊กอินเหล่านั้นก่อน หรือส่ง ?force=1", e.ID, strings.Join(e.Dependents, ", "))
}

// CodeProblem is a build / activation problem at a file position.
type CodeProblem struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

// CodeDestructive is a destructive migration step that needs confirmation.
type CodeDestructive struct {
	Migration string `json:"migration"`
	Op        string `json:"op"`
	Table     string `json:"table"`
	Column    string `json:"column,omitempty"`
}

// DestructiveError: migrations would drop/rename data; repeat with
// confirmDestructive (HTTP 400 {error, destructive}).
type DestructiveError struct{ List []CodeDestructive }

func (e *DestructiveError) Error() string {
	return "migration ของปลั๊กอินจะลบหรือเปลี่ยนข้อมูล — ยืนยันก่อน (confirmDestructive)"
}

// ActivationError: the plugin's code did not build, register or migrate
// (HTTP 400 {error, activationError, errors}); the plugin stays inactive.
type ActivationError struct {
	Msg      string
	Problems []CodeProblem
}

func (e *ActivationError) Error() string { return e.Msg }

// CodePlan is the dry run of a JS plugin's activation.
type CodePlan struct {
	Errors      []CodeProblem     `json:"errors"`
	Destructive []CodeDestructive `json:"destructive"`
	Pending     []string          `json:"pending"` // migrations that would run
}

// CodeRuntime is the Site Code engine as seen by the plugin manager.
type CodeRuntime interface {
	// PlanUnit builds a unit from files (not installed yet) and dry-runs its
	// pending migrations.
	PlanUnit(ref CodeUnitRef, files map[string]string) (*CodePlan, error)
	// ActivateUnit builds the installed unit, runs its pending migrations
	// (destructive ones only with confirm) and checks it registers cleanly.
	ActivateUnit(id string, confirmDestructive bool) error
	// UnitsChanged tells the engine the set of JS plugins changed.
	UnitsChanged()
	// DropUnitData backs up and drops the plugin's tables and deletes its
	// settings, access overrides, jobs and migration records.
	DropUnitData(id string) error
	// UnitError is the last load error of an active unit ("" = fine).
	UnitError(id string) string
}

// SetCodeRuntime attaches the Site Code engine (cmd/server).
func (m *Manager) SetCodeRuntime(rt CodeRuntime) { m.code = rt }

// Package sources (plugin_packages.source).
const (
	SourceUpload = "upload" // an uploaded zip
	SourceStore  = "store"  // downloaded from the Store
)

// CodeUnitRef describes an installed JS plugin for the engine.
type CodeUnitRef struct {
	ID          string
	Name        string
	Version     string
	InstalledAt time.Time
	Active      bool
	Code        ManifestCode
	Requires    map[string]string
	// OptionalRequires: plugins this one works with (phase 6).
	OptionalRequires map[string]string
	// Source is where the package came from: SourceStore or SourceUpload
	// (demo mode runs only Store packages).
	Source string
	// Capabilities are the plugin's declared capabilities, "<id>.<capability>"
	// (manifest "capabilities", since 2.0.6).
	Capabilities []string
}

func (k *pkg) codeRef() CodeUnitRef {
	r := CodeUnitRef{ID: k.mf.ID, Name: k.mf.Name, Version: k.version, InstalledAt: k.installedAt, Active: k.active,
		Requires: k.mf.Requires, OptionalRequires: k.mf.OptionalRequires, Source: k.source, Capabilities: k.mf.CapKeys()}
	if k.mf.Code != nil {
		r.Code = *k.mf.Code
	}
	if r.Requires == nil {
		r.Requires = map[string]string{}
	}
	return r
}

func (k *pkg) isJS() bool { return k.mf.Runtime == RuntimeJS && k.mf.Code != nil }

// CodeUnits lists the installed JS plugins (cheap: from the package cache).
func (m *Manager) CodeUnits() []CodeUnitRef {
	var out []CodeUnitRef
	for _, k := range m.pkgs.all() {
		if k.isJS() {
			out = append(out, k.codeRef())
		}
	}
	return out
}

// CodeFiles reads the code area (code/**, migrations/**) of an installed JS plugin.
func (m *Manager) CodeFiles(id string) (map[string]string, error) {
	k := m.pkgs.get(id)
	if k == nil || !k.isJS() {
		return nil, ErrUnknownPackage
	}
	return readCodeDir(k.dir)
}

func readCodeDir(dir string) (map[string]string, error) {
	out := map[string]string{}
	for _, sub := range []string{"code", "migrations"} {
		root := filepath.Join(dir, sub)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !d.Type().IsRegular() || !codeExt[strings.ToLower(filepath.Ext(p))] {
				return nil
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(b)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func codeFilesOf(files map[string][]byte) map[string]string {
	out := map[string]string{}
	for name, b := range files {
		if isCodeFile(name) && codeExt[strings.ToLower(path.Ext(name))] {
			out[name] = string(b)
		}
	}
	return out
}

// checkRequires returns the dependencies of mf that are not installed with
// a compatible version (and, with needActive, not active).
func (m *Manager) checkRequires(mf *Manifest, needActive bool) []MissingDep {
	var out []MissingDep
	ids := make([]string, 0, len(mf.Requires))
	for id := range mf.Requires {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		rng := mf.Requires[id]
		k := m.pkgs.get(id)
		switch {
		case k == nil:
			out = append(out, MissingDep{ID: id, Range: rng})
		case !SatisfiesRange(k.version, rng):
			out = append(out, MissingDep{ID: id, Range: rng, Installed: k.version})
		case needActive && !k.active:
			f := false
			out = append(out, MissingDep{ID: id, Range: rng, Installed: k.version, Active: &f})
		}
	}
	return out
}

// dependents lists installed (or, with activeOnly, active) packages whose
// requires name id.
func (m *Manager) dependents(id string, activeOnly bool) []string {
	var out []string
	for _, k := range m.pkgs.all() {
		if _, ok := k.mf.Requires[id]; ok && (!activeOnly || k.active) {
			out = append(out, k.mf.ID)
		}
	}
	sort.Strings(out)
	return out
}

// RequireStatus is one dependency of an installed package (API).
type RequireStatus struct {
	ID        string `json:"id"`
	Range     string `json:"range"`
	Installed string `json:"installed"` // "" = not installed
	Active    bool   `json:"active"`
	OK        bool   `json:"ok"` // installed, compatible and active
}

func (m *Manager) requireStatus(mf *Manifest) []RequireStatus { return m.depStatus(mf.Requires) }

// depStatus is the status of each dependency of deps (requires or
// optionalRequires).
func (m *Manager) depStatus(deps map[string]string) []RequireStatus {
	out := []RequireStatus{}
	ids := make([]string, 0, len(deps))
	for id := range deps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		rs := RequireStatus{ID: id, Range: deps[id]}
		if k := m.pkgs.get(id); k != nil {
			rs.Installed, rs.Active = k.version, k.active
			rs.OK = k.active && SatisfiesRange(k.version, rs.Range)
		}
		out = append(out, rs)
	}
	return out
}

// setCodeError stores (or clears) the activation error of a package.
func (m *Manager) setCodeError(id, msg string) {
	if _, err := m.pkgs.db.Exec(`UPDATE plugin_packages SET code_error=? WHERE id=?`, msg, id); err != nil {
		log.Printf("plugin %s: save activation error: %v", id, err)
	}
	m.pkgs.invalidate()
}

// activateCode runs the engine's activation of a JS plugin; a failure is
// stored as the package's activation error and the package is switched off.
func (m *Manager) activateCode(id string, confirm bool) error {
	if m.code == nil {
		return &ActivationError{Msg: "ระบบ Site Code ไม่พร้อมใช้งาน — เปิดปลั๊กอิน JavaScript ไม่ได้"}
	}
	err := m.code.ActivateUnit(id, confirm)
	var de *DestructiveError
	if errors.As(err, &de) {
		return err // nothing ran: the administrator decides
	}
	if err != nil {
		msg := err.Error()
		var ae *ActivationError
		if errors.As(err, &ae) {
			msg = ae.Msg
			if len(ae.Problems) > 0 && ae.Problems[0].File != "" {
				p := ae.Problems[0]
				msg = fmt.Sprintf("%s (%s:%d:%d)", ae.Msg, p.File, p.Line, p.Column)
			}
		}
		_, _ = m.pkgs.db.Exec(`UPDATE plugin_packages SET active=FALSE WHERE id=?`, id)
		m.setCodeError(id, msg)
		m.code.UnitsChanged()
		return err
	}
	return nil
}

// ---------------------------------------------------------------- service functions and events

// CallFunction calls service.functions fn of an active service / WASM
// plugin: POST /functions/<fn> (signed) with body {input, user, visitor}.
// It returns the answer's status and body.
func (m *Manager) CallFunction(id, fn string, body []byte, timeout time.Duration) (int, []byte, error) {
	k := m.pkgs.get(id)
	if k == nil || !k.active || !k.hasService() {
		return 0, nil, fmt.Errorf("ไม่มีปลั๊กอิน %s ที่เปิดใช้งานอยู่", id)
	}
	if !k.serviceReady() {
		return 0, nil, fmt.Errorf("ปลั๊กอิน %s ยังไม่ได้เชื่อมต่อโปรแกรม (service)", id)
	}
	found := false
	for _, f := range k.mf.Service.Functions {
		found = found || f == fn
	}
	if !found {
		return 0, nil, fmt.Errorf("ปลั๊กอิน %s ไม่มีฟังก์ชัน %q (service.functions)", id, fn)
	}
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	req, cancel, err := newServiceRequest(k, http.MethodPost, "/functions/"+url.PathEscape(fn), body, timeout)
	if err != nil {
		return 0, nil, err
	}
	defer cancel()
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.pkgs.do(k, req)
	if err != nil {
		return 0, nil, fmt.Errorf("เรียก %s.%s ไม่สำเร็จ: %v", id, fn, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20+1))
	if err != nil {
		return 0, nil, err
	}
	if len(b) > 5<<20 {
		return 0, nil, fmt.Errorf("คำตอบของ %s.%s ใหญ่เกิน 5 MB", id, fn)
	}
	return resp.StatusCode, bytes.TrimSpace(b), nil
}

// HasFunction reports whether an installed service / WASM plugin declares fn.
func (m *Manager) HasFunction(id, fn string) bool {
	k := m.pkgs.get(id)
	if k == nil || !k.hasService() {
		return false
	}
	for _, f := range k.mf.Service.Functions {
		if f == fn {
			return true
		}
	}
	return false
}

// DeliverEvent sends an event raised by code (site.emit) to the service /
// WASM plugins subscribed to it and to package webhooks — the delivery of
// core events, without re-entering Site Code.
func (m *Manager) DeliverEvent(event string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	m.pkgs.webhooks(event, payload)
}

// ServiceEventNames lists the events declared by active service plugins'
// subscriptions and the ids that may emit "<id>.*" (hooks screen).
func (m *Manager) ServiceEventNames() map[string][]string {
	out := map[string][]string{}
	for _, k := range m.pkgs.active() {
		if k.hasService() {
			out[k.mf.ID] = append([]string{}, k.mf.Service.Events...)
		}
	}
	return out
}

// ---------------------------------------------------------------- phase 6: package files and plugin state

// MaxAssetSize caps a package file read by site.asset (5 MB).
const MaxAssetSize = 5 << 20

// ErrAssetNotFound: no such readable file in the package.
var ErrAssetNotFound = errors.New("asset not found")

// ErrAssetTooLarge: the file is larger than MaxAssetSize.
var ErrAssetTooLarge = errors.New("asset too large")

// AssetPathOK reports whether rel may be read with site.asset: a clean
// relative path inside the package, not code (code/, migrations/) and not
// the manifest.
func AssetPathOK(rel string) bool {
	if rel == "" || len(rel) > 300 || strings.ContainsAny(rel, `:\`) || strings.IndexByte(rel, 0) >= 0 || strings.HasPrefix(rel, "/") || path.Clean(rel) != rel {
		return false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, ".") {
			return false
		}
	}
	low := strings.ToLower(rel)
	return !strings.HasPrefix(low, "code/") && !strings.HasPrefix(low, "migrations/") && low != "manifest.json"
}

// PackageFile reads file rel of the installed, active package id (the
// version is returned for caching and the public asset URL).
func (m *Manager) PackageFile(id, rel string) ([]byte, string, error) {
	k := m.pkgs.get(id)
	if k == nil || !k.active || !AssetPathOK(rel) {
		return nil, "", ErrAssetNotFound
	}
	full := filepath.Join(k.dir, filepath.FromSlash(rel))
	if !strings.HasPrefix(full, filepath.Clean(k.dir)+string(filepath.Separator)) {
		return nil, "", ErrAssetNotFound
	}
	fi, err := os.Lstat(full)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, "", ErrAssetNotFound
	}
	if fi.Size() > MaxAssetSize {
		return nil, "", ErrAssetTooLarge
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, "", ErrAssetNotFound
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxAssetSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(b) > MaxAssetSize {
		return nil, "", ErrAssetTooLarge
	}
	return b, k.version, nil
}

// PluginState reports an installed package plugin's version and whether it
// is active (ok=false when not installed). Built-in plugins are not listed.
func (m *Manager) PluginState(id string) (version string, active, ok bool) {
	k := m.pkgs.get(id)
	if k == nil {
		return "", false, false
	}
	return k.version, k.active, true
}
