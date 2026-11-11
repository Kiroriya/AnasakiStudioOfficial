package sitecode

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/go-sql-driver/mysql"
)

// Migrations: files migrations/NNN_name.ts|js with
// `export default migration({ up(schema) {...}, down(schema) {...} })`.
// Pending ones (not "ran" in script_migrations) run in file-name order on
// publish, before the new code goes live.

// migFile is a migration file of the project.
type migFile struct {
	Name string // file name without extension, e.g. 001_create_orders
	Path string
}

var migNameRe = regexp.MustCompile(`^migrations/([A-Za-z0-9][A-Za-z0-9._-]*)\.(ts|js)$`)

// MigrationFiles lists the migration files sorted by name.
func MigrationFiles(files map[string]string) ([]migFile, []Problem) {
	var out []migFile
	var probs []Problem
	seen := map[string]string{}
	for p := range files {
		m := migNameRe.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		if prev, dup := seen[m[1]]; dup {
			probs = append(probs, Problem{File: p, Line: 1, Column: 1, Message: fmt.Sprintf("migration ชื่อซ้ำกับ %s", prev)})
			continue
		}
		seen[m[1]] = p
		out = append(out, migFile{Name: m[1], Path: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	sort.Slice(probs, func(i, j int) bool { return probs[i].File < probs[j].File })
	return out, probs
}

// PendingMigration is a migration a publish would run (POST /build).
type PendingMigration struct {
	Name        string        `json:"name"`
	SQL         []string      `json:"sql"`
	Destructive []Destructive `json:"destructive"`
}

// MigrationsDry is the "migrations" part of POST /build.
type MigrationsDry struct {
	Pending []PendingMigration `json:"pending"`
}

// HasDestructive reports whether any pending migration is destructive.
func (m *MigrationsDry) HasDestructive() []Destructive {
	out := []Destructive{}
	if m == nil {
		return out
	}
	for _, p := range m.Pending {
		out = append(out, p.Destructive...)
	}
	return out
}

// ---------------------------------------------------------------- loading

// loadMigration bundles and evaluates a migration file; it returns the VM
// and the object passed to migration().
func (e *Engine) loadMigration(files map[string]string, mf migFile) (*vm, *goja.Object, []Problem) {
	res := buildEntry(files, mf.Path, "__migration")
	if !res.OK() {
		return nil, nil, res.Errors
	}
	prog, err := goja.Compile("bundle.js", res.Bundle, false)
	if err != nil {
		return nil, nil, []Problem{{File: mf.Path, Line: 1, Column: 1, Message: "คอมไพล์ไม่สำเร็จ: " + err.Error()}}
	}
	v := newVM(e.h)
	if err := v.load(prog, modeRegister, e.RouteTimeout*5); err != nil {
		re := v.runErr(err)
		return nil, nil, []Problem{problemOf(re, mf.Path)}
	}
	mod := v.rt.Get("__migration")
	var def goja.Value
	if mod != nil && !goja.IsUndefined(mod) && !goja.IsNull(mod) {
		def = mod.ToObject(v.rt).Get("default")
	}
	o, ok := def.(*goja.Object)
	if !ok || def == nil {
		return nil, nil, []Problem{{File: mf.Path, Line: 1, Column: 1, Message: "migration ต้อง export default migration({ up(schema) { ... }, down(schema) { ... } })"}}
	}
	if _, ok := goja.AssertFunction(o.Get("up")); !ok {
		return nil, nil, []Problem{{File: mf.Path, Line: 1, Column: 1, Message: "migration ต้องมีฟังก์ชัน up(schema)"}}
	}
	return v, o, nil
}

func problemOf(re *RunError, file string) Problem {
	if re.At.File != "" {
		return Problem{File: re.At.File, Line: re.At.Line, Column: re.At.Column, Message: re.Message}
	}
	return Problem{File: file, Line: 1, Column: 1, Message: re.Message}
}

// invoke calls a migration function without switching to handler mode (db,
// cache and crypto stay unavailable; only schema and log work).
func (v *vm) invoke(fn goja.Callable, timeout time.Duration, args ...goja.Value) error {
	_, err := v.guarded(timeout, func() (goja.Value, error) {
		r, err := fn(goja.Undefined(), args...)
		if err != nil {
			return nil, err
		}
		return v.settle(r)
	})
	if err != nil {
		return v.runErr(err)
	}
	return nil
}

// migrationGlobal is the migration() helper.
func (v *vm) migrationGlobal(c goja.FunctionCall) goja.Value {
	o, ok := c.Argument(0).(*goja.Object)
	if !ok {
		v.throw("migration({ up(schema) { ... }, down(schema) { ... } }) ต้องรับ object")
	}
	if _, ok := goja.AssertFunction(o.Get("up")); !ok {
		v.throw("migration: ต้องมีฟังก์ชัน up(schema)")
	}
	if d := o.Get("down"); d != nil && !goja.IsUndefined(d) {
		if _, ok := goja.AssertFunction(d); !ok {
			v.throw("migration: down ต้องเป็นฟังก์ชัน")
		}
	}
	return o
}

// ---------------------------------------------------------------- schema object

func (v *vm) str0(c goja.FunctionCall, i int, what string) string {
	s, ok := c.Argument(i).Export().(string)
	if !ok {
		v.throw("%s: ต้องเป็นข้อความ", what)
	}
	return s
}

func (v *vm) strList(x goja.Value, what string) []string {
	if s, ok := x.Export().(string); ok {
		return []string{s}
	}
	items, ok := v.array(x)
	if !ok {
		v.throw("%s: ต้องเป็นชื่อคอลัมน์หรือ array ของชื่อ", what)
	}
	out := make([]string, len(items))
	for i, it := range items {
		s, ok := it.Export().(string)
		if !ok {
			v.throw("%s: ชื่อคอลัมน์ต้องเป็นข้อความ", what)
		}
		out[i] = s
	}
	return out
}

// schemaObject is the `schema` argument of up/down.
func (v *vm) schemaObject(p *Planner) *goja.Object {
	o := v.rt.NewObject()
	check := func(err error) {
		if err != nil {
			v.throw("%s", err.Error())
		}
	}
	blueprint := func(c goja.FunctionCall, create bool, what string) *Blueprint {
		name := v.str0(c, 0, what)
		fn, ok := goja.AssertFunction(c.Argument(1))
		if !ok {
			v.throw("%s(%q, (t) => { ... }): ต้องมีฟังก์ชัน", what, name)
		}
		bp := &Blueprint{Table: name, Create: create}
		if _, err := fn(goja.Undefined(), v.tableBuilder(bp)); err != nil {
			panic(err)
		}
		return bp
	}
	_ = o.Set("create", func(c goja.FunctionCall) goja.Value {
		at := v.caller()
		check(p.Create(blueprint(c, true, "schema.create"), at))
		return goja.Undefined()
	})
	_ = o.Set("table", func(c goja.FunctionCall) goja.Value {
		at := v.caller()
		check(p.Alter(blueprint(c, false, "schema.table"), at))
		return goja.Undefined()
	})
	_ = o.Set("drop", func(c goja.FunctionCall) goja.Value {
		check(p.Drop(v.str0(c, 0, "schema.drop"), false, v.caller()))
		return goja.Undefined()
	})
	_ = o.Set("dropIfExists", func(c goja.FunctionCall) goja.Value {
		check(p.Drop(v.str0(c, 0, "schema.dropIfExists"), true, v.caller()))
		return goja.Undefined()
	})
	_ = o.Set("rename", func(c goja.FunctionCall) goja.Value {
		check(p.Rename(v.str0(c, 0, "schema.rename"), v.str0(c, 1, "schema.rename"), v.caller()))
		return goja.Undefined()
	})
	_ = o.Set("hasTable", func(c goja.FunctionCall) goja.Value {
		return v.rt.ToValue(p.HasTable(v.str0(c, 0, "schema.hasTable")))
	})
	_ = o.Set("hasColumn", func(c goja.FunctionCall) goja.Value {
		return v.rt.ToValue(p.HasColumn(v.str0(c, 0, "schema.hasColumn"), v.str0(c, 1, "schema.hasColumn")))
	})
	_ = o.Set("raw", func(c goja.FunctionCall) goja.Value {
		check(p.Raw(v.str0(c, 0, "schema.raw"), v.caller()))
		return goja.Undefined()
	})
	return o
}

// tableBuilder is the `t` of schema.create / schema.table.
func (v *vm) tableBuilder(bp *Blueprint) *goja.Object {
	t := v.rt.NewObject()
	cols := map[*goja.Object]*ColumnDef{}
	intArg := func(c goja.FunctionCall, i, def int) int {
		a := c.Argument(i)
		if goja.IsUndefined(a) || goja.IsNull(a) {
			return def
		}
		return int(a.ToInteger())
	}
	column := func(cd *ColumnDef) *goja.Object {
		cd.At = v.caller()
		bp.Columns = append(bp.Columns, cd)
		o := v.columnBuilder(cd)
		cols[o] = cd
		return o
	}
	simple := func(kind string) func(goja.FunctionCall) goja.Value {
		return func(c goja.FunctionCall) goja.Value {
			return column(&ColumnDef{Name: v.str0(c, 0, "t."+kind), Kind: kind})
		}
	}
	_ = t.Set("id", func(c goja.FunctionCall) goja.Value {
		name := "id"
		if s, ok := c.Argument(0).Export().(string); ok {
			name = s
		}
		return column(&ColumnDef{Name: name, Kind: "id", Unsigned: true, AutoIncrement: true, Primary: true})
	})
	_ = t.Set("string", func(c goja.FunctionCall) goja.Value {
		return column(&ColumnDef{Name: v.str0(c, 0, "t.string"), Kind: "string", Length: intArg(c, 1, 255)})
	})
	for _, k := range []string{"text", "integer", "bigInteger", "boolean", "date", "datetime", "json", "uuid"} {
		_ = t.Set(k, simple(k))
	}
	_ = t.Set("decimal", func(c goja.FunctionCall) goja.Value {
		return column(&ColumnDef{Name: v.str0(c, 0, "t.decimal"), Kind: "decimal", Precision: intArg(c, 1, 12), Scale: intArg(c, 2, 2)})
	})
	_ = t.Set("enum", func(c goja.FunctionCall) goja.Value {
		return column(&ColumnDef{Name: v.str0(c, 0, "t.enum"), Kind: "enum", Values: v.strList(c.Argument(1), "t.enum")})
	})
	_ = t.Set("foreignId", func(c goja.FunctionCall) goja.Value {
		return column(&ColumnDef{Name: v.str0(c, 0, "t.foreignId"), Kind: "foreignId", Unsigned: true})
	})
	_ = t.Set("timestamps", func(goja.FunctionCall) goja.Value {
		column(&ColumnDef{Name: "created_at", Kind: "timestamp", UseCurrent: true})
		column(&ColumnDef{Name: "updated_at", Kind: "timestamp", UseCurrent: true, OnUpdate: true})
		return goja.Undefined()
	})
	cmd := func(c *TableCmd) {
		c.At = v.caller()
		bp.Cmds = append(bp.Cmds, c)
	}
	optName := func(c goja.FunctionCall, i int) string {
		s, _ := c.Argument(i).Export().(string)
		return s
	}
	_ = t.Set("index", func(c goja.FunctionCall) goja.Value {
		cmd(&TableCmd{Kind: "index", Columns: v.strList(c.Argument(0), "t.index"), Name: optName(c, 1)})
		return goja.Undefined()
	})
	_ = t.Set("unique", func(c goja.FunctionCall) goja.Value {
		cmd(&TableCmd{Kind: "unique", Columns: v.strList(c.Argument(0), "t.unique"), Name: optName(c, 1)})
		return goja.Undefined()
	})
	_ = t.Set("dropColumn", func(c goja.FunctionCall) goja.Value {
		for _, n := range v.strList(c.Argument(0), "t.dropColumn") {
			cmd(&TableCmd{Kind: "dropColumn", Name: n})
		}
		return goja.Undefined()
	})
	_ = t.Set("renameColumn", func(c goja.FunctionCall) goja.Value {
		cmd(&TableCmd{Kind: "renameColumn", From: v.str0(c, 0, "t.renameColumn"), To: v.str0(c, 1, "t.renameColumn")})
		return goja.Undefined()
	})
	drop := func(kind string) func(goja.FunctionCall) goja.Value {
		return func(c goja.FunctionCall) goja.Value {
			cmd(&TableCmd{Kind: kind, Name: v.str0(c, 0, "t."+kind)})
			return goja.Undefined()
		}
	}
	_ = t.Set("dropIndex", drop("dropIndex"))
	_ = t.Set("dropUnique", drop("dropIndex"))
	_ = t.Set("dropForeign", drop("dropForeign"))
	_ = t.Set("change", func(c goja.FunctionCall) goja.Value {
		o, _ := c.Argument(0).(*goja.Object)
		cd := cols[o]
		if cd == nil {
			v.throw("t.change(column): ส่งคอลัมน์ที่สร้างจาก t เช่น t.change(t.string('name', 100)) หรือใช้ .change()")
		}
		cd.Change = true
		return o
	})
	return t
}

// columnBuilder chains column modifiers.
func (v *vm) columnBuilder(cd *ColumnDef) *goja.Object {
	o := v.rt.NewObject()
	set := func(name string, fn func(c goja.FunctionCall)) {
		_ = o.Set(name, func(c goja.FunctionCall) goja.Value {
			fn(c)
			return o
		})
	}
	set("nullable", func(c goja.FunctionCall) {
		cd.Nullable = goja.IsUndefined(c.Argument(0)) || c.Argument(0).ToBoolean()
	})
	set("default", func(c goja.FunctionCall) {
		a := c.Argument(0)
		var x any
		if !goja.IsNull(a) && !goja.IsUndefined(a) {
			x = a.Export()
		}
		lit, err := LiteralDefault(x)
		if err != nil {
			v.throw("คอลัมน์ %s: %s", cd.Name, err.Error())
		}
		cd.Default = &lit
	})
	set("useCurrent", func(goja.FunctionCall) { cd.UseCurrent = true })
	set("useCurrentOnUpdate", func(goja.FunctionCall) { cd.OnUpdate = true })
	set("unique", func(goja.FunctionCall) { cd.Unique = true })
	set("index", func(goja.FunctionCall) { cd.Index = true })
	set("unsigned", func(goja.FunctionCall) { cd.Unsigned = true })
	set("comment", func(c goja.FunctionCall) {
		s := v.str0(c, 0, "comment")
		if len(s) > 1024 {
			v.throw("comment ยาวเกิน 1024 ตัวอักษร")
		}
		cd.Comment = &s
	})
	set("after", func(c goja.FunctionCall) { cd.After = v.str0(c, 0, "after") })
	// phase 7: case-sensitive text (utf8mb4_bin) or another allowed collation
	set("binary", func(goja.FunctionCall) { cd.Collation = "utf8mb4_bin" })
	set("collation", func(c goja.FunctionCall) {
		name := strings.ToLower(v.str0(c, 0, "collation"))
		if !Collations[name] {
			v.throw("collation %q ไม่รองรับ (utf8mb4_bin, utf8mb4_unicode_ci, utf8mb4_general_ci)", name)
		}
		cd.Collation = name
	})
	set("change", func(goja.FunctionCall) { cd.Change = true })
	set("references", func(c goja.FunctionCall) {
		r := &Reference{Table: v.str0(c, 0, "references"), Column: "id"}
		if s, ok := c.Argument(1).Export().(string); ok {
			r.Column = s
		}
		cd.Ref = r
	})
	set("onDelete", func(c goja.FunctionCall) {
		if cd.Ref == nil {
			v.throw("onDelete() ต้องเรียกหลัง references()")
		}
		cd.Ref.OnDelete = strings.ToLower(v.str0(c, 0, "onDelete"))
	})
	return o
}

// ---------------------------------------------------------------- bookkeeping

type ranRow struct {
	Status string
	Batch  int
}

func (d *Data) ranMigrations(ctx context.Context) (map[string]ranRow, error) {
	rows, err := d.admin.QueryContext(ctx, `SELECT name, status, batch FROM script_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ranRow{}
	for rows.Next() {
		var n string
		var r ranRow
		if err := rows.Scan(&n, &r.Status, &r.Batch); err != nil {
			return nil, err
		}
		out[n] = r
	}
	return out, rows.Err()
}

func (d *Data) record(ctx context.Context, name string, batch int, user, status string, dur time.Duration, errText *string) error {
	_, err := d.admin.ExecContext(ctx, `INSERT INTO script_migrations (name, batch, ran_at, ran_by, duration_ms, status, error) VALUES (?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE batch = VALUES(batch), ran_at = VALUES(ran_at), ran_by = VALUES(ran_by), duration_ms = VALUES(duration_ms),
		status = VALUES(status), error = VALUES(error)`, name, batch, time.Now().UTC(), clip(user, 191), dur.Milliseconds(), status, errText)
	return err
}

func pendingOf(list []migFile, ran map[string]ranRow) []migFile {
	return pendingFor(list, ran, Naming{})
}

// migKey is the script_migrations name of a migration of the unit named by
// n ("<id>/<file>" for a JS plugin).
func migKey(n Naming, name string) string {
	if n.Unit == "" {
		return name
	}
	return n.Unit + "/" + name
}

func pendingFor(list []migFile, ran map[string]ranRow, n Naming) []migFile {
	var out []migFile
	for _, m := range list {
		if ran[migKey(n, m.Name)].Status != "ran" {
			out = append(out, m)
		}
	}
	return out
}

// planner returns a dry-run planner for the unit named by n.
func (d *Data) planner(snap *Snapshot, n Naming, migration string) *Planner {
	p := &Planner{Snap: snap, Prefix: n.Prefix, Fallback: n.Fallback, Migration: migration}
	if n.Unit != "" {
		p.RelPrefix = UnitTablePrefix(n.Unit)
		p.NoRaw = true
	}
	return p
}

// ---------------------------------------------------------------- dry run

// planMigrations evaluates the pending migrations of files against an
// in-memory snapshot: their SQL, destructive changes and problems.
func (e *Engine) planMigrations(files map[string]string) (*MigrationsDry, []Problem) {
	if e.data == nil {
		_, probs := MigrationFiles(files)
		return &MigrationsDry{Pending: []PendingMigration{}}, probs
	}
	return e.planMigrationsFor(files, e.data.N)
}

// planMigrationsFor is planMigrations for the unit named by n.
func (e *Engine) planMigrationsFor(files map[string]string, n Naming) (*MigrationsDry, []Problem) {
	out := &MigrationsDry{Pending: []PendingMigration{}}
	list, probs := MigrationFiles(files)
	if len(list) == 0 || e.data == nil {
		return out, probs
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	ran, err := e.data.ranMigrations(ctx)
	if err != nil {
		return out, append(probs, Problem{Message: "อ่านประวัติ migration ไม่ได้: " + err.Error()})
	}
	snap, err := e.data.loadSnapshot(ctx, n)
	if err != nil {
		return out, append(probs, Problem{Message: "อ่านโครงสร้างฐานข้อมูลไม่ได้: " + err.Error()})
	}
	for _, mf := range pendingFor(list, ran, n) {
		v, def, ps := e.loadMigration(files, mf)
		if len(ps) > 0 {
			probs = append(probs, ps...)
			continue
		}
		p := e.data.planner(snap, n, migKey(n, mf.Name))
		up, _ := goja.AssertFunction(def.Get("up"))
		if err := v.invoke(up, 5*time.Second, v.schemaObject(p)); err != nil {
			probs = append(probs, problemOf(toRunError(err), mf.Path))
		}
		snap = p.Snap
		pm := PendingMigration{Name: mf.Name, SQL: p.SQL(), Destructive: p.Destructive}
		if pm.Destructive == nil {
			pm.Destructive = []Destructive{}
		}
		out.Pending = append(out.Pending, pm)
	}
	return out, probs
}

// ---------------------------------------------------------------- real runs

// MigrationFailed is a migration that failed while publishing / rolling back.
type MigrationFailed struct {
	Migration string
	Errors    []Problem
}

func (m *MigrationFailed) Error() string {
	if len(m.Errors) > 0 {
		return fmt.Sprintf("migration %s ล้มเหลว: %s", m.Migration, m.Errors[0].Message)
	}
	return "migration " + m.Migration + " ล้มเหลว"
}

// DestructiveRefused: publish needs confirmDestructive.
type DestructiveRefused struct{ List []Destructive }

func (d *DestructiveRefused) Error() string { return "destructive" }

// ErrNotLatest: only the newest batch can be rolled back.
var ErrNotLatest = errors.New("not latest batch")

// planner for a real run: statements run as they are planned; destructive
// ones back up their table first.
func (e *Engine) livePlanner(ctx context.Context, name string) (*Planner, error) {
	return e.livePlannerFor(ctx, name, e.data.N)
}

func (e *Engine) livePlannerFor(ctx context.Context, name string, n Naming) (*Planner, error) {
	d := e.data
	snap, err := d.loadSnapshot(ctx, n)
	if err != nil {
		return nil, err
	}
	backed := map[string]bool{}
	p := d.planner(snap, n, name)
	p.Exec = func(s Stmt) error {
		sctx, cancel := context.WithTimeout(context.Background(), ddlTimeout)
		defer cancel()
		if s.Backup != "" && !backed[s.Backup] {
			if _, err := d.backupWith(sctx, n, s.Backup, name); err != nil {
				return fmt.Errorf("สำรองตาราง %s ไม่สำเร็จ: %v", s.Backup, err)
			}
			backed[s.Backup] = true
		}
		conn := d.code
		if s.Meta {
			conn = d.admin
		}
		if _, err := conn.ExecContext(sctx, s.SQL); err != nil {
			var me *mysql.MySQLError
			if errors.As(err, &me) {
				return fmt.Errorf("MySQL %d: %s", me.Number, me.Message)
			}
			return err
		}
		return nil
	}
	p.Reload = func() (*Snapshot, error) {
		rctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
		defer cancel()
		return d.loadSnapshot(rctx, n)
	}
	return p, nil
}

// runMigrations runs the pending migrations of files as one batch.
func (e *Engine) runMigrations(files map[string]string, user string) ([]string, error) {
	if e.data == nil {
		return nil, nil
	}
	return e.runMigrationsFor(files, user, e.data.N)
}

// runMigrationsFor is runMigrations for the unit named by n.
func (e *Engine) runMigrationsFor(files map[string]string, user string, n Naming) ([]string, error) {
	d := e.data
	list, _ := MigrationFiles(files)
	if len(list) == 0 || d == nil {
		return nil, nil
	}
	src := "migrate"
	if n.Unit != "" {
		src = n.Unit + ": migrate"
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	ran, err := d.ranMigrations(ctx)
	if err != nil {
		return nil, err
	}
	pending := pendingFor(list, ran, n)
	if len(pending) == 0 {
		return nil, nil
	}
	var batch int
	if err := d.admin.QueryRowContext(ctx, `SELECT COALESCE(MAX(batch), 0) + 1 FROM script_migrations`).Scan(&batch); err != nil {
		return nil, err
	}
	var done []string
	for _, mf := range pending {
		start := time.Now()
		fail := func(ps []Problem) ([]string, error) {
			msg := ps[0].Message
			if ps[0].File != "" {
				msg = fmt.Sprintf("%s (%s:%d:%d)", ps[0].Message, ps[0].File, ps[0].Line, ps[0].Column)
			}
			_ = d.record(context.Background(), migKey(n, mf.Name), batch, user, "failed", time.Since(start), &msg)
			e.logUnit(n.Unit, "error", src, fmt.Sprintf("migration %s ล้มเหลว: %s", mf.Name, msg))
			return done, &MigrationFailed{Migration: mf.Name, Errors: ps}
		}
		v, def, ps := e.loadMigration(files, mf)
		if len(ps) > 0 {
			return fail(ps)
		}
		// each migration gets its own time to read the schema: the batch's ctx would run out on a long list
		// (a plugin installed fresh with many migrations on a slow server)
		pctx, pcancel := context.WithTimeout(context.Background(), adminTimeout)
		p, err := e.livePlannerFor(pctx, migKey(n, mf.Name), n)
		pcancel()
		if err != nil {
			return done, err
		}
		up, _ := goja.AssertFunction(def.Get("up"))
		if err := v.invoke(up, ddlTimeout, v.schemaObject(p)); err != nil {
			return fail([]Problem{problemOf(toRunError(err), mf.Path)})
		}
		if err := d.record(context.Background(), migKey(n, mf.Name), batch, user, "ran", time.Since(start), nil); err != nil {
			return done, err
		}
		e.logUnit(n.Unit, "info", src, fmt.Sprintf("รัน migration %s (%d คำสั่ง, %d ms)", mf.Name, len(p.Stmts), time.Since(start).Milliseconds()))
		done = append(done, mf.Name)
	}
	return done, nil
}

// RollbackMigration runs the down() of a migration of the newest batch and
// marks it rolled back.
func (e *Engine) RollbackMigration(name, user string) error {
	e.pubMu.Lock()
	defer e.pubMu.Unlock()
	d := e.data
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	var status string
	var batch, maxBatch int
	err := d.admin.QueryRowContext(ctx, `SELECT status, batch FROM script_migrations WHERE name = ?`, name).Scan(&status, &batch)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "ran") {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := d.admin.QueryRowContext(ctx, `SELECT COALESCE(MAX(batch), 0) FROM script_migrations WHERE status = 'ran' AND name NOT LIKE '%/%'`).Scan(&maxBatch); err != nil {
		return err
	}
	if batch != maxBatch {
		return ErrNotLatest
	}
	files := map[string]string{}
	if live, err := e.LiveVersion(); err == nil && live > 0 {
		if row, err := e.DB.GetVersion(live, live, false); err == nil {
			files = row.FileMap
		}
	}
	mf := findMig(files, name)
	if mf == nil {
		if draft, err := e.DB.Draft(); err == nil {
			files = draft
			mf = findMig(files, name)
		}
	}
	if mf == nil {
		return &MigrationFailed{Migration: name, Errors: []Problem{{Message: "ไม่พบไฟล์ migration นี้ในเวอร์ชันที่ใช้งานหรือร่าง"}}}
	}
	v, def, ps := e.loadMigration(files, *mf)
	if len(ps) > 0 {
		return &MigrationFailed{Migration: name, Errors: ps}
	}
	down, ok := goja.AssertFunction(def.Get("down"))
	if !ok {
		return &MigrationFailed{Migration: name, Errors: []Problem{{File: mf.Path, Line: 1, Column: 1, Message: "migration นี้ไม่มี down(schema)"}}}
	}
	p, err := e.livePlanner(ctx, name)
	if err != nil {
		return err
	}
	start := time.Now()
	if err := v.invoke(down, ddlTimeout, v.schemaObject(p)); err != nil {
		pr := problemOf(toRunError(err), mf.Path)
		msg := fmt.Sprintf("down: %s (%s:%d:%d)", pr.Message, pr.File, pr.Line, pr.Column)
		_, _ = d.admin.ExecContext(context.Background(), `UPDATE script_migrations SET error = ? WHERE name = ?`, msg, name)
		return &MigrationFailed{Migration: name, Errors: []Problem{pr}}
	}
	if err := d.record(context.Background(), name, batch, user, "rolled_back", time.Since(start), nil); err != nil {
		return err
	}
	e.Log("warn", "migrate", fmt.Sprintf("ย้อน migration %s โดย %s", name, user), 0)
	if err := d.RefreshViews(); err != nil {
		e.Log("error", "migrate", "สร้าง view core_* ใหม่ไม่สำเร็จ: "+err.Error(), 0)
	}
	return nil
}

func findMig(files map[string]string, name string) *migFile {
	list, _ := MigrationFiles(files)
	for _, m := range list {
		if m.Name == name {
			mm := m
			return &mm
		}
	}
	return nil
}

// MigrationInfo is a row of GET /db/migrations.
type MigrationInfo struct {
	Name       string     `json:"name"`
	Batch      *int       `json:"batch"`
	RanAt      *time.Time `json:"ranAt"`
	RanBy      *string    `json:"ranBy"`
	DurationMs *int       `json:"durationMs"`
	Status     string     `json:"status"` // ran | failed | rolled_back | pending
	Error      *string    `json:"error"`
	Pending    bool       `json:"pending"` // would run on the next publish
	InDraft    bool       `json:"inDraft"`
	InLive     bool       `json:"inLive"`
}

// Migrations lists history and pending migrations (draft and live), by name.
func (e *Engine) Migrations() ([]MigrationInfo, error) {
	d := e.data
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	byName := map[string]*MigrationInfo{}
	rows, err := d.admin.QueryContext(ctx, `SELECT name, batch, ran_at, ran_by, duration_ms, status, error FROM script_migrations WHERE name NOT LIKE '%/%'`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		m := &MigrationInfo{}
		var batch, dur int
		var at time.Time
		var by string
		var errText sql.NullString
		if err := rows.Scan(&m.Name, &batch, &at, &by, &dur, &m.Status, &errText); err != nil {
			rows.Close()
			return nil, err
		}
		m.Batch, m.RanAt, m.RanBy, m.DurationMs = &batch, &at, &by, &dur
		if errText.Valid {
			m.Error = &errText.String
		}
		byName[m.Name] = m
	}
	rows.Close()
	mark := func(files map[string]string, draft bool) {
		list, _ := MigrationFiles(files)
		for _, mf := range list {
			m := byName[mf.Name]
			if m == nil {
				m = &MigrationInfo{Name: mf.Name, Status: "pending"}
				byName[mf.Name] = m
			}
			if draft {
				m.InDraft = true
			} else {
				m.InLive = true
			}
			if m.Status != "ran" {
				m.Pending = true
			}
		}
	}
	if draft, err := e.DB.Draft(); err == nil {
		mark(draft, true)
	}
	if live, err := e.LiveVersion(); err == nil && live > 0 {
		if row, err := e.DB.GetVersion(live, live, false); err == nil {
			mark(row.FileMap, false)
		}
	}
	out := make([]MigrationInfo, 0, len(byName))
	for _, m := range byName {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// pendingCount counts draft migrations that have not run.
func (e *Engine) pendingCount(draft map[string]string) int {
	list, _ := MigrationFiles(draft)
	if len(list) == 0 || e.data == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	ran, err := e.data.ranMigrations(ctx)
	if err != nil {
		return 0
	}
	return len(pendingOf(list, ran))
}
