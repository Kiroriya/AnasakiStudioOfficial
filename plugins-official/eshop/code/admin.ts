// The "Orders" admin page (admin UI kit, drawn on the server; assets/admin.js runs the status buttons through
// anasaki.call) and a dashboard card. Capability shop.orders (CMS 2.0.6+), else editors.
import { OrderRow, STATUSES, Status, counts, esc, getOrder, itemTitle, listOrders, money, orderItems, setStatus, setTracking } from './model';
import { statusChanged } from './events';

const CAPS = typeof (site as any).can === 'function';
const ACCESS = CAPS ? ['cap:eshop.orders'] : ['editor'];

const L = {
  title: ['คำสั่งซื้อ', 'Orders'],
  desc: ['คำสั่งซื้อจากหน้าร้าน ตรวจสลิป เปลี่ยนสถานะ แล้วระบบส่งอีเมลแจ้งลูกค้าให้', 'Orders from the shop: check slips and change the status; customers are mailed'],
  all: ['ทั้งหมด', 'All'],
  search: ['ค้นหาชื่อ เบอร์ อีเมล หรือรหัส', 'Search name, phone, email or code'],
  none: ['ยังไม่มีคำสั่งซื้อ', 'No orders yet'],
  no: ['เลขที่', 'No.'], customer: ['ลูกค้า', 'Customer'], total: ['ยอด', 'Total'], pay: ['ชำระด้วย', 'Payment'],
  status: ['สถานะ', 'Status'], date: ['วันที่', 'Date'], slip: ['สลิป', 'Slip'], back: ['← คำสั่งซื้อทั้งหมด', '← All orders'],
  items: ['รายการ', 'Items'], shipTo: ['จัดส่งถึง', 'Ship to'], note: ['หมายเหตุ', 'Note'], csv: ['ส่งออก CSV', 'Export CSV'],
  setTo: ['เปลี่ยนเป็น', 'Mark as'], noSlip: ['ยังไม่มีสลิป', 'No slip yet'], download: ['ดาวน์โหลด', 'Download'],
  view: ['หน้าคำสั่งซื้อของลูกค้า ↗', 'Customer’s order page ↗'], products: ['จัดการสินค้า', 'Products'], settings: ['ตั้งค่าร้าน', 'Shop settings'],
  pending: ['รอชำระ', 'Pending'], paid: ['ชำระแล้ว', 'Paid'], shipped: ['จัดส่งแล้ว', 'Shipped'], done: ['สำเร็จ', 'Completed'], cancelled: ['ยกเลิก', 'Cancelled'],
  promptpay: ['พร้อมเพย์', 'PromptPay'], bank: ['โอนเงิน', 'Bank transfer'], cod: ['ปลายทาง', 'Cash on delivery'], card: ['บัตร (ออนไลน์)', 'Card (online)'],
  widget: ['ร้านค้า', 'Shop'], waiting: ['รอตรวจ/รอชำระ', 'Awaiting payment'], toShip: ['รอจัดส่ง', 'To ship'], today: ['ยอดวันนี้', 'Today'],
  prev: ['‹ ก่อนหน้า', '‹ Previous'], next: ['ถัดไป ›', 'Next ›'],
  discount: ['ส่วนลด', 'Discount'], tracking: ['เลขพัสดุ', 'Tracking number'], carrier: ['ขนส่ง', 'Carrier'],
  trackSave: ['บันทึกเลขพัสดุ', 'Save tracking'], trackShip: ['บันทึกและเปลี่ยนเป็นจัดส่งแล้ว', 'Save and mark as shipped'],
  trackHint: ['ลูกค้าเห็นเลขพัสดุในหน้าคำสั่งซื้อและในอีเมลแจ้งจัดส่ง (ลิงก์ติดตามของไปรษณีย์ไทย Kerry Flash และ J&T)',
    'Customers see it on their order page and in the shipped mail (tracking links for Thailand Post, Kerry, Flash and J&T)'],
} as const;
type LK = keyof typeof L;

const jsonData = (v: unknown) => JSON.stringify(v).replace(/</g, '\\u003c').replace(/>/g, '\\u003e').replace(/&/g, '\\u0026');
const q1 = (v: string | string[] | undefined) => String(Array.isArray(v) ? v[0] : v ?? '');
const badge = (s: string, tr: (k: LK) => string) => {
  const cls = s === 'pending' ? 'ak-badge-warn' : s === 'cancelled' ? 'ak-badge-danger' : s === 'done' ? 'ak-badge-ok' : 'ak-badge-info';
  return `<span class="ak-badge ${cls}">${esc(tr(s as LK))}</span>`;
};
const when = (s: string | null) => (s ? String(s).replace('T', ' ').slice(0, 16) : '');

