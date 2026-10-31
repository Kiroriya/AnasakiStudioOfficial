// SEO (official plugin 2.0.0) — the JavaScript port of the built-in Go plugin "seo":
//  - per-post fields (site.fields, post meta "seo.title|description|image|noindex") shown in a "SEO" panel of
//    the post / page editor (authors on their own posts, like the built-in panel) and on the SEO admin page
//  - page.meta: per-page overrides, site-wide defaults, the title template (%title% %site% %tagline%),
//    canonical, Open Graph and Twitter card tags
//  - sitemap.urls: core's sitemap minus pages marked noindex (core then serves /sitemap.xml and /robots.txt)
//  - admin page "SEO": search-result previews, per-page editor, the site-wide defaults (site.saveSettings)
//  - exports: legacyImport (the built-in's seo_meta rows → post meta), metaOf(postId)
import { applyTemplate, safeURL, truthy } from './lib';
import { EMPTY, Meta, metaOfPost, saveMeta, setting, siteInfo } from './meta';
import { registerAdmin } from './admin';

// ---------------------------------------------------------------- per-post fields (post meta "seo.*")

const FIELDS: Record<string, CustomField> = {
  title: { type: 'text', max: 255, label: { th: 'ชื่อในผลการค้นหา', en: 'Search title' } },
  description: { type: 'textarea', max: 2000, label: { th: 'คำอธิบาย (meta description)', en: 'Meta description' } },
  image: { type: 'image', label: { th: 'รูปตอนแชร์ (og:image)', en: 'Share image (og:image)' } },
  noindex: { type: 'bool', label: { th: 'ไม่ให้ Google แสดงหน้านี้ (noindex)', en: 'Hide from search engines (noindex)' } },
};
site.fields('post', FIELDS);
site.fields('page', FIELDS);

// ---------------------------------------------------------------- page.meta

/** Absolute URL of a site path or URL, from the page's absolute url and path. */
function absURL(page: any, u: string): string {
  if (!u || /^https?:\/\//i.test(u)) return u;
  const url = String(page?.url ?? ''), path = String(page?.path ?? '');
  const base = path && url.endsWith(path) ? url.slice(0, url.length - path.length) : '';
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
  const shownTitle = title || info.title;
  let canonical = m.canonical ?? '';
  if ((Number(page.status) || 200) === 200 && page.path && page.url) {
    canonical = page.url;
    put('og:url', page.url);
  }
  put('og:type', page.type === 'post' ? 'article' : 'website');
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

// ---------------------------------------------------------------- sitemap (core caches the result)

/** Ids of published pages / posts marked noindex. */
function noindexIds(): Set<number> {
  const out = new Set<number>();
  for (const type of ['page', 'post']) {
    for (let p = 1; p <= 200; p++) {
      const res = db.posts.list({ type, perPage: 100, page: p, where: { 'meta.seo.noindex': true } });
      for (const it of res.items) out.add(Number(it.id));
      if (res.items.length < 100) break;
    }
  }
  return out;
}

site.filter('sitemap.urls', (urls) => {
  const hidden = noindexIds();
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
