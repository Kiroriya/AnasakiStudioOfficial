import { ChangeDetectionStrategy, Component, OnInit, inject, signal } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { InstalledPackage, SchemaField, ServiceHealth, ServiceView } from '../core/models';
import { Plugins } from '../core/plugins';
import { SchemaForm } from '../shared/schema-form';
import { oneLanguage } from '../shared/bilingual';
import './messages-platform';

/**
 * Generic settings screen for any plugin: built-ins at /p/:id, packages at /packages/:id.
 * The form is generated from the plugin's schema, so plugins need no admin code.
 */
@Component({
  selector: 'rea-plugin-settings',
  imports: [SchemaForm, TPipe, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <a class="back-btn" routerLink="/plugins" [queryParams]="{ tab: pkg ? 'installed' : 'builtin' }">←</a>
      <span class="page-icon">{{ icon() }}</span>
      <div class="page-head-text"><h1>{{ name() }}</h1><p>{{ oneLang(desc()) || ('plugins.settingsSub' | t) }}</p></div>
      @if (schema().length) { <button class="btn btn-primary" [disabled]="busy() || !dirty()" (click)="save()">💾 {{ 'common.save' | t }}</button> }
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }

    @if (service(); as s) {
      <section class="card schema-card svc-card">
        @if (s.runtime === 'wasm') {
        <h3>⚙️ {{ 'svc.wasmTitle' | t }}</h3>
        <p class="hint">{{ 'svc.wasmIntro' | t }}</p>
        } @else {
        <h3>🛰 {{ 'svc.title' | t }}</h3>
        <p class="hint">{{ 'svc.intro' | t }}</p>

        <label>{{ 'svc.url' | t }} <span class="muted small">{{ 'svc.urlHint' | t }}</span>
          <div class="inline-add">
            <input class="mono" [value]="url()" (input)="url.set(val($event))" placeholder="http://my-plugin:9000" />
            <button class="btn btn-primary" [disabled]="busy() || url() === s.url" (click)="saveUrl()">💾 {{ 'common.save' | t }}</button>
          </div>
        </label>

        <label>{{ 'svc.secret' | t }} <span class="muted small">{{ 'svc.secretHint' | t }}</span>
          <div class="inline-add">
            <input class="mono" [type]="showSecret() ? 'text' : 'password'" [value]="s.secret" readonly (focus)="$any($event.target).select()" />
            <button class="btn" (click)="showSecret.set(!showSecret())">{{ (showSecret() ? 'svc.hide' : 'svc.show') | t }}</button>
            <button class="btn" (click)="copy(s.secret)">📋</button>
            <button class="btn btn-danger" (click)="rotate()" [title]="'svc.rotateHint' | t">↻</button>
          </div>
        </label>
        }

        <div class="row-actions">
          <button class="btn" [disabled]="testing() || (!s.url && s.runtime !== 'wasm')" (click)="test()">🔌 {{ (testing() ? 'svc.testing' : 'svc.test') | t }}</button>
          @if (health(); as h) {
            @if (h.ok) { <span class="badge badge-live">✓ {{ 'svc.ok' | t: { name: h.name || id, version: h.version || '?', ms: h.ms } }}</span> }
            @else { <span class="badge badge-danger">✕ {{ h.error }}</span> }
          }
        </div>

        @if (s.runtime !== 'wasm') {
        <details class="svc-howto">
          <summary>{{ 'svc.howto' | t }}</summary>
          <p class="hint">{{ 'svc.envHint' | t }}</p>
          <pre class="mono">{{ envSnippet(s) }}</pre>
          <p class="hint">{{ 'svc.composeHint' | t }}</p>
          <pre class="mono">{{ composeSnippet(s) }}</pre>
        </details>
        }

        <h4>{{ 'svc.handles' | t }}</h4>
        <ul class="list small svc-list">
          @for (p of s.public; track p) { <li>🌐 {{ 'svc.public' | t }} <code>{{ p }}</code></li> }
          @for (a of s.admin; track a.path) { <li>🧭 {{ 'svc.admin' | t }} <b>{{ a.label }}</b></li> }
          @for (c of s.collections; track c.source) { <li>▦ {{ 'svc.collection' | t }} <b>{{ c.label }}</b></li> }
          @for (e of s.events; track e) { <li>📣 {{ 'svc.event' | t }} <code>{{ e }}</code></li> }
          @for (a of s.api; track a) { <li>🔑 {{ 'svc.api.' + a | t }}</li> }
          @for (h of s.hosts ?? []; track h) { <li>🌍 {{ 'svc.host' | t }} <code>{{ h }}</code></li> }
        </ul>
      </section>
    }

    @if (schema().length) {
      <section class="card form-card schema-card">
        <rea-schema-form [fields]="schema()" [values]="values()" [secretsSet]="secretsSet()" (changed)="values.set($event); dirty.set(true)" />
      </section>
    }
  `,
  styles: `.schema-card { padding: 24px; max-width: 720px; margin-bottom: 16px; }
    .svc-card h3 { margin: 0 0 4px; } .svc-card label { display: block; margin-top: 12px; }
    .svc-card .inline-add { display: flex; gap: 6px; margin-top: 4px; } .svc-card .inline-add input { flex: 1; min-width: 0; }
    .svc-card pre { background: #0f172a; color: #e2e8f0; padding: 12px 14px; border-radius: 8px; overflow-x: auto; font-size: 12px; white-space: pre; }
    .svc-howto { margin: 14px 0; } .svc-howto summary { cursor: pointer; font-weight: 600; }
    .svc-list li { padding: 3px 0; } .badge-danger { background: #fee2e2; color: #b91c1c; }`,
})
export class PluginSettings implements OnInit {
  private api = inject(Api);
  private i18n = inject(I18n);
  private plugins = inject(Plugins);
  private route = inject(ActivatedRoute);

  readonly pkg = this.route.snapshot.data['kind'] === 'package';
  readonly id = this.route.snapshot.paramMap.get('id') ?? '';
  readonly name = signal('');
  readonly desc = signal('');
  oneLang(d: string) { return oneLanguage(d, this.i18n.locale()); }
  readonly icon = signal('⚙️');
  readonly schema = signal<SchemaField[]>([]);
  readonly values = signal<Record<string, unknown>>({});
  /** Secret settings with a value (their value never comes back; CMS 2.1.0). */
  readonly secretsSet = signal<string[]>([]);
  readonly dirty = signal(false);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly service = signal<ServiceView | null>(null);
  readonly url = signal('');
  readonly showSecret = signal(false);
  readonly testing = signal(false);
  readonly health = signal<ServiceHealth | null>(null);

  async ngOnInit() {
    try {
      if (this.pkg) {
        const p = (await this.api.packages()).find(x => x.id === this.id);
        if (!p) throw new Error(this.i18n.t('plugins.notFound'));
        this.name.set(p.name);
        this.desc.set(p.description);
        this.icon.set(p.icon || '📦');
        this.schema.set(p.settingsSchema);
        this.values.set(p.settings ?? {});
        this.secretsSet.set(p.secretsSet ?? []);
        this.setService(p);
      } else {
        await this.plugins.load();
        const p = this.plugins.list().find(x => x.id === this.id);
        const en = this.i18n.locale() !== 'th'; // English for every non-Thai admin (2.2.2)
        this.name.set(p ? (en && p.nameEn) || p.name : this.id);
        this.desc.set(p ? (en && p.descriptionEn) || p.description : '');
        this.icon.set(p?.icon ?? '⚙️');
        const s = await this.api.pluginSettings(this.id);
        this.schema.set(s.schema);
        this.values.set(s.values ?? {});
      }
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  private setService(p: InstalledPackage) {
    this.service.set(p.service ?? null);
    this.url.set(p.service?.url ?? '');
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }

  async saveUrl() {
    this.busy.set(true);
    try {
      this.setService(await this.api.setServiceUrl(this.id, this.url().trim()));
      Dialog.toast(this.i18n.t('common.saved'));
      if (this.url()) await this.test();
    } catch (e) { Dialog.error(errorMessage(e)); } finally { this.busy.set(false); }
  }

  async rotate() {
    if (!(await Dialog.confirm({ title: this.i18n.t('svc.rotateConfirm'), text: this.i18n.t('svc.rotateText'), danger: true }))) return;
    try { this.setService(await this.api.rotateServiceSecret(this.id)); this.health.set(null); } catch (e) { Dialog.error(errorMessage(e)); }
  }

  async test() {
    this.testing.set(true);
    try { this.health.set(await this.api.serviceHealth(this.id)); } catch (e) { Dialog.error(errorMessage(e)); } finally { this.testing.set(false); }
  }

  copy(v: string) {
    navigator.clipboard?.writeText(v).then(() => Dialog.toast(this.i18n.t('svc.copied'))).catch(() => undefined);
  }

  envSnippet(s: ServiceView) {
    return `ANASAKI_PLUGIN_ID=${this.id}\nANASAKI_PLUGIN_SECRET=${this.showSecret() ? s.secret : this.i18n.t('svc.secretPh')}\nANASAKI_URL=http://studio:8090`;
  }

  composeSnippet(s: ServiceView) {
    return `  ${this.id}:\n    image: ${this.i18n.t('svc.imagePh')}\n    environment:\n      ANASAKI_PLUGIN_ID: ${this.id}\n      ANASAKI_PLUGIN_SECRET: \${${this.id.toUpperCase().replace(/-/g, '_')}_SECRET}\n      ANASAKI_URL: http://studio:8090\n    restart: unless-stopped\n\n# ${this.i18n.t('svc.composeUrlHint')} http://${this.id}:<port>`;
  }

  async save() {
    this.busy.set(true);
    try {
      if (this.pkg) {
        const p = await this.api.savePackageSettings(this.id, this.values());
        this.values.set(p.settings ?? this.values());
        this.secretsSet.set(p.secretsSet ?? []);
      } else {
        const s = await this.api.savePluginSettings(this.id, this.values());
        this.values.set(s.values ?? this.values());
      }
      this.dirty.set(false);
      Dialog.toast(this.i18n.t('common.saved'));
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
