import type { DNode } from './design-types';
import { LruCache, deepEqual, normalizeBlocks, placeInBox, replaceInRichHtml, resizedBox, richText, selectionRelatives, shareTree } from './editor-utils';
import { siteRoleIds } from './site-role';

const n = (id: string, children?: DNode[], extra: Partial<DNode> = {}): DNode =>
  ({ id, type: 'frame', width: { mode: 'hug' }, height: { mode: 'hug' }, ...(children ? { children } : {}), ...extra } as DNode);

describe('shareTree', () => {
  const tree = () => n('root', [n('a', [n('a1'), n('a2')]), n('b')]);

  it('returns prev itself when nothing changed', () => {
    const prev = tree();
    expect(shareTree(prev, structuredClone(prev))).toBe(prev);
  });

  it('reuses unchanged subtrees and copies the changed path', () => {
    const prev = tree();
    const next = structuredClone(prev);
    next.children![0].children![1].x = 10;
    const out = shareTree(prev, next);
    expect(out).not.toBe(prev);
    expect(out.children![1]).toBe(prev.children![1]); // b untouched
    expect(out.children![0].children![0]).toBe(prev.children![0].children![0]); // a1 untouched
    expect(out.children![0].children![1].x).toBe(10);
    expect(prev.children![0].children![1].x).toBeUndefined();
  });

  it('copy mode never hands out the working objects', () => {
    const prev = tree();
    const work = structuredClone(prev);
    work.children![1].y = 5;
    const out = shareTree(prev, work, true);
    expect(out.children![1]).not.toBe(work.children![1]);
    work.children![1].y = 99; // keep editing the working copy
    expect(out.children![1].y).toBe(5);
  });

  it('handles reorder and removal by id', () => {
    const prev = tree();
    const next = structuredClone(prev);
    next.children = [next.children![1]];
    const out = shareTree(prev, next);
    expect(out.children!.length).toBe(1);
    expect(out.children![0]).toBe(prev.children![1]);
  });

  it('deepEqual ignores undefined keys', () => {
    expect(deepEqual({ a: 1, b: undefined }, { a: 1 })).toBe(true);
    expect(deepEqual([1, { x: 2 }], [1, { x: 3 }])).toBe(false);
  });
});

describe('selectionRelatives', () => {
  it('finds ancestors and layers inside the selection', () => {
    const root = n('root', [n('a', [n('a1', [n('a1x')]), n('a2')]), n('b')]);
    const r = selectionRelatives(root, ['a1']);
    expect([...r.ancestors].sort()).toEqual(['a', 'root']);
    expect([...r.inside]).toEqual(['a1x']);
    expect(selectionRelatives(root, []).ancestors.size).toBe(0);
  });
});

describe('LruCache', () => {
  it('drops the least recently used entry over the cap', () => {
    const c = new LruCache<string, number>(2);
    c.set('a', 1);
    c.set('b', 2);
    c.get('a'); // a is now newest
    c.set('c', 3);
    expect(c.has('b')).toBe(false);
    expect(c.has('a')).toBe(true);
    expect(c.entriesNewestFirst().map(e => e[0])).toEqual(['c', 'a']);
  });
});

describe('placeInBox', () => {
  it('without rotation puts the box at its centre', () => {
    const [p] = placeInBox([{ x: 0, y: 0 }], 10, 20, 105, 210);
    expect(p).toEqual({ x: 100, y: 200 });
  });
  it('rotates clockwise around the centre like CSS', () => {
    const [p] = placeInBox([{ x: 10, y: 5 }], 10, 10, 0, 0, 90); // right-middle → bottom-middle
    expect(p.x).toBeCloseTo(0);
    expect(p.y).toBeCloseTo(5);
  });
  it('flips', () => {
    const [p] = placeInBox([{ x: 0, y: 0 }], 10, 10, 5, 5, 0, true);
    expect(p).toEqual({ x: 10, y: 0 });
  });
});

describe('resizedBox', () => {
  it('keeps the opposite edge on n / w handles', () => {
    expect(resizedBox('nw', 10, 10, 100, 50, 120, 60, false)).toEqual({ x: -10, y: 0 });
  });
  it('Shift on a side handle grows the other axis from its centre', () => {
    expect(resizedBox('n', 0, 0, 100, 100, 120, 120, false)).toEqual({ x: -10, y: -20 });
    expect(resizedBox('e', 0, 0, 100, 100, 120, 120, false)).toEqual({ x: 0, y: -10 });
  });
  it('Alt resizes from the centre', () => {
    expect(resizedBox('se', 0, 0, 100, 100, 140, 120, true)).toEqual({ x: -20, y: -10 });
  });
});

describe('rich text replace', () => {
  it('replaces inside text runs and keeps formatting', () => {
    const html = 'Hello <b>world</b>, <a href="/w">world</a>!';
    expect(replaceInRichHtml(html, /world/g, 'there')).toBe('Hello <b>there</b>, <a href="/w">there</a>!');
    expect(replaceInRichHtml(html, /xyz/g, 'q')).toBe(html);
    expect(richText(html)).toBe('Hello world, world!');
  });
});

describe('siteRoleIds', () => {
  const cs = [{ id: 1, name: 'header' }, { id: 2, name: 'My top bar', siteRole: 'header' }, { id: 3, name: 'ส่วนท้ายเว็บ' }];
  it('prefers the stored role, falls back to the name', () => {
    expect(siteRoleIds(cs)).toEqual({ header: 2, footer: 3 });
    expect(siteRoleIds(cs, { site_footer_component: '1' })).toEqual({ header: 2, footer: 1 });
    expect(siteRoleIds([{ id: 1, name: ' Header ' }])).toEqual({ header: 1, footer: null });
  });
});

describe('normalizeBlocks', () => {
  it('stringifies select option values and keeps label objects', () => {
    const b = normalizeBlocks([
      { plugin: 'forms', pluginName: 'Forms', type: 'form', name: 'Form', icon: '📝', props: [
        { key: 'formId', type: 'select', label: 'Form', options: [{ value: 3, label: { th: 'ติดต่อ', en: 'Contact' } }, { value: 'x' }] },
        { key: 'title', type: 'text', label: 'Title' },
      ] },
      { nope: true },
    ]);
    expect(b.length).toBe(1);
    expect(b[0].props[0].options).toEqual([{ value: '3', label: { th: 'ติดต่อ', en: 'Contact' } }, { value: 'x', label: 'x' }]);
    expect(b[0].props[1].type).toBe('text');
    expect(normalizeBlocks(null)).toEqual([]);
  });
});
