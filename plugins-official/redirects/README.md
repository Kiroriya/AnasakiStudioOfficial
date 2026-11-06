# Redirects (`redirects`) — official plugin 2.1.0

## ภาษาไทย

ปลั๊กอินทางการสำหรับเปลี่ยนเส้นทางลิงก์ — แทนปลั๊กอิน Redirects แบบ built-in เดิม (ทำงานเหมือนเดิม)

- **กฎ redirect** 301 (ย้ายถาวร), 302 (ชั่วคราว), 307, 308 จากเส้นทางเดิม (`/old-page`) ไปยังเส้นทางใหม่หรือลิงก์ภายนอก
  - ตรงกับเส้นทางทั้งหมดแบบตัวพิมพ์ใหญ่-เล็กตรงกัน ไม่สนใจ `/` ท้ายลิงก์ และส่งต่อ `?query` ไปยังปลายทาง
    (ถ้าปลายทางยังไม่มี query) ใช้กับคำขอ GET/HEAD เท่านั้น
  - ตรวจกฎที่ทำให้วนซ้ำไม่รู้จบ (A → B → A) ก่อนบันทึก, ห้าม redirect `/`, `/api/`, `/uploads/`, `/plugin-assets/`, `/ext-ui/`
- **ลิงก์ที่หาไม่เจอ (404)** — เก็บ 200 เส้นทางที่มีคนเข้าบ่อยที่สุด กด "แก้" เพื่อสร้าง redirect, "ล้างรายการ" ได้ (ตั้งแต่ 2.0.2 ไม่นับเส้นทางของระบบข้างต้น — เช่น API ที่ตอบ 404 หรือไฟล์อัปโหลดที่ถูกลบ ไม่ใช่ลิงก์เสียของเว็บ) — 2.1.0: พาธที่บอทใช้สแกนหาไฟล์ลับ (`/.env`, `/.git/…`, `*.php`, `/wp-…`) ก็ไม่ถูกเก็บ
- **2.1.0:** ข้อความเตือนบอกตรงจุด (หน้าแรก `/` และเส้นทางของระบบ) และแสดงใต้ฟอร์ม ล้างเองเมื่อกดเพิ่มครั้งถัดไป
- นับจำนวนครั้งที่ใช้แต่ละกฎ — บันทึกหลังส่งหน้าเว็บแล้ว (`site.afterResponse`) ผู้เยี่ยมชมไม่ต้องรอฐานข้อมูล
- กฎที่เพิ่งบันทึกใช้ได้ทันที และ redirect ไม่หลุดเป็น 404 เวลาเครื่องช้า (2.0.2: เก็บกฎไว้ใน cache 5 นาทีและสร้างใหม่ทันทีที่แก้ — เดิมหมดอายุทุก 5 วินาที คำขอที่อ่านฐานข้อมูลใหม่อาจเกินเวลา 20 ms ของ middleware แล้วถูกข้าม)

**หน้าจัดการ:** หลังบ้าน → เปลี่ยนเส้นทาง (ผู้ดูแลระบบ)

**สิทธิ์ (scopes):** `db` (ตาราง `p_redirects_rules`, `p_redirects_notfound`), `middleware` (redirect ก่อนจัดเส้นทาง
และบันทึก 404), `admin-pages`

**ย้ายข้อมูลจากปลั๊กอินเดิม:** ตาราง `redirects_rules` และ `redirects_404` ถูกย้ายอัตโนมัติผ่าน export `legacyImport`
(รันซ้ำได้: กฎจับคู่ด้วยเส้นทางต้นทาง, 404 จับคู่ด้วยเส้นทาง — จำนวนครั้งไม่ถูกนับซ้ำ) ตารางเดิมไม่ถูกลบอัตโนมัติ

**สิทธิ์ (CMS 2.0.6+):** `redirects.manage` — หน้าเปลี่ยนเส้นทางและรายการ 404 (ค่าเริ่มต้น: ผู้ดูแลระบบ) — ปรับได้ที่ ผู้ใช้ → ตำแหน่งและสิทธิ์ · CMS รุ่นก่อนหน้าใช้รายการตำแหน่งแบบเดิม

## English

The official redirects plugin — replaces the former built-in Redirects plugin with the same behaviour.

- **Rules** 301 / 302 / 307 / 308 from an old path (`/old-page`) to a new path or an external URL: exact,
  case-sensitive path match; a trailing slash is ignored; the query string is carried over when the target has none;
  GET/HEAD only. Loops (A → B → A) are refused when saving; `/`, `/api/`, `/uploads/`, `/plugin-assets/`, `/ext-ui/`
  cannot be redirected.
- **404 log** of the 200 most hit missing paths, with "Fix" (prefills a rule) and "Clear" — since 2.0.2 without the
  system paths above (an API answering 404 or a deleted upload is not a broken link of the site).
- Rule hit counters and the 404 log are written after the response (`site.afterResponse`), never delaying visitors.
- A saved rule works at once and a redirect does not fall through to a 404 on a slow machine (2.0.2: the rules stay
  cached for five minutes and are rebuilt on every edit — they used to expire every five seconds, and the database read
  could exceed the 20 ms middleware budget).

**Admin page:** Redirects (administrators).

**Scopes:** `db` (tables `p_redirects_rules`, `p_redirects_notfound`), `middleware`, `admin-pages`.

**Migration from the built-in plugin:** `redirects_rules` and `redirects_404` rows are imported automatically through
the `legacyImport` export (idempotent: rules by source path, 404s by path without doubling counts). The old tables are
never deleted automatically.

**Permissions (CMS 2.0.6+):** `redirects.manage` — the Redirects page and the 404 log (default: administrators) — change them in Users → Roles · older CMS versions keep the former role list
