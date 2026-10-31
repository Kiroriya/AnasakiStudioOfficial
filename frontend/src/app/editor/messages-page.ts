import { registerMessages } from '../core/messages';

// Page settings (nothing selected in the designer).
registerMessages({
  'ps.title': { th: 'ตั้งค่าหน้านี้', en: 'This page' },
  'ps.name': { th: 'ชื่อหน้า', en: 'Page name' },
  'ps.home': { th: 'หน้านี้คือหน้าแรกของเว็บ (ที่อยู่ /)', en: 'This is the home page (address /)' },
  'ps.address': { th: 'ที่อยู่ (URL)', en: 'Address (URL)' },
  'ps.addressPh': { th: 'เช่น contact', en: 'e.g. contact' },
  'ps.addressHint': { th: 'ปุ่มหรือเมนูที่ลิงก์มาหน้านี้ต้องใช้ที่อยู่เดียวกัน (เช่น ปุ่ม "ติดต่อเรา" ในส่วนหัวเว็บลิงก์ไป /contact) — มีผลเมื่อกดบันทึก', en: 'Buttons and menus linking here must use the same address (the header "Contact" button links to /contact) — applies when you save' },
  'fl.title': { th: 'มี {n} ชิ้นที่ลอยอยู่ (Absolute) ในกล่องที่จัดเรียงอัตโนมัติ', en: '{n} layer(s) float (Absolute) inside auto-layout frames' },
  'fl.hint': { th: 'ชิ้นที่ลอยจะอยู่ที่เดิมเสมอ ขณะที่ข้อความรอบๆ ขึ้นบรรทัดใหม่บนจอเล็ก — บนแท็บเล็ต/มือถืออาจทับกัน แนะนำจัดเข้าแถวให้ปรับตามจอเอง', en: 'Floating layers keep their spot while the text around them reflows — on tablets and phones they can cover it. Put them back in the flow so they adapt.' },
  'fl.fixAll': { th: 'จัดเข้าแถวทั้งหมด', en: 'Put all back in the flow' },
  'fl.show': { th: 'เลือกดู', en: 'Select them' },
  'fl.one': { th: 'ชิ้นนี้ลอยอยู่ อาจทับข้อความบนจอเล็ก', en: 'This layer floats and may cover text on small screens' },
  'fl.fixOne': { th: 'จัดเข้าแถว', en: 'Put in the flow' },
  'fl.fixed': { th: 'จัดเข้าแถวแล้ว {n} ชิ้น', en: 'Put {n} layer(s) back in the flow' },
  'fl.textWhite': { th: 'ปรับตัวอักษรเป็นสีขาวให้อ่านออกบนพื้นสีนี้', en: 'Text switched to white so it stays readable' },
  'fl.textDark': { th: 'ปรับตัวอักษรเป็นสีเข้มให้อ่านออกบนพื้นสีนี้', en: 'Text switched to dark so it stays readable' },
});
