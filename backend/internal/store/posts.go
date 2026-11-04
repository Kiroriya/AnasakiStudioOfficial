package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/model"
)

const postSelect = `SELECT p.id, p.type, p.title, p.slug, p.status, COALESCE(p.excerpt,''), %s,
  p.featured_media_id, COALESCE(m.url,''), p.author_id, COALESCE(u.name,''),
  p.pinned, p.sort_order, p.created_at, p.updated_at, p.published_at
FROM posts p
LEFT JOIN users u ON u.id = p.author_id
LEFT JOIN media m ON m.id = p.featured_media_id`

func scanPost(sc interface{ Scan(...any) error }, withBody bool) (*model.Post, error) {
	var p model.Post
	var content sql.NullString
	var design []byte
	var fm sql.NullInt64
	var pub sql.NullTime
	dst := []any{&p.ID, &p.Type, &p.Title, &p.Slug, &p.Status, &p.Excerpt}
	if withBody {
		dst = append(dst, &content, &design)
	} else {
		dst = append(dst, &content)
	}
	dst = append(dst, &fm, &p.FeaturedImage, &p.AuthorID, &p.AuthorName, &p.Pinned, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt, &pub)
	if err := sc.Scan(dst...); err != nil {
		return nil, err
	}
	p.Content = content.String
	if len(design) > 0 {
		p.Design = json.RawMessage(copyBytes(design))
	}
	if fm.Valid {
		v := fm.Int64
		p.FeaturedMediaID = &v
	}
	if pub.Valid {
		t := pub.Time
		p.PublishedAt = &t
	}
	p.Categories = []model.Term{}
	p.Terms = map[string][]model.Term{}
	p.Meta = map[string]any{}
	return &p, nil
}

