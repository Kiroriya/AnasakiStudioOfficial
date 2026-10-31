package design

// TextStyle is a shared, named text look (Figma "text style"), e.g. H1 or
// Body. A node uses one with style.textStyle = <id>; the node's own values
// win over the style's, so a style can be adjusted per layer.
type TextStyle struct {
	Base   map[string]any // desktop values
	Tablet map[string]any // overrides ≤1024px
	Mobile map[string]any // overrides ≤640px
}

// TextStyleKeys are the style keys a text style may set.
var TextStyleKeys = map[string]bool{
	"fontFamily": true, "fontSize": true, "fontWeight": true, "lineHeight": true,
	"letterSpacing": true, "textTransform": true, "italic": true, "textDecoration": true,
}

// withTextStyle returns n with its text style's values filled in under the
// node's own (desktop and per breakpoint). n itself is not changed.
func withTextStyle(n *Node, styles map[string]TextStyle) *Node {
	id := getStr(n.Style, "textStyle")
	if id == "" || styles == nil {
		return n
	}
	ts, ok := styles[id]
	if !ok {
		return n
	}
	c := *n
	c.Style = mergeMap(cloneMap(ts.Base), n.Style)
	resp := map[string]map[string]any{}
	for k, v := range n.Responsive {
		resp[k] = v
	}
	for bp, vals := range map[string]map[string]any{"tablet": ts.Tablet, "mobile": ts.Mobile} {
		if len(vals) == 0 {
			continue
		}
		add := map[string]any{}
		for k, v := range vals {
			if _, own := n.Style[k]; !own { // a value set on the layer itself wins everywhere
				add[k] = v
			}
		}
		if len(add) == 0 {
			continue
		}
		o := map[string]any{}
		for k, v := range resp[bp] {
			o[k] = v
		}
		own, _ := o["style"].(map[string]any)
		o["style"] = mergeMap(add, own)
		resp[bp] = o
	}
	c.Responsive = resp
	return &c
}
