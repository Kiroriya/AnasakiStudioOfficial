package design

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// fakeDyn is fakeData plus Site Code collections and formatters.
type fakeDyn struct {
	fakeData
	reqs []CollectionReq
}

func (f *fakeDyn) LoadCollection(req CollectionReq) CollectionRes {
	f.reqs = append(f.reqs, req)
	per := req.PerPage
	if per == 0 {
		per = 2
	}
	switch req.Source {
	case "code:products":
		all := []map[string]any{}
		for i := 1; i <= 5; i++ {
			all = append(all, map[string]any{"id": float64(i), "name": fmt.Sprintf("P<%d>", i), "price": "10.50", "stock": float64(i % 2)})
		}
		if q, _ := req.Params["q"].(string); q != "" {
			var out []map[string]any
			for _, it := range all {
				if strings.Contains(it["name"].(string), q) {
					out = append(out, it)
				}
			}
			all = out
		}
		lo := min((req.Page-1)*per, len(all))
		return CollectionRes{Items: all[lo:min(lo+per, len(all))], Total: len(all), PerPage: per, Page: req.Page}
	case "table:orders":
		return CollectionRes{Items: []map[string]any{{"code": "A1"}}, Total: 1, PerPage: per, Page: 1}
	case "code:secret":
		return CollectionRes{Denied: true, PerPage: per}
	}
	return CollectionRes{PerPage: per}
}

func (f *fakeDyn) Format(name string, v any, args []any) (string, bool) {
	if name == "baht" {
		return "฿" + Stringify(v), true
	}
	return "", false
}

func renderDoc(t *testing.T, doc string, ctx Context, data DataSource) Output {
	t.Helper()
	root, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return Render(root, ctx, data)
}

func must(t *testing.T, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			t.Errorf("expected %q in:\n%s", sub, s)
		}
	}
}

func mustNot(t *testing.T, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			t.Errorf("did not expect %q in:\n%s", sub, s)
		}
	}
}

const shopDoc = `{"id":"root","type":"frame","layout":{"mode":"vertical"},"children":[
 {"id":"pg1","type":"pagination","props":{"collection":"list","style":"numbers","window":1}},
 {"id":"q","type":"input","props":{"kind":"search","placeholder":"ค้นหา","filter":{"collection":"list","param":"q"}}},
 {"id":"list","type":"collection","props":{"source":"code:products","perPage":2,"key":"shop","params":{"cat":"query.cat"}},"children":[
   {"id":"card","type":"frame","children":[
     {"id":"name","type":"text","props":{"text":"{{ $index + 1 }}. {{item.name}} {{ item.price * 2 | number:2 }} {{ item.price | baht }}"}},
     {"id":"out","type":"text","props":{"text":"หมด","showWhen":"item.stock == 0"}},
     {"id":"buy","type":"button","props":{"text":"Buy","action":{"type":"code","name":"addToCart","input":{"id":"item.id","note":"'<x> \"q\"'","qty":"2"},"confirm":"Sure {{item.name}}?","update":["cartBox"],"success":{"message":"Added","redirect":"/cart"}}}}
   ]},
   {"id":"empty","type":"text","props":{"text":"Nothing here","showWhen":"$empty"}}
 ]},
 {"id":"pg2","type":"pagination","props":{"collection":"list","style":"prevnext","labels":{"prev":"Back","next":{"th":"ต่อ","en":"On"}}}},
 {"id":"pg3","type":"pagination","props":{"collection":"list","style":"loadmore","itemStyle":{"fill":"#eee","radius":6}}},
 {"id":"cartBox","type":"text","props":{"text":"Total {{page.total | number}}","showWhen":"page.total > 0"}},
 {"id":"hidden","type":"frame","props":{"showWhen":"query.debug"},"children":[{"id":"secret","type":"text","props":{"text":"SECRET"}}]}
]}`

