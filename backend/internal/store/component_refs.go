package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"strconv"
)

// ComponentUse is one page/template or component that places a component.
type ComponentUse struct {
	Kind  string `json:"kind"` // "post" or "component"
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type,omitempty"` // post type
}

// decodeTree parses design JSON keeping numbers exact (json.Number).
func decodeTree(raw []byte) (map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil || m == nil {
		return nil, false
	}
	return m, true
}

func refID(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Int64()
		if n == 0 {
			f, _ := x.Float64()
			n = int64(f)
		}
		return n
	case float64:
		return int64(x)
	}
	return 0
}

// visitInstances calls fn for every instance node in the tree; fn returns
// true when it changed the node.
func visitInstances(n map[string]any, fn func(props map[string]any) bool) bool {
	changed := false
	if n["type"] == "instance" {
		if p, ok := n["props"].(map[string]any); ok && fn(p) {
			changed = true
		}
	}
	if kids, ok := n["children"].([]any); ok {
		for _, k := range kids {
			if km, ok := k.(map[string]any); ok && visitInstances(km, fn) {
				changed = true
			}
		}
	}
	return changed
}

func usesComponent(n map[string]any, id int64) bool {
	found := false
	visitInstances(n, func(p map[string]any) bool {
		if refID(p["componentId"]) == id {
			found = true
		}
		return false
	})
	return found
}

