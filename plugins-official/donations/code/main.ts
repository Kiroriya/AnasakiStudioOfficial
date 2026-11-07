// donations 1.0 — gifts for a foundation or cause: /donate (any amount, the donor or anonymous, a receipt for
// tax deduction on request) → /donate/d/<code> with a PromptPay QR of the amount (or bank transfer) and the slip
// upload → the "Donations" admin page confirms it, numbers the receipt and mails the donor; the receipt page is
// printable. A designer block shows suggested amounts and the progress to the goal. Works without JavaScript
// except the slip upload.
import { payload } from './lib/payload';
import { qrSvg } from './lib/qr';

const CAPS = typeof (site as any).can === 'function';
const ACCESS = CAPS ? ['cap:donations.manage'] : ['editor'];

export const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);
const set = (k: string) => String(site.settings(k) ?? '').trim();
const baht = (n: number) => '฿' + (Math.round(n * 100) / 100).toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: 2 });

function lang(): 0 | 1 {
  try {
    return String(site.info().language || 'th').startsWith('th') ? 0 : 1;
  } catch {
    return 0;
  }
}
const T = {
  title: ['ร่วมบริจาค', 'Donate'], amount: ['จำนวนเงิน (บาท)', 'Amount (THB)'], other: ['ระบุเอง', 'Other'],
  name: ['ชื่อผู้บริจาค', 'Your name'], email: ['อีเมล (รับการยืนยันและใบอนุโมทนาบัตร)', 'Email (for the confirmation and receipt)'],
  phone: ['เบอร์โทร', 'Phone'], message: ['ข้อความถึงเรา (ไม่ใส่ก็ได้)', 'A message (optional)'],
  anonymous: ['ไม่ประสงค์ออกนาม (ไม่แสดงชื่อในรายชื่อผู้บริจาค)', 'Anonymous (not shown in the donor list)'],
  receipt: ['ต้องการใบอนุโมทนาบัตรเพื่อลดหย่อนภาษี', 'I need a receipt for tax deduction'],
  taxId: ['เลขประจำตัวผู้เสียภาษี (13 หลัก)', 'Tax ID (13 digits)'], address: ['ที่อยู่สำหรับใบอนุโมทนาบัตร', 'Address on the receipt'],
  give: ['บริจาค', 'Donate'], bad: ['กรุณาตรวจข้อมูล: ', 'Please check: '], busy: ['ส่งถี่เกินไป รอสักครู่แล้วลองใหม่', 'Too many tries — wait a moment'],
  e_amount: ['จำนวนเงิน 1–1,000,000 บาท', 'an amount of 1–1,000,000'], e_name: ['ชื่อ', 'your name'], e_email: ['อีเมล', 'the email'],
  e_tax: ['เลขผู้เสียภาษี 13 หลักและที่อยู่', 'a 13-digit tax ID and the address'],
  thanks: ['ขอบคุณที่ร่วมบริจาค', 'Thank you for your gift'], scan: ['สแกนจ่ายด้วยแอปธนาคาร ยอด {amount}', 'Scan with your banking app to give {amount}'],
  transfer: ['โอน {amount} มาที่', 'Transfer {amount} to'], account: ['ชื่อบัญชี: {name}', 'Account name: {name}'],
  slip: ['แนบสลิปการโอน', 'Upload the payment slip'], slipSend: ['ส่งสลิป', 'Send slip'],
  slipDone: ['ได้รับสลิปแล้ว เราจะยืนยันและส่งใบอนุโมทนาบัตรให้', 'Slip received — we will confirm it and send the receipt'],
  slipBad: ['ส่งได้เฉพาะรูป JPG หรือ PNG', 'Only JPG or PNG images'],
  s_pending: ['รอตรวจสอบ', 'Being checked'], s_confirmed: ['ได้รับแล้ว', 'Received'], s_cancelled: ['ยกเลิก', 'Cancelled'],
  status: ['สถานะ', 'Status'], notFound: ['ไม่พบรายการ', 'Not found'], keepLink: ['เก็บลิงก์หน้านี้ไว้ดูสถานะและใบอนุโมทนาบัตร', 'Keep this link to see the status and the receipt'],
  receiptTitle: ['ใบอนุโมทนาบัตร', 'Donation receipt'], receiptNo: ['เลขที่', 'No.'], date: ['วันที่', 'Date'], from: ['ได้รับเงินบริจาคจาก', 'Received from'],
  sum: ['จำนวนเงิน', 'Amount'], print: ['พิมพ์ / บันทึกเป็น PDF', 'Print / save as PDF'], viewReceipt: ['ดูใบอนุโมทนาบัตร', 'View the receipt'],
  raised: ['ได้รับแล้ว {raised} จากเป้าหมาย {goal}', '{raised} raised of {goal}'], donors: ['ผู้ร่วมบริจาค {n} ราย', '{n} donors'],
  anon: ['ผู้ไม่ประสงค์ออกนาม', 'Anonymous'],
  mailConfirmed: ['ได้รับเงินบริจาคแล้ว', 'Your gift has been received'],
  payCard: ['บริจาคด้วยบัตร (ออนไลน์)', 'Give by card (online)'], cardFail: ['เปิดหน้าชำระเงินไม่สำเร็จ ลองอีกครั้ง', 'The payment page could not be opened — try again'],
} as const;
type K = keyof typeof T;
const t = (k: K, p?: Record<string, string | number>) => {
  let s: string = T[k][lang()];
  for (const [a, b] of Object.entries(p ?? {})) s = s.split(`{${a}}`).join(String(b));
  return s;
};

