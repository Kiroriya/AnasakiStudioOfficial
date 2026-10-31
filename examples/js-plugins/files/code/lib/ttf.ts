// A small TrueType reader for PDF embedding: accepts a .ttf/.otf (TrueType outlines) or a WOFF 1.0 file
// whose tables are stored uncompressed (no zlib in goja), and reads what a PDF writer needs: cmap
// (formats 4 and 12), advance widths, metrics, the PostScript name and the GPOS mark-to-base /
// mark-to-mark anchors (enough to place Thai vowels and tone marks above/below their consonant).
//
// Not a shaper: no GSUB (ligatures, contextual forms) and no kerning. Thai SARA AM is decomposed
// (ำ → ํ + า) and reordered after a tone mark like the font's ccmp would do.

export interface Anchor {
  x: number;
  y: number;
}

interface MarkLookup {
  /** mark glyph → [class, anchor] */
  marks: Map<number, [number, Anchor]>;
  /** base (or mark2) glyph → anchors by class */
  bases: Map<number, (Anchor | null)[]>;
}

export interface TrueTypeFont {
  /** The sfnt bytes (TrueType), embedded as FontFile2. */
  sfnt: Uint8Array;
  postscriptName: string;
  unitsPerEm: number;
  ascent: number;
  descent: number;
  capHeight: number;
  italicAngle: number;
  bbox: [number, number, number, number];
  numGlyphs: number;
  /** Unicode code point → glyph id (0 = .notdef). */
  glyph(cp: number): number;
  /** Advance width of a glyph in font units. */
  advance(gid: number): number;
  isMark(gid: number): boolean;
  markBase: MarkLookup[];
  markMark: MarkLookup[];
}

class Reader {
  constructor(public b: ArrayLike<number>) {}
  u8(o: number) {
    return this.b[o];
  }
  u16(o: number) {
    return (this.b[o] << 8) | this.b[o + 1];
  }
  i16(o: number) {
    const v = this.u16(o);
    return v & 0x8000 ? v - 0x10000 : v;
  }
  u32(o: number) {
    return ((this.b[o] << 24) >>> 0) + ((this.b[o + 1] << 16) | (this.b[o + 2] << 8) | this.b[o + 3]);
  }
  tag(o: number) {
    return String.fromCharCode(this.b[o], this.b[o + 1], this.b[o + 2], this.b[o + 3]);
  }
}

function toBytes(src: ArrayLike<number> | ArrayBuffer): Uint8Array {
  if (src instanceof Uint8Array) return src;
  if (src instanceof ArrayBuffer) return new Uint8Array(src);
  const out = new Uint8Array(src.length);
  for (let i = 0; i < src.length; i++) out[i] = src[i] & 0xff;
  return out;
}

/** WOFF 1.0 with uncompressed tables → sfnt bytes (throws on compressed tables). */
function woffToSfnt(b: Uint8Array): Uint8Array {
  const r = new Reader(b);
  const flavor = r.u32(4);
  const n = r.u16(12);
  const tables: { tag: number; off: number; len: number; sum: number }[] = [];
  for (let i = 0; i < n; i++) {
    const e = 44 + i * 20;
    const comp = r.u32(e + 8);
    const orig = r.u32(e + 12);
    if (comp !== orig) throw new Error('files/ttf: WOFF tables must be stored uncompressed (no zlib here) — or use a .ttf');
    tables.push({ tag: r.u32(e), off: r.u32(e + 4), len: orig, sum: r.u32(e + 16) });
  }
  const pad = (x: number) => (x + 3) & ~3;
  let size = 12 + 16 * n;
  for (const t of tables) size += pad(t.len);
  const out = new Uint8Array(size);
  const w32 = (o: number, v: number) => {
    out[o] = (v >>> 24) & 255;
    out[o + 1] = (v >>> 16) & 255;
    out[o + 2] = (v >>> 8) & 255;
    out[o + 3] = v & 255;
  };
  const w16 = (o: number, v: number) => {
    out[o] = (v >>> 8) & 255;
    out[o + 1] = v & 255;
  };
  let es = 0;
  while (1 << (es + 1) <= n) es++;
  w32(0, flavor);
  w16(4, n);
  w16(6, (1 << es) * 16);
  w16(8, es);
  w16(10, n * 16 - (1 << es) * 16);
  let off = 12 + 16 * n;
  tables.forEach((t, i) => {
    const d = 12 + i * 16;
    w32(d, t.tag);
    w32(d + 4, t.sum);
    w32(d + 8, off);
    w32(d + 12, t.len);
    for (let k = 0; k < t.len; k++) out[off + k] = b[t.off + k];
    off += pad(t.len);
  });
  return out;
}

