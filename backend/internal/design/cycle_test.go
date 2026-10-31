package design

import (
	"strings"
	"testing"
	"time"
)

// loopData has component 1 holding two instances of itself.
type loopData struct{ fakeData }

func (loopData) Component(id int64) *Node {
	inst := func(i string) *Node {
		return &Node{ID: "i" + i, Type: TypeInstance, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
			Props: map[string]any{"componentId": float64(1)}}
	}
	return &Node{ID: "c", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Children: []*Node{{ID: "t", Type: TypeText, Props: map[string]any{"text": "loop"}}, inst("a"), inst("b")}}
}

func TestRenderComponentCycle(t *testing.T) {
	root := &Node{ID: "root", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Children: []*Node{{ID: "x", Type: TypeInstance, Props: map[string]any{"componentId": float64(1)}}}}
	start := time.Now()
	out := Render(root, Context{}, loopData{})
	if time.Since(start) > 2*time.Second {
		t.Fatalf("cyclic component took %s", time.Since(start))
	}
	if n := strings.Count(out.HTML, "loop"); n != 1 {
		t.Fatalf("component rendered %d times, want 1", n)
	}
}
