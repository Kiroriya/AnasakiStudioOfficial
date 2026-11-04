import { ChangeDetectionStrategy, Component, ElementRef, OnDestroy, OnInit, computed, inject, signal, viewChild } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import type * as Monaco from 'monaco-editor';
import { Api, blobErrorMessage, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import {
  CodeConflict, CodeDestructive, CodeFile, CodeFileInfo, CodePendingMigration, CodePublishRefusal, CodeRoute, CodeRunResult, CodeStatus, CodeUploadMode,
} from '../core/code-models';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { PostTypes } from '../core/post-types';
import { mapLimit } from '../shared/concurrency';
import { saveBlob } from '../shared/download';
import { CodeTabs } from './code-tabs';
import {
  CODE_EXTENSIONS, PUBLIC_EXTENSIONS, CODE_LIMITS, Problem, RUN_METHODS, UploadSummary, ancestors, baseName, buildTree, entryPoint, filenameFromDisposition,
  flattenTree, formatBytes, isZipName, languageFor, movedPath, parentDir, parseHeaderLines, prettyBody, problemsByFile, splitRunPath,
  summarizeUpload, toProblems, validatePath,
} from './code-utils';
import { formatDestructive, migrationFileName, migrationTemplate, pendingDestructive } from './db-utils';
import { MonacoApi, MonacoLoader } from './monaco-loader';
import { ExportPlugin } from './export-plugin';
import './messages-code4b';
import './messages-code';

interface Buffer {
  model: Monaco.editor.ITextModel;
  /** Model version at the last load/save: differs → unsaved changes. */
  savedVersion: number;
  /** updatedAt of the stored copy this buffer is based on ('' = not stored yet). */
  updatedAt: string;
  sub: Monaco.IDisposable;
}

interface UploadState {
  dir: string;
  file: File | null;
  mode: CodeUploadMode;
  merge: UploadSummary | null;
  replace: UploadSummary | null;
  busy: boolean;
  error: string;
}

interface PublishState {
  message: string;
  busy: boolean;
  error: string;
  errors: Problem[];
  /** Dry run of the draft (POST /build) in progress. */
  checking: boolean;
  /** Pending migrations with their SQL (null = unknown: no dry run possible). */
  migrations: CodePendingMigration[] | null;
  /** Destructive changes the publish would make — need the checkbox (confirmDestructive). */
  destructive: CodeDestructive[];
  confirmDestructive: boolean;
}

/** Op codes of destructive migration changes that have a label (others are shown as sent). */
const DESTRUCTIVE_OPS = ['drop_table', 'drop_column', 'change_column', 'rename_table', 'rename_column'];
interface TreeMenu { path: string; kind: 'file' | 'folder'; x: number; y: number }

const MARKER_OWNER = 'anasaki-build';

/** /code — Site Code editor: file tree, Monaco, build problems, upload/download, publish, test route, safe mode. */
@Component({
  selector: 'rea-code-editor',
  imports: [TPipe, CodeTabs, ExportPlugin],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    class: 'code-page',
    '(document:keydown)': 'keydown($event)',
    '(document:click)': 'menu.set(null)',
    '(window:beforeunload)': 'beforeUnload($event)',
  },
  template: `
    <header class="page-head code-head">
      <span class="page-icon">⌨️</span>
      <div class="page-head-text"><h1>{{ 'code.title' | t }}</h1><p>{{ 'code.desc' | t }}</p></div>
      @if (status(); as s) {
        @if (canPublish()) {
          <label class="code-switch" id="code-safe-mode" [class.on]="s.safeMode" [title]="'code.safe.hint' | t">
            <input type="checkbox" id="code-safe-mode-input" [checked]="s.safeMode" (change)="toggleSafeMode($event)" />
            <span class="code-switch-track"><span class="code-switch-dot"></span></span>
            <span>{{ 'code.safe.label' | t }}</span>
          </label>
        } @else if (s.safeMode) {
          <span class="badge badge-warn" id="code-safe-mode-badge">🛟 {{ 'code.safe.on' | t }}</span>
        }
      }
    </header>
    <rea-code-tabs />

    @if (error()) { <p class="alert" id="code-error">{{ error() }}</p> }
    @if (status(); as s) {
      @if (s.safeMode) { <div class="alert code-banner" id="code-safe-banner">🛟 {{ 'code.safe.banner' | t }}</div> }
      @if (s.envDisabled) { <div class="alert code-banner" id="code-env-banner">⛔ {{ 'code.envDisabled' | t }}</div> }
      @if (s.lastError) { <div class="alert code-banner" id="code-last-error">⚠ {{ 'code.lastError' | t }}: <span class="mono">{{ s.lastError }}</span></div> }
    }

    <div class="code-toolbar" [class.hidden]="!!error() && loading()">
      @if (canEdit()) {
        <button class="btn btn-sm" id="code-new-file" (click)="newFile(currentDir())">＋ {{ 'code.newFile' | t }}</button>
        <button class="btn btn-sm" id="code-new-folder" (click)="newFolder(currentDir())">📁 {{ 'code.newFolder' | t }}</button>
        <button class="btn btn-sm" id="code-upload" (click)="openUpload('')">⬆ {{ 'code.upload' | t }}</button>
      }
      <button class="btn btn-sm" id="code-download-all" (click)="download('', 'folder')">⬇ {{ 'code.downloadAll' | t }}</button>
      <button class="btn btn-sm btn-ghost" id="code-download-types" (click)="downloadTypes()" [title]="'code.typesHint' | t">⬇ anasaki.d.ts</button>
      <span class="grow"></span>
      @if (canEdit()) {
        <button class="btn btn-sm" id="code-build" [disabled]="build() === 'running'" (click)="runBuild()">✓ {{ 'code.check' | t }}</button>
        <button class="btn btn-sm" id="code-test-toggle" [class.on]="runOpen()" (click)="runOpen.set(!runOpen())">▶ {{ 'code.run.title' | t }}</button>
        <button class="btn btn-sm btn-primary" id="code-save" [disabled]="!current() || saving() || !isDirty(current())" (click)="save()" [title]="'code.saveHint' | t">💾 {{ 'common.save' | t }}</button>
      }
      @if (canPublish()) {
        <button class="btn btn-sm btn-primary" id="code-publish" (click)="openPublish()">🚀 {{ 'code.publish' | t }}</button>
        <button class="btn btn-sm" id="code-export-plugin" (click)="exportOpen.set(true)" [title]="'cx.intro' | t">📦 {{ 'cx.open' | t }}</button>
      }
    </div>

    <div class="code-layout" [class.with-run]="runOpen() && canEdit()">
      <!-- file tree -->
      <aside class="code-tree" id="code-tree" (contextmenu)="openMenu($event, '', 'folder')">
        <div class="code-tree-head">
          <span>{{ 'code.files' | t }}</span>
          <span class="muted small" id="code-file-count">{{ files().length }}/{{ limits.files }} · {{ size(totalSize()) }}</span>
        </div>
        <div class="code-tree-body" role="tree">
          @for (r of rows(); track r.node.path) {
            <div class="code-tree-item" role="treeitem" [class.folder]="r.node.kind === 'folder'" [class.file]="r.node.kind === 'file'"
              [class.active]="r.node.path === current()" [class.dirty]="isDirty(r.node.path)" [attr.data-path]="r.node.path"
              [attr.aria-expanded]="r.node.kind === 'folder' ? expanded().has(r.node.path) : null"
              [style.padding-left.px]="8 + r.depth * 14" (click)="clickNode(r.node.path, r.node.kind)" (contextmenu)="openMenu($event, r.node.path, r.node.kind)">
              <span class="code-tree-icon">{{ r.node.kind === 'folder' ? (expanded().has(r.node.path) ? '📂' : '📁') : fileIcon(r.node.path) }}</span>
              <span class="code-tree-name">{{ r.node.name }}</span>
              @if (isDirty(r.node.path)) { <span class="code-dirty-dot" [title]="'code.unsaved' | t">●</span> }
              @if (problemCount(r.node.path); as n) { <span class="code-tree-err">{{ n }}</span> }
              <button class="code-tree-more" [title]="'common.actions' | t" (click)="openMenu($event, r.node.path, r.node.kind)">⋯</button>
            </div>
          } @empty {
            @if (!loading()) { <p class="muted small code-tree-empty">{{ 'code.noFiles' | t }}</p> }
          }
        </div>
      </aside>

      <!-- editor + problems -->
      <section class="code-main">
        <div class="code-filebar">
          @if (current(); as c) {
            <span class="mono" id="code-current-path">{{ c }}</span>
            @if (isDirty(c)) { <span class="code-dirty-dot" id="code-dirty">● {{ 'code.unsaved' | t }}</span> }
          } @else { <span class="muted">{{ loading() ? ('common.loading' | t) : ('code.pickFile' | t) }}</span> }
          @if (!canEdit()) { <span class="badge" id="code-readonly">🔒 {{ 'code.readOnly' | t }}</span> }
        </div>
        <div class="code-editor-host" id="code-editor" #host></div>
        <div class="code-problems" id="code-problems" [class.collapsed]="!problemsOpen()">
          <button class="code-problems-head" (click)="problemsOpen.set(!problemsOpen())">
            <span>{{ problemsOpen() ? '▾' : '▸' }} {{ 'code.problems' | t }}</span>
            <span class="badge badge-danger" id="code-problem-errors">{{ errorCount() }}</span>
            <span class="badge badge-warn" id="code-problem-warnings">{{ warningCount() }}</span>
            <span class="muted small code-build-state" id="code-build-state" [attr.data-state]="build()">{{ buildLabel() }}</span>
          </button>
          @if (problemsOpen()) {
            <div class="code-problems-list">
              @for (p of problems(); track $index) {
                <button class="code-problem" [class.error]="p.severity === 'error'" [class.warning]="p.severity === 'warning'" [attr.data-file]="p.path ?? p.file" [attr.data-line]="p.line" (click)="goToProblem(p)">
                  <span class="code-problem-icon">{{ p.severity === 'error' ? '⛔' : '⚠' }}</span>
                  <span class="code-problem-msg">{{ p.message }}</span>
                  <span class="mono muted code-problem-loc">{{ p.file || '—' }}:{{ p.line }}:{{ p.column }}</span>
                </button>
              } @empty {
                <p class="muted small code-problems-empty">{{ build() === 'idle' ? ('code.problems.notRun' | t) : ('code.problems.none' | t) }}</p>
              }
            </div>
          }
        </div>
      </section>

      <!-- test route -->
      @if (runOpen() && canEdit()) {
        <aside class="code-run" id="code-run-panel">
          <div class="code-run-head"><b>▶ {{ 'code.run.title' | t }}</b><button class="icon-btn" (click)="runOpen.set(false)" [title]="'common.close' | t">✕</button></div>
          <p class="hint">{{ 'code.run.hint' | t }}</p>
          <p class="code-run-db-notice small" id="code-run-db-notice">🗄️ {{ 'code.run.realDb' | t }}</p>
          @if (dirty().size) { <p class="code-run-warn small">⚠ {{ 'code.run.unsaved' | t }}</p> }
          <div class="code-run-req">
            <select class="code-run-method" id="code-run-method" [value]="runMethod()" (change)="runMethod.set(val($event))">
              @for (m of methods; track m) { <option [value]="m">{{ m }}</option> }
            </select>
            <input class="code-run-path mono" id="code-run-path" [value]="runPath()" (input)="runPath.set(val($event))" (keydown.enter)="run()" placeholder="/hello" />
          </div>
          @if (draftRoutes().length) {
            <div class="code-run-routes">
              @for (r of draftRoutes(); track r.method + r.path) {
                <button class="chip code-run-route mono" (click)="pickRoute(r)">{{ r.method }} {{ r.path }}</button>
              }
            </div>
          }
          <label>{{ 'code.run.query' | t }}<input class="mono" id="code-run-query" [value]="runQuery()" (input)="runQuery.set(val($event))" placeholder="a=1&b=2" /></label>
          <label>{{ 'code.run.headers' | t }}<textarea class="mono" id="code-run-headers" rows="2" [value]="runHeaders()" (input)="runHeaders.set(val($event))" placeholder="Accept: application/json"></textarea></label>
          <label>{{ 'code.run.body' | t }}<textarea class="mono" id="code-run-body" rows="3" [value]="runBody()" (input)="runBody.set(val($event))" placeholder='{"name":"Ann"}'></textarea></label>
          <button class="btn btn-primary btn-sm" id="code-run-send" [disabled]="runBusy()" (click)="run()">{{ runBusy() ? ('code.run.running' | t) : ('code.run.send' | t) }}</button>

          @if (runResult(); as r) {
            <div class="code-run-result" id="code-run-result">
              @if (r.error) { <p class="alert code-run-error" id="code-run-error">{{ r.error }}</p> }
              @if (r.status !== undefined) {
                <div class="code-run-meta">
                  <span class="badge code-run-status" id="code-run-status" [class.badge-live]="r.status < 400" [class.badge-danger]="r.status >= 400">{{ r.status }}</span>
                  @if (r.durationMs !== undefined) { <span class="muted small code-run-duration" id="code-run-duration">{{ r.durationMs }} ms</span> }
                </div>
              }
              @if (runHeaderList().length) {
                <details class="code-run-section" id="code-run-headers-out">
                  <summary>{{ 'code.run.respHeaders' | t }} ({{ runHeaderList().length }})</summary>
                  <table class="code-run-headers-table">
                    @for (h of runHeaderList(); track h[0]) { <tr><td class="mono">{{ h[0] }}</td><td class="mono">{{ h[1] }}</td></tr> }
                  </table>
                </details>
              }
              @if (r.body !== undefined) {
                <div class="code-run-section">
                  <div class="small muted">{{ 'code.run.respBody' | t }} @if (runPretty().json) { <span class="chip">JSON</span> }</div>
                  <pre class="code-run-body-out mono" id="code-run-body-out">{{ runPretty().text }}</pre>
                </div>
              }
              <div class="code-run-section">
                <div class="small muted">{{ 'code.run.logs' | t }} ({{ r.logs?.length ?? 0 }})</div>
                <div class="code-run-logs" id="code-run-logs">
                  @for (l of r.logs ?? []; track $index) {
                    <div class="code-run-log mono" [attr.data-level]="l.level"><span class="code-level" [attr.data-level]="l.level">{{ l.level }}</span> {{ l.message }}</div>
                  } @empty { <span class="muted small">{{ 'code.run.noLogs' | t }}</span> }
                </div>
              </div>
            </div>
          }
        </aside>
      }
    </div>

    <!-- status bar -->
    <footer class="code-status" id="code-status">
      @if (status(); as s) {
        <span id="code-status-live" [attr.data-version]="s.liveVersion ?? ''">🌐 {{ s.liveVersion ? ('code.status.live' | t: { n: s.liveVersion }) : ('code.status.noLive' | t) }}</span>
        @if (s.draftChanged) { <span class="badge badge-warn" id="code-status-draft">{{ 'code.status.draftChanged' | t }}</span> }
        @else { <span class="badge" id="code-status-draft-same">{{ 'code.status.draftSame' | t }}</span> }
        @if (s.db?.pendingMigrations) { <span class="badge badge-warn" id="code-status-migrations">🗄️ {{ 'code.status.migrations' | t: { n: s.db!.pendingMigrations } }}</span> }
        @if (s.safeMode) { <span class="badge badge-warn" id="code-status-safe">🛟 {{ 'code.safe.on' | t }}</span> }
        @if (s.envDisabled) { <span class="badge badge-danger">{{ 'code.status.envOff' | t }}</span> }
        <span class="muted">{{ 'code.status.routes' | t: { n: s.registered.routes.length || 0 } }} · {{ 'code.status.events' | t: { n: s.registered.events.length || 0 } }}</span>
      }
      <span class="grow"></span>
      @if (current(); as c) {
        <span class="muted" id="code-status-cursor">{{ 'code.status.cursor' | t: { line: cursor().line, col: cursor().column } }}</span>
        <span class="muted">{{ langLabel(c) }}</span>
      }
      @if (dirty().size) { <span class="code-dirty-dot" id="code-status-unsaved">● {{ 'code.status.unsaved' | t: { n: dirty().size } }}</span> }
    </footer>

    <!-- tree context menu -->
    @if (menu(); as mn) {
      <div class="code-menu" id="code-tree-menu" [style.left.px]="mn.x" [style.top.px]="mn.y" (click)="$event.stopPropagation()">
        @if (mn.kind === 'folder') {
          @if (canEdit()) {
            <button data-action="new-file" (click)="menu.set(null); newFile(mn.path)">＋ {{ 'code.newFile' | t }}</button>
            <button data-action="new-folder" (click)="menu.set(null); newFolder(mn.path)">📁 {{ 'code.newFolder' | t }}</button>
            <button data-action="upload" (click)="menu.set(null); openUpload(mn.path)">⬆ {{ 'code.uploadHere' | t }}</button>
            @if (mn.path === 'migrations' || mn.path === '') {
              <button data-action="new-migration" id="code-new-migration" (click)="menu.set(null); newMigration()">🗄️ {{ 'code.newMigration' | t }}</button>
            }
          }
          <button data-action="download" (click)="menu.set(null); download(mn.path, 'folder')">⬇ {{ mn.path ? ('code.downloadFolder' | t) : ('code.downloadAll' | t) }}</button>
        } @else {
          <button data-action="open" (click)="menu.set(null); openFile(mn.path)">📄 {{ 'code.open' | t }}</button>
          <button data-action="download" (click)="menu.set(null); download(mn.path, 'file')">⬇ {{ 'code.downloadFile' | t }}</button>
        }
        @if (canEdit() && mn.path) {
          <button data-action="rename" (click)="menu.set(null); rename(mn.path, mn.kind)">✎ {{ 'code.rename' | t }}</button>
          <button data-action="delete" class="danger" (click)="menu.set(null); remove(mn.path, mn.kind)">🗑 {{ 'common.delete' | t }}</button>
        }
      </div>
    }

    <!-- upload dialog -->
    @if (upload(); as u) {
      <div class="modal-backdrop" (click)="closeUpload()">
        <div class="modal code-modal" id="code-upload-dialog" role="dialog" (click)="$event.stopPropagation()">
          <div class="modal-head"><h3>⬆ {{ 'code.upload.title' | t }}</h3><button class="icon-btn" (click)="closeUpload()">✕</button></div>
          <p class="hint">{{ 'code.upload.hint' | t: { exts: exts } }}</p>
          <label>{{ 'code.upload.file' | t }}
            <input type="file" id="code-upload-file" [accept]="accept" (change)="pickUpload($event)" />
          </label>
          @if (!u.file || !isZip(u.file.name)) {
            <label>{{ 'code.upload.dir' | t }}
              <input class="mono" id="code-upload-dir" [value]="u.dir" placeholder="lib" (change)="setUploadDir(val($event))" />
              <small class="hint">{{ 'code.upload.dirHint' | t }}</small>
            </label>
          }
          @if (u.error) { <p class="alert" id="code-upload-error">{{ u.error }}</p> }
          @if (u.busy) { <p class="muted">{{ 'code.upload.checking' | t }}</p> }
          @if (u.file && u.merge) {
            @if (isZip(u.file.name)) {
              <div class="radio-list code-upload-modes">
                <label class="check"><input type="radio" name="code-upload-mode" id="code-upload-merge" value="merge" [checked]="u.mode === 'merge'" (change)="setUploadMode('merge')" />
                  <span><b>{{ 'code.upload.merge' | t }}</b> <span class="muted small">{{ 'code.upload.mergeHint' | t }}</span></span></label>
                <label class="check"><input type="radio" name="code-upload-mode" id="code-upload-replace" value="replace" [checked]="u.mode === 'replace'" (change)="setUploadMode('replace')" />
                  <span><b>{{ 'code.upload.replace' | t }}</b> <span class="muted small">{{ 'code.upload.replaceHint' | t }}</span></span></label>
              </div>
            }
            @if (uploadSummary(); as sm) {
              <div class="code-diff" id="code-upload-diff">
                <p class="small"><b>{{ 'code.upload.summary' | t: { a: sm.added.length, c: sm.changed.length, r: sm.removed.length, u: sm.unchanged } }}</b></p>
                @if (sm.empty) { <p class="muted" id="code-upload-nochange">{{ 'code.upload.nothing' | t }}</p> }
                @if (sm.added.length) {
                  <div class="code-diff-group added"><div class="small">{{ 'code.upload.added' | t }}</div>
                    @for (p of sm.added; track p) { <div class="code-diff-item mono added">+ {{ p }}</div> }</div>
                }
                @if (sm.changed.length) {
                  <div class="code-diff-group changed"><div class="small">{{ 'code.upload.changed' | t }}</div>
                    @for (p of sm.changed; track p) { <div class="code-diff-item mono changed">~ {{ p }}</div> }</div>
                }
                @if (sm.removed.length) {
                  <div class="code-diff-group removed"><div class="small">{{ 'code.upload.removed' | t }}</div>
                    @for (p of sm.removed; track p) { <div class="code-diff-item mono removed">− {{ p }}</div> }</div>
                }
              </div>
            }
          }
          <div class="form-foot">
            <button class="btn" (click)="closeUpload()">{{ 'common.cancel' | t }}</button>
            <button class="btn btn-primary" id="code-upload-confirm" [disabled]="u.busy || !u.file || !uploadSummary() || uploadSummary()!.empty" (click)="confirmUpload()">{{ 'code.upload.confirm' | t }}</button>
          </div>
        </div>
      </div>
    }

    <!-- publish dialog -->
    @if (publish(); as p) {
      <div class="modal-backdrop" (click)="publish.set(null)">
        <div class="modal code-modal" id="code-publish-dialog" role="dialog" (click)="$event.stopPropagation()">
          <div class="modal-head"><h3>🚀 {{ 'code.publish.title' | t }}</h3><button class="icon-btn" (click)="publish.set(null)">✕</button></div>
          <p class="hint">{{ 'code.publish.hint' | t }}</p>
          @if (dirty().size) {
            <div class="alert code-publish-unsaved" id="code-publish-unsaved">
              {{ 'code.publish.unsaved' | t: { n: dirty().size } }}
              <button class="btn btn-sm" id="code-save-all" [disabled]="saving()" (click)="saveAll()">💾 {{ 'code.saveAll' | t }}</button>
            </div>
          }
          <label>{{ 'code.publish.message' | t }}
            <textarea id="code-publish-message" rows="3" [value]="p.message" (input)="setPublishMessage(val($event))" [placeholder]="'code.publish.messagePh' | t"></textarea>
          </label>
          @if (p.checking) { <p class="muted small" id="code-publish-checking">{{ 'code.publish.checking' | t }}</p> }
          @if (p.migrations?.length) {
            <div class="code-publish-migrations" id="code-publish-migrations">
              <div class="small"><b>🗄️ {{ 'code.publish.migrations' | t: { n: p.migrations!.length } }}</b> <span class="muted">{{ 'code.publish.migrationsHint' | t }}</span></div>
              @for (m of p.migrations; track m.name) {
                <details class="code-publish-migration" [attr.data-name]="m.name">
                  <summary><span class="mono">{{ m.name }}</span>
                    @if (m.destructive?.length) { <span class="badge badge-danger">{{ 'code.publish.destructiveBadge' | t }}</span> }</summary>
                  <pre class="code-publish-sql mono">{{ sqlText(m.sql) }}</pre>
                </details>
              }
            </div>
          }
          @if (destructiveGroups().length) {
            <div class="code-publish-destructive" id="code-publish-destructive">
              <b>⚠ {{ 'code.publish.destructiveTitle' | t }}</b>
              @for (g of destructiveGroups(); track g.migration) {
                @if (g.migration) { <div class="mono small code-publish-destructive-mig">{{ g.migration }}</div> }
                <ul>
                  @for (i of g.items; track i.op + i.target) { <li class="code-publish-destructive-item mono" [attr.data-op]="i.op" [attr.data-target]="i.target">{{ i.text }}</li> }
                </ul>
              }
              <label class="check"><input type="checkbox" id="code-publish-destructive-confirm" [checked]="p.confirmDestructive" (change)="setConfirmDestructive($event)" />
                <span>{{ 'code.publish.destructiveConfirm' | t }}</span></label>
            </div>
          }
          @if (p.error) { <p class="alert" id="code-publish-error">{{ p.error }}</p> }
          @if (p.errors.length) {
            <div class="code-publish-errors" id="code-publish-errors">
              @for (e of p.errors; track $index) {
                <button class="code-problem error" (click)="publish.set(null); goToProblem(e)">
                  <span class="code-problem-icon">⛔</span><span class="code-problem-msg">{{ e.message }}</span>
                  <span class="mono muted code-problem-loc">{{ e.file || '—' }}:{{ e.line }}:{{ e.column }}</span>
                </button>
              }
            </div>
          }
          <div class="form-foot">
            <button class="btn" (click)="publish.set(null)">{{ 'common.cancel' | t }}</button>
            <button class="btn btn-primary" id="code-publish-confirm" [disabled]="p.busy || (p.destructive.length > 0 && !p.confirmDestructive)" (click)="confirmPublish()">{{ p.busy ? ('code.publish.busy' | t) : ('code.publish' | t) }}</button>
          </div>
        </div>
      </div>
    }

    @if (exportOpen()) {
      @defer { <rea-export-plugin (closed)="exportOpen.set(false)" /> } @loading { <div class="modal-backdrop"></div> }
    }
  `,
})
export class CodeEditor implements OnInit, OnDestroy {
  private api = inject(Api);
  private postTypes = inject(PostTypes);
  private auth = inject(Auth);
  private i18n = inject(I18n);
  private loader = inject(MonacoLoader);
  private host = viewChild.required<ElementRef<HTMLDivElement>>('host');

