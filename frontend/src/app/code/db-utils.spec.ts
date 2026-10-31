import {
  ER, canRollBack, columnKind, dbExportQuery, dbRowsQuery, destructiveTarget, erLayout, formatCell, formatDateText, formatDestructive,
  isAutoColumn, isReadSql, migrationFileName, migrationSlug, migrationStatusClass, migrationTemplate, nextSort, normalizeBackups,
  normalizeMigrations, parseInput, parseSqlParams, pendingDestructive, sqlRows, sqlVerb, summarizeImport, toInputValue,
} from './db-utils';

describe('columnKind', () => {
  it('maps MySQL types', () => {
    expect(columnKind('bigint unsigned')).toBe('int');
    expect(columnKind('int(11)')).toBe('int');
    expect(columnKind('tinyint(1)')).toBe('bool');
    expect(columnKind('tinyint(4)')).toBe('int');
    expect(columnKind('decimal(12,2)')).toBe('decimal');
    expect(columnKind('double')).toBe('decimal');
    expect(columnKind('datetime(3)')).toBe('datetime');
    expect(columnKind('timestamp')).toBe('datetime');
    expect(columnKind('date')).toBe('date');
    expect(columnKind('json')).toBe('json');
    expect(columnKind("enum('a','b')")).toBe('text');
    expect(columnKind('varchar(255)')).toBe('text');
    expect(columnKind(undefined)).toBe('text');
  });
});

describe('formatCell', () => {
  it('shows NULL, booleans and numbers', () => {
    expect(formatCell(null)).toEqual({ text: 'NULL', kind: 'null' });
    expect(formatCell(undefined, 'int')).toEqual({ text: 'NULL', kind: 'null' });
    expect(formatCell(true)).toEqual({ text: 'true', kind: 'bool' });
    expect(formatCell(0, 'tinyint(1)')).toEqual({ text: 'false', kind: 'bool' });
    expect(formatCell(42, 'int')).toEqual({ text: '42', kind: 'number' });
  });

  it('keeps decimals and big integers exact', () => {
    expect(formatCell('12.50', 'decimal(12,2)')).toEqual({ text: '12.50', kind: 'number' });
    expect(formatCell('0.10', 'decimal(5,2)').text).toBe('0.10');
    expect(formatCell('9007199254740993', 'bigint unsigned')).toEqual({ text: '9007199254740993', kind: 'number' });
  });

  it('formats dates', () => {
    expect(formatCell('2026-10-24T05:06:07Z', 'datetime')).toEqual({ text: '2026-10-24 05:06:07', kind: 'date' });
    expect(formatCell('2026-10-24T05:06:07.120Z', 'datetime(3)').text).toBe('2026-10-24 05:06:07.120');
    expect(formatCell('2026-10-24T05:06:07.000Z', 'datetime(3)').text).toBe('2026-10-24 05:06:07');
    expect(formatCell('2026-10-24T00:00:00Z', 'date').text).toBe('2026-10-24');
    expect(formatCell('2026-10-24', 'date').text).toBe('2026-10-24');
    // ISO strings in untyped (SQL console) results are recognised too
    expect(formatCell('2026-10-24T05:06:07+07:00').text).toBe('2026-10-24 05:06:07 +07:00');
    expect(formatDateText('not a date')).toBe('not a date');
  });

  it('shows JSON compactly and shortens long values', () => {
    expect(formatCell({ a: 1, b: [1, 2] }, 'json')).toEqual({ text: '{"a":1,"b":[1,2]}', kind: 'json' });
    expect(formatCell([1], undefined).kind).toBe('json');
    const long = formatCell('x'.repeat(200), 'text', 50);
    expect(long.text.length).toBe(50);
    expect(long.text.endsWith('…')).toBe(true);
    expect(long.title).toBe('x'.repeat(200));
    expect(formatCell('hello', 'varchar(10)')).toEqual({ text: 'hello', kind: 'text' });
  });
});

