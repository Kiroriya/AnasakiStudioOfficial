// Package seed installs a demo site on first run (the equivalent of the
// WordPress "famous five-minute install" + a starter theme).
package seed

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anasaki/studio/backend/internal/auth"
	"github.com/anasaki/studio/backend/internal/config"
	"github.com/anasaki/studio/backend/internal/model"
	"github.com/anasaki/studio/backend/internal/store"
)

type M = map[string]any

// RolesRemovedOption lists the built-in roles an administrator deleted
// (comma separated), so they are not re-created on the next start.
const RolesRemovedOption = "roles_removed"

// Roles inserts the built-in roles that are missing (never overwrites
// capabilities edited by an administrator, never brings back deleted ones).
func Roles(st *store.Store) error {
	removed := map[string]bool{}
	if opts, err := st.Options(); err == nil {
		for _, s := range strings.Split(opts[RolesRemovedOption], ",") {
			removed[strings.TrimSpace(s)] = true
		}
	}
	for _, b := range auth.BuiltinRoles() {
		if removed[b.Slug] && b.Slug != model.RoleAdmin && b.Slug != model.RoleSubscriber {
			continue
		}
		if err := st.EnsureRole(&model.Role{Slug: b.Slug, Name: b.Name, Caps: b.Caps, Builtin: true}); err != nil {
			return err
		}
	}
	return nil
}

// Run installs the site non-interactively on first run when both adminEmail
// and adminPassword are given (docker automation); otherwise the admin goes
// through the setup wizard (POST /api/v1/setup).
func Run(st *store.Store, adminEmail, adminPassword, uploadDir string) error {
	if adminEmail == "" || adminPassword == "" {
		return nil
	}
	n, err := st.CountUsers()
	if err != nil || n > 0 {
		return err
	}
	log.Printf("installing demo site (admin: %s)", adminEmail)
	admin, err := Admin(st, "Administrator", adminEmail, adminPassword, model.LocaleTH)
	if err != nil {
		return err
	}
	if err := Demo(st, uploadDir, admin.ID); err != nil {
		return err
	}
	return st.SetOptions(map[string]string{
		"site_title":  config.Product,
		"tagline":     "ออกแบบเว็บได้อิสระ ด้วย CMS + Visual Designer",
		"language":    "th",
		"admin_email": adminEmail,
	})
}

// Admin creates the first administrator account.
func Admin(st *store.Store, name, email, password, locale string) (*model.User, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	admin := &model.User{Email: email, Name: name, Role: model.RoleAdmin, Locale: locale}
	if err := st.CreateUser(admin, hash); err != nil {
		return nil, err
	}
	return admin, nil
}

// Blank creates a minimal published home page (setup wizard without demo).
func Blank(st *store.Store, authorID int64) error {
	b := &builder{}
	// a main menu to start from, so a new header shows links right away
	if menus, err := st.ListMenus(); err == nil && len(menus) == 0 {
		if err := st.SaveMenu(&model.Menu{Name: "เมนูหลัก", Items: mustJSON([]M{{"label": "หน้าแรก", "url": "/"}})}); err != nil {
			return err
		}
	}
	// the page itself has no padding (headers/footers added later span the full
	// width); the welcome text sits in its own section
	design := M{"id": "root", "type": "frame", "name": "หน้า", "width": fill(), "height": hug(),
		"layout": M{"mode": "vertical"}, "style": M{"fill": "#ffffff"},
		"children": []M{b.frame("ส่วนเปิดหน้า", fill(), hug(),
			M{"mode": "vertical", "align": "center", "gap": 16, "padding": pad(120, 24, 120, 24)}, M{"textAlign": "center"},
			b.text("หัวข้อ", "{{site.title}}", "h1", M{"fontSize": 48, "fontWeight": 800, "color": "#111827"}),
			b.text("คำโปรย", "{{site.tagline}}", "p", M{"fontSize": 18, "color": "#6b7280"}),
		)}}
	slug, err := st.UniqueSlug(model.TypePage, "home", 0)
	if err != nil {
		return err
	}
	p := &model.Post{Type: model.TypePage, Title: "หน้าแรก", Slug: slug, Status: model.StatusPublish,
		Design: mustJSON(design), AuthorID: authorID}
	if err := st.CreatePost(p, nil); err != nil {
		return err
	}
	return st.SetOptions(map[string]string{"home_page_id": strconv.FormatInt(p.ID, 10)})
}

