import { EvalOptions, ExprCtx, renderTemplate } from './expr';
import { DNode, InstanceOverride, LayoutMode, PageItemStyle, PartStyle, Style } from './design-types';
import { safeImageSrc, safeUrl } from '../shared/safe-url';

/**
 * CSS for one node given its parent's layout mode ('' for the root).
 *
 * IMPORTANT: mirrors Decls() in backend/internal/design/css.go so the editor
 * canvas matches the published page. Keep both in sync.
 */
export function nodeCss(n: DNode, parentMode: LayoutMode | ''): Record<string, string> {
  if (n.absolute && parentMode !== '') parentMode = 'none'; // ignore parent's auto layout, like Figma
  const d: Record<string, string> = {};
  const lm = n.layout?.mode ?? 'none';
  const s = n.style ?? {};
  const maxW = s.maxWidth;
  const hasMaxW = maxW !== undefined && maxW !== null;

  if (parentMode === 'none') {
    d['position'] = 'absolute';
    d['left'] = anchoredLeft(n.x ?? 0, s.anchorX, s.anchorRef);
    d['top'] = px(n.y ?? 0);
  } else if (parentMode !== '' && s.sticky) {
    d['position'] = 'sticky'; // stays at the top while the page scrolls (headers)
    d['top'] = '0';
    d['z-index'] = '50';
  } else {
    d['position'] = 'relative';
  }

  switch (n.width.mode) {
    case 'fixed':
      d['width'] = px(n.width.value ?? 0);
      if (parentMode === 'horizontal') d['flex-shrink'] = '0';
      if (parentMode !== 'none') d['max-width'] = '100%';
      break;
    case 'fill':
      if (parentMode === 'horizontal') {
        d['flex-grow'] = '1';
        d['flex-basis'] = '0';
        d['min-width'] = '0';
      } else if (parentMode === 'vertical' && !hasMaxW) {
        d['align-self'] = 'stretch';
      } else {
        d['width'] = '100%';
      }
      break;
    case 'hug':
      if (parentMode === 'vertical') d['width'] = 'fit-content';
      else if (parentMode === 'none') d['width'] = 'max-content';
      break;
  }

  let fixedMin = 0; // fixed height rendered as min-height (see growsWithContent)
  switch (n.height.mode) {
    case 'fixed':
      if (growsWithContent(n, lm)) {
        // a fixed-height auto-layout frame grows when its content needs more room
        // (cards stacking on a phone) instead of spilling over the next section
        fixedMin = n.height.value ?? 0;
        d['min-height'] = px(fixedMin);
      } else {
        d['height'] = px(n.height.value ?? 0);
      }
      if (parentMode === 'vertical') d['flex-shrink'] = '0';
      break;
    case 'fill':
      if (parentMode === 'vertical') {
        d['flex-grow'] = '1';
        d['flex-basis'] = '0';
        d['min-height'] = '0';
      } else if (parentMode === 'horizontal') {
        d['align-self'] = 'stretch';
      } else {
        d['height'] = '100%';
      }
      break;
  }

  if (hasMaxW && maxW! > 0) {
    d['max-width'] = px(maxW!);
    if (parentMode !== 'none' && parentMode !== 'horizontal') {
      d['margin-left'] = 'auto';
      d['margin-right'] = 'auto';
    }
  }

  if (s.minHeight && s.minHeight > fixedMin) d['min-height'] = px(s.minHeight);
  // 2.2.0: at least the screen height (minus a header above); the artboard sets --rea-screen-h per breakpoint
  if (s.minHeightScreen) d['min-height'] = s.minHeightOffset && s.minHeightOffset > 0
    ? `calc(var(--rea-screen-h,100svh) - ${px(s.minHeightOffset)})` : 'var(--rea-screen-h,100svh)';

  if (n.type === 'button' || n.type === 'download') {
    d['display'] = 'inline-flex';
    d['align-items'] = 'center';
    d['justify-content'] = 'center';
    d['text-decoration'] = 'none';
    d['cursor'] = 'pointer';
  } else if (n.type === 'image') {
    d['display'] = 'block';
    d['object-fit'] = s.objectFit || 'cover';
    if (focal(s) !== 'center') d['object-position'] = focal(s);
  } else if (n.type === 'text') {
    d['white-space'] = 'pre-wrap';
    d['margin'] = '0';
  } else if (n.type === 'icon' || n.type === 'shape' || n.type === 'vector') {
    d['display'] = 'block';
  }

  const cols = columnsOf(n);
  if (cols) {
    // horizontal layout with a column count: an even grid
    d['display'] = 'grid';
    d['grid-template-columns'] = `repeat(${cols},minmax(0,1fr))`;
    const l = n.layout!;
    if (l.gap) d['gap'] = px(l.gap);
    if (l.align && ALIGN[l.align]) d['align-items'] = ALIGN[l.align];
  } else if (lm === 'horizontal' || lm === 'vertical') {
    d['display'] = 'flex';
    d['flex-direction'] = lm === 'horizontal' ? 'row' : 'column';
    const l = n.layout!;
    if (l.gap) d['gap'] = px(l.gap);
    if (l.align && ALIGN[l.align]) d['align-items'] = ALIGN[l.align];
    if (l.justify && JUSTIFY[l.justify]) d['justify-content'] = JUSTIFY[l.justify];
    if (l.wrap) d['flex-wrap'] = 'wrap';
  }
  const p = n.layout?.padding;
  if (p && p.length === 4 && p[0] + p[1] + p[2] + p[3] !== 0) {
    d['padding'] = p.map(px).join(' ');
  }

  if (s.fill && n.type !== 'vector') { // a vector paints its path (vector.ts), not the box
    if (n.type === 'shape') d['color'] = s.fill; // a shape is painted with currentColor
    else d['background'] = s.fill;
  }
  const bg = cssUrl(s.backgroundImage);
  if (bg) {
    d['background-image'] = bg;
    d['background-size'] = 'cover';
    d['background-position'] = focal(s);
  }
  const ov = cssSafe(s.overlay);
  if (ov && bg) d['background-image'] = `linear-gradient(${ov},${ov}),${bg}`;
  if ((s.strokeWidth ?? 0) > 0 && s.stroke && n.type !== 'vector') d['border'] = `${px(s.strokeWidth!)} ${s.strokeStyle === 'dashed' || s.strokeStyle === 'dotted' ? s.strokeStyle : 'solid'} ${s.stroke}`;
  if (s.radius) d['border-radius'] = px(s.radius);
  const rs = s.radii;
  if (rs && rs.length === 4 && rs[0] + rs[1] + rs[2] + rs[3] !== 0) d['border-radius'] = rs.map(px).join(' ');
  if (s.opacity !== undefined && s.opacity < 1) d['opacity'] = String(s.opacity);
  if (s.shadow) d['box-shadow'] = s.shadow;
  if (s.clip) d['overflow'] = 'hidden';
  const f = filterValue(s);
  if (f) d['filter'] = f;
  if (s.backdropBlur && s.backdropBlur > 0) {
    d['backdrop-filter'] = `blur(${px(s.backdropBlur)})`;
    d['-webkit-backdrop-filter'] = `blur(${px(s.backdropBlur)})`;
  }

  if (s.color) d['color'] = s.color;
  if (s.fontSize) d['font-size'] = px(s.fontSize);
  if (s.fontWeight) d['font-weight'] = String(s.fontWeight);
  if (s.fontFamily) { const f = fontStack(s.fontFamily); if (f) d['font-family'] = f; }
  if (s.lineHeight) d['line-height'] = String(s.lineHeight);
  if (s.letterSpacing) d['letter-spacing'] = px(s.letterSpacing);
  if (s.textAlign) d['text-align'] = s.textAlign;
  if (s.textTransform) d['text-transform'] = s.textTransform;
  if (s.italic) d['font-style'] = 'italic';
  if (s.textDecoration === 'underline' || s.textDecoration === 'line-through') d['text-decoration'] = s.textDecoration;
  if (s.textShadow) d['text-shadow'] = s.textShadow;
  if (s.textGradient) {
    d['background'] = s.textGradient;
    d['-webkit-background-clip'] = 'text';
    d['background-clip'] = 'text';
    d['-webkit-text-fill-color'] = 'transparent';
  }
  if (s.anim && ANIM_TYPES.includes(s.anim.type)) {
    if (s.anim.duration && s.anim.duration > 0) d['--anim-dur'] = `${s.anim.duration}ms`;
    if (s.anim.delay && s.anim.delay > 0) d['--anim-delay'] = `${s.anim.delay}ms`;
  }
  if (s.hover && Object.keys(s.hover).length) {
    const t = `${s.hover.duration ?? 250}ms ease`;
    d['transition'] = `transform ${t},box-shadow ${t},background ${t},color ${t},opacity ${t},border-color ${t}`;
  }

  const tf = baseTransform(n);
  if (tf.length) d['transform'] = tf.join(' ');
  if (n.visible === false) d['display'] = 'none';
  return d;
}

