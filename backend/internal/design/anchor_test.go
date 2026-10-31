package design

import (
	"strings"
	"testing"
)

func TestAnchoredLeft(t *testing.T) {
	cases := []struct {
		x, ref float64
		anchor string
		want   string
	}{
		{1000, 0, "center", "1000px"},               // no reference: plain left
		{1000, 1440, "", "1000px"},                  // left
		{1000, 1440, "center", "calc(50% + 280px)"}, // 280px right of the centre
		{400, 1440, "center", "calc(50% - 320px)"},  // left of the centre
		{720, 1440, "center", "50%"},                // exactly the centre
		{1340, 1440, "right", "calc(100% - 100px)"}, // 100px from the right edge
		{360, 1440, "scale", "25%"},
		{100.456, 0, "", "100.46px"},
	}
	for _, c := range cases {
		if got := anchoredLeft(c.x, c.anchor, c.ref); got != c.want {
			t.Errorf("anchoredLeft(%v, %q, %v) = %q, want %q", c.x, c.anchor, c.ref, got, c.want)
		}
	}
}

func TestAnchoredLayerCSS(t *testing.T) {
	n := &Node{ID: "pb", Type: "rect", Absolute: true, X: 1000, Y: 120, Width: Size{Mode: "fixed", Value: 300}, Height: Size{Mode: "fixed", Value: 100},
		Style: map[string]any{"anchorX": "center", "anchorRef": float64(1440)}}
	var left string
	for _, d := range Decls(n, "vertical") {
		if d.Prop == "left" {
			left = d.Value
		}
	}
	if left != "calc(50% + 280px)" {
		t.Fatalf("left = %q", left)
	}
	// a tablet override with its own x and reference
	n.Responsive = map[string]map[string]any{"tablet": {"x": float64(500), "style": map[string]any{"anchorRef": float64(834)}}}
	e := n.Effective("tablet")
	for _, d := range Decls(e, "vertical") {
		if d.Prop == "left" && !strings.Contains(d.Value, "83px") {
			t.Fatalf("tablet left = %q", d.Value)
		}
	}
}

func TestFixedHeightFrameGrows(t *testing.T) {
	css := func(n *Node) map[string]string {
		out := map[string]string{}
		for _, d := range Decls(n, "vertical") {
			out[d.Prop] = d.Value
		}
		return out
	}
	frame := &Node{ID: "s", Type: "frame", Width: Size{Mode: "fill"}, Height: Size{Mode: "fixed", Value: 500},
		Layout: map[string]any{"mode": "vertical"}}
	if c := css(frame); c["min-height"] != "500px" || c["height"] != "" {
		t.Fatalf("auto-layout frame: %v", c)
	}
	frame.Style = map[string]any{"minHeight": float64(760)}
	if c := css(frame); c["min-height"] != "760px" {
		t.Fatalf("the larger minimum wins: %v", c)
	}
	frame.Style = map[string]any{"clip": true}
	if c := css(frame); c["height"] != "500px" {
		t.Fatalf("clip keeps the exact height: %v", c)
	}
	free := &Node{ID: "f", Type: "frame", Width: Size{Mode: "fill"}, Height: Size{Mode: "fixed", Value: 300}}
	if c := css(free); c["height"] != "300px" {
		t.Fatalf("freeform frame keeps its height: %v", c)
	}
	img := &Node{ID: "i", Type: "image", Width: Size{Mode: "fill"}, Height: Size{Mode: "fixed", Value: 284}}
	if c := css(img); c["height"] != "284px" {
		t.Fatalf("an image keeps its height: %v", c)
	}
}
