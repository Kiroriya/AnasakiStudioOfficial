# REA: WordPress × Figma → REA Studio (Angular + Go)

เอกสารนี้สรุปการ Reverse Engineer แนวคิดหลักของ **WordPress** (ระบบจัดการเนื้อหา) และ **Figma** (เครื่องมือออกแบบ) แล้วนำมาสร้างใหม่เป็นระบบเดียวด้วย **Angular 22** และ **Go 1.26 + MySQL**

> ขอบเขตทางกฎหมาย: WordPress เป็น open source (GPL) จึงศึกษาโครงสร้างได้โดยตรง ส่วน Figma เป็น closed source เราจึงทำแบบ clean-room คือเลียนแบบ *พฤติกรรมและแนวคิด* (frame, auto layout, components, layers) โดยไม่ใช้โค้ดหรือรูปแบบไฟล์ของ Figma

---

## 1. สิ่งที่ได้จากการวิเคราะห์ WordPress

| แนวคิดใน WordPress | กลไกเดิม | REA Studio |
|---|---|---|
| `wp_posts` ตารางเดียวสำหรับทุก content type | `post_type` = post / page / attachment / nav_menu_item / wp_template … | ตาราง `posts` ใช้ `type` = `post` / `page` / `template` และเพิ่มคอลัมน์ `design JSON` |
| `wp_terms` + `wp_term_taxonomy` + `wp_term_relationships` | 3 ตาราง | ยุบเหลือ `terms` + `post_terms` |
| `wp_options` | key/value autoload | `options` (site_title, tagline, home_page_id, post_template_id, archive_template_id) |
| Roles & Capabilities | administrator / editor / author … + `current_user_can()` | role → capability matrix ใน [auth.go](../backend/internal/auth/auth.go) + meta-cap `canEdit` (own vs others) |
| `unfiltered_html` / `wp_kses` | กรอง HTML ตามสิทธิ์ | `sanitizeHTML` สำหรับผู้ที่ไม่มี `unfiltered_html` |
| Revisions | เก็บเป็น post_type=revision | ตาราง `revisions` เก็บล่าสุด 25 ฉบับ พร้อมกู้คืน |
| Media Library | attachment posts + ไฟล์ใน `wp-content/uploads/YYYY/MM` | ตาราง `media` + `uploads/YYYY/MM` |
| Nav Menus | nav_menu_item posts | ตาราง `menus` (items JSON) |
| Template hierarchy | front-page → page → single → category → 404 | [site.go](../backend/internal/site/site.go): `/` → `/{slug}` → `/blog/{slug}` (Single template) → `/category/{slug}` (Archive template) → 404 page |
| Block: Query Loop | วนซ้ำ inner blocks ต่อโพสต์ | node `postList`: ลูกคือ template ต่อบทความ |
| Block: Navigation / Post Content | | node `menu` / `postContent` |
| Template parts (header/footer) | | **Components** ของ Figma (ใช้ซ้ำทุกหน้า) |
| REST API `/wp-json/wp/v2` | | `/api/v1` (JWT) |

## 2. สิ่งที่ได้จากการวิเคราะห์ Figma

| แนวคิดใน Figma | REA Studio |
|---|---|
| Document = tree ของ nodes (Frame, Text, Rectangle, Image, Instance) | `DNode` tree ([design-types.ts](../frontend/src/app/editor/design-types.ts), [node.go](../backend/internal/design/node.go)) |
| Auto layout (direction, gap, padding, alignment, wrap) | `layout.mode = horizontal / vertical` → **CSS Flexbox** |
| Freeform / absolute positioning | `layout.mode = none` → ลูกเป็น `position:absolute` ด้วย x,y |
| Resizing: Fixed / Fill container / Hug contents | `width/height.mode = fixed / fill / hug` → flex-grow / align-self / fit-content |
| Main component + instances | ตาราง `components` + node `instance` (แก้ main → ทุกหน้าอัปเดต, Detach ได้) |
| Layers panel, selection, resize handles, undo/redo | Angular editor (signals + structuredClone snapshots) |
| Variables / modes (บางส่วน) | **Breakpoint overrides**: `responsive.tablet / mobile` ทับค่าเฉพาะหน้าจอ (cascade แบบ media query) |
| Prototype / Dev mode (inspect CSS) | ไม่ต้องใช้เลย เพราะดีไซน์ **คือ** หน้าเว็บจริงที่ Go render ออกมา |