export const ANIM_TYPES = ['fade', 'up', 'down', 'left', 'right', 'zoom'];

/** Layer blur + image adjustments as a CSS filter. Mirrors filterValue() in css.go. */
export function filterValue(s: Partial<Style>): string {
  const parts: string[] = [];
  if (s.blur && s.blur > 0) parts.push(`blur(${px(s.blur)})`);
  if (s.grayscale && s.grayscale > 0) parts.push(`grayscale(${Math.min(s.grayscale, 100)}%)`);
  for (const k of ['brightness', 'contrast', 'saturate'] as const) {
    const v = s[k];
    if (v !== undefined && v >= 0 && v !== 100) parts.push(`${k}(${Math.min(v, 300)}%)`);
  }
  return parts.join(' ');
}

/** A layer's own rotation and flip. Mirrors baseTransform() in css.go. */
export function baseTransform(n: DNode): string[] {
  const tf: string[] = [];
  if (n.style?.rotation) tf.push(`rotate(${n.style.rotation}deg)`);
  const fx = !!n.props?.flipX, fy = !!n.props?.flipY;
  if (fx || fy) tf.push(`scale(${fx ? -1 : 1},${fy ? -1 : 1})`);
  return tf;
}

/** Declarations of the :hover state. Mirrors HoverDecls() in css.go. */
export function hoverCss(n: DNode): Record<string, string> {
  const h = n.style?.hover;
  const d: Record<string, string> = {};
  if (!h) return d;
  const tf = baseTransform(n); // keep the layer's own rotation / flip
  if (h.lift) tf.push(`translateY(${px(-h.lift)})`);
  if (h.scale && h.scale > 0 && h.scale !== 1) tf.push(`scale(${h.scale})`);
  if (tf.length) d['transform'] = tf.join(' ');
  if (h.fill) d['background'] = h.fill;
  if (h.color) d['color'] = h.color;
  if (h.stroke) d['border-color'] = h.stroke;
  if (h.shadow) d['box-shadow'] = h.shadow;
  if (h.opacity !== undefined && h.opacity >= 0 && h.opacity <= 1) d['opacity'] = String(h.opacity);
  return d;
}

