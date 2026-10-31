package api

import (
	"strings"
	"testing"
	"time"
)

func TestDesignCommentPermissionsAndSaveConflicts(t *testing.T) {
	c, st := newTestAPI(t)
	if _, err := st.DB.Exec("DELETE FROM design_comments"); err != nil {
		t.Fatal(err)
	}
	code, body := c.do("POST", "/api/v1/setup", map[string]any{"siteTitle": "S", "adminName": "Owner",
		"adminEmail": "owner@x.io", "adminPassword": "secret123", "locale": "en", "demo": false})
	if code != 201 {
		t.Fatalf("setup: %d %v", code, body)
	}
	admin := c.as(body["token"].(string))
	user := func(name, role string) *client {
		email := strings.ToLower(name) + "@x.io"
		if code, b := admin.do("POST", "/api/v1/users", map[string]any{"name": name, "email": email, "password": "password1", "role": role}); code != 201 {
			t.Fatalf("create %s: %d %v", name, code, b)
		}
		_, l := c.do("POST", "/api/v1/auth/login", map[string]any{"email": email, "password": "password1"})
		return c.as(l["token"].(string))
	}
	somchai, malee := user("Somchai", "editor"), user("Malee", "editor")
	author, contrib := user("Arthur", "author"), user("Connie", "contributor")

	_, page := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Home"})
	pageID := page["id"].(float64)
	_, own := author.do("POST", "/api/v1/posts", map[string]any{"type": "post", "title": "Mine"})
	postID := own["id"].(float64)
	_, comp := admin.do("POST", "/api/v1/components", map[string]any{"name": "Card", "node": map[string]any{"id": "root", "type": "frame"}})
	compID := comp["id"].(float64)

	// ---- comments: access follows edit access
	code, root := somchai.do("POST", "/api/v1/design/comments", map[string]any{"kind": "post", "id": pageID, "nodeId": "n1", "x": 10, "y": 20, "body": " Bigger heading? "})
	if code != 201 || root["body"] != "Bigger heading?" || root["authorName"] != "Somchai" || root["nodeId"] != "n1" {
		t.Fatalf("editor comment on page: %d %v", code, root)
	}
	rootID := jsonNum(root["id"].(float64))
	if code, _ := contrib.do("POST", "/api/v1/design/comments", map[string]any{"kind": "post", "id": pageID, "body": "x"}); code != 403 {
		t.Fatalf("contributor commenting on a page must be 403, got %d", code)
	}
	if code, _ := contrib.do("POST", "/api/v1/design/comments", map[string]any{"kind": "post", "id": postID, "body": "x"}); code != 403 {
		t.Fatalf("contributor commenting on another's post must be 403, got %d", code)
	}
	if code, _ := contrib.do("GET", "/api/v1/design/comments?kind=post&id="+jsonNum(pageID), nil); code != 403 {
		t.Fatalf("contributor listing page comments must be 403, got %d", code)
	}
	if code, _ := author.do("POST", "/api/v1/design/comments", map[string]any{"kind": "post", "id": postID, "body": "note to self"}); code != 201 {
		t.Fatalf("author commenting on own post: %d", code)
	}
	if code, _ := author.do("POST", "/api/v1/design/comments", map[string]any{"kind": "component", "id": compID, "body": "x"}); code != 403 {
		t.Fatalf("author (no edit_pages) commenting on a component must be 403, got %d", code)
	}
	if code, _ := somchai.do("POST", "/api/v1/design/comments", map[string]any{"kind": "component", "id": compID, "body": "ok"}); code != 201 {
		t.Fatalf("editor commenting on a component: %d", code)
	}
	if code, _ := somchai.do("POST", "/api/v1/design/comments", map[string]any{"kind": "post", "id": pageID, "body": "   "}); code != 400 {
		t.Fatalf("empty comment must be 400, got %d", code)
	}
	if code, _ := somchai.do("POST", "/api/v1/design/comments", map[string]any{"kind": "post", "id": pageID, "body": strings.Repeat("ก", 5001)}); code != 400 {
		t.Fatalf("too long comment must be 400, got %d", code)
	}
	if code, _ := somchai.do("POST", "/api/v1/design/comments", map[string]any{"kind": "post", "id": pageID, "nodeId": "<script>", "body": "x"}); code != 400 {
		t.Fatalf("bad node id must be 400, got %d", code)
	}

	// ---- replies, edit own, resolve, delete own / admin any
	code, reply := malee.do("POST", "/api/v1/design/comments", map[string]any{"parentId": root["id"], "body": "Agreed", "kind": "component", "id": compID})
	if code != 201 || reply["parentId"] != root["id"] || reply["targetKind"] != "post" || reply["targetId"] != pageID {
		t.Fatalf("reply must attach to the root's target: %d %v", code, reply)
	}
	replyID := jsonNum(reply["id"].(float64))
	if code, _ := malee.do("PUT", "/api/v1/design/comments/"+rootID, map[string]any{"body": "hijack"}); code != 403 {
		t.Fatalf("editing someone else's comment must be 403, got %d", code)
	}
	if code, _ := admin.do("PUT", "/api/v1/design/comments/"+rootID, map[string]any{"body": "hijack"}); code != 403 {
		t.Fatalf("even admins cannot edit someone else's comment, got %d", code)
	}
	if code, b := somchai.do("PUT", "/api/v1/design/comments/"+rootID, map[string]any{"body": "Bigger heading please"}); code != 200 || b["body"] != "Bigger heading please" {
		t.Fatalf("edit own comment: %d %v", code, b)
	}
	if code, _ := malee.do("DELETE", "/api/v1/design/comments/"+rootID, nil); code != 403 {
		t.Fatalf("deleting someone else's comment must be 403, got %d", code)
	}
	if code, b := malee.do("POST", "/api/v1/design/comments/"+rootID+"/resolve", map[string]any{"resolved": true}); code != 200 || b["resolved"] != true {
		t.Fatalf("resolve: %d %v", code, b)
	}
	if code, _ := malee.do("POST", "/api/v1/design/comments/"+replyID+"/resolve", map[string]any{"resolved": true}); code != 400 {
		t.Fatalf("resolving a reply must be 400, got %d", code)
	}
	if code, _ := contrib.do("POST", "/api/v1/design/comments/"+rootID+"/resolve", map[string]any{"resolved": false}); code != 403 {
		t.Fatalf("contributor resolving page comment must be 403, got %d", code)
	}
	if code, _ := malee.do("DELETE", "/api/v1/design/comments/"+replyID, nil); code != 204 {
		t.Fatalf("delete own reply: %d", code)
	}
	if code, _ := admin.do("DELETE", "/api/v1/design/comments/"+rootID, nil); code != 204 {
		t.Fatalf("admin deletes any comment: %d", code)
	}
	if _, err := st.GetDesignComment(int64(root["id"].(float64))); err == nil {
		t.Fatal("deleted comment still there")
	}

	// ---- optimistic concurrency (posts)
	if _, err := st.DB.Exec("UPDATE posts SET updated_at = ? WHERE id = ?", time.Now().UTC().Add(-time.Hour), int64(pageID)); err != nil {
		t.Fatal(err)
	}
	_, loaded := admin.do("GET", "/api/v1/posts/"+jsonNum(pageID), nil)
	base := loaded["updatedAt"]
	if code, b := somchai.do("PUT", "/api/v1/posts/"+jsonNum(pageID), map[string]any{"title": "Home v2", "baseUpdatedAt": base}); code != 200 {
		t.Fatalf("save with current base: %d %v", code, b)
	}
	code, b := malee.do("PUT", "/api/v1/posts/"+jsonNum(pageID), map[string]any{"title": "Home v3", "baseUpdatedAt": base})
	if code != 409 {
		t.Fatalf("stale save must be 409, got %d %v", code, b)
	}
	if cf, _ := b["conflict"].(map[string]any); cf == nil || cf["updatedBy"] != "Somchai" {
		t.Fatalf("409 must name who saved: %v", b)
	}
	if code, _ := malee.do("PUT", "/api/v1/posts/"+jsonNum(pageID), map[string]any{"title": "Home v3"}); code != 200 {
		t.Fatalf("save without base (old clients / overwrite) must pass, got %d", code)
	}

	// ---- optimistic concurrency (components): the response's updatedAt is the next base
	if _, err := st.DB.Exec("UPDATE components SET updated_at = ? WHERE id = ?", time.Now().UTC().Add(-time.Hour), int64(compID)); err != nil {
		t.Fatal(err)
	}
	cb, _ := st.TargetUpdatedAt("component", int64(compID))
	node := map[string]any{"id": "root", "type": "frame"}
	code, saved := somchai.do("PUT", "/api/v1/components/"+jsonNum(compID), map[string]any{"name": "Card 2", "node": node, "baseUpdatedAt": cb})
	if code != 200 {
		t.Fatalf("component save: %d %v", code, saved)
	}
	if code, b := somchai.do("PUT", "/api/v1/components/"+jsonNum(compID), map[string]any{"name": "Card 3", "node": node, "baseUpdatedAt": saved["updatedAt"]}); code != 200 {
		t.Fatalf("saving again from the returned updatedAt must pass: %d %v", code, b)
	}
	if code, b := malee.do("PUT", "/api/v1/components/"+jsonNum(compID), map[string]any{"name": "Card X", "node": node, "baseUpdatedAt": cb}); code != 409 {
		t.Fatalf("stale component save must be 409, got %d %v", code, b)
	}

	// ---- presence
	if code, b := somchai.do("POST", "/api/v1/design/presence", map[string]any{"kind": "post", "id": pageID, "session": "s1", "selectedId": "n1"}); code != 200 || len(b["users"].([]any)) != 0 {
		t.Fatalf("first heartbeat: %d %v", code, b)
	}
	code, b = malee.do("POST", "/api/v1/design/presence", map[string]any{"kind": "post", "id": pageID, "session": "s2"})
	users, _ := b["users"].([]any)
	if code != 200 || len(users) != 1 || users[0].(map[string]any)["name"] != "Somchai" || users[0].(map[string]any)["selectedId"] != "n1" || b["updatedBy"] != "Malee" {
		t.Fatalf("second heartbeat must list Somchai: %d %v", code, b)
	}
	if code, _ := contrib.do("POST", "/api/v1/design/presence", map[string]any{"kind": "post", "id": pageID, "session": "s3"}); code != 403 {
		t.Fatalf("presence needs edit access, got %d", code)
	}
	somchai.do("DELETE", "/api/v1/design/presence?kind=post&id="+jsonNum(pageID)+"&session=s1", nil)
	if _, b := malee.do("POST", "/api/v1/design/presence", map[string]any{"kind": "post", "id": pageID, "session": "s2"}); len(b["users"].([]any)) != 0 {
		t.Fatalf("left user still present: %v", b)
	}
}
