// booking 1.1 — appointments and reservations: services with a length and price, weekly hours and closed days
// (admin page), and a booking page that works without JavaScript: /booking → service → day (party size, staff)
// → free time → details (the shop's own questions) → /booking/b/<code> (the customer's page, with a cancel
// button, and the deposit's PromptPay QR and slip upload when the service asks for one). No double booking
// beyond a service's capacity (seats for parties) or a person's diary; a reminder mail before the visit.
import {
  Booking, Service, answersOf, awaitingDeposit, bookableDays, byCode, esc, freeSlots, freeStaff, getBooking, getService, hhmm, isDay,
  localNow, partySizes, questions, services, staffFor, startsAt, weekday,
} from './model';
import { payload } from './lib/payload';
import { qrSvg } from './lib/qr';
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

/** The page's name: the owner's own ("Book a table", "Reserve") or "Book an appointment" (1.1). */
const title = () => String(site.settings('pageTitle') ?? '').trim().slice(0, 60) || t('title');
const bookLabel = () => String(site.settings('buttonLabel') ?? '').trim().slice(0, 40) || t('bookNow');

const CSS = `.bk{max-width:760px;margin:48px auto;padding:0 20px}.bk h1{font-size:28px;margin:0 0 6px}.bk .bk-sub{color:#6b7280;margin:0 0 20px}
.bk-steps{display:flex;gap:8px;flex-wrap:wrap;margin:0 0 18px;font-size:.9em;color:#6b7280}.bk-steps b{color:inherit}
.bk-grid{display:grid;gap:12px;grid-template-columns:repeat(auto-fill,minmax(200px,1fr))}
.bk-card{display:block;padding:16px;border:1px solid #e5e7eb;border-radius:14px;color:inherit;text-decoration:none;background:#fff}
.bk-card:hover{border-color:var(--c-primary,#4f46e5)}.bk-card small{display:block;color:#6b7280;margin-top:4px}
.bk-days{display:grid;gap:8px;grid-template-columns:repeat(auto-fill,minmax(76px,1fr))}.bk-day{text-align:center;padding:10px 6px;border:1px solid #e5e7eb;border-radius:12px;color:inherit;text-decoration:none;background:#fff;font-size:.92em}
.bk-day[aria-disabled=true]{opacity:.4;pointer-events:none}.bk-day[aria-current=true]{border-color:var(--c-primary,#4f46e5);background:var(--c-primary,#4f46e5);color:#fff}
.bk-times{display:grid;gap:8px;grid-template-columns:repeat(auto-fill,minmax(86px,1fr));margin-top:16px}
.bk-time{padding:10px;border:1px solid #d1d5db;border-radius:10px;text-align:center;color:inherit;text-decoration:none;font-weight:600}
.bk-time:hover{border-color:var(--c-primary,#4f46e5);color:var(--c-primary,#4f46e5)}
.bk-opts{display:flex;flex-wrap:wrap;gap:12px;align-items:end;margin:0 0 16px;padding:12px 14px;border-radius:12px;background:#f9fafb}
.bk-opts label{display:flex;flex-direction:column;gap:4px;font-weight:600;font-size:.9em}.bk-opts select{font:inherit;min-height:40px;padding:0 10px;border:1px solid #d1d5db;border-radius:10px;background:#fff}
.bk-opts button{min-height:40px;padding:0 16px;border:1px solid #d1d5db;border-radius:10px;background:#fff;font:inherit;cursor:pointer}
.bk-form{display:flex;flex-direction:column;gap:12px;max-width:460px}.bk-form label{display:flex;flex-direction:column;gap:6px;font-weight:600;font-size:.92em}
.bk-form input,.bk-form textarea{font:inherit;font-weight:400;padding:10px 12px;border:1px solid #d1d5db;border-radius:10px}
.bk-btn{display:inline-flex;align-items:center;justify-content:center;min-height:46px;padding:0 22px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer;text-decoration:none}
.bk-btn-ghost{background:#fff;color:#dc2626;border:1px solid #fecaca}
.bk-sum{padding:16px;border-radius:14px;background:#f9fafb;margin:0 0 16px;line-height:1.7}
.bk-pay{display:flex;flex-direction:column;align-items:center;text-align:center;gap:8px;padding:20px;margin:0 0 16px;border:1px solid #e5e7eb;border-radius:14px}
.bk-qr{width:220px;height:220px;image-rendering:pixelated}.bk-bank{white-space:pre-wrap;font:inherit;text-align:left;background:#f9fafb;padding:12px 16px;border-radius:8px}
.bk-slip{display:flex;flex-direction:column;align-items:center;gap:10px}
.bk-msg{padding:12px 14px;border-radius:10px;background:#dcfce7;color:#166534}.bk-bad{background:#fee2e2;color:#991b1b}.bk-empty{color:#6b7280}`;

