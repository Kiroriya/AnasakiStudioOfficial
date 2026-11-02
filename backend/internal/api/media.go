package api

import (
	"errors"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/config"
	"github.com/anasaki/studio/backend/internal/demo"
	"github.com/anasaki/studio/backend/internal/media"
	"github.com/anasaki/studio/backend/internal/model"
)

// The file rules live in internal/media (shared with Site Code, 2.1.0).
var (
	parseUploadTypes = media.ParseTypes
	uploadAllowed    = media.Allowed
	contentMatches   = media.ContentMatches
)

func (a *API) uploadLimits() (maxMB int64, types []string) {
	opts, _ := a.st.Options()
	return media.Limits(withDefaults(opts))
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
// media library (media.beforeSave filters run first), answering 201 with the
// new media item.
func (a *API) saveUpload(w http.ResponseWriter, r *http.Request, f io.ReadSeeker, filename, alt string, types []string) {
	me := auth.From(r)
	source := "upload"
	if strings.HasSuffix(r.URL.Path, "/import") {
		source = "import"
	}
	var filter media.Filter
	if a.code != nil {
		filter = func(m *media.Meta) error { return a.code.MediaBeforeSave(m, principalUser(me)) }
	}
	m, err := media.Save(a.db(r), a.uploadDir, media.Input{File: f, Filename: filename, Alt: alt, AuthorID: me.ID,
		Types: types, Unfiltered: me.Can(auth.CapUnfilteredHTML), Source: source}, filter)
	if err != nil {
		var me *media.Error
		if errors.As(err, &me) {
			writeErr(w, me.Status, me.Msg)
			return
		}
		serverErr(w, err)
		return
	}
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
		if ext := media.ExtFor(ct); ext != "" {
			return name + "." + ext
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
