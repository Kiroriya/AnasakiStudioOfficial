package sitecode

// Plugin capabilities in code (since 2.0.6). A JS plugin declares
// capabilities in its manifest ("capabilities": [{"id": "view", …}]); the
// core registers them as "<plugin id>.<id>" and the administrator decides
// per role which ones each role holds (Users → Roles). Code uses them:
//
//   - access lists accept "cap:<capability>" entries next to role slugs:
//     site.route / collection / action / page, site.adminPage,
//     site.dashboardWidget, site.adminNotice and a post type's access.edit;
//   - ctx.user.can('analytics.view') and site.can(user, 'analytics.view');
//   - site.postType(id, { caps: { edit: 'docs.edit', publish: 'docs.publish' } }).
//
// A plugin may only name its own capabilities ("<own id>.*", declared in the
// manifest) and core capabilities (edit_pages, manage_options, …); naming
// another plugin's is refused. Site Code (the site's own code) may name any.

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/auth"
)

// CapPrefix starts a capability entry of an access list ("cap:docs.edit").
const CapPrefix = "cap:"

// capEntry returns the capability of a "cap:" access entry.
func capEntry(a string) (string, bool) { return strings.CutPrefix(a, CapPrefix) }

// validCapEntry reports a syntactically valid "cap:<capability>" entry.
func validCapEntry(a string) bool {
	c, ok := capEntry(a)
	return ok && auth.ValidCapName(c)
}

// adminEntry reports a valid entry of an admin-only access list (admin
// pages, notices, widgets, post type editors): a role slug or a capability.
func adminEntry(a string) bool { return accessRe.MatchString(a) || validCapEntry(a) }

// capProblem says why unit u may not name capability c ("" = it may).
func (u *unitInfo) capProblem(c string) string {
	if !auth.ValidCapName(c) {
		return fmt.Sprintf("%q ไม่ใช่ชื่อสิทธิ์ที่ถูกต้อง (เช่น 'edit_pages' หรือ '<รหัสปลั๊กอิน>.view')", c)
	}
	if !u.plugin() {
		return ""
	}
	if pl, _, ok := auth.SplitPluginCap(c); ok {
		if pl != u.ID {
			return fmt.Sprintf("ปลั๊กอินใช้ได้เฉพาะสิทธิ์ของตัวเอง (%s.*) และสิทธิ์ของระบบ — %q เป็นสิทธิ์ของปลั๊กอิน %s / a plugin may only use its own capabilities", u.ID, c, pl)
		}
		if !u.caps[c] {
			return fmt.Sprintf("%q ไม่ได้ประกาศไว้ใน \"capabilities\" ของ manifest / not declared in the manifest", c)
		}
		return ""
	}
	if !auth.IsCoreCap(c) {
		return fmt.Sprintf("%q ไม่ใช่สิทธิ์ของระบบ — สิทธิ์ของปลั๊กอินต้องเป็น '%s.<id>' / not a core capability", c, u.ID)
	}
	return ""
}

// vetCaps throws for "cap:" entries of list the running unit may not name.
func (v *vm) vetCaps(list []string, what string) {
	for _, a := range list {
		if c, ok := capEntry(a); ok {
			if p := v.u.capProblem(c); p != "" {
				v.throw("%s: access: %s", what, p)
			}
		}
	}
}

// vetCap throws when the running unit may not name capability c.
func (v *vm) vetCap(c, what string) {
	if p := v.u.capProblem(c); p != "" {
		v.throw("%s: %s", what, p)
	}
}

// checkCanArg is ctx.user.can / site.can's rule: core-style names keep the
// old behaviour (unknown = false); a plugin capability must be the unit's
// own, declared one.
func (v *vm) checkCanArg(c, what string) {
	if _, _, ok := auth.SplitPluginCap(c); ok {
		v.vetCap(c, what)
	}
}

// userCaps is ctx.user.caps: a plugin sees the core capabilities and its own.
func (v *vm) userCaps(role string) []string {
	if v.u.plugin() {
		return auth.CapsFor(role, v.u.ID)
	}
	return auth.Caps(role)
}

// siteCan is site.can(user, capability): whether user (ctx.user, or any
// object with a role) holds the capability; null → false.
func (v *vm) siteCan(c goja.FunctionCall) goja.Value {
	const what = "site.can"
	capName := c.Argument(1).String()
	if goja.IsUndefined(c.Argument(1)) || capName == "" {
		v.throw("%s(user, capability): ต้องระบุสิทธิ์ เช่น '%s.view'", what, v.u.id())
	}
	v.checkCanArg(capName, what)
	u := c.Argument(0)
	if u == nil || goja.IsUndefined(u) || goja.IsNull(u) {
		return v.rt.ToValue(false)
	}
	o := u.ToObject(v.rt)
	role := ""
	if r := o.Get("role"); r != nil && !goja.IsUndefined(r) && !goja.IsNull(r) {
		role = r.String()
	}
	if role == "" {
		return v.rt.ToValue(false)
	}
	return v.rt.ToValue(auth.RoleHas(role, capName))
}

// capOpt reads an optional capability name o[key] the unit may use.
func (v *vm) capOpt(o *goja.Object, key, what string) string {
	x := o.Get(key)
	if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
		return ""
	}
	c, ok := x.Export().(string)
	if !ok || c == "" {
		v.throw("%s: caps.%s ต้องเป็นชื่อสิทธิ์ เช่น '%s.edit'", what, key, v.u.id())
	}
	v.vetCap(c, what+": caps."+key)
	return c
}
