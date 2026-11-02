// analytics 2.0 — the built-in Go plugin as a JS plugin. Page views are counted after the response was sent
// (site.afterResponse: no cookies, no third-party script, never delays a page); visitors are a daily salted
// hash of IP + user agent (raw IPs are never stored). Admin page "Analytics" and the dashboard widget
// "Visitors, last 7 days" for the roles holding analytics.view (default: administrators and editors).
import { record } from './track';
import { summary } from './summary';
import { renderAdmin, renderWidget, WIDGET_HEIGHT } from './admin';
import { legacyImport } from './legacy';

// Who sees the statistics. CMS 2.0.6+: the plugin's own capability analytics.view (manifest "capabilities",
// default administrators and editors), ticked per role in Users → Roles. Older CMS versions: editors (the
// administrator always may).
const VIEW = typeof (site as any).can === 'function' ? ['cap:analytics.view'] : ['editor'];

// prefetch / prerender requests and internal referrers are told apart with ResponseInfo.headers / host
site.afterResponse((info) => record(info));

site.adminPage('stats', {
  label: { th: 'สถิติผู้เข้าชม', en: 'Analytics' },
  icon: '📈',
  access: VIEW,
  render: (ctx) => renderAdmin(ctx),
});

// the admin dashboard card (was the Angular AnalyticsWidget)
site.dashboardWidget('visitors', {
  label: { th: 'ผู้เข้าชม 7 วันล่าสุด', en: 'Visitors, last 7 days' },
  icon: '📊',
  access: VIEW,
  height: WIDGET_HEIGHT, // fits the card; CMS 2.0.3+ then sizes the frame to its content (kit auto-height)
  render: (ctx) => renderWidget(ctx),
});

/** The summary of the last 7 / 30 / 90 days (same JSON as the Go plugin's GET /api/v1/analytics/summary). */
site.export('summary', (days?: number) => summary(days));

/** Rows of the built-in plugin's tables (core migration). */
site.export('legacyImport', legacyImport);
