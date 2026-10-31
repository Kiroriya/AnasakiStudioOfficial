import { registerMessages } from '../core/messages';

// Link fields and moved pages.
registerMessages({
  'lk.pick': { th: 'เลือกหน้าในเว็บ', en: 'Pick a page' },
  'lk.missing': { th: 'ยังไม่มีหน้าที่อยู่ {path} — คลิกแล้วจะเจอหน้า "ไม่พบ"', en: 'No page at {path} yet — visitors would get "not found"' },
  'lk.create': { th: 'สร้างหน้านี้', en: 'Create this page' },
  'lk.createPrompt': { th: 'ชื่อหน้าใหม่ (ที่อยู่ {path})', en: 'Name of the new page ({path})' },
  'lk.created': { th: 'สร้างหน้า "{title}" ({path}) เป็นฉบับร่างแล้ว — เปิดไปออกแบบเลยไหม?', en: 'Created "{title}" ({path}) as a draft — open it now?' },
  'lk.open': { th: 'เปิดออกแบบ', en: 'Open it' },
  'lk.stay': { th: 'อยู่หน้านี้ต่อ', en: 'Stay here' },
  'lk.t.contact': { th: 'ติดต่อเรา', en: 'Contact' },
  'lk.t.about': { th: 'เกี่ยวกับเรา', en: 'About us' },
  'lk.t.services': { th: 'บริการ', en: 'Services' },
  'lk.t.menu': { th: 'เมนู', en: 'Menu' },
  'lk.t.shop': { th: 'ร้านค้า', en: 'Shop' },
  'ln.relinked': { th: 'ย้ายที่อยู่ {from} → {to} และอัปเดตลิงก์ที่ชี้มาแล้ว {n} จุด', en: 'Moved {from} → {to} and updated links in {n} places' },
});
