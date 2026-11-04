import { CodeBuildResult, CodeUploadMode, CodeUploadResult, CodeVersion } from '../core/code-models';

/** Pure helpers of the Site Code screens (no Angular, no Monaco) — unit tested in code-utils.spec.ts. */

// ------------------------------------------------------------------ file rules (mirror the backend contract)

export const CODE_EXTENSIONS = ['.ts', '.js', '.mjs', '.json', '.md'] as const;
/** Static files (2.2.0): allowed under public/, served at /site-assets/ (PublicDir in sitecode/public.go). */
export const PUBLIC_EXTENSIONS = ['.css', '.js', '.mjs', '.json', '.svg', '.txt', '.xml', '.webmanifest', '.md'] as const;
export const CODE_LIMITS = { files: 200, fileBytes: 512 * 1024, totalBytes: 2 * 1024 * 1024, pathLength: 255 } as const;
/** Entry points in the order the bundler looks for them. */
export const CODE_ENTRIES = ['main.ts', 'main.js', 'index.ts', 'index.js'] as const;

const SEGMENT = /^[A-Za-z0-9._-]+$/;

export type PathError = 'empty' | 'tooLong' | 'leadingSlash' | 'backslash' | 'dotSegment' | 'badSegment' | 'badExtension' | 'duplicate';

/** Lower-cased extension including the dot ('' when none). */
export function extOf(path: string): string {
  const name = path.slice(path.lastIndexOf('/') + 1);
  const i = name.lastIndexOf('.');
  return i > 0 ? name.slice(i).toLowerCase() : '';
}

/**
 * Checks a project path like the server does: relative, `/`-separated, segments of
 * `[A-Za-z0-9._-]`, no `.`/`..`, max 255 chars, an allowed extension (files only) and
 * no case-insensitive clash with `existing` (paths of other files). A folder clashes
 * with an existing file of the same name or with an existing folder.
 */
export function validatePath(path: string, existing: readonly string[] = [], kind: 'file' | 'folder' = 'file'): PathError | null {
  if (!path) return 'empty';
  if (path.length > CODE_LIMITS.pathLength) return 'tooLong';
  if (path.includes('\\')) return 'backslash';
  if (path.startsWith('/')) return 'leadingSlash';
  const segments = path.split('/');
  for (const s of segments) {
    if (s === '.' || s === '..') return 'dotSegment';
    if (!SEGMENT.test(s)) return 'badSegment';
  }
  const allowed: readonly string[] = path.startsWith('public/') ? PUBLIC_EXTENSIONS : CODE_EXTENSIONS;
  if (kind === 'file' && !allowed.includes(extOf(path))) return 'badExtension';
  const lower = path.toLowerCase();
  for (const e of existing) {
    const el = e.toLowerCase();
    if (el === lower) return 'duplicate';
    if (kind === 'folder' && el.startsWith(lower + '/')) return 'duplicate';
    if (kind === 'file' && el.startsWith(lower + '/')) return 'duplicate'; // a folder of that name exists
    if (lower.startsWith(el + '/')) return 'duplicate'; // a file is in the way of a folder segment
  }
  return null;
}

/** `dir` + `name` → path ('' dir = project root). */
export function joinPath(dir: string, name: string): string {
  const d = dir.replace(/\/+$/, '');
  return d ? `${d}/${name}` : name;
}

/** Folder that holds `path` ('' for the root). */
export function parentDir(path: string): string {
  const i = path.lastIndexOf('/');
  return i < 0 ? '' : path.slice(0, i);
}

export function baseName(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1);
}

/** Where `path` ends up when `from` (a file or folder) moves to `to`; null when unaffected. */
export function movedPath(path: string, from: string, to: string): string | null {
  if (path === from) return to;
  if (path.startsWith(from + '/')) return to + path.slice(from.length);
  return null;
}

/** Monaco language id for a file. */
export function languageFor(path: string): string {
  switch (extOf(path)) {
    case '.ts': return 'typescript';
    case '.js': case '.mjs': return 'javascript';
    case '.json': return 'json';
    case '.md': return 'markdown';
    case '.css': return 'css';
    case '.svg': case '.xml': return 'xml';
    case '.webmanifest': return 'json';
    default: return 'plaintext';
  }
}

/** First entry point present among `paths` (null = build will fail). */
export function entryPoint(paths: readonly string[]): string | null {
  return CODE_ENTRIES.find(e => paths.includes(e)) ?? null;
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10 * 1024 ? 1 : 0)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

// ------------------------------------------------------------------ file tree

