// Structured data (schema.org JSON-LD) in <head>, since 2.0.3: one @graph per page with
//  - WebSite (the site's name and address) and WebPage (or CollectionPage on archives) on every page,
//  - BlogPosting for a post, Article for a post of another type (a doc, an event…; not a page): headline,
//    description, image, dates, author, publisher.
// The per-post SEO values (title, description, image) are used like in the meta tags; nothing on a page marked
// noindex or answered with an error. Setting "structuredData" (default on) switches it off.
import { truthy } from './lib';
import { metaOfPost } from './meta';

/** The site's address (scheme + host) from the page's absolute url and path. */
function origin(page: any): string {
  const url = String(page?.url ?? ''), path = String(page?.path ?? '');
  const base = path && url.endsWith(path) ? url.slice(0, url.length - path.length) : '';
  return base.replace(/\/+$/, '');
}

/** JSON for a <script> element: "<" escaped so the text can never close the element. */
export function ldScript(data: unknown): string {
  return '<script type="application/ld+json">' + JSON.stringify(data).replace(/</g, '\\u003c') + '</script>';
}

/** The @graph of a page ('' when there is nothing to describe). */
export function jsonLd(page: any, img: (u: string) => string): string {
  const on = site.settings('structuredData');
  if (on !== undefined && on !== null && on !== '' && !truthy(on)) return '';
  if ((Number(page?.status) || 200) !== 200 || !page?.url) return '';
  const id = Number(page.id) || 0;
  const o = metaOfPost(id);
  if (o?.noindex) return '';
  const base = origin(page);
  if (!base) return '';
  const siteName = String(page.siteTitle ?? site.info().title ?? '');
  const lang = String(page.lang ?? '') || undefined;
  const title = o?.title || String(page.title ?? '');
  const description = o?.description || String(page.description ?? '') || undefined;
  const image = img(o?.image || String(page.image ?? '')) || undefined;
  const website = { '@type': 'WebSite', '@id': base + '/#website', url: base + '/', name: siteName, inLanguage: lang };
  const webpage: Record<string, unknown> = {
    '@type': page.archiveType ? 'CollectionPage' : 'WebPage', '@id': page.url + '#webpage', url: page.url,
    name: title || siteName, description, isPartOf: { '@id': website['@id'] }, inLanguage: lang,
  };
  const graph: Record<string, unknown>[] = [website, webpage];
  if (id > 0 && page.type && page.type !== 'page' && !page.isHome && !page.archiveType) {
    const post = db.posts.get(id, { type: String(page.type) });
    if (post && post.status === 'publish') {
      const article: Record<string, unknown> = {
        '@type': post.type === 'post' ? 'BlogPosting' : 'Article', '@id': page.url + '#article',
        headline: (title || post.title).slice(0, 110), description, image: image ? [image] : undefined,
        datePublished: post.publishedAt || undefined, dateModified: post.updatedAt || undefined,
        author: post.author ? { '@type': 'Person', name: post.author } : undefined,
        publisher: { '@type': 'Organization', name: siteName },
        articleSection: post.category || undefined, inLanguage: lang,
        mainEntityOfPage: { '@id': webpage['@id'] }, isPartOf: { '@id': website['@id'] },
      };
      graph.push(article);
      webpage.mainEntity = { '@id': article['@id'] };
    }
  }
  return ldScript({ '@context': 'https://schema.org', '@graph': graph });
}
