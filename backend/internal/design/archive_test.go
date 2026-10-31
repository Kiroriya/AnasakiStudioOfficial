package design

import "testing"

// queryRecorder records the post query a postList asks for.
type queryRecorder struct {
	fakeData
	got []Query
}

func (q *queryRecorder) QueryPosts(qq Query) []PostCtx {
	q.got = append(q.got, qq)
	return nil
}

func TestPostListOnTypeArchive(t *testing.T) {
	list := func(query map[string]any) *Node {
		return &Node{ID: "root", Type: TypeFrame, Children: []*Node{{ID: "l", Type: TypePostList, Props: map[string]any{"query": query}}}}
	}
	cases := []struct {
		name     string
		query    map[string]any
		post     PostCtx
		category string
		wantType string
		wantCat  string
	}{
		{"archive template on /events lists events", map[string]any{"postType": "post", "category": "current"}, PostCtx{"archiveType": "event"}, "", "event", ""},
		{"postType current on /events", map[string]any{"postType": "current"}, PostCtx{"archiveType": "event"}, "", "event", ""},
		{"explicit post list on /events stays blog posts", map[string]any{"postType": "post"}, PostCtx{"archiveType": "event"}, "", "post", ""},
		{"category archive unchanged", map[string]any{"postType": "post", "category": "current"}, PostCtx{}, "news", "post", "news"},
		{"postType current elsewhere = post", map[string]any{"postType": "current"}, PostCtx{}, "", "post", ""},
	}
	for _, c := range cases {
		rec := &queryRecorder{}
		Render(list(c.query), Context{Post: c.post, Category: c.category}, rec)
		if len(rec.got) != 1 || rec.got[0].PostType != c.wantType || rec.got[0].Category != c.wantCat {
			t.Errorf("%s: got %+v, want type %q category %q", c.name, rec.got, c.wantType, c.wantCat)
		}
	}
}
