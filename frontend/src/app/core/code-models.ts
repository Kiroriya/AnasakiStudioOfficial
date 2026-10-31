/** Types of the Site Code admin API (/api/v1/code, phase 1). */

export interface CodeRoute { method: string; path: string; access: string[] }
export interface CodeRegistered { routes: CodeRoute[]; events: string[]; visitorGroups?: string[] }

/** GET /code/status */
export interface CodeStatus {
  safeMode: boolean;
  envDisabled: boolean;
  demo: boolean;
  liveVersion: number | null;
  /** The draft differs from the live version. */
  draftChanged: boolean;
  registered: CodeRegistered;
  lastError: string | null;
  /** Phase 2: database mode and number of migrations not run yet. */
  db?: CodeStatusDb;
  /** Phase 4b: code units ('site' + active JS plugins), as ids or objects. */
  units?: (string | CodeUnitInfo)[];
}

/** One draft file in GET /code/files (sorted by path). */
export interface CodeFileInfo { path: string; size: number; updatedAt: string; updatedBy: string }
/** GET/PUT /code/files/{path} */
export interface CodeFile { path: string; content: string; updatedAt: string }
/** Body of a 409 from PUT /code/files/{path}. */
export interface CodeConflict { error: string; current: CodeFile }

/** POST /code/upload (dry run or real). */
export interface CodeUploadResult { added: string[]; changed: string[]; removed: string[]; unchanged: number }
export type CodeUploadMode = 'merge' | 'replace';

export interface CodeBuildMessage { file: string; line: number; column: number; message: string }
/** POST /code/build */
export interface CodeBuildResult { ok: boolean; errors: CodeBuildMessage[]; warnings: CodeBuildMessage[]; registered?: CodeRegistered; migrations?: CodeBuildMigrations }

/** POST /code/run */
export interface CodeRunRequest { method: string; path: string; query: Record<string, string>; headers: Record<string, string>; body: string }
export interface CodeRunLog { level: string; message: string }
export interface CodeRunResult {
  status?: number;
  headers?: Record<string, string | string[]>;
  body?: string;
  logs?: CodeRunLog[];
  durationMs?: number;
  error?: string;
}

/** One row of GET /code/versions (newest first). */
export interface CodeVersion { id: number; message: string; createdAt: string; createdBy: string; live: boolean; files: number }
/** GET /code/versions/{id} */
export interface CodeVersionDetail extends Omit<CodeVersion, 'files'> { files: { path: string; size: number }[] }

export type CodeLogLevel = 'debug' | 'info' | 'warn' | 'error';
export interface CodeLog { id: number; at: string; level: CodeLogLevel | string; source: string; message: string; trace: string | null; version: number | null; unit?: string }
export interface CodeLogList { items: CodeLog[]; total: number }

// ------------------------------------------------------------------ phase 2: the site's own database (/code/db)

/** One destructive change found in a pending migration (drop/rename table or column, narrowing change). */
export interface CodeDestructive { migration?: string; op: string; table: string; column?: string | null }
/** A pending migration as computed by the dry run of POST /code/build. */
export interface CodePendingMigration { name: string; sql: string[]; destructive?: CodeDestructive[] }
/** POST /code/build — phase 2 adds the migration dry run. */
export interface CodeBuildMigrations { pending: CodePendingMigration[] }
/** GET /code/status — phase 2 adds the database summary. */
export interface CodeStatusDb { mode: CodeDbMode; pendingMigrations: number }
/** 400 body of POST /code/publish when destructive migrations were not confirmed. */
export interface CodePublishRefusal { error: string; destructive?: CodeDestructive[]; errors?: CodeBuildMessage[] }

export type CodeDbMode = 'separate' | 'fallback';

/** GET /code/db/status */
export interface CodeDbStatus {
  mode: CodeDbMode;
  database: string;
  configured: boolean;
  /** The SQL statements to create the code DB/user (only when not configured). */
  setupSql?: string[];
  sizeBytes: number;
  tables: number;
}

/** One row of GET /code/db/tables. */
export interface CodeDbTable { name: string; rows: number; sizeBytes: number; comment?: string; system?: boolean; view?: boolean }

