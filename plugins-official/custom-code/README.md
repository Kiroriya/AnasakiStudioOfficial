# Custom code (`custom-code`) — โค้ดเพิ่มเติม

## ภาษาไทย

แทรกโค้ด HTML / JavaScript ใน `<head>` และก่อน `</body>` ของทุกหน้าเว็บ เช่น Google Analytics, Facebook Pixel,
แชท — ปลั๊กอินทางการเวอร์ชัน 2 ของปลั๊กอินในตัว (ค่าตั้งค่าเดิมย้ายมาอัตโนมัติ: `headCode`, `bodyCode`)

- ตั้งค่าที่ **ปลั๊กอิน → โค้ดเพิ่มเติม → ตั้งค่า** (ช่องละไม่เกิน 100,000 ตัวอักษร เท่ากับปลั๊กอินเดิม)
- โค้ดถูกแทรกตามที่เขียนทุกตัวอักษร (scope `head-scripts`) **ใส่เฉพาะโค้ดที่เชื่อถือได้** — เฉพาะผู้ดูแลระบบแก้ได้
- แสดงทั้งหน้าที่ออกแบบและหน้า 404/หน้าสำรองของระบบ

## English

Raw HTML / JavaScript in `<head>` and before `</body>` of every public page (analytics tags, pixels, chat widgets) —
version 2 of the built-in plugin; settings `headCode` / `bodyCode` migrate with the same keys. Edit them on
**Plugins → Custom code → Settings** (up to 100,000 characters each, like the built-in plugin). The code is inserted
verbatim (scope `head-scripts`): trusted code only.
