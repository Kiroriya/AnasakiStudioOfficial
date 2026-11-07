// The public pages: /cart, /checkout and /shop/order/{code} — inside the site's header and footer, working
// without JavaScript (assets/shop.js only adds the slip upload and quicker cart buttons).
import {
  CheckoutInput, Line, OrderRow, Payment, couponDiscount, coupons, esc, itemTitle, lineTitle, methods, money, orderItems, priceCart, shippingFor,
  trackingURL, Cart,
} from './model';
import { Key, t } from './text';

export function asset(path: string): string {
  try {
    return site.asset(path).url;
  } catch {
    return '';
  }
}

let cssText: string | null = null;
export function css(): string {
  if (cssText === null) {
    try {
      cssText = site.asset('templates/shop.css').text();
    } catch {
      cssText = '';
    }
  }
  return cssText;
}

export const script = () => {
  const u = asset('assets/shop.js');
  return u ? `<script defer src="${esc(u)}"></script>` : '';
};

/** A page in the site's layout. */
export function page(title: string, body: string, status = 200): RouteResult {
  return {
    page: { title, html: `<style>${css()}</style><div class="shop-page">${body}</div>${script()}` },
    status,
    headers: { 'Cache-Control': 'no-store' },
  };
}

const lineRow = (l: Line) => {
  const p = l.product;
  const img = p.image ? `<img src="${esc(p.image)}" alt="" loading="lazy">` : '<span class="shop-noimg"></span>';
  const stock = l.variant ? l.variant.stock : p.stock;
  const warn = l.problem ? `<div class="shop-warn">${esc(t(l.problem as Key))}${stock !== null && l.problem === 'low' ? ' · ' + esc(t('left', { n: stock })) : ''}</div>` : '';
  const v = l.variant ? `<input type="hidden" name="v" value="${l.variant.index}">` : '';
  return `<tr data-id="${p.id}"><td class="shop-prod"><a href="${esc(p.url)}">${img}<span>${esc(lineTitle(l))}</span></a>${warn}</td>` +
    `<td class="shop-num">${money(l.price)}</td>` +
    `<td><form method="post" action="/cart/update" class="shop-qty"><input type="hidden" name="id" value="${p.id}">${v}` +
    `<input type="number" name="qty" min="0" max="99" value="${l.qty}" aria-label="${esc(t('qty'))}">` +
    `<button type="submit" class="shop-btn-ghost">${esc(t('update'))}</button></form></td>` +
    `<td class="shop-num">${money(l.total)}</td>` +
    `<td><form method="post" action="/cart/update"><input type="hidden" name="id" value="${p.id}">${v}<input type="hidden" name="qty" value="0">` +
    `<button type="submit" class="shop-btn-ghost shop-remove" aria-label="${esc(t('remove'))}">✕</button></form></td></tr>`;
};

const totals = (subtotal: number, discount = 0, code = '') => {
  const ship = shippingFor(subtotal - discount);
  return `<dl class="shop-totals"><dt>${esc(t('subtotal'))}</dt><dd>${money(subtotal)}</dd>` +
    (discount > 0 ? `<dt>${esc(t('discount'))} <span class="shop-code">${esc(code)}</span></dt><dd>-${money(discount)}</dd>` : '') +
    `<dt>${esc(t('shipping'))}</dt><dd>${ship ? money(ship) : esc(t('free'))}</dd>` +
    `<dt class="shop-grand">${esc(t('grand'))}</dt><dd class="shop-grand">${money(subtotal - discount + ship)}</dd></dl>`;
};

export function cartPage(c: Cart): RouteResult {
  const { lines, subtotal } = priceCart(c);
  if (!lines.length) {
    return page(t('cart'), `<h1>${esc(t('cart'))}</h1><p class="shop-empty">${esc(t('empty'))}</p><p><a class="shop-btn" href="/">${esc(t('keepShopping'))}</a></p>`);
  }
  const blocked = lines.some((l) => l.problem);
  return page(t('cart'), `<h1>${esc(t('cart'))}</h1>` +
    `<div class="shop-table-wrap"><table class="shop-table"><thead><tr><th>${esc(t('product'))}</th><th class="shop-num">${esc(t('price'))}</th>` +
    `<th>${esc(t('qty'))}</th><th class="shop-num">${esc(t('total'))}</th><th></th></tr></thead><tbody>${lines.map(lineRow).join('')}</tbody></table></div>` +
    totals(subtotal) +
    `<div class="shop-actions"><a class="shop-btn-ghost" href="/">${esc(t('keepShopping'))}</a>` +
    (blocked ? `<span class="shop-btn" aria-disabled="true">${esc(t('toCheckout'))}</span>` : `<a class="shop-btn" href="/checkout" id="shop-to-checkout">${esc(t('toCheckout'))}</a>`) +
    '</div>');
}

