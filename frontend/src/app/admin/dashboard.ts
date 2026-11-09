import { ChangeDetectionStrategy, Component, ElementRef, OnInit, computed, inject, signal } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { Router, RouterLink } from '@angular/router';
import { DomSanitizer, SafeResourceUrl } from '@angular/platform-browser';
import { DashboardWidget, normalizeWidgets } from '../core/admin-ext';
import { Api, errorMessage } from '../core/api';
import { Auth, Cap } from '../core/auth';
import { Dialog } from '../core/dialog';
import { I18n, TPipe, localized } from '../core/i18n';
import { Post, Settings, Stats } from '../core/models';
import { Plugins } from '../core/plugins';
import { starterPage } from '../editor/presets';
import { MediaPicker } from '../shared/media-picker';
import { FrameMediaPick, WIDGET_MAX_HEIGHT, WIDGET_MIN_HEIGHT, adminBase, clampHeight, readFrameMessage, sourceFrame } from './plugin-frame';
import './messages-v250';

interface Action { icon: string; key: string; cap?: Cap; show?: () => boolean; run: () => void }

/** 2.5.3: the user's arrangement of the plugin widgets (ids are "<plugin>:<widget>"). */
export type DashSize = 'auto' | 'normal' | 'wide';
export interface DashLayout { order: string[]; hidden: string[]; size: Record<string, DashSize> }
function emptyLayout(): DashLayout { return { order: [], hidden: [], size: {} }; }
/** Whatever is stored, a well-formed layout (unknown ids are harmless: they just match no widget). */
export function readLayout(v: unknown): DashLayout {
  const o = (v && typeof v === 'object' ? v : {}) as Record<string, unknown>;
  const ids = (x: unknown) => Array.isArray(x) ? x.filter((s): s is string => typeof s === 'string').slice(0, 500) : [];
  const size: Record<string, DashSize> = {};
  if (o['size'] && typeof o['size'] === 'object') for (const [k, s] of Object.entries(o['size'] as object)) if (s === 'normal' || s === 'wide') size[k] = s;
  return { order: ids(o['order']), hidden: ids(o['hidden']), size };
}
/** The widgets in the user's order (ones never placed, e.g. from a plugin installed later, keep their registered
 *  order after the placed ones), split into shown and hidden. */
export function arrange<T extends { id: string }>(widgets: T[], l: DashLayout): { shown: T[]; hidden: T[] } {
  const pos = new Map(l.order.map((id, i) => [id, i]));
  const all = widgets.map((w, i) => ({ w, k: pos.get(w.id) ?? 100000 + i })).sort((a, b) => a.k - b.k).map(x => x.w);
  const h = new Set(l.hidden);
  return { shown: all.filter(w => !h.has(w.id)), hidden: all.filter(w => h.has(w.id)) };
}
/** ids with id moved by one place (-1 up, 1 down); unchanged at the ends. */
export function moveId(ids: string[], id: string, by: number): string[] {
  const out = [...ids], i = out.indexOf(id), j = i + by;
  if (i < 0 || j < 0 || j >= out.length) return out;
  [out[i], out[j]] = [out[j], out[i]];
  return out;
}
/** ids with from dropped on to: it takes to's place (after it when moving down, before it when moving up). */
export function dropId(ids: string[], from: string, to: string): string[] {
  if (from === to || !ids.includes(from) || !ids.includes(to)) return [...ids];
  const down = ids.indexOf(from) < ids.indexOf(to);
  const out = ids.filter(x => x !== from);
  out.splice(out.indexOf(to) + (down ? 1 : 0), 0, from);
  return out;
}

