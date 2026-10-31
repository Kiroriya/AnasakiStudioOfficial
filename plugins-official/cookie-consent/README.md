# Cookie consent (`cookie-consent`) — แบนเนอร์ขอความยินยอมคุกกี้

## ภาษาไทย

แบนเนอร์ขอความยินยอมการใช้คุกกี้ตาม PDPA / GDPR — ปลั๊กอินทางการเวอร์ชัน 2 ของปลั๊กอินในตัว (ค่าตั้งค่าเดิมย้ายมา
อัตโนมัติ ชื่อค่าเหมือนเดิม)

- ตั้งค่าที่ ปลั๊กอิน → Cookie consent → ตั้งค่า: ข้อความ, ปุ่มยอมรับ, ปุ่มปฏิเสธ (เว้นว่าง = ซ่อน), ลิงก์และข้อความ
  นโยบายความเป็นส่วนตัว, ตำแหน่ง (ล่าง/บน), สีพื้นหลัง สีตัวอักษร สีปุ่ม สีตัวอักษรบนปุ่ม
- ทางเลือกของผู้เข้าชมเก็บใน `localStorage.anasaki_consent` (`accepted` | `declined`, เวลาใน `anasaki_consent_at`)
  ผู้ที่เคยเลือกแล้วจะไม่เห็นแบนเนอร์อีก
- สคริปต์อื่นรอความยินยอมได้:

```js
function start() { /* … */ }
if (window.anasakiConsent === 'accepted') start();
document.addEventListener('anasaki:consent', e => e.detail === 'accepted' && start());
```

ทำงานด้วย template `bodyEnd` ของแพ็กเกจ (ทุกค่าถูก escape สีถูกตรวจรูปแบบ) จึงไม่ต้องใช้ scope ของโค้ดใด ๆ

## English

PDPA / GDPR cookie banner — version 2 of the built-in plugin (settings migrate with the same keys: `message`,
`acceptText`, `declineText` (empty = hidden), `policyUrl`, `policyText`, `position` (`bottom` | `top`), `background`,
`textColor`, `buttonColor`, `buttonTextColor`). The choice is stored in `localStorage.anasaki_consent` and announced
with the `anasaki:consent` event (`window.anasakiConsent`). Rendered by the package's `bodyEnd` template (values
escaped, colours validated) — no code scope needed. Fixed in 2.0: the banner now really hides after a choice.
