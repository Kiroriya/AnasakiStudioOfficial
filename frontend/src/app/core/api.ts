import { HttpClient, HttpParams, HttpResponse } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { DNode } from '../editor/design-types';
import {
  DesignComponent, Media, Menu, Page, PluginInfo, Post, ServiceHealth, DemoStatus, PostInput, PostType, Revision,
  Settings, Stats, Taxonomy, Term, User, UserList, RolesResponse, RoleInfo, SetupStatus, PublicSite, InstalledPackage, BlockInfo,
  StoreSummary, StoreDetail, StoreCategory, StoreReview,
  SchemaField, ApiToken, ApiTokenCreate,
} from './models';
import {
  CodeBuildResult, CodeFile, CodeFileInfo, CodeLogList, CodeRunRequest, CodeRunResult, CodeStatus, CodeUploadMode, CodeUploadResult,
  CodeVersion, CodeVersionDetail, CodeDbStatus, CodeDbTable, CodeDbTableDetail, CodeDbRows, CodeDbRow, CodeDbImportResult,
  CodeDbSqlRequest, CodeDbSqlResult, CodeDbBackup, CodeBindings, CodePreviewRequest, CodePreviewResult, CodeAccessRow, CodeAccessKind,
  CodePostType, CodeAdminPage, CodeSettings, CodeSettingsUpdate, CodeJobList, CodeCron, CodeHook, CodePackageRequest, CodePackageDryRun, CodeFormatter,
} from './code-models';

import type { LegacySummary, RecommendedInstall, RecommendedList, ToolsImportResult } from './legacy';

const BASE = '/api/v1';

/** Where the public site lives (same origin in production, Go server in dev). */
export const SITE_ORIGIN = location.port === '4200' ? 'http://localhost:8090' : '';

/** Thin promise-based wrapper around the Go REST API. */
@Injectable({ providedIn: 'root' })
export class Api {
  private http = inject(HttpClient);

  private get<T>(url: string, params?: Record<string, string | number | undefined>) {
    let p = new HttpParams();
    for (const [k, v] of Object.entries(params ?? {})) if (v !== undefined && v !== '') p = p.set(k, String(v));
    return firstValueFrom(this.http.get<T>(BASE + url, { params: p }));
  }
  private post<T>(url: string, body: unknown) { return firstValueFrom(this.http.post<T>(BASE + url, body)); }
  private put<T>(url: string, body: unknown) { return firstValueFrom(this.http.put<T>(BASE + url, body)); }
  private del(url: string) { return firstValueFrom(this.http.delete<void>(BASE + url)); }

  // auth
  login(email: string, password: string) { return this.post<{ token: string; user: User }>('/auth/login', { email, password }); }
  me() { return this.get<User>('/auth/me'); }
  /** Revokes this session on the server; `all` signs the account out everywhere. */
  logout(all = false) { return this.post<void>('/auth/logout', all ? { all: true } : {}); }
  /** A fresh session token (sliding expiry while the admin is open). */
  refreshSession() { return this.post<{ token: string; user: User }>('/auth/refresh', {}); }
  updateMe(data: Partial<User> & { password?: string; currentPassword?: string }) { return this.put<User & { token?: string }>('/auth/me', data); }
  setupStatus() { return this.get<SetupStatus>('/setup'); }
  runSetup(d: { siteTitle: string; tagline: string; language: string; adminName: string; adminEmail: string; adminPassword: string; locale: string; demo: boolean; setupToken?: string }) {
    return this.post<{ token: string; user: User }>('/setup', d);
  }
  forgotPassword(email: string) { return this.post<{ message: string }>('/auth/forgot', { email }); }
  resetPassword(token: string, password: string) { return this.post<{ token: string; user: User }>('/auth/reset', { token, password }); }
  /** Personal access tokens: own ones, or a user's (`user`, manage_users only). */
  apiTokens(user?: number) { return this.get<{ items?: ApiToken[] } | ApiToken[]>('/auth/tokens', { user }); }
  createApiToken(body: ApiTokenCreate) { return this.post<{ token: string; item: ApiToken }>('/auth/tokens', body); }
  revokeApiToken(id: number, user?: number) { return this.del(`/auth/tokens/${id}` + (user ? `?user=${user}` : '')); }
  mailStatus() { return this.get<{ configured: boolean }>('/mail/status'); }
  testMail(to: string) { return this.post<{ sentTo: string }>('/mail/test', { to }); }
  publicSite() { return firstValueFrom(this.http.get<PublicSite>('/api/public/site')); }
  stats() { return this.get<Stats>('/stats'); }

