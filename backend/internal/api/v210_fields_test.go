package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// CMS 2.1.0 (P2-7): custom field types relation, user, term, file, gallery,
// repeater, richtext and color — declared in site.postType / site.fields,
// listed by GET /post-types, validated on save (REST and db.posts).

func TestFieldTypes210(t *testing.T) {
	c, _, _ := newCodeAPI(t, nil)
	admin, user := setupSite(t, c)
	publishCode(t, admin, map[string]string{"main.ts": `
site.postType('event', { label: 'Events', slug: 'events', fields: {
  related: { type: 'relation', postType: ['event', 'post'], multiple: true, max: 3 },
  host: 'user',
  genre: { type: 'term', taxonomy: 'category' },
  flyer: 'file',
  photos: 'gallery',
  tickets: { type: 'repeater', max: 2, fields: { name: { type: 'text', required: true }, price: { type: 'number', min: 0 } } },
  lead: 'richtext',
  accent: 'color',
} })
site.route('POST /mk', { access: ['*'] }, ({ request }) => db.asSystem(() => db.posts.create({ type: 'event', title: 'E', meta: request.json() })))
`})
	if code, b := publishBad(admin, `site.postType('xx', { fields: { r: { type: 'repeater' } } })`); code == 200 || !strings.Contains(b, "repeater") {
		t.Fatalf("repeater without fields: %d %s", code, b)
	}
	if code, b := publishBad(admin, `site.postType('xx', { fields: { r: { type: 'repeater', fields: { g: 'gallery' } } } })`); code == 200 || !strings.Contains(b, "gallery") {
		t.Fatalf("gallery inside a repeater: %d %s", code, b)
	}
	_, pt := admin.do("GET", "/api/v1/post-types", nil)
	spec := jsonString(pt["postTypes"])
	for _, want := range []string{`"type":"relation"`, `"postTypes":["event","post"]`, `"multiple":true`, `"taxonomy":"category"`,
		`"type":"repeater"`, `"fields":{"name":{`, `"type":"gallery"`, `"type":"richtext"`} {
		if !strings.Contains(spec, want) {
			t.Fatalf("catalog lacks %s: %s", want, spec)
		}
	}
	good := map[string]any{"related": []any{1, "2", 2}, "host": 1, "genre": 5, "flyer": 7, "photos": []any{3, 4},
		"tickets": []any{map[string]any{"name": "VIP", "price": 990}, map[string]any{}}, "lead": `<p>Hi<script>x()</script></p>`, "accent": "#ff8800"}
	code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "event", "title": "Gig", "meta": good})
	if code != 201 {
		t.Fatalf("save: %d %v", code, b)
	}
	m := b["meta"].(map[string]any)
	if jsonString(m["related"]) != "[1,2]" || m["host"] != 1.0 || jsonString(m["photos"]) != "[3,4]" || m["accent"] != "#ff8800" ||
		jsonString(m["tickets"]) != `[{"name":"VIP","price":990}]` || strings.Contains(m["lead"].(string), "script") {
		t.Fatalf("stored meta: %v", m)
	}
	for name, bad := range map[string]map[string]any{
		"relation max":     {"related": []any{1, 2, 3, 4}},
		"single gets list": {"host": []any{1, 2}},
		"bad id":           {"flyer": -1},
		"repeater rows":    {"tickets": []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}, map[string]any{"name": "c"}}},
		"repeater req":     {"tickets": []any{map[string]any{"price": 1}}},
		"repeater sub":     {"tickets": []any{map[string]any{"name": "a", "price": -1}}},
		"color":            {"accent": "url(javascript:x)"},
	} {
		if code, b := admin.do("POST", "/api/v1/posts", map[string]any{"type": "event", "title": "Bad", "meta": bad}); code != 400 {
			t.Errorf("%s: %d %v", name, code, b)
		}
	}
	// db.posts validates the same way
	if code, _, raw := postJSON(c, "/mk", map[string]any{"tickets": []any{map[string]any{"name": "x", "price": "abc"}}}); code != 500 { // a visitor sees a generic error
		t.Fatalf("db.posts repeater: %d %s", code, raw)
	}
	// the user picker: names only, for editors
	ed := user("Ed", "editor")
	if code, l := ed.doList("GET", "/api/v1/users/options?search=own"); code != 200 || len(l) != 1 || l[0]["name"] != "Owner" || l[0]["email"] != nil {
		t.Fatalf("user options: %d %v", code, l)
	}
	if code, _ := user("Au", "author").doList("GET", "/api/v1/users/options"); code != 403 {
		t.Fatalf("author lists users: %d", code)
	}
}

// publishBad publishes main.ts and returns the status and body (no t.Fatal).
func publishBad(admin *client, main string) (int, string) {
	code, f := admin.do("GET", "/api/v1/code/files/main.ts", nil)
	base := ""
	if code == 200 {
		base, _ = f["updatedAt"].(string)
	}
	admin.put("main.ts", main, base)
	_, st := admin.do("GET", "/api/v1/code/status", nil)
	code, b := publishWith(admin, st["liveVersion"], true)
	return code, jsonString(b)
}

// postJSON POSTs a JSON body and returns the status, decoded object and raw body.
func postJSON(cl *client, path string, body any) (int, map[string]any, string) {
	rec := cl.send("POST", path, strings.NewReader(jsonString(body)), map[string]string{"Accept": "application/json", "Content-Type": "application/json"})
	out := map[string]any{}
	_ = jsonUnmarshalTest(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func jsonUnmarshalTest(b []byte, v any) error { return json.Unmarshal(b, v) }
