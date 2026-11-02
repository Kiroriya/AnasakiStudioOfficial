// Pure helpers of the docs plugin (no site.* / db.* calls): escaping, products and redirect settings,
// heading ids + table of contents, ordering of sections and docs, dates.

export type Lang = 'th' | 'en';
export type I18n = string | { th?: string; en?: string };

export interface Product {
  id: string;
  name: I18n;
  desc: I18n;
  icon: string;
}

/** A published doc as the public pages use it (the index keeps no content). */
export interface Doc {
  id: number;
  slug: string;
  title: string;
  excerpt: string;
  product: string;
  section: string;
  order: number;
  sectionOrder: number;
  updatedAt: string;
  headings: Heading[];
}

export interface Heading {
  level: 2 | 3;
  id: string;
  text: string;
}

export interface Section {
  name: string;
  docs: Doc[];
}

export const BASE = '/docs';

// Until the site sets its own products (settings or an imported bundle) every article lives in one generic
// group — nothing about any particular site or product is built into the plugin.
export const DEFAULT_PRODUCTS: Product[] = [
  { id: 'general', name: { th: 'เอกสาร', en: 'Documentation' }, icon: '📘',
    desc: { th: 'คู่มือและบทความทั้งหมด', en: 'All guides and articles' } },
];

export const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);

/** JSON inside <script type="application/json"> (no "</script>" breakout). */
export const jsonData = (v: unknown) =>
  JSON.stringify(v).replace(/</g, '\\u003c').replace(/>/g, '\\u003e').replace(/&/g, '\\u0026');

export const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? '';

export function tr(v: I18n | undefined, lang: Lang): string {
  if (v == null) return '';
  if (typeof v === 'string') return v;
  return String(v[lang] || v.th || v.en || '');
}

const idRe = /^[a-z0-9][a-z0-9-]{0,39}$/;
export const SLUG_RE = /^[a-z0-9][a-z0-9-]{0,190}$/;

function cleanI18n(v: any, max: number): I18n {
  const clip = (s: any) => String(s ?? '').trim().slice(0, max);
  if (typeof v === 'string') return clip(v);
  if (v && typeof v === 'object') {
    const out: { th?: string; en?: string } = {};
    if (v.th) out.th = clip(v.th);
    if (v.en) out.en = clip(v.en);
    return out;
  }
  return '';
}

/**
 * Validates the `products` setting (JSON array). Returns the cleaned list or an error message (Thai).
 * Empty input → the defaults.
 */
export function parseProducts(raw: unknown): { products: Product[]; error?: string } {
  if (raw == null || (typeof raw === 'string' && raw.trim() === '')) return { products: DEFAULT_PRODUCTS };
  let list: any = raw;
  if (typeof raw === 'string') {
    try {
      list = JSON.parse(raw);
    } catch {
      return { products: DEFAULT_PRODUCTS, error: 'products: JSON ไม่ถูกต้อง' };
    }
  }
  if (!Array.isArray(list) || list.length === 0) return { products: DEFAULT_PRODUCTS, error: 'products ต้องเป็น array ที่มีอย่างน้อยหนึ่งรายการ' };
  if (list.length > 20) return { products: DEFAULT_PRODUCTS, error: 'มีผลิตภัณฑ์ได้ไม่เกิน 20 รายการ' };
  const out: Product[] = [];
  const seen = new Set<string>();
  for (const p of list) {
    const id = String(p?.id ?? '').trim();
    if (!idRe.test(id) || id === 'search.json' || seen.has(id)) {
      return { products: DEFAULT_PRODUCTS, error: `รหัสผลิตภัณฑ์ "${id}" ไม่ถูกต้องหรือซ้ำ (ใช้ a-z 0-9 -)` };
    }
    seen.add(id);
    const name = cleanI18n(p.name, 80);
    if (!tr(name, 'th') && !tr(name, 'en')) return { products: DEFAULT_PRODUCTS, error: `ผลิตภัณฑ์ "${id}" ต้องมีชื่อ (name)` };
    out.push({ id, name, desc: cleanI18n(p.desc, 300), icon: String(p.icon ?? '').trim().slice(0, 16) });
  }
  return { products: out };
}

export interface Redirect {
  from: string;
  to: string;
}

