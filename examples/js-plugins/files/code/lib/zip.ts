// A tiny zip writer: "stored" entries (no compression), UTF-8 names, CRC-32. Enough for OOXML.
import { utf8, crc32 } from './bytes';

export interface ZipEntry {
  name: string;
  data: string | ArrayLike<number>;
}

/** A zip archive of the entries as bytes. */
export function zip(entries: ZipEntry[], date: Date = new Date(Date.UTC(2026, 0, 1))): Uint8Array {
  const out: number[] = [];
  const central: number[] = [];
  const u16 = (a: number[], v: number) => a.push(v & 0xff, (v >>> 8) & 0xff);
  const u32 = (a: number[], v: number) => a.push(v & 0xff, (v >>> 8) & 0xff, (v >>> 16) & 0xff, (v >>> 24) & 0xff);
  const dosTime = (date.getUTCHours() << 11) | (date.getUTCMinutes() << 5) | (date.getUTCSeconds() >> 1);
  const dosDate = (Math.max(0, date.getUTCFullYear() - 1980) << 9) | ((date.getUTCMonth() + 1) << 5) | date.getUTCDate();
  for (const e of entries) {
    const name = utf8(e.name);
    const data = typeof e.data === 'string' ? utf8(e.data) : e.data;
    const crc = crc32(data);
    const offset = out.length;
    // local file header (flag 0x0800 = UTF-8 names, method 0 = stored)
    u32(out, 0x04034b50); u16(out, 20); u16(out, 0x0800); u16(out, 0); u16(out, dosTime); u16(out, dosDate);
    u32(out, crc); u32(out, data.length); u32(out, data.length); u16(out, name.length); u16(out, 0);
    for (const b of name) out.push(b);
    for (let i = 0; i < data.length; i++) out.push(data[i]);
    // central directory entry
    u32(central, 0x02014b50); u16(central, 20); u16(central, 20); u16(central, 0x0800); u16(central, 0);
    u16(central, dosTime); u16(central, dosDate); u32(central, crc); u32(central, data.length); u32(central, data.length);
    u16(central, name.length); u16(central, 0); u16(central, 0); u16(central, 0); u16(central, 0); u32(central, 0);
    u32(central, offset);
    for (const b of name) central.push(b);
  }
  const cdOffset = out.length;
  for (const b of central) out.push(b);
  // end of central directory
  u32(out, 0x06054b50); u16(out, 0); u16(out, 0); u16(out, entries.length); u16(out, entries.length);
  u32(out, central.length); u32(out, cdOffset); u16(out, 0);
  return new Uint8Array(out);
}
