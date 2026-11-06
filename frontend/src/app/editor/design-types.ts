import { tr } from '../core/i18n';
import './messages-editor';
import './messages-codeblock';

/**
 * The design document model, shared with backend/internal/design/node.go.
 * A page is a tree of nodes, like a Figma file; dynamic nodes (postList,
 * menu, postContent) pull content from the CMS like WordPress blocks.
 */
export type NodeType =
  | 'frame' | 'text' | 'rect' | 'image' | 'button'
  | 'menu' | 'postList' | 'postContent' | 'instance' | 'embed'
  | 'collection' // data from a plugin (built-in or service)
  | 'pluginBlock' // any plugin's block (props.plugin / props.block / props.values)
  | 'download' // file download button (props.mediaId)
  | 'breadcrumb' // Home › Category › Title, from the page context
  | 'icon' // line icon from the built-in set (props.icon)
  | 'shape' // decorative shape: section divider or blob (props.shape)
  | 'vector' // pen path / boolean result (props.path, props.viewBox) — see vector.ts
  | 'pagination' // page links of a collection (props.collection, props.style) — Site Code
  | 'input' // form field (props.kind, props.name) — filters and Site Code actions
  | 'codeblock'; // frontend code: props.html / css (scoped to .rea-cb-<id>) / js (runs on the site only) — needs edit_scripts

export type SizeMode = 'fixed' | 'fill' | 'hug';
export type LayoutMode = 'none' | 'horizontal' | 'vertical';
export type Breakpoint = 'desktop' | 'tablet' | 'mobile';

export interface Size {
  mode: SizeMode;
  value?: number;
}

export interface Layout {
  mode?: LayoutMode;
  gap?: number;
  padding?: [number, number, number, number];
  align?: 'start' | 'center' | 'end' | 'stretch';
  justify?: 'start' | 'center' | 'end' | 'between';
  wrap?: boolean;
  /** Horizontal layout as an even grid with this many columns (cards, galleries). */
  columns?: number;
}

export interface Style {
  fill?: string;
  backgroundImage?: string;
  stroke?: string;
  strokeWidth?: number;
  radius?: number;
  opacity?: number;
  shadow?: string;
  clip?: boolean;
  /** Look of a tab / accordion trigger while active (Prototype). */
  active?: { fill?: string; color?: string; stroke?: string };
  /** Menu links (2.2.0): [vertical, horizontal] padding and corner radius of every link. */
  linkPadding?: [number, number];
  linkRadius?: number;
  /** Clip the layer to a shape (Figma mask): a mask name from /design/assets. */
  mask?: string;
  /** Point kept in view when an image is cropped, in % (default 50/50). */
  focalX?: number;
  focalY?: number;
  /** Shared text style id (theme textStyles); the layer's own values win. */
  textStyle?: string;
  /** Stays at the top of the screen while the page scrolls (headers). */
  sticky?: boolean;
  color?: string;
  fontSize?: number;
  fontWeight?: number;
  fontFamily?: string;
  lineHeight?: number;
  letterSpacing?: number;
  textAlign?: 'left' | 'center' | 'right' | 'justify';
  textTransform?: string;
  maxWidth?: number;
  minHeight?: number;
  /** 2.2.0: min-height = the screen height (100svh) minus minHeightOffset px (a header above). */
  minHeightScreen?: boolean;
  minHeightOffset?: number;
  rotation?: number;
  /** Horizontal anchor of a freely placed layer (Figma constraints); see anchoredLeft(). */
  anchorX?: 'left' | 'center' | 'right' | 'scale';
  /** The parent's width (design px) when x was set — the reference for the anchor. */
  anchorRef?: number;
  objectFit?: 'cover' | 'contain' | 'fill';
  /** Per-corner radius: top-left, top-right, bottom-right, bottom-left (wins over radius). */
  radii?: [number, number, number, number];
  strokeStyle?: 'dashed' | 'dotted';
  /** Colour laid over the background image (e.g. rgba(0,0,0,.45) for readable text). */
  overlay?: string;
  /** Layer blur and "frosted glass" (blur of what is behind), px. */
  blur?: number;
  backdropBlur?: number;
  /** Image / layer adjustments in % (100 = unchanged; grayscale 0 = colour). */
  grayscale?: number;
  brightness?: number;
  contrast?: number;
  saturate?: number;
  italic?: boolean;
  textDecoration?: 'underline' | 'line-through';
  /** "x y blur color" */
  textShadow?: string;
  /** A CSS gradient painted into the letters. */
  textGradient?: string;
  /** Mouse-over state. */
  hover?: Hover;
  /** Scroll-in animation. */
  anim?: Anim;
  /** 2.2.2 (Figma parity): layer blend mode (CSS mix-blend-mode). */
  blend?: string;
  /** Scrolls inside the layer instead of growing / clipping. */
  scroll?: 'x' | 'y' | 'both';
  minWidth?: number;
  maxHeight?: number;
  /** Width / height ratio kept when the layer resizes, e.g. "16/9". */
  aspect?: string;
  /** Text cut after this many lines with "…". */
  maxLines?: number;
  /** Stroke on these sides only: top, right, bottom, left. */
  strokeSides?: [boolean, boolean, boolean, boolean];
}

