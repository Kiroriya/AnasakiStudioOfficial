package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"strings"
	"testing"
	"time"
)

// CMS 2.1.0 (P1-5): the store fires the core events for every write, with
// "actor": {id, source} and "previous" on updates — from the admin REST API,
// Site Code (db.posts) and the scheduler alike.

func TestCoreEventCatalog(t *testing.T) {
	c, code, _ := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	events := []string{"post.created", "post.updated", "post.saved", "post.statusChanged", "post.published", "post.scheduled",
		"post.unpublished", "post.deleted", "user.created", "user.updated", "user.roleChanged", "user.deleted",
		"media.uploaded", "media.updated", "media.deleted", "term.created", "term.updated", "term.deleted", "settings.updated"}
	js := "const ev = (name) => site.on(name, (p) => { cache.set(name + ':' + (p.id ?? ''), p); cache.set(name + ':last', p) })\n"
	for _, e := range events {
		js += "ev('" + e + "')\n"
	}
	js += `site.route('GET /ev/{name}', { access: ['*'] }, ({ request }) => cache.get(request.params.name) ?? null)
site.route('POST /code-post', { access: ['*'] }, () => db.asSystem(() => db.posts.create({ title: 'from code', status: 'publish' })))
`
	publishCode(t, admin, map[string]string{"main.ts": js})
	got := func(key string) map[string]any {
		t.Helper()
		code.WaitEvents()
		_, b := c.public("GET", "/ev/"+key)
		out := map[string]any{}
		if json.Unmarshal([]byte(b), &out) != nil {
			return nil
		}
		return out
	}
	actor := func(p map[string]any) string {
		a, _ := p["actor"].(map[string]any)
		if a == nil {
			return ""
		}
		return a["source"].(string) + "#" + jsonNum(a["id"].(float64))
	}
	_, me := admin.do("GET", "/api/v1/auth/me", nil)
	if me["user"] != nil {
		me = me["user"].(map[string]any)
	}
	adminActor := "admin#" + jsonNum(me["id"].(float64))

	// posts: draft → publish → unpublish → delete
	_, p := admin.do("POST", "/api/v1/posts", map[string]any{"title": "E1"})
	pid := id(p)
	if e := got("post.created:" + pid); e == nil || e["status"] != "draft" || actor(e) != adminActor {
		t.Fatalf("post.created: %v", e)
	}
	if e := got("post.saved:" + pid); e == nil || e["isNew"] != true {
		t.Fatalf("post.saved (new): %v", e)
	}
	admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"title": "E2", "status": "publish"})
	if e := got("post.updated:" + pid); e == nil || e["title"] != "E2" || e["previous"].(map[string]any)["title"] != "E1" ||
		e["previous"].(map[string]any)["status"] != "draft" {
		t.Fatalf("post.updated: %v", e)
	}
	if e := got("post.statusChanged:" + pid); e == nil || e["from"] != "draft" || e["to"] != "publish" {
		t.Fatalf("post.statusChanged: %v", e)
	}
	if e := got("post.published:" + pid); e == nil || e["url"] != "/blog/e1" && !strings.HasPrefix(e["url"].(string), "/") {
		t.Fatalf("post.published: %v", e)
	}
	admin.do("PUT", "/api/v1/posts/"+pid, map[string]any{"status": "draft"})
	if e := got("post.unpublished:" + pid); e == nil || e["status"] != "draft" {
		t.Fatalf("post.unpublished: %v", e)
	}
	// scheduled, then published by the scheduler (actor "scheduler")
	_, s := admin.do("POST", "/api/v1/posts", map[string]any{"title": "Sch", "status": "publish", "publishedAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
	if e := got("post.scheduled:" + id(s)); e == nil || e["status"] != "scheduled" || e["publishedAt"] == nil {
		t.Fatalf("post.scheduled: %v", e)
	}
	c.api.PublishScheduled(time.Now().Add(2 * time.Hour))
	if e := got("post.published:" + id(s)); e == nil || actor(e) != "scheduler#0" {
		t.Fatalf("post.published by the scheduler: %v", e)
	}
	admin.do("DELETE", "/api/v1/posts/"+pid, nil)
	if e := got("post.deleted:" + pid); e == nil || e["title"] != "E2" || actor(e) != adminActor {
		t.Fatalf("post.deleted: %v", e)
	}
	// Site Code writes: source "code"
	_, cp := c.send2("POST", "/code-post")
	if e := got("post.created:" + id(cp)); e == nil || actor(e) != "code#0" {
		t.Fatalf("post.created from code: %v", e)
	}

	// users
	u := user("Uma", "author")
	_ = u
	ue := got("user.created:last")
	if ue == nil || ue["email"] != "uma@x.io" || ue["role"] != "author" || actor(ue) != adminActor {
		t.Fatalf("user.created: %v", ue)
	}
	uid := jsonNum(ue["id"].(float64))
	admin.do("PUT", "/api/v1/users/"+uid, map[string]any{"role": "editor"})
	if e := got("user.roleChanged:" + uid); e == nil || e["from"] != "author" || e["to"] != "editor" {
		t.Fatalf("user.roleChanged: %v", e)
	}
	if e := got("user.updated:" + uid); e == nil || e["previous"].(map[string]any)["role"] != "author" || strings.Contains(strings.ToLower(jsonString(e)), "hash") {
		t.Fatalf("user.updated: %v", e)
	}
	admin.do("DELETE", "/api/v1/users/"+uid, nil)
	if e := got("user.deleted:" + uid); e == nil || e["email"] != "uma@x.io" {
		t.Fatalf("user.deleted: %v", e)
	}

	// terms
	_, term := admin.do("POST", "/api/v1/terms", map[string]any{"name": "News", "taxonomy": "category"})
	tid := id(term)
	admin.do("PUT", "/api/v1/terms/"+tid, map[string]any{"name": "World news", "slug": "world"})
	if e := got("term.updated:" + tid); e == nil || e["slug"] != "world" || e["previous"].(map[string]any)["slug"] != "news" || e["taxonomy"] != "category" {
		t.Fatalf("term.updated: %v", e)
	}
	admin.do("DELETE", "/api/v1/terms/"+tid, nil)
	if e := got("term.deleted:" + tid); e == nil || e["name"] != "World news" {
		t.Fatalf("term.deleted: %v", e)
	}
	if got("term.created:"+tid) == nil {
		t.Fatal("term.created missing")
	}

	// media
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "dot.png")
	_, _ = fw.Write(tinyPNG)
	_ = mw.Close()
	rec := admin.send("POST", "/api/v1/media", &buf, map[string]string{"Content-Type": mw.FormDataContentType()})
	m := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if rec.Code != 201 && rec.Code != 200 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	mid := id(m)
	if e := got("media.uploaded:" + mid); e == nil || e["kind"] != "image" || e["mime"] != "image/png" {
		t.Fatalf("media.uploaded: %v", e)
	}
	admin.do("PUT", "/api/v1/media/"+mid, map[string]any{"alt": "a dot"})
	if e := got("media.updated:" + mid); e == nil || e["alt"] != "a dot" || e["previous"].(map[string]any)["alt"] != "" {
		t.Fatalf("media.updated: %v", e)
	}
	admin.do("DELETE", "/api/v1/media/"+mid, nil)
	if e := got("media.deleted:" + mid); e == nil {
		t.Fatal("media.deleted missing")
	}

	// settings: the changed keys, never the values
	admin.do("PUT", "/api/v1/settings", map[string]any{"tagline": "New tagline"})
	if e := got("settings.updated:last"); e == nil || !strings.Contains(jsonString(e["keys"]), "tagline") || strings.Contains(jsonString(e), "New tagline") {
		t.Fatalf("settings.updated: %v", e)
	}
}

// send2 POSTs with no body and decodes the JSON answer.
func (c *client) send2(method, path string) (int, map[string]any) {
	c.t.Helper()
	rec := c.send(method, path, strings.NewReader(""), map[string]string{"Accept": "application/json", "Content-Type": "application/json"})
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// tinyPNG is a 1×1 transparent PNG.
var tinyPNG = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}
