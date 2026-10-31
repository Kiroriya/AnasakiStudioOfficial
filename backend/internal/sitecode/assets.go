package sitecode

import (
	"errors"
	"mime"
	"path"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/dop251/goja"

	"github.com/anasaki/studio/backend/internal/plugin"
)

// Phase 6: site.asset(path) reads a file of the calling JS plugin's own
// package (fonts, images, templates…): any file except code/, migrations/
// and manifest.json, at most 5 MB, cached per plugin version. Site Code has
// no package: site.asset throws there.

// assetHost is the plugin manager's package file reader (optional on
// PluginHost so test hosts need not implement it).
type assetHost interface {
	PackageFile(id, rel string) ([]byte, string, error)
}

// assetCache keeps read package files (immutable per version).
type assetCache struct {
	mu    sync.Mutex
	size  int
	files map[string][]byte // "<id>@<version>/<path>"
}

const maxAssetCache = 64 << 20

func (c *assetCache) get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.files[k]
	return b, ok
}

func (c *assetCache) put(k string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.files == nil || c.size+len(b) > maxAssetCache {
		c.files, c.size = map[string][]byte{}, 0
	}
	c.files[k] = b
	c.size += len(b)
}

var assets assetCache

// assetType is the media type of a package file.
func assetType(p string) string {
	ext := strings.ToLower(path.Ext(p))
	switch ext {
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".woff2":
		return "font/woff2"
	case ".md":
		return "text/markdown; charset=utf-8"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

func (v *vm) siteAsset(c goja.FunctionCall) goja.Value {
	const what = "site.asset"
	if !v.u.plugin() {
		v.fail("%s ใช้ได้เฉพาะในปลั๊กอิน JavaScript (อ่านไฟล์ในแพ็กเกจของตัวเอง) — Site Code ใช้ไฟล์ในคลังสื่อแทน", what)
	}
	p, _ := c.Argument(0).Export().(string)
	p = strings.TrimPrefix(strings.TrimSpace(p), "./")
	if !plugin.AssetPathOK(p) {
		v.fail("%s(%q): path ต้องเป็นไฟล์ในแพ็กเกจ เช่น 'fonts/Sarabun-Regular.ttf' (ไม่มี .. และไม่ใช่ code/, migrations/ หรือ manifest.json)", what, p)
	}
	var host assetHost
	if v.h != nil && v.h.e != nil && v.h.e.cfg.Plugins != nil {
		host, _ = v.h.e.cfg.Plugins.(assetHost)
	}
	if host == nil {
		v.fail("%s: ไม่พร้อมใช้งาน", what)
	}
	key := v.u.ID + "@" + v.u.Version + "/" + p
	b, ok := assets.get(key)
	version := v.u.Version
	if !ok {
		var err error
		b, version, err = host.PackageFile(v.u.ID, p)
		switch {
		case errors.Is(err, plugin.ErrAssetTooLarge):
			v.fail("%s(%q): ไฟล์ใหญ่เกิน 5 MB", what, p)
		case err != nil:
			v.fail("%s(%q): ไม่พบไฟล์นี้ในแพ็กเกจของปลั๊กอิน %s", what, p, v.u.ID)
		}
		if version == v.u.Version {
			assets.put(key, b)
		}
	}
	o := v.rt.NewObject()
	_ = o.Set("path", p)
	_ = o.Set("size", len(b))
	_ = o.Set("type", assetType(p))
	_ = o.Set("url", plugin.AssetURL(v.u.ID, version, p))
	_ = o.Set("bytes", func(goja.FunctionCall) goja.Value {
		return v.uint8Array(b)
	})
	_ = o.Set("text", func(goja.FunctionCall) goja.Value {
		if !utf8.Valid(b) {
			return v.rt.ToValue(strings.ToValidUTF8(string(b), "�"))
		}
		return v.rt.ToValue(string(b))
	})
	return o
}

// uint8Array returns a new Uint8Array holding a copy of b.
func (v *vm) uint8Array(b []byte) goja.Value {
	buf := v.rt.NewArrayBuffer(append([]byte(nil), b...))
	ctor, ok := goja.AssertConstructor(v.rt.Get("Uint8Array"))
	if !ok {
		return v.rt.ToValue(buf)
	}
	arr, err := ctor(nil, v.rt.ToValue(buf))
	if err != nil {
		return v.rt.ToValue(buf)
	}
	return arr
}