function coverage(r: Reader, o: number): Map<number, number> {
  const m = new Map<number, number>();
  const fmt = r.u16(o);
  const n = r.u16(o + 2);
  if (fmt === 1) for (let i = 0; i < n; i++) m.set(r.u16(o + 4 + i * 2), i);
  else if (fmt === 2) {
    for (let i = 0; i < n; i++) {
      const e = o + 4 + i * 6;
      const start = r.u16(e);
      const end = r.u16(e + 2);
      const idx = r.u16(e + 4);
      for (let g = start; g <= end; g++) m.set(g, idx + g - start);
    }
  }
  return m;
}

const anchor = (r: Reader, o: number): Anchor => ({ x: r.i16(o + 2), y: r.i16(o + 4) });

/** MarkBasePos / MarkMarkPos format 1 (same layout: mark coverage, base coverage, classes, arrays). */
function markLookup(r: Reader, st: number): MarkLookup | null {
  if (r.u16(st) !== 1) return null;
  const markCov = coverage(r, st + r.u16(st + 2));
  const baseCov = coverage(r, st + r.u16(st + 4));
  const classes = r.u16(st + 6);
  const markArr = st + r.u16(st + 8);
  const baseArr = st + r.u16(st + 10);
  const marks = new Map<number, [number, Anchor]>();
  markCov.forEach((i, gid) => {
    const rec = markArr + 2 + i * 4;
    marks.set(gid, [r.u16(rec), anchor(r, markArr + r.u16(rec + 2))]);
  });
  const bases = new Map<number, (Anchor | null)[]>();
  baseCov.forEach((i, gid) => {
    const list: (Anchor | null)[] = [];
    for (let c = 0; c < classes; c++) {
      const ao = r.u16(baseArr + 2 + (i * classes + c) * 2);
      list.push(ao ? anchor(r, baseArr + ao) : null);
    }
    bases.set(gid, list);
  });
  return { marks, bases };
}

function readGPOS(r: Reader, g: number, font: TrueTypeFont) {
  const featureList = g + r.u16(g + 6);
  const lookupList = g + r.u16(g + 8);
  const wanted = new Set<number>();
  const nf = r.u16(featureList);
  for (let i = 0; i < nf; i++) {
    const rec = featureList + 2 + i * 6;
    const tag = r.tag(rec);
    if (tag !== 'mark' && tag !== 'mkmk') continue;
    const f = featureList + r.u16(rec + 4);
    const n = r.u16(f + 2);
    for (let k = 0; k < n; k++) wanted.add(r.u16(f + 4 + k * 2));
  }
  [...wanted].sort((a, b) => a - b).forEach((li) => {
    const l = lookupList + r.u16(lookupList + 2 + li * 2);
    let type = r.u16(l);
    const n = r.u16(l + 4);
    for (let k = 0; k < n; k++) {
      let st = l + r.u16(l + 6 + k * 2);
      if (type === 9) {
        type = r.u16(st + 2);
        st = st + r.u32(st + 4);
      }
      if (type !== 4 && type !== 6) continue;
      const ml = markLookup(r, st);
      if (ml) (type === 4 ? font.markBase : font.markMark).push(ml);
    }
  });
}

