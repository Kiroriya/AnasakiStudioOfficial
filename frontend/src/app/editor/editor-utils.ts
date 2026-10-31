/**
 * Small pure helpers of the designer (no Angular, no DOM unless stated), kept
 * apart so they can be unit-tested on their own.
 */
import type { DNode } from './design-types';
import type { BlockInfo } from '../core/models';

// ------------------------------------------------------------------ structural sharing

/** Deep equality of JSON-like values (what a design document is made of). */
export function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (typeof a !== 'object' || typeof b !== 'object' || a === null || b === null) return a !== a && b !== b; // NaN
  if (Array.isArray(a) !== Array.isArray(b)) return false;
  if (Array.isArray(a)) {
    const bb = b as unknown[];
    if (a.length !== bb.length) return false;
    for (let i = 0; i < a.length; i++) if (!deepEqual(a[i], bb[i])) return false;
    return true;
  }
  const ao = a as Record<string, unknown>, bo = b as Record<string, unknown>;
  const ak = Object.keys(ao).filter(k => ao[k] !== undefined), bk = Object.keys(bo).filter(k => bo[k] !== undefined);
  if (ak.length !== bk.length) return false;
  for (const k of ak) if (!deepEqual(ao[k], bo[k])) return false;
  return true;
}

/** Same node fields, children aside. */
function sameFields(a: DNode, b: DNode): boolean {
  const ao = a as unknown as Record<string, unknown>, bo = b as unknown as Record<string, unknown>;
  const ak = Object.keys(ao).filter(k => k !== 'children' && ao[k] !== undefined);
  const bk = Object.keys(bo).filter(k => k !== 'children' && bo[k] !== undefined);
  if (ak.length !== bk.length) return false;
  for (const k of ak) if (!deepEqual(ao[k], bo[k])) return false;
  return true;
}

/**
 * Structural sharing between two versions of a document: returns a tree equal
 * to `next` in which every subtree that did not change is the very object of
 * `prev` (so unchanged layers keep their identity — nothing re-renders — and
 * undo history entries share memory). When nothing changed, `prev` itself is
 * returned.
 *
 * `copy` = `next` is a working copy that will keep being edited in place, so
 * the result never contains its objects (changed nodes are copied).
 */
export function shareTree(prev: DNode | undefined, next: DNode, copy = false): DNode {
  let kids: DNode[] | undefined;
  if (next.children) {
    const byId = new Map((prev?.children ?? []).map(c => [c.id, c] as const));
    kids = next.children.map(c => shareTree(byId.get(c.id), c, copy));
  }
  const pk = prev?.children;
  const sameKids = kids === undefined ? pk === undefined : !!pk && pk.length === kids.length && kids.every((k, i) => k === pk[i]);
  const fieldsSame = !!prev && sameFields(prev, next);
  if (prev && fieldsSame && sameKids) return prev;
  let out: DNode;
  if (prev && fieldsSame) out = { ...prev }; // fields of prev are immutable: share them
  else if (copy) {
    const { children: _c, ...fields } = next;
    out = structuredClone(fields) as DNode;
  } else out = next;
  if (kids) out.children = kids;
  else delete out.children;
  return out;
}

// ------------------------------------------------------------------ layers panel

/**
 * For the layer tree: every layer that contains a selected layer
 * (`ancestors`, to auto-expand them) and every layer inside a selected one
 * (`inside`, drawn as "part of the selection"). One walk for all rows.
 */
export function selectionRelatives(root: DNode, selected: readonly string[]): { ancestors: Set<string>; inside: Set<string> } {
  const ancestors = new Set<string>(), inside = new Set<string>();
  if (!selected.length) return { ancestors, inside };
  const want = new Set(selected);
  const path: string[] = [];
  const walk = (n: DNode, under: boolean) => {
    if (under) inside.add(n.id);
    if (want.has(n.id)) for (const a of path) ancestors.add(a);
    path.push(n.id);
    const sel = under || want.has(n.id);
    n.children?.forEach(c => walk(c, sel));
    path.pop();
  };
  walk(root, false);
  return { ancestors, inside };
}

// ------------------------------------------------------------------ caches