@Component({
  selector: 'rea-dashboard',
  imports: [RouterLink, NgTemplateOutlet, TPipe, MediaPicker],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '(window:message)': 'onFrameMessage($event)' },
  template: `
    <section class="welcome">
      <div>
        <h1>{{ 'dash.hello' | t: { name: greetName() } }}</h1>
        <p>{{ 'dash.sub' | t }}</p>
      </div>
      <div class="welcome-actions">
        @if (auth.can('edit_pages')) { <button class="btn btn-light" (click)="editHome()">🏠 {{ 'dash.a.home' | t }}</button> }
        @if (auth.can('edit_own_posts')) { <a class="btn btn-white" routerLink="/posts/new">✍️ {{ 'dash.a.write' | t }}</a> }
      </div>
    </section>
    @if (error()) { <p class="alert">{{ error() }}</p> }
    @if (pending() > 0) {
      <a class="alert alert-info dash-pending" id="dash-pending" [routerLink]="['/posts']" [queryParams]="{ status: 'pending' }">📨 {{ 'dash.pending' | t }}: <b>{{ pending() }}</b> →</a>
    }

    @if (stats(); as s) {
      <div class="stat-grid">
        @for (c of statCards; track c.key) {
          <ng-template #body><span class="stat-icon" [class]="c.color">{{ c.icon }}</span><span><b>{{ s[c.key] }}</b><small>{{ 'dash.stat.' + c.key | t }}</small></span></ng-template>
          <!-- Only a link when this account may open the list; otherwise just the number. -->
          @if (auth.can(c.cap)) { <a class="stat-card" [routerLink]="c.path"><ng-container [ngTemplateOutlet]="body" /></a> }
          @else { <div class="stat-card"><ng-container [ngTemplateOutlet]="body" /></div> }
        }
      </div>
    }


    @if (widgets().length) {
      <div class="dash-bar" id="dash-bar">
        <h2 class="section-title">{{ 'dash.widgets' | t }}
          @if (!editing() && hiddenCount()) { <small class="muted">· {{ 'dash.hiddenCount' | t: { n: hiddenCount() } }}</small> }</h2>
        @if (editing()) {
          <button class="btn btn-sm" id="dash-reset" (click)="resetLayout()">↺ {{ 'dash.reset' | t }}</button>
          <button class="btn btn-primary btn-sm" id="dash-done" (click)="editing.set(false)">✓ {{ 'dash.customizeDone' | t }}</button>
        } @else {
          <button class="btn btn-sm" id="dash-customize" (click)="editing.set(true)">⚙ {{ 'dash.customize' | t }}</button>
        }
      </div>
      @if (editing()) {
        <p class="hint dash-hint">{{ 'dash.customizeHint' | t }}</p>
        @if (hiddenWidgets().length) {
          <div class="dash-hidden" id="dash-hidden"><b>{{ 'dash.hidden' | t }}:</b>
            @for (w of hiddenWidgets(); track w.id) {
              <button class="chip" [attr.data-show]="w.id" (click)="setHidden(w.id, false)">{{ w.icon || '🧩' }} {{ label(w) }} <span>＋ {{ 'dash.show' | t }}</span></button>
            }
          </div>
        }
      }
      <div class="dash-widgets" id="dash-widgets" [class.editing]="editing()">
        @for (w of shown(); track w.id; let i = $index, last = $last) {
          <section class="card dash-widget" [attr.data-id]="w.id" [class.wide]="isWide(w)" [style.grid-row-end]="'span ' + rowSpan(w)"
            [attr.draggable]="editing() ? 'true' : null" [class.drag-over]="dragOver() === w.id"
            (dragstart)="dragFrom = w.id" (dragover)="onDragOver($event, w.id)" (dragleave)="dragOver.set('')"
            (drop)="$event.preventDefault(); dropOn(w.id)" (dragend)="dragOver.set('')">
            <h3><span class="dash-title">{{ w.icon || '🧩' }} {{ label(w) }}</span>
              @if (editing()) {
                <span class="dash-tools">
                  <button class="icon-btn" [attr.data-up]="w.id" [disabled]="i === 0" (click)="move(w.id, -1)" [title]="'dash.up' | t" [attr.aria-label]="'dash.up' | t">↑</button>
                  <button class="icon-btn" [attr.data-down]="w.id" [disabled]="last" (click)="move(w.id, 1)" [title]="'dash.down' | t" [attr.aria-label]="'dash.down' | t">↓</button>
                  <select [attr.data-size]="w.id" [value]="sizeOf(w.id)" (change)="setSize(w.id, $any($event.target).value)" [attr.aria-label]="'dash.size' | t">
                    <option value="auto">{{ 'dash.size.auto' | t }}</option>
                    <option value="normal">{{ 'dash.size.normal' | t }}</option>
                    <option value="wide">{{ 'dash.size.wide' | t }}</option>
                  </select>
                  <button class="icon-btn" [attr.data-hide]="w.id" (click)="setHidden(w.id, true)" [title]="'dash.hide' | t" [attr.aria-label]="'dash.hide' | t">✕</button>
                </span>
              }
            </h3>
            <iframe class="dash-widget-frame" [attr.data-id]="w.id" [src]="frameSrc(w)" [style.height.px]="frameHeight(w)" sandbox="allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads"
              referrerpolicy="no-referrer" [title]="label(w)"></iframe>
          </section>
        }
      </div>
      <rea-media-picker [kind]="mediaPick.pending()?.accept ?? 'image'" [open]="mediaPick.pending() !== null" (openChange)="!$event && mediaPick.done(null)" (picked)="mediaPick.done($event)" />
    }

    <h2 class="section-title">{{ 'dash.quick' | t }}</h2>
    <div class="action-grid">
      @for (a of visibleActions(); track a.key) {
        <button class="action-card" (click)="a.run()">
          <span class="action-icon">{{ a.icon }}</span>
          <span class="action-title">{{ a.key | t }}</span>
          <span class="action-desc">{{ a.key + '.d' | t }}</span>
        </button>
      }
    </div>

    <div class="grid-2">
      @if (auth.can('edit_own_posts')) {
      <section class="card">
        <header class="card-head"><h3>{{ 'dash.recent' | t }}</h3><a routerLink="/posts">{{ 'dash.viewAll' | t }}</a></header>
        <ul class="list">
          @for (p of recent(); track p.id) {
            <li>
              <a [routerLink]="['/posts', p.id]">{{ p.title }}</a>
              <span class="badge" [class.badge-live]="p.status === 'publish'">{{ (p.status === 'publish' ? 'common.published' : p.status === 'scheduled' ? 'post.scheduledBadge' : 'common.draft') | t }}</span>
            </li>
          } @empty { <li class="muted">{{ 'dash.noPosts' | t }}</li> }
        </ul>
      </section>
      }
      <section class="card">
        <header class="card-head"><h3>{{ 'dash.steps' | t }}</h3></header>
        <ol class="steps">
          <li><span>1</span>{{ 'dash.step1' | t }}</li>
          <li><span>2</span>{{ 'dash.step2' | t }}</li>
          <li><span>3</span>{{ 'dash.step3' | t }}</li>
          <li><span>4</span>{{ 'dash.step4' | t }}</li>
        </ol>
        <p class="hint">{{ 'dash.tip' | t }}</p>
      </section>
    </div>
  `,
})
export class Dashboard implements OnInit {
  readonly auth = inject(Auth);
  readonly i18n = inject(I18n);
  /** The greeting already says "คุณ": don't repeat it when the name starts with it. */
  greetName() { return (this.auth.user()?.name ?? '').replace(/^คุณ\s*/, ''); }
  private api = inject(Api);
  private router = inject(Router);
  private plugins = inject(Plugins);
  readonly stats = signal<Stats | null>(null);
  readonly recent = signal<Post[]>([]);
  readonly settings = signal<Settings>({});
  readonly error = signal('');
  private sanitizer = inject(DomSanitizer);
  /** site.dashboardWidget cards (sandboxed iframes, same host as admin pages). */
  readonly widgets = signal<DashboardWidget[]>([]);
  private frames = new Map<string, SafeResourceUrl>();
  label(w: DashboardWidget) { this.i18n.locale(); return localized(w.label) || w.id; }
  /** Only same-origin /ext-ui/code/ URLs reach here (normalizeWidgets). */
  frameSrc(w: DashboardWidget): SafeResourceUrl {
    let s = this.frames.get(w.url);
    if (!s) { s = this.sanitizer.bypassSecurityTrustResourceUrl(w.url); this.frames.set(w.url, s); }
    return s;
  }

