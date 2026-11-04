/**
 * Pure helpers of the Site Code ↔ designer panels (collection params, pickers,
 * input options, binding autocomplete). No Angular here — unit tested in
 * code-logic.spec.ts.
 */
import { CodeBindings, CodePreviewRequest, CodeViewAs } from '../core/code-models';
import { DNode, InputOption, NODE_ICON, Props, findNode, pathTo } from './design-types';
import { CORE_FORMATTERS, ExprCtx, evalExpr, toText } from './expr';

// ------------------------------------------------------------------ sources

export type SourceKind = 'code' | 'table' | 'plugin' | '';
export function parseSource(src: string | undefined): { kind: SourceKind; name: string } {
  if (!src) return { kind: '', name: '' };
  if (src.startsWith('code:')) return { kind: 'code', name: src.slice(5) };
  if (src.startsWith('table:')) return { kind: 'table', name: src.slice(6) };
  return { kind: 'plugin', name: src };
}
export const isCodeSource = (src?: string) => { const k = parseSource(src).kind; return k === 'code' || k === 'table'; };

/** Default URL key of a collection: the node id's letters/digits, up to 12. */
export function defaultKey(id: string): string {
  return id.toLowerCase().replace(/[^a-z0-9]/g, '').slice(0, 12) || 'c';
}
export const validKey = (k: string) => /^[a-z0-9]{1,12}$/.test(k);
export const collectionKey = (n: DNode) => (n.props?.key && validKey(n.props.key) ? n.props.key : defaultKey(n.id));

/** Declared fields of a source (code collection fields / table columns), name → type. */
export function sourceFields(src: string | undefined, b: CodeBindings | null, table?: Props['table']): Record<string, string> {
  const s = parseSource(src);
  if (!b) return {};
  if (s.kind === 'code') return { ...(b.collections.find(c => c.name === s.name)?.fields ?? {}) };
  if (s.kind === 'table') {
    const cols = b.tables.find(t => t.name === s.name)?.columns ?? [];
    const pick = table?.columns?.length ? cols.filter(c => table.columns!.includes(c.name)) : cols;
    return Object.fromEntries(pick.map(c => [c.name, c.type]));
  }
  return {};
}

/** Params a source accepts (code collections only), name → type. */
export function sourceParams(src: string | undefined, b: CodeBindings | null): Record<string, string> {
  const s = parseSource(src);
  return s.kind === 'code' ? { ...(b?.collections.find(c => c.name === s.name)?.params ?? {}) } : {};
}

// ------------------------------------------------------------------ params / input mapping rows

export interface MapRow { name: string; type?: string; expr: string; declared: boolean }

/**
 * Rows of a `{ name: expr }` mapping: every declared name (in declared order),
 * then names mapped but not declared (shown with a warning; the server drops them).
 */
export function mappingRows(declared: Record<string, string> | undefined, mapping: Record<string, string> | undefined): MapRow[] {
  const d = declared ?? {}, m = mapping ?? {};
  const rows: MapRow[] = Object.entries(d).map(([name, type]) => ({ name, type, expr: m[name] ?? '', declared: true }));
  for (const [name, expr] of Object.entries(m)) if (!(name in d)) rows.push({ name, expr, declared: false });
  return rows;
}

/** Sets one mapping entry; an empty expression removes it; an empty mapping becomes undefined. */
export function setMapping(mapping: Record<string, string> | undefined, name: string, expr: string): Record<string, string> | undefined {
  const out = { ...(mapping ?? {}) };
  if (expr.trim()) out[name] = expr.trim();
  else delete out[name];
  return Object.keys(out).length ? out : undefined;
}

export function renameMapping(mapping: Record<string, string> | undefined, from: string, to: string): Record<string, string> | undefined {
  if (!mapping || from === to || !/^[A-Za-z_][\w-]*$/.test(to)) return mapping;
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(mapping)) out[k === from ? to : k] = v;
  return out;
}

/** Evaluates a params mapping in the page context (empty / null values are left out). */
export function evalParams(mapping: Record<string, string> | undefined, ctx: ExprCtx): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, src] of Object.entries(mapping ?? {})) {
    const r = evalExpr(src, ctx);
    if (r.error || r.value === null || r.value === undefined || r.value === '') continue;
    out[k] = r.value;
  }
  return out;
}

