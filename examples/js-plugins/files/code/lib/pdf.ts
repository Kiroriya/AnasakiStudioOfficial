// toPDF: a minimal PDF 1.4 with a title, paragraphs and a simple table, paginated on A4 / Letter.
// The file is pure ASCII (bytes ≥ 0x80 written as escapes, an embedded font as ASCII85), so it can be
// returned as a route's string body.
//
// Fonts: by default the built-in Helvetica (WinAnsi) — Latin-1 only, other characters become "?".
// With `spec.font` (a TrueType font: .ttf, or WOFF with uncompressed tables, e.g. the Sarabun font of
// this package via the `thaiPdf` export, or your own via site.asset) the text is written with an
// embedded CIDFontType2 font (Identity-H, ToUnicode map for copy/search), so Thai and any other script
// the font covers work. Thai vowels and tone marks are placed with the font's GPOS mark anchors; there
// is no GSUB shaping (ligatures / contextual forms).
import { ColumnSpec, normColumns, cellValue, text } from './columns';
import { ascii85 } from './bytes';
import { parseTTF, layout, TrueTypeFont } from './ttf';

export interface PdfTable {
  columns?: ColumnSpec[];
  rows: any[];
}

export interface PdfSpec {
  title?: string;
  /** Paragraphs (wrapped to the page width). */
  lines?: string[];
  table?: PdfTable;
  /** Body font size in points (default 10). */
  fontSize?: number;
  /** Default 'A4'. */
  pageSize?: 'A4' | 'letter';
  landscape?: boolean;
  /** Footer "page / pages" (default true). */
  pageNumbers?: boolean;
  /**
   * A TrueType font to embed (bytes of a .ttf / uncompressed .woff, or parseTTF() of them) — needed
   * for Thai. Without it the built-in Helvetica (Latin-1) is used.
   */
  font?: TrueTypeFont | ArrayLike<number> | ArrayBuffer;
}

/** A PDF string literal: Latin-1 only, ( ) \ escaped, 0x80-0xFF as octal. */
function pdfString(s: string): string {
  let out = '(';
  for (const ch of s) {
    const c = ch.codePointAt(0) as number;
    if (ch === '(' || ch === ')' || ch === '\\') out += '\\' + ch;
    else if (c >= 32 && c < 127) out += ch;
    else if (c >= 160 && c <= 255) out += '\\' + c.toString(8).padStart(3, '0');
    else if (c === 9) out += ' ';
    else out += '?';
  }
  return out + ')';
}

