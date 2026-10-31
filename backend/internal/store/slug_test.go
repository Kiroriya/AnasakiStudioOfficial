package store

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSlugifyLongThai(t *testing.T) {
	s := Slugify(strings.Repeat("ภาษาไทย", 60))
	if !utf8.ValidString(s) {
		t.Fatalf("invalid UTF-8: %q", s)
	}
	if n := utf8.RuneCountInString(s); n != 180 {
		t.Fatalf("want 180 characters, got %d", n)
	}
	if Slugify("Hello World") != "hello-world" {
		t.Fatal("ascii slug changed")
	}
}