interface Gift {
  id: number; code: string; amount: string; campaign: string; name: string; email: string; phone: string; message: string;
  anonymous: boolean; receipt: boolean; tax_id: string; address: string | null; status: 'pending' | 'confirmed' | 'cancelled';
  slip: string; slip_at: string | null; receipt_no: string; confirmed_at: string | null; created_at: string;
}
const byCode = (c: string) => (/^[0-9a-f]{24}$/.test(c) ? (db.table('gifts').where('code', c).first() as Gift | null) : null);
const byId = (id: number) => db.table('gifts').where('id', id).first() as Gift | null;

export const amounts = () => set('amounts').split(',').map((x) => Math.round(Number(x.trim()))).filter((n) => n > 0 && n <= 1_000_000).slice(0, 8);

function totals(): { raised: number; donors: number } {
  const r = db.table('gifts').where('status', 'confirmed').sum('amount');
  return { raised: Number(r ?? 0), donors: db.table('gifts').where('status', 'confirmed').count() };
}

const CSS = `.dn{max-width:640px;margin:48px auto;padding:0 20px}.dn h1{font-size:28px;margin:0 0 16px}
.dn-amounts{display:flex;flex-wrap:wrap;gap:8px}.dn-amounts label{display:inline-flex}.dn-amounts input{position:absolute;opacity:0}
.dn-amounts span{display:inline-flex;align-items:center;min-height:44px;padding:0 18px;border:1px solid #d1d5db;border-radius:999px;cursor:pointer;font-weight:600}
.dn-amounts input:checked+span{background:var(--c-primary,#4f46e5);border-color:var(--c-primary,#4f46e5);color:#fff}.dn-amounts input:focus-visible+span{outline:2px solid #111827}
.dn-form{display:flex;flex-direction:column;gap:12px}.dn-form>label{display:flex;flex-direction:column;gap:6px;font-weight:600;font-size:.92em}
.dn-form input:not([type=radio]):not([type=checkbox]),.dn-form textarea{font:inherit;font-weight:400;padding:10px 12px;border:1px solid #d1d5db;border-radius:10px}
.dn-check{flex-direction:row!important;align-items:center;font-weight:400!important}
.dn-btn{display:inline-flex;align-items:center;justify-content:center;min-height:46px;padding:0 22px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer;text-decoration:none}
.dn-pay{display:flex;flex-direction:column;align-items:center;text-align:center;gap:8px;padding:20px;margin:16px 0;border:1px solid #e5e7eb;border-radius:14px}
.dn-qr{width:240px;height:240px;image-rendering:pixelated}.dn-bank{white-space:pre-wrap;font:inherit;text-align:left;background:#f9fafb;padding:12px 16px;border-radius:8px}
.dn-msg{padding:12px 14px;border-radius:10px;background:#dcfce7;color:#166534}.dn-bad{background:#fee2e2;color:#991b1b}.dn-muted{color:#6b7280}
.dn-bar{height:12px;border-radius:999px;background:#e5e7eb;overflow:hidden}.dn-bar i{display:block;height:100%;background:var(--c-primary,#4f46e5)}`;

let scriptURL = '';
try {
  scriptURL = site.asset('assets/donate.js').url;
} catch {
  /* none */
}
const page = (title: string, html: string, status = 200): RouteResult => ({
  page: { title, html: `<style>${CSS}</style><div class="dn">${html}</div>${scriptURL ? `<script defer src="${esc(scriptURL)}"></script>` : ''}` },
  status, headers: { 'Cache-Control': 'no-store' },
});

