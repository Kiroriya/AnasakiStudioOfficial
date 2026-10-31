package sitecode

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Option keys.
const (
	OptVersion  = "site_code_version"   // id of the live version ("" = none)
	OptSafeMode = "site_code_safe_mode" // "1" = Site Code switched off
)

// Retention.
const (
	keepVersions = 100
	keepLogs     = 10000
	logDays      = 7
)

// ErrNotFound is returned for a missing file or version.
var ErrNotFound = errors.New("not found")

// FileInfo is a draft file in the file list.
type FileInfo struct {
	Path      string    `json:"path"`
	Size      int       `json:"size"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy string    `json:"updatedBy"`
}

// File is a draft file with its content.
type File struct {
	Path      string    `json:"path"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Version summarises a published snapshot.
type Version struct {
	ID        int64     `json:"id"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy"`
	Live      bool      `json:"live"`
	Files     int       `json:"files"`
}

// VersionRow is a full snapshot as stored.
type VersionRow struct {
	Version
	FileMap   map[string]string
	Bundle    string
	SourceMap string
	Manifest  Manifest
}

// LogEntry is a script_logs row.
type LogEntry struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Source  string    `json:"source"`
	Message string    `json:"message"`
	Trace   *string   `json:"trace"`
	Version *int64    `json:"version"`
	// Unit is "site" (Site Code) or the id of the JS plugin that logged it.
	Unit string `json:"unit"`
}

// LogFilter selects log rows.
type LogFilter struct {
	Level, Source, Q string
	Unit             string // "" = every unit
	From, To         time.Time
	Limit, Offset    int
}

// DB wraps the Site Code tables.
type DB struct{ db *sql.DB }

func now() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }

// Draft returns every draft file.
func (s *DB) Draft() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT path, content FROM script_files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, c string
		if err := rows.Scan(&p, &c); err != nil {
			return nil, err
		}
		out[p] = c
	}
	return out, rows.Err()
}

