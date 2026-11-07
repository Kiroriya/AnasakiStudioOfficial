// Data of the docs plugin: settings (products, hub texts, redirects) and the index of published docs (cached,
// dropped when a doc is saved or deleted).
import { DEFAULT_PRODUCTS, DOC_LANGS, Doc, Lang, Product, Redirect, docFromPost, parseProducts, parseRedirects, tzOffset } from './lib';

export const TYPE = 'docs_doc';
const INDEX_KEY = 'docs:index:v1';
const INDEX_TTL = 300;

export function setting(key: string): any {
  try {
    return site.settings(key);
  } catch {
    return null;
  }
}

export function products(): Product[] {
  return parseProducts(setting('products')).products;
}

export function productById(id: string): Product | undefined {
  return products().find((p) => p.id === id);
}

/** The site's language: articles without a language are written in it, and its docs live at /docs. */
export function siteLang(): Lang {
  try {
    return String(site.info().language || 'th').toLowerCase().startsWith('en') ? 'en' : 'th';
  } catch {
    return 'th';
  }
}

// The language of the page being answered (1.2.0): /docs/en/… is English on a Thai site. Each route sets it
// first (the runtime may answer the next request with the same module state).
let current: Lang | null = null;
export function setLang(l: Lang | null) {
  current = l;
}

/** The language of the page being answered. */
export function lang(): Lang {
  return current ?? siteLang();
}

/** A language other than the site's that has docs, e.g. "en" on a Thai site ("" when s is none). */
export function otherLang(s: string): Lang | '' {
  const l = s.toLowerCase() as Lang;
  return (DOC_LANGS as string[]).includes(l) && l !== siteLang() && index().some((d) => d.lang === l) ? l : '';
}

/** Languages that have docs, the site's first. */
export function langs(): Lang[] {
  const out: Lang[] = [siteLang()];
  for (const l of DOC_LANGS) if (!out.includes(l) && index().some((d) => d.lang === l)) out.push(l);
  return out;
}

/** The language of a doc ('' = the site's). */
export const docLang = (d: { lang: string }): Lang => ((d.lang || siteLang()) as Lang);

/** The docs home of a language: /docs, or /docs/<lang> for another language. */
export const hubPath = (l: Lang = lang()) => (l === siteLang() ? BASE_PATH : `${BASE_PATH}/${l}`);
const BASE_PATH = '/docs';

export function offset(): number {
  try {
    return tzOffset(String(site.info().timezone || 'Asia/Bangkok'));
  } catch {
    return 7;
  }
}

export function redirects(): Redirect[] {
  return parseRedirects(setting('legacyRedirects')).items;
}

/** Every published doc (also for signed-in editors: drafts are never shown on the public pages). */
export function loadAll(withContent = false): any[] {
  const out: any[] = [];
  for (let p = 1; p <= 50; p++) {
    const res = db.posts.list({ type: TYPE, perPage: 100, page: p, order: 'oldest' });
    for (const it of res.items) if (withContent || it.status === 'publish') out.push(it);
    if (res.items.length < 100) break;
  }
  return out;
}

// The index is parsed once per version in each VM: hooks call index() many times per request and the head hook
// has 50 ms — re-reading the whole cached index each time ran out of it with ~90 docs (1.2.0). A small version
// key tells every VM when the index changed.
const VER_KEY = 'docs:index:ver';
let memo: { v: string; docs: Doc[] } | null = null;

function memoized(): Doc[] | null {
  const v = cache.get<string>(VER_KEY);
  if (typeof v !== 'string') return null;
  if (memo && memo.v === v) return memo.docs;
  const c = cache.get<Doc[]>(INDEX_KEY);
  if (!Array.isArray(c)) return null;
  memo = { v, docs: c };
  return c;
}

/** The published docs without their content (cached). */
export function index(): Doc[] {
  const hit = memoized();
  if (hit) return hit;
  const docs = loadAll().map(docFromPost);
  const v = String(Date.now()) + '-' + Math.random().toString(36).slice(2, 8);
  try {
    cache.set(INDEX_KEY, docs, INDEX_TTL);
    cache.set(VER_KEY, v, INDEX_TTL);
  } catch {
    // too big for the cache: served uncached
  }
  memo = { v, docs };
  return docs;
}

/** The index only when it is already cached (hooks with a small time budget). */
export function cachedIndex(): Doc[] | null {
  return memoized();
}

export const invalidate = () => {
  memo = null;
  cache.delete(VER_KEY);
  cache.delete(INDEX_KEY);
};

/** A published doc with its content, by slug. */
export function docPost(slug: string): any {
  const p = db.posts.get(slug, { type: TYPE });
  return p && p.status === 'publish' ? p : null;
}

/** The published doc of language l whose URL slug is base (1.2.0: a translation's post slug differs). */
export function docPostIn(base: string, l: Lang): any {
  const d = index().find((x) => x.base === base && docLang(x) === l);
  if (d) return docPost(d.slug);
  // the site's language: also a doc not in the index yet — never a translation by its own post slug
  if (l !== siteLang()) return null;
  const p = docPost(base);
  const pl = String(p?.meta?.lang ?? '').trim().toLowerCase();
  return p && (!pl || pl === l) ? p : null;
}

/** The translation of doc d in language l (null when there is none). */
export const translationOf = (d: Doc, l: Lang): Doc | null => index().find((x) => x.base === d.base && docLang(x) === l && x.product === d.product) ?? null;

export const docURL = (d: { product: string; slug: string; base?: string; lang?: string }) =>
  `${hubPath(((d.lang || siteLang()) as Lang))}/${d.product}/${d.base || d.slug}`;

export const knownProduct = (d: Doc, list: Product[] = products()) => list.some((p) => p.id === d.product);

export { DEFAULT_PRODUCTS };
