// PromptPay payload (Thai QR Payment, EMVCo merchant-presented mode as profiled by the Bank of
// Thailand): tag 00 "01", tag 01 "11" (static, reusable) / "12" (dynamic, with an amount), merchant
// account tag 29 (credit transfer: AID A000000677010111 + 01 phone / 02 citizen or tax ID /
// 03 e-wallet) or tag 30 (bill payment: AID A000000677010112 + 01 biller ID, 02 ref1, 03 ref2),
// 58 "TH", 53 "764" (THB), 54 amount (2 decimals), 63 CRC-16/CCITT-FALSE (upper-case hex).

const AID_TRANSFER = 'A000000677010111';
const AID_BILL = 'A000000677010112';

/** TLV: 2-digit tag, 2-digit length, value (ASCII only). */
export function tlv(tag: string, value: string): string {
  if (value.length > 99) throw new Error(`promptpay: tag ${tag} longer than 99 characters`);
  return tag + String(value.length).padStart(2, '0') + value;
}

/** CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF, no reflection, no final xor) as 4 upper-case hex digits. */
export function crc16(data: string): string {
  let crc = 0xffff;
  for (let i = 0; i < data.length; i++) {
    crc ^= (data.charCodeAt(i) & 0xff) << 8;
    for (let b = 0; b < 8; b++) crc = crc & 0x8000 ? ((crc << 1) ^ 0x1021) & 0xffff : (crc << 1) & 0xffff;
  }
  return crc.toString(16).toUpperCase().padStart(4, '0');
}

export type PromptPayType = 'phone' | 'citizenId' | 'eWallet';

export interface PayloadOptions {
  /** Phone (0812345678, +66…), citizen / tax ID (13 digits) or e-wallet ID (15 digits). */
  id?: string;
  /** Forces how id is read (default: by its length). */
  type?: PromptPayType;
  /** Bill payment (tag 30) instead of id: biller ID (15 digits: tax ID + suffix) and references. */
  billerId?: string;
  ref1?: string;
  ref2?: string;
  /** THB; omitted / 0 = the payer types the amount (static QR). */
  amount?: number | string;
}

/** What kind of PromptPay ID a string is, and its tag 29 value. */
export function target(id: string, type?: PromptPayType): { type: PromptPayType; sub: string; value: string } {
  const d = String(id).replace(/[^0-9+]/g, '');
  let digits = d.replace(/^\+/, '');
  const t: PromptPayType = type ?? (/^0066\d{9}$/.test(digits) ? 'phone' : digits.length >= 15 ? 'eWallet' : digits.length >= 13 ? 'citizenId' : 'phone');
  if (t === 'phone') {
    if (digits.startsWith('0066')) digits = digits.slice(4);
    else if (digits.startsWith('66')) digits = digits.slice(2);
    else if (digits.startsWith('0')) digits = digits.slice(1);
    if (!/^\d{9}$/.test(digits)) throw new Error('promptpay: a phone number has 10 digits (0XXXXXXXXX)');
    return { type: t, sub: '01', value: '0066' + digits };
  }
  if (t === 'citizenId') {
    if (!/^\d{13}$/.test(digits)) throw new Error('promptpay: a citizen / tax ID has 13 digits');
    return { type: t, sub: '02', value: digits };
  }
  if (!/^\d{15}$/.test(digits)) throw new Error('promptpay: an e-wallet ID has 15 digits');
  return { type: t, sub: '03', value: digits };
}

/** "100" → "100.00"; null when no amount. Throws for negative / non-numeric / too large. */
export function formatAmount(amount: number | string | undefined | null): string | null {
  if (amount === undefined || amount === null || amount === '') return null;
  const n = Number(amount);
  if (!Number.isFinite(n) || n < 0) throw new Error('promptpay: amount must be a positive number');
  if (n === 0) return null;
  const s = (Math.round(n * 100 + 1e-9) / 100).toFixed(2);
  if (s.length > 13) throw new Error('promptpay: amount too large');
  return s;
}

/**
 * The PromptPay payload string to put in a QR code. / ข้อความ PromptPay สำหรับสร้าง QR
 *   payload({ id: '0812345678' })                 // anyone pays any amount
 *   payload({ id: '1234567890121', amount: 100 }) // 100.00 THB to a citizen ID
 *   payload({ billerId: '010555512345601', ref1: 'INV001', amount: 990 })
 */
export function payload(opts: PayloadOptions | string): string {
  const o: PayloadOptions = typeof opts === 'string' ? { id: opts } : opts;
  const amount = formatAmount(o.amount);
  let account: string;
  if (o.billerId) {
    const biller = String(o.billerId).replace(/\D/g, '');
    if (!/^\d{15}$/.test(biller)) throw new Error('promptpay: billerId has 15 digits');
    const ref = (r: string | undefined, name: string) => {
      const v = String(r ?? '').toUpperCase();
      if (!/^[A-Z0-9]{0,20}$/.test(v)) throw new Error(`promptpay: ${name} is up to 20 letters / digits`);
      return v;
    };
    const r1 = ref(o.ref1, 'ref1');
    if (!r1) throw new Error('promptpay: ref1 is required for a bill payment');
    const r2 = ref(o.ref2, 'ref2');
    account = tlv('30', tlv('00', AID_BILL) + tlv('01', biller) + tlv('02', r1) + (r2 ? tlv('03', r2) : ''));
  } else {
    if (!o.id) throw new Error('promptpay: id (or billerId) is required');
    const t = target(o.id, o.type);
    account = tlv('29', tlv('00', AID_TRANSFER) + tlv(t.sub, t.value));
  }
  const body = tlv('00', '01') + tlv('01', amount ? '12' : '11') + account + tlv('58', 'TH') + tlv('53', '764') +
    (amount ? tlv('54', amount) : '') + '6304';
  return body + crc16(body);
}

/** Checks the CRC of a payload (e.g. one scanned back). */
export function verifyPayload(p: string): boolean {
  return p.length > 8 && p.slice(-8, -4) === '6304' && crc16(p.slice(0, -4)) === p.slice(-4).toUpperCase();
}
