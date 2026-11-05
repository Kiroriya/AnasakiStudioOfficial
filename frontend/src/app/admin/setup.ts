import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { Auth, SetupState } from '../core/auth';
import { I18n, LOCALES, Locale, TPipe, localized, tr } from '../core/i18n';
import { PRODUCT, SiteInfo } from '../core/site';
import { RecommendedInstall, RecommendedItem, defaultSelection, installOutcome, selectionIds } from '../core/legacy';
import { scopeLabel, sortScopes } from './plugin-deps';
import './messages-platform';
import './messages-plugins4b';
import './messages-phase7';

type Step = 0 | 1 | 2 | 3 | 4;

/** "Asks for" line of a recommended plugin: permissions then code scopes, in words. */
export function recommendedPerms(r: Pick<RecommendedItem, 'permissions' | 'scopes'>): string {
  const perms = (r.permissions ?? []).filter(p => p !== 'code').map(p => { const k = 'perm.' + p, t = tr(k); return t === k ? p : t; });
  return [...perms, ...sortScopes(r.scopes).map(scopeLabel)].join(' · ');
}

/**
 * First-run wizard: language → site → admin account → finish (like WordPress' famous 5-minute install),
 * then (phase 7) "Recommended plugins" from the Store. The site is created before that step, so an
 * unreachable Store never blocks the setup: the wizard says "install later from the Plugins page".
 */
