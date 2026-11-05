# Members (`members`) — ระบบสมาชิก

ปลั๊กอินทางการ: บัญชีผู้เยี่ยมชมเว็บ (แยกจากผู้ใช้หลังบ้าน) — สมัครพร้อมอีเมลยืนยัน, เข้าสู่ระบบด้วยคุกกี้, ลืมรหัสผ่าน,
หน้าพร้อมใช้ `/members/login` `/members/register` `/members/forgot` `/members/reset` `/members/account`, บล็อก **เมนูสมาชิก**,
หน้า **สมาชิก** ในหลังบ้าน (ค้นหา ระงับ/เปิดใช้ ส่งออก CSV) และ `site.visitor` ของเว็บ (กลุ่ม `member`, `verified`)

ซ่อน/แสดงส่วนของหน้าเฉพาะสมาชิกได้ในหน้าออกแบบ: เลือกเลเยอร์ → เงื่อนไข **แสดงเมื่อ** `visitor.is.member`
(ตรวจฝั่งเซิร์ฟเวอร์ ส่วนที่ซ่อนไม่ถูกส่งไปที่เบราว์เซอร์)

ความปลอดภัย: รหัสผ่าน bcrypt, เก็บแค่ hash ของ session/โทเคน, POST เป็น JSON เท่านั้น (กัน CSRF), กันเดารหัส 10 ครั้ง/15 นาที

## English

Visitor accounts: sign-up with email confirmation, cookie sign-in, password reset, ready-made pages under `/members`,
a member menu block, a members admin screen (search, disable, CSV export) and the site's `site.visitor` (groups
`member`, `verified`) — hide or show designer layers with the condition `visitor.is.member`. Only one `site.visitor`
may exist on a site, so Members cannot run next to other code that defines one.
Events `members.registered`, `members.verified`, `members.login`; filter `members.mail`; export `memberByEmail(email)`.
