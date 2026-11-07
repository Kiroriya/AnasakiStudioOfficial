// Command kits writes the starter kits the Plugin Store serves to the setup
// wizard (store 1.6.0, CMS 2.5.0) as JSON files:
//
//	go run ./cmd/kits -out ../../store/backend/starters
//
// A kit is data only: page sections (design nodes; the CMS adds the site
// header and footer), a menu, categories, content types, sample posts and
// the official plugins it works with. This program is a tool for the store's
// data — the CMS server never names a plugin; it applies whatever kit the
// store sends.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type M = map[string]any

type kit struct {
	ID           string            `json:"id"`
	Name         map[string]string `json:"name"`
	Description  map[string]string `json:"description"`
	Icon         string            `json:"icon"`
	Order        int               `json:"order"`
	Plugins      []string          `json:"plugins"`
	Menu         []M               `json:"menu"`
	Categories   []M               `json:"categories,omitempty"`
	ContentTypes []M               `json:"contentTypes,omitempty"`
	Pages        []M               `json:"pages"`
	Posts        []M               `json:"posts,omitempty"`
}

// ---------------------------------------------------------------- design DSL

type b struct{ seq int }

func (x *b) id(p string) string { x.seq++; return fmt.Sprintf("%s%d", p, x.seq) }
func fill() M                   { return M{"mode": "fill"} }
func hug() M                    { return M{"mode": "hug"} }
func fixed(v int) M             { return M{"mode": "fixed", "value": v} }
func pad(t, r, bt, l int) []int { return []int{t, r, bt, l} }

func (x *b) frame(name string, w, h M, layout, style M, kids ...M) M {
	return M{"id": x.id("f"), "type": "frame", "name": name, "width": w, "height": h, "layout": layout, "style": style, "children": kids}
}
func (x *b) text(name, t, tag string, style M, extra ...M) M {
	n := M{"id": x.id("t"), "type": "text", "name": name, "width": fill(), "height": hug(), "style": style, "props": M{"text": t, "tag": tag}}
	for _, e := range extra {
		for k, v := range e {
			n[k] = v
		}
	}
	return n
}
func (x *b) button(name, label, href string, primary bool, color string) M {
	style := M{"fill": "#ffffff", "color": color, "radius": 999, "fontWeight": 700, "fontSize": 16}
	if !primary {
		style = M{"fill": "rgba(255,255,255,.12)", "color": "#ffffff", "radius": 999, "fontWeight": 600, "fontSize": 16, "stroke": "rgba(255,255,255,.5)", "strokeWidth": 1.5}
	}
	return M{"id": x.id("btn"), "type": "button", "name": name, "width": hug(), "height": hug(), "layout": M{"padding": pad(14, 28, 14, 28)},
		"style": style, "props": M{"text": label, "href": href}}
}

type link struct{ label, href string }

func (x *b) hero(kicker, title, sub string, b1, b2 link, gradient, btnColor string) M {
	btns := []M{x.button("ปุ่มหลัก", b1.label, b1.href, true, btnColor)}
	if b2.label != "" {
		btns = append(btns, x.button("ปุ่มรอง", b2.label, b2.href, false, btnColor))
	}
	h := x.frame("ส่วนเปิดหน้า", fill(), hug(), M{"mode": "vertical", "align": "center", "gap": 24, "padding": pad(120, 24, 128, 24)},
		M{"fill": gradient, "color": "#ffffff", "textAlign": "center"},
		x.text("ข้อความนำ", kicker, "span", M{"fontSize": 14, "fontWeight": 600, "letterSpacing": 1, "opacity": 0.85}, M{"width": hug()}),
		x.text("หัวข้อใหญ่", title, "h1", M{"fontSize": 56, "fontWeight": 800, "lineHeight": 1.15, "maxWidth": 900, "letterSpacing": -0.5},
			M{"responsive": M{"tablet": M{"style": M{"fontSize": 44}}, "mobile": M{"style": M{"fontSize": 32}}}}),
		x.text("คำอธิบาย", sub, "p", M{"fontSize": 20, "lineHeight": 1.6, "opacity": 0.92, "maxWidth": 680},
			M{"responsive": M{"mobile": M{"style": M{"fontSize": 17}}}}),
		x.frame("กลุ่มปุ่ม", hug(), hug(), M{"mode": "horizontal", "gap": 12, "wrap": true, "justify": "center"}, M{}, btns...),
	)
	h["responsive"] = M{"mobile": M{"layout": M{"padding": pad(72, 20, 80, 20)}}}
	return h
}

type card struct{ icon, title, text string }

