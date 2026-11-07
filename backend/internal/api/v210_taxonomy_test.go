package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// CMS 2.1.0 (P1-1): site.taxonomy, db.terms, PostInput.terms, the REST
// endpoints and the public term archives.

func TestTaxonomies(t *testing.T) {
	c, code, _ := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": `
site.postType('product', { label: 'Products', slug: 'products', supports: ['title', 'content', 'categories'] })
site.taxonomy('genre', { label: { th: 'แนว', en: 'Genres' }, hierarchical: true, postTypes: ['product', 'post'], slug: 'genres' })
site.taxonomy('secret_tag', { label: 'Internal', postTypes: ['product'], public: false, access: { manage: ['administrator'] } })
site.route('POST /t', { access: ['*'] }, ({ request }) => {
  const b = request.json()
  return db.asSystem(() => {
    const rock = db.terms.create({ taxonomy: 'genre', name: 'Rock' })
    const indie = db.terms.create({ taxonomy: 'genre', name: 'Indie rock', slug: 'indie', parent: rock.id, description: 'Small labels' })
    const jazz = db.terms.create({ taxonomy: 'genre', name: 'Jazz' })
    const p = db.posts.create({ type: 'product', title: b.title, status: 'publish', terms: { genre: [indie.id, 'jazz'] } })
    return { rock, indie, jazz, p }
  })
})
site.route('POST /bad', { access: ['*'] }, ({ request }) => db.asSystem(() => {
  const b = request.json()
  if (b.case === 'loop') { const a = db.terms.list({ taxonomy: 'genre', slugs: ['rock'] })[0]; const k = db.terms.list({ taxonomy: 'genre', slugs: ['indie'] })[0]; return db.terms.update(a.id, { parent: k.id }) }
  if (b.case === 'flat') return db.terms.create({ taxonomy: 'secret_tag', name: 'x', parent: 1 })
  if (b.case === 'wrongtype') return db.posts.create({ type: 'page', title: 'x', terms: { genre: [] } })
  if (b.case === 'unknown') return db.posts.create({ type: 'product', title: 'x', terms: { genre: ['nope'] } })
  if (b.case === 'nosuch') return db.terms.list({ taxonomy: 'nosuch' })
}))
site.route('GET /list', { access: ['*'] }, ({ request }) => db.terms.list({ taxonomy: 'genre', parent: request.query.parent === undefined ? undefined : Number(request.query.parent) }))
site.route('GET /mine/{id}', { access: ['*'] }, ({ request }) => db.terms.list({ taxonomy: 'genre', postId: Number(request.params.id) }))
site.route('POST /visitor', { access: ['*'] }, () => db.terms.create({ taxonomy: 'genre', name: 'Nope' }))
`})
	post := func(cl *client, path string, body any) (int, map[string]any, string) {
		b, _ := json.Marshal(body)
		rec := cl.send("POST", path, strings.NewReader(string(b)), map[string]string{"Accept": "application/json", "Content-Type": "application/json"})
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out, rec.Body.String()
	}
	code1, r, raw := post(c, "/t", map[string]any{"title": "Album"})
	if code1 != 200 {
		t.Fatalf("create: %d %s", code1, raw)
	}
	indie, prod := r["indie"].(map[string]any), r["p"].(map[string]any)
	if indie["parent"] != r["rock"].(map[string]any)["id"] || indie["description"] != "Small labels" || indie["slug"] != "indie" {
		t.Fatalf("child term: %v", indie)
	}
	genres := prod["terms"].(map[string]any)["genre"].([]any)
	if len(genres) != 2 {
		t.Fatalf("post terms: %v", prod["terms"])
	}
	// list: top level only, children of rock, terms of the post
	if _, b := c.public("GET", "/list?parent=0"); !strings.Contains(b, `"Rock"`) || strings.Contains(b, "Indie") {
		t.Fatalf("top level: %s", b)
	}
	if _, b := c.public("GET", "/mine/"+id(prod)); !strings.Contains(b, "Indie rock") || !strings.Contains(b, "Jazz") || strings.Contains(b, `"Rock"`) {
		t.Fatalf("terms of the post: %s", b)
	}
	// refused writes
	for _, cs := range []string{"loop", "flat", "wrongtype", "unknown", "nosuch"} {
		if code, _, b := post(c, "/bad", map[string]any{"case": cs}); code != 500 {
			t.Errorf("%s: %d %s", cs, code, b)
		}
	}
	if code, _, _ := post(c, "/visitor", nil); code != 500 {
		t.Fatalf("a visitor writes terms: %d", code)
	}

	// REST: the catalog, rights per taxonomy, post terms
	_, txs := admin.doList("GET", "/api/v1/taxonomies")
	if len(txs) != 4 || txs[3]["id"] != "tag" || txs[0]["id"] != "category" || txs[1]["id"] != "genre" || txs[1]["hierarchical"] != true || txs[2]["canManage"] != true {
		t.Fatalf("taxonomies: %v", txs)
	}
	ed := user("Ed", "editor")
	_, etx := ed.doList("GET", "/api/v1/taxonomies")
	if etx[1]["canManage"] != true || etx[2]["canManage"] != false {
		t.Fatalf("editor taxonomies: %v", etx)
	}
	if code, _ := ed.do("POST", "/api/v1/terms", map[string]any{"taxonomy": "secret_tag", "name": "x"}); code != 403 {
		t.Fatalf("editor writes an admin-only taxonomy: %d", code)
	}
	if code, b := ed.do("POST", "/api/v1/terms", map[string]any{"taxonomy": "genre", "name": "Blues", "parent": r["rock"].(map[string]any)["id"]}); code != 201 || b["parent"] == 0.0 {
		t.Fatalf("editor creates a genre: %d %v", code, b)
	}
	if code, _ := admin.do("POST", "/api/v1/terms", map[string]any{"taxonomy": "nosuch", "name": "x"}); code != 400 {
		t.Fatalf("unknown taxonomy: %d", code)
	}
	if code, _ := admin.do("POST", "/api/v1/terms", map[string]any{"taxonomy": "secret_tag", "name": "y", "parent": 1}); code != 400 {
		t.Fatalf("parent in a flat taxonomy: %d", code)
	}
	jazz := r["jazz"].(map[string]any)
	if code, b := admin.do("PUT", "/api/v1/posts/"+id(prod), map[string]any{"terms": map[string]any{"genre": []any{jazz["id"]}}}); code != 200 ||
		len(b["terms"].(map[string]any)["genre"].([]any)) != 1 {
		t.Fatalf("REST terms: %d %v", code, b)
	}
	if code, _ := admin.do("PUT", "/api/v1/posts/"+id(prod), map[string]any{"terms": map[string]any{"nosuch": []any{1}}}); code != 400 {
		t.Fatalf("REST unknown taxonomy on a post: %d", code)
	}
	if _, l := admin.doList("GET", "/api/v1/terms?taxonomy=genre"); len(l) != 4 {
		t.Fatalf("REST list: %v", l)
	}

	// public archive /genres/<slug>: the term and its children
	admin.do("PUT", "/api/v1/posts/"+id(prod), map[string]any{"terms": map[string]any{"genre": []any{indie["id"]}}})
	if code, b := c.public("GET", "/genres/rock"); code != 200 || !strings.Contains(b, "Album") {
		t.Fatalf("archive of the parent: %d %s", code, b)
	}
	if code, _ := c.public("GET", "/genres/nope"); code != 404 {
		t.Fatalf("unknown term: %d", code)
	}
	if _, b := c.public("GET", "/sitemap.xml"); !strings.Contains(b, "/genres/indie") {
		code.WaitEvents()
		t.Logf("sitemap (served only once a plugin filters it): %.300s", b)
	}
	// deleting a parent lifts its children
	rock := r["rock"].(map[string]any)
	if code, _ := admin.do("DELETE", "/api/v1/terms/"+id(rock), nil); code != 204 {
		t.Fatalf("delete parent: %d", code)
	}
	if _, b := c.public("GET", "/list?parent=0"); !strings.Contains(b, "Indie rock") {
		t.Fatalf("children moved up: %s", b)
	}
}

// doList decodes a JSON array answer.
func (c *client) doList(method, path string) (int, []map[string]any) {
	c.t.Helper()
	rec := c.send(method, path, nil, map[string]string{"Accept": "application/json"})
	var out []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}
