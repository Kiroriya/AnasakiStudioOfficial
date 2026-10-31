import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { Api, blobErrorMessage, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { CodeDbBackup, CodeDbMigration } from '../core/code-models';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { saveBlob } from '../shared/download';
import { filenameFromDisposition } from './code-utils';
import { DbNav } from './db-nav';
import { MigrationsView, canRollBack, migrationStatusClass, normalizeBackups, normalizeMigrations } from './db-utils';
import './messages-db';

/** /code/db/migrations — history (status, errors), pending, rollback of the latest batch, table backups. */
@Component({
  selector: 'rea-db-migrations',
  imports: [TPipe, DbNav],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <rea-db-nav />
    @if (error()) { <p class="alert" id="db-mig-error">{{ error() }}</p> }
    <div class="info-box db-mig-note">ℹ️ {{ 'db.mig.note' | t }}</div>

    <section class="card table-card">
      <div class="table-toolbar"><b>{{ 'db.mig.pending' | t }}</b><span class="badge" [class.badge-warn]="mig().pending.length" id="db-mig-pending-count">{{ mig().pending.length }}</span>
        <span class="grow"></span><button class="btn btn-sm" id="db-mig-refresh" (click)="load()">↻ {{ 'code.logs.refresh' | t }}</button></div>
      <ul class="db-mig-pending" id="db-mig-pending">
        @for (p of mig().pending; track p.name + (p.source ?? '')) {
          <li class="db-mig-pending-item" [attr.data-name]="p.name" [attr.data-source]="p.source ?? ''">
            <span class="mono">{{ p.name }}</span>
            @if (p.source) { <span class="badge" [class.badge-info]="p.source === 'live'">{{ p.source === 'live' ? ('db.mig.fromLive' | t) : p.source === 'draft' ? ('db.mig.fromDraft' | t) : p.source }}</span> }
          </li>
        } @empty { <li class="muted small" id="db-mig-pending-empty">{{ 'db.mig.noPending' | t }}</li> }
      </ul>
    </section>

    <section class="card table-card">
      <div class="table-toolbar"><b>{{ 'db.mig.history' | t }}</b></div>
      <table class="table db-migrations" id="db-migrations">
        <thead><tr><th>{{ 'db.mig.name' | t }}</th><th class="num">{{ 'db.mig.batch' | t }}</th><th>{{ 'db.mig.status' | t }}</th><th>{{ 'db.mig.ranAt' | t }}</th><th>{{ 'db.mig.ranBy' | t }}</th><th class="num">ms</th><th class="th-actions"></th></tr></thead>
        <tbody>
          @for (m of mig().history; track m.name + m.batch) {
            <tr class="db-mig-row" [attr.data-name]="m.name" [attr.data-status]="m.status" [attr.data-batch]="m.batch">
              <td class="mono">{{ m.name }}
                @if (m.error) { <div class="db-mig-error mono">{{ m.error }}</div> }
              </td>
              <td class="num mono">{{ m.batch }}</td>
              <td><span [class]="'badge db-mig-status ' + badge(m.status)">{{ statusLabel(m.status) }}</span></td>
              <td class="small nowrap">{{ m.ranAt ? date(m.ranAt) : '—' }}</td>
              <td class="small">{{ m.ranBy || '—' }}</td>
              <td class="num mono small">{{ m.durationMs }}</td>
              <td class="row-actions">
                @if (canRollback() && rollbackable(m)) {
                  <button class="btn btn-sm btn-danger db-mig-rollback" [disabled]="busy()" (click)="rollback(m)">↶ {{ 'db.mig.rollback' | t }}</button>
                }
              </td>
            </tr>
          } @empty {
            <tr><td colspan="7" class="empty-state" id="db-mig-empty">{{ loading() ? ('common.loading' | t) : ('db.mig.empty' | t) }}</td></tr>
          }
        </tbody>
      </table>
    </section>

    <section class="card table-card">
      <div class="table-toolbar"><b>{{ 'db.backups' | t }}</b><span class="hint">{{ 'db.backups.hint' | t }}</span></div>
      <table class="table db-backups" id="db-backups">
        <thead><tr><th>{{ 'db.table' | t }}</th><th>{{ 'db.backups.copy' | t }}</th><th>{{ 'db.mig.name' | t }}</th><th>{{ 'db.backups.created' | t }}</th><th>{{ 'db.backups.expires' | t }}</th><th class="th-actions"></th></tr></thead>
        <tbody>
          @for (b of backups(); track b.id) {
            <tr class="db-backup-row" [attr.data-id]="b.id" [attr.data-table]="b.tableName">
              <td class="mono">{{ b.tableName }}</td>
              <td class="mono small">{{ b.backupTable }}</td>
              <td class="mono small">{{ b.migration }}</td>
              <td class="small nowrap">{{ b.createdAt ? date(b.createdAt) : '' }}</td>
              <td class="small nowrap">{{ b.expiresAt ? date(b.expiresAt) : '' }}</td>
              <td class="row-actions">
                <button class="btn btn-sm db-backup-download" (click)="download(b)">⬇ .sql</button>
                @if (canEdit()) { <button class="icon-btn danger db-backup-delete" [title]="'common.delete' | t" (click)="removeBackup(b)">🗑</button> }
              </td>
            </tr>
          } @empty {
            <tr><td colspan="6" class="muted small" id="db-backups-empty">{{ 'db.backups.empty' | t }}</td></tr>
          }
        </tbody>
      </table>
    </section>
  `,
})
export class DbMigrations implements OnInit {
  private api = inject(Api);
  private auth = inject(Auth);
  private i18n = inject(I18n);

  readonly canEdit = computed(() => this.auth.can('edit_code_data'));
  /** Rollback runs code against the data: needs both edit_code_data and publish_scripts. */
  readonly canRollback = computed(() => this.auth.can('edit_code_data') && this.auth.can('publish_scripts'));
  readonly loading = signal(true);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly mig = signal<MigrationsView>({ history: [], pending: [], latestBatch: null });
  readonly backups = signal<CodeDbBackup[]>([]);

  ngOnInit() { void this.load(); }

  date(iso: string) { return this.i18n.date(iso, true); }
  badge(s: string) { return migrationStatusClass(s); }
  rollbackable(m: CodeDbMigration) { return canRollBack(m, this.mig().latestBatch); }

  statusLabel(s: string) {
    const c = migrationStatusClass(s);
    const key = c === 'badge-live' ? 'db.mig.st.ran' : c === 'badge-danger' ? 'db.mig.st.failed' : c === 'badge-warn' ? 'db.mig.st.rolledBack' : '';
    return key ? this.i18n.t(key) : s;
  }

  async load() {
    this.loading.set(true);
    try {
      const [m, b] = await Promise.all([this.api.codeDbMigrations(), this.api.codeDbBackups()]);
      this.mig.set(normalizeMigrations(m));
      this.backups.set(normalizeBackups(b));
      this.error.set('');
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.loading.set(false);
    }
  }

  async rollback(m: CodeDbMigration) {
    if (!(await Dialog.confirm({
      title: this.i18n.t('db.mig.rollbackTitle', { name: m.name }), text: this.i18n.t('db.mig.rollbackText'),
      confirm: this.i18n.t('db.mig.rollback'), danger: true,
    }))) return;
    this.busy.set(true);
    try {
      await this.api.rollbackCodeDbMigration(m.name);
      Dialog.toast(this.i18n.t('db.mig.rolledBack', { name: m.name }));
      await this.load();
    } catch (e) { Dialog.error(errorMessage(e)); } finally { this.busy.set(false); }
  }

  async download(b: CodeDbBackup) {
    try {
      const res = await this.api.downloadCodeDbBackup(b.id);
      saveBlob(res.body ?? new Blob(), filenameFromDisposition(res.headers.get('Content-Disposition'), `${b.backupTable || 'backup-' + b.id}.sql`));
    } catch (e) { Dialog.error(await blobErrorMessage(e)); }
  }

  async removeBackup(b: CodeDbBackup) {
    if (!(await Dialog.confirmDelete(b.backupTable || `#${b.id}`, this.i18n.t('db.backups.deleteText')))) return;
    try {
      await this.api.deleteCodeDbBackup(b.id);
      this.backups.set(this.backups().filter(x => x.id !== b.id));
      Dialog.toast(this.i18n.t('code.deleted'));
    } catch (e) { Dialog.error(errorMessage(e)); }
  }
}
