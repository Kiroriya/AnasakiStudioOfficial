package api

import (
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/seed"
)

var roleSlugRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)

// reloadRoles refreshes the capability cache from the roles table.
func (a *API) reloadRoles() error {
	m, err := a.st.RoleCaps()
	if err != nil {
		return err
	}
	if len(m) == 0 {
		return nil // roles not seeded yet: keep the built-in defaults
	}
	auth.SetRoles(m)
	return nil
}

func (a *API) listRoles(w http.ResponseWriter, r *http.Request) {
	rs, err := a.st.ListRoles()
	if err != nil {
		serverErr(w, err)
		return
	}
	for i := range rs {
		if rs[i].Slug == model.RoleAdmin {
			rs[i].Caps = auth.Caps(model.RoleAdmin) // administrator has every capability
		}
	}
	builtin := []string{}
	for _, b := range auth.BuiltinRoles() {
		builtin = append(builtin, b.Slug)
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": rs, "caps": auth.Catalog(), "builtin": builtin})
}

// cleanCaps validates caps against the catalog and de-duplicates them.
func cleanCaps(caps []string) ([]string, string) {
	known := map[string]bool{}
	for _, c := range auth.Catalog() {
		known[c.Key] = true
	}
	seen := map[string]bool{}
	out := []string{}
	for _, c := range caps {
		if !known[c] {
			return nil, "ไม่รู้จักสิทธิ์: " + c
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out, ""
}

func (a *API) createRole(w http.ResponseWriter, r *http.Request) {
	var in model.Role
	if !readJSON(w, r, &in) {
		return
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Name = strings.TrimSpace(in.Name)
	if !roleSlugRe.MatchString(in.Slug) {
		writeErr(w, http.StatusBadRequest, "รหัสบทบาทต้องเป็น a-z, 0-9, - หรือ _ (2-32 ตัว)")
		return
	}
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "กรุณาใส่ชื่อ")
		return
	}
	caps, msg := cleanCaps(in.Caps)
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if me := auth.From(r); me.Role != model.RoleAdmin && !holdsAll(me, caps) {
		writeErr(w, http.StatusForbidden, errRoleTooHigh)
		return
	}
	in.Caps = caps
	if err := a.st.CreateRole(&in); err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeErr(w, http.StatusConflict, "มีบทบาทนี้แล้ว")
			return
		}
		serverErr(w, err)
		return
	}
	a.respondRole(w, in.Slug, http.StatusCreated)
}

func (a *API) updateRole(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name *string  `json:"name"`
		Caps []string `json:"caps"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	role, err := a.st.GetRole(r.PathValue("slug"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if role.Slug == model.RoleAdmin {
		writeErr(w, http.StatusBadRequest, "ไม่สามารถแก้ไขบทบาทผู้ดูแลระบบได้")
		return
	}
	me := auth.From(r)
	if !canManageRole(me, role.Slug) {
		writeErr(w, http.StatusForbidden, errRoleTooHigh)
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		role.Name = strings.TrimSpace(*in.Name)
	}
	if in.Caps != nil {
		caps, msg := cleanCaps(in.Caps)
		if msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		if me.Role != model.RoleAdmin && !holdsAll(me, caps) {
			writeErr(w, http.StatusForbidden, errRoleTooHigh)
			return
		}
		role.Caps = caps
	}
	if err := a.st.UpdateRole(role); err != nil {
		serverErr(w, err)
		return
	}
	a.respondRole(w, role.Slug, http.StatusOK)
}

func (a *API) deleteRole(w http.ResponseWriter, r *http.Request) {
	role, err := a.st.GetRole(r.PathValue("slug"))
	if err != nil {
		serverErr(w, err)
		return
	}
	if role.Slug == model.RoleAdmin || role.Slug == model.RoleSubscriber {
		// administrators can never lock themselves out; subscriber takes the users of deleted roles
		writeErr(w, http.StatusBadRequest, "ตำแหน่งผู้ดูแลระบบและสมาชิกลบไม่ได้")
		return
	}
	if role.Slug == auth.From(r).Role {
		writeErr(w, http.StatusBadRequest, "ไม่สามารถลบบทบาทของตัวเองได้")
		return
	}
	if !canManageRole(auth.From(r), role.Slug) {
		writeErr(w, http.StatusForbidden, errRoleTooHigh)
		return
	}
	if err := a.st.DeleteRole(role.Slug, model.RoleSubscriber); err != nil {
		serverErr(w, err)
		return
	}
	if role.Builtin { // remember it, or the next start would re-create it
		opts, err := a.st.Options()
		if err != nil {
			serverErr(w, err)
			return
		}
		list := []string{}
		for _, s := range strings.Split(opts[seed.RolesRemovedOption], ",") {
			if s = strings.TrimSpace(s); s != "" && s != role.Slug {
				list = append(list, s)
			}
		}
		list = append(list, role.Slug)
		if err := a.st.SetOptions(map[string]string{seed.RolesRemovedOption: strings.Join(list, ",")}); err != nil {
			serverErr(w, err)
			return
		}
	}
	if err := a.reloadRoles(); err != nil {
		log.Printf("reload roles: %v", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// restoreRoles brings back deleted built-in roles (with their default
// permissions): POST /roles/restore → the role list.
func (a *API) restoreRoles(w http.ResponseWriter, r *http.Request) {
	if err := a.st.SetOptions(map[string]string{seed.RolesRemovedOption: ""}); err != nil {
		serverErr(w, err)
		return
	}
	if err := seed.Roles(a.st); err != nil {
		serverErr(w, err)
		return
	}
	if err := a.reloadRoles(); err != nil {
		serverErr(w, err)
		return
	}
	a.listRoles(w, r)
}

// respondRole reloads the capability cache and answers with the role.
func (a *API) respondRole(w http.ResponseWriter, slug string, status int) {
	if err := a.reloadRoles(); err != nil {
		serverErr(w, err)
		return
	}
	role, err := a.st.GetRole(slug)
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, status, role)
}
