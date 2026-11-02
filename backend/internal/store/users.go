package store

import (
	"database/sql"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/model"
)

const userCols = `id, email, name, first_name, last_name, COALESCE(bio, ''), website, avatar, role, status, locale,
	last_login_at, created_at, (SELECT COUNT(*) FROM posts p WHERE p.author_id = users.id AND p.type = 'post')`

func scanUser(sc interface{ Scan(...any) error }, u *model.User, extra ...any) error {
	var last sql.NullTime
	dst := []any{&u.ID, &u.Email, &u.Name, &u.FirstName, &u.LastName, &u.Bio, &u.Website, &u.Avatar,
		&u.Role, &u.Status, &u.Locale, &last, &u.CreatedAt, &u.PostCount}
	if err := sc.Scan(append(dst, extra...)...); err != nil {
		return err
	}
	u.LastLoginAt = nil
	if last.Valid {
		t := last.Time
		u.LastLoginAt = &t
	}
	return nil
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func defaults(u *model.User) {
	if u.Locale == "" {
		u.Locale = model.LocaleTH
	}
	if u.Status == "" {
		u.Status = model.UserActive
	}
}

func (s *Store) CreateUser(u *model.User, hash string) error {
	defaults(u)
	u.CreatedAt = time.Now().UTC().Truncate(time.Second)
	res, err := s.DB.Exec(`INSERT INTO users (email, name, first_name, last_name, bio, website, avatar, password_hash,
		role, status, locale, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.Email, u.Name, u.FirstName, u.LastName, u.Bio, u.Website, u.Avatar, hash, u.Role, u.Status, u.Locale, u.CreatedAt)
	if err != nil {
		return err
	}
	u.ID, _ = res.LastInsertId()
	return nil
}

// UserByEmail returns the user and its password hash.
func (s *Store) UserByEmail(email string) (*model.User, string, error) {
	var u model.User
	var hash string
	if err := scanUser(s.DB.QueryRow(`SELECT `+userCols+`, password_hash FROM users WHERE email = ?`, email), &u, &hash); err != nil {
		return nil, "", notFound(err)
	}
	return &u, hash, nil
}

// credCol is what sessions and tickets are bound to (auth stamp): the
// password hash and the session epoch, so a new password or "sign out
// everywhere" (BumpSessionEpoch) ends every session and ticket. It is not a
// bcrypt hash — check passwords with UserByEmail's hash.
const credCol = `CONCAT(password_hash, '|', session_epoch)`

// UserAuthByID returns the user and its session credential (credCol).
func (s *Store) UserAuthByID(id int64) (*model.User, string, error) {
	var u model.User
	var hash string
	if err := scanUser(s.DB.QueryRow(`SELECT `+userCols+`, `+credCol+` FROM users WHERE id = ?`, id), &u, &hash); err != nil {
		return nil, "", notFound(err)
	}
	return &u, hash, nil
}

// SessionUser returns what every authenticated request checks (id, name,
// role, status) and the session credential (credCol), by primary key only —
// no post count or other profile columns, since it runs on each API call.
func (s *Store) SessionUser(id int64) (*model.User, string, error) {
	u, hash, _, err := s.SessionCheck(id, "")
	return u, hash, err
}

// SessionCheck is SessionUser that also reports whether session token jti
// was revoked by a logout ("" = no token id: never revoked), in one query.
func (s *Store) SessionCheck(id int64, jti string) (*model.User, string, bool, error) {
	u := model.User{ID: id}
	var hash string
	var revoked bool
	if err := s.DB.QueryRow(`SELECT name, role, status, `+credCol+`,
		EXISTS(SELECT 1 FROM revoked_sessions WHERE jti = ?) FROM users WHERE id = ?`, jti, id).
		Scan(&u.Name, &u.Role, &u.Status, &hash, &revoked); err != nil {
		return nil, "", false, notFound(err)
	}
	return &u, hash, revoked && jti != "", nil
}

// RevokeSession records a logged-out session token until it expires (and
// drops expired entries).
func (s *Store) RevokeSession(jti string, expires time.Time) error {
	if jti == "" {
		return nil
	}
	if expires.IsZero() {
		expires = time.Now().Add(24 * time.Hour)
	}
	if _, err := s.DB.Exec(`DELETE FROM revoked_sessions WHERE expires_at < ?`, time.Now().UTC()); err != nil {
		return err
	}
	_, err := s.DB.Exec(`INSERT IGNORE INTO revoked_sessions (jti, expires_at) VALUES (?, ?)`, jti, expires.UTC())
	return err
}

// BumpSessionEpoch signs user id out everywhere: every session token and
// ticket issued before stops working (the credential changes).
func (s *Store) BumpSessionEpoch(id int64) error {
	_, err := s.DB.Exec(`UPDATE users SET session_epoch = session_epoch + 1 WHERE id = ?`, id)
	return err
}

func (s *Store) UserByID(id int64) (*model.User, error) {
	var u model.User
	if err := scanUser(s.DB.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id), &u); err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

// ListUsers returns one page of users matching f and the total match count.
func (s *Store) ListUsers(f model.UserFilter) ([]model.User, int, error) {
	where := []string{"1=1"}
	var args []any
	if q := strings.TrimSpace(f.Search); q != "" {
		like := "%" + q + "%"
		where = append(where, "(email LIKE ? OR name LIKE ? OR first_name LIKE ? OR last_name LIKE ?)")
		args = append(args, like, like, like, like)
	}
	if f.Role != "" {
		where = append(where, "role = ?")
		args = append(args, f.Role)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if f.Limit <= 0 {
		f.Limit = 1000
	}
	rows, err := s.DB.Query(`SELECT `+userCols+` FROM users WHERE `+cond+` ORDER BY id LIMIT ? OFFSET ?`,
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.User{}
	for rows.Next() {
		var u model.User
		if err := scanUser(rows, &u); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// UserCounts returns {"all": n, "suspended": n, "<role>": n…}.
func (s *Store) UserCounts() (map[string]int, error) {
	out := map[string]int{"all": 0, "suspended": 0}
	rows, err := s.DB.Query(`SELECT role, status, COUNT(*) FROM users GROUP BY role, status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var role, status string
		var n int
		if err := rows.Scan(&role, &status, &n); err != nil {
			return nil, err
		}
		out["all"] += n
		out[role] += n
		if status == model.UserSuspended {
			out["suspended"] += n
		}
	}
	return out, rows.Err()
}

// UpdateUser updates profile fields; hash is changed only when non-empty.
func (s *Store) UpdateUser(u *model.User, hash string) error {
	defaults(u)
	q := `UPDATE users SET email=?, name=?, first_name=?, last_name=?, bio=?, website=?, avatar=?, role=?, status=?, locale=?`
	args := []any{u.Email, u.Name, u.FirstName, u.LastName, u.Bio, u.Website, u.Avatar, u.Role, u.Status, u.Locale}
	if hash == "" {
		_, err := s.DB.Exec(q+` WHERE id=?`, append(args, u.ID)...)
		return err
	}
	// a new password ends every session (the stamp in the JWT) and revokes
	// the user's personal access tokens
	q += `, password_hash=?`
	args = append(args, hash)
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(q+` WHERE id=?`, append(args, u.ID)...); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM api_tokens WHERE user_id=?`, u.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetUsersRole changes the role of the given users.
func (s *Store) SetUsersRole(ids []int64, role string) (int64, error) {
	return s.updateUsers(ids, "role = ?", role)
}

// SetUsersStatus changes the status of the given users.
func (s *Store) SetUsersStatus(ids []int64, status string) (int64, error) {
	return s.updateUsers(ids, "status = ?", status)
}

func (s *Store) updateUsers(ids []int64, set string, v any) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	args := []any{v}
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := s.DB.Exec(`UPDATE users SET `+set+` WHERE id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// TouchLogin records a successful login.
func (s *Store) TouchLogin(id int64) error {
	_, err := s.DB.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, time.Now().UTC(), id)
	return err
}

// DeleteUser removes a user and reassigns their content, like WordPress'
// "attribute all content to" option.
func (s *Store) DeleteUser(id, reassignTo int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE posts SET author_id=? WHERE author_id=?`, reassignTo, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE media SET author_id=? WHERE author_id=?`, reassignTo, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM api_tokens WHERE user_id=?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
