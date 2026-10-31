import { tr } from '../core/i18n';
import { createInput, createPagination } from './code-presets';
import { DNode, NodeType, newId } from './design-types';
import { siteRoleIds } from './site-role';
import './messages-editor';

/** Layer name in the user's language (presets are built when inserted). */
const L = (en: string) => tr('ln.' + en.replace(/ /g, '_'));
const fill = { mode: 'fill' as const };
const hug = { mode: 'hug' as const };
const fixed = (value: number) => ({ mode: 'fixed' as const, value });
type Pad = [number, number, number, number];

/** Default node for each element in the Assets panel (texts in the user's language). */
export function createNode(type: NodeType, opts: { menuId?: number; plugin?: string; block?: string; blockName?: string; values?: Record<string, unknown>; icon?: string; shape?: string } = {}): DNode {
  const id = newId();
  const name = tr('ed.node.' + type);
  switch (type) {
    case 'frame':
      return { id, type, name, width: fill, height: hug, layout: { mode: 'vertical', gap: 16, padding: [24, 24, 24, 24] }, style: { fill: '#f3f4f6', radius: 12 }, children: [] };
    case 'text':
      return { id, type, name, width: fill, height: hug, style: { fontSize: 18, color: '#111827' }, props: { text: tr('ed.def.text'), tag: 'p' } };
    case 'rect':
      return { id, type, name, width: fixed(160), height: fixed(100), style: { fill: '#c7d2fe', radius: 12 } };
    case 'icon':
      return { id, type, name, width: fixed(40), height: fixed(40), style: { color: 'var(--c-primary)' }, props: { icon: opts.icon ?? 'star' } };
    case 'shape': {
      const blob = !!opts.shape && /^(blob|circle|hexagon|star|dots)/.test(opts.shape);
      return blob
        ? { id, type, name, width: fixed(200), height: fixed(200), style: { fill: 'var(--c-primary)', opacity: 0.15 }, props: { shape: opts.shape } }
        : { id, type, name, width: fill, height: fixed(80), style: { fill: '#f3f4f6' }, props: { shape: opts.shape ?? 'wave' } };
    }
    case 'image':
      return { id, type, name, width: fill, height: fixed(280), style: { fill: '#e5e7eb', radius: 12 }, props: { src: '', alt: '' } };
    case 'button':
      return { id, type, name, width: hug, height: hug, layout: { padding: [12, 24, 12, 24] }, style: { fill: '#4f46e5', color: '#ffffff', radius: 10, fontWeight: 600, fontSize: 16 }, props: { text: tr('ed.def.button'), href: '#' } };
    case 'menu':
      return { id, type, name, width: hug, height: hug, layout: { mode: 'horizontal', gap: 24, align: 'center' }, style: { fontSize: 15, fontWeight: 500 }, props: { menuId: opts.menuId } };
    case 'postContent':
      return { id, type, name, width: fill, height: hug, style: { fontSize: 18 } };
    case 'embed':
      return { id, type, name, width: fill, height: hug, props: { html: `<div style="padding:24px;border:2px dashed #cbd5e1;border-radius:12px;text-align:center">${tr('ed.def.embed')}</div>` } };
    case 'codeblock':
      return { id, type, name, width: fill, height: hug, props: { html: tr('cb.def.html'), css: ':scope {\n  padding: 24px;\n}\n.hello {\n  font-weight: 600;\n}\n', js: tr('cb.def.js') } };
    case 'postList':
      return postList();
    case 'collection':
      return collectionGrid();
    case 'download':
      return { id, type, name, width: hug, height: hug, layout: { padding: [12, 22, 12, 22] }, style: { fill: '#0f766e', color: '#ffffff', radius: 10, fontWeight: 600, fontSize: 15 }, props: { text: '⬇ ' + tr('ed.def.download'), mediaId: '', showSize: true } };
    case 'breadcrumb':
      return breadcrumb();
    case 'pagination':
      return createPagination();
    case 'input':
      return createInput('text');
    case 'pluginBlock':
      return { id, type, name: opts.blockName || name, width: fill, height: hug, props: { plugin: opts.plugin ?? '', block: opts.block ?? '', values: opts.values ?? {} } };
    default:
      return { id, type: 'frame', width: fill, height: hug };
  }
}

function postList(): DNode {
  const style = CARD_STYLES[0];
  return {
    id: newId(), type: 'postList', name: tr('ed.node.postList'), width: fill, height: hug,
    layout: { ...style.layout }, props: { query: { postType: 'post', limit: 3, order: 'newest' }, emptyText: tr('ed.def.noPosts') },
    responsive: structuredClone(style.responsive),
    children: [style.card()],
  };
}