/** What changes while the mouse is over a layer (published page). */
export interface Hover {
  scale?: number;
  /** Moves up by px. */
  lift?: number;
  fill?: string;
  color?: string;
  stroke?: string;
  shadow?: string;
  opacity?: number;
  /** Transition, ms (default 250). */
  duration?: number;
}

export type AnimType = 'fade' | 'up' | 'down' | 'left' | 'right' | 'zoom';
export interface Anim { type: AnimType; delay?: number; duration?: number }

export interface PostQuery {
  postType?: string;
  category?: string;
  limit?: number;
  order?: 'newest' | 'oldest' | 'title' | 'manual';
}

export interface Props {
  text?: string;
  tag?: string;
  src?: string;
  alt?: string;
  href?: string;
  /** Name for code (2.2.0): rendered as data-ref, found with anasaki.ref(name). */
  ref?: string;
  newTab?: boolean;
  componentId?: number;
  menuId?: number;
  query?: PostQuery;
  emptyText?: string;
  html?: string;
  /** codeblock: CSS scoped to the block (`.rea-cb-<id>`) and a module script run on the site with `el` + `anasaki`. */
  css?: string;
  js?: string;
  /** collection: data source provided by a plugin (e.g. "products"). */
  source?: string;
  /** pluginBlock: which plugin and block, and the block's prop values. */
  plugin?: string;
  block?: string;
  values?: Record<string, unknown>;
  /** download: media id (string so it can be a binding) and whether to show "PDF · 1.2 MB". */
  mediaId?: string;
  showSize?: boolean;
  /** text: formatted inline HTML (bold, colors, links) — `text` keeps the plain version. */
  rich?: string;
  /** breadcrumb: first link label, separator, color of the current page, hide the category step. */
  homeLabel?: string;
  separator?: string;
  currentColor?: string;
  hideCategory?: boolean;
  /** icon: name in the built-in set, line weight (1–4); shape: name; both: flips and an accessible label. */
  icon?: string;
  /** Button icon side (2.2.0): before the text (default) or 'end'. */
  iconPosition?: 'start' | 'end';
  /** Button icon size in px (2.3.0); unset = 1.3em, a little larger than the text. */
  iconSize?: number;
  strokeWidth?: number;
  shape?: string;
  flipX?: boolean;
  flipY?: boolean;
  label?: string;
  /** vector: SVG path data in viewBox coordinates ("0 0 w h"), line ends / corners, fill rule. */
  path?: string;
  viewBox?: string;
  strokeCap?: 'butt' | 'round' | 'square';
  strokeJoin?: 'miter' | 'round' | 'bevel';
  fillRule?: 'nonzero' | 'evenodd';
  /** menu: fold the links into a ☰ button on phones. */
  burger?: boolean;
  /** instance: this placement's changes to the component's inner layers, by layer id. */
  overrides?: Record<string, InstanceOverride>;
  /** component root: other designs of the component (Figma variants), copies of the main design. */
  variants?: { name: string; node: DNode }[];
  /** component root: name of the main design in the variant list. */
  variantName?: string;
  /** instance: which variant this placement shows ('' = main design). */
  variant?: string;
  /** On click (Prototype tab): open/close a popup, show/hide or tab to a layer, scroll to it — or call a site.action ('code'). */
  action?: NodeAction;
  /** Shown only when this expression is truthy (rendered server-side; `$empty` inside a collection = no items). */
  showWhen?: string;
  /** frame: a form — inputs inside are sent (by name) with a "Call code" action. */
  form?: boolean;
  /** collection (code:/table: sources): items per page, params (expressions), URL key, page change without reload, table query. */
  perPage?: number;
  params?: Record<string, string>;
  key?: string;
  noReload?: boolean;
  table?: TableQuery;
  /** pagination: the collection node it pages, look, numbers around the current page, labels, item looks. */
  collection?: string;
  style?: PaginationStyle;
  window?: number;
  labels?: { prev?: string; next?: string; more?: string };
  itemStyle?: PageItemStyle;
  activeStyle?: PageItemStyle;
  disabledStyle?: PageItemStyle;
  /** input: kind, field name, placeholder, value (expression), options (static or from a source), required, filter binding. */
  kind?: InputKind;
  name?: string;
  placeholder?: string;
  value?: string;
  /** The value is plain text, not an expression (2.2.0). */
  valueText?: boolean;
  options?: InputOption[];
  optionsFrom?: { collection: string; value: string; label: string };
  required?: boolean;
  filter?: { collection: string; param: string };
  /** frame: a modal shown by an "open" action (or after popupDelay seconds). */
  popup?: boolean;
  popupDelay?: number;
  popupOnce?: boolean;
  /** Hidden on the site until a toggle/tab action shows it. */
  collapsed?: boolean;
  /** frame: a horizontal slider of its children. */
  carousel?: { arrows?: boolean; dots?: boolean; autoplay?: number };
  /** pluginBlock: look of elements inside the block, by class name. */
  parts?: Record<string, PartStyle>;
  /** root: ruler guides in design px from the artboard's top-left (editor only, never rendered). */
  guides?: { x: number[]; y: number[] };
  /** frame: Figma layout grid drawn over the frame in the editor only. */
  layoutGrid?: LayoutGrid;
}

