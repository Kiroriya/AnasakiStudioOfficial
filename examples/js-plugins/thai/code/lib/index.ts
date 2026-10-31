// Library of the "thai" plugin: `import { bahtText, thaiDate } from 'thai'` bundles these files into
// the importer (they run with the importer's scopes — this library needs none).
import { bahtText } from './bahttext';
import { thaiDate } from './date';

export { bahtText, thaiInteger } from './bahttext';
export { thaiDate, dateParts, DATE_FORMATS, MONTHS, MONTHS_SHORT, WEEKDAYS, WEEKDAYS_SHORT } from './date';
export { isThaiPhone, normalizePhone, isThaiCitizenId, formatCitizenId } from './ids';

const THAI_DIGITS = '๐๑๒๓๔๕๖๗๘๙';

/** 1234.5 → "1,234.50" (decimals default 2; grouping with commas; half-up like toFixed). */
export function formatNumber(value: unknown, decimals = 2): string {
  const n = Number(value);
  if (!Number.isFinite(n)) return '';
  const [i, f] = Math.abs(n).toFixed(Math.max(0, Math.min(20, decimals))).split('.');
  return (n < 0 && Number(i + '.' + (f ?? '0')) !== 0 ? '-' : '') + i.replace(/\B(?=(\d{3})+(?!\d))/g, ',') + (f ? '.' + f : '');
}

/** "฿1,234.50" (negative: "-฿1,234.50"). / จำนวนเงินบาท */
export function baht(value: unknown, decimals = 2): string {
  const s = formatNumber(value, decimals);
  if (s === '') return '';
  return s.startsWith('-') ? '-฿' + s.slice(1) : '฿' + s;
}

/** Latin digits → Thai digits ("2569" → "๒๕๖๙"); numbers are grouped first (decimals optional). / เลขไทย */
export function thaiNumber(value: unknown, decimals?: number): string {
  let s: string;
  if (typeof value === 'number' || (decimals !== undefined && value !== null && value !== '' && Number.isFinite(Number(value)))) {
    const n = Number(value);
    s = decimals === undefined ? formatNumber(n, (String(n).split('.')[1] ?? '').length) : formatNumber(n, decimals);
  } else s = String(value ?? '');
  return s.replace(/[0-9]/g, (d) => THAI_DIGITS[d.charCodeAt(0) - 48]);
}

/** The binding formatters of the plugin by local name (registered by the thai unit itself). */
export function formatterFns(): Record<string, (value: any, ...args: any[]) => string> {
  return {
    baht: (v, decimals) => baht(v, decimals === undefined ? 2 : Number(decimals)),
    bahtText: (v) => (v === null || v === undefined || v === '' ? '' : bahtText(v)),
    thaiDate: (v, fmt) => thaiDate(v, fmt === undefined ? 'long' : String(fmt)),
    thaiNumber: (v, decimals) => thaiNumber(v, decimals === undefined ? undefined : Number(decimals)),
  };
}

/**
 * @deprecated Since phase 6 the thai plugin registers its formatters itself — use
 * `{{ x | thai:baht }}` (or enable the alias `| baht` on the Plugins page). Kept for compatibility:
 * it still registers `baht`, `bahtText`, `thaiDate`, `thaiNumber` (optionally prefixed) as formatters
 * of the **calling** unit — in Site Code that gives the old unprefixed names.
 * ลงทะเบียน formatter ในหน่วยที่เรียก (เลิกแนะนำ — ปลั๊กอินลงทะเบียนเองแล้ว)
 */
export function registerFormatters(prefix = ''): void {
  for (const [name, fn] of Object.entries(formatterFns())) site.formatter(prefix + name, fn);
}
