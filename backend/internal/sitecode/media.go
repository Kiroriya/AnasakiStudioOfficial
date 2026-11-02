package sitecode

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/media"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/plugin"
	"github.com/anasaki/studio/backend/internal/store"
)

// The media library and the code's own files (CMS 2.1.0, P1-2):
//
//	media.list / get / read           scope "media"
//	media.upload / update / replace / delete
//	                                   scope "media-write" (upload_files, or db.asSystem)
//	media.resize(bytes, opts)          JPEG / PNG down-scaling (no scope)
//	files.put / get / list / delete / url
//	                                   scope "files": the unit's own files, public ones under
//	                                   /uploads/plugins/<unit>/…, private ones in PLUGINS_DIR; quota per unit
//	request.files() / request.form()   multipart bodies of a route (maxBody)

const (
	maxMediaRead = 20 << 20
	// defaultFilesQuota is what one unit may store with files.put.
	defaultFilesQuota = 100 << 20
)

// FilesQuota is the files.put quota per unit (tests lower it).
var FilesQuota int64 = defaultFilesQuota

func (v *vm) mediaStore() *store.Store {
	st, ok := v.h.posts.(*store.Store)
	if !ok || st == nil || v.h.e == nil || v.h.e.cfg.UploadDir == "" {
		v.throw("media ไม่พร้อมใช้งานในสภาพแวดล้อมนี้")
	}
	return st.As(v.actor())
}

func mediaJSON(m *model.Media) map[string]any {
	return map[string]any{"id": m.ID, "url": m.URL, "filename": m.Filename, "mime": m.Mime, "size": m.Size, "kind": m.Kind,
		"ext": m.Ext, "width": m.Width, "height": m.Height, "alt": m.Alt, "authorId": m.AuthorID,
		"createdAt": m.CreatedAt.UTC().Format(time.RFC3339)}
}

// mediaByArg loads a media item by id.
func (v *vm) mediaByArg(st *store.Store, x goja.Value, what string) *model.Media {
	id := anyID(x.Export())
	if id < 1 {
		v.throw("%s: ต้องระบุ id ของไฟล์", what)
	}
	m, err := st.GetMedia(id)
	if err != nil {
		v.fail("%s: ไม่พบไฟล์ #%d", what, id)
	}
	return m
}

// mayChangeMedia: system code, or a user who may upload and owns the file
// (or edits others' posts) — the admin's rule.
func (v *vm) mayChangeMedia(m *model.Media) bool {
	if v.system {
		return true
	}
	return v.user != nil && v.user.Can(auth.CapUploadFiles) && (m.AuthorID == v.user.ID || v.user.Can(auth.CapEditOthersPosts))
}

// media.list({kind?, search?, ids?, limit?}) → newest first
func (v *vm) mediaList(c goja.FunctionCall) goja.Value {
	const what = "media.list"
	v.handlerOnly(what)
	v.need("media", what)
	st := v.mediaStore()
	o := optObject(v.rt, c.Argument(0))
	kind, search := optString(o, "kind"), strings.ToLower(strings.TrimSpace(optString(o, "search")))
	limit := 100
	if l := optString(o, "limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 500 {
			v.throw("%s: limit ต้องอยู่ระหว่าง 1 ถึง 500", what)
		}
		limit = n
	}
	var ids map[int64]bool
	if o != nil {
		if x := o.Get("ids"); x != nil && !goja.IsUndefined(x) && !goja.IsNull(x) {
			ids = map[int64]bool{}
			for _, id := range v.idList(x, what, "ids") {
				ids[id] = true
			}
		}
	}
	all, err := st.ListMedia()
	if err != nil {
		v.throw("%s: อ่านข้อมูลไม่สำเร็จ", what)
	}
	out := []map[string]any{}
	for i := range all {
		m := &all[i]
		if (kind != "" && m.Kind != kind) || (ids != nil && !ids[m.ID]) ||
			(search != "" && !strings.Contains(strings.ToLower(m.Filename+" "+m.Alt), search)) {
			continue
		}
		out = append(out, mediaJSON(m))
		if len(out) >= limit {
			break
		}
	}
	return v.toJS(out)
}

