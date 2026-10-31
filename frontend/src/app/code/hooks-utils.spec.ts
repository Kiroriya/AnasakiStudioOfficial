import { CodeHook } from '../core/code-models';
import { groupHooks, hookTouchesUnit, hookUnits, normalizeHooks, orderedSubscribers, subscriberWhere } from './hooks-utils';

const sample: CodeHook[] = normalizeHooks([
  { name: 'shop.price', kind: 'filter', declaredBy: 'shop', subscribers: [{ unit: 'site', priority: 10, file: 'main.ts', line: 3 }] },
  { name: 'post.content', kind: 'filter', declaredBy: 'core', subscribers: [
    { unit: 'shop', priority: 10, file: 'code/main.ts', line: 9 }, { unit: 'site', priority: 5, file: 'lib/a.ts', line: 1 }, { unit: 'site', priority: 10 },
  ] },
  { name: 'shop.order.paid', kind: 'event', declaredBy: 'shop', subscribers: [] },
  { name: 'post.published', kind: 'event', declaredBy: 'core', subscribers: [{ unit: 'money', priority: 20 }] },
  { name: 'weird', kind: 'x', declaredBy: '', subscribers: null },
  { kind: 'filter' },
]);

describe('hooks list', () => {
  it('normalises the answer', () => {
    expect(sample.length).toBe(5);
    expect(sample[4]).toEqual({ name: 'weird', kind: 'x', declaredBy: 'core', subscribers: [] });
    expect(normalizeHooks({ items: [{ name: 'a', kind: 'event', declaredBy: 'site', subscribers: [{ unit: 'site' }] }] })[0].subscribers[0].priority).toBe(10);
    expect(normalizeHooks({ hooks: [{ name: 'b', kind: 'filter' }] }).map(h => h.name)).toEqual(['b']);
    expect(normalizeHooks(null)).toEqual([]);
  });

  it('groups filters, events, other; core first then by name', () => {
    const g = groupHooks(sample);
    expect(g.map(x => [x.kind, x.hooks.map(h => h.name)])).toEqual([
      ['filter', ['post.content', 'shop.price']], ['event', ['post.published', 'shop.order.paid']], ['other', ['weird']],
    ]);
  });

  it('filters by text and unit', () => {
    expect(groupHooks(sample, 'SHOP').flatMap(g => g.hooks.map(h => h.name))).toEqual(['post.content', 'shop.price', 'shop.order.paid']);
    expect(groupHooks(sample, 'lib/a').flatMap(g => g.hooks.map(h => h.name))).toEqual(['post.content']);
    expect(groupHooks(sample, '', 'money').flatMap(g => g.hooks.map(h => h.name))).toEqual(['post.published']);
    expect(groupHooks(sample, 'nothing-matches')).toEqual([]);
    expect(hookTouchesUnit(sample[0], 'shop')).toBe(true);
    expect(hookTouchesUnit(sample[0], '')).toBe(true);
  });

  it('subscribers in effective order (priority, then server order)', () => {
    const subs = orderedSubscribers(sample[1].subscribers);
    expect(subs.map(s => `${s.unit}@${s.priority}${s.file ? ':' + s.file : ''}`)).toEqual(['site@5:lib/a.ts', 'shop@10:code/main.ts', 'site@10']);
    expect(subscriberWhere(subs[0])).toBe('lib/a.ts:1');
    expect(subscriberWhere(subs[2])).toBe('');
  });

  it('units used in the list', () => {
    expect(hookUnits(sample)).toEqual(['site', 'money', 'shop']);
  });
});
