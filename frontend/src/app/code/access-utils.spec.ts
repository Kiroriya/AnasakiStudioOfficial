import { CodeAccessRow } from '../core/code-models';
import { applyOverride, cellState, defaultRoles, groupAccess, groupColumns, groupName, isGroupEntry, isOverridden, normRoles, overrideFor, roleColumns, sameRoles, toggleRole } from './access-utils';

const row = (kind: string, name: string, codeDefault: string[] | null, override: string[] | null, effective: string[]): CodeAccessRow =>
  ({ kind: kind as CodeAccessRow['kind'], name, codeDefault, override, effective });

const ROWS: CodeAccessRow[] = [
  row('action', 'book', null, null, ['administrator']),
  row('collection', 'events', ['*'], null, ['*']),
  row('route', 'GET /api/x', ['administrator'], ['administrator', 'editor'], ['administrator', 'editor']),
  row('table', 'orders', null, null, []),
  row('collection', 'agenda', null, ['member'], ['member']),
  row('page', '/checkout', null, null, ['*']),
  row('postType', 'event', null, null, ['administrator']),
];

describe('access defaults and comparison', () => {
  it('code default, else administrator (page: everyone, table: nobody)', () => {
    expect(defaultRoles('collection', ['*', 'editor'])).toEqual(['*', 'editor']);
    expect(defaultRoles('action', null)).toEqual(['administrator']);
    expect(defaultRoles('page', null)).toEqual(['*']);
    expect(defaultRoles('table', null)).toEqual([]);
  });
  it('normalises role lists', () => {
    expect(normRoles(['editor', '*', 'author', 'editor', ''])).toEqual(['*', 'author', 'editor']);
    expect(sameRoles(['b', 'a'], ['a', 'b', 'a'])).toBe(true);
    expect(sameRoles(['a'], ['a', '*'])).toBe(false);
    expect(sameRoles(null, [])).toBe(true);
  });
});

describe('grouping', () => {
  it('groups by kind in a fixed order, sorted by name, unknown kinds last', () => {
    const g = groupAccess(ROWS);
    expect(g.map(x => x.kind)).toEqual(['route', 'collection', 'action', 'page', 'table', 'postType']);
    expect(g[1].rows.map(r => r.name)).toEqual(['agenda', 'events']);
  });
  it('filters by name', () => {
    expect(groupAccess(ROWS, 'EVE').map(x => [x.kind, x.rows.map(r => r.name)])).toEqual([['collection', ['events']], ['postType', ['event']]]);
    expect(groupAccess([], '')).toEqual([]);
  });
  it('role columns: everyone, administrator, known roles, then slugs from the rows', () => {
    expect(roleColumns([{ slug: 'administrator' }, { slug: 'editor' }, { slug: 'subscriber' }], ROWS)).toEqual(['*', 'administrator', 'editor', 'subscriber', 'member']);
  });
});

describe('checkboxes and overrides', () => {
  it('cell state: administrator locked on, roles implied by everyone', () => {
    const ev = ROWS[1], book = ROWS[0];
    expect(cellState(book, 'administrator')).toEqual({ checked: true, locked: true, implied: false });
    expect(cellState(book, '*')).toEqual({ checked: false, locked: false, implied: false });
    expect(cellState(book, 'editor')).toEqual({ checked: false, locked: false, implied: false });
    expect(cellState(ev, 'editor')).toEqual({ checked: true, locked: true, implied: true });
    expect(cellState(ev, '*').checked).toBe(true);
  });
  it('toggling builds the override; matching the code default sends null', () => {
    const book = ROWS[0];
    const on = toggleRole(book.effective, 'editor', true);
    expect(on).toEqual(['administrator', 'editor']);
    expect(overrideFor(book, on)).toEqual(['administrator', 'editor']);
    // unticking it again = back to the code default → remove the override
    const route = ROWS[2];
    expect(overrideFor(route, toggleRole(route.effective, 'editor', false))).toBeNull();
    // everyone on a table (default nobody)
    expect(overrideFor(ROWS[3], toggleRole([], '*', true))).toEqual(['*']);
    expect(overrideFor(ROWS[3], [])).toBeNull();
    // page default is everyone
    expect(overrideFor(ROWS[5], toggleRole(['*'], '*', false))).toEqual([]);
  });
  it('applies an override locally (null = back to the default)', () => {
    const r = applyOverride(ROWS[0], ['administrator', 'editor']);
    expect(isOverridden(r)).toBe(true);
    expect(r.effective).toEqual(['administrator', 'editor']);
    const back = applyOverride(ROWS[2], null);
    expect(isOverridden(back)).toBe(false);
    expect(back.effective).toEqual(['administrator']);
    expect(applyOverride(ROWS[3], null).effective).toEqual([]);
  });
});

describe('visitor group columns (phase 4a)', () => {
  const rows: CodeAccessRow[] = [
    row('collection', 'members', ['group:member', 'editor'], null, ['group:member', 'editor']),
    row('action', 'vote', null, ['group:gold', 'group:vip'], ['group:gold', 'group:vip']),
    row('page', '/club', ['*'], null, ['*']),
  ];
  it('groups are not role columns; declared groups first, then groups found in rows', () => {
    expect(roleColumns([{ slug: 'administrator' }, { slug: 'editor' }], rows)).toEqual(['*', 'administrator', 'editor']);
    expect(groupColumns(['gold', 'member', 'bad name', 'gold'], rows)).toEqual(['group:gold', 'group:member', 'group:vip']);
    expect(groupColumns(null, [])).toEqual([]);
    expect(isGroupEntry('group:gold')).toBe(true);
    expect(isGroupEntry('editor')).toBe(false);
    expect(groupName('group:gold')).toBe('gold');
  });
  it('group cells tick like roles, implied by everyone', () => {
    expect(cellState(rows[0], 'group:member')).toEqual({ checked: true, locked: false, implied: false });
    expect(cellState(rows[0], 'group:gold')).toEqual({ checked: false, locked: false, implied: false });
    expect(cellState(rows[2], 'group:gold')).toEqual({ checked: true, locked: true, implied: true });
  });
  it('ticking a group builds the override with group: entries', () => {
    const on = toggleRole(rows[0].effective, 'group:gold', true);
    expect(on).toEqual(['editor', 'group:gold', 'group:member']);
    expect(overrideFor(rows[0], on)).toEqual(['editor', 'group:gold', 'group:member']);
    expect(overrideFor(rows[0], toggleRole(on, 'group:gold', false))).toBeNull();
  });
});

describe('admin pages (phase 6)', () => {
  it('groups adminPage after tables and before unknown kinds', () => {
    const g = groupAccess([...ROWS, row('adminPage', 'reports', null, null, ['administrator'])]);
    expect(g.map(x => x.kind)).toEqual(['route', 'collection', 'action', 'page', 'table', 'adminPage', 'postType']);
    expect(defaultRoles('adminPage', null)).toEqual(['administrator']);
  });
  it('locks "Everyone" for admin pages unless it is already on', () => {
    expect(cellState(row('adminPage', 'r', null, null, ['administrator']), '*')).toEqual({ checked: false, locked: true, implied: false });
    expect(cellState(row('adminPage', 'r', ['*'], null, ['*']), '*')).toEqual({ checked: true, locked: false, implied: false });
    expect(cellState(row('adminPage', 'r', null, ['editor'], ['editor']), 'editor')).toEqual({ checked: true, locked: false, implied: false });
    expect(overrideFor(row('adminPage', 'r', null, null, ['administrator']), ['administrator', 'editor'])).toEqual(['administrator', 'editor']);
  });
});
