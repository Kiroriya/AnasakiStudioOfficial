package api

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// CMS 2.0.6: plugin capabilities. A manifest declares "capabilities"; the
// core registers them as "<plugin>.<id>", grants the defaults once (install /
// upgrade of a capability never seen before), keeps the administrator's
// choices across upgrades, removes them on uninstall (reinstall: no re-grant)
// and enforces them in every access path of a JS plugin (routes, admin pages,
// widgets, notices, actions, post types + REST /posts). A plugin may only name
// its own capabilities.

type capKitOpts struct {
	id, version string
	caps        []map[string]any
	main        string
}

func capKitPackage(o capKitOpts) map[string]string {
	if o.id == "" {
		o.id = "cap-kit"
	}
	if o.version == "" {
		o.version = "1.0.0"
	}
	m := map[string]any{"id": o.id, "name": "Cap Kit", "version": o.version, "runtime": "js", "permissions": []any{"code"},
		"capabilities": o.caps,
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"routes", "admin-pages", "content", "content-write", "post-types"},
			"routes": []any{"/" + strings.ReplaceAll(o.id, "-", "")}}}
	b, _ := json.Marshal(m)
	return map[string]string{"manifest.json": string(b), "code/main.ts": o.main}
}

func capDecl(id string, def ...string) map[string]any {
	return map[string]any{"id": id, "label": map[string]any{"th": "สิทธิ์ " + id, "en": "Cap " + id},
		"description": map[string]any{"th": "คำอธิบาย " + id, "en": "About " + id}, "default": def}
}

const capKitMain = `
site.route('GET /capkit/view', { access: ['cap:cap-kit.view'] }, () => 'seen')
site.route('GET /capkit/can', { access: ['administrator', 'editor', 'author', 'contributor'] }, ({ user }) => ({
  view: user.can('cap-kit.view'), edit: site.can(user, 'cap-kit.edit'), core: user.can('edit_pages'),
  caps: user.caps, nobody: site.can(null, 'cap-kit.view'),
}))
site.adminPage('kit', { label: 'Kit', access: ['cap:cap-kit.view'], render: () => ({ html: '<p id="kit">kit</p>' }) })
site.dashboardWidget('w', { label: 'W', access: ['cap:cap-kit.view'], render: () => ({ html: '<p>w</p>' }) })
site.adminNotice(() => ({ level: 'info', text: 'edit-notice' }), { access: ['cap:cap-kit.edit'] })
site.action('act', { input: {}, access: ['cap:cap-kit.edit'], run: () => ({ data: { done: true } }) })
site.postType('cap_kit_doc', { label: 'Kit docs', slug: 'capkit', public: false, caps: { edit: 'cap-kit.edit', publish: 'cap-kit.publish' } })
`

func capKitV1() map[string]string {
	return capKitPackage(capKitOpts{caps: []map[string]any{
		capDecl("view", "administrator", "editor"), capDecl("edit", "administrator", "author"), capDecl("publish", "administrator"),
	}, main: capKitMain})
}