function breadcrumb(): DNode {
  return {
    id: newId(), type: 'breadcrumb', name: tr('ed.node.breadcrumb'), width: fill, height: hug,
    style: { fontSize: 14, color: 'var(--c-muted)' },
    props: { homeLabel: tr('ed.def.home'), separator: '›', currentColor: 'var(--c-primary)' },
  };
}

const txt = (name: string, text: string, style: DNode['style'], tag = 'p', href?: string): DNode =>
  ({ id: newId(), type: 'text', name, width: fill, height: hug, style, props: { text, tag, ...(href ? { href } : {}) } });

/**
 * Ready-made looks for a post list (Content tab → card style). Each one is the
 * repeated card plus the list layout; colors come from the site theme.
 */
export const CARD_STYLES: { id: string; icon: string; layout: NonNullable<DNode['layout']>; responsive: DNode['responsive']; card: () => DNode }[] = [
  {
    id: 'card', icon: '▦',
    layout: { mode: 'horizontal', gap: 24, columns: 3 },
    responsive: { tablet: { layout: { columns: 2 } }, mobile: { layout: { columns: 1 } } },
    card: () => ({
      id: newId(), type: 'frame', name: tr('ed.def.postCard'), width: fill, height: hug,
      layout: { mode: 'vertical' }, style: { fill: '#ffffff', radius: 16, clip: true, shadow: '0 4px 20px rgba(17,24,39,.08)' },
      children: [
        { id: newId(), type: 'image', name: L('Cover'), width: fill, height: fixed(200), style: { fill: '#e5e7eb' }, props: { src: '{{post.image}}', alt: '{{post.title}}', href: '{{post.url}}' } },
        {
          id: newId(), type: 'frame', name: L('Body'), width: fill, height: hug, layout: { mode: 'vertical', gap: 8, padding: [20, 20, 24, 20] },
          children: [
            txt('Category', '{{post.category}}', { fontSize: 13, fontWeight: 600, color: 'var(--c-primary)' }),
            txt('Title', '{{post.title}}', { fontSize: 19, fontWeight: 700, lineHeight: 1.35 }, 'h3', '{{post.url}}'),
            txt('Excerpt', '{{post.excerpt}}', { fontSize: 15, color: 'var(--c-muted)', lineHeight: 1.6 }),
            txt('Read more', tr('ed.def.readMore'), { fontSize: 14, fontWeight: 600, color: 'var(--c-primary)' }, 'p', '{{post.url}}'),
          ],
        },
      ],
    }),
  },
  {
    id: 'row', icon: '▭',
    layout: { mode: 'horizontal', gap: 24, columns: 2 },
    responsive: { tablet: { layout: { columns: 1 } } },
    card: () => ({
      id: newId(), type: 'frame', name: tr('ed.def.postCard'), width: fill, height: hug,
      layout: { mode: 'horizontal', gap: 20, align: 'center', padding: [16, 16, 16, 16] }, style: { fill: '#ffffff', radius: 16, stroke: '#e5e7eb', strokeWidth: 1 },
      responsive: { mobile: { layout: { mode: 'vertical', align: 'stretch' } } },
      children: [
        { id: newId(), type: 'image', name: L('Cover'), width: fixed(200), height: fixed(140), style: { fill: '#e5e7eb', radius: 12 }, props: { src: '{{post.image}}', alt: '{{post.title}}', href: '{{post.url}}' }, responsive: { mobile: { width: fill, height: fixed(180) } } },
        {
          id: newId(), type: 'frame', name: L('Body'), width: fill, height: hug, layout: { mode: 'vertical', gap: 6 },
          children: [
            txt('Title', '{{post.title}}', { fontSize: 18, fontWeight: 700, lineHeight: 1.35 }, 'h3', '{{post.url}}'),
            txt('Excerpt', '{{post.excerpt}}', { fontSize: 14, color: 'var(--c-muted)', lineHeight: 1.6 }),
            txt('Date', '{{post.date}}', { fontSize: 13, color: 'var(--c-muted)' }),
          ],
        },
      ],
    }),
  },
  {
    id: 'compact', icon: '☰',
    layout: { mode: 'vertical', gap: 12 },
    responsive: undefined,
    card: () => ({
      id: newId(), type: 'frame', name: tr('ed.def.postCard'), width: fill, height: hug,
      layout: { mode: 'horizontal', gap: 14, align: 'center' },
      children: [
        { id: newId(), type: 'image', name: L('Thumbnail'), width: fixed(72), height: fixed(72), style: { fill: '#e5e7eb', radius: 10 }, props: { src: '{{post.image}}', alt: '{{post.title}}', href: '{{post.url}}' } },
        {
          id: newId(), type: 'frame', name: L('Body'), width: fill, height: hug, layout: { mode: 'vertical', gap: 4 },
          children: [
            txt('Title', '{{post.title}}', { fontSize: 16, fontWeight: 600, lineHeight: 1.4 }, 'h3', '{{post.url}}'),
            txt('Date', '{{post.date}}', { fontSize: 13, color: 'var(--c-muted)' }),
          ],
        },
      ],
    }),
  },
  {
    id: 'minimal', icon: '≡',
    layout: { mode: 'vertical', gap: 0 },
    responsive: undefined,
    card: () => ({
      id: newId(), type: 'frame', name: tr('ed.def.postCard'), width: fill, height: hug,
      layout: { mode: 'vertical', gap: 6, padding: [18, 0, 18, 0] }, style: { stroke: '#e5e7eb', strokeWidth: 1 },
      children: [
        txt('Date', '{{post.date}} · {{post.category}}', { fontSize: 13, color: 'var(--c-muted)' }),
        txt('Title', '{{post.title}}', { fontSize: 20, fontWeight: 700, lineHeight: 1.35 }, 'h3', '{{post.url}}'),
      ],
    }),
  },
];