  /** Heights the widgets reported (kit.js {type:'ak:height'}); the registered height until then. */
  readonly heights = signal<Record<string, number>>({});
  private host = inject<ElementRef<HTMLElement>>(ElementRef);
  frameHeight(w: DashboardWidget) { return this.heights()[w.id] ?? w.height; }
  /** 2.5.3: a widget that was ever taller than this is two columns wide on a wide screen (admin.scss .dash-widget.wide).
   *  "Ever": once wide it stays wide, even if the wider frame then reports a shorter height (no flip-flopping). */
  readonly tallWidget = 420;
  readonly tallest = signal<Record<string, number>>({});
  /** The user's own size choice (layout.size) wins over the automatic one. */
  isWide(w: DashboardWidget) {
    const s = this.sizeOf(w.id);
    return s === 'wide' || (s === 'auto' && Math.max(w.height, this.tallest()[w.id] ?? 0) > this.tallWidget);
  }

  // ---- 2.5.3: each user arranges the widgets (order, hidden, size), saved with the account (/auth/me/prefs/dashboard)
  readonly layout = signal<DashLayout>(emptyLayout());
  readonly editing = signal(false);
  private readonly arranged = computed(() => arrange(this.widgets(), this.layout()));
  readonly shown = computed(() => this.arranged().shown);
  readonly hiddenWidgets = computed(() => this.arranged().hidden);
  readonly hiddenCount = computed(() => this.hiddenWidgets().length);
  sizeOf(id: string): DashSize { return this.layout().size[id] ?? 'auto'; }
  move(id: string, by: number) { this.setOrder(moveId(this.shown().map(w => w.id), id, by)); }
  dragFrom = '';
  readonly dragOver = signal('');
  onDragOver(e: DragEvent, id: string) {
    if (!this.editing()) return;
    e.preventDefault();
    if (this.dragOver() !== id) this.dragOver.set(id);
  }
  dropOn(id: string) {
    this.dragOver.set('');
    const from = this.dragFrom;
    this.dragFrom = '';
    if (this.editing() && from && from !== id) this.setOrder(dropId(this.shown().map(w => w.id), from, id));
  }
  /** The visible order first, then the hidden ones (showing one again puts it at the end, where it is easy to find). */
  private setOrder(visible: string[]) {
    const hidden = this.hiddenWidgets().map(w => w.id);
    this.change(l => ({ ...l, order: [...visible, ...hidden] }));
  }
  setHidden(id: string, hide: boolean) {
    this.change(l => ({ ...l, hidden: hide ? [...new Set([...l.hidden, id])] : l.hidden.filter(x => x !== id) }));
  }
  setSize(id: string, s: DashSize) {
    this.change(l => { const size = { ...l.size }; if (s === 'auto') delete size[id]; else size[id] = s; return { ...l, size }; });
  }
  async resetLayout() {
    clearTimeout(this.saveTimer);
    this.layout.set(emptyLayout());
    try { await this.api.setMyPref('dashboard', null); } catch { Dialog.error(this.i18n.t('dash.saveFailed')); }
  }
  private saveTimer?: ReturnType<typeof setTimeout>;
  /** Changes apply at once; saving waits 400 ms so a few quick clicks are one request. */
  private change(f: (l: DashLayout) => DashLayout) {
    this.layout.update(f);
    clearTimeout(this.saveTimer);
    this.saveTimer = setTimeout(() => { this.api.setMyPref('dashboard', this.layout()).catch(() => Dialog.error(this.i18n.t('dash.saveFailed'))); }, 400);
  }
  /** Grid rows (4px each, admin.scss .dash-widgets) a widget covers: its frame, its title bar and the gap under it. */
  rowSpan(w: DashboardWidget) { return Math.ceil((this.frameHeight(w) + 46 + 16) / 4); }

