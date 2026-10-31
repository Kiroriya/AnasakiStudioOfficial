import polygonClipping, { Geom, MultiPolygon, Ring } from 'polygon-clipping';
import { tr } from '../core/i18n';
import { DNode, Style, findNode, newId } from './design-types';
import type { Command } from './commands';
import { EditorStore } from './editor-store';
import { exportNode } from './export';
import { SubPath, bounds, flattenPath, mapPath, nodePath, parsePath, roundRectPath, serialize, svgBodyToPath } from './vector';
import { placeInBox } from './editor-utils';
import './messages-vector';

export type BoolOp = 'union' | 'subtract' | 'intersect' | 'exclude';
export const BOOL_OPS: { op: BoolOp; icon: string; keys: string }[] = [
  { op: 'union', icon: '⊔', keys: 'Ctrl+Alt+U' },
  { op: 'subtract', icon: '⊖', keys: 'Ctrl+Alt+S' },
  { op: 'intersect', icon: '⊓', keys: 'Ctrl+Alt+I' },
  { op: 'exclude', icon: '⊻', keys: 'Ctrl+Alt+X' },
];

const SHAPE_TYPES = ['rect', 'vector', 'shape'];
const FLATTEN_TYPES = ['rect', 'shape', 'icon'];

/** Rect / ellipse / shape / vector — what boolean operations accept. */
export const isShapeNode = (n: DNode) => SHAPE_TYPES.includes(n.type);

/** 2+ shape layers in the same parent are selected. */
export function canBoolean(s: EditorStore): boolean {
  const sel = s.selection();
  return sel.length >= 2 && sel.every(o => o.parent && o.parent.id === sel[0].parent?.id && isShapeNode(o.node) && !o.node.locked);
}

export function canFlatten(s: EditorStore): boolean {
  const sel = s.selection();
  return sel.length > 0 && sel.every(o => FLATTEN_TYPES.includes(o.node.type) && !o.node.locked);
}

/** Size of a layer in design px (fixed size, else as drawn on the canvas). */
function boxSize(s: EditorStore, n: DNode): { w: number; h: number } {
  const e = s.eff(n);
  const r = s.measure?.(n.id);
  return {
    w: e.width.mode === 'fixed' ? e.width.value ?? 0 : r?.w ?? 100,
    h: e.height.mode === 'fixed' ? e.height.value ?? 0 : r?.h ?? 100,
  };
}

function radiiOf(st: Style | undefined): [number, number, number, number] {
  if (st?.radii?.length === 4 && st.radii.some(v => v)) return st.radii;
  const r = st?.radius ?? 0;
  return [r, r, r, r];
}

/** Outline of a shape layer in its own box px (strokes are not outlined). */
export function outlineOf(s: EditorStore, n: DNode, w: number, h: number): SubPath[] {
  const e = s.eff(n);
  if (n.type === 'rect') return roundRectPath(w, h, radiiOf(e.style));
  if (n.type === 'vector') return nodePath(n, w, h);
  if (n.type === 'shape' || n.type === 'icon') {
    const a = s.assets();
    if (n.type === 'shape') {
      const sh = a?.shapes[n.props?.shape ?? ''] ?? a?.shapes['wave'];
      if (!sh) return [];
      const vb = sh.viewBox.split(/[\s,]+/).map(Number);
      return fitInto(parsePath(svgBodyToPath(sh.body)), vb, w, h, !!sh.stretch);
    }
    const ic = a?.icons[n.props?.icon ?? ''] ?? a?.icons['star'];
    return ic ? fitInto(parsePath(svgBodyToPath(ic.body)), [0, 0, 24, 24], w, h, false) : [];
  }
  return [];
}

/** viewBox → box mapping (preserveAspectRatio none, or xMidYMid meet). */
function fitInto(subs: SubPath[], vb: number[], w: number, h: number, stretch: boolean): SubPath[] {
  const [vx, vy, vw, vh] = vb.length === 4 && vb[2] > 0 && vb[3] > 0 ? vb : [0, 0, 100, 100];
  if (stretch) return mapPath(subs, p => ({ x: ((p.x - vx) * w) / vw, y: ((p.y - vy) * h) / vh }));
  const k = Math.min(w / vw, h / vh), ox = (w - vw * k) / 2, oy = (h - vh * k) / 2;
  return mapPath(subs, p => ({ x: ox + (p.x - vx) * k, y: oy + (p.y - vy) * k }));
}

/**
 * A layer's filled area as polygons in artboard px (subpaths combined even-odd),
 * with its rotation and flips applied around its centre like on the canvas.
 * (The measured rect of a rotated layer is its rotated bounding box; its
 * centre is still the layer's centre.)
 */
