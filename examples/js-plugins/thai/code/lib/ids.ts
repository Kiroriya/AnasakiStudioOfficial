// Thai phone numbers and citizen IDs.

const digitsOf = (s: unknown) => String(s ?? '').replace(/[\s\-().]/g, '');

/**
 * A Thai phone number as 0XXXXXXXXX (mobile, 10 digits: 06/08/09) or 0XXXXXXXX (landline, 9 digits:
 * 02-07), from "081-234-5678", "+66 81 234 5678", "66812345678"…; null when it is not one.
 * format 'e164' → "+66812345678". รูปแบบเบอร์โทรไทย
 */
export function normalizePhone(value: unknown, format: 'local' | 'e164' = 'local'): string | null {
  let s = digitsOf(value);
  if (s.startsWith('+')) s = s.slice(1);
  if (!/^\d+$/.test(s)) return null;
  if (s.startsWith('66')) s = '0' + s.slice(2);
  else if (s.startsWith('0066')) s = '0' + s.slice(4);
  const ok = /^0[689]\d{8}$/.test(s) || /^0[2-7]\d{7}$/.test(s);
  if (!ok) return null;
  return format === 'e164' ? '+66' + s.slice(1) : s;
}

/** Whether the value is a Thai phone number (mobile or landline). / เป็นเบอร์โทรไทยหรือไม่ */
export function isThaiPhone(value: unknown, opts: { mobileOnly?: boolean } = {}): boolean {
  const n = normalizePhone(value);
  return n !== null && (!opts.mobileOnly || n.length === 10);
}

/**
 * Whether the value is a valid 13-digit Thai citizen ID ("1-2345-67890-12-1" ok): the last digit is
 * (11 − Σ dᵢ·(13−i) mod 11) mod 10 over the first 12 digits. ตรวจเลขบัตรประชาชน
 */
export function isThaiCitizenId(value: unknown): boolean {
  const s = digitsOf(value);
  if (!/^\d{13}$/.test(s)) return false;
  let sum = 0;
  for (let i = 0; i < 12; i++) sum += (s.charCodeAt(i) - 48) * (13 - i);
  return (11 - (sum % 11)) % 10 === s.charCodeAt(12) - 48;
}

/** "1234567890121" → "1-2345-67890-12-1" (null when invalid). */
export function formatCitizenId(value: unknown): string | null {
  const s = digitsOf(value);
  if (!isThaiCitizenId(s)) return null;
  return `${s[0]}-${s.slice(1, 5)}-${s.slice(5, 10)}-${s.slice(10, 12)}-${s[12]}`;
}