func TestPluginCapabilities(t *testing.T) {
	e := offBNew(t)
	admin, user := setupSite(t, e.c)
	editor, author, contrib := user("Ed", "editor"), user("Au", "author"), user("Co", "contributor")

	roleCaps := func(slug string) []string {
		t.Helper()
		code, out := admin.do("GET", "/api/v1/roles", nil)
		if code != 200 {
			t.Fatalf("roles: %d %v", code, out)
		}
		for _, r := range out["roles"].([]any) {
			if m := r.(map[string]any); m["slug"] == slug {
				var l []string
				for _, c := range m["caps"].([]any) {
					l = append(l, c.(string))
				}
				return l
			}
		}
		t.Fatalf("no role %s", slug)
		return nil
	}
	catalog := func() map[string]map[string]any {
		t.Helper()
		_, out := admin.do("GET", "/api/v1/roles", nil)
		m := map[string]map[string]any{}
		for _, c := range out["caps"].([]any) {
			x := c.(map[string]any)
			m[x["key"].(string)] = x
		}
		return m
	}
	setRole := func(slug string, caps []string) {
		t.Helper()
		if code, out := admin.do("PUT", "/api/v1/roles/"+slug, map[string]any{"caps": caps}); code != 200 {
			t.Fatalf("save role %s: %d %v", slug, code, out)
		}
	}
	has := func(slug, c string) bool { return slices.Contains(roleCaps(slug), c) }
	status := func(c *client, path string) int {
		t.Helper()
		return c.send("GET", path, nil, map[string]string{"Accept": "application/json"}).Code
	}

	// manifest validation
	for _, bad := range []struct {
		caps []map[string]any
		want string
	}{
		{[]map[string]any{{"id": "View", "label": "x"}}, "capabilities"},
		{[]map[string]any{{"id": "view", "label": "x"}, {"id": "view", "label": "y"}}, "twice"},
		{[]map[string]any{{"id": "view"}}, "label"},
		{[]map[string]any{{"id": "view", "label": "x", "default": []any{"Bad Role"}}}, "default"},
	} {
		if code, out := uploadPkg(t, admin, "", capKitPackage(capKitOpts{caps: bad.caps, main: `site.route('GET /capkit/x', { access: ['*'] }, () => 'x')`})); code != 400 ||
			!strings.Contains(fmt.Sprint(out), bad.want) {
			t.Fatalf("bad capabilities %v accepted: %d %v", bad.caps, code, out)
		}
	}

	// install: defaults granted, catalog entries (th/en labels, plugin section), administrator has all
	mustUpload(t, admin, capKitV1())
	pkg := pkgOf(t, admin, "cap-kit")
	if pc, _ := pkg["capabilities"].([]any); len(pc) != 3 || pc[0].(map[string]any)["key"] != "cap-kit.view" {
		t.Fatalf("installed package capabilities: %v", pkg["capabilities"])
	}
	cat := catalog()
	if v := cat["cap-kit.view"]; v == nil || v["plugin"] != "cap-kit" || v["pluginName"] != "Cap Kit" || v["group"] != "plugins" ||
		v["label"].(map[string]any)["en"] != "Cap view" || v["description"].(map[string]any)["th"] != "คำอธิบาย view" {
		t.Fatalf("catalog: %v", v)
	}
	if !has("editor", "cap-kit.view") || has("editor", "cap-kit.edit") || !has("author", "cap-kit.edit") || has("author", "cap-kit.view") ||
		has("contributor", "cap-kit.view") || !has("administrator", "cap-kit.publish") {
		t.Fatalf("defaults: editor %v author %v", roleCaps("editor"), roleCaps("author"))
	}
	activate(t, admin, "cap-kit")

	// routes: cap: entries
	if status(editor, "/capkit/view") != 200 || status(author, "/capkit/view") != 403 || status(admin, "/capkit/view") != 200 ||
		status(e.c, "/capkit/view") != 401 {
		t.Fatalf("route access: editor %d author %d admin %d anon %d", status(editor, "/capkit/view"), status(author, "/capkit/view"),
			status(admin, "/capkit/view"), status(e.c, "/capkit/view"))
	}
	// ctx.user.can / site.can / ctx.user.caps (core + own only)
	_, can := getJSON(t, editor, "GET", "/capkit/can", "", nil)
	if can["view"] != true || can["edit"] != false || can["core"] != true || can["nobody"] != false ||
		!strings.Contains(fmt.Sprint(can["caps"]), "cap-kit.view") {
		t.Fatalf("editor can: %v", can)
	}
	if _, can := getJSON(t, author, "GET", "/capkit/can", "", nil); can["view"] != false || can["edit"] != true || can["core"] != false {
		t.Fatalf("author can: %v", can)
	}
	// admin pages, widgets, notices
	listIDs := func(c *client, path string) string {
		t.Helper()
		_, l := c.doAny("GET", path, nil)
		var ids []string
		for _, x := range l.([]any) {
			m := x.(map[string]any)
			ids = append(ids, fmt.Sprint(m["id"], m["text"]))
		}
		return strings.Join(ids, ",")
	}
	if !strings.Contains(listIDs(editor, "/api/v1/code/admin-pages"), "cap-kit:kit") || strings.Contains(listIDs(author, "/api/v1/code/admin-pages"), "cap-kit:kit") {
		t.Fatalf("admin pages: editor %s author %s", listIDs(editor, "/api/v1/code/admin-pages"), listIDs(author, "/api/v1/code/admin-pages"))
	}
	if code, _ := author.do("POST", "/api/v1/code/admin-pages/cap-kit:kit/ticket", map[string]any{}); code == 200 {
		t.Fatal("author opened the admin page")
	}
	if !strings.Contains(listIDs(editor, "/api/v1/code/dashboard-widgets"), "cap-kit:w") || listIDs(author, "/api/v1/code/dashboard-widgets") != "" {
		t.Fatalf("widgets: editor %s author %s", listIDs(editor, "/api/v1/code/dashboard-widgets"), listIDs(author, "/api/v1/code/dashboard-widgets"))
	}
	if !strings.Contains(listIDs(author, "/api/v1/code/admin-notices"), "edit-notice") || strings.Contains(listIDs(editor, "/api/v1/code/admin-notices"), "edit-notice") {
		t.Fatalf("notices: author %s editor %s", listIDs(author, "/api/v1/code/admin-notices"), listIDs(editor, "/api/v1/code/admin-notices"))
	}
	// actions (from the admin page: the editor may open it, but "act" needs cap-kit.edit)
	_, call := offAAdminPage(t, e.pluginEnv, editor, "cap-kit:kit", "")
	if code, out := call("cap-kit:act", map[string]any{}); code == 200 {
		t.Fatalf("editor ran an action needing cap-kit.edit: %d %v", code, out)
	}
	// an administrator's choice: the editor gets cap-kit.edit
	setRole("editor", append(roleCaps("editor"), "cap-kit.edit"))
	if code, out := call("cap-kit:act", map[string]any{}); code != 200 || out["data"].(map[string]any)["done"] != true {
		t.Fatalf("editor with cap-kit.edit: %d %v", code, out)
	}
	if !strings.Contains(listIDs(editor, "/api/v1/code/admin-notices"), "edit-notice") {
		t.Fatal("notice after the grant")
	}

	// post types: caps.edit / caps.publish on the REST endpoints
	_, pt := editor.do("GET", "/api/v1/post-types", nil)
	var kitType map[string]any
	for _, x := range pt["postTypes"].([]any) {
		if m := x.(map[string]any); m["type"] == "cap_kit_doc" {
			kitType = m
		}
	}
	if kitType == nil || kitType["canEdit"] != true || kitType["canPublish"] != false || kitType["caps"].(map[string]any)["publish"] != "cap-kit.publish" ||
		fmt.Sprint(kitType["access"].(map[string]any)["edit"]) != "[cap:cap-kit.edit]" {
		t.Fatalf("post type: %v", kitType)
	}
	if code, out := contrib.do("POST", "/api/v1/posts", map[string]any{"type": "cap_kit_doc", "title": "No"}); code != 403 {
		t.Fatalf("contributor created a kit doc: %d %v", code, out)
	}
	if code, _ := contrib.do("GET", "/api/v1/posts?type=cap_kit_doc", nil); code != 403 {
		t.Fatal("contributor listed kit docs")
	}
	code, doc := author.do("POST", "/api/v1/posts", map[string]any{"type": "cap_kit_doc", "title": "Draft only", "status": "publish"})
	if code != 201 || doc["status"] != "pending" { // 2.5.0: sent for review

		t.Fatalf("author (edit, no publish): %d %v", code, doc)
	}
	setRole("author", append(roleCaps("author"), "cap-kit.publish"))
	if code, out := author.do("PUT", "/api/v1/posts/"+id(doc), map[string]any{"status": "publish"}); code != 200 || out["status"] != "publish" {
		t.Fatalf("author with cap-kit.publish: %d %v", code, out)
	}
	setRole("author", slices.DeleteFunc(roleCaps("author"), func(c string) bool { return c == "cap-kit.publish" }))
	if code, _ := author.do("PUT", "/api/v1/posts/"+id(doc), map[string]any{"title": "changed"}); code != 403 {
		t.Fatal("a published kit doc changed without cap-kit.publish")
	}
	if code, out := editor.do("PUT", "/api/v1/posts/"+id(doc), map[string]any{"title": "x"}); code != 403 {
		t.Fatalf("editor without publish changed a published doc: %d %v", code, out)
	}

	// cross-plugin capabilities are refused (registration and runtime), undeclared own ones too
	failAct := func(main, want string) {
		t.Helper()
		mustUpload(t, admin, capKitPackage(capKitOpts{id: "cap-oth", caps: []map[string]any{capDecl("own", "editor")}, main: main}))
		code, out := admin.do("POST", "/api/v1/package-plugins/cap-oth/activate", map[string]any{})
		if code == 200 || !strings.Contains(fmt.Sprint(out), want) {
			t.Fatalf("activation should fail with %q: %d %v", want, code, out)
		}
		admin.do("DELETE", "/api/v1/package-plugins/cap-oth", nil)
	}
	failAct(`site.route('GET /capoth', { access: ['cap:cap-kit.view'] }, () => 'x')`, "cap-kit")
	failAct(`site.adminPage('p', { label: 'P', access: ['cap:cap-kit.view'], render: () => ({ html: 'x' }) })`, "cap-kit")
	failAct(`site.dashboardWidget('w', { label: 'W', access: ['cap:cap-kit.view'], render: () => ({ html: 'x' }) })`, "cap-kit")
	failAct(`site.adminNotice(() => null, { access: ['cap:cap-kit.edit'] })`, "cap-kit")
	failAct(`site.action('a', { input: {}, access: ['cap:cap-kit.edit'], run: () => ({}) })`, "cap-kit")
	failAct(`site.postType('cap_oth_doc', { caps: { edit: 'cap-kit.edit' } })`, "cap-kit")
	failAct(`site.route('GET /capoth', { access: ['cap:cap-oth.nope'] }, () => 'x')`, "nope")
	failAct(`site.route('GET /capoth', { access: ['cap:not_a_core_cap'] }, () => 'x')`, "not_a_core_cap")
	mustUpload(t, admin, capKitPackage(capKitOpts{id: "cap-oth", caps: []map[string]any{capDecl("own", "editor")}, main: `
site.route('GET /capoth/probe', { access: ['cap:cap-oth.own', 'cap:edit_pages'] }, ({ user }) => {
  const out = { own: user.can('cap-oth.own'), core: user.can('edit_pages'), caps: user.caps };
  try { out.other = user.can('cap-kit.view') } catch (e) { out.other = 'refused: ' + e.message }
  try { out.site = site.can(user, 'cap-kit.view') } catch (e) { out.site = 'refused: ' + e.message }
  return out;
})`}))
	activate(t, admin, "cap-oth")
	// cap-oth.own was registered by the failed attempts above: its default is not granted again
	if has("editor", "cap-oth.own") {
		t.Fatal("a reinstall re-granted a capability seen before")
	}
	setRole("editor", append(roleCaps("editor"), "cap-oth.own"))
	_, probe := getJSON(t, editor, "GET", "/capoth/probe", "", nil)
	if probe["own"] != true || probe["core"] != true || !strings.HasPrefix(fmt.Sprint(probe["other"]), "refused") ||
		!strings.HasPrefix(fmt.Sprint(probe["site"]), "refused") || strings.Contains(fmt.Sprint(probe["caps"]), "cap-kit.") {
		t.Fatalf("cross-plugin probe: %v", probe)
	}
	admin.do("DELETE", "/api/v1/package-plugins/cap-oth", nil)

	// upgrade: the administrator's choices stay (editor lost view), a new capability gets its defaults
	setRole("editor", slices.DeleteFunc(roleCaps("editor"), func(c string) bool { return c == "cap-kit.view" }))
	if status(editor, "/capkit/view") != 403 {
		t.Fatal("editor without cap-kit.view still opens the route")
	}
	v2 := capKitPackage(capKitOpts{version: "1.1.0", caps: []map[string]any{
		capDecl("view", "administrator", "editor"), capDecl("edit", "administrator", "author"), capDecl("export", "editor"),
	}, main: capKitMain + `site.route('GET /capkit/export', { access: ['cap:cap-kit.export'] }, () => 'csv')`})
	// v2 drops "publish": the post type still names it → the code no longer activates; keep caps.publish out
	v2["code/main.ts"] = strings.Replace(v2["code/main.ts"], ", publish: 'cap-kit.publish'", "", 1)
	mustUpload(t, admin, v2)
	if has("editor", "cap-kit.view") || !has("editor", "cap-kit.edit") || !has("editor", "cap-kit.export") || has("author", "cap-kit.publish") {
		t.Fatalf("after upgrade: editor %v author %v", roleCaps("editor"), roleCaps("author"))
	}
	if _, ok := catalog()["cap-kit.publish"]; ok {
		t.Fatal("a capability the new version dropped is still offered")
	}
	if status(editor, "/capkit/export") != 200 || status(editor, "/capkit/view") != 403 {
		t.Fatalf("after upgrade: export %d view %d", status(editor, "/capkit/export"), status(editor, "/capkit/view"))
	}

	// Site Code (the site's own code) may name any capability, a plugin's too
	publishCode(t, admin, map[string]string{"main.ts": `site.route('GET /sc/caps', { access: ['cap:cap-kit.export'] }, ({ user }) => ({ e: user.can('cap-kit.export'), v: site.can(user, 'cap-kit.view') }))`})
	if code, out := getJSON(t, editor, "GET", "/sc/caps", "", nil); code != 200 || out["e"] != true || out["v"] != false {
		t.Fatalf("site code caps: %d %v", code, out)
	}
	if status(author, "/sc/caps") != 403 {
		t.Fatal("site code cap route open to the author")
	}

	// uninstall: the capabilities leave every role (other capabilities stay)
	if code, out := admin.do("DELETE", "/api/v1/package-plugins/cap-kit", nil); code != 200 && code != 204 {
		t.Fatalf("uninstall: %d %v", code, out)
	}
	for _, r := range []string{"editor", "author", "contributor", "subscriber"} {
		for _, c := range roleCaps(r) {
			if strings.HasPrefix(c, "cap-kit.") {
				t.Fatalf("%s keeps %s after uninstall", r, c)
			}
		}
	}
	if !has("editor", "edit_pages") || !has("author", "publish_posts") {
		t.Fatal("uninstall removed core capabilities")
	}
	for k := range catalog() {
		if strings.HasPrefix(k, "cap-kit.") {
			t.Fatalf("catalog keeps %s", k)
		}
	}
	// a role saved with a leftover plugin capability: dropped, not refused
	setRole("editor", append(roleCaps("editor"), "cap-kit.view"))
	if has("editor", "cap-kit.view") {
		t.Fatal("leftover plugin capability stored")
	}

	// reinstall: capabilities seen before are not granted again; a new one is
	v3 := capKitPackage(capKitOpts{version: "1.2.0", caps: []map[string]any{
		capDecl("view", "administrator", "editor"), capDecl("edit", "administrator", "author"), capDecl("report", "author"),
	}, main: `site.route('GET /capkit/view', { access: ['cap:cap-kit.view'] }, () => 'seen')`})
	mustUpload(t, admin, v3)
	if has("editor", "cap-kit.view") || has("author", "cap-kit.edit") || !has("author", "cap-kit.report") {
		t.Fatalf("after reinstall: editor %v author %v", roleCaps("editor"), roleCaps("author"))
	}
	activate(t, admin, "cap-kit")
	if status(editor, "/capkit/view") != 403 || status(admin, "/capkit/view") != 200 {
		t.Fatal("reinstall re-granted view")
	}

	// a deleted built-in role, restored: it gets the plugin defaults that name it
	if code, _ := admin.do("DELETE", "/api/v1/roles/author", nil); code != 204 {
		t.Fatal("delete author role")
	}
	if code, _ := admin.do("POST", "/api/v1/roles/restore", map[string]any{}); code != 200 {
		t.Fatal("restore roles")
	}
	if !has("author", "cap-kit.edit") || !has("author", "cap-kit.report") || has("author", "cap-kit.view") {
		t.Fatalf("restored author: %v", roleCaps("author"))
	}
}

