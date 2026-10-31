import { tr } from '../core/i18n';
import { DNode, cloneWithNewIds, newId } from './design-types';
import './messages-components';

/**
 * Ready-made starting points for a new component (header, footer, sections,
 * cards…). Everything stays editable; colours come from the site theme so a
 * starter matches the site out of the box.
 */

const fill = { mode: 'fill' as const };
const hug = { mode: 'hug' as const };
const fixed = (value: number) => ({ mode: 'fixed' as const, value });
type Pad = [number, number, number, number];

export type StarterRole = 'header' | 'footer';
export interface Starter {
  id: string;
  group: 'header' | 'footer' | 'section' | 'block';
  icon: string;
  /** Header/footer starters can replace the site's header/footer. */
  role?: StarterRole;
  build: (menuId?: number) => DNode;
}

const frame = (name: string, layout: DNode['layout'], style: DNode['style'], children: DNode[], extra: Partial<DNode> = {}): DNode =>
  ({ id: newId(), type: 'frame', name, width: fill, height: hug, layout, style, children, ...extra });
const text = (name: string, t: string, style: DNode['style'], extra: Partial<DNode> & { tag?: string; href?: string } = {}): DNode => {
  const { tag = 'p', href, ...rest } = extra;
  return { id: newId(), type: 'text', name, width: hug, height: hug, style, props: { text: t, tag, ...(href ? { href } : {}) }, ...rest };
};
const icon = (name: string, ic: string, size: number, color: string, href?: string): DNode =>
  ({ id: newId(), type: 'icon', name, width: fixed(size), height: fixed(size), style: { color }, props: { icon: ic, ...(href ? { href } : {}) } });
const button = (name: string, t: string, style: DNode['style'], pad: Pad = [12, 22, 12, 22], href = '#'): DNode =>
  ({ id: newId(), type: 'button', name, width: hug, height: hug, layout: { padding: pad }, style: { fontWeight: 600, fontSize: 15, radius: 10, ...style }, props: { text: t, href } });
const menu = (menuId: number | undefined, style: DNode['style'], layout: DNode['layout'], burger = false): DNode =>
  ({ id: newId(), type: 'menu', name: tr('cs.n.menu'), width: hug, height: hug, layout, style, props: { menuId, ...(burger ? { burger: true } : {}) } });
const logo = (color = 'var(--c-text)', size = 22): DNode =>
  text(tr('cs.n.logo'), '◆ {{site.title}}', { fontSize: size, fontWeight: 800, color, letterSpacing: -0.5 }, { href: '/' });
const socials = (color: string): DNode =>
  frame(tr('cs.n.socials'), { mode: 'horizontal', gap: 14, align: 'center' }, {}, [
    icon('Facebook', 'facebook', 22, color, 'https://facebook.com/'),
    icon('Instagram', 'instagram', 22, color, 'https://instagram.com/'),
    icon('YouTube', 'youtube', 22, color, 'https://youtube.com/'),
    icon('LINE', 'line-chat', 22, color, 'https://line.me/'),
  ], { width: hug });
const contactRow = (ic: string, t: string, color: string, iconColor: string): DNode =>
  frame(tr('cs.n.contactRow'), { mode: 'horizontal', gap: 10, align: 'center' }, {}, [icon(ic, ic, 18, iconColor), text(tr('cs.n.text'), t, { fontSize: 14, color })], { width: hug });

