package legacy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// Migration statuses (legacy_migrations.status).
const (
	StatusPending    = "pending"
	StatusInstalling = "installing"
	StatusImporting  = "importing"
	StatusDone       = "done"
	StatusFailed     = "failed"
)

// ImportExport is the code.exports function of an official plugin that takes
// the old rows over.
const ImportExport = "legacyImport"

// OfficialAuthor is the manifest author of official plugins (plugin.OfficialAuthor).
const OfficialAuthor = plugin.OfficialAuthor

// ApprovedBy is recorded as the approver of the permissions of a migrated plugin.
const ApprovedBy = "system migration"

const schema = `CREATE TABLE IF NOT EXISTS legacy_migrations (
  id              VARCHAR(64)  NOT NULL PRIMARY KEY,
  status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
  error           TEXT         NULL,
  attempts        INT          NOT NULL DEFAULT 0,
  was_active      BOOLEAN      NOT NULL DEFAULT FALSE,
  had_data        BOOLEAN      NOT NULL DEFAULT FALSE,
  settings_copied BOOLEAN      NOT NULL DEFAULT FALSE,
  cursor_table    INT          NOT NULL DEFAULT 0,
  cursor_offset   BIGINT       NOT NULL DEFAULT 0,
  imported        BIGINT       NOT NULL DEFAULT 0,
  source          VARCHAR(10)  NOT NULL DEFAULT '',
  version         VARCHAR(64)  NOT NULL DEFAULT '',
  approved_by     VARCHAR(64)  NOT NULL DEFAULT '',
  approved_at     DATETIME     NULL,
  approved        JSON         NULL,
  next_attempt_at DATETIME     NULL,
  created_at      DATETIME     NOT NULL,
  updated_at      DATETIME     NOT NULL,
  done_at         DATETIME     NULL,
  cleaned_at      DATETIME     NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// Runtime runs a JS plugin's export (the Site Code engine).
type Runtime interface {
	HasExport(unit, name string) bool
	CallExport(unit, name string, arg any, timeout time.Duration) (any, error)
}

// Options tune a Migrator (zero values: production defaults).
type Options struct {
	// Removed reports whether built-in id is gone from this build (default:
	// not compiled in). Only removed built-ins are migrated.
	Removed func(id string) bool
	// Now is the clock (tests).
	Now func() time.Time
	// Backoff is the wait before attempt n+1 after n failures (default
	// 1 min · 2^(n-1), at most 1 hour).
	Backoff func(attempts int) time.Duration
	// CallTimeout bounds one legacyImport call (default 60 s).
	CallTimeout time.Duration
	// InboxDir holds official zips uploaded by an administrator (default
	// $PLUGINS_DIR/.legacy-inbox).
	InboxDir string
	// Every is the interval of the background loop (default 1 min).
	Every time.Duration
}

// Migrator moves the data of removed built-ins to their official plugins.
type Migrator struct {
	db   *sql.DB
	pm   *plugin.Manager
	rt   Runtime
	opts Options

	runMu   sync.Mutex // one run at a time in this process
	stateMu sync.Mutex
	running bool
	next    chan struct{} // closed when the next run ends (Kick)
	kick    chan struct{}
}

// New creates the legacy_migrations table and returns a Migrator.
func New(db *sql.DB, pm *plugin.Manager, rt Runtime, opts Options) (*Migrator, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("legacy_migrations: %w", err)
	}
	if opts.Removed == nil {
		opts.Removed = func(id string) bool { return !pm.IsBuiltin(id) }
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Backoff == nil {
		opts.Backoff = DefaultBackoff
	}
	if opts.CallTimeout <= 0 {
		opts.CallTimeout = 60 * time.Second
	}
	if opts.InboxDir == "" {
		root := os.Getenv("PLUGINS_DIR")
		if root == "" {
			root = "./plugins-data"
		}
		opts.InboxDir = filepath.Join(root, ".legacy-inbox")
	}
	if opts.Every <= 0 {
		opts.Every = time.Minute
	}
	return &Migrator{db: db, pm: pm, rt: rt, opts: opts, kick: make(chan struct{}, 1)}, nil
}

// DefaultBackoff is 1 min, 2 min, 4 min … capped at one hour.
func DefaultBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 7 {
		return time.Hour
	}
	return min(time.Minute<<(attempts-1), time.Hour)
}

func (m *Migrator) now() time.Time { return m.opts.Now().UTC().Truncate(time.Second) }

// ---------------------------------------------------------------- state

// Item is a row of GET /api/v1/legacy-migrations.
type Item struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	NameEn        string     `json:"nameEn"`
	Icon          string     `json:"icon"`
	Status        string     `json:"status"`
	Error         string     `json:"error"`
	Attempts      int        `json:"attempts"`
	WasActive     bool       `json:"wasActive"`
	HadData       bool       `json:"hadData"`
	Imported      int64      `json:"imported"`
	Source        string     `json:"source"`  // store | upload
	Version       string     `json:"version"` // of the installed official plugin
	ApprovedBy    string     `json:"approvedBy"`
	ApprovedAt    *time.Time `json:"approvedAt"`
	Approved      *Approved  `json:"approved"`
	NextAttemptAt *time.Time `json:"nextAttemptAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	DoneAt        *time.Time `json:"doneAt"`
	CleanedAt     *time.Time `json:"cleanedAt"`
	// Cleanable: done, not cleaned yet, and old tables/options remain.
	Cleanable bool `json:"cleanable"`
}

