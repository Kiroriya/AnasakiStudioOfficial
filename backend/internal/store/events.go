package store

import (
	"sync"
	"time"

	"github.com/anasaki/studio/backend/internal/model"
)

// Core events (2.1.0, P1-5): the store fires them for every write, whoever
// makes it — the admin, Site Code and JS plugins (db.posts), the scheduler,
// an importer — so a subscriber never misses one. Each payload carries
// "actor": {"id", "source"} (As). Delivery is the emitter's (plugin.Manager:
// asynchronous).

// Actor is who writes through a store copy: the user (0 = none) and where
// the write came from ("admin", "code", "plugin:<id>", "scheduler",
// "setup", "system", …).
type Actor struct {
	ID     int64
	Source string
}

type events struct {
	mu   sync.RWMutex
	emit func(string, map[string]any)
	url  func(*model.Post) string
}

// SetEvents connects the store to the event bus (plugin.NewManager does).
func (s *Store) SetEvents(emit func(string, map[string]any)) {
	s.ev.mu.Lock()
	s.ev.emit = emit
	s.ev.mu.Unlock()
}

// SetPostURL gives a post's public path for the payloads (site.New does).
func (s *Store) SetPostURL(url func(*model.Post) string) {
	s.ev.mu.Lock()
	s.ev.url = url
	s.ev.mu.Unlock()
}

// As returns a copy of the store whose writes are attributed to a (same
// database, option cache and event bus).
func (s *Store) As(a Actor) *Store {
	c := *s
	c.actor = a
	return &c
}

// Quiet returns a copy that fires no events (restoring a backup, seeding).
func (s *Store) Quiet() *Store {
	c := *s
	c.ev = &events{}
	return &c
}

func (s *Store) fire(name string, payload map[string]any) {
	s.ev.mu.RLock()
	emit := s.ev.emit
	s.ev.mu.RUnlock()
	if emit == nil {
		return
	}
	src := s.actor.Source
	if src == "" {
		src = "system"
	}
	payload["actor"] = map[string]any{"id": s.actor.ID, "source": src}
	emit(name, payload)
}

func (s *Store) postURL(p *model.Post) string {
	s.ev.mu.RLock()
	url := s.ev.url
	s.ev.mu.RUnlock()
	if url == nil {
		return ""
	}
	return url(p)
}

func isoOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// postPayload is the event payload of p (the keys of 2.0 kept: id, type,
// title, slug, status, url).
func (s *Store) postPayload(p *model.Post) map[string]any {
	return map[string]any{"id": p.ID, "type": p.Type, "title": p.Title, "slug": p.Slug, "status": p.Status,
		"url": s.postURL(p), "authorId": p.AuthorID, "publishedAt": isoOrNil(p.PublishedAt)}
}

// firePostSaved fires the events of a created (old == nil) or updated post.
func (s *Store) firePostSaved(p, old *model.Post) {
	if old == nil {
		s.fire("post.created", s.postPayload(p))
	} else {
		pl := s.postPayload(p)
		pl["previous"] = map[string]any{"title": old.Title, "slug": old.Slug, "status": old.Status,
			"authorId": old.AuthorID, "publishedAt": isoOrNil(old.PublishedAt)}
		s.fire("post.updated", pl)
	}
	saved := s.postPayload(p)
	saved["isNew"] = old == nil
	if old != nil {
		saved["previous"] = map[string]any{"title": old.Title, "slug": old.Slug, "status": old.Status}
	}
	s.fire("post.saved", saved)
	was := ""
	if old != nil {
		was = old.Status
	}
	if was == p.Status && (p.Status != model.StatusScheduled || timesEqual(old.PublishedAt, p.PublishedAt)) {
		return
	}
	if was != p.Status {
		pl := s.postPayload(p)
		pl["from"], pl["to"] = was, p.Status
		s.fire("post.statusChanged", pl)
	}
	switch {
	case p.Status == model.StatusPublish && was != model.StatusPublish:
		s.fire("post.published", s.postPayload(p))
	case p.Status == model.StatusScheduled:
		s.fire("post.scheduled", s.postPayload(p))
	}
	if was == model.StatusPublish && p.Status != model.StatusPublish {
		s.fire("post.unpublished", s.postPayload(p))
	}
}

func timesEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func userPayload(u *model.User) map[string]any {
	return map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "role": u.Role, "status": u.Status}
}

func mediaPayload(m *model.Media) map[string]any {
	return map[string]any{"id": m.ID, "filename": m.Filename, "url": m.URL, "mime": m.Mime, "size": m.Size,
		"kind": m.Kind, "width": m.Width, "height": m.Height, "alt": m.Alt, "authorId": m.AuthorID}
}

func termPayload(t *model.Term) map[string]any {
	return map[string]any{"id": t.ID, "taxonomy": t.Taxonomy, "name": t.Name, "slug": t.Slug, "parent": t.Parent}
}
