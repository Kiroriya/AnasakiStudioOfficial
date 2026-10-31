package sitecode

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"net/http"
	"strings"
)

// Phase 6: the admin UI kit for plugin admin pages (site.adminPage), served
// by core at /ext-ui/kit.css and /ext-ui/kit.js and linked by every code
// admin page document. Window.AK (kit.js) and the ak-* classes (kit.css) are
// documented in docs/site-code.md and kit/kit-demo.html.

//go:embed kit/kit.css
var kitCSS []byte

//go:embed kit/kit.js
var kitJS []byte

// KitVersion changes with the kit's content (cache busting: ?v=).
var KitVersion = func() string {
	h := sha256.New()
	h.Write(kitCSS)
	h.Write(kitJS)
	return hex.EncodeToString(h.Sum(nil))[:12]
}()

// ServeKit answers /ext-ui/kit.css and /ext-ui/kit.js (false for any other
// path).
func ServeKit(w http.ResponseWriter, r *http.Request) bool {
	var body []byte
	var ctype string
	switch strings.TrimPrefix(r.URL.Path, "/ext-ui/") {
	case "kit.css":
		body, ctype = kitCSS, "text/css; charset=utf-8"
	case "kit.js":
		body, ctype = kitJS, "text/javascript; charset=utf-8"
	default:
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Access-Control-Allow-Origin", "*") // admin pages run in a sandbox with an opaque origin
	if r.URL.Query().Get("v") == KitVersion {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "public, max-age=300")
	}
	h.Set("ETag", `"`+KitVersion+`"`)
	if r.Header.Get("If-None-Match") == `"`+KitVersion+`"` {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
	return true
}