describe('edit values', () => {
  it('fills edit boxes', () => {
    expect(toInputValue(null)).toEqual({ text: '', isNull: true });
    expect(toInputValue({ a: 1 })).toEqual({ text: '{"a":1}', isNull: false });
    expect(toInputValue(1, 'tinyint(1)')).toEqual({ text: 'true', isNull: false });
    expect(toInputValue('12.50', 'decimal(12,2)')).toEqual({ text: '12.50', isNull: false });
  });

  it('parses typed values', () => {
    expect(parseInput('', 'int', true)).toEqual({ ok: true, value: null });
    expect(parseInput(' 42 ', 'int')).toEqual({ ok: true, value: 42 });
    expect(parseInput('9007199254740993', 'bigint')).toEqual({ ok: true, value: '9007199254740993' });
    expect(parseInput('4.2', 'int')).toEqual({ ok: false, error: 'notInteger' });
    expect(parseInput('12.50', 'decimal(12,2)')).toEqual({ ok: true, value: '12.50' });
    expect(parseInput('abc', 'decimal(12,2)')).toEqual({ ok: false, error: 'notNumber' });
    expect(parseInput('TRUE', 'tinyint(1)')).toEqual({ ok: true, value: true });
    expect(parseInput('0', 'tinyint(1)')).toEqual({ ok: true, value: false });
    expect(parseInput('maybe', 'tinyint(1)')).toEqual({ ok: false, error: 'notBoolean' });
    expect(parseInput('{"a":[1]}', 'json')).toEqual({ ok: true, value: { a: [1] } });
    expect(parseInput('{a:1}', 'json')).toEqual({ ok: false, error: 'invalidJson' });
    expect(parseInput(' keep spaces ', 'varchar(20)')).toEqual({ ok: true, value: ' keep spaces ' });
    expect(parseInput('2026-10-24 10:00:00', 'datetime')).toEqual({ ok: true, value: '2026-10-24 10:00:00' });
  });

  it('knows auto-filled columns', () => {
    expect(isAutoColumn({ extra: 'auto_increment', default: null })).toBe(true);
    expect(isAutoColumn({ extra: 'DEFAULT_GENERATED on update CURRENT_TIMESTAMP(3)', default: 'CURRENT_TIMESTAMP(3)' })).toBe(true);
    expect(isAutoColumn({ extra: '', default: null })).toBe(false);
  });
});

describe('dbRowsQuery', () => {
  it('builds paging, search, sort and filters', () => {
    expect(dbRowsQuery({ page: 2, perPage: 50 })).toEqual({ page: '2', perPage: '50' });
    expect(dbRowsQuery({ page: 1, perPage: 25, q: '  ann ', sort: 'total', dir: 'desc', filters: { status: 'paid', note: '  ', code: 'A%' } }))
      .toEqual({ page: '1', perPage: '25', q: 'ann', sort: 'total', dir: 'desc', 'filter[code]': 'A%', 'filter[status]': 'paid' });
  });

  it('clamps paging and defaults the direction', () => {
    expect(dbRowsQuery({ page: 0, perPage: 500, sort: 'id' })).toEqual({ page: '1', perPage: '100', sort: 'id', dir: 'asc' });
  });

  it('export query has no paging', () => {
    expect(dbExportQuery({ q: 'x', filters: { a: '1' } })).toEqual({ q: 'x', 'filter[a]': '1' });
  });

  it('cycles sort asc → desc → off', () => {
    expect(nextSort({}, 'a')).toEqual({ sort: 'a', dir: 'asc' });
    expect(nextSort({ sort: 'a', dir: 'asc' }, 'a')).toEqual({ sort: 'a', dir: 'desc' });
    expect(nextSort({ sort: 'a', dir: 'desc' }, 'a')).toEqual({});
    expect(nextSort({ sort: 'a', dir: 'desc' }, 'b')).toEqual({ sort: 'b', dir: 'asc' });
  });
});

