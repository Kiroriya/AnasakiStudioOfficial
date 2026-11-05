// The public pages: /cart, /checkout and /shop/order/{code} — inside the site's header and footer, working
// without JavaScript (assets/shop.js only adds the slip upload and quicker cart buttons).
import { CheckoutInput, Line, OrderRow, Payment, esc, methods, money, orderItems, priceCart, shippingFor, Cart } from './model';
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
  const warn = l.problem ? `<div class="shop-warn">${esc(t(l.problem as Key))}${p.stock !== null && l.problem === 'low' ? ' · ' + esc(t('left', { n: p.stock })) : ''}</div>` : '';
  return `<tr data-id="${p.id}"><td class="shop-prod"><a href="${esc(p.url)}">${img}<span>${esc(p.title)}</span></a>${warn}</td>` +
    `<td class="shop-num">${money(p.price)}</td>` +
    `<td><form method="post" action="/cart/update" class="shop-qty"><input type="hidden" name="id" value="${p.id}">` +
    `<input type="number" name="qty" min="0" max="99" value="${l.qty}" aria-label="${esc(t('qty'))}">` +
    `<button type="submit" class="shop-btn-ghost">${esc(t('update'))}</button></form></td>` +
    `<td class="shop-num">${money(l.total)}</td>` +
    `<td><form method="post" action="/cart/update"><input type="hidden" name="id" value="${p.id}"><input type="hidden" name="qty" value="0">` +
    `<button type="submit" class="shop-btn-ghost shop-remove" aria-label="${esc(t('remove'))}">✕</button></form></td></tr>`;
};

const totals = (subtotal: number) => {
  const ship = shippingFor(subtotal);
  return `<dl class="shop-totals"><dt>${esc(t('subtotal'))}</dt><dd>${money(subtotal)}</dd>` +
    `<dt>${esc(t('shipping'))}</dt><dd>${ship ? money(ship) : esc(t('free'))}</dd>` +
    `<dt class="shop-grand">${esc(t('grand'))}</dt><dd class="shop-grand">${money(subtotal + ship)}</dd></dl>`;
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
  const summary = `<aside class="shop-summary"><h2>${esc(t('items'))}</h2><ul>` +
    lines.map((l) => `<li><span>${esc(l.product.title)} × ${l.qty}</span><span>${money(l.total)}</span></li>`).join('') + `</ul>${totals(subtotal)}</aside>`;
  return page(t('checkout'), `<h1>${esc(t('checkout'))}</h1>` + (message ? `<p class="shop-alert" role="alert">${esc(message)}</p>` : '') +
    `<div class="shop-checkout"><form method="post" action="/checkout" class="shop-form" id="shop-checkout">` +
    field('name', 'text', ' required maxlength="191" autocomplete="name"') +
    field('phone', 'tel', ' required maxlength="20" autocomplete="tel"') +
    field('email', 'email', ' maxlength="191" autocomplete="email"') +
    field('address', 'textarea', ' required maxlength="2000" autocomplete="street-address"') +
    field('note', 'textarea', ' maxlength="2000"') + pay +
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
    } else {
      pay = `<div class="shop-pay"><p>${esc(t('codNote', { amount: money(total) }))}</p></div>`;
    }
    if (o.payment !== 'cod') {
      pay += o.slip
        ? `<p class="shop-ok" id="shop-slip-done">${esc(t('slipDone'))}</p>`
        : `<form class="shop-slip" data-shop-slip="${esc(o.code)}" data-bad="${esc(t('slipBad'))}" data-done="${esc(t('slipDone'))}">` +
          `<label>${esc(t('slip'))} <input type="file" accept="image/jpeg,image/png" required></label>` +
          `<button type="submit" class="shop-btn">${esc(t('slipSend'))}</button><p class="shop-muted" role="status"></p></form>`;
    }
  }
  const rows = items.map((i) => `<li><span>${esc(i.title)} × ${i.qty}</span><span>${money(Number(i.price) * i.qty)}</span></li>`).join('');
  return page(t('order') + ' #' + o.id, (notice ? `<p class="shop-ok">${esc(notice)}</p>` : '') +
    `<h1>${esc(t('order'))} #${o.id}</h1><p>${esc(t('status'))}: ${statusBadge(o.status)}</p>` + pay +
    `<section class="shop-summary"><h2>${esc(t('items'))}</h2><ul>${rows}</ul>` +
    `<dl class="shop-totals"><dt>${esc(t('subtotal'))}</dt><dd>${money(Number(o.subtotal))}</dd><dt>${esc(t('shipping'))}</dt>` +
    `<dd>${Number(o.shipping) ? money(Number(o.shipping)) : esc(t('free'))}</dd><dt class="shop-grand">${esc(t('grand'))}</dt><dd class="shop-grand">${money(total)}</dd></dl>` +
    `<h2>${esc(t('shipTo'))}</h2><p class="shop-address">${esc(o.name)}<br>${esc(o.phone)}<br>${esc(o.address).replace(/\n/g, '<br>')}</p></section>` +
    `<p class="shop-muted">${esc(t('keepLink'))}</p>`);
}

export const notFoundPage = () => page(t('notFound'), `<h1>${esc(t('notFound'))}</h1>`, 404);
