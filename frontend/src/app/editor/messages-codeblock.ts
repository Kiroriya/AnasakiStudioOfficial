import { registerMessages } from '../core/messages';

/** Messages of the "Frontend code" designer block (Site Code phase 5). */
registerMessages({
  'ed.node.codeblock': { th: 'โค้ดหน้าเว็บ', en: 'Frontend code' },
  'ed.help.codeblock': {
    th: 'HTML, CSS และ JavaScript ของคุณเอง — CSS มีผลเฉพาะในบล็อกนี้ ส่วน JavaScript ทำงานบนหน้าเว็บจริงเท่านั้น',
    en: 'Your own HTML, CSS and JavaScript — the CSS stays inside this block; the JavaScript runs on the published page only',
  },
  'cb.insTip': { th: 'บล็อก HTML/CSS/JS ของคุณเอง (ต้องมีสิทธิ์แก้โค้ดของเว็บไซต์)', en: 'Your own HTML/CSS/JS block (needs permission to edit site code)' },
  'cb.def.html': { th: '<div class="hello">สวัสดีจากโค้ดหน้าเว็บ</div>', en: '<div class="hello">Hello from frontend code</div>' },
  'cb.def.js': { th: '// el = บล็อกนี้, anasaki = ตัวช่วยของเว็บ (call, get, post, refresh, on)\n', en: '// el = this block, anasaki = site helpers (call, get, post, refresh, on)\n' },
  'cb.title': { th: 'โค้ดหน้าเว็บ', en: 'Frontend code' },
  'cb.expand': { th: 'ขยาย', en: 'Expand' },
  'cb.expandTip': { th: 'เปิดตัวแก้โค้ดขนาดใหญ่ (Esc เพื่อย่อ)', en: 'Open the code editor large (Esc to shrink)' },
  'cb.collapse': { th: 'ย่อ', en: 'Shrink' },
  'cb.run': { th: 'ลองรันในหน้าตัวอย่าง', en: 'Run in preview' },
  'cb.runHint': { th: 'JavaScript ไม่ทำงานในตัวแก้ไข — เปิดหน้าตัวอย่างเพื่อลองรัน', en: 'JavaScript does not run in the editor — open the preview to try it' },
  'cb.cssHint': { th: 'CSS มีผลเฉพาะในบล็อกนี้ ใช้ :scope แทนตัวบล็อกเอง (@import ใช้ไม่ได้)', en: 'The CSS only applies inside this block; use :scope for the block itself (@import is not allowed)' },
  'cb.jsHint': { th: 'ทำงานแบบ module หลังบล็อกแสดงบนหน้า — ใช้ el (บล็อกนี้) และ anasaki ได้', en: 'Runs as a module once the block is on the page — use el (this block) and anasaki' },
  'cb.locked': { th: 'บล็อกนี้ล็อกอยู่ — เฉพาะผู้ที่มีสิทธิ์แก้โค้ดของเว็บไซต์ (edit_scripts) จึงแก้ไขได้', en: 'This block is locked — only people allowed to edit the site code (edit_scripts) can change it' },
  'cb.lockedBadge': { th: 'ล็อก', en: 'Locked' },
  'cb.loading': { th: 'กำลังโหลดตัวแก้โค้ด…', en: 'Loading the code editor…' },
  'cb.loadFail': { th: 'โหลดตัวแก้โค้ดไม่สำเร็จ:', en: 'The code editor failed to load:' },
  'cb.empty': { th: 'โค้ดหน้าเว็บ — เขียน HTML/CSS/JS ที่แผงด้านขวา', en: 'Frontend code — write the HTML/CSS/JS in the right panel' },
  'cb.hasJs': { th: 'มี JS (ทำงานบนเว็บจริง)', en: 'Has JS (runs on the site)' },
});
