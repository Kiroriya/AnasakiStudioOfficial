# Members (`members`) — ระบบสมาชิก

## ภาษาไทย

ระบบสมาชิกสำหรับผู้เยี่ยมชมหน้าเว็บ (แยกจากผู้ใช้แอดมินของ CMS)

- ตาราง: `members`, `sessions`, `tokens` (→ `p_members_*`). รหัสผ่านเก็บแบบ bcrypt (`crypto.password`), session และ
  token เก็บเฉพาะ sha256 (ค่าจริงอยู่ใน cookie / ลิงก์ในอีเมลเท่านั้น)
- Route (JSON, POST ต้องเป็น `Content-Type: application/json` กัน CSRF จากฟอร์มข้ามเว็บ):
  `POST /members/register {email,password,name}`, `GET /members/verify?token=` (ลิงก์ในอีเมล),
  `POST /members/login {email,password}`, `POST /members/logout`, `POST /members/forgot {email}`,
  `POST /members/reset {token,password}`, `GET /members/me`
- Action สำหรับฟอร์มใน designer (มี CSRF ของระบบ): `members:register`, `members:login`, `members:logout`,
  `members:forgot`, `members:reset`, `members:profile` (กลุ่ม `member`)
- Cookie `pl_members_sid` (HttpOnly, SameSite=Lax, Secure บน https) อายุตาม `sessionDays`
- **`site.visitor`**: ระบุตัวสมาชิกจาก cookie → `{ id, name, email, verified, groups }` กลุ่ม `member` และ `verified`
  (+ กลุ่มเพิ่มจากคอลัมน์ `groups`) ใช้ได้ทั้งเว็บ: `access: ['group:verified']`, `{{visitor.is.member}}`
- อีเมลยืนยัน (48 ชม.) และรีเซ็ตรหัสผ่าน (1 ชม.) ไทย+อังกฤษ ปรับแต่งได้ด้วย filter `members.mail`
- collection `code:members:me` (ข้อมูลของฉัน), หน้าแอดมิน **สมาชิก** (ค้นหา, ระงับ/เปิดใช้ — ระงับแล้ว session หลุดทันที,
  ปุ่ม **ส่งออก CSV** ดาวน์โหลดรายชื่อตามคำค้นผ่าน `download` ของ `site.adminPage` — ตรวจ ticket และสิทธิ์เหมือนหน้าแอดมิน
  ไม่มีรหัสผ่าน/โทเคนในไฟล์) หน้าแอดมินแสดงภาษาตามภาษาแอดมินของผู้ใช้ (`ctx.locale`) หรือ `?lang=th|en`
- กันเดารหัส: ผิด 10 ครั้ง / 15 นาที ต่ออีเมล, ใช้เวลาเท่ากันเมื่อไม่มีอีเมลนี้, ลืมรหัสตอบเหมือนกันทุกกรณี
- เหตุการณ์ `members.registered`, `members.verified`, `members.login`; export `memberByEmail(email)`

**ข้อควรรู้: มี `site.visitor` ได้เพียงตัวเดียวทั้งเว็บ** ถ้า Site Code หรือปลั๊กอินอื่นเรียก `site.visitor` อยู่แล้ว ตัวที่
โหลดทีหลังจะเปิดใช้งาน/เผยแพร่ไม่ผ่าน (ข้อความแจ้งชื่อ unit ที่ชนกัน) — ต้องลบตัวใดตัวหนึ่งออก

**สิทธิ์:** `db` (ตารางของตัวเอง), `routes` (`/members`), `visitor` (ระบุตัวผู้เยี่ยมชม), `mail` (อีเมลยืนยัน/รีเซ็ต),
`admin-pages` (หน้าจัดการสมาชิก)

**การตั้งค่า (unit `members`):**
| key | ค่าเริ่มต้น | |
|---|---|---|
| `requireVerification` | `true` | `false` = ล็อกอินได้โดยไม่ต้องยืนยันอีเมล |
| `allowedDomains` | (ทุกโดเมน) | เช่น `corp.co.th, example.com` |
| `sessionDays` | `30` | อายุ session (1-365) |
| `siteUrl` | จาก Host ของคำขอ | ใช้สร้างลิงก์ในอีเมล เช่น `https://www.example.com` |
| `afterVerifyUrl` | (หน้า "ยืนยันแล้ว") | path ที่ redirect ไปหลังยืนยัน เช่น `/login` |
| `afterLoginUrl` | — | path ที่ action `login` redirect ไป |
| `resetPath` | `/reset-password` | หน้าของคุณที่มีฟอร์ม `members:reset` (รับ `?token=`) |
| `lang` | `th` | `en` = ข้อความถึงผู้เยี่ยมชมเป็นภาษาอังกฤษ (หน้าแอดมินใช้ภาษาของผู้ดูแลเอง) |

ต้องตั้งค่า SMTP ของเว็บ (หรือ `SITE_CODE_MAIL_SINK=log` ตอนพัฒนา)

## English

Visitor accounts for the public site: own tables, JSON routes under `/members` (JSON-only POSTs against CSRF),
the same flows as `site.action`s for designer forms, bcrypt passwords, hashed session / one-time tokens,
the **one** `site.visitor` resolver of the site (groups `member`, `verified`), verification and reset mails
(filter `members.mail`), a `code:members:me` collection, an admin screen to search and disable members — with an
**Export CSV** button (the admin page's `download` handler: same ticket and role check, no password hashes or tokens;
the screen follows the signed-in user's admin language, `ctx.locale`, or `?lang=th|en`) — events and the
`memberByEmail` export. The `lang` setting only changes visitor-facing messages.

**Only one `site.visitor` per site:** if Site Code or another plugin already registers one, the later unit fails
to activate / publish with an error naming the conflicting unit.

**Scopes:** `db`, `routes`, `visitor`, `mail`, `admin-pages`. **Settings:** see the table above.

## Site Code example / ตัวอย่าง

```ts
// a members-only page and API (group from the members plugin's visitor resolver)
site.page('/account', { access: ['group:member'], load: ({ request }) => ({ hello: 'สวัสดี' }) });
site.route('GET /api/my-orders', { access: ['group:verified'] }, ({ visitor }) =>
  db.table('orders').where('member_id', visitor.id).latest().get());

site.on('members.registered', ({ email }) => log.info('welcome', email));
```

Designer: a form with action `members:login` (inputs `email`, `password`), `showWhen: visitor.is.member` for
"signed in" content, and a Collection `code:members:me` for the profile.
