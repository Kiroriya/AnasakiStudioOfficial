// events 1.0 — events on one of the site's content types (set in the settings; the starter kits' "event" type
// by default): a sign-up form under each event's description with a seat limit, the visitor's registration
// page (cancel, add to calendar), a month calendar at /calendar, .ics files and Google Calendar links, mails,
// and the "Registrations" admin page (attendees per event, check-in, cancel, CSV). No JavaScript needed.

const CAPS = typeof (site as any).can === 'function';
const ACCESS = CAPS ? ['cap:events.manage'] : ['editor'];

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);
const set = (k: string, d = '') => String(site.settings(k) ?? '').trim() || d;
const TYPE = () => set('postType', 'event');
const F = () => ({ date: set('dateField', 'when'), end: set('endField'), place: set('placeField', 'place'), seats: set('seatsField', 'seats') });

function lang(): 0 | 1 {
  try {
    return String(site.info().language || 'th').startsWith('th') ? 0 : 1;
  } catch {
    return 0;
  }
}
const T = {
  register: ['ลงทะเบียนเข้าร่วม', 'Sign up'], name: ['ชื่อ', 'Name'], email: ['อีเมล (รับการยืนยัน)', 'Email (for the confirmation)'], phone: ['เบอร์โทร', 'Phone'],
  seats: ['จำนวนที่นั่ง', 'Seats'], note: ['หมายเหตุ', 'Note'], send: ['ลงทะเบียน', 'Sign up'], left: ['เหลือ {n} ที่', '{n} seats left'],
  full: ['ที่นั่งเต็มแล้ว', 'This event is full'], past: ['กิจกรรมนี้ผ่านไปแล้ว', 'This event has passed'],
  addCal: ['เพิ่มลงปฏิทิน', 'Add to calendar'], google: ['Google Calendar', 'Google Calendar'], ics: ['ไฟล์ .ics (Apple, Outlook)', '.ics file (Apple, Outlook)'],
  done: ['ลงทะเบียนแล้ว! แล้วพบกัน', 'You are signed up — see you there!'], yours: ['การลงทะเบียนของคุณ', 'Your registration'],
  cancel: ['ยกเลิกการลงทะเบียน', 'Cancel my registration'], cancelled: ['ยกเลิกแล้ว', 'Cancelled'], confirmed: ['ยืนยันแล้ว', 'Confirmed'],
  keepLink: ['เก็บลิงก์หน้านี้ไว้ดูหรือยกเลิก', 'Keep this link to see or cancel it'], notFound: ['ไม่พบรายการ', 'Not found'],
  check: ['กรุณากรอกชื่อ และอีเมลหรือเบอร์โทรให้ถูกต้อง', 'Please enter your name and a valid email or phone'],
  busy: ['ส่งถี่เกินไป รอสักครู่', 'Too many tries — wait a moment'], back: ['กลับไปหน้ากิจกรรม', 'Back to the event'],
  calendar: ['ปฏิทินกิจกรรม', 'Events calendar'], prev: ['‹ เดือนก่อน', '‹ Previous'], next: ['เดือนถัดไป ›', 'Next ›'], none: ['ไม่มีกิจกรรมในเดือนนี้', 'No events this month'],
  mailSubject: ['ลงทะเบียนแล้ว: {title}', 'You are signed up: {title}'], where: ['สถานที่', 'Where'], when: ['วันเวลา', 'When'],
} as const;
type K = keyof typeof T;
const t = (k: K, p?: Record<string, string | number>) => {
  let s: string = T[k][lang()];
  for (const [a, b] of Object.entries(p ?? {})) s = s.split(`{${a}}`).join(String(b));
  return s;
};
const DAYS = [['อา', 'จ', 'อ', 'พ', 'พฤ', 'ศ', 'ส'], ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']];
const MONTHS = [['มกราคม', 'กุมภาพันธ์', 'มีนาคม', 'เมษายน', 'พฤษภาคม', 'มิถุนายน', 'กรกฎาคม', 'สิงหาคม', 'กันยายน', 'ตุลาคม', 'พฤศจิกายน', 'ธันวาคม'],
  ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']];

interface Ev { id: number; title: string; url: string; start: string; end: string; place: string; seats: number }
interface Reg { id: number; code: string; post_id: number; title: string; starts: string; name: string; email: string; phone: string; seats: number; note: string; status: string; checked_in_at: string | null; created_at: string }

function metaOf(p: any): Record<string, any> {
  let m = p?.meta;
  if (typeof m === 'string') {
    try {
      m = JSON.parse(m);
    } catch {
      m = {};
    }
  }
  return m && typeof m === 'object' ? m : {};
}

/** "2027-01-16T08:00" (date or date-time fields; "" when missing). */
const when = (v: unknown) => {
  const s = String(v ?? '').trim();
  return /^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2})?/.test(s) ? s.slice(0, 16) : '';
};

