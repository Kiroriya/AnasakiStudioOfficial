package sitecode

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// Phase 4b: code units. The engine runs Site Code (unit "site") and one
// unit per active JavaScript plugin ("runtime": "js"). Each unit has its own
// bundle, registrations, VMs, logs and circuit breakers; a plugin unit's
// names are namespaced by its id (collections "code:<id>:<name>", actions
// "/_a/<id>:<name>", tables p_<id>_*, cookies pl_<id>_*, settings
// "<id>.<key>", hooks "<id>.*") and its APIs are limited to its code scopes.

// SiteUnit is the unit id of Site Code.
const SiteUnit = "site"

// unitInfo is the identity and permissions of a code unit.
type unitInfo struct {
	ID          string
	Plugin      bool
	Name        string
	Version     string
	InstalledAt time.Time
	scopes      map[string]bool
	hosts       []string
	routes      []string // route prefixes (plugins)
	exports     map[string]bool
	requires    map[string]string
	optional    map[string]string // optionalRequires (phase 6)
	declared    map[string]bool   // code.hooks filters and events
	library     string
	n           Naming            // the unit's tables (plugins)
	aliases     map[string]string // code.eventAliases (phase 7)
}

// siteUnitInfo is Site Code without an engine (unit tests).
var siteUnitInfo = &unitInfo{ID: SiteUnit}

func (u *unitInfo) plugin() bool { return u != nil && u.Plugin }

func (u *unitInfo) id() string {
	if u == nil {
		return SiteUnit
	}
	return u.ID
}

// can reports a code scope (Site Code has every scope).
func (u *unitInfo) can(scope string) bool { return !u.plugin() || u.scopes[scope] }

// qualify namespaces a registration name: "<id>:<name>" for a plugin.
func (u *unitInfo) qualify(name string) string {
	if !u.plugin() {
		return name
	}
	return u.ID + ":" + name
}

// src prefixes a log source with the plugin id ("shop: route GET /shop").
func (u *unitInfo) src(s string) string {
	if !u.plugin() {
		return s
	}
	return u.ID + ": " + s
}

// cookiePrefix is the unit's cookie namespace.
func (u *unitInfo) cookiePrefix() string {
	if !u.plugin() {
		return CookiePrefix
	}
	return plugin.CookiePrefix(u.ID)
}

// hookPrefix is what the unit's own hook names start with.
func (u *unitInfo) hookPrefix() string {
	if !u.plugin() {
		return "site."
	}
	return u.ID + "."
}

// settingKey is the script_settings key of the unit's key.
func (u *unitInfo) settingKey(k string) string {
	if !u.plugin() {
		return k
	}
	return u.ID + "." + k
}

