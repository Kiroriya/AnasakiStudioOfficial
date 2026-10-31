package site

import (
	"encoding/json"

	"github.com/anasaki/studio/backend/internal/design"
	"regexp"
	"strings"
)

// Theme is the site's design tokens (option "theme", JSON): brand colors and
// fonts set once and used everywhere. Designs refer to the colors as CSS
// variables, e.g. style.fill "var(--c-primary)", so changing a token restyles
// every page at once.
type Theme struct {
	Colors      map[string]string `json:"colors"`
	FontHeading string            `json:"fontHeading"`
	FontBody    string            `json:"fontBody"`
	// Palette holds extra named colors (Figma "color styles"): var(--c-<id>).
	Palette []PaletteColor `json:"palette,omitempty"`
	// TextStyles are shared text looks (H1, Body…): style.textStyle = id.
	TextStyles []TextStyle `json:"textStyles,omitempty"`
}

// PaletteColor is one extra named color.
type PaletteColor struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// TextStyle is one shared text look; Tablet/Mobile override Style.
type TextStyle struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Style  map[string]any `json:"style"`
	Tablet map[string]any `json:"tablet,omitempty"`
	Mobile map[string]any `json:"mobile,omitempty"`
}

// DefaultTextStyles are used until the site defines its own.
var DefaultTextStyles = []TextStyle{
	{ID: "h1", Name: "Heading 1", Style: map[string]any{"fontSize": 52.0, "fontWeight": 800.0, "lineHeight": 1.15, "letterSpacing": -0.5},
		Tablet: map[string]any{"fontSize": 42.0}, Mobile: map[string]any{"fontSize": 34.0}},
	{ID: "h2", Name: "Heading 2", Style: map[string]any{"fontSize": 38.0, "fontWeight": 800.0, "lineHeight": 1.2},
		Tablet: map[string]any{"fontSize": 32.0}, Mobile: map[string]any{"fontSize": 28.0}},
	{ID: "h3", Name: "Heading 3", Style: map[string]any{"fontSize": 24.0, "fontWeight": 700.0, "lineHeight": 1.3},
		Mobile: map[string]any{"fontSize": 21.0}},
	{ID: "lead", Name: "Lead", Style: map[string]any{"fontSize": 20.0, "fontWeight": 400.0, "lineHeight": 1.6},
		Mobile: map[string]any{"fontSize": 17.0}},
	{ID: "body", Name: "Body", Style: map[string]any{"fontSize": 17.0, "fontWeight": 400.0, "lineHeight": 1.65}},
	{ID: "small", Name: "Small", Style: map[string]any{"fontSize": 14.0, "fontWeight": 400.0, "lineHeight": 1.5}},
	{ID: "caption", Name: "Caption", Style: map[string]any{"fontSize": 12.0, "fontWeight": 700.0, "letterSpacing": 1.0, "textTransform": "uppercase"}},
}

// ThemeColorKeys are the color tokens, in display order (--c-<key>).
var ThemeColorKeys = []string{"primary", "secondary", "accent", "text", "muted", "surface"}

// DefaultThemeColors match the look of a site without a theme.
var DefaultThemeColors = map[string]string{
	"primary": "#4f46e5", "secondary": "#0ea5e9", "accent": "#f59e0b",
	"text": "#111827", "muted": "#6b7280", "surface": "#f8fafc",
}

var (
	themeColorRe = regexp.MustCompile(`(?i)^(#[0-9a-f]{3,8}|rgba?\([0-9.,\s%]+\))$`)
	themeFontRe  = regexp.MustCompile(`^[A-Za-z0-9 ]{1,40}$`)
	styleIDRe    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)
	transformRe  = regexp.MustCompile(`^(none|uppercase|lowercase|capitalize)$`)
	decorationRe = regexp.MustCompile(`^(none|underline|line-through)$`)
)

const maxStyles = 40

// cleanTextValues keeps only known text keys with values of the right type.
func cleanTextValues(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if !design.TextStyleKeys[k] {
			continue
		}
		switch k {
		case "fontFamily":
			if s, ok := v.(string); ok && themeFontRe.MatchString(s) {
				out[k] = s
			}
		case "textTransform":
			if s, ok := v.(string); ok && transformRe.MatchString(s) {
				out[k] = s
			}
		case "textDecoration":
			if s, ok := v.(string); ok && decorationRe.MatchString(s) {
				out[k] = s
			}
		case "italic":
			if b, ok := v.(bool); ok {
				out[k] = b
			}
		default:
			if f, ok := v.(float64); ok && f > -100 && f < 1000 {
				out[k] = f
			}
		}
	}
	return out
}

func cleanName(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 40 {
		s = string(r[:40])
	}
	return s
}

