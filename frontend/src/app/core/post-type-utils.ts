/**
 * Pure helpers of Site Code post types and custom fields (phase 4a):
 * normalising what the server sends, field declarations → schema-form
 * fields, client-side validation, API field errors and admin paths.
 * No Angular here — unit tested in post-type-utils.spec.ts.
 */
import { CodeAdminPage, CodeFieldDef, CodeFieldSpec, CodeLabel, CodePostType } from './code-models';
import { SchemaField } from './models';

/** Type ids: `^[a-z][a-z0-9_]{1,19}$`, never a built-in type. */
export const TYPE_ID_RE = /^[a-z][a-z0-9_]{1,19}$/;
export const RESERVED_TYPES = ['post', 'page', 'template', 'revision', 'attachment', 'component', 'menu', 'media', 'nav_menu_item'];
/** What a post type supports when it does not say. */
export const DEFAULT_SUPPORTS = ['title', 'content', 'excerpt', 'image'];
/** Everything the classic post editor shows (type `post`). */
export const POST_SUPPORTS = ['title', 'content', 'excerpt', 'image', 'categories'];
/** Site Code pages are `name`; plugin pages are `<plugin id>:<name>` (e.g. `forms:forms`). */
const ADMIN_PAGE_ID_RE = /^(?:[a-z0-9][a-z0-9-]{0,63}:)?[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/;

/** A list from an array or from `{key: [...]}` envelopes (`postTypes`, `items`, …). */
export function asList<T>(raw: unknown, keys: string[]): T[] {
  if (Array.isArray(raw)) return raw as T[];
  if (raw && typeof raw === 'object') {
    for (const k of keys) {
      const v = (raw as Record<string, unknown>)[k];
      if (Array.isArray(v)) return v as T[];
    }
  }
  return [];
}

export const isCustomType = (type: string) => TYPE_ID_RE.test(type) && !RESERVED_TYPES.includes(type);

/** Valid, unique custom post types (bad ids and built-ins dropped). */
export function normalizePostTypes(raw: unknown): CodePostType[] {
  const out: CodePostType[] = [];
  for (const x of asList<CodePostType>(raw, ['postTypes', 'items'])) {
    if (!x || typeof x !== 'object' || typeof x.type !== 'string' || !isCustomType(x.type)) continue;
    if (out.some(p => p.type === x.type)) continue;
    out.push(x);
  }
  return out;
}

/** Admin pages with usable ids (unique). */
export function normalizeAdminPages(raw: unknown): CodeAdminPage[] {
  const out: CodeAdminPage[] = [];
  for (const x of asList<CodeAdminPage>(raw, ['adminPages', 'items'])) {
    if (!x || typeof x.id !== 'string' || !ADMIN_PAGE_ID_RE.test(x.id) || out.some(p => p.id === x.id)) continue;
    out.push(x);
  }
  return out;
}

/** `'text'` → `{type: 'text'}`. */
export function normalizeFieldDef(spec: CodeFieldSpec): CodeFieldDef {
  if (typeof spec === 'string') return { type: spec };
  return spec && typeof spec === 'object' ? { ...spec, type: spec.type || 'text' } : { type: 'text' };
}

/** "readingTime" / "start_date" → "Reading time" / "Start date". */
export function humanizeKey(key: string): string {
  const s = key.replace(/[_-]+/g, ' ').replace(/([a-z0-9])([A-Z])/g, '$1 $2').trim().toLowerCase();
  return s ? s[0].toUpperCase() + s.slice(1) : key;
}

const FIELD_TYPES: Record<string, SchemaField['type']> = {
  text: 'text', textarea: 'textarea', number: 'number', bool: 'boolean', boolean: 'boolean', date: 'date', datetime: 'datetime',
  select: 'select', url: 'url', image: 'image', email: 'email', json: 'json',
};

function labelOf(l: CodeLabel | undefined, fallback: string): SchemaField['label'] {
  if (!l) return fallback;
  if (typeof l === 'string') return l;
  return { th: l.th || l.en || fallback, en: l.en || l.th || fallback };
}

/** Custom field declarations → schema-form fields (declared order kept). */
export function fieldsToSchema(fields: Record<string, CodeFieldSpec> | null | undefined): SchemaField[] {
  return Object.entries(fields ?? {}).map(([key, spec]) => {
    const d = normalizeFieldDef(spec);
    const f: SchemaField = { key, type: FIELD_TYPES[d.type] ?? 'text', label: labelOf(d.label, humanizeKey(key.slice(key.lastIndexOf('.') + 1))) };
    if (d.help) f.help = labelOf(d.help, '');
    if (d.required) f.required = true;
    if (typeof d.min === 'number') f.min = d.min;
    if (typeof d.max === 'number') f.max = d.max;
    if (d.placeholder) f.placeholder = d.placeholder;
    if (d.default !== undefined) f.default = d.default;
    if (f.type === 'select') {
      f.options = (d.options ?? []).map(o => (typeof o === 'string' ? { value: o, label: o } : { value: String(o.value), label: labelOf(o.label, String(o.value)) }));
    }
    return f;
  });
}

/** Fields of one JS plugin (site.fields in a plugin), shown in their own panel. */
export interface PluginFieldGroup { plugin: string; name: string; fields: SchemaField[] }

/**
 * Splits field declarations into the site's own fields and one group per JS plugin
 * (specs with `plugin`; keys "<plugin>.<key>"), keeping the declared order.
 */
export function groupFields(fields: Record<string, CodeFieldSpec> | null | undefined): { site: SchemaField[]; plugins: PluginFieldGroup[] } {
  const site: Record<string, CodeFieldSpec> = {};
  const byPlugin = new Map<string, { name: string; specs: Record<string, CodeFieldSpec> }>();
  for (const [key, spec] of Object.entries(fields ?? {})) {
    const plugin = spec && typeof spec === 'object' && typeof spec.plugin === 'string' && spec.plugin ? spec.plugin : '';
    if (!plugin) { site[key] = spec; continue; }
    let g = byPlugin.get(plugin);
    if (!g) { g = { name: (spec as CodeFieldDef).pluginName || plugin, specs: {} }; byPlugin.set(plugin, g); }
    g.specs[key] = spec;
  }
  return {
    site: fieldsToSchema(site),
    plugins: [...byPlugin.entries()].map(([plugin, g]) => ({ plugin, name: g.name, fields: fieldsToSchema(g.specs) })),
  };
}

export type MetaErrorCode = 'required' | 'min' | 'max' | 'minLength' | 'maxLength' | 'number' | 'option' | 'email' | 'url' | 'date' | 'json';
export interface MetaError { code: MetaErrorCode; params?: Record<string, string | number> }

const isEmpty = (v: unknown) => v === undefined || v === null || v === '';

/**
 * Client-side check before saving (the server validates again and its
 * messages win). Numbers: min/max = value; text: min/max = length.
 */
export function validateMeta(fields: SchemaField[], values: Record<string, unknown>): Record<string, MetaError> {
  const errs: Record<string, MetaError> = {};
  for (const f of fields) {
    const v = values[f.key];
    if (f.type === 'boolean') continue;
    if (isEmpty(v)) { if (f.required) errs[f.key] = { code: 'required' }; continue; }
    switch (f.type) {
      case 'number': {
        const n = typeof v === 'number' ? v : Number(v);
        if (!Number.isFinite(n)) errs[f.key] = { code: 'number' };
        else if (f.min !== undefined && n < f.min) errs[f.key] = { code: 'min', params: { n: f.min } };
        else if (f.max !== undefined && n > f.max) errs[f.key] = { code: 'max', params: { n: f.max } };
        break;
      }
      case 'select':
        if (f.options?.length && !f.options.some(o => o.value === String(v))) errs[f.key] = { code: 'option' };
        break;
      case 'email':
        if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(String(v))) errs[f.key] = { code: 'email' };
        break;
      case 'url':
        if (!/^(https?:\/\/[^\s]+|\/[^\s]*)$/i.test(String(v))) errs[f.key] = { code: 'url' };
        break;
      case 'date':
        if (!/^\d{4}-\d{2}-\d{2}$/.test(String(v))) errs[f.key] = { code: 'date' };
        break;
      case 'datetime':
        if (!/^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}/.test(String(v))) errs[f.key] = { code: 'date' };
        break;
      case 'json':
        if (v instanceof InvalidJson) errs[f.key] = { code: 'json' };
        break;
      default: {
        const len = Array.from(String(v)).length;
        if (f.min !== undefined && len < f.min) errs[f.key] = { code: 'minLength', params: { n: f.min } };
        else if (f.max !== undefined && len > f.max) errs[f.key] = { code: 'maxLength', params: { n: f.max } };
      }
    }
  }
  return errs;
}

