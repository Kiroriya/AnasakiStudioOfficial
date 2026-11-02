# Image optimizer (`image-optimizer`) — ย่อรูปอัตโนมัติ (example JS plugin, CMS 2.1.0)

## ภาษาไทย

เมื่อมีการอัปโหลดรูป JPEG/PNG (จากหลังบ้าน, นำเข้าจากลิงก์ หรือ `media.upload` ของโค้ด) ปลั๊กอินจะอ่านไฟล์
ย่อให้กว้างไม่เกิน **maxWidth** และบีบอัดใหม่ด้วยคุณภาพ **quality** แล้วแทนที่ไฟล์เดิม (URL เดิม) เฉพาะเมื่อไฟล์เล็กลง
ตั้ง **maxUploadMB** เพื่อปฏิเสธรูปที่ใหญ่เกินก่อนบันทึก (filter `media.beforeSave`)

**สิทธิ์:** `media`, `media-write`, `filters-core`

## English

On every JPEG/PNG upload (admin, URL import or `media.upload`) it reads the file, scales it down to **maxWidth**,
re-encodes it at **quality**, and replaces the file (same URL) only when the result is smaller. **maxUploadMB**
refuses oversized images before they are stored (`media.beforeSave`). WebP and GIF are left as they are.

Joints: `site.on('media.uploaded')`, `media.read`, `media.resize`, `media.replace`, `db.asSystem`,
`site.filter('media.beforeSave')`.
