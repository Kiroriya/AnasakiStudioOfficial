import { ChangeDetectionStrategy, Component, OnInit, computed, inject, input, signal, viewChild } from '@angular/core';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { Api, SITE_ORIGIN, errorMessage } from '../core/api';
import { Auth } from '../core/auth';
import { Dialog } from '../core/dialog';
import { I18n, TPipe, localized } from '../core/i18n';
import { BUILTIN_POST_TYPES, Media, Post, PostInput, PostStatus, Revision, Taxonomy, Term } from '../core/models';
import { termTree } from '../core/terms';
import { Plugins } from '../core/plugins';
import { PostTypes } from '../core/post-types';
import { editRoute, metaErrorsFromApi, pickMeta, supportsOf, templateOption, typePrefix, validateMeta } from '../core/post-type-utils';
import { MediaPicker } from '../shared/media-picker';
import { langLabel, parseLangs } from '../shared/languages';
import { RichEditor } from '../shared/rich-editor';
import { SchemaForm } from '../shared/schema-form';
import './messages-content-types';

import './messages-v250';
/**
 * Post editor: title, rich text, excerpt, categories, featured image, SEO,
 * revisions — and custom fields (site.fields / site.postType). Also the
 * editor of the site code's post types (`type` = /content/:type).
 */
