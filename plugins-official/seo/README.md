# SEO (`seo`) — official plugin 2.0.0

## ภาษาไทย

ปลั๊กอินทางการของ Anasaki Studio สำหรับ SEO — แทนปลั๊กอิน SEO แบบ built-in เดิม (ทำงานเหมือนเดิมทุกอย่าง)

- **ชื่อหน้า (`<title>`)** ตามรูปแบบ `รูปแบบชื่อหน้า` (ค่าเริ่มต้น `%title% – %site%`, ใช้ `%title%` `%site%` `%tagline%`)
  หน้าที่ไม่มีชื่อหรือชื่อเหมือนชื่อเว็บจะแสดงชื่อเว็บอย่างเดียว
- **meta description / รูปแชร์** — ใช้ค่าของหน้า ถ้าไม่มีใช้ค่าเริ่มต้นทั้งเว็บ
- **canonical, Open Graph (`og:*`), Twitter card** ทุกหน้า (`og:type` = `article` สำหรับบทความ,
  `twitter:card` = `summary_large_image` เมื่อมีรูป, `twitter:site` จากบัญชี X/Twitter)
- **ปรับแต่งรายหน้า** ได้สองที่: แผง **SEO** ในหน้าแก้ไขโพสต์/หน้า (ทุกคนที่แก้โพสต์นั้นได้ รวมผู้เขียนกับบทความของตัวเอง
  เหมือนเดิม) และหน้า **SEO** ในหลังบ้าน (ผู้ดูแลระบบ: ตัวอย่างผลการค้นหาของหน้า/บทความที่เผยแพร่แล้ว) — ชื่อ คำอธิบาย
  รูปแชร์ และ `noindex` (ไม่ให้ Google แสดงหน้านี้ — หน้าจะไม่อยู่ใน sitemap ด้วย) เก็บใน post meta `seo.*`
- **ค่าเริ่มต้นทั้งเว็บ** แก้ได้ในหน้า SEO (ฟอร์มด้านขวา) หรือที่ ปลั๊กอิน → SEO → ตั้งค่า (ค่าเดียวกัน)
- **/sitemap.xml และ /robots.txt** อัตโนมัติ (ระบบเก็บ sitemap ไว้ใช้ซ้ำจนกว่าเนื้อหาจะเปลี่ยน; ถ้าตั้ง
  "ไม่ให้เครื่องมือค้นหาเก็บข้อมูล" robots.txt จะเป็น `Disallow: /` และ sitemap ว่าง)

**การตั้งค่า** (ปลั๊กอิน → SEO → ตั้งค่า): `titleTemplate`, `defaultDescription`, `defaultImage`, `twitterHandle` —
ชื่อเดียวกับปลั๊กอินเดิม ค่าที่ตั้งไว้จึงย้ายมาได้ทันที

**สิทธิ์ (scopes):** `content` (รายการหน้า/บทความ), `content-fields` (ช่อง SEO ในหน้าแก้ไขโพสต์ = post meta `seo.*`),
`filters-core` (`page.meta`, `sitemap.urls`), `admin-pages` · หน้า SEO ใช้ได้เฉพาะผู้ดูแลระบบ (administrator)

**ย้ายข้อมูลจากปลั๊กอินเดิม:** ระบบย้ายตาราง `seo_meta` เดิมเข้า post meta `seo.*` ให้อัตโนมัติผ่าน export `legacyImport`
(รันซ้ำได้ ไม่ซ้ำข้อมูล ข้ามโพสต์ที่ถูกลบไปแล้ว) ตารางเดิมไม่ถูกลบจนกว่าผู้ดูแลจะกด "ล้างข้อมูลเก่า"

**สำหรับนักพัฒนา:** `import { applyTemplate } from 'seo'` (library), `plugins.call('seo', 'metaOf', postId)` →
`{title, description, image, noindex}`, หรืออ่าน `post.meta['seo.title']` จาก `db.posts`

## English

The official Anasaki Studio SEO plugin — replaces the former built-in SEO plugin with the same behaviour.

- **`<title>`** from the *title template* (default `%title% – %site%`; `%title%` `%site%` `%tagline%`); a page without a
  title, or titled like the site, shows the site title alone.
- **Meta description / share image**: the page's own, else the site-wide defaults.
- **Canonical, Open Graph and Twitter card** tags on every page (`og:type` `article` for posts, `summary_large_image`
  when there is an image, `twitter:site` from the handle).
- **Per-page overrides** in the **SEO panel of the post / page editor** (everyone who may edit the post, authors on
  their own posts, like the built-in panel) and on the **SEO** admin page (administrators: search-result previews of
  published pages and posts) — title, description, share image and `noindex` (noindex pages are also left out of the
  sitemap), stored as post meta `seo.*`.
- **Site-wide defaults**: the form on the SEO page or Plugins → SEO → Settings (the same values).
- **/sitemap.xml and /robots.txt** (core reuses the sitemap until something it depends on changes; search engines
  discouraged → `Disallow: /` and an empty sitemap).

**Settings** (Plugins → SEO → Settings): `titleTemplate`, `defaultDescription`, `defaultImage`, `twitterHandle` — the
same keys as the built-in plugin, so existing values carry over.

**Scopes:** `content`, `content-fields` (the editor's SEO fields, post meta `seo.*`), `filters-core` (`page.meta`,
`sitemap.urls`), `admin-pages`. The SEO page is for administrators.

**Migration from the built-in plugin:** the old `seo_meta` rows are imported into post meta `seo.*` through the
`legacyImport` export (idempotent; rows of deleted posts are skipped). The old table is kept until an administrator
presses "Clean up old data".

**Developers:** `import { applyTemplate } from 'seo'` (library); `plugins.call('seo', 'metaOf', postId)`; or read
`post.meta['seo.title']` from `db.posts`.