// Demo installs the demo content (categories, menu, header/footer
// components, sample posts, pages and templates) authored by authorID, and
// points home_page_id / post_template_id / archive_template_id at it.
func Demo(st *store.Store, uploadDir string, authorID int64) error {
	admin := &model.User{ID: authorID}

	// taxonomy
	cats := map[string]*model.Term{}
	for _, c := range [][2]string{{"ข่าวสาร", "news"}, {"บทความ", "articles"}, {"Design", "design"}} {
		if t, err := st.TermBySlug("category", c[1]); err == nil {
			cats[c[1]] = t
			continue
		}
		t := &model.Term{Taxonomy: "category", Name: c[0], Slug: c[1]}
		if err := st.CreateTerm(t); err != nil {
			return err
		}
		cats[c[1]] = t
	}

	// navigation
	menu := &model.Menu{Name: "เมนูหลัก", Items: mustJSON([]M{
		{"label": "หน้าแรก", "url": "/"}, {"label": "บล็อก", "url": "/blog"}, {"label": "เกี่ยวกับเรา", "url": "/about"},
	})}
	if err := st.SaveMenu(menu); err != nil {
		return err
	}

	// global components (Figma main components ≈ WordPress template parts)
	header := &model.Component{Name: "ส่วนหัวเว็บ", Node: mustJSON(headerNode(menu.ID))}
	footer := &model.Component{Name: "ส่วนท้ายเว็บ", Node: mustJSON(footerNode())}
	for _, c := range []*model.Component{header, footer} {
		if err := st.SaveComponent(c); err != nil {
			return err
		}
	}
	b := &builder{header: header.ID, footer: footer.ID}

	// sample media + posts
	posts := []struct {
		title, slug, cat, excerpt, content string
		c1, c2                             color.RGBA
	}{
		{"ยินดีต้อนรับสู่เว็บไซต์ใหม่ของคุณ", "welcome", "news",
			"นี่คือบทความตัวอย่าง แก้ไขหรือลบได้จากหน้าจัดการ แล้วเริ่มเล่าเรื่องราวของคุณได้เลย",
			`<p>บทความนี้สร้างขึ้นเป็นตัวอย่าง เพื่อให้เห็นว่าบทความจะแสดงบนเว็บไซต์อย่างไร แก้ไขหรือลบได้ตลอดเวลาจากเมนู “บทความ / ข่าว” ในหน้าจัดการ</p><h2>เริ่มต้นใช้งาน</h2><ul><li>ตั้งชื่อเว็บ โลโก้ และไอคอนที่ ตั้งค่า → ทั่วไป</li><li>ปรับหน้าแรกให้เป็นแบบของคุณจากเมนู “หน้าเว็บ” แล้วกด “ออกแบบ”</li><li>เขียนบทความแรก ใส่รูปปก แล้วกดเผยแพร่</li></ul><p>ขอให้สนุกกับการสร้างเว็บไซต์ครับ</p>`,
			color.RGBA{79, 70, 229, 255}, color.RGBA{147, 51, 234, 255}},
		{"5 หลักการจัด Layout ด้วย Auto Layout", "auto-layout-principles", "design",
			"ทำความเข้าใจ Fill, Hug และ Fixed เพื่อออกแบบหน้าเว็บที่ responsive โดยไม่ต้องเขียนโค้ด",
			`<p>Auto layout ช่วยจัดวางองค์ประกอบบนหน้าเว็บให้เรียงตัวเป็นระเบียบ และปรับตามขนาดหน้าจอโดยอัตโนมัติ</p><h2>1. Fill container</h2><p>ขยายเต็มพื้นที่ของ parent</p><h2>2. Hug contents</h2><p>ขนาดพอดีกับเนื้อหา</p><h2>3. Fixed</h2><p>กำหนดขนาดเป็นพิกเซล</p><h2>4. Gap และ Padding</h2><p>ควบคุมระยะห่างอย่างเป็นระบบ</p><h2>5. Breakpoints</h2><p>override ค่าต่าง ๆ สำหรับ Tablet และ Mobile ได้อิสระ</p>`,
			color.RGBA{14, 165, 233, 255}, color.RGBA{16, 185, 129, 255}},
		{"หน้าบล็อกที่อัปเดตตัวเองอัตโนมัติ", "auto-updating-blog", "articles",
			"เขียนบทความใหม่เมื่อไร หน้าแรกและหน้าบล็อกก็แสดงบทความล่าสุดให้ทันที ไม่ต้องแก้หน้าเว็บเอง",
			`<p>ส่วน “รายการบทความ” บนหน้าเว็บจะดึงบทความล่าสุดมาแสดงให้อัตโนมัติ พร้อมรูปปก ชื่อเรื่อง คำโปรย และวันที่</p><h2>ปรับได้ตามต้องการ</h2><ul><li>เลือกจำนวนบทความที่จะแสดง</li><li>แสดงเฉพาะบางหมวดหมู่</li><li>จัดหน้าตาการ์ดบทความได้อิสระ ทุกการ์ดเปลี่ยนตามกัน</li></ul>`,
			color.RGBA{244, 63, 94, 255}, color.RGBA{249, 115, 22, 255}},
	}
	for _, p := range posts {
		var mediaID *int64
		if m, err := GradientMedia(st, uploadDir, p.slug, p.c1, p.c2, admin.ID); err == nil {
			mediaID = &m.ID
		} else {
			log.Printf("seed image: %v", err)
		}
		slug, err := st.UniqueSlug(model.TypePost, p.slug, 0)
		if err != nil {
			return err
		}
		post := &model.Post{Type: model.TypePost, Title: p.title, Slug: slug, Status: model.StatusPublish,
			Excerpt: p.excerpt, Content: p.content, FeaturedMediaID: mediaID, AuthorID: admin.ID}
		if err := st.CreatePost(post, []int64{cats[p.cat].ID}); err != nil {
			return err
		}
	}

	// pages & templates
	pages := []struct {
		typ, title, slug string
		design           M
	}{
		{model.TypePage, "หน้าแรก", "home", b.homePage()},
		{model.TypePage, "บล็อก", "blog", b.blogPage()},
		{model.TypePage, "เกี่ยวกับเรา", "about", b.aboutPage()},
		{model.TypeTemplate, "Single post", "single-post", b.singleTemplate()},
		{model.TypeTemplate, "Category archive", "category-archive", b.archiveTemplate()},
	}
	ids := map[string]int64{}
	for _, pg := range pages {
		slug, err := st.UniqueSlug(pg.typ, pg.slug, 0)
		if err != nil {
			return err
		}
		p := &model.Post{Type: pg.typ, Title: pg.title, Slug: slug, Status: model.StatusPublish,
			Design: mustJSON(pg.design), AuthorID: admin.ID}
		if err := st.CreatePost(p, nil); err != nil {
			return err
		}
		ids[pg.slug] = p.ID
	}

	return st.SetOptions(map[string]string{
		"home_page_id":        strconv.FormatInt(ids["home"], 10),
		"post_template_id":    strconv.FormatInt(ids["single-post"], 10),
		"archive_template_id": strconv.FormatInt(ids["category-archive"], 10),
		// the site header / footer (site.OptionHeaderComponent / OptionFooterComponent)
		"site_header_component": strconv.FormatInt(b.header, 10),
		"site_footer_component": strconv.FormatInt(b.footer, 10),
	})
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// GradientMedia writes a generated 1200×675 cover image to the upload dir
// (uploads/seed/<name>.png) and registers it in the media library.
func GradientMedia(st *store.Store, uploadDir, name string, c1, c2 color.RGBA, authorID int64) (*model.Media, error) {
	return GradientMediaSize(st, uploadDir, name, c1, c2, authorID, 1200, 675)
}

// GradientMediaSize is GradientMedia with a custom size.
func GradientMediaSize(st *store.Store, uploadDir, name string, c1, c2 color.RGBA, authorID int64, w, h int) (*model.Media, error) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := float64(x+y) / float64(w+h)
			img.Set(x, y, color.RGBA{lerp(c1.R, c2.R, t), lerp(c1.G, c2.G, t), lerp(c1.B, c2.B, t), 255})
		}
	}
	dir := filepath.Join(uploadDir, "seed")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return nil, err
	}
	fi, _ := f.Stat()
	m := &model.Media{Filename: name + ".png", URL: "/uploads/seed/" + name + ".png", Mime: "image/png",
		Width: w, Height: h, Alt: name, AuthorID: authorID}
	if fi != nil {
		m.Size = fi.Size()
	}
	return m, st.CreateMedia(m)
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }

