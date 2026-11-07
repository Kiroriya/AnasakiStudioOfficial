// HTML of the public docs pages (hub, product landing, article, not found). Every page is a route result
// {page: {title, html}} so the site's header and footer wrap it; the look lives in assets/docs.css (class
// prefix dx-) and the behaviour (drawer, search, TOC scroll-spy, copy buttons) in assets/docs.js.
import { Doc, Heading, Lang, Product, Section, docFromPost, editHref, esc, formatDate, htmlToMarkdown, jsonData, prepareContent, readMinutes, readingOrder, sectionsOf, tr } from './lib';
import { docLang, docURL, hubPath, index, knownProduct, lang, langs, offset, products, setting, siteLang, translationOf } from './model';

const T = {
  th: {
    docs: 'เอกสาร', hubTitle: 'ศูนย์รวมเอกสาร', hubIntro: 'คู่มือการใช้งานและข้อมูลอ้างอิง — ค้นหาสิ่งที่ต้องการ หรือเลือกหมวดด้านล่าง',
    search: 'ค้นหาเอกสาร…', searchLabel: 'ค้นหาเอกสาร', start: 'เริ่มอ่าน', docsCount: (n: number) => `${n} บทความ`,
    quick: 'เริ่มต้นอย่างรวดเร็ว', quickHint: 'บทความที่คนส่วนใหญ่เริ่มอ่าน', products: 'ผลิตภัณฑ์',
    onPage: 'ในหน้านี้', updated: 'อัปเดตล่าสุด', minutes: (n: number) => `อ่าน ${n} นาที`, prev: 'ก่อนหน้า', next: 'ถัดไป',
    menu: 'เมนูเอกสาร', openMenu: 'เปิดเมนู', closeMenu: 'ปิดเมนู', switchProduct: 'เปลี่ยนผลิตภัณฑ์', breadcrumb: 'เส้นทาง',
    noSection: 'ทั่วไป', empty: 'ยังไม่มีบทความในส่วนนี้', notFound: 'ไม่พบหน้าเอกสารนี้',
    notFoundText: 'หน้าที่คุณหาอาจถูกย้ายหรือเปลี่ยนชื่อ ลองค้นหา หรือกลับไปที่ศูนย์รวมเอกสาร', backHub: 'กลับไปหน้าเอกสาร',
    allDocs: 'เอกสารทั้งหมด', backToTop: 'กลับขึ้นด้านบน', readGuide: 'อ่านคู่มือ', sections: (n: number) => `${n} หมวด`,
    noDocs: 'ยังไม่มีเอกสาร', noDocsHint: 'ผู้ดูแลเพิ่มเอกสารได้ที่หลังบ้าน → เอกสาร',
    edit: 'แก้ไขหน้านี้', copyMd: 'คัดลอก Markdown', viewMd: 'ดูเป็น Markdown', source: 'ที่มา',
  },
  en: {
    docs: 'Docs', hubTitle: 'Documentation', hubIntro: 'Guides and reference — search for what you need or pick a topic below.',
    search: 'Search docs…', searchLabel: 'Search the docs', start: 'Start reading', docsCount: (n: number) => `${n} ${n === 1 ? 'article' : 'articles'}`,
    quick: 'Quick start', quickHint: 'Where most people begin', products: 'Products',
    onPage: 'On this page', updated: 'Last updated', minutes: (n: number) => `${n} min read`, prev: 'Previous', next: 'Next',
    menu: 'Docs navigation', openMenu: 'Open menu', closeMenu: 'Close menu', switchProduct: 'Switch product', breadcrumb: 'Breadcrumb',
    noSection: 'General', empty: 'No articles here yet', notFound: 'Page not found',
    notFoundText: 'The page may have moved or been renamed. Try searching, or go back to the docs home.', backHub: 'Back to the docs',
    allDocs: 'All docs', backToTop: 'Back to top', readGuide: 'Read the guide', sections: (n: number) => `${n} sections`,
    noDocs: 'No docs yet', noDocsHint: 'Administrators add docs in the admin → Docs',
    edit: 'Edit this page', copyMd: 'Copy as Markdown', viewMd: 'View as Markdown', source: 'Source',
  },
};
type Strings = (typeof T)['th'];

