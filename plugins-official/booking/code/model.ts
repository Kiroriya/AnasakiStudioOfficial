// Services, opening hours, closed days, bookings and the free time slots of a day.

/** capacity: bookings at once — or seats when max_party > 1 (a table for up to max_party people). deposit: paid to book (1.1). */
export interface Service {
  id: number; name: string; description: string; minutes: number; price: string; capacity: number; active: boolean; sort: number;
  max_party: number; deposit: string;
}
/** Someone who gives services (1.1); services = ids joined by commas, "" = every service. */
export interface Staff { id: number; name: string; services: string; active: boolean; sort: number }
export interface Hours { weekday: number; open_min: number; close_min: number; closed: boolean }
export type BStatus = 'pending' | 'confirmed' | 'cancelled' | 'done';
export const STATUSES: BStatus[] = ['pending', 'confirmed', 'cancelled', 'done'];
export interface Booking {
  id: number; code: string; service_id: number; service_name: string; day: string; start_min: number; end_min: number;
  name: string; phone: string; email: string; note: string; status: BStatus; created_at: string;
  /** 1.1 */
  party: number; staff_id: number; staff_name: string; answers: string | null; deposit: string; slip: string; slip_at: string | null;
  reminded_at: string | null;
}

/** The booking still waits for its deposit (no slip yet). */
export const awaitingDeposit = (b: Booking) => b.status === 'pending' && Number(b.deposit) > 0 && !b.slip;

export const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);

/** "09:30" for 570. */
export const hhmm = (m: number) => `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`;
export const parseHHMM = (s: string): number | null => {
  const m = /^(\d{1,2}):(\d{2})$/.exec(String(s ?? '').trim());
  if (!m) return null;
  const h = Number(m[1]), mm = Number(m[2]);
  return h <= 24 && mm < 60 && h * 60 + mm <= 1440 ? h * 60 + mm : null;
};
export const isDay = (s: string) => /^\d{4}-\d{2}-\d{2}$/.test(s) && !Number.isNaN(Date.parse(s + 'T00:00:00Z'));
export const dayStr = (v: unknown) => String(v ?? '').slice(0, 10);

// The runtime has no time-zone database: the site's zone maps to a fixed offset (no daylight saving).
const OFFSETS: Record<string, number> = {
  'Asia/Bangkok': 420, 'Asia/Jakarta': 420, 'Asia/Ho_Chi_Minh': 420, 'Asia/Saigon': 420, 'Asia/Phnom_Penh': 420, 'Asia/Vientiane': 420,
  'Asia/Yangon': 390, 'Asia/Kolkata': 330, 'Asia/Dhaka': 360, 'Asia/Singapore': 480, 'Asia/Kuala_Lumpur': 480, 'Asia/Manila': 480,
  'Asia/Shanghai': 480, 'Asia/Hong_Kong': 480, 'Asia/Taipei': 480, 'Australia/Perth': 480, 'Asia/Tokyo': 540, 'Asia/Seoul': 540,
  'Asia/Dubai': 240, 'UTC': 0, 'Etc/UTC': 0, 'Europe/London': 0,
};
export function offsetMinutes(): number {
  let tz = '';
  try {
    tz = site.info().timezone || '';
  } catch {
    /* default */
  }
  return OFFSETS[tz] ?? 420;
}

/** The site's local "now": the day (YYYY-MM-DD), minutes after midnight and the weekday (0 = Sunday). */
export function localNow(): { day: string; minutes: number } {
  const d = new Date(Date.now() + offsetMinutes() * 60_000);
  return { day: d.toISOString().slice(0, 10), minutes: d.getUTCHours() * 60 + d.getUTCMinutes() };
}
export const weekday = (day: string) => new Date(day + 'T00:00:00Z').getUTCDay();
export const addDays = (day: string, n: number) => new Date(Date.parse(day + 'T00:00:00Z') + n * 86400_000).toISOString().slice(0, 10);

