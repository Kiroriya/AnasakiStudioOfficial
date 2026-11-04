import { ChangeDetectionStrategy, Component, DestroyRef, computed, inject, input } from '@angular/core';
import { TPipe } from '../core/i18n';
import { parseColor, toCss } from './color-utils';
import { AnimType, DNode, Hover, Style } from './design-types';
import { EditorStore } from './editor-store';
import './messages-fx';
import { Scrub } from './scrub';

/** A gradient as the editor edits it (written as a CSS gradient string). */
interface Grad { kind: 'linear' | 'radial'; angle: number; stops: { color: string; pos: number }[] }

const GRADIENTS = [
  'linear-gradient(135deg,#4f46e5 0%,#c026d3 100%)', 'linear-gradient(135deg,#f97316 0%,#ec4899 100%)',
  'linear-gradient(135deg,#06b6d4 0%,#3b82f6 100%)', 'linear-gradient(135deg,#22c55e 0%,#14b8a6 100%)',
  'linear-gradient(180deg,#111827 0%,#374151 100%)', 'linear-gradient(135deg,#fde68a 0%,#fca5a5 50%,#c4b5fd 100%)',
  'radial-gradient(circle,#ffffff 0%,#e0e7ff 100%)', 'linear-gradient(90deg,#ff7a7a 0%,#ffd36e 100%)',
];

const SHADOWS = {
  soft: '0px 12px 28px 0px rgba(17,24,39,0.12)',
  strong: '0px 20px 40px 0px rgba(17,24,39,0.22)',
  glow: '0px 0px 24px 4px rgba(79,70,229,0.45)',
} as const;

/**
 * Extra decoration tools of the Design tab: gradients (fill or text), image
 * overlay, per-corner radius, dashed borders, blur / frosted glass, image
 * filters, text effects, hover state and scroll-in animation. Values live in
 * the node's style (so breakpoint overrides work like everywhere else).
 */
