import { registerMessages } from '../core/messages';

/** Messages of the Plugins page for JavaScript plugins (Site Code phase 4b). */
registerMessages({
  'perm.code': { th: 'รันโค้ด JavaScript บนเซิร์ฟเวอร์ของเว็บ (ตามขอบเขตด้านล่าง)', en: 'Run JavaScript code on the site server (within the scopes below)' },

  // runtime / card
  'pjs.runtime': { th: 'ปลั๊กอิน JS', en: 'JS plugin' },
  'pjs.runtimeHint': { th: 'ปลั๊กอินที่เขียนด้วย JavaScript/TypeScript รันโดยเอนจินเดียวกับโค้ดของเว็บ', en: 'A JavaScript/TypeScript plugin run by the same engine as the Site Code' },
  'pjs.requires': { th: 'ต้องใช้ปลั๊กอิน', en: 'Requires' },
  'pjs.dependents': { th: 'ปลั๊กอินที่ใช้ตัวนี้', en: 'Required by' },
  'pjs.scopes': { th: 'สิทธิ์ของโค้ด', en: 'Code permissions' },
  'pjs.activationError': { th: 'เปิดใช้ไม่สำเร็จ', en: 'Could not activate' },
  'pjs.dep.ok': { th: 'ติดตั้งแล้ว', en: 'Installed' },
  'pjs.dep.missing': { th: 'ยังไม่ได้ติดตั้ง', en: 'Not installed' },
  'pjs.dep.incompatible': { th: 'เวอร์ชันไม่ตรง (มี v{v})', en: 'Wrong version (v{v} installed)' },
  'pjs.dep.inactive': { th: 'ติดตั้งแล้วแต่ปิดอยู่', en: 'Installed but off' },
  // optionalRequires (phase 6) — informational, never blocking
  'pjs.worksWith': { th: 'ใช้ร่วมกับ', en: 'Works with' },
  'pjs.worksWithHint': { th: 'ไม่บังคับ — ปลั๊กอินทำงานได้โดยไม่ต้องมีตัวเหล่านี้ แต่จะมีความสามารถเพิ่มเมื่อติดตั้ง', en: 'Optional — the plugin works without these, but does more when they are installed' },
  'pjs.opt.ok': { th: 'ติดตั้งแล้ว', en: 'Installed' },
  'pjs.opt.inactive': { th: 'ติดตั้งแล้วแต่ปิดอยู่', en: 'Installed but off' },
  'pjs.opt.missing': { th: 'ยังไม่ได้ติดตั้ง (ไม่บังคับ)', en: 'Not installed (optional)' },
  'pjs.opt.incompatible': { th: 'เวอร์ชันอาจไม่เข้ากัน (มี v{v}) — ใช้ร่วมกันไม่ได้จนกว่าจะอัปเดต', en: 'Version may not match (v{v} installed) — not used together until updated' },
  'pjs.dep.install': { th: 'ติดตั้ง', en: 'Install' },
  'pjs.dep.update': { th: 'อัปเดต', en: 'Update' },
  'pjs.dep.activate': { th: 'เปิดใช้', en: 'Activate' },

  // scopes
  'scope.db': { th: 'ตารางข้อมูลของตัวเองและ migration', en: 'Its own database tables and migrations' },
  'scope.content': { th: 'อ่านบทความและหน้าเว็บ', en: 'Read posts and pages' },
  'scope.content-write': { th: 'สร้าง แก้ไข และลบบทความ/หน้าเว็บ (รวมถึงในสิทธิ์ระบบ)', en: 'Create, edit and delete posts/pages (also with system rights)' },
  'scope.mail': { th: 'ส่งอีเมลจากเว็บ', en: 'Send email from the site' },
  'scope.http': { th: 'เรียกเว็บภายนอก (เฉพาะโดเมนที่อนุญาต)', en: 'Call external websites (allowed hosts only)' },
  'scope.visitor': { th: 'ระบุตัวตนผู้เยี่ยมชม/สมาชิกของเว็บ', en: 'Identify site visitors / members' },
  'scope.admin-pages': { th: 'เพิ่มหน้าในหลังบ้าน', en: 'Add admin pages' },
  'scope.jobs': { th: 'งานเบื้องหลังและงานตามเวลา (cron)', en: 'Background jobs and schedules (cron)' },
  'scope.routes': { th: 'เพิ่มหน้า/ที่อยู่สาธารณะบนเว็บ', en: 'Add public routes to the site' },
  'scope.filters-core': { th: 'แก้ไขข้อมูลของระบบผ่านตัวกรองหลัก (เนื้อหา, head, อีเมล…)', en: 'Change core data through core filters (content, head, mail…)' },
  'scope.content-fields': { th: 'เพิ่มช่องข้อมูลในหน้าแก้ไขโพสต์/หน้า (บันทึกใน post meta ของปลั๊กอินเอง)', en: "Add fields to the post/page editor (stored in the plugin's own post meta)" },
  'scope.post-types': { th: 'เพิ่มประเภทเนื้อหาใหม่ (เมนูและหน้าแก้ไขในหลังบ้าน)', en: 'Add content types (admin menu and editor)' },
  'scope.users': { th: 'อ่านโปรไฟล์สาธารณะของผู้ใช้ (ชื่อ รูป ประวัติ — ไม่รวมอีเมล)', en: "Read users' public profiles (name, avatar, bio — never the email)" },
  'scope.taxonomies': { th: 'เพิ่มชุดหมวดหมู่ใหม่ (เช่น หมวดสินค้า) พร้อมหน้าจัดการในหลังบ้าน', en: 'Add taxonomies (e.g. product categories) with admin screens' },
  'scope.terms-write': { th: 'สร้าง แก้ไข และลบหมวดหมู่/แท็ก', en: 'Create, edit and delete categories and terms' },
  'scope.media': { th: 'อ่านคลังสื่อ (รูปและไฟล์ที่อัปโหลด)', en: 'Read the media library' },
  'scope.media-write': { th: 'อัปโหลด แก้ไข และลบไฟล์ในคลังสื่อ', en: 'Upload, edit and delete media files' },
  'scope.files': { th: 'เก็บไฟล์ของปลั๊กอินเอง (มีโควตา)', en: "Store the plugin's own files (with a quota)" },
  'scope.webhooks': { th: 'ส่ง webhook ไปยัง URL ใดก็ได้ (เฉพาะที่อยู่สาธารณะ)', en: 'Send webhooks to any public URL' },
  'scope.sensitive': { th: 'สิทธิ์สำคัญ — ตรวจสอบให้แน่ใจว่าเชื่อถือผู้พัฒนา', en: 'Sensitive — make sure you trust the developer' },
  'pjs.hosts': { th: 'โดเมนที่เรียกได้ (ต้องอยู่ในรายการที่อนุญาตของเว็บด้วย)', en: 'Hosts it may call (must also be on the site allowlist)' },
  'pjs.routes': { th: 'ที่อยู่บนเว็บที่ใช้', en: 'Site paths it uses' },

  // consent dialog
  'pjs.consent.install': { th: 'ติดตั้ง "{name}"?', en: 'Install "{name}"?' },
  'pjs.consent.upgrade': { th: 'อัปเดต "{name}"?', en: 'Update "{name}"?' },
  'pjs.consent.reconsent': { th: 'อนุมัติสิทธิ์ใหม่ของ "{name}"?', en: 'Approve the new permissions of "{name}"?' },
  'pjs.consent.activate': { th: 'เปิดใช้ "{name}"?', en: 'Activate "{name}"?' },
  'pjs.consent.intro': { th: 'ปลั๊กอินนี้ขอสิทธิ์ต่อไปนี้:', en: 'This plugin asks for:' },
  'pjs.consent.introRe': { th: 'เวอร์ชันนี้ขอสิทธิ์เพิ่มจากที่อนุมัติไว้:', en: 'This version asks for more than you approved:' },
  'pjs.consent.perms': { th: 'สิทธิ์ของปลั๊กอิน', en: 'Plugin permissions' },
  'pjs.consent.api': { th: 'สิทธิ์ API ของเซอร์วิส', en: 'Service API access' },
  'pjs.consent.none': { th: 'ไม่ขอสิทธิ์พิเศษ', en: 'No special permissions' },
  'pjs.consent.caps': { th: 'สิทธิ์ที่ปลั๊กอินเพิ่มให้ตำแหน่งผู้ใช้', en: 'Permissions this plugin adds for roles' },
  'pjs.consent.capsHint': { th: 'ปรับได้ภายหลังที่ ผู้ใช้ → ตำแหน่งและสิทธิ์ — ผู้ดูแลระบบได้ทุกข้อเสมอ', en: 'Change them later in Users → Roles — administrators always have all of them' },
  'pjs.consent.capDefault': { th: 'ค่าเริ่มต้น: {roles}', en: 'Default: {roles}' },
  'pjs.consent.migrations': { th: 'การติดตั้งนี้จะแก้ตารางข้อมูลแบบที่อาจทำให้ข้อมูลหาย:', en: 'This will change tables in a way that may lose data:' },
  'pjs.consent.destructiveConfirm': { th: 'เข้าใจแล้วว่าข้อมูลอาจหาย — ระบบจะสำรองตารางก่อน', en: 'I understand data may be lost — a backup copy will be made' },
  'pjs.consent.approve': { th: 'อนุมัติและติดตั้ง', en: 'Approve & install' },
  'pjs.consent.approveUpgrade': { th: 'อนุมัติและอัปเดต', en: 'Approve & update' },
  'pjs.consent.approveActivate': { th: 'อนุมัติและเปิดใช้', en: 'Approve & activate' },

  // dependencies dialog
  'pjs.deps.title': { th: '"{name}" ต้องใช้ปลั๊กอินอื่นก่อน', en: '"{name}" needs other plugins first' },
  'pjs.deps.intro': { th: 'ติดตั้งปลั๊กอินเหล่านี้ก่อน (แต่ละตัวจะถามสิทธิ์แยกกัน):', en: 'Install these first (each one asks for its own permissions):' },
  'pjs.deps.range': { th: 'ต้องการ {range}', en: 'needs {range}' },
  'pjs.deps.latest': { th: 'ในสโตร์: v{v}', en: 'in store: v{v}' },
  'pjs.deps.installAll': { th: 'ติดตั้งทั้งหมด', en: 'Install all' },
  'pjs.deps.failed': { th: 'ติดตั้ง "{id}" ไม่สำเร็จ — หยุดการติดตั้ง', en: 'Installing "{id}" failed — stopped' },
  'pjs.deps.tooDeep': { th: 'ปลั๊กอินที่ต้องใช้ซ้อนกันลึกเกินไป', en: 'Dependencies are nested too deeply' },

  // deactivate with dependents
  'pjs.off.dependentsTitle': { th: 'ปิด "{name}" แล้วปลั๊กอินที่ใช้ตัวนี้จะถูกปิดด้วย', en: 'Turning "{name}" off also turns off the plugins that need it' },
  'pjs.off.force': { th: 'ปิดทั้งหมด', en: 'Turn all off' },

  // uninstall dialog
  'pjs.del.title': { th: 'ลบปลั๊กอิน "{name}"?', en: 'Delete plugin "{name}"?' },
  'pjs.del.text': { th: 'ไฟล์ของปลั๊กอินจะถูกลบจากเว็บ', en: 'The plugin files are removed from the site' },
  'pjs.del.dependents': { th: 'ปลั๊กอินเหล่านี้ต้องใช้ตัวนี้ และจะถูกปิด:', en: 'These plugins need it and will be turned off:' },
  'pjs.del.deleteData': { th: 'ลบข้อมูลของปลั๊กอินด้วย (ตาราง p_{id}_* — สำรองไว้ก่อนลบ)', en: 'Also delete the plugin\'s data (tables p_{id}_* — backed up first)' },
  'pjs.del.keepData': { th: 'ถ้าไม่เลือก ข้อมูลจะถูกเก็บไว้และใช้ต่อได้เมื่อติดตั้งใหม่', en: 'If unticked, the data is kept and reused when you reinstall' },
  'pjs.del.confirm': { th: 'ลบปลั๊กอิน', en: 'Delete plugin' },
  'pjs.del.done': { th: 'ลบปลั๊กอินแล้ว', en: 'Plugin deleted' },
  'pjs.del.doneData': { th: 'ลบปลั๊กอินและข้อมูลแล้ว', en: 'Plugin and its data deleted' },
});
