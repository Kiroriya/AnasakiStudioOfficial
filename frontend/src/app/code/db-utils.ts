import {
  CodeDbBackup, CodeDbColumn, CodeDbImportResult, CodeDbMigration, CodeDbPendingInfo, CodeDbRow, CodeDbSqlResult, CodeDestructive,
} from '../core/code-models';

/** Pure helpers of the Site Code database screens (no Angular) — unit tested in db-utils.spec.ts. */

// ------------------------------------------------------------------ values

export type ColumnKind = 'int' | 'decimal' | 'bool' | 'date' | 'datetime' | 'json' | 'text';

/** What kind of value a MySQL column type holds (`decimal(12,2)`, `tinyint(1)`, `bigint unsigned`…). */
export function columnKind(type: string | undefined | null): ColumnKind {
  const t = (type ?? '').toLowerCase().trim();
  if (/^tinyint\(1\)(?!\d)|^bool(ean)?\b/.test(t)) return 'bool';
  if (/^(tiny|small|medium|big)?int\b|^integer\b/.test(t)) return 'int';
  if (/^(decimal|numeric|float|double|real)\b/.test(t)) return 'decimal';
  if (/^(datetime|timestamp)\b/.test(t)) return 'datetime';
  if (/^date\b/.test(t)) return 'date';
  if (/^json\b/.test(t)) return 'json';
  return 'text';
}

export type CellKind = 'null' | 'bool' | 'number' | 'date' | 'json' | 'text';
export interface CellView { text: string; kind: CellKind; /** Full text when `text` was shortened. */ title?: string }

const ISO_DT = /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})(\.\d+)?(Z|[+-]\d{2}:?\d{2})?$/;

/**
 * How a value from the API is shown in a grid cell. The API sends DECIMAL as an exact string
 * (kept as is), DATE/DATETIME as ISO strings, JSON parsed, BOOLEAN as true/false, big BIGINTs as
 * strings. Long text/JSON is cut to `max` characters (the full text goes to `title`).
 */
export function formatCell(value: unknown, type?: string, max = 120): CellView {
  if (value === null || value === undefined) return { text: 'NULL', kind: 'null' };
  const kind = columnKind(type);
  let v: CellView;
  if (typeof value === 'boolean') v = { text: value ? 'true' : 'false', kind: 'bool' };
  else if (kind === 'bool' && (value === 0 || value === 1)) v = { text: value ? 'true' : 'false', kind: 'bool' };
  else if (typeof value === 'number' || typeof value === 'bigint') v = { text: String(value), kind: 'number' };
  else if (typeof value === 'object') v = { text: JSON.stringify(value), kind: 'json' };
  else {
    const s = String(value);
    if (kind === 'int' || kind === 'decimal') v = { text: s, kind: 'number' };
    else if (kind === 'date' || kind === 'datetime' || ISO_DT.test(s)) v = { text: formatDateText(s, kind === 'date'), kind: 'date' };
    else if (kind === 'json') v = { text: s, kind: 'json' };
    else v = { text: s, kind: 'text' };
  }
  if (v.text.length > max) return { ...v, text: v.text.slice(0, max - 1) + '…', title: v.text };
  return v;
}

/** `2026-10-24T05:06:07.000Z` → `2026-10-24 05:06:07` (fraction kept unless zero; dateOnly → `2026-10-24`). */
export function formatDateText(s: string, dateOnly = false): string {
  const m = ISO_DT.exec(s);
  if (!m) return s;
  if (dateOnly && (m[2] === '00:00:00' || !m[2])) return m[1];
  const frac = m[3] && !/^\.0+$/.test(m[3]) ? m[3] : '';
  const tz = m[4] && m[4] !== 'Z' && !/^[+-]00:?00$/.test(m[4]) ? ' ' + m[4] : '';
  return `${m[1]} ${m[2]}${frac}${tz}`;
}

/** Text put into an edit box for a value (null → '' and the NULL box ticked). */
export function toInputValue(value: unknown, type?: string): { text: string; isNull: boolean } {
  if (value === null || value === undefined) return { text: '', isNull: true };
  if (typeof value === 'boolean') return { text: value ? 'true' : 'false', isNull: false };
  if (columnKind(type) === 'bool' && (value === 0 || value === 1)) return { text: value ? 'true' : 'false', isNull: false };
  if (typeof value === 'object') return { text: JSON.stringify(value), isNull: false };
  return { text: String(value), isNull: false };
}

