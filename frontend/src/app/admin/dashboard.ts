import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
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

interface Action { icon: string; key: string; cap?: Cap; show?: () => boolean; run: () => void }

@Component({
  selector: 'rea-dashboard',
  imports: [RouterLink, NgTemplateOutlet, TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
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
      <div class="dash-widgets" id="dash-widgets">
        @for (w of widgets(); track w.id) {
          <section class="card dash-widget" [attr.data-id]="w.id">
            <h3>{{ w.icon || '🧩' }} {{ label(w) }}</h3>
            <iframe class="dash-widget-frame" [src]="frameSrc(w)" [style.height.px]="w.height" sandbox="allow-scripts allow-forms allow-popups allow-downloads"
              referrerpolicy="no-referrer" [title]="label(w)"></iframe>
          </section>
        }
      </div>
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
              <span class="badge" [class.badge-live]="p.status === 'publish'">{{ (p.status === 'publish' ? 'common.published' : 'common.draft') | t }}</span>
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

  async ngOnInit() {
    this.api.codeDashboardWidgets().then(r => this.widgets.set(normalizeWidgets(r))).catch(() => undefined);
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

  async newPage() {
    try {
      const design = starterPage(await this.api.components());
      const p = await this.api.createPost({ type: 'page', title: this.i18n.t('content.page.newTitle'), design });
      this.router.navigate(['/design', p.id]);
    } catch (e) {
      this.error.set(errorMessage(e));
    }
  }
}

