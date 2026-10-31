import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { CodeFormatter } from '../core/code-models';
import { Dialog } from '../core/dialog';
import { TPipe, tr } from '../core/i18n';
import { CodeTabs } from './code-tabs';
import {
  FormatterAliases, aliasConflict, aliasEnabled, aliasesOf, groupFormatters, isPluginFormatter, normalizeFormatters, shortFormatterName, toggleAlias,
} from './formatter-utils';
import './messages-code6';

/**
 * /code/formatters (view_scripts; switching needs manage_scripts) — formatters
 * of the site code and plugins (GET /code/formatters) with a switch per plugin
 * formatter for its unprefixed alias (PUT /code/formatters/aliases).
 */
@Component({
  selector: 'rea-code-formatters',
  imports: [TPipe, CodeTabs],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <header class="page-head">
      <span class="page-icon">🧮</span>
      <div class="page-head-text"><h1>{{ 'cf.title' | t }}</h1><p>{{ 'cf.desc' | t }}</p></div>
    </header>
    <rea-code-tabs />
    @if (error()) { <p class="alert" id="code-formatters-error">{{ error() }}</p> }

    <section class="card table-card">
      <div class="table-toolbar cf-toolbar">
        <input id="code-formatters-q" type="search" [value]="q()" (input)="q.set($any($event.target).value)" [placeholder]="'cf.search' | t" />
        <button class="btn btn-sm" id="code-formatters-reload" (click)="load()">↻ {{ 'cf.reload' | t }}</button>
        <span class="grow"></span>
        @if (!canManage()) { <span class="muted small" id="code-formatters-readonly">{{ 'cf.readOnly' | t }}</span> }
      </div>
      <div id="code-formatters">
        @for (g of groups(); track g.unit) {
          <div class="cf-group" [attr.data-unit]="g.unit">
            <h3 class="cf-group-head">{{ g.unit === 'site' ? ('cf.unit.site' | t) : ('cf.unit.plugin' | t: { id: g.unit }) }} <span class="muted small">({{ g.formatters.length }})</span></h3>
            <table class="table code-formatters-table">
              <thead><tr><th>{{ 'cf.col.name' | t }}</th><th>{{ 'cf.col.usage' | t }}</th><th>{{ 'cf.col.alias' | t }}</th></tr></thead>
              <tbody>
                @for (f of g.formatters; track f.name) {
                  @let plugin = isPlugin(f);
                  @let on = enabled(f);
                  @let why = conflictText(f);
                  <tr class="code-formatter-row" [attr.data-name]="f.name" [attr.data-unit]="f.unit" [attr.data-alias]="on ? short(f) : null" [class.cf-conflict]="!!why">
                    <td class="mono"><b>{{ f.name }}</b></td>
                    <td class="mono small muted">{{ usage(f.name) }}@if (plugin && on && !why) { <br />{{ usage(short(f)) }} }</td>
                    <td>
                      @if (plugin) {
                        <label class="check cf-alias">
                          <input type="checkbox" class="code-formatter-alias" [attr.data-name]="f.name" [checked]="on"
                                 [disabled]="!canManage() || busy() || (!on && !!blocked(f))" (change)="toggle(f, $any($event.target).checked)" />
                          {{ 'cf.aliasOn' | t: { alias: short(f) } }}
                        </label>
                        @if (why) { <div class="small cf-conflict-msg code-formatter-conflict" [attr.data-name]="f.name">⚠ {{ why }}</div> }
                      } @else {
                        <span class="muted small">{{ 'cf.siteNoAlias' | t }}</span>
                      }
                    </td>
                  </tr>
                }
              </tbody>
            </table>
          </div>
        } @empty {
          <p class="empty-state" id="code-formatters-empty">{{ loading() ? ('common.loading' | t) : ('cf.empty' | t) }}</p>
        }
      </div>
      <p class="hint cf-hint">{{ 'cf.hint' | t }}</p>
    </section>
  `,
  styles: `
    .cf-toolbar { input { width: auto; height: 34px; } }
    .cf-group { padding: 4px 0 12px; }
    .cf-group-head { margin: 8px 16px; font-size: 14px; }
    .cf-alias { margin: 0; font-weight: 500; }
    .cf-conflict-msg { color: var(--warn); margin-top: 4px; }
    .cf-hint { padding: 0 16px 12px; }
  `,
})
export class CodeFormatters implements OnInit {
  private api = inject(Api);
  private auth = inject(Auth);
  readonly list = signal<CodeFormatter[]>([]);
  readonly aliases = signal<FormatterAliases>({});
  readonly loading = signal(true);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly q = signal('');
  readonly canManage = computed(() => this.auth.can('manage_scripts'));
  readonly groups = computed(() => {
    const s = this.q().trim().toLowerCase();
    return groupFormatters(s ? this.list().filter(f => f.name.toLowerCase().includes(s)) : this.list());
  });

  ngOnInit() { void this.load(); }

  async load() {
    this.loading.set(true);
    this.error.set('');
    try {
      const list = normalizeFormatters(await this.api.codeFormatters());
      this.list.set(list);
      this.aliases.set(aliasesOf(list));
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.loading.set(false);
    }
  }

  isPlugin(f: CodeFormatter) { return isPluginFormatter(f); }
  short(f: CodeFormatter) { return shortFormatterName(f.name); }
  usage(name: string) { return `{{ value | ${name} }}`; }
  enabled(f: CodeFormatter) { return aliasEnabled(f, this.aliases()); }
  blocked(f: CodeFormatter) { return aliasConflict(f, this.list(), this.aliases()); }

  /** Server text first (it knows the effective state), else the local rule. */
  conflictText(f: CodeFormatter): string {
    if (!isPluginFormatter(f)) return '';
    if (f.conflict) return tr('cf.conflict.server', { why: f.conflict });
    const c = this.blocked(f);
    if (!c) return '';
    return c.kind === 'plugin' ? tr('cf.conflict.plugin', { by: c.by }) : tr('cf.conflict.' + c.kind, { name: c.by });
  }

  async toggle(f: CodeFormatter, on: boolean) {
    const before = this.aliases();
    const next = toggleAlias(before, f, on, this.list());
    if (next === before) { this.aliases.set({ ...before }); return; } // refused: re-render the checkbox
    this.aliases.set(next);
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.saveFormatterAliases(next);
      Dialog.toast(tr('cf.saved'), 'success');
      const list = normalizeFormatters(await this.api.codeFormatters().catch(() => null));
      if (list.length) {
        const a = aliasesOf(list);
        this.list.set(list);
        this.aliases.set(Object.keys(a).length || !Object.keys(next).length ? a : next); // a server without `alias` rows keeps what we sent
      }
    } catch (e) {
      this.aliases.set(before);
      this.error.set(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