/** The POST /code/preview request filling a collection node's preview (first page). */
export function collectionPreviewRequest(n: DNode, b: CodeBindings | null, ctx: ExprCtx, viewAs: CodeViewAs, draft: boolean): CodePreviewRequest | null {
  const src = n.props?.source;
  const s = parseSource(src);
  if (s.kind !== 'code' && s.kind !== 'table') return null;
  const def = s.kind === 'code' ? b?.collections.find(c => c.name === s.name)?.perPage : undefined;
  const max = Math.min(def?.max ?? 50, 100);
  const perPage = Math.max(1, Math.min(n.props?.perPage || def?.default || 10, max));
  const req: CodePreviewRequest = { kind: 'collection', source: src, page: 1, perPage, params: evalParams(n.props?.params, ctx), viewAs };
  if (draft) req.draft = true;
  if (s.kind === 'table' && n.props?.table) req.table = n.props.table;
  return req;
}

// ------------------------------------------------------------------ pickers

export interface LayerChoice { id: string; label: string; depth: number; type: DNode['type']; name: string }

/** Every layer of the document under the root (indented labels), without `excludeId`. */
export function layerChoices(root: DNode, excludeId?: string): LayerChoice[] {
  const out: LayerChoice[] = [];
  const walk = (x: DNode, depth: number) => {
    if (depth > 0 && x.id !== excludeId) {
      const name = x.name || x.type;
      out.push({ id: x.id, depth, type: x.type, name, label: '  '.repeat(depth - 1) + NODE_ICON[x.type] + ' ' + name });
    }
    x.children?.forEach(c => walk(c, depth + 1));
  };
  walk(root, 0);
  return out;
}

/** Layer search (name or type, case-insensitive). */
export function filterLayers(list: LayerChoice[], q: string): LayerChoice[] {
  const s = q.trim().toLowerCase();
  return s ? list.filter(l => l.name.toLowerCase().includes(s) || l.type.toLowerCase().includes(s)) : list;
}

export interface CollectionChoice { id: string; label: string; source: string; key: string }

/** Collections of the document a pagination node / filter can point at: only code: and table: sources page. */
export function collectionChoices(root: DNode, opts: { codeOnly?: boolean; postLists?: boolean } = { codeOnly: true }): CollectionChoice[] {
  const out: CollectionChoice[] = [];
  const walk = (x: DNode) => {
    if (x.type === 'collection' && (!opts.codeOnly || isCodeSource(x.props?.source))) {
      out.push({ id: x.id, label: (x.name || 'Collection') + (x.props?.source ? ` (${x.props.source})` : ''), source: x.props?.source ?? '', key: collectionKey(x) });
    }
    // 2.2.0: a post list takes a search box (?<key>_q) and a category select (?<key>_cat) too
    if (opts.postLists && x.type === 'postList') {
      out.push({ id: x.id, label: (x.name || 'Post list') + ' (posts)', source: '', key: collectionKey(x) });
    }
    x.children?.forEach(walk);
  };
  walk(root);
  return out;
}

/** Nearest collection around a layer (the layer itself excluded). */
export function enclosingCollection(root: DNode, id: string): DNode | null {
  const p = pathTo(root, id);
  for (let i = p.length - 2; i >= 0; i--) if (p[i].type === 'collection') return p[i];
  return null;
}

/** Nearest frame marked as a form around a layer (the layer itself included). */
export function enclosingForm(root: DNode, id: string): DNode | null {
  const p = pathTo(root, id);
  for (let i = p.length - 1; i >= 0; i--) if (p[i].type === 'frame' && p[i].props?.form) return p[i];
  return null;
}

/** Field names of the inputs inside a layer (form values sent with an action). */
export function formInputNames(n: DNode | null): string[] {
  const out: string[] = [];
  const walk = (x: DNode) => {
    if (x.type === 'input' && x.props?.name && !out.includes(x.props.name)) out.push(x.props.name);
    x.children?.forEach(walk);
  };
  if (n) walk(n);
  return out;
}