function asset(path: string): string {
  try {
    return site.asset(path).url;
  } catch {
    return '';
  }
}

const ARROW = '<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M6 3l5 5-5 5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';
const ARROW_L = '<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true"><path d="M10 3L5 8l5 5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';
const SEARCH = '<svg viewBox="0 0 20 20" width="18" height="18" aria-hidden="true"><circle cx="9" cy="9" r="6" fill="none" stroke="currentColor" stroke-width="1.7"/><path d="M13.5 13.5L17 17" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>';
const MENU = '<svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true"><path d="M3 5h14M3 10h14M3 15h14" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>';
const CLOSE = '<svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true"><path d="M5 5l10 10M15 5L5 15" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>';
const CHEVRONS = '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M5 6l3-3 3 3M5 10l3 3 3-3" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>';
const PEN = '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><path d="M10.5 2.5l3 3L6 13H3v-3z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>';
const MD = '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><rect x="1.5" y="3.5" width="13" height="9" rx="1.5" fill="none" stroke="currentColor" stroke-width="1.3"/><path d="M4 10V6l2 2 2-2v4M11 6v4m-1.5-1.5L11 10l1.5-1.5" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>';
const CLOCK = '<svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true"><circle cx="8" cy="8" r="6.2" fill="none" stroke="currentColor" stroke-width="1.4"/><path d="M8 4.6V8l2.3 1.4" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg>';

interface Ctx {
  lang: Lang;
  t: Strings;
  products: Product[];
  docs: Doc[];
}

function ctx(): Ctx {
  const l = lang();
  const ps = products();
  // 1.2.0: the docs of the page's language only (an article without a language is in the site's)
  const docs = index().filter((d) => knownProduct(d, ps) && docLang(d) === l);
  return { lang: l, t: T[l], products: ps, docs };
}

const LANG_NAMES: Record<Lang, string> = { th: 'ไทย', en: 'English' };

/**
 * Links to the other languages of this page (1.2.0): the same article when it is translated, else that
 * language's docs home. Nothing when the docs exist in one language only.
 */
function langLinks(c: Ctx, hrefFor: (l: Lang) => string): string {
  const others = langs().filter((l) => l !== c.lang);
  if (!others.length) return '';
  return `<nav class="dx-langs" aria-label="${c.lang === 'th' ? 'ภาษา' : 'Language'}">🌐 ` +
    others.map((l) => `<a href="${esc(hrefFor(l))}" hreflang="${l}" lang="${l}">${esc(LANG_NAMES[l])}</a>`).join(' · ') + `</nav>`;
}

const countOf = (c: Ctx, p: Product) => c.docs.filter((d) => d.product === p.id).length;
const firstDoc = (c: Ctx, p: Product) => readingOrder(sectionsOf(c.docs, p.id, c.t.noSection))[0];

/** Wraps a page body: stylesheet, data for the script, the script (deferred). */
function shell(c: Ctx, cls: string, body: string): string {
  const css = asset('assets/docs.css'), js = asset('assets/docs.js');
  const th = c.lang === 'th';
  const data = { search: hubPath(c.lang) + '/search.json', lang: c.lang, t: { noResults: th ? 'ไม่พบผลลัพธ์สำหรับ' : 'No results for',
    copy: th ? 'คัดลอก' : 'Copy', copied: th ? 'คัดลอกแล้ว' : 'Copied', loading: th ? 'กำลังโหลด…' : 'Loading…', error: th ? 'โหลดดัชนีค้นหาไม่สำเร็จ' : 'Could not load the search index',
    placeholder: c.t.search, label: c.t.searchLabel, suggested: th ? 'แนะนำ' : 'Suggested', results: th ? 'ผลการค้นหา' : 'Results',
    navigate: th ? 'เลื่อน' : 'navigate', open: th ? 'เปิด' : 'open', close: th ? 'ปิด' : 'close', code: th ? 'โค้ด' : 'code',
    copyMd: c.t.copyMd, copiedMd: th ? 'คัดลอก Markdown แล้ว' : 'Markdown copied' } };
  return (css ? `<link rel="stylesheet" href="${esc(css)}">` : '') +
    `<div class="dx ${cls}" data-dx>${body}</div>` +
    `<script type="application/json" id="dx-data">${jsonData(data)}</script>` +
    (js ? `<script defer src="${esc(js)}"></script>` : '');
}

