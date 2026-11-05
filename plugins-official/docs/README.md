# Docs (`docs`) — official plugin 1.1.5

## ภาษาไทย

ปลั๊กอินทางการของ Anasaki Studio สำหรับทำ **ศูนย์รวมเอกสาร** ที่ `/docs` หน้าตาแบบเว็บเอกสารสมัยใหม่ (Stripe / Vercel /
Docusaurus): เมนูด้านซ้ายแบ่งตามหมวด, breadcrumb, สารบัญ "ในหน้านี้" ด้านขวา (ไฮไลต์หัวข้อที่กำลังอ่าน), ก่อนหน้า/ถัดไป,
วันที่อัปเดตล่าสุด, ค้นหา (Ctrl K หรือ `/`), ปุ่มคัดลอกโค้ด และเมนูแบบลิ้นชักบนมือถือ — ใช้สีและฟอนต์ตามธีมของเว็บ
(`--c-primary`, `--c-text`, `--c-muted`, `--c-surface`) อยู่ภายในส่วนหัวและส่วนท้ายของเว็บ

- **ประเภทเนื้อหา "เอกสาร" (`docs_doc`)** — เขียนในหน้าแก้ไขโพสต์ปกติ (เมนู เอกสาร ในหลังบ้าน) มีชื่อ เนื้อหา และคำโปรย
  (ใช้เป็นคำนำใต้ชื่อและ meta description) พร้อมช่อง:
  | ช่อง | ความหมาย |
  |---|---|
  | `product` | รหัสผลิตภัณฑ์ (ต้องตรงกับรหัสในการตั้งค่า `products` — ค่าเริ่มต้นมีแค่ `general`) |
  | `section` | หมวดในเมนูด้านข้าง เช่น "เริ่มต้น" |
  | `order` | ลำดับในหมวด (น้อยขึ้นก่อน) |
  | `sectionOrder` | ลำดับของหมวด (ไม่บังคับ — ใช้ค่าน้อยที่สุดของบทความในหมวด, หมวดที่ไม่ตั้งเรียงตามบทความแรก) |
