package sitecode

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Schema builder: migrations describe tables with schema.create / table /
// drop / rename; the Planner turns each call into MySQL DDL, checks it
// against an in-memory Snapshot of the code database (so dry runs list the
// exact SQL without touching anything) and flags destructive changes.

// identRe is the rule for table and column names developers create.
var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// reserved name prefixes of the code database (views, bookkeeping, the
// fallback prefix and backup copies).
var reservedPrefixes = []string{"core_", "script_", "sc_", "bak_", "p_"}

// ValidTableName checks a table name a migration creates or renames to.
func ValidTableName(name string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("ชื่อตาราง %q ไม่ถูกต้อง: ใช้ a-z 0-9 _ ขึ้นต้นด้วยตัวอักษร ยาวไม่เกิน 63 ตัว", name)
	}
	if name == "kv_docs" {
		return fmt.Errorf("ชื่อตาราง %q สงวนไว้สำหรับ db.docs", name)
	}
	for _, p := range reservedPrefixes {
		if strings.HasPrefix(name, p) {
			return fmt.Errorf("ชื่อตาราง %q ขึ้นต้นด้วย %s ซึ่งสงวนไว้โดยระบบ", name, p)
		}
	}
	return nil
}

// ValidColumnName checks a column (or index) name a migration creates.
func ValidColumnName(name string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("ชื่อคอลัมน์ %q ไม่ถูกต้อง: ใช้ a-z 0-9 _ ขึ้นต้นด้วยตัวอักษร ยาวไม่เกิน 63 ตัว", name)
	}
	return nil
}

// quoteIdent backticks an identifier (already validated).
func quoteIdent(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// quoteString is a MySQL string literal.
func quoteString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `''`, "\x00", `\0`, "\n", `\n`, "\r", `\r`, "\x1a", `\Z`)
	return "'" + r.Replace(s) + "'"
}

// shortName keeps a generated identifier within MySQL's 64 characters.
func shortName(s string) string {
	if len(s) <= 64 {
		return s
	}
	h := sha1.Sum([]byte(s))
	return s[:55] + "_" + hex.EncodeToString(h[:4])
}

// coreViews are the read-only views of CMS data in the code database.
var coreViews = map[string][]string{
	"core_posts":      {"id", "type", "title", "slug", "status", "excerpt", "author_id", "featured_media_id", "published_at", "created_at", "updated_at"},
	"core_terms":      {"id", "taxonomy", "name", "slug"},
	"core_post_terms": {"post_id", "term_id"},
	"core_users":      {"id", "name", "email", "role", "status", "created_at"},
}

// ---------------------------------------------------------------- columns

// Reference is a .references(table, column).onDelete(...) on a column.
type Reference struct {
	Table, Column, OnDelete string
}

// ColumnDef is a column as a migration describes it.
type ColumnDef struct {
	Name      string
	Kind      string // id string text integer bigInteger decimal boolean date datetime json enum uuid foreignId timestamp
	Length    int
	Precision int
	Scale     int
	Values    []string // enum

	Nullable      bool
	Unsigned      bool
	AutoIncrement bool
	Primary       bool
	Default       *string // SQL literal
	UseCurrent    bool
	OnUpdate      bool // ON UPDATE CURRENT_TIMESTAMP(3)
	Unique        bool
	Index         bool
	Comment       *string
	// Collation of a text column (phase 7: .binary() / .collation(name));
	// "" = the table default utf8mb4_unicode_ci.
	Collation string
	After     string
	Change    bool
	Ref       *Reference
	At        Problem
}

// SQLType is the MySQL column type, written like information_schema's
// COLUMN_TYPE (lower case) so snapshots compare.
func (c *ColumnDef) SQLType() string {
	var t string
	switch c.Kind {
	case "id", "foreignId", "bigInteger":
		t = "bigint"
	case "integer":
		t = "int"
	case "string":
		t = "varchar(" + strconv.Itoa(c.Length) + ")"
	case "text":
		t = "text"
	case "decimal":
		t = fmt.Sprintf("decimal(%d,%d)", c.Precision, c.Scale)
	case "boolean":
		t = "tinyint(1)"
	case "date":
		t = "date"
	case "datetime", "timestamp":
		t = "datetime(3)"
	case "json":
		t = "json"
	case "uuid":
		t = "char(36)"
	case "enum":
		vals := make([]string, len(c.Values))
		for i, v := range c.Values {
			vals[i] = quoteString(v)
		}
		t = "enum(" + strings.Join(vals, ",") + ")"
	default:
		t = c.Kind
	}
	if c.Unsigned && (t == "bigint" || t == "int" || strings.HasPrefix(t, "decimal")) {
		t += " unsigned"
	}
	return t
}

// exprDefault: TEXT/JSON columns only take expression defaults.
func (c *ColumnDef) exprDefault() bool { return c.Kind == "text" || c.Kind == "json" }