  /** AK.pickMedia() of a widget frame: the admin's media library dialog. */
  readonly mediaPick = new FrameMediaPick();

  /** Auto-height, AK.open() and AK.pickMedia() of a widget frame — only messages whose window is one of our widget iframes. */
  onFrameMessage(e: MessageEvent) {
    const frame = sourceFrame(this.host.nativeElement.querySelectorAll<HTMLIFrameElement>('iframe.dash-widget-frame'), e.source);
    const id = frame?.dataset['id'];
    const m = frame && id ? readFrameMessage(e.data, adminBase()) : null;
    if (!frame || !id || !m) return;
    if (m.type === 'height') {
      const h = clampHeight(m.height, WIDGET_MIN_HEIGHT, WIDGET_MAX_HEIGHT);
      if (this.heights()[id] !== h) this.heights.update(x => ({ ...x, [id]: h }));
      if ((this.tallest()[id] ?? 0) < h) this.tallest.update(x => ({ ...x, [id]: h }));
    } else if (m.type === 'navigate') {
      this.router.navigateByUrl(m.route);
    } else {
      this.mediaPick.request(frame, m.reqId, m.accept, this.auth.can('upload_files'));
    }
  }

  /** Stat cards; each links to its list only for accounts that may open it. */
  readonly statCards: { key: 'pages' | 'posts' | 'media' | 'components'; path: string; icon: string; color: string; cap: Cap }[] = [
    { key: 'pages', path: '/pages', icon: '📄', color: 'c1', cap: 'edit_pages' },
    { key: 'posts', path: '/posts', icon: '📝', color: 'c2', cap: 'edit_own_posts' },
    { key: 'media', path: '/media', icon: '🖼', color: 'c3', cap: 'upload_files' },
    { key: 'components', path: '/components', icon: '◈', color: 'c4', cap: 'edit_pages' },
  ];

