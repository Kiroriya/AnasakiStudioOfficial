package config

import "testing"

func TestCORSOffByDefault(t *testing.T) {
	t.Setenv("CORS_ORIGIN", "")
	if got := Load().CORSOrigin; got != "" {
		t.Fatalf("no cross-origin access unless CORS_ORIGIN is set, got %q", got)
	}
	t.Setenv("CORS_ORIGIN", " http://localhost:4200/ ")
	if got := Load().CORSOrigin; got != "http://localhost:4200" {
		t.Fatalf("CORS_ORIGIN: %q", got)
	}
}
