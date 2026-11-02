package plugin

// Plugin capabilities (since 2.0.6): a manifest declares what its plugin lets
// people do —
//
//	"capabilities": [{"id": "view", "label": {"th": "…", "en": "…"},
//	                  "description": {"th": "…", "en": "…"}, "default": ["administrator", "editor"]}]
//
// and the core registers each as the capability "<plugin id>.<id>"
// (analytics.view, docs.edit). Roles hold them like core capabilities (the
// roles table), so the administrator ticks them per role in Users → Roles.
//
//   - Install / upgrade: a capability seen for the first time (ever, on this
//     site: the "plugin_caps_seen" option) is granted to its default roles. A capability
//     seen before is left as the administrator set it — an upgrade or a
//     reinstall never re-grants what was taken away.
//   - A capability the new version no longer declares, and every capability
//     of an uninstalled plugin, is removed from the roles (their other
//     capabilities stay). auth.RoleHas also ignores undeclared plugin
//     capabilities, so leftovers can never grant anything.
//   - The administrator always has every capability.
//
// The core never knows any plugin's capabilities: this file only reads the
// manifests. Enforcement: JS plugins (access lists "cap:<plugin>.<id>",
// ctx.user.can, site.can, post type caps), service / WASM plugins (the user
// context's caps, admin page "cap").

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
)

// OptionCapsSeen is the options key listing every plugin capability the site
// has ever registered (JSON array): defaults are granted only the first time.
// It lives with the roles (options), so a site export / import keeps both.
const OptionCapsSeen = "plugin_caps_seen"

// Manifest capability limits (the Plugin Store's validator uses the same).
const (
	maxCapabilities  = 30
	maxCapLabel      = 80
	maxCapDesc       = 300
	maxCapDefaults   = 20
	capRoleSlugShape = `^[a-z][a-z0-9_-]{1,31}$`
)

var capRoleRe = regexp.MustCompile(capRoleSlugShape)

// CapKey is the capability name of manifest capability id ("<plugin>.<id>").
func (mf *Manifest) CapKey(id string) string { return mf.ID + "." + id }

// DeclaresCap reports whether the manifest declares capability key
// ("<plugin>.<id>" with its own id).
func (mf *Manifest) DeclaresCap(key string) bool {
	for _, c := range mf.Capabilities {
		if mf.CapKey(c.ID) == key {
			return true
		}
	}
	return false
}

// CapKeys lists the manifest's capability names.
func (mf *Manifest) CapKeys() []string {
	out := make([]string, 0, len(mf.Capabilities))
	for _, c := range mf.Capabilities {
		out = append(out, mf.CapKey(c.ID))
	}
	return out
}

// validateCapabilities checks the "capabilities" section.
func (mf *Manifest) validateCapabilities() error {
	if len(mf.Capabilities) > maxCapabilities {
		return pkgErr("manifest: capabilities: at most %d entries", maxCapabilities)
	}
	seen := map[string]bool{}
	for _, c := range mf.Capabilities {
		if !auth.ValidPluginCapID(c.ID) {
			return pkgErr("manifest: capabilities: id %q must match ^[a-z][a-z0-9_]{1,30}$ (e.g. \"view\", \"edit_meta\")", c.ID)
		}
		if seen[c.ID] {
			return pkgErr("manifest: capabilities: %q is listed twice", c.ID)
		}
		seen[c.ID] = true
		if strings.TrimSpace(c.Label.TH) == "" && strings.TrimSpace(c.Label.EN) == "" {
			return pkgErr("manifest: capabilities: %s needs a label ({\"th\": \"…\", \"en\": \"…\"})", c.ID)
		}
		if utf8.RuneCountInString(c.Label.TH) > maxCapLabel || utf8.RuneCountInString(c.Label.EN) > maxCapLabel {
			return pkgErr("manifest: capabilities: %s: label longer than %d characters", c.ID, maxCapLabel)
		}
		if d := c.Description; d != nil && (utf8.RuneCountInString(d.TH) > maxCapDesc || utf8.RuneCountInString(d.EN) > maxCapDesc) {
			return pkgErr("manifest: capabilities: %s: description longer than %d characters", c.ID, maxCapDesc)
		}
		if err := uniqueList("capabilities."+c.ID+".default", c.Default, maxCapDefaults, capRoleRe.MatchString,
			"a role slug such as administrator, editor, author, contributor, subscriber or a custom role"); err != nil {
			return err
		}
	}
	// a service admin page may require one of the plugin's own capabilities
	if mf.Service != nil {
		for _, a := range mf.Service.Admin {
			if pl, _, ok := auth.SplitPluginCap(a.Cap); ok && (pl != mf.ID || !mf.DeclaresCap(a.Cap)) {
				return pkgErr("manifest: service: admin page %q: cap %q must be one of the plugin's own capabilities (\"%s.<id>\" declared in \"capabilities\")", a.Path, a.Cap, mf.ID)
			}
		}
	}
	return nil
}

