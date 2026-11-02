package sitecode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every core event and filter is typed in anasaki.d.ts and listed in
// docs/plugin-api.md (2.1.0: a joint nobody can find is a gap again).
func TestCoreHookCatalog(t *testing.T) {
	dts, err := os.ReadFile("anasaki.d.ts")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "plugin-api.md"))
	if err != nil {
		t.Skip("docs/plugin-api.md not in this checkout")
	}
	check := func(kind string, names map[string]bool) {
		for name := range names {
			if !strings.Contains(string(dts), "'"+name+"'") {
				t.Errorf("core %s %q is not in anasaki.d.ts", kind, name)
			}
			if !strings.Contains(string(docs), "`"+name+"`") {
				t.Errorf("core %s %q is not in docs/plugin-api.md", kind, name)
			}
		}
	}
	check("event", coreEvents)
	check("filter", coreFilters)
}
