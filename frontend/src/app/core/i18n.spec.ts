import { I18n, LOCALES, loadPack, localized, tr } from './i18n';
import ja from '../i18n/ja.json';
import zh from '../i18n/zh.json';
import ko from '../i18n/ko.json';
import { MESSAGES } from './messages';
import { registerMessages } from './messages';

describe('tr', () => {
  const i18n = new I18n();
  let before: ReturnType<I18n['locale']>;

  beforeAll(() => {
    registerMessages({
      'spec.hello': { th: 'สวัสดี {name}', en: 'Hello {name}' },
      'spec.thOnly': { th: 'ไทยเท่านั้น' } as { th: string; en: string },
    });
  });
  beforeEach(() => { before = i18n.locale(); });
  afterEach(() => i18n.set(before));

  it('returns the key itself when there is no message', () => {
    expect(tr('spec.missing.key')).toBe('spec.missing.key');
    expect(tr('spec.missing.key', { n: 1 })).toBe('spec.missing.key');
  });

  it('uses the current language and fills placeholders (every occurrence)', () => {
    i18n.set('en');
    expect(tr('spec.hello', { name: 'Ann' })).toBe('Hello Ann');
    i18n.set('th');
    expect(tr('spec.hello', { name: 'แอน' })).toBe('สวัสดี แอน');
    expect(tr('spec.missing {x} {x}', { x: 2 })).toBe('spec.missing 2 2');
  });

  it('falls back to Thai when a language is missing', () => {
    i18n.set('en');
    expect(tr('spec.thOnly')).toBe('ไทยเท่านั้น');
  });

  it('localized() picks the language from a manifest pair', () => {
    i18n.set('en');
    expect(localized({ th: 'ก', en: 'A' })).toBe('A');
    expect(localized({ th: 'ก' })).toBe('ก');
    expect(localized('plain')).toBe('plain');
    expect(localized(null)).toBe('');
  });

  it('reads a loaded pack for ja/zh/ko and falls back to English for keys it lacks', async () => {
    await loadPack('ja');
    i18n.set('ja');
    expect(tr('shell.logout')).toBe('ログアウト');
    expect(tr('spec.hello', { name: 'Ann' })).toBe('Hello Ann');
    expect(tr('spec.thOnly')).toBe('ไทยเท่านั้น');
    expect(i18n.intl()).toBe('ja-JP');
    await loadPack('ko');
    i18n.set('ko');
    expect(tr('shell.logout')).toBe('로그아웃');
  });

  it('localized() uses the language when given, else English', () => {
    i18n.set('zh');
    expect(localized({ th: 'ก', en: 'A', zh: '甲' })).toBe('甲');
    expect(localized({ th: 'ก', en: 'A' })).toBe('A');
    expect(localized({ th: 'ก' })).toBe('ก');
  });

  it('offers five languages', () => {
    expect(LOCALES.map(l => l.id)).toEqual(['th', 'en', 'ja', 'zh', 'ko']);
  });
});

describe('language packs', () => {
  const packs = { ja, zh, ko } as Record<string, Record<string, string>>;
  const params = (s: string) => (s.match(/\{[a-zA-Z0-9_]+\}/g) ?? []).sort().join(',');
  for (const [id, pack] of Object.entries(packs)) {
    it(`${id}: keeps every {placeholder} of the English text`, () => {
      const bad = Object.entries(pack).filter(([k, v]) => MESSAGES[k]?.en !== undefined && params(v) !== params(MESSAGES[k].en)).map(([k]) => k);
      expect(bad).toEqual([]);
    });
    it(`${id}: has no empty texts (except keys that are empty on purpose)`, () => {
      expect(Object.entries(pack).filter(([k, v]) => !v.trim() && k !== 'content.post.tip').map(([k]) => k)).toEqual([]);
    });
  }
});
