package design

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

type exprVectors struct {
	Ctx   map[string]any `json:"ctx"`
	Cases []struct {
		Expr  string  `json:"expr"`
		Out   *string `json:"out"`
		Error bool    `json:"error"`
	} `json:"cases"`
}

func vectorEnv(ctx map[string]any, locale string) *Env {
	return &Env{Var: func(n string) any { return ctx[n] }, Locale: locale, Loc: time.UTC}
}

// TestExprVectors runs the vectors shared with the editor's TypeScript evaluator.
func TestExprVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/expr_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v exprVectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cases) < 40 {
		t.Fatalf("only %d cases", len(v.Cases))
	}
	env := vectorEnv(v.Ctx, "en")
	for _, c := range v.Cases {
		e, err := ParseExpr(c.Expr)
		if c.Error {
			if err == nil {
				t.Errorf("%q: parsed, want a parse error", c.Expr)
			}
			// the template keeps the raw text
			if got := ParseTemplate("a {{"+c.Expr+"}} b").Render(env, nil); got != "a {{"+c.Expr+"}} b" {
				t.Errorf("%q: template rendered %q", c.Expr, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.Expr, err)
			continue
		}
		if got := Stringify(e.Eval(env)); c.Out == nil || got != *c.Out {
			t.Errorf("%q = %q, want %v", c.Expr, got, *c.Out)
		}
		if got := ParseTemplate("{{ "+c.Expr+" }}").Render(env, nil); got != *c.Out {
			t.Errorf("template %q = %q, want %q", c.Expr, got, *c.Out)
		}
	}
}

func TestExprThaiDatesAndTimezone(t *testing.T) {
	ctx := map[string]any{"item": map[string]any{"date": "2026-10-24T20:05:00Z", "day": "2026-01-02", "ms": 1767312000.0, "local": "2026-10-24 20:05:00"}}
	th := vectorEnv(ctx, "th")
	cases := map[string]string{
		"item.day | date:'D MMM YYYY'":        "2 ม.ค. 2026",
		"item.day | date:'D MMMM BBBB'":       "2 มกราคม 2569",
		"item.day | date":                     "2 ม.ค. 2026",
		"item.local | date:'D MMM H:mm'":      "24 ต.ค. 20:05",
		"item.date | date:'D MMM BBBB HH:mm'": "24 ต.ค. 2569 20:05",
		"item.date | date:'[วันที่] D/M/YY'":  "วันที่ 24/10/26",
		"item.ms | date:'D MMM YYYY'":         "2 ม.ค. 2026",
	}
	for src, want := range cases {
		if got := Stringify(EvalString(src, th)); got != want {
			t.Errorf("th %q = %q, want %q", src, got, want)
		}
	}
	// site timezone: Bangkok is UTC+7 → the next day; a calendar date never shifts
	bkk, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		t.Skip("no tzdata")
	}
	th.Loc = bkk
	if got := Stringify(EvalString("item.date | date:'D MMMM BBBB H:mm'", th)); got != "25 ตุลาคม 2569 3:05" {
		t.Errorf("timezone: %q", got)
	}
	if got := Stringify(EvalString("item.day | date:'D/MM/YYYY'", th)); got != "2/01/2026" {
		t.Errorf("date-only shifted: %q", got)
	}
	en := vectorEnv(ctx, "en")
	if got := Stringify(EvalString("item.day | date", en)); got != "2 Jan 2026" {
		t.Errorf("en default: %q", got)
	}
	if got := Stringify(EvalString("item.nope | date:'D'", en)); got != "" {
		t.Errorf("missing date: %q", got)
	}
}