  private readonly actions: Action[] = [
    { icon: '✍️', key: 'dash.a.write', cap: 'edit_own_posts', run: () => this.router.navigate(['/posts/new']) },
    { icon: '🏠', key: 'dash.a.home', cap: 'edit_pages', run: () => this.editHome() },
    { icon: '➕', key: 'dash.a.page', cap: 'edit_pages', run: () => this.newPage() },
    { icon: '🖼', key: 'dash.a.media', cap: 'upload_files', run: () => this.router.navigate(['/media']) },
    { icon: '🧭', key: 'dash.a.menu', cap: 'edit_pages', run: () => this.router.navigate(['/menus']) },
    { icon: '🧩', key: 'dash.a.plugins', cap: 'manage_options', run: () => this.router.navigate(['/plugins']) },
  ];
  /** Always an even, symmetric grid: at most 6 cards. */
  readonly visibleActions = computed(() =>
    this.actions.filter(a => (!a.cap || this.auth.can(a.cap)) && (!a.show || a.show())).slice(0, 6));

  /** 2.5.0: posts contributors sent for review (shown to those who may publish them). */
  readonly pending = signal(0);

  async ngOnInit() {
    this.api.codeDashboardWidgets().then(r => this.widgets.set(normalizeWidgets(r))).catch(() => undefined);
    this.api.myPrefs().then(p => this.layout.set(readLayout(p?.['dashboard']))).catch(() => undefined);
    if (this.auth.can('publish_posts')) this.api.posts({ type: 'post', status: 'pending', perPage: 1 }).then(r => this.pending.set(r.total ?? 0)).catch(() => undefined);
    try {
      const [s, p] = await Promise.all([
        this.api.stats(),
        this.auth.can('edit_own_posts') ? this.api.posts({ type: 'post', perPage: 5, order: 'updated' }) : null,
      ]);
      this.stats.set(s);
      this.recent.set(p?.items ?? []);
      if (this.auth.can('edit_pages')) this.settings.set(await this.api.settings());
      await this.plugins.load();
    } catch (e) {
      this.error.set(errorMessage(e));
    }
  }

  async editHome() {
    try {
      let id = Number(this.settings()['home_page_id']);
      if (!id) {
        const pages = await this.api.posts({ type: 'page', perPage: 100 });
        id = pages.items.find(p => p.slug === 'home')?.id ?? pages.items[0]?.id ?? 0;
      }
      if (id) await this.router.navigate(['/design', id]);
      else await this.newPage();
    } catch (e) {
      Dialog.error(errorMessage(e));
    }
  }

  private creating = false;
  async newPage() {
    if (this.creating) return;
    // ask for the name first; one page per click even while it loads (2.1.1)
    this.creating = true;
    const got = await Dialog.nameAndSlug({
      title: this.i18n.t('content.page.new'), nameLabel: this.i18n.t('content.newName2'), value: this.i18n.t('content.page.newTitle'),
      slugLabel: this.i18n.t('content.newSlug'), slugHint: this.i18n.t('content.newSlugHint'), slugWarn: this.i18n.t('content.newSlugWarn'), prefix: '/',
    });
    if (!got) { this.creating = false; return; }
    try {
      const design = starterPage(await this.api.components());
      const p = await this.api.createPost({ type: 'page', title: got.title, ...(got.slug ? { slug: got.slug } : {}), design });
      this.router.navigate(['/design', p.id]);
    } catch (e) {
      this.error.set(errorMessage(e));
      this.creating = false;
    }
  }
}

