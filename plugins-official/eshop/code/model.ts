// Products (post type eshop_product), the cart cookie and orders (p_eshop_orders / p_eshop_items).

export const TYPE = 'eshop_product';
export const STATUSES = ['pending', 'paid', 'shipped', 'done', 'cancelled'] as const;
export type Status = (typeof STATUSES)[number];
/** card (1.1): online through the Payments plugin, when it is installed and set up. */
export type Payment = 'promptpay' | 'bank' | 'cod' | 'card';

/** Go's html.EscapeString. */
export const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);

const num = (v: unknown): number | null => {
  if (v === null || v === undefined || v === '') return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
};

/** 1234.5 → "1,234.50" */
export function baht(n: number): string {
  const v = Math.round(Math.abs(Number(n) || 0) * 100);
  const int = String(Math.floor(v / 100)).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  return (Number(n) < 0 ? '-' : '') + int + '.' + String(v % 100).padStart(2, '0');
}
export const money = (n: number) => '฿' + baht(n);

/** One choice of a product (size, colour, grind …, 1.1.0) with its own price and stock. */
export interface Variant {
  /** Its place in the product's list (the cart keeps it). */
  index: number;
  name: string;
  price: number;
  /** null = not counted. */
  stock: number | null;
  inStock: boolean;
}

export interface Product {
  id: number;
  title: string;
  url: string;
  image: string;
  /** What the customer pays (the sale price when set and lower); with variants, the lowest of theirs. */
  price: number;
  /** The normal price (shown struck through when on sale). */
  regular: number;
  onSale: boolean;
  sku: string;
  /** null = not counted. */
  stock: number | null;
  inStock: boolean;
  /** The choices to pick from (empty = none); optionLabel names them ("Size"). */
  variants: Variant[];
  optionLabel: string;
  /** Media ids of more photos (1.1.0). */
  gallery: number[];
}

function metaOf(p: { meta?: unknown }): Record<string, unknown> {
  let m = p.meta;
  if (typeof m === 'string') {
    try {
      m = JSON.parse(m);
    } catch {
      m = {};
    }
  }
  return (m && typeof m === 'object' ? m : {}) as Record<string, unknown>;
}

export function toProduct(p: Post): Product {
  const m = metaOf(p);
  const regular = Math.max(0, num(m.price) ?? 0);
  const sale = num(m.salePrice);
  const onSale = sale !== null && sale > 0 && sale < regular;
  const base = onSale ? (sale as number) : regular;
  const stock = num(m.stock);
  const variants: Variant[] = [];
  if (Array.isArray(m.variants)) {
    m.variants.slice(0, 30).forEach((row: any, index: number) => {
      const name = String(row?.name ?? '').trim();
      if (!name) return;
      const own = num(row?.price);
      const vs = num(row?.stock);
      const st = vs === null ? null : Math.max(0, Math.floor(vs));
      variants.push({ index, name, price: own !== null && own > 0 ? own : base, stock: st, inStock: st === null || st > 0 });
    });
  }
  const gallery = Array.isArray(m.gallery) ? m.gallery.map(Number).filter((n) => Number.isInteger(n) && n > 0).slice(0, 20) : [];
  const counted = stock === null ? null : Math.max(0, Math.floor(stock));
  return {
    id: p.id, title: p.title, url: p.url, image: p.image,
    price: variants.length ? Math.min(...variants.map((v) => v.price)) : base, regular, onSale: onSale && !variants.length,
    sku: String(m.sku ?? ''), stock: variants.length ? null : counted,
    inStock: variants.length ? variants.some((v) => v.inStock) : counted === null || counted > 0,
    variants, optionLabel: String(m.optionLabel ?? '').trim(), gallery,
  };
}

/** The variants' prices differ ("from ฿…"). */
export const pricesVary = (p: Product) => p.variants.length > 1 && p.variants.some((v) => v.price !== p.variants[0].price);

