import { DNode } from './design-types';

/**
 * Vector paths (the `vector` node, pen tool, boolean ops, flatten).
 *
 * A vector node stores SVG path data in props.path, drawn in props.viewBox
 * ("0 0 w h"); it stretches with the layer box. vectorSvg() mirrors
 * VectorSVG() in backend/internal/design/vector.go — keep both in sync.
 */

export interface Pt { x: number; y: number }
/** Normalized segment: absolute line or cubic. */
export type Seg = { t: 'L'; p: Pt } | { t: 'C'; c1: Pt; c2: Pt; p: Pt };
export interface SubPath { start: Pt; segs: Seg[]; closed: boolean }
export interface ViewBox { x: number; y: number; w: number; h: number }

export const MAX_VECTOR_PATH = 60000;
const PATH_RE = /^[MmLlHhVvCcSsQqTtAaZz0-9.,\-\s eE+]*$/;
const PAINT_RE = /^(#[0-9a-fA-F]{3,8}|(?:rgb|rgba|hsl|hsla)\([0-9.,%\s/+\-]{1,60}\)|[a-zA-Z]{1,30}|var\(--[A-Za-z0-9_\-]{1,60}\))$/;

/** Same rule as design.ValidVectorPath. */
export function validPath(d: string | undefined): boolean {
  const s = (d ?? '').trim();
  return !!s && s.length <= MAX_VECTOR_PATH && PATH_RE.test(s) && (s[0] === 'M' || s[0] === 'm');
}

/** Same rule as design.VectorPaint ('' = not a plain colour). */
export function validPaint(v: string | undefined): string {
  const s = (v ?? '').trim();
  return s && PAINT_RE.test(s) ? s : '';
}

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&#34;').replace(/'/g, '&#39;');
const fnum = (v: number) => String(+v.toFixed(6));

export function parseViewBox(v: string | undefined): ViewBox | null {
  const f = (v ?? '').split(/[\s,]+/).filter(Boolean).map(Number);
  if (f.length !== 4 || f.some(x => !isFinite(x) || Math.abs(x) > 1e6) || f[2] <= 0 || f[3] <= 0) return null;
  return { x: f[0], y: f[1], w: f[2], h: f[3] };
}

/** The node's viewBox, or its box size when missing (as in Go). */
export function viewBoxOf(n: DNode): ViewBox {
  const vb = parseViewBox(n.props?.viewBox);
  if (vb) return vb;
  const w = n.width.mode === 'fixed' && (n.width.value ?? 0) > 0 ? n.width.value! : 100;
  const h = n.height.mode === 'fixed' && (n.height.value ?? 0) > 0 ? n.height.value! : 100;
  return { x: 0, y: 0, w, h };
}

/** Paint attributes of the <path>. Mirrors VectorPaintAttrs() in vector.go. */
export function vectorPaintAttrs(n: DNode): string {
  const s = n.style ?? {}, p = n.props ?? {};
  const fill = validPaint(s.fill) || 'none';
  let stroke = validPaint(s.stroke);
  let sw = s.strokeWidth ?? 0;
  if (!stroke || sw <= 0) { stroke = 'none'; sw = 0; }
  sw = Math.min(sw, 200);
  let out = ` style="fill:${esc(fill)};stroke:${esc(stroke)}"`;
  if (sw > 0) {
    out += ` stroke-width="${fnum(sw)}"`;
    let c = ['round', 'square', 'butt'].includes(p.strokeCap ?? '') ? p.strokeCap! : '';
    if (!c && s.strokeStyle === 'dotted') c = 'round';
    if (c) out += ` stroke-linecap="${c}"`;
    if (['round', 'bevel', 'miter'].includes(p.strokeJoin ?? '')) out += ` stroke-linejoin="${p.strokeJoin}"`;
    if (s.strokeStyle === 'dashed') out += ` stroke-dasharray="${fnum(sw * 3)} ${fnum(sw * 2)}"`;
    else if (s.strokeStyle === 'dotted') out += ` stroke-dasharray="0 ${fnum(sw * 2)}"`;
  }
  if (p.fillRule === 'evenodd') out += ` fill-rule="evenodd"`;
  return out + ` vector-effect="non-scaling-stroke"`;
}

/** The <svg> of a vector node (canvas). Mirrors VectorSVG() in vector.go (the class list is the host's job here). */
export function vectorSvg(n: DNode): string {
  const vb = viewBoxOf(n);
  const label = (n.props?.label ?? '').trim();
  const a11y = label ? ` role="img" aria-label="${esc(label)}"` : ' aria-hidden="true"';
  const open = `<svg viewBox="${fnum(vb.x)} ${fnum(vb.y)} ${fnum(vb.w)} ${fnum(vb.h)}" preserveAspectRatio="none" overflow="visible"${a11y} width="100%" height="100%" style="display:block">`;
  const d = (n.props?.path ?? '').trim();
  if (!validPath(d)) return open + '</svg>';
  return `${open}<path d="${esc(d)}"${vectorPaintAttrs(n)}/></svg>`;
}

// ------------------------------------------------------------------ parsing

/** Parses any SVG path data into absolute lines and cubics (arcs and quadratics converted). */
export function parsePath(d: string): SubPath[] {
  const toks = d.match(/[MmLlHhVvCcSsQqTtAaZz]|[-+]?(?:\d*\.\d+|\d+\.?)(?:[eE][-+]?\d+)?/g) ?? [];
  const out: SubPath[] = [];
  let i = 0, cmd = '', cur: Pt = { x: 0, y: 0 }, sub: SubPath | null = null;
  let lastC: Pt | null = null, lastQ: Pt | null = null;
  const isCmd = (t: string) => /^[a-zA-Z]$/.test(t);
  const num = () => Number(toks[i++]);
  const flag = () => {
    // arc flags may be written without separators ("a1 1 0 01 2 2")
    const t = toks[i];
    if (t.length > 1 && (t[0] === '0' || t[0] === '1') && !t.startsWith('0.')) { toks[i] = t.slice(1); return t[0] === '1'; }
    i++;
    return t === '1';
  };
  const ensure = () => {
    if (!sub) { sub = { start: { ...cur }, segs: [], closed: false }; out.push(sub); }
    return sub;
  };
  while (i < toks.length) {
    if (isCmd(toks[i])) cmd = toks[i++];
    else if (!cmd) { i++; continue; }
    const rel = cmd === cmd.toLowerCase();
    const C = cmd.toUpperCase();
    const ox = rel ? cur.x : 0, oy = rel ? cur.y : 0;
    if (C === 'Z') {
      if (sub) { sub.closed = true; cur = { ...sub.start }; sub = null; }
      lastC = lastQ = null;
      continue;
    }
    if (i >= toks.length || isCmd(toks[i])) continue;
    if (C === 'M') {
      cur = { x: ox + num(), y: oy + num() };
      sub = { start: { ...cur }, segs: [], closed: false };
      out.push(sub);
      cmd = rel ? 'l' : 'L';
      lastC = lastQ = null;
      continue;
    }
    const s = ensure();
    if (C === 'L' || C === 'H' || C === 'V') {
      const p: Pt = C === 'L' ? { x: ox + num(), y: oy + num() } : C === 'H' ? { x: ox + num(), y: cur.y } : { x: cur.x, y: oy + num() };
      s.segs.push({ t: 'L', p });
      cur = p; lastC = lastQ = null;
    } else if (C === 'C' || C === 'S') {
      const c1: Pt = C === 'C' ? { x: ox + num(), y: oy + num() } : lastC ? { x: 2 * cur.x - lastC.x, y: 2 * cur.y - lastC.y } : { ...cur };
      const c2 = { x: ox + num(), y: oy + num() };
      const p = { x: ox + num(), y: oy + num() };
      s.segs.push({ t: 'C', c1, c2, p });
      cur = p; lastC = c2; lastQ = null;
    } else if (C === 'Q' || C === 'T') {
      const q: Pt = C === 'Q' ? { x: ox + num(), y: oy + num() } : lastQ ? { x: 2 * cur.x - lastQ.x, y: 2 * cur.y - lastQ.y } : { ...cur };
      const p = { x: ox + num(), y: oy + num() };
      s.segs.push({ t: 'C', c1: lerp(cur, q, 2 / 3), c2: lerp(p, q, 2 / 3), p });
      cur = p; lastQ = q; lastC = null;
    } else if (C === 'A') {
      const rx = num(), ry = num(), rot = num(), large = flag(), sweep = flag();
      const p = { x: ox + num(), y: oy + num() };
      for (const c of arcToCubics(cur, p, rx, ry, rot, large, sweep)) s.segs.push(c);
      cur = p; lastC = lastQ = null;
    } else i++;
    if ([cur.x, cur.y].some(v => !isFinite(v))) return out.filter(sp => sp.segs.every(sg => isFinite(sg.p.x) && isFinite(sg.p.y)));
  }
  return out;
}

const lerp = (a: Pt, b: Pt, t: number): Pt => ({ x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t });

/** SVG endpoint arc → cubic Béziers (≤ 90° each). */
function arcToCubics(p0: Pt, p1: Pt, rx: number, ry: number, rotDeg: number, large: boolean, sweep: boolean): Seg[] {
  rx = Math.abs(rx); ry = Math.abs(ry);
  if (!rx || !ry || (p0.x === p1.x && p0.y === p1.y)) return [{ t: 'L', p: { ...p1 } }];
  const phi = (rotDeg * Math.PI) / 180, cos = Math.cos(phi), sin = Math.sin(phi);
  const dx = (p0.x - p1.x) / 2, dy = (p0.y - p1.y) / 2;
  const x1 = cos * dx + sin * dy, y1 = -sin * dx + cos * dy;
  const lam = (x1 * x1) / (rx * rx) + (y1 * y1) / (ry * ry);
  if (lam > 1) { rx *= Math.sqrt(lam); ry *= Math.sqrt(lam); }
  const num = rx * rx * ry * ry - rx * rx * y1 * y1 - ry * ry * x1 * x1;
  let co = Math.sqrt(Math.max(0, num / (rx * rx * y1 * y1 + ry * ry * x1 * x1)));
  if (large === sweep) co = -co;
  const cxp = (co * rx * y1) / ry, cyp = (-co * ry * x1) / rx;
  const cx = cos * cxp - sin * cyp + (p0.x + p1.x) / 2, cy = sin * cxp + cos * cyp + (p0.y + p1.y) / 2;
  const ang = (ux: number, uy: number, vx: number, vy: number) => Math.atan2(ux * vy - uy * vx, ux * vx + uy * vy);
  const t1 = ang(1, 0, (x1 - cxp) / rx, (y1 - cyp) / ry);
  let dt = ang((x1 - cxp) / rx, (y1 - cyp) / ry, (-x1 - cxp) / rx, (-y1 - cyp) / ry);
  if (!sweep && dt > 0) dt -= 2 * Math.PI;
  else if (sweep && dt < 0) dt += 2 * Math.PI;
  const n = Math.max(1, Math.ceil(Math.abs(dt) / (Math.PI / 2)));
  const step = dt / n, k = (4 / 3) * Math.tan(step / 4);
  const pt = (t: number): Pt => ({ x: cx + rx * Math.cos(t) * cos - ry * Math.sin(t) * sin, y: cy + rx * Math.cos(t) * sin + ry * Math.sin(t) * cos });
  const dpt = (t: number): Pt => ({ x: -rx * Math.sin(t) * cos - ry * Math.cos(t) * sin, y: -rx * Math.sin(t) * sin + ry * Math.cos(t) * cos });
  const segs: Seg[] = [];
  for (let j = 0; j < n; j++) {
    const a = t1 + j * step, b = a + step;
    const pa = pt(a), pb = j === n - 1 ? { ...p1 } : pt(b), da = dpt(a), db = dpt(b);
    segs.push({ t: 'C', c1: { x: pa.x + k * da.x, y: pa.y + k * da.y }, c2: { x: pb.x - k * db.x, y: pb.y - k * db.y }, p: pb });
  }
  return segs;
}

// ------------------------------------------------------------------ writing / geometry

const r2 = (v: number) => String(Math.round(v * 100) / 100);

export function serialize(subs: SubPath[]): string {
  return subs.map(s => {
    let d = `M${r2(s.start.x)} ${r2(s.start.y)}`;
    for (const g of s.segs) {
      d += g.t === 'L' ? `L${r2(g.p.x)} ${r2(g.p.y)}` : `C${r2(g.c1.x)} ${r2(g.c1.y)} ${r2(g.c2.x)} ${r2(g.c2.y)} ${r2(g.p.x)} ${r2(g.p.y)}`;
    }
    return s.closed ? d + 'Z' : d;
  }).join('');
}

export function mapPath(subs: SubPath[], f: (p: Pt) => Pt): SubPath[] {
  return subs.map(s => ({
    start: f(s.start), closed: s.closed,
    segs: s.segs.map(g => (g.t === 'L' ? { t: 'L' as const, p: f(g.p) } : { t: 'C' as const, c1: f(g.c1), c2: f(g.c2), p: f(g.p) })),
  }));
}

/** Polylines of each subpath (curves flattened). */
export function flattenPath(subs: SubPath[], tol = 0.5): Pt[][] {
  return subs.map(s => {
    const pts: Pt[] = [s.start];
    let cur = s.start;
    for (const g of s.segs) {
      if (g.t === 'L') pts.push(g.p);
      else {
        const len = dist(cur, g.c1) + dist(g.c1, g.c2) + dist(g.c2, g.p);
        const n = Math.min(64, Math.max(4, Math.ceil(Math.sqrt(len / tol))));
        for (let i = 1; i <= n; i++) pts.push(bez(cur, g.c1, g.c2, g.p, i / n));
      }
      cur = g.p;
    }
    return pts;
  });
}

const dist = (a: Pt, b: Pt) => Math.hypot(a.x - b.x, a.y - b.y);
function bez(p0: Pt, c1: Pt, c2: Pt, p1: Pt, t: number): Pt {
  const u = 1 - t;
  return {
    x: u * u * u * p0.x + 3 * u * u * t * c1.x + 3 * u * t * t * c2.x + t * t * t * p1.x,
    y: u * u * u * p0.y + 3 * u * u * t * c1.y + 3 * u * t * t * c2.y + t * t * t * p1.y,
  };
}

export function bounds(subs: SubPath[]): { x: number; y: number; w: number; h: number } | null {
  const pts = flattenPath(subs, 0.25).flat();
  if (!pts.length) return null;
  const xs = pts.map(p => p.x), ys = pts.map(p => p.y);
  const x = Math.min(...xs), y = Math.min(...ys);
  return { x, y, w: Math.max(...xs) - x, h: Math.max(...ys) - y };
}

/** A node's path in its own box px (viewBox scaled to width × height). */
export function nodePath(n: DNode, w: number, h: number): SubPath[] {
  const vb = viewBoxOf(n);
  return mapPath(parsePath(n.props?.path ?? ''), p => ({ x: ((p.x - vb.x) * w) / vb.w, y: ((p.y - vb.y) * h) / vb.h }));
}

/** Rounded rectangle as a path (CSS border-radius clamping; 9999 = ellipse/pill). */
export function roundRectPath(w: number, h: number, radii: [number, number, number, number]): SubPath[] {
  let [tl, tr, br, bl] = radii.map(r => Math.max(0, r));
  const f = Math.min(1, w / (tl + tr || 1), w / (bl + br || 1), h / (tl + bl || 1), h / (tr + br || 1));
  [tl, tr, br, bl] = [tl * f, tr * f, br * f, bl * f];
  const k = 0.5523;
  const segs: Seg[] = [];
  const C = (c1: Pt, c2: Pt, p: Pt) => segs.push({ t: 'C', c1, c2, p });
  const L = (p: Pt) => segs.push({ t: 'L', p });
  L({ x: w - tr, y: 0 });
  if (tr) C({ x: w - tr + tr * k, y: 0 }, { x: w, y: tr - tr * k }, { x: w, y: tr });
  L({ x: w, y: h - br });
  if (br) C({ x: w, y: h - br + br * k }, { x: w - br + br * k, y: h }, { x: w - br, y: h });
  L({ x: bl, y: h });
  if (bl) C({ x: bl - bl * k, y: h }, { x: 0, y: h - bl + bl * k }, { x: 0, y: h - bl });
  L({ x: 0, y: tl });
  if (tl) C({ x: 0, y: tl - tl * k }, { x: tl - tl * k, y: 0 }, { x: tl, y: 0 });
  return [{ start: { x: tl, y: 0 }, segs: segs.filter((g, i) => i < segs.length - 1 || g.t !== 'L' || dist(g.p, { x: tl, y: 0 }) > 1e-6), closed: true }];
}

/** Path data of the elements of a built-in icon / shape body (path, circle, ellipse, rect, line, polygon, polyline). */
export function svgBodyToPath(body: string): string {
  const doc = new DOMParser().parseFromString(`<svg xmlns="http://www.w3.org/2000/svg">${body}</svg>`, 'image/svg+xml');
  const parts: string[] = [];
  const n = (el: Element, a: string) => Number(el.getAttribute(a) ?? 0) || 0;
  doc.querySelectorAll('path,circle,ellipse,rect,line,polygon,polyline').forEach(el => {
    switch (el.tagName) {
      case 'path': { const d = el.getAttribute('d'); if (d) parts.push(d); break; }
      case 'circle': case 'ellipse': {
        const cx = n(el, 'cx'), cy = n(el, 'cy');
        const rx = el.tagName === 'circle' ? n(el, 'r') : n(el, 'rx'), ry = el.tagName === 'circle' ? n(el, 'r') : n(el, 'ry');
        parts.push(`M${cx - rx} ${cy}A${rx} ${ry} 0 1 0 ${cx + rx} ${cy}A${rx} ${ry} 0 1 0 ${cx - rx} ${cy}Z`);
        break;
      }
      case 'rect': {
        const x = n(el, 'x'), y = n(el, 'y'), w = n(el, 'width'), h = n(el, 'height');
        const r = n(el, 'rx') || n(el, 'ry');
        parts.push(serialize(mapPath(roundRectPath(w, h, [r, r, r, r]), p => ({ x: p.x + x, y: p.y + y }))));
        break;
      }
      case 'line': parts.push(`M${n(el, 'x1')} ${n(el, 'y1')}L${n(el, 'x2')} ${n(el, 'y2')}`); break;
      default: {
        const pts = (el.getAttribute('points') ?? '').trim().split(/[\s,]+/).map(Number);
        if (pts.length >= 4) parts.push('M' + pts.join(' ') + (el.tagName === 'polygon' ? 'Z' : ''));
      }
    }
  });
  return parts.join(' ');
}
