package design

import (
	"strings"
	"testing"
)

// 2.4.0: images set to open large on click carry the gallery (their frame) and the full-size source; the page
// gets the interaction runtime and CSS; a linked image keeps its link instead.
func TestImageLightbox(t *testing.T) {
	doc := `{"id":"root","type":"frame","children":[
	 {"id":"gal","type":"frame","layout":{"mode":"horizontal","wrap":true},"children":[
	  {"id":"a","type":"image","props":{"src":"/uploads/a.jpg","alt":"First","lightbox":true}},
	  {"id":"b","type":"image","props":{"src":"/uploads/b.jpg","alt":"Second","lightbox":true}}]},
	 {"id":"c","type":"image","props":{"src":"/uploads/c.jpg","alt":"Alone","lightbox":true}},
	 {"id":"d","type":"image","props":{"src":"/uploads/d.jpg","lightbox":true,"href":"/elsewhere"}},
	 {"id":"e","type":"image","props":{"src":"/uploads/e.jpg"}}]}`
	out := renderDoc(t, doc, Context{}, nil)
	must(t, out.HTML,
		`alt="First" data-rea-lb="gal" data-rea-full="/uploads/a.jpg" tabindex="0" role="button">`,
		`alt="Second" data-rea-lb="gal" data-rea-full="/uploads/b.jpg"`,
		`alt="Alone" loading="lazy" data-rea-lb="root" data-rea-full="/uploads/c.jpg"`)
	if strings.Contains(out.HTML, `data-rea-full="/uploads/d.jpg"`) || strings.Contains(out.HTML, `data-rea-full="/uploads/e.jpg"`) {
		t.Fatalf("lightbox on a linked or plain image:\n%s", out.HTML)
	}
	if !out.Interact || !strings.Contains(out.CSS, ".rea-lbx{") || !strings.Contains(InteractScript, "data-rea-lb") {
		t.Fatal("no lightbox runtime / CSS")
	}
}
