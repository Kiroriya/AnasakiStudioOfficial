import { ChangeDetectionStrategy, Component, ElementRef, Injector, OnDestroy, OnInit, afterNextRender, computed, inject, signal, viewChild } from '@angular/core';
import type * as Monaco from 'monaco-editor';
import { Api, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { CodeDbSqlResult, CodeDbStatus } from '../core/code-models';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { DbNav } from './db-nav';
import { CellView, formatCell, isReadSql, parseSqlParams, sqlRows } from './db-utils';
import { MonacoLoader } from './monaco-loader';
import './messages-db';

const DRAFT_KEY = 'anasaki.code.sql';
const DISPLAY_CAP = 1000;

/** /code/db/sql — SQL console on the code database: read-only by default, "allow writes" with edit_code_data. */
@Component({
  selector: 'rea-db-sql',
  imports: [TPipe, DbNav],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <rea-db-nav />
    @if (error()) { <p class="alert" id="db-sql-load-error">{{ error() }}</p> }
    @if (disabled()) {
      <div class="info-box warn" id="db-sql-disabled">⛔ {{ 'db.sql.disabled' | t }}</div>
    } @else if (status()) {
      <section class="card db-sql-card">
        <div class="db-sql-toolbar">
          <button class="btn btn-sm btn-primary" id="db-sql-run" [disabled]="busy()" (click)="run()" [title]="'db.sql.runHint' | t">▶ {{ busy() ? ('db.sql.running' | t) : ('db.sql.run' | t) }}</button>
          @if (canWrite()) {
            <label class="code-switch db-sql-write" id="db-sql-write" [class.on]="write()" [title]="'db.sql.writeHint' | t">
              <input type="checkbox" id="db-sql-write-input" [checked]="write()" (change)="toggleWrite($event)" />
              <span class="code-switch-track"><span class="code-switch-dot"></span></span>
              <span>{{ 'db.sql.allowWrites' | t }}</span>
            </label>
          }
          <span class="badge" id="db-sql-mode" [attr.data-mode]="write() ? 'write' : 'read'" [class.badge-danger]="write()" [class.badge-live]="!write()">
            {{ write() ? ('db.sql.modeWrite' | t) : ('db.sql.modeRead' | t) }}</span>
          <span class="grow"></span>
          <span class="muted small mono">{{ status()!.database }}</span>
        </div>
        <div class="db-sql-editor" id="db-sql-editor" #host></div>
        <label class="db-sql-params-label">{{ 'db.sql.params' | t }}
          <input class="mono" id="db-sql-params" [value]="params()" (input)="params.set(val($event))" placeholder='[1, "paid"]' />
        </label>
      </section>

      @if (runError()) { <p class="alert" id="db-sql-error">{{ runError() }}</p> }
      @if (result(); as r) {
        <section class="card table-card db-sql-result" id="db-sql-result">
          <div class="table-toolbar small" id="db-sql-meta">
            @if (grid().columns.length) { <span id="db-sql-rowcount">{{ 'db.sql.rows' | t: { n: grid().rows.length } }}</span> }
            @if (r.affectedRows !== undefined && r.affectedRows !== null) { <span id="db-sql-affected">{{ 'db.sql.affected' | t: { n: r.affectedRows } }}</span> }
            @if (r.durationMs !== undefined) { <span class="muted" id="db-sql-duration">{{ r.durationMs }} ms</span> }
            @if (capped()) { <span class="badge badge-warn" id="db-sql-capped">{{ 'db.sql.capped' | t: { n: cap } }}</span> }
          </div>
          @if (grid().columns.length) {
            <div class="db-grid-wrap">
              <table class="table db-grid" id="db-sql-grid">
                <thead><tr>@for (c of grid().columns; track $index) { <th class="mono">{{ c }}</th> }</tr></thead>
                <tbody>
                  @for (row of cells(); track $index) {
                    <tr class="db-sql-row">@for (cell of row; track $index) { <td class="db-cell" [attr.data-kind]="cell.kind" [class.null]="cell.kind === 'null'" [title]="cell.title ?? ''">{{ cell.text }}</td> }</tr>
                  } @empty { <tr><td [attr.colspan]="grid().columns.length" class="muted small">{{ 'db.sql.noRows' | t }}</td></tr> }
                </tbody>
              </table>
            </div>
          }
        </section>
      }
    }
  `,
})
export class DbSql implements OnInit, OnDestroy {
  private api = inject(Api);
  private auth = inject(Auth);
  private i18n = inject(I18n);
  private loader = inject(MonacoLoader);
  private injector = inject(Injector);
  private host = viewChild<ElementRef<HTMLDivElement>>('host');

  readonly cap = DISPLAY_CAP;
  readonly canWrite = computed(() => this.auth.can('edit_code_data'));
  readonly status = signal<CodeDbStatus | null>(null);
  readonly error = signal('');
  readonly disabled = computed(() => this.status()?.mode === 'fallback');
  readonly write = signal(false);
  readonly params = signal('');
  readonly busy = signal(false);
  readonly runError = signal('');
  readonly result = signal<CodeDbSqlResult | null>(null);
  readonly grid = computed(() => sqlRows(this.result()));
  readonly cells = computed<CellView[][]>(() => this.grid().rows.slice(0, DISPLAY_CAP).map(r => r.map(v => formatCell(v))));
  readonly capped = computed(() => !!this.result()?.truncated || this.grid().rows.length >= DISPLAY_CAP);

  private editor: Monaco.editor.IStandaloneCodeEditor | null = null;
  private destroyed = false;

  async ngOnInit() {
    try {
      this.status.set(await this.api.codeDbStatus());
    } catch (e) { this.error.set(errorMessage(e)); return; }
    if (this.disabled()) return;
    try {
      const m = await this.loader.load();
      if (this.destroyed) return;
      afterNextRender(() => this.createEditor(m), { injector: this.injector });
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  ngOnDestroy() {
    this.destroyed = true;
    if (this.editor) {
      this.saveDraft();
      this.editor.getModel()?.dispose();
      this.editor.dispose();
    }
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }

  private createEditor(m: typeof Monaco) {
    const host = this.host()?.nativeElement;
    if (!host || this.editor || this.destroyed) return;
    let text = 'SELECT * FROM core_posts LIMIT 20;';
    try { text = localStorage.getItem(DRAFT_KEY) ?? text; } catch { /* storage blocked */ }
    const model = m.editor.createModel(text, 'sql', m.Uri.parse('inmemory://site-code/console.sql'));
    this.editor = m.editor.create(host, {
      model, automaticLayout: true, minimap: { enabled: false }, fontSize: 13, tabSize: 2, scrollBeyondLastLine: false, fixedOverflowWidgets: true, theme: 'vs',
    });
    this.editor.addCommand(m.KeyMod.CtrlCmd | m.KeyCode.Enter, () => void this.run());
    this.editor.onDidBlurEditorText(() => this.saveDraft());
  }

  private saveDraft() {
    try { if (this.editor) localStorage.setItem(DRAFT_KEY, this.editor.getValue()); } catch { /* storage blocked */ }
  }

  /** The selected text when there is a selection, else the whole editor. */
  private sql(): string {
    const ed = this.editor;
    if (!ed) return '';
    const sel = ed.getSelection();
    const model = ed.getModel();
    if (sel && model && !sel.isEmpty()) return model.getValueInRange(sel);
    return ed.getValue();
  }

  async toggleWrite(e: Event) {
    const input = e.target as HTMLInputElement;
    const on = input.checked;
    if (!on) { this.write.set(false); return; }
    input.checked = false;
    const ok = await Dialog.confirm({
      title: this.i18n.t('db.sql.writeTitle'), text: this.i18n.t('db.sql.writeText'), confirm: this.i18n.t('db.sql.allowWrites'), danger: true,
    });
    if (!ok) return;
    input.checked = true;
    this.write.set(true);
  }

  async run() {
    if (this.busy()) return;
    const sql = this.sql().trim();
    if (!sql) return;
    const p = parseSqlParams(this.params());
    if (!p.ok) { this.runError.set(this.i18n.t('db.sql.paramsInvalid')); return; }
    const write = this.write() && this.canWrite();
    if (write && !isReadSql(sql) && !(await Dialog.confirm({
      title: this.i18n.t('db.sql.confirmWriteTitle'), text: sql.length > 300 ? sql.slice(0, 300) + '…' : sql, confirm: this.i18n.t('db.sql.run'), danger: true,
    }))) return;
    this.saveDraft();
    this.busy.set(true);
    this.runError.set('');
    try {
      this.result.set(await this.api.codeDbSql({ sql, params: p.params, write }));
    } catch (e) {
      this.result.set(null);
      this.runError.set(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
