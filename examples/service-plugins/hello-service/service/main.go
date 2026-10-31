// Hello Service: an example Anasaki Studio plugin with server code. It shows
// every part of the protocol: a public page (/hello), a server-rendered
// designer block, a collection, an admin screen, core events and storage.
//
// The same code builds two ways:
//
//	GOOS=wasip1 GOARCH=wasm go build -o ../package/plugin.wasm .   # runs inside the CMS (one-click install)
//	ANASAKI_PLUGIN_ID=hello-service ANASAKI_PLUGIN_SECRET=… ANASAKI_URL=http://localhost:8090 go run .   # separate service
package main

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anasaki/studio/sdk/go/anasaki"
)

// Events are kept with the CMS storage API (scope "storage"): a WASM plugin
// starts fresh for every request, so nothing survives in memory.
const eventsKey = "events"

var quotes = []map[string]any{
	{"title": "เริ่มจากสิ่งเล็ก ๆ", "text": "งานใหญ่ทุกชิ้นเริ่มจากก้าวแรก", "author": "Anasaki"},
	{"title": "Keep it simple", "text": "Simple things are easier to love.", "author": "Anasaki"},
	{"title": "ทำให้เสร็จ", "text": "เสร็จดีกว่าสมบูรณ์แบบแต่ไม่เสร็จ", "author": "Anasaki"},
}

func main() {
	p := anasaki.New(anasaki.Config{Name: "Hello Service", Version: "1.0.0"})

	// Public page: wrapped in the site's header and footer by the CMS.
	p.Public("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		site, err := p.CMS.Site(ctx)
		if err != nil {
			log.Printf("site: %v", err)
		}
		posts, _ := p.CMS.Posts(ctx, url.Values{"limit": {"3"}})
		var visits int
		_, _ = p.CMS.Get(ctx, "visits", &visits)
		visits++
		_ = p.CMS.Set(ctx, "visits", visits)
		var b strings.Builder
		fmt.Fprintf(&b, `<section style="max-width:760px;margin:64px auto;padding:0 24px">`)
		fmt.Fprintf(&b, `<h1 style="font-size:40px;font-weight:800;color:var(--c-primary)">สวัสดีจาก Hello Service 👋</h1>`)
		fmt.Fprintf(&b, `<p>หน้านี้สร้างโดยโปรแกรมของปลั๊กอิน แต่ใช้ส่วนหัว/ท้ายของเว็บ <b>%s</b> · เปิดดูแล้ว %d ครั้ง</p>`,
			html.EscapeString(site.Title), visits)
		if len(posts) > 0 {
			b.WriteString(`<h2 style="margin-top:32px;font-size:22px;font-weight:700">บทความล่าสุด (อ่านผ่าน API ของ CMS)</h2><ul>`)
			for _, po := range posts {
				fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, html.EscapeString(po["url"]), html.EscapeString(po["title"]))
			}
			b.WriteString(`</ul>`)
		}
		b.WriteString(`</section>`)
		anasaki.Page(w, http.StatusOK, "Hello", b.String())
	})

	// Designer block rendered by the service.
	p.Block("greeting", func(in anasaki.BlockInput) anasaki.BlockOutput {
		greet, _ := in.Settings["greeting"].(string)
		name, _ := in.Values["name"].(string)
		return anasaki.BlockOutput{
			HTML: fmt.Sprintf(`<div class="hello">%s, <b>%s</b>! 👋 <small>%s</small></div>`,
				html.EscapeString(greet), html.EscapeString(name), time.Now().Format("15:04")),
			CSS: `.hello{padding:16px 20px;border-radius:12px;background:var(--c-surface);border:1px solid #e5e7eb;font-size:18px}.hello small{opacity:.5}`,
		}
	})

	// Collection for the designer's collection block ({{item.title}} …).
	p.Collection("quotes", func(q map[string]any) []map[string]any {
		limit := len(quotes)
		if l, ok := q["limit"].(float64); ok && int(l) < limit && l > 0 {
			limit = int(l)
		}
		return quotes[:limit]
	})

	// Core events.
	p.OnEvent(func(e anasaki.Event) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var events []anasaki.Event
		_, _ = p.CMS.Get(ctx, eventsKey, &events)
		events = append([]anasaki.Event{e}, events...)
		if len(events) > 50 {
			events = events[:50]
		}
		if err := p.CMS.Set(ctx, eventsKey, events); err != nil {
			log.Printf("store event: %v", err)
		}
	})

	// Admin screen (shown inside the CMS admin in a sandboxed iframe).
	p.Admin("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		u := anasaki.UserFrom(r)
		var list []anasaki.Event
		_, _ = p.CMS.Get(r.Context(), eventsKey, &list)
		var b strings.Builder
		b.WriteString(`<!doctype html><meta charset="utf-8"><style>body{font:15px system-ui,'Noto Sans Thai',sans-serif;margin:24px;color:#111827}
table{border-collapse:collapse;width:100%}td,th{border-bottom:1px solid #e5e7eb;padding:8px;text-align:left}th{color:#6b7280;font-weight:600}
.btn{display:inline-block;padding:8px 14px;border-radius:8px;background:#4f46e5;color:#fff;border:0;cursor:pointer}</style>`)
		fmt.Fprintf(&b, `<h2>สวัสดี %s 👋</h2><p>ตำแหน่ง: %s · หน้านี้มาจากโปรแกรมของปลั๊กอิน</p>`,
			html.EscapeString(u.Name), html.EscapeString(u.Role))
		fmt.Fprintf(&b, `<form method="post" action="%s"><button class="btn">ส่งเหตุการณ์ทดสอบ (hello-service.ping)</button></form>`,
			html.EscapeString(anasaki.AdminURL(r, "ping")))
		b.WriteString(`<h3 style="margin-top:24px">เหตุการณ์ที่ได้รับ</h3><table><tr><th>เวลา</th><th>เหตุการณ์</th><th>ข้อมูล</th></tr>`)
		for _, e := range list {
			fmt.Fprintf(&b, `<tr><td>%s</td><td><code>%s</code></td><td>%s</td></tr>`, html.EscapeString(e.SentAt),
				html.EscapeString(e.Event), html.EscapeString(fmt.Sprint(e.Data)))
		}
		if len(list) == 0 {
			b.WriteString(`<tr><td colspan="3" style="color:#6b7280">ยังไม่มี — ลองเผยแพร่บทความหรืออัปโหลดรูป</td></tr>`)
		}
		b.WriteString(`</table>`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(b.String()))
	})
	p.Admin("POST /ping", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := p.CMS.Emit(ctx, "ping", map[string]any{"by": anasaki.UserFrom(r).Name}); err != nil {
			log.Printf("emit: %v", err)
		}
		http.Redirect(w, r, anasaki.AdminURL(r, "dashboard"), http.StatusSeeOther)
	})

	if err := anasaki.Run(p, ":9300"); err != nil {
		log.Fatal(err)
	}
}
