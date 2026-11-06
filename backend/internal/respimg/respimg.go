// Package respimg gives uploaded photos several sizes for different screens
// (CMS 2.2.2): pages point at /uploads/<file>?w=<width> in a srcset, and the
// uploads handler makes that size on the first request and keeps it under
// <upload dir>/.sizes/<width>/<file>. Only JPEG and PNG are resized; the
// original is served whenever it is not wider than the size asked for.
package respimg

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Widths are the only sizes made (px), so a visitor cannot fill the disk with
// arbitrary ones. 2.3.0 added the small ones for logos and icons.
var Widths = []int{96, 160, 320, 480, 768, 1200, 1600}

// CacheDir is the folder under the upload dir that keeps the made sizes.
const CacheDir = ".sizes"

// maxPixels: larger images are not decoded (memory); they are served as they are.
const maxPixels = 40_000_000

func resizableExt(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".jpg", ".jpeg", ".png":
		return true
	}
	return false
}

// Resizable reports whether src is an uploaded JPEG/PNG (/uploads/…, no query).
func Resizable(src string) bool {
	rel, ok := strings.CutPrefix(src, "/uploads/")
	return ok && rel != "" && !strings.ContainsAny(rel, "?#\\") && !strings.Contains(rel, "..") && resizableExt(rel)
}

// Srcset is the srcset of an upload ("" when it gets none).
func Srcset(src string) string {
	if !Resizable(src) {
		return ""
	}
	parts := make([]string, len(Widths))
	for i, w := range Widths {
		n := strconv.Itoa(w)
		parts[i] = src + "?w=" + n + " " + n + "w"
	}
	return strings.Join(parts, ", ")
}

// Sizes is the sizes attribute that goes with Srcset: browsers that know
// `auto` use the image's laid-out width (lazy images), others assume full width.
const Sizes = "auto, 100vw"

var imgTag = regexp.MustCompile(`(?i)<img\b[^>]*>`)
var srcAttr = regexp.MustCompile(`(?i)\ssrc\s*=\s*"([^"]*)"`)
var hasSrcset = regexp.MustCompile(`(?i)\ssrcset\s*=`)
var hasLoading = regexp.MustCompile(`(?i)\sloading\s*=`)

// AddToHTML adds srcset/sizes (and lazy loading) to the uploaded photos in
// post content that do not have a srcset yet.
func AddToHTML(s string) string {
	if !strings.Contains(s, "/uploads/") {
		return s
	}
	return imgTag.ReplaceAllStringFunc(s, func(tag string) string {
		if hasSrcset.MatchString(tag) {
			return tag
		}
		m := srcAttr.FindStringSubmatch(tag)
		if m == nil {
			return tag
		}
		set := Srcset(m[1])
		if set == "" {
			return tag
		}
		extra := ` srcset="` + set + `" sizes="` + Sizes + `"`
		if !hasLoading.MatchString(tag) {
			extra += ` loading="lazy"`
		}
		return tag[:4] + extra + tag[4:]
	})
}

// Handler serves /uploads/ (path already stripped of the prefix): a request
// with ?w=<one of Widths> for a JPEG/PNG gets that size, everything else goes
// to next unchanged.
func Handler(uploadDir string, next http.Handler) http.Handler {
	h := &handler{dir: uploadDir, next: next, sem: make(chan struct{}, 2)}
	return h
}

type handler struct {
	dir   string
	next  http.Handler
	sem   chan struct{} // at most two resizes at a time
	locks sync.Map      // cache path → *sync.Mutex
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("w")
	width, _ := strconv.Atoi(q)
	rel := path.Clean("/" + r.URL.Path)[1:]
	if q == "" || !allowed(width) || !resizableExt(rel) || rel == "" || strings.HasPrefix(rel, CacheDir+"/") {
		h.next.ServeHTTP(w, r)
		return
	}
	orig := filepath.Join(h.dir, filepath.FromSlash(rel))
	oi, err := os.Stat(orig)
	if err != nil || oi.IsDir() {
		h.next.ServeHTTP(w, r)
		return
	}
	cache := filepath.Join(h.dir, CacheDir, strconv.Itoa(width), filepath.FromSlash(rel))
	if p, ok := h.sized(orig, oi.ModTime(), cache, width); ok {
		serveFile(w, r, p)
		return
	}
	h.next.ServeHTTP(w, r)
}

func allowed(w int) bool {
	for _, x := range Widths {
		if x == w {
			return true
		}
	}
	return false
}

// sized answers the file to serve: the cached size (made when missing or
// not stamped with the original's modification time) or the original itself; ok false = serve as usual.
func (h *handler) sized(orig string, origMod time.Time, cache string, width int) (string, bool) {
	if ci, err := os.Stat(cache); err == nil && ci.ModTime().Equal(origMod) {
		return cache, true
	}
	mu, _ := h.locks.LoadOrStore(cache, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	if ci, err := os.Stat(cache); err == nil && ci.ModTime().Equal(origMod) {
		return cache, true // made by the request we waited for
	}
	data, err := os.ReadFile(orig)
	if err != nil {
		return "", false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= width || cfg.Width*cfg.Height > maxPixels {
		return orig, err == nil
	}
	h.sem <- struct{}{}
	out, _, err := resize(data, width)
	<-h.sem
	if err != nil {
		return orig, true
	}
	if len(out) >= len(data) {
		out = data // a simple image can grow when re-encoded: keep the original bytes for this size
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return orig, true
	}
	tmp := cache + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return orig, true
	}
	if err := os.Rename(tmp, cache); err != nil {
		_ = os.Remove(tmp)
		return orig, true
	}
	// the copy carries the original's time: any change of the original (a replace, even one racing this
	// resize) makes the times differ and the size is made again
	_ = os.Chtimes(cache, origMod, origMod)
	return cache, true
}

func serveFile(w http.ResponseWriter, r *http.Request, p string) {
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeContent(w, r, p, fi.ModTime(), f)
}

// Forget removes the made sizes of an upload URL (when the file is deleted).
func Forget(uploadDir, url string) {
	rel, ok := strings.CutPrefix(url, "/uploads/")
	if !ok || strings.Contains(rel, "..") || !resizableExt(rel) {
		return
	}
	for _, w := range Widths {
		_ = os.Remove(filepath.Join(uploadDir, CacheDir, strconv.Itoa(w), filepath.FromSlash(rel)))
	}
}

// resize makes a copy at most width px wide, in the original's format (JPEG quality 82).
func resize(data []byte, width int) ([]byte, string, error) {
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	b := src.Bounds()
	img := Scale(src, width, max(b.Dy()*width/b.Dx(), 1))
	var out bytes.Buffer
	if format == "png" {
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, img)
	} else {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 82})
	}
	return out.Bytes(), format, err
}

// Scale is an area-averaging downscale (box filter): good for shrinking.
func Scale(src image.Image, nw, nh int) image.Image {
	b := src.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, src, b.Min, draw.Src)
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	w, h := b.Dx(), b.Dy()
	for y := 0; y < nh; y++ {
		y0, y1 := y*h/nh, max((y+1)*h/nh, y*h/nh+1)
		for x := 0; x < nw; x++ {
			x0, x1 := x*w/nw, max((x+1)*w/nw, x*w/nw+1)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				off := rgba.PixOffset(b.Min.X+x0, b.Min.Y+sy)
				for sx := x0; sx < x1; sx++ {
					p := rgba.Pix[off : off+4 : off+4]
					r, g, bl, a = r+uint64(p[0]), g+uint64(p[1]), bl+uint64(p[2]), a+uint64(p[3])
					n++
					off += 4
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	return dst
}