describe('summarizeImport', () => {
  it('sorts errors, puts file-level ones first and drops duplicates', () => {
    const s = summarizeImport({
      inserted: 0,
      errors: [
        { row: 5, message: 'total: not a number' }, { row: 2, message: 'code: required' }, { row: 0, message: 'unknown column "foo"' },
        { row: 2, message: 'code: required' }, { row: 2, message: 'total: not a number' },
      ],
    });
    expect(s.ok).toBe(false);
    expect(s.errors.map(e => [e.row, e.file])).toEqual([[0, true], [2, false], [2, false], [5, false]]);
    expect(s.badRows).toBe(2);
    expect(s.more).toBe(0);
  });

  it('caps the shown list', () => {
    const errors = Array.from({ length: 130 }, (_, i) => ({ row: 130 - i, message: 'bad' }));
    const s = summarizeImport({ inserted: 0, errors }, 100);
    expect(s.shown.length).toBe(100);
    expect(s.shown[0].row).toBe(1);
    expect(s.more).toBe(30);
  });

  it('reports a clean run', () => {
    expect(summarizeImport({ inserted: 12, errors: [] })).toEqual({ inserted: 12, errors: [], shown: [], more: 0, badRows: 0, ok: true });
    expect(summarizeImport(null).inserted).toBe(0);
  });
});

describe('migration files', () => {
  it('slugifies names', () => {
    expect(migrationSlug('Create Orders!')).toBe('create_orders');
    expect(migrationSlug('  add-note to   orders ')).toBe('add_note_to_orders');
    expect(migrationSlug('ไทย')).toBe('migration');
    expect(migrationSlug('a'.repeat(80)).length).toBe(60);
  });

  it('numbers new files after the highest existing one', () => {
    expect(migrationFileName([], 'create orders')).toBe('migrations/001_create_orders.ts');
    expect(migrationFileName(['main.ts', 'migrations/001_a.ts', 'migrations/007_b.js', 'migrations/readme.md', 'lib/010_x.ts'], 'add note'))
      .toBe('migrations/008_add_note.ts');
    expect(migrationFileName(['migrations/999_z.ts'], 'next')).toBe('migrations/1000_next.ts');
    expect(migrationFileName(['migrations/0042_z.ts'], 'next', 'js')).toBe('migrations/0043_next.js');
  });

  it('writes a template', () => {
    const c = migrationTemplate('create_orders');
    expect(c).toContain("schema.create('orders'");
    expect(c).toContain("schema.dropIfExists('orders')");
    expect(c).toContain('export default migration({');
    expect(migrationTemplate('create_order_items_table')).toContain("schema.create('order_items'");
    const alter = migrationTemplate('add_note');
    expect(alter).toContain('up(schema)');
    expect(alter).not.toContain('schema.create(');
  });
});

describe('normalizeMigrations', () => {
  const rows = [
    { name: '001_a', batch: 1, ranAt: '2026-10-01T00:00:00Z', ranBy: 'Admin', durationMs: 5, status: 'ran', error: null },
    { name: '002_b', batch: 2, ranAt: '2026-10-02T00:00:00Z', ranBy: 'Admin', durationMs: 7, status: 'ran', error: null },
    { name: '003_c', batch: 3, ranAt: '2026-10-03T00:00:00Z', ranBy: 'Admin', durationMs: 1, status: 'failed', error: 'Duplicate column' },
  ];

  it('reads an object envelope with pending names', () => {
    const v = normalizeMigrations({ items: rows, pending: [{ name: '004_d', source: 'draft' }, '003_c', { name: '004_d', source: 'draft' }] });
    expect(v.history.map(h => h.name)).toEqual(['003_c', '002_b', '001_a']);
    expect(v.pending).toEqual([{ name: '003_c' }, { name: '004_d', source: 'draft' }]);
    expect(v.latestBatch).toBe(2);
    expect(canRollBack(v.history[1], v.latestBatch)).toBe(true);
    expect(canRollBack(v.history[0], v.latestBatch)).toBe(false); // failed
    expect(canRollBack(v.history[2], v.latestBatch)).toBe(false); // older batch
  });

  it('reads a plain array and pending grouped by source', () => {
    const v = normalizeMigrations([...rows, { name: '005_e', status: 'pending' }]);
    expect(v.pending).toEqual([{ name: '005_e', source: undefined }]);
    const w = normalizeMigrations({ history: rows, pending: { draft: ['006_f'], live: ['005_e'] } });
    expect(w.pending).toEqual([{ name: '005_e', source: 'live' }, { name: '006_f', source: 'draft' }]);
    expect(normalizeMigrations(null)).toEqual({ history: [], pending: [], latestBatch: null });
  });

  it('skips rolled back migrations for the latest batch', () => {
    const v = normalizeMigrations([{ ...rows[0] }, { ...rows[1], status: 'rolled_back' }]);
    expect(v.latestBatch).toBe(1);
  });

  it('maps statuses to badges', () => {
    expect(migrationStatusClass('ran')).toBe('badge-live');
    expect(migrationStatusClass('failed')).toBe('badge-danger');
    expect(migrationStatusClass('rolled_back')).toBe('badge-warn');
    expect(migrationStatusClass('weird')).toBe('');
  });

  it('normalises backups', () => {
    expect(normalizeBackups([
      { id: 1, table_name: 'orders', backup_table: 'bak_orders_20261001000000', migration: '002_b', created_at: '2026-10-01T00:00:00Z', expires_at: '2026-10-08T00:00:00Z' },
      { id: 2, tableName: 'items', backupTable: 'bak_items_20261002000000', migration: '003_c', createdAt: '2026-10-02T00:00:00Z', expiresAt: '2026-10-09T00:00:00Z' },
    ]).map(b => [b.id, b.tableName, b.backupTable])).toEqual([[2, 'items', 'bak_items_20261002000000'], [1, 'orders', 'bak_orders_20261001000000']]);
    expect(normalizeBackups({})).toEqual([]);
  });
});

