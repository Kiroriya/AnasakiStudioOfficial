import { baht, rate } from 'money';

const PRODUCTS = [
  { id: 1, name: 'Mug', price: 250 },
  { id: 2, name: 'T-shirt', price: 590 },
];

// Other units may change prices: site.filter('shop.price', (p, { product }) => …)
const priceOf = (p: { id: number; price: number }) => site.applyFilter('shop.price', p.price, { product: p });

// ---------------------------------------------------------------- optional integrations
// "optionalRequires": { thai, promptpay } — never needed to install shop; used when active. They are
// reached with plugins.has / plugins.call (an import would need "requires"). plugins.call throws an
// error with code 'unavailable' if the plugin went away in between: fall back the same way.
const optional = <T>(id: string, fn: () => T): T | null => {
  if (!plugins.has(id)) return null;
  try {
    return fn();
  } catch (e) {
    if ((e as { code?: string })?.code === 'unavailable') return null;
    throw e;
  }
};
/** "หนึ่งร้อยบาทถ้วน" with thai, else null. */
const bahtText = (n: number) => optional('thai', () => plugins.call('thai', 'format', 'bahtText', n) as string);
/** PromptPay QR for the amount (promptpay installed and its promptpayId set), else null. */
const promptpayQr = (n: number) =>
  optional('promptpay', () => plugins.call('promptpay', 'qrFor', n, { size: 200 }) as { payload: string; svg: string; url: string } | null);

/** What the buyer needs to pay an order: amount, the amount in words, a QR. */
function payment(total: number) {
  return { total, label: baht(total), words: bahtText(total), promptpay: promptpayQr(total) };
}

// GET /shop: products; GET /shop?order=<id>: also how to pay that order
// ({ id, item, status, total, label, words (thai), promptpay: { payload, svg, url } (promptpay) })
site.route('GET /shop', { access: ['*'] }, ({ request }) => {
  const out: Record<string, unknown> = {
    products: PRODUCTS.map((p) => ({ ...p, price: priceOf(p), label: baht(priceOf(p)), usd: Math.round(priceOf(p) * rate('THB', 'USD') * 100) / 100 })),
    orders: db.table('orders').count(),
    worksWith: { thai: plugins.has('thai'), promptpay: plugins.has('promptpay') },
  };
  const id = Number(Array.isArray(request.query.order) ? request.query.order[0] : request.query.order);
  if (id) {
    const o = db.table('orders').where('id', id).first();
    if (!o) return { status: 404, body: { error: 'no such order' } };
    out.pay = { id: o.id, item: o.item, status: o.status, ...payment(Number(o.total)) };
  }
  return out;
});

site.route('POST /shop/order', { access: ['*'] }, ({ request }) => {
  const { id } = request.json();
  const p = PRODUCTS.find((x) => x.id === Number(id));
  if (!p) return { status: 404, body: { error: 'no such product' } };
  const orderId = db.table('orders').insert({ item: p.name, total: priceOf(p), status: 'paid' });
  site.emit('shop.order.paid', { id: orderId, item: p.name, total: priceOf(p) });
  return { id: orderId, ...payment(priceOf(p)) };
});

site.collection('products', {
  label: { th: 'สินค้า', en: 'Products' },
  fields: { name: 'text', label: 'text' },
  access: ['*'],
  load: () => PRODUCTS.map((p) => ({ name: p.name, label: baht(priceOf(p)) })),
});

site.action('order', {
  input: { id: 'number' },
  access: ['*'],
  run: ({ input }) => {
    const p = PRODUCTS.find((x) => x.id === input.id);
    if (!p) return { errors: { id: 'ไม่พบสินค้า' } };
    db.table('orders').insert({ item: p.name, total: priceOf(p) });
    return { message: 'สั่งซื้อแล้ว', refresh: ['products'] };
  },
});

// Site Code (or plugins requiring shop) can import this; it runs here, with
// shop's own tables.
site.export('cartTotal', () => Number(db.table('orders').sum('total') ?? 0));

// Orders for a generic admin screen (the admin-crud sample: resource "orders" with
// source { plugin: "shop", list: "listOrders", get: "getOrder", save: "saveOrder" }). Called through
// plugins.call by that admin page / its actions (administrators by default).
const ORDER_COLS = ['id', 'item', 'total', 'status', 'created_at'];
const STATUSES = ['pending', 'paid', 'cancelled'];

site.export('listOrders', (opts: { q?: string; page?: number; perPage?: number } = {}) => {
  const page = Math.max(1, Math.floor(Number(opts.page) || 1));
  const perPage = Math.min(100, Math.max(1, Math.floor(Number(opts.perPage) || 20)));
  let b = db.table('orders').select(...ORDER_COLS);
  const q = String(opts.q ?? '').trim().slice(0, 100);
  if (q) b = b.where('item', 'like', '%' + q.replace(/[\\%_]/g, (c) => '\\' + c) + '%');
  const pg = b.orderByDesc('id').paginate(page, perPage);
  return { items: pg.items.map((o) => ({ ...o, total: Number(o.total) })), total: pg.total };
});

site.export('getOrder', (id: number) => {
  const o = db.table('orders').where('id', Number(id)).select(...ORDER_COLS).first();
  return o ? { ...o, total: Number(o.total) } : null;
});

site.export('saveOrder', ({ id, values }: { id?: number | null; values: Record<string, any> }) => {
  const errors: Record<string, string> = {};
  const row: Record<string, any> = {};
  if (values.item !== undefined && values.item !== null) row.item = String(values.item).slice(0, 100);
  if (values.total !== undefined && values.total !== null) {
    if (!(Number(values.total) >= 0)) errors.total = 'total >= 0';
    else row.total = Number(values.total);
  }
  if (values.status !== undefined && values.status !== null) {
    if (!STATUSES.includes(values.status)) errors.status = STATUSES.join(' | ');
    else row.status = values.status;
  }
  if (!id && !row.item) errors.item = 'required';
  if (Object.keys(errors).length) return { errors };
  if (id) {
    if (!db.table('orders').where('id', id).exists()) return { errors: { id: 'not found' } };
    db.table('orders').where('id', id).update(row);
    return { id };
  }
  return { id: db.table('orders').insert({ total: 0, status: 'pending', ...row }) };
});

// Unpaid orders (status "pending", e.g. from the "order" action) older than the setting
// "cancelAfterMinutes" (default 30) are cancelled every 5 minutes. Needs the "jobs" scope.
site.job('cancelStale', () => {
  const minutes = Math.max(1, Number(site.settings('cancelAfterMinutes') ?? 30) || 30);
  const cutoff = new Date(Date.now() - minutes * 60_000);
  const ids = db.table('orders').where('status', 'pending').where('created_at', '<', cutoff).pluck('id');
  if (!ids.length) return { cancelled: 0 };
  const n = db.table('orders').whereIn('id', ids).where('status', 'pending').update({ status: 'cancelled' });
  site.emit('shop.order.cancelled', { ids, count: n });
  return { cancelled: n };
});
site.cron('*/5 * * * *', 'cancelStale');