export type ParseError = 'notNumber' | 'notInteger' | 'notBoolean' | 'invalidJson';
export type ParsedInput = { ok: true; value: unknown } | { ok: false; error: ParseError };

/**
 * Edit box text → value sent to the API. Integers become numbers when safe (else stay strings,
 * like the API sends them); decimals stay exact strings; JSON is parsed; booleans accept
 * true/false/1/0. Dates and text go as typed.
 */
export function parseInput(text: string, type: string | undefined, isNull = false): ParsedInput {
  if (isNull) return { ok: true, value: null };
  const kind = columnKind(type);
  const t = text.trim();
  switch (kind) {
    case 'int': {
      if (!/^[-+]?\d+$/.test(t)) return { ok: false, error: 'notInteger' };
      const n = Number(t);
      return { ok: true, value: Number.isSafeInteger(n) ? n : t.replace(/^\+/, '') };
    }
    case 'decimal':
      if (!/^[-+]?(\d+\.?\d*|\.\d+)(e[-+]?\d+)?$/i.test(t)) return { ok: false, error: 'notNumber' };
      return { ok: true, value: t.replace(/^\+/, '') };
    case 'bool': {
      const l = t.toLowerCase();
      if (['true', '1', 'yes'].includes(l)) return { ok: true, value: true };
      if (['false', '0', 'no'].includes(l)) return { ok: true, value: false };
      return { ok: false, error: 'notBoolean' };
    }
    case 'json':
      try { return { ok: true, value: JSON.parse(t) }; } catch { return { ok: false, error: 'invalidJson' }; }
    default:
      return { ok: true, value: text };
  }
}

/** Columns the database fills itself (auto-increment, generated) — left out of a new row unless typed. */
export function isAutoColumn(c: Pick<CodeDbColumn, 'extra' | 'default'>): boolean {
  const e = (c.extra ?? '').toLowerCase();
  return e.includes('auto_increment') || e.includes('generated') || /default_generated/.test(e);
}

// ------------------------------------------------------------------ grid query

export interface GridQuery {
  page: number;
  perPage: number;
  q?: string;
  sort?: string;
  dir?: 'asc' | 'desc';
  filters?: Record<string, string>;
}

/** Query parameters of GET /db/tables/{t}/rows (and export): empty values are left out; filters as `filter[col]`. */
export function dbRowsQuery(g: GridQuery): Record<string, string> {
  const out: Record<string, string> = { page: String(Math.max(1, Math.floor(g.page || 1))), perPage: String(Math.min(100, Math.max(1, Math.floor(g.perPage || 50)))) };
  const q = g.q?.trim();
  if (q) out['q'] = q;
  if (g.sort) { out['sort'] = g.sort; out['dir'] = g.dir === 'desc' ? 'desc' : 'asc'; }
  for (const [col, v] of Object.entries(g.filters ?? {}).sort(([a], [b]) => a.localeCompare(b))) {
    const t = v?.trim();
    if (col && t) out[`filter[${col}]`] = t;
  }
  return out;
}

/** Same as dbRowsQuery but without paging (export takes the whole filtered table). */
export function dbExportQuery(g: Omit<GridQuery, 'page' | 'perPage'>): Record<string, string> {
  const { page: _p, perPage: _pp, ...rest } = dbRowsQuery({ ...g, page: 1, perPage: 1 });
  return rest;
}

/** Next sort state when a column header is clicked: asc → desc → off. */
export function nextSort(cur: { sort?: string; dir?: 'asc' | 'desc' }, col: string): { sort?: string; dir?: 'asc' | 'desc' } {
  if (cur.sort !== col) return { sort: col, dir: 'asc' };
  if (cur.dir === 'asc') return { sort: col, dir: 'desc' };
  return {};
}

// ------------------------------------------------------------------ CSV import

export interface ImportLine { row: number; message: string; /** row ≤ 0: a problem with the whole file (e.g. unknown column in the header). */ file: boolean }
export interface ImportSummary {
  inserted: number;
  errors: ImportLine[];
  /** First `limit` errors for display. */
  shown: ImportLine[];
  /** Errors not shown. */
  more: number;
  /** Data rows with at least one error. */
  badRows: number;
  ok: boolean;
}

