// analytics 2.0 — the built-in Go plugin as a JS plugin. Page views are counted after the response was sent
// (site.afterResponse: no cookies, no third-party script, never delays a page); visitors are a daily salted
// hash of IP + user agent (raw IPs are never stored). Admin page "Analytics" for administrators and editors
// (was capability view_stats) and the dashboard widget "Visitors, last 7 days".
import { record } from './track';
import { summary } from './summary';
import { renderAdmin, renderWidget, WIDGET_HEIGHT } from './admin';
import { legacyImport } from './legacy';

// prefetch / prerender requests and internal referrers are told apart with ResponseInfo.headers / host
site.afterResponse((info) => record(info));

site.adminPage('stats', {
  label: { th: 'สถิติผู้เข้าชม', en: 'Analytics' },
  icon: '📈',
  access: ['editor'],
  render: (ctx) => renderAdmin(ctx),
});

// the admin dashboard card (was the Angular AnalyticsWidget)
site.dashboardWidget('visitors', {
  label: { th: 'ผู้เข้าชม 7 วันล่าสุด', en: 'Visitors, last 7 days' },
  icon: '📊',
  access: ['editor'],
  height: WIDGET_HEIGHT, // initial: the kit sizes the frame to the card (KPIs, chart, top pages)
  render: (ctx) => renderWidget(ctx),
});

/** The summary of the last 7 / 30 / 90 days (same JSON as the Go plugin's GET /api/v1/analytics/summary). */
site.export('summary', (days?: number) => summary(days));

/** Rows of the built-in plugin's tables (core migration). */
site.export('legacyImport', legacyImport);
