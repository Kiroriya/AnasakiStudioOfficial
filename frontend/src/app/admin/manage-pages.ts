import { siteRoleOf } from '../editor/site-role';
import { ChangeDetectionStrategy, Component, OnDestroy, OnInit, computed, inject, input, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { Api, SITE_ORIGIN, errorMessage } from '../core/api';
import { Dialog } from '../core/dialog';
import { I18n, TPipe, localized } from '../core/i18n';
import { PostTypes } from '../core/post-types';
import { termTree } from '../core/terms';
import { DesignComponent, Menu, MenuItem, Post, Term } from '../core/models';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { fitFrame, freshIds, previewDoc } from '../editor/component-starters';
import { NewComponent, NewComponentResult } from '../editor/new-component';
import './messages-admin';

// ===================================================================== Categories

import { Pager, pageSlice } from '../shared/pager';
import { safeUrl } from '../shared/safe-url';

/**
 * Terms of a taxonomy: /categories ("category") and, since CMS 2.1.0,
 * /terms/:taxonomy for the taxonomies registered with site.taxonomy
 * (parent and description when hierarchical; archive links when public).
 */
@Component({
  selector: 'rea-categories',
  imports: [FormsModule, TPipe, Pager, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">🏷</span>
      <div class="page-head-text" id="terms-page" [attr.data-taxonomy]="taxonomy()">
        <h1>{{ builtin() ? ('nav.categories' | t) : label() }}</h1>
        <p>{{ builtin() ? ('cat.desc' | t) : ('terms.desc' | t: { name: label() }) }}</p>
      </div>
    </header>
    @if (public()) {
      <div class="info-box">🧩 {{ 'cat.lookHint' | t }}
        @if (archiveId()) { <a class="btn btn-sm" [routerLink]="['/design', archiveId()]">🎨 {{ 'cat.lookEdit' | t }}</a> }
        @else { <a class="btn btn-sm" routerLink="/templates">{{ 'post.lookCreate' | t }}</a> }
      </div>
    }
    @if (error()) { <p class="alert">{{ error() }}</p> }
    <div class="grid-side">
      <form class="card" (ngSubmit)="save()" id="term-form">
        <h3>{{ (editing() ? 'cat.edit' : 'cat.add') | t }}</h3>
        <label>{{ 'common.name' | t }} <input name="name" id="term-name" [(ngModel)]="name" required /></label>
        <label>{{ 'cat.slug' | t }} <span class="muted small">{{ 'common.optional' | t }}</span><input name="slug" id="term-slug" [(ngModel)]="slug" [placeholder]="'common.auto' | t" /></label>
        @if (hierarchical()) {
          <label>{{ 'terms.parent' | t }}
            <select name="parent" id="term-parent" [(ngModel)]="parent">
              <option [ngValue]="0">{{ 'terms.noParent' | t }}</option>
              @for (o of parentOptions(); track o.term.id) { <option [ngValue]="o.term.id">{{ o.indent }}{{ o.term.name }}</option> }
            </select>
          </label>
        }
        @if (!builtin()) {
          <label>{{ 'terms.description' | t }} <span class="muted small">{{ 'common.optional' | t }}</span>
            <textarea name="description" id="term-description" rows="3" maxlength="500" [(ngModel)]="description"></textarea>
          </label>
        }
        <div class="row-actions">
          <button class="btn btn-primary" id="term-save">{{ (editing() ? 'common.save' : 'common.add') | t }}</button>
          @if (editing()) { <button type="button" class="btn" (click)="reset()">{{ 'common.cancel' | t }}</button> }
        </div>
      </form>
      <section class="card table-card">
        <table class="table" id="terms-table">
          <thead><tr><th>{{ 'common.name' | t }}</th><th>{{ 'content.url' | t }}</th><th>{{ 'cat.count' | t }}</th><th class="th-actions">{{ 'common.actions' | t }}</th></tr></thead>
          <tbody>
            @for (o of shown(); track o.term.id) {
              <tr [attr.data-term]="o.term.slug">
                <td><span class="muted">{{ o.indent }}</span><b>{{ o.term.name }}</b>
                  @if (o.term.description) { <div class="muted small">{{ o.term.description }}</div> }
                </td>
                <td class="mono muted small">@if (public()) { /{{ base() }}/{{ o.term.slug }} } @else { — }</td>
                <td>{{ o.term.count }}</td>
                <td class="row-actions">
                  @if (public()) { <a class="btn btn-sm" [href]="site + '/' + base() + '/' + o.term.slug" target="_blank">↗</a> }
                  <button class="btn btn-sm" (click)="edit(o.term)">{{ 'common.edit' | t }}</button>
                  <button class="btn btn-sm btn-danger" (click)="remove(o.term)">{{ 'common.delete' | t }}</button>
                </td>
              </tr>
            } @empty { <tr><td colspan="4" class="empty-state">{{ 'cat.empty' | t }}</td></tr> }
          </tbody>
        </table>
        <rea-pager [page]="page()" [total]="tree().length" [perPage]="20" (pageChange)="page.set($event)" />
      </section>
    </div>
  `,
})
export class Categories implements OnInit {
  private api = inject(Api);
  private i18n = inject(I18n);
  private postTypes = inject(PostTypes);
  readonly site = SITE_ORIGIN;
  /** Route param (/terms/:taxonomy); /categories leaves it empty. */
  readonly taxonomy = input('category', { transform: (v: string | null | undefined) => v || 'category' });
  readonly info = computed(() => this.postTypes.taxonomy(this.taxonomy()));
  readonly builtin = computed(() => this.taxonomy() === 'category');
  readonly label = computed(() => { this.i18n.locale(); return localized(this.info()?.label) || this.taxonomy(); });
  readonly hierarchical = computed(() => !!this.info()?.hierarchical);
  readonly public = computed(() => this.builtin() || !!this.info()?.public);
  readonly base = computed(() => (this.builtin() ? 'category' : this.info()?.slug || this.taxonomy()));
  readonly terms = signal<Term[]>([]);
  /** Terms in tree order (children under their parent) with an indent. */
  readonly tree = computed(() => termTree(this.terms()));
  readonly page = signal(1);
  readonly shown = computed(() => pageSlice(this.tree(), this.page(), 20));
  /** Possible parents: every term except the one edited and its descendants. */
  readonly parentOptions = computed(() => {
    const e = this.editing();
    if (!e) return this.tree();
    const banned = new Set([e.id]);
    for (const o of this.tree()) if (o.term.parent && banned.has(o.term.parent)) banned.add(o.term.id);
    return this.tree().filter(o => !banned.has(o.term.id));
  });
  readonly editing = signal<Term | null>(null);
  readonly error = signal('');
  readonly archiveId = signal(0);
  name = '';
  slug = '';
  parent = 0;
  description = '';

  ngOnInit() {
    void this.postTypes.ensureLoaded();
    this.load();
    this.api.settings().then(s => this.archiveId.set(Number(s['archive_template_id']) || 0)).catch(() => undefined);
  }
  async load() { try { this.terms.set(await this.api.terms(this.taxonomy())); } catch (e) { this.error.set(errorMessage(e)); } }
  edit(c: Term) { this.editing.set(c); this.name = c.name; this.slug = c.slug; this.parent = c.parent || 0; this.description = c.description || ''; }
  reset() { this.editing.set(null); this.name = ''; this.slug = ''; this.parent = 0; this.description = ''; }

  async save() {
    try {
      const e = this.editing();
      const body: Partial<Term> = { name: this.name, slug: this.slug, parent: this.hierarchical() ? Number(this.parent) || 0 : 0 };
      if (!this.builtin()) body.description = this.description;
      if (e) await this.api.updateTerm(e.id, body);
      else await this.api.createTerm({ ...body, taxonomy: this.taxonomy() });
      this.reset();
      await this.load();
      Dialog.toast(this.i18n.t('common.saved'));
    } catch (err) { this.error.set(errorMessage(err)); }
  }

  async remove(c: Term) {
    if (!(await Dialog.confirmDelete(c.name, this.i18n.t('cat.deleteText')))) return;
    try { await this.api.deleteTerm(c.id); await this.load(); } catch (e) { this.error.set(errorMessage(e)); }
  }
}

// ===================================================================== Menus (autosave)

@Component({
  selector: 'rea-menus',
  imports: [FormsModule, TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">🧭</span>
      <div class="page-head-text"><h1>{{ 'nav.menus' | t }}</h1><p>{{ 'menus.desc' | t }}</p></div>
      <button class="btn btn-primary" (click)="create()">+ {{ 'menus.new' | t }}</button>
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }
    <div class="tabs-row">
      @for (m of menus(); track m.id) {
        <button class="tab" [class.on]="current()?.id === m.id" (click)="open(m)">{{ m.name }}</button>
      } @empty { <p class="muted">{{ 'menus.none' | t }}</p> }
    </div>
    @if (current(); as m) {
      <div class="grid-side">
        <section class="card">
          <h3>① {{ 'menus.addLinks' | t }}</h3>
          <p class="hint">{{ 'menus.addHint' | t }}</p>
          <h4>{{ 'nav.pages' | t }}</h4>
          <div class="pick-list">
            @for (o of pageOptions(); track o.url) {
              <button [class.in-menu]="inMenu(o.url)" (click)="toggleItem(o.label, o.url)" [title]="o.url + ' — ' + ((inMenu(o.url) ? 'menus.clickRemove' : 'menus.clickAdd') | t)">
                {{ inMenu(o.url) ? '✓' : '+' }} {{ o.label }}
              </button>
            }
          </div>
          <h4>{{ 'nav.categories' | t }}</h4>
          <div class="pick-list">
            @for (c of terms(); track c.id) {
              <button [class.in-menu]="inMenu('/category/' + c.slug)" (click)="toggleItem(c.name, '/category/' + c.slug)" [title]="'/category/' + c.slug">
                {{ inMenu('/category/' + c.slug) ? '✓' : '+' }} {{ c.name }}
              </button>
            }
          </div>
          <h4>{{ 'menus.custom' | t }}</h4>
          <input [placeholder]="'menus.customLabel' | t" [(ngModel)]="customLabel" />
          <input placeholder="https://…" [(ngModel)]="customUrl" />
          <button class="btn btn-sm" (click)="addCustom()">{{ 'common.add' | t }}</button>
        </section>
        <section class="card">
          <div class="card-head">
            <h3>② {{ 'menus.items' | t }}</h3>
            <span class="save-state" [class.busy]="saving()">{{ (saving() ? 'menus.saving' : 'menus.autosaved') | t }}</span>
          </div>
          <label>{{ 'menus.name' | t }} <span class="muted small">{{ 'menus.nameHint' | t }}</span><input [value]="m.name" (input)="rename($event)" /></label>
          <ul class="menu-items">
            @for (it of m.items; track $index; let i = $index) {
              <li>
                <span class="drag-num">{{ i + 1 }}</span>
                <input [value]="it.label" (input)="setItem(i, 'label', $event)" />
                <input class="mono" [value]="it.url" (input)="setItem(i, 'url', $event)" [style.border-color]="badUrls().has(i) ? 'var(--danger)' : null"
                       [attr.aria-invalid]="badUrls().has(i)" [title]="badUrls().has(i) ? ('url.unsafe' | t) : ''" />
                <button class="icon-btn" [disabled]="i === 0" (click)="moveItem(i, -1)" [title]="'menus.up' | t">↑</button>
                <button class="icon-btn" [disabled]="i === m.items.length - 1" (click)="moveItem(i, 1)" [title]="'menus.down' | t">↓</button>
                <button class="btn btn-sm btn-danger item-del" (click)="removeItem(i)" [title]="'menus.removeLink' | t">🗑 {{ 'common.delete' | t }}</button>
              </li>
            } @empty { <li class="muted">{{ 'menus.emptyItems' | t }}</li> }
          </ul>
          @if (badUrls().size) { <p class="alert" role="alert">⚠ {{ 'url.unsafe' | t }}</p> }
          <div class="row-actions">
            <button class="btn btn-danger" (click)="remove()">🗑 {{ 'menus.delete' | t }}</button>
          </div>
        </section>
      </div>
    }
  `,
})
export class Menus implements OnInit, OnDestroy {
  readonly i18n = inject(I18n);
  private api = inject(Api);
  readonly menus = signal<Menu[]>([]);
  readonly current = signal<Menu | null>(null);
  readonly pages = signal<Post[]>([]);
  readonly terms = signal<Term[]>([]);
  readonly error = signal('');
  readonly saving = signal(false);
  private timer?: ReturnType<typeof setTimeout>;
  customLabel = '';
  customUrl = '';
  private homeId = signal('');

  /** Pages to offer: the home page links to "/", and the blog listing is offered once. */
  readonly pageOptions = computed(() => {
    const out = this.pages().map(p => ({ label: p.title, url: String(p.id) === this.homeId() || p.slug === 'home' ? '/' : '/' + p.slug }));
    if (!out.some(o => o.url === '/blog')) out.push({ label: this.i18n.t('menus.blog'), url: '/blog' });
    return out.filter((o, i) => out.findIndex(x => x.url === o.url) === i);
  });

  inMenu(url: string) { return !!this.current()?.items.some(it => it.url === url); }

  /** Click once to add; click a ✓ item again to take it out of the menu. */
  toggleItem(label: string, url: string) {
    const i = this.current()?.items.findIndex(it => it.url === url) ?? -1;
    if (i >= 0) this.removeItem(i);
    else this.addItem(label, url);
  }

  async ngOnInit() {
    try {
      const [m, p, t] = await Promise.all([this.api.menus(), this.api.posts({ type: 'page', perPage: 100 }), this.api.terms()]);
      this.api.settings().then(st => this.homeId.set(st['home_page_id'] ?? '')).catch(() => undefined);
      this.menus.set(m);
      this.pages.set(p.items);
      this.terms.set(t);
      if (m[0]) this.open(m[0]);
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  ngOnDestroy() { this.flush(); }

  open(m: Menu) { this.flush(); this.badUrls.set(new Set()); this.current.set(structuredClone(m)); }

  /** Every change is saved automatically (debounced) — nothing is lost on reload. */
  private patch(fn: (m: Menu) => void) {
    const m = structuredClone(this.current()!);
    fn(m);
    this.current.set(m);
    this.saving.set(true);
    clearTimeout(this.timer);
    this.timer = setTimeout(() => this.save(), 500);
  }

  private flush() {
    if (this.timer) { clearTimeout(this.timer); this.timer = undefined; this.save(); }
  }

  rename(e: Event) { this.patch(m => (m.name = (e.target as HTMLInputElement).value)); }
  /** Rows whose typed address was rejected (not saved until fixed). */
  readonly badUrls = signal<ReadonlySet<number>>(new Set());
  addItem(label: string, url: string) {
    const safe = safeUrl(url);
    if (safe === null) { Dialog.error(this.i18n.t('url.unsafe')); return false; }
    if (label && safe) this.patch(m => m.items.push({ label, url: safe }));
    return true;
  }
  addCustom() {
    if (this.addItem(this.customLabel, this.customUrl)) { this.customLabel = ''; this.customUrl = ''; }
  }
  setItem(i: number, k: keyof MenuItem, e: Event) {
    let v = (e.target as HTMLInputElement).value;
    if (k === 'url') {
      const safe = safeUrl(v);
      this.badUrls.update(s => { const n = new Set(s); if (safe === null) n.add(i); else n.delete(i); return n; });
      if (safe === null) return; // keep the last safe address saved until this one is fixed
      v = safe;
    }
    this.patch(m => (m.items[i][k] = v));
  }
  /** Removes one link; the toast offers an undo. */
  async removeItem(i: number) {
    const item = this.current()?.items[i];
    if (!item) return;
    this.badUrls.set(new Set());
    this.patch(m => m.items.splice(i, 1));
    const r = await Dialog.undoToast(this.i18n.t('menus.linkRemoved', { name: item.label || item.url }), this.i18n.t('menus.undo'));
    if (r) this.patch(m => m.items.splice(Math.min(i, m.items.length), 0, item));
  }
  moveItem(i: number, d: number) {
    this.badUrls.set(new Set());
    this.patch(m => {
      const j = i + d;
      if (j < 0 || j >= m.items.length) return;
      [m.items[i], m.items[j]] = [m.items[j], m.items[i]];
    });
  }

  async save() {
    const m = this.current();
    this.timer = undefined;
    if (!m) return;
    try {
      const saved = await this.api.saveMenu(m);
      this.menus.update(ms => ms.map(x => (x.id === saved.id ? saved : x)));
    } catch (e) { this.error.set(errorMessage(e)); } finally { this.saving.set(false); }
  }

  async create() {
    const name = await Dialog.prompt({ title: this.i18n.t('menus.newPrompt'), value: this.i18n.t('menus.newDefault') });
    if (!name) return;
    try {
      const m = await this.api.saveMenu({ id: 0, name, items: [] });
      this.menus.update(ms => [...ms, m]);
      this.open(m);
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  async remove() {
    const m = this.current();
    if (!m || !(await Dialog.confirmDelete(m.name))) return;
    clearTimeout(this.timer);
    this.timer = undefined;
    try {
      await this.api.deleteMenu(m.id);
      const rest = this.menus().filter(x => x.id !== m.id);
      this.menus.set(rest);
      this.current.set(rest[0] ? structuredClone(rest[0]) : null);
      Dialog.toast(this.i18n.t('menus.deleted'));
    } catch (e) { this.error.set(errorMessage(e)); }
  }
}

// ===================================================================== Components


@Component({
  selector: 'rea-components',
  imports: [RouterLink, TPipe, NewComponent],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">◈</span>
      <div class="page-head-text"><h1>{{ 'nav.components' | t }}</h1><p>{{ 'comp.desc' | t }}</p></div>
      <div class="row-actions">
        <button class="btn" (click)="newIn.set('header')">+ {{ 'cs.g.header' | t }}</button>
        <button class="btn" (click)="newIn.set('footer')">+ {{ 'cs.g.footer' | t }}</button>
        <button class="btn btn-primary" (click)="newIn.set('section')">+ {{ 'comp.new' | t }}</button>
      </div>
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }
    <div class="card-grid">
      @for (c of items(); track c.id) {
        @let role = roleOf(c);
        <div class="card comp-card">
          <a class="comp-thumb comp-thumb-live" [routerLink]="['/design/component', c.id]" [attr.aria-label]="c.name">
            @if (thumbs()[c.id]; as h) { <iframe sandbox="allow-same-origin" [srcdoc]="h" (load)="fit($event)" tabindex="-1" title=""></iframe> } @else { ◈ }
            @if (role) { <span class="comp-role">{{ (role === 'header' ? 'cs.isHeader' : 'cs.isFooter') | t }}</span> }
          </a>
          <h3>{{ c.name }}</h3>
          <p class="muted small">
            @if (usage()[c.id]; as n) { {{ 'cs.usedIn' | t: { n } }} } @else { {{ 'cs.unused' | t }} }
            · {{ i18n.date(c.updatedAt ?? '') }}
          </p>
          <div class="row-actions">
            <a class="btn btn-sm btn-primary" [routerLink]="['/design/component', c.id]">🎨 {{ 'content.design' | t }}</a>
            <button class="btn btn-sm" (click)="duplicate(c)">⧉ {{ 'cs.duplicate' | t }}</button>
            @if (!role && c.node.props?.tag === 'header') { <button class="btn btn-sm" (click)="setRole(c, 'header')">{{ 'cs.setHeader' | t }}</button> }
            @if (!role && c.node.props?.tag === 'footer') { <button class="btn btn-sm" (click)="setRole(c, 'footer')">{{ 'cs.setFooter' | t }}</button> }
            <button class="btn btn-sm btn-danger" (click)="remove(c)">{{ 'common.delete' | t }}</button>
          </div>
        </div>
      } @empty { <div class="card empty-state">{{ 'comp.empty' | t }}</div> }
    </div>
    @if (newIn(); as g) {
      <rea-new-component [startGroup]="g" (closed)="newIn.set(null)" (created)="opened($event)" />
    }
  `,
})
export class Components implements OnInit {
  readonly i18n = inject(I18n);
  private api = inject(Api);
  private router = inject(Router);
  private sanitizer = inject(DomSanitizer);
  readonly items = signal<DesignComponent[]>([]);
  readonly error = signal('');
  readonly newIn = signal<'header' | 'footer' | 'section' | null>(null);
  readonly thumbs = signal<Record<number, SafeHtml>>({});
  readonly usage = signal<Record<number, number>>({});
  /** Header/footer chosen with "Set as header/footer" (site options), else by name for old sites — same rule as the site renderer. */
  private readonly roleSettings = signal<Record<string, string>>({});
  roleOf(c: DesignComponent) { return siteRoleOf(c, this.items(), this.roleSettings()); }

  ngOnInit() { this.load(); }
  async load() {
    try {
      const [cs, st] = await Promise.all([this.api.components(), this.api.settings().catch(() => ({}))]);
      this.roleSettings.set(st);
      this.items.set(cs);
      this.loadDetails(cs);
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  /** Live thumbnails (site renderer, sandboxed frame) and usage counts. */
  private loadDetails(cs: DesignComponent[]) {
    for (const c of cs) {
      this.api.preview(previewDoc(c.node), undefined, c.name)
        .then(h => this.thumbs.update(t => ({ ...t, [c.id]: this.sanitizer.bypassSecurityTrustHtml(h) })))
        .catch(() => undefined);
      this.api.componentUsage(c.id)
        .then(u => this.usage.update(m => ({ ...m, [c.id]: u.length })))
        .catch(() => undefined);
    }
  }

  fit(e: Event) { fitFrame(e.target as HTMLIFrameElement, 1280, true); }

  opened(r: NewComponentResult) {
    this.newIn.set(null);
    this.router.navigate(['/design/component', r.component.id]);
  }

  async duplicate(c: DesignComponent) {
    try {
      await this.api.saveComponent({ name: this.i18n.t('cs.copyOf', { name: c.name }), node: freshIds(c.node) });
      await this.load();
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  async setRole(c: DesignComponent, role: 'header' | 'footer') {
    const roleName = this.i18n.t(role === 'header' ? 'cs.roleHeader' : 'cs.roleFooter');
    if (!(await Dialog.confirm({ title: this.i18n.t('cs.setConfirm', { name: c.name, role: roleName }) }))) return;
    try {
      const r = await this.api.setComponentSiteRole(c.id, role);
      Dialog.toast(this.i18n.t('cs.madeSite', { role: roleName, n: r.replaced + r.added }));
      await this.load();
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  async remove(c: DesignComponent) {
    let text = this.i18n.t('comp.deleteText');
    try {
      const uses = await this.api.componentUsage(c.id);
      if (uses.length) text = this.i18n.t('cs.deleteUsed', { list: uses.slice(0, 6).map(u => u.title || '#' + u.id).join(', ') + (uses.length > 6 ? ' …' : '') });
    } catch { /* fall back to the generic warning */ }
    if (!(await Dialog.confirmDelete(c.name, text))) return;
    try { await this.api.deleteComponent(c.id); await this.load(); } catch (e) { this.error.set(errorMessage(e)); }
  }
}
