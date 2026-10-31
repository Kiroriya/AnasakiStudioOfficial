package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/design"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Site Code phase 3 admin API: access overrides (layer 2), and what the
// designer needs — the bindings catalog and live/draft previews.
//
//	GET  /api/v1/code/access    manage_scripts
//	PUT  /api/v1/code/access    manage_scripts  {kind, name, roles: [...] | null}
//	GET  /api/v1/code/bindings  edit_pages      (?draft=1 also needs edit_scripts)
//	POST /api/v1/code/preview   edit_pages      (draft: also edit_scripts)

func (a *API) codeBindRoutes(h, c func(pattern, cap string, fn http.HandlerFunc)) {
	const p = "/api/v1/code"
	c("GET "+p+"/access", auth.CapManageScripts, a.codeAccessList)
	c("PUT "+p+"/access", auth.CapManageScripts, a.codeAccessSet)
	h("GET "+p+"/bindings", "", a.codeBindings) // full catalog with edit_pages; post types / admin pages for everyone
	h("POST "+p+"/preview", auth.CapEditPages, a.codePreview)
	// phase 6: plugin formatters and their unprefixed aliases
	c("GET "+p+"/formatters", auth.CapViewScripts, a.codeFormatters)
	c("PUT "+p+"/formatters/aliases", auth.CapManageScripts, a.codeSetAliases)
}