  // posts / pages / templates
  posts(q: { type: PostType; status?: string; search?: string; category?: string; page?: number; perPage?: number; order?: string }) {
    return this.get<Page<Post>>('/posts', q);
  }
  getPost(id: number) { return this.get<Post>(`/posts/${id}`); }
  createPost(data: PostInput) { return this.post<Post>('/posts', data); }
  updatePost(id: number, data: PostInput) { return this.put<Post>(`/posts/${id}`, data); }
  deletePost(id: number) { return this.del(`/posts/${id}`); }
  duplicatePost(id: number) { return this.post<Post>(`/posts/${id}/duplicate`, {}); }
  revisions(id: number) { return this.get<Revision[]>(`/posts/${id}/revisions`); }
  restoreRevision(id: number, rid: number) { return this.post<Post>(`/posts/${id}/revisions/${rid}/restore`, {}); }
  revision(id: number, rid: number) { return this.get<Revision>(`/posts/${id}/revisions/${rid}`); }
  componentRevisions(id: number) { return this.get<Revision[]>(`/components/${id}/revisions`); }
  componentRevision(id: number, rid: number) { return this.get<Revision>(`/components/${id}/revisions/${rid}`); }
  /** Saves a hand-made order (first id = 1). */
  orderPosts(type: PostType, ids: number[]) { return this.put<void>('/posts/order', { type, ids }); }

  // taxonomy
  terms(taxonomy = 'category') { return this.get<Term[]>('/terms', { taxonomy }); }
  /** "category" and the site.taxonomy taxonomies (CMS 2.1.0). */
  taxonomies() { return this.get<Taxonomy[]>('/taxonomies'); }
  /** {id, name} of users for "user" fields (edit_others_posts; CMS 2.1.0). */
  userOptions(q: { search?: string; ids?: string }) { return this.get<{ id: number; name: string }[]>('/users/options', q); }
  createTerm(t: Partial<Term>) { return this.post<Term>('/terms', t); }
  updateTerm(id: number, t: Partial<Term>) { return this.put<Term>(`/terms/${id}`, t); }
  deleteTerm(id: number) { return this.del(`/terms/${id}`); }

  // media
  media(q: { kind?: string; search?: string } = {}) { return this.get<Media[]>('/media', q); }
  /** Downloads a file from a public URL into the media library. */
  importMedia(url: string, alt = '') { return this.post<Media>('/media/import', { url, alt }); }
  mediaPage(q: { kind?: string; search?: string; page: number; perPage: number }) { return this.get<{ items: Media[]; total: number }>('/media', q); }
  upload(file: File) {
    const fd = new FormData();
    fd.append('file', file);
    return firstValueFrom(this.http.post<Media>(BASE + '/media', fd));
  }
  updateMedia(id: number, alt: string) { return this.put<Media>(`/media/${id}`, { alt }); }
  deleteMedia(id: number) { return this.del(`/media/${id}`); }

  // menus
  menus() { return this.get<Menu[]>('/menus'); }
  saveMenu(m: Menu) { return m.id ? this.put<Menu>(`/menus/${m.id}`, m) : this.post<Menu>('/menus', m); }
  deleteMenu(id: number) { return this.del(`/menus/${id}`); }

  // components
  components() { return this.get<DesignComponent[]>('/components'); }
  saveComponent(c: { id?: number; name: string; node: DNode }) {
    return c.id ? this.put<DesignComponent>(`/components/${c.id}`, c) : this.post<DesignComponent>('/components', c);
  }
  deleteComponent(id: number) { return this.del(`/components/${id}`); }
  /** Points every link to a page's old address at its new one (designs, components, menus). */
  replaceLinks(from: string, to: string) { return this.post<{ changed: number }>('/links/replace', { from, to }); }
  /** Pages, templates and components that place this component. */
  componentUsage(id: number) { return this.get<ComponentUse[]>(`/components/${id}/usage`); }
  /** Makes a component the site header/footer on every page (swapping the current one). */
  setComponentSiteRole(id: number, role: 'header' | 'footer') {
    return this.post<{ replaced: number; added: number }>(`/components/${id}/site-role`, { role });
  }
  /** Placements saved anywhere that show variant `from` of this component switch to `to`. */
  renameVariant(id: number, from: string, to: string) {
    return this.post<{ updated: number }>(`/components/${id}/variants/rename`, { from, to });
  }

