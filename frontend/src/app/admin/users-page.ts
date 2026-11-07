import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { Dialog } from '../core/dialog';
import { I18n, LOCALES, Locale, TPipe } from '../core/i18n';
import { CapInfo, Media, RoleInfo, User } from '../core/models';
import { BUILTIN_ROLE_NAMES, capInfoLabel, capLabel, capSections, i18nText, roleName } from '../core/roles';
import { MediaPicker } from '../shared/media-picker';
import { randomPassword } from '../shared/password';
import './messages-admin';
import './messages-platform';
import { SelectValue } from '../shared/select-value';
import { ApiTokens } from './api-tokens';

function initials(u: Partial<User>) {
  return avatarInitial(u.name || u.email);
}

type Draft = Partial<User> & { password?: string; sendWelcome?: boolean };

// ===================================================================== Users

import { Pager } from '../shared/pager';
import { avatarInitial } from '../shared/initial';

@Component({
  selector: 'rea-users',
  imports: [SelectValue, FormsModule, TPipe, MediaPicker, RouterLink, Pager, ApiTokens],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">👥</span>
      <div class="page-head-text"><h1>{{ 'nav.users' | t }}</h1><p>{{ 'users.desc' | t }}</p></div>
      <div class="head-actions">
        <a class="btn" routerLink="/roles">🛡 {{ 'nav.roles' | t }}</a>
        <button class="btn btn-primary" (click)="openNew()">+ {{ 'users.add' | t }}</button>
      </div>
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }

    <div class="tabs-row">
      <button class="tab" [class.on]="role() === ''" (click)="setRole('')">{{ 'common.all' | t }} <span class="tab-count">{{ allCount() }}</span></button>
      @for (r of roles(); track r.slug) {
        @if ((counts()[r.slug] || 0) > 0) {
          <button class="tab" [class.on]="role() === r.slug" (click)="setRole(r.slug)">{{ roleLabel(r.slug) }} <span class="tab-count">{{ counts()[r.slug] }}</span></button>
        }
      }
    </div>

    <section class="card table-card">
      <div class="table-toolbar">
        <input class="search" [placeholder]="'users.searchPh' | t" [value]="search()" (input)="search.set(val($event))" (keyup.enter)="go(1)" />
        <select [value]="status()" (change)="status.set(val($event)); go(1)">
          <option value="">{{ 'common.allStatus' | t }}</option>
          <option value="active">{{ 'users.status.active' | t }}</option>
          <option value="suspended">{{ 'users.status.suspended' | t }}</option>
        </select>
        <span class="grow"></span>
        @if (picked().size) {
          <span class="muted small">{{ 'users.selected' | t: { n: picked().size } }}</span>
          <select #bulk>
            <option value="">{{ 'users.bulk' | t }}</option>
            <optgroup [label]="'users.bulkRole' | t">
              @for (r of roles(); track r.slug) { <option [value]="'role:' + r.slug">→ {{ roleLabel(r.slug) }}</option> }
            </optgroup>
            <option value="activate">✅ {{ 'users.bulkActivate' | t }}</option>
            <option value="suspend">⏸ {{ 'users.bulkSuspend' | t }}</option>
            <option value="delete">🗑 {{ 'common.delete' | t }}</option>
          </select>
          <button class="btn btn-sm" (click)="applyBulk(bulk.value)">{{ 'users.apply' | t }}</button>
        }
      </div>
      <table class="table">
        <thead>
          <tr>
            <th class="th-check"><input type="checkbox" [checked]="allPicked()" (change)="toggleAll()" [attr.aria-label]="'common.all' | t" /></th>
            <th>{{ 'common.name' | t }}</th>
            <th>{{ 'users.role' | t }}</th>
            <th class="num">{{ 'users.posts' | t }}</th>
            <th>{{ 'common.status' | t }}</th>
            <th>{{ 'users.lastLogin' | t }}</th>
            <th class="th-actions">{{ 'common.actions' | t }}</th>
          </tr>
        </thead>
        <tbody>
          @for (u of items(); track u.id) {
            <tr>
              <td class="th-check"><input type="checkbox" [checked]="picked().has(u.id)" (change)="toggle(u.id)" [disabled]="u.id === me()" /></td>
              <td>
                <div class="user-cell">
                  @if (u.avatar) { <img class="avatar-img" [src]="u.avatar" alt="" /> } @else { <span class="avatar">{{ initials(u) }}</span> }
                  <div>
                    <a class="row-title" (click)="openEdit(u)">{{ u.name || u.email }}</a>
                    @if (u.id === me()) { <span class="badge badge-info">{{ 'users.you' | t }}</span> }
                    <div class="muted small">{{ u.email }}</div>
                  </div>
                </div>
              </td>
              <td><span class="chip">{{ roleLabel(u.role) }}</span></td>
              <td class="num">{{ u.postCount ?? 0 }}</td>
              <td>
                @if (u.status === 'suspended') { <span class="badge badge-warn">⏸ {{ 'users.status.suspended' | t }}</span> }
                @else { <span class="badge badge-live">● {{ 'users.status.active' | t }}</span> }
              </td>
              <td class="muted small">{{ u.lastLoginAt ? i18n.date(u.lastLoginAt, true) : ('users.never' | t) }}</td>
              <td class="row-actions">
                <button class="btn btn-sm" (click)="openEdit(u)">✏️ {{ 'common.edit' | t }}</button>
                @if (u.id !== me()) { <button class="btn btn-sm btn-danger" (click)="remove(u)">🗑</button> }
              </td>
            </tr>
          } @empty {
            <tr><td colspan="7" class="empty-state"><div class="empty-icon">👥</div><p>{{ 'users.empty' | t }}</p></td></tr>
          }
        </tbody>
      </table>
      <rea-pager [page]="page()" [total]="total()" [perPage]="perPage" (pageChange)="go($event)" />
    </section>

    @if (draft(); as d) {
      <div class="drawer-backdrop" (click)="draft.set(null)"></div>
      <aside class="drawer drawer-wide">
        <header class="drawer-head">
          <h3>{{ (d.id ? 'users.edit' : 'users.add') | t }}</h3>
          <button class="icon-btn" (click)="draft.set(null)" [attr.aria-label]="'common.close' | t">✕</button>
        </header>
        <div class="drawer-body">
          <div class="avatar-row">
            @if (d.avatar) { <img class="avatar-lg" [src]="d.avatar" alt="" /> } @else { <span class="avatar avatar-lg">{{ initials(d) }}</span> }
            <div class="row-actions">
              <button type="button" class="btn btn-sm" (click)="picking.set(true)">🖼 {{ 'users.avatar' | t }}</button>
              @if (d.avatar) { <button type="button" class="btn btn-sm" (click)="patch({ avatar: '' })">{{ 'post.removeImage' | t }}</button> }
            </div>
          </div>

          <h4>{{ 'users.account' | t }}</h4>
          <label [class.field-invalid]="fieldErr()['email']">{{ 'common.email' | t }} <b class="req" aria-hidden="true">*</b>
            <input type="email" id="user-email" [ngModel]="d.email" (ngModelChange)="patch({ email: $event })" required [attr.aria-invalid]="!!fieldErr()['email']" />
            @if (fieldErr()['email']; as m) { <span class="field-error" id="user-email-error">{{ m }}</span> }</label>
          @if (!d.id) {
            <label class="check welcome-check"><input type="checkbox" [checked]="!!d.sendWelcome" [disabled]="!mailReady()" (change)="patch({ sendWelcome: $any($event.target).checked })" /> {{ 'users.sendWelcome' | t }}</label>
            <p class="hint">{{ (mailReady() ? 'users.sendWelcomeHint' : 'users.mailOff') | t }}</p>
          }
          @if (d.id || !d.sendWelcome) {
          <label>{{ (d.id ? 'profile.newPw' : 'common.password') | t }}
            <span class="muted small">{{ (d.id ? 'users.pwKeep' : 'users.pwMin') | t }}</span>
            <div class="pw-row">
              <input [type]="showPw() ? 'text' : 'password'" [ngModel]="d.password" (ngModelChange)="patch({ password: $event })" autocomplete="new-password" />
              <button type="button" class="btn btn-sm" (click)="showPw.set(!showPw())">{{ (showPw() ? 'users.hide' : 'users.show') | t }}</button>
              <button type="button" class="btn btn-sm" (click)="generate()">🎲 {{ 'users.generate' | t }}</button>
            </div>
          </label>
          }
          <div class="form-2">
            <label>{{ 'users.role' | t }}
              <select [ngModel]="d.role" (ngModelChange)="patch({ role: $event })" [disabled]="d.id === me()">
                @for (r of roles(); track r.slug) { <option [value]="r.slug">{{ roleLabel(r.slug) }}</option> }
              </select>
            </label>
            <label>{{ 'common.status' | t }}
              <select [ngModel]="d.status ?? 'active'" (ngModelChange)="patch({ status: $event })" [disabled]="d.id === me()">
                <option value="active">{{ 'users.status.active' | t }}</option>
                <option value="suspended">{{ 'users.status.suspended' | t }}</option>
              </select>
            </label>
          </div>
          <p class="hint">{{ roleHint(d.role ?? '') }}</p>

          <h4>{{ 'users.personal' | t }}</h4>
          <div class="form-2">
            <label>{{ 'users.firstName' | t }} <input [ngModel]="d.firstName" (ngModelChange)="patch({ firstName: $event })" /></label>
            <label>{{ 'users.lastName' | t }} <input [ngModel]="d.lastName" (ngModelChange)="patch({ lastName: $event })" /></label>
          </div>
          <label [class.field-invalid]="fieldErr()['name']">{{ 'users.displayName' | t }} <b class="req" aria-hidden="true">*</b> <span class="muted small">{{ 'users.displayNameHint' | t }}</span>
            <input id="user-name" [ngModel]="d.name" (ngModelChange)="patch({ name: $event })" required [attr.aria-invalid]="!!fieldErr()['name']"
                   [placeholder]="((d.firstName ?? '') + ' ' + (d.lastName ?? '')).trim()" />
            @if (fieldErr()['name']; as m) { <span class="field-error" id="user-name-error">{{ m }}</span> }
          </label>
          <label>{{ 'users.website' | t }} <input type="url" placeholder="https://" [ngModel]="d.website" (ngModelChange)="patch({ website: $event })" /></label>
          <label>{{ 'users.bio' | t }} <textarea rows="3" [ngModel]="d.bio" (ngModelChange)="patch({ bio: $event })"></textarea></label>
          <label>{{ 'common.language' | t }}
            <select [ngModel]="d.locale ?? 'th'" (ngModelChange)="patch({ locale: $event })">
              @for (l of locales; track l.id) { <option [value]="l.id">{{ l.label }}</option> }
            </select>
          </label>
          @if (d.id && d.createdAt) { <p class="muted small">{{ 'users.since' | t: { date: i18n.date(d.createdAt) } }}</p> }
          @if (d.id) { <rea-api-tokens [userId]="d.id" /> }
        </div>
        <footer class="drawer-foot">
          <button class="btn" (click)="draft.set(null)">{{ 'common.cancel' | t }}</button>
          <button class="btn btn-primary" [disabled]="busy()" (click)="save()">{{ (d.id ? 'common.save' : 'users.add') | t }}</button>
        </footer>
      </aside>
    }
    <rea-media-picker [open]="picking()" (openChange)="picking.set($event)" (picked)="avatarPicked($event)" />
  `,
})
export class Users implements OnInit {
  readonly i18n = inject(I18n);
  private api = inject(Api);
  private auth = inject(Auth);

  readonly perPage = 20;
  readonly locales = LOCALES;
  readonly items = signal<User[]>([]);
  readonly total = signal(0);
  readonly counts = signal<Record<string, number>>({});
  readonly roles = signal<RoleInfo[]>([]);
  readonly role = signal('');
  readonly status = signal('');
  readonly search = signal('');
  readonly page = signal(1);
  readonly picked = signal(new Set<number>());
  readonly draft = signal<Draft | null>(null);
  readonly picking = signal(false);
  readonly showPw = signal(false);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly me = computed(() => this.auth.user()?.id ?? 0);
  readonly mailReady = signal(false);
  readonly allCount = computed(() => this.counts()['all'] ?? this.total());
  readonly allPicked = computed(() => {
    const ids = this.items().filter(u => u.id !== this.me()).map(u => u.id);
    return ids.length > 0 && ids.every(id => this.picked().has(id));
  });

  async ngOnInit() {
    try { this.roles.set((await this.api.roles()).roles); } catch { /* roles endpoint optional */ }
    this.api.mailStatus().then(s => this.mailReady.set(s.configured)).catch(() => undefined);
    await this.load();
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  initials(u: Partial<User>) { return initials(u); }
  roleLabel(slug: string) { return roleName(slug, this.roles()); }
  roleHint(slug: string) {
    const key = `users.role.${slug}`;
    const t = this.i18n.t(key);
    if (t !== key) return t;
    const r = this.roles().find(x => x.slug === slug);
    return r ? r.caps.map(capLabel).join(' · ') : '';
  }

  setRole(r: string) { this.role.set(r); this.go(1); }
  go(p: number) { this.page.set(p); this.load(); }

  async load() {
    try {
      const res = await this.api.users({ role: this.role(), status: this.status(), search: this.search(), page: this.page(), perPage: this.perPage });
      this.items.set(res.items);
      this.total.set(res.total);
      this.counts.set(res.counts ?? {});
      this.picked.set(new Set());
      this.error.set('');
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  toggle(id: number) {
    const s = new Set(this.picked());
    if (s.has(id)) s.delete(id); else s.add(id);
    this.picked.set(s);
  }

  toggleAll() {
    this.picked.set(this.allPicked() ? new Set() : new Set(this.items().filter(u => u.id !== this.me()).map(u => u.id)));
  }

  async applyBulk(action: string) {
    if (!action) return;
    const ids = [...this.picked()];
    if (action === 'delete' && !(await Dialog.confirmDelete(this.i18n.t('users.selected', { n: ids.length }), this.i18n.t('users.deleteText')))) return;
    try {
      if (action.startsWith('role:')) await this.api.bulkUsers(ids, 'role', action.slice(5));
      else await this.api.bulkUsers(ids, action as 'suspend' | 'activate' | 'delete');
      Dialog.toast(this.i18n.t('common.saved'));
      await this.load();
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  openNew() {
    this.showPw.set(false);
    this.fieldErr.set({});
    this.draft.set({ email: '', name: '', role: this.defaultRole(), status: 'active', locale: this.i18n.locale(), password: '', sendWelcome: this.mailReady() });
  }

  /** Errors of the add / edit drawer by field (2.3.0, Qraft feedback 40: the message used to name no field). */
  readonly fieldErr = signal<Record<string, string>>({});

  /** A new account's role: "author" when the site has it, else the role with the fewest rights (never administrator). */
  private defaultRole(): string {
    const list = this.roles();
    if (list.some(r => r.slug === 'author')) return 'author';
    const pick = [...list].filter(r => r.slug !== 'administrator').sort((a, b) => (a.caps?.length ?? 0) - (b.caps?.length ?? 0))[0];
    return pick?.slug ?? 'subscriber';
  }

  openEdit(u: User) {
    this.showPw.set(false);
    this.draft.set({ ...u, password: '' });
  }

  patch(p: Draft) { this.draft.update(d => (d ? { ...d, ...p } : d)); }

  generate() {
    this.patch({ password: randomPassword(14) });
    this.showPw.set(true);
  }

  avatarPicked(m: Media) { this.patch({ avatar: m.url }); }

  async save() {
    const d = this.draft();
    if (!d) return;
    // check the fields here first, so the drawer can point at the one to fix
    const name = (d.name ?? '').trim() || [d.firstName, d.lastName].map(x => (x ?? '').trim()).filter(Boolean).join(' ');
    const errs: Record<string, string> = {};
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test((d.email ?? '').trim())) errs['email'] = this.i18n.t('users.err.email');
    if (!name) errs['name'] = this.i18n.t('users.err.name');
    this.fieldErr.set(errs);
    if (Object.keys(errs).length) {
      document.getElementById(errs['email'] ? 'user-email' : 'user-name')?.focus();
      return;
    }
    if (name !== d.name) this.patch({ name });
    this.busy.set(true);
    try {
      const body: Draft = {
        email: d.email, name, firstName: d.firstName, lastName: d.lastName, bio: d.bio, website: d.website,
        avatar: d.avatar, role: d.role, status: d.status, locale: d.locale,
      };
      if (d.password) body.password = d.password;
      if (d.id) await this.api.updateUser(d.id, body);
      else await this.api.createUser({ ...body, password: d.password ?? '', sendWelcome: !!d.sendWelcome } as Draft & { password: string });
      if (d.id === this.me()) await this.auth.refresh();
      this.draft.set(null);
      Dialog.toast(this.i18n.t('common.saved'));
      await this.load();
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }

  /** Deletes an account after the admin picks who receives its content (the API always reassigns it). */
  async remove(u: User) {
    let others: User[];
    try {
      others = (await this.api.users({ status: 'active', perPage: 200 })).items.filter(x => x.id !== u.id);
    } catch (e) { Dialog.error(errorMessage(e)); return; }
    // Me first (the usual choice), then everyone else; keep me even if the list was cut short.
    const mine = this.auth.user();
    const options: [string, string][] = [];
    if (mine && mine.id !== u.id) options.push([String(mine.id), this.i18n.t('users.deleteToMe', { name: mine.name || mine.email })]);
    for (const x of others) if (x.id !== mine?.id) options.push([String(x.id), `${x.name || x.email} — ${x.email}`]);
    const to = await Dialog.select({
      title: this.i18n.t('dialog.deleteTitle', { name: u.name || u.email }),
      text: this.i18n.t('users.deleteTo'),
      options, value: String(this.me()), confirm: this.i18n.t('common.delete'), danger: true,
    });
    if (!to) return;
    try {
      await this.api.deleteUser(u.id, Number(to));
      Dialog.toast(this.i18n.t('common.deleted'));
      await this.load();
    } catch (e) { Dialog.error(errorMessage(e)); }
  }
}

// ===================================================================== Roles & capabilities

@Component({
  selector: 'rea-roles',
  imports: [FormsModule, TPipe, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <a class="back-btn" routerLink="/users" [attr.aria-label]="'common.back' | t">←</a>
      <span class="page-icon">🛡</span>
      <div class="page-head-text"><h1>{{ 'nav.roles' | t }}</h1><p>{{ 'roles.desc' | t }}</p></div>
      @if (missingBuiltins()) { <button class="btn" (click)="restore()" [title]="'roles.restoreHint' | t">↺ {{ 'roles.restore' | t: { n: missingBuiltins() } }}</button> }
      <button class="btn btn-primary" (click)="create()">+ {{ 'roles.new' | t }}</button>
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }

    <div class="roles-layout">
      <nav class="card role-list">
        @for (r of roles(); track r.slug) {
          <button class="role-item" [class.on]="sel()?.slug === r.slug" (click)="select(r)">
            <span class="role-name">{{ label(r) }}</span>
            <span class="muted small">{{ 'roles.users' | t: { n: r.userCount } }} · {{ r.builtin ? ('roles.builtin' | t) : ('roles.custom' | t) }}</span>
          </button>
        }
      </nav>

      @if (sel(); as r) {
        <section class="card role-editor">
          <div class="card-head">
            <div>
              <h2>{{ label(r) }}</h2>
              <p class="muted small mono">{{ r.slug }}</p>
            </div>
            <span class="grow"></span>
            @if (deletable(r)) { <button class="btn btn-sm btn-danger" (click)="remove(r)">🗑 {{ 'roles.delete' | t }}</button> }
          </div>
          @if (r.slug === 'administrator') { <div class="info-box">🔒 {{ 'roles.adminLocked' | t }}</div> }
          @else if (r.slug === 'subscriber') { <div class="info-box">ℹ️ {{ 'roles.subscriberLocked' | t }}</div> }
          @else if (r.builtin) { <div class="info-box">ℹ️ {{ 'roles.builtinHint' | t }}</div> }
          @if (r.slug !== 'administrator') {
            <label>{{ 'roles.displayName' | t }} <input [ngModel]="name()" (ngModelChange)="name.set($event)" [placeholder]="label(r)" /></label>
          }
          <div class="cap-groups" id="role-core-caps">
            @for (g of coreSections(); track g.id) {
              <fieldset class="cap-group" [attr.data-group]="g.id">
                <legend>{{ groupLabel(g.group) }}</legend>
                @for (c of g.caps; track c.key) {
                  <label class="check">
                    <input type="checkbox" [attr.data-cap]="c.key" [checked]="caps().has(c.key)" [disabled]="r.slug === 'administrator'" (change)="toggleCap(c.key)" />
                    <span>{{ capText(c) }} <span class="muted small mono">{{ c.key }}</span>
                      @if (c.key === 'publish_scripts') { <span class="cap-warn" id="cap-warn-publish-scripts">⚠️ {{ 'roles.publishScriptsWarn' | t }}</span> }</span>
                  </label>
                }
              </fieldset>
            }
          </div>
          @if (pluginSections().length) {
            <div class="role-plugin-caps" id="role-plugin-caps">
              <h3 class="role-plugin-caps-title">🔌 {{ 'roles.pluginCaps' | t }}</h3>
              <p class="muted small">{{ 'roles.pluginCapsHint' | t }}</p>
              <div class="cap-groups">
                @for (g of pluginSections(); track g.id) {
                  <fieldset class="cap-group cap-group-plugin" [attr.data-group]="g.id" [attr.data-plugin]="g.plugin">
                    <legend>@if (g.pluginIcon) { <span class="cap-plugin-icon">{{ g.pluginIcon }}</span> }{{ g.pluginName }}</legend>
                    @for (c of g.caps; track c.key) {
                      <label class="check plugin-cap" [title]="capDesc(c)">
                        <input type="checkbox" [attr.data-cap]="c.key" [checked]="caps().has(c.key)" [disabled]="r.slug === 'administrator'" (change)="toggleCap(c.key)" />
                        <span>{{ capText(c) }} <span class="muted small mono">{{ c.key }}</span>
                          @if (capDesc(c)) { <span class="muted small cap-desc">{{ capDesc(c) }}</span> }</span>
                      </label>
                    }
                  </fieldset>
                }
              </div>
            </div>
          }
          @if (r.slug !== 'administrator') {
            <div class="form-foot">
              <button class="btn" (click)="select(r)">{{ 'roles.reset' | t }}</button>
              <button class="btn btn-primary" [disabled]="busy()" (click)="save()">{{ 'common.save' | t }}</button>
            </div>
          }
        </section>
      }
    </div>
  `,
})
export class Roles implements OnInit {
  private api = inject(Api);
  private auth = inject(Auth);
  private i18n = inject(I18n);
  readonly roles = signal<RoleInfo[]>([]);
  readonly allCaps = signal<CapInfo[]>([]);
  readonly sel = signal<RoleInfo | null>(null);
  readonly caps = signal(new Set<string>());
  readonly name = signal('');
  readonly busy = signal(false);
  readonly error = signal('');
  /** Core groups, then one section per installed plugin (its manifest capabilities, CMS 2.0.6). */
  readonly sections = computed(() => capSections(this.allCaps()));
  readonly coreSections = computed(() => this.sections().filter(s => !s.plugin));
  readonly pluginSections = computed(() => this.sections().filter(s => !!s.plugin));

