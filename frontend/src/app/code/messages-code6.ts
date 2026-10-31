import { registerMessages } from '../core/messages';

/** Messages of Site Code phase 6: Formatters tab (plugin formatters + aliases), admin pages on the Access screen, "Works with" chips. */
registerMessages({
  // formatters tab
  'code.tab.formatters': { th: 'ตัวจัดรูปแบบ', en: 'Formatters' },
  'cf.title': { th: 'ตัวจัดรูปแบบ (formatters)', en: 'Formatters' },
  'cf.desc': { th: 'ตัวจัดรูปแบบจากโค้ดของเว็บและปลั๊กอิน ใช้ในข้อความ เช่น {{ price | thai:baht }} — เปิดชื่อสั้นเพื่อเขียน | baht ได้', en: 'Formatters from the site code and plugins, used in text like {{ price | thai:baht }} — enable the short name to write | baht' },
  'cf.reload': { th: 'โหลดใหม่', en: 'Reload' },
  'cf.search': { th: 'ค้นหาชื่อ…', en: 'Search names…' },
  'cf.col.name': { th: 'ชื่อ', en: 'Name' },
  'cf.col.usage': { th: 'ตัวอย่างการใช้', en: 'Usage' },
  'cf.col.alias': { th: 'ชื่อสั้น (ไม่มีชื่อปลั๊กอินนำหน้า)', en: 'Short name (no plugin prefix)' },
  'cf.unit.site': { th: 'โค้ดของเว็บ', en: 'Site code' },
  'cf.unit.plugin': { th: 'ปลั๊กอิน {id}', en: 'Plugin {id}' },
  'cf.siteNoAlias': { th: 'ใช้ชื่อนี้ได้เลย', en: 'Usable as is' },
  'cf.aliasOn': { th: 'เปิด | {alias}', en: 'Enable | {alias}' },
  'cf.conflict.core': { th: 'ชื่อ “{name}” เป็นตัวจัดรูปแบบของระบบ — ใช้ชื่อเต็มแทน', en: '“{name}” is a built-in formatter — use the full name' },
  'cf.conflict.site': { th: 'โค้ดของเว็บมี “{name}” อยู่แล้ว (โค้ดของเว็บชนะ)', en: 'The site code already has “{name}” (the site code wins)' },
  'cf.conflict.plugin': { th: 'ชื่อนี้ถูกใช้โดย {by} แล้ว — ปิดตัวนั้นก่อน', en: 'Already used by {by} — turn that one off first' },
  'cf.conflict.server': { th: 'ใช้ชื่อสั้นไม่ได้: {why}', en: 'Short name inactive: {why}' },
  'cf.saved': { th: 'บันทึกชื่อสั้นแล้ว', en: 'Short names saved' },
  'cf.empty': { th: 'โค้ดของเว็บและปลั๊กอินที่เปิดอยู่ยังไม่มีตัวจัดรูปแบบ (site.formatter)', en: 'No formatters yet (site.formatter) in the site code or active plugins' },
  'cf.hint': { th: 'ลำดับเมื่อชื่อซ้ำ: ตัวจัดรูปแบบของระบบ → โค้ดของเว็บ → ชื่อสั้นที่เปิดก่อน · ชื่อเต็ม (ปลั๊กอิน:ชื่อ) ใช้ได้เสมอ', en: 'When names clash: built-in → site code → the short name enabled first · the full name (plugin:name) always works' },
  'cf.readOnly': { th: 'ต้องมีสิทธิ์จัดการโค้ดจึงจะเปิด/ปิดชื่อสั้นได้', en: 'Managing code is required to switch short names' },

  // access screen
  'ca.kind.adminPage': { th: 'หน้าผู้ดูแล (site.adminPage)', en: 'Admin pages (site.adminPage)' },
  'ca.adminPageNoEveryone': { th: 'หน้าผู้ดูแลต้องล็อกอินเสมอ', en: 'Admin pages always require signing in' },
});