/** Published products by id (missing / drafts left out). */
export function productsByIds(ids: number[]): Map<number, Product> {
  const out = new Map<number, Product>();
  const uniq = [...new Set(ids.filter((n) => Number.isInteger(n) && n > 0))].slice(0, 200);
  if (!uniq.length) return out;
  const { items } = db.posts.list({ type: TYPE, ids: uniq, perPage: 100, status: 'publish' });
  for (const p of items) out.set(p.id, toProduct(p));
  return out;
}

export function getProduct(idOrSlug: number | string): Product | null {
  const p = db.posts.get(idOrSlug, { type: TYPE });
  return p && p.type === TYPE && p.status === 'publish' ? toProduct(p) : null;
}

// ---------------------------------------------------------------- cart (cookie "cart": "12:2,15.1:1" — id[.variant]:qty)

export const MAX_LINES = 30;
export const MAX_QTY = 99;
/** v = the variant's index, -1 = the product itself. */
export type Cart = { id: number; v: number; qty: number }[];

const same = (a: { id: number; v: number }, b: { id: number; v: number }) => a.id === b.id && a.v === b.v;

export function parseCart(raw: string | undefined): Cart {
  const out: Cart = [];
  for (const part of String(raw ?? '').split(',')) {
    const m = /^(\d{1,12})(?:\.(\d{1,2}))?:(\d{1,3})$/.exec(part.trim());
    if (!m) continue;
    const l = { id: Number(m[1]), v: m[2] === undefined ? -1 : Number(m[2]), qty: Math.min(MAX_QTY, Number(m[3])) };
    if (l.id > 0 && l.qty > 0 && !out.some((x) => same(x, l)) && out.length < MAX_LINES) out.push(l);
  }
  return out;
}

export const formatCart = (c: Cart) => c.map((l) => `${l.id}${l.v >= 0 ? '.' + l.v : ''}:${l.qty}`).join(',');

/** Sets a line's quantity (0 removes it); add = add to what is there. */
export function changeCart(c: Cart, id: number, v: number, qty: number, add: boolean): Cart {
  const key = { id, v };
  const out = c.filter((l) => !same(l, key));
  const had = c.find((l) => same(l, key))?.qty ?? 0;
  const q = Math.max(0, Math.min(MAX_QTY, Math.floor(add ? had + qty : qty)));
  if (q > 0 && id > 0) {
    const i = c.findIndex((l) => same(l, key));
    if (i >= 0) out.splice(Math.min(i, out.length), 0, { id, v, qty: q });
    else if (out.length < MAX_LINES) out.push({ id, v, qty: q });
  }
  return out;
}

export const COOKIE = 'cart';
export const cartCookie = (res: SiteResponse, c: Cart) =>
  res.setCookie(COOKIE, formatCart(c), { maxAge: c.length ? 60 * 60 * 24 * 30 : 0, httpOnly: false });

export interface Line { product: Product; variant: Variant | null; qty: number; price: number; total: number; problem: string }

/** "Mug — Large" */
export const lineTitle = (l: { product: Product; variant: Variant | null }) => l.product.title + (l.variant ? ' — ' + l.variant.name : '');

/** The cart priced from the products as they are now (a line whose variant is gone is left out). */
export function priceCart(c: Cart): { lines: Line[]; subtotal: number; count: number } {
  const prods = productsByIds(c.map((l) => l.id));
  const lines: Line[] = [];
  for (const l of c) {
    const product = prods.get(l.id);
    if (!product) continue;
    let variant: Variant | null = null;
    if (product.variants.length) {
      variant = product.variants.find((x) => x.index === l.v) ?? null;
      if (!variant) continue;
    } else if (l.v >= 0) continue;
    const stock = variant ? variant.stock : product.stock;
    const inStock = variant ? variant.inStock : product.inStock;
    const price = variant ? variant.price : product.price;
    let problem = '';
    if (!inStock) problem = 'soldOut';
    else if (stock !== null && l.qty > stock) problem = 'low';
    lines.push({ product, variant, qty: l.qty, price, total: round2(price * l.qty), problem });
  }
  return { lines, subtotal: round2(lines.reduce((s, l) => s + l.total, 0)), count: lines.reduce((s, l) => s + l.qty, 0) };
}

export const round2 = (n: number) => Math.round(n * 100) / 100;

