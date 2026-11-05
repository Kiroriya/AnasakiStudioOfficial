// booking 1.0 — appointments: services with a length and price, weekly hours and closed days (admin page), and a
// booking page that works without JavaScript: /booking → service → day → free time → details → /booking/b/<code>
// (the customer's page, with a cancel button). No double booking beyond a service's capacity.
import {
  Booking, Service, bookableDays, byCode, esc, freeSlots, getBooking, getService, hhmm, isDay, localNow, services, weekday,
} from './model';
import { registerAdmin } from './admin';
import { lang, t } from './text';

const DAYS = [['อา.', 'จ.', 'อ.', 'พ.', 'พฤ.', 'ศ.', 'ส.'], ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']];
const MONTHS = [['ม.ค.', 'ก.พ.', 'มี.ค.', 'เม.ย.', 'พ.ค.', 'มิ.ย.', 'ก.ค.', 'ส.ค.', 'ก.ย.', 'ต.ค.', 'พ.ย.', 'ธ.ค.'],
  ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']];
export function dayLabel(day: string, long = false): string {
  const l = lang();
  const [y, m, d] = day.split('-').map(Number);
  const w = DAYS[l][weekday(day)];
  return long ? `${w} ${d} ${MONTHS[l][m - 1]} ${l === 0 ? y + 543 : y}` : `${w} ${d} ${MONTHS[l][m - 1]}`;
}
const price = (p: string | number) => (Number(p) > 0 ? '฿' + Number(p).toFixed(2).replace(/\B(?=(\d{3})+(?!\d))/g, ',') : '');

const CSS = `.bk{max-width:760px;margin:48px auto;padding:0 20px}.bk h1{font-size:28px;margin:0 0 6px}.bk .bk-sub{color:#6b7280;margin:0 0 20px}
.bk-steps{display:flex;gap:8px;flex-wrap:wrap;margin:0 0 18px;font-size:.9em;color:#6b7280}.bk-steps b{color:inherit}
.bk-grid{display:grid;gap:12px;grid-template-columns:repeat(auto-fill,minmax(200px,1fr))}
.bk-card{display:block;padding:16px;border:1px solid #e5e7eb;border-radius:14px;color:inherit;text-decoration:none;background:#fff}
.bk-card:hover{border-color:var(--c-primary,#4f46e5)}.bk-card small{display:block;color:#6b7280;margin-top:4px}
.bk-days{display:flex;gap:8px;overflow-x:auto;padding-bottom:6px}.bk-day{flex:0 0 auto;min-width:76px;text-align:center;padding:10px 8px;border:1px solid #e5e7eb;border-radius:12px;color:inherit;text-decoration:none;background:#fff}
.bk-day[aria-disabled=true]{opacity:.4;pointer-events:none}.bk-day[aria-current=true]{border-color:var(--c-primary,#4f46e5);background:var(--c-primary,#4f46e5);color:#fff}
.bk-times{display:grid;gap:8px;grid-template-columns:repeat(auto-fill,minmax(86px,1fr));margin-top:16px}
.bk-time{padding:10px;border:1px solid #d1d5db;border-radius:10px;text-align:center;color:inherit;text-decoration:none;font-weight:600}
.bk-time:hover{border-color:var(--c-primary,#4f46e5);color:var(--c-primary,#4f46e5)}
.bk-form{display:flex;flex-direction:column;gap:12px;max-width:460px}.bk-form label{display:flex;flex-direction:column;gap:6px;font-weight:600;font-size:.92em}
.bk-form input,.bk-form textarea{font:inherit;font-weight:400;padding:10px 12px;border:1px solid #d1d5db;border-radius:10px}
.bk-btn{display:inline-flex;align-items:center;justify-content:center;min-height:46px;padding:0 22px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer;text-decoration:none}
.bk-btn-ghost{background:#fff;color:#dc2626;border:1px solid #fecaca}
.bk-sum{padding:16px;border-radius:14px;background:#f9fafb;margin:0 0 16px;line-height:1.7}
.bk-msg{padding:12px 14px;border-radius:10px;background:#dcfce7;color:#166534}.bk-bad{background:#fee2e2;color:#991b1b}.bk-empty{color:#6b7280}`;

const page = (title: string, html: string, status = 200): RouteResult => ({
  page: { title, html: `<style>${CSS}</style><div class="bk">${html}</div>` }, status, headers: { 'Cache-Control': 'no-store' },
});
const q1 = (v: string | string[] | undefined) => String(Array.isArray(v) ? v[0] : v ?? '');
const steps = (n: number) => `<p class="bk-steps">${[t('stepService'), t('stepDay'), t('stepTime'), t('stepDetails')]
  .map((s, i) => (i === n ? `<b>${i + 1}. ${esc(s)}</b>` : `${i + 1}. ${esc(s)}`)).join(' › ')}</p>`;
const svcLine = (s: Service) => `${esc(s.name)} · ${s.minutes} ${esc(t('min'))}${price(s.price) ? ' · ' + price(s.price) : ''}`;

site.route('GET /booking', { access: ['*'] }, ({ request }) => {
  const list = services();
  const sid = Number(q1(request.query.service)) || (list.length === 1 ? list[0].id : 0);
  const svc = sid ? getService(sid) : null;
  if (!svc || !svc.active) {
    return page(t('title'), `<h1>${esc(t('title'))}</h1>${steps(0)}` + (list.length
      ? `<div class="bk-grid">${list.map((s) => `<a class="bk-card" href="/booking?service=${s.id}"><b>${esc(s.name)}</b><small>${s.minutes} ${esc(t('min'))}${price(s.price) ? ' · ' + price(s.price) : ''}</small>${s.description ? `<small>${esc(s.description)}</small>` : ''}</a>`).join('')}</div>`
      : `<p class="bk-empty">${esc(t('noServices'))}</p>`));
  }
  const days = bookableDays();
  const day = q1(request.query.date);
  const chosen = isDay(day) && days.some((d) => d.day === day && d.open) ? day : '';
  const dayChips = `<div class="bk-days" role="list">${days.map((d) => `<a role="listitem" class="bk-day" href="/booking?service=${svc.id}&date=${d.day}"` +
    `${d.open ? '' : ' aria-disabled="true"'}${d.day === chosen ? ' aria-current="true"' : ''}>${esc(dayLabel(d.day))}</a>`).join('')}</div>`;
  const head = `<h1>${esc(t('title'))}</h1><p class="bk-sub">${svcLine(svc)} · <a href="/booking">${esc(t('change'))}</a></p>`;
  if (!chosen) return page(t('title'), head + steps(1) + dayChips);
  const time = Number(q1(request.query.time));
  const slots = freeSlots(svc, chosen);
  if (!q1(request.query.time) || !slots.includes(time)) {
    return page(t('title'), head + steps(2) + dayChips + (slots.length
      ? `<div class="bk-times">${slots.map((s) => `<a class="bk-time" href="/booking?service=${svc.id}&date=${chosen}&time=${s}">${hhmm(s)}</a>`).join('')}</div>`
      : `<p class="bk-empty" style="margin-top:16px">${esc(t('full'))}</p>`));
  }
  return page(t('title'), head + steps(3) +
    `<div class="bk-sum">📅 ${esc(dayLabel(chosen, true))} · ${hhmm(time)}–${hhmm(time + Number(svc.minutes))}<br>${svcLine(svc)}</div>` +
    `<form class="bk-form" method="post" action="/booking"><input type="hidden" name="service" value="${svc.id}"><input type="hidden" name="date" value="${chosen}"><input type="hidden" name="time" value="${time}">` +
    `<label>${esc(t('name'))}<input name="name" required maxlength="120" autocomplete="name"></label>` +
    `<label>${esc(t('phone'))}<input type="tel" name="phone" required maxlength="40" autocomplete="tel"></label>` +
    `<label>${esc(t('email'))}<input type="email" name="email" maxlength="191" autocomplete="email"></label>` +
    `<label>${esc(t('note'))}<textarea name="note" rows="3" maxlength="1000"></textarea></label>` +
    '<div style="position:absolute;left:-9999px" aria-hidden="true"><input name="website" tabindex="-1" autocomplete="off"></div>' +
    `<button class="bk-btn" type="submit">${esc(t('confirm'))}</button></form>`);
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

site.route('POST /booking', { access: ['*'] }, ({ request }) => {
  const b = formOf(request);
  if (String(b.website ?? '').trim()) return page(t('title'), `<p class="bk-msg">${esc(t('thanks'))}</p>`);
  const svc = getService(Number(b.service) || 0);
  const day = String(b.date ?? '');
  const time = Number(b.time);
  const back = svc ? `/booking?service=${svc.id}&date=${encodeURIComponent(day)}` : '/booking';
  const name = String(b.name ?? '').trim(), phone = String(b.phone ?? '').trim(), email = String(b.email ?? '').trim().toLowerCase();
  const bad = (msg: string, status = 422) => page(t('title'), `<p class="bk-msg bk-bad">${esc(msg)}</p><p><a href="${esc(back)}">← ${esc(t('back'))}</a></p>`, status);
  if (!svc || !svc.active) return bad(t('noService'), 404);
  if (!name || name.length > 120 || !/^[0-9+\-\s()]{9,40}$/.test(phone) || (email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email))) return bad(t('check'));
  const key = 'bk:rate:' + crypto.hash('sha256', request.ip || '-');
  const n = Number(cache.get(key) ?? 0);
  if (n >= 5) return bad(t('busy'), 429);
  cache.set(key, n + 1, 600);
  const code = crypto.random(12);
  const status = site.settings('autoConfirm') === false ? 'pending' : 'confirmed';
  // the slot is checked again right before it is booked (someone may have taken it meanwhile)
  if (!freeSlots(svc, day).includes(time)) return bad(t('taken'), 409);
  const id = db.table('bookings').insert({ code, service_id: svc.id, service_name: svc.name, day, start_min: time, end_min: time + Number(svc.minutes),
    name, phone, email, note: String(b.note ?? '').trim().slice(0, 1000), status });
  site.emit('booking.created', { id, service: svc.name, day, time: hhmm(time), status });
  jobs.enqueue('mailNew', { id });
  return { status: 303, headers: { Location: '/booking/b/' + code + '?new=1', 'Cache-Control': 'no-store' } };
});

function bookingPage(bk: Booking, notice = ''): RouteResult {
  const day = String(bk.day).slice(0, 10);
  const status = `<b>${esc(t(('s_' + bk.status) as any))}</b>`;
  const canCancel = (bk.status === 'pending' || bk.status === 'confirmed') && `${day} ${hhmm(bk.start_min)}` > `${localNow().day} ${hhmm(localNow().minutes)}`;
  return page(t('yourBooking'), (notice ? `<p class="bk-msg">${esc(notice)}</p>` : '') + `<h1>${esc(t('yourBooking'))}</h1>` +
    `<div class="bk-sum">📅 ${esc(dayLabel(day, true))} · ${hhmm(bk.start_min)}–${hhmm(bk.end_min)}<br>${esc(bk.service_name)}<br>${esc(t('status'))}: ${status}<br>` +
    `${esc(bk.name)} · ${esc(bk.phone)}</div>` +
    (canCancel ? `<form method="post" action="/booking/b/${esc(bk.code)}/cancel"><button class="bk-btn bk-btn-ghost" type="submit">${esc(t('cancel'))}</button></form>` : '') +
    `<p class="bk-empty" style="margin-top:16px">${esc(t('keepLink'))}</p>`);
}

site.route('GET /booking/b/{code}', { access: ['*'] }, ({ request }) => {
  const bk = byCode(request.params.code);
  if (!bk) return page(t('notFound'), `<p class="bk-empty">${esc(t('notFound'))}</p>`, 404);
  return bookingPage(bk, request.query.new ? (bk.status === 'pending' ? t('thanksPending') : t('thanks')) : '');
});

site.route('POST /booking/b/{code}/cancel', { access: ['*'] }, ({ request }) => {
  const bk = byCode(request.params.code);
  if (!bk) return page(t('notFound'), `<p class="bk-empty">${esc(t('notFound'))}</p>`, 404);
  if (bk.status === 'pending' || bk.status === 'confirmed') {
    db.table('bookings').where('id', bk.id).update({ status: 'cancelled' });
    site.emit('booking.status', { id: bk.id, from: bk.status, status: 'cancelled', by: 'customer' });
    jobs.enqueue('mailCancelled', { id: bk.id });
  }
  return bookingPage(getBooking(bk.id) as Booking, t('cancelled'));
});

// ---------------------------------------------------------------- mails

function siteInfo() {
  try {
    return site.info();
  } catch {
    return { title: '', url: '', adminEmail: '' } as any;
  }
}
const shopEmail = () => String(site.settings('notifyEmail') ?? '').trim() || siteInfo().adminEmail || '';
const summary = (bk: Booking) => `${dayLabel(String(bk.day).slice(0, 10), true)} ${hhmm(bk.start_min)}–${hhmm(bk.end_min)} · ${bk.service_name}`;
const link = (bk: Booking) => `${String(siteInfo().url || '').replace(/\/+$/, '')}/booking/b/${bk.code}`;

site.job('mailNew', (ctx) => {
  const bk = getBooking(Number(ctx.data?.id));
  if (!bk) return;
  if (bk.email) mail.send({ to: bk.email, subject: `${t(bk.status === 'pending' ? 'mailPending' : 'mailConfirmed')} — ${siteInfo().title}`,
    html: `<p>${esc(bk.name)}</p><p><b>${esc(summary(bk))}</b></p><p><a href="${esc(link(bk))}">${esc(t('mailView'))}</a></p>` });
  const to = shopEmail();
  if (to) mail.send({ to, subject: `📅 ${summary(bk)}`, html: `<p>${esc(bk.name)} · ${esc(bk.phone)}${bk.email ? ' · ' + esc(bk.email) : ''}</p><p>${esc(bk.note)}</p><p>${esc(t(('s_' + bk.status) as any))}</p>` });
});

site.job('mailStatus', (ctx) => {
  const bk = getBooking(Number(ctx.data?.id));
  if (!bk || !bk.email) return;
  mail.send({ to: bk.email, subject: `${t(('s_' + bk.status) as any)} — ${summary(bk)}`,
    html: `<p>${esc(bk.name)}</p><p><b>${esc(summary(bk))}</b>: ${esc(t(('s_' + bk.status) as any))}</p><p><a href="${esc(link(bk))}">${esc(t('mailView'))}</a></p>` });
});

site.job('mailCancelled', (ctx) => {
  const bk = getBooking(Number(ctx.data?.id));
  const to = shopEmail();
  if (bk && to) mail.send({ to, subject: `❌ ${summary(bk)}`, text: `${bk.name} ${bk.phone}: ${t('cancelled')}` });
});

// ---------------------------------------------------------------- designer block: a "book now" button

site.block('button', {
  label: { th: 'ปุ่มจองคิว', en: 'Booking button' },
  icon: '📅',
  props: [
    { key: 'service', type: 'select', label: { th: 'บริการ', en: 'Service' } },
    { key: 'label', type: 'text', maxLength: 40, label: { th: 'ข้อความบนปุ่ม', en: 'Button text' } },
  ],
  options: () => ({ service: [{ value: '', label: { th: '— ให้ลูกค้าเลือก —', en: '— let the customer choose —' } },
    ...services().map((s) => ({ value: String(s.id), label: s.name }))] }),
  render: (_ctx, props) => ({
    html: `<a class="bk-btn" href="/booking${Number(props.service) > 0 ? '?service=' + Number(props.service) : ''}">${esc(props.label || t('bookNow'))}</a>`,
    css: '.bk-btn{display:inline-flex;align-items:center;justify-content:center;min-height:46px;padding:0 22px;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font-weight:600;text-decoration:none}',
  }),
});

registerAdmin();
