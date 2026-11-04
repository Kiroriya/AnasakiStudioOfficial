package design

import (
	"strings"
	"testing"
)

func TestFontStack(t *testing.T) {
	for in, want := range map[string]string{
		"Kanit":                                "'Kanit', sans-serif",
		"ui-sans-serif, system-ui, sans-serif": "ui-sans-serif, system-ui, sans-serif",
		"'Noto Sans Thai', Segoe UI, serif":    "'Noto Sans Thai', 'Segoe UI', serif",
		"Kanit, system-ui":                     "'Kanit', system-ui",
		"x';}body{color:red":                   "'xbodycolorred', sans-serif",
		" , ":                                  "",
	} {
		if got := FontStack(in); got != want {
			t.Errorf("FontStack(%q) = %q, want %q", in, got, want)
		}
	}
	if PrimaryFont("system-ui, Kanit") != "" || PrimaryFont("Kanit, system-ui") != "Kanit" {
		t.Error("PrimaryFont")
	}
}

func TestParseForSaveDuplicates(t *testing.T) {
	if _, err := ParseForSave([]byte(`{"id":"r","type":"frame","children":[{"id":"a","type":"text"},{"id":"b","type":"frame","children":[{"id":"a","type":"text"}]}]}`)); err == nil || !strings.Contains(err.Error(), `"a"`) {
		t.Fatalf("nested duplicate: %v", err)
	}
	if _, err := ParseForSave([]byte(`{"id":"r","type":"frame","children":[{"type":"text"},{"type":"text"}]}`)); err != nil {
		t.Fatalf("layers without ids are left alone: %v", err)
	}
}

func TestInputBoxed(t *testing.T) {
	if !inputBoxed(&Node{Style: map[string]any{"stroke": "#ddd"}}) || !inputBoxed(&Node{Style: map[string]any{"radius": 8.0}}) ||
		!inputBoxed(&Node{Responsive: map[string]map[string]any{"mobile": {"style": map[string]any{"fill": "#fff"}}}}) {
		t.Error("boxed layer not detected")
	}
	if inputBoxed(&Node{Style: map[string]any{"fontSize": 14.0}}) {
		t.Error("unstyled field must keep the browser box")
	}
	if !strings.Contains(RuntimeCSS, ".rea-field-box .rea-field-el{border:0") {
		t.Error("runtime css")
	}
}
