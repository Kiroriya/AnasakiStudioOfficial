// legacyImport: rows of the built-in plugin's tables (analytics_daily, analytics_visitors,
// analytics_page_visitors, analytics_referrers) → this plugin's tables. Idempotent: rows are matched by
// their natural key (day + path / hash / host); a counter keeps the larger of the stored and the imported
// value, so importing a batch twice changes nothing and views counted since the upgrade are never lost.
import { clipRunes, normDay } from './lib';

type Row = Record<string, any>;

interface Payload {
  kind: string;
  table: string;
  rows: Row[];
  last?: boolean;
}

const str = (v: unknown, n: number) => clipRunes(String(v ?? ''), n);
const int = (v: unknown) => Math.max(0, Math.floor(Number(v) || 0));

/** Counters (daily, referrers): insert missing keys, raise smaller stored values. */
function importCounters(table: string, keyCol: 'path' | 'host', rows: Row[], affected: Set<string>): number {
  const byDay = new Map<string, Row[]>();
  for (const r of rows) {
    const day = normDay(r.day);
    const key = str(r[keyCol], 191);
    if (!day || !key) continue;
    const list = byDay.get(day) ?? [];
    list.push({ day, key, views: int(r.views), title: str(r.title, 255) });
    byDay.set(day, list);
  }
  let n = 0;
  const inserts: Row[] = [];
  for (const [day, list] of byDay) {
    affected.add(day);
    const existing = new Map<string, Row>();
    for (const e of db.table(table).where('day', day).whereIn(keyCol, list.map((x) => x.key)).get() as Row[]) existing.set(String(e[keyCol]), e);
    for (const x of list) {
      n++;
      const e = existing.get(x.key);
      if (!e) {
        const row: Row = { day, [keyCol]: x.key, views: x.views };
        if (table === 'daily') row.title = x.title;
        inserts.push(row);
        existing.set(x.key, row);
        continue;
      }
      const patch: Row = {};
      if (x.views > (Number(e.views) || 0)) patch.views = x.views;
      if (table === 'daily' && x.title && !e.title) patch.title = x.title;
      if (Object.keys(patch).length && e.id) db.table(table).where('id', e.id).update(patch);
    }
  }
  for (let i = 0; i < inserts.length; i += 500) db.table(table).insert(inserts.slice(i, i + 500));
  return n;
}

/** Sets (visitors, page_visitors): insert the hashes not stored yet. */
function importSet(table: string, rows: Row[], affected: Set<string>): number {
  const withPath = table === 'page_visitors';
  const byDay = new Map<string, Row[]>();
  for (const r of rows) {
    const day = normDay(r.day);
    const hash = str(r.hash, 16);
    const path = withPath ? str(r.path, 191) : '';
    if (!day || !hash || (withPath && !path)) continue;
    const list = byDay.get(day) ?? [];
    list.push(withPath ? { day, path, hash } : { day, hash });
    byDay.set(day, list);
  }
  let n = 0;
  const inserts: Row[] = [];
  for (const [day, list] of byDay) {
    affected.add(day);
    const seen = new Set<string>();
    const q = db.table(table).where('day', day).whereIn('hash', [...new Set(list.map((x) => x.hash))]);
    for (const e of q.get() as Row[]) seen.add((withPath ? e.path + '\u0000' : '') + e.hash);
    for (const x of list) {
      n++;
      const k = (withPath ? x.path + '\u0000' : '') + x.hash;
      if (seen.has(k)) continue;
      seen.add(k);
      inserts.push(x);
    }
  }
  for (let i = 0; i < inserts.length; i += 500) db.table(table).insert(inserts.slice(i, i + 500));
  return n;
}

/** Recomputes the per-day totals (views = Σ daily, visitors = distinct hashes) of the given days. */
function refreshDays(days: Set<string>): void {
  for (const day of days) {
    const views = Number(db.table('daily').where('day', day).sum('views')) || 0;
    const visitors = db.table('visitors').where('day', day).count();
    if (db.table('days').where('day', day).update({ views, visitors }) === 0 && !db.table('days').where('day', day).exists()) {
      db.table('days').insert({ day, views, visitors });
    }
  }
}

const TABLES: Record<string, (rows: Row[], affected: Set<string>) => number> = {
  analytics_daily: (rows, a) => importCounters('daily', 'path', rows, a),
  analytics_referrers: (rows, a) => importCounters('referrers', 'host', rows, a),
  analytics_visitors: (rows, a) => importSet('visitors', rows, a),
  analytics_page_visitors: (rows, a) => importSet('page_visitors', rows, a),
};

export function legacyImport(p: Payload): { imported: number } {
  if (!p || p.kind !== 'rows' || !Array.isArray(p.rows)) return { imported: 0 };
  const fn = TABLES[p.table];
  if (!fn) return { imported: 0 };
  // no transaction: every step is idempotent, so a failed batch is simply sent again
  const affected = new Set<string>();
  const imported = fn(p.rows, affected);
  if (p.table === 'analytics_daily' || p.table === 'analytics_visitors') refreshDays(affected);
  return { imported };
}