// ---------------------------------------------------------------- design DSL

type builder struct {
	seq            int
	header, footer int64
}

func (b *builder) id(prefix string) string { b.seq++; return fmt.Sprintf("%s%d", prefix, b.seq) }

func fill() M                   { return M{"mode": "fill"} }
func hug() M                    { return M{"mode": "hug"} }
func fixed(v int) M             { return M{"mode": "fixed", "value": v} }
func pad(t, r, bt, l int) []int { return []int{t, r, bt, l} }

func (b *builder) frame(name string, w, h M, layout, style M, children ...M) M {
	return M{"id": b.id("f"), "type": "frame", "name": name, "width": w, "height": h,
		"layout": layout, "style": style, "children": children}
}

func (b *builder) text(name, txt, tag string, style M, extra ...M) M {
	n := M{"id": b.id("t"), "type": "text", "name": name, "width": fill(), "height": hug(),
		"style": style, "props": M{"text": txt, "tag": tag}}
	for _, e := range extra {
		for k, v := range e {
			n[k] = v
		}
	}
	return n
}

func (b *builder) instance(name string, compID int64) M {
	return M{"id": b.id("i"), "type": "instance", "name": name, "width": fill(), "height": hug(),
		"props": M{"componentId": compID}}
}

func (b *builder) page(children ...M) M {
	all := append([]M{b.instance("ส่วนหัวเว็บ", b.header)}, children...)
	all = append(all, b.instance("ส่วนท้ายเว็บ", b.footer))
	return M{"id": "root", "type": "frame", "name": "หน้า", "width": fill(), "height": hug(),
		"layout": M{"mode": "vertical"}, "style": M{"fill": "#ffffff"}, "children": all}
}

