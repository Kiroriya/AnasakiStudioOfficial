import {
  ChangeDetectionStrategy, Component, ElementRef, OnDestroy, afterEveryRender, afterNextRender, computed, effect, inject, signal, untracked,
} from '@angular/core';
import { TPipe } from '../core/i18n';
import { findNode, isContainer } from './design-types';
import { EditorStore, Rect } from './editor-store';
import { Pt, Seg, SubPath, parsePath, serialize, viewBoxOf } from './vector';
import { refitVector, vectorFromPath } from './vector-ops';
import './messages-vector';

/** A pen anchor in artboard px, with optional bezier handles. */
interface Anchor { p: Pt; in?: Pt; out?: Pt }
/** A draggable point of the path being edited (anchor, or a segment's control point). */
interface EditPt { si: number; k: number; kind: 'a' | 'c1' | 'c2'; seg: number; x: number; y: number; ax?: number; ay?: number }

const CLOSE_PX = 8;

/**
 * Pen tool (P) and point editing of vector layers, drawn over the canvas.
 * Lives inside the canvas stage; listens on the viewport in the capture phase
 * so it wins over layer selection while the pen is active.
 */
@Component({
  selector: 'rea-vector-overlay',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'vo-host' },
  template: `
    @if (penPath() || editPts().length) {
      <svg class="vo-svg" width="1" height="1">
        @if (penPath(); as d) {
          <path class="vo-path" [attr.d]="d" />
          @if (rubber(); as r) { <path class="vo-rubber" [attr.d]="r" /> }
          @for (h of penHandles(); track $index) {
            <line class="vo-hline" [attr.x1]="h.x1" [attr.y1]="h.y1" [attr.x2]="h.x2" [attr.y2]="h.y2" />
            <circle class="vo-hdot" [attr.cx]="h.x2" [attr.cy]="h.y2" r="3.5" />
          }
          @for (a of penPts(); track $index; let i = $index) {
            <rect class="vo-pt" [class.first]="i === 0" [class.closing]="i === 0 && closeHover()" [attr.x]="a.x - 4" [attr.y]="a.y - 4" width="8" height="8" />
          }
        }
        @if (editPts().length) {
          @for (h of editPts(); track $index) {
            @if (h.kind !== 'a') { <line class="vo-hline" [attr.x1]="h.ax" [attr.y1]="h.ay" [attr.x2]="h.x" [attr.y2]="h.y" /> }
          }
          @for (h of editPts(); track $index) {
            @if (h.kind === 'a') {
              <rect class="vo-pt vo-h" [attr.x]="h.x - 4.5" [attr.y]="h.y - 4.5" width="9" height="9" (pointerdown)="dragPoint($event, h)" />
            } @else {
              <circle class="vo-hdot vo-h" [attr.cx]="h.x" [attr.cy]="h.y" r="4" (pointerdown)="dragPoint($event, h)" />
            }
          }
        }
      </svg>
    }
    @if (store.tool() === 'pen') { <div class="vo-hint">🖋 {{ 'vec.penHint' | t }}</div> }
    @else if (editId()) { <div class="vo-hint">✎ {{ 'vec.editHint' | t }} <button (click)="leaveEdit()">{{ 'vec.done' | t }}</button></div> }
  `,
  styles: [`
    :host { position: absolute; left: 0; top: 0; width: 0; height: 0; overflow: visible; z-index: 40; pointer-events: none; }
    .vo-svg { position: absolute; left: 0; top: 0; overflow: visible; pointer-events: none; }
    .vo-path { fill: none; stroke: #0d99ff; stroke-width: 1.5; }
    .vo-rubber { fill: none; stroke: #0d99ff; stroke-width: 1; stroke-dasharray: 4 3; }
    .vo-hline { stroke: #0d99ff; stroke-width: 1; }
    .vo-hdot { fill: #fff; stroke: #0d99ff; stroke-width: 1.5; }
    .vo-pt { fill: #fff; stroke: #0d99ff; stroke-width: 1.5; }
    .vo-pt.first { fill: #e5f4ff; }
    .vo-pt.closing { fill: #0d99ff; }
    .vo-h { pointer-events: all; cursor: move; }
    .vo-hint { position: fixed; left: 50%; bottom: 84px; transform: translateX(-50%); background: #1e1e1e; color: #fff; font-size: 12px;
      padding: 6px 12px; border-radius: 8px; box-shadow: 0 4px 16px rgba(0,0,0,.2); white-space: nowrap; pointer-events: auto; z-index: 60; }
    .vo-hint button { margin-left: 8px; background: #0d99ff; color: #fff; border: 0; border-radius: 4px; padding: 2px 8px; cursor: pointer; font: inherit; }
  `],
})
export class VectorOverlay implements OnDestroy {
  readonly store = inject(EditorStore);
  private host = inject<ElementRef<HTMLElement>>(ElementRef);

