package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// forms 2.1: single / multiple choice fields, fields shown only when an earlier field has a value (not required
// while hidden, cleared), and the auto-reply to the visitor's email address.
func TestOfficialForms21(t *testing.T) {
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "forms")
	_, call := adminPage(t, e.toSample(), admin, "forms:forms", "")

	save := func(form map[string]any) (map[string]any, string) {
		t.Helper()
		code, out := call("forms:save", map[string]any{"form": form})
		if code != 200 {
			t.Fatalf("save: %d %v", code, out)
		}
		if errs, ok := out["errors"].(map[string]any); ok && len(errs) > 0 {
			return nil, fmt.Sprint(errs["form"])
		}
		return out["data"].(map[string]any), ""
	}
	// invalid: options missing, a condition on a later / unknown field, auto-reply without an email field
	for i, f := range []map[string]any{
		{"name": "x", "fields": []any{map[string]any{"key": "a", "type": "radio"}}},
		{"name": "x", "fields": []any{map[string]any{"key": "a", "showIf": map[string]any{"field": "b", "value": "1"}}, map[string]any{"key": "b"}}},
		{"name": "x", "fields": []any{map[string]any{"key": "a"}}, "autoReply": map[string]any{"subject": "Hi", "message": "x"}},
	} {
		if _, msg := save(f); msg == "" {
			t.Errorf("case %d should be invalid", i)
		}
	}
	fm, msg := save(map[string]any{"name": "Quote", "fields": []any{
		map[string]any{"key": "email", "label": "Email", "type": "email", "required": true},
		map[string]any{"key": "kind", "label": "Kind", "type": "radio", "required": true, "options": []any{"Home", "Office"}},
		map[string]any{"key": "rooms", "label": "Rooms", "type": "checkboxes", "options": []any{"Kitchen", "Bath", "Garden"}},
		map[string]any{"key": "company", "label": "Company", "type": "text", "required": true, "showIf": map[string]any{"field": "kind", "value": "Office"}},
		map[string]any{"key": "garden_size", "label": "Garden size", "type": "number", "showIf": map[string]any{"field": "rooms", "value": "Garden"}},
	}, "autoReply": map[string]any{"subject": "We got your request", "message": "Hello!\n\nWe will call you."}})
	if msg != "" {
		t.Fatalf("save: %s", msg)
	}
	id := fmt.Sprint(fm["id"])

	html := e.offBSend(offBReq{path: "/forms/" + id}).Body.String()
	offBContains(t, "render", html, `type="radio" name="kind"`, `type="checkbox" name="rooms[]"`, `value="Garden"`,
		`data-show-if="kind" data-show-value="Office"`, `data-show-if="rooms" data-show-value="Garden"`)

	submit := func(ip string, v url.Values) (int, map[string]any) {
		t.Helper()
		rec := e.offBSend(offBReq{method: "POST", path: "/forms/" + id + "/submit", ip: ip, body: strings.NewReader(v.Encode()),
			hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"}})
		out := map[string]any{}
		_ = jsonUnmarshalTest(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	// Home: the company field is hidden, so not required (and a sent value is dropped)
	if c, out := submit("10.21.0.1", url.Values{"email": {"a@x.io"}, "kind": {"Home"}, "company": {"ignored"}, "rooms[]": {"Kitchen", "Garden"}, "garden_size": {"40"}}); c != 200 || out["ok"] != true {
		t.Fatalf("home: %d %v", c, out)
	}
	// Office without a company: required
	if c, out := submit("10.21.0.2", url.Values{"email": {"b@x.io"}, "kind": {"Office"}}); c == 200 && out["ok"] == true {
		t.Fatalf("office without company accepted: %v", out)
	}
	// a choice that is not an option
	if c, out := submit("10.21.0.3", url.Values{"email": {"c@x.io"}, "kind": {"Castle"}}); c == 200 && out["ok"] == true {
		t.Fatalf("bad radio accepted: %v", out)
	}
	if c, out := submit("10.21.0.4", url.Values{"email": {"d@x.io"}, "kind": {"Home"}, "rooms[]": {"Pool"}}); c == 200 && out["ok"] == true {
		t.Fatalf("bad checkbox accepted: %v", out)
	}
	_, list := call("forms:submissions", map[string]any{"formId": fm["id"]})
	got := fmt.Sprint(list["data"])
	if !strings.Contains(got, "rooms:Kitchen, Garden") || !strings.Contains(got, "garden_size:40") || !strings.Contains(got, "company:]") && !strings.Contains(got, "company: ") {
		t.Fatalf("stored: %s", got)
	}
	e.code.WaitEvents()
	e.code.ProcessJobs(context.Background(), 10)
	mails := fmt.Sprint(e.offBMailbox())
	if !strings.Contains(mails, "a@x.io") || !strings.Contains(mails, "We got your request") || !strings.Contains(mails, "<p>We will call you.</p>") {
		t.Fatalf("auto-reply: %s", mails)
	}
	// the builder keeps the new parts
	_, forms := call("forms:list", map[string]any{})
	if s := fmt.Sprint(forms["data"]); !strings.Contains(s, "showIf:map[field:kind value:Office]") || !strings.Contains(s, "autoReply:map[message:Hello!") {
		t.Fatalf("list: %s", s)
	}
}
