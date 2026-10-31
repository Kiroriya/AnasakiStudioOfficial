package api

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/config"
	"github.com/anasaki/studio/backend/internal/demo"
	"github.com/anasaki/studio/backend/internal/kses"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

// deniedExt can never be uploaded, whatever allowed_upload_types says.
var deniedExt = kses.DeniedExt

// extMime maps extensions to the stored/served content type (the OS mime
// table differs between Windows and Linux, so the common ones are pinned).
var extMime = map[string]string{
	"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif", "webp": "image/webp",
	"svg": "image/svg+xml", "mp4": "video/mp4", "webm": "video/webm", "mp3": "audio/mpeg", "wav": "audio/wav",
	"ogg": "audio/ogg", "pdf": "application/pdf", "doc": "application/msword",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xls":  "application/vnd.ms-excel", "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"ppt": "application/vnd.ms-powerpoint", "pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"odt": "application/vnd.oasis.opendocument.text", "ods": "application/vnd.oasis.opendocument.spreadsheet",
	"odp": "application/vnd.oasis.opendocument.presentation", "txt": "text/plain", "csv": "text/csv",
	"rtf": "application/rtf", "zip": "application/zip", "rar": "application/vnd.rar", "7z": "application/x-7z-compressed",
}

var extRe = regexp.MustCompile(`^[a-z0-9]{1,10}$`)

// parseUploadTypes normalises a comma separated extension list; the message
// is non-empty when an entry is invalid or always denied.
func parseUploadTypes(v string) ([]string, string) {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range strings.Split(v, ",") {
		e = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(e)), ".")
		if e == "" || seen[e] {
			continue
		}
		if !extRe.MatchString(e) {
			return nil, "นามสกุลไฟล์ไม่ถูกต้อง: " + e
		}
		if deniedExt[e] {
			return nil, "ไม่อนุญาตให้อัปโหลดไฟล์ ." + e + " เพื่อความปลอดภัย"
		}
		seen[e] = true
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, "กรุณาระบุนามสกุลไฟล์ที่อนุญาตอย่างน้อย 1 รายการ"
	}
	return out, ""
}

// uploadAllowed decides whether a file with ext may be uploaded; "" = ok.
func uploadAllowed(ext string, allowed []string, canUnfiltered bool) string {
	if ext == "" || deniedExt[ext] {
		return "ไม่อนุญาตให้อัปโหลดไฟล์ประเภทนี้"
	}
	ok := false
	for _, a := range allowed {
		if a == ext {
			ok = true
			break
		}
	}
	if !ok {
		return "ไม่รองรับไฟล์ ." + ext + " (ตั้งค่าได้ที่ การตั้งค่า → ไฟล์ที่อนุญาต)"
	}
	if ext == "svg" && !canUnfiltered {
		return "เฉพาะผู้ดูแลระบบหรือบรรณาธิการเท่านั้นที่อัปโหลดไฟล์ SVG ได้"
	}
	return ""
}

// contentMatches rejects files whose bytes contradict their extension
// (e.g. an HTML page renamed to .jpg).
func contentMatches(ext, sniffed string) bool {
	switch model.FileKind(ext) {
	case "image":
		return ext == "svg" || strings.HasPrefix(sniffed, "image/")
	}
	if strings.HasPrefix(sniffed, "text/html") {
		return ext == "txt" || ext == "csv" || ext == "md"
	}
	return true
}

func mimeFor(ext string) string {
	if m, ok := extMime[ext]; ok {
		return m
	}
	if m := mime.TypeByExtension("." + ext); m != "" {
		return strings.Split(m, ";")[0]
	}
	return "application/octet-stream"
}

func (a *API) uploadLimits() (maxMB int64, types []string) {
	opts, _ := a.st.Options()
	o := withDefaults(opts)
	maxMB, _ = strconv.ParseInt(o["max_upload_mb"], 10, 64)
	if maxMB < 1 || maxMB > 512 {
		maxMB = 32
	}
	types, msg := parseUploadTypes(o["allowed_upload_types"])
	if msg != "" {
		types, _ = parseUploadTypes(DefaultUploadTypes)
	}
	return maxMB, types
}

