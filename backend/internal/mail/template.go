package mail

import (
	"html"
	"strings"
)

// Email is the content of a branded notification email.
type Email struct {
	Site       string // site title, shown in the header
	Heading    string
	Paragraphs []string // plain text, escaped here
	Rows       [][2]string
	Button     string // optional call to action
	ButtonURL  string
	Footer     string
}

// Render returns a simple, mobile-friendly HTML email (inline styles only,
// so it looks right in Gmail, Outlook and mobile mail apps).
func (e Email) Render() string {
	var b strings.Builder
	esc := html.EscapeString
	b.WriteString(`<!doctype html><html><body style="margin:0;background:#f5f6fa;font-family:-apple-system,Segoe UI,Roboto,'Noto Sans Thai',sans-serif;color:#171a26">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">`)
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:16px;border:1px solid #e6e8ef">`)
	b.WriteString(`<tr><td style="padding:24px 32px;border-bottom:1px solid #eef0f4;font-weight:700;font-size:15px;color:#4f46e5">` + esc(e.Site) + `</td></tr>`)
	b.WriteString(`<tr><td style="padding:28px 32px">`)
	b.WriteString(`<h1 style="margin:0 0 16px;font-size:20px;line-height:1.3">` + esc(e.Heading) + `</h1>`)
	for _, p := range e.Paragraphs {
		b.WriteString(`<p style="margin:0 0 14px;font-size:15px;line-height:1.6;color:#454a5c">` + strings.ReplaceAll(esc(p), "\n", "<br>") + `</p>`)
	}
	if len(e.Rows) > 0 {
		b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:8px 0 16px;border:1px solid #eef0f4;border-radius:10px">`)
		for i, r := range e.Rows {
			border := "border-top:1px solid #eef0f4;"
			if i == 0 {
				border = ""
			}
			b.WriteString(`<tr><td style="` + border + `padding:10px 14px;width:35%;font-size:13px;color:#6b7184;vertical-align:top">` + esc(r[0]) +
				`</td><td style="` + border + `padding:10px 14px;font-size:14px;white-space:pre-wrap;word-break:break-word">` + esc(r[1]) + `</td></tr>`)
		}
		b.WriteString(`</table>`)
	}
	if e.Button != "" && e.ButtonURL != "" {
		b.WriteString(`<p style="margin:24px 0 8px"><a href="` + esc(e.ButtonURL) + `" style="display:inline-block;background:#4f46e5;color:#ffffff;text-decoration:none;font-weight:600;padding:12px 22px;border-radius:10px">` + esc(e.Button) + `</a></p>`)
		b.WriteString(`<p style="margin:0;font-size:12px;color:#9aa0b0;word-break:break-all">` + esc(e.ButtonURL) + `</p>`)
	}
	b.WriteString(`</td></tr>`)
	footer := e.Footer
	if footer == "" {
		footer = "ส่งจาก " + e.Site + " · Anasaki Studio"
	}
	b.WriteString(`<tr><td style="padding:16px 32px;border-top:1px solid #eef0f4;font-size:12px;color:#9aa0b0">` + esc(footer) + `</td></tr>`)
	b.WriteString(`</table></td></tr></table></body></html>`)
	return b.String()
}
