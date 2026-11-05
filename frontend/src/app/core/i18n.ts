import { Injectable, Pipe, PipeTransform, inject, signal } from '@angular/core';
import { MESSAGES } from './messages';

/**
 * Admin UI languages. th and en live in the messages files ({th, en} per key); since 2.2.2 the other languages are
 * packs (../i18n/<id>.json, key → text) loaded when chosen — a key missing there reads English.
 */
export type Locale = 'th' | 'en' | 'ja' | 'zh' | 'ko';
export const LOCALES: { id: Locale; label: string; short: string }[] = [
  { id: 'th', label: 'ไทย', short: 'TH' },
  { id: 'en', label: 'English', short: 'EN' },
  { id: 'ja', label: '日本語', short: 'JA' },
  { id: 'zh', label: '中文', short: 'ZH' },
  { id: 'ko', label: '한국어', short: 'KO' },
];
const IDS = new Set<string>(LOCALES.map(l => l.id));
export const isLocale = (v: unknown): v is Locale => typeof v === 'string' && IDS.has(v);

/** Intl locale per admin language. */
const INTL: Record<Locale, string> = { th: 'th-TH', en: 'en-GB', ja: 'ja-JP', zh: 'zh-CN', ko: 'ko-KR' };

/** Loaded packs of the non-built-in languages; `packTick` re-renders the templates when one arrives. */
const packs: Partial<Record<Locale, Record<string, string>>> = {};
const packTick = signal(0);
const PACK_LOADERS: Partial<Record<Locale, () => Promise<{ default: Record<string, string> }>>> = {
  ja: () => import('../i18n/ja.json'),
  zh: () => import('../i18n/zh.json'),
  ko: () => import('../i18n/ko.json'),
};
/** Loads the pack of l (no-op for th / en or when loaded). */
export async function loadPack(l: Locale): Promise<void> {
  const load = PACK_LOADERS[l];
  if (!load || packs[l]) return;
  try {
    packs[l] = (await load()).default;
    packTick.update(n => n + 1);
  } catch { /* offline: English stays */ }
}

const KEY = 'anasaki.locale';

/** Current language, shared by the I18n service and `tr()`. */
const current = signal<Locale>(initialLocale());

/** Translate outside of components (factories, presets, helpers). */
export function tr(key: string, params?: Record<string, string | number>): string {
  const m = MESSAGES[key];
  const l = current();
  packTick(); // read: a pack arriving later re-renders
  let s = l === 'th' || l === 'en' ? (m ? (m[l] ?? m.th) : key) : (packs[l]?.[key] ?? (m ? (m.en ?? m.th) : key));
  if (params) for (const [k, v] of Object.entries(params)) s = s.split(`{${k}}`).join(String(v));
  return s;
}

/**
 * Per-account UI language. The account's `locale` (saved on the server) wins;
 * before login we use the last choice on this device, then the browser language.
 */
@Injectable({ providedIn: 'root' })
export class I18n {
  readonly locale = current;

  /** Translate `key`; `{name}` placeholders are filled from `params`. */
  t(key: string, params?: Record<string, string | number>): string {
    return tr(key, params);
  }

  set(l: Locale) {
    void loadPack(l);
    this.locale.set(l);
    document.documentElement.lang = l;
    try { localStorage.setItem(KEY, l); } catch { /* private mode */ }
  }

  /** The Intl locale of the admin language (th-TH, en-GB, ja-JP, zh-CN, ko-KR). */
  intl(): string { return INTL[this.locale()] ?? 'en-GB'; }

  /** Locale-aware number/money/date formatting. */
  money(v: number) {
    return new Intl.NumberFormat(INTL[this.locale()] ?? 'en-US', { style: 'currency', currency: 'THB', maximumFractionDigits: 2 }).format(v || 0);
  }
  date(v: string | Date, withTime = false) {
    return new Intl.DateTimeFormat(INTL[this.locale()] ?? 'en-GB', {
      day: 'numeric', month: 'short', year: 'numeric', ...(withTime ? { hour: '2-digit', minute: '2-digit' } : {}),
    }).format(new Date(v));
  }
}

function initialLocale(): Locale {
  try {
    const saved = localStorage.getItem(KEY);
    if (isLocale(saved)) { void loadPack(saved); return saved; }
  } catch { /* ignore */ }
  const nav = (navigator.language || '').toLowerCase().slice(0, 2);
  const l: Locale = isLocale(nav) ? nav : 'en';
  void loadPack(l);
  return l;
}

/** `{{ 'nav.posts' | t }}` — impure so it follows language switches. */
@Pipe({ name: 't', pure: false })
export class TPipe implements PipeTransform {
  private i18n = inject(I18n);
  transform(key: string, params?: Record<string, string | number>): string {
    return this.i18n.t(key, params);
  }
}

/** Picks the current language from a `{th, en}` pair (plugin manifests); plain strings pass through. */
export function localized(v: { th?: string; en?: string; ja?: string; zh?: string; ko?: string } | string | null | undefined): string {
  if (!v) return '';
  if (typeof v === 'string') return v;
  const l = current();
  // other languages: the plugin's own text in that language, else English (2.2.2)
  return v[l] || (l === 'th' ? v.th || v.en : v.en || v.th) || '';
}