## 3. จุดที่รวมสองระบบเข้าด้วยกัน

1. **Data binding**: ทุก text / image src / link ใช้ `{{site.title}}`, `{{post.title}}`, `{{post.image}}`, `{{post.url}}` ฯลฯ ได้
2. **Post list (Query Loop)** เป็น node ที่มี auto layout ของตัวเอง ลูกของมันคือ card template ที่ออกแบบได้อิสระ
3. **Templates** ออกแบบใน Designer เหมือนหน้าปกติ แล้วผูกกับ Single post / Category archive ใน Settings
4. **Renderer เดียวกันสองฝั่ง**: `Decls()` ใน Go ([css.go](../backend/internal/design/css.go)) และ `nodeCss()` ใน TypeScript ([design-css.ts](../frontend/src/app/editor/design-css.ts)) ใช้กฎเดียวกัน canvas จึงตรงกับเว็บจริง (มี unit test ยืนยันฝั่ง Go)
5. **Responsive**: Go สร้าง CSS ฐาน แล้วสร้าง `@media (max-width:1024px)` / `(max-width:640px)` เฉพาะ declaration ที่ต่างไป (รวมถึงกรณี parent เปลี่ยน layout แล้วลูกต้องเปลี่ยนกฎ flex)

```
 Angular (admin + designer)                    Go server (:8090)
 ┌───────────────────────────┐   REST/JWT   ┌────────────────────────────────┐
 │ CMS: pages/posts/media/…  │ ───────────▶ │ api/   handlers + capabilities │
 │ Designer: canvas, layers, │              │ store/ MySQL repository        │
 │ props, components, bp     │              │ design/ node tree → HTML + CSS │◀── MySQL 8
 │ nodeCss() ≡ Decls()       │              │ site/  template hierarchy      │
 └───────────────────────────┘              └──────────────┬─────────────────┘
                                                           │ HTML/CSS ล้วน (ไม่มี JS)
                                                      ผู้เยี่ยมชมเว็บ
```

## 4. โมเดลข้อมูลของดีไซน์ (ตัวอย่าง)

```json
{
  "id": "hero", "type": "frame", "name": "Hero",
  "width": { "mode": "fill" }, "height": { "mode": "hug" },
  "layout": { "mode": "vertical", "gap": 24, "align": "center", "padding": [120, 24, 120, 24] },
  "style": { "fill": "linear-gradient(135deg,#4f46e5,#c026d3)", "color": "#fff" },
  "responsive": { "mobile": { "layout": { "padding": [72, 20, 72, 20] } } },
  "children": [
    { "id": "h1", "type": "text", "width": { "mode": "fill" }, "height": { "mode": "hug" },
      "style": { "fontSize": 60, "fontWeight": 800 }, "props": { "text": "{{site.title}}", "tag": "h1" },
      "responsive": { "mobile": { "style": { "fontSize": 34 } } } }
  ]
}
```

## 5. ข้อจำกัดของ MVP และงานถัดไป

- ยังไม่มี real-time collaboration (Figma multiplayer) แนวทางคือใช้ WebSocket + CRDT (เช่น Yjs) บน design JSON
- Component instances ยังไม่รองรับ override ข้อความ/ภาพต่อ instance (Figma instance overrides)
- ยังไม่มีระบบ Plugin/Hooks แบบ `add_action` / `add_filter` แนวทางคือ Go interface + event bus
- ยังไม่มี import ไฟล์ `.fig` หรือ WordPress WXR export (ทำได้ด้วย Figma REST API / WXR parser)
- Multi-select, groups, constraints แบบ Figma, design tokens/variables
- Role ที่เก็บใน JWT ถูกตรวจซ้ำกับฐานข้อมูลทุก request แล้ว แต่ยังไม่มี token revocation list
