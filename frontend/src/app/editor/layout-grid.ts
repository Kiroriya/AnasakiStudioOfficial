import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { TPipe } from '../core/i18n';
import { parseColor, toCss } from './color-utils';
import { DNode, LayoutGrid } from './design-types';
import { EditorStore } from './editor-store';
import { GRID_COLOR, GRID_PRESETS, GuidePrefs } from './guides-model';
import './messages-grid';
import { Scrub } from './scrub';

/** Design tab section "Layout grid" of a frame (Figma): presets plus type / count / gutter / margin / size / colour. */
@Component({
  selector: 'rea-layout-grid-editor',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TPipe, Scrub],
  template: `
    @let g = grid();
    <section class="fg-sec">
      <h5>{{ 'lg.title' | t }}
        <span>
          @if (g) {
            <button class="fg-add" [title]="'lg.show' | t" [attr.aria-label]="'lg.show' | t" (click)="set({ visible: g.visible === false ? undefined : false })">{{ g.visible === false ? '◌' : '👁' }}</button>
            <button class="fg-add" [title]="'lg.remove' | t" [attr.aria-label]="'lg.remove' | t" (click)="replace(undefined)">−</button>
          } @else {
            <button class="fg-add" [title]="'lg.add' | t" [attr.aria-label]="'lg.add' | t" (click)="replace(presets.p12)">+</button>
          }
        </span>
      </h5>
      <div class="fg-seg lg-presets">
        <button [class.on]="is('p12')" (click)="replace(presets.p12)">{{ 'lg.p12' | t }}</button>
        <button [class.on]="is('p4')" (click)="replace(presets.p4)">{{ 'lg.p4' | t }}</button>
        <button [class.on]="is('p8')" (click)="replace(presets.p8)">{{ 'lg.p8' | t }}</button>
        <button [class.on]="!g" (click)="replace(undefined)">{{ 'lg.none' | t }}</button>
      </div>
      @if (g) {
        <div class="fg-row2 tight">
          <select [value]="g.type" (change)="setType($any(val($event)))">
            <option value="columns">{{ 'lg.columns' | t }}</option>
            <option value="rows">{{ 'lg.rows' | t }}</option>
            <option value="grid">{{ 'lg.grid' | t }}</option>
          </select>
          @if (g.type === 'grid') {
            <label class="fg-field" [title]="'lg.size' | t"><span [reaScrub]="g.size ?? 8" [minValue]="1" (scrub)="set({ size: $event })">▦</span>
              <input [attr.aria-label]="'lg.size' | t" type="number" min="1" [value]="g.size ?? 8" (change)="set({ size: pos($event, 8) })" /></label>
          } @else {
            <label class="fg-field" [title]="'lg.count' | t"><span [reaScrub]="g.count ?? 12" [minValue]="1" (scrub)="set({ count: $event })">#</span>
              <input [attr.aria-label]="'lg.count' | t" type="number" min="1" max="48" [value]="g.count ?? 12" (change)="set({ count: pos($event, 12) })" /></label>
          }
        </div>
        @if (g.type !== 'grid') {
          <div class="fg-row2 tight">
            <label class="fg-field" [title]="'lg.gutter' | t"><span [reaScrub]="g.gutter ?? 0" [minValue]="0" (scrub)="set({ gutter: $event })">⇹</span>
              <input [attr.aria-label]="'lg.gutter' | t" type="number" min="0" [value]="g.gutter ?? 0" (change)="set({ gutter: pos($event, 0, 0) })" /></label>
            <label class="fg-field" [title]="'lg.margin' | t"><span [reaScrub]="g.margin ?? 0" [minValue]="0" (scrub)="set({ margin: $event })">⇤</span>
              <input [attr.aria-label]="'lg.margin' | t" type="number" min="0" [value]="g.margin ?? 0" (change)="set({ margin: pos($event, 0, 0) })" /></label>
          </div>
        }
        @if (color(); as c) {
          <div class="fg-paint" [title]="'lg.color' | t">
            <input type="color" [value]="c.hex" (change)="set({ color: toCss(val($event), c.a) })" />
            <input class="fg-hex" [value]="c.hex.slice(1).toUpperCase()" (change)="set({ color: toCss('#' + val($event), c.a) })" />
            <label class="fg-pct"><input type="number" min="0" max="100" [value]="round(c.a * 100)" (change)="set({ color: toCss(c.hex, (num($event) ?? 10) / 100) })" />%</label>
          </div>
        }
        <p class="hint">{{ (prefs.grids() ? 'lg.hint' : 'lg.hidden') | t }}</p>
      }
    </section>
  `,
  styles: `.lg-presets { display: flex; margin-bottom: 6px; } .lg-presets button { flex: 1; white-space: nowrap; padding-inline: 4px; } h5 span { display: flex; gap: 2px; }`,
})
export class LayoutGridEditor {
  readonly node = input.required<DNode>();
  readonly store = inject(EditorStore);
  readonly prefs = inject(GuidePrefs);
  readonly presets = GRID_PRESETS;
  readonly toCss = toCss;
  readonly round = Math.round;

  readonly grid = computed(() => this.node().props?.layoutGrid ?? null);
  readonly color = computed(() => parseColor(this.grid()?.color || GRID_COLOR) ?? parseColor(GRID_COLOR));

  is(p: keyof typeof GRID_PRESETS) {
    const g = this.grid(), q = GRID_PRESETS[p];
    return !!g && g.type === q.type && (q.type === 'grid' ? g.size === q.size : g.count === q.count && g.gutter === q.gutter && g.margin === q.margin);
  }

  replace(g: LayoutGrid | undefined) {
    // keep the user's colour when switching presets
    const keep = this.grid()?.color;
    this.store.setProps(this.node().id, { layoutGrid: g ? { ...g, color: keep ?? g.color } : undefined });
  }

  set(patch: Partial<LayoutGrid>) {
    const g = this.grid();
    if (!g) return;
    const next = { ...g, ...patch } as LayoutGrid;
    for (const k of Object.keys(next) as (keyof LayoutGrid)[]) if (next[k] === undefined) delete next[k];
    this.store.setProps(this.node().id, { layoutGrid: next });
  }

  setType(type: LayoutGrid['type']) {
    const g = this.grid();
    if (!g || g.type === type) return;
    if (type === 'grid') this.set({ type, size: g.size ?? 8 });
    else this.set({ type, count: g.count ?? (type === 'rows' ? 4 : 12), gutter: g.gutter ?? 20, margin: g.margin ?? 0 });
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  num(e: Event): number | undefined {
    const v = parseFloat(this.val(e));
    return Number.isFinite(v) ? v : undefined;
  }
  pos(e: Event, def: number, min = 1) { return Math.max(min, Math.round(this.num(e) ?? def)); }
}
