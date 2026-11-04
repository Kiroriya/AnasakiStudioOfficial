# Page Translate (`page-translate`) — ปุ่มแปลภาษาหน้าเว็บ

## ภาษาไทย

ปุ่มลอยมุมล่างขวาของทุกหน้าเว็บ ให้ผู้เข้าชมเลือกแปลหน้าเป็น English / ไทย / 日本語 / 简体中文 / 繁體中文 / 한국어 /
Tiếng Việt และกด "Show original" เพื่อกลับเป็นต้นฉบับ มี 2 โหมด

- **ฟรี (ค่าเริ่มต้น)** — ใช้ Google Translate Website Widget ไม่ต้องมี API key และไม่มีค่าใช้จ่าย ปลั๊กอินซ่อน
  แถบของ Google แล้วใช้ปุ่มลอยของตัวเอง ภาษาที่เลือกเก็บในคุกกี้ `googtrans` ของโดเมนนี้เท่านั้น แล้วโหลดหน้าใหม่
- **API** — ใช้ Google Cloud Translation API v2 แปลข้อความเป็นชุด เก็บผลไว้ใน `localStorage` (ไม่แปลซ้ำ)
  ข้าม `<script> <style> <code> <pre> <textarea>` และส่วนที่มี `class="notranslate"` หรือ `translate="no"`
  ข้อความที่โหลดเข้ามาทีหลัง (เช่น รายการโพสต์) ก็แปลตามให้

ตั้งค่าที่ **แปลภาษา (ตั้งค่า)** ในเมนูหลังบ้าน: โหมด, ภาษาต้นฉบับของเว็บ (th / en / ja / zh), สีปุ่มลอย และ
Google API key (แสดงเฉพาะ 4 ตัวท้าย) · key ใส่ในหน้าเว็บเฉพาะตอนใช้โหมด API (`<meta name="ptw-api-key">`) จึง
**ต้อง**จำกัด key ใน Google Cloud Console ด้วย HTTP referrer และให้ใช้ได้แค่ Cloud Translation API

สิทธิ์ของตำแหน่ง (ผู้ใช้ → สิทธิ์การใช้งาน → สิทธิ์ของปลั๊กอิน):

| สิทธิ์ | ความหมาย | ค่าเริ่มต้น |
|---|---|---|
| `page-translate.manage` | เปิดหน้าตั้งค่า แก้โหมด ภาษาต้นฉบับ และสีปุ่ม | ผู้ดูแลระบบ, บรรณาธิการ |
| `page-translate.api_key` | ใส่ เปลี่ยน หรือลบ API key (ต้องมี `manage` ด้วย) | ผู้ดูแลระบบ |

ต้องการ Anasaki Studio 2.0.0 ขึ้นไป (สิทธิ์แยกต่อปลั๊กอินตั้งแต่ 2.0.6) · เบราว์เซอร์ของผู้เข้าชมเรียก
`translate.google.com` (ฟรี) หรือ `translation.googleapis.com` (API)

## English

A floating translate button on every public page (7 target languages plus "Show original"). **Free** mode uses
the Google Translate Website Widget (no key, no billing; the choice is a host-only `googtrans` cookie and a
reload). **API** mode calls Google Cloud Translation API v2 in batches with a `localStorage` cache, skips code,
scripts and `notranslate` parts, and translates text added later. The admin page **Page Translate** sets the
mode, site language, button colour and the API key (masked). The key is written into the page only in API mode
(`<meta name="ptw-api-key">`) — restrict it by HTTP referrer and to the Cloud Translation API. Capabilities:
`page-translate.manage` (administrator, editor) and `page-translate.api_key` (administrator).

## Changelog

- **1.4.0** — official plugin (Anasaki Studio). The API key is no longer in the page while the free mode is on;
  API mode asks for plain text (no more `&amp;` / `&#39;` in translations) and translates content added later;
  the `googtrans` cookie is host-only so sites under the same parent domain keep their own language; the menu
  updates `aria-expanded` and Escape returns focus to the button.
- **1.3.0** — admin page and the `manage` / `api_key` capabilities; JavaScript plugin (`runtime: js`).
- **1.2.0** — `minCore` 2.0.0.