/**
 * Query-string name a filter input writes for `param` of a collection:
 * the collection's mapping `query.<x>` wins, else `<key>_<param>`.
 */
/** The parameters a post list reads from the URL (2.2.0, queryPosts in render.go): search words and a category slug. */
export const POST_LIST_PARAMS = ['q', 'cat'];

export function filterQueryName(collection: DNode, param: string): string {
  const m = /^\s*query\.([A-Za-z0-9_-]+)\s*$/.exec(collection.props?.params?.[param] ?? '');
  return m ? m[1] : `${collectionKey(collection)}_${param}`;
}

/** Query keys the document reads or writes (filters, params mapping), for autocomplete and condition presets. */
export function docQueryKeys(root: DNode): string[] {
  const keys = new Set<string>();
  const walk = (x: DNode) => {
    for (const v of Object.values(x.props?.params ?? {})) for (const m of v.matchAll(/query\.([A-Za-z0-9_-]+)/g)) keys.add(m[1]);
    const f = x.props?.filter;
    if (x.type === 'input' && f) {
      const c = findNode(root, f.collection)?.node;
      if (c) keys.add(filterQueryName(c, f.param));
    }
    x.children?.forEach(walk);
  };
  walk(root);
  return [...keys];
}

/** Does the page use Site Code features (shows the code toolbar, loads bindings)? */
export function docUsesCode(root: DNode): boolean {
  let used = false;
  const walk = (x: DNode) => {
    const p = x.props;
    if (p?.showWhen || x.type === 'pagination' || x.type === 'input' || p?.action?.type === 'code' || (x.type === 'collection' && isCodeSource(p?.source))) used = true;
    if (!used) x.children?.forEach(walk);
  };
  walk(root);
  return used;
}

/** Does a site.page() pattern ("/events/{slug}", "/docs/*") match a path? */
export function matchPagePath(pattern: string, path: string): boolean {
  const ps = pattern.split('/').filter(Boolean), xs = path.split('?')[0].split('/').filter(Boolean);
  for (let i = 0; i < ps.length; i++) {
    if (ps[i] === '*' && i === ps.length - 1) return xs.length >= i;
    if (i >= xs.length) return false;
    if (/^\{[^}]+\}$/.test(ps[i])) continue;
    if (ps[i] !== xs[i]) return false;
  }
  return ps.length === xs.length;
}

// ------------------------------------------------------------------ input options

/** Options of a select input: from preview items of `optionsFrom`, else the static list. */
export function buildInputOptions(props: Props | undefined, items: Record<string, unknown>[] | null | undefined): InputOption[] {
  const out: InputOption[] = [];
  const seen = new Set<string>();
  const add = (value: string, label: string) => {
    if (seen.has(value)) return;
    seen.add(value);
    out.push({ value, label: label || value });
  };
  const from = props?.optionsFrom;
  if (from?.collection && from.value) {
    for (const it of items ?? []) {
      const v = toText(it[from.value]);
      if (v === '') continue;
      add(v, toText(it[from.label || from.value]));
    }
    return out;
  }
  for (const o of props?.options ?? []) if (o && o.value !== undefined) add(String(o.value), String(o.label ?? ''));
  return out;
}

// ------------------------------------------------------------------ conditions

export interface ConditionPreset { id: string; expr: string; label: string; params?: Record<string, string> }

/**
 * Quick conditions: collection empty ($empty), item field empty / set, query key set,
 * and (phase 4a, when `visitorGroups` is given) visitor signed in / out and
 * visitor in group (one per declared group, plus "in group…" asking for a name).
 */
export function conditionPresets(inCollection: boolean, itemFields: string[], queryKeys: string[], visitorGroups?: string[]): ConditionPreset[] {
  const out: ConditionPreset[] = [];
  if (inCollection) {
    out.push({ id: 'empty', expr: '$empty', label: 'sc.cond.p.empty' });
    for (const f of itemFields) {
      out.push({ id: 'itemEmpty:' + f, expr: `!item.${f}`, label: 'sc.cond.p.itemEmpty', params: { f } });
      out.push({ id: 'itemSet:' + f, expr: `item.${f}`, label: 'sc.cond.p.itemSet', params: { f } });
    }
  }
  for (const k of queryKeys.length ? queryKeys : ['q']) out.push({ id: 'query:' + k, expr: `query.${k}`, label: 'sc.cond.p.query', params: { k } });
  if (visitorGroups) {
    out.push({ id: 'visitor', expr: 'visitor', label: 'sc.cond.p.visitor' });
    out.push({ id: 'visitorOut', expr: '!visitor', label: 'sc.cond.p.visitorOut' });
    for (const g of visitorGroups.filter(validGroup)) out.push({ id: 'group:' + g, expr: `visitor.is.${g}`, label: 'sc.cond.p.group', params: { g } });
    out.push({ id: GROUP_ASK, expr: '', label: 'sc.cond.p.groupAsk' });
  }
  return out;
}

