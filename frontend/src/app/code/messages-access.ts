import { registerMessages } from '../core/messages';

/** Messages of the Site Code access screen (/code/access). */
registerMessages({
  'code.tab.access': { th: 'สิทธิ์การเข้าถึง', en: 'Access' },
  'ca.title': { th: 'สิทธิ์การเข้าถึงโค้ด', en: 'Code access' },
  'ca.desc': { th: 'ใครเรียกใช้แต่ละ route / collection / action / หน้า / ตาราง / หน้าผู้ดูแล ได้ — ค่าเริ่มต้นมาจาก access ในโค้ด ติ๊กทับได้โดยไม่ต้องแก้โค้ด', en: 'Who may use each route / collection / action / page / table / admin page — defaults come from access in the code; tick over them without editing code' },
  'ca.search': { th: 'ค้นหาชื่อ…', en: 'Search names…' },
  'ca.reload': { th: 'โหลดใหม่', en: 'Reload' },
  'ca.col.name': { th: 'ชื่อ', en: 'Name' },
  'ca.col.everyone': { th: 'ทุกคน (ไม่ล็อกอิน)', en: 'Everyone (not signed in)' },
  'ca.kind.route': { th: 'Route (API / หน้าเว็บของโค้ด)', en: 'Routes' },
  'ca.kind.collection': { th: 'Collection (ข้อมูลให้ designer)', en: 'Collections' },
  'ca.kind.action': { th: 'Action (ปุ่ม / ฟอร์ม)', en: 'Actions' },
  'ca.kind.page': { th: 'ข้อมูลระดับหน้า (site.page)', en: 'Page data (site.page)' },
  'ca.kind.table': { th: 'ตาราง (ผูกตรงกับ designer)', en: 'Tables (bound directly in the designer)' },
  'ca.overridden': { th: 'ปรับโดยแอดมิน', en: 'Changed by admin' },
  'ca.reset': { th: 'กลับเป็นค่าในโค้ด', en: 'Reset to code default' },
  'ca.codeDefault': { th: 'ค่าในโค้ด', en: 'Code default' },
  'ca.nobody': { th: 'ไม่มีใคร', en: 'Nobody' },
  'ca.adminAlways': { th: 'ผู้ดูแลระบบใช้ได้เสมอ', en: 'Administrators always have access' },
  'ca.implied': { th: 'รวมอยู่ใน “ทุกคน” แล้ว', en: 'Included in “Everyone”' },
  'ca.empty': { th: 'โค้ดที่เผยแพร่อยู่ยังไม่ได้ประกาศ route, collection, action, page หรือตาราง', en: 'The live code declares no routes, collections, actions, pages or tables yet' },
  'ca.hint': { th: 'ไม่ระบุ access ในโค้ด = ผู้ดูแลระบบเท่านั้น (ข้อมูลระดับหน้า = ทุกคน, ตาราง = ไม่มีใคร) · server ตรวจทุกครั้งก่อนเรียกโค้ด', en: 'No access in the code = administrators only (page data = everyone, tables = nobody) · checked on the server before every call' },
});