// routeAllowed: a plugin route must be under a declared prefix.
func (u *unitInfo) routeAllowed(p string) bool {
	if !u.plugin() {
		return true
	}
	for _, pre := range u.routes {
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}

var unitIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,39}$`)

// SplitUnit splits "<id>:<name>" into the unit and the local name ("site"
// for an unqualified name).
func SplitUnit(name string) (string, string) {
	if i := strings.Index(name, ":"); i > 0 && unitIDRe.MatchString(name[:i]) {
		return name[:i], name[i+1:]
	}
	return SiteUnit, name
}

func (e *Engine) newUnitInfo(ref plugin.CodeUnitRef) *unitInfo {
	set := func(l []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range l {
			m[x] = true
		}
		return m
	}
	u := &unitInfo{ID: ref.ID, Plugin: true, Name: ref.Name, Version: ref.Version, InstalledAt: ref.InstalledAt,
		scopes: set(ref.Code.Scopes), hosts: ref.Code.Hosts, routes: ref.Code.Routes, exports: set(ref.Code.Exports),
		requires: ref.Requires, optional: ref.OptionalRequires, library: ref.Code.Library, declared: map[string]bool{},
		aliases: ref.Code.EventAliases}
	if h := ref.Code.Hooks; h != nil {
		for _, n := range append(append([]string{}, h.Filters...), h.Events...) {
			u.declared[n] = true
		}
	}
	if e.data != nil {
		u.n = e.data.UnitNaming(ref.ID, !u.scopes["content"])
	} else {
		u.n = Naming{Prefix: UnitTablePrefix(ref.ID), Unit: ref.ID, NoCore: !u.scopes["content"]}
	}
	return u
}

// ---------------------------------------------------------------- the set of plugin units

// unitSet is the loaded plugin units (immutable once built).
type unitSet struct {
	at   atomic.Int64 // unix nanoseconds of the last freshness check
	fp   string
	list []*liveProg // load order
	byID map[string]*liveProg
	errs map[string]string             // active plugins that did not load
	refs map[string]plugin.CodeUnitRef // every installed JS plugin
}

func (s *unitSet) loaded() []*liveProg {
	if s == nil {
		return nil
	}
	return s.list
}

func (s *unitSet) get(id string) *liveProg {
	if s == nil {
		return nil
	}
	return s.byID[id]
}

func (s *unitSet) empty() bool { return s == nil || len(s.list) == 0 }

// rank is a unit's place in the load order (Site Code last).
func (s *unitSet) rank(id string) int {
	if s != nil {
		for i, lp := range s.list {
			if lp.unit.ID == id {
				return i
			}
		}
		return len(s.list)
	}
	return 0
}

const unitTTL = 2 * time.Second

// units returns the loaded plugin units, rebuilt when the installed JS
// plugins changed (checked at most every two seconds, or right away after
// UnitsChanged). nil without a plugin manager.
func (e *Engine) units() *unitSet {
	if e.cfg.Plugins == nil || e.cfg.Demo {
		return nil
	}
	fresh := func(s *unitSet) bool {
		return s != nil && !e.unitsDirty.Load() && time.Now().UnixNano()-s.at.Load() < int64(unitTTL)
	}
	if s := e.unitSetP.Load(); fresh(s) {
		return s
	}
	e.unitsMu.Lock()
	defer e.unitsMu.Unlock()
	s := e.unitSetP.Load()
	if fresh(s) {
		return s
	}
	dirty := e.unitsDirty.Swap(false)
	refs := e.cfg.Plugins.CodeUnits()
	fp := unitFingerprint(refs)
	if s != nil && s.fp == fp && !dirty {
		s.at.Store(time.Now().UnixNano())
		return s
	}
	ns := e.loadUnits(refs, fp)
	e.unitSetP.Store(ns)
	return ns
}

// UnitsChanged makes the next request reload the plugin units (plugin
// manager: install, upgrade, (de)activation, uninstall).
func (e *Engine) UnitsChanged() { e.unitsDirty.Store(true) }

func unitFingerprint(refs []plugin.CodeUnitRef) string {
	var parts []string
	for _, r := range refs {
		if r.Active {
			parts = append(parts, r.ID+"@"+r.Version+"#"+r.InstalledAt.UTC().Format(time.RFC3339Nano))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// topoOrder sorts active plugins so that every plugin comes after what it
// requires (ties: install time, then id). Plugins with missing or
// incompatible dependencies, in a cycle, or depending on such a plugin are
// returned in errs instead.
func topoOrder(refs []plugin.CodeUnitRef) ([]plugin.CodeUnitRef, map[string]string) {
	errs := map[string]string{}
	sorted := append([]plugin.CodeUnitRef{}, refs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].InstalledAt.Equal(sorted[j].InstalledAt) {
			return sorted[i].InstalledAt.Before(sorted[j].InstalledAt)
		}
		return sorted[i].ID < sorted[j].ID
	})
	byID := map[string]plugin.CodeUnitRef{}
	pos := map[string]int{}
	for i, r := range sorted {
		byID[r.ID], pos[r.ID] = r, i
	}
	indeg := map[string]int{}
	users := map[string][]string{}
	for _, r := range sorted {
		deps := make([]string, 0, len(r.Requires))
		for d := range r.Requires {
			deps = append(deps, d)
		}
		sort.Strings(deps)
		for _, d := range deps {
			dr, ok := byID[d]
			switch {
			case !ok:
				if errs[r.ID] == "" {
					errs[r.ID] = fmt.Sprintf("ต้องใช้ปลั๊กอิน %s (%s) ซึ่งยังไม่ได้ติดตั้งหรือเปิดใช้งาน", d, r.Requires[d])
				}
			case !plugin.SatisfiesRange(dr.Version, r.Requires[d]):
				if errs[r.ID] == "" {
					errs[r.ID] = fmt.Sprintf("ต้องใช้ปลั๊กอิน %s เวอร์ชัน %s แต่ติดตั้งเวอร์ชัน %s", d, r.Requires[d], dr.Version)
				}
			default:
				indeg[r.ID]++
				users[d] = append(users[d], r.ID)
			}
		}
	}
	var ready []string
	for _, r := range sorted {
		if indeg[r.ID] == 0 {
			ready = append(ready, r.ID)
		}
	}
	var order []string
	done := map[string]bool{}
	for len(ready) > 0 {
		sort.SliceStable(ready, func(i, j int) bool { return pos[ready[i]] < pos[ready[j]] })
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		done[id] = true
		for _, u := range users[id] {
			indeg[u]--
			if indeg[u] == 0 {
				ready = append(ready, u)
			}
		}
	}
	var cyc []string
	for _, r := range sorted {
		if !done[r.ID] {
			cyc = append(cyc, r.ID)
		}
	}
	for _, id := range cyc {
		if errs[id] == "" {
			errs[id] = "ปลั๊กอินพึ่งพากันเป็นวง (requires วนกลับมา): " + strings.Join(cyc, ", ") + " — ไม่ได้เปิดใช้งาน"
		}
	}
	var out []plugin.CodeUnitRef
	for _, id := range order {
		r := byID[id]
		if errs[id] == "" {
			for d := range r.Requires {
				if errs[d] != "" {
					errs[id] = fmt.Sprintf("ต้องใช้ปลั๊กอิน %s ซึ่งเปิดใช้งานไม่สำเร็จ", d)
					break
				}
			}
		}
		if errs[id] == "" {
			out = append(out, r)
		}
	}
	return out, errs
}

// jsOnly keeps the requires of r that name JS plugins: dependencies on
// service / WASM plugins are checked by the plugin manager, not ordered here.
func jsOnly(r plugin.CodeUnitRef, js map[string]plugin.CodeUnitRef) plugin.CodeUnitRef {
	req := map[string]string{}
	for id, rng := range r.Requires {
		if _, ok := js[id]; ok {
			req[id] = rng
		}
	}
	r.Requires = req
	return r
}

// loadUnits builds the active plugins in load order.
func (e *Engine) loadUnits(refs []plugin.CodeUnitRef, fp string) *unitSet {
	set := &unitSet{fp: fp, byID: map[string]*liveProg{}, errs: map[string]string{}, refs: map[string]plugin.CodeUnitRef{}}
	set.at.Store(time.Now().UnixNano())
	var active []plugin.CodeUnitRef
	for _, r := range refs {
		set.refs[r.ID] = r
	}
	for _, r := range refs {
		if r.Active {
			active = append(active, jsOnly(r, set.refs))
		}
	}
	order, errs := topoOrder(active)
	for id, msg := range errs {
		set.errs[id] = msg
	}
	opts, _ := e.st.Options()
	siteVisitor := e.ManifestFor(opts).Visitor
	visitorBy := ""
	for _, ref := range order {
		bad := ""
		for d := range ref.Requires {
			if set.byID[d] == nil {
				bad = fmt.Sprintf("ต้องใช้ปลั๊กอิน %s ซึ่งโหลดไม่สำเร็จ", d)
			}
		}
		if bad != "" {
			set.errs[ref.ID] = bad
			continue
		}
		files, err := e.cfg.Plugins.CodeFiles(ref.ID)
		if err != nil {
			set.errs[ref.ID] = "อ่านไฟล์โค้ดของปลั๊กอินไม่ได้: " + err.Error()
			continue
		}
		lp, probs := e.buildUnit(set.refs[ref.ID], files, set, opts) // full requires (plugins.call)
		if len(probs) > 0 {
			set.errs[ref.ID] = problemsText(probs)
			continue
		}
		if lp.prog.hasVisitor {
			switch {
			case siteVisitor:
				set.errs[ref.ID] = "site.visitor มีได้ที่เดียว: Site Code ลงทะเบียน site.visitor ไว้แล้ว"
				continue
			case visitorBy != "":
				set.errs[ref.ID] = "site.visitor มีได้ที่เดียว: ปลั๊กอิน " + visitorBy + " ลงทะเบียนไว้แล้ว"
				continue
			}
			visitorBy = ref.ID
		}
		set.list = append(set.list, lp)
		set.byID[ref.ID] = lp
		// optionalRequires: an installed but incompatible version is only a warning
		for dep, rng := range set.refs[ref.ID].OptionalRequires {
			if r, ok := set.refs[dep]; ok && !plugin.SatisfiesRange(r.Version, rng) {
				e.logUnit(ref.ID, "warn", "load", fmt.Sprintf("ใช้ร่วมกับปลั๊กอิน %s %s ได้ แต่ติดตั้งเวอร์ชัน %s — บางส่วนอาจทำงานไม่ถูกต้อง", dep, rng, r.Version))
			}
		}
	}
	for id, msg := range set.errs {
		e.logUnit(id, "error", "load", "ปลั๊กอินไม่ทำงาน: "+msg)
	}
	return set
}

func problemsText(probs []Problem) string {
	p := probs[0]
	if p.File != "" {
		return fmt.Sprintf("%s (%s:%d:%d)", p.Message, p.File, p.Line, p.Column)
	}
	return p.Message
}

// buildUnit bundles and registers a plugin unit against the loaded units
// set (its dependencies).
func (e *Engine) buildUnit(ref plugin.CodeUnitRef, files map[string]string, set *unitSet, opts map[string]string) (*liveProg, []Problem) {
	u := e.newUnitInfo(ref)
	if strings.TrimSpace(ref.Code.Entry) == "" {
		return nil, []Problem{{Message: "manifest ไม่มี code.entry"}}
	}
	res := buildEntryWith(files, ref.Code.Entry, "", e.importerFor(u, set))
	if !res.OK() {
		return nil, res.Errors
	}
	prog, probs := RegisterUnit(res.Bundle, e.reserved(opts), e.h, e.RouteTimeout, u)
	if len(probs) > 0 {
		return nil, probs
	}
	lp := &liveProg{unit: u, prog: prog, ready: make(chan *vm, 2), files: files}
	var err error
	lp.mux, err = newMux(prog.specs, func(i int) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { e.serveRoute(lp, i, w, r) })
	})
	if err != nil {
		return nil, []Problem{{Message: err.Error()}}
	}
	return lp, nil
}

// ---------------------------------------------------------------- activation (plugin manager)

func toCodeProblems(probs []Problem) []plugin.CodeProblem {
	out := make([]plugin.CodeProblem, 0, len(probs))
	for _, p := range probs {
		out = append(out, plugin.CodeProblem{File: p.File, Line: p.Line, Column: p.Column, Message: p.Message})
	}
	return out
}

// prepareUnit builds ref against the other active units and dry-runs its
// migrations.
func (e *Engine) prepareUnit(ref plugin.CodeUnitRef, files map[string]string) (*liveProg, *plugin.CodePlan) {
	plan := &plugin.CodePlan{Errors: []plugin.CodeProblem{}, Destructive: []plugin.CodeDestructive{}, Pending: []string{}}
	set := e.units()
	fail := func(msg string) (*liveProg, *plugin.CodePlan) {
		plan.Errors = append(plan.Errors, plugin.CodeProblem{Message: msg})
		return nil, plan
	}
	// load order with this plugin active: cycles and missing dependencies
	var refs []plugin.CodeUnitRef
	if set != nil {
		for _, r := range set.refs {
			if r.ID != ref.ID && set.byID[r.ID] != nil {
				refs = append(refs, r)
			}
		}
	}
	ref.Active = true
	all := map[string]plugin.CodeUnitRef{}
	for _, r := range e.cfg.Plugins.CodeUnits() {
		all[r.ID] = r
	}
	all[ref.ID] = ref
	for i := range refs {
		refs[i] = jsOnly(refs[i], all)
	}
	refs = append(refs, jsOnly(ref, all))
	if _, errs := topoOrder(refs); errs[ref.ID] != "" {
		return fail(errs[ref.ID])
	}
	others := &unitSet{byID: map[string]*liveProg{}}
	for _, lp := range set.loaded() {
		if lp.unit.ID != ref.ID {
			others.list = append(others.list, lp)
			others.byID[lp.unit.ID] = lp
		}
	}
	opts, err := e.st.Options()
	if err != nil {
		return fail(err.Error())
	}
	lp, probs := e.buildUnit(ref, files, others, opts)
	if len(probs) > 0 {
		plan.Errors = append(plan.Errors, toCodeProblems(probs)...)
		return nil, plan
	}
	if lp.prog.hasVisitor {
		if e.ManifestFor(opts).Visitor {
			return fail("site.visitor มีได้ที่เดียว: Site Code ลงทะเบียน site.visitor ไว้แล้ว")
		}
		for _, o := range others.list {
			if o.prog.hasVisitor {
				return fail("site.visitor มีได้ที่เดียว: ปลั๊กอิน " + o.unit.ID + " ลงทะเบียนไว้แล้ว")
			}
		}
	}
	if len(MigrationFilesOnly(files)) > 0 && !lp.unit.can("db") {
		return fail("ปลั๊กอินมี migrations/ แต่ไม่ได้ขอ scope \"db\"")
	}
	migs, mprobs := e.planMigrationsFor(files, lp.unit.n)
	if len(mprobs) > 0 {
		plan.Errors = append(plan.Errors, toCodeProblems(mprobs)...)
		return nil, plan
	}
	for _, p := range migs.Pending {
		plan.Pending = append(plan.Pending, p.Name)
		for _, d := range p.Destructive {
			plan.Destructive = append(plan.Destructive, plugin.CodeDestructive{Migration: d.Migration, Op: d.Op, Table: d.Table, Column: d.Column})
		}
	}
	return lp, plan
}

// MigrationFilesOnly lists the migration files of a project.
func MigrationFilesOnly(files map[string]string) []string {
	list, _ := MigrationFiles(files)
	out := make([]string, 0, len(list))
	for _, m := range list {
		out = append(out, m.Path)
	}
	return out
}

// PlanUnit dry-runs the activation of a plugin from files (upgrade checks).
func (e *Engine) PlanUnit(ref plugin.CodeUnitRef, files map[string]string) (*plugin.CodePlan, error) {
	_, plan := e.prepareUnit(ref, files)
	return plan, nil
}

// ActivateUnit builds an installed JS plugin, runs its pending migrations
// (destructive ones need confirm) and makes sure it registers cleanly.
func (e *Engine) ActivateUnit(id string, confirm bool) error {
	if e.cfg.Plugins == nil {
		return &plugin.ActivationError{Msg: "ระบบ Site Code ไม่ได้เชื่อมกับปลั๊กอิน"}
	}
	if e.cfg.Demo {
		return &plugin.ActivationError{Msg: "ปิดในเว็บทดลอง"}
	}
	e.pubMu.Lock()
	defer e.pubMu.Unlock()
	var ref *plugin.CodeUnitRef
	for _, r := range e.cfg.Plugins.CodeUnits() {
		if r.ID == id {
			rr := r
			ref = &rr
		}
	}
	if ref == nil {
		return &plugin.ActivationError{Msg: "ไม่พบปลั๊กอิน JavaScript " + id}
	}
	files, err := e.cfg.Plugins.CodeFiles(id)
	if err != nil {
		return err
	}
	lp, plan := e.prepareUnit(*ref, files)
	if len(plan.Errors) > 0 {
		return &plugin.ActivationError{Msg: "เปิดใช้งานปลั๊กอิน " + id + " ไม่สำเร็จ: " + plan.Errors[0].Message, Problems: plan.Errors}
	}
	if len(plan.Destructive) > 0 && !confirm {
		return &plugin.DestructiveError{List: plan.Destructive}
	}
	if lp.unit.can("db") && e.data != nil {
		if err := e.data.ensureDocs(lp.unit.n); err != nil {
			return &plugin.ActivationError{Msg: "สร้างตาราง kv_docs ของปลั๊กอินไม่สำเร็จ: " + err.Error()}
		}
	}
	if _, err := e.runMigrationsFor(files, "plugin "+id, lp.unit.n); err != nil {
		var mf *MigrationFailed
		if errors.As(err, &mf) {
			return &plugin.ActivationError{Msg: mf.Error(), Problems: toCodeProblems(mf.Errors)}
		}
		return &plugin.ActivationError{Msg: "migration ของปลั๊กอินล้มเหลว: " + err.Error()}
	}
	e.logUnit(id, "info", "activate", fmt.Sprintf("เปิดใช้งานปลั๊กอิน %s %s", id, ref.Version))
	e.UnitsChanged()
	return nil
}

// UnitError is the load error of an active JS plugin ("" when it runs).
func (e *Engine) UnitError(id string) string {
	set := e.units()
	if set == nil {
		return ""
	}
	return set.errs[id]
}

// DropUnitData removes a JS plugin's data (uninstall with "delete data"):
// each p_<id>_* table is backed up (bak_…, 7 days) and dropped; its
// migration records, relations, settings, access overrides, jobs and cron
// rows are deleted.
func (e *Engine) DropUnitData(id string) error {
	if !unitIDRe.MatchString(id) {
		return fmt.Errorf("bad plugin id")
	}
	ctx, cancel := context.WithTimeout(context.Background(), ddlTimeout)
	defer cancel()
	if d := e.data; d != nil {
		n := d.UnitNaming(id, false)
		rows, err := d.admin.QueryContext(ctx, `SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE' AND TABLE_NAME LIKE ?`,
			d.Database, likeEscape(n.Prefix)+"%")
		if err != nil {
			return err
		}
		var tables []string
		for rows.Next() {
			var t string
			if rows.Scan(&t) == nil {
				tables = append(tables, t)
			}
		}
		rows.Close()
		conn, err := d.admin.Conn(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		_, _ = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0")
		defer func() { _, _ = conn.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS = 1") }()
		for _, t := range tables {
			logical := strings.TrimPrefix(t, n.Prefix)
			if _, err := d.backupWith(ctx, n, logical, id+"/uninstall"); err != nil {
				return fmt.Errorf("สำรองตาราง %s ไม่สำเร็จ: %w", t, err)
			}
			if _, err := conn.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoteIdent(t)); err != nil {
				return err
			}
		}
		_, _ = d.admin.ExecContext(ctx, `DELETE FROM script_migrations WHERE name LIKE ?`, likeEscape(id+"/")+"%")
		_, _ = d.admin.ExecContext(ctx, `DELETE FROM script_relations WHERE table_name LIKE ?`, likeEscape(UnitTablePrefix(id))+"%")
	}
	db := e.st.DB
	for _, q := range []struct{ sql, arg string }{
		{`DELETE FROM script_settings WHERE k LIKE ?`, likeEscape(id+".") + "%"},
		{`DELETE FROM script_access WHERE name LIKE ?`, likeEscape(id+":") + "%"},
		{`DELETE FROM script_jobs WHERE name LIKE ?`, likeEscape(id+":") + "%"},
		{`DELETE FROM script_cron WHERE name LIKE ?`, likeEscape(id+":") + "%"},
	} {
		if _, err := db.ExecContext(ctx, q.sql, q.arg); err != nil {
			return err
		}
	}
	e.settings.Store(nil)
	e.access.Store(nil)
	e.logUnit(id, "warn", "uninstall", "ลบข้อมูลของปลั๊กอินแล้ว (ตารางสำรองไว้ 7 วัน)")
	return nil
}