/** Path normalisation for redirect matching: trailing slashes dropped, "/" kept. */
export const normPath = (p: string) => {
  const s = String(p ?? '').trim();
  return s.length > 1 ? s.replace(/\/+$/, '') : s;
};

const pathOK = (p: string) => /^\/[^\s<>"\\]*$/.test(p) && !p.startsWith('//');

/**
 * The `legacyRedirects` setting: a JSON array [{from, to}] or lines "from to" / "from -> to". Bad entries are
 * reported, never thrown.
 */
export function parseRedirects(raw: unknown): { items: Redirect[]; errors: string[] } {
  const items: Redirect[] = [];
  const errors: string[] = [];
  if (raw == null) return { items, errors };
  let list: any[] = [];
  if (Array.isArray(raw)) list = raw;
  else {
    const s = String(raw).trim();
    if (!s) return { items, errors };
    if (s.startsWith('[')) {
      try {
        list = JSON.parse(s);
      } catch {
        return { items, errors: ['legacyRedirects: JSON ไม่ถูกต้อง'] };
      }
    } else {
      for (const line of s.split(/\r?\n/)) {
        const t = line.trim();
        if (!t || t.startsWith('#')) continue;
        const parts = t.split(/\s*(?:->|→|\s)\s*/).filter(Boolean);
        list.push({ from: parts[0], to: parts[1] });
      }
    }
  }
  const seen = new Set<string>();
  for (const r of list.slice(0, 500)) {
    const from = normPath(r?.from), to = String(r?.to ?? '').trim();
    if (!pathOK(from) || !(pathOK(to) || /^https:\/\/[^\s<>"]+$/.test(to))) {
      errors.push(`${from || '?'} → ${to || '?'}`);
      continue;
    }
    if (seen.has(from) || normPath(to) === from) continue;
    seen.add(from);
    items.push({ from, to });
  }
  return { items, errors };
}

// ---------------------------------------------------------------- headings and TOC

/** Plain text of an HTML fragment (tags dropped, common entities decoded, spaces collapsed). */
export function textOf(html: string): string {
  return String(html ?? '')
    .replace(/<[^>]*>/g, ' ')
    .replace(/&nbsp;/g, ' ')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#0?39;/g, "'")
    .replace(/&amp;/g, '&')
    .replace(/\s+/g, ' ')
    .trim();
}

/** A heading id: lower-case words joined by "-", Thai kept, ASCII punctuation dropped. */
export function slugify(text: string): string {
  const s = text
    .toLowerCase()
    .replace(/[\u0000-\u002c\u002e\u002f\u003a-\u0040\u005b-\u005e\u0060\u007b-\u007f\u2013\u2014\u2018\u2019\u201c\u201d\u2026]+/g, ' ')
    .trim()
    .replace(/\s+/g, '-');
  return s.slice(0, 60).replace(/-+$/, '');
}

/**
 * Gives every <h2>/<h3> of the content an id (its own one is kept) and returns the new HTML and the headings.
 * Ids are unique within the document; "h-<n>" when the text has no usable characters.
 */
export function withHeadingIds(html: string): { html: string; headings: Heading[] } {
  const headings: Heading[] = [];
  const used = new Set<string>();
  let n = 0;
  const out = String(html ?? '').replace(/<h([23])(\s[^>]*)?>([\s\S]*?)<\/h\1>/gi, (_m, lv: string, attrs: string | undefined, inner: string) => {
    n++;
    const a = attrs ?? '';
    const own = /\sid="([^"]+)"/.exec(a);
    const text = textOf(inner);
    let id = own ? own[1] : slugify(text) || 'h-' + n;
    if (!own) {
      const base = id;
      for (let i = 2; used.has(id); i++) id = base + '-' + i;
    }
    used.add(id);
    headings.push({ level: (lv === '2' ? 2 : 3) as 2 | 3, id, text });
    const attrOut = own ? a : ` id="${esc(id)}"` + a;
    return `<h${lv}${attrOut}>${inner}<a class="dx-anchor" href="#${esc(id)}" aria-label="ลิงก์ไปหัวข้อนี้">#</a></h${lv}>`;
  });
  return { html: out, headings };
}

/** Content HTML for the article: heading ids, tables in a scroll wrapper, external links opened safely. */
export function prepareContent(html: string): { html: string; headings: Heading[] } {
  const r = withHeadingIds(html);
  let out = r.html.replace(/<table([\s>])/gi, '<div class="dx-table"><table$1').replace(/<\/table>/gi, '</table></div>');
  out = out.replace(/<a\s([^>]*href="https?:\/\/[^"]*"[^>]*)>/gi, (m, attrs: string) =>
    /\starget=/.test(' ' + attrs) ? m : `<a ${attrs} target="_blank" rel="noopener">`,
  );
  return { html: out, headings: r.headings };
}

// ---------------------------------------------------------------- ordering

const num = (v: unknown, def: number) => {
  const n = Number(v);
  return v === '' || v == null || !isFinite(n) ? def : n;
};

export function docFromPost(p: any): Doc {
  const m = p?.meta ?? {};
  const pick = (k: string) => m[k];
  return {
    id: Number(p.id),
    slug: String(p.slug ?? ''),
    title: String(p.title ?? ''),
    excerpt: String(p.excerpt ?? ''),
    product: String(pick('product') ?? '').trim(),
    section: String(pick('section') ?? '').trim(),
    order: num(pick('order'), 1000),
    sectionOrder: num(pick('sectionOrder'), NaN),
    updatedAt: String(p.updatedAt ?? ''),
    headings: withHeadingIds(String(p.content ?? '')).headings,
  };
}

/**
 * The docs of one product grouped into sections: sections by their smallest sectionOrder (unset = after the
 * numbered ones, in order of their first doc), docs by order then title.
 */
export function sectionsOf(docs: Doc[], product: string, noSection: string): Section[] {
  const mine = docs.filter((d) => d.product === product);
  mine.sort((a, b) => a.order - b.order || a.title.localeCompare(b.title) || a.id - b.id);
  const map = new Map<string, { s: Section; so: number; first: number }>();
  mine.forEach((d, i) => {
    const name = d.section || noSection;
    let g = map.get(name);
    if (!g) {
      g = { s: { name, docs: [] }, so: Infinity, first: i };
      map.set(name, g);
    }
    g.s.docs.push(d);
    if (isFinite(d.sectionOrder) && d.sectionOrder < g.so) g.so = d.sectionOrder;
  });
  return [...map.values()].sort((a, b) => a.so - b.so || a.first - b.first).map((g) => g.s);
}

/** Docs of a product in reading order (sidebar order) — prev/next follow it. */
export const readingOrder = (sections: Section[]) => sections.flatMap((s) => s.docs);

// ---------------------------------------------------------------- dates

const TH_MONTHS = ['ม.ค.', 'ก.พ.', 'มี.ค.', 'เม.ย.', 'พ.ค.', 'มิ.ย.', 'ก.ค.', 'ส.ค.', 'ก.ย.', 'ต.ค.', 'พ.ย.', 'ธ.ค.'];
const EN_MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/** "1 พ.ย. 2569" / "Nov 1, 2026" of an RFC 3339 time, shifted by the site's UTC offset (hours). */
export function formatDate(iso: string, lang: Lang, offsetHours = 7): string {
  const t = Date.parse(iso);
  if (!isFinite(t)) return '';
  const d = new Date(t + offsetHours * 3600_000);
  const day = d.getUTCDate(), mon = d.getUTCMonth(), y = d.getUTCFullYear();
  return lang === 'th' ? `${day} ${TH_MONTHS[mon]} ${y + 543}` : `${EN_MONTHS[mon]} ${day}, ${y}`;
}

/** UTC offset of common site timezones (no Intl in the runtime); 0 when unknown. */
export function tzOffset(tz: string): number {
  const m: Record<string, number> = {
    'Asia/Bangkok': 7, 'Asia/Ho_Chi_Minh': 7, 'Asia/Jakarta': 7, 'Asia/Vientiane': 7, 'Asia/Phnom_Penh': 7,
    'Asia/Singapore': 8, 'Asia/Kuala_Lumpur': 8, 'Asia/Manila': 8, 'Asia/Shanghai': 8, 'Asia/Hong_Kong': 8,
    'Asia/Tokyo': 9, 'Asia/Seoul': 9, 'Asia/Yangon': 6.5, 'Asia/Kolkata': 5.5, 'UTC': 0, 'Etc/UTC': 0,
  };
  return m[tz] ?? 0;
}

/** Minutes to read (Thai has no spaces: ~ 600 characters a minute). */
export function readMinutes(html: string): number {
  return Math.max(1, Math.round(textOf(html).length / 600));
}

// ---------------------------------------------------------------- Markdown (/docs/<product>/<slug>.md, llms.txt)

const decode = (s: string) =>
  s.replace(/&nbsp;/g, ' ').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#0?39;/g, "'").replace(/&amp;/g, '&');

/**
 * Markdown of article HTML (what the rich editor produces): headings, paragraphs, links, images, bold/italic,
 * inline code, fenced code blocks with their language, lists (one level of nesting), tables, quotes, rules.
 * Anything else keeps its text. Meant for reading and for AI tools, not for a round trip.
 */
export function htmlToMarkdown(html: string): string {
  const blocks: string[] = [];
  const keep = (md: string) => `\n\n\u0000${blocks.push(md) - 1}\u0000\n\n`;
  let s = String(html ?? '').replace(/\r\n?/g, '\n');
  s = s.replace(/<(script|style)[\s\S]*?<\/\1>/gi, '');
  // code blocks first: their text stays as written
  s = s.replace(/<pre([^>]*)>([\s\S]*?)<\/pre>/gi, (_m, attrs: string, inner: string) => {
    const lang = /(?:language|lang)-([\w+#.-]+)/.exec(inner + ' ' + attrs)?.[1] ?? '';
    const code = decode(inner.replace(/<br\s*\/?>/gi, '\n').replace(/<[^>]*>/g, '')).replace(/\n+$/, '');
    const fence = code.includes('```') ? '````' : '```';
    return keep(`${fence}${lang}\n${code}\n${fence}`);
  });
  s = s.replace(/<table[^>]*>([\s\S]*?)<\/table>/gi, (_m, inner: string) => {
    const rows = [...inner.matchAll(/<tr[^>]*>([\s\S]*?)<\/tr>/gi)].map((r) =>
      [...r[1].matchAll(/<t[hd][^>]*>([\s\S]*?)<\/t[hd]>/gi)].map((c) => inlineMd(c[1]).replace(/\|/g, '\\|').replace(/\n+/g, ' ')),
    );
    if (!rows.length) return '';
    const n = Math.max(...rows.map((r) => r.length));
    const line = (r: string[]) => '| ' + Array.from({ length: n }, (_, i) => r[i] ?? '').join(' | ') + ' |';
    return keep([line(rows[0]), '|' + ' --- |'.repeat(n), ...rows.slice(1).map(line)].join('\n'));
  });
  s = s.replace(/<h([1-6])[^>]*>([\s\S]*?)<\/h\1>/gi, (_m, lv: string, inner: string) => keep('#'.repeat(Number(lv)) + ' ' + inlineMd(inner)));
  s = s.replace(/<blockquote[^>]*>([\s\S]*?)<\/blockquote>/gi, (_m, inner: string) =>
    keep(blockMd(inner).split('\n').map((l) => (l ? '> ' + l : '>')).join('\n')));
  s = s.replace(/<div[^>]*class="[^"]*\bdx-(note|tip|warn|warning|danger)\b[^"]*"[^>]*>([\s\S]*?)<\/div>/gi, (_m, kind: string, inner: string) => {
    const tag = { note: 'NOTE', tip: 'TIP', warn: 'WARNING', warning: 'WARNING', danger: 'CAUTION' }[kind.toLowerCase()] ?? 'NOTE';
    return keep(`> [!${tag}]\n` + blockMd(inner).split('\n').map((l) => (l ? '> ' + l : '>')).join('\n'));
  });
  s = listsMd(s, keep);
  s = s.replace(/<hr[^>]*>/gi, () => keep('---'));
  s = s.replace(/<\/?(p|div|section|figure|figcaption|details|summary)[^>]*>/gi, '\n\n');
  s = inlineMd(s, true);
  s = s.replace(/\u0000(\d+)\u0000/g, (_m, i: string) => blocks[Number(i)]);
  return s.split('\n').map((l) => l.replace(/[ \t]+$/, '')).join('\n').replace(/\n{3,}/g, '\n\n').trim() + '\n';

  function blockMd(inner: string): string {
    return inlineMd(listsMd(inner.replace(/<\/?(p|div)[^>]*>/gi, '\n\n'), (md) => '\n' + md + '\n'), true).replace(/\n{3,}/g, '\n\n').trim();
  }
}

function listsMd(s: string, keep: (md: string) => string): string {
  // innermost lists first, so a nested list becomes indented lines of its parent item
  for (let guard = 0; guard < 20; guard++) {
    const next = s.replace(/<(ul|ol)[^>]*>((?:(?!<(?:ul|ol)[\s>])[\s\S])*?)<\/\1>/i, (_m, kind: string, inner: string) => {
      let n = 0;
      const items = [...inner.matchAll(/<li[^>]*>([\s\S]*?)<\/li>/gi)].map((li) => {
        const body = inlineMd(li[1].replace(/<\/?p[^>]*>/gi, '\n'), true).trim();
        const [first, ...rest] = body.split('\n').filter((l) => l.trim() !== '');
        const mark = kind.toLowerCase() === 'ol' ? `${++n}.` : '-';
        return [`${mark} ${first ?? ''}`, ...rest.map((l) => '   ' + l)].join('\n');
      });
      return '\n' + items.join('\n') + '\n';
    });
    if (next === s) break;
    s = next;
  }
  return s.replace(/((?:^|\n)(?:[-]|\d+\.) [^\n]*(?:\n(?:   |[-] |\d+\. )[^\n]*)*)/g, (m) => (m.trim() ? keep(m.trim()) : m));
}

function inlineMd(html: string, multiline = false): string {
  let s = html;
  s = s.replace(/<br\s*\/?>/gi, multiline ? '\\\n' : ' '); // a backslash break survives trimmed line ends
  s = s.replace(/<code[^>]*>([\s\S]*?)<\/code>/gi, (_m, c: string) => {
    const t = decode(c.replace(/<[^>]*>/g, ''));
    return t.includes('`') ? '`` ' + t + ' ``' : '`' + t + '`';
  });
  s = s.replace(/<img[^>]*>/gi, (m) => {
    const src = /\ssrc="([^"]*)"/i.exec(m)?.[1] ?? '';
    const alt = /\salt="([^"]*)"/i.exec(m)?.[1] ?? '';
    return src ? `![${decode(alt)}](${src})` : '';
  });
  s = s.replace(/<a\s[^>]*href="([^"]*)"[^>]*>([\s\S]*?)<\/a>/gi, (_m, href: string, inner: string) => {
    const t = inlineMd(inner);
    if (!t || t === '#' || href.startsWith('#') && /dx-anchor/.test(_m)) return '';
    return `[${t}](${decode(href)})`;
  });
  s = s.replace(/<(strong|b)(\s[^>]*)?>([\s\S]*?)<\/\1>/gi, (_m, _t, _a, inner: string) => (inner.trim() ? `**${inner.trim()}**` : ''));
  s = s.replace(/<(em|i)(\s[^>]*)?>([\s\S]*?)<\/\1>/gi, (_m, _t, _a, inner: string) => (inner.trim() ? `*${inner.trim()}*` : ''));
  s = s.replace(/<(s|del|strike)(\s[^>]*)?>([\s\S]*?)<\/\1>/gi, (_m, _t, _a, inner: string) => `~~${inner}~~`);
  s = s.replace(/<[^>\u0000]*>/g, '');
  s = decode(s);
  if (!multiline) s = s.replace(/\s+/g, ' ').trim();
  else s = s.replace(/[ \t]+/g, ' ').replace(/ *\n */g, '\n');
  return s;
}

// ---------------------------------------------------------------- page options

/**
 * The "Edit this page" link of an article from the `editUrl` setting — a template with {id}, {slug} and {product}
 * (e.g. a GitHub edit URL). https or a site path only; empty when unset or invalid.
 */
export function editHref(template: unknown, d: { id: number; slug: string; product: string }): string {
  const t = String(template ?? '').trim();
  if (!t || !(/^https:\/\/[^\s<>"]+$/.test(t) || /^\/(?!\/)[^\s<>"]*$/.test(t))) return '';
  return t.replace(/\{id\}/g, String(d.id)).replace(/\{slug\}/g, encodeURIComponent(d.slug)).replace(/\{product\}/g, encodeURIComponent(d.product));
}
