import { ChangeDetectionStrategy, Component, ElementRef, Injector, OnDestroy, OnInit, afterNextRender, computed, inject, signal, viewChild } from '@angular/core';
import type * as Monaco from 'monaco-editor';
import { Api, blobErrorMessage, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { CodeVersion, CodeVersionDetail } from '../core/code-models';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { PostTypes } from '../core/post-types';
import { saveBlob } from '../shared/download';
import { CodeTabs } from './code-tabs';
import { filenameFromDisposition, formatBytes, languageFor, versionRows } from './code-utils';
import { MonacoApi, MonacoLoader } from './monaco-loader';
import './messages-code';

/** /code/versions — published snapshots: view files (read-only), download, restore. */
@Component({
  selector: 'rea-code-versions',
  imports: [TPipe, CodeTabs],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <header class="page-head">
      <span class="page-icon">🕘</span>
      <div class="page-head-text"><h1>{{ 'code.versions.title' | t }}</h1><p>{{ 'code.versions.desc' | t }}</p></div>
    </header>
    <rea-code-tabs />
    @if (error()) { <p class="alert" id="code-versions-error">{{ error() }}</p> }

    <section class="card table-card">
      <table class="table code-versions-table" id="code-versions">
        <thead><tr>
          <th>{{ 'code.versions.version' | t }}</th><th>{{ 'code.versions.message' | t }}</th><th>{{ 'code.versions.by' | t }}</th>
          <th>{{ 'code.versions.when' | t }}</th><th class="num">{{ 'code.versions.files' | t }}</th><th class="th-actions"></th>
        </tr></thead>
        <tbody>
          @for (v of rows(); track v.id) {
            <tr class="code-version-row" [class.selected]="detail()?.id === v.id" [attr.data-id]="v.id">
              <td class="mono"><b>{{ v.label }}</b> @if (v.live) { <span class="badge badge-live code-version-live">● {{ 'code.versions.live' | t }}</span> }</td>
              <td class="code-version-message">{{ v.message }}</td>
              <td>{{ v.by }}</td>
              <td class="muted">{{ v.when }}</td>
              <td class="num">{{ v.files }}</td>
              <td class="row-actions">
                <button class="btn btn-sm code-version-view" (click)="view(v.id)">👁 {{ 'common.view' | t }}</button>
                <button class="btn btn-sm code-version-download" (click)="download(v.id)">⬇ {{ 'code.versions.download' | t }}</button>
                @if (canPublish() && !v.live) {
                  <button class="btn btn-sm code-version-restore" (click)="askRestore(v.id)">↺ {{ 'code.versions.restore' | t }}</button>
                }
              </td>
            </tr>
          } @empty {
            <tr><td colspan="6" class="empty-state" id="code-versions-empty">{{ loading() ? ('common.loading' | t) : ('code.versions.empty' | t) }}</td></tr>
          }
        </tbody>
      </table>
    </section>

    @if (detail(); as d) {
      <section class="card code-version-detail" id="code-version-detail">
        <div class="card-head">
          <h3>{{ 'code.versions.filesOf' | t: { n: d.id } }}</h3>
          <button class="icon-btn" (click)="close()" [title]="'common.close' | t">✕</button>
        </div>
        <div class="code-version-view-grid">
          <nav class="code-version-files">
            @for (f of d.files; track f.path) {
              <button class="code-version-file mono" [class.active]="f.path === file()" [attr.data-path]="f.path" (click)="openFile(f.path)">
                {{ f.path }} <span class="muted small">{{ size(f.size) }}</span>
              </button>
            } @empty { <p class="muted small">{{ 'code.noFiles' | t }}</p> }
          </nav>
          <div class="code-editor-host code-version-editor" id="code-version-editor" #host></div>
        </div>
      </section>
    }

    @if (restoring(); as r) {
      <div class="modal-backdrop" (click)="restoring.set(null)">
        <div class="modal code-modal" id="code-restore-dialog" role="dialog" (click)="$event.stopPropagation()">
          <div class="modal-head"><h3>↺ {{ 'code.restore.title' | t: { n: r.id } }}</h3><button class="icon-btn" (click)="restoring.set(null)">✕</button></div>
          <p>{{ 'code.restore.text' | t: { n: r.id } }}</p>
          <label class="check"><input type="checkbox" id="code-restore-also-draft" [checked]="r.alsoDraft" (change)="setAlsoDraft($event)" /> {{ 'code.restore.alsoDraft' | t }}</label>
          <p class="hint code-restore-db-warn" id="code-restore-db-warn">🗄️ {{ 'code.restore.dbWarn' | t }}</p>
          @if (r.alsoDraft) { <p class="hint code-restore-warn">⚠ {{ 'code.restore.alsoDraftHint' | t }}</p> }
          <div class="form-foot">
            <button class="btn" (click)="restoring.set(null)">{{ 'common.cancel' | t }}</button>
            <button class="btn btn-primary" id="code-restore-confirm" [disabled]="r.busy" (click)="restore()">{{ 'code.versions.restore' | t }}</button>
          </div>
        </div>
      </div>
    }
  `,
})
export class CodeVersions implements OnInit, OnDestroy {
  private api = inject(Api);
  private postTypes = inject(PostTypes);
  private auth = inject(Auth);
  private i18n = inject(I18n);
  private loader = inject(MonacoLoader);
  private injector = inject(Injector);
  private host = viewChild<ElementRef<HTMLDivElement>>('host');

  readonly canPublish = computed(() => this.auth.can('publish_scripts'));
  readonly loading = signal(true);
  readonly error = signal('');
  readonly versions = signal<CodeVersion[]>([]);
  readonly rows = computed(() => {
    this.i18n.locale();
    return versionRows(this.versions(), iso => this.i18n.date(iso, true), this.i18n.t('code.versions.untitled'));
  });
  readonly detail = signal<CodeVersionDetail | null>(null);
  readonly file = signal<string | null>(null);
  readonly restoring = signal<{ id: number; alsoDraft: boolean; busy: boolean } | null>(null);

  private m: MonacoApi | null = null;
  private editor: Monaco.editor.IStandaloneCodeEditor | null = null;
  private model: Monaco.editor.ITextModel | null = null;

  ngOnInit() { void this.load(); }

  ngOnDestroy() { this.disposeEditor(); }

  size(n: number) { return formatBytes(n); }

  async load() {
    try { this.versions.set(await this.api.codeVersions()); } catch (e) { this.error.set(errorMessage(e)); } finally { this.loading.set(false); }
  }

  async view(id: number) {
    try {
      const [d, m] = await Promise.all([this.api.codeVersion(id), this.loader.load()]);
      this.m = m;
      this.disposeEditor();
      this.detail.set(d);
      this.file.set(null);
      const first = d.files.find(f => f.path === 'main.ts') ?? d.files[0];
      // the editor host of the detail panel exists after the next render
      afterNextRender(() => { if (first) void this.openFile(first.path); }, { injector: this.injector });
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  close() { this.disposeEditor(); this.detail.set(null); this.file.set(null); }

  private disposeEditor() {
    this.editor?.dispose();
    this.model?.dispose();
    this.editor = null;
    this.model = null;
  }

  async openFile(path: string) {
    const d = this.detail();
    const host = this.host()?.nativeElement;
    if (!d || !this.m || !host) return;
    try {
      const f = await this.api.codeVersionFile(d.id, path);
      const m = this.m;
      this.model?.dispose();
      // not file:/// — snapshot files must not mix with the draft's models
      this.model = m.editor.createModel(f.content, languageFor(path), m.Uri.parse(`inmemory://version-${d.id}/${path}`));
      if (!this.editor) {
        this.editor = m.editor.create(host, { model: this.model, readOnly: true, automaticLayout: true, minimap: { enabled: false }, fontSize: 13, scrollBeyondLastLine: false, theme: 'vs' });
      } else this.editor.setModel(this.model);
      this.file.set(path);
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  async download(id: number) {
    try {
      const res = await this.api.downloadCodeVersion(id);
      saveBlob(res.body ?? new Blob(), filenameFromDisposition(res.headers.get('Content-Disposition'), `site-code-v${id}.zip`));
    } catch (e) { Dialog.error(await blobErrorMessage(e)); }
  }

  askRestore(id: number) { this.restoring.set({ id, alsoDraft: false, busy: false }); }

  setAlsoDraft(e: Event) {
    const r = this.restoring();
    if (r) this.restoring.set({ ...r, alsoDraft: (e.target as HTMLInputElement).checked });
  }

  async restore() {
    const r = this.restoring();
    if (!r) return;
    this.restoring.set({ ...r, busy: true });
    try {
      const res = await this.api.restoreCodeVersion(r.id, r.alsoDraft);
      this.restoring.set(null);
      Dialog.toast(this.i18n.t('code.restore.done', { n: res.version?.id ?? '' }));
      void this.postTypes.load();
      await this.load();
    } catch (e) {
      this.restoring.set({ ...r, busy: false });
      Dialog.error(errorMessage(e));
    }
  }
}
