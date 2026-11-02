package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"
)

// CMS 2.0.5: JS plugins register post types (site.postType, scope
// "post-types"): type ids "<id>_<name>", slug checks, admin menu / editor
// catalog, fields, REST, public routing (public: false = the plugin's own
// routes), db.posts by the owner and reads by others, bindings,
// export / import, deactivate / uninstall keep the posts.

const ptkMain = `
site.postType('pt_kit_doc', {
  label: { th: 'เอกสาร', en: 'Docs' }, icon: '📘', slug: 'ptkit', public: false,
  supports: ['title', 'content', 'excerpt'],
  fields: { order: { type: 'number', min: 0 }, product: { type: 'select', options: ['studio', 'store'] } },
})
site.postType('pt_kit_item', { label: 'Items', slug: 'pt-items' })
site.fields('pt_kit_doc', { note: 'text' })
site.route('GET /ptkit/doc/{slug}', { access: ['*'] }, ({ request }) => {
  const p = db.posts.get(request.params.slug, { type: 'pt_kit_doc' })
  return p ? { page: { title: p.title, html: '<article id="doc">' + p.title + '|' + p.meta.product + '|' + p.url + '</article>' } } : { status: 404, body: 'nope' }
})
site.route('GET /ptkit/list', { access: ['*'] }, () => db.posts.list({ type: 'pt_kit_doc', where: { 'meta.order': { gte: 2 } }, order: 'title' }))
site.route('POST /ptkit/make', { access: ['*'] }, ({ request }) => db.asSystem(() => db.posts.create({ type: 'pt_kit_doc', status: 'publish', ...request.json() })))
`

func ptkPackage(id, main string, scopes []any, routes []any) map[string]string {
	m := map[string]any{"id": id, "name": "PT Kit", "version": "1.0.0", "runtime": "js", "permissions": []any{"code"},
		"code": map[string]any{"entry": "code/main.ts", "scopes": scopes, "routes": routes}}
	b, _ := json.Marshal(m)
	return map[string]string{"manifest.json": string(b), "code/main.ts": main}
}

var ptkScopes = []any{"routes", "content", "content-write", "content-fields", "post-types"}

