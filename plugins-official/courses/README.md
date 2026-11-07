# Courses (`courses`) — คอร์สออนไลน์

ปลั๊กอินทางการ (ใช้คู่กับ **Members**): คอร์ส `/course/<slug>` และบทเรียน `/lesson/<slug>` (ผูกกับคอร์ส ลำดับบท วิดีโอ YouTube/Vimeo
ความยาว ดูตัวอย่างได้) — คอร์สฟรี สำหรับสมาชิก หรือขาย (ชำระด้วยบัตรผ่านปลั๊กอิน **Payments** หรือโอนแล้วเจ้าของกดเปิดสิทธิ์) บทเรียนที่ยังไม่มีสิทธิ์
แสดงกล่องเข้าสู่ระบบ/ซื้อคอร์สแทนเนื้อหา สมาชิกกด "เรียนจบบทนี้" เห็นความคืบหน้าในหน้าคอร์ส และพิมพ์ใบประกาศเมื่อเรียนครบ
หน้า **ผู้เรียน** ในหลังบ้านแสดงผู้ลงทะเบียน ความคืบหน้า เปิด/ปิดสิทธิ์ — ใส่ "คำอธิบายย่อ" ให้บทเรียนเสมอ (รายการและการค้นหาแสดงคำอธิบายย่อ ไม่ใช่เนื้อหาที่ล็อกไว้)

## English

Course and lesson content types, free / members / paid access (Payments plugin or granted by hand), locked lessons (the
post.content filter with the CMS 2.5.0 visitor), progress, "next lesson", a printable certificate, the Students admin page,
events `courses.enrolled` and `courses.completed`. Tables `p_courses_enrollments`, `p_courses_progress`.
