import { registerMessages } from '../core/messages';

/** Designer properties of CMS 2.2.2 (what Figma has): min W / max H, aspect, scroll, blend, max lines, stroke sides. */
registerMessages({
  'ed.pp.minW': { th: 'กว้างอย่างน้อย (px)', en: 'Minimum width (px)' },
  'ed.pp.maxH': { th: 'สูงไม่เกิน (px)', en: 'Maximum height (px)' },
  'ed.pp.aspect': { th: 'สัดส่วนกว้าง:สูง', en: 'Aspect ratio' },
  'ed.pp.aspectTip': { th: 'คงสัดส่วนเมื่อกล่องยืดหด (เช่น รูป 16:9) — ใช้กับความสูงแบบพอดีเนื้อหาหรือเต็มพื้นที่', en: 'Keeps the ratio while the layer resizes (e.g. a 16:9 image) — with a hug or fill height' },
  'ed.pp.aspectFree': { th: 'อิสระ', en: 'Free' },
  'ed.pp.scroll': { th: 'เลื่อนดูในกล่อง', en: 'Scroll inside' },
  'ed.pp.scrollTip': { th: 'เนื้อหาที่ล้นเลื่อนดูได้ในกล่องนี้ (เช่น แถวการ์ดเลื่อนแนวนอนบนมือถือ)', en: 'Overflowing content scrolls inside this layer (e.g. a row of cards on phones)' },
  'ed.pp.scrollNone': { th: 'ไม่เลื่อน', en: 'No scroll' },
  'ed.pp.scrollX': { th: 'แนวนอน', en: 'Horizontal' },
  'ed.pp.scrollY': { th: 'แนวตั้ง', en: 'Vertical' },
  'ed.pp.scrollBoth': { th: 'ทั้งสองแนว', en: 'Both' },
  'ed.pp.blend': { th: 'โหมดผสมสี (blend)', en: 'Blend mode' },
  'ed.pp.blendNormal': { th: 'ปกติ', en: 'Normal' },
  'ed.pp.maxLines': { th: 'แสดงไม่เกิน (บรรทัด)', en: 'Maximum lines' },
  'ed.pp.maxLinesTip': { th: 'ข้อความที่ยาวกว่านี้ถูกตัดและต่อท้ายด้วย … (เช่น ชื่อบทความในการ์ด)', en: 'Longer text is cut with … (e.g. post titles on cards)' },
  'ed.pp.maxLinesAll': { th: 'ทั้งหมด', en: 'All' },
  'fx.strokeSides': { th: 'ด้านที่มีเส้นขอบ', en: 'Stroke sides' },
  'fx.sideTop': { th: 'ขอบบน', en: 'Top' },
  'fx.sideRight': { th: 'ขอบขวา', en: 'Right' },
  'fx.sideBottom': { th: 'ขอบล่าง', en: 'Bottom' },
  'fx.sideLeft': { th: 'ขอบซ้าย', en: 'Left' },
});