@Component({
  selector: 'rea-setup',
  imports: [FormsModule, TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="setup-page">
      <div class="setup-card">
        <header class="setup-head">
          <div class="logo">◆</div>
          <div><b>{{ product }}</b><small class="muted">{{ 'setup.kicker' | t }}</small></div>
          <span class="grow"></span>
          <div class="lang-switch">
            @for (l of locales; track l.id) {
              <button [class.on]="i18n.locale() === l.id" (click)="setLocale(l.id)">{{ l.short }}</button>
            }
          </div>
        </header>

        <ol class="setup-steps">
          @for (s of steps; track s; let i = $index) {
            <li [class.on]="step() === i" [class.done]="step() > i || (i === 4 && !!recResult())"><span>{{ step() > i ? '✓' : i + 1 }}</span>{{ 'setup.step.' + s | t }}</li>
          }
        </ol>

        @if (error()) { <p class="alert">{{ error() }}</p> }

        @switch (step()) {
          @case (0) {
            <section class="setup-body">
              <h1>{{ 'setup.welcome' | t }}</h1>
              <p class="muted">{{ 'setup.welcomeSub' | t }}</p>
              <p class="form-label">{{ 'setup.adminLang' | t }}<small>{{ 'setup.adminLangHint' | t }}</small></p>
              <div class="lang-cards">
                @for (l of locales; track l.id) {
                  <button type="button" class="lang-card" [class.on]="i18n.locale() === l.id" (click)="setLocale(l.id)"><b>{{ l.short }}</b><span>{{ l.label }}</span></button>
                }
              </div>
              <ul class="setup-checks">
                <li>✅ {{ 'setup.check.db' | t }}</li>
                <li>✅ {{ 'setup.check.version' | t: { v: version() } }}</li>
              </ul>
              @if (tokenRequired()) {
                <label>{{ 'setup.token' | t }}
                  <input name="setupToken" id="setup-token" type="password" autocomplete="off" [ngModel]="setupToken()" (ngModelChange)="setupToken.set($event)" />
                </label>
                <p class="hint">{{ 'setup.tokenHint' | t }}</p>
              }
            </section>
          }
          @case (1) {
            <section class="setup-body">
              <h1>{{ 'setup.siteTitle' | t }}</h1>
              <p class="muted">{{ 'setup.siteSub' | t }}</p>
              <label>{{ 'settings.siteTitle' | t }} <input name="siteTitle" [(ngModel)]="siteTitle" [placeholder]="'setup.siteTitlePh' | t" autofocus /></label>
              <label>{{ 'settings.tagline' | t }} <span class="muted small">{{ 'common.optional' | t }}</span><input name="tagline" [(ngModel)]="tagline" [placeholder]="'setup.taglinePh' | t" /></label>
              <label>{{ 'settings.contentLang' | t }}
                <select name="language" [(ngModel)]="language">
                  <option value="th">ไทย (th)</option>
                  <option value="en">English (en)</option>
                </select>
              </label>
              <label class="check"><input type="checkbox" name="demo" [(ngModel)]="demo" /> {{ 'setup.demo' | t }}</label>
              <p class="hint">{{ 'setup.demoHint' | t }}</p>
            </section>
          }
          @case (2) {
            <section class="setup-body">
              <h1>{{ 'setup.adminTitle' | t }}</h1>
              <p class="muted">{{ 'setup.adminSub' | t }}</p>
              <label>{{ 'users.displayName' | t }} <input name="adminName" [(ngModel)]="adminName" autocomplete="name" /></label>
              <label>{{ 'common.email' | t }} <input name="adminEmail" type="email" [(ngModel)]="adminEmail" autocomplete="username" /></label>
              <label>{{ 'common.password' | t }}
                <div class="pw-row">
                  <input name="adminPassword" [type]="showPw() ? 'text' : 'password'" [ngModel]="password()" (ngModelChange)="password.set($event)" autocomplete="new-password" />
                  <button type="button" class="btn btn-sm" (click)="showPw.set(!showPw())">{{ (showPw() ? 'users.hide' : 'users.show') | t }}</button>
                </div>
              </label>
              <div class="pw-meter" [attr.data-level]="strength()"><span></span><span></span><span></span><span></span></div>
              <p class="hint">{{ 'setup.pw.' + strength() | t }}</p>
              <label>{{ 'setup.pwConfirm' | t }} <input name="confirm" type="password" [(ngModel)]="confirm" autocomplete="new-password" /></label>
            </section>
          }
          @case (4) {
            <section id="setup-recommended" class="setup-body">
              <h1>{{ 'setup.rec.title' | t }}</h1>
              <p class="muted">{{ 'setup.rec.sub' | t }}</p>
              @if (recLoading()) { <p class="muted">{{ 'setup.rec.loading' | t }}</p> }
              @if (recStoreDown()) { <p class="alert setup-rec-store">{{ 'setup.rec.storeDown' | t }}</p> }
              @else if (!recLoading() && !recItems().length) { <p class="muted setup-rec-none">{{ 'setup.rec.none' | t }}</p> }
              <ul class="setup-rec-list">
                @for (r of recItems(); track r.id) {
                  <li>
                    <label class="setup-rec" [attr.data-id]="r.id" [class.off]="r.installed || recStoreDown()">
                      <input type="checkbox" [checked]="r.installed || recSelected().has(r.id)" [disabled]="r.installed || recBusy() || !!recResult()" (change)="toggleRec(r.id, $event)" />
                      <span class="setup-rec-text">
                        <b>{{ r.icon }} {{ name(r) }}</b>
                        <small>{{ desc(r) }}</small>
                        @if (perms(r); as p) { <span class="setup-rec-perms">{{ 'setup.rec.perms' | t }}: {{ p }}</span> }
                        @if (r.installed) { <span class="setup-rec-perms">✓ {{ 'setup.rec.installed' | t }}</span> }
                        @else if (!r.available && !recLoading() && !recStoreDown()) { <span class="setup-rec-perms">{{ 'setup.rec.notInStore' | t }}</span> }
                        @if (resultOf(r.id); as x) {
                          <span class="setup-rec-perms setup-rec-result" [attr.data-ok]="x.ok">{{ x.ok ? ('setup.rec.ok' | t: { v: x.version ?? '' }) : '✗ ' + x.error }}</span>
                        }
                      </span>
                    </label>
                  </li>
                }
              </ul>
              @if (recResult(); as res) {
                @switch (outcome(res)) {
                  @case ('ok') { <p class="notice setup-rec-outcome">✓ {{ 'setup.rec.done' | t }}</p> }
                  @case ('partial') { <p class="alert setup-rec-outcome">{{ 'setup.rec.partial' | t }}</p> }
                  @case ('store') { <p class="alert setup-rec-outcome">{{ 'setup.rec.storeDown' | t }}</p> }
                }
              }
            </section>
          }
          @case (3) {
            <section class="setup-body">
              <h1>{{ 'setup.reviewTitle' | t }}</h1>
              <p class="muted">{{ 'setup.reviewSub' | t }}</p>
              <dl class="setup-review">
                <dt>{{ 'settings.siteTitle' | t }}</dt><dd>{{ siteTitle }}</dd>
                <dt>{{ 'settings.tagline' | t }}</dt><dd>{{ tagline || '—' }}</dd>
                <dt>{{ 'settings.contentLang' | t }}</dt><dd>{{ language === 'th' ? 'ไทย' : 'English' }}</dd>
                <dt>{{ 'setup.step.admin' | t }}</dt><dd>{{ adminName }} · {{ adminEmail }}</dd>
                <dt>{{ 'setup.adminLang' | t }}</dt><dd>{{ localeLabel() }}</dd>
                <dt>{{ 'setup.demoShort' | t }}</dt><dd>{{ (demo ? 'common.yes' : 'setup.no') | t }}</dd>
              </dl>
            </section>
          }
        }

        <footer class="setup-foot">
          @if (step() > 0 && step() < 4) { <button class="btn" (click)="back()">← {{ 'common.back' | t }}</button> }
          <span class="grow"></span>
          @if (step() < 3) {
            <button class="btn btn-primary" (click)="next()">{{ 'setup.next' | t }} →</button>
          } @else if (step() === 3) {
            <button class="btn btn-primary" [disabled]="busy()" (click)="finish()">🚀 {{ (busy() ? 'setup.installing' : 'setup.install') | t }}</button>
          } @else if (recResult() || recStoreDown() || (!recLoading() && !toInstall().length)) {
            <button id="setup-rec-done" class="btn btn-primary" (click)="done()">{{ 'setup.rec.finish' | t }}</button>
          } @else {
            <button id="setup-rec-skip" class="btn" [disabled]="recBusy()" (click)="done()">{{ 'setup.rec.skip' | t }}</button>
            <button id="setup-rec-install" class="btn btn-primary" [disabled]="recBusy() || recLoading()" (click)="installRecommended()">
              {{ recBusy() ? ('setup.rec.installing' | t) : ('setup.rec.install' | t: { n: toInstall().length }) }}
            </button>
          }
        </footer>
      </div>
    </div>
  `,
})
export class Setup {
  readonly i18n = inject(I18n);
  /** The chosen admin language's own name (ไทย, English, 日本語, 中文, 한국어). */
  localeLabel() { return LOCALES.find(l => l.id === this.i18n.locale())?.label ?? this.i18n.locale(); }
  private api = inject(Api);
  private auth = inject(Auth);
  private state = inject(SetupState);
  private site = inject(SiteInfo);
  private router = inject(Router);

  readonly product = PRODUCT;
  readonly locales = LOCALES;
  readonly steps = ['language', 'site', 'admin', 'finish', 'plugins'];
  readonly step = signal<Step>(0);
  readonly version = signal('');
  /** The server set SETUP_TOKEN: the wizard asks for it (audit M-5). */
  readonly tokenRequired = signal(false);
  readonly setupToken = signal('');
  readonly busy = signal(false);
  readonly error = signal('');
  readonly showPw = signal(false);
  readonly password = signal('');
  // step 4: recommended plugins (after the site exists)
  readonly recItems = signal<RecommendedItem[]>([]);
  readonly recSelected = signal<Set<string>>(new Set());
  readonly recLoading = signal(false);
  readonly recStoreDown = signal(false);
  readonly recBusy = signal(false);
  readonly recResult = signal<RecommendedInstall | null>(null);
  readonly toInstall = computed(() => selectionIds(this.recItems(), this.recSelected()));
  siteTitle = '';
  tagline = '';
  language = this.i18n.locale();
  demo = true;
  adminName = '';
  adminEmail = '';
  confirm = '';

  /** 0 = too short, 1 = weak, 2 = okay, 3 = strong. */
  readonly strength = computed(() => {
    const p = this.password();
    if (p.length < 8) return 0;
    let score = 0;
    if (/[a-z]/.test(p) && /[A-Z]/.test(p)) score++;
    if (/\d/.test(p)) score++;
    if (/[^A-Za-z0-9]/.test(p)) score++;
    if (p.length >= 12) score++;
    return score >= 3 ? 3 : score >= 2 ? 2 : 1;
  });

  constructor() {
    this.api.setupStatus().then(s => { this.version.set(s.version ?? ''); this.tokenRequired.set(!!s.tokenRequired); }).catch(() => undefined);
    this.site.setTitle('');
  }

  setLocale(l: Locale) {
    this.i18n.set(l);
    this.language = l;
  }

  name(r: RecommendedItem) { this.i18n.locale(); return localized(r.name); }
  desc(r: RecommendedItem) { this.i18n.locale(); return localized(r.description); }
  perms(r: RecommendedItem) { this.i18n.locale(); return recommendedPerms(r); }
  outcome(r: RecommendedInstall) { return installOutcome(r); }
  resultOf(id: string) { return this.recResult()?.results.find(x => x.id === id) ?? null; }

  async loadRecommended() {
    this.recLoading.set(true);
    try {
      const list = await this.api.setupRecommended();
      this.recItems.set(list.items);
      this.recSelected.set(list.storeAvailable ? defaultSelection(list.items.filter(i => i.available)) : new Set());
      this.recStoreDown.set(!list.storeAvailable);
    } catch {
      this.recStoreDown.set(true);
    } finally {
      this.recLoading.set(false);
    }
  }

  toggleRec(id: string, e: Event) {
    const on = (e.target as HTMLInputElement).checked;
    this.recSelected.update(s => { const n = new Set(s); if (on) n.add(id); else n.delete(id); return n; });
  }

  async installRecommended() {
    this.recBusy.set(true);
    try {
      const res = await this.api.setupInstallRecommended(this.toInstall());
      this.recResult.set(res);
      if (!res.storeAvailable) this.recStoreDown.set(true);
    } catch (e) {
      this.recResult.set({ storeAvailable: false, results: [] });
      this.recStoreDown.set(true);
      this.error.set(errorMessage(e));
    } finally {
      this.recBusy.set(false);
    }
  }

  done() { this.router.navigateByUrl('/?welcome=1'); }

  back() {
    this.error.set('');
    this.step.update(s => (s - 1) as Step);
  }

  next() {
    const err = this.validate(this.step());
    this.error.set(err);
    if (!err) this.step.update(s => (s + 1) as Step);
  }

  private validate(step: Step): string {
    const t = (k: string) => this.i18n.t(k);
    if (step === 0 && this.tokenRequired() && !this.setupToken().trim()) return t('setup.err.token');
    if (step === 1 && !this.siteTitle.trim()) return t('setup.err.title');
    if (step === 2) {
      if (!this.adminName.trim()) return t('setup.err.name');
      if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(this.adminEmail)) return t('setup.err.email');
      if (this.password().length < 8) return t('users.pwMin');
      if (this.password() !== this.confirm) return t('setup.err.confirm');
    }
    return '';
  }

  async finish() {
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.runSetup({
        siteTitle: this.siteTitle.trim(), tagline: this.tagline.trim(), language: this.language,
        adminName: this.adminName.trim(), adminEmail: this.adminEmail.trim(), adminPassword: this.password(),
        locale: this.i18n.locale(), demo: this.demo,
        ...(this.tokenRequired() ? { setupToken: this.setupToken().trim() } : {}),
      });
      this.state.done();
      this.auth.accept(res);
      // the site exists now: offer the recommended plugins (never blocks the setup)
      this.step.set(4);
      void this.loadRecommended();
    } catch (e) {
      this.error.set(errorMessage(e));
      // a wrong setup token: back to the first step, where it is entered
      if ((e as { error?: { setupToken?: boolean } })?.error?.setupToken) this.step.set(0);
    } finally {
      this.busy.set(false);
    }
  }
}