function searchBox(c: Ctx, big = false): string {
  return `<div class="dx-search${big ? ' dx-search-lg' : ''}"><button type="button" class="dx-search-btn" data-dx-search aria-haspopup="dialog" ` +
    `aria-label="${esc(c.t.searchLabel)}">${SEARCH}<span>${esc(c.t.search)}</span><kbd class="dx-kbd" aria-hidden="true">Ctrl K</kbd></button></div>`;
}

function switcher(c: Ctx, current: Product): string {
  const items = c.products.map((p) => {
    const on = p.id === current.id;
    return `<li><a href="${esc(hubPath(c.lang))}/${esc(p.id)}" class="dx-switch-item${on ? ' is-active' : ''}"${on ? ' aria-current="true"' : ''}>` +
      `<span class="dx-picon" aria-hidden="true">${esc(p.icon)}</span><span><strong>${esc(tr(p.name, c.lang))}</strong>` +
      `<small>${esc(tr(p.desc, c.lang))}</small></span></a></li>`;
  }).join('');
  return `<div class="dx-switch" data-dx-switch>` +
    `<button type="button" class="dx-switch-btn" aria-haspopup="true" aria-expanded="false" aria-label="${esc(c.t.switchProduct)}">` +
    `<span class="dx-picon" aria-hidden="true">${esc(current.icon)}</span><span class="dx-switch-name">${esc(tr(current.name, c.lang))}</span>${CHEVRONS}</button>` +
    `<ul class="dx-switch-menu" hidden>${items}</ul></div>`;
}

function sidebar(c: Ctx, product: Product, sections: Section[], currentSlug: string): string {
  const groups = sections.map((s) => {
    const links = s.docs.map((d) => {
      const on = d.slug === currentSlug;
      return `<li><a href="${esc(docURL(d))}"${on ? ' class="is-active" aria-current="page"' : ''}>${esc(d.title)}</a></li>`;
    }).join('');
    return `<details class="dx-nav-group" open data-dx-group="${esc(product.id + '/' + s.name)}"><summary class="dx-nav-title">${esc(s.name)}</summary><ul>${links}</ul></details>`;
  }).join('');
  return `<aside class="dx-sidebar" id="dx-sidebar" aria-label="${esc(c.t.menu)}">` +
    `<div class="dx-sidebar-head"><a class="dx-home" href="${esc(hubPath(c.lang))}">${ARROW_L}<span>${esc(c.t.allDocs)}</span></a>` +
    `<button type="button" class="dx-icon-btn dx-close" data-dx-close aria-label="${esc(c.t.closeMenu)}">${CLOSE}</button></div>` +
    switcher(c, product) + searchBox(c) +
    `<nav class="dx-nav" aria-label="${esc(tr(product.name, c.lang))}">${groups || `<p class="dx-muted">${esc(c.t.empty)}</p>`}</nav></aside>`;
}

function mobileBar(c: Ctx, label: string): string {
  return `<div class="dx-mbar"><button type="button" class="dx-menu-btn" data-dx-open aria-controls="dx-sidebar" aria-expanded="false">${MENU}` +
    `<span>${esc(c.t.menu)}</span></button><span class="dx-mbar-label">${esc(label)}</span></div>`;
}