func (a *API) listMedia(w http.ResponseWriter, r *http.Request) {
	ms, err := a.st.ListMedia()
	if err != nil {
		serverErr(w, err)
		return
	}
	kind := r.URL.Query().Get("kind")
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	out := []model.Media{}
	for _, m := range ms {
		if kind != "" && m.Kind != kind {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(m.Filename+" "+m.Alt), search) {
			continue
		}
		out = append(out, m)
	}
	// ?page= switches to a paged answer {items, total}; without it the full
	// list is returned (media picker, designer).
	if p := r.URL.Query().Get("page"); p != "" {
		page, _ := strconv.Atoi(p)
		per, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
		if page < 1 {
			page = 1
		}
		if per < 1 || per > 200 {
			per = 40
		}
		from := min((page-1)*per, len(out))
		writeJSON(w, http.StatusOK, map[string]any{"items": out[from:min(from+per, len(out))], "total": len(out)})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) uploadMedia(w http.ResponseWriter, r *http.Request) {
	maxMB, types := a.uploadLimits()
	limit := maxMB << 20
	if a.visitor(r) { // demo site: small images only
		limit = demo.MaxUploadBytes
		types, _ = parseUploadTypes(demo.UploadTypes)
	}
	tooBig := fmt.Sprintf("ไฟล์ใหญ่เกินไป (ไม่เกิน %.0fMB)", float64(limit)/(1<<20))
	r.Body = http.MaxBytesReader(w, r.Body, limit+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, tooBig)
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "กรุณาเลือกไฟล์")
		return
	}
	defer f.Close()
	if hdr.Size > limit {
		writeErr(w, http.StatusBadRequest, tooBig)
		return
	}
	a.saveUpload(w, r, f, hdr.Filename, r.FormValue("alt"), types)
}

// saveUpload checks a file (by extension and content) and stores it in the
// media library, answering 201 with the new media item.
func (a *API) saveUpload(w http.ResponseWriter, r *http.Request, f io.ReadSeeker, filename, alt string, types []string) {
	me := auth.From(r)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if msg := uploadAllowed(ext, types, me.Can(auth.CapUnfilteredHTML)); msg != "" {
		status := http.StatusBadRequest
		if ext == "svg" {
			status = http.StatusForbidden
		}
		writeErr(w, status, msg)
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	if !contentMatches(ext, http.DetectContentType(head[:n])) {
		writeErr(w, http.StatusBadRequest, "เนื้อหาไฟล์ไม่ตรงกับนามสกุล ."+ext)
		return
	}

	base := store.Slugify(strings.TrimSuffix(filename, filepath.Ext(filename)))
	dir := time.Now().Format("2006/01")
	if err := os.MkdirAll(filepath.Join(a.uploadDir, dir), 0o755); err != nil {
		serverErr(w, err)
		return
	}
	name := fmt.Sprintf("%s-%d.%s", base, time.Now().UnixNano()%1e9, ext)
	dst, err := os.Create(filepath.Join(a.uploadDir, dir, name))
	if err != nil {
		serverErr(w, err)
		return
	}
	defer dst.Close()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		serverErr(w, err)
		return
	}
	size, err := io.Copy(dst, f)
	if err != nil {
		serverErr(w, err)
		return
	}
	m := &model.Media{Filename: filepath.Base(filename), URL: "/uploads/" + dir + "/" + name, Mime: mimeFor(ext),
		Size: size, AuthorID: me.ID, Alt: alt}
	if model.FileKind(ext) == "image" {
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			if cfg, _, err := image.DecodeConfig(f); err == nil {
				m.Width, m.Height = cfg.Width, cfg.Height
			}
		}
	}
	if err := a.st.CreateMedia(m); err != nil {
		serverErr(w, err)
		return
	}
	a.plugins.Emit("media.uploaded", map[string]any{"id": m.ID, "filename": m.Filename, "url": m.URL, "mime": m.Mime, "size": m.Size})
	writeJSON(w, http.StatusCreated, m)
}