  readonly limits = CODE_LIMITS;
  readonly methods = RUN_METHODS;
  readonly exts = CODE_EXTENSIONS.join(' ');
  readonly accept = ['.zip', ...new Set([...CODE_EXTENSIONS, ...PUBLIC_EXTENSIONS])].join(',');

  readonly canEdit = computed(() => this.auth.can('edit_scripts'));
  readonly canPublish = computed(() => this.auth.can('publish_scripts'));
  /** "Export as plugin" dialog (lazy chunk). */
  readonly exportOpen = signal(false);

  readonly loading = signal(true);
  readonly error = signal('');
  readonly status = signal<CodeStatus | null>(null);
  readonly files = signal<CodeFileInfo[]>([]);
  /** Empty folders made in the UI (a folder only exists on the server through its files). */
  readonly extraFolders = signal<string[]>([]);
  readonly expanded = signal(new Set<string>());
  readonly current = signal<string | null>(null);
  readonly dirty = signal(new Set<string>());
  readonly problems = signal<Problem[]>([]);
  readonly build = signal<'idle' | 'running' | 'ok' | 'failed'>('idle');
  readonly draftRoutes = signal<CodeRoute[]>([]);
  readonly saving = signal(false);
  readonly menu = signal<TreeMenu | null>(null);
  readonly cursor = signal({ line: 1, column: 1 });
  readonly problemsOpen = signal(true);
  readonly upload = signal<UploadState | null>(null);
  readonly publish = signal<PublishState | null>(null);

