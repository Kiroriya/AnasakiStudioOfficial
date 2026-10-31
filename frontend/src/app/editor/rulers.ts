import {
  ChangeDetectionStrategy, Component, ElementRef, OnDestroy, ViewEncapsulation, afterEveryRender, afterNextRender, computed, inject, input, signal,
} from '@angular/core';
import { TPipe } from '../core/i18n';
import { DNode } from './design-types';
import { EditorStore } from './editor-store';
import { GuideAxis, GuidePrefs, activeGrid, docGuides, gridBands, snapOffset, writeGuides } from './guides-model';
import './messages-grid';

const R = 18; // ruler thickness, screen px
const SNAP_PX = 5;
const STEPS = [1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000];

/** Artboard origin and size in this overlay's (screen) px. */
interface Geo { ox: number; oy: number; z: number; w: number; h: number }
interface Box { x: number; y: number; w: number; h: number }
interface GridView extends Box { id: string; color: string; bands: Box[]; bg: string | null }
interface Draft { axis: GuideAxis; v: number; from: number; out: boolean; px: number; py: number }

/**
 * Figma rulers, ruler guides and layout grids, drawn over the canvas viewport
 * (not inside it, so they stay put while the page scrolls). Rulers count design
 * px from the artboard's top-left at the current zoom; guides are dragged out
 * of a ruler and saved on the root node (props.guides) through the store.
 */