// Definition is the column definition in CREATE/ALTER TABLE.
func (c *ColumnDef) Definition() string {
	var b strings.Builder
	b.WriteString(quoteIdent(c.Name) + " " + strings.ToUpper(c.SQLType()))
	if strings.HasPrefix(c.SQLType(), "enum(") { // keep the values' case
		b.Reset()
		b.WriteString(quoteIdent(c.Name) + " " + "ENUM" + strings.TrimPrefix(c.SQLType(), "enum"))
	}
	if c.Collation != "" {
		b.WriteString(" CHARACTER SET utf8mb4 COLLATE " + c.Collation)
	}
	if c.Nullable {
		b.WriteString(" NULL")
	} else {
		b.WriteString(" NOT NULL")
	}
	switch {
	case c.UseCurrent:
		b.WriteString(" DEFAULT CURRENT_TIMESTAMP(3)")
	case c.Default != nil && c.exprDefault() && *c.Default != "NULL":
		b.WriteString(" DEFAULT (" + *c.Default + ")")
	case c.Default != nil:
		b.WriteString(" DEFAULT " + *c.Default)
	}
	if c.OnUpdate {
		b.WriteString(" ON UPDATE CURRENT_TIMESTAMP(3)")
	}
	if c.AutoIncrement {
		b.WriteString(" AUTO_INCREMENT")
	}
	if c.Comment != nil {
		b.WriteString(" COMMENT " + quoteString(*c.Comment))
	}
	return b.String()
}

// Check validates a column definition.
func (c *ColumnDef) Check() error {
	if err := ValidColumnName(c.Name); err != nil {
		return err
	}
	switch c.Kind {
	case "string":
		if c.Length < 1 || c.Length > 16383 {
			return fmt.Errorf("คอลัมน์ %s: ความยาว string ต้องอยู่ระหว่าง 1 ถึง 16383", c.Name)
		}
	case "decimal":
		if c.Precision < 1 || c.Precision > 65 || c.Scale < 0 || c.Scale > 30 || c.Scale > c.Precision {
			return fmt.Errorf("คอลัมน์ %s: decimal(precision 1-65, scale 0-30 และไม่เกิน precision)", c.Name)
		}
	case "enum":
		if len(c.Values) == 0 || len(c.Values) > 1000 {
			return fmt.Errorf("คอลัมน์ %s: enum ต้องมีค่าอย่างน้อย 1 ค่า", c.Name)
		}
		seen := map[string]bool{}
		for _, v := range c.Values {
			if v == "" || len(v) > 255 || seen[strings.ToLower(v)] {
				return fmt.Errorf("คอลัมน์ %s: ค่า enum ว่าง ซ้ำ หรือยาวเกิน 255 ตัว", c.Name)
			}
			seen[strings.ToLower(v)] = true
		}
	}
	if c.Collation != "" && !(c.Kind == "string" || c.Kind == "text" || c.Kind == "enum" || c.Kind == "uuid") {
		return fmt.Errorf("คอลัมน์ %s: binary() / collation() ใช้ได้กับ string, text, enum และ uuid เท่านั้น", c.Name)
	}
	if c.Collation != "" && !Collations[c.Collation] {
		return fmt.Errorf("คอลัมน์ %s: collation %q ไม่รองรับ (utf8mb4_bin, utf8mb4_unicode_ci, utf8mb4_general_ci)", c.Name, c.Collation)
	}
	if c.Unsigned && !(c.Kind == "integer" || c.Kind == "bigInteger" || c.Kind == "decimal" || c.Kind == "id" || c.Kind == "foreignId") {
		return fmt.Errorf("คอลัมน์ %s: unsigned() ใช้ได้กับตัวเลขเท่านั้น", c.Name)
	}
	if c.Default != nil && *c.Default == "NULL" && !c.Nullable {
		return fmt.Errorf("คอลัมน์ %s: default(null) ต้องใช้คู่กับ nullable()", c.Name)
	}
	if c.UseCurrent && c.Kind != "datetime" && c.Kind != "timestamp" {
		return fmt.Errorf("คอลัมน์ %s: useCurrent() ใช้ได้กับ datetime เท่านั้น", c.Name)
	}
	if c.Ref != nil {
		switch c.Ref.OnDelete {
		case "", "cascade", "restrict", "set null", "no action":
		default:
			return fmt.Errorf("คอลัมน์ %s: onDelete ต้องเป็น 'cascade', 'restrict' หรือ 'set null'", c.Name)
		}
		if c.Ref.OnDelete == "set null" && !c.Nullable {
			return fmt.Errorf("คอลัมน์ %s: onDelete('set null') ต้องใช้คู่กับ nullable()", c.Name)
		}
	}
	return nil
}

// LiteralDefault converts a JS default value to a SQL literal.
func LiteralDefault(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "NULL", nil
	case bool:
		if x {
			return "1", nil
		}
		return "0", nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case string:
		return quoteString(x), nil
	}
	return "", fmt.Errorf("default() รับได้เฉพาะข้อความ ตัวเลข true/false หรือ null")
}

// ---------------------------------------------------------------- blueprints

// TableCmd is a table-level command inside schema.create / schema.table.
type TableCmd struct {
	Kind     string // index unique dropColumn renameColumn dropIndex dropForeign
	Columns  []string
	Name     string
	From, To string
	At       Problem
}

// Blueprint collects what a schema.create / schema.table callback declared.
type Blueprint struct {
	Table   string
	Create  bool
	Columns []*ColumnDef
	Cmds    []*TableCmd
}

// ---------------------------------------------------------------- snapshot

// SColumn is a column in the snapshot.
type SColumn struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"` // COLUMN_TYPE, lower case
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default"`
	Key      string  `json:"key"`
	Extra    string  `json:"extra"`
	Comment  string  `json:"comment"`
	// Collation when not the table default (utf8mb4_bin for case-sensitive keys).
	Collation string `json:"collation,omitempty"`
}

