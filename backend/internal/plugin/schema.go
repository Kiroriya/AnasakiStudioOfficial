package plugin

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// I18n is a label in Thai and English. In JSON it is {"th": "", "en": ""};
// a plain string is accepted on input and used for both languages.
type I18n struct {
	TH string `json:"th"`
	EN string `json:"en"`
}

// T returns a Thai + English label.
func T(th, en string) I18n { return I18n{TH: th, EN: en} }

// UnmarshalJSON accepts {"th","en"} or a plain string.
func (l *I18n) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		l.TH, l.EN = s, s
		return nil
	}
	type plain I18n
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*l = I18n(p)
	if l.TH == "" {
		l.TH = l.EN
	}
	if l.EN == "" {
		l.EN = l.TH
	}
	return nil
}

// Option is one choice of a select field.
type Option struct {
	Value string `json:"value"`
	Label I18n   `json:"label"`
}

// Field types of a settings / block props schema.
const (
	FieldText     = "text"
	FieldTextarea = "textarea"
	FieldNumber   = "number"
	FieldColor    = "color"
	FieldBoolean  = "boolean"
	FieldSelect   = "select"
	FieldURL      = "url"
	FieldImage    = "image"
)

var fieldTypes = map[string]bool{FieldText: true, FieldTextarea: true, FieldNumber: true, FieldColor: true,
	FieldBoolean: true, FieldSelect: true, FieldURL: true, FieldImage: true}

// Field is one entry of a settings schema (also used for block props).
type Field struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Label       I18n     `json:"label"`
	Help        *I18n    `json:"help,omitempty"`
	Default     any      `json:"default,omitempty"`
	Options     []Option `json:"options,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	// MaxLength caps text values (default 2000, textarea 20000, raw code 100000).
	MaxLength int `json:"maxLength,omitempty"`
}

var keyRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,39}$`)

// ValidateSchema checks a schema declared by a package manifest.
func ValidateSchema(fields []Field) error {
	if len(fields) > 60 {
		return fmt.Errorf("too many fields (max 60)")
	}
	seen := map[string]bool{}
	for _, f := range fields {
		if !keyRe.MatchString(f.Key) {
			return fmt.Errorf("invalid field key %q", f.Key)
		}
		if seen[f.Key] {
			return fmt.Errorf("duplicate field key %q", f.Key)
		}
		seen[f.Key] = true
		if !fieldTypes[f.Type] {
			return fmt.Errorf("field %q: unknown type %q", f.Key, f.Type)
		}
		if f.Type == FieldSelect && len(f.Options) == 0 {
			return fmt.Errorf("field %q: select needs options", f.Key)
		}
		if f.Default != nil {
			if _, err := f.coerce(f.Default); err != nil {
				return fmt.Errorf("field %q: default: %v", f.Key, err)
			}
		}
	}
	return nil
}

// Values are validated settings / props values keyed by field key.
type Values map[string]any

// Str returns a value as a string ("" when missing).
func (v Values) Str(k string) string { return ToString(v[k]) }

// Bool returns a boolean value.
func (v Values) Bool(k string) bool {
	switch t := v[k].(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	}
	return false
}

