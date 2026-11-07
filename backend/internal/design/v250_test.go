package design

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// recData records the post queries and answers the Fields block (2.5.0).
type recData struct {
	fakeData
	queries []Query
	maxes   []int
}

func (r *recData) QueryPosts(q Query) []PostCtx {
	r.queries = append(r.queries, q)
	return []PostCtx{{"title": "Condo", "type": "listing", "meta": `{"price":3200000}`}, {"title": "Blog post", "type": "post"}}
}

func (r *recData) PostFields(post PostCtx, max int, _ bool) string {
	r.maxes = append(r.maxes, max)
	if post["type"] == "post" {
		return ""
	}
	return `<dl class="rea-fields"><div class="rea-field"><dt>ราคา</dt><dd>3,200,000</dd></div></dl>`
}

// 2.5.0: a post list filters and sorts by custom fields — its own conditions (a number, "today", a URL
// parameter) and those its inputs send (?<key>_f_ / _min_ / _max_); bad keys and operators are dropped.
func TestPostListFieldConditions(t *testing.T) {
	doc := `{"id":"root","type":"frame","children":[
	 {"id":"homes","type":"postList","props":{"query":{"postType":"listing","orderBy":"price","orderDesc":true,"where":[
	   {"field":"price","op":">=","value":"1000000"},
	   {"field":"when","op":">=","value":"today"},
	   {"field":"beds","value":"query.beds"},
	   {"field":"rooms","value":"query.missing"},
	   {"field":"bad key","value":"1"},
	   {"field":"price","op":"LIKE","value":"1"}]}},
	  "children":[{"id":"t","type":"text","props":{"text":"{{post.title}}"}}]}]}`
	d := &recData{}
	q := url.Values{"beds": {"3"}, "homes_min_area": {"50"}, "homes_f_district": {"บางนา"}, "homes_max_bad-key": {"1"}, "other_f_x": {"1"}}
	renderDoc(t, doc, Context{Query: q, Loc: time.UTC}, d)
	if len(d.queries) != 1 {
		t.Fatalf("queries: %d", len(d.queries))
	}
	got := d.queries[0]
	if got.OrderBy != "price" || !got.OrderDesc {
		t.Errorf("order: %q desc=%v", got.OrderBy, got.OrderDesc)
	}
	want := map[string]FieldCond{
		"price":    {"price", ">=", "1000000"},
		"when":     {"when", ">=", time.Now().UTC().Format("2006-01-02")},
		"beds":     {"beds", "=", "3"},
		"area":     {"area", ">=", "50"},
		"district": {"district", "=", "บางนา"},
	}
	if len(got.Where) != len(want) {
		t.Fatalf("conditions: %+v", got.Where)
	}
	for _, c := range got.Where {
		if w, ok := want[c.Key]; !ok || w != c {
			t.Errorf("condition %+v (want %+v)", c, want[c.Key])
		}
	}
}

// 2.5.0: the Fields block renders the source's HTML for each post (inline layout for cards, max passed on) and
// nothing at all when a post has no fields.
func TestPostFieldsBlock(t *testing.T) {
	doc := `{"id":"root","type":"frame","children":[
	 {"id":"list","type":"postList","children":[{"id":"card","type":"frame","children":[
	  {"id":"tt","type":"text","props":{"text":"{{post.title}}"}},
	  {"id":"pf","type":"postFields","props":{"max":3,"layout":"inline"}}]}]}]}`
	d := &recData{}
	out := renderDoc(t, doc, Context{}, d)
	must(t, out.HTML, `<div class="n-pf rea-fields-inline"><dl class="rea-fields"><div class="rea-field"><dt>ราคา</dt><dd>3,200,000</dd></div></dl></div>`)
	if strings.Count(out.HTML, `class="n-pf`) != 1 {
		t.Fatalf("the blog post (no fields) got a Fields block:\n%s", out.HTML)
	}
	if len(d.maxes) != 2 || d.maxes[0] != 3 {
		t.Fatalf("max: %v", d.maxes)
	}
	// without a FieldsSource the block is simply left out
	if o := renderDoc(t, doc, Context{}, fakeData{}); strings.Contains(o.HTML, "n-pf") {
		t.Fatal("Fields block without a source")
	}
}
