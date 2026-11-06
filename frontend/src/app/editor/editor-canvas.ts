import {
  ChangeDetectionStrategy, Component, ElementRef, OnDestroy, afterEveryRender, afterNextRender, computed, inject, signal, viewChild,
} from '@angular/core';
import { TPipe, tr } from '../core/i18n';
import { NodeView } from './canvas-node';
import { BREAKPOINTS, DNode, findNode, isAncestor, isContainer, newId } from './design-types';
import { EditorStore, Rect } from './editor-store';
import { resizedBox } from './editor-utils';
import { GuidePrefs, guideSnapTargets, snapOffset } from './guides-model';
import { RulersOverlay } from './rulers';
import { VectorOverlay } from './vector-overlay';

interface Line { x1: number; y1: number; x2: number; y2: number; label?: string }
type Handle = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw';
interface SpaceZone { kind: 'pad' | 'gap'; side: number; x: number; y: number; w: number; h: number; value: number; horizontal: boolean }
interface SnapCtx { box: Rect; xs: number[]; ys: number[]; pr: Rect }

const SNAP_PX = 5; // screen pixels
const HINT_SECTION = 'ed.hint.section';

/**
 * The Figma-like canvas: renders the artboard and handles every pointer
 * interaction (select, move with smart guides, reorder/reparent, resize,
 * draw tools, hand/pan, marquee, zoom, Alt-measure).
 */