/** What a json field holds while its text does not parse (never sent; validateMeta reports it). */
export class InvalidJson {
  constructor(readonly text: string) {}
}

/**
 * Per-field messages of a refused save. Accepts `{error, fields:{key: msg}}`,
 * `{errors:{key: msg}}` or `{errors:[{field|key, message|error}]}` (the
 * backend's Thai messages are shown as they come).
 */
export function metaErrorsFromApi(e: unknown): Record<string, string> {
  const err = e as { error?: unknown };
  let body = err?.error;
  if (typeof body === 'string') { try { body = JSON.parse(body); } catch { return {}; } }
  if (!body || typeof body !== 'object') return {};
  const b = body as Record<string, unknown>;
  const out: Record<string, string> = {};
  for (const src of [b['fields'], b['errors'], b['fieldErrors']]) {
    if (Array.isArray(src)) {
      for (const x of src) {
        const r = x as Record<string, unknown>;
        const k = String(r?.['field'] ?? r?.['key'] ?? '').replace(/^meta\./, '');
        const m = r?.['message'] ?? r?.['error'];
        if (k && typeof m === 'string') out[k] = m;
      }
    } else if (src && typeof src === 'object') {
      for (const [k, m] of Object.entries(src)) if (typeof m === 'string') out[k.replace(/^meta\./, '')] = m;
    }
  }
  return out;
}

