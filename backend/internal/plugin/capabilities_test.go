package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

// Manifest "capabilities" (2.0.6): syntax, labels, default roles, and the
// service admin page "cap" naming the plugin's own capability.
func TestManifestCapabilities(t *testing.T) {
	check := func(caps string, service string) error {
		t.Helper()
		raw := `{"id":"cap-kit","name":"Cap Kit","version":"1.0.0","permissions":[` + map[bool]string{true: `"service"`, false: ``}[service != ""] + `]` +
			`,"capabilities":` + caps
		if service != "" {
			raw += `,"service":` + service
		}
		raw += `}`
		var mf Manifest
		if err := json.Unmarshal([]byte(raw), &mf); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		return mf.Validate(map[string][]byte{"manifest.json": []byte(raw)})
	}
	ok := `[{"id":"view","label":{"th":"ดู","en":"View"},"description":{"th":"ก","en":"d"},"default":["administrator","editor","shop_manager"]},` +
		`{"id":"edit_meta","label":"Edit meta"}]`
	if err := check(ok, ""); err != nil {
		t.Fatalf("valid capabilities refused: %v", err)
	}
	var mf Manifest
	_ = json.Unmarshal([]byte(`{"id":"cap-kit","capabilities":`+ok+`}`), &mf)
	if keys := mf.CapKeys(); len(keys) != 2 || keys[0] != "cap-kit.view" || keys[1] != "cap-kit.edit_meta" ||
		!mf.DeclaresCap("cap-kit.view") || mf.DeclaresCap("other.view") || mf.Capabilities[1].Label.TH != "Edit meta" {
		t.Fatalf("keys: %v %+v", keys, mf.Capabilities)
	}
	if v := mf.capabilityViews(); v[0].Key != "cap-kit.view" || v[1].Default == nil {
		t.Fatalf("views: %+v", v)
	}
	many := make([]string, 31)
	for i := range many {
		many[i] = `{"id":"c` + strings.Repeat("a", i+1)[:1] + string(rune('a'+i%26)) + string(rune('a'+i/26)) + `","label":"x"}`
	}
	for caps, want := range map[string]string{
		`[{"id":"View","label":"x"}]`:                                                  "must match",
		`[{"id":"v","label":"x"}]`:                                                     "must match",
		`[{"id":"view","label":"x"},{"id":"view","label":"y"}]`:                        "twice",
		`[{"id":"view"}]`:                                                              "label",
		`[{"id":"view","label":{"th":"","en":""}}]`:                                    "label",
		`[{"id":"view","label":"` + strings.Repeat("ก", 81) + `"}]`:                    "longer",
		`[{"id":"view","label":"x","description":"` + strings.Repeat("d", 301) + `"}]`: "longer",
		`[{"id":"view","label":"x","default":["Editor"]}]`:                             "default",
		`[{"id":"view","label":"x","default":["editor","editor"]}]`:                    "twice",
		`[` + strings.Join(many, ",") + `]`:                                            "at most",
	} {
		if err := check(caps, ""); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want %q, got %v", caps, want, err)
		}
	}
	// a service admin page may require its own capability, never another plugin's or an undeclared one
	svc := func(cp string) string {
		return `{"public":[],"admin":[{"path":"orders","label":"Orders","cap":"` + cp + `"}]}`
	}
	if err := check(ok, svc("cap-kit.view")); err != nil {
		t.Fatalf("own capability on a service page: %v", err)
	}
	if err := check(ok, svc("manage_options")); err != nil {
		t.Fatalf("core capability on a service page: %v", err)
	}
	for _, bad := range []string{"other.view", "cap-kit.nope", "Bad Cap"} {
		if err := check(ok, svc(bad)); err == nil {
			t.Errorf("service admin cap %q accepted", bad)
		}
	}
}
