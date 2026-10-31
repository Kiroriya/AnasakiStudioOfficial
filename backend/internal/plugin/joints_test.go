package plugin

import "testing"

func TestValidateEventAliases(t *testing.T) {
	if err := validateEventAliases("forms", map[string]string{"forms.submitted": "form.submitted"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string]string{
		{"other.submitted": "form.submitted"}, // not the plugin's own event
		{"forms.submitted": "post.saved"},     // a CMS event
		{"forms.submitted": "user.created"},
		{"forms.submitted": "forms.other"}, // own prefix
		{"forms.submitted": "nodot"},
		{"forms.submitted": "Form.Submitted"},
	} {
		if err := validateEventAliases("forms", bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}

func TestMergeCodeBlocks(t *testing.T) {
	list := []BlockInfo{{Plugin: "forms", Type: "form", Icon: "old"}, {Plugin: "x", Type: "y"}}
	out := mergeCodeBlocks(list, []BlockInfo{{Plugin: "forms", Type: "form", Icon: "new"}})
	if len(out) != 2 || out[0].Plugin != "x" || out[1].Icon != "new" {
		t.Fatalf("%+v", out)
	}
	if got := mergeCodeBlocks(list, nil); len(got) != 2 {
		t.Fatal("nil code blocks")
	}
}
