package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/backup"
	"github.com/anasaki/studio/backend/internal/sitecode"
	"github.com/anasaki/studio/backend/internal/store"
)

// Site Code phase 4a: hooks, visitor, post types, content writes, services,
// jobs/cron, webhooks and admin pages, end to end.

func publishCode(t *testing.T, admin *client, files map[string]string) {
	t.Helper()
	for p, c := range files {
		base := ""
		if code, f := admin.do("GET", "/api/v1/code/files/"+p, nil); code == 200 {
			base = f["updatedAt"].(string)
		}
		admin.put(p, c, base)
	}
	_, st := admin.do("GET", "/api/v1/code/status", nil)
	if code, b := publishWith(admin, st["liveVersion"], true); code != 200 {
		t.Fatalf("publish: %d %v", code, b)
	}
}

func withCookie(c *client, method, path, cookie string) (int, string) {
	c.t.Helper()
	h := map[string]string{"Accept": "application/json"}
	if cookie != "" {
		h["Cookie"] = cookie
	}
	rec := c.send(method, path, nil, h)
	return rec.Code, rec.Body.String()
}

func TestCodeHooksAndVisitor(t *testing.T) {
	c, code, _ := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": `
site.visitor(({ request }) => request.cookies.member ? { id: request.cookies.member, name: 'M-' + request.cookies.member,
  groups: request.cookies.member === 'g' ? ['gold'] : ['member'] } : null)
site.visitorGroups(['gold', 'member'])
site.filter('post.content', (h) => h + '<p class="sig">SIG</p>')
site.filter('page.head', (head, { path }) => ({ ...head, title: 'HEAD:' + path, noindex: true }))
site.filter('post.beforeSave', (p, { isNew, user }) => p.title === 'forbidden' ? { error: 'ห้ามใช้ชื่อนี้' } :
  (isNew && !p.excerpt ? { ...p, excerpt: 'auto by ' + user.role } : p))
site.on('site.ping', (d) => { cache.set('pinged', d) })
site.route('GET /emit', { access: ['*'] }, () => { site.emit('site.ping', { n: 1 }); return 'ok' })
site.route('GET /pinged', { access: ['*'] }, () => cache.get('pinged') ?? null)
site.route('GET /club', { access: ['group:gold'] }, ({ visitor }) => 'club ' + (visitor ? visitor.name : 'staff'))
`})
	// group access on a route
	if code, _ := withCookie(c, "GET", "/club", ""); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
	if code, _ := withCookie(c, "GET", "/club", "pl_site_member=m"); code != 401 {
		t.Fatalf("member (not gold): %d", code)
	}
	if code, b := withCookie(c, "GET", "/club", "pl_site_member=g"); code != 200 || b != "club M-g" {
		t.Fatalf("gold: %d %s", code, b)
	}
	if code, b := admin.do("PUT", "/api/v1/code/access", map[string]any{"kind": "route", "name": "GET /club", "roles": []string{"group:member"}}); code != 200 {
		t.Fatalf("group override: %d %v", code, b)
	}
	if code, _ := withCookie(c, "GET", "/club", "pl_site_member=m"); code != 200 {
		t.Fatalf("member after override: %d", code)
	}
	if code, _ := withCookie(admin, "GET", "/club", ""); code != 200 {
		t.Fatalf("administrator always passes: %d", code)
	}
	// visitor + filters on a designed page
	dsg := map[string]any{"id": "root", "type": "frame", "children": []any{
		map[string]any{"id": "who", "type": "text", "props": map[string]any{"text": "WHO={{visitor.name ?? 'guest'}}"}},
		map[string]any{"id": "gold", "type": "text", "props": map[string]any{"text": "GOLD-ONLY", "showWhen": "visitor.is.gold"}},
		map[string]any{"id": "body", "type": "postContent"},
	}}
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "V", "slug": "vpage", "status": "publish",
		"content": "<p>body</p>", "design": dsg}); code != 201 {
		t.Fatalf("page: %d %v", code, b)
	}
	_, h := withCookie(c, "GET", "/vpage", "pl_site_member=g")
	for _, want := range []string{"WHO=M-g", "GOLD-ONLY", `<p>body</p><p class="sig">SIG</p>`, "<title>HEAD:/vpage</title>", `content="noindex,nofollow"`} {
		if !strings.Contains(h, want) {
			t.Errorf("gold page: missing %q", want)
		}
	}
	_, h = withCookie(c, "GET", "/vpage", "")
	if !strings.Contains(h, "WHO=guest") || strings.Contains(h, "GOLD-ONLY") {
		t.Errorf("anonymous page:\n%s", h)
	}
	_, st := admin.do("GET", "/api/v1/code/status", nil)
	reg := st["registered"].(map[string]any)
	if strs(reg["visitorGroups"]) != "gold,member" || reg["visitor"] != true || len(reg["filters"].([]any)) != 3 {
		t.Fatalf("status registered: %v", reg)
	}
	// post.beforeSave through the admin API
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"title": "forbidden"}); code != 400 || b["error"] != "ห้ามใช้ชื่อนี้" {
		t.Fatalf("refused save: %d %v", code, b)
	}
	editor := user("Ed", "editor")
	code0, p := editor.do("POST", "/api/v1/posts", map[string]any{"title": "Fine"})
	if code0 != 201 || p["excerpt"] != "auto by editor" {
		t.Fatalf("modified save: %d %v", code0, p)
	}
	if code, b := editor.do("PUT", "/api/v1/posts/"+id(p), map[string]any{"title": "forbidden"}); code != 400 || b["error"] != "ห้ามใช้ชื่อนี้" {
		t.Fatalf("refused update: %d %v", code, b)
	}
	// custom events
	withCookie(c, "GET", "/emit", "")
	code.WaitEvents()
	if _, b := withCookie(c, "GET", "/pinged", ""); strings.TrimSpace(b) != `{"n":1}` {
		t.Fatalf("site.emit → site.on: %s", b)
	}
	// preview "view as visitor in group…"
	publishCode(t, admin, map[string]string{"extra.ts": `site.collection('vip', { access: ['group:gold'], load: ({ visitor }) => [{ n: visitor.name }] })`,
		"main.ts": "import './extra'\nsite.visitor(() => null)\n"})
	if code, b := admin.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "code:vip", "viewAs": map[string]any{"groups": []string{"gold"}}}); code != 200 || b["total"] != 1.0 {
		t.Fatalf("preview as gold visitor: %d %v", code, b)
	}
	if _, b := admin.do("POST", "/api/v1/code/preview", map[string]any{"kind": "collection", "source": "code:vip", "viewAs": "anonymous"}); b["denied"] != true {
		t.Fatalf("preview anonymous: %v", b)
	}
}

func TestCodePostTypes(t *testing.T) {
	c, _, st := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": `
site.postType('event', { label: { th: 'อีเวนต์', en: 'Events' }, icon: '📅', slug: 'events',
  fields: { date: { type: 'date', required: true, label: { th: 'วันที่', en: 'Date' } }, seats: { type: 'number', min: 0 } },
  access: { edit: ['editor', 'author'] } })
site.fields('post', { readingTime: 'number' })
site.route('GET /upcoming', { access: ['*'] }, () => db.posts.list({ type: 'event', where: { 'meta.date': { gte: '2026-06-01' } }, orderBy: '-meta.date' })
  .items.map((p) => p.slug + ':' + p.meta.date + ':' + p.url))
site.route('GET /seats', { access: ['*'] }, () => db.posts.list({ type: 'event', where: { 'meta.seats': { in: [5, 7] } } }).total)
`})
	author, contrib := user("Au", "author"), user("Co", "contributor")
	_, pt := author.do("GET", "/api/v1/post-types", nil)
	types := pt["postTypes"].([]any)
	if len(types) != 1 || types[0].(map[string]any)["type"] != "event" || types[0].(map[string]any)["canEdit"] != true ||
		types[0].(map[string]any)["fields"].(map[string]any)["date"].(map[string]any)["required"] != true {
		t.Fatalf("post types: %v", pt)
	}
	if pt["postFields"].(map[string]any)["post"].(map[string]any)["readingTime"] == nil {
		t.Fatalf("postFields: %v", pt["postFields"])
	}
	if _, cp := contrib.do("GET", "/api/v1/post-types", nil); cp["postTypes"].([]any)[0].(map[string]any)["canEdit"] != false {
		t.Fatal("contributor canEdit")
	}
	// bindings: authors (no edit_pages) get the post types only
	if code, b := author.do("GET", "/api/v1/code/bindings", nil); code != 200 || len(b["postTypes"].([]any)) != 1 || b["postFields"] == nil {
		t.Fatalf("bindings for an author: %d %v", code, b)
	}
	// CRUD with meta + validation + permissions
	if code, b := author.do("POST", "/api/v1/posts", map[string]any{"type": "event", "title": "No date", "meta": map[string]any{"seats": 5}}); code != 400 ||
		b["fields"].(map[string]any)["date"] == nil {
		t.Fatalf("required field: %d %v", code, b)
	}
	if code, b := author.do("POST", "/api/v1/posts", map[string]any{"type": "event", "title": "Neg", "meta": map[string]any{"date": "2026-01-01", "seats": -1}}); code != 400 ||
		b["fields"].(map[string]any)["seats"] == nil {
		t.Fatalf("min: %d %v", code, b)
	}
	if code, _ := contrib.do("POST", "/api/v1/posts", map[string]any{"type": "event", "title": "x", "meta": map[string]any{"date": "2026-01-01"}}); code != 403 {
		t.Fatalf("contributor creates event: %d", code)
	}
	if code, _ := contrib.do("GET", "/api/v1/posts?type=event", nil); code != 403 {
		t.Fatalf("contributor lists events: %d", code)
	}
	ids := []string{}
	for i, ev := range []map[string]any{{"date": "2026-01-10", "seats": 5}, {"date": "2026-07-01", "seats": 7}, {"date": "2026-09-01", "seats": 9}} {
		code, p := author.do("POST", "/api/v1/posts", map[string]any{"type": "event", "title": fmt.Sprintf("Event %d", i+1), "slug": fmt.Sprintf("e%d", i+1),
			"status": "publish", "meta": ev})
		if code != 201 || p["meta"].(map[string]any)["date"] != ev["date"] || p["status"] != "publish" {
			t.Fatalf("create event: %d %v", code, p)
		}
		ids = append(ids, id(p))
	}
	if code, b := author.do("GET", "/api/v1/posts?type=event", nil); code != 200 || b["total"] != 3.0 {
		t.Fatalf("list events: %d %v", code, b)
	}
	if code, b := author.do("PUT", "/api/v1/posts/"+ids[0], map[string]any{"meta": map[string]any{"seats": ""}}); code != 200 || b["meta"].(map[string]any)["seats"] != nil {
		t.Fatalf("remove a value: %d %v", code, b)
	}
	author.do("PUT", "/api/v1/posts/"+ids[0], map[string]any{"meta": map[string]any{"seats": 5}})
	if code, b := admin.do("PUT", "/api/v1/posts/"+ids[0], map[string]any{"title": "Event one"}); code != 200 || b["meta"].(map[string]any)["date"] != "2026-01-10" {
		t.Fatalf("update without meta keeps it: %d %v", code, b)
	}
	// built-in type with site.fields
	if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"title": "Article", "meta": map[string]any{"readingTime": "4", "junk": 1}}); code != 201 ||
		b["meta"].(map[string]any)["readingTime"] != 4.0 || b["meta"].(map[string]any)["junk"] != nil {
		t.Fatalf("site.fields on post: %d %v", code, b)
	}
	// meta filters in db.posts
	if code, b := c.public("GET", "/upcoming"); code != 200 || strings.TrimSpace(b) != `["e3:2026-09-01:/events/e3","e2:2026-07-01:/events/e2"]` {
		t.Fatalf("meta where/orderBy: %d %s", code, b)
	}
	if _, b := c.public("GET", "/seats"); strings.TrimSpace(b) != "2" {
		t.Fatalf("meta in: %s", b)
	}
	// public URL, template per type, archive
	if code, h := withCookie(c, "GET", "/events/e2", ""); code != 200 || !strings.Contains(h, "Event 2") {
		t.Fatalf("single event: %d", code)
	}
	tplDesign := map[string]any{"id": "root", "type": "frame", "children": []any{
		map[string]any{"id": "t", "type": "text", "props": map[string]any{"text": "TPL {{post.title}} @ {{post.meta.date}} seats {{post.meta.seats}}"}}}}
	code, tpl := admin.do("POST", "/api/v1/posts", map[string]any{"type": "template", "title": "Event tpl", "design": tplDesign})
	if code != 201 {
		t.Fatalf("template: %d %v", code, tpl)
	}
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"post_template_event": id(tpl)}); code != 200 || b["post_template_event"] != id(tpl) {
		t.Fatalf("post_template_event: %d %v", code, b)
	}
	if code, b := admin.do("PUT", "/api/v1/settings", map[string]any{"post_template_event": "abc"}); code != 400 {
		t.Fatalf("bad template id: %d %v", code, b)
	}
	if _, h := withCookie(c, "GET", "/events/e2", ""); !strings.Contains(h, "TPL Event 2 @ 2026-07-01 seats 7") {
		t.Fatalf("type template:\n%s", h)
	}
	if code, h := withCookie(c, "GET", "/events", ""); code != 200 || !strings.Contains(h, `href="/events/e3"`) || !strings.Contains(h, "อีเวนต์") {
		t.Fatalf("archive: %d\n%s", code, h)
	}
	if code, _ := withCookie(c, "GET", "/events/nope", ""); code != 404 {
		t.Fatalf("missing event: %d", code)
	}
	// backup export/import keeps meta
	d, err := backup.Collect(st)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range d.Posts {
		if p.Type == "event" && p.Slug == "e2" && p.Meta["seats"] == 7.0 {
			found = true
		}
	}
	if !found {
		t.Fatal("backup data lacks meta")
	}
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	f, _ := zw.Create("data.json")
	_ = json.NewEncoder(f).Encode(d)
	_ = zw.Close()
	var ufile multipart.File = nopFile{bytes.NewReader(zbuf.Bytes())}
	if _, err := backup.Import(st, t.TempDir(), ufile, &multipart.FileHeader{Size: int64(zbuf.Len())}, 1, true); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM post_meta pm JOIN posts p ON p.id = pm.post_id WHERE p.type = 'event' AND pm.meta_key = 'seats'`).Scan(&n)
	if n != 6 {
		t.Fatalf("meta after import: %d rows", n)
	}
	// deleting a post deletes its meta
	if code, _ := admin.do("DELETE", "/api/v1/posts/"+ids[1], nil); code != 204 {
		t.Fatal("delete")
	}
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM post_meta WHERE post_id = ?`, ids[1]).Scan(&n)
	if n != 0 {
		t.Fatalf("meta left after delete: %d", n)
	}
	// the type disappears from code: posts stay, admin hides them
	publishCode(t, admin, map[string]string{"main.ts": "site.route('GET /x', { access: ['*'] }, () => 1)\n"})
	if _, b := author.do("GET", "/api/v1/post-types", nil); len(b["postTypes"].([]any)) != 0 {
		t.Fatalf("types after unpublish: %v", b)
	}
	if code, _ := admin.do("GET", "/api/v1/posts?type=event", nil); code != 400 {
		t.Fatalf("list of a removed type: %d", code)
	}
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM posts WHERE type = 'event'`).Scan(&n)
	if n != 5 {
		t.Fatalf("event posts kept: %d", n)
	}
}

