package design

import (
	"strings"
	"testing"
)

func TestEffectsCSS(t *testing.T) {
	n := &Node{ID: "card", Type: "frame", Width: Size{Mode: "fixed", Value: 300}, Height: Size{Mode: "hug"},
		Layout: map[string]any{"mode": "vertical"},
		Style: map[string]any{
			"fill":   "linear-gradient(135deg,#4f46e5 0%,#c026d3 100%)",
			"radii":  []any{float64(24), float64(24), float64(0), float64(0)},
			"stroke": "#ffffff", "strokeWidth": float64(2), "strokeStyle": "dashed",
			"blur": float64(2), "grayscale": float64(100), "brightness": float64(120),
			"backdropBlur": float64(12),
			"italic":       true, "textDecoration": "underline", "textShadow": "0px 2px 6px rgba(0,0,0,0.25)",
			"rotation": float64(-3),
			"hover":    map[string]any{"scale": 1.05, "lift": float64(6), "shadow": "0px 20px 40px 0px rgba(17,24,39,0.22)", "duration": float64(300)},
			"anim":     map[string]any{"type": "up", "delay": float64(200), "duration": float64(900)},
		}}
	css := map[string]string{}
	for _, d := range Decls(n, "vertical") {
		css[d.Prop] = d.Value
	}
	want := map[string]string{
		"background":      "linear-gradient(135deg,#4f46e5 0%,#c026d3 100%)",
		"border-radius":   "24px 24px 0px 0px",
		"border":          "2px dashed #ffffff",
		"filter":          "blur(2px) grayscale(100%) brightness(120%)",
		"backdrop-filter": "blur(12px)",
		"font-style":      "italic",
		"text-decoration": "underline",
		"text-shadow":     "0px 2px 6px rgba(0,0,0,0.25)",
		"--anim-dur":      "900ms",
		"--anim-delay":    "200ms",
	}
	for k, v := range want {
		if css[k] != v {
			t.Errorf("%s = %q, want %q", k, css[k], v)
		}
	}
	if !strings.HasPrefix(css["transition"], "transform 300ms ease") {
		t.Errorf("transition = %q", css["transition"])
	}
	hover := map[string]string{}
	for _, d := range HoverDecls(n) {
		hover[d.Prop] = d.Value
	}
	if hover["transform"] != "rotate(-3deg) translateY(-6px) scale(1.05)" || hover["box-shadow"] == "" {
		t.Errorf("hover = %v", hover)
	}
	if AnimClasses(n) != " rea-anim rea-anim-up" {
		t.Errorf("anim classes = %q", AnimClasses(n))
	}

	// rendered page: hover rule, animation classes and the shared animation CSS
	out := Render(&Node{ID: "root", Type: "frame", Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Layout: map[string]any{"mode": "vertical"}, Children: []*Node{n}}, Context{}, nil)
	for _, s := range []string{".n-card:hover{transform:rotate(-3deg) translateY(-6px) scale(1.05)", ".rea-js .rea-anim{"} {
		if !strings.Contains(out.CSS, s) {
			t.Errorf("CSS misses %q", s)
		}
	}
	if !strings.Contains(out.HTML, `class="n-card rea-anim rea-anim-up"`) || !out.Anim {
		t.Errorf("HTML / Anim: %s %v", out.HTML, out.Anim)
	}

	// text gradient and image overlay; unsafe values are cleaned
	txt := &Node{ID: "t", Type: "text", Width: Size{Mode: "hug"}, Height: Size{Mode: "hug"},
		Style: map[string]any{"textGradient": "linear-gradient(90deg,#f00 0%,#00f 100%)", "textShadow": "1px 1px red;}body{display:none"}}
	css = map[string]string{}
	for _, d := range Decls(txt, "vertical") {
		css[d.Prop] = d.Value
	}
	if css["background-clip"] != "text" || css["-webkit-text-fill-color"] != "transparent" || strings.ContainsAny(css["text-shadow"], ";{}") {
		t.Errorf("text css = %v", css)
	}
	img := &Node{ID: "h", Type: "frame", Width: Size{Mode: "fill"}, Height: Size{Mode: "fixed", Value: 400},
		Style: map[string]any{"backgroundImage": "/uploads/a.jpg", "overlay": "rgba(0,0,0,0.45)"}}
	for _, d := range Decls(img, "vertical") {
		if d.Prop == "background-image" && d.Value != "linear-gradient(rgba(0,0,0,0.45),rgba(0,0,0,0.45)),url('/uploads/a.jpg')" {
			t.Errorf("overlay = %q", d.Value)
		}
	}
	if (&Node{ID: "x", Style: map[string]any{"anim": map[string]any{"type": "evil\"><script>"}}}).Style != nil && AnimClasses(&Node{Style: map[string]any{"anim": map[string]any{"type": "x\"y"}}}) != "" {
		t.Error("unknown animation types must be ignored")
	}
}