/** Parses a TrueType font (.ttf, or WOFF 1.0 with uncompressed tables). / อ่านฟอนต์ TrueType */
export function parseTTF(src: ArrayLike<number> | ArrayBuffer): TrueTypeFont {
  let b = toBytes(src);
  if (b.length < 12) throw new Error('files/ttf: not a font');
  let r = new Reader(b);
  if (r.tag(0) === 'wOFF') {
    b = woffToSfnt(b);
    r = new Reader(b);
  }
  const ver = r.u32(0);
  if (ver !== 0x00010000 && r.tag(0) !== 'true') throw new Error('files/ttf: only TrueType outlines (glyf) are supported');
  const dir: Record<string, { off: number; len: number }> = {};
  const n = r.u16(4);
  for (let i = 0; i < n; i++) {
    const e = 12 + i * 16;
    dir[r.tag(e)] = { off: r.u32(e + 8), len: r.u32(e + 12) };
  }
  for (const t of ['head', 'hhea', 'hmtx', 'maxp', 'cmap', 'glyf', 'loca']) {
    if (!dir[t]) throw new Error('files/ttf: table ' + t + ' missing');
  }
  const head = dir.head.off;
  const hhea = dir.hhea.off;
  const numGlyphs = r.u16(dir.maxp.off + 4);
  const numH = r.u16(hhea + 34);
  const hmtx = dir.hmtx.off;
  const advances = new Array<number>(numGlyphs);
  for (let i = 0; i < numGlyphs; i++) advances[i] = r.u16(hmtx + Math.min(i, numH - 1) * 4);

  // cmap: prefer (3,10) format 12, then (3,1)/(0,x) format 4
  const cmap = dir.cmap.off;
  const map = new Map<number, number>();
  let best = -1;
  let bestScore = -1;
  for (let i = 0; i < r.u16(cmap + 2); i++) {
    const e = cmap + 4 + i * 8;
    const pid = r.u16(e);
    const eid = r.u16(e + 2);
    const st = cmap + r.u32(e + 4);
    const fmt = r.u16(st);
    const score = fmt === 12 && (pid === 3 || pid === 0) ? 3 : fmt === 4 && ((pid === 3 && eid === 1) || pid === 0) ? 2 : -1;
    if (score > bestScore) {
      best = st;
      bestScore = score;
    }
  }
  if (best < 0) throw new Error('files/ttf: no Unicode cmap (format 4 or 12)');
  if (r.u16(best) === 4) {
    const segs = r.u16(best + 6) / 2;
    const ends = best + 14;
    const starts = ends + segs * 2 + 2;
    const deltas = starts + segs * 2;
    const ranges = deltas + segs * 2;
    for (let s = 0; s < segs; s++) {
      const end = r.u16(ends + s * 2);
      const start = r.u16(starts + s * 2);
      const delta = r.u16(deltas + s * 2);
      const ro = r.u16(ranges + s * 2);
      for (let c = start; c <= end && c !== 0xffff; c++) {
        let gid: number;
        if (ro === 0) gid = (c + delta) & 0xffff;
        else {
          gid = r.u16(ranges + s * 2 + ro + (c - start) * 2);
          if (gid) gid = (gid + delta) & 0xffff;
        }
        if (gid) map.set(c, gid);
      }
    }
  } else {
    const groups = r.u32(best + 12);
    for (let i = 0; i < groups; i++) {
      const e = best + 16 + i * 12;
      const s = r.u32(e);
      const end = Math.min(r.u32(e + 4), s + 0xffff);
      const g = r.u32(e + 8);
      for (let c = s; c <= end; c++) map.set(c, g + c - s);
    }
  }

  // PostScript name (name ID 6), Windows UTF-16BE or Mac Roman
  let psName = '';
  if (dir.name) {
    const nm = dir.name.off;
    const strings = nm + r.u16(nm + 4);
    for (let i = 0; i < r.u16(nm + 2) && !psName; i++) {
      const e = nm + 6 + i * 12;
      if (r.u16(e + 6) !== 6) continue;
      const pid = r.u16(e);
      const len = r.u16(e + 8);
      const off = strings + r.u16(e + 10);
      let s = '';
      if (pid === 3 || pid === 0) for (let k = 0; k + 1 < len; k += 2) s += String.fromCharCode(r.u16(off + k));
      else for (let k = 0; k < len; k++) s += String.fromCharCode(r.u8(off + k));
      psName = s;
    }
  }
  psName = (psName || 'EmbeddedFont').replace(/[^A-Za-z0-9+\-_.]/g, '');

  const upm = r.u16(head + 18) || 1000;
  let capHeight = Math.round(r.i16(hhea + 4) * 0.7);
  if (dir['OS/2'] && r.u16(dir['OS/2'].off) >= 2 && dir['OS/2'].len >= 90) capHeight = r.i16(dir['OS/2'].off + 88);
  const italic = dir.post ? r.i16(dir.post.off + 4) + r.u16(dir.post.off + 6) / 65536 : 0;

  const font: TrueTypeFont = {
    sfnt: b,
    postscriptName: psName,
    unitsPerEm: upm,
    ascent: r.i16(hhea + 4),
    descent: r.i16(hhea + 6),
    capHeight,
    italicAngle: italic,
    bbox: [r.i16(head + 36), r.i16(head + 38), r.i16(head + 40), r.i16(head + 42)],
    numGlyphs,
    glyph: (cp) => map.get(cp) ?? 0,
    advance: (gid) => advances[gid] ?? 0,
    isMark: (gid) => markSet.has(gid),
    markBase: [],
    markMark: [],
  };
  const markSet = new Set<number>();
  if (dir.GPOS) {
    try {
      readGPOS(r, dir.GPOS.off, font);
    } catch (e) {
      font.markBase = [];
      font.markMark = [];
    }
  }
  for (const l of [...font.markBase, ...font.markMark]) l.marks.forEach((_, g) => markSet.add(g));
  return font;
}