func headerNode(menuID int64) M {
	return M{"id": "c-header", "type": "frame", "name": "ส่วนหัวเว็บ", "width": fill(), "height": hug(),
		"props":      M{"tag": "header"},
		"layout":     M{"mode": "horizontal", "justify": "between", "align": "center", "padding": pad(20, 48, 20, 48), "gap": 24},
		"style":      M{"fill": "#ffffff", "shadow": "0 1px 0 rgba(17,24,39,.08)"},
		"responsive": M{"mobile": M{"layout": M{"mode": "vertical", "gap": 10, "padding": pad(14, 20, 14, 20)}}},
		"children": []M{
			{"id": "c-header-logo", "type": "text", "name": "โลโก้", "width": hug(), "height": hug(),
				"style": M{"fontSize": 22, "fontWeight": 800, "color": "#111827", "letterSpacing": -0.5},
				"props": M{"text": "◆ {{site.title}}", "href": "/"}},
			{"id": "c-header-menu", "type": "menu", "name": "เมนูหลัก", "width": hug(), "height": hug(),
				"layout":     M{"mode": "horizontal", "gap": 28, "align": "center"},
				"style":      M{"fontSize": 15, "fontWeight": 500, "color": "#374151"},
				"props":      M{"menuId": menuID},
				"responsive": M{"mobile": M{"layout": M{"gap": 14}, "style": M{"fontSize": 14}}}},
		}}
}

