/**
 * Pure helpers of the Site Code access screen (/code/access): grouping,
 * role columns, checkbox state and the override to send (null = back to
 * the code default). Unit tested in access-utils.spec.ts.
 */
import { CodeAccessKind, CodeAccessRow } from '../core/code-models';

export const ACCESS_KINDS: CodeAccessKind[] = ['route', 'collection', 'action', 'page', 'table', 'adminPage'];
export const EVERYONE = '*';
export const ADMIN = 'administrator';
/** Access entries for site visitor groups (site.visitor / site.visitorGroups): 'group:<name>'. */
export const GROUP_PREFIX = 'group:';
export const isGroupEntry = (r: string) => r.startsWith(GROUP_PREFIX);
export const groupName = (r: string) => (isGroupEntry(r) ? r.slice(GROUP_PREFIX.length) : r);

/** Roles when there is no admin override: the code's `access`, else administrator only (page: everyone; table: nobody). */
export function defaultRoles(kind: string, codeDefault: string[] | null | undefined): string[] {
  if (codeDefault) return codeDefault;
  if (kind === 'page') return [EVERYONE];
  if (kind === 'table') return [];
  return [ADMIN];
}

/** Unique, sorted, '*' first — so two role lists can be compared. */
export function normRoles(roles: readonly string[] | null | undefined): string[] {
  const set = [...new Set((roles ?? []).filter(r => !!r))];
  return set.sort((a, b) => (a === EVERYONE ? -1 : b === EVERYONE ? 1 : a.localeCompare(b)));
}

export function sameRoles(a: readonly string[] | null | undefined, b: readonly string[] | null | undefined): boolean {
  const x = normRoles(a), y = normRoles(b);
  return x.length === y.length && x.every((r, i) => r === y[i]);
}

export const isOverridden = (row: CodeAccessRow) => row.override !== null && row.override !== undefined;

/** Rows grouped by kind (route, collection, action, page, table, adminPage, then any other kind), sorted by name; empty groups left out. */
export function groupAccess(rows: CodeAccessRow[], q = ''): { kind: string; rows: CodeAccessRow[] }[] {
  const s = q.trim().toLowerCase();
  const list = s ? rows.filter(r => r.name.toLowerCase().includes(s) || labelText(r).toLowerCase().includes(s)) : rows;
  const kinds = [...ACCESS_KINDS, ...new Set(list.map(r => r.kind).filter(k => !ACCESS_KINDS.includes(k)))];
  return kinds
    .map(kind => ({ kind, rows: list.filter(r => r.kind === kind).sort((a, b) => a.name.localeCompare(b.name)) }))
    .filter(g => g.rows.length > 0);
}

function labelText(r: CodeAccessRow): string {
  const l = r.label;
  return !l ? '' : typeof l === 'string' ? l : [l.th, l.en].filter(Boolean).join(' ');
}

/** Column order: Everyone ('*'), the known roles (administrator first), then other slugs found in the rows (visitor groups excluded — see groupColumns). */
export function roleColumns(roles: { slug: string; name?: string }[], rows: CodeAccessRow[]): string[] {
  const out: string[] = [EVERYONE];
  const add = (r: string) => { if (r && !out.includes(r)) out.push(r); };
  add(ADMIN);
  roles.forEach(r => add(r.slug));
  for (const row of rows) for (const r of [...(row.codeDefault ?? []), ...(row.override ?? []), ...(row.effective ?? [])]) if (!isGroupEntry(r)) add(r);
  return out;
}

/**
 * Visitor group columns ('group:<name>'): the groups declared with
 * site.visitorGroups() in their order, then groups only found in the rows.
 */
export function groupColumns(declared: readonly string[] | null | undefined, rows: CodeAccessRow[]): string[] {
  const out: string[] = [];
  const add = (name: string) => { const c = GROUP_PREFIX + name; if (name && /^[A-Za-z0-9_.:-]{1,64}$/.test(name) && !out.includes(c)) out.push(c); };
  (declared ?? []).forEach(g => add(String(g)));
  for (const row of rows) for (const r of [...(row.codeDefault ?? []), ...(row.override ?? []), ...(row.effective ?? [])]) if (isGroupEntry(r)) add(groupName(r));
  return out;
}

/** Is the checkbox of `role` ticked? Administrator always; a role is implied when everyone is allowed. */
export function cellState(row: CodeAccessRow, role: string): { checked: boolean; locked: boolean; implied: boolean } {
  const eff = row.effective ?? [];
  if (role === ADMIN) return { checked: true, locked: true, implied: false };
  // admin pages always need a signed-in user: "Everyone" can only be switched off (phase 6)
  if (role === EVERYONE) return { checked: eff.includes(EVERYONE), locked: row.kind === 'adminPage' && !eff.includes(EVERYONE), implied: false };
  if (eff.includes(EVERYONE)) return { checked: true, locked: true, implied: true };
  return { checked: eff.includes(role), locked: false, implied: false };
}

/** Effective roles after ticking / unticking one role. */
export function toggleRole(effective: readonly string[], role: string, on: boolean): string[] {
  const set = new Set(effective);
  if (on) set.add(role); else set.delete(role);
  return normRoles([...set]);
}

/** What to PUT for a row once its roles become `roles`: null when that equals the code default (override removed). */
export function overrideFor(row: CodeAccessRow, roles: readonly string[]): string[] | null {
  return sameRoles(roles, defaultRoles(row.kind, row.codeDefault)) ? null : normRoles(roles);
}

/** The row as it will look after saving (optimistic update). */
export function applyOverride(row: CodeAccessRow, override: string[] | null): CodeAccessRow {
  return { ...row, override, effective: override ?? normRoles(defaultRoles(row.kind, row.codeDefault)) };
}