let scriptURL = '';
try {
  scriptURL = site.asset('assets/booking.js').url;
} catch {
  /* none */
}

const page = (name: string, html: string, status = 200): RouteResult => ({
  page: { title: name, html: `<style>${CSS}</style><div class="bk">${html}</div>${scriptURL ? `<script defer src="${esc(scriptURL)}"></script>` : ''}` },
  status, headers: { 'Cache-Control': 'no-store' },
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
    return page(title(), `<h1>${esc(title())}</h1>${steps(0)}` + (list.length
      ? `<div class="bk-grid">${list.map((s) => `<a class="bk-card" href="/booking?service=${s.id}"><b>${esc(s.name)}</b><small>${s.minutes} ${esc(t('min'))}${price(s.price) ? ' · ' + price(s.price) : ''}</small>${s.description ? `<small>${esc(s.description)}</small>` : ''}</a>`).join('')}</div>`
      : `<p class="bk-empty">${esc(t('noServices'))}</p>`));
  }
  // party size and person (1.1): chosen first, they decide the free times
  const sizes = partySizes(svc);
  const party = Math.min(sizes.length, Math.max(1, Math.floor(Number(q1(request.query.party))) || 1));
  const people = staffFor(svc);
  const staffId = people.some((p) => p.id === Number(q1(request.query.staff))) ? Number(q1(request.query.staff)) : 0;
  const keep = `service=${svc.id}${sizes.length > 1 ? '&party=' + party : ''}${staffId ? '&staff=' + staffId : ''}`;
  const opts = sizes.length > 1 || people.length
    ? `<form class="bk-opts" method="get" action="/booking"><input type="hidden" name="service" value="${svc.id}">` +
      (q1(request.query.date) ? `<input type="hidden" name="date" value="${esc(q1(request.query.date))}">` : '') +
      (sizes.length > 1 ? `<label>${esc(t('party'))}<select name="party" data-bk-auto>${sizes.map((n) => `<option value="${n}"${n === party ? ' selected' : ''}>${n}</option>`).join('')}</select></label>` : '') +
      (people.length ? `<label>${esc(t('staff'))}<select name="staff" data-bk-auto><option value="0">${esc(t('anyStaff'))}</option>` +
        people.map((p) => `<option value="${p.id}"${p.id === staffId ? ' selected' : ''}>${esc(p.name)}</option>`).join('') + '</select></label>' : '') +
      `<button type="submit">${esc(t('apply'))}</button></form>`
    : '';
  const days = bookableDays();
  const day = q1(request.query.date);
  const chosen = isDay(day) && days.some((d) => d.day === day && d.open) ? day : '';
  const dayChips = `<div class="bk-days" role="list">${days.map((d) => `<a role="listitem" class="bk-day" href="/booking?${keep}&date=${d.day}"` +
    `${d.open ? '' : ' aria-disabled="true"'}${d.day === chosen ? ' aria-current="true"' : ''}>${esc(dayLabel(d.day))}</a>`).join('')}</div>`;
  const head = `<h1>${esc(title())}</h1><p class="bk-sub">${svcLine(svc)} · <a href="/booking">${esc(t('change'))}</a></p>`;
  if (!chosen) return page(title(), head + steps(1) + opts + dayChips);
  const time = Number(q1(request.query.time));
  const slots = freeSlots(svc, chosen, party, staffId);
  if (!q1(request.query.time) || !slots.includes(time)) {
    return page(title(), head + steps(2) + opts + dayChips + (slots.length
      ? `<div class="bk-times">${slots.map((s) => `<a class="bk-time" href="/booking?${keep}&date=${chosen}&time=${s}">${hhmm(s)}</a>`).join('')}</div>`
      : `<p class="bk-empty" style="margin-top:16px">${esc(t('full'))}</p>`));
  }
  const who = staffId ? people.find((p) => p.id === staffId)?.name ?? '' : '';
  const qs = questions();
  return page(title(), head + steps(3) +
    `<div class="bk-sum">📅 ${esc(dayLabel(chosen, true))} · ${hhmm(time)}–${hhmm(time + Number(svc.minutes))}<br>${svcLine(svc)}` +
    (sizes.length > 1 ? `<br>👥 ${esc(t('people', { n: party }))}` : '') + (who ? `<br>👤 ${esc(who)}` : '') +
    (Number(svc.deposit) > 0 ? `<br>💳 ${esc(t('depositNote', { amount: price(svc.deposit) }))}` : '') + '</div>' +
    `<form class="bk-form" method="post" action="/booking"><input type="hidden" name="service" value="${svc.id}"><input type="hidden" name="date" value="${chosen}"><input type="hidden" name="time" value="${time}">` +
    `<input type="hidden" name="party" value="${party}"><input type="hidden" name="staff" value="${staffId}">` +
    `<label>${esc(t('name'))}<input name="name" required maxlength="120" autocomplete="name"></label>` +
    `<label>${esc(t('phone'))}<input type="tel" name="phone" required maxlength="40" autocomplete="tel"></label>` +
    `<label>${esc(t('email'))}<input type="email" name="email" maxlength="191" autocomplete="email"></label>` +
    qs.map((q) => `<label>${esc(q.label)}${q.required ? ' *' : ''}<input name="${q.key}" maxlength="500"${q.required ? ' required' : ''}></label>`).join('') +
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
  if (String(b.website ?? '').trim()) return page(title(), `<p class="bk-msg">${esc(t('thanks'))}</p>`);
  const svc = getService(Number(b.service) || 0);
  const day = String(b.date ?? '');
  const time = Number(b.time);
  const back = svc ? `/booking?service=${svc.id}&date=${encodeURIComponent(day)}` : '/booking';
  const name = String(b.name ?? '').trim(), phone = String(b.phone ?? '').trim(), email = String(b.email ?? '').trim().toLowerCase();
  const bad = (msg: string, status = 422) => page(title(), `<p class="bk-msg bk-bad">${esc(msg)}</p><p><a href="${esc(back)}">← ${esc(t('back'))}</a></p>`, status);
  if (!svc || !svc.active) return bad(t('noService'), 404);
  if (!name || name.length > 120 || !/^[0-9+\-\s()]{9,40}$/.test(phone) || (email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email))) return bad(t('check'));
  const sizes = partySizes(svc);
  const party = Math.floor(Number(b.party) || 1);
  if (party < 1 || party > sizes.length) return bad(t('check'));
  const answers = questions().map((q) => ({ label: q.label, value: String(b[q.key] ?? '').trim().slice(0, 500), required: q.required }));
  if (answers.some((a) => a.required && !a.value)) return bad(t('answerAll'));
  const key = 'bk:rate:' + crypto.hash('sha256', request.ip || '-');
  const n = Number(cache.get(key) ?? 0);
  if (n >= 5) return bad(t('busy'), 429);
  cache.set(key, n + 1, 600);
  // the slot (and a person for it) is checked again right before it is booked (someone may have taken it meanwhile)
  const staffId = Math.max(0, Math.floor(Number(b.staff) || 0));
  if (!freeSlots(svc, day, party, staffId).includes(time)) return bad(t('taken'), 409);
  const end = time + Number(svc.minutes);
  const person = staffFor(svc).length ? freeStaff(svc, day, time, end, staffId) : null;
  if (staffFor(svc).length && !person) return bad(t('taken'), 409);
  const deposit = Math.max(0, Number(svc.deposit) || 0);
  const status = deposit > 0 || site.settings('autoConfirm') === false ? 'pending' : 'confirmed';
  const code = crypto.random(12);
  const id = db.table('bookings').insert({ code, service_id: svc.id, service_name: svc.name, day, start_min: time, end_min: end,
    name, phone, email, note: String(b.note ?? '').trim().slice(0, 1000), status, party,
    staff_id: person?.id ?? 0, staff_name: person?.name ?? '', deposit,
    answers: JSON.stringify(answers.filter((a) => a.value).map(({ label, value }) => ({ label, value }))) });
  site.emit('booking.created', { id, service: svc.name, day, time: hhmm(time), status, party, staff: person?.name ?? '' });
  jobs.enqueue('mailNew', { id });
  return { status: 303, headers: { Location: '/booking/b/' + code + '?new=1', 'Cache-Control': 'no-store' } };
});