func TestPluginPostTypes(t *testing.T) {
	e := offBNew(t)
	admin, user := setupSite(t, e.c)
	editor := user("Ed", "editor")

	failActivate := func(files map[string]string, want string) {
		t.Helper()
		mustUpload(t, admin, files)
		id := "pt-kit"
		if strings.Contains(files["manifest.json"], `"pt-oth"`) {
			id = "pt-oth"
		}
		code, out := admin.do("POST", "/api/v1/package-plugins/"+id+"/activate", map[string]any{})
		if code == 200 || !strings.Contains(fmt.Sprint(out), want) {
			t.Fatalf("activation should fail with %q: %d %v", want, code, out)
		}
		admin.do("DELETE", "/api/v1/package-plugins/"+id, nil)
	}
	// scope, type id rule, slug checks
	failActivate(ptkPackage("pt-kit", ptkMain, []any{"routes", "content", "content-write", "content-fields"}, []any{"/ptkit"}), "post-types")
	failActivate(ptkPackage("pt-kit", `site.postType('doc', { slug: 'x-doc' })`, ptkScopes, []any{"/ptkit"}), `"pt_kit_"`)
	failActivate(ptkPackage("pt-kit", `site.postType('pt_kit', { slug: 'x-doc' })`, ptkScopes, []any{"/ptkit"}), `"pt_kit_"`)
	failActivate(ptkPackage("pt-kit", `site.postType('pt_kit_abcdefghijklmn', { slug: 'x-doc' })`, ptkScopes, []any{"/ptkit"}), "20")
	failActivate(ptkPackage("pt-kit", `site.postType('pt_kit_doc', { slug: 'blog' })`, ptkScopes, []any{"/ptkit"}), "/blog")
	// another installed plugin's URL prefix
	mustUpload(t, admin, ptkPackage("pt-oth", `site.route('GET /othp', { access: ['*'] }, () => 'o')`, []any{"routes"}, []any{"/othp"}))
	failActivate(ptkPackage("pt-kit", `site.postType('pt_kit_doc', { slug: 'othp' })`, ptkScopes, []any{"/ptkit"}), "pt-oth")
	admin.do("DELETE", "/api/v1/package-plugins/pt-oth", nil)
	// a Site Code type id / slug
	publishCode(t, admin, map[string]string{"main.ts": `site.postType('event', { slug: 'events' })`})
	failActivate(ptkPackage("pt-kit", `site.postType('pt_kit_doc', { slug: 'events' })`, ptkScopes, []any{"/ptkit"}), "events")
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('GET /sc/docs', { access: ['*'] }, () => db.posts.list({ type: 'pt_kit_doc' }))
site.route('GET /sc/write', { access: ['*'] }, () => { try { db.asSystem(() => db.posts.create({ type: 'pt_kit_doc', title: 'x' })); return 'wrote' } catch (e) { return String(e.message) } })`})

	mustUpload(t, admin, ptkPackage("pt-kit", ptkMain, ptkScopes, []any{"/ptkit"}))
	activate(t, admin, "pt-kit")

	// Site Code may not take a plugin's type slug now
	putFile(t, admin, "main.ts", `site.postType('thing', { slug: 'pt-items' })`)
	if code, out := publishWith(admin, liveID(t, admin), true); code == 200 || !strings.Contains(fmt.Sprint(out), "pt-kit") {
		t.Fatalf("site code publish with a plugin slug: %d %v", code, out)
	}
	putFile(t, admin, "main.ts", `
site.route('GET /sc/docs', { access: ['*'] }, () => db.posts.list({ type: 'pt_kit_doc' }))
site.route('GET /sc/write', { access: ['*'] }, () => { try { db.asSystem(() => db.posts.create({ type: 'pt_kit_doc', title: 'x' })); return 'wrote' } catch (e) { return String(e.message) } })`)

	// catalog: GET /post-types and the bindings
	catalog := func(c *client, path string) (map[string]map[string]any, map[string]any) {
		t.Helper()
		code, out := c.do("GET", path, nil)
		if code != 200 {
			t.Fatalf("%s: %d %v", path, code, out)
		}
		types := map[string]map[string]any{}
		for _, x := range out["postTypes"].([]any) {
			m := x.(map[string]any)
			types[m["type"].(string)] = m
		}
		return types, out["postFields"].(map[string]any)
	}
	types, fields := catalog(editor, "/api/v1/post-types")
	doc := types["pt_kit_doc"]
	if doc == nil || doc["plugin"] != "pt-kit" || doc["pluginName"] != "PT Kit" || doc["public"] != false || doc["slug"] != "ptkit" ||
		doc["canEdit"] != true || doc["fields"].(map[string]any)["product"] == nil || types["pt_kit_item"]["slug"] != "pt-items" {
		t.Fatalf("post types: %v", types)
	}
	if _, ok := fields["pt_kit_doc"].(map[string]any)["pt-kit.note"]; !ok {
		t.Fatalf("plugin fields of its type: %v", fields)
	}
	if bt, _ := catalog(admin, "/api/v1/code/bindings"); bt["pt_kit_doc"] == nil || bt["pt_kit_doc"]["plugin"] != "pt-kit" {
		t.Fatalf("bindings: %v", bt)
	}

	// REST: create / validate / list
	if code, out := editor.do("POST", "/api/v1/posts", map[string]any{"type": "pt_kit_doc", "title": "Bad", "meta": map[string]any{"product": "nope"}}); code != 400 {
		t.Fatalf("bad select accepted: %d %v", code, out)
	}
	code, p := editor.do("POST", "/api/v1/posts", map[string]any{"type": "pt_kit_doc", "title": "Install", "slug": "install", "status": "publish",
		"content": "<p>hi</p>", "meta": map[string]any{"order": 3, "product": "studio", "pt-kit.note": "n1"}})
	if code != 201 {
		t.Fatalf("create: %d %v", code, p)
	}
	if m := p["meta"].(map[string]any); m["order"] != float64(3) || m["pt-kit.note"] != "n1" {
		t.Fatalf("meta: %v", m)
	}
	if code, out := editor.do("POST", "/api/v1/posts", map[string]any{"type": "pt_kit_item", "title": "Thing one", "slug": "one", "status": "publish"}); code != 201 {
		t.Fatalf("create item: %d %v", code, out)
	}
	if code, out := admin.do("GET", "/api/v1/posts?type=pt_kit_doc", nil); code != 200 || out["total"] != float64(1) {
		t.Fatalf("list: %d %v", code, out)
	}

	// db.posts by the owner: create (meta validated), list with a meta filter, get + url
	if code, out := getJSON(t, e.c, "POST", "/ptkit/make", `{"title":"Alpha","slug":"alpha","meta":{"order":1,"product":"store"}}`, nil); code != 200 || out["type"] != "pt_kit_doc" {
		t.Fatalf("make: %d %v", code, out)
	}
	if code, out := getJSON(t, e.c, "POST", "/ptkit/make", `{"title":"Bad","meta":{"order":-1}}`, nil); code == 200 {
		t.Fatalf("invalid meta from db.posts: %d %v", code, out)
	}
	if code, out := getJSON(t, e.c, "GET", "/ptkit/list", "", nil); code != 200 || out["total"] != float64(1) {
		t.Fatalf("meta filter: %d %v", code, out)
	}

	// public: false → the plugin's own pages, the CMS does not route /ptkit/<slug>
	rec := e.offBSend(offBReq{path: "/ptkit/doc/install"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `<article id="doc">Install|studio|/ptkit/install</article>`) {
		t.Fatalf("plugin page: %d %s", rec.Code, rec.Body.String())
	}
	if rec = e.offBSend(offBReq{path: "/ptkit/install"}); rec.Code != 404 {
		t.Fatalf("core routed a public:false type: %d", rec.Code)
	}
	// a public plugin type: /<slug>/<post-slug> and the archive
	if rec = e.offBSend(offBReq{path: "/pt-items/one"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Thing one") {
		t.Fatalf("public plugin type: %d %s", rec.Code, rec.Body.String())
	}
	if rec = e.offBSend(offBReq{path: "/pt-items"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `href="/pt-items/one"`) {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body.String())
	}

	// other units read (not write) a plugin's type
	publishCode(t, admin, map[string]string{})
	if code, out := getJSON(t, e.c, "GET", "/sc/docs", "", nil); code != 200 || out["total"] != float64(2) {
		t.Fatalf("site code read: %d %v", code, out)
	}
	if rec := e.offBSend(offBReq{path: "/sc/write"}); strings.Contains(rec.Body.String(), "wrote") {
		t.Fatalf("site code wrote a plugin type: %s", rec.Body.String())
	}

	// export / import keep the posts and their meta
	rec = admin.send("GET", "/api/v1/tools/export", nil, nil)
	if rec.Code != 200 {
		t.Fatalf("export: %d", rec.Code)
	}
	zipData := rec.Body.Bytes()
	data := unzip(t, zipData)["data.json"]
	if !strings.Contains(data, `"pt_kit_doc"`) || !strings.Contains(data, `"pt-kit.note"`) {
		t.Fatalf("export lacks the plugin type posts")
	}
	countMeta := func() (n int) {
		_ = e.st.DB.QueryRow(`SELECT COUNT(*) FROM posts p JOIN post_meta m ON m.post_id = p.id WHERE p.type = 'pt_kit_doc' AND m.meta_key = 'pt-kit.note'`).Scan(&n)
		return n
	}
	before := countMeta()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "site.zip")
	_, _ = fw.Write(zipData)
	_ = mw.Close()
	if rec := admin.send("POST", "/api/v1/tools/import", &buf, map[string]string{"Content-Type": mw.FormDataContentType()}); rec.Code != 200 {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	if after := countMeta(); after != before+1 {
		t.Fatalf("imported plugin type posts with meta: %d → %d", before, after)
	}
	var n int

	// deactivate: the type is gone from the admin (posts kept), back on activation
	if code, out := admin.do("POST", "/api/v1/package-plugins/pt-kit/deactivate", nil); code != 200 {
		t.Fatalf("deactivate: %d %v", code, out)
	}
	if types, _ = catalog(admin, "/api/v1/post-types"); types["pt_kit_doc"] != nil {
		t.Fatalf("type still listed: %v", types)
	}
	if code, _ := admin.do("GET", "/api/v1/posts?type=pt_kit_doc", nil); code != 400 {
		t.Fatalf("unregistered type listed: %d", code)
	}
	activate(t, admin, "pt-kit")
	if code, out := admin.do("GET", "/api/v1/posts?type=pt_kit_doc", nil); code != 200 || out["total"].(float64) < 2 {
		t.Fatalf("after re-activation: %d %v", code, out)
	}
	// uninstall keeps the posts
	if code, _ := admin.do("DELETE", "/api/v1/package-plugins/pt-kit", nil); code != 204 {
		t.Fatalf("uninstall: %d", code)
	}
	_ = e.st.DB.QueryRow(`SELECT COUNT(*) FROM posts WHERE type = 'pt_kit_doc'`).Scan(&n)
	if n < 2 {
		t.Fatalf("uninstall removed posts: %d", n)
	}
	if code, _ := admin.do("GET", "/api/v1/posts?type=pt_kit_doc", nil); code != 400 {
		t.Fatalf("uninstalled type listed: %d", code)
	}
}
