// Pure helpers of the analytics plugin, ported 1:1 from the built-in Go plugin (internal/plugins/analytics).

const botRe = new RegExp(
  'bot\\b|bot/|crawl|spider|slurp|archiver|facebookexternalhit|embedly|preview|headless|phantomjs|' +
    'lighthouse|pagespeed|gtmetrix|pingdom|uptime|monitor|curl/|wget/|python|java/|go-http-client|okhttp|axios|node-fetch|' +
    'httpclient|libwww|scrapy|semrush|ahrefs|mj12|petalbot|yandex|baiduspider|whatsapp|telegram|discord|slack|' +
    'googleother|feedfetcher|validator|bingpreview|postman|insomnia',
  'i',
);

/** A crawler / tool user agent (empty or shorter than 12 characters counts as a bot). */
export function isBot(ua: string): boolean {
  ua = String(ua ?? '').trim();
  return ua === '' || ua.length < 12 || botRe.test(ua);
}

const skipPrefixes = ['/api/', '/uploads/', '/admin', '/plugin-assets/', '/download/', '/forms/'];

/** Extension of the last path segment ("" when none), like Go's path.Ext. */
export function pathExt(p: string): string {
  const seg = p.slice(p.lastIndexOf('/') + 1);
  const i = seg.lastIndexOf('.');
  return i >= 0 ? seg.slice(i) : '';
}

/** A request that may count as a page view (method, path and user agent; prefetches are checked apart). */
export function trackable(method: string, path: string, ua: string): boolean {
  if (method !== 'GET') return false;
  for (const s of skipPrefixes) if (path.startsWith(s)) return false;
  const ext = pathExt(path).toLowerCase();
  if (ext !== '' && ext !== '.html') return false;
  return !isBot(ua);
}

/** A prefetch / prerender request (Purpose, Sec-Purpose, X-Moz headers). */
export function isPrefetch(headers: Record<string, string>): boolean {
  const h = headers ?? {};
  return h['purpose'] === 'prefetch' || (h['sec-purpose'] ?? '') !== '' || h['x-moz'] === 'prefetch';
}

/** Host name of an absolute URL, lower-case, without port / user info ("" when relative or invalid). */
export function hostOf(u: string): string {
  const m = /^[a-z][a-z0-9+.-]*:\/\/([^/?#]*)/i.exec(String(u ?? '').trim());
  if (!m) return '';
  let a = m[1];
  const at = a.lastIndexOf('@');
  if (at >= 0) a = a.slice(at + 1);
  if (a.startsWith('[')) {
    const end = a.indexOf(']');
    return end > 0 ? a.slice(1, end).toLowerCase() : '';
  }
  const c = a.indexOf(':');
  if (c >= 0) a = a.slice(0, c);
  return a.toLowerCase();
}

/** Cuts s to at most n characters (code points) without splitting a UTF-8 sequence. */
export function clipRunes(s: string, n: number): string {
  if (s.length <= n) return s;
  const r = Array.from(s);
  return r.length > n ? r.slice(0, n).join('') : s;
}

/** The external referrer host ("" for direct / internal; "www." removed). */
export function refHost(referrer: string, siteHosts: string[]): string {
  if (!referrer) return '';
  let host = hostOf(referrer);
  if (host === '') return '';
  if (siteHosts.indexOf(host) >= 0) return '';
  if (host.startsWith('www.')) host = host.slice(4);
  return clipRunes(host, 191);
}

/** UTF-8 byte length. */
export function byteLen(s: string): number {
  let n = 0;
  for (const ch of s) {
    const c = ch.codePointAt(0) as number;
    n += c < 0x80 ? 1 : c < 0x800 ? 2 : c < 0x10000 ? 3 : 4;
  }
  return n;
}

/** The anonymous daily visitor id: sha256(salt|ip|ua), first 16 hex characters. */
export function visitorHash(salt: string, ip: string, ua: string): string {
  return crypto.hash('sha256', salt + '|' + ip + '|' + ua).slice(0, 16);
}

const pad = (n: number) => (n < 10 ? '0' : '') + n;

/** YYYY-MM-DD of a date in the server's local time (like the Go plugin's time.Now().Format). */
export function dayStr(d: Date): string {
  return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate());
}

/** Today minus n days (local). */
export function daysAgo(n: number, now = new Date()): string {
  return dayStr(new Date(now.getFullYear(), now.getMonth(), now.getDate() - n));
}

/** A DATE value from the database or a legacy row ("2026-10-01", RFC 3339, Date) → "YYYY-MM-DD" or "". */
export function normDay(v: unknown): string {
  if (v instanceof Date) return isNaN(v.getTime()) ? '' : v.toISOString().slice(0, 10);
  const s = String(v ?? '').trim();
  return /^\d{4}-\d{2}-\d{2}/.test(s) ? s.slice(0, 10) : '';
}

export const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);

export const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? '';

/** 1234567 → "1,234,567" (goja has no locale-aware toLocaleString). */
export function num(n: number): string {
  const s = String(Math.round(Number(n) || 0));
  const neg = s.startsWith('-');
  const d = neg ? s.slice(1) : s;
  return (neg ? '-' : '') + d.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}
