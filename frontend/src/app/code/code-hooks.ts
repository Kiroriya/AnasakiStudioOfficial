import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { CodeHook, CodeHookSubscriber } from '../core/code-models';
import { TPipe, tr } from '../core/i18n';
import { CodeTabs } from './code-tabs';
import { groupHooks, hookUnits, normalizeHooks, orderedSubscribers, subscriberWhere } from './hooks-utils';
import './messages-code4b';

/**
 * /code/hooks (view_scripts) — every filter / event hook (core and the units'
 * own) with its subscribers in the order they run (GET /code/hooks).
 */
@Component({
  selector: 'rea-code-hooks',
  imports: [TPipe, CodeTabs],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <header class="page-head">
      <span class="page-icon">🪝</span>
      <div class="page-head-text"><h1>{{ 'ch.title' | t }}</h1><p>{{ 'ch.desc' | t }}</p></div>
    </header>
    <rea-code-tabs />
    @if (error()) { <p class="alert" id="code-hooks-error">{{ error() }}</p> }

    <section class="card table-card">
      <div class="table-toolbar ch-toolbar">
        <input id="code-hooks-q" type="search" [value]="q()" (input)="q.set($any($event.target).value)" [placeholder]="'ch.search' | t" />
        <select id="code-hooks-unit" [value]="unit()" (change)="unit.set($any($event.target).value)" [attr.aria-label]="'cu.label' | t">
          <option value="">{{ 'ch.allUnits' | t }}</option>
          @for (u of units(); track u) { <option [value]="u" [selected]="unit() === u">{{ u === 'site' ? ('cu.site' | t) : u }}</option> }
        </select>
        <button class="btn btn-sm" id="code-hooks-reload" (click)="load()">↻ {{ 'ch.reload' | t }}</button>
        <span class="grow"></span>
        <span class="muted small">{{ 'ch.orderHint' | t }}</span>
      </div>
      <div id="code-hooks">
        @for (g of groups(); track g.kind) {
          <div class="ch-group" [attr.data-kind]="g.kind">
            <h3 class="ch-group-head">{{ (g.kind === 'filter' ? 'ch.filters' : g.kind === 'event' ? 'ch.events' : 'ch.other') | t }} <span class="muted small">({{ g.hooks.length }})</span></h3>
            <table class="table code-hooks-table">
              <thead><tr><th>{{ 'ch.col.name' | t }}</th><th>{{ 'ch.declaredBy' | t }}</th><th>{{ 'ch.col.subs' | t }}</th></tr></thead>
              <tbody>
                @for (h of g.hooks; track h.name) {
                  <tr class="code-hook-row" [attr.data-name]="h.name" [attr.data-kind]="h.kind" [attr.data-declared-by]="h.declaredBy">
                    <td class="mono"><b>{{ h.name }}</b></td>
                    <td><span class="badge code-hook-owner" [class.badge-info]="h.declaredBy !== 'core'">{{ owner(h.declaredBy) }}</span></td>
                    <td>
                      @if (h.subscribers.length) {
                        <ol class="ch-subs">
                          @for (s of subs(h); track $index) {
                            <li class="code-hook-sub" [attr.data-unit]="s.unit" [attr.data-priority]="s.priority">
                              <span class="badge">{{ owner(s.unit) }}</span>
                              <span class="small" [title]="'ch.priority' | t">⚖ {{ s.priority }}</span>
                              @if (where(s); as w) { <span class="mono small muted code-hook-where">{{ w }}</span> }
                            </li>
                          }
                        </ol>
                      } @else { <span class="muted small">{{ 'ch.noSubscribers' | t }}</span> }
                    </td>
                  </tr>
                }
              </tbody>
            </table>
          </div>
        } @empty {
          <p class="empty-state" id="code-hooks-empty">{{ loading() ? ('common.loading' | t) : ('ch.empty' | t) }}</p>
        }
      </div>
    </section>
  `,
  styles: `
    .ch-toolbar { input, select { width: auto; height: 34px; } }
    .ch-group { padding: 4px 0 12px; }
    .ch-group-head { margin: 8px 16px; font-size: 14px; }
    .ch-subs { margin: 0; padding-left: 18px; display: flex; flex-direction: column; gap: 3px; }
    .code-hook-sub { display: list-item; > * { margin-right: 6px; } }
  `,
})
export class CodeHooks implements OnInit {
  private api = inject(Api);
  readonly hooks = signal<CodeHook[]>([]);
  readonly loading = signal(true);
  readonly error = signal('');
  readonly q = signal('');
  readonly unit = signal('');
  readonly units = computed(() => hookUnits(this.hooks()));
  readonly groups = computed(() => groupHooks(this.hooks(), this.q(), this.unit()));

  ngOnInit() { void this.load(); }

  async load() {
    this.loading.set(true);
    this.error.set('');
    try {
      this.hooks.set(normalizeHooks(await this.api.codeHooks()));
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.loading.set(false);
    }
  }

  subs(h: CodeHook) { return orderedSubscribers(h.subscribers); }
  where(s: CodeHookSubscriber) { return subscriberWhere(s); }
  owner(u: string) { return u === 'core' ? '⚙️ ' + tr('ch.core') : u === 'site' ? '⌨️ ' + tr('cu.site') : '🧩 ' + u; }
}
