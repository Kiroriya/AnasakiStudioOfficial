import { randomPassword } from './password';

describe('randomPassword', () => {
  it('has the requested length and only uses the unambiguous alphabet', () => {
    for (let i = 0; i < 50; i++) {
      const p = randomPassword(14);
      expect(p).toHaveLength(14);
      expect(p).toMatch(/^[A-HJ-NP-Za-km-z2-9!@#$%]+$/);
    }
  });

  it('never uses bytes from the biased tail (rejection sampling)', () => {
    // The alphabet has 61 characters, so bytes 244..255 (above 4 × 61) must be skipped.
    // Feed 244..255 first, then 0, 1, 2…: the result has to start from the first characters.
    const bytes = [...Array.from({ length: 12 }, (_, i) => 244 + i), 0, 1, 2, 60, 61];
    let next = 0;
    const spy = vi.spyOn(crypto, 'getRandomValues').mockImplementation(<T extends ArrayBufferView | null>(a: T): T => {
      const u = a as unknown as Uint8Array;
      for (let i = 0; i < u.length; i++) u[i] = bytes[next++ % bytes.length];
      return a;
    });
    try {
      expect(randomPassword(5)).toBe('ABC%A');
    } finally {
      spy.mockRestore();
    }
  });
});