func footerNode() M {
	return M{"id": "c-footer", "type": "frame", "name": "ส่วนท้ายเว็บ", "width": fill(), "height": hug(),
		"props":  M{"tag": "footer"},
		"layout": M{"mode": "vertical", "align": "center", "gap": 8, "padding": pad(48, 24, 48, 24)},
		"style":  M{"fill": "#0b0f19", "color": "#9ca3af", "fontSize": 14, "textAlign": "center"},
		"children": []M{
			{"id": "c-footer-title", "type": "text", "name": "ชื่อเว็บ", "width": hug(), "height": hug(),
				"style": M{"fontSize": 18, "fontWeight": 700, "color": "#ffffff"}, "props": M{"text": "{{site.title}}"}},
			{"id": "c-footer-tag", "type": "text", "name": "คำโปรย", "width": hug(), "height": hug(),
				"props": M{"text": "{{site.tagline}}"}},
			{"id": "c-footer-copy", "type": "text", "name": "ลิขสิทธิ์", "width": hug(), "height": hug(),
				"style": M{"fontSize": 13, "opacity": 0.6}, "props": M{"text": "© 2026 {{site.title}} · สงวนลิขสิทธิ์"}},
		}}
}

func (b *builder) postCard(w M) M {
	return b.frame("การ์ดบทความ", w, hug(), M{"mode": "vertical"},
		M{"fill": "#ffffff", "radius": 16, "clip": true, "shadow": "0 1px 3px rgba(0,0,0,.06), 0 8px 24px rgba(17,24,39,.06)"},
		M{"id": b.id("img"), "type": "image", "name": "ภาพปก", "width": fill(), "height": fixed(200),
			"style": M{"fill": "#e5e7eb"}, "props": M{"src": "{{post.image}}", "alt": "{{post.title}}", "href": "{{post.url}}"}},
		b.frame("เนื้อหาการ์ด", fill(), hug(), M{"mode": "vertical", "gap": 8, "padding": pad(24, 24, 28, 24)}, M{},
			b.text("หมวดหมู่", "{{post.category}}", "span", M{"fontSize": 12, "fontWeight": 700, "color": "#6366f1", "textTransform": "uppercase", "letterSpacing": 0.8}),
			b.text("หัวข้อ", "{{post.title}}", "h3", M{"fontSize": 20, "fontWeight": 700, "lineHeight": 1.35, "color": "#111827"}, M{"props": M{"text": "{{post.title}}", "tag": "h3", "href": "{{post.url}}"}}),
			b.text("คำโปรยบทความ", "{{post.excerpt}}", "p", M{"fontSize": 15, "color": "#6b7280", "lineHeight": 1.6}),
			b.text("วันที่", "{{post.date}}", "span", M{"fontSize": 13, "color": "#9ca3af"}),
		),
	)
}