function crumbs(c: Ctx, items: { label: string; href?: string }[]): string {
  const li = items.map((it, i) => {
    const last = i === items.length - 1;
    const inner = it.href && !last ? `<a href="${esc(it.href)}">${esc(it.label)}</a>` : `<span${last ? ' aria-current="page"' : ''}>${esc(it.label)}</span>`;
    return `<li>${inner}</li>`;
  }).join('');
  return `<nav class="dx-crumbs" aria-label="${esc(c.t.breadcrumb)}"><ol>${li}</ol></nav>`;
}

function toc(c: Ctx, hs: Heading[]): string {
  if (!hs.length) return '';
  const li = hs.map((h) => `<li class="dx-toc-l${h.level}"><a href="#${esc(h.id)}" data-dx-toc="${esc(h.id)}">${esc(h.text)}</a></li>`).join('');
  return `<ul>${li}</ul>`;
}

// ---------------------------------------------------------------- pages

/** The hub's title / intro: the settings are written in the site's language; another language uses its own. */
const hubText = (c: Ctx, key: 'hubTitle' | 'hubIntro') => (c.lang === siteLang() ? String(setting(key) || '').trim() : '') || c.t[key];

export function hubPage(): RouteResult {
  const c = ctx();
  const title = hubText(c, 'hubTitle');
  const intro = hubText(c, 'hubIntro');
  const cards = c.products.map((p) => {
    const first = firstDoc(c, p);
    const n = countOf(c, p);
    return `<div class="dx-card dx-pcard">` +
      `<span class="dx-pcard-icon" aria-hidden="true">${esc(p.icon)}</span>` +
      `<h3 class="dx-pcard-name"><a class="dx-stretch" href="${esc(hubPath(c.lang))}/${esc(p.id)}">${esc(tr(p.name, c.lang))}</a></h3>` +
      `<p class="dx-pcard-desc">${esc(tr(p.desc, c.lang))}</p>` +
      `<div class="dx-pcard-foot"><span class="dx-badge">${esc(c.t.docsCount(n))}</span>` +
      (first ? `<a class="dx-pcard-cta" href="${esc(docURL(first))}">${esc(c.t.start)} <span aria-hidden="true">→</span></a>` : '') + `</div></div>`;
  }).join('');
  // quick links: the first two docs of every product
  const quick = c.products.flatMap((p) => readingOrder(sectionsOf(c.docs, p.id, c.t.noSection)).slice(0, 2).map((d) => ({ d, p })));
  const quickHTML = quick.length
    ? `<section class="dx-quick" aria-labelledby="dx-quick-h"><div class="dx-section-head"><h2 id="dx-quick-h">${esc(c.t.quick)}</h2>` +
      `<p>${esc(c.t.quickHint)}</p></div><ul class="dx-quick-list">` +
      quick.map(({ d, p }) => `<li><a href="${esc(docURL(d))}"><span class="dx-quick-icon" aria-hidden="true">${esc(p.icon)}</span>` +
        `<span class="dx-quick-text"><strong>${esc(d.title)}</strong><small>${esc(tr(p.name, c.lang))}${d.section ? ' · ' + esc(d.section) : ''}</small></span>${ARROW}</a></li>`).join('') +
      `</ul></section>`
    : '';
  const body = `<header class="dx-hero"><div class="dx-hero-inner"><p class="dx-eyebrow">${esc(c.t.docs)}</p>` +
    `<h1 class="dx-hero-title">${esc(title)}</h1><p class="dx-hero-intro">${esc(intro)}</p>${searchBox(c, true)}` +
    langLinks(c, (l) => hubPath(l)) + `</div></header>` +
    `<div class="dx-wrap"><section aria-labelledby="dx-products-h"><h2 id="dx-products-h" class="dx-sr">${esc(c.t.products)}</h2>` +
    `<div class="dx-pgrid">${cards}</div></section>${quickHTML}</div>`;
  return { page: { title, html: shell(c, 'dx-hub', body) }, headers: { 'Cache-Control': 'no-cache' } };
}

