// Package media stores files in the media library (CMS 2.1.0, P1-2): the
// type and content checks, the file on disk under the upload directory and
// the media row. The admin's uploads (REST) and code (Site Code / JS plugins:
// media.upload, media.replace, media.delete) use the same rules.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // image sizes
	_ "image/jpeg" // image sizes
	_ "image/png"  // image sizes
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anasaki/studio/backend/internal/kses"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

// DefaultUploadTypes is the default allowed_upload_types option.
const DefaultUploadTypes = "jpg,jpeg,png,gif,webp,svg,mp4,webm,mp3,wav,ogg,pdf,doc,docx,xls,xlsx,ppt,pptx,odt,ods,odp,txt,csv,rtf,zip,rar,7z"

// DefaultMaxMB is the default max_upload_mb option.
const DefaultMaxMB = 32

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

// MimeFor is the content type stored for an extension.
func MimeFor(ext string) string {
	if m, ok := extMime[ext]; ok {
		return m
	}
	if m := mime.TypeByExtension("." + ext); m != "" {
		return strings.Split(m, ";")[0]
	}
	return "application/octet-stream"
}

// ExtFor is an extension for a content type ("" when unknown).
func ExtFor(contentType string) string {
	ct := strings.TrimSpace(strings.Split(contentType, ";")[0])
	for ext, m := range extMime {
		if m == ct && ext != "jpeg" {
			return ext
		}
	}
	return ""
}

// ParseTypes normalises a comma separated extension list; the message is
// non-empty when an entry is invalid or always denied.
func ParseTypes(v string) ([]string, string) {
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
		if kses.DeniedExt[e] {
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

// Allowed decides whether a file with ext may be uploaded; "" = ok.
func Allowed(ext string, allowed []string, canUnfiltered bool) string {
	if ext == "" || kses.DeniedExt[ext] {
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

// ContentMatches rejects files whose bytes contradict their extension (e.g.
// an HTML page renamed to .jpg).
func ContentMatches(ext, sniffed string) bool {
	switch model.FileKind(ext) {
	case "image":
		return ext == "svg" || strings.HasPrefix(sniffed, "image/")
	}
	if strings.HasPrefix(sniffed, "text/html") {
		return ext == "txt" || ext == "csv" || ext == "md"
	}
	return true
}

// Limits reads max_upload_mb and allowed_upload_types from the site options
// (defaults for missing or invalid values).
func Limits(opts map[string]string) (maxMB int64, types []string) {
	maxMB, _ = strconv.ParseInt(opts["max_upload_mb"], 10, 64)
	if maxMB < 1 || maxMB > 512 {
		maxMB = DefaultMaxMB
	}
	types, msg := ParseTypes(opts["allowed_upload_types"])
	if msg != "" || strings.TrimSpace(opts["allowed_upload_types"]) == "" {
		types, _ = ParseTypes(DefaultUploadTypes)
	}
	return maxMB, types
}

// Error is a refused file: Status is the HTTP status to answer.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func refuse(status int, msg string) error { return &Error{Status: status, Msg: msg} }

// Meta is what media.beforeSave filters see and may change (2.1.0):
// filename and alt; returning an error refuses the file.
type Meta struct {
	Filename string
	Alt      string
	Mime     string
	Size     int64
	Source   string // "upload", "import" (URL), "code"
}

// Filter runs the media.beforeSave filters; nil = none.
type Filter func(m *Meta) error

// Input is a file to store.
type Input struct {
	File       io.ReadSeeker
	Filename   string
	Alt        string
	AuthorID   int64
	Types      []string // allowed extensions (Limits)
	Unfiltered bool     // the uploader may upload SVG (unfiltered_html)
	Source     string
}

// Save checks and stores a file in the media library. st fires
// media.uploaded (pass st.As(actor)). Refusals are *Error.
func Save(st *store.Store, uploadDir string, in Input, filter Filter) (*model.Media, error) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(in.Filename)), ".")
	if msg := Allowed(ext, in.Types, in.Unfiltered); msg != "" {
		status := http.StatusBadRequest
		if ext == "svg" {
			status = http.StatusForbidden
		}
		return nil, refuse(status, msg)
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(in.File, head)
	if !ContentMatches(ext, http.DetectContentType(head[:n])) {
		return nil, refuse(http.StatusBadRequest, "เนื้อหาไฟล์ไม่ตรงกับนามสกุล ."+ext)
	}
	size, err := in.File.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	if filter != nil {
		m := &Meta{Filename: filepath.Base(in.Filename), Alt: in.Alt, Mime: MimeFor(ext), Size: size, Source: in.Source}
		if err := filter(m); err != nil {
			return nil, refuse(http.StatusBadRequest, err.Error())
		}
		// a filter may rename the file, not change its type
		if e := strings.TrimPrefix(strings.ToLower(filepath.Ext(m.Filename)), "."); e == ext && strings.TrimSpace(m.Filename) != "" {
			in.Filename = m.Filename
		}
		in.Alt = m.Alt
	}
	base := store.Slugify(strings.TrimSuffix(filepath.Base(in.Filename), filepath.Ext(in.Filename)))
	if base == "" {
		base = "file"
	}
	dir := time.Now().Format("2006/01")
	if err := os.MkdirAll(filepath.Join(uploadDir, dir), 0o755); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%d.%s", base, time.Now().UnixNano()%1e9, ext)
	dst, err := os.Create(filepath.Join(uploadDir, dir, name))
	if err != nil {
		return nil, err
	}
	if _, err := in.File.Seek(0, io.SeekStart); err != nil {
		dst.Close()
		return nil, err
	}
	written, err := io.Copy(dst, in.File)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst.Name())
		return nil, err
	}
	m := &model.Media{Filename: filepath.Base(in.Filename), URL: "/uploads/" + dir + "/" + name, Mime: MimeFor(ext),
		Size: written, AuthorID: in.AuthorID, Alt: in.Alt}
	if model.FileKind(ext) == "image" {
		if _, err := in.File.Seek(0, io.SeekStart); err == nil {
			if cfg, _, err := image.DecodeConfig(in.File); err == nil {
				m.Width, m.Height = cfg.Width, cfg.Height
			}
		}
	}
	if err := st.CreateMedia(m); err != nil {
		_ = os.Remove(filepath.Join(uploadDir, dir, name))
		return nil, err
	}
	return m, nil
}

