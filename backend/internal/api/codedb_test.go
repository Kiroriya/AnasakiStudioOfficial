package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Phase 2: the site's own database. TestCodeDBSeparate needs
// TEST_CODE_DB_DSN (a user with privileges on its own database only);
// TestCodeDBFallback runs without it.

const migCreate = `export default migration({
  up(schema) {
    schema.create('customers', (t) => { t.id(); t.string('name', 100); t.timestamps() })
    schema.create('orders', (t) => {
      t.id()
      t.string('code', 20).unique()
      t.foreignId('customer_id').references('customers').onDelete('cascade')
      t.foreignId('user_id').nullable().references('core_users')
      t.decimal('total', 10, 2).default(0)
      t.boolean('paid').default(false)
      t.json('meta').nullable()
      t.datetime('placed_at').nullable()
      t.text('note').nullable()
      t.timestamps()
    })
    schema.create('order_items', (t) => {
      t.id()
      t.foreignId('order_id').references('orders').onDelete('cascade')
      t.integer('qty')
    })
    schema.create('tags', (t) => { t.id(); t.string('label', 50); t.integer('weight').nullable(); t.date('day').nullable() })
  },
  down(schema) { schema.drop('order_items'); schema.drop('orders'); schema.drop('customers'); schema.drop('tags') },
})
`

const migAlter = `export default migration({
  up(schema) {
    schema.table('customers', (t) => {
      t.string('email', 191).nullable().after('name')
      t.index('email')
    })
  },
  down(schema) { schema.table('customers', (t) => { t.dropIndex('email_index'); t.dropColumn('email') }) },
})
`

const migDrop = `export default migration({
  up(schema) { schema.table('orders', (t) => { t.dropColumn('note') }) },
  down(schema) { schema.table('orders', (t) => { t.text('note').nullable() }) },
})
`

const codeRoutes = `site.route('GET /version', { access: ['*'] }, () => '%s')
site.route('GET /orders', { access: ['*'] }, ({ request }) => db.table('orders as o')
  .join('customers as c', 'c.id', 'o.customer_id')
  .leftJoin('core_users as u', 'u.id', 'o.user_id')
  .select('o.id', 'o.code', 'o.total', 'o.paid', 'o.meta', 'o.placed_at', 'c.name as customer', 'u.name as owner')
  .where((q) => q.where('o.total', '>', 0).orWhereNull('o.code'))
  .orderBy('o.id')
  .paginate(Number(request.query.page || 1), 2))
site.route('POST /orders', { access: ['*'] }, ({ request }) => {
  const b = request.json()
  try {
    return db.transaction((tx) => {
      const w = b.global ? db : tx // 2.5.4: the global db inside the transaction is the same transaction
      const id = w.table('orders').insert({ code: b.code, customer_id: b.customer, total: b.total, user_id: b.user ?? null,
        meta: { tags: ['a'] }, placed_at: new Date('2026-10-24T10:30:00.123Z') })
      w.table('order_items').insert(b.items.map((q) => ({ order_id: id, qty: q })))
      if (b.fail) throw new Error('boom')
      return { id }
    })
  } catch (e) {
    return { status: 409, body: { error: String(e.message) } }
  }
})
site.route('GET /stats', { access: ['*'] }, () => ({
  count: db.table('orders').count(),
  sum: db.table('order_items').sum('qty'),
  grouped: db.table('order_items').select('order_id', db.raw('SUM(qty) AS n')).groupBy('order_id').having('n', '>', 1).orderBy('order_id').get(),
  raw: db.query('SELECT COUNT(*) AS c FROM orders o JOIN core_users u ON u.id = o.user_id', []),
  first: db.table('orders').where({ paid: false }).whereIn('id', [1, 2, 3]).first()?.code ?? null,
  unsafe: (() => { try { db.table('orders').delete(); return 'deleted' } catch (e) { return 'refused' } })(),
}))
site.route('GET /docs', { access: ['*'] }, () => {
  const notes = db.docs('notes')
  const id = notes.insert({ title: 'a', n: 1, nested: { k: 'x' } })
  notes.insert({ title: 'b', n: 2 })
  notes.update(id, { n: 5 })
  return { got: notes.get(id), found: notes.find({ where: { 'nested.k': 'x' } }).total,
    sorted: notes.find({ orderBy: '-n' }).items.map((d) => d.title), del: notes.delete(id), gone: notes.get(id) }
})
site.route('GET /evil', { access: ['*'] }, () => db.query('SELECT * FROM ' + '%s' + '.users'))
`

