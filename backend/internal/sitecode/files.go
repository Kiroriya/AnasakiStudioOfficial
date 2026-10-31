// Package sitecode runs the site's own server-side JavaScript/TypeScript
// ("Site Code", the equivalent of WordPress' functions.php): draft files
// stored in the database, esbuild bundling, published version snapshots and
// a goja sandbox serving routes on the public site and core events.
package sitecode

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// File limits (contract: phase 1).
const (
	MaxFiles     = 200
	MaxFileBytes = 512 << 10
	MaxTotal     = 2 << 20
	MaxPathLen   = 255

	// MaxUpload bounds an upload request (zip or single file).
	MaxUpload = 8 << 20
)

// TypesFile is the type definition added to project downloads (and ignored
// at the project root on upload: it is regenerated).
const TypesFile = "anasaki.d.ts"

var (
	allowedExt = map[string]bool{".ts": true, ".js": true, ".mjs": true, ".json": true, ".md": true}
	segmentRe  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// PathError is a user-facing (Thai) validation error.
type PathError struct{ Msg string }

func (e *PathError) Error() string { return e.Msg }

func perr(format string, a ...any) error { return &PathError{Msg: fmt.Sprintf(format, a...)} }

// CleanDir validates a folder path ("" = project root) and returns it
// without leading/trailing slashes.
func CleanDir(p string) (string, error) {
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "", nil
	}
	if err := checkSegments(p); err != nil {
		return "", err
	}
	return p, nil
}

// ValidPath checks a file path: relative, "/"-separated, segments of
// [A-Za-z0-9._-], no "..", an allowed extension, at most 255 characters.
func ValidPath(p string) error {
	if err := checkSegments(p); err != nil {
		return err
	}
	if !allowedExt[strings.ToLower(path.Ext(p))] {
		return perr("ไม่อนุญาตไฟล์ชนิดนี้: %s (ใช้ได้เฉพาะ .ts .js .mjs .json .md)", p)
	}
	return nil
}

