package sitecode

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Problem is a build error or warning at a position of an original file.
type Problem struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

// BuildResult is the outcome of bundling the project.
type BuildResult struct {
	Bundle    string    // IIFE bundle with an inline source map (what goja runs)
	SourceMap string    // the same source map as JSON (stored with a version)
	Errors    []Problem // empty = success
	Warnings  []Problem
}

// OK reports whether the build produced a bundle.
func (b *BuildResult) OK() bool { return len(b.Errors) == 0 }

// ns is the esbuild namespace of the in-memory project files.
const ns = "site"

// esbuildSupported tells esbuild what goja cannot run, so it lowers it.
// goja runs ES2022 (classes with private fields and static blocks, async/
// await, optional chaining, BigInt, named groups / lookbehind) but has no
// async generators and no RegExp "d" flag.
var esbuildSupported = map[string]bool{"async-generator": false, "for-await": true, "regexp-match-indices": false}

// Build bundles files (TS/JS, imports between files) into one IIFE script
// for goja with an inline source map pointing at the original files.
func Build(files map[string]string) *BuildResult {
	res := &BuildResult{Errors: []Problem{}, Warnings: []Problem{}}
	entry := EntryPoint(files)
	if entry == "" {
		res.Errors = append(res.Errors, Problem{Message: "ไม่พบไฟล์เริ่มต้น: สร้าง main.ts (หรือ main.js, index.ts, index.js)"})
		return res
	}
	return buildEntry(files, entry, "")
}

// buildEntry bundles one entry file; globalName exposes its exports (used
// for migration files: __migration.default).
func buildEntry(files map[string]string, entry, globalName string) *BuildResult {
	return buildEntryWith(files, entry, globalName, nil)
}

// buildEntryWith is buildEntry with plugin imports resolved by imp (nil =
// relative imports only).
func buildEntryWith(files map[string]string, entry, globalName string, imp *importer) *BuildResult {
	res := &BuildResult{Errors: []Problem{}, Warnings: []Problem{}}
	out := api.Build(api.BuildOptions{
		GlobalName:        globalName,
		EntryPoints:       []string{entry},
		Bundle:            true,
		Write:             false,
		Outfile:           "bundle.js",
		Format:            api.FormatIIFE,
		Platform:          api.PlatformNeutral,
		Target:            api.ES2022,
		Supported:         esbuildSupported,
		Sourcemap:         api.SourceMapExternal,
		SourcesContent:    api.SourcesContentExclude,
		LogLevel:          api.LogLevelSilent,
		Charset:           api.CharsetUTF8,
		MainFields:        []string{"module", "main"},
		ResolveExtensions: []string{".ts", ".js", ".mjs", ".json"},
		Plugins:           []api.Plugin{memoryFiles(files, imp)},
	})
	for _, m := range out.Errors {
		res.Errors = append(res.Errors, problem(m))
	}
	for _, m := range out.Warnings {
		res.Warnings = append(res.Warnings, problem(m))
	}
	if len(res.Errors) > 0 {
		return res
	}
	var js, sm []byte
	for _, f := range out.OutputFiles {
		if strings.HasSuffix(f.Path, ".map") {
			sm = f.Contents
		} else {
			js = f.Contents
		}
	}
	sm = cleanSourceMap(sm)
	code := string(js)
	if i := strings.LastIndex(code, "//# sourceMappingURL="); i >= 0 {
		code = code[:i]
	}
	res.Bundle = code + "//# sourceMappingURL=data:application/json;base64," + base64.StdEncoding.EncodeToString(sm) + "\n"
	res.SourceMap = string(sm)
	return res
}