export function shippingFor(subtotal: number): number {
  const fee = Math.max(0, num(site.settings('shippingFee')) ?? 0);
  const free = Math.max(0, num(site.settings('freeShippingOver')) ?? 0);
  return subtotal <= 0 || (free > 0 && subtotal >= free) ? 0 : fee;
}

// ---------------------------------------------------------------- coupons (setting "coupons", 1.1.0)

export interface Coupon { code: string; percent: number; amount: number; min: number; until: string }

/** One per line: CODE | 10% or 100 (baht) | minimum order (optional) | last day YYYY-MM-DD (optional). */
export function coupons(): Coupon[] {
  const out: Coupon[] = [];
  for (const line of String(site.settings('coupons') ?? '').split('\n')) {
    const [c, d, min, until] = line.split('|').map((s) => s.trim());
    const code = String(c ?? '').toUpperCase();
    if (!/^[A-Z0-9_-]{2,30}$/.test(code) || !d) continue;
    const pct = /^(\d{1,2}(?:\.\d+)?|100)\s*%$/.exec(d);
    const amt = num(d.replace(/[฿,\s]/g, ''));
    if (!pct && !(amt !== null && amt > 0)) continue;
    out.push({ code, percent: pct ? Number(pct[1]) : 0, amount: pct ? 0 : (amt as number), min: Math.max(0, num((min ?? '').replace(/[฿,\s]/g, '')) ?? 0),
      until: /^\d{4}-\d{2}-\d{2}$/.test(until ?? '') ? (until as string) : '' });
  }
  return out;
}

const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
};

/** The discount of code on subtotal; problem: '' | 'unknown' | 'expired' | 'min' (min = the minimum order). */
export function couponDiscount(code: string, subtotal: number): { coupon: Coupon | null; discount: number; problem: string } {
  const c = code.trim().toUpperCase();
  if (!c) return { coupon: null, discount: 0, problem: '' };
  const k = coupons().find((x) => x.code === c);
  if (!k) return { coupon: null, discount: 0, problem: 'unknown' };
  if (k.until && today() > k.until) return { coupon: k, discount: 0, problem: 'expired' };
  if (subtotal < k.min) return { coupon: k, discount: 0, problem: 'min' };
  const d = k.percent ? round2((subtotal * k.percent) / 100) : k.amount;
  return { coupon: k, discount: Math.min(subtotal, round2(d)), problem: '' };
}

// ---------------------------------------------------------------- payment methods

export function methods(): Payment[] {
  const out: Payment[] = [];
  if (String(site.settings('promptpayId') ?? '').trim()) out.push('promptpay');
  if (String(site.settings('bankInfo') ?? '').trim()) out.push('bank');
  if (site.settings('allowCod') === true) out.push('cod');
  if (cardReady()) out.push('card');
  return out;
}

/** The Payments plugin is active and has its keys (feature-detected: the shop works without it). */
export function cardReady(): boolean {
  try {
    return plugins.has('payments') && plugins.call<boolean>('payments', 'available') === true;
  } catch {
    return false;
  }
}

// ---------------------------------------------------------------- orders

export interface OrderRow {
  id: number; code: string; status: Status; payment: Payment; name: string; phone: string; email: string;
  address: string; note: string; subtotal: string; shipping: string; total: string; slip: string;
  slip_at: string | null; paid_at: string | null; created_at: string; updated_at: string;
  /** 1.1.0 */
  discount: string; coupon: string; carrier: string; tracking: string;
  receipt_no: string; tax_name: string; tax_id: string; tax_address: string | null; member_id: string;
}
export interface ItemRow { id: number; order_id: number; product_id: number; title: string; price: string; qty: number; variant: string; variant_index: number }

export const getOrderByCode = (code: string): OrderRow | null =>
  /^[a-f0-9]{24}$/.test(code) ? (db.table('orders').where('code', code).first() as OrderRow | null) : null;
export const getOrder = (id: number): OrderRow | null => (db.table('orders').where('id', id).first() as OrderRow | null);
export const orderItems = (orderId: number): ItemRow[] => db.table('items').where('order_id', orderId).orderBy('id').get() as ItemRow[];
export const itemTitle = (i: ItemRow) => i.title + (i.variant ? ' — ' + i.variant : '');

