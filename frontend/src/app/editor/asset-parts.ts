import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal, untracked } from '@angular/core';
import { TPipe } from '../core/i18n';
import { parseColor } from './color-utils';
import { FONTS, PART_CLASS } from './design-css';
import { DNode, InstanceOverride, NODE_ICON, PartStyle, Style, findNode, pathTo } from './design-types';
import { EditorStore } from './editor-store';
import './messages-fx';

const WEIGHTS = [300, 400, 500, 600, 700, 800];

/**
 * Where the selection sits (Page › Hero › Card) and the layers inside it, so
 * nested parts of sections, post cards and components are one click away.
 */
@Component({
  selector: 'rea-layer-path',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TPipe],
  template: `
    @if (path().length > 1) {
      <nav class="lp-path" [attr.aria-label]="'pt.path' | t">
        @for (p of path(); track p.id; let last = $last) {
          @if (!last) {
            <button (click)="store.select(p.id)" [title]="'pt.selectParent' | t">{{ p.name || p.type }}</button><span>›</span>
          } @else { <b>{{ p.name || p.type }}</b> }
        }
      </nav>
    }
    @if (kids().length) {
      <div class="lp-kids">
        <span>{{ 'pt.inside' | t }}</span>
        @for (k of kids(); track k.id) {
          <button (click)="store.select(k.id)" [title]="'pt.selectChild' | t"><i>{{ icon(k) }}</i>{{ k.name || k.type }}</button>
        }
      </div>
    }
  `,
  styles: `
    .lp-path { display: flex; flex-wrap: wrap; align-items: center; gap: 2px 4px; font-size: 11px; color: var(--fg-muted, #888); padding: 8px 12px 0;
      button { border: 0; background: none; padding: 1px 4px; border-radius: 4px; color: #4f46e5; cursor: pointer; font: inherit; &:hover { background: #eef0ff; } }
      b { color: var(--fg-text, #222); font-weight: 600; } }
    .lp-kids { display: flex; flex-wrap: wrap; gap: 4px; padding: 6px 12px 0; font-size: 11px; color: var(--fg-muted, #888); align-items: center;
      button { display: inline-flex; gap: 4px; align-items: center; border: 1px solid var(--fg-border, #e5e5e5); background: #fff; border-radius: 999px;
        padding: 2px 8px; cursor: pointer; font: inherit; color: var(--fg-text, #222); max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
        &:hover { border-color: #4f46e5; color: #4f46e5; } i { font-style: normal; opacity: .6; } } }
  `,
})
export class LayerPath {
  readonly store = inject(EditorStore);
  readonly node = input.required<DNode>();
  readonly path = computed(() => pathTo(this.store.doc(), this.node().id));
  /** Direct children (not for instances: their layers come from the component). */
  readonly kids = computed(() => (this.node().type === 'instance' ? [] : (this.node().children ?? [])).slice(0, 12));
  icon(n: DNode) { return NODE_ICON[n.type] ?? '▢'; }
}

