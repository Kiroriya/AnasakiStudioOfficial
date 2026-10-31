/**
 * Pure helpers of the Plugins page for JavaScript plugins (Site Code phase 4b):
 * semver ranges of `requires`, dependency status against the installed
 * packages, code scopes (labels / sensitivity), consent-dialog content and the
 * error bodies of install / activate / uninstall (missing, destructive,
 * dependents). No Angular here — unit tested in plugin-deps.spec.ts.
 */
import { CodeDestructive } from '../core/code-models';
import { tr } from '../core/i18n';
import { InstalledPackage, PackageCode, PackageRequireEntry, PackageRequiresRaw, Reconsent } from '../core/models';
import './messages-plugins4b';

// ------------------------------------------------------------------ semver

type Ver = [number, number, number, string];

/** "1.2.3-beta+x" → [1,2,3,'beta']; partial versions fill zeros; null when unreadable. */
export function parseVersion(v: string | null | undefined): Ver | null {
  const m = /^\s*v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?\s*$/.exec(v ?? '');
  return m ? [+m[1], +(m[2] ?? 0), +(m[3] ?? 0), m[4] ?? ''] : null;
}

export function compareVersions(a: string, b: string): number {
  const x = parseVersion(a), y = parseVersion(b);
  if (!x || !y) return (a ?? '').localeCompare(b ?? '');
  for (let i = 0; i < 3; i++) if (x[i] !== y[i]) return (x[i] as number) < (y[i] as number) ? -1 : 1;
  if (x[3] === y[3]) return 0;
  if (!x[3]) return 1; // a release is newer than its prerelease
  if (!y[3]) return -1;
  return x[3] < y[3] ? -1 : 1;
}

/** One comparator → [op, version] pairs (x-ranges, caret and tilde expand to two). */
function comparators(part: string): [string, string][] | null {
  const m = /^(\^|~|>=|<=|>|<|=)?\s*v?([0-9xX*]+)(?:\.([0-9xX*]+))?(?:\.([0-9xX*]+))?(-[0-9A-Za-z.-]+)?$/.exec(part.trim());
  if (!m) return null;
  const op = m[1] ?? '';
  const wild = (s?: string) => s === undefined || /^[xX*]$/.test(s);
  const [M, mi, pa] = [m[2], m[3], m[4]];
  if (wild(M)) return op === '<' || op === '>' ? [['<', '0.0.0']] : [];
  const maj = +M, min = wild(mi) ? null : +mi!, pat = wild(pa) ? null : +pa!;
  const full = `${maj}.${min ?? 0}.${pat ?? 0}${m[5] ?? ''}`;
  if (op === '^') {
    const upper = maj > 0 || min === null ? `${maj + 1}.0.0` : min > 0 || pat === null ? `0.${min + 1}.0` : `0.0.${pat + 1}`;
    return [['>=', full], ['<', upper]];
  }
  if (op === '~') return [['>=', full], ['<', min === null ? `${maj + 1}.0.0` : `${maj}.${min + 1}.0`]];
  if (min === null || pat === null) {
    // x-range: 1.x / 1.2 / 1.2.x
    const upper = min === null ? `${maj + 1}.0.0` : `${maj}.${min + 1}.0`;
    switch (op) {
      case '': case '=': return [['>=', full], ['<', upper]];
      case '>': return [['>=', upper]];
      case '<=': return [['<', upper]];
      default: return [[op, full]];
    }
  }
  return [[op || '=', full]];
}

/**
 * Does `version` satisfy `range`? Supports `^1.2.3`, `~1.2`, `1.x`, `*`,
 * `>=1.0.0 <2.0.0`, `1.2.3`, and `||` alternatives. An unreadable range is false.
 */
export function satisfies(version: string | null | undefined, range: string | null | undefined): boolean {
  if (!parseVersion(version)) return false;
  const r = (range ?? '').trim();
  if (!r || r === '*' || r.toLowerCase() === 'x' || r === 'latest') return true;
  return r.split('||').some(alt => {
    const parts = alt.trim().replace(/(>=|<=|>|<|=|\^|~)\s+/g, '$1').split(/\s+/).filter(Boolean);
    if (!parts.length) return true;
    for (const p of parts) {
      const cs = comparators(p);
      if (!cs) return false;
      for (const [op, v] of cs) {
        const c = compareVersions(version!, v);
        const ok = op === '>=' ? c >= 0 : op === '<=' ? c <= 0 : op === '>' ? c > 0 : op === '<' ? c < 0 : c === 0;
        if (!ok) return false;
      }
    }
    return true;
  });
}

// ------------------------------------------------------------------ requires / dependents