  /** Artboard top-left in stage px. */
  private readonly origin = signal<Pt>({ x: 0, y: 0 });
  /** Box of the layer being edited (artboard px). */
  private readonly editRect = signal<Rect | null>(null);
  readonly anchors = signal<Anchor[]>([]);
  readonly cursor = signal<Pt | null>(null);
  readonly closeHover = signal(false);
  readonly editId = this.store.vectorEditId;
  private containerId: string | null = null;
  private cleanup: (() => void)[] = [];

  private toStage = (p: Pt): Pt => {
    const o = this.origin(), z = this.store.zoom();
    return { x: o.x + p.x * z, y: o.y + p.y * z };
  };

  readonly penPts = computed(() => this.anchors().map(a => this.toStage(a.p)));
  readonly penPath = computed(() => {
    const a = this.anchors();
    if (!a.length) return null;
    const sub = anchorsToPath(a, false);
    const s = this.toStage(sub.start);
    let d = `M${s.x} ${s.y}`;
    for (const g of sub.segs) d += segD(g, this.toStage);
    return d;
  });
  readonly rubber = computed(() => {
    const a = this.anchors(), c = this.cursor();
    if (!a.length || !c) return null;
    const last = a[a.length - 1];
    const p0 = this.toStage(last.p), c1 = this.toStage(last.out ?? last.p), p1 = this.toStage(this.closeHover() ? a[0].p : c);
    return `M${p0.x} ${p0.y}C${c1.x} ${c1.y} ${p1.x} ${p1.y} ${p1.x} ${p1.y}`;
  });
  readonly penHandles = computed(() => {
    const out: { x1: number; y1: number; x2: number; y2: number }[] = [];
    for (const a of this.anchors()) {
      const p = this.toStage(a.p);
      for (const h of [a.in, a.out]) if (h) { const q = this.toStage(h); out.push({ x1: p.x, y1: p.y, x2: q.x, y2: q.y }); }
    }
    return out;
  });

  /** The edited path in viewBox units. */
  private readonly editSubs = computed(() => {
    const id = this.editId();
    const n = id ? findNode(this.store.doc(), id)?.node : null;
    return n?.type === 'vector' ? parsePath(n.props?.path ?? '') : [];
  });
  readonly editPts = computed<EditPt[]>(() => {
    const id = this.editId(), r = this.editRect();
    const n = id ? findNode(this.store.doc(), id)?.node : null;
    if (!n || !r || n.type !== 'vector') return [];
    const vb = viewBoxOf(n);
    const map = (p: Pt) => this.toStage({ x: r.x + ((p.x - vb.x) * r.w) / vb.w, y: r.y + ((p.y - vb.y) * r.h) / vb.h });
    const out: EditPt[] = [];
    this.editSubs().forEach((s, si) => {
      const anchors = [s.start, ...s.segs.map(g => g.p)];
      const skipLast = s.closed && s.segs.length > 0 && same(anchors[anchors.length - 1], s.start);
      anchors.forEach((p, k) => {
        if (skipLast && k === anchors.length - 1) return;
        const q = map(p);
        out.push({ si, k, kind: 'a', seg: -1, x: q.x, y: q.y });
      });
      s.segs.forEach((g, gi) => {
        if (g.t !== 'C') return;
        const from = map(gi === 0 ? s.start : s.segs[gi - 1].p), to = map(g.p);
        const c1 = map(g.c1), c2 = map(g.c2);
        if (!same(c1, from)) out.push({ si, k: gi, kind: 'c1', seg: gi, x: c1.x, y: c1.y, ax: from.x, ay: from.y });
        if (!same(c2, to)) out.push({ si, k: gi + 1, kind: 'c2', seg: gi, x: c2.x, y: c2.y, ax: to.x, ay: to.y });
      });
    });
    return out;
  });