type nopFile struct{ *bytes.Reader }

func (nopFile) Close() error { return nil }

func TestCodeContentWrites(t *testing.T) {
	c, code, st := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": `
site.fields('post', { readingTime: 'number' })
site.route('POST /mk', { access: ['*'] }, ({ request }) => db.posts.create({ type: 'post', title: request.json().title, status: 'publish', meta: { readingTime: 3 } }))
site.route('POST /mksys', { access: ['*'] }, () => db.asSystem(() => db.posts.create({ title: 'sys', status: 'publish' })))
site.route('POST /upd/{id}', { access: ['*'] }, ({ request }) => db.posts.update(Number(request.params.id), { title: 'changed' }))
site.route('POST /del/{id}', { access: ['*'] }, ({ request }) => db.posts.delete(Number(request.params.id)))
site.route('POST /badmeta', { access: ['*'] }, () => db.asSystem(() => db.posts.create({ title: 'x', meta: { readingTime: 'abc' } })))
site.on('post.saved', (p) => { cache.set('saved', p.title) })
site.route('GET /saved', { access: ['*'] }, () => cache.get('saved') ?? null)
`})
	post := func(cl *client, path, body string) (int, map[string]any) {
		rec := cl.send("POST", path, strings.NewReader(body), map[string]string{"Accept": "application/json", "Content-Type": "application/json"})
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, _ := post(c, "/mk", `{"title":"anon"}`); code != 500 {
		t.Fatalf("anonymous create: %d", code)
	}
	code1, p := post(admin, "/mk", `{"title":"by admin"}`)
	if code1 != 200 || p["status"] != "publish" || p["meta"].(map[string]any)["readingTime"] != 3.0 {
		t.Fatalf("admin create: %d %v", code1, p)
	}
	con := user("Co", "contributor")
	if code, cp := post(con, "/mk", `{"title":"by contributor"}`); code != 200 || cp["status"] != "draft" {
		t.Fatalf("contributor create (no publish cap → draft): %d %v", code, cp)
	}
	code2, sp := post(c, "/mksys", ``)
	if code2 != 200 || sp["title"] != "sys" {
		t.Fatalf("asSystem: %d %v", code2, sp)
	}
	var n int
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM script_audit WHERE action = 'as_system' AND detail LIKE 'main.ts:%'`).Scan(&n)
	if n != 1 {
		t.Fatalf("as_system audit rows: %d", n)
	}
	code.WaitEvents()
	if _, b := c.public("GET", "/saved"); strings.TrimSpace(b) != "sys" {
		t.Fatalf("post.saved event from db.posts: %s", b)
	}
	pid := id(p)
	if code, _ := post(c, "/upd/"+pid, ``); code != 500 {
		t.Fatalf("anonymous update: %d", code)
	}
	if code, b := post(admin, "/upd/"+pid, ``); code != 200 || b["title"] != "changed" || b["meta"].(map[string]any)["readingTime"] != 3.0 {
		t.Fatalf("admin update: %d %v", code, b)
	}
	if code, _ := post(con, "/del/"+pid, ``); code != 500 {
		t.Fatalf("contributor deletes a published post: %d", code)
	}
	if code, _ := post(c, "/badmeta", ``); code != 500 {
		t.Fatalf("invalid meta: %d", code)
	}
	if code, _ := post(admin, "/del/"+pid, ``); code != 200 {
		t.Fatalf("admin delete: %d", code)
	}
	if code, _ := admin.do("GET", "/api/v1/posts/"+pid, nil); code != 404 {
		t.Fatalf("deleted post: %d", code)
	}
}

