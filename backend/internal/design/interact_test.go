package design

import (
	"strings"
	"testing"
)

func TestInteractions(t *testing.T) {
	txt := func(id, s string, extra map[string]any) *Node {
		p := map[string]any{"text": s}
		for k, v := range extra {
			p[k] = v
		}
		return &Node{ID: id, Type: TypeText, Width: Size{Mode: "hug"}, Height: Size{Mode: "hug"}, Props: p}
	}
	root := &Node{ID: "r", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Layout: map[string]any{"mode": "vertical"},
		Children: []*Node{
			txt("open", "Open", map[string]any{"action": map[string]any{"type": "open", "target": "pop"}}),
			txt("bad", "Bad", map[string]any{"action": map[string]any{"type": "eval", "target": "pop"}}),
			{ID: "pop", Type: TypeFrame, Width: Size{Mode: "fixed", Value: 400}, Height: Size{Mode: "hug"}, Props: map[string]any{"popup": true, "popupDelay": 3.0, "popupOnce": true},
				Children: []*Node{txt("x", "Close", map[string]any{"action": map[string]any{"type": "close"}})}},
			txt("q", "Question", map[string]any{"action": map[string]any{"type": "toggle", "target": "a\"><script>"}}),
			txt("a", "Answer", map[string]any{"collapsed": true}),
			{ID: "car", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "fixed", Value: 300}, Layout: map[string]any{"mode": "horizontal"},
				Props:    map[string]any{"carousel": map[string]any{"autoplay": 4.0, "arrows": true, "dots": true}},
				Children: []*Node{{ID: "s1", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "fill"}}, {ID: "s2", Type: TypeFrame, Width: Size{Mode: "fixed", Value: 200}, Height: Size{Mode: "fill"}}}},
		}}
	root.Children[0].Style = map[string]any{"active": map[string]any{"fill": "#111111"}}
	out := Render(root, Context{}, nil)
	if !out.Interact || len(out.Actions) != 3 {
		t.Fatalf("actions: %+v", out.Actions)
	}
	if out.Actions[0] != (Action{S: "n-open", A: "open", T: "n-pop"}) || out.Actions[1].A != "close" {
		t.Errorf("unexpected actions %+v", out.Actions)
	}
	if strings.ContainsAny(out.Actions[2].T, `"<>`) {
		t.Errorf("target must be a plain class: %q", out.Actions[2].T)
	}
	for _, want := range []string{`<div class="rea-pop" role="dialog" aria-modal="true" data-id="n-pop" data-delay="3" data-once="1"><div class="rea-pop-bg"></div>`,
		`rea-collapsed`, `class="n-car rea-car" data-autoplay="4"><div class="rea-track">`, `n-s1 rea-slide-fill`, `rea-car-prev`, `rea-car-dots`} {
		if !strings.Contains(out.HTML, want) {
			t.Errorf("missing %q in %s", want, out.HTML)
		}
	}
	if strings.Contains(out.HTML, "n-s2 rea-slide-fill") {
		t.Error("fixed-width slides keep their width")
	}
	if !strings.Contains(out.CSS, ".n-open.rea-active{background:#111111}") || !strings.Contains(out.CSS, ".rea-pop{") {
		t.Errorf("css: %s", out.CSS)
	}
}

type interactData struct{ fakeData }

func (interactData) Component(int64) *Node {
	return &Node{ID: "faq", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Children: []*Node{
			{ID: "q", Type: TypeText, Width: Size{Mode: "hug"}, Height: Size{Mode: "hug"}, Props: map[string]any{"text": "Q", "action": map[string]any{"type": "toggle", "target": "a"}}},
			{ID: "a", Type: TypeText, Width: Size{Mode: "hug"}, Height: Size{Mode: "hug"}, Props: map[string]any{"text": "A", "collapsed": true}},
		}}
}

func TestInteractionsInsideComponents(t *testing.T) {
	root := &Node{ID: "r", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Children: []*Node{
			{ID: "i1", Type: TypeInstance, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Props: map[string]any{"componentId": 1.0}},
			{ID: "i2", Type: TypeInstance, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"}, Props: map[string]any{"componentId": 1.0}},
		}}
	out := Render(root, Context{}, interactData{})
	want := []Action{{S: "n-i1-q", A: "toggle", T: "n-i1-a"}, {S: "n-i2-q", A: "toggle", T: "n-i2-a"}}
	if len(out.Actions) != 2 || out.Actions[0] != want[0] || out.Actions[1] != want[1] {
		t.Fatalf("each placement must toggle its own answer: %+v", out.Actions)
	}
}