  readonly runOpen = signal(false);
  readonly runMethod = signal('GET');
  readonly runPath = signal('/');
  readonly runQuery = signal('');
  readonly runHeaders = signal('');
  readonly runBody = signal('');
  readonly runBusy = signal(false);
  readonly runResult = signal<CodeRunResult | null>(null);

  readonly tree = computed(() => buildTree(this.files(), this.extraFolders()));
  readonly rows = computed(() => flattenTree(this.tree(), this.expanded()));
  readonly paths = computed(() => this.files().map(f => f.path));
  readonly totalSize = computed(() => this.files().reduce((s, f) => s + (f.size || 0), 0));
  readonly errorCount = computed(() => this.problems().filter(p => p.severity === 'error').length);
  readonly warningCount = computed(() => this.problems().filter(p => p.severity === 'warning').length);
  private readonly problemMap = computed(() => problemsByFile(this.problems().filter(p => p.severity === 'error')));
  readonly uploadSummary = computed(() => {
    const u = this.upload();
    return u ? (u.mode === 'replace' ? u.replace : u.merge) : null;
  });
  readonly destructiveGroups = computed(() =>
    formatDestructive(this.publish()?.destructive, op => (DESTRUCTIVE_OPS.includes(op) ? this.i18n.t('code.op.' + op) : null)));
  readonly runPretty = computed(() => prettyBody(this.runResult()?.body, this.runResult()?.headers));
  readonly runHeaderList = computed(() =>
    Object.entries(this.runResult()?.headers ?? {}).map(([k, v]) => [k, Array.isArray(v) ? v.join(', ') : String(v)] as const));

