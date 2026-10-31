package design

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Decl is one CSS declaration. Order is preserved for readable output.
type Decl struct{ Prop, Value string }

// Decls computes the CSS declarations for a node given its parent's layout
// mode ("none", "horizontal", "vertical"; "" for the root).
//
// IMPORTANT: frontend/src/app/editor/design-css.ts mirrors this function so
// the editor canvas is pixel-identical to the published page. Keep in sync.
func Decls(n *Node, parentMode string) []Decl {
	var d []Decl
	add := func(p, v string) {
		for i := range d {
			if d[i].Prop == p {
				d[i].Value = v
				return
			}
		}
		d = append(d, Decl{p, v})
	}

	lm := getStr(n.Layout, "mode")
	if lm == "" {
		lm = "none"
	}
	maxW, hasMaxW := getNum(n.Style, "maxWidth")
	fixedMin := 0.0 // fixed height rendered as min-height (see growsWithContent)

	// Position
	if n.Absolute && parentMode != "" {
		parentMode = "none" // ignore the parent's auto layout, like Figma
	}
	if parentMode == "none" {
		add("position", "absolute")
		add("left", anchoredLeft(n.X, getStr(n.Style, "anchorX"), num(n.Style["anchorRef"])))
		add("top", px(n.Y))
	} else if parentMode != "" && getBool(n.Style, "sticky") {
		add("position", "sticky") // stays at the top while the page scrolls (headers)
		add("top", "0")
		add("z-index", "50")
	} else {
		add("position", "relative")
	}

	// Width
	switch n.Width.Mode {
	case "fixed":
		add("width", px(n.Width.Value))
		if parentMode == "horizontal" {
			add("flex-shrink", "0")
		}
		if parentMode != "none" {
			add("max-width", "100%")
		}
	case "fill":
		switch {
		case parentMode == "horizontal":
			add("flex-grow", "1")
			add("flex-basis", "0")
			add("min-width", "0")
		case parentMode == "vertical" && !hasMaxW:
			add("align-self", "stretch")
		default:
			add("width", "100%")
		}
	case "hug":
		switch parentMode {
		case "vertical":
			add("width", "fit-content")
		case "none":
			add("width", "max-content")
		}
	}

	// Height
	switch n.Height.Mode {
	case "fixed":
		if growsWithContent(n, lm) {
			// a fixed-height auto-layout frame grows when its content needs more
			// room (cards stacking on a phone) instead of spilling over the next
			// section; "Clip content" keeps the exact height
			fixedMin = n.Height.Value
			add("min-height", px(fixedMin))
		} else {
			add("height", px(n.Height.Value))
		}
		if parentMode == "vertical" {
			add("flex-shrink", "0")
		}
	case "fill":
		switch parentMode {
		case "vertical":
			add("flex-grow", "1")
			add("flex-basis", "0")
			add("min-height", "0")
		case "horizontal":
			add("align-self", "stretch")
		default:
			add("height", "100%")
		}
	}

	if hasMaxW && maxW > 0 {
		add("max-width", px(maxW))
		if parentMode != "none" && parentMode != "horizontal" {
			add("margin-left", "auto")
			add("margin-right", "auto")
		}
	}
	if v, ok := getNum(n.Style, "minHeight"); ok && v > fixedMin {
		add("min-height", px(v))
	}

	// Auto layout
	switch n.Type {
	case TypeButton, TypeDownload:
		add("display", "inline-flex")
		add("align-items", "center")
		add("justify-content", "center")
		add("text-decoration", "none")
		add("cursor", "pointer")
	case TypeImage:
		add("display", "block")
		fit := getStr(n.Style, "objectFit")
		if !objectFits[fit] {
			fit = "cover"
		}
		add("object-fit", fit)
		if p := focal(n.Style); p != "center" {
			add("object-position", p)
		}
	case TypeText:
		add("white-space", "pre-wrap")
		add("margin", "0")
	case TypeIcon, TypeShape, TypeVector:
		add("display", "block")
	}
	if cols := Columns(n); cols > 0 {
		// Horizontal layout with a column count: an even grid (cards, galleries).
		add("display", "grid")
		add("grid-template-columns", "repeat("+fnum(float64(cols))+",minmax(0,1fr))")
		if g, ok := getNum(n.Layout, "gap"); ok && g != 0 {
			add("gap", px(g))
		}
		if a := alignMap[getStr(n.Layout, "align")]; a != "" {
			add("align-items", a)
		}
	} else if lm == "horizontal" || lm == "vertical" {
		add("display", "flex")
		if lm == "horizontal" {
			add("flex-direction", "row")
		} else {
			add("flex-direction", "column")
		}
		if g, ok := getNum(n.Layout, "gap"); ok && g != 0 {
			add("gap", px(g))
		}
		if a := alignMap[getStr(n.Layout, "align")]; a != "" {
			add("align-items", a)
		}
		if j := justifyMap[getStr(n.Layout, "justify")]; j != "" {
			add("justify-content", j)
		}
		if getBool(n.Layout, "wrap") {
			add("flex-wrap", "wrap")
		}
	}
	if p, ok := n.Layout["padding"].([]any); ok && len(p) == 4 {
		if num(p[0])+num(p[1])+num(p[2])+num(p[3]) != 0 {
			add("padding", px(num(p[0]))+" "+px(num(p[1]))+" "+px(num(p[2]))+" "+px(num(p[3])))
		}
	}

	// Fill, stroke, effects
	s := n.Style
	if v := getStr(s, "fill"); v != "" {
		if n.Type == TypeVector {
			// painted into the svg path (VectorSVG), not the box
		} else if n.Type == TypeShape {
			add("color", safeCSS(v)) // a shape is painted with currentColor
		} else {
			add("background", safeCSS(v))
		}
	}
	if v := getStr(s, "backgroundImage"); v != "" {
		add("background-image", "url('"+safeURL(v)+"')")
		add("background-size", "cover")
		add("background-position", focal(n.Style))
	}
	if o := getStr(s, "overlay"); o != "" && getStr(s, "backgroundImage") != "" {
		o = safeCSS(o) // a colour laid over the background image (readable text on photos)
		add("background-image", "linear-gradient("+o+","+o+"),url('"+safeURL(getStr(s, "backgroundImage"))+"')")
	}
	if w, _ := getNum(s, "strokeWidth"); w > 0 && getStr(s, "stroke") != "" && n.Type != TypeVector {
		style := "solid"
		if v := getStr(s, "strokeStyle"); v == "dashed" || v == "dotted" {
			style = v
		}
		add("border", px(w)+" "+style+" "+safeCSS(getStr(s, "stroke")))
	}
	if r, ok := getNum(s, "radius"); ok && r != 0 {
		add("border-radius", px(r))
	}
	if rs, ok := s["radii"].([]any); ok && len(rs) == 4 && num(rs[0])+num(rs[1])+num(rs[2])+num(rs[3]) != 0 {
		add("border-radius", px(num(rs[0]))+" "+px(num(rs[1]))+" "+px(num(rs[2]))+" "+px(num(rs[3])))
	}
	if o, ok := getNum(s, "opacity"); ok && o < 1 {
		add("opacity", fnum(o))
	}
	if v := getStr(s, "shadow"); v != "" {
		add("box-shadow", safeCSS(v))
	}
	if getBool(s, "clip") {
		add("overflow", "hidden")
	}
	if f := filterValue(s); f != "" {
		add("filter", f)
	}
	if b, ok := getNum(s, "backdropBlur"); ok && b > 0 { // frosted glass
		add("backdrop-filter", "blur("+px(b)+")")
		add("-webkit-backdrop-filter", "blur("+px(b)+")")
	}

	// Typography (inherited by descendants, like CSS)
	if v := getStr(s, "color"); v != "" {
		add("color", safeCSS(v))
	}
	if v, ok := getNum(s, "fontSize"); ok && v > 0 {
		add("font-size", px(v))
	}
	if v, ok := getNum(s, "fontWeight"); ok && v > 0 {
		add("font-weight", fnum(v))
	}
	if v := getStr(s, "fontFamily"); v != "" {
		if f := FontName(v); f != "" {
			add("font-family", "'"+f+"', sans-serif")
		}
	}
	if v, ok := getNum(s, "lineHeight"); ok && v > 0 {
		add("line-height", fnum(v))
	}
	if v, ok := getNum(s, "letterSpacing"); ok && v != 0 {
		add("letter-spacing", px(v))
	}
	if v := getStr(s, "textAlign"); v != "" {
		add("text-align", safeCSS(v))
	}
	if v := getStr(s, "textTransform"); v != "" {
		add("text-transform", safeCSS(v))
	}
	if u := MaskURL(getStr(s, "mask")); u != "" { // clip to a shape (Figma mask)
		add("-webkit-mask", u+" center/100% 100% no-repeat")
		add("mask", u+" center/100% 100% no-repeat")
	}
	if getBool(s, "italic") {
		add("font-style", "italic")
	}
	if v := getStr(s, "textDecoration"); v == "underline" || v == "line-through" {
		add("text-decoration", v)
	}
	if v := getStr(s, "textShadow"); v != "" {
		add("text-shadow", safeCSS(v))
	}
	if g := getStr(s, "textGradient"); g != "" { // gradient painted into the letters
		add("background", safeCSS(g))
		add("-webkit-background-clip", "text")
		add("background-clip", "text")
		add("-webkit-text-fill-color", "transparent")
	}
	if a, ok := s["anim"].(map[string]any); ok && animTypes[getStr(a, "type")] {
		if v, ok := getNum(a, "duration"); ok && v > 0 {
			add("--anim-dur", fnum(v)+"ms")
		}
		if v, ok := getNum(a, "delay"); ok && v > 0 {
			add("--anim-delay", fnum(v)+"ms")
		}
	}
	if h, ok := s["hover"].(map[string]any); ok && len(h) > 0 {
		ms := 250.0
		if v, ok := getNum(h, "duration"); ok && v >= 0 {
			ms = v
		}
		t := fnum(ms) + "ms ease"
		add("transition", "transform "+t+",box-shadow "+t+",background "+t+",color "+t+",opacity "+t+",border-color "+t)
	}

	if tf := baseTransform(n); len(tf) > 0 {
		add("transform", strings.Join(tf, " "))
	}
	if !n.IsVisible() {
		add("display", "none")
	}
	return d
}

