# Thai helpers (`thai`) — ตัวช่วยภาษาไทย

## ภาษาไทย

ปลั๊กอินแบบ **library**: เมื่อ `import` โค้ดจะถูกรวม (bundle) เข้าไปในโค้ดของผู้เรียก และทำงานด้วยสิทธิ์ของผู้เรียก
ตัวปลั๊กอินเอง (unit `thai`) ลงทะเบียน **formatter** และมี export `format` สำหรับปลั๊กอินที่ใช้ thai แบบไม่บังคับ

- `bahtText(n)` — จำนวนเงินเป็นตัวหนังสือแบบ BAHTTEXT: `21` → ยี่สิบเอ็ดบาทถ้วน, `1000001` → หนึ่งล้านเอ็ดบาทถ้วน,
  `12.5` → สิบสองบาทห้าสิบสตางค์, `0.5` → ห้าสิบสตางค์, ติดลบขึ้นต้นด้วย "ลบ". ปัดสตางค์ครึ่งขึ้นจากตัวเลขฐานสิบจริง
  (`1.005` → หนึ่งบาทหนึ่งสตางค์), รับ number / string (`"1,250.75"`) / BigInt, เลขใหญ่แค่ไหนก็ได้ (ล้านล้าน)
- `thaiDate(d, fmt)` — วันที่ พ.ศ. เวลา Asia/Bangkok. รูปแบบสำเร็จ `short` (30 ต.ค. 69), `medium`, `long` (ค่าเริ่มต้น:
  30 ตุลาคม 2569), `full` (วันศุกร์ที่ 30 ตุลาคม พ.ศ. 2569), `datetime`, `time` หรือ token `D DD M MM MMM MMMM BB BBBB
  YY YYYY ddd dddd H HH m mm s ss [ข้อความ]`. ค่า `YYYY-MM-DD` ไม่ถูกเลื่อนเขตเวลา
- `thaiNumber(v, decimals?)` → เลขไทย (`1234.5` → ๑,๒๓๔.๕), `baht(v)` → `฿1,234.50`
- `isThaiPhone`, `normalizePhone(v, 'local' | 'e164')`, `isThaiCitizenId` (ตรวจ checksum หลักที่ 13), `formatCitizenId`
- **Formatter (ลงทะเบียนเองเมื่อเปิดใช้ปลั๊กอิน):** ใน binding/สูตรใช้ชื่อแบบมี namespace
  `{{ item.price | thai:baht }}`, `{{ total | thai:bahtText }}`, `{{ post.publishedAt | thai:thaiDate:'full' }}`,
  `{{ n | thai:thaiNumber }}` — ผู้ดูแลเปิดชื่อสั้น (alias) เช่น `| baht` ได้ที่หน้า **ปลั๊กอิน** (ถ้าชื่อชนกัน formatter
  ของ Site Code ชนะ แล้วจึงเป็น alias ที่เปิดก่อน)
- export `format(name, value, ...args)` — สำหรับปลั๊กอินที่ระบุ thai ใน `optionalRequires`:
  `plugins.has('thai') ? plugins.call('thai', 'format', 'bahtText', 1250.75) : …`
- `registerFormatters(prefix?)` — **เลิกแนะนำ (deprecated)** ยังใช้ได้เหมือนเดิม: ลงทะเบียน `baht`, `bahtText`,
  `thaiDate`, `thaiNumber` เป็น formatter ของหน่วยที่เรียก (ใน Site Code ได้ชื่อสั้นแบบเดิม) — ไม่จำเป็นอีกแล้ว

**สิทธิ์ (scopes):** ไม่มี — library ทำงานในบริบทของผู้ import, formatter และ export ไม่ต้องใช้สิทธิ์

**การตั้งค่า:** ไม่มี

## English

A **library** plugin: importing it bundles the code into the importer, which runs it with its own scopes.
The plugin's own unit registers the **binding formatters** and an export `format` for optional users.

- `bahtText(n)` — the amount in Thai words like Excel's BAHTTEXT (เอ็ด / ยี่ rules, ล้าน groups, สตางค์ rounded
  half-up on the decimal digits, ถ้วน, negative → ลบ). Accepts numbers, numeric strings and BigInt.
- `thaiDate(d, fmt)` — Buddhist-era dates in Asia/Bangkok; named formats or tokens (see above).
- `thaiNumber`, `baht`, `isThaiPhone`, `normalizePhone`, `isThaiCitizenId` (13th-digit checksum), `formatCitizenId`.
- **Formatters** (registered by the plugin itself): `| thai:baht`, `| thai:bahtText`, `| thai:thaiDate:'full'`,
  `| thai:thaiNumber`. An administrator can enable unprefixed aliases (`| baht`) on the **Plugins** page (on a
  clash Site Code's own formatter wins, then the first enabled alias).
- Export `format(name, value, ...args)` for plugins that list thai in `optionalRequires`:
  `plugins.has('thai') ? plugins.call('thai', 'format', 'bahtText', 1250.75) : …`
- `registerFormatters(prefix?)` — **deprecated**, still works: registers `baht`, `bahtText`, `thaiDate`,
  `thaiNumber` as formatters of the *calling* unit (unprefixed names in Site Code). No longer needed.

**Scopes:** none — the library runs in its importer's context; formatters and the export need none.

**Settings:** none.

## Site Code example / ตัวอย่าง

```ts
import { bahtText, thaiDate, isThaiCitizenId } from 'thai';

// formatters need no code: {{ item.total | thai:bahtText }}  {{ post.publishedAt | thai:thaiDate:'full' }}  {{ item.price | thai:baht }}

site.route('GET /receipt/{amount}', { access: ['*'] }, ({ request }) => ({
  amount: Number(request.params.amount),
  words: bahtText(request.params.amount),      // "1250.75" → หนึ่งพันสองร้อยห้าสิบบาทเจ็ดสิบห้าสตางค์
  date: thaiDate(new Date(), 'long'),
}));

site.action('kyc', {
  input: { citizenId: 'text' }, access: ['*'],
  run: ({ input }) => isThaiCitizenId(input.citizenId) ? { message: 'OK' } : { errors: { citizenId: 'เลขบัตรไม่ถูกต้อง' } },
});
```

A JS plugin uses it by adding `"requires": { "thai": "^1.0.0" }` to its manifest and importing the same way — or,
to use it only when installed, `"optionalRequires": { "thai": "^1.0.0" }` plus `plugins.has` / `plugins.call`
(see the `shop` sample).
Install: zip this folder (manifest.json at the root) and upload it in **Plugins**, then activate.