@Component({
  selector: 'rea-post-edit',
  imports: [MediaPicker, TPipe, RichEditor, RouterLink, SchemaForm],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">{{ custom() ? (pt()?.icon || '📦') : id() ? '✏️' : '✍️' }}</span>
      <div class="page-head-text">
        @if (custom()) {
          <h1 id="post-type-title">{{ (id() ? 'post.editType' : 'post.newType') | t: { name: singular() } }}</h1>
        } @else {
          <h1>{{ (id() ? 'post.edit' : 'post.new') | t }}</h1>
        }
        <p>{{ 'post.sub' | t }}</p>
      </div>
      <div class="head-actions">
        @if (post()?.status === 'publish') { <a class="btn" [href]="viewUrl()" target="_blank">↗ {{ 'common.view' | t }}</a> }
        <button class="btn" id="post-save-draft" [disabled]="busy() || locked()" (click)="save('draft')" [title]="'post.draftHint' | t">💾 {{ 'post.saveDraft' | t }}</button>
        <button class="btn btn-primary" id="post-publish" [disabled]="busy() || locked()" (click)="save('publish')">{{ !mayPublish() ? '📨' : future() ? '⏰' : '🚀' }} {{ (!mayPublish() ? (post()?.status === 'pending' ? 'post.updateReview' : 'post.submitReview') : future() ? 'post.schedule' : post()?.status === 'publish' ? 'post.update' : post()?.status === 'pending' ? 'post.approve' : 'post.publish') | t }}</button>
      </div>
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }
    @if (locked()) { <p class="alert">🔒 {{ 'post.lockedPublished' | t }}</p> }

    <div class="editor-grid">
      <div class="card editor-main">
        <input class="title-input" id="post-title" [placeholder]="'post.titlePh' | t" [value]="title()" (input)="title.set(val($event))" />
        <div class="slug-row muted">
          {{ 'post.url' | t }}: {{ urlPrefix() }}<input class="slug-input" id="post-slug" [value]="slug()" [placeholder]="'common.auto' | t" (input)="slug.set(val($event))" />
        </div>
        @if (supports().has('content')) {
          <rea-rich-editor #editor [value]="content()" (valueChange)="content.set($event)" [placeholder]="'post.bodyPh' | t" />
        }
        @if (supports().has('excerpt')) {
          <label class="block">{{ 'post.excerpt' | t }} <span class="muted small">{{ 'post.excerptHint' | t }}</span>
            <textarea rows="3" [value]="excerpt()" (input)="excerpt.set(val($event))" [placeholder]="'post.excerptPh' | t"></textarea>
          </label>
        }
        @if (siteFields().length) {
          <section class="post-fields" id="post-fields" [attr.data-type]="type()">
            <h4>🧾 {{ 'post.fields' | t }}</h4>
            <p class="hint">{{ (pt()?.admin ? 'post.fieldsHintAdmin' : 'post.fieldsHint') | t }}</p>
            <rea-schema-form idPrefix="post-field" [fields]="siteFields()" [values]="meta()" [errors]="fieldErrors()" [blankOption]="true" (changed)="setMeta($event)" />
          </section>
        }
        @for (g of pluginFields(); track g.plugin) {
          <section class="post-fields plugin-fields" [attr.data-plugin]="g.plugin" [id]="'plugin-fields-' + g.plugin">
            <h4>🧩 {{ g.name }}</h4>
            <p class="hint">{{ 'post.pluginFieldsHint' | t: { name: g.name } }}</p>
            <rea-schema-form [idPrefix]="'plugin-field-' + g.plugin" [fields]="g.fields" [values]="meta()" [errors]="fieldErrors()" [blankOption]="true" (changed)="setMeta($event)" />
          </section>
        }
      </div>

      <aside class="editor-side">
        <section class="card">
          <h4>{{ 'post.publishBox' | t }}</h4>
          <p><span class="badge" id="post-status-badge" [attr.data-status]="post()?.status || 'draft'" [class.badge-live]="post()?.status === 'publish'">{{ post()?.status === 'scheduled' ? '⏰' : '●' }} {{ (post()?.status === 'publish' ? 'common.published' : post()?.status === 'scheduled' ? 'post.scheduledBadge' : post()?.status === 'pending' ? 'post.pendingBadge' : 'post.notLive') | t }}</span></p>
          @if (post(); as p) {
            @if (p.publishedAt) { <p class="muted small">{{ i18n.date(p.publishedAt, true) }}</p> }
            <p class="muted small">{{ 'common.by' | t: { name: p.authorName } }}</p>
          }
          @if (mayPublish()) {
            <label class="block small">{{ 'post.publishAt' | t }}
              <input type="datetime-local" id="post-publish-at" [value]="publishAt()" (input)="publishAt.set(val($event))" />
              <span class="hint">{{ 'post.publishAtHint' | t }}</span>
            </label>
          }
          @if (auth.can('edit_others_posts')) {
            <label class="check" [title]="'post.pinHint' | t"><input type="checkbox" [checked]="pinned()" (change)="pinned.set($any($event.target).checked)" /> 📌 {{ 'post.pin' | t }}</label>
          }
        </section>

        @if (langs().length && post(); as p) {
          <section class="card" id="post-translations">
            <h4>🌐 {{ 'lang.title' | t }}</h4>
            <p class="small">{{ 'lang.thisIs' | t: { lang: langLabel(p.lang || siteLang()) } }}</p>
            <ul class="plain-list">
              @for (tr of translations(); track tr.id) {
                @if (tr.id !== p.id) {
                  <li><a [routerLink]="editRouteOf(tr)" [attr.data-lang]="tr.lang">{{ langLabel(tr.lang || siteLang()) }}: {{ tr.title }}</a>
                    <span class="muted small">{{ (tr.status === 'publish' ? 'common.published' : 'common.draft') | t }}</span></li>
                }
              }
            </ul>
            @for (l of missingLangs(); track l) {
              <button class="btn btn-sm post-translate" [attr.data-lang]="l" (click)="translate(l)">+ {{ 'lang.translateTo' | t: { lang: langLabel(l) } }}</button>
            }
          </section>
        }

        <section class="card tpl-card">
          <h4>🧩 {{ 'post.lookTitle' | t }}</h4>
          <p class="hint">{{ 'post.lookHint' | t }}</p>
          @if (auth.can('edit_pages')) {
            @if (templateId()) {
              <a class="btn btn-sm" [routerLink]="['/design', templateId()]">🎨 {{ 'post.lookEdit' | t }}</a>
            } @else {
              <a class="btn btn-sm" routerLink="/templates">{{ 'post.lookCreate' | t }}</a>
            }
          }
        </section>

        @if (supports().has('categories')) {
        <section class="card">
          <h4>{{ 'nav.categories' | t }}</h4>
          <div class="check-list">
            @for (c of terms(); track c.id) {
              <label class="check"><input type="checkbox" [checked]="cats().has(c.id)" (change)="toggleCat(c.id)" /> {{ c.name }}</label>
            } @empty { <p class="muted small">{{ 'post.noCats' | t }}</p> }
          </div>
          @if (auth.can('manage_categories')) {
            <div class="inline-add">
              <input #newCat [placeholder]="'post.newCat' | t" />
              <button class="btn btn-sm" (click)="addCategory(newCat)">{{ 'common.add' | t }}</button>
            </div>
          }
        </section>
        }

        @for (tx of taxes(); track tx.id) {
          <section class="card tax-card" [attr.data-taxonomy]="tx.id" [id]="'post-tax-' + tx.id">
            <h4>🏷 {{ txLabel(tx) }}</h4>
            <div class="check-list">
              @for (o of txTree(tx.id); track o.term.id) {
                <label class="check" [style.padding-left.px]="o.depth * 16"><input type="checkbox" [attr.data-term]="o.term.slug" [checked]="txHas(tx.id, o.term.id)" (change)="txToggle(tx.id, o.term.id)" /> {{ o.term.name }}</label>
              } @empty { <p class="muted small">{{ 'terms.empty' | t }}</p> }
            </div>
            @if (tx.canManage) {
              <div class="inline-add">
                <input #newTerm [placeholder]="txLabel(tx)" [attr.data-new-term]="tx.id" />
                <button class="btn btn-sm" (click)="txAdd(tx.id, newTerm)">{{ 'common.add' | t }}</button>
              </div>
            }
          </section>
        }

        @if (supports().has('image')) {
        <section class="card">
          <h4>{{ 'post.featured' | t }}</h4>
          <p class="hint">{{ 'post.featuredHint' | t }}</p>
          @if (featured(); as f) { <img class="featured" [src]="f" alt="" /> }
          <div class="row-actions">
            <button class="btn btn-sm" (click)="picking.set(true)">🖼 {{ (featured() ? 'post.changeImage' : 'post.chooseImage') | t }}</button>
            @if (featured()) { <button class="btn btn-sm" (click)="featuredId.set(null); featured.set('')">{{ 'post.removeImage' | t }}</button> }
          </div>
        </section>
        }

        @if (revisions().length) {
          <section class="card">
            <h4>{{ 'post.revisions' | t: { n: revisions().length } }}</h4>
            <p class="hint">{{ 'post.revisionsHint' | t }}</p>
            <ul class="list small">
              @for (r of revisions(); track r.id) {
                <li><span>{{ i18n.date(r.createdAt, true) }} · {{ r.authorName }}</span>@if (!locked()) { <button class="link" (click)="restore(r)">{{ 'post.restore' | t }}</button> }</li>
              }
            </ul>
          </section>
        }
      </aside>
    </div>

    <rea-media-picker [open]="picking()" (openChange)="picking.set($event)" (picked)="picked($event)" />
  `,
  styles: `.post-fields { margin-top: 18px; padding-top: 14px; border-top: 1px solid var(--border); } .post-fields h4 { margin: 0 0 2px; }`,
})
export class PostEdit implements OnInit {
  readonly auth = inject(Auth);
  /** A published post can only be changed by someone who may publish (the server refuses otherwise). */
  /** Publishing: a custom type's own rule (canPublish, CMS 2.0.6), else publish_posts (the server decides too). */
  readonly mayPublish = computed(() => this.pt()?.canPublish ?? this.auth.can('publish_posts'));
  readonly locked = computed(() => !!this.post() && this.post()!.status !== 'draft' && !this.mayPublish());
  readonly i18n = inject(I18n);
  readonly plugins = inject(Plugins);
  private postTypes = inject(PostTypes);
  private api = inject(Api);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private editor = viewChild<RichEditor>('editor');

  readonly origin = SITE_ORIGIN || location.origin;
  /** 'post' (the classic editor) or a post type id of the site code (/content/:type). */
  readonly type = input('post', { transform: (v: string | null | undefined) => v || 'post' });
  readonly custom = computed(() => !BUILTIN_POST_TYPES.includes(this.type()));
  readonly pt = computed(() => this.postTypes.get(this.type()));
  readonly singular = computed(() => { this.i18n.locale(); const p = this.pt(); return localized(p?.singular) || localized(p?.label) || this.type(); });
  readonly supports = computed(() => supportsOf(this.pt(), this.type()));
  readonly urlPrefix = computed(() => (this.custom() ? `/${typePrefix(this.pt() ?? { type: this.type() })}/` : '/blog/'));
  /** Custom fields of this type (schema form under the content). */
  readonly fields = computed(() => this.postTypes.schema(this.type()));
  private readonly grouped = computed(() => this.postTypes.groups(this.type()));
  /** The site's own fields (site.fields / site.postType in Site Code). */
  readonly siteFields = computed(() => this.grouped().site);
  /** Fields added by JS plugins: one panel per plugin. */
  readonly pluginFields = computed(() => this.grouped().plugins);
  hasPluginPanel(id: string) { return this.pluginFields().some(g => g.plugin === id); }
  readonly meta = signal<Record<string, unknown>>({});
  /** Message per field: client checks, then the server's validation (400). */
  readonly fieldErrors = signal<Record<string, string>>({});
  readonly id = signal(0);
  readonly post = signal<Post | null>(null);
  /** 2.5.0: multilingual content — the site's content languages and this post's translation group. */
  readonly siteLang = signal('th');
  readonly langs = signal<string[]>([]);
  readonly translations = signal<{ id: number; lang: string; title: string; status: string; type: string }[]>([]);
  readonly missingLangs = computed(() => {
    const have = new Set(this.translations().map(t => t.lang));
    return this.langs().filter(l => !have.has(l));
  });
  readonly title = signal('');
  readonly slug = signal('');
  readonly content = signal('');
  readonly excerpt = signal('');
  readonly cats = signal(new Set<number>());
  readonly terms = signal<Term[]>([]);
  readonly featuredId = signal<number | null>(null);
  readonly featured = signal('');
  readonly revisions = signal<Revision[]>([]);
  readonly picking = signal(false);
  readonly pinned = signal(false);
  /** Taxonomies of this type from site.taxonomy (CMS 2.1.0; "category" has its own box). */
  // the loaded post's own type (a page opened at /posts/<id> must not get the tags of posts)
  readonly taxes = computed(() => this.postTypes.taxonomiesOf(this.post()?.type ?? this.type()));
  /** Their terms, and the ids chosen per taxonomy. */
  readonly txTerms = signal<Record<string, Term[]>>({});
  readonly txSel = signal<Record<string, Set<number>>>({});
  /** The publish date box (datetime-local, the browser's time zone); '' = now / unchanged. */
  readonly publishAt = signal('');
  /** A publish date in the future: "Publish" schedules the post. */
  /** The box as loaded: the date is sent only when it was changed (it has minutes, the server seconds). */
  private publishAt0 = '';
  readonly future = computed(() => { const t = Date.parse(this.publishAt()); return !isNaN(t) && t > Date.now(); });
  /** The post template's id (to open "how every post looks" in the designer). */
  readonly templateId = signal(0);
  readonly busy = signal(false);
  readonly error = signal('');

  async ngOnInit() {
    // another post in the same screen (a translation's link): start over with it
    this.route.paramMap.subscribe(pm => {
      const n = Number(pm.get('id')) || 0;
      if (this.id() && n && n !== this.id()) location.reload();
    });
    this.plugins.load();
    void this.postTypes.ensureLoaded();
    void this.loadTaxTerms();
    this.api.settings().then(s => {
      this.templateId.set(Number(s[templateOption(this.type())] || s['post_template_id']) || 0);
      this.siteLang.set(s['language'] || 'th');
      this.langs.set(parseLangs(s['content_languages'] ?? '', s['language'] || 'th'));
    }).catch(() => undefined);
    // categories are optional for editing: a failure here must not stop the post from loading
    this.api.terms().then(t => this.terms.set(t)).catch(e => this.error.set(errorMessage(e)));
    const id = Number(this.route.snapshot.paramMap.get('id')) || 0;
    this.id.set(id);
    if (!id) return;
    try {
      this.fill(await this.api.getPost(id));
      this.revisions.set(await this.api.revisions(id));
      this.api.translations(id).then(t => this.translations.set(t)).catch(() => undefined);
    } catch (e) {
      this.error.set(errorMessage(e));
    }
  }

  langLabel(code: string) { return langLabel(code); }
  editRouteOf(t: { id: number; type: string }) { return editRoute(t.type, t.id); }
  async translate(lang: string) {
    const p = this.post();
    if (!p) return;
    try {
      const t = await this.api.translatePost(p.id, lang);
      await this.router.navigate(editRoute(t.type, t.id));
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  private fill(p: Post) {
    this.post.set(p);
    this.title.set(p.title);
    this.slug.set(p.slug);
    this.content.set(p.content);
    this.excerpt.set(p.excerpt);
    this.cats.set(new Set(p.categories.map(c => c.id)));
    this.featuredId.set(p.featuredMediaId);
    this.featured.set(p.featuredImage);
    this.pinned.set(!!p.pinned);
    const sel: Record<string, Set<number>> = {};
    for (const [tax, list] of Object.entries(p.terms ?? {})) if (tax !== 'category') sel[tax] = new Set(list.map(t => t.id));
    this.txSel.set(sel);
    this.publishAt.set(p.publishedAt ? toLocalInput(p.publishedAt) : '');
    this.publishAt0 = this.publishAt();
    this.meta.set({ ...(p.meta ?? {}) });
    this.fieldErrors.set({});
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  viewUrl() { return `${SITE_ORIGIN}${this.urlPrefix()}${this.post()?.slug}`; }

  setMeta(v: Record<string, unknown>) {
    const before = this.meta();
    this.meta.set(v);
    // a field's message goes away once it is edited
    const changed = Object.keys(v).filter(k => v[k] !== before[k]);
    if (changed.some(k => this.fieldErrors()[k])) this.fieldErrors.update(e => { const o = { ...e }; changed.forEach(k => delete o[k]); return o; });
  }

  /** Client-side field check; messages shown under the fields. */
  private checkFields(): boolean {
    const errs = validateMeta(this.fields(), this.meta());
    const msgs: Record<string, string> = {};
    for (const [k, er] of Object.entries(errs)) msgs[k] = this.i18n.t('pt.err.' + er.code, er.params);
    this.fieldErrors.set(msgs);
    return !Object.keys(msgs).length;
  }

  txLabel(tx: Taxonomy) { this.i18n.locale(); return localized(tx.label) || tx.id; }
  txTree(tax: string) { return termTree(this.txTerms()[tax] ?? []); }
  txHas(tax: string, id: number) { return !!this.txSel()[tax]?.has(id); }
  txToggle(tax: string, id: number) {
    this.txSel.update(all => {
      const s = new Set(all[tax] ?? []);
      if (s.has(id)) s.delete(id); else s.add(id);
      return { ...all, [tax]: s };
    });
  }
  private async loadTaxTerms() {
    await this.postTypes.ensureLoaded();
    const out: Record<string, Term[]> = {};
    await Promise.all(this.taxes().map(async tx => { out[tx.id] = await this.api.terms(tx.id).catch(() => []); }));
    this.txTerms.set(out);
  }
  /** Adds terms typed in the box — several at once separated by commas (tags, 2.5.0); an existing name is ticked. */
  async txAdd(tax: string, input: HTMLInputElement) {
    const names = [...new Set(input.value.split(/[,，]/).map(s => s.trim()).filter(Boolean))];
    if (!names.length) return;
    try {
      for (const name of names) {
        const have = (this.txTerms()[tax] ?? []).find(t => t.name.toLowerCase() === name.toLowerCase());
        if (have) { if (!this.txHas(tax, have.id)) this.txToggle(tax, have.id); continue; }
        const t = await this.api.createTerm({ name, taxonomy: tax });
        this.txTerms.update(all => ({ ...all, [tax]: [...(all[tax] ?? []), t] }));
        this.txToggle(tax, t.id);
      }
      input.value = '';
    } catch (e) {
      this.error.set(errorMessage(e));
    }
  }

  toggleCat(id: number) {
    const s = new Set(this.cats());
    if (s.has(id)) s.delete(id); else s.add(id);
    this.cats.set(s);
  }

  async addCategory(input: HTMLInputElement) {
    if (!input.value.trim()) return;
    try {
      const c = await this.api.createTerm({ name: input.value.trim(), taxonomy: 'category' });
      this.terms.update(ts => [...ts, c]);
      this.toggleCat(c.id);
      input.value = '';
    } catch (e) {
      this.error.set(errorMessage(e));
    }
  }

  picked(m: Media) {
    this.featuredId.set(m.id);
    this.featured.set(m.url);
    this.picking.set(false);
  }

  async save(status: PostStatus) {
    const html = this.editor()?.current() ?? this.content();
    if (this.fields().length && !this.checkFields()) {
      this.error.set(this.i18n.t('post.fieldsInvalid'));
      return;
    }
    this.busy.set(true);
    this.error.set('');
    const data: PostInput = {
      type: this.type(), title: this.title(), slug: this.slug() || undefined, content: html,
      excerpt: this.excerpt(), status, categoryIds: [...this.cats()],
      featuredMediaId: this.featuredId() ?? undefined, clearFeatured: this.featuredId() === null,
      ...(this.auth.can('edit_others_posts') ? { pinned: this.pinned() } : {}),
      ...(this.taxes().length ? { terms: Object.fromEntries(this.taxes().map(tx => [tx.id, [...(this.txSel()[tx.id] ?? [])]])) } : {}),
      ...(this.mayPublish() && this.publishAt() !== this.publishAt0 && !isNaN(Date.parse(this.publishAt())) ? { publishedAt: new Date(this.publishAt()).toISOString() } : {}),
      ...(this.fields().length ? { meta: pickMeta(this.fields(), this.meta()) } : {}),
    };
    try {
      const created = !this.id();
      const p = created ? await this.api.createPost(data) : await this.api.updatePost(this.id(), data);
      if (created) this.id.set(p.id); // a second click must update, not create again
      if (created) {
        // /posts/new → /posts/:id is another route, so a fresh editor loads the saved post
        // (with its SEO and revisions); this instance stops here.
        Dialog.toast(this.savedMsg(p));
        await this.router.navigate(editRoute(this.type(), p.id), { replaceUrl: true });
        return;
      }
      this.fill(p);
      this.revisions.set(await this.api.revisions(p.id));
      Dialog.toast(this.savedMsg(p));
    } catch (e) {
      this.error.set(errorMessage(e));
      const fieldMsgs = metaErrorsFromApi(e);
      if (Object.keys(fieldMsgs).length) this.fieldErrors.set(fieldMsgs);
    } finally {
      this.busy.set(false);
    }
  }

  private savedMsg(p: Post) {
    if (p.status === 'scheduled') return this.i18n.t('post.scheduledMsg', { when: this.i18n.date(p.publishedAt!, true) });
    return this.i18n.t(p.status === 'publish' ? 'post.publishedMsg' : p.status === 'pending' ? 'post.pendingMsg' : 'post.draftMsg');
  }

  async restore(r: Revision) {
    if (!(await Dialog.confirm({ title: this.i18n.t('post.restoreConfirm') }))) return;
    try {
      this.fill(await this.api.restoreRevision(this.id(), r.id));
      this.revisions.set(await this.api.revisions(this.id()));
    } catch (e) {
      this.error.set(errorMessage(e));
    }
  }
}

/** An ISO date as the value of a datetime-local input (local time, minutes). */
function toLocalInput(iso: string): string {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '';
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}