@Component({
  selector: 'rea-style-fx',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TPipe, Scrub],
  template: `
    @let s = st();
    @let n = node();

    <!-- Gradient -->
    <section class="fg-sec">
      <h5>{{ 'fx.gradient' | t }}
        @if (!grad()) { <button class="fg-add" [title]="'fx.gradientAdd' | t" [attr.aria-label]="'fx.gradientAdd' | t" (click)="setGrad(parse(presets[0])!)">+</button> }
      </h5>
      @if (isText()) {
        <div class="fg-seg fx-target">
          <button [class.on]="target() === 'fill'" (click)="moveGrad('fill')">{{ 'fx.toBox' | t }}</button>
          <button [class.on]="target() === 'textGradient'" (click)="moveGrad('textGradient')">{{ 'fx.toText' | t }}</button>
        </div>
      }
      @if (grad(); as g) {
        <div class="fx-grad-bar" [style.background]="css(g, true)"></div>
        <div class="fg-row2">
          <select [value]="g.kind" (change)="setGrad({ ...g, kind: $any(val($event)) })">
            <option value="linear">{{ 'fx.linear' | t }}</option><option value="radial">{{ 'fx.radial' | t }}</option>
          </select>
          @if (g.kind === 'linear') {
            <label class="fg-field" [title]="'fx.angle' | t"><span [reaScrub]="g.angle" (scrub)="setGrad({ ...g, angle: $event })">∠</span>
              <input [attr.aria-label]="'fx.angle' | t" type="number" [value]="g.angle" (change)="setGrad({ ...g, angle: num($event) ?? 0 })" /><em>°</em></label>
          } @else { <span></span> }
        </div>
        @for (stop of g.stops; track $index; let i = $index) {
          <div class="fg-paint">
            <input type="color" [value]="hex(stop.color)" (change)="setStop(g, i, { color: val($event) })" />
            <input class="fg-hex" [value]="hex(stop.color).slice(1).toUpperCase()" (change)="setStop(g, i, { color: '#' + val($event) })" />
            <label class="fg-pct"><input type="number" min="0" max="100" [value]="stop.pos" (change)="setStop(g, i, { pos: num($event) ?? 0 })" />%</label>
            @if (g.stops.length > 2) { <button class="fg-rm" (click)="setGrad({ ...g, stops: without(g.stops, i) })">−</button> }
          </div>
        }
        <div class="fg-row2">
          @if (g.stops.length < 4) { <button class="fg-link" (click)="addStop(g)">+ {{ 'fx.addStop' | t }}</button> } @else { <span></span> }
          <button class="fg-link" (click)="clearGrad()">{{ 'fx.remove' | t }}</button>
        </div>
      }
      <div class="swatches">
        @for (p of presets; track p) { <button [style.background]="p" [title]="p" [attr.aria-label]="p" (click)="setGrad(parse(p)!)"></button> }
      </div>
    </section>

    <!-- Image overlay -->
    @if (s.backgroundImage) {
      <section class="fg-sec">
        <h5>{{ 'fx.overlay' | t }}</h5>
        <div class="fg-paint">
          @let ov = overlay();
          <input type="color" [value]="ov.hex" (change)="style({ overlay: toCss(val($event), ov.a || 0.45) })" />
          <label class="fg-pct" [title]="'fx.overlayStrength' | t"><input type="number" min="0" max="100" [value]="s.overlay ? round(ov.a * 100) : 0"
            (change)="style({ overlay: (num($event) ?? 0) > 0 ? toCss(ov.hex, (num($event) ?? 0) / 100) : undefined })" />%</label>
          @if (s.overlay) { <button class="fg-rm" (click)="style({ overlay: undefined })">−</button> }
        </div>
        <p class="hint">{{ 'fx.overlayHint' | t }}</p>
      </section>
    }

    <!-- Corners & border style -->
    <section class="fg-sec">
      <h5>{{ 'fx.corners' | t }}
        <button class="fg-add" [class.on]="!!s.radii" [title]="'fx.cornersEach' | t" [attr.aria-label]="'fx.cornersEach' | t" (click)="toggleRadii()">{{ s.radii ? '◻' : '⌗' }}</button>
      </h5>
      @if (s.radii; as r) {
        <div class="fg-row4">
          @for (c of corners; track c.i) {
            <label class="fg-field" [title]="c.title | t"><span [reaScrub]="r[c.i]" [minValue]="0" (scrub)="setCorner(r, c.i, $event)">{{ c.icon }}</span>
              <input [attr.aria-label]="c.title | t" type="number" min="0" [value]="r[c.i]" (change)="setCorner(r, c.i, num($event) ?? 0)" /></label>
          }
        </div>
      } @else {
        <p class="hint">{{ 'fx.cornersHint' | t }}</p>
      }
      @if (s.stroke) {
        <div class="fg-seg">
          @for (b of borders; track b.id) {
            <button [class.on]="(s.strokeStyle ?? 'solid') === b.id" [title]="b.title | t" [attr.aria-label]="b.title | t" (click)="style({ strokeStyle: b.id === 'solid' ? undefined : $any(b.id) })">{{ b.icon }}</button>
          }
        </div>
      }
    </section>

    <!-- Blur & filters -->
    <section class="fg-sec">
      <h5>{{ 'fx.blurFilters' | t }}
        @if (hasFilters()) { <button class="fg-link" (click)="resetFilters()">{{ 'fx.reset' | t }}</button> }
      </h5>
      <div class="fg-row2">
        <label class="fg-field" [title]="'fx.blur' | t"><span [reaScrub]="s.blur ?? 0" [minValue]="0" (scrub)="style({ blur: $event || undefined })">◌</span>
          <input [attr.aria-label]="'fx.blur' | t" type="number" min="0" [value]="s.blur ?? 0" (change)="style({ blur: num($event) || undefined })" /></label>
        <label class="fg-field" [title]="'fx.glass' | t"><span [reaScrub]="s.backdropBlur ?? 0" [minValue]="0" (scrub)="style({ backdropBlur: $event || undefined })">❄</span>
          <input [attr.aria-label]="'fx.glass' | t" type="number" min="0" [value]="s.backdropBlur ?? 0" (change)="style({ backdropBlur: num($event) || undefined })" /></label>
      </div>
      <button class="fg-link" (click)="glass()">✨ {{ 'fx.glassPreset' | t }}</button>
      @for (f of filters; track f.key) {
        <label class="fx-slider">
          <span>{{ f.label | t }}</span>
          <input type="range" [min]="f.min" [max]="f.max" [value]="s[f.key] ?? f.def" (input)="slide(f.key, $event, f.def)" (change)="endLive()" (blur)="endLive()" />
          <em>{{ s[f.key] ?? f.def }}%</em>
        </label>
      }
    </section>

    <!-- Text effects -->
    @if (hasText()) {
      <section class="fg-sec">
        <h5>{{ 'fx.textFx' | t }}</h5>
        <div class="fg-row2">
          <div class="fg-seg">
            <button [class.on]="!!s.italic" [title]="'fx.italic' | t" [attr.aria-label]="'fx.italic' | t" (click)="style({ italic: !s.italic || undefined })"><i>I</i></button>
            <button [class.on]="s.textDecoration === 'underline'" [title]="'fx.underline' | t" [attr.aria-label]="'fx.underline' | t" (click)="style({ textDecoration: s.textDecoration === 'underline' ? undefined : 'underline' })"><u>U</u></button>
            <button [class.on]="s.textDecoration === 'line-through'" [title]="'fx.strike' | t" [attr.aria-label]="'fx.strike' | t" (click)="style({ textDecoration: s.textDecoration === 'line-through' ? undefined : 'line-through' })"><s>S</s></button>
          </div>
          <select [value]="textShadowKind()" (change)="setTextShadow(val($event))" [title]="'fx.textShadow' | t">
            <option value="">{{ 'fx.tsNone' | t }}</option>
            <option value="soft">{{ 'fx.tsSoft' | t }}</option>
            <option value="strong">{{ 'fx.tsStrong' | t }}</option>
            <option value="glow">{{ 'fx.tsGlow' | t }}</option>
            @if (textShadowKind() === 'custom') { <option value="custom">{{ s.textShadow }}</option> }
          </select>
        </div>
      </section>
    }

    <!-- Hover -->
    <section class="fg-sec">
      <h5>{{ 'fx.hover' | t }}
        @if (s.hover) { <button class="fg-link" (click)="store.previewFx(n.id, 'hover')">▶ {{ 'fx.preview' | t }}</button> }
      </h5>
      <div class="fg-seg fx-presets">
        @for (p of hoverPresets; track p.id) { <button [title]="p.title | t" (click)="hoverPreset(p.id)">{{ p.icon }} {{ p.label | t }}</button> }
      </div>
      <!-- colours on hover are always offered (2.2.0: they only showed after picking an effect) -->
      <div class="fx-colors" id="fx-hover-colors">
        <label [title]="'fx.hoverFill' | t">{{ 'fx.fill' | t }}
          <input type="color" id="fx-hover-fill" [attr.aria-label]="'fx.hoverFill' | t" [value]="hex(s.hover?.fill, hoverBase(s.fill, '#ffffff'))" (change)="hover({ fill: val($event) })" />
          @if (s.hover?.fill) { <button class="fg-rm" (click)="hover({ fill: undefined })">−</button> }</label>
        <label [title]="'fx.hoverColor' | t">{{ 'fx.text' | t }}
          <input type="color" id="fx-hover-color" [attr.aria-label]="'fx.hoverColor' | t" [value]="hex(s.hover?.color, hoverBase(s.color, '#111827'))" (change)="hover({ color: val($event) })" />
          @if (s.hover?.color) { <button class="fg-rm" (click)="hover({ color: undefined })">−</button> }</label>
        <label [title]="'fx.hoverStroke' | t">{{ 'fx.strokeShort' | t }}
          <input type="color" id="fx-hover-stroke" [attr.aria-label]="'fx.hoverStroke' | t" [value]="hex(s.hover?.stroke, hoverBase(s.stroke, '#d1d5db'))" (change)="hover({ stroke: val($event) })" />
          @if (s.hover?.stroke) { <button class="fg-rm" (click)="hover({ stroke: undefined })">−</button> }</label>
      </div>
      @if (s.hover; as h) {
        <div class="fg-row2">
          <label class="fg-field" [title]="'fx.scale' | t"><span [reaScrub]="pct(h.scale)" [minValue]="50" (scrub)="hover({ scale: $event / 100 })">⤢</span>
            <input [attr.aria-label]="'fx.scale' | t" type="number" min="50" max="200" [value]="pct(h.scale)" (change)="hover({ scale: (num($event) ?? 100) / 100 })" /><em>%</em></label>
          <label class="fg-field" [title]="'fx.lift' | t"><span [reaScrub]="h.lift ?? 0" (scrub)="hover({ lift: $event || undefined })">↑</span>
            <input [attr.aria-label]="'fx.lift' | t" type="number" [value]="h.lift ?? 0" (change)="hover({ lift: num($event) || undefined })" /><em>px</em></label>
        </div>
        <div class="fg-row2">
          <select [value]="shadowKind(h.shadow)" (change)="hover({ shadow: shadowOf(val($event)) })" [title]="'fx.hoverShadow' | t">
            <option value="">{{ 'fx.shNone' | t }}</option><option value="soft">{{ 'fx.shSoft' | t }}</option>
            <option value="strong">{{ 'fx.shStrong' | t }}</option><option value="glow">{{ 'fx.shGlow' | t }}</option>
          </select>
          <label class="fg-field" [title]="'fx.duration' | t"><span>⏱</span>
            <input [attr.aria-label]="'fx.duration' | t" type="number" min="0" step="50" [value]="h.duration ?? 250" (change)="hover({ duration: num($event) })" /><em>ms</em></label>
        </div>
        <button class="fg-link" (click)="style({ hover: undefined })">{{ 'fx.remove' | t }}</button>
      }
    </section>

    <!-- Animation -->
    <section class="fg-sec">
      <h5>{{ 'fx.anim' | t }}
        @if (s.anim) { <button class="fg-link" (click)="store.previewFx(n.id, 'anim')">▶ {{ 'fx.play' | t }}</button> }
      </h5>
      <select [value]="s.anim?.type ?? ''" (change)="setAnim(val($event))">
        <option value="">{{ 'fx.animNone' | t }}</option>
        @for (a of anims; track a) { <option [value]="a">{{ ('fx.anim.' + a) | t }}</option> }
      </select>
      @if (s.anim; as a) {
        <div class="fg-row2">
          <label class="fg-field" [title]="'fx.duration' | t"><span>⏱</span>
            <input [attr.aria-label]="'fx.duration' | t" type="number" min="100" step="100" [value]="a.duration ?? 700" (change)="style({ anim: { ...a, duration: num($event) } })" /><em>ms</em></label>
          <label class="fg-field" [title]="'fx.delay' | t"><span>⏳</span>
            <input [attr.aria-label]="'fx.delay' | t" type="number" min="0" step="100" [value]="a.delay ?? 0" (change)="style({ anim: { ...a, delay: num($event) || undefined } })" /><em>ms</em></label>
        </div>
        <p class="hint">{{ 'fx.animHint' | t }}</p>
      }
    </section>
  `,
  styles: `
    :host { display: contents; }
    .fx-target { margin-bottom: 6px; }
    .fx-grad-bar { height: 22px; border-radius: 6px; margin-bottom: 6px; border: 1px solid var(--fg-border, #e5e5e5); }
    .fx-slider { display: grid; grid-template-columns: 74px 1fr 40px; align-items: center; gap: 6px; font-size: 11px; color: var(--fg-muted, #888);
      input { width: 100%; } em { font-style: normal; text-align: right; } }
    .fx-colors { display: flex; gap: 10px; margin-bottom: 6px; font-size: 11px; color: var(--fg-muted, #888);
      label { display: flex; align-items: center; gap: 4px; } input[type=color] { width: 26px; height: 22px; padding: 0; border: 1px solid #ddd; border-radius: 4px; } }
    .fx-presets { flex-wrap: wrap; margin-bottom: 6px; button { flex: 1 1 auto; white-space: nowrap; } }
    .hint { font-size: 11px; color: var(--fg-muted, #888); margin: 4px 0 0; }
  `,
})
export class StyleFx {
  readonly store = inject(EditorStore);
  readonly node = input.required<DNode>();
  /** The effective node at the current breakpoint. */
  readonly eff = input.required<DNode>();
  readonly st = computed<Partial<Style>>(() => this.eff().style ?? {});

