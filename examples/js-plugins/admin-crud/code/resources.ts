// Resource definitions (code/config.json, or the "resources" setting as JSON), value validation and
// the data source of a resource: one of this plugin's tables, or another JS plugin's exports
// (plugins.call — phase 6), which may be missing (plugins.has → the screen degrades gracefully).
import config from './config.json';

export type ColumnType = 'text' | 'textarea' | 'email' | 'number' | 'bool' | 'date' | 'datetime' | 'select';
export type Label = string | { th?: string; en?: string };

export interface ColumnDef {
  name: string;
  label?: Label;
  type?: ColumnType;
  required?: boolean;
  /** text: max length; number: max value. */
  max?: number;
  min?: number;
  options?: string[];
  /** Shown in the list. */
  list?: boolean;
  /** Searched by the search box (LIKE). */
  search?: boolean;
}

/**
 * A resource backed by another JS plugin's exports (names listed in that plugin's code.exports):
 *   list({ q, page, perPage, orderBy }) → { items, total } (or an array: paginated here)
 *   get(id) → row | null;  save({ id, values }) → { id } | { errors };  delete(id) → number | boolean
 * Without save / delete the resource is read-only for that operation.
 */
export interface Source {
  plugin: string;
  list: string;
  get?: string;
  save?: string;
  delete?: string;
}

export interface Resource {
  id: string;
  /** One of this plugin's own tables (created by its migrations) — or `source`. */
  table?: string;
  /** Another plugin's exports instead of a table. */
  source?: Source;
  label?: Label;
  perPage?: number;
  /** "column" or "-column" (descending). Default "-id". */
  orderBy?: string;
  columns: ColumnDef[];
}

const IDENT = /^[a-z][a-z0-9_]{0,62}$/;

/** The configured resources (setting "resources" overrides config.json when it is valid JSON). */
export function resources(): Resource[] {
  let list: Resource[] = (config as { resources: Resource[] }).resources;
  const raw = site.settings('resources');
  if (raw) {
    try {
      const parsed = JSON.parse(raw);
      list = Array.isArray(parsed) ? parsed : parsed.resources;
    } catch (e) {
      log.warn('admin-crud: the "resources" setting is not valid JSON; using config.json', String(e));
    }
  }
  return (list ?? []).filter((r) => r && IDENT.test(r.id ?? '') && Array.isArray(r.columns) &&
      (r.source ? validSource(r.source) : IDENT.test(r.table ?? '')))
    .map((r) => ({ ...r, columns: r.columns.filter((c) => c && IDENT.test(c.name ?? '')) }));
}

const PLUGIN_ID = /^[a-z0-9][a-z0-9-]{1,49}$/;
const EXPORT = /^[A-Za-z_$][A-Za-z0-9_$]{0,62}$/;
function validSource(s: Source): boolean {
  if (!s || typeof s !== 'object' || !PLUGIN_ID.test(s.plugin ?? '') || !EXPORT.test(s.list ?? '')) return false;
  return [s.get, s.save, s.delete].every((n) => n === undefined || EXPORT.test(n));
}

export function resource(id: unknown): Resource | null {
  return resources().find((r) => r.id === id) ?? null;
}

export const label = (l: Label | undefined, fallback: string, en: boolean): string =>
  l === undefined ? fallback : typeof l === 'string' ? l : (en ? l.en ?? l.th : l.th ?? l.en) ?? fallback;

/** Validates and converts form values for a resource: { row } or { errors }. */
export function validate(res: Resource, values: Record<string, any>, en: boolean): { row?: Record<string, any>; errors?: Record<string, string> } {
  const row: Record<string, any> = {};
  const errors: Record<string, string> = {};
  const msg = (th: string, e: string) => (en ? e : th);
  for (const c of res.columns) {
    const type = c.type ?? 'text';
    let v = values?.[c.name];
    if (type === 'bool') {
      row[c.name] = v === true || v === 'true' || v === '1' || v === 1 || v === 'on';
      continue;
    }
    if (v === undefined || v === null || String(v).trim() === '') {
      if (c.required) errors[c.name] = msg('จำเป็นต้องกรอก', 'Required');
      else row[c.name] = null;
      continue;
    }
    v = String(v).trim();
    switch (type) {
      case 'number': {
        const n = Number(v);
        if (!Number.isFinite(n)) errors[c.name] = msg('ต้องเป็นตัวเลข', 'Must be a number');
        else if (c.min !== undefined && n < c.min) errors[c.name] = msg(`อย่างน้อย ${c.min}`, `At least ${c.min}`);
        else if (c.max !== undefined && n > c.max) errors[c.name] = msg(`ไม่เกิน ${c.max}`, `At most ${c.max}`);
        else row[c.name] = n;
        break;
      }
      case 'email':
        if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v) || v.length > 190) errors[c.name] = msg('อีเมลไม่ถูกต้อง', 'Invalid email');
        else row[c.name] = v.toLowerCase();
        break;
      case 'date':
        if (!/^\d{4}-\d{2}-\d{2}$/.test(v)) errors[c.name] = msg('วันที่ไม่ถูกต้อง', 'Invalid date');
        else row[c.name] = v;
        break;
      case 'datetime':
        if (!/^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2})?$/.test(v)) errors[c.name] = msg('วันเวลาไม่ถูกต้อง', 'Invalid date/time');
        else row[c.name] = v.replace('T', ' ') + (v.length === 16 ? ':00' : '');
        break;
      case 'select':
        if (!(c.options ?? []).includes(v)) errors[c.name] = msg('ตัวเลือกไม่ถูกต้อง', 'Invalid option');
        else row[c.name] = v;
        break;
      default:
        if (c.max !== undefined && v.length > c.max) errors[c.name] = msg(`ยาวเกิน ${c.max} ตัวอักษร`, `At most ${c.max} characters`);
        else row[c.name] = v;
    }
  }
  return Object.keys(errors).length ? { errors } : { row };
}