function depositBox(bk: Booking): string {
  if (!awaitingDeposit(bk) && !(bk.status === 'pending' && Number(bk.deposit) > 0)) return '';
  if (bk.slip) return `<p class="bk-msg" id="bk-slip-done">${esc(t('slipDone'))}</p>`;
  const amount = price(bk.deposit);
  const pp = String(site.settings('promptpayId') ?? '').trim();
  const bank = String(site.settings('bankInfo') ?? '').trim();
  const nm = String(site.settings('promptpayName') ?? '').trim();
  return `<div class="bk-pay"><b>${esc(t('payDeposit', { amount }))}</b>` +
    (pp ? `<img class="bk-qr" src="/booking/b/${esc(bk.code)}/qr.svg" width="220" height="220" alt="PromptPay QR">${nm ? `<p class="bk-empty">${esc(t('account', { name: nm }))}</p>` : ''}` : '') +
    (bank ? `<pre class="bk-bank">${esc(bank)}</pre>` : '') +
    `<form class="bk-slip" data-bk-slip="${esc(bk.code)}" data-bad="${esc(t('slipBad'))}" data-done="${esc(t('slipDone'))}">` +
    `<label>${esc(t('slip'))} <input type="file" accept="image/jpeg,image/png" required></label>` +
    `<button type="submit" class="bk-btn">${esc(t('slipSend'))}</button><p class="bk-empty" role="status"></p></form></div>`;
}