// Approved is what the migration approved on the administrator's behalf.
type Approved struct {
	Permissions []string `json:"permissions"`
	Scopes      []string `json:"scopes"`
	Hosts       []string `json:"hosts"`
}

// Summary is the banner state.
type Summary struct {
	Waiting int    `json:"waiting"` // not done yet
	Running bool   `json:"running"`
	Items   []Item `json:"items"`
}

type row struct {
	id                                 string
	status, errMsg                     string
	attempts                           int
	wasActive, hadData, settingsCopied bool
	cursorTable                        int
	cursorOffset, imported             int64
	source, version                    string
	nextAttempt, doneAt, cleanedAt     sql.NullTime
	approvedBy                         string
	approvedAt                         sql.NullTime
	approved                           []byte
	updatedAt                          time.Time
}

const rowCols = `id, status, COALESCE(error, ''), attempts, was_active, had_data, settings_copied, cursor_table, cursor_offset,
	imported, source, version, next_attempt_at, done_at, cleaned_at, approved_by, approved_at, approved, updated_at`

func scanRow(sc interface{ Scan(...any) error }) (*row, error) {
	r := &row{}
	err := sc.Scan(&r.id, &r.status, &r.errMsg, &r.attempts, &r.wasActive, &r.hadData, &r.settingsCopied, &r.cursorTable,
		&r.cursorOffset, &r.imported, &r.source, &r.version, &r.nextAttempt, &r.doneAt, &r.cleanedAt, &r.approvedBy,
		&r.approvedAt, &r.approved, &r.updatedAt)
	return r, err
}

