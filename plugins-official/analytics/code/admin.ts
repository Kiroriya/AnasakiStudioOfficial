// The "Analytics" admin page (was the Angular page /analytics), built with the admin UI kit.
import { esc, num, one } from './lib';
import { rangeOf, summary, Summary } from './summary';

const M = {
  th: {
    title: 'สถิติผู้เข้าชม', desc: 'ดูว่ามีคนเข้าเว็บกี่คน และหน้าไหนได้รับความนิยมที่สุด', range: '{n} วัน', views: 'การเข้าชม',
    visitors: 'ผู้เข้าชม', todayViews: 'การเข้าชมวันนี้', todayVisitors: 'ผู้เข้าชมวันนี้', periodViews: 'การเข้าชม {n} วัน',
    periodVisitors: 'ผู้เข้าชม {n} วัน', trend: 'แนวโน้มรายวัน', topPages: 'หน้าที่เข้าชมมากที่สุด', page: 'หน้า',
    referrers: 'มาจากที่ไหน', direct: 'พิมพ์เข้ามาเอง / ไม่ทราบ', noRef: 'ยังไม่มีข้อมูล', empty: 'ยังไม่มีคนเข้าชมในช่วงนี้',
    privacy: 'ไม่ใช้คุกกี้ ไม่เก็บ IP — นับผู้เข้าชมด้วยรหัสที่เปลี่ยนทุกวัน', asTable: 'ดูเป็นตาราง', asChart: 'ดูเป็นกราฟ', date: 'วันที่',
    viewAll: 'ดูทั้งหมด →',
  },
  en: {
    title: 'Visitor statistics', desc: 'See how many people visit and which pages are most popular.', range: '{n} days', views: 'Views',
    visitors: 'Visitors', todayViews: 'Views today', todayVisitors: 'Visitors today', periodViews: 'Views, {n} days',
    periodVisitors: 'Visitors, {n} days', trend: 'Daily trend', topPages: 'Most viewed pages', page: 'Page',
    referrers: 'Top referrers', direct: 'Direct / unknown', noRef: 'No data yet', empty: 'No visits in this period yet',
    privacy: 'No cookies, no stored IPs — visitors are counted with a hash that rotates daily.', asTable: 'View as table',
    asChart: 'View as chart', date: 'Date', viewAll: 'View all →',
  },
};
type Key = keyof typeof M.th;

const MONTHS = {
  th: ['ม.ค.', 'ก.พ.', 'มี.ค.', 'เม.ย.', 'พ.ค.', 'มิ.ย.', 'ก.ค.', 'ส.ค.', 'ก.ย.', 'ต.ค.', 'พ.ย.', 'ธ.ค.'],
  en: ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'],
};

/** "31 ต.ค." / "Oct 31" (the Angular page used toLocaleDateString day + short month). */
function dayLabel(day: string, en: boolean): string {
  const m = Number(day.slice(5, 7)) - 1;
  const d = Number(day.slice(8, 10));
  return en ? MONTHS.en[m] + ' ' + d : d + ' ' + MONTHS.th[m];
}

const attr = (v: unknown) => esc(JSON.stringify(v));

/** Validated categorical slots 1–2 (blue, orange) — the colours of the former built-in charts. */
const VIEWS_COLOR = '#2a78d6';
const VISITORS_COLOR = '#eb6834';

/** data-ak-line options (AK.line of the admin UI kit, else assets/chart.js): views + visitors per day. */
function lineChart(a: Summary, en: boolean, L: typeof M.th, height: number): string {
  return attr({
    labels: a.daily.map((d) => dayLabel(d.day, en)),
    series: [
      { label: L.views, color: VIEWS_COLOR, values: a.daily.map((d) => d.views) },
      { label: L.visitors, color: VISITORS_COLOR, values: a.daily.map((d) => d.visitors) },
    ],
    height,
    locale: en ? 'en-US' : 'th-TH',
    tableLabel: L.asTable,
    chartLabel: L.asChart,
    dateLabel: L.date,
  });
}

/** The admin page of this plugin (site.adminPage 'stats') as an admin path. */
export const STATS_PATH = '/admin/x/code/analytics:stats';

/**
 * The package's chart script (assets/chart.js): on CMS 2.0.3+ the kit's AK.line draws [data-ak-line]; on older CMS
 * (no AK.line in the kit) chart.js draws it with its own copy of the chart and loads assets/chart.css. Deferred, so
 * it runs after the kit (deferred in <head>).
 */
function chartScript(): string {
  let u = '';
  try {
    u = site.asset('assets/chart.js').url;
  } catch {
    u = '';
  }
  return u ? `<script src="${esc(u)}" defer></script>` : '';
}