function formOf(req: SiteRequest): Record<string, string> {
  const out: Record<string, string> = {};
  for (const part of String(req.body ?? '').split('&')) {
    if (!part) continue;
    const i = part.indexOf('=');
    const dec = (s: string) => {
      try {
        return decodeURIComponent(s.replace(/\+/g, ' '));
      } catch {
        return '';
      }
    };
    out[dec(i < 0 ? part : part.slice(0, i))] = dec(i < 0 ? '' : part.slice(i + 1));
  }
  return out;
}

function progress(): string {
  const goal = Number(site.settings('goal') ?? 0) || 0;
  if (!(goal > 0)) return '';
  const { raised, donors } = totals();
  const pct = Math.min(100, Math.round((raised / goal) * 100));
  return `<div class="dn-progress"><div class="dn-bar" role="progressbar" aria-valuenow="${pct}" aria-valuemin="0" aria-valuemax="100"><i style="width:${pct}%"></i></div>` +
    `<p class="dn-muted">${esc(t('raised', { raised: baht(raised), goal: baht(goal) }))} · ${esc(t('donors', { n: donors }))}</p></div>`;
}

function donateForm(v: Record<string, string> = {}, error = ''): string {
  const list = amounts();
  const chosen = Number(v.amount) || list[1] || list[0] || 0;
  const custom = v.amount && !list.includes(Number(v.amount)) ? v.amount : '';
  const campaign = String(v.campaign ?? '').slice(0, 120);
  return (error ? `<p class="dn-msg dn-bad" role="alert">${esc(t('bad') + error)}</p>` : '') + progress() +
    `<form class="dn-form" method="post" action="/donate">${campaign ? `<input type="hidden" name="campaign" value="${esc(campaign)}">` : ''}` +
    `<fieldset style="border:0;padding:0;margin:0"><legend style="font-weight:600;margin-bottom:8px">${esc(t('amount'))}</legend><div class="dn-amounts">` +
    list.map((n) => `<label><input type="radio" name="amount" value="${n}"${!custom && n === chosen ? ' checked' : ''}><span>${baht(n)}</span></label>`).join('') +
    `</div></fieldset><label>${esc(t('other'))}<input type="number" name="other" min="1" max="1000000" step="1" inputmode="numeric" value="${esc(custom)}"></label>` +
    `<label>${esc(t('name'))}<input name="name" maxlength="191" autocomplete="name" value="${esc(v.name ?? '')}"></label>` +
    `<label>${esc(t('email'))}<input type="email" name="email" maxlength="191" autocomplete="email" value="${esc(v.email ?? '')}"></label>` +
    `<label>${esc(t('phone'))}<input type="tel" name="phone" maxlength="40" autocomplete="tel" value="${esc(v.phone ?? '')}"></label>` +
    `<label>${esc(t('message'))}<textarea name="message" rows="2" maxlength="500">${esc(v.message ?? '')}</textarea></label>` +
    `<label class="dn-check"><input type="checkbox" name="anonymous" value="1"${v.anonymous ? ' checked' : ''}> ${esc(t('anonymous'))}</label>` +
    `<label class="dn-check"><input type="checkbox" name="receipt" value="1"${v.receipt ? ' checked' : ''}> ${esc(t('receipt'))}</label>` +
    `<label>${esc(t('taxId'))}<input name="tax_id" maxlength="20" inputmode="numeric" value="${esc(v.tax_id ?? '')}"></label>` +
    `<label>${esc(t('address'))}<textarea name="address" rows="2" maxlength="500">${esc(v.address ?? '')}</textarea></label>` +
    '<div style="position:absolute;left:-9999px" aria-hidden="true"><input name="website" tabindex="-1" autocomplete="off"></div>' +
    `<button class="dn-btn" type="submit">💝 ${esc(t('give'))}</button></form>`;
}

site.route('GET /donate', { access: ['*'] }, ({ request }) => {
  const q = (k: string) => String(Array.isArray(request.query[k]) ? (request.query[k] as string[])[0] : request.query[k] ?? '');
  return page(t('title'), `<h1>${esc(t('title'))}${set('orgName') ? ' · ' + esc(set('orgName')) : ''}</h1>` + donateForm({ amount: q('amount'), campaign: q('campaign') }));
});

