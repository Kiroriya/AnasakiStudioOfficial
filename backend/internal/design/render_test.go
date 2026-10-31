package design

import (
	"strings"
	"testing"
)

type fakeData struct{}

func (fakeData) QueryPosts(q Query) []PostCtx {
	return []PostCtx{{"title": "First <post>", "url": "/blog/first"}, {"title": "Second", "url": "/blog/second"}}
}
func (fakeData) MenuItems(int64) []MenuItem {
	return []MenuItem{{"Home", "/"}, {"Bad", "javascript:alert(1)"}}
}
func (fakeData) Component(id int64) *Node {
	if id != 7 {
		return nil
	}
	return &Node{ID: "comp", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Layout:     map[string]any{"mode": "horizontal"},
		Responsive: map[string]map[string]any{"mobile": {"layout": map[string]any{"mode": "vertical"}}},
		Children: []*Node{{ID: "comp-t", Type: TypeText, Width: Size{Mode: "hug"}, Height: Size{Mode: "hug"},
			Props: map[string]any{"text": "{{site.title}}"}}}}
}

const doc = `{
 "id":"root","type":"frame","width":{"mode":"fill"},"height":{"mode":"hug"},
 "layout":{"mode":"vertical","gap":16,"padding":[10,20,10,20]},
 "children":[
  {"id":"hdr","type":"instance","width":{"mode":"fill"},"height":{"mode":"hug"},"props":{"componentId":7}},
  {"id":"row","type":"frame","width":{"mode":"fill"},"height":{"mode":"hug"},
   "layout":{"mode":"horizontal","gap":24},
   "responsive":{"mobile":{"layout":{"mode":"vertical"}}},
   "children":[
     {"id":"a","type":"rect","width":{"mode":"fill"},"height":{"mode":"fixed","value":40}},
     {"id":"b","type":"rect","width":{"mode":"fixed","value":200},"height":{"mode":"fixed","value":40},
      "responsive":{"tablet":{"visible":false}}}
   ]},
  {"id":"free","type":"frame","width":{"mode":"fill"},"height":{"mode":"fixed","value":300},"layout":{"mode":"none"},
   "children":[{"id":"abs","type":"text","x":12,"y":34,"width":{"mode":"hug"},"height":{"mode":"hug"},
     "props":{"text":"Hi {{post.title}}","href":"javascript:evil()"}}]},
  {"id":"loop","type":"postList","width":{"mode":"fill"},"height":{"mode":"hug"},"layout":{"mode":"vertical"},
   "children":[{"id":"item","type":"text","width":{"mode":"fill"},"height":{"mode":"hug"},"props":{"text":"{{post.title}}","href":"{{post.url}}"}}]},
  {"id":"nav","type":"menu","width":{"mode":"hug"},"height":{"mode":"hug"},"props":{"menuId":1}}
 ]}`

func TestRender(t *testing.T) {
	root, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	out := Render(root, Context{Site: map[string]string{"title": "My Site"}, Post: PostCtx{"title": "Ctx"}}, fakeData{})

	mustContain := func(s, sub string) {
		t.Helper()
		if !strings.Contains(s, sub) {
			t.Errorf("expected %q in:\n%s", sub, s)
		}
	}
	mustNot := func(s, sub string) {
		t.Helper()
		if strings.Contains(s, sub) {
			t.Errorf("did not expect %q in:\n%s", sub, s)
		}
	}

	// component instance renders with the instance's class and bound site title
	mustContain(out.HTML, `class="n-hdr"`)
	mustContain(out.HTML, "My Site")
	// component breakpoint overrides apply to instances
	mustContain(out.CSS, ".n-hdr{flex-direction:column}")
	// auto layout → flexbox
	mustContain(out.CSS, ".n-row{position:relative;align-self:stretch;display:flex;flex-direction:row;gap:24px}")
	// fill inside a horizontal parent → flex-grow
	mustContain(out.CSS, ".n-a{position:relative;flex-grow:1;flex-basis:0;min-width:0;height:40px}")
	// freeform child → absolute
	mustContain(out.CSS, ".n-abs{position:absolute;left:12px;top:34px;width:max-content")
	// breakpoints: row becomes a column on mobile, b hidden from tablet down
	mustContain(out.CSS, "@media (max-width:640px)")
	mustContain(out.CSS, ".n-row{flex-direction:column}")
	mustContain(out.CSS, ".n-b{display:none}")
	// child of a column on mobile: fill → align-self:stretch, flex-grow unset
	mustContain(out.CSS, ".n-a{align-self:stretch;flex-shrink:0;flex-grow:unset;flex-basis:unset;min-width:unset}")
	// query loop repeats the template per post, escaping content
	mustContain(out.HTML, `href="/blog/first"`)
	mustContain(out.HTML, "First &lt;post&gt;")
	mustContain(out.HTML, "Second")
	// bindings use the page context outside loops
	mustContain(out.HTML, "Hi Ctx")
	// dangerous links neutralised
	mustNot(out.HTML, "javascript:")
	// CSS for repeated nodes is emitted once
	if strings.Count(out.CSS, ".n-item{") != 1 {
		t.Errorf("expected one .n-item rule, css:\n%s", out.CSS)
	}
}