/** A glyph placed on a line: x/y in font units relative to the line start / baseline. */
export interface PlacedGlyph {
  gid: number;
  x: number;
  y: number;
  /** The text this glyph stands for (ToUnicode). */
  text: string;
}

const isThaiAboveTone = (cp: number) => cp >= 0x0e48 && cp <= 0x0e4b;

/**
 * Lays out one line: glyphs from the cmap, advances, and marks attached with the GPOS anchors.
 * Returns the glyphs and the total advance (font units).
 */
export function layout(font: TrueTypeFont, s: string): { glyphs: PlacedGlyph[]; width: number } {
  // code points; SARA AM → NIKHAHIT + SARA AA, NIKHAHIT moved before a preceding tone mark
  const cps: number[] = [];
  for (const ch of s) {
    const cp = ch.codePointAt(0) as number;
    if (cp === 0x0e33 && font.glyph(0x0e4d) && font.glyph(0x0e32)) {
      let at = cps.length;
      while (at > 0 && isThaiAboveTone(cps[at - 1])) at--;
      cps.splice(at, 0, 0x0e4d);
      cps.push(0x0e32);
    } else cps.push(cp === 9 ? 32 : cp);
  }
  const glyphs: PlacedGlyph[] = [];
  let pen = 0;
  let base = -1; // index of the last base glyph
  const fallback = font.glyph(0x3f);
  for (const cp of cps) {
    let gid = font.glyph(cp);
    if (!gid && cp >= 32) gid = fallback;
    const text = String.fromCodePoint(cp);
    const g: PlacedGlyph = { gid, x: pen, y: 0, text };
    if (font.isMark(gid) && glyphs.length) {
      const prev = glyphs[glyphs.length - 1];
      let placed = false;
      if (font.isMark(prev.gid)) {
        for (const l of font.markMark) {
          const m = l.marks.get(gid);
          const a = m && l.bases.get(prev.gid)?.[m[0]];
          if (m && a) {
            g.x = prev.x + a.x - m[1].x;
            g.y = prev.y + a.y - m[1].y;
            placed = true;
            break;
          }
        }
      }
      if (!placed && base >= 0) {
        const b = glyphs[base];
        for (const l of font.markBase) {
          const m = l.marks.get(gid);
          const a = m && l.bases.get(b.gid)?.[m[0]];
          if (m && a) {
            g.x = b.x + a.x - m[1].x;
            g.y = b.y + a.y - m[1].y;
            break;
          }
        }
      }
    } else base = glyphs.length;
    glyphs.push(g);
    pen += font.advance(gid);
  }
  return { glyphs, width: pen };
}
