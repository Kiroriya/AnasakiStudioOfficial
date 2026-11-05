import { registerMessages } from '../core/messages';

/** Messages of Site Code phase 4b: unit switcher, Hooks tab, Export as plugin, designer source groups. */
registerMessages({
  // unit switcher
  'cu.label': { th: 'โค้ดของ', en: 'Code of' },
  'cu.site': { th: 'โค้ดของเว็บ', en: 'Site code' },
  'cu.plugin': { th: 'ปลั๊กอิน {name}', en: 'Plugin {name}' },
  'cu.pluginSettingsHint': { th: 'ค่าของปลั๊กอินนี้ (โค้ดอ่านด้วย site.settings(\'KEY\') เฉพาะของตัวเอง) — โดเมนที่เรียกได้มาจาก manifest ของปลั๊กอินและต้องอยู่ในรายการที่อนุญาตของ “โค้ดของเว็บ” ด้วย', en: 'Values of this plugin (its code reads only its own keys with site.settings(\'KEY\')) — callable hosts come from the plugin manifest and must also be on the “Site code” allowlist' },

  // hooks tab
  'code.tab.hooks': { th: 'ฮุก', en: 'Hooks' },
  'ch.title': { th: 'ฮุก (ตัวกรองและเหตุการณ์)', en: 'Hooks (filters and events)' },
  'ch.desc': { th: 'ตัวกรอง (site.filter) และเหตุการณ์ (site.on) ของทุกหน่วยโค้ด เรียงตามลำดับที่ทำงานจริง', en: 'Filters (site.filter) and events (site.on) of every code unit, in the order they run' },
  'ch.search': { th: 'ค้นหาชื่อฮุก…', en: 'Search hooks…' },
  'ch.allUnits': { th: 'ทุกหน่วยโค้ด', en: 'All units' },
  'ch.filters': { th: 'ตัวกรอง', en: 'Filters' },
  'ch.events': { th: 'เหตุการณ์', en: 'Events' },
  'ch.other': { th: 'อื่น ๆ', en: 'Other' },
  'ch.declaredBy': { th: 'ประกาศโดย', en: 'Declared by' },
  'ch.core': { th: 'ระบบหลัก', en: 'Core' },
  'ch.col.name': { th: 'ชื่อฮุก', en: 'Hook' },
  'ch.col.subs': { th: 'ผู้รับ (ตามลำดับที่ทำงาน)', en: 'Subscribers (in run order)' },
  'ch.noSubscribers': { th: 'ยังไม่มีผู้รับ', en: 'No subscribers' },
  'ch.priority': { th: 'ลำดับความสำคัญ', en: 'Priority' },
  'ch.empty': { th: 'ยังไม่มีฮุก', en: 'No hooks yet' },
  'ch.reload': { th: 'โหลดใหม่', en: 'Reload' },
  'ch.orderHint': { th: 'ลำดับ: priority น้อยก่อน แล้วตามลำดับการโหลด (ปลั๊กอินตาม requires → โค้ดของเว็บทำงานท้ายสุด)', en: 'Order: lower priority first, then load order (plugins by requires → Site code runs last)' },

  // export as plugin
  'cx.open': { th: 'ส่งออกเป็นปลั๊กอิน', en: 'Export as plugin' },
  'cx.title': { th: 'ส่งออกโค้ดของเว็บเป็นปลั๊กอิน', en: 'Export Site code as a plugin' },
  'cx.intro': { th: 'สร้างไฟล์ .zip ของปลั๊กอิน JS จากเวอร์ชันที่เผยแพร่อยู่ (ไม่ใช่ฉบับร่าง) — นำไปติดตั้งที่เว็บอื่นหรือส่งขึ้นสโตร์ได้', en: 'Builds a JS plugin .zip from the LIVE version (not the draft) — install it on another site or upload it to the store' },
  'cx.checking': { th: 'กำลังตรวจโค้ด…', en: 'Checking the code…' },
  'cx.id': { th: 'รหัสปลั๊กอิน (id)', en: 'Plugin id' },
  'cx.idHint': { th: 'a-z 0-9 และ - (3–40 ตัว) ใช้เป็นชื่อตาราง p_<id>_ และคำนำหน้าฮุก <id>.', en: 'a-z 0-9 and - (3–40 chars); used for tables p_<id>_ and the hook prefix <id>.' },
  'cx.name': { th: 'ชื่อ', en: 'Name' },
  'cx.version': { th: 'เวอร์ชัน', en: 'Version' },
  'cx.description': { th: 'คำอธิบาย', en: 'Description' },
  'cx.scopes': { th: 'สิทธิ์ของโค้ด', en: 'Code permissions' },
  'cx.scopesHint': { th: 'เลือกไว้ตาม API ที่โค้ดใช้ — ปลั๊กอินจะใช้ได้เฉพาะสิทธิ์ที่เลือก', en: 'Pre-selected from the APIs the code uses — the plugin may only use what is ticked' },
  'cx.inferred': { th: 'โค้ดใช้', en: 'used by the code' },
  'cx.routes': { th: 'ที่อยู่ที่ใช้ (route prefix)', en: 'Route prefixes' },
  'cx.routesHint': { th: 'บรรทัดละหนึ่ง เช่น /shop', en: 'One per line, e.g. /shop' },
  'cx.exports': { th: 'ฟังก์ชันที่ให้ปลั๊กอินอื่นเรียก (exports)', en: 'Functions other plugins may call (exports)' },
  'cx.library': { th: 'ไฟล์ไลบรารี (ไม่บังคับ)', en: 'Library file (optional)' },
  'cx.warnings': { th: 'คำเตือน', en: 'Warnings' },
  'cx.recheck': { th: 'ตรวจอีกครั้ง', en: 'Check again' },
  'cx.download': { th: 'ดาวน์โหลด .zip', en: 'Download .zip' },
  'cx.downloading': { th: 'กำลังสร้าง…', en: 'Building…' },
  'cx.done': { th: 'ดาวน์โหลดปลั๊กอินแล้ว', en: 'Plugin downloaded' },
  'cx.err.id': { th: 'รหัสต้องเป็น a-z 0-9 และ - ยาว 3–40 ตัว ขึ้นต้นด้วยตัวอักษรหรือตัวเลข', en: 'Use 3–40 of a-z, 0-9 and -, starting with a letter or digit' },
  'cx.err.reserved': { th: 'ใช้รหัส "site" ไม่ได้', en: 'The id "site" is reserved' },
  'cx.err.name': { th: 'ต้องมีชื่อ (ไม่เกิน 80 ตัว)', en: 'A name is required (up to 80 chars)' },
  'cx.err.version': { th: 'เวอร์ชันต้องเป็นแบบ 1.0.0', en: 'Use a version like 1.0.0' },
  'cx.err.route': { th: 'route ต้องขึ้นต้นด้วย / และไม่ใช่ที่อยู่ของระบบ: {r}', en: 'Routes must start with / and not be a system path: {r}' },
  'cx.err.description': { th: 'คำอธิบายยาวเกิน 500 ไบต์ที่ Plugin Store รับ (ภาษาไทยประมาณ 160 ตัวอักษร)', en: 'Description is longer than the 500 bytes the Plugin Store accepts' },
  'cx.noLive': { th: 'ยังไม่มีเวอร์ชันที่เผยแพร่ — เผยแพร่ก่อนจึงส่งออกได้', en: 'Nothing is published yet — publish first to export' },

  // designer source groups
  'sc.col.grpUnit': { th: 'ปลั๊กอิน JS: {name}', en: 'JS plugin: {name}' },
});
