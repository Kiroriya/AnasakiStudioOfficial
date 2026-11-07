// A small QR Code encoder (ISO/IEC 18004): byte mode, error correction level M, versions 1-40,
// automatic mask selection by the standard penalty rules. No dependencies.
// Structure follows Project Nayuki's reference implementation (MIT).

/** Error-correction codewords per block, level M, by version (index 0 unused). */
const ECC_PER_BLOCK = [-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28];
/** Number of error-correction blocks, level M, by version. */
const NUM_BLOCKS = [-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49];
/** Format bits of level M (L = 1, M = 0, Q = 3, H = 2). */
const ECL_M_BITS = 0;

/** UTF-8 bytes of a string (goja has no TextEncoder). */
export function utf8(s: string): number[] {
  const out: number[] = [];
  for (let i = 0; i < s.length; i++) {
    let c = s.charCodeAt(i);
    if (c >= 0xd800 && c < 0xdc00 && i + 1 < s.length) {
      const d = s.charCodeAt(i + 1);
      if (d >= 0xdc00 && d < 0xe000) {
        c = 0x10000 + ((c - 0xd800) << 10) + (d - 0xdc00);
        i++;
      }
    }
    if (c < 0x80) out.push(c);
    else if (c < 0x800) out.push(0xc0 | (c >> 6), 0x80 | (c & 63));
    else if (c < 0x10000) out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
    else out.push(0xf0 | (c >> 18), 0x80 | ((c >> 12) & 63), 0x80 | ((c >> 6) & 63), 0x80 | (c & 63));
  }
  return out;
}

// ---------------------------------------------------------------- Reed–Solomon over GF(2^8), poly 0x11D

function gfMul(x: number, y: number): number {
  let z = 0;
  for (let i = 7; i >= 0; i--) {
    z = (z << 1) ^ ((z >>> 7) * 0x11d);
    z ^= ((y >>> i) & 1) * x;
  }
  return z;
}

/** Generator polynomial of the given degree (coefficients, highest first, leading 1 omitted). */
export function rsDivisor(degree: number): number[] {
  const r: number[] = new Array(degree).fill(0);
  r[degree - 1] = 1;
  let root = 1;
  for (let i = 0; i < degree; i++) {
    for (let j = 0; j < r.length; j++) {
      r[j] = gfMul(r[j], root);
      if (j + 1 < r.length) r[j] ^= r[j + 1];
    }
    root = gfMul(root, 0x02);
  }
  return r;
}

/** The error-correction codewords of data for a divisor (rsDivisor(n)). */
export function rsRemainder(data: number[], divisor: number[]): number[] {
  const r: number[] = divisor.map(() => 0);
  for (const b of data) {
    const factor = b ^ (r.shift() as number);
    r.push(0);
    divisor.forEach((coef, i) => (r[i] ^= gfMul(coef, factor)));
  }
  return r;
}

// ---------------------------------------------------------------- BCH codes of format and version info

/** 15 format bits for level M and a mask (0-7), already XORed with 0x5412. */
export function formatBits(mask: number): number {
  const data = (ECL_M_BITS << 3) | mask;
  let rem = data;
  for (let i = 0; i < 10; i++) rem = (rem << 1) ^ ((rem >>> 9) * 0x537);
  return ((data << 10) | rem) ^ 0x5412;
}

/** 18 version bits (versions 7-40). */
export function versionBits(version: number): number {
  let rem = version;
  for (let i = 0; i < 12; i++) rem = (rem << 1) ^ ((rem >>> 11) * 0x1f25);
  return (version << 12) | rem;
}

// ---------------------------------------------------------------- capacity

function rawDataModules(ver: number): number {
  let r = (16 * ver + 128) * ver + 64;
  if (ver >= 2) {
    const align = Math.floor(ver / 7) + 2;
    r -= (25 * align - 10) * align - 55;
    if (ver >= 7) r -= 36;
  }
  return r;
}