export function productPage(productId: string): RouteResult {
  const c = ctx();
  const p = c.products.find((x) => x.id === productId);
  if (!p) return notFoundPage(c);
  const sections = sectionsOf(c.docs, p.id, c.t.noSection);
  const name = tr(p.name, c.lang);
  const groups = sections.map((s, i) => {
    const sid = 'sec-' + (i + 1);
    return `<section class="dx-group" aria-labelledby="${sid}"><h2 id="${sid}">${esc(s.name)}</h2><div class="dx-dgrid">` +
      s.docs.map((d) => `<a class="dx-card dx-dcard" href="${esc(docURL(d))}"><strong>${esc(d.title)}</strong>` +
        (d.excerpt ? `<span>${esc(d.excerpt)}</span>` : '') + `<span class="dx-dcard-go" aria-hidden="true">${ARROW}</span></a>`).join('') +
      `</div></section>`;
  }).join('');
  const first = readingOrder(sections)[0];
  const n = countOf(c, p);
  const main = `<main class="dx-main" id="dx-main">` + crumbs(c, [{ label: c.t.docs, href: hubPath(c.lang) }, { label: name }]) + langLinks(c, (l) => `${hubPath(l)}/${p.id}`) +
    `<header class="dx-phead"><span class="dx-phead-icon" aria-hidden="true">${esc(p.icon)}</span><div><h1>${esc(name)}</h1>` +
    `<p class="dx-lead">${esc(tr(p.desc, c.lang))}</p><p class="dx-meta"><span class="dx-badge">${esc(c.t.docsCount(n))}</span>` +
    `<span class="dx-badge dx-badge-soft">${esc(c.t.sections(sections.length))}</span>` +
    (first ? `<a class="dx-btn" href="${esc(docURL(first))}">${esc(c.t.start)} <span aria-hidden="true">→</span></a>` : '') + `</p></div></header>` +
    (groups || `<div class="dx-empty"><strong>${esc(c.t.noDocs)}</strong><p>${esc(c.t.noDocsHint)}</p></div>`) + `</main>`;
  const body = mobileBar(c, name) + `<div class="dx-layout dx-layout-2">${sidebar(c, p, sections, '')}${main}</div><div class="dx-scrim" data-dx-close hidden></div>`;
  return { page: { title: `${name} — ${c.t.docs}`, html: shell(c, 'dx-product', body) }, headers: { 'Cache-Control': 'no-cache' } };
}

