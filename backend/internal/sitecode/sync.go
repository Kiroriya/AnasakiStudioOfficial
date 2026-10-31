package sitecode

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Phase 5: tree + sync for tools (the anasaki-code CLI). GET /code/tree
// lists the draft with content hashes; POST /code/sync applies several
// writes and deletes at once, refusing all of them when a file changed on
// the server since the client's base hash.

// TreeFile is a draft file with the SHA-256 of its content (UTF-8, hex).
type TreeFile struct {
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	Size      int       `json:"size"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ContentHash is the hex SHA-256 of a file's content.
func ContentHash(content string) string {
	s := sha256.Sum256([]byte(content))
	return hex.EncodeToString(s[:])
}

// Tree lists the draft with content hashes, sorted by path.
func (s *DB) Tree() ([]TreeFile, error) {
	rows, err := s.db.Query(`SELECT path, content, updated_at FROM script_files ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TreeFile{}
	for rows.Next() {
		var f TreeFile
		var c string
		if err := rows.Scan(&f.Path, &c, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.SHA256, f.Size = ContentHash(c), len(c)
		out = append(out, f)
	}
	return out, rows.Err()
}

// SyncPut writes one file; BaseSHA256 (when not nil) is the hash the client
// last saw: "" = the file must not exist on the server.
type SyncPut struct {
	Path       string  `json:"path"`
	Content    *string `json:"content"`
	BaseSHA256 *string `json:"baseSha256"`
}

// SyncDelete removes one file (BaseSHA256 as in SyncPut).
type SyncDelete struct {
	Path       string  `json:"path"`
	BaseSHA256 *string `json:"baseSha256"`
}

// SyncConflict is a file whose server content differs from the base hash
// ("" = the file does not exist on the server).
type SyncConflict struct {
	Path         string `json:"path"`
	ServerSHA256 string `json:"serverSha256"`
}

// SyncConflicts is returned when a base hash does not match; nothing was
// applied.
type SyncConflicts struct{ List []SyncConflict }

func (c *SyncConflicts) Error() string { return fmt.Sprintf("%d conflict(s)", len(c.List)) }

// SyncPlan is what a sync changes.
type SyncPlan struct {
	Write  map[string]string
	Remove []string
	Result map[string]string // the draft afterwards
}

// CheckSyncRequest validates the paths of a sync request (before any
// database access): valid paths, content present, each path at most once.
func CheckSyncRequest(put []SyncPut, del []SyncDelete) error {
	if len(put)+len(del) > MaxFiles*2 {
		return perr("ส่งไฟล์มากเกินไปในครั้งเดียว (สูงสุด %d รายการ)", MaxFiles*2)
	}
	seen := map[string]bool{}
	for _, p := range put {
		if err := ValidPath(p.Path); err != nil {
			return err
		}
		if p.Content == nil {
			return perr("ไฟล์ %s: กรุณาส่ง content", p.Path)
		}
		if seen[p.Path] {
			return perr("ไฟล์ %s อยู่ในคำขอมากกว่าหนึ่งครั้ง", p.Path)
		}
		seen[p.Path] = true
	}
	for _, d := range del {
		if err := ValidPath(d.Path); err != nil {
			return err
		}
		if seen[d.Path] {
			return perr("ไฟล์ %s อยู่ในคำขอมากกว่าหนึ่งครั้ง", d.Path)
		}
		seen[d.Path] = true
	}
	return nil
}

// PlanSync checks base hashes against cur (the draft now) and computes the
// change. A conflict → *SyncConflicts; limits/paths → *PathError. Writing a
// file with unchanged content is skipped; deleting a missing file is a
// no-op.
func PlanSync(cur map[string]string, put []SyncPut, del []SyncDelete) (*SyncPlan, error) {
	if err := CheckSyncRequest(put, del); err != nil {
		return nil, err
	}
	serverHash := func(p string) string {
		if c, ok := cur[p]; ok {
			return ContentHash(c)
		}
		return ""
	}
	var conflicts []SyncConflict
	check := func(p string, base *string) {
		if base != nil && *base != serverHash(p) {
			conflicts = append(conflicts, SyncConflict{Path: p, ServerSHA256: serverHash(p)})
		}
	}
	for _, p := range put {
		check(p.Path, p.BaseSHA256)
	}
	for _, d := range del {
		check(d.Path, d.BaseSHA256)
	}
	if len(conflicts) > 0 {
		return nil, &SyncConflicts{List: conflicts}
	}
	plan := &SyncPlan{Write: map[string]string{}, Result: make(map[string]string, len(cur)+len(put))}
	for p, c := range cur {
		plan.Result[p] = c
	}
	for _, p := range put {
		if c, ok := cur[p.Path]; !ok || c != *p.Content {
			plan.Write[p.Path] = *p.Content
		}
		plan.Result[p.Path] = *p.Content
	}
	for _, d := range del {
		if _, ok := cur[d.Path]; ok {
			plan.Remove = append(plan.Remove, d.Path)
			delete(plan.Result, d.Path)
		}
	}
	if err := CheckSet(plan.Result); err != nil {
		return nil, err
	}
	return plan, nil
}

// Sync plans and (unless dryRun) applies a sync in one transaction, with
// the draft rows locked while the base hashes are compared.
func (s *DB) Sync(put []SyncPut, del []SyncDelete, dryRun bool, userID int64) (*SyncPlan, error) {
	if err := CheckSyncRequest(put, del); err != nil {
		return nil, err
	}
	var plan *SyncPlan
	err := s.inTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT path, content FROM script_files FOR UPDATE`)
		if err != nil {
			return err
		}
		cur := map[string]string{}
		for rows.Next() {
			var p, c string
			if err := rows.Scan(&p, &c); err != nil {
				rows.Close()
				return err
			}
			cur[p] = c
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if plan, err = PlanSync(cur, put, del); err != nil {
			return err
		}
		if dryRun {
			return nil
		}
		t := now()
		for _, p := range plan.Remove {
			if _, err := tx.Exec(`DELETE FROM script_files WHERE path = ?`, p); err != nil {
				return err
			}
		}
		for _, p := range Sorted(plan.Write) {
			if _, err := tx.Exec(`INSERT INTO script_files (path, content, updated_at, updated_by) VALUES (?,?,?,?)
				ON DUPLICATE KEY UPDATE content=VALUES(content), updated_at=VALUES(updated_at), updated_by=VALUES(updated_by)`,
				p, plan.Write[p], t, nullID(userID)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return plan, nil
}
