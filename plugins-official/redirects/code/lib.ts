// Pure helpers of the redirects plugin (ported 1:1 from the built-in Go plugin, internal/plugins/redirects).

export interface Rule {
  id?: number;
  from: string;
  to: string;
  code?: number;
}

/** Lookup key of a path: no trailing slash (except "/"). */
export function normalize(path: string): string {
  if (path.length > 1) {
    path = path.replace(/\/+$/, '');
    if (path === '') path = '/';
  }
  return path;
}

/** The redirect location, carrying over the query string when the rule's target has none. */
export function target(to: string, rawQuery: string): string {
  if (rawQuery && !to.includes('?')) {
    const i = to.indexOf('#');
    return i >= 0 ? to.slice(0, i) + '?' + rawQuery + to.slice(i) : to + '?' + rawQuery;
  }
  return to;
}

/** The site path a redirect target leads to ("/b" for "/b?x=1#y"), or "" for another site. */
export function localTarget(to: string): string {
  if (!to.startsWith('/') || to.startsWith('//')) return '';
  const i = to.search(/[?#]/);
  return normalize(i >= 0 ? to.slice(0, i) : to);
}

/**
 * The chain of paths when saving rule (replacing the rule with the same id among existing) would make
 * visitors go round in circles (A → B → A, or longer); null when it leads somewhere.
 */
export function findLoop(existing: Rule[], rule: Rule): string[] | null {
  const next = new Map<string, string>();
  for (const r of existing) {
    if (r.id && r.id === rule.id) continue;
    const to = localTarget(r.to);
    if (to) next.set(normalize(r.from), to);
  }
  const start = normalize(rule.from);
  const to = localTarget(rule.to);
  if (to) next.set(start, to);
  const chain = [start];
  const seen = new Set([start]);
  for (let cur = start; ; ) {
    const n = next.get(cur);
    if (n === undefined) return null;
    chain.push(n);
    if (seen.has(n)) return chain;
    seen.add(n);
    cur = n;
  }
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

export const CODES = [301, 302, 307, 308];

/** The system's own paths: never redirected, and their 404s are not broken links of the site (API calls, missing uploads). */
export const SYSTEM_PATHS = ['/api/', '/uploads/', '/plugin-assets/', '/ext-ui/'];
export const isSystemPath = (path: string) => SYSTEM_PATHS.some((p) => (String(path ?? '') + '/').startsWith(p));

/** Normalises a rule in place; a Thai error message when it is invalid (same messages as the built-in). */
export function validate(r: Rule): string | null {
  r.from = String(r.from ?? '').trim();
  r.to = String(r.to ?? '').trim();
  const q = r.from.indexOf('?');
  if (q >= 0) r.from = r.from.slice(0, q);
  if (!r.from.startsWith('/') || r.from.startsWith('//') || byteLen(r.from) > 191 || r.from === '/') {
    return 'เส้นทางต้นทางต้องขึ้นต้นด้วย / เช่น /old-page';
  }
  if (isSystemPath(r.from)) return 'ไม่สามารถ redirect เส้นทางของระบบได้';
  if (r.to === '' || byteLen(r.to) > 1000 || !safeURL(r.to)) return 'ปลายทางต้องเป็นลิงก์ http(s):// หรือเส้นทางที่ขึ้นต้นด้วย /';
  if (normalize(r.to) === normalize(r.from)) return 'ปลายทางต้องไม่ซ้ำกับต้นทาง';
  if (!r.code) r.code = 301;
  if (!CODES.includes(r.code)) return 'รหัส redirect ต้องเป็น 301, 302, 307 หรือ 308';
  r.from = normalize(r.from);
  return null;
}

/** English versions of the validation messages (admin pages in English). */
export const EN: Record<string, string> = {
  'เส้นทางต้นทางต้องขึ้นต้นด้วย / เช่น /old-page': 'The old path must start with /, e.g. /old-page',
  'ไม่สามารถ redirect เส้นทางของระบบได้': 'System paths cannot be redirected',
  'ปลายทางต้องเป็นลิงก์ http(s):// หรือเส้นทางที่ขึ้นต้นด้วย /': 'The target must be an http(s):// link or a path starting with /',
  'ปลายทางต้องไม่ซ้ำกับต้นทาง': 'The target must differ from the old path',
  'รหัส redirect ต้องเป็น 301, 302, 307 หรือ 308': 'The redirect type must be 301, 302, 307 or 308',
  'มี redirect สำหรับเส้นทางนี้อยู่แล้ว': 'A redirect for this path already exists',
  'ไม่พบ redirect นี้': 'Redirect not found',
  'redirect นี้ทำให้เกิดการวนซ้ำไม่รู้จบ: ': 'This redirect makes an endless loop: ',
};

/** At most n characters, never splitting a UTF-16 surrogate pair (VARCHAR(n) counts characters). */
export function clipRunes(s: string, n: number): string {
  const a = Array.from(s);
  return a.length > n ? a.slice(0, n).join('') : s;
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