export interface CodeDbColumn { name: string; type: string; nullable: boolean; default: string | null; key: string; extra: string; comment: string }
export interface CodeDbIndex { name: string; columns: string[]; unique: boolean }
export interface CodeDbForeignKey { column: string; refTable: string; refColumn: string; onDelete?: string }
/** GET /code/db/tables/{t} — `relations` are metadata-only links to core_* views (no FK constraint). */
export interface CodeDbTableDetail {
  name: string;
  columns: CodeDbColumn[];
  indexes: CodeDbIndex[];
  foreignKeys: CodeDbForeignKey[];
  relations?: CodeDbForeignKey[];
}

export type CodeDbRow = Record<string, unknown>;
/** GET /code/db/tables/{t}/rows — primaryKey is null/empty for a table without a single-column key (read-only). */
export interface CodeDbRows { items: CodeDbRow[]; total: number; primaryKey: string | null }

/** POST /code/db/tables/{t}/import */
export interface CodeDbImportError { row: number; message: string }
export interface CodeDbImportResult { inserted: number; errors: CodeDbImportError[] }

/** POST /code/db/sql */
export interface CodeDbSqlRequest { sql: string; params: unknown[]; write: boolean }
export interface CodeDbSqlResult { columns?: string[]; rows?: (CodeDbRow | unknown[])[]; affectedRows?: number; durationMs?: number; truncated?: boolean }

/** One row of the migration history (GET /code/db/migrations). */
export interface CodeDbMigration {
  name: string;
  batch: number;
  ranAt: string;
  ranBy: string;
  durationMs: number;
  status: string;
  error?: string | null;
}
/** A migration not run yet: in the draft only, or already in the live version. */
export interface CodeDbPendingInfo { name: string; source?: 'draft' | 'live' | string }

/** GET /code/db/backups */
export interface CodeDbBackup { id: number; tableName: string; backupTable: string; migration: string; createdAt: string; expiresAt: string }

// ------------------------------------------------------------------ phase 3: designer bindings, preview, access

/** A label from site.collection({label}) — a string or {th,en}. */
export type CodeLabel = string | { th?: string; en?: string };

/** GET /code/bindings — what the live (or draft) code offers the designer. */
export interface CodeBindings {
  collections: { name: string; label?: CodeLabel; fields: Record<string, string>; params?: Record<string, string>; perPage?: { default?: number; max?: number }; unit?: string }[];
  actions: { name: string; input?: Record<string, string>; unit?: string }[];
  pages: { path: string }[];
  formatters: string[];
  tables: { name: string; columns: { name: string; type: string }[] }[];
  /** Phase 4a (optional — older servers leave them out). */
  postTypes?: CodePostType[];
  postFields?: Record<string, Record<string, CodeFieldSpec>>;
  visitorGroups?: string[];
  adminPages?: CodeAdminPage[];
}

/** POST /code/preview */
export interface CodePreviewRequest {
  kind: 'collection' | 'page';
  source?: string;
  path?: string;
  page?: number;
  perPage?: number;
  params?: Record<string, unknown>;
  /** Phase 4a: `{groups}` previews as a site visitor in those groups (site.visitor). */
  viewAs: CodeViewAs;
  draft?: boolean;
  /** table: sources — the node's props.table (extra field, see report). */
  table?: unknown;
}
export interface CodePreviewResult { items?: Record<string, unknown>[]; total?: number; data?: Record<string, unknown>; error?: string }

export type CodeAccessKind = 'route' | 'collection' | 'action' | 'page' | 'table' | 'adminPage';
/** GET /code/access row. Roles are role slugs; '*' = everyone (not signed in). */
export interface CodeAccessRow {
  kind: CodeAccessKind;
  name: string;
  label?: CodeLabel;
  codeDefault: string[] | null;
  override: string[] | null;
  effective: string[];
  /** Phase 4b: the unit that registered it ('site' or a plugin id). */
  unit?: string;
}

// ------------------------------------------------------------------ phase 4a: visitor, post types, settings, jobs, admin pages

/** "View as" of designer previews: not signed in, the CMS user, or a site visitor in some groups (site.visitor). */
export type CodeViewAs = 'anonymous' | 'me' | { groups: string[] };

/** Field types of site.postType({fields}) / site.fields(). */
export type CodeFieldType = 'text' | 'textarea' | 'number' | 'bool' | 'date' | 'datetime' | 'select' | 'url' | 'image' | 'email' | 'json';

