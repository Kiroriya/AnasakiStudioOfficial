# PromptPay QR (`promptpay`) — คิวอาร์พร้อมเพย์

## ภาษาไทย

สร้างข้อความ PromptPay (Thai QR Payment ตามมาตรฐาน EMVCo / ธปท.) และรูป QR แบบ SVG โดยไม่ใช้ไลบรารีภายนอก

- `payload({ id, amount? })` — `id` เป็นเบอร์มือถือ (`0812345678`, `+66…` → tag 29 sub 01 `0066`+9 หลัก),
  เลขบัตรประชาชน/เลขผู้เสียภาษี 13 หลัก (sub 02) หรือ e-Wallet 15 หลัก (sub 03). มี `amount` → QR แบบระบุยอด
  (tag 01 = `12`, tag 54 ทศนิยม 2 ตำแหน่ง) ไม่มี → ผู้จ่ายกรอกยอดเอง (`11`). จ่ายบิล (tag 30):
  `payload({ billerId: '0105555123456' + '01', ref1: 'INV001', ref2?, amount })`. ปิดท้ายด้วย CRC16-CCITT-FALSE (tag 63)
- `qrSvg(payload, { size, margin, color, background })` — QR code (byte mode, ECC M, version 1-40, เลือก mask ตามกฎ penalty)
  เป็น SVG หนึ่ง `<path>`; `qrMatrix(text)` ให้ตาราง module ไปวาดเองได้
- `crc16`, `verifyPayload`, `target`, `formatAmount`
- Route ของปลั๊กอิน: `GET /promptpay/qr.svg?amount=150&size=256` (รูป SVG) และ `GET /promptpay/payload?amount=150` (JSON)
  ใช้ PromptPay ID จากการตั้งค่า `promptpayId`
- export `qrFor(amount?, { size })` → `{ payload, svg, url }` หรือ `null` (ยังไม่ตั้ง `promptpayId`) สำหรับปลั๊กอินที่ใช้
  promptpay แบบไม่บังคับ (`optionalRequires` + `plugins.has('promptpay')` / `plugins.call('promptpay', 'qrFor', 150)`)
  ทำงานใน unit ของ promptpay จึงใช้การตั้งค่าของร้านได้ — ดูตัวอย่างในปลั๊กอิน `shop`

**สิทธิ์:** `routes` (prefix `/promptpay`) — เฉพาะสำหรับ route ตัวอย่างข้างบน. ส่วน library ไม่ต้องใช้สิทธิ์

**การตั้งค่า:** Site code → Settings → unit `promptpay` → `promptpayId` (เบอร์/เลขบัตร/e-Wallet ของร้าน)

**ทดสอบแล้ว:** ข้อความตรงกับชุดทดสอบที่เผยแพร่ของ `promptpay-qr` (dtinth) ทุกกรณี, CRC ตรวจด้วยตัวคำนวณแยกอีกชุด,
Reed–Solomon ตรงกับตัวอย่างใน ISO/IEC 18004, QR ของ payload ตรงกับไลบรารี segno ทุก module และอ่านกลับได้ด้วย zxing

## English

PromptPay payloads (Thai QR Payment, EMVCo merchant-presented mode as profiled by the Bank of Thailand) and a
dependency-free QR encoder.

- `payload({ id, amount? })` — phone (tag 29 sub 01, `0066` + 9 digits), citizen / tax ID (sub 02) or e-wallet
  (sub 03); with an amount the QR is dynamic (`01`=`12`, tag 54 with 2 decimals). Bill payment (tag 30) with
  `billerId`, `ref1`, `ref2`. Tags 58 `TH`, 53 `764`, 63 CRC-16/CCITT-FALSE (upper-case hex).
- `qrSvg(payload, opts)` / `qrMatrix(text)` — QR Code, byte mode, level M, versions 1-40, penalty-based mask.
- Plugin routes `GET /promptpay/qr.svg?amount=` and `GET /promptpay/payload?amount=` for the `promptpayId` setting.
- Export `qrFor(amount?, { size })` → `{ payload, svg, url }` or `null` (no `promptpayId`) for plugins that use
  promptpay optionally (`optionalRequires` + `plugins.has` / `plugins.call('promptpay', 'qrFor', 150)`); it runs in
  the promptpay unit, with its settings (see the `shop` sample).

**Scopes:** `routes` (prefix `/promptpay`) for the sample routes only; the library needs none.

**Settings:** `promptpayId` (Site code → Settings → unit `promptpay`).

## Site Code example / ตัวอย่าง

```ts
import { payload, qrSvg } from 'promptpay';

// checkout page data: {{page.qr}} in an HTML block, {{page.amount}}
site.page('/checkout/{order}', {
  load: ({ params }) => {
    const amount = 990;                                     // look the order up in your own tables
    return { amount, qr: qrSvg(payload({ id: '0812345678', amount }), { size: 240 }) };
  },
});
```

Or without code: `<img src="/promptpay/qr.svg?amount=150" alt="PromptPay">` once `promptpayId` is set.