// CapabilityView is a declared capability of an installed package (API).
type CapabilityView struct {
	Key         string   `json:"key"` // "<plugin>.<id>"
	ID          string   `json:"id"`
	Label       I18n     `json:"label"`
	Description *I18n    `json:"description,omitempty"`
	Default     []string `json:"default"`
}

func (mf *Manifest) capabilityViews() []CapabilityView {
	out := []CapabilityView{}
	for _, c := range mf.Capabilities {
		out = append(out, CapabilityView{Key: mf.CapKey(c.ID), ID: c.ID, Label: c.Label, Description: c.Description,
			Default: nonNil(c.Default)})
	}
	return out
}

// ---------------------------------------------------------------- catalog

// capCatalog lists the capabilities of every installed package (the roles
// editor's plugin sections; auth.SetPluginCapSource). Cached per reload of
// the package list.
func (p *packages) capCatalog() []auth.CapInfo {
	list := p.all()
	p.mu.Lock()
	if p.capsAt.Equal(p.loadedAt) && p.capsOK {
		out := p.caps
		p.mu.Unlock()
		return out
	}
	at := p.loadedAt
	p.mu.Unlock()
	var out []auth.CapInfo
	for _, k := range list {
		for _, c := range k.mf.Capabilities {
			info := auth.CapInfo{Key: k.mf.CapKey(c.ID), Group: "plugins", Plugin: k.mf.ID, PluginName: k.mf.Name,
				Label: map[string]string{"th": c.Label.TH, "en": c.Label.EN}, Default: nonNil(c.Default)}
			if k.mf.IconPath() == "" {
				info.PluginIcon = k.mf.Icon
			}
			if c.Description != nil {
				info.Description = map[string]string{"th": c.Description.TH, "en": c.Description.EN}
			}
			out = append(out, info)
		}
	}
	p.mu.Lock()
	if p.loadedAt.Equal(at) {
		p.caps, p.capsAt, p.capsOK = out, at, true
	}
	p.mu.Unlock()
	return out
}

// ---------------------------------------------------------------- roles

// editRoles applies fn to every role's capability set (administrator
// excluded: it always has everything) and stores the roles fn changed, then
// refreshes the capability cache.
func (m *Manager) editRoles(fn func(slug string, caps map[string]bool) bool) error {
	st := m.host.Store
	rs, err := st.ListRoles()
	if err != nil {
		return err
	}
	changed := false
	for i := range rs {
		r := &rs[i]
		if r.Slug == model.RoleAdmin {
			continue
		}
		set := map[string]bool{}
		for _, c := range r.Caps {
			set[c] = true
		}
		if !fn(r.Slug, set) {
			continue
		}
		r.Caps = make([]string, 0, len(set))
		for c, ok := range set {
			if ok {
				r.Caps = append(r.Caps, c)
			}
		}
		sort.Strings(r.Caps)
		if err := st.UpdateRole(r); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		m.reloadRoles()
	}
	return nil
}

// reloadRoles refreshes auth's role cache from the roles table.
func (m *Manager) reloadRoles() {
	rc, err := m.host.Store.RoleCaps()
	if err != nil {
		log.Printf("plugin capabilities: reload roles: %v", err)
		return
	}
	if len(rc) > 0 {
		auth.SetRoles(rc)
	}
}

// seenCaps reads the set of capabilities registered before.
func (m *Manager) seenCaps() (map[string]bool, error) {
	var raw string
	err := m.pkgs.db.QueryRow(`SELECT value FROM options WHERE name = ?`, OptionCapsSeen).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []string
	_ = json.Unmarshal([]byte(raw), &list)
	out := make(map[string]bool, len(list))
	for _, c := range list {
		out[c] = true
	}
	return out, nil
}

// markSeen records capability keys and returns those never seen before.
// Callers hold installMu (or run at startup), so the read-modify-write of
// the option does not race another install.
func (m *Manager) markSeen(keys []string) ([]string, error) {
	seen, err := m.seenCaps()
	if err != nil {
		return nil, err
	}
	var fresh []string
	for _, k := range keys {
		if !seen[k] {
			seen[k] = true
			fresh = append(fresh, k)
		}
	}
	if len(fresh) == 0 {
		return nil, nil
	}
	list := make([]string, 0, len(seen))
	for c := range seen {
		list = append(list, c)
	}
	sort.Strings(list)
	b, _ := json.Marshal(list)
	if err := m.host.Store.SetOptions(map[string]string{OptionCapsSeen: string(b)}); err != nil {
		return nil, err
	}
	return fresh, nil
}

