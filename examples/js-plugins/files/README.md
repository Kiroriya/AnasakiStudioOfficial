# File exports (`files`) — ส่งออกไฟล์ CSV / Excel / PDF

## ภาษาไทย

ปลั๊กอินแบบ **library** สำหรับสร้างไฟล์ส่งออก ไม่มี dependency

- `toCSV(rows, columns?, { separator, bom, header, safe })` — CSV ตาม RFC 4180 (CRLF, ใส่ `"` เมื่อจำเป็น, `""` แทน `"`)
  ขึ้นต้นด้วย UTF-8 BOM ให้ Excel อ่านภาษาไทยถูก. `safe` (ค่าเริ่มต้น true) เติม `'` หน้าข้อความที่ขึ้นต้นด้วย `= + - @`
  เพื่อกัน CSV/formula injection (ตัวเลขไม่ถูกแตะ)
- `toXLSX(rows, columns?, { sheetName, header })` → `Uint8Array` ไฟล์ .xlsx (OOXML ขั้นต่ำ: inline strings, หัวตารางตัวหนา
  ตรึงแถวแรก) ใช้ zip แบบ stored ที่เขียนเอง
- `toPDF({ title, lines, table, fontSize, pageSize, landscape, font? })` → PDF 1.4 ขึ้นหน้าใหม่อัตโนมัติ ไฟล์เป็น ASCII ล้วน
  ไม่มี `font` = Helvetica ในตัว (Latin-1 เท่านั้น); มี `font` (ไบต์ของ .ttf หรือ .woff ที่ไม่บีบอัด หรือ `parseTTF(...)`) = ฝังฟอนต์
  TrueType แบบ CIDFontType2 / Identity-H พร้อม ToUnicode (คัดลอก/ค้นหาข้อความไทยได้) วางสระ/วรรณยุกต์ด้วย anchor ใน GPOS
  ของฟอนต์ (mark/mkmk) แยก ำ เป็น ํ+า — ไม่มี GSUB (ไม่มี ligature/รูปตามบริบท)
- **PDF ภาษาไทย**: `thaiPdf(spec)` / `thaiPdfDownload(name, spec)` ใช้ฟอนต์ **Sarabun** ที่มากับแพ็กเกจ
  (`fonts/Sarabun-Regular.woff` — subset ไทย + Latin-1 + เครื่องหมายวรรคตอน ~43 KB, ตารางไม่บีบอัด, สัญญาอนุญาต SIL OFL 1.1
  ใน `fonts/OFL.txt`) เรียก export `renderThaiPdf` ของปลั๊กอิน files ผ่าน `plugins.call` (เพราะ `site.asset` อ่านไฟล์ในแพ็กเกจ
  ของ unit ที่เรียกเท่านั้น) — spec ถูกคัดลอกเป็น JSON: ฟังก์ชัน `format` ของคอลัมน์หายไป ให้จัดรูปแบบแถวก่อน
  ฟอนต์อื่น: ใส่ในแพ็กเกจของคุณแล้ว `toPDF({ ..., font: site.asset('fonts/x.woff').bytes() })` — แพ็กเกจรับ `.woff` แต่ไม่รับ
  `.ttf` และตารางใน WOFF ต้องไม่บีบอัด (ไม่มี zlib ใน engine) สร้างได้ด้วย fontTools ดูหัวข้อ "Font files" ด้านล่าง
- `columns`: `'key'` หรือ `{ key: 'customer.name', label: 'ลูกค้า', format: (v, row) => …, width }`
- `download(name, content, type)` (ข้อความ หรือไบต์ `Uint8Array`/`ArrayBuffer`), `csvDownload(name, rows, columns)`,
  `xlsxDownload(name, rows, columns)` (ส่งไฟล์ .xlsx เป็น binary ตรง ๆ), `pdfDownload(name, spec)` — ผลลัพธ์ route หรือ
  `download` ของ `site.adminPage` พร้อม `Content-Type` และ `Content-Disposition` (ชื่อไฟล์ภาษาไทยได้ผ่าน `filename*`)
- `xlsxPayload(name, rows, columns)` → `{ filename, contentType, base64 }` (คงไว้เพื่อความเข้ากันได้ และใช้จาก `site.action`)
- ดาวน์โหลดจากหน้าแอดมิน: ใส่ผลของ helper เหล่านี้ใน `download(ctx)` ของ `site.adminPage` แล้วเรียก `anasaki.download(query)`
  ในหน้า (ดูตัวอย่างใน admin-crud / members)

**ข้อจำกัด:** PDF ไม่มี GSUB shaping (เพียงพอสำหรับภาษาไทยด้วย Sarabun; อักษรที่ต้องการ ligature/รูปตามบริบท เช่น อาหรับ ยังไม่รองรับ)
ฟอนต์ถูกฝังทั้งไฟล์ (ไม่ subset ตอนสร้าง PDF) — PDF ไทยมีขนาดราว 60–100 KB ขึ้นไป (ฟอนต์ ~55 KB เมื่อเข้ารหัส ASCII85)