func TestExprMisc(t *testing.T) {
	env := vectorEnv(map[string]any{
		"post":  PostCtx{"title": "A", "id": "5"},
		"site":  map[string]string{"title": "S"},
		"item":  map[string]any{"n": -1234567.891, "s": "x", "zero": "0", "list": []any{}},
		"query": map[string]any{"q": "a b"},
	}, "en")
	cases := map[string]string{
		"post.id + 1":                         "6",
		"post.title + site.title":             "AS",
		"item.n | number":                     "-1,234,567.891",
		"item.n | number:0":                   "-1,234,568",
		"0.1 + 0.2":                           "0.3",
		"item.zero ? 'y' : 'n'":               "n",
		"item.list ? 'y' : 'n'":               "n",
		"!item.list":                          "true",
		"item.s | upper | default:'-'":        "X",
		"item.s | baht":                       "x",
		"(item.s | upper) + '!'":              "X!",
		"'a\\'b'":                             "a'b",
		"1 == '1'":                            "true",
		"'1' == '1.0'":                        "false",
		"true == 'true'":                      "false",
		"item.s && 'y'":                       "true",
		"item.missing || 0":                   "false",
		"'b' < 2":                             "false",
		"null == item.missing":                "true",
		"'10' < '9'":                          "false",
		"'b' > 'a'":                           "true",
		"2 > 'x'":                             "false",
		"'x' * 2":                             "",
		"true + 1":                            "2",
		"item.s.length":                       "1",
		"item.missing | default:0 | number:1": "0.0",
	}
	for src, want := range cases {
		if got := Stringify(EvalString(src, env)); got != want {
			t.Errorf("%q = %q, want %q", src, got, want)
		}
	}
	for _, bad := range []string{"a === b", "a, b", "{a}", "a.b(", "a.'x'", "a |", "a | b:", "a ? b", "(a", "a.1"} {
		if _, err := ParseExpr(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// script formatters through Env.Format; core names cannot be overridden
	env.Format = func(name string, v any, args []any) (string, bool) {
		if name == "baht" {
			return "฿" + Stringify(v) + Stringify(args[0]), true
		}
		return "", false
	}
	if got := Stringify(EvalString("item.s | baht:'!'", env)); got != "฿x!" {
		t.Errorf("script formatter: %q", got)
	}
	if got := Stringify(EvalString("item.s | upper", env)); got != "X" {
		t.Errorf("core formatter: %q", got)
	}
	// conditions
	if !Condition("", env) || Condition("item.missing", env) || !Condition("{{ item.s }}", env) || !Condition("bad ===", env) {
		t.Error("Condition")
	}
	// templates escape evaluated values only
	if got := ParseTemplate("<b>{{ item.s + '<' }}</b>").Render(env, func(s string) string { return strings.ReplaceAll(s, "<", "&lt;") }); got != "<b>x&lt;</b>" {
		t.Errorf("escape: %q", got)
	}
	if got := ParseTemplate("{{ '}}' }} {{ unclosed").Render(env, nil); got != "}} {{ unclosed" {
		t.Errorf("braces: %q", got)
	}
}

// TestExprVisitorVectors runs the phase-4a visitor vectors shared with the editor.
func TestExprVisitorVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/expr_vectors_visitor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Groups []struct {
			Name  string         `json:"name"`
			Ctx   map[string]any `json:"ctx"`
			Cases []struct {
				Expr   string `json:"expr"`
				Out    string `json:"out"`
				Truthy *bool  `json:"truthy"`
			} `json:"cases"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, g := range v.Groups {
		env := vectorEnv(g.Ctx, "en")
		for _, c := range g.Cases {
			n++
			e, err := ParseExpr(c.Expr)
			if err != nil {
				t.Errorf("%s: %q: %v", g.Name, c.Expr, err)
				continue
			}
			val := e.Eval(env)
			if got := Stringify(val); got != c.Out {
				t.Errorf("%s: %q = %q, want %q", g.Name, c.Expr, got, c.Out)
			}
			if c.Truthy != nil && Truthy(val) != *c.Truthy {
				t.Errorf("%s: %q truthy = %v, want %v", g.Name, c.Expr, Truthy(val), *c.Truthy)
			}
		}
	}
	if n < 28 {
		t.Fatalf("only %d cases", n)
	}
}
