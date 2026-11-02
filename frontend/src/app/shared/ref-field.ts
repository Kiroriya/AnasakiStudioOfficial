import { ChangeDetectionStrategy, Component, computed, effect, inject, input, output, signal, untracked } from '@angular/core';
import { Api } from '../core/api';
import { TPipe } from '../core/i18n';
import { Media, SchemaField } from '../core/models';
import { termTree } from '../core/terms';
import { MediaPicker } from './media-picker';

/** One choice of a reference field: a post, user, term or media file. */
export interface RefItem { id: number; label: string; sub?: string; thumb?: string }

/** The ids of a reference value: one id, a list, or nothing. */
export function refIds(v: unknown): number[] {
  const list = Array.isArray(v) ? v : v === undefined || v === null || v === '' ? [] : [v];
  return list.map(Number).filter(n => Number.isInteger(n) && n > 0);
}

/**
 * Custom fields that hold ids (CMS 2.1.0): relation (posts of `postTypes`),
 * user, term (of `taxonomy`), file and gallery (media). Shows the chosen
 * items with remove / reorder, and a search box or the media picker to add.
 * Emits a number, or a list when `multiple` (gallery always).
 */
@Component({
  selector: 'rea-ref-field',
  imports: [TPipe, MediaPicker],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="ref-field" [attr.data-ref-type]="field().type">
      @if (chosen().length) {
        <ul class="ref-chosen" [class.ref-gallery]="field().type === 'gallery'">
          @for (it of chosen(); track it.id; let i = $index) {
            <li [attr.data-ref-id]="it.id">
              @if (it.thumb) { <img [src]="it.thumb" alt="" /> }
              <span class="ref-label">{{ it.label }} @if (it.sub) { <small class="muted">{{ it.sub }}</small> }</span>
              @if (multiple() && i > 0) { <button type="button" class="btn btn-sm" (click)="move(i, -1)" [title]="'sf.up' | t">↑</button> }
              <button type="button" class="btn btn-sm" (click)="remove(it.id)" [title]="'common.delete' | t">✕</button>
            </li>
          }
        </ul>
      }
      @if (multiple() || !chosen().length) {
        @if (isMedia()) {
          <button type="button" class="btn btn-sm" [id]="inputId()" (click)="picking.set(true)">{{ 'sf.chooseFile' | t }}</button>
        } @else if (field().type === 'term') {
          <select [id]="inputId()" (change)="pickOption($any($event.target))">
            <option value="">{{ 'sf.choose' | t }}</option>
            @for (o of termOptions(); track o.id) { <option [value]="o.id">{{ o.label }}</option> }
          </select>
        } @else {
          <input [id]="inputId()" class="ref-search" [placeholder]="'sf.search' | t" [value]="query()" (input)="search($any($event.target).value)" />
          @if (results().length) {
            <ul class="ref-results">
              @for (r of results(); track r.id) {
                <li><button type="button" class="link-btn" [attr.data-pick]="r.id" (click)="add(r)">{{ r.label }} @if (r.sub) { <small class="muted">{{ r.sub }}</small> }</button></li>
              }
            </ul>
          }
        }
      }
      @if (error()) { <p class="sf-error">{{ error() }}</p> }
    </div>
    @if (isMedia()) {
      <rea-media-picker [open]="picking()" (openChange)="picking.set($event)" [kind]="field().type === 'gallery' ? 'image' : ''" (picked)="addMedia($event)" />
    }
  `,
  styles: `
    .ref-chosen { list-style: none; margin: 6px 0; padding: 0; display: grid; gap: 6px; }
    .ref-chosen li { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border: 1px solid var(--border); border-radius: 8px; }
    .ref-chosen img { width: 40px; height: 40px; object-fit: cover; border-radius: 6px; }
    .ref-gallery { grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); }
    .ref-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
    .ref-results { list-style: none; margin: 4px 0 0; padding: 4px; border: 1px solid var(--border); border-radius: 8px; max-height: 220px; overflow: auto; }
    .ref-results .link-btn { background: none; border: 0; padding: 4px 6px; cursor: pointer; text-align: left; width: 100%; color: inherit; }
    .ref-search, select { margin-top: 6px; width: 100%; }
  `,
})
export class RefField {
  private api = inject(Api);
  readonly field = input.required<SchemaField>();
  readonly value = input<unknown>(null);
  readonly inputId = input('');
  readonly changed = output<unknown>();

  readonly multiple = computed(() => !!this.field().multiple || this.field().type === 'gallery');
  readonly isMedia = computed(() => this.field().type === 'file' || this.field().type === 'gallery');
  /** Labels of the ids we know (loaded, searched or picked). */
  private readonly known = signal<Record<number, RefItem>>({});
  readonly chosen = computed(() => refIds(this.value()).map(id => this.known()[id] ?? { id, label: '#' + id }));
  readonly query = signal('');
  readonly results = signal<RefItem[]>([]);
  readonly termOptions = signal<RefItem[]>([]);
  readonly picking = signal(false);
  readonly error = signal('');
  private timer: ReturnType<typeof setTimeout> | undefined;

  constructor() {
    // labels of the ids already stored (once per field and value change)
    effect(() => {
      const f = this.field();
      const ids = refIds(this.value()).filter(id => !untracked(this.known)[id]);
      if (f.type === 'term') untracked(() => this.loadTerms(f));
      if (ids.length) untracked(() => this.resolve(f, ids));
    });
  }

  private remember(items: RefItem[]) { this.known.update(k => ({ ...k, ...Object.fromEntries(items.map(i => [i.id, i])) })); }

  private async resolve(f: SchemaField, ids: number[]) {
    try {
      if (f.type === 'user') this.remember((await this.api.userOptions({ ids: ids.join(',') })).map(u => ({ id: u.id, label: u.name })));
      else if (f.type === 'file' || f.type === 'gallery') this.remember((await this.api.media()).filter(m => ids.includes(m.id)).map(mediaItem));
      else if (f.type === 'relation') {
        for (const type of f.postTypes ?? ['post']) {
          const res = await this.api.posts({ type, perPage: 100 });
          this.remember(res.items.filter(p => ids.includes(p.id)).map(p => ({ id: p.id, label: p.title, sub: type })));
        }
      }
    } catch { /* labels are a nicety; the ids stay */ }
  }

  private async loadTerms(f: SchemaField) {
    if (this.termOptions().length) return;
    try {
      const rows = termTree(await this.api.terms(f.taxonomy || 'category'));
      const items = rows.map(r => ({ id: r.term.id, label: r.indent + r.term.name }));
      this.termOptions.set(items);
      this.remember(rows.map(r => ({ id: r.term.id, label: r.term.name })));
    } catch (e) { this.error.set(String((e as Error)?.message ?? e)); }
  }

  search(q: string) {
    this.query.set(q);
    clearTimeout(this.timer);
    this.timer = setTimeout(() => void this.runSearch(q), 250);
  }

  private async runSearch(q: string) {
    const f = this.field();
    try {
      let items: RefItem[] = [];
      if (f.type === 'user') items = (await this.api.userOptions({ search: q })).map(u => ({ id: u.id, label: u.name }));
      else for (const type of f.postTypes ?? ['post']) {
        const res = await this.api.posts({ type, search: q, perPage: 20 });
        items.push(...res.items.map(p => ({ id: p.id, label: p.title, sub: p.status === 'publish' ? type : `${type} · ${p.status}` })));
      }
      this.remember(items);
      this.results.set(items.filter(i => !refIds(this.value()).includes(i.id)));
      this.error.set('');
    } catch (e) { this.error.set(String((e as Error)?.message ?? e)); }
  }

  add(it: RefItem) {
    this.remember([it]);
    this.results.set([]);
    this.query.set('');
    const ids = refIds(this.value());
    this.changed.emit(this.multiple() ? [...ids.filter(x => x !== it.id), it.id] : it.id);
  }

  pickOption(sel: HTMLSelectElement) {
    const id = Number(sel.value);
    sel.value = '';
    const it = this.termOptions().find(o => o.id === id);
    if (it) this.add({ ...it, label: it.label.replace(/^(— )+/, '') });
  }

  addMedia(m: Media) {
    this.picking.set(false);
    this.add(mediaItem(m));
  }

  remove(id: number) {
    const rest = refIds(this.value()).filter(x => x !== id);
    this.changed.emit(this.multiple() ? rest : null);
  }

  move(i: number, d: number) {
    const ids = refIds(this.value());
    const j = i + d;
    if (j < 0 || j >= ids.length) return;
    [ids[i], ids[j]] = [ids[j], ids[i]];
    this.changed.emit(ids);
  }
}

function mediaItem(m: Media): RefItem {
  return { id: m.id, label: m.filename, sub: m.kind, thumb: m.kind === 'image' ? m.url : undefined };
}