func TestCodeCollectionPaginationAndBindings(t *testing.T) {
	dyn := &fakeDyn{}
	q, _ := url.ParseQuery("shop_page=2&x=1")
	out := renderDoc(t, shopDoc, Context{Query: q, Path: "/shop", Page: map[string]any{"total": 1234.5}, Locale: "en"}, dyn)
	h := out.HTML
	// collection root, items of page 2 with expressions and script formatters
	must(t, h, `<div class="n-list" data-rea-collection="list" data-rea-key="shop" data-rea-source="code:products">`,
		`1. P&lt;3&gt; 21.00 ฿10.50`, `2. P&lt;4&gt; 21.00 ฿10.50`)
	mustNot(t, h, "P&lt;1&gt;", "P&lt;5&gt;", "Nothing here", "SECRET", `class="n-hidden"`)
	// showWhen per item: only the out-of-stock item (P4, stock 0) shows หมด
	if strings.Count(h, `<p class="n-out">หมด</p>`) != 1 {
		t.Errorf("showWhen per item:\n%s", h)
	}
	if len(dyn.reqs) != 1 || dyn.reqs[0].Page != 2 || dyn.reqs[0].PerPage != 2 || dyn.reqs[0].Params["cat"] != nil {
		t.Errorf("collection request: %+v", dyn.reqs)
	}
	// numbers: window 1 around page 2 of 3, other query params kept, rel prev/next
	must(t, h,
		`<nav class="n-pg1 rea-pagination" data-rea-pagination="list" data-rea-style="numbers" aria-label="Pagination">`,
		`<a class="rea-pg-item rea-pg-prev" href="/shop?x=1" rel="prev">Previous</a>`,
		`<a class="rea-pg-item" href="/shop?x=1">1</a>`,
		`<a class="rea-pg-item rea-pg-active" href="/shop?shop_page=2&amp;x=1" aria-current="page">2</a>`,
		`<a class="rea-pg-item" href="/shop?shop_page=3&amp;x=1">3</a>`,
		`<a class="rea-pg-item rea-pg-next" href="/shop?shop_page=3&amp;x=1" rel="next">Next</a>`)
	// prev/next with labels; loadmore with the next page URL
	must(t, h, `>Back</a>`, `<span class="rea-pg-status">2 / 3</span>`, `rel="next">On</a>`,
		`<a class="rea-pg-item rea-pg-more" href="/shop?shop_page=3&amp;x=1" rel="next" data-rea-more>Load more</a>`)
	must(t, out.Head, `<link rel="prev" href="/shop?x=1">`, `<link rel="next" href="/shop?shop_page=3&amp;x=1">`)
	must(t, out.CSS, ".n-pg3.rea-pagination .rea-pg-item{background:#eee;border-radius:6px}", ".rea-pagination{display:flex")
	// action markup: input evaluated server-side, escaped in the attribute
	must(t, h, `<a data-rea-action="addToCart" data-rea-input="{&#34;id&#34;:3,&#34;note&#34;:&#34;&lt;x&gt; \&#34;q\&#34;&#34;,&#34;qty&#34;:2}" data-rea-confirm="Sure P&lt;3&gt;?" data-rea-update="cartBox" data-rea-success="Added" data-rea-redirect="/cart" class="n-buy rea-btn" href="#">Buy</a>`)
	// filter input: name <key>_<param>, GET form fallback keeping other params, error slot
	must(t, h, `<form class="rea-filter-form" method="get" action="/shop" style="display:contents" data-rea-filter-form><input type="hidden" name="x" value="1">`,
		`<div class="n-q rea-field"><input class="rea-field-el" type="search" name="shop_q" data-rea-filter="list" data-rea-reset="shop_page" placeholder="ค้นหา"><span class="rea-error" data-rea-error-for="shop_q"></span></div></form>`)
	// page data
	must(t, h, `<p class="n-cartBox">Total 1,234.5</p>`)
	if !out.Runtime {
		t.Error("runtime needed (action, filter, loadmore)")
	}
}

func TestCodeCollectionEmptyFirstPageAndGenericPage(t *testing.T) {
	dyn := &fakeDyn{}
	q, _ := url.ParseQuery("shop_q=zzz&debug=1")
	out := renderDoc(t, shopDoc, Context{Query: q, Path: "/shop", Locale: "th"}, dyn)
	h := out.HTML
	must(t, h, `<p class="n-empty">Nothing here</p>`, "SECRET", `value="zzz"`)
	mustNot(t, h, "n-card", `class="n-cartBox"`)
	// one item page → every pagination is hidden, no head links
	must(t, h, `data-rea-pagination="list" data-rea-style="numbers" aria-label="เลขหน้า" hidden></nav>`,
		`data-rea-style="loadmore" aria-label="เลขหน้า" hidden></nav>`)
	if out.Head != "" {
		t.Errorf("head links on a single page: %q", out.Head)
	}
	if dyn.reqs[0].Params["q"] != "zzz" {
		t.Errorf("filter param from the URL: %+v", dyn.reqs[0].Params)
	}
	// ?page= is used when the collection is the only paginated one
	const one = `{"id":"root","type":"frame","children":[
	 {"id":"c1","type":"collection","props":{"source":"code:products","perPage":2},"children":[{"id":"t","type":"text","props":{"text":"{{item.name}}"}}]},
	 {"id":"p1","type":"pagination","props":{"collection":"c1"}}]}`
	q, _ = url.ParseQuery("page=3")
	out = renderDoc(t, one, Context{Query: q, Path: "/x"}, &fakeDyn{})
	must(t, out.HTML, "P&lt;5&gt;", `data-rea-key="c1"`, `<a class="rea-pg-item rea-pg-prev" href="/x?c1_page=2" rel="prev">`,
		`<a class="rea-pg-item rea-pg-next is-disabled" aria-disabled="true">ถัดไป</a>`)
	mustNot(t, out.HTML, "page=3&")
}

