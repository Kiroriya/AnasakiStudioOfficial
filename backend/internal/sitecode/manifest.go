package sitecode

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// RouteInfo is a route the code registered with site.route.
type RouteInfo struct {
	Method string   `json:"method"`
	Path   string   `json:"path"`
	Access []string `json:"access"`
	// RawBody: request.rawBody / rawBodyBase64 (webhooks); MaxBody in bytes.
	RawBody bool `json:"rawBody,omitempty"`
	MaxBody int  `json:"maxBody,omitempty"`
}

// Manifest lists what the code registered (stored with each version and
// shown by GET /status and POST /build).
type Manifest struct {
	Routes      []RouteInfo      `json:"routes"`
	Events      []string         `json:"events"`
	Collections []CollectionInfo `json:"collections"`
	Actions     []ActionInfo     `json:"actions"`
	Pages       []PageInfo       `json:"pages"`
	Formatters  []string         `json:"formatters"`

	// phase 4a
	Filters       []FilterInfo           `json:"filters"`
	Visitor       bool                   `json:"visitor"`
	VisitorGroups []string               `json:"visitorGroups"`
	PostTypes     []PostTypeInfo         `json:"postTypes"`
	Fields        map[string][]FieldInfo `json:"fields"`
	// 2.1.0: site.taxonomy
	Taxonomies []TaxonomyInfo  `json:"taxonomies"`
	Jobs       []JobInfo       `json:"jobs"`
	Crons      []CronInfo      `json:"crons"`
	AdminPages []AdminPageInfo `json:"adminPages"`

	// phase 4b: site.export names
	Exports []string `json:"exports"`
}

// EmptyManifest is a manifest with no registrations (empty lists, not nil).
func EmptyManifest() Manifest {
	var m Manifest
	m.normalize()
	return m
}

func (m *Manifest) normalize() {
	if m.Routes == nil {
		m.Routes = []RouteInfo{}
	}
	if m.Events == nil {
		m.Events = []string{}
	}
	if m.Collections == nil {
		m.Collections = []CollectionInfo{}
	}
	if m.Actions == nil {
		m.Actions = []ActionInfo{}
	}
	if m.Pages == nil {
		m.Pages = []PageInfo{}
	}
	if m.Formatters == nil {
		m.Formatters = []string{}
	}
	if m.Filters == nil {
		m.Filters = []FilterInfo{}
	}
	if m.VisitorGroups == nil {
		m.VisitorGroups = []string{}
	}
	if m.PostTypes == nil {
		m.PostTypes = []PostTypeInfo{}
	}
	if m.Taxonomies == nil {
		m.Taxonomies = []TaxonomyInfo{}
	}
	if m.Fields == nil {
		m.Fields = map[string][]FieldInfo{}
	}
	if m.Jobs == nil {
		m.Jobs = []JobInfo{}
	}
	if m.Crons == nil {
		m.Crons = []CronInfo{}
	}
	if m.AdminPages == nil {
		m.AdminPages = []AdminPageInfo{}
	}
	if m.Exports == nil {
		m.Exports = []string{}
	}
	for i := range m.Routes {
		if m.Routes[i].Access == nil {
			m.Routes[i].Access = []string{}
		}
	}
}

// Registration limits.
const (
	maxRoutes   = 200
	maxHandlers = 200
)

