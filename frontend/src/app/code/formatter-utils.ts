/**
 * Pure helpers of the Site Code "Formatters" tab (phase 6, G1): plugin
 * formatters are registered as `<pluginId>:<name>` (`{{ price | thai:baht }}`);
 * an admin may enable the unprefixed alias (`| baht`). Conflict rules (same as
 * the server): core formatters win, then Site Code's own formatter, then the
 * alias already enabled (one target per alias name). No Angular here — unit
 * tested in formatter-utils.spec.ts.
 */
import { CodeFormatter } from '../core/code-models';
import { CORE_FORMATTERS } from '../editor/expr';

/** alias name → full plugin formatter name: `{"baht": "thai:baht"}` (PUT /code/formatters/aliases). */
export type FormatterAliases = Record<string, string>;

const NAME = /^[A-Za-z_][A-Za-z0-9_]*$/;

/** "thai:baht" → "baht"; a Site Code formatter keeps its name. */
export const shortFormatterName = (name: string) => (name.includes(':') ? name.slice(name.indexOf(':') + 1) : name);
/** Plugin formatter (`ns:name`)? */
export const isPluginFormatter = (f: Pick<CodeFormatter, 'name' | 'unit'>) => f.name.includes(':') && f.unit !== 'site';

/**
 * GET /code/formatters → list. Accepts `[...]`, `{items}` or `{formatters}`,
 * string rows (`"thai:baht"`, `"price"`), fills `unit` from the prefix,
 * drops malformed / duplicate names; site first, then by unit and name.
 */
export function normalizeFormatters(raw: unknown): CodeFormatter[] {
  const r = raw as { items?: unknown; formatters?: unknown } | null;
  const list: unknown[] = Array.isArray(raw) ? raw : Array.isArray(r?.items) ? (r!.items as unknown[]) : Array.isArray(r?.formatters) ? (r!.formatters as unknown[]) : [];
  const out: CodeFormatter[] = [];
  const seen = new Set<string>();
  for (const x of list) {
    const o = (typeof x === 'string' ? { name: x } : x) as Partial<CodeFormatter> | null;
    if (!o || typeof o.name !== 'string') continue;
    const name = o.name.trim();
    const parts = name.split(':');
    if (parts.length > 2 || !parts.every(p => NAME.test(p)) || seen.has(name)) continue;
    seen.add(name);
    const unit = typeof o.unit === 'string' && o.unit ? o.unit : parts.length === 2 ? parts[0] : 'site';
    out.push({
      name, unit,
      ...(typeof o.alias === 'string' && o.alias ? { alias: o.alias } : {}),
      ...(typeof o.conflict === 'string' && o.conflict ? { conflict: o.conflict } : {}),
    });
  }
  return out.sort((a, b) => (a.unit === 'site' ? 0 : 1) - (b.unit === 'site' ? 0 : 1) || a.unit.localeCompare(b.unit) || a.name.localeCompare(b.name));
}

/** Enabled aliases as the server reports them (rows with `alias`). */
export function aliasesOf(list: CodeFormatter[]): FormatterAliases {
  const out: FormatterAliases = {};
  for (const f of list) if (f.alias && isPluginFormatter(f) && !(f.alias in out)) out[f.alias] = f.name;
  return out;
}

export type AliasConflictKind = 'core' | 'site' | 'plugin';
export interface AliasConflict { kind: AliasConflictKind; by: string }

/**
 * Why the alias of `f` cannot be (or is not) active: a core formatter or a
 * Site Code formatter with the same name, or another plugin formatter that
 * already holds the alias. null = free (or held by `f` itself).
 */
export function aliasConflict(f: CodeFormatter, list: CodeFormatter[], aliases: FormatterAliases): AliasConflict | null {
  const short = shortFormatterName(f.name);
  if ((CORE_FORMATTERS as readonly string[]).includes(short)) return { kind: 'core', by: short };
  if (list.some(x => !isPluginFormatter(x) && x.name === short)) return { kind: 'site', by: short };
  const holder = aliases[short];
  if (holder && holder !== f.name) return { kind: 'plugin', by: holder };
  return null;
}

/** Is the alias of `f` switched on? */
export const aliasEnabled = (f: CodeFormatter, aliases: FormatterAliases) => aliases[shortFormatterName(f.name)] === f.name;

/**
 * Aliases after switching `f` on/off. Switching on never steals an alias
 * held by another plugin formatter, nor shadows a core / Site Code one
 * (the map is returned unchanged).
 */
export function toggleAlias(aliases: FormatterAliases, f: CodeFormatter, on: boolean, list: CodeFormatter[]): FormatterAliases {
  const short = shortFormatterName(f.name);
  const next = { ...aliases };
  if (!on) { if (next[short] === f.name) delete next[short]; return next; }
  if (!isPluginFormatter(f) || aliasConflict(f, list, aliases)) return aliases;
  next[short] = f.name;
  return next;
}

/** Plugin formatters grouped by plugin id (Site Code's own listed separately). */
export function groupFormatters(list: CodeFormatter[]): { unit: string; formatters: CodeFormatter[] }[] {
  const units = [...new Set(list.map(f => (isPluginFormatter(f) ? f.unit : 'site')))];
  return units.map(unit => ({ unit, formatters: list.filter(f => (isPluginFormatter(f) ? f.unit : 'site') === unit) }));
}

/**
 * Names the designer autocomplete offers after `|` (besides the core ones):
 * Site Code formatters (bindings + list), plugin formatters (`thai:baht`) and
 * enabled aliases (`baht`) — with a short origin label.
 */
export function suggestFormatterNames(bindings: readonly string[] | null | undefined, list: CodeFormatter[]): { names: string[]; detail: Record<string, string> } {
  const names: string[] = [];
  const detail: Record<string, string> = {};
  const add = (n: string, d: string) => { if (n && !names.includes(n) && !(CORE_FORMATTERS as readonly string[]).includes(n)) { names.push(n); detail[n] = d; } };
  for (const n of bindings ?? []) if (typeof n === 'string') add(n, n.includes(':') ? n.slice(0, n.indexOf(':')) : 'site');
  for (const f of list) if (!isPluginFormatter(f)) add(f.name, 'site');
  for (const f of list) if (isPluginFormatter(f)) add(f.name, f.unit);
  const aliases = aliasesOf(list);
  for (const [a, target] of Object.entries(aliases)) {
    const f = list.find(x => x.name === target);
    if (f && !aliasConflict(f, list, aliases)) add(a, '→ ' + target);
  }
  return { names, detail };
}