// ensureDocs creates the kv_docs table of the unit named by n.
func (d *Data) ensureDocs(n Naming) error {
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	_, err := d.admin.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+quoteIdent(n.Prefix+"kv_docs")+docsDDL)
	return err
}

// logUnit writes one log row of a unit.
func (e *Engine) logUnit(unit, level, source, msg string) {
	if unit == "" {
		unit = SiteUnit
	}
	if unit != SiteUnit {
		source = unit + ": " + source
	}
	e.persist([]LogEntry{{Level: level, Source: source, Message: msg, Unit: unit}}, 0)
}

// ---------------------------------------------------------------- public routes of every unit

// routeTable is the public route matcher of all units: plugins in load
// order, then Site Code; a route that conflicts with an earlier unit's is
// refused (logged).
type routeTable struct {
	site *liveProg
	set  *unitSet
	mux  *http.ServeMux
	n    int
}

func (e *Engine) routes(site *liveProg, set *unitSet) *routeTable {
	if set.empty() {
		if site == nil || site.mux == nil {
			return nil
		}
		return &routeTable{site: site, mux: site.mux, n: len(site.prog.specs)}
	}
	if rt := e.routeTab.Load(); rt != nil && rt.site == site && rt.set == set {
		return rt
	}
	rt := &routeTable{site: site, set: set, mux: http.NewServeMux()}
	units := append([]*liveProg{}, set.list...)
	if site != nil && site.prog != nil {
		units = append(units, site)
	}
	for _, lp := range units {
		for i, rs := range lp.prog.specs {
			lp, i, rs := lp, i, rs
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						e.logUnit(lp.unit.id(), "warn", "route "+rs.Method+" "+rs.Path,
							fmt.Sprintf("route นี้ถูกปฏิเสธ: ชนกับ route ของหน่วยโค้ดที่โหลดก่อน (%v)", rec))
					}
				}()
				rt.mux.Handle(rs.Pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { e.serveRoute(lp, i, w, r) }))
				rt.n++
			}()
		}
	}
	e.routeTab.Store(rt)
	return rt
}