function areaOf(s: EditorStore, n: DNode): MultiPolygon {
  const r = s.measure?.(n.id);
  if (!r) return [];
  const { w, h } = boxSize(s, n);
  const e = s.eff(n);
  const cx = r.x + r.w / 2, cy = r.y + r.h / 2;
  const rings: Ring[] = flattenPath(outlineOf(s, n, w, h), 0.2)
    .filter(pts => pts.length >= 3)
    .map(pts => placeInBox(pts, w, h, cx, cy, e.style?.rotation ?? 0, !!n.props?.flipX, !!n.props?.flipY).map(p => [p.x, p.y] as [number, number]));
  if (!rings.length) return [];
  return polygonClipping.xor([[rings[0]]], ...rings.slice(1).map(rg => [[rg]] as Geom));
}

/** Figma boolean operation: replaces the selected shapes with one vector layer. */
export function booleanOp(s: EditorStore, op: BoolOp) {
  if (!canBoolean(s)) { s.notify(tr('vec.needShapes'), true); return; }
  const sel = [...s.selection()].sort((a, b) => a.index - b.index); // bottom-most first
  const parent = sel[0].parent!;
  const areas = sel.map(o => areaOf(s, o.node));
  let res: MultiPolygon;
  try {
    const [a, ...rest] = areas;
    res = op === 'union' ? polygonClipping.union(a, ...rest)
      : op === 'subtract' ? polygonClipping.difference(a, ...rest)
      : op === 'intersect' ? polygonClipping.intersection(a, ...rest)
      : polygonClipping.xor(a, ...rest);
  } catch {
    s.notify(tr('vec.failed'), true);
    return;
  }
  if (!res.length) { s.notify(tr('vec.empty'), true); return; }
  const subs: SubPath[] = res.flat().map(ring => {
    const pts = ring.map(([x, y]) => ({ x, y }));
    if (pts.length > 1 && pts[0].x === pts[pts.length - 1].x && pts[0].y === pts[pts.length - 1].y) pts.pop();
    return { start: pts[0], segs: pts.slice(1).map(p => ({ t: 'L' as const, p })), closed: true };
  });
  const base = sel[0].node;
  const node = vectorFromPath(subs, s.measure?.(parent.id) ?? { x: 0, y: 0 }, {
    name: tr('vec.op.' + op),
    style: paintFrom(s, base),
    fillRule: 'evenodd',
  });
  if (!node) return;
  if ((s.eff(parent).layout?.mode ?? 'none') !== 'none') node.absolute = true; // keep it where it was drawn
  const ids = new Set(sel.map(o => o.node.id));
  s.mutate(root => {
    const p = findNode(root, parent.id)!.node;
    p.children = (p.children ?? []).filter(c => !ids.has(c.id));
    p.children.splice(Math.min(sel[0].index, p.children.length), 0, node);
  });
  s.select(node.id);
}

/** Style of the bottom-most layer, as vector paint (fill falls back to grey). */
function paintFrom(s: EditorStore, n: DNode): Style {
  const st: Style = { ...s.eff(n).style };
  delete st.radius; delete st.radii; delete st.backgroundImage; delete st.overlay;
  delete st.rotation; // the rotation is already in the path points
  if (n.type === 'icon') { st.fill = st.color; delete st.color; }
  if (!st.fill) st.fill = '#d9d9d9';
  return st;
}

/**
 * New vector layer for a path given in artboard px; `origin` is the parent's
 * artboard position, so x / y come out relative to the parent.
 */
export function vectorFromPath(subs: SubPath[], origin: { x: number; y: number }, opts: { name?: string; style?: Style; fillRule?: 'evenodd' } = {}): DNode | null {
  const b = bounds(subs);
  if (!b) return null;
  const w = Math.max(1, round2(b.w)), h = Math.max(1, round2(b.h));
  const path = serialize(mapPath(subs, p => ({ x: p.x - b.x, y: p.y - b.y })));
  return {
    id: newId(), type: 'vector', name: opts.name ?? tr('ed.node.vector'),
    x: round2(b.x - origin.x), y: round2(b.y - origin.y),
    width: { mode: 'fixed', value: w }, height: { mode: 'fixed', value: h },
    style: opts.style ?? { stroke: '#000000', strokeWidth: 2 },
    props: { path, viewBox: `0 0 ${w} ${h}`, ...(opts.fillRule ? { fillRule: opts.fillRule } : {}) },
  };
}

const round2 = (v: number) => Math.round(v * 100) / 100;