export type PrototypeActionType = 'open' | 'close' | 'toggle' | 'tab' | 'scroll';
/** Prototype action, or `type: 'code'` = POST /_a/{name} with `input` expressions (evaluated on the server). */
export interface NodeAction {
  type: PrototypeActionType | 'code';
  target?: string;
  name?: string;
  input?: Record<string, string>;
  confirm?: string;
  /** Node ids re-rendered with the response. */
  update?: string[];
  success?: { message?: string; redirect?: string };
}

export type PaginationStyle = 'numbers' | 'prevnext' | 'loadmore';
/** Look of one pagination state (normal / current / disabled). */
export interface PageItemStyle { fill?: string; color?: string; stroke?: string; radius?: number; padY?: number; padX?: number; fontSize?: number; fontWeight?: number }

export type InputKind = 'text' | 'search' | 'number' | 'email' | 'select' | 'checkbox' | 'textarea' | 'hidden';
export interface InputOption { value: string; label: string }

export type WhereOp = '=' | '!=' | '<' | '<=' | '>' | '>=' | 'like' | 'in' | 'null' | 'notnull';
/** table: sources — columns, filters (values are expressions) and order. */
export interface TableQuery {
  columns?: string[];
  where?: { column: string; op: WhereOp; value?: string }[];
  orderBy?: { column: string; dir: 'asc' | 'desc' }[];
}

/** Figma "Layout grid": columns / rows (count, gutter, margin) or a square grid (size). */
export interface LayoutGrid { type: 'columns' | 'rows' | 'grid'; count?: number; gutter?: number; margin?: number; size?: number; color?: string; visible?: boolean }

