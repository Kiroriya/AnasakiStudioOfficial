package plugin

import (
	"html"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
)

// PageInfo describes the public page being rendered; head/body hooks and
// page filters receive it.
type PageInfo struct {
	Path        string `json:"path"`        // request path, e.g. "/blog/hello"
	Title       string `json:"title"`       // page title without the site name
	Description string `json:"description"` // meta description ("" = none)
	Image       string `json:"image"`       // featured / share image URL
	PostID      int64  `json:"postId"`      // 0 when not a post/page
	PostType    string `json:"postType"`    // page | post | archive | 404 | plugin
	Status      int    `json:"status"`      // HTTP status of the page

	// FullTitle is the <title> text (default "Title – Site"); filters may
	// replace it (SEO title template).
	FullTitle string `json:"fullTitle"`
	// NoIndex adds <meta name="robots" content="noindex,nofollow">.
	NoIndex bool `json:"noindex"`
	// BaseURL is scheme://host of the site (option site_url or the request).
	BaseURL string `json:"baseUrl"`
	// Site holds title, tagline and url for templates.
	Site map[string]string `json:"-"`
}

// AbsURL makes u absolute against the page's base URL.
func (p PageInfo) AbsURL(u string) string {
	if u == "" || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	return strings.TrimRight(p.BaseURL, "/") + u
}

// Middleware chains the active plugins' middlewares around next. The chain
// is rebuilt per request so (de)activation applies immediately.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		set := m.activeSet()
		h := next
		for i := len(m.entries) - 1; i >= 0; i-- {
			e := m.entries[i]
			if !set[e.info.ID] {
				continue
			}
			for j := len(e.reg.middleware) - 1; j >= 0; j-- {
				h = e.reg.middleware[j](h)
			}
		}
		h.ServeHTTP(w, r)
	})
}

// Emit publishes a core event (e.g. "post.published") to the active plugins
// that subscribed with Registry.On and to package webhooks listing it.
// Delivery is asynchronous; Emit never blocks the caller.
func (m *Manager) Emit(event string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	set := m.activeSet()
	for _, e := range m.entries {
		if !set[e.info.ID] {
			continue
		}
		for _, fn := range e.reg.events[event] {
			go func(id string, fn func(map[string]any)) {
				defer func() {
					if rec := recover(); rec != nil {
						log.Printf("plugin %s: event %s: panic: %v", id, event, rec)
					}
				}()
				fn(payload)
			}(e.info.ID, fn)
		}
	}
	for _, fn := range m.listeners {
		fn(event, payload)
	}
	m.pkgs.webhooks(event, payload)
}

// Listen subscribes a core component (Site Code) to every emitted event;
// fn must not block. Call it at startup, before requests are served.
func (m *Manager) Listen(fn func(event string, payload map[string]any)) {
	m.listeners = append(m.listeners, fn)
}

// FilterPage lets active plugins adjust the page's title/description/image.
func (m *Manager) FilterPage(p *PageInfo) {
	set := m.activeSet()
	for _, e := range m.entries {
		if set[e.info.ID] {
			for _, f := range e.reg.filters {
				safeCall(e.info.ID, func() { f(p) })
			}
		}
	}
}

// HeadHTML returns every active plugin's <head> HTML for page p: static
// tags, per-page hooks and package plugin assets.
func (m *Manager) HeadHTML(p PageInfo) string {
	set := m.activeSet()
	var b strings.Builder
	for _, e := range m.entries {
		if !set[e.info.ID] {
			continue
		}
		for _, h := range e.reg.head {
			b.WriteString(h)
		}
		for _, fn := range e.reg.heads {
			safeCall(e.info.ID, func() { b.WriteString(fn(p)) })
		}
	}
	b.WriteString(m.pkgs.headHTML())
	return b.String()
}

// BodyEndHTML returns the HTML active plugins append before </body>.
func (m *Manager) BodyEndHTML(p PageInfo) string {
	set := m.activeSet()
	var b strings.Builder
	for _, e := range m.entries {
		if !set[e.info.ID] {
			continue
		}
		for _, fn := range e.reg.bodyEnds {
			safeCall(e.info.ID, func() { b.WriteString(fn(p)) })
		}
	}
	b.WriteString(m.pkgs.bodyEndHTML(p))
	return b.String()
}

func safeCall(id string, fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("plugin %s: hook panic: %v", id, rec)
		}
	}()
	fn()
}

// Esc is html.EscapeString, exported for plugin hooks.
func Esc(s string) string { return html.EscapeString(s) }

// StatusRecorder wraps a ResponseWriter to remember the status code and
// content type (used by middlewares that act after the site answered).
type StatusRecorder struct {
	http.ResponseWriter
	Status int
}

// WriteHeader records the status.
func (s *StatusRecorder) WriteHeader(code int) {
	if s.Status == 0 {
		s.Status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

// Write records an implicit 200.
func (s *StatusRecorder) Write(b []byte) (int, error) {
	if s.Status == 0 {
		s.Status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Code returns the final status (200 when nothing was written).
func (s *StatusRecorder) Code() int {
	if s.Status == 0 {
		return http.StatusOK
	}
	return s.Status
}

// Unwrap exposes the underlying writer (http.ResponseController).
func (s *StatusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

var trustProxy atomic.Bool

// SetTrustProxy makes ClientIP believe X-Real-IP / X-Forwarded-For (only
// when the server sits behind a reverse proxy that sets them: TRUST_PROXY).
func SetTrustProxy(v bool) { trustProxy.Store(v) }

// TrustProxy reports whether proxy headers are trusted.
func TrustProxy() bool { return trustProxy.Load() }

// RequestScheme is "https" or "http" for r. X-Forwarded-Proto is believed
// only behind a trusted proxy (TRUST_PROXY), so a client can't forge it.
func RequestScheme(r *http.Request) string {
	if r.TLS != nil || (TrustProxy() && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")) {
		return "https"
	}
	return "http"
}

// RequestHost is the host the visitor asked for: X-Forwarded-Host behind a
// trusted proxy (TRUST_PROXY), else the Host header.
func RequestHost(r *http.Request) string {
	if TrustProxy() {
		if fh := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); fh != "" {
			return fh
		}
	}
	return r.Host
}

// ClientIP returns the caller's IP: the connection's address, or behind a
// trusted proxy X-Real-IP / the last X-Forwarded-For hop (the one the proxy added).
func ClientIP(r *http.Request) string {
	if trustProxy.Load() {
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
		if xf := r.Header.Values("X-Forwarded-For"); len(xf) > 0 {
			last := xf[len(xf)-1]
			if i := strings.LastIndex(last, ","); i >= 0 {
				last = last[i+1:]
			}
			if ip := strings.TrimSpace(last); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return strings.Trim(host, "[]")
}