  private m: MonacoApi | null = null;
  private editor: Monaco.editor.IStandaloneCodeEditor | null = null;
  private buffers = new Map<string, Buffer>();
  private viewStates = new Map<string, Monaco.editor.ICodeEditorViewState | null>();
  private destroyed = false;

  // ---------------------------------------------------------------- lifecycle

  async ngOnInit() {
    try {
      this.status.set(await this.api.codeStatus());
      const [m, list, types] = await Promise.all([
        this.loader.load(),
        this.api.codeFiles(),
        this.api.codeTypes().catch(() => ''),
      ]);
      if (this.destroyed) return;
      this.m = m;
      this.loader.configure(m);
      const contents = await this.fetchContents(list);
      if (this.destroyed) return;
      this.files.set(list);
      for (const f of list) this.addBuffer(f.path, contents.get(f.path)?.content ?? '', contents.get(f.path)?.updatedAt ?? f.updatedAt);
      this.types = types;
      this.applyTypes();
      this.createEditor(m);
      const first = entryPoint(this.paths()) ?? list[0]?.path;
      if (first) this.openFile(first);
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.loading.set(false);
    }
  }

  ngOnDestroy() {
    this.destroyed = true;
    this.editor?.dispose();
    for (const b of this.buffers.values()) { b.sub.dispose(); b.model.dispose(); }
    this.buffers.clear();
  }

