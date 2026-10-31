package design

import (
	"strings"
	"testing"
)

type variantData struct{ fakeData }

func (variantData) Component(int64) *Node {
	return &Node{ID: "btn", Type: "frame", Width: Size{Mode: "hug"}, Height: Size{Mode: "hug"},
		Style: map[string]any{"fill": "#4f46e5"},
		Props: map[string]any{"variants": []any{map[string]any{"name": "Outline", "node": map[string]any{
			"id": "btn", "type": "frame", "width": map[string]any{"mode": "hug"}, "height": map[string]any{"mode": "hug"},
			"style":    map[string]any{"stroke": "#4f46e5", "strokeWidth": 2.0},
			"children": []any{map[string]any{"id": "label", "type": "text", "width": map[string]any{"mode": "hug"}, "height": map[string]any{"mode": "hug"}, "props": map[string]any{"text": "Outline text"}}},
		}}}},
		Children: []*Node{{ID: "label", Type: "text", Width: Size{Mode: "hug"}, Height: Size{Mode: "hug"}, Props: map[string]any{"text": "Main text"}}},
	}
}

func TestVariants(t *testing.T) {
	inst := func(id, v string) *Node {
		return &Node{ID: id, Type: TypeInstance, Width: Size{Mode: "hug"}, Height: Size{Mode: "hug"},
			Props: map[string]any{"componentId": 1.0, "variant": v, "overrides": map[string]any{"label": map[string]any{"style": map[string]any{"color": "#ff0000"}}}}}
	}
	root := &Node{ID: "r", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Layout: map[string]any{"mode": "vertical"},
		Children: []*Node{inst("a", ""), inst("b", "Outline"), inst("c", "Missing")}}
	out := Render(root, Context{}, variantData{})
	if strings.Count(out.HTML, "Main text") != 2 || strings.Count(out.HTML, "Outline text") != 1 {
		t.Fatalf("variant not chosen per placement: %s", out.HTML)
	}
	if !strings.Contains(out.CSS, ".n-b{") || !strings.Contains(out.CSS, "border:2px solid #4f46e5") {
		t.Errorf("variant style missing: %s", out.CSS)
	}
	if !strings.Contains(out.CSS, ".n-b-label{") || !strings.Contains(out.CSS, "color:#ff0000") {
		t.Errorf("overrides must apply to the variant's layers too: %s", out.CSS)
	}
}
