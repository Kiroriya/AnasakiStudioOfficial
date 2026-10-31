import { exportRequest, exportValid, formFromDryRun, parseRoutes, slugId, toggleIn, validateExport, warningText } from './export-utils';

describe('export as plugin', () => {
  it('prefills from the dry run manifest', () => {
    const f = formFromDryRun({
      manifest: { id: 'my-shop', name: 'My Shop', version: '0.3.0', description: 'd', code: { scopes: ['db', 'routes', 'db'], routes: ['/shop', '/cart'], exports: ['cartOf'], library: 'lib/index.ts' } },
      warnings: [],
    });
    expect(f).toEqual({ id: 'my-shop', name: 'My Shop', version: '0.3.0', description: 'd', scopes: ['db', 'routes'], routes: '/shop\n/cart', exports: ['cartOf'], library: 'lib/index.ts' });
  });

  it('falls back to defaults (id from the site title, 1.0.0, nothing ticked)', () => {
    expect(formFromDryRun(null, 'Café Corner!')).toEqual({ id: 'cafe-corner', name: 'Café Corner!', version: '1.0.0', description: '', scopes: [], routes: '', exports: [], library: '' });
    expect(formFromDryRun({ manifest: { id: 'Bad ID', version: 'one' } }).id).toBe('bad-id');
    expect(formFromDryRun({ manifest: { version: 'one' } }).version).toBe('1.0.0');
    expect(slugId('ร้านค้า')).toBe('site-code');
    expect(slugId('a')).toBe('a-x');
    expect(slugId('x'.repeat(60)).length).toBe(40);
  });

  it('validates id, name, version, description and routes', () => {
    const ok = formFromDryRun({ manifest: { id: 'shop', name: 'Shop', version: '1.0.0', code: { routes: ['/shop'] } } });
    expect(exportValid(ok)).toBe(true);
    expect(validateExport({ ...ok, id: 'ab' }).errors.id).toBe('cx.err.id');
    expect(validateExport({ ...ok, id: 'Shop' }).errors.id).toBe('cx.err.id');
    expect(validateExport({ ...ok, id: '-shop' }).errors.id).toBe('cx.err.id');
    expect(validateExport({ ...ok, id: 'site' }).errors.id).toBe('cx.err.reserved');
    expect(validateExport({ ...ok, name: '  ' }).errors.name).toBe('cx.err.name');
    expect(validateExport({ ...ok, version: '1.0' }).errors.version).toBe('cx.err.version');
    expect(validateExport({ ...ok, version: '1.0.0-beta.1' }).errors.version).toBeUndefined();
    expect(validateExport({ ...ok, description: 'x'.repeat(501) }).errors.description).toBe('cx.err.description');
    const r = validateExport({ ...ok, routes: '/shop\n/admin/x' });
    expect([r.errors.routes, r.badRoute]).toEqual(['cx.err.route', '/admin/x']);
    expect(validateExport({ ...ok, routes: 'shop' }).badRoute).toBe('shop');
    expect(validateExport({ ...ok, routes: '/' }).badRoute).toBe('/');
    expect(validateExport({ ...ok, routes: '/api' }).badRoute).toBe('/api');
    expect(validateExport({ ...ok, routes: '/apiary' }).errors.routes).toBeUndefined();
  });

  it('builds the request body', () => {
    expect(parseRoutes(' /shop/ , /cart\n\n/shop ')).toEqual(['/shop', '/cart']);
    const f = { ...formFromDryRun(null, 'x'), id: ' shop ', name: ' Shop ', version: '1.2.0', scopes: ['db'], routes: '/shop' };
    expect(exportRequest(f)).toEqual({ id: 'shop', name: 'Shop', version: '1.2.0', scopes: ['db'], routes: ['/shop'] });
    expect(exportRequest({ ...f, description: ' hi ', exports: ['a'], library: 'lib/i.ts', routes: '' })).toEqual({
      id: 'shop', name: 'Shop', version: '1.2.0', scopes: ['db'], description: 'hi', exports: ['a'], library: 'lib/i.ts',
    });
  });

  it('warnings and checklist toggles', () => {
    expect(warningText('renamed site.x → shop.x')).toBe('renamed site.x → shop.x');
    expect(warningText({ message: 'uses http', file: 'main.ts', line: 4 })).toBe('uses http (main.ts:4)');
    expect(toggleIn(['a'], 'b', true)).toEqual(['a', 'b']);
    expect(toggleIn(['a', 'b'], 'a', false)).toEqual(['b']);
    expect(toggleIn(['a'], 'a', true)).toEqual(['a']);
  });
});
