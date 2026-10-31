package store

import (
	"database/sql"
	"strconv"
	"time"
)

// DesignComment is a designer comment (thread root or reply) pinned to a
// layer of a page/post/template or a component.
type DesignComment struct {
	ID           int64     `json:"id"`
	TargetKind   string    `json:"targetKind"` // post | component
	TargetID     int64     `json:"targetId"`
	NodeID       *string   `json:"nodeId"`
	X            float64   `json:"x"`
	Y            float64   `json:"y"`
	Body         string    `json:"body"`
	AuthorID     int64     `json:"authorId"`
	AuthorName   string    `json:"authorName"`
	AuthorAvatar string    `json:"authorAvatar"`
	ParentID     *int64    `json:"parentId"`
	Resolved     bool      `json:"resolved"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

const commentCols = `c.id, c.target_kind, c.target_id, c.node_id, c.x, c.y, c.body, c.author_id,
	COALESCE(u.name, ''), COALESCE(u.avatar, ''), c.parent_id, c.resolved, c.created_at, c.updated_at`

func scanComment(row interface{ Scan(...any) error }) (*DesignComment, error) {
	var c DesignComment
	var node sql.NullString
	var parent sql.NullInt64
	if err := row.Scan(&c.ID, &c.TargetKind, &c.TargetID, &node, &c.X, &c.Y, &c.Body, &c.AuthorID,
		&c.AuthorName, &c.AuthorAvatar, &parent, &c.Resolved, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	if node.Valid {
		c.NodeID = &node.String
	}
	if parent.Valid {
		c.ParentID = &parent.Int64
	}
	return &c, nil
}

// ListDesignComments returns every comment (roots and replies) of a target, oldest first.
func (s *Store) ListDesignComments(kind string, id int64) ([]*DesignComment, error) {
	rows, err := s.DB.Query(`SELECT `+commentCols+` FROM design_comments c LEFT JOIN users u ON u.id = c.author_id
		WHERE c.target_kind = ? AND c.target_id = ? ORDER BY c.id LIMIT 2000`, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*DesignComment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetDesignComment(id int64) (*DesignComment, error) {
	c, err := scanComment(s.DB.QueryRow(`SELECT `+commentCols+` FROM design_comments c
		LEFT JOIN users u ON u.id = c.author_id WHERE c.id = ?`, id))
	return c, notFound(err)
}

func (s *Store) CreateDesignComment(c *DesignComment) error {
	now := time.Now().UTC().Truncate(time.Second)
	res, err := s.DB.Exec(`INSERT INTO design_comments (target_kind, target_id, node_id, x, y, body, author_id, parent_id, resolved, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,FALSE,?,?)`, c.TargetKind, c.TargetID, c.NodeID, c.X, c.Y, c.Body, c.AuthorID, c.ParentID, now, now)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateDesignCommentBody(id int64, body string) error {
	_, err := s.DB.Exec(`UPDATE design_comments SET body = ?, updated_at = ? WHERE id = ?`, body, time.Now().UTC(), id)
	return err
}

func (s *Store) SetDesignCommentResolved(id int64, resolved bool) error {
	_, err := s.DB.Exec(`UPDATE design_comments SET resolved = ?, updated_at = ? WHERE id = ?`, resolved, time.Now().UTC(), id)
	return err
}

// DeleteDesignComment removes a comment and, for a thread root, its replies.
func (s *Store) DeleteDesignComment(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM design_comments WHERE id = ? OR parent_id = ?`, id, id)
	return err
}

// DesignCommentsStamp is a cheap change marker for a target's comments
// (count + latest update), polled by the designer's presence heartbeat.
func (s *Store) DesignCommentsStamp(kind string, id int64) (string, error) {
	var n int64
	var last sql.NullTime
	if err := s.DB.QueryRow(`SELECT COUNT(*), MAX(updated_at) FROM design_comments WHERE target_kind = ? AND target_id = ?`,
		kind, id).Scan(&n, &last); err != nil {
		return "", err
	}
	stamp := time.Time{}
	if last.Valid {
		stamp = last.Time
	}
	return stamp.UTC().Format(time.RFC3339) + "#" + strconv.FormatInt(n, 10), nil
}

// TargetUpdatedAt returns the stored updated_at of a post or component (no
// content is loaded), for optimistic-concurrency checks and presence.
func (s *Store) TargetUpdatedAt(kind string, id int64) (time.Time, error) {
	table := "posts"
	if kind == "component" {
		table = "components"
	}
	var t time.Time
	err := s.DB.QueryRow(`SELECT updated_at FROM `+table+` WHERE id = ?`, id).Scan(&t)
	return t, notFound(err)
}

// LastEditorName returns who most recently saved a changed version of a post
// or component (from the newest revision; empty when unknown).
func (s *Store) LastEditorName(kind string, id int64) string {
	q := `SELECT COALESCE(u.name, '') FROM revisions r LEFT JOIN users u ON u.id = r.author_id WHERE r.post_id = ? ORDER BY r.id DESC LIMIT 1`
	if kind == "component" {
		q = `SELECT COALESCE(u.name, '') FROM component_revisions r LEFT JOIN users u ON u.id = r.author_id WHERE r.component_id = ? ORDER BY r.id DESC LIMIT 1`
	}
	var name string
	_ = s.DB.QueryRow(q, id).Scan(&name)
	return name
}

// UserBrief returns a user's display name and avatar URL.
func (s *Store) UserBrief(id int64) (name, avatar string) {
	_ = s.DB.QueryRow(`SELECT name, avatar FROM users WHERE id = ?`, id).Scan(&name, &avatar)
	return
}