- **หน้าเว็บ**
  | URL | หน้า |
  |---|---|
  | `/docs` | หน้าแรก: หัวข้อ คำแนะนำ ช่องค้นหา การ์ดผลิตภัณฑ์ (จำนวนบทความ, "เริ่มอ่าน →") และ "เริ่มต้นอย่างรวดเร็ว" |
  | `/docs/<product>` | หน้าผลิตภัณฑ์: เมนูด้านข้าง + การ์ดบทความแยกตามหมวด |
  | `/docs/<product>/<slug>` | บทความ: เมนู (สลับผลิตภัณฑ์ได้), breadcrumb, สารบัญจาก h2/h3, ก่อนหน้า/ถัดไป, อัปเดตล่าสุด |
  | `/docs/search.json` | ดัชนีค้นหา (title, excerpt, product, section, url, headings) — ค้นหาในเบราว์เซอร์ |
  | `/docs/<product>/<slug>.md` | บทความเป็น Markdown (ปุ่ม "คัดลอก Markdown" / "ดูเป็น Markdown" ในหัวบทความ) |
  | `/docs/llms.txt`, `/docs/llms-full.txt` | สารบัญสำหรับ AI ตามรูปแบบ [llmstxt.org](https://llmstxt.org) และเนื้อหาทั้งหมดเป็น Markdown ไฟล์เดียว |

  แสดงเฉพาะบทความที่ **เผยแพร่แล้ว** · หน้าที่ไม่มี → 404 ภายในเลย์เอาต์ของเว็บ · `/docs/<slug>` (ลิงก์ "ดู" ของหลังบ้าน)
  และผลิตภัณฑ์ผิด → redirect 301 ไปยัง URL หลักของบทความ
- **เนื้อหา** — หัวข้อ h2/h3 ได้ id อัตโนมัติ (ลิงก์ `#` ข้างหัวข้อ), ตารางเลื่อนแนวนอนได้, ลิงก์ภายนอกเปิดแท็บใหม่,
  `<pre><code class="language-bash">` มีแถบชื่อภาษาและปุ่มคัดลอก, กล่องข้อความ: `<blockquote>` หรือ
  `<div class="dx-note">` / `dx-tip` / `dx-warn` (หรือ `dx-warning`) / `dx-danger`,
  **การ์ดลิงก์** `<div class="dx-cards"><a href="…"><strong>ชื่อ</strong> คำอธิบาย</a>…</div>` และ
  **แท็บโค้ด** `<div class="dx-tabs"><pre title="npm"><code class="language-bash">…</code></pre><pre title="pnpm">…</pre></div>`
  (ชื่อแท็บจาก `title` หรือชื่อภาษา — แท็บที่เลือกจำไว้ทั้งเว็บ) · เมนูด้านข้างพับแต่ละหมวดได้ (จำไว้ในเบราว์เซอร์)
  · สีตามธีมของเว็บ รวมถึง `--c-bg` ถ้าธีมหรือปลั๊กอินโหมดมืดตั้งไว้
- **SEO** — `page.meta` (คำโปรยเป็น description, canonical, `og:type`) ทำงานร่วมกับปลั๊กอิน SEO (รูปแบบชื่อหน้า) และเพิ่ม
  `/docs`, หน้าผลิตภัณฑ์ และทุกบทความใน `/sitemap.xml` (ลิงก์เดิมที่ redirect แล้วถูกตัดออก) · BreadcrumbList (JSON-LD) ในหน้าผลิตภัณฑ์และบทความ ·
  บน CMS 2.0.8+ แผง SEO อยู่ในหน้าแก้ไขบทความ และค่า SEO รายบทความใช้กับหน้าเว็บ (`postId`)
- **เปลี่ยนเส้นทางจากลิงก์เดิม** (`legacyRedirects`) — เช่นย้ายบทความจาก `/blog/...` มา `/docs/...` ลิงก์เดิมยังใช้ได้ (301,
  ไม่สนใจ `/` ท้าย, query string ตามไปด้วย, เฉพาะ GET/HEAD)
- **หน้า "เอกสาร (ตั้งค่า)" ในหลังบ้าน** (บรรณาธิการและผู้ดูแลระบบ): รายการบทความแยกตามผลิตภัณฑ์/หมวด (สถานะ ปุ่มแก้ไข) —
  **ลาก ⠿ เพื่อเรียงบทความหรือหมวด ย้ายบทความไปหมวดอื่น** (หรือปุ่ม ↑ ↓) และ ✎ เปลี่ยนชื่อหมวด แล้วกด "บันทึกลำดับ",
  **นำเข้าชุดเอกสาร JSON** (เลือกไฟล์หรือวาง — บทความที่ slug ซ้ำจะถูกอัปเดต ไม่สร้างซ้ำ, ส่งทีละ 25 บทความ) และแท็บตั้งค่า
  (ผู้ดูแลระบบ) เป็นฟอร์มปกติ: หัวข้อ คำแนะนำ ลิงก์แก้ไข ผลิตภัณฑ์ (เพิ่ม/ลบ/เรียง — รหัสที่มีบทความใช้อยู่แก้ไม่ได้) และการเปลี่ยนเส้นทาง

  ปลั๊กอิน **ไม่มีบทความหรือข้อมูลของเว็บใดติดมา** — ติดตั้งแล้วจะเป็นศูนย์รวมเอกสารว่าง ๆ หมวดเดียว ("เอกสาร", รหัส `general`)
  จนกว่าจะเขียนบทความเองหรือนำเข้าชุดเอกสารของตัวเอง ชุดเอกสารใส่การตั้งค่าของเว็บมาด้วยได้ (`products`, `hubTitle`, `hubIntro`,
  `legacyRedirects` — ใช้เมื่อผู้ดูแลระบบติ๊ก "ใช้การตั้งค่าในชุดข้อมูลด้วย") มีแค่ `docs` ที่จำเป็น:

  ```json
  { "products": [ { "id": "guide", "name": { "th": "คู่มือ", "en": "Guide" }, "icon": "📘" } ],
    "hubTitle": "ศูนย์ช่วยเหลือ",
    "docs": [ { "slug": "install", "title": "ติดตั้งด้วย Docker", "excerpt": "…", "product": "guide",
                "section": "เริ่มต้น", "order": 1, "sectionOrder": 1, "content": "<p>…</p>" } ],
    "legacyRedirects": [ { "from": "/blog/getting-started", "to": "/docs/guide/install" } ] }
  ```

**การตั้งค่า** (หน้า เอกสาร (ตั้งค่า) → ตั้งค่า หรือ ปลั๊กอิน → Docs → ตั้งค่า): `hubTitle`, `hubIntro`, `editUrl` (ลิงก์
"แก้ไขหน้านี้" เช่น `https://github.com/<org>/<repo>/edit/main/docs/{product}/{slug}.md` — ใช้ `{id}` `{slug}` `{product}` ได้,
ทุกคนเห็นลิงก์นี้ จึงไม่ควรใส่ที่อยู่หลังบ้าน), `products` (JSON:
`[{"id","name":{th,en},"desc":{th,en},"icon"}]`, เว้นว่าง = หมวดเดียว "เอกสาร" รหัส `general`), `legacyRedirects`
(JSON `[{from,to}]` หรือบรรทัดละคู่ "/เดิม /ใหม่") · URL อยู่ที่ `/docs` เสมอ (ประกาศใน `code.routes`)

**เวอร์ชัน CMS:** ต้องการ Anasaki Studio **2.0.5** ขึ้นไป (ปลั๊กอินลงทะเบียนประเภทเนื้อหาได้ — `site.postType`, scope `post-types`)

**สิทธิ์ (scopes):** `post-types` (ประเภท `docs_doc`), `content` / `content-write` (อ่านและนำเข้าบทความ), `routes` (`/docs`),
`filters-core` (`page.meta`, `sitemap.urls`), `middleware` (redirect ลิงก์เดิม), `admin-pages`

**ถอนการติดตั้ง:** บทความยังอยู่ในฐานข้อมูล (ซ่อนจากหลังบ้านจนกว่าจะติดตั้งปลั๊กอินอีกครั้ง) และอยู่ในไฟล์ส่งออกเว็บไซต์

**สิทธิ์ (CMS 2.0.6+):** `docs.edit` — เขียน/แก้เอกสาร · `docs.publish` — เผยแพร่ · `docs.manage` — หน้า เอกสาร (ตั้งค่า) และนำเข้า (ค่าเริ่มต้น: ผู้ดูแลระบบ, บรรณาธิการ) — ปรับได้ที่ ผู้ใช้ → ตำแหน่งและสิทธิ์ · CMS รุ่นก่อนหน้าใช้รายการตำแหน่งแบบเดิม

## English

The official Anasaki Studio plugin for a **documentation hub** at `/docs`, styled like modern docs sites (Stripe / Vercel /
Docusaurus): a sectioned left sidebar, breadcrumb, an "On this page" table of contents with scroll-spy, prev/next cards,
last-updated date, search (Ctrl K or `/`), copy buttons on code blocks and a drawer on phones — in the site's header and footer,
using the theme's colours and fonts.

- **Content type "Docs" (`docs_doc`)** — written in the normal post editor (Docs in the admin menu): title, content and excerpt
  (the lead under the title and the meta description), plus the fields `product` (a product id from the `products` setting),
  `section`, `order` (lowest first) and `sectionOrder` (optional; a section uses its docs' smallest value, unnumbered sections
  follow in order of their first doc).
- **Pages**: `/docs` (hub: title, intro, search, product cards with counts and "Start reading", quick start), `/docs/<product>`
  (landing: sidebar + doc cards per section), `/docs/<product>/<slug>` (article), `/docs/search.json` (client-side search
  index: title, excerpt, product, section, url, headings), `/docs/<product>/<slug>.md` (the article as Markdown — "Copy as
  Markdown" / "View as Markdown" in the article header), `/docs/llms.txt` ([llmstxt.org](https://llmstxt.org) index) and
  `/docs/llms-full.txt` (every article as one Markdown file). Published docs only; unknown pages get a 404 in the site's layout;
  `/docs/<slug>` (the admin's "view" link) and a wrong product redirect (301) to the article's canonical URL.
- **Content**: h2/h3 get ids (with a `#` anchor), tables scroll, external links open in a new tab, `<pre><code class="language-x">`
  gets a language bar and a copy button; callouts with `<blockquote>` or `<div class="dx-note">` / `dx-tip` / `dx-warn` (`dx-warning`) /
  `dx-danger`; link cards `<div class="dx-cards"><a href="…"><strong>Title</strong> text</a>…</div>`; code tabs
  `<div class="dx-tabs"><pre title="npm"><code class="language-bash">…</code></pre><pre title="pnpm">…</pre></div>` (tab label =
  `title` or the language; the picked tab is remembered site-wide). Sidebar sections collapse (remembered per browser). Colours
  follow the site's theme, including `--c-bg` when a theme or a dark-mode plugin sets it.
- **SEO**: `page.meta` (excerpt as description, canonical, `og:type`) — works with the SEO plugin's title template — and `/docs`,
  the product pages and every doc in `/sitemap.xml` (old addresses that now redirect are left out) · a BreadcrumbList (JSON-LD) on
  product pages and articles · on CMS 2.0.8+ the SEO panel is in the doc editor and its values apply to the article (`postId`).
- **Legacy redirects** (`legacyRedirects`): old links keep working after content moves (301, trailing slash ignored, query kept,
  GET/HEAD only).
- **Admin page "Docs hub"** (editors and administrators): docs grouped by product / section (status, edit links) — **drag ⠿ to
  reorder docs and sections or move a doc to another section** (or ↑ ↓), ✎ renames a section, then "Save order"; the **JSON
  bundle import** (file or paste; existing slugs are updated, never duplicated; sent 25 docs at a time) and the settings tab
  (administrators) as a normal form: title, intro, edit link, products (add / remove / reorder — an id in use is locked) and
  redirects. Bundle format: see the Thai section above.
- **Nothing site-specific is bundled**: a fresh install is an empty hub with one group ("Documentation", id `general`) until you
  write docs or import your own bundle. A bundle may carry the site's settings too (`products`, `hubTitle`, `hubIntro`,
  `legacyRedirects` — applied when an administrator ticks "Also apply the bundle's settings"); only `docs` is required.

**Settings:** `hubTitle`, `hubIntro`, `editUrl` ("Edit this page" link template with `{id}` `{slug}` `{product}`, e.g. a GitHub
edit URL — public, so never the admin address), `products` (JSON `[{"id","name":{th,en},"desc":{th,en},"icon"}]`; empty = one group
"Documentation", id `general`), `legacyRedirects` (JSON `[{from,to}]` or "/old /new" lines). The URL is always `/docs` (declared in
`code.routes`).

**CMS version:** needs Anasaki Studio **2.0.5** or newer (plugins may register post types — `site.postType`, scope `post-types`).

**Scopes:** `post-types`, `content`, `content-write`, `routes`, `filters-core`, `middleware`, `admin-pages`.

**Uninstalling** keeps the docs in the database (hidden in the admin until the plugin is installed again) and in site exports.

**Permissions (CMS 2.0.6+):** `docs.edit` — write / edit docs · `docs.publish` — publish them · `docs.manage` — the Docs hub page and import (default: administrators, editors) — change them in Users → Roles · older CMS versions keep the former role list
