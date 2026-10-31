// Byte helpers (goja has no TextEncoder / btoa).

/** UTF-8 bytes of a string. */
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

const B64 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

/** Standard base64 of bytes. */
export function base64(bytes: ArrayLike<number>): string {
  let out = '';
  for (let i = 0; i < bytes.length; i += 3) {
    const a = bytes[i];
    const b = i + 1 < bytes.length ? bytes[i + 1] : -1;
    const c = i + 2 < bytes.length ? bytes[i + 2] : -1;
    const n = (a << 16) | ((b < 0 ? 0 : b) << 8) | (c < 0 ? 0 : c);
    out += B64[(n >> 18) & 63] + B64[(n >> 12) & 63] + (b < 0 ? '=' : B64[(n >> 6) & 63]) + (c < 0 ? '=' : B64[n & 63]);
  }
  return out;
}

let CRC_TABLE: number[] | null = null;

/** CRC-32 (IEEE 802.3, as used by zip). */
export function crc32(bytes: ArrayLike<number>): number {
  if (!CRC_TABLE) {
    CRC_TABLE = [];
    for (let n = 0; n < 256; n++) {
      let c = n;
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
      CRC_TABLE.push(c >>> 0);
    }
  }
  let crc = 0xffffffff;
  for (let i = 0; i < bytes.length; i++) crc = CRC_TABLE[(crc ^ bytes[i]) & 0xff] ^ (crc >>> 8);
  return (crc ^ 0xffffffff) >>> 0;
}

/** ASCII85 (Adobe, with the "~>" end marker) of bytes — binary data inside a pure-ASCII PDF. */
export function ascii85(bytes: ArrayLike<number>): string {
  const parts: string[] = [];
  let line = '';
  const push = (s: string) => {
    line += s;
    if (line.length >= 76) {
      parts.push(line);
      line = '';
    }
  };
  const n = bytes.length;
  for (let i = 0; i < n; i += 4) {
    const k = Math.min(4, n - i);
    let v = 0;
    for (let j = 0; j < 4; j++) v = v * 256 + (j < k ? bytes[i + j] & 0xff : 0);
    if (v === 0 && k === 4) {
      push('z');
      continue;
    }
    const c: number[] = [];
    for (let j = 4; j >= 0; j--) {
      c[j] = (v % 85) + 33;
      v = Math.floor(v / 85);
    }
    push(String.fromCharCode.apply(null, c.slice(0, k + 1)));
  }
  parts.push(line + '~>');
  return parts.join('\n');
}