export interface CheckoutInput {
  name: string; phone: string; email: string; address: string; note: string; payment: string; coupon: string;
  /** 1.1: a full tax invoice — the buyer's name / company, 13-digit tax ID and address. */
  wantTax?: string; taxName?: string; taxId?: string; taxAddress?: string;
}

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Field problems of a checkout ({} = fine); keys are the form fields. */
export function checkCheckout(inp: CheckoutInput, subtotal = 0): Record<string, string> {
  const e: Record<string, string> = {};
  if (!inp.name.trim()) e.name = 'required';
  if (inp.name.length > 191) e.name = 'tooLong';
  if (!/^[0-9+\-\s()]{9,20}$/.test(inp.phone.trim())) e.phone = 'phone';
  if (inp.email.trim() && !EMAIL.test(inp.email.trim())) e.email = 'email';
  if (inp.address.trim().length < 10) e.address = 'address';
  if (inp.address.length > 2000) e.address = 'tooLong';
  if (inp.note.length > 2000) e.note = 'tooLong';
  if (!methods().includes(inp.payment as Payment)) e.payment = 'payment';
  const cp = couponDiscount(inp.coupon ?? '', subtotal).problem;
  if (cp) e.coupon = 'coupon_' + cp;
  if (inp.wantTax) {
    if (!String(inp.taxName ?? '').trim() || String(inp.taxName).length > 191) e.taxName = 'required';
    if (!/^\d{13}$/.test(String(inp.taxId ?? '').replace(/[\s-]/g, ''))) e.taxId = 'taxId';
    if (String(inp.taxAddress ?? '').trim().length < 10 || String(inp.taxAddress).length > 1000) e.taxAddress = 'address';
  }
  return e;
}

export class CheckoutError extends Error {}

/**
 * Creates the order from the cart (prices and stock read again here, inside a transaction), takes the
 * stock and answers the new order. Throws CheckoutError('empty' | 'soldOut' | 'low').
 */
export function createOrder(c: Cart, inp: CheckoutInput, memberId = ''): OrderRow {
  const { lines, subtotal } = priceCart(c);
  if (!lines.length) throw new CheckoutError('empty');
  const bad = lines.find((l) => l.problem);
  if (bad) throw new CheckoutError(bad.problem);
  const { coupon, discount } = couponDiscount(inp.coupon ?? '', subtotal);
  const shipping = shippingFor(subtotal - discount);
  const code = crypto.random(12);
  const id = db.transaction((tx) => {
    const oid = tx.table('orders').insert({
      code, status: 'pending', payment: inp.payment, name: inp.name.trim(), phone: inp.phone.trim(), email: inp.email.trim(),
      address: inp.address.trim(), note: inp.note.trim(), subtotal, shipping, discount, coupon: coupon && discount > 0 ? coupon.code : '',
      total: round2(subtotal - discount + shipping), member_id: memberId.slice(0, 64),
      ...(inp.wantTax ? { tax_name: String(inp.taxName).trim().slice(0, 191), tax_id: String(inp.taxId).replace(/[\s-]/g, ''),
        tax_address: String(inp.taxAddress).trim().slice(0, 1000) } : {}),
    });
    tx.table('items').insert(lines.map((l) => ({ order_id: oid, product_id: l.product.id, title: l.product.title, price: l.price, qty: l.qty,
      variant: l.variant ? l.variant.name.slice(0, 120) : '', variant_index: l.variant ? l.variant.index : -1 })));
    return oid;
  });
  takeStock(lines.map((l) => ({ id: l.product.id, v: l.variant ? l.variant.index : -1, qty: l.qty })), -1);
  return getOrder(id) as OrderRow;
}