/** Normalises an import answer: file-level errors first, then by row; duplicate messages dropped; capped at `limit`. */
export function summarizeImport(r: Partial<CodeDbImportResult> | null | undefined, limit = 100): ImportSummary {
  const seen = new Set<string>();
  const errors: ImportLine[] = [];
  for (const e of r?.errors ?? []) {
    const row = Number.isFinite(+e.row) ? Math.floor(+e.row) : 0;
    const message = String(e.message ?? '').trim();
    const k = `${row}\u0000${message}`;
    if (seen.has(k)) continue;
    seen.add(k);
    errors.push({ row, message, file: row <= 0 });
  }
  errors.sort((a, b) => (a.file !== b.file ? (a.file ? -1 : 1) : a.row - b.row));
  const badRows = new Set(errors.filter(e => !e.file).map(e => e.row)).size;
  return { inserted: Math.max(0, Math.floor(r?.inserted ?? 0)), errors, shown: errors.slice(0, limit), more: Math.max(0, errors.length - limit), badRows, ok: errors.length === 0 };
}

// ------------------------------------------------------------------ migrations

const MIGRATION_FILE = /^migrations\/(\d+)_[^/]*\.(ts|js)$/i;

/** `Create Orders!` → `create_orders` (empty → `migration`), max 60 chars. */
export function migrationSlug(name: string): string {
  const s = name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 60).replace(/_+$/, '');
  return s || 'migration';
}

/**
 * Path of a new migration: `migrations/NNN_<slug>.ts` where NNN is one more than the highest
 * number already used in migrations/ (3 digits, more when needed).
 */
export function migrationFileName(existing: readonly string[], name: string, ext: 'ts' | 'js' = 'ts'): string {
  let max = 0;
  let width = 3;
  for (const p of existing) {
    const m = MIGRATION_FILE.exec(p);
    if (!m) continue;
    max = Math.max(max, parseInt(m[1], 10));
    width = Math.max(width, m[1].length);
  }
  return `migrations/${String(max + 1).padStart(width, '0')}_${migrationSlug(name)}.${ext}`;
}

/** Starter content of a new migration (`create_<table>` names get a create-table example). */
export function migrationTemplate(slug: string): string {
  const create = /^create_([a-z][a-z0-9_]*?)(?:_table)?$/.exec(slug);
  if (create) {
    const t = create[1];
    return `// Migration ${slug} — runs once when you publish (files run in name order).
// down() undoes it: the latest batch can be rolled back on the Database → Migrations page.
export default migration({
  up(schema) {
    schema.create('${t}', t => {
      t.id();
      t.string('name');
      t.timestamps();
    });
  },
  down(schema) {
    schema.dropIfExists('${t}');
  },
});
`;
  }
  return `// Migration ${slug} — runs once when you publish (files run in name order).
// down() undoes it: the latest batch can be rolled back on the Database → Migrations page.
export default migration({
  up(schema) {
    // schema.table('orders', t => {
    //   t.string('note').nullable();
    // });
  },
  down(schema) {
    // schema.table('orders', t => {
    //   t.dropColumn('note');
    // });
  },
});
`;
}

export interface MigrationsView { history: CodeDbMigration[]; pending: CodeDbPendingInfo[]; latestBatch: number | null }

const isRolledBack = (s: string) => /roll|down|revert/i.test(s);
const isFailed = (s: string) => /fail|error/i.test(s);

/** Badge class of a migration status. */
export function migrationStatusClass(status: string): 'badge-live' | 'badge-danger' | 'badge-warn' | '' {
  if (isFailed(status)) return 'badge-danger';
  if (isRolledBack(status)) return 'badge-warn';
  if (/ran|ok|done|success|applied/i.test(status)) return 'badge-live';
  return '';
}

/**
 * Normalises GET /db/migrations: either a plain array (pending rows carry status "pending") or an
 * object `{items|history|migrations, pending}`; pending entries may be names or objects. History
 * comes newest first; latestBatch = the highest batch that still has applied migrations.
 */
