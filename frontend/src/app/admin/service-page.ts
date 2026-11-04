import { ChangeDetectionStrategy, Component, ElementRef, computed, inject, signal } from '@angular/core';
import { HttpErrorResponse } from '@angular/common/http';
import { DomSanitizer, SafeResourceUrl } from '@angular/platform-browser';
import { ActivatedRoute, Router } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { I18n, TPipe, localized } from '../core/i18n';
import { Plugins } from '../core/plugins';
import { PostTypes } from '../core/post-types';
import { MediaPicker } from '../shared/media-picker';
import { FrameMediaPick, PAGE_MAX_HEIGHT, adminBase, clampHeight, readFrameMessage, sourceFrame } from './plugin-frame';
import './messages-platform';
import './messages-content-types';

/** The frame's CSS min-height (styles below): shorter pages keep the default frame. */
const FRAME_MIN_HEIGHT = 480;

/** /x/code/<id> hosts the admin pages of the site code (site.adminPage). */
export const CODE_PAGES_ID = 'code';

/** Site code pages are "<name>"; a plugin's code admin pages are "<plugin>:<name>". */
export function isPluginPageId(id: string, page: string): boolean {
  return id === CODE_PAGES_ID && page.includes(':');
}

/**
 * An admin screen of a service plugin (/x/:id/:page) — or of the site code
 * (/x/code/:page, site.adminPage): its own UI in a sandboxed iframe. The
 * iframe never shares the admin's origin, so that code can't read the admin
 * session; it is reached through a short-lived ticket.
 */
@Component({
  selector: 'rea-service-page',
  imports: [TPipe, MediaPicker],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '(window:message)': 'onFrameMessage($event)' },
  template: `
    <!-- Plugin pages (<plugin>:<page>) draw their own header with the kit; the admin's header would repeat it. -->
    @if (!isPluginPage()) {
      <header class="page-head">
        <span class="page-icon">{{ icon() }}</span>
        <div class="page-head-text">
          <h1 [attr.id]="isCode() ? 'code-admin-title' : null">{{ title() }}</h1>
          <p>{{ isCode() ? ('svc.codePageSub' | t) : ('svc.pageSub' | t: { name: pluginName() }) }}</p>
        </div>
        <button class="btn" [attr.id]="isCode() ? 'code-admin-reload' : null" (click)="load()">↻ {{ 'svc.reload' | t }}</button>
      </header>
    }
    @if (error()) {
      <p class="alert" [attr.id]="isCode() ? 'code-admin-error' : null">{{ error() }}
        @if (isPluginPage()) { <button class="btn btn-sm" id="code-admin-reload" (click)="load()">↻ {{ 'svc.reload' | t }}</button> }
      </p>
    }
    @if (src(); as s) {
      @if (isCode()) {
        <iframe class="svc-frame" id="code-admin-frame" [class.svc-frame-plugin]="isPluginPage()" [src]="s" [style.min-height.px]="contentHeight()" sandbox="allow-scripts allow-forms allow-popups allow-downloads" referrerpolicy="no-referrer" [title]="title()"></iframe>
      } @else {
        <iframe class="svc-frame" [src]="s" [style.min-height.px]="contentHeight()" sandbox="allow-scripts allow-forms allow-popups allow-downloads allow-modals" referrerpolicy="no-referrer" [title]="title()"></iframe>
      }
    }
    <rea-media-picker [kind]="mediaPick.pending()?.accept ?? 'image'" [open]="mediaPick.pending() !== null" (openChange)="!$event && mediaPick.done(null)" (picked)="mediaPick.done($event)" />
  `,
  styles: `.svc-frame { width: 100%; height: calc(100vh - 190px); min-height: 480px; border: 1px solid var(--border); border-radius: var(--r); background: #fff; }
    .svc-frame-plugin { height: calc(100vh - 110px); }
    .alert .btn { margin-left: 8px; }`,
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
  /** The page's own height (kit.js {type:'ak:height'}): the frame grows to it, the admin scrolls instead of the frame. */
  readonly contentHeight = signal<number | null>(null);
  private host = inject<ElementRef<HTMLElement>>(ElementRef);
  private router = inject(Router);
  private auth = inject(Auth);
  /** AK.pickMedia() of the page's frame: the admin's media library dialog. */
  readonly mediaPick = new FrameMediaPick();
  readonly isCode = computed(() => this.id() === CODE_PAGES_ID);
  /** A plugin's admin page (/x/code/<plugin>:<page>): it renders its own header (kit), so the admin shows none. */
  readonly isPluginPage = computed(() => isPluginPageId(this.id(), this.page()));
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

  /** Auto-height, AK.open() and AK.pickMedia() — only messages whose window is this page's iframe. */
  onFrameMessage(e: MessageEvent) {
    const frame = sourceFrame(this.host.nativeElement.querySelectorAll<HTMLIFrameElement>('iframe.svc-frame'), e.source);
    if (!frame) return;
    const m = readFrameMessage(e.data, adminBase());
    // + the frame's 1px top and bottom border (box-sizing: border-box): without them the page overflowed
    // by a pixel and the frame showed its own scrollbar (2.1.1)
    if (m?.type === 'height') this.contentHeight.set(clampHeight(m.height + 2, FRAME_MIN_HEIGHT, PAGE_MAX_HEIGHT));
    else if (m?.type === 'navigate') this.router.navigateByUrl(m.route);
    else if (m?.type === 'pick-media') this.mediaPick.request(frame, m.reqId, m.accept, this.auth.can('upload_files'));
  }

  async load() {
    this.error.set('');
    this.mediaPick.done(null);
    this.src.set(null);
    this.contentHeight.set(null);
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