func (m *Migrator) rows() ([]*row, error) {
	rs, err := m.db.Query(`SELECT ` + rowCols + ` FROM legacy_migrations`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	byID := map[string]*row{}
	for rs.Next() {
		r, err := scanRow(rs)
		if err != nil {
			return nil, err
		}
		byID[r.id] = r
	}
	if err := rs.Err(); err != nil {
		return nil, err
	}
	var out []*row // Builtins order
	for _, b := range Builtins {
		if r := byID[b.ID]; r != nil {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *Migrator) get(id string) (*row, error) {
	return scanRow(m.db.QueryRow(`SELECT `+rowCols+` FROM legacy_migrations WHERE id = ?`, id))
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

// List returns the migrations (Builtins order) and the banner count.
func (m *Migrator) List() (Summary, error) {
	rs, err := m.rows()
	if err != nil {
		return Summary{}, err
	}
	s := Summary{Items: []Item{}, Running: m.Running()}
	for _, r := range rs {
		b := Get(r.id)
		it := Item{ID: r.id, Name: b.Name, NameEn: b.NameEn, Icon: b.Icon, Status: r.status, Error: r.errMsg,
			Attempts: r.attempts, WasActive: r.wasActive, HadData: r.hadData, Imported: r.imported, Source: r.source,
			Version: r.version, ApprovedBy: r.approvedBy, ApprovedAt: timePtr(r.approvedAt), NextAttemptAt: timePtr(r.nextAttempt),
			UpdatedAt: r.updatedAt.UTC(), DoneAt: timePtr(r.doneAt), CleanedAt: timePtr(r.cleanedAt)}
		if len(r.approved) > 0 {
			var a Approved
			if json.Unmarshal(r.approved, &a) == nil {
				it.Approved = &a
			}
		}
		if r.status != StatusDone {
			s.Waiting++
		} else if !r.cleanedAt.Valid {
			if d, err := Detect(m.db, r.id); err == nil && d.Leftovers() {
				it.Cleanable = true
			}
		}
		s.Items = append(s.Items, it)
	}
	return s, nil
}

// ---------------------------------------------------------------- detection

// Detect records a pending migration for every removed built-in that was
// active or left data, unless one is already recorded (any status).
func (m *Migrator) Detect() ([]string, error) {
	var added []string
	for _, b := range Builtins {
		if !m.opts.Removed(b.ID) {
			continue
		}
		var n int
		if err := m.db.QueryRow(`SELECT COUNT(*) FROM legacy_migrations WHERE id = ?`, b.ID).Scan(&n); err != nil {
			return added, err
		}
		if n > 0 {
			continue
		}
		d, err := Detect(m.db, b.ID)
		if err != nil {
			return added, err
		}
		if !d.Active && !d.HasData() {
			continue
		}
		now := m.now()
		res, err := m.db.Exec(`INSERT IGNORE INTO legacy_migrations (id, status, was_active, had_data, created_at, updated_at)
			VALUES (?, 'pending', ?, ?, ?, ?)`, b.ID, d.Active, d.HasData(), now, now)
		if err != nil {
			return added, err
		}
		if k, _ := res.RowsAffected(); k > 0 {
			added = append(added, b.ID)
			log.Printf("legacy migration: %s queued (active=%v data=%v)", b.ID, d.Active, d.HasData())
		}
	}
	return added, nil
}

// ---------------------------------------------------------------- running

// Running reports a run in progress in this process.
func (m *Migrator) Running() bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.running
}

// Start runs the migration now and then every Options.Every (or when kicked)
// until ctx ends.
func (m *Migrator) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(m.opts.Every)
		defer t.Stop()
		for {
			m.RunOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-m.kick:
			}
		}
	}()
}

// Kick asks the background loop to run now and returns a channel closed when
// that run is over.
func (m *Migrator) Kick() <-chan struct{} {
	m.stateMu.Lock()
	if m.next == nil {
		m.next = make(chan struct{})
	}
	ch := m.next
	m.stateMu.Unlock()
	select {
	case m.kick <- struct{}{}:
	default:
	}
	return ch
}

