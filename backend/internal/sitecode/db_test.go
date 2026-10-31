package sitecode

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

// ---------------------------------------------------------------- identifiers

func TestCodeIdentifiers(t *testing.T) {
	for _, ok := range []string{"orders", "order_items", "a", "t2", strings.Repeat("a", 63)} {
		if err := ValidTableName(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Orders", "1x", "_x", "x-y", "x y", "x;drop", strings.Repeat("a", 64), "core_users", "script_x",
		"sc_orders", "bak_orders_1", "kv_docs", "orders`", "o.r"} {
		if err := ValidTableName(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if ValidColumnName("core_x") != nil || ValidColumnName("Name") == nil {
		t.Error("column rules")
	}
	for in, want := range map[string]string{"id": "`id`", "o.id": "`o`.`id`", "*": "*", "o.*": "`o`.*", "Total_2": "`Total_2`"} {
		if got, err := ColumnRef(in); err != nil || got != want {
			t.Errorf("ColumnRef(%q) = %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"a.b.c", "id;", "id)", "a b", "1=1", "`x`", "a.*.b", "*.a", ""} {
		if _, err := ColumnRef(bad); err == nil {
			t.Errorf("ColumnRef(%q) accepted", bad)
		}
	}
	n := Naming{}
	for _, bad := range []string{"script_migrations", "bak_x", "sc_x", "core_nope", "a.b", "users; drop", "x as", "a as b c"} {
		if _, err := n.ParseTable(bad); err == nil {
			t.Errorf("ParseTable(%q) accepted", bad)
		}
	}
	if _, err := (Naming{Prefix: "sc_", Fallback: true}).ParseTable("core_users"); err == nil {
		t.Error("core_* allowed in fallback mode")
	}
	if r, err := n.ParseTable("orders AS o"); err != nil || r.name != "orders" || r.alias != "o" {
		t.Errorf("alias: %v %v", r, err)
	}
}

// ---------------------------------------------------------------- schema builder DDL

func col(name, kind string, mods ...func(*ColumnDef)) *ColumnDef {
	c := &ColumnDef{Name: name, Kind: kind}
	switch kind {
	case "string":
		c.Length = 255
	case "decimal":
		c.Precision, c.Scale = 12, 2
	case "id":
		c.Unsigned, c.AutoIncrement, c.Primary = true, true, true
	case "foreignId":
		c.Unsigned = true
	}
	for _, m := range mods {
		m(c)
	}
	return c
}

func strp(s string) *string { return &s }

func TestCodeSchemaColumnDDL(t *testing.T) {
	cases := []struct {
		c    *ColumnDef
		want string
	}{
		{col("id", "id"), "`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT"},
		{col("name", "string"), "`name` VARCHAR(255) NOT NULL"},
		{col("code", "string", func(c *ColumnDef) { c.Length = 20; c.Nullable = true }), "`code` VARCHAR(20) NULL"},
		{col("body", "text"), "`body` TEXT NOT NULL"},
		{col("qty", "integer", func(c *ColumnDef) { c.Unsigned = true; c.Default = strp("0") }), "`qty` INT UNSIGNED NOT NULL DEFAULT 0"},
		{col("big", "bigInteger"), "`big` BIGINT NOT NULL"},
		{col("total", "decimal"), "`total` DECIMAL(12,2) NOT NULL"},
		{col("price", "decimal", func(c *ColumnDef) { c.Precision, c.Scale = 8, 3 }), "`price` DECIMAL(8,3) NOT NULL"},
		{col("ok", "boolean", func(c *ColumnDef) { c.Default = strp("1") }), "`ok` TINYINT(1) NOT NULL DEFAULT 1"},
		{col("day", "date"), "`day` DATE NOT NULL"},
		{col("at", "datetime", func(c *ColumnDef) { c.Nullable = true }), "`at` DATETIME(3) NULL"},
		{col("at2", "datetime", func(c *ColumnDef) { c.UseCurrent = true }), "`at2` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)"},
		{col("meta", "json", func(c *ColumnDef) { c.Default = strp("'{}'") }), "`meta` JSON NOT NULL DEFAULT ('{}')"},
		{col("note", "text", func(c *ColumnDef) { c.Nullable = true; c.Default = strp("NULL") }), "`note` TEXT NULL DEFAULT NULL"},
		{col("status", "enum", func(c *ColumnDef) { c.Values = []string{"Pending", "it's"}; c.Default = strp("'Pending'") }), "`status` ENUM('Pending','it''s') NOT NULL DEFAULT 'Pending'"},
		{col("uid", "uuid"), "`uid` CHAR(36) NOT NULL"},
		{col("user_id", "foreignId"), "`user_id` BIGINT UNSIGNED NOT NULL"},
		{col("updated_at", "timestamp", func(c *ColumnDef) { c.UseCurrent, c.OnUpdate = true, true }), "`updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)"},
		{col("c", "string", func(c *ColumnDef) { c.Comment = strp("ชื่อ 'x'") }), "`c` VARCHAR(255) NOT NULL COMMENT 'ชื่อ ''x'''"},
	}
	for _, tc := range cases {
		if err := tc.c.Check(); err != nil {
			t.Errorf("%s: check: %v", tc.c.Name, err)
		}
		if got := tc.c.Definition(); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.c.Name, got, tc.want)
		}
	}
	for name, c := range map[string]*ColumnDef{
		"len 0":            col("a", "string", func(c *ColumnDef) { c.Length = 0 }),
		"decimal scale":    col("a", "decimal", func(c *ColumnDef) { c.Precision, c.Scale = 2, 5 }),
		"enum empty":       col("a", "enum"),
		"enum dup":         col("a", "enum", func(c *ColumnDef) { c.Values = []string{"x", "X"} }),
		"unsigned text":    col("a", "text", func(c *ColumnDef) { c.Unsigned = true }),
		"null not null":    col("a", "string", func(c *ColumnDef) { c.Default = strp("NULL") }),
		"set null":         col("a", "foreignId", func(c *ColumnDef) { c.Ref = &Reference{Table: "x", OnDelete: "set null"} }),
		"bad onDelete":     col("a", "foreignId", func(c *ColumnDef) { c.Ref = &Reference{Table: "x", OnDelete: "drop"} }),
		"useCurrent int":   col("a", "integer", func(c *ColumnDef) { c.UseCurrent = true }),
		"bad column name":  col("A", "integer"),
		"injected comment": col("a;drop", "integer"),
	} {
		if err := c.Check(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for v, want := range map[any]string{nil: "NULL", true: "1", false: "0", int64(5): "5", 1.5: "1.5", "a'b\\": `'a''b\\'`} {
		if got, err := LiteralDefault(v); err != nil || got != want {
			t.Errorf("LiteralDefault(%v) = %q %v", v, got, err)
		}
	}
}

func ordersPlanner(prefix string) *Planner {
	return &Planner{Snap: NewSnapshot(), Prefix: prefix, Fallback: prefix != "", Migration: "m"}
}

func createOrders(t *testing.T, p *Planner) {
	t.Helper()
	if err := p.Create(&Blueprint{Table: "customers", Create: true, Columns: []*ColumnDef{col("id", "id"), col("name", "string")}}, Problem{}); err != nil {
		t.Fatal(err)
	}
	bp := &Blueprint{Table: "orders", Create: true, Columns: []*ColumnDef{
		col("id", "id"),
		col("code", "string", func(c *ColumnDef) { c.Length = 20; c.Unique = true }),
		col("customer_id", "foreignId", func(c *ColumnDef) { c.Ref = &Reference{Table: "customers", OnDelete: "cascade"} }),
		col("user_id", "foreignId", func(c *ColumnDef) { c.Nullable = true; c.Ref = &Reference{Table: "core_users"} }),
		col("status", "enum", func(c *ColumnDef) {
			c.Values = []string{"pending", "paid"}
			c.Default = strp("'pending'")
			c.Index = true
		}),
		col("total", "decimal"),
		col("created_at", "timestamp", func(c *ColumnDef) { c.UseCurrent = true }),
	}, Cmds: []*TableCmd{{Kind: "index", Columns: []string{"status", "created_at"}}}}
	if err := p.Create(bp, Problem{}); err != nil {
		t.Fatal(err)
	}
}

func TestCodeSchemaCreateAndAlter(t *testing.T) {
	p := ordersPlanner("")
	createOrders(t, p)
	sql := p.SQL()
	if len(sql) != 2 {
		t.Fatalf("statements: %v", sql)
	}
	want := "CREATE TABLE `orders` (\n" +
		"  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,\n" +
		"  `code` VARCHAR(20) NOT NULL,\n" +
		"  `customer_id` BIGINT UNSIGNED NOT NULL,\n" +
		"  `user_id` BIGINT UNSIGNED NULL,\n" +
		"  `status` ENUM('pending','paid') NOT NULL DEFAULT 'pending',\n" +
		"  `total` DECIMAL(12,2) NOT NULL,\n" +
		"  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  UNIQUE KEY `code_unique` (`code`),\n" +
		"  KEY `status_index` (`status`),\n" +
		"  CONSTRAINT `fk_orders_customer_id` FOREIGN KEY (`customer_id`) REFERENCES `customers` (`id`) ON DELETE CASCADE,\n" +
		"  KEY `status_created_at_index` (`status`, `created_at`)\n" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"
	if sql[1] != want {
		t.Fatalf("create:\n%s\nwant\n%s", sql[1], want)
	}
	// the core_users reference is metadata only
	var meta []string
	for _, s := range p.Stmts {
		if s.Meta {
			meta = append(meta, s.SQL)
		}
	}
	if len(meta) != 1 || !strings.Contains(meta[0], "script_relations") || !strings.Contains(meta[0], "'core_users'") {
		t.Fatalf("relation meta: %v", meta)
	}
	o := p.Snap.Tables["orders"]
	if len(o.Columns) != 7 || len(o.FKs) != 1 || len(o.Relations) != 1 || o.column("total").Type != "decimal(12,2)" || o.column("id").Key != "PRI" {
		t.Fatalf("snapshot: %+v", o)
	}
	if len(p.Destructive) != 0 {
		t.Fatalf("create flagged destructive: %v", p.Destructive)
	}
	if !p.HasTable("orders") || !p.HasColumn("orders", "code") || p.HasColumn("orders", "nope") {
		t.Fatal("hasTable/hasColumn")
	}

	// alter: add, change (wider = safe, narrower = destructive), indexes
	p.Stmts = nil
	err := p.Alter(&Blueprint{Table: "orders", Columns: []*ColumnDef{
		col("note", "text", func(c *ColumnDef) { c.Nullable = true; c.After = "code" }),
		col("code", "string", func(c *ColumnDef) { c.Length = 50; c.Change = true }),
		col("shipped", "boolean", func(c *ColumnDef) { c.Default = strp("0"); c.Index = true }),
	}, Cmds: []*TableCmd{{Kind: "unique", Columns: []string{"customer_id", "code"}, Name: "cust_code"}, {Kind: "dropIndex", Name: "status_index"}}}, Problem{})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(p.SQL(), "\n")
	want = "ALTER TABLE `orders` ADD COLUMN `note` TEXT NULL AFTER `code`\n" +
		"ALTER TABLE `orders` MODIFY COLUMN `code` VARCHAR(50) NOT NULL\n" +
		"ALTER TABLE `orders` ADD COLUMN `shipped` TINYINT(1) NOT NULL DEFAULT 0\n" +
		"ALTER TABLE `orders` ADD INDEX `shipped_index` (`shipped`)\n" +
		"ALTER TABLE `orders` ADD UNIQUE INDEX `cust_code` (`customer_id`, `code`)\n" +
		"ALTER TABLE `orders` DROP INDEX `status_index`"
	if got != want {
		t.Fatalf("alter:\n%s\nwant\n%s", got, want)
	}
	if len(p.Destructive) != 0 || p.Snap.Tables["orders"].Columns[2].Name != "note" {
		t.Fatalf("widening flagged / order: %v %v", p.Destructive, p.Snap.Tables["orders"].Columns[2].Name)
	}
	// destructive ones
	p.Stmts = nil
	err = p.Alter(&Blueprint{Table: "orders", Columns: []*ColumnDef{col("code", "string", func(c *ColumnDef) { c.Length = 10; c.Change = true })},
		Cmds: []*TableCmd{{Kind: "dropColumn", Name: "customer_id"}, {Kind: "renameColumn", From: "note", To: "remark"}}}, Problem{})
	if err != nil {
		t.Fatal(err)
	}
	got = strings.Join(p.SQL(), "\n")
	want = "ALTER TABLE `orders` MODIFY COLUMN `code` VARCHAR(10) NOT NULL\n" +
		"ALTER TABLE `orders` DROP FOREIGN KEY `fk_orders_customer_id`\n" +
		"ALTER TABLE `orders` DROP COLUMN `customer_id`\n" +
		"ALTER TABLE `orders` RENAME COLUMN `note` TO `remark`"
	if got != want {
		t.Fatalf("destructive alter:\n%s\nwant\n%s", got, want)
	}
	if fmt.Sprint(p.Destructive) != "[{m change_column orders code} {m drop_column orders customer_id} {m rename_column orders note}]" {
		t.Fatalf("destructive list: %v", p.Destructive)
	}
	for _, s := range p.Stmts {
		if strings.Contains(s.SQL, "MODIFY") || strings.Contains(s.SQL, "DROP COLUMN") || strings.Contains(s.SQL, "RENAME COLUMN") {
			if s.Backup != "orders" {
				t.Errorf("no backup for %s", s.SQL)
			}
		}
	}
	// rename + drop
	p.Stmts, p.Destructive = nil, nil
	if err := p.Rename("orders", "purchases", Problem{}); err != nil {
		t.Fatal(err)
	}
	if err := p.Drop("customers", false, Problem{}); err != nil {
		t.Fatal(err)
	}
	if err := p.Drop("ghost", true, Problem{}); err != nil {
		t.Fatal(err)
	}
	got = strings.Join(p.SQL(), "\n")
	if got != "RENAME TABLE `orders` TO `purchases`\nDROP TABLE `customers`\nDROP TABLE IF EXISTS `ghost`" {
		t.Fatalf("rename/drop: %s", got)
	}
	if len(p.Destructive) != 2 || p.Destructive[0].Op != "rename_table" || p.Destructive[1].Op != "drop_table" {
		t.Fatalf("destructive: %v", p.Destructive)
	}
}

func TestCodeSchemaErrors(t *testing.T) {
	p := ordersPlanner("")
	createOrders(t, p)
	for name, fn := range map[string]func() error{
		"create existing": func() error {
			return p.Create(&Blueprint{Table: "orders", Columns: []*ColumnDef{col("id", "id")}}, Problem{})
		},
		"create reserved": func() error {
			return p.Create(&Blueprint{Table: "core_x", Columns: []*ColumnDef{col("id", "id")}}, Problem{})
		},
		"no columns": func() error { return p.Create(&Blueprint{Table: "empty"}, Problem{}) },
		"dup column": func() error {
			return p.Create(&Blueprint{Table: "d", Columns: []*ColumnDef{col("a", "integer"), col("a", "text")}}, Problem{})
		},
		"missing ref": func() error {
			return p.Create(&Blueprint{Table: "x", Columns: []*ColumnDef{col("y", "foreignId", func(c *ColumnDef) { c.Ref = &Reference{Table: "nope"} })}}, Problem{})
		},
		"bad core ref": func() error {
			return p.Create(&Blueprint{Table: "x", Columns: []*ColumnDef{col("y", "foreignId", func(c *ColumnDef) { c.Ref = &Reference{Table: "core_secrets"} })}}, Problem{})
		},
		"alter missing": func() error { return p.Alter(&Blueprint{Table: "nope"}, Problem{}) },
		"add existing": func() error {
			return p.Alter(&Blueprint{Table: "orders", Columns: []*ColumnDef{col("code", "text")}}, Problem{})
		},
		"change missing": func() error {
			return p.Alter(&Blueprint{Table: "orders", Columns: []*ColumnDef{col("zz", "text", func(c *ColumnDef) { c.Change = true })}}, Problem{})
		},
		"drop col missing": func() error {
			return p.Alter(&Blueprint{Table: "orders", Cmds: []*TableCmd{{Kind: "dropColumn", Name: "zz"}}}, Problem{})
		},
		"drop idx missing": func() error {
			return p.Alter(&Blueprint{Table: "orders", Cmds: []*TableCmd{{Kind: "dropIndex", Name: "zz"}}}, Problem{})
		},
		"drop referenced": func() error { return p.Drop("customers", false, Problem{}) },
		"drop missing":    func() error { return p.Drop("nope", false, Problem{}) },
		"rename to used":  func() error { return p.Rename("orders", "customers", Problem{}) },
		"raw empty":       func() error { return p.Raw("  ", Problem{}) },
		"after missing": func() error {
			return p.Alter(&Blueprint{Table: "orders", Columns: []*ColumnDef{col("q", "integer", func(c *ColumnDef) { c.After = "nope" })}}, Problem{})
		},
	} {
		if err := fn(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	fb := ordersPlanner("sc_")
	if err := fb.Raw("CREATE TABLE x (a int)", Problem{}); err == nil {
		t.Error("schema.raw allowed in fallback mode")
	}
	p.Stmts, p.Destructive = nil, nil
	if err := p.Raw("DROP TABLE orders;", Problem{}); err != nil || len(p.Destructive) != 1 || p.Destructive[0].Op != "raw" || p.SQL()[0] != "DROP TABLE orders" {
		t.Fatalf("raw: %v %v %v", err, p.Destructive, p.SQL())
	}
	if err := p.Raw("CREATE INDEX i ON orders (code)", Problem{}); err != nil || len(p.Destructive) != 1 {
		t.Fatalf("raw create flagged: %v", p.Destructive)
	}
}

func TestCodeFallbackPrefix(t *testing.T) {
	p := ordersPlanner("sc_")
	createOrders(t, p)
	sql := strings.Join(p.SQL(), "\n")
	if !strings.Contains(sql, "CREATE TABLE `sc_customers`") || !strings.Contains(sql, "CREATE TABLE `sc_orders`") ||
		!strings.Contains(sql, "REFERENCES `sc_customers` (`id`)") || !strings.Contains(sql, "`fk_sc_orders_customer_id`") {
		t.Fatalf("fallback DDL: %s", sql)
	}
	if _, ok := p.Snap.Tables["orders"]; !ok {
		t.Fatal("snapshot keys must stay logical")
	}
	p.Stmts = nil
	_ = p.Rename("orders", "o2", Problem{})
	if p.SQL()[0] != "RENAME TABLE `sc_orders` TO `sc_o2`" {
		t.Fatalf("rename: %v", p.SQL())
	}
	n := Naming{Prefix: "sc_", Fallback: true}
	q, err := NewQuery(n, "orders")
	if err != nil {
		t.Fatal(err)
	}
	q.Where(false, mc(Compare("orders.status", "=", "paid")))
	_ = q.Join("LEFT", "customers as c", []cond{mc(OnCols("c.id", "=", "orders.customer_id"))})
	s, args := q.SelectSQL(0)
	if s != "SELECT * FROM `sc_orders` AS `orders` LEFT JOIN `sc_customers` AS `c` ON `c`.`id` = `orders`.`customer_id` WHERE `orders`.`status` = ?" || fmt.Sprint(args) != "[paid]" {
		t.Fatalf("fallback select: %s %v", s, args)
	}
	qi, _ := NewQuery(n, "orders")
	ins, _, _ := qi.InsertSQL([][]KV{{{"code", "A"}}})
	if ins != "INSERT INTO `sc_orders` (`code`) VALUES (?)" {
		t.Fatalf("fallback insert: %s", ins)
	}
	q2, _ := NewQuery(n, "orders")
	q2.Where(false, mc(Compare("id", "=", int64(1))))
	del, _, _ := q2.DeleteSQL()
	if del != "DELETE FROM `sc_orders` AS `orders` WHERE `id` = ?" {
		t.Fatalf("fallback delete: %s", del)
	}
	if n.Phys("kv_docs") != "sc_kv_docs" {
		t.Fatal("kv_docs prefix")
	}
	// long names keep under 64 characters
	if len(p.fkName(strings.Repeat("a", 63), strings.Repeat("b", 63))) > 64 {
		t.Fatal("fk name too long")
	}
}

func TestCodeNarrower(t *testing.T) {
	for _, c := range []struct {
		old, new string
		want     bool
	}{
		{"varchar(20)", "varchar(50)", false}, {"varchar(50)", "varchar(20)", true}, {"varchar(255)", "text", false},
		{"text", "varchar(255)", true}, {"int", "bigint", false}, {"bigint", "int", true}, {"int", "int unsigned", true},
		{"decimal(12,2)", "decimal(14,2)", false}, {"decimal(12,2)", "decimal(12,4)", true}, {"decimal(12,2)", "decimal(10,2)", true},
		{"enum('a','b')", "enum('a','b','c')", false}, {"enum('a','b')", "enum('a')", true}, {"datetime", "datetime(3)", false},
		{"datetime(3)", "datetime", true}, {"date", "datetime(3)", false}, {"json", "text", true}, {"tinyint(1)", "int", false},
		{"varchar(10)", "int", true}, {"text", "mediumtext", false}, {"longtext", "text", true}, {"char(36)", "varchar(36)", false},
	} {
		if got := Narrower(c.old, c.new); got != c.want {
			t.Errorf("Narrower(%s, %s) = %v", c.old, c.new, got)
		}
	}
}

// ---------------------------------------------------------------- dry run against a snapshot (JS)

func TestCodeSnapshotDryRun(t *testing.T) {
	e := testEngine()
	files := map[string]string{
		"main.ts": "export {}\n",
		"migrations/001_create.ts": `export default migration({
  up(schema) {
    schema.create('customers', (t) => { t.id(); t.string('name', 100); t.timestamps() })
    schema.create('orders', (t) => {
      t.id()
      t.foreignId('customer_id').references('customers').onDelete('restrict')
      t.foreignId('user_id').nullable().references('core_users')
      t.decimal('total', 10, 2).default(0)
      t.json('meta').nullable()
      t.index(['customer_id', 'total'])
    })
    if (!schema.hasTable('orders')) throw new Error('hasTable')
  },
  down(schema) { schema.drop('orders'); schema.drop('customers') },
})`,
		"migrations/002_alter.ts": `import { label } from '../lib/x'
export default migration({
  up(schema) {
    schema.table('orders', (t) => {
      t.string('note', 50).nullable().after('total').comment(label)
      t.change(t.decimal('total', 8, 2))
      t.dropColumn('meta')
    })
    if (!schema.hasColumn('orders', 'note')) throw new Error('hasColumn')
  },
})`,
		"lib/x.ts": "export const label = 'note'\n",
	}
	list, probs := MigrationFiles(files)
	if len(probs) > 0 || len(list) != 2 || list[0].Name != "001_create" {
		t.Fatalf("files: %v %v", list, probs)
	}
	snap := NewSnapshot()
	var all []string
	var destructive []Destructive
	for _, mf := range list {
		v, def, ps := e.loadMigration(files, mf)
		if len(ps) > 0 {
			t.Fatalf("%s: %v", mf.Name, ps)
		}
		p := &Planner{Snap: snap, Migration: mf.Name}
		up, _ := asFunc(def.Get("up"))
		if err := v.invoke(up, time.Second, v.schemaObject(p)); err != nil {
			t.Fatalf("%s: %v", mf.Name, err)
		}
		snap = p.Snap
		all = append(all, p.SQL()...)
		destructive = append(destructive, p.Destructive...)
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{"CREATE TABLE `customers`", "`created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)",
		"CONSTRAINT `fk_orders_customer_id` FOREIGN KEY (`customer_id`) REFERENCES `customers` (`id`) ON DELETE RESTRICT",
		"`total` DECIMAL(10,2) NOT NULL DEFAULT 0", "ADD COLUMN `note` VARCHAR(50) NULL COMMENT 'note' AFTER `total`",
		"MODIFY COLUMN `total` DECIMAL(8,2) NOT NULL", "DROP COLUMN `meta`"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "core_users") {
		t.Errorf("FK to a view emitted: %s", joined)
	}
	if len(destructive) != 2 || destructive[0].Migration != "002_alter" || destructive[0].Op != "change_column" || destructive[1].Op != "drop_column" {
		t.Fatalf("destructive: %v", destructive)
	}

	// builder errors carry the migration file:line
	files["migrations/003_bad.ts"] = "export default migration({\n  up(schema) {\n    schema.create('orders', (t) => { t.id() })\n  },\n})\n"
	v, def, ps := e.loadMigration(files, migFile{Name: "003_bad", Path: "migrations/003_bad.ts"})
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	up, _ := asFunc(def.Get("up"))
	err := v.invoke(up, time.Second, v.schemaObject(&Planner{Snap: snap, Migration: "003_bad"}))
	re := toRunError(err)
	if err == nil || re.At.File != "migrations/003_bad.ts" || re.At.Line != 3 || !strings.Contains(re.Message, "orders") {
		t.Fatalf("error position: %v %+v", err, re)
	}
	// a file without export default
	files["migrations/004_x.ts"] = "export const x = 1\n"
	if _, _, ps := e.loadMigration(files, migFile{Name: "004_x", Path: "migrations/004_x.ts"}); len(ps) == 0 {
		t.Fatal("missing default export accepted")
	}
	// duplicate names
	if _, probs := MigrationFiles(map[string]string{"migrations/1_a.ts": "", "migrations/1_a.js": ""}); len(probs) != 1 {
		t.Fatal("duplicate migration names accepted")
	}
}

// ---------------------------------------------------------------- query builder

func mc(c cond, err error) cond {
	if err != nil {
		panic(err)
	}
	return c
}

func TestCodeQueryBuilderSQL(t *testing.T) {
	n := Naming{}
	q, err := NewQuery(n, "orders as o")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Select("o.id", "o.code as c", &Expr{SQL: "SUM(i.qty) AS items"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Join("INNER", "order_items as i", []cond{mc(OnCols("i.order_id", "=", "o.id"))}); err != nil {
		t.Fatal(err)
	}
	if err := q.Join("LEFT", "core_users as u", []cond{mc(OnCols("u.id", "=", "o.user_id"))}); err != nil {
		t.Fatal(err)
	}
	q.Where(false, mc(Compare("o.status", "=", "paid")))
	q.Where(false, mc(Compare("o.total", ">", int64(100))))
	_ = q.WhereIn(false, "o.id", []any{int64(1), int64(2)}, false)
	_ = q.WhereBetween(false, "o.created_at", "2026-01-01", "2026-12-31", false)
	_ = q.WhereNull(false, "o.deleted_at", false)
	sub := q.SubQuery()
	sub.Where(false, mc(Compare("o.code", "like", "A%")))
	sub.Where(true, mc(Compare("o.code", "=", nil)))
	q.WhereGroup(true, sub)
	_ = q.GroupBy("o.id")
	q.Having(false, mc(Compare("items", ">", int64(1))))
	_ = q.OrderBy("o.created_at", "desc")
	_ = q.Limit(10)
	_ = q.Offset(20)
	s, args := q.SelectSQL(4000)
	want := "SELECT /*+ MAX_EXECUTION_TIME(4000) */ `o`.`id`, `o`.`code` AS `c`, SUM(i.qty) AS items FROM `orders` AS `o` " +
		"INNER JOIN `order_items` AS `i` ON `i`.`order_id` = `o`.`id` LEFT JOIN `core_users` AS `u` ON `u`.`id` = `o`.`user_id` " +
		"WHERE `o`.`status` = ? AND `o`.`total` > ? AND `o`.`id` IN (?, ?) AND `o`.`created_at` BETWEEN ? AND ? AND `o`.`deleted_at` IS NULL " +
		"OR (`o`.`code` LIKE ? OR `o`.`code` IS NULL) GROUP BY `o`.`id` HAVING `items` > ? ORDER BY `o`.`created_at` DESC LIMIT 10 OFFSET 20"
	if s != want {
		t.Fatalf("select:\n%s\nwant\n%s", s, want)
	}
	if fmt.Sprint(args) != "[paid 100 1 2 2026-01-01 2026-12-31 A% 1]" {
		t.Fatalf("args: %v", args)
	}
	// paginate / count on a grouped query wrap a subquery
	cs, cargs := q.CountSQL(0)
	if !strings.HasPrefix(cs, "SELECT COUNT(*) AS aggregate FROM (SELECT `o`.`id`") || strings.Contains(cs, "LIMIT") || len(cargs) != len(args) {
		t.Fatalf("grouped count: %s %v", cs, cargs)
	}
	ps, _ := q.PageSQL(0, 3, 25)
	if !strings.HasSuffix(ps, "LIMIT 25 OFFSET 50") {
		t.Fatalf("page: %s", ps)
	}
	// aggregates
	q2, _ := NewQuery(n, "orders")
	q2.Where(false, mc(Compare("status", "!=", "void")))
	if s, a := q2.CountSQL(100); s != "SELECT /*+ MAX_EXECUTION_TIME(100) */ COUNT(*) AS aggregate FROM `orders` WHERE `status` <> ?" || len(a) != 1 {
		t.Fatalf("count: %s", s)
	}
	for fn, want := range map[string]string{"SUM": "SELECT SUM(`total`) AS aggregate FROM `orders` WHERE `status` <> ?",
		"MAX": "SELECT MAX(`total`) AS aggregate FROM `orders` WHERE `status` <> ?"} {
		if s, _, err := q2.AggregateSQL(0, fn, "total"); err != nil || s != want {
			t.Errorf("%s: %s %v", fn, s, err)
		}
	}
	if _, _, err := q2.AggregateSQL(0, "SUM", "*"); err == nil {
		t.Error("sum(*) accepted")
	}
	// insert / update / delete / increment
	ins, iargs, err := q2.InsertSQL([][]KV{{{"code", "A"}, {"total", 5.5}}, {{"code", "B"}, {"note", nil}}})
	if err != nil || ins != "INSERT INTO `orders` (`code`, `total`, `note`) VALUES (?, ?, DEFAULT), (?, DEFAULT, ?)" || fmt.Sprint(iargs) != "[A 5.5 B <nil>]" {
		t.Fatalf("insert: %s %v %v", ins, iargs, err)
	}
	up, uargs, err := q2.UpdateSQL([]KV{{"code", "Z"}, {"n", &Expr{SQL: "n + ?", Args: []any{int64(2)}}}})
	if err != nil || up != "UPDATE `orders` SET `code` = ?, `n` = n + ? WHERE `status` <> ?" || fmt.Sprint(uargs) != "[Z 2 void]" {
		t.Fatalf("update: %s %v %v", up, uargs, err)
	}
	inc, incArgs, err := q2.IncrementSQL("stock", -3, []KV{{"touched", true}})
	if err != nil || inc != "UPDATE `orders` SET `stock` = `stock` + ?, `touched` = ? WHERE `status` <> ?" || fmt.Sprint(incArgs) != "[-3 true void]" {
		t.Fatalf("increment: %s %v %v", inc, incArgs, err)
	}
	del, _, err := q2.DeleteSQL()
	if err != nil || del != "DELETE FROM `orders` WHERE `status` <> ?" {
		t.Fatalf("delete: %s %v", del, err)
	}
	// refuse unsafe update/delete, writes to views, joins in writes
	q3, _ := NewQuery(n, "orders")
	if _, _, err := q3.UpdateSQL([]KV{{"a", int64(1)}}); err == nil {
		t.Error("update without where accepted")
	}
	if _, _, err := q3.DeleteSQL(); err == nil {
		t.Error("delete without where accepted")
	}
	q3.All()
	if s, _, err := q3.DeleteSQL(); err != nil || s != "DELETE FROM `orders`" {
		t.Errorf("delete all: %s %v", s, err)
	}
	q4, _ := NewQuery(n, "core_users")
	q4.All()
	if _, _, err := q4.DeleteSQL(); err == nil {
		t.Error("delete from a core view accepted")
	}
	if _, _, err := q4.InsertSQL([][]KV{{{"name", "x"}}}); err == nil {
		t.Error("insert into a core view accepted")
	}
	if _, _, err := q2.InsertSQL([][]KV{{{"bad col", "x"}}}); err == nil {
		t.Error("bad insert column accepted")
	}
	// operators and edge cases
	if _, err := Compare("a", "; drop", 1); err == nil {
		t.Error("bad operator accepted")
	}
	if c, _ := Compare("a", "in", []any{}); c.sql != "1 = 0" {
		t.Errorf("empty in: %s", c.sql)
	}
	if c, _ := Compare("a", "not in", []any{int64(1)}); c.sql != "`a` NOT IN (?)" {
		t.Errorf("not in: %s", c.sql)
	}
	if c, _ := Compare("a", "!=", nil); c.sql != "`a` IS NOT NULL" {
		t.Errorf("!= null: %s", c.sql)
	}
	if _, err := Compare("a", ">", nil); err == nil {
		t.Error("> null accepted")
	}
	if _, err := Compare("a", "=", []any{1}); err == nil {
		t.Error("array with = accepted")
	}
	if err := q2.OrderBy("x", "sideways"); err == nil {
		t.Error("bad direction accepted")
	}
	if _, err := OnCols("a", "like", "b"); err == nil {
		t.Error("join like accepted")
	}
	if err := q2.Select("a; drop"); err == nil {
		t.Error("bad select accepted")
	}
}

// ---------------------------------------------------------------- result mapping

func TestCodeResultMapping(t *testing.T) {
	ts := time.Date(2026, 10, 24, 10, 30, 0, 123e6, time.UTC)
	for _, c := range []struct {
		typ  string
		in   any
		want any
	}{
		{"DECIMAL", []byte("12.50"), "12.50"},
		{"DATETIME", ts, "2026-10-24T10:30:00.123Z"},
		{"TIMESTAMP", ts, "2026-10-24T10:30:00.123Z"},
		{"DATE", ts, "2026-10-24"},
		{"DATETIME", []byte("2026-10-24 10:30:00.123"), "2026-10-24T10:30:00.123Z"},
		{"JSON", []byte(`{"a":[1]}`), JSONText(`{"a":[1]}`)},
		{"BIGINT", int64(42), int64(42)},
		{"BIGINT", int64(1) << 60, "1152921504606846976"},
		{"UNSIGNED BIGINT", uint64(1) << 63, "9223372036854775808"},
		{"BIGINT", []byte("9007199254740993"), "9007199254740993"},
		{"INT", []byte("7"), int64(7)},
		{"TINYINT", int64(1), true},
		{"TINYINT", []byte("0"), false},
		{"UNSIGNED TINYINT", int64(3), int64(3)},
		{"VARCHAR", []byte("héllo"), "héllo"},
		{"DOUBLE", float64(1.5), 1.5},
		{"VARCHAR", nil, nil},
	} {
		if got := mapValue(c.typ, c.in); fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", c.want) {
			t.Errorf("%s %v: %#v want %#v", c.typ, c.in, got, c.want)
		}
	}
	b, _ := json.Marshal(Row{Cols: []string{"z", "a", "j", "z"}, Vals: []any{1, "x", JSONText(`{"k":true}`), 2}})
	if string(b) != `{"a":"x","j":{"k":true},"z":2}` {
		t.Fatalf("row json: %s", b)
	}
	if tm, ok := BindValue("2026-10-24T10:30:00.123Z").(time.Time); !ok || !tm.Equal(ts) {
		t.Fatalf("ISO bind: %v", BindValue("2026-10-24T10:30:00.123Z"))
	}
	if BindValue("2026-10-24") != "2026-10-24" || BindValue("hello") != "hello" {
		t.Fatal("plain strings must stay")
	}
	if sqlLiteral("VARCHAR", []byte("it's\n\\")) != `'it''s\n\\'` || sqlLiteral("DECIMAL", []byte("1.50")) != "1.50" ||
		sqlLiteral("BLOB", []byte{0, 1}) != "X'0001'" || sqlLiteral("INT", nil) != "NULL" || sqlLiteral("DATE", ts) != "'2026-10-24'" {
		t.Fatal("sql literals")
	}
}

func TestCodeReadSQLGuard(t *testing.T) {
	for _, ok := range []string{"SELECT 1", "  select * from t;", "-- c\nSELECT 1", "/* c */ SELECT 1", "WITH x AS (SELECT 1) SELECT * FROM x",
		"SHOW TABLES", "EXPLAIN SELECT 1", "DESCRIBE t"} {
		if !IsReadSQL(ok) {
			t.Errorf("refused %q", ok)
		}
	}
	for _, bad := range []string{"INSERT INTO t VALUES (1)", "DROP TABLE t", "/*! DROP TABLE t */ SELECT 1", "CREATE TABLE x (a int)",
		"/* SELECT */ DELETE FROM t", "", "CALL p()", "SET @a = 1", "selectx 1"} {
		if IsReadSQL(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
}

// ---------------------------------------------------------------- registration / runtime

func TestCodeDBNotAtRegistration(t *testing.T) {
	for _, src := range []string{"db.table('orders')\n", "db.raw('1')\n", "db.transaction(() => 1)\n", "db.docs('x')\n", "db.query('SELECT 1')\n"} {
		if _, probs := compile(t, map[string]string{"main.ts": src}); len(probs) == 0 || !strings.Contains(probs[0].Message, "handler") {
			t.Errorf("%q at registration: %v", src, probs)
		}
	}
	// inside a handler without a database: a clear error
	e := testEngine()
	p := mustCompile(t, map[string]string{"main.ts": "site.route('GET /x', { access: ['*'] }, () => db.table('orders').get())\n"})
	if _, _, err := call(t, e, p, "GET", "/x", nil); err == nil || !strings.Contains(err.Error(), "db") {
		t.Fatalf("no database: %v", err)
	}
	if GeneratedTypes(nil) != "" {
		t.Fatal("types without tables")
	}
	snap := NewSnapshot()
	snap.Tables["order_items"] = &STable{Name: "order_items", Columns: []*SColumn{{Name: "id", Type: "bigint unsigned"}, {Name: "price", Type: "decimal(12,2)"},
		{Name: "ok", Type: "tinyint(1)"}, {Name: "st", Type: "enum('a','b')", Nullable: true}, {Name: "meta", Type: "json"}, {Name: "at", Type: "datetime(3)"}}}
	ts := GeneratedTypes(snap)
	for _, want := range []string{"interface OrderItemsRow {", "id: number;", "price: string;", "ok: boolean;", `st: "a" | "b" | null;`, "meta: any;", "at: string;",
		"interface TableRows {\n  order_items: OrderItemsRow;"} {
		if !strings.Contains(ts, want) {
			t.Errorf("types missing %q:\n%s", want, ts)
		}
	}
}

func asFunc(v any) (func(goja.Value, ...goja.Value) (goja.Value, error), bool) {
	gv, ok := v.(goja.Value)
	if !ok {
		return nil, false
	}
	f, ok := goja.AssertFunction(gv)
	return f, ok
}
