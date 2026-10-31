package sitecode

import (
	"sort"
	"strings"
)

// Admin views over every unit (phase 4b): status of one unit, the hooks
// screen and the designer catalog of plugin bindings.

// StatusOf is GET /status?unit=: Site Code ("" / "site") or a JS plugin.
func (e *Engine) StatusOf(unit string) (*Status, error) {
	if unit == "" || unit == SiteUnit {
		return e.Status()
	}
	opts, err := e.st.Options()
	if err != nil {
		return nil, err
	}
	s := &Status{SafeMode: opts[OptSafeMode] == "1", EnvDisabled: e.cfg.Disabled, Demo: e.cfg.Demo, Unit: unit, Units: []UnitInfo{}}
	s.Registered.normalize()
	if e.data != nil {
		s.DB.Mode = e.data.Mode
	}
	s.Units = e.Units() // demo mode: Store plugins run, uploaded ones carry MsgDemoStoreOnly
	set := e.units()
	if lp := set.get(unit); lp != nil {
		s.Registered = lp.prog.Manifest
	} else if set == nil || set.refs[unit].ID == "" {
		return nil, ErrNotFound
	}
	if set != nil {
		s.ActivationError = set.errs[unit]
	}
	if x, ok := e.unitErr.Load(unit); ok {
		msg := x.(string)
		s.LastError = &msg
	}
	return s, nil
}

// UnitManifest is the registrations of a loaded JS plugin.
type UnitManifest struct {
	Unit     string
	Name     string
	Manifest Manifest
}

// PluginManifests lists the loaded JS plugins' registrations (load order).
func (e *Engine) PluginManifests() []UnitManifest {
	var out []UnitManifest
	for _, lp := range e.units().loaded() {
		out = append(out, UnitManifest{Unit: lp.unit.ID, Name: lp.unit.Name, Manifest: lp.prog.Manifest})
	}
	return out
}

// HookSub is a subscriber of a hook (GET /code/hooks).
type HookSub struct {
	Unit     string `json:"unit"`
	Priority int    `json:"priority"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// HookRow is one filter or event with its subscribers in effective order.
type HookRow struct {
	Name        string    `json:"name"`
	Kind        string    `json:"kind"` // filter | event
	DeclaredBy  string    `json:"declaredBy"`
	Subscribers []HookSub `json:"subscribers"`
}

// coreEvents are the events the CMS emits itself.
var coreEvents = map[string]bool{"post.published": true, "post.saved": true, "post.deleted": true, "user.created": true,
	"media.uploaded": true, "form.submitted": true}

// Hooks lists every filter and event subscribed or declared by the units:
// subscribers by ascending priority, then unit load order (Site Code last),
// then registration order.
func (e *Engine) Hooks() []HookRow {
	type key struct{ name, kind string }
	rows := map[key]*HookRow{}
	declared := map[key]string{}
	get := func(name, kind string) *HookRow {
		k := key{name, kind}
		r := rows[k]
		if r == nil {
			r = &HookRow{Name: name, Kind: kind, Subscribers: []HookSub{}}
			rows[k] = r
		}
		return r
	}
	set := e.units()
	var site *Program
	if opts, err := e.st.Options(); err == nil && opts[OptVersion] != "" && !e.siteOff(opts) {
		if lp := e.current(opts); lp != nil {
			site = lp.prog
		}
	}
	units := []*Program{}
	for _, lp := range set.loaded() {
		units = append(units, lp.prog)
	}
	if site != nil {
		units = append(units, site)
	}
	names := map[string]bool{}
	for _, p := range units {
		for _, f := range p.filterList {
			names[f.Name] = true
		}
		for _, ev := range p.eventList {
			r := get(ev.Name, "event")
			r.Subscribers = append(r.Subscribers, HookSub{Unit: p.unit.id(), Priority: ev.Priority, File: ev.At.File, Line: ev.At.Line})
		}
	}
	for n := range names {
		r := get(n, "filter")
		for _, h := range filterChain(set, site, n) {
			r.Subscribers = append(r.Subscribers, HookSub{Unit: h.unit, Priority: h.prio, File: h.at.File, Line: h.at.Line})
		}
	}
	if set != nil {
		for id, ref := range set.refs {
			if ref.Code.Hooks == nil {
				continue
			}
			for _, n := range ref.Code.Hooks.Filters {
				get(n, "filter")
				declared[key{n, "filter"}] = id
			}
			for _, n := range ref.Code.Hooks.Events {
				get(n, "event")
				declared[key{n, "event"}] = id
			}
		}
	}
	out := make([]HookRow, 0, len(rows))
	for k, r := range rows {
		switch {
		case declared[k] != "":
			r.DeclaredBy = declared[k]
		case (k.kind == "filter" && coreFilters[k.name]) || (k.kind == "event" && coreEvents[k.name]):
			r.DeclaredBy = "core"
		default:
			pre, _, _ := strings.Cut(k.name, ".")
			r.DeclaredBy = pre
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}