const STYLE = `<style>
.an-seg{display:flex;gap:4px}.an-seg a{padding:6px 12px;border:1px solid var(--ak-border,#d1d5db);border-radius:8px;text-decoration:none;color:inherit}
.an-seg a.on{background:var(--ak-primary,#4f46e5);border-color:var(--ak-primary,#4f46e5);color:#fff}
.an-stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:12px;margin-bottom:12px}
.an-cols{display:grid;grid-template-columns:2fr 1fr;gap:12px}@media (max-width:800px){.an-cols{grid-template-columns:1fr}}
.an-rank{display:inline-flex;flex-shrink:0;width:24px;height:24px;border-radius:50%;align-items:center;justify-content:center;font-size:12px;font-weight:700;background:var(--ak-surface-2);color:var(--ak-muted)}
.an-rank.top{background:var(--ak-primary-soft);color:var(--ak-primary)}.an-bar{height:4px;border-radius:2px;background:var(--ak-border,#e5e7eb);margin-top:4px}
.an-bar span{display:block;height:100%;border-radius:2px;background:#2a78d6}.an-list{list-style:none;margin:0;padding:0}
.an-list li{display:flex;justify-content:space-between;gap:8px;padding:6px 0;border-bottom:1px solid var(--ak-border,#e5e7eb)}
.an-path{font-family:ui-monospace,monospace;font-size:12px}
</style>`;

const WIDGET_STYLE = `<style>
.an-w-head{display:flex;justify-content:space-between;align-items:center;gap:8px;margin-bottom:10px;font-size:13px}
.an-w-grid{display:grid;grid-template-columns:minmax(0,1fr) 280px;gap:28px}
.an-kpis{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px;margin-bottom:12px}
.an-kpis b{display:block;font-size:22px;line-height:1.25;font-variant-numeric:tabular-nums}.an-kpis small{color:var(--ak-muted);font-size:12px}
.an-w h4{margin:0 0 4px;font-size:14px}.an-top{list-style:none;margin:0;padding:0}
.an-top li{display:flex;align-items:center;gap:10px;min-height:40px;border-bottom:1px solid var(--ak-border);font-size:13px}.an-top li:last-child{border-bottom:0}
.an-top .an-t{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.an-top b{font-variant-numeric:tabular-nums}
.an-rank{display:inline-flex;flex-shrink:0;width:24px;height:24px;border-radius:50%;align-items:center;justify-content:center;font-size:12px;font-weight:700;background:var(--ak-surface-2);color:var(--ak-muted)}
.an-rank.top{background:var(--ak-primary-soft);color:var(--ak-primary)}
@media (max-width:720px){.an-w-grid{grid-template-columns:minmax(0,1fr);gap:16px}}
@media (max-width:420px){.an-kpis{grid-template-columns:1fr 1fr}}
body.ak:has(>.ak-widget){padding:16px;background:var(--ak-surface)}

</style>`;

/**
 * Height of the dashboard card. CMS 2.0.3+ then sizes the frame to the content (kit auto-height); older CMS keep this
 * height, so it fits the card without a scrollbar there: 2 × 16px body padding (the widget surface rule of the newer
 * kit.css, repeated in WIDGET_STYLE for older kits) + head 30 + KPIs 58 + legend 27 + chart 150 + x labels 22 ≈ 321
 * (the top-5 list beside the chart is shorter), plus a little room.
 */
export const WIDGET_HEIGHT = 340;

/** The dashboard card "Visitors, last 7 days" — the former built-in widget: KPIs, views + visitors chart, top 5 pages. */
export function renderWidget(ctx: { locale?: string }): string {
  const en = ctx.locale === 'en';
  const L = en ? M.en : M.th;
  const t = (k: Key, n?: number) => L[k].replace('{n}', String(n ?? ''));
  const a: Summary = summary(7);
  const kpi = (id: string, v: number, label: string) => `<div data-stat="${id}"><b>${num(v)}</b><small>${esc(label)}</small></div>`;
  const first = a.daily[0]?.day ?? '';
  const last = a.daily[a.daily.length - 1]?.day ?? '';
  const top = a.topPages.slice(0, 5);
  const pages = top.length
    ? top.map((p, i) => `<li data-path="${esc(p.path)}"><span class="an-rank${i < 3 ? ' top' : ''}">${i + 1}</span>` +
        `<span class="an-t" title="${esc(p.path)}">${esc(p.title || p.path)}</span><b>${num(p.views)}</b></li>`).join('')
    : `<li class="ak-muted">${esc(t('empty'))}</li>`;
  return WIDGET_STYLE +
    `<div class="an-w" id="an-widget"><div class="an-w-head"><span class="ak-muted">${esc(first ? dayLabel(first, en) + ' – ' + dayLabel(last, en) : '')}</span>` +
    // CMS 2.0.3+: data-ak-open → AK.open (the admin opens its page); older kits ignore it and the link opens a new tab
    // (a sandboxed frame may not navigate the admin window)
    `<a href="${STATS_PATH}" id="an-widget-all" data-ak-open target="_blank" rel="noopener">${esc(t('viewAll'))}</a></div>` +
    `<div class="an-w-grid"><div><div class="an-kpis">${kpi('todayViews', a.totals.todayViews, t('todayViews'))}` +
    `${kpi('views', a.totals.views, t('periodViews', 7))}${kpi('visitors', a.totals.visitors, t('periodVisitors', 7))}</div>` +
    `<div id="an-widget-chart" data-ak-line="${lineChart(a, en, L, 150)}"></div></div>` +
    `<div><h4>🏆 ${esc(t('topPages'))}</h4><ol class="an-top" id="an-widget-top">${pages}</ol></div></div></div>` + chartScript();
}