function toEvent(p: any): Ev {
  const m = metaOf(p), f = F();
  return { id: Number(p.id), title: String(p.title ?? ''), url: String(p.url ?? ''), start: when(m[f.date]), end: f.end ? when(m[f.end]) : '',
    place: String(m[f.place] ?? '').slice(0, 300), seats: Math.max(0, Math.floor(Number(m[f.seats]) || 0)) };
}

function getEvent(id: number): Ev | null {
  const p = db.posts.get(id, { type: TYPE() });
  return p && p.type === TYPE() && p.status === 'publish' ? toEvent(p) : null;
}

const taken = (postId: number) => Number(db.table('registrations').where('post_id', postId).where('status', 'confirmed').sum('seats') ?? 0);

function nowLocal(): string {
  let off = 420;
  try {
    off = site.info().timezone === 'UTC' ? 0 : 420;
  } catch {
    /* Bangkok */
  }
  return new Date(Date.now() + off * 60_000).toISOString().slice(0, 16);
}

function label(start: string): string {
  if (!start) return '';
  const [d, tm] = start.split('T');
  const [y, m, day] = d.split('-').map(Number);
  const l = lang();
  return `${day} ${MONTHS[l][m - 1]} ${l === 0 ? y + 543 : y}${tm ? ' ' + tm : ''}`;
}

/** Google Calendar link of an event (floating local times, two hours when no end). */
function googleLink(ev: Ev): string {
  const fmt = (s: string) => s.replace(/[-:]/g, '') + (s.length > 10 ? '00' : '');
  const end = ev.end || (ev.start.length > 10 ? new Date(Date.parse(ev.start + ':00Z') + 2 * 3600_000).toISOString().slice(0, 16) : ev.start);
  const dates = ev.start.length > 10 ? `${fmt(ev.start)}/${fmt(end)}` : `${fmt(ev.start)}/${fmt(end)}`;
  return 'https://calendar.google.com/calendar/render?action=TEMPLATE&text=' + encodeURIComponent(ev.title) + '&dates=' + dates +
    (ev.place ? '&location=' + encodeURIComponent(ev.place) : '');
}

const calLinks = (ev: Ev) => !ev.start ? '' :
  `<p class="ev-cal">📅 ${esc(t('addCal'))}: <a href="${esc(googleLink(ev))}" target="_blank" rel="noopener">${esc(t('google'))}</a> · ` +
  `<a href="/calendar/${ev.id}.ics">${esc(t('ics'))}</a></p>`;

const CSS = `.ev{max-width:860px;margin:48px auto;padding:0 20px}.ev h1{font-size:28px;margin:0 0 16px}
.ev-grid{display:grid;grid-template-columns:repeat(7,minmax(0,1fr));gap:4px}.ev-grid b{font-size:12px;color:#6b7280;text-align:center;padding:4px}
.ev-day{min-height:92px;border:1px solid #eef0f3;border-radius:8px;padding:4px;font-size:12px;overflow:hidden}.ev-day.ev-out{background:#f9fafb;color:#9ca3af}
.ev-day.ev-today{border-color:var(--c-primary,#4f46e5)}.ev-day a{display:block;margin-top:3px;padding:2px 4px;border-radius:4px;background:#eef0ff;color:#3730a3;text-decoration:none;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.ev-nav{display:flex;justify-content:space-between;align-items:center;margin:0 0 12px}.ev-list{display:none}
@media (max-width:640px){.ev-grid{display:none}.ev-list{display:block}}
.ev-list li{margin:8px 0}.ev-msg{padding:12px 14px;border-radius:10px;background:#dcfce7;color:#166534}.ev-bad{background:#fee2e2;color:#991b1b}
.ev-sum{padding:16px;border-radius:14px;background:#f9fafb;line-height:1.7;margin:0 0 16px}.ev-muted{color:#6b7280}
.ev-btn{display:inline-flex;align-items:center;justify-content:center;min-height:44px;padding:0 20px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer}
.ev-ghost{background:#fff;color:#dc2626;border:1px solid #fecaca}`;