function asset(path: string): string {
  try {
    return site.asset(path).url;
  } catch {
    return '';
  }
}

function b64(bytes: Uint8Array): string {
  const A = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  let out = '';
  for (let i = 0; i < bytes.length; i += 3) {
    const n = (bytes[i] << 16) | ((bytes[i + 1] ?? 0) << 8) | (bytes[i + 2] ?? 0);
    out += A[(n >> 18) & 63] + A[(n >> 12) & 63] + (i + 1 < bytes.length ? A[(n >> 6) & 63] : '=') + (i + 2 < bytes.length ? A[n & 63] : '=');
  }
  return out;
}

function detail(o: OrderRow, tr: (k: LK) => string): string {
  const rows = orderItems(o.id).map((i) =>
    `<tr><td>${esc(itemTitle(i))}</td><td class="ak-num">${i.qty}</td><td class="ak-num">${money(Number(i.price) * i.qty)}</td></tr>`).join('') +
    (Number(o.discount) > 0 ? `<tr><td>${esc(tr('discount'))} <code>${esc(o.coupon)}</code></td><td></td><td class="ak-num">-${money(Number(o.discount))}</td></tr>` : '');
  const carriers = ['ไปรษณีย์ไทย', 'Kerry Express', 'Flash Express', 'J&T Express', 'Ninja Van', 'Lalamove'];
  const track = o.status === 'cancelled' ? '' :
    `<div class="ak-card"><div class="ak-card-head"><h3>${esc(tr('tracking'))}</h3></div>` +
    `<div class="ak-actions" style="flex-wrap:wrap"><input id="shop-carrier" list="shop-carriers" value="${esc(o.carrier)}" placeholder="${esc(tr('carrier'))}" maxlength="60" style="max-width:200px">` +
    `<datalist id="shop-carriers">${carriers.map((c) => `<option value="${esc(c)}">`).join('')}</datalist>` +
    `<input id="shop-tracking" value="${esc(o.tracking)}" placeholder="${esc(tr('tracking'))}" maxlength="120" style="max-width:260px">` +
    `<button class="ak-btn ak-btn-sm" data-shop-track="${o.id}">${esc(tr('trackSave'))}</button>` +
    (o.status === 'paid' ? `<button class="ak-btn ak-btn-sm ak-btn-primary" data-shop-track="${o.id}" data-ship="1">${esc(tr('trackShip'))}</button>` : '') +
    `</div><p class="ak-hint">${esc(tr('trackHint'))}</p></div>`;
  let slip = `<p class="ak-muted">${esc(tr('noSlip'))}</p>`;
  if (o.slip) {
    const bytes = files.get(o.slip);
    const type = o.slip.endsWith('.png') ? 'image/png' : 'image/jpeg';
    slip = bytes ? `<img src="data:${type};base64,${b64(bytes)}" alt="slip" style="max-width:100%;max-height:520px;border-radius:8px;border:1px solid var(--ak-border)">` +
      `<p class="ak-hint">${esc(when(o.slip_at))} · <a href="#" data-ak-download="slip=${o.id}">${esc(tr('download'))}</a></p>` : slip;
  }
  const buttons = STATUSES.filter((s) => s !== o.status).map((s) =>
    `<button class="ak-btn ak-btn-sm${s === 'cancelled' ? ' ak-btn-danger' : s === 'paid' ? ' ak-btn-primary' : ''}" data-shop-status="${s}" data-id="${o.id}">${esc(tr(s as LK))}</button>`).join('');
  return `<p><a href="?" class="ak-btn ak-btn-sm ak-btn-ghost">${esc(tr('back'))}</a></p>` +
    `<header class="ak-page-head"><div class="ak-page-head-text"><h1>#${o.id} ${badge(o.status, tr)}</h1><p>${esc(when(o.created_at))} · ${esc(tr(o.payment as LK))} · <b>${money(Number(o.total))}</b></p></div>` +
    `<div class="ak-actions"><a class="ak-btn ak-btn-sm" href="/shop/order/${esc(o.code)}" target="_blank" rel="noopener">${esc(tr('view'))}</a></div></header>` +
    `<div class="ak-card"><div class="ak-card-head"><h3>${esc(tr('setTo'))}</h3></div><div class="ak-actions" id="shop-status-buttons">${buttons}</div></div>` + track +
    `<div class="ak-grid" style="display:grid;gap:16px;grid-template-columns:repeat(auto-fit,minmax(280px,1fr))">` +
    `<section class="ak-card"><h3>${esc(tr('items'))}</h3><div class="ak-table-wrap"><table class="ak-table"><tbody>${rows}` +
    `<tr><td>${esc(tr('total'))}</td><td></td><td class="ak-num"><b>${money(Number(o.total))}</b></td></tr></tbody></table></div>` +
    `<h3>${esc(tr('shipTo'))}</h3><p>${esc(o.name)}<br>${esc(o.phone)}${o.email ? '<br>' + esc(o.email) : ''}<br>${esc(o.address).replace(/\n/g, '<br>')}</p>` +
    (o.note ? `<h3>${esc(tr('note'))}</h3><p>${esc(o.note)}</p>` : '') + '</section>' +
    `<section class="ak-card"><h3>${esc(tr('slip'))}</h3>${slip}</section></div>`;
}