  private types = '';

  /** anasaki.d.ts as an extra lib — unless the project carries its own copy (then that file is the source). */
  private applyTypes() {
    if (!this.m || !this.types) return;
    if (this.paths().some(p => p.toLowerCase() === 'anasaki.d.ts')) this.loader.setTypes(this.m, '');
    else this.loader.setTypes(this.m, this.types);
  }

  private createEditor(m: MonacoApi) {
    this.editor = m.editor.create(this.host().nativeElement, {
      model: null,
      automaticLayout: true,
      readOnly: !this.canEdit(),
      minimap: { enabled: false },
      fontSize: 13,
      tabSize: 2,
      scrollBeyondLastLine: false,
      fixedOverflowWidgets: true,
      theme: 'vs',
    });
    this.editor.addCommand(m.KeyMod.CtrlCmd | m.KeyCode.KeyS, () => void this.save());
    this.editor.onDidChangeCursorPosition(e => this.cursor.set({ line: e.position.lineNumber, column: e.position.column }));
  }

  private async fetchContents(list: CodeFileInfo[]): Promise<Map<string, CodeFile>> {
    const res = await mapLimit(list, 6, f => this.api.codeFile(f.path));
    const out = new Map<string, CodeFile>();
    res.forEach((r, i) => {
      if (r.status === 'fulfilled') out.set(list[i].path, r.value);
      else throw r.reason;
    });
    return out;
  }

  private addBuffer(path: string, content: string, updatedAt: string): Buffer {
    const m = this.m!;
    const uri = m.Uri.parse('file:///' + path);
    m.editor.getModel(uri)?.dispose();
    const model = m.editor.createModel(content, languageFor(path), uri);
    const b: Buffer = { model, savedVersion: model.getAlternativeVersionId(), updatedAt, sub: null! };
    b.sub = model.onDidChangeContent(() => this.markDirty(path, model.getAlternativeVersionId() !== b.savedVersion));
    this.buffers.set(path, b);
    return b;
  }

  private dropBuffer(path: string) {
    const b = this.buffers.get(path);
    if (!b) return;
    if (this.editor?.getModel() === b.model) this.editor.setModel(null);
    b.sub.dispose();
    b.model.dispose();
    this.buffers.delete(path);
    this.viewStates.delete(path);
    this.markDirty(path, false);
  }

  private markClean(b: Buffer, path: string) {
    b.savedVersion = b.model.getAlternativeVersionId();
    this.markDirty(path, false);
  }

  private markDirty(path: string, d: boolean) {
    const s = this.dirty();
    if (s.has(path) === d) return;
    const n = new Set(s);
    if (d) n.add(path); else n.delete(path);
    this.dirty.set(n);
  }

  isDirty(path: string | null) { return !!path && this.dirty().has(path); }

  // ---------------------------------------------------------------- tree

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  size(n: number) { return formatBytes(n); }
  isZip(name: string) { return isZipName(name); }

  fileIcon(path: string) {
    const lang = languageFor(path);
    return lang === 'typescript' ? '🟦' : lang === 'javascript' ? '🟨' : lang === 'json' ? '🧾' : lang === 'markdown' ? '📝' : '📄';
  }

  langLabel(path: string) {
    return { typescript: 'TypeScript', javascript: 'JavaScript', json: 'JSON', markdown: 'Markdown' }[languageFor(path)] ?? '';
  }

  problemCount(path: string) { return this.problemMap().get(path)?.length ?? 0; }

  /** Folder new files go into: the folder of the open file. */
  currentDir() { const c = this.current(); return c ? parentDir(c) : ''; }

  clickNode(path: string, kind: 'file' | 'folder') {
    if (kind === 'folder') {
      const s = new Set(this.expanded());
      if (s.has(path)) s.delete(path); else s.add(path);
      this.expanded.set(s);
    } else this.openFile(path);
  }

  openMenu(e: MouseEvent, path: string, kind: 'file' | 'folder') {
    e.preventDefault();
    e.stopPropagation();
    this.menu.set({ path, kind, x: Math.min(e.clientX, window.innerWidth - 230), y: Math.min(e.clientY, window.innerHeight - 260) });
  }

  private expandTo(path: string) {
    const s = new Set(this.expanded());
    ancestors(path).forEach(a => s.add(a));
    this.expanded.set(s);
  }