@Component({
  selector: 'rea-rulers',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: { class: 'rl-host', '[class.rl-on]': 'prefs.rulers()', '[class.rl-idle]': 'store.tool() !== "move"', '(wheel)': 'wheel($event)' },
  styles: `
    .rl-host { position: absolute; inset: 0; pointer-events: none; overflow: hidden; }
    .fg-canvas:has(> rea-canvas .rl-on) > .fg-bp, .fg-canvas:has(> rea-canvas .rl-on) > .tpl-banner { top: 28px; }
    .rl-grid { position: absolute; z-index: 4; pointer-events: none; overflow: hidden; }
    .rl-grid i { position: absolute; display: block; }
    .rl-gd { position: absolute; z-index: 6; pointer-events: auto; }
    .rl-gd::after { content: ''; position: absolute; background: #f2487a; }
    .rl-gd.v { top: 0; bottom: 0; width: 7px; margin-left: -3px; cursor: ew-resize; }
    .rl-gd.v::after { top: 0; bottom: 0; left: 3px; width: 1px; }
    .rl-gd.h { left: 0; right: 0; height: 7px; margin-top: -3px; cursor: ns-resize; }
    .rl-gd.h::after { left: 0; right: 0; top: 3px; height: 1px; }
    .rl-gd.sel::after, .rl-gd:hover::after { background: var(--fg-blue); }
    .rl-gd.draft { pointer-events: none; }
    .rl-gd.draft.out::after { opacity: .35; }
    .rl-idle .rl-gd { pointer-events: none; }
    .rl-tip { position: absolute; z-index: 9; pointer-events: none; background: #1e1e1e; color: #fff; font-size: 11px; padding: 2px 7px; border-radius: 4px; white-space: nowrap; }
    .rl-bar { position: absolute; z-index: 8; pointer-events: auto; background: #fff; overflow: hidden; }
    .rl-bar.top { left: 0; right: 0; top: 0; height: ${R}px; border-bottom: 1px solid var(--fg-border); cursor: ns-resize; }
    .rl-bar.left { top: 0; bottom: 0; left: 0; width: ${R}px; border-right: 1px solid var(--fg-border); cursor: ew-resize; }
    .rl-bar svg { position: absolute; inset: 0; width: 100%; height: 100%; }
    .rl-bar path { stroke: #b3b3b3; stroke-width: 1; fill: none; shape-rendering: crispEdges; }
    .rl-bar text { font-size: 9px; fill: #8c8c8c; font-family: 'Inter', system-ui, sans-serif; }
    .rl-bar .rl-sel { fill: rgba(13,153,255,.14); }
    .rl-bar .rl-mark { fill: var(--fg-blue); font-weight: 600; }
    .rl-corner { position: absolute; z-index: 9; left: 0; top: 0; width: ${R}px; height: ${R}px; background: #fff; border: solid var(--fg-border); border-width: 0 1px 1px 0; pointer-events: auto; }
  `,
  template: `
    @let g = geo();
    @for (v of grids(); track v.id) {
      <div class="rl-grid" [style.left.px]="v.x" [style.top.px]="v.y" [style.width.px]="v.w" [style.height.px]="v.h"
           [style.background-image]="v.bg" [style.background-size.px]="v.bg ? cell(v) : null">
        @for (b of v.bands; track $index) {
          <i [style.left.px]="b.x" [style.top.px]="b.y" [style.width.px]="b.w" [style.height.px]="b.h" [style.background]="v.color"></i>
        }
      </div>
    }

    @if (prefs.guides()) {
      @for (x of guides().x; track $index; let i = $index) {
        @if (!(draft()?.axis === 'x' && draft()?.from === i)) {
          <div class="rl-gd v" [class.sel]="isSel('x', i)" [style.left.px]="g.ox + x * g.z" [title]="'gd.guide' | t" (pointerdown)="grab($event, 'x', i)"></div>
        }
      }
      @for (y of guides().y; track $index; let i = $index) {
        @if (!(draft()?.axis === 'y' && draft()?.from === i)) {
          <div class="rl-gd h" [class.sel]="isSel('y', i)" [style.top.px]="g.oy + y * g.z" [title]="'gd.guide' | t" (pointerdown)="grab($event, 'y', i)"></div>
        }
      }
    }
    @if (draft(); as d) {
      @if (d.axis === 'x') { <div class="rl-gd v sel draft" [class.out]="d.out" [style.left.px]="g.ox + d.v * g.z"></div> }
      @else { <div class="rl-gd h sel draft" [class.out]="d.out" [style.top.px]="g.oy + d.v * g.z"></div> }
      <div class="rl-tip" [style.left.px]="d.px + 12" [style.top.px]="d.py + 14">{{ d.out ? ('gd.removeHint' | t) : (d.axis === 'x' ? 'X ' : 'Y ') + d.v }}</div>
    }

    @if (prefs.rulers()) {
      <div class="rl-bar top" [title]="'gd.ruler' | t" (pointerdown)="fromRuler($event, 'y')">
        <svg>
          @if (selBox(); as s) { <rect class="rl-sel" [attr.x]="g.ox + s.x * g.z" y="0" [attr.width]="s.w * g.z" [attr.height]="R" /> }
          <path [attr.d]="ticksX().d" />
          @for (l of ticksX().labels; track l.p) { <text [attr.x]="l.p + 3" y="9">{{ l.t }}</text> }
          @if (selBox(); as s) {
            <text class="rl-mark" [attr.x]="g.ox + s.x * g.z + 2" y="16">{{ round(s.x) }}</text>
            <text class="rl-mark" [attr.x]="g.ox + (s.x + s.w) * g.z - 2" y="16" text-anchor="end">{{ round(s.x + s.w) }}</text>
          }
        </svg>
      </div>
      <div class="rl-bar left" [title]="'gd.ruler' | t" (pointerdown)="fromRuler($event, 'x')">
        <svg>
          @if (selBox(); as s) { <rect class="rl-sel" x="0" [attr.y]="g.oy + s.y * g.z" [attr.width]="R" [attr.height]="s.h * g.z" /> }
          <path [attr.d]="ticksY().d" />
          @for (l of ticksY().labels; track l.p) { <text [attr.transform]="'translate(9 ' + (l.p - 3) + ') rotate(-90)'">{{ l.t }}</text> }
        </svg>
      </div>
      <div class="rl-corner"></div>
    }
  `,
})
export class RulersOverlay implements OnDestroy {
  readonly viewport = input.required<HTMLElement>();
  readonly artboard = input.required<HTMLElement>();
  readonly store = inject(EditorStore);
  readonly prefs = inject(GuidePrefs);
  private host: HTMLElement = inject(ElementRef).nativeElement;

  readonly R = R;
  readonly round = Math.round;
  readonly geo = signal<Geo>({ ox: 0, oy: 0, z: 1, w: 0, h: 0 });
  readonly grids = signal<GridView[]>([]);
  readonly selBox = signal<Box | null>(null);
  readonly draft = signal<Draft | null>(null);
  readonly selGuide = signal<{ axis: GuideAxis; i: number } | null>(null);
  readonly guides = computed(() => docGuides(this.store.doc()));
  readonly ticksX = computed(() => { const g = this.geo(); return ticks(g.ox, g.z, g.w, true); });
  readonly ticksY = computed(() => { const g = this.geo(); return ticks(g.oy, g.z, g.h, false); });

  private ro = new ResizeObserver(() => this.refresh());
  private cleanup: (() => void)[] = [];

