// Docs (official plugin 1.1.5) — a documentation hub for the site:
//  - post type "docs_doc" (site.postType in a plugin, CMS 2.0.5, scope post-types) edited in the normal post
//    editor, with the fields product, section, order and sectionOrder (post meta, unprefixed); public: false, so
//    core does not route the type — the plugin serves its pages under /docs (core's post URL /docs/<slug> is
//    redirected to /docs/<product>/<slug>)
//  - GET /docs (hub), /docs/<product> (landing), /docs/<product>/<slug> (article: sidebar, breadcrumb, TOC,
//    prev/next), /docs/search.json (client-side search index); unknown pages → 404 in the site's layout
//  - Markdown for readers and AI tools: /docs/<product>/<slug>.md, /docs/llms.txt (llmstxt.org) and /docs/llms-full.txt
//  - page.meta (description, canonical, og:type) and sitemap.urls (every doc)
//  - legacy redirects (setting legacyRedirects) in a middleware, so old links keep working after a move
//  - admin page "Docs hub": docs grouped by product / section, the settings editor and the JSON bundle import
import { BASE, Lang, SLUG_RE, docFromPost, normPath, tr } from './lib';
import { TYPE, cachedIndex, docLang, docPost, docPostIn, docURL, hubPath, index, invalidate, knownProduct, lang, langs, otherLang, products, redirects, setLang, setting, siteLang, translationOf } from './model';
import { articlePage, hubPage, llmsFull, llmsTxt, markdownPage, notFoundPage, productPage, searchIndex } from './render';
import { registerAdmin } from './admin';

// ---------------------------------------------------------------- content type and fields

// CMS 2.0.6+: who edits and publishes docs are the plugin's own capabilities docs.edit / docs.publish
// (manifest "capabilities", default administrators and editors), ticked per role in Users → Roles. Older CMS
// versions: the role list (editors and administrators edit and publish).
const CAPS = typeof (site as any).can === 'function';

site.postType(TYPE, {
  label: { th: 'เอกสาร', en: 'Docs' },
  singular: { th: 'เอกสาร', en: 'Doc' },
  icon: '📘',
  slug: 'docs',
  public: false, // the plugin serves /docs/<product>/<slug> itself
  supports: ['title', 'content', 'excerpt'],
  ...(CAPS ? { caps: { edit: 'docs.edit', publish: 'docs.publish' } } : { access: { edit: ['editor', 'administrator'] } }),
  fields: {
    product: { type: 'text', required: true, max: 40, label: { th: 'ผลิตภัณฑ์', en: 'Product' } },
    section: { type: 'text', max: 80, label: { th: 'หมวด', en: 'Section' } },
    order: { type: 'number', label: { th: 'ลำดับ', en: 'Order' } },
    sectionOrder: { type: 'number', label: { th: 'ลำดับหมวด', en: 'Section order' } },
    // 1.2.0: a translation — its language, and the slug of the URL it shares with the original
    lang: { type: 'text', max: 5, label: { th: 'ภาษา (th / en — ว่าง = ภาษาของเว็บ)', en: 'Language (th / en — empty = the site language)' } },
    base: { type: 'text', max: 190, label: { th: 'slug ในลิงก์ (ฉบับแปล: slug ของต้นฉบับ)', en: 'URL slug (a translation: the original slug)' } },
  },
});

// ---------------------------------------------------------------- public pages

const ANY = { access: ['*'] };
const moved = (to: string): RouteResult => ({ status: 301, headers: { Location: to, 'Cache-Control': 'no-cache' }, body: '' });
const productOf = (post: any) => String(post?.meta?.product ?? '').trim();

// 1.2.0: /docs/… is the site's language, /docs/<lang>/… another one (e.g. /docs/en on a Thai site). Every
// route sets the page's language first.
const MD_404: RouteResult = { status: 404, headers: { 'Content-Type': 'text/plain; charset=utf-8' }, body: 'not found' };

