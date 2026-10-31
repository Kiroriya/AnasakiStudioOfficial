// The statistics summary — same shape and rules as the Go plugin's GET /api/v1/analytics/summary.
import { daysAgo, normDay } from './lib';

export interface Summary {
  totals: { views: number; visitors: number; todayViews: number; todayVisitors: number };
  daily: { day: string; views: number; visitors: number }[];
  topPages: { path: string; title: string; views: number; visitors: number }[];
  topReferrers: { host: string; views: number }[];
}

/** 30 and 90 are kept, anything else is 7 (like the Go plugin). */
export function rangeOf(v: unknown): number {
  const n = Number(v);
  return n === 30 || n === 90 ? n : 7;
}

interface Agg {
  key: string;
  views: number;
  title: string;
}

/** Top 10 keys of a (day, key, views[, title]) table since a day, by summed views then key (SQL aggregates). */
function top(table: string, keyCol: string, since: string, withTitle: boolean): Agg[] {
  let q = db.table(table).select(keyCol).selectSum('views', 'v');
  if (withTitle) q = q.selectMax('title', 't');
  const rows = q.where('day', '>=', since).groupBy(keyCol).orderBy('v', 'desc').orderBy(keyCol, 'asc').limit(10).get();
  return rows.map((r: any) => ({ key: String(r[keyCol]), views: Number(r.v) || 0, title: String(r.t ?? '') }));
}

/** Title of the page / post a path shows now (titles change after they were counted). */
function titleForPath(p: string): string {
  const parts = p.replace(/^\/+|\/+$/g, '').split('/');
  if (p === '/' || parts.length > 2 || parts[0] === '') return '';
  try {
    const post = db.posts.get(parts[parts.length - 1], { type: parts.length === 1 ? 'page' : 'post' });
    if (!post) return '';
    const url = String(post.url ?? '').replace(/^[a-z]+:\/\/[^/]+/i, '');
    return url.replace(/\/+$/, '') === p.replace(/\/+$/, '') ? String(post.title ?? '') : '';
  } catch {
    return '';
  }
}

export function summary(daysIn?: unknown): Summary {
  const days = rangeOf(daysIn);
  const now = new Date();
  const today = daysAgo(0, now);
  const since = daysAgo(days - 1, now);
  const out: Summary = { totals: { views: 0, visitors: 0, todayViews: 0, todayVisitors: 0 }, daily: [], topPages: [], topReferrers: [] };
  const byDay: Record<string, { day: string; views: number; visitors: number }> = {};
  for (let i = days - 1; i >= 0; i--) {
    const d = { day: daysAgo(i, now), views: 0, visitors: 0 };
    out.daily.push(d);
    byDay[d.day] = d;
  }
  for (const r of db.table('days').where('day', '>=', since).get() as any[]) {
    const d = byDay[normDay(r.day)];
    if (d) {
      d.views = Number(r.views) || 0;
      d.visitors = Number(r.visitors) || 0;
    }
  }
  for (const d of out.daily) {
    out.totals.views += d.views;
    out.totals.visitors += d.visitors;
    if (d.day === today) {
      out.totals.todayViews = d.views;
      out.totals.todayVisitors = d.visitors;
    }
  }
  for (const a of top('daily', 'path', since, true)) {
    const visitors = db.table('page_visitors').where('day', '>=', since).where('path', a.key).count();
    out.topPages.push({ path: a.key, title: titleForPath(a.key) || a.title, views: a.views, visitors });
  }
  for (const a of top('referrers', 'host', since, false)) out.topReferrers.push({ host: a.key, views: a.views });
  return out;
}
