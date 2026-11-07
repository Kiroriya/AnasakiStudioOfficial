# Events (`events`) — กิจกรรมและการลงทะเบียน

ปลั๊กอินทางการ: ใช้กับประเภทเนื้อหาของคุณ (ค่าเริ่มต้น `event` ของชุดเริ่มต้นองค์กร ช่อง `when` `place` `seats`) —
ฟอร์มลงทะเบียนใต้รายละเอียดกิจกรรม จำกัดที่นั่ง ปิดรับเมื่อเต็มหรือกิจกรรมผ่านไปแล้ว, หน้าการลงทะเบียนของผู้เข้าร่วม (ยกเลิกได้),
ปฏิทินรายเดือนที่ `/calendar`, เพิ่มลง Google Calendar หรือไฟล์ .ics, อีเมลยืนยัน และหน้า **ผู้ลงทะเบียนกิจกรรม** (เช็กอิน ยกเลิก CSV)

## English

Works on a content type of yours (settings: type, start / end / place / seats fields). Sign-up form under each event (seat limit,
closed when full or past), the attendee's page with cancel, a month calendar at `/calendar` (a list on phones), Google Calendar
links and `/calendar/<id>.ics`, mails, and the admin page with check-in, cancel and CSV. Events `events.registered`,
`events.cancelled`; table `p_events_registrations`.
