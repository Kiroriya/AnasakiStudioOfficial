package api

import (
	"fmt"
	"strings"
	"testing"
)

// CMS 2.1.0: a JS plugin registers a taxonomy (scope "taxonomies", id
// "<plugin>_<name>") for its own post type and writes its terms (scope
// "terms-write"); Site Code reads them but may not write them.

const ptxMain = `
site.postType('px_kit_item', { label: 'Items', slug: 'px-items' })
site.taxonomy('px_kit_brand', { label: 'Brands', postTypes: ['px_kit_item'], hierarchical: true })
site.route('POST /pxkit/mk', { access: ['*'] }, () => db.asSystem(() => {
  const b = db.terms.create({ taxonomy: 'px_kit_brand', name: 'Acme' })
  const p = db.posts.create({ type: 'px_kit_item', title: 'Anvil', status: 'publish', terms: { px_kit_brand: [b.slug] } })
  return { b, p }
}))
`

func TestPluginTaxonomy(t *testing.T) {
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	scopes := []any{"routes", "content", "content-write", "post-types", "taxonomies", "terms-write"}
	fail := func(main string, sc []any, want string) {
		t.Helper()
		mustUpload(t, admin, ptkPackage("px-kit", main, sc, []any{"/pxkit"}))
		code, out := admin.do("POST", "/api/v1/package-plugins/px-kit/activate", map[string]any{})
		if code == 200 || !strings.Contains(fmt.Sprint(out), want) {
			t.Fatalf("activation should fail with %q: %d %v", want, code, out)
		}
		admin.do("DELETE", "/api/v1/package-plugins/px-kit", nil)
	}
	fail(ptxMain, []any{"routes", "content", "content-write", "post-types", "terms-write"}, "taxonomies")
	fail(`site.taxonomy('brand', { label: 'B', postTypes: ['post'] })`, scopes, `"px_kit_"`)

	mustUpload(t, admin, ptkPackage("px-kit", ptxMain, scopes, []any{"/pxkit"}))
	activate(t, admin, "px-kit")
	rec := e.c.send("POST", "/pxkit/mk", nil, map[string]string{"Accept": "application/json"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"px_kit_brand":[{`) {
		t.Fatalf("plugin terms: %d %s", rec.Code, rec.Body.String())
	}
	_, txs := admin.doList("GET", "/api/v1/taxonomies")
	if len(txs) != 3 || txs[2]["id"] != "tag" || txs[1]["id"] != "px_kit_brand" || txs[1]["plugin"] != "px-kit" || txs[1]["slug"] != "px-kit-brand" {
		t.Fatalf("catalog: %v", txs)
	}
	if code, b := e.c.public("GET", "/px-kit-brand/acme"); code != 200 || !strings.Contains(b, "Anvil") {
		t.Fatalf("plugin term archive: %d %.200s", code, b)
	}
	// Site Code reads the plugin's terms but may not write them
	publishCode(t, admin, map[string]string{"main.ts": `
site.route('GET /sc/terms', { access: ['*'] }, () => db.terms.list({ taxonomy: 'px_kit_brand' }).map((t) => t.name).join(','))
site.route('GET /sc/write', { access: ['*'] }, () => { try { db.asSystem(() => db.terms.create({ taxonomy: 'px_kit_brand', name: 'x' })); return 'wrote' } catch (e) { return 'refused' } })`})
	if _, b := e.c.public("GET", "/sc/terms"); !strings.Contains(b, "Acme") {
		t.Fatalf("Site Code reads plugin terms: %s", b)
	}
	if _, b := e.c.public("GET", "/sc/write"); !strings.Contains(b, "refused") {
		t.Fatalf("Site Code writes plugin terms: %s", b)
	}
}