export const services = (all = false) =>
  (all ? db.table('services') : db.table('services').where('active', true)).orderBy('sort').orderBy('id').get() as Service[];
export const getService = (id: number) => db.table('services').where('id', id).first() as Service | null;

const DEFAULT_HOURS: Hours[] = [0, 1, 2, 3, 4, 5, 6].map((w) => ({ weekday: w, open_min: 540, close_min: 1080, closed: w === 0 }));

export function hours(): Hours[] {
  const rows = db.table('hours').get() as Hours[];
  return DEFAULT_HOURS.map((d) => rows.find((r) => Number(r.weekday) === d.weekday) ?? d)
    .map((h) => ({ weekday: Number(h.weekday), open_min: Number(h.open_min), close_min: Number(h.close_min), closed: !!h.closed }));
}

export const closedDays = () => (db.table('closed_days').orderBy('day').get() as { id: number; day: string; note: string }[]).map((r) => ({ ...r, day: dayStr(r.day) }));

/** Days a visitor may pick: from today (site time) for daysAhead days, without closed weekdays / days. */
export function bookableDays(): { day: string; open: boolean }[] {
  const ahead = Math.max(1, Math.min(180, Number(site.settings('daysAhead') ?? 30) || 30));
  const h = hours();
  const closed = new Set(closedDays().map((c) => c.day));
  const today = localNow().day;
  const out: { day: string; open: boolean }[] = [];
  for (let i = 0; i < ahead; i++) {
    const d = addDays(today, i);
    out.push({ day: d, open: !closed.has(d) && !h[weekday(d)].closed });
  }
  return out;
}

// ---------------------------------------------------------------- staff (1.1)

export const allStaff = () => db.table('staff').orderBy('sort').orderBy('id').get() as Staff[];
const gives = (s: Staff, svc: Service) => !String(s.services ?? '').trim() || String(s.services).split(',').map(Number).includes(Number(svc.id));
/** The active staff who give svc ([] = the service is booked without choosing anyone). */
export const staffFor = (svc: Service) => allStaff().filter((s) => s.active && gives(s, svc));

type Taken = { start_min: number; end_min: number; party: number; staff_id: number };
const overlaps = (b: Taken, s: number, e: number) => Number(b.start_min) < e && Number(b.end_min) > s;

/**
 * Who serves a booking of svc on day from s to e: staffId (0 = anyone free) among the service's staff, or null
 * when nobody is free. A person is busy with any booking of theirs, of any service.
 */
export function freeStaff(svc: Service, day: string, s: number, e: number, staffId = 0, taken?: Taken[]): Staff | null {
  const people = staffFor(svc).filter((p) => !staffId || p.id === staffId);
  if (!people.length) return null;
  const rows = taken ?? (db.table('bookings').where('day', day).whereIn('status', ['pending', 'confirmed']).where('staff_id', '>', 0)
    .select('start_min', 'end_min', 'party', 'staff_id').get() as Taken[]);
  return people.find((p) => !rows.some((b) => Number(b.staff_id) === p.id && overlaps(b, s, e))) ?? null;
}

/** The party sizes a visitor may choose (1 … max_party, at most the capacity). */
export const partySizes = (svc: Service) => {
  const max = Math.max(1, Math.min(Number(svc.max_party) || 1, Number(svc.capacity) || 1, 50));
  return Array.from({ length: max }, (_, i) => i + 1);
};

/**
 * Free start times (minutes) of a service on a day: steps of the service's length (at most 30 min apart).
 * party: people of the booking (seats of a service with max_party > 1); staffId: a chosen person (0 = anyone).
 */