// Path is the file of a media item on disk ("" when its URL is not an upload).
func Path(uploadDir string, m *model.Media) string {
	rel, ok := strings.CutPrefix(m.URL, "/uploads/")
	if !ok || strings.Contains(rel, "..") {
		return ""
	}
	return filepath.Join(uploadDir, filepath.FromSlash(rel))
}

// Read returns a media file's bytes (at most max bytes).
func Read(uploadDir string, m *model.Media, max int64) ([]byte, error) {
	p := Path(uploadDir, m)
	if p == "" {
		return nil, errors.New("not an uploaded file")
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, refuse(http.StatusRequestEntityTooLarge, fmt.Sprintf("ไฟล์ใหญ่เกิน %d MB", max>>20))
	}
	return b, nil
}

// Replace writes new bytes for a media item (same URL and extension; the
// content must still match it) and updates its size and image size; st
// fires media.updated with replaced: true.
func Replace(st *store.Store, uploadDir string, m *model.Media, data []byte) (*model.Media, error) {
	p := Path(uploadDir, m)
	if p == "" {
		return nil, refuse(http.StatusBadRequest, "แทนที่ได้เฉพาะไฟล์ที่อัปโหลดไว้")
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")
	if ext == "svg" || !ContentMatches(ext, http.DetectContentType(data)) {
		return nil, refuse(http.StatusBadRequest, "เนื้อหาใหม่ไม่ตรงกับนามสกุล ."+ext)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	out := *m
	out.Size = int64(len(data))
	if model.FileKind(ext) == "image" {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			out.Width, out.Height = cfg.Width, cfg.Height
		}
	}
	if err := st.UpdateMediaFile(&out, m); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a media item and its file; st fires media.deleted.
func Delete(st *store.Store, uploadDir string, m *model.Media) error {
	if err := st.DeleteMedia(m.ID); err != nil {
		return err
	}
	if p := Path(uploadDir, m); p != "" {
		_ = os.Remove(p)
	}
	return nil
}
