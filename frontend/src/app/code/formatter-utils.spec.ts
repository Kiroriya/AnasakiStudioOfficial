import { CodeFormatter } from '../core/code-models';
import {
  aliasConflict, aliasEnabled, aliasesOf, groupFormatters, isPluginFormatter, normalizeFormatters, shortFormatterName, suggestFormatterNames, toggleAlias,
} from './formatter-utils';

const LIST: CodeFormatter[] = normalizeFormatters([
  { name: 'thai:baht', unit: 'thai', alias: 'baht' },
  { name: 'thai:thaiDate', unit: 'thai' },
  { name: 'shop:baht', unit: 'shop' },
  { name: 'shop:price', unit: 'shop' },
  { name: 'price', unit: 'site' },
  { name: 'thai:date', unit: 'thai' },
]);

describe('normalizeFormatters', () => {
  it('accepts arrays, envelopes and strings; fills unit; drops bad names', () => {
    expect(normalizeFormatters({ items: ['thai:baht', 'x'] })).toEqual([{ name: 'x', unit: 'site' }, { name: 'thai:baht', unit: 'thai' }]);
    expect(normalizeFormatters({ formatters: [{ name: 'a:b:c' }, { name: '1x' }, { name: 'ok' }, { name: 'ok' }, null, 5] })).toEqual([{ name: 'ok', unit: 'site' }]);
    expect(normalizeFormatters(null)).toEqual([]);
    expect(normalizeFormatters([{ name: 'thai:baht', conflict: 'site formatter baht', alias: '' }])).toEqual([{ name: 'thai:baht', unit: 'thai', conflict: 'site formatter baht' }]);
  });
  it('sorts site first, then unit and name', () => {
    expect(LIST.map(f => f.name)).toEqual(['price', 'shop:baht', 'shop:price', 'thai:baht', 'thai:date', 'thai:thaiDate']);
  });
  it('helpers', () => {
    expect(shortFormatterName('thai:baht')).toBe('baht');
    expect(shortFormatterName('price')).toBe('price');
    expect(isPluginFormatter({ name: 'thai:baht', unit: 'thai' })).toBe(true);
    expect(isPluginFormatter({ name: 'price', unit: 'site' })).toBe(false);
    expect(groupFormatters(LIST).map(g => [g.unit, g.formatters.length])).toEqual([['site', 1], ['shop', 2], ['thai', 3]]);
  });
});

describe('aliases and conflicts', () => {
  const aliases = aliasesOf(LIST);
  const f = (n: string) => LIST.find(x => x.name === n)!;
  it('reads enabled aliases from the rows', () => {
    expect(aliases).toEqual({ baht: 'thai:baht' });
    expect(aliasEnabled(f('thai:baht'), aliases)).toBe(true);
    expect(aliasEnabled(f('shop:baht'), aliases)).toBe(false);
  });
  it('core wins, then the site code, then the alias enabled first', () => {
    expect(aliasConflict(f('thai:date'), LIST, aliases)).toEqual({ kind: 'core', by: 'date' });
    expect(aliasConflict(f('shop:price'), LIST, aliases)).toEqual({ kind: 'site', by: 'price' });
    expect(aliasConflict(f('shop:baht'), LIST, aliases)).toEqual({ kind: 'plugin', by: 'thai:baht' });
    expect(aliasConflict(f('thai:baht'), LIST, aliases)).toBeNull();
    expect(aliasConflict(f('thai:thaiDate'), LIST, aliases)).toBeNull();
  });
  it('toggles without stealing or shadowing', () => {
    expect(toggleAlias(aliases, f('thai:thaiDate'), true, LIST)).toEqual({ baht: 'thai:baht', thaiDate: 'thai:thaiDate' });
    expect(toggleAlias(aliases, f('shop:baht'), true, LIST)).toBe(aliases);
    expect(toggleAlias(aliases, f('shop:price'), true, LIST)).toBe(aliases);
    expect(toggleAlias(aliases, f('thai:date'), true, LIST)).toBe(aliases);
    expect(toggleAlias(aliases, f('shop:baht'), false, LIST)).toEqual(aliases); // not its alias: unchanged
    expect(toggleAlias(aliases, f('thai:baht'), false, LIST)).toEqual({});
    expect(toggleAlias({}, f('shop:baht'), true, LIST)).toEqual({ baht: 'shop:baht' });
    expect(toggleAlias({}, f('price'), true, LIST)).toEqual({});
  });
});

describe('designer autocomplete names', () => {
  it('lists site, plugin and enabled alias names with their origin', () => {
    const r = suggestFormatterNames(['price', 'number', 'legacy'], LIST);
    expect(r.names).toEqual(['price', 'legacy', 'shop:baht', 'shop:price', 'thai:baht', 'thai:date', 'thai:thaiDate', 'baht']);
    expect(r.detail['thai:baht']).toBe('thai');
    expect(r.detail['baht']).toBe('→ thai:baht');
    expect(r.detail['price']).toBe('site');
  });
  it('leaves out aliases blocked by a core or site formatter', () => {
    const list = normalizeFormatters([{ name: 'shop:price', alias: 'price' }, { name: 'price' }]);
    expect(suggestFormatterNames(null, list).names).toEqual(['price', 'shop:price']);
    expect(suggestFormatterNames(null, []).names).toEqual([]);
  });
});