function bookingPage(bk: Booking, notice = ''): RouteResult {
  const day = String(bk.day).slice(0, 10);
  const label = awaitingDeposit(bk) ? t('s_deposit') : t(('s_' + bk.status) as any);
  const canCancel = (bk.status === 'pending' || bk.status === 'confirmed') && `${day} ${hhmm(bk.start_min)}` > `${localNow().day} ${hhmm(localNow().minutes)}`;
  const answers = answersOf(bk).map((a) => `<br>${esc(a.label)}: ${esc(a.value)}`).join('');
  return page(t('yourBooking'), (notice ? `<p class="bk-msg">${esc(notice)}</p>` : '') + `<h1>${esc(t('yourBooking'))}</h1>` +
    `<div class="bk-sum">📅 ${esc(dayLabel(day, true))} · ${hhmm(bk.start_min)}–${hhmm(bk.end_min)}<br>${esc(bk.service_name)}` +
    (Number(bk.party) > 1 ? `<br>👥 ${esc(t('people', { n: bk.party }))}` : '') + (bk.staff_name ? `<br>👤 ${esc(bk.staff_name)}` : '') +
    `<br>${esc(t('status'))}: <b>${esc(label)}</b><br>${esc(bk.name)} · ${esc(bk.phone)}${answers}</div>` + depositBox(bk) +
    (canCancel ? `<form method="post" action="/booking/b/${esc(bk.code)}/cancel"><button class="bk-btn bk-btn-ghost" type="submit">${esc(t('cancel'))}</button></form>` : '') +
    `<p class="bk-empty" style="margin-top:16px">${esc(t('keepLink'))}</p>`);
}

