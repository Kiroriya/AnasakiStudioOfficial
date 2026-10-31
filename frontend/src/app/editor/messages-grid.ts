import { registerMessages } from '../core/messages';

/** Designer messages for rulers, guides and layout grids (rulers.ts, layout-grid.ts). */
registerMessages({
  'lg.title': { th: 'กริดเลย์เอาต์', en: 'Layout grid' },
  'lg.add': { th: 'เพิ่มกริดเลย์เอาต์ (แสดงเฉพาะในตัวออกแบบ)', en: 'Add a layout grid (shown in the designer only)' },
  'lg.remove': { th: 'นำกริดออก', en: 'Remove grid' },
  'lg.show': { th: 'แสดง/ซ่อนกริดนี้', en: 'Show/hide this grid' },
  'lg.p12': { th: '12 คอลัมน์', en: '12 columns' },
  'lg.p4': { th: '4 คอลัมน์', en: '4 columns' },
  'lg.p8': { th: 'กริด 8px', en: '8px grid' },
  'lg.none': { th: 'ไม่มี', en: 'None' },
  'lg.columns': { th: 'คอลัมน์', en: 'Columns' },
  'lg.rows': { th: 'แถว', en: 'Rows' },
  'lg.grid': { th: 'ตาราง', en: 'Grid' },
  'lg.count': { th: 'จำนวน', en: 'Count' },
  'lg.gutter': { th: 'ระยะห่างระหว่างคอลัมน์ (Gutter)', en: 'Gutter' },
  'lg.margin': { th: 'ระยะขอบ (Margin)', en: 'Margin' },
  'lg.size': { th: 'ขนาดช่อง', en: 'Cell size' },
  'lg.color': { th: 'สีกริด', en: 'Grid colour' },
  'lg.hint': { th: 'กริดช่วยจัดวาง แสดงเฉพาะตอนออกแบบ ไม่แสดงบนเว็บจริง · ลากชิ้นงานจะดูดติดขอบคอลัมน์', en: 'A guide for placing layers: shown only while designing, never on the site. Moved layers snap to column edges.' },
  'lg.hidden': { th: 'กริดทั้งหมดถูกซ่อนอยู่ — เปิดที่ View → Layout grids (Ctrl+Shift+4)', en: 'All layout grids are hidden: turn them on in View → Layout grids (Ctrl+Shift+4)' },
  'gd.removeHint': { th: 'ลากกลับไปที่ไม้บรรทัดเพื่อลบ', en: 'Drag back onto the ruler to remove' },
  'gd.guide': { th: 'เส้นไกด์ — ลากเพื่อย้าย, ลากกลับไปที่ไม้บรรทัดหรือกด Delete เพื่อลบ', en: 'Guide: drag to move; drag onto the ruler or press Delete to remove' },
  'gd.ruler': { th: 'ลากออกมาเพื่อสร้างเส้นไกด์', en: 'Drag out to create a guide' },
});
