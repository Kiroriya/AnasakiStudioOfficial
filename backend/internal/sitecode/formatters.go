package sitecode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/design"
)

// Phase 6: formatters from JS plugins. A plugin's site.formatter is used as
// "<id>:<name>" in expressions ({{ price | thai:baht }}). The administrator
// may enable an unprefixed alias per plugin formatter ("baht" → "thai:baht",
// option OptFormatterAliases). Resolution of an unprefixed name: a core
// formatter (the evaluator handles it), then Site Code's own formatter, then
// the enabled alias. Each alias names exactly one formatter, so the first
// plugin whose alias was enabled keeps the name; the others show a conflict.

// OptFormatterAliases is the option holding the enabled aliases as JSON
// {"baht": "thai:baht"}.
const OptFormatterAliases = "site_code_formatter_aliases"

// FormatterRow is one formatter of GET /api/v1/code/formatters.
type FormatterRow struct {
	Name     string `json:"name"`               // "baht" (Site Code) or "thai:baht" (a JS plugin)
	Unit     string `json:"unit"`               // "site" or the plugin id
	Alias    string `json:"alias,omitempty"`    // the enabled unprefixed alias
	Conflict string `json:"conflict,omitempty"` // who holds the unprefixed name instead: "core", "site" or "<id>:<name>"
}

// AliasesFrom reads the enabled aliases from options.
func AliasesFrom(opts map[string]string) map[string]string {
	out := map[string]string{}
	if raw := strings.TrimSpace(opts[OptFormatterAliases]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

func (e *Engine) formatterAliases() map[string]string {
	if e == nil || e.st == nil || e.st.DB == nil {
		return map[string]string{}
	}
	opts, err := e.st.Options()
	if err != nil {
		return map[string]string{}
	}
	return AliasesFrom(opts)
}

// ErrBadAliases is a refused PUT /code/formatters/aliases.
type ErrBadAliases struct{ Msg string }

func (e *ErrBadAliases) Error() string { return e.Msg }

var aliasNameRe = regexp.MustCompile(`^[a-z][a-zA-Z0-9_-]{0,62}$`)

// SetFormatterAliases replaces the enabled aliases: each key is the local
// name of the plugin formatter it points to ({"baht": "thai:baht"}).
func (e *Engine) SetFormatterAliases(aliases map[string]string) error {
	if len(aliases) > 500 {
		return &ErrBadAliases{Msg: "เปิดชื่อย่อได้ไม่เกิน 500 รายการ"}
	}
	core := map[string]bool{}
	for _, n := range design.CoreFormatters() {
		core[n] = true
	}
	clean := map[string]string{}
	for alias, target := range aliases {
		unit, local := SplitUnit(target)
		switch {
		case !aliasNameRe.MatchString(alias):
			return &ErrBadAliases{Msg: fmt.Sprintf("ชื่อย่อ %q ไม่ถูกต้อง", alias)}
		case unit == SiteUnit || local != alias:
			return &ErrBadAliases{Msg: fmt.Sprintf("ชื่อย่อ %q ต้องชี้ไปที่ formatter ของปลั๊กอินชื่อเดียวกัน เช่น \"<id>:%s\" (ไม่ใช่ %q)", alias, alias, target)}
		case core[alias]:
			return &ErrBadAliases{Msg: fmt.Sprintf("%q เป็น formatter ของระบบ — ใช้เป็นชื่อย่อไม่ได้", alias)}
		}
		clean[alias] = target
	}
	b, _ := json.Marshal(clean)
	return e.st.SetOptions(map[string]string{OptFormatterAliases: string(b)})
}

// Formatters lists Site Code's formatters and every loaded JS plugin's,
// with the enabled aliases and conflicts.
func (e *Engine) Formatters() ([]FormatterRow, error) {
	out := []FormatterRow{}
	opts, err := e.st.Options()
	if err != nil {
		return nil, err
	}
	aliases := AliasesFrom(opts)
	site := map[string]bool{}
	for _, f := range e.ManifestFor(opts).Formatters {
		site[f] = true
		out = append(out, FormatterRow{Name: f, Unit: SiteUnit})
	}
	core := map[string]bool{}
	for _, n := range design.CoreFormatters() {
		core[n] = true
	}
	for _, lp := range e.units().loaded() {
		names := append([]string{}, lp.prog.Manifest.Formatters...)
		sort.Strings(names)
		for _, f := range names {
			row := FormatterRow{Name: lp.unit.ID + ":" + f, Unit: lp.unit.ID}
			holder := aliases[f]
			switch {
			case core[f]:
				row.Conflict = "core"
			case holder == row.Name:
				row.Alias = f
				if site[f] {
					row.Conflict = SiteUnit // enabled, but Site Code's own formatter wins
				}
			case site[f]:
				row.Conflict = SiteUnit
			case holder != "":
				row.Conflict = holder
			}
			out = append(out, row)
		}
	}
	return out, nil
}

// AliasNames returns the enabled aliases that resolve to a loaded plugin
// formatter and are not shadowed by Site Code (designer autocomplete).
func (e *Engine) AliasNames(site []string) []string {
	shadow := map[string]bool{}
	for _, s := range site {
		shadow[s] = true
	}
	set := e.units()
	var out []string
	for alias, target := range e.formatterAliases() {
		unit, local := SplitUnit(target)
		if lp := set.get(unit); lp != nil && lp.prog.formatters[local] && !shadow[alias] {
			out = append(out, alias)
		}
	}
	sort.Strings(out)
	return out
}

// resolveFormatter finds the unit and local name of formatter name for the
// session: "<id>:<name>" of a loaded plugin, a Site Code formatter, or an
// enabled alias.
func (s *Session) resolveFormatter(name string) (string, string, bool) {
	if strings.Contains(name, ":") {
		unit, local := SplitUnit(name)
		if unit == SiteUnit {
			return "", "", false
		}
		if p := s.progOf(unit); p != nil && p.formatters[local] {
			return unit, local, true
		}
		return "", "", false
	}
	if s.sp().formatters[name] {
		return SiteUnit, name, true
	}
	if s.aliases == nil {
		s.aliases = s.e.formatterAliases()
	}
	if target, ok := s.aliases[name]; ok {
		unit, local := SplitUnit(target)
		if unit != SiteUnit {
			if p := s.progOf(unit); p != nil && p.formatters[local] {
				return unit, local, true
			}
		}
	}
	return "", "", false
}

// Format runs a formatter (50 ms per call; failure → the raw value):
// Site Code's, a plugin's "<id>:<name>", or an enabled alias.
func (s *Session) Format(name string, val any, args []any) (string, bool) {
	unit, local, ok := s.resolveFormatter(name)
	if !ok {
		return "", false
	}
	out, err := s.callIn(unit, "formatter "+local, FormatterTimeout, func(v *vm) goja.Callable {
		for _, f := range v.fmts {
			if f.name == local {
				return f.fn
			}
		}
		return nil
	}, func(v *vm) []goja.Value {
		a := []goja.Value{v.toJS(val)}
		for _, x := range args {
			a = append(a, v.toJS(x))
		}
		return a
	})
	if err != nil {
		return design.Stringify(val), true
	}
	return design.Stringify(out), true
}