func TestAbsoluteInAutoLayoutAndRotation(t *testing.T) {
	n := &Node{ID: "badge", Type: TypeRect, Absolute: true, X: 10, Y: 20,
		Width: Size{Mode: "fixed", Value: 40}, Height: Size{Mode: "fixed", Value: 40},
		Style: map[string]any{"rotation": 45.0}}
	got := map[string]string{}
	for _, d := range Decls(n, "horizontal") {
		got[d.Prop] = d.Value
	}
	if got["position"] != "absolute" || got["left"] != "10px" || got["top"] != "20px" {
		t.Errorf("absolute child in auto layout: %v", got)
	}
	if _, ok := got["flex-shrink"]; ok {
		t.Errorf("absolute child must not get flex rules: %v", got)
	}
	if got["transform"] != "rotate(45deg)" {
		t.Errorf("rotation: %v", got)
	}
}

func (fakeData) Collection(source string, q map[string]any) []PostCtx {
	if source != "products" {
		return nil
	}
	return []PostCtx{
		{"id": "11", "title": "Mug <XL>", "price": "฿290", "url": "/product/mug"},
		{"id": "12", "title": "Tee", "price": "฿590", "url": "/product/tee"},
	}
}

func TestCollectionNodes(t *testing.T) {
	const d = `{"id":"root","type":"frame","width":{"mode":"fill"},"height":{"mode":"hug"},"layout":{"mode":"vertical"},
	 "style":{"minHeight":480},
	 "children":[
	  {"id":"shop","type":"collection","width":{"mode":"fill"},"height":{"mode":"hug"},"layout":{"mode":"horizontal","wrap":true},
	   "props":{"source":"products","query":{"limit":4,"order":"price_asc"}},
	   "children":[{"id":"card","type":"frame","width":{"mode":"fixed","value":200},"height":{"mode":"hug"},"layout":{"mode":"vertical"},
	     "children":[
	      {"id":"name","type":"text","width":{"mode":"fill"},"height":{"mode":"hug"},"props":{"text":"{{item.title}} @ {{post.title}}","href":"{{item.url}}"}},
	      {"id":"buy","type":"button","width":{"mode":"hug"},"height":{"mode":"hug"},"props":{"text":"#{{item.id}}","href":"{{item.url}}"}}
	     ]}]},
	  {"id":"none","type":"collection","width":{"mode":"fill"},"height":{"mode":"hug"},"props":{"source":"nope","emptyText":"ไม่มีสินค้า"}},
	  {"id":"outside","type":"text","width":{"mode":"hug"},"height":{"mode":"hug"},"props":{"text":"[{{item.title}}]"}}
	 ]}`
	root, err := Parse([]byte(d))
	if err != nil {
		t.Fatal(err)
	}
	out := Render(root, Context{Post: PostCtx{"id": "5", "title": "Shop"}}, fakeData{})
	for _, want := range []string{
		`Mug &lt;XL&gt; @ Shop`, `href="/product/tee"`,
		`<a class="n-buy rea-btn" href="/product/mug">#11</a>`,
		`#12</a>`,
		`<p class="rea-empty">ไม่มีสินค้า</p>`,
		`<p class="n-outside">[]</p>`,
	} {
		if !strings.Contains(out.HTML, want) {
			t.Errorf("expected %q in:\n%s", want, out.HTML)
		}
	}
	if strings.Count(out.CSS, ".n-card{") != 1 {
		t.Errorf("expected one .n-card rule, css:\n%s", out.CSS)
	}
	if !strings.Contains(out.CSS, "min-height:480px") {
		t.Errorf("minHeight not emitted, css:\n%s", out.CSS)
	}
}

func (fakeData) Media(id int64) *MediaInfo {
	if id != 3 {
		return nil
	}
	return &MediaInfo{ID: 3, URL: "/uploads/guide.pdf", Filename: "guide.pdf", Mime: "application/pdf", Size: 1258291}
}