func (x *b) features(heading string, accent string, cards ...card) M {
	var kids []M
	for _, c := range cards {
		kids = append(kids, x.frame("การ์ดจุดเด่น", fill(), hug(), M{"mode": "vertical", "gap": 12, "padding": pad(32, 28, 32, 28)},
			M{"fill": "#f9fafb", "radius": 20, "stroke": "#e5e7eb", "strokeWidth": 1},
			x.frame("ไอคอน", fixed(52), fixed(52), M{"mode": "vertical", "align": "center", "justify": "center"},
				M{"fill": accent, "radius": 14, "fontSize": 26}, x.text("อีโมจิ", c.icon, "span", M{}, M{"width": hug()})),
			x.text("หัวข้อ", c.title, "h3", M{"fontSize": 20, "fontWeight": 700, "color": "#111827"}),
			x.text("คำอธิบาย", c.text, "p", M{"fontSize": 15, "lineHeight": 1.65, "color": "#6b7280"})))
	}
	row := x.frame("แถวจุดเด่น", fill(), hug(), M{"mode": "horizontal", "gap": 24}, M{"maxWidth": 1120}, kids...)
	row["responsive"] = M{"tablet": M{"layout": M{"mode": "vertical"}}}
	return x.frame("ส่วนจุดเด่น", fill(), hug(), M{"mode": "vertical", "align": "center", "gap": 48, "padding": pad(96, 24, 96, 24)}, M{},
		x.text("หัวข้อส่วน", heading, "h2", M{"fontSize": 38, "fontWeight": 800, "textAlign": "center", "color": "#111827"},
			M{"responsive": M{"mobile": M{"style": M{"fontSize": 28}}}}), row)
}

// list is a section with a post list of postType: cards with the photo, the title and the first fields.
func (x *b) list(heading, postType string, limit int, where []M, orderBy string, desc bool, more link) M {
	q := M{"postType": postType, "limit": limit, "order": "newest"}
	if len(where) > 0 {
		q["where"] = where
	}
	if orderBy != "" {
		q["orderBy"], q["orderDesc"] = orderBy, desc
	}
	cardNode := x.frame("การ์ด", fill(), hug(), M{"mode": "vertical"},
		M{"fill": "#ffffff", "radius": 16, "clip": true, "shadow": "0 1px 3px rgba(0,0,0,.06), 0 8px 24px rgba(17,24,39,.06)"},
		M{"id": x.id("img"), "type": "image", "name": "รูป", "width": fill(), "height": fixed(200),
			"style": M{"fill": "#e5e7eb", "objectFit": "cover"}, "props": M{"src": "{{post.image}}", "alt": "{{post.title}}", "href": "{{post.url}}"}},
		x.frame("เนื้อหาการ์ด", fill(), hug(), M{"mode": "vertical", "gap": 8, "padding": pad(20, 22, 24, 22)}, M{},
			x.text("ชื่อ", "{{post.title}}", "h3", M{"fontSize": 19, "fontWeight": 700, "lineHeight": 1.35, "color": "#111827"},
				M{"props": M{"text": "{{post.title}}", "tag": "h3", "href": "{{post.url}}"}}),
			M{"id": x.id("pf"), "type": "postFields", "name": "ข้อมูลเพิ่มเติม", "width": fill(), "height": hug(),
				"style": M{"fontSize": 14}, "props": M{"max": 3, "layout": "inline"}},
		))
	lst := M{"id": x.id("pl"), "type": "postList", "name": heading, "width": fill(), "height": hug(),
		"layout": M{"mode": "horizontal", "gap": 24, "wrap": true}, "style": M{"maxWidth": 1120},
		"props":      M{"query": q, "emptyText": "ยังไม่มีรายการ — เพิ่มได้จากเมนูซ้ายของหน้าจัดการ"},
		"responsive": M{"tablet": M{"layout": M{"mode": "vertical"}}},
		"children":   []M{cardNode}}
	cardNode["width"] = fixed(352)
	cardNode["responsive"] = M{"tablet": M{"width": fill()}}
	kids := []M{x.text("หัวข้อส่วน", heading, "h2", M{"fontSize": 38, "fontWeight": 800, "textAlign": "center", "color": "#111827"},
		M{"responsive": M{"mobile": M{"style": M{"fontSize": 28}}}}), lst}
	if more.label != "" {
		kids = append(kids, M{"id": x.id("btn"), "type": "button", "name": "ปุ่มดูทั้งหมด", "width": hug(), "height": hug(),
			"layout": M{"padding": pad(12, 24, 12, 24)}, "style": M{"fill": "#111827", "color": "#ffffff", "radius": 999, "fontWeight": 600},
			"props": M{"text": more.label, "href": more.href}})
	}
	return x.frame("ส่วนรายการ", fill(), hug(), M{"mode": "vertical", "align": "center", "gap": 40, "padding": pad(96, 24, 96, 24)},
		M{"fill": "#f3f4f6"}, kids...)
}

func (x *b) cta(title, sub string, btn link, fillColor string) M {
	return x.frame("ส่วนชวน", fill(), hug(), M{"mode": "vertical", "align": "center", "gap": 18, "padding": pad(88, 24, 88, 24)},
		M{"fill": fillColor, "color": "#ffffff", "textAlign": "center"},
		x.text("หัวข้อชวน", title, "h2", M{"fontSize": 38, "fontWeight": 800, "maxWidth": 760}, M{"responsive": M{"mobile": M{"style": M{"fontSize": 28}}}}),
		x.text("คำชวน", sub, "p", M{"fontSize": 18, "opacity": 0.85, "maxWidth": 640}),
		M{"id": x.id("btn"), "type": "button", "name": "ปุ่มชวน", "width": hug(), "height": hug(), "layout": M{"padding": pad(14, 30, 14, 30)},
			"style": M{"fill": "#ffffff", "color": "#111827", "radius": 999, "fontWeight": 700}, "props": M{"text": btn.label, "href": btn.href}})
}

