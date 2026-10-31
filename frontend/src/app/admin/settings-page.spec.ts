import { CORE_PATHS, reservedOwner } from './settings-page';

describe('settings: reserved paths', () => {
  const reserved = [{ path: 'basket', plugin: 'shop-x' }, { path: 'api' }];
  it('knows only the core paths by itself', () => {
    expect(CORE_PATHS).toContain('api');
    for (const p of ['cart', 'checkout', 'thank-you', 'forms', 'shop', 'product']) expect(CORE_PATHS).not.toContain(p);
    expect(reservedOwner('cart', [])).toBeNull();
  });
  it('takes the plugin paths from the server', () => {
    expect(reservedOwner('basket', reserved)).toBe('shop-x');
    expect(reservedOwner('api', reserved)).toBe('');
    expect(reservedOwner('uploads', [])).toBe('');
    expect(reservedOwner('backstage', reserved)).toBeNull();
  });
});