export function articlePage(productId: string, post: any): RouteResult {
  const c = ctx();
  const p = c.products.find((x) => x.id === productId);
  const meta = post?.meta ?? {};
  if (!p || String(meta.product ?? '').trim() !== p.id) return notFoundPage(c);
  const sections = sectionsOf(c.docs, p.id, c.t.noSection);
  const order = readingOrder(sections);
  const i = order.findIndex((d) => d.slug === post.slug);
  const prev = i > 0 ? order[i - 1] : null, next = i >= 0 && i < order.length - 1 ? order[i + 1] : null;
  const section = String(meta.section ?? '').trim() || c.t.noSection;
  const name = tr(p.name, c.lang);
  const { html, headings } = prepareContent(String(post.content ?? ''));
  const updated = formatDate(String(post.updatedAt ?? ''), c.lang, offset());
  const pager = (d: Doc | null, dir: 'prev' | 'next') => d
    ? `<a class="dx-pager-${dir}" href="${esc(docURL(d))}" rel="${dir}"><small>${dir === 'prev' ? ARROW_L + esc(c.t.prev) : esc(c.t.next) + ARROW}</small>` +
      `<strong>${esc(d.title)}</strong></a>`
    : '<span></span>';
  const tocHTML = toc(c, headings);
  const doc = docFromPost(post);
  const mdURL = docURL(doc) + '.md';
  const edit = editHref(setting('editUrl'), { id: Number(post.id), slug: String(post.slug), product: p.id });
  const ext = (u: string) => (u.startsWith('https://') ? ' target="_blank" rel="noopener"' : '');
  const main = `<main class="dx-main" id="dx-main">` +
    crumbs(c, [{ label: c.t.docs, href: hubPath(c.lang) }, { label: name, href: `${hubPath(c.lang)}/${p.id}` }, { label: section }]) +
    langLinks(c, (l) => { const tl = translationOf(doc, l); return tl ? docURL(tl) : `${hubPath(l)}/${p.id}`; }) +
    `<article class="dx-article"><header class="dx-ahead"><p class="dx-eyebrow">${esc(section)}</p><h1>${esc(post.title)}</h1>` +
    (post.excerpt ? `<p class="dx-lead">${esc(post.excerpt)}</p>` : '') +
    `<p class="dx-meta">${CLOCK}<span>${esc(c.t.minutes(readMinutes(post.content)))}</span>` +
    (updated ? `<span class="dx-dot" aria-hidden="true"></span><span>${esc(c.t.updated)} <time datetime="${esc(post.updatedAt)}">${esc(updated)}</time></span>` : '') + `</p>` +
    `<p class="dx-actions"><button type="button" class="dx-chip" data-dx-copy-md="${esc(mdURL)}">${MD}<span>${esc(c.t.copyMd)}</span></button>` +
    `<a class="dx-chip" href="${esc(mdURL)}" type="text/markdown">${esc(c.t.viewMd)}</a>` +
    (edit ? `<a class="dx-chip" href="${esc(edit)}" data-dx-edit${ext(edit)}>${PEN}<span>${esc(c.t.edit)}</span></a>` : '') + `</p></header>` +
    (tocHTML ? `<details class="dx-toc-m"><summary>${esc(c.t.onPage)}</summary>${tocHTML}</details>` : '') +
    `<div class="dx-content">${html}</div></article>` +
    (prev || next ? `<nav class="dx-pager" aria-label="${esc(c.t.prev)} / ${esc(c.t.next)}">${pager(prev, 'prev')}${pager(next, 'next')}</nav>` : '') +
    `<footer class="dx-afoot">${edit ? `<a href="${esc(edit)}"${ext(edit)}>${PEN} ${esc(c.t.edit)}</a>` : ''}${updated ? `<span>${esc(c.t.updated)}: <time datetime="${esc(post.updatedAt)}">${esc(updated)}</time></span>` : '<span></span>'}` +
    `<a href="#dx-top" data-dx-top>${esc(c.t.backToTop)} ↑</a></footer></main>`;
  const aside = `<aside class="dx-toc" aria-label="${esc(c.t.onPage)}">` +
    (tocHTML ? `<p class="dx-toc-title">${esc(c.t.onPage)}</p>${tocHTML}` : '') + `</aside>`;
  const body = `<span id="dx-top"></span>` + mobileBar(c, name) +
    `<div class="dx-layout dx-layout-3">${sidebar(c, p, sections, post.slug)}${main}${aside}</div><div class="dx-scrim" data-dx-close hidden></div>`;
  // postId (CMS 2.0.8): the page hooks see this doc, so the SEO plugin applies its per-doc title / description / noindex
  return { page: { title: String(post.title), html: shell(c, 'dx-doc', body), postId: Number(post.id) || undefined }, headers: { 'Cache-Control': 'no-cache' } };
}

export function notFoundPage(c: Ctx = ctx()): RouteResult {
  const body = `<div class="dx-wrap dx-404"><p class="dx-404-code">404</p><h1>${esc(c.t.notFound)}</h1><p>${esc(c.t.notFoundText)}</p>` +
    searchBox(c, true) + `<p><a class="dx-btn" href="${esc(hubPath(c.lang))}">${ARROW_L} ${esc(c.t.backHub)}</a></p></div>`;
  return { status: 404, page: { title: c.t.notFound, html: shell(c, 'dx-nf', body) }, headers: { 'Cache-Control': 'no-cache' } };
}