function list(query: Record<string, string | string[]>, tr: (k: LK) => string): string {
  const status = q1(query.status);
  const search = q1(query.q);
  const page = Math.max(1, Number(q1(query.page)) || 1);
  const r = listOrders({ status, search, page, perPage: 30 });
  const c = counts();
  const all = Object.values(c).reduce((s, n) => s + n, 0);
  const tab = (s: string, label: string, n: number) =>
    `<a class="ak-tab" href="?status=${s}" aria-selected="${status === s}">${esc(label)} (${n})</a>`;
  const rows = r.items.map((o) =>
    `<tr><td><a href="?order=${o.id}"><b>#${o.id}</b></a></td><td>${esc(o.name)}<div class="ak-hint">${esc(o.phone)}</div></td>` +
    `<td class="ak-num">${money(Number(o.total))}</td><td>${esc(tr(o.payment as LK))}${o.slip ? ' 🧾' : ''}</td><td>${badge(o.status, tr)}</td>` +
    `<td class="ak-hint">${esc(when(o.created_at))}</td></tr>`).join('');
  const pages = Math.ceil(r.total / r.perPage);
  const link = (p: number, label: string) => `<a class="ak-btn ak-btn-sm" href="?status=${esc(status)}&q=${encodeURIComponent(search)}&page=${p}">${esc(label)}</a>`;
  return `<div class="ak-tabs">${tab('', tr('all'), all)}${STATUSES.map((s) => tab(s, tr(s as LK), c[s] ?? 0)).join('')}</div>` +
    `<form class="ak-actions" method="get" style="margin-bottom:12px"><input type="hidden" name="status" value="${esc(status)}">` +
    `<input name="q" value="${esc(search)}" placeholder="${esc(tr('search'))}" style="max-width:320px">` +
    `<button class="ak-btn">🔍</button><span style="flex:1"></span><a href="#" class="ak-btn" data-ak-download="csv=1&status=${esc(status)}">⬇ ${esc(tr('csv'))}</a></form>` +
    (r.items.length
      ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="shop-orders"><thead><tr><th>${esc(tr('no'))}</th><th>${esc(tr('customer'))}</th>` +
        `<th class="ak-num">${esc(tr('total'))}</th><th>${esc(tr('pay'))}</th><th>${esc(tr('status'))}</th><th>${esc(tr('date'))}</th></tr></thead><tbody>${rows}</tbody></table></div></div>`
      : `<div class="ak-card"><p class="ak-muted">${esc(tr('none'))}</p></div>`) +
    (pages > 1 ? `<div class="ak-actions" style="margin-top:12px">${page > 1 ? link(page - 1, tr('prev')) : ''}<span class="ak-hint">${page} / ${pages}</span>${page < pages ? link(page + 1, tr('next')) : ''}</div>` : '');
}

const csvCell = (v: unknown) => {
  const s = String(v ?? '');
  return /[",\n\r]|^[=+\-@]/.test(s) ? '"' + s.replace(/^([=+\-@])/, "'$1").replace(/"/g, '""') + '"' : s;
};

export function registerAdmin(): void {
  site.adminPage('orders', {
    label: { th: 'คำสั่งซื้อ', en: 'Orders' },
    icon: '🧾',
    access: ACCESS,
    render: (ctx) => {
      const en = ctx.locale === 'en' ? 1 : 0;
      const tr = (k: LK) => L[k][en];
      const oid = Number(q1(ctx.query.order)) || 0;
      const o = oid ? getOrder(oid) : null;
      const head = `<header class="ak-page-head"><span class="ak-page-icon">🧾</span><div class="ak-page-head-text"><h1>${esc(tr('title'))}</h1><p>${esc(tr('desc'))}</p></div></header>`;
      return {
        title: tr('title'),
        html: (o ? detail(o, tr) : head + list(ctx.query, tr)) +
          `<script type="application/json" id="shop-admin-data">${jsonData({ locale: en ? 'en' : 'th' })}</script>` +
          `<script src="${asset('assets/admin.js')}" defer></script>`,
      };
    },
    download: (ctx) => {
      const slipId = Number(q1(ctx.query.slip)) || 0;
      if (slipId) {
        const o = getOrder(slipId);
        const bytes = o?.slip ? files.get(o.slip) : null;
        if (!o || !bytes) return { status: 404, headers: { 'Content-Type': 'text/plain' }, body: 'not found' };
        return { status: 200, headers: { 'Content-Type': o.slip.endsWith('.png') ? 'image/png' : 'image/jpeg', 'Content-Disposition': `attachment; filename="slip-${o.id}${o.slip.endsWith('.png') ? '.png' : '.jpg'}"` }, body: bytes };
      }
      const status = q1(ctx.query.status);
      const lines = ['id,code,date,status,payment,name,phone,email,address,items,subtotal,discount,coupon,shipping,total,carrier,tracking'];
      for (let page = 1; page <= 200; page++) {
        const r = listOrders({ status, page, perPage: 100 });
        for (const o of r.items) {
          const items = orderItems(o.id).map((i) => `${itemTitle(i)} x${i.qty}`).join('; ');
          lines.push([o.id, o.code, when(o.created_at), o.status, o.payment, o.name, o.phone, o.email, o.address, items, o.subtotal, o.discount, o.coupon,
            o.shipping, o.total, o.carrier, o.tracking].map(csvCell).join(','));
        }
        if (page * r.perPage >= r.total) break;
      }
      return { status: 200, headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': 'attachment; filename="orders.csv"', 'Cache-Control': 'no-store' }, body: '\ufeff' + lines.join('\r\n') };
    },
  });

  site.action('setStatus', {
    input: { id: 'number', status: 'text' },
    access: ACCESS,
    run: ({ input }) => {
      const o = getOrder(Number(input.id));
      if (!o) return { message: 'not found', errors: { id: 'not found' } };
      if (!STATUSES.includes(input.status as Status)) return { message: 'bad status', errors: { status: 'bad status' } };
      const after = setStatus(o, input.status as Status);
      statusChanged(o, after);
      return { data: { id: after.id, status: after.status } };
    },
  });

  // the parcel's carrier and number; ship = also mark a paid order as shipped (the customer is mailed)
  site.action('setTracking', {
    input: { id: 'number', carrier: 'text?', tracking: 'text?', ship: 'bool' },
    access: ACCESS,
    run: ({ input }) => {
      const o = getOrder(Number(input.id));
      if (!o) return { message: 'not found', errors: { id: 'not found' } };
      let after = setTracking(o, String(input.carrier ?? ''), String(input.tracking ?? ''));
      if (input.ship && after.status === 'paid' && after.tracking) {
        const shipped = setStatus(after, 'shipped');
        statusChanged(after, shipped);
        after = shipped;
      }
      return { data: { id: after.id, status: after.status, tracking: after.tracking } };
    },
  });

  site.dashboardWidget('summary', {
    label: { th: 'ร้านค้า', en: 'Shop' },
    icon: '🛒',
    access: ACCESS,
    height: 150,
    render: (ctx) => {
      const en = ctx.locale === 'en' ? 1 : 0;
      const tr = (k: LK) => L[k][en];
      const c = counts();
      const start = new Date();
      start.setHours(0, 0, 0, 0);
      const today = Number(db.table('orders').whereIn('status', ['paid', 'shipped', 'done']).where('paid_at', '>=', start).sum('total') ?? 0);
      const stat = (n: string, label: string) => `<div class="ak-stat"><b>${esc(n)}</b><span>${esc(label)}</span></div>`;
      return `<div style="display:flex;gap:28px;flex-wrap:wrap">${stat(String(c.pending ?? 0), tr('waiting'))}${stat(String(c.paid ?? 0), tr('toShip'))}${stat(money(today), tr('today'))}</div>`;
    },
  });
}