export interface PackageRequire { id: string; range: string; installed?: string | null; latest?: string | null; name?: string }

/** `requires` as a manifest map `{id: range}` or a resolved list → a sorted list. */
export function normalizeRequires(raw: PackageRequiresRaw | null | undefined): PackageRequire[] {
  if (!raw || typeof raw !== 'object') return [];
  const list: PackageRequire[] = Array.isArray(raw)
    ? raw.filter((x): x is PackageRequireEntry => !!x && typeof x.id === 'string')
        .map(x => ({ id: x.id, range: x.range ?? '*', installed: x.installed ?? null, latest: x.latest ?? x.version ?? null, ...(x.name ? { name: x.name } : {}) }))
    : Object.entries(raw).map(([id, range]) => ({ id, range: String(range ?? '*') }));
  return list.sort((a, b) => a.id.localeCompare(b.id));
}

export type DepState = 'ok' | 'missing' | 'incompatible' | 'inactive';
export interface DepStatus extends PackageRequire { state: DepState; installedVersion: string | null; name?: string }

/** Status of each required plugin against what is installed here. */
export function dependencyStatus(reqs: PackageRequire[], installed: Pick<InstalledPackage, 'id' | 'version' | 'active' | 'name'>[]): DepStatus[] {
  return reqs.map(r => {
    const p = installed.find(x => x.id === r.id);
    if (!p) return { ...r, state: 'missing', installedVersion: null };
    const state: DepState = !satisfies(p.version, r.range) ? 'incompatible' : !p.active ? 'inactive' : 'ok';
    return { ...r, state, installedVersion: p.version, name: p.name || r.name };
  });
}

// ------------------------------------------------------------------ optionalRequires ("works with", phase 6)

export type OptionalDepState = 'ok' | 'inactive' | 'missing' | 'incompatible';
export type OptionalDepStatus = DepStatus & { state: OptionalDepState };

/**
 * Status of each `optionalRequires` plugin (manifest map or resolved list,
 * malformed input → []): installed ✓, installed but off, not installed, or
 * incompatible version ⚠. Informational only — never blocks anything.
 */
export function optionalStatus(raw: unknown, installed: Pick<InstalledPackage, 'id' | 'version' | 'active' | 'name'>[]): OptionalDepStatus[] {
  if (!raw || typeof raw !== 'object') return [];
  const list = Array.isArray(raw) ? raw.filter(x => x && typeof x === 'object' && typeof x.id === 'string' && x.id)
    : Object.fromEntries(Object.entries(raw as Record<string, unknown>).filter(([id, r]) => id && (typeof r === 'string' || r === null || r === undefined)));
  const seen = new Set<string>();
  return dependencyStatus(normalizeRequires(list as PackageRequiresRaw), installed).filter(d => !seen.has(d.id) && !!seen.add(d.id)) as OptionalDepStatus[];
}

export const optionalIcon = (d: Pick<DepStatus, 'state'>) => (d.state === 'ok' ? '✓' : d.state === 'incompatible' ? '⚠' : d.state === 'inactive' ? '⏸' : '○');

/** Dependencies that must be installed (or updated) before this plugin installs. */
export function missingDeps(reqs: PackageRequire[], installed: Pick<InstalledPackage, 'id' | 'version' | 'active' | 'name'>[]): DepStatus[] {
  return dependencyStatus(reqs, installed).filter(d => d.state === 'missing' || d.state === 'incompatible');
}

/** Installed packages that require `id` (from `dependents`, else from the others' `requires`). */
export function dependentsOf(id: string, all: Pick<InstalledPackage, 'id' | 'requires' | 'dependents'>[]): string[] {
  const own = all.find(p => p.id === id)?.dependents;
  if (own?.length) return [...own];
  return all.filter(p => p.id !== id && normalizeRequires(p.requires).some(r => r.id === id)).map(p => p.id);
}

/** Pretty names for plugin ids (falls back to the id). */
export function namesOf(ids: string[], all: Pick<InstalledPackage, 'id' | 'name'>[]): string[] {
  return ids.map(i => all.find(p => p.id === i)?.name || i);
}

// ------------------------------------------------------------------ error bodies

interface ErrLike { status?: number; error?: unknown }
function body(e: unknown): Record<string, unknown> | null {
  const b = (e as ErrLike)?.error;
  if (b && typeof b === 'object' && !(typeof Blob !== 'undefined' && b instanceof Blob)) return b as Record<string, unknown>;
  if (typeof b === 'string') { try { const j = JSON.parse(b); return j && typeof j === 'object' ? j : null; } catch { return null; } }
  return null;
}