func (v *vm) mediaGet(c goja.FunctionCall) goja.Value {
	v.handlerOnly("media.get")
	v.need("media", "media.get")
	st := v.mediaStore()
	id := anyID(c.Argument(0).Export())
	m, err := st.GetMedia(id)
	if id < 1 || err != nil {
		return goja.Null()
	}
	return v.toJS(mediaJSON(m))
}

// media.read(id) → Uint8Array (≤ 20 MB)
func (v *vm) mediaRead(c goja.FunctionCall) goja.Value {
	const what = "media.read"
	v.handlerOnly(what)
	v.need("media", what)
	st := v.mediaStore()
	m := v.mediaByArg(st, c.Argument(0), what)
	b, err := media.Read(v.h.e.cfg.UploadDir, m, maxMediaRead)
	if err != nil {
		v.fail("%s: อ่านไฟล์ไม่สำเร็จ: %s", what, err.Error())
	}
	return v.uint8Array(b)
}

// dataArg reads bytes from a Uint8Array / ArrayBuffer, or base64 text.
func (v *vm) dataArg(o *goja.Object, what string) ([]byte, bool) {
	if b, ok := binaryOf(o.Get("bytes")); ok {
		return b, true
	}
	if s := optString(o, "base64"); s != "" {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			v.throw("%s: base64 ไม่ถูกต้อง", what)
		}
		return b, true
	}
	return nil, false
}

// media.upload({bytes | base64 | url, filename, alt?}) → media item
func (v *vm) mediaUpload(c goja.FunctionCall) goja.Value {
	const what = "media.upload"
	v.handlerOnly(what)
	v.need("media-write", what)
	st := v.mediaStore()
	if !v.system && (v.user == nil || !v.user.Can(auth.CapUploadFiles)) {
		v.fail("%s: ไม่มีสิทธิ์อัปโหลดไฟล์ (ผู้เยี่ยมชมอัปโหลดไม่ได้ — ใช้ db.asSystem เมื่อโค้ดทำแทนระบบ)", what)
	}
	o := optObject(v.rt, c.Argument(0))
	if o == nil {
		v.throw("%s: ต้องระบุ { bytes | base64 | url, filename }", what)
	}
	opts, _ := v.h.e.st.Options()
	maxMB, types := media.Limits(opts)
	filename := strings.TrimSpace(optString(o, "filename"))
	data, ok := v.dataArg(o, what)
	source := "code"
	if !ok {
		u := optString(o, "url")
		if u == "" {
			v.throw("%s: ต้องระบุ bytes, base64 หรือ url", what)
		}
		data, filename = v.fetchForMedia(u, filename, maxMB, what)
		source = "import"
	}
	if filename == "" {
		v.throw("%s: ต้องระบุ filename (มีนามสกุล เช่น photo.jpg)", what)
	}
	if int64(len(data)) > maxMB<<20 {
		v.fail("%s: ไฟล์ใหญ่เกินไป (ไม่เกิน %d MB)", what, maxMB)
	}
	author := v.h.e.systemAuthor()
	unfiltered := v.system
	if v.user != nil && !v.system {
		author, unfiltered = v.user.ID, v.user.Can(auth.CapUnfilteredHTML)
	}
	m, err := media.Save(st, v.h.e.cfg.UploadDir, media.Input{File: bytes.NewReader(data), Filename: filename, Alt: clip(optString(o, "alt"), 255),
		AuthorID: author, Types: types, Unfiltered: unfiltered, Source: source}, v.mediaFilter())
	if err != nil {
		var me *media.Error
		if errors.As(err, &me) {
			v.fail("%s: %s", what, me.Msg)
		}
		v.fail("%s: บันทึกไม่สำเร็จ", what)
	}
	return v.toJS(mediaJSON(m))
}

