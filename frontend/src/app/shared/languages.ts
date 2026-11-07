// Content languages (2.5.0, multilingual content): the languages a site may add beside its own, with their own
// names, and the "content_languages" option ("en, ja") as a list.

export interface ContentLang { code: string; name: string }

export const CONTENT_LANGS: ContentLang[] = [
  { code: 'th', name: 'ไทย' }, { code: 'en', name: 'English' }, { code: 'zh', name: '中文' }, { code: 'ja', name: '日本語' },
  { code: 'ko', name: '한국어' }, { code: 'vi', name: 'Tiếng Việt' }, { code: 'lo', name: 'ລາວ' }, { code: 'my', name: 'မြန်မာ' },
  { code: 'km', name: 'ខ្មែរ' }, { code: 'ms', name: 'Bahasa Melayu' }, { code: 'id', name: 'Bahasa Indonesia' },
  { code: 'fr', name: 'Français' }, { code: 'de', name: 'Deutsch' }, { code: 'es', name: 'Español' }, { code: 'ru', name: 'Русский' },
];

/** "en, ja" → ['en', 'ja'] (valid codes, no repeats, not the site's own language, at most 10). */
export function parseLangs(v: string, siteLang = 'th'): string[] {
  const out: string[] = [];
  for (const raw of String(v ?? '').split(',')) {
    const c = raw.trim().toLowerCase();
    if (/^[a-z]{2}(-[a-z]{2})?$/.test(c) && c !== siteLang && !out.includes(c) && out.length < 10) out.push(c);
  }
  return out;
}

/** The language's own name ("en" → "English"), else its code in capitals. */
export function langLabel(code: string): string {
  return CONTENT_LANGS.find(l => l.code === code)?.name ?? code.toUpperCase();
}