/** One inner layer of a component, changed for one placement only. */
export interface InstanceOverride { style?: Partial<Style>; text?: string; hidden?: boolean }

/** Look of an element inside a plugin block. */
export interface PartStyle {
  color?: string; fill?: string; fontSize?: number; fontWeight?: number; radius?: number; hidden?: boolean;
  fontFamily?: string; textAlign?: 'left' | 'center' | 'right' | 'justify'; lineHeight?: number; letterSpacing?: number;
  textTransform?: 'none' | 'uppercase' | 'lowercase' | 'capitalize'; padY?: number; padX?: number;
  stroke?: string; strokeWidth?: number; shadow?: string; opacity?: number;
  /** Look while the pointer is over the part. */
  hover?: { color?: string; fill?: string };
}

/** Per-breakpoint override. Narrower breakpoints inherit wider ones. */
export interface Override {
  x?: number;
  y?: number;
  width?: Size;
  height?: Size;
  visible?: boolean;
  layout?: Partial<Layout>;
  style?: Partial<Style>;
}

export interface DNode {
  id: string;
  type: NodeType;
  name?: string;
  visible?: boolean;
  locked?: boolean;
  /** Figma-style group: picked as one unit on the canvas (Ctrl+G). */
  group?: boolean;
  /** Floats inside an auto-layout parent (Figma "absolute position"). */
  absolute?: boolean;
  x?: number;
  y?: number;
  width: Size;
  height: Size;
  layout?: Layout;
  style?: Style;
  props?: Props;
  responsive?: { tablet?: Override; mobile?: Override };
  children?: DNode[];
}

/** screenH: the visible height of a typical browser window there (laptop, iPad, iPhone) — what
 *  "full screen height" layers (100svh) measure on the canvas. */
export const BREAKPOINTS: { id: Breakpoint; label: string; icon: string; width: number; screenH: number; max?: number }[] = [
  { id: 'desktop', get label() { return tr('ed.bp.desktop'); }, icon: '🖥', width: 1440, screenH: 800 },
  { id: 'tablet', get label() { return tr('ed.bp.tablet'); }, icon: '▭', width: 834, screenH: 1050, max: 1024 },
  { id: 'mobile', get label() { return tr('ed.bp.mobile'); }, icon: '📱', width: 390, screenH: 740, max: 640 },
];

export const CONTAINER_TYPES: NodeType[] = ['frame', 'postList', 'collection'];

export const isContainer = (n: DNode) => CONTAINER_TYPES.includes(n.type);

export function newId(): string {
  return Math.random().toString(36).slice(2, 10);
}

/** Deep copy with fresh ids (used for duplicate, paste, detach, presets). */
export function cloneWithNewIds(n: DNode): DNode {
  const c: DNode = structuredClone(n);
  const ids = new Map<string, string>();
  const walk = (x: DNode) => {
    const id = newId();
    ids.set(x.id, id);
    x.id = id;
    x.children?.forEach(walk);
  };
  walk(c);
  // Prototype actions (and pagination / filter / update links) inside the copy point at the copied layers
  const relink = (x: DNode) => {
    const p = x.props;
    if (p) {
      const t = p.action?.target;
      if (t && ids.has(t)) x.props = { ...x.props, action: { ...x.props!.action!, target: ids.get(t) } };
      const up = p.action?.update;
      if (up?.some(u => ids.has(u))) x.props = { ...x.props, action: { ...x.props!.action!, update: up.map(u => ids.get(u) ?? u) } };
      if (p.collection && ids.has(p.collection)) x.props = { ...x.props, collection: ids.get(p.collection) };
      if (p.filter && ids.has(p.filter.collection)) x.props = { ...x.props, filter: { ...p.filter, collection: ids.get(p.filter.collection)! } };
    }
    x.children?.forEach(relink);
  };
  relink(c);
  return c;
}

/** Gives every layer after the first that shares an id a fresh one (the server refuses
 *  duplicate ids since 2.1.1; pages made by scripts or imports may have them). Returns
 *  the tree unchanged when the ids are already unique. */
