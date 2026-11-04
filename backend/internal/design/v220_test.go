package design

import (
	"net/url"
	"strings"
	"testing"
)

// CMS 2.2.0: the designer features from the Qraft feedback.

// queryRec records the post queries of a render.
type queryRec struct {
	fakeData
	got []Query
}

func (q *queryRec) QueryPosts(x Query) []PostCtx {
	q.got = append(q.got, x)
	return q.fakeData.QueryPosts(x)
}

func node(id, typ string, props map[string]any, style map[string]any, kids ...*Node) *Node {
	return &Node{ID: id, Type: typ, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Props: props, Style: style, Children: kids}
}

func TestRefAttr(t *testing.T) {
	root := node("root", TypeFrame, nil, nil,
		node("dl", TypeButton, map[string]any{"text": "Download", "ref": "download"}, nil),
		node("bad", TypeText, map[string]any{"text": "x", "ref": "Bad name\"><script>"}, nil))
	out := Render(root, Context{}, fakeData{})
	if !strings.Contains(out.HTML, `<a data-ref="download" class="n-dl rea-btn"`) {
		t.Fatalf("data-ref missing: %s", out.HTML)
	}
	if strings.Contains(out.HTML, "Bad name") || strings.Contains(out.HTML, "<script>") {
		t.Fatalf("invalid ref rendered: %s", out.HTML)
	}
	if !strings.Contains(RuntimeScript(), "data-ref") {
		t.Error("runtime has no anasaki.ref")
	}
}

func TestMenuCurrentPage(t *testing.T) {
	data := menuData{items: []MenuItem{{"Home", "/"}, {"Gen QR", "/qr/"}, {"เกี่ยวกับ", "/%E0%B9%80%E0%B8%81%E0%B8%B5%E0%B9%88%E0%B8%A2%E0%B8%A7"}, {"Out", "https://x.io/qr"}}}
	m := node("m", TypeMenu, map[string]any{"menuId": 1.0}, map[string]any{
		"active": map[string]any{"fill": "#ff7a7a", "color": "#fff"}, "linkPadding": []any{6.0, 14.0}, "linkRadius": 999.0})
	out := Render(node("root", TypeFrame, nil, nil, m), Context{Path: "/qr"}, data)
	if !strings.Contains(out.HTML, `<a href="/qr/" aria-current="page">Gen QR</a>`) || strings.Count(out.HTML, "aria-current") != 1 {
		t.Fatalf("aria-current: %s", out.HTML)
	}
	if !strings.Contains(out.CSS, `.n-m a[aria-current="page"]{background:#ff7a7a;color:#fff}`) || !strings.Contains(out.CSS, ".n-m a{padding:6px 14px;border-radius:999px}") {
		t.Fatalf("menu css: %s", out.CSS)
	}
	if out := Render(node("root", TypeFrame, nil, nil, m), Context{Path: "/เกี่ยว"}, data); !strings.Contains(out.HTML, `aria-current="page">เกี่ยวกับ`) {
		t.Fatalf("encoded path: %s", out.HTML)
	}
	if !SamePage("/", "/") || SamePage("https://x.io/qr", "/qr") || !SamePage("/qr?x=1#a", "/qr/") {
		t.Error("SamePage")
	}
}

type menuData struct {
	fakeData
	items []MenuItem
}

func (m menuData) MenuItems(int64) []MenuItem { return m.items }

func TestButtonIcon(t *testing.T) {
	var name string
	for k := range Icons {
		name = k
		break
	}
	b := node("b", TypeButton, map[string]any{"text": "Download", "icon": name, "iconPosition": "end"}, nil)
	out := Render(node("root", TypeFrame, nil, nil, b), Context{}, fakeData{})
	if !strings.Contains(out.HTML, `<span>Download</span><svg class="rea-btn-ic"`) || !strings.Contains(out.HTML, `aria-hidden="true"`) {
		t.Fatalf("icon after text: %s", out.HTML)
	}
	b.Props["iconPosition"] = nil
	if out := Render(node("root", TypeFrame, nil, nil, b), Context{}, fakeData{}); !strings.Contains(out.HTML, `href="#"><svg class="rea-btn-ic"`) {
		t.Fatalf("icon before text: %s", out.HTML)
	}
	b.Props["icon"] = "no-such-icon"
	if out := Render(node("root", TypeFrame, nil, nil, b), Context{}, fakeData{}); strings.Contains(out.HTML, "rea-btn-ic") {
		t.Fatalf("unknown icon drawn: %s", out.HTML)
	}
}

func TestMinHeightScreen(t *testing.T) {
	hero := node("hero", TypeFrame, nil, map[string]any{"minHeightScreen": true, "minHeightOffset": 64.0})
	hero.Responsive = map[string]map[string]any{"mobile": {"style": map[string]any{"minHeightScreen": false}}}
	out := Render(node("root", TypeFrame, nil, nil, hero), Context{}, fakeData{})
	if !strings.Contains(out.CSS, "min-height:calc(var(--rea-screen-h,100svh) - 64px)") {
		t.Fatalf("screen height: %s", out.CSS)
	}
	if !strings.Contains(out.CSS, "@media (max-width:640px){\n.n-hero{min-height:") {
		t.Fatalf("mobile override does not unset it: %s", out.CSS)
	}
}

func TestMediaMax(t *testing.T) {
	row := node("row", TypeFrame, nil, nil)
	row.Responsive = map[string]map[string]any{"mobile": {"layout": map[string]any{"mode": "vertical"}}}
	out := Render(node("root", TypeFrame, nil, nil, row), Context{MediaMax: map[string]int{"mobile": 767}}, fakeData{})
	if !strings.Contains(out.CSS, "@media (max-width:767px)") || strings.Contains(out.CSS, "max-width:640px") {
		t.Fatalf("mobile breakpoint: %s", out.CSS)
	}
}

func TestPostListFilters(t *testing.T) {
	list := node("blog", TypePostList, map[string]any{"query": map[string]any{"limit": 9.0, "category": "news"}}, nil,
		node("card", TypeText, map[string]any{"text": "{{post.title}}"}, nil))
	search := node("q", TypeInput, map[string]any{"kind": "search", "filter": map[string]any{"collection": "blog", "param": "q"}}, nil)
	data := &queryRec{}
	out := Render(node("root", TypeFrame, nil, nil, search, list), Context{Query: url.Values{"blog_q": {"qr code"}, "blog_cat": {"tools"}}}, data)
	if len(data.got) != 1 || data.got[0].Search != "qr code" || data.got[0].Category != "tools" || data.got[0].Limit != 9 {
		t.Fatalf("query: %+v", data.got)
	}
	if !strings.Contains(out.HTML, `data-rea-collection="blog" data-rea-key="blog"`) || !strings.Contains(out.HTML, `name="blog_q"`) || !strings.Contains(out.HTML, `value="qr code"`) {
		t.Fatalf("filter markup: %s", out.HTML)
	}
	data.got = nil
	Render(node("root", TypeFrame, nil, nil, list), Context{Query: url.Values{"blog_cat": {"../x"}}}, data)
	if data.got[0].Category != "news" {
		t.Fatalf("bad category slug accepted: %+v", data.got)
	}
}

func TestInputValueText(t *testing.T) {
	in := node("u", TypeInput, map[string]any{"kind": "text", "name": "url", "value": "Bangkok", "valueText": true}, nil)
	out := Render(node("root", TypeFrame, nil, nil, in), Context{}, fakeData{})
	if !strings.Contains(out.HTML, `value="Bangkok"`) {
		t.Fatalf("text value: %s", out.HTML)
	}
}