/** One custom field declaration; the code may also write just the type name (`place: 'text'`). */
export interface CodeFieldDef {
  type: CodeFieldType | string;
  label?: CodeLabel;
  required?: boolean;
  min?: number;
  max?: number;
  /** select: `['a','b']` or `[{value,label}]`. */
  options?: (string | { value: string; label?: CodeLabel })[];
  help?: CodeLabel;
  placeholder?: string;
  default?: unknown;
  /** Set on fields a JS plugin added with site.fields (key "<plugin>.<key>"). */
  plugin?: string;
  /** The plugin's display name (panel heading). */
  pluginName?: string;
}
export type CodeFieldSpec = CodeFieldDef | string;

/** A post type registered with site.postType (GET /code/bindings `postTypes`, or GET /post-types). */
export interface CodePostType {
  type: string;
  label?: CodeLabel;
  singular?: CodeLabel;
  icon?: string;
  /** Public URL prefix (default = type). */
  slug?: string;
  fields?: Record<string, CodeFieldSpec>;
  /** Default title + content + excerpt + image. */
  supports?: string[];
  /** The signed-in user may edit posts of this type (access.edit roles). */
  canEdit?: boolean;
  public?: boolean;
}

/** An admin page from site.adminPage (sidebar group Developer → /x/code/<id>). */
export interface CodeAdminPage { id: string; label?: CodeLabel; icon?: string }

/** GET /code/settings */
export interface CodeSettingValue { key: string; secret: boolean; set: boolean; value?: string; unit?: string }
export interface CodeSettings { values: CodeSettingValue[]; fetchHosts: string[]; limits: { mailPerHour: number } }
/** PUT /code/settings — secret values are write-only: '' = keep the stored one. */
export interface CodeSettingsUpdate {
  values: { key: string; secret: boolean; value: string }[];
  fetchHosts: string[];
  limits: { mailPerHour: number };
  remove: string[];
}

export type CodeJobStatus = 'pending' | 'running' | 'done' | 'failed' | 'cancelled';
/** One row of GET /code/jobs. */
export interface CodeJob {
  id: number;
  name: string;
  data?: unknown;
  status: CodeJobStatus | string;
  attempts: number;
  maxAttempts: number;
  runAt?: string | null;
  startedAt?: string | null;
  finishedAt?: string | null;
  progress?: string | null;
  result?: string | null;
  error?: string | null;
  createdAt?: string | null;
  dedupeKey?: string | null;
  unit?: string;
}
export interface CodeJobList { items: CodeJob[]; total: number }
/** One row of GET /code/cron. */
export interface CodeCron { name: string; expr: string; nextRun?: string | null; lastRun?: string | null; lastStatus?: string | null; unit?: string }

// ------------------------------------------------------------------ phase 4b: code units, hooks, export as plugin

/** A code unit: 'site' (Site Code) or an active JS plugin. */
export interface CodeUnitInfo { id: string; name?: string; label?: CodeLabel; active?: boolean; error?: string | null; version?: string }

/** One subscriber of a hook (GET /code/hooks), in effective order. */
export interface CodeHookSubscriber { unit: string; priority: number; file?: string; line?: number }
/** GET /code/hooks row. */
export interface CodeHook { name: string; kind: 'filter' | 'event' | string; declaredBy: string; subscribers: CodeHookSubscriber[] }

/** POST /code/package body. */
export interface CodePackageRequest {
  id: string; name: string; version: string; description?: string;
  scopes?: string[]; routes?: string[]; library?: string; exports?: string[];
}
/** The manifest the export would write (dry run). */
export interface CodePackageManifest {
  id?: string; name?: string; version?: string; description?: string; runtime?: string; permissions?: string[];
  code?: { entry?: string; scopes?: string[]; hosts?: string[]; routes?: string[]; library?: string | null; exports?: string[]; hooks?: { filters?: string[]; events?: string[] } };
  requires?: Record<string, string>;
}
/** POST /code/package?dryRun=1 */
export interface CodePackageDryRun { manifest: CodePackageManifest; warnings?: (string | { message?: string; file?: string; line?: number })[] }

// ------------------------------------------------------------------ phase 6: plugin formatters

/**
 * GET /code/formatters row. `name` = "thai:baht" (plugin) or "price" (Site Code);
 * `unit` = plugin id or 'site'; `alias` = the unprefixed name when enabled;
 * `conflict` = why the alias is not active (server text, shown as-is).
 */
export interface CodeFormatter { name: string; unit: string; alias?: string; conflict?: string }