const page = (title: string, html: string, status = 200): RouteResult => ({
  page: { title, html: `<style>${CSS}</style><div class="ev">${html}</div>` }, status, headers: { 'Cache-Control': 'no-store' },
});

// ---------------------------------------------------------------- the sign-up form under an event

// inline styles: the post.content filter has 50 ms and reads no package files
function rsvpBox(ev: Ev): string {
  const box = 'margin:28px 0;padding:20px;border:1px solid #e5e7eb;border-radius:14px';
  const inp = 'font:inherit;padding:10px 12px;border:1px solid #d1d5db;border-radius:10px;width:100%;box-sizing:border-box';
  const info = `<p style="margin:0 0 12px;line-height:1.7">${ev.start ? `🗓 ${esc(label(ev.start))}` : ''}${ev.place ? `<br>📍 ${esc(ev.place)}` : ''}</p>`;
  const cal = calLinks(ev);
  if (ev.start && ev.start < nowLocal().slice(0, ev.start.length)) return `<div class="ev-rsvp" style="${box}">${info}<p style="color:#6b7280">${esc(t('past'))}</p></div>`;
  const left = ev.seats > 0 ? ev.seats - taken(ev.id) : Infinity;
  if (left <= 0) return `<div class="ev-rsvp" style="${box}">${info}<p style="color:#991b1b;font-weight:600">${esc(t('full'))}</p>${cal}</div>`;
  const most = Math.max(1, Math.min(Number(site.settings('maxPerPerson') ?? 4) || 4, Number.isFinite(left) ? left : 50));
  return `<div class="ev-rsvp" style="${box}"><h2 style="font-size:20px;margin:0 0 8px">${esc(t('register'))}</h2>${info}` +
    (Number.isFinite(left) ? `<p style="color:#6b7280;margin:0 0 12px">${esc(t('left', { n: left }))}</p>` : '') +
    `<form method="post" action="/calendar/rsvp/${ev.id}" style="display:grid;gap:10px;max-width:460px">` +
    `<label style="display:grid;gap:4px;font-weight:600">${esc(t('name'))}<input name="name" required maxlength="191" autocomplete="name" style="${inp}"></label>` +
    `<label style="display:grid;gap:4px;font-weight:600">${esc(t('email'))}<input type="email" name="email" maxlength="191" autocomplete="email" style="${inp}"></label>` +
    `<label style="display:grid;gap:4px;font-weight:600">${esc(t('phone'))}<input type="tel" name="phone" maxlength="40" autocomplete="tel" style="${inp}"></label>` +
    (most > 1 ? `<label style="display:grid;gap:4px;font-weight:600">${esc(t('seats'))}<select name="seats" style="${inp}">` +
      Array.from({ length: most }, (_, i) => `<option value="${i + 1}">${i + 1}</option>`).join('') + '</select></label>' : '<input type="hidden" name="seats" value="1">') +
    `<label style="display:grid;gap:4px;font-weight:600">${esc(t('note'))}<input name="note" maxlength="500" style="${inp}"></label>` +
    '<div style="position:absolute;left:-9999px" aria-hidden="true"><input name="website" tabindex="-1" autocomplete="off"></div>' +
    `<button type="submit" style="min-height:46px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer">${esc(t('send'))}</button>` +
    `</form>${cal}</div>`;
}

