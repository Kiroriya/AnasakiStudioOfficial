import { tr } from '../core/i18n';
import { DNode, newId } from './design-types';
import './messages-prototype-presets';

/** Ready-made interactive blocks (Assets → Interactive): built from the Prototype features. */

const fill = { mode: 'fill' as const };
const hug = { mode: 'hug' as const };
const fixed = (value: number) => ({ mode: 'fixed' as const, value });
const text = (name: string, t: string, style: DNode['style'], extra: Partial<DNode> = {}): DNode =>
  ({ id: newId(), type: 'text', name, width: fill, height: hug, style, props: { text: t, tag: 'p' }, ...extra });
const button = (name: string, t: string, style: DNode['style'], props: DNode['props'] = {}): DNode =>
  ({ id: newId(), type: 'button', name, width: hug, height: hug, layout: { padding: [12, 24, 12, 24] }, style: { fill: 'var(--c-primary)', color: '#ffffff', radius: 10, fontWeight: 600, fontSize: 15, ...style }, props: { text: t, href: '', ...props } });

export type InteractiveKind = 'popup' | 'faq' | 'tabs' | 'slider';
export const INTERACTIVE: { kind: InteractiveKind; icon: string }[] = [
  { kind: 'popup', icon: '🗔' }, { kind: 'faq', icon: '☰' }, { kind: 'tabs', icon: '⊟' }, { kind: 'slider', icon: '🎞' },
];

