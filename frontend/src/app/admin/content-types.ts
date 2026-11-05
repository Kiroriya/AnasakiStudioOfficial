import { ChangeDetectionStrategy, Component, OnInit, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { Dialog } from '../core/dialog';
import { I18n, TPipe, localized } from '../core/i18n';
import { PostTypes } from '../core/post-types';
import { listRoute } from '../core/post-type-utils';
import {
  ContentTypeRow, FIELD_TYPES, FieldSpecForm, REPEATER_TYPES, SUPPORTS, TakenType, TypeSpecForm, checkType, emptyField, emptyType, idFrom,
  moved, rowToForm, specBody,
} from './content-types-utils';
import './messages-admin';
import './messages-content-types2';

/**
 * Settings → Content types (CMS 2.2.2): content types and their fields made without code. Saved types join the
 * admin menu, the editor and the site (/<slug>, /<slug>/<post>) like the types of Site Code.
 */
@Component({
  selector: 'rea-content-types',
  imports: [FormsModule, TPipe, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">🗂</span>
      <div class="page-head-text"><h1>{{ 'nav.contentTypes' | t }}</h1><p>{{ 'ct.desc' | t }}</p></div>
      @if (!form()) { <button class="btn btn-primary" id="ct-new" (click)="startNew()">+ {{ 'ct.new' | t }}</button> }
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }

    @if (form(); as f) {
      <section class="card ct-editor" id="ct-editor">
        <div class="card-head"><h3>{{ (isNew() ? 'ct.newTitle' : 'ct.editTitle') | t }}</h3></div>
        <div class="ct-grid">
          <label>{{ 'ct.labelTh' | t }}<input id="ct-label-th" [ngModel]="f.label.th" (ngModelChange)="setLabel('th', $event)" /></label>
          <label>{{ 'ct.labelEn' | t }}<input id="ct-label-en" [ngModel]="f.label.en" (ngModelChange)="setLabel('en', $event)" /></label>
          <label>{{ 'ct.singularTh' | t }} <span class="muted small">{{ 'common.optional' | t }}</span><input [ngModel]="f.singular.th" (ngModelChange)="patch(x => x.singular.th = $event)" /></label>
          <label>{{ 'ct.icon' | t }}<input class="ct-icon" maxlength="8" [ngModel]="f.icon" (ngModelChange)="patch(x => x.icon = $event)" /></label>
          <label>{{ 'ct.type' | t }} <span class="muted small">{{ 'ct.typeHint' | t }}</span>
            <input id="ct-type" class="mono" [disabled]="!isNew()" [ngModel]="f.type" (ngModelChange)="setType($event)" /></label>
          <label>{{ 'ct.slug' | t }} <span class="muted small">/{{ f.slug || '…' }}/…</span>
            <input id="ct-slug" class="mono" [ngModel]="f.slug" (ngModelChange)="setSlug($event)" /></label>
        </div>
        <div class="ct-row">
          <label class="check"><input type="checkbox" id="ct-public" [ngModel]="f.public" (ngModelChange)="patch(x => x.public = $event)" /> {{ 'ct.public' | t }}</label>
          <span class="muted small">{{ 'ct.supports' | t }}:</span>
          @for (s of supports; track s) {
            <label class="check"><input type="checkbox" [checked]="f.supports.includes(s)" (change)="toggleSupport(s, $any($event.target).checked)" /> {{ ('ct.sup.' + s) | t }}</label>
          }
        </div>

        <h4>{{ 'ct.fields' | t }}</h4>
        <p class="hint">{{ 'ct.fieldsHint' | t }}</p>
        <ul class="ct-fields" id="ct-fields">
          @for (fd of f.fields; track $index; let i = $index) {
            <li class="ct-field">
              <div class="ct-field-main">
                <input [placeholder]="'ct.fieldLabel' | t" [ngModel]="fd.label.th" (ngModelChange)="setFieldLabel(i, $event)" [attr.aria-label]="'ct.fieldLabel' | t" />
                <input class="mono" [placeholder]="'ct.fieldKey' | t" [ngModel]="fd.key" (ngModelChange)="patchField(i, x => x.key = $event)" [attr.aria-label]="'ct.fieldKey' | t" />
                <select [ngModel]="fd.type" (ngModelChange)="patchField(i, x => setFieldType(x, $event))" [attr.aria-label]="'ct.fieldType' | t">
                  @for (ty of fieldTypes; track ty) { <option [value]="ty">{{ ('ct.ft.' + ty) | t }}</option> }
                </select>
                <label class="check"><input type="checkbox" [ngModel]="fd.required" (ngModelChange)="patchField(i, x => x.required = $event)" /> {{ 'ct.required' | t }}</label>
                <button class="icon-btn" [disabled]="i === 0" (click)="moveField(i, -1)" [attr.aria-label]="'menus.up' | t">↑</button>
                <button class="icon-btn" [disabled]="i === f.fields.length - 1" (click)="moveField(i, 1)" [attr.aria-label]="'menus.down' | t">↓</button>
                <button class="icon-btn" (click)="removeField(i)" [attr.aria-label]="'common.delete' | t">✕</button>
              </div>
              @switch (fd.type) {
                @case ('select') {
                  <label class="ct-sub">{{ 'ct.options' | t }}<textarea rows="3" [placeholder]="'ct.optionsPh' | t" [ngModel]="fd.options" (ngModelChange)="patchField(i, x => x.options = $event)"></textarea></label>
                }
                @case ('number') {
                  <div class="ct-sub ct-minmax">
                    <label>{{ 'ct.min' | t }}<input type="number" [ngModel]="fd.min" (ngModelChange)="patchField(i, x => x.min = $event)" /></label>
                    <label>{{ 'ct.max' | t }}<input type="number" [ngModel]="fd.max" (ngModelChange)="patchField(i, x => x.max = $event)" /></label>
                  </div>
                }
                @case ('relation') {
                  <div class="ct-sub ct-row">
                    <span class="muted small">{{ 'ct.linksTo' | t }}:</span>
                    @for (o of relationTargets(); track o.type) {
                      <label class="check"><input type="checkbox" [checked]="(fd.postTypes ?? []).includes(o.type)" (change)="toggleTarget(i, o.type, $any($event.target).checked)" /> {{ o.label }}</label>
                    }
                    <label class="check"><input type="checkbox" [ngModel]="fd.multiple" (ngModelChange)="patchField(i, x => x.multiple = $event)" /> {{ 'ct.multiple' | t }}</label>
                  </div>
                }
                @case ('repeater') {
                  <div class="ct-sub">
                    <span class="muted small">{{ 'ct.rowFields' | t }}</span>
                    @for (sf of fd.fields ?? []; track $index; let j = $index) {
                      <div class="ct-field-main">
                        <input [placeholder]="'ct.fieldLabel' | t" [ngModel]="sf.label.th" (ngModelChange)="setSubLabel(i, j, $event)" />
                        <input class="mono" [placeholder]="'ct.fieldKey' | t" [ngModel]="sf.key" (ngModelChange)="patchSub(i, j, x => x.key = $event)" />
                        <select [ngModel]="sf.type" (ngModelChange)="patchSub(i, j, x => x.type = $event)">
                          @for (ty of repeaterTypes; track ty) { <option [value]="ty">{{ ('ct.ft.' + ty) | t }}</option> }
                        </select>
                        <button class="icon-btn" (click)="removeSub(i, j)" [attr.aria-label]="'common.delete' | t">✕</button>
                      </div>
                    }
                    <button class="btn btn-sm" (click)="addSub(i)">+ {{ 'ct.addRowField' | t }}</button>
                  </div>
                }
              }
            </li>
          } @empty { <li class="muted">{{ 'ct.noFields' | t }}</li> }
        </ul>
        <button class="btn btn-sm" id="ct-add-field" (click)="addField()">+ {{ 'ct.addField' | t }}</button>

        @if (problem(); as pr) { <p class="alert" role="alert">⚠ {{ pr }}</p> }
        <div class="row-actions">
          <button class="btn" (click)="cancel()">{{ 'common.cancel' | t }}</button>
          <button class="btn btn-primary" id="ct-save" [disabled]="busy() || !!problem()" (click)="save()">💾 {{ 'common.save' | t }}</button>
        </div>
      </section>
    }

    <section class="card">
      <h3>{{ 'ct.yours' | t }}</h3>
      @for (r of rows(); track r.type) {
        <div class="ct-item" [attr.data-type]="r.type">
          <span class="ct-item-icon">{{ r.icon || '📦' }}</span>
          <div class="ct-item-text">
            <b>{{ label(r) }}</b>
            <span class="muted small mono">/{{ r.slug }} · {{ r.fields.length }} {{ 'ct.fieldsCount' | t }} · {{ r.postCount }} {{ 'ct.postsCount' | t }}</span>
            @if (!r.active) { <span class="badge badge-warn">{{ 'ct.inactive' | t }}</span> }
          </div>
          <a class="btn btn-sm" [routerLink]="listRoute(r.type)">{{ 'ct.openList' | t }}</a>
          <button class="btn btn-sm" (click)="edit(r)">✎ {{ 'common.edit' | t }}</button>
          <button class="btn btn-sm btn-danger" [disabled]="r.postCount > 0" [title]="r.postCount > 0 ? ('ct.deleteBlocked' | t) : ''" (click)="remove(r)">🗑</button>
        </div>
      } @empty { <p class="muted">{{ 'ct.none' | t }}</p> }
      @if (taken().length) {
        <h4>{{ 'ct.fromCode' | t }}</h4>
        <p class="hint">{{ 'ct.fromCodeHint' | t }}</p>
        <ul class="ct-taken">
          @for (t of taken(); track t.type) { <li><span class="mono">{{ t.type }}</span> — {{ label(t) }} <span class="muted small">/{{ t.slug }} · {{ ('ct.owner.' + t.owner) | t }}</span></li> }
        </ul>
      }
    </section>
  `,
  styles: [`
    .ct-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 16px; }
    .ct-grid > label { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
    @media (max-width: 720px) { .ct-grid { grid-template-columns: minmax(0, 1fr); } }
    .ct-row { display: flex; flex-wrap: wrap; gap: 8px 16px; align-items: center; margin: 12px 0; }
    .ct-icon { width: 6em; }
    .ct-fields { list-style: none; padding: 0; margin: 0 0 8px; display: flex; flex-direction: column; gap: 8px; }
    .ct-field { border: 1px solid var(--border, #e5e7eb); border-radius: 10px; padding: 10px 12px; display: flex; flex-direction: column; gap: 10px; }
    /* name · key · type · required · ↑ ↓ ✕ on one line; stacked on phones */
    .ct-field-main { display: grid; grid-template-columns: minmax(0, 2fr) minmax(0, 1.4fr) minmax(0, 1.4fr) auto auto auto auto; gap: 8px; align-items: center; }
    .ct-field-main > input, .ct-field-main > select { width: 100%; min-width: 0; margin: 0; }
    .ct-field-main > .check { white-space: nowrap; margin: 0; }
    @media (max-width: 760px) { .ct-field-main { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); } }
    .ct-sub { padding-left: 12px; border-left: 2px solid var(--border, #e5e7eb); display: flex; flex-direction: column; gap: 8px; }
    .ct-sub > label { display: flex; flex-direction: column; gap: 6px; }
    .ct-minmax { display: grid; grid-template-columns: repeat(2, minmax(0, 180px)); gap: 12px; }
    .ct-minmax > label { display: flex; flex-direction: column; gap: 6px; }
    .ct-sub .ct-field-main { grid-template-columns: minmax(0, 2fr) minmax(0, 1.4fr) minmax(0, 1.4fr) auto; }
    .ct-item { display: flex; flex-wrap: wrap; gap: 10px; align-items: center; padding: 10px 0; border-bottom: 1px solid var(--border, #e5e7eb); }
    .ct-item-icon { font-size: 22px; }
    .ct-item-text { flex: 1 1 200px; display: flex; flex-direction: column; min-width: 0; }
    .ct-taken { margin: 0; padding-left: 1.2em; }
    .ct-editor .row-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
  `],
})
export class ContentTypes implements OnInit {
  readonly i18n = inject(I18n);
  private api = inject(Api);
  private types = inject(PostTypes);
  readonly rows = signal<ContentTypeRow[]>([]);
  readonly taken = signal<TakenType[]>([]);
  readonly form = signal<TypeSpecForm | null>(null);
  readonly isNew = signal(false);
  readonly busy = signal(false);
  readonly error = signal('');
  readonly fieldTypes = FIELD_TYPES;
  readonly repeaterTypes = REPEATER_TYPES;
  readonly supports = SUPPORTS;
  readonly listRoute = listRoute;
  /** The slug follows the label/type until the user types one. */
  private slugTouched = false;
  private typeTouched = false;

  readonly problem = computed(() => {
    const f = this.form();
    if (!f) return '';
    const others = [...this.taken(), ...this.rows().filter(r => r.type !== f.type)];
    const p = checkType(f, others, this.isNew());
    return p ? this.i18n.t(p.key, { x: p.x ?? '' }) : '';
  });

  /** What a relation field may point at: posts, pages and every content type. */
  readonly relationTargets = computed(() => [
    { type: 'post', label: this.i18n.t('nav.posts') }, { type: 'page', label: this.i18n.t('nav.pages') },
    ...this.rows().map(r => ({ type: r.type, label: this.label(r) })), ...this.taken().map(t => ({ type: t.type, label: this.label(t) })),
  ]);

  async ngOnInit() { await this.load(); }

  private async load() {
    try {
      const r = await this.api.contentTypes();
      this.rows.set(r.types ?? []);
      this.taken.set(r.taken ?? []);
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  label(r: { label: { th?: string; en?: string } }) { return localized(r.label as Record<string, string>) || ''; }

  startNew() { this.slugTouched = false; this.typeTouched = false; this.isNew.set(true); this.form.set(emptyType()); }
  edit(r: ContentTypeRow) { this.slugTouched = true; this.typeTouched = true; this.isNew.set(false); this.form.set(rowToForm(r)); }
  cancel() { this.form.set(null); }

  patch(fn: (f: TypeSpecForm) => void) {
    const f = structuredClone(this.form()!);
    fn(f);
    this.form.set(f);
  }

  setLabel(lang: 'th' | 'en', v: string) {
    this.patch(f => {
      f.label[lang] = v;
      if (this.isNew() && !this.typeTouched) f.type = idFrom(f.label.en || f.label.th, '_');
      if (this.isNew() && !this.slugTouched) f.slug = idFrom(f.label.en || f.label.th, '-') || f.type.replace(/_/g, '-');
    });
  }
  setType(v: string) {
    this.typeTouched = true;
    this.patch(f => { f.type = v; if (!this.slugTouched) f.slug = v.replace(/_/g, '-'); });
  }
  setSlug(v: string) { this.slugTouched = true; this.patch(f => (f.slug = v)); }
  toggleSupport(s: string, on: boolean) { this.patch(f => (f.supports = on ? [...new Set([...f.supports, s])] : f.supports.filter(x => x !== s))); }

  addField() { this.patch(f => f.fields.push(emptyField())); }
  removeField(i: number) { this.patch(f => f.fields.splice(i, 1)); }
  moveField(i: number, dir: number) { this.patch(f => (f.fields = moved(f.fields, i, dir))); }
  patchField(i: number, fn: (x: FieldSpecForm) => void) { this.patch(f => fn(f.fields[i])); }
  setFieldLabel(i: number, v: string) {
    this.patchField(i, x => {
      const auto = !x.key || x.key === idFrom(x.label.th || x.label.en, '_').replace(/^_+/, '');
      x.label = { th: v, en: x.label.en || '' };
      if (auto) x.key = idFrom(v, '_');
    });
  }
  setFieldType(x: FieldSpecForm, ty: string) {
    x.type = ty;
    if (ty === 'repeater' && !x.fields?.length) x.fields = [emptyField()];
    if (ty === 'relation' && !x.postTypes?.length) x.postTypes = ['post'];
  }
  toggleTarget(i: number, type: string, on: boolean) {
    this.patchField(i, x => (x.postTypes = on ? [...new Set([...(x.postTypes ?? []), type])] : (x.postTypes ?? []).filter(t => t !== type)));
  }
  addSub(i: number) { this.patchField(i, x => (x.fields = [...(x.fields ?? []), emptyField()])); }
  removeSub(i: number, j: number) { this.patchField(i, x => x.fields?.splice(j, 1)); }
  patchSub(i: number, j: number, fn: (x: FieldSpecForm) => void) { this.patchField(i, x => fn(x.fields![j])); }
  setSubLabel(i: number, j: number, v: string) {
    this.patchSub(i, j, x => { const auto = !x.key; x.label = { th: v, en: x.label.en || '' }; if (auto) x.key = idFrom(v, '_'); });
  }

  async save() {
    const f = this.form();
    if (!f || this.problem()) return;
    this.busy.set(true);
    try {
      await this.api.saveContentType(f.type.trim(), specBody(f));
      Dialog.toast(this.i18n.t('ct.saved'));
      this.form.set(null);
      await Promise.all([this.load(), this.types.reload()]);
    } catch (e) { Dialog.error(errorMessage(e)); }
    finally { this.busy.set(false); }
  }

  async remove(r: ContentTypeRow) {
    if (!(await Dialog.confirmDelete(this.label(r), this.i18n.t('ct.deleteText')))) return;
    try {
      await this.api.deleteContentType(r.type);
      await Promise.all([this.load(), this.types.reload()]);
    } catch (e) { Dialog.error(errorMessage(e)); }
  }
}
