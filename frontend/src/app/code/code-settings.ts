import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { CodeSettings } from '../core/code-models';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { ActivatedRoute } from '@angular/router';
import { CodeTabs } from './code-tabs';
import { CodeUnits, UnitSwitcher } from './code-units';
import { SITE_UNIT, settingsForUnit } from './units-utils';
import {
  MAIL_LIMIT_MAX, MASK, SettingRow, buildSettingsPut, hostError, keyError, mailLimitError, normalizeHost, rowInvalid, rowsFromSettings, settingsDirty,
} from './settings-utils';
import './messages-code4';

/**
 * /code/settings (manage_scripts) — values the code reads with
 * site.settings(): secrets are write-only (masked; replace or remove only),
 * the http.fetch host allowlist and the mail quota. One Save sends it all.
 */
@Component({
  selector: 'rea-code-settings',
  imports: [TPipe, CodeTabs, UnitSwitcher],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'code-page' },
  template: `
    <header class="page-head">
      <span class="page-icon">🔑</span>
      <div class="page-head-text"><h1>{{ 'cs.title' | t }}</h1><p>{{ 'cs.desc' | t }}</p></div>
      <div class="head-actions">
        @if (dirty()) { <span class="save-state" id="code-settings-unsaved">● {{ 'cs.unsaved' | t }}</span> }
        <button class="btn btn-primary" id="code-settings-save-top" [disabled]="!canSave()" (click)="save()">💾 {{ 'common.save' | t }}</button>
      </div>
    </header>
    <rea-code-tabs />
    <rea-unit-switcher (changed)="load()" />
    @if (error()) { <p class="alert" id="code-settings-error">{{ error() }}</p> }
    @if (isPlugin()) { <p class="info-box" id="code-settings-plugin-hint">🧩 {{ 'cu.pluginSettingsHint' | t }}</p> }

    <section class="card cs-card" id="code-settings-values">
      <h3>🔒 {{ 'cs.values' | t }}</h3>
      <p class="hint">{{ 'cs.valuesHint' | t }}</p>
      <div class="table-scroll">
        <table class="table cs-table">
          <thead><tr><th>{{ 'cs.col.key' | t }}</th><th>{{ 'cs.col.kind' | t }}</th><th>{{ 'cs.col.value' | t }}</th><th class="th-actions">{{ 'common.actions' | t }}</th></tr></thead>
          <tbody>
            @for (r of rows(); track r.key; let i = $index) {
              <tr class="cs-row" [attr.data-key]="r.key" [attr.data-secret]="r.secret ? '1' : '0'" [class.removed]="r.removed">
                <td class="mono"><b>{{ r.key }}</b>@if (r.isNew) { <span class="badge badge-info cs-new-badge">{{ 'cs.new' | t }}</span> }</td>
                <td>@if (r.secret) { <span class="badge badge-warn cs-kind">🔒 {{ 'cs.secret' | t }}</span> } @else { <span class="badge cs-kind">{{ 'cs.plain' | t }}</span> }</td>
                <td class="cs-value-cell">
                  @if (r.removed) {
                    <span class="muted small cs-removed">{{ 'cs.removed' | t }}</span>
                  } @else if (r.secret) {
                    @if (r.replace !== null) {
                      <input type="password" class="cs-replace-input" autocomplete="new-password" [value]="r.replace" [placeholder]="'cs.replacePh' | t" (input)="setReplace(i, val($event))" />
                      @if (invalidRow(r)) { <p class="hint danger">{{ 'cs.err.secretValue' | t }}</p> }
                    } @else if (r.set) {
                      <span class="mono cs-mask" [title]="'cs.secret' | t">{{ mask }}</span>
                    } @else {
                      <span class="muted small cs-not-set">{{ 'cs.notSet' | t }}</span>
                    }
                  } @else {
                    <input class="cs-value-input mono" [value]="r.value" (input)="setValue(i, val($event))" />
                  }
                </td>
                <td class="row-actions">
                  @if (!r.removed && r.secret && !r.isNew) {
                    @if (r.replace === null) { <button class="btn btn-sm cs-replace" (click)="startReplace(i)">✎ {{ 'cs.replace' | t }}</button> }
                    @else { <button class="btn btn-sm cs-cancel-replace" (click)="cancelReplace(i)">{{ 'cs.cancelReplace' | t }}</button> }
                  }
                  @if (r.removed) { <button class="btn btn-sm cs-undo-remove" (click)="undoRemove(i)">↺ {{ 'cs.undoRemove' | t }}</button> }
                  @else { <button class="btn btn-sm btn-danger cs-remove" (click)="remove(i)">🗑 {{ 'cs.remove' | t }}</button> }
                </td>
              </tr>
            } @empty {
              <tr><td colspan="4" class="empty-state" id="code-settings-empty">{{ loading() ? ('common.loading' | t) : ('cs.empty' | t) }}</td></tr>
            }
          </tbody>
        </table>
      </div>
      <div class="cs-add" id="code-settings-add">
        <input id="cs-new-key" class="mono" [value]="newKey()" (input)="newKey.set(val($event)); newKeyTouched.set(true)" (keydown.enter)="add()" [placeholder]="'cs.newKeyPh' | t" [attr.aria-label]="'cs.col.key' | t" />
        <input id="cs-new-value" [type]="newSecret() ? 'password' : 'text'" autocomplete="new-password" [value]="newValue()" (input)="newValue.set(val($event))" (keydown.enter)="add()" [placeholder]="'cs.newValuePh' | t" [attr.aria-label]="'cs.col.value' | t" />
        <label class="check cs-secret-check"><input type="checkbox" id="cs-new-secret" [checked]="newSecret()" (change)="newSecret.set(chk($event))" /> {{ 'cs.isSecret' | t }}</label>
        <button class="btn" id="cs-add" [disabled]="!!newKeyError() || (newSecret() && !newValue())" (click)="add()">+ {{ 'cs.add' | t }}</button>
      </div>
      @if (newKeyTouched() && newKeyError(); as e) { <p class="hint danger" id="cs-new-error">{{ 'cs.err.' + e | t }}</p> }
    </section>

    @if (!isPlugin()) {
    <section class="card cs-card" id="code-settings-hosts">
      <h3>🌐 {{ 'cs.hosts' | t }}</h3>
      <p class="hint">{{ 'cs.hostsHint' | t }}</p>
      <div class="cs-chips">
        @for (h of hosts(); track h) {
          <span class="chip cs-host-chip" [attr.data-host]="h"><span class="mono">{{ h }}</span>
            <button class="cs-host-remove" [attr.aria-label]="'cs.removeHost' | t: { host: h }" [title]="'cs.removeHost' | t: { host: h }" (click)="removeHost(h)">×</button></span>
        } @empty { <span class="muted small" id="cs-no-hosts">{{ 'cs.noHosts' | t }}</span> }
      </div>
      <div class="cs-add">
        <input id="cs-host-input" class="mono" [value]="hostInput()" (input)="hostInput.set(val($event)); hostTouched.set(false)" (keydown.enter)="addHost()" [placeholder]="'cs.hostPh' | t" [attr.aria-label]="'cs.hosts' | t" />
        <button class="btn" id="cs-host-add" (click)="addHost()">+ {{ 'cs.addHost' | t }}</button>
      </div>
      @if (hostTouched() && hostErr(); as e) { <p class="hint danger" id="cs-host-error">{{ 'cs.host.' + e | t }}</p> }
    </section>

    <section class="card cs-card" id="code-settings-mail">
      <h3>✉️ {{ 'cs.mail' | t }}</h3>
      <label class="cs-mail">{{ 'cs.mailPerHour' | t }}
        <input type="number" id="cs-mail-per-hour" min="0" [attr.max]="mailMax" step="1" class="input-narrow" [value]="mail()" (input)="mail.set(val($event))" />
      </label>
      @if (mailErr()) { <p class="hint danger" id="cs-mail-error">{{ 'cs.mailInvalid' | t: { max: mailMax } }}</p> }
      <p class="hint">{{ 'cs.mailHint' | t }}</p>
    </section>
    }

    <div class="form-foot cs-foot">
      <button class="btn" id="code-settings-revert" [disabled]="!dirty() || busy()" (click)="revert()">{{ 'cs.revert' | t }}</button>
      <button class="btn btn-primary" id="code-settings-save" [disabled]="!canSave()" (click)="save()">💾 {{ 'common.save' | t }}</button>
    </div>
  `,
  styles: `
    .cs-card { margin-bottom: 16px; padding: 16px 18px; h3 { margin: 0 0 4px; font-size: 15px; } }
    .cs-table { margin: 8px 0 12px; .removed td { opacity: .55; text-decoration: line-through; } .removed td.row-actions { opacity: 1; text-decoration: none; } }
    .cs-value-cell input { width: 100%; max-width: 420px; }
    .cs-new-badge { margin-left: 6px; }
    .cs-add { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; input { width: auto; flex: 1 1 180px; min-width: 0; } label.check { margin: 0; } }
    .cs-chips { display: flex; flex-wrap: wrap; gap: 4px; margin: 8px 0 12px; }
    .cs-host-chip { display: inline-flex; align-items: center; gap: 6px; }
    .cs-host-remove { border: 0; background: none; cursor: pointer; font-size: 15px; line-height: 1; color: var(--muted); padding: 0 2px; &:hover { color: #b91c1c; } }
    .cs-mail { display: flex; flex-direction: column; gap: 4px; max-width: 260px; }
    .cs-foot { display: flex; justify-content: flex-end; gap: 8px; }
  `,
})
export class CodeSettingsPage implements OnInit {
  private api = inject(Api);
  private i18n = inject(I18n);
  private units = inject(CodeUnits);
  private route = inject(ActivatedRoute);

