import { registerMessages } from '../core/messages';

/** 2.5.0: posts sent for review. */
registerMessages({
  'post.pendingBadge': { th: 'รอตรวจ', en: 'Pending review' },
  'post.submitReview': { th: 'ส่งให้ตรวจ', en: 'Submit for review' },
  'post.updateReview': { th: 'บันทึกและส่งให้ตรวจ', en: 'Save and keep in review' },
  'post.approve': { th: 'อนุมัติและเผยแพร่', en: 'Approve and publish' },
  'post.pendingMsg': { th: 'ส่งให้ตรวจแล้ว — บรรณาธิการจะได้รับแจ้ง', en: 'Sent for review — the editors are told' },
  'dash.pending': { th: 'บทความรอตรวจ', en: 'Posts waiting for review' },
});

/** 2.5.0: multilingual content. */
registerMessages({
  'settings.languages': { th: 'ภาษาเพิ่มเติมของเนื้อหา', en: 'More content languages' },
  'settings.languagesHint': { th: 'เปิดแล้วแปลหน้าและบทความได้ (เมนู ⋯ → แปลเป็น…) ฉบับแปลอยู่ที่ /en/… และมี hreflang ให้ Google — ใส่บล็อก “เลือกภาษา” ในส่วนหัวเว็บ', en: 'Then translate pages and posts (⋯ → Translate to…); translations live at /en/… with hreflang for search engines — add the “Language switch” block to the site header' },
  'settings.langParts': { th: 'ส่วนหัว / ส่วนท้ายของ {lang}', en: 'Header / footer for {lang}' },
  'settings.langHeader': { th: 'ส่วนหัวเว็บของภาษานี้', en: 'Site header in this language' },
  'settings.langHeaderSame': { th: 'ส่วนหัว: ใช้อันเดียวกับภาษาหลัก', en: 'Header: same as the main language' },
  'settings.langFooter': { th: 'ส่วนท้ายเว็บของภาษานี้', en: 'Site footer in this language' },
  'settings.langFooterSame': { th: 'ส่วนท้าย: ใช้อันเดียวกับภาษาหลัก', en: 'Footer: same as the main language' },
  'lang.filter': { th: 'ภาษา', en: 'Language' },
  'lang.all': { th: 'ทุกภาษา', en: 'All languages' },
  'lang.translateTo': { th: 'แปลเป็น {lang}', en: 'Translate to {lang}' },
  'lang.title': { th: 'ภาษา', en: 'Languages' },
  'lang.thisIs': { th: 'ฉบับนี้: {lang}', en: 'This version: {lang}' },
});

/** 2.5.3: each user arranges the plugin widgets of their dashboard. */
registerMessages({
  'dash.widgets': { th: 'วิดเจ็ตจากปลั๊กอิน', en: 'Plugin widgets' },
  'dash.customize': { th: 'ปรับแต่งหน้าหลัก', en: 'Customize' },
  'dash.customizeDone': { th: 'เสร็จแล้ว', en: 'Done' },
  'dash.customizeHint': { th: 'ลากการ์ดเพื่อสลับที่ หรือใช้ปุ่ม ↑ ↓ — ซ่อนการ์ดที่ไม่ใช้ เลือกขนาดได้ ตั้งค่าเป็นของคุณคนเดียวและตามไปทุกเครื่อง', en: 'Drag cards (or use ↑ ↓) to reorder, hide what you do not use, pick a size — saved to your account on every device' },
  'dash.reset': { th: 'คืนค่าเริ่มต้น', en: 'Reset' },
  'dash.hidden': { th: 'ซ่อนอยู่', en: 'Hidden' },
  'dash.hiddenCount': { th: 'ซ่อน {n} การ์ด', en: '{n} hidden' },
  'dash.show': { th: 'แสดง', en: 'Show' },
  'dash.hide': { th: 'ซ่อน', en: 'Hide' },
  'dash.up': { th: 'เลื่อนขึ้น', en: 'Move up' },
  'dash.down': { th: 'เลื่อนลง', en: 'Move down' },
  'dash.size': { th: 'ขนาด', en: 'Size' },
  'dash.size.auto': { th: 'อัตโนมัติ', en: 'Auto' },
  'dash.size.normal': { th: 'ปกติ', en: 'Normal' },
  'dash.size.wide': { th: 'กว้าง', en: 'Wide' },
  'dash.saveFailed': { th: 'บันทึกการจัดหน้าไม่สำเร็จ', en: 'Could not save the layout' },
});