  readonly presets = GRADIENTS;
  readonly round = Math.round;
  readonly toCss = toCss;
  readonly anims: AnimType[] = ['fade', 'up', 'down', 'left', 'right', 'zoom'];
  readonly corners = [
    { i: 0, icon: '◜', title: 'fx.cTL' }, { i: 1, icon: '◝', title: 'fx.cTR' },
    { i: 2, icon: '◞', title: 'fx.cBR' }, { i: 3, icon: '◟', title: 'fx.cBL' },
  ];
  readonly borders = [
    { id: 'solid', icon: '━', title: 'fx.bSolid' }, { id: 'dashed', icon: '┅', title: 'fx.bDashed' }, { id: 'dotted', icon: '┈', title: 'fx.bDotted' },
  ];
  readonly filters: { key: 'grayscale' | 'brightness' | 'contrast' | 'saturate'; label: string; min: number; max: number; def: number }[] = [
    { key: 'grayscale', label: 'fx.grayscale', min: 0, max: 100, def: 0 },
    { key: 'brightness', label: 'fx.brightness', min: 0, max: 200, def: 100 },
    { key: 'contrast', label: 'fx.contrast', min: 0, max: 200, def: 100 },
    { key: 'saturate', label: 'fx.saturate', min: 0, max: 300, def: 100 },
  ];
  readonly hoverPresets = [
    { id: 'lift', icon: '↑', label: 'fx.hpLift', title: 'fx.hpLiftT' },
    { id: 'grow', icon: '⤢', label: 'fx.hpGrow', title: 'fx.hpGrowT' },
    { id: 'glow', icon: '✦', label: 'fx.hpGlow', title: 'fx.hpGlowT' },
    { id: 'fade', icon: '◐', label: 'fx.hpFade', title: 'fx.hpFadeT' },
  ];

