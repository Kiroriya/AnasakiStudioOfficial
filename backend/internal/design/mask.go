package design

import (
	"net/url"
	"sort"
	"strings"
)

// Masks (Figma "use as mask"): style.mask = <name> clips a layer — usually an
// image — to a shape. The blob-like shapes are reused, plus a few frames
// common on websites.
var maskExtra = map[string]Shape{
	"arch":    {Kind: "mask", ViewBox: "0 0 200 200", Body: `<path d="M0,200V100A100,100 0 0 1 200,100V200Z"/>`},
	"diamond": {Kind: "mask", ViewBox: "0 0 200 200", Body: `<path d="M100,0L200,100L100,200L0,100Z"/>`},
	"leaf":    {Kind: "mask", ViewBox: "0 0 200 200", Body: `<path d="M0,200V90A90,90 0 0 1 90,0H200V110A90,90 0 0 1 110,200Z"/>`},
	"ticket":  {Kind: "mask", ViewBox: "0 0 200 200", Body: `<path d="M0,0H200V80A20,20 0 0 0 200,120V200H0V120A20,20 0 0 0 0,80Z"/>`},
}

// Masks lists every mask shape by name.
func Masks() map[string]Shape {
	out := map[string]Shape{}
	for k, s := range Shapes {
		if s.Kind == "blob" {
			out[k] = s
		}
	}
	for k, s := range maskExtra {
		out[k] = s
	}
	return out
}

// MaskNames is the sorted list of mask names.
func MaskNames() []string {
	m := Masks()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MaskURL is the CSS url() of a mask shape ("" for an unknown name).
func MaskURL(name string) string {
	sh, ok := Masks()[name]
	if !ok {
		return ""
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="` + sh.ViewBox + `" preserveAspectRatio="none">` +
		strings.ReplaceAll(sh.Body, "currentColor", "#000") + `</svg>`
	return `url("data:image/svg+xml,` + url.PathEscape(svg) + `")`
}
