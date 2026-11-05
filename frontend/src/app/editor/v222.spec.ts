import vectors from './testdata/css_vectors_222.json';
import { cssText } from './copy-css';
import { nodeCss } from './design-css';
import type { DNode } from './design-types';

type Vec = { name: string; type: DNode['type']; style: Record<string, unknown>; expect?: Record<string, string>; absent?: string[] };

describe('style properties of 2.2.2 (same vectors as css.go)', () => {
  for (const v of vectors as Vec[]) {
    it(v.name, () => {
      const d = nodeCss({ id: 'x', type: v.type, width: { mode: 'hug' }, height: { mode: 'hug' }, style: v.style } as DNode, 'vertical');
      for (const [p, want] of Object.entries(v.expect ?? {})) expect(d[p]).toBe(want);
      for (const p of v.absent ?? []) expect(d[p]).toBeUndefined();
    });
  }
});

describe('Copy CSS (2.2.2)', () => {
  it('writes the layer as a CSS rule with its class name', () => {
    const css = cssText({ id: 'hero', type: 'frame', name: 'Hero', width: { mode: 'fill' }, height: { mode: 'hug' },
      style: { fill: '#111827', radius: 16 } } as DNode, 'vertical');
    expect(css.startsWith('/* Hero */\n.n-hero {\n')).toBe(true);
    expect(css).toContain('  background: #111827;\n');
    expect(css).toContain('  border-radius: 16px;\n');
    expect(css.trimEnd().endsWith('}')).toBe(true);
  });
});