  /** Plugin units only have their own values; the host allowlist and mail quota belong to the site. */
  readonly isPlugin = computed(() => this.units.current() !== SITE_UNIT);
  readonly mask = MASK;
  readonly mailMax = MAIL_LIMIT_MAX;
  readonly loading = signal(true);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly original = signal<CodeSettings | null>(null);
  readonly rows = signal<SettingRow[]>([]);
  readonly hosts = signal<string[]>([]);
  readonly mail = signal<string>('100');
  readonly newKey = signal('');
  readonly newKeyTouched = signal(false);
  readonly newValue = signal('');
  readonly newSecret = signal(true);
  readonly hostInput = signal('');
  readonly hostTouched = signal(false);

  readonly newKeyError = computed(() => keyError(this.newKey(), this.rows()));
  readonly hostErr = computed(() => hostError(normalizeHost(this.hostInput()), this.hosts()));
  readonly mailErr = computed(() => mailLimitError(this.mail()));
  readonly dirty = computed(() => settingsDirty(this.original(), this.rows(), this.hosts(), this.mail()));
  readonly canSave = computed(() => this.dirty() && !this.busy() && !this.mailErr() && !this.rows().some(rowInvalid));

  ngOnInit() { this.units.adopt(this.route); void this.load(); }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  chk(e: Event) { return (e.target as HTMLInputElement).checked; }
  invalidRow(r: SettingRow) { return rowInvalid(r); }

