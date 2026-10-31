/**
 * Pure helpers of Site Code "units" (phase 4b): 'site' (the Site Code) plus
 * one unit per active JavaScript plugin. Registrations of a plugin are named
 * `<id>:<name>`, its settings keys are stored as `<id>.<key>`. These helpers
 * normalise the unit list, tell which unit a row belongs to (from a `unit`
 * field, else from the name prefix) and group designer sources by unit.
 * No Angular here — unit tested in units-utils.spec.ts.
 */
import { CodeLabel, CodeUnitInfo } from '../core/code-models';

export const SITE_UNIT = 'site';

export interface CodeUnit { id: string; name: string; label?: CodeLabel; error?: string | null; version?: string }

const UNIT_ID_RE = /^[a-z0-9][a-z0-9-]{1,39}$/;

/**
 * Unit list from /code/status `units` (ids or objects) or from installed JS
 * plugins; 'site' always comes first, duplicates and inactive units dropped.
 */
export function normalizeUnits(raw: (string | CodeUnitInfo)[] | null | undefined, fromPackages: { id: string; name?: string; runtime?: string; active?: boolean; version?: string }[] = []): CodeUnit[] {
  const out: CodeUnit[] = [{ id: SITE_UNIT, name: SITE_UNIT }];
  const seen = new Set([SITE_UNIT]);
  const add = (u: CodeUnit) => { if (!seen.has(u.id) && UNIT_ID_RE.test(u.id)) { seen.add(u.id); out.push(u); } };
  for (const x of raw ?? []) {
    if (typeof x === 'string') add({ id: x, name: x });
    else if (x && typeof x.id === 'string' && x.active !== false) add({ id: x.id, name: x.name || x.id, label: x.label, error: x.error ?? null, version: x.version });
  }
  if (!raw?.length) {
    for (const p of fromPackages) if ((p.runtime ?? '').toLowerCase() === 'js' && p.active !== false) add({ id: p.id, name: p.name || p.id, version: p.version });
  }
  return out;
}

/** A requested unit that is not in the list falls back to 'site'. */
export function pickUnit(want: string | null | undefined, units: CodeUnit[]): string {
  return want && units.some(u => u.id === want) ? want : SITE_UNIT;
}

/** Unit of a registration named `<id>:<name>` (plugin) or plain (site). Route names ("GET /a/:id") never count. */
export function unitOfName(name: string, unit?: string | null, pluginIds?: string[]): string {
  if (unit) return unit;
  const m = /^([a-z0-9][a-z0-9-]{1,39}):(?!\/)/.exec(name ?? '');
  if (!m) return SITE_UNIT;
  return !pluginIds || pluginIds.includes(m[1]) ? m[1] : SITE_UNIT;
}

/** The name without its `<unit>:` prefix (for display under a unit heading). */
export function shortName(name: string, unit: string): string {
  return unit !== SITE_UNIT && name.startsWith(unit + ':') ? name.slice(unit.length + 1) : name;
}

/** Rows of one unit: by their `unit` field when the server sends it, else by name prefix. */
export function filterByUnit<T extends { name: string; unit?: string }>(rows: T[], unit: string, pluginIds: string[]): T[] {
  return rows.filter(r => unitOfName(r.name, r.unit, pluginIds) === unit);
}

/** Settings keys of a plugin may come back as `<id>.<key>`; the tab shows (and saves) the bare key. */
export function settingKeyForUnit(key: string, unit: string): string {
  return unit !== SITE_UNIT && key.startsWith(unit + '.') ? key.slice(unit.length + 1) : key;
}

/** Settings values of one unit (rows of other units dropped when the server marks them). */
export function settingsForUnit<T extends { key: string; unit?: string }>(values: T[], unit: string): T[] {
  return values.filter(v => !v.unit || v.unit === unit).map(v => ({ ...v, key: settingKeyForUnit(v.key, unit) }));
}

export interface UnitGroup<T> { unit: string; items: T[] }

/** Designer pickers: items grouped by unit, 'site' first, then plugins by id; order inside a group kept. */
export function groupByUnit<T extends { name: string; unit?: string }>(items: T[]): UnitGroup<T>[] {
  const map = new Map<string, T[]>();
  for (const it of items) {
    const u = unitOfName(it.name, it.unit);
    if (!map.has(u)) map.set(u, []);
    map.get(u)!.push(it);
  }
  return [...map.entries()]
    .sort(([a], [b]) => (a === SITE_UNIT ? -1 : b === SITE_UNIT ? 1 : a.localeCompare(b)))
    .map(([unit, list]) => ({ unit, items: list }));
}
