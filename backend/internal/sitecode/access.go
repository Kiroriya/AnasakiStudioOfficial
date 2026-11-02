package sitecode

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Access layer 2 (design §7.2): who may use each route, collection, action,
// page and developer table. The code declares a default (access: [...]);
// an administrator can override it in script_access without touching code.
// Anonymous visitors match only '*'; the administrator always passes.

// Access kinds.
const (
	KindRoute      = "route"
	KindCollection = "collection"
	KindAction     = "action"
	KindPage       = "page"
	KindTable      = "table"
	KindAdminPage  = "adminPage" // phase 6: who may open a site.adminPage (admin roles only)
)

var accessKinds = map[string]bool{KindRoute: true, KindCollection: true, KindAction: true, KindPage: true, KindTable: true, KindAdminPage: true}

// IsAccessKind reports a valid script_access kind.
func IsAccessKind(k string) bool { return accessKinds[k] }

// AccessRow is one entry of GET /api/v1/code/access.
type AccessRow struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Label       any      `json:"label"`
	CodeDefault []string `json:"codeDefault"`
	Override    []string `json:"override"`
	Effective   []string `json:"effective"`
	Unit        string   `json:"unit"` // "site" or the JS plugin (names "<id>:<name>")
}

type accessCache struct {
	at time.Time
	m  map[string][]string
}

const accessTTL = 5 * time.Second

func accessKey(kind, name string) string { return kind + "\x00" + name }

// overrides returns the script_access rows (cached a few seconds; another
// server process may change them).
func (e *Engine) overrides() map[string][]string {
	if c := e.access.Load(); c != nil && time.Since(c.at) < accessTTL {
		return c.m
	}
	if e.DB == nil || e.DB.db == nil {
		return map[string][]string{}
	}
	m, err := e.DB.AccessOverrides()
	if err != nil {
		if c := e.access.Load(); c != nil {
			return c.m
		}
		return map[string][]string{}
	}
	e.access.Store(&accessCache{at: time.Now(), m: m})
	return m
}

// defaultAccess is the access without a code declaration.
func defaultAccess(kind string) []string {
	switch kind {
	case KindPage:
		return []string{"*"}
	case KindTable:
		return []string{}
	}
	return []string{"administrator"}
}

// Effective is the access of kind/name: the admin override, else the code
// default, else the kind's default.
func (e *Engine) Effective(kind, name string, codeDefault []string) []string {
	if o, ok := e.overrides()[accessKey(kind, name)]; ok {
		return o
	}
	if codeDefault != nil {
		return codeDefault
	}
	return defaultAccess(kind)
}

