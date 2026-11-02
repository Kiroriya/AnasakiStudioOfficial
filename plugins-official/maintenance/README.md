# Maintenance mode (`maintenance`) — official plugin 2.0.0

## ภาษาไทย

ปลั๊กอินทางการสำหรับ "โหมดปิดปรับปรุง" — แทนปลั๊กอินแบบ built-in เดิม (ทำงานเหมือนเดิม)

- เมื่อเปิด ผู้เยี่ยมชมจะเห็นหน้า "ปิดปรับปรุงชั่วคราว" (HTTP 503 + `Retry-After: 3600`, ไม่ถูก cache, `noindex`)
  พร้อมหัวข้อ ข้อความที่ตั้งไว้ ชื่อเว็บ และภาษาของเว็บ (`<html lang>`)
- ระหว่างที่เปิด หลังบ้านแสดงแถบแจ้งเตือน "โหมดปิดปรับปรุงเปิดอยู่" ให้ผู้ดูแลระบบ พร้อมปุ่ม "ดูเว็บจริง (ลิงก์ bypass)"
- หลังบ้าน, `/api/`, `/uploads/`, `/plugin-assets/` ใช้งานได้ตามปกติ
- **ลิงก์ bypass** `/?bypass=รหัส` — ตั้ง cookie (HttpOnly, 7 วัน) ให้ดูเว็บจริงได้ แล้วพากลับไปที่ลิงก์เดิมโดยตัด
  `bypass` ออก cookie ผูกกับรหัส: เปลี่ยนรหัสแล้ว cookie เดิมใช้ไม่ได้ทันที หน้าจริงที่ผู้ถือ cookie เห็นจะไม่ถูก cache
  (`Cache-Control: no-store`)
- ถ้าไม่ได้ตั้งรหัส ระบบจะสร้างรหัสสุ่มให้เมื่อเปิดโหมดปิดปรับปรุง ดู/คัดลอก/สร้างใหม่ได้ที่หน้า **โหมดปิดปรับปรุง** ในหลังบ้าน
- หน้า **โหมดปิดปรับปรุง** มีฟอร์มตั้งค่า (เปิด/ปิด หัวข้อ ข้อความ รหัส bypass) — ค่าเดียวกับหน้า ปลั๊กอิน → ตั้งค่า

**การตั้งค่า** (ปลั๊กอิน → โหมดปิดปรับปรุง → ตั้งค่า): `enabled`, `title`, `message`, `bypassKey` — ชื่อเดียวกับปลั๊กอินเดิม

**สิทธิ์ (scopes):** `db` (ตาราง `p_maintenance_state` เก็บรหัสที่สร้างให้), `middleware`, `admin-pages`

**ย้ายจากปลั๊กอินเดิม:** ปลั๊กอินเดิมไม่มีตาราง ค่าตั้งค่าถูกย้ายโดยระบบ (ชื่อเดียวกัน) — cookie bypass เดิม
(`anasaki_bypass`) ใช้ไม่ได้ ผู้ทดสอบต้องเปิดลิงก์ bypass อีกครั้ง

**สำหรับนักพัฒนา:** `plugins.call('maintenance', 'status')` → `{enabled, bypassKey, bypassPath}`

**สิทธิ์ (CMS 2.0.6+):** `maintenance.manage` — หน้าโหมดปิดปรับปรุงและแถบแจ้งเตือน (ค่าเริ่มต้น: ผู้ดูแลระบบ) — ปรับได้ที่ ผู้ใช้ → ตำแหน่งและสิทธิ์ · CMS รุ่นก่อนหน้าใช้รายการตำแหน่งแบบเดิม

## English

The official maintenance-mode plugin — replaces the former built-in plugin with the same behaviour.

- While enabled, visitors get a standalone "back soon" page (HTTP 503, `Retry-After: 3600`, `no-store`, `noindex`) with
  the configured title and message. The admin, `/api/`, `/uploads/` and `/plugin-assets/` keep working.
- **Bypass link** `/?bypass=KEY` sets an HttpOnly cookie (7 days) and redirects to the same URL without `bypass`.
  The cookie is bound to the key: changing the key ends old cookies.
- With no key set, a random key is generated when maintenance mode is on; see, copy or regenerate it on the
  **Maintenance** admin page, which also has the settings form (on/off, title, message, bypass key).
- The page shows the site name and uses the site's language (`<html lang>`); real pages served to bypass-cookie
  holders get `Cache-Control: no-store`; while the mode is on the admin shows administrators a "Maintenance mode is
  on" notice with a "View the real site (bypass link)" button.

**Settings** (Plugins → Maintenance mode → Settings): `enabled`, `title`, `message`, `bypassKey` (same keys as the
built-in plugin).

**Scopes:** `db` (table `p_maintenance_state` for the generated key), `middleware`, `admin-pages`.

**Migration:** the built-in plugin had no tables; settings are mapped by core. Old bypass cookies (`anasaki_bypass`)
no longer apply — testers open the bypass link again.

**Developers:** `plugins.call('maintenance', 'status')` → `{enabled, bypassKey, bypassPath}`.

**Permissions (CMS 2.0.6+):** `maintenance.manage` — the Maintenance page and the admin notice (default: administrators) — change them in Users → Roles · older CMS versions keep the former role list