export function createInteractive(kind: InteractiveKind): DNode {
  switch (kind) {
    case 'popup': {
      const pop: DNode = {
        id: newId(), type: 'frame', name: 'Popup', width: fixed(460), height: hug,
        layout: { mode: 'vertical', gap: 14, padding: [32, 32, 28, 32] }, style: { fill: '#ffffff', radius: 20, shadow: '0 24px 64px rgba(15,18,32,.25)' },
        props: { popup: true }, children: [],
      };
      const close: DNode = { id: newId(), type: 'icon', name: tr('pp.close'), width: fixed(24), height: fixed(24), absolute: true, x: 420, y: 16, style: { color: 'var(--c-muted)' }, props: { icon: 'x', label: tr('pp.close') } };
      close.props!.action = { type: 'close' };
      pop.children = [
        close,
        text(tr('pp.popTitle'), tr('pp.popTitleT'), { fontSize: 24, fontWeight: 800, color: 'var(--c-text)' }),
        text(tr('pp.popText'), tr('pp.popTextT'), { fontSize: 16, lineHeight: 1.6, color: 'var(--c-muted)' }),
        button(tr('pp.popCta'), tr('pp.popCtaT'), {}, { href: '#' }),
      ];
      const opener = button(tr('pp.opener'), tr('pp.openerT'), {});
      opener.props!.action = { type: 'open', target: pop.id };
      return { id: newId(), type: 'frame', name: tr('pp.popupBlock'), width: fill, height: hug, layout: { mode: 'vertical', align: 'center', gap: 16, padding: [32, 24, 32, 24] }, children: [opener, pop] };
    }
    case 'faq': {
      const item = (q: string, a: string, open = false): DNode => {
        const ans = text(tr('pp.answer'), a, { fontSize: 15, lineHeight: 1.7, color: 'var(--c-muted)' }, { props: { text: a, tag: 'p', collapsed: !open || undefined } });
        const qn: DNode = {
          id: newId(), type: 'frame', name: tr('pp.question'), width: fill, height: hug, layout: { mode: 'horizontal', justify: 'between', align: 'center', gap: 12 },
          style: { active: { color: 'var(--c-primary)' } },
          children: [text(tr('pp.question'), q, { fontSize: 17, fontWeight: 700 }), { id: newId(), type: 'icon', name: '+', width: fixed(20), height: fixed(20), style: { color: 'var(--c-primary)' }, props: { icon: 'chevron-down' } }],
          props: { action: { type: 'toggle', target: ans.id } },
        };
        return { id: newId(), type: 'frame', name: tr('pp.faqItem'), width: fill, height: hug, layout: { mode: 'vertical', gap: 10, padding: [18, 20, 18, 20] }, style: { fill: '#ffffff', radius: 14, stroke: '#e5e7eb', strokeWidth: 1 }, children: [qn, ans] };
      };
      return {
        id: newId(), type: 'frame', name: 'FAQ', width: fill, height: hug, layout: { mode: 'vertical', gap: 12, padding: [48, 24, 48, 24] }, style: { maxWidth: 760 },
        children: [
          text(tr('pp.faqTitle'), tr('pp.faqTitleT'), { fontSize: 32, fontWeight: 800, textAlign: 'center', color: 'var(--c-text)' }, { props: { text: tr('pp.faqTitleT'), tag: 'h2' } }),
          item(tr('pp.q1'), tr('pp.a1')), item(tr('pp.q2'), tr('pp.a2')), item(tr('pp.q3'), tr('pp.a3')),
        ],
      };
    }
    case 'tabs': {
      const panel = (t: string, body: string, hidden: boolean): DNode => ({
        id: newId(), type: 'frame', name: tr('pp.panel') + ' ' + t, width: fill, height: hug, layout: { mode: 'vertical', gap: 10, padding: [24, 24, 24, 24] },
        style: { fill: 'var(--c-surface)', radius: 14 }, props: hidden ? { collapsed: true } : {},
        children: [text(tr('pp.heading'), t, { fontSize: 20, fontWeight: 700, color: 'var(--c-text)' }), text(tr('pp.body'), body, { fontSize: 15, lineHeight: 1.7, color: 'var(--c-muted)' })],
      });
      const names = [tr('pp.tab1'), tr('pp.tab2'), tr('pp.tab3')];
      const panels = names.map((n, i) => panel(n, tr('pp.tabBody'), i > 0));
      const tabs: DNode[] = names.map((n, i) => ({
        ...button(tr('pp.tabBtn') + ' ' + (i + 1), n, { fill: 'transparent', color: 'var(--c-muted)', radius: 999, fontSize: 15 }),
        style: { fill: 'transparent', color: 'var(--c-muted)', radius: 999, fontWeight: 600, fontSize: 15, active: { fill: 'var(--c-primary)', color: '#ffffff' } },
        props: { text: n, href: '', action: { type: 'tab' as const, target: panels[i].id } },
      }));
      return {
        id: newId(), type: 'frame', name: tr('pp.tabsBlock'), width: fill, height: hug, layout: { mode: 'vertical', gap: 16, padding: [32, 24, 32, 24] }, style: { maxWidth: 900 },
        children: [
          { id: newId(), type: 'frame', name: tr('pp.tabBar'), width: hug, height: hug, layout: { mode: 'horizontal', gap: 6, padding: [6, 6, 6, 6], wrap: true }, style: { fill: 'var(--c-surface)', radius: 999 }, children: tabs },
          { id: newId(), type: 'frame', name: tr('pp.panels'), width: fill, height: hug, layout: { mode: 'vertical' }, children: panels },
        ],
      };
    }
    case 'slider': {
      const colors = ['linear-gradient(135deg,#4f46e5,#7c3aed)', 'linear-gradient(135deg,#0ea5e9,#22d3ee)', 'linear-gradient(135deg,#f59e0b,#ef4444)'];
      const slides: DNode[] = colors.map((c, i) => ({
        id: newId(), type: 'frame', name: tr('pp.slide') + ' ' + (i + 1), width: fill, height: fill,
        layout: { mode: 'vertical', align: 'center', justify: 'center', gap: 10, padding: [32, 64, 32, 64] }, style: { fill: c, color: '#ffffff', textAlign: 'center' },
        children: [text(tr('pp.heading'), tr('pp.slideT', { n: i + 1 }), { fontSize: 40, fontWeight: 800 }, { responsive: { mobile: { style: { fontSize: 28 } } } }), text(tr('pp.body'), tr('pp.slideBody'), { fontSize: 17, opacity: 0.9 })],
      }));
      return {
        id: newId(), type: 'frame', name: tr('pp.sliderBlock'), width: fill, height: fixed(420), layout: { mode: 'horizontal' }, style: { radius: 0, clip: true },
        props: { carousel: { arrows: true, dots: true, autoplay: 5 } }, children: slides,
        responsive: { mobile: { height: fixed(320) } },
      };
    }
  }
}
