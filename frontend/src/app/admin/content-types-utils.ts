// Content types made in the admin without code (CMS 2.2.2): the form model and its checks. The server
// validates again (sitecode.ValidateTypeSpec); these give instant messages and build the request body.

export type Label2 = { th: string; en: string };

export interface FieldSpecForm {
  key: string;
  type: string;
  label: Label2;
  required?: boolean;
  min?: number | null;
  max?: number | null;
  /** select: one choice per line, "value" or "value | label" */
  options?: string;
  /** relation: the content types it links to */
  postTypes?: string[];
  multiple?: boolean;
  /** repeater: one level of simple fields */
  fields?: FieldSpecForm[];
}

export interface TypeSpecForm {
  type: string;
  label: Label2;
  singular: Label2;
  icon: string;
  slug: string;
  public: boolean;
  supports: string[];
  fields: FieldSpecForm[];
}

/** What the server sends back (GET /content-types). */
export interface ContentTypeRow {
  type: string;
  label: Partial<Label2>;
  singular?: Partial<Label2>;
  icon?: string;
  slug: string;
  public: boolean;
  supports: string[];
  fields: ServerField[];
  postCount: number;
  active: boolean;
}

export interface ServerField {
  key: string;
  type: string;
  label: Partial<Label2>;
  required?: boolean;
  min?: number;
  max?: number;
  options?: { value: string; label: Partial<Label2> }[];
  postTypes?: string[];
  multiple?: boolean;
  fields?: ServerField[];
}

export interface TakenType { type: string; slug: string; label: Partial<Label2>; owner: 'code' | 'plugin' }

/** Field types offered in the form, in the order shown (all of them work in the editor already). */
export const FIELD_TYPES = ['text', 'textarea', 'richtext', 'number', 'bool', 'date', 'datetime', 'select', 'url', 'email',
  'image', 'gallery', 'file', 'color', 'relation', 'repeater'] as const;
/** What a repeater row may hold (no nesting). */
export const REPEATER_TYPES = ['text', 'textarea', 'number', 'bool', 'date', 'datetime', 'select', 'url', 'email', 'image', 'file', 'color'] as const;
export const SUPPORTS = ['content', 'excerpt', 'image', 'categories'] as const;

const TYPE_RE = /^[a-z][a-z0-9_]{1,19}$/;
const SLUG_RE = /^[a-z0-9][a-z0-9-]{0,40}$/;
const KEY_RE = /^[a-zA-Z][a-zA-Z0-9_]{0,39}$/;
/** The core's own content type ids (sitecode builtinTypes). */
const BUILTIN = new Set(['post', 'page', 'template', 'revision', 'attachment', 'media', 'menu', 'nav_menu', 'component', 'user', 'users',
  'category', 'tag', 'term', 'comment', 'option', 'plugin', 'setting', 'settings', 'archive', 'search', 'feed']);

export function emptyType(): TypeSpecForm {
  return { type: '', label: { th: '', en: '' }, singular: { th: '', en: '' }, icon: '📦', slug: '', public: true,
    supports: ['content', 'excerpt', 'image'], fields: [] };
}

export function emptyField(type = 'text'): FieldSpecForm {
  return { key: '', type, label: { th: '', en: '' }, required: false };
}

/** An ASCII id from any text (Thai labels give '' — the user types the id then). */
export function idFrom(text: string, sep: '_' | '-'): string {
  const s = text.toLowerCase().normalize('NFKD').replace(/[̀-ͯ]/g, '').replace(/[^a-z0-9]+/g, sep)
    .replace(new RegExp(`^[${sep}0-9]+|[${sep}]+$`, 'g'), '');
  return s.slice(0, sep === '_' ? 20 : 41).replace(new RegExp(`[${sep}]+$`), '');
}

const lab = (l: Partial<Label2> | undefined): Label2 => ({ th: l?.th ?? '', en: l?.en ?? '' });

/** "value | label" lines ↔ options. */
export function optionsText(list: ServerField['options']): string {
  return (list ?? []).map(o => (o.label?.th && o.label.th !== o.value ? `${o.value} | ${o.label.th}` : o.value)).join('\n');
}
export function parseOptions(text: string): { value: string; label: Label2 }[] {
  return text.split('\n').map(l => l.trim()).filter(Boolean).map(l => {
    const [v, ...rest] = l.split('|');
    const value = v.trim(), name = rest.join('|').trim() || value;
    return { value, label: { th: name, en: name } };
  });
}