site.filter('post.content', (html: string, ctx: any) => {
  const post = ctx?.post;
  if (!post || post.type !== TYPE() || site.settings('rsvp') === false) return html;
  return html + rsvpBox(toEvent(post));
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

site.route('POST /calendar/rsvp/{id}', { access: ['*'] }, ({ request }) => {
  const ev = getEvent(Number(request.params.id) || 0);
  if (!ev) return page(t('notFound'), `<p>${esc(t('notFound'))}</p>`, 404);
  const b = formOf(request);
  const bad = (msg: string, status = 422) => page(ev.title, `<p class="ev-msg ev-bad">${esc(msg)}</p><p><a href="${esc(ev.url)}">← ${esc(t('back'))}</a></p>`, status);
  if (String(b.website ?? '').trim()) return page(ev.title, `<p class="ev-msg">${esc(t('done'))}</p>`);
  const name = String(b.name ?? '').trim().slice(0, 191), email = String(b.email ?? '').trim().toLowerCase(), phone = String(b.phone ?? '').trim();
  const okMail = !email || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email), okPhone = !phone || /^[0-9+\-\s()]{9,40}$/.test(phone);
  if (!name || !okMail || !okPhone || (!email && !phone)) return bad(t('check'));
  if (ev.start && ev.start < nowLocal().slice(0, ev.start.length)) return bad(t('past'), 409);
  const seats = Math.max(1, Math.min(50, Number(site.settings('maxPerPerson') ?? 4) || 4, Math.floor(Number(b.seats) || 1)));
  const key = 'ev:rate:' + crypto.hash('sha256', request.ip || '-');
  const n = Number(cache.get(key) ?? 0);
  if (n >= 5) return bad(t('busy'), 429);
  cache.set(key, n + 1, 600);
  // the seats are counted before and again after the row is in: a sign-up that went over the limit (two at
  // the same moment for the last seats) takes itself back out
  if (ev.seats > 0 && taken(ev.id) + seats > ev.seats) return bad(t('full'), 409);
  const code = crypto.random(12);
  const id = db.table('registrations').insert({ code, post_id: ev.id, title: ev.title.slice(0, 191), starts: ev.start, name, email, phone, seats,
    note: String(b.note ?? '').trim().slice(0, 500) });
  if (ev.seats > 0 && taken(ev.id) > ev.seats) {
    db.table('registrations').where('id', id).delete();
    return bad(t('full'), 409);
  }
  site.emit('events.registered', { id, eventId: ev.id, title: ev.title, seats });
  jobs.enqueue('mailNew', { id });
  return { status: 303, headers: { Location: '/calendar/r/' + code + '?new=1', 'Cache-Control': 'no-store' } };
});

const byCode = (c: string) => (/^[0-9a-f]{24}$/.test(c) ? (db.table('registrations').where('code', c).first() as Reg | null) : null);

function regPage(r: Reg, notice = ''): RouteResult {
  const ev = getEvent(r.post_id);
  return page(t('yours'), (notice ? `<p class="ev-msg">${esc(notice)}</p>` : '') + `<h1>${esc(t('yours'))}</h1>` +
    `<div class="ev-sum"><b>${ev ? `<a href="${esc(ev.url)}">${esc(r.title)}</a>` : esc(r.title)}</b><br>🗓 ${esc(label(r.starts))}${ev?.place ? `<br>📍 ${esc(ev.place)}` : ''}` +
    `<br>${esc(r.name)} · ${esc(t('seats'))} ${r.seats}<br><b>${esc(t(r.status === 'cancelled' ? 'cancelled' : 'confirmed'))}</b></div>` +
    (ev && r.status === 'confirmed' ? calLinks(ev) : '') +
    (r.status === 'confirmed' ? `<form method="post" action="/calendar/r/${esc(r.code)}/cancel"><button class="ev-btn ev-ghost">${esc(t('cancel'))}</button></form>` : '') +
    `<p class="ev-muted">${esc(t('keepLink'))}</p>`);
}

site.route('GET /calendar/r/{code}', { access: ['*'] }, ({ request }) => {
  const r = byCode(request.params.code);
  return r ? regPage(r, request.query.new ? t('done') : '') : page(t('notFound'), `<p>${esc(t('notFound'))}</p>`, 404);
});

site.route('POST /calendar/r/{code}/cancel', { access: ['*'] }, ({ request }) => {
  const r = byCode(request.params.code);
  if (!r) return page(t('notFound'), `<p>${esc(t('notFound'))}</p>`, 404);
  if (r.status === 'confirmed') {
    db.table('registrations').where('id', r.id).update({ status: 'cancelled' });
    site.emit('events.cancelled', { id: r.id, eventId: r.post_id, by: 'attendee' });
  }
  return regPage(byCode(r.code) as Reg, t('cancelled'));
});

// ---------------------------------------------------------------- calendar and .ics

site.route('GET /calendar', { access: ['*'] }, ({ request }) => {
  const now = nowLocal();
  const q = String(Array.isArray(request.query.month) ? request.query.month[0] : request.query.month ?? '');
  const month = /^\d{4}-(0[1-9]|1[0-2])$/.test(q) ? q : now.slice(0, 7);
  const [y, m] = month.split('-').map(Number);
  const first = `${month}-01`;
  const nextMonth = m === 12 ? `${y + 1}-01` : `${y}-${String(m + 1).padStart(2, '0')}`;
  const prevMonth = m === 1 ? `${y - 1}-12` : `${y}-${String(m - 1).padStart(2, '0')}`;
  const f = F();
  const list = db.posts.list({ type: TYPE(), status: 'publish', perPage: 100, where: { ['meta.' + f.date]: { gte: first, lt: nextMonth + '-01' } },
    orderBy: 'meta.' + f.date } as any).items.map(toEvent).filter((e) => e.start.startsWith(month));
  const l = lang();
  const startDow = new Date(first + 'T00:00:00Z').getUTCDay();
  const days = new Date(Date.UTC(y, m, 0)).getUTCDate();
  let cells = DAYS[l].map((d) => `<b>${d}</b>`).join('');
  for (let i = 0; i < startDow; i++) cells += '<div class="ev-day ev-out"></div>';
  for (let d = 1; d <= days; d++) {
    const day = `${month}-${String(d).padStart(2, '0')}`;
    const evs = list.filter((e) => e.start.startsWith(day));
    cells += `<div class="ev-day${day === now.slice(0, 10) ? ' ev-today' : ''}">${d}` +
      evs.map((e) => `<a href="${esc(e.url)}" title="${esc(e.title)}">${e.start.length > 10 ? esc(e.start.slice(11)) + ' ' : ''}${esc(e.title)}</a>`).join('') + '</div>';
  }
  const items = list.map((e) => `<li><b>${esc(label(e.start))}</b> — <a href="${esc(e.url)}">${esc(e.title)}</a>${e.place ? ` · ${esc(e.place)}` : ''}</li>`).join('');
  return page(t('calendar'), `<h1>${esc(t('calendar'))}</h1><div class="ev-nav"><a href="/calendar?month=${prevMonth}">${esc(t('prev'))}</a>` +
    `<b>${esc(MONTHS[l][m - 1])} ${l === 0 ? y + 543 : y}</b><a href="/calendar?month=${nextMonth}">${esc(t('next'))}</a></div>` +
    `<div class="ev-grid" id="ev-grid">${cells}</div><ul class="ev-list">${items || `<li class="ev-muted">${esc(t('none'))}</li>`}</ul>` +
    (list.length ? '' : `<p class="ev-muted">${esc(t('none'))}</p>`));
});

const icsText = (s: string) => s.replace(/\\/g, '\\\\').replace(/[,;]/g, (c) => '\\' + c).replace(/\r?\n/g, '\\n');

site.route('GET /calendar/{id}', { access: ['*'] }, ({ request }) => {
  const m = /^(\d{1,12})\.ics$/.exec(request.params.id);
  const ev = m ? getEvent(Number(m[1])) : null;
  if (!ev || !ev.start) return { status: 404, headers: { 'Content-Type': 'text/plain' }, body: 'not found' };
  const dt = (s: string) => (s.length > 10 ? s.replace(/[-:]/g, '') + '00' : s.replace(/-/g, ''));
  const end = ev.end || (ev.start.length > 10 ? new Date(Date.parse(ev.start + ':00Z') + 2 * 3600_000).toISOString().slice(0, 16) : ev.start);
  let base = '';
  try {
    base = String(site.info().url || '').replace(/\/+$/, '');
  } catch {
    /* none */
  }
  const allDay = ev.start.length <= 10;
  const body = ['BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//Anasaki Studio//events//EN', 'BEGIN:VEVENT',
    `UID:event-${ev.id}@${base.replace(/^https?:\/\//, '') || 'anasaki'}`, `DTSTAMP:${new Date().toISOString().replace(/[-:]/g, '').slice(0, 15)}Z`,
    allDay ? `DTSTART;VALUE=DATE:${dt(ev.start)}` : `DTSTART:${dt(ev.start)}`, allDay ? `DTEND;VALUE=DATE:${dt(end)}` : `DTEND:${dt(end)}`,
    `SUMMARY:${icsText(ev.title)}`, ev.place ? `LOCATION:${icsText(ev.place)}` : '', `URL:${base}${ev.url}`, 'END:VEVENT', 'END:VCALENDAR']
    .filter(Boolean).join('\r\n');
  return { status: 200, headers: { 'Content-Type': 'text/calendar; charset=utf-8', 'Content-Disposition': `attachment; filename="event-${ev.id}.ics"` }, body };
});

// ---------------------------------------------------------------- mails

site.job('mailNew', (ctx) => {
  const r = db.table('registrations').where('id', Number(ctx.data?.id)).first() as Reg | null;
  if (!r) return;
  let base = '';
  try {
    base = String(site.info().url || '').replace(/\/+$/, '');
  } catch {
    /* none */
  }
  const ev = getEvent(r.post_id);
  if (r.email) {
    mail.send({ to: r.email, subject: t('mailSubject', { title: r.title }),
      html: `<p>${esc(r.name)}</p><p><b>${esc(r.title)}</b><br>${esc(t('when'))}: ${esc(label(r.starts))}${ev?.place ? `<br>${esc(t('where'))}: ${esc(ev.place)}` : ''}` +
        `<br>${esc(t('seats'))}: ${r.seats}</p><p><a href="${esc(base)}/calendar/r/${esc(r.code)}">${esc(t('yours'))}</a> · <a href="${esc(base)}/calendar/${r.post_id}.ics">${esc(t('ics'))}</a></p>` });
  }
  const to = set('notifyEmail');
  if (to) mail.send({ to, subject: `🎟️ ${r.title} — ${r.name} (${r.seats})`, text: `${r.name} ${r.email} ${r.phone}\n${r.note}` });
});

// ---------------------------------------------------------------- the admin page

const L = {
  title: ['ผู้ลงทะเบียนกิจกรรม', 'Event registrations'], event: ['กิจกรรม', 'Event'], when: ['วันเวลา', 'When'], count: ['ลงทะเบียน', 'Signed up'],
  back: ['← กิจกรรมทั้งหมด', '← All events'], name: ['ชื่อ', 'Name'], contact: ['ติดต่อ', 'Contact'], seats: ['ที่นั่ง', 'Seats'], status: ['สถานะ', 'Status'],
  checkin: ['เช็กอิน', 'Check in'], checked: ['มาแล้ว', 'Arrived'], cancel: ['ยกเลิก', 'Cancel'], csv: ['ส่งออก CSV', 'Export CSV'], none: ['ยังไม่มีผู้ลงทะเบียน', 'No sign-ups yet'],
  calendar: ['ปฏิทินบนเว็บ ↗', 'Public calendar ↗'],
} as const;
const q1 = (v: unknown) => String(Array.isArray(v) ? v[0] : v ?? '');
const csvCell = (v: unknown) => {
  const s = String(v ?? '');
  return /[",\n\r]|^[=+\-@]/.test(s) ? '"' + s.replace(/^([=+\-@])/, "'$1").replace(/"/g, '""') + '"' : s;
};

site.adminPage('registrations', {
  label: { th: 'ผู้ลงทะเบียนกิจกรรม', en: 'Event registrations' },
  icon: '🎟️',
  access: ACCESS,
  render: (ctx) => {
    const en = ctx.locale === 'en' ? 1 : 0;
    const tr = (k: keyof typeof L) => L[k][en];
    const evId = Number(q1(ctx.query.event)) || 0;
    let script = '';
    try {
      script = site.asset('assets/admin.js').url;
    } catch {
      /* none */
    }
    const head = `<header class="ak-page-head"><span class="ak-page-icon">🎟️</span><div class="ak-page-head-text"><h1>${esc(tr('title'))}</h1></div>` +
      `<div class="ak-actions"><a class="ak-btn ak-btn-sm" href="/calendar" target="_blank" rel="noopener">${esc(tr('calendar'))}</a></div></header>`;
    let body: string;
    if (evId) {
      const regs = db.table('registrations').where('post_id', evId).orderBy('id').get() as Reg[];
      const ev = getEvent(evId);
      body = `<p><a href="?" class="ak-btn ak-btn-sm ak-btn-ghost">${esc(tr('back'))}</a> <a href="#" class="ak-btn ak-btn-sm" data-ak-download="csv=${evId}">⬇ ${esc(tr('csv'))}</a></p>` +
        `<h2>${esc(ev?.title ?? regs[0]?.title ?? '')}</h2><p class="ak-hint">${esc(label(ev?.start ?? ''))} · ${taken(evId)}${ev && ev.seats ? ' / ' + ev.seats : ''}</p>` +
        (regs.length ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="ev-regs"><thead><tr><th>${esc(tr('name'))}</th><th>${esc(tr('contact'))}</th>` +
          `<th class="ak-num">${esc(tr('seats'))}</th><th>${esc(tr('status'))}</th><th></th></tr></thead><tbody>` +
          regs.map((r) => `<tr><td>${esc(r.name)}${r.note ? `<div class="ak-hint">${esc(r.note)}</div>` : ''}</td><td>${esc(r.email)}<div class="ak-hint">${esc(r.phone)}</div></td>` +
            `<td class="ak-num">${r.seats}</td><td>${r.status === 'cancelled' ? `<span class="ak-badge ak-badge-danger">${esc(tr('cancel'))}</span>` : r.checked_in_at ? `<span class="ak-badge ak-badge-ok">✓ ${esc(tr('checked'))}</span>` : ''}</td>` +
            `<td><div class="ak-actions">${r.status === 'confirmed' && !r.checked_in_at ? `<button class="ak-btn ak-btn-sm ak-btn-primary" data-ev="checkIn" data-id="${r.id}">${esc(tr('checkin'))}</button>` : ''}` +
            `${r.status === 'confirmed' ? `<button class="ak-btn ak-btn-sm ak-btn-danger" data-ev="cancel" data-id="${r.id}">${esc(tr('cancel'))}</button>` : ''}</div></td></tr>`).join('') +
          '</tbody></table></div></div>' : `<div class="ak-card"><p class="ak-muted">${esc(tr('none'))}</p></div>`);
    } else {
      const rows = db.table('registrations').where('status', 'confirmed').select('post_id').selectMax('title', 'title').selectMax('starts', 'starts')
        .selectSum('seats', 'n').groupBy('post_id').orderBy('starts', 'desc').limit(200).get() as { post_id: number; title: string; starts: string; n: number }[];
      body = rows.length ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="ev-events"><thead><tr><th>${esc(tr('event'))}</th><th>${esc(tr('when'))}</th><th class="ak-num">${esc(tr('count'))}</th></tr></thead><tbody>` +
        rows.map((r) => `<tr><td><a href="?event=${r.post_id}">${esc(r.title)}</a></td><td>${esc(label(r.starts))}</td><td class="ak-num">${Number(r.n)}</td></tr>`).join('') + '</tbody></table></div></div>'
        : `<div class="ak-card"><p class="ak-muted">${esc(tr('none'))}</p></div>`;
    }
    return { title: tr('title'), html: head + body + (script ? `<script src="${esc(script)}" defer></script>` : '') };
  },
  download: (ctx) => {
    const id = Number(q1(ctx.query.csv)) || 0;
    const lines = ['id,event,starts,name,email,phone,seats,status,checked_in,note,created'];
    for (const r of db.table('registrations').where('post_id', id).orderBy('id').get() as Reg[]) {
      lines.push([r.id, r.title, r.starts, r.name, r.email, r.phone, r.seats, r.status, r.checked_in_at ? 1 : 0, r.note, String(r.created_at).slice(0, 16)].map(csvCell).join(','));
    }
    return { status: 200, headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': `attachment; filename="event-${id}.csv"`, 'Cache-Control': 'no-store' }, body: '﻿' + lines.join('\r\n') };
  },
});

site.action('checkIn', {
  input: { id: 'number' },
  access: ACCESS,
  run: ({ input }) => {
    const n = db.table('registrations').where('id', Number(input.id)).where('status', 'confirmed').update({ checked_in_at: new Date() });
    return n ? { data: { id: Number(input.id) } } : { message: 'not found', errors: { id: 'not found' } };
  },
});

site.action('cancel', {
  input: { id: 'number' },
  access: ACCESS,
  run: ({ input }) => {
    const r = db.table('registrations').where('id', Number(input.id)).first() as Reg | null;
    if (!r || r.status !== 'confirmed') return { message: 'not found', errors: { id: 'not found' } };
    db.table('registrations').where('id', r.id).update({ status: 'cancelled' });
    site.emit('events.cancelled', { id: r.id, eventId: r.post_id, by: 'admin' });
    return { data: { id: r.id } };
  },
});