// mediaFilter runs media.beforeSave inside the handler (nested).
func (v *vm) mediaFilter() media.Filter {
	if len(v.filtersFor("media.beforeSave")) == 0 {
		return nil
	}
	return func(m *media.Meta) error {
		ctx := v.toJS(mediaCtx(m)).ToObject(v.rt)
		_ = ctx.Set("user", v.userValue())
		out := v.applyNested("media.beforeSave", v.toJS(map[string]any{"filename": m.Filename, "alt": m.Alt}), ctx)
		s, ok, err := v.toJSON(out)
		var res map[string]any
		if err != nil || !ok || jsonUnmarshal(s, &res) != nil {
			return nil
		}
		applyMediaMeta(m, res)
		if msg, ok := res["error"].(string); ok && msg != "" {
			return errors.New(msg)
		}
		return nil
	}
}

var mediaFetch = &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: plugin.PublicDialer(10 * time.Second).DialContext},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
			return errors.New("bad redirect")
		}
		return nil
	}}

// fetchForMedia downloads a public http(s) URL (internal addresses refused).
func (v *vm) fetchForMedia(raw, filename string, maxMB int64, what string) ([]byte, string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		v.throw("%s: url ต้องขึ้นต้นด้วย http:// หรือ https://", what)
	}
	resp, err := mediaFetch.Get(u.String())
	if err != nil {
		if plugin.IsPrivateAddrErr(err) {
			v.fail("%s: ไม่อนุญาตให้ดึงไฟล์จากที่อยู่ภายในเครือข่าย", what)
		}
		v.fail("%s: ดาวน์โหลดไม่สำเร็จ", what)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		v.fail("%s: ดาวน์โหลดไม่สำเร็จ (HTTP %d)", what, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMB<<20+1))
	if err != nil {
		v.fail("%s: ดาวน์โหลดไม่สำเร็จ", what)
	}
	if filename == "" {
		filename = path.Base(u.Path)
		if d, err := url.PathUnescape(filename); err == nil {
			filename = d
		}
		if filename == "." || filename == "/" {
			filename = "file"
		}
		if filepath.Ext(filename) == "" {
			if ext := media.ExtFor(resp.Header.Get("Content-Type")); ext != "" {
				filename += "." + ext
			}
		}
	}
	return data, filename
}

// media.update(id, {alt})
func (v *vm) mediaUpdate(c goja.FunctionCall) goja.Value {
	const what = "media.update"
	v.handlerOnly(what)
	v.need("media-write", what)
	st := v.mediaStore()
	m := v.mediaByArg(st, c.Argument(0), what)
	if !v.mayChangeMedia(m) {
		v.fail("%s: ไม่มีสิทธิ์แก้ไขไฟล์นี้", what)
	}
	o := optObject(v.rt, c.Argument(1))
	if o == nil || o.Get("alt") == nil || goja.IsUndefined(o.Get("alt")) {
		v.throw("%s: ต้องระบุ { alt }", what)
	}
	alt := clip(o.Get("alt").String(), 255)
	if err := st.UpdateMediaAlt(m.ID, alt); err != nil {
		v.fail("%s: บันทึกไม่สำเร็จ", what)
	}
	m.Alt = alt
	return v.toJS(mediaJSON(m))
}

// media.replace(id, bytes) — new content for the same file (an optimizer)
func (v *vm) mediaReplace(c goja.FunctionCall) goja.Value {
	const what = "media.replace"
	v.handlerOnly(what)
	v.need("media-write", what)
	st := v.mediaStore()
	m := v.mediaByArg(st, c.Argument(0), what)
	if !v.mayChangeMedia(m) {
		v.fail("%s: ไม่มีสิทธิ์แก้ไขไฟล์นี้", what)
	}
	data, ok := binaryOf(c.Argument(1))
	if !ok || len(data) == 0 {
		v.throw("%s: ต้องส่ง Uint8Array ของไฟล์ใหม่", what)
	}
	opts, _ := v.h.e.st.Options()
	if maxMB, _ := media.Limits(opts); int64(len(data)) > maxMB<<20 {
		v.fail("%s: ไฟล์ใหญ่เกินไป (ไม่เกิน %d MB)", what, maxMB)
	}
	out, err := media.Replace(st, v.h.e.cfg.UploadDir, m, data)
	if err != nil {
		var me *media.Error
		if errors.As(err, &me) {
			v.fail("%s: %s", what, me.Msg)
		}
		v.fail("%s: บันทึกไม่สำเร็จ", what)
	}
	return v.toJS(mediaJSON(out))
}

