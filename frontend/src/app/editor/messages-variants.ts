import { registerMessages } from '../core/messages';

// Component variants (Figma).
registerMessages({
  'vr.title': { th: 'Variants', en: 'Variants' },
  'vr.tip': { th: 'แบบต่างๆ ของ component เดียวกัน เช่น ปุ่มหลัก/ปุ่มรอง หรือการ์ดแนวตั้ง/แนวนอน — แต่ละจุดที่วางเลือกได้ว่าจะใช้แบบไหน (ดับเบิลคลิกเพื่อเปลี่ยนชื่อ)', en: 'Different designs of one component (primary/secondary button, vertical/horizontal card…) — each placement picks one. Double-click to rename.' },
  'vr.add': { th: 'เพิ่มแบบ', en: 'Add variant' },
  'vr.addTip': { th: 'คัดลอกแบบที่เปิดอยู่เป็นแบบใหม่ แล้วแก้ได้อิสระ', en: 'Copy the open design as a new variant and change it freely' },
  'vr.addPrompt': { th: 'ชื่อแบบใหม่', en: 'Variant name' },
  'vr.newName': { th: 'แบบที่ {n}', en: 'Variant {n}' },
  'vr.rename': { th: 'เปลี่ยนชื่อ', en: 'Rename' },
  'vr.renamePrompt': { th: 'ชื่อแบบ', en: 'Variant name' },
  'vr.taken': { th: 'มีแบบชื่อนี้แล้ว', en: 'A variant with this name exists' },
  'vr.deleteHint': { th: 'จุดที่วางแบบนี้ไว้จะกลับไปใช้แบบหลัก', en: 'Placements using it go back to the main design' },
  'vr.pick': { th: 'แบบ (Variant)', en: 'Variant' },
  'vr.renameWarnTitle': { th: 'เปลี่ยนชื่อแบบนี้?', en: 'Rename this variant?' },
  'vr.renameWarn': {
    th: 'จุดที่วาง component นี้ไว้ในหน้าอื่นหรือ component อื่นโดยเลือกแบบ “{old}” จะกลับไปแสดงแบบหลัก จนกว่าจะเลือกแบบ “{name}” ให้ใหม่',
    en: 'Placements on other pages or components that use “{old}” will show the main design until you pick “{name}” for them again.',
  },
});