// AccessOverrides reads script_access.
func (s *DB) AccessOverrides() (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT kind, name, roles FROM script_access`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var kind, name string
		var roles sql.NullString
		if err := rows.Scan(&kind, &name, &roles); err != nil {
			return nil, err
		}
		if !roles.Valid {
			continue
		}
		var list []string
		if json.Unmarshal([]byte(roles.String), &list) != nil {
			continue
		}
		if list == nil {
			list = []string{}
		}
		out[accessKey(kind, name)] = list
	}
	return out, rows.Err()
}

// ErrBadAccess is a refused PUT /code/access.
type ErrBadAccess struct{ Msg string }

func (e *ErrBadAccess) Error() string { return e.Msg }

// SetAccess stores (roles != nil) or removes (nil) an override.
func (e *Engine) SetAccess(kind, name string, roles []string, userID int64) error {
	if !accessKinds[kind] {
		return &ErrBadAccess{Msg: "kind ต้องเป็น route, collection, action, page, table หรือ adminPage"}
	}
	if name == "" || len(name) > 191 {
		return &ErrBadAccess{Msg: "กรุณาระบุชื่อ (ไม่เกิน 191 ตัวอักษร)"}
	}
	defer e.access.Store(nil)
	if roles == nil {
		_, err := e.st.DB.Exec(`DELETE FROM script_access WHERE kind = ? AND name = ?`, kind, name)
		return err
	}
	clean := []string{}
	if kind == KindAdminPage {
		for _, r := range roles {
			if !adminEntry(r) {
				return &ErrBadAccess{Msg: fmt.Sprintf("หน้าแอดมินกำหนดได้เฉพาะ role ของผู้ดูแล (เช่น 'editor') — %q ใช้ไม่ได้", r)}
			}
		}
	}
	if len(roles) > 0 {
		list, err := checkAccess(roles)
		if err != nil {
			return &ErrBadAccess{Msg: err.Error()}
		}
		seen := map[string]bool{}
		for _, r := range list {
			if !seen[r] {
				seen[r] = true
				clean = append(clean, r)
			}
		}
	}
	b, _ := json.Marshal(clean)
	_, err := e.st.DB.Exec(`INSERT INTO script_access (kind, name, roles, updated_by, updated_at) VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE roles = VALUES(roles), updated_by = VALUES(updated_by), updated_at = VALUES(updated_at)`,
		kind, name, string(b), nullID(userID), time.Now().UTC())
	return err
}

// LiveManifest is the manifest of the live version (empty without one).
func (e *Engine) LiveManifest() (Manifest, error) {
	var m Manifest
	m.normalize()
	id, err := e.LiveVersion()
	if err != nil || id == 0 {
		return m, err
	}
	row, err := e.DB.GetVersion(id, id, false)
	if errors.Is(err, ErrNotFound) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	return row.Manifest, nil
}

// DeveloperTables lists the developer tables of the code database.
func (e *Engine) DeveloperTables(ctx context.Context) ([]TableInfo, error) {
	if e.data == nil {
		return nil, nil
	}
	list, _, err := e.data.objects(ctx)
	if err != nil {
		return nil, err
	}
	var out []TableInfo
	for _, t := range list {
		if !t.View && !t.System {
			out = append(out, t)
		}
	}
	return out, nil
}

// AccessList is GET /api/v1/code/access: everything the live version
// registered plus every developer table.
func (e *Engine) AccessList(ctx context.Context) ([]AccessRow, error) { return e.AccessListOf(ctx, "") }

// AccessListOf is AccessList of one unit ("" = every unit; plugin rows are
// named "<id>:<name>", routes "<id>:GET /x").
func (e *Engine) AccessListOf(ctx context.Context, unit string) ([]AccessRow, error) {
	m, err := e.LiveManifest()
	if err != nil {
		return nil, err
	}
	e.access.Store(nil) // fresh view for the admin screen
	ov := e.overrides()
	out := []AccessRow{}
	cur := SiteUnit
	add := func(kind, name string, label any, def []string) {
		row := AccessRow{Kind: kind, Name: name, Label: label, CodeDefault: def, Unit: cur}
		if o, ok := ov[accessKey(kind, name)]; ok {
			row.Override = o
		}
		row.Effective = e.Effective(kind, name, def)
		out = append(out, row)
	}
	for _, lp := range e.units().loaded() {
		u := lp.unit
		if unit != "" && unit != u.ID {
			continue
		}
		cur = u.ID
		pm := lp.prog.Manifest
		for _, r := range pm.Routes {
			add(KindRoute, u.qualify(r.Method+" "+r.Path), r.Method+" "+r.Path, r.Access)
		}
		for _, c := range pm.Collections {
			add(KindCollection, u.qualify(c.Name), c.Label, c.Access)
		}
		for _, a := range pm.Actions {
			add(KindAction, u.qualify(a.Name), a.Name, a.Access)
		}
		for _, p := range pm.Pages {
			add(KindPage, u.qualify(p.Path), p.Path, p.Access)
		}
		for _, a := range pm.AdminPages {
			add(KindAdminPage, u.qualify(a.ID), a.Label, a.Access)
		}
	}
	cur = SiteUnit
	if unit != "" && unit != SiteUnit {
		return out, nil
	}
	for _, r := range m.Routes {
		add(KindRoute, r.Method+" "+r.Path, r.Method+" "+r.Path, r.Access)
	}
	for _, c := range m.Collections {
		add(KindCollection, c.Name, c.Label, c.Access)
	}
	for _, a := range m.Actions {
		add(KindAction, a.Name, a.Name, a.Access)
	}
	for _, p := range m.Pages {
		add(KindPage, p.Path, p.Path, p.Access)
	}
	for _, a := range m.AdminPages {
		add(KindAdminPage, a.ID, a.Label, a.Access)
	}
	tables, err := e.DeveloperTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].Name < tables[j].Name })
	for _, t := range tables {
		add(KindTable, t.Name, t.Name, nil)
	}
	return out, nil
}
