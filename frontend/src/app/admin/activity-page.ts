import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { I18n, TPipe, tr } from '../core/i18n';
import { AuditEntry } from '../core/models';
import { Pager } from '../shared/pager';
import './messages-activity';

/** Object names of the API's first path segment (GET /audit routes). */
const OBJECTS: Record<string, string> = {
  posts: 'act.obj.posts', users: 'act.obj.users', media: 'act.obj.media', menus: 'act.obj.menus', components: 'act.obj.components',
  terms: 'act.obj.terms', settings: 'act.obj.settings', 'package-plugins': 'act.obj.plugins', plugins: 'act.obj.plugins',
  'plugin-settings': 'act.obj.plugins', roles: 'act.obj.roles', code: 'act.obj.code', auth: 'act.obj.account', tools: 'act.obj.tools',
  design: 'act.obj.design', links: 'act.obj.links', backups: 'act.obj.tools', 'content-types': 'act.obj.contentTypes',
};

/**
 * One line's action in words: "Edited · Content", "Signed in", "Sign-in failed"… (the route itself is shown
 * under it, so the details stay visible).
 */
export function actionText(e: Pick<AuditEntry, 'method' | 'route' | 'status' | 'via'>, t: (k: string) => string): string {
  if (e.via === 'login') {
    return t(e.status === 200 ? 'act.login' : e.status === 429 ? 'act.loginLocked' : e.status === 403 ? 'act.loginSuspended' : 'act.loginFailed');
  }
  const path = (e.route.split(' ')[1] ?? '').replace(/^\/api\/v1\//, '');
  const seg = path.split('/')[0];
  const last = path.split('/').filter(x => x && !x.startsWith('{')).pop() ?? '';
  const special: Record<string, string> = {
    logout: 'act.logout', activate: 'act.activate', deactivate: 'act.deactivate', install: 'act.install', upload: 'act.upload',
    publish: 'act.publish', restore: 'act.restore', duplicate: 'act.duplicate', import: 'act.import', bulk: 'act.bulk',
  };
  const verb = special[last] ?? (e.method === 'DELETE' ? 'act.delete' : e.method === 'POST' ? 'act.create' : 'act.update');
  return t(verb) + ' · ' + t(OBJECTS[seg] ?? 'act.obj.other');
}

/** Settings → Activity log (2.4.0): who changed what and when, sign-ins included (administrators). */
@Component({
  selector: 'rea-activity',
  imports: [TPipe, Pager],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">📜</span>
      <div class="page-head-text"><h1>{{ 'nav.activity' | t }}</h1><p>{{ 'act.desc' | t }}</p></div>
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }
    <section class="card table-card">
      <div class="table-toolbar">
        <input class="search" id="act-search" [placeholder]="'act.searchPh' | t" [value]="q()" (input)="q.set(val($event))" (keyup.enter)="go(1)" />
        <select id="act-kind" [value]="kind()" (change)="kind.set(val($event)); go(1)">
          <option value="">{{ 'act.kindAll' | t }}</option>
          @for (k of kinds; track k[0]) { <option [value]="k[0]">{{ k[1] | t }}</option> }
        </select>
        <label class="check"><input type="checkbox" id="act-failed" [checked]="failed()" (change)="failed.set(checked($event)); go(1)" /> {{ 'act.failedOnly' | t }}</label>
        <span class="grow"></span>
        <span class="muted small">{{ 'act.kept' | t }}</span>
      </div>
      <div class="table-wrap">
        <table class="table" id="act-table">
          <thead><tr><th>{{ 'act.when' | t }}</th><th>{{ 'act.who' | t }}</th><th>{{ 'act.what' | t }}</th><th>{{ 'act.target' | t }}</th><th>{{ 'act.result' | t }}</th><th>IP</th></tr></thead>
          <tbody>
            @for (e of items(); track e.id) {
              <tr class="act-row" [attr.data-route]="e.route" [attr.data-status]="e.status">
                <td class="nowrap">{{ i18n.date(e.at, true) }}</td>
                <td>{{ e.userName || ('act.unknown' | t) }} @if (e.via === 'token') { <span class="badge" [title]="'act.tokenHint' | t">API</span> }</td>
                <td><b>{{ action(e) }}</b><br /><span class="mono small muted">{{ e.route }}</span></td>
                <td>{{ e.label }} @if (e.target) { <span class="mono small muted">#{{ e.target }}</span> }</td>
                <td><span class="badge" [class.badge-live]="e.status < 400" [class.badge-danger]="e.status >= 400">{{ e.status < 400 ? ('act.ok' | t) : e.status }}</span></td>
                <td class="mono small">{{ e.ip }}</td>
              </tr>
            } @empty { <tr><td colspan="6" class="muted">{{ 'act.none' | t }}</td></tr> }
          </tbody>
        </table>
      </div>
      <rea-pager [page]="page()" [total]="total()" [perPage]="perPage" (pageChange)="go($event)" />
    </section>
  `,
})
export class ActivityPage {
  private api = inject(Api);
  readonly i18n = inject(I18n);
  readonly perPage = 50;
  readonly items = signal<AuditEntry[]>([]);
  readonly total = signal(0);
  readonly page = signal(1);
  readonly q = signal('');
  readonly kind = signal('');
  readonly failed = signal(false);
  readonly error = signal('');
  readonly kinds: [string, string][] = [
    ['auth', 'act.obj.account'], ['posts', 'act.obj.posts'], ['users', 'act.obj.users'], ['media', 'act.obj.media'],
    ['settings', 'act.obj.settings'], ['package-plugins', 'act.obj.plugins'], ['code', 'act.obj.code'], ['roles', 'act.obj.roles'],
  ];

  constructor() { void this.go(1); }

  async go(page: number) {
    try {
      const r = await this.api.audit({ page, perPage: this.perPage, q: this.q().trim() || undefined, kind: this.kind() || undefined, failed: this.failed() ? 1 : undefined });
      this.items.set(r.items);
      this.total.set(r.total);
      this.page.set(page);
      this.error.set('');
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  action(e: AuditEntry) { return actionText(e, tr); }
  val(e: Event) { return (e.target as HTMLInputElement).value; }
  checked(e: Event) { return (e.target as HTMLInputElement).checked; }
}