// Collations are the column collations the schema builder accepts.
var Collations = map[string]bool{"utf8mb4_bin": true, "utf8mb4_unicode_ci": true, "utf8mb4_general_ci": true}

// SIndex is an index in the snapshot.
type SIndex struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

// SFK is a foreign key constraint in the snapshot.
type SFK struct {
	Name      string `json:"-"`
	Column    string `json:"column"`
	RefTable  string `json:"refTable"`
	RefColumn string `json:"refColumn"`
	OnDelete  string `json:"onDelete"`
}

// SRel is a relationship to a core_* view (metadata only: MySQL foreign
// keys cannot reference views).
type SRel struct {
	Column    string `json:"column"`
	RefTable  string `json:"refTable"`
	RefColumn string `json:"refColumn"`
}

// STable is a developer table in the snapshot (logical name).
type STable struct {
	Name      string
	Comment   string
	Columns   []*SColumn
	Indexes   []*SIndex
	FKs       []*SFK
	Relations []*SRel
}

func (t *STable) column(name string) *SColumn {
	for _, c := range t.Columns {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (t *STable) index(name string) *SIndex {
	for _, i := range t.Indexes {
		if i.Name == name {
			return i
		}
	}
	return nil
}

func (t *STable) clone() *STable {
	n := &STable{Name: t.Name, Comment: t.Comment}
	for _, c := range t.Columns {
		cc := *c
		n.Columns = append(n.Columns, &cc)
	}
	for _, i := range t.Indexes {
		ii := *i
		ii.Columns = append([]string{}, i.Columns...)
		n.Indexes = append(n.Indexes, &ii)
	}
	for _, f := range t.FKs {
		ff := *f
		n.FKs = append(n.FKs, &ff)
	}
	for _, r := range t.Relations {
		rr := *r
		n.Relations = append(n.Relations, &rr)
	}
	return n
}

// Snapshot is the structure of the developer tables.
type Snapshot struct {
	Tables map[string]*STable
}

// NewSnapshot returns an empty snapshot.
func NewSnapshot() *Snapshot { return &Snapshot{Tables: map[string]*STable{}} }

// Clone deep-copies the snapshot.
func (s *Snapshot) Clone() *Snapshot {
	n := NewSnapshot()
	for k, t := range s.Tables {
		n.Tables[k] = t.clone()
	}
	return n
}

// Names lists the tables sorted.
func (s *Snapshot) Names() []string {
	out := make([]string, 0, len(s.Tables))
	for k := range s.Tables {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- planner

// Destructive is a change that may lose data (publish needs confirmation).
type Destructive struct {
	Migration string `json:"migration"`
	Op        string `json:"op"` // drop_table drop_column change_column rename_table rename_column raw
	Table     string `json:"table"`
	Column    string `json:"column"`
}

// Stmt is one planned statement.
type Stmt struct {
	SQL    string
	Meta   bool   // bookkeeping (script_relations), run by the CMS user
	Backup string // logical table copied before this statement runs
	At     Problem
}

// Planner turns schema builder calls into statements against a snapshot.
// With Exec set (a real run) each statement runs as soon as it is planned.
type Planner struct {
	Snap     *Snapshot
	Prefix   string // "sc_" in fallback mode; "p_<id>_" for a JS plugin
	Fallback bool
	// RelPrefix qualifies the script_relations rows of a JS plugin
	// ("p_<id>_"; "" for Site Code); NoRaw refuses schema.raw (plugins: raw
	// SQL could reach other units' tables).
	RelPrefix   string
	NoRaw       bool
	Migration   string
	Stmts       []Stmt
	Destructive []Destructive
	// Exec runs a statement (nil = dry run). Reload re-reads the snapshot
	// after schema.raw in a real run.
	Exec   func(Stmt) error
	Reload func() (*Snapshot, error)
}

func (p *Planner) phys(table string) string { return p.Prefix + table }

func (p *Planner) relKey(table string) string { return p.RelPrefix + table }

// checkPhys refuses names whose physical form is too long for MySQL.
func (p *Planner) checkPhys(table string) error {
	if len(p.phys(table)) > 64 {
		return fmt.Errorf("ชื่อตาราง %q ยาวเกินไป (รวม prefix %q แล้วต้องไม่เกิน 64 ตัว)", table, p.Prefix)
	}
	return nil
}

func (p *Planner) emit(s Stmt) error {
	p.Stmts = append(p.Stmts, s)
	if p.Exec != nil {
		return p.Exec(s)
	}
	return nil
}

func (p *Planner) destructive(op, table, column string) {
	p.Destructive = append(p.Destructive, Destructive{Migration: p.Migration, Op: op, Table: table, Column: column})
}

// SQL lists the planned statements (bookkeeping as SQL comments).
func (p *Planner) SQL() []string {
	out := []string{}
	for _, s := range p.Stmts {
		if s.Meta {
			continue
		}
		out = append(out, s.SQL)
	}
	return out
}

func (p *Planner) relSQL(table string, r *SRel) string {
	return fmt.Sprintf("INSERT INTO script_relations (table_name, column_name, ref_table, ref_column) VALUES (%s, %s, %s, %s) "+
		"ON DUPLICATE KEY UPDATE ref_table = VALUES(ref_table), ref_column = VALUES(ref_column)",
		quoteString(p.relKey(table)), quoteString(r.Column), quoteString(r.RefTable), quoteString(r.RefColumn))
}

// fkName names a foreign key constraint (unique per schema).
func (p *Planner) fkName(table, col string) string {
	return shortName("fk_" + p.phys(table) + "_" + col)
}

func idxName(cols []string, suffix string) string {
	return shortName(strings.Join(cols, "_") + "_" + suffix)
}

// checkRef validates a reference and returns the constraint clause ("" for
// a core_* relation, which is metadata only).
func (p *Planner) checkRef(table string, c *ColumnDef, self *STable) (string, *SRel, error) {
	r := c.Ref
	if r.Column == "" {
		r.Column = "id"
	}
	if strings.HasPrefix(r.Table, "core_") {
		cols, ok := coreViews[r.Table]
		if !ok {
			return "", nil, fmt.Errorf("references(%q): ไม่มี view นี้ (มี core_posts, core_users, core_terms, core_post_terms)", r.Table)
		}
		found := false
		for _, cc := range cols {
			found = found || cc == r.Column
		}
		if !found {
			return "", nil, fmt.Errorf("references(%q, %q): ไม่มีคอลัมน์นี้ใน view", r.Table, r.Column)
		}
		return "", &SRel{Column: c.Name, RefTable: r.Table, RefColumn: r.Column}, nil
	}
	ref := p.Snap.Tables[r.Table]
	if r.Table == table {
		ref = self
	}
	if ref == nil {
		return "", nil, fmt.Errorf("references(%q): ไม่พบตารางนี้ (สร้างตารางที่อ้างถึงใน migration ก่อนหน้า)", r.Table)
	}
	if ref.column(r.Column) == nil {
		return "", nil, fmt.Errorf("references(%q, %q): ไม่พบคอลัมน์ %s ในตาราง %s", r.Table, r.Column, r.Column, r.Table)
	}
	clause := fmt.Sprintf("CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)", quoteIdent(p.fkName(table, c.Name)),
		quoteIdent(c.Name), quoteIdent(p.phys(r.Table)), quoteIdent(r.Column))
	if r.OnDelete != "" {
		clause += " ON DELETE " + strings.ToUpper(r.OnDelete)
	}
	return clause, nil, nil
}

func scol(c *ColumnDef) *SColumn {
	sc := &SColumn{Name: c.Name, Type: c.SQLType(), Nullable: c.Nullable, Default: c.Default}
	if c.UseCurrent {
		d := "CURRENT_TIMESTAMP(3)"
		sc.Default = &d
	}
	if c.AutoIncrement {
		sc.Extra = "auto_increment"
	}
	if c.Comment != nil {
		sc.Comment = *c.Comment
	}
	if c.Collation != "" && c.Collation != "utf8mb4_unicode_ci" {
		sc.Collation = c.Collation
	}
	return sc
}

func (p *Planner) checkCmdCols(t *STable, cols []string) error {
	if len(cols) == 0 {
		return fmt.Errorf("ต้องระบุคอลัมน์อย่างน้อย 1 คอลัมน์")
	}
	for _, c := range cols {
		if t.column(c) == nil {
			return fmt.Errorf("ไม่พบคอลัมน์ %s ในตาราง %s", c, t.Name)
		}
	}
	return nil
}

// Create plans schema.create.
func (p *Planner) Create(bp *Blueprint, at Problem) error {
	if err := ValidTableName(bp.Table); err != nil {
		return err
	}
	if err := p.checkPhys(bp.Table); err != nil {
		return err
	}
	if _, ok := p.Snap.Tables[bp.Table]; ok {
		return fmt.Errorf("schema.create: มีตาราง %s อยู่แล้ว", bp.Table)
	}
	if len(bp.Columns) == 0 {
		return fmt.Errorf("schema.create(%q): ต้องมีอย่างน้อย 1 คอลัมน์", bp.Table)
	}
	t := &STable{Name: bp.Table}
	var lines, keys []string
	var rels []*SRel
	var primary []string
	for _, c := range bp.Columns {
		if err := c.Check(); err != nil {
			return err
		}
		if c.Change || c.After != "" {
			return fmt.Errorf("คอลัมน์ %s: change()/after() ใช้ได้เฉพาะใน schema.table", c.Name)
		}
		if t.column(c.Name) != nil {
			return fmt.Errorf("schema.create(%q): คอลัมน์ %s ซ้ำ", bp.Table, c.Name)
		}
		t.Columns = append(t.Columns, scol(c))
		lines = append(lines, "  "+c.Definition())
		if c.Primary {
			primary = append(primary, c.Name)
		}
		if c.Unique {
			n := idxName([]string{c.Name}, "unique")
			keys = append(keys, "  UNIQUE KEY "+quoteIdent(n)+" ("+quoteIdent(c.Name)+")")
			t.Indexes = append(t.Indexes, &SIndex{Name: n, Columns: []string{c.Name}, Unique: true})
		}
		if c.Index {
			n := idxName([]string{c.Name}, "index")
			keys = append(keys, "  KEY "+quoteIdent(n)+" ("+quoteIdent(c.Name)+")")
			t.Indexes = append(t.Indexes, &SIndex{Name: n, Columns: []string{c.Name}})
		}
	}
	for _, c := range bp.Columns {
		if c.Ref == nil {
			continue
		}
		clause, rel, err := p.checkRef(bp.Table, c, t)
		if err != nil {
			return err
		}
		if rel != nil {
			rels = append(rels, rel)
			t.Relations = append(t.Relations, rel)
		} else {
			keys = append(keys, "  "+clause)
			t.FKs = append(t.FKs, &SFK{Name: p.fkName(bp.Table, c.Name), Column: c.Name, RefTable: c.Ref.Table, RefColumn: c.Ref.Column, OnDelete: c.Ref.OnDelete})
		}
	}
	for _, cmd := range bp.Cmds {
		switch cmd.Kind {
		case "index", "unique":
			if err := p.checkCmdCols(t, cmd.Columns); err != nil {
				return err
			}
			n := cmd.Name
			if n == "" {
				n = idxName(cmd.Columns, cmd.Kind)
			} else if err := ValidColumnName(n); err != nil {
				return err
			}
			if t.index(n) != nil {
				return fmt.Errorf("schema.create(%q): index %s ซ้ำ", bp.Table, n)
			}
			q := make([]string, len(cmd.Columns))
			for i, c := range cmd.Columns {
				q[i] = quoteIdent(c)
			}
			kw := "KEY"
			if cmd.Kind == "unique" {
				kw = "UNIQUE KEY"
			}
			keys = append(keys, "  "+kw+" "+quoteIdent(n)+" ("+strings.Join(q, ", ")+")")
			t.Indexes = append(t.Indexes, &SIndex{Name: n, Columns: cmd.Columns, Unique: cmd.Kind == "unique"})
		default:
			return fmt.Errorf("schema.create(%q): t.%s() ใช้ได้เฉพาะใน schema.table", bp.Table, cmd.Kind)
		}
	}
	if len(primary) > 0 {
		q := make([]string, len(primary))
		for i, c := range primary {
			q[i] = quoteIdent(c)
		}
		lines = append(lines, "  PRIMARY KEY ("+strings.Join(q, ", ")+")")
		t.Indexes = append([]*SIndex{{Name: "PRIMARY", Columns: primary, Unique: true}}, t.Indexes...)
		for _, c := range primary {
			t.column(c).Key = "PRI"
		}
	}
	lines = append(lines, keys...)
	sql := "CREATE TABLE " + quoteIdent(p.phys(bp.Table)) + " (\n" + strings.Join(lines, ",\n") +
		"\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"
	if err := p.emit(Stmt{SQL: sql, At: at}); err != nil {
		return err
	}
	p.Snap.Tables[bp.Table] = t
	for _, r := range rels {
		if err := p.emit(Stmt{SQL: p.relSQL(bp.Table, r), Meta: true, At: at}); err != nil {
			return err
		}
	}
	return nil
}

// alterSQL prefixes an ALTER TABLE clause.
func (p *Planner) alterSQL(table, clause string) string {
	return "ALTER TABLE " + quoteIdent(p.phys(table)) + " " + clause
}

// Alter plans schema.table.
func (p *Planner) Alter(bp *Blueprint, at Problem) error {
	t0, ok := p.Snap.Tables[bp.Table]
	if !ok {
		return fmt.Errorf("schema.table: ไม่พบตาราง %s", bp.Table)
	}
	t := t0.clone()
	commit := func() { p.Snap.Tables[bp.Table] = t }
	defer commit() // statements that already ran stay in the snapshot
	for _, c := range bp.Columns {
		if err := c.Check(); err != nil {
			return err
		}
		pos := c.At
		if pos.File == "" {
			pos = at
		}
		if c.Change {
			old := t.column(c.Name)
			if old == nil {
				return fmt.Errorf("change(): ไม่พบคอลัมน์ %s ในตาราง %s", c.Name, bp.Table)
			}
			if Narrower(old.Type, c.SQLType()) {
				p.destructive("change_column", bp.Table, c.Name)
			}
			clause := "MODIFY COLUMN " + c.Definition()
			if c.After != "" {
				if t.column(c.After) == nil {
					return fmt.Errorf("after(%q): ไม่พบคอลัมน์นี้", c.After)
				}
				clause += " AFTER " + quoteIdent(c.After)
			}
			bk := ""
			if Narrower(old.Type, c.SQLType()) {
				bk = bp.Table
			}
			if err := p.emit(Stmt{SQL: p.alterSQL(bp.Table, clause), Backup: bk, At: pos}); err != nil {
				return err
			}
			nc := scol(c)
			nc.Key = old.Key
			*old = *nc
		} else {
			if t.column(c.Name) != nil {
				return fmt.Errorf("schema.table(%q): มีคอลัมน์ %s อยู่แล้ว (ใช้ .change() เพื่อแก้ไข)", bp.Table, c.Name)
			}
			if c.Primary {
				return fmt.Errorf("schema.table(%q): เพิ่ม primary key ภายหลังไม่ได้", bp.Table)
			}
			clause := "ADD COLUMN " + c.Definition()
			if c.After != "" {
				if t.column(c.After) == nil {
					return fmt.Errorf("after(%q): ไม่พบคอลัมน์นี้", c.After)
				}
				clause += " AFTER " + quoteIdent(c.After)
			}
			if err := p.emit(Stmt{SQL: p.alterSQL(bp.Table, clause), At: pos}); err != nil {
				return err
			}
			nc := scol(c)
			if c.After != "" {
				idx := 0
				for i, cc := range t.Columns {
					if cc.Name == c.After {
						idx = i + 1
					}
				}
				t.Columns = append(t.Columns[:idx], append([]*SColumn{nc}, t.Columns[idx:]...)...)
			} else {
				t.Columns = append(t.Columns, nc)
			}
		}
		for _, kind := range []string{"unique", "index"} {
			if (kind == "unique" && !c.Unique) || (kind == "index" && !c.Index) {
				continue
			}
			n := idxName([]string{c.Name}, kind)
			if t.index(n) != nil {
				continue
			}
			kw := "ADD INDEX "
			if kind == "unique" {
				kw = "ADD UNIQUE INDEX "
			}
			if err := p.emit(Stmt{SQL: p.alterSQL(bp.Table, kw+quoteIdent(n)+" ("+quoteIdent(c.Name)+")"), At: pos}); err != nil {
				return err
			}
			t.Indexes = append(t.Indexes, &SIndex{Name: n, Columns: []string{c.Name}, Unique: kind == "unique"})
		}
		if c.Ref != nil {
			clause, rel, err := p.checkRef(bp.Table, c, t)
			if err != nil {
				return err
			}
			if rel != nil {
				if err := p.emit(Stmt{SQL: p.relSQL(bp.Table, rel), Meta: true, At: pos}); err != nil {
					return err
				}
				t.Relations = append(t.Relations, rel)
			} else {
				if err := p.emit(Stmt{SQL: p.alterSQL(bp.Table, "ADD "+clause), At: pos}); err != nil {
					return err
				}
				t.FKs = append(t.FKs, &SFK{Name: p.fkName(bp.Table, c.Name), Column: c.Name, RefTable: c.Ref.Table, RefColumn: c.Ref.Column, OnDelete: c.Ref.OnDelete})
			}
		}
	}
	for _, cmd := range bp.Cmds {
		pos := cmd.At
		if pos.File == "" {
			pos = at
		}
		if err := p.alterCmd(t, cmd, pos); err != nil {
			return err
		}
	}
	return nil
}

func (p *Planner) dropRelations(table, column string) []Stmt {
	q := "DELETE FROM script_relations WHERE table_name = " + quoteString(p.relKey(table))
	if column != "" {
		q += " AND column_name = " + quoteString(column)
	}
	return []Stmt{{SQL: q, Meta: true}}
}

func (p *Planner) alterCmd(t *STable, cmd *TableCmd, at Problem) error {
	table := t.Name
	switch cmd.Kind {
	case "index", "unique":
		if err := p.checkCmdCols(t, cmd.Columns); err != nil {
			return err
		}
		n := cmd.Name
		if n == "" {
			n = idxName(cmd.Columns, cmd.Kind)
		} else if err := ValidColumnName(n); err != nil {
			return err
		}
		if t.index(n) != nil {
			return fmt.Errorf("schema.table(%q): มี index %s อยู่แล้ว", table, n)
		}
		q := make([]string, len(cmd.Columns))
		for i, c := range cmd.Columns {
			q[i] = quoteIdent(c)
		}
		kw := "ADD INDEX "
		if cmd.Kind == "unique" {
			kw = "ADD UNIQUE INDEX "
		}
		if err := p.emit(Stmt{SQL: p.alterSQL(table, kw+quoteIdent(n)+" ("+strings.Join(q, ", ")+")"), At: at}); err != nil {
			return err
		}
		t.Indexes = append(t.Indexes, &SIndex{Name: n, Columns: cmd.Columns, Unique: cmd.Kind == "unique"})
	case "dropIndex":
		ix := t.index(cmd.Name)
		if ix == nil || cmd.Name == "PRIMARY" {
			return fmt.Errorf("dropIndex(%q): ไม่พบ index นี้ในตาราง %s", cmd.Name, table)
		}
		if err := p.emit(Stmt{SQL: p.alterSQL(table, "DROP INDEX "+quoteIdent(cmd.Name)), At: at}); err != nil {
			return err
		}
		t.Indexes = removeIndex(t.Indexes, cmd.Name)
	case "dropForeign":
		found := false
		for _, f := range t.FKs {
			if f.Column == cmd.Name || f.Name == cmd.Name {
				found = true
				if err := p.emit(Stmt{SQL: p.alterSQL(table, "DROP FOREIGN KEY "+quoteIdent(f.Name)), At: at}); err != nil {
					return err
				}
			}
		}
		t.FKs = filterFKs(t.FKs, func(f *SFK) bool { return f.Column != cmd.Name && f.Name != cmd.Name })
		for _, r := range t.Relations {
			if r.Column == cmd.Name {
				found = true
				for _, s := range p.dropRelations(table, r.Column) {
					s.At = at
					if err := p.emit(s); err != nil {
						return err
					}
				}
			}
		}
		t.Relations = filterRels(t.Relations, cmd.Name)
		if !found {
			return fmt.Errorf("dropForeign(%q): ไม่พบความสัมพันธ์นี้ในตาราง %s", cmd.Name, table)
		}
	case "dropColumn":
		if t.column(cmd.Name) == nil {
			return fmt.Errorf("dropColumn(%q): ไม่พบคอลัมน์นี้ในตาราง %s", cmd.Name, table)
		}
		if len(t.Columns) == 1 {
			return fmt.Errorf("dropColumn(%q): ลบคอลัมน์สุดท้ายไม่ได้ (ใช้ schema.drop)", cmd.Name)
		}
		p.destructive("drop_column", table, cmd.Name)
		for _, f := range t.FKs {
			if f.Column == cmd.Name {
				if err := p.emit(Stmt{SQL: p.alterSQL(table, "DROP FOREIGN KEY "+quoteIdent(f.Name)), At: at}); err != nil {
					return err
				}
			}
		}
		t.FKs = filterFKs(t.FKs, func(f *SFK) bool { return f.Column != cmd.Name })
		if err := p.emit(Stmt{SQL: p.alterSQL(table, "DROP COLUMN "+quoteIdent(cmd.Name)), Backup: table, At: at}); err != nil {
			return err
		}
		cols := t.Columns[:0]
		for _, c := range t.Columns {
			if c.Name != cmd.Name {
				cols = append(cols, c)
			}
		}
		t.Columns = cols
		var idx []*SIndex
		for _, ix := range t.Indexes {
			var keep []string
			for _, c := range ix.Columns {
				if c != cmd.Name {
					keep = append(keep, c)
				}
			}
			if len(keep) > 0 {
				ix.Columns = keep
				idx = append(idx, ix)
			}
		}
		t.Indexes = idx
		if len(filterRels(t.Relations, cmd.Name)) != len(t.Relations) {
			for _, s := range p.dropRelations(table, cmd.Name) {
				s.At = at
				if err := p.emit(s); err != nil {
					return err
				}
			}
			t.Relations = filterRels(t.Relations, cmd.Name)
		}
	case "renameColumn":
		c := t.column(cmd.From)
		if c == nil {
			return fmt.Errorf("renameColumn(%q): ไม่พบคอลัมน์นี้ในตาราง %s", cmd.From, table)
		}
		if err := ValidColumnName(cmd.To); err != nil {
			return err
		}
		if t.column(cmd.To) != nil {
			return fmt.Errorf("renameColumn: มีคอลัมน์ %s อยู่แล้ว", cmd.To)
		}
		p.destructive("rename_column", table, cmd.From)
		if err := p.emit(Stmt{SQL: p.alterSQL(table, "RENAME COLUMN "+quoteIdent(cmd.From)+" TO "+quoteIdent(cmd.To)), Backup: table, At: at}); err != nil {
			return err
		}
		c.Name = cmd.To
		for _, ix := range t.Indexes {
			for i, cc := range ix.Columns {
				if cc == cmd.From {
					ix.Columns[i] = cmd.To
				}
			}
		}
		for _, f := range t.FKs {
			if f.Column == cmd.From {
				f.Column = cmd.To
			}
		}
		for _, r := range t.Relations {
			if r.Column == cmd.From {
				r.Column = cmd.To
				if err := p.emit(Stmt{SQL: "UPDATE script_relations SET column_name = " + quoteString(cmd.To) + " WHERE table_name = " +
					quoteString(p.relKey(table)) + " AND column_name = " + quoteString(cmd.From), Meta: true, At: at}); err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("คำสั่ง %s ไม่รองรับ", cmd.Kind)
	}
	return nil
}

func removeIndex(list []*SIndex, name string) []*SIndex {
	var out []*SIndex
	for _, i := range list {
		if i.Name != name {
			out = append(out, i)
		}
	}
	return out
}

func filterFKs(list []*SFK, keep func(*SFK) bool) []*SFK {
	var out []*SFK
	for _, f := range list {
		if keep(f) {
			out = append(out, f)
		}
	}
	return out
}

func filterRels(list []*SRel, column string) []*SRel {
	var out []*SRel
	for _, r := range list {
		if r.Column != column {
			out = append(out, r)
		}
	}
	return out
}

// Drop plans schema.drop / schema.dropIfExists.
func (p *Planner) Drop(table string, ifExists bool, at Problem) error {
	if _, ok := p.Snap.Tables[table]; !ok {
		if !ifExists {
			return fmt.Errorf("schema.drop: ไม่พบตาราง %s", table)
		}
		if err := ValidTableName(table); err != nil {
			return err
		}
		return p.emit(Stmt{SQL: "DROP TABLE IF EXISTS " + quoteIdent(p.phys(table)), At: at})
	}
	for _, other := range p.Snap.Tables {
		if other.Name == table {
			continue
		}
		for _, f := range other.FKs {
			if f.RefTable == table {
				return fmt.Errorf("schema.drop(%q): ตาราง %s ยังอ้างถึงตารางนี้ (ลบ foreign key หรือตารางนั้นก่อน)", table, other.Name)
			}
		}
	}
	p.destructive("drop_table", table, "")
	kw := "DROP TABLE "
	if ifExists {
		kw = "DROP TABLE IF EXISTS "
	}
	if err := p.emit(Stmt{SQL: kw + quoteIdent(p.phys(table)), Backup: table, At: at}); err != nil {
		return err
	}
	delete(p.Snap.Tables, table)
	for _, s := range p.dropRelations(table, "") {
		s.At = at
		if err := p.emit(s); err != nil {
			return err
		}
	}
	return nil
}

// Rename plans schema.rename.
func (p *Planner) Rename(from, to string, at Problem) error {
	t, ok := p.Snap.Tables[from]
	if !ok {
		return fmt.Errorf("schema.rename: ไม่พบตาราง %s", from)
	}
	if err := ValidTableName(to); err != nil {
		return err
	}
	if err := p.checkPhys(to); err != nil {
		return err
	}
	if _, ok := p.Snap.Tables[to]; ok {
		return fmt.Errorf("schema.rename: มีตาราง %s อยู่แล้ว", to)
	}
	p.destructive("rename_table", from, "")
	if err := p.emit(Stmt{SQL: "RENAME TABLE " + quoteIdent(p.phys(from)) + " TO " + quoteIdent(p.phys(to)), Backup: from, At: at}); err != nil {
		return err
	}
	delete(p.Snap.Tables, from)
	t.Name = to
	p.Snap.Tables[to] = t
	for _, other := range p.Snap.Tables {
		for _, f := range other.FKs {
			if f.RefTable == from {
				f.RefTable = to
			}
		}
	}
	if len(t.Relations) > 0 {
		return p.emit(Stmt{SQL: "UPDATE script_relations SET table_name = " + quoteString(p.relKey(to)) + " WHERE table_name = " + quoteString(p.relKey(from)), Meta: true, At: at})
	}
	return nil
}

var rawDestructiveRe = regexp.MustCompile(`(?is)^\s*(drop|truncate|alter|rename|delete|update|replace)\b`)

// Raw plans schema.raw (separate database only).
func (p *Planner) Raw(sql string, at Problem) error {
	if p.NoRaw {
		return fmt.Errorf("schema.raw ใช้ไม่ได้ในปลั๊กอิน (ใช้ schema builder)")
	}
	if p.Fallback {
		return fmt.Errorf("schema.raw ใช้ไม่ได้เมื่อยังไม่ได้ตั้งฐานข้อมูลแยก (CODE_DB_DSN)")
	}
	if strings.TrimSpace(sql) == "" {
		return fmt.Errorf("schema.raw: ต้องมีคำสั่ง SQL")
	}
	if rawDestructiveRe.MatchString(sql) {
		p.destructive("raw", "", "")
	}
	if err := p.emit(Stmt{SQL: strings.TrimRight(strings.TrimSpace(sql), ";"), At: at}); err != nil {
		return err
	}
	if p.Exec != nil && p.Reload != nil {
		snap, err := p.Reload()
		if err != nil {
			return err
		}
		p.Snap = snap
	}
	return nil
}

// HasTable answers schema.hasTable from the snapshot.
func (p *Planner) HasTable(name string) bool { _, ok := p.Snap.Tables[name]; return ok }

// HasColumn answers schema.hasColumn from the snapshot.
func (p *Planner) HasColumn(table, col string) bool {
	t, ok := p.Snap.Tables[table]
	return ok && t.column(col) != nil
}

// ---------------------------------------------------------------- destructive type changes

var typeRe = regexp.MustCompile(`^([a-z]+)(?:\((.*)\))?( unsigned)?( zerofill)?$`)

var intRank = map[string]int{"tinyint": 1, "smallint": 2, "mediumint": 3, "int": 4, "bigint": 5}
var textRank = map[string]int{"tinytext": 1, "text": 2, "mediumtext": 3, "longtext": 4}

// Narrower reports whether changing a column from type old to type new may
// lose data (shorter, smaller, different kind).
func Narrower(old, new string) bool {
	old, new = strings.ToLower(strings.TrimSpace(old)), strings.ToLower(strings.TrimSpace(new))
	if old == new {
		return false
	}
	mo, mn := typeRe.FindStringSubmatch(old), typeRe.FindStringSubmatch(new)
	if mo == nil || mn == nil {
		return true
	}
	bo, bn := mo[1], mn[1]
	uo, un := mo[3] != "", mn[3] != ""
	arg := func(s string, i int) int {
		parts := strings.Split(s, ",")
		if i >= len(parts) {
			return 0
		}
		n, _ := strconv.Atoi(strings.TrimSpace(parts[i]))
		return n
	}
	switch {
	case intRank[bo] > 0 && intRank[bn] > 0:
		if bo == "tinyint" && mo[2] == "1" && !(bn == "tinyint" && mn[2] == "1") {
			return false // boolean → wider integer
		}
		return uo != un || intRank[bn] < intRank[bo]
	case (bo == "varchar" || bo == "char") && (bn == "varchar" || bn == "char"):
		return arg(mn[2], 0) < arg(mo[2], 0)
	case (bo == "varchar" || bo == "char") && textRank[bn] > 0:
		return false
	case textRank[bo] > 0 && textRank[bn] > 0:
		return textRank[bn] < textRank[bo]
	case bo == "decimal" && bn == "decimal":
		po, so, pn, sn := arg(mo[2], 0), arg(mo[2], 1), arg(mn[2], 0), arg(mn[2], 1)
		return sn < so || pn-sn < po-so || (uo != un && !un)
	case bo == "enum" && bn == "enum":
		have := map[string]bool{}
		for _, v := range splitEnum(mn[2]) {
			have[v] = true
		}
		for _, v := range splitEnum(mo[2]) {
			if !have[v] {
				return true
			}
		}
		return false
	case bo == "datetime" && bn == "datetime":
		return arg(mn[2], 0) < arg(mo[2], 0)
	case bo == "date" && bn == "datetime":
		return false
	}
	return true
}

func splitEnum(s string) []string {
	var out []string
	for _, part := range strings.Split(s, "','") {
		out = append(out, strings.Trim(part, "'"))
	}
	return out
}
