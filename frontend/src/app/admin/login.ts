import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { I18n, LOCALES, TPipe } from '../core/i18n';
import './messages-platform';
import { PRODUCT, SiteInfo } from '../core/site';

@Component({
  selector: 'rea-login',
  imports: [FormsModule, TPipe, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="login-page">
      <section class="login-pitch">
        @if (site.logo() || site.icon()) { <img class="logo logo-lg logo-img" [src]="site.logo() || site.icon()" alt="" /> } @else { <div class="logo logo-lg">◆</div> }
        <p class="login-brand">{{ site.title() || product }}</p>
        <h2>{{ 'login.pitch.title' | t }}</h2>
        <p>{{ 'login.pitch.body' | t }}</p>
      </section>
      <section class="login-panel">
        <div class="lang-switch login-lang">
          @for (l of locales; track l.id) {
            <button [class.on]="i18n.locale() === l.id" (click)="i18n.set(l.id)">{{ l.label }}</button>
          }
        </div>
        <form class="login-card" (ngSubmit)="submit()">
          <h1>{{ 'login.title' | t }}</h1>
          <p class="muted">{{ 'login.sub' | t }}</p>
          @if (site.demo(); as d) {
            <div class="demo-card">
              <b>🧪 {{ 'demo.loginTitle' | t }}</b>
              <span>{{ 'common.email' | t }}: <code>{{ d.email }}</code></span>
              <span>{{ 'common.password' | t }}: <code>{{ d.password }}</code></span>
              <button type="button" class="btn btn-primary btn-block" [disabled]="busy()" (click)="tryDemo(d.email, d.password)">{{ 'demo.loginBtn' | t }}</button>
              @if (d.everyMinutes) { <small>{{ 'demo.loginReset' | t: { n: d.everyMinutes } }}</small> }
            </div>
          }
          @if (error()) { <p class="alert">{{ error() }}</p> }
          <label>{{ 'common.email' | t }} <input name="email" type="email" [(ngModel)]="email" autocomplete="username" required /></label>
          <label>{{ 'common.password' | t }} <input name="password" type="password" [(ngModel)]="password" autocomplete="current-password" required /></label>
          <button class="btn btn-primary btn-block" [disabled]="busy()">{{ (busy() ? 'login.busy' : 'login.submit') | t }}</button>
          <p class="login-forgot"><a routerLink="/forgot">{{ 'pw.forgotLink' | t }}</a></p>
          <p class="login-foot muted small">{{ product }} {{ site.version() }}</p>
        </form>
      </section>
    </div>
  `,
})
export class Login {
  private auth = inject(Auth);
  private router = inject(Router);
  readonly i18n = inject(I18n);
  readonly locales = LOCALES;
  readonly site = inject(SiteInfo);
  readonly product = PRODUCT;

  email = '';
  password = '';
  readonly busy = signal(false);
  readonly error = signal('');

  tryDemo(email: string, password: string) {
    this.email = email;
    this.password = password;
    this.submit();
  }

  async submit() {
    this.busy.set(true);
    this.error.set('');
    try {
      await this.auth.login(this.email, this.password);
      this.router.navigateByUrl(this.auth.home());
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