// cleanSourceMap rewrites the sources ("site:main.ts" → "main.ts") so stack
// traces show the project paths.
func cleanSourceMap(sm []byte) []byte {
	var m map[string]any
	if json.Unmarshal(sm, &m) != nil {
		return sm
	}
	if srcs, ok := m["sources"].([]any); ok {
		for i, s := range srcs {
			if str, ok := s.(string); ok {
				srcs[i] = projectPath(str)
			}
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return sm
	}
	return b
}

// projectPath strips the namespace / relative prefixes esbuild adds; files
// of another plugin's library show as "@<id>/<path>".
func projectPath(p string) string {
	if rest, ok := strings.CutPrefix(p, libNS+":"); ok {
		return "@" + rest
	}
	if rest, ok := strings.CutPrefix(p, pluginNS+":"); ok {
		return "@" + rest
	}
	p = strings.TrimPrefix(p, ns+":")
	for strings.HasPrefix(p, "../") {
		p = strings.TrimPrefix(p, "../")
	}
	return strings.TrimPrefix(p, "./")
}

func problem(m api.Message) Problem {
	p := Problem{Message: m.Text}
	if m.Location != nil {
		p.File, p.Line, p.Column = projectPath(m.Location.File), m.Location.Line, m.Location.Column+1
	}
	return p
}

// Namespaces of plugin imports: "plugin:<id>" is the generated module of
// `import … from '<id>'`, "lib:<id>/<path>" a file of that plugin's library.
const (
	pluginNS  = "plugin"
	libNS     = "lib"
	libPrefix = "anasaki-lib:"
)

// importer resolves `import … from '<plugin id>'` for a unit's build.
type importer struct {
	resolve func(id string) (*depInfo, string) // the dependency, or why it cannot be imported
	cache   map[string]*depInfo
}

// depInfo is a plugin another unit imports.
type depInfo struct {
	id      string
	library string   // the library entry ("" = none)
	exports []string // code.exports (proxies running in the plugin)
	files   map[string]string
}

func (imp *importer) dep(id string) (*depInfo, string) {
	if imp == nil {
		return nil, "import ปลั๊กอินไม่ได้ในที่นี้"
	}
	if imp.cache == nil {
		imp.cache = map[string]*depInfo{}
	}
	if d, ok := imp.cache[id]; ok {
		return d, ""
	}
	d, msg := imp.resolve(id)
	if d != nil {
		imp.cache[id] = d
	}
	return d, msg
}

// importerFor resolves plugin imports of unit u against the loaded units:
// a plugin may import what its requires lists, Site Code any active plugin.
func (e *Engine) importerFor(u *unitInfo, set *unitSet) *importer {
	return &importer{resolve: func(id string) (*depInfo, string) {
		if u.plugin() {
			if id == u.ID {
				return nil, fmt.Sprintf("import %q: ปลั๊กอิน import ตัวเองไม่ได้ (ใช้ ./)", id)
			}
			if _, ok := u.requires[id]; !ok {
				return nil, fmt.Sprintf("import %q: ปลั๊กอิน %s ไม่ได้อยู่ใน requires ของ manifest", id, id)
			}
		}
		lp := set.get(id)
		if lp == nil {
			return nil, fmt.Sprintf("import %q: ปลั๊กอิน %s ไม่ได้ติดตั้งหรือยังไม่ได้เปิดใช้งาน", id, id)
		}
		var ex []string
		for n := range lp.unit.exports {
			ex = append(ex, n)
		}
		sort.Strings(ex)
		return &depInfo{id: id, library: lp.unit.library, exports: ex, files: lp.files}, ""
	}}
}

// pluginModule is the generated module of `import … from '<id>'`: the
// library re-exported (bundled into the importer, runs with the importer's
// scopes) plus one proxy per export (runs in the plugin's own context).
func pluginModule(d *depInfo) string {
	var b strings.Builder
	if d.library != "" {
		fmt.Fprintf(&b, "export * from %q;\n", libPrefix+d.id+"/"+d.library)
	}
	for _, n := range d.exports {
		fmt.Fprintf(&b, "export const %s = /* @__PURE__ */ __anasaki_export(%q, %q);\n", n, d.id, n)
	}
	if b.Len() == 0 {
		b.WriteString("export {};\n")
	}
	return b.String()
}

func lookupIn(files map[string]string, p string) (string, bool) {
	if _, ok := files[p]; ok {
		return p, true
	}
	for _, ext := range []string{".ts", ".js", ".mjs", ".json", "/index.ts", "/index.js"} {
		if _, ok := files[p+ext]; ok {
			return p + ext, true
		}
	}
	return "", false
}

func loaderFor(p string) api.Loader {
	switch strings.ToLower(path.Ext(p)) {
	case ".ts":
		return api.LoaderTS
	case ".json":
		return api.LoaderJSON
	case ".md":
		return api.LoaderText
	}
	return api.LoaderJS
}

func resolveErr(msg string) (api.OnResolveResult, error) {
	return api.OnResolveResult{Errors: []api.Message{{Text: msg}}}, nil
}

// memoryFiles resolves and loads imports from the project map: relative
// imports between project files and, with imp, plugin imports.
func memoryFiles(files map[string]string, imp *importer) api.Plugin {
	lookup := func(p string) (string, bool) { return lookupIn(files, p) }
	return api.Plugin{Name: "site-files", Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(a api.OnResolveArgs) (api.OnResolveResult, error) {
			if a.Kind == api.ResolveEntryPoint {
				return api.OnResolveResult{Path: a.Path, Namespace: ns}, nil
			}
			if rest, ok := strings.CutPrefix(a.Path, libPrefix); ok && imp != nil && a.Namespace == pluginNS {
				return api.OnResolveResult{Path: rest, Namespace: libNS}, nil
			}
			if a.Namespace == libNS { // inside another plugin's library: its own files only
				id, rel, _ := strings.Cut(a.Importer, "/")
				d, _ := imp.dep(id)
				if d == nil || (!strings.HasPrefix(a.Path, "./") && !strings.HasPrefix(a.Path, "../")) {
					return resolveErr("library ของปลั๊กอิน " + id + " import ได้เฉพาะไฟล์ของตัวเอง: " + a.Path)
				}
				target := path.Clean(path.Join(path.Dir(rel), a.Path))
				if p, ok := lookupIn(d.files, target); ok && !strings.HasPrefix(target, "../") {
					return api.OnResolveResult{Path: id + "/" + p, Namespace: libNS}, nil
				}
				return resolveErr("ไม่พบไฟล์ใน library ของปลั๊กอิน " + id + ": " + a.Path)
			}
			if !strings.HasPrefix(a.Path, "./") && !strings.HasPrefix(a.Path, "../") {
				id := strings.TrimPrefix(a.Path, "anasaki:")
				if imp != nil && unitIDRe.MatchString(id) {
					if d, msg := imp.dep(id); d == nil {
						return resolveErr(msg)
					}
					return api.OnResolveResult{Path: id, Namespace: pluginNS}, nil
				}
				if imp != nil {
					return resolveErr("import ได้เฉพาะไฟล์ในโปรเจกต์ (./ หรือ ../) หรือปลั๊กอินที่ติดตั้งไว้ (เช่น 'money') — ไม่รองรับแพ็กเกจ: " + a.Path)
				}
				return resolveErr("import ได้เฉพาะไฟล์ในโปรเจกต์ (ขึ้นต้นด้วย ./ หรือ ../) — ไม่รองรับแพ็กเกจ: " + a.Path)
			}

			target := path.Clean(path.Join(path.Dir(a.Importer), a.Path))
			if strings.HasPrefix(target, "../") || target == ".." {
				return api.OnResolveResult{Errors: []api.Message{{Text: "import ออกนอกโปรเจกต์ไม่ได้: " + a.Path}}}, nil
			}
			if p, ok := lookup(target); ok {
				return api.OnResolveResult{Path: p, Namespace: ns}, nil
			}
			return api.OnResolveResult{Errors: []api.Message{{Text: "ไม่พบไฟล์: " + a.Path}}}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: pluginNS}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			d, msg := imp.dep(a.Path)
			if d == nil {
				return api.OnLoadResult{Errors: []api.Message{{Text: msg}}}, nil
			}
			c := pluginModule(d)
			return api.OnLoadResult{Contents: &c, Loader: api.LoaderJS}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: libNS}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			id, rel, _ := strings.Cut(a.Path, "/")
			d, _ := imp.dep(id)
			if d == nil {
				return api.OnLoadResult{Errors: []api.Message{{Text: "ไม่พบปลั๊กอิน " + id}}}, nil
			}
			c, ok := d.files[rel]
			if !ok {
				return api.OnLoadResult{Errors: []api.Message{{Text: "ไม่พบไฟล์: @" + a.Path}}}, nil
			}
			return api.OnLoadResult{Contents: &c, Loader: loaderFor(rel)}, nil
		})
		b.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: ns}, func(a api.OnLoadArgs) (api.OnLoadResult, error) {
			c, ok := files[a.Path]
			if !ok {
				return api.OnLoadResult{Errors: []api.Message{{Text: "ไม่พบไฟล์: " + a.Path}}}, nil
			}
			loader := api.LoaderJS
			switch strings.ToLower(path.Ext(a.Path)) {
			case ".ts":
				loader = api.LoaderTS
			case ".json":
				loader = api.LoaderJSON
			case ".md":
				loader = api.LoaderText
			}
			return api.OnLoadResult{Contents: &c, Loader: loader}, nil
		})
	}}
}
