import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { Api, blobErrorMessage, errorMessage } from '../core/api';
import { CodeDbStatus, CodeDbTable } from '../core/code-models';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { saveBlob } from '../shared/download';
import { filenameFromDisposition, formatBytes } from './code-utils';
import { DbNav } from './db-nav';
import './messages-db';

/** /code/db — mode, size, setup SQL when the separate database is not configured, tables list, dump download. */
@Component({
  selector: 'rea-db-overview',
  imports: [TPipe, RouterLink, DbNav],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <rea-db-nav />
    @if (error()) { <p class="alert" id="db-error">{{ error() }}</p> }

    @if (status(); as s) {
      <section class="card db-summary" id="db-summary">
        <div class="db-summary-item">
          <span class="muted small">{{ 'db.mode' | t }}</span>
          <span class="badge db-mode" id="db-mode" [attr.data-mode]="s.mode" [class.badge-live]="s.mode === 'separate'" [class.badge-warn]="s.mode !== 'separate'">
            {{ s.mode === 'separate' ? ('db.mode.separate' | t) : ('db.mode.fallback' | t) }}</span>
        </div>
        <div class="db-summary-item"><span class="muted small">{{ 'db.database' | t }}</span><b class="mono" id="db-name">{{ s.database || '—' }}</b></div>
        <div class="db-summary-item"><span class="muted small">{{ 'db.size' | t }}</span><b id="db-size">{{ size(s.sizeBytes) }}</b></div>
        <div class="db-summary-item"><span class="muted small">{{ 'db.tableCount' | t }}</span><b id="db-table-count">{{ s.tables }}</b></div>
        <span class="grow"></span>
        <button class="btn btn-sm" id="db-dump" (click)="dump()" [title]="'db.dumpHint' | t">⬇ {{ 'db.dump' | t }}</button>
      </section>

      @if (s.mode === 'fallback') {
        <div class="info-box warn db-fallback" id="db-fallback-note">
          <b>{{ 'db.fallback.title' | t }}</b>
          <ul>
            <li>{{ 'db.fallback.prefix' | t }}</li>
            <li>{{ 'db.fallback.noRaw' | t }}</li>
            <li>{{ 'db.fallback.noCore' | t }}</li>
          </ul>
        </div>
      }
      @if (!s.configured) {
        <section class="card db-setup" id="db-setup">
          <h3>{{ 'db.setup.title' | t }}</h3>
          <p class="hint">{{ 'db.setup.hint' | t }}</p>
          <pre class="db-sql-block mono" id="db-setup-sql">{{ setupText() }}</pre>
          <div class="form-foot">
            <button class="btn btn-sm" id="db-setup-copy" (click)="copySetup()">📋 {{ 'db.setup.copy' | t }}</button>
          </div>
          <p class="hint">{{ 'db.setup.after' | t }}</p>
        </section>
      }
    }

    <section class="card table-card">
      <div class="table-toolbar">
        <b>{{ 'db.tables' | t }}</b>
        <span class="grow"></span>
        <a class="btn btn-sm" id="db-open-er" routerLink="/code/db/er">🔗 {{ 'db.nav.er' | t }}</a>
        <button class="btn btn-sm" id="db-tables-refresh" (click)="load()">↻ {{ 'code.logs.refresh' | t }}</button>
      </div>
      <table class="table db-tables" id="db-tables">
        <thead><tr><th>{{ 'db.table' | t }}</th><th class="num">{{ 'db.rows' | t }}</th><th class="num">{{ 'db.size' | t }}</th><th>{{ 'db.comment' | t }}</th></tr></thead>
        <tbody>
          @for (t of tables(); track t.name) {
            <tr class="db-table-row" [attr.data-table]="t.name" [class.view]="t.view" [class.system]="t.system">
              <td>
                <a class="db-table-link mono" [routerLink]="['/code/db/tables', t.name]">{{ t.name }}</a>
                @if (t.view) { <span class="badge db-badge-view" [title]="'db.viewHint' | t">{{ 'db.view' | t }}</span> }
                @if (t.system) { <span class="badge db-badge-system" [title]="'db.systemHint' | t">{{ 'db.system' | t }}</span> }
              </td>
              <td class="num mono">{{ t.view ? '—' : t.rows }}</td>
              <td class="num mono small">{{ t.view ? '—' : size(t.sizeBytes) }}</td>
              <td class="muted small">{{ t.comment }}</td>
            </tr>
          } @empty {
            <tr><td colspan="4" class="empty-state" id="db-tables-empty">{{ loading() ? ('common.loading' | t) : ('db.noTables' | t) }}</td></tr>
          }
        </tbody>
      </table>
    </section>
  `,
})
export class DbOverview implements OnInit {
  private api = inject(Api);
  private i18n = inject(I18n);

  readonly loading = signal(true);
  readonly error = signal('');
  readonly status = signal<CodeDbStatus | null>(null);
  readonly tables = signal<CodeDbTable[]>([]);
  readonly setupText = computed(() => (this.status()?.setupSql ?? []).map(s => s.trim().replace(/;?$/, ';')).join('\n'));

  ngOnInit() { void this.load(); }

  size(n: number) { return formatBytes(n || 0); }

  async load() {
    this.loading.set(true);
    try {
      this.status.set(await this.api.codeDbStatus());
      this.error.set('');
      // developer tables first, then views and system tables (each by name)
      const list = await this.api.codeDbTables();
      const rank = (t: CodeDbTable) => (t.view ? 2 : t.system ? 1 : 0);
      this.tables.set([...list].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name)));
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.loading.set(false);
    }
  }

  async copySetup() {
    try {
      await navigator.clipboard.writeText(this.setupText());
      Dialog.toast(this.i18n.t('db.setup.copied'));
    } catch { Dialog.error(this.i18n.t('db.setup.copyFailed')); }
  }

  async dump() {
    try {
      const res = await this.api.downloadCodeDbDump();
      saveBlob(res.body ?? new Blob(), filenameFromDisposition(res.headers.get('Content-Disposition'), `${this.status()?.database || 'site-code'}.sql`));
    } catch (e) { Dialog.error(await blobErrorMessage(e)); }
  }
}
