package store

import (
	"strings"
	"time"
	"unicode/utf8"
)

// AuditEntry is one line of the admin activity log (2.4.0).
type AuditEntry struct {
	ID       int64     `json:"id"`
	At       time.Time `json:"at"`
	UserID   int64     `json:"userId"`
	UserName string    `json:"userName"`
	Method   string    `json:"method"`
	Route    string    `json:"route"`
	Target   string    `json:"target"`
	Label    string    `json:"label"`
	Status   int       `json:"status"`
	IP       string    `json:"ip"`
	Via      string    `json:"via"` // session | token | login
}

// AuditFilter narrows ListAudit.
type AuditFilter struct {
	UserID int64
	Search string // in route, target and label
	Kind   string // route segment after /api/v1/ ("posts", "users", "code"…), or "auth"
	Failed bool   // only answers with status >= 400
	Limit  int
	Offset int
}

// auditKeepDays is how long the activity log keeps a line.
const auditKeepDays = 180

func clipTo(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// AddAudit records one line; now and then lines older than auditKeepDays go.
func (s *Store) AddAudit(e AuditEntry) error {
	res, err := s.DB.Exec(`INSERT INTO admin_audit (at, user_id, user_name, method, route, target, label, status, ip, via)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, time.Now().UTC(), e.UserID, clipTo(e.UserName, 191), clipTo(e.Method, 8), clipTo(e.Route, 191),
		clipTo(e.Target, 191), clipTo(e.Label, 255), e.Status, clipTo(e.IP, 64), clipTo(e.Via, 16))
	if err != nil {
		return err
	}
	if id, _ := res.LastInsertId(); id%500 == 0 {
		_, err = s.DB.Exec(`DELETE FROM admin_audit WHERE at < ?`, time.Now().UTC().AddDate(0, 0, -auditKeepDays))
	}
	return err
}

// ListAudit returns a page of the activity log, newest first, and the total.
func (s *Store) ListAudit(f AuditFilter) ([]AuditEntry, int, error) {
	where := []string{"1=1"}
	var args []any
	if f.UserID > 0 {
		where = append(where, "user_id = ?")
		args = append(args, f.UserID)
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		like := "%" + q + "%"
		where = append(where, "(route LIKE ? OR target LIKE ? OR label LIKE ? OR user_name LIKE ? OR ip LIKE ?)")
		args = append(args, like, like, like, like, like)
	}
	if k := strings.TrimSpace(f.Kind); k != "" {
		where = append(where, "route LIKE ?")
		args = append(args, "% /api/v1/"+k+"%")
	}
	if f.Failed {
		where = append(where, "status >= 400")
	}
	cond := " WHERE " + strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM admin_audit`+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	rows, err := s.DB.Query(`SELECT id, at, user_id, user_name, method, route, target, label, status, ip, via FROM admin_audit`+cond+
		` ORDER BY id DESC LIMIT ? OFFSET ?`, append(args, f.Limit, max(0, f.Offset))...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.At, &e.UserID, &e.UserName, &e.Method, &e.Route, &e.Target, &e.Label, &e.Status, &e.IP, &e.Via); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}
