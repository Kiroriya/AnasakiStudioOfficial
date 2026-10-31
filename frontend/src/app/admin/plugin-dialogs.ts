import { ChangeDetectionStrategy, Component, computed, input, output, signal } from '@angular/core';
import { TPipe } from '../core/i18n';
import { ConsentState, DepStatus, consentCanConfirm, destructiveText, isSensitiveScope, scopeLabel } from './plugin-deps';
import './messages-plugins4b';

const STYLES = `
  .pjs-sec { margin: 10px 0; h4 { margin: 0 0 6px; font-size: 13px; } }
  .pjs-list { margin: 0; padding-left: 0; list-style: none; display: flex; flex-direction: column; gap: 4px; }
  .pjs-list li { display: flex; gap: 8px; align-items: baseline; padding: 6px 8px; border-radius: 6px; background: #f6f7fb; font-size: 13px; }
  .pjs-list li.sensitive { background: #fff7ed; border: 1px solid #fed7aa; }
  .pjs-scope-id { font-size: 11px; color: var(--muted); }
  .pjs-sens { font-size: 11px; color: #b45309; font-weight: 600; }
  .pjs-destructive { border: 1px solid #fecaca; background: #fef2f2; border-radius: 8px; padding: 10px 12px; margin: 10px 0;
    ul { margin: 6px 0; padding-left: 18px; } li { font-size: 12.5px; } label.check { margin-top: 6px; } }
  .pjs-dep-state { margin-left: auto; font-size: 12px; white-space: nowrap; }
  .pjs-dep-state[data-state='missing'], .pjs-dep-state[data-state='incompatible'] { color: #b91c1c; }
`;

/** Install / upgrade / re-consent dialog: permissions, code scopes (th/en), hosts, routes and destructive migrations. */
@Component({
  selector: 'rea-plugin-consent',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  styles: STYLES,
  template: `
    @let c = state();
    <div class="modal-backdrop" (click)="decide.emit(false)">
      <div class="modal" id="plugin-consent-dialog" role="dialog" aria-modal="true" [attr.data-mode]="c.mode" (click)="$event.stopPropagation()">
        <div class="modal-head">
          <h3 id="plugin-consent-title">{{ 'pjs.consent.' + c.mode | t: { name: c.name } }}@if (c.version) { <span class="muted small"> v{{ c.version }}</span> }</h3>
          @if (c.runtime === 'js') { <span class="badge badge-info plugin-runtime-badge" data-runtime="js">JS</span> }
          <button class="icon-btn" (click)="decide.emit(false)" [attr.aria-label]="'common.cancel' | t">✕</button>
        </div>
        <p class="small">{{ (c.mode === 'reconsent' ? 'pjs.consent.introRe' : 'pjs.consent.intro') | t }}</p>
        @if (c.permissions.length) {
          <div class="pjs-sec" id="plugin-consent-perms"><h4>🔐 {{ 'pjs.consent.perms' | t }}</h4>
            <ul class="pjs-list">@for (p of c.permissions; track p) { <li class="plugin-consent-perm" [attr.data-perm]="p">{{ 'perm.' + p | t }}</li> }</ul></div>
        }
        @if (c.api.length) {
          <div class="pjs-sec" id="plugin-consent-api"><h4>🛰 {{ 'pjs.consent.api' | t }}</h4>
            <ul class="pjs-list">@for (a of c.api; track a) { <li class="plugin-consent-api" [attr.data-api]="a">{{ 'svc.api.' + a | t }}</li> }</ul></div>
        }
        @if (c.scopes.length) {
          <div class="pjs-sec" id="plugin-consent-scopes"><h4>⌨️ {{ 'pjs.scopes' | t }}</h4>
            <ul class="pjs-list">
              @for (s of c.scopes; track s) {
                <li class="plugin-consent-scope" [attr.data-scope]="s" [class.sensitive]="sensitive(s)">
                  <span>{{ label(s) }}</span><code class="pjs-scope-id">{{ s }}</code>
                  @if (sensitive(s)) { <span class="pjs-sens" [title]="'scope.sensitive' | t">⚠ {{ 'scope.sensitive' | t }}</span> }
                </li>
              }
            </ul></div>
        }
        @if (c.hosts.length) {
          <div class="pjs-sec" id="plugin-consent-hosts"><h4>🌐 {{ 'pjs.hosts' | t }}</h4>
            <ul class="pjs-list">@for (h of c.hosts; track h) { <li class="plugin-consent-host mono">{{ h }}</li> }</ul></div>
        }
        @if (c.routes.length) {
          <div class="pjs-sec" id="plugin-consent-routes"><h4>🧭 {{ 'pjs.routes' | t }}</h4>
            <ul class="pjs-list">@for (r of c.routes; track r) { <li class="plugin-consent-route mono">{{ r }}</li> }</ul></div>
        }
        @if (empty()) { <p class="muted" id="plugin-consent-none">{{ 'pjs.consent.none' | t }}</p> }
        @if (c.destructive.length) {
          <div class="pjs-destructive" id="plugin-consent-destructive">
            <b>⚠ {{ 'pjs.consent.migrations' | t }}</b>
            <ul>@for (d of c.destructive; track $index) { <li class="mono plugin-consent-destructive-item" [attr.data-op]="d.op">{{ text(d) }}</li> }</ul>
            <label class="check"><input type="checkbox" id="plugin-consent-destructive-confirm" [checked]="checked()" (change)="checked.set($any($event.target).checked)" />
              <span>{{ 'pjs.consent.destructiveConfirm' | t }}</span></label>
          </div>
        }
        <div class="form-foot">
          <button class="btn" id="plugin-consent-cancel" (click)="decide.emit(false)">{{ 'common.cancel' | t }}</button>
          <button class="btn btn-primary" id="plugin-consent-confirm" [disabled]="!canConfirm()" (click)="decide.emit(true)">{{ confirmKey() | t }}</button>
        </div>
      </div>
    </div>
  `,
})
export class PluginConsentDialog {
  readonly state = input.required<ConsentState>();
  readonly decide = output<boolean>();
  readonly checked = signal(false);
  readonly canConfirm = computed(() => consentCanConfirm(this.state(), this.checked()));
  readonly empty = computed(() => { const c = this.state(); return !c.permissions.length && !c.api.length && !c.scopes.length && !c.hosts.length; });
  readonly confirmKey = computed(() => ({ install: 'pjs.consent.approve', upgrade: 'pjs.consent.approveUpgrade', reconsent: 'pjs.consent.approveActivate', activate: 'pjs.consent.approveActivate' })[this.state().mode]);
  sensitive(s: string) { return isSensitiveScope(s); }
  label(s: string) { return scopeLabel(s); }
  text = destructiveText;
}