  async load() {
    this.loading.set(true);
    this.error.set('');
    try {
      await this.units.load();
      this.apply(await this.api.codeSettings(this.units.param()));
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.loading.set(false);
    }
  }

  private apply(s: CodeSettings) {
    const norm: CodeSettings = { values: settingsForUnit(s?.values ?? [], this.units.param()), fetchHosts: s?.fetchHosts ?? [], limits: { mailPerHour: s?.limits?.mailPerHour ?? 100 } };
    this.original.set(norm);
    this.rows.set(rowsFromSettings(norm.values));
    this.hosts.set([...norm.fetchHosts]);
    this.mail.set(String(norm.limits.mailPerHour));
  }

  private patch(i: number, p: Partial<SettingRow>) { this.rows.update(rs => rs.map((r, j) => (j === i ? { ...r, ...p } : r))); }
  setValue(i: number, v: string) { this.patch(i, { value: v }); }
  setReplace(i: number, v: string) { this.patch(i, { replace: v }); }
  startReplace(i: number) { this.patch(i, { replace: '' }); }
  cancelReplace(i: number) { this.patch(i, { replace: null }); }
  undoRemove(i: number) { this.patch(i, { removed: false }); }

  async remove(i: number) {
    const r = this.rows()[i];
    if (!r) return;
    if (r.isNew) { this.rows.update(rs => rs.filter((_, j) => j !== i)); return; }
    if (!(await Dialog.confirm({ title: this.i18n.t('cs.removeConfirm', { key: r.key }), text: this.i18n.t('cs.removeText', { key: r.key }), danger: true }))) return;
    this.patch(i, { removed: true, replace: null });
  }

  add() {
    this.newKeyTouched.set(true);
    if (this.newKeyError() || (this.newSecret() && !this.newValue())) return;
    const secret = this.newSecret();
    const row: SettingRow = { key: this.newKey().trim(), secret, set: false, value: secret ? '' : this.newValue(), replace: secret ? this.newValue() : null, isNew: true, removed: false };
    this.rows.update(rs => [...rs, row]);
    this.newKey.set('');
    this.newValue.set('');
    this.newKeyTouched.set(false);
  }

  addHost() {
    this.hostTouched.set(true);
    const h = normalizeHost(this.hostInput());
    if (hostError(h, this.hosts())) return;
    this.hosts.update(list => [...list, h]);
    this.hostInput.set('');
    this.hostTouched.set(false);
  }

  removeHost(h: string) { this.hosts.update(list => list.filter(x => x !== h)); }

  revert() { const o = this.original(); if (o) this.apply(o); }

  async save() {
    if (!this.canSave()) return;
    this.busy.set(true);
    this.error.set('');
    try {
      const unit = this.units.param();
      const res = await this.api.saveCodeSettings(buildSettingsPut(this.rows(), this.hosts(), Number(this.mail())), unit === SITE_UNIT ? undefined : unit);
      // the server answers with the stored state; reload when it sends nothing
      if (res && Array.isArray(res.values)) this.apply(res); else await this.load();
      Dialog.toast(this.i18n.t('cs.saved'));
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
