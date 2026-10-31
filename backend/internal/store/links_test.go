package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReplaceHref(t *testing.T) {
	var v any
	_ = json.Unmarshal([]byte(`{"props":{"href":"/old"},"children":[{"props":{"href":"/old"}},{"props":{"href":"/older","text":"/old"}}]}`), &v)
	if !replaceHref(v, "/old", "/new", "href") {
		t.Fatal("expected a change")
	}
	b, _ := json.Marshal(v)
	s := string(b)
	if strings.Count(s, `"href":"/new"`) != 2 || !strings.Contains(s, `"href":"/older"`) || !strings.Contains(s, `"text":"/old"`) {
		t.Errorf("only exact href values change: %s", s)
	}
	var m any
	_ = json.Unmarshal([]byte(`[{"label":"Contact","url":"/old"}]`), &m)
	if !replaceHref(m, "/old", "/new", "url") {
		t.Error("menu urls change too")
	}
}
