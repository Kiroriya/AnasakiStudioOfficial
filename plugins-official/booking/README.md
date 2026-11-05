# Booking (`booking`) — ระบบจองคิว

ปลั๊กอินทางการ: บริการ (ระยะเวลา ราคา รับพร้อมกันกี่คน), เวลาเปิดรายสัปดาห์และวันหยุดพิเศษ, หน้า `/booking` (บริการ → วัน → เวลาว่าง →
ข้อมูล) ที่ใช้ได้โดยไม่ต้องมี JavaScript, ไม่มีจองซ้อนเกินจำนวนที่รับได้, หน้า **การจองของคุณ** พร้อมปุ่มยกเลิก, อีเมลยืนยันถึงลูกค้าและแจ้งร้าน,
บล็อก **ปุ่มจองคิว** และหน้า **การจอง** ในหลังบ้าน (รายการจอง บริการ เวลาเปิด)

คู่มือ: [/docs/store/plugin-booking](https://anasaki.live/docs/store/plugin-booking)

## English

Services, weekly hours and closed days, a no-JavaScript booking flow at `/booking`, capacity-checked slots (no double
booking), the customer's page with a cancel button, confirmation mails, a booking button block and the Bookings admin
page. Times use the site's time zone as a fixed offset (no daylight saving). Events `booking.created`, `booking.status`;
tables `p_booking_*`; capability `booking.manage`.
