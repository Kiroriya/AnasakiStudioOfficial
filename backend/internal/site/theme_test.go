package site

import (
	"strings"
	"testing"
)

func TestTheme(t *testing.T) {
	th := ParseTheme(`{"colors":{"primary":"#F59AA6","text":"red;}"},"fontHeading":"Prompt","fontBody":"bad;font"}`)
	css := th.CSS()
	if !strings.Contains(css, "--c-primary:#F59AA6") {
		t.Errorf("primary not applied: %s", css)
	}
	if !strings.Contains(css, "--c-text:#111827") {
		t.Errorf("invalid color should fall back to default: %s", css)
	}
	if !strings.Contains(css, "h1,h2,h3,h4,h5,h6{font-family:'Prompt'") || strings.Contains(css, "bad") {
		t.Errorf("fonts: %s", css)
	}
	if ValidateTheme(`{"colors":{"primary":"url(x)"}}`) == "" || ValidateTheme(`{"colors":{"nope":"#fff"}}`) == "" {
		t.Error("invalid theme accepted")
	}
	if msg := ValidateTheme(`{"colors":{"primary":"#fff"},"fontBody":"Sarabun"}`); msg != "" {
		t.Errorf("valid theme rejected: %s", msg)
	}
}
