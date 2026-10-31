package design

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The frontend (codeblock.ts scopeCss) and the server must scope a code
// block's CSS identically: shared vectors.
func TestScopeCSSVectors(t *testing.T) {
	b, err := os.ReadFile("testdata/codeblock_scope_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Cases []struct{ Name, ID, CSS, Out string }
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cases) < 30 {
		t.Fatalf("only %d vectors", len(v.Cases))
	}
	for _, c := range v.Cases {
		if got := ScopeCSS(c.CSS, c.ID); got != c.Out {
			t.Errorf("%s:\n got %q\nwant %q", c.Name, got, c.Out)
		}
	}
}

func TestCodeBlockRender(t *testing.T) {
	root := &Node{ID: "root", Type: TypeFrame, Children: []*Node{{ID: "cb1", Type: TypeCodeBlock, Props: map[string]any{
		"html": `<p class="x">Hi</p><style>p{color:red}</style>`,
		"css":  ":scope { padding: 4px } .x { color: blue } @import 'evil.css';",
		"js":   "el.querySelector('.x').textContent = '</script><script>alert(1)</script>';",
	}}}}
	out := Render(root, Context{}, nil)
	h := out.HTML
	for _, want := range []string{
		`<div class="n-cb1 rea-cb-cb1" data-rea-code-block="cb1"><p class="x">Hi</p><style>.rea-cb-cb1 p { color:red }</style></div>`,
		`<style>.rea-cb-cb1 { padding: 4px }` + "\n" + `.rea-cb-cb1 .x { color: blue }</style>`,
		`<script type="module">document.querySelectorAll('[data-rea-code-block="cb1"]').forEach(el => (async (el, anasaki) => {`,
		`})(el, window.anasaki).catch(`,
		`'<\/script><script>alert(1)<\/script>'`,
	} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %s\nin %s", want, h)
		}
	}
	if strings.Contains(h, "evil.css") {
		t.Error("@import kept")
	}
	if !out.Runtime {
		t.Error("a code block with JS needs the client runtime (window.anasaki)")
	}
	// a block repeated (collection items) writes its style and script once
	root.Children = append(root.Children, root.Children[0])
	h = Render(root, Context{}, nil).HTML
	if strings.Count(h, `data-rea-code-block="cb1"`) != 2+1 || strings.Count(h, `<script type="module">`) != 1 {
		t.Errorf("repeat: %s", h)
	}
	// no JS → no runtime
	root.Children = []*Node{{ID: "c2", Type: TypeCodeBlock, Props: map[string]any{"html": "<b>x</b>"}}}
	if out := Render(root, Context{}, nil); out.Runtime || strings.Contains(out.HTML, "<script") {
		t.Errorf("html-only block: %+v", out)
	}
}

func TestChangedAndStripCodeBlocks(t *testing.T) {
	old := json.RawMessage(`{"id":"r","type":"frame","children":[{"id":"a","type":"codeblock","props":{"html":"<b>a</b>","css":"","js":"x()"}}]}`)
	same := json.RawMessage(`{"id":"r","type":"frame","name":"renamed","children":[{"id":"a","type":"codeblock","props":{"js":"x()","html":"<b>a</b>","css":""}}]}`)
	if c := ChangedCodeBlocks(same, old); len(c) != 0 {
		t.Errorf("unchanged block reported: %+v", c)
	}
	changed := json.RawMessage(`{"id":"r","type":"frame","children":[{"id":"a","type":"codeblock","props":{"html":"<b>a</b>","js":"y()"}},` +
		`{"id":"b","type":"codeblock","props":{"js":"z()"}},{"id":"e","type":"codeblock","props":{"html":" "}}]}`)
	c := ChangedCodeBlocks(changed, old)
	if len(c) != 2 || c[0].ID != "a" || c[1].ID != "b" {
		t.Errorf("changed = %+v", c)
	}
	if c := ChangedCodeBlocks(old); len(c) != 1 { // new document: every non-empty block is new
		t.Errorf("new = %+v", c)
	}
	// stripping keeps the unchanged block byte for byte
	mixed := json.RawMessage(`{"id":"r","type":"frame","children":[{"id":"a","type":"codeblock","props":{"html":"<b>a</b>","css":"","js":"x()"}},{"id":"b","type":"codeblock","props":{"js":"evil()"}}]}`)
	s := string(StripCodeBlocks(mixed, old))
	if !strings.Contains(s, `"js":"x()"`) || strings.Contains(s, "evil") {
		t.Errorf("strip: %s", s)
	}
	if string(StripCodeBlocks(same, old)) != string(same) {
		t.Error("nothing to strip must return the input")
	}
	if err := CheckCodeBlockCSS([]CodeBlock{{ID: "a", CSS: "@IMPORT url(x)"}}); err == nil {
		t.Error("@import accepted")
	}
	if err := CheckCodeBlockCSS([]CodeBlock{{ID: "a", HTML: "<style>@import 'x';</style>"}}); err == nil {
		t.Error("@import in <style> accepted")
	}
	if err := CheckCodeBlockCSS([]CodeBlock{{ID: "a", CSS: "a::after{content:'@import'}"}}); err != nil {
		t.Errorf("@import inside a string: %v", err)
	}
}
