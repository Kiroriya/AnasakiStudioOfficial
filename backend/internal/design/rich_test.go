package design

import (
	"strings"
	"testing"
)

func TestSanitizeRich(t *testing.T) {
	cases := []struct{ in, want string }{
		{`Hello <b>bold</b> and <span style="color:var(--c-primary)">brand</span>`,
			`Hello <b>bold</b> and <span style="color:var(--c-primary)">brand</span>`},
		{`<script>alert(1)</script>x`, `alert(1)x`},
		{`<img src=x onerror=alert(1)>y`, `y`},
		{`<a href="javascript:alert(1)" onclick="x()">l</a>`, `<a href="#">l</a>`},
		{`<a href="/about" target="_blank">l</a>`, `<a href="/about" target="_blank" rel="noopener">l</a>`},
		{`<span style="color:red;position:fixed;background:url(x)">s</span>`, `<span style="color:red">s</span>`},
		{`<b><i>unclosed`, `<b><i>unclosed</i></b>`},
		{`stray </i> close`, `stray  close`},
		{`a < b<br/>c`, `a &lt; b<br>c`},
	}
	for _, c := range cases {
		if got := SanitizeRich(c.in); got != c.want {
			t.Errorf("SanitizeRich(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestRichTextBindingsEscaped(t *testing.T) {
	root := &Node{ID: "r", Type: TypeFrame, Width: Size{Mode: "fill"}, Children: []*Node{{
		ID: "t", Type: TypeText, Width: Size{Mode: "fill"},
		Props: map[string]any{"text": "plain", "rich": `<b>{{post.title}}</b>`},
	}}}
	out := Render(root, Context{Post: PostCtx{"title": `<script>x</script>`}}, nil)
	if !strings.Contains(out.HTML, `<b>&lt;script&gt;x&lt;/script&gt;</b>`) {
		t.Fatalf("binding not escaped: %s", out.HTML)
	}
}

func TestGridColumns(t *testing.T) {
	n := &Node{ID: "g", Type: TypeFrame, Width: Size{Mode: "fill"},
		Layout:     map[string]any{"mode": "horizontal", "columns": float64(3), "gap": float64(24)},
		Responsive: map[string]map[string]any{"mobile": {"layout": map[string]any{"columns": float64(1)}}}}
	out := Render(n, Context{}, nil)
	if !strings.Contains(out.CSS, "grid-template-columns:repeat(3,minmax(0,1fr))") || !strings.Contains(out.CSS, "display:grid") {
		t.Fatalf("no grid: %s", out.CSS)
	}
	if !strings.Contains(out.CSS, "grid-template-columns:repeat(1,minmax(0,1fr))") {
		t.Fatalf("mobile override missing: %s", out.CSS)
	}
}

func TestBreadcrumb(t *testing.T) {
	n := &Node{ID: "b", Type: TypeBreadcrumb, Width: Size{Mode: "fill"}, Props: map[string]any{"homeLabel": "Home"}}
	root := &Node{ID: "r", Type: TypeFrame, Width: Size{Mode: "fill"}, Children: []*Node{n}}
	post := Render(root, Context{Post: PostCtx{"type": "post", "title": "PrimePOS", "category": "Service", "categoryUrl": "/category/service"}}, nil).HTML
	for _, want := range []string{`<a href="/">Home</a>`, `<a href="/category/service">Service</a>`, `aria-current="page">PrimePOS</span>`} {
		if !strings.Contains(post, want) {
			t.Errorf("post breadcrumb missing %q: %s", want, post)
		}
	}
	home := Render(root, Context{Post: PostCtx{"type": "page", "title": "Home page", "isHome": "1"}}, nil).HTML
	if strings.Contains(home, "Home page") {
		t.Errorf("home page should show Home only: %s", home)
	}
}