const methodLabel = (m: Payment) => t(('m_' + m) as Key);

export function checkoutPage(c: Cart, inp: Partial<CheckoutInput> = {}, errors: Record<string, string> = {}, message = ''): RouteResult {
  const { lines, subtotal } = priceCart(c);
  if (!lines.length) return cartPage(c);
  const ms = methods();
  const field = (key: keyof CheckoutInput, type: string, extra = '') => {
    const err = errors[key] ? `<span class="shop-err">${esc(t(('e_' + errors[key]) as Key))}</span>` : '';
    if (key === 'coupon') extra += ` autocomplete="off" maxlength="40" style="text-transform:uppercase"`;
    const v = esc(inp[key] ?? '');
    const control = type === 'textarea'
      ? `<textarea name="${key}" id="shop-${key}" rows="3"${extra}>${v}</textarea>`
      : `<input type="${type}" name="${key}" id="shop-${key}" value="${v}"${extra}>`;
    return `<div class="shop-field${errors[key] ? ' is-invalid' : ''}"><label for="shop-${key}">${esc(t(key as Key))}</label>${control}${err}</div>`;
  };
  const pay = ms.length
    ? `<fieldset class="shop-field${errors.payment ? ' is-invalid' : ''}"><legend>${esc(t('payWith'))}</legend>` +
      ms.map((m, i) => `<label class="shop-radio"><input type="radio" name="payment" value="${m}"${(inp.payment ? inp.payment === m : i === 0) ? ' checked' : ''}> ${esc(methodLabel(m))}</label>`).join('') +
      (errors.payment ? `<span class="shop-err">${esc(t('e_payment'))}</span>` : '') + '</fieldset>'
    : `<p class="shop-warn">${esc(t('noMethods'))}</p>`;
  // the coupon box shows when the shop has codes; a valid code lowers the summary at once (after a submit)
  const cd = couponDiscount(inp.coupon ?? '', subtotal);
  const summary = `<aside class="shop-summary"><h2>${esc(t('items'))}</h2><ul>` +
    lines.map((l) => `<li><span>${esc(lineTitle(l))} × ${l.qty}</span><span>${money(l.total)}</span></li>`).join('') + `</ul>` +
    `${totals(subtotal, cd.problem ? 0 : cd.discount, cd.coupon?.code ?? '')}</aside>`;
  return page(t('checkout'), `<h1>${esc(t('checkout'))}</h1>` + (message ? `<p class="shop-alert" role="alert">${esc(message)}</p>` : '') +
    `<div class="shop-checkout"><form method="post" action="/checkout" class="shop-form" id="shop-checkout">` +
    field('name', 'text', ' required maxlength="191" autocomplete="name"') +
    field('phone', 'tel', ' required maxlength="20" autocomplete="tel"') +
    field('email', 'email', ' maxlength="191" autocomplete="email"') +
    field('address', 'textarea', ' required maxlength="2000" autocomplete="street-address"') +
    field('note', 'textarea', ' maxlength="2000"') + (coupons().length ? field('coupon', 'text') : '') +
    // 1.1: a full tax invoice (the fields are checked only when the box is ticked)
    `<details class="shop-field shop-tax"${inp.wantTax ? ' open' : ''}><summary><label class="shop-radio"><input type="checkbox" name="wantTax" value="1"${inp.wantTax ? ' checked' : ''}> ${esc(t('wantTax'))}</label></summary>` +
    field('taxName', 'text', ' maxlength="191" autocomplete="organization"') + field('taxId', 'text', ' maxlength="20" inputmode="numeric"') +
    field('taxAddress', 'textarea', ' maxlength="1000"') + '</details>' + pay +
    `<button type="submit" class="shop-btn"${ms.length ? '' : ' disabled'}>${esc(t('placeOrder'))}</button></form>${summary}</div>`);
}

const statusBadge = (s: string) => `<span class="shop-status shop-status-${esc(s)}">${esc(t(('s_' + s) as Key))}</span>`;