/** Grid column count of a horizontal auto layout (layout.columns), 0 = a normal row. Mirrors design.Columns. */
export function columnsOf(n: DNode): number {
  if (n.layout?.mode !== 'horizontal') return 0;
  const c = n.layout.columns ?? 0;
  return c >= 1 ? Math.min(12, Math.floor(c)) : 0;
}

const ALIGN: Record<string, string> = { start: 'flex-start', center: 'center', end: 'flex-end', stretch: 'stretch' };
const JUSTIFY: Record<string, string> = { start: 'flex-start', center: 'center', end: 'flex-end', between: 'space-between' };

function px(v: number): string {
  return `${Math.round(v * 100) / 100}px`;
}

/**
 * A fixed height on an auto-layout container is a minimum, so content that
 * wraps or stacks on smaller screens is never cut off or overlapped. Freeform
 * frames and "Clip content" keep the exact height. Mirrors css.go.
 */
function growsWithContent(n: DNode, layoutMode: string): boolean {
  return ['frame', 'postList', 'collection'].includes(n.type) && layoutMode !== 'none' && !n.style?.clip;
}

/**
 * CSS left of a freely placed layer. x is measured from the parent's left edge
 * in a parent that was `ref` px wide in the designer; the anchor keeps the
 * layer at the same distance from the parent's centre / right edge (or at the
 * same fraction for "scale") when the real page is wider or narrower.
 * Mirrors anchoredLeft() in backend/internal/design/css.go.
 */
export function anchoredLeft(x: number, anchor?: string, ref?: number): string {
  if (!ref || ref <= 0) return px(x);
  const off = (base: string, v: number) => {
    v = Math.round(v * 100) / 100;
    return v === 0 ? base : `calc(${base} ${v < 0 ? '-' : '+'} ${px(Math.abs(v))})`;
  };
  switch (anchor) {
    case 'center': return off('50%', x - ref / 2);
    case 'right': return off('100%', x - ref);
    case 'scale': return `${Math.round((x / ref) * 10000) / 100}%`;
  }
  return px(x);
}

/** Binding context: site/post/item as before, plus Site Code's page data, URL query, $index and $empty. */
export interface BindCtx {
  site: Record<string, unknown>;
  post?: Record<string, unknown> | null;
  item?: Record<string, unknown> | null;
  page?: Record<string, unknown> | null;
  query?: Record<string, unknown> | null;
  /** Site visitor of the preview (site.visitor; null = not signed in). */
  visitor?: Record<string, unknown> | null;
  index?: number;
  empty?: boolean;
  /** Month names and time zone of date formatting (site language / timezone). */
  opts?: EvalOptions;
}

/** The expression context of a BindCtx (same names as the Go renderer). */
export function exprCtx(ctx: BindCtx): ExprCtx {
  const c: ExprCtx = { site: ctx.site, post: ctx.post ?? null, item: ctx.item ?? null, page: ctx.page ?? {}, query: ctx.query ?? {}, visitor: ctx.visitor ?? null };
  if (ctx.index !== undefined) c['$index'] = ctx.index;
  c['$empty'] = !!ctx.empty;
  return c;
}

/**
 * `{{ expr }}` bindings, same as render.go: `{{post.title}}` paths keep
 * working as before; expressions and formatters per expr.ts.
 */
export function bind(text: string | undefined, ctx: BindCtx): string {
  if (!text) return '';
  return renderTemplate(text, exprCtx(ctx), ctx.opts);
}

/** Same as bind() for HTML templates (rich text): values are HTML-escaped. */
export function bindEscaped(html: string, ctx: BindCtx): string {
  return renderTemplate(html, exprCtx(ctx), ctx.opts, escapeHtml);
}

export function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