export function dedupeIds(root: DNode): { root: DNode; renamed: number } {
  const seen = new Set<string>();
  let renamed = 0;
  const walk = (x: DNode): DNode => {
    let n = x;
    if (x.id && seen.has(x.id)) {
      n = { ...x, id: newId() };
      renamed++;
    }
    if (n.id) seen.add(n.id);
    if (!n.children?.length) return n;
    const kids = n.children.map(walk);
    return kids.some((k, i) => k !== n.children![i]) ? { ...n, children: kids } : n;
  };
  const out = walk(root);
  return { root: renamed ? out : root, renamed };
}

export function findNode(root: DNode, id: string, parent: DNode | null = null): { node: DNode; parent: DNode | null; index: number } | null {
  if (root.id === id) return { node: root, parent, index: parent ? parent.children!.indexOf(root) : 0 };
  for (const c of root.children ?? []) {
    const r = findNode(c, id, root);
    if (r) return r;
  }
  return null;
}

export function isAncestor(root: DNode, ancestorId: string, id: string): boolean {
  const a = findNode(root, ancestorId);
  return !!a && !!findNode(a.node, id);
}

/** Applies breakpoint overrides (tablet, then mobile) — same as Node.Effective in Go. */
export function effective(n: DNode, bp: Breakpoint): DNode {
  if (bp === 'desktop' || !n.responsive) return n;
  const e: DNode = { ...n, layout: { ...n.layout }, style: { ...n.style } };
  const order: ('tablet' | 'mobile')[] = bp === 'tablet' ? ['tablet'] : ['tablet', 'mobile'];
  for (const b of order) {
    const o = n.responsive[b];
    if (!o) continue;
    if (o.x !== undefined) e.x = o.x;
    if (o.y !== undefined) e.y = o.y;
    if (o.visible !== undefined) e.visible = o.visible;
    if (o.width) e.width = o.width;
    if (o.height) e.height = o.height;
    if (o.layout) e.layout = mergeDefined(e.layout!, o.layout);
    if (o.style) e.style = mergeDefined(e.style!, o.style);
  }
  return e;
}

function mergeDefined<T extends object>(a: T, b: Partial<T>): T {
  const out = { ...a } as Record<string, unknown>;
  for (const [k, v] of Object.entries(b)) {
    if (v === null) delete out[k];
    else if (v !== undefined) out[k] = v;
  }
  return out as T;
}

/** Localized layer type names (`ed.node.*`), looked up on access so they follow the UI language. */
export const NODE_LABEL = new Proxy({} as Record<NodeType, string>, { get: (_, k) => tr('ed.node.' + String(k)) });

/** Localized one-line explanations (`ed.help.*`). */
export const NODE_HELP = new Proxy({} as Record<NodeType, string>, { get: (_, k) => tr('ed.help.' + String(k)) });

export const NODE_ICON: Record<NodeType, string> = {
  frame: '▢', text: 'T', rect: '■', image: '🖼', button: '⏺',
  menu: '☰', postList: '▤', postContent: '¶', instance: '◈', embed: '</>',
  collection: '▦', pluginBlock: '🧩', download: '⬇', breadcrumb: '»', icon: '★', shape: '〰', vector: '✎',
  pagination: '⋯', input: '⌨', codeblock: '{ }',
};

/** Root-to-node path ([root, …, node]); empty if not found. */
export function pathTo(root: DNode, id: string): DNode[] {
  if (root.id === id) return [root];
  for (const c of root.children ?? []) {
    const p = pathTo(c, id);
    if (p.length) return [root, ...p];
  }
  return [];
}

/** Layer-panel icon, Figma style: frames show their auto-layout direction. */
export function layerIcon(n: DNode): string {
  if (n.type === 'frame') {
    if (n.group) return '⬚';
    const m = n.layout?.mode;
    return m === 'horizontal' ? '⫼' : m === 'vertical' ? '☰' : '#';
  }
  if (n.type === 'rect') return (n.style?.radius ?? 0) >= 999 ? '◯' : '▭';
  return NODE_ICON[n.type];
}
