import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { Api, ReservedPath, SITE_ORIGIN, errorMessage } from '../core/api';
import { Dialog } from '../core/dialog';
import { I18n, TPipe, localized } from '../core/i18n';
import { CodePostType } from '../core/code-models';
import { PostTypes } from '../core/post-types';
import { routedTypes, templateOption, typePrefix } from '../core/post-type-utils';
import { Media, Post, Settings } from '../core/models';
import { Plugins } from '../core/plugins';
import { SiteInfo } from '../core/site';
import { MediaPicker } from '../shared/media-picker';
import './messages-admin';
import './messages-platform';
import './messages-content-types';
import { SelectValue } from '../shared/select-value';

type Tab = 'general' | 'reading' | 'permalinks' | 'media' | 'email' | 'admin';

const TIMEZONES = [
  'Asia/Bangkok', 'Asia/Ho_Chi_Minh', 'Asia/Jakarta', 'Asia/Kuala_Lumpur', 'Asia/Singapore', 'Asia/Manila', 'Asia/Yangon',
  'Asia/Tokyo', 'Asia/Seoul', 'Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Taipei', 'Asia/Kolkata', 'Asia/Dubai',
  'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'America/New_York', 'America/Chicago', 'America/Los_Angeles',
  'Australia/Sydney', 'UTC',
];
/** Go time layouts (the server formats dates with them). */
const DATE_FORMATS = ['2 Jan 2006', '02/01/2006', '2006-01-02', '01/02/2006', '2 January 2006', 'January 2, 2006'];
/**
 * The core's own top-level paths (same list as the server's corePaths). The paths of plugins are never
 * listed here: they come from GET /settings/reserved-paths (the routes the installed plugins declare).
 */
export const CORE_PATHS = ['api', 'uploads', 'download', 'plugin-assets', 'ext-ui', 'setup', '_a', '_c',
  'sitemap.xml', 'robots.txt', 'favicon.ico', 'favicon.svg', 'assets', 'static'];

/** Who owns path p: null = free, '' = the core, else the id of the plugin that declares it. */
export function reservedOwner(p: string, reserved: ReservedPath[]): string | null {
  if (CORE_PATHS.includes(p)) return '';
  const r = reserved.find(x => x.path === p);
  return r ? (r.plugin ?? '') : null;
}

