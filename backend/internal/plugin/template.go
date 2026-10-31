package plugin

import (
	"html"
	"regexp"
	"strings"
)

// TemplateData is what package templates can read. Every placeholder
// ({{settings.k}}, {{props.k}}, {{site.k}}, {{post.k}}, {{item.k}}) is
// HTML-escaped; there is no raw output and no logic.
type TemplateData struct {
	Settings Values
	Props    Values
	Site     map[string]string
	Post     map[string]string
	Item     map[string]string
}

var placeholderRe = regexp.MustCompile(`\{\{\s*(settings|props|site|post|item)\.([a-zA-Z0-9_]+)\s*\}\}`)

// strayRe catches anything left that looks like a placeholder (unknown
// namespaces, broken syntax) so it never reaches the page verbatim.
var strayRe = regexp.MustCompile(`\{\{[^{}]{0,200}\}\}`)

// lookup returns the raw (unescaped) value of a placeholder.
func (d TemplateData) lookup(ns, key string) string {
	switch ns {
	case "settings":
		return ToString(d.Settings[key])
	case "props":
		return ToString(d.Props[key])
	case "site":
		return d.Site[key]
	case "post":
		return d.Post[key]
	case "item":
		return d.Item[key]
	}
	return ""
}

// RenderTemplate expands placeholders, escaping every value for HTML.
func RenderTemplate(tpl string, d TemplateData) string {
	if !strings.Contains(tpl, "{{") {
		return tpl
	}
	out := placeholderRe.ReplaceAllStringFunc(tpl, func(m string) string {
		p := placeholderRe.FindStringSubmatch(m)
		return html.EscapeString(d.lookup(p[1], p[2]))
	})
	return strayRe.ReplaceAllString(out, "")
}

// RenderText expands placeholders without escaping (webhook URLs). The
// result must be validated by the caller.
func RenderText(tpl string, d TemplateData) string {
	out := placeholderRe.ReplaceAllStringFunc(tpl, func(m string) string {
		p := placeholderRe.FindStringSubmatch(m)
		return d.lookup(p[1], p[2])
	})
	return strayRe.ReplaceAllString(out, "")
}