const RICH_TAGS = new Set(['b', 'strong', 'i', 'em', 'u', 's', 'br', 'span', 'a', 'mark', 'sup', 'sub']);
const RICH_DECL = /^\s*(color|background-color|font-weight)\s*:\s*(#[0-9a-f]{3,8}|rgba?\([0-9.,\s%]+\)|var\(--c-[a-z0-9-]+\)|[a-z]+|[1-9]00)\s*$/i;

/**
 * Keeps only the inline formatting the text toolbar writes (bold, italic,
 * underline, strike, links, colored spans, line breaks). Mirrors
 * design.SanitizeRich in Go; the browser's parser is used for robustness.
 */
export function sanitizeRich(html: string): string {
  const tpl = document.createElement('template');
  tpl.innerHTML = html;
  const out: string[] = [];
  const walk = (n: Node) => {
    if (n.nodeType === Node.TEXT_NODE) { out.push(escapeHtml(n.textContent ?? '').replace(/&quot;/g, '"')); return; }
    if (n.nodeType !== Node.ELEMENT_NODE) return;
    const el = n as HTMLElement;
    let tag = el.tagName.toLowerCase();
    if (tag === 'div' || tag === 'p') { // browsers wrap new lines in blocks
      if (out.length && !out[out.length - 1].endsWith('<br>')) out.push('<br>');
      el.childNodes.forEach(walk);
      return;
    }
    if (tag === 'font') tag = 'span'; // old execCommand color markup
    if (!RICH_TAGS.has(tag)) { el.childNodes.forEach(walk); return; }
    if (tag === 'br') { out.push('<br>'); return; }
    let attrs = '';
    if (tag === 'a') {
      const href = el.getAttribute('href') ?? '';
      attrs += ` href="${escapeHtml(safeUrl(href) ?? '#')}"`;
      if (el.getAttribute('target') === '_blank') attrs += ' target="_blank" rel="noopener"';
    }
    const decls: string[] = [];
    const color = el.getAttribute('color'); // <font color>
    if (color) decls.push('color:' + color);
    for (const d of (el.getAttribute('style') ?? '').split(';')) {
      const m = RICH_DECL.exec(d);
      if (m) decls.push(m[1].toLowerCase() + ':' + m[2]);
    }
    const keep = decls.filter(d => RICH_DECL.test(d));
    if (keep.length) attrs += ` style="${escapeHtml(keep.join(';'))}"`;
    if (tag === 'span' && !attrs) { el.childNodes.forEach(walk); return; } // bare span adds nothing
    out.push(`<${tag}${attrs}>`);
    el.childNodes.forEach(walk);
    out.push(`</${tag}>`);
  };
  tpl.content.childNodes.forEach(walk);
  return out.join('').replace(/(<br>)+$/, '');
}

/** True when rich HTML carries formatting beyond plain text and line breaks. */
export function hasFormatting(html: string): boolean {
  return /<(?!br)[a-z]/i.test(html);
}

// ------------------------------------------------------------------ theme (design tokens)

/** Theme color tokens, in display order; used as var(--c-<key>). Mirrors site.ThemeColorKeys. */
export const THEME_COLOR_KEYS = ['primary', 'secondary', 'accent', 'text', 'muted', 'surface'] as const;
export type ThemeColorKey = (typeof THEME_COLOR_KEYS)[number];
export interface Theme {
  colors: Record<ThemeColorKey, string>;
  fontHeading: string;
  fontBody: string;
  /** Extra named colors (Figma color styles): var(--c-<id>). */
  palette: PaletteColor[];
  /** Shared text looks (Figma text styles): style.textStyle = id. */
  textStyles: TextStyleDef[];
  /** Where tablet / mobile start (max-width px, 2.2.0); absent = 1024 / 640. Same rules as ParseTheme in theme.go. */
  breakpoints?: { tablet?: number; mobile?: number };
}
export interface PaletteColor { id: string; name: string; value: string }
export type TextStyleValues = Pick<Style, 'fontFamily' | 'fontSize' | 'fontWeight' | 'lineHeight' | 'letterSpacing' | 'textTransform' | 'italic' | 'textDecoration'>;
export interface TextStyleDef { id: string; name: string; style: TextStyleValues; tablet?: TextStyleValues; mobile?: TextStyleValues }

/** Mirrors site.DefaultTextStyles. */
export const DEFAULT_TEXT_STYLES: TextStyleDef[] = [
  { id: 'h1', name: 'Heading 1', style: { fontSize: 52, fontWeight: 800, lineHeight: 1.15, letterSpacing: -0.5 }, tablet: { fontSize: 42 }, mobile: { fontSize: 34 } },
  { id: 'h2', name: 'Heading 2', style: { fontSize: 38, fontWeight: 800, lineHeight: 1.2 }, tablet: { fontSize: 32 }, mobile: { fontSize: 28 } },
  { id: 'h3', name: 'Heading 3', style: { fontSize: 24, fontWeight: 700, lineHeight: 1.3 }, mobile: { fontSize: 21 } },
  { id: 'lead', name: 'Lead', style: { fontSize: 20, fontWeight: 400, lineHeight: 1.6 }, mobile: { fontSize: 17 } },
  { id: 'body', name: 'Body', style: { fontSize: 17, fontWeight: 400, lineHeight: 1.65 } },
  { id: 'small', name: 'Small', style: { fontSize: 14, fontWeight: 400, lineHeight: 1.5 } },
  { id: 'caption', name: 'Caption', style: { fontSize: 12, fontWeight: 700, letterSpacing: 1, textTransform: 'uppercase' } },
];
export const TEXT_STYLE_KEYS = ['fontFamily', 'fontSize', 'fontWeight', 'lineHeight', 'letterSpacing', 'textTransform', 'italic', 'textDecoration'] as const;
export const STYLE_ID = /^[a-z][a-z0-9-]{0,23}$/;

function cleanTextValues(v: unknown): TextStyleValues {
  const out: Record<string, unknown> = {};
  if (!v || typeof v !== 'object') return out;
  for (const [k, x] of Object.entries(v as Record<string, unknown>)) {
    if (!(TEXT_STYLE_KEYS as readonly string[]).includes(k)) continue;
    if (k === 'fontFamily') { if (typeof x === 'string' && /^[A-Za-z0-9 ]{1,40}$/.test(x)) out[k] = x; }
    else if (k === 'textTransform') { if (typeof x === 'string' && /^(none|uppercase|lowercase|capitalize)$/.test(x)) out[k] = x; }
    else if (k === 'textDecoration') { if (typeof x === 'string' && /^(none|underline|line-through)$/.test(x)) out[k] = x; }
    else if (k === 'italic') { if (typeof x === 'boolean') out[k] = x; }
    else if (typeof x === 'number' && x > -100 && x < 1000) out[k] = x;
  }
  return out as TextStyleValues;
}

/**
 * A node with its text style's values under its own (desktop and per
 * breakpoint). Mirrors withTextStyle() in styles.go.
 */
export function withTextStyle(n: DNode, styles: Map<string, TextStyleDef>): DNode {
  const ts = n.style?.textStyle ? styles.get(n.style.textStyle) : undefined;
  if (!ts) return n;
  const own = n.style ?? {};
  const out: DNode = { ...n, style: { ...ts.style, ...own }, responsive: { ...n.responsive } };
  for (const bp of ['tablet', 'mobile'] as const) {
    const vals = ts[bp];
    if (!vals) continue;
    const add: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(vals)) if (!(k in own) || (own as Record<string, unknown>)[k] === undefined) add[k] = v;
    if (!Object.keys(add).length) continue;
    const o = { ...out.responsive![bp] };
    o.style = { ...add, ...o.style };
    out.responsive![bp] = o;
  }
  return out;
}