export const STARTERS: Starter[] = [
  // ------------------------------------------------------------ headers
  {
    id: 'header-classic', group: 'header', role: 'header', icon: '▬',
    build: menuId => frame(tr('cs.header'), { mode: 'horizontal', justify: 'between', align: 'center', gap: 24, padding: [16, 48, 16, 48] },
      { fill: '#ffffff', shadow: '0 1px 0 rgba(17,24,39,.08)', sticky: true }, [
        logo(),
        frame(tr('cs.n.right'), { mode: 'horizontal', gap: 28, align: 'center' }, {}, [
          menu(menuId, { fontSize: 15, fontWeight: 500, color: 'var(--c-text)' }, { mode: 'horizontal', gap: 28, align: 'center' }, true),
          { ...button(tr('cs.n.cta'), tr('cs.t.contact'), { fill: 'var(--c-primary)', color: '#ffffff', radius: 999 }, [10, 20, 10, 20], '/contact'), responsive: { mobile: { visible: false } } },
        ], { width: hug }),
      ], { props: { tag: 'header' }, responsive: { tablet: { layout: { padding: [14, 24, 14, 24] } }, mobile: { layout: { padding: [12, 16, 12, 16] } } } }),
  },
  {
    id: 'header-centered', group: 'header', role: 'header', icon: '☰',
    build: menuId => frame(tr('cs.header'), { mode: 'vertical', align: 'center', gap: 14, padding: [24, 24, 18, 24] },
      { fill: '#ffffff', stroke: '#e5e7eb', strokeWidth: 1, textAlign: 'center' }, [
        logo('var(--c-text)', 28),
        menu(menuId, { fontSize: 15, fontWeight: 500, color: 'var(--c-muted)' }, { mode: 'horizontal', gap: 32, align: 'center' }, true),
      ], { props: { tag: 'header' }, responsive: { mobile: { layout: { mode: 'horizontal', justify: 'between', padding: [12, 16, 12, 16] } } } }),
  },
  {
    id: 'header-bar', group: 'header', role: 'header', icon: '▤',
    build: menuId => frame(tr('cs.header'), { mode: 'vertical' }, { fill: '#0b0f19', color: '#ffffff', sticky: true }, [
      { ...frame(tr('cs.n.topbar'), { mode: 'horizontal', justify: 'end', align: 'center', gap: 24, padding: [8, 48, 8, 48] }, { fill: 'var(--c-primary)' }, [
        contactRow('phone', '02-123-4567', '#ffffff', '#ffffff'),
        contactRow('mail', 'hello@example.com', '#ffffff', '#ffffff'),
      ]), responsive: { mobile: { visible: false } } },
      frame(tr('cs.n.main'), { mode: 'horizontal', justify: 'between', align: 'center', gap: 24, padding: [18, 48, 18, 48] }, {}, [
        logo('#ffffff'),
        menu(menuId, { fontSize: 15, fontWeight: 500, color: '#e5e7eb' }, { mode: 'horizontal', gap: 28, align: 'center' }, true),
      ], { responsive: { mobile: { layout: { padding: [12, 16, 12, 16] } } } }),
    ], { props: { tag: 'header' } }),
  },
  // ------------------------------------------------------------ footers
  {
    id: 'footer-columns', group: 'footer', role: 'footer', icon: '▥',
    build: menuId => frame(tr('cs.footer'), { mode: 'vertical', align: 'center', gap: 40, padding: [64, 24, 32, 24] }, { fill: '#0b0f19', color: '#9ca3af', fontSize: 14 }, [
      frame(tr('cs.n.columns'), { mode: 'horizontal', gap: 48 }, { maxWidth: 1120 }, [
        frame(tr('cs.n.brand'), { mode: 'vertical', gap: 14 }, {}, [
          logo('#ffffff', 20),
          text(tr('cs.n.tagline'), '{{site.tagline}}', { fontSize: 14, lineHeight: 1.7 }, { width: fill }),
          socials('#ffffff'),
        ]),
        frame(tr('cs.n.links'), { mode: 'vertical', gap: 12 }, {}, [
          text(tr('cs.n.colTitle'), tr('cs.t.menu'), { fontSize: 14, fontWeight: 700, color: '#ffffff', textTransform: 'uppercase', letterSpacing: 0.8 }),
          menu(menuId, { fontSize: 14, color: '#9ca3af' }, { mode: 'vertical', gap: 10 }),
        ]),
        frame(tr('cs.n.contact'), { mode: 'vertical', gap: 12 }, {}, [
          text(tr('cs.n.colTitle'), tr('cs.t.contact'), { fontSize: 14, fontWeight: 700, color: '#ffffff', textTransform: 'uppercase', letterSpacing: 0.8 }),
          contactRow('phone', '02-123-4567', '#9ca3af', 'var(--c-primary)'),
          contactRow('mail', 'hello@example.com', '#9ca3af', 'var(--c-primary)'),
          contactRow('map-pin', tr('cs.t.address'), '#9ca3af', 'var(--c-primary)'),
        ]),
      ], { responsive: { tablet: { layout: { mode: 'vertical', gap: 32 } } } }),
      { id: newId(), type: 'rect', name: tr('cs.n.divider'), width: fill, height: fixed(1), style: { fill: 'rgba(255,255,255,.12)', maxWidth: 1120 } },
      text(tr('cs.n.copyright'), '© 2026 {{site.title}} · ' + tr('cs.t.rights'), { fontSize: 13, opacity: 0.7 }),
    ], { props: { tag: 'footer' } }),
  },
  {
    id: 'footer-simple', group: 'footer', role: 'footer', icon: '▁',
    build: menuId => frame(tr('cs.footer'), { mode: 'horizontal', justify: 'between', align: 'center', gap: 24, padding: [28, 48, 28, 48] },
      { fill: 'var(--c-surface)', color: 'var(--c-muted)', fontSize: 14, stroke: '#e5e7eb', strokeWidth: 1 }, [
        text(tr('cs.n.copyright'), '© 2026 {{site.title}}', { fontSize: 14 }),
        menu(menuId, { fontSize: 14, color: 'var(--c-muted)' }, { mode: 'horizontal', gap: 24, align: 'center' }),
        socials('var(--c-muted)'),
      ], { props: { tag: 'footer' }, responsive: { mobile: { layout: { mode: 'vertical', gap: 16, padding: [28, 16, 28, 16] } } } }),
  },
  {
    id: 'footer-cta', group: 'footer', role: 'footer', icon: '▣',
    build: () => frame(tr('cs.footer'), { mode: 'vertical', align: 'center', gap: 20, padding: [72, 24, 28, 24] },
      { fill: 'linear-gradient(135deg,var(--c-primary) 0%,var(--c-secondary) 100%)', color: '#ffffff', textAlign: 'center' }, [
        text(tr('cs.n.heading'), tr('cs.t.ready'), { fontSize: 36, fontWeight: 800, lineHeight: 1.2 }, { tag: 'h2', responsive: { mobile: { style: { fontSize: 28 } } } }),
        text(tr('cs.n.text'), tr('cs.t.readyText'), { fontSize: 17, opacity: 0.9, maxWidth: 560 }, { width: fill }),
        button(tr('cs.n.cta'), tr('cs.t.contact') + ' →', { fill: '#ffffff', color: 'var(--c-primary)', radius: 999, fontWeight: 700 }, [14, 28, 14, 28], '/contact'),
        socials('#ffffff'),
        text(tr('cs.n.copyright'), '© 2026 {{site.title}} · ' + tr('cs.t.rights'), { fontSize: 13, opacity: 0.75 }, { layout: { padding: [24, 0, 0, 0] } }),
      ], { props: { tag: 'footer' } }),
  },
  // ------------------------------------------------------------ sections
  {
    id: 'hero', group: 'section', icon: '◭',
    build: () => frame(tr('cs.hero'), { mode: 'vertical', align: 'center', gap: 22, padding: [112, 24, 112, 24] },
      { fill: 'linear-gradient(135deg,var(--c-primary) 0%,var(--c-secondary) 100%)', color: '#ffffff', textAlign: 'center' }, [
        text(tr('cs.n.heading'), tr('cs.t.heroTitle'), { fontSize: 56, fontWeight: 800, lineHeight: 1.12, maxWidth: 880 }, { tag: 'h1', width: fill, responsive: { tablet: { style: { fontSize: 44 } }, mobile: { style: { fontSize: 34 } } } }),
        text(tr('cs.n.text'), tr('cs.t.heroText'), { fontSize: 19, lineHeight: 1.6, opacity: 0.9, maxWidth: 640 }, { width: fill }),
        frame(tr('cs.n.buttons'), { mode: 'horizontal', gap: 12, wrap: true, justify: 'center' }, {}, [
          button(tr('cs.n.cta'), tr('cs.t.start'), { fill: '#ffffff', color: 'var(--c-primary)', radius: 999, fontWeight: 700 }, [14, 28, 14, 28]),
          button(tr('cs.n.cta2'), tr('cs.t.more'), { fill: 'rgba(255,255,255,.12)', color: '#ffffff', radius: 999, stroke: 'rgba(255,255,255,.5)', strokeWidth: 1.5 }, [14, 28, 14, 28]),
        ], { width: hug }),
      ], { responsive: { mobile: { layout: { padding: [72, 20, 72, 20] } } } }),
  },
  {
    id: 'cta', group: 'section', icon: '▭',
    build: () => frame(tr('cs.cta'), { mode: 'horizontal', justify: 'between', align: 'center', gap: 24, padding: [40, 48, 40, 48] },
      { fill: 'var(--c-surface)', radius: 24, stroke: '#e5e7eb', strokeWidth: 1, maxWidth: 1120 }, [
        frame(tr('cs.n.texts'), { mode: 'vertical', gap: 8 }, {}, [
          text(tr('cs.n.heading'), tr('cs.t.ready'), { fontSize: 28, fontWeight: 800, color: 'var(--c-text)' }, { tag: 'h2', width: fill }),
          text(tr('cs.n.text'), tr('cs.t.readyText'), { fontSize: 16, color: 'var(--c-muted)' }, { width: fill }),
        ]),
        button(tr('cs.n.cta'), tr('cs.t.contact') + ' →', { fill: 'var(--c-primary)', color: '#ffffff', radius: 999 }, [14, 26, 14, 26], '/contact'),
      ], { responsive: { mobile: { layout: { mode: 'vertical', align: 'start', padding: [28, 24, 28, 24] } } } }),
  },
  {
    id: 'features', group: 'section', icon: '▦',
    build: () => {
      const card = (ic: string, title: string, desc: string) => frame(tr('cs.n.card'), { mode: 'vertical', gap: 12, padding: [28, 24, 28, 24] },
        { fill: '#ffffff', radius: 20, stroke: '#e5e7eb', strokeWidth: 1, hover: { lift: 6, shadow: '0 16px 40px rgba(17,24,39,.12)' } }, [
          frame(tr('cs.n.iconBox'), { mode: 'vertical', align: 'center', justify: 'center' }, { fill: 'var(--c-surface)', radius: 14 }, [icon(ic, ic, 26, 'var(--c-primary)')],
            { width: fixed(52), height: fixed(52) }),
          text(tr('cs.n.heading'), title, { fontSize: 19, fontWeight: 700, color: 'var(--c-text)' }, { tag: 'h3', width: fill }),
          text(tr('cs.n.text'), desc, { fontSize: 15, lineHeight: 1.65, color: 'var(--c-muted)' }, { width: fill }),
        ]);
      return frame(tr('cs.features'), { mode: 'vertical', align: 'center', gap: 40, padding: [88, 24, 88, 24] }, {}, [
        text(tr('cs.n.heading'), tr('cs.t.featuresTitle'), { fontSize: 36, fontWeight: 800, color: 'var(--c-text)', textAlign: 'center' }, { tag: 'h2', width: fill, responsive: { mobile: { style: { fontSize: 28 } } } }),
        frame(tr('cs.n.row'), { mode: 'horizontal', gap: 24 }, { maxWidth: 1120 }, [
          card('zap', tr('cs.t.f1'), tr('cs.t.f1d')), card('shield', tr('cs.t.f2'), tr('cs.t.f2d')), card('heart', tr('cs.t.f3'), tr('cs.t.f3d')),
        ], { responsive: { tablet: { layout: { mode: 'vertical' } } } }),
      ]);
    },
  },
  {
    id: 'contact', group: 'section', icon: '✉',
    build: () => frame(tr('cs.contact'), { mode: 'horizontal', gap: 48, padding: [72, 48, 72, 48] }, { fill: 'var(--c-surface)' }, [
      frame(tr('cs.n.texts'), { mode: 'vertical', gap: 12 }, {}, [
        text(tr('cs.n.heading'), tr('cs.t.contact'), { fontSize: 34, fontWeight: 800, color: 'var(--c-text)' }, { tag: 'h2' }),
        text(tr('cs.n.text'), tr('cs.t.contactText'), { fontSize: 16, lineHeight: 1.7, color: 'var(--c-muted)' }, { width: fill }),
      ]),
      frame(tr('cs.n.contact'), { mode: 'vertical', gap: 16 }, {}, [
        contactRow('phone', '02-123-4567', 'var(--c-text)', 'var(--c-primary)'),
        contactRow('mail', 'hello@example.com', 'var(--c-text)', 'var(--c-primary)'),
        contactRow('map-pin', tr('cs.t.address'), 'var(--c-text)', 'var(--c-primary)'),
        contactRow('clock', tr('cs.t.hours'), 'var(--c-text)', 'var(--c-primary)'),
      ]),
    ], { responsive: { tablet: { layout: { mode: 'vertical', gap: 28 } }, mobile: { layout: { padding: [48, 20, 48, 20] } } } }),
  },
  // ------------------------------------------------------------ blocks
  {
    id: 'card', group: 'block', icon: '▢',
    build: () => ({
      ...frame(tr('cs.card'), { mode: 'vertical' }, { fill: '#ffffff', radius: 16, clip: true, shadow: '0 1px 3px rgba(0,0,0,.06), 0 8px 24px rgba(17,24,39,.06)', hover: { lift: 4 } }, [
        { id: newId(), type: 'image', name: tr('cs.n.image'), width: fill, height: fixed(180), style: { fill: '#e5e7eb' }, props: { src: '', alt: '' } },
        frame(tr('cs.n.body'), { mode: 'vertical', gap: 8, padding: [20, 20, 22, 20] }, {}, [
          text(tr('cs.n.heading'), tr('cs.t.cardTitle'), { fontSize: 19, fontWeight: 700, color: 'var(--c-text)' }, { tag: 'h3', width: fill }),
          text(tr('cs.n.text'), tr('cs.t.cardText'), { fontSize: 15, lineHeight: 1.6, color: 'var(--c-muted)' }, { width: fill }),
          text(tr('cs.n.link'), tr('cs.t.more') + ' →', { fontSize: 15, fontWeight: 600, color: 'var(--c-primary)' }, { href: '#' }),
        ]),
      ]), width: fixed(340),
    }),
  },
  {
    id: 'testimonial', group: 'block', icon: '❝',
    build: () => ({
      ...frame(tr('cs.testimonial'), { mode: 'vertical', gap: 16, padding: [28, 28, 28, 28] }, { fill: '#ffffff', radius: 20, stroke: '#e5e7eb', strokeWidth: 1 }, [
        icon(tr('cs.n.quote'), 'quote', 32, 'var(--c-primary)'),
        text(tr('cs.n.text'), tr('cs.t.quote'), { fontSize: 17, lineHeight: 1.7, color: 'var(--c-text)', italic: true }, { width: fill }),
        frame(tr('cs.n.person'), { mode: 'horizontal', gap: 12, align: 'center' }, {}, [
          { id: newId(), type: 'image', name: tr('cs.n.avatar'), width: fixed(44), height: fixed(44), style: { fill: '#e5e7eb', radius: 999 }, props: { src: '', alt: '' } },
          frame(tr('cs.n.texts'), { mode: 'vertical', gap: 2 }, {}, [
            text(tr('cs.n.name'), tr('cs.t.person'), { fontSize: 15, fontWeight: 700, color: 'var(--c-text)' }),
            text(tr('cs.n.role'), tr('cs.t.personRole'), { fontSize: 13, color: 'var(--c-muted)' }),
          ]),
        ]),
      ]), width: fixed(380),
    }),
  },
  {
    id: 'pricing', group: 'block', icon: '฿',
    build: () => {
      const li = (t: string) => frame(tr('cs.n.listItem'), { mode: 'horizontal', gap: 10, align: 'center' }, {}, [icon('check', 'check', 18, 'var(--c-primary)'), text(tr('cs.n.text'), t, { fontSize: 15, color: 'var(--c-text)' })]);
      return {
        ...frame(tr('cs.pricing'), { mode: 'vertical', gap: 18, padding: [32, 28, 32, 28] }, { fill: '#ffffff', radius: 24, stroke: 'var(--c-primary)', strokeWidth: 2, shadow: '0 20px 48px rgba(17,24,39,.10)' }, [
          text(tr('cs.n.plan'), tr('cs.t.plan'), { fontSize: 14, fontWeight: 700, color: 'var(--c-primary)', textTransform: 'uppercase', letterSpacing: 1 }),
          frame(tr('cs.n.price'), { mode: 'horizontal', gap: 6, align: 'end' }, {}, [
            text(tr('cs.n.price'), '฿990', { fontSize: 44, fontWeight: 800, color: 'var(--c-text)', lineHeight: 1 }),
            text(tr('cs.n.per'), tr('cs.t.perMonth'), { fontSize: 15, color: 'var(--c-muted)' }),
          ]),
          frame(tr('cs.n.list'), { mode: 'vertical', gap: 10 }, {}, [li(tr('cs.t.p1')), li(tr('cs.t.p2')), li(tr('cs.t.p3'))]),
          { ...button(tr('cs.n.cta'), tr('cs.t.choose'), { fill: 'var(--c-primary)', color: '#ffffff', radius: 12, textAlign: 'center' }, [14, 24, 14, 24]), width: fill },
        ]), width: fixed(340),
      };
    },
  },
  {
    id: 'blank', group: 'block', icon: '＋',
    build: () => frame(tr('cs.blank'), { mode: 'vertical', gap: 12, padding: [24, 24, 24, 24] }, {}, []),
  },
];

