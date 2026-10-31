package sitecode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

// Phase 5: anasaki.d.ts gets one `declare module '<id>'` block per active
// JavaScript plugin, so `import { x } from '<id>'` is typed in VS Code: the
// plugin's own code/index.d.ts when it ships one, otherwise its library's
// export names (typed any) and its code.exports proxies (functions running
// in the plugin, JSON arguments and result).

// PluginTypesFile is the declaration file a JS plugin may ship.
const PluginTypesFile = "code/index.d.ts"

// moduleInfo is what the d.ts says about one plugin module.
type moduleInfo struct {
	ID      string
	Version string
	DTS     string   // the plugin's code/index.d.ts ("" = none)
	Library []string // export names of code.library
	Exports []string // code.exports
}

// libExportCache: library export names per installed plugin version.
var libExportCache sync.Map // "<id>@<version>#<installed>" → []string

// libraryExports lists the export names of a plugin library (ESM build with
// a metafile; nil when it does not build).
func libraryExports(files map[string]string, entry string) []string {
	if _, ok := files[entry]; !ok {
		return nil
	}
	out := api.Build(api.BuildOptions{
		EntryPoints:       []string{entry},
		Bundle:            true,
		Write:             false,
		Outfile:           "lib.js",
		Format:            api.FormatESModule,
		Platform:          api.PlatformNeutral,
		Target:            api.ES2022,
		Metafile:          true,
		LogLevel:          api.LogLevelSilent,
		ResolveExtensions: []string{".ts", ".js", ".mjs", ".json"},
		Plugins:           []api.Plugin{memoryFiles(files, nil)},
	})
	if len(out.Errors) > 0 {
		return nil
	}
	var meta struct {
		Outputs map[string]struct {
			Exports []string `json:"exports"`
		} `json:"outputs"`
	}
	if json.Unmarshal([]byte(out.Metafile), &meta) != nil {
		return nil
	}
	var names []string
	for _, o := range meta.Outputs {
		names = append(names, o.Exports...)
	}
	sort.Strings(names)
	return names
}

var (
	jsIdentRe     = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
	ambientDeclRe = regexp.MustCompile(`(?m)^([ \t]*(?:export[ \t]+)?)declare[ \t]+`)
)

// ambientBody adapts a plugin's .d.ts to the inside of a `declare module`
// block (a `declare` modifier is an error in an ambient context).
func ambientBody(dts string) string {
	return ambientDeclRe.ReplaceAllString(dts, "$1")
}

// moduleTypes renders the `declare module` blocks.
func moduleTypes(mods []moduleInfo) string {
	if len(mods) == 0 {
		return ""
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].ID < mods[j].ID })
	var b strings.Builder
	b.WriteString("\n// ---------------------------------------------------------------- active JavaScript plugins\n")
	b.WriteString("// import { … } from '<plugin id>' (generated). / โมดูลของปลั๊กอิน JS ที่เปิดใช้งาน (สร้างอัตโนมัติ)\n")
	for _, m := range mods {
		fmt.Fprintf(&b, "\n/** Plugin `%s` %s. */\ndeclare module '%s' {\n", m.ID, m.Version, m.ID)
		if strings.TrimSpace(m.DTS) != "" {
			for _, l := range strings.Split(strings.TrimRight(ambientBody(m.DTS), "\n"), "\n") {
				b.WriteString("  " + l + "\n")
			}
		} else {
			seen := map[string]bool{}
			for _, n := range m.Library {
				if seen[n] || (!jsIdentRe.MatchString(n) && n != "default") {
					continue
				}
				seen[n] = true
				if n == "default" {
					b.WriteString("  /** Library (bundled into your code). */\n  const _default: any;\n  export default _default;\n")
					continue
				}
				fmt.Fprintf(&b, "  /** Library (bundled into your code). */\n  export const %s: any;\n", n)
			}
			for _, n := range m.Exports {
				if seen[n] || !jsIdentRe.MatchString(n) {
					continue
				}
				seen[n] = true
				fmt.Fprintf(&b, "  /** Runs in plugin %s (JSON arguments and result). */\n  export function %s(...args: any[]): any;\n", m.ID, n)
			}
		}
		b.WriteString("}\n")
		fmt.Fprintf(&b, "declare module 'anasaki:%s' {\n  export * from '%s';\n}\n", m.ID, m.ID)
	}
	return b.String()
}

// pluginModules describes the loaded JS plugin units.
func (e *Engine) pluginModules() []moduleInfo {
	set := e.units()
	var mods []moduleInfo
	for _, lp := range set.loaded() {
		u := lp.unit
		if u == nil || !u.plugin() {
			continue
		}
		m := moduleInfo{ID: u.ID, Version: u.Version, DTS: lp.files[PluginTypesFile]}
		if m.DTS == "" {
			for n := range u.exports {
				m.Exports = append(m.Exports, n)
			}
			sort.Strings(m.Exports)
			if u.library != "" {
				key := u.ID + "@" + u.Version + "#" + u.InstalledAt.UTC().Format(time.RFC3339Nano) + "#" + u.library
				if v, ok := libExportCache.Load(key); ok {
					m.Library = v.([]string)
				} else {
					m.Library = libraryExports(lp.files, u.library)
					libExportCache.Store(key, m.Library)
				}
			}
		}
		mods = append(mods, m)
	}
	return mods
}
