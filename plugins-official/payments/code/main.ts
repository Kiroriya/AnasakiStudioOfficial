// payments 1.0 — online payments through Stripe Checkout (cards; PromptPay for Stripe Thailand accounts) for the
// Shop, Donations and the site's own code. A caller asks for a checkout of a reference ("eshop:<order code>")
// and sends the customer to the returned URL; the payment is confirmed by Stripe's signed webhook or, without
// one, when the customer comes back (the session is read from Stripe). Either way "payments.paid" is emitted
// once per reference: {ref, amount, currency, provider, sessionId}.
//
//   plugins.call('payments', 'available')                     → true when a secret key is set
//   plugins.call('payments', 'checkout', {ref, amount, description, email?, successPath, cancelPath}) → {url}
//   plugins.call('payments', 'status', ref)                  → 'open' | 'paid' | null

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);
const set = (k: string) => String(site.settings(k) ?? '').trim();
const REF = /^[a-z][a-z0-9_-]{0,30}:[A-Za-z0-9_-]{1,80}$/;
const API = 'https://api.stripe.com/v1/';

interface Payment { id: number; ref: string; provider: string; session_id: string; amount: string; currency: string; description: string; status: string; paid_at: string | null; created_at: string }

const byRef = (ref: string) => (REF.test(ref) ? (db.table('payments').where('ref', ref).first() as Payment | null) : null);
const currency = () => (/^[a-z]{3}$/i.test(set('currency')) ? set('currency').toLowerCase() : 'thb');

function baseURL(): string {
  try {
    return String(site.info().url || '').replace(/\/+$/, '');
  } catch {
    return '';
  }
}

/** application/x-www-form-urlencoded with Stripe's nested keys. */
function form(o: Record<string, string | number>): string {
  return Object.entries(o).map(([k, v]) => encodeURIComponent(k) + '=' + encodeURIComponent(String(v))).join('&');
}

function stripe(method: 'GET' | 'POST', path: string, body?: Record<string, string | number>): any {
  const key = set('secretKey');
  if (!/^sk_(live|test)_[A-Za-z0-9]+$/.test(key)) throw new Error('payments: no Stripe secret key');
  const r = http.fetch(API + path, {
    method, timeout: 8000,
    headers: { Authorization: 'Bearer ' + key, ...(body ? { 'Content-Type': 'application/x-www-form-urlencoded' } : {}) },
    ...(body ? { body: form(body) } : {}),
  });
  let j: any = {};
  try {
    j = JSON.parse(r.body || '{}');
  } catch {
    /* not JSON */
  }
  if (!r.ok) throw new Error('payments: Stripe ' + r.status + ' ' + String(j?.error?.message ?? '').slice(0, 200));
  return j;
}

/** Marks ref paid once and announces it. */
function markPaid(ref: string, amount: number, sessionId: string): boolean {
  const row = byRef(ref);
  if (row && row.status === 'paid') return false;
  if (row) db.table('payments').where('id', row.id).update({ status: 'paid', paid_at: new Date(), ...(sessionId ? { session_id: sessionId } : {}) });
  else db.table('payments').insert({ ref, provider: 'stripe', session_id: sessionId, amount, currency: currency(), status: 'paid', paid_at: new Date() });
  const final = byRef(ref) as Payment;
  site.emit('payments.paid', { ref, amount: Number(final.amount), currency: final.currency, provider: 'stripe', sessionId: final.session_id });
  return true;
}

site.export('available', () => /^sk_(live|test)_[A-Za-z0-9]+$/.test(set('secretKey')));

site.export('status', (ref: string) => byRef(String(ref ?? ''))?.status ?? null);

site.export('checkout', (input: { ref: string; amount: number; description?: string; email?: string; successPath: string; cancelPath: string }) => {
  const ref = String(input?.ref ?? '');
  const amount = Math.round(Number(input?.amount) * 100) / 100;
  const local = (p: unknown) => typeof p === 'string' && p.startsWith('/') && !p.startsWith('//') && !p.includes('\\');
  if (!REF.test(ref)) throw new Error('payments.checkout: bad ref');
  if (!(amount >= 10 && amount <= 1_000_000)) throw new Error('payments.checkout: the amount must be 10 – 1,000,000');
  if (!local(input.successPath) || !local(input.cancelPath)) throw new Error('payments.checkout: successPath / cancelPath must be paths on this site');
  const row = byRef(ref);
  if (row?.status === 'paid') return { url: baseURL() + input.successPath, paid: true };
  const base = baseURL();
  const methods = set('methods') === 'card,promptpay' ? ['card', 'promptpay'] : ['card'];
  const body: Record<string, string | number> = {
    mode: 'payment', client_reference_id: ref,
    success_url: `${base}/payments/return?ref=${encodeURIComponent(ref)}&session_id={CHECKOUT_SESSION_ID}&to=${encodeURIComponent(input.successPath)}`,
    cancel_url: base + input.cancelPath,
    'line_items[0][quantity]': 1,
    'line_items[0][price_data][currency]': currency(),
    'line_items[0][price_data][unit_amount]': Math.round(amount * 100),
    'line_items[0][price_data][product_data][name]': String(input.description || ref).slice(0, 120),
    'metadata[ref]': ref,
  };
  methods.forEach((m, i) => (body[`payment_method_types[${i}]`] = m));
  if (input.email && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(input.email)) body.customer_email = input.email;
  const s = stripe('POST', 'checkout/sessions', body);
  const fields = { session_id: String(s.id ?? ''), amount, currency: currency(), description: String(input.description ?? '').slice(0, 191), status: 'open' };
  if (row) db.table('payments').where('id', row.id).update(fields);
  else db.table('payments').insert({ ref, provider: 'stripe', ...fields });
  return { url: String(s.url ?? '') };
});