// Official plugins (2.0.6): their capabilities and the default roles keep the former behaviour.
func TestOfficialPluginCapabilities(t *testing.T) {
	e := offBNew(t)
	admin, user := setupSite(t, e.c)
	editor, author := user("Ed", "editor"), user("Au", "author")
	offAInstall(t, admin, "analytics", "seo", "forms", "redirects", "maintenance")
	_, out := admin.do("GET", "/api/v1/roles", nil)
	caps := map[string][]string{}
	for _, r := range out["roles"].([]any) {
		m := r.(map[string]any)
		for _, c := range m["caps"].([]any) {
			caps[m["slug"].(string)] = append(caps[m["slug"].(string)], c.(string))
		}
	}
	for _, want := range []struct {
		role, cap string
		ok        bool
	}{
		{"editor", "analytics.view", true}, {"author", "analytics.view", false},
		{"editor", "forms.manage", true}, {"editor", "forms.view_submissions", true}, {"author", "forms.manage", false},
		{"editor", "seo.manage", false}, {"editor", "seo.edit_meta", false},
		{"editor", "redirects.manage", false}, {"editor", "maintenance.manage", false},
	} {
		if slices.Contains(caps[want.role], want.cap) != want.ok {
			t.Fatalf("%s %s: want %v, caps %v", want.role, want.cap, want.ok, caps[want.role])
		}
	}
	pages := func(c *client) string {
		_, l := c.doAny("GET", "/api/v1/code/admin-pages", nil)
		var ids []string
		for _, x := range l.([]any) {
			ids = append(ids, fmt.Sprint(x.(map[string]any)["id"]))
		}
		return strings.Join(ids, ",")
	}
	if p := pages(editor); !strings.Contains(p, "analytics:stats") || !strings.Contains(p, "forms:forms") || strings.Contains(p, "seo:seo") ||
		strings.Contains(p, "redirects:") || strings.Contains(p, "maintenance:") {
		t.Fatalf("editor pages: %s", p)
	}
	if p := pages(author); p != "" {
		t.Fatalf("author pages: %s", p)
	}
	// SEO for an editor once the administrator ticks seo.edit_meta (not seo.manage: no defaults form)
	if code, o := admin.do("PUT", "/api/v1/roles/editor", map[string]any{"caps": append(caps["editor"], "seo.edit_meta")}); code != 200 {
		t.Fatalf("grant: %d %v", code, o)
	}
	html, call := offAAdminPage(t, e.pluginEnv, editor, "seo:seo", "")
	if strings.Contains(html, `id="seo-defaults"`) || !strings.Contains(html, `data-seo="list"`) {
		t.Fatal("editor without seo.manage sees the defaults form")
	}
	if code, o := call("seo:saveDefaults", map[string]any{"titleTemplate": "%title%"}); code == 200 {
		t.Fatalf("editor saved the SEO defaults: %d %v", code, o)
	}
}

