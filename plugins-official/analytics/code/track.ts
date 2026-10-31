// Recording page views (site.afterResponse) — the Go plugin's middleware + writer, one hit at a time.
import { byteLen, clipRunes, dayStr, isPrefetch, refHost, trackable, visitorHash, hostOf } from './lib';

/** The request's own host (internal referrers are not counted as sources). */
function siteHosts(info: ResponseInfo): string[] {
  const h = hostOf('http://' + String(info.host ?? ''));
  return h ? [h] : [];
}

/** The secret salt of day, rotated daily; older salts are deleted (yesterday's hashes become unlinkable). */
export function saltFor(day: string): string {
  const key = 'salt:' + day;
  const hit = cache.get<string>(key);
  if (hit) return hit;
  let salt = '';
  const row = db.table('salts').where('day', day).first();
  if (row) salt = String(row.salt);
  else {
    salt = crypto.random(16);
    try {
      db.table('salts').insert({ day, salt });
      db.table('salts').where('day', '<', day).delete();
    } catch {
      const again = db.table('salts').where('day', day).first(); // another process won the race
      if (again) salt = String(again.salt);
    }
  }
  cache.set(key, salt, 2 * 86400);
  return salt;
}

/** Adds n to a counter row identified by key, creating it when missing (unique key → retry on a race). */
function bump(table: string, key: Record<string, SqlValue>, inc: Record<string, number>, extra: Record<string, SqlValue> = {}): void {
  const cols = Object.keys(inc);
  const first = cols[0];
  const rest: Record<string, SqlValue> = { ...extra };
  // increment() takes one column; the others are raised with a follow-up update of the same row
  const n = db.table(table).where(key).increment(first, inc[first], rest);
  if (n > 0) {
    for (const c of cols.slice(1)) if (inc[c]) db.table(table).where(key).increment(c, inc[c]);
    return;
  }
  try {
    db.table(table).insert({ ...key, ...extra, ...inc });
  } catch {
    db.table(table).where(key).increment(first, inc[first], rest);
    for (const c of cols.slice(1)) if (inc[c]) db.table(table).where(key).increment(c, inc[c]);
  }
}

/** Inserts a row unless the unique key exists; true when it was new. */
function addOnce(table: string, row: Record<string, SqlValue>): boolean {
  if (db.table(table).where(row).exists()) return false;
  try {
    db.table(table).insert(row);
    return true;
  } catch {
    return false;
  }
}

export interface Hit {
  day: string;
  path: string;
  title: string;
  visitor: string;
  ref: string;
}

/** Writes one page view into the aggregate tables. */
export function store(h: Hit): void {
  let title = h.title;
  if (byteLen(title) > 255) title = Array.from(title).slice(0, 80).join('');
  bump('daily', { day: h.day, path: h.path }, { views: 1 }, title ? { title } : {});
  const newVisitor = addOnce('visitors', { day: h.day, hash: h.visitor });
  addOnce('page_visitors', { day: h.day, path: h.path, hash: h.visitor });
  if (h.ref) bump('referrers', { day: h.day, host: h.ref }, { views: 1 });
  bump('days', { day: h.day }, newVisitor ? { views: 1, visitors: 1 } : { views: 1 });
}

/** site.afterResponse: counts successful HTML page views by people (not bots, prefetches, API calls…). */
export function record(info: ResponseInfo): void {
  if (!trackable(info.method, info.path, info.userAgent)) return;
  if (info.status !== 200 || !String(info.contentType ?? '').startsWith('text/html')) return;
  if (isPrefetch(info.headers ?? {})) return; // prefetch / prerender
  const day = dayStr(new Date());
  store({
    day,
    path: clipRunes(info.path, 191),
    title: info.title ?? '',
    visitor: visitorHash(saltFor(day), info.ip, info.userAgent),
    ref: refHost(info.referrer, siteHosts(info)),
  });
}