export function freeSlots(svc: Service, day: string, party = 1, staffId = 0): number[] {
  if (!isDay(day) || !bookableDays().some((d) => d.day === day && d.open)) return [];
  const h = hours()[weekday(day)];
  const minutes = Math.max(5, Number(svc.minutes) || 60);
  const step = Math.min(30, minutes);
  const now = localNow();
  const notice = Math.max(0, Number(site.settings('minNoticeHours') ?? 2) || 0) * 60;
  // minutes after the day's midnight before which no slot may start (now + notice, in site time)
  const earliest = (Date.parse(now.day + 'T00:00:00Z') - Date.parse(day + 'T00:00:00Z')) / 60_000 + now.minutes + notice;
  const out: number[] = [];
  if (staffFor(svc).length) {
    // with staff: a time is free while one of them is
    const busy = db.table('bookings').where('day', day).whereIn('status', ['pending', 'confirmed']).where('staff_id', '>', 0)
      .select('start_min', 'end_min', 'party', 'staff_id').get() as Taken[];
    for (let s = h.open_min; s + minutes <= h.close_min; s += step) {
      if (s >= earliest && freeStaff(svc, day, s, s + minutes, staffId, busy)) out.push(s);
    }
    return out;
  }
  const taken = db.table('bookings').where('day', day).whereIn('status', ['pending', 'confirmed']).where('service_id', svc.id)
    .select('start_min', 'end_min', 'party', 'staff_id').get() as Taken[];
  const cap = Math.max(1, Number(svc.capacity) || 1);
  // seats (party sizes) when the service takes parties, else one per booking
  const seats = (Number(svc.max_party) || 1) > 1;
  const need = seats ? Math.max(1, party) : 1;
  for (let s = h.open_min; s + minutes <= h.close_min; s += step) {
    if (s < earliest) continue;
    const used = taken.filter((b) => overlaps(b, s, s + minutes)).reduce((n, b) => n + (seats ? Math.max(1, Number(b.party) || 1) : 1), 0);
    if (used + need <= cap) out.push(s);
  }
  return out;
}

// ---------------------------------------------------------------- the shop's own questions (setting "questions", 1.1)

export interface Question { key: string; label: string; required: boolean }

/** One per line; a line ending in * must be answered. Keys q0, q1 … follow the lines. */
export function questions(): Question[] {
  return String(site.settings('questions') ?? '').split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 10)
    .map((l, i) => ({ key: 'q' + i, label: l.replace(/\s*\*$/, '').slice(0, 200), required: /\*$/.test(l) }));
}

/** answers JSON → [{label, value}] */
export function answersOf(b: Booking): { label: string; value: string }[] {
  try {
    const a = JSON.parse(String(b.answers ?? '[]'));
    return Array.isArray(a) ? a.filter((x) => x && x.value).map((x) => ({ label: String(x.label ?? ''), value: String(x.value) })) : [];
  } catch {
    return [];
  }
}

/** When a booking starts, as a Date (the day and minutes are site time). */
export const startsAt = (b: Booking) => new Date(Date.parse(dayStr(b.day) + 'T00:00:00Z') + (Number(b.start_min) - offsetMinutes()) * 60_000);

export const getBooking = (id: number) => db.table('bookings').where('id', id).first() as Booking | null;
export const byCode = (code: string) => (/^[0-9a-f]{24}$/.test(code) ? (db.table('bookings').where('code', code).first() as Booking | null) : null);

export function listBookings(opts: { status?: string; from?: string; page?: number; perPage?: number }) {
  const perPage = Math.min(100, Math.max(1, opts.perPage ?? 50));
  const page = Math.max(1, opts.page ?? 1);
  const q = () => {
    const b = db.table('bookings');
    if (opts.status && STATUSES.includes(opts.status as BStatus)) b.where('status', opts.status);
    if (opts.from && isDay(opts.from)) b.where('day', '>=', opts.from);
    return b;
  };
  return { items: (q().orderBy('day').orderBy('start_min').limit(perPage).offset((page - 1) * perPage).get() as Booking[]).map((b) => ({ ...b, day: dayStr(b.day) })), total: q().count(), page, perPage };
}