site.route('POST /donate', { access: ['*'] }, ({ request }) => {
  const b = formOf(request);
  if (String(b.website ?? '').trim()) return page(t('title'), `<p class="dn-msg">${esc(t('thanks'))}</p>`);
  const amount = Math.round((Number(b.other) || Number(b.amount) || 0) * 100) / 100;
  const v = { ...b, amount: String(amount || '') };
  const problems: string[] = [];
  if (!(amount >= 1 && amount <= 1_000_000)) problems.push(t('e_amount'));
  const name = String(b.name ?? '').trim().slice(0, 191), email = String(b.email ?? '').trim().toLowerCase();
  const receipt = b.receipt === '1';
  if (receipt && !name) problems.push(t('e_name'));
  if (email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) problems.push(t('e_email'));
  const taxId = String(b.tax_id ?? '').replace(/[\s-]/g, '');
  if (receipt && (!/^\d{13}$/.test(taxId) || String(b.address ?? '').trim().length < 10)) problems.push(t('e_tax'));
  if (problems.length) return { ...(page(t('title'), `<h1>${esc(t('title'))}</h1>` + donateForm(v, problems.join(', '))) as object), status: 422 } as RouteResult;
  const key = 'dn:rate:' + crypto.hash('sha256', request.ip || '-');
  const n = Number(cache.get(key) ?? 0);
  if (n >= 5) return page(t('title'), `<p class="dn-msg dn-bad">${esc(t('busy'))}</p>`, 429);
  cache.set(key, n + 1, 600);
  const code = crypto.random(12);
  const id = db.table('gifts').insert({ code, amount, campaign: String(b.campaign ?? '').slice(0, 120), name, email, phone: String(b.phone ?? '').trim().slice(0, 40),
    message: String(b.message ?? '').trim().slice(0, 500), anonymous: b.anonymous === '1', receipt, tax_id: receipt ? taxId : '',
    address: receipt ? String(b.address ?? '').trim().slice(0, 500) : '' });
  site.emit('donations.received', { id, amount, campaign: String(b.campaign ?? '') });
  jobs.enqueue('mailNew', { id });
  return { status: 303, headers: { Location: '/donate/d/' + code + '?new=1', 'Cache-Control': 'no-store' } };
});

function giftPage(g: Gift, notice = ''): RouteResult {
  const amount = baht(Number(g.amount));
  let pay = '';
  if (g.status === 'pending') {
    const pp = set('promptpayId');
    pay = `<div class="dn-pay">` + (pp ? `<img class="dn-qr" src="/donate/d/${esc(g.code)}/qr.svg" width="240" height="240" alt="PromptPay QR"><p>${esc(t('scan', { amount }))}</p>` +
      (set('promptpayName') ? `<p class="dn-muted">${esc(t('account', { name: set('promptpayName') }))}</p>` : '') : '') +
      (set('bankInfo') ? `<p>${esc(t('transfer', { amount }))}</p><pre class="dn-bank">${esc(set('bankInfo'))}</pre>` : '') +
      (g.slip ? `<p class="dn-msg" id="dn-slip-done">${esc(t('slipDone'))}</p>` :
        `<form data-dn-slip="${esc(g.code)}" data-bad="${esc(t('slipBad'))}" data-done="${esc(t('slipDone'))}" style="display:flex;flex-direction:column;align-items:center;gap:10px">` +
        `<label>${esc(t('slip'))} <input type="file" accept="image/jpeg,image/png" required></label><button type="submit" class="dn-btn">${esc(t('slipSend'))}</button><p class="dn-muted" role="status"></p></form>`) +
      '</div>';
  }
  if (g.status === 'pending' && !g.slip && cardReady()) {
    pay += `<form method="post" action="/donate/d/${esc(g.code)}/pay" style="text-align:center"><button type="submit" class="dn-btn" id="dn-pay-card">💳 ${esc(t('payCard'))}</button></form>`;
  }
  const rec = g.status === 'confirmed' && g.receipt ? `<p><a class="dn-btn" href="/donate/d/${esc(g.code)}/receipt">🧾 ${esc(t('viewReceipt'))}</a></p>` : '';
  return page(t('title'), (notice ? `<p class="dn-msg">${esc(notice)}</p>` : '') + `<h1>${esc(t('thanks'))}</h1>` +
    `<p><b>${amount}</b>${g.campaign ? ' · ' + esc(g.campaign) : ''}<br>${esc(t('status'))}: <b>${esc(t(('s_' + g.status) as K))}</b></p>` + pay + rec +
    `<p class="dn-muted">${esc(t('keepLink'))}</p>`);
}

site.route('GET /donate/d/{code}', { access: ['*'] }, ({ request }) => {
  const g = byCode(request.params.code);
  return g ? giftPage(g, request.query.card === 'failed' ? t('cardFail') : request.query.new ? t('thanks') : '') : page(t('notFound'), `<p>${esc(t('notFound'))}</p>`, 404);
});

// ---------------------------------------------------------------- card gifts (the Payments plugin, when installed)

function cardReady(): boolean {
  try {
    return plugins.has('payments') && plugins.call<boolean>('payments', 'available') === true;
  } catch {
    return false;
  }
}