/** Preset id of "visitor in group…" (asks for the group name). */
export const GROUP_ASK = 'group?';
/** Group names usable after `visitor.is.` (identifier syntax of the expression language). */
export const validGroup = (g: string) => /^[A-Za-z_$][A-Za-z0-9_$]*$/.test(g) && !['__proto__', 'constructor', 'prototype'].includes(g);
/** `visitor.is.<g>` for a typed group name (null when it is not a valid name). */
export function groupCondition(g: string): string | null {
  const name = g.trim();
  return validGroup(name) ? `visitor.is.${name}` : null;
}

/** Group list typed in the "view as visitor in group" box ("gold, member" → ['gold', 'member']). */
export function parseGroups(text: string): string[] {
  return [...new Set(text.split(/[\s,]+/).map(s => s.trim()).filter(validGroup))];
}

/** The site visitor the canvas previews with (view as "Visitor in group…"); null otherwise. */
export function previewVisitor(viewAs: CodeViewAs, name: string): Record<string, unknown> | null {
  return typeof viewAs === 'object' ? { id: 'preview', name, groups: [...viewAs.groups] } : null;
}

// ------------------------------------------------------------------ autocomplete

export interface SuggestSource {
  /** Inside a collection: item, $index, $empty are offered. */
  inCollection: boolean;
  itemFields: string[];
  item?: Record<string, unknown> | null;
  page?: Record<string, unknown> | null;
  queryKeys: string[];
  /** Script formatters (bindings.formatters, plugin `ns:name`, enabled aliases); core ones are always offered. */
  formatters: string[];
  /** Origin label per formatter name ('site', plugin id, '→ thai:baht' for an alias); default from the name. */
  formatterDetail?: Record<string, string>;
  /** site.visitorGroups() — offered after `visitor.is.`. */
  visitorGroups?: string[];
  /** Custom field names of the page/post type — offered after `post.meta.`. */
  postMeta?: string[];
}
export interface Suggestion { label: string; insert: string; kind: 'root' | 'field' | 'formatter'; detail?: string }
export interface SuggestResult { from: number; to: number; items: Suggestion[] }

export const POST_KEYS = ['title', 'excerpt', 'date', 'author', 'category', 'url', 'image', 'slug', 'meta'];
/** Keys of the site visitor (site.visitor result) — `is` is the group test `visitor.is.<group>`. */
export const VISITOR_KEYS = ['id', 'name', 'groups', 'is'];
export const SITE_KEYS = ['title', 'tagline', 'url', 'logo', 'icon', 'language'];
const MAX_SUGGESTIONS = 40;

function keysOf(v: unknown): string[] {
  if (Array.isArray(v) || typeof v === 'string') return ['length'];
  return v && typeof v === 'object' ? Object.keys(v) : [];
}

function nested(v: unknown, path: string[]): unknown {
  for (const p of path) {
    if (!v || typeof v !== 'object' || Array.isArray(v)) return undefined;
    v = (v as Record<string, unknown>)[p];
  }
  return v;
}

/**
 * Suggestions for the text before the caret. mode 'template' = text with
 * `{{ }}` (suggests only inside an open `{{`), 'expr' = a bare expression.
 * After `|` → formatter names; after `root.` → keys; else root names.
 */
