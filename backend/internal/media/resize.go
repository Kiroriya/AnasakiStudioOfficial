package media

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/anasaki/studio/backend/internal/respimg"
)

// Resize scales a JPEG or PNG down to fit maxW × maxH (0 = no limit; never
// up) and re-encodes it in the same format (JPEG at quality 1-100, default
// 82). It answers the bytes and the format ("jpeg" / "png"). Core media
// helper for plugins (media.resize, CMS 2.1.0) — e.g. an image optimizer
// that shrinks uploads; WebP / GIF are read-only here.
func Resize(data []byte, maxW, maxH, quality int) ([]byte, string, error) {
	src, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", errors.New("อ่านรูปไม่ได้ (รองรับ JPEG, PNG)")
	}
	if format != "jpeg" && format != "png" {
		return nil, "", errors.New("ย่อได้เฉพาะ JPEG และ PNG")
	}
	if quality < 1 || quality > 100 {
		quality = 82
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w, h
	if maxW > 0 && nw > maxW {
		nh, nw = nh*maxW/nw, maxW
	}
	if maxH > 0 && nh > maxH {
		nw, nh = nw*maxH/nh, maxH
	}
	nw, nh = max(nw, 1), max(nh, 1)
	img := src
	if nw != w || nh != h {
		img = respimg.Scale(src, nw, nh)
	}
	var out bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: quality})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, img)
	}
	if err != nil {
		return nil, "", err
	}
	return out.Bytes(), format, nil
}

// ImageSize reads the pixel size of an image ("ok" false when not one).
func ImageSize(data []byte) (int, int, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}
