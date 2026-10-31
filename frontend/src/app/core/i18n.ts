import { Injectable, Pipe, PipeTransform, inject, signal } from '@angular/core';
import { MESSAGES } from './messages';

/** Admin UI languages. Add a language: extend this union and add the field to every message. */
export type Locale = 'th' | 'en';
export const LOCALES: { id: Locale; label: string; short: string }[] = [
  { id: 'th', label: 'ไทย', short: 'TH' },
  { id: 'en', label: 'English', short: 'EN' },
];

const KEY = 'anasaki.locale';

/** Current language, shared by the I18n service and `tr()`. */
const current = signal<Locale>(initialLocale());

/** Translate outside of components (factories, presets, helpers). */
export function tr(key: string, params?: Record<string, string | number>): string {
  const m = MESSAGES[key];
  let s = m ? (m[current()] ?? m.th) : key;
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
    this.locale.set(l);
    document.documentElement.lang = l;
    try { localStorage.setItem(KEY, l); } catch { /* private mode */ }
  }

  /** Locale-aware number/money/date formatting. */
  money(v: number) {
    return new Intl.NumberFormat(this.locale() === 'th' ? 'th-TH' : 'en-US', { style: 'currency', currency: 'THB', maximumFractionDigits: 2 }).format(v || 0);
  }
  date(v: string | Date, withTime = false) {
    return new Intl.DateTimeFormat(this.locale() === 'th' ? 'th-TH' : 'en-GB', {
      day: 'numeric', month: 'short', year: 'numeric', ...(withTime ? { hour: '2-digit', minute: '2-digit' } : {}),
    }).format(new Date(v));
  }
}

function initialLocale(): Locale {
  try {
    const saved = localStorage.getItem(KEY);
    if (saved === 'th' || saved === 'en') return saved;
  } catch { /* ignore */ }
  return navigator.language?.toLowerCase().startsWith('th') ? 'th' : 'en';
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
export function localized(v: { th?: string; en?: string } | string | null | undefined): string {
  if (!v) return '';
  if (typeof v === 'string') return v;
  return v[current()] || v.th || v.en || '';
}