func TestCodeSettingsSecretsMail(t *testing.T) {
	var mu sync.Mutex
	var sent []sitecode.MailMessage
	c, _, st := newCodeAPI(t, func(cfg *sitecode.Config) {
		cfg.Mailer = func(m sitecode.MailMessage) error { mu.Lock(); sent = append(sent, m); mu.Unlock(); return nil }
	})
	admin, user := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('GET /secret', { access: ['*'] }, () => ({ k: site.settings('API_KEY'), m: site.settings('MODE'), none: site.settings('NOPE') }))
site.route('GET /mail', { access: ['*'] }, () => mail.send({ to: 'x@y.io', subject: 'S', html: '<b>h</b>' }))
`})
	in := map[string]any{"values": []any{map[string]any{"key": "API_KEY", "secret": true, "value": "sk_live_1"}, map[string]any{"key": "MODE", "value": "test"}},
		"fetchHosts": []string{"API.example.com", "*.x.io"}, "limits": map[string]any{"mailPerHour": 1}}
	code, b := admin.do("PUT", "/api/v1/code/settings", in)
	if code != 200 {
		t.Fatalf("PUT settings: %d %v", code, b)
	}
	vals := map[string]map[string]any{}
	for _, x := range b["values"].([]any) {
		vals[x.(map[string]any)["key"].(string)] = x.(map[string]any)
	}
	if vals["API_KEY"]["set"] != true || vals["API_KEY"]["value"] != nil || vals["MODE"]["value"] != "test" || strs(b["fetchHosts"]) != "*.x.io,api.example.com" ||
		b["limits"].(map[string]any)["mailPerHour"] != 1.0 || b["limits"].(map[string]any)["routePerMinute"] != 120.0 {
		t.Fatalf("settings view: %v", b)
	}
	if strings.Contains(fmt.Sprint(b), "sk_live") {
		t.Fatal("secret value returned")
	}
	var raw string
	_ = st.DB.QueryRow(`SELECT v FROM script_settings WHERE k = 'API_KEY'`).Scan(&raw)
	if !strings.HasPrefix(raw, "enc:v1:") || strings.Contains(raw, "sk_live") {
		t.Fatalf("not encrypted at rest: %q", raw)
	}
	if _, b := c.public("GET", "/secret"); strings.TrimSpace(b) != `{"k":"sk_live_1","m":"test","none":null}` {
		t.Fatalf("site.settings: %s", b)
	}
	// empty secret = unchanged; remove
	if code, _ := admin.do("PUT", "/api/v1/code/settings", map[string]any{"values": []any{map[string]any{"key": "API_KEY", "secret": true, "value": ""}}, "remove": []string{"MODE"}}); code != 200 {
		t.Fatal("PUT unchanged")
	}
	if _, b := c.public("GET", "/secret"); strings.TrimSpace(b) != `{"k":"sk_live_1","m":null,"none":null}` {
		t.Fatalf("after unchanged + remove: %s", b)
	}
	for name, bad := range map[string]map[string]any{
		"host":       {"fetchHosts": []string{"http://x.io/"}},
		"ip":         {"fetchHosts": []string{"10.0.0.1"}},
		"new empty":  {"values": []any{map[string]any{"key": "NEW", "secret": true, "value": ""}}},
		"bad key":    {"values": []any{map[string]any{"key": "1 bad", "value": "x"}}},
		"reveal":     {"values": []any{map[string]any{"key": "API_KEY", "secret": false, "value": ""}}},
		"route rate": {"limits": map[string]any{"routePerMinute": 0}},
	} {
		if code, _ := admin.do("PUT", "/api/v1/code/settings", bad); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := user("Ed", "editor").do("GET", "/api/v1/code/settings", nil); code != 403 {
		t.Fatalf("editor reads settings: %d", code)
	}
	var detail string
	_ = st.DB.QueryRow(`SELECT detail FROM script_audit WHERE action = 'settings' ORDER BY id LIMIT 1`).Scan(&detail)
	if !strings.Contains(detail, "API_KEY") || strings.Contains(detail, "sk_live") {
		t.Fatalf("audit: %s", detail)
	}
	// mail: quota from the settings (1 / hour)
	if code, b := c.public("GET", "/mail"); code != 200 || strings.TrimSpace(b) != `{"ok":true}` {
		t.Fatalf("mail.send: %d %s", code, b)
	}
	if code, _ := c.public("GET", "/mail"); code != 500 {
		t.Fatalf("quota: %d", code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || sent[0].To[0] != "x@y.io" || sent[0].HTML != "<b>h</b>" {
		t.Fatalf("sent: %+v", sent)
	}
}

const jobsCode = `
site.job('work', { retries: 2, timeout: 5 }, ({ data, attempt, progress }) => {
  progress('half')
  db.docs('runs').insert({ id: data && data.id, attempt })
  if (data && data.fail) throw new Error('fail ' + attempt)
  return { done: data && data.id }
})
site.job('slow', { timeout: 1, retries: 0 }, () => { while (true) {} })
site.job('par', { concurrency: 20, retries: 0 }, ({ data }) => { db.docs('par').insert({ id: data.id }); return data.id })
site.cron('0 8 * * *', 'work')
site.cron('* * * * *', () => { db.docs('cronfn').insert({ at: 1 }) })
site.route('POST /enq', { access: ['*'] }, ({ request }) => { const b = request.json(); return { id: jobs.enqueue(b.name || 'work', b.data, b.opts) } })
site.route('GET /count/{c}', { access: ['*'] }, ({ request }) => db.docs(request.params.c).find({}).total)
`

func jobRow(t *testing.T, st *store.Store, id float64) (status string, attempts int, progress, errMsg string) {
	t.Helper()
	var p, e *string
	if err := st.DB.QueryRow(`SELECT status, attempts, progress, error FROM script_jobs WHERE id = ?`, int64(id)).Scan(&status, &attempts, &p, &e); err != nil {
		t.Fatal(err)
	}
	if p != nil {
		progress = *p
	}
	if e != nil {
		errMsg = *e
	}
	return
}

func TestCodeJobsAndCron(t *testing.T) {
	c, code, st := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": jobsCode})
	code.JobBackoff = 50 * time.Millisecond
	ctx := context.Background()
	enq := func(body map[string]any) float64 {
		t.Helper()
		b, _ := json.Marshal(body)
		rec := c.send("POST", "/enq", bytes.NewReader(b), map[string]string{"Content-Type": "application/json", "Accept": "application/json"})
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 200 {
			t.Fatalf("enqueue: %d %s", rec.Code, rec.Body.String())
		}
		return out["id"].(float64)
	}
	// run + progress + result
	j1 := enq(map[string]any{"data": map[string]any{"id": 1}})
	if n := code.ProcessJobs(ctx, 10); n != 1 {
		t.Fatalf("processed %d", n)
	}
	if s, a, p, _ := jobRow(t, st, j1); s != "done" || a != 1 || p != "half" {
		t.Fatalf("job 1: %s %d %q", s, a, p)
	}
	_, list := admin.do("GET", "/api/v1/code/jobs?status=done", nil)
	if list["total"] != 1.0 || list["items"].([]any)[0].(map[string]any)["result"].(map[string]any)["done"] != 1.0 {
		t.Fatalf("jobs list: %v", list)
	}
	// delay + dedupe
	j2 := enq(map[string]any{"data": map[string]any{"id": 2}, "opts": map[string]any{"key": "k", "delay": "1h"}})
	if j2b := enq(map[string]any{"data": map[string]any{"id": 2}, "opts": map[string]any{"key": "k"}}); j2b != j2 {
		t.Fatalf("dedupe: %v vs %v", j2b, j2)
	}
	if n := code.ProcessJobs(ctx, 10); n != 0 {
		t.Fatal("delayed job ran early")
	}
	// cancel / retry
	if code, _ := admin.do("POST", fmt.Sprintf("/api/v1/code/jobs/%d/cancel", int64(j2)), nil); code != 200 {
		t.Fatal("cancel")
	}
	if code, _ := admin.do("POST", fmt.Sprintf("/api/v1/code/jobs/%d/cancel", int64(j2)), nil); code != 409 {
		t.Fatal("cancel twice")
	}
	if code, _ := admin.do("POST", fmt.Sprintf("/api/v1/code/jobs/%d/retry", int64(j2)), nil); code != 200 {
		t.Fatal("retry")
	}
	if code, _ := admin.do("POST", "/api/v1/code/jobs/999999/retry", nil); code != 404 {
		t.Fatal("retry missing job")
	}
	if code.ProcessJobs(ctx, 10) != 1 {
		t.Fatal("retried job did not run")
	}
	// failures: exponential backoff, then failed after retries+1 attempts
	j3 := enq(map[string]any{"data": map[string]any{"id": 3, "fail": true}})
	code.ProcessJobs(ctx, 10)
	if s, a, _, e := jobRow(t, st, j3); s != "pending" || a != 1 || !strings.Contains(e, "fail 1") {
		t.Fatalf("first failure: %s %d %q", s, a, e)
	}
	var delayMs float64
	_ = st.DB.QueryRow(`SELECT TIMESTAMPDIFF(MICROSECOND, finished_at, run_at) / 1000 FROM script_jobs WHERE id = ?`, int64(j3)).Scan(&delayMs)
	if delayMs < 40 || delayMs > 70 {
		t.Fatalf("first backoff %v ms (want 50)", delayMs)
	}
	if code.ProcessJobs(ctx, 10) != 0 {
		t.Fatal("retried before the backoff")
	}
	time.Sleep(80 * time.Millisecond)
	code.ProcessJobs(ctx, 10)
	_ = st.DB.QueryRow(`SELECT TIMESTAMPDIFF(MICROSECOND, finished_at, run_at) / 1000 FROM script_jobs WHERE id = ?`, int64(j3)).Scan(&delayMs)
	if delayMs < 90 || delayMs > 130 {
		t.Fatalf("second backoff %v ms (want 100)", delayMs)
	}
	time.Sleep(150 * time.Millisecond)
	code.ProcessJobs(ctx, 10)
	if s, a, _, e := jobRow(t, st, j3); s != "failed" || a != 3 || !strings.Contains(e, "fail 3") {
		t.Fatalf("final failure: %s %d %q", s, a, e)
	}
	// timeout
	j4 := enq(map[string]any{"name": "slow"})
	start := time.Now()
	code.ProcessJobs(ctx, 10)
	if s, _, _, e := jobRow(t, st, j4); s != "failed" || !strings.Contains(e, "timeout") || time.Since(start) > 4*time.Second {
		t.Fatalf("timeout: %s %q %v", s, e, time.Since(start))
	}
	// a job the live code does not know
	res, _ := st.DB.Exec(`INSERT INTO script_jobs (name, data, status, attempts, max_attempts, run_at, created_at) VALUES ('ghost', 'null', 'pending', 0, 3, UTC_TIMESTAMP(3), UTC_TIMESTAMP(3))`)
	gid, _ := res.LastInsertId()
	code.ProcessJobs(ctx, 10)
	if s, _, _, e := jobRow(t, st, float64(gid)); s != "failed" || !strings.Contains(e, "job not found") {
		t.Fatalf("unknown job: %s %q", s, e)
	}
	if code, _ := user("Ed", "editor").do("GET", "/api/v1/code/jobs", nil); code != 403 {
		t.Fatal("editor reads jobs")
	}
	// two workers (two engines, same database) claim with SKIP LOCKED: every job runs once
	e2 := sitecode.New(st, sitecode.Config{DSN: os.Getenv("TEST_DB_DSN"), CodeDSN: os.Getenv("TEST_CODE_DB_DSN")})
	defer e2.Close()
	for i := 0; i < 30; i++ {
		if _, err := code.Enqueue("par", fmt.Sprintf(`{"id":%d}`, i), 0, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	counts := make([]int, 2)
	for i, e := range []*sitecode.Engine{code, e2} {
		wg.Add(1)
		go func(i int, e *sitecode.Engine) { defer wg.Done(); counts[i] = e.ProcessJobs(ctx, 100) }(i, e)
	}
	wg.Wait()
	var done, multi int
	_ = st.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(attempts > 1), 0) FROM script_jobs WHERE name = 'par' AND status = 'done'`).Scan(&done, &multi)
	if _, b := c.public("GET", "/count/par"); done != 30 || multi != 0 || counts[0]+counts[1] != 30 || strings.TrimSpace(b) != "30" {
		t.Fatalf("SKIP LOCKED: done %d multi %d counts %v docs %s", done, multi, counts, b)
	}
	t.Logf("two workers ran %d + %d jobs", counts[0], counts[1])

	// cron: 08:00 Bangkok = 01:00 UTC; two instances tick the same minute → one run per schedule
	minute := time.Date(2026, 10, 20, 1, 0, 30, 0, time.UTC)
	if n := code.CronTick(minute) + e2.CronTick(minute); n != 2 {
		t.Fatalf("cron runs started: %d (want 2: work + @cron-1)", n)
	}
	if n := code.CronTick(minute) + e2.CronTick(minute); n != 0 {
		t.Fatalf("second tick in the same minute started %d", n)
	}
	if n := code.CronTick(minute.Add(time.Minute)); n != 1 {
		t.Fatalf("next minute: %d (only the every-minute schedule)", n)
	}
	_, cr := admin.doAny("GET", "/api/v1/code/cron", nil)
	rows := map[string]map[string]any{}
	for _, x := range cr.([]any) {
		rows[x.(map[string]any)["name"].(string)] = x.(map[string]any)
	}
	if rows["work"]["nextRun"] != "2026-10-21T01:00:00Z" || rows["work"]["lastStatus"] != "queued" || rows["@cron-1"]["expr"] != "* * * * *" {
		t.Fatalf("cron list: %v", cr)
	}
	code.ProcessJobs(ctx, 10)
	_, cr = admin.doAny("GET", "/api/v1/code/cron", nil)
	for _, x := range cr.([]any) {
		if x.(map[string]any)["lastStatus"] != "done" {
			t.Fatalf("lastStatus after run: %v", x)
		}
	}
	if _, b := c.public("GET", "/count/cronfn"); strings.TrimSpace(b) != "2" {
		t.Fatalf("function cron runs: %s", b)
	}
	if code, b := admin.do("POST", "/api/v1/code/cron/work/run", nil); code != 200 || b["jobId"] == nil {
		t.Fatalf("run now: %d %v", code, b)
	}
	if code, _ := admin.do("POST", "/api/v1/code/cron/nope/run", nil); code != 404 {
		t.Fatal("run unknown schedule")
	}
	// the worker goroutines (fast polling) and a clean stop
	code.JobPoll, code.JobWorkers = 20*time.Millisecond, 2
	code.StartWorker()
	j9 := enq(map[string]any{"data": map[string]any{"id": 9}})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if s, _, _, _ := jobRow(t, st, j9); s == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not run the job")
		}
		time.Sleep(20 * time.Millisecond)
	}
	code.StopWorker()
	// safe mode pauses the queue
	admin.do("POST", "/api/v1/code/safe-mode", map[string]any{"enabled": true})
	enqDirect, _ := code.Enqueue("work", "null", 0, "", 1)
	if code.ProcessJobs(ctx, 10) != 0 {
		t.Fatal("jobs ran in safe mode")
	}
	if s, _, _, _ := jobRow(t, st, float64(enqDirect)); s != "pending" {
		t.Fatal("job not pending in safe mode")
	}
}

