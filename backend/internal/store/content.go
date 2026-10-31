package store

import (
	"database/sql"
	"encoding/json"
	"path"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/model"
)

// ---------------- Terms (categories / tags) ----------------

func (s *Store) ListTerms(taxonomy string) ([]model.Term, error) {
	rows, err := s.DB.Query(`SELECT t.id, t.taxonomy, t.name, t.slug, COUNT(pt.post_id) FROM terms t
		LEFT JOIN post_terms pt ON pt.term_id = t.id WHERE t.taxonomy = ? GROUP BY t.id, t.taxonomy, t.name, t.slug
		ORDER BY t.name`, taxonomy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Term{}
	for rows.Next() {
		var t model.Term
		if err := rows.Scan(&t.ID, &t.Taxonomy, &t.Name, &t.Slug, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) TermBySlug(taxonomy, slug string) (*model.Term, error) {
	var t model.Term
	err := s.DB.QueryRow(`SELECT id, taxonomy, name, slug FROM terms WHERE taxonomy=? AND slug=?`, taxonomy, slug).
		Scan(&t.ID, &t.Taxonomy, &t.Name, &t.Slug)
	if err != nil {
		return nil, notFound(err)
	}
	return &t, nil
}

func (s *Store) CreateTerm(t *model.Term) error {
	res, err := s.DB.Exec(`INSERT INTO terms (taxonomy, name, slug) VALUES (?,?,?)`, t.Taxonomy, t.Name, t.Slug)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateTerm(t *model.Term) error {
	_, err := s.DB.Exec(`UPDATE terms SET name=?, slug=? WHERE id=?`, t.Name, t.Slug, t.ID)
	return err
}

func (s *Store) DeleteTerm(id int64) error {
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM post_terms WHERE term_id=?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM terms WHERE id=?`, id)
		return err
	})
}

// ---------------- Media ----------------

const mediaCols = `id, filename, url, mime, size, width, height, alt, downloads, author_id, created_at`

func scanMedia(sc interface{ Scan(...any) error }, m *model.Media) error {
	if err := sc.Scan(&m.ID, &m.Filename, &m.URL, &m.Mime, &m.Size, &m.Width, &m.Height, &m.Alt, &m.Downloads, &m.AuthorID, &m.CreatedAt); err != nil {
		return err
	}
	FillMediaKind(m)
	return nil
}

// FillMediaKind derives Ext and Kind from the stored file URL.
func FillMediaKind(m *model.Media) {
	m.Ext = strings.TrimPrefix(strings.ToLower(path.Ext(m.URL)), ".")
	m.Kind = model.FileKind(m.Ext)
}

// IncMediaDownloads counts one download of a media file.
func (s *Store) IncMediaDownloads(id int64) error {
	_, err := s.DB.Exec(`UPDATE media SET downloads = downloads + 1 WHERE id=?`, id)
	return err
}

func (s *Store) ListMedia() ([]model.Media, error) {
	rows, err := s.DB.Query(`SELECT ` + mediaCols + ` FROM media ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Media{}
	for rows.Next() {
		var m model.Media
		if err := scanMedia(rows, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetMedia(id int64) (*model.Media, error) {
	var m model.Media
	if err := scanMedia(s.DB.QueryRow(`SELECT `+mediaCols+` FROM media WHERE id=?`, id), &m); err != nil {
		return nil, notFound(err)
	}
	return &m, nil
}

func (s *Store) CreateMedia(m *model.Media) error {
	m.CreatedAt = time.Now().UTC()
	res, err := s.DB.Exec(`INSERT INTO media (filename, url, mime, size, width, height, alt, author_id, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`, m.Filename, m.URL, m.Mime, m.Size, m.Width, m.Height, m.Alt, m.AuthorID, m.CreatedAt)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	FillMediaKind(m)
	return nil
}

func (s *Store) UpdateMediaAlt(id int64, alt string) error {
	_, err := s.DB.Exec(`UPDATE media SET alt=? WHERE id=?`, alt, id)
	return err
}

func (s *Store) DeleteMedia(id int64) error {
	return s.inTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE posts SET featured_media_id=NULL WHERE featured_media_id=?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM media WHERE id=?`, id)
		return err
	})
}

// ---------------- Menus ----------------

func (s *Store) ListMenus() ([]model.Menu, error) {
	rows, err := s.DB.Query(`SELECT id, name, items, updated_at FROM menus ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Menu{}
	for rows.Next() {
		var m model.Menu
		var items []byte
		if err := rows.Scan(&m.ID, &m.Name, &items, &m.UpdatedAt); err != nil {
			return nil, err
		}
		m.Items = json.RawMessage(copyBytes(items))
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetMenu(id int64) (*model.Menu, error) {
	var m model.Menu
	var items []byte
	if err := s.DB.QueryRow(`SELECT id, name, items, updated_at FROM menus WHERE id=?`, id).
		Scan(&m.ID, &m.Name, &items, &m.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	m.Items = json.RawMessage(copyBytes(items))
	return &m, nil
}

func (s *Store) SaveMenu(m *model.Menu) error {
	if len(m.Items) == 0 {
		m.Items = json.RawMessage("[]")
	}
	m.UpdatedAt = time.Now().UTC()
	if m.ID == 0 {
		res, err := s.DB.Exec(`INSERT INTO menus (name, items, updated_at) VALUES (?,?,?)`, m.Name, string(m.Items), m.UpdatedAt)
		if err != nil {
			return err
		}
		m.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.Exec(`UPDATE menus SET name=?, items=?, updated_at=? WHERE id=?`, m.Name, string(m.Items), m.UpdatedAt, m.ID)
	return err
}

func (s *Store) DeleteMenu(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM menus WHERE id=?`, id)
	return err
}

// ---------------- Components ----------------

func (s *Store) ListComponents() ([]model.Component, error) {
	rows, err := s.DB.Query(`SELECT id, name, node, updated_at FROM components ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Component{}
	for rows.Next() {
		var c model.Component
		var node []byte
		if err := rows.Scan(&c.ID, &c.Name, &node, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Node = json.RawMessage(copyBytes(node))
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetComponent(id int64) (*model.Component, error) {
	var c model.Component
	var node []byte
	if err := s.DB.QueryRow(`SELECT id, name, node, updated_at FROM components WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &node, &c.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	c.Node = json.RawMessage(copyBytes(node))
	return &c, nil
}

func (s *Store) SaveComponent(c *model.Component) error { return saveComponent(s.DB, c) }

func saveComponent(db interface {
	Exec(string, ...any) (sql.Result, error)
}, c *model.Component) error {
	c.UpdatedAt = time.Now().UTC()
	if c.ID == 0 {
		res, err := db.Exec(`INSERT INTO components (name, node, updated_at) VALUES (?,?,?)`, c.Name, string(c.Node), c.UpdatedAt)
		if err != nil {
			return err
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := db.Exec(`UPDATE components SET name=?, node=?, updated_at=? WHERE id=?`, c.Name, string(c.Node), c.UpdatedAt, c.ID)
	return err
}

// SaveComponentKeepingRevision saves c and, when its node or name changed,
// first keeps the previous version (the 25 most recent are kept).
func (s *Store) SaveComponentKeepingRevision(c *model.Component, editorID int64) error {
	var old *model.Component
	if c.ID != 0 {
		var err error
		if old, err = s.GetComponent(c.ID); err != nil {
			return err
		}
	}
	return s.inTx(func(tx *sql.Tx) error {
		if old != nil && (old.Name != c.Name || !JSONEqual(old.Node, c.Node)) {
			if err := keepComponentRevision(tx, old.ID, old.Name, old.Node, editorID); err != nil {
				return err
			}
		}
		return saveComponent(tx, c)
	})
}

// keepComponentRevision stores a previous version of component id and keeps
// the 25 most recent.
func keepComponentRevision(tx *sql.Tx, id int64, name string, node []byte, editorID int64) error {
	if _, err := tx.Exec(`INSERT INTO component_revisions (component_id, name, node, author_id) VALUES (?,?,?,?)`,
		id, name, string(node), editorID); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM component_revisions WHERE component_id = ? AND id NOT IN (
		SELECT id FROM (SELECT id FROM component_revisions WHERE component_id = ? ORDER BY id DESC LIMIT 25) keep)`, id, id)
	return err
}

// ListComponentRevisions lists a component's earlier versions, newest first
// (PostID holds the component id).
func (s *Store) ListComponentRevisions(id int64) ([]model.Revision, error) {
	rows, err := s.DB.Query(`SELECT r.id, r.component_id, r.name, COALESCE(u.name,''), r.created_at FROM component_revisions r
		LEFT JOIN users u ON u.id = r.author_id WHERE r.component_id = ? ORDER BY r.id DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Revision{}
	for rows.Next() {
		var r model.Revision
		if err := rows.Scan(&r.ID, &r.PostID, &r.Title, &r.AuthorName, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetComponentRevision returns one earlier version with its node in Design.
func (s *Store) GetComponentRevision(componentID, id int64) (*model.Revision, error) {
	var r model.Revision
	var node []byte
	err := s.DB.QueryRow(`SELECT id, component_id, name, node, created_at FROM component_revisions WHERE id=? AND component_id=?`,
		id, componentID).Scan(&r.ID, &r.PostID, &r.Title, &node, &r.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	r.Design = json.RawMessage(copyBytes(node))
	return &r, nil
}

// DeleteComponent deletes component id with its history and comments, and
// takes its placements out of every design (pages, templates, posts and
// other components) on behalf of editorID, so no instance is left pointing
// at nothing; each changed design keeps its previous version as a revision.
// It returns how many designs changed.
func (s *Store) DeleteComponent(id, editorID int64) (int, error) {
	n := 0
	err := s.inTx(func(tx *sql.Tx) error {
		var err error
		if n, err = removeComponentRefs(tx, id, editorID); err != nil {
			return err
		}
		for _, q := range []string{`DELETE FROM component_revisions WHERE component_id=?`,
			`DELETE FROM design_comments WHERE target_kind='component' AND target_id=?`, `DELETE FROM components WHERE id=?`} {
			if _, err := tx.Exec(q, id); err != nil {
				return err
			}
		}
		return nil
	})
	return n, err
}

// ---------------- Options ----------------

// Options returns every site option. They are read on each public request,
// so they are cached for a few seconds; SetOptions refreshes the cache at
// once. The map is the caller's own copy.
func (s *Store) Options() (map[string]string, error) {
	s.optMu.Lock()
	if s.opts != nil && time.Since(s.optsAt) < optionsTTL {
		out := cloneOptions(s.opts)
		s.optMu.Unlock()
		return out, nil
	}
	gen := s.optsGen
	s.optMu.Unlock()

	rows, err := s.DB.Query(`SELECT name, value FROM options`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.optMu.Lock()
	if gen == s.optsGen { // no write happened meanwhile
		s.opts, s.optsAt = cloneOptions(out), time.Now()
	}
	s.optMu.Unlock()
	return out, nil
}

func cloneOptions(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *Store) SetOptions(opts map[string]string) error {
	defer s.InvalidateCache()
	for k, v := range opts {
		if _, err := s.DB.Exec(`INSERT INTO options (name, value) VALUES (?,?) ON DUPLICATE KEY UPDATE value=VALUES(value)`, k, v); err != nil {
			return err
		}
	}
	return nil
}
