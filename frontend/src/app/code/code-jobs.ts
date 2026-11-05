import { ChangeDetectionStrategy, Component, OnDestroy, OnInit, computed, inject, signal } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { CodeCron, CodeJob } from '../core/code-models';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { Pager } from '../shared/pager';
import { SelectValue } from '../shared/select-value';
import { ActivatedRoute } from '@angular/router';
import { CodeTabs } from './code-tabs';
import { CodeUnits, UnitSwitcher } from './code-units';
import { JOBS_PER_PAGE, JOB_STATUSES, canCancel, canRetry, formatDuration, jobProgress, jobsQuery, prettyValue } from './jobs-utils';
import './messages-code4';

const AUTO_MS = 5000;

/**
 * /code/jobs (view_scripts) — background jobs (jobs.enqueue) with status
 * filter, progress, errors; cancel / retry and "run now" for schedules
 * (site.cron) need publish_scripts.
 */
@Component({
  selector: 'rea-code-jobs',
  imports: [TPipe, CodeTabs, Pager, SelectValue, UnitSwitcher],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <header class="page-head">
      <span class="page-icon">⏱</span>
      <div class="page-head-text"><h1>{{ 'cj.title' | t }}</h1><p>{{ 'cj.desc' | t }}</p></div>
    </header>
    <rea-code-tabs />
    <rea-unit-switcher (changed)="onUnit()" />
    @if (error()) { <p class="alert" id="code-jobs-error">{{ error() }}</p> }
    @if (!canAct()) { <p class="hint" id="code-jobs-noperm">🔒 {{ 'cj.noPerm' | t }}</p> }

    <section class="card table-card cj-card" id="code-cron-card">
      <div class="table-toolbar"><h3 class="cj-h">🗓 {{ 'cj.cron' | t }}</h3><span class="grow"></span><span class="muted small">{{ 'cj.cron.tz' | t }}</span></div>
      <table class="table" id="code-cron">
        <thead><tr><th>{{ 'cj.cron.name' | t }}</th><th>{{ 'cj.cron.expr' | t }}</th><th>{{ 'cj.cron.next' | t }}</th><th>{{ 'cj.cron.last' | t }}</th><th>{{ 'cj.cron.status' | t }}</th><th class="th-actions"></th></tr></thead>
        <tbody>
          @for (c of cron(); track c.name) {
            <tr class="code-cron-row" [attr.data-name]="c.name">
              <td class="mono"><b>{{ c.name }}</b></td>
              <td class="mono small code-cron-expr">{{ c.expr }}</td>
              <td class="small code-cron-next">{{ when(c.nextRun) }}</td>
              <td class="small code-cron-last">{{ when(c.lastRun) }}</td>
              <td>@if (c.lastStatus) { <span class="cj-status code-cron-status" [attr.data-status]="c.lastStatus">{{ statusLabel(c.lastStatus) }}</span> } @else { <span class="muted">—</span> }</td>
              <td class="row-actions">
                @if (canAct()) { <button class="btn btn-sm code-cron-run" [disabled]="busy() === 'cron:' + c.name" (click)="runCron(c)">▶ {{ 'cj.cron.run' | t }}</button> }
              </td>
            </tr>
          } @empty {
            <tr><td colspan="6" class="empty-state" id="code-cron-empty">{{ loadingCron() ? ('common.loading' | t) : ('cj.cron.empty' | t) }}</td></tr>
          }
        </tbody>
      </table>
    </section>

    <section class="card table-card cj-card" id="code-jobs-card">
      <div class="table-toolbar cj-toolbar">
        <h3 class="cj-h">📦 {{ 'cj.jobs' | t }}</h3>
        <select id="code-jobs-status" [value]="status()" (change)="setStatus(val($event))" [attr.aria-label]="'cj.status' | t">
          <option value="">{{ 'cj.allStatus' | t }}</option>
          @for (s of statuses; track s) { <option [value]="s">{{ statusLabel(s) }}</option> }
        </select>
        <input id="code-jobs-name" type="search" [value]="name()" (change)="setName(val($event))" (keydown.enter)="setName(val($event))" [placeholder]="'cj.namePh' | t" />
        <label class="check"><input type="checkbox" id="code-jobs-auto" [checked]="auto()" (change)="setAuto(chk($event))" /> {{ 'cj.auto' | t }}</label>
        <button class="btn btn-sm" id="code-jobs-refresh" (click)="refresh()">↻ {{ 'cj.refresh' | t }}</button>
        <span class="grow"></span>
        <span class="muted small">{{ total() }}</span>
      </div>
      <div class="table-scroll">
        <table class="table" id="code-jobs">
          <thead><tr>
            <th class="num">{{ 'cj.col.id' | t }}</th><th>{{ 'cj.col.name' | t }}</th><th>{{ 'cj.status' | t }}</th><th class="num">{{ 'cj.col.attempts' | t }}</th>
            <th>{{ 'cj.col.progress' | t }}</th><th>{{ 'cj.col.runAt' | t }}</th><th>{{ 'cj.col.finished' | t }}</th><th class="th-actions"></th>
          </tr></thead>
          <tbody>
            @for (j of items(); track j.id) {
              @let pr = progress(j);
              <tr class="code-job-row" [attr.data-id]="j.id" [attr.data-status]="j.status" [class.open]="open() === j.id" (click)="toggle(j)">
                <td class="num mono small">{{ j.id }}</td>
                <td class="mono"><b>{{ j.name }}</b>@if (j.error && open() !== j.id) { <span class="muted small"> · {{ 'cj.details' | t }} ▸</span> }</td>
                <td><span class="cj-status code-job-status" [attr.data-status]="j.status">{{ statusLabel(j.status) }}</span></td>
                <td class="num small code-job-attempts">{{ j.attempts }}/{{ j.maxAttempts }}</td>
                <td class="code-job-progress">
                  @if (pr.pct !== null) { <span class="cj-bar" role="progressbar" [attr.aria-valuenow]="pr.pct" aria-valuemin="0" aria-valuemax="100"><i [style.width.%]="pr.pct"></i></span> }
                  @if (pr.text) { <span class="small">{{ pr.text }}</span> }
                </td>
                <td class="small">{{ when(j.startedAt || j.runAt) }}</td>
                <td class="small">{{ when(j.finishedAt) }}</td>
                <td class="row-actions" (click)="$event.stopPropagation()">
                  @if (canAct() && cancelable(j)) { <button class="btn btn-sm code-job-cancel" [disabled]="busy() === 'job:' + j.id" (click)="cancel(j)">✕ {{ 'cj.cancel' | t }}</button> }
                  @if (canAct() && retryable(j)) { <button class="btn btn-sm code-job-retry" [disabled]="busy() === 'job:' + j.id" (click)="retry(j)">↻ {{ 'cj.retry' | t }}</button> }
                </td>
              </tr>
              @if (open() === j.id) {
                <tr class="code-job-details-row"><td colspan="8">
                  <div class="code-job-details" [attr.data-id]="j.id">
                    <div class="cj-meta small muted">
                      @if (j.createdAt) { <span>{{ 'cj.created' | t }}: {{ when(j.createdAt) }}</span> }
                      @if (j.startedAt) { <span>{{ 'cj.started' | t }}: {{ when(j.startedAt) }}</span> }
                      @if (duration(j); as d) { <span>{{ 'cj.duration' | t }}: {{ d }}</span> }
                      @if (j.dedupeKey) { <span>{{ 'cj.dedupe' | t }}: <code>{{ j.dedupeKey }}</code></span> }
                    </div>
                    @if (j.error) { <div class="small"><b>{{ 'cj.error' | t }}</b></div><pre class="code-job-error mono">{{ j.error }}</pre> }
                    @if (pretty(j.result); as r) { <div class="small"><b>{{ 'cj.result' | t }}</b></div><pre class="code-job-result mono">{{ r }}</pre> }
                    @if (pretty(j.data); as d) { <div class="small"><b>{{ 'cj.data' | t }}</b></div><pre class="code-job-data mono">{{ d }}</pre> }
                  </div>
                </td></tr>
              }
            } @empty {
              <tr><td colspan="8" class="empty-state" id="code-jobs-empty">{{ loading() ? ('common.loading' | t) : ('cj.empty' | t) }}</td></tr>
            }
          </tbody>
        </table>
      </div>
      <rea-pager [page]="page()" [total]="total()" [perPage]="perPage" (pageChange)="setPage($event)" />
    </section>
  `,
  styles: `
    .cj-card { margin-bottom: 16px; }
    .cj-h { margin: 0; font-size: 15px; }
    .cj-toolbar { select, input { width: auto; height: 34px; } label.check { margin: 0; } }
    .code-job-row { cursor: pointer; }
    .code-job-row[data-status='failed'] td { background: #fffafa; }
    .cj-status { display: inline-block; padding: 1px 8px; border-radius: 999px; font-size: 11.5px; font-weight: 700; background: #f1f2f6; color: #4b5164; }
    .cj-status[data-status='running'] { background: #dbeafe; color: #1e40af; }
    .cj-status[data-status='done'], .cj-status[data-status='ok'], .cj-status[data-status='success'] { background: #dcfce7; color: #166534; }
    .cj-status[data-status='failed'], .cj-status[data-status='error'] { background: #fee2e2; color: #991b1b; }
    .cj-status[data-status='cancelled'] { background: #f3f4f6; color: #6b7280; text-decoration: line-through; }
    .code-job-progress { min-width: 120px; }
    .cj-bar { display: inline-block; width: 90px; height: 8px; border-radius: 4px; background: #eef0f5; overflow: hidden; vertical-align: middle; margin-right: 6px;
      i { display: block; height: 100%; background: var(--primary); } }
    .code-job-details { display: flex; flex-direction: column; gap: 6px; padding: 4px 2px; }
    .cj-meta { display: flex; flex-wrap: wrap; gap: 4px 16px; }
    .code-job-details pre { margin: 0; padding: 8px; max-height: 260px; overflow: auto; background: #f6f7fb; border: 1px solid var(--border); border-radius: 6px; white-space: pre-wrap; word-break: break-word; font-size: 12px; }
    .code-job-error { color: #991b1b; }
  `,
})
export class CodeJobs implements OnInit, OnDestroy {
  private api = inject(Api);
  private auth = inject(Auth);
  private i18n = inject(I18n);
  private units = inject(CodeUnits);
  private route = inject(ActivatedRoute);

  readonly statuses = JOB_STATUSES;
  readonly perPage = JOBS_PER_PAGE;
  readonly canAct = computed(() => this.auth.can('publish_scripts'));
  readonly loading = signal(true);
  readonly loadingCron = signal(true);
  readonly error = signal('');
  readonly items = signal<CodeJob[]>([]);
  readonly total = signal(0);
  readonly cron = signal<CodeCron[]>([]);
  readonly page = signal(1);
  readonly status = signal('');
  readonly name = signal('');
  readonly auto = signal(false);
  readonly open = signal<number | null>(null);
  readonly busy = signal('');
  private timer: ReturnType<typeof setInterval> | null = null;
  private seq = 0;

  ngOnInit() { this.units.adopt(this.route); void this.refresh(); }
  onUnit() { this.page.set(1); this.open.set(null); void this.refresh(); }
  ngOnDestroy() { this.stopTimer(); }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  chk(e: Event) { return (e.target as HTMLInputElement).checked; }
  statusLabel(s: string) { const k = 'cj.st.' + s; const t = this.i18n.t(k); return t === k ? s : t; }
  progress(j: CodeJob) { return jobProgress(j.progress); }
  cancelable(j: CodeJob) { return canCancel(j); }
  retryable(j: CodeJob) { return canRetry(j); }
  duration(j: CodeJob) { return formatDuration(j.startedAt, j.finishedAt); }
  pretty(v: unknown) { return prettyValue(v); }
  when(iso?: string | null) {
    if (!iso) return '—';
    const d = new Date(iso);
    return isNaN(+d) ? iso : d.toLocaleString(this.i18n.intl(), { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit', second: '2-digit' });
  }

  setStatus(v: string) { this.status.set(v); this.page.set(1); void this.loadJobs(); }
  setName(v: string) { if (v.trim() === this.name()) return; this.name.set(v.trim()); this.page.set(1); void this.loadJobs(); }
  setPage(p: number) { this.page.set(p); void this.loadJobs(); }
  toggle(j: CodeJob) { this.open.set(this.open() === j.id ? null : j.id); }

  setAuto(on: boolean) {
    this.auto.set(on);
    this.stopTimer();
    if (on) this.timer = setInterval(() => void this.refresh(true), AUTO_MS);
  }
  private stopTimer() { if (this.timer) { clearInterval(this.timer); this.timer = null; } }

  async refresh(quiet = false) { await Promise.all([this.loadJobs(quiet), this.loadCron(quiet)]); }

  async loadJobs(quiet = false) {
    const my = ++this.seq;
    if (!quiet) this.loading.set(true);
    try {
      const r = await this.api.codeJobs({ ...jobsQuery(this.status(), this.name(), this.page()), unit: this.units.param() });
      if (my !== this.seq) return; // a newer request answered
      this.items.set(r?.items ?? []);
      this.total.set(r?.total ?? r?.items?.length ?? 0);
      if (!quiet) this.error.set('');
    } catch (e) {
      if (my === this.seq) this.error.set(errorMessage(e));
    } finally {
      if (my === this.seq) this.loading.set(false);
    }
  }

  async loadCron(quiet = false) {
    if (!quiet) this.loadingCron.set(true);
    try {
      const r = await this.api.codeCron(this.units.param());
      this.cron.set(Array.isArray(r) ? r : ((r as { items?: CodeCron[] } | null)?.items ?? []));
    } catch (e) {
      if (!quiet) this.error.set(errorMessage(e));
    } finally {
      this.loadingCron.set(false);
    }
  }

  async cancel(j: CodeJob) {
    if (!(await Dialog.confirm({ title: this.i18n.t('cj.cancelConfirm', { id: j.id, name: j.name }), text: this.i18n.t('cj.cancelText'), danger: true }))) return;
    await this.act('job:' + j.id, () => this.api.cancelCodeJob(j.id), this.i18n.t('cj.cancelled', { id: j.id }));
  }

  async retry(j: CodeJob) { await this.act('job:' + j.id, () => this.api.retryCodeJob(j.id), this.i18n.t('cj.retried', { id: j.id })); }

  async runCron(c: CodeCron) {
    if (!(await Dialog.confirm({ title: this.i18n.t('cj.cron.runConfirm', { name: c.name }) }))) return;
    await this.act('cron:' + c.name, () => this.api.runCodeCron(c.name, this.units.param()), this.i18n.t('cj.cron.started', { name: c.name }));
  }

  private async act(key: string, fn: () => Promise<unknown>, done: string) {
    this.busy.set(key);
    this.error.set('');
    try {
      await fn();
      Dialog.toast(done);
      await this.refresh(true);
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.busy.set('');
    }
  }
}