/** 400 `{error, missing:[{id, range, installed?}]}` of install/upload/activate. */
export function missingFromError(e: unknown): PackageRequire[] {
  const m = body(e)?.['missing'];
  if (!Array.isArray(m)) return [];
  return m.filter(x => x && typeof x.id === 'string').map(x => ({ id: x.id, range: x.range ?? '*', installed: x.installed ?? null, latest: x.latest ?? x.version ?? null }));
}

/** 409 `{error, dependents:[ids]}` of deactivate / uninstall. */
export function dependentsFromError(e: unknown): string[] {
  const d = body(e)?.['dependents'];
  return Array.isArray(d) ? d.map(x => (typeof x === 'string' ? x : x?.id)).filter((x): x is string => typeof x === 'string') : [];
}

/** 400 `{error, destructive:[…]}`: migrations that may lose data, to be confirmed. */
export function destructiveFromError(e: unknown): CodeDestructive[] {
  const d = body(e)?.['destructive'];
  return Array.isArray(d) ? d.filter(x => x && typeof x === 'object' && typeof x.op === 'string') : [];
}

/** "drop_column posts.x (shop/002_x.ts)" */
export function destructiveText(d: CodeDestructive): string {
  const target = d.column ? `${d.table}.${d.column}` : d.table;
  return `${d.op} ${target}` + (d.migration ? ` (${d.migration})` : '');
}

// ------------------------------------------------------------------ code scopes

export const CODE_SCOPES = ['db', 'content', 'content-write', 'mail', 'http', 'visitor', 'admin-pages', 'jobs', 'routes', 'filters-core', 'content-fields', 'webhooks'] as const;
export type CodeScope = (typeof CODE_SCOPES)[number];
/** Scopes reviewers and admins should look at twice. */
export const SENSITIVE_SCOPES: readonly string[] = ['content-write', 'mail', 'http', 'visitor', 'filters-core', 'content-fields', 'webhooks'];
export const isSensitiveScope = (s: string) => SENSITIVE_SCOPES.includes(s);
/** Message key of a scope label (`scope.<id>`); unknown scopes show their id. */
export const scopeKey = (s: string) => 'scope.' + s;
/** th/en label of a code scope in the current language (unknown scopes show their id). */
export function scopeLabel(s: string): string {
  const k = scopeKey(s), t = tr(k);
  return t === k ? s : t;
}

/** Scopes sorted in the canonical order, unknown ones last (deduplicated). */
export function sortScopes(scopes: string[] | null | undefined): string[] {
  const u = [...new Set(scopes ?? [])];
  const idx = (s: string) => { const i = (CODE_SCOPES as readonly string[]).indexOf(s); return i < 0 ? 999 : i; };
  return u.sort((a, b) => idx(a) - idx(b) || a.localeCompare(b));
}

export const isJsPlugin = (p: { runtime?: string } | null | undefined) => (p?.runtime ?? '').toLowerCase() === 'js';

// ------------------------------------------------------------------ consent dialog

export type ConsentMode = 'install' | 'upgrade' | 'reconsent' | 'activate';
export interface ConsentState {
  mode: ConsentMode;
  name: string;
  version?: string;
  runtime?: string;
  permissions: string[];
  /** Service API scopes (service plugins). */
  api: string[];
  scopes: string[];
  hosts: string[];
  routes: string[];
  destructive: CodeDestructive[];
}

/** Consent content from a manifest-like source (store detail, installed package or reconsent). */
export function consentFor(mode: ConsentMode, src: { name: string; version?: string; runtime?: string; permissions?: string[]; code?: PackageCode | null }, rc?: Reconsent | null, destructive: CodeDestructive[] = []): ConsentState {
  if (rc) {
    return {
      mode, name: src.name, version: src.version, runtime: src.runtime, permissions: [...(rc.permissions ?? [])], api: [...(rc.api ?? [])],
      scopes: sortScopes(rc.scopes), hosts: [...(rc.hosts ?? [])], routes: [], destructive,
    };
  }
  return {
    mode, name: src.name, version: src.version, runtime: src.runtime, permissions: [...(src.permissions ?? [])], api: [],
    scopes: sortScopes(src.code?.scopes), hosts: [...(src.code?.hosts ?? [])], routes: [...(src.code?.routes ?? [])], destructive,
  };
}

/** Anything to show at all? (an install without permissions or code needs no question) */
export const consentNeeded = (c: ConsentState) =>
  c.permissions.length + c.api.length + c.scopes.length + c.hosts.length + c.destructive.length > 0 || c.mode === 'reconsent';

/** The confirm button is enabled once destructive migrations are acknowledged. */
export const consentCanConfirm = (c: ConsentState, destructiveChecked: boolean) => !c.destructive.length || destructiveChecked;