func (fakeData) PluginBlock(c BlockCall) (string, string, bool) {
	if c.Plugin != "hello" || c.Block != "card" {
		return "", "", false
	}
	return "<b>" + c.Values["text"].(string) + "|" + c.Post["title"] + "|" + c.Item["title"] + "</b>",
		".pb-hello-card b{color:red}", true
}

func TestDownloadAndPluginBlock(t *testing.T) {
	const d = `{"id":"root","type":"frame","width":{"mode":"fill"},"height":{"mode":"hug"},"layout":{"mode":"vertical"},
	 "children":[
	  {"id":"dl","type":"download","width":{"mode":"hug"},"height":{"mode":"hug"},"props":{"mediaId":3,"text":"Get <it>","showSize":true}},
	  {"id":"dl2","type":"download","width":{"mode":"hug"},"height":{"mode":"hug"},"props":{"mediaId":3}},
	  {"id":"dl3","type":"download","width":{"mode":"hug"},"height":{"mode":"hug"},"props":{"mediaId":99,"text":"x"}},
	  {"id":"pb","type":"pluginBlock","width":{"mode":"fill"},"height":{"mode":"hug"},"props":{"plugin":"hello","block":"card","values":{"text":"Hi"}}},
	  {"id":"pb2","type":"pluginBlock","width":{"mode":"fill"},"height":{"mode":"hug"},"props":{"plugin":"hello","block":"card","values":{"text":"Again"}}},
	  {"id":"pb3","type":"pluginBlock","width":{"mode":"fill"},"height":{"mode":"hug"},"props":{"plugin":"gone","block":"x"}},
	  {"id":"loop","type":"collection","width":{"mode":"fill"},"height":{"mode":"hug"},"props":{"source":"products"},
	   "children":[{"id":"pbi","type":"pluginBlock","width":{"mode":"fill"},"height":{"mode":"hug"},"props":{"plugin":"hello","block":"card","values":{"text":"In"}}}]}
	 ]}`
	root, err := Parse([]byte(d))
	if err != nil {
		t.Fatal(err)
	}
	out := Render(root, Context{Post: PostCtx{"title": "Page"}}, fakeData{})
	for _, want := range []string{
		`<a class="n-dl rea-btn rea-download" href="/download/3" download>Get &lt;it&gt; <span class="rea-file-meta">PDF · 1.2 MB</span></a>`,
		`<a class="n-dl2 rea-btn rea-download" href="/download/3" download>guide.pdf</a>`,
		`<a class="n-dl3 rea-btn rea-download" href="#" download>x</a>`,
		`<div class="n-pb pb-hello-card"><b>Hi|Page|</b></div>`,
		`<div class="n-pb2 pb-hello-card"><b>Again|Page|</b></div>`,
		`<div class="n-pb3 pb-gone-x"></div>`,
		`<div class="n-pbi pb-hello-card"><b>In|Page|Mug <XL></b></div>`,
	} {
		if !strings.Contains(out.HTML, want) {
			t.Errorf("expected %q in:\n%s", want, out.HTML)
		}
	}
	if strings.Count(out.CSS, ".pb-hello-card b{color:red}") != 1 {
		t.Errorf("block CSS should be emitted once:\n%s", out.CSS)
	}
	if !strings.Contains(out.CSS, ".n-dl{position:relative;width:fit-content;display:inline-flex;align-items:center;justify-content:center;text-decoration:none;cursor:pointer}") {
		t.Errorf("download should be styled like a button:\n%s", out.CSS)
	}
	if HumanSize(512) != "512 B" || HumanSize(12*1024) != "12 KB" || HumanSize(3*1024*1024*1024) != "3.0 GB" {
		t.Errorf("HumanSize: %s %s %s", HumanSize(512), HumanSize(12*1024), HumanSize(3*1024*1024*1024))
	}
}

func TestBoundImageWithoutValueIsOmitted(t *testing.T) {
	root := &Node{ID: "root", Type: TypeFrame, Children: []*Node{
		{ID: "cover", Type: TypeImage, Props: map[string]any{"src": "{{post.image}}"}},
		{ID: "ph", Type: TypeImage},
	}}
	out := Render(root, Context{Post: PostCtx{"title": "x"}}, nil)
	if strings.Contains(out.HTML, ClassName("cover")+`"`) {
		t.Fatalf("bound image without a value must not render: %s", out.HTML)
	}
	if !strings.Contains(out.HTML, ClassName("ph")) {
		t.Fatal("unbound empty image keeps its placeholder")
	}
}