/** An article of language l at …/<product>/<slug>[.md]. */
function article(l: Lang, id: string, raw: string): RouteResult {
  // …/<slug>.md — the article as Markdown ("Copy as Markdown", AI tools)
  const md = raw.endsWith('.md');
  const slug = md ? raw.slice(0, -3) : raw;
  const post = SLUG_RE.test(slug) ? docPostIn(slug, l) : null;
  if (md) return post ? markdownPage(productOf(post) === id ? id : '', post) : MD_404;
  if (!post) return notFoundPage();
  const prod = productOf(post);
  // the doc moved to another product: one canonical URL
  if (prod !== id && products().some((p) => p.id === prod)) return moved(docURL(docFromPost(post)));
  return articlePage(id, post);
}

site.route(`GET ${BASE}`, ANY, () => (setLang(null), hubPage()));
site.route(`GET ${BASE}/search.json`, ANY, () => (setLang(null), searchIndex()));
site.route(`GET ${BASE}/llms.txt`, ANY, () => (setLang(null), llmsTxt()));
site.route(`GET ${BASE}/llms-full.txt`, ANY, () => (setLang(null), llmsFull(docPost)));
site.route(`GET ${BASE}/{product}`, ANY, ({ request }) => {
  setLang(null);
  const id = String(request.params.product);
  if (products().some((p) => p.id === id)) return productPage(id);
  const other = otherLang(id); // /docs/en — the docs home in English
  if (other) {
    setLang(other);
    return hubPage();
  }
  // core's post URL of the type (/docs/<post-slug>) → the article
  const post = SLUG_RE.test(id) ? docPost(id) : null;
  const prod = productOf(post);
  return post && products().some((p) => p.id === prod) ? moved(docURL(docFromPost(post))) : notFoundPage();
});
site.route(`GET ${BASE}/{product}/{slug}`, ANY, ({ request }) => {
  setLang(null);
  const raw = String(request.params.slug), id = String(request.params.product);
  const other = products().some((p) => p.id === id) ? '' : otherLang(id);
  if (other) { // /docs/en/<product>, /docs/en/search.json, /docs/en/llms.txt …
    setLang(other);
    if (raw === 'search.json') return searchIndex();
    if (raw === 'llms.txt') return llmsTxt();
    if (raw === 'llms-full.txt') return llmsFull(docPost);
    return products().some((p) => p.id === raw) ? productPage(raw) : notFoundPage();
  }
  return article(siteLang(), id, raw);
});
site.route(`GET ${BASE}/{lang}/{product}/{slug}`, ANY, ({ request }) => {
  setLang(null);
  const other = otherLang(String(request.params.lang));
  if (!other) return String(request.params.slug).endsWith('.md') ? MD_404 : notFoundPage();
  setLang(other);
  return article(other, String(request.params.product), String(request.params.slug));
});

// ---------------------------------------------------------------- caches

for (const ev of ['post.saved', 'post.published', 'post.deleted']) {
  site.on(ev, (e: any) => {
    if (!e || e.type === TYPE) invalidate();
  });
}

// ---------------------------------------------------------------- SEO: page.meta and sitemap

site.filter('page.meta', (m: any, ctx: any) => {
  const path = normPath(String(ctx?.page?.path ?? ctx?.path ?? ''));
  if (path !== BASE && !path.startsWith(BASE + '/')) return m;
  if ((Number(ctx?.page?.status) || 200) !== 200) return m;
  const parts = path.split('/').filter(Boolean); // ["docs", product?, slug?] — or ["docs", lang, …] (1.2.0)
  const other = parts.length > 1 && !products().some((p) => p.id === parts[1]) ? otherLang(parts[1]) : '';
  if (other) parts.splice(1, 1);
  setLang(other || null);
  let description = '', type = 'website';
  if (parts.length === 1) {
    description = String(setting('hubIntro') || '').trim();
  } else if (parts.length === 2) {
    const p = products().find((x) => x.id === parts[1]);
    description = p ? tr(p.desc, lang()) : '';
  } else if (parts.length === 3) {
    const docs = cachedIndex() ?? [];
    const doc = docs.find((x) => x.base === parts[2] && x.product === parts[1] && docLang(x) === lang());
    if (doc) description = doc.excerpt;
    type = 'article';
  }
  return {
    ...m,
    description: m.description || description,
    canonical: m.canonical || path,
    og: { ...(m.og ?? {}), 'og:type': type },
  };
}, { priority: 5 });

