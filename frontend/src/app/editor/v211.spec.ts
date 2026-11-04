import { fontStack } from './design-css';
import { DNode, dedupeIds } from './design-types';

const n = (id: string, children?: DNode[]): DNode =>
  ({ id, type: 'frame', width: { mode: 'hug' }, height: { mode: 'hug' }, ...(children ? { children } : {}) } as DNode);

describe('dedupeIds (2.1.1)', () => {
  it('keeps a tree with unique ids as it is', () => {
    const t = n('root', [n('a'), n('b', [n('c')])]);
    const r = dedupeIds(t);
    expect(r.renamed).toBe(0);
    expect(r.root).toBe(t);
  });

  it('renames every repeat after the first, nested ones too', () => {
    const t = n('root', [n('imin'), n('b', [n('imin'), n('imin')])]);
    const r = dedupeIds(t);
    expect(r.renamed).toBe(2);
    const ids = [r.root.children![0].id, r.root.children![1].children![0].id, r.root.children![1].children![1].id];
    expect(ids[0]).toBe('imin');
    expect(new Set(ids).size).toBe(3);
    expect(t.children![1].children![0].id).toBe('imin'); // the input is not mutated
  });
});

describe('fontStack (2.1.1, mirrors FontStack in css.go)', () => {
  it('keeps comma lists and leaves generic families bare', () => {
    expect(fontStack('Kanit')).toBe("'Kanit', sans-serif");
    expect(fontStack('ui-sans-serif, system-ui, sans-serif')).toBe('ui-sans-serif, system-ui, sans-serif');
    expect(fontStack("'Noto Sans Thai', Segoe UI, serif")).toBe("'Noto Sans Thai', 'Segoe UI', serif");
    expect(fontStack("x';}body{color:red")).toBe("'xbodycolorred', sans-serif");
    expect(fontStack(' , ')).toBe('');
  });
});