  // settings & users
  settings() { return this.get<Settings>('/settings'); }
  saveSettings(s: Settings) { return this.put<Settings>('/settings', s); }
  /** Paths the admin path / URL bases may not use: the core's and those the installed plugins declare (since 2.0.4). */
  reservedPaths() { return this.get<{ items: ReservedPath[] }>('/settings/reserved-paths'); }
  users(q: { role?: string; search?: string; status?: string; page?: number; perPage?: number } = {}) { return this.get<UserList>('/users', q); }
  bulkUsers(ids: number[], action: 'role' | 'suspend' | 'activate' | 'delete', role?: string) { return this.post<{ updated: number }>('/users/bulk', { ids, action, role }); }
  createUser(u: Partial<User> & { password: string }) { return this.post<User>('/users', u); }
  updateUser(id: number, u: Partial<User> & { password?: string }) { return this.put<User>(`/users/${id}`, u); }
  deleteUser(id: number, reassign?: number) { return this.del(`/users/${id}` + (reassign ? `?reassign=${reassign}` : '')); }
  roles() { return this.get<RolesResponse>('/roles'); }
  saveRole(r: Partial<RoleInfo>, isNew: boolean) { return isNew ? this.post<RoleInfo>('/roles', r) : this.put<RoleInfo>(`/roles/${r.slug}`, r); }
  deleteRole(slug: string) { return this.del(`/roles/${slug}`); }
  /** Brings back deleted built-in roles with their default permissions. */
  demoSnapshot() { return this.post<DemoStatus>('/demo/snapshot', {}); }
  demoReset() { return this.post<DemoStatus>('/demo/reset', {}); }
  restoreRoles() { return this.post<RolesResponse>('/roles/restore', {}); }

  // plugins
  plugins() { return this.get<PluginInfo[]>('/plugins'); }
  setPluginActive(id: string, active: boolean) { return this.post<PluginInfo>(`/plugins/${id}/${active ? 'activate' : 'deactivate'}`, {}); }
  pluginSettings(id: string) { return this.get<{ schema: SchemaField[]; values: Record<string, unknown> }>(`/plugin-settings/${id}`); }
  savePluginSettings(id: string, values: Record<string, unknown>) { return this.put<{ schema: SchemaField[]; values: Record<string, unknown> }>(`/plugin-settings/${id}`, { values }); }