// Num returns a numeric value.
func (v Values) Num(k string) float64 {
	switch t := v[k].(type) {
	case float64:
		return t
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

// ToString formats a JSON-ish value for templates.
func ToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case json.Number:
		return t.String()
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// Defaults returns the schema's default values.
func Defaults(fields []Field) Values {
	out := Values{}
	for _, f := range fields {
		out[f.Key] = f.zero()
		if f.Default != nil {
			if v, err := f.coerce(f.Default); err == nil {
				out[f.Key] = v
			}
		}
	}
	return out
}

func (f Field) zero() any {
	switch f.Type {
	case FieldBoolean:
		return false
	case FieldNumber:
		return float64(0)
	case FieldSelect:
		if len(f.Options) > 0 {
			return f.Options[0].Value
		}
	}
	return ""
}

// ValidationError lists invalid fields (key → message).
type ValidationError map[string]string

func (e ValidationError) Error() string {
	parts := make([]string, 0, len(e))
	for k, m := range e {
		parts = append(parts, k+": "+m)
	}
	return "invalid settings: " + strings.Join(parts, "; ")
}

// Validate coerces in against the schema. Unknown keys are dropped, missing
// keys get their defaults; type mismatches are reported.
func Validate(fields []Field, in map[string]any) (Values, error) {
	out := Defaults(fields)
	bad := ValidationError{}
	for _, f := range fields {
		raw, ok := in[f.Key]
		if !ok || raw == nil {
			continue
		}
		v, err := f.coerce(raw)
		if err != nil {
			bad[f.Key] = err.Error()
			continue
		}
		out[f.Key] = v
	}
	if len(bad) > 0 {
		return out, bad
	}
	return out, nil
}

// Sanitize is Validate without errors: invalid values fall back to defaults
// (used when rendering stored block props).
func Sanitize(fields []Field, in map[string]any) Values {
	v, _ := Validate(fields, in)
	return v
}

var colorRe = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|(rgb|rgba|hsl|hsla)\([0-9.,%\s/]+\)|[a-zA-Z]{3,20}|transparent)$`)

func (f Field) coerce(raw any) (any, error) {
	switch f.Type {
	case FieldBoolean:
		switch t := raw.(type) {
		case bool:
			return t, nil
		case string:
			if t == "true" || t == "1" || t == "on" {
				return true, nil
			}
			if t == "false" || t == "0" || t == "" {
				return false, nil
			}
		case float64:
			return t != 0, nil
		}
		return nil, fmt.Errorf("must be true or false")
	case FieldNumber:
		var n float64
		switch t := raw.(type) {
		case float64:
			n = t
		case int:
			n = float64(t)
		case string:
			if strings.TrimSpace(t) == "" {
				return float64(0), nil
			}
			p, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
			if err != nil {
				return nil, fmt.Errorf("must be a number")
			}
			n = p
		default:
			return nil, fmt.Errorf("must be a number")
		}
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, fmt.Errorf("must be a number")
		}
		if f.Min != nil && n < *f.Min {
			return nil, fmt.Errorf("must be ≥ %v", *f.Min)
		}
		if f.Max != nil && n > *f.Max {
			return nil, fmt.Errorf("must be ≤ %v", *f.Max)
		}
		return n, nil
	}
	var s string
	switch t := raw.(type) {
	case string:
		s = t
	case float64, int, int64, bool:
		s = ToString(t)
	default:
		return nil, fmt.Errorf("must be a string")
	}
	max := f.MaxLength
	if max == 0 {
		max = 2000
		if f.Type == FieldTextarea {
			max = 20000
		}
	}
	if len(s) > max {
		return nil, fmt.Errorf("too long (max %d bytes)", max)
	}
	switch f.Type {
	case FieldColor:
		s = strings.TrimSpace(s)
		if s != "" && !colorRe.MatchString(s) {
			return nil, fmt.Errorf("must be a CSS color")
		}
	case FieldSelect:
		for _, o := range f.Options {
			if o.Value == s {
				return s, nil
			}
		}
		if s == "" {
			return f.zero(), nil
		}
		return nil, fmt.Errorf("must be one of the options")
	case FieldURL, FieldImage:
		s = strings.TrimSpace(s)
		if s != "" && !SafeURL(s) {
			return nil, fmt.Errorf("must be an http(s) URL or a path starting with /")
		}
	}
	return s, nil
}

// SafeURL accepts http(s) URLs, root-relative paths, #anchors, mailto: and tel:.
func SafeURL(u string) bool {
	l := strings.ToLower(strings.TrimSpace(u))
	if strings.ContainsAny(l, "\x00\r\n\t<>\"") {
		return false
	}
	switch {
	case strings.HasPrefix(l, "https://"), strings.HasPrefix(l, "http://"):
		return len(l) > 8
	case strings.HasPrefix(l, "//"):
		return false
	case strings.HasPrefix(l, "/"), strings.HasPrefix(l, "#"), strings.HasPrefix(l, "mailto:"), strings.HasPrefix(l, "tel:"):
		return true
	}
	return false
}
