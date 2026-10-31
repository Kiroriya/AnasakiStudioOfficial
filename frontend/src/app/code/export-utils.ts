/**
 * Pure helpers of the "Export as plugin" dialog (POST /code/package, phase 4b):
 * prefill from the dry run's manifest, validation, the request body and the
 * warnings' text. No Angular here — unit tested in export-utils.spec.ts.
 */
import { CodePackageDryRun, CodePackageRequest } from '../core/code-models';

/** Same rules as the CMS package parser (pkgzip.go). */
export const PKG_ID_RE = /^[a-z0-9][a-z0-9-]{2,39}$/;
export const SEMVER_RE = /^(\d{1,6})\.(\d{1,6})\.(\d{1,6})(?:-([0-9A-Za-z.-]{1,40}))?(?:\+[0-9A-Za-z.-]{1,40})?$/;
/** Paths a plugin may not claim. */
export const RESERVED_ROUTES = ['/admin', '/api', '/_a', '/ext-ui', '/uploads', '/assets', '/preview'];
const ROUTE_RE = /^\/[A-Za-z0-9._~\-/]*$/;

export interface ExportForm {
  id: string;
  name: string;
  version: string;
  description: string;
  scopes: string[];
  /** One route prefix per line (or comma separated). */
  routes: string;
  exports: string[];
  library: string;
}

/** "My Shop!" → "my-shop" (padded to 3 chars, cut at 40). */
export function slugId(s: string): string {
  let id = (s ?? '').toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40).replace(/-+$/, '');
  if (!id) id = 'site-code';
  while (id.length < 3) id += '-x';
  return id;
}

/** The dialog's start values: the dry run's manifest, else sensible defaults. */
export function formFromDryRun(dry: CodePackageDryRun | null | undefined, siteTitle = ''): ExportForm {
  const m = dry?.manifest ?? {};
  const c = m.code ?? {};
  const name = m.name || siteTitle || 'Site code';
  return {
    id: m.id && PKG_ID_RE.test(m.id) ? m.id : slugId(m.id || siteTitle || name),
    name,
    version: m.version && SEMVER_RE.test(m.version) ? m.version : '1.0.0',
    description: m.description ?? '',
    scopes: [...new Set(c.scopes ?? [])],
    routes: (c.routes ?? []).join('\n'),
    exports: [...new Set(c.exports ?? [])],
    library: c.library ?? '',
  };
}

/** Route prefixes typed in the textarea. */
export function parseRoutes(text: string): string[] {
  return [...new Set((text ?? '').split(/[\n,]+/).map(r => r.trim().replace(/\/+$/, '') || (r.trim() ? '/' : '')).filter(Boolean))];
}

export type ExportErrors = Partial<Record<'id' | 'name' | 'version' | 'description' | 'routes', string>>;

/** Message keys (cx.err.*) per field; `{r}` of routes = the first bad route. */
export function validateExport(f: ExportForm): { errors: ExportErrors; badRoute?: string } {
  const errors: ExportErrors = {};
  const id = f.id.trim();
  if (id === 'site') errors.id = 'cx.err.reserved';
  else if (!PKG_ID_RE.test(id)) errors.id = 'cx.err.id';
  if (!f.name.trim() || f.name.trim().length > 80) errors.name = 'cx.err.name';
  if (!SEMVER_RE.test(f.version.trim())) errors.version = 'cx.err.version';
  if (f.description.length > 500) errors.description = 'cx.err.description';
  const bad = parseRoutes(f.routes).find(r => r === '/' || !ROUTE_RE.test(r) || RESERVED_ROUTES.some(x => r === x || r.startsWith(x + '/')));
  if (bad) errors.routes = 'cx.err.route';
  return { errors, badRoute: bad };
}

export const exportValid = (f: ExportForm) => Object.keys(validateExport(f).errors).length === 0;

/** POST /code/package body (empty optional fields left out). */
export function exportRequest(f: ExportForm): CodePackageRequest {
  const body: CodePackageRequest = { id: f.id.trim(), name: f.name.trim(), version: f.version.trim(), scopes: [...f.scopes] };
  if (f.description.trim()) body.description = f.description.trim();
  const routes = parseRoutes(f.routes);
  if (routes.length) body.routes = routes;
  if (f.exports.length) body.exports = [...f.exports];
  if (f.library.trim()) body.library = f.library.trim();
  return body;
}

/** Warnings arrive as strings or `{message, file, line}`. */
export function warningText(w: string | { message?: string; file?: string; line?: number }): string {
  if (typeof w === 'string') return w;
  const where = w?.file ? ` (${w.file}${w.line ? ':' + w.line : ''})` : '';
  return (w?.message ?? '') + where;
}

export function toggleIn(list: string[], v: string, on: boolean): string[] {
  return on ? (list.includes(v) ? list : [...list, v]) : list.filter(x => x !== v);
}
