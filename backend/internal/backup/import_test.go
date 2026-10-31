package backup

import "testing"

func TestImportableFile(t *testing.T) {
	cases := map[string][2]bool{ // name → {without unfiltered_html, with}
		"2024/01/a.png":  {true, true},
		"2024/01/a.PDF":  {true, true},
		"2024/01/a.svg":  {false, true},
		"2024/01/a.html": {false, false},
		"2024/01/a.js":   {false, false},
		"2024/01/noext":  {false, false},
	}
	for name, want := range cases {
		if got := importableFile(name, false); got != want[0] {
			t.Errorf("%s without unfiltered_html: %v", name, got)
		}
		if got := importableFile(name, true); got != want[1] {
			t.Errorf("%s with unfiltered_html: %v", name, got)
		}
	}
}
