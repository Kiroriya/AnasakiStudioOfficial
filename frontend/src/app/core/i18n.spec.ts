import { I18n, localized, tr } from './i18n';
import { registerMessages } from './messages';

describe('tr', () => {
  const i18n = new I18n();
  let before: 'th' | 'en';

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
});