export function normalizeMigrations(res: unknown): MigrationsView {
  let rows: Partial<CodeDbMigration>[] = [];
  let pendingRaw: unknown[] = [];
  if (Array.isArray(res)) rows = res as Partial<CodeDbMigration>[];
  else if (res && typeof res === 'object') {
    const o = res as Record<string, unknown>;
    const list = o['items'] ?? o['history'] ?? o['migrations'] ?? o['ran'] ?? [];
    rows = Array.isArray(list) ? (list as Partial<CodeDbMigration>[]) : [];
    const p = o['pending'];
    if (Array.isArray(p)) pendingRaw = p;
    else if (p && typeof p === 'object') {
      for (const [source, names] of Object.entries(p as Record<string, unknown>)) {
        if (Array.isArray(names)) for (const n of names) pendingRaw.push(typeof n === 'string' ? { name: n, source } : { source, ...(n as object) });
      }
    }
  }
  const history: CodeDbMigration[] = [];
  for (const r of rows) {
    if (!r || !r.name) continue;
    if ((r.status ?? '').toLowerCase() === 'pending') { pendingRaw.push({ name: r.name, source: (r as { source?: string }).source }); continue; }
    history.push({
      name: r.name, batch: Number(r.batch) || 0, ranAt: r.ranAt ?? '', ranBy: r.ranBy ?? '', durationMs: Number(r.durationMs) || 0,
      status: r.status ?? '', error: r.error ?? null,
    });
  }
  history.sort((a, b) => b.batch - a.batch || b.name.localeCompare(a.name));
  const seen = new Set<string>();
  const pending: CodeDbPendingInfo[] = [];
  for (const p of pendingRaw) {
    const item: CodeDbPendingInfo = typeof p === 'string' ? { name: p } : { name: String((p as CodeDbPendingInfo)?.name ?? ''), source: (p as CodeDbPendingInfo)?.source };
    const k = `${item.name}\u0000${item.source ?? ''}`;
    if (!item.name || seen.has(k)) continue;
    seen.add(k);
    pending.push(item);
  }
  pending.sort((a, b) => a.name.localeCompare(b.name) || String(a.source ?? '').localeCompare(String(b.source ?? '')));
  const applied = history.filter(h => !isFailed(h.status) && !isRolledBack(h.status));
  const latestBatch = applied.length ? Math.max(...applied.map(h => h.batch)) : null;
  return { history, pending, latestBatch };
}

/** A migration that may be rolled back: applied, in the latest batch. */
export function canRollBack(m: CodeDbMigration, latestBatch: number | null): boolean {
  return latestBatch !== null && m.batch === latestBatch && !isFailed(m.status) && !isRolledBack(m.status);
}

/** Accepts backups in camelCase or snake_case. */
export function normalizeBackups(list: unknown): CodeDbBackup[] {
  if (!Array.isArray(list)) return [];
  return list.map(x => {
    const o = (x ?? {}) as Record<string, unknown>;
    const pick = (a: string, b: string) => String(o[a] ?? o[b] ?? '');
    return {
      id: Number(o['id']) || 0,
      tableName: pick('tableName', 'table_name'), backupTable: pick('backupTable', 'backup_table'), migration: pick('migration', 'migration'),
      createdAt: pick('createdAt', 'created_at'), expiresAt: pick('expiresAt', 'expires_at'),
    };
  }).sort((a, b) => b.createdAt.localeCompare(a.createdAt) || b.id - a.id);
}

// ------------------------------------------------------------------ destructive changes

export interface DestructiveGroup { migration: string; items: { op: string; target: string; text: string }[] }

/** `orders` / `orders.note`. */
export function destructiveTarget(d: CodeDestructive): string {
  return d.column ? `${d.table}.${d.column}` : d.table;
}

/**
 * Display lines for destructive changes, grouped by migration (in name order). `opLabel` turns
 * an op code (drop_table, drop_column, change_column, rename_table, rename_column…) into words;
 * unknown ops fall back to the code itself. `fallbackMigration` names items without one.
 */
