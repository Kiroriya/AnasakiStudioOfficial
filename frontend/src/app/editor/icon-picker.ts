import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { TPipe } from '../core/i18n';
import { DNode } from './design-types';
import { EditorStore } from './editor-store';
import './messages-fx';

/** Content tab of an icon or shape layer: pick from the built-in sets, line weight, flips, label. */
@Component({
  selector: 'rea-icon-picker',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TPipe],
  template: `
    @let n = node();
    @if (forButton()) {
      <!-- 2.2.0: the icon of a button (props.icon + iconPosition), drawn in the text colour -->
      <section class="fg-sec" id="pp-button-icon">
        <h5>{{ 'ic.buttonIcon' | t }}</h5>
        <div class="fg-seg">
          <button [class.on]="!n.props?.icon" (click)="set({ icon: undefined, iconPosition: undefined })">{{ 'ic.noIcon' | t }}</button>
          <button [class.on]="!!n.props?.icon && n.props.iconPosition !== 'end'" [disabled]="!n.props?.icon" (click)="set({ iconPosition: undefined })">⟵ {{ 'ic.before' | t }}</button>
          <button [class.on]="n.props?.iconPosition === 'end'" [disabled]="!n.props?.icon" (click)="set({ iconPosition: 'end' })">{{ 'ic.after' | t }} ⟶</button>
        </div>
        @if (n.props?.icon) {
          <!-- 2.3.0: icon size in px (empty = a little larger than the text) -->
          <label class="fg-field ip-size" [title]="'ic.sizeTip' | t"><span>{{ 'ic.size' | t }}</span>
            <input type="number" id="pp-icon-size" min="8" max="200" [value]="n.props.iconSize ?? ''" [placeholder]="'ic.sizeAuto' | t" [attr.aria-label]="'ic.size' | t"
                   (change)="set({ iconSize: +val($event) > 0 ? +val($event) : undefined })" /><em>px</em></label>
        }
        <input class="ip-search" type="search" [value]="q()" (input)="q.set(val($event))" [placeholder]="'ic.search' | t" [attr.aria-label]="'ic.search' | t" />
        <div class="ip-grid">
          @for (name of iconNames(); track name) {
            <button [class.on]="n.props?.icon === name" [title]="name" [attr.aria-label]="name" (click)="set({ icon: name })" [innerHTML]="iconSvg(name)"></button>
          } @empty { <p class="hint">{{ 'ic.none' | t }}</p> }
        </div>
      </section>
    } @else {
    @if (n.type === 'icon') {
      <section class="fg-sec">
        <h5>{{ 'ic.icon' | t }}</h5>
        <input class="ip-search" type="search" [value]="q()" (input)="q.set(val($event))" [placeholder]="'ic.search' | t" />
        <div class="ip-grid">
          @for (name of iconNames(); track name) {
            <button [class.on]="(n.props?.icon ?? 'star') === name" [title]="name" (click)="set({ icon: name })" [innerHTML]="iconSvg(name)"></button>
          } @empty { <p class="hint">{{ 'ic.none' | t }}</p> }
        </div>
        <label class="ip-row">{{ 'ic.weight' | t }}
          <input type="range" min="1" max="3" step="0.25" [value]="n.props?.strokeWidth ?? 2" (change)="set({ strokeWidth: num($event) })" />
          <em>{{ n.props?.strokeWidth ?? 2 }}</em>
        </label>
        <p class="hint">{{ 'ic.colorHint' | t }}</p>
      </section>
    } @else {
      <section class="fg-sec">
        <h5>{{ 'ic.dividers' | t }}</h5>
        <div class="ip-grid wide">
          @for (name of shapeNames('divider'); track name) {
            <button [class.on]="n.props?.shape === name" [title]="name" (click)="set({ shape: name })" [innerHTML]="shapeSvg(name)"></button>
          }
        </div>
        <h5>{{ 'ic.blobs' | t }}</h5>
        <div class="ip-grid">
          @for (name of shapeNames('blob'); track name) {
            <button [class.on]="n.props?.shape === name" [title]="name" (click)="set({ shape: name })" [innerHTML]="shapeSvg(name)"></button>
          }
        </div>
        <div class="fg-seg">
          <button [class.on]="!!n.props?.flipX" (click)="set({ flipX: !n.props?.flipX || undefined })">⇋ {{ 'ic.flipX' | t }}</button>
          <button [class.on]="!!n.props?.flipY" (click)="set({ flipY: !n.props?.flipY || undefined })">⇅ {{ 'ic.flipY' | t }}</button>
        </div>
        <p class="hint">{{ 'ic.shapeHint' | t }}</p>
      </section>
    }
    <section class="fg-sec">
      <h5>{{ 'ic.label' | t }}</h5>
      <input class="ip-search" [value]="n.props?.label ?? ''" (change)="set({ label: val($event) || undefined })" [placeholder]="'ic.labelPh' | t" />
    </section>
    }
  `,
  styles: `
    .ip-search { width: 100%; margin-bottom: 6px; }
    .ip-grid { display: grid; grid-template-columns: repeat(6, 1fr); gap: 4px; max-height: 236px; overflow: auto; margin-bottom: 8px;
      button { aspect-ratio: 1; border: 1px solid transparent; background: var(--fg-hover, #f5f5f5); border-radius: 6px; padding: 6px; cursor: pointer; color: #1f2937;
        &:hover { border-color: #c7d2fe; } &.on { border-color: #4f46e5; background: #eef0ff; color: #4f46e5; } }
      &.wide { grid-template-columns: repeat(2, 1fr); button { aspect-ratio: 4 / 1; padding: 4px 0 0; } } }
    .ip-row { display: grid; grid-template-columns: auto 1fr 28px; gap: 6px; align-items: center; font-size: 11px; color: var(--fg-muted, #888); em { font-style: normal; } }
    .hint { font-size: 11px; color: var(--fg-muted, #888); margin: 4px 0 0; }
  `,
})
export class IconPicker {
  private readonly store = inject(EditorStore);
  private readonly sanitizer = inject(DomSanitizer);
  readonly node = input.required<DNode>();
  /** The icon of a button instead of an icon layer (2.2.0). */
  readonly forButton = input(false);
  readonly q = signal('');

  readonly iconNames = computed(() => {
    const q = this.q().trim().toLowerCase();
    const all = this.store.assets()?.iconNames ?? [];
    return q ? all.filter(n => n.includes(q)) : all;
  });

  private cache = new Map<string, SafeHtml>();

  iconSvg(name: string): SafeHtml { return this.svg('i:' + name, { type: 'icon', props: { icon: name } }); }
  shapeSvg(name: string): SafeHtml { return this.svg('s:' + name, { type: 'shape', props: { shape: name } }); }
  private svg(key: string, n: Pick<DNode, 'type' | 'props'>): SafeHtml {
    let v = this.cache.get(key);
    if (!v) {
      // built-in markup from our own server (design.AssetCatalog), not user content
      v = this.sanitizer.bypassSecurityTrustHtml(this.store.svgFor({ id: '', width: { mode: 'hug' }, height: { mode: 'hug' }, ...n } as DNode));
      this.cache.set(key, v);
    }
    return v;
  }

  shapeNames(kind: 'divider' | 'blob') {
    const s = this.store.assets()?.shapes ?? {};
    return Object.keys(s).filter(k => s[k].kind === kind);
  }

  set(patch: NonNullable<DNode['props']>) { this.store.setProps(this.store.selectedIds(), patch); }
  val(e: Event) { return (e.target as HTMLInputElement).value; }
  num(e: Event) { return Number((e.target as HTMLInputElement).value); }
}
