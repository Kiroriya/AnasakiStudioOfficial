package design

import (
	"strings"
	"testing"
)

func TestStyleValuesCannotLeaveStyleBlock(t *testing.T) {
	n := &Node{ID: "a", Type: TypeFrame, Width: Size{Mode: "fill"}, Height: Size{Mode: "hug"},
		Style: map[string]any{"backgroundImage": "x');}</style><script>alert(1)</script>", "fill": "red;}</style><script>"}}
	out := Render(n, Context{}, nil)
	if strings.Contains(strings.ToLower(out.CSS), "</style") || strings.Contains(out.CSS, "<script") {
		t.Fatalf("css escapes the style block: %s", out.CSS)
	}
}

func TestSafeHrefControlChars(t *testing.T) {
	for _, u := range []string{"java\tscript:alert(1)", "\x01javascript:alert(1)", " JaVa\nScRiPt:alert(1)", "data:text/html,x", "vb\rscript:x"} {
		if got := safeHref(u); got != "#" {
			t.Errorf("safeHref(%q) = %q, want #", u, got)
		}
	}
	for _, u := range []string{"/about", "https://example.com/a?b=c", "mailto:a@b.c", "tel:+66 2 123"} {
		if got := safeHref(u); got != u {
			t.Errorf("safeHref(%q) = %q, want unchanged", u, got)
		}
	}
}