export function renderAdmin(ctx: { query: Record<string, string | string[]>; locale?: string }): { title: string; html: string } {
  const en = ctx.locale === 'en';
  const L = en ? M.en : M.th;
  const t = (k: Key, n?: number) => L[k].replace('{n}', String(n ?? ''));
  const days = rangeOf(one(ctx.query?.days) || 30);
  const a: Summary = summary(days);
  const seg = [7, 30, 90].map((d) => `<a href="?days=${d}" class="${d === days ? 'on' : ''}" data-days="${d}">${esc(t('range', d))}</a>`).join('');
  const stat = (icon: string, v: number, label: string, id: string) =>
    `<div class="ak-card"><div class="ak-stat" data-stat="${id}"><span>${icon} ${esc(label)}</span><b>${num(v)}</b></div></div>`;
  const maxViews = a.topPages[0]?.views ?? 0;
  const pages = a.topPages.length
    ? a.topPages.map((p, i) => `<tr data-path="${esc(p.path)}"><td><span class="an-rank${i < 3 ? ' top' : ''}">${i + 1}</span></td>` +
        `<td><div>${esc(p.title || p.path)}</div><a class="an-path ak-muted" href="${esc(p.path)}" target="_blank" rel="noopener">${esc(p.path)}</a>` +
        `<div class="an-bar"><span style="width:${maxViews ? Math.max(2, (p.views / maxViews) * 100).toFixed(1) : 0}%"></span></div></td>` +
        `<td class="ak-num"><b>${num(p.views)}</b></td><td class="ak-num">${num(p.visitors)}</td></tr>`).join('')
    : `<tr><td colspan="4" class="ak-empty">📭 ${esc(t('empty'))}</td></tr>`;
  const refs = a.topReferrers.length
    ? a.topReferrers.map((r) => `<li data-host="${esc(r.host)}"><span>${esc(r.host || t('direct'))}</span><b class="ak-num">${num(r.views)}</b></li>`).join('')
    : `<li class="ak-muted">${esc(t('noRef'))}</li>`;
  const html = STYLE +
    `<header class="ak-page-head"><span class="ak-page-icon">📊</span><div class="ak-page-head-text"><h1>${esc(t('title'))}</h1><p>${esc(t('desc'))}</p></div>` +
    `<div class="ak-actions an-seg" id="an-range">${seg}</div></header>` +
    `<div class="an-stats">${stat('👁', a.totals.todayViews, t('todayViews'), 'todayViews')}${stat('🙋', a.totals.todayVisitors, t('todayVisitors'), 'todayVisitors')}` +
    `${stat('📈', a.totals.views, t('periodViews', days), 'views')}${stat('👥', a.totals.visitors, t('periodVisitors', days), 'visitors')}</div>` +
    `<div class="ak-card" id="an-trend"><div class="ak-card-head"><h2>${esc(t('trend'))}</h2></div>` +
    `<div id="an-chart" data-ak-line="${lineChart(a, en, L, 260)}"></div></div>` +
    `<div class="an-cols"><div class="ak-card ak-card-flush"><div class="ak-card-head"><h2>🏆 ${esc(t('topPages'))}</h2><span class="ak-muted ak-small">${esc(t('range', days))}</span></div>` +
    `<div class="ak-table-wrap"><table class="ak-table" id="an-pages"><thead><tr><th>#</th><th>${esc(t('page'))}</th><th class="ak-num">${esc(t('views'))}</th>` +
    `<th class="ak-num">${esc(t('visitors'))}</th></tr></thead><tbody>${pages}</tbody></table></div></div>` +
    `<div class="ak-card"><div class="ak-card-head"><h2>🔗 ${esc(t('referrers'))}</h2></div><ul class="an-list" id="an-referrers">${refs}</ul>` +
    `<p class="ak-hint">🔒 ${esc(t('privacy'))}</p></div></div>` + chartScript();
  return { title: t('title'), html };
}