  openFile(path: string, line?: number, column = 1) {
    const b = this.buffers.get(path);
    if (!b || !this.editor) return;
    const prev = this.current();
    if (prev && prev !== path && this.buffers.get(prev)?.model === this.editor.getModel()) this.viewStates.set(prev, this.editor.saveViewState());
    if (this.editor.getModel() !== b.model) {
      this.editor.setModel(b.model);
      const vs = this.viewStates.get(path);
      if (vs) this.editor.restoreViewState(vs);
    }
    this.current.set(path);
    this.expandTo(path);
    const pos = this.editor.getPosition();
    this.cursor.set({ line: pos?.lineNumber ?? 1, column: pos?.column ?? 1 });
    if (line) {
      this.editor.revealLineInCenter(line);
      this.editor.setPosition({ lineNumber: line, column });
    }
    this.editor.focus();
  }

  goToProblem(p: Problem) {
    if (p.path && this.buffers.has(p.path)) this.openFile(p.path, p.line, p.column);
  }

  private pathError(err: string) { return this.i18n.t('code.path.' + err); }

  async newFile(dir: string) {
    if (this.files().length >= CODE_LIMITS.files) return void Dialog.error(this.i18n.t('code.limit.files', { n: CODE_LIMITS.files }));
    const path = await Dialog.prompt({
      title: this.i18n.t('code.newFile'), label: this.i18n.t('code.newFileLabel', { exts: this.exts }),
      value: dir ? dir + '/' : '', placeholder: 'lib/helpers.ts',
    });
    if (!path) return;
    const err = validatePath(path, this.paths());
    if (err) return void Dialog.error(this.pathError(err));
    try {
      const f = await this.api.saveCodeFile(path, '', '');
      this.files.set([...this.files(), { path: f.path, size: 0, updatedAt: f.updatedAt, updatedBy: this.auth.user()?.name ?? '' }]
        .sort((a, b) => a.path.localeCompare(b.path)));
      this.extraFolders.set(this.extraFolders().filter(x => !path.startsWith(x + '/')));
      this.addBuffer(f.path, f.content ?? '', f.updatedAt);
      this.applyTypes();
      this.openFile(f.path);
      void this.refreshStatus();
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  async newFolder(dir: string) {
    const path = await Dialog.prompt({ title: this.i18n.t('code.newFolder'), label: this.i18n.t('code.newFolderLabel'), value: dir ? dir + '/' : '', placeholder: 'lib' });
    if (!path) return;
    const clean = path.replace(/\/+$/, '');
    const err = validatePath(clean, [...this.paths(), ...this.extraFolders()], 'folder');
    if (err) return void Dialog.error(this.pathError(err));
    this.extraFolders.set([...this.extraFolders(), clean]);
    const s = new Set(this.expanded());
    [...ancestors(clean + '/x'), clean].forEach(a => s.add(a));
    this.expanded.set(s);
    Dialog.toast(this.i18n.t('code.folderHint'), 'info');
  }

  /** Creates migrations/NNN_<name>.ts with a starter template (numbered after the existing ones). */
  async newMigration() {
    if (this.files().length >= CODE_LIMITS.files) return void Dialog.error(this.i18n.t('code.limit.files', { n: CODE_LIMITS.files }));
    const name = await Dialog.prompt({ title: this.i18n.t('code.newMigration'), label: this.i18n.t('code.newMigrationLabel'), placeholder: 'create_orders' });
    if (!name) return;
    const path = migrationFileName(this.paths(), name);
    const err = validatePath(path, this.paths());
    if (err) return void Dialog.error(this.pathError(err));
    const slug = path.slice('migrations/'.length).replace(/^\d+_/, '').replace(/\.ts$/, '');
    const content = migrationTemplate(slug);
    try {
      const f = await this.api.saveCodeFile(path, content, '');
      this.files.set([...this.files(), { path: f.path, size: new Blob([content]).size, updatedAt: f.updatedAt, updatedBy: this.auth.user()?.name ?? '' }]
        .sort((a, b) => a.path.localeCompare(b.path)));
      this.extraFolders.set(this.extraFolders().filter(x => x !== 'migrations'));
      this.addBuffer(f.path, f.content ?? content, f.updatedAt);
      this.openFile(f.path);
      Dialog.toast(this.i18n.t('code.migrationCreated', { path: f.path }));
      void this.refreshStatus();
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  async rename(path: string, kind: 'file' | 'folder') {
    const to = await Dialog.prompt({ title: this.i18n.t('code.renameTitle', { name: baseName(path) }), label: this.i18n.t('code.renameLabel'), value: path });
    if (!to || to === path) return;
    const target = to.replace(/\/+$/, '');
    const others = this.paths().filter(p => (kind === 'file' ? p !== path : movedPath(p, path, path) === null));
    const err = validatePath(target, kind === 'folder' ? [...others, ...this.extraFolders().filter(x => movedPath(x, path, path) === null)] : others, kind);
    if (err) return void Dialog.error(this.pathError(err));
    const onServer = kind === 'file' || this.paths().some(p => p.startsWith(path + '/'));
    try {
      if (onServer) await this.api.moveCode(path, target);
      this.extraFolders.set(this.extraFolders().map(x => movedPath(x, path, target) ?? x));
      if (onServer) await this.reload({ from: path, to: target });
      Dialog.toast(this.i18n.t('code.moved'));
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  async remove(path: string, kind: 'file' | 'folder') {
    const inside = kind === 'folder' ? this.paths().filter(p => p.startsWith(path + '/')).length : 1;
    const text = kind === 'folder' ? this.i18n.t('code.deleteFolderText', { n: inside }) : undefined;
    if (!(await Dialog.confirmDelete(baseName(path), text))) return;
    try {
      if (kind === 'file') await this.api.deleteCodeFile(path);
      else if (inside) await this.api.deleteCodeFolder(path);
      this.extraFolders.set(this.extraFolders().filter(x => x !== path && !x.startsWith(path + '/')));
      if (kind === 'file' || inside) await this.reload();
      Dialog.toast(this.i18n.t('code.deleted'));
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  /**
   * Re-reads the draft after a move/delete/upload/restore. Clean files that did not change keep
   * their model; unsaved edits are carried over (to the new path after a move) and keep the
   * version they were based on, so saving them over a newer copy still asks first (409).
   */
  async reload(move?: { from: string; to: string }) {
    const list = await this.api.codeFiles();
    const carry = new Map<string, { text: string; updatedAt: string; moved: boolean }>();
    for (const p of this.dirty()) {
      const b = this.buffers.get(p);
      if (!b) continue;
      const np = move ? movedPath(p, move.from, move.to) : null;
      carry.set(np ?? p, { text: b.model.getValue(), updatedAt: b.updatedAt, moved: np !== null });
    }
    const keep = new Set(list.map(f => f.path));
    const need = list.filter(f => {
      const b = this.buffers.get(f.path);
      return !b || (b.updatedAt !== f.updatedAt && !carry.has(f.path));
    });
    const contents = await this.fetchContents(need);
    const cur = this.current();
    for (const p of [...this.buffers.keys()]) if (!keep.has(p)) this.dropBuffer(p);
    for (const f of list) {
      const c = contents.get(f.path);
      const existing = this.buffers.get(f.path);
      const kept = carry.get(f.path);
      if (c) {
        const b = existing ?? this.addBuffer(f.path, c.content, c.updatedAt);
        if (existing) { b.model.setValue(c.content); b.updatedAt = c.updatedAt; this.markClean(b, f.path); }
        if (kept?.moved) { b.model.setValue(kept.text); } // moved file: same base, edits re-applied (dirty again)
      }
    }
    this.files.set(list);
    this.applyTypes();
    const next = (cur && move ? movedPath(cur, move.from, move.to) : null) ?? cur;
    if (next && this.buffers.has(next)) this.openFile(next);
    else {
      this.current.set(null);
      const first = entryPoint(this.paths()) ?? list[0]?.path;
      if (first) this.openFile(first);
    }
    void this.refreshStatus();
  }

  // ---------------------------------------------------------------- save / build

  keydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's' && !e.defaultPrevented) {
      e.preventDefault();
      void this.save();
    }
    if (e.key === 'Escape') this.menu.set(null);
  }

  beforeUnload(e: BeforeUnloadEvent) {
    if (this.dirty().size) e.preventDefault();
  }

  /** Route guard: confirm before leaving with unsaved files. */
  async canLeave(): Promise<boolean> {
    if (!this.dirty().size) return true;
    return Dialog.confirm({
      title: this.i18n.t('code.leaveTitle'), text: this.i18n.t('code.leaveText', { n: this.dirty().size }),
      confirm: this.i18n.t('code.leaveConfirm'), danger: true,
    });
  }

  /** Saves one file (the open one by default), then builds the draft. */
  async save(path = this.current(), build = true): Promise<boolean> {
    if (!path || !this.canEdit() || this.saving()) return false;
    const b = this.buffers.get(path);
    if (!b) return false;
    const content = b.model.getValue();
    if (new Blob([content]).size > CODE_LIMITS.fileBytes) { Dialog.error(this.i18n.t('code.limit.fileSize', { size: formatBytes(CODE_LIMITS.fileBytes) })); return false; }
    this.saving.set(true);
    const version = b.model.getAlternativeVersionId();
    let ok = false;
    try {
      const f = await this.api.saveCodeFile(path, content, b.updatedAt);
      this.afterSave(path, b, f, version);
      ok = true;
    } catch (e) {
      const conflict = (e as HttpErrorResponse)?.status === 409 ? ((e as HttpErrorResponse).error as CodeConflict) : null;
      if (conflict?.current) ok = await this.resolveConflict(path, b, conflict);
      else Dialog.error(errorMessage(e));
    } finally {
      this.saving.set(false);
    }
    if (ok && build) await this.runBuild();
    return ok;
  }

  private afterSave(path: string, b: Buffer, f: CodeFile, version: number) {
    b.updatedAt = f.updatedAt;
    b.savedVersion = version; // edits typed while the request was in flight stay unsaved
    this.markDirty(path, b.model.getAlternativeVersionId() !== version);
    this.files.set(this.files().map(x => (x.path === path ? { ...x, size: new Blob([b.model.getValue()]).size, updatedAt: f.updatedAt, updatedBy: this.auth.user()?.name ?? x.updatedBy } : x)));
  }

  /** 409: someone saved this file after we loaded it — take theirs, or overwrite with ours. */
  private async resolveConflict(path: string, b: Buffer, c: CodeConflict): Promise<boolean> {
    const choice = await Dialog.choose({
      title: this.i18n.t('code.conflict.title', { name: path }),
      text: c.error || this.i18n.t('code.conflict.text'),
      confirm: this.i18n.t('code.conflict.reload'),
      deny: this.i18n.t('code.conflict.overwrite'),
    });
    if (choice === 'confirm') {
      b.model.setValue(c.current.content);
      b.updatedAt = c.current.updatedAt;
      this.markClean(b, path);
      return false;
    }
    if (choice === 'deny') {
      try {
        const version = b.model.getAlternativeVersionId();
        const f = await this.api.saveCodeFile(path, b.model.getValue(), c.current.updatedAt);
        this.afterSave(path, b, f, version);
        return true;
      } catch (e) { Dialog.error(errorMessage(e)); }
    }
    return false;
  }

  async saveAll() {
    for (const p of [...this.dirty()]) if (!(await this.save(p, false))) return;
    await this.runBuild();
  }

  buildLabel() {
    switch (this.build()) {
      case 'running': return this.i18n.t('code.build.running');
      case 'ok': return this.i18n.t('code.build.ok');
      case 'failed': return this.i18n.t('code.build.failed');
      default: return '';
    }
  }

  async runBuild() {
    if (!this.canEdit()) return;
    this.build.set('running');
    try {
      const r = await this.api.buildCode();
      this.problems.set(toProblems(r, this.paths()));
      this.build.set(r.ok ? 'ok' : 'failed');
      if (r.registered) this.draftRoutes.set(r.registered.routes ?? []);
      this.applyMarkers();
      if (!r.ok) this.problemsOpen.set(true);
    } catch (e) {
      this.build.set('failed');
      Dialog.error(errorMessage(e));
    }
    void this.refreshStatus();
  }

  private applyMarkers() {
    const m = this.m;
    if (!m) return;
    const by = problemsByFile(this.problems());
    for (const [path, b] of this.buffers) {
      const list = by.get(path) ?? [];
      const lines = b.model.getLineCount();
      m.editor.setModelMarkers(b.model, MARKER_OWNER, list.map(p => {
        const line = Math.min(p.line, lines);
        return {
          severity: p.severity === 'error' ? m.MarkerSeverity.Error : m.MarkerSeverity.Warning,
          message: p.message,
          startLineNumber: line, startColumn: p.column,
          endLineNumber: line, endColumn: Math.max(p.column + 1, b.model.getLineMaxColumn(line)),
          source: 'build',
        };
      }));
    }
  }

  private async refreshStatus() {
    try { this.status.set(await this.api.codeStatus()); } catch { /* keep */ }
  }

  // ---------------------------------------------------------------- safe mode

  async toggleSafeMode(e: Event) {
    const input = e.target as HTMLInputElement;
    const enabled = input.checked;
    input.checked = !enabled; // the switch follows the server, not the click
    const ok = await Dialog.confirm({
      title: this.i18n.t(enabled ? 'code.safe.onTitle' : 'code.safe.offTitle'),
      text: this.i18n.t(enabled ? 'code.safe.onText' : 'code.safe.offText'),
      confirm: this.i18n.t(enabled ? 'code.safe.onConfirm' : 'code.safe.offConfirm'),
      danger: !enabled,
    });
    if (!ok) return;
    try {
      this.status.set(await this.api.setCodeSafeMode(enabled));
      input.checked = enabled;
      Dialog.toast(this.i18n.t(enabled ? 'code.safe.turnedOn' : 'code.safe.turnedOff'));
    } catch (err) { Dialog.error(errorMessage(err)); }
  }

  // ---------------------------------------------------------------- download

  async download(path: string, kind: 'file' | 'folder') {
    try {
      const res = await this.api.downloadCode(path);
      const fallback = kind === 'file' ? baseName(path) : (path ? baseName(path) : 'site-code') + '.zip';
      saveBlob(res.body ?? new Blob(), filenameFromDisposition(res.headers.get('Content-Disposition'), fallback));
    } catch (e) { Dialog.error(await blobErrorMessage(e)); }
  }

  async downloadTypes() {
    try {
      const text = this.types || (this.types = await this.api.codeTypes());
      saveBlob(new Blob([text], { type: 'text/plain' }), 'anasaki.d.ts');
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  // ---------------------------------------------------------------- upload

  openUpload(dir: string) {
    this.upload.set({ dir, file: null, mode: 'merge', merge: null, replace: null, busy: false, error: '' });
  }

  closeUpload() { if (!this.upload()?.busy) this.upload.set(null); }

  private patchUpload(p: Partial<UploadState>) {
    const u = this.upload();
    if (u) this.upload.set({ ...u, ...p });
  }

  pickUpload(e: Event) {
    const file = (e.target as HTMLInputElement).files?.[0] ?? null;
    this.patchUpload({ file, mode: 'merge', merge: null, replace: null, error: '' });
    if (file) void this.dryRun();
  }

  setUploadDir(dir: string) {
    this.patchUpload({ dir: dir.trim().replace(/^\/+|\/+$/g, '') });
    if (this.upload()?.file) void this.dryRun();
  }

  setUploadMode(mode: CodeUploadMode) { this.patchUpload({ mode }); }

  /** Asks the server what the upload would do — for both modes when it is a project zip. */
  private async dryRun() {
    const u = this.upload();
    if (!u?.file) return;
    const file = u.file;
    if (!isZipName(file.name)) {
      const err = validatePath(u.dir ? `${u.dir}/${file.name}` : file.name);
      if (err && err !== 'duplicate') return this.patchUpload({ error: this.pathError(err), merge: null, replace: null });
    }
    this.patchUpload({ busy: true, error: '' });
    try {
      const zip = isZipName(file.name);
      const [merge, replace] = await Promise.all([
        this.api.uploadCode(file, { dryRun: true, mode: 'merge', dir: u.dir }),
        zip ? this.api.uploadCode(file, { dryRun: true, mode: 'replace', dir: u.dir }) : Promise.resolve(null),
      ]);
      if (this.upload()?.file !== file) return;
      this.patchUpload({ busy: false, merge: summarizeUpload(merge, 'merge'), replace: replace ? summarizeUpload(replace, 'replace') : null });
    } catch (e) {
      this.patchUpload({ busy: false, error: errorMessage(e) });
    }
  }

  async confirmUpload() {
    const u = this.upload();
    const sm = this.uploadSummary();
    if (!u?.file || !sm || sm.empty) return;
    if (sm.destructive && !(await Dialog.confirm({
      title: this.i18n.t('code.upload.replaceTitle'), text: this.i18n.t('code.upload.replaceText', { n: sm.removed.length }),
      confirm: this.i18n.t('code.upload.confirm'), danger: true,
    }))) return;
    this.patchUpload({ busy: true, error: '' });
    try {
      await this.api.uploadCode(u.file, { dryRun: false, mode: u.mode, dir: u.dir });
      this.upload.set(null);
      await this.reload();
      Dialog.toast(this.i18n.t('code.upload.done', { n: sm.writes + sm.removed.length }));
      await this.runBuild();
    } catch (e) {
      this.patchUpload({ busy: false, error: errorMessage(e) });
    }
  }

  // ---------------------------------------------------------------- publish

  openPublish() {
    this.publish.set({ message: '', busy: false, error: '', errors: [], checking: this.canEdit(), migrations: null, destructive: [], confirmDestructive: false });
    if (this.canEdit()) void this.publishDryRun();
  }

  /** Builds the draft to list pending migrations (SQL) and destructive changes before publishing. */
  private async publishDryRun() {
    try {
      const r = await this.api.buildCode();
      const p = this.publish();
      if (!p) return;
      const pending = r.migrations?.pending ?? [];
      const errors = r.ok ? [] : toProblems({ errors: r.errors ?? [], warnings: [] }, this.paths());
      this.publish.set({ ...p, checking: false, migrations: pending, destructive: pendingDestructive(pending), errors });
    } catch {
      const p = this.publish();
      if (p) this.publish.set({ ...p, checking: false });
    }
  }

  setConfirmDestructive(e: Event) {
    const p = this.publish();
    if (p) this.publish.set({ ...p, confirmDestructive: (e.target as HTMLInputElement).checked });
  }

  sqlText(sql: string[] | undefined) { return (sql ?? []).map(x => x.trim().replace(/;?$/, ';')).join('\n\n'); }

  setPublishMessage(message: string) {
    const p = this.publish();
    if (p) this.publish.set({ ...p, message });
  }

  async confirmPublish() {
    const p = this.publish();
    if (!p) return;
    this.publish.set({ ...p, busy: true, error: '', errors: [] });
    try {
      const res = await this.api.publishCode(p.message.trim(), this.status()?.liveVersion ?? null, p.destructive.length > 0 && p.confirmDestructive);
      this.publish.set(null);
      Dialog.toast(this.i18n.t('code.publish.done', { n: res.version?.id ?? '' }));
      void this.postTypes.load(); // the sidebar follows new post types / admin pages
      await this.refreshStatus();
    } catch (e) {
      const err = e as HttpErrorResponse;
      const body = err?.error as CodePublishRefusal | null;
      let errors: Problem[] = [];
      const destructive = err?.status === 400 && body?.destructive?.length ? body.destructive : null;
      if (err?.status === 400 && body?.errors?.length) {
        errors = toProblems({ errors: body.errors, warnings: [] }, this.paths());
        this.problems.set(errors);
        this.build.set('failed');
        this.applyMarkers();
      }
      let msg = errorMessage(e);
      if (err?.status === 409) { msg = `${msg}\n${this.i18n.t('code.publish.conflict')}`; await this.refreshStatus(); }
      const cur = this.publish();
      if (cur) this.publish.set({ ...cur, busy: false, error: msg, errors, ...(destructive ? { destructive, confirmDestructive: false } : {}) });
    }
  }

  // ---------------------------------------------------------------- test route

  pickRoute(r: CodeRoute) {
    this.runMethod.set((RUN_METHODS as readonly string[]).includes(r.method) ? r.method : 'GET');
    // fill path params with a placeholder value so the request can run as-is
    this.runPath.set(r.path.replace(/\{[^}]+\}/g, '1').replace(/\/\*$/, '/'));
  }

  async run() {
    if (this.runBusy()) return;
    const { path, query } = splitRunPath(this.runPath(), this.runQuery());
    this.runBusy.set(true);
    try {
      this.runResult.set(await this.api.runCode({
        method: this.runMethod(), path, query, headers: parseHeaderLines(this.runHeaders()), body: this.runBody(),
      }));
    } catch (e) {
      const body = (e as HttpErrorResponse)?.error as CodeRunResult | null;
      this.runResult.set({ error: body?.error ?? errorMessage(e), logs: body?.logs ?? [] });
    } finally {
      this.runBusy.set(false);
    }
  }
}