/** Flatten (Ctrl+E): rectangles, ellipses, icons and shapes become editable vectors in place. */
export function flattenSelection(s: EditorStore) {
  if (!canFlatten(s)) { s.notify(tr('vec.needFlatten'), true); return; }
  const jobs = s.selection().map(o => {
    const n = o.node, { w, h } = boxSize(s, n);
    const subs = outlineOf(s, n, w, h);
    const st: Style = { ...n.style };
    const props: DNode['props'] = { path: serialize(subs), viewBox: `0 0 ${round2(w)} ${round2(h)}`, label: n.props?.label };
    if (n.type === 'rect') { delete st.radius; delete st.radii; }
    if (n.type === 'icon') {
      const ic = s.assets()?.icons[n.props?.icon ?? ''];
      const color = st.color || 'currentColor';
      delete st.color;
      if (ic?.fill) { st.fill = color; delete st.stroke; delete st.strokeWidth; }
      else {
        st.stroke = color;
        st.strokeWidth = round2((n.props?.strokeWidth || 2) * Math.min(w, h) / 24);
        delete st.fill;
        props.strokeCap = 'round';
        props.strokeJoin = 'round';
      }
    }
    if (n.type === 'shape') props.fillRule = 'evenodd';
    return { id: n.id, w, h, st, props };
  });
  s.mutate(root => {
    for (const j of jobs) {
      const f = findNode(root, j.id);
      if (!f) continue;
      const n = f.node;
      n.type = 'vector';
      n.width = { mode: 'fixed', value: round2(j.w) };
      n.height = { mode: 'fixed', value: round2(j.h) };
      n.style = j.st;
      n.props = Object.fromEntries(Object.entries(j.props!).filter(([, v]) => v !== undefined));
      if (n.responsive) for (const o of Object.values(n.responsive)) { if (o?.style) { delete o.style.radius; delete o.style.radii; } }
    }
  });
}

/**
 * After editing points: the layer box is refitted to the path's bounds (the
 * layer moves / resizes, the drawing stays where it is on the canvas).
 */
export function refitVector(s: EditorStore, id: string) {
  const f = findNode(s.doc(), id);
  if (!f || f.node.type !== 'vector') return;
  const n = f.node, e = s.eff(n), { w, h } = boxSize(s, n);
  const subs = nodePath(n, w, h);
  const b = bounds(subs);
  if (!b) return;
  const nw = Math.max(1, round2(b.w)), nh = Math.max(1, round2(b.h));
  const path = serialize(mapPath(subs, p => ({ x: p.x - b.x, y: p.y - b.y })));
  const free = s.parentMode(id) === 'none';
  s.setProps(id, { path, viewBox: `0 0 ${nw} ${nh}` });
  s.setBox(id, free ? { x: round2((e.x ?? 0) + b.x), y: round2((e.y ?? 0) + b.y), w: nw, h: nh } : { w: nw, h: nh });
}

/**
 * Figma shortcuts of the vector commands (command id in commands.ts, or null):
 * Ctrl+Alt+U/S/I/X boolean ops, Ctrl+E flatten, Ctrl+Shift+E export.
 */
export function vectorShortcut(e: KeyboardEvent): string | null {
  if (!(e.ctrlKey || e.metaKey)) return null;
  if (e.altKey && !e.shiftKey) {
    return ({ KeyU: 'union', KeyS: 'subtract', KeyI: 'intersect', KeyX: 'exclude' } as Record<string, string>)[e.code] ?? null;
  }
  if (e.code === 'KeyE' && !e.altKey) return e.shiftKey ? 'exportSel' : 'flatten';
  return null;
}

/** Object-menu commands: boolean operations and flatten (labels follow the UI language). */
export function vectorCommands(s: EditorStore): Command[] {
  const bool = () => canBoolean(s);
  return [
    ...BOOL_OPS.map(b => ({ id: b.op, get label() { return tr('vec.op.' + b.op); }, keys: b.keys, run: () => booleanOp(s, b.op), enabled: bool })),
    { id: 'flatten', get label() { return tr('vec.flatten'); }, keys: 'Ctrl+E', run: () => flattenSelection(s), enabled: () => canFlatten(s) },
    { id: 'editPath', get label() { return tr('vec.editPath'); }, keys: 'Enter', run: () => s.vectorEditId.set(s.selectedId()),
      enabled: () => s.selectedIds().length === 1 && s.selected()?.node.type === 'vector' },
  ];
}

/** File-menu command: export the selection with the last used settings (Export section). */
export function exportCommand(s: EditorStore): Command {
  return { id: 'exportSel', get label() { return tr('vec.exportSel'); }, keys: 'Ctrl+Shift+E', run: () => { void exportNode(s); }, enabled: () => s.selectedIds().length === 1 };
}