site.route('POST /donate/d/{code}/pay', { access: ['*'] }, ({ request }) => {
  const g = byCode(request.params.code);
  if (!g || g.status !== 'pending') return page(t('notFound'), `<p>${esc(t('notFound'))}</p>`, 404);
  let to = '/donate/d/' + g.code + '?card=failed';
  try {
    const r = plugins.call<{ url: string }>('payments', 'checkout', { ref: 'donations:' + g.code, amount: Number(g.amount),
      description: (set('orgName') || t('title')) + (g.campaign ? ' · ' + g.campaign : ''), email: g.email,
      successPath: '/donate/d/' + g.code + '?new=1', cancelPath: '/donate/d/' + g.code });
    if (r && /^https:\/\//.test(String(r.url))) to = String(r.url);
  } catch (e) {
    log.warn('donations: card payment:', String((e as Error).message ?? e));
  }
  return { status: 303, headers: { Location: to, 'Cache-Control': 'no-store' } };
});

site.on('payments.paid', (ev: any) => {
  const m = /^donations:([a-f0-9]{24})$/.exec(String(ev?.ref ?? ''));
  const g = m ? byCode(m[1]) : null;
  if (g && g.status === 'pending' && Math.abs(Number(ev.amount) - Number(g.amount)) <= 0.01) confirmGift(g);
});

site.route('GET /donate/d/{code}/qr.svg', { access: ['*'] }, ({ request }) => {
  const g = byCode(request.params.code);
  const id = set('promptpayId');
  if (!g || g.status !== 'pending' || !id) return { status: 404, headers: { 'Content-Type': 'text/plain' }, body: 'not found' };
  try {
    return { status: 200, headers: { 'Content-Type': 'image/svg+xml', 'Cache-Control': 'private, max-age=300' },
      body: qrSvg(payload({ id, amount: Number(g.amount).toFixed(2) }), { size: 240 }) };
  } catch (e) {
    return { status: 500, headers: { 'Content-Type': 'text/plain' }, body: String((e as Error).message) };
  }
});

function base64Bytes(b64: string): Uint8Array {
  const A = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  const clean = b64.replace(/=+$/, '');
  const out = new Uint8Array(Math.floor((clean.length * 3) / 4));
  let buf = 0, bits = 0, j = 0;
  for (let i = 0; i < clean.length; i++) {
    buf = (buf << 6) | A.indexOf(clean[i]);
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      out[j++] = (buf >> bits) & 0xff;
    }
  }
  return out.subarray(0, j);
}

site.route('POST /donate/d/{code}/slip', { access: ['*'] }, ({ request }) => {
  const g = byCode(request.params.code);
  if (!g || g.status !== 'pending') return { status: 404, body: { error: t('notFound') } };
  if (g.slip) return { status: 409, body: { error: t('slipDone') } };
  let image = '';
  try {
    image = String(request.json<{ image?: string }>()?.image ?? '');
  } catch {
    /* not JSON */
  }
  const m = /^data:image\/(jpeg|png);base64,([A-Za-z0-9+/=]+)$/.exec(image);
  if (!m) return { status: 400, body: { error: t('slipBad') } };
  const bytes = base64Bytes(m[2]);
  const ok = m[1] === 'jpeg' ? bytes[0] === 0xff && bytes[1] === 0xd8 : bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4e && bytes[3] === 0x47;
  if (!ok || bytes.length > 900_000) return { status: 400, body: { error: t('slipBad') } };
  const path = `slips/${g.code}.${m[1] === 'png' ? 'png' : 'jpg'}`;
  files.put(path, bytes);
  db.table('gifts').where('id', g.id).update({ slip: path, slip_at: new Date() });
  jobs.enqueue('mailSlip', { id: g.id });
  return { status: 200, body: { ok: true, message: t('slipDone') } };
});