/** Data codewords of a version at level M. */
export function dataCodewords(ver: number): number {
  return Math.floor(rawDataModules(ver) / 8) - ECC_PER_BLOCK[ver] * NUM_BLOCKS[ver];
}

const getBit = (x: number, i: number) => ((x >>> i) & 1) !== 0;

// ---------------------------------------------------------------- the symbol

export interface QrMatrix {
  version: number;
  mask: number;
  size: number;
  /** modules[y][x], true = dark. */
  modules: boolean[][];
}

/**
 * Encodes text (UTF-8, byte mode) at level M. opts.mask forces a mask (0-7); opts.minVersion /
 * maxVersion bound the version. Throws when the data does not fit version 40.
 */
export function qrMatrix(text: string | number[], opts: { mask?: number; minVersion?: number; maxVersion?: number } = {}): QrMatrix {
  const bytes = typeof text === 'string' ? utf8(text) : text;
  const minV = opts.minVersion ?? 1, maxV = opts.maxVersion ?? 40;
  let ver = minV;
  let bitsNeeded = 0;
  for (; ; ver++) {
    if (ver > maxV) throw new Error(`qr: data too long (${bytes.length} bytes)`);
    const ccBits = ver <= 9 ? 8 : 16;
    bitsNeeded = 4 + ccBits + bytes.length * 8;
    if (bytes.length < 1 << ccBits && bitsNeeded <= dataCodewords(ver) * 8) break;
  }
  // data bits: mode 0100 (byte), character count, the bytes, terminator, padding
  const bits: number[] = [];
  const push = (val: number, len: number) => {
    for (let i = len - 1; i >= 0; i--) bits.push((val >>> i) & 1);
  };
  push(0x4, 4);
  push(bytes.length, ver <= 9 ? 8 : 16);
  for (const b of bytes) push(b, 8);
  const capBits = dataCodewords(ver) * 8;
  push(0, Math.min(4, capBits - bits.length));
  push(0, (8 - (bits.length % 8)) % 8);
  for (let pad = 0xec; bits.length < capBits; pad ^= 0xec ^ 0x11) push(pad, 8);
  const data: number[] = [];
  for (let i = 0; i < bits.length; i += 8) {
    let b = 0;
    for (let j = 0; j < 8; j++) b = (b << 1) | bits[i + j];
    data.push(b);
  }
  const codewords = addEccAndInterleave(data, ver);

  const size = ver * 4 + 17;
  const modules: boolean[][] = [];
  const isFn: boolean[][] = [];
  for (let i = 0; i < size; i++) {
    modules.push(new Array(size).fill(false));
    isFn.push(new Array(size).fill(false));
  }
  const setFn = (x: number, y: number, dark: boolean) => {
    modules[y][x] = dark;
    isFn[y][x] = true;
  };

  // function patterns: timing, finders, alignment, format (placeholder), version
  for (let i = 0; i < size; i++) {
    setFn(6, i, i % 2 === 0);
    setFn(i, 6, i % 2 === 0);
  }
  const finder = (x: number, y: number) => {
    for (let dy = -4; dy <= 4; dy++) {
      for (let dx = -4; dx <= 4; dx++) {
        const d = Math.max(Math.abs(dx), Math.abs(dy));
        const xx = x + dx, yy = y + dy;
        if (xx >= 0 && xx < size && yy >= 0 && yy < size) setFn(xx, yy, d !== 2 && d !== 4);
      }
    }
  };
  finder(3, 3);
  finder(size - 4, 3);
  finder(3, size - 4);
  const align = alignmentPositions(ver, size);
  const n = align.length;
  for (let i = 0; i < n; i++) {
    for (let j = 0; j < n; j++) {
      if ((i === 0 && j === 0) || (i === 0 && j === n - 1) || (i === n - 1 && j === 0)) continue;
      for (let dy = -2; dy <= 2; dy++) {
        for (let dx = -2; dx <= 2; dx++) setFn(align[i] + dx, align[j] + dy, Math.max(Math.abs(dx), Math.abs(dy)) !== 1);
      }
    }
  }
  const drawFormat = (mask: number) => {
    const fb = formatBits(mask);
    for (let i = 0; i <= 5; i++) setFn(8, i, getBit(fb, i));
    setFn(8, 7, getBit(fb, 6));
    setFn(8, 8, getBit(fb, 7));
    setFn(7, 8, getBit(fb, 8));
    for (let i = 9; i < 15; i++) setFn(14 - i, 8, getBit(fb, i));
    for (let i = 0; i < 8; i++) setFn(size - 1 - i, 8, getBit(fb, i));
    for (let i = 8; i < 15; i++) setFn(8, size - 15 + i, getBit(fb, i));
    setFn(8, size - 8, true); // the dark module
  };
  drawFormat(0);
  if (ver >= 7) {
    const vb = versionBits(ver);
    for (let i = 0; i < 18; i++) {
      const dark = getBit(vb, i);
      const a = size - 11 + (i % 3), b = Math.floor(i / 3);
      setFn(a, b, dark);
      setFn(b, a, dark);
    }
  }

  // codewords in the zigzag order
  let k = 0;
  for (let right = size - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5;
    for (let vert = 0; vert < size; vert++) {
      for (let j = 0; j < 2; j++) {
        const x = right - j;
        const upward = ((right + 1) & 2) === 0;
        const y = upward ? size - 1 - vert : vert;
        if (!isFn[y][x] && k < codewords.length * 8) {
          modules[y][x] = getBit(codewords[k >>> 3], 7 - (k & 7));
          k++;
        }
      }
    }
  }

  const applyMask = (mask: number) => {
    for (let y = 0; y < size; y++) {
      for (let x = 0; x < size; x++) {
        let inv: boolean;
        switch (mask) {
          case 0: inv = (x + y) % 2 === 0; break;
          case 1: inv = y % 2 === 0; break;
          case 2: inv = x % 3 === 0; break;
          case 3: inv = (x + y) % 3 === 0; break;
          case 4: inv = (Math.floor(x / 3) + Math.floor(y / 2)) % 2 === 0; break;
          case 5: inv = ((x * y) % 2) + ((x * y) % 3) === 0; break;
          case 6: inv = (((x * y) % 2) + ((x * y) % 3)) % 2 === 0; break;
          default: inv = (((x + y) % 2) + ((x * y) % 3)) % 2 === 0;
        }
        if (!isFn[y][x] && inv) modules[y][x] = !modules[y][x];
      }
    }
  };
  let mask = opts.mask ?? -1;
  if (mask < 0 || mask > 7) {
    let best = Infinity;
    for (let m = 0; m < 8; m++) {
      applyMask(m);
      drawFormat(m);
      const p = penalty(modules, size);
      if (p < best) {
        best = p;
        mask = m;
      }
      applyMask(m); // XOR again = undo
    }
  }
  applyMask(mask);
  drawFormat(mask);
  return { version: ver, mask, size, modules };
}

