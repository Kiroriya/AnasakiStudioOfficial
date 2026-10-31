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
  },
  en: {
    title: 'Visitor statistics', desc: 'See how many people visit and which pages are most popular.', range: '{n} days', views: 'Views',
    visitors: 'Visitors', todayViews: 'Views today', todayVisitors: 'Visitors today', periodViews: 'Views, {n} days',
    periodVisitors: 'Visitors, {n} days', trend: 'Daily trend', topPages: 'Most viewed pages', page: 'Page',
    referrers: 'Top referrers', direct: 'Direct / unknown', noRef: 'No data yet', empty: 'No visits in this period yet',
    privacy: 'No cookies, no stored IPs — visitors are counted with a hash that rotates daily.', asTable: 'View as table',
    asChart: 'View as chart', date: 'Date',
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

const STYLE = `<style>
.an-seg{display:flex;gap:4px}.an-seg a{padding:6px 12px;border:1px solid var(--ak-border,#d1d5db);border-radius:8px;text-decoration:none;color:inherit}
.an-seg a.on{background:var(--ak-primary,#4f46e5);border-color:var(--ak-primary,#4f46e5);color:#fff}
.an-stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:12px;margin-bottom:12px}
.an-cols{display:grid;grid-template-columns:2fr 1fr;gap:12px}@media (max-width:800px){.an-cols{grid-template-columns:1fr}}
.an-rank{display:inline-flex;width:22px;height:22px;border-radius:50%;align-items:center;justify-content:center;font-size:12px;background:var(--ak-muted-bg,#f3f4f6)}
.an-rank.top{background:#fde68a;color:#78350f}.an-bar{height:4px;border-radius:2px;background:var(--ak-border,#e5e7eb);margin-top:4px}
.an-bar span{display:block;height:100%;border-radius:2px;background:#2a78d6}.an-list{list-style:none;margin:0;padding:0}
.an-list li{display:flex;justify-content:space-between;gap:8px;padding:6px 0;border-bottom:1px solid var(--ak-border,#e5e7eb)}
.an-path{font-family:ui-monospace,monospace;font-size:12px}.an-legend{display:flex;gap:16px;font-size:13px;margin:4px 0 8px}
</style>`;

/** The dashboard card: totals of the last 7 days and the daily views chart (admin UI kit). */
export function renderWidget(ctx: { locale?: string }): string {
  const en = ctx.locale === 'en';
  const L = en ? M.en : M.th;
  const a: Summary = summary(7);
  const chart = a.daily.map((d) => ({ label: dayLabel(d.day, en), value: d.views }));
  return `<style>.an-w{display:flex;gap:16px;margin-bottom:8px}.an-w div{display:flex;flex-direction:column}.an-w b{font-size:22px}</style>` +
    `<div class="an-w" id="an-widget"><div data-stat="views"><span class="ak-muted ak-small">${esc(L.views)}</span><b>${num(a.totals.views)}</b></div>` +
    `<div data-stat="visitors"><span class="ak-muted ak-small">${esc(L.visitors)}</span><b>${num(a.totals.visitors)}</b></div>` +
    `<div data-stat="todayViews"><span class="ak-muted ak-small">${esc(L.todayViews)}</span><b>${num(a.totals.todayViews)}</b></div></div>` +
    `<div data-ak-bar="${attr(chart)}" id="an-widget-chart"></div>`;
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
  const chartViews = a.daily.map((d) => ({ label: dayLabel(d.day, en), value: d.views }));
  const chartVisitors = a.daily.map((d) => ({ label: dayLabel(d.day, en), value: d.visitors }));
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
  const table = `<table class="ak-table" data-ak-sortable><thead><tr><th data-sort>${esc(t('date'))}</th><th data-sort class="ak-num">${esc(t('views'))}</th>` +
    `<th data-sort class="ak-num">${esc(t('visitors'))}</th></tr></thead><tbody>` +
    a.daily.map((d) => `<tr><td data-value="${d.day}">${esc(dayLabel(d.day, en))}</td><td class="ak-num">${d.views}</td><td class="ak-num">${d.visitors}</td></tr>`).join('') +
    `</tbody></table>`;
  const html = STYLE +
    `<header class="ak-page-head"><span class="ak-page-icon">📊</span><div class="ak-page-head-text"><h1>${esc(t('title'))}</h1><p>${esc(t('desc'))}</p></div>` +
    `<div class="ak-actions an-seg" id="an-range">${seg}</div></header>` +
    `<div class="an-stats">${stat('👁', a.totals.todayViews, t('todayViews'), 'todayViews')}${stat('🙋', a.totals.todayVisitors, t('todayVisitors'), 'todayVisitors')}` +
    `${stat('📈', a.totals.views, t('periodViews', days), 'views')}${stat('👥', a.totals.visitors, t('periodVisitors', days), 'visitors')}</div>` +
    `<div class="ak-card" id="an-trend"><div class="ak-card-head"><h2>${esc(t('trend'))}</h2>` +
    `<div class="ak-tabs" data-ak-tabs><button class="ak-tab" data-tab="chart">${esc(t('asChart'))}</button><button class="ak-tab" data-tab="table">${esc(t('asTable'))}</button></div></div>` +
    `<section data-tab-panel="chart"><div class="an-legend"><span>■ ${esc(t('views'))}</span></div><div data-ak-bar="${attr(chartViews)}" id="an-chart-views"></div>` +
    `<div class="an-legend"><span>■ ${esc(t('visitors'))}</span></div><div data-ak-bar="${attr(chartVisitors)}" id="an-chart-visitors"></div></section>` +
    `<section data-tab-panel="table" hidden>${table}</section></div>` +
    `<div class="an-cols"><div class="ak-card ak-card-flush"><div class="ak-card-head"><h2>🏆 ${esc(t('topPages'))}</h2><span class="ak-muted ak-small">${esc(t('range', days))}</span></div>` +
    `<div class="ak-table-wrap"><table class="ak-table" id="an-pages"><thead><tr><th>#</th><th>${esc(t('page'))}</th><th class="ak-num">${esc(t('views'))}</th>` +
    `<th class="ak-num">${esc(t('visitors'))}</th></tr></thead><tbody>${pages}</tbody></table></div></div>` +
    `<div class="ak-card"><div class="ak-card-head"><h2>🔗 ${esc(t('referrers'))}</h2></div><ul class="an-list" id="an-referrers">${refs}</ul>` +
    `<p class="ak-hint">🔒 ${esc(t('privacy'))}</p></div></div>`;
  return { title: t('title'), html };
}
