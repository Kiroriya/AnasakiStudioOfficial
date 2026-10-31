/**
 * Messages from sandboxed plugin iframes (dashboard widgets, code admin pages)
 * that use the admin UI kit (/ext-ui/kit.js):
 *   {type: 'ak:height', height}  — the document's height (auto-height frames)
 *   {type: 'ak:navigate', path}  — AK.open('/admin/…'): an admin page in the top window
 * The frames have an opaque origin, so a message is matched to its iframe by
 * window identity (event.source === iframe.contentWindow), never by origin.
 * Pure helpers, unit tested in plugin-frame.spec.ts.
 */

export type FrameMessage = { type: 'height'; height: number } | { type: 'navigate'; route: string };

/** Widget frames: never smaller than a line, never taller than a page. */
export const WIDGET_MIN_HEIGHT = 80;
export const WIDGET_MAX_HEIGHT = 2000;
/** Admin page frames grow to their content (but stay at least the default frame height). */
export const PAGE_MAX_HEIGHT = 50000;

/**
 * The router path of an admin URL path ("/admin/x/code/a:b?days=7" → "/x/code/a:b?days=7"),
 * or null for anything else: other site paths, absolute or protocol-relative URLs,
 * backslashes, control characters.
 */
export function adminRoute(path: unknown, adminBase: string): string | null {
  if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//') || path.length > 2048) return null;
  if (/[\\\u0000-\u001f\u007f]/.test(path)) return null;
  let u: URL;
  try {
    u = new URL(path, 'http://admin.invalid');
  } catch {
    return null;
  }
  if (u.origin !== 'http://admin.invalid') return null;
  const p = u.pathname + u.search + u.hash; // dot segments resolved
  const base = adminBase.endsWith('/') ? adminBase : adminBase + '/';
  let rest: string | null = null;
  if (base.length > 1 && p.startsWith(base)) rest = p.slice(base.length);
  else if (p === '/admin' || p.startsWith('/admin/') || p.startsWith('/admin?') || p.startsWith('/admin#')) rest = p.slice('/admin'.length).replace(/^\//, '');
  return rest === null ? null : '/' + rest;
}

/** A valid kit message (unknown types, bad heights and non-admin paths → null). */
export function readFrameMessage(data: unknown, adminBase: string): FrameMessage | null {
  if (!data || typeof data !== 'object') return null;
  const m = data as Record<string, unknown>;
  if (m['type'] === 'ak:height') {
    const h = Number(m['height']);
    return typeof m['height'] === 'number' && Number.isFinite(h) && h >= 0 ? { type: 'height', height: Math.ceil(h) } : null;
  }
  if (m['type'] === 'ak:navigate') {
    const route = adminRoute(m['path'], adminBase);
    return route ? { type: 'navigate', route } : null;
  }
  return null;
}

/** The iframe whose window sent the message (undefined when none of ours did). */
export function sourceFrame<T extends HTMLIFrameElement>(frames: Iterable<T>, source: MessageEventSource | null): T | undefined {
  if (!source) return undefined;
  for (const f of frames) if (f.contentWindow && f.contentWindow === source) return f;
  return undefined;
}

export function clampHeight(h: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, Math.ceil(h)));
}

/** The admin's base path ("/admin/" in production, "/" in development). */
export function adminBase(): string {
  return new URL(document.baseURI).pathname;
}
