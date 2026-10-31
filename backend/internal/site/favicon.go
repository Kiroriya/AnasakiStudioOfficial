package site

import (
	_ "embed"
	"net/http"
)

// defaultFavicon is the Anasaki icon, identical to the admin's favicon.svg,
// used until the owner uploads a site icon (Settings → General).
//
//go:embed favicon.svg
var defaultFavicon []byte

// favicon answers /favicon.ico and /favicon.svg: the site icon when one is
// set (browsers and crawlers request /favicon.ico directly), else the default.
func (s *Site) favicon(w http.ResponseWriter, r *http.Request, opts map[string]string) {
	if icon := s.siteIcon(opts); icon != "" {
		w.Header().Set("Cache-Control", "no-cache")
		http.Redirect(w, r, icon, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(defaultFavicon)
}
