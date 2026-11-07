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