/** The component's layers, changed for this placement only (props.overrides). */
@Component({
  selector: 'rea-instance-parts',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TPipe],
  template: `
    <section class="fg-sec">
      <h5>{{ 'pt.instTitle' | t }}</h5>
      <p class="hint">{{ 'pt.instHint' | t }}</p>
      @for (l of layers(); track l.node.id) {
        @let o = ov()[l.node.id];
        <div class="pt-row" [style.padding-left.px]="8 + l.depth * 12" [class.open]="open() === l.node.id" [class.changed]="!!o">
          <button class="pt-head" (click)="open.set(open() === l.node.id ? null : l.node.id)">
            <i>{{ icon(l.node) }}</i><span>{{ l.node.name || l.node.type }}</span>@if (o) { <em>●</em> }
          </button>
          @if (open() === l.node.id) {
            <div class="pt-edit">
              @if (l.node.type === 'text' || l.node.type === 'button') {
                <textarea rows="2" [value]="o?.text ?? l.node.props?.text ?? ''" (change)="set(l.node.id, { text: val($event) })"></textarea>
              }
              <div class="pt-grid">
                <label>{{ 'fx.text' | t }}<input type="color" [value]="hex(o?.style?.color ?? l.node.style?.color, '#111827')" (change)="setStyle(l.node.id, { color: val($event) })" /></label>
                <label>{{ 'fx.fill' | t }}<input type="color" [value]="hex(o?.style?.fill ?? l.node.style?.fill, '#ffffff')" (change)="setStyle(l.node.id, { fill: val($event) })" /></label>
                <label>{{ 'pt.size' | t }}<input type="number" min="1" [value]="o?.style?.fontSize ?? l.node.style?.fontSize ?? ''" placeholder="16" (change)="setStyle(l.node.id, { fontSize: num($event) })" /></label>
                <label>{{ 'pt.radius' | t }}<input type="number" min="0" [value]="o?.style?.radius ?? l.node.style?.radius ?? ''" placeholder="0" (change)="setStyle(l.node.id, { radius: num($event) })" /></label>
              </div>
              <div class="pt-actions">
                <label class="fg-check"><input type="checkbox" [checked]="!!o?.hidden" (change)="set(l.node.id, { hidden: checked($event) || undefined })" /> {{ 'pt.hide' | t }}</label>
                @if (o) { <button class="fg-link" (click)="reset(l.node.id)">{{ 'pt.reset' | t }}</button> }
              </div>
            </div>
          }
        </div>
      } @empty { <p class="hint">{{ 'pt.instEmpty' | t }}</p> }
    </section>
  `,
  styleUrl: './asset-parts.scss',
})
export class InstanceParts {
  private readonly store = inject(EditorStore);
  readonly node = input.required<DNode>();
  readonly open = signal<string | null>(null);
  readonly ov = computed<Record<string, InstanceOverride>>(() => this.node().props?.overrides ?? {});
  /** The component's layers, flattened with their depth. */
  readonly layers = computed(() => {
    const comp = this.store.componentMap().get(this.node().props?.componentId ?? -1);
    const out: { node: DNode; depth: number }[] = [];
    const walk = (n: DNode, depth: number) => n.children?.forEach(c => { out.push({ node: c, depth }); walk(c, depth + 1); });
    if (comp) walk(comp.node, 0);
    return out.slice(0, 80);
  });

  icon(n: DNode) { return NODE_ICON[n.type] ?? '▢'; }
  hex(c: string | undefined, def: string) { return parseColor(c)?.hex ?? def; }
  val(e: Event) { return (e.target as HTMLInputElement).value; }
  num(e: Event) { const v = (e.target as HTMLInputElement).value; return v === '' ? undefined : Number(v); }
  checked(e: Event) { return (e.target as HTMLInputElement).checked; }

  set(id: string, patch: Partial<InstanceOverride>) {
    const cur: InstanceOverride = { ...this.ov()[id], ...patch };
    for (const k of Object.keys(cur) as (keyof InstanceOverride)[]) if (cur[k] === undefined) delete cur[k];
    this.write(id, Object.keys(cur).length ? cur : null);
  }
  setStyle(id: string, patch: Partial<Style>) {
    const style = { ...this.ov()[id]?.style, ...patch };
    for (const k of Object.keys(style) as (keyof Style)[]) if (style[k] === undefined) delete style[k];
    this.set(id, { style: Object.keys(style).length ? style : undefined });
  }
  reset(id: string) { this.write(id, null); }
  private write(id: string, o: InstanceOverride | null) {
    const all = { ...this.ov() };
    if (o) all[id] = o; else delete all[id];
    this.store.setProps(this.node().id, { overrides: Object.keys(all).length ? all : undefined });
  }
}