func (b *builder) homePage() M {
	hero := b.frame("ส่วนเปิดหน้า", fill(), hug(),
		M{"mode": "vertical", "align": "center", "gap": 24, "padding": pad(120, 24, 128, 24)},
		M{"fill": "linear-gradient(135deg,#4f46e5 0%,#7c3aed 55%,#c026d3 100%)", "color": "#ffffff", "textAlign": "center"},
		b.text("ข้อความนำ", "ยินดีต้อนรับ", "span", M{"fontSize": 14, "fontWeight": 600, "letterSpacing": 1, "opacity": 0.85}, M{"width": hug()}),
		b.text("หัวข้อใหญ่", "เว็บไซต์ใหม่ของคุณ\nพร้อมใช้งานแล้ว", "h1",
			M{"fontSize": 60, "fontWeight": 800, "lineHeight": 1.12, "maxWidth": 900, "letterSpacing": -1},
			M{"responsive": M{"tablet": M{"style": M{"fontSize": 46}}, "mobile": M{"style": M{"fontSize": 34}}}}),
		b.text("คำอธิบาย", "นี่คือหน้าแรกตัวอย่าง เปลี่ยนข้อความ รูปภาพ และสีได้ทุกส่วนจากหน้าออกแบบ แล้วกดเผยแพร่ได้ทันที", "p",
			M{"fontSize": 20, "lineHeight": 1.6, "opacity": 0.9, "maxWidth": 660},
			M{"responsive": M{"mobile": M{"style": M{"fontSize": 17}}}}),
		b.frame("กลุ่มปุ่ม", hug(), hug(), M{"mode": "horizontal", "gap": 12, "wrap": true, "justify": "center"}, M{},
			M{"id": b.id("btn"), "type": "button", "name": "ปุ่มหลัก", "width": hug(), "height": hug(),
				"layout": M{"padding": pad(14, 28, 14, 28)},
				"style":  M{"fill": "#ffffff", "color": "#4f46e5", "radius": 999, "fontWeight": 700, "fontSize": 16},
				"props":  M{"text": "รู้จักเรา", "href": "/about"}},
			M{"id": b.id("btn"), "type": "button", "name": "ปุ่มรอง", "width": hug(), "height": hug(),
				"layout": M{"padding": pad(14, 28, 14, 28)},
				"style":  M{"fill": "rgba(255,255,255,.12)", "color": "#ffffff", "radius": 999, "fontWeight": 600, "fontSize": 16, "stroke": "rgba(255,255,255,.5)", "strokeWidth": 1.5},
				"props":  M{"text": "อ่านบล็อก →", "href": "/blog"}},
		),
	)
	hero["responsive"] = M{"mobile": M{"layout": M{"padding": pad(72, 20, 80, 20)}}}

	card := func(icon, title, desc string) M {
		return b.frame("การ์ดจุดเด่น", fill(), hug(), M{"mode": "vertical", "gap": 12, "padding": pad(32, 28, 32, 28)},
			M{"fill": "#f9fafb", "radius": 20, "stroke": "#e5e7eb", "strokeWidth": 1},
			b.frame("ไอคอน", fixed(52), fixed(52), M{"mode": "vertical", "align": "center", "justify": "center"},
				M{"fill": "linear-gradient(135deg,#eef2ff,#f5d0fe)", "radius": 14, "fontSize": 26},
				b.text("อีโมจิ", icon, "span", M{}, M{"width": hug()})),
			b.text("หัวข้อ", title, "h3", M{"fontSize": 20, "fontWeight": 700, "color": "#111827"}),
			b.text("คำอธิบาย", desc, "p", M{"fontSize": 15, "lineHeight": 1.65, "color": "#6b7280"}),
		)
	}
	features := b.frame("ส่วนจุดเด่น", fill(), hug(), M{"mode": "vertical", "align": "center", "gap": 48, "padding": pad(96, 24, 96, 24)}, M{},
		b.text("หัวข้อส่วน", "สิ่งที่เราทำ", "h2", M{"fontSize": 38, "fontWeight": 800, "textAlign": "center", "color": "#111827"},
			M{"responsive": M{"mobile": M{"style": M{"fontSize": 28}}}}),
		func() M {
			row := b.frame("แถวจุดเด่น", fill(), hug(), M{"mode": "horizontal", "gap": 24}, M{"maxWidth": 1120},
				card("🎨", "ออกแบบ", "งานออกแบบที่สวยงาม ใช้งานง่าย และสื่อถึงตัวตนของแบรนด์ได้ชัดเจน"),
				card("📝", "เนื้อหา", "บทความ ข่าวสาร และเรื่องราวที่อัปเดตสม่ำเสมอ ให้ลูกค้ารู้จักเรามากขึ้น"),
				card("⚡", "รวดเร็ว", "เว็บไซต์โหลดเร็ว แสดงผลดีทั้งบนคอมพิวเตอร์ แท็บเล็ต และมือถือ"),
			)
			row["responsive"] = M{"tablet": M{"layout": M{"mode": "vertical"}}}
			return row
		}(),
	)

	list := M{"id": b.id("pl"), "type": "postList", "name": "บทความล่าสุด", "width": fill(), "height": hug(),
		"layout": M{"mode": "horizontal", "gap": 24}, "style": M{"maxWidth": 1120},
		"props":      M{"query": M{"postType": "post", "limit": 3, "order": "newest"}, "emptyText": "ยังไม่มีบทความ"},
		"responsive": M{"tablet": M{"layout": M{"mode": "vertical"}}},
		"children":   []M{b.postCard(fill())}}
	latest := b.frame("บทความล่าสุด", fill(), hug(), M{"mode": "vertical", "align": "center", "gap": 40, "padding": pad(96, 24, 96, 24)},
		M{"fill": "#f3f4f6"},
		b.text("หัวข้อส่วน", "บทความล่าสุด", "h2", M{"fontSize": 38, "fontWeight": 800, "textAlign": "center", "color": "#111827"},
			M{"responsive": M{"mobile": M{"style": M{"fontSize": 28}}}}),
		list,
	)

	// A freeform (absolute) section that turns into auto-layout on mobile.
	cta := b.frame("ส่วนชวนเริ่มต้น (วางอิสระ)", fill(), fixed(340), M{"mode": "none"}, M{"fill": "#111827", "clip": true, "color": "#ffffff"},
		M{"id": b.id("r"), "type": "rect", "name": "วงกลมตกแต่ง 1", "x": -90, "y": -110, "width": fixed(300), "height": fixed(300),
			"style": M{"fill": "#4f46e5", "radius": 999, "opacity": 0.55}},
		M{"id": b.id("r"), "type": "rect", "name": "วงกลมตกแต่ง 2", "x": 980, "y": 150, "width": fixed(320), "height": fixed(320),
			"style": M{"fill": "#c026d3", "radius": 999, "opacity": 0.45}},
		M{"id": b.id("t"), "type": "text", "name": "หัวข้อชวน", "x": 120, "y": 96, "width": fixed(720), "height": hug(),
			"style":      M{"fontSize": 40, "fontWeight": 800, "lineHeight": 1.2},
			"props":      M{"text": "สนใจร่วมงานกับเรา?", "tag": "h2"},
			"responsive": M{"mobile": M{"width": M{"mode": "fill"}, "style": M{"fontSize": 28}}}},
		M{"id": b.id("btn"), "type": "button", "name": "ปุ่มชวน", "x": 120, "y": 200, "width": hug(), "height": hug(),
			"layout": M{"padding": pad(14, 28, 14, 28)},
			"style":  M{"fill": "#ffffff", "color": "#111827", "radius": 12, "fontWeight": 700},
			"props":  M{"text": "ติดต่อเรา", "href": "/about"}},
	)
	cta["responsive"] = M{"tablet": M{"layout": M{"mode": "vertical", "gap": 20, "padding": pad(64, 32, 64, 32)}, "height": M{"mode": "hug"}}}
	children := cta["children"].([]M)
	children[0]["responsive"] = M{"tablet": M{"visible": false}}
	children[1]["responsive"] = M{"tablet": M{"visible": false}}

	return b.page(hero, features, latest, cta)
}