func (x *b) textPage(title, sub string, paras ...string) []M {
	head := x.frame("ส่วนหัวข้อ", fill(), hug(), M{"mode": "vertical", "align": "center", "gap": 12, "padding": pad(80, 24, 40, 24)}, M{"textAlign": "center"},
		x.text("หัวข้อ", title, "h1", M{"fontSize": 46, "fontWeight": 800, "color": "#111827"}, M{"responsive": M{"mobile": M{"style": M{"fontSize": 32}}}}),
		x.text("คำอธิบาย", sub, "p", M{"fontSize": 18, "color": "#6b7280", "maxWidth": 620}))
	var ps []M
	for _, p := range paras {
		ps = append(ps, x.text("ย่อหน้า", p, "p", M{"fontSize": 18, "lineHeight": 1.8, "color": "#374151"}))
	}
	body := x.frame("เนื้อหา", fill(), hug(), M{"mode": "vertical", "gap": 18, "padding": pad(0, 24, 96, 24)}, M{"maxWidth": 760}, ps...)
	return []M{head, body}
}

// listPage is a page with a heading and a full list of postType (with optional conditions / sort).
func (x *b) listPage(title, sub, postType string, where []M, orderBy string, desc bool) []M {
	head := x.frame("ส่วนหัวข้อ", fill(), hug(), M{"mode": "vertical", "align": "center", "gap": 12, "padding": pad(80, 24, 24, 24)}, M{"textAlign": "center"},
		x.text("หัวข้อ", title, "h1", M{"fontSize": 46, "fontWeight": 800, "color": "#111827"}, M{"responsive": M{"mobile": M{"style": M{"fontSize": 32}}}}),
		x.text("คำอธิบาย", sub, "p", M{"fontSize": 18, "color": "#6b7280", "maxWidth": 620}))
	sec := x.list("", postType, 24, where, orderBy, desc, link{})
	sec["style"] = M{}
	sec["children"] = sec["children"].([]M)[1:] // no second heading
	return []M{head, sec}
}

func typeSpec(id, th, en, slug, icon string, supports []string, fields ...M) M {
	return M{"type": id, "label": M{"th": th, "en": en}, "icon": icon, "slug": slug, "public": true, "supports": supports, "fields": fields}
}
func field(key, typ, th string, extra ...M) M {
	f := M{"key": key, "type": typ, "label": M{"th": th}}
	for _, e := range extra {
		for k, v := range e {
			f[k] = v
		}
	}
	return f
}
func options(vals ...string) M {
	var o []M
	for _, v := range vals {
		o = append(o, M{"value": v, "label": M{"th": v}})
	}
	return M{"options": o}
}
func post(typ, title, slug, excerpt, content string, meta M, c1, c2 string) M {
	return M{"type": typ, "title": title, "slug": slug, "excerpt": excerpt, "content": content, "meta": meta, "image": []string{c1, c2}}
}

func inCat(cat, typ, title, slug, excerpt, content string, meta M, c1, c2 string) M {
	p := post(typ, title, slug, excerpt, content, meta, c1, c2)
	p["category"] = cat
	return p
}

var upcoming = []M{{"field": "when", "op": ">=", "value": "today"}}

// ---------------------------------------------------------------- the kits