// Columns is the grid column count of a horizontal auto-layout node
// (layout.columns, 1-12), or 0 when it lays out as a normal row.
func Columns(n *Node) int {
	if getStr(n.Layout, "mode") != "horizontal" {
		return 0
	}
	c, _ := getNum(n.Layout, "columns")
	if c < 1 {
		return 0
	}
	return int(min(c, 12))
}

var alignMap = map[string]string{"start": "flex-start", "center": "center", "end": "flex-end", "stretch": "stretch"}
var justifyMap = map[string]string{"start": "flex-start", "center": "center", "end": "flex-end", "between": "space-between"}

var objectFits = map[string]bool{"cover": true, "contain": true, "fill": true, "none": true, "scale-down": true}

var (
	// characters that end a declaration, rule or <style>, open a string (an
	// unterminated one swallows the rest of the line) or escape
	cssUnsafe = regexp.MustCompile(`[;{}<>\\"'` + "`" + `\x00-\x1f\x7f]`)
	// comments (an unterminated one swallows the rest of the stylesheet) and
	// functions that fetch resources or run script in old browsers
	cssUnsafeFunc = regexp.MustCompile(`(?i)/\*|\*/|(url|image|image-set|cross-fade|element|src|expression)\s*\(`)
)

// safeCSS makes a style value (colour, gradient, shadow…) safe inside a
// declaration: characters that could break out of it are stripped, and a
// value that would still escape — a comment, unbalanced parentheses (an
// open "(" swallows the following rules) or a resource-loading function —
// becomes "" (an ignored declaration).
func safeCSS(v string) string {
	v = cssUnsafe.ReplaceAllString(v, "")
	if cssUnsafeFunc.MatchString(v) {
		return ""
	}
	depth := 0
	for _, c := range v {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth < 0 {
			return ""
		}
	}
	if depth != 0 {
		return ""
	}
	return v
}

