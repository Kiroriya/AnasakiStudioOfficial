# Forms (`forms`) — แบบฟอร์ม

## ภาษาไทย

สร้างฟอร์ม (ติดต่อเรา สมัครรับข่าว ขอใบเสนอราคา) วางบนหน้าเว็บด้วยดีไซเนอร์ เก็บข้อความที่ส่งเข้ามา ส่งออก CSV
แจ้งเตือนทางอีเมล — ปลั๊กอินทางการเวอร์ชัน 2 ของปลั๊กอินในตัว "ฟอร์มติดต่อ" (ฟอร์มและข้อความเดิมย้ายมาอัตโนมัติ
โดยคงหมายเลขเดิม หน้าที่วางบล็อกฟอร์มไว้แล้วจึงใช้งานต่อได้ทันที)

- **หน้าแอดมิน แบบฟอร์ม** (ผู้ดูแลระบบและบรรณาธิการ): สร้าง/แก้ไขฟอร์ม — ชื่อ, ข้อความปุ่ม, ช่องกรอก 8 ชนิด
  (ข้อความสั้น อีเมล เบอร์โทร ข้อความยาว ตัวเลือก ช่องติ๊ก ตัวเลข วันที่) จำเป็น/ไม่จำเป็น เรียงลำดับได้, ข้อความขอบคุณ,
  หน้าที่จะพาไปหลังส่ง (เฉพาะหน้าในเว็บนี้), กล่องข้อความ (อ่านแล้ว/ยังไม่อ่าน, ลบ, แบ่งหน้า) และปุ่ม **CSV**
  (UTF-8 BOM, กันสูตร Excel)
- **บล็อกดีไซเนอร์** "ฟอร์ม": แผงเพิ่ม → บล็อกปลั๊กอิน → ฟอร์ม แล้ว **เลือกฟอร์มจากรายการ** — ฟอร์มถูกสร้างฝั่ง
  เซิร์ฟเวอร์ (ใช้ได้แม้ปิด JavaScript และ Google อ่านได้) สคริปต์เล็ก ๆ ทำให้ส่งแบบไม่โหลดหน้าใหม่; หน้าฟอร์มเดี่ยว
  `/forms/<id>` แสดงในหัว/ท้ายของเว็บ
- **การส่ง** `POST /forms/<id>/submit` รับ urlencoded / multipart / JSON, ตอบ JSON (`{ok, message, redirect?}`) หรือ
  redirect กลับหน้าเดิมพร้อม `?sent=<id>`; ตรวจข้อมูลตามชนิดช่อง (อีเมล ตัวเลข วันที่ ตัวเลือก ความยาว ≤ 5000)
- **กันสแปม:** ช่องหลอก `_hp` (บอทได้คำตอบว่าสำเร็จแต่ไม่บันทึก) และจำกัด 5 ครั้ง/นาที/IP (นับเฉพาะการส่งจริง)
- ส่งไม่สำเร็จแบบไม่ใช้ JavaScript → หน้าแจ้งข้อผิดพลาดในหัว/ท้ายของเว็บ พร้อมลิงก์กลับ
- **หลังส่ง:** เหตุการณ์ `forms.submitted` `{formId, formName, submissionId, data, submittedAt}` ซึ่งส่งในชื่อเดิม
  `form.submitted` ด้วย (Site Code / ปลั๊กอินที่ฟัง `site.on('form.submitted', …)` ใช้ต่อได้), อีเมลแจ้งเตือน
  (ถึง `notifyEmail` หรืออีเมลผู้ดูแลเว็บ ตอบกลับถึงผู้ส่งได้ทันที), **webhook รายฟอร์ม** (POST
  `{event: "form.submitted", form, submissionId, data, submittedAt}` ไปยัง URL ของฟอร์ม) และ webhook ทุกฟอร์มจากการตั้งค่า

**การตั้งค่า:** `notifyEmail` (อีเมลรับแจ้งเตือน), `siteTitle` (ชื่อเว็บในอีเมล), `webhookUrl` (POST JSON ทุกครั้งที่มี
ข้อความใหม่) ที่ ปลั๊กอิน → แบบฟอร์ม → ตั้งค่า

**สิทธิ์ (scopes):** `db`, `routes` (`/forms`), `mail`, `admin-pages`, `webhooks` (webhook รายฟอร์ม);
permissions `settings`, `webhooks`

**ย้ายจากปลั๊กอินในตัว:** `forms_forms` และ `forms_submissions` นำเข้าด้วย `legacyImport` (คงหมายเลขเดิม ทำซ้ำได้)
ข้อแตกต่างเล็กน้อยดู `../GAPS.md` (port B)

**สิทธิ์ (CMS 2.0.6+):** `forms.manage` — สร้าง/แก้/ลบฟอร์ม · `forms.view_submissions` — ดูข้อความและส่งออก CSV (ค่าเริ่มต้น: ผู้ดูแลระบบ, บรรณาธิการ) — ปรับได้ที่ ผู้ใช้ → ตำแหน่งและสิทธิ์ · CMS รุ่นก่อนหน้าใช้รายการตำแหน่งแบบเดิม

## English

Form builder: forms are designed on the **Forms** admin page (administrators and editors), placed with the designer
block "Form" (choose the form from a list; rendered on the server, works without JavaScript), and submissions land
in an inbox with CSV export. Version 2 of the built-in
plugin; forms and submissions are migrated with their ids, so pages that already use the block keep working.

- Submissions: `POST /forms/<id>/submit` (urlencoded, multipart or JSON) → JSON or a redirect back with `?sent=<id>`;
  validation per field type; honeypot `_hp`; 5 submissions per minute per IP (real submissions only); a failed
  classic post shows an error page inside the site's header and footer.
- After a submission: event `forms.submitted` (also delivered as the former core event `form.submitted`), a
  notification mail (to `notifyEmail` or the site's admin email; reply-to = the first email field), the form's own
  webhook (`{event: "form.submitted", form, submissionId, data, submittedAt}`) and the site-wide webhook setting.
- Settings (Plugins → Forms → Settings): `notifyEmail`, `siteTitle`, `webhookUrl`. Scopes: `db`, `routes`, `mail`,
  `admin-pages`, `webhooks`.
- Legacy import: `forms_forms`, `forms_submissions` (ids kept, idempotent). Small differences: `../GAPS.md` (port B).

**Permissions (CMS 2.0.6+):** `forms.manage` — build / edit / delete forms · `forms.view_submissions` — the inbox and CSV export (default: administrators, editors) — change them in Users → Roles · older CMS versions keep the former role list
