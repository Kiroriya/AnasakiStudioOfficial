import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { CodeDbTableDetail } from '../core/code-models';
import { TPipe } from '../core/i18n';
import { mapLimit } from '../shared/concurrency';
import { DbNav } from './db-nav';
import { ER, ErLinkIn, erLayout } from './db-utils';
import './messages-db';

/** /code/db/er — boxes for the developer tables, lines for foreign keys; referenced core_* views greyed. */
@Component({
  selector: 'rea-db-er',
  imports: [TPipe, DbNav],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <rea-db-nav />
    @if (error()) { <p class="alert" id="db-er-error">{{ error() }}</p> }
    <section class="card db-er-card">
      <div class="db-er-legend small muted">
        <span><svg width="28" height="8"><line x1="0" y1="4" x2="28" y2="4" class="db-er-edge" /></svg> {{ 'db.er.fk' | t }}</span>
        <span><svg width="28" height="8"><line x1="0" y1="4" x2="28" y2="4" class="db-er-edge meta" /></svg> {{ 'db.er.meta' | t }}</span>
        <span><span class="db-er-swatch core"></span> {{ 'db.er.core' | t }}</span>
        <span class="grow"></span>
        <span>{{ 'db.er.hint' | t }}</span>
      </div>
      @if (layout().nodes.length) {
        <div class="db-er-scroll">
          <svg class="db-er" id="db-er" [attr.width]="layout().width" [attr.height]="layout().height" [attr.viewBox]="'0 0 ' + layout().width + ' ' + layout().height" role="img" [attr.aria-label]="'db.nav.er' | t">
            <defs>
              <marker id="db-er-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" class="db-er-arrow" /></marker>
            </defs>
            @for (e of layout().edges; track e.from + '.' + e.fromColumn + '>' + e.to) {
              <path class="db-er-edge" [class.meta]="e.meta" [attr.d]="e.d" [attr.data-from]="e.from" [attr.data-to]="e.to" [attr.data-column]="e.fromColumn" marker-end="url(#db-er-arrow)">
                <title>{{ e.from }}.{{ e.fromColumn }} → {{ e.to }}.{{ e.toColumn }}</title>
              </path>
            }
            @for (n of layout().nodes; track n.name) {
              <g class="db-er-node" [class.core]="n.core" [attr.data-table]="n.name" [attr.transform]="'translate(' + n.x + ',' + n.y + ')'" (click)="open(n.name, n.core)">
                <rect class="db-er-box" [attr.width]="n.w" [attr.height]="n.h" rx="8" />
                <rect class="db-er-head" [attr.width]="n.w" [attr.height]="er.head" rx="8" />
                <rect class="db-er-head" [attr.y]="er.head - 8" [attr.width]="n.w" height="8" />
                <text class="db-er-title" x="10" [attr.y]="er.head / 2 + 5">{{ n.name }}</text>
                @for (c of n.columns; track c.name; let i = $index) {
                  <text class="db-er-col" [class.pk]="c.pk" [class.fk]="c.fk" x="10" [attr.y]="er.head + er.row * i + er.row / 2 + 4">{{ c.pk ? '🔑 ' : c.fk ? '↗ ' : '' }}{{ c.name }}</text>
                  <text class="db-er-type" [attr.x]="n.w - 10" [attr.y]="er.head + er.row * i + er.row / 2 + 4" text-anchor="end">{{ short(c.type) }}</text>
                }
                @if (n.more) { <text class="db-er-more" x="10" [attr.y]="er.head + er.row * n.columns.length + er.row / 2 + 4">{{ 'db.er.more' | t: { n: n.more } }}</text> }
              </g>
            }
          </svg>
        </div>
      } @else {
        <p class="empty-state" id="db-er-empty">{{ loading() ? ('common.loading' | t) : ('db.er.empty' | t) }}</p>
      }
    </section>
  `,
})
export class DbEr implements OnInit {
  private api = inject(Api);
  private router = inject(Router);

  readonly er = ER;
  readonly loading = signal(true);
  readonly error = signal('');
  readonly details = signal<CodeDbTableDetail[]>([]);
  readonly layout = computed(() => {
    const ds = this.details();
    const links: ErLinkIn[] = [];
    for (const d of ds) {
      for (const f of d.foreignKeys ?? []) links.push({ from: d.name, fromColumn: f.column, to: f.refTable, toColumn: f.refColumn });
      for (const f of d.relations ?? []) links.push({ from: d.name, fromColumn: f.column, to: f.refTable, toColumn: f.refColumn, meta: true });
    }
    return erLayout(ds.map(d => ({ name: d.name, columns: d.columns ?? [] })), links);
  });

  async ngOnInit() {
    try {
      // developer tables only (views and system tables appear when referenced)
      const tables = (await this.api.codeDbTables()).filter(t => !t.view && !t.system);
      const res = await mapLimit(tables, 6, t => this.api.codeDbTable(t.name));
      const ok: CodeDbTableDetail[] = [];
      res.forEach(r => { if (r.status === 'fulfilled') ok.push(r.value); });
      const failed = res.find(r => r.status === 'rejected');
      if (failed && failed.status === 'rejected') this.error.set(errorMessage(failed.reason));
      this.details.set(ok);
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.loading.set(false);
    }
  }

  /** `bigint unsigned` → `bigint`, `varchar(255)` → `varchar(255)`. */
  short(type: string) { return (type || '').split(' ')[0].slice(0, 16); }

  open(name: string, core: boolean) {
    void this.router.navigate(['/code/db/tables', name], core ? { queryParams: { view: 'structure' } } : {});
  }
}
