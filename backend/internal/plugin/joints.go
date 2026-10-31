package plugin

// Phase 7 joints for JS plugins (generic, used by the official plugins that
// replaced the Go built-ins): the code engine reads and writes a package's
// manifest settings (site.settings / site.saveSettings), renders route pages
// inside the site's layout (route result {page}), and lists / renders the
// designer blocks a plugin registers in code (site.block).

import (
	"encoding/json"
	"net/http"

	"github.com/anasaki/studio/backend/internal/design"
)

// PackageSettings returns the manifest settings schema of installed package
// id and its saved values (validated, defaults filled in); ok is false for
// an unknown package.
func (m *Manager) PackageSettings(id string) ([]Field, Values, bool) {
	k := m.pkgs.get(id)
	if k == nil {
		return nil, nil, false
	}
	out := Values{}
	for key, v := range k.settings {
		out[key] = v
	}
	return k.mf.Settings, out, true
}

// MergePackageSettings validates a partial update of package id's manifest
// settings (keys of the schema only; the others keep their saved values) and
// stores it. Errors: ErrUnknownPackage or a ValidationError.
func (m *Manager) MergePackageSettings(id string, patch map[string]any) (Values, error) {
	k := m.pkgs.get(id)
	if k == nil {
		return nil, ErrUnknownPackage
	}
	known := map[string]bool{}
	for _, f := range k.mf.Settings {
		known[f.Key] = true
	}
	bad := ValidationError{}
	in := map[string]any{}
	for key, v := range k.settings {
		in[key] = v
	}
	for key, v := range patch {
		if !known[key] {
			bad[key] = "not a setting of this plugin"
			continue
		}
		in[key] = v
	}
	if len(bad) > 0 {
		return nil, bad
	}
	v, err := Validate(k.mf.Settings, in)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(v)
	if _, err := m.pkgs.db.Exec(`UPDATE plugin_packages SET settings=? WHERE id=?`, b, id); err != nil {
		return nil, err
	}
	m.pkgs.invalidate()
	return v, nil
}

// RenderPage writes body inside the site's header and footer (the renderer
// attached with SetPages); false when no renderer is attached.
func (m *Manager) RenderPage(w http.ResponseWriter, status int, title, body string) bool {
	if m.host.Pages == nil {
		return false
	}
	m.host.Pages.HTML(w, status, title, body)
	return true
}

// CodeBlocks is implemented by the code runtime when JS plugins register
// designer blocks in code (site.block): the designer lists them with the
// package blocks and the renderer asks the runtime first.
type CodeBlocks interface {
	// CodeBlockInfos lists the blocks of running JS plugins (dynamic select
	// options resolved now).
	CodeBlockInfos() []BlockInfo
	// RenderCodeBlock renders a code block (no request context); ok is
	// false when no running plugin registered plugin/block.
	RenderCodeBlock(c design.BlockCall) (html, css string, ok bool)
}

// codeBlocks returns the code runtime's block joint (nil when none).
func (m *Manager) codeBlocks() CodeBlocks {
	if cb, ok := m.code.(CodeBlocks); ok && cb != nil {
		return cb
	}
	return nil
}

// mergeCodeBlocks adds the code blocks to list: a code block replaces a
// manifest block of the same plugin and type.
func mergeCodeBlocks(list []BlockInfo, code []BlockInfo) []BlockInfo {
	if len(code) == 0 {
		return list
	}
	have := map[string]bool{}
	for _, b := range code {
		have[b.Plugin+"/"+b.Type] = true
	}
	out := make([]BlockInfo, 0, len(list)+len(code))
	for _, b := range list {
		if !have[b.Plugin+"/"+b.Type] {
			out = append(out, b)
		}
	}
	return append(out, code...)
}
