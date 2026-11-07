# Payments (`payments`) — รับชำระเงินออนไลน์ (Stripe)

ปลั๊กอินทางการ: รับชำระด้วยบัตรเครดิต/เดบิต (และพร้อมเพย์สำหรับบัญชี Stripe ประเทศไทย) ผ่านหน้า Stripe Checkout
ใช้กับปลั๊กอิน **Shop** (วิธีชำระ "บัตร") และ **Donations** (ปุ่ม "บริจาคด้วยบัตร") อัตโนมัติเมื่อติดตั้ง ยืนยันการชำระด้วย webhook ที่ลงลายเซ็น
และตอนลูกค้ากลับมาที่เว็บ

ตั้งค่า: ใส่ Stripe secret key และ webhook signing secret · เพิ่ม `api.stripe.com` ใน Site code → ตั้งค่า → host ที่อนุญาต ·
สร้าง webhook ที่ Stripe ชี้มาที่ `https://<เว็บ>/payments/stripe/webhook` (checkout.session.completed, checkout.session.async_payment_succeeded)

## English

Stripe Checkout for cards (and PromptPay on Stripe Thailand). Exports `available()`, `checkout({ref, amount, description, email,
successPath, cancelPath})` → `{url}`, `status(ref)`; emits `payments.paid` {ref, amount, currency, provider, sessionId} once per
reference, from the signed webhook or the return page. The Shop (refs `eshop:<order code>`) and Donations (`donations:<code>`)
feature-detect it. Table `p_payments_payments`; admin page "Online payments".