/** GET /docs/search.json — the client-side search index. */
export function searchIndex(): RouteResult {
  const c = ctx();
  const items = c.docs.map((d) => {
    const p = c.products.find((x) => x.id === d.product) as Product;
    return { title: d.title, excerpt: d.excerpt, product: d.product, productName: tr(p.name, c.lang), section: d.section,
      url: docURL(d), headings: d.headings.map((h) => ({ id: h.id, text: h.text })) };
  });
  return { status: 200, headers: { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'public, max-age=60' },
    body: JSON.stringify({ items }) };
}

// ---------------------------------------------------------------- Markdown and llms.txt (for readers and AI tools)

const MD_HEADERS = { 'Content-Type': 'text/markdown; charset=utf-8', 'Cache-Control': 'public, max-age=60', 'X-Robots-Tag': 'noindex' };
const TXT_HEADERS = { ...MD_HEADERS, 'Content-Type': 'text/plain; charset=utf-8' };

function siteURL(): string {
  try {
    return String(site.info().url || '').replace(/\/+$/, '');
  } catch {
    return '';
  }
}

function docMarkdown(c: Ctx, p: Product, post: any, base: string): string {
  const lead = post.excerpt ? `> ${String(post.excerpt).replace(/\s+/g, ' ').trim()}\n\n` : '';
  return `# ${post.title}\n\n${lead}${c.t.source}: ${base}${docURL(docFromPost(post))}\n\n${htmlToMarkdown(String(post.content ?? ''))}`;
}

/** GET /docs/<product>/<slug>.md — the article as Markdown. */
export function markdownPage(productId: string, post: any): RouteResult {
  const c = ctx();
  const p = c.products.find((x) => x.id === productId);
  if (!p || String(post?.meta?.product ?? '').trim() !== p.id) return { status: 404, headers: { 'Content-Type': 'text/plain; charset=utf-8' }, body: 'not found' };
  return { status: 200, headers: MD_HEADERS, body: docMarkdown(c, p, post, siteURL()) };
}

/** GET /docs/llms.txt — the llmstxt.org index: every article with its Markdown URL, by product. */
export function llmsTxt(): RouteResult {
  const c = ctx();
  const base = siteURL();
  const title = hubText(c, 'hubTitle');
  const intro = hubText(c, 'hubIntro');
  let out = `# ${title}\n\n> ${intro}\n\n`;
  for (const p of c.products) {
    const docs = readingOrder(sectionsOf(c.docs, p.id, c.t.noSection));
    if (!docs.length) continue;
    out += `## ${tr(p.name, c.lang)}\n\n` +
      docs.map((d) => `- [${d.title}](${base}${docURL(d)}.md)${d.excerpt ? ': ' + d.excerpt.replace(/\s+/g, ' ').trim() : ''}`).join('\n') + '\n\n';
  }
  out += `## Optional\n\n- [${c.t.allDocs}](${base}${hubPath(c.lang)}/llms-full.txt)\n`;
  for (const l of langs()) if (l !== c.lang) out += `- [${LANG_NAMES[l]}](${base}${hubPath(l)}/llms.txt)\n`;
  return { status: 200, headers: TXT_HEADERS, body: out };
}

/** GET /docs/llms-full.txt — every published article as one Markdown file (capped at 4 MB). */
export function llmsFull(loadPost: (slug: string) => any): RouteResult {
  const c = ctx();
  const base = siteURL();
  const parts: string[] = [];
  let size = 0;
  for (const p of c.products) {
    for (const d of readingOrder(sectionsOf(c.docs, p.id, c.t.noSection))) {
      const post = loadPost(d.slug);
      if (!post) continue;
      const md = docMarkdown(c, p, post, base);
      if (size + md.length > 4_000_000) break;
      size += md.length;
      parts.push(md);
    }
  }
  return { status: 200, headers: TXT_HEADERS, body: parts.join('\n---\n\n') };
}
