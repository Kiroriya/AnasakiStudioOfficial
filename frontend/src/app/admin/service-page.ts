import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { DomSanitizer, SafeResourceUrl } from '@angular/platform-browser';
import { ActivatedRoute } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { I18n, TPipe, localized } from '../core/i18n';
import { Plugins } from '../core/plugins';
import { PostTypes } from '../core/post-types';
import './messages-platform';
import './messages-content-types';

/** /x/code/<id> hosts the admin pages of the site code (site.adminPage). */
export const CODE_PAGES_ID = 'code';

/**
 * An admin screen of a service plugin (/x/:id/:page) — or of the site code
 * (/x/code/:page, site.adminPage): its own UI in a sandboxed iframe. The
 * iframe never shares the admin's origin, so that code can't read the admin
 * session; it is reached through a short-lived ticket.
 */
@Component({
  selector: 'rea-service-page',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">{{ icon() }}</span>
      <div class="page-head-text">
        <h1 [attr.id]="isCode() ? 'code-admin-title' : null">{{ title() }}</h1>
        <p>{{ isCode() ? ('svc.codePageSub' | t) : ('svc.pageSub' | t: { name: pluginName() }) }}</p>
      </div>
      <button class="btn" [attr.id]="isCode() ? 'code-admin-reload' : null" (click)="load()">↻ {{ 'svc.reload' | t }}</button>
    </header>
    @if (error()) { <p class="alert" [attr.id]="isCode() ? 'code-admin-error' : null">{{ error() }}</p> }
    @if (src(); as s) {
      @if (isCode()) {
        <iframe class="svc-frame" id="code-admin-frame" [src]="s" sandbox="allow-scripts allow-forms allow-popups allow-downloads" referrerpolicy="no-referrer" [title]="title()"></iframe>
      } @else {
        <iframe class="svc-frame" [src]="s" sandbox="allow-scripts allow-forms allow-popups allow-downloads allow-modals" referrerpolicy="no-referrer" [title]="title()"></iframe>
      }
    }
  `,
  styles: `.svc-frame { width: 100%; height: calc(100vh - 190px); min-height: 480px; border: 1px solid var(--border); border-radius: var(--r); background: #fff; }`,
})
export class ServicePage {
  private api = inject(Api);
  private i18n = inject(I18n);
  private plugins = inject(Plugins);
  private postTypes = inject(PostTypes);
  private sanitizer = inject(DomSanitizer);
  private route = inject(ActivatedRoute);

  readonly id = signal('');
  readonly page = signal('');
  readonly src = signal<SafeResourceUrl | null>(null);
  readonly error = signal('');
  readonly isCode = computed(() => this.id() === CODE_PAGES_ID);
  readonly codePage = computed(() => (this.isCode() ? this.postTypes.adminPages().find(p => p.id === this.page()) : undefined));
  readonly plugin = computed(() => this.plugins.list().find(p => p.id === this.id()));
  readonly item = computed(() => this.plugin()?.menu.find(m => m.path === `/x/${this.id()}/${this.page()}`));
  readonly pluginName = computed(() => this.plugin()?.name ?? this.id());
  readonly icon = computed(() => (this.isCode() ? this.codePage()?.icon || '🧰' : this.item()?.icon || '🛰'));
  readonly title = computed(() => {
    this.i18n.locale();
    if (this.isCode()) return localized(this.codePage()?.label) || this.page();
    const m = this.item();
    return (m && (this.i18n.locale() === 'en' && m.labelEn ? m.labelEn : m.label)) || this.page();
  });

  constructor() {
    this.route.paramMap.subscribe(pm => {
      this.id.set(pm.get('id') ?? '');
      this.page.set(pm.get('page') ?? '');
      this.load();
    });
  }

  async load() {
    this.error.set('');
    this.src.set(null);
    try {
      const { url } = this.isCode() ? await this.codeTicket() : await this.api.serviceTicket(this.id(), this.page());
      this.src.set(this.sanitizer.bypassSecurityTrustResourceUrl(url));
    } catch (e) {
      this.error.set(errorMessage(e));
    }
  }

  /** Ticket of a site.adminPage; the admin URL's query (?tab=x) becomes ctx.query. */
  private async codeTicket(): Promise<{ url: string }> {
    const qp = this.route.snapshot.queryParamMap;
    const query: Record<string, string> = {};
    for (const k of qp.keys) query[k] = qp.get(k) ?? '';
    try {
      return await this.api.codeAdminTicket(this.page(), query);
    } catch (e) {
      // a server that hosts code pages through the plugin ticket endpoint instead
      // (an unknown endpoint answers 404/405 without our JSON {error}; a missing page keeps its message)
      const err = e as HttpErrorResponse;
      const ours = !!(err?.error && typeof err.error === 'object' && typeof (err.error as { error?: unknown }).error === 'string');
      if ((err?.status === 404 || err?.status === 405) && !ours) return this.api.serviceTicket(CODE_PAGES_ID, this.page());
      throw e;
    }
  }
}
