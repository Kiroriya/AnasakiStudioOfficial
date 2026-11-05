package api

// Content types made in the admin without code (CMS 2.2.2, Settings → Content types).
//
//	GET    /api/v1/content-types          manage_options → {types:[spec+{postCount}], taken:[{type, slug, label, owner}]}
//	PUT    /api/v1/content-types/{type}   manage_options, body = spec → the saved spec (creates or replaces)
//	DELETE /api/v1/content-types/{type}   manage_options; refused while posts of the type exist
//
// The specs live in the option content_types; sitecode.ContentFor adds them to the manifest, so the rest of
// the system (menu, editor, routes, designer bindings, db.posts) treats them like site.postType types.

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/sitecode"
)

func (a *API) contentTypeRoutes(h func(pattern, cap string, fn http.HandlerFunc)) {
	h("GET /api/v1/content-types", auth.CapManageOptions, a.listContentTypes)
	h("PUT /api/v1/content-types/{type}", auth.CapManageOptions, a.saveContentType)
	h("DELETE /api/v1/content-types/{type}", auth.CapManageOptions, a.deleteContentType)
}

type contentTypeOut struct {
	sitecode.TypeSpec
	PostCount int  `json:"postCount"`
	Active    bool `json:"active"` // false: Site Code or a plugin took its id or slug, it is left out
}

type takenType struct {
	Type  string            `json:"type"`
	Slug  string            `json:"slug"`
	Label map[string]string `json:"label"`
	Owner string            `json:"owner"` // "code" | "plugin"
}

// codeOwnedTypes are the types of Site Code and the running plugins (not the admin ones).
func (a *API) codeOwnedTypes(opts map[string]string) []sitecode.PostTypeInfo {
	if a.code == nil {
		return nil
	}
	return a.code.CodeOwnedTypes(opts)
}

func (a *API) postCount(typ string) int {
	_, total, err := a.st.ListPosts(model.PostFilter{Type: typ, Limit: 1, NoContent: true})
	if err != nil {
		return 0
	}
	return total
}

func (a *API) listContentTypes(w http.ResponseWriter, r *http.Request) {
	opts, err := a.st.Options()
	if err != nil {
		serverErr(w, err)
		return
	}
	others := a.codeOwnedTypes(opts)
	types := []contentTypeOut{}
	for _, s := range sitecode.ParseTypeSpecs(opts[sitecode.OptContentTypes]) {
		active := true
		for _, t := range others {
			if t.Type == s.Type || t.Slug == s.Slug {
				active = false
			}
		}
		types = append(types, contentTypeOut{TypeSpec: s, PostCount: a.postCount(s.Type), Active: active})
	}
	taken := []takenType{}
	for _, t := range others {
		owner := "code"
		if t.Plugin != "" {
			owner = "plugin"
		}
		taken = append(taken, takenType{Type: t.Type, Slug: t.Slug, Label: t.Label, Owner: owner})
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": types, "taken": taken})
}

func (a *API) saveContentType(w http.ResponseWriter, r *http.Request) {
	var in sitecode.TypeSpec
	if !readJSON(w, r, &in) {
		return
	}
	in.Type = strings.TrimSpace(in.Type)
	if in.Type != r.PathValue("type") {
		writeErr(w, http.StatusBadRequest, "รหัสประเภทในที่อยู่และในข้อมูลต้องตรงกัน")
		return
	}
	info, err := sitecode.ValidateTypeSpec(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	opts, err := a.st.Options()
	if err != nil {
		serverErr(w, err)
		return
	}
	list := sitecode.ParseTypeSpecs(opts[sitecode.OptContentTypes])
	at := -1
	for i, s := range list {
		if s.Type == info.Type {
			at = i
		} else if strings.EqualFold(s.Slug, info.Slug) || (s.Slug == "" && strings.ReplaceAll(s.Type, "_", "-") == info.Slug) {
			writeErr(w, http.StatusBadRequest, "ที่อยู่ /"+info.Slug+" ถูกใช้โดยประเภท "+s.Type+" แล้ว")
			return
		}
	}
	if at < 0 && len(list) >= sitecode.MaxAdminTypes {
		writeErr(w, http.StatusBadRequest, "สร้างประเภทเนื้อหาได้ไม่เกิน 30 ประเภท")
		return
	}
	for _, t := range a.codeOwnedTypes(opts) {
		if t.Type == info.Type || t.Slug == info.Slug {
			owner := "Site Code"
			if t.Plugin != "" {
				owner = "ปลั๊กอิน " + orDefault(t.PluginName, t.Plugin)
			}
			writeErr(w, http.StatusBadRequest, "รหัสหรือที่อยู่นี้ถูกใช้โดย "+owner+" (ประเภท "+t.Type+")")
			return
		}
	}
	if msg := a.typeSlugProblem(info.Slug, opts); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	spec := sitecode.SpecOf(info)
	if at >= 0 {
		list[at] = spec
	} else {
		list = append(list, spec)
	}
	b, _ := json.Marshal(list)
	if err := a.st.SetOptions(map[string]string{sitecode.OptContentTypes: string(b)}); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contentTypeOut{TypeSpec: spec, PostCount: a.postCount(spec.Type), Active: true})
}

// typeSlugProblem: the type's address must not take a path of the system, a plugin, the admin, the blog or
// category bases, or an existing page ("" = free).
func (a *API) typeSlugProblem(slug string, opts map[string]string) string {
	if id, ok := a.reservedPaths()[slug]; ok {
		if id == "" {
			return "ที่อยู่ /" + slug + " ถูกสงวนไว้โดยระบบ"
		}
		return "ที่อยู่ /" + slug + " ถูกใช้โดยปลั๊กอิน " + id
	}
	merged := withDefaults(opts)
	for _, k := range []string{"admin_path", "blog_base", "category_base"} {
		if merged[k] == slug {
			return "ที่อยู่ /" + slug + " ซ้ำกับการตั้งค่าลิงก์ของเว็บ (" + k + ")"
		}
	}
	if p, err := a.st.PostBySlug(model.TypePage, slug, false); err == nil && p != nil {
		return "มีหน้าเว็บ /" + slug + " อยู่แล้ว — เปลี่ยนที่อยู่ของประเภทหรือของหน้านั้นก่อน"
	}
	return ""
}

func (a *API) deleteContentType(w http.ResponseWriter, r *http.Request) {
	typ := r.PathValue("type")
	opts, err := a.st.Options()
	if err != nil {
		serverErr(w, err)
		return
	}
	list := sitecode.ParseTypeSpecs(opts[sitecode.OptContentTypes])
	out := list[:0:0]
	found := false
	for _, s := range list {
		if s.Type == typ {
			found = true
			continue
		}
		out = append(out, s)
	}
	if !found {
		writeErr(w, http.StatusNotFound, "ไม่พบประเภทเนื้อหานี้")
		return
	}
	if n := a.postCount(typ); n > 0 {
		writeErr(w, http.StatusConflict, "ยังมีเนื้อหาของประเภทนี้อยู่ — ลบเนื้อหาทั้งหมดก่อน แล้วจึงลบประเภท")
		return
	}
	b, _ := json.Marshal(out)
	if err := a.st.SetOptions(map[string]string{sitecode.OptContentTypes: string(b)}); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": typ})
}