/** A grid over a plugin's collection ({{item.*}} bindings); pick the source in the Content tab. */
function collectionGrid(): DNode {
  return {
    id: newId(), type: 'collection', name: tr('ed.node.collection'), width: fill, height: hug,
    layout: { mode: 'horizontal', gap: 24, columns: 3 }, style: { maxWidth: 1180 },
    props: { source: '', query: { limit: 6, order: 'newest' }, emptyText: tr('ed.def.noItems') },
    responsive: { tablet: { layout: { columns: 2 } }, mobile: { layout: { columns: 1 } } },
    children: [{
      id: newId(), type: 'frame', name: tr('ed.def.itemCard'), width: fill, height: hug,
      layout: { mode: 'vertical' }, style: { fill: '#ffffff', radius: 16, clip: true, shadow: '0 4px 20px rgba(17,24,39,.08)' },
      children: [
        { id: newId(), type: 'image', name: L('Image'), width: fill, height: fixed(200), style: { fill: '#f3f4f6' }, props: { src: '{{item.image}}', alt: '{{item.title}}', href: '{{item.url}}' } },
        {
          id: newId(), type: 'frame', name: L('Body'), width: fill, height: hug, layout: { mode: 'vertical', gap: 8, padding: [16, 18, 20, 18] },
          children: [
            { id: newId(), type: 'text', name: L('Title'), width: fill, height: hug, style: { fontSize: 17, fontWeight: 700 }, props: { text: '{{item.title}}', tag: 'h3', href: '{{item.url}}' } },
            { id: newId(), type: 'text', name: L('Text'), width: fill, height: hug, style: { fontSize: 14, color: 'var(--c-muted)' }, props: { text: '{{item.text}}' } },
          ],
        },
      ],
    }],
  };
}

/**
 * A new page starts with the site's Header/Footer components (if any) around a section.
 * `settings` lets the site role settings decide which components those are (site-role.ts).
 */
export function starterPage(components: { id: number; name: string; siteRole?: string | null }[], kind: 'page' | 'template' = 'page', settings?: Record<string, string>): DNode {
  const roles = siteRoleIds(components, settings);
  const inst = (role: 'header' | 'footer'): DNode[] => {
    const c = components.find(x => x.id === roles[role]);
    return c ? [{ id: newId(), type: 'instance', name: c.name, width: fill, height: hug, props: { componentId: c.id } }] : [];
  };
  const body: DNode = kind === 'template'
    ? {
      id: newId(), type: 'frame', name: L('Article'), width: fill, height: hug, style: { maxWidth: 800 },
      layout: { mode: 'vertical', gap: 20, padding: [64, 24, 80, 24] },
      children: [
        { id: newId(), type: 'text', name: L('Title'), width: fill, height: hug, style: { fontSize: 42, fontWeight: 800 }, props: { text: '{{post.title}}', tag: 'h1' } },
        { id: newId(), type: 'postContent', name: tr('ed.node.postContent'), width: fill, height: hug, style: { fontSize: 18 } },
      ],
    }
    : {
      id: newId(), type: 'frame', name: L('Section'), width: fill, height: hug,
      layout: { mode: 'vertical', align: 'center', gap: 16, padding: [96, 24, 96, 24] },
      children: [
        { id: newId(), type: 'text', name: L('Heading'), width: fill, height: hug, style: { fontSize: 44, fontWeight: 800, textAlign: 'center' }, props: { text: tr('ed.def.newPage'), tag: 'h1' } },
        { id: newId(), type: 'text', name: L('Text'), width: fill, height: hug, style: { fontSize: 18, color: '#6b7280', textAlign: 'center', maxWidth: 620 }, props: { text: tr('ed.def.newPageText') } },
      ],
    };
  return {
    id: 'root', type: 'frame', name: L('Page'), width: fill, height: hug, layout: { mode: 'vertical' }, style: { fill: '#ffffff' },
    children: [...inst('header'), body, ...inst('footer')],
  };
}