  // package plugins + store
  packages() { return this.get<InstalledPackage[]>('/package-plugins'); }
  /** Phase 4b: `confirmDestructive` approves destructive migrations of a JS plugin (a 400 `{error, destructive}` asks for it). */
  uploadPackage(file: File, opts: { confirmDestructive?: boolean } = {}) {
    const fd = new FormData();
    fd.append('file', file);
    let p = new HttpParams();
    if (opts.confirmDestructive) { p = p.set('confirmDestructive', '1'); fd.append('confirmDestructive', '1'); }
    return firstValueFrom(this.http.post<InstalledPackage>(BASE + '/package-plugins/upload', fd, { params: p }));
  }
  /** A 400 `{error, missing:[…]}` lists dependencies to install first; `{error, destructive:[…]}` asks for confirmDestructive. */
  /** 409 `{error, publisherChanged}`: the update comes from another Store publisher; `acceptPublisher` installs it anyway. */
  installPackage(id: string, version?: string, opts: { confirmDestructive?: boolean; acceptPublisher?: boolean } = {}) {
    return this.post<InstalledPackage>('/package-plugins/install', { id, version, ...(opts.confirmDestructive ? { confirmDestructive: true } : {}),
      ...(opts.acceptPublisher ? { acceptPublisher: true } : {}) });
  }
  /** `force` also deactivates plugins that require this one (else 409 `{error, dependents}`). */
  setPackageActive(id: string, active: boolean, opts: { force?: boolean; confirmDestructive?: boolean } = {}) {
    const q = [opts.force ? 'force=1' : '', opts.confirmDestructive ? 'confirmDestructive=1' : ''].filter(Boolean).join('&');
    return this.post<InstalledPackage>(`/package-plugins/${id}/${active ? 'activate' : 'deactivate'}` + (q ? '?' + q : ''), opts.confirmDestructive ? { confirmDestructive: true } : {});
  }
  /** `deleteData` drops the plugin's tables (after a backup); `force` deactivates its dependents. */
  deletePackage(id: string, opts: { deleteData?: boolean; force?: boolean } = {}) {
    const q = [opts.deleteData ? 'deleteData=1' : '', opts.force ? 'force=1' : ''].filter(Boolean).join('&');
    return this.del(`/package-plugins/${id}` + (q ? '?' + q : ''));
  }
  savePackageSettings(id: string, values: Record<string, unknown>) { return this.put<InstalledPackage>(`/package-plugins/${id}/settings`, { values }); }
  /** Service plugins: where the plugin's server program listens, its shared secret, a connection test, admin screens. */
  setServiceUrl(id: string, url: string) { return this.put<InstalledPackage>(`/package-plugins/${id}/service`, { url }); }
  rotateServiceSecret(id: string) { return this.post<InstalledPackage>(`/package-plugins/${id}/service/secret`, {}); }
  serviceHealth(id: string) { return this.get<ServiceHealth>(`/package-plugins/${id}/service/health`); }
  serviceTicket(id: string, page: string) { return this.post<{ url: string }>(`/package-plugins/${id}/ui-ticket`, { page }); }
  blocks() { return this.get<BlockInfo[]>('/blocks'); }
  /** Built-in icons and decorative shapes (design.AssetCatalog). */
  designAssets() { return this.get<DesignAssets>('/design/assets'); }
  renderBlock(plugin: string, block: string, values: Record<string, unknown>) { return this.post<{ html: string; css: string }>('/blocks/render', { plugin, block, values }); }
  storeCategories() { return this.get<StoreCategory[]>('/store/categories'); }
  storePlugins(q: { category?: string; search?: string; sort?: string; page?: number; perPage?: number } = {}) { return this.get<Page<StoreSummary>>('/store/plugins', q); }
  storePlugin(id: string) { return this.get<StoreDetail>(`/store/plugins/${id}`); }
  storeReviews(id: string) { return this.get<StoreReview[]>(`/store/plugins/${id}/reviews`); }
  storeReview(id: string, r: { stars: number; comment: string; author: string }) { return this.post<{ rating: number; ratingCount: number }>(`/store/plugins/${id}/reviews`, r); }
  storeRankings() { return this.get<{ trending: StoreSummary[]; topRated: StoreSummary[]; topDownloads: StoreSummary[] }>('/store/rankings'); }

  // Export / import site (core system tool since phase 7; the former backup plugin)
  toolsExport() { return firstValueFrom(this.http.get(BASE + '/tools/export', { responseType: 'blob' })); }
  toolsImport(file: File) {
    const fd = new FormData();
    fd.append('file', file);
    return firstValueFrom(this.http.post<ToolsImportResult>(BASE + '/tools/import', fd));
  }

  // phase 7: migration of the former built-in plugins + setup wizard recommended plugins
  legacyMigrations() { return this.get<LegacySummary>('/legacy-migrations'); }
  legacyRetry(id?: string) { return this.post<LegacySummary>('/legacy-migrations/retry', id ? { id } : {}); }
  legacyUpload(file: File) {
    const fd = new FormData();
    fd.append('file', file);
    return firstValueFrom(this.http.post<LegacySummary>(BASE + '/legacy-migrations/upload', fd));
  }
  legacyCleanup(id: string) { return this.post<LegacySummary>(`/legacy-migrations/${encodeURIComponent(id)}/cleanup`, { confirm: true }); }
  setupRecommended() { return this.get<RecommendedList>('/setup/recommended'); }
  setupInstallRecommended(ids: string[]) { return this.post<RecommendedInstall>('/setup/recommended/install', { ids }); }
  collection(source: string, limit = 6) { return this.get<Record<string, string>[]>(`/collections/${source}`, { limit }); }