/** Least-recently-used map with a size cap (reads refresh an entry). */
export class LruCache<K, V> {
  private map = new Map<K, V>();
  constructor(readonly cap: number) {}
  get size() { return this.map.size; }
  has(k: K) { return this.map.has(k); }
  get(k: K): V | undefined {
    if (!this.map.has(k)) return undefined;
    const v = this.map.get(k) as V;
    this.map.delete(k);
    this.map.set(k, v);
    return v;
  }
  /** Without refreshing it. */
  peek(k: K): V | undefined { return this.map.get(k); }
  set(k: K, v: V) {
    this.map.delete(k);
    this.map.set(k, v);
    while (this.map.size > this.cap) this.map.delete(this.map.keys().next().value as K);
  }
  /** Newest first. */
  entriesNewestFirst(): [K, V][] { return [...this.map].reverse(); }
}

// ------------------------------------------------------------------ geometry

export interface Pt { x: number; y: number }

/**
 * Places points given in a layer's own box (0…w × 0…h) where the layer is
 * drawn: CSS `rotate(deg) scale(±1, ±1)` around the box centre, the centre put
 * at (cx, cy). Rotation is clockwise in screen coordinates, like CSS.
 */
export function placeInBox(pts: Pt[], w: number, h: number, cx: number, cy: number, deg = 0, flipX = false, flipY = false): Pt[] {
  const a = (deg * Math.PI) / 180, cos = Math.cos(a), sin = Math.sin(a);
  return pts.map(p => {
    let x = p.x - w / 2, y = p.y - h / 2;
    if (flipX) x = -x;
    if (flipY) y = -y;
    return { x: cx + x * cos - y * sin, y: cy + x * sin + y * cos };
  });
}

/**
 * Box of a layer resized from handle `h` (n, ne, e … nw): the opposite edge
 * stays put, an axis that only follows the aspect ratio (Shift on a side
 * handle) grows from its centre, and Alt grows from the box centre.
 */
export function resizedBox(h: string, x0: number, y0: number, w0: number, h0: number, w: number, hh: number, fromCenter: boolean): { x: number; y: number } {
  if (fromCenter) return { x: x0 + (w0 - w) / 2, y: y0 + (h0 - hh) / 2 };
  const x = h.includes('w') ? x0 + w0 - w : h.includes('e') ? x0 : x0 + (w0 - w) / 2;
  const y = h.includes('n') ? y0 + h0 - hh : h.includes('s') ? y0 : y0 + (h0 - hh) / 2;
  return { x, y };
}

// ------------------------------------------------------------------ text

/**
 * Replaces `re` (global) inside the text of formatted HTML, text node by text
 * node, so bold / colors / links stay. A match split across two formats is
 * not replaced. Needs a DOM (`document`).
 */
export function replaceInRichHtml(html: string, re: RegExp, by: string): string {
  const t = document.createElement('template');
  t.innerHTML = html;
  const walker = document.createTreeWalker(t.content, NodeFilter.SHOW_TEXT);
  let changed = false;
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const v = n.nodeValue ?? '';
    re.lastIndex = 0;
    const next = v.replace(re, () => by);
    if (next !== v) { n.nodeValue = next; changed = true; }
  }
  return changed ? t.innerHTML : html;
}

/** Plain text of formatted HTML (find in rich text). Needs a DOM. */
export function richText(html: string): string {
  const t = document.createElement('template');
  t.innerHTML = html;
  return t.content.textContent ?? '';
}

// ------------------------------------------------------------------ plugin blocks

/**
 * GET /blocks as the props panel needs it: select options with string values
 * (server-resolved options of JS plugin blocks may carry numbers, e.g. a form id)
 * and a label (the value when missing). Bad entries are dropped.
 */
export function normalizeBlocks(raw: unknown): BlockInfo[] {
  if (!Array.isArray(raw)) return [];
  return (raw as BlockInfo[]).filter(b => b && typeof b.plugin === 'string' && typeof b.type === 'string').map(b => ({
    ...b,
    props: (Array.isArray(b.props) ? b.props : []).map(f => (f.type === 'select' && Array.isArray(f.options)
      ? { ...f, options: f.options.filter(o => o && o.value !== undefined && o.value !== null).map(o => ({ value: String(o.value), label: o.label ?? String(o.value) })) }
      : f)),
  }));
}
