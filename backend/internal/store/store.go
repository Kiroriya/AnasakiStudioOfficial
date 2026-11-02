// Package store is the MySQL repository layer.
package store

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	DB *sql.DB

	*optCache // shared by every copy (As)
	ev        *events
	actor     Actor // who writes through this copy (As); zero = the system
}

// optCache: the site options are read on every public request.
type optCache struct {
	optMu   sync.Mutex
	opts    map[string]string
	optsAt  time.Time
	optsGen uint64 // bumped by every invalidation, so a slow read never caches stale values
}

// optionsTTL bounds how long cached options live: writes through SetOptions
// (and InvalidateCache) apply at once, other writers (another process
// sharing the database, manual SQL) within this time.
const optionsTTL = 3 * time.Second

func New(db *sql.DB) *Store { return &Store{DB: db, optCache: &optCache{}, ev: &events{}} }

// InvalidateCache drops the cached options; call it after changing the
// options table without SetOptions (e.g. restoring a backup or the demo
// baseline).
func (s *Store) InvalidateCache() {
	s.optMu.Lock()
	s.opts, s.optsGen = nil, s.optsGen+1
	s.optMu.Unlock()
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Slugify keeps Unicode letters (Thai included) and digits, joined by '-'.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "untitled"
	}
	// 180 characters leaves room for a "-N" suffix in the VARCHAR(191) column
	return clipRunes(out, 180)
}

func nullBytes(b []byte) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// clipRunes cuts s to at most n characters without splitting a UTF-8
// sequence (MySQL rejects invalid UTF-8; VARCHAR(n) counts characters).
func clipRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// inTx runs fn in a transaction, committed when fn returns nil.
func (s *Store) inTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