  // site code (/code, phase 1)
  codeStatus() { return this.get<CodeStatus>('/code/status'); }
  codeFiles() { return this.get<CodeFileInfo[]>('/code/files'); }
  codeFile(path: string) { return this.get<CodeFile>('/code/files/' + codePath(path)); }
  /** `baseUpdatedAt` = updatedAt the edit started from ('' for a new file); a newer stored copy answers 409 {error, current}. */
  saveCodeFile(path: string, content: string, baseUpdatedAt: string) { return this.put<CodeFile>('/code/files/' + codePath(path), { content, baseUpdatedAt }); }
  deleteCodeFile(path: string) { return this.del('/code/files/' + codePath(path)); }
  deleteCodeFolder(folder: string) { return this.del('/code/files/' + codePath(folder) + '/?recursive=1'); }
  moveCode(from: string, to: string) { return this.post<void>('/code/files/move', { from, to }); }
  uploadCode(file: File, o: { dryRun: boolean; mode: CodeUploadMode; dir: string }) {
    const fd = new FormData();
    fd.append('file', file);
    let p = new HttpParams().set('mode', o.mode).set('dir', o.dir);
    if (o.dryRun) p = p.set('dryRun', '1');
    return firstValueFrom(this.http.post<CodeUploadResult>(BASE + '/code/upload', fd, { params: p }));
  }
  /** path '' = whole draft (zip), folder = zip, file = the file itself. */
  downloadCode(path = '') { return this.blob('/code/download', { path }); }
  buildCode() { return this.post<CodeBuildResult>('/code/build', {}); }
  runCode(r: CodeRunRequest) { return this.post<CodeRunResult>('/code/run', r); }
  /** `confirmDestructive` (phase 2) lets pending migrations that drop/rename/narrow data run; without it such a publish answers 400 {error, destructive}. */
  publishCode(message: string, baseVersion: number | null, confirmDestructive = false) {
    return this.post<{ version: CodeVersion }>('/code/publish', { message, baseVersion, confirmDestructive });
  }
  codeVersions() { return this.get<CodeVersion[]>('/code/versions'); }
  codeVersion(id: number) { return this.get<CodeVersionDetail>(`/code/versions/${id}`); }
  codeVersionFile(id: number, path: string) { return this.get<{ path: string; content: string }>(`/code/versions/${id}/files/` + codePath(path)); }
  downloadCodeVersion(id: number) { return this.blob(`/code/versions/${id}/download`); }
  restoreCodeVersion(id: number, alsoDraft: boolean) { return this.post<{ version: CodeVersion }>(`/code/versions/${id}/restore`, { alsoDraft }); }
  setCodeSafeMode(enabled: boolean) { return this.post<CodeStatus>('/code/safe-mode', { enabled }); }
  codeLogs(q: { level?: string; source?: string; q?: string; page?: number; perPage?: number; unit?: string }) { return this.get<CodeLogList>('/code/logs', q); }
  downloadCodeLogs(q: { level?: string; from?: string; to?: string; unit?: string }) { return this.blob('/code/logs/download', q); }
  clearCodeLogs() { return this.del('/code/logs'); }
  // site code database (/code/db, phase 2)
  codeDbStatus() { return this.get<CodeDbStatus>('/code/db/status'); }
  codeDbTables() { return this.get<CodeDbTable[]>('/code/db/tables'); }
  codeDbTable(t: string) { return this.get<CodeDbTableDetail>('/code/db/tables/' + encodeURIComponent(t)); }
  /** `q` = query parameters built by dbRowsQuery (page, perPage, q, sort, dir, filter[col]). */
  codeDbRows(t: string, q: Record<string, string>) { return this.get<CodeDbRows>(`/code/db/tables/${encodeURIComponent(t)}/rows`, q); }
  createCodeDbRow(t: string, row: CodeDbRow) { return this.post<CodeDbRow>(`/code/db/tables/${encodeURIComponent(t)}/rows`, row); }
  updateCodeDbRow(t: string, pk: string | number, row: CodeDbRow) { return this.put<CodeDbRow>(`/code/db/tables/${encodeURIComponent(t)}/rows/${encodeURIComponent(String(pk))}`, row); }
  deleteCodeDbRow(t: string, pk: string | number) { return this.del(`/code/db/tables/${encodeURIComponent(t)}/rows/${encodeURIComponent(String(pk))}`); }
  importCodeDb(t: string, file: File, dryRun: boolean) {
    const fd = new FormData();
    fd.append('file', file);
    let p = new HttpParams();
    if (dryRun) p = p.set('dryRun', '1');
    return firstValueFrom(this.http.post<CodeDbImportResult>(`${BASE}/code/db/tables/${encodeURIComponent(t)}/import`, fd, { params: p }));
  }
  exportCodeDb(t: string, format: 'csv' | 'json', q: Record<string, string>) { return this.blob(`/code/db/tables/${encodeURIComponent(t)}/export`, { ...q, format }); }
  downloadCodeDbDump() { return this.blob('/code/db/dump'); }
  codeDbSql(r: CodeDbSqlRequest) { return this.post<CodeDbSqlResult>('/code/db/sql', r); }
  /** History + pending; the exact envelope is normalised by normalizeMigrations (db-utils). */
  codeDbMigrations() { return this.get<unknown>('/code/db/migrations'); }
  rollbackCodeDbMigration(name: string) { return this.post<{ ok: boolean }>(`/code/db/migrations/${encodeURIComponent(name)}/down`, {}); }
  codeDbBackups() { return this.get<CodeDbBackup[]>('/code/db/backups'); }
  downloadCodeDbBackup(id: number) { return this.blob(`/code/db/backups/${id}/download`); }
  deleteCodeDbBackup(id: number) { return this.del(`/code/db/backups/${id}`); }
  // site code ↔ designer (phase 3)
  codeBindings(draft = false) { return this.get<CodeBindings>('/code/bindings', { draft: draft ? 1 : undefined }); }
  codePreview(r: CodePreviewRequest) { return this.post<CodePreviewResult>('/code/preview', r); }
  codeAccess(unit?: string) { return this.get<CodeAccessRow[]>('/code/access', { unit }); }
  /** roles null = remove the admin override (back to the code default). Plugin rows are named `<id>:<name>`. */
  setCodeAccess(kind: CodeAccessKind, name: string, roles: string[] | null, unit?: string) {
    return this.put<unknown>('/code/access', { kind, name, roles, ...(unit ? { unit } : {}) });
  }
  // site code phase 4a: post types, admin pages, settings/secrets, jobs and cron
  /** Post types for the admin menu when /code/bindings is not available to this user (backend may offer either). */
  postTypes() { return this.get<CodePostType[] | { postTypes?: CodePostType[]; items?: CodePostType[] }>('/post-types'); }
  /** Admin pages from site.adminPage the signed-in user may open. */
  codeAdminPages() { return this.get<CodeAdminPage[] | { adminPages?: CodeAdminPage[]; items?: CodeAdminPage[] }>('/code/admin-pages'); }
  /** Ticket URL of an admin page from code (sandboxed iframe host, like service plugin screens). */
  /** site.adminNotice of the running code units (any signed-in user). */
  codeAdminNotices() { return this.get<unknown>('/code/admin-notices'); }
  /** site.dashboardWidget cards with their iframe URLs (ticket included). */
  codeDashboardWidgets() { return this.get<unknown>('/code/dashboard-widgets'); }
  codeAdminTicket(id: string, query: Record<string, string>) {
    return this.post<{ url: string }>(`/code/admin-pages/${encodeURIComponent(id)}/ticket`, { query });
  }
  codeSettings(unit?: string) { return this.get<CodeSettings>('/code/settings', { unit }); }
  saveCodeSettings(body: CodeSettingsUpdate, unit?: string) {
    return unit ? firstValueFrom(this.http.put<CodeSettings>(BASE + '/code/settings', { ...body, unit }, { params: { unit } })) : this.put<CodeSettings>('/code/settings', body);
  }
  codeJobs(q: { status?: string; name?: string; page?: number; perPage?: number; unit?: string }) { return this.get<CodeJobList>('/code/jobs', q); }
  cancelCodeJob(id: number) { return this.post<unknown>(`/code/jobs/${id}/cancel`, {}); }
  retryCodeJob(id: number) { return this.post<unknown>(`/code/jobs/${id}/retry`, {}); }
  codeCron(unit?: string) { return this.get<CodeCron[]>('/code/cron', { unit }); }
  runCodeCron(name: string, unit?: string) {
    return this.post<unknown>(`/code/cron/${encodeURIComponent(name)}/run` + (unit ? '?unit=' + encodeURIComponent(unit) : ''), {});
  }
  // site code phase 4b: hooks, export as plugin
  codeHooks() { return this.get<CodeHook[] | { items?: CodeHook[]; hooks?: CodeHook[] }>('/code/hooks'); }
  codePackageDryRun(body: Partial<CodePackageRequest>) { return this.post<CodePackageDryRun>('/code/package?dryRun=1', body); }
  /** The plugin zip of the live Site Code (Content-Disposition carries the file name). */
  codePackage(body: CodePackageRequest) {
    return firstValueFrom(this.http.post(BASE + '/code/package', body, { responseType: 'blob', observe: 'response' }));
  }
  // site code phase 6: plugin formatters and their unprefixed aliases
  codeFormatters() { return this.get<CodeFormatter[] | { items?: CodeFormatter[]; formatters?: CodeFormatter[] }>('/code/formatters'); }
  /** aliases = {"baht": "thai:baht"} — the whole set (aliases left out are switched off). */
  saveFormatterAliases(aliases: Record<string, string>) { return this.put<unknown>('/code/formatters/aliases', { aliases }); }
  /** anasaki.d.ts (text/plain). */
  codeTypes() { return firstValueFrom(this.http.get(BASE + '/code/types', { responseType: 'text' })); }