func (b *builder) sectionTitle(title, sub string) M {
	return b.frame("ส่วนหัวข้อ", fill(), hug(), M{"mode": "vertical", "align": "center", "gap": 12, "padding": pad(80, 24, 48, 24)},
		M{"textAlign": "center"},
		b.text("หัวข้อ", title, "h1", M{"fontSize": 48, "fontWeight": 800, "color": "#111827"},
			M{"responsive": M{"mobile": M{"style": M{"fontSize": 34}}}}),
		b.text("คำอธิบาย", sub, "p", M{"fontSize": 18, "color": "#6b7280", "maxWidth": 620}),
	)
}

func (b *builder) blogPage() M {
	card := b.postCard(fixed(352))
	card["responsive"] = M{"mobile": M{"width": M{"mode": "fill"}}}
	list := M{"id": b.id("pl"), "type": "postList", "name": "บทความทั้งหมด", "width": fill(), "height": hug(),
		"layout":   M{"mode": "horizontal", "gap": 24, "wrap": true, "justify": "center", "padding": pad(0, 24, 96, 24)},
		"style":    M{"maxWidth": 1150},
		"props":    M{"query": M{"postType": "post", "limit": 12, "order": "newest"}, "emptyText": "ยังไม่มีบทความ"},
		"children": []M{card}}
	return b.page(b.sectionTitle("บล็อก", "เรื่องราว แนวคิด และอัปเดตล่าสุดจากทีมงาน"), list, M(PagerNode(list["id"].(string))))
}

