package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

// Custom post types and fields from Site Code (phase 4a) and, since 2.0.5,
// from JS plugins (scope "post-types"; plugin / pluginName set).
//
//	GET /api/v1/post-types   any signed-in user (the admin sidebar)
//	  → {"postTypes":[{type,label,singular,icon,slug,public,supports,fields,access,caps,canEdit,canPublish,plugin?,pluginName?}],
//	     "postFields":{"post":{…}, "page":{…}, "<type>":{…}}}
//
// fields / postFields entries are objects {"<key>": {type, label:{th,en}, required?, min?, max?, options?}}
// in declaration order; postTypes[].fields are the type's own fields, postFields the site.fields additions.

func (a *API) postTypeRoutes(h func(pattern, cap string, fn http.HandlerFunc)) {
	h("GET /api/v1/post-types", "", a.listPostTypes)
}

type postTypeOut struct {
	Type     string                  `json:"type"`
	Label    map[string]string       `json:"label"`
	Singular map[string]string       `json:"singular"`
	Icon     string                  `json:"icon"`
	Slug     string                  `json:"slug"`
	Public   bool                    `json:"public"`
	Supports []string                `json:"supports"`
	Fields   json.RawMessage         `json:"fields"`
	Access   sitecode.PostTypeAccess `json:"access"`
	Caps     sitecode.PostTypeCaps   `json:"caps"`
	CanEdit  bool                    `json:"canEdit"`
	// CanPublish: the caller may publish posts of the type (2.0.6).
	CanPublish bool `json:"canPublish"`
	// Plugin / PluginName: the JS plugin that registered the type (2.0.5).
	Plugin     string `json:"plugin,omitempty"`
	PluginName string `json:"pluginName,omitempty"`
	// Admin: made in Settings → Content types (2.2.2).
	Admin bool `json:"admin,omitempty"`
}

// postCatalog lists the custom post types (with canEdit for me) and the
// site.fields additions per type.
func postCatalog(m sitecode.Manifest, me *auth.Principal) ([]postTypeOut, map[string]json.RawMessage) {
	types := []postTypeOut{}
	for _, t := range m.PostTypes {
		types = append(types, postTypeOut{Type: t.Type, Label: t.Label, Singular: t.Singular, Icon: t.Icon, Slug: t.Slug,
			Public: t.Public, Supports: t.Supports, Fields: sitecode.FieldsObject(t.Fields), Access: t.Access, CanEdit: t.CanEditType(me.Role),
			Caps: t.Caps, CanPublish: t.CanPublishType(me.Role),
			Plugin: t.Plugin, PluginName: t.PluginName, Admin: t.Admin})
	}
	fields := map[string]json.RawMessage{model.TypePost: json.RawMessage("{}"), model.TypePage: json.RawMessage("{}")}
	for typ, list := range m.Fields {
		fields[typ] = sitecode.FieldsObject(list)
	}
	return types, fields
}

func (a *API) listPostTypes(w http.ResponseWriter, r *http.Request) {
	var m sitecode.Manifest
	if a.code != nil {
		m = a.code.PostTypes()
	}
	types, fields := postCatalog(m, auth.From(r))
	writeJSON(w, http.StatusOK, map[string]any{"postTypes": types, "postFields": fields})
}

// prepareSave validates custom fields (in: the request's meta, existing:
// the stored values) and runs Site Code's post.beforeSave filters; it
// writes the error response and returns ok=false when the save is refused.
// The result is the meta patch to store (nil value = remove).
func (a *API) prepareSave(w http.ResponseWriter, me *auth.Principal, p *model.Post, in, existing map[string]any, isNew bool) (map[string]any, bool) {
	fields := a.metaFields(p.Type)
	patch := map[string]any{}
	if len(fields) > 0 && (isNew || in != nil) {
		if in == nil {
			in = map[string]any{}
		}
		clean, err := sitecode.ValidateMeta(fields, in, existing)
		if err != nil {
			writeMetaErr(w, err)
			return nil, false
		}
		patch = clean
	}
	if a.code == nil {
		return patch, true
	}
	merged := map[string]any{}
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range patch {
		if v == nil {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}
	value := map[string]any{"type": p.Type, "title": p.Title, "slug": p.Slug, "status": p.Status,
		"excerpt": p.Excerpt, "content": p.Content, "meta": merged}
	user := &sitecode.User{ID: me.ID, Name: me.Name, Role: me.Role}
	out, msg := a.code.BeforeSave(value, user, isNew)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return nil, false
	}
	if s, ok := out["title"].(string); ok {
		p.Title = strings.TrimSpace(s)
		if p.Title == "" {
			p.Title = "(ไม่มีชื่อ)"
		}
	}
	if s, ok := out["slug"].(string); ok {
		p.Slug = s
	}
	if s, ok := out["status"].(string); ok && model.ValidStatus(s) {
		if s != model.StatusDraft && !a.canPublish(me, p.Type) {
			s = model.StatusDraft
		}
		p.Status = s
	}
	if s, ok := out["excerpt"].(string); ok {
		p.Excerpt = s
	}
	if s, ok := out["content"].(string); ok && s != p.Content {
		p.Content = s
		if !me.Can(auth.CapUnfilteredHTML) {
			p.Content = sanitizeHTML(p.Content)
		}
	}
	if om, ok := out["meta"].(map[string]any); ok && len(fields) > 0 {
		full := map[string]any{}
		for _, f := range fields {
			v, set := om[f.Key]
			if !set {
				v = nil
			}
			full[f.Key] = v
		}
		clean, err := sitecode.ValidateMeta(fields, full, nil)
		if err != nil {
			writeMetaErr(w, err)
			return nil, false
		}
		// only what changed compared with the stored values
		patch = map[string]any{}
		for k, v := range clean {
			if ev, had := existing[k]; !had && v == nil || had && jsonSame(ev, v) {
				continue
			}
			patch[k] = v
		}
	}
	return patch, true
}

func writeMetaErr(w http.ResponseWriter, err error) {
	var me *sitecode.MetaError
	if errors.As(err, &me) {
		out := map[string]any{"error": me.Msg}
		if me.Key != "" {
			out["fields"] = map[string]string{me.Key: me.Msg}
		}
		writeJSON(w, http.StatusBadRequest, out)
		return
	}
	writeErr(w, http.StatusBadRequest, err.Error())
}

func jsonSame(a, b any) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(x) == string(y)
}