  readonly isText = computed(() => this.node().type === 'text');
  readonly hasText = computed(() => !['image', 'rect', 'instance', 'icon', 'shape'].includes(this.node().type));
  /** Where the gradient goes: the box (fill) or the letters (text only). */
  readonly target = computed<'fill' | 'textGradient'>(() => (this.isText() && this.st().textGradient ? 'textGradient' : 'fill'));
  readonly grad = computed(() => this.parse(this.target() === 'textGradient' ? this.st().textGradient : this.st().fill));
  readonly overlay = computed(() => parseColor(this.st().overlay) ?? { hex: '#000000', a: 0.45 });
  readonly hasFilters = computed(() => {
    const s = this.st();
    return !!(s.blur || s.backdropBlur || s.grayscale || (s.brightness ?? 100) !== 100 || (s.contrast ?? 100) !== 100 || (s.saturate ?? 100) !== 100);
  });

  style(patch: Partial<Style>) { this.store.setStyle(this.store.selectedIds(), patch); }

  private live = false;
  private liveTimer?: ReturnType<typeof setTimeout>;
  private readonly closeLive = inject(DestroyRef).onDestroy(() => this.endLive());
  /** Slider drags update live as one undo step (closed on change, blur, or after a pause if `change` never comes). */
  liveStyle(patch: Partial<Style>) {
    if (!this.live) { this.store.beginGesture(); this.live = true; }
    this.style(patch);
    clearTimeout(this.liveTimer);
    this.liveTimer = setTimeout(() => this.endLive(), 800);
  }
  slide(key: 'grayscale' | 'brightness' | 'contrast' | 'saturate', e: Event, def: number) { this.liveStyle({ [key]: this.rangeVal(e, def) }); }
  endLive() {
    clearTimeout(this.liveTimer);
    if (this.live) { this.store.endGesture(); this.live = false; }
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  num(e: Event): number | undefined {
    const v = (e.target as HTMLInputElement).value;
    return v === '' ? undefined : Number(v);
  }
  rangeVal(e: Event, def: number): number | undefined {
    const v = Number((e.target as HTMLInputElement).value);
    return v === def ? undefined : v;
  }
  hex(c?: string, def = '#000000') { return parseColor(c)?.hex ?? def; }
  /** Where an unset hover colour picker starts: the layer's own colour a little darker (#FF7A7A → #E06B6B). */
  hoverBase(c: string | undefined, def: string): string {
    const h = parseColor(c)?.hex;
    if (!h) return def;
    const ch = (i: number) => Math.round(parseInt(h.slice(i, i + 2), 16) * 0.88).toString(16).padStart(2, '0');
    return '#' + ch(1) + ch(3) + ch(5);
  }
  pct(v?: number) { return Math.round((v ?? 1) * 100); }
  without<T>(list: T[], i: number) { return list.filter((_, j) => j !== i); }

  // ---- gradient
  parse(v?: string): Grad | null { return parseGradient(v); }
  css(g: Grad, bar = false) { return formatGradient(bar && g.kind === 'radial' ? { ...g, kind: 'linear', angle: 90 } : g); }
  setGrad(g: Grad) { this.style({ [this.target()]: formatGradient(g) }); }
  setStop(g: Grad, i: number, patch: Partial<Grad['stops'][number]>) {
    this.setGrad({ ...g, stops: g.stops.map((s, j) => (j === i ? { ...s, ...patch } : s)) });
  }
  addStop(g: Grad) {
    const last = g.stops[g.stops.length - 1];
    this.setGrad({ ...g, stops: [...g.stops.map(s => ({ ...s, pos: Math.round(s.pos * 0.8) })), { color: last.color, pos: 100 }] });
  }
  clearGrad() { this.style({ [this.target()]: undefined }); }
  /** Moves the gradient between the box and the letters of a text layer. */
  moveGrad(to: 'fill' | 'textGradient') {
    if (to === this.target()) return;
    const cur = this.target() === 'textGradient' ? this.st().textGradient : (isGradient(this.st().fill) ? this.st().fill : undefined);
    const g = cur ?? GRADIENTS[0];
    this.style(to === 'textGradient' ? { textGradient: g, fill: isGradient(this.st().fill) ? undefined : this.st().fill } : { fill: g, textGradient: undefined });
  }

  // ---- corners
  toggleRadii() {
    const s = this.st();
    if (s.radii) this.style({ radii: undefined, radius: s.radii[0] || undefined });
    else { const r = s.radius ?? 0; this.style({ radii: [r, r, r, r] }); }
  }
  setCorner(r: [number, number, number, number], i: number, v: number) {
    const next = [...r] as [number, number, number, number];
    next[i] = Math.max(0, v);
    this.style({ radii: next });
  }

  // ---- filters
  glass() {
    const s = this.st();
    const fill = parseColor(s.fill);
    this.style({ backdropBlur: 14, fill: toCss(fill?.hex ?? '#ffffff', 0.55), stroke: s.stroke ?? 'rgba(255,255,255,0.6)', strokeWidth: s.strokeWidth ?? 1 });
  }
  resetFilters() { this.style({ blur: undefined, backdropBlur: undefined, grayscale: undefined, brightness: undefined, contrast: undefined, saturate: undefined }); }

  // ---- text
  textShadowKind(): string {
    const v = this.st().textShadow;
    if (!v) return '';
    return Object.entries(TEXT_SHADOWS).find(([, x]) => x === v)?.[0] ?? 'custom';
  }
  setTextShadow(kind: string) {
    if (kind === 'custom') return;
    this.style({ textShadow: TEXT_SHADOWS[kind as keyof typeof TEXT_SHADOWS] });
  }

  // ---- hover
  hover(patch: Partial<Hover>) {
    const h: Hover = { ...this.st().hover, ...patch };
    for (const k of Object.keys(h) as (keyof Hover)[]) if (h[k] === undefined || (k === 'scale' && h[k] === 1)) delete h[k];
    this.style({ hover: Object.keys(h).length ? h : undefined });
    this.store.previewFx(this.node().id, 'hover');
  }
  hoverPreset(id: string) {
    const p: Record<string, Hover> = {
      lift: { lift: 6, shadow: SHADOWS.strong },
      grow: { scale: 1.05 },
      glow: { shadow: SHADOWS.glow },
      fade: { opacity: 0.8 },
    };
    this.hover({ ...p[id] });
  }
  shadowKind(v?: string) { return v ? Object.entries(SHADOWS).find(([, x]) => x === v)?.[0] ?? 'soft' : ''; }
  shadowOf(kind: string) { return kind ? SHADOWS[kind as keyof typeof SHADOWS] : undefined; }

  // ---- animation
  setAnim(type: string) {
    if (!type) { this.style({ anim: undefined }); return; }
    this.style({ anim: { ...this.st().anim, type: type as AnimType } });
    this.store.previewFx(this.node().id, 'anim');
  }
}

const TEXT_SHADOWS = {
  soft: '0px 2px 6px rgba(0,0,0,0.25)',
  strong: '0px 3px 0px rgba(0,0,0,0.35)',
  glow: '0px 0px 12px rgba(255,255,255,0.85)',
};

function isGradient(v?: string) { return !!v && /gradient\(/.test(v); }

/** Reads "linear-gradient(135deg,#a 0%,#b 100%)" / "radial-gradient(circle,…)" (null for anything else). */
export function parseGradient(v?: string): Grad | null {
  const m = /^\s*(linear|radial)-gradient\((.*)\)\s*$/i.exec(v ?? '');
  if (!m) return null;
  const parts: string[] = [];
  let depth = 0, cur = '';
  for (const ch of m[2]) {
    if (ch === '(') depth++;
    if (ch === ')') depth--;
    if (ch === ',' && depth === 0) { parts.push(cur.trim()); cur = ''; } else cur += ch;
  }
  if (cur.trim()) parts.push(cur.trim());
  let angle = 180;
  if (parts.length && /deg$|^to |^circle|^ellipse/.test(parts[0])) {
    const a = /(-?[\d.]+)deg/.exec(parts[0]);
    if (a) angle = Number(a[1]);
    parts.shift();
  }
  const stops = parts.map((p, i) => {
    const pm = /^(.*?)\s+([\d.]+)%$/.exec(p);
    return { color: (pm ? pm[1] : p).trim(), pos: pm ? Number(pm[2]) : Math.round((i / Math.max(1, parts.length - 1)) * 100) };
  });
  if (stops.length < 2) return null;
  return { kind: m[1].toLowerCase() as Grad['kind'], angle, stops };
}

export function formatGradient(g: Grad): string {
  const stops = g.stops.map(s => `${s.color} ${Math.round(s.pos)}%`).join(',');
  return g.kind === 'radial' ? `radial-gradient(circle,${stops})` : `linear-gradient(${Math.round(g.angle)}deg,${stops})`;
}
