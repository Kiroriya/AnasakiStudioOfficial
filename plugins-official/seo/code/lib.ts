// Pure helpers of the SEO plugin (ported 1:1 from the built-in Go plugin, internal/plugins/seo).

export const DEFAULT_TEMPLATE = '%title% – %site%';

/**
 * Expands %title% %site% %tagline% (one pass, like Go's strings.NewReplacer); a page without a title
 * (or titled like the site) shows the site title alone. Leading/trailing separators are trimmed.
 */
export function applyTemplate(tpl: string, title: string, siteTitle: string, tagline: string): string {
  if (title === '' || title === siteTitle) return siteTitle === '' ? title : siteTitle;
  if (String(tpl ?? '').trim() === '') tpl = DEFAULT_TEMPLATE;
  const map: Record<string, string> = { '%title%': title, '%site%': siteTitle, '%tagline%': tagline };
  let out = tpl.replace(/%title%|%site%|%tagline%/g, (m) => map[m]);
  out = out.trim().replace(/^[–\-|·]+|[–\-|·]+$/g, '').trim();
  return out === '' ? title : out;
}

/** Same rule as the CMS (plugin.SafeURL): http(s)://…, /path, #anchor, mailto:, tel: — never javascript:. */
export function safeURL(u: string): boolean {
  const l = String(u ?? '').trim().toLowerCase();
  if (/[\x00\r\n\t<>"]/.test(l)) return false;
  if (l.startsWith('https://') || l.startsWith('http://')) return l.length > 8;
  if (l.startsWith('//')) return false;
  return l.startsWith('/') || l.startsWith('#') || l.startsWith('mailto:') || l.startsWith('tel:');
}

/** UTF-8 byte length (the Go plugin's limits count bytes). */
export function byteLen(s: string): number {
  let n = 0;
  for (const ch of s) {
    const c = ch.codePointAt(0) as number;
    n += c < 0x80 ? 1 : c < 0x800 ? 2 : c < 0x10000 ? 3 : 4;
  }
  return n;
}

export const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);

export const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? '';

/** A DATETIME value from a legacy row (Date, RFC 3339 or "YYYY-MM-DD HH:MM:SS" UTC); now when missing/invalid. */
export function toDate(v: unknown): Date {
  if (v instanceof Date && !isNaN(v.getTime())) return v;
  if (typeof v === 'string' && v.trim() !== '') {
    let s = v.trim();
    if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}/.test(s)) s = s.replace(' ', 'T');
    if (/^\d{4}-\d{2}-\d{2}T[\d:.]+$/.test(s)) s += 'Z';
    const d = new Date(s);
    if (!isNaN(d.getTime())) return d;
  }
  if (typeof v === 'number' && isFinite(v)) return new Date(v > 1e12 ? v : v * 1000);
  return new Date();
}

export const truthy = (v: unknown) => v === true || v === 1 || v === '1' || v === 'true';
