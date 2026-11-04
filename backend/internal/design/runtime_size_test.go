package design

import (
	"strings"
	"testing"
)

func TestRuntimeScriptSize(t *testing.T) {
	js := RuntimeScript()
	if js == runtimeSource {
		t.Fatal("runtime was not minified")
	}
	// phase 3 budget was 6 KB; phase 5 (window.anasaki get/post/refresh/on)
	// raised it to 7 KB; 2.2.0 (ref/refs, lib/style for public/ files) to 8 KB
	if len(js) >= 8*1024 {
		t.Fatalf("runtime is %d bytes minified (budget < 8 KB)", len(js))
	}
	for _, want := range []string{"/_a/", "/_c/", "X-CSRF-Token", "anasaki_csrf", "data-rea-action", "pushState",
		"window.anasaki", "refresh:", "ref:", "lib:", "style:", "/site-assets/", "on:", "get:", "post:", "anasaki:", "data-rea-source"} {
		if !strings.Contains(js, want) {
			t.Errorf("runtime lacks %q", want)
		}
	}
	t.Logf("client runtime: %d bytes minified", len(js))
}
