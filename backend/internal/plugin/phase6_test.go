package plugin

import "testing"

func TestAssetPathOK(t *testing.T) {
	for _, p := range []string{"fonts/Sarabun.ttf", "data/a.json", "README.md", "templates/x.html"} {
		if !AssetPathOK(p) {
			t.Errorf("%q refused", p)
		}
	}
	for _, p := range []string{"", "/etc/passwd", "../x", "a/../b", "./a", "code/main.ts", "CODE/main.ts", "migrations/1.ts", "manifest.json",
		`a\b`, "c:x", ".env", "a/.hidden", "a//b", "a/"} {
		if AssetPathOK(p) {
			t.Errorf("%q accepted", p)
		}
	}
}

func TestOptionalRequiresManifest(t *testing.T) {
	ok := &Manifest{ID: "shop", OptionalRequires: map[string]string{"thai": "^1.0.0"}, Requires: map[string]string{"money": "^1"}}
	if err := ok.validateRequires(); err != nil {
		t.Fatal(err)
	}
	for name, mf := range map[string]*Manifest{
		"bad range": {ID: "shop", OptionalRequires: map[string]string{"thai": "nope"}},
		"bad id":    {ID: "shop", OptionalRequires: map[string]string{"X!": "*"}},
		"self":      {ID: "shop", OptionalRequires: map[string]string{"shop": "*"}},
		"both":      {ID: "shop", OptionalRequires: map[string]string{"thai": "*"}, Requires: map[string]string{"thai": "*"}},
		"builtin":   {ID: "shop", OptionalRequires: map[string]string{"backup": "*"}},
	} {
		if err := mf.validateRequires(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