  constructor() {
    afterNextRender(() => {
      const vp = this.viewport();
      if (!vp) return;
      const down = (e: PointerEvent) => this.down(e);
      const move = (e: PointerEvent) => this.move(e);
      const dbl = (e: MouseEvent) => this.dbl(e);
      const key = (e: KeyboardEvent) => this.key(e);
      vp.addEventListener('pointerdown', down, true);
      vp.addEventListener('pointermove', move);
      vp.addEventListener('dblclick', dbl, true);
      window.addEventListener('keydown', key, true);
      this.cleanup.push(() => {
        vp.removeEventListener('pointerdown', down, true);
        vp.removeEventListener('pointermove', move);
        vp.removeEventListener('dblclick', dbl, true);
        window.removeEventListener('keydown', key, true);
      });
    });
    afterEveryRender(() => this.sync());
    // crosshair while drawing; leaving the pen finishes the path
    effect(() => {
      const pen = this.store.tool() === 'pen';
      const vp = this.viewport();
      if (vp) vp.style.cursor = pen ? 'crosshair' : '';
      if (!pen && untracked(this.anchors).length) queueMicrotask(() => this.finish(false));
    });
    // selecting something else leaves point editing
    effect(() => {
      const id = this.editId();
      if (id && (this.store.selectedIds().length !== 1 || this.store.selectedId() !== id)) untracked(() => this.leaveEdit());
    });
  }

  ngOnDestroy() {
    this.cleanup.forEach(f => f());
    this.store.vectorEditId.set(null);
  }

  // ------------------------------------------------------------------ geometry

  private viewport(): HTMLElement | null { return this.host.nativeElement.closest('.viewport'); }
  private stage(): HTMLElement | null { return this.host.nativeElement.parentElement; }
  private artboard(): HTMLElement | null { return this.stage()?.querySelector('.artboard') ?? null; }

  /** Pointer → artboard px. */
  private toDesign(cx: number, cy: number): Pt {
    const a = this.artboard()!.getBoundingClientRect(), z = this.store.zoom();
    return { x: (cx - a.left) / z, y: (cy - a.top) / z };
  }

  private sync() {
    const a = this.artboard(), s = this.stage();
    if (!a || !s) return;
    const ar = a.getBoundingClientRect(), sr = s.getBoundingClientRect();
    const o = this.origin();
    if (o.x !== ar.left - sr.left || o.y !== ar.top - sr.top) this.origin.set({ x: ar.left - sr.left, y: ar.top - sr.top });
    const id = this.editId();
    const r = id ? this.store.measure?.(id) ?? null : null;
    const cur = this.editRect();
    if (!(r === cur || (r && cur && r.x === cur.x && r.y === cur.y && r.w === cur.w && r.h === cur.h))) this.editRect.set(r);
  }

  /** Deepest unlocked frame under the pointer (else the page). */
  private containerAt(cx: number, cy: number): string {
    const root = this.store.doc(), art = this.artboard();
    for (const el of document.elementsFromPoint(cx, cy)) {
      const id = (el as HTMLElement).dataset?.['nodeId'];
      if (!id || !art?.contains(el)) continue;
      const n = findNode(root, id)?.node;
      if (n && isContainer(n) && n.type !== 'postList' && !n.locked) return id;
    }
    return root.id;
  }

  // ------------------------------------------------------------------ pen