func TestTableSourceFormsAndDenied(t *testing.T) {
	dyn := &fakeDyn{}
	const doc = `{"id":"root","type":"frame","children":[
	 {"id":"f","type":"frame","props":{"form":true},"children":[
	   {"id":"em","type":"input","props":{"kind":"email","name":"email","required":true,"value":"post.mail"}},
	   {"id":"sel","type":"input","props":{"kind":"select","name":"size","placeholder":"Size","options":[{"value":"s","label":"Small"},{"value":"m","label":"M"}],"value":"'m'"}},
	   {"id":"ck","type":"input","props":{"kind":"checkbox","name":"agree","label":"I agree <b>"}},
	   {"id":"ta","type":"input","props":{"kind":"textarea","name":"msg","value":"'<hi>'"}},
	   {"id":"hd","type":"input","props":{"kind":"hidden","name":"ref","value":"post.id"}},
	   {"id":"send","type":"frame","props":{"action":{"type":"code","name":"contact"}},"children":[{"id":"st","type":"text","props":{"text":"Send"}}]}
	 ]},
	 {"id":"orders","type":"collection","props":{"source":"table:orders","table":{"columns":["code"],"where":[{"column":"status","op":"=","value":"query.s"}],"orderBy":[{"column":"id","dir":"desc"}]}},
	  "children":[{"id":"oc","type":"text","props":{"text":"{{item.code}}"}}]},
	 {"id":"sec","type":"collection","props":{"source":"code:secret","emptyText":"none"},"children":[{"id":"x","type":"text","props":{"text":"{{item.a}}"}}]}
	]}`
	q, _ := url.ParseQuery("s=paid")
	out := renderDoc(t, doc, Context{Query: q, Post: PostCtx{"id": "9", "mail": "a@b.co"}}, dyn)
	h := out.HTML
	must(t, h, `<form class="n-f" data-rea-form onsubmit="return false">`,
		`<div class="n-em rea-field"><input class="rea-field-el" type="email" name="email" required value="a@b.co"><span class="rea-error" data-rea-error-for="email"></span></div>`,
		`<div class="n-sel rea-field"><select class="rea-field-el" name="size"><option value="">Size</option><option value="s">Small</option><option value="m" selected>M</option></select>`,
		`<label class="n-ck rea-field rea-check"><input class="rea-field-el" type="checkbox" value="1" name="agree"><span>I agree &lt;b&gt;</span><span class="rea-error" data-rea-error-for="agree"></span></label>`,
		`<div class="n-ta rea-field"><textarea class="rea-field-el" name="msg">&lt;hi&gt;</textarea>`,
		`<div class="n-hd rea-field" hidden><input class="rea-field-el" type="hidden" name="ref" value="9"></div>`,
		`<div data-rea-action="contact" data-rea-input="{}" class="n-send">`,
		`>A1</p>`, `<p class="rea-empty">none</p>`)
	mustNot(t, h, `data-rea-error-for="ref"`)
	var tbl *TableSpec
	for _, r := range dyn.reqs {
		if r.Source == "table:orders" {
			tbl = r.Table
		}
	}
	if tbl == nil || tbl.Where[0].Value != "paid" || tbl.Columns[0] != "code" || tbl.OrderBy[0].Dir != "desc" {
		t.Fatalf("table spec: %+v", tbl)
	}
}