// ComponentUsage lists the pages, templates, posts and other components that
// place component id.
func (s *Store) ComponentUsage(id int64) ([]ComponentUse, error) {
	out := []ComponentUse{}
	rows, err := s.DB.Query(`SELECT id, type, title, design FROM posts WHERE design LIKE '%componentId%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u ComponentUse
		var raw []byte
		if err := rows.Scan(&u.ID, &u.Type, &u.Title, &raw); err != nil {
			return nil, err
		}
		if m, ok := decodeTree(raw); ok && usesComponent(m, id) {
			u.Kind = "post"
			out = append(out, u)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	cs, err := s.ListComponents()
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		if c.ID == id {
			continue
		}
		if m, ok := decodeTree(c.Node); ok && usesComponent(m, id) {
			out = append(out, ComponentUse{Kind: "component", ID: c.ID, Title: c.Name})
		}
	}
	return out, nil
}

// ReplaceComponentRefs points every placement of component from at component
// to (pages, templates, posts and other components) on behalf of editorID
// and returns how many designs changed. The component "to" itself is left
// alone so it can never end up containing itself.
func (s *Store) ReplaceComponentRefs(from, to, editorID int64) (int, error) {
	swap := func(raw []byte) ([]byte, bool) {
		m, ok := decodeTree(raw)
		if !ok {
			return nil, false
		}
		if !visitInstances(m, func(p map[string]any) bool {
			if refID(p["componentId"]) != from {
				return false
			}
			p["componentId"] = to
			return true
		}) {
			return nil, false
		}
		b, err := json.Marshal(m)
		return b, err == nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, table := range []string{"posts", "components"} {
		col := map[string]string{"posts": "design", "components": "node"}[table]
		list, err := collectDesigns(tx, designQueries[table]+` AND `+col+` LIKE '%componentId%'`)
		if err != nil {
			return 0, err
		}
		for _, r := range list {
			if table == "components" && r.id == to {
				continue
			}
			if b, ok := swap(r.raw); ok {
				if err := rewriteDesign(tx, table, r, b, editorID); err != nil {
					return 0, err
				}
				n++
			}
		}
	}
	return n, tx.Commit()
}

// PlaceComponentOnPages adds component id (role names the layer id, e.g. "header") as the first (atEnd=false) or last
// child of every page and template design that does not place it yet, on
// behalf of editorID, and returns how many designs changed.
func (s *Store) PlaceComponentOnPages(id int64, name, role string, atEnd bool, editorID int64) (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := collectDesigns(tx, designQueries["posts"]+` AND type IN ('page','template') AND design <> ''`)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		m, ok := decodeTree(r.raw)
		if !ok || m["type"] != "frame" || usesComponent(m, id) {
			continue
		}
		inst := map[string]any{"id": "site-" + role + "-" + strconv.FormatInt(id, 10), "type": "instance", "name": name,
			"width": map[string]any{"mode": "fill"}, "height": map[string]any{"mode": "hug"},
			"props": map[string]any{"componentId": id}}
		kids, _ := m["children"].([]any)
		if atEnd {
			kids = append(kids, inst)
		} else {
			kids = append([]any{inst}, kids...)
		}
		m["children"] = kids
		b, err := json.Marshal(m)
		if err != nil {
			continue
		}
		if err := rewriteDesign(tx, "posts", r, b, editorID); err != nil {
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}

// removeInstances drops every instance of component id from the tree under
// n and reports whether anything was removed.
func removeInstances(n map[string]any, id int64) bool {
	kids, ok := n["children"].([]any)
	if !ok {
		return false
	}
	changed := false
	kept := kids[:0:0]
	for _, k := range kids {
		km, ok := k.(map[string]any)
		if ok && km["type"] == "instance" {
			if p, ok := km["props"].(map[string]any); ok && refID(p["componentId"]) == id {
				changed = true
				continue
			}
		}
		if ok && removeInstances(km, id) {
			changed = true
		}
		kept = append(kept, k)
	}
	if changed {
		n["children"] = kept
	}
	return changed
}

// removeComponentRefs takes every placement of component id out of the
// designs of posts and other components (keeping revisions) and returns
// how many designs changed.
func removeComponentRefs(tx *sql.Tx, id, editorID int64) (int, error) {
	n := 0
	for _, table := range []string{"posts", "components"} {
		col := map[string]string{"posts": "design", "components": "node"}[table]
		list, err := collectDesigns(tx, designQueries[table]+` AND `+col+` LIKE '%componentId%'`)
		if err != nil {
			return 0, err
		}
		for _, r := range list {
			if table == "components" && r.id == id {
				continue
			}
			m, ok := decodeTree(r.raw)
			if !ok || !removeInstances(m, id) {
				continue
			}
			b, err := json.Marshal(m)
			if err != nil {
				continue
			}
			if err := rewriteDesign(tx, table, r, b, editorID); err != nil {
				return 0, err
			}
			n++
		}
	}
	return n, nil
}

// RenameVariant points every instance of component id that shows variant
// from (props.variant) at variant to instead, in page/template/post designs
// and other components, on behalf of editorID. Each changed design keeps
// its previous version as a revision and gets a new updated_at (an editor
// holding the old version gets a conflict). It returns how many designs
// changed.
func (s *Store) RenameVariant(id int64, from, to string, editorID int64) (int, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, table := range []string{"posts", "components"} {
		col := map[string]string{"posts": "design", "components": "node"}[table]
		list, err := collectDesigns(tx, designQueries[table]+` AND `+col+` LIKE '%componentId%'`)
		if err != nil {
			return 0, err
		}
		for _, r := range list {
			m, ok := decodeTree(r.raw)
			if !ok {
				continue
			}
			if !visitInstances(m, func(p map[string]any) bool {
				if refID(p["componentId"]) != id || p["variant"] != from {
					return false
				}
				p["variant"] = to
				return true
			}) {
				continue
			}
			b, err := json.Marshal(m)
			if err != nil {
				continue
			}
			if err := rewriteDesign(tx, table, r, b, editorID); err != nil {
				return 0, err
			}
			n++
		}
	}
	return n, tx.Commit()
}

// ComponentName is a component's id and name (no design), for lookups that
// must stay cheap on every page view.
type ComponentName struct {
	ID   int64
	Name string
}

// ComponentNames lists every component's id and name, ordered by name.
func (s *Store) ComponentNames() ([]ComponentName, error) {
	rows, err := s.DB.Query(`SELECT id, name FROM components ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ComponentName
	for rows.Next() {
		var c ComponentName
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