func (b *builder) aboutPage() M {
	body := b.frame("เนื้อหา", fill(), hug(), M{"mode": "vertical", "gap": 20, "padding": pad(0, 24, 96, 24)}, M{"maxWidth": 760},
		b.text("ย่อหน้า", "เล่าเรื่องราวของคุณที่นี่ — คุณเป็นใคร ทำอะไร และอะไรที่ทำให้ลูกค้าเลือกคุณ ข้อความนี้เป็นตัวอย่าง แก้ไขได้ทั้งหมด", "p",
			M{"fontSize": 18, "lineHeight": 1.8, "color": "#374151"}),
		b.text("ย่อหน้า", "อยากคุยกับเรา? เพิ่มเบอร์โทร อีเมล หรือแบบฟอร์มติดต่อลงในหน้านี้ได้จากหน้าออกแบบ", "p",
			M{"fontSize": 18, "lineHeight": 1.8, "color": "#374151"}),
	)
	return b.page(b.sectionTitle("เกี่ยวกับเรา", "ทำความรู้จักเรามากขึ้น"), body)
}

func (b *builder) singleTemplate() M {
	article := b.frame("บทความ", fill(), hug(), M{"mode": "vertical", "gap": 20, "padding": pad(48, 24, 96, 24)}, M{"maxWidth": 800},
		M{"id": b.id("bc"), "type": "breadcrumb", "name": "เส้นทางนำทาง", "width": fill(), "height": hug(),
			"style": M{"fontSize": 14, "color": "var(--c-muted)"},
			"props": M{"homeLabel": "หน้าแรก", "separator": "›", "currentColor": "var(--c-primary)", "hideCategory": false}},
		b.text("หมวดหมู่", "{{post.category}}", "span", M{"fontSize": 13, "fontWeight": 700, "color": "var(--c-primary)", "textTransform": "uppercase", "letterSpacing": 0.8}),
		b.text("หัวข้อ", "{{post.title}}", "h1", M{"fontSize": 44, "fontWeight": 800, "lineHeight": 1.2, "color": "#111827"},
			M{"responsive": M{"mobile": M{"style": M{"fontSize": 30}}}}),
		b.text("ผู้เขียนและวันที่", "{{post.author}} · {{post.date}}", "p", M{"fontSize": 15, "color": "#6b7280"}),
		M{"id": b.id("img"), "type": "image", "name": "ภาพปก", "width": fill(), "height": fixed(420),
			"style": M{"fill": "#e5e7eb", "radius": 20}, "props": M{"src": "{{post.image}}", "alt": "{{post.title}}"},
			"responsive": M{"mobile": M{"height": M{"mode": "fixed", "value": 220}}}},
		M{"id": b.id("pc"), "type": "postContent", "name": "เนื้อหาบทความ", "width": fill(), "height": hug(),
			"style": M{"fontSize": 18, "color": "#1f2937"}},
		M(TagsNode()),
	)
	return b.page(article)
}

func (b *builder) archiveTemplate() M {
	card := b.postCard(fixed(352))
	// 2.5.0: on a content type's archive (/menu, /events) the card shows the post's first fields, not a date
	body := card["children"].([]M)[1]
	kids := body["children"].([]M)
	date := kids[len(kids)-1]
	date["props"].(M)["showWhen"] = "post.type == 'post'"
	body["children"] = append(kids, M(CardFieldsNode()))
	card["responsive"] = M{"mobile": M{"width": M{"mode": "fill"}}}
	list := M{"id": b.id("pl"), "type": "postList", "name": "บทความในหมวด", "width": fill(), "height": hug(),
		"layout":   M{"mode": "horizontal", "gap": 24, "wrap": true, "justify": "center", "padding": pad(0, 24, 96, 24)},
		"style":    M{"maxWidth": 1150},
		"props":    M{"query": M{"postType": "post", "category": "current", "limit": 24}, "emptyText": "ยังไม่มีบทความในหมวดนี้"},
		"children": []M{card}}
	return b.page(b.sectionTitle("{{post.title}}", "{{post.excerpt}}"), list, M(PagerNode(list["id"].(string))))
}