// ListPosts returns a page of posts (without design JSON) and the total count.
func (s *Store) ListPosts(f model.PostFilter) ([]*model.Post, int, error) {
	var where []string
	var args []any
	if f.Type != "" {
		where = append(where, "p.type = ?")
		args = append(args, f.Type)
	} else if len(f.Types) > 0 {
		where = append(where, "p.type IN ("+placeholders(len(f.Types))+")")
		for _, t := range f.Types {
			args = append(args, t)
		}
	}
	if f.TermIDs != nil {
		if len(f.TermIDs) == 0 {
			return []*model.Post{}, 0, nil
		}
		where = append(where, "p.id IN (SELECT post_id FROM post_terms WHERE term_id IN ("+placeholders(len(f.TermIDs))+"))")
		for _, id := range f.TermIDs {
			args = append(args, id)
		}
	}
	if f.Status != "" {
		where = append(where, "p.status = ?")
		args = append(args, f.Status)
	}
	if f.AuthorID != 0 {
		where = append(where, "p.author_id = ?")
		args = append(args, f.AuthorID)
	}
	if f.IDs != nil {
		if len(f.IDs) == 0 {
			return []*model.Post{}, 0, nil
		}
		ph := make([]string, len(f.IDs))
		for i, id := range f.IDs {
			ph[i] = "?"
			args = append(args, id)
		}
		where = append(where, "p.id IN ("+strings.Join(ph, ",")+")")
	}
	if f.Search != "" {
		where = append(where, "(p.title LIKE ? OR p.content LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like)
	}
	if f.Category != "" {
		where = append(where, `p.id IN (SELECT pt.post_id FROM post_terms pt JOIN terms t ON t.id = pt.term_id
			WHERE t.taxonomy = 'category' AND t.slug = ?)`)
		args = append(args, f.Category)
	}
	if len(f.Meta) > 0 {
		mw, margs, err := metaWhere(f.Meta)
		if err != nil {
			return nil, 0, err
		}
		where = append(where, mw...)
		args = append(args, margs...)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM posts p`+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := "COALESCE(p.published_at, p.created_at) DESC, p.id DESC"
	switch f.Order {
	case "oldest":
		order = "COALESCE(p.published_at, p.created_at) ASC, p.id ASC"
	case "title":
		order = "p.title ASC"
	case "updated":
		order = "p.updated_at DESC"
	case "manual":
		// posts never placed by hand (sort_order 0) follow, newest first
		order = "p.sort_order = 0, p.sort_order ASC, COALESCE(p.published_at, p.created_at) DESC, p.id DESC"
	}
	var orderArgs []any
	if f.MetaOrder != "" {
		dir := "ASC"
		if f.MetaDesc {
			dir = "DESC"
		}
		order = "(SELECT pm.value FROM post_meta pm WHERE pm.post_id = p.id AND pm.meta_key = ?) " + dir + ", p.id " + dir
		orderArgs = append(orderArgs, f.MetaOrder)
	}
	if f.PinnedFirst {
		order = "p.pinned DESC, " + order
	}
	order = " ORDER BY " + order
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	body := "p.content"
	if f.NoContent {
		body = "NULL" // bodies can be large; lists that only show titles skip them
	}
	q := fmt.Sprintf(postSelect, body) + cond + order + " LIMIT ? OFFSET ?"
	offset := max(f.Offset, 0)
	qargs := append(append(append([]any{}, args...), orderArgs...), limit, offset)
	rows, err := s.DB.Query(q, qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*model.Post{}
	for rows.Next() {
		p, err := scanPost(rows, false)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := s.attachMeta(out); err != nil {
		return nil, 0, err
	}
	return out, total, s.attachTerms(out)
}

func (s *Store) GetPost(id int64) (*model.Post, error) {
	p, err := scanPost(s.DB.QueryRow(fmt.Sprintf(postSelect, "p.content, p.design")+` WHERE p.id = ?`, id), true)
	if err != nil {
		return nil, notFound(err)
	}
	if err := s.attachMeta([]*model.Post{p}); err != nil {
		return nil, err
	}
	return p, s.attachTerms([]*model.Post{p})
}

// PostBySlug finds a post of a type; publishedOnly hides drafts (public site).
func (s *Store) PostBySlug(typ, slug string, publishedOnly bool) (*model.Post, error) {
	q := fmt.Sprintf(postSelect, "p.content, p.design") + ` WHERE p.type = ? AND p.slug = ?`
	if publishedOnly {
		q += ` AND p.status = 'publish'`
	}
	p, err := scanPost(s.DB.QueryRow(q, typ, slug), true)
	if err != nil {
		return nil, notFound(err)
	}
	if err := s.attachMeta([]*model.Post{p}); err != nil {
		return nil, err
	}
	return p, s.attachTerms([]*model.Post{p})
}

func (s *Store) attachTerms(posts []*model.Post) error {
	if len(posts) == 0 {
		return nil
	}
	idx := map[int64]*model.Post{}
	ph := make([]string, 0, len(posts))
	args := make([]any, 0, len(posts))
	for _, p := range posts {
		idx[p.ID] = p
		ph = append(ph, "?")
		args = append(args, p.ID)
	}
	rows, err := s.DB.Query(`SELECT pt.post_id, t.id, t.taxonomy, t.name, t.slug, t.parent, t.description FROM post_terms pt
		JOIN terms t ON t.id = pt.term_id WHERE pt.post_id IN (`+strings.Join(ph, ",")+`) ORDER BY t.name`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var pid int64
		var t model.Term
		if err := rows.Scan(&pid, &t.ID, &t.Taxonomy, &t.Name, &t.Slug, &t.Parent, &t.Description); err != nil {
			return err
		}
		if p := idx[pid]; p != nil {
			if t.Taxonomy == "category" {
				p.Categories = append(p.Categories, t)
			}
			if p.Terms == nil {
				p.Terms = map[string][]model.Term{}
			}
			p.Terms[t.Taxonomy] = append(p.Terms[t.Taxonomy], t)
		}
	}
	return rows.Err()
}

// UniqueSlug appends -2, -3, … until the slug is free for the type.
func (s *Store) UniqueSlug(typ, slug string, exceptID int64) (string, error) {
	base := Slugify(slug)
	cand := base
	for i := 2; ; i++ {
		var n int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM posts WHERE type=? AND slug=? AND id<>?`, typ, cand, exceptID).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return cand, nil
		}
		cand = fmt.Sprintf("%s-%d", base, i)
	}
}

// PostSave is what SavePost writes besides the post row.
type PostSave struct {
	TermIDs []int64 // every term of the post, all taxonomies (update: nil = unchanged)
	// Terms replaces the post's terms of the given taxonomies only (2.1.0);
	// ids of another taxonomy are dropped. Applied after TermIDs.
	Terms    map[string][]int64
	Meta     map[string]any // custom fields to set (nil value removes one); nil = none
	EditorID int64          // update: who made the revision
}