  ngOnInit() { this.load(); }

  async load(keep?: string) {
    try {
      const res = await this.api.roles();
      this.roles.set(res.roles);
      this.builtinSlugs.set(res.builtin ?? Object.keys(BUILTIN_ROLE_NAMES));
      this.allCaps.set(res.caps);
      const pick = res.roles.find(r => r.slug === (keep ?? this.sel()?.slug)) ?? res.roles[0];
      if (pick) this.select(pick);
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  label(r: RoleInfo) { return roleName(r.slug, this.roles()); }
  capText(c: CapInfo) { return capInfoLabel(c, this.i18n.locale()); }
  capDesc(c: CapInfo) { return i18nText(c.description, this.i18n.locale()); }
  groupLabel(g: string) {
    const t = this.i18n.t(`capgroup.${g}`);
    return t !== `capgroup.${g}` ? t : g;
  }

  select(r: RoleInfo) {
    this.sel.set(r);
    this.caps.set(new Set(r.caps));
    this.name.set(r.name === BUILTIN_ROLE_NAMES[r.slug] ? '' : r.name); // empty = keep the translated name
  }

  /** Administrator (can't lock yourself out) and subscriber (takes users of deleted roles) stay. */
  deletable(r: RoleInfo) { return r.slug !== 'administrator' && r.slug !== 'subscriber'; }

  /** How many built-in roles were deleted (they can be brought back). */
  readonly builtinSlugs = signal<string[]>([]);
  readonly missingBuiltins = computed(() => this.builtinSlugs().filter(s => !this.roles().some(r => r.slug === s)).length);

  async restore() {
    try {
      await this.api.restoreRoles();
      await this.load();
      Dialog.toast(this.i18n.t('roles.restored'));
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  toggleCap(c: string) {
    const s = new Set(this.caps());
    if (s.has(c)) s.delete(c); else s.add(c);
    this.caps.set(s);
  }

  async create() {
    const name = await Dialog.prompt({ title: this.i18n.t('roles.new'), label: this.i18n.t('roles.namePrompt'), required: true });
    if (!name) return;
    const slug = name.toLowerCase().trim().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '') || `role_${Date.now().toString(36)}`;
    try {
      await this.api.saveRole({ slug, name, caps: ['read'] }, true);
      await this.load(slug);
      Dialog.toast(this.i18n.t('common.saved'));
    } catch (e) { Dialog.error(errorMessage(e)); }
  }

  async save() {
    const r = this.sel();
    if (!r) return;
    this.busy.set(true);
    try {
      const name = this.name().trim() || BUILTIN_ROLE_NAMES[r.slug] || r.name;
      await this.api.saveRole({ slug: r.slug, name, caps: [...this.caps()] }, false);
      await this.load(r.slug);
      await this.auth.refresh();
      Dialog.toast(this.i18n.t('common.saved'));
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }

  async remove(r: RoleInfo) {
    if (!(await Dialog.confirmDelete(this.label(r), this.i18n.t('roles.deleteText')))) return;
    try {
      await this.api.deleteRole(r.slug);
      this.sel.set(null);
      await this.load();
    } catch (e) { Dialog.error(errorMessage(e)); }
  }
}

// ===================================================================== My account

@Component({
  selector: 'rea-profile',
  imports: [FormsModule, TPipe, MediaPicker, ApiTokens],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">🙂</span>
      <div class="page-head-text"><h1>{{ 'nav.profile' | t }}</h1><p>{{ 'profile.desc' | t }}</p></div>
    </header>

    <section class="card form-card">
      <div class="form-row">
        <div class="form-label">{{ 'common.language' | t }}<small>{{ 'profile.langHint' | t }}</small></div>
        <div class="lang-cards">
          @for (l of locales; track l.id) {
            <button type="button" class="lang-card" [class.on]="i18n.locale() === l.id" (click)="setLocale(l.id)"><b>{{ l.short }}</b><span>{{ l.label }}</span></button>
          }
        </div>
      </div>
      @if (isDemo()) {
        <div class="info-box">🧪 {{ 'demo.profileNote' | t }}</div>
      } @else {
      <div class="form-row">
        <div class="form-label">{{ 'users.avatar' | t }}<small>{{ 'profile.avatarHint' | t }}</small></div>
        <div class="avatar-row">
          @if (avatar()) { <img class="avatar-lg" [src]="avatar()" alt="" /> } @else { <span class="avatar avatar-lg">{{ initialOf(name() || email()) }}</span> }
          <button type="button" class="btn btn-sm" (click)="picking.set(true)">🖼 {{ 'post.chooseImage' | t }}</button>
          @if (avatar()) { <button type="button" class="btn btn-sm" (click)="avatar.set('')">{{ 'post.removeImage' | t }}</button> }
        </div>
      </div>
      <div class="form-row">
        <div class="form-label">{{ 'users.personal' | t }}</div>
        <div class="form-2">
          <label>{{ 'users.firstName' | t }} <input [ngModel]="firstName()" (ngModelChange)="firstName.set($event)" /></label>
          <label>{{ 'users.lastName' | t }} <input [ngModel]="lastName()" (ngModelChange)="lastName.set($event)" /></label>
        </div>
      </div>
      <div class="form-row">
        <div class="form-label">{{ 'users.displayName' | t }}<small>{{ 'users.displayNameHint' | t }}</small></div>
        <input [ngModel]="name()" (ngModelChange)="name.set($event)" />
      </div>
      <div class="form-row">
        <div class="form-label">{{ 'common.email' | t }}</div>
        <input type="email" [ngModel]="email()" (ngModelChange)="email.set($event)" />
      </div>
      <div class="form-row">
        <div class="form-label">{{ 'users.website' | t }}</div>
        <input type="url" placeholder="https://" [ngModel]="website()" (ngModelChange)="website.set($event)" />
      </div>
      <div class="form-row">
        <div class="form-label">{{ 'users.bio' | t }}<small>{{ 'profile.bioHint' | t }}</small></div>
        <textarea rows="3" [ngModel]="bio()" (ngModelChange)="bio.set($event)"></textarea>
      </div>
      <div class="form-row">
        <div class="form-label">{{ 'profile.newPw' | t }}<small>{{ 'users.pwKeep' | t }}</small></div>
        <input type="password" autocomplete="new-password" [ngModel]="password()" (ngModelChange)="password.set($event)" />
      </div>
      @if (needsCurrent()) {
      <div class="form-row">
        <div class="form-label">{{ 'profile.currentPw' | t }}<small>{{ 'profile.currentPwHint' | t }}</small></div>
        <input type="password" autocomplete="current-password" [ngModel]="currentPassword()" (ngModelChange)="currentPassword.set($event)" />
      </div>
      }
      }
      <div class="form-foot">
        <span class="muted small">{{ roleLabel() }}</span>
        <span class="grow"></span>
        @if (!isDemo()) { <button class="btn btn-primary" [disabled]="busy()" (click)="save()">{{ 'common.save' | t }}</button> }
      </div>
    </section>
    @if (!isDemo()) { <rea-api-tokens /> }
    <rea-media-picker [open]="picking()" (openChange)="picking.set($event)" (picked)="avatar.set($event.url)" />
  `,
})
export class Profile implements OnInit {
  readonly initialOf = avatarInitial;
  /** The shared demo visitor account: only the language can change. */
  readonly isDemo = computed(() => this.auth.user()?.role === 'demo');
  readonly i18n = inject(I18n);
  private api = inject(Api);
  private auth = inject(Auth);
  readonly locales = LOCALES;
  readonly name = signal('');
  readonly email = signal('');
  readonly firstName = signal('');
  readonly lastName = signal('');
  readonly website = signal('');
  readonly bio = signal('');
  readonly avatar = signal('');
  readonly password = signal('');
  readonly currentPassword = signal('');
  /** A new password or email needs the current password (checked by the server). */
  readonly needsCurrent = computed(() => !!this.password() || this.email().trim().toLowerCase() !== (this.auth.user()?.email ?? '').toLowerCase());
  readonly picking = signal(false);
  readonly busy = signal(false);
  readonly roleLabel = computed(() => {
    const r = this.auth.user()?.role;
    return r ? `${this.i18n.t('users.role')}: ${roleName(r)}` : '';
  });

  ngOnInit() {
    const u = this.auth.user();
    if (!u) return;
    this.name.set(u.name);
    this.email.set(u.email);
    this.firstName.set(u.firstName ?? '');
    this.lastName.set(u.lastName ?? '');
    this.website.set(u.website ?? '');
    this.bio.set(u.bio ?? '');
    this.avatar.set(u.avatar ?? '');
  }

  setLocale(l: Locale) { this.auth.setLocale(l); }

  async save() {
    this.busy.set(true);
    try {
      const data: Partial<User> & { password?: string; currentPassword?: string } = {
        name: this.name(), email: this.email(), firstName: this.firstName(), lastName: this.lastName(),
        website: this.website(), bio: this.bio(), avatar: this.avatar(),
      };
      if (this.password()) data.password = this.password();
      if (this.needsCurrent()) data.currentPassword = this.currentPassword();
      await this.auth.updateMe(data);
      this.password.set('');
      this.currentPassword.set('');
      Dialog.toast(this.i18n.t('common.saved'));
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
