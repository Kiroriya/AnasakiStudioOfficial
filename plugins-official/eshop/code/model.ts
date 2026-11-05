// Products (post type eshop_product), the cart cookie and orders (p_eshop_orders / p_eshop_items).

export const TYPE = 'eshop_product';
export const STATUSES = ['pending', 'paid', 'shipped', 'done', 'cancelled'] as const;
export type Status = (typeof STATUSES)[number];
export type Payment = 'promptpay' | 'bank' | 'cod';

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

export interface Product {
  id: number;
  title: string;
  url: string;
  image: string;
  /** What the customer pays (the sale price when set and lower). */
  price: number;
  /** The normal price (shown struck through when on sale). */
  regular: number;
  onSale: boolean;
  sku: string;
  /** null = not counted. */
  stock: number | null;
  inStock: boolean;
}

export function toProduct(p: Post): Product {
  const m = (p.meta ?? {}) as Record<string, unknown>;
  const regular = Math.max(0, num(m.price) ?? 0);
  const sale = num(m.salePrice);
  const onSale = sale !== null && sale > 0 && sale < regular;
  const stock = num(m.stock);
  return {
    id: p.id, title: p.title, url: p.url, image: p.image, price: onSale ? (sale as number) : regular, regular, onSale,
    sku: String(m.sku ?? ''), stock: stock === null ? null : Math.max(0, Math.floor(stock)),
    inStock: stock === null || stock > 0,
  };
}

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

// ---------------------------------------------------------------- cart (cookie "cart": "12:2,15:1")

export const MAX_LINES = 30;
export const MAX_QTY = 99;
export type Cart = { id: number; qty: number }[];

export function parseCart(raw: string | undefined): Cart {
  const out: Cart = [];
  for (const part of String(raw ?? '').split(',')) {
    const m = /^(\d{1,12}):(\d{1,3})$/.exec(part.trim());
    if (!m) continue;
    const id = Number(m[1]);
    const qty = Math.min(MAX_QTY, Number(m[2]));
    if (id > 0 && qty > 0 && !out.some((l) => l.id === id) && out.length < MAX_LINES) out.push({ id, qty });
  }
  return out;
}

export const formatCart = (c: Cart) => c.map((l) => `${l.id}:${l.qty}`).join(',');

/** Sets a line's quantity (0 removes it); add = add to what is there. */
export function changeCart(c: Cart, id: number, qty: number, add: boolean): Cart {
  const out = c.filter((l) => l.id !== id);
  const had = c.find((l) => l.id === id)?.qty ?? 0;
  const q = Math.max(0, Math.min(MAX_QTY, Math.floor(add ? had + qty : qty)));
  if (q > 0 && id > 0) {
    const i = c.findIndex((l) => l.id === id);
    if (i >= 0) out.splice(Math.min(i, out.length), 0, { id, qty: q });
    else if (out.length < MAX_LINES) out.push({ id, qty: q });
  }
  return out;
}

export const COOKIE = 'cart';
export const cartCookie = (res: SiteResponse, c: Cart) =>
  res.setCookie(COOKIE, formatCart(c), { maxAge: c.length ? 60 * 60 * 24 * 30 : 0, httpOnly: false });

export interface Line { product: Product; qty: number; total: number; problem: string }

/** The cart priced from the products as they are now. */
export function priceCart(c: Cart): { lines: Line[]; subtotal: number; count: number } {
  const prods = productsByIds(c.map((l) => l.id));
  const lines: Line[] = [];
  for (const l of c) {
    const product = prods.get(l.id);
    if (!product) continue;
    let problem = '';
    if (!product.inStock) problem = 'soldOut';
    else if (product.stock !== null && l.qty > product.stock) problem = 'low';
    lines.push({ product, qty: l.qty, total: round2(product.price * l.qty), problem });
  }
  return { lines, subtotal: round2(lines.reduce((s, l) => s + l.total, 0)), count: lines.reduce((s, l) => s + l.qty, 0) };
}

export const round2 = (n: number) => Math.round(n * 100) / 100;

export function shippingFor(subtotal: number): number {
  const fee = Math.max(0, num(site.settings('shippingFee')) ?? 0);
  const free = Math.max(0, num(site.settings('freeShippingOver')) ?? 0);
  return subtotal <= 0 || (free > 0 && subtotal >= free) ? 0 : fee;
}

// ---------------------------------------------------------------- payment methods

export function methods(): Payment[] {
  const out: Payment[] = [];
  if (String(site.settings('promptpayId') ?? '').trim()) out.push('promptpay');
  if (String(site.settings('bankInfo') ?? '').trim()) out.push('bank');
  if (site.settings('allowCod') === true) out.push('cod');
  return out;
}

// ---------------------------------------------------------------- orders

