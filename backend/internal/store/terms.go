package store

import (
	"database/sql"
	"strings"

	"github.com/anasaki/studio/backend/internal/model"
)

// Terms of taxonomies: "category" (core) and, since 2.1.0, the taxonomies
// registered with site.taxonomy (hierarchical ones use Parent).

const termCols = `t.id, t.taxonomy, t.name, t.slug, t.parent, t.description`

func scanTerm(sc interface{ Scan(...any) error }, t *model.Term, count bool) error {
	dst := []any{&t.ID, &t.Taxonomy, &t.Name, &t.Slug, &t.Parent, &t.Description}
	if count {
		dst = append(dst, &t.Count)
	}
	return sc.Scan(dst...)
}

// ListTerms returns every term of a taxonomy with its post count, by name.
func (s *Store) ListTerms(taxonomy string) ([]model.Term, error) {
	return s.FindTerms(model.TermFilter{Taxonomy: taxonomy, Limit: 1000})
}

// FindTerms lists terms (with post counts) by the filter, by name.
func (s *Store) FindTerms(f model.TermFilter) ([]model.Term, error) {
	where := []string{"t.taxonomy = ?"}
	args := []any{f.Taxonomy}
	if f.Parent != nil {
		where = append(where, "t.parent = ?")
		args = append(args, *f.Parent)
	}
	if f.Search != "" {
		where = append(where, "(t.name LIKE ? OR t.slug LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like)
	}
	if f.PostID != 0 {
		where = append(where, "t.id IN (SELECT term_id FROM post_terms WHERE post_id = ?)")
		args = append(args, f.PostID)
	}
	if f.IDs != nil {
		if len(f.IDs) == 0 {
			return []model.Term{}, nil
		}
		where = append(where, "t.id IN ("+placeholders(len(f.IDs))+")")
		for _, id := range f.IDs {
			args = append(args, id)
		}
	}
	if f.Slugs != nil {
		if len(f.Slugs) == 0 {
			return []model.Term{}, nil
		}
		where = append(where, "t.slug IN ("+placeholders(len(f.Slugs))+")")
		for _, sl := range f.Slugs {
			args = append(args, sl)
		}
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.DB.Query(`SELECT `+termCols+`, COUNT(pt.post_id) FROM terms t
		LEFT JOIN post_terms pt ON pt.term_id = t.id WHERE `+strings.Join(where, " AND ")+`
		GROUP BY t.id, t.taxonomy, t.name, t.slug, t.parent, t.description ORDER BY t.name LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Term{}
	for rows.Next() {
		var t model.Term
		if err := scanTerm(rows, &t, true); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) TermBySlug(taxonomy, slug string) (*model.Term, error) {
	var t model.Term
	if err := scanTerm(s.DB.QueryRow(`SELECT `+termCols+` FROM terms t WHERE t.taxonomy=? AND t.slug=?`, taxonomy, slug), &t, false); err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

// GetTerm returns one term by id.
func (s *Store) GetTerm(id int64) (*model.Term, error) {
	var t model.Term
	if err := scanTerm(s.DB.QueryRow(`SELECT `+termCols+` FROM terms t WHERE t.id=?`, id), &t, false); err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

func (s *Store) termByID(id int64) (*model.Term, error) { return s.GetTerm(id) }

// TermAncestorOf reports whether term a is b or one of b's ancestors (a
// parent that would make a cycle).
func (s *Store) TermAncestorOf(a, b int64) (bool, error) {
	for i := 0; b != 0 && i < 100; i++ {
		if a == b {
			return true, nil
		}
		t, err := s.GetTerm(b)
		if err != nil {
			return false, err
		}
		b = t.Parent
	}
	return false, nil
}

func (s *Store) CreateTerm(t *model.Term) error {
	res, err := s.DB.Exec(`INSERT INTO terms (taxonomy, name, slug, parent, description) VALUES (?,?,?,?,?)`,
		t.Taxonomy, t.Name, t.Slug, t.Parent, t.Description)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	s.fire("term.created", termPayload(t))
	return nil
}

// UpdateTerm saves name, slug, parent and description (not the taxonomy).
func (s *Store) UpdateTerm(t *model.Term) error {
	old, _ := s.termByID(t.ID)
	if _, err := s.DB.Exec(`UPDATE terms SET name=?, slug=?, parent=?, description=? WHERE id=?`,
		t.Name, t.Slug, t.Parent, t.Description, t.ID); err != nil {
		return err
	}
	pl := termPayload(t)
	if old != nil {
		t.Taxonomy = old.Taxonomy
		pl["taxonomy"] = old.Taxonomy
		pl["previous"] = map[string]any{"name": old.Name, "slug": old.Slug, "parent": old.Parent}
	}
	s.fire("term.updated", pl)
	return nil
}

// DeleteTerm removes a term; its children move up to its parent.
func (s *Store) DeleteTerm(id int64) error {
	old, _ := s.termByID(id)
	err := s.inTx(func(tx *sql.Tx) error {
		if old != nil {
			if _, err := tx.Exec(`UPDATE terms SET parent=? WHERE parent=? AND taxonomy=?`, old.Parent, id, old.Taxonomy); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`DELETE FROM post_terms WHERE term_id=?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM terms WHERE id=?`, id)
		return err
	})
	if err == nil && old != nil {
		s.fire("term.deleted", termPayload(old))
	}
	return err
}
