package plugin

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.7:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.9")
	r.Header.Set("X-Real-IP", "5.6.7.8")
	defer SetTrustProxy(false)

	SetTrustProxy(false)
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Fatalf("untrusted proxy headers must be ignored: %s", got)
	}
	SetTrustProxy(true)
	if got := ClientIP(r); got != "5.6.7.8" {
		t.Fatalf("X-Real-IP behind a trusted proxy: %s", got)
	}
	r.Header.Del("X-Real-IP")
	if got := ClientIP(r); got != "198.51.100.9" {
		t.Fatalf("last X-Forwarded-For hop: %s", got)
	}
	r.Header.Del("X-Forwarded-For")
	r.RemoteAddr = "[2001:db8::1]:443"
	if got := ClientIP(r); got != "2001:db8::1" {
		t.Fatalf("ipv6 remote addr: %s", got)
	}
}
