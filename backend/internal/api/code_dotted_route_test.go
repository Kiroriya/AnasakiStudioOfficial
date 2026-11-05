package api

import (
	"strings"
	"testing"
)

// CMS 2.2.2: a JS plugin can serve a file-like path such as /feed.xml (an RSS plugin), declared in code.routes.
func TestJSPluginDottedRoute(t *testing.T) {
	e := newPluginAPI(t)
	admin, _ := setupSite(t, e.c)
	mustUpload(t, admin, jsPlugin(t, map[string]any{"id": "feeds",
		"code": map[string]any{"entry": "code/main.ts", "scopes": []any{"routes"}, "routes": []any{"/feed.xml"}}},
		map[string]string{"code/main.ts": `
site.route('GET /feed.xml', { access: ['*'] }, () => ({ status: 200, headers: { 'Content-Type': 'application/rss+xml; charset=utf-8' }, body: '<rss version="2.0"></rss>' }));
`}))
	activate(t, admin, "feeds")
	rec := e.c.send("GET", "/feed.xml", nil, nil)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/rss+xml") || !strings.Contains(rec.Body.String(), `<rss version="2.0">`) {
		t.Fatalf("GET /feed.xml: %d %q %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
