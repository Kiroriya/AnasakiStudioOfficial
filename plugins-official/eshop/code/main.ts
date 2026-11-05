// eshop 1.0 — an online shop for Thai sites: products are the content type eshop_product (edited like posts,
// shown at /product/<slug> with the post template or one picked in Settings), a cart kept in a cookie, a
// checkout that writes p_eshop_orders, PromptPay QR codes with the exact amount (or bank transfer / cash on
// delivery), payment slips kept as private files, the "Orders" admin page and mails to the customer and shop.
import { payload } from './lib/payload';
import { qrSvg } from './lib/qr';
import { publicOrder, statusChanged } from './events';
import {
  CheckoutError, CheckoutInput, OrderRow, Status, TYPE, cartCookie, changeCart, checkCheckout, createOrder, esc, getOrder,
  getOrderByCode, getProduct, money, orderItems, parseCart, productsByIds, round2, setStatus, toProduct, COOKIE,
} from './model';
import { cartPage, checkoutPage, notFoundPage, orderPage } from './pages';
import { registerBlocks, buyBox } from './blocks';
import { registerAdmin } from './admin';
import { t } from './text';

const CAPS = typeof (site as any).can === 'function';

site.postType(TYPE, {
  label: { th: 'สินค้า', en: 'Products' },
  singular: { th: 'สินค้า', en: 'Product' },
  icon: '🛍️',
  slug: 'product',
  supports: ['title', 'content', 'excerpt', 'image', 'categories'],
  fields: {
    price: { type: 'number', label: { th: 'ราคา (บาท)', en: 'Price (THB)' }, required: true, min: 0 },
    salePrice: { type: 'number', label: { th: 'ราคาลด (บาท)', en: 'Sale price (THB)' }, min: 0,
      help: { th: 'เว้นว่าง = ไม่ลดราคา', en: 'Empty = not on sale' } },
    stock: { type: 'number', label: { th: 'จำนวนในสต็อก', en: 'Stock' }, min: 0,
      help: { th: 'เว้นว่าง = ไม่นับสต็อก', en: 'Empty = not counted' } },
    sku: { type: 'text', label: { th: 'รหัสสินค้า (SKU)', en: 'SKU' } },
  },
  ...(CAPS ? { caps: { edit: 'eshop.products' } } : {}),
} as any);