/** Elements inside a plugin block (found in its preview), restyled by class (props.parts). */
@Component({
  selector: 'rea-block-parts',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TPipe],
  template: `
    <section class="fg-sec">
      <h5>{{ 'pt.blockTitle' | t }}</h5>
      <p class="hint">{{ 'pt.blockHint' | t }}</p>
      @for (p of found(); track p.cls) {
        @let s = parts()[p.cls];
        <div class="pt-row" [class.open]="open() === p.cls" [class.changed]="!!s">
          <button class="pt-head" (click)="open.set(open() === p.cls ? null : p.cls)">
            <i>.</i><span>{{ p.sample || p.cls }}</span><small>{{ p.cls }}</small>@if (s) { <em>●</em> }
          </button>
          @if (open() === p.cls) {
            <div class="pt-edit">
              <div class="pt-grid">
                <label>{{ 'fx.text' | t }}<input type="color" [value]="hex(s?.color, '#111827')" (change)="set(p.cls, { color: val($event) })" /></label>
                <label>{{ 'fx.fill' | t }}<input type="color" [value]="hex(s?.fill, '#ffffff')" (change)="set(p.cls, { fill: val($event) })" /></label>
                <label>{{ 'pt.size' | t }}<input type="number" min="1" [value]="s?.fontSize ?? ''" (change)="set(p.cls, { fontSize: num($event) })" /></label>
                <label>{{ 'pt.radius' | t }}<input type="number" min="0" [value]="s?.radius ?? ''" (change)="set(p.cls, { radius: num($event) })" /></label>
              </div>
              <select [value]="s?.fontWeight ?? ''" (change)="set(p.cls, { fontWeight: num($event) })">
                <option value="">{{ 'pt.weight' | t }}</option>
                @for (w of weights; track w) { <option [value]="w">{{ w }}</option> }
              </select>
              <details class="pt-more" [open]="more()">
                <summary (click)="more.set(!more()); $event.preventDefault()">{{ 'pt.more' | t }}</summary>
                <select [value]="s?.fontFamily ?? ''" (change)="set(p.cls, { fontFamily: val($event) || undefined })">
                  <option value="">{{ 'pt.font' | t }}</option>
                  @for (f of fonts; track f) { <option [value]="f" [selected]="s?.fontFamily === f">{{ f }}</option> }
                </select>
                <div class="pt-grid">
                  <label>{{ 'pt.align' | t }}<select [value]="s?.textAlign ?? ''" (change)="set(p.cls, { textAlign: $any(val($event)) || undefined })"><option value="">—</option><option value="left">⟸</option><option value="center">⟺</option><option value="right">⟹</option></select></label>
                  <label>{{ 'pt.case' | t }}<select [value]="s?.textTransform ?? ''" (change)="set(p.cls, { textTransform: $any(val($event)) || undefined })"><option value="">—</option><option value="uppercase">ABC</option><option value="capitalize">Abc</option><option value="lowercase">abc</option></select></label>
                  <label>{{ 'pt.line' | t }}<input type="number" step="0.05" [value]="s?.lineHeight ?? ''" (change)="set(p.cls, { lineHeight: num($event) })" /></label>
                  <label>{{ 'pt.spacing' | t }}<input type="number" step="0.1" [value]="s?.letterSpacing ?? ''" (change)="set(p.cls, { letterSpacing: num($event) })" /></label>
                  <label>{{ 'pt.padY' | t }}<input type="number" min="0" [value]="s?.padY ?? ''" (change)="set(p.cls, { padY: num($event) })" /></label>
                  <label>{{ 'pt.padX' | t }}<input type="number" min="0" [value]="s?.padX ?? ''" (change)="set(p.cls, { padX: num($event) })" /></label>
                  <label>{{ 'pt.stroke' | t }}<input type="color" [value]="hex(s?.stroke, '#e5e7eb')" (change)="set(p.cls, { stroke: val($event) })" /></label>
                  <label>{{ 'pt.strokeW' | t }}<input type="number" min="0" max="20" [value]="s?.strokeWidth ?? ''" (change)="set(p.cls, { strokeWidth: num($event) })" /></label>
                  <label>{{ 'pt.opacity' | t }}<input type="number" min="0" max="100" [value]="pct(s?.opacity)" (change)="setOpacity(p.cls, $event)" /></label>
                  <label>{{ 'pt.shadow' | t }}<select [value]="s?.shadow ?? ''" (change)="set(p.cls, { shadow: val($event) || undefined })">
                    <option value="">—</option>
                    @for (sh of shadows; track sh.v) { <option [value]="sh.v" [selected]="s?.shadow === sh.v">{{ sh.label | t }}</option> }
                  </select></label>
                  <label>{{ 'pt.hoverText' | t }}<input type="color" [value]="hex(s?.hover?.color, '#111827')" (change)="setHover(p.cls, { color: val($event) })" /></label>
                  <label>{{ 'pt.hoverFill' | t }}<input type="color" [value]="hex(s?.hover?.fill, '#ffffff')" (change)="setHover(p.cls, { fill: val($event) })" /></label>
                </div>
                @if (s?.stroke || s?.hover) {
                  <div class="pt-actions">
                    @if (s?.stroke) { <button class="fg-link" (click)="set(p.cls, { stroke: undefined, strokeWidth: undefined })">{{ 'pt.noStroke' | t }}</button> }
                    @if (s?.hover) { <button class="fg-link" (click)="set(p.cls, { hover: undefined })">{{ 'pt.noHover' | t }}</button> }
                  </div>
                }
              </details>
              <div class="pt-actions">
                <label class="fg-check"><input type="checkbox" [checked]="!!s?.hidden" (change)="set(p.cls, { hidden: checked($event) || undefined })" /> {{ 'pt.hide' | t }}</label>
                @if (s) { <button class="fg-link" (click)="reset(p.cls)">{{ 'pt.reset' | t }}</button> }
              </div>
            </div>
          }
        </div>
      } @empty { <p class="hint">{{ 'pt.blockEmpty' | t }}</p> }
      <p class="hint">💡 {{ 'pt.dblHint' | t }}</p>
    </section>
  `,
  styleUrl: './asset-parts.scss',
})
export class BlockParts {
  private readonly store = inject(EditorStore);
  readonly node = input.required<DNode>();
  readonly open = signal<string | null>(null);
  readonly weights = WEIGHTS;
  readonly parts = computed<Record<string, PartStyle>>(() => this.node().props?.parts ?? {});
  readonly more = signal(false);
  readonly fonts = FONTS;
  pct(v?: number) { return v === undefined ? '' : Math.round(v * 100); }
  readonly shadows = [
    { v: '0 1px 3px rgba(0,0,0,.12)', label: 'pt.sh.soft' },
    { v: '0 8px 24px rgba(17,24,39,.12)', label: 'pt.sh.float' },
    { v: '0 20px 48px rgba(17,24,39,.18)', label: 'pt.sh.deep' },
  ];
  /** A part double-clicked on the canvas opens here. */
  private readonly picked = effect(() => {
    const p = this.store.partPick();
    if (p && p.id === this.node().id) untracked(() => { this.open.set(p.cls); this.store.partPick.set(null); });
  });
  /** Class names used inside the block preview, with a text sample of each. */
  readonly found = computed(() => {
    const html = this.store.blockPreview(this.node());
    if (!html) return [];
    const doc = new DOMParser().parseFromString(html, 'text/html');
    const seen = new Map<string, string>();
    doc.body.querySelectorAll('[class]').forEach(el => {
      const cls = [...el.classList].find(c => PART_CLASS.test(c) && !c.startsWith('pb-'));
      if (cls && !seen.has(cls)) seen.set(cls, (el.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 28));
    });
    return [...seen].map(([cls, sample]) => ({ cls, sample })).slice(0, 30);
  });