func kits() []kit {
	var out []kit
	about := func(x *b, who string) M {
		return M{"title": "เกี่ยวกับเรา", "slug": "about", "sections": x.textPage("เกี่ยวกับเรา", "ทำความรู้จัก"+who+"มากขึ้น",
			"เล่าเรื่องราวของคุณที่นี่ — เริ่มต้นอย่างไร ทำอะไร และอะไรที่ทำให้ลูกค้าเลือกคุณ (ข้อความตัวอย่าง แก้ได้ทั้งหมดจากหน้าออกแบบ)",
			"ใส่ที่อยู่ เบอร์โทร อีเมล หรือแผนที่ (องค์ประกอบ → โค้ดฝัง) ลงในหน้านี้ได้")}
	}

	{ // café / restaurant
		x := &b{}
		out = append(out, kit{ID: "cafe", Order: 10, Icon: "☕", Plugins: []string{"booking", "chat-buttons", "maps"},
			Name:        map[string]string{"th": "ร้านกาแฟ / ร้านอาหาร", "en": "Café / restaurant"},
			Description: map[string]string{"th": "เมนูพร้อมราคา เวลาเปิด จองโต๊ะออนไลน์", "en": "Menu with prices, opening hours, table booking"},
			Menu:        []M{{"label": "หน้าแรก", "url": "/"}, {"label": "เมนู", "url": "/menu"}, {"label": "จองโต๊ะ", "url": "/booking"}, {"label": "เกี่ยวกับเรา", "url": "/about"}},
			ContentTypes: []M{typeSpec("menu_item", "เมนู", "Menu", "menu", "☕", []string{"title", "content", "excerpt", "image"},
				field("price", "number", "ราคา (บาท)", M{"min": 0}), field("kind", "select", "ประเภท", options("กาแฟ", "ชา", "เบเกอรี่", "อาหาร")),
				field("featured", "bool", "เมนูแนะนำ"))},
			Pages: []M{
				{"title": "หน้าแรก", "slug": "home", "home": true, "sections": []M{
					x.hero("ร้านเล็ก ๆ ใกล้บ้านคุณ", "กาแฟคั่วเอง ขนมอบสดทุกเช้า", "เปิดทุกวัน 08:00–18:00 · นั่งทำงานได้ มีปลั๊กและ Wi-Fi",
						link{"ดูเมนู", "/menu"}, link{"จองโต๊ะ", "/booking"}, "linear-gradient(135deg,#6b3e26 0%,#a0714f 60%,#d4a373 100%)", "#6b3e26"),
					x.list("เมนูแนะนำ", "menu_item", 6, []M{{"field": "featured", "op": "=", "value": "true"}}, "", false, link{"ดูเมนูทั้งหมด", "/menu"}),
					x.features("ทำไมต้องร้านเรา", "linear-gradient(135deg,#fdf0d5,#f4d6b0)",
						card{"🌱", "เมล็ดคัดพิเศษ", "คั่วเองทุกสัปดาห์ จากไร่กาแฟไทย"}, card{"🥐", "อบสดทุกเช้า", "ครัวซองต์และขนมปังออกจากเตาทุกวัน"},
						card{"📅", "จองโต๊ะออนไลน์", "เลือกวันและเวลาได้ล่วงหน้า ไม่ต้องรอคิว"}),
					x.cta("แวะมาหาเราได้ทุกวัน", "แผนที่และการเดินทางอยู่ที่หน้าเกี่ยวกับเรา", link{"จองโต๊ะ", "/booking"}, "#3b2417"),
				}},
				{"title": "เมนู (หน้ารวม)", "slug": "menu-archive", "archiveFor": "menu_item", "sections": x.listPage("เมนู", "เครื่องดื่มและของว่างทั้งหมด", "menu_item", nil, "price", false)},
				about(x, "ร้านของเรา"),
			},
			Posts: []M{
				post("menu_item", "เอสเปรสโซ่", "espresso", "ช็อตเข้มข้นจากเมล็ดคั่วกลาง", "", M{"price": 55, "kind": "กาแฟ", "featured": true}, "#6b3e26", "#2b1a10"),
				post("menu_item", "ลาเต้เย็น", "iced-latte", "นมสดหอมมัน กาแฟกลมกล่อม", "", M{"price": 75, "kind": "กาแฟ", "featured": true}, "#c8a27a", "#5c4033"),
				post("menu_item", "ชาไทยเย็น", "thai-tea", "ชาไทยสูตรต้นตำรับ", "", M{"price": 60, "kind": "ชา", "featured": true}, "#e07b39", "#7a3412"),
				post("menu_item", "ครัวซองต์เนยสด", "croissant", "อบใหม่ทุกเช้า", "", M{"price": 65, "kind": "เบเกอรี่"}, "#d4a373", "#7f5539"),
			}})
	}
	{ // online shop
		x := &b{}
		out = append(out, kit{ID: "shop", Order: 20, Icon: "🛍️", Plugins: []string{"eshop", "chat-buttons"},
			Name:        map[string]string{"th": "ร้านค้าออนไลน์", "en": "Online shop"},
			Description: map[string]string{"th": "สินค้า ตะกร้า ชำระด้วยพร้อมเพย์ / โอน / ปลายทาง", "en": "Products, cart, PromptPay / transfer / COD"},
			Menu:        []M{{"label": "หน้าแรก", "url": "/"}, {"label": "สินค้า", "url": "/product"}, {"label": "ตะกร้า", "url": "/cart"}, {"label": "เกี่ยวกับเรา", "url": "/about"}},
			Pages: []M{
				{"title": "หน้าแรก", "slug": "home", "home": true, "sections": []M{
					x.hero("ส่งไวทั่วประเทศ", "สินค้าคุณภาพ ราคาเป็นมิตร", "สั่งง่าย จ่ายด้วยพร้อมเพย์ โอนเงิน หรือเก็บเงินปลายทาง",
						link{"เลือกซื้อสินค้า", "/product"}, link{"ตะกร้าของฉัน", "/cart"}, "linear-gradient(135deg,#0f766e 0%,#14b8a6 60%,#5eead4 100%)", "#0f766e"),
					x.list("สินค้าแนะนำ", "eshop_product", 6, nil, "", false, link{"ดูสินค้าทั้งหมด", "/product"}),
					x.features("ช้อปกับเราอุ่นใจ", "linear-gradient(135deg,#ccfbf1,#99f6e4)",
						card{"🚚", "ส่งไว", "แพ็กและส่งภายใน 1–2 วันทำการ"}, card{"💳", "จ่ายง่าย", "QR พร้อมเพย์ใส่ยอดให้แล้ว แนบสลิปได้ทันที"},
						card{"💬", "ถามได้ตลอด", "ติดต่อเราได้ทุกช่องทาง ตอบไว"}),
					x.cta("มีคำถามเรื่องสินค้า?", "ทักมาได้เลย เรายินดีช่วยเลือก", link{"ติดต่อเรา", "/about"}, "#134e4a"),
				}},
				about(x, "ร้านของเรา"),
			}})
	}
	{ // clinic / salon / services with booking
		x := &b{}
		out = append(out, kit{ID: "service", Order: 30, Icon: "🩺", Plugins: []string{"booking", "chat-buttons", "maps"},
			Name:        map[string]string{"th": "คลินิก / ร้านเสริมสวย / บริการ", "en": "Clinic / salon / services"},
			Description: map[string]string{"th": "บริการพร้อมราคา จองคิวออนไลน์ คำถามที่พบบ่อย", "en": "Services with prices, online booking, FAQ"},
			Menu:        []M{{"label": "หน้าแรก", "url": "/"}, {"label": "บริการ", "url": "/services"}, {"label": "จองคิว", "url": "/booking"}, {"label": "บทความ", "url": "/blog"}, {"label": "เกี่ยวกับเรา", "url": "/about"}},
			ContentTypes: []M{typeSpec("service", "บริการ", "Services", "services", "✨", []string{"title", "content", "excerpt", "image"},
				field("price", "number", "ราคาเริ่มต้น (บาท)", M{"min": 0}), field("minutes", "number", "ระยะเวลา (นาที)", M{"min": 0}))},
			Pages: []M{
				{"title": "หน้าแรก", "slug": "home", "home": true, "sections": []M{
					x.hero("ดูแลโดยผู้เชี่ยวชาญ", "ให้เราดูแลคุณอย่างใส่ใจ", "เลือกบริการและเวลาที่สะดวก จองคิวออนไลน์ได้ตลอด 24 ชั่วโมง",
						link{"จองคิว", "/booking"}, link{"ดูบริการ", "/services"}, "linear-gradient(135deg,#1d4ed8 0%,#3b82f6 60%,#93c5fd 100%)", "#1d4ed8"),
					x.list("บริการของเรา", "service", 6, nil, "price", false, link{"ดูบริการทั้งหมด", "/services"}),
					x.features("มั่นใจได้ทุกครั้ง", "linear-gradient(135deg,#dbeafe,#bfdbfe)",
						card{"👩‍⚕️", "ผู้เชี่ยวชาญดูแล", "ประสบการณ์มากกว่า 10 ปี"}, card{"🧼", "สะอาด ปลอดภัย", "ฆ่าเชื้ออุปกรณ์ทุกครั้งตามมาตรฐาน"},
						card{"⏱️", "ไม่ต้องรอคิว", "จองเวลาล่วงหน้า มาถึงแล้วรับบริการได้เลย"}),
					x.cta("พร้อมนัดหมายแล้วหรือยัง?", "เลือกบริการ วัน และเวลาที่สะดวกได้เลย", link{"จองคิว", "/booking"}, "#1e3a8a"),
				}},
				{"title": "บริการ (หน้ารวม)", "slug": "service-archive", "archiveFor": "service", "sections": x.listPage("บริการ", "บริการทั้งหมดพร้อมราคาเริ่มต้น", "service", nil, "price", false)},
				about(x, "เรา"),
			},
			Posts: []M{
				post("service", "ตรวจและปรึกษา", "consult", "พูดคุยปัญหาและวางแผนการดูแล", "", M{"price": 0, "minutes": 30}, "#1d4ed8", "#1e3a8a"),
				post("service", "บริการมาตรฐาน", "standard", "บริการยอดนิยมของเรา", "", M{"price": 800, "minutes": 45}, "#3b82f6", "#1d4ed8"),
				post("service", "บริการพิเศษ", "premium", "ดูแลครบทุกขั้นตอน", "", M{"price": 1500, "minutes": 90}, "#60a5fa", "#2563eb"),
			}})
	}
	{ // blog / news / magazine
		x := &b{}
		out = append(out, kit{ID: "blog", Order: 40, Icon: "📰", Plugins: []string{"blog-extras", "newsletter"},
			Name:        map[string]string{"th": "บล็อก / ข่าว / นิตยสาร", "en": "Blog / news / magazine"},
			Description: map[string]string{"th": "บทความตามหมวด ความคิดเห็น ปุ่มแชร์ จดหมายข่าว", "en": "Articles by category, comments, sharing, newsletter"},
			Menu:        []M{{"label": "หน้าแรก", "url": "/"}, {"label": "บทความ", "url": "/blog"}, {"label": "ค้นหา", "url": "/search"}, {"label": "เกี่ยวกับเรา", "url": "/about"}},
			Categories:  []M{{"name": "ข่าว", "slug": "news"}, {"name": "ไลฟ์สไตล์", "slug": "lifestyle"}, {"name": "เทคโนโลยี", "slug": "tech"}},
			Pages: []M{
				{"title": "หน้าแรก", "slug": "home", "home": true, "sections": []M{
					x.hero("อัปเดตทุกวัน", "เรื่องเล่า ข่าวสาร และแรงบันดาลใจ", "อ่านบทความล่าสุด หรือสมัครรับข่าวสารทางอีเมล",
						link{"อ่านบทความ", "/blog"}, link{"ค้นหา", "/search"}, "linear-gradient(135deg,#111827 0%,#374151 60%,#6b7280 100%)", "#111827"),
					x.list("บทความล่าสุด", "post", 6, nil, "", false, link{"ดูบทความทั้งหมด", "/blog"}),
					x.cta("ไม่พลาดเรื่องใหม่", "สมัครรับข่าวสารทางอีเมล (วางบล็อก “ฟอร์มรับข่าวสาร” ในหน้านี้ได้)", link{"สมัครรับข่าวสาร", "/about"}, "#111827"),
				}},
				about(x, "เรา"),
			},
			Posts: []M{
				inCat("news", "post", "เริ่มต้นเขียนบทความแรกของคุณ", "first-story", "เคล็ดลับเขียนให้น่าอ่านใน 5 ข้อ", "<p>บทความตัวอย่าง แก้ไขหรือลบได้จากเมนู บทความ / ข่าว</p>", nil, "#111827", "#4b5563"),
				inCat("lifestyle", "post", "ไลฟ์สไตล์ช้า ๆ ในเมืองใหญ่", "slow-living", "ใช้ชีวิตให้ช้าลงสักนิด", "<p>บทความตัวอย่าง</p>", nil, "#be185d", "#f472b6"),
				inCat("tech", "post", "เครื่องมือดิจิทัลที่ช่วยให้ทำงานง่ายขึ้น", "digital-tools", "แอปที่ทีมเราใช้ทุกวัน", "<p>บทความตัวอย่าง</p>", nil, "#0e7490", "#22d3ee"),
			}})
	}
	{ // company / organisation
		x := &b{}
		out = append(out, kit{ID: "company", Order: 50, Icon: "🏢", Plugins: []string{"forms", "maps", "chat-buttons"},
			Name:        map[string]string{"th": "บริษัท / ธุรกิจ", "en": "Company / business"},
			Description: map[string]string{"th": "บริการ ผลงาน ข่าวสาร รับสมัครงาน และฟอร์มติดต่อ", "en": "Services, news, careers and a contact form"},
			Menu:        []M{{"label": "หน้าแรก", "url": "/"}, {"label": "ข่าวสาร", "url": "/blog"}, {"label": "ร่วมงานกับเรา", "url": "/jobs"}, {"label": "เกี่ยวกับเรา", "url": "/about"}},
			ContentTypes: []M{typeSpec("job", "ตำแหน่งงาน", "Jobs", "jobs", "💼", []string{"title", "content", "excerpt"},
				field("department", "text", "ฝ่าย"), field("location", "text", "สถานที่"), field("kind", "select", "รูปแบบงาน", options("งานประจำ", "พาร์ตไทม์", "ฝึกงาน", "ทำงานทางไกล")))},
			Pages: []M{
				{"title": "หน้าแรก", "slug": "home", "home": true, "sections": []M{
					x.hero("พันธมิตรทางธุรกิจของคุณ", "โซลูชันที่ช่วยธุรกิจของคุณเติบโต", "ทีมผู้เชี่ยวชาญพร้อมดูแลตั้งแต่วางแผนจนถึงส่งมอบ",
						link{"ติดต่อเรา", "/about"}, link{"ร่วมงานกับเรา", "/jobs"}, "linear-gradient(135deg,#312e81 0%,#4f46e5 60%,#818cf8 100%)", "#312e81"),
					x.features("บริการของเรา", "linear-gradient(135deg,#e0e7ff,#c7d2fe)",
						card{"📊", "วางกลยุทธ์", "วิเคราะห์ตลาดและวางแผนการเติบโต"}, card{"🛠️", "พัฒนาระบบ", "สร้างเครื่องมือที่เหมาะกับงานของคุณ"},
						card{"🤝", "ดูแลต่อเนื่อง", "ทีมซัพพอร์ตพร้อมช่วยเหลือตลอดสัญญา"}),
					x.list("ข่าวสารล่าสุด", "post", 3, nil, "", false, link{"ดูข่าวทั้งหมด", "/blog"}),
					x.cta("มาเติบโตไปด้วยกัน", "เล่าโจทย์ของคุณให้เราฟัง แล้วเราจะติดต่อกลับภายใน 1 วันทำการ", link{"ติดต่อเรา", "/about"}, "#1e1b4b"),
				}},
				{"title": "ตำแหน่งงาน (หน้ารวม)", "slug": "job-archive", "archiveFor": "job", "sections": x.listPage("ร่วมงานกับเรา", "ตำแหน่งที่เปิดรับตอนนี้", "job", nil, "", false)},
				about(x, "บริษัทของเรา"),
			},
			Posts: []M{
				post("job", "นักพัฒนาเว็บ (Full-stack)", "web-developer", "ร่วมสร้างผลิตภัณฑ์ให้ลูกค้าทั่วประเทศ", "<p>รายละเอียดงานตัวอย่าง</p>", M{"department": "เทคโนโลยี", "location": "กรุงเทพฯ", "kind": "งานประจำ"}, "#4f46e5", "#312e81"),
				post("job", "นักการตลาดดิจิทัล", "digital-marketer", "ดูแลแคมเปญและโซเชียลมีเดีย", "<p>รายละเอียดงานตัวอย่าง</p>", M{"department": "การตลาด", "location": "ทำงานทางไกลได้", "kind": "ทำงานทางไกล"}, "#7c3aed", "#4c1d95"),
			}})
	}
	{ // online course / membership
		x := &b{}
		out = append(out, kit{ID: "course", Order: 60, Icon: "🎓", Plugins: []string{"members"},
			Name:        map[string]string{"th": "คอร์สออนไลน์ / สมาชิก", "en": "Online course / membership"},
			Description: map[string]string{"th": "บทเรียน สมัครสมาชิก ส่วนเฉพาะสมาชิก", "en": "Lessons, sign-up, members-only content"},
			Menu:        []M{{"label": "หน้าแรก", "url": "/"}, {"label": "บทเรียน", "url": "/lessons"}, {"label": "สมัครสมาชิก", "url": "/members/register"}, {"label": "เข้าสู่ระบบ", "url": "/members/login"}},
			ContentTypes: []M{typeSpec("lesson", "บทเรียน", "Lessons", "lessons", "🎓", []string{"title", "content", "excerpt", "image"},
				field("no", "number", "บทที่", M{"min": 0}), field("minutes", "number", "ความยาว (นาที)", M{"min": 0}), field("video", "url", "วิดีโอ"))},
			Pages: []M{
				{"title": "หน้าแรก", "slug": "home", "home": true, "sections": []M{
					x.hero("เรียนได้ทุกที่ ทุกเวลา", "คอร์สออนไลน์ที่เรียนจบได้จริง", "สมัครสมาชิกฟรี แล้วเริ่มบทเรียนแรกได้ทันที",
						link{"สมัครสมาชิก", "/members/register"}, link{"ดูบทเรียน", "/lessons"}, "linear-gradient(135deg,#7c2d12 0%,#ea580c 60%,#fdba74 100%)", "#7c2d12"),
					x.list("บทเรียน", "lesson", 6, nil, "no", false, link{"ดูบทเรียนทั้งหมด", "/lessons"}),
					x.features("เรียนกับเราได้อะไร", "linear-gradient(135deg,#ffedd5,#fed7aa)",
						card{"🎬", "วิดีโอสั้น เข้าใจง่าย", "แต่ละบทไม่เกิน 15 นาที"}, card{"📝", "แบบฝึกหัดท้ายบท", "ทบทวนความเข้าใจทุกครั้ง"},
						card{"👥", "ชุมชนผู้เรียน", "ถามตอบกับเพื่อนและผู้สอน"}),
					x.cta("เริ่มเรียนวันนี้", "สมัครฟรี ใช้เวลาไม่ถึงนาที", link{"สมัครสมาชิก", "/members/register"}, "#431407"),
				}},
				{"title": "บทเรียน (หน้ารวม)", "slug": "lesson-archive", "archiveFor": "lesson", "sections": x.listPage("บทเรียน", "เรียงตามบท", "lesson", nil, "no", false)},
				about(x, "ผู้สอน"),
			},
			Posts: []M{
				post("lesson", "บทนำ: ภาพรวมของคอร์ส", "intro", "รู้จักเนื้อหาและวิธีเรียน", "<p>วางลิงก์ YouTube ลงในเนื้อหาเพื่อแทรกวิดีโอ</p>", M{"no": 1, "minutes": 8}, "#ea580c", "#7c2d12"),
				post("lesson", "พื้นฐานที่ต้องรู้", "basics", "เริ่มจากศูนย์อย่างเป็นขั้นตอน", "<p>เนื้อหาตัวอย่าง</p>", M{"no": 2, "minutes": 12}, "#f97316", "#9a3412"),
				post("lesson", "ลงมือทำจริง", "practice", "ฝึกด้วยตัวอย่างจากงานจริง", "<p>เนื้อหาตัวอย่าง</p>", M{"no": 3, "minutes": 15}, "#fb923c", "#c2410c"),
			}})
	}
	{ // portfolio
		x := &b{}
		out = append(out, kit{ID: "portfolio", Order: 70, Icon: "🎨", Plugins: []string{"forms"},
			Name:        map[string]string{"th": "พอร์ตโฟลิโอ / ช่างภาพ / ฟรีแลนซ์", "en": "Portfolio / photographer / freelancer"},
			Description: map[string]string{"th": "ผลงานพร้อมแกลเลอรี ลูกค้า ปี และฟอร์มจ้างงาน", "en": "Work with galleries, clients, years and a hire-me form"},
			Menu:        []M{{"label": "หน้าแรก", "url": "/"}, {"label": "ผลงาน", "url": "/work"}, {"label": "เกี่ยวกับฉัน", "url": "/about"}},
			ContentTypes: []M{typeSpec("project", "ผลงาน", "Projects", "work", "🖼️", []string{"title", "content", "excerpt", "image"},
				field("client", "text", "ลูกค้า"), field("year", "number", "ปี"), field("photos", "gallery", "ภาพผลงาน"))},
			Pages: []M{
				{"title": "หน้าแรก", "slug": "home", "home": true, "sections": []M{
					x.hero("ออกแบบ · ถ่ายภาพ · กำกับศิลป์", "งานที่เล่าเรื่องแบรนด์ของคุณ", "ผลงานคัดสรร พร้อมรับงานใหม่",
						link{"ดูผลงาน", "/work"}, link{"จ้างงาน", "/about"}, "linear-gradient(135deg,#18181b 0%,#3f3f46 60%,#a1a1aa 100%)", "#18181b"),
					x.list("ผลงานล่าสุด", "project", 6, nil, "year", true, link{"ดูผลงานทั้งหมด", "/work"}),
					x.cta("มีโปรเจกต์ในใจ?", "เล่ารายละเอียดมาได้เลย ตอบกลับภายใน 24 ชั่วโมง", link{"ติดต่อ", "/about"}, "#18181b"),
				}},
				{"title": "ผลงาน (หน้ารวม)", "slug": "project-archive", "archiveFor": "project", "sections": x.listPage("ผลงาน", "ผลงานทั้งหมด", "project", nil, "year", true)},
				about(x, "ฉัน"),
			},
			Posts: []M{
				post("project", "อัตลักษณ์แบรนด์ร้านกาแฟ", "cafe-branding", "โลโก้ แพ็กเกจ และป้ายร้าน", "<p>รายละเอียดผลงานตัวอย่าง</p>", M{"client": "บ้านกาแฟ", "year": 2026}, "#18181b", "#52525b"),
				post("project", "ภาพสินค้าคอลเลกชันฤดูร้อน", "summer-shoot", "ถ่ายภาพสินค้าและนางแบบ", "<p>รายละเอียดผลงานตัวอย่าง</p>", M{"client": "แบรนด์แฟชั่น", "year": 2025}, "#78716c", "#292524"),
			}})
	}
	{ // foundation / community / school
		x := &b{}
		out = append(out, kit{ID: "org", Order: 80, Icon: "🤝", Plugins: []string{"forms", "events", "donations"},
			Name:        map[string]string{"th": "มูลนิธิ / ชุมชน / โรงเรียน", "en": "Foundation / community / school"},
			Description: map[string]string{"th": "กิจกรรมที่กำลังจะมาถึง ข่าวสาร และฟอร์มสมัครอาสา", "en": "Upcoming events, news and a volunteer form"},
			Menu:        []M{{"label": "หน้าแรก", "url": "/"}, {"label": "กิจกรรม", "url": "/events"}, {"label": "ข่าวสาร", "url": "/blog"}, {"label": "เกี่ยวกับเรา", "url": "/about"}},
			ContentTypes: []M{typeSpec("event", "กิจกรรม", "Events", "events", "📅", []string{"title", "content", "excerpt", "image"},
				field("when", "datetime", "วันเวลา", M{"required": true}), field("place", "text", "สถานที่"), field("seats", "number", "รับสมัคร (คน)", M{"min": 0}))},
			Pages: []M{
				{"title": "หน้าแรก", "slug": "home", "home": true, "sections": []M{
					x.hero("ร่วมกันสร้างการเปลี่ยนแปลง", "ทุกคนช่วยได้ เริ่มจากใกล้ตัว", "ร่วมกิจกรรม เป็นอาสาสมัคร หรือสนับสนุนเรา",
						link{"กิจกรรมที่จะมาถึง", "/events"}, link{"เกี่ยวกับเรา", "/about"}, "linear-gradient(135deg,#065f46 0%,#10b981 60%,#6ee7b7 100%)", "#065f46"),
					x.list("กิจกรรมที่กำลังจะมาถึง", "event", 6, upcoming, "when", false, link{"ดูกิจกรรมทั้งหมด", "/events"}),
					x.list("ข่าวสาร", "post", 3, nil, "", false, link{"ดูข่าวทั้งหมด", "/blog"}),
					x.cta("อยากเป็นอาสาสมัคร?", "วางบล็อกฟอร์ม “สมัครอาสาสมัคร” ในหน้านี้ได้จากหน้าออกแบบ", link{"ติดต่อเรา", "/about"}, "#064e3b"),
				}},
				{"title": "กิจกรรม (หน้ารวม)", "slug": "event-archive", "archiveFor": "event", "sections": x.listPage("กิจกรรม", "กิจกรรมที่กำลังจะมาถึง เรียงตามวัน", "event", upcoming, "when", false)},
				about(x, "เรา"),
			},
			Posts: []M{
				post("event", "เก็บขยะริมคลอง", "canal-cleanup", "ร่วมกันดูแลคลองใกล้บ้าน", "<p>รายละเอียดกิจกรรมตัวอย่าง</p>", M{"when": "2027-01-16T08:00", "place": "ท่าเรือชุมชน", "seats": 40}, "#10b981", "#065f46"),
				post("event", "ปลูกต้นไม้ในโรงเรียน", "tree-planting", "ปลูกต้นไม้ 100 ต้นกับนักเรียน", "<p>รายละเอียดกิจกรรมตัวอย่าง</p>", M{"when": "2027-02-06T09:00", "place": "โรงเรียนในชุมชน", "seats": 60}, "#34d399", "#047857"),
			}})
	}
	return out
}

func main() {
	dir := flag.String("out", "../../store/backend/starters", "folder for the kit files")
	flag.Parse()
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, k := range kits() {
		b, err := json.MarshalIndent(k, "", " ")
		if err != nil {
			log.Fatal(err)
		}
		f := filepath.Join(*dir, k.ID+".json")
		if err := os.WriteFile(f, append(b, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println("wrote", f, len(b), "bytes")
	}
}
