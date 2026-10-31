package design

import (
	"strings"
	"testing"
)

func vecNode(id string, style, props map[string]any) *Node {
	return &Node{ID: id, Type: TypeVector, Width: Size{Mode: "fixed", Value: 120}, Height: Size{Mode: "fixed", Value: 60},
		Style: style, Props: props}
}

func renderOne(n *Node) string {
	return Render(&Node{ID: "root", Type: "frame", Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Layout: map[string]any{"mode": "none"}, Children: []*Node{n}}, Context{}, nil).HTML
}

func TestVectorRender(t *testing.T) {
	n := vecNode("v1",
		map[string]any{"fill": "var(--c-primary)", "stroke": "#111827", "strokeWidth": float64(3), "strokeStyle": "dashed"},
		map[string]any{"path": "M0 0 C 40 -10, 80 70, 120 60 L 0 60 Z", "viewBox": "0 0 120 60",
			"strokeCap": "round", "strokeJoin": "bevel", "fillRule": "evenodd", "label": "Logo mark"})
	out := renderOne(n)
	for _, want := range []string{
		`<svg class="n-v1" viewBox="0 0 120 60" preserveAspectRatio="none" overflow="visible" role="img" aria-label="Logo mark">`,
		`<path d="M0 0 C 40 -10, 80 70, 120 60 L 0 60 Z" style="fill:var(--c-primary);stroke:#111827" stroke-width="3"`,
		`stroke-linecap="round"`, `stroke-linejoin="bevel"`, `stroke-dasharray="9 6"`, `fill-rule="evenodd"`,
		`vector-effect="non-scaling-stroke"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	// fill / stroke paint the path, not the box
	css := map[string]string{}
	for _, d := range Decls(n, "none") {
		css[d.Prop] = d.Value
	}
	if css["background"] != "" || css["border"] != "" {
		t.Errorf("vector box got background/border: %v", css)
	}
	if css["display"] != "block" || css["width"] != "120px" || css["height"] != "60px" {
		t.Errorf("vector box css = %v", css)
	}
}

func TestVectorDefaultsAndDecorative(t *testing.T) {
	out := renderOne(vecNode("v2", nil, map[string]any{"path": "M0 0L10 10"}))
	if !strings.Contains(out, `viewBox="0 0 120 60"`) || !strings.Contains(out, `aria-hidden="true"`) ||
		!strings.Contains(out, `style="fill:none;stroke:none"`) || strings.Contains(out, "stroke-width") {
		t.Errorf("defaults: %s", out)
	}
}

func TestVectorRejectsBadInput(t *testing.T) {
	cases := []map[string]any{
		{"path": `M0 0"/><script>alert(1)</script>`},
		{"path": "M0 0 url(javascript:alert(1))"},
		{"path": "L0 0 10 10"}, // must start with a move
		{"path": "M0 0 " + strings.Repeat("L1 1 ", MaxVectorPath/5+10)},
	}
	for i, p := range cases {
		out := renderOne(vecNode("bad", nil, p))
		if strings.Contains(out, "<path") || strings.Contains(out, "<script") || strings.Contains(out, "javascript") {
			t.Errorf("case %d: invalid path rendered: %.200s", i, out)
		}
		if !strings.Contains(out, `<svg class="n-bad"`) {
			t.Errorf("case %d: svg box missing", i)
		}
	}

	// paint and viewBox injection attempts
	n := vecNode("esc",
		map[string]any{"fill": `red" onload="alert(1)`, "stroke": "url(#x)", "strokeWidth": float64(2)},
		map[string]any{"path": "M0 0L1 1", "viewBox": `0 0 10 10" onload="x`, "strokeCap": `round" onclick="x`,
			"label": `<b>"Hi"</b>`})
	out := renderOne(n)
	for _, bad := range []string{"onload", "onclick", "url(#x)", "<b>"} {
		if strings.Contains(out, bad) {
			t.Errorf("unescaped %q in %s", bad, out)
		}
	}
	if !strings.Contains(out, `aria-label="&lt;b&gt;&#34;Hi&#34;&lt;/b&gt;"`) {
		t.Errorf("label not escaped: %s", out)
	}
	if !strings.Contains(out, `style="fill:none;stroke:none"`) {
		t.Errorf("invalid paints should fall back to none: %s", out)
	}
}

func TestVectorPaintAndPath(t *testing.T) {
	for _, ok := range []string{"#fff", "#11223344", "rgba(0, 0, 0, 0.5)", "hsl(200 50% 40%)", "currentColor", "var(--c-primary)", "none"} {
		if VectorPaint(ok) != ok {
			t.Errorf("paint %q rejected", ok)
		}
	}
	for _, bad := range []string{"linear-gradient(red,blue)", "red;x:y", "var(--a);", "url(#g)", `"`} {
		if VectorPaint(bad) != "" {
			t.Errorf("paint %q accepted", bad)
		}
	}
	if !ValidVectorPath("m1e2,2E-1 a5 5 0 1 0 10 0z") || ValidVectorPath("") || ValidVectorPath("M0 0 X") {
		t.Error("path validation")
	}
}