/** Moves counted stock (dir -1 = sold, +1 = back); a variant's own count when v ≥ 0. Uncounted stock is left alone. */
export function takeStock(lines: { id: number; v: number; qty: number }[], dir: 1 | -1): void {
  db.asSystem(() => {
    for (const l of lines) {
      const p = db.posts.get(l.id, { type: TYPE });
      if (!p || p.type !== TYPE) continue;
      const meta = { ...metaOf(p) };
      if (l.v >= 0) {
        if (!Array.isArray(meta.variants) || !meta.variants[l.v]) continue;
        const rows = meta.variants.map((r: any) => ({ ...r }));
        const s = num(rows[l.v].stock);
        if (s === null) continue;
        rows[l.v].stock = Math.max(0, s + dir * l.qty);
        meta.variants = rows;
      } else {
        const stock = num(meta.stock);
        if (stock === null) continue;
        meta.stock = Math.max(0, stock + dir * l.qty);
      }
      db.posts.update(p.id, { meta });
    }
  });
}

const stockLines = (o: OrderRow) => orderItems(o.id).map((i) => ({ id: i.product_id, v: Number(i.variant_index ?? -1), qty: i.qty }));

/** Changes an order's status; cancelling gives the stock back (once), paying stamps paid_at. */
export function setStatus(o: OrderRow, to: Status): OrderRow {
  if (!STATUSES.includes(to) || o.status === to) return o;
  const patch: Record<string, SqlValue> = { status: to };
  if (to === 'paid' && !o.paid_at) patch.paid_at = new Date();
  // 1.1: a paid order gets its receipt number once (R<year>-0001, counted per year)
  if (to === 'paid' && !o.receipt_no) {
    const prefix = `R${new Date().getFullYear()}-`;
    patch.receipt_no = prefix + String(db.table('orders').whereLike('receipt_no', prefix + '%').count() + 1).padStart(4, '0');
  }
  db.table('orders').where('id', o.id).update(patch);
  if (to === 'cancelled') takeStock(stockLines(o), 1);
  else if (o.status === 'cancelled') takeStock(stockLines(o), -1);
  return getOrder(o.id) as OrderRow;
}

/** The parcel's carrier and tracking number (1.1.0). */
export function setTracking(o: OrderRow, carrier: string, tracking: string): OrderRow {
  db.table('orders').where('id', o.id).update({ carrier: carrier.trim().slice(0, 60), tracking: tracking.trim().slice(0, 120) });
  return getOrder(o.id) as OrderRow;
}

/** A link to follow the parcel on the carrier's site (known Thai carriers; "" = none). */
export function trackingURL(carrier: string, tracking: string): string {
  const n = encodeURIComponent(tracking.trim());
  if (!n) return '';
  const c = carrier.toLowerCase();
  if (/ไปรษณีย์|thai\s*post|ems/.test(c)) return 'https://track.thailandpost.co.th/?trackNumber=' + n;
  if (/kerry|เคอรี่|kex/.test(c)) return 'https://th.kex-express.com/th/track/?track=' + n;
  if (/flash|แฟลช/.test(c)) return 'https://www.flashexpress.co.th/fle/tracking?se=' + n;
  if (/j\s*&?\s*t|เจแอนด์ที/.test(c)) return 'https://www.jtexpress.co.th/service/track?billcode=' + n;
  return '';
}

export function listOrders(opts: { status?: string; search?: string; page?: number; perPage?: number }) {
  const perPage = Math.min(100, Math.max(1, opts.perPage ?? 30));
  const page = Math.max(1, opts.page ?? 1);
  const q = () => {
    const b = db.table('orders');
    if (opts.status && STATUSES.includes(opts.status as Status)) b.where('status', opts.status);
    const s = String(opts.search ?? '').trim().slice(0, 100);
    if (s) b.where((w) => { w.whereLike('name', '%' + s + '%').orWhereLike('phone', '%' + s + '%').orWhereLike('email', '%' + s + '%').orWhere('code', s).orWhere('tracking', s); });
    return b;
  };
  const total = q().count();
  const items = q().orderBy('id', 'desc').limit(perPage).offset((page - 1) * perPage).get() as OrderRow[];
  return { items, total, page, perPage };
}

export function counts(): Record<string, number> {
  const out: Record<string, number> = {};
  for (const r of db.table('orders').select('status').selectCount('n').groupBy('status').get() as { status: string; n: number }[]) out[r.status] = Number(r.n);
  return out;
}
