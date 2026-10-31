import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { CodeAccessRow } from '../core/code-models';
import { I18n, TPipe, localized } from '../core/i18n';
import { BUILTIN_ROLE_NAMES, roleName } from '../core/roles';
import { RoleInfo } from '../core/models';
import { ActivatedRoute } from '@angular/router';
import { CodeTabs } from './code-tabs';
import { CodeUnits, UnitSwitcher } from './code-units';
import { filterByUnit, shortName } from './units-utils';
import {
  ADMIN, EVERYONE, applyOverride, cellState, defaultRoles, groupAccess, groupColumns, groupName, isGroupEntry, isOverridden, overrideFor, roleColumns, toggleRole,
} from './access-utils';
import './messages-access';
import './messages-code4';
import './messages-code6';

/**
 * /code/access — who may use each route / collection / action / page / table / admin page
 * of the site code (design §7.2). Defaults come from `access` in the code;
 * admins tick over them (script_access overrides) and can reset to the default.
 */
@Component({
  selector: 'rea-code-access',
  imports: [TPipe, CodeTabs, UnitSwitcher],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <header class="page-head">
      <span class="page-icon">🔐</span>
      <div class="page-head-text"><h1>{{ 'ca.title' | t }}</h1><p>{{ 'ca.desc' | t }}</p></div>
    </header>
    <rea-code-tabs />
    <rea-unit-switcher (changed)="load()" />
    @if (error()) { <p class="alert" id="code-access-error">{{ error() }}</p> }

    <section class="card table-card">
      <div class="table-toolbar">
        <input id="code-access-q" type="search" [value]="q()" (input)="q.set(val($event))" [placeholder]="'ca.search' | t" />
        <button class="btn btn-sm" id="code-access-reload" (click)="load()">↻ {{ 'ca.reload' | t }}</button>
        <span class="grow"></span>
        <span class="muted small">{{ 'ca.adminAlways' | t }}</span>
      </div>
      <div class="table-scroll">
        <table class="table code-access-table" id="code-access">
          <thead>
            <tr>
              <th>{{ 'ca.col.name' | t }}</th>
              @for (c of columns(); track c) {
                @if (isGroup(c)) {
                  <th class="ca-col ca-col-group center" [attr.data-role]="c" [attr.data-group]="group(c)" [title]="'ca.groupTip' | t: { name: group(c) }">👥 {{ 'ca.col.group' | t: { name: group(c) } }}</th>
                } @else {
                  <th class="ca-col center" [attr.data-role]="c">{{ c === everyone ? ('ca.col.everyone' | t) : roleLabel(c) }}</th>
                }
              }
              <th></th>
            </tr>
          </thead>
          @for (g of groups(); track g.kind) {
            <tbody class="ca-group" [attr.data-kind]="g.kind">
              <tr class="ca-group-head"><th [attr.colspan]="columns().length + 2">{{ ('ca.kind.' + g.kind) | t }} <span class="muted small">({{ g.rows.length }})</span></th></tr>
              @for (r of g.rows; track r.kind + ':' + r.name) {
                <tr class="code-access-row" [attr.data-kind]="r.kind" [attr.data-name]="r.name" [class.overridden]="overridden(r)">
                  <td class="ca-name">
                    <b class="mono" [title]="r.name">{{ short(r.name) }}</b>
                    @if (label(r); as l) { <small class="muted"> · {{ l }}</small> }
                    @if (overridden(r)) { <span class="badge badge-warn ca-badge">{{ 'ca.overridden' | t }}</span> }
                    <div class="muted small ca-default">{{ 'ca.codeDefault' | t }}: {{ defaultText(r) }}</div>
                  </td>
                  @for (c of columns(); track c) {
                    @let st = cell(r, c);
                    <td class="center">
                      <input type="checkbox" class="ca-check" [attr.data-role]="c" [checked]="st.checked" [disabled]="st.locked || busy() === key(r)"
                             [title]="c === admin ? ('ca.adminAlways' | t) : st.implied ? ('ca.implied' | t) : c === everyone && r.kind === 'adminPage' ? ('ca.adminPageNoEveryone' | t) : ''" (change)="toggle(r, c, chk($event))" />
                    </td>
                  }
                  <td class="right">
                    @if (overridden(r)) {
                      <button class="btn btn-sm ca-reset" [disabled]="busy() === key(r)" (click)="reset(r)">↺ {{ 'ca.reset' | t }}</button>
                    }
                  </td>
                </tr>
              }
            </tbody>
          } @empty {
            <tbody><tr><td [attr.colspan]="columns().length + 2" class="empty-state" id="code-access-empty">{{ loading() ? ('common.loading' | t) : ('ca.empty' | t) }}</td></tr></tbody>
          }
        </table>
      </div>
      <p class="hint ca-hint">{{ 'ca.hint' | t }}</p>
      @if (groupCols().length) { <p class="hint ca-groups-hint" id="code-access-groups-hint">{{ 'ca.groupsHint' | t }}</p> }
    </section>
  `,
})
export class CodeAccess implements OnInit {
  private api = inject(Api);
  private i18n = inject(I18n);
  private units = inject(CodeUnits);
  private route = inject(ActivatedRoute);
  readonly everyone = EVERYONE;
  readonly admin = ADMIN;
  readonly rows = signal<CodeAccessRow[]>([]);
  readonly roles = signal<RoleInfo[]>([]);
  readonly loading = signal(true);
  readonly error = signal('');
  readonly q = signal('');
  readonly busy = signal('');

  readonly groups = computed(() => groupAccess(this.rows(), this.q()));
  /** Declared site.visitorGroups() (from /code/status or /code/bindings). */
  readonly visitorGroups = signal<string[]>([]);
  readonly groupCols = computed(() => groupColumns(this.visitorGroups(), this.rows()));
  readonly columns = computed(() => [
    ...roleColumns(this.roles().length ? this.roles() : Object.keys(BUILTIN_ROLE_NAMES).map(slug => ({ slug })), this.rows()),
    ...this.groupCols(),
  ]);

  ngOnInit() { this.units.adopt(this.route); void this.load(); }

  /** Rows of the selected unit (the server filters by ?unit=; older servers send everything). */
  private mine(rows: CodeAccessRow[] | null | undefined) { return filterByUnit(rows ?? [], this.units.param(), this.units.pluginIds()); }
  short(name: string) { return shortName(name, this.units.param()); }

  async load() {
    this.loading.set(true);
    this.error.set('');
    try {
      const [rows, roles, status, bindings] = await Promise.all([
        this.units.load().then(() => this.api.codeAccess(this.units.param())), this.api.roles().catch(() => null), this.api.codeStatus().catch(() => null), this.api.codeBindings().catch(() => null),
      ]);
      this.rows.set(this.mine(rows));
      if (roles?.roles) this.roles.set(roles.roles);
      this.visitorGroups.set([...new Set([...(status?.registered?.visitorGroups ?? []), ...(bindings?.visitorGroups ?? [])])]);
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.loading.set(false);
    }
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  chk(e: Event) { return (e.target as HTMLInputElement).checked; }
  key(r: CodeAccessRow) { return r.kind + ':' + r.name; }
  overridden(r: CodeAccessRow) { return isOverridden(r); }
  cell(r: CodeAccessRow, role: string) { return cellState(r, role); }
  label(r: CodeAccessRow) { const l = localized(r.label); return l && l !== r.name ? l : ''; }
  roleLabel(slug: string) { return roleName(slug, this.roles()); }
  isGroup(c: string) { return isGroupEntry(c); }
  group(c: string) { return groupName(c); }
  defaultText(r: CodeAccessRow) {
    const d = defaultRoles(r.kind, r.codeDefault);
    if (!d.length) return this.i18n.t('ca.nobody');
    return d.map(x => (x === EVERYONE ? this.i18n.t('ca.col.everyone') : isGroupEntry(x) ? this.i18n.t('ca.col.group', { name: groupName(x) }) : this.roleLabel(x))).join(', ');
  }

  toggle(r: CodeAccessRow, role: string, on: boolean) {
    void this.save(r, overrideFor(r, toggleRole(r.effective ?? [], role, on)));
  }

  reset(r: CodeAccessRow) { void this.save(r, null); }

  private async save(r: CodeAccessRow, override: string[] | null) {
    const k = this.key(r);
    this.busy.set(k);
    this.error.set('');
    this.rows.update(list => list.map(x => (this.key(x) === k ? applyOverride(x, override) : x)));
    try {
      await this.api.setCodeAccess(r.kind, r.name, override, r.unit ?? this.units.param());
      const fresh = await this.api.codeAccess(this.units.param()).catch(() => null);
      if (fresh) this.rows.set(this.mine(fresh));
    } catch (e) {
      this.error.set(errorMessage(e));
      this.rows.update(list => list.map(x => (this.key(x) === k ? r : x))); // back to what the server has
    } finally {
      this.busy.set('');
    }
  }
}
