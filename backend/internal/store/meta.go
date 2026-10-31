package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anasaki/studio/backend/internal/model"
)

// Custom fields of posts (post_meta): Site Code's site.postType / site.fields
// declare them, the admin editor and db.posts read and write them.

// attachMeta loads post_meta into each post's Meta ({} when none).
func (s *Store) attachMeta(posts []*model.Post) error {
	if len(posts) == 0 {
		return nil
	}
	idx := map[int64]*model.Post{}
	ph := make([]string, 0, len(posts))
	args := make([]any, 0, len(posts))
	for _, p := range posts {
		if p.Meta == nil {
			p.Meta = map[string]any{}
		}
		idx[p.ID] = p
		ph = append(ph, "?")
		args = append(args, p.ID)
	}
	rows, err := s.DB.Query(`SELECT post_id, meta_key, value FROM post_meta WHERE post_id IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var k string
		var raw sql.NullString
		if err := rows.Scan(&id, &k, &raw); err != nil {
			return err
		}
		p := idx[id]
		if p == nil {
			continue
		}
		var v any
		if raw.Valid {
			_ = json.Unmarshal([]byte(raw.String), &v)
		}
		p.Meta[k] = v
	}
	return rows.Err()
}

// PostMeta returns the custom fields of one post.
func (s *Store) PostMeta(id int64) (map[string]any, error) {
	p := &model.Post{ID: id}
	if err := s.attachMeta([]*model.Post{p}); err != nil {
		return nil, err
	}
	return p.Meta, nil
}

// SetPostMeta writes custom fields of a post: a nil value removes the key.
func (s *Store) SetPostMeta(id int64, meta map[string]any) error {
	if len(meta) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range meta {
		if v == nil {
			if _, err := tx.Exec(`DELETE FROM post_meta WHERE post_id = ? AND meta_key = ?`, id, k); err != nil {
				return err
			}
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("meta %s: %w", k, err)
		}
		if _, err := tx.Exec(`INSERT INTO post_meta (post_id, meta_key, value) VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE value = VALUES(value)`, id, k, string(b)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var metaOps = map[string]string{"=": "=", "!=": "=", "<": "<", "<=": "<=", ">": ">", ">=": ">="}

// metaWhere renders f.Meta as SQL conditions on posts p.
func metaWhere(conds []model.MetaCond) ([]string, []any, error) {
	var where []string
	var args []any
	jsonArg := func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	}
	for _, c := range conds {
		switch c.Op {
		case "in":
			list, _ := c.Value.([]any)
			if len(list) == 0 {
				where = append(where, "1 = 0")
				continue
			}
			parts := make([]string, 0, len(list))
			args = append(args, c.Key)
			for _, x := range list {
				j, err := jsonArg(x)
				if err != nil {
					return nil, nil, err
				}
				parts = append(parts, "pm.value = CAST(? AS JSON)")
				args = append(args, j)
			}
			where = append(where, `EXISTS (SELECT 1 FROM post_meta pm WHERE pm.post_id = p.id AND pm.meta_key = ? AND (`+
				strings.Join(parts, " OR ")+`))`)
		default:
			op, ok := metaOps[c.Op]
			if !ok {
				return nil, nil, fmt.Errorf("meta operator %q", c.Op)
			}
			j, err := jsonArg(c.Value)
			if err != nil {
				return nil, nil, err
			}
			cond := `EXISTS (SELECT 1 FROM post_meta pm WHERE pm.post_id = p.id AND pm.meta_key = ? AND pm.value ` + op + ` CAST(? AS JSON))`
			if c.Op == "!=" {
				cond = "NOT " + cond
			}
			where = append(where, cond)
			args = append(args, c.Key, j)
		}
	}
	return where, args, nil
}
