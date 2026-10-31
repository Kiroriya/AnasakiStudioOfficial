import { ChangeDetectionStrategy, Component, input, output, signal } from '@angular/core';
import { TPipe, localized } from '../core/i18n';
import { Media, SchemaField } from '../core/models';
import { InvalidJson, fromDatetimeLocal, toDatetimeLocal } from '../core/post-type-utils';
import { MediaPicker } from './media-picker';
import { SelectValue } from './select-value';

/**
 * Form generated from a schema: plugin settings, designer block props and
 * (phase 4a) custom fields of post types — with required marks, min/max and
 * a message under each field (`errors`, e.g. the server's validation).
 */
@Component({
  selector: 'rea-schema-form',
  imports: [MediaPicker, TPipe, SelectValue],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @for (f of fields(); track f.key) {
      <div class="schema-field" [class.compact]="compact()" [class.has-error]="!!errors()[f.key]" [attr.data-field]="f.key" [attr.data-type]="f.type">
        @switch (f.type) {
          @case ('boolean') {
            <label class="check"><input type="checkbox" [id]="fid(f)" [checked]="!!val(f)" (change)="set(f, $any($event.target).checked)" /> {{ label(f.label) }}</label>
          }
          @default {
            <label [attr.for]="fid(f)">{{ label(f.label) }}@if (f.required) { <span class="sf-req" aria-hidden="true"> *</span> }
              @switch (f.type) {
                @case ('textarea') { <textarea rows="4" [id]="fid(f)" [required]="!!f.required" [attr.placeholder]="f.placeholder || null" [value]="str(f)" (input)="set(f, $any($event.target).value)"></textarea> }
                @case ('json') {
                  <textarea rows="5" class="mono" [id]="fid(f)" spellcheck="false" [value]="jsonText(f)" (input)="setJson(f, $any($event.target).value)"></textarea>
                }
                @case ('number') {
                  <input type="number" [id]="fid(f)" [required]="!!f.required" [attr.min]="f.min ?? null" [attr.max]="f.max ?? null" [attr.placeholder]="f.placeholder || null"
                         [value]="str(f)" (input)="set(f, num($any($event.target).value))" />
                }
                @case ('date') { <input type="date" [id]="fid(f)" [required]="!!f.required" [value]="str(f).slice(0, 10)" (input)="set(f, $any($event.target).value)" /> }
                @case ('datetime') { <input type="datetime-local" [id]="fid(f)" [required]="!!f.required" [value]="local(f)" (input)="set(f, fromLocal($any($event.target).value))" /> }
                @case ('email') { <input type="email" [id]="fid(f)" [required]="!!f.required" [attr.placeholder]="f.placeholder || 'name@example.com'" [value]="str(f)" (input)="set(f, $any($event.target).value)" /> }
                @case ('color') {
                  <span class="color-row">
                    <input type="color" [value]="str(f) || '#000000'" (input)="set(f, $any($event.target).value)" />
                    <input [id]="fid(f)" [value]="str(f)" (input)="set(f, $any($event.target).value)" />
                  </span>
                }
                @case ('select') {
                  <select [id]="fid(f)" [required]="!!f.required" [value]="str(f)" (change)="set(f, $any($event.target).value)">
                    @if (blankOption() && (!f.required || !str(f))) { <option value="" [selected]="!str(f)">—</option> }
                    @for (o of f.options ?? []; track o.value) { <option [value]="o.value" [selected]="o.value === str(f)">{{ label(o.label) }}</option> }
                  </select>
                }
                @case ('image') {
                  <span class="image-field">
                    <span class="image-preview icon-preview">@if (str(f)) { <img [src]="str(f)" alt="" /> } @else { 🖼 }</span>
                    <button type="button" class="btn btn-sm" [id]="fid(f)" (click)="picking.set(f.key)">{{ 'post.chooseImage' | t }}</button>
                    @if (str(f)) { <button type="button" class="btn btn-sm" (click)="set(f, '')">{{ 'post.removeImage' | t }}</button> }
                  </span>
                }
                @case ('url') { <input type="url" [id]="fid(f)" [required]="!!f.required" [attr.placeholder]="f.placeholder || 'https://'" [value]="str(f)" (input)="set(f, $any($event.target).value)" /> }
                @default {
                  <input [id]="fid(f)" [required]="!!f.required" [attr.minlength]="f.min ?? null" [attr.maxlength]="f.max ?? null" [attr.placeholder]="f.placeholder || null"
                         [value]="str(f)" (input)="set(f, $any($event.target).value)" />
                }
              }
            </label>
          }
        }
        @if (errors()[f.key]; as msg) { <p class="sf-error" role="alert" [attr.data-error-for]="f.key">{{ msg }}</p> }
        @if (f.help) { <p class="hint">{{ label(f.help) }}</p> }
      </div>
    } @empty {
      <p class="muted small">{{ emptyText() || ('plugins.noSettings' | t) }}</p>
    }
    <rea-media-picker [open]="picking() !== null" (openChange)="!$event && picking.set(null)" (picked)="picked($event)" />
  `,
  styles: `
    .schema-field { margin-bottom: 4px; }
    .schema-field.compact label { font-size: 12px; margin-bottom: 10px; }
    .color-row { display: flex; gap: 8px; margin-top: 6px; input[type='color'] { width: 44px; padding: 2px; } }
    .image-field { margin-top: 6px; }
    .sf-req { color: #dc2626; }
    .sf-error { margin: -4px 0 8px; color: #b91c1c; font-size: 12.5px; }
    .has-error input, .has-error select, .has-error textarea { border-color: #f87171; }
  `,
})
export class SchemaForm {
  readonly fields = input<SchemaField[]>([]);
  readonly values = input<Record<string, unknown>>({});
  readonly compact = input(false);
  /** Message per field key (client checks or the server's validation). */
  readonly errors = input<Record<string, string>>({});
  /** Element ids are `<idPrefix>-<key>` (several forms on one screen). */
  readonly idPrefix = input('sf');
  readonly emptyText = input('');
  /** Selects offer an empty choice (custom fields: not chosen yet). */
  readonly blankOption = input(false);
  readonly changed = output<Record<string, unknown>>();
  readonly picking = signal<string | null>(null);
  /** Text of json fields while typed (kept even when it does not parse yet). */
  private readonly jsonDraft = signal<Record<string, string>>({});

  label(v: SchemaField['label'] | undefined) { return localized(v as never); }
  fid(f: SchemaField) { return `${this.idPrefix()}-${f.key}`; }
  val(f: SchemaField) { const v = this.values()[f.key]; return v === undefined ? f.default : v; }
  str(f: SchemaField) {
    const v = this.val(f);
    if (v === undefined || v === null) return '';
    if (v instanceof InvalidJson) return v.text;
    return typeof v === 'object' ? JSON.stringify(v) : String(v);
  }
  num(s: string) { return s === '' ? '' : Number(s); }
  local(f: SchemaField) { return toDatetimeLocal(this.val(f)); }
  fromLocal(s: string) { return fromDatetimeLocal(s); }

  jsonText(f: SchemaField) {
    const d = this.jsonDraft()[f.key];
    if (d !== undefined) return d;
    const v = this.val(f);
    if (v instanceof InvalidJson) return v.text;
    return v === undefined || v === null || v === '' ? '' : JSON.stringify(v, null, 2);
  }

  setJson(f: SchemaField, text: string) {
    this.jsonDraft.update(d => ({ ...d, [f.key]: text }));
    if (!text.trim()) { this.set(f, null); return; }
    try { this.set(f, JSON.parse(text)); } catch { this.set(f, new InvalidJson(text)); }
  }

  set(f: SchemaField, v: unknown) { this.changed.emit({ ...this.values(), [f.key]: v }); }

  picked(m: Media) {
    const key = this.picking();
    const f = this.fields().find(x => x.key === key);
    if (f) this.set(f, m.url);
    this.picking.set(null);
  }
}
