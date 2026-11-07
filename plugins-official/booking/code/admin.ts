// The "Bookings" admin page: upcoming bookings (confirm, mark done, cancel, the deposit slip), services, staff
// (1.1), weekly hours and closed days. Drawn on the server with the admin UI kit; assets/admin.js sends the forms through anasaki.call.
import {
  BStatus, STATUSES, allStaff, answersOf, awaitingDeposit, closedDays, esc, getBooking, hhmm, hours, isDay, listBookings, localNow, parseHHMM, services,
} from './model';

const CAPS = typeof (site as any).can === 'function';
const ACCESS = CAPS ? ['cap:booking.manage'] : ['editor'];

const L = {
  title: ['การจอง', 'Bookings'], desc: ['คิวที่ลูกค้าจองผ่านหน้า /booking', 'Appointments made on the /booking page'],
  list: ['รายการจอง', 'Bookings'], svc: ['บริการ', 'Services'], hours: ['เวลาเปิด', 'Opening hours'],
  upcoming: ['ที่จะมาถึง', 'Upcoming'], all: ['ทั้งหมด', 'All'],
  pending: ['รอยืนยัน', 'Pending'], confirmed: ['ยืนยันแล้ว', 'Confirmed'], cancelled: ['ยกเลิก', 'Cancelled'], done: ['เสร็จสิ้น', 'Done'],
  when: ['วันเวลา', 'When'], customer: ['ลูกค้า', 'Customer'], service: ['บริการ', 'Service'], status: ['สถานะ', 'Status'],
  none: ['ยังไม่มี', 'None yet'], confirm: ['ยืนยัน', 'Confirm'], markDone: ['เสร็จสิ้น', 'Done'], cancel: ['ยกเลิก', 'Cancel'],
  name: ['ชื่อบริการ', 'Name'], minutes: ['นาที', 'Minutes'], price: ['ราคา (บาท)', 'Price (THB)'], capacity: ['รับพร้อมกัน', 'At once'],
  active: ['เปิดจอง', 'Bookable'], save: ['บันทึก', 'Save'], add: ['＋ เพิ่มบริการ', '＋ Add a service'], descr: ['คำอธิบาย', 'Description'],
  closed: ['ปิด', 'Closed'], open: ['เปิด', 'Opens'], close: ['ปิด', 'Closes'], closedDays: ['วันหยุดพิเศษ', 'Closed days'],
  addDay: ['เพิ่มวันหยุด', 'Add a closed day'], note: ['หมายเหตุ', 'Note'], remove: ['ลบ', 'Remove'],
  page: ['หน้าจองของลูกค้า ↗', 'Booking page ↗'],
  maxParty: ['จำนวนคนสูงสุดต่อการจอง', 'Most people per booking'],
  partyHint: ['มากกว่า 1 = ลูกค้าเลือกจำนวนคน และ "รับพร้อมกัน" นับเป็นจำนวนที่นั่ง', 'Over 1 = customers pick a party size and "At once" counts seats'],
  deposit: ['มัดจำ (บาท)', 'Deposit (THB)'], staff: ['ผู้ให้บริการ', 'Staff'], staffName: ['ชื่อ', 'Name'],
  staffSvc: ['ให้บริการ', 'Gives'], allSvc: ['ทุกบริการ', 'Every service'], addStaff: ['＋ เพิ่มผู้ให้บริการ', '＋ Add a person'],
  staffHint: ['เมื่อบริการมีผู้ให้บริการ ลูกค้าเลือกคนได้ (หรือใครก็ได้) และแต่ละคนรับได้ทีละหนึ่งการจอง', 'When a service has staff, customers may pick one (or anyone) and each person takes one booking at a time'],
  people: ['คน', 'people'], depositWait: ['รอมัดจำ', 'Deposit due'], slip: ['สลิปมัดจำ', 'Deposit slip'],
} as const;
type LK = keyof typeof L;
const DAYS = [['อาทิตย์', 'จันทร์', 'อังคาร', 'พุธ', 'พฤหัสบดี', 'ศุกร์', 'เสาร์'], ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']];
const q1 = (v: string | string[] | undefined) => String(Array.isArray(v) ? v[0] : v ?? '');

export function registerAdmin(): void {
  site.adminPage('bookings', {
    label: { th: 'การจอง', en: 'Bookings' },
    icon: '📅',
    access: ACCESS,
    render: (ctx) => {
      const en = ctx.locale === 'en' ? 1 : 0;
      const tr = (k: LK) => L[k][en];
      const tab = q1(ctx.query.tab) || 'list';
      let body = '';
      if (tab === 'svc') {
        const rows = services(true).map((s) => `<form class="ak-card" data-bk-form="saveService" style="display:grid;gap:10px;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));align-items:end">` +
          `<input type="hidden" name="id" value="${s.id}"><label class="ak-field"><span>${esc(tr('name'))}</span><input name="name" value="${esc(s.name)}" maxlength="120" required></label>` +
          `<label class="ak-field"><span>${esc(tr('minutes'))}</span><input type="number" name="minutes" value="${s.minutes}" min="5" max="720"></label>` +
          `<label class="ak-field"><span>${esc(tr('price'))}</span><input type="number" name="price" value="${Number(s.price)}" min="0" step="0.01"></label>` +
          `<label class="ak-field"><span>${esc(tr('capacity'))}</span><input type="number" name="capacity" value="${s.capacity}" min="1" max="500"></label>` +
          `<label class="ak-field"><span>${esc(tr('maxParty'))}</span><input type="number" name="max_party" value="${Number(s.max_party) || 1}" min="1" max="50" title="${esc(tr('partyHint'))}"></label>` +
          `<label class="ak-field"><span>${esc(tr('deposit'))}</span><input type="number" name="deposit" value="${Number(s.deposit) || 0}" min="0" step="0.01"></label>` +
          `<div class="ak-actions"><label class="ak-check"><input type="checkbox" name="active"${s.active ? ' checked' : ''}> ${esc(tr('active'))}</label><button class="ak-btn ak-btn-primary">${esc(tr('save'))}</button></div>` +
          `<label class="ak-field" style="grid-column:1/-1"><span>${esc(tr('descr'))}</span><input name="description" value="${esc(s.description)}" maxlength="500"></label>` +
          `<p class="ak-hint" style="grid-column:1/-1">${esc(tr('partyHint'))}</p></form>`).join('');
        body = rows + `<form class="ak-card" data-bk-form="saveService" id="bk-new-service" style="display:grid;gap:10px;grid-template-columns:2fr 1fr 1fr auto;align-items:end">` +
          `<label class="ak-field"><span>${esc(tr('name'))}</span><input name="name" maxlength="120" required></label>` +
          `<label class="ak-field"><span>${esc(tr('minutes'))}</span><input type="number" name="minutes" value="60" min="5" max="720"></label>` +
          `<label class="ak-field"><span>${esc(tr('price'))}</span><input type="number" name="price" value="0" min="0" step="0.01"></label>` +
          `<input type="hidden" name="active" value="on"><button class="ak-btn ak-btn-primary">${esc(tr('add'))}</button></form>`;
      } else if (tab === 'staff') {
        const svcs = services(true);
        const boxes = (chosen: string, id: string) => {
          const ids = String(chosen ?? '').split(',').filter(Boolean).map(Number);
          return `<div class="ak-actions" style="flex-wrap:wrap">` + svcs.map((s) =>
            `<label class="ak-check"><input type="checkbox" name="svc" value="${s.id}"${ids.includes(s.id) ? ' checked' : ''} data-of="${id}"> ${esc(s.name)}</label>`).join('') + '</div>';
        };
        body = `<p class="ak-hint">${esc(tr('staffHint'))} · ${esc(tr('allSvc'))} = ${en ? 'none ticked' : 'ไม่ติ๊กเลย'}</p>` +
          allStaff().map((p) => `<form class="ak-card" data-bk-form="saveStaff" style="display:grid;gap:10px">` +
            `<input type="hidden" name="id" value="${p.id}"><div class="ak-actions"><label class="ak-field" style="flex:1"><span>${esc(tr('staffName'))}</span><input name="name" value="${esc(p.name)}" maxlength="120" required></label>` +
            `<label class="ak-check"><input type="checkbox" name="active"${p.active ? ' checked' : ''}> ${esc(tr('active'))}</label><button class="ak-btn ak-btn-primary">${esc(tr('save'))}</button>` +
            `<button type="button" class="ak-btn ak-btn-sm ak-btn-danger" data-bk="removeStaff" data-id="${p.id}">${esc(tr('remove'))}</button></div>` +
            `<span class="ak-hint">${esc(tr('staffSvc'))}</span>${boxes(p.services, String(p.id))}</form>`).join('') +
          `<form class="ak-card" data-bk-form="saveStaff" id="bk-new-staff" style="display:grid;gap:10px"><div class="ak-actions"><label class="ak-field" style="flex:1"><span>${esc(tr('staffName'))}</span><input name="name" maxlength="120" required></label>` +
          `<input type="hidden" name="active" value="on"><button class="ak-btn ak-btn-primary">${esc(tr('addStaff'))}</button></div><span class="ak-hint">${esc(tr('staffSvc'))}</span>${boxes('', 'new')}</form>`;
      } else if (tab === 'hours') {
        const h = hours();
        body = `<form class="ak-card" data-bk-form="saveHours"><table class="ak-table" id="bk-hours"><tbody>` + [1, 2, 3, 4, 5, 6, 0].map((w) => {
          const x = h[w];
          return `<tr><td><b>${esc(DAYS[en][w])}</b></td><td><label class="ak-check"><input type="checkbox" name="closed${w}"${x.closed ? ' checked' : ''}> ${esc(tr('closed'))}</label></td>` +
            `<td><input type="time" name="open${w}" value="${hhmm(x.open_min)}" aria-label="${esc(tr('open'))}"></td><td><input type="time" name="close${w}" value="${hhmm(x.close_min)}" aria-label="${esc(tr('close'))}"></td></tr>`;
        }).join('') + `</tbody></table><div class="ak-form-actions"><button class="ak-btn ak-btn-primary">${esc(tr('save'))}</button></div></form>` +
          `<div class="ak-card"><h3>${esc(tr('closedDays'))}</h3>` + closedDays().map((c) => `<div class="ak-actions" style="margin-bottom:6px"><b>${esc(c.day)}</b> <span class="ak-hint">${esc(c.note)}</span>` +
            `<button class="ak-btn ak-btn-sm ak-btn-danger" data-bk="removeClosedDay" data-id="${c.id}">${esc(tr('remove'))}</button></div>`).join('') +
          `<form data-bk-form="addClosedDay" class="ak-actions" style="margin-top:10px"><input type="date" name="day" required style="max-width:180px"><input name="note" placeholder="${esc(tr('note'))}" maxlength="120" style="max-width:260px">` +
          `<button class="ak-btn">${esc(tr('addDay'))}</button></form></div>`;
      } else {
        const status = q1(ctx.query.status);
        const from = status ? '' : localNow().day;
        const r = listBookings({ status, from, page: Math.max(1, Number(q1(ctx.query.page)) || 1) });
        const st = (s: string, label: string) => `<a class="ak-tab" href="?tab=list&status=${s}" aria-selected="${status === s}">${esc(label)}</a>`;
        const badge = (s: string) => `<span class="ak-badge ${s === 'confirmed' ? 'ak-badge-ok' : s === 'pending' ? 'ak-badge-warn' : s === 'cancelled' ? 'ak-badge-danger' : ''}">${esc(tr(s as LK))}</span>`;
        const btn = (id: number, s: BStatus, label: string, cls = '') => `<button class="ak-btn ak-btn-sm ${cls}" data-bk="setStatus" data-status="${s}" data-id="${id}">${esc(label)}</button>`;
        const extra = (b: (typeof r.items)[number]) => answersOf(b).map((a) => `<div class="ak-hint">${esc(a.label)}: ${esc(a.value)}</div>`).join('') +
          (b.note ? `<div class="ak-hint">${esc(b.note)}</div>` : '');
        const pay = (b: (typeof r.items)[number]) => !(Number(b.deposit) > 0) ? '' : b.slip
          ? `<div><a href="#" class="ak-hint" data-ak-download="slip=${b.id}">🧾 ${esc(tr('slip'))}</a></div>`
          : awaitingDeposit(b) ? `<div><span class="ak-badge ak-badge-warn">${esc(tr('depositWait'))} ฿${Number(b.deposit)}</span></div>` : '';
        const rows = r.items.map((b) => `<tr><td><b>${esc(b.day)}</b> ${hhmm(b.start_min)}–${hhmm(b.end_min)}</td><td>${esc(b.name)}<div class="ak-hint">${esc(b.phone)}${b.email ? ' · ' + esc(b.email) : ''}</div>${extra(b)}</td>` +
          `<td>${esc(b.service_name)}${Number(b.party) > 1 ? `<div class="ak-hint">👥 ${b.party} ${esc(tr('people'))}</div>` : ''}${b.staff_name ? `<div class="ak-hint">👤 ${esc(b.staff_name)}</div>` : ''}</td>` +
          `<td>${badge(b.status)}${pay(b)}</td><td><div class="ak-actions">` +
          (b.status === 'pending' ? btn(b.id, 'confirmed', tr('confirm'), 'ak-btn-primary') : '') +
          (b.status === 'confirmed' ? btn(b.id, 'done', tr('markDone')) : '') +
          (b.status === 'pending' || b.status === 'confirmed' ? btn(b.id, 'cancelled', tr('cancel'), 'ak-btn-danger') : '') + '</div></td></tr>').join('');
        body = `<div class="ak-tabs">${st('', tr('upcoming'))}${STATUSES.map((s) => st(s, tr(s as LK))).join('')}</div>` +
          (rows ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="bk-bookings"><thead><tr><th>${esc(tr('when'))}</th><th>${esc(tr('customer'))}</th><th>${esc(tr('service'))}</th><th>${esc(tr('status'))}</th><th></th></tr></thead><tbody>${rows}</tbody></table></div></div>`
            : `<div class="ak-card"><p class="ak-muted">${esc(tr('none'))}</p></div>`);
      }
      let script = '';
      try {
        script = site.asset('assets/admin.js').url;
      } catch {
        /* none */
      }
      return {
        title: tr('title'),
        html: `<header class="ak-page-head"><span class="ak-page-icon">📅</span><div class="ak-page-head-text"><h1>${esc(tr('title'))}</h1><p>${esc(tr('desc'))}</p></div>` +
          `<div class="ak-actions"><a class="ak-btn ak-btn-sm" href="/booking" target="_blank" rel="noopener">${esc(tr('page'))}</a></div></header>` +
          `<div class="ak-tabs"><a class="ak-tab" href="?tab=list" aria-selected="${tab === 'list'}">${esc(tr('list'))}</a><a class="ak-tab" href="?tab=svc" aria-selected="${tab === 'svc'}">${esc(tr('svc'))}</a>` +
          `<a class="ak-tab" href="?tab=staff" aria-selected="${tab === 'staff'}">${esc(tr('staff'))}</a>` +
          `<a class="ak-tab" href="?tab=hours" aria-selected="${tab === 'hours'}">${esc(tr('hours'))}</a></div>` + body +
          (script ? `<script src="${esc(script)}" defer></script>` : ''),
      };
    },
    // the deposit slip of a booking
    download: (ctx) => {
      const b = getBooking(Number(q1(ctx.query.slip)) || 0);
      const bytes = b?.slip ? files.get(b.slip) : null;
      if (!b || !bytes) return { status: 404, headers: { 'Content-Type': 'text/plain' }, body: 'not found' };
      const png = b.slip.endsWith('.png');
      return { status: 200, headers: { 'Content-Type': png ? 'image/png' : 'image/jpeg', 'Content-Disposition': `attachment; filename="slip-${b.id}${png ? '.png' : '.jpg'}"` }, body: bytes };
    },
  });

  const fail = (m: string) => ({ message: m, errors: { form: m } });

  site.action('setStatus', {
    input: { id: 'number', status: 'text' },
    access: ACCESS,
    run: ({ input }) => {
      const b = getBooking(Number(input.id));
      if (!b) return fail('not found');
      if (!STATUSES.includes(input.status as BStatus)) return fail('bad status');
      if (b.status !== input.status) {
        db.table('bookings').where('id', b.id).update({ status: input.status });
        site.emit('booking.status', { id: b.id, from: b.status, status: input.status, by: 'admin' });
        if (input.status === 'confirmed' || input.status === 'cancelled') jobs.enqueue('mailStatus', { id: b.id });
      }
      return { data: { id: b.id, status: input.status } };
    },
  });

  site.action('saveService', {
    input: { id: 'number?', name: 'text', minutes: 'number?', price: 'number?', capacity: 'number?', active: 'text?', description: 'text?', max_party: 'number?', deposit: 'number?' },
    access: ACCESS,
    run: ({ input }) => {
      const row = {
        name: String(input.name).trim().slice(0, 120),
        minutes: Math.min(720, Math.max(5, Math.round(Number(input.minutes) || 60))),
        price: Math.max(0, Number(input.price) || 0),
        capacity: Math.min(500, Math.max(1, Math.round(Number(input.capacity) || 1))),
        max_party: Math.min(50, Math.max(1, Math.round(Number(input.max_party) || 1))),
        deposit: Math.max(0, Number(input.deposit) || 0),
        active: input.active === 'on' || input.active === 'true',
        description: String(input.description ?? '').trim().slice(0, 500),
      };
      if (!row.name) return fail('name');
      if (input.id) {
        db.table('services').where('id', Number(input.id)).update(row);
        return { data: { id: Number(input.id) } };
      }
      return { data: { id: db.table('services').insert(row) } };
    },
  });

  site.action('saveStaff', {
    input: { id: 'number?', name: 'text', active: 'text?', services: 'text?' },
    access: ACCESS,
    run: ({ input }) => {
      const ids = String(input.services ?? '').split(',').map((x) => Math.floor(Number(x))).filter((n) => n > 0).slice(0, 60);
      const row = { name: String(input.name).trim().slice(0, 120), active: input.active === 'on' || input.active === 'true', services: [...new Set(ids)].join(',') };
      if (!row.name) return fail('name');
      if (input.id) {
        db.table('staff').where('id', Number(input.id)).update(row);
        return { data: { id: Number(input.id) } };
      }
      return { data: { id: db.table('staff').insert(row) } };
    },
  });

  // a person leaves; their past bookings keep the name
  site.action('removeStaff', {
    input: { id: 'number' },
    access: ACCESS,
    run: ({ input }) => ({ data: { removed: db.table('staff').where('id', Number(input.id)).delete() } }),
  });

  site.action('saveHours', {
    input: { values: 'json' },
    access: ACCESS,
    run: ({ input }) => {
      const v = (input.values ?? {}) as Record<string, string>;
      for (let w = 0; w < 7; w++) {
        const open = parseHHMM(v['open' + w]) ?? 540;
        const close = parseHHMM(v['close' + w]) ?? 1080;
        if (close <= open) return fail(DAYS[0][w] + ': ' + v['open' + w] + '–' + v['close' + w]);
        const row = { weekday: w, open_min: open, close_min: close, closed: v['closed' + w] === 'on' };
        if (db.table('hours').where('weekday', w).exists()) db.table('hours').where('weekday', w).update(row);
        else db.table('hours').insert(row);
      }
      return { message: '✓' };
    },
  });

  site.action('addClosedDay', {
    input: { day: 'text', note: 'text?' },
    access: ACCESS,
    run: ({ input }) => {
      const day = String(input.day);
      if (!isDay(day)) return fail('day');
      if (!db.table('closed_days').where('day', day).exists()) db.table('closed_days').insert({ day, note: String(input.note ?? '').slice(0, 120) });
      return { message: '✓' };
    },
  });

  site.action('removeClosedDay', {
    input: { id: 'number' },
    access: ACCESS,
    run: ({ input }) => ({ data: { removed: db.table('closed_days').where('id', Number(input.id)).delete() } }),
  });
}
