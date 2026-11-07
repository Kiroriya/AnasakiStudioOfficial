package api

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"
)

// forms 2.2: a file field — the form posts multipart, the file is checked by type and first bytes, kept as a
// private file, listed by name in the inbox and downloaded from the Forms page; deleting the submission
// deletes the file.
func TestOfficialFormsFileUpload(t *testing.T) {
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "forms")
	_, call := adminPage(t, e.toSample(), admin, "forms:forms", "")
	code, out := call("forms:save", map[string]any{"form": map[string]any{"name": "Jobs", "fields": []any{
		map[string]any{"key": "name", "label": "Name", "type": "text", "required": true},
		map[string]any{"key": "cv", "label": "CV", "type": "file", "required": true},
	}}})
	if code != 200 || out["data"] == nil {
		t.Fatalf("save: %d %v", code, out)
	}
	id := int(out["data"].(map[string]any)["id"].(float64))

	page := e.offBSend(offBReq{path: fmt.Sprintf("/forms/%d", id)}).Body.String()
	offBContains(t, "form page", page, `enctype="multipart/form-data"`, `type="file" name="cv"`, `accept=".jpg,`)

	post := func(filename string, content []byte) *httpRec {
		t.Helper()
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		_ = w.WriteField("name", "Mali")
		if filename != "" {
			fw, _ := w.CreateFormFile("cv", filename)
			_, _ = fw.Write(content)
		}
		_ = w.Close()
		r := e.offBSend(offBReq{method: "POST", path: fmt.Sprintf("/forms/%d/submit", id), ip: fmt.Sprintf("203.0.113.%d", len(filename)+60),
			body: &b, hdr: map[string]string{"Content-Type": w.FormDataContentType(), "Accept": "application/json"}})
		return &httpRec{code: r.Code, body: r.Body.String(), hdr: r.Header()}
	}
	pdf := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), 2000)...)
	for name, c := range map[string][]byte{
		"":          nil,                                 // the file is required
		"page.html": []byte("<script>alert(1)</script>"), // not a type taken
		"fake.pdf":  []byte("MZ not a pdf at all"),       // bytes do not match
	} {
		if r := post(name, c); r.code != 400 {
			t.Fatalf("%q accepted: %d %s", name, r.code, r.body)
		}
	}
	if r := post("../ประวัติ ส่วนตัว.pdf", pdf); r.code != 200 || !strings.Contains(r.body, `"ok":true`) {
		t.Fatalf("pdf: %d %s %s", r.code, r.body, logsOf(t, admin, "forms"))
	}
	code, subs := call("forms:submissions", map[string]any{"formId": id})
	list, _ := subs["data"].([]any)
	if code != 200 || len(list) != 1 {
		t.Fatalf("submissions: %d %v", code, subs)
	}
	sub := list[0].(map[string]any)
	data := sub["data"].(map[string]any)
	if data["cv"] != "ประวัติ-ส่วนตัว.pdf" || !strings.HasPrefix(fmt.Sprint(data["_files"]), `{"cv":"forms/`) {
		t.Fatalf("stored: %v", data)
	}
	sid := int(sub["id"].(float64))
	d := adminDownload(t, e.toSample(), admin, "forms:forms", fmt.Sprintf("file=%d&key=cv", sid))
	if d.Code != 200 || !bytes.Equal(d.Body.Bytes(), pdf) || d.Header().Get("Content-Type") != "application/pdf" ||
		!strings.Contains(d.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %d %v", d.Code, d.Header())
	}
	// the CSV lists the name, not the private path
	if csv := adminDownload(t, e.toSample(), admin, "forms:forms", fmt.Sprintf("form=%d", id)).Body.String(); !strings.Contains(csv, "ประวัติ-ส่วนตัว.pdf") || strings.Contains(csv, "forms/") {
		t.Fatalf("csv: %s", csv)
	}
	call("forms:deleteSubmission", map[string]any{"id": sid})
	if d := adminDownload(t, e.toSample(), admin, "forms:forms", fmt.Sprintf("file=%d&key=cv", sid)); d.Code != 404 {
		t.Fatalf("file after delete: %d", d.Code)
	}
}

// forms 2.2: a form block on a post's page carries which page it was sent from ("enquire about this listing"):
// a hidden field in the block, kept with the answers, shown in the inbox data and the CSV.
func TestOfficialFormsSentFrom(t *testing.T) {
	e := offBNew(t)
	admin, _ := setupSite(t, e.c)
	offBInstall(t, admin, "forms")
	_, call := adminPage(t, e.toSample(), admin, "forms:forms", "")
	_, out := call("forms:save", map[string]any{"form": map[string]any{"name": "Enquiry", "fields": []any{map[string]any{"key": "name", "label": "Name", "type": "text"}}}})
	id := fmt.Sprint(out["data"].(map[string]any)["id"])
	_, pg := admin.do("POST", "/api/v1/posts", map[string]any{"type": "page", "title": "Condo Sukhumvit", "slug": "condo", "status": "publish",
		"design": map[string]any{"id": "root", "type": "frame", "children": []any{
			map[string]any{"id": "pb", "type": "pluginBlock", "props": map[string]any{"plugin": "forms", "block": "form", "values": map[string]any{"formId": id}}}}}})
	if pg["id"] == nil {
		t.Fatalf("page: %v", pg)
	}
	html := e.offBSend(offBReq{path: "/condo"}).Body.String()
	if !strings.Contains(html, `name="_about" value="Condo Sukhumvit — /condo"`) {
		t.Fatalf("no hidden page field: %s", html[strings.Index(html, "rea-form"):][:600])
	}
	r := e.offBSend(offBReq{method: "POST", path: "/forms/" + id + "/submit", ip: "203.0.113.90", body: strings.NewReader("name=Mali&_about=Condo+Sukhumvit+%E2%80%94+%2Fcondo"),
		hdr: map[string]string{"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"}})
	if r.Code != 200 {
		t.Fatalf("submit: %d %s", r.Code, r.Body.String())
	}
	_, subs := call("forms:submissions", map[string]any{"formId": id})
	if s := fmt.Sprint(subs["data"]); !strings.Contains(s, "_about:Condo Sukhumvit — /condo") {
		t.Fatalf("stored: %s", s)
	}
	if csv := adminDownload(t, e.toSample(), admin, "forms:forms", "form="+id).Body.String(); !strings.Contains(csv, "sentFrom") || !strings.Contains(csv, "Condo Sukhumvit") {
		t.Fatalf("csv: %s", csv)
	}
}