func (v *vm) mediaDelete(c goja.FunctionCall) goja.Value {
	const what = "media.delete"
	v.handlerOnly(what)
	v.need("media-write", what)
	st := v.mediaStore()
	m := v.mediaByArg(st, c.Argument(0), what)
	if !v.mayChangeMedia(m) {
		v.fail("%s: ไม่มีสิทธิ์ลบไฟล์นี้", what)
	}
	if err := media.Delete(st, v.h.e.cfg.UploadDir, m); err != nil {
		v.fail("%s: ลบไม่สำเร็จ", what)
	}
	return v.rt.ToValue(true)
}

// media.resize(bytes, {maxWidth?, maxHeight?, quality?}) → {bytes, format, width, height}
func (v *vm) mediaResize(c goja.FunctionCall) goja.Value {
	const what = "media.resize"
	v.handlerOnly(what)
	data, ok := binaryOf(c.Argument(0))
	if !ok {
		v.throw("%s: ต้องส่ง Uint8Array ของรูป", what)
	}
	if len(data) > maxMediaRead {
		v.fail("%s: รูปใหญ่เกิน %d MB", what, maxMediaRead>>20)
	}
	o := optObject(v.rt, c.Argument(1))
	num := func(k string) int {
		n, _ := strconv.Atoi(optString(o, k))
		return n
	}
	out, format, err := media.Resize(data, num("maxWidth"), num("maxHeight"), num("quality"))
	if err != nil {
		v.fail("%s: %s", what, err.Error())
	}
	w, h := imageSize(out)
	res := v.rt.NewObject()
	_ = res.Set("bytes", v.uint8Array(out))
	_ = res.Set("format", format)
	_ = res.Set("width", w)
	_ = res.Set("height", h)
	_ = res.Set("size", len(out))
	return res
}

// ---------------------------------------------------------------- files