  constructor() {
    afterNextRender(() => {
      const vp = this.viewport();
      this.ro.observe(vp);
      const scroll = () => this.refresh();
      const key = (e: KeyboardEvent) => this.key(e);
      const down = (e: PointerEvent) => { if (!(e.target as HTMLElement).closest?.('.rl-gd')) this.selGuide.set(null); };
      vp.addEventListener('scroll', scroll, { passive: true });
      window.addEventListener('keydown', key, true); // before the editor's document handler
      window.addEventListener('pointerdown', down, true);
      this.cleanup.push(() => vp.removeEventListener('scroll', scroll), () => window.removeEventListener('keydown', key, true),
        () => window.removeEventListener('pointerdown', down, true));
    });
    afterEveryRender(() => this.refresh());
  }

  ngOnDestroy() {
    this.ro.disconnect();
    this.cleanup.forEach(f => f());
  }

  isSel(axis: GuideAxis, i: number) {
    const s = this.selGuide();
    return !!s && s.axis === axis && s.i === i;
  }

  cell(v: GridView) {
    const lg = findGrid(this.store.doc(), v.id);
    return (lg?.size ?? 8) * this.geo().z;
  }

  // ------------------------------------------------------------------ measuring

  private refresh() {
    const vp = this.viewport(), art = this.artboard();
    const h = this.host.getBoundingClientRect(), a = art.getBoundingClientRect();
    const z = this.store.zoom();
    const geo: Geo = { ox: a.left - h.left, oy: a.top - h.top, z, w: vp.clientWidth, h: vp.clientHeight };
    if (!same(geo, this.geo())) this.geo.set(geo);

    // selection extent, for the blue band on the rulers
    const rects = this.prefs.rulers() ? this.store.selectedIds().map(id => this.store.measure?.(id)).filter(r => !!r) : [];
    let sel: Box | null = null;
    if (rects.length) {
      const x0 = Math.min(...rects.map(r => r!.x)), y0 = Math.min(...rects.map(r => r!.y));
      sel = { x: x0, y: y0, w: Math.max(...rects.map(r => r!.x + r!.w)) - x0, h: Math.max(...rects.map(r => r!.y + r!.h)) - y0 };
    }
    if (!same(sel, this.selBox())) this.selBox.set(sel);

    const out: GridView[] = [];
    if (this.prefs.grids()) {
      walk(this.store.doc(), n => {
        const lg = activeGrid(n, this.prefs);
        if (!lg) return;
        const el = art.querySelector(`[data-node-id="${CSS.escape(n.id)}"]`);
        const r = el?.getBoundingClientRect();
        if (!r || !r.width || !r.height) return;
        const box = { id: n.id, x: r.left - h.left, y: r.top - h.top, w: r.width, h: r.height };
        const color = lg.color || 'rgba(255,0,0,0.1)';
        if (lg.type === 'grid') {
          const cell = (lg.size ?? 8) * z;
          if (cell < 4) return; // too dense to be useful at this zoom
          const line = `linear-gradient(to right, ${color} 1px, transparent 1px), linear-gradient(to bottom, ${color} 1px, transparent 1px)`;
          out.push({ ...box, color, bands: [], bg: line });
          return;
        }
        const cols = lg.type === 'columns';
        const bands = gridBands(lg, (cols ? r.width : r.height) / z).map(b => cols
          ? { x: b.start * z, y: 0, w: b.size * z, h: r.height }
          : { x: 0, y: b.start * z, w: r.width, h: b.size * z });
        out.push({ ...box, color, bands: bands.map(roundBox), bg: null });
      });
    }
    if (JSON.stringify(out) !== JSON.stringify(this.grids())) this.grids.set(out);
  }

  // ------------------------------------------------------------------ guides

  /** Press on a ruler: the top ruler makes horizontal guides (y), the left one vertical (x). */
  fromRuler(e: PointerEvent, axis: GuideAxis) {
    if (e.button !== 0) return;
    this.drag(e, axis, -1);
  }

  /** Press on a guide: select it, drag to move, drop on the ruler to remove. */
  grab(e: PointerEvent, axis: GuideAxis, i: number) {
    if (e.button !== 0) return;
    this.selGuide.set({ axis, i });
    this.drag(e, axis, i);
  }