export function orderPage(o: OrderRow, notice = ''): RouteResult {
  const items = orderItems(o.id);
  const total = Number(o.total);
  let pay = '';
  if (o.status === 'pending') {
    if (o.payment === 'promptpay') {
      const nm = String(site.settings('promptpayName') ?? '').trim();
      pay = `<div class="shop-pay"><img class="shop-qr" src="/shop/order/${esc(o.code)}/qr.svg" width="240" height="240" alt="PromptPay QR">` +
        `<p>${esc(t('scan', { amount: money(total) }))}</p>${nm ? `<p class="shop-muted">${esc(t('account', { name: nm }))}</p>` : ''}</div>`;
    } else if (o.payment === 'bank') {
      pay = `<div class="shop-pay"><p>${esc(t('transferTo', { amount: money(total) }))}</p><pre class="shop-bank">${esc(site.settings('bankInfo') ?? '')}</pre></div>`;
    } else if (o.payment === 'card') {
      pay = `<div class="shop-pay"><form method="post" action="/shop/order/${esc(o.code)}/pay"><button type="submit" class="shop-btn" id="shop-pay-card">💳 ${esc(t('payCard', { amount: money(total) }))}</button></form></div>`;
    } else {
      pay = `<div class="shop-pay"><p>${esc(t('codNote', { amount: money(total) }))}</p></div>`;
    }
    if (o.payment !== 'cod' && o.payment !== 'card') {
      pay += o.slip
        ? `<p class="shop-ok" id="shop-slip-done">${esc(t('slipDone'))}</p>`
        : `<form class="shop-slip" data-shop-slip="${esc(o.code)}" data-bad="${esc(t('slipBad'))}" data-done="${esc(t('slipDone'))}">` +
          `<label>${esc(t('slip'))} <input type="file" accept="image/jpeg,image/png" required></label>` +
          `<button type="submit" class="shop-btn">${esc(t('slipSend'))}</button><p class="shop-muted" role="status"></p></form>`;
    }
  }
  const rows = items.map((i) => `<li><span>${esc(itemTitle(i))} × ${i.qty}</span><span>${money(Number(i.price) * i.qty)}</span></li>`).join('');
  const turl = trackingURL(o.carrier, o.tracking);
  const track = o.tracking
    ? `<div class="shop-pay shop-track"><p>${esc(t('tracking'))}${o.carrier ? ' · ' + esc(o.carrier) : ''}</p><p class="shop-grand">${esc(o.tracking)}</p>` +
      (turl ? `<a class="shop-btn" href="${esc(turl)}" target="_blank" rel="noopener">${esc(t('trackIt'))}</a>` : '') + '</div>'
    : '';
  return page(t('order') + ' #' + o.id, (notice ? `<p class="shop-ok">${esc(notice)}</p>` : '') +
    `<h1>${esc(t('order'))} #${o.id}</h1><p>${esc(t('status'))}: ${statusBadge(o.status)}</p>` + track + pay +
    `<section class="shop-summary"><h2>${esc(t('items'))}</h2><ul>${rows}</ul>` +
    `<dl class="shop-totals"><dt>${esc(t('subtotal'))}</dt><dd>${money(Number(o.subtotal))}</dd>` +
    (Number(o.discount) > 0 ? `<dt>${esc(t('discount'))} <span class="shop-code">${esc(o.coupon)}</span></dt><dd>-${money(Number(o.discount))}</dd>` : '') +
    `<dt>${esc(t('shipping'))}</dt>` +
    `<dd>${Number(o.shipping) ? money(Number(o.shipping)) : esc(t('free'))}</dd><dt class="shop-grand">${esc(t('grand'))}</dt><dd class="shop-grand">${money(total)}</dd></dl>` +
    `<h2>${esc(t('shipTo'))}</h2><p class="shop-address">${esc(o.name)}<br>${esc(o.phone)}<br>${esc(o.address).replace(/\n/g, '<br>')}</p></section>` +
    (o.receipt_no ? `<p><a class="shop-btn-ghost" id="shop-receipt" href="/shop/order/${esc(o.code)}/receipt">${esc(t('receipt'))}</a></p>` : '') +
    `<p class="shop-muted">${esc(t('keepLink'))}</p>`);
}

