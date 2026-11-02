package sitecode

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

// The site's own database. In "separate" mode developer tables live in their
// own MySQL database reached by a dedicated user (CODE_DB_DSN) that has no
// privileges on the CMS database; the CMS user creates read-only core_*
// views there. Without CODE_DB_DSN (or when it does not work) "fallback"
// mode keeps developer tables in the CMS database with the sc_ prefix and
// disables raw SQL.

// Data modes.
const (
	ModeSeparate = "separate"
	ModeFallback = "fallback"
)

// Limits.
const (
	stmtTimeout   = 5 * time.Second
	backupDays    = 7
	ddlTimeout    = 5 * time.Minute
	adminTimeout  = 30 * time.Second
	consoleTimout = 10 * time.Second
)

// Data owns the connections to the code database.
type Data struct {
	Mode       string
	Configured bool
	Err        string
	Database   string // the code database (fallback: the CMS database)
	cmsDB      string
	cmsUser    string
	N          Naming

	code  *sql.DB // scripts, migrations, SQL console (code user; fallback: CMS)
	admin *sql.DB // the CMS user on the code database: views, bookkeeping, admin screens

	owned    []*sql.DB
	stop     chan struct{}
	stopOnce sync.Once
}

// OpenData sets up the code database (separate when codeDSN works, else
// fallback in the CMS database cms).
func OpenData(cms *sql.DB, cmsDSN, codeDSN string) *Data {
	d := &Data{Mode: ModeFallback, Configured: strings.TrimSpace(codeDSN) != "", stop: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	_ = cms.QueryRowContext(ctx, `SELECT COALESCE(DATABASE(), '')`).Scan(&d.cmsDB)
	if c, err := mysql.ParseDSN(cmsDSN); err == nil && cmsDSN != "" {
		d.cmsUser = c.User
	} else {
		var cu string
		_ = cms.QueryRowContext(ctx, `SELECT CURRENT_USER()`).Scan(&cu)
		d.cmsUser, _, _ = strings.Cut(cu, "@")
	}
	if d.Configured {
		if err := d.openSeparate(ctx, cmsDSN, codeDSN); err != nil {
			d.Err = err.Error()
			log.Printf("site code: code database (CODE_DB_DSN) not usable, using fallback mode: %v", err)
			for _, db := range d.owned {
				_ = db.Close()
			}
			d.owned = nil
		} else {
			d.Mode = ModeSeparate
		}
	}
	if d.Mode == ModeFallback {
		d.code, d.admin, d.Database = cms, cms, d.cmsDB
		d.N = Naming{Prefix: "sc_", Fallback: true}
		if err := d.ensureTables(ctx); err != nil {
			log.Printf("site code: create bookkeeping tables: %v", err)
			if d.Err == "" {
				d.Err = err.Error()
			}
		}
	}
	go d.cleanupLoop()
	return d
}

func openDB(cfg *mysql.Config) (*sql.DB, error) {
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.MultiStatements = false
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	delete(cfg.Params, "multiStatements")
	if _, ok := cfg.Params["charset"]; !ok {
		cfg.Params["charset"] = "utf8mb4"
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

func (d *Data) openSeparate(ctx context.Context, cmsDSN, codeDSN string) error {
	ccfg, err := mysql.ParseDSN(codeDSN)
	if err != nil {
		return fmt.Errorf("CODE_DB_DSN ไม่ถูกต้อง: %w", err)
	}
	if ccfg.DBName == "" {
		return errors.New("CODE_DB_DSN ต้องระบุชื่อฐานข้อมูล")
	}
	if ccfg.DBName == d.cmsDB {
		return errors.New("CODE_DB_DSN ต้องเป็นฐานข้อมูลคนละตัวกับของ CMS")
	}
	acfg, err := mysql.ParseDSN(cmsDSN)
	if err != nil || cmsDSN == "" {
		return errors.New("ต้องมี DB_DSN ของ CMS เพื่อสร้าง view ในฐานข้อมูลของโค้ด")
	}
	acfg.DBName = ccfg.DBName
	code, err := openDB(ccfg)
	if err != nil {
		return err
	}
	d.owned = append(d.owned, code)
	admin, err := openDB(acfg)
	if err != nil {
		return err
	}
	d.owned = append(d.owned, admin)
	var lastErr error
	for i := 0; i < 5; i++ {
		if lastErr = code.PingContext(ctx); lastErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		return fmt.Errorf("เชื่อมต่อฐานข้อมูลของโค้ดไม่ได้: %w", lastErr)
	}
	if err := admin.PingContext(ctx); err != nil {
		return fmt.Errorf("ผู้ใช้ของ CMS เข้าฐานข้อมูล %s ไม่ได้ (ต้อง GRANT ALL): %w", ccfg.DBName, err)
	}
	// the code user must not reach the CMS database
	var n int
	if err := code.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?`, d.cmsDB).Scan(&n); err == nil && n > 0 {
		return fmt.Errorf("ผู้ใช้ของ CODE_DB_DSN มองเห็นฐานข้อมูลของ CMS (%s) — ใช้ผู้ใช้ที่มีสิทธิ์เฉพาะฐานข้อมูลของโค้ด", d.cmsDB)
	}
	d.code, d.admin, d.Database = code, admin, ccfg.DBName
	d.N = Naming{}
	if err := d.ensureTables(ctx); err != nil {
		return fmt.Errorf("สร้างตารางระบบในฐานข้อมูลของโค้ดไม่ได้: %w", err)
	}
	if err := d.refreshViews(); err != nil {
		return fmt.Errorf("สร้าง view core_* ไม่ได้: %w", err)
	}
	return nil
}

// Close stops the cleanup and closes the connections the Data opened.
func (d *Data) Close() {
	if d == nil {
		return
	}
	d.stopOnce.Do(func() { close(d.stop) })
	for _, db := range d.owned {
		_ = db.Close()
	}
}

// Separate reports the separate-database mode.
func (d *Data) Separate() bool { return d != nil && d.Mode == ModeSeparate }

// DocsTable is the physical kv_docs table.
func (d *Data) DocsTable() string { return d.N.Prefix + "kv_docs" }

func (d *Data) ensureTables(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS script_migrations (
  name        VARCHAR(191) NOT NULL PRIMARY KEY,
  batch       INT          NOT NULL,
  ran_at      DATETIME(3)  NOT NULL,
  ran_by      VARCHAR(191) NOT NULL DEFAULT '',
  duration_ms INT          NOT NULL DEFAULT 0,
  status      VARCHAR(16)  NOT NULL,
  error       TEXT         NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS script_table_backups (
  id           BIGINT AUTO_INCREMENT PRIMARY KEY,
  table_name   VARCHAR(64)  NOT NULL,
  backup_table VARCHAR(64)  NOT NULL,
  migration    VARCHAR(191) NOT NULL DEFAULT '',
  created_at   DATETIME(3)  NOT NULL,
  expires_at   DATETIME(3)  NOT NULL,
  KEY idx_script_table_backups_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS script_relations (
  table_name  VARCHAR(64) NOT NULL,
  column_name VARCHAR(64) NOT NULL,
  ref_table   VARCHAR(64) NOT NULL,
  ref_column  VARCHAR(64) NOT NULL,
  PRIMARY KEY (table_name, column_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS ` + quoteIdent(d.DocsTable()) + docsDDL,
	}
	for _, s := range stmts {
		if _, err := d.admin.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// docsDDL is the body of a kv_docs table (db.docs).
const docsDDL = ` (
  collection VARCHAR(64) NOT NULL,
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  data       JSON        NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  KEY idx_kv_docs_collection (collection, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// viewSQL defines the core_* views over the CMS tables.
func (d *Data) viewSQL() map[string]string {
	c := quoteIdent(d.cmsDB) + "."
	return map[string]string{
		"core_posts": `SELECT id, type, title, slug, status, excerpt, author_id, featured_media_id, published_at, created_at, updated_at FROM ` +
			c + "`posts` WHERE status = 'publish'",
		"core_terms":      `SELECT id, taxonomy, name, slug, parent FROM ` + c + "`terms`",
		"core_post_terms": `SELECT pt.post_id, pt.term_id FROM ` + c + "`post_terms` pt JOIN " + c + "`posts` p ON p.id = pt.post_id WHERE p.status = 'publish'",
		"core_users":      `SELECT id, name, email, role, status, created_at FROM ` + c + "`users`",
	}
}

// RefreshViews (re)creates the read-only core_* views as the CMS user
// (SQL SECURITY DEFINER; TEMPTABLE so they are never updatable).
func (d *Data) RefreshViews() error {
	if !d.Separate() {
		return nil
	}
	return d.refreshViews()
}

func (d *Data) refreshViews() error {
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	for name, body := range d.viewSQL() {
		var typ string
		err := d.admin.QueryRowContext(ctx, `SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`,
			d.Database, name).Scan(&typ)
		if err == nil && typ == "BASE TABLE" { // someone replaced the view with a table
			if _, err := d.admin.ExecContext(ctx, "DROP TABLE "+quoteIdent(name)); err != nil {
				return err
			}
		}
		if _, err := d.admin.ExecContext(ctx, "CREATE OR REPLACE ALGORITHM=TEMPTABLE SQL SECURITY DEFINER VIEW "+quoteIdent(name)+" AS "+body); err != nil {
			return err
		}
	}
	return nil
}

// SetupSQL is what an administrator runs on an existing install to create
// the code database and user (docker-compose does it on a new volume).
func (d *Data) SetupSQL() []string {
	db := d.cmsDB
	if db == "" {
		db = "anasaki"
	}
	code := shortName(db + "_code")
	user := d.cmsUser
	if user == "" {
		user = "anasaki"
	}
	return []string{
		"CREATE DATABASE IF NOT EXISTS " + quoteIdent(code) + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;",
		"CREATE USER IF NOT EXISTS " + quoteString(code) + "@'%' IDENTIFIED BY '<password>';",
		"GRANT ALL PRIVILEGES ON " + quoteIdent(code) + ".* TO " + quoteString(code) + "@'%';",
		"GRANT ALL PRIVILEGES ON " + quoteIdent(code) + ".* TO " + quoteString(user) + "@'%';",
	}
}

// SetupDSN is an example CODE_DB_DSN for SetupSQL.
func (d *Data) SetupDSN() string {
	db := d.cmsDB
	if db == "" {
		db = "anasaki"
	}
	code := shortName(db + "_code")
	return code + ":<password>@tcp(mysql:3306)/" + code + "?parseTime=true&charset=utf8mb4&loc=UTC"
}

// ---------------------------------------------------------------- tables

// logical returns the developer-visible name of a physical table and
// whether it belongs to the code area at all.
func (d *Data) logical(phys string) (string, bool) {
	if d.N.Fallback {
		if !strings.HasPrefix(phys, "sc_") {
			return "", false
		}
		return strings.TrimPrefix(phys, "sc_"), true
	}
	return phys, true
}

// isDeveloperTable: not a view, bookkeeping, backup, kv_docs or a table of
// a JS plugin (p_<id>_*).
func isDeveloperTable(name string) bool {
	return name != "kv_docs" && !strings.HasPrefix(name, "script_") && !strings.HasPrefix(name, "bak_") &&
		!strings.HasPrefix(name, "core_") && !strings.HasPrefix(name, "p_")
}

// UnitNaming is the table naming of JS plugin id: p_<id>_ (dashes become
// underscores), sc_p_<id>_ in fallback mode.
func (d *Data) UnitNaming(id string, noCore bool) Naming {
	return Naming{Prefix: d.N.Prefix + UnitTablePrefix(id), Fallback: d.N.Fallback, Unit: id, NoCore: noCore}
}

// UnitTablePrefix is the table prefix of JS plugin id ("p_<id>_").
func UnitTablePrefix(id string) string { return "p_" + strings.ReplaceAll(id, "-", "_") + "_" }

// logicalFor is logical for the unit of n.
func (d *Data) logicalFor(n Naming, phys string) (string, bool) {
	if n.Unit == "" {
		l, ok := d.logical(phys)
		return l, ok && isDeveloperTable(l)
	}
	l, ok := strings.CutPrefix(phys, n.Prefix)
	if !ok || l == "" || l == "kv_docs" {
		return "", false
	}
	return l, true
}

// LoadSnapshot reads the developer tables from information_schema.
func (d *Data) LoadSnapshot(ctx context.Context) (*Snapshot, error) { return d.loadSnapshot(ctx, d.N) }

// loadSnapshot reads the tables of the unit named by n.
func (d *Data) loadSnapshot(ctx context.Context, n Naming) (*Snapshot, error) {
	snap := NewSnapshot()
	rows, err := d.admin.QueryContext(ctx, `SELECT TABLE_NAME, COALESCE(TABLE_COMMENT, '') FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'`, d.Database)
	if err != nil {
		return nil, err
	}
	phys := map[string]string{}
	for rows.Next() {
		var name, comment string
		if err := rows.Scan(&name, &comment); err != nil {
			rows.Close()
			return nil, err
		}
		if l, ok := d.logicalFor(n, name); ok {
			snap.Tables[l] = &STable{Name: l, Comment: comment}
			phys[name] = l
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(snap.Tables) == 0 {
		return snap, nil
	}
	rows, err = d.admin.QueryContext(ctx, `SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_KEY, EXTRA, COLUMN_COMMENT,
		COALESCE(COLLATION_NAME, '') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME, ORDINAL_POSITION`, d.Database)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var tn string
		var c SColumn
		var nullable string
		var def sql.NullString
		var coll string
		if err := rows.Scan(&tn, &c.Name, &c.Type, &nullable, &def, &c.Key, &c.Extra, &c.Comment, &coll); err != nil {
			rows.Close()
			return nil, err
		}
		t := snap.Tables[phys[tn]]
		if t == nil {
			continue
		}
		c.Type = strings.ToLower(c.Type)
		c.Nullable = nullable == "YES"
		if coll = strings.ToLower(coll); coll != "" && coll != "utf8mb4_unicode_ci" && Collations[coll] {
			c.Collation = coll
		}
		if def.Valid {
			s := def.String
			c.Default = &s
		}
		cc := c
		t.Columns = append(t.Columns, &cc)
	}
	rows.Close()
	rows, err = d.admin.QueryContext(ctx, `SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, COLUMN_NAME FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`, d.Database)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var tn, in string
		var nonUnique int
		var col sql.NullString
		if err := rows.Scan(&tn, &in, &nonUnique, &col); err != nil {
			rows.Close()
			return nil, err
		}
		t := snap.Tables[phys[tn]]
		if t == nil {
			continue
		}
		ix := t.index(in)
		if ix == nil {
			ix = &SIndex{Name: in, Unique: nonUnique == 0}
			t.Indexes = append(t.Indexes, ix)
		}
		if col.Valid {
			ix.Columns = append(ix.Columns, col.String)
		}
	}
	rows.Close()
	rows, err = d.admin.QueryContext(ctx, `SELECT k.TABLE_NAME, k.CONSTRAINT_NAME, k.COLUMN_NAME, k.REFERENCED_TABLE_NAME, k.REFERENCED_COLUMN_NAME, r.DELETE_RULE
		FROM information_schema.KEY_COLUMN_USAGE k JOIN information_schema.REFERENTIAL_CONSTRAINTS r
		  ON r.CONSTRAINT_SCHEMA = k.CONSTRAINT_SCHEMA AND r.CONSTRAINT_NAME = k.CONSTRAINT_NAME AND r.TABLE_NAME = k.TABLE_NAME
		WHERE k.TABLE_SCHEMA = ? AND k.REFERENCED_TABLE_NAME IS NOT NULL ORDER BY k.TABLE_NAME, k.ORDINAL_POSITION`, d.Database)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f SFK
		var tn, rule string
		if err := rows.Scan(&tn, &f.Name, &f.Column, &f.RefTable, &f.RefColumn, &rule); err != nil {
			rows.Close()
			return nil, err
		}
		t := snap.Tables[phys[tn]]
		if t == nil {
			continue
		}
		if l, ok := d.logicalFor(n, f.RefTable); ok {
			f.RefTable = l
		}
		f.OnDelete = strings.ToLower(rule)
		ff := f
		t.FKs = append(t.FKs, &ff)
	}
	rows.Close()
	rows, err = d.admin.QueryContext(ctx, `SELECT table_name, column_name, ref_table, ref_column FROM script_relations ORDER BY table_name, column_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tn string
		var r SRel
		if err := rows.Scan(&tn, &r.Column, &r.RefTable, &r.RefColumn); err != nil {
			return nil, err
		}
		rel := ""
		if n.Unit != "" {
			rel = UnitTablePrefix(n.Unit)
		}
		l, ok := strings.CutPrefix(tn, rel)
		if !ok {
			continue
		}
		if t := snap.Tables[l]; t != nil {
			rr := r
			t.Relations = append(t.Relations, &rr)
		}
	}
	return snap, rows.Err()
}

// ---------------------------------------------------------------- backups

// Backup copies a developer table to bak_<table>_<yyyymmddhhmmss> and
// records it (kept 7 days).
func (d *Data) Backup(ctx context.Context, table, migration string) (string, error) {
	return d.backupWith(ctx, d.N, table, migration)
}

// backupWith backs up a table of the unit named by n (a JS plugin's table
// "orders" is recorded as p_<id>_orders).
func (d *Data) backupWith(ctx context.Context, n Naming, table, migration string) (string, error) {
	ts := time.Now().UTC().Format("20060102150405")
	src := n.Phys(table)
	table = strings.TrimPrefix(src, d.N.Prefix)
	base := table
	maxLen := 64 - len(d.N.Prefix) - len("bak_") - 1 - len(ts) - 3
	if len(base) > maxLen {
		base = base[:maxLen]
	}
	var name string
	for i := 1; i < 100; i++ {
		name = "bak_" + base + "_" + ts
		if i > 1 {
			name += fmt.Sprintf("_%d", i)
		}
		_, err := d.admin.ExecContext(ctx, "CREATE TABLE "+quoteIdent(d.N.Prefix+name)+" LIKE "+quoteIdent(src))
		if err == nil {
			break
		}
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1050 { // exists
			continue
		}
		return "", err
	}
	if _, err := d.admin.ExecContext(ctx, "INSERT INTO "+quoteIdent(d.N.Prefix+name)+" SELECT * FROM "+quoteIdent(src)); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if _, err := d.admin.ExecContext(ctx, `INSERT INTO script_table_backups (table_name, backup_table, migration, created_at, expires_at) VALUES (?,?,?,?,?)`,
		table, name, migration, now, now.AddDate(0, 0, backupDays)); err != nil {
		return "", err
	}
	return name, nil
}

// CleanupBackups drops expired backup copies.
func (d *Data) CleanupBackups() error {
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	rows, err := d.admin.QueryContext(ctx, `SELECT id, backup_table FROM script_table_backups WHERE expires_at < ?`, time.Now().UTC())
	if err != nil {
		return err
	}
	type bk struct {
		id   int64
		name string
	}
	var list []bk
	for rows.Next() {
		var b bk
		if err := rows.Scan(&b.id, &b.name); err != nil {
			rows.Close()
			return err
		}
		list = append(list, b)
	}
	rows.Close()
	for _, b := range list {
		if err := d.dropBackup(ctx, b.id, b.name); err != nil {
			return err
		}
	}
	return nil
}

func (d *Data) dropBackup(ctx context.Context, id int64, name string) error {
	if !strings.HasPrefix(name, "bak_") {
		return fmt.Errorf("invalid backup name %q", name)
	}
	if _, err := d.admin.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoteIdent(d.N.Prefix+name)); err != nil {
		return err
	}
	_, err := d.admin.ExecContext(ctx, `DELETE FROM script_table_backups WHERE id = ?`, id)
	return err
}

func (d *Data) cleanupLoop() {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-t.C:
			if err := d.CleanupBackups(); err != nil {
				log.Printf("site code: backup cleanup: %v", err)
			}
		}
	}
}