var (
	methods     = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	literalRe   = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)
	paramRe     = regexp.MustCompile(`^\{([A-Za-z_][A-Za-z0-9_]*)(\.\.\.)?\}$`)
	eventNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,99}$`)
	accessRe    = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)
)

// routeSpec is a parsed "METHOD /path" spec.
type routeSpec struct {
	Method  string
	Path    string   // as written (normalised "/*" form kept)
	Pattern string   // net/http ServeMux pattern
	Params  []string // parameter names in order ("*" for a trailing /*)
	Shape   string   // path with parameters blanked, for duplicate detection
}

// parseSpec validates a route spec like "GET /hello/{name}" or "GET /files/*".
func parseSpec(spec string) (*routeSpec, error) {
	method, p, ok := strings.Cut(strings.TrimSpace(spec), " ")
	p = strings.TrimSpace(p)
	method = strings.ToUpper(method)
	if !ok || !methods[method] {
		return nil, fmt.Errorf("route %q: ต้องขึ้นต้นด้วย GET, POST, PUT, PATCH หรือ DELETE ตามด้วย path เช่น \"GET /hello\"", spec)
	}
	if !strings.HasPrefix(p, "/") || len(p) > 200 || strings.ContainsAny(p, "?# ") {
		return nil, fmt.Errorf("route %q: path ต้องขึ้นต้นด้วย / (ไม่มี ? หรือ #, ยาวไม่เกิน 200)", spec)
	}
	rs := &routeSpec{Method: method, Path: p}
	if p == "/" {
		rs.Pattern, rs.Shape = method+" /{$}", "/"
		return rs, nil
	}
	if strings.HasSuffix(p, "/") {
		return nil, fmt.Errorf("route %q: path ต้องไม่ลงท้ายด้วย / (ใช้ /* เพื่อรับทุก path ข้างใต้)", spec)
	}
	segs := strings.Split(p[1:], "/")
	pat := make([]string, len(segs))
	shape := make([]string, len(segs))
	seen := map[string]bool{}
	for i, s := range segs {
		last := i == len(segs)-1
		switch {
		case s == "":
			return nil, fmt.Errorf("route %q: path มี // ไม่ได้", spec)
		case s == "*":
			if !last {
				return nil, fmt.Errorf("route %q: * ใช้ได้เฉพาะท้าย path", spec)
			}
			pat[i], shape[i] = "{_rest...}", "*"
			rs.Params = append(rs.Params, "*")
		case strings.HasPrefix(s, "{"):
			m := paramRe.FindStringSubmatch(s)
			if m == nil {
				return nil, fmt.Errorf("route %q: พารามิเตอร์ %s ไม่ถูกต้อง (ใช้ {name} หรือ {name...} ท้าย path)", spec, s)
			}
			if m[2] != "" && !last {
				return nil, fmt.Errorf("route %q: {name...} ใช้ได้เฉพาะท้าย path", spec)
			}
			if seen[m[1]] || m[1] == "_rest" {
				return nil, fmt.Errorf("route %q: ชื่อพารามิเตอร์ซ้ำ: %s", spec, m[1])
			}
			seen[m[1]] = true
			pat[i] = s
			if m[2] != "" {
				shape[i] = "*"
			} else {
				shape[i] = "{}"
			}
			rs.Params = append(rs.Params, m[1])
		case s == "." || s == ".." || !literalRe.MatchString(s):
			return nil, fmt.Errorf("route %q: ส่วนของ path %q ใช้ได้เฉพาะ A-Z a-z 0-9 . _ ~ -", spec, s)
		default:
			pat[i], shape[i] = s, s
		}
	}
	rs.Pattern = method + " /" + strings.Join(pat, "/")
	rs.Shape = "/" + strings.Join(shape, "/")
	return rs, nil
}

// firstSegment returns the first literal segment of a route path ("" when
// it starts with a parameter or is the root).
func (rs *routeSpec) firstSegment() string {
	first, _, _ := strings.Cut(strings.TrimPrefix(rs.Path, "/"), "/")
	if strings.HasPrefix(first, "{") || first == "*" {
		return ""
	}
	return strings.ToLower(first)
}

// checkAccess validates an access list: "*" (anyone), role slugs,
// "group:<name>" (visitor groups) and "cap:<capability>" (users whose role
// holds it, since 2.0.6); empty = administrator only.
func checkAccess(list []string) ([]string, error) {
	if len(list) == 0 {
		return []string{"administrator"}, nil
	}
	if len(list) > 50 {
		return nil, fmt.Errorf("access: ระบุได้ไม่เกิน 50 รายการ")
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		if g, ok := strings.CutPrefix(a, "group:"); ok {
			if !groupNameRe.MatchString(g) {
				return nil, fmt.Errorf("access: %q ไม่ใช่กลุ่มที่ถูกต้อง (ใช้ 'group:member')", a)
			}
		} else if strings.HasPrefix(a, CapPrefix) {
			if !validCapEntry(a) {
				return nil, fmt.Errorf("access: %q ไม่ใช่สิทธิ์ที่ถูกต้อง (ใช้ 'cap:edit_pages' หรือ 'cap:<รหัสปลั๊กอิน>.<สิทธิ์>')", a)
			}
		} else if a != "*" && !accessRe.MatchString(a) {
			return nil, fmt.Errorf("access: %q ไม่ใช่ role ที่ถูกต้อง (ใช้ '*', รหัส role เช่น 'editor', 'group:member' หรือ 'cap:<สิทธิ์>')", a)
		}
		out = append(out, a)
	}
	return out, nil
}

// Reserved returns the first path segments Site Code routes may not use.
func Reserved(opts map[string]string, servicePrefixes []string) map[string]bool {
	r := map[string]bool{"api": true, "admin": true, "uploads": true, "download": true, "plugin-assets": true, "_a": true, "_c": true,
		"ext-ui": true, "sitemap.xml": true, "robots.txt": true, "favicon.ico": true}
	add := func(v, def string) {
		v = strings.ToLower(strings.Trim(v, "/ "))
		if v == "" {
			v = def
		}
		r[v] = true
	}
	add(opts["admin_path"], "admin")
	add(opts["blog_base"], "blog")
	add(opts["category_base"], "category")
	for _, p := range servicePrefixes {
		first, _, _ := strings.Cut(strings.TrimPrefix(p, "/"), "/")
		if first != "" {
			r[strings.ToLower(first)] = true
		}
	}
	return r
}

// requestReserved reports whether a request path starts with a reserved
// segment (never handed to Site Code, whatever the patterns match).
func requestReserved(path string, reserved map[string]bool) bool {
	first, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	return reserved[strings.ToLower(first)]
}

// newMux builds the route matcher; handler(i) serves route i. A conflict
// between two patterns (ServeMux panics) is returned as an error.
func newMux(specs []*routeSpec, handler func(i int) http.Handler) (mux *http.ServeMux, err error) {
	mux = http.NewServeMux()
	for i, rs := range specs {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					err = fmt.Errorf("route \"%s %s\" ชนกับ route อื่น: %v", rs.Method, rs.Path, rec)
				}
			}()
			mux.Handle(rs.Pattern, handler(i))
		}()
		if err != nil {
			return nil, err
		}
	}
	return mux, nil
}