describe('destructive changes', () => {
  const labels: Record<string, string> = { drop_table: 'Drop table', drop_column: 'Drop column' };
  const label = (op: string) => labels[op] ?? null;

  it('formats and groups by migration', () => {
    const g = formatDestructive([
      { migration: '005_b', op: 'drop_column', table: 'orders', column: 'note' },
      { migration: '004_a', op: 'drop_table', table: 'tmp' },
      { migration: '005_b', op: 'rename_column', table: 'orders', column: 'total' },
      { migration: '005_b', op: 'drop_column', table: 'orders', column: 'note' },
    ], label);
    expect(g.map(x => x.migration)).toEqual(['004_a', '005_b']);
    expect(g[0].items.map(i => i.text)).toEqual(['Drop table: tmp']);
    expect(g[1].items.map(i => i.text)).toEqual(['Drop column: orders.note', 'rename_column: orders.total']);
    expect(destructiveTarget({ op: 'x', table: 't', column: null })).toBe('t');
    expect(formatDestructive(null, label)).toEqual([]);
  });

  it('collects the destructive items of a dry run', () => {
    const list = pendingDestructive([
      { name: '001_a', destructive: [] },
      { name: '002_b', destructive: [{ op: 'drop_table', table: 'x' }] },
      { name: '003_c' },
    ]);
    expect(list).toEqual([{ op: 'drop_table', table: 'x', migration: '002_b' }]);
    expect(formatDestructive(list, label)[0].migration).toBe('002_b');
  });
});

describe('SQL console helpers', () => {
  it('parses params', () => {
    expect(parseSqlParams('')).toEqual({ ok: true, params: [] });
    expect(parseSqlParams(' [1, "paid", null] ')).toEqual({ ok: true, params: [1, 'paid', null] });
    expect(parseSqlParams('{"a":1}')).toEqual({ ok: false });
    expect(parseSqlParams('1, 2')).toEqual({ ok: false });
  });

  it('finds the statement verb', () => {
    expect(sqlVerb('  -- note\n/* x */ SELECT 1')).toBe('select');
    expect(sqlVerb('# c\nupdate t set a=1')).toBe('update');
    expect(isReadSql('WITH x AS (SELECT 1) SELECT * FROM x')).toBe(true);
    expect(isReadSql('DELETE FROM t')).toBe(false);
  });

  it('turns rows into arrays', () => {
    expect(sqlRows({ columns: ['a', 'b'], rows: [{ a: 1, b: 2 }, [3, 4]] })).toEqual({ columns: ['a', 'b'], rows: [[1, 2], [3, 4]] });
    expect(sqlRows({ rows: [{ x: 1 }] })).toEqual({ columns: ['x'], rows: [[1]] });
    expect(sqlRows(null)).toEqual({ columns: [], rows: [] });
  });
});

