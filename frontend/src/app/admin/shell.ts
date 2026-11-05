import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { Location } from '@angular/common';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { filter, map } from 'rxjs';
import { Api, SITE_ORIGIN, errorMessage } from '../core/api';
import { Dialog } from '../core/dialog';
import { Auth, Cap } from '../core/auth';
import { I18n, LOCALES, Locale, TPipe, localized } from '../core/i18n';
import { LegacyMigrations } from '../core/legacy';
import { AdminNotice, normalizeNotices, noticeTarget } from '../core/admin-ext';
import { Plugins } from '../core/plugins';
import { PostTypes } from '../core/post-types';
import { listRoute, typeSource } from '../core/post-type-utils';
import { SiteInfo } from '../core/site';
import { roleName } from '../core/roles';
import './messages-content-types2';
import './messages-platform';
import './messages-content-types';
import './messages-phase7';

interface NavItem { path: string; label: string; desc: string; icon: string; cap?: Cap; id?: string }
interface NavGroup { title: string; items: NavItem[] }

/** Admin layout: sidebar navigation + top bar (back, breadcrumb, language, view site). */
@Component({
  selector: 'rea-shell',
  imports: [RouterOutlet, RouterLink, RouterLinkActive, TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="app" [class.nav-open]="navOpen()">
      <div class="nav-backdrop" (click)="navOpen.set(false)"></div>
      <aside class="side">
        <a class="side-brand" routerLink="/">
          @if (site.logo()) { <img class="logo logo-img" [src]="site.logo()" alt="" /> } @else if (site.icon()) { <img class="logo logo-img" [src]="site.icon()" alt="" /> } @else { <span class="logo">◆</span> }
          <span class="side-brand-text"><b>{{ site.title() || 'Anasaki Studio' }}</b><small>{{ 'brand.sub' | t }}</small></span>
        </a>
        <nav class="side-nav">
          @for (g of groups(); track g.title) {
            <div class="side-group">
              <div class="side-title">{{ g.title }}</div>
              @for (item of g.items; track item.path) {
                <a class="side-item" [routerLink]="item.path" routerLinkActive="active" [routerLinkActiveOptions]="{ exact: item.path === '/' }" [title]="item.desc" [attr.id]="item.id ?? null">
                  <span class="side-icon">{{ item.icon }}</span>
                  <span class="side-text"><span class="side-label">{{ item.label }}</span><span class="side-desc">{{ item.desc }}</span></span>
                </a>
              }
            </div>
          }
        </nav>
        <div class="side-user">
          @if (auth.user()?.avatar) { <img class="avatar-img" [src]="auth.user()?.avatar" alt="" /> } @else { <span class="avatar">{{ initial() }}</span> }
          <span class="side-user-text"><b>{{ auth.user()?.name }}</b><small>{{ roleLabel() }}</small></span>
          <button class="icon-btn" [title]="'shell.logout' | t" (click)="auth.logout()">⎋</button>
        </div>
      </aside>

      <div class="main">
        <header class="topbar">
          <div class="topbar-left">
            <button class="menu-btn" (click)="navOpen.set(!navOpen())" [attr.aria-label]="'shell.menu' | t" [attr.aria-expanded]="navOpen()">☰</button>
            @if (!isHome()) {
              <button class="back-btn" (click)="goBack()" [title]="'common.back' | t">← {{ 'common.back' | t }}</button>
            }
            <nav class="crumbs">
              <a routerLink="/">{{ 'shell.home' | t }}</a>
              @if (current(); as c) { <span>/</span><b>{{ c.label }}</b> }
            </nav>
          </div>
          <div class="topbar-right">
            <div class="lang-switch" [title]="'common.language' | t">
              @for (l of locales; track l.id) {
                <button [class.on]="i18n.locale() === l.id" (click)="setLocale(l.id)">{{ l.short }}</button>
              }
            </div>
            <a class="btn btn-ghost" [href]="siteUrl()" target="_blank">🌐 {{ 'shell.viewSite' | t }}</a>
          </div>
        </header>
        @if (site.demo(); as d) {
          <div class="demo-banner">
            🧪 {{ 'demo.banner' | t }}
            @if (d.everyMinutes) { <span>{{ 'demo.next' | t: { time: nextReset() } }}</span> }
            @if (auth.can('manage_options')) {
              <button class="link" (click)="demoSnapshot()">{{ 'demo.snapshot' | t }}</button>
              <button class="link" (click)="demoReset()">{{ 'demo.resetNow' | t }}</button>
            }
          </div>
        }
        @if (auth.can('manage_options') && legacy.waiting().length) {
          <div id="legacy-banner" class="legacy-banner" role="status">
            <b>📦 {{ 'legacy.banner' | t: { n: legacy.waiting().length } }}</b>
            <button id="legacy-retry" class="btn btn-sm" [disabled]="legacy.busy() || legacy.summary().running" (click)="legacyRetry()">↻ {{ 'legacy.retry' | t }}</button>
            <label class="btn btn-sm" [class.disabled]="legacy.busy()" [title]="'legacy.uploadHint' | t">⬆ {{ 'legacy.upload' | t }}
              <input id="legacy-upload" type="file" accept=".zip,application/zip" hidden (change)="legacyUpload($event)" />
            </label>
            <a routerLink="/plugins" fragment="legacy">{{ 'legacy.details' | t }}</a>
            <small>{{ 'legacy.bannerHint' | t }}</small>
          </div>
        }
        @if (notices().length) {
          <div id="admin-notices">
            @for (n of notices(); track n.id) {
              <div class="admin-notice" role="status" [attr.data-unit]="n.unit" [attr.data-level]="n.level">
                <span class="admin-notice-text">{{ n.text }}</span>
                @if (n.link; as l) {
                  @if (noticeLink(l); as tg) {
                    @if (tg.kind === 'router') { <a class="admin-notice-link" [routerLink]="tg.path">{{ l.label }}</a> }
                    @else { <a class="admin-notice-link" [href]="tg.href" target="_blank" rel="noopener">{{ l.label }}</a> }
                  }
                }
              </div>
            }
          </div>
        }
        <main class="page"><router-outlet /></main>
      </div>
    </div>
  `,
})
export class Shell {
  readonly auth = inject(Auth);
  readonly i18n = inject(I18n);
  readonly site = inject(SiteInfo);
  readonly plugins = inject(Plugins);
  /** Post types and admin pages of the live site code (phase 4a). */
  readonly postTypes = inject(PostTypes);
  private router = inject(Router);
  private location = inject(Location);
  private api = inject(Api);
  /** Phase 7: former built-in plugins waiting to be moved to their official plugins (banner). */
  readonly legacy = inject(LegacyMigrations);
  /** Mobile: the sidebar slides in over the page. */
  readonly navOpen = signal(false);
  /** The public site (maintenance mode is the official plugin's admin notice with its own bypass link). */
  readonly siteUrl = computed(() => SITE_ORIGIN + '/');
  readonly locales = LOCALES;
  /** site.adminNotice of JS plugins / Site Code (refreshed on navigation, at most every 30 s). */
  readonly notices = signal<AdminNotice[]>([]);
  private noticesAt = 0;
  private loadNotices() {
    if (Date.now() - this.noticesAt < 30_000) return;
    this.noticesAt = Date.now();
    this.api.codeAdminNotices().then(r => this.notices.set(normalizeNotices(r))).catch(() => undefined);
  }
  noticeLink(l: { href: string; external: boolean }) { return noticeTarget(l, new URL(document.baseURI).pathname, SITE_ORIGIN); }

  private url = toSignal(this.router.events.pipe(filter(e => e instanceof NavigationEnd), map(() => this.router.url)), { initialValue: this.router.url });
  readonly isHome = computed(() => this.url().split('?')[0] === '/');

  /** Sidebar groups for the current language, role and active plugins. */
  readonly groups = computed<NavGroup[]>(() => {
    const t = (k: string) => this.i18n.t(k);
    const item = (path: string, key: string, icon: string, cap?: Cap): NavItem => ({ path, label: t(key), desc: t(key + '.d'), icon, cap });
    const en = this.i18n.locale() === 'en';
    // post types from site.postType that this user may edit (the server filters by access.edit)
    const typeItems: NavItem[] = this.postTypes.editable().map(p => ({
      path: listRoute(p.type), label: localized(p.label) || p.type, icon: p.icon || '📦', id: 'nav-type-' + p.type,
      desc: typeSource(p) ? this.i18n.t('nav.type.plugin.d', { name: typeSource(p) }) : p.admin ? t('nav.type.admin.d') : t('nav.type.d'),
    }));
    // taxonomies from site.taxonomy whose terms this user may manage (CMS 2.1.0)
    const taxItems: NavItem[] = this.postTypes.manageableTaxonomies().map(x => ({
      path: '/terms/' + x.id, label: localized(x.label) || x.id, icon: '🏷', id: 'nav-tax-' + x.id,
      desc: this.i18n.t('nav.terms.d', { name: x.pluginName || 'Site Code' }),
    }));
    const adminPages: NavItem[] = this.postTypes.adminPages().map(p => ({
      // "<plugin>:<name>" pages come from a plugin; the others from the site's own code
      path: '/x/code/' + p.id, label: localized(p.label) || p.id, icon: p.icon || '🧰', id: 'nav-code-page-' + p.id,
      desc: p.id.includes(':') ? this.i18n.t('nav.adminPage.plugin.d', { name: p.id.split(':')[0] }) : t('nav.adminPage.d'),
    }));
    const groups: NavGroup[] = [
      { title: t('nav.group.start'), items: [item('/', 'nav.dashboard', '🏠')] },
      {
        title: t('nav.group.content'), items: [
          item('/posts', 'nav.posts', '📝', 'edit_own_posts'),
          ...typeItems,
          item('/pages', 'nav.pages', '📄', 'edit_pages'),
          item('/media', 'nav.media', '🖼', 'upload_files'),
          item('/categories', 'nav.categories', '🏷', 'manage_categories'),
          ...taxItems,
        ],
      },
      {
        title: t('nav.group.look'), items: [
          item('/menus', 'nav.menus', '🧭', 'edit_pages'),
          item('/components', 'nav.components', '◈', 'edit_pages'),
          item('/templates', 'nav.templates', '🧩', 'edit_pages'),
        ],
      },
      {
        title: t('nav.group.plugins'), items: this.plugins.menu().map(m => ({
          path: m.path, label: en ? m.labelEn || m.label : m.label, desc: '', icon: m.icon, cap: m.cap as Cap,
        })),
      },
      { title: t('nav.group.dev'), items: [item('/code', 'nav.code', '⌨️', 'view_scripts'), ...adminPages] },
      {
        title: t('nav.group.settings'), items: [
          item('/plugins', 'nav.plugins', '🧩', 'manage_options'),
          item('/settings', 'nav.settings', '⚙️', 'manage_options'),
          item('/content-types', 'nav.contentTypes', '🗂', 'manage_options'),
          { ...item('/tools/export', 'nav.tools', '💾', 'manage_options'), id: 'nav-tools' },
          item('/users', 'nav.users', '👥', 'manage_users'),
          item('/roles', 'nav.roles', '🛡', 'manage_users'),
          item('/profile', 'nav.profile', '🙂'),
        ],
      },
    ];
    return groups
      .map(g => ({ ...g, items: g.items.filter(i => !i.cap || this.auth.can(i.cap)) }))
      .filter(g => g.items.length);
  });

  /** The sidebar item of the current screen: the longest item path the URL starts with (segment-wise). */
  readonly current = computed(() => {
    const path = this.url().split('?')[0].split('#')[0];
    let best: NavItem | undefined;
    for (const i of this.groups().flatMap(g => g.items)) {
      const hit = i.path === '/' ? path === '/' : path === i.path || path.startsWith(i.path + '/');
      if (hit && (!best || i.path.length > best.path.length)) best = i;
    }
    return best;
  });

  /** Time of the next demo reset (HH:MM, local). */
  readonly nextReset = computed(() => {
    const d = this.site.demo();
    return d?.nextReset ? new Date(d.nextReset).toLocaleTimeString(this.i18n.locale() === 'th' ? 'th-TH' : 'en-GB', { hour: '2-digit', minute: '2-digit' }) : '';
  });

  async demoSnapshot() {
    if (!(await Dialog.confirm({ title: this.i18n.t('demo.snapshotConfirm'), text: this.i18n.t('demo.snapshotText') }))) return;
    try { this.site.demo.set(await this.api.demoSnapshot()); Dialog.toast(this.i18n.t('demo.snapshotDone')); } catch (e) { Dialog.error(errorMessage(e)); }
  }

  async demoReset() {
    if (!(await Dialog.confirm({ title: this.i18n.t('demo.resetConfirm'), text: this.i18n.t('demo.resetText'), danger: true }))) return;
    try { this.site.demo.set(await this.api.demoReset()); Dialog.toast(this.i18n.t('demo.resetDone')); location.reload(); } catch (e) { Dialog.error(errorMessage(e)); }
  }

  constructor() {
    this.site.load();
    void this.plugins.load();
    void this.postTypes.load();
    if (this.auth.can('manage_options')) void this.legacy.load();
    this.loadNotices();
    this.router.events.pipe(filter(e => e instanceof NavigationEnd)).subscribe(() => this.loadNotices());
    this.router.events.pipe(filter(e => e instanceof NavigationEnd)).subscribe(() => this.navOpen.set(false));
  }

  async legacyRetry() {
    try { await this.legacy.retry(); } catch (e) { Dialog.error(errorMessage(e)); }
  }

  async legacyUpload(e: Event) {
    const input = e.target as HTMLInputElement;
    const f = input.files?.[0];
    input.value = '';
    if (!f) return;
    try { await this.legacy.upload(f); Dialog.toast(this.i18n.t('legacy.uploaded')); } catch (err) { Dialog.error(errorMessage(err)); }
  }

  roleLabel() { this.i18n.locale(); return roleName(this.auth.user()?.role ?? ''); }

  initial() { return (this.auth.user()?.name ?? '?').slice(0, 1).toUpperCase(); }

  setLocale(l: Locale) { this.auth.setLocale(l); }

  /** Back to the previous screen, or the dashboard when opened directly. */
  goBack() {
    if (history.length > 1) this.location.back();
    else this.router.navigateByUrl('/');
  }
}

/** Translation key for a role (used across admin screens). */
export const roleKey = (role?: string) => 'role.' + (role ?? 'author');