/** Missing dependencies of a plugin about to be installed → "Install all". */
@Component({
  selector: 'rea-plugin-deps',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  styles: STYLES,
  template: `
    <div class="modal-backdrop" (click)="decide.emit(false)">
      <div class="modal" id="plugin-deps-dialog" role="dialog" aria-modal="true" (click)="$event.stopPropagation()">
        <div class="modal-head"><h3 id="plugin-deps-title">{{ 'pjs.deps.title' | t: { name: name() } }}</h3>
          <button class="icon-btn" (click)="decide.emit(false)" [attr.aria-label]="'common.cancel' | t">✕</button></div>
        <p class="small">{{ 'pjs.deps.intro' | t }}</p>
        <ul class="pjs-list" id="plugin-deps-list">
          @for (d of deps(); track d.id) {
            <li class="plugin-dep-row" [attr.data-id]="d.id" [attr.data-state]="d.state">
              <b class="mono">{{ d.id }}</b>
              <span class="muted small">{{ 'pjs.deps.range' | t: { range: d.range } }}</span>
              @if (d.latest) { <span class="muted small">· {{ 'pjs.deps.latest' | t: { v: d.latest } }}</span> }
              <span class="pjs-dep-state" [attr.data-state]="d.state">{{ d.state === 'incompatible' ? ('pjs.dep.incompatible' | t: { v: d.installedVersion || '?' }) : ('pjs.dep.' + d.state | t) }}</span>
            </li>
          }
        </ul>
        <div class="form-foot">
          <button class="btn" id="plugin-deps-cancel" (click)="decide.emit(false)">{{ 'common.cancel' | t }}</button>
          <button class="btn btn-primary" id="plugin-deps-install-all" (click)="decide.emit(true)">⬇ {{ 'pjs.deps.installAll' | t }}</button>
        </div>
      </div>
    </div>
  `,
})
export class PluginDepsDialog {
  readonly name = input.required<string>();
  readonly deps = input.required<DepStatus[]>();
  readonly decide = output<boolean>();
}

export interface UninstallState { id: string; name: string; runtime?: string; dependents: string[]; dependentNames: string[] }
export interface UninstallChoice { deleteData: boolean }

/** Uninstall: dependents warning + "delete plugin data". */
@Component({
  selector: 'rea-plugin-uninstall',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  styles: STYLES,
  template: `
    @let s = state();
    <div class="modal-backdrop" (click)="decide.emit(null)">
      <div class="modal" id="plugin-uninstall-dialog" role="dialog" aria-modal="true" [attr.data-id]="s.id" (click)="$event.stopPropagation()">
        <div class="modal-head"><h3 id="plugin-uninstall-title">🗑 {{ 'pjs.del.title' | t: { name: s.name } }}</h3>
          <button class="icon-btn" (click)="decide.emit(null)" [attr.aria-label]="'common.cancel' | t">✕</button></div>
        <p class="small">{{ 'pjs.del.text' | t }}</p>
        @if (s.dependents.length) {
          <div class="pjs-destructive" id="plugin-uninstall-dependents">
            <b>⚠ {{ 'pjs.del.dependents' | t }}</b>
            <ul>@for (d of s.dependentNames; track $index) { <li class="plugin-uninstall-dependent" [attr.data-id]="s.dependents[$index]">{{ d }}</li> }</ul>
          </div>
        }
        @if (s.runtime === 'js') {
          <label class="check" id="plugin-uninstall-delete-data-label"><input type="checkbox" id="plugin-uninstall-delete-data" [checked]="deleteData()" (change)="deleteData.set($any($event.target).checked)" />
            <span>{{ 'pjs.del.deleteData' | t: { id: s.id } }}</span></label>
          <p class="hint">{{ 'pjs.del.keepData' | t }}</p>
        }
        <div class="form-foot">
          <button class="btn" id="plugin-uninstall-cancel" (click)="decide.emit(null)">{{ 'common.cancel' | t }}</button>
          <button class="btn btn-danger" id="plugin-uninstall-confirm" (click)="decide.emit({ deleteData: deleteData() })">🗑 {{ 'pjs.del.confirm' | t }}</button>
        </div>
      </div>
    </div>
  `,
})
export class PluginUninstallDialog {
  readonly state = input.required<UninstallState>();
  readonly decide = output<UninstallChoice | null>();
  readonly deleteData = signal(false);
}
