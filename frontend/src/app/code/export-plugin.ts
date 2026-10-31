import { ChangeDetectionStrategy, Component, OnInit, computed, inject, output, signal } from '@angular/core';
import { Api, blobErrorMessage, errorMessage } from '../core/api';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { SiteInfo } from '../core/site';
import { saveBlob } from '../shared/download';
import { CODE_SCOPES, isSensitiveScope, scopeLabel, sortScopes } from '../admin/plugin-deps';
import { filenameFromDisposition } from './code-utils';
import { ExportForm, exportRequest, formFromDryRun, toggleIn, validateExport, warningText } from './export-utils';
import './messages-code4b';

/**
 * "Export as plugin" (publish_scripts): a dry run of POST /code/package
 * prefills id / name / version / scopes / routes / exports from the live
 * Site Code; the real call downloads the plugin zip.
 */
@Component({
  selector: 'rea-export-plugin',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let f = form();
    <div class="modal-backdrop" (click)="closed.emit()">
      <div class="modal code-modal" id="code-export-dialog" role="dialog" aria-modal="true" (click)="$event.stopPropagation()">
        <div class="modal-head"><h3>📦 {{ 'cx.title' | t }}</h3><button class="icon-btn" (click)="closed.emit()" [attr.aria-label]="'common.close' | t">✕</button></div>
        <p class="hint">{{ 'cx.intro' | t }}</p>
        @if (checking()) { <p class="muted small" id="code-export-checking">⏳ {{ 'cx.checking' | t }}</p> }
        @if (error()) { <p class="alert" id="code-export-error">{{ error() }}</p> }

        <div class="cx-grid">
          <label>{{ 'cx.id' | t }}
            <input id="code-export-id" class="mono" [value]="f.id" (input)="set('id', val($event))" autocomplete="off" />
            @if (errs().id; as e) { <span class="hint danger" id="code-export-id-error">{{ e | t }}</span> } @else { <span class="hint">{{ 'cx.idHint' | t }}</span> }
          </label>
          <label>{{ 'cx.version' | t }}
            <input id="code-export-version" class="mono" [value]="f.version" (input)="set('version', val($event))" />
            @if (errs().version; as e) { <span class="hint danger" id="code-export-version-error">{{ e | t }}</span> }
          </label>
        </div>
        <label>{{ 'cx.name' | t }}
          <input id="code-export-name" [value]="f.name" (input)="set('name', val($event))" />
          @if (errs().name; as e) { <span class="hint danger" id="code-export-name-error">{{ e | t }}</span> }
        </label>
        <label>{{ 'cx.description' | t }}
          <textarea id="code-export-description" rows="2" [value]="f.description" (input)="set('description', val($event))"></textarea>
          @if (errs().description; as e) { <span class="hint danger">{{ e | t }}</span> }
        </label>

        <fieldset class="cx-scopes" id="code-export-scopes">
          <legend>{{ 'cx.scopes' | t }}</legend>
          <p class="hint">{{ 'cx.scopesHint' | t }}</p>
          @for (s of scopeList(); track s) {
            <label class="check cx-scope" [attr.data-scope]="s">
              <input type="checkbox" class="code-export-scope" [attr.data-scope]="s" [checked]="f.scopes.includes(s)" (change)="toggleScope(s, $any($event.target).checked)" />
              <span>{{ label(s) }} <code class="small">{{ s }}</code>
                @if (inferred().includes(s)) { <span class="badge badge-info cx-inferred">{{ 'cx.inferred' | t }}</span> }
                @if (sensitive(s)) { <span class="small cx-sens">⚠</span> }</span>
            </label>
          }
        </fieldset>

        <label>{{ 'cx.routes' | t }}
          <textarea id="code-export-routes" class="mono" rows="2" [value]="f.routes" (input)="set('routes', val($event))" [placeholder]="'/shop'"></textarea>
          @if (errs().routes; as e) { <span class="hint danger" id="code-export-routes-error">{{ e | t: { r: badRoute() } }}</span> } @else { <span class="hint">{{ 'cx.routesHint' | t }}</span> }
        </label>
        @if (exportNames().length) {
          <fieldset class="cx-exports" id="code-export-exports">
            <legend>{{ 'cx.exports' | t }}</legend>
            @for (x of exportNames(); track x) {
              <label class="check"><input type="checkbox" class="code-export-export" [attr.data-name]="x" [checked]="f.exports.includes(x)" (change)="toggleExport(x, $any($event.target).checked)" /> <code>{{ x }}</code></label>
            }
          </fieldset>
        }
        <label>{{ 'cx.library' | t }}
          <input id="code-export-library" class="mono" [value]="f.library" (input)="set('library', val($event))" placeholder="lib/index.ts" />
        </label>

        @if (warnings().length) {
          <div class="info-box warn" id="code-export-warnings">
            <b>⚠ {{ 'cx.warnings' | t }}</b>
            <ul>@for (w of warnings(); track $index) { <li class="code-export-warning small">{{ w }}</li> }</ul>
          </div>
        }

        <div class="form-foot">
          <button class="btn" id="code-export-recheck" [disabled]="checking() || busy()" (click)="dryRun(true)">↻ {{ 'cx.recheck' | t }}</button>
          <span class="grow"></span>
          <button class="btn" id="code-export-cancel" (click)="closed.emit()">{{ 'common.cancel' | t }}</button>
          <button class="btn btn-primary" id="code-export-download" [disabled]="!valid() || busy() || checking()" (click)="download()">⬇ {{ (busy() ? 'cx.downloading' : 'cx.download') | t }}</button>
        </div>
      </div>
    </div>
  `,
  styles: `
    label { display: flex; flex-direction: column; gap: 4px; margin-bottom: 10px; }
    label.check { flex-direction: row; align-items: flex-start; gap: 8px; margin: 2px 0; }
    .cx-grid { display: grid; grid-template-columns: 2fr 1fr; gap: 12px; }
    fieldset { border: 1px solid var(--border); border-radius: 8px; padding: 8px 12px; margin: 0 0 10px; legend { font-weight: 600; font-size: 13px; padding: 0 4px; } }
    .cx-inferred { margin-left: 4px; font-size: 10.5px; }
    .cx-sens { color: #b45309; }
    .form-foot .grow { flex: 1; }
    #code-export-warnings ul { margin: 6px 0 0; padding-left: 18px; }
  `,
})
export class ExportPlugin implements OnInit {
  private api = inject(Api);
  private i18n = inject(I18n);
  private site = inject(SiteInfo);
  readonly closed = output<void>();

  readonly form = signal<ExportForm>(formFromDryRun(null));
  readonly inferred = signal<string[]>([]);
  readonly exportNames = signal<string[]>([]);
  readonly warnings = signal<string[]>([]);
  readonly checking = signal(false);
  readonly busy = signal(false);
  readonly error = signal('');
  private readonly validation = computed(() => validateExport(this.form()));
  readonly errs = computed(() => this.validation().errors);
  readonly badRoute = computed(() => this.validation().badRoute ?? '');
  readonly valid = computed(() => !Object.keys(this.errs()).length);
  /** Known scopes, plus any unknown ones the dry run inferred. */
  readonly scopeList = computed(() => sortScopes([...CODE_SCOPES, ...this.inferred(), ...this.form().scopes]));

  ngOnInit() { void this.dryRun(false); }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  label(s: string) { return scopeLabel(s); }
  sensitive(s: string) { return isSensitiveScope(s); }
  set<K extends keyof ExportForm>(k: K, v: ExportForm[K]) { this.form.update(f => ({ ...f, [k]: v })); }
  toggleScope(s: string, on: boolean) { this.set('scopes', toggleIn(this.form().scopes, s, on)); }
  toggleExport(x: string, on: boolean) { this.set('exports', toggleIn(this.form().exports, x, on)); }

  /**
   * First run: prefill everything. "Check again": send the current values
   * (the server renames hooks to the new id) and only refresh the warnings
   * and what the code is found to use.
   */
  async dryRun(keepEdits: boolean) {
    this.checking.set(true);
    this.error.set('');
    try {
      const body = keepEdits ? exportRequest(this.form()) : {};
      const dry = await this.api.codePackageDryRun(body);
      const pre = formFromDryRun(dry, this.site.title());
      this.inferred.set(dry?.manifest?.code?.scopes ?? []);
      this.exportNames.set([...new Set([...(dry?.manifest?.code?.exports ?? []), ...(keepEdits ? this.form().exports : [])])]);
      this.warnings.set((dry?.warnings ?? []).map(warningText).filter(Boolean));
      if (!keepEdits) this.form.set(pre);
    } catch (e) {
      this.error.set(errorMessage(e));
      if (!keepEdits) this.form.set(formFromDryRun(null, this.site.title()));
    } finally {
      this.checking.set(false);
    }
  }

  async download() {
    if (!this.valid()) return;
    const body = exportRequest(this.form());
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.codePackage(body);
      saveBlob(res.body ?? new Blob(), filenameFromDisposition(res.headers.get('Content-Disposition'), `${body.id}-${body.version}.zip`));
      Dialog.toast(this.i18n.t('cx.done'));
      this.closed.emit();
    } catch (e) {
      this.error.set(await blobErrorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
