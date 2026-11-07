import { oneLanguage } from './bilingual';

describe('oneLanguage', () => {
  const d = 'Online shop: cart, PromptPay QR / slip upload. / ร้านค้าออนไลน์ ตะกร้า ชำระด้วย QR พร้อมเพย์';
  it('keeps the half of the admin language', () => {
    expect(oneLanguage(d, 'th')).toBe('ร้านค้าออนไลน์ ตะกร้า ชำระด้วย QR พร้อมเพย์');
    expect(oneLanguage(d, 'en')).toBe('Online shop: cart, PromptPay QR / slip upload.');
    expect(oneLanguage(d, 'ja')).toBe('Online shop: cart, PromptPay QR / slip upload.');
  });
  it('leaves single-language texts alone', () => {
    expect(oneLanguage('Just English / with a slash', 'th')).toBe('Just English / with a slash');
    expect(oneLanguage('ภาษาไทยอย่างเดียว', 'en')).toBe('ภาษาไทยอย่างเดียว');
    expect(oneLanguage(undefined, 'th')).toBe('');
  });
});