// The receipt (ใบอนุโมทนาบัตร) of a confirmed gift: a printable page (the browser saves it as PDF).
site.route('GET /donate/d/{code}/receipt', { access: ['*'] }, ({ request }) => {
  const g = byCode(request.params.code);
  if (!g || g.status !== 'confirmed' || !g.receipt) return page(t('notFound'), `<p>${esc(t('notFound'))}</p>`, 404);
  const d = new Date(g.confirmed_at ?? g.created_at);
  const day = lang() === 0 ? `${d.getDate()}/${d.getMonth() + 1}/${d.getFullYear() + 543}` : d.toISOString().slice(0, 10);
  const html = `<!doctype html><html lang="${lang() === 0 ? 'th' : 'en'}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">` +
    `<meta name="robots" content="noindex"><title>${esc(t('receiptTitle'))} ${esc(g.receipt_no)}</title><style>body{font-family:'Noto Sans Thai',system-ui,sans-serif;color:#111827;margin:0;background:#f3f4f6}` +
    `.r{max-width:720px;margin:24px auto;background:#fff;padding:48px;border:1px solid #e5e7eb}.r h1{text-align:center;font-size:26px;margin:0 0 4px}.c{text-align:center;color:#4b5563;margin:0}` +
    `table{width:100%;margin:28px 0;border-collapse:collapse}td{padding:8px 0;vertical-align:top}td:first-child{width:38%;color:#6b7280}.amt{font-size:22px;font-weight:700}` +
    `.sign{margin-top:56px;text-align:right}.sign span{display:inline-block;min-width:240px;border-top:1px solid #9ca3af;padding-top:6px;text-align:center}` +
    `button{font:inherit;padding:10px 18px;border-radius:8px;border:1px solid #d1d5db;background:#fff;cursor:pointer}@media print{body{background:#fff}.r{border:0;margin:0}button{display:none}}</style></head><body>` +
    `<div class="r"><h1>${esc(t('receiptTitle'))}</h1><p class="c"><b>${esc(set('orgName'))}</b></p>` +
    (set('orgTaxId') ? `<p class="c">${esc(t('taxId'))}: ${esc(set('orgTaxId'))}</p>` : '') +
    (set('orgAddress') ? `<p class="c">${esc(set('orgAddress'))}</p>` : '') +
    `<table><tr><td>${esc(t('receiptNo'))}</td><td><b>${esc(g.receipt_no)}</b></td></tr><tr><td>${esc(t('date'))}</td><td>${esc(day)}</td></tr>` +
    `<tr><td>${esc(t('from'))}</td><td>${esc(g.name)}<br>${esc(t('taxId'))}: ${esc(g.tax_id)}<br>${esc(g.address ?? '').replace(/\n/g, '<br>')}</td></tr>` +
    `<tr><td>${esc(t('sum'))}</td><td class="amt">${baht(Number(g.amount))}</td></tr>${g.campaign ? `<tr><td></td><td>${esc(g.campaign)}</td></tr>` : ''}</table>` +
    `<div class="sign"><span>${esc(set('signer') || set('orgName'))}</span></div><p style="text-align:center;margin-top:32px"><button onclick="print()">🖨 ${esc(t('print'))}</button></p></div></body></html>`;
  return { status: 200, headers: { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' }, body: html };
});

// ---------------------------------------------------------------- mails

function info() {
  try {
    return site.info();
  } catch {
    return { title: '', url: '', adminEmail: '' } as any;
  }
}
const notify = () => set('notifyEmail') || info().adminEmail || '';
const link = (g: Gift) => `${String(info().url || '').replace(/\/+$/, '')}/donate/d/${g.code}`;

site.job('mailNew', (ctx) => {
  const g = byId(Number(ctx.data?.id));
  const to = notify();
  if (g && to) mail.send({ to, subject: `💝 ${baht(Number(g.amount))} — ${g.anonymous ? t('anon') : g.name || '-'}`, text: `${g.name} ${g.email} ${g.phone}\n${g.message}\n${link(g)}` });
});
site.job('mailSlip', (ctx) => {
  const g = byId(Number(ctx.data?.id));
  const to = notify();
  if (g && to) mail.send({ to, subject: `🧾 ${baht(Number(g.amount))} — slip`, text: `${g.name}: ${t('slipDone')}` });
});
site.job('mailConfirmed', (ctx) => {
  const g = byId(Number(ctx.data?.id));
  if (!g || !g.email) return;
  mail.send({ to: g.email, subject: `${t('mailConfirmed')} — ${set('orgName') || info().title}`,
    html: `<p>${esc(g.name || t('thanks'))}</p><p>${esc(t('thanks'))}: <b>${baht(Number(g.amount))}</b></p>` +
      (g.receipt ? `<p><a href="${esc(link(g))}/receipt">${esc(t('viewReceipt'))}</a></p>` : '') });
});

// ---------------------------------------------------------------- the designer block

site.block('donate', {
  label: { th: 'ร่วมบริจาค', en: 'Donate' },
  icon: '💝',
  props: [
    { key: 'campaign', type: 'text', maxLength: 120, label: { th: 'ชื่อโครงการ (ไม่ใส่ก็ได้)', en: 'Campaign (optional)' } },
    { key: 'progress', type: 'boolean', default: true, label: { th: 'แถบความคืบหน้า', en: 'Progress bar' } },
  ],
  render: (_ctx, props) => {
    const camp = String(props.campaign ?? '').trim();
    const q = (n: number) => `/donate?amount=${n}${camp ? '&campaign=' + encodeURIComponent(camp) : ''}`;
    return {
      css: '.dnb{display:flex;flex-direction:column;gap:12px}.dnb-row{display:flex;flex-wrap:wrap;gap:8px}' +
        '.dnb a{display:inline-flex;align-items:center;min-height:44px;padding:0 18px;border-radius:999px;border:1px solid #d1d5db;color:inherit;text-decoration:none;font-weight:600}' +
        '.dnb a.dnb-go{background:var(--c-primary,#4f46e5);border-color:transparent;color:#fff}.dn-bar{height:12px;border-radius:999px;background:#e5e7eb;overflow:hidden}' +
        '.dn-bar i{display:block;height:100%;background:var(--c-primary,#4f46e5)}.dn-muted{color:#6b7280;margin:6px 0 0}',
      html: `<div class="dnb">${props.progress === false ? '' : progress()}<div class="dnb-row">` +
        amounts().map((n) => `<a href="${esc(q(n))}">${baht(n)}</a>`).join('') +
        `<a class="dnb-go" href="${esc(camp ? '/donate?campaign=' + encodeURIComponent(camp) : '/donate')}">💝 ${esc(t('give'))}</a></div></div>`,
    };
  },
});

// ---------------------------------------------------------------- the admin page

const L = {
  title: ['เงินบริจาค', 'Donations'], none: ['ยังไม่มีผู้บริจาค', 'No gifts yet'], date: ['วันที่', 'Date'], donor: ['ผู้บริจาค', 'Donor'],
  amount: ['ยอด', 'Amount'], status: ['สถานะ', 'Status'], confirm: ['ยืนยันได้รับเงิน', 'Confirm received'], cancel: ['ยกเลิก', 'Cancel'],
  slip: ['สลิป', 'Slip'], receipt: ['ใบอนุโมทนาบัตร', 'Receipt'], csv: ['ส่งออก CSV', 'Export CSV'], total: ['ยอดที่ได้รับ', 'Received'],
  pending: ['รอตรวจสอบ', 'Pending'], confirmed: ['ได้รับแล้ว', 'Received'], cancelled: ['ยกเลิก', 'Cancelled'],
} as const;
const q1 = (v: unknown) => String(Array.isArray(v) ? v[0] : v ?? '');
const csvCell = (v: unknown) => {
  const s = String(v ?? '');
  return /[",\n\r]|^[=+\-@]/.test(s) ? '"' + s.replace(/^([=+\-@])/, "'$1").replace(/"/g, '""') + '"' : s;
};

site.adminPage('gifts', {
  label: { th: 'เงินบริจาค', en: 'Donations' },
  icon: '💝',
  access: ACCESS,
  render: (ctx) => {
    const en = ctx.locale === 'en' ? 1 : 0;
    const tr = (k: keyof typeof L) => L[k][en];
    const status = q1(ctx.query.status);
    const b = db.table('gifts');
    if (['pending', 'confirmed', 'cancelled'].includes(status)) b.where('status', status);
    const rows = (b.orderBy('id', 'desc').limit(200).get() as Gift[]).map((g) =>
      `<tr><td class="ak-hint">${esc(String(g.created_at).replace('T', ' ').slice(0, 16))}</td><td>${esc(g.anonymous ? '🙈 ' + (g.name || '-') : g.name || '-')}` +
      `<div class="ak-hint">${esc(g.email)} ${esc(g.phone)}${g.campaign ? ' · ' + esc(g.campaign) : ''}</div>${g.message ? `<div class="ak-hint">“${esc(g.message)}”</div>` : ''}</td>` +
      `<td class="ak-num"><b>${baht(Number(g.amount))}</b></td><td><span class="ak-badge ${g.status === 'confirmed' ? 'ak-badge-ok' : g.status === 'pending' ? 'ak-badge-warn' : 'ak-badge-danger'}">${esc(tr(g.status))}</span>` +
      `${g.receipt ? `<div class="ak-hint">🧾 ${esc(g.receipt_no || tr('receipt'))}</div>` : ''}</td><td><div class="ak-actions">` +
      (g.slip ? `<a href="#" class="ak-btn ak-btn-sm" data-ak-download="slip=${g.id}">${esc(tr('slip'))}</a>` : '') +
      (g.status === 'pending' ? `<button class="ak-btn ak-btn-sm ak-btn-primary" data-dn="confirmed" data-id="${g.id}">${esc(tr('confirm'))}</button>` +
        `<button class="ak-btn ak-btn-sm ak-btn-danger" data-dn="cancelled" data-id="${g.id}">${esc(tr('cancel'))}</button>` : '') +
      (g.status === 'confirmed' && g.receipt ? `<a class="ak-btn ak-btn-sm" href="/donate/d/${esc(g.code)}/receipt" target="_blank" rel="noopener">${esc(tr('receipt'))}</a>` : '') +
      '</div></td></tr>').join('');
    const tab = (s: string, label: string) => `<a class="ak-tab" href="?status=${s}" aria-selected="${status === s}">${esc(label)}</a>`;
    let script = '';
    try {
      script = site.asset('assets/admin.js').url;
    } catch {
      /* none */
    }
    const { raised, donors } = totals();
    return {
      title: tr('title'),
      html: `<header class="ak-page-head"><span class="ak-page-icon">💝</span><div class="ak-page-head-text"><h1>${esc(tr('title'))}</h1>` +
        `<p>${esc(tr('total'))}: <b>${baht(raised)}</b> · ${donors}</p></div><div class="ak-actions"><a href="#" class="ak-btn" data-ak-download="csv=1">⬇ ${esc(tr('csv'))}</a></div></header>` +
        `<div class="ak-tabs">${tab('', en ? 'All' : 'ทั้งหมด')}${tab('pending', tr('pending'))}${tab('confirmed', tr('confirmed'))}${tab('cancelled', tr('cancelled'))}</div>` +
        (rows ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="dn-gifts"><thead><tr><th>${esc(tr('date'))}</th><th>${esc(tr('donor'))}</th>` +
          `<th class="ak-num">${esc(tr('amount'))}</th><th>${esc(tr('status'))}</th><th></th></tr></thead><tbody>${rows}</tbody></table></div></div>`
          : `<div class="ak-card"><p class="ak-muted">${esc(tr('none'))}</p></div>`) + (script ? `<script src="${esc(script)}" defer></script>` : ''),
    };
  },
  download: (ctx) => {
    const sid = Number(q1(ctx.query.slip)) || 0;
    if (sid) {
      const g = byId(sid);
      const bytes = g?.slip ? files.get(g.slip) : null;
      if (!g || !bytes) return { status: 404, headers: { 'Content-Type': 'text/plain' }, body: 'not found' };
      const png = g.slip.endsWith('.png');
      return { status: 200, body: bytes, headers: { 'Content-Type': png ? 'image/png' : 'image/jpeg', 'Content-Disposition': `attachment; filename="slip-${g.id}${png ? '.png' : '.jpg'}"` } };
    }
    const lines = ['id,date,status,amount,campaign,name,email,phone,anonymous,receipt_no,tax_id,address,message'];
    for (const g of db.table('gifts').orderBy('id').limit(50000).get() as Gift[]) {
      lines.push([g.id, String(g.created_at).slice(0, 16), g.status, g.amount, g.campaign, g.name, g.email, g.phone, g.anonymous ? 1 : 0, g.receipt_no, g.tax_id, g.address ?? '', g.message].map(csvCell).join(','));
    }
    return { status: 200, headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': 'attachment; filename="donations.csv"', 'Cache-Control': 'no-store' }, body: '﻿' + lines.join('\r\n') };
  },
});

// Confirming numbers the receipt (D<year>-<0001>, counted per year) and mails the donor.
site.action('setStatus', {
  input: { id: 'number', status: 'text' },
  access: ACCESS,
  run: ({ input }) => {
    const g = byId(Number(input.id));
    if (!g) return { message: 'not found', errors: { id: 'not found' } };
    const to = String(input.status);
    if (!['confirmed', 'cancelled'].includes(to) || g.status !== 'pending') return { message: 'bad status', errors: { status: 'bad status' } };
    if (to === 'cancelled') {
      db.table('gifts').where('id', g.id).update({ status: 'cancelled' });
      return { data: { id: g.id, status: to, receiptNo: '' } };
    }
    return { data: { id: g.id, status: to, receiptNo: confirmGift(g) } };
  },
});

/** Confirms a gift: numbers its receipt (D<year>-0001, counted per year), announces it and mails the donor. */
function confirmGift(g: Gift): string {
  const patch: Record<string, SqlValue> = { status: 'confirmed', confirmed_at: new Date() };
  if (g.receipt) {
    const prefix = `D${new Date().getFullYear()}-`;
    patch.receipt_no = prefix + String(db.table('gifts').whereLike('receipt_no', prefix + '%').count() + 1).padStart(4, '0');
  }
  db.table('gifts').where('id', g.id).update(patch);
  site.emit('donations.confirmed', { id: g.id, amount: Number(g.amount), campaign: g.campaign });
  jobs.enqueue('mailConfirmed', { id: g.id });
  return String(patch.receipt_no ?? '');
}
