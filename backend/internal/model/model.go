// Package model holds the API/database entities.
package model

import (
	"encoding/json"
	"time"
)

// Built-in roles, modelled on WordPress' role/capability system. Their
// capabilities live in the roles table (editable, except administrator).
const (
	RoleAdmin       = "administrator" // everything
	RoleEditor      = "editor"        // all content, design, media, menus, components
	RoleAuthor      = "author"        // own posts + media, may publish
	RoleContributor = "contributor"   // own drafts only (cannot publish)
	RoleSubscriber  = "subscriber"    // read only
)

// User account statuses.
const (
	UserActive    = "active"
	UserSuspended = "suspended"
)

// Admin UI languages an account can choose.
const (
	LocaleTH = "th"
	LocaleEN = "en"
)

// ValidLocale reports whether l is a supported admin UI language.
func ValidLocale(l string) bool { return l == LocaleTH || l == LocaleEN }

// Post types and statuses.
const (
	TypePage     = "page"
	TypePost     = "post"
	TypeTemplate = "template"

	StatusDraft   = "draft"
	StatusPublish = "publish"
)

type User struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"` // display name
	FirstName   string     `json:"firstName"`
	LastName    string     `json:"lastName"`
	Bio         string     `json:"bio"`
	Website     string     `json:"website"`
	Avatar      string     `json:"avatar"` // image URL
	Role        string     `json:"role"`
	Status      string     `json:"status"` // active | suspended
	Locale      string     `json:"locale"`
	Caps        []string   `json:"caps"` // capabilities of the role, filled by the API
	LastLoginAt *time.Time `json:"lastLoginAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	PostCount   int        `json:"postCount"` // posts (type post) authored
}

// UserFilter is used by the users list endpoint.
type UserFilter struct {
	Search string
	Role   string
	Status string
	Limit  int
	Offset int
}

// Role is a named set of capabilities (WordPress roles).
type Role struct {
	Slug      string   `json:"slug"`
	Name      string   `json:"name"`
	Builtin   bool     `json:"builtin"`
	Caps      []string `json:"caps"`
	UserCount int      `json:"userCount"`
}

type Post struct {
	ID              int64           `json:"id"`
	Type            string          `json:"type"`
	Title           string          `json:"title"`
	Slug            string          `json:"slug"`
	Status          string          `json:"status"`
	Excerpt         string          `json:"excerpt"`
	Content         string          `json:"content"`
	Design          json.RawMessage `json:"design,omitempty"`
	FeaturedMediaID *int64          `json:"featuredMediaId"`
	FeaturedImage   string          `json:"featuredImage"`
	AuthorID        int64           `json:"authorId"`
	AuthorName      string          `json:"authorName"`
	Categories      []Term          `json:"categories"`
	Pinned          bool            `json:"pinned"`    // shown first in post lists (WordPress "sticky")
	SortOrder       int             `json:"sortOrder"` // position for the "manual" order
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	PublishedAt     *time.Time      `json:"publishedAt"`
	// Meta holds the custom fields (post_meta: site.postType / site.fields of
	// Site Code); {} when there are none.
	Meta map[string]any `json:"meta"`
}

// Revision is an earlier version of a post (PostID) or a component (also in
// PostID, with Design holding the component's node).
type Revision struct {
	ID         int64           `json:"id"`
	PostID     int64           `json:"postId"`
	Title      string          `json:"title"`
	Content    string          `json:"content,omitempty"`
	Design     json.RawMessage `json:"design,omitempty"`
	AuthorName string          `json:"authorName"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type Term struct {
	ID       int64  `json:"id"`
	Taxonomy string `json:"taxonomy"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Count    int    `json:"count"`
}

type Media struct {
	ID        int64     `json:"id"`
	Filename  string    `json:"filename"`
	URL       string    `json:"url"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Alt       string    `json:"alt"`
	Kind      string    `json:"kind"` // image | video | audio | document | archive | other
	Ext       string    `json:"ext"`  // lower-case extension without the dot
	Downloads int64     `json:"downloads"`
	AuthorID  int64     `json:"authorId"`
	CreatedAt time.Time `json:"createdAt"`
}

type Menu struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Items     json.RawMessage `json:"items"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type Component struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Node      json.RawMessage `json:"node"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// PostFilter is used by list endpoints and the Query Loop renderer.
type PostFilter struct {
	Type     string
	Status   string
	Search   string
	Category string // term slug
	AuthorID int64
	Order    string // newest | oldest | title | updated | manual
	// PinnedFirst lists pinned posts before the others (public post lists).
	PinnedFirst bool
	// NoContent leaves the post bodies out (Content is ""): lists that only
	// show titles, links or counts.
	NoContent bool
	Limit     int
	Offset    int
	// Meta filters on custom fields (Site Code: db.posts.list where 'meta.x').
	Meta []MetaCond
	// MetaOrder sorts by a custom field instead of Order ("" = none).
	MetaOrder string
	MetaDesc  bool
}

// MetaCond compares the custom field Key (a JSON value) with Value using Op:
// = != < <= > >= or in (Value is then a []any).
type MetaCond struct {
	Key   string
	Op    string
	Value any
}

// FileKind classifies a lower-case file extension (without the dot) for the
// media library filter.
func FileKind(ext string) string {
	switch ext {
	case "jpg", "jpeg", "png", "gif", "webp", "svg", "avif", "bmp", "ico":
		return "image"
	case "mp4", "webm", "mov", "m4v", "ogv":
		return "video"
	case "mp3", "wav", "ogg", "m4a", "aac", "flac", "oga":
		return "audio"
	case "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "ods", "odp", "txt", "csv", "rtf", "md", "epub":
		return "document"
	case "zip", "rar", "7z", "tar", "gz", "tgz", "bz2", "xz":
		return "archive"
	}
	return "other"
}
