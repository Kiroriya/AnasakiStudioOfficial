import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { I18n, LOCALES, TPipe } from '../core/i18n';
import { PRODUCT, SiteInfo } from '../core/site';
import './messages-platform';

/** "Forgot password?" — asks for the email and sends a reset link. */
@Component({
  selector: 'rea-forgot',
  imports: [FormsModule, TPipe, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="setup-page">
      <form class="setup-card pw-card" (ngSubmit)="submit()">
        <header class="setup-head">
          @if (site.logo() || site.icon()) { <img class="logo logo-img" [src]="site.logo() || site.icon()" alt="" /> } @else { <div class="logo">◆</div> }
          <div><b>{{ site.title() || product }}</b></div>
          <span class="grow"></span>
          <div class="lang-switch">@for (l of locales; track l.id) { <button type="button" [class.on]="i18n.locale() === l.id" (click)="i18n.set(l.id)">{{ l.short }}</button> }</div>
        </header>
        <h1>{{ 'pw.forgotTitle' | t }}</h1>
        @if (sent()) {
          <div class="notice">✉️ {{ sent() }}</div>
          <p class="hint">{{ 'pw.checkSpam' | t }}</p>
        } @else {
          <p class="muted">{{ 'pw.forgotSub' | t }}</p>
          @if (error()) { <p class="alert">{{ error() }}</p> }
          <label>{{ 'common.email' | t }} <input name="email" type="email" [(ngModel)]="email" autocomplete="username" required /></label>
          <button class="btn btn-primary btn-block" [disabled]="busy() || !email">{{ (busy() ? 'pw.sending' : 'pw.sendLink') | t }}</button>
        }
        <p class="pw-foot"><a routerLink="/login">← {{ 'pw.backToLogin' | t }}</a></p>
      </form>
    </div>
  `,
})
export class ForgotPassword {
  readonly i18n = inject(I18n);
  readonly site = inject(SiteInfo);
  private api = inject(Api);
  readonly product = PRODUCT;
  readonly locales = LOCALES;
  email = '';
  readonly busy = signal(false);
  readonly sent = signal('');
  readonly error = signal('');

  async submit() {
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.forgotPassword(this.email.trim());
      this.sent.set(this.i18n.t('pw.sent'));
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}

/** Set a new password from the emailed link (also used by the welcome email). */
@Component({
  selector: 'rea-reset',
  imports: [FormsModule, TPipe, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="setup-page">
      <form class="setup-card pw-card" (ngSubmit)="submit()">
        <header class="setup-head">
          @if (site.logo() || site.icon()) { <img class="logo logo-img" [src]="site.logo() || site.icon()" alt="" /> } @else { <div class="logo">◆</div> }
          <div><b>{{ site.title() || product }}</b></div>
        </header>
        <h1>{{ (welcome ? 'pw.welcomeTitle' : 'pw.resetTitle') | t }}</h1>
        <p class="muted">{{ 'pw.resetSub' | t }}</p>
        @if (!token) { <p class="alert">{{ 'pw.noToken' | t }}</p> }
        @if (error()) { <p class="alert">{{ error() }}</p> }
        <label>{{ 'profile.newPw' | t }} <input name="pw" type="password" [(ngModel)]="password" autocomplete="new-password" required /></label>
        <label>{{ 'setup.pwConfirm' | t }} <input name="pw2" type="password" [(ngModel)]="confirm" autocomplete="new-password" required /></label>
        <button class="btn btn-primary btn-block" [disabled]="busy() || !token">{{ 'pw.save' | t }}</button>
        <p class="pw-foot"><a routerLink="/forgot">{{ 'pw.newLink' | t }}</a></p>
      </form>
    </div>
  `,
})
export class ResetPassword {
  readonly site = inject(SiteInfo);
  private i18n = inject(I18n);
  private api = inject(Api);
  private auth = inject(Auth);
  private router = inject(Router);
  private q = inject(ActivatedRoute).snapshot.queryParamMap;
  readonly product = PRODUCT;
  readonly token = this.q.get('token') ?? '';
  readonly welcome = this.q.get('welcome') === '1';
  password = '';
  confirm = '';
  readonly busy = signal(false);
  readonly error = signal('');

  async submit() {
    if (this.password.length < 8) { this.error.set(this.i18n.t('users.pwMin')); return; }
    if (this.password !== this.confirm) { this.error.set(this.i18n.t('setup.err.confirm')); return; }
    this.busy.set(true);
    this.error.set('');
    try {
      this.auth.accept(await this.api.resetPassword(this.token, this.password));
      this.router.navigateByUrl(this.auth.home());
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
