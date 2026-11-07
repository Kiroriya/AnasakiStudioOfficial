package site

import (
	"encoding/json"
	"html"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/design"
)

// The fields of a custom type on its pages (2.5.0): the "Fields" block
// (design node postFields) lists a post's custom fields with their labels, so
// a menu item shows its price and a listing its bedrooms without a template
// made by hand. Values are formatted by the field's type and always escaped.

// RouteField is one custom field of a type, as the public site shows it.
type RouteField struct {
	Key     string
	Type    string
	Label   map[string]string // {th, en}
	Options []RouteOption     // select: value → label
	Hidden  bool              // hideOnSite: kept for the owner / the plugin (stock, cost …), not listed by the Fields block
	OnCards bool              // hideOnSite "page": listed on cards in lists only (a price the product page shows its own way)
}

// RouteOption is one choice of a select field.
type RouteOption struct {
	Value string
	Label map[string]string
}

func pickLang(m map[string]string, lang string) string {
	if v := m[lang]; v != "" {
		return v
	}
	if v := m["th"]; v != "" {
		return v
	}
	return m["en"]
}

// PostFields renders the custom fields of post (design.FieldsSource): one row
// per field with a value; max > 0 keeps the first max of them ("" when the
// post has none). card: the one-line layout of a card.
func (s *Source) PostFields(post design.PostCtx, max int, card bool) string {
	t := typeByID(s.types, post["type"])
	if t == nil || len(t.Fields) == 0 || post["meta"] == "" {
		return ""
	}
	var meta map[string]any
	if json.Unmarshal([]byte(post["meta"]), &meta) != nil {
		return ""
	}
	var b strings.Builder
	n := 0
	for _, f := range t.Fields {
		v, ok := meta[f.Key]
		if !ok || f.Hidden || f.OnCards && !card {
			continue
		}
		val := fieldValueHTML(f, v, s.lang, s.mediaURL)
		if val == "" {
			continue
		}
		b.WriteString(`<div class="rea-field" data-field="` + html.EscapeString(f.Key) + `"><dt>` +
			html.EscapeString(pickLang(f.Label, s.lang)) + `</dt><dd>` + val + `</dd></div>`)
		n++
		if max > 0 && n >= max {
			break
		}
	}
	if n == 0 {
		return ""
	}
	return `<dl class="rea-fields">` + b.String() + `</dl>`
}

// mediaURL is the address of media id ("" when it is gone).
func (s *Source) mediaURL(id int64) (url, name string) {
	if m := s.Media(id); m != nil {
		return m.URL, m.Filename
	}
	return "", ""
}

// fieldValueHTML formats one value as safe HTML ("" = nothing to show);
// media turns the ids of image, gallery and file fields into addresses.
func fieldValueHTML(f RouteField, v any, lang string, media func(int64) (string, string)) string {
	str := func(x any) string {
		switch y := x.(type) {
		case string:
			return strings.TrimSpace(y)
		case float64:
			return strconv.FormatFloat(y, 'f', -1, 64)
		case bool:
			if y {
				return "1"
			}
			return ""
		}
		return ""
	}
	esc := html.EscapeString
	switch f.Type {
	case "number":
		x, ok := v.(float64)
		if !ok {
			if p, err := strconv.ParseFloat(str(v), 64); err == nil {
				x, ok = p, true
			}
		}
		if !ok {
			return ""
		}
		return esc(FormatNumber(x))
	case "bool":
		b, _ := v.(bool)
		if !b {
			return ""
		}
		if lang == "en" {
			return "✓ Yes"
		}
		return "✓ ใช่"
	case "select":
		s := str(v)
		for _, o := range f.Options {
			if o.Value == s {
				return esc(pickLang(o.Label, lang))
			}
		}
		return esc(s)
	case "url":
		u := str(v)
		if !safeURL(u) {
			return ""
		}
		return `<a href="` + esc(u) + `" rel="nofollow noopener" target="_blank">` + esc(shortURL(u)) + `</a>`
	case "email":
		e := str(v)
		if e == "" || strings.ContainsAny(e, " <>\"'") {
			return ""
		}
		return `<a href="mailto:` + esc(e) + `">` + esc(e) + `</a>`
	case "image":
		u := str(v)
		if id, ok := v.(float64); ok && media != nil { // a media id
			u, _ = media(int64(id))
		}
		if !safeURL(u) {
			return ""
		}
		return `<img src="` + esc(u) + `" alt="" loading="lazy">`
	case "gallery":
		list, _ := v.([]any)
		var b strings.Builder
		for _, it := range list {
			u := str(it)
			if m, ok := it.(map[string]any); ok {
				u = str(m["url"])
			} else if id, ok := it.(float64); ok && media != nil { // gallery fields keep media ids
				u, _ = media(int64(id))
			}
			if safeURL(u) {
				b.WriteString(`<a href="` + esc(u) + `" target="_blank" rel="noopener"><img src="` + esc(u) + `" alt="" loading="lazy"></a>`)
			}
		}
		if b.Len() == 0 {
			return ""
		}
		return `<span class="rea-field-gallery">` + b.String() + `</span>`
	case "date", "datetime":
		s := str(v)
		for _, layout := range []string{"2006-01-02T15:04:05Z07:00", "2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil {
				out := FormatDate(t)
				if f.Type == "datetime" && (t.Hour() != 0 || t.Minute() != 0) {
					out += " " + t.Format("15:04")
				}
				return esc(out)
			}
		}
		return esc(s)
	case "color":
		c := str(v)
		if !colorRe(c) {
			return ""
		}
		return `<span class="rea-field-swatch" style="background:` + c + `"></span> ` + esc(c)
	case "text", "textarea":
		s := str(v)
		if s == "" {
			return ""
		}
		return strings.ReplaceAll(esc(s), "\n", "<br>")
	case "richtext":
		s := strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(str(v), " ")))
		return esc(strings.Join(strings.Fields(s), " "))
	case "file":
		var b []string
		ids, ok := v.([]any)
		if !ok {
			ids = []any{v}
		}
		for _, it := range ids {
			id, ok := it.(float64)
			if !ok || media == nil {
				continue
			}
			if u, name := media(int64(id)); safeURL(u) {
				b = append(b, `<a href="`+esc(u)+`" download>`+esc(orStr(name, shortURL(u)))+`</a>`)
			}
		}
		return strings.Join(b, "<br>")
	}
	return "" // relation, user, term, repeater: not listed (a template shows them)
}

// FormatNumber writes x with thousands separators (1,234,567.5).
func FormatNumber(x float64) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return ""
	}
	neg := x < 0
	if neg {
		x = -x
	}
	s := strconv.FormatFloat(x, 'f', -1, 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	out := b.String() + frac
	if neg {
		out = "-" + out
	}
	return out
}

func safeURL(u string) bool {
	if u == "" || len(u) > 2000 {
		return false
	}
	if strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") {
		return true
	}
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "https" || p.Scheme == "http") && p.Host != ""
}

func shortURL(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		s := p.Host + strings.TrimRight(p.Path, "/")
		if r := []rune(s); len(r) > 48 {
			s = string(r[:47]) + "…"
		}
		return s
	}
	return u
}

func colorRe(c string) bool {
	if len(c) != 4 && len(c) != 7 || c[0] != '#' {
		return false
	}
	for _, r := range c[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