/** A copy of a component tree with fresh layer ids (for "duplicate"). */
export function freshIds(n: DNode): DNode {
  return cloneWithNewIds(n);
}

/** Page-like wrapper used to preview a component with the site renderer. */
export function previewDoc(node: DNode, mobile = false): DNode {
  return {
    id: 'root', type: 'frame', name: 'Preview', width: fill, height: hug, layout: { mode: 'vertical', align: 'center', padding: mobile ? [0, 0, 0, 0] : [0, 0, 0, 0] },
    style: { fill: '#f3f4f6' }, children: [node],
  };
}

/**
 * Scales a preview frame (rendered at `width` px) to fit its box, centred.
 * The frame is sandboxed without scripts; same-origin only so its height can
 * be measured here.
 */
export function fitFrame(f: HTMLIFrameElement, width: number, thumbnail = false) {
  const box = f.parentElement;
  const doc = f.contentDocument;
  if (!box || !doc) return;
  const measure = (w: number) => {
    f.style.width = w + 'px'; // lay out at the real width (media queries), then measure
    f.style.height = '1px';
    const h = Math.max(40, doc.documentElement.scrollHeight);
    f.style.height = h + 'px';
    // thumbnails zoom in on the component itself (a button is small on a wide page)
    const el = thumbnail ? doc.querySelector('.n-root > *') : null;
    const r = el?.getBoundingClientRect();
    const area = r && r.width > 0 && r.height > 0 ? { x: r.x, y: r.y, w: r.width, h: r.height } : { x: 0, y: 0, w, h };
    const scale = Math.min(box.clientWidth / area.w, box.clientHeight / area.h, thumbnail ? 1.5 : 1);
    return { w, h, area, scale };
  };
  let m = measure(width);
  // long, flat things (headers, footers) are unreadable at full width in a small
  // thumbnail: lay them out at tablet width when that makes them bigger
  if (thumbnail && width > 900 && m.area.h * m.scale < box.clientHeight * 0.35) {
    const n = measure(820);
    if (n.area.h * n.scale <= m.area.h * m.scale) m = measure(width); else m = n;
  }
  const { area } = m;
  let scale = m.scale;
  let left = box.clientWidth / 2 - (area.x + area.w / 2) * scale;
  // still a thin strip (a header): zoom until it is readable and show its start (the logo)
  if (thumbnail && area.h * scale < box.clientHeight * 0.3) {
    scale = (box.clientHeight * 0.3) / area.h;
    left = 8 - area.x * scale;
  }
  Object.assign(f.style, {
    width: m.w + 'px', height: m.h + 'px', transformOrigin: '0 0', transform: `scale(${scale})`, position: 'absolute',
    left: left + 'px', top: (box.clientHeight / 2 - (area.y + area.h / 2) * scale) + 'px',
  });
}
