package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The Store refuses a package whose manifest description is over 500 bytes (pkgfmt), and Thai
// letters are 3 bytes each — check every official plugin before it reaches publish-official.sh.
func TestOfficialManifestsFitTheStore(t *testing.T) {
	root := filepath.Join("..", "..", "..", "plugins-official")
	dirs, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, d.Name(), "manifest.json"))
		if err != nil {
			t.Fatalf("%s: %v", d.Name(), err)
		}
		var mf struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal(b, &mf); err != nil {
			t.Fatalf("%s manifest: %v", d.Name(), err)
		}
		if n := len(mf.Description); n == 0 || n > 500 {
			t.Errorf("%s: description is %d bytes (the Store allows 1–500)", d.Name(), n)
		}
	}
}