@Component({
  selector: 'rea-canvas',
  imports: [NodeView, TPipe, RulersOverlay, VectorOverlay],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="viewport" #viewport
         [class.cur-hand]="handMode()" [class.cur-grabbing]="panning()" [class.cur-cross]="drawTool()"
         (contextmenu)="bgMenu($event)">
      <div class="stage" #stage (pointerleave)="store.hoverId.set(null)">
        <div class="artboard-label">{{ store.title() }} <span>· {{ bp().label }} {{ bp().width }}</span></div>
        @if (store.bp() !== 'desktop') {
          <div class="artboard-bp">{{ 'ed.bpOnly' | t: { bp: bp().label } }}</div>
          @if (overflow().length; as n) {
            <button type="button" class="artboard-overflow" (click)="selectOverflow()" [title]="'ed.overflow.hint' | t">
              ⚠ {{ 'ed.overflow.count' | t: { n: n, w: bp().width } }}
            </button>
          }
        }
        <div class="artboard rea-canvas" #artboard [style]="store.themeVars()" [style.width.px]="bp().width" [style.--rea-screen-h.px]="bp().screenH" [style.zoom]="store.zoom()">
          <div reaNode [node]="store.doc()" parentMode=""></div>
        </div>
        <rea-vector-overlay /><!-- pen tool + point editing (vector-overlay.ts) -->

        <!-- overlays (screen space, not zoomed) -->
        @if (hoverRect(); as r) {
          <div class="ov ov-hover" [class.inst]="hoverIsInstance()" [style.left.px]="r.x" [style.top.px]="r.y" [style.width.px]="r.w" [style.height.px]="r.h"></div>
        }
        @for (r of multiRects(); track $index) {
          <div class="ov ov-multi" [style.left.px]="r.x" [style.top.px]="r.y" [style.width.px]="r.w" [style.height.px]="r.h"></div>
        }
        @if (selRect(); as r) {
          <div class="ov ov-sel" [class.inst]="selIsInstance()" [style.left.px]="r.x" [style.top.px]="r.y" [style.width.px]="r.w" [style.height.px]="r.h">
            @if (!store.editingTextId()) {
              <span class="ov-size">{{ sizeLabel() }}</span>
              @if (canResize()) {
                @for (h of handles; track h) { <i class="hdl hdl-{{ h }}" (pointerdown)="resize($event, h)"></i> }
              }
            }
          </div>
        }
        @for (o of overflow(); track o.id) {
          <div class="ov ov-overflow" [style.left.px]="o.r.x" [style.top.px]="o.r.y" [style.width.px]="o.r.w" [style.height.px]="o.r.h"></div>
        }
        @if (dropTargetRect(); as r) {
          <div class="ov ov-droptarget" [style.left.px]="r.x" [style.top.px]="r.y" [style.width.px]="r.w" [style.height.px]="r.h"></div>
        }
        @for (z of spacing(); track $index) {
          <div class="ov-space" [class.gap]="z.kind === 'gap'" [class.hz]="z.horizontal" [style.left.px]="z.x" [style.top.px]="z.y" [style.width.px]="z.w" [style.height.px]="z.h"
               [title]="(z.kind === 'gap' ? 'ed.hint.gap' : 'ed.hint.pad') | t" (pointerdown)="spaceDrag($event, z)">
            <i></i><span>{{ z.value }}</span>
          </div>
        }
        @if (store.dropIndicator(); as d) {
          <div class="ov ov-drop" [style.left.px]="d.x" [style.top.px]="d.y" [style.width.px]="d.w" [style.height.px]="d.h"></div>
        }
        @for (g of guides(); track $index) {
          <div class="ov ov-guide" [style.left.px]="min(g.x1, g.x2)" [style.top.px]="min(g.y1, g.y2)"
               [style.width.px]="max(1, abs(g.x2 - g.x1))" [style.height.px]="max(1, abs(g.y2 - g.y1))"></div>
        }
        @for (m of measures(); track $index) {
          <div class="ov ov-measure" [style.left.px]="min(m.x1, m.x2)" [style.top.px]="min(m.y1, m.y2)"
               [style.width.px]="max(1, abs(m.x2 - m.x1))" [style.height.px]="max(1, abs(m.y2 - m.y1))">
            <span>{{ m.label }}</span>
          </div>
        }
        @if (dragHint(); as h) { <div class="ov-hint" [style.left.px]="h.x" [style.top.px]="h.y">{{ h.text }}</div> }
        @if (marquee(); as r) {
          <div class="ov ov-marquee" [style.left.px]="r.x" [style.top.px]="r.y" [style.width.px]="r.w" [style.height.px]="r.h"></div>
        }
        @if (drawRect(); as r) {
          <div class="ov ov-draw" [style.left.px]="r.x" [style.top.px]="r.y" [style.width.px]="r.w" [style.height.px]="r.h">
            <span class="ov-size">{{ round(r.w / store.zoom()) }} × {{ round(r.h / store.zoom()) }}</span>
          </div>
        }
      </div>
    </div>
    <rea-rulers [viewport]="viewport" [artboard]="artboard" />
  `,
})
export class EditorCanvas implements OnDestroy {
  readonly store = inject(EditorStore);
  private guidePrefs = inject(GuidePrefs);
  private viewport = viewChild.required<ElementRef<HTMLElement>>('viewport');
  private stage = viewChild.required<ElementRef<HTMLElement>>('stage');
  private artboard = viewChild.required<ElementRef<HTMLElement>>('artboard');

  readonly handles: Handle[] = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w'];
  readonly bp = computed(() => BREAKPOINTS.find(b => b.id === this.store.bp())!);
  readonly selRect = signal<Rect | null>(null);
  readonly hoverRect = signal<Rect | null>(null);
  readonly multiRects = signal<Rect[]>([]);
  readonly dropTargetRect = signal<Rect | null>(null);
  readonly guides = signal<Line[]>([]);
  readonly measures = signal<Line[]>([]);
  readonly marquee = signal<Rect | null>(null);
  readonly drawRect = signal<Rect | null>(null);
  readonly panning = signal(false);
  readonly spacing = signal<SpaceZone[]>([]);
  readonly dragHint = signal<{ text: string; x: number; y: number } | null>(null);
  /** Tablet/phone view: the outermost layers that stick out past the screen edge (2.1.1). */
  readonly overflow = signal<{ id: string; r: Rect }[]>([]);
  private overflowKey: unknown[] = [];
  readonly handMode = computed(() => this.store.tool() === 'hand' || this.store.spaceDown());
  readonly drawTool = computed(() => ['frame', 'rect', 'ellipse', 'text'].includes(this.store.tool()));

  readonly selIsInstance = computed(() => this.store.selected()?.node.type === 'instance');
  readonly hoverIsInstance = computed(() => {
    const id = this.store.hoverId();
    return !!id && findNode(this.store.doc(), id)?.node.type === 'instance';
  });
  readonly canResize = computed(() => {
    const s = this.store.selected();
    return this.store.selectedIds().length === 1 && !!s?.parent && s.node.type !== 'menu' && !s.node.locked;
  });
  readonly sizeLabel = computed(() => {
    const r = this.selRect();
    const n = this.store.selected()?.node;
    if (!r) return '';
    const z = this.store.zoom();
    if (this.store.selectedIds().length > 1 || !n) return `${Math.round(r.w / z)} × ${Math.round(r.h / z)}`;
    const e = this.store.eff(n);
    const tag = (m: string) => (m === 'fixed' ? '' : m === 'hug' ? ' Hug' : ' Fill');
    return `${Math.round(r.w / z)}${tag(e.width.mode)} × ${Math.round(r.h / z)}${tag(e.height.mode)}`;
  });

  readonly round = Math.round;
  readonly min = Math.min;
  readonly max = Math.max;
  readonly abs = Math.abs;

  private ro = new ResizeObserver(() => this.refreshOverlay());
  private cleanup: (() => void)[] = [];

  constructor() {
    this.store.nodePointerDown = (id, e, opts) => this.startDrag(id, e, opts);
    this.store.measure = id => this.designRect(id);
    afterNextRender(() => {
      this.ro.observe(this.artboard().nativeElement);
      const vp = this.viewport().nativeElement;
      const capture = (e: PointerEvent) => this.captureDown(e);
      const bubble = (e: PointerEvent) => this.backgroundDown(e);
      const wheel = (e: WheelEvent) => this.wheel(e);
      vp.addEventListener('pointerdown', capture, true);
      vp.addEventListener('pointerdown', bubble);
      vp.addEventListener('wheel', wheel, { passive: false });
      const over = (e: DragEvent) => this.assetOver(e);
      const drop = (e: DragEvent) => this.assetDrop(e);
      const leave = (e: DragEvent) => { if (!vp.contains(e.relatedTarget as Node)) this.clearAssetDrag(); };
      vp.addEventListener('dragover', over);
      vp.addEventListener('drop', drop);
      vp.addEventListener('dragleave', leave);
      this.cleanup.push(() => { vp.removeEventListener('dragover', over); vp.removeEventListener('drop', drop); vp.removeEventListener('dragleave', leave); });
      this.cleanup.push(() => vp.removeEventListener('pointerdown', capture, true));
      this.cleanup.push(() => vp.removeEventListener('pointerdown', bubble));
      this.cleanup.push(() => vp.removeEventListener('wheel', wheel));
    });
    afterEveryRender(() => this.refreshOverlay());
  }

  ngOnDestroy() {
    this.ro.disconnect();
    this.cleanup.forEach(f => f());
    this.store.nodePointerDown = undefined;
    this.store.measure = undefined;
  }

  // ------------------------------------------------------------------ view

  /** Shift+1: fit the artboard width to the window. */
  fit() {
    const avail = this.viewport().nativeElement.clientWidth - 96;
    this.store.zoom.set(clampZoom(Math.floor((avail / this.bp().width) * 100) / 100, 1));
  }

  /** Shift+2: zoom to the selection. */
  zoomToSelection() {
    const id = this.store.selectedId();
    const r = id && this.designRect(id);
    if (!r) return;
    const vp = this.viewport().nativeElement;
    const z = clampZoom(Math.min((vp.clientWidth - 120) / r.w, (vp.clientHeight - 120) / r.h), 4);
    this.store.zoom.set(Math.round(z * 100) / 100);
    requestAnimationFrame(() => this.scrollIntoCenter(id));
  }

  /** Zoom keeping the point under (clientX, clientY) fixed. */
  zoomAt(factor: number, clientX?: number, clientY?: number) {
    const vp = this.viewport().nativeElement;
    const art = this.artboard().nativeElement.getBoundingClientRect();
    const vr = vp.getBoundingClientRect();
    const cx = clientX ?? vr.left + vr.width / 2, cy = clientY ?? vr.top + vr.height / 2;
    const z0 = this.store.zoom();
    const ax = (cx - art.left) / z0, ay = (cy - art.top) / z0;
    // rounded to whole percents, but always at least one percent per step: at low zoom a small
    // wheel factor (e.g. 0.12 × 1.04) would otherwise round back to the same value and stick
    let r = Math.round(z0 * factor * 100) / 100;
    if (factor > 1 && r <= z0) r = Math.round(z0 * 100 + 1) / 100;
    if (factor < 1 && r >= z0) r = Math.round(z0 * 100 - 1) / 100;
    const z1 = clampZoom(r);
    this.store.zoom.set(z1);
    requestAnimationFrame(() => {
      const a2 = this.artboard().nativeElement.getBoundingClientRect();
      vp.scrollLeft += a2.left + ax * z1 - cx;
      vp.scrollTop += a2.top + ay * z1 - cy;
    });
  }

  private scrollIntoCenter(id: string) {
    const el = this.el(id);
    if (!el) return;
    const vp = this.viewport().nativeElement;
    const r = el.getBoundingClientRect(), v = vp.getBoundingClientRect();
    vp.scrollLeft += r.left + r.width / 2 - (v.left + v.width / 2);
    vp.scrollTop += r.top + r.height / 2 - (v.top + v.height / 2);
  }

  private wheel(e: WheelEvent) {
    if (!e.ctrlKey && !e.metaKey) return; // plain wheel scrolls (pans) natively
    e.preventDefault();
    this.zoomAt(Math.exp(-e.deltaY * 0.01), e.clientX, e.clientY);
  }

  // ------------------------------------------------------------------ geometry helpers

  private el(id: string): HTMLElement | null {
    return this.artboard().nativeElement.querySelector(`[data-node-id="${CSS.escape(id)}"]`);
  }

  private stageOrigin() {
    const s = this.stage().nativeElement.getBoundingClientRect();
    return { x: s.left, y: s.top };
  }

  private toStage(r: DOMRect | Rect): Rect {
    const o = this.stageOrigin();
    const x = 'left' in r ? r.left : r.x, y = 'top' in r ? r.top : r.y;
    const w = 'width' in r ? r.width : r.w, h = 'height' in r ? r.height : r.h;
    return { x: x - o.x, y: y - o.y, w, h };
  }

  private screenRect(id: string): Rect | null {
    const el = this.el(id);
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: r.left, y: r.top, w: r.width, h: r.height };
  }

  /** Rect in design px relative to the artboard (zoom-independent). */
  private designRect(id: string): Rect | null {
    const r = this.screenRect(id);
    if (!r) return null;
    const a = this.artboard().nativeElement.getBoundingClientRect();
    const z = this.store.zoom();
    return { x: (r.x - a.left) / z, y: (r.y - a.top) / z, w: r.w / z, h: r.h / z };
  }

  /** Deepest frame under the pointer that isn't being moved. */
  private containerAt(x: number, y: number, exclude: string[] = []): string {
    const root = this.store.doc();
    for (const el of document.elementsFromPoint(x, y)) {
      const id = (el as HTMLElement).dataset?.['nodeId'];
      if (!id || !this.artboard().nativeElement.contains(el)) continue;
      if (exclude.some(ex => ex === id || isAncestor(root, ex, id))) continue;
      const n = findNode(root, id)?.node;
      if (n && isContainer(n) && n.type !== 'postList' && !n.locked) return id;
    }
    return root.id;
  }

  /** Insertion index among a container's flow children for the pointer. */
  private flowIndex(containerId: string, x: number, y: number, exclude: string[]): { index: number; line: Rect | null } {
    const c = findNode(this.store.doc(), containerId)?.node;
    const horizontal = c ? this.store.eff(c).layout?.mode === 'horizontal' : false;
    const kids = (c?.children ?? []).filter(k => !k.absolute);
    let index = c?.children?.length ?? 0;
    let line: Rect | null = null;
    const rects = kids.map(k => ({ k, r: this.screenRect(k.id) })).filter(o => o.r && !exclude.includes(o.k.id)) as { k: DNode; r: Rect }[];
    for (const { k, r } of rects) {
      if (horizontal ? x < r.x + r.w / 2 : y < r.y + r.h / 2) {
        index = c!.children!.indexOf(k);
        line = horizontal ? { x: r.x - 2, y: r.y, w: 3, h: r.h } : { x: r.x, y: r.y - 2, w: r.w, h: 3 };
        break;
      }
    }
    if (!line && rects.length) {
      const r = rects[rects.length - 1].r;
      line = horizontal ? { x: r.x + r.w, y: r.y, w: 3, h: r.h } : { x: r.x, y: r.y + r.h, w: r.w, h: 3 };
    }
    if (!line) {
      const cr = this.screenRect(containerId);
      if (cr) line = horizontal ? { x: cr.x + 4, y: cr.y + 4, w: 3, h: cr.h - 8 } : { x: cr.x + 4, y: cr.y + 4, w: cr.w - 8, h: 3 };
    }
    return { index, line: line ? this.toStage(line) : null };
  }

  // ------------------------------------------------------------------ overlay refresh

  private refreshOverlay() {
    const ids = this.store.selectedIds();
    const hov = this.store.hoverId();
    const rects = ids.map(id => this.screenRect(id)).filter((r): r is Rect => !!r);
    if (rects.length) {
      const x0 = Math.min(...rects.map(r => r.x)), y0 = Math.min(...rects.map(r => r.y));
      const x1 = Math.max(...rects.map(r => r.x + r.w)), y1 = Math.max(...rects.map(r => r.y + r.h));
      setRect(this.selRect, this.toStage({ x: x0, y: y0, w: x1 - x0, h: y1 - y0 }));
    } else setRect(this.selRect, null);
    const multi = rects.length > 1 ? rects.map(r => this.toStage(r)) : [];
    if (JSON.stringify(multi) !== JSON.stringify(this.multiRects())) this.multiRects.set(multi);
    const hr = hov && !ids.includes(hov) ? this.screenRect(hov) : null;
    setRect(this.hoverRect, hr ? this.toStage(hr) : null);
    const dt = this.store.dropTarget();
    const dr = dt ? this.screenRect(dt) : null;
    setRect(this.dropTargetRect, dr ? this.toStage(dr) : null);
    this.updateMeasures();
    this.updateSpacing();
    this.updateOverflow();
  }

  /** Layers wider than the tablet/phone screen, or placed past its edge (a free-placed frame, a fixed
   *  width): outlined in red, with a count above the artboard. Checked when the page, the breakpoint or
   *  the zoom changes. */
  private updateOverflow() {
    const bp = this.store.bp();
    const key = [this.store.doc(), bp, this.store.zoom(), this.artboard().nativeElement.offsetHeight];
    if (key.every((k, i) => k === this.overflowKey[i])) return;
    if (document.body.classList.contains('rea-dragging')) return; // checked again after the drop
    this.overflowKey = key;
    const out: { id: string; r: Rect }[] = [];
    if (bp !== 'desktop') {
      const art = this.artboard().nativeElement;
      const a = art.getBoundingClientRect();
      const flagged: HTMLElement[] = [];
      art.querySelectorAll<HTMLElement>('[data-node-id]').forEach(el => {
        if (flagged.some(f => f.contains(el))) return;
        const r = el.getBoundingClientRect();
        if (!r.width || !r.height) return;
        if (r.right > a.right + 1 || r.left < a.left - 1) {
          flagged.push(el);
          out.push({ id: el.dataset['nodeId']!, r: this.toStage(r) });
        }
      });
    }
    if (JSON.stringify(out) !== JSON.stringify(this.overflow())) this.overflow.set(out);
  }

  selectOverflow() {
    const ids = this.overflow().map(o => o.id);
    if (!ids.length) return;
    this.store.select(ids[0]);
    if (ids.length > 1) this.store.notify(tr('ed.overflow.more', { n: ids.length - 1 }), true);
    requestAnimationFrame(() => this.scrollIntoCenter(ids[0]));
  }

  /** Figma auto-layout spacing handles: padding bands and gaps of the selected frame. */
  private updateSpacing() {
    let out: SpaceZone[] = [];
    const s = this.store.selected();
    const busy = document.body.classList.contains('rea-dragging') || this.store.editingTextId();
    if (!busy && s && this.store.selectedIds().length === 1 && (s.node.type === 'frame' || s.node.type === 'postList')) {
      const e = this.store.eff(s.node);
      const mode = e.layout?.mode;
      const F = this.screenRect(s.node.id);
      if (F && (mode === 'horizontal' || mode === 'vertical')) {
        const z = this.store.zoom();
        const p = (e.layout?.padding ?? [0, 0, 0, 0]).map(v => v * z);
        const o = this.stageOrigin();
        const at = (x: number, y: number, w: number, h: number) => ({ x: x - o.x, y: y - o.y, w, h });
        const pads = [
          at(F.x, F.y, F.w, p[0]), at(F.x + F.w - p[1], F.y, p[1], F.h),
          at(F.x, F.y + F.h - p[2], F.w, p[2]), at(F.x, F.y, p[3], F.h),
        ];
        pads.forEach((r, side) => { if (r.w > 2 && r.h > 2) out.push({ kind: 'pad', side, ...r, value: e.layout!.padding![side], horizontal: side === 1 || side === 3 }); });
        const horizontal = mode === 'horizontal';
        const kids = (s.node.children ?? []).filter(k => !k.absolute && this.store.eff(k).visible !== false)
          .map(k => this.screenRect(k.id)).filter((r): r is Rect => !!r);
        for (let i = 0; i + 1 < kids.length; i++) {
          const a = kids[i], b = kids[i + 1];
          if (horizontal) {
            const w = b.x - (a.x + a.w);
            if (w > 0) out.push({ kind: 'gap', side: -1, ...at(a.x + a.w, F.y + p[0], w, F.h - p[0] - p[2]), value: e.layout?.gap ?? 0, horizontal: true });
          } else {
            const h = b.y - (a.y + a.h);
            if (h > 0) out.push({ kind: 'gap', side: -1, ...at(F.x + p[3], a.y + a.h, F.w - p[1] - p[3], h), value: e.layout?.gap ?? 0, horizontal: false });
          }
        }
      }
    }
    out = out.map(r => ({ ...r, x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.w), h: Math.round(r.h) }));
    if (JSON.stringify(out) !== JSON.stringify(this.spacing())) this.spacing.set(out);
  }

  /** Drag a padding band or gap to change it (Alt = opposite side too, Shift = all sides). */
  spaceDrag(e: PointerEvent, zone: SpaceZone) {
    if (e.button !== 0) return;
    e.stopPropagation();
    e.preventDefault();
    const store = this.store;
    const id = store.selectedId();
    const n = id && findNode(store.doc(), id)?.node;
    if (!id || !n) return;
    const lay = store.eff(n).layout ?? {};
    const pad0 = [...(lay.padding ?? [0, 0, 0, 0])] as [number, number, number, number];
    const gap0 = lay.gap ?? 0;
    const sx = e.clientX, sy = e.clientY, z = store.zoom();
    store.beginGesture();
    document.body.classList.add('rea-dragging');
    this.track(ev => {
      const dx = (ev.clientX - sx) / z, dy = (ev.clientY - sy) / z;
      if (zone.kind === 'gap') {
        store.setLayout(id, { gap: Math.max(0, Math.round(gap0 + (zone.horizontal ? dx : dy))), justify: lay.justify === 'between' ? undefined : lay.justify });
        this.hint('Gap ' + Math.max(0, Math.round(gap0 + (zone.horizontal ? dx : dy))), ev);
        return;
      }
      const delta = [dy, -dx, -dy, dx][zone.side];
      const v = Math.max(0, Math.round(pad0[zone.side] + delta));
      const pad = [...pad0] as [number, number, number, number];
      if (ev.shiftKey) pad.fill(v);
      else { pad[zone.side] = v; if (ev.altKey) pad[(zone.side + 2) % 4] = v; }
      store.setLayout(id, { padding: pad });
      this.hint('Padding ' + v + (ev.shiftKey ? tr('ed.hint.padAll') : ev.altKey ? tr('ed.hint.padBoth') : ''), ev);
    }, () => { this.hint(null); store.endGesture(); });
  }

  /** Alt held: red distance lines between the selection and the hovered layer. */
  private updateMeasures() {
    const sel = this.store.selectedId(), hov = this.store.hoverId();
    let out: Line[] = [];
    if (this.store.altDown() && sel && hov && sel !== hov) {
      const a = this.screenRect(sel), b = this.screenRect(hov);
      if (a && b) out = measureLines(a, b, this.store.zoom()).map(l => {
        const o = this.stageOrigin();
        return { ...l, x1: l.x1 - o.x, x2: l.x2 - o.x, y1: l.y1 - o.y, y2: l.y2 - o.y };
      });
    }
    if (JSON.stringify(out) !== JSON.stringify(this.measures())) this.measures.set(out);
  }

  // ------------------------------------------------------------------ pointer entry points

  /** Capture phase: hand/pan and drawing tools win over node selection. */
  private captureDown(e: PointerEvent) {
    if (e.button === 1 || (e.button === 0 && this.handMode())) {
      e.preventDefault();
      e.stopPropagation();
      this.pan(e);
      return;
    }
    if (e.button === 0 && this.drawTool()) {
      e.preventDefault();
      e.stopPropagation();
      this.draw(e);
    }
  }

  /** Bubble phase: only reached when the press wasn't on a layer. */
  private backgroundDown(e: PointerEvent) {
    if (e.button !== 0 || this.store.tool() !== 'move') return;
    if ((e.target as HTMLElement).closest('.hdl')) return;
    this.store.contextMenu.set(null);
    this.marqueeSelect(e);
  }

  bgMenu(e: MouseEvent) {
    e.preventDefault();
    if (!(e.target as HTMLElement).closest('[data-node-id]')) this.store.select(null);
    this.store.contextMenu.set({ x: e.clientX, y: e.clientY });
  }

  /**
   * Follows the pointer until it is released. A cancelled pointer (touch taken
   * over by the browser, pen out of range, window lost) ends the drag too — at
   * the last position seen — so the gesture and its undo step are always closed.
   */
  private track(move: (ev: PointerEvent) => void, up: (ev: PointerEvent) => void) {
    let last: PointerEvent | null = null;
    const m = (ev: PointerEvent) => { last = ev; move(ev); };
    const u = (ev: PointerEvent) => {
      window.removeEventListener('pointermove', m);
      window.removeEventListener('pointerup', u);
      window.removeEventListener('pointercancel', c);
      document.body.classList.remove('rea-dragging');
      up(ev);
    };
    const c = (ev: PointerEvent) => u(last ?? ev);
    window.addEventListener('pointermove', m);
    window.addEventListener('pointerup', u);
    window.addEventListener('pointercancel', c);
  }

  // ------------------------------------------------------------------ hand tool

  private pan(e: PointerEvent) {
    const vp = this.viewport().nativeElement;
    const sx = e.clientX, sy = e.clientY, sl = vp.scrollLeft, st = vp.scrollTop;
    this.panning.set(true);
    this.track(ev => {
      vp.scrollLeft = sl - (ev.clientX - sx);
      vp.scrollTop = st - (ev.clientY - sy);
    }, () => this.panning.set(false));
  }

  // ------------------------------------------------------------------ marquee

  /**
   * Box selection inside `contextId` (the artboard by default): selects the
   * frame's children the box touches; Ctrl/⌘ selects the deepest layers instead.
   * A press without dragging selects `clickSelect` (or clears the selection).
   */
  private marqueeSelect(e: PointerEvent, contextId?: string, clickSelect?: string) {
    const sx = e.clientX, sy = e.clientY;
    const additive = e.shiftKey;
    const before = this.store.selectedIds();
    const root = this.store.doc();
    const context = (contextId && findNode(root, contextId)?.node) || root;
    let moved = false;
    this.track(ev => {
      if (!moved && Math.abs(ev.clientX - sx) + Math.abs(ev.clientY - sy) < 4) return;
      moved = true;
      const r = { x: Math.min(sx, ev.clientX), y: Math.min(sy, ev.clientY), w: Math.abs(ev.clientX - sx), h: Math.abs(ev.clientY - sy) };
      this.marquee.set(this.toStage(r));
      const candidates = ev.ctrlKey || ev.metaKey ? deepLayers(context) : (context.children ?? []);
      const hits = candidates.filter(c => {
        const cr = this.screenRect(c.id);
        return !!cr && !c.locked && cr.x < r.x + r.w && cr.x + cr.w > r.x && cr.y < r.y + r.h && cr.y + cr.h > r.y;
      }).map(c => c.id);
      this.store.selectMany(additive ? [...before, ...hits] : hits);
    }, () => {
      this.marquee.set(null);
      if (moved) return;
      if (clickSelect) this.store.select(clickSelect);
      else if (!additive) this.store.select(null);
    });
  }

  // ------------------------------------------------------------------ draw tools (F, R, O, T)

  private draw(e: PointerEvent) {
    const tool = this.store.tool();
    const containerId = this.containerAt(e.clientX, e.clientY);
    const sx = e.clientX, sy = e.clientY;
    const z = this.store.zoom();
    let last = { x: sx, y: sy };
    this.track(ev => {
      last = { x: ev.clientX, y: ev.clientY };
      if (tool === 'text') return;
      let w = ev.clientX - sx, h = ev.clientY - sy;
      if (ev.shiftKey) { const m = Math.max(Math.abs(w), Math.abs(h)); w = Math.sign(w || 1) * m; h = Math.sign(h || 1) * m; }
      this.drawRect.set(this.toStage({ x: Math.min(sx, sx + w), y: Math.min(sy, sy + h), w: Math.abs(w), h: Math.abs(h) }));
    }, () => {
      const dr = this.drawRect();
      this.drawRect.set(null);
      const cRect = this.screenRect(containerId) ?? { x: sx, y: sy, w: 0, h: 0 };
      const dragged = dr && dr.w > 4 && dr.h > 4;
      const o = this.stageOrigin();
      const box = dragged
        ? { x: (dr!.x + o.x - cRect.x) / z, y: (dr!.y + o.y - cRect.y) / z, w: dr!.w / z, h: dr!.h / z }
        : { x: (sx - cRect.x) / z, y: (sy - cRect.y) / z, w: 100, h: 100 };
      const node = makeDrawn(tool, box);
      const container = findNode(this.store.doc(), containerId)!.node;
      const free = (this.store.eff(container).layout?.mode ?? 'none') === 'none';
      if (free) {
        node.x = Math.round(box.x);
        node.y = Math.round(box.y);
        this.store.insertAt(node, containerId, undefined, true);
      } else {
        const { index } = this.flowIndex(containerId, last.x, last.y, []);
        this.store.insertAt(node, containerId, index, true);
      }
      if (tool === 'text') this.store.editingTextId.set(node.id);
      this.store.tool.set('move');
    });
  }

  // ------------------------------------------------------------------ move / reorder / reparent

  private startDrag(id: string, e: PointerEvent, opts: { clickSelect?: string; marqueeIn?: string } = {}) {
    const store = this.store;
    const info = findNode(store.doc(), id);
    if (!info || store.editingTextId()) return;
    if (!info.parent) { this.marqueeSelect(e); return; } // pressing on the artboard background
    if (opts.marqueeIn) { this.marqueeSelect(e, opts.marqueeIn, opts.marqueeIn); return; } // empty area of a frame
    let ids = store.selectedIds().includes(id) ? store.siblingSelection() : [id];
    if (!ids.length) ids = [id];
    if (ids.some(x => findNode(store.doc(), x)?.node.locked)) return;
    const z = store.zoom();
    const sx = e.clientX, sy = e.clientY;
    const free = store.parentMode(id) === 'none';
    let started = false;
    let starts = new Map<string, { x: number; y: number }>();
    let movingEls: HTMLElement[] = [];
    let target: { parent: string; index?: number; pos?: { x: number; y: number } } | null = null;
    let grab = { x: 0, y: 0 };
    /** Each dragged layer's offset from the first one (design px), kept when dropped freely. */
    let offsets = new Map<string, { x: number; y: number }>();
    let snapCtx: SnapCtx | null = null;

    const begin = (ev: PointerEvent) => {
      started = true;
      document.body.classList.add('rea-dragging');
      store.beginGesture();
      if (ev.altKey) ids = store.duplicateMany(ids); // Alt+drag duplicates (Figma)
      starts = new Map(ids.map(x => {
        const n = findNode(store.doc(), x)!.node;
        const ef = store.eff(n);
        return [x, { x: ef.x ?? 0, y: ef.y ?? 0 }];
      }));
      const r = this.screenRect(ids[0]);
      grab = r ? { x: (sx - r.x) / z, y: (sy - r.y) / z } : { x: 0, y: 0 };
      offsets = new Map(ids.map(x => {
        const rx = this.screenRect(x);
        return [x, r && rx ? { x: Math.round((rx.x - r.x) / z), y: Math.round((rx.y - r.y) / z) } : { x: 0, y: 0 }];
      }));
      movingEls = ids.map(x => this.el(x)).filter((x): x is HTMLElement => !!x);
      snapCtx = free ? this.snapContext(ids) : null;
    };

    this.track(ev => {
      let dx = (ev.clientX - sx) / z, dy = (ev.clientY - sy) / z;
      if (!started) {
        if (Math.abs(dx) + Math.abs(dy) < 3 / z) return;
        begin(ev);
      }
      if (ev.shiftKey) { if (Math.abs(dx) > Math.abs(dy)) dy = 0; else dx = 0; }
      const over = this.containerAt(ev.clientX, ev.clientY, ids);
      const parentId = findNode(store.doc(), ids[0])?.parent?.id;

      // free move (default) drops exactly where released, even inside auto layout; Ctrl flips the mode.
      // Page sections (children of the artboard) always reorder so the page stays a clean stack.
      // Only full-width page sections (header, hero, footer…) keep the page a clean stack;
      // smaller items placed on the page (buttons, text, images) move freely like everything else.
      const sectionLevel = parentId === store.doc().id && ids.every(x => this.isSection(x));
      const freePlace = !sectionLevel && store.freeDrag() !== (ev.ctrlKey || ev.metaKey);
      const dropAt = (overId: string) => {
        const c = findNode(store.doc(), overId)!.node;
        const cFree = (store.eff(c).layout?.mode ?? 'none') === 'none';
        if (cFree || freePlace) {
          const cr = this.screenRect(overId)!;
          store.dropIndicator.set(null);
          return { parent: overId, pos: { x: Math.round((ev.clientX - cr.x) / z - grab.x), y: Math.round((ev.clientY - cr.y) / z - grab.y) } };
        }
        const { index, line } = this.flowIndex(overId, ev.clientX, ev.clientY, ids);
        store.dropIndicator.set(line);
        return { parent: overId, index };
      };

      if (free) {
        // freeform: move live with smart guides (Ctrl/⌘ disables snapping)
        const snap = this.snap(snapCtx, dx, dy, !(ev.ctrlKey || ev.metaKey));
        dx = snap.dx; dy = snap.dy;
        this.guides.set(snap.lines);
        for (const x of ids) {
          const s = starts.get(x)!;
          store.setPos(x, Math.round(s.x + dx), Math.round(s.y + dy));
        }
        // hovering another frame: drop into it on release (Figma reparenting)
        // don't pop out into an enclosing auto-layout frame just because the layer overflows its parent
        const upIntoFlow = !!parentId && isAncestor(store.doc(), over, parentId) && !this.isFree(over);
        const leaving = over !== parentId && !upIntoFlow;
        store.dropTarget.set(leaving ? over : null);
        target = leaving ? dropAt(over) : null;
        if (!leaving) store.dropIndicator.set(null);
        this.hint(leaving && !freePlace && !this.isFree(over) ? this.reorderHint() : null, ev);
        return;
      }

      // auto layout: drag a ghost and show the insertion line (same or other frame)
      this.guides.set([]);
      movingEls.forEach(el => { el.style.translate = `${dx}px ${dy}px`; el.style.opacity = '0.6'; el.style.zIndex = '50'; });
      store.dropTarget.set(over !== parentId ? over : null);
      target = dropAt(over);
      this.hint(freePlace ? this.freeHint() : this.isFree(over) ? null : sectionLevel ? tr(HINT_SECTION) : this.reorderHint(), ev);
    }, () => {
      this.hint(null);
      movingEls.forEach(el => { el.style.translate = ''; el.style.opacity = ''; el.style.zIndex = ''; });
      this.guides.set([]);
      store.dropIndicator.set(null);
      store.dropTarget.set(null);
      if (!started) {
        if (opts.clickSelect) store.select(opts.clickSelect);
        return;
      }
      // the drag and the drop (reparent/reorder) are one undo step: close the gesture after moving
      if (target && ids.length === 1) {
        const p = findNode(store.doc(), target.parent)!.node;
        store.move(ids[0], target.parent, target.index ?? p.children?.length ?? 0, target.pos);
      } else if (target && ids.length > 1 && target.pos) {
        // several layers dropped freely: keep their arrangement
        const p = findNode(store.doc(), target.parent)!.node;
        for (const x of ids) {
          const o = offsets.get(x) ?? { x: 0, y: 0 };
          store.move(x, target.parent, p.children?.length ?? 0, { x: target.pos.x + o.x, y: target.pos.y + o.y });
        }
      } else if (target && ids.length > 1 && target.index !== undefined) {
        [...ids].reverse().forEach(x => store.move(x, target!.parent, target!.index!));
      }
      store.endGesture();
    });
  }

  /** Smart-guide targets, captured once when a freeform drag starts. */
  private snapContext(ids: string[]): SnapCtx | null {
    const store = this.store;
    const parent = findNode(store.doc(), ids[0])?.parent;
    const pr = parent && this.screenRect(parent.id);
    if (!parent || !pr) return null;
    const z = store.zoom();
    const rel = (r: Rect): Rect => ({ x: (r.x - pr.x) / z, y: (r.y - pr.y) / z, w: r.w / z, h: r.h / z });
    const moving = ids.map(x => this.screenRect(x)).filter((r): r is Rect => !!r).map(rel);
    if (!moving.length) return null;
    const x0 = Math.min(...moving.map(r => r.x)), y0 = Math.min(...moving.map(r => r.y));
    const box = { x: x0, y: y0, w: Math.max(...moving.map(r => r.x + r.w)) - x0, h: Math.max(...moving.map(r => r.y + r.h)) - y0 };
    const pw = pr.w / z, ph = pr.h / z;
    const xs = [0, pw / 2, pw], ys = [0, ph / 2, ph];
    for (const c of parent.children ?? []) {
      if (ids.includes(c.id)) continue;
      const r = this.screenRect(c.id);
      if (!r) continue;
      const q = rel(r);
      xs.push(q.x, q.x + q.w / 2, q.x + q.w);
      ys.push(q.y, q.y + q.h / 2, q.y + q.h);
    }
    const extra = this.extraSnap(parent, pr); // ruler guides + layout-grid columns
    xs.push(...extra.x);
    ys.push(...extra.y);
    return { box, xs, ys, pr };
  }

  /** Ruler guides and the parent's layout-grid edges, in the parent's design px. */
  private extraSnap(parent: DNode, pr: Rect | null) {
    const a = this.artboard().nativeElement.getBoundingClientRect();
    return pr ? guideSnapTargets(this.store, this.guidePrefs, parent, pr, { x: a.left, y: a.top, w: a.width, h: a.height }, this.store.zoom()) : { x: [], y: [] };
  }

  /** Smart guides: snap the moved box to sibling/parent edges and centers. */
  private snap(ctx: SnapCtx | null, dx: number, dy: number, enabled: boolean): { dx: number; dy: number; lines: Line[] } {
    if (!ctx || !enabled) return { dx, dy, lines: [] };
    const z = this.store.zoom();
    const th = SNAP_PX / z;
    const best = (vals: number[], start: number, size: number, d: number) => {
      let out: { d: number; v: number } | null = null;
      for (const edge of [0, size / 2, size]) for (const v of vals) {
        const diff = v - (start + d + edge);
        if (Math.abs(diff) < th && (!out || Math.abs(diff) < Math.abs(out.d))) out = { d: diff, v };
      }
      return out;
    };
    const bx = best(ctx.xs, ctx.box.x, ctx.box.w, dx);
    const by = best(ctx.ys, ctx.box.y, ctx.box.h, dy);
    const o = this.stageOrigin();
    const lines: Line[] = [];
    if (bx) {
      dx += bx.d;
      const x = ctx.pr.x + bx.v * z - o.x;
      lines.push({ x1: x, y1: ctx.pr.y - o.y, x2: x, y2: ctx.pr.y + ctx.pr.h - o.y });
    }
    if (by) {
      dy += by.d;
      const y = ctx.pr.y + by.v * z - o.y;
      lines.push({ x1: ctx.pr.x - o.x, y1: y, x2: ctx.pr.x + ctx.pr.w - o.x, y2: y });
    }
    return { dx, dy, lines };
  }

  // ------------------------------------------------------------------ drag & drop from the Assets panel

  private assetOver(e: DragEvent) {
    if (!this.store.assetDrag()) return;
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
    const over = this.containerAt(e.clientX, e.clientY);
    const free = this.assetFree(over, e);
    this.store.dropTarget.set(over === this.store.doc().id ? null : over);
    this.store.dropIndicator.set(free ? null : this.flowIndex(over, e.clientX, e.clientY, []).line);
    this.hint(free ? tr('ed.hint.dropHere') : tr('ed.hint.dropFlow') + (this.store.assetDrag()?.section ? '' : tr('ed.hint.ctrlSwitch')), e);
  }

  private assetDrop(e: DragEvent) {
    const drag = this.store.assetDrag();
    if (!drag) return;
    e.preventDefault();
    const over = this.containerAt(e.clientX, e.clientY);
    const free = this.isFree(over);
    const node = drag.make();
    if (this.assetFree(over, e)) {
      const cr = this.screenRect(over)!;
      const z = this.store.zoom();
      node.x = Math.round((e.clientX - cr.x) / z);
      node.y = Math.round((e.clientY - cr.y) / z);
      if (!free) node.absolute = true;
      if (node.width.mode === 'fill') node.width = { mode: 'fixed', value: Math.min(480, Math.round(cr.w / z)) };
      if (node.height.mode === 'fill') node.height = { mode: 'fixed', value: 200 };
      // keep it where it was dropped relative to the centre / edge on wider and narrower screens
      const target = findNode(this.store.doc(), over)?.node;
      const anchor = target && this.store.autoAnchor(target, cr.w / z, node.x, node.width.mode === 'fixed' ? node.width.value ?? 0 : 0);
      if (anchor) node.style = { ...node.style, ...anchor };
      this.store.insertAt(node, over, undefined, true);
    } else {
      this.store.insertAt(node, over, this.flowIndex(over, e.clientX, e.clientY, []).index);
    }
    this.clearAssetDrag();
  }

  /** Assets land exactly where dropped (free move), except whole sections and drops on the page itself. */
  private assetFree(over: string, e: DragEvent) {
    if (this.isFree(over)) return true;
    const d = this.store.assetDrag();
    if (!d || d.section || over === this.store.doc().id) return false;
    // into an auto-layout frame a new layer joins the flow, so it reflows on
    // tablets and phones instead of floating over the text; Ctrl places it freely
    return e.ctrlKey || e.metaKey;
  }

  private clearAssetDrag() {
    this.store.assetDrag.set(null);
    this.store.dropTarget.set(null);
    this.store.dropIndicator.set(null);
    this.hint(null);
  }

  private freeHint() { return tr(this.store.freeDrag() ? 'ed.hint.freeOn' : 'ed.hint.freeAbs'); }
  private reorderHint() { return tr(this.store.freeDrag() ? 'ed.hint.reorderCtrl' : 'ed.hint.reorder'); }

  /** A full-width, in-flow block directly on the page. */
  private isSection(id: string): boolean {
    const n = findNode(this.store.doc(), id)?.node;
    if (!n) return false;
    const e = this.store.eff(n);
    return !e.absolute && e.width?.mode === 'fill';
  }

  private isFree(id: string): boolean {
    const n = findNode(this.store.doc(), id)?.node;
    return !!n && (this.store.eff(n).layout?.mode ?? 'none') === 'none';
  }

  /** Small tip that follows the pointer while dragging. */
  private hint(text: string | null, ev?: PointerEvent | DragEvent) {
    if (!text || !ev) { if (this.dragHint()) this.dragHint.set(null); return; }
    const o = this.stageOrigin();
    this.dragHint.set({ text, x: ev.clientX - o.x + 14, y: ev.clientY - o.y + 18 });
  }

  // ------------------------------------------------------------------ resize (8 handles, Shift = ratio, Alt = from center)

  resize(e: PointerEvent, h: Handle) {
    e.stopPropagation();
    e.preventDefault();
    const store = this.store;
    const id = store.selectedId();
    const r0 = id && store.measure?.(id);
    const info = id && findNode(store.doc(), id);
    if (!id || !r0 || !info || !info.parent) return;
    const pr = store.measure?.(info.parent.id) ?? { x: 0, y: 0, w: 0, h: 0 };
    const free = store.parentMode(id) === 'none';
    const ef = store.eff(info.node);
    const x0 = free ? (ef.x ?? r0.x - pr.x) : 0, y0 = free ? (ef.y ?? r0.y - pr.y) : 0;
    const z = store.zoom();
    const sx = e.clientX, sy = e.clientY;
    const ratio = r0.w / Math.max(1, r0.h);
    const snapT = this.extraSnap(info.parent, this.screenRect(info.parent.id));
    // 2.3.0 (Qraft feedback 27): a click on a handle, or a twitch of a few pixels, changes nothing — before,
    // any pointer move turned a "hug" height into a fixed one at the size it had then
    let started = false;
    store.beginGesture();
    this.track(ev => {
      if (!started) {
        if (Math.hypot(ev.clientX - sx, ev.clientY - sy) < RESIZE_DEAD_PX) return;
        started = true;
      }
      let dx = (ev.clientX - sx) / z, dy = (ev.clientY - sy) / z;
      if (!(ev.ctrlKey || ev.metaKey)) { // the dragged edge snaps to guides / grid columns (Ctrl disables)
        if (/[ew]/.test(h)) dx += snapOffset(snapT.x, (h.includes('e') ? r0.x + r0.w : r0.x) - pr.x + dx, SNAP_PX / z);
        if (/[ns]/.test(h)) dy += snapOffset(snapT.y, (h.includes('s') ? r0.y + r0.h : r0.y) - pr.y + dy, SNAP_PX / z);
      }
      let w = r0.w, hh = r0.h;
      const k = ev.altKey ? 2 : 1;
      if (h.includes('e')) w = r0.w + dx * k;
      if (h.includes('w')) w = r0.w - dx * k;
      if (h.includes('s')) hh = r0.h + dy * k;
      if (h.includes('n')) hh = r0.h - dy * k;
      if (ev.shiftKey) {
        if (h === 'n' || h === 's') w = hh * ratio;
        else if (h === 'e' || h === 'w') hh = w / ratio;
        else if (Math.abs(w / r0.w) > Math.abs(hh / r0.h)) hh = w / ratio; else w = hh * ratio;
      }
      w = Math.max(1, Math.round(w));
      hh = Math.max(1, Math.round(hh));
      // position from the final size: the opposite edge (or the centre with Alt) stays put — also
      // when Shift changed the other axis, e.g. a Shift-drag on the n / w handle
      const { x, y } = resizedBox(h, x0, y0, r0.w, r0.h, w, hh, ev.altKey);
      const box: Partial<Rect> = {};
      if (h.includes('e') || h.includes('w') || ev.shiftKey) box.w = w;
      if (h.includes('n') || h.includes('s') || ev.shiftKey) box.h = hh;
      if (free) { box.x = Math.round(x); box.y = Math.round(y); }
      store.setBox(id, box);
    }, () => store.endGesture());
  }
}

// ------------------------------------------------------------------ helpers

/** Screen pixels a resize handle must be dragged before the layer's size changes. */
const RESIZE_DEAD_PX = 4;

/** Every layer inside `n` that has no children (texts, images, shapes, instances…). */
function deepLayers(n: DNode): DNode[] {
  const out: DNode[] = [];
  const walk = (x: DNode) => (x.children ?? []).forEach(c => (c.children?.length && c.type !== 'instance' ? walk(c) : out.push(c)));
  walk(n);
  return out;
}

function clampZoom(z: number, max = 8) {
  return Math.min(max, Math.max(0.05, z));
}

function setRect(sig: ReturnType<typeof signal<Rect | null>>, r: Rect | null) {
  const cur = sig();
  if (cur === r) return;
  if (cur && r && cur.x === r.x && cur.y === r.y && cur.w === r.w && cur.h === r.h) return;
  sig.set(r);
}

/** Node created by a draw tool, sized to the dragged box. */
function makeDrawn(tool: string, b: Rect): DNode {
  const w = Math.max(1, Math.round(b.w)), h = Math.max(1, Math.round(b.h));
  switch (tool) {
    case 'frame':
      return { id: newId(), type: 'frame', name: 'Frame', width: { mode: 'fixed', value: w }, height: { mode: 'fixed', value: h }, layout: { mode: 'none' }, style: { fill: '#ffffff' }, children: [] };
    case 'ellipse':
      return { id: newId(), type: 'rect', name: 'Ellipse', width: { mode: 'fixed', value: w }, height: { mode: 'fixed', value: h }, style: { fill: '#d9d9d9', radius: 9999 } };
    case 'text':
      return { id: newId(), type: 'text', name: 'Text', width: { mode: 'hug' }, height: { mode: 'hug' }, style: { fontSize: 16 }, props: { text: 'Text', tag: 'p' } };
    default:
      return { id: newId(), type: 'rect', name: 'Rectangle', width: { mode: 'fixed', value: w }, height: { mode: 'fixed', value: h }, style: { fill: '#d9d9d9' } };
  }
}

/** Figma Alt-measure: distances from rect a (selection) to b (hovered). */
function measureLines(a: Rect, b: Rect, z: number): Line[] {
  const lbl = (v: number) => String(Math.round(v / z));
  const lines: Line[] = [];
  const ay = a.y + a.h / 2, ax = a.x + a.w / 2;
  const inside = a.x >= b.x && a.y >= b.y && a.x + a.w <= b.x + b.w && a.y + a.h <= b.y + b.h;
  if (inside) {
    lines.push({ x1: b.x, y1: ay, x2: a.x, y2: ay, label: lbl(a.x - b.x) });
    lines.push({ x1: a.x + a.w, y1: ay, x2: b.x + b.w, y2: ay, label: lbl(b.x + b.w - a.x - a.w) });
    lines.push({ x1: ax, y1: b.y, x2: ax, y2: a.y, label: lbl(a.y - b.y) });
    lines.push({ x1: ax, y1: a.y + a.h, x2: ax, y2: b.y + b.h, label: lbl(b.y + b.h - a.y - a.h) });
    return lines.filter(l => l.label !== '0');
  }
  if (b.x >= a.x + a.w) lines.push({ x1: a.x + a.w, y1: ay, x2: b.x, y2: ay, label: lbl(b.x - a.x - a.w) });
  else if (b.x + b.w <= a.x) lines.push({ x1: b.x + b.w, y1: ay, x2: a.x, y2: ay, label: lbl(a.x - b.x - b.w) });
  if (b.y >= a.y + a.h) lines.push({ x1: ax, y1: a.y + a.h, x2: ax, y2: b.y, label: lbl(b.y - a.y - a.h) });
  else if (b.y + b.h <= a.y) lines.push({ x1: ax, y1: b.y + b.h, x2: ax, y2: a.y, label: lbl(a.y - b.y - b.h) });
  return lines;
}
