package sitecode

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// Static files of Site Code (2.2.0): files under public/ in the project are
// not bundled; the live version serves them at /site-assets/<path below
// public/> (site.asset('public/x.css').url adds ?v=<version>, cached for a
// year; without ?v= they are revalidated). Text only, as every project file.

// PublicDir is the project folder whose files are served as they are.
const PublicDir = "public/"

// PublicPrefix is where the live version's public/ files are served.
const PublicPrefix = "/site-assets/"

// publicExt are the file types allowed under public/ (besides the code ones).
var publicExt = map[string]string{
	".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8",
	".json": "application/json; charset=utf-8", ".svg": "image/svg+xml", ".txt": "text/plain; charset=utf-8",
	".xml": "application/xml; charset=utf-8", ".webmanifest": "application/manifest+json", ".md": "text/markdown; charset=utf-8",
}

// isPublic reports whether p is a static file of the project.
func isPublic(p string) bool { return strings.HasPrefix(p, PublicDir) && len(p) > len(PublicDir) }

// servePublicFile answers /site-assets/… from the live version (false: not
// such a path). An unknown file is a 404 here, never a page.
func (e *Engine) servePublicFile(w http.ResponseWriter, r *http.Request, opts map[string]string) bool {
	rest, ok := strings.CutPrefix(r.URL.Path, PublicPrefix)
	if !ok {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	lp := e.current(opts)
	src, found := "", false
	if lp != nil && checkSegments(rest) == nil {
		src, found = lp.files[PublicDir+rest]
	}
	ct, typed := publicExt[strings.ToLower(path.Ext(rest))]
	if !found || !typed {
		http.NotFound(w, r)
		return true
	}
	sum := sha256.Sum256([]byte(src))
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("ETag", etag)
	h.Set("X-Content-Type-Options", "nosniff")
	if strings.HasSuffix(ct, "svg+xml") { // an SVG opened directly must not run script
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	}
	if v := r.URL.Query().Get("v"); v != "" && v == strconv.FormatInt(lp.id, 10) {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "public, max-age=0, must-revalidate")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	h.Set("Content-Length", strconv.Itoa(len(src)))
	if r.Method == http.MethodHead {
		return true
	}
	_, _ = w.Write([]byte(src))
	return true
}

// publicURL is the address of a public/ file of the live (or loading)
// version id.
func publicURL(p string, version int64) string {
	return PublicPrefix + strings.TrimPrefix(p, PublicDir) + "?v=" + strconv.FormatInt(version, 10)
}
