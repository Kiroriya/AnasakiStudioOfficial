package design

import (
	"html"
	"regexp"
	"strings"
)

// Rich text (props.rich on text nodes) is a small inline HTML subset written
// by the designer's text toolbar: bold, italic, underline, strike, line
// breaks, links and colored spans. SanitizeRich keeps exactly that subset.

var richTagRe = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9]*)([^<>]*)>`)

// richTags are the allowed elements; a and span keep a few attributes.
var richTags = map[string]bool{"b": true, "strong": true, "i": true, "em": true, "u": true, "s": true,
	"br": true, "span": true, "a": true, "mark": true, "sup": true, "sub": true}

var (
	richHrefRe   = regexp.MustCompile(`(?i)\bhref\s*=\s*("([^"]*)"|'([^']*)')`)
	richStyleRe  = regexp.MustCompile(`(?i)\bstyle\s*=\s*("([^"]*)"|'([^']*)')`)
	richTargetRe = regexp.MustCompile(`(?i)\btarget\s*=\s*["']?_blank`)
	// color / background-color / font-weight declarations with plain values
	// (hex, rgb(), named colors, theme var(--c-*)).
	richDeclRe = regexp.MustCompile(`(?i)^\s*(color|background-color|font-weight)\s*:\s*(#[0-9a-f]{3,8}|rgba?\([0-9.,\s%]+\)|var\(--c-[a-z0-9-]+\)|[a-z]+|[1-9]00)\s*$`)
)

// SanitizeRich returns s with only the allowed inline tags and attributes.
// Text between tags is kept as is, except that a stray "<" is escaped.
func SanitizeRich(s string) string {
	var b strings.Builder
	open := []string{}
	last := 0
	for _, m := range richTagRe.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(strings.ReplaceAll(s[last:m[0]], "<", "&lt;"))
		last = m[1]
		closing := s[m[2]:m[3]] == "/"
		tag := strings.ToLower(s[m[4]:m[5]])
		attrs := s[m[6]:m[7]]
		if !richTags[tag] {
			continue
		}
		if tag == "br" {
			if !closing {
				b.WriteString("<br>")
			}
			continue
		}
		if closing {
			// close only a tag that is open (and everything opened after it)
			for i := len(open) - 1; i >= 0; i-- {
				if open[i] == tag {
					for j := len(open) - 1; j >= i; j-- {
						b.WriteString("</" + open[j] + ">")
					}
					open = open[:i]
					break
				}
			}
			continue
		}
		b.WriteString("<" + tag + richAttrs(tag, attrs) + ">")
		open = append(open, tag)
	}
	b.WriteString(strings.ReplaceAll(s[last:], "<", "&lt;"))
	for i := len(open) - 1; i >= 0; i-- {
		b.WriteString("</" + open[i] + ">")
	}
	return b.String()
}

func richAttrs(tag, attrs string) string {
	var out strings.Builder
	if tag == "a" {
		if m := richHrefRe.FindStringSubmatch(attrs); m != nil {
			href := html.UnescapeString(m[2] + m[3])
			out.WriteString(` href="` + html.EscapeString(safeHref(href)) + `"`)
		}
		if richTargetRe.MatchString(attrs) {
			out.WriteString(` target="_blank" rel="noopener"`)
		}
	}
	if m := richStyleRe.FindStringSubmatch(attrs); m != nil {
		var keep []string
		for _, d := range strings.Split(html.UnescapeString(m[2]+m[3]), ";") {
			if dm := richDeclRe.FindStringSubmatch(d); dm != nil {
				keep = append(keep, strings.ToLower(dm[1])+":"+dm[2])
			}
		}
		if len(keep) > 0 {
			out.WriteString(` style="` + html.EscapeString(strings.Join(keep, ";")) + `"`)
		}
	}
	return out.String()
}