func TestIconsAndShapes(t *testing.T) {
	icon := &Node{ID: "ic", Type: TypeIcon, Width: Size{Mode: "fixed", Value: 32}, Height: Size{Mode: "fixed", Value: 32},
		Props: map[string]any{"icon": "heart", "strokeWidth": float64(1.5), "label": `Love "it"`}, Style: map[string]any{"color": "#e11d48"}}
	shape := &Node{ID: "sh", Type: TypeShape, Width: Size{Mode: "fill"}, Height: Size{Mode: "fixed", Value: 80},
		Props: map[string]any{"shape": "wave", "flipY": true}, Style: map[string]any{"fill": "#111827"}}
	bad := &Node{ID: "bd", Type: TypeIcon, Width: Size{Mode: "fixed", Value: 24}, Height: Size{Mode: "fixed", Value: 24},
		Props: map[string]any{"icon": `"><script>alert(1)</script>`}}
	out := Render(&Node{ID: "root", Type: "frame", Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Layout: map[string]any{"mode": "vertical"}, Children: []*Node{icon, shape, bad}}, Context{}, nil)
	for _, s := range []string{`<svg class="n-ic" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"`,
		`role="img" aria-label="Love &#34;it&#34;"`, `preserveAspectRatio="none"`, ".n-sh{", "color:#111827", "transform:scale(1,-1)"} {
		if !strings.Contains(out.HTML+out.CSS, s) {
			t.Errorf("missing %q in\n%s\n%s", s, out.HTML, out.CSS)
		}
	}
	if strings.Contains(out.HTML, "<script>") {
		t.Fatal("icon names must never reach the markup")
	}
	if len(Icons) < 60 || len(Shapes) < 10 {
		t.Errorf("icon set %d, shapes %d", len(Icons), len(Shapes))
	}
}

// partsData serves one component and one plugin block for the tests below.
type partsData struct{ fakeData }

func (partsData) Component(int64) *Node {
	return &Node{ID: "card", Type: "frame", Width: Size{Mode: "fixed", Value: 300}, Height: Size{Mode: "hug"},
		Layout: map[string]any{"mode": "vertical"}, Style: map[string]any{"fill": "#ffffff", "radius": float64(8)},
		Children: []*Node{
			{ID: "title", Type: "text", Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Props: map[string]any{"text": "Original"}, Style: map[string]any{"color": "#111111"}},
			{ID: "note", Type: "text", Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Props: map[string]any{"text": "Small print"}},
		}}
}
func (partsData) PluginBlock(c BlockCall) (string, string, bool) {
	return `<div class="pt"><h3 class="pt-name">Pro</h3></div>`, ".pb-p-b .pt-name{color:#000}", true
}

func TestInstanceOverridesAndBlockParts(t *testing.T) {
	root := &Node{ID: "root", Type: "frame", Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Layout: map[string]any{"mode": "vertical"},
		Children: []*Node{
			{ID: "a", Type: TypeInstance, Width: Size{Mode: "fixed", Value: 300}, Height: Size{Mode: "hug"},
				Props: map[string]any{"componentId": float64(1), "overrides": map[string]any{
					"title": map[string]any{"text": "Custom", "style": map[string]any{"color": "#ff0000"}},
					"note":  map[string]any{"hidden": true}}},
				Style: map[string]any{"fill": "#000000"}},
			{ID: "b", Type: TypeInstance, Width: Size{Mode: "fixed", Value: 300}, Height: Size{Mode: "hug"}, Props: map[string]any{"componentId": float64(1)}},
			{ID: "pb", Type: TypePluginBlock, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
				Props: map[string]any{"plugin": "p", "block": "b", "parts": map[string]any{
					"pt-name": map[string]any{"color": "#e11d48", "fontSize": float64(28)},
					"x;}evil": map[string]any{"color": "red"}}}},
		}}
	out := Render(root, Context{}, partsData{})
	for _, s := range []string{
		">Custom<", ">Original<", // each placement keeps its own text
		".n-a-title{", "color:#ff0000", ".n-b-title{", // unique inner classes per placement
		".n-a{", "background:#000000", // the placement's own fill
		".n-pb.n-pb .pt-name{color:#e11d48;font-size:28px}",
	} {
		if !strings.Contains(out.HTML+out.CSS, s) {
			t.Errorf("missing %q", s)
		}
	}
	if strings.Contains(out.CSS, "evil") {
		t.Error("part keys must be plain class names")
	}
	if !strings.Contains(out.CSS, ".n-a-note{") || !strings.Contains(out.CSS, "display:none") {
		t.Error("hidden override must hide the layer")
	}
}

func TestMaskAndFocal(t *testing.T) {
	n := &Node{ID: "m", Type: TypeImage, Width: Size{Mode: "fixed", Value: 200}, Height: Size{Mode: "fixed", Value: 200},
		Style: map[string]any{"mask": "blob1", "focalX": 20.0, "focalY": 120.0}, Props: map[string]any{"src": "/a.jpg"}}
	css := Render(n, Context{}, nil).CSS
	for _, want := range []string{"mask:url(\"data:image/svg+xml,", "-webkit-mask:", "object-position:20% 100%"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q in %s", want, css)
		}
	}
	if strings.Contains(css, "</") || strings.Contains(css[strings.Index(css, "url(\"")+5:], "\"><") {
		t.Errorf("mask svg must be encoded: %s", css)
	}
	n.Style = map[string]any{"mask": "nope"}
	if strings.Contains(Render(n, Context{}, nil).CSS, "mask") {
		t.Error("unknown masks are ignored")
	}
}

func TestBlockPartExtras(t *testing.T) {
	n := &Node{ID: "pb", Type: TypePluginBlock, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Props: map[string]any{"plugin": "p", "block": "b", "parts": map[string]any{"pt-btn": map[string]any{
			"fontFamily": "Kanit", "textAlign": "center", "padY": 10.0, "padX": 24.0, "stroke": "#111111", "strokeWidth": 2.0,
			"shadow": "0 4px 12px rgba(0,0,0,.2)", "opacity": 0.9, "textTransform": "uppercase",
			"hover": map[string]any{"fill": "#4f46e5", "color": "#ffffff"}}}}}
	css := Render(n, Context{}, partsData{}).CSS
	for _, want := range []string{"font-family:'Kanit', sans-serif", "text-align:center", "padding:10px 24px", "border:2px solid #111111",
		"opacity:0.9", "text-transform:uppercase", ".n-pb.n-pb .pt-btn:hover{color:#ffffff;background:#4f46e5}"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q in %s", want, css)
		}
	}
}