// ListFiles lists the draft, sorted by path.
func (s *DB) ListFiles() ([]FileInfo, error) {
	rows, err := s.db.Query(`SELECT f.path, LENGTH(f.content), f.updated_at, COALESCE(u.name,'')
		FROM script_files f LEFT JOIN users u ON u.id = f.updated_by ORDER BY f.path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FileInfo{}
	for rows.Next() {
		var f FileInfo
		if err := rows.Scan(&f.Path, &f.Size, &f.UpdatedAt, &f.UpdatedBy); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetFile returns one draft file.
func (s *DB) GetFile(p string) (*File, error) {
	f := &File{}
	err := s.db.QueryRow(`SELECT path, content, updated_at FROM script_files WHERE path = ?`, p).Scan(&f.Path, &f.Content, &f.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return f, err
}

// PutFile writes a file; it is the caller's job to check conflicts.
func (s *DB) PutFile(p, content string, userID int64) (*File, error) {
	t := now()
	if _, err := s.db.Exec(`INSERT INTO script_files (path, content, updated_at, updated_by) VALUES (?,?,?,?)
		ON DUPLICATE KEY UPDATE content=VALUES(content), updated_at=VALUES(updated_at), updated_by=VALUES(updated_by)`,
		p, content, t, nullID(userID)); err != nil {
		return nil, err
	}
	return &File{Path: p, Content: content, UpdatedAt: t}, nil
}

// DeleteFiles removes draft files.
func (s *DB) DeleteFiles(paths []string) error {
	return s.inTx(func(tx *sql.Tx) error {
		for _, p := range paths {
			if _, err := tx.Exec(`DELETE FROM script_files WHERE path = ?`, p); err != nil {
				return err
			}
		}
		return nil
	})
}

// ApplyDraft writes changed files and removes others in one transaction.
func (s *DB) ApplyDraft(write map[string]string, remove []string, userID int64) error {
	t := now()
	return s.inTx(func(tx *sql.Tx) error {
		for _, p := range remove {
			if _, err := tx.Exec(`DELETE FROM script_files WHERE path = ?`, p); err != nil {
				return err
			}
		}
		for _, p := range Sorted(write) {
			if _, err := tx.Exec(`INSERT INTO script_files (path, content, updated_at, updated_by) VALUES (?,?,?,?)
				ON DUPLICATE KEY UPDATE content=VALUES(content), updated_at=VALUES(updated_at), updated_by=VALUES(updated_by)`,
				p, write[p], t, nullID(userID)); err != nil {
				return err
			}
		}
		return nil
	})
}

// MoveFiles renames paths (old → new) in one transaction.
func (s *DB) MoveFiles(moves map[string]string, userID int64) error {
	t := now()
	return s.inTx(func(tx *sql.Tx) error {
		for from, to := range moves {
			if _, err := tx.Exec(`UPDATE script_files SET path = ?, updated_at = ?, updated_by = ? WHERE path = ?`,
				to, t, nullID(userID), from); err != nil {
				return err
			}
		}
		return nil
	})
}

// InsertVersion stores a snapshot and prunes the oldest beyond 100.
func (s *DB) InsertVersion(message string, files map[string]string, b *BuildResult, m Manifest, userID int64) (int64, error) {
	fj, _ := json.Marshal(files)
	mj, _ := json.Marshal(m)
	res, err := s.db.Exec(`INSERT INTO script_versions (message, files, bundle, source_map, manifest, created_by, created_at)
		VALUES (?,?,?,?,?,?,?)`, clip(message, 500), string(fj), b.Bundle, b.SourceMap, string(mj), nullID(userID), time.Now().UTC())
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	_, err = s.db.Exec(`DELETE FROM script_versions WHERE id <= (SELECT id FROM
		(SELECT id FROM script_versions ORDER BY id DESC LIMIT 1 OFFSET ?) t)`, keepVersions)
	return id, err
}

const versionCols = `v.id, v.message, v.created_at, COALESCE(u.name,''), JSON_LENGTH(v.files)`

// ListVersions lists snapshots, newest first.
func (s *DB) ListVersions(live int64) ([]Version, error) {
	rows, err := s.db.Query(`SELECT ` + versionCols + ` FROM script_versions v
		LEFT JOIN users u ON u.id = v.created_by ORDER BY v.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		var v Version
		if err := rows.Scan(&v.ID, &v.Message, &v.CreatedAt, &v.CreatedBy, &v.Files); err != nil {
			return nil, err
		}
		v.Live = v.ID == live
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetVersion loads a snapshot (withCode adds the bundle and source map).
func (s *DB) GetVersion(id, live int64, withCode bool) (*VersionRow, error) {
	var v VersionRow
	var files string
	var bundle, sm, manifest sql.NullString
	cols := versionCols + `, v.files, NULL, NULL, v.manifest`
	if withCode {
		cols = versionCols + `, v.files, v.bundle, v.source_map, v.manifest`
	}
	err := s.db.QueryRow(`SELECT `+cols+` FROM script_versions v LEFT JOIN users u ON u.id = v.created_by WHERE v.id = ?`, id).
		Scan(&v.ID, &v.Message, &v.CreatedAt, &v.CreatedBy, &v.Files, &files, &bundle, &sm, &manifest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v.Live = v.ID == live
	v.FileMap = map[string]string{}
	_ = json.Unmarshal([]byte(files), &v.FileMap)
	v.Bundle, v.SourceMap = bundle.String, sm.String
	_ = json.Unmarshal([]byte(manifest.String), &v.Manifest)
	v.Manifest.normalize()
	return &v, nil
}

// InsertLogs writes log rows (one statement) and prunes now and then.
func (s *DB) InsertLogs(entries []LogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(`INSERT INTO script_logs (at, level, source, message, trace, version, unit) VALUES `)
	args := make([]any, 0, len(entries)*7)
	for i, e := range entries {
		if i > 0 {
			sb.WriteByte(',')
		}
		unit := e.Unit
		if unit == "" {
			unit = SiteUnit
		}
		sb.WriteString("(?,?,?,?,?,?,?)")
		args = append(args, e.At, e.Level, clip(e.Source, 191), clip(e.Message, 8000), e.Trace, e.Version, unit)
	}
	_, err := s.db.Exec(sb.String(), args...)
	return err
}

// PruneLogs keeps 7 days and the newest 10,000 rows.
func (s *DB) PruneLogs() error {
	if _, err := s.db.Exec(`DELETE FROM script_logs WHERE at < ?`, time.Now().UTC().AddDate(0, 0, -logDays)); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM script_logs WHERE id <= (SELECT id FROM
		(SELECT id FROM script_logs ORDER BY id DESC LIMIT 1 OFFSET ?) t)`, keepLogs)
	return err
}

func (f LogFilter) where() (string, []any) {
	var w []string
	var args []any
	if f.Level != "" {
		w = append(w, "level = ?")
		args = append(args, f.Level)
	}
	if f.Unit != "" {
		w = append(w, "unit = ?")
		args = append(args, f.Unit)
	}
	if f.Source != "" {
		w = append(w, "source LIKE ?")
		args = append(args, likeEscape(f.Source)+"%")
	}
	if f.Q != "" {
		w = append(w, "(message LIKE ? OR source LIKE ?)")
		q := "%" + likeEscape(f.Q) + "%"
		args = append(args, q, q)
	}
	if !f.From.IsZero() {
		w = append(w, "at >= ?")
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		w = append(w, "at < ?")
		args = append(args, f.To)
	}
	if len(w) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(w, " AND "), args
}

// Logs returns matching rows (newest first) and the total.
func (s *DB) Logs(f LogFilter) ([]LogEntry, int, error) {
	cond, args := f.where()
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM script_logs`+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT id, at, level, source, message, trace, version, unit FROM script_logs`+cond+
		` ORDER BY id DESC LIMIT ? OFFSET ?`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		var trace sql.NullString
		var ver sql.NullInt64
		if err := rows.Scan(&e.ID, &e.At, &e.Level, &e.Source, &e.Message, &trace, &ver, &e.Unit); err != nil {
			return nil, 0, err
		}
		if trace.Valid {
			e.Trace = &trace.String
		}
		if ver.Valid {
			e.Version = &ver.Int64
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ClearLogs removes every log row.
func (s *DB) ClearLogs() error {
	_, err := s.db.Exec(`DELETE FROM script_logs`)
	return err
}

// Audit records who did what.
func (s *DB) Audit(userID int64, action, detail string) error {
	_, err := s.db.Exec(`INSERT INTO script_audit (at, user_id, action, detail) VALUES (?,?,?,?)`,
		time.Now().UTC(), nullID(userID), action, clip(detail, 60000))
	return err
}

func (s *DB) inTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// clip cuts s to at most n characters (VARCHAR/TEXT limits).
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