site.route('GET /booking/b/{code}', { access: ['*'] }, ({ request }) => {
  const bk = byCode(request.params.code);
  if (!bk) return page(t('notFound'), `<p class="bk-empty">${esc(t('notFound'))}</p>`, 404);
  const fresh = request.query.new ? (awaitingDeposit(bk) ? t('thanksDeposit') : bk.status === 'pending' ? t('thanksPending') : t('thanks')) : '';
  return bookingPage(bk, fresh);
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

// ---------------------------------------------------------------- the deposit (1.1): PromptPay QR and the slip

site.route('GET /booking/b/{code}/qr.svg', { access: ['*'] }, ({ request }) => {
  const bk = byCode(request.params.code);
  const id = String(site.settings('promptpayId') ?? '').trim();
  if (!bk || !(Number(bk.deposit) > 0) || !id) return { status: 404, headers: { 'Content-Type': 'text/plain' }, body: 'not found' };
  let p: string;
  try {
    p = payload({ id, amount: Number(bk.deposit).toFixed(2) });
  } catch (e) {
    return { status: 500, headers: { 'Content-Type': 'text/plain' }, body: String((e as Error).message) };
  }
  return { status: 200, headers: { 'Content-Type': 'image/svg+xml', 'Cache-Control': 'private, max-age=300' }, body: qrSvg(p, { size: 220 }) };
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

// The slip: a JPEG/PNG data URL (assets/booking.js shrinks the photo first), kept as a private file.
site.route('POST /booking/b/{code}/slip', { access: ['*'] }, ({ request }) => {
  const bk = byCode(request.params.code);
  if (!bk || bk.status !== 'pending' || !(Number(bk.deposit) > 0)) return { status: 404, body: { error: t('notFound') } };
  if (bk.slip) return { status: 409, body: { error: t('slipDone') } };
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
  const path = `slips/${bk.code}.${m[1] === 'png' ? 'png' : 'jpg'}`;
  files.put(path, bytes);
  db.table('bookings').where('id', bk.id).update({ slip: path, slip_at: new Date() });
  jobs.enqueue('mailSlip', { id: bk.id });
  return { status: 200, body: { ok: true, message: t('slipDone') } };
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
const summary = (bk: Booking) => `${dayLabel(String(bk.day).slice(0, 10), true)} ${hhmm(bk.start_min)}–${hhmm(bk.end_min)} · ${bk.service_name}` +
  (Number(bk.party) > 1 ? ` · ${t('people', { n: bk.party })}` : '') + (bk.staff_name ? ` · ${bk.staff_name}` : '');
const link = (bk: Booking) => `${String(siteInfo().url || '').replace(/\/+$/, '')}/booking/b/${bk.code}`;
const answersHTML = (bk: Booking) => answersOf(bk).map((a) => `<br>${esc(a.label)}: ${esc(a.value)}`).join('');

site.job('mailNew', (ctx) => {
  const bk = getBooking(Number(ctx.data?.id));
  if (!bk) return;
  const head = awaitingDeposit(bk) ? t('mailDeposit', { amount: price(bk.deposit) }) : t(bk.status === 'pending' ? 'mailPending' : 'mailConfirmed');
  if (bk.email) mail.send({ to: bk.email, subject: `${head} — ${siteInfo().title}`,
    html: `<p>${esc(bk.name)}</p><p><b>${esc(summary(bk))}</b></p><p><a href="${esc(link(bk))}">${esc(t('mailView'))}</a></p>` });
  const to = shopEmail();
  if (to) mail.send({ to, subject: `📅 ${summary(bk)}`, html: `<p>${esc(bk.name)} · ${esc(bk.phone)}${bk.email ? ' · ' + esc(bk.email) : ''}${answersHTML(bk)}</p><p>${esc(bk.note)}</p><p>${esc(awaitingDeposit(bk) ? t('s_deposit') : t(('s_' + bk.status) as any))}</p>` });
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

site.job('mailSlip', (ctx) => {
  const bk = getBooking(Number(ctx.data?.id));
  const to = shopEmail();
  if (bk && to) mail.send({ to, subject: `💳 ${summary(bk)}`, text: `${bk.name} ${bk.phone}: ${t('slipDone')} (${price(bk.deposit)})` });
});

site.job('mailReminder', (ctx) => {
  const bk = getBooking(Number(ctx.data?.id));
  if (!bk || !bk.email || bk.status !== 'confirmed') return;
  mail.send({ to: bk.email, subject: `⏰ ${t('mailReminder')} — ${summary(bk)}`,
    html: `<p>${esc(bk.name)}</p><p>${esc(t('reminderText'))}</p><p><b>${esc(summary(bk))}</b></p><p><a href="${esc(link(bk))}">${esc(t('mailView'))}</a></p>` });
});

// Every 15 minutes: reminders for confirmed bookings starting within remindHours (once each), and bookings
// whose deposit never came are cancelled after depositHours (the time is free again).
site.cron('*/15 * * * *', () => {
  const now = Date.now();
  const remind = Math.max(0, Number(site.settings('remindHours') ?? 24) || 0);
  if (remind > 0) {
    const soon = db.table('bookings').where('status', 'confirmed').where('email', '!=', '').whereNull('reminded_at')
      .where('day', '>=', localNow().day).where('day', '<=', new Date(now + (remind + 24) * 3600_000).toISOString().slice(0, 10)).limit(200).get() as Booking[];
    for (const bk of soon) {
      const at = startsAt(bk).getTime();
      if (at > now && at - now <= remind * 3600_000) {
        db.table('bookings').where('id', bk.id).update({ reminded_at: new Date() });
        jobs.enqueue('mailReminder', { id: bk.id });
      }
    }
  }
  const hours = Math.max(0, Number(site.settings('depositHours') ?? 24) || 0);
  if (hours > 0) {
    const stale = db.table('bookings').where('status', 'pending').where('deposit', '>', 0).where('slip', '')
      .where('created_at', '<', new Date(now - hours * 3600_000)).limit(200).get() as Booking[];
    for (const bk of stale) {
      db.table('bookings').where('id', bk.id).update({ status: 'cancelled' });
      site.emit('booking.status', { id: bk.id, from: 'pending', status: 'cancelled', by: 'system' });
      jobs.enqueue('mailStatus', { id: bk.id });
    }
  }
}, { name: 'remindAndExpire' });

// ---------------------------------------------------------------- designer block: a "book now" button

site.block('button', {
  label: { th: 'ปุ่มจอง', en: 'Booking button' },
  icon: '📅',
  props: [
    { key: 'service', type: 'select', label: { th: 'บริการ', en: 'Service' } },
    { key: 'label', type: 'text', maxLength: 40, label: { th: 'ข้อความบนปุ่ม', en: 'Button text' } },
  ],
  options: () => ({ service: [{ value: '', label: { th: '— ให้ลูกค้าเลือก —', en: '— let the customer choose —' } },
    ...services().map((s) => ({ value: String(s.id), label: s.name }))] }),
  render: (_ctx, props) => ({
    html: `<a class="bk-btn" href="/booking${Number(props.service) > 0 ? '?service=' + Number(props.service) : ''}">${esc(props.label || bookLabel())}</a>`,
    css: '.bk-btn{display:inline-flex;align-items:center;justify-content:center;min-height:46px;padding:0 22px;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font-weight:600;text-decoration:none}',
  }),
});

registerAdmin();
