// Package design implements the Figma-like document model (a tree of nodes
// with auto-layout, constraints-free absolute positioning, styles and
// per-breakpoint overrides) and renders it to semantic HTML + CSS.
package design

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Node types. Visual primitives mirror Figma; dynamic nodes mirror WordPress
// blocks (Query Loop, Navigation Menu, Post Content).
const (
	TypeFrame       = "frame"
	TypeText        = "text"
	TypeRect        = "rect"
	TypeImage       = "image"
	TypeButton      = "button"
	TypeMenu        = "menu"
	TypePostList    = "postList"
	TypePostContent = "postContent"
	TypeInstance    = "instance"
	TypeEmbed       = "embed"

	// A Query Loop over a collection provided by a plugin (built-in or service).
	TypeCollection = "collection"

	// A block contributed by a plugin (built-in or package), rendered through
	// the plugin block registry; and a file download button.
	TypePluginBlock = "pluginBlock"
	TypeDownload    = "download"

	// Where-am-I links (Home › Category › Title), built from the page context.
	TypeBreadcrumb = "breadcrumb"

	// Site Code (phase 3): page links of a collection, and a form field
	// (filters, action forms).
	TypePagination = "pagination"
	TypeInput      = "input"
)

// Breakpoints, widest first. Each narrower breakpoint inherits the overrides
// of the wider ones, exactly like CSS max-width media queries cascade.
var Breakpoints = []struct {
	Name     string
	MaxWidth int
}{
	{"tablet", 1024},
	{"mobile", 640},
}

// Size describes one axis: fixed pixels, fill the parent, or hug content.
type Size struct {
	Mode  string  `json:"mode"`
	Value float64 `json:"value,omitempty"`
}

// Node is one layer in the design tree. Layout, Style and Props are free-form
// maps so that breakpoint overrides can express "set to zero" unambiguously.
type Node struct {
	ID         string                    `json:"id"`
	Type       string                    `json:"type"`
	Name       string                    `json:"name,omitempty"`
	Visible    *bool                     `json:"visible,omitempty"`
	Locked     bool                      `json:"locked,omitempty"`
	Absolute   bool                      `json:"absolute,omitempty"` // floats inside an auto-layout parent (Figma "absolute position")
	X          float64                   `json:"x,omitempty"`
	Y          float64                   `json:"y,omitempty"`
	Width      Size                      `json:"width"`
	Height     Size                      `json:"height"`
	Layout     map[string]any            `json:"layout,omitempty"`
	Style      map[string]any            `json:"style,omitempty"`
	Props      map[string]any            `json:"props,omitempty"`
	Responsive map[string]map[string]any `json:"responsive,omitempty"`
	Children   []*Node                   `json:"children,omitempty"`
}

// Parse decodes a design document (the root node).
func Parse(raw []byte) (*Node, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty design")
	}
	var n Node
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

// IsVisible reports the node's visibility (default true).
func (n *Node) IsVisible() bool { return n.Visible == nil || *n.Visible }

// Effective returns a shallow copy of n with the overrides for bp (and every
// wider breakpoint) applied. bp "" or "desktop" returns n itself.
func (n *Node) Effective(bp string) *Node {
	if bp == "" || bp == "desktop" || len(n.Responsive) == 0 {
		return n
	}
	c := *n
	c.Layout = cloneMap(n.Layout)
	c.Style = cloneMap(n.Style)
	for _, b := range Breakpoints {
		if o, ok := n.Responsive[b.Name]; ok {
			c.apply(o)
		}
		if b.Name == bp {
			break
		}
	}
	return &c
}

func (n *Node) apply(o map[string]any) {
	for k, v := range o {
		switch k {
		case "x":
			n.X = num(v)
		case "y":
			n.Y = num(v)
		case "visible":
			b, _ := v.(bool)
			n.Visible = &b
		case "width", "height":
			var s Size
			if m, ok := v.(map[string]any); ok {
				s.Mode, _ = m["mode"].(string)
				s.Value = num(m["value"])
			}
			if k == "width" {
				n.Width = s
			} else {
				n.Height = s
			}
		case "layout":
			n.Layout = mergeMap(n.Layout, v)
		case "style":
			n.Style = mergeMap(n.Style, v)
		}
	}
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mergeMap(dst map[string]any, v any) map[string]any {
	src, ok := v.(map[string]any)
	if !ok {
		return dst
	}
	if dst == nil {
		dst = map[string]any{}
	}
	for k, val := range src {
		if val == nil {
			delete(dst, k)
		} else {
			dst[k] = val
		}
	}
	return dst
}

// --- small typed accessors over map[string]any -------------------------------

func num(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

func getNum(m map[string]any, k string) (float64, bool) {
	v, ok := m[k]
	if !ok || v == nil {
		return 0, false
	}
	return num(v), true
}

func getStr(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	if v, ok := m[k].(float64); ok {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func getBool(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}

func px(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64) + "px"
}

func fnum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// ClassName returns the CSS class used for a node id.
func ClassName(id string) string {
	var b strings.Builder
	b.WriteString("n-")
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
