package design

import (
	"strings"
	"testing"
)

type contentData struct{ fakeData }

func (contentData) FilterContent(h string, post PostCtx) string {
	return h + "<i>" + post["title"] + "</i>"
}

func TestRenderVisitorMetaAndContentFilter(t *testing.T) {
	root := &Node{ID: "root", Type: TypeFrame, Children: []*Node{
		{ID: "hi", Type: TypeText, Props: map[string]any{"text": "Hi {{visitor.name ?? 'guest'}} {{post.meta.date}} {{post.meta.seats * 2}}"}},
		{ID: "vip", Type: TypeText, Props: map[string]any{"text": "VIP", "showWhen": "visitor.is.gold"}},
		{ID: "body", Type: TypePostContent},
	}}
	post := PostCtx{"title": "T", "content": "<p>c</p>", "meta": `{"date":"2026-11-01","seats":4}`}
	calls := 0
	ctx := Context{Post: post, Visitor: func() map[string]any {
		calls++
		return map[string]any{"name": "Ann", "groups": []any{"gold"}}
	}}
	out := Render(root, ctx, contentData{})
	for _, want := range []string{"Hi Ann 2026-11-01 8", ">VIP<", "<p>c</p><i>T</i>"} {
		if !strings.Contains(out.HTML, want) {
			t.Errorf("missing %q in %s", want, out.HTML)
		}
	}
	if calls == 0 {
		t.Fatal("visitor provider not used")
	}
	out = Render(root, Context{Post: post}, fakeData{})
	if !strings.Contains(out.HTML, "Hi guest") || strings.Contains(out.HTML, ">VIP<") || !strings.Contains(out.HTML, "<p>c</p></div>") {
		t.Fatalf("no visitor / no filter: %s", out.HTML)
	}
}