export function fieldToForm(f: ServerField): FieldSpecForm {
  return { key: f.key, type: f.type, label: lab(f.label), required: !!f.required, min: f.min ?? null, max: f.max ?? null,
    options: optionsText(f.options), postTypes: f.postTypes ?? [], multiple: !!f.multiple, fields: (f.fields ?? []).map(fieldToForm) };
}

export function rowToForm(r: ContentTypeRow): TypeSpecForm {
  return { type: r.type, label: lab(r.label), singular: lab(r.singular), icon: r.icon ?? '', slug: r.slug, public: r.public,
    supports: (r.supports ?? []).filter(s => s !== 'title'), fields: (r.fields ?? []).map(fieldToForm) };
}

function fieldBody(f: FieldSpecForm): Record<string, unknown> {
  const out: Record<string, unknown> = { key: f.key.trim(), type: f.type, label: { th: f.label.th.trim(), en: f.label.en.trim() } };
  if (f.required) out['required'] = true;
  if (f.type === 'number') {
    if (f.min !== null && f.min !== undefined && `${f.min}` !== '') out['min'] = Number(f.min);
    if (f.max !== null && f.max !== undefined && `${f.max}` !== '') out['max'] = Number(f.max);
  }
  if (f.type === 'select') out['options'] = parseOptions(f.options ?? '');
  if (f.type === 'relation') { out['postTypes'] = f.postTypes?.length ? f.postTypes : ['post']; if (f.multiple) out['multiple'] = true; }
  if (f.type === 'file' && f.multiple) out['multiple'] = true;
  if (f.type === 'repeater') out['fields'] = (f.fields ?? []).map(fieldBody);
  return out;
}

/** PUT /content-types/{type} body. */
export function specBody(f: TypeSpecForm): Record<string, unknown> {
  return {
    type: f.type.trim(), label: { th: f.label.th.trim(), en: f.label.en.trim() },
    singular: { th: f.singular.th.trim(), en: f.singular.en.trim() }, icon: f.icon.trim(), slug: f.slug.trim(), public: f.public,
    supports: ['title', ...f.supports], fields: f.fields.map(fieldBody),
  };
}

/** Message keys (ct.err.*) of the first problem, '' when the form can be sent. `{x}` = the field. */
export function checkType(f: TypeSpecForm, others: { type: string; slug: string }[], isNew: boolean): { key: string; x?: string } | null {
  const id = f.type.trim(), slug = f.slug.trim();
  if (!f.label.th.trim() && !f.label.en.trim()) return { key: 'ct.err.label' };
  if (!TYPE_RE.test(id)) return { key: 'ct.err.type' };
  if (BUILTIN.has(id)) return { key: 'ct.err.builtin' };
  if (isNew && others.some(o => o.type === id)) return { key: 'ct.err.typeTaken' };
  if (!SLUG_RE.test(slug)) return { key: 'ct.err.slug' };
  if (others.some(o => o.slug === slug && o.type !== id)) return { key: 'ct.err.slugTaken' };
  const bad = checkFields(f.fields, '');
  return bad;
}

function checkFields(list: FieldSpecForm[], parent: string): { key: string; x?: string } | null {
  const seen = new Set<string>();
  for (const fd of list) {
    const k = fd.key.trim(), name = parent + (k || '?');
    if (!KEY_RE.test(k)) return { key: 'ct.err.key', x: name };
    if (seen.has(k)) return { key: 'ct.err.keyTwice', x: name };
    seen.add(k);
    if (fd.type === 'select' && !parseOptions(fd.options ?? '').length) return { key: 'ct.err.options', x: name };
    if (fd.type === 'number' && fd.min != null && fd.max != null && `${fd.min}` !== '' && `${fd.max}` !== '' && Number(fd.min) > Number(fd.max)) {
      return { key: 'ct.err.minMax', x: name };
    }
    if (fd.type === 'repeater') {
      if (!fd.fields?.length) return { key: 'ct.err.repeater', x: name };
      const sub = checkFields(fd.fields, name + '.');
      if (sub) return sub;
    }
  }
  return null;
}

/** Moves item i of list by dir (−1 up, +1 down); a new array. */
export function moved<T>(list: T[], i: number, dir: number): T[] {
  const j = i + dir;
  if (j < 0 || j >= list.length) return list;
  const out = [...list];
  [out[i], out[j]] = [out[j], out[i]];
  return out;
}