site.filter('sitemap.urls', (urls: any[]) => {
  const ps = products();
  const docs = index().filter((d) => knownProduct(d, ps));
  // an old address that now redirects (legacy redirects) must not be listed: search engines want final URLs
  const moved = new Set(redirects().map((r) => r.from));
  const pathOf = (u: any) => normPath(String(u && typeof u === 'object' ? u.loc ?? u.url ?? '' : u ?? '').replace(/^https?:\/\/[^/]+/i, ''));
  const out = [...urls.filter((u) => !moved.has(pathOf(u)))];
  for (const l of langs()) { // every language's home and product pages (1.2.0)
    out.push({ loc: hubPath(l), changefreq: 'weekly' });
    for (const p of ps) out.push({ loc: `${hubPath(l)}/${p.id}`, changefreq: 'weekly' });
  }
  // id + type: other sitemap filters (the SEO plugin's noindex) see which post an entry is
  for (const d of docs) out.push({ loc: docURL(d), id: d.id, type: TYPE, ...(d.updatedAt ? { lastmod: d.updatedAt } : {}) });
  return out;
});

// schema.org BreadcrumbList (JSON-LD) of a product page and an article: Docs › product › article (1.1.2)
site.head((page: any) => {
  const path = normPath(String(page?.path ?? ''));
  if ((path !== BASE && !path.startsWith(BASE + '/')) || (Number(page?.status) || 200) !== 200 || !page?.url) return '';
  const parts = path.split('/').filter(Boolean); // ["docs", product, slug?] — or ["docs", lang, …] (1.2.0)
  const other = parts.length > 1 && !products().some((x) => x.id === parts[1]) ? otherLang(parts[1]) : '';
  if (other) parts.splice(1, 1);
  const l: Lang = other || siteLang();
  const origin = String(page.url).slice(0, String(page.url).length - path.length);
  // hreflang: this page in every language that has it (1.2.0)
  const alt = (href: (x: Lang) => string | null) => {
    if (langs().length < 2) return '';
    const links = langs().map((x) => [x, href(x)] as const).filter(([, h]) => !!h);
    return links.length < 2 ? '' : links.map(([x, h]) => `<link rel="alternate" hreflang="${x}" href="${origin}${h}">`).join('');
  };
  if (parts.length === 1) return alt((x) => hubPath(x));
  const p = products().find((x) => x.id === parts[1]);
  if (!p || parts.length > 3) return '';
  const items = [
    { name: l === 'en' ? 'Docs' : 'เอกสาร', item: origin + hubPath(l) },
    { name: tr(p.name, l), item: origin + hubPath(l) + '/' + p.id },
  ];
  let alts: string;
  if (parts.length === 3) {
    const doc = (cachedIndex() ?? []).find((x) => x.base === parts[2] && x.product === p.id && docLang(x) === l);
    if (!doc) return '';
    items.push({ name: doc.title, item: origin + path });
    alts = alt((x) => { const t = x === l ? doc : translationOf(doc, x); return t ? docURL(t) : null; });
  } else alts = alt((x) => `${hubPath(x)}/${p.id}`);
  const data = { '@context': 'https://schema.org', '@type': 'BreadcrumbList',
    itemListElement: items.map((x, i) => ({ '@type': 'ListItem', position: i + 1, name: x.name, item: x.item })) };
  return alts + '<script type="application/ld+json">' + JSON.stringify(data).replace(/</g, '\\u003c') + '</script>';
});

// ---------------------------------------------------------------- legacy redirects (old URL → new URL)

site.middleware((ctx) => {
  const req = ctx.request;
  if (req.method !== 'GET' && req.method !== 'HEAD') return;
  const path = normPath(req.path);
  const hit = redirects().find((r) => r.from === path);
  if (!hit) return;
  const q = req.url.indexOf('?');
  const query = q >= 0 && !hit.to.includes('?') ? req.url.slice(q) : '';
  return { status: 301, headers: { Location: hit.to + query, 'Cache-Control': 'no-cache' }, body: '' };
}, { priority: 4 });

registerAdmin();
