import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { Api, blobErrorMessage, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { CodeDbColumn, CodeDbRow, CodeDbTable, CodeDbTableDetail } from '../core/code-models';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { saveBlob } from '../shared/download';
import { Pager } from '../shared/pager';
import { filenameFromDisposition, formatBytes } from './code-utils';
import { DbNav } from './db-nav';
import {
  CellView, ImportSummary, dbExportQuery, dbRowsQuery, formatCell, isAutoColumn, nextSort, parseInput, summarizeImport, toInputValue,
} from './db-utils';
import './messages-db';

type View = 'data' | 'structure' | 'import';
const VIEWS: View[] = ['data', 'structure', 'import'];
const PER_PAGE = 50;

interface Field { text: string; isNull: boolean }
interface EditState {
  /** Primary key of the edited row; null = a new row. */
  key: string | number | null;
  original: CodeDbRow | null;
  fields: Record<string, Field>;
  errors: Record<string, string>;
  busy: boolean;
}
interface ViewRow { key: string | number | null; index: number; row: CodeDbRow; cells: CellView[] }
interface ImportState { file: File | null; summary: ImportSummary | null; busy: boolean; error: string; checked: boolean }

/** /code/db/tables/:name — structure, data grid (search/sort/filter/inline edit), CSV import, export. */
@Component({
  selector: 'rea-db-table',
  imports: [TPipe, RouterLink, DbNav, Pager],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <rea-db-nav />
    <div class="db-table-head">
      <a class="btn btn-sm btn-ghost" id="db-back" routerLink="/code/db">← {{ 'db.tables' | t }}</a>
      <h2 class="mono" id="db-table-name">{{ name() }}</h2>
      @if (meta()?.view) { <span class="badge db-badge-view">{{ 'db.view' | t }}</span> }
      @if (meta()?.system) { <span class="badge db-badge-system">{{ 'db.system' | t }}</span> }
      @if (!writable()) { <span class="badge" id="db-readonly" [title]="readOnlyReason()">🔒 {{ 'db.readOnly' | t }}</span> }
      <span class="grow"></span>
      <div class="db-views" role="tablist">
        <button class="db-view-btn" id="db-view-data" role="tab" [class.on]="view() === 'data'" (click)="setView('data')">{{ 'db.view.data' | t }}</button>
        <button class="db-view-btn" id="db-view-structure" role="tab" [class.on]="view() === 'structure'" (click)="setView('structure')">{{ 'db.view.structure' | t }}</button>
        @if (canImport()) {
          <button class="db-view-btn" id="db-view-import" role="tab" [class.on]="view() === 'import'" (click)="setView('import')">{{ 'db.view.import' | t }}</button>
        }
      </div>
    </div>
    @if (error()) { <p class="alert" id="db-table-error">{{ error() }}</p> }

    @switch (view()) {
      @case ('structure') {
        @if (detail(); as d) {
          <section class="card table-card db-structure">
            <div class="table-toolbar"><b>{{ 'db.columns' | t }}</b></div>
            <table class="table" id="db-columns">
              <thead><tr><th>{{ 'db.col.name' | t }}</th><th>{{ 'db.col.type' | t }}</th><th>{{ 'db.col.null' | t }}</th><th>{{ 'db.col.default' | t }}</th><th>{{ 'db.col.key' | t }}</th><th>{{ 'db.col.extra' | t }}</th><th>{{ 'db.comment' | t }}</th></tr></thead>
              <tbody>
                @for (c of d.columns; track c.name) {
                  <tr class="db-col-row" [attr.data-col]="c.name">
                    <td class="mono"><b>{{ c.name }}</b></td>
                    <td class="mono small">{{ c.type }}</td>
                    <td>{{ c.nullable ? ('db.yes' | t) : ('db.no' | t) }}</td>
                    <td class="mono small">{{ c.default ?? (c.nullable ? 'NULL' : '—') }}</td>
                    <td>@if (c.key) { <span class="badge" [class.badge-info]="c.key === 'PRI'">{{ c.key }}</span> }</td>
                    <td class="mono small">{{ c.extra }}</td>
                    <td class="muted small">{{ c.comment }}</td>
                  </tr>
                }
              </tbody>
            </table>
          </section>
          <section class="card table-card db-structure">
            <div class="table-toolbar"><b>{{ 'db.indexes' | t }}</b></div>
            <table class="table" id="db-indexes">
              <thead><tr><th>{{ 'db.col.name' | t }}</th><th>{{ 'db.columns' | t }}</th><th>{{ 'db.unique' | t }}</th></tr></thead>
              <tbody>
                @for (i of d.indexes; track i.name) {
                  <tr class="db-index-row" [attr.data-index]="i.name"><td class="mono">{{ i.name }}</td><td class="mono small">{{ i.columns.join(', ') }}</td><td>{{ i.unique ? '✓' : '' }}</td></tr>
                } @empty { <tr><td colspan="3" class="muted small">{{ 'db.none' | t }}</td></tr> }
              </tbody>
            </table>
          </section>
          <section class="card table-card db-structure">
            <div class="table-toolbar"><b>{{ 'db.fks' | t }}</b></div>
            <table class="table" id="db-fks">
              <thead><tr><th>{{ 'db.col.name' | t }}</th><th>{{ 'db.references' | t }}</th><th>{{ 'db.onDelete' | t }}</th></tr></thead>
              <tbody>
                @for (f of d.foreignKeys; track f.column + f.refTable) {
                  <tr class="db-fk-row" [attr.data-col]="f.column">
                    <td class="mono">{{ f.column }}</td>
                    <td class="mono"><a [routerLink]="['/code/db/tables', f.refTable]">{{ f.refTable }}</a>.{{ f.refColumn }}</td>
                    <td class="mono small">{{ f.onDelete || '—' }}</td>
                  </tr>
                } @empty { <tr><td colspan="3" class="muted small">{{ 'db.none' | t }}</td></tr> }
              </tbody>
            </table>
            @if (d.relations?.length) {
              <div class="table-toolbar"><b>{{ 'db.relations' | t }}</b><span class="hint">{{ 'db.relationsHint' | t }}</span></div>
              <table class="table" id="db-relations">
                <tbody>
                  @for (f of d.relations; track f.column + f.refTable) {
                    <tr class="db-relation-row" [attr.data-col]="f.column"><td class="mono">{{ f.column }}</td><td class="mono muted">→ {{ f.refTable }}.{{ f.refColumn }}</td><td class="mono small">{{ f.onDelete || '' }}</td></tr>
                  }
                </tbody>
              </table>
            }
          </section>
        } @else if (loadingDetail()) { <p class="muted">{{ 'common.loading' | t }}</p> }
      }

      @case ('import') {
        @if (imp(); as s) {
          <section class="card db-import" id="db-import">
            <h3>{{ 'db.import.title' | t }}</h3>
            <p class="hint">{{ 'db.import.hint' | t: { cols: columnNames().join(', ') } }}</p>
            <label>{{ 'db.import.file' | t }}<input type="file" id="db-import-file" accept=".csv,text/csv" (change)="pickImport($event)" /></label>
            @if (s.busy) { <p class="muted" id="db-import-busy">{{ 'db.import.checking' | t }}</p> }
            @if (s.error) { <p class="alert" id="db-import-error">{{ s.error }}</p> }
            @if (s.summary; as sm) {
              <div class="db-import-result" id="db-import-result" [attr.data-ok]="sm.ok">
                @if (sm.ok) {
                  <p class="db-import-ok" id="db-import-ok">✓ {{ (s.checked ? 'db.import.wouldInsert' : 'db.import.inserted') | t: { n: sm.inserted } }}</p>
                } @else {
                  <p class="hint danger" id="db-import-summary">{{ 'db.import.errors' | t: { n: sm.errors.length, rows: sm.badRows } }}</p>
                  <table class="table db-import-errors" id="db-import-errors">
                    <thead><tr><th class="num">{{ 'db.import.row' | t }}</th><th>{{ 'db.import.message' | t }}</th></tr></thead>
                    <tbody>
                      @for (e of sm.shown; track $index) {
                        <tr class="db-import-error" [attr.data-row]="e.row"><td class="num mono">{{ e.file ? ('db.import.wholeFile' | t) : e.row }}</td><td>{{ e.message }}</td></tr>
                      }
                    </tbody>
                  </table>
                  @if (sm.more) { <p class="muted small" id="db-import-more">{{ 'db.import.more' | t: { n: sm.more } }}</p> }
                  <p class="hint">{{ 'db.import.fix' | t }}</p>
                }
              </div>
            }
            <div class="form-foot">
              <button class="btn" id="db-import-check" [disabled]="!s.file || s.busy" (click)="checkImport()">{{ 'db.import.check' | t }}</button>
              <button class="btn btn-primary" id="db-import-confirm" [disabled]="!s.file || s.busy || !s.checked || !(s.summary && s.summary.ok && s.summary.inserted)" (click)="confirmImport()">⬆ {{ 'db.import.confirm' | t }}</button>
            </div>
          </section>
        }
      }

      @default {
        <section class="card table-card db-data">
          <div class="table-toolbar db-grid-toolbar">
            <input type="search" id="db-grid-q" [value]="q()" (change)="setQ(val($event))" (keydown.enter)="setQ(val($event))" [placeholder]="'db.grid.search' | t" />
            <label class="check"><input type="checkbox" id="db-grid-filters-toggle" [checked]="showFilters()" (change)="showFilters.set(!showFilters())" /> {{ 'db.grid.filters' | t }}</label>
            <button class="btn btn-sm" id="db-grid-refresh" (click)="loadRows()">↻ {{ 'code.logs.refresh' | t }}</button>
            <span class="grow"></span>
            <button class="btn btn-sm" id="db-export-csv" (click)="exportAs('csv')">⬇ CSV</button>
            <button class="btn btn-sm" id="db-export-json" (click)="exportAs('json')">⬇ JSON</button>
            @if (canAdd()) { <button class="btn btn-sm btn-primary" id="db-row-add" [disabled]="!!editing()" (click)="startAdd()">＋ {{ 'db.grid.add' | t }}</button> }
          </div>
          <div class="db-grid-wrap">
            <table class="table db-grid" id="db-grid">
              <thead>
                <tr>
                  @for (c of columns(); track c.name) {
                    <th class="db-grid-th" [attr.data-col]="c.name" [class.sort-asc]="sort() === c.name && dir() === 'asc'" [class.sort-desc]="sort() === c.name && dir() === 'desc'"
                      (click)="toggleSort(c.name)" [title]="c.type">
                      {{ c.name }}@if (c.name === pk()) { <span class="db-pk-mark">🔑</span> }
                      <span class="db-sort-mark">{{ sort() === c.name ? (dir() === 'asc' ? '▲' : '▼') : '' }}</span>
                    </th>
                  }
                  @if (writable()) { <th class="th-actions"></th> }
                </tr>
                @if (showFilters()) {
                  <tr class="db-grid-filter-row">
                    @for (c of columns(); track c.name) {
                      <th><input class="db-grid-filter" [attr.data-col]="c.name" [value]="filters()[c.name] ?? ''" (change)="setFilter(c.name, val($event))" (keydown.enter)="setFilter(c.name, val($event))" [placeholder]="'db.grid.filter' | t" /></th>
                    }
                    @if (writable()) { <th></th> }
                  </tr>
                }
              </thead>
              <tbody>
                @if (editing(); as e) {
                  @if (e.key === null) {
                    <tr class="db-grid-row editing new" id="db-row-new">
                      @for (c of columns(); track c.name) { <td class="db-cell-edit" [attr.data-col]="c.name">
                        <input class="db-cell-input mono" [attr.data-col]="c.name" [class.invalid]="!!e.errors[c.name]" [title]="e.errors[c.name] || c.type"
                          [value]="e.fields[c.name]?.text ?? ''" [disabled]="e.fields[c.name]?.isNull" (input)="setField(c.name, val($event))"
                          [placeholder]="auto(c) ? ('db.grid.auto' | t) : c.type" />
                        @if (c.nullable) { <label class="db-null-toggle"><input type="checkbox" class="db-cell-null" [attr.data-col]="c.name" [checked]="e.fields[c.name]?.isNull" (change)="setNull(c.name, $event)" />NULL</label> }
                      </td> }
                      <td class="row-actions">
                        <button class="btn btn-sm btn-primary db-row-save" [disabled]="e.busy" (click)="saveEdit()">{{ 'common.save' | t }}</button>
                        <button class="btn btn-sm db-row-cancel" (click)="editing.set(null)">{{ 'common.cancel' | t }}</button>
                      </td>
                    </tr>
                  }
                }
                @for (r of viewRows(); track r.key ?? r.index) {
                  @if (isEditing(r)) {
                    <tr class="db-grid-row editing" [attr.data-pk]="r.key">
                      @for (c of columns(); track c.name) { <td class="db-cell-edit" [attr.data-col]="c.name">
                        @if (c.name === pk()) { <span class="mono">{{ r.key }}</span> }
                        @else {
                          <input class="db-cell-input mono" [attr.data-col]="c.name" [class.invalid]="!!editing()!.errors[c.name]" [title]="editing()!.errors[c.name] || c.type"
                            [value]="editing()!.fields[c.name]?.text ?? ''" [disabled]="editing()!.fields[c.name]?.isNull" (input)="setField(c.name, val($event))" (keydown.enter)="saveEdit()" (keydown.escape)="editing.set(null)" />
                          @if (c.nullable) { <label class="db-null-toggle"><input type="checkbox" class="db-cell-null" [attr.data-col]="c.name" [checked]="editing()!.fields[c.name]?.isNull" (change)="setNull(c.name, $event)" />NULL</label> }
                        }
                      </td> }
                      <td class="row-actions">
                        <button class="btn btn-sm btn-primary db-row-save" [disabled]="editing()!.busy" (click)="saveEdit()">{{ 'common.save' | t }}</button>
                        <button class="btn btn-sm db-row-cancel" (click)="editing.set(null)">{{ 'common.cancel' | t }}</button>
                      </td>
                    </tr>
                  } @else {
                    <tr class="db-grid-row" [attr.data-pk]="r.key">
                      @for (cell of r.cells; track $index) {
                        <td class="db-cell" [attr.data-col]="columns()[$index]?.name" [attr.data-kind]="cell.kind" [class.null]="cell.kind === 'null'" [title]="cell.title ?? ''"
                          (dblclick)="startEdit(r)">{{ cell.text }}</td>
                      }
                      @if (writable()) {
                        <td class="row-actions">
                          <button class="icon-btn db-row-edit" [title]="'common.edit' | t" [disabled]="!!editing()" (click)="startEdit(r)">✎</button>
                          <button class="icon-btn danger db-row-delete" [title]="'common.delete' | t" (click)="remove(r)">🗑</button>
                        </td>
                      }
                    </tr>
                  }
                } @empty {
                  @if (editing()?.key !== null) {
                    <tr><td [attr.colspan]="columns().length + 1" class="empty-state" id="db-grid-empty">{{ loadingRows() ? ('common.loading' | t) : ('db.grid.empty' | t) }}</td></tr>
                  }
                }
              </tbody>
            </table>
          </div>
          <rea-pager [page]="page()" [total]="total()" [perPage]="perPage" (pageChange)="setPage($event)" />
        </section>
      }
    }
  `,
})
export class DbTable {
  private api = inject(Api);
  private auth = inject(Auth);
  private i18n = inject(I18n);
  private route = inject(ActivatedRoute);
  private router = inject(Router);

  readonly perPage = PER_PAGE;
  readonly canEdit = computed(() => this.auth.can('edit_code_data'));

  readonly name = signal('');
  readonly view = signal<View>('data');
  readonly error = signal('');
  readonly meta = signal<CodeDbTable | null>(null);
  readonly detail = signal<CodeDbTableDetail | null>(null);
  readonly loadingDetail = signal(true);

  readonly loadingRows = signal(true);
  readonly items = signal<CodeDbRow[]>([]);
  readonly total = signal(0);
  readonly pk = signal<string | null>(null);
  readonly page = signal(1);
  readonly q = signal('');
  readonly sort = signal<string | undefined>(undefined);
  readonly dir = signal<'asc' | 'desc' | undefined>(undefined);
  readonly filters = signal<Record<string, string>>({});
  readonly showFilters = signal(false);
  readonly editing = signal<EditState | null>(null);
  readonly imp = signal<ImportState>({ file: null, summary: null, busy: false, error: '', checked: false });

  readonly readonlyTable = computed(() => !!this.meta()?.view || !!this.meta()?.system);
  readonly canAdd = computed(() => this.canEdit() && !this.readonlyTable() && !!this.detail());
  readonly canImport = computed(() => this.canAdd());
  /** Rows can be edited/deleted: edit_code_data, a developer table, and a primary key to address rows. */
  readonly writable = computed(() => this.canAdd() && !!this.pk());
  readonly readOnlyReason = computed(() =>
    !this.canEdit() ? this.i18n.t('db.readOnly.cap') : this.readonlyTable() ? this.i18n.t('db.readOnly.view') : this.i18n.t('db.readOnly.noPk'));

  /** Columns from the structure; falls back to the keys of the first row. */
  readonly columns = computed<CodeDbColumn[]>(() => {
    const d = this.detail();
    if (d?.columns?.length) return d.columns;
    const first = this.items()[0];
    return first ? Object.keys(first).map(name => ({ name, type: '', nullable: true, default: null, key: '', extra: '', comment: '' })) : [];
  });
  readonly columnNames = computed(() => this.columns().filter(c => !isAutoColumn(c)).map(c => c.name));
  readonly viewRows = computed<ViewRow[]>(() => {
    const cols = this.columns();
    const pk = this.pk();
    return this.items().map((row, index) => ({
      key: pk ? (row[pk] as string | number | null) ?? null : null,
      index, row,
      cells: cols.map(c => formatCell(row[c.name], c.type)),
    }));
  });

  private seq = 0;

  constructor() {
    const destroy = inject(DestroyRef);
    this.route.paramMap.pipe(takeUntilDestroyed(destroy)).subscribe(p => {
      const name = p.get('name') ?? '';
      if (name === this.name()) return;
      this.name.set(name);
      this.reset();
      void this.loadTable();
    });
    this.route.queryParamMap.pipe(takeUntilDestroyed(destroy)).subscribe(p => {
      const v = p.get('view') as View | null;
      this.view.set(v && VIEWS.includes(v) ? v : 'data');
    });
  }

  private reset() {
    this.meta.set(null);
    this.detail.set(null);
    this.items.set([]);
    this.total.set(0);
    this.pk.set(null);
    this.page.set(1);
    this.q.set('');
    this.sort.set(undefined);
    this.dir.set(undefined);
    this.filters.set({});
    this.editing.set(null);
    this.error.set('');
    this.imp.set({ file: null, summary: null, busy: false, error: '', checked: false });
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  size(n: number) { return formatBytes(n || 0); }
  auto(c: CodeDbColumn) { return isAutoColumn(c); }

  setView(v: View) {
    void this.router.navigate([], { relativeTo: this.route, queryParams: { view: v === 'data' ? null : v }, replaceUrl: true });
  }

  private async loadTable() {
    const name = this.name();
    this.loadingDetail.set(true);
    try {
      const [list, detail] = await Promise.all([this.api.codeDbTables().catch(() => [] as CodeDbTable[]), this.api.codeDbTable(name)]);
      if (name !== this.name()) return;
      this.meta.set(list.find(t => t.name === name) ?? { name, rows: 0, sizeBytes: 0, view: name.startsWith('core_') });
      this.detail.set(detail);
      const pri = detail.columns.filter(c => c.key === 'PRI');
      if (pri.length === 1) this.pk.set(pri[0].name);
    } catch (e) {
      if (name === this.name()) this.error.set(errorMessage(e));
    } finally {
      if (name === this.name()) this.loadingDetail.set(false);
    }
    await this.loadRows();
  }

  private gridQuery() {
    return { page: this.page(), perPage: PER_PAGE, q: this.q(), sort: this.sort(), dir: this.dir(), filters: this.filters() };
  }

  async loadRows() {
    const seq = ++this.seq;
    const name = this.name();
    this.loadingRows.set(true);
    try {
      const res = await this.api.codeDbRows(name, dbRowsQuery(this.gridQuery()));
      if (seq !== this.seq) return;
      this.items.set(res.items ?? []);
      this.total.set(res.total ?? 0);
      if (res.primaryKey !== undefined) this.pk.set(res.primaryKey || null);
      this.error.set('');
    } catch (e) {
      if (seq === this.seq) this.error.set(errorMessage(e));
    } finally {
      if (seq === this.seq) this.loadingRows.set(false);
    }
  }

  setQ(v: string) { if (v.trim() === this.q()) return; this.q.set(v.trim()); this.page.set(1); void this.loadRows(); }
  setPage(p: number) { this.page.set(p); this.editing.set(null); void this.loadRows(); }
  setFilter(col: string, v: string) {
    if ((this.filters()[col] ?? '') === v.trim()) return;
    this.filters.set({ ...this.filters(), [col]: v.trim() });
    this.page.set(1);
    void this.loadRows();
  }
  toggleSort(col: string) {
    const n = nextSort({ sort: this.sort(), dir: this.dir() }, col);
    this.sort.set(n.sort);
    this.dir.set(n.dir);
    void this.loadRows();
  }

  // ---------------------------------------------------------------- export

  async exportAs(format: 'csv' | 'json') {
    try {
      const res = await this.api.exportCodeDb(this.name(), format, dbExportQuery(this.gridQuery()));
      saveBlob(res.body ?? new Blob(), filenameFromDisposition(res.headers.get('Content-Disposition'), `${this.name()}.${format}`));
    } catch (e) { Dialog.error(await blobErrorMessage(e)); }
  }

  // ---------------------------------------------------------------- inline edit

  isEditing(r: ViewRow) { const e = this.editing(); return !!e && e.key !== null && e.key === r.key; }

  startEdit(r: ViewRow) {
    if (!this.writable() || r.key === null || this.editing()) return;
    const fields: Record<string, Field> = {};
    for (const c of this.columns()) fields[c.name] = toInputValue(r.row[c.name], c.type);
    this.editing.set({ key: r.key, original: r.row, fields, errors: {}, busy: false });
  }

  startAdd() {
    if (!this.canAdd() || this.editing()) return;
    const fields: Record<string, Field> = {};
    for (const c of this.columns()) fields[c.name] = { text: '', isNull: false };
    this.editing.set({ key: null, original: null, fields, errors: {}, busy: false });
  }

  setField(col: string, text: string) {
    const e = this.editing();
    if (!e) return;
    const { [col]: _gone, ...errors } = e.errors;
    this.editing.set({ ...e, fields: { ...e.fields, [col]: { ...e.fields[col], text } }, errors });
  }

  setNull(col: string, ev: Event) {
    const e = this.editing();
    if (!e) return;
    const isNull = (ev.target as HTMLInputElement).checked;
    this.editing.set({ ...e, fields: { ...e.fields, [col]: { ...e.fields[col], isNull } } });
  }

  /** The body to send: changed columns for an edit; typed (or NULL-ticked) columns for a new row. */
  private payload(e: EditState): { row: CodeDbRow; errors: Record<string, string> } {
    const row: CodeDbRow = {};
    const errors: Record<string, string> = {};
    for (const c of this.columns()) {
      if (e.key !== null && c.name === this.pk()) continue;
      const f = e.fields[c.name];
      if (!f) continue;
      if (e.key === null && !f.isNull && f.text === '') continue; // leave it to the column default
      const p = parseInput(f.text, c.type, f.isNull);
      if (!p.ok) { errors[c.name] = this.i18n.t('db.parse.' + p.error); continue; }
      if (e.original && JSON.stringify(p.value) === JSON.stringify(e.original[c.name] ?? null)) continue;
      row[c.name] = p.value;
    }
    return { row, errors };
  }

  async saveEdit() {
    const e = this.editing();
    if (!e || e.busy) return;
    const { row, errors } = this.payload(e);
    if (Object.keys(errors).length) { this.editing.set({ ...e, errors }); return; }
    if (e.key !== null && !Object.keys(row).length) { this.editing.set(null); return; }
    this.editing.set({ ...e, busy: true, errors: {} });
    try {
      if (e.key === null) await this.api.createCodeDbRow(this.name(), row);
      else await this.api.updateCodeDbRow(this.name(), e.key, row);
      this.editing.set(null);
      Dialog.toast(this.i18n.t(e.key === null ? 'db.grid.added' : 'db.grid.saved'));
      await this.loadRows();
    } catch (err) {
      const cur = this.editing();
      if (cur) this.editing.set({ ...cur, busy: false });
      Dialog.error(errorMessage(err));
    }
  }

  async remove(r: ViewRow) {
    if (!this.writable() || r.key === null) return;
    if (!(await Dialog.confirmDelete(`${this.name()} #${r.key}`, this.i18n.t('db.grid.deleteText')))) return;
    try {
      await this.api.deleteCodeDbRow(this.name(), r.key);
      Dialog.toast(this.i18n.t('db.grid.deleted'));
      if (this.items().length === 1 && this.page() > 1) this.page.set(this.page() - 1);
      await this.loadRows();
    } catch (err) { Dialog.error(errorMessage(err)); }
  }

  // ---------------------------------------------------------------- CSV import

  pickImport(e: Event) {
    const file = (e.target as HTMLInputElement).files?.[0] ?? null;
    this.imp.set({ file, summary: null, busy: false, error: '', checked: false });
    if (file) void this.checkImport();
  }

  /** Dry run: what would be inserted and which rows are wrong (nothing is written). */
  async checkImport() {
    const s = this.imp();
    if (!s.file) return;
    const file = s.file;
    this.imp.set({ ...s, busy: true, error: '', summary: null, checked: false });
    try {
      const res = await this.api.importCodeDb(this.name(), file, true);
      if (this.imp().file !== file) return;
      this.imp.set({ ...this.imp(), busy: false, summary: summarizeImport(res), checked: true });
    } catch (e) {
      if (this.imp().file === file) this.imp.set({ ...this.imp(), busy: false, error: errorMessage(e) });
    }
  }

  async confirmImport() {
    const s = this.imp();
    if (!s.file || !s.summary?.ok) return;
    if (!(await Dialog.confirm({
      title: this.i18n.t('db.import.confirmTitle', { n: s.summary.inserted, table: this.name() }),
      text: this.i18n.t('db.import.confirmText'), confirm: this.i18n.t('db.import.confirm'),
    }))) return;
    this.imp.set({ ...s, busy: true, error: '' });
    try {
      const res = await this.api.importCodeDb(this.name(), s.file, false);
      const summary = summarizeImport(res);
      this.imp.set({ file: s.file, summary, busy: false, error: '', checked: false });
      if (summary.ok) Dialog.toast(this.i18n.t('db.import.inserted', { n: summary.inserted }));
      this.page.set(1);
      await this.loadRows();
    } catch (e) {
      this.imp.set({ ...this.imp(), busy: false, error: errorMessage(e) });
    }
  }
}