export function formatDestructive(list: readonly CodeDestructive[] | null | undefined, opLabel: (op: string) => string | null, fallbackMigration = ''): DestructiveGroup[] {
  const groups = new Map<string, DestructiveGroup>();
  const seen = new Set<string>();
  for (const d of list ?? []) {
    if (!d || !d.table) continue;
    const migration = d.migration || fallbackMigration;
    const target = destructiveTarget(d);
    const k = `${migration}\u0000${d.op}\u0000${target}`;
    if (seen.has(k)) continue;
    seen.add(k);
    const label = opLabel(d.op) || d.op;
    const g = groups.get(migration) ?? { migration, items: [] };
    g.items.push({ op: d.op, target, text: `${label}: ${target}` });
    groups.set(migration, g);
  }
  return [...groups.values()].sort((a, b) => a.migration.localeCompare(b.migration));
}

/** All destructive items of the pending migrations of a dry run (each tagged with its migration). */
export function pendingDestructive(pending: readonly { name: string; destructive?: CodeDestructive[] }[] | null | undefined): CodeDestructive[] {
  return (pending ?? []).flatMap(p => (p.destructive ?? []).map(d => ({ ...d, migration: d.migration || p.name })));
}

// ------------------------------------------------------------------ SQL console

export type ParamsResult = { ok: true; params: unknown[] } | { ok: false };

/** Params box: empty → []; otherwise a JSON array (`[1, "paid"]`). */
export function parseSqlParams(text: string): ParamsResult {
  const t = text.trim();
  if (!t) return { ok: true, params: [] };
  try {
    const v = JSON.parse(t);
    return Array.isArray(v) ? { ok: true, params: v } : { ok: false };
  } catch { return { ok: false }; }
}

