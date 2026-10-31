package config

import "testing"

func TestCheckJWTSecret(t *testing.T) {
	for _, s := range []string{"", "  ", "dev-secret-change-me"} {
		if _, err := CheckJWTSecret(s); err == nil {
			t.Errorf("%q must be refused", s)
		}
	}
	if warn, err := CheckJWTSecret("dev"); err != nil || warn == "" {
		t.Errorf("short secret: warn=%q err=%v", warn, err)
	}
	if warn, err := CheckJWTSecret("0123456789abcdef0123456789abcdef"); err != nil || warn != "" {
		t.Errorf("long secret: warn=%q err=%v", warn, err)
	}
}
