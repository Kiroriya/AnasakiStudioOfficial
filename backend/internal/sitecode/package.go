package sitecode

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// Phase 4b: export the live Site Code as a JS plugin package (POST
// /api/v1/code/package): code/… + migrations/… + a pre-filled manifest that
// passes the plugin package validator.

// PackageRequest is the body of POST /code/package. Nil lists are inferred.
type PackageRequest struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Scopes      *[]string `json:"scopes"`
	Routes      *[]string `json:"routes"`
	Library     string    `json:"library"`
	Exports     *[]string `json:"exports"`
}

// PackageWarning is something the developer should check in the package.
type PackageWarning struct {
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

// PackageResult is the dry run ({manifest, warnings}) and the zip.
type PackageResult struct {
	Manifest map[string]any   `json:"manifest"`
	Warnings []PackageWarning `json:"warnings"`
	Zip      []byte           `json:"-"`
	FileName string           `json:"fileName"`
}

// ErrBadPackage is a refused export (400).
type ErrBadPackage struct{ Msg string }

func (e *ErrBadPackage) Error() string { return e.Msg }

var (
	pkgIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,39}$`)
	semverLike   = regexp.MustCompile(`^\d{1,6}\.\d{1,6}\.\d{1,6}(?:-[0-9A-Za-z.-]{1,40})?$`)
	hookRenameRe = regexp.MustCompile(`(site\.(?:applyFilter|emit|filter|on)\(\s*['"` + "`" + `])site\.([a-zA-Z0-9][a-zA-Z0-9._:-]*)`)
	hookUseRe    = regexp.MustCompile(`site\.(applyFilter|emit)\(\s*['"` + "`" + `]([a-z0-9][a-z0-9-]*\.[a-zA-Z0-9][a-zA-Z0-9._:-]*)`)
	importRe     = regexp.MustCompile(`(?m)\bfrom\s+['"](?:anasaki:)?([a-z0-9][a-z0-9-]{2,39})['"]`)
	scopeUses    = []struct {
		re    *regexp.Regexp
		scope string
	}{
		{regexp.MustCompile(`\bdb\.(table|from|docs|transaction|raw)\s*\(`), "db"},
		{regexp.MustCompile(`\bdb\.posts\.(list|get)\s*\(`), "content"},
		{regexp.MustCompile(`\bdb\.posts\.(create|update|delete)\s*\(|\bdb\.asSystem\s*\(`), "content-write"},
		{regexp.MustCompile(`\bmail\.send\s*\(`), "mail"},
		{regexp.MustCompile(`\bhttp\.fetch\s*\(`), "http"},
		{regexp.MustCompile(`\bsite\.visitor\s*\(`), "visitor"},
		{regexp.MustCompile(`\bsite\.adminPage\s*\(`), "admin-pages"},
		{regexp.MustCompile(`\bsite\.(job|cron)\s*\(|\bjobs\.enqueue\s*\(`), "jobs"},
		{regexp.MustCompile(`\bsite\.route\s*\(`), "routes"},
		{regexp.MustCompile(`\bsite\.(middleware|afterResponse)\s*\(`), "middleware"},
		{regexp.MustCompile(`\bsite\.(head|bodyEnd)\s*\(`), "head-scripts"},
		{regexp.MustCompile(`\bsite\.(fields|meta|setMeta)\s*\(`), "content-fields"},
		{regexp.MustCompile(`\bsite\.postType\s*\(`), "post-types"},
		{regexp.MustCompile(`\bsite\.taxonomy\s*\(`), "taxonomies"},
		{regexp.MustCompile(`\bdb\.terms\.(list|get)\s*\(`), "content"},
		{regexp.MustCompile(`\bdb\.terms\.(create|update|delete)\s*\(`), "terms-write"},
		{regexp.MustCompile(`\bmedia\.(list|get|read)\s*\(`), "media"},
		{regexp.MustCompile(`\bmedia\.(upload|update|replace|delete)\s*\(`), "media-write"},
		{regexp.MustCompile(`\bfiles\.(put|get|text|list|delete|url)\s*\(`), "files"},
		{regexp.MustCompile(`\bhttp\.webhook\s*\(`), "webhooks"},
		{regexp.MustCompile(`\bsite\.(adminNotice|dashboardWidget)\s*\(`), "admin-pages"},
	}
)

// reservedExportRoutes are first segments a packaged route may not use.
var reservedExportRoutes = map[string]bool{"api": true, "admin": true, "uploads": true, "download": true, "plugin-assets": true,
	"_a": true, "_c": true, "ext-ui": true, "sitemap.xml": true, "robots.txt": true, "favicon.ico": true, "favicon.svg": true,
	"assets": true, "static": true, "blog": true, "category": true, "preview": true}

func lineOf(src string, idx int) int { return strings.Count(src[:idx], "\n") + 1 }

// ExportPackage restructures the live version as a plugin package.
func (e *Engine) ExportPackage(in PackageRequest, dryRun bool) (*PackageResult, error) {
	live, err := e.LiveVersion()
	if err != nil {
		return nil, err
	}
	if live == 0 {
		return nil, &ErrBadPackage{Msg: "ยังไม่มีเวอร์ชันที่เผยแพร่ — เผยแพร่ก่อนแล้วจึงส่งออกเป็นปลั๊กอิน"}
	}
	row, err := e.DB.GetVersion(live, live, false)
	if err != nil {
		return nil, err
	}
	in.ID, in.Name, in.Version = strings.TrimSpace(in.ID), strings.TrimSpace(in.Name), strings.TrimSpace(in.Version)
	if in.ID == "" && dryRun {
		in.ID = "my-site-code"
	}
	if in.Name == "" {
		in.Name = in.ID
	}
	if in.Version == "" {
		in.Version = "1.0.0"
	}
	if !pkgIDPattern.MatchString(in.ID) || in.ID == SiteUnit {
		return nil, &ErrBadPackage{Msg: "id ต้องเป็น a-z 0-9 - ยาว 3–40 ตัว (ขึ้นต้นด้วยตัวอักษรหรือตัวเลข) และไม่ใช่ 'site'"}
	}
	if !semverLike.MatchString(in.Version) {
		return nil, &ErrBadPackage{Msg: "version ต้องเป็น semver เช่น 1.0.0"}
	}
	if len([]rune(in.Name)) > 100 || len([]rune(in.Description)) > 1000 {
		return nil, &ErrBadPackage{Msg: "name ยาวไม่เกิน 100 ตัว และ description ไม่เกิน 1000 ตัว"}
	}
	m := row.Manifest
	res := &PackageResult{Warnings: []PackageWarning{}, FileName: in.ID + "-" + in.Version + ".zip"}
	warn := func(file string, line int, format string, a ...any) {
		res.Warnings = append(res.Warnings, PackageWarning{Message: fmt.Sprintf(format, a...), File: file, Line: line})
	}
	entry := EntryPoint(row.FileMap)
	files := map[string]string{}
	scopes := map[string]bool{}
	hooks := map[string]map[string]bool{"applyFilter": {}, "emit": {}}
	requires := map[string]string{}
	set := e.units()
	for _, p := range Sorted(row.FileMap) {
		src := row.FileMap[p]
		dst := "code/" + p
		if strings.HasPrefix(p, "migrations/") {
			dst = p
			if strings.Contains(src, "'../") || strings.Contains(src, "\"../") {
				warn(p, 1, "migration import ไฟล์นอกโฟลเดอร์ migrations/ — ในแพ็กเกจไฟล์โค้ดอยู่ใต้ code/ ตรวจ path ของ import")
			}
		}
		if isScript(p) {
			for _, mt := range hookRenameRe.FindAllStringSubmatchIndex(src, -1) {
				name := src[mt[4]:mt[5]]
				warn(dst, lineOf(src, mt[0]), "เปลี่ยนชื่อ hook site.%s เป็น %s.%s", name, in.ID, name)
			}
			src = hookRenameRe.ReplaceAllString(src, "${1}"+in.ID+".$2")
			for _, u := range scopeUses {
				if u.re.MatchString(src) {
					scopes[u.scope] = true
				}
			}
			for _, mt := range hookUseRe.FindAllStringSubmatch(src, -1) {
				hooks[mt[1]][mt[2]] = true
			}
			for _, mt := range importRe.FindAllStringSubmatch(src, -1) {
				if lp := set.get(mt[1]); lp != nil {
					requires[mt[1]] = "^" + lp.unit.Version
				}
			}
			if strings.Contains(src, "site.settings(") {
				warn(dst, 0, "site.settings ในปลั๊กอินอ่านค่าของปลั๊กอินเอง (%s.<key>) — ต้องตั้งค่าใหม่หลังติดตั้ง", in.ID)
			}
			if strings.Contains(src, "db.query(") || strings.Contains(src, "schema.raw(") {
				warn(dst, 0, "db.query / schema.raw ใช้ไม่ได้ในปลั๊กอิน — เปลี่ยนเป็น query builder / schema builder")
			}
		}
		files[dst] = src
	}
	if len(MigrationFilesOnly(row.FileMap)) > 0 {
		scopes["db"] = true
	}
	for _, f := range m.Filters {
		if coreFilters[f.Name] {
			scopes["filters-core"] = true
		}
	}
	if len(m.PostTypes) > 0 {
		warn("", 0, "site.postType ในปลั๊กอินต้องใช้ชื่อประเภท %s<ชื่อ> (scope post-types, CMS 2.0.5) — เปลี่ยนชื่อประเภทในโค้ด โพสต์เดิมของประเภท %s ไม่ย้ายตาม", PluginTypePrefix(in.ID), m.PostTypes[0].Type)
	}
	if len(m.Fields) > 0 {
		warn("", 0, "site.fields ในปลั๊กอินเก็บค่าเป็น <id>.<ชื่อฟิลด์> ใน post meta (ค่าเดิมของ Site Code ไม่ย้ายตาม)")
	}
	if len(m.Formatters) > 0 {
		warn("", 0, "site.formatter ในปลั๊กอินใช้ชื่อ %s:<ชื่อ> ในนิพจน์ (เช่น | %s:%s) — ผู้ดูแลเปิดชื่อย่อได้ที่หน้าปลั๊กอิน", in.ID, in.ID, m.Formatters[0])
	}
	// routes: the first segment of every registered route
	routeSet := map[string]bool{}
	for _, r := range m.Routes {
		first, _, _ := strings.Cut(strings.TrimPrefix(r.Path, "/"), "/")
		if first == "" || strings.HasPrefix(first, "{") || first == "*" {
			warn("", 0, "route %s %s ไม่มีส่วนแรกคงที่ — ปลั๊กอินต้องประกาศ prefix ใน code.routes", r.Method, r.Path)
			continue
		}
		routeSet["/"+first] = true
	}
	var routes []string
	if in.Routes != nil {
		routes = *in.Routes
	} else {
		for r := range routeSet {
			routes = append(routes, r)
		}
		sort.Strings(routes)
	}
	for _, r := range routes {
		first, _, _ := strings.Cut(strings.TrimPrefix(r, "/"), "/")
		if reservedExportRoutes[first] {
			return nil, &ErrBadPackage{Msg: fmt.Sprintf("route %q เป็น path ที่สงวนไว้ของ CMS", r)}
		}
	}
	var scopeList []string
	if in.Scopes != nil {
		scopeList = *in.Scopes
	} else {
		if len(routes) > 0 {
			scopes["routes"] = true
		}
		for s := range scopes {
			scopeList = append(scopeList, s)
		}
		sort.Strings(scopeList)
	}
	exports := m.Exports
	if in.Exports != nil {
		exports = *in.Exports
	}
	code := map[string]any{"entry": "code/" + entry, "scopes": nonNilStrings(scopeList)}
	if len(routes) > 0 {
		code["routes"] = routes
	}
	if len(exports) > 0 {
		code["exports"] = exports
	}
	hasScope := func(s string) bool {
		for _, x := range scopeList {
			if x == s {
				return true
			}
		}
		return false
	}
	if hasScope("http") {
		opts, _ := e.st.Options()
		if hosts := FetchHosts(opts); len(hosts) > 0 {
			code["hosts"] = hosts
			warn("", 0, "code.hosts ใช้รายการ host ที่อนุญาตของเว็บนี้ — ตรวจให้เหลือเฉพาะที่ปลั๊กอินใช้")
		}
	}
	if lib := strings.TrimPrefix(strings.TrimSpace(in.Library), "code/"); lib != "" {
		if _, ok := row.FileMap[lib]; !ok {
			return nil, &ErrBadPackage{Msg: "ไม่พบไฟล์ library: " + lib}
		}
		code["library"] = "code/" + lib
	}
	hk := map[string]any{}
	for kind, key := range map[string]string{"applyFilter": "filters", "emit": "events"} {
		var l []string
		for n := range hooks[kind] {
			if strings.HasPrefix(n, in.ID+".") {
				l = append(l, n)
			}
		}
		sort.Strings(l)
		if len(l) > 0 {
			hk[key] = l
		}
	}
	if len(hk) > 0 {
		code["hooks"] = hk
	}
	man := map[string]any{"id": in.ID, "name": in.Name, "version": in.Version, "minCore": plugin.CoreVersion,
		"runtime": plugin.RuntimeJS, "permissions": []string{"code"}, "code": code}
	if d := strings.TrimSpace(in.Description); d != "" {
		man["description"] = d
	}
	if len(requires) > 0 {
		man["requires"] = requires
	}
	res.Manifest = man
	mj, _ := json.MarshalIndent(man, "", "  ")
	readme := fmt.Sprintf("# %s\n\n%s\n\nส่งออกจาก Site Code เวอร์ชัน #%d (%s)\n", in.Name, in.Description, live, time.Now().UTC().Format("2006-01-02"))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, content string) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: zipTime})
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	if err := write("manifest.json", string(mj)); err != nil {
		return nil, err
	}
	if err := write("README.md", readme); err != nil {
		return nil, err
	}
	for _, p := range Sorted(files) {
		if err := write(p, files[p]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if _, _, err := plugin.ParsePackage(buf.Bytes()); err != nil {
		var pe *plugin.PackageError
		if errors.As(err, &pe) {
			return nil, &ErrBadPackage{Msg: "แพ็กเกจไม่ผ่านการตรวจ: " + pe.Msg}
		}
		return nil, err
	}
	if !dryRun {
		res.Zip = buf.Bytes()
	}
	return res, nil
}

func isScript(p string) bool {
	return strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs")
}

func nonNilStrings(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}
