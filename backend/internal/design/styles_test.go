package design

import (
	"strings"
	"testing"
)

func TestTextStyles(t *testing.T) {
	styles := map[string]TextStyle{"h1": {Base: map[string]any{"fontSize": 52.0, "fontWeight": 800.0},
		Mobile: map[string]any{"fontSize": 34.0}}}
	root := &Node{ID: "r", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Layout: map[string]any{"mode": "vertical"},
		Children: []*Node{
			{ID: "a", Type: TypeText, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Style: map[string]any{"textStyle": "h1"}, Props: map[string]any{"text": "A"}},
			{ID: "b", Type: TypeText, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Style: map[string]any{"textStyle": "h1", "fontSize": 60.0}, Props: map[string]any{"text": "B"}},
			{ID: "c", Type: TypeText, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Style: map[string]any{"textStyle": "nope"}, Props: map[string]any{"text": "C"}},
		}}
	css := Render(root, Context{TextStyles: styles}, nil).CSS
	for _, want := range []string{".n-a{", "font-size:52px", "font-weight:800", "font-size:60px"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %q in %s", want, css)
		}
	}
	mobile := css[strings.Index(css, "@media (max-width:640px)"):]
	if !strings.Contains(mobile, ".n-a{font-size:34px}") {
		t.Errorf("phone size of the style not applied: %s", mobile)
	}
	if strings.Contains(mobile, ".n-b{font-size:34px}") {
		t.Errorf("the layer's own size must win on phones too: %s", mobile)
	}
}