/** Form or JSON bodies → flat strings. */
function bodyOf(req: SiteRequest): Record<string, string> {
  const ct = String(req.headers['content-type'] ?? '');
  if (ct.includes('application/json')) {
    try {
      const o = req.json<Record<string, unknown>>();
      return Object.fromEntries(Object.entries(o ?? {}).map(([k, v]) => [k, typeof v === 'string' ? v : String(v ?? '')]));
    } catch {
      return {};
    }
  }
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

const wantsJSON = (req: SiteRequest) =>
  String(req.headers['content-type'] ?? '').includes('application/json') || String(req.headers.accept ?? '').includes('application/json');

const redirect = (to: string): RouteResult => ({ status: 303, headers: { Location: to, 'Cache-Control': 'no-store' } });

const cartOf = (req: SiteRequest) => parseCart(req.cookies[COOKIE]);

// ---------------------------------------------------------------- cart

site.route('GET /cart', { access: ['*'] }, ({ request }) => cartPage(cartOf(request)));

site.route('GET /cart/count', { access: ['*'] }, ({ request }) => ({
  status: 200, headers: { 'Cache-Control': 'no-store' }, body: { count: cartOf(request).reduce((s, l) => s + l.qty, 0) },
}));

function cartChange(ctx: RouteContext, add: boolean): RouteResult {
  const b = bodyOf(ctx.request);
  const id = Number(b.id) || 0;
  const qty = Math.floor(Number(b.qty ?? (add ? 1 : 0)));
  const p = id ? getProduct(id) : null;
  if (add && (!p || !p.inStock)) {
    return wantsJSON(ctx.request) ? { status: 400, body: { error: t(p ? 'soldOut' : 'notFound') } } : redirect('/cart');
  }
  const c = changeCart(cartOf(ctx.request), id, Number.isFinite(qty) ? qty : 0, add);
  cartCookie(ctx.response, c);
  if (wantsJSON(ctx.request)) return { status: 200, headers: { 'Cache-Control': 'no-store' }, body: { count: c.reduce((s, l) => s + l.qty, 0), message: add ? t('added') : '' } };
  return redirect(add ? '/cart' : '/cart');
}

site.route('POST /cart/add', { access: ['*'] }, (ctx) => cartChange(ctx, true));
site.route('POST /cart/update', { access: ['*'] }, (ctx) => cartChange(ctx, false));

// ---------------------------------------------------------------- checkout

site.route('GET /checkout', { access: ['*'] }, ({ request }) => checkoutPage(cartOf(request)));

site.route('POST /checkout', { access: ['*'] }, ({ request, response }) => {
  const b = bodyOf(request);
  const inp: CheckoutInput = {
    name: String(b.name ?? ''), phone: String(b.phone ?? ''), email: String(b.email ?? ''),
    address: String(b.address ?? ''), note: String(b.note ?? ''), payment: String(b.payment ?? ''),
  };
  const c = cartOf(request);
  const errors = checkCheckout(inp);
  if (Object.keys(errors).length) return { ...(checkoutPage(c, inp, errors, t('fix')) as object), status: 422 } as RouteResult;
  // at most 5 orders a minute from one address (spam)
  const key = 'shop:rate:' + crypto.hash('sha256', request.ip || '-');
  const n = Number(cache.get(key) ?? 0);
  if (n >= 5) return { ...(checkoutPage(c, inp, {}, t('e_busy')) as object), status: 429 } as RouteResult;
  cache.set(key, n + 1, 60);
  let o: OrderRow;
  try {
    o = createOrder(c, inp);
  } catch (e) {
    if (e instanceof CheckoutError) return { ...(checkoutPage(c, inp, {}, t(('e_' + e.message) as any)) as object), status: 409 } as RouteResult;
    throw e;
  }
  cartCookie(response, []);
  site.emit('eshop.order.created', publicOrder(o));
  jobs.enqueue('mailNew', { id: o.id });
  return redirect('/shop/order/' + o.code + '?new=1');
});

// ---------------------------------------------------------------- the customer's order page

site.route('GET /shop/order/{code}', { access: ['*'] }, ({ request }) => {
  const o = getOrderByCode(request.params.code);
  if (!o) return notFoundPage();
  return orderPage(o, request.query.new ? t('thanks') : '');
});

site.route('GET /shop/order/{code}/qr.svg', { access: ['*'] }, ({ request }) => {
  const o = getOrderByCode(request.params.code);
  const id = String(site.settings('promptpayId') ?? '').trim();
  if (!o || o.payment !== 'promptpay' || !id) return { status: 404, headers: { 'Content-Type': 'text/plain' }, body: 'not found' };
  let p: string;
  try {
    p = payload({ id, amount: Number(o.total).toFixed(2) });
  } catch (e) {
    return { status: 500, headers: { 'Content-Type': 'text/plain' }, body: String((e as Error).message) };
  }
  return { status: 200, headers: { 'Content-Type': 'image/svg+xml', 'Cache-Control': 'private, max-age=300' }, body: qrSvg(p, { size: 240 }) };
});

// The slip: a JPEG/PNG as a data URL (the page's script shrinks the photo first), kept as a private file.
site.route('POST /shop/order/{code}/slip', { access: ['*'] }, ({ request }) => {
  const o = getOrderByCode(request.params.code);
  if (!o || o.status !== 'pending' || o.payment === 'cod') return { status: 404, body: { error: t('notFound') } };
  if (o.slip) return { status: 409, body: { error: t('slipDone') } };
  let image = '';
  try {
    image = String(request.json<{ image?: string }>()?.image ?? '');
  } catch {
    /* not JSON */
  }
  const m = /^data:image\/(jpeg|png);base64,([A-Za-z0-9+/=]+)$/.exec(image);
  if (!m) return { status: 400, body: { error: t('slipBad') } };
  const bytes = base64Bytes(m[2]);
  const isJpeg = bytes[0] === 0xff && bytes[1] === 0xd8;
  const isPng = bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4e && bytes[3] === 0x47;
  if ((m[1] === 'jpeg' && !isJpeg) || (m[1] === 'png' && !isPng) || bytes.length > 900_000) return { status: 400, body: { error: t('slipBad') } };
  const path = `slips/${o.code}.${m[1] === 'png' ? 'png' : 'jpg'}`;
  files.put(path, bytes);
  db.table('orders').where('id', o.id).update({ slip: path, slip_at: new Date() });
  jobs.enqueue('mailSlip', { id: o.id });
  return { status: 200, body: { ok: true, message: t('slipDone') } };
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

// ---------------------------------------------------------------- designer data and the product page

site.collection('products', {
  label: { th: 'สินค้า (ร้านค้า)', en: 'Products (shop)' },
  fields: { id: 'number', title: 'text', url: 'url', image: 'image', price: 'text', regularPrice: 'text', onSale: 'bool', inStock: 'bool', priceValue: 'number' },
  params: { search: 'text', sort: 'text', category: 'text' },
  perPage: { default: 12, max: 48 },
  access: ['*'],
  cache: 30,
  load: ({ page, perPage, params }) => {
    const sort = String(params.sort ?? '');
    const r = db.posts.list({
      type: TYPE, page, perPage, status: 'publish', search: params.search || undefined, category: params.category || undefined,
      ...(sort === 'price' ? { orderBy: 'meta.price' } : sort === '-price' ? { orderBy: '-meta.price' } : sort === 'title' ? { order: 'title' } : {}),
    } as any);
    return {
      total: r.total,
      items: r.items.map((p) => {
        const x = toProduct(p);
        return { id: x.id, title: x.title, url: x.url, image: x.image, price: money(x.price), regularPrice: x.onSale ? money(x.regular) : '',
          onSale: x.onSale, inStock: x.inStock, priceValue: x.price };
      }),
    };
  },
});

// The price and an add-to-cart button under the product's description (setting buyBox, on by default) —
// for sites that show products with the plain post template.
site.filter('post.content', (html: string, ctx: any) => {
  const post = ctx?.post;
  if (!post || post.type !== TYPE || site.settings('buyBox') === false) return html;
  // ctx.post.meta may come as JSON text (designer post context) or as an object
  let meta = post.meta;
  if (typeof meta === 'string') {
    try {
      meta = JSON.parse(meta);
    } catch {
      meta = {};
    }
  }
  return html + buyBox(toProduct({ ...post, id: Number(post.id), meta }));
});

// ---------------------------------------------------------------- mails, events, unpaid orders

const siteTitle = () => {
  try {
    return site.info().title || '';
  } catch {
    return '';
  }
};
const orderURL = (o: OrderRow) => {
  let base = '';
  try {
    base = site.info().url || '';
  } catch {
    /* jobs without SITE_URL */
  }
  return base + '/shop/order/' + o.code;
};

function itemsHTML(o: OrderRow): string {
  return '<table cellpadding="6" style="border-collapse:collapse">' + orderItems(o.id).map((i) =>
    `<tr><td>${esc(i.title)} × ${i.qty}</td><td align="right">${money(Number(i.price) * i.qty)}</td></tr>`).join('') +
    `<tr><td><b>${esc(t('grand'))}</b></td><td align="right"><b>${money(Number(o.total))}</b></td></tr></table>`;
}

function mailCustomer(o: OrderRow, subjectKey: 'mailNewSubject' | 'mailPaidSubject' | 'mailShippedSubject') {
  if (!o.email) return;
  mail.send({
    to: o.email,
    subject: t(subjectKey, { id: o.id, site: siteTitle() }),
    html: `<p>${esc(o.name)}</p><p>${esc(t('status'))}: <b>${esc(t(('s_' + o.status) as any))}</b></p>${itemsHTML(o)}` +
      `<p><a href="${esc(orderURL(o))}">${esc(t('mailView'))}</a></p>`,
  });
}

function shopEmail(): string {
  const own = String(site.settings('notifyEmail') ?? '').trim();
  if (own) return own;
  try {
    return site.info().adminEmail || '';
  } catch {
    return '';
  }
}

site.job('mailNew', (ctx) => {
  const o = getOrder(Number(ctx.data?.id));
  if (!o) return;
  mailCustomer(o, 'mailNewSubject');
  const to = shopEmail();
  if (to) {
    mail.send({ to, subject: t('mailAdminSubject', { id: o.id, amount: money(Number(o.total)) }),
      html: `<p>${esc(o.name)} · ${esc(o.phone)} · ${esc(t(('m_' + o.payment) as any))}</p>${itemsHTML(o)}<p>${esc(o.address)}</p>` });
  }
});

site.job('mailSlip', (ctx) => {
  const o = getOrder(Number(ctx.data?.id));
  const to = shopEmail();
  if (o && to) mail.send({ to, subject: `Slip #${o.id} — ${money(Number(o.total))}`, text: `${o.name} ${o.phone}: ${t('slipDone')}` });
});

site.job('mailStatus', (ctx) => {
  const o = getOrder(Number(ctx.data?.id));
  if (!o) return;
  if (o.status === 'paid') mailCustomer(o, 'mailPaidSubject');
  if (o.status === 'shipped') mailCustomer(o, 'mailShippedSubject');
});

// Unpaid PromptPay / transfer orders older than cancelAfterHours are cancelled and their stock returns.
site.cron('17 * * * *', () => {
  const hours = Number(site.settings('cancelAfterHours') ?? 48);
  if (!(hours > 0)) return;
  const before = new Date(Date.now() - hours * 3600_000);
  const stale = db.table('orders').where('status', 'pending').whereIn('payment', ['promptpay', 'bank']).where('slip', '')
    .where('created_at', '<', before).limit(200).get() as OrderRow[];
  for (const o of stale) statusChanged(o, setStatus(o, 'cancelled' as Status));
}, { name: 'cancelUnpaid' });

site.export('orderTotal', (cart: { id: number; qty: number }[]) => {
  const prods = productsByIds((cart ?? []).map((l) => Number(l.id)));
  return round2((cart ?? []).reduce((s, l) => s + (prods.get(Number(l.id))?.price ?? 0) * Math.max(0, Number(l.qty) || 0), 0));
});

registerBlocks();
registerAdmin();