/** Rough Helvetica advance width (em) — good enough for wrapping and truncation. */
function charWidth(ch: string): number {
  if (/[ijl.,:;'|!]/.test(ch)) return 0.28;
  if (/[ftrI\-()\[\] ]/.test(ch)) return 0.33;
  if (/[mwMW]/.test(ch)) return 0.85;
  if (/[A-Z]/.test(ch)) return 0.68;
  return 0.556;
}

const num = (n: number) => (Math.round(n * 100) / 100).toString();
const hex4 = (n: number) => n.toString(16).toUpperCase().padStart(4, '0');

/** Thai (and general) combining marks: a line is never cut just before one. */
const isCombining = (ch: string) => /[ัิ-ฺ็-๎̀-ͯ]/.test(ch);

/** How text is measured and drawn: built-in Helvetica or an embedded TrueType font. */
interface Face {
  width(s: string, size: number): number;
  draw(x: number, y: number, s: string, size: number, bold: boolean): string;
}

const helvetica: Face = {
  width: (s, size) => [...s].reduce((w, ch) => w + charWidth(ch), 0) * size,
  draw: (x, y, s, size, bold) => `BT /${bold ? 'F2' : 'F1'} ${num(size)} Tf ${num(x)} ${num(y)} Td ${pdfString(s)} Tj ET`,
};

class TrueTypeFace implements Face {
  /** gid → text, for the ToUnicode map and the W array. */
  used = new Map<number, string>();
  constructor(public font: TrueTypeFont) {}
  width(s: string, size: number): number {
    return (layout(this.font, s).width * size) / this.font.unitsPerEm;
  }
  draw(x: number, y: number, s: string, size: number, bold: boolean): string {
    const { glyphs } = layout(this.font, s);
    const k = size / this.font.unitsPerEm;
    // bold: fill + stroke (the sample ships one weight)
    let out = `BT /F3 ${num(size)} Tf ${bold ? `2 Tr ${num(size * 0.03)} w` : '0 Tr'}`;
    let run = '';
    let expect = NaN;
    const flush = () => {
      if (run) out += ` <${run}> Tj`;
      run = '';
    };
    for (const g of glyphs) {
      if (!this.used.has(g.gid)) this.used.set(g.gid, g.text);
      if (g.x !== expect || g.y !== 0) {
        flush();
        out += ` 1 0 0 1 ${num(x + g.x * k)} ${num(y + g.y * k)} Tm`;
      }
      run += hex4(g.gid);
      expect = g.y === 0 ? g.x + this.font.advance(g.gid) : NaN;
    }
    flush();
    return out + ' ET';
  }
}

function wrap(face: Face, s: string, size: number, max: number): string[] {
  const out: string[] = [];
  for (const para of s.split(/\r?\n/)) {
    let line = '';
    for (const word of para.split(/(\s+)/)) {
      const next = line + word;
      if (line && face.width(next.trimEnd(), size) > max) {
        out.push(line.trimEnd());
        line = word.trimStart();
      } else line = next;
      while (face.width(line, size) > max) {
        let cut = line.length;
        while (cut > 1 && face.width(line.slice(0, cut), size) > max) cut--;
        while (cut > 1 && isCombining(line[cut])) cut--;
        out.push(line.slice(0, cut));
        line = line.slice(cut);
      }
    }
    out.push(line.trimEnd());
  }
  return out;
}

function fit(face: Face, s: string, size: number, max: number): string {
  if (face.width(s, size) <= max) return s;
  let cut = s.length;
  while (cut > 0 && (face.width(s.slice(0, cut) + '...', size) > max || isCombining(s[cut] ?? ''))) cut--;
  return s.slice(0, cut) + '...';
}

/** The font objects of an embedded TrueType face (Type0 → CIDFontType2 → descriptor, file, ToUnicode). */
function fontObjects(face: TrueTypeFace, first: number): string[] {
  const f = face.font;
  const k = 1000 / f.unitsPerEm;
  const gids = [...face.used.keys()].sort((a, b) => a - b);
  const w = gids.map((g) => `${g} [${Math.round(f.advance(g) * k)}]`).join(' ');
  const utf16 = (s: string) => {
    let h = '';
    for (let i = 0; i < s.length; i++) h += hex4(s.charCodeAt(i));
    return h;
  };
  let cmap = '/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n' +
    '/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n' +
    '1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n';
  const pairs = gids.filter((g) => g && face.used.get(g));
  for (let i = 0; i < pairs.length; i += 100) {
    const chunk = pairs.slice(i, i + 100);
    cmap += `${chunk.length} beginbfchar\n` + chunk.map((g) => `<${hex4(g)}> <${utf16(face.used.get(g) as string)}>`).join('\n') + '\nendbfchar\n';
  }
  cmap += 'endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend';
  // the encoded font file is cached on the (parsed, reusable) font object
  const cache = f as TrueTypeFont & { _a85?: string };
  const file = cache._a85 ?? (cache._a85 = ascii85(f.sfnt));
  const name = f.postscriptName;
  const bbox = f.bbox.map((v) => Math.round(v * k)).join(' ');
  return [
    `<< /Type /Font /Subtype /Type0 /BaseFont /${name} /Encoding /Identity-H /DescendantFonts [${first + 1} 0 R] /ToUnicode ${first + 4} 0 R >>`,
    `<< /Type /Font /Subtype /CIDFontType2 /BaseFont /${name} /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> ` +
      `/FontDescriptor ${first + 2} 0 R /CIDToGIDMap /Identity /DW 1000 /W [${w}] >>`,
    `<< /Type /FontDescriptor /FontName /${name} /Flags 4 /FontBBox [${bbox}] /ItalicAngle ${num(f.italicAngle)} ` +
      `/Ascent ${Math.round(f.ascent * k)} /Descent ${Math.round(f.descent * k)} /CapHeight ${Math.round(f.capHeight * k)} /StemV 80 /FontFile2 ${first + 3} 0 R >>`,
    `<< /Length ${file.length} /Length1 ${f.sfnt.length} /Filter /ASCII85Decode >>\nstream\n${file}\nendstream`,
    `<< /Length ${cmap.length} >>\nstream\n${cmap}\nendstream`,
  ];
}

function isFont(f: unknown): f is TrueTypeFont {
  return !!f && typeof f === 'object' && typeof (f as TrueTypeFont).glyph === 'function' && (f as TrueTypeFont).sfnt !== undefined;
}

/** The document as a PDF (ASCII string). / สร้าง PDF อย่างง่าย (ภาษาไทยได้เมื่อระบุ font) */
export function toPDF(spec: PdfSpec): string {
  let [W, H] = spec.pageSize === 'letter' ? [612, 792] : [595.28, 841.89];
  if (spec.landscape) [W, H] = [H, W];
  const tt = spec.font ? new TrueTypeFace(isFont(spec.font) ? spec.font : parseTTF(spec.font)) : null;
  const face: Face = tt ?? helvetica;
  const M = 50;
  const size = spec.fontSize ?? 10;
  const lead = size * (tt ? 1.6 : 1.4);
  const pages: string[][] = [[]];
  let y = H - M;
  const ops = () => pages[pages.length - 1];
  const newPage = () => {
    pages.push([]);
    y = H - M;
  };
  const need = (h: number) => {
    if (y - h < M + 20) newPage();
  };
  const textAt = (x: number, s: string, bold: boolean, sz: number) => ops().push(face.draw(x, y, s, sz, bold));

  if (spec.title) {
    for (const l of wrap(face, spec.title, 16, W - 2 * M)) {
      need(22);
      y -= 16;
      textAt(M, l, true, 16);
      y -= 6;
    }
    y -= 8;
  }
  for (const p of spec.lines ?? []) {
    for (const l of wrap(face, p, size, W - 2 * M)) {
      need(lead);
      y -= lead;
      textAt(M, l, false, size);
    }
    y -= size * 0.6;
  }
  if (spec.table) {
    const cols = normColumns(spec.table.rows, spec.table.columns);
    const total = cols.reduce((s, c) => s + (c.width ?? 15), 0) || 1;
    const xs: number[] = [];
    let x = M;
    for (const c of cols) {
      xs.push(x);
      x += ((c.width ?? 15) / total) * (W - 2 * M);
    }
    const colW = (i: number) => (i + 1 < xs.length ? xs[i + 1] : W - M) - xs[i] - 4;
    const header = () => {
      need(lead * 2);
      y -= lead;
      cols.forEach((c, i) => textAt(xs[i], fit(face, c.label ?? c.key, size, colW(i)), true, size));
      ops().push(`0.5 w ${num(M)} ${num(y - 4)} m ${num(W - M)} ${num(y - 4)} l S`);
      y -= 4;
    };
    header();
    for (const row of spec.table.rows) {
      if (y - lead < M + 20) {
        newPage();
        header();
      }
      y -= lead;
      cols.forEach((c, i) => textAt(xs[i], fit(face, text(cellValue(row, c)), size, colW(i)), false, size));
    }
  }

  // objects: 1 catalog, 2 pages, 3 Helvetica, 4 Helvetica-Bold, [5-9 embedded font], then page + content per page
  const n = pages.length;
  const first = tt ? 10 : 5;
  const contents = pages.map((p, i) => {
    const content = p.slice();
    if (spec.pageNumbers !== false) {
      const label = `${i + 1} / ${n}`;
      content.push(helvetica.draw(W - M - helvetica.width(label, 8), M / 2, label, 8, false));
    }
    return content.join('\n');
  });
  const objs: string[] = [];
  const kids = pages.map((_, i) => `${first + i * 2} 0 R`).join(' ');
  objs.push('<< /Type /Catalog /Pages 2 0 R >>');
  objs.push(`<< /Type /Pages /Kids [${kids}] /Count ${n} >>`);
  objs.push('<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>');
  objs.push('<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>');
  if (tt) objs.push(...fontObjects(tt, 5));
  const fonts = '/F1 3 0 R /F2 4 0 R' + (tt ? ' /F3 5 0 R' : '');
  contents.forEach((stream, i) => {
    objs.push(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ${num(W)} ${num(H)}] /Resources << /Font << ${fonts} >> >> /Contents ${first + 1 + i * 2} 0 R >>`);
    objs.push(`<< /Length ${stream.length} >>\nstream\n${stream}\nendstream`);
  });
  let out = '%PDF-1.4\n';
  const offsets: number[] = [];
  objs.forEach((o, i) => {
    offsets.push(out.length);
    out += `${i + 1} 0 obj\n${o}\nendobj\n`;
  });
  const xref = out.length;
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n`;
  for (const off of offsets) out += String(off).padStart(10, '0') + ' 00000 n \n';
  out += `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return out;
}