export function suggest(text: string, caret: number, mode: 'template' | 'expr', src: SuggestSource): SuggestResult | null {
  const before = text.slice(0, caret);
  let expr = before;
  if (mode === 'template') {
    const open = before.lastIndexOf('{{');
    if (open < 0 || before.lastIndexOf('}}') > open) return null;
    expr = before.slice(open + 2);
  }
  if (((expr.match(/'/g)?.length ?? 0) % 2) || ((expr.match(/"/g)?.length ?? 0) % 2)) return null; // inside a string
  // formatter name after `|` — plugin formatters are `ns:name` (`| thai:ba`); `| number:` is an argument
  const fm = /(?:^|[^|])\|\s*([A-Za-z_]\w*:\w*|[A-Za-z_]*)$/.exec(expr);
  const ns = fm && fm[1].includes(':') ? fm[1].slice(0, fm[1].indexOf(':')) : '';
  if (fm && !(CORE_FORMATTERS as readonly string[]).includes(ns)) {
    const prefix = fm[1].toLowerCase();
    const isCore = (n: string) => (CORE_FORMATTERS as readonly string[]).includes(n);
    const names = [...CORE_FORMATTERS, ...src.formatters.filter(f => !isCore(f))];
    const items = names.filter(n => n.toLowerCase().startsWith(prefix))
      .map<Suggestion>(n => ({ label: n, insert: n, kind: 'formatter', detail: isCore(n) ? 'core' : src.formatterDetail?.[n] ?? (n.includes(':') ? n.slice(0, n.indexOf(':')) : 'site') }));
    if (items.length) return { from: caret - fm[1].length, to: caret, items: items.slice(0, MAX_SUGGESTIONS) };
    if (!ns) return null;
    // `| baht:` of a site formatter: an argument follows — fall through to roots
  }
  const pm = /(^|[^\w$.])([$A-Za-z_][\w$]*(?:\.[\w$]*)*)$/.exec(expr);
  let token = '';
  if (pm) token = pm[2];
  else if (expr.trimEnd() !== '' && !/[(!?:*/%+\-<>=&|]$/.test(expr.trimEnd())) return null; // after a value: a number, a string, ")" or a finished path
  const parts = token.split('.');
  const prefix = parts[parts.length - 1];
  let items: Suggestion[];
  if (parts.length === 1) {
    const roots: [string, string][] = [
      ...(src.inCollection ? ([['item', 'sc.ac.item'], ['$index', 'sc.ac.index'], ['$empty', 'sc.ac.empty']] as [string, string][]) : []),
      ['page', 'sc.ac.page'], ['post', 'sc.ac.post'], ['site', 'sc.ac.site'], ['query', 'sc.ac.query'], ['visitor', 'sc.ac.visitor'],
    ];
    items = roots.filter(([r]) => r.startsWith(prefix)).map(([r, d]) => ({ label: r, insert: r, kind: 'root', detail: d }));
  } else {
    const root = parts[0], mid = parts.slice(1, -1);
    let keys: string[] = [];
    if (root === 'item' && src.inCollection) keys = mid.length ? keysOf(nested(src.item, mid)) : [...new Set([...src.itemFields, ...keysOf(src.item)])];
    else if (root === 'page') keys = keysOf(nested(src.page, mid));
    else if (!mid.length && root === 'post') keys = POST_KEYS;
    else if (root === 'post' && mid.length === 1 && mid[0] === 'meta') keys = src.postMeta ?? [];
    else if (!mid.length && root === 'visitor') keys = VISITOR_KEYS;
    else if (root === 'visitor' && mid.length === 1 && mid[0] === 'is') keys = (src.visitorGroups ?? []).filter(validGroup);
    else if (!mid.length && root === 'site') keys = SITE_KEYS;
    else if (!mid.length && root === 'query') keys = src.queryKeys;
    items = keys.filter(k => k.startsWith(prefix) && k !== '__proto__').map(k => ({ label: k, insert: k, kind: 'field', detail: [root, ...mid, k].join('.') }));
  }
  return items.length ? { from: caret - prefix.length, to: caret, items: items.slice(0, MAX_SUGGESTIONS) } : null;
}

/** Text and caret after picking a suggestion. */
export function applySuggestion(text: string, r: SuggestResult, s: Suggestion): { text: string; caret: number } {
  const out = text.slice(0, r.from) + s.insert + text.slice(r.to);
  return { text: out, caret: r.from + s.insert.length };
}