func cmsDBName(t *testing.T) string {
	cfg, err := mysql.ParseDSN(os.Getenv("TEST_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.DBName
}

func publishWith(c *client, base any, confirm bool) (int, map[string]any) {
	return c.do("POST", "/api/v1/code/publish", map[string]any{"message": "t", "baseVersion": base, "confirmDestructive": confirm})
}

func liveID(t *testing.T, c *client) any {
	t.Helper()
	_, st := c.do("GET", "/api/v1/code/status", nil)
	return st["liveVersion"]
}

func putFile(t *testing.T, c *client, path, content string) {
	t.Helper()
	_, cur := c.do("GET", "/api/v1/code/files/"+path, nil)
	base := ""
	if u, ok := cur["updatedAt"].(string); ok {
		base = u
	}
	c.put(path, content, base)
}

func multipartFile(t *testing.T, c *client, url, name string, data []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	rec := c.send("POST", url, &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestCodeDBSeparate(t *testing.T) {
	codeDSN := os.Getenv("TEST_CODE_DB_DSN")
	if codeDSN == "" {
		t.Skip("TEST_CODE_DB_DSN not set (least-privilege code database user)")
	}
	c, _, st := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	cms := cmsDBName(t)
	_, me := admin.do("GET", "/api/v1/auth/me", nil)
	ownerID := me["id"]

	code, s := admin.do("GET", "/api/v1/code/db/status", nil)
	if code != 200 || s["mode"] != "separate" || s["configured"] != true || s["setupSql"] != nil {
		t.Fatalf("status: %d %v", code, s)
	}

	// the core_* views exist from the start (created by the CMS user)
	_, tl := admin.doAny("GET", "/api/v1/code/db/tables", nil)
	if names := fmt.Sprint(tl); !strings.Contains(names, "name:core_users") || !strings.Contains(names, "name:core_posts") || !strings.Contains(names, "view:true") {
		t.Fatalf("core views at startup: %v", tl)
	}

	// ---- dry run, then publish (create + alter + FK)
	admin.put("main.ts", fmt.Sprintf(codeRoutes, "v1", cms), "")
	admin.put("migrations/001_create.ts", migCreate, "")
	admin.put("migrations/002_alter.ts", migAlter, "")
	if _, st := admin.do("GET", "/api/v1/code/status", nil); st["db"].(map[string]any)["pendingMigrations"].(float64) != 2 || st["db"].(map[string]any)["mode"] != "separate" {
		t.Fatalf("status db: %v", st["db"])
	}
	code, b := admin.do("POST", "/api/v1/code/build", nil)
	if code != 200 || b["ok"] != true {
		t.Fatalf("build: %d %v", code, b)
	}
	pending := b["migrations"].(map[string]any)["pending"].([]any)
	if len(pending) != 2 || pending[0].(map[string]any)["name"] != "001_create" ||
		!strings.Contains(fmt.Sprint(pending[1].(map[string]any)["sql"]), "ALTER TABLE `customers` ADD COLUMN `email` VARCHAR(191) NULL AFTER `name`") ||
		len(pending[0].(map[string]any)["destructive"].([]any)) != 0 {
		t.Fatalf("dry run: %v", pending)
	}
	if !strings.Contains(fmt.Sprint(pending[0].(map[string]any)["sql"]), "FOREIGN KEY (`customer_id`) REFERENCES `customers` (`id`) ON DELETE CASCADE") {
		t.Fatalf("dry run FK: %v", pending[0])
	}
	// the dry run did not touch the database
	if code, _ := admin.do("GET", "/api/v1/code/db/tables/orders", nil); code != 404 {
		t.Fatalf("dry run created tables: %d", code)
	}
	if code, b := publishWith(admin, nil, false); code != 200 {
		t.Fatalf("publish: %d %v", code, b)
	}
	_, list := admin.doAny("GET", "/api/v1/code/db/migrations", nil)
	ms := list.([]any)
	if len(ms) != 2 || ms[0].(map[string]any)["status"] != "ran" || ms[0].(map[string]any)["batch"].(float64) != 1 || ms[1].(map[string]any)["ranBy"] != "Owner" ||
		ms[0].(map[string]any)["pending"] != false || ms[0].(map[string]any)["inLive"] != true {
		t.Fatalf("migrations: %v", ms)
	}
	_, tbl := admin.do("GET", "/api/v1/code/db/tables/orders", nil)
	if tbl["primaryKey"] != "id" || len(tbl["foreignKeys"].([]any)) != 1 || tbl["foreignKeys"].([]any)[0].(map[string]any)["refTable"] != "customers" ||
		len(tbl["relations"].([]any)) != 1 || tbl["relations"].([]any)[0].(map[string]any)["refTable"] != "core_users" {
		t.Fatalf("table detail: %v", tbl)
	}
	_, cust := admin.do("GET", "/api/v1/code/db/tables/customers", nil)
	if cols := cust["columns"].([]any); cols[2].(map[string]any)["name"] != "email" {
		t.Fatalf("alter after(): %v", cols)
	}

	// ---- rows through the admin API (edit_code_data) and routes
	code, row := admin.do("POST", "/api/v1/code/db/tables/customers/rows", map[string]any{"name": "Ann"})
	if code != 201 || row["name"] != "Ann" || row["id"].(float64) != 1 || !strings.HasSuffix(row["created_at"].(string), "Z") {
		t.Fatalf("insert row: %d %v", code, row)
	}
	order := func(body map[string]any) (int, string) {
		bb, _ := json.Marshal(body)
		rec := c.send("POST", "/orders", bytes.NewReader(bb), map[string]string{"Content-Type": "application/json", "Accept": "application/json"})
		return rec.Code, rec.Body.String()
	}
	if code, body := order(map[string]any{"code": "A1", "customer": 1, "total": 12.5, "user": ownerID, "items": []int{1, 2}}); code != 200 || !strings.Contains(body, `"id":1`) {
		t.Fatalf("order: %d %s", code, body)
	}
	if code, body := order(map[string]any{"code": "A2", "customer": 1, "total": 3, "items": []int{5}}); code != 200 {
		t.Fatalf("order 2: %d %s", code, body)
	}
	if code, body := order(map[string]any{"code": "A3", "customer": 1, "total": 9, "items": []int{1}}); code != 200 {
		t.Fatalf("order 3: %d %s", code, body)
	}
	// a throw inside the transaction rolls everything back
	if code, body := order(map[string]any{"code": "BAD", "customer": 1, "total": 1, "items": []int{1}, "fail": true}); code != 409 || !strings.Contains(body, "boom") {
		t.Fatalf("failing transaction: %d %s", code, body)
	}
	// 2.5.4: writes through the global db inside db.transaction roll back too (they ran outside it before)
	if code, body := order(map[string]any{"code": "BAD2", "customer": 1, "total": 1, "items": []int{1}, "fail": true, "global": true}); code != 409 || !strings.Contains(body, "boom") {
		t.Fatalf("failing transaction (global db): %d %s", code, body)
	}
	code, body := c.public("GET", "/orders?page=1")
	var pg map[string]any
	_ = json.Unmarshal([]byte(body), &pg)
	if code != 200 || pg["total"].(float64) != 3 || pg["perPage"].(float64) != 2 || len(pg["items"].([]any)) != 2 {
		t.Fatalf("paginate: %d %s", code, body)
	}
	first := pg["items"].([]any)[0].(map[string]any)
	if first["total"] != "12.50" || first["paid"] != false || first["placed_at"] != "2026-10-24T10:30:00.123Z" || first["customer"] != "Ann" ||
		first["owner"] != "Owner" || fmt.Sprint(first["meta"]) != "map[tags:[a]]" {
		t.Fatalf("row mapping / core_users join: %v", first)
	}
	if _, body := c.public("GET", "/orders?page=2"); !strings.Contains(body, `"code":"A3"`) || strings.Contains(body, "BAD") {
		t.Fatalf("page 2: %s", body)
	}
	code, body = c.public("GET", "/stats")
	if code != 200 || !strings.Contains(body, `"count":3`) || !strings.Contains(body, `"sum":9`) || !strings.Contains(body, `"grouped":[{"order_id":1,"n":"3"},{"order_id":2,"n":"5"}]`) ||
		!strings.Contains(body, `"raw":[{"c":1}]`) || !strings.Contains(body, `"unsafe":"refused"`) || !strings.Contains(body, `"first":"A1"`) {
		t.Fatalf("stats: %d %s", code, body)
	}
	code, body = c.public("GET", "/docs")
	if code != 200 || !strings.Contains(body, `"got":{"n":5,"title":"a","nested":{"k":"x"},"id":1}`) && !strings.Contains(body, `"n":5`) ||
		!strings.Contains(body, `"found":1`) || !strings.Contains(body, `"sorted":["a","b"]`) || !strings.Contains(body, `"del":true`) || !strings.Contains(body, `"gone":null`) {
		t.Fatalf("docs: %d %s", code, body)
	}
	// the code user cannot read CMS tables, even with raw SQL
	if code, _ := c.public("GET", "/evil"); code != 500 {
		t.Fatalf("raw SQL reached the CMS database: %d", code)
	}
	_, logs := admin.do("GET", "/api/v1/code/logs?source=route%20GET%20/evil", nil)
	if !strings.Contains(fmt.Sprint(logs["items"]), "denied") {
		t.Fatalf("evil log: %v", logs)
	}

	// ---- least privilege, straight on the code user's connection
	codeDB, err := sql.Open("mysql", codeDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer codeDB.Close()
	for _, q := range []string{
		"SELECT * FROM `" + cms + "`.users",
		"UPDATE `" + cms + "`.users SET name = 'pwned'",
		"CREATE OR REPLACE VIEW core_users AS SELECT password_hash FROM `" + cms + "`.users",
		"UPDATE core_users SET name = 'pwned'",
		"INSERT INTO core_posts (title) VALUES ('x')",
		"CREATE TABLE stolen AS SELECT * FROM `" + cms + "`.options",
	} {
		if _, err := codeDB.Exec(q); err == nil {
			t.Errorf("code user could run: %s", q)
		}
	}
	var n int
	if err := codeDB.QueryRow("SELECT COUNT(*) FROM orders o JOIN core_users u ON u.id = o.user_id").Scan(&n); err != nil || n != 1 {
		t.Fatalf("core_users join as the code user: %d %v", n, err)
	}
	var cols string
	_ = codeDB.QueryRow("SELECT GROUP_CONCAT(COLUMN_NAME ORDER BY ORDINAL_POSITION) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'core_users'").Scan(&cols)
	if cols != "id,name,email,role,status,created_at" {
		t.Fatalf("core_users columns: %s", cols)
	}
	var owner string
	if err := st.DB.QueryRow("SELECT name FROM users WHERE id = ?", ownerID).Scan(&owner); err != nil || owner != "Owner" {
		t.Fatalf("CMS users changed: %s %v", owner, err)
	}

	// ---- a failing migration keeps the live code
	putFile(t, admin, "main.ts", fmt.Sprintf(codeRoutes, "v2", cms))
	admin.put("migrations/003_bad.ts", "export default migration({\n  up(schema) {\n    schema.table('customers', (t) => { t.string('vip', 10).nullable() })\n    schema.raw('CREATE INDEX ix ON no_such_table (x)')\n  },\n})\n", "")
	v1 := liveID(t, admin)
	code, b = publishWith(admin, v1, false)
	if code != 400 || b["migration"] != "003_bad" || !strings.Contains(fmt.Sprint(b["errors"]), "migrations/003_bad.ts") ||
		!strings.Contains(fmt.Sprint(b["errors"]), "no_such_table") {
		t.Fatalf("failing migration: %d %v", code, b)
	}
	if errs := b["errors"].([]any); errs[0].(map[string]any)["line"].(float64) != 4 {
		t.Fatalf("failing migration line: %v", errs)
	}
	if _, body := c.public("GET", "/version"); body != "v1" || liveID(t, admin) != v1 {
		t.Fatalf("live code changed after a failed migration: %s", body)
	}
	_, list = admin.doAny("GET", "/api/v1/code/db/migrations", nil)
	if m := list.([]any)[2].(map[string]any); m["name"] != "003_bad" || m["status"] != "failed" || !strings.Contains(fmt.Sprint(m["error"]), "no_such_table") || m["pending"] != true {
		t.Fatalf("failed migration recorded: %v", m)
	}
	// partially applied: the first statement ran (documented); the fix is a new migration
	if code, _ := admin.do("DELETE", "/api/v1/code/files/migrations/003_bad.ts", nil); code != 204 {
		t.Fatal("delete bad migration")
	}

	// ---- destructive: confirmation + automatic backup
	admin.put("migrations/003_drop_note.ts", migDrop, "")
	code, b = admin.do("POST", "/api/v1/code/build", nil)
	if code != 200 || !strings.Contains(fmt.Sprint(b["migrations"]), "drop_column") {
		t.Fatalf("build destructive: %v", b)
	}
	code, b = publishWith(admin, v1, false)
	ds, _ := b["destructive"].([]any)
	if code != 400 || len(ds) != 1 || ds[0].(map[string]any)["migration"] != "003_drop_note" || ds[0].(map[string]any)["op"] != "drop_column" ||
		ds[0].(map[string]any)["table"] != "orders" || ds[0].(map[string]any)["column"] != "note" {
		t.Fatalf("destructive refused: %d %v", code, b)
	}
	if _, body := c.public("GET", "/version"); body != "v1" {
		t.Fatalf("refused publish changed live code: %s", body)
	}
	if code, b = publishWith(admin, v1, true); code != 200 {
		t.Fatalf("confirmed publish: %d %v", code, b)
	}
	if _, body := c.public("GET", "/version"); body != "v2" {
		t.Fatalf("new version not live: %s", body)
	}
	_, bl := admin.doAny("GET", "/api/v1/code/db/backups", nil)
	backups := bl.([]any)
	if len(backups) != 1 || backups[0].(map[string]any)["tableName"] != "orders" || !strings.HasPrefix(backups[0].(map[string]any)["backupTable"].(string), "bak_orders_") ||
		backups[0].(map[string]any)["migration"] != "003_drop_note" {
		t.Fatalf("backups: %v", backups)
	}
	bid := int(backups[0].(map[string]any)["id"].(float64))
	rec := admin.send("GET", fmt.Sprintf("/api/v1/code/db/backups/%d/download", bid), nil, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "CREATE TABLE `bak_orders_") || strings.Count(rec.Body.String(), "'A") != 3 ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), ".sql") {
		t.Fatalf("backup download: %d %s", rec.Code, rec.Body.String())
	}
	_, tbls := admin.doAny("GET", "/api/v1/code/db/tables", nil)
	if strings.Contains(fmt.Sprint(tbls), "bak_") || strings.Contains(fmt.Sprint(tbls), "script_") {
		t.Fatalf("tables list shows backups/bookkeeping: %v", tbls)
	}

	// ---- rollback: only the latest batch, needs edit_code_data + publish_scripts
	if code, _ := admin.do("POST", "/api/v1/code/db/migrations/001_create/down", nil); code != 409 {
		t.Fatalf("rollback of an old batch: %d", code)
	}
	dataEditor := roleUser(t, c, admin, "data-editor", auth.CapRead, auth.CapViewScripts, auth.CapViewCodeData, auth.CapEditCodeData)
	if code, _ := dataEditor.do("POST", "/api/v1/code/db/migrations/003_drop_note/down", nil); code != 403 {
		t.Fatalf("rollback without publish_scripts: %d", code)
	}
	if code, b := admin.do("POST", "/api/v1/code/db/migrations/003_drop_note/down", nil); code != 200 || b["ok"] != true {
		t.Fatalf("rollback: %d %v", code, b)
	}
	_, tbl = admin.do("GET", "/api/v1/code/db/tables/orders", nil)
	if !strings.Contains(fmt.Sprint(tbl["columns"]), "name:note") {
		t.Fatalf("down() did not restore the column: %v", tbl["columns"])
	}
	_, list = admin.doAny("GET", "/api/v1/code/db/migrations", nil)
	for _, m := range list.([]any) {
		mm := m.(map[string]any)
		if mm["name"] == "003_drop_note" && (mm["status"] != "rolled_back" || mm["pending"] != true) {
			t.Fatalf("rolled back status: %v", mm)
		}
	}
	if code, _ := admin.do("POST", "/api/v1/code/db/migrations/003_drop_note/down", nil); code != 404 {
		t.Fatalf("second rollback: %d", code)
	}

	// ---- caps: view vs edit
	viewer := roleUser(t, c, admin, "data-viewer", auth.CapRead, auth.CapViewCodeData)
	nobody := roleUser(t, c, admin, "no-data", auth.CapRead, auth.CapViewScripts)
	if code, _ := nobody.do("GET", "/api/v1/code/db/tables", nil); code != 403 {
		t.Fatalf("tables without view_code_data: %d", code)
	}
	if code, _ := viewer.doAny("GET", "/api/v1/code/db/tables", nil); code != 200 {
		t.Fatalf("viewer tables: %d", code)
	}
	if code, rows := viewer.do("GET", "/api/v1/code/db/tables/orders/rows?sort=code&dir=desc&perPage=2", nil); code != 200 || rows["total"].(float64) != 3 ||
		rows["items"].([]any)[0].(map[string]any)["code"] != "A3" || rows["primaryKey"] != "id" {
		t.Fatalf("viewer rows: %d %v", code, rows)
	}
	if _, rows := viewer.do("GET", "/api/v1/code/db/tables/orders/rows?q=A2", nil); rows["total"].(float64) != 1 {
		t.Fatalf("search: %v", rows)
	}
	if _, rows := viewer.do("GET", "/api/v1/code/db/tables/orders/rows?filter[code]=A", nil); rows["total"].(float64) != 3 {
		t.Fatalf("filter: %v", rows)
	}
	if _, rows := viewer.do("GET", "/api/v1/code/db/tables/core_users/rows", nil); rows["total"].(float64) < 1 || strings.Contains(fmt.Sprint(rows), "password") {
		t.Fatalf("core_users rows: %v", rows)
	}
	for _, rq := range [][2]string{{"POST", "/api/v1/code/db/tables/customers/rows"}, {"PUT", "/api/v1/code/db/tables/customers/rows/1"},
		{"DELETE", "/api/v1/code/db/tables/customers/rows/1"}, {"POST", "/api/v1/code/db/tables/tags/import"}, {"DELETE", fmt.Sprintf("/api/v1/code/db/backups/%d", bid)},
		{"POST", "/api/v1/code/db/migrations/002_alter/down"}} {
		if code, _ := viewer.do(rq[0], rq[1], map[string]any{"name": "x"}); code != 403 {
			t.Errorf("viewer %s %s: %d", rq[0], rq[1], code)
		}
	}
	if code, _ := viewer.do("POST", "/api/v1/code/db/sql", map[string]any{"sql": "UPDATE tags SET label = 'x'", "write": true}); code != 403 {
		t.Fatalf("viewer SQL write: %d", code)
	}
	if code, r := viewer.do("POST", "/api/v1/code/db/sql", map[string]any{"sql": "SELECT code, total FROM orders WHERE total > ? ORDER BY id", "params": []any{5}}); code != 200 ||
		fmt.Sprint(r["columns"]) != "[code total]" || fmt.Sprint(r["rows"]) != "[[A1 12.50] [A3 9.00]]" {
		t.Fatalf("viewer SQL read: %d %v", code, r)
	}
	// views and system tables are read-only
	if code, _ := admin.do("POST", "/api/v1/code/db/tables/core_users/rows", map[string]any{"name": "x"}); code != 400 {
		t.Fatalf("insert into a view: %d", code)
	}
	if code, _ := admin.do("DELETE", "/api/v1/code/db/tables/kv_docs/rows/1", nil); code != 400 {
		t.Fatalf("delete from kv_docs: %d", code)
	}
	if code, r := admin.do("PUT", "/api/v1/code/db/tables/customers/rows/1", map[string]any{"email": "ann@x.io"}); code != 200 || r["email"] != "ann@x.io" {
		t.Fatalf("update row: %d %v", code, r)
	}
	if code, _ := admin.do("PUT", "/api/v1/code/db/tables/customers/rows/99", map[string]any{"email": "x"}); code != 404 {
		t.Fatalf("update missing row: %d", code)
	}
	if code, _ := admin.do("POST", "/api/v1/code/db/tables/customers/rows", map[string]any{"nope": 1}); code != 400 {
		t.Fatalf("unknown column: %d", code)
	}

	// ---- SQL console: read-only rejects writes, write mode works
	for _, q := range []string{"INSERT INTO tags (label) VALUES ('x')", "DROP TABLE tags", "WITH c AS (SELECT 1 AS id) DELETE FROM tags WHERE id IN (SELECT id FROM c)"} {
		if code, r := admin.do("POST", "/api/v1/code/db/sql", map[string]any{"sql": q}); code != 400 {
			t.Errorf("read-only console ran %q: %d %v", q, code, r)
		}
	}
	if code, r := admin.do("POST", "/api/v1/code/db/sql", map[string]any{"sql": "SELECT * FROM `" + cms + "`.users"}); code != 400 || !strings.Contains(fmt.Sprint(r["error"]), "denied") {
		t.Fatalf("console reached the CMS database: %d %v", code, r)
	}
	code, r := admin.do("POST", "/api/v1/code/db/sql", map[string]any{"sql": "INSERT INTO tags (label, weight, day) VALUES (?, ?, ?), ('b', NULL, NULL)",
		"params": []any{"a,\"quoted\"\nline", 7, "2026-10-24"}, "write": true})
	if code != 200 || r["affectedRows"].(float64) != 2 {
		t.Fatalf("console write: %d %v", code, r)
	}

	// ---- export → delete → import (dry run first) → export again
	exp := admin.send("GET", "/api/v1/code/db/tables/tags/export?format=csv", nil, nil)
	if exp.Code != 200 || !strings.Contains(exp.Header().Get("Content-Disposition"), "tags.csv") || !strings.HasPrefix(exp.Body.String(), "id,label,weight,day\n") {
		t.Fatalf("export: %d %s", exp.Code, exp.Body.String())
	}
	csv1 := exp.Body.String()
	if code, _ := admin.do("POST", "/api/v1/code/db/sql", map[string]any{"sql": "DELETE FROM tags", "write": true}); code != 200 {
		t.Fatal("delete tags")
	}
	code, imp := multipartFile(t, admin, "/api/v1/code/db/tables/tags/import?dryRun=1", "tags.csv", []byte(csv1+"x,bad,1,2026-01-01\n"))
	if code != 200 || imp["inserted"].(float64) != 2 || len(imp["errors"].([]any)) != 1 || imp["errors"].([]any)[0].(map[string]any)["row"].(float64) != 4 || imp["dryRun"] != true {
		t.Fatalf("import dry run: %d %v", code, imp)
	}
	if _, rows := admin.do("GET", "/api/v1/code/db/tables/tags/rows", nil); rows["total"].(float64) != 0 {
		t.Fatalf("dry run wrote rows: %v", rows)
	}
	if code, imp = multipartFile(t, admin, "/api/v1/code/db/tables/tags/import", "tags.csv", []byte(csv1)); code != 200 || imp["inserted"].(float64) != 2 {
		t.Fatalf("import: %d %v", code, imp)
	}
	if csv2 := admin.send("GET", "/api/v1/code/db/tables/tags/export", nil, nil).Body.String(); csv2 != csv1 {
		t.Fatalf("CSV round trip:\n%s\n---\n%s", csv1, csv2)
	}
	if exp := admin.send("GET", "/api/v1/code/db/tables/tags/export?format=json", nil, nil); !strings.Contains(exp.Body.String(), `"day":"2026-10-24"`) {
		t.Fatalf("json export: %s", exp.Body.String())
	}
	if code, _ := multipartFile(t, admin, "/api/v1/code/db/tables/tags/import", "x.csv", []byte("nope\n1\n")); code != 400 {
		t.Fatalf("import with unknown column: %d", code)
	}

	// ---- SQL dump round trip (structure + data, no views/backups)
	dump := admin.send("GET", "/api/v1/code/db/dump", nil, nil)
	ds2 := dump.Body.String()
	if dump.Code != 200 || !strings.Contains(ds2, "CREATE TABLE `orders`") || !strings.Contains(ds2, "INSERT INTO `tags`") || strings.Contains(ds2, "core_users") ||
		strings.Contains(ds2, "bak_orders") || strings.Contains(ds2, "script_migrations") || !strings.Contains(ds2, "CREATE TABLE `kv_docs`") {
		t.Fatalf("dump: %d %s", dump.Code, ds2)
	}
	cfg, _ := mysql.ParseDSN(codeDSN)
	cfg.MultiStatements = true
	multi, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer multi.Close()
	if _, err := multi.Exec(ds2); err != nil {
		t.Fatalf("restore dump: %v", err)
	}
	if csv3 := admin.send("GET", "/api/v1/code/db/tables/tags/export", nil, nil).Body.String(); csv3 != csv1 {
		t.Fatalf("dump round trip:\n%s\n---\n%s", csv1, csv3)
	}
	if _, body := c.public("GET", "/orders?page=1"); !strings.Contains(body, `"total":3`) {
		t.Fatalf("orders after dump restore: %s", body)
	}

	// ---- backups delete, generated types, audit
	if code, _ := admin.do("DELETE", fmt.Sprintf("/api/v1/code/db/backups/%d", bid), nil); code != 204 {
		t.Fatalf("delete backup: %d", code)
	}
	if _, bl := admin.doAny("GET", "/api/v1/code/db/backups", nil); len(bl.([]any)) != 0 {
		t.Fatalf("backup still listed: %v", bl)
	}
	types := admin.send("GET", "/api/v1/code/types", nil, nil).Body.String()
	for _, want := range []string{"interface OrdersRow {", "total: string;", "paid: boolean;", "interface TableRows {\n  customers: CustomersRow;", "declare function migration"} {
		if !strings.Contains(types, want) {
			t.Errorf("types missing %q", want)
		}
	}
	var audits int
	_ = st.DB.QueryRow(`SELECT COUNT(DISTINCT action) FROM script_audit WHERE action IN ('db_insert','db_update','db_sql','db_import','db_rollback','db_backup_delete')`).Scan(&audits)
	if audits != 6 {
		t.Fatalf("audit actions: %d", audits)
	}
}

func TestCodeDBFallback(t *testing.T) {
	c, _, st := newCodeAPI(t, func(cfg *sitecode.Config) { cfg.CodeDSN = "" })
	admin, _ := setupSite(t, c)
	code, s := admin.do("GET", "/api/v1/code/db/status", nil)
	if code != 200 || s["mode"] != "fallback" || s["configured"] != false || len(s["setupSql"].([]any)) != 4 ||
		!strings.Contains(fmt.Sprint(s["setupSql"]), "GRANT ALL PRIVILEGES") {
		t.Fatalf("fallback status: %d %v", code, s)
	}
	admin.put("migrations/001_notes.ts", "export default migration({\n  up(schema) {\n    schema.create('notes', (t) => { t.id(); t.string('title'); t.boolean('done').default(false); t.timestamps() })\n  },\n  down(schema) { schema.drop('notes') },\n})\n", "")
	admin.put("main.ts", `site.route('GET /notes', { access: ['*'] }, ({ request }) => {
  if (request.query.add) db.table('notes').insert({ title: request.query.add })
  return { items: db.table('notes').select('id', 'title', 'done').where('notes.done', false).orderBy('notes.id').get(),
    docs: db.docs('d').insert({ a: 1 }) }
})
site.route('GET /raw', { access: ['*'] }, () => db.raw('1'))
site.route('GET /query', { access: ['*'] }, () => db.query('SELECT 1'))
site.route('GET /core', { access: ['*'] }, () => db.table('core_users').get())
site.route('GET /cms', { access: ['*'] }, () => db.table('users').get())
`, "")
	code, b := admin.do("POST", "/api/v1/code/build", nil)
	if code != 200 || b["ok"] != true || !strings.Contains(fmt.Sprint(b["migrations"]), "CREATE TABLE `sc_notes`") {
		t.Fatalf("fallback build: %d %v", code, b)
	}
	if code, b := publishWith(admin, nil, false); code != 200 {
		t.Fatalf("fallback publish: %d %v", code, b)
	}
	var n int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('sc_notes', 'sc_kv_docs')`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("sc_ tables in the CMS database: %d %v", n, err)
	}
	if code, body := c.public("GET", "/notes?add=hello"); code != 200 || !strings.Contains(body, `"items":[{"id":1,"title":"hello","done":false}]`) || !strings.Contains(body, `"docs":1`) {
		t.Fatalf("fallback query builder: %d %s", code, body)
	}
	for _, p := range []string{"/raw", "/query", "/core", "/cms"} {
		if code, _ := c.public("GET", p); code != 500 {
			t.Errorf("fallback %s: %d", p, code)
		}
	}
	_, logs := admin.do("GET", "/api/v1/code/logs?level=error", nil)
	if !strings.Contains(fmt.Sprint(logs["items"]), "db.query ใช้ไม่ได้") || !strings.Contains(fmt.Sprint(logs["items"]), "db.raw ใช้ไม่ได้") {
		t.Fatalf("fallback errors: %v", logs)
	}
	if code, r := admin.do("POST", "/api/v1/code/db/sql", map[string]any{"sql": "SELECT 1"}); code != 400 || !strings.Contains(fmt.Sprint(r["error"]), "CODE_DB_DSN") {
		t.Fatalf("fallback console: %d %v", code, r)
	}
	_, tl := admin.doAny("GET", "/api/v1/code/db/tables", nil)
	names := fmt.Sprint(tl)
	if !strings.Contains(names, "name:notes") || !strings.Contains(names, "name:kv_docs") || strings.Contains(names, "name:users") ||
		strings.Contains(names, "sc_") || strings.Contains(names, "script_") {
		t.Fatalf("fallback tables: %v", tl)
	}
	if code, r := admin.do("POST", "/api/v1/code/db/tables/notes/rows", map[string]any{"title": "row", "done": true}); code != 201 || r["done"] != true {
		t.Fatalf("fallback insert row: %d %v", code, r)
	}
	if code, _ := admin.do("GET", "/api/v1/code/db/tables/users", nil); code != 404 {
		t.Fatalf("CMS table visible in fallback admin: %d", code)
	}
	// destructive + rollback work in fallback too (sc_ backups)
	admin.put("migrations/002_drop.ts", "export default migration({ up(s) { s.drop('notes') }, down(s) { s.create('notes', (t) => { t.id() }) } })\n", "")
	if code, b := publishWith(admin, liveID(t, admin), false); code != 400 || b["destructive"] == nil {
		t.Fatalf("fallback destructive: %d %v", code, b)
	}
	if code, b := publishWith(admin, liveID(t, admin), true); code != 200 {
		t.Fatalf("fallback confirmed: %d %v", code, b)
	}
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME LIKE 'sc\_bak\_notes\_%'`).Scan(&n)
	if n != 1 {
		t.Fatalf("fallback backup table: %d", n)
	}
	if code, b := admin.do("POST", "/api/v1/code/db/migrations/002_drop/down", nil); code != 200 {
		t.Fatalf("fallback rollback: %d %v", code, b)
	}
}

func TestCodeDBDemo(t *testing.T) {
	c, _, _ := newCodeAPI(t, func(cfg *sitecode.Config) { cfg.Demo = true })
	admin, _ := setupSite(t, c)
	if code, s := admin.do("GET", "/api/v1/code/db/status", nil); code != 200 || s["mode"] == nil {
		t.Fatalf("demo db status: %d %v", code, s)
	}
	for _, rq := range [][2]string{{"GET", "/api/v1/code/db/tables"}, {"GET", "/api/v1/code/db/dump"}, {"POST", "/api/v1/code/db/sql"},
		{"GET", "/api/v1/code/db/migrations"}, {"GET", "/api/v1/code/db/backups"}} {
		if code, _ := admin.do(rq[0], rq[1], map[string]any{"sql": "SELECT 1"}); code != 403 {
			t.Errorf("demo %s %s: %d", rq[0], rq[1], code)
		}
	}
}