// GET /api/v1/code/formatters          view_scripts   [{name, unit, alias?, conflict?}]
// PUT /api/v1/code/formatters/aliases  manage_scripts {aliases: {"baht": "thai:baht"}} (the whole map) → same as GET
func (a *API) codeFormatters(w http.ResponseWriter, r *http.Request) {
	rows, err := a.code.Formatters()
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *API) codeSetAliases(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Aliases map[string]string `json:"aliases"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Aliases == nil {
		in.Aliases = map[string]string{}
	}
	if err := a.code.SetFormatterAliases(in.Aliases); err != nil {
		var bad *sitecode.ErrBadAliases
		if errors.As(err, &bad) {
			writeErr(w, http.StatusBadRequest, bad.Msg)
			return
		}
		serverErr(w, err)
		return
	}
	a.codeAudit(r, "formatter_aliases", map[string]any{"aliases": in.Aliases})
	a.codeFormatters(w, r)
}

func (a *API) codeAccessList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rows, err := a.code.AccessListOf(ctx, r.URL.Query().Get("unit"))
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (a *API) codeAccessSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind  string    `json:"kind"`
		Name  string    `json:"name"`
		Roles *[]string `json:"roles"`
		Unit  string    `json:"unit"` // a JS plugin: the name gets its "<id>:" prefix
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Unit != "" && in.Unit != sitecode.SiteUnit && !strings.HasPrefix(in.Name, in.Unit+":") {
		in.Name = in.Unit + ":" + in.Name
	}
	var roles []string
	if in.Roles != nil {
		roles = *in.Roles
		if roles == nil {
			roles = []string{}
		}
	}
	if err := a.code.SetAccess(in.Kind, in.Name, roles, auth.From(r).ID); err != nil {
		var bad *sitecode.ErrBadAccess
		if errors.As(err, &bad) {
			writeErr(w, http.StatusBadRequest, bad.Msg)
			return
		}
		serverErr(w, err)
		return
	}
	a.codeAudit(r, "access", map[string]any{"kind": in.Kind, "name": in.Name, "roles": roles})
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	list, err := a.code.AccessList(ctx)
	if err != nil {
		serverErr(w, err)
		return
	}
	for _, row := range list {
		if row.Kind == in.Kind && row.Name == in.Name {
			writeJSON(w, http.StatusOK, row)
			return
		}
	}
	// an override for something the live code does not register (yet)
	writeJSON(w, http.StatusOK, sitecode.AccessRow{Kind: in.Kind, Name: in.Name, Label: in.Name, Override: roles,
		Effective: a.code.Effective(in.Kind, in.Name, nil)})
}

// bindingsOut is GET /code/bindings.
type bindingsOut struct {
	Collections []bindCollection `json:"collections"`
	Actions     []bindAction     `json:"actions"`
	Pages       []bindPage       `json:"pages"`
	Formatters  []string         `json:"formatters"`
	Tables      []bindTable      `json:"tables"`
	// phase 4a: custom post types and fields ({{post.meta.*}}), visitor groups
	PostTypes     []postTypeOut              `json:"postTypes"`
	PostFields    map[string]json.RawMessage `json:"postFields"`
	Visitor       bool                       `json:"visitor"`
	VisitorGroups []string                   `json:"visitorGroups"`
	AdminPages    []adminPageOut             `json:"adminPages"`
}

type bindCollection struct {
	Name    string               `json:"name"`
	Label   any                  `json:"label"`
	Fields  map[string]string    `json:"fields"`
	Params  map[string]string    `json:"params"`
	PerPage sitecode.PerPageInfo `json:"perPage"`
	Unit    string               `json:"unit"` // "site" or the JS plugin
}

type bindAction struct {
	Name  string            `json:"name"`
	Input map[string]string `json:"input"`
	Unit  string            `json:"unit"`
}

type bindPage struct {
	Path string `json:"path"`
	Unit string `json:"unit"`
}

type bindTable struct {
	Name    string       `json:"name"`
	Columns []bindColumn `json:"columns"`
}

type bindColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (a *API) codeBindings(w http.ResponseWriter, r *http.Request) {
	out := bindingsOut{Collections: []bindCollection{}, Actions: []bindAction{}, Pages: []bindPage{}, Formatters: []string{}, Tables: []bindTable{},
		PostTypes: []postTypeOut{}, PostFields: map[string]json.RawMessage{}, VisitorGroups: []string{}, AdminPages: []adminPageOut{}}
	me := auth.From(r)
	designer := me.Can(auth.CapEditPages) // others (e.g. authors of a custom type) get only the post types, groups and admin pages
	if a.code.EnvDisabled() {
		writeJSON(w, http.StatusOK, out)
		return
	}
	// demo mode: Site Code is off (no draft, no live manifest, no developer
	// tables) but the JS plugins from the Store run, so their catalog is listed
	demo := a.code.Demo()
	if demo && r.URL.Query().Get("draft") == "1" {
		writeErr(w, http.StatusForbidden, msgCodeDemo)
		return
	}
	var m sitecode.Manifest
	if demo {
		m = sitecode.EmptyManifest()
	} else if r.URL.Query().Get("draft") == "1" {
		if !auth.From(r).Can(auth.CapEditScripts) {
			writeErr(w, http.StatusForbidden, "ต้องมีสิทธิ์แก้ไขโค้ด (edit_scripts) จึงจะดูร่างได้")
			return
		}
		res, _, err := a.code.BuildDraft()
		if err != nil {
			serverErr(w, err)
			return
		}
		if !res.OK {
			writeJSON(w, http.StatusOK, map[string]any{"collections": out.Collections, "actions": out.Actions, "pages": out.Pages,
				"formatters": out.Formatters, "tables": out.Tables, "error": "build ไม่ผ่าน — แก้ข้อผิดพลาดก่อน", "errors": res.Errors})
			return
		}
		m = res.Registered
	} else {
		var err error
		if m, err = a.code.LiveManifest(); err != nil {
			serverErr(w, err)
			return
		}
	}
	out.PostTypes, out.PostFields = postCatalog(a.code.WithUnitFields(m), me)
	out.Visitor = m.Visitor
	out.VisitorGroups = append(out.VisitorGroups, m.VisitorGroups...)
	plugins := a.code.PluginManifests()
	for _, pm := range plugins {
		out.Visitor = out.Visitor || pm.Manifest.Visitor
		for _, g := range pm.Manifest.VisitorGroups {
			if !slices.Contains(out.VisitorGroups, g) {
				out.VisitorGroups = append(out.VisitorGroups, g)
			}
		}
	}
	for _, pg := range a.code.AdminPages(principalUser(me)) {
		out.AdminPages = append(out.AdminPages, adminPageOut{ID: pg.ID, Label: pg.Label, Icon: pg.Icon})
	}
	if !designer {
		writeJSON(w, http.StatusOK, out)
		return
	}
	for _, c := range m.Collections {
		out.Collections = append(out.Collections, bindCollection{Name: c.Name, Label: c.Label, Fields: c.Fields, Params: c.Params, PerPage: c.PerPage, Unit: sitecode.SiteUnit})
	}
	for _, ac := range m.Actions {
		out.Actions = append(out.Actions, bindAction{Name: ac.Name, Input: ac.Input, Unit: sitecode.SiteUnit})
	}
	for _, p := range m.Pages {
		out.Pages = append(out.Pages, bindPage{Path: p.Path, Unit: sitecode.SiteUnit})
	}
	{
		for _, pm := range plugins { // JS plugins: "<id>:<name>" (source code:<id>:<name>, action /_a/<id>:<name>)
			for _, c := range pm.Manifest.Collections {
				out.Collections = append(out.Collections, bindCollection{Name: pm.Unit + ":" + c.Name, Label: c.Label, Fields: c.Fields,
					Params: c.Params, PerPage: c.PerPage, Unit: pm.Unit})
			}
			for _, ac := range pm.Manifest.Actions {
				out.Actions = append(out.Actions, bindAction{Name: pm.Unit + ":" + ac.Name, Input: ac.Input, Unit: pm.Unit})
			}
			for _, p := range pm.Manifest.Pages {
				out.Pages = append(out.Pages, bindPage{Path: p.Path, Unit: pm.Unit})
			}
		}
	}
	out.Formatters = append(out.Formatters, m.Formatters...)
	for _, pm := range plugins { // phase 6: "<id>:<name>", plus the enabled aliases
		for _, f := range pm.Manifest.Formatters {
			out.Formatters = append(out.Formatters, pm.Unit+":"+f)
		}
	}
	if r.URL.Query().Get("draft") != "1" {
		out.Formatters = append(out.Formatters, a.code.AliasNames(m.Formatters)...)
	}
	if demo {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	tables, err := a.code.DeveloperTables(ctx)
	if err != nil {
		serverErr(w, err)
		return
	}
	for _, t := range tables {
		bt := bindTable{Name: t.Name, Columns: []bindColumn{}}
		if d, err := a.code.Data().Describe(ctx, t.Name); err == nil {
			for _, c := range d.Columns {
				bt.Columns = append(bt.Columns, bindColumn{Name: c.Name, Type: c.Type})
			}
		}
		out.Tables = append(out.Tables, bt)
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) codePreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind    string            `json:"kind"`
		Source  string            `json:"source"`
		Path    string            `json:"path"`
		Page    int               `json:"page"`
		PerPage int               `json:"perPage"`
		Params  map[string]any    `json:"params"`
		Table   *design.TableSpec `json:"table"`
		ViewAs  json.RawMessage   `json:"viewAs"`
		Draft   bool              `json:"draft"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	// demo mode: the live preview runs the Store plugins (Site Code is off
	// there); the draft is Site Code's and stays off
	if a.code.EnvDisabled() || (a.code.Demo() && in.Draft) {
		writeJSON(w, http.StatusOK, map[string]any{"error": msgCodeDemo})
		return
	}
	p := auth.From(r)
	if in.Draft && !p.Can(auth.CapEditScripts) {
		writeErr(w, http.StatusForbidden, "ต้องมีสิทธิ์แก้ไขโค้ด (edit_scripts) จึงจะพรีวิวร่างได้")
		return
	}
	var user *sitecode.User
	viewAs, groups, ok := parseViewAs(in.ViewAs)
	if !ok {
		writeErr(w, http.StatusBadRequest, "viewAs ต้องเป็น 'anonymous', 'me' หรือ {groups: [...]}")
		return
	}
	switch viewAs {
	case "", "anonymous", "visitor":
	case "me":
		user = &sitecode.User{ID: p.ID, Name: p.Name, Role: p.Role}
		if u, err := a.st.UserByID(p.ID); err == nil && u != nil {
			user.Email = u.Email
		}
	default:
		writeErr(w, http.StatusBadRequest, "viewAs ต้องเป็น 'anonymous' หรือ 'me'")
		return
	}
	sess, probs, err := a.code.PreviewSession(user, in.Draft)
	if err != nil {
		serverErr(w, err)
		return
	}
	if len(probs) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"error": "build ไม่ผ่าน — แก้ข้อผิดพลาดก่อนพรีวิว", "errors": probs})
		return
	}
	if sess == nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": "ยังไม่มีโค้ดที่เผยแพร่ (หรือ Site Code ปิดอยู่)"})
		return
	}
	defer sess.Close()
	if viewAs == "visitor" {
		sess.SetVisitor(map[string]any{"id": "preview", "name": "Preview", "groups": groups})
	}
	switch in.Kind {
	case "collection":
		if !design.IsCodeSource(in.Source) {
			writeErr(w, http.StatusBadRequest, "source ต้องเป็น code:<name> หรือ table:<name>")
			return
		}
		if in.Params == nil {
			in.Params = map[string]any{}
		}
		res := sess.LoadCollection(design.CollectionReq{Source: in.Source, Page: max(1, in.Page), PerPage: in.PerPage,
			Params: in.Params, Table: in.Table})
		if res.Error != "" {
			writeJSON(w, http.StatusOK, map[string]any{"error": res.Error})
			return
		}
		out := map[string]any{"items": res.Items, "total": res.Total, "perPage": res.PerPage, "page": res.Page}
		if res.Items == nil {
			out["items"] = []any{}
		}
		if res.Denied {
			out["denied"] = true
		}
		writeJSON(w, http.StatusOK, out)
	case "page":
		if in.Path == "" || in.Path[0] != '/' {
			writeErr(w, http.StatusBadRequest, "path ต้องขึ้นต้นด้วย /")
			return
		}
		data := sess.PageData(in.Path)
		if data == nil && sess.LastError != "" {
			writeJSON(w, http.StatusOK, map[string]any{"error": sess.LastError})
			return
		}
		if data == nil {
			data = map[string]any{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	default:
		writeErr(w, http.StatusBadRequest, "kind ต้องเป็น 'collection' หรือ 'page'")
	}
}

// parseViewAs reads preview viewAs: "anonymous" | "me" | {groups: [...]}
// (a visitor in those site.visitor groups, phase 4a).
func parseViewAs(raw json.RawMessage) (kind string, groups []any, ok bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil, true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch s {
		case "", "anonymous", "me":
			return s, nil, true
		}
		return "", nil, false
	}
	var o struct {
		Groups []string `json:"groups"`
	}
	if json.Unmarshal(raw, &o) != nil {
		return "", nil, false
	}
	groups = []any{}
	for _, g := range o.Groups {
		if len(groups) < 50 {
			groups = append(groups, g)
		}
	}
	return "visitor", groups, true
}
