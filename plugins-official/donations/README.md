# Donations (`donations`) — รับบริจาค

ปลั๊กอินทางการสำหรับมูลนิธิและโครงการ: หน้า `/donate` เลือกยอดหรือใส่เอง ไม่ประสงค์ออกนามได้ ขอใบอนุโมทนาบัตรพร้อมเลขผู้เสียภาษี →
หน้า `/donate/d/<รหัส>` มี QR พร้อมเพย์ตามยอดหรือบัญชีโอน และแนบสลิป → หลังบ้าน **เงินบริจาค** กดยืนยัน ระบบออกเลขที่ใบอนุโมทนาบัตร
(D<ปี>-0001) และส่งอีเมลถึงผู้บริจาค ใบอนุโมทนาบัตรพิมพ์หรือบันทึกเป็น PDF ได้ บล็อก **ร่วมบริจาค** แสดงยอดแนะนำและแถบเป้าหมาย ส่งออก CSV ได้

## English

Any-amount gifts by PromptPay QR (or bank transfer) with slip upload, anonymous gifts, a receipt for tax deduction (tax ID and
address), the Donations admin page (confirm, cancel, slip, CSV), numbered printable receipts, mails, the `donations/donate` block
(suggested amounts, goal bar) and the events `donations.received`, `donations.confirmed`. Table `p_donations_gifts`.