export interface TreeNode {
  name: string;
  /** Full path (folders without a trailing slash). */
  path: string;
  kind: 'folder' | 'file';
  size?: number;
  children: TreeNode[];
}

const byName = (a: TreeNode, b: TreeNode) =>
  a.kind !== b.kind ? (a.kind === 'folder' ? -1 : 1)
    : a.name.localeCompare(b.name, 'en', { sensitivity: 'base', numeric: true }) || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0);

/**
 * Builds the folder tree from flat file paths. Folders are derived from the paths;
 * `extraFolders` adds empty folders created in the UI (not stored until a file is
 * saved in them). Each level lists folders first, then files, by name (case-insensitive,
 * numbers in natural order).
 */
export function buildTree(files: readonly ({ path: string; size?: number } | string)[], extraFolders: readonly string[] = []): TreeNode[] {
  const root: TreeNode = { name: '', path: '', kind: 'folder', children: [] };
  const folders = new Map<string, TreeNode>([['', root]]);
  const folder = (path: string): TreeNode => {
    let f = folders.get(path);
    if (f) return f;
    const parent = folder(parentDir(path));
    f = { name: baseName(path), path, kind: 'folder', children: [] };
    parent.children.push(f);
    folders.set(path, f);
    return f;
  };
  for (const x of extraFolders) if (x) folder(x.replace(/\/+$/, ''));
  for (const x of files) {
    const path = typeof x === 'string' ? x : x.path;
    const size = typeof x === 'string' ? undefined : x.size;
    folder(parentDir(path)).children.push({ name: baseName(path), path, kind: 'file', size, children: [] });
  }
  const sort = (n: TreeNode) => { n.children.sort(byName); n.children.forEach(sort); };
  sort(root);
  return root.children;
}

export interface TreeRow { node: TreeNode; depth: number }

/** Visible rows of the tree: children of collapsed folders are skipped. */
export function flattenTree(nodes: readonly TreeNode[], expanded: ReadonlySet<string>, depth = 0, out: TreeRow[] = []): TreeRow[] {
  for (const n of nodes) {
    out.push({ node: n, depth });
    if (n.kind === 'folder' && expanded.has(n.path)) flattenTree(n.children, expanded, depth + 1, out);
  }
  return out;
}

/** Every folder that contains `path` (to expand the tree down to a file). */
export function ancestors(path: string): string[] {
  const out: string[] = [];
  for (let d = parentDir(path); d; d = parentDir(d)) out.unshift(d);
  return out;
}

// ------------------------------------------------------------------ upload diff

export interface UploadSummary {
  added: string[];
  changed: string[];
  removed: string[];
  unchanged: number;
  /** Files written (added + changed). */
  writes: number;
  /** Nothing would change. */
  empty: boolean;
  /** Draft files would be deleted (replace mode). */
  destructive: boolean;
}

/** Normalises a dry-run answer for display (sorted lists; merge mode never removes). */
export function summarizeUpload(r: CodeUploadResult, mode: CodeUploadMode): UploadSummary {
  const sort = (a?: string[]) => [...(a ?? [])].sort((x, y) => x.localeCompare(y));
  const added = sort(r.added), changed = sort(r.changed);
  const removed = mode === 'replace' ? sort(r.removed) : [];
  return {
    added, changed, removed, unchanged: r.unchanged ?? 0,
    writes: added.length + changed.length,
    empty: !added.length && !changed.length && !removed.length,
    destructive: removed.length > 0,
  };
}

/** Upload of a project zip (both modes) vs a single file (merge only). */
export function isZipName(name: string): boolean {
  return name.toLowerCase().endsWith('.zip');
}

// ------------------------------------------------------------------ build problems

export interface Problem {
  severity: 'error' | 'warning';
  /** Project path as reported (normalised); '' when the message has no file. */
  file: string;
  /** Matching draft file, or null when it is not one of ours. */
  path: string | null;
  line: number;
  column: number;
  message: string;
}

/** Normalises a path from the bundler (`./main.ts`, `/main.ts`, `site-code\\lib\\x.ts`). */
export function normalizeProblemFile(file: string): string {
  return (file ?? '').replace(/\\/g, '/').replace(/^(\.\/)+/, '').replace(/^\/+/, '');
}

/** Finds the draft file a reported path refers to (exact, case-insensitive, then a unique suffix). */
export function resolveProblemPath(file: string, known: readonly string[]): string | null {
  const f = normalizeProblemFile(file);
  if (!f) return null;
  if (known.includes(f)) return f;
  const ci = known.find(k => k.toLowerCase() === f.toLowerCase());
  if (ci) return ci;
  const suffix = known.filter(k => f.endsWith('/' + k));
  return suffix.length === 1 ? suffix[0] : null;
}

