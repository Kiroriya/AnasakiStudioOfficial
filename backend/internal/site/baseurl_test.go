package site

import (
	"net/http/httptest"
	"testing"
)

// Behind a CDN that ends TLS the request arrives as plain http: the owner's address (option site_url, else
// SITE_URL) must win, or canonical / sitemap / og:image URLs come out as http://.
func TestBaseURLPrefersTheConfiguredAddress(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com/about", nil)
	t.Setenv("SITE_URL", "")
	if got := BaseURL(map[string]string{}, r); got != "http://example.com" {
		t.Fatalf("from the request: %q", got)
	}
	t.Setenv("SITE_URL", "https://example.com/")
	if got := BaseURL(map[string]string{}, r); got != "https://example.com" {
		t.Fatalf("SITE_URL: %q", got)
	}
	if got := BaseURL(map[string]string{"site_url": "https://www.example.com"}, r); got != "https://www.example.com" {
		t.Fatalf("option site_url first: %q", got)
	}
	t.Setenv("SITE_URL", "example.com") // not an absolute URL: ignored
	if got := BaseURL(map[string]string{}, r); got != "http://example.com" {
		t.Fatalf("invalid SITE_URL: %q", got)
	}
	t.Setenv("SITE_URL", "https://example.com")
	if got := BaseURL(map[string]string{}, nil); got != "https://example.com" {
		t.Fatalf("no request: %q", got)
	}
	// a same-origin POST from the https page is accepted although the request itself looks like http
	p := httptest.NewRequest("POST", "http://example.com/_dyn", nil)
	p.Header.Set("Origin", "https://example.com")
	if !sameOrigin(p, map[string]string{}) {
		t.Fatal("Origin https://example.com refused with SITE_URL set")
	}
}
