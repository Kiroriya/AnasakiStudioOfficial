import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal, untracked } from '@angular/core';
import { Dialog } from '../core/dialog';
import { Router, RouterLink } from '@angular/router';
import { Api, SITE_ORIGIN, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { I18n, TPipe, localized } from '../core/i18n';
import { BUILTIN_POST_TYPES, Post, SchemaField, Settings, Term } from '../core/models';
import { PostTypes } from '../core/post-types';
import { editRoute, listColumns, metaCell, publicPath, supportsOf, typePrefix, typeSource } from '../core/post-type-utils';
import { starterPage } from '../editor/presets';
import './messages-admin';
import './messages-content-types';
import './messages-content-types2';
import { SelectValue } from '../shared/select-value';
import { Pager } from '../shared/pager';

/**
 * List screen shared by Posts / Pages / Templates and the post types of the
 * site code (`type` from the route data, or the :type of /content/:type).
 */
@Component({
  selector: 'rea-content-list',
  imports: [SelectValue, RouterLink, TPipe, Pager],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">{{ icon() }}</span>
      <div class="page-head-text">
        @if (custom()) {
          <h1 id="content-type-title">{{ typeLabel() }}</h1>
          @if (source()) {
            <p>{{ 'content.plugin.desc' | t: { name: typeLabel(), plugin: source() } }}</p>
          } @else if (pt()?.admin) {
            <p>{{ 'content.admin.desc' | t: { name: typeLabel() } }}</p>
          } @else {
            <p>{{ 'content.custom.desc' | t: { name: typeLabel() } }}</p>
          }
        } @else {
          <h1>{{ 'content.' + type() + '.title' | t }}</h1>
          <p>{{ 'content.' + type() + '.desc' | t }}</p>
        }
      </div>
      @if (type() === 'post' && auth.can('edit_others_posts')) {
        <button class="btn" (click)="openOrder()" [title]="'order.hint' | t">↕ {{ 'order.open' | t }}</button>
      }
      <button class="btn btn-primary" id="content-new" (click)="create()" [disabled]="creating()">+ {{ newLabel() }}</button>
    </header>

    @if (custom()) {
      @if (pt()?.public === false && source()) {
        <div class="info-box" id="content-plugin-tip">💡 {{ 'content.plugin.tip' | t: { plugin: source() } }}</div>
      } @else {
        <div class="info-box">💡 {{ 'content.custom.tip' | t: { slug: prefix() } }}</div>
      }
    }
    @else if (type() !== 'post') { <div class="info-box">💡 {{ 'content.' + type() + '.tip' | t }}</div> }
    @if (error()) { <p class="alert">{{ error() }}</p> }

    <section class="card table-card">
      <div class="table-toolbar">
        <input class="search" [placeholder]="'common.searchPh' | t" [value]="search()" (input)="search.set(val($event))" (keyup.enter)="page.set(1); load()" />
        <select [value]="status()" (change)="status.set(val($event)); page.set(1); load()">
          <option value="">{{ 'common.allStatus' | t }}</option>
          <option value="publish">{{ 'common.published' | t }}</option>
          <option value="draft">{{ 'common.draft' | t }}</option>
          <option value="scheduled">{{ 'post.scheduledBadge' | t }}</option>
        </select>
        @if (hasCategories()) {
          <select [value]="category()" (change)="category.set(val($event)); page.set(1); load()">
            <option value="">{{ 'content.allCategories' | t }}</option>
            @for (c of terms(); track c.id) { <option [value]="c.slug">{{ c.name }}</option> }
          </select>
        }
        <span class="grow"></span>
        <span class="muted small">{{ 'content.count' | t: { n: total() } }}</span>
      </div>
      <table class="table" id="content-table" [attr.data-type]="type()">
        <thead>
          <tr>
            <th>{{ 'common.name' | t }}</th>
            @if (type() !== 'template') { <th>{{ 'content.url' | t }}</th> }
            @for (f of metaCols(); track f.key) { <th class="content-meta-col" [attr.data-field]="f.key">{{ fieldLabel(f) }}</th> }
            @if (hasCategories()) { <th>{{ 'nav.categories' | t }}</th> }
            <th>{{ 'common.status' | t }}</th>
            <th>{{ 'common.updated' | t }}</th>
            <th class="th-actions">{{ 'common.actions' | t }}</th>
          </tr>
        </thead>
        <tbody>
          @for (p of items(); track p.id) {
            <tr class="content-row" [attr.data-id]="p.id">
              <td class="cell-title">
                @if (p.pinned) { <span class="pin-badge" [title]="'post.pinned' | t">📌</span> }
                <a class="row-title" [routerLink]="editLink(p)">{{ p.title }}</a>
                @if (roleOf(p); as r) { <span class="badge badge-info">{{ r | t }}</span> }
                <div class="muted small">{{ 'common.by' | t: { name: p.authorName } }}</div>
              </td>
              @if (type() !== 'template') { <td class="mono muted small cell-path">{{ path(p) }}</td> }
              @for (f of metaCols(); track f.key) { <td class="content-meta-cell small" [attr.data-field]="f.key">{{ cell(f, p) }}</td> }
              @if (hasCategories()) {
                <td>@for (c of p.categories; track c.id) { <span class="chip">{{ c.name }}</span> } @empty { <span class="muted">—</span> }</td>
              }
              <td><span class="badge" [attr.data-status]="p.status" [class.badge-live]="p.status === 'publish'" [title]="p.status === 'scheduled' && p.publishedAt ? i18n.date(p.publishedAt, true) : ''">{{ p.status === 'scheduled' ? '⏰' : '●' }} {{ (p.status === 'publish' ? 'common.published' : p.status === 'scheduled' ? 'post.scheduledBadge' : 'common.draft') | t }}</span></td>
              <td class="muted small nowrap">{{ i18n.date(p.updatedAt, true) }}</td>
              <td class="row-actions">
                @if (designed()) { <a class="btn btn-sm btn-primary" [routerLink]="['/design', p.id]">🎨 {{ 'content.design' | t }}</a> }
                @else { <a class="btn btn-sm btn-primary content-edit" [routerLink]="editLink(p)">✏️ {{ 'common.edit' | t }}</a> }
                @if (type() !== 'template') {
                  @if (p.status === 'publish') { <a class="btn btn-sm" [href]="viewUrl(p)" target="_blank">↗ {{ 'common.view' | t }}</a> }
                  @else { <span class="btn btn-sm slot-hidden" aria-hidden="true">↗ {{ 'common.view' | t }}</span> }
                }
                <button class="btn btn-sm" (click)="toggleMore(p.id)">⋯</button>
                @if (more() === p.id) {
                  <div class="row-menu">
                    @if (type() === 'page' && auth.can('manage_options') && settings()['home_page_id'] !== '' + p.id) {
                      <button (click)="assign('home_page_id', p)">🏠 {{ 'content.setHome' | t }}</button>
                    }
                    @if (type() === 'template' && auth.can('manage_options')) {
                      <button (click)="assign('post_template_id', p)">📰 {{ 'content.useSingle' | t }}</button>
                      <button (click)="assign('archive_template_id', p)">🗂 {{ 'content.useArchive' | t }}</button>
                    }
                    @if (type() === 'post' && auth.can('edit_others_posts')) {
                      <button (click)="togglePin(p)">📌 {{ (p.pinned ? 'post.unpin' : 'post.pin') | t }}</button>
                    }
                    <button (click)="duplicate(p)">⧉ {{ 'common.duplicate' | t }}</button>
                    @if (canModify(p)) { <button class="danger" (click)="remove(p)">🗑 {{ 'common.delete' | t }}</button> }
                  </div>
                }
              </td>
            </tr>
          } @empty {
            <tr><td [attr.colspan]="6 + metaCols().length" class="empty-state">
              <div class="empty-icon">{{ icon() }}</div>
              <p>{{ 'content.empty' | t }}</p>
              <button class="btn btn-primary" (click)="create()" [disabled]="creating()">+ {{ newLabel() }}</button>
            </td></tr>
          }
        </tbody>
      </table>
      <rea-pager [page]="page()" [total]="total()" [perPage]="perPage" (pageChange)="go($event)" />
    </section>

    @if (ordering(); as list) {
      <div class="modal-backdrop" (click)="ordering.set(null)">
        <div class="modal order-modal" (click)="$event.stopPropagation()">
          <header class="modal-head"><h3>↕ {{ 'order.title' | t }}</h3><button class="btn btn-ghost btn-sm" (click)="ordering.set(null)">✕</button></header>
          <p class="hint">{{ 'order.help' | t }}</p>
          <div class="table-toolbar">
            <select [value]="orderCat()" (change)="orderCat.set(val($event)); openOrder()">
              <option value="">{{ 'content.allCategories' | t }}</option>
              @for (c of terms(); track c.id) { <option [value]="c.slug">{{ c.name }}</option> }
            </select>
          </div>
          <ol class="order-list">
            @for (p of list; track p.id; let i = $index) {
              <li draggable="true" [class.drag-over]="dragOver() === i" (dragstart)="dragFrom = i" (dragover)="$event.preventDefault(); dragOver.set(i)"
                  (dragleave)="dragOver.set(null)" (drop)="dropAt(i)" (dragend)="dragOver.set(null)">
                <span class="grip">⋮⋮</span>
                <span class="drag-num">{{ i + 1 }}</span>
                <span class="order-title">@if (p.pinned) { 📌 }{{ p.title }}</span>
                <button class="icon-btn" [disabled]="i === 0" (click)="moveOrder(i, -1)">↑</button>
                <button class="icon-btn" [disabled]="i === list.length - 1" (click)="moveOrder(i, 1)">↓</button>
              </li>
            }
          </ol>
          <div class="row-actions">
            <button class="btn" (click)="ordering.set(null)">{{ 'common.cancel' | t }}</button>
            <button class="btn btn-primary" [disabled]="savingOrder()" (click)="saveOrder()">{{ 'common.save' | t }}</button>
          </div>
        </div>
      </div>
    }
  `,
  host: { '(document:click)': 'closeMore($event)' },
})
export class ContentList {
  readonly auth = inject(Auth);
  readonly i18n = inject(I18n);
  private api = inject(Api);
  private router = inject(Router);
  private postTypes = inject(PostTypes);

  readonly perPage = 20;
  /** 'post' | 'page' | 'template' (route data) or a post type id from the site code (/content/:type). */
  readonly type = input('post', { transform: (v: string | null | undefined) => v || 'post' });
  readonly custom = computed(() => !BUILTIN_POST_TYPES.includes(this.type()));
  readonly pt = computed(() => (this.custom() ? this.postTypes.get(this.type()) : undefined));
  readonly typeLabel = computed(() => { this.i18n.locale(); return localized(this.pt()?.label) || this.type(); });
  /** The JS plugin that registered this type ('' = the site code). */
  readonly source = computed(() => typeSource(this.pt()));
  readonly prefix = computed(() => typePrefix(this.pt() ?? { type: this.type() }));
  /** Pages and templates open in the designer; posts and post types in the post editor. */
  readonly designed = computed(() => this.type() === 'page' || this.type() === 'template');
  readonly hasCategories = computed(() => this.type() === 'post' || (this.custom() && supportsOf(this.pt()).has('categories')));
  /** A couple of short custom fields of the type itself as extra columns (plugin fields such as SEO stay in the editor). */
  readonly metaCols = computed(() => (this.designed() ? [] : listColumns(this.postTypes.groups(this.type()).site)));
  readonly newLabel = computed(() => {
    if (!this.custom()) return this.i18n.t('content.' + this.type() + '.new');
    return this.i18n.t('content.custom.new', { name: localized(this.pt()?.singular) || this.typeLabel() });
  });
  readonly items = signal<Post[]>([]);
  readonly total = signal(0);
  readonly page = signal(1);
  readonly search = signal('');
  readonly status = signal('');
  readonly category = signal('');
  readonly terms = signal<Term[]>([]);
  readonly settings = signal<Settings>({});
  readonly error = signal('');
  readonly more = signal<number | null>(null);
  /** Manual-order dialog: posts in their current hand-made order (null = closed). */
  readonly ordering = signal<Post[] | null>(null);
  readonly orderCat = signal('');
  readonly savingOrder = signal(false);
  readonly dragOver = signal<number | null>(null);
  dragFrom = -1;
  readonly icon = computed(() => ({ post: '📝', page: '📄', template: '🧩' } as Record<string, string>)[this.type()] ?? (this.pt()?.icon || '📦'));

  constructor() {
    this.api.settings().then(s => this.settings.set(s)).catch(() => undefined);
    void this.postTypes.ensureLoaded();
    // one screen serves every type: going from /content/a to /content/b starts over
    effect(() => {
      const type = this.type();
      untracked(() => {
        this.items.set([]);
        this.total.set(0);
        this.page.set(1);
        this.search.set('');
        this.status.set('');
        this.category.set('');
        this.more.set(null);
        if (type === 'post' || !BUILTIN_POST_TYPES.includes(type)) this.api.terms().then(t => this.terms.set(t)).catch(() => undefined);
        void this.load();
      });
    });
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  toggleMore(id: number) { this.more.set(this.more() === id ? null : id); }
  closeMore(e: MouseEvent) { if (!(e.target as HTMLElement).closest('.row-actions')) this.more.set(null); }

  async load() {
    try {
      const res = await this.api.posts({ type: this.type(), search: this.search(), status: this.status(), category: this.category(), page: this.page(), perPage: this.perPage });
      this.items.set(res.items);
      this.total.set(res.total);
      this.error.set('');
    } catch (e) {
      this.error.set(errorMessage(e));
    }
  }

  go(p: number) { this.page.set(p); this.load(); }

  editLink(p: Post) { return p.type === 'page' || p.type === 'template' ? ['/design', p.id] : editRoute(p.type, p.id); }
  path(p: Post) {
    if (this.custom()) return publicPath(this.pt() ?? { type: this.type() }, p.slug);
    return this.settings()['home_page_id'] === String(p.id) ? '/' : `/${p.type === 'post' ? 'blog/' : ''}${p.slug}`;
  }
  viewUrl(p: Post) { return this.custom() ? SITE_ORIGIN + this.path(p) : `${SITE_ORIGIN}/${p.type === 'post' ? 'blog/' : ''}${p.slug}`; }
  fieldLabel(f: SchemaField) { this.i18n.locale(); return localized(f.label as never); }
  cell(f: SchemaField, p: Post) { return metaCell(f, p.meta?.[f.key]); }

  roleOf(p: Post): string {
    const s = this.settings();
    const id = String(p.id);
    if (s['home_page_id'] === id) return 'content.badge.home';
    if (s['post_template_id'] === id) return 'content.badge.single';
    if (s['archive_template_id'] === id) return 'content.badge.archive';
    return '';
  }

  async create() {
    if (!this.designed()) {
      this.router.navigate(editRoute(this.type()));
      return;
    }
    if (this.creating()) return;
    // 2.1.1: ask for the name first (it also becomes the address) — a click used to create
    // "หน้าใหม่" at once, and a second click while it loaded made another one
    this.creating.set(true);
    const title = (await Dialog.prompt({
      title: this.newLabel(), label: this.i18n.t('content.newName'), value: this.i18n.t(`content.${this.type()}.newTitle`), required: true,
    }))?.trim();
    if (!title) { this.creating.set(false); return; }
    try {
      const design = starterPage(await this.api.components(), this.type() === 'template' ? 'template' : 'page');
      const p = await this.api.createPost({ type: this.type(), title, design });
      this.router.navigate(['/design', p.id]);
    } catch (e) {
      this.error.set(errorMessage(e));
      this.creating.set(false);
    }
  }
  readonly creating = signal(false);

  async assign(key: string, p: Post) {
    this.more.set(null);
    try { this.settings.set(await this.api.saveSettings({ [key]: String(p.id) })); } catch (e) { this.error.set(errorMessage(e)); }
  }

  async togglePin(p: Post) {
    this.more.set(null);
    try {
      await this.api.updatePost(p.id, { pinned: !p.pinned });
      await this.load();
      Dialog.toast(this.i18n.t(p.pinned ? 'post.unpinnedMsg' : 'post.pinnedMsg'));
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  async openOrder() {
    try {
      const res = await this.api.posts({ type: 'post', status: 'publish', category: this.orderCat(), order: 'manual', perPage: 200 });
      this.ordering.set(res.items);
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  moveOrder(i: number, d: number) {
    const list = [...(this.ordering() ?? [])];
    const j = i + d;
    if (j < 0 || j >= list.length) return;
    [list[i], list[j]] = [list[j], list[i]];
    this.ordering.set(list);
  }

  dropAt(i: number) {
    this.dragOver.set(null);
    const from = this.dragFrom;
    const list = [...(this.ordering() ?? [])];
    if (from < 0 || from === i || !list[from]) return;
    const [m] = list.splice(from, 1);
    list.splice(i, 0, m);
    this.ordering.set(list);
    this.dragFrom = -1;
  }

  async saveOrder() {
    const list = this.ordering();
    if (!list) return;
    this.savingOrder.set(true);
    try {
      await this.api.orderPosts('post', list.map(p => p.id));
      this.ordering.set(null);
      Dialog.toast(this.i18n.t('order.saved'));
    } catch (e) { this.error.set(errorMessage(e)); } finally { this.savingOrder.set(false); }
  }

  async duplicate(p: Post) {
    this.more.set(null);
    try { await this.api.duplicatePost(p.id); await this.load(); } catch (e) { this.error.set(errorMessage(e)); }
  }

  /** Published content needs publish_posts to change or delete (the server enforces it too). */
  canModify(p: Post) { return p.status === 'draft' || this.auth.can('publish_posts'); }

  async remove(p: Post) {
    this.more.set(null);
    // 2.1.1: the server also removes the menu links to the page — say which menus first
    const at = this.path(p);
    const norm = (u: string) => { try { u = decodeURIComponent(u); } catch { /* keep */ } return u.length > 1 ? u.replace(/\/+$/, '') : u; };
    let inMenus: string[] = [];
    if (at !== '/' && p.type !== 'template') {
      try { inMenus = (await this.api.menus()).filter(m => m.items.some(it => norm(it.url) === norm(at))).map(m => m.name); } catch { /* ask without it */ }
    }
    const text = inMenus.length ? this.i18n.t('list.deleteInMenus', { menus: inMenus.join(', ') }) : undefined;
    if (!(await Dialog.confirmDelete(p.title, text))) return;
    try { await this.api.deletePost(p.id); await this.load(); } catch (e) { this.error.set(errorMessage(e)); }
  }
}