  private down(e: PointerEvent) {
    const s = this.store;
    if (e.button !== 0 || s.spaceDown()) return;
    if (s.tool() !== 'pen') {
      // point editing: a press outside the handles leaves it
      if (this.editId() && !(e.target as Element).closest('.vo-h, .vo-hint')) this.leaveEdit();
      return;
    }
    e.preventDefault();
    e.stopPropagation();
    this.sync();
    const list = this.anchors();
    if (list.length >= 2 && this.nearFirst(e.clientX, e.clientY)) { this.finish(true); return; }
    if (!list.length) this.containerId = this.containerAt(e.clientX, e.clientY);
    let p = this.toDesign(e.clientX, e.clientY);
    if (e.shiftKey && list.length) p = snap45(list[list.length - 1].p, p);
    const idx = list.length;
    this.anchors.set([...list, { p }]);
    const sx = e.clientX, sy = e.clientY;
    let dragged = false;
    const mv = (ev: PointerEvent) => {
      if (!dragged && Math.abs(ev.clientX - sx) + Math.abs(ev.clientY - sy) < 3) return;
      dragged = true;
      const q = this.toDesign(ev.clientX, ev.clientY);
      // click-drag pulls symmetric bezier handles
      this.anchors.update(a => a.map((x, i) => (i === idx ? { p: x.p, out: q, in: { x: 2 * x.p.x - q.x, y: 2 * x.p.y - q.y } } : x)));
      this.cursor.set(null);
    };
    const up = () => { window.removeEventListener('pointermove', mv); window.removeEventListener('pointerup', up); };
    window.addEventListener('pointermove', mv);
    window.addEventListener('pointerup', up);
  }

  private nearFirst(cx: number, cy: number): boolean {
    const a = this.anchors()[0];
    if (!a) return false;
    const p = this.toDesign(cx, cy), z = this.store.zoom();
    return Math.hypot(p.x - a.p.x, p.y - a.p.y) * z <= CLOSE_PX;
  }

  private move(e: PointerEvent) {
    if (this.store.tool() !== 'pen' || !this.anchors().length) return;
    let p = this.toDesign(e.clientX, e.clientY);
    const a = this.anchors();
    if (e.shiftKey) p = snap45(a[a.length - 1].p, p);
    this.cursor.set(p);
    this.closeHover.set(a.length >= 2 && this.nearFirst(e.clientX, e.clientY));
  }

  /** Creates the vector layer from the anchors (needs 2+). */
  finish(closed: boolean) {
    const a = this.anchors();
    this.anchors.set([]);
    this.cursor.set(null);
    this.closeHover.set(false);
    if (a.length < 2) return;
    const s = this.store;
    const cid = this.containerId && findNode(s.doc(), this.containerId) ? this.containerId : s.doc().id;
    const cr = s.measure?.(cid) ?? { x: 0, y: 0 };
    const node = vectorFromPath([anchorsToPath(a, closed)], cr, { style: { stroke: '#000000', strokeWidth: 2 } });
    if (!node) return;
    node.props!.strokeCap = 'round';
    node.props!.strokeJoin = 'round';
    const container = findNode(s.doc(), cid)!.node;
    if ((s.eff(container).layout?.mode ?? 'none') !== 'none') node.absolute = true;
    s.insertAt(node, cid, undefined, true);
    s.tool.set('move');
  }

  // ------------------------------------------------------------------ keys

