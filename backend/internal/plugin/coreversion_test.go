package plugin

import "testing"

func TestSetCoreVersion(t *testing.T) {
	old := CoreVersion
	t.Cleanup(func() { CoreVersion = old })

	for _, v := range []string{"dev", "", "2.0", "v2.0.0", "0.0.0-abc 1"} {
		if SetCoreVersion(v) || CoreVersion != old {
			t.Fatalf("SetCoreVersion(%q) accepted a non-release stamp (CoreVersion=%q)", v, CoreVersion)
		}
	}
	if !SetCoreVersion("2.0.0") || CoreVersion != "2.0.0" {
		t.Fatalf("release version not applied: %q", CoreVersion)
	}
	// a plugin asking for a newer core is refused, an older minCore is fine
	if CompareVersions("2.1.0", CoreVersion) <= 0 || CompareVersions("1.0.0", CoreVersion) > 0 {
		t.Fatal("minCore comparison against the release version")
	}
}