// CreatePost inserts p (no custom fields); fires post.created / post.saved.
func (s *Store) CreatePost(p *model.Post, termIDs []int64) error {
	return s.SavePost(p, true, PostSave{TermIDs: termIDs})
}

// UpdatePost saves the post and, when content or design changed, stores the
// previous version as a revision (WordPress keeps revisions the same way).
func (s *Store) UpdatePost(p *model.Post, termIDs []int64, editorID int64) error {
	return s.SavePost(p, false, PostSave{TermIDs: termIDs, EditorID: editorID})
}

// SavePost creates (isNew) or updates p with its terms and custom fields in
// one transaction, then fires the post events (post.created / post.updated,
// post.saved, post.statusChanged, post.published / post.scheduled /
// post.unpublished) — after the commit, so a subscriber reads the saved post.
func (s *Store) SavePost(p *model.Post, isNew bool, o PostSave) error {
	if isNew {
		if err := s.createPost(p, o); err != nil {
			return err
		}
		s.firePostSaved(p, nil)
		return nil
	}
	old, err := s.updatePost(p, o)
	if err != nil {
		return err
	}
	s.firePostSaved(p, old)
	return nil
}

func (s *Store) createPost(p *model.Post, o PostSave) error {
	termIDs := o.TermIDs
	now := time.Now().UTC().Truncate(time.Second) // DATETIME keeps seconds: never round up
	p.ResolveSchedule(now)
	if p.Status == model.StatusPublish && p.PublishedAt == nil {
		p.PublishedAt = &now
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO posts (type, title, slug, status, excerpt, content, design, featured_media_id,
		author_id, pinned, sort_order, created_at, updated_at, published_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.Type, p.Title, p.Slug, p.Status, p.Excerpt, p.Content, nullBytes(p.Design), p.FeaturedMediaID,
		p.AuthorID, p.Pinned, p.SortOrder, now, now, p.PublishedAt)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	if err := setTerms(tx, p.ID, termIDs); err != nil {
		return err
	}
	if err := setTaxTerms(tx, p.ID, o.Terms); err != nil {
		return err
	}
	if err := setMetaTx(tx, p.ID, o.Meta); err != nil {
		return err
	}
	p.CreatedAt, p.UpdatedAt = now, now
	return tx.Commit()
}

func (s *Store) updatePost(p *model.Post, o PostSave) (*model.Post, error) {
	termIDs, editorID := o.TermIDs, o.EditorID
	old, err := s.GetPost(p.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Truncate(time.Second) // DATETIME keeps seconds: never round up
	// the caller may set PublishedAt (back-dating, scheduling); otherwise the
	// first publish stamps it
	p.ResolveSchedule(now)
	if p.Status == model.StatusPublish && p.PublishedAt == nil {
		p.PublishedAt = &now
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if old.Content != p.Content || !JSONEqual(old.Design, p.Design) || old.Title != p.Title {
		if err := keepPostRevision(tx, old.ID, old.Title, old.Content, old.Design, editorID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(`UPDATE posts SET title=?, slug=?, status=?, excerpt=?, content=?, design=?,
		featured_media_id=?, author_id=?, pinned=?, sort_order=?, updated_at=?, published_at=? WHERE id=?`,
		p.Title, p.Slug, p.Status, p.Excerpt, p.Content, nullBytes(p.Design), p.FeaturedMediaID,
		p.AuthorID, p.Pinned, p.SortOrder, now, p.PublishedAt, p.ID); err != nil {
		return nil, err
	}
	if termIDs != nil {
		if err := setTerms(tx, p.ID, termIDs); err != nil {
			return nil, err
		}
	}
	if err := setTaxTerms(tx, p.ID, o.Terms); err != nil {
		return nil, err
	}
	if err := setMetaTx(tx, p.ID, o.Meta); err != nil {
		return nil, err
	}
	p.UpdatedAt = now
	return old, tx.Commit()
}

// PublishDue publishes the scheduled posts whose time has come and returns
// them (each only once, also with several servers on one database).
func (s *Store) PublishDue(now time.Time) ([]*model.Post, error) {
	rows, err := s.DB.Query(`SELECT id FROM posts WHERE status = ? AND published_at <= ? ORDER BY published_at LIMIT 100`,
		model.StatusScheduled, now.UTC())
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []*model.Post{}
	for _, id := range ids {
		res, err := s.DB.Exec(`UPDATE posts SET status = ? WHERE id = ? AND status = ?`, model.StatusPublish, id, model.StatusScheduled)
		if err != nil {
			return out, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // another server got it first
		}
		p, err := s.GetPost(id)
		if err != nil {
			return out, err
		}
		old := *p
		old.Status = model.StatusScheduled
		s.firePostSaved(p, &old)
		out = append(out, p)
	}
	return out, nil
}

// keepPostRevision stores a previous version of post id as a revision and
// keeps the 25 most recent.
func keepPostRevision(tx *sql.Tx, id int64, title, content string, design []byte, editorID int64) error {
	if _, err := tx.Exec(`INSERT INTO revisions (post_id, title, content, design, author_id) VALUES (?,?,?,?,?)`,
		id, title, content, nullBytes(design), editorID); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM revisions WHERE post_id = ? AND id NOT IN (
		SELECT id FROM (SELECT id FROM revisions WHERE post_id = ? ORDER BY id DESC LIMIT 25) keep)`, id, id)
	return err
}

// bumpedAt is the updated_at of a design rewritten behind its editor's back
// (links, component swaps): at least a second past the stored one, so the
// designer's baseUpdatedAt check (second precision) answers 409.
const bumpedAt = `GREATEST(?, updated_at + INTERVAL 1 SECOND)`

// JSONEqual reports whether two JSON documents hold the same value.
func JSONEqual(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return string(a) == string(b)
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return string(ja) == string(jb)
}

func setTerms(tx *sql.Tx, postID int64, termIDs []int64) error {
	if _, err := tx.Exec(`DELETE FROM post_terms WHERE post_id = ?`, postID); err != nil {
		return err
	}
	for _, t := range termIDs {
		if _, err := tx.Exec(`INSERT IGNORE INTO post_terms (post_id, term_id) VALUES (?,?)`, postID, t); err != nil {
			return err
		}
	}
	return nil
}

// DeletePost removes a post with its terms, revisions and fields; fires post.deleted.
// setTaxTerms replaces the post's terms of each taxonomy in terms with the
// listed ids of that taxonomy.
func setTaxTerms(tx *sql.Tx, postID int64, terms map[string][]int64) error {
	for tax, ids := range terms {
		if _, err := tx.Exec(`DELETE pt FROM post_terms pt JOIN terms t ON t.id = pt.term_id WHERE pt.post_id = ? AND t.taxonomy = ?`,
			postID, tax); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.Exec(`INSERT IGNORE INTO post_terms (post_id, term_id) SELECT ?, id FROM terms WHERE id = ? AND taxonomy = ?`,
				postID, id, tax); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) DeletePost(id int64) error {
	old, err := s.GetPost(id)
	if err != nil {
		return err
	}
	if err := s.deletePost(id); err != nil {
		return err
	}
	s.fire("post.deleted", s.postPayload(old))
	s.dropPostMenuLinks(old)
	return nil
}

func (s *Store) deletePost(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM post_terms WHERE post_id=?`, `DELETE FROM revisions WHERE post_id=?`, `DELETE FROM post_meta WHERE post_id=?`,
		`DELETE FROM design_comments WHERE target_kind='post' AND target_id=?`, `DELETE FROM posts WHERE id=?`} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetPostOrder stores the hand-made order of a list of posts (1, 2, 3 …);
// posts not in ids keep their position. Only touches posts of type typ.
func (s *Store) SetPostOrder(typ string, ids []int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE posts SET sort_order=? WHERE id=? AND type=?`, i+1, id, typ); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListRevisions(postID int64) ([]model.Revision, error) {
	rows, err := s.DB.Query(`SELECT r.id, r.post_id, r.title, COALESCE(u.name,''), r.created_at FROM revisions r
		LEFT JOIN users u ON u.id = r.author_id WHERE r.post_id = ? ORDER BY r.id DESC`, postID)
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

func (s *Store) GetRevision(postID, id int64) (*model.Revision, error) {
	var r model.Revision
	var content sql.NullString
	var design []byte
	err := s.DB.QueryRow(`SELECT id, post_id, title, content, design, created_at FROM revisions WHERE id=? AND post_id=?`, id, postID).
		Scan(&r.ID, &r.PostID, &r.Title, &content, &design, &r.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	r.Content = content.String
	if len(design) > 0 {
		r.Design = json.RawMessage(copyBytes(design))
	}
	return &r, nil
}
