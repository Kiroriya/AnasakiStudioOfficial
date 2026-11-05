// Designer blocks: "add to cart" (a product chosen in the panel, else the product page's own product or the
// collection item), the cart link with its count, and the price. All work without JavaScript; assets/shop.js
// adds to the cart in place and fills the count.
import { Product, TYPE, esc, getProduct, money } from './model';
import { css } from './pages';
import { t } from './text';

const SCRIPT = 'assets/shop.js';
/** Looked up once when the plugin loads (post.content has 50 ms: no package files are read there). */
const SCRIPT_URL = (() => {
  try {
    return site.asset(SCRIPT).url;
  } catch {
    return '';
  }
})();

const idFrom = (ctx: { post: Record<string, string> | null; item: Record<string, string> | null }, props: Record<string, any>) => {
  const own = Number(props.product);
  if (own > 0) return own;
  if (ctx.item && Number(ctx.item.id) > 0) return Number(ctx.item.id);
  if (ctx.post && ctx.post.type === TYPE && Number(ctx.post.id) > 0) return Number(ctx.post.id);
  return 0;
};

export function priceHTML(p: Product): string {
  return `<span class="shop-price">${p.onSale ? `<del>${money(p.regular)}</del> ` : ''}<b>${money(p.price)}</b></span>`;
}

export function addForm(p: Product, label: string, showQty: boolean): string {
  if (!p.inStock) return `<button type="button" class="shop-btn" disabled>${esc(t('soldOut'))}</button>`;
  const max = p.stock === null ? 99 : Math.min(99, p.stock);
  return `<form class="shop-add" method="post" action="/cart/add" data-shop-add data-added="${esc(t('added'))}" data-view="${esc(t('viewCart'))}">` +
    `<input type="hidden" name="id" value="${p.id}">` +
    (showQty ? `<input type="number" name="qty" value="1" min="1" max="${max}" aria-label="${esc(t('qty'))}">` : '<input type="hidden" name="qty" value="1">') +
    `<button type="submit" class="shop-btn">${esc(label || t('addToCart'))}</button></form>`;
}

/**
 * The price + button under a product's description (post.content filter, 50 ms: no package files read here,
 * so the styles are inline; the form also works without the script).
 */
export function buyBox(p: Product): string {
  const btn = 'min-height:44px;padding:0 20px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer';
  const price = `<span class="shop-price" style="font-size:1.25em">${p.onSale ? `<del style="color:#9ca3af;font-size:.8em">${money(p.regular)}</del> ` : ''}<b>${money(p.price)}</b></span>`;
  const left = p.stock !== null && p.inStock && p.stock <= 5 ? `<span class="shop-muted" style="color:#6b7280">${esc(t('left', { n: p.stock }))}</span>` : '';
  const form = !p.inStock
    ? `<button type="button" class="shop-btn" style="${btn};opacity:.5" disabled>${esc(t('soldOut'))}</button>`
    : `<form class="shop-add" method="post" action="/cart/add" data-shop-add data-added="${esc(t('added'))}" data-view="${esc(t('viewCart'))}" style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">` +
      `<input type="hidden" name="id" value="${p.id}"><input type="number" name="qty" value="1" min="1" max="${p.stock === null ? 99 : Math.min(99, p.stock)}" aria-label="${esc(t('qty'))}" style="width:72px;min-height:40px;padding:0 8px;border:1px solid #d1d5db;border-radius:8px;font:inherit">` +
      `<button type="submit" class="shop-btn" style="${btn}">${esc(t('addToCart'))}</button></form>`;
  return `<div class="shop-buy" data-product="${p.id}" style="display:flex;flex-wrap:wrap;align-items:center;gap:12px;margin:24px 0">${price}${left}${form}</div>` +
    (SCRIPT_URL ? `<script defer src="${esc(SCRIPT_URL)}"></script>` : '');
}

export function registerBlocks(): void {
  // the product select of the property panel (cached a minute: the designer asks for every block at once)
  const productOptions = () => {
    let list = cache.get<{ value: string; label: string }[]>('shop:productOptions');
    if (!list) {
      const { items } = db.posts.list({ type: TYPE, perPage: 100, status: 'publish', order: 'title' });
      list = items.map((p) => ({ value: String(p.id), label: p.title }));
      cache.set('shop:productOptions', list, 60);
    }
    return [{ value: '', label: { th: '— สินค้าของหน้านี้ —', en: '— this page’s product —' } }, ...list];
  };

  site.block('add-to-cart', {
    label: { th: 'ปุ่มใส่ตะกร้า', en: 'Add to cart' },
    icon: '🛒',
    props: [
      { key: 'product', type: 'select', label: { th: 'สินค้า', en: 'Product' },
        help: { th: 'เว้นว่างในแม่แบบหน้าสินค้าหรือการ์ดในรายการสินค้า', en: 'Leave empty in the product template or a product card' } },
      { key: 'label', type: 'text', label: { th: 'ข้อความบนปุ่ม', en: 'Button text' }, maxLength: 40 },
      { key: 'showQty', type: 'boolean', label: { th: 'ให้เลือกจำนวน', en: 'Quantity box' }, default: true },
      { key: 'showPrice', type: 'boolean', label: { th: 'แสดงราคา', en: 'Show the price' }, default: true },
    ],
    options: () => ({ product: productOptions() }),
    script: SCRIPT,
    render: (ctx, props) => {
      const id = idFrom(ctx, props);
      const p = id ? getProduct(id) : null;
      if (!p) return { html: (ctx as any).preview === false ? '' : `<p class="shop-hint">${esc(t('chooseProduct'))}</p>`, css: css() };
      return { html: `<div class="shop-buy">${props.showPrice === false ? '' : priceHTML(p)}${addForm(p, String(props.label ?? ''), props.showQty !== false)}</div>`, css: css() };
    },
  });

  site.block('cart-link', {
    label: { th: 'ลิงก์ตะกร้า', en: 'Cart link' },
    icon: '🧺',
    props: [{ key: 'label', type: 'text', label: { th: 'ข้อความ', en: 'Text' }, maxLength: 30 }],
    script: SCRIPT,
    cache: 3600,
    render: (_ctx, props) => ({
      html: `<a class="shop-cart-link" href="/cart" aria-label="${esc(t('cart'))}"><span aria-hidden="true">🛒</span>` +
        `${props.label ? `<span>${esc(props.label)}</span>` : ''}<span class="shop-count" data-shop-count hidden>0</span></a>`,
      css: css(),
    }),
  });

  site.block('price', {
    label: { th: 'ราคาสินค้า', en: 'Product price' },
    icon: '฿',
    props: [{ key: 'product', type: 'select', label: { th: 'สินค้า', en: 'Product' } }],
    options: () => ({ product: productOptions() }),
    render: (ctx, props) => {
      const id = idFrom(ctx, props);
      const p = id ? getProduct(id) : null;
      if (!p) return { html: (ctx as any).preview === false ? '' : `<p class="shop-hint">${esc(t('chooseProduct'))}</p>`, css: css() };
      return { html: priceHTML(p), css: css() };
    },
  });
}
