package plugin

import (
	"encoding/json"
	"log"
	"time"
)

// Option names used by the plugin platform.
const (
	// OptionSettingsPrefix + id holds a built-in plugin's settings (JSON).
	OptionSettingsPrefix = "plugin_settings."
	// OptionDefaultsApplied is set once the "default active" plugins were
	// switched on, so a later deactivation is never overridden.
	OptionDefaultsApplied = "plugins_defaults_applied"
)

type cachedValues struct {
	v  Values
	at time.Time
}

// settingsValues returns plugin id's settings with defaults applied, cached
// for two seconds (several processes may share the database).
func (m *Manager) settingsValues(id string, schema []Field) Values {
	m.setMu.Lock()
	c, ok := m.settings[id]
	m.setMu.Unlock()
	if ok && time.Since(c.at) < 2*time.Second {
		return c.v
	}
	var raw string
	stored := map[string]any{}
	if err := m.host.Store.DB.QueryRow(`SELECT value FROM options WHERE name = ?`,
		OptionSettingsPrefix+id).Scan(&raw); err == nil {
		_ = json.Unmarshal([]byte(raw), &stored)
	}
	v := Sanitize(schema, stored)
	m.setMu.Lock()
	m.settings[id] = cachedValues{v: v, at: time.Now()}
	m.setMu.Unlock()
	return v
}

// SettingsView is the GET/PUT /api/v1/plugin-settings/{id} payload.
type SettingsView struct {
	Schema []Field `json:"schema"`
	Values Values  `json:"values"`
}

// PluginSettings returns a built-in plugin's schema and values.
func (m *Manager) PluginSettings(id string) (SettingsView, error) {
	e := m.byID[id]
	if e == nil {
		return SettingsView{}, ErrUnknown
	}
	schema := e.reg.settings
	if schema == nil {
		schema = []Field{}
	}
	v := m.settingsValues(id, e.reg.settings)
	if fix := e.reg.settingsFix; fix != nil {
		// values stored before the plugin's filter existed: complete them once
		if fixed := fix(copyValues(v)); !sameValues(fixed, v) {
			if err := m.storeSettings(id, fixed); err != nil {
				return SettingsView{}, err
			}
			v = m.settingsValues(id, e.reg.settings)
		}
	}
	return SettingsView{Schema: schema, Values: v}, nil
}

// SetPluginSettings validates and stores a built-in plugin's settings.
// Errors are ErrUnknown or a ValidationError.
func (m *Manager) SetPluginSettings(id string, in map[string]any) (SettingsView, error) {
	e := m.byID[id]
	if e == nil {
		return SettingsView{}, ErrUnknown
	}
	v, err := Validate(e.reg.settings, in)
	if err != nil {
		return SettingsView{}, err
	}
	if fix := e.reg.settingsFix; fix != nil {
		v = fix(v)
	}
	if err := m.storeSettings(id, v); err != nil {
		return SettingsView{}, err
	}
	return m.PluginSettings(id)
}

func (m *Manager) storeSettings(id string, v Values) error {
	b, _ := json.Marshal(v)
	if err := m.host.Store.SetOptions(map[string]string{OptionSettingsPrefix + id: string(b)}); err != nil {
		return err
	}
	m.setMu.Lock()
	delete(m.settings, id)
	m.setMu.Unlock()
	return nil
}

func copyValues(v Values) Values {
	out := make(Values, len(v))
	for k, x := range v {
		out[k] = x
	}
	return out
}

func sameValues(a, b Values) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// applyDefaults switches on the "default active" plugins once.
func (m *Manager) applyDefaults() {
	var done string
	err := m.host.Store.DB.QueryRow(`SELECT value FROM options WHERE name = ?`, OptionDefaultsApplied).Scan(&done)
	if err == nil && done != "" {
		return
	}
	for _, e := range m.entries {
		if !e.reg.defaultActive || m.Active(e.info.ID) {
			continue
		}
		if _, err := m.Activate(e.info.ID, 0); err != nil {
			log.Printf("plugin %s: default activation: %v", e.info.ID, err)
		}
	}
	if err := m.host.Store.SetOptions(map[string]string{OptionDefaultsApplied: "1"}); err != nil {
		log.Printf("plugins: %v", err)
	}
}