  private key(e: KeyboardEvent) {
    const t = e.target as HTMLElement;
    if (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName)) return;
    const s = this.store;
    const stop = () => { e.preventDefault(); e.stopImmediatePropagation(); };
    if (s.tool() === 'pen' && this.anchors().length) {
      if (e.key === 'Enter' || e.key === 'Escape') { stop(); this.finish(false); }
      else if (e.key === 'Backspace' || e.key === 'Delete') { stop(); this.anchors.update(a => a.slice(0, -1)); }
      return;
    }
    if (this.editId()) {
      if (e.key === 'Escape' || e.key === 'Enter') { stop(); this.leaveEdit(); }
      else if (e.key === 'Backspace' || e.key === 'Delete') stop(); // don't delete the layer by accident
      return;
    }
    // Enter on a selected vector edits its points (Figma)
    if (e.key === 'Enter' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey && s.tool() === 'move' && this.editableSelection()) {
      stop();
      this.store.vectorEditId.set(s.selectedId());
    }
  }

  // ------------------------------------------------------------------ point editing

  private editableSelection(): boolean {
    const sel = this.store.selected();
    return this.store.selectedIds().length === 1 && sel?.node.type === 'vector' && !sel.node.locked;
  }

  private dbl(e: MouseEvent) {
    if (this.store.tool() !== 'move' || !this.editableSelection()) return;
    const id = this.store.selectedId()!;
    const hit = (e.target as Element).closest('[data-node-id]') as HTMLElement | null;
    if (hit?.dataset['nodeId'] !== id) return;
    e.preventDefault();
    e.stopPropagation();
    this.store.vectorEditId.set(id);
  }

  leaveEdit() { this.store.vectorEditId.set(null); }

  dragPoint(e: PointerEvent, h: EditPt) {
    e.preventDefault();
    e.stopPropagation();
    const s = this.store, id = this.editId(), r = this.editRect();
    const n = id ? findNode(s.doc(), id)?.node : null;
    if (!id || !n || !r) return;
    const vb = viewBoxOf(n), z = s.zoom();
    const kx = vb.w / (r.w * z), ky = vb.h / (r.h * z); // screen px → viewBox units
    const base = this.editSubs();
    const sx = e.clientX, sy = e.clientY;
    let started = false;
    const mv = (ev: PointerEvent) => {
      if (!started) { started = true; s.beginGesture(); document.body.classList.add('rea-dragging'); }
      const d = { x: (ev.clientX - sx) * kx, y: (ev.clientY - sy) * ky };
      s.setProps(id, { path: serialize(movePoint(base, h, d)) });
    };
    const up = () => {
      window.removeEventListener('pointermove', mv);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
      document.body.classList.remove('rea-dragging');
      if (!started) return;
      refitVector(s, id);
      s.endGesture();
    };
    window.addEventListener('pointermove', mv);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  }
}

// ------------------------------------------------------------------ helpers

const same = (a: Pt, b: Pt) => Math.abs(a.x - b.x) < 1e-6 && Math.abs(a.y - b.y) < 1e-6;

function segD(g: Seg, f: (p: Pt) => Pt): string {
  const p = f(g.p);
  if (g.t === 'L') return `L${p.x} ${p.y}`;
  const a = f(g.c1), b = f(g.c2);
  return `C${a.x} ${a.y} ${b.x} ${b.y} ${p.x} ${p.y}`;
}

/** Pen anchors → path (lines where neither end has a handle). */
function anchorsToPath(a: Anchor[], closed: boolean): SubPath {
  const seg = (from: Anchor, to: Anchor): Seg =>
    from.out || to.in ? { t: 'C', c1: from.out ?? from.p, c2: to.in ?? to.p, p: to.p } : { t: 'L', p: to.p };
  const segs: Seg[] = [];
  for (let i = 1; i < a.length; i++) segs.push(seg(a[i - 1], a[i]));
  if (closed) segs.push(seg(a[a.length - 1], a[0]));
  return { start: a[0].p, segs, closed };
}

/** Shift: constrain to 45° steps from the previous point. */
function snap45(from: Pt, p: Pt): Pt {
  const dx = p.x - from.x, dy = p.y - from.y, len = Math.hypot(dx, dy);
  const ang = Math.round(Math.atan2(dy, dx) / (Math.PI / 4)) * (Math.PI / 4);
  return { x: from.x + Math.cos(ang) * len, y: from.y + Math.sin(ang) * len };
}

/** Moves an anchor (with its handles) or one control point by d (viewBox units). */
function movePoint(base: SubPath[], h: EditPt, d: Pt): SubPath[] {
  const subs: SubPath[] = structuredClone(base);
  const s = subs[h.si];
  if (!s) return subs;
  const add = (p: Pt) => { p.x += d.x; p.y += d.y; };
  if (h.kind !== 'a') {
    const g = s.segs[h.seg];
    if (g?.t === 'C') add(g[h.kind]);
    return subs;
  }
  const n = s.segs.length;
  const moveAnchor = (k: number) => {
    if (k === 0) add(s.start); else add(s.segs[k - 1].p);
    const inSeg = k > 0 ? s.segs[k - 1] : null, outSeg = k < n ? s.segs[k] : null;
    if (inSeg?.t === 'C') add(inSeg.c2);
    if (outSeg?.t === 'C') add(outSeg.c1);
  };
  moveAnchor(h.k);
  // a closed path whose last point sits on the start: both move together
  if (h.k === 0 && s.closed && n && same(base[h.si].segs[n - 1].p, base[h.si].start)) moveAnchor(n);
  return subs;
}