// FontName returns a font family name reduced to letters, digits, spaces,
// "-", "_" and "." (safe inside '…' in CSS and in a Google Fonts URL).
func FontName(v string) string {
	f := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return -1
	}, v)
	f = strings.TrimSpace(f)
	if len(f) > 80 {
		f = f[:80]
		for !utf8.ValidString(f) {
			f = f[:len(f)-1]
		}
	}
	return f
}

// safeURL makes a value safe inside url('…') in a <style> block: quotes,
// brackets and angle brackets are percent-encoded so it can end neither the
// url() nor the <style> element, and control characters are dropped.
func safeURL(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch {
		case r < 0x20 || r == 0x7f || r == '\\':
		case strings.ContainsRune(`'"()<> {};`, r):
			fmt.Fprintf(&b, "%%%02X", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// diff returns the declarations needed to turn `from` into `to`: changed or
// added props, plus "unset" for props that `to` no longer sets.
func diff(from, to []Decl) []Decl {
	fm := map[string]string{}
	for _, d := range from {
		fm[d.Prop] = d.Value
	}
	tm := map[string]bool{}
	var out []Decl
	for _, d := range to {
		tm[d.Prop] = true
		if fm[d.Prop] != d.Value {
			out = append(out, d)
		}
	}
	for _, d := range from {
		if !tm[d.Prop] {
			out = append(out, Decl{d.Prop, unsetValue(d.Prop)})
		}
	}
	return out
}

func unsetValue(prop string) string {
	switch prop {
	case "position":
		return "relative"
	case "display":
		return "revert"
	}
	return "unset"
}

func writeRule(b *strings.Builder, selector string, decls []Decl) {
	if len(decls) == 0 {
		return
	}
	b.WriteString(selector)
	b.WriteString("{")
	for i, d := range decls {
		if i > 0 {
			b.WriteString(";")
		}
		b.WriteString(d.Prop)
		b.WriteString(":")
		b.WriteString(d.Value)
	}
	b.WriteString("}\n")
}

// anchoredLeft is the CSS left of a freely placed layer: x is measured from
// the parent's left edge in a parent that was ref px wide in the designer;
// the anchor keeps the distance to the parent's centre / right edge (or the
// fraction, "scale") on wider or narrower pages. Mirrors anchoredLeft() in
// frontend/src/app/editor/design-css.ts.
func anchoredLeft(x float64, anchor string, ref float64) string {
	round := func(v float64) float64 { return math.Round(v*100) / 100 }
	if ref <= 0 {
		return px(round(x))
	}
	off := func(base string, v float64) string {
		v = round(v)
		switch {
		case v == 0:
			return base
		case v < 0:
			return "calc(" + base + " - " + px(-v) + ")"
		}
		return "calc(" + base + " + " + px(v) + ")"
	}
	switch anchor {
	case "center":
		return off("50%", x-ref/2)
	case "right":
		return off("100%", x-ref)
	case "scale":
		return fnum(math.Round(x/ref*10000)/100) + "%"
	}
	return px(round(x))
}

// growsWithContent: a fixed height on an auto-layout container is a minimum,
// so content that wraps or stacks on smaller screens is never cut off or
// overlapped. Freeform frames (absolute children) and "Clip content" keep
// the exact height. Mirrors growsWithContent() in design-css.ts.
func growsWithContent(n *Node, layoutMode string) bool {
	switch n.Type {
	case "frame", "postList", "collection":
	default:
		return false
	}
	return layoutMode != "none" && !getBool(n.Style, "clip")
}

// filterValue combines the layer blur and the image adjustments into a CSS
// filter ("" when nothing is set). Mirrors filterValue() in design-css.ts.
func filterValue(s map[string]any) string {
	var parts []string
	if v, ok := getNum(s, "blur"); ok && v > 0 {
		parts = append(parts, "blur("+px(v)+")")
	}
	if v, ok := getNum(s, "grayscale"); ok && v > 0 {
		parts = append(parts, "grayscale("+fnum(min(v, 100))+"%)")
	}
	for _, k := range []string{"brightness", "contrast", "saturate"} {
		if v, ok := getNum(s, k); ok && v >= 0 && v != 100 {
			parts = append(parts, k+"("+fnum(min(v, 300))+"%)")
		}
	}
	return strings.Join(parts, " ")
}

// animTypes are the scroll-in animations (see AnimCSS).
var animTypes = map[string]bool{"fade": true, "up": true, "down": true, "left": true, "right": true, "zoom": true}

// AnimClasses are the extra classes of a node with a scroll-in animation
// ("" without one), e.g. " rea-anim rea-anim-up".
func AnimClasses(n *Node) string {
	if a, ok := n.Style["anim"].(map[string]any); ok && animTypes[getStr(a, "type")] {
		return " rea-anim rea-anim-" + getStr(a, "type")
	}
	return ""
}

// HoverDecls are the declarations of a node's :hover rule (mouse over).
// Mirrors hoverCss() in design-css.ts.
func HoverDecls(n *Node) []Decl {
	h, ok := n.Style["hover"].(map[string]any)
	if !ok || len(h) == 0 {
		return nil
	}
	var d []Decl
	tf := baseTransform(n) // keep the layer's own rotation / flip
	if v, ok := getNum(h, "lift"); ok && v != 0 {
		tf = append(tf, "translateY("+px(-v)+")")
	}
	if v, ok := getNum(h, "scale"); ok && v > 0 && v != 1 {
		tf = append(tf, "scale("+fnum(v)+")")
	}
	if len(tf) > 0 {
		d = append(d, Decl{"transform", strings.Join(tf, " ")})
	}
	if v := getStr(h, "fill"); v != "" {
		d = append(d, Decl{"background", safeCSS(v)})
	}
	if v := getStr(h, "color"); v != "" {
		d = append(d, Decl{"color", safeCSS(v)})
	}
	if v := getStr(h, "stroke"); v != "" {
		d = append(d, Decl{"border-color", safeCSS(v)})
	}
	if v := getStr(h, "shadow"); v != "" {
		d = append(d, Decl{"box-shadow", safeCSS(v)})
	}
	if v, ok := getNum(h, "opacity"); ok && v >= 0 && v <= 1 {
		d = append(d, Decl{"opacity", fnum(v)})
	}
	return d
}

// AnimCSS is the shared CSS of the scroll-in animations; the page adds it
// (with AnimScript) only when a node uses one. Without JavaScript, or with
// reduced motion, everything is simply visible. The individual translate /
// scale properties leave transform (rotation, hover) alone.
const AnimCSS = `.rea-js .rea-anim{opacity:0;transition:opacity var(--anim-dur,700ms) ease var(--anim-delay,0ms),translate var(--anim-dur,700ms) cubic-bezier(.2,.7,.2,1) var(--anim-delay,0ms),scale var(--anim-dur,700ms) cubic-bezier(.2,.7,.2,1) var(--anim-delay,0ms)}
.rea-js .rea-anim-up{translate:0 40px}.rea-js .rea-anim-down{translate:0 -40px}.rea-js .rea-anim-left{translate:-48px 0}.rea-js .rea-anim-right{translate:48px 0}.rea-js .rea-anim-zoom{scale:.85}
.rea-js .rea-anim.rea-in{opacity:1;translate:0 0;scale:1}
@media (prefers-reduced-motion:reduce){.rea-js .rea-anim{opacity:1;translate:none;scale:none;transition:none}}
`

// AnimScript reveals .rea-anim elements as they scroll into view.
const AnimScript = `(function(){var d=document.documentElement;if(!('IntersectionObserver' in window))return;d.classList.add('rea-js');var o=new IntersectionObserver(function(es){es.forEach(function(e){if(e.isIntersecting){e.target.classList.add('rea-in');o.unobserve(e.target)}})},{threshold:.12,rootMargin:'0px 0px -5% 0px'});document.addEventListener('DOMContentLoaded',function(){document.querySelectorAll('.rea-anim').forEach(function(el){o.observe(el)})})})();`

// baseTransform is a layer's own rotation and flip (props flipX / flipY).
// Mirrors baseTransform() in design-css.ts.
func baseTransform(n *Node) []string {
	var tf []string
	if r, ok := getNum(n.Style, "rotation"); ok && r != 0 {
		tf = append(tf, "rotate("+fnum(r)+"deg)")
	}
	fx, fy := getBool(n.Props, "flipX"), getBool(n.Props, "flipY")
	if fx || fy {
		x, y := "1", "1"
		if fx {
			x = "-1"
		}
		if fy {
			y = "-1"
		}
		tf = append(tf, "scale("+x+","+y+")")
	}
	return tf
}

// BlockPart is a styled element inside a plugin block (props.parts).
type BlockPart struct {
	Class string
	Decls []Decl
	Hover []Decl // look while the pointer is over the part
}

var partClassRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,60}$`)

// BlockParts reads props.parts of a plugin block: {"pt-name": {"color": …,
// "fill": …, "fontSize": …, "fontWeight": …, "radius": …, "hidden": true}},
// keyed by a class name used inside the block. Sorted for stable CSS.
// Mirrors partCss() in design-css.ts.
func BlockParts(n *Node) []BlockPart {
	if n.Type != TypePluginBlock {
		return nil
	}
	parts, _ := n.Props["parts"].(map[string]any)
	keys := make([]string, 0, len(parts))
	for k := range parts {
		if partClassRe.MatchString(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out []BlockPart
	for _, k := range keys {
		m, _ := parts[k].(map[string]any)
		var d []Decl
		if getBool(m, "hidden") {
			d = append(d, Decl{"display", "none"})
		}
		if v := getStr(m, "color"); v != "" {
			d = append(d, Decl{"color", safeCSS(v)})
		}
		if v := getStr(m, "fill"); v != "" {
			d = append(d, Decl{"background", safeCSS(v)})
		}
		if v, ok := getNum(m, "fontSize"); ok && v > 0 {
			d = append(d, Decl{"font-size", px(v)})
		}
		if v, ok := getNum(m, "fontWeight"); ok && v > 0 {
			d = append(d, Decl{"font-weight", fnum(v)})
		}
		if v, ok := getNum(m, "radius"); ok && v >= 0 && m["radius"] != nil {
			d = append(d, Decl{"border-radius", px(v)})
		}
		d = append(d, partExtra(m)...)
		var h []Decl
		if hv, ok := m["hover"].(map[string]any); ok {
			if v := getStr(hv, "color"); v != "" {
				h = append(h, Decl{"color", safeCSS(v)})
			}
			if v := getStr(hv, "fill"); v != "" {
				h = append(h, Decl{"background", safeCSS(v)})
			}
		}
		if len(d) > 0 || len(h) > 0 {
			out = append(out, BlockPart{Class: k, Decls: d, Hover: h})
		}
	}
	return out
}

// focal is the point of an image kept in view when it is cropped
// (style.focalX / focalY in %, default the center).
func focal(s map[string]any) string {
	x, okx := getNum(s, "focalX")
	y, oky := getNum(s, "focalY")
	if !okx && !oky {
		return "center"
	}
	if !okx {
		x = 50
	}
	if !oky {
		y = 50
	}
	clamp := func(v float64) float64 { return math.Max(0, math.Min(100, v)) }
	return strconv.FormatFloat(clamp(x), 'f', -1, 64) + "% " + strconv.FormatFloat(clamp(y), 'f', -1, 64) + "%"
}

var partAlignRe = regexp.MustCompile(`^(left|center|right|justify)$`)
var partCaseRe = regexp.MustCompile(`^(none|uppercase|lowercase|capitalize)$`)
var partFontRe = regexp.MustCompile(`^[A-Za-z0-9 ]{1,40}$`)

// partExtra: the finer settings of a plugin block part (typography, spacing,
// border, shadow, opacity). Mirrors partCss() in design-css.ts.
func partExtra(m map[string]any) []Decl {
	var d []Decl
	if v := getStr(m, "fontFamily"); partFontRe.MatchString(v) {
		d = append(d, Decl{"font-family", "'" + v + "', sans-serif"})
	}
	if v := getStr(m, "textAlign"); partAlignRe.MatchString(v) {
		d = append(d, Decl{"text-align", v})
	}
	if v, ok := getNum(m, "lineHeight"); ok && v > 0 && v < 10 {
		d = append(d, Decl{"line-height", fnum(v)})
	}
	if v, ok := getNum(m, "letterSpacing"); ok && m["letterSpacing"] != nil && v > -20 && v < 50 {
		d = append(d, Decl{"letter-spacing", px(v)})
	}
	if v := getStr(m, "textTransform"); partCaseRe.MatchString(v) {
		d = append(d, Decl{"text-transform", v})
	}
	if py, okY := getNum(m, "padY"); okY || m["padX"] != nil {
		px2, _ := getNum(m, "padX")
		d = append(d, Decl{"padding", px(math.Max(0, py)) + " " + px(math.Max(0, px2))})
	}
	if v := getStr(m, "stroke"); v != "" {
		w, ok := getNum(m, "strokeWidth")
		if !ok || w <= 0 {
			w = 1
		}
		d = append(d, Decl{"border", px(w) + " solid " + safeCSS(v)})
	}
	if v := getStr(m, "shadow"); v != "" {
		d = append(d, Decl{"box-shadow", safeCSS(v)})
	}
	if v, ok := getNum(m, "opacity"); ok && m["opacity"] != nil && v >= 0 && v < 1 {
		d = append(d, Decl{"opacity", fnum(v)})
	}
	return d
}
