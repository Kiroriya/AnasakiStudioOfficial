package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// replaceHref swaps every "href" (designs) or "url" (menu items) equal to
// from with to, in place; it reports whether anything changed.
func replaceHref(v any, from, to string, key string) bool {
	changed := false
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, ok := val.(string); ok && k == key && s == from {
				x[k] = to
				changed = true
				continue
			}
			if replaceHref(val, from, to, key) {
				changed = true
			}
		}
	case []any:
		for _, it := range x {
			if replaceHref(it, from, to, key) {
				changed = true
			}
		}
	}
	return changed
}

// designRow is a stored design (post design, component node or menu items)
// with what a revision of it needs.
type designRow struct {
	id             int64
	raw            []byte
	title, content string // post title/content, component/menu name
}

// designQueries select the designs of each table as designRows.
var designQueries = map[string]string{
	"posts":      `SELECT id, design, title, COALESCE(content,'') FROM posts WHERE design IS NOT NULL`,
	"components": `SELECT id, node, name, '' FROM components WHERE node IS NOT NULL`,
	"menus":      `SELECT id, items, name, '' FROM menus WHERE items IS NOT NULL`,
}

func collectDesigns(tx *sql.Tx, q string, args ...any) ([]designRow, error) {
	rs, err := tx.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []designRow
	for rs.Next() {
		var r designRow
		if err := rs.Scan(&r.id, &r.raw, &r.title, &r.content); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

// rewriteDesign stores b as the new design of r (a row of table) on behalf
// of editorID: the previous version becomes a revision (posts, components)
// and updated_at moves on, so a designer still holding the old version gets
// a conflict instead of silently saving over the change.
func rewriteDesign(tx *sql.Tx, table string, r designRow, b []byte, editorID int64) error {
	now := time.Now().UTC()
	switch table {
	case "posts":
		if err := keepPostRevision(tx, r.id, r.title, r.content, r.raw, editorID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE posts SET design=?, updated_at=`+bumpedAt+` WHERE id=?`, string(b), now, r.id)
		return err
	case "components":
		if err := keepComponentRevision(tx, r.id, r.title, r.raw, editorID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE components SET node=?, updated_at=`+bumpedAt+` WHERE id=?`, string(b), now, r.id)
		return err
	default:
		_, err := tx.Exec(`UPDATE menus SET items=?, updated_at=`+bumpedAt+` WHERE id=?`, string(b), now, r.id)
		return err
	}
}

// ReplaceLinks points every link to address from (a site path like
// "/old-page") at to instead: links in page/post designs, components and
// menus, on behalf of editorID. It returns how many designs/menus changed.
func (s *Store) ReplaceLinks(from, to string, editorID int64) (int, error) {
	if from == "" || from == to {
		return 0, nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, t := range []struct{ table, key string }{{"posts", "href"}, {"components", "href"}, {"menus", "url"}} {
		list, err := collectDesigns(tx, designQueries[t.table])
		if err != nil {
			return 0, err
		}
		for _, r := range list {
			var v any
			if json.Unmarshal(r.raw, &v) != nil || !replaceHref(v, from, to, t.key) {
				continue
			}
			b, err := json.Marshal(v)
			if err != nil {
				continue
			}
			if err := rewriteDesign(tx, t.table, r, b, editorID); err != nil {
				return 0, err
			}
			n++
		}
	}
	return n, tx.Commit()
}
