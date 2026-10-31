// Settings, site information and the per-post overrides (post meta "seo.*") of the SEO plugin.
import { DEFAULT_TEMPLATE, truthy } from './lib';

export const SETTING_KEYS = ['titleTemplate', 'defaultDescription', 'defaultImage', 'twitterHandle'] as const;
export type SettingKey = (typeof SETTING_KEYS)[number];

const DEFAULTS: Record<SettingKey, string> = {
  titleTemplate: DEFAULT_TEMPLATE,
  defaultDescription: '',
  defaultImage: '',
  twitterHandle: '',
};

/** A setting (Plugins → SEO → Settings, same keys as the built-in plugin); its default when empty. */
export function setting(key: SettingKey): string {
  const v = site.settings(key);
  return v === null || v === undefined || v === '' ? DEFAULTS[key] : String(v);
}

/** Site title / tagline: from the rendered page (siteTitle / siteTagline), else site.info(). */
export function siteInfo(page?: any): { title: string; tagline: string } {
  const info = page && page.siteTitle !== undefined && page.siteTagline !== undefined ? null : site.info();
  return {
    title: String(page?.siteTitle ?? info?.title ?? ''),
    tagline: String(page?.siteTagline ?? info?.tagline ?? ''),
  };
}

export interface Meta {
  title: string;
  description: string;
  image: string;
  noindex: boolean;
}

export const EMPTY: Meta = { title: '', description: '', image: '', noindex: false };

/** Meta from the plugin's own fields ({title, …}, site.meta) or a post's meta ({'seo.title', …}, prefix 'seo.'). */
export function metaFrom(m: any, prefix = ''): Meta {
  if (!m) return { ...EMPTY };
  const g = (k: string) => m[prefix + k];
  return { title: String(g('title') ?? ''), description: String(g('description') ?? ''), image: String(g('image') ?? ''), noindex: truthy(g('noindex')) };
}

/** A post's overrides (null when the post does not exist). */
export function metaOfPost(postId: number): Meta | null {
  if (!(postId > 0)) return null;
  const m = site.meta(postId);
  return m ? metaFrom(m) : null;
}

/** Writes a post's overrides ('' removes a value; core checks the caller may edit the post). */
export function saveMeta(postId: number, m: Meta): void {
  site.setMeta(postId, { title: m.title, description: m.description, image: m.image, noindex: m.noindex });
}
