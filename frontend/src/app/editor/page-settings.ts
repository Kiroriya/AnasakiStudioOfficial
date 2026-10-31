import { ChangeDetectionStrategy, Component, computed, effect, inject, signal, untracked } from '@angular/core';
import { Api, SITE_ORIGIN, errorMessage } from '../core/api';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { PostTypes } from '../core/post-types';
import { metaErrorsFromApi, pickMeta, validateMeta } from '../core/post-type-utils';
import { SchemaForm } from '../shared/schema-form';
import '../admin/messages-content-types';
import { EditorStore, slugify } from './editor-store';
import './messages-page';

/** Nothing selected on a page: its name and web address. */
@Component({
  selector: 'rea-page-settings',
  imports: [TPipe, SchemaForm],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (store.floating().length; as n) {
      <section class="fg-sec ps-float">
        <b>⚠ {{ 'fl.title' | t: { n } }}</b>
        <p>{{ 'fl.hint' | t }}</p>
        <div class="ps-fl-act">
          <button class="btn btn-sm btn-primary" (click)="store.unfloat(ids())">{{ 'fl.fixAll' | t }}</button>
          <button class="btn btn-sm" (click)="store.selectMany(ids())">{{ 'fl.show' | t }}</button>
        </div>
      </section>
    }
    <section class="fg-sec ps">
      <h5>📄 {{ 'ps.title' | t }}</h5>
      <label>{{ 'ps.name' | t }}<input [value]="store.title()" (change)="store.renamePage($any($event.target).value)" /></label>
      @if (isHome()) {
        <p class="hint">🏠 {{ 'ps.home' | t }}</p>
      } @else {
        <label>{{ 'ps.address' | t }}
          <span class="ps-url"><i>{{ prefix() }}</i><input [value]="decoded()" (change)="setSlug($any($event.target).value)" [placeholder]="'ps.addressPh' | t" /></span>
        </label>
        <p class="hint">{{ 'ps.addressHint' | t }}</p>
        <div class="ps-sug">
          @for (s of suggestions; track s) { <button class="fg-link" (click)="setSlug(s)">/{{ s }}</button> }
        </div>
      }
    </section>
    @if (fields().length) {
      <section class="fg-sec ps ps-fields" id="page-fields">
        <h5>🧾 {{ 'ps.fields' | t }}</h5>
        <rea-schema-form idPrefix="page-field" [compact]="true" [blankOption]="true" [fields]="fields()" [values]="meta()" [errors]="fieldErrors()" (changed)="meta.set($event); metaDirty.set(true)" />
        <button class="btn btn-sm btn-primary" id="page-fields-save" [disabled]="!metaDirty() || savingMeta()" (click)="saveMeta()">💾 {{ 'ps.fieldsSave' | t }}</button>
      </section>
    }
  `,
  styles: [`
    .ps label { display: flex; flex-direction: column; gap: 3px; font-size: 11px; color: var(--fg-muted, #777); margin-bottom: 8px; }
    .ps-url { display: flex; align-items: center; border: 1px solid var(--fg-border, #e5e5e5); border-radius: 6px; overflow: hidden; background: #fff; }
    .ps-url i { font-style: normal; padding: 0 2px 0 8px; color: #9ca3af; font-size: 12px; white-space: nowrap; }
    .ps-url input { border: 0 !important; flex: 1; min-width: 0; }
    .ps-float { background: #fffbeb; border: 1px solid #fde68a; border-radius: 8px; margin: 8px; padding: 10px; font-size: 12px; color: #92400e; }
    .ps-float p { margin: 4px 0 8px; } .ps-fl-act { display: flex; gap: 6px; }
    .ps-sug { display: flex; flex-wrap: wrap; gap: 4px 10px; font-size: 11px; }
  `],
})
export class PageSettings {
  readonly store = inject(EditorStore);
  private api = inject(Api);
  private i18n = inject(I18n);
  private postTypes = inject(PostTypes);
  /** Custom fields of pages (site.fields('page', …)), saved on their own. */
  readonly fields = computed(() => (this.store.post()?.type === 'page' ? this.postTypes.schema('page') : []));
  readonly meta = signal<Record<string, unknown>>({});
  readonly metaDirty = signal(false);
  readonly savingMeta = signal(false);
  readonly fieldErrors = signal<Record<string, string>>({});
  private loadedId = 0;

  constructor() {
    void this.postTypes.ensureLoaded();
    effect(() => {
      const p = this.store.post();
      untracked(() => {
        // a save of the design hands back the same post: keep fields being edited
        if (p && p.id === this.loadedId && this.metaDirty()) return;
        this.loadedId = p?.id ?? 0;
        this.meta.set({ ...(p?.meta ?? {}) });
        this.metaDirty.set(false);
        this.fieldErrors.set({});
      });
    });
  }

  async saveMeta() {
    const p = this.store.post();
    if (!p) return;
    const errs = validateMeta(this.fields(), this.meta());
    const msgs: Record<string, string> = {};
    for (const [k, er] of Object.entries(errs)) msgs[k] = this.i18n.t('pt.err.' + er.code, er.params);
    this.fieldErrors.set(msgs);
    if (Object.keys(msgs).length) return;
    this.savingMeta.set(true);
    try {
      await this.api.updatePost(p.id, { meta: pickMeta(this.fields(), this.meta()) });
      this.metaDirty.set(false);
      Dialog.toast(this.i18n.t('ps.fieldsSaved'));
    } catch (e) {
      const f = metaErrorsFromApi(e);
      if (Object.keys(f).length) this.fieldErrors.set(f);
      Dialog.error(errorMessage(e));
    } finally {
      this.savingMeta.set(false);
    }
  }
  readonly suggestions = ['about', 'contact', 'menu', 'services', 'shop'];
  readonly ids = computed(() => this.store.floating().map(n => n.id));
  readonly isHome = computed(() => this.store.settings()['home_page_id'] === String(this.store.post()?.id));
  readonly decoded = computed(() => { try { return decodeURIComponent(this.store.slug()); } catch { return this.store.slug(); } });
  readonly prefix = computed(() => `${(SITE_ORIGIN || location.origin).replace(/^https?:\/\//, '')}/${this.store.post()?.type === 'post' ? (this.store.settings()['blog_base'] || 'blog') + '/' : ''}`);

  setSlug(v: string) {
    const s = slugify(v);
    if (!s || s === this.store.slug()) return;
    this.store.slug.set(s);
    this.store.dirty.set(true);
  }
}
