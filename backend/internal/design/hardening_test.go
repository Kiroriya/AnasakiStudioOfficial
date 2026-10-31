package design

import (
	"strings"
	"testing"
)

func TestSanitizeRichSchemeBypasses(t *testing.T) {
	for _, href := range []string{
		"javascript:alert(1)", "JaVaScRiPt:alert(1)", "java\tscript:alert(1)", "java&#x09;script:alert(1)",
		"java&#9;script:alert(1)", "java&Tab;script:alert(1)", "java&NewLine;script:alert(1)",
		"&#x6A;avascript:alert(1)", "&#106;&#97;vascript:alert(1)", "&#x01;javascript:alert(1)",
		" \x01\x1fjavascript:alert(1)", "\njavascript:alert(1)", "javascript&colon;alert(1)",
		"vbscript:msgbox(1)", "data:text/html;base64,PHNjcmlwdD4=", "jav&#x0A;ascript:alert(1)",
	} {
		for _, q := range []string{`"`, `'`} {
			got := SanitizeRich(`<a href=` + q + href + q + `>l</a>`)
			if got != `<a href="#">l</a>` {
				t.Errorf("SanitizeRich(href=%q) = %q", href, got)
			}
		}
	}
	// ordinary links are untouched (entities re-escaped)
	if got := SanitizeRich(`<a href="/a?x=1&amp;y=2">l</a>`); got != `<a href="/a?x=1&amp;y=2">l</a>` {
		t.Errorf("plain link: %q", got)
	}
}

func TestSafeHrefSchemes(t *testing.T) {
	for _, u := range []string{"javascript:x", "JAVASCRIPT:x", "\x00javascript:x", "java\tscript:x", "java\r\nscript:x",
		" \fjavascript:x", "\x7fjavascript:x", "Data:text/html,x", "vbScript:x"} {
		if got := safeHref(u); got != "#" {
			t.Errorf("safeHref(%q) = %q", u, got)
		}
	}
	for _, u := range []string{"/about", "https://x.com/javascript:y", "mailto:a@b.c", "#top"} {
		if safeHref(u) != u {
			t.Errorf("safeHref(%q) changed a safe link", u)
		}
	}
	// a button link: entity-encoded schemes stay literal text (escaped on output)
	n := &Node{ID: "b", Type: TypeButton, Props: map[string]any{"text": "go", "href": "java&#x09;script:alert(1)"}}
	out := Render(&Node{ID: "r", Type: TypeFrame, Children: []*Node{n}}, Context{}, nil).HTML
	if !strings.Contains(out, `href="java&amp;#x09;script:alert(1)"`) {
		t.Errorf("button href not escaped: %s", out)
	}
	n.Props["href"] = "  JaVa\tScRiPt:alert(1)"
	out = Render(&Node{ID: "r", Type: TypeFrame, Children: []*Node{n}}, Context{}, nil).HTML
	if !strings.Contains(out, `href="#"`) {
		t.Errorf("javascript: button link kept: %s", out)
	}
}

func TestSafeCSSValues(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"#ff0000", "#ff0000"},
		{"rgba(0,0,0,.5)", "rgba(0,0,0,.5)"},
		{"linear-gradient(90deg, var(--c-primary), #fff)", "linear-gradient(90deg, var(--c-primary), #fff)"},
		{"0 4px 12px rgba(0,0,0,.2)", "0 4px 12px rgba(0,0,0,.2)"},
		{"red;}body{display:none", "redbodydisplay:none"},
		{`red "`, "red "},
		{"red '", "red "},
		{"red\n}", "red"},
		{"url(https://evil.example/x)", ""},
		{"URL (x)", ""},
		{"image-set(x 1x)", ""},
		{"expression(alert(1))", ""},
		{"red /* x", ""},
		{"rgb(1,2,3", ""},
		{"rgb(1,2,3))", ""},
		{"red\\3b", "red3b"},
	} {
		if got := safeCSS(c.in); got != c.want {
			t.Errorf("safeCSS(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStyleValuesCannotBreakOut(t *testing.T) {
	evil := []string{`x'; } body { display:none } .a{ a:'`, "red;}body{display:none}", `red "`, "rgb(1,2,3",
		"red /*", "url(https://evil.example/x)"}
	for _, v := range evil {
		n := &Node{ID: "n", Type: TypeImage, Width: Size{Mode: "fill"}, Props: map[string]any{"src": "/a.png"},
			Style: map[string]any{"fill": v, "color": v, "shadow": v, "textShadow": v, "stroke": v, "strokeWidth": float64(1),
				"fontFamily": v, "objectFit": v, "overlay": v, "backgroundImage": `x') } body{display:none} .a{b:url('`,
				"textGradient": v}}
		root := &Node{ID: "r", Type: TypeFrame, Children: []*Node{n}}
		out := Render(root, Context{}, nil)
		for _, bad := range []string{"body{", "body {", "/*", "url(https", `"`} {
			if strings.Contains(out.CSS, bad) {
				t.Errorf("value %q: %q in CSS:\n%s", v, bad, out.CSS)
			}
		}
		// every rule of the node stays a single rule: braces balance per line
		for _, line := range strings.Split(out.CSS, "\n") {
			if strings.Count(line, "{") != strings.Count(line, "}") || strings.Count(line, "'")%2 != 0 ||
				strings.Count(line, "(") != strings.Count(line, ")") {
				t.Errorf("value %q: unbalanced CSS line %q", v, line)
			}
		}
		for _, f := range out.Fonts {
			if strings.ContainsAny(f, `'";{}()<>\`) {
				t.Errorf("unsafe font name %q", f)
			}
		}
	}
}

func TestFontName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Noto Sans Thai", "Noto Sans Thai"}, {"IBM Plex Sans", "IBM Plex Sans"}, {"Kanit", "Kanit"},
		{`Inter', sans-serif; } body{x:y} '`, "Inter sans-serif  bodyxy"}, {"M PLUS 1p", "M PLUS 1p"},
	} {
		if got := FontName(c.in); got != c.want {
			t.Errorf("FontName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
