const THAI = /[฀-๿]/;

/**
 * Plugin texts written as "English … / ไทย …" (manifests and the store carry one description for both
 * languages): the half in the admin's language. A text without such a split comes back unchanged.
 */
export function oneLanguage(text: string | null | undefined, locale: string): string {
  const s = (text ?? '').trim();
  for (let i = s.lastIndexOf(' / '); i > 0; i = s.lastIndexOf(' / ', i - 1)) {
    const en = s.slice(0, i).trim(), th = s.slice(i + 3).trim();
    if (en && th && THAI.test(th) && !THAI.test(en)) return locale === 'th' ? th : en;
  }
  return s;
}