/** Ready-made sections (WordPress "block patterns"). `name`/`desc` are i18n keys. */
export const PRESETS: { name: string; desc: string; icon: string; plugin?: string; build: () => DNode }[] = [
  {
    name: 'ed.preset.hero', desc: 'ed.preset.hero.d', icon: '🚀', build: () => ({
      id: newId(), type: 'frame', name: L('Hero'), width: fill, height: hug,
      layout: { mode: 'vertical', align: 'center', gap: 20, padding: [112, 24, 112, 24] },
      style: { fill: 'linear-gradient(135deg,#0ea5e9,#6366f1)', color: '#ffffff', textAlign: 'center' },
      responsive: { mobile: { layout: { padding: [64, 20, 64, 20] } } },
      children: [
        { id: newId(), type: 'text', name: L('Headline'), width: fill, height: hug, style: { fontSize: 56, fontWeight: 800, lineHeight: 1.1, maxWidth: 820 }, props: { text: tr('ed.def.heroTitle'), tag: 'h1' }, responsive: { mobile: { style: { fontSize: 34 } } } },
        { id: newId(), type: 'text', name: L('Subtitle'), width: fill, height: hug, style: { fontSize: 20, opacity: 0.9, maxWidth: 620, lineHeight: 1.6 }, props: { text: tr('ed.def.heroText') } },
        { id: newId(), type: 'button', name: L('CTA'), width: hug, height: hug, layout: { padding: [14, 30, 14, 30] }, style: { fill: '#ffffff', color: '#1e293b', radius: 999, fontWeight: 700 }, props: { text: tr('ed.def.start'), href: '#' } },
      ],
    }),
  },
  {
    name: 'ed.preset.cols', desc: 'ed.preset.cols.d', icon: '▥', build: () => ({
      id: newId(), type: 'frame', name: L('Features'), width: fill, height: hug,
      layout: { mode: 'horizontal', gap: 24, padding: [80, 24, 80, 24] }, style: { maxWidth: 1180 },
      responsive: { tablet: { layout: { mode: 'vertical' } } },
      children: [1, 2, 3].map(i => ({
        id: newId(), type: 'frame' as const, name: L('Column') + ' ' + i, width: fill, height: hug,
        layout: { mode: 'vertical' as const, gap: 10, padding: [28, 28, 28, 28] as Pad }, style: { fill: '#f8fafc', radius: 16, stroke: '#e2e8f0', strokeWidth: 1 },
        children: [
          { id: newId(), type: 'text' as const, name: L('Title'), width: fill, height: hug, style: { fontSize: 20, fontWeight: 700 }, props: { text: tr('ed.def.feature', { n: i }), tag: 'h3' } },
          { id: newId(), type: 'text' as const, name: L('Body'), width: fill, height: hug, style: { fontSize: 15, color: '#64748b', lineHeight: 1.65 }, props: { text: tr('ed.def.featureText') } },
        ],
      })),
    }),
  },
  {
    name: 'ed.preset.split', desc: 'ed.preset.split.d', icon: '◧', build: () => ({
      id: newId(), type: 'frame', name: L('Split'), width: fill, height: hug,
      layout: { mode: 'horizontal', gap: 48, align: 'center', padding: [80, 24, 80, 24] }, style: { maxWidth: 1180 },
      responsive: { tablet: { layout: { mode: 'vertical', gap: 28 } } },
      children: [
        { id: newId(), type: 'image', name: L('Image'), width: fill, height: fixed(380), style: { fill: '#e2e8f0', radius: 20 }, props: { src: '' } },
        {
          id: newId(), type: 'frame', name: L('Copy'), width: fill, height: hug, layout: { mode: 'vertical', gap: 16 },
          children: [
            { id: newId(), type: 'text', name: L('Heading'), width: fill, height: hug, style: { fontSize: 36, fontWeight: 800 }, props: { text: tr('ed.def.story'), tag: 'h2' } },
            { id: newId(), type: 'text', name: L('Body'), width: fill, height: hug, style: { fontSize: 17, color: '#475569', lineHeight: 1.7 }, props: { text: tr('ed.def.storyText') } },
          ],
        },
      ],
    }),
  },
  {
    name: 'ed.preset.blog', desc: 'ed.preset.blog.d', icon: '▤', build: () => {
      const list = postList();
      list.style = { maxWidth: 1180 };
      return {
        id: newId(), type: 'frame', name: L('Blog section'), width: fill, height: hug,
        layout: { mode: 'vertical', align: 'center', gap: 32, padding: [80, 24, 80, 24] }, style: { fill: '#f8fafc' },
        children: [
          { id: newId(), type: 'text', name: L('Heading'), width: hug, height: hug, style: { fontSize: 36, fontWeight: 800 }, props: { text: tr('ed.preset.blog'), tag: 'h2' } },
          list,
        ],
      };
    },
  },
  {
    name: 'ed.preset.cta', desc: 'ed.preset.cta.d', icon: '📣', build: () => ({
      id: newId(), type: 'frame', name: L('CTA'), width: fill, height: hug,
      layout: { mode: 'horizontal', justify: 'between', align: 'center', gap: 24, padding: [48, 56, 48, 56], wrap: true },
      style: { fill: '#111827', color: '#ffffff', radius: 24, maxWidth: 1180 },
      responsive: { mobile: { layout: { mode: 'vertical', padding: [36, 24, 36, 24] } } },
      children: [
        { id: newId(), type: 'text', name: L('Text'), width: hug, height: hug, style: { fontSize: 28, fontWeight: 700 }, props: { text: tr('ed.def.ctaText'), tag: 'h2' } },
        { id: newId(), type: 'button', name: L('Button'), width: hug, height: hug, layout: { padding: [14, 28, 14, 28] }, style: { fill: '#6366f1', color: '#ffffff', radius: 12, fontWeight: 700 }, props: { text: tr('ed.def.contact'), href: '#' } },
      ],
    }),
  },
];

