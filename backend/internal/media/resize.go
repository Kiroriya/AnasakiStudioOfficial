package media

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
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
		img = scale(src, nw, nh)
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

// scale is an area-averaging downscale (box filter): good for shrinking.
func scale(src image.Image, nw, nh int) image.Image {
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

// ImageSize reads the pixel size of an image ("ok" false when not one).
func ImageSize(data []byte) (int, int, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}
