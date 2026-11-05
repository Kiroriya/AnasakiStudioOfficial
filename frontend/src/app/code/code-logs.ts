import { ChangeDetectionStrategy, Component, OnDestroy, OnInit, computed, inject, signal } from '@angular/core';
import { Api, blobErrorMessage, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { CodeLog } from '../core/code-models';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { saveBlob } from '../shared/download';
import { Pager } from '../shared/pager';
import { SelectValue } from '../shared/select-value';
import { ActivatedRoute } from '@angular/router';
import { CodeTabs } from './code-tabs';
import { CodeUnits, UnitSwitcher } from './code-units';
import { filenameFromDisposition } from './code-utils';
import './messages-code';

const PER_PAGE = 50;
const AUTO_MS = 5000;

/** /code/logs — what the site's code logged (newest first), with filters, auto-refresh, download and clear. */
@Component({
  selector: 'rea-code-logs',
  imports: [TPipe, CodeTabs, Pager, SelectValue, UnitSwitcher],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <header class="page-head">
      <span class="page-icon">📜</span>
      <div class="page-head-text"><h1>{{ 'code.logs.title' | t }}</h1><p>{{ 'code.logs.desc' | t }}</p></div>
    </header>
    <rea-code-tabs />
    <rea-unit-switcher (changed)="onUnit()" />
    @if (error()) { <p class="alert" id="code-logs-error">{{ error() }}</p> }

    <section class="card table-card">
      <div class="table-toolbar code-logs-toolbar">
        <select id="code-logs-level" [value]="level()" (change)="setLevel(val($event))" [attr.aria-label]="'code.logs.level' | t">
          <option value="">{{ 'code.logs.allLevels' | t }}</option>
          @for (l of levels; track l) { <option [value]="l">{{ l }}</option> }
        </select>
        <input id="code-logs-source" class="code-logs-source" [value]="source()" (change)="setSource(val($event))" (keydown.enter)="setSource(val($event))" [placeholder]="'code.logs.source' | t" />
        <input id="code-logs-q" type="search" [value]="q()" (change)="setQ(val($event))" (keydown.enter)="setQ(val($event))" [placeholder]="'code.logs.search' | t" />
        <label class="check code-logs-auto"><input type="checkbox" id="code-logs-auto" [checked]="auto()" (change)="setAuto($event)" /> {{ 'code.logs.auto' | t }}</label>
        <button class="btn btn-sm" id="code-logs-refresh" (click)="load()">↻ {{ 'code.logs.refresh' | t }}</button>
        <span class="grow"></span>
        <details class="code-logs-dl">
          <summary class="btn btn-sm" id="code-logs-download-open">⬇ {{ 'code.logs.download' | t }}</summary>
          <div class="code-logs-dl-pop">
            <label>{{ 'code.logs.from' | t }}<input type="datetime-local" id="code-logs-from" [value]="from()" (change)="from.set(val($event))" /></label>
            <label>{{ 'code.logs.to' | t }}<input type="datetime-local" id="code-logs-to" [value]="to()" (change)="to.set(val($event))" /></label>
            <button class="btn btn-sm btn-primary" id="code-logs-download" (click)="download()">⬇ {{ 'code.logs.downloadTxt' | t }}</button>
          </div>
        </details>
        @if (canManage()) { <button class="btn btn-sm btn-danger" id="code-logs-clear" (click)="clear()">🗑 {{ 'code.logs.clear' | t }}</button> }
      </div>
      <table class="table code-logs-table" id="code-logs">
        <thead><tr><th>{{ 'code.logs.at' | t }}</th><th>{{ 'code.logs.level' | t }}</th><th>{{ 'code.logs.sourceCol' | t }}</th><th>{{ 'code.logs.message' | t }}</th><th class="num">{{ 'code.logs.version' | t }}</th></tr></thead>
        <tbody>
          @for (l of items(); track l.id) {
            <tr class="code-log-row" [attr.data-level]="l.level" [attr.data-id]="l.id" [class.open]="open() === l.id" (click)="toggle(l)">
              <td class="mono small nowrap">{{ when(l.at) }}</td>
              <td><span class="code-level code-log-level" [attr.data-level]="l.level">{{ l.level }}</span></td>
              <td class="mono small code-log-source">{{ l.source }}</td>
              <td class="code-log-message">
                <span class="code-log-text">{{ l.message }}</span>
                @if (l.trace && open() === l.id) { <pre class="code-log-trace mono">{{ l.trace }}</pre> }
                @else if (l.trace) { <span class="muted small"> · {{ 'code.logs.trace' | t }} ▸</span> }
              </td>
              <td class="num mono small">{{ l.version ? '#' + l.version : '—' }}</td>
            </tr>
          } @empty {
            <tr><td colspan="5" class="empty-state" id="code-logs-empty">{{ loading() ? ('common.loading' | t) : ('code.logs.empty' | t) }}</td></tr>
          }
        </tbody>
      </table>
      <rea-pager [page]="page()" [total]="total()" [perPage]="perPage" (pageChange)="setPage($event)" />
    </section>
  `,
})
export class CodeLogs implements OnInit, OnDestroy {
  private api = inject(Api);
  private auth = inject(Auth);
  private i18n = inject(I18n);
  private units = inject(CodeUnits);
  private route = inject(ActivatedRoute);

  readonly levels = ['debug', 'info', 'warn', 'error'];
  readonly perPage = PER_PAGE;
  readonly canManage = computed(() => this.auth.can('manage_scripts'));
  readonly loading = signal(true);
  readonly error = signal('');
  readonly items = signal<CodeLog[]>([]);
  readonly total = signal(0);
  readonly page = signal(1);
  readonly level = signal('');
  readonly source = signal('');
  readonly q = signal('');
  readonly from = signal('');
  readonly to = signal('');
  readonly auto = signal(false);
  readonly open = signal<number | null>(null);
  private timer: ReturnType<typeof setInterval> | null = null;
  private seq = 0;

  ngOnInit() { this.units.adopt(this.route); void this.load(); }
  onUnit() { this.page.set(1); this.open.set(null); void this.load(); }
  ngOnDestroy() { this.stopTimer(); }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  when(iso: string) {
    const d = new Date(iso);
    return isNaN(+d) ? iso : d.toLocaleString(this.i18n.intl(), { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit', second: '2-digit' });
  }

  setLevel(v: string) { this.level.set(v); this.page.set(1); void this.load(); }
  setSource(v: string) { if (v === this.source()) return; this.source.set(v.trim()); this.page.set(1); void this.load(); }
  setQ(v: string) { if (v === this.q()) return; this.q.set(v.trim()); this.page.set(1); void this.load(); }
  setPage(p: number) { this.page.set(p); void this.load(); }
  toggle(l: CodeLog) { if (l.trace) this.open.set(this.open() === l.id ? null : l.id); }

  setAuto(e: Event) {
    const on = (e.target as HTMLInputElement).checked;
    this.auto.set(on);
    this.stopTimer();
    if (on) this.timer = setInterval(() => void this.load(true), AUTO_MS);
  }

  private stopTimer() { if (this.timer) { clearInterval(this.timer); this.timer = null; } }

  async load(quiet = false) {
    const seq = ++this.seq;
    if (!quiet) this.loading.set(true);
    try {
      const res = await this.api.codeLogs({ level: this.level(), source: this.source(), q: this.q(), page: this.page(), perPage: PER_PAGE, unit: this.units.param() });
      if (seq !== this.seq) return;
      this.items.set(res.items ?? []);
      this.total.set(res.total ?? 0);
      this.error.set('');
    } catch (e) {
      if (seq === this.seq) this.error.set(errorMessage(e));
    } finally {
      if (seq === this.seq) this.loading.set(false);
    }
  }

  /** datetime-local value → RFC 3339 in UTC ('' stays ''). */
  private iso(v: string) {
    if (!v) return '';
    const d = new Date(v);
    return isNaN(+d) ? '' : d.toISOString();
  }

  async download() {
    try {
      const res = await this.api.downloadCodeLogs({ level: this.level(), from: this.iso(this.from()), to: this.iso(this.to()), unit: this.units.param() });
      saveBlob(res.body ?? new Blob(), filenameFromDisposition(res.headers.get('Content-Disposition'), 'site-code-logs.txt'));
    } catch (e) { Dialog.error(await blobErrorMessage(e)); }
  }

  async clear() {
    if (!(await Dialog.confirm({ title: this.i18n.t('code.logs.clearTitle'), text: this.i18n.t('code.logs.clearText'), confirm: this.i18n.t('code.logs.clear'), danger: true }))) return;
    try {
      await this.api.clearCodeLogs();
      this.page.set(1);
      await this.load();
      Dialog.toast(this.i18n.t('code.logs.cleared'));
    } catch (e) { Dialog.error(errorMessage(e)); }
  }
}
