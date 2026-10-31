import { registerMessages } from '../core/messages';

registerMessages({
  'url.unsafe': {
    th: 'ลิงก์นี้ใช้ไม่ได้ — ใช้ได้เฉพาะ http(s)://, mailto:, tel:, ที่อยู่ในเว็บ (/หน้า) หรือ #จุดยึด',
    en: 'This link is not allowed — use http(s)://, mailto:, tel:, a site path (/page) or an #anchor',
  },
});

const SAFE_SCHEMES = new Set(['http', 'https', 'mailto', 'tel']);

/** Decodes HTML character references (`&#106;`, `&colon;` …) without running anything. */
function decodeEntities(s: string): string {
  if (!s.includes('&')) return s;
  const doc = new DOMParser().parseFromString(`<!doctype html><body>${s.replace(/</g, '&lt;')}`, 'text/html');
  return doc.body.textContent ?? s;
}

/**
 * Returns the link cleaned up (trimmed, control characters removed) when it is
 * safe to put in an `href`: http(s), mailto:, tel:, a relative/site path or an
 * #anchor. Returns `null` for anything else (`javascript:`, `data:`,
 * `vbscript:`, unknown schemes), including obfuscated forms such as
 * `java\tscript:` or `&#106;avascript:`. An empty string stays empty.
 */
export function safeUrl(raw: string | null | undefined): string | null {
  // eslint-disable-next-line no-control-regex
  const cleaned = (raw ?? '').replace(/[\u0000-\u001f\u007f-\u009f]/g, '').trim();
  if (!cleaned) return '';
  // What the browser's URL parser would see: entities decoded (possibly twice-encoded),
  // whitespace/control/zero-width characters removed anywhere.
  let probe = cleaned;
  for (let i = 0; i < 3; i++) {
    const next = decodeEntities(probe);
    if (next === probe) break;
    probe = next;
  }
  // eslint-disable-next-line no-control-regex
  probe = probe.replace(/[\s\u0000-\u001f\u007f-\u009f​-‍⁠﻿]/g, '');
  const m = /^([a-z][a-z0-9+.-]*):/i.exec(probe);
  if (!m) return cleaned; // relative path, ?query, #anchor or //host
  return SAFE_SCHEMES.has(m[1].toLowerCase()) ? cleaned : null;
}

/** True when `safeUrl` accepts the link. */
export function isSafeUrl(raw: string | null | undefined): boolean {
  return safeUrl(raw) !== null;
}

/** Like `safeUrl` but also accepts inline raster images (`data:image/png|jpeg|gif|webp;…`) for `<img src>`. */
export function safeImageSrc(raw: string | null | undefined): string | null {
  const s = (raw ?? '').trim();
  if (/^data:image\/(png|jpe?g|gif|webp|avif);base64,[a-z0-9+/=\s]+$/i.test(s)) return s;
  return safeUrl(s);
}
