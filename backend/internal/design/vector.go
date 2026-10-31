package design

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// TypeVector is a free-form vector path drawn with the pen tool or made by a
// boolean operation / flatten in the designer. props.path is SVG path data
// in the coordinates of props.viewBox ("0 0 w h"); the drawing stretches with
// the layer's box (preserveAspectRatio="none") while the stroke keeps its
// width (vector-effect="non-scaling-stroke"). Fill and stroke come from
// style.fill / style.stroke and are painted into the SVG, never as the CSS
// background / border of the box (see Decls).
//
// Mirrors vectorSvg() in frontend/src/app/editor/vector.ts.
const TypeVector = "vector"

// MaxVectorPath caps the size of props.path (boolean results of many curves
// are long, but a page should never carry megabytes of path data).
const MaxVectorPath = 60000

var (
	vectorPathRe  = regexp.MustCompile(`^[MmLlHhVvCcSsQqTtAaZz0-9.,\-\s eE+]*$`)
	vectorPaintRe = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|(?:rgb|rgba|hsl|hsla)\([0-9.,%\s/+\-]{1,60}\)|[a-zA-Z]{1,30}|var\(--[A-Za-z0-9_\-]{1,60}\))$`)
)

// ValidVectorPath reports whether d is acceptable SVG path data: only path
// commands, numbers and separators, starting with a move, length-capped.
func ValidVectorPath(d string) bool {
	d = strings.TrimSpace(d)
	if d == "" || len(d) > MaxVectorPath || !vectorPathRe.MatchString(d) {
		return false
	}
	return d[0] == 'M' || d[0] == 'm'
}

// VectorPaint validates a fill / stroke colour for the SVG ("" when it is
// not a plain colour: gradients and anything else are not painted).
func VectorPaint(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || !vectorPaintRe.MatchString(v) {
		return ""
	}
	return v
}

// vectorViewBox returns a normalized "x y w h" (w, h > 0) or "" when invalid.
func vectorViewBox(v string) string {
	f := strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == ',' })
	if len(f) != 4 {
		return ""
	}
	var n [4]float64
	for i, s := range f {
		x, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > 1e6 {
			return ""
		}
		n[i] = x
	}
	if n[2] <= 0 || n[3] <= 0 {
		return ""
	}
	return fnum(n[0]) + " " + fnum(n[1]) + " " + fnum(n[2]) + " " + fnum(n[3])
}

// VectorSVG is the <svg> of a vector node (cls is the class list). An
// invalid path renders an empty svg so the box keeps its place.
func VectorSVG(n *Node, cls string) string {
	vb := vectorViewBox(getStr(n.Props, "viewBox"))
	if vb == "" {
		w, h := n.Width.Value, n.Height.Value
		if n.Width.Mode != "fixed" || w <= 0 {
			w = 100
		}
		if n.Height.Mode != "fixed" || h <= 0 {
			h = 100
		}
		vb = "0 0 " + fnum(w) + " " + fnum(h)
	}
	open := `<svg class="` + cls + `" viewBox="` + vb + `" preserveAspectRatio="none" overflow="visible"` + a11y(n) + `>`
	d := strings.TrimSpace(getStr(n.Props, "path"))
	if !ValidVectorPath(d) {
		return open + `</svg>`
	}
	return open + `<path d="` + attr(d) + `"` + VectorPaintAttrs(n) + `/></svg>`
}

// VectorPaintAttrs are the paint attributes of a vector's <path>. Colours go
// in a style attribute so theme colours (var(--c-primary)) work.
func VectorPaintAttrs(n *Node) string {
	fill := VectorPaint(getStr(n.Style, "fill"))
	if fill == "" {
		fill = "none"
	}
	stroke := VectorPaint(getStr(n.Style, "stroke"))
	sw, _ := getNum(n.Style, "strokeWidth")
	if stroke == "" || sw <= 0 {
		stroke, sw = "none", 0
	}
	sw = math.Min(sw, 200)
	var b strings.Builder
	b.WriteString(` style="fill:` + attr(fill) + `;stroke:` + attr(stroke) + `"`)
	if sw > 0 {
		b.WriteString(` stroke-width="` + fnum(sw) + `"`)
		c := getStr(n.Props, "strokeCap")
		if c != "round" && c != "square" && c != "butt" {
			c = ""
		}
		if c == "" && getStr(n.Style, "strokeStyle") == "dotted" {
			c = "round" // dots need round caps
		}
		if c != "" {
			b.WriteString(` stroke-linecap="` + c + `"`)
		}
		if j := getStr(n.Props, "strokeJoin"); j == "round" || j == "bevel" || j == "miter" {
			b.WriteString(` stroke-linejoin="` + j + `"`)
		}
		if st := getStr(n.Style, "strokeStyle"); st == "dashed" {
			b.WriteString(` stroke-dasharray="` + fnum(sw*3) + ` ` + fnum(sw*2) + `"`)
		} else if st == "dotted" {
			b.WriteString(` stroke-dasharray="0 ` + fnum(sw*2) + `"`)
		}
	}
	if getStr(n.Props, "fillRule") == "evenodd" {
		b.WriteString(` fill-rule="evenodd"`)
	}
	b.WriteString(` vector-effect="non-scaling-stroke"`)
	return b.String()
}