export interface OrderRow {
  id: number; code: string; status: Status; payment: Payment; name: string; phone: string; email: string;
  address: string; note: string; subtotal: string; shipping: string; total: string; slip: string;
  slip_at: string | null; paid_at: string | null; created_at: string; updated_at: string;
}
export interface ItemRow { id: number; order_id: number; product_id: number; title: string; price: string; qty: number }

export const getOrderByCode = (code: string): OrderRow | null =>
  /^[a-f0-9]{24}$/.test(code) ? (db.table('orders').where('code', code).first() as OrderRow | null) : null;
export const getOrder = (id: number): OrderRow | null => (db.table('orders').where('id', id).first() as OrderRow | null);
export const orderItems = (orderId: number): ItemRow[] => db.table('items').where('order_id', orderId).orderBy('id').get() as ItemRow[];

export interface CheckoutInput { name: string; phone: string; email: string; address: string; note: string; payment: string }

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Field problems of a checkout ({} = fine); keys are the form fields. */
export function checkCheckout(inp: CheckoutInput): Record<string, string> {
  const e: Record<string, string> = {};
  if (!inp.name.trim()) e.name = 'required';
  if (inp.name.length > 191) e.name = 'tooLong';
  if (!/^[0-9+\-\s()]{9,20}$/.test(inp.phone.trim())) e.phone = 'phone';
  if (inp.email.trim() && !EMAIL.test(inp.email.trim())) e.email = 'email';
  if (inp.address.trim().length < 10) e.address = 'address';
  if (inp.address.length > 2000) e.address = 'tooLong';
  if (inp.note.length > 2000) e.note = 'tooLong';
  if (!methods().includes(inp.payment as Payment)) e.payment = 'payment';
  return e;
}

export class CheckoutError extends Error {}

/**
 * Creates the order from the cart (prices and stock read again here, inside a transaction), takes the
 * stock and answers the new order. Throws CheckoutError('empty' | 'soldOut' | 'low').
 */
export function createOrder(c: Cart, inp: CheckoutInput): OrderRow {
  const { lines, subtotal } = priceCart(c);
  if (!lines.length) throw new CheckoutError('empty');
  const bad = lines.find((l) => l.problem);
  if (bad) throw new CheckoutError(bad.problem);
  const shipping = shippingFor(subtotal);
  const code = crypto.random(12);
  const id = db.transaction((tx) => {
    const oid = tx.table('orders').insert({
      code, status: 'pending', payment: inp.payment, name: inp.name.trim(), phone: inp.phone.trim(), email: inp.email.trim(),
      address: inp.address.trim(), note: inp.note.trim(), subtotal, shipping, total: round2(subtotal + shipping),
    });
    tx.table('items').insert(lines.map((l) => ({ order_id: oid, product_id: l.product.id, title: l.product.title, price: l.product.price, qty: l.qty })));
    return oid;
  });
  takeStock(lines.map((l) => ({ id: l.product.id, qty: l.qty })), -1);
  return getOrder(id) as OrderRow;
}

/** Moves counted stock (dir -1 = sold, +1 = back). Products without a stock count are left alone. */
export function takeStock(lines: { id: number; qty: number }[], dir: 1 | -1): void {
  db.asSystem(() => {
    for (const l of lines) {
      const p = db.posts.get(l.id, { type: TYPE });
      if (!p || p.type !== TYPE) continue;
      const stock = num((p.meta ?? {}).stock);
      if (stock === null) continue;
      db.posts.update(p.id, { meta: { ...(p.meta ?? {}), stock: Math.max(0, stock + dir * l.qty) } });
    }
  });
}

/** Changes an order's status; cancelling gives the stock back (once), paying stamps paid_at. */
export function setStatus(o: OrderRow, to: Status): OrderRow {
  if (!STATUSES.includes(to) || o.status === to) return o;
  const patch: Record<string, SqlValue> = { status: to };
  if (to === 'paid' && !o.paid_at) patch.paid_at = new Date();
  db.table('orders').where('id', o.id).update(patch);
  if (to === 'cancelled') takeStock(orderItems(o.id).map((i) => ({ id: i.product_id, qty: i.qty })), 1);
  else if (o.status === 'cancelled') takeStock(orderItems(o.id).map((i) => ({ id: i.product_id, qty: i.qty })), -1);
  return getOrder(o.id) as OrderRow;
}

export function listOrders(opts: { status?: string; search?: string; page?: number; perPage?: number }) {
  const perPage = Math.min(100, Math.max(1, opts.perPage ?? 30));
  const page = Math.max(1, opts.page ?? 1);
  const q = () => {
    const b = db.table('orders');
    if (opts.status && STATUSES.includes(opts.status as Status)) b.where('status', opts.status);
    const s = String(opts.search ?? '').trim().slice(0, 100);
    if (s) b.where((w) => { w.whereLike('name', '%' + s + '%').orWhereLike('phone', '%' + s + '%').orWhereLike('email', '%' + s + '%').orWhere('code', s); });
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
