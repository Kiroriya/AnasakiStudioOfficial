import { adminRoute, clampHeight, readFrameMessage, sourceFrame } from './plugin-frame';

describe('plugin frame messages (admin UI kit)', () => {
  it('maps admin paths to router paths and rejects everything else', () => {
    expect(adminRoute('/admin/x/code/analytics:stats', '/admin/')).toBe('/x/code/analytics:stats');
    expect(adminRoute('/admin/x/code/analytics:stats?days=30', '/admin/')).toBe('/x/code/analytics:stats?days=30');
    expect(adminRoute('/admin', '/admin/')).toBe('/');
    expect(adminRoute('/admin/x/code/a:b', '/')).toBe('/x/code/a:b'); // dev server (base "/")
    expect(adminRoute('/studio/plugins', '/studio/')).toBe('/plugins');
    for (const bad of ['https://evil.test/admin/', '//evil.test/admin/x', '/\evil.test', 'javascript:alert(1)', '/about', '/api/v1/users',
      '/admin/../api/v1/users', '/administrator', 'admin/x', '/admin/x\n', 42, null, undefined]) {
      expect(adminRoute(bad, '/admin/'), String(bad)).toBeNull();
    }
  });

  it('reads height and navigate messages only', () => {
    expect(readFrameMessage({ type: 'ak:height', height: 312.4 }, '/admin/')).toEqual({ type: 'height', height: 313 });
    expect(readFrameMessage({ type: 'ak:navigate', path: '/admin/x/code/analytics:stats' }, '/admin/')).toEqual({ type: 'navigate', route: '/x/code/analytics:stats' });
    expect(readFrameMessage({ type: 'ak:navigate', path: 'https://evil.test' }, '/admin/')).toBeNull();
    expect(readFrameMessage({ type: 'ak:height', height: '300' }, '/admin/')).toBeNull();
    expect(readFrameMessage({ type: 'ak:height', height: -1 }, '/admin/')).toBeNull();
    expect(readFrameMessage({ type: 'ak:height', height: Infinity }, '/admin/')).toBeNull();
    expect(readFrameMessage({ type: 'other' }, '/admin/')).toBeNull();
    expect(readFrameMessage('ak:height', '/admin/')).toBeNull();
    expect(readFrameMessage(null, '/admin/')).toBeNull();
  });

  it('finds the sending iframe by window identity and clamps heights', () => {
    const a = document.body.appendChild(document.createElement('iframe'));
    const b = document.body.appendChild(document.createElement('iframe'));
    expect(sourceFrame([a, b], b.contentWindow)).toBe(b);
    expect(sourceFrame([a], b.contentWindow)).toBeUndefined();
    expect(sourceFrame([a, b], window)).toBeUndefined();
    expect(sourceFrame([a, b], null)).toBeUndefined();
    a.remove();
    b.remove();
    expect(clampHeight(10, 80, 2000)).toBe(80);
    expect(clampHeight(99999, 80, 2000)).toBe(2000);
    expect(clampHeight(300.2, 80, 2000)).toBe(301);
  });
});
