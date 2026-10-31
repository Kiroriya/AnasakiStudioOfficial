/**
 * Admin-wide joints of JS plugins / Site Code: notices in the admin shell
 * (site.adminNotice → GET /code/admin-notices) and dashboard widgets
 * (site.dashboardWidget → GET /code/dashboard-widgets). Pure helpers, unit
 * tested in admin-ext.spec.ts.
 */
import { CodeLabel } from './code-models';

export type NoticeLevel = 'info' | 'warning' | 'danger' | 'success';

export interface AdminNotice {
  id: string;
  unit: string;
  level: NoticeLevel;
  /** Already in the user's admin language. */
  text: string;
  link?: { label: string; href: string; external: boolean };
}

export interface DashboardWidget {
  /** "<unit>:<id>". */
  id: string;
  label: CodeLabel;
  icon: string;
  /** "/ext-ui/code/<ticket>/widget/<id>" (a sandboxed document like admin pages). */
  url: string;
  height: number;
}

const LEVELS: NoticeLevel[] = ['info', 'warning', 'danger', 'success'];

/** Valid notices only (unknown levels → info). */
export function normalizeNotices(raw: unknown): AdminNotice[] {
  if (!Array.isArray(raw)) return [];
  const out: AdminNotice[] = [];
  for (const x of raw as Record<string, unknown>[]) {
    if (!x || typeof x !== 'object' || typeof x['text'] !== 'string' || !x['text']) continue;
    const n: AdminNotice = {
      id: String(x['id'] ?? out.length), unit: String(x['unit'] ?? ''), text: x['text'] as string,
      level: LEVELS.includes(x['level'] as NoticeLevel) ? (x['level'] as NoticeLevel) : 'info',
    };
    const l = x['link'] as Record<string, unknown> | undefined;
    if (l && typeof l['href'] === 'string' && safeHref(l['href'])) {
      n.link = { label: String(l['label'] ?? l['href']), href: l['href'], external: !!l['external'] };
    }
    out.push(n);
  }
  return out;
}

function safeHref(h: string): boolean {
  return /^https?:\/\//i.test(h) || (h.startsWith('/') && !h.startsWith('//'));
}

export type NoticeTarget = { kind: 'router'; path: string } | { kind: 'tab'; href: string };

/**
 * Where a notice link goes: an admin path (under the admin base, or /admin/…)
 * opens in the app; other site paths and external URLs open in a new tab.
 */
export function noticeTarget(link: { href: string; external: boolean }, adminBase: string, siteOrigin: string): NoticeTarget {
  const href = link.href;
  if (link.external || /^https?:\/\//i.test(href)) return { kind: 'tab', href };
  const base = adminBase.endsWith('/') ? adminBase : adminBase + '/';
  if (base.length > 1 && href.startsWith(base)) return { kind: 'router', path: '/' + href.slice(base.length) };
  if (href === '/admin' || href.startsWith('/admin/')) return { kind: 'router', path: '/' + href.slice('/admin/'.length) };
  return { kind: 'tab', href: siteOrigin + href };
}

/** Widgets with a same-origin /ext-ui/code/ URL only (never another origin in the iframe). */
export function normalizeWidgets(raw: unknown): DashboardWidget[] {
  if (!Array.isArray(raw)) return [];
  const out: DashboardWidget[] = [];
  for (const x of raw as Record<string, unknown>[]) {
    if (!x || typeof x['id'] !== 'string' || typeof x['url'] !== 'string' || !/^\/ext-ui\/code\/[^/]+\/widget\//.test(x['url'])) continue;
    const h = Number(x['height']);
    out.push({
      id: x['id'], url: x['url'], icon: typeof x['icon'] === 'string' ? x['icon'] : '',
      label: (x['label'] as CodeLabel) ?? x['id'], height: Number.isFinite(h) && h >= 60 && h <= 2000 ? Math.round(h) : 220,
    });
  }
  return out;
}