function alignmentPositions(ver: number, size: number): number[] {
  if (ver === 1) return [];
  const n = Math.floor(ver / 7) + 2;
  const step = Math.floor((ver * 8 + n * 3 + 5) / (n * 4 - 4)) * 2;
  const out = [6];
  for (let pos = size - 7; out.length < n; pos -= step) out.splice(1, 0, pos);
  return out;
}

function addEccAndInterleave(data: number[], ver: number): number[] {
  const numBlocks = NUM_BLOCKS[ver];
  const eccLen = ECC_PER_BLOCK[ver];
  const raw = Math.floor(rawDataModules(ver) / 8);
  const numShort = numBlocks - (raw % numBlocks);
  const shortLen = Math.floor(raw / numBlocks);
  const div = rsDivisor(eccLen);
  const blocks: number[][] = [];
  for (let i = 0, k = 0; i < numBlocks; i++) {
    const dat = data.slice(k, k + shortLen - eccLen + (i < numShort ? 0 : 1));
    k += dat.length;
    const ecc = rsRemainder(dat, div);
    if (i < numShort) dat.push(0);
    blocks.push(dat.concat(ecc));
  }
  const out: number[] = [];
  for (let i = 0; i < blocks[0].length; i++) {
    blocks.forEach((b, j) => {
      if (i !== shortLen - eccLen || j >= numShort) out.push(b[i]);
    });
  }
  return out;
}