/** First keyword of a statement (comments skipped), lower-cased — to warn before running a write. */
export function sqlVerb(sql: string): string {
  const s = sql.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/(^|\n)\s*(--|#)[^\n]*/g, '\n').trim();
  return (/^[a-z]+/i.exec(s)?.[0] ?? '').toLowerCase();
}

const READ_VERBS = ['select', 'show', 'describe', 'desc', 'explain', 'with', 'values', 'table'];
export function isReadSql(sql: string): boolean { return READ_VERBS.includes(sqlVerb(sql)); }

/** Result rows as arrays in column order (the API may send objects or arrays). */
export function sqlRows(r: CodeDbSqlResult | null | undefined): { columns: string[]; rows: unknown[][] } {
  const rows = r?.rows ?? [];
  let columns = r?.columns ?? [];
  if (!columns.length && rows.length && !Array.isArray(rows[0])) columns = Object.keys(rows[0] as CodeDbRow);
  return { columns, rows: rows.map(row => (Array.isArray(row) ? row : columns.map(c => (row as CodeDbRow)[c]))) };
}

// ------------------------------------------------------------------ ER diagram

export interface ErTableIn { name: string; columns: { name: string; type?: string; key?: string }[] }
export interface ErLinkIn { from: string; fromColumn: string; to: string; toColumn: string; /** metadata-only relation (core_* views) */ meta?: boolean }
export interface ErColumn { name: string; type: string; pk: boolean; fk: boolean }
export interface ErNode { name: string; x: number; y: number; w: number; h: number; core: boolean; columns: ErColumn[]; more: number; layer: number }
export interface ErEdge { from: string; fromColumn: string; to: string; toColumn: string; meta: boolean; d: string; x1: number; y1: number; x2: number; y2: number }
export interface ErLayout { nodes: ErNode[]; edges: ErEdge[]; width: number; height: number }

export const ER = { width: 210, head: 30, row: 20, pad: 8, gapX: 90, gapY: 28, margin: 20, maxColumns: 12 } as const;

/**
 * Lays out the developer tables for the ER diagram. A table referencing others is placed in a
 * column to the right of the tables it references (longest reference chain; cycles cut);
 * tables of a column are sorted by name. core_* views that are referenced but not listed are
 * added as greyed nodes showing only the referenced columns. Edges go from the referencing
 * column (left side) to the referenced column (right side) as cubic curves.
 */
export function erLayout(tables: readonly ErTableIn[], links: readonly ErLinkIn[]): ErLayout {
  const byName = new Map<string, ErTableIn & { core: boolean }>();
  for (const t of tables) byName.set(t.name, { ...t, core: t.name.startsWith('core_') });
  const valid = links.filter(l => byName.has(l.from) && l.to);
  for (const l of valid) {
    if (byName.has(l.to)) continue;
    byName.set(l.to, { name: l.to, columns: [], core: true });
  }
  // greyed (unlisted) nodes show just the columns that are referenced
  for (const l of valid) {
    const t = byName.get(l.to)!;
    if (t.core && !tables.some(x => x.name === t.name) && !t.columns.some(c => c.name === l.toColumn)) t.columns.push({ name: l.toColumn });
  }
  const refs = new Map<string, Set<string>>();
  for (const l of valid) if (l.from !== l.to) (refs.get(l.from) ?? refs.set(l.from, new Set()).get(l.from)!).add(l.to);

  const layer = new Map<string, number>();
  const visiting = new Set<string>();
  const depth = (n: string): number => {
    const known = layer.get(n);
    if (known !== undefined) return known;
    if (visiting.has(n)) return 0; // cycle: cut here
    visiting.add(n);
    let d = 0;
    for (const r of [...(refs.get(n) ?? [])].sort()) d = Math.max(d, depth(r) + 1);
    visiting.delete(n);
    layer.set(n, d);
    return d;
  };
  const names = [...byName.keys()].sort();
  names.forEach(depth);

  const fkCols = new Set(valid.map(l => `${l.from}.${l.fromColumn}`));
  const columns = new Map<number, string[]>();
  for (const n of names) (columns.get(layer.get(n)!) ?? columns.set(layer.get(n)!, []).get(layer.get(n)!)!).push(n);

  const nodes: ErNode[] = [];
  const nodeMap = new Map<string, ErNode>();
  let height = 0;
  const layers = [...columns.keys()].sort((a, b) => a - b);
  layers.forEach((li, ci) => {
    let y = ER.margin;
    for (const n of columns.get(li)!) {
      const t = byName.get(n)!;
      const shown = t.columns.slice(0, ER.maxColumns);
      const more = t.columns.length - shown.length;
      const h = ER.head + ER.row * (shown.length + (more > 0 ? 1 : 0)) + ER.pad;
      const node: ErNode = {
        name: n, x: ER.margin + ci * (ER.width + ER.gapX), y, w: ER.width, h, core: t.core, layer: ci, more: Math.max(0, more),
        columns: shown.map(c => ({ name: c.name, type: c.type ?? '', pk: (c.key ?? '').toUpperCase() === 'PRI', fk: fkCols.has(`${n}.${c.name}`) })),
      };
      nodes.push(node);
      nodeMap.set(n, node);
      y += h + ER.gapY;
    }
    height = Math.max(height, y - ER.gapY + ER.margin);
  });
  const width = layers.length ? ER.margin * 2 + layers.length * ER.width + (layers.length - 1) * ER.gapX : 0;

  const rowY = (node: ErNode, col: string) => {
    const i = node.columns.findIndex(c => c.name === col);
    const idx = i < 0 ? -0.5 : i; // unknown/hidden column → header line
    return node.y + ER.head + ER.row * idx + ER.row / 2;
  };
  const edges: ErEdge[] = valid.map(l => {
    const a = nodeMap.get(l.from)!, b = nodeMap.get(l.to)!;
    const y1 = rowY(a, l.fromColumn), y2 = rowY(b, l.toColumn);
    let x1: number, x2: number, d: string;
    if (a === b || a.layer === b.layer) {
      // same column: loop out of the right side
      x1 = a.x + a.w; x2 = b.x + b.w;
      const bend = x1 + 36;
      d = `M${x1},${y1} C${bend},${y1} ${bend},${y2} ${x2},${y2}`;
    } else if (a.layer > b.layer) {
      x1 = a.x; x2 = b.x + b.w;
      const mid = (x1 - x2) / 2;
      d = `M${x1},${y1} C${x1 - mid},${y1} ${x2 + mid},${y2} ${x2},${y2}`;
    } else {
      x1 = a.x + a.w; x2 = b.x;
      const mid = (x2 - x1) / 2;
      d = `M${x1},${y1} C${x1 + mid},${y1} ${x2 - mid},${y2} ${x2},${y2}`;
    }
    return { from: l.from, fromColumn: l.fromColumn, to: l.to, toColumn: l.toColumn, meta: !!l.meta || b.core, d, x1, y1, x2, y2 };
  });
  return { nodes, edges, width, height: Math.max(height, 0) };
}