func TestCodeWebhookAndAdminPages(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('POST /hooks/pay', { access: ['*'], rawBody: true, maxBody: 200 }, ({ request }) => {
  const sig = crypto.hmac('sha256', site.settings('PAY_SECRET'), request.rawBody)
  if (!crypto.timingSafeEqual(sig, request.headers['x-signature'] || '')) return { status: 401, body: { error: 'bad signature' } }
  return { ok: true, b64: request.rawBodyBase64, ref: JSON.parse(request.rawBody).ref }
})
site.route('GET /ping', { access: ['*'] }, () => 'pong')
site.action('approve', { input: { id: 'number' }, access: ['editor'], run: ({ input, user }) => ({ message: 'approved ' + input.id + ' by ' + user.role }) })
site.adminPage('orders', { label: { th: 'ออเดอร์', en: 'Orders' }, icon: '🧾', access: ['editor'],
  render: ({ user, query }) => ({ title: 'Orders ' + (query.tab || 'all'), html: '<h1>Hi ' + user.name + ' tab=' + (query.tab || '') + '</h1>' }) })
site.adminPage('boom', { render: () => { throw new Error('render broke') } })
`})
	if code, _ := admin.do("PUT", "/api/v1/code/settings", map[string]any{"values": []any{map[string]any{"key": "PAY_SECRET", "secret": true, "value": "whsec"}},
		"limits": map[string]any{"routePerMinute": 6}}); code != 200 {
		t.Fatal("settings")
	}
	body := `{"ref":"A1","amount":10}`
	m := hmac.New(sha256.New, []byte("whsec"))
	m.Write([]byte(body))
	sig := hex.EncodeToString(m.Sum(nil))
	hook := func(b, s string) (int, string) {
		rec := c.send("POST", "/hooks/pay", strings.NewReader(b), map[string]string{"Content-Type": "application/json", "X-Signature": s, "Accept": "application/json"})
		return rec.Code, rec.Body.String()
	}
	if code, b := hook(body, sig); code != 200 || !strings.Contains(b, `"ref":"A1"`) || !strings.Contains(b, base64.StdEncoding.EncodeToString([]byte(body))) {
		t.Fatalf("signed webhook: %d %s", code, b)
	}
	if code, _ := hook(body, "deadbeef"); code != 401 {
		t.Fatalf("bad signature: %d", code)
	}
	if code, _ := hook(`{"ref":"`+strings.Repeat("x", 300)+`"}`, sig); code != 413 {
		t.Fatalf("maxBody: %d", code)
	}
	// public routes: per-IP limit (setting 6 / minute; 3 calls so far)
	got429 := false
	for i := 0; i < 5; i++ {
		code, _ := c.public("GET", "/ping")
		if i < 3 && code != 200 {
			t.Fatalf("ping %d: %d", i, code)
		}
		got429 = got429 || code == 429
	}
	if !got429 {
		t.Fatal("public route rate limit")
	}
	// admin pages
	editor, author := user("Ed", "editor"), user("Au", "author")
	if _, l := editor.doAny("GET", "/api/v1/code/admin-pages", nil); len(l.([]any)) != 1 || l.([]any)[0].(map[string]any)["id"] != "orders" {
		t.Fatalf("editor's pages: %v", l)
	}
	if _, l := author.doAny("GET", "/api/v1/code/admin-pages", nil); len(l.([]any)) != 0 {
		t.Fatalf("author's pages: %v", l)
	}
	if _, l := admin.doAny("GET", "/api/v1/code/admin-pages", nil); len(l.([]any)) != 2 {
		t.Fatalf("admin's pages: %v", l)
	}
	if _, b := editor.do("GET", "/api/v1/code/bindings", nil); len(b["adminPages"].([]any)) != 1 {
		t.Fatalf("bindings adminPages: %v", b["adminPages"])
	}
	if code, _ := author.do("POST", "/api/v1/code/admin-pages/orders/ticket", map[string]any{}); code != 404 {
		t.Fatalf("author ticket: %d", code)
	}
	code, tk := editor.do("POST", "/api/v1/code/admin-pages/orders/ticket", map[string]any{"query": "tab=open"})
	u, _ := tk["url"].(string)
	if code != 200 || !strings.HasPrefix(u, "/ext-ui/code/") || !strings.HasSuffix(u, "/orders?tab=open") {
		t.Fatalf("ticket: %d %v", code, tk)
	}
	rec := c.send("GET", u, nil, nil)
	h := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(h, "<h1>Hi Ed tab=open</h1>") || !strings.Contains(h, "<title>Orders open</title>") || !strings.Contains(h, "window.anasaki") ||
		!strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox allow-scripts allow-forms allow-popups") {
		t.Fatalf("admin page: %d %v\n%s", rec.Code, rec.Header(), h)
	}
	tok := strings.Split(strings.TrimPrefix(u, "/ext-ui/code/"), "/")[0]
	act := c.send("POST", "/ext-ui/code/"+tok+"/_a/approve", strings.NewReader(`{"input":{"id":1}}`), map[string]string{"Content-Type": "text/plain;charset=utf-8", "Origin": "null"})
	if act.Code != 200 || !strings.Contains(act.Body.String(), `"message":"approved 1 by editor"`) || act.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("anasaki.call: %d %s", act.Code, act.Body.String())
	}
	if pre := c.send("OPTIONS", "/ext-ui/code/"+tok+"/_a/approve", nil, nil); pre.Code != 204 {
		t.Fatalf("preflight: %d", pre.Code)
	}
	if bad := c.send("POST", "/ext-ui/code/"+tok+"/_a/approve", strings.NewReader(`{"input":{"id":"x"}}`), nil); bad.Code != 400 {
		t.Fatalf("action validation: %d", bad.Code)
	}
	if r := c.send("GET", "/ext-ui/code/garbage/orders", nil, nil); r.Code != 401 {
		t.Fatalf("bad ticket: %d", r.Code)
	}
	if r := c.send("GET", "/ext-ui/code/"+tok+"/boom", nil, nil); r.Code != 403 {
		t.Fatalf("editor opens an admin-only page: %d", r.Code)
	}
	_, atk := admin.do("POST", "/api/v1/code/admin-pages/boom/ticket", nil)
	if r := c.send("GET", atk["url"].(string), nil, nil); r.Code != 500 || strings.Contains(r.Body.String(), "render broke") {
		t.Fatalf("failing render: %d %s", r.Code, r.Body.String())
	}
	_ = auth.CapEditPages
}