/** Errors then warnings, each by file and position; lines/columns are 1-based (min 1). */
export function toProblems(r: Pick<CodeBuildResult, 'errors' | 'warnings'>, known: readonly string[]): Problem[] {
  const map = (severity: Problem['severity']) => (m: CodeBuildResult['errors'][number]): Problem => ({
    severity,
    file: normalizeProblemFile(m.file),
    path: resolveProblemPath(m.file, known),
    line: Math.max(1, Math.floor(m.line || 1)),
    column: Math.max(1, Math.floor(m.column || 1)),
    message: m.message ?? '',
  });
  const order = (a: Problem, b: Problem) => a.file.localeCompare(b.file) || a.line - b.line || a.column - b.column;
  return [...(r.errors ?? []).map(map('error')).sort(order), ...(r.warnings ?? []).map(map('warning')).sort(order)];
}

/** Problems grouped by draft file (for editor markers). */
export function problemsByFile(problems: readonly Problem[]): Map<string, Problem[]> {
  const out = new Map<string, Problem[]>();
  for (const p of problems) {
    if (!p.path) continue;
    const list = out.get(p.path) ?? [];
    list.push(p);
    out.set(p.path, list);
  }
  return out;
}

// ------------------------------------------------------------------ versions

export interface VersionRow {
  id: number;
  label: string;
  message: string;
  when: string;
  by: string;
  files: number;
  live: boolean;
}

/** Rows for the versions table: newest first, '#id' labels, empty messages shown as `untitled`. */
export function versionRows(list: readonly CodeVersion[], fmtDate: (iso: string) => string, untitled: string): VersionRow[] {
  return [...list].sort((a, b) => b.id - a.id).map(v => ({
    id: v.id,
    label: `#${v.id}`,
    message: v.message?.trim() || untitled,
    when: v.createdAt ? fmtDate(v.createdAt) : '',
    by: v.createdBy || '—',
    files: v.files ?? 0,
    live: !!v.live,
  }));
}

// ------------------------------------------------------------------ test route

export const RUN_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as const;

/** `a=1&b=x%20y` (optionally starting with `?`) → {a:'1', b:'x y'}. */
export function parseQuery(s: string): Record<string, string> {
  const out: Record<string, string> = {};
  const q = s.trim().replace(/^\?/, '');
  if (!q) return out;
  for (const [k, v] of new URLSearchParams(q)) if (k) out[k] = v;
  return out;
}

/** One `Name: value` per line → headers object (blank/invalid lines skipped). */
export function parseHeaderLines(s: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of s.split(/\r?\n/)) {
    const i = line.indexOf(':');
    if (i <= 0) continue;
    const k = line.slice(0, i).trim();
    if (k) out[k] = line.slice(i + 1).trim();
  }
  return out;
}

/** Splits `/hello?x=1` into path + query, merging the extra query text. */
export function splitRunPath(path: string, query: string): { path: string; query: Record<string, string> } {
  const p = path.trim() || '/';
  const i = p.indexOf('?');
  const base = i < 0 ? p : p.slice(0, i);
  return { path: base.startsWith('/') ? base : '/' + base, query: { ...(i < 0 ? {} : parseQuery(p.slice(i + 1))), ...parseQuery(query) } };
}

/** Pretty-prints a JSON body; other bodies come back as they are. */
export function prettyBody(body: string | undefined, headers?: Record<string, string | string[]>): { text: string; json: boolean } {
  const text = body ?? '';
  const ct = Object.entries(headers ?? {}).find(([k]) => k.toLowerCase() === 'content-type')?.[1];
  const ctype = (Array.isArray(ct) ? ct.join(';') : ct ?? '').toLowerCase();
  const looks = /^\s*[[{]/.test(text);
  if (ctype.includes('json') || looks) {
    try { return { text: JSON.stringify(JSON.parse(text), null, 2), json: true }; } catch { /* not JSON */ }
  }
  return { text, json: false };
}

/** File name from a Content-Disposition header (RFC 5987 `filename*` first). */
export function filenameFromDisposition(h: string | null | undefined, fallback: string): string {
  if (!h) return fallback;
  const star = /filename\*\s*=\s*(?:UTF-8|utf-8)''([^;]+)/.exec(h);
  if (star) { try { return decodeURIComponent(star[1].trim().replace(/^"|"$/g, '')); } catch { /* ignore */ } }
  const plain = /filename\s*=\s*"?([^";]+)"?/.exec(h);
  return plain ? plain[1].trim() : fallback;
}
