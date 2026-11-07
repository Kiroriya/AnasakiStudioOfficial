import { avatarInitial } from './initial';

describe('avatarInitial', () => {
  it('skips Thai leading vowels and upper-cases Latin letters', () => {
    expect(avatarInitial('เจ้าของร้าน')).toBe('จ');
    expect(avatarInitial('ไพลิน')).toBe('พ');
    expect(avatarInitial('สมชาย')).toBe('ส');
    expect(avatarInitial(' anna ')).toBe('A');
    expect(avatarInitial('')).toBe('?');
    expect(avatarInitial(undefined)).toBe('?');
  });
});
