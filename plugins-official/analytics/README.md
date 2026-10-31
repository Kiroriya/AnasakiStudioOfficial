# Analytics (`analytics`) — สถิติผู้เข้าชม

## ภาษาไทย

นับยอดเข้าชมหน้าเว็บฝั่งเซิร์ฟเวอร์ **ไม่ใช้คุกกี้ ไม่มีสคริปต์ของบุคคลที่สาม ไม่เก็บ IP** — ปลั๊กอินทางการเวอร์ชัน 2
ของปลั๊กอินในตัว "สถิติผู้เข้าชม" (ข้อมูลเดิมย้ายมาอัตโนมัติเมื่ออัปเกรด)

- นับหลังส่งหน้าเว็บให้ผู้เข้าชมแล้ว (`site.afterResponse`) จึงไม่ทำให้เว็บช้า เฉพาะคำขอ `GET` ที่ได้หน้า HTML สถานะ 200
- ไม่นับ: บอท/เครื่องมือ (Googlebot, curl, HeadlessChrome, …, user agent สั้นกว่า 12 ตัวอักษร), การโหลดล่วงหน้า
  (`Purpose` / `Sec-Purpose` / `X-Moz: prefetch`), `/api/`, `/uploads/`, `/admin`, `/plugin-assets/`, `/download/`,
  `/forms/` และไฟล์ที่มีนามสกุลอื่นนอกจาก `.html`
- นับ path แบบแยกตัวพิมพ์เล็ก/ใหญ่ (`/About` กับ `/about` เป็นคนละหน้า เหมือนเดิม)
- ผู้เข้าชม = hash รายวันของ `salt|IP|user agent` (16 ตัวอักษร) — salt สุ่มใหม่ทุกวันและลบของเก่าทิ้ง จึงย้อนหา IP ไม่ได้
  และเชื่อมโยงข้ามวันไม่ได้
- แหล่งที่มา: โดเมนของ referrer ภายนอก (ตัด `www.`) — ลิงก์ภายในเว็บไม่นับ
- หน้าแอดมิน **สถิติผู้เข้าชม** (ผู้ดูแลระบบและบรรณาธิการ): ยอดวันนี้, ยอด 7/30/90 วัน, กราฟรายวัน (ดูเป็นตารางได้),
  หน้ายอดนิยม 10 อันดับ (ชื่อหน้าปัจจุบัน), แหล่งที่มา 10 อันดับ
- วิดเจ็ต **ผู้เข้าชม 7 วันล่าสุด** บนแดชบอร์ดหลังบ้าน (ผู้ดูแลระบบและบรรณาธิการ)
- export `summary(days)` → JSON เดียวกับ API เดิม `GET /api/v1/analytics/summary`
  (`plugins.call('analytics', 'summary', 30)`)

**สิทธิ์ (scopes):** `db` (ตารางของตัวเอง `p_analytics_*`), `content` (ชื่อหน้าปัจจุบัน), `middleware`
(`site.afterResponse`), `admin-pages` (หน้าสถิติและวิดเจ็ตแดชบอร์ด)

**ย้ายจากปลั๊กอินในตัว:** ตาราง `analytics_daily`, `analytics_visitors`, `analytics_page_visitors`,
`analytics_referrers` ถูกนำเข้าด้วย `legacyImport` (ทำซ้ำได้ ไม่นับซ้ำ) ตารางเดิมไม่ถูกลบจนกว่าผู้ดูแลจะกด
"ล้างข้อมูลเก่า" ข้อแตกต่างเล็กน้อยจากเดิมดู `../GAPS.md` (ส่วน port B)

## English

Server-side, privacy-friendly page-view counter — **no cookies, no third-party script, raw IPs are never stored**.
Version 2 of the built-in "Analytics" plugin as an official JS plugin (old data is migrated on upgrade).

- Counted after the response was sent (`site.afterResponse`): `GET` requests answered with a 200 HTML page.
- Skipped: bots and tools, prefetch / prerender requests, `/api/`, `/uploads/`, `/admin`, `/plugin-assets/`,
  `/download/`, `/forms/`, and files other than `.html`.
- Visitors are a daily hash of `salt|IP|user agent` (16 hex); the salt rotates daily and old salts are deleted.
- Referrers: external referrer host without `www.`; internal links are not sources. Paths are case-sensitive
  (`/About` and `/about` are counted apart, like the built-in tables).
- Dashboard widget **Visitors, last 7 days** (administrators and editors).
- Admin page **Analytics** (administrators and editors): today, 7 / 30 / 90-day totals, a daily chart (or table), the
  top 10 pages (current titles) and referrers.
- Export `summary(days)` returns the JSON of the old `GET /api/v1/analytics/summary`.

**Scopes:** `db`, `content`, `middleware`, `admin-pages`. **Legacy import:** the four `analytics_*` tables, idempotent
(counters keep the larger value). Small differences: `../GAPS.md` (port B).