**สิทธิ์:** ไม่มี (library ทำงานด้วยสิทธิ์ของผู้ import; export `renderThaiPdf` ไม่ต้องใช้สิทธิ์) **การตั้งค่า:** ไม่มี

## English

A dependency-free **library** for exports: `toCSV` (RFC 4180, UTF-8 BOM for Excel, formula-injection guard),
`toXLSX` (minimal OOXML in a stored zip → `Uint8Array`), `toPDF` (PDF 1.4, paginated table, pure ASCII), and helpers
for routes and admin-page downloads: `download` (text or bytes), `csvDownload`, `xlsxDownload` (binary body),
`pdfDownload`, `xlsxPayload` (JSON + base64, kept for compatibility / actions).

**Thai PDFs:** `toPDF({ ..., font })` embeds a TrueType font (bytes of a .ttf, or a .woff with uncompressed tables,
or `parseTTF(bytes)`) as CIDFontType2 / Identity-H with a ToUnicode map (copy & search work); Thai vowels and tone
marks are placed with the font's GPOS mark/mkmk anchors and SARA AM is decomposed — there is no GSUB shaping.
`thaiPdf(spec)` / `thaiPdfDownload(name, spec)` use the bundled **Sarabun** (`fonts/Sarabun-Regular.woff`, a Thai +
Latin-1 + punctuation subset, ~43 KB, SIL Open Font License 1.1 in `fonts/OFL.txt`) through the files unit's export
`renderThaiPdf` (`plugins.call`, because `site.asset` reads the *calling* unit's package); the spec is copied as JSON
(column `format` functions are dropped — format rows first). Own font: ship it in your package and pass
`font: site.asset('fonts/x.woff').bytes()`. The whole font is embedded (a Thai PDF is ~60–100 KB; the font is ~55 KB as ASCII85).

**Font files:** packages accept `.woff` (not `.ttf`), and the reader needs WOFF tables stored *uncompressed* (no
zlib in the engine). The bundled file was made with fontTools (subset, then WOFF at zlib level 0, which stores
every table as is):

```sh
pyftsubset Sarabun-Regular.ttf --unicodes="U+0020-007E,U+00A0-00FF,U+0E01-0E5B,U+2010-2027,U+2030,U+2039,U+203A,U+20AC,U+2122,U+25CC" \
  --layout-features="kern,mark,mkmk,ccmp,locl" --name-IDs='*' --notdef-outline --drop-tables+=DSIG --output-file=sub.ttf
python -c "import fontTools.ttLib.sfnt as s; s.ZLIB_COMPRESSION_LEVEL = 0
from fontTools.ttLib import TTFont; f = TTFont('sub.ttf'); f.flavor = 'woff'; f.save('Sarabun-Regular.woff')"
```

Keep the font's licence file next to it (OFL fonts must ship with `OFL.txt`).

**Scopes:** none. **Settings:** none.

## Site Code example / ตัวอย่าง

```ts
import { csvDownload, pdfDownload, xlsxDownload, thaiPdfDownload } from 'files';

const cols = [{ key: 'id', label: '#' }, { key: 'item', label: 'สินค้า' }, { key: 'total', label: 'ยอด' }];

site.route('GET /reports/orders.csv', { access: ['group:staff'] }, () =>
  csvDownload('orders.csv', db.table('orders').latest().limit(5000).get(), cols));

site.route('GET /reports/orders.pdf', { access: ['group:staff'] }, () =>
  pdfDownload('orders.pdf', { title: 'Orders', table: { columns: cols, rows: db.table('orders').latest().limit(500).get() } }));

site.route('GET /reports/orders.xlsx', { access: ['group:staff'] }, () =>
  xlsxDownload('orders.xlsx', db.table('orders').get(), cols));

// Thai PDF with the bundled Sarabun font
site.route('GET /reports/summary.pdf', { access: ['group:staff'] }, () =>
  thaiPdfDownload('ใบสรุป.pdf', { title: 'สรุปยอดขาย', table: { columns: cols, rows: db.table('orders').limit(500).get() } }));

// an "Export" button on an admin page: anasaki.download({ format: 'xlsx' }) in the page's HTML
site.adminPage('reports', {
  label: { th: 'รายงาน', en: 'Reports' },
  render: () => `<button onclick="anasaki.download({ format: 'xlsx' })">Excel</button>`,
  download: ({ query }) => query.format === 'xlsx'
    ? xlsxDownload('orders.xlsx', db.table('orders').get(), cols)
    : csvDownload('orders.csv', db.table('orders').get(), cols),
});
```