  hex(c: string | undefined, def: string) { return parseColor(c)?.hex ?? def; }
  val(e: Event) { return (e.target as HTMLInputElement).value; }
  num(e: Event) { const v = (e.target as HTMLInputElement).value; return v === '' ? undefined : Number(v); }
  checked(e: Event) { return (e.target as HTMLInputElement).checked; }

  set(cls: string, patch: Partial<PartStyle>) {
    const cur: PartStyle = { ...this.parts()[cls], ...patch };
    for (const k of Object.keys(cur) as (keyof PartStyle)[]) if (cur[k] === undefined) delete cur[k];
    this.write(cls, Object.keys(cur).length ? cur : null);
  }
  setHover(cls: string, patch: { color?: string; fill?: string }) { this.set(cls, { hover: { ...this.parts()[cls]?.hover, ...patch } }); }
  setOpacity(cls: string, e: Event) {
    const v = this.num(e);
    this.set(cls, { opacity: v === undefined || v >= 100 ? undefined : Math.max(0, v) / 100 });
  }
  reset(cls: string) { this.write(cls, null); }
  private write(cls: string, p: PartStyle | null) {
    const all = { ...this.parts() };
    if (p) all[cls] = p; else delete all[cls];
    this.store.setProps(this.node().id, { parts: Object.keys(all).length ? all : undefined });
  }
}

/** Selects the first card of a post list / collection (its template). */
export function firstCard(store: EditorStore, n: DNode) {
  const c = n.children?.[0];
  if (c && findNode(store.doc(), c.id)) store.select(c.id);
}
