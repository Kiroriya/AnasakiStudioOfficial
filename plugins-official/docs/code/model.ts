// Data of the docs plugin: settings (products, hub texts, redirects) and the index of published docs (cached,
// dropped when a doc is saved or deleted).
import { DEFAULT_PRODUCTS, Doc, Lang, Product, Redirect, docFromPost, parseProducts, parseRedirects, tzOffset } from './lib';

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

export function lang(): Lang {
  try {
    return String(site.info().language || 'th').toLowerCase().startsWith('en') ? 'en' : 'th';
  } catch {
    return 'th';
  }
}

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

/** The published docs without their content (cached). */
export function index(): Doc[] {
  const c = cache.get<Doc[]>(INDEX_KEY);
  if (Array.isArray(c)) return c;
  const docs = loadAll().map(docFromPost);
  try {
    cache.set(INDEX_KEY, docs, INDEX_TTL);
  } catch {
    // too big for the cache: served uncached
  }
  return docs;
}

/** The index only when it is already cached (hooks with a small time budget). */
export function cachedIndex(): Doc[] | null {
  const c = cache.get<Doc[]>(INDEX_KEY);
  return Array.isArray(c) ? c : null;
}

export const invalidate = () => cache.delete(INDEX_KEY);

/** A published doc with its content, by slug. */
export function docPost(slug: string): any {
  const p = db.posts.get(slug, { type: TYPE });
  return p && p.status === 'publish' ? p : null;
}

export const docURL = (d: { product: string; slug: string }) => `/docs/${d.product}/${d.slug}`;

export const knownProduct = (d: Doc, list: Product[] = products()) => list.some((p) => p.id === d.product);

export { DEFAULT_PRODUCTS };
