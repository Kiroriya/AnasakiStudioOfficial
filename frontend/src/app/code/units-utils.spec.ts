import { filterByUnit, groupByUnit, normalizeUnits, pickUnit, settingKeyForUnit, settingsForUnit, shortName, unitOfName } from './units-utils';

describe('code units', () => {
  it('normalises the unit list: site first, objects or ids, inactive and bad ids dropped', () => {
    expect(normalizeUnits(['site', 'shop', 'shop', 'Bad Id', { id: 'money', name: 'Money', active: true }, { id: 'off', active: false }]).map(u => [u.id, u.name])).toEqual([
      ['site', 'site'], ['shop', 'shop'], ['money', 'Money'],
    ]);
    // no units from /code/status → active JS packages
    expect(normalizeUnits(undefined, [
      { id: 'shop', name: 'Shop', runtime: 'js', active: true }, { id: 'svc', runtime: 'service', active: true }, { id: 'old', runtime: 'js', active: false },
    ]).map(u => u.id)).toEqual(['site', 'shop']);
    expect(normalizeUnits([], []).map(u => u.id)).toEqual(['site']);
  });

  it('switcher state: unknown units fall back to site', () => {
    const units = normalizeUnits(['shop']);
    expect([pickUnit('shop', units), pickUnit('ghost', units), pickUnit(null, units), pickUnit('', units)]).toEqual(['shop', 'site', 'site', 'site']);
  });

  it('unit of a registration name', () => {
    expect(unitOfName('events')).toBe('site');
    expect(unitOfName('shop:products')).toBe('shop');
    expect(unitOfName('GET /posts/:id')).toBe('site');
    expect(unitOfName('anything', 'money')).toBe('money');
    expect(unitOfName('ghost:x', undefined, ['shop'])).toBe('site');
    expect(shortName('shop:products', 'shop')).toBe('products');
    expect(shortName('events', 'site')).toBe('events');
  });

  it('filters rows by unit field or name prefix', () => {
    const rows = [
      { name: 'events' }, { name: 'shop:cart' }, { name: 'GET /a/:id' }, { name: 'x', unit: 'money' }, { name: 'shop:orders', unit: 'shop' },
    ];
    expect(filterByUnit(rows, 'site', ['shop', 'money']).map(r => r.name)).toEqual(['events', 'GET /a/:id']);
    expect(filterByUnit(rows, 'shop', ['shop', 'money']).map(r => r.name)).toEqual(['shop:cart', 'shop:orders']);
    expect(filterByUnit(rows, 'money', ['shop', 'money']).map(r => r.name)).toEqual(['x']);
  });

  it('settings keys of plugin units', () => {
    expect(settingKeyForUnit('shop.API_KEY', 'shop')).toBe('API_KEY');
    expect(settingKeyForUnit('shop.API_KEY', 'site')).toBe('shop.API_KEY');
    expect(settingsForUnit([
      { key: 'shop.A', secret: true, set: true, unit: 'shop' }, { key: 'B', secret: false, set: true }, { key: 'C', secret: false, set: true, unit: 'site' },
    ], 'shop').map(v => v.key)).toEqual(['A', 'B']);
  });

  it('groups designer sources by unit (site first, plugins by id, order kept)', () => {
    const g = groupByUnit([{ name: 'shop:b' }, { name: 'events' }, { name: 'money:rates' }, { name: 'shop:a' }, { name: 'posts' }, { name: 'z', unit: 'auth' }]);
    expect(g.map(x => [x.unit, x.items.map(i => i.name)])).toEqual([
      ['site', ['events', 'posts']], ['auth', ['z']], ['money', ['money:rates']], ['shop', ['shop:b', 'shop:a']],
    ]);
    expect(groupByUnit([])).toEqual([]);
  });
});
