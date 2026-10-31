import { Injectable, signal } from '@angular/core';
import { DNode, LayoutGrid } from './design-types';
import type { EditorStore, Rect } from './editor-store';

/**
 * Rulers, guides and layout grids: per-user view switches plus the pure
 * geometry shared by the overlay (rulers.ts), the props panel
 * (layout-grid.ts) and snapping in editor-canvas.ts.
 */

export type GuideAxis = 'x' | 'y';
export interface Guides { x: number[]; y: number[] }

/** Per-user view switches (View menu), remembered in localStorage. */
@Injectable({ providedIn: 'root' })
export class GuidePrefs {
  readonly rulers = signal(read('anasaki.rulers', false));
  readonly guides = signal(read('anasaki.guides', true));
  readonly grids = signal(read('anasaki.layoutGrids', true));

  toggle(which: 'rulers' | 'guides' | 'grids') {
    const s = this[which];
    s.set(!s());
    const key = which === 'grids' ? 'anasaki.layoutGrids' : 'anasaki.' + which;
    try { localStorage.setItem(key, s() ? '1' : '0'); } catch { /* private mode etc. */ }
  }
}

function read(key: string, def: boolean): boolean {
  try {
    const v = localStorage.getItem(key);
    return v === null ? def : v === '1';
  } catch {
    return def;
  }
}

// ------------------------------------------------------------------ guides (stored on the root node)

export function docGuides(root: DNode): Guides {
  const g = root.props?.guides;
  return { x: [...(g?.x ?? [])], y: [...(g?.y ?? [])] };
}

/** Writes the guides through the store (one undo step); empty lists drop the prop. */
export function writeGuides(store: EditorStore, g: Guides) {
  store.mutate(r => {
    const props = { ...(r.props ?? {}) };
    if (g.x.length || g.y.length) props.guides = { x: g.x, y: g.y };
    else delete props.guides;
    r.props = props;
  });
}

export function clearGuides(store: EditorStore) {
  const g = store.doc().props?.guides;
  if (g && (g.x.length || g.y.length)) writeGuides(store, { x: [], y: [] });
}

export const hasGuides = (store: EditorStore) => {
  const g = store.doc().props?.guides;
  return !!g && (g.x.length > 0 || g.y.length > 0);
};

// ------------------------------------------------------------------ layout grids

export const GRID_COLOR = 'rgba(255,0,0,0.1)';

export const GRID_PRESETS: Record<'p12' | 'p4' | 'p8', LayoutGrid> = {
  p12: { type: 'columns', count: 12, gutter: 24, margin: 40, color: GRID_COLOR },
  p4: { type: 'columns', count: 4, gutter: 16, margin: 16, color: GRID_COLOR },
  p8: { type: 'grid', size: 8, color: GRID_COLOR },
};

export interface Band { start: number; size: number }

/** Column (or row) bands of a grid laid over a box of `len` design px. */
export function gridBands(g: LayoutGrid, len: number): Band[] {
  const n = Math.max(1, Math.round(g.count ?? 12));
  let gutter = Math.max(0, g.gutter ?? 0), margin = Math.max(0, g.margin ?? 0);
  const spacing = 2 * margin + (n - 1) * gutter;
  if (spacing >= len * 0.75 && spacing > 0) { // e.g. a desktop grid on a phone-width frame: shrink the gaps, keep the columns
    const k = (len * 0.5) / spacing;
    gutter *= k; margin *= k;
  }
  const size = (len - 2 * margin - (n - 1) * gutter) / n;
  if (size <= 0) return [];
  return Array.from({ length: n }, (_, i) => ({ start: margin + i * (size + gutter), size }));
}

/** A frame's grid when it should be drawn / snapped to. */
export function activeGrid(n: DNode | null | undefined, prefs: GuidePrefs): LayoutGrid | null {
  const g = n?.props?.layoutGrid;
  return g && g.visible !== false && prefs.grids() ? g : null;
}

// ------------------------------------------------------------------ snapping

/**
 * Extra snap targets for a layer moved / resized inside `parent`: ruler guides
 * and the parent's layout-grid column (row) edges. Everything is returned in
 * the parent's design px (the space editor-canvas.ts snaps in).
 * `pr` and `art` are screen rects of the parent and the artboard.
 */
export function guideSnapTargets(store: EditorStore, prefs: GuidePrefs, parent: DNode, pr: Rect, art: Rect, z: number): Guides {
  const out: Guides = { x: [], y: [] };
  if (prefs.guides()) {
    const g = docGuides(store.doc());
    const ox = (art.x - pr.x) / z, oy = (art.y - pr.y) / z;
    out.x.push(...g.x.map(v => v + ox));
    out.y.push(...g.y.map(v => v + oy));
  }
  const lg = activeGrid(parent, prefs);
  if (lg && lg.type !== 'grid') {
    const bands = gridBands(lg, (lg.type === 'columns' ? pr.w : pr.h) / z);
    const edges = bands.flatMap(b => [b.start, b.start + b.size]);
    (lg.type === 'columns' ? out.x : out.y).push(...edges);
  }
  return out;
}

/** Closest target within `th` of `pos` (returns the offset to add), or 0. */
export function snapOffset(targets: number[], pos: number, th: number): number {
  let best = 0, bestAbs = th;
  for (const t of targets) {
    const d = t - pos;
    if (Math.abs(d) < bestAbs) { best = d; bestAbs = Math.abs(d); }
  }
  return best;
}