/** Frame layouts offered at the top of Assets (Frame section). */
export type FrameKind = 'stack' | 'row' | 'cols2' | 'cols3' | 'cols4' | 'free';

export const FRAME_KINDS: { kind: FrameKind; icon: string }[] = [
  { kind: 'stack', icon: '☰' }, { kind: 'row', icon: '⫼' }, { kind: 'cols2', icon: '▥' },
  { kind: 'cols3', icon: '▦' }, { kind: 'cols4', icon: '▩' }, { kind: 'free', icon: '✥' },
];

/** A new frame of the given layout; column frames get one empty cell per column and stack on phones. */
export function createFrame(kind: FrameKind): DNode {
  const base = createNode('frame');
  const cell = (i: number): DNode => ({
    id: newId(), type: 'frame', name: `${tr('ed.frame.cell')} ${i + 1}`, width: fill, height: hug,
    layout: { mode: 'vertical', gap: 12, padding: [20, 20, 20, 20] }, style: { fill: '#ffffff', radius: 10, minHeight: 120 }, children: [],
  });
  switch (kind) {
    case 'row':
      return { ...base, name: tr('ed.frame.row'), layout: { mode: 'horizontal', gap: 16, align: 'center', padding: [24, 24, 24, 24] },
        responsive: { mobile: { layout: { mode: 'vertical' } } } };
    case 'cols2':
    case 'cols3':
    case 'cols4': {
      const n = Number(kind.slice(4));
      return {
        ...base, name: tr('ed.frame.cols', { n }), layout: { mode: 'horizontal', gap: 16, columns: n, padding: [24, 24, 24, 24] },
        responsive: { tablet: { layout: { columns: Math.min(n, 2) } }, mobile: { layout: { columns: 1 } } },
        children: Array.from({ length: n }, (_, i) => cell(i)),
      };
    }
    case 'free':
      return { ...base, name: tr('ed.frame.free'), height: { mode: 'fixed', value: 320 }, layout: { mode: 'none' } };
  }
  return { ...base, name: tr('ed.frame.stack') };
}
