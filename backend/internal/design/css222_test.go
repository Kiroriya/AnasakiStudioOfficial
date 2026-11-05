package design

import (
	"encoding/json"
	"os"
	"testing"
)

// The style properties of 2.2.2 (blend, scroll, minWidth, maxHeight, aspect, maxLines, strokeSides) give the same
// CSS on the site as on the designer canvas: testdata/css_vectors_222.json is shared with design-css.ts.
func TestCSSVectors222(t *testing.T) {
	raw, err := os.ReadFile("testdata/css_vectors_222.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string            `json:"name"`
		Type   string            `json:"type"`
		Style  map[string]any    `json:"style"`
		Expect map[string]string `json:"expect"`
		Absent []string          `json:"absent"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		n := &Node{ID: "x", Type: c.Type, Style: c.Style, Width: Size{Mode: "hug"}, Height: Size{Mode: "hug"}}
		got := map[string]string{}
		for _, d := range Decls(n, "vertical") {
			got[d.Prop] = d.Value
		}
		for p, v := range c.Expect {
			if got[p] != v {
				t.Errorf("%s: %s = %q, want %q", c.Name, p, got[p], v)
			}
		}
		for _, p := range c.Absent {
			if _, ok := got[p]; ok {
				t.Errorf("%s: %s should be absent (%q)", c.Name, p, got[p])
			}
		}
	}
}
