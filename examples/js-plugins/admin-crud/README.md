# Admin data screens (`admin-crud`) — หน้าจัดการข้อมูลในแอดมิน

## ภาษาไทย

ปลั๊กอินระบบที่สร้างหน้าแอดมิน (**Developer → จัดการข้อมูล**) สำหรับตารางของปลั๊กอินเอง **หรือข้อมูลของปลั๊กอินอื่นผ่าน
export ของปลั๊กอินนั้น**: รายการ, ค้นหา, แบ่งหน้า, เพิ่ม, แก้ไข, ลบ, **ส่งออก CSV** — กำหนดด้วย JSON ไม่ต้องเขียนหน้าจอเอง

- ตารางตัวอย่างจาก migration: `contacts` (รายชื่อติดต่อ) และ `faqs` (คำถามที่พบบ่อย) → `p_admin_crud_contacts`, `p_admin_crud_faqs`
- คำอธิบายตาราง (`resources`) อยู่ใน `code/config.json`: `id`, `table`, `label {th,en}`, `perPage`, `orderBy`
  (`"-id"` = มากไปน้อย), `columns[]` ที่มี `name`, `label`, `type` (`text textarea email number bool date datetime select`),
  `required`, `max`, `min`, `options`, `list` (แสดงในตาราง), `search` (ค้นด้วยช่องค้นหา)
- การบันทึก/ลบเรียก `site.action` `admin-crud:save` / `admin-crud:delete` ผ่าน `anasaki.call` ในหน้าแอดมิน จึงปรับสิทธิ์ได้ที่
  หน้า **Access** (ค่าเริ่มต้น: ผู้ดูแลระบบ) ตรวจค่าตามชนิดคอลัมน์ฝั่ง server และ escape HTML ทุกค่า
- **ส่งออก CSV**: ปุ่มในหน้ารายการเรียก `anasaki.download({ r, q })` → `download(ctx)` ของ `site.adminPage` (ใช้ ticket และ
  ตรวจสิทธิ์เดียวกับหน้า) ได้ไฟล์ CSV (UTF-8 BOM ให้ Excel อ่านไทยได้, กันสูตร `= + - @`) ตามคำค้น สูงสุด 10,000 แถว
- **ข้อมูลของปลั๊กอินอื่น** (`plugins.call`): resource ที่มี `source` แทน `table`
  `{ "plugin": "shop", "list": "listOrders", "get": "getOrder", "save": "saveOrder", "delete"?: "…" }` — ชื่อเหล่านี้ต้องอยู่ใน
  `code.exports` ของปลั๊กอินนั้น: `list({ q, page, perPage, orderBy })` → `{ items, total }` (หรืออาร์เรย์), `get(id)` → แถว,
  `save({ id, values })` → `{ id }` หรือ `{ errors }`, `delete(id)` → จำนวนที่ลบ. ไม่มี `save`/`delete` = ทำไม่ได้ (ซ่อนปุ่ม).
  `config.json` มีตัวอย่าง resource `orders` จากปลั๊กอิน **shop**: ถ้ายังไม่ได้ติดตั้ง/เปิดใช้ shop แท็บนั้นแสดงข้อความแจ้ง
  (`plugins.has`) ส่วนการบันทึก/ส่งออกตอบเป็นข้อผิดพลาดแทนการล่ม (error `code: 'unavailable'`)
- ภาษา: ตามภาษาแอดมินของผู้ใช้ (`ctx.locale`) หรือ `?lang=th|en` บนหน้า
- collection สาธารณะ `code:admin-crud:faqs` (คำถามที่เผยแพร่ เรียงตาม `sort`) ใช้ใน designer ได้ทันที
- เหตุการณ์ `admin-crud.saved`, `admin-crud.deleted` ให้ Site Code ฟัง (`site.on`)

**สิทธิ์:** `db` (ตารางและ migration ของตัวเอง), `admin-pages` (หน้าในแอดมิน)

**การตั้งค่า (unit `admin-crud`):**
- `resources` — JSON (อาร์เรย์ หรือ `{ "resources": [...] }`) ใช้แทน `config.json` โดยไม่ต้องอัปโหลดใหม่
  ตารางต้องเป็นของปลั๊กอินนี้เท่านั้น (ระบบเติม `p_admin_crud_` ให้เอง จึงอ้างตารางของ unit อื่นไม่ได้)
- `lang` — ใช้เฉพาะเมื่อ core ไม่มี `ctx.locale` (เวอร์ชันเก่า): `en` เพื่อแสดงภาษาอังกฤษ

**เพิ่มตารางของคุณ:** เพิ่ม migration ใหม่ใน `migrations/` แล้วเพิ่ม resource ใน `config.json` (หรือ setting) แล้วอัปโหลดเวอร์ชันใหม่

## English

A system plugin that renders a generic admin screen (list, search, paginate, create, edit, delete, **export CSV**)
for its **own** tables — or **another plugin's data through that plugin's exports** — configured in
`code/config.json` or the `resources` setting. Writes go through `site.action` (`admin-crud:save`,
`admin-crud:delete`), so access is managed on the Access screen; values are validated by column type on the server
and all output is HTML-escaped. Ships sample `contacts` / `faqs` tables, a public `code:admin-crud:faqs` collection
and the events `admin-crud.saved` / `admin-crud.deleted`.

- **Export CSV**: the list's button calls `anasaki.download({ r, q })` → the admin page's `download(ctx)` handler
  (same ticket and role check as the page): UTF-8 BOM, formula-injection guard, the current search, ≤ 10 000 rows.
- **Resources backed by another plugin** (`plugins.call`): `"source": { "plugin": "shop", "list": "listOrders",
  "get": "getOrder", "save": "saveOrder", "delete"?: "…" }` instead of `table` — names from that plugin's
  `code.exports`: `list({ q, page, perPage, orderBy })` → `{ items, total }` (or an array), `get(id)`,
  `save({ id, values })` → `{ id }` | `{ errors }`, `delete(id)` → count. A missing `save`/`delete` makes that
  operation unavailable (buttons hidden). The bundled `orders` resource uses the **shop** sample; without shop the
  tab shows a notice (`plugins.has`) and writes / exports answer with an error (`code: 'unavailable'`).
- **Language**: the signed-in user's admin language (`ctx.locale`), or `?lang=th|en` on the page.

**Scopes:** `db` (own tables + migrations), `admin-pages`. **Settings:** `resources` (JSON), `lang` (`en`, only
used by cores without `ctx.locale`).

It still cannot address other units' tables directly (by design) — data of other plugins goes through their exports.

## Site Code example / ตัวอย่าง

```ts
// react to changes made in the admin screen
site.on('admin-crud.saved', ({ resource, id, created }) => {
  if (resource === 'contacts' && created) log.info('new contact', id);
});
```

In the designer: a Collection with source `code:admin-crud:faqs` and `{{item.question}}` / `{{item.answer}}`.
