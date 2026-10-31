package site

import (
	"html"
	"net/http"
	"os"
	"strings"
)

// adminPath is where the admin lives: ADMIN_PATH env (recovery override),
// then the admin_path setting, then "admin" (mirrors api.AdminPath).
func adminPath(opts map[string]string) string {
	if v := strings.Trim(os.Getenv("ADMIN_PATH"), "/"); v != "" {
		return v
	}
	if v := strings.Trim(opts["admin_path"], "/"); v != "" {
		return v
	}
	return "admin"
}

// comingSoon is shown at "/" while a site has no home page yet. It names the
// site but never links to the admin, so a custom admin path stays private.
func (s *Site) comingSoon(w http.ResponseWriter, opts map[string]string) {
	title := opts["site_title"]
	if title == "" {
		title = "Coming soon"
	}
	tagline := opts["tagline"]
	lang := opts["language"]
	msg := "เว็บไซต์กำลังเตรียมพร้อม เร็ว ๆ นี้"
	if lang == "en" {
		msg = "This website is getting ready. Please check back soon."
	}
	e := html.EscapeString
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="` + e(orDefault(lang, "th")) + `"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">`)
	b.WriteString(`<title>` + e(title) + `</title><link rel="icon" href="` + e(orDefault(s.siteIcon(opts), "/favicon.svg")) + `">`)
	b.WriteString(`<style>html,body{height:100%;margin:0}body{display:grid;place-items:center;font-family:Inter,'Noto Sans Thai',system-ui,sans-serif;` +
		`background:linear-gradient(135deg,#eef2ff,#fdf4ff);color:#171a26;text-align:center;padding:24px;box-sizing:border-box}` +
		`h1{font-size:clamp(28px,6vw,44px);margin:0 0 8px;letter-spacing:-.02em}p{margin:6px 0;color:#6b7184;font-size:17px}` +
		`.mark{width:56px;height:56px;margin:0 auto 20px;border-radius:16px;background:linear-gradient(135deg,#4f46e5,#a855f7)}</style></head><body><main>`)
	b.WriteString(`<div class="mark" aria-hidden="true"></div><h1>` + e(title) + `</h1>`)
	if tagline != "" {
		b.WriteString(`<p>` + e(tagline) + `</p>`)
	}
	b.WriteString(`<p>` + e(msg) + `</p></main></body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

// demoRibbon is the fixed badge shown on every public page of a demo site.
func demoRibbon(opts map[string]string) string {
	admin := "/" + html.EscapeString(adminPath(opts)) + "/"
	return `<a href="` + admin + `" style="position:fixed;z-index:2147483000;left:16px;bottom:16px;display:flex;align-items:center;gap:8px;` +
		`padding:10px 16px;border-radius:999px;background:#111827;color:#fff;font:600 14px/1.3 Inter,'Noto Sans Thai',system-ui,sans-serif;` +
		`text-decoration:none;box-shadow:0 8px 24px rgba(0,0,0,.25)">🧪 เว็บทดลอง — เข้าไปลองแก้ไขได้เลย <span style="opacity:.7">/ Try the admin →</span></a>`
}
