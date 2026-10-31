package design

import "encoding/json"

// VariantNode returns the design of a component's variant by name (Figma
// variants: props.variants = [{name, node}] on the component's root), or nil
// for the main design / an unknown name. Variants are copies of the main
// design, so layer ids — and an instance's per-layer overrides — carry over.
func VariantNode(comp *Node, name string) *Node {
	if name == "" || comp == nil {
		return nil
	}
	list, _ := comp.Props["variants"].([]any)
	for _, it := range list {
		m, _ := it.(map[string]any)
		if m == nil || getStr(m, "name") != name {
			continue
		}
		raw, err := json.Marshal(m["node"])
		if err != nil {
			return nil
		}
		v, err := Parse(raw)
		if err != nil || v == nil || v.Type == "" {
			return nil
		}
		return v
	}
	return nil
}