// ParseTheme reads the option value; invalid parts fall back to defaults.
func ParseTheme(raw string) Theme {
	var t Theme
	_ = json.Unmarshal([]byte(raw), &t)
	colors := make(map[string]string, len(ThemeColorKeys))
	for _, k := range ThemeColorKeys {
		colors[k] = DefaultThemeColors[k]
		if v := strings.TrimSpace(t.Colors[k]); themeColorRe.MatchString(v) {
			colors[k] = v
		}
	}
	t.Colors = colors
	if !themeFontRe.MatchString(t.FontHeading) {
		t.FontHeading = ""
	}
	if !themeFontRe.MatchString(t.FontBody) {
		t.FontBody = ""
	}
	pal := []PaletteColor{}
	seen := map[string]bool{}
	for _, c := range t.Palette {
		v := strings.TrimSpace(c.Value)
		if len(pal) >= maxStyles || !styleIDRe.MatchString(c.ID) || seen[c.ID] || DefaultThemeColors[c.ID] != "" || !themeColorRe.MatchString(v) {
			continue
		}
		seen[c.ID] = true
		pal = append(pal, PaletteColor{ID: c.ID, Name: cleanName(c.Name), Value: v})
	}
	t.Palette = pal
	if t.TextStyles == nil {
		t.TextStyles = DefaultTextStyles
	}
	ts := []TextStyle{}
	seen = map[string]bool{}
	for _, s := range t.TextStyles {
		if len(ts) >= maxStyles || !styleIDRe.MatchString(s.ID) || seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		ts = append(ts, TextStyle{ID: s.ID, Name: cleanName(s.Name), Style: cleanTextValues(s.Style),
			Tablet: cleanTextValues(s.Tablet), Mobile: cleanTextValues(s.Mobile)})
	}
	t.TextStyles = ts
	return t
}

// DesignTextStyles gives the text styles to the design renderer.
func (t Theme) DesignTextStyles() map[string]design.TextStyle {
	out := make(map[string]design.TextStyle, len(t.TextStyles))
	for _, s := range t.TextStyles {
		out[s.ID] = design.TextStyle{Base: s.Style, Tablet: s.Tablet, Mobile: s.Mobile}
	}
	return out
}

// ValidateTheme checks an incoming option value; "" = ok.
func ValidateTheme(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var t Theme
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return "ข้อมูลธีมไม่ถูกต้อง"
	}
	for k, v := range t.Colors {
		if DefaultThemeColors[k] == "" {
			return "ไม่รู้จักสีธีม: " + k
		}
		if v != "" && !themeColorRe.MatchString(strings.TrimSpace(v)) {
			return "รหัสสีไม่ถูกต้อง: " + v
		}
	}
	for _, f := range []string{t.FontHeading, t.FontBody} {
		if f != "" && !themeFontRe.MatchString(f) {
			return "ชื่อฟอนต์ไม่ถูกต้อง: " + f
		}
	}
	if len(t.Palette) > maxStyles || len(t.TextStyles) > maxStyles {
		return "สไตล์มากเกินไป (สูงสุด 40)"
	}
	for _, c := range t.Palette {
		if !styleIDRe.MatchString(c.ID) || DefaultThemeColors[c.ID] != "" {
			return "รหัสสีไม่ถูกต้อง: " + c.ID
		}
		if !themeColorRe.MatchString(strings.TrimSpace(c.Value)) {
			return "รหัสสีไม่ถูกต้อง: " + c.Value
		}
	}
	for _, s := range t.TextStyles {
		if !styleIDRe.MatchString(s.ID) {
			return "รหัสสไตล์ตัวอักษรไม่ถูกต้อง: " + s.ID
		}
	}
	return ""
}

// CSS returns the :root variables and the theme's font rules.
func (t Theme) CSS() string {
	var b strings.Builder
	b.WriteString(":root{")
	for i, k := range ThemeColorKeys {
		if i > 0 {
			b.WriteString(";")
		}
		b.WriteString("--c-" + k + ":" + t.Colors[k])
	}
	for _, c := range t.Palette {
		b.WriteString(";--c-" + c.ID + ":" + c.Value)
	}
	b.WriteString("}\nbody{color:var(--c-text)")
	if t.FontBody != "" {
		b.WriteString(";font-family:'" + t.FontBody + "','Noto Sans Thai',system-ui,sans-serif")
	}
	b.WriteString("}\n")
	if t.FontHeading != "" {
		b.WriteString("h1,h2,h3,h4,h5,h6{font-family:'" + t.FontHeading + "','Noto Sans Thai',system-ui,sans-serif}\n")
	}
	return b.String()
}

// Fonts lists the theme's font families (for the Google Fonts link).
func (t Theme) Fonts() []string {
	var out []string
	for _, f := range []string{t.FontHeading, t.FontBody} {
		if f != "" {
			out = append(out, f)
		}
	}
	for _, s := range t.TextStyles {
		if f, _ := s.Style["fontFamily"].(string); f != "" {
			out = append(out, f)
		}
	}
	return out
}
