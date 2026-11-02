// SEO (official plugin 2.0.3) — the JavaScript port of the built-in Go plugin "seo":
//  - per-post fields (site.fields on every type since CMS 2.0.8, post meta "seo.*") shown in a "SEO" panel of
//    the post / page editor (authors on their own posts, like the built-in panel) and on the SEO admin page
//  - page.meta: per-page overrides, site-wide defaults, the title template (%title% %site% %tagline%),
//    canonical, Open Graph and Twitter card tags
//  - JSON-LD (WebSite, WebPage, BlogPosting / Article) in <head> (setting structuredData)
//  - sitemap.urls: core's sitemap minus pages marked noindex (core then serves /sitemap.xml and /robots.txt)
//  - admin page "SEO": search-result previews, per-page editor, the site-wide defaults (site.saveSettings)
//  - exports: legacyImport (the built-in's seo_meta rows → post meta), metaOf(postId)
import { applyTemplate, safeURL, truthy } from './lib';
import { EMPTY, Meta, metaOfPost, saveMeta, setting, siteInfo } from './meta';
import { registerAdmin } from './admin';
import { jsonLd } from './jsonld';

// ---------------------------------------------------------------- per-post fields (post meta "seo.*")

const FIELDS: Record<string, CustomField> = {
  title: { type: 'text', max: 255, label: { th: 'ชื่อในผลการค้นหา', en: 'Search title' } },
  description: { type: 'textarea', max: 2000, label: { th: 'คำอธิบาย (meta description)', en: 'Meta description' } },
  image: { type: 'image', label: { th: 'รูปตอนแชร์ (og:image)', en: 'Share image (og:image)' } },
  noindex: { type: 'bool', label: { th: 'ไม่ให้ Google แสดงหน้านี้ (noindex)', en: 'Hide from search engines (noindex)' } },
};
// CMS 2.0.8+: every content type (also the post types of Site Code and other plugins, e.g. docs); older: post + page
try {
  site.fields('*', FIELDS);
} catch {
  site.fields('post', FIELDS);
  site.fields('page', FIELDS);
}

// ---------------------------------------------------------------- page.meta

/** Absolute URL of a site path or URL, from the page's absolute url and path. A URL on the site's own host takes
 *  the page's scheme (an image picked while the site answered on http:// is shared as https:// on an https site). */
function absURL(page: any, u: string): string {
  const url = String(page?.url ?? ''), path = String(page?.path ?? '');
  const base = path && url.endsWith(path) ? url.slice(0, url.length - path.length) : '';
  if (!u) return u;
  if (/^https?:\/\//i.test(u)) {
    const own = /^(https?):\/\/([^/]+)/i.exec(base), abs = /^(https?):\/\/([^/]+)(.*)$/i.exec(u);
    return own && abs && own[2].toLowerCase() === abs[2].toLowerCase() && own[1] !== abs[1] ? `${own[1]}://${abs[2]}${abs[3]}` : u;
  }
  if (!base) return u;
  return base.replace(/\/+$/, '') + (u.startsWith('/') ? u : '/' + u);
}

site.filter('page.meta', (m, ctx) => {
  const page = ctx?.page ?? {};
  let title = String(m.title ?? ''), description = String(m.description ?? ''), image = String(m.image ?? '');
  let noindex = !!m.noindex;
  const o = metaOfPost(Number(page.id) || 0);
  if (o) {
    if (o.title) title = o.title;
    if (o.description) description = o.description;
    if (o.image) image = o.image;
    noindex = noindex || o.noindex;
  }
  if (!description) description = setting('defaultDescription');
  if (!image) image = setting('defaultImage');
  const info = siteInfo(page);
  const fullTitle = applyTemplate(setting('titleTemplate'), title, info.title, info.tagline);

  const og: Record<string, string> = { ...(m.og ?? {}) };
  const put = (k: string, v: string) => {
    if (v) og[k] = v;
  };
  // what the page is may be set by the plugin that serves it (e.g. og:type article on a docs article)
  const keep = (k: string, v: string) => {
    if (!og[k]) put(k, v);
  };
  const shownTitle = title || info.title;
  let canonical = m.canonical ?? '';
  if ((Number(page.status) || 200) === 200 && page.path && page.url) {
    canonical = page.url;
    put('og:url', page.url);
  }
  keep('og:type', page.type === 'post' ? 'article' : 'website');
  put('og:site_name', info.title);
  put('og:title', shownTitle);
  put('og:description', description);
  const img = image ? absURL(page, image) : '';
  put('og:image', img);
  put('twitter:card', img ? 'summary_large_image' : 'summary');
  let handle = setting('twitterHandle').trim();
  if (handle) {
    if (!handle.startsWith('@')) handle = '@' + handle;
    put('twitter:site', handle);
  }
  put('twitter:title', shownTitle);
  put('twitter:description', description);
  put('twitter:image', img);
  return { ...m, title, fullTitle, description, image, noindex, canonical, og };
});

// ---------------------------------------------------------------- structured data (JSON-LD, 2.0.3)

site.head((page) => jsonLd(page, (u) => (u ? absURL(page, u) : '')));

// ---------------------------------------------------------------- sitemap (core caches the result)

/** Ids of published posts of these types marked noindex. */
function noindexIds(types: string[]): Set<number> {
  const out = new Set<number>();
  for (const type of types) {
    for (let p = 1; p <= 200; p++) {
      const res = db.posts.list({ type, perPage: 100, page: p, where: { 'meta.seo.noindex': true } });
      for (const it of res.items) out.add(Number(it.id));
      if (res.items.length < 100) break;
    }
  }
  return out;
}

site.filter('sitemap.urls', (urls) => {
  // the content types in the list (core and plugins give {id, type} for a post)
  const types = new Set<string>(['page', 'post']);
  for (const u of urls) if (u && typeof u === 'object' && u.id && /^[a-z][a-z0-9_]{1,19}$/.test(String(u.type ?? ''))) types.add(String(u.type));
  const hidden = noindexIds([...types]);
  if (!hidden.size) return urls;
  return urls.filter((u: any) => !(u && typeof u === 'object' && u.id && hidden.has(Number(u.id))));
});

// ---------------------------------------------------------------- exports

site.export('metaOf', (postId: number) => metaOfPost(Number(postId)) ?? { ...EMPTY });

/**
 * Legacy import (phase 7 migration): rows of the built-in plugin's seo_meta table
 * ({post_id, title, description, image, noindex, updated_at}) → the posts' "seo.*" meta (idempotent; rows of
 * deleted posts are skipped). The migration runs it as a system call.
 */
site.export('legacyImport', (payload: any) => {
  if (!payload || payload.kind !== 'rows' || payload.table !== 'seo_meta' || !Array.isArray(payload.rows)) return { imported: 0 };
  let n = 0;
  const seen = new Set<number>();
  for (const r of payload.rows) {
    const id = Number(r?.post_id);
    if (!(id > 0) || seen.has(id)) continue;
    seen.add(id);
    const m: Meta = {
      title: String(r.title ?? '').slice(0, 255),
      description: r.description === null || r.description === undefined ? '' : String(r.description).slice(0, 2000),
      image: String(r.image ?? '').slice(0, 512),
      noindex: truthy(r.noindex),
    };
    if (m.image && !safeURL(m.image)) m.image = '';
    try {
      saveMeta(id, m);
      n++;
    } catch (e) {
      log.warn('seo: legacyImport: post', id, String((e as Error)?.message ?? e));
    }
  }
  return { imported: n };
});

registerAdmin();