// importMedia downloads a file from a public http(s) URL into the media
// library: POST /media/import {url, alt}. The same type and size rules as
// uploads apply, and private/internal addresses are refused.
func (a *API) importMedia(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
		Alt string `json:"alt"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if a.visitor(r) {
		demoDenied(w)
		return
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeErr(w, http.StatusBadRequest, "ลิงก์ไม่ถูกต้อง (ต้องขึ้นต้นด้วย http:// หรือ https://)")
		return
	}
	maxMB, types := a.uploadLimits()
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ลิงก์ไม่ถูกต้อง")
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AnasakiStudio/"+config.Version+"; media import)")
	resp, err := importClient.Do(req)
	if err != nil {
		msg := "ดาวน์โหลดไฟล์ไม่สำเร็จ"
		if errors.Is(err, errPrivateAddr) {
			msg = "ไม่อนุญาตให้ดึงไฟล์จากที่อยู่ภายในเครือข่าย"
		}
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("ดาวน์โหลดไฟล์ไม่สำเร็จ (HTTP %d)", resp.StatusCode))
		return
	}
	tmp, err := os.CreateTemp("", "anasaki-import-*")
	if err != nil {
		serverErr(w, err)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxMB<<20+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ดาวน์โหลดไฟล์ไม่สำเร็จ")
		return
	}
	if n > maxMB<<20 {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("ไฟล์ใหญ่เกินไป (ไม่เกิน %dMB)", maxMB))
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		serverErr(w, err)
		return
	}
	a.saveUpload(w, r, tmp, importName(u, resp.Header.Get("Content-Type")), in.Alt, types)
}

// importName picks a file name for a downloaded file: the URL's last path
// segment, with an extension taken from the content type when it has none.
func importName(u *url.URL, contentType string) string {
	name := path.Base(u.Path)
	if name == "." || name == "/" {
		name = "file"
	}
	if dec, err := url.PathUnescape(name); err == nil {
		name = dec
	}
	if filepath.Ext(name) == "" {
		ct := strings.TrimSpace(strings.Split(contentType, ";")[0])
		for ext, m := range extMime {
			if m == ct && ext != "jpeg" {
				return name + "." + ext
			}
		}
	}
	return name
}

var errPrivateAddr = errors.New("private address")

// blockedNets are internal ranges the net.IP helpers do not cover.
var blockedNets = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "2002::/16"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

// internalIP reports whether ip must not be fetched by the server.
func internalIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// importClient refuses loopback, private, link-local and other internal
// addresses (checked on the resolved IP, so DNS tricks and redirects are covered).
var importClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				if internalIP(net.ParseIP(host)) {
					return errPrivateAddr
				}
				return nil
			},
		}).DialContext,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("bad redirect")
		}
		return nil
	},
}

// download serves GET /download/{id} (public): the media file as an
// attachment, counting the download.
func (a *API) download(w http.ResponseWriter, r *http.Request) {
	m, err := a.st.GetMedia(pathID(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel, ok := strings.CutPrefix(m.URL, "/uploads/")
	if !ok || strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(a.uploadDir, filepath.FromSlash(rel)))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			serverErr(w, err)
			return
		}
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}
	rng := r.Header.Get("Range")
	if r.Method == http.MethodGet && (rng == "" || strings.HasPrefix(rng, "bytes=0-")) {
		_ = a.st.IncMediaDownloads(m.ID)
	}
	w.Header().Set("Content-Type", m.Mime)
	w.Header().Set("Content-Disposition", ContentDisposition(m.Filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

// ContentDisposition builds an attachment header with an ASCII fallback
// and the RFC 5987 UTF-8 file name.
func ContentDisposition(name string) string {
	var ascii, enc strings.Builder
	for _, b := range []byte(name) {
		c := rune(b)
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', strings.ContainsRune("!#$&+-.^_`|~", c):
			enc.WriteByte(b)
		default:
			fmt.Fprintf(&enc, "%%%02X", b)
		}
		if b >= 0x20 && b < 0x7f && b != '"' && b != '\\' {
			ascii.WriteByte(b)
		} else if b < 0x80 {
			ascii.WriteByte('_')
		}
	}
	fallback := strings.TrimSpace(ascii.String())
	ext := filepath.Ext(fallback)
	if strings.Trim(strings.TrimSuffix(fallback, ext), "._- ") == "" {
		fallback = "download" + ext
	}
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + enc.String()
}
