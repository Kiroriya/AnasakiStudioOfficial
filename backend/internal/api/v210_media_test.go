package api

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/anasaki/studio/backend/internal/sitecode"
)

// CMS 2.1.0 (P1-2): the media library from code (media.*), media.beforeSave,
// media.resize / replace, the code's own files (files.*) and multipart
// request bodies (request.files / form).

func bigPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestCodeMedia(t *testing.T) {
	c, code, _ := newCodeAPI(t, nil)
	admin, _ := setupSite(t, c)
	old := sitecode.FilesQuota
	sitecode.FilesQuota = 4000
	t.Cleanup(func() { sitecode.FilesQuota = old })
	publishCode(t, admin, map[string]string{"main.ts": `
site.filter('media.beforeSave', (m, ctx) => {
  if (m.filename.includes('blocked')) return { error: 'ชื่อไฟล์ต้องห้าม' }
  return { ...m, filename: m.filename.replace('raw-', ''), alt: m.alt || ('auto ' + ctx.kind) }
})
site.on('media.uploaded', (m) => { cache.set('up:' + m.id, m.actor.source) })
site.route('GET /up-src/{id}', { access: ['*'] }, ({ request }) => cache.get('up:' + request.params.id) ?? null)
site.route('POST /up', { access: ['*'] }, ({ request }) => db.asSystem(() => media.upload(request.json())))
site.route('POST /visitor-up', { access: ['*'] }, ({ request }) => media.upload(request.json()))
site.route('GET /media/{id}', { access: ['*'] }, ({ request }) => {
  const id = Number(request.params.id)
  const m = media.get(id), b = media.read(id)
  return { m, len: b.length, list: media.list({ kind: 'image' }).map((x) => x.id) }
})
site.route('POST /shrink/{id}', { access: ['*'] }, ({ request }) => db.asSystem(() => {
  const id = Number(request.params.id)
  const r = media.resize(media.read(id), { maxWidth: 100 })
  const m = media.replace(id, r.bytes)
  media.update(id, { alt: 'shrunk' })
  return { w: r.width, h: r.height, m: media.get(id) }
}))
site.route('POST /del/{id}', { access: ['*'] }, ({ request }) => db.asSystem(() => media.delete(Number(request.params.id))))
site.route('POST /files2', { access: ['*'] }, () => {
  files.put('notes/a.txt', 'hello')
  const pub = files.put('pub/data.json', '{"a":1}', { public: true })
  let refusedExt = false, refusedQuota = false
  try { files.put('pub/page.html', '<b>x</b>', { public: true }) } catch (e) { refusedExt = true }
  try { files.put('big.bin', 'x'.repeat(5000)) } catch (e) { refusedQuota = true }
  return { pub, text: files.text('notes/a.txt'), list: files.list().map((f) => f.path + ':' + f.public), url: files.url('pub/data.json'),
    priv: files.url('notes/a.txt'), refusedExt, refusedQuota, del: files.delete('notes/a.txt'), after: files.text('notes/a.txt') }
})
site.route('POST /mp', { access: ['*'], maxBody: 2000000 }, ({ request }) => db.asSystem(() => {
  const f = request.files()[0], form = request.form()
  return media.upload({ bytes: f.bytes, filename: f.filename, alt: form.alt })
}))
`})
	png64 := base64.StdEncoding.EncodeToString(bigPNG(300, 200))
	code1, m, raw := postJSON(c, "/up", map[string]any{"base64": png64, "filename": "raw-photo.png"})
	if code1 != 200 || m["filename"] != "photo.png" || m["alt"] != "auto image" || m["width"] != 300.0 || m["kind"] != "image" {
		t.Fatalf("upload: %d %s", code1, raw)
	}
	mid := id(m)
	code.WaitEvents()
	if _, b := c.public("GET", "/up-src/"+mid); !strings.Contains(b, "code") {
		t.Fatalf("media.uploaded actor: %s", b)
	}
	if code, _, _ := postJSON(c, "/up", map[string]any{"base64": png64, "filename": "blocked.png"}); code != 500 {
		t.Fatalf("filter refusal: %d", code)
	}
	if code, _, _ := postJSON(c, "/up", map[string]any{"base64": base64.StdEncoding.EncodeToString([]byte("<html>")), "filename": "x.jpg"}); code != 500 {
		t.Fatalf("content mismatch: %d", code)
	}
	if code, _, _ := postJSON(c, "/visitor-up", map[string]any{"base64": png64, "filename": "v.png"}); code != 500 {
		t.Fatalf("a visitor uploads: %d", code)
	}
	if _, b := c.public("GET", "/media/"+mid); !strings.Contains(b, `"len":`) || !strings.Contains(b, `"list":[`+mid+`]`) {
		t.Fatalf("get/read/list: %s", b)
	}
	code2, s, raw := postJSON(c, "/shrink/"+mid, nil)
	if code2 != 200 || s["w"] != 100.0 || s["h"] != 66.0 || s["m"].(map[string]any)["width"] != 100.0 || s["m"].(map[string]any)["alt"] != "shrunk" {
		t.Fatalf("resize/replace: %d %s", code2, raw)
	}
	// the admin's upload runs the same filter
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "blocked-too.png")
	_, _ = fw.Write(tinyPNG)
	_ = mw.Close()
	if rec := admin.send("POST", "/api/v1/media", &buf, map[string]string{"Content-Type": mw.FormDataContentType()}); rec.Code != 400 || !strings.Contains(rec.Body.String(), "ต้องห้าม") {
		t.Fatalf("admin upload filter: %d %s", rec.Code, rec.Body.String())
	}
	// multipart route
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	_ = mw.WriteField("alt", "from a form")
	fw, _ = mw.CreateFormFile("file", "form.png")
	_, _ = fw.Write(tinyPNG)
	_ = mw.Close()
	rec := c.send("POST", "/mp", &buf, map[string]string{"Content-Type": mw.FormDataContentType(), "Accept": "application/json"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"alt":"from a form"`) {
		t.Fatalf("request.files: %d %s", rec.Code, rec.Body.String())
	}
	// files
	_, f, raw := postJSON(c, "/files2", nil)
	if f["text"] != "hello" || f["url"] != "/uploads/plugins/site/pub/data.json" || f["priv"] != nil || f["refusedExt"] != true ||
		f["refusedQuota"] != true || f["del"] != true || f["after"] != nil || jsonString(f["list"]) != `["notes/a.txt:false","pub/data.json:true"]` {
		t.Fatalf("files: %s", raw)
	}
	// delete removes the file
	if code, _, raw := postJSON(c, "/del/"+mid, nil); code != 200 {
		t.Fatalf("delete: %d %s", code, raw)
	}
	if code, _ := admin.do("GET", "/api/v1/media", nil); code != 200 {
		t.Fatal("media list")
	}
}