/** Site settings, split into tabs like WordPress (General, Reading, Permalinks, Media) plus admin URL and plugin store. */
@Component({
  selector: 'rea-settings',
  imports: [SelectValue, TPipe, MediaPicker],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">⚙️</span>
      <div class="page-head-text"><h1>{{ 'nav.settings' | t }}</h1><p>{{ 'settings.desc' | t }}</p></div>
      <div class="head-actions">
        @if (dirty()) { <span class="save-state">● {{ 'settings.unsaved' | t }}</span> }
        <button class="btn btn-primary" [disabled]="busy() || !dirty() || !!pathError()" [title]="pathError()" (click)="save()">💾 {{ 'common.save' | t }}</button>
      </div>
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }

    <div class="settings-layout">
      <nav class="card settings-nav">
        @for (t of tabs; track t.id) {
          <button class="settings-tab" [class.on]="tab() === t.id" (click)="setTab(t.id)">
            <span class="side-icon">{{ t.icon }}</span>
            <span><b>{{ 'settings.tab.' + t.id | t }}</b><small>{{ 'settings.tab.' + t.id + '.sub' | t }}</small></span>
          </button>
        }
      </nav>

      <section class="card form-card settings-body">
        @switch (tab()) {
          @case ('general') {
            <div class="form-row">
              <div class="form-label">{{ 'settings.siteTitle' | t }}<small>{{ 'settings.siteTitleHint' | t }}</small></div>
              <input [value]="v('site_title')" (input)="set('site_title', $event)" />
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.tagline' | t }}<small>{{ 'settings.taglineHint' | t }}</small></div>
              <input [value]="v('tagline')" (input)="set('tagline', $event)" />
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.icon' | t }}<small>{{ 'settings.iconHint' | t }}</small></div>
              <div class="image-field">
                <span class="image-preview icon-preview">@if (v('site_icon')) { <img [src]="v('site_icon')" alt="" /> } @else { 🌐 }</span>
                <button class="btn btn-sm" (click)="pickFor.set('site_icon')">🖼 {{ 'post.chooseImage' | t }}</button>
                @if (v('site_icon')) { <button class="btn btn-sm" (click)="put('site_icon', '')">{{ 'post.removeImage' | t }}</button> }
              </div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.logo' | t }}<small>{{ 'settings.logoHint' | t }}</small></div>
              <div class="image-field">
                <span class="image-preview logo-preview">@if (v('site_logo')) { <img [src]="v('site_logo')" alt="" /> } @else { <span class="muted small">{{ 'settings.noLogo' | t }}</span> }</span>
                <button class="btn btn-sm" (click)="pickFor.set('site_logo')">🖼 {{ 'post.chooseImage' | t }}</button>
                @if (v('site_logo')) { <button class="btn btn-sm" (click)="put('site_logo', '')">{{ 'post.removeImage' | t }}</button> }
              </div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.adminEmail' | t }}<small>{{ 'settings.adminEmailHint' | t }}</small></div>
              <input type="email" [value]="v('admin_email')" (input)="set('admin_email', $event)" />
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.contentLang' | t }}<small>{{ 'settings.contentLangHint' | t }}</small></div>
              <select [value]="v('language') || 'th'" (change)="set('language', $event)">
                <option value="th">ไทย (th)</option>
                <option value="en">English (en)</option>
              </select>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.timezone' | t }}<small>{{ 'settings.timezoneHint' | t }}</small></div>
              <select [value]="v('timezone') || 'Asia/Bangkok'" (change)="set('timezone', $event)">
                @for (z of timezones; track z) { <option [value]="z">{{ z }}</option> }
              </select>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.dateFormat' | t }}<small>{{ 'settings.dateFormatHint' | t }}</small></div>
              <div class="radio-list">
                @for (f of dateFormats; track f) {
                  <label class="check"><input type="radio" name="df" [checked]="(v('date_format') || '2 Jan 2006') === f" (change)="put('date_format', f)" />
                    <span>{{ sampleDate(f) }}</span> <code class="muted small">{{ f }}</code></label>
                }
              </div>
            </div>
          }

          @case ('reading') {
            <div class="form-row">
              <div class="form-label">{{ 'settings.home' | t }}<small>{{ 'settings.homeHint' | t }}</small></div>
              <select [value]="v('home_page_id')" (change)="set('home_page_id', $event)">
                <option value="">{{ 'settings.latestPosts' | t }}</option>
                @for (p of pages(); track p.id) { <option [value]="'' + p.id">{{ p.title }}</option> }
              </select>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.single' | t }}<small>{{ 'settings.singleHint' | t }}</small></div>
              <select [value]="v('post_template_id')" (change)="set('post_template_id', $event)">
                <option value="">{{ 'settings.basic' | t }}</option>
                @for (p of templates(); track p.id) { <option [value]="'' + p.id">{{ p.title }}</option> }
              </select>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.archive' | t }}<small>{{ 'settings.archiveHint' | t }}</small></div>
              <select [value]="v('archive_template_id')" (change)="set('archive_template_id', $event)">
                <option value="">{{ 'settings.basic' | t }}</option>
                @for (p of templates(); track p.id) { <option [value]="'' + p.id">{{ p.title }}</option> }
              </select>
            </div>
            @if (routedTypes().length) {
              <div class="form-row" id="settings-type-templates">
                <div class="form-label">{{ 'settings.typeTemplates' | t }}<small>{{ 'settings.typeTemplatesHint' | t }}</small></div>
                <div class="type-tpl-list">
                  @for (pt of routedTypes(); track pt.type) {
                    <label class="type-tpl-row" [attr.data-type]="pt.type">
                      <span>{{ pt.icon || '📦' }} {{ typeName(pt) }} <small class="muted mono">{{ 'settings.typeTemplateHint' | t: { slug: prefix(pt) } }}</small></span>
                      <select [id]="'settings-type-template-' + pt.type" [value]="v(tplKey(pt.type))" (change)="set(tplKey(pt.type), $event)">
                        <option value="">{{ 'settings.postTemplateDefault' | t }}</option>
                        @for (p of templates(); track p.id) { <option [value]="'' + p.id">{{ p.title }}</option> }
                      </select>
                    </label>
                  }
                </div>
              </div>
            }
            <div class="form-row">
              <div class="form-label">{{ 'settings.perPage' | t }}<small>{{ 'settings.perPageHint' | t }}</small></div>
              <input type="number" min="1" max="100" class="input-narrow" [value]="v('posts_per_page') || '10'" (input)="set('posts_per_page', $event)" />
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.searchVis' | t }}<small>{{ 'settings.searchVisHint' | t }}</small></div>
              <label class="check switch-row">
                <input type="checkbox" [checked]="v('search_visibility') === '0'" (change)="put('search_visibility', check($event) ? '0' : '1')" />
                {{ 'settings.discourage' | t }}
              </label>
            </div>
          }

          @case ('permalinks') {
            <div class="info-box">🔗 {{ 'settings.permalinkInfo' | t }}</div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.blogBase' | t }}<small>{{ 'settings.blogBaseHint' | t }}</small></div>
              <div>
                <div class="prefix-input"><span>{{ origin }}/</span><input [value]="v('blog_base') || 'blog'" (input)="setSlug('blog_base', $event)" /></div>
                <p class="hint mono">{{ origin }}/{{ v('blog_base') || 'blog' }}/hello-world</p>
              </div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.catBase' | t }}<small>{{ 'settings.catBaseHint' | t }}</small></div>
              <div>
                <div class="prefix-input"><span>{{ origin }}/</span><input [value]="v('category_base') || 'category'" (input)="setSlug('category_base', $event)" /></div>
                <p class="hint mono">{{ origin }}/{{ v('category_base') || 'category' }}/news</p>
              </div>
            </div>
          }

          @case ('media') {
            <div class="form-row">
              <div class="form-label">{{ 'settings.maxUpload' | t }}<small>{{ 'settings.maxUploadHint' | t }}</small></div>
              <div class="suffix-input"><input type="number" min="1" max="2048" [value]="v('max_upload_mb') || '32'" (input)="set('max_upload_mb', $event)" /><span>MB</span></div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.allowedTypes' | t }}<small>{{ 'settings.allowedTypesHint' | t }}</small></div>
              <div>
                <textarea rows="3" class="mono" [value]="v('allowed_upload_types')" (input)="set('allowed_upload_types', $event)" [placeholder]="defaultTypes"></textarea>
                <div class="pick-list">
                  @for (g of typePresets; track g.key) {
                    <button type="button" (click)="addTypes(g.exts)">+ {{ 'settings.types.' + g.key | t }}</button>
                  }
                </div>
              </div>
            </div>
          }

          @case ('email') {
            <div class="info-box">✉️ {{ 'mail.info' | t }}</div>
            <div class="form-row">
              <div class="form-label">{{ 'mail.preset' | t }}<small>{{ 'mail.presetHint' | t }}</small></div>
              <div class="pick-list">
                @for (p of mailPresets; track p.name) { <button type="button" (click)="applyPreset(p)">{{ p.name }}</button> }
              </div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'mail.host' | t }}<small>{{ 'mail.hostHint' | t }}</small></div>
              <div class="smtp-row">
                <input [value]="v('smtp_host')" placeholder="smtp.gmail.com" (input)="set('smtp_host', $event)" />
                <div class="contents">
                  <input type="number" [value]="v('smtp_port')" [placeholder]="portFor(v('smtp_encryption'))" (input)="set('smtp_port', $event)" [attr.aria-label]="'mail.port' | t" />
                  <select [value]="v('smtp_encryption') || 'starttls'" (change)="set('smtp_encryption', $event)" [attr.aria-label]="'mail.encryption' | t">
                    <option value="starttls">STARTTLS</option>
                    <option value="ssl">SSL/TLS</option>
                    <option value="none">{{ 'mail.noEnc' | t }}</option>
                  </select>
                </div>
              </div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'mail.account' | t }}<small>{{ 'mail.accountHint' | t }}</small></div>
              <div class="form-2">
                <input [value]="v('smtp_user')" autocomplete="off" [placeholder]="'mail.user' | t" (input)="set('smtp_user', $event)" />
                <input type="password" autocomplete="new-password" [value]="v('smtp_password')" [placeholder]="'mail.password' | t" (input)="set('smtp_password', $event)" />
              </div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'mail.from' | t }}<small>{{ 'mail.fromHint' | t }}</small></div>
              <div class="form-2">
                <input [value]="v('mail_from_name')" [placeholder]="v('site_title')" (input)="set('mail_from_name', $event)" />
                <input type="email" [value]="v('mail_from_email')" [placeholder]="v('admin_email') || 'no-reply@example.com'" (input)="set('mail_from_email', $event)" />
              </div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'mail.test' | t }}<small>{{ 'mail.testHint' | t }}</small></div>
              <div class="mail-test">
                <input type="email" [value]="testTo()" [placeholder]="v('admin_email') || 'you@example.com'" (input)="testTo.set($any($event.target).value)" />
                <button class="btn" [disabled]="testing()" (click)="sendTest()">{{ (testing() ? 'pw.sending' : 'mail.sendTest') | t }}</button>
              </div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'mail.uses' | t }}</div>
              <ul class="mail-uses">
                <li>🧩 {{ 'mail.use.plugins' | t }}</li>
                <li>🔑 {{ 'mail.use.reset' | t }}</li>
                <li>👋 {{ 'mail.use.welcome' | t }}</li>
              </ul>
            </div>
          }

          @case ('admin') {
            <div class="info-box warn">⚠️ {{ 'settings.adminPathWarn' | t }}</div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.adminPath' | t }}<small>{{ 'settings.adminPathHint' | t }}</small></div>
              <div>
                <div class="prefix-input"><span>{{ origin }}/</span><input [value]="v('admin_path') || 'admin'" (input)="setSlug('admin_path', $event)" /></div>
                @if (pathError()) { <p class="hint danger">{{ pathError() }}</p> }
                @else { <p class="hint">{{ 'settings.adminPathNew' | t }} <b class="mono">{{ origin }}/{{ v('admin_path') || 'admin' }}/</b></p> }
              </div>
            </div>
            <div class="form-row">
              <div class="form-label">{{ 'settings.version' | t }}</div>
              <div class="muted">{{ product() }}</div>
            </div>
          }

        }
        <div class="form-foot">
          <button class="btn" [disabled]="!dirty()" (click)="revert()">{{ 'settings.revert' | t }}</button>
          <button class="btn btn-primary" [disabled]="busy() || !dirty() || !!pathError()" (click)="save()">💾 {{ 'common.save' | t }}</button>
        </div>
      </section>
    </div>
    <rea-media-picker [open]="pickFor() !== null" (openChange)="!$event && pickFor.set(null)" (picked)="picked($event)" />
  `,
  styles: `.type-tpl-list { display: flex; flex-direction: column; gap: 10px; } .type-tpl-row { display: flex; flex-direction: column; gap: 4px; margin: 0; }`,
})
export class SettingsPage implements OnInit {
  readonly plugins = inject(Plugins);
  /** Post types of the site code: a single-post template each (option post_template_<type>). */
  readonly postTypes = inject(PostTypes);
  /** Types the site routes itself (a public: false type has no single-post template). */
  readonly routedTypes = computed(() => routedTypes(this.postTypes.list()));
  private i18n = inject(I18n);
  private api = inject(Api);
  private site = inject(SiteInfo);
  private route = inject(ActivatedRoute);
  private router = inject(Router);

  readonly origin = SITE_ORIGIN || location.origin;
  readonly timezones = TIMEZONES;
  readonly dateFormats = DATE_FORMATS;
  readonly defaultTypes = 'jpg,jpeg,png,gif,webp,svg,pdf,doc,docx,xls,xlsx,ppt,pptx,txt,csv,zip,mp4,mp3';
  readonly typePresets = [
    { key: 'images', exts: 'jpg,jpeg,png,gif,webp,svg,avif' },
    { key: 'docs', exts: 'pdf,doc,docx,xls,xlsx,ppt,pptx,txt,csv,odt,ods' },
    { key: 'archives', exts: 'zip,rar,7z' },
    { key: 'av', exts: 'mp4,webm,mp3,wav,ogg' },
  ];
  readonly tabs: { id: Tab; icon: string }[] = [
    { id: 'general', icon: '🏠' }, { id: 'reading', icon: '📖' }, { id: 'permalinks', icon: '🔗' },
    { id: 'media', icon: '📁' }, { id: 'email', icon: '✉️' }, { id: 'admin', icon: '🔐' },
  ];

  readonly mailPresets = [
    { name: 'Gmail / Google Workspace', host: 'smtp.gmail.com', port: '587', enc: 'starttls' },
    { name: 'Microsoft 365 / Outlook', host: 'smtp.office365.com', port: '587', enc: 'starttls' },
    { name: 'Zoho Mail', host: 'smtp.zoho.com', port: '465', enc: 'ssl' },
    { name: 'SendGrid', host: 'smtp.sendgrid.net', port: '587', enc: 'starttls' },
    { name: 'Amazon SES (Singapore)', host: 'email-smtp.ap-southeast-1.amazonaws.com', port: '587', enc: 'starttls' },
  ];
  readonly testTo = signal('');
  readonly testing = signal(false);
  readonly tab = signal<Tab>('general');
  readonly saved = signal<Settings>({});
  readonly draft = signal<Settings>({});
  /** Paths the installed plugins (and the core) reserve, from the server. */
  readonly reserved = signal<ReservedPath[]>([]);
  readonly pages = signal<Post[]>([]);
  readonly templates = signal<Post[]>([]);
  readonly product = signal('');
  readonly pickFor = signal<'site_icon' | 'site_logo' | null>(null);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly dirty = computed(() => {
    const a = this.saved(), b = this.draft();
    return Object.keys(b).some(k => (a[k] ?? '') !== (b[k] ?? ''));
  });
  readonly pathError = computed(() => {
    const p = this.draft()['admin_path'] ?? 'admin';
    // same length as the server (3–32)
    if (!/^[a-z0-9][a-z0-9-]{2,31}$/.test(p)) return this.i18n.t('settings.adminPathInvalid');
    const owner = reservedOwner(p, this.reserved());
    if (owner) return this.i18n.t('settings.adminPathPlugin', { id: owner });
    if (owner === '' || p === this.draft()['blog_base'] || p === this.draft()['category_base']) return this.i18n.t('settings.adminPathTaken');
    return '';
  });

  async ngOnInit() {
    const t = this.route.snapshot.queryParamMap.get('tab') as Tab | null;
    if (t && this.tabs.some(x => x.id === t)) this.tab.set(t);
    try {
      const s = await this.api.settings();
      this.saved.set(s);
      this.draft.set({ ...s });
    } catch (e) { this.error.set(errorMessage(e)); }
    this.api.posts({ type: 'page', perPage: 100 }).then(r => this.pages.set(r.items)).catch(() => undefined);
    this.api.posts({ type: 'template', perPage: 100 }).then(r => this.templates.set(r.items)).catch(() => undefined);
    this.api.publicSite().then(s => this.product.set(`${s.product} ${s.version}`)).catch(() => undefined);
    this.api.reservedPaths().then(r => this.reserved.set(r.items)).catch(() => undefined); // the server checks again on save
    void this.postTypes.ensureLoaded();
  }

  tplKey(type: string) { return templateOption(type); }
  typeName(pt: CodePostType) { this.i18n.locale(); return localized(pt.label) || pt.type; }
  prefix(pt: CodePostType) { return typePrefix(pt); }

  setTab(t: Tab) {
    this.tab.set(t);
    this.router.navigate([], { queryParams: { tab: t }, replaceUrl: true });
  }

  v(k: string) { return this.draft()[k] ?? ''; }
  put(k: string, value: string) { this.draft.update(d => ({ ...d, [k]: value })); }
  set(k: string, e: Event) { this.put(k, (e.target as HTMLInputElement).value); }
  check(e: Event) { return (e.target as HTMLInputElement).checked; }
  setSlug(k: string, e: Event) {
    const el = e.target as HTMLInputElement;
    const clean = el.value.toLowerCase().replace(/[^a-z0-9-]/g, '');
    if (clean !== el.value) el.value = clean;
    this.put(k, clean);
  }

  addTypes(exts: string) {
    const cur = (this.v('allowed_upload_types') || this.defaultTypes).split(',').map(s => s.trim()).filter(Boolean);
    for (const x of exts.split(',')) if (!cur.includes(x)) cur.push(x);
    this.put('allowed_upload_types', cur.join(','));
  }

  /** Renders a Go layout with today's date, e.g. "2 Jan 2006" → "13 Oct 2026". */
  sampleDate(f: string) {
    const d = new Date();
    const loc = this.i18n.locale() === 'th' ? 'th-TH' : 'en-US';
    const pad = (n: number) => String(n).padStart(2, '0');
    const map: Record<string, string> = {
      '2006': String(d.getFullYear()), January: d.toLocaleString(loc, { month: 'long' }), Jan: d.toLocaleString(loc, { month: 'short' }),
      '01': pad(d.getMonth() + 1), '02': pad(d.getDate()), '2': String(d.getDate()),
    };
    return f.replace(/2006|January|Jan|01|02|2/g, t => map[t]);
  }

  picked(m: Media) {
    const k = this.pickFor();
    if (k) this.put(k, m.url);
    this.pickFor.set(null);
  }

  portFor(enc: string) { return enc === 'ssl' ? '465' : enc === 'none' ? '25' : '587'; }

  applyPreset(p: { host: string; port: string; enc: string }) {
    this.put('smtp_host', p.host);
    this.put('smtp_port', p.port);
    this.put('smtp_encryption', p.enc);
  }

  /** Saves pending changes first, then sends a test email with them. */
  async sendTest() {
    this.testing.set(true);
    try {
      // the test must use what is on screen: stop if saving was refused or cancelled
      if (this.dirty() && !(await this.save(true))) return;
      const r = await this.api.testMail(this.testTo());
      Dialog.alert(this.i18n.t('mail.testOk'), r.sentTo, 'success');
    } catch (e) {
      Dialog.error(errorMessage(e), this.i18n.t('mail.testFail'));
    } finally {
      this.testing.set(false);
    }
  }

  revert() { this.draft.set({ ...this.saved() }); }

  /** Saves the changed settings; resolves false when nothing was saved (invalid or cancelled). */
  async save(quiet = false): Promise<boolean> {
    const pathErr = this.pathError();
    if (pathErr) {
      this.setTab('admin');
      Dialog.error(pathErr);
      return false;
    }
    const before = this.saved();
    const changed: Settings = {};
    for (const [k, val] of Object.entries(this.draft())) if ((before[k] ?? '') !== (val ?? '')) changed[k] = val;
    const pathChanged = 'admin_path' in changed;
    if (pathChanged && !(await Dialog.confirm({ title: this.i18n.t('settings.adminPathConfirm'), text: `${this.origin}/${changed['admin_path']}/`, icon: 'warning' }))) return false;
    this.busy.set(true);
    try {
      const s = await this.api.saveSettings(changed);
      this.saved.set(s);
      this.draft.set({ ...s });
      this.site.setTitle(s['site_title'] ?? '');
      this.site.setIcon(s['site_icon'] ?? '');
      this.site.logo.set(s['site_logo'] ?? '');
      if (pathChanged && location.port !== '4200') {
        await Dialog.alert(this.i18n.t('settings.adminPathMoved'), `${location.origin}/${s['admin_path']}/`, 'success');
        location.href = `/${s['admin_path']}/settings?tab=admin`;
        return true;
      }
      if (!quiet) Dialog.toast(this.i18n.t('common.saved'));
      return true;
    } catch (e) {
      if (quiet) throw e;
      Dialog.error(errorMessage(e));
      return false;
    } finally {
      this.busy.set(false);
    }
  }
}
