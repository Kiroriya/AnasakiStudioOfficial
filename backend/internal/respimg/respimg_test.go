package respimg

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSrcset(t *testing.T) {
	want := "/uploads/2026/11/a.jpg?w=96 96w, /uploads/2026/11/a.jpg?w=160 160w, /uploads/2026/11/a.jpg?w=320 320w, /uploads/2026/11/a.jpg?w=480 480w, /uploads/2026/11/a.jpg?w=768 768w, /uploads/2026/11/a.jpg?w=1200 1200w, /uploads/2026/11/a.jpg?w=1600 1600w"
	if got := Srcset("/uploads/2026/11/a.jpg"); got != want {
		t.Fatalf("Srcset = %q", got)
	}
	for _, src := range []string{"/uploads/a.gif", "/uploads/a.svg", "/uploads/a.webp", "https://x.test/uploads/a.jpg", "/uploads/a.jpg?v=2", "/uploads/../a.jpg", "/img/a.png", "/uploads/"} {
		if Srcset(src) != "" {
			t.Errorf("Srcset(%q) should be empty", src)
		}
	}
	if Srcset("/uploads/B.PNG") == "" || Srcset("/uploads/c.jpeg") == "" {
		t.Error("upper-case / .jpeg should resize")
	}
}

func TestAddToHTML(t *testing.T) {
	in := `<p>x</p><img src="/uploads/a.jpg" alt="a"><img src="/uploads/b.png" srcset="/b2.png 2x"><img loading="eager" src="/uploads/c.jpg"><img src="https://cdn.test/d.jpg">`
	out := AddToHTML(in)
	if !strings.Contains(out, `<img srcset="/uploads/a.jpg?w=96 96w`) || !strings.Contains(out, `sizes="auto, 100vw" loading="lazy" src="/uploads/a.jpg" alt="a">`) {
		t.Errorf("a: %s", out)
	}
	if !strings.Contains(out, `<img src="/uploads/b.png" srcset="/b2.png 2x">`) {
		t.Errorf("own srcset kept: %s", out)
	}
	if strings.Count(out, "loading=") != 2 || !strings.Contains(out, `loading="eager" src="/uploads/c.jpg"`) {
		t.Errorf("loading kept: %s", out)
	}
	if !strings.HasSuffix(out, `<img src="https://cdn.test/d.jpg">`) {
		t.Errorf("external untouched: %s", out)
	}
	if AddToHTML("<p>no images</p>") != "<p>no images</p>" {
		t.Error("plain text changed")
	}
}

func writeJPEG(t *testing.T, p string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(h http.Handler, url string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", url, nil)
	http.StripPrefix("/uploads/", h).ServeHTTP(rec, r)
	return rec
}

func TestHandler(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, filepath.Join(dir, "2026", "big.jpg"), 2000, 1000)
	writeJPEG(t, filepath.Join(dir, "2026", "small.jpg"), 300, 200)
	h := Handler(dir, http.FileServer(http.Dir(dir)))

	rec := get(h, "/uploads/2026/big.jpg?w=768")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	cfg, format, err := image.DecodeConfig(rec.Body)
	if err != nil || format != "jpeg" || cfg.Width != 768 || cfg.Height != 384 {
		t.Fatalf("resized: %v %s %dx%d", err, format, cfg.Width, cfg.Height)
	}
	cache := filepath.Join(dir, CacheDir, "768", "2026", "big.jpg")
	if _, err := os.Stat(cache); err != nil {
		t.Fatal("size not cached")
	}

	// not wider than asked: the original, nothing cached
	rec = get(h, "/uploads/2026/small.jpg?w=480")
	cfg, _, _ = image.DecodeConfig(rec.Body)
	if rec.Code != 200 || cfg.Width != 300 {
		t.Fatalf("small: %d %d", rec.Code, cfg.Width)
	}
	if _, err := os.Stat(filepath.Join(dir, CacheDir, "480", "2026", "small.jpg")); err == nil {
		t.Error("original-size copy cached")
	}

	// other widths and paths go to the file server untouched
	rec = get(h, "/uploads/2026/big.jpg?w=999")
	cfg, _, _ = image.DecodeConfig(rec.Body)
	if cfg.Width != 2000 {
		t.Errorf("w=999 served %d", cfg.Width)
	}
	if rec := get(h, "/uploads/2026/none.jpg?w=480"); rec.Code != 404 {
		t.Errorf("missing file: %d", rec.Code)
	}
	if rec := get(h, "/uploads/../../etc/passwd.jpg?w=480"); rec.Code == 200 {
		t.Error("path escaped the upload dir")
	}

	// a replaced original (newer file) makes the size again
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(cache, old, old); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(dir, "2026", "big.jpg"), 1000, 1000)
	rec = get(h, "/uploads/2026/big.jpg?w=768")
	cfg, _, _ = image.DecodeConfig(rec.Body)
	if cfg.Width != 768 || cfg.Height != 768 {
		t.Errorf("stale size served: %dx%d", cfg.Width, cfg.Height)
	}

	Forget(dir, "/uploads/2026/big.jpg")
	if _, err := os.Stat(cache); err == nil {
		t.Error("Forget left the size")
	}
}

func TestHandlerPNG(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 1000, 500))
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	_ = os.WriteFile(filepath.Join(dir, "p.png"), b.Bytes(), 0o644)
	rec := get(Handler(dir, http.FileServer(http.Dir(dir))), "/uploads/p.png?w=480")
	cfg, format, err := image.DecodeConfig(rec.Body)
	if err != nil || format != "png" || cfg.Width != 480 || cfg.Height != 240 {
		t.Fatalf("png: %v %s %dx%d", err, format, cfg.Width, cfg.Height)
	}
}

// A flat image can grow when re-encoded: the original bytes are served (and cached) for that size.
func TestHandlerNeverLarger(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 1400, 700))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 30})
	_ = os.WriteFile(filepath.Join(dir, "flat.jpg"), b.Bytes(), 0o644)
	rec := get(Handler(dir, http.FileServer(http.Dir(dir))), "/uploads/flat.jpg?w=768")
	if rec.Code != 200 || rec.Body.Len() > b.Len() {
		t.Fatalf("flat: %d %d > %d", rec.Code, rec.Body.Len(), b.Len())
	}
}
