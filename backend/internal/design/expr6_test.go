package design

import "testing"

// Phase 6: "| ns:name" is one plugin formatter name only when written
// without spaces, ns is not a core formatter, name is not a context root and
// not followed by "." — the same rule as the editor's evaluator.
func TestExprPluginFormatterNames(t *testing.T) {
	cases := map[string]struct {
		name  string
		nargs int
	}{
		"x | thai:baht":         {"thai:baht", 0},
		"x | thai:baht:2":       {"thai:baht", 1},
		"x | thai:baht:'a':b.c": {"thai:baht", 2},
		"x | thai :baht":        {"thai", 1},
		"x | thai: baht":        {"thai", 1},
		"x | thai:baht.x":       {"thai", 1},
		"x | fmt:item":          {"fmt", 1},
		"x | fmt:$index":        {"fmt", 1},
		"x | truncate:n":        {"truncate", 1},
		"x | default:fallback":  {"default", 1},
		"x | my_plugin:fmt":     {"my_plugin:fmt", 0},
		"x | fmt:a$b":           {"fmt", 1},
	}
	for src, want := range cases {
		e, err := ParseExpr(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		pn, ok := e.root.(*pipeNode)
		if !ok || pn.name != want.name || len(pn.args) != want.nargs {
			t.Fatalf("%q: %+v, want %+v", src, pn, want)
		}
	}
	env := &Env{Var: func(string) any { return 5.0 }, Format: func(name string, v any, args []any) (string, bool) {
		if name == "thai:baht" {
			return "฿" + Stringify(v) + Stringify(append(args, "")[0]), true
		}
		return "", false
	}}
	if got := Stringify(EvalString("x | thai:baht:'!' | upper", env)); got != "฿5!" {
		t.Fatalf("eval: %q", got)
	}
}