// syncCaps applies mf's capabilities to the roles after an install or
// upgrade: new capabilities go to their default roles, capabilities of the
// plugin it no longer declares leave every role.
func (m *Manager) syncCaps(mf *Manifest) error {
	fresh, err := m.markSeen(mf.CapKeys())
	if err != nil {
		return fmt.Errorf("plugin %s: capabilities: %w", mf.ID, err)
	}
	grant := map[string][]string{} // role → caps to add
	for _, c := range mf.Capabilities {
		key := mf.CapKey(c.ID)
		for _, f := range fresh {
			if f != key {
				continue
			}
			for _, r := range c.Default {
				grant[r] = append(grant[r], key)
			}
		}
	}
	declared := map[string]bool{}
	for _, k := range mf.CapKeys() {
		declared[k] = true
	}
	return m.editRoles(func(slug string, caps map[string]bool) bool {
		changed := false
		for c := range caps {
			if p, _, ok := auth.SplitPluginCap(c); ok && p == mf.ID && !declared[c] {
				delete(caps, c)
				changed = true
			}
		}
		for _, c := range grant[slug] {
			if !caps[c] {
				caps[c] = true
				changed = true
			}
		}
		return changed
	})
}

// dropCaps removes every capability of plugin id from the roles (uninstall).
// The "seen" records stay: a reinstall does not re-grant the defaults of
// capabilities the site already had.
func (m *Manager) dropCaps(id string) error {
	return m.editRoles(func(_ string, caps map[string]bool) bool {
		changed := false
		for c := range caps {
			if p, _, ok := auth.SplitPluginCap(c); ok && p == id {
				delete(caps, c)
				changed = true
			}
		}
		return changed
	})
}

// GrantPluginDefaults gives role (a role just created again, e.g. a restored
// built-in role) the default plugin capabilities that name it.
func (m *Manager) GrantPluginDefaults(role string) error {
	var add []string
	for _, k := range m.pkgs.all() {
		for _, c := range k.mf.Capabilities {
			for _, r := range c.Default {
				if r == role {
					add = append(add, k.mf.CapKey(c.ID))
				}
			}
		}
	}
	if len(add) == 0 {
		return nil
	}
	return m.editRoles(func(slug string, caps map[string]bool) bool {
		if slug != role {
			return false
		}
		changed := false
		for _, c := range add {
			if !caps[c] {
				caps[c] = true
				changed = true
			}
		}
		return changed
	})
}

// syncAllCaps runs at startup: manifests stored by an older CMS (which did
// not know "capabilities") get them back from the package's manifest.json,
// every installed plugin's new capabilities are granted, and plugin
// capabilities nobody declares any more leave the roles.
func (m *Manager) syncAllCaps() {
	p := m.pkgs
	if p.db == nil {
		return
	}
	for _, k := range p.all() {
		if len(k.mf.Capabilities) > 0 {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(k.dir, manifestFileName))
		if err != nil {
			continue
		}
		var disk Manifest
		if json.Unmarshal(trimBOM(raw), &disk) != nil || disk.ID != k.mf.ID || len(disk.Capabilities) == 0 {
			continue
		}
		mf := *k.mf
		mf.Capabilities = disk.Capabilities
		if mf.validateCapabilities() != nil {
			continue
		}
		b, _ := json.Marshal(&mf)
		if _, err := p.db.Exec(`UPDATE plugin_packages SET manifest=? WHERE id=? AND version=?`, b, mf.ID, k.version); err != nil {
			log.Printf("plugin %s: store capabilities: %v", mf.ID, err)
		}
	}
	p.invalidate()
	installed := map[string]map[string]bool{}
	for _, k := range p.all() {
		if err := m.syncCaps(k.mf); err != nil {
			log.Printf("%v", err)
		}
		installed[k.mf.ID] = map[string]bool{}
		for _, c := range k.mf.CapKeys() {
			installed[k.mf.ID][c] = true
		}
	}
	// leftovers of plugins uninstalled while the roles could not be updated
	// (only names the core registered once)
	seen, err := m.seenCaps()
	if err != nil {
		log.Printf("plugin capabilities: %v", err)
		return
	}
	if err := m.editRoles(func(_ string, caps map[string]bool) bool {
		changed := false
		for c := range caps {
			if pl, _, ok := auth.SplitPluginCap(c); ok && !installed[pl][c] && seen[c] {
				delete(caps, c)
				changed = true
			}
		}
		return changed
	}); err != nil {
		log.Printf("plugin capabilities: %v", err)
	}
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xef && b[1] == 0xbb && b[2] == 0xbf {
		return b[3:]
	}
	return b
}