func checkSegments(p string) error {
	switch {
	case p == "":
		return perr("กรุณาระบุชื่อไฟล์")
	case len(p) > MaxPathLen:
		return perr("ชื่อไฟล์ยาวเกิน %d ตัวอักษร", MaxPathLen)
	case strings.Contains(p, `\`):
		return perr("ชื่อไฟล์ต้องใช้ / คั่นโฟลเดอร์ (ห้ามใช้ \\): %s", p)
	case strings.HasPrefix(p, "/"):
		return perr("ชื่อไฟล์ต้องเป็น path แบบ relative (ห้ามขึ้นต้นด้วย /): %s", p)
	}
	for _, s := range strings.Split(p, "/") {
		if s == "" || s == "." || s == ".." {
			return perr("ชื่อไฟล์ไม่ถูกต้อง (ห้ามมี .. หรือ // ): %s", p)
		}
		if !segmentRe.MatchString(s) {
			return perr("ชื่อไฟล์ใช้ได้เฉพาะ A-Z a-z 0-9 . _ - : %s", p)
		}
	}
	return nil
}

// CheckSet validates a whole project: paths, case-insensitive duplicates,
// a file never also used as a folder, and the size limits.
func CheckSet(files map[string]string) error {
	if len(files) > MaxFiles {
		return perr("ไฟล์มากเกินไป (สูงสุด %d ไฟล์)", MaxFiles)
	}
	total := 0
	lower := map[string]string{}
	for p, c := range files {
		if err := ValidPath(p); err != nil {
			return err
		}
		if len(c) > MaxFileBytes {
			return perr("ไฟล์ %s ใหญ่เกิน %d KB", p, MaxFileBytes>>10)
		}
		total += len(c)
		if prev, dup := lower[strings.ToLower(p)]; dup {
			return perr("ชื่อไฟล์ซ้ำกัน (ตัวพิมพ์เล็ก/ใหญ่ต่างกันเท่านั้น): %s และ %s", prev, p)
		}
		lower[strings.ToLower(p)] = p
	}
	if total > MaxTotal {
		return perr("ขนาดไฟล์รวมเกิน %d MB", MaxTotal>>20)
	}
	for p := range files {
		for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
			if other, ok := lower[strings.ToLower(dir)]; ok {
				return perr("%s เป็นทั้งไฟล์และโฟลเดอร์", other)
			}
		}
	}
	return nil
}

// EntryPoint picks the bundle entry: main.ts, main.js, index.ts, index.js.
func EntryPoint(files map[string]string) string {
	for _, e := range []string{"main.ts", "main.js", "index.ts", "index.js"} {
		if _, ok := files[e]; ok {
			return e
		}
	}
	return ""
}

// Sorted returns the paths of files in order.
func Sorted(files map[string]string) []string {
	out := make([]string, 0, len(files))
	for p := range files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// junk names some zip tools add; skipped silently on upload.
func junk(p string) bool {
	base := path.Base(p)
	return strings.HasPrefix(p, "__MACOSX/") || base == ".DS_Store" || base == "Thumbs.db" || base == "desktop.ini"
}

// ReadZip extracts a project zip: a single top folder is stripped, the root
// anasaki.d.ts and OS junk are skipped, every path is validated (zip-slip)
// and the limits are enforced while reading (zip bombs).
func ReadZip(data []byte) (map[string]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, perr("ไฟล์ zip ไม่ถูกต้อง")
	}
	var names []*zip.File
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "./")
		if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") || junk(name) {
			continue
		}
		if err := checkSegments(name); err != nil { // zip-slip: absolute, ../, backslashes
			return nil, err
		}
		names = append(names, f)
	}
	if len(names) > MaxFiles+1 { // +1: anasaki.d.ts
		return nil, perr("ไฟล์มากเกินไป (สูงสุด %d ไฟล์)", MaxFiles)
	}
	// a single top folder (project-folder/…) is stripped
	strip := ""
	for i, f := range names {
		first, _, ok := strings.Cut(strings.TrimPrefix(f.Name, "./"), "/")
		if !ok {
			strip = ""
			break
		}
		if i == 0 {
			strip = first + "/"
		} else if strip != first+"/" {
			strip = ""
			break
		}
	}
	out := map[string]string{}
	total := 0
	for _, f := range names {
		name := strings.TrimPrefix(strings.TrimPrefix(f.Name, "./"), strip)
		if name == TypesFile {
			continue
		}
		if err := ValidPath(name); err != nil {
			return nil, err
		}
		if f.UncompressedSize64 > MaxFileBytes {
			return nil, perr("ไฟล์ %s ใหญ่เกิน %d KB", name, MaxFileBytes>>10)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, perr("อ่านไฟล์ใน zip ไม่ได้: %s", name)
		}
		b, err := io.ReadAll(io.LimitReader(rc, MaxFileBytes+1))
		rc.Close()
		if err != nil {
			return nil, perr("อ่านไฟล์ใน zip ไม่ได้: %s", name)
		}
		if len(b) > MaxFileBytes {
			return nil, perr("ไฟล์ %s ใหญ่เกิน %d KB", name, MaxFileBytes>>10)
		}
		total += len(b)
		if total > MaxTotal {
			return nil, perr("ขนาดไฟล์รวมเกิน %d MB", MaxTotal>>20)
		}
		out[name] = string(b)
	}
	if err := CheckSet(out); err != nil {
		return nil, err
	}
	return out, nil
}

// zipTime is the fixed modification time of downloaded entries, so the same
// files always give the same zip.
var zipTime = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// WriteZip writes files (sorted) as a zip; prefix is prepended to every name.
func WriteZip(w io.Writer, files map[string]string, prefix string) error {
	zw := zip.NewWriter(w)
	for _, p := range Sorted(files) {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: prefix + p, Method: zip.Deflate, Modified: zipTime})
		if err != nil {
			return err
		}
		if _, err := io.WriteString(fw, files[p]); err != nil {
			return err
		}
	}
	return zw.Close()
}

// Diff compares an incoming set with the draft. With replace, draft files
// under scope ("" = all) missing from incoming are removed.
type Diff struct {
	Added     []string `json:"added"`
	Changed   []string `json:"changed"`
	Removed   []string `json:"removed"`
	Unchanged int      `json:"unchanged"`
}

// ComputeDiff returns the diff and the resulting draft.
func ComputeDiff(draft, incoming map[string]string, replace bool, scope string) (Diff, map[string]string) {
	d := Diff{Added: []string{}, Changed: []string{}, Removed: []string{}}
	result := make(map[string]string, len(draft)+len(incoming))
	for p, c := range draft {
		result[p] = c
	}
	for _, p := range Sorted(incoming) {
		old, ok := draft[p]
		switch {
		case !ok:
			d.Added = append(d.Added, p)
		case old != incoming[p]:
			d.Changed = append(d.Changed, p)
		default:
			d.Unchanged++
		}
		result[p] = incoming[p]
	}
	if replace {
		for _, p := range Sorted(draft) {
			if _, ok := incoming[p]; ok {
				continue
			}
			if scope == "" || strings.HasPrefix(p, scope+"/") {
				d.Removed = append(d.Removed, p)
				delete(result, p)
			}
		}
	}
	return d, result
}