func TestFragments(t *testing.T) {
	root, err := Parse([]byte(shopDoc))
	if err != nil {
		t.Fatal(err)
	}
	dyn := &fakeDyn{}
	q, _ := url.ParseQuery("shop_page=3")
	out := RenderFragments(root, Context{Query: q, Path: "/shop", Page: map[string]any{"total": 5.0}}, dyn,
		FragmentReq{IDs: []string{"list"}, Names: []string{}})
	if len(out.Fragments) != 4 { // the collection and its 3 pagination nodes
		t.Fatalf("fragments: %v", keys(out.Fragments))
	}
	must(t, out.Fragments["list"], `<div class="n-list" data-rea-collection="list" data-rea-key="shop" data-rea-source="code:products">`, "P&lt;5&gt;")
	must(t, out.Fragments["pg1"], `aria-current="page">3</a>`)
	must(t, out.Fragments["pg3"], `hidden></nav>`) // last page: no "load more"
	if len(dyn.reqs) != 1 {
		t.Errorf("only the requested collection loads: %d", len(dyn.reqs))
	}
	// by source and by layer name (action refresh / update)
	root.Children[5].Name = "cart"
	out = RenderFragments(root, Context{Page: map[string]any{"total": 5.0}}, &fakeDyn{}, FragmentReq{Sources: []string{"code:products"}, Names: []string{"cart"}})
	must(t, out.Fragments["cartBox"], "Total 5")
	if out.Fragments["list"] == "" || out.Fragments["pg2"] == "" {
		t.Errorf("refresh by source: %v", keys(out.Fragments))
	}
	// a hidden node renders nothing
	out = RenderFragments(root, Context{}, &fakeDyn{}, FragmentReq{IDs: []string{"cartBox"}})
	if _, ok := out.Fragments["cartBox"]; ok {
		t.Errorf("hidden node returned: %v", out.Fragments)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Existing {{post.x}} / {{site.x}} / {{item.x}} bindings render exactly as before.
func TestLegacyBindingsUnchanged(t *testing.T) {
	const doc = `{"id":"root","type":"frame","children":[
	 {"id":"a","type":"text","props":{"text":"{{post.title}} | {{ site.title }} | {{post.missing}} | {{item.x}} | {{ nope }}","href":"{{post.url}}"}},
	 {"id":"r","type":"text","props":{"rich":"<b>{{post.title}}</b> {{ post.title == &#39;A &amp; B&#39; ? &#39;yes&#39; : &#39;no&#39; }}"}},
	 {"id":"i","type":"image","props":{"src":"{{post.image}}","alt":"{{post.title}}"}}
	]}`
	out := renderDoc(t, doc, Context{Site: map[string]string{"title": "S<"}, Post: PostCtx{"title": "A & B", "url": "/x", "image": "/i.png"}}, nil)
	must(t, out.HTML, `<p class="n-a"><a href="/x" style="color:inherit;text-decoration:inherit">A &amp; B | S&lt; |  |  | </a></p>`,
		`<b>A &amp; B</b> yes`, `<img class="n-i" src="/i.png" alt="A &amp; B" loading="lazy">`)
	if out.Runtime {
		t.Error("plain pages do not need the runtime")
	}
	if strings.Contains(out.CSS, "rea-pagination") || strings.Contains(out.CSS, "rea-toast") {
		t.Error("plain pages get no Site Code CSS")
	}
}

func TestComponentPaginationPrefix(t *testing.T) {
	comp := &Node{ID: "croot", Type: TypeFrame, Children: []*Node{
		{ID: "cl", Type: TypeCollection, Props: map[string]any{"source": "code:products", "perPage": 2.0},
			Children: []*Node{{ID: "ct", Type: TypeText, Props: map[string]any{"text": "{{item.name}}"}}}},
		{ID: "cp", Type: TypePagination, Props: map[string]any{"collection": "cl"}},
	}}
	data := &compDyn{fakeDyn: &fakeDyn{}, comp: comp}
	root := &Node{ID: "root", Type: TypeFrame, Children: []*Node{{ID: "inst", Type: TypeInstance, Props: map[string]any{"componentId": 7.0}}}}
	out := Render(root, Context{Path: "/p"}, data)
	must(t, out.HTML, `data-rea-collection="inst-cl"`, `data-rea-pagination="inst-cl"`, `href="/p?instcl_page=2"`)
}

type compDyn struct {
	*fakeDyn
	comp *Node
}

func (c *compDyn) Component(id int64) *Node {
	if id == 7 {
		return c.comp
	}
	return nil
}