// The customer comes back from Stripe: the session is read from Stripe (not trusted from the address).
site.route('GET /payments/return', { access: ['*'] }, ({ request }) => {
  const q = (k: string) => String(Array.isArray(request.query[k]) ? (request.query[k] as string[])[0] : request.query[k] ?? '');
  const ref = q('ref'), sid = q('session_id'), to = q('to');
  const dest = to.startsWith('/') && !to.startsWith('//') && !to.includes('\\') ? to : '/';
  const row = byRef(ref);
  if (row && row.status !== 'paid' && /^cs_[A-Za-z0-9_]+$/.test(sid) && sid === row.session_id) {
    try {
      const s = stripe('GET', 'checkout/sessions/' + sid);
      if (s.payment_status === 'paid' && s.client_reference_id === ref) markPaid(ref, Number(s.amount_total) / 100, sid);
    } catch (e) {
      log.warn('payments: return:', String((e as Error).message));
    }
  }
  return { status: 303, headers: { Location: dest, 'Cache-Control': 'no-store' } };
});

/** Stripe-Signature: t=<unix>,v1=<hex HMAC-SHA256(secret, "<t>.<body>")>; at most 5 minutes old. */
export function validSignature(header: string, body: string, secret: string, now = Date.now()): boolean {
  if (!secret || !header) return false;
  const parts = header.split(',').map((p) => p.trim().split('='));
  const ts = parts.find((p) => p[0] === 't')?.[1] ?? '';
  if (!/^\d{1,12}$/.test(ts) || Math.abs(now / 1000 - Number(ts)) > 300) return false;
  const want = crypto.hmac('sha256', secret, ts + '.' + body);
  return parts.some((p) => p[0] === 'v1' && crypto.timingSafeEqual(p[1] ?? '', want));
}

site.route('POST /payments/stripe/webhook', { access: ['*'], rawBody: true }, ({ request }) => {
  const raw = String(request.rawBody ?? '');
  if (!validSignature(String(request.headers['stripe-signature'] ?? ''), raw, set('webhookSecret'))) return { status: 400, body: { error: 'bad signature' } };
  let ev: any;
  try {
    ev = JSON.parse(raw);
  } catch {
    return { status: 400, body: { error: 'bad json' } };
  }
  const s = ev?.data?.object ?? {};
  if ((ev?.type === 'checkout.session.completed' || ev?.type === 'checkout.session.async_payment_succeeded') && s.payment_status === 'paid') {
    const ref = String(s.client_reference_id ?? s.metadata?.ref ?? '');
    if (REF.test(ref)) markPaid(ref, Number(s.amount_total) / 100, String(s.id ?? ''));
  }
  return { status: 200, body: { received: true } };
});

// ---------------------------------------------------------------- admin

site.adminNotice(({ locale }) => {
  if (/^sk_(live|test)_/.test(set('secretKey'))) return null;
  return { level: 'info', text: locale === 'en' ? 'Payments: add your Stripe secret key, and api.stripe.com to the allowed hosts.' : 'Payments: ใส่ Stripe secret key และเพิ่ม api.stripe.com ในรายการ host ที่อนุญาต',
    link: { label: { th: 'host ที่อนุญาต', en: 'Allowed hosts' }, href: '/admin/code/settings' } };
}, { access: ['administrator'] });

site.adminPage('list', {
  label: { th: 'การชำระเงินออนไลน์', en: 'Online payments' },
  icon: '💳',
  access: ['administrator'],
  render: (ctx) => {
    const en = ctx.locale === 'en';
    const rows = (db.table('payments').orderBy('id', 'desc').limit(200).get() as Payment[]).map((p) =>
      `<tr><td>${esc(String(p.created_at).replace('T', ' ').slice(0, 16))}</td><td><code>${esc(p.ref)}</code><div class="ak-hint">${esc(p.description)}</div></td>` +
      `<td class="ak-num">${esc(Number(p.amount).toLocaleString('en-US'))} ${esc(p.currency.toUpperCase())}</td>` +
      `<td><span class="ak-badge ${p.status === 'paid' ? 'ak-badge-ok' : 'ak-badge-warn'}">${esc(p.status === 'paid' ? (en ? 'Paid' : 'ชำระแล้ว') : en ? 'Not paid' : 'ยังไม่ชำระ')}</span></td></tr>`).join('');
    return {
      title: en ? 'Online payments' : 'การชำระเงินออนไลน์',
      html: `<header class="ak-page-head"><span class="ak-page-icon">💳</span><div class="ak-page-head-text"><h1>${en ? 'Online payments' : 'การชำระเงินออนไลน์'}</h1>` +
        `<p>Stripe Checkout · webhook: <code>${esc(baseURL())}/payments/stripe/webhook</code></p></div></header>` +
        (rows ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="pay-list"><tbody>${rows}</tbody></table></div></div>`
          : `<div class="ak-card"><p class="ak-muted">${en ? 'No payments yet' : 'ยังไม่มีการชำระเงิน'}</p></div>`),
    };
  },
});