// A package installed by a CMS older than 2.0.6 has a stored manifest without "capabilities": the next start
// reads them from the package's manifest.json and grants the defaults of the ones never seen.
func TestPluginCapabilitiesStartupSync(t *testing.T) {
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	mustUpload(t, admin, capKitPackage(capKitOpts{caps: []map[string]any{capDecl("view", "administrator", "editor")},
		main: `site.route('GET /capkit/view', { access: ['cap:cap-kit.view'] }, () => 'seen')`}))
	// what an older CMS left: no capabilities stored, nothing seen, nothing granted
	if _, err := e.st.DB.Exec(`UPDATE plugin_packages SET manifest = JSON_REMOVE(manifest, '$.capabilities') WHERE id = 'cap-kit'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.DB.Exec(`DELETE FROM options WHERE name = ?`, plugin.OptionCapsSeen); err != nil {
		t.Fatal(err)
	}
	if code, out := admin.do("PUT", "/api/v1/roles/editor", map[string]any{"caps": []string{"read", "edit_pages"}}); code != 200 {
		t.Fatalf("role: %d %v", code, out)
	}
	pm, err := plugin.NewManager(e.st, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in, err := pm.GetPackage("cap-kit")
	if err != nil || len(in.Capabilities) != 1 || in.Capabilities[0].Key != "cap-kit.view" {
		t.Fatalf("restored capabilities: %v %+v", err, in.Capabilities)
	}
	r, err := e.st.GetRole("editor")
	if err != nil || !slices.Contains(r.Caps, "cap-kit.view") || !slices.Contains(r.Caps, "edit_pages") {
		t.Fatalf("editor after the start: %v %v", err, r)
	}
}