  /** GET as a Blob with headers (Content-Disposition carries the file name). */
  private blob(url: string, params?: Record<string, string | undefined>): Promise<HttpResponse<Blob>> {
    let p = new HttpParams();
    for (const [k, v] of Object.entries(params ?? {})) if (v !== undefined && (v !== '' || k === 'path')) p = p.set(k, v);
    return firstValueFrom(this.http.get(BASE + url, { params: p, responseType: 'blob', observe: 'response' }));
  }

  // live preview rendered by the Go renderer (pixel-identical to the public site)
  preview(design: DNode, postId?: number, title?: string) {
    return firstValueFrom(this.http.post(BASE + '/preview', { design, postId, title }, { responseType: 'text' }));
  }
}

/** Site Code paths go into the URL segment by segment (`lib/a b.ts` → `lib/a%20b.ts`). */
export function codePath(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/');
}

/** Extracts a readable message from an HttpErrorResponse. */
export function errorMessage(e: unknown): string {
  const err = e as { error?: { error?: string } | string; message?: string };
  if (err?.error && typeof err.error === 'object' && err.error.error) return err.error.error;
  if (typeof err?.error === 'string') {
    try { return JSON.parse(err.error).error ?? err.error; } catch { return err.error; }
  }
  return err?.message ?? 'Error';
}

/** Like errorMessage, for requests made with responseType 'blob' (the JSON error arrives as a Blob). */
export async function blobErrorMessage(e: unknown): Promise<string> {
  const err = e as { error?: unknown };
  if (typeof Blob !== 'undefined' && err?.error instanceof Blob) {
    try {
      const text = await err.error.text();
      try { return JSON.parse(text).error ?? text; } catch { return text || errorMessage(e); }
    } catch { /* fall through */ }
  }
  return errorMessage(e);
}

/** Built-in icon and shape sets of the designer. */
/** A path the admin path / URL bases may not use; plugin = the declaring plugin ('' or absent = the core). */
export interface ReservedPath { path: string; plugin?: string }

export interface DesignAssets {
  icons: Record<string, { body: string; fill?: boolean }>;
  iconNames: string[];
  shapes: Record<string, { viewBox: string; body: string; stretch?: boolean; kind: 'divider' | 'blob' }>;
  /** Mask shapes (style.mask) and their names. */
  masks?: Record<string, { viewBox: string; body: string }>;
  maskNames?: string[];
}

/** One place a component is used (GET /components/{id}/usage). */
export interface ComponentUse { kind: 'post' | 'component'; id: number; title: string; type?: string }