  private drag(e: PointerEvent, axis: GuideAxis, from: number) {
    e.preventDefault();
    e.stopPropagation();
    const sx = e.clientX, sy = e.clientY;
    let moved = false;
    const at = (ev: PointerEvent): Draft => {
      const h = this.host.getBoundingClientRect(), g = this.geo();
      const px = ev.clientX - h.left, py = ev.clientY - h.top;
      let v = axis === 'x' ? (px - g.ox) / g.z : (py - g.oy) / g.z;
      const s = this.selBox();
      if (s && !(ev.ctrlKey || ev.metaKey)) { // snap to the selection's edges / centre
        const t = axis === 'x' ? [s.x, s.x + s.w / 2, s.x + s.w] : [s.y, s.y + s.h / 2, s.y + s.h];
        v += snapOffset(t, v, SNAP_PX / g.z);
      }
      const out = (axis === 'x' ? px : py) < R;
      return { axis, v: Math.round(v), from, out, px, py };
    };
    const move = (ev: PointerEvent) => {
      if (!moved && Math.abs(ev.clientX - sx) + Math.abs(ev.clientY - sy) < 3) return;
      moved = true;
      this.draft.set(at(ev));
    };
    const up = (ev: PointerEvent) => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      document.body.classList.remove('rea-dragging');
      this.draft.set(null);
      if (!moved) return;
      const d = at(ev);
      const g = docGuides(this.store.doc());
      const list = g[axis];
      if (from >= 0) list.splice(from, 1);
      if (!d.out) list.push(d.v);
      if (from < 0 && d.out) return; // dragged out and straight back: nothing to do
      writeGuides(this.store, g);
      this.selGuide.set(d.out ? null : { axis, i: list.length - 1 });
    };
    document.body.classList.add('rea-dragging');
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
  }

  private key(e: KeyboardEvent) {
    const s = this.selGuide();
    if (!s) return;
    const t = e.target as HTMLElement;
    if (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName)) return;
    if (e.key === 'Escape') { this.selGuide.set(null); e.stopPropagation(); e.preventDefault(); return; }
    if (e.key !== 'Delete' && e.key !== 'Backspace') return;
    e.stopPropagation(); // the editor would delete the selected layer
    e.preventDefault();
    const g = docGuides(this.store.doc());
    g[s.axis].splice(s.i, 1);
    writeGuides(this.store, g);
    this.selGuide.set(null);
  }

  /** The rulers sit outside the scrolling viewport: pass wheel scrolling / Ctrl-zoom through. */
  wheel(e: WheelEvent) {
    const vp = this.viewport();
    if (e.ctrlKey || e.metaKey) { e.preventDefault(); vp.dispatchEvent(new WheelEvent('wheel', e)); return; }
    vp.scrollLeft += e.deltaX;
    vp.scrollTop += e.deltaY;
  }
}

// ------------------------------------------------------------------ helpers

/** Tick marks for one ruler: a path (major ticks full height) and the major labels. */
function ticks(origin: number, z: number, len: number, top: boolean): { d: string; labels: { p: number; t: string }[] } {
  const major = STEPS.find(s => s * z >= 64) ?? STEPS[STEPS.length - 1];
  const minor = major % 5 === 0 ? major / 5 : major / 2;
  const per = Math.round(major / minor);
  const k0 = Math.ceil((R - origin) / z / minor), k1 = Math.floor((len - origin) / z / minor);
  let d = '';
  const labels: { p: number; t: string }[] = [];
  for (let k = k0; k <= k1 && k - k0 < 2000; k++) {
    const p = Math.round(origin + k * minor * z) + 0.5;
    const isMajor = k % per === 0;
    const l = isMajor ? R : 4;
    d += top ? `M${p} ${R}V${R - l}` : `M${R} ${p}H${R - l}`;
    if (isMajor) labels.push({ p, t: String(Math.round(k * minor)) });
  }
  return { d, labels };
}

function walk(n: DNode, fn: (n: DNode) => void) {
  fn(n);
  n.children?.forEach(c => walk(c, fn));
}

function findGrid(root: DNode, id: string) {
  let out: DNode | null = null;
  walk(root, n => { if (n.id === id) out = n; });
  return (out as DNode | null)?.props?.layoutGrid;
}

function roundBox(b: Box): Box {
  return { x: Math.round(b.x * 10) / 10, y: Math.round(b.y * 10) / 10, w: Math.round(b.w * 10) / 10, h: Math.round(b.h * 10) / 10 };
}

function same<T>(a: T, b: T) {
  return JSON.stringify(a) === JSON.stringify(b);
}
