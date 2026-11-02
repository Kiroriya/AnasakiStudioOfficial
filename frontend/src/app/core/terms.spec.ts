import { termTree } from './terms';
import { Term } from './models';

const t = (id: number, name: string, parent = 0): Term => ({ id, name, parent, slug: name.toLowerCase(), taxonomy: 'genre', count: 0 });

describe('termTree', () => {
  it('puts children under their parent with an indent', () => {
    const rows = termTree([t(1, 'Rock'), t(2, 'Jazz'), t(3, 'Indie', 1), t(4, 'Lo-fi', 3)]);
    expect(rows.map(r => r.indent + r.term.name)).toEqual(['Rock', '— Indie', '— — Lo-fi', 'Jazz']);
  });
  it('shows orphans at the top level and survives a cycle', () => {
    expect(termTree([t(1, 'A', 9)]).map(r => r.depth)).toEqual([0]);
    expect(termTree([t(1, 'A', 2), t(2, 'B', 1)])).toEqual([]);
  });
});
