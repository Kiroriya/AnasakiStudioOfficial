import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal, untracked } from '@angular/core';
import { Api, SITE_ORIGIN, errorMessage } from '../core/api';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { ApiToken } from '../core/models';
import {
  DEFAULT_TOKEN_EXPIRY, TOKEN_EXPIRY_DAYS, TOKEN_NAME_MAX, TOKEN_SCOPES, cliLoginCommand, normalizeTokens, tokenCreateBody, tokenExpired, tokenFormError, tokenPasswordError,
} from './api-tokens-utils';
import './messages-apitokens';

/**
 * "API tokens / โทเค็นสำหรับเครื่องมือ": personal access tokens for tools such as the
 * anasaki-code CLI. On the profile: list, create (the secret is shown once), revoke. With
 * `userId` (user editor, manage_users): that user's tokens, revoke only.
 */
@Component({
  selector: 'rea-api-tokens',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="card api-tokens" id="api-tokens" [class.api-tokens-admin]="!!userId()">
      <div class="card-head">
        <div>
          <h3>🔑 {{ 'apitok.title' | t }}</h3>
          <p class="muted small">{{ (userId() ? 'apitok.descAdmin' : 'apitok.desc') | t }}</p>
        </div>
        @if (!userId()) { <button type="button" class="btn btn-sm btn-primary" id="api-token-create" (click)="openCreate()">+ {{ 'apitok.new' | t }}</button> }
      </div>
      @if (error()) { <p class="alert">{{ error() }}</p> }
      <div class="api-token-table">
        <table class="table">
          <thead>
            <tr>
              <th>{{ 'apitok.name' | t }}</th>
              <th>{{ 'apitok.prefix' | t }}</th>
              <th>{{ 'apitok.scopes' | t }}</th>
              <th>{{ 'apitok.lastUsed' | t }}</th>
              <th>{{ 'apitok.expires' | t }}</th>
              <th class="th-actions"></th>
            </tr>
          </thead>
          <tbody>
            @for (t of items(); track t.id) {
              <tr class="api-token-row" [attr.data-id]="t.id" [class.is-expired]="expired(t)">
                <td><b class="api-token-name">{{ t.name }}</b></td>
                <td><code class="mono api-token-prefix">{{ t.prefix }}…</code></td>
                <td>@for (s of t.scopes; track s) { <span class="chip mono">{{ s }}</span> }</td>
                <td class="muted small">{{ t.lastUsedAt ? i18n.date(t.lastUsedAt, true) : ('apitok.neverUsed' | t) }}</td>
                <td class="small">
                  @if (expired(t)) { <span class="badge badge-danger">{{ 'apitok.expired' | t }}</span> }
                  @else if (t.expiresAt) { {{ i18n.date(t.expiresAt) }} }
                  @else { <span class="muted">{{ 'apitok.never' | t }}</span> }
                </td>
                <td class="row-actions"><button type="button" class="btn btn-sm btn-danger api-token-revoke" (click)="revoke(t)">{{ 'apitok.revoke' | t }}</button></td>
              </tr>
            } @empty {
              <tr><td colspan="6" class="muted small api-tokens-empty" id="api-tokens-empty">{{ (loading() ? 'common.loading' : 'apitok.empty') | t }}</td></tr>
            }
          </tbody>
        </table>
      </div>
    </section>

    @if (dialogOpen()) {
      <div class="modal-backdrop" (click)="created() ? null : close()">
        <div class="modal" id="api-token-dialog" role="dialog" aria-modal="true" (click)="$event.stopPropagation()">
          @if (created(); as c) {
            <header class="modal-head"><h3>✅ {{ 'apitok.createdTitle' | t }}</h3></header>
            <div class="info-box warn">⚠️ {{ 'apitok.onceWarn' | t }}</div>
            <div class="api-token-copy-row">
              <code class="mono api-token-secret" id="api-token-value">{{ c.token }}</code>
              <button type="button" class="btn btn-sm" id="api-token-copy" (click)="copy(c.token)">📋 {{ 'apitok.copy' | t }}</button>
            </div>
            <p class="muted small">{{ 'apitok.cli' | t }}</p>
            <div class="api-token-copy-row">
              <code class="mono api-token-secret" id="api-token-cli">{{ cli() }}</code>
              <button type="button" class="btn btn-sm" id="api-token-cli-copy" (click)="copy(cli())">📋 {{ 'apitok.copy' | t }}</button>
            </div>
            <div class="form-foot"><span class="grow"></span><button type="button" class="btn btn-primary" id="api-token-done" (click)="close()">{{ 'apitok.done' | t }}</button></div>
          } @else {
            <header class="modal-head"><h3>🔑 {{ 'apitok.new' | t }}</h3><button type="button" class="btn btn-ghost btn-sm" (click)="close()" [attr.aria-label]="'common.close' | t">✕</button></header>
            <label>{{ 'apitok.name' | t }}
              <input id="api-token-name" [maxLength]="nameMax" [placeholder]="'apitok.namePh' | t" [value]="name()" (input)="name.set(val($event))" (keyup.enter)="save()" />
            </label>
            <label>{{ 'apitok.expiry' | t }}
              <select id="api-token-expiry" [value]="expiry()" (change)="expiry.set(+val($event))">
                @for (d of expiryChoices; track d) { <option [value]="d" [selected]="d === expiry()">{{ expiryLabel(d) }}</option> }
              </select>
            </label>
            <label>{{ 'apitok.password' | t }}
              <input id="api-token-password" type="password" autocomplete="current-password" [value]="password()" (input)="password.set(val($event))" (keyup.enter)="save()" />
            </label>
            <fieldset class="api-token-scopes">
              <legend>{{ 'apitok.scopes' | t }}</legend>
              @for (s of scopeChoices; track s) {
                <label class="check"><input type="checkbox" [id]="'api-token-scope-' + s" [checked]="scopes().includes(s)" (change)="toggleScope(s)" /> {{ ('apitok.scope.' + s) | t }}</label>
              }
              <p class="hint">{{ 'apitok.scopeHint' | t }}</p>
            </fieldset>
            @if (formError() && touched()) { <p class="alert small" id="api-token-error">{{ formError() | t }}</p> }
            <div class="form-foot">
              <span class="grow"></span>
              <button type="button" class="btn" (click)="close()">{{ 'common.cancel' | t }}</button>
              <button type="button" class="btn btn-primary" id="api-token-save" [disabled]="busy()" (click)="save()">{{ 'apitok.create' | t }}</button>
            </div>
          }
        </div>
      </div>
    }
  `,
  styles: `
    .api-tokens { margin-top: 20px; max-width: 860px; }
    .api-tokens-admin { margin-top: 8px; box-shadow: none; padding: 12px 0 0; border: 0; }
    .api-token-table { overflow-x: auto; }
    .is-expired { opacity: .6; }
    .api-token-copy-row { display: flex; gap: 8px; align-items: center; margin: 6px 0 12px; }
    .api-token-secret { flex: 1; min-width: 0; padding: 8px 10px; background: #f6f7fb; border: 1px solid #e3e5ee; border-radius: 8px; word-break: break-all; user-select: all; }
    .api-token-scopes { border: 0; padding: 0; margin: 8px 0; legend { font-weight: 600; font-size: 13px; margin-bottom: 4px; } }
  `,
})
export class ApiTokens {
  readonly i18n = inject(I18n);
  private api = inject(Api);

  /** Another user's tokens (user editor, manage_users): list + revoke, no create. */
  readonly userId = input<number | undefined>(undefined);

  readonly items = signal<ApiToken[]>([]);
  readonly loading = signal(true);
  readonly error = signal('');
  readonly dialogOpen = signal(false);
  readonly name = signal('');
  readonly password = signal('');
  readonly expiry = signal<number>(DEFAULT_TOKEN_EXPIRY);
  readonly scopes = signal<string[]>([...TOKEN_SCOPES]);
  readonly touched = signal(false);
  readonly busy = signal(false);
  readonly created = signal<{ token: string; item: ApiToken } | null>(null);
  readonly formError = computed(() => tokenFormError(this.name(), this.scopes()) || tokenPasswordError(this.password()));
  readonly cli = computed(() => cliLoginCommand(SITE_ORIGIN || location.origin, this.created()?.token ?? ''));

  readonly expiryChoices = TOKEN_EXPIRY_DAYS;
  readonly scopeChoices = TOKEN_SCOPES;
  readonly nameMax = TOKEN_NAME_MAX;

  constructor() {
    effect(() => {
      const u = this.userId();
      untracked(() => void this.load(u));
    });
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  expired(t: ApiToken) { return tokenExpired(t); }
  expiryLabel(d: number) {
    if (d === 0) return this.i18n.t('apitok.never');
    if (d === 365) return this.i18n.t('apitok.year');
    return this.i18n.t('apitok.days', { n: d });
  }

  async load(user = this.userId()) {
    this.loading.set(true);
    try {
      this.items.set(normalizeTokens(await this.api.apiTokens(user)));
      this.error.set('');
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.loading.set(false);
    }
  }

  openCreate() {
    this.name.set('');
    this.password.set('');
    this.expiry.set(DEFAULT_TOKEN_EXPIRY);
    this.scopes.set([...TOKEN_SCOPES]);
    this.touched.set(false);
    this.created.set(null);
    this.dialogOpen.set(true);
    queueMicrotask(() => document.getElementById('api-token-name')?.focus());
  }

  close() {
    this.dialogOpen.set(false);
    this.created.set(null); // the secret is gone for good
  }

  toggleScope(s: string) {
    this.scopes.update(l => (l.includes(s) ? l.filter(x => x !== s) : [...l, s]));
  }

  async save() {
    this.touched.set(true);
    if (this.formError() || this.busy()) return;
    this.busy.set(true);
    try {
      const res = await this.api.createApiToken(tokenCreateBody(this.name(), this.expiry(), this.scopes(), this.password()));
      this.password.set('');
      this.created.set(res);
      if (res.item) this.items.update(l => normalizeTokens([res.item, ...l.filter(x => x.id !== res.item.id)]));
      else await this.load();
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }

  copy(text: string) {
    navigator.clipboard?.writeText(text).then(() => Dialog.toast(this.i18n.t('apitok.copied'))).catch(() => undefined);
  }

  async revoke(t: ApiToken) {
    const ok = await Dialog.confirm({
      title: this.i18n.t('apitok.revokeTitle', { name: t.name }), text: this.i18n.t('apitok.revokeText'),
      confirm: this.i18n.t('apitok.revoke'), danger: true,
    });
    if (!ok) return;
    try {
      await this.api.revokeApiToken(t.id, this.userId());
      this.items.update(l => l.filter(x => x.id !== t.id));
      Dialog.toast(this.i18n.t('apitok.revoked'));
    } catch (e) { Dialog.error(errorMessage(e)); }
  }
}