// RunOnce detects new migrations and processes every due one. Several
// servers sharing a database are serialised by a MySQL named lock.
func (m *Migrator) RunOnce(ctx context.Context) {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	m.stateMu.Lock()
	m.running = true
	done := m.next
	m.next = nil
	m.stateMu.Unlock()
	defer func() {
		m.stateMu.Lock()
		m.running = false
		m.stateMu.Unlock()
		if done != nil {
			close(done)
		}
	}()

	conn, err := m.db.Conn(ctx)
	if err != nil {
		log.Printf("legacy migration: %v", err)
		return
	}
	defer conn.Close()
	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('anasaki_legacy_migration', 0)`).Scan(&got); err != nil || got.Int64 != 1 {
		return // another server is migrating
	}
	defer conn.ExecContext(context.Background(), `SELECT RELEASE_LOCK('anasaki_legacy_migration')`) //nolint:errcheck

	if _, err := m.Detect(); err != nil {
		log.Printf("legacy migration: detect: %v", err)
	}
	rs, err := m.rows()
	if err != nil {
		log.Printf("legacy migration: %v", err)
		return
	}
	now := m.now()
	for _, r := range rs {
		if ctx.Err() != nil {
			return
		}
		if r.status == StatusDone || (r.status == StatusFailed && r.nextAttempt.Valid && r.nextAttempt.Time.After(now)) {
			continue
		}
		if !m.opts.Removed(r.id) {
			continue // the built-in is (back) in this build: leave it alone
		}
		if err := m.process(r); err != nil {
			m.fail(r, err)
		}
	}
}

// Retry makes failed migrations due now (id "" = all) and kicks a run.
func (m *Migrator) Retry(id string) (<-chan struct{}, error) {
	q := `UPDATE legacy_migrations SET next_attempt_at = NULL WHERE status <> 'done'`
	args := []any{}
	if id != "" {
		q += ` AND id = ?`
		args = append(args, id)
	}
	if _, err := m.db.Exec(q, args...); err != nil {
		return nil, err
	}
	return m.Kick(), nil
}

func (m *Migrator) setStatus(id, status string) error {
	_, err := m.db.Exec(`UPDATE legacy_migrations SET status = ?, updated_at = ? WHERE id = ?`, status, m.now(), id)
	return err
}

func (m *Migrator) fail(r *row, err error) {
	attempts := r.attempts + 1
	next := m.now().Add(m.opts.Backoff(attempts))
	msg := err.Error()
	var se *plugin.StoreError
	if errors.Is(err, plugin.ErrStoreUnavailable) || (errors.As(err, &se) && se.Status >= 500) {
		msg = "store unreachable: " + msg
	}
	if len(msg) > 2000 {
		msg = msg[:2000]
	}
	log.Printf("legacy migration: %s failed (attempt %d, next %s): %s", r.id, attempts, next.Format(time.RFC3339), msg)
	if _, e := m.db.Exec(`UPDATE legacy_migrations SET status = 'failed', error = ?, attempts = ?, next_attempt_at = ?, updated_at = ?
		WHERE id = ?`, msg, attempts, next, m.now(), r.id); e != nil {
		log.Printf("legacy migration: %s: %v", r.id, e)
	}
}

// process runs one migration from where it stopped: install + activate
// (consent approved as "system migration"), settings, rows, done.
func (m *Migrator) process(r *row) error {
	b := Get(r.id)
	if b == nil {
		return fmt.Errorf("unknown built-in %q", r.id)
	}
	if err := m.setStatus(r.id, StatusInstalling); err != nil {
		return err
	}
	inst, err := m.ensureInstalled(r)
	if err != nil {
		return err
	}
	settings, hasSettings, err := Settings(m.db, r.id)
	if err != nil {
		return err
	}
	copySettings := func() error {
		if r.settingsCopied || !hasSettings {
			return nil
		}
		if _, err := m.pm.ImportPackageSettings(r.id, settings); err != nil {
			return fmt.Errorf("settings: %w", err)
		}
		r.settingsCopied = true
		_, err := m.db.Exec(`UPDATE legacy_migrations SET settings_copied = TRUE WHERE id = ?`, r.id)
		return err
	}
	// an active built-in: settings first, so the plugin starts with them
	if r.wasActive {
		if err := copySettings(); err != nil {
			return err
		}
	}
	if !inst.Active {
		if inst, err = m.pm.SetPackageActive(r.id, true); err != nil {
			return fmt.Errorf("activate: %w", err)
		}
		if err := m.recordConsent(r.id, inst); err != nil {
			return err
		}
	}
	if err := m.setStatus(r.id, StatusImporting); err != nil {
		return err
	}
	if err := m.importRows(r, b); err != nil {
		return err
	}
	// an inactive built-in with data: the plugin was only switched on to take
	// the rows; it ends up off like before, with its settings
	if !r.wasActive {
		if _, err := m.pm.SetPackageActive(r.id, false); err != nil {
			return fmt.Errorf("deactivate: %w", err)
		}
		if err := copySettings(); err != nil {
			return err
		}
	}
	now := m.now()
	_, err = m.db.Exec(`UPDATE legacy_migrations SET status = 'done', error = NULL, next_attempt_at = NULL, done_at = ?, updated_at = ?
		WHERE id = ?`, now, now, r.id)
	if err == nil {
		log.Printf("legacy migration: %s done (%d rows)", r.id, r.imported)
	}
	return err
}

// recordConsent stores what activation approved on the administrator's behalf.
func (m *Migrator) recordConsent(id string, inst plugin.Installed) error {
	a := Approved{Permissions: nonNil(inst.Permissions), Scopes: []string{}, Hosts: []string{}}
	if inst.Code != nil {
		a.Scopes, a.Hosts = nonNil(inst.Code.Scopes), nonNil(inst.Code.Hosts)
	}
	b, _ := json.Marshal(a)
	_, err := m.db.Exec(`UPDATE legacy_migrations SET approved_by = ?, approved_at = ?, approved = ? WHERE id = ?`,
		ApprovedBy, m.now(), string(b), id)
	return err
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// VerifyOfficial checks that a package is the official replacement of
// built-in id (validated like any install: ParsePackage).
func VerifyOfficial(data []byte, id string) (*plugin.Manifest, error) {
	mf, _, err := plugin.ParsePackage(data)
	if err != nil {
		return nil, err
	}
	if err := checkManifest(mf.ID, mf.Legacy, mf.Author, mf.Runtime, id); err != nil {
		return nil, err
	}
	return mf, nil
}

func checkManifest(pkgID, legacyID, author, runtime, want string) error {
	if want != "" && pkgID != want {
		return &plugin.PackageError{Msg: fmt.Sprintf("this is plugin %q, not the official %q", pkgID, want)}
	}
	if Get(pkgID) == nil || legacyID != pkgID {
		return &plugin.PackageError{Msg: fmt.Sprintf("%q is not an official replacement of a former built-in plugin (manifest \"legacy\")", pkgID)}
	}
	if author != OfficialAuthor || runtime != plugin.RuntimeJS {
		return &plugin.PackageError{Msg: fmt.Sprintf("%q is not an official Anasaki Studio JS plugin", pkgID)}
	}
	return nil
}

// ensureInstalled makes sure the official plugin is installed: already there
// (any source), an uploaded zip in the inbox, or a download from the store.
func (m *Migrator) ensureInstalled(r *row) (plugin.Installed, error) {
	if inst, err := m.pm.GetPackage(r.id); err == nil {
		if err := checkManifest(inst.ID, inst.Legacy, inst.Author, inst.Runtime, r.id); err != nil {
			return inst, err
		}
		return inst, m.noteSource(r.id, inst.Source, inst.Version)
	}
	source := "upload"
	inbox := m.inboxPath(r.id)
	data, err := os.ReadFile(inbox)
	if err != nil {
		source = "store"
		if data, err = m.pm.DownloadFromStore(r.id, ""); err != nil {
			return plugin.Installed{}, err
		}
	}
	if _, err := VerifyOfficial(data, r.id); err != nil {
		if source == "upload" {
			_ = os.Remove(inbox)
		}
		return plugin.Installed{}, err
	}
	inst, err := m.pm.InstallPackage(data, source, r.id)
	if err != nil {
		return inst, err
	}
	if source == "upload" {
		_ = os.Remove(inbox)
	}
	log.Printf("legacy migration: %s %s installed from %s", r.id, inst.Version, source)
	return inst, m.noteSource(r.id, source, inst.Version)
}

func (m *Migrator) noteSource(id, source, version string) error {
	_, err := m.db.Exec(`UPDATE legacy_migrations SET source = IF(source = '', ?, source), version = ? WHERE id = ?`, source, version, id)
	return err
}

// importRows streams the old tables to legacyImport in batches, saving the
// position after each batch (a retry resumes there; the plugin upserts, so a
// repeated batch is harmless).
func (m *Migrator) importRows(r *row, b *Builtin) error {
	if len(b.Tables) == 0 {
		return nil
	}
	total := int64(0)
	for _, t := range b.Tables {
		ok, err := TableExists(m.db, t.Name)
		if err != nil {
			return err
		}
		if ok {
			var n int64
			if err := m.db.QueryRow("SELECT COUNT(*) FROM `" + t.Name + "`").Scan(&n); err != nil {
				return err
			}
			total += n
		}
	}
	if total == 0 && r.imported == 0 {
		return nil // nothing to hand over
	}
	if !m.rt.HasExport(r.id, ImportExport) {
		return fmt.Errorf("plugin %s does not provide %s (code.exports) or is not running", r.id, ImportExport)
	}
	for ti := r.cursorTable; ti < len(b.Tables); ti++ {
		t := b.Tables[ti]
		offset := int64(0)
		if ti == r.cursorTable {
			offset = r.cursorOffset
		}
		for {
			rows, err := ReadRows(m.db, t, offset, MaxBatch)
			if err != nil {
				return fmt.Errorf("read %s: %w", t.Name, err)
			}
			last := len(rows) < MaxBatch
			if len(rows) == MaxBatch { // exactly a full page: peek for more
				more, err := ReadRows(m.db, t, offset+MaxBatch, 1)
				if err != nil {
					return err
				}
				last = len(more) == 0
			}
			if len(rows) > 0 || offset == 0 {
				res, err := m.rt.CallExport(r.id, ImportExport, map[string]any{"kind": "rows", "table": t.Name, "rows": rows, "last": last},
					m.opts.CallTimeout)
				if err != nil {
					return fmt.Errorf("%s(%s): %w", ImportExport, t.Name, err)
				}
				n := importedCount(res, len(rows))
				r.imported += n
			}
			offset += int64(len(rows))
			nextTable, nextOffset := ti, offset
			if last {
				nextTable, nextOffset = ti+1, 0
			}
			if _, err := m.db.Exec(`UPDATE legacy_migrations SET cursor_table = ?, cursor_offset = ?, imported = ?, updated_at = ? WHERE id = ?`,
				nextTable, nextOffset, r.imported, m.now(), r.id); err != nil {
				return err
			}
			r.cursorTable, r.cursorOffset = nextTable, nextOffset
			if last {
				break
			}
		}
	}
	return nil
}

// importedCount reads {imported: n} (falls back to the batch size).
func importedCount(res any, fallback int) int64 {
	if o, ok := res.(map[string]any); ok {
		if f, ok := o["imported"].(float64); ok && f >= 0 {
			return int64(f)
		}
	}
	return int64(fallback)
}

// ---------------------------------------------------------------- manual upload and clean-up

func (m *Migrator) inboxPath(id string) string { return filepath.Join(m.opts.InboxDir, id+".zip") }

// ErrNotWaiting: no unfinished migration for that plugin.
var ErrNotWaiting = errors.New("no migration is waiting for this plugin")

// AcceptZip takes an official zip uploaded by an administrator (air-gapped
// sites): it must be the official package of a migration that is not done;
// it is kept in the inbox and installed by the next run (kicked now).
func (m *Migrator) AcceptZip(data []byte) (string, <-chan struct{}, error) {
	mf, _, err := plugin.ParsePackage(data)
	if err != nil {
		return "", nil, err
	}
	if err := checkManifest(mf.ID, mf.Legacy, mf.Author, mf.Runtime, ""); err != nil {
		return "", nil, err
	}
	r, err := m.get(mf.ID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && r.status == StatusDone) {
		return mf.ID, nil, ErrNotWaiting
	}
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(m.opts.InboxDir, 0o755); err != nil {
		return "", nil, err
	}
	tmp := m.inboxPath(mf.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", nil, err
	}
	if err := os.Rename(tmp, m.inboxPath(mf.ID)); err != nil {
		return "", nil, err
	}
	if _, err := m.db.Exec(`UPDATE legacy_migrations SET next_attempt_at = NULL WHERE id = ?`, mf.ID); err != nil {
		return "", nil, err
	}
	return mf.ID, m.Kick(), nil
}

// ErrNotDone: clean-up is only possible once the migration is done.
var ErrNotDone = errors.New("the migration of this plugin is not done")

// Cleanup drops the old tables and options of a migrated built-in
// (administrator's explicit request) and records when.
func (m *Migrator) Cleanup(id string) error {
	if Get(id) == nil {
		return ErrNotWaiting
	}
	r, err := m.get(id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotDone
	}
	if err != nil {
		return err
	}
	if r.status != StatusDone {
		return ErrNotDone
	}
	if !m.opts.Removed(id) {
		return errors.New("the built-in plugin is still part of this build")
	}
	if err := DropOld(m.db, id); err != nil {
		return err
	}
	now := m.now()
	_, err = m.db.Exec(`UPDATE legacy_migrations SET cleaned_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	if err == nil {
		log.Printf("legacy migration: %s old data cleaned up", id)
	}
	return err
}

// Waiting reports whether id's migration is recorded and not done (used by
// the plugin screens to explain why a feature is off).
func (m *Migrator) Waiting(id string) bool {
	var st string
	err := m.db.QueryRow(`SELECT status FROM legacy_migrations WHERE id = ?`, id).Scan(&st)
	return err == nil && st != StatusDone
}
