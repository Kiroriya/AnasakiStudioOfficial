// bahtText: the amount in Thai words, like Excel's BAHTTEXT.
//   21        → ยี่สิบเอ็ดบาทถ้วน
//   1000001   → หนึ่งล้านเอ็ดบาทถ้วน
//   12.5      → สิบสองบาทห้าสิบสตางค์
// Satang are rounded half-up to 2 decimals on the decimal digits (no
// floating-point drift: 1.005 → 1.01). Any size works (BigInt), ล้าน repeats
// for every 6 digits (ล้านล้าน).

const DIGITS = ['ศูนย์', 'หนึ่ง', 'สอง', 'สาม', 'สี่', 'ห้า', 'หก', 'เจ็ด', 'แปด', 'เก้า'];
const PLACES = ['', 'สิบ', 'ร้อย', 'พัน', 'หมื่น', 'แสน'];

/** Reads up to 6 digits ("000001" ok). hasHigher: a non-zero digit stands before this group. */
function readGroup(digits: string, hasHigher: boolean): string {
  let out = '';
  const n = digits.length;
  for (let i = 0; i < n; i++) {
    const d = digits.charCodeAt(i) - 48;
    const place = n - 1 - i;
    if (d === 0) continue;
    if (place === 0) {
      // เอ็ด: a final 1 after any other non-zero digit of the whole number (11, 101, 1000001)
      const higherHere = digits.slice(0, i).replace(/0/g, '') !== '';
      out += d === 1 && (higherHere || hasHigher) ? 'เอ็ด' : DIGITS[d];
    } else if (place === 1) {
      out += (d === 1 ? '' : d === 2 ? 'ยี่' : DIGITS[d]) + 'สิบ';
    } else {
      out += DIGITS[d] + PLACES[place];
    }
  }
  return out;
}

/** Reads a non-negative integer given as a digit string ("0" → ศูนย์). */
export function thaiInteger(digits: string): string {
  digits = digits.replace(/^0+(?=\d)/, '');
  if (!/^\d+$/.test(digits)) throw new Error('thaiInteger: digits only');
  if (/^0+$/.test(digits)) return DIGITS[0];
  return readMillions(digits);
}

// n = high × 1,000,000 + low → read(high) + ล้าน + read(low), so 10^12 = หนึ่งล้านล้าน and
// 1,000,001,000,000 = หนึ่งล้านเอ็ดล้าน.
function readMillions(digits: string): string {
  if (digits.length <= 6) return readGroup(digits, false);
  const high = digits.slice(0, -6);
  const low = digits.slice(-6);
  return readMillions(high) + 'ล้าน' + readGroup(low, true);
}

/** Splits a number (or numeric string) into [negative, integer digits, 2 satang digits], half-up. */
export function splitAmount(value: number | string | bigint): [boolean, string, string] {
  let s: string;
  if (typeof value === 'bigint') s = value.toString();
  else if (typeof value === 'number') {
    if (!Number.isFinite(value)) throw new Error('bahtText: not a finite number');
    s = Math.abs(value) >= 1e21 ? BigInt(Math.trunc(value)).toString() : decimalString(value);
  } else s = String(value).trim().replace(/[,_\s฿]/g, '');
  const m = /^([+-])?(\d*)(?:\.(\d*))?$/.exec(s);
  if (!m || (m[2] === '' && (m[3] ?? '') === '')) throw new Error('bahtText: not a number: ' + value);
  const neg = m[1] === '-';
  let int = m[2] || '0';
  const frac = (m[3] ?? '').padEnd(3, '0');
  let satang = Number(frac.slice(0, 2));
  if (frac.charCodeAt(2) - 48 >= 5) satang++;
  if (satang === 100) {
    satang = 0;
    int = (BigInt(int) + 1n).toString();
  }
  int = int.replace(/^0+(?=\d)/, '');
  const zero = /^0+$/.test(int) && satang === 0;
  return [neg && !zero, int, String(satang).padStart(2, '0')];
}

/** A number as a plain decimal string (no exponent): 1e-7 → "0.0000001". */
function decimalString(n: number): string {
  const s = String(Math.abs(n));
  const sign = n < 0 ? '-' : '';
  const e = /^(\d)(?:\.(\d+))?e([+-]\d+)$/.exec(s);
  if (!e) return sign + s;
  const digits = e[1] + (e[2] ?? '');
  const exp = Number(e[3]);
  if (exp < 0) return sign + '0.' + '0'.repeat(-exp - 1) + digits;
  return sign + digits.padEnd(exp + 1, '0');
}

/**
 * The amount in Thai words: บาท, สตางค์ (rounded half-up to 2 decimals) and ถ้วน when there are no
 * satang; negative amounts start with ลบ. จำนวนเงินเป็นตัวหนังสือ
 */
export function bahtText(value: number | string | bigint): string {
  const [neg, int, sat] = splitAmount(value);
  const satang = Number(sat);
  let out = '';
  if (int !== '0' || satang === 0) out = thaiInteger(int) + 'บาท';
  out += satang === 0 ? 'ถ้วน' : readGroup(sat.replace(/^0/, ''), false) + 'สตางค์';
  return (neg ? 'ลบ' : '') + out;
}