// ---------------------------------------------------------------- mask penalty (ISO 18004 §7.8.3)

function penalty(m: boolean[][], size: number): number {
  let result = 0;
  const addHistory = (run: number, h: number[]) => {
    if (h[0] === 0) run += size; // light border before the symbol
    h.pop();
    h.unshift(run);
  };
  const countPatterns = (h: number[]) => {
    const n = h[1];
    const core = n > 0 && h[2] === n && h[3] === n * 3 && h[4] === n && h[5] === n;
    return (core && h[0] >= n * 4 && h[6] >= n ? 1 : 0) + (core && h[6] >= n * 4 && h[0] >= n ? 1 : 0);
  };
  const line = (get: (i: number) => boolean) => {
    let color = false, run = 0;
    const h = [0, 0, 0, 0, 0, 0, 0];
    for (let i = 0; i < size; i++) {
      if (get(i) === color) {
        run++;
        if (run === 5) result += 3;
        else if (run > 5) result++;
      } else {
        addHistory(run, h);
        if (!color) result += countPatterns(h) * 40;
        color = get(i);
        run = 1;
      }
    }
    if (color) {
      addHistory(run, h);
      run = 0;
    }
    run += size;
    addHistory(run, h);
    result += countPatterns(h) * 40;
  };
  for (let y = 0; y < size; y++) line((x) => m[y][x]);
  for (let x = 0; x < size; x++) line((y) => m[y][x]);
  let dark = 0;
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      if (m[y][x]) dark++;
      if (y < size - 1 && x < size - 1) {
        const c = m[y][x];
        if (c === m[y][x + 1] && c === m[y + 1][x] && c === m[y + 1][x + 1]) result += 3;
      }
    }
  }
  const total = size * size;
  result += (Math.ceil(Math.abs(dark * 20 - total * 10) / total) - 1) * 10;
  return result;
}

// ---------------------------------------------------------------- SVG

export interface SvgOptions {
  /** Width/height in px (default 256). */
  size?: number;
  /** Quiet zone in modules (default 4). */
  margin?: number;
  color?: string;
  background?: string;
  /** Forces a mask (testing). */
  mask?: number;
}

const attr = (s: string) => String(s).replace(/[^#a-zA-Z0-9(),.% -]/g, '');

/** A scalable SVG of the QR code of text (one <path>; horizontal runs merged). / รูป QR เป็น SVG */
export function qrSvg(text: string, opts: SvgOptions = {}): string {
  const q = qrMatrix(text, { mask: opts.mask });
  const margin = Math.max(0, Math.floor(opts.margin ?? 4));
  const dim = q.size + margin * 2;
  const px = Math.max(1, Math.floor(opts.size ?? 256));
  let d = '';
  for (let y = 0; y < q.size; y++) {
    for (let x = 0; x < q.size; ) {
      if (!q.modules[y][x]) {
        x++;
        continue;
      }
      let run = 1;
      while (x + run < q.size && q.modules[y][x + run]) run++;
      d += `M${x + margin} ${y + margin}h${run}v1h-${run}z`;
      x += run;
    }
  }
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${px}" height="${px}" viewBox="0 0 ${dim} ${dim}" shape-rendering="crispEdges">` +
    `<rect width="${dim}" height="${dim}" fill="${attr(opts.background ?? '#fff')}"/>` +
    `<path d="${d}" fill="${attr(opts.color ?? '#000')}"/></svg>`;
}