export const DEFAULT_THEME_COLORS: Record<ThemeColorKey, string> = {
  primary: '#4f46e5', secondary: '#0ea5e9', accent: '#f59e0b', text: '#111827', muted: '#6b7280', surface: '#f8fafc',
};

/** Reads the "theme" setting (JSON); missing or invalid parts use the defaults. */
export function parseTheme(raw?: string): Theme {
  let t: Partial<Theme> = {};
  try { t = raw ? JSON.parse(raw) : {}; } catch { /* default theme */ }
  const colors = { ...DEFAULT_THEME_COLORS };
  for (const k of THEME_COLOR_KEYS) {
    const v = t.colors?.[k]?.trim();
    if (v && /^(#[0-9a-f]{3,8}|rgba?\([0-9.,\s%]+\))$/i.test(v)) colors[k] = v;
  }
  const font = (f?: string) => (f && /^[A-Za-z0-9 ]{1,40}$/.test(f) ? f : '');
  const seen = new Set<string>();
  const palette: PaletteColor[] = [];
  for (const c of Array.isArray(t.palette) ? t.palette : []) {
    const v = String(c?.value ?? '').trim();
    if (palette.length >= 40 || !STYLE_ID.test(c?.id ?? '') || seen.has(c.id) || c.id in DEFAULT_THEME_COLORS || !/^(#[0-9a-f]{3,8}|rgba?\([0-9.,\s%]+\))$/i.test(v)) continue;
    seen.add(c.id);
    palette.push({ id: c.id, name: String(c.name ?? '').slice(0, 40), value: v });
  }
  seen.clear();
  const textStyles: TextStyleDef[] = [];
  for (const x of Array.isArray(t.textStyles) ? t.textStyles : DEFAULT_TEXT_STYLES) {
    if (textStyles.length >= 40 || !STYLE_ID.test(x?.id ?? '') || seen.has(x.id)) continue;
    seen.add(x.id);
    const d: TextStyleDef = { id: x.id, name: String(x.name ?? '').slice(0, 40), style: cleanTextValues(x.style) };
    const tb = cleanTextValues(x.tablet), mb = cleanTextValues(x.mobile);
    if (Object.keys(tb).length) d.tablet = tb;
    if (Object.keys(mb).length) d.mobile = mb;
    textStyles.push(d);
  }
  const bp: { tablet?: number; mobile?: number } = {};
  const mob = Number(t.breakpoints?.mobile), tab = Number(t.breakpoints?.tablet);
  if (mob >= 480 && mob <= 900 && mob !== 640) bp.mobile = mob;
  if (tab >= (bp.mobile ?? 640) + 100 && tab <= 1600 && tab !== 1024) bp.tablet = tab;
  return { colors, fontHeading: font(t.fontHeading), fontBody: font(t.fontBody), palette, textStyles, ...(bp.mobile || bp.tablet ? { breakpoints: bp } : {}) };
}

/** CSS variables (and fonts) for the canvas, like Theme.CSS() on the site. */
export function themeStyle(t: Theme): Record<string, string> {
  const s: Record<string, string> = { color: 'var(--c-text)' };
  for (const k of THEME_COLOR_KEYS) s['--c-' + k] = t.colors[k];
  for (const c of t.palette) s['--c-' + c.id] = c.value;
  if (t.fontBody) s['font-family'] = `'${t.fontBody}', 'Noto Sans Thai', system-ui, sans-serif`;
  if (t.fontHeading) s['--font-heading'] = `'${t.fontHeading}', 'Noto Sans Thai', system-ui, sans-serif`;
  return s;
}

/** "var(--c-primary)" → "primary"; anything else → null. */
export function tokenOf(v?: string): ThemeColorKey | null {
  const m = /^var\(--c-([a-z]+)\)$/.exec(v?.trim() ?? '');
  return m && (THEME_COLOR_KEYS as readonly string[]).includes(m[1]) ? (m[1] as ThemeColorKey) : null;
}

/** Data you can insert into text; labels are i18n keys. */
export const BINDINGS = [
  { token: '{{site.title}}', label: 'ed.bind.siteTitle' },
  { token: '{{site.tagline}}', label: 'ed.bind.tagline' },
  { token: '{{post.title}}', label: 'ed.bind.postTitle' },
  { token: '{{post.excerpt}}', label: 'ed.bind.excerpt' },
  { token: '{{post.date}}', label: 'ed.bind.date' },
  { token: '{{post.author}}', label: 'ed.bind.author' },
  { token: '{{post.category}}', label: 'ed.bind.category' },
  { token: '{{post.url}}', label: 'ed.bind.url' },
  { token: '{{post.image}}', label: 'ed.bind.image' },
  { token: '{{item.title}}', label: 'ed.bind.itemTitle' },
  { token: '{{item.image}}', label: 'ed.bind.itemImage' },
  { token: '{{item.url}}', label: 'ed.bind.itemUrl' },
];

export const FONTS = ['Inter', 'Noto Sans Thai', 'Prompt', 'Sarabun', 'Kanit', 'IBM Plex Sans Thai', 'Poppins', 'Roboto', 'Playfair Display', 'Space Grotesk'];

const GENERIC_FONTS = new Set(['serif', 'sans-serif', 'monospace', 'cursive', 'fantasy', 'system-ui', 'ui-sans-serif', 'ui-serif',
  'ui-monospace', 'ui-rounded', 'emoji', 'math', 'fangsong', '-apple-system', 'blinkmacsystemfont']);
/** A fontFamily value split on commas, each entry cleaned like FontName (max 8). Mirrors fontList() in css.go. */
function fontList(v: string): string[] {
  const out: string[] = [];
  for (const raw of v.split(',')) {
    const p = raw.trim().replace(/^['"]+|['"]+$/g, '');
    if (GENERIC_FONTS.has(p.toLowerCase())) out.push(p.toLowerCase());
    else {
      const f = p.replace(/[^\p{L}\p{N} ._-]/gu, '').trim().slice(0, 80);
      if (f) out.push(f);
    }
    if (out.length === 8) break;
  }
  return out;
}
/** The CSS font-family of a fontFamily setting ("Kanit, system-ui" → 'Kanit', system-ui). Mirrors FontStack() in css.go. */
export function fontStack(v: string): string {
  const list = fontList(v);
  if (!list.length) return '';
  const parts = list.map(f => (GENERIC_FONTS.has(f.toLowerCase()) ? f : `'${f}'`));
  if (!GENERIC_FONTS.has(list[list.length - 1].toLowerCase())) parts.push('sans-serif');
  return parts.join(', ');
}

const loadedFonts = new Set(['Inter', 'Noto Sans Thai']);
/** Lazily loads a Google Font for the canvas (the public site does the same server-side): the first
 *  family of a stack, unless it is a generic / system one. */
export function ensureFont(value?: string) {
  const first = value ? fontList(value)[0] : undefined;
  const family = first && !GENERIC_FONTS.has(first.toLowerCase()) ? first : undefined;
  if (!family || loadedFonts.has(family)) return;
  loadedFonts.add(family);
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = `https://fonts.googleapis.com/css2?family=${encodeURIComponent(family).replace(/%20/g, '+')}:wght@300;400;500;600;700;800&display=swap`;
  document.head.appendChild(link);
}

// ------------------------------------------------------------------ instances & plugin block parts

/** Component layers as one placement shows them (props.overrides). Mirrors instanceChildren() in render.go. */
export function instanceChildren(children: DNode[] | undefined, ov: Record<string, InstanceOverride> | undefined): DNode[] | undefined {
  if (!children || !ov || !Object.keys(ov).length) return children;
  return children.map(c => {
    const o = ov[c.id];
    let n: DNode = c;
    if (o) {
      n = { ...c };
      if (o.style) n.style = { ...c.style, ...o.style };
      if (o.text !== undefined) { n.props = { ...c.props, text: o.text }; delete n.props.rich; }
      if (o.hidden) n.visible = false;
    }
    const kids = instanceChildren(c.children, ov);
    return kids === c.children ? n : { ...n, children: kids };
  });
}

/**
 * A free-form CSS value from document data (color, gradient, shadow) checked
 * before it goes into a stylesheet: anything that could end the declaration or
 * rule, open a comment, escape characters or load a resource is refused ('').
 */
export function cssSafe(v: unknown): string {
  if (typeof v !== 'string') return '';
  const s = v.trim();
  if (!s || s.length > 400) return '';
  // eslint-disable-next-line no-control-regex
  if (/[;{}<>\\@'"`\u0000-\u001f\u007f]|\/\*|\*\/|url\s*\(|image-set|image\s*\(|expression|binding|script:/i.test(s)) return '';
  let depth = 0;
  for (const ch of s) {
    if (ch === '(') depth++;
    else if (ch === ')' && --depth < 0) return '';
  }
  return depth === 0 ? s : '';
}

/**
 * `url('…')` for an image address, or '' when the address is not a safe link
 * (http(s), site path, inline raster image). Characters that could close the
 * string or the url() are percent-encoded.
 */
export function cssUrl(u: unknown): string {
  if (typeof u !== 'string') return '';
  const safe = safeImageSrc(u);
  if (!safe) return '';
  const enc = safe.replace(/[\s'"()\\]/g, c => '%' + c.charCodeAt(0).toString(16).toUpperCase().padStart(2, '0'));
  return `url('${enc}')`;
}

/** CSS of one plugin block part. Mirrors BlockParts() in css.go. */
export function partCss(p: PartStyle): string {
  const d: string[] = [];
  const color = cssSafe(p.color), fill = cssSafe(p.fill), stroke = cssSafe(p.stroke), shadow = cssSafe(p.shadow);
  if (p.hidden) d.push('display:none');
  if (color) d.push(`color:${color}`);
  if (fill) d.push(`background:${fill}`);
  if (p.fontSize && p.fontSize > 0) d.push(`font-size:${px(p.fontSize)}`);
  if (p.fontWeight && p.fontWeight > 0) d.push(`font-weight:${p.fontWeight}`);
  if (p.radius !== undefined && p.radius >= 0) d.push(`border-radius:${px(p.radius)}`);
  if (p.fontFamily && /^[A-Za-z0-9 ]{1,40}$/.test(p.fontFamily)) d.push(`font-family:'${p.fontFamily}', sans-serif`);
  if (p.textAlign && /^(left|center|right|justify)$/.test(p.textAlign)) d.push(`text-align:${p.textAlign}`);
  if (p.lineHeight && p.lineHeight > 0 && p.lineHeight < 10) d.push(`line-height:${p.lineHeight}`);
  if (p.letterSpacing !== undefined && p.letterSpacing > -20 && p.letterSpacing < 50) d.push(`letter-spacing:${px(p.letterSpacing)}`);
  if (p.textTransform && /^(none|uppercase|lowercase|capitalize)$/.test(p.textTransform)) d.push(`text-transform:${p.textTransform}`);
  if (p.padY !== undefined || p.padX !== undefined) d.push(`padding:${px(Math.max(0, p.padY ?? 0))} ${px(Math.max(0, p.padX ?? 0))}`);
  if (stroke) d.push(`border:${px(p.strokeWidth && p.strokeWidth > 0 ? p.strokeWidth : 1)} solid ${stroke}`);
  if (shadow) d.push(`box-shadow:${shadow}`);
  if (p.opacity !== undefined && p.opacity >= 0 && p.opacity < 1) d.push(`opacity:${p.opacity}`);
  return d.join(';');
}

export const PART_CLASS = /^[A-Za-z_][A-Za-z0-9_-]{0,60}$/;

/** Point of an image kept in view when cropped (focalX/Y in %). Mirrors focal() in css.go. */
export function focal(s: Style): string {
  if (s.focalX === undefined && s.focalY === undefined) return 'center';
  const c = (v: number | undefined) => Math.max(0, Math.min(100, v ?? 50));
  return `${c(s.focalX)}% ${c(s.focalY)}%`;
}

/** CSS url() of a mask shape. Mirrors MaskURL() in mask.go. */
export function maskUrl(sh: { viewBox: string; body: string } | undefined): string {
  if (!sh) return '';
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${sh.viewBox}" preserveAspectRatio="none">${sh.body.replace(/currentColor/g, '#000')}</svg>`;
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
}

/** Hover look of a plugin block part. Mirrors BlockParts() Hover in css.go. */
export function partHoverCss(p: PartStyle): string {
  const d: string[] = [];
  const color = cssSafe(p.hover?.color), fill = cssSafe(p.hover?.fill);
  if (color) d.push(`color:${color}`);
  if (fill) d.push(`background:${fill}`);
  return d.join(';');
}

// ------------------------------------------------------------------ Site Code nodes (pagination, input)

/** Default looks of the pagination states (merged under props.itemStyle / activeStyle / disabledStyle). */
export const PAGE_ITEM_DEFAULTS: Record<'item' | 'active' | 'disabled', PageItemStyle> = {
  item: { fill: '#ffffff', color: '#111827', stroke: '#e5e7eb', radius: 8, padY: 8, padX: 14, fontSize: 14, fontWeight: 500 },
  active: { fill: 'var(--c-primary)', color: '#ffffff', stroke: 'var(--c-primary)' },
  disabled: { color: '#9ca3af' },
};

/**
 * Declarations of one pagination state. `item` is complete (defaults filled);
 * `active` / `disabled` only list what differs from the item look.
 * Mirrors the pagination CSS of the Go renderer (see paginationCss).
 */
export function pageItemDecls(n: DNode, kind: 'item' | 'active' | 'disabled'): Record<string, string> {
  const own = kind === 'item' ? n.props?.itemStyle : kind === 'active' ? n.props?.activeStyle : n.props?.disabledStyle;
  const s: PageItemStyle = { ...PAGE_ITEM_DEFAULTS[kind], ...stripUndefined(own) };
  const d: Record<string, string> = {};
  if (s.fill) d['background'] = cssColor(s.fill);
  if (s.color) d['color'] = cssColor(s.color);
  if (s.stroke) d['border-color'] = cssColor(s.stroke);
  if (s.radius !== undefined) d['border-radius'] = px(s.radius);
  if (s.padY !== undefined || s.padX !== undefined) d['padding'] = `${px(s.padY ?? 8)} ${px(s.padX ?? 14)}`;
  if (s.fontSize) d['font-size'] = px(s.fontSize);
  if (s.fontWeight) d['font-weight'] = String(s.fontWeight);
  if (kind === 'disabled') { d['pointer-events'] = 'none'; d['opacity'] = d['opacity'] ?? '0.6'; }
  return d;
}

function stripUndefined<T extends object>(o: T | undefined): Partial<T> {
  const out: Partial<T> = {};
  for (const [k, v] of Object.entries(o ?? {})) if (v !== undefined && v !== null && v !== '') (out as Record<string, unknown>)[k] = v;
  return out;
}

/** Only colours and theme tokens get through (values go into a stylesheet). */
function cssColor(v: string): string {
  return /^(#[0-9a-f]{3,8}|rgba?\([0-9.,\s%]+\)|var\(--c-[a-z0-9-]+\)|[a-z]+)$/i.test(v.trim()) ? v.trim() : 'inherit';
}

const declText = (d: Record<string, string>) => Object.entries(d).map(([k, v]) => `${k}:${v}`).join(';');

/** The look of one menu link (2.2.0, mirrors menuLinkDecls + the active rule in render.go): link padding /
 *  radius, plus style.active when it is the current page's link (aria-current). */
export function menuLinkCss(n: DNode, current: boolean): Record<string, string> {
  const s = n.style ?? {};
  const d: Record<string, string> = {};
  const p = s.linkPadding;
  if (p && (p[0] > 0 || p[1] > 0)) d['padding'] = `${px(p[0] || 0)} ${px(p[1] || 0)}`;
  if (s.linkRadius && s.linkRadius > 0) d['border-radius'] = px(s.linkRadius);
  if (current && s.active) {
    if (s.active.fill) d['background'] = s.active.fill;
    if (s.active.color) d['color'] = s.active.color;
    if (s.active.stroke) d['border-color'] = s.active.stroke;
  }
  return d;
}

/** Same rule as SamePage in render.go: a site path equal to `path` (no query, trailing slash or encoding). */
export function samePage(link: string, path: string): boolean {
  if (!link.startsWith('/') || link.startsWith('//')) return false;
  const norm = (u: string) => { u = u.split(/[?#]/)[0]; try { u = decodeURIComponent(u); } catch { /* keep */ } return u.length > 1 ? u.replace(/\/+$/, '') : u; };
  return norm(link) === norm(path);
}

/**
 * The stylesheet of one pagination node, as the server writes it:
 * `.n-<id> .rea-pg-item{…}`, `[aria-current=page]{…}`, `.is-disabled{…}`.
 */
export function paginationCss(n: DNode): string {
  const c = `.n-${n.id}`;
  return `${c} .rea-pg-item{${declText(pageItemDecls(n, 'item'))}}`
    + `${c} .rea-pg-item[aria-current=page]{${declText(pageItemDecls(n, 'active'))}}`
    + `${c} .rea-pg-item.is-disabled{${declText(pageItemDecls(n, 'disabled'))}}`;
}

/** Shared base rules of the Site Code nodes (also in styles.scss under .rea-canvas and in the site's BaseCSS). */
export const SITE_CODE_BASE_CSS =
  '.rea-pagination{display:flex;flex-wrap:wrap;align-items:center;gap:6px}'
  + '.rea-pg-item{display:inline-flex;align-items:center;justify-content:center;min-width:2.5em;border:1px solid transparent;text-decoration:none;cursor:pointer;font:inherit;line-height:1.2}'
  + '.rea-pg-gap{padding:0 .25em;opacity:.6}'
  + '.rea-field{display:flex;align-items:center}'
  + '.rea-field-el{font:inherit;color:inherit;background:transparent;border:0;outline:0;width:100%;min-width:0;padding:0;margin:0}'
  + '.rea-field-el::placeholder{color:inherit;opacity:.55}'
  + 'textarea.rea-field-el{resize:vertical;min-height:4.5em}'
  + '.rea-check{gap:.5em;cursor:pointer}.rea-check .rea-field-el{width:auto}';

export interface PageLink { kind: 'prev' | 'next' | 'page' | 'gap' | 'more'; page?: number; active?: boolean; disabled?: boolean }

/**
 * Links of a pagination node for page `current` of `pages` (same list as the server):
 * numbers = prev, 1, …, window around current, …, last, next; prevnext = prev, next; loadmore = more.
 */
export function pageLinks(style: string | undefined, current: number, pages: number, window = 2): PageLink[] {
  pages = Math.max(1, pages);
  current = Math.min(Math.max(1, current), pages);
  const prev: PageLink = { kind: 'prev', page: current - 1, disabled: current <= 1 };
  const next: PageLink = { kind: 'next', page: current + 1, disabled: current >= pages };
  if (style === 'loadmore') return [{ kind: 'more', page: current + 1, disabled: current >= pages }];
  if (style === 'prevnext') return [prev, next];
  const w = Math.max(0, window);
  const out: PageLink[] = [prev];
  let last = 0;
  for (let p = 1; p <= pages; p++) {
    if (p === 1 || p === pages || Math.abs(p - current) <= w) {
      if (p - last > 1) out.push({ kind: 'gap' });
      out.push({ kind: 'page', page: p, active: p === current });
      last = p;
    }
  }
  out.push(next);
  return out;
}
