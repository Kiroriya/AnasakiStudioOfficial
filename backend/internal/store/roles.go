package store

import (
	"encoding/json"

	"github.com/anasaki/studio/backend/internal/model"
)

// ListRoles returns every role with its database capabilities and user count,
// built-in roles first.
func (s *Store) ListRoles() ([]model.Role, error) {
	rows, err := s.DB.Query(`SELECT r.slug, r.name, r.builtin, r.caps,
		(SELECT COUNT(*) FROM users u WHERE u.role = r.slug) FROM roles r ORDER BY r.builtin DESC, r.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Role{}
	for rows.Next() {
		var r model.Role
		var caps []byte
		if err := rows.Scan(&r.Slug, &r.Name, &r.Builtin, &caps, &r.UserCount); err != nil {
			return nil, err
		}
		r.Caps = []string{}
		_ = json.Unmarshal(caps, &r.Caps)
		out = append(out, r)
	}
	return out, rows.Err()
}

// RoleCaps returns slug → capabilities for every role (for the auth cache).
func (s *Store) RoleCaps() (map[string][]string, error) {
	rs, err := s.ListRoles()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(rs))
	for _, r := range rs {
		out[r.Slug] = r.Caps
	}
	return out, nil
}

func (s *Store) GetRole(slug string) (*model.Role, error) {
	rs, err := s.ListRoles()
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		if r.Slug == slug {
			return &r, nil
		}
	}
	return nil, ErrNotFound
}

// EnsureRole inserts a role when it does not exist yet (built-in seeding).
func (s *Store) EnsureRole(r *model.Role) error {
	caps, _ := json.Marshal(r.Caps)
	_, err := s.DB.Exec(`INSERT IGNORE INTO roles (slug, name, caps, builtin) VALUES (?,?,?,?)`, r.Slug, r.Name, string(caps), r.Builtin)
	return err
}

func (s *Store) CreateRole(r *model.Role) error {
	caps, _ := json.Marshal(r.Caps)
	_, err := s.DB.Exec(`INSERT INTO roles (slug, name, caps, builtin) VALUES (?,?,?,FALSE)`, r.Slug, r.Name, string(caps))
	return err
}

func (s *Store) UpdateRole(r *model.Role) error {
	caps, _ := json.Marshal(r.Caps)
	_, err := s.DB.Exec(`UPDATE roles SET name=?, caps=? WHERE slug=?`, r.Name, string(caps), r.Slug)
	return err
}

// DeleteRole removes a role and moves its users to fallback.
func (s *Store) DeleteRole(slug, fallback string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET role=? WHERE role=?`, fallback, slug); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM roles WHERE slug=?`, slug); err != nil {
		return err
	}
	return tx.Commit()
}