var filePathRe = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._/-]{0,199}$`)

// publicFileExt: what files.put({public: true}) may serve from /uploads.
var publicFileExt = map[string]bool{"jpg": true, "jpeg": true, "png": true, "gif": true, "webp": true, "pdf": true,
	"mp3": true, "mp4": true, "webm": true, "json": true, "txt": true, "csv": true, "css": true, "woff2": true}

// fileRoots: the unit's public and private folders.
func (v *vm) fileRoots() (pub, priv string) {
	if v.h.e == nil || v.h.e.cfg.UploadDir == "" {
		v.throw("files ไม่พร้อมใช้งานในสภาพแวดล้อมนี้")
	}
	unit := "site"
	if v.u != nil && v.u.ID != "" {
		unit = v.u.ID
	}
	keyDir := v.h.e.cfg.KeyDir
	if keyDir == "" {
		keyDir = "./plugins-data"
	}
	return filepath.Join(v.h.e.cfg.UploadDir, "plugins", unit), filepath.Join(keyDir, "files", unit)
}

func (v *vm) filePath(p, what string) string {
	if !filePathRe.MatchString(p) || strings.Contains(p, "..") || strings.Contains(p, "//") || strings.HasSuffix(p, "/") {
		v.throw("%s: path ต้องเป็น a-z A-Z 0-9 . _ - / (ไม่ขึ้นต้นด้วย /, ไม่มี .., ไม่เกิน 200 ตัว)", what)
	}
	return p
}

// dirSize is the bytes stored under dir.
func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// files.put(path, data, {public?}) → {path, size, public, url?}
func (v *vm) filesPut(c goja.FunctionCall) goja.Value {
	const what = "files.put"
	v.handlerOnly(what)
	v.need("files", what)
	p := v.filePath(c.Argument(0).String(), what)
	var data []byte
	if b, ok := binaryOf(c.Argument(1)); ok {
		data = b
	} else if s, ok := c.Argument(1).Export().(string); ok {
		data = []byte(s)
	} else {
		v.throw("%s: data ต้องเป็น Uint8Array หรือข้อความ", what)
	}
	if len(data) > maxMediaRead {
		v.fail("%s: ไฟล์ใหญ่เกิน %d MB", what, maxMediaRead>>20)
	}
	public := false
	if o := optObject(v.rt, c.Argument(2)); o != nil {
		public = o.Get("public") != nil && o.Get("public").ToBoolean()
	}
	pub, priv := v.fileRoots()
	if public && !publicFileExt[strings.TrimPrefix(strings.ToLower(path.Ext(p)), ".")] {
		v.throw("%s: ไฟล์สาธารณะต้องเป็น jpg, png, gif, webp, pdf, mp3, mp4, webm, json, txt, csv, css หรือ woff2", what)
	}
	root, other := priv, pub
	if public {
		root, other = pub, priv
	}
	full := filepath.Join(root, filepath.FromSlash(p))
	var old int64
	if fi, err := os.Stat(full); err == nil {
		old = fi.Size()
	}
	if dirSize(pub)+dirSize(priv)-old+int64(len(data)) > FilesQuota {
		v.fail("%s: เกินโควตา %d MB ของไฟล์ (ลบไฟล์เก่าก่อน)", what, FilesQuota>>20)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		v.fail("%s: บันทึกไม่สำเร็จ", what)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		v.fail("%s: บันทึกไม่สำเร็จ", what)
	}
	_ = os.Remove(filepath.Join(other, filepath.FromSlash(p))) // one path, one place
	return v.toJS(v.fileInfo(p, int64(len(data)), public))
}

func (v *vm) fileInfo(p string, size int64, public bool) map[string]any {
	out := map[string]any{"path": p, "size": size, "public": public}
	if public {
		out["url"] = v.fileURL(p)
	}
	return out
}

func (v *vm) fileURL(p string) string {
	unit := "site"
	if v.u != nil && v.u.ID != "" {
		unit = v.u.ID
	}
	return "/uploads/plugins/" + unit + "/" + p
}

// find returns where p is stored ("" = nowhere) and whether it is public.
func (v *vm) findFile(p string) (string, bool) {
	pub, priv := v.fileRoots()
	for _, x := range []struct {
		dir    string
		public bool
	}{{priv, false}, {pub, true}} {
		full := filepath.Join(x.dir, filepath.FromSlash(p))
		if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
			return full, x.public
		}
	}
	return "", false
}

// files.get(path) → Uint8Array | null; files.text(path) → string | null
func (v *vm) filesGet(c goja.FunctionCall) goja.Value {
	v.handlerOnly("files.get")
	v.need("files", "files.get")
	full, _ := v.findFile(v.filePath(c.Argument(0).String(), "files.get"))
	if full == "" {
		return goja.Null()
	}
	b, err := os.ReadFile(full)
	if err != nil {
		v.fail("files.get: อ่านไม่สำเร็จ")
	}
	return v.uint8Array(b)
}

func (v *vm) filesText(c goja.FunctionCall) goja.Value {
	v.handlerOnly("files.text")
	v.need("files", "files.text")
	full, _ := v.findFile(v.filePath(c.Argument(0).String(), "files.text"))
	if full == "" {
		return goja.Null()
	}
	b, err := os.ReadFile(full)
	if err != nil {
		v.fail("files.text: อ่านไม่สำเร็จ")
	}
	return v.rt.ToValue(string(b))
}

// files.list(prefix?) → [{path, size, public, url?, modifiedAt}] by path
func (v *vm) filesList(c goja.FunctionCall) goja.Value {
	v.handlerOnly("files.list")
	v.need("files", "files.list")
	prefix := ""
	if a := c.Argument(0); !goja.IsUndefined(a) && !goja.IsNull(a) {
		prefix = a.String()
	}
	pub, priv := v.fileRoots()
	out := []map[string]any{}
	for _, x := range []struct {
		dir    string
		public bool
	}{{priv, false}, {pub, true}} {
		_ = filepath.WalkDir(x.dir, func(full string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(x.dir, full)
			rel = filepath.ToSlash(rel)
			if !strings.HasPrefix(rel, prefix) {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			it := v.fileInfo(rel, fi.Size(), x.public)
			it["modifiedAt"] = fi.ModTime().UTC().Format(time.RFC3339)
			out = append(out, it)
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["path"].(string) < out[j]["path"].(string) })
	return v.toJS(out)
}

func (v *vm) filesDelete(c goja.FunctionCall) goja.Value {
	v.handlerOnly("files.delete")
	v.need("files", "files.delete")
	full, _ := v.findFile(v.filePath(c.Argument(0).String(), "files.delete"))
	if full == "" {
		return v.rt.ToValue(false)
	}
	return v.rt.ToValue(os.Remove(full) == nil)
}

// files.url(path) → the public URL of a public file, null otherwise
func (v *vm) filesURL(c goja.FunctionCall) goja.Value {
	v.need("files", "files.url")
	p := v.filePath(c.Argument(0).String(), "files.url")
	if full, public := v.findFile(p); full == "" || !public {
		return goja.Null()
	}
	return v.rt.ToValue(v.fileURL(p))
}

// ---------------------------------------------------------------- multipart

// multipartOf parses the route's body as multipart/form-data.
func (v *vm) multipartOf(r *http.Request, what string) *multipart.Form {
	ct, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || ct != "multipart/form-data" || params["boundary"] == "" {
		v.throw("%s: request ไม่ใช่ multipart/form-data", what)
	}
	if v.form != nil && v.formReq == r {
		return v.form
	}
	body := v.readBody(r)
	f, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(int64(len(body)) + 1)
	if err != nil {
		v.throw("%s: อ่าน multipart ไม่สำเร็จ (body เกิน maxBody ของ route หรือไม่สมบูรณ์)", what)
	}
	v.form, v.formReq = f, r
	return f
}

func (v *vm) requestFiles(r *http.Request) func(goja.FunctionCall) goja.Value {
	return func(goja.FunctionCall) goja.Value {
		f := v.multipartOf(r, "request.files()")
		fields := make([]string, 0, len(f.File))
		for k := range f.File {
			fields = append(fields, k)
		}
		sort.Strings(fields)
		out := []goja.Value{}
		for _, field := range fields {
			for _, fh := range f.File[field] {
				file, err := fh.Open()
				if err != nil {
					continue
				}
				b, _ := io.ReadAll(file)
				file.Close()
				o := v.rt.NewObject()
				_ = o.Set("field", field)
				_ = o.Set("filename", filepath.Base(fh.Filename))
				_ = o.Set("type", fh.Header.Get("Content-Type"))
				_ = o.Set("size", len(b))
				_ = o.Set("bytes", v.uint8Array(b))
				out = append(out, o)
			}
		}
		return v.rt.NewArray(anySlice(out)...)
	}
}

func (v *vm) requestForm(r *http.Request) func(goja.FunctionCall) goja.Value {
	return func(goja.FunctionCall) goja.Value {
		f := v.multipartOf(r, "request.form()")
		out := map[string]any{}
		for k, vals := range f.Value {
			if len(vals) == 1 {
				out[k] = vals[0]
			} else {
				out[k] = vals
			}
		}
		return v.toJS(out)
	}
}

func anySlice(vals []goja.Value) []any {
	out := make([]any, len(vals))
	for i, x := range vals {
		out[i] = x
	}
	return out
}

func imageSize(b []byte) (int, int) {
	if w, h, ok := media.ImageSize(b); ok {
		return w, h
	}
	return 0, 0
}