/** The printable receipt / tax invoice of a paid order (a page of its own, saved as PDF by the browser). */
export function receiptPage(o: OrderRow): RouteResult {
  const s = (k: string) => String(site.settings(k) ?? '').trim();
  const vat = site.settings('vatRegistered') === true;
  const total = Number(o.total);
  const d = new Date(o.paid_at ?? o.created_at);
  const th = t('date') === 'วันที่';
  const day = th ? `${d.getDate()}/${d.getMonth() + 1}/${d.getFullYear() + 543}` : d.toISOString().slice(0, 10);
  const rows = orderItems(o.id).map((i) => `<tr><td>${esc(itemTitle(i))}</td><td class="n">${i.qty}</td><td class="n">${money(Number(i.price))}</td><td class="n">${money(Number(i.price) * i.qty)}</td></tr>`).join('');
  const before = vat ? Math.round((total / 1.07) * 100) / 100 : 0;
  const buyer = o.tax_id ? `${esc(o.tax_name)}<br>${esc(t('taxId'))}: ${esc(o.tax_id)}<br>${esc(o.tax_address ?? '').replace(/\n/g, '<br>')}`
    : `${esc(o.name)}<br>${esc(o.address).replace(/\n/g, '<br>')}`;
  const html = `<!doctype html><html lang="${th ? 'th' : 'en'}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">` +
    `<title>${esc(vat ? t('taxInvoice') : t('receiptTitle'))} ${esc(o.receipt_no)}</title><style>body{margin:0;background:#f3f4f6;font-family:'Noto Sans Thai',system-ui,sans-serif;color:#111827}` +
    `.r{max-width:800px;margin:24px auto;background:#fff;padding:40px;border:1px solid #e5e7eb}h1{font-size:24px;margin:0 0 4px}.top{display:flex;justify-content:space-between;gap:24px;flex-wrap:wrap}` +
    `table{width:100%;border-collapse:collapse;margin:24px 0}th,td{padding:8px;border-bottom:1px solid #e5e7eb;text-align:left}.n{text-align:right;white-space:nowrap}` +
    `.sum{margin-left:auto;max-width:320px}.sum td{border:0;padding:4px 8px}.g{font-weight:700;font-size:18px}button{font:inherit;padding:10px 18px;border-radius:8px;border:1px solid #d1d5db;background:#fff;cursor:pointer}` +
    `@media print{body{background:#fff}.r{border:0;margin:0}button{display:none}}</style></head><body><div class="r">` +
    `<div class="top"><div><h1>${esc(vat ? t('taxInvoice') : t('receiptTitle'))}</h1><p><b>${esc(s('sellerName'))}</b>${s('sellerTaxId') ? `<br>${esc(t('taxId'))}: ${esc(s('sellerTaxId'))}` : ''}` +
    `${s('sellerAddress') ? `<br>${esc(s('sellerAddress')).replace(/\n/g, '<br>')}` : ''}</p></div><div><p>${esc(t('receiptNo'))}: <b>${esc(o.receipt_no)}</b><br>${esc(t('date'))}: ${esc(day)}<br>${esc(t('order'))} #${o.id}</p></div></div>` +
    `<p><b>${esc(t('buyer'))}</b><br>${buyer}</p>` +
    `<table><thead><tr><th>${esc(t('product'))}</th><th class="n">${esc(t('qty'))}</th><th class="n">${esc(t('price'))}</th><th class="n">${esc(t('total'))}</th></tr></thead><tbody>${rows}</tbody></table>` +
    `<table class="sum"><tr><td>${esc(t('subtotal'))}</td><td class="n">${money(Number(o.subtotal))}</td></tr>` +
    (Number(o.discount) > 0 ? `<tr><td>${esc(t('discount'))}</td><td class="n">-${money(Number(o.discount))}</td></tr>` : '') +
    `<tr><td>${esc(t('shipping'))}</td><td class="n">${money(Number(o.shipping))}</td></tr>` +
    (vat ? `<tr><td>${esc(t('beforeVat'))}</td><td class="n">${money(before)}</td></tr><tr><td>${esc(t('vat'))}</td><td class="n">${money(Math.round((total - before) * 100) / 100)}</td></tr>` : '') +
    `<tr><td class="g">${esc(t('grand'))}</td><td class="n g">${money(total)}</td></tr></table>` +
    `<p style="text-align:center"><button onclick="print()">🖨 ${esc(t('print'))}</button></p></div></body></html>`;
  return { status: 200, headers: { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' }, body: html };
}

/** A member's orders (newest first). */
export function ordersPage(list: OrderRow[]): RouteResult {
  const rows = list.map((o) => `<li><a href="/shop/order/${esc(o.code)}"><b>#${o.id}</b></a> · ${esc(String(o.created_at).slice(0, 10))} · ${money(Number(o.total))} · ` +
    `${statusBadge(o.status)}${o.receipt_no ? ` · <a href="/shop/order/${esc(o.code)}/receipt">${esc(t('receipt'))}</a>` : ''}</li>`).join('');
  return page(t('myOrders'), `<h1>${esc(t('myOrders'))}</h1>` + (rows ? `<ul class="shop-orders">${rows}</ul>` : `<p class="shop-empty">${esc(t('noOrders'))}</p>`));
}

export const notFoundPage = () => page(t('notFound'), `<h1>${esc(t('notFound'))}</h1>`, 404);