/** Values to send: declared keys only; empty → null; json text that does not parse is never sent. */
export function pickMeta(fields: SchemaField[], values: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of fields) {
    const v = values[f.key];
    if (v instanceof InvalidJson) continue;
    if (f.type === 'boolean') out[f.key] = !!(v ?? f.default);
    else if (f.type === 'number') out[f.key] = isEmpty(v) ? null : Number(v);
    else out[f.key] = isEmpty(v) ? null : v;
  }
  return out;
}

export function supportsOf(pt: CodePostType | null | undefined, type = pt?.type): Set<string> {
  if (type === 'post') return new Set(POST_SUPPORTS);
  return new Set(pt?.supports?.length ? pt.supports : DEFAULT_SUPPORTS);
}

/** Public URL prefix of a type ('events' for slug 'events', else the type id). */
export function typePrefix(pt: Pick<CodePostType, 'type' | 'slug'>): string {
  return (pt.slug || pt.type).replace(/^\/+|\/+$/g, '');
}

export function publicPath(pt: Pick<CodePostType, 'type' | 'slug'>, slug: string): string {
  return `/${typePrefix(pt)}/${slug}`;
}

/** Admin list / editor routes: the classic post screens for `post`, /content/<type> for custom types. */
export function listRoute(type: string): string { return type === 'post' ? '/posts' : `/content/${type}`; }
export function editRoute(type: string, id?: number): (string | number)[] {
  return type === 'post' ? ['/posts', id ?? 'new'] : ['/content', type, id ?? 'new'];
}

/** Option holding the single-post template of a type (Settings → Reading). */
export const templateOption = (type: string) => (type === 'post' ? 'post_template_id' : `post_template_${type}`);

/** Up to `max` short custom fields shown as list columns. */
export function listColumns(fields: SchemaField[], max = 2): SchemaField[] {
  return fields.filter(f => !['textarea', 'json', 'image'].includes(f.type)).slice(0, max);
}

/** How a custom field value prints in a list cell. */
export function metaCell(f: SchemaField, v: unknown): string {
  if (isEmpty(v)) return '—';
  if (f.type === 'boolean') return v ? '✓' : '—';
  if (f.type === 'select') {
    const o = f.options?.find(x => x.value === String(v));
    if (o) return typeof o.label === 'string' ? o.label : String(v);
  }
  if (f.type === 'datetime' && typeof v === 'string') return v.replace('T', ' ').slice(0, 16);
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

/** Stored datetime → value of an `<input type=datetime-local>` (YYYY-MM-DDTHH:mm). */
export function toDatetimeLocal(v: unknown): string {
  if (typeof v !== 'string') return '';
  const m = /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2})/.exec(v);
  return m ? `${m[1]}T${m[2]}` : '';
}

/** `<input type=datetime-local>` value → stored datetime (seconds added, no zone = site time zone). */
export function fromDatetimeLocal(s: string): string {
  return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(s) ? s + ':00' : s;
}