/** LIKE pattern for a search term (%, _ and \ escaped). */
export const likeOf = (q: string) => '%' + q.replace(/[\\%_]/g, (c) => '\\' + c) + '%';

// ---------------------------------------------------------------- data sources

export interface Page {
  items: Record<string, any>[];
  total: number;
}

export interface DataSource {
  /** false: the source plugin is not installed / active (reason says so). */
  available: boolean;
  reason?: string;
  canCreate: boolean;
  canEdit: boolean;
  canDelete: boolean;
  list(q: string, page: number, perPage: number): Page;
  get(id: string | number): Record<string, any> | null;
  save(id: number | null, row: Record<string, any>): { id?: any; errors?: Record<string, string> };
  delete(id: number): number;
}

function tableSource(res: Resource): DataSource {
  const table = res.table as string;
  return {
    available: true,
    canCreate: true,
    canEdit: true,
    canDelete: true,
    list(q, page, perPage) {
      const search = res.columns.filter((c) => c.search);
      let b = db.table(table);
      if (q && search.length) b = b.where((w) => search.forEach((c, i) => (i ? w.orWhere(c.name, 'like', likeOf(q)) : w.where(c.name, 'like', likeOf(q)))));
      const ob = res.orderBy ?? '-id';
      const col = ob.replace(/^-/, '');
      if (/^[a-z][a-z0-9_]*$/.test(col)) b = b.orderBy(col, ob.startsWith('-') ? 'desc' : 'asc');
      return b.paginate(page, perPage);
    },
    get: (id) => db.table(table).where('id', Number(id)).first(),
    save(id, row) {
      if (id) {
        const n = db.table(table).where('id', id).update(row);
        if (!n && !db.table(table).where('id', id).exists()) return { errors: { id: 'not found' } };
        return { id };
      }
      return { id: db.table(table).insert(row) };
    },
    delete: (id) => db.table(table).where('id', id).delete(),
  };
}

/** Is this error "the plugin is not installed / active" (plugins.call → code 'unavailable')? */
export const isUnavailable = (e: unknown) => !!e && typeof e === 'object' && (e as { code?: string }).code === 'unavailable';

function pluginSource(res: Resource, en: boolean): DataSource {
  const s = res.source as Source;
  if (!plugins.has(s.plugin)) {
    const why = en ? `The plugin "${s.plugin}" is not installed or not active.` : `ยังไม่ได้ติดตั้งหรือเปิดใช้ปลั๊กอิน "${s.plugin}"`;
    const none = () => {
      throw Object.assign(new Error(why), { code: 'unavailable' });
    };
    return { available: false, reason: why, canCreate: false, canEdit: false, canDelete: false, list: none, get: none, save: none, delete: none };
  }
  const call = (fn: string, ...args: unknown[]) => plugins.call(s.plugin, fn, ...args);
  return {
    available: true,
    canCreate: !!s.save,
    canEdit: !!s.save && !!s.get,
    canDelete: !!s.delete,
    list(q, page, perPage) {
      const out = call(s.list, { q, page, perPage, orderBy: res.orderBy ?? '-id' });
      if (Array.isArray(out)) return { items: out.slice((page - 1) * perPage, page * perPage), total: out.length };
      return { items: Array.isArray(out?.items) ? out.items : [], total: Number(out?.total) || 0 };
    },
    get: (id) => (s.get ? call(s.get, Number(id)) ?? null : null),
    save(id, row) {
      if (!s.save) return { errors: { form: en ? 'Read-only' : 'อ่านอย่างเดียว' } };
      const out = call(s.save, { id, values: row }) ?? {};
      return out.errors ? { errors: out.errors } : { id: out.id ?? id };
    },
    delete(id) {
      if (!s.delete) return 0;
      const out = call(s.delete, id);
      return typeof out === 'number' ? out : out ? 1 : 0;
    },
  };
}

/** The data source of a resource. */
export function sourceOf(res: Resource, en: boolean): DataSource {
  return res.source ? pluginSource(res, en) : tableSource(res);
}
