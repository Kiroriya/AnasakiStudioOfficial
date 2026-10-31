// thaiDate: dates in Thai (พ.ศ., Thai month and weekday names), Asia/Bangkok time (UTC+7, no DST).

export const MONTHS = ['มกราคม', 'กุมภาพันธ์', 'มีนาคม', 'เมษายน', 'พฤษภาคม', 'มิถุนายน', 'กรกฎาคม', 'สิงหาคม', 'กันยายน', 'ตุลาคม', 'พฤศจิกายน', 'ธันวาคม'];
export const MONTHS_SHORT = ['ม.ค.', 'ก.พ.', 'มี.ค.', 'เม.ย.', 'พ.ค.', 'มิ.ย.', 'ก.ค.', 'ส.ค.', 'ก.ย.', 'ต.ค.', 'พ.ย.', 'ธ.ค.'];
export const WEEKDAYS = ['อาทิตย์', 'จันทร์', 'อังคาร', 'พุธ', 'พฤหัสบดี', 'ศุกร์', 'เสาร์'];
export const WEEKDAYS_SHORT = ['อา.', 'จ.', 'อ.', 'พ.', 'พฤ.', 'ศ.', 'ส.'];

/** Named formats. */
export const DATE_FORMATS: Record<string, string> = {
  short: 'D MMM BB',                              // 30 ต.ค. 69
  medium: 'D MMM BBBB',                           // 30 ต.ค. 2569
  long: 'D MMMM BBBB',                            // 30 ตุลาคม 2569
  full: '[วัน]dddd[ที่] D MMMM [พ.ศ.] BBBB',        // วันศุกร์ที่ 30 ตุลาคม พ.ศ. 2569
  datetime: 'D MMM BBBB HH:mm',                   // 30 ต.ค. 2569 14:05
  time: 'HH:mm[ น.]',                             // 14:05 น.
};

const BANGKOK_MS = 7 * 3600 * 1000;

interface Parts { y: number; mo: number; d: number; h: number; mi: number; s: number; wd: number }

/** Accepts a Date, an ISO string, "YYYY-MM-DD" (never shifted) or Unix seconds. null when invalid. */
export function dateParts(value: unknown): Parts | null {
  if (value === null || value === undefined || value === '') return null;
  if (typeof value === 'string') {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim());
    if (m) {
      const y = +m[1], mo = +m[2], d = +m[3];
      const wd = new Date(Date.UTC(y, mo - 1, d)).getUTCDay();
      if (mo < 1 || mo > 12 || d < 1 || d > 31) return null;
      return { y, mo, d, h: 0, mi: 0, s: 0, wd };
    }
  }
  let ms: number;
  if (value instanceof Date) ms = value.getTime();
  else if (typeof value === 'number') ms = value * 1000;
  else if (typeof value === 'string' && /^\d+$/.test(value.trim())) ms = Number(value) * 1000;
  else {
    let s = String(value).trim();
    // "2026-10-30 14:05:00" (MySQL style, no zone) is read as UTC like the query builder returns it
    if (/^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(:\d{2}(\.\d+)?)?$/.test(s)) s = s.replace(' ', 'T') + 'Z';
    ms = Date.parse(s);
  }
  if (!Number.isFinite(ms)) return null;
  const t = new Date(ms + BANGKOK_MS);
  return { y: t.getUTCFullYear(), mo: t.getUTCMonth() + 1, d: t.getUTCDate(), h: t.getUTCHours(), mi: t.getUTCMinutes(), s: t.getUTCSeconds(), wd: t.getUTCDay() };
}

const pad = (n: number, w = 2) => String(n).padStart(w, '0');

/**
 * Formats a date in Thai. fmt: a named format (short, medium, long, full, datetime, time) or tokens
 * D DD M MM MMM MMMM YYYY YY (ค.ศ.) BBBB BB (พ.ศ.) ddd dddd H HH m mm s ss and [literal text].
 * Default 'long' (30 ตุลาคม 2569). Invalid dates → ''. วันที่แบบไทย
 */
export function thaiDate(value: unknown, fmt: string = 'long'): string {
  const p = dateParts(value);
  if (!p) return '';
  const f = DATE_FORMATS[fmt] ?? fmt;
  const be = p.y + 543;
  return f.replace(/\[([^\]]*)]|BBBB|BB|YYYY|YY|MMMM|MMM|MM|M|DD|D|dddd|ddd|HH|H|mm|m|ss|s/g, (tok, lit) => {
    if (lit !== undefined) return lit;
    switch (tok) {
      case 'BBBB': return String(be);
      case 'BB': return pad(be % 100);
      case 'YYYY': return String(p.y);
      case 'YY': return pad(p.y % 100);
      case 'MMMM': return MONTHS[p.mo - 1];
      case 'MMM': return MONTHS_SHORT[p.mo - 1];
      case 'MM': return pad(p.mo);
      case 'M': return String(p.mo);
      case 'DD': return pad(p.d);
      case 'D': return String(p.d);
      case 'dddd': return WEEKDAYS[p.wd];
      case 'ddd': return WEEKDAYS_SHORT[p.wd];
      case 'HH': return pad(p.h);
      case 'H': return String(p.h);
      case 'mm': return pad(p.mi);
      case 'm': return String(p.mi);
      case 'ss': return pad(p.s);
      default: return String(p.s);
    }
  });
}