describe('erLayout', () => {
  const tables = [
    { name: 'orders', columns: [{ name: 'id', type: 'bigint', key: 'PRI' }, { name: 'user_id', type: 'bigint' }, { name: 'total', type: 'decimal' }] },
    { name: 'order_items', columns: [{ name: 'id', key: 'PRI' }, { name: 'order_id' }, { name: 'product_id' }] },
    { name: 'products', columns: [{ name: 'id', key: 'PRI' }, { name: 'name' }] },
  ];
  const links = [
    { from: 'order_items', fromColumn: 'order_id', to: 'orders', toColumn: 'id' },
    { from: 'order_items', fromColumn: 'product_id', to: 'products', toColumn: 'id' },
    { from: 'orders', fromColumn: 'user_id', to: 'core_users', toColumn: 'id', meta: true },
  ];

  it('places referenced tables to the left of the tables that reference them', () => {
    const l = erLayout(tables, links);
    const n = (name: string) => l.nodes.find(x => x.name === name)!;
    expect(n('core_users').layer).toBe(0);
    expect(n('products').layer).toBe(0);
    expect(n('orders').layer).toBe(1);
    expect(n('order_items').layer).toBe(2);
    expect(n('order_items').x).toBeGreaterThan(n('orders').x);
    // same column: sorted by name, stacked without overlap
    expect(n('core_users').y).toBeLessThan(n('products').y);
    expect(n('products').y).toBeGreaterThanOrEqual(n('core_users').y + n('core_users').h);
    expect(l.width).toBe(ER.margin * 2 + 3 * ER.width + 2 * ER.gapX);
    expect(l.height).toBeGreaterThan(0);
  });

  it('adds referenced core_* views as greyed nodes with the referenced columns', () => {
    const l = erLayout(tables, links);
    const core = l.nodes.find(x => x.name === 'core_users')!;
    expect(core.core).toBe(true);
    expect(core.columns.map(c => c.name)).toEqual(['id']);
    expect(l.nodes.find(x => x.name === 'orders')!.core).toBe(false);
  });

  it('draws edges from the referencing column to the referenced column', () => {
    const l = erLayout(tables, links);
    const e = l.edges.find(x => x.from === 'order_items' && x.to === 'orders')!;
    const items = l.nodes.find(x => x.name === 'order_items')!, orders = l.nodes.find(x => x.name === 'orders')!;
    expect(e.x1).toBe(items.x);
    expect(e.x2).toBe(orders.x + orders.w);
    expect(e.y1).toBe(items.y + ER.head + ER.row * 1 + ER.row / 2);
    expect(e.y2).toBe(orders.y + ER.head + ER.row / 2);
    expect(e.d.startsWith(`M${e.x1},${e.y1} C`)).toBe(true);
    expect(e.meta).toBe(false);
    expect(l.edges.find(x => x.to === 'core_users')!.meta).toBe(true);
    expect(items.columns.find(c => c.name === 'order_id')!.fk).toBe(true);
    expect(orders.columns.find(c => c.name === 'id')!.pk).toBe(true);
  });

  it('survives cycles and self references', () => {
    const l = erLayout(
      [{ name: 'a', columns: [{ name: 'id' }, { name: 'b_id' }, { name: 'parent_id' }] }, { name: 'b', columns: [{ name: 'id' }, { name: 'a_id' }] }],
      [{ from: 'a', fromColumn: 'b_id', to: 'b', toColumn: 'id' }, { from: 'b', fromColumn: 'a_id', to: 'a', toColumn: 'id' }, { from: 'a', fromColumn: 'parent_id', to: 'a', toColumn: 'id' }],
    );
    expect(l.nodes.length).toBe(2);
    expect(l.edges.length).toBe(3);
    const self = l.edges.find(e => e.from === 'a' && e.to === 'a')!;
    expect(self.x1).toBe(self.x2);
  });

  it('limits shown columns and handles an empty schema', () => {
    const many = { name: 'wide', columns: Array.from({ length: 20 }, (_, i) => ({ name: 'c' + i })) };
    const node = erLayout([many], []).nodes[0];
    expect(node.columns.length).toBe(ER.maxColumns);
    expect(node.more).toBe(8);
    expect(node.h).toBe(ER.head + ER.row * (ER.maxColumns + 1) + ER.pad);
    expect(erLayout([], [])).toEqual({ nodes: [], edges: [], width: 0, height: 0 });
  });
});
