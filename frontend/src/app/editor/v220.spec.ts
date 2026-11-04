import { validatePath } from '../code/code-utils';
import { POST_LIST_PARAMS, collectionChoices } from './code-logic';
import { menuLinkCss, nodeCss, parseTheme, samePage } from './design-css';
import type { DNode } from './design-types';

const n = (id: string, type: DNode['type'], extra: Partial<DNode> = {}, children?: DNode[]): DNode =>
  ({ id, type, width: { mode: 'fill' }, height: { mode: 'hug' }, ...(children ? { children } : {}), ...extra } as DNode);

describe('menus (2.2.0)', () => {
  it('samePage matches SamePage in render.go', () => {
    expect(samePage('/qr/', '/qr')).toBe(true);
    expect(samePage('/qr?x=1#a', '/qr')).toBe(true);
    expect(samePage('/%E0%B8%81', '/ก')).toBe(true);
    expect(samePage('https://x.io/qr', '/qr')).toBe(false);
    expect(samePage('//x.io/qr', '/qr')).toBe(false);
    expect(samePage('/', '/')).toBe(true);
  });
  it('menuLinkCss: padding / radius on every link, the active colours on the current one', () => {
    const m = n('m', 'menu', { style: { linkPadding: [6, 14], linkRadius: 999, active: { fill: '#ff7a7a', color: '#fff' } } });
    expect(menuLinkCss(m, false)).toEqual({ padding: '6px 14px', 'border-radius': '999px' });
    expect(menuLinkCss(m, true)).toEqual({ padding: '6px 14px', 'border-radius': '999px', background: '#ff7a7a', color: '#fff' });
  });
});

describe('screen height (2.2.0)', () => {
  it('min-height follows the screen, minus the offset', () => {
    expect(nodeCss(n('h', 'frame', { style: { minHeightScreen: true, minHeightOffset: 64 } }), 'vertical')['min-height'])
      .toBe('calc(var(--rea-screen-h,100svh) - 64px)');
    expect(nodeCss(n('h', 'frame', { style: { minHeightScreen: true } }), 'vertical')['min-height']).toBe('var(--rea-screen-h,100svh)');
  });
});

describe('theme breakpoints (2.2.0, ParseTheme rules)', () => {
  it('keeps valid values and drops defaults or out-of-range ones', () => {
    expect(parseTheme('{"breakpoints":{"mobile":767}}').breakpoints).toEqual({ mobile: 767 });
    expect(parseTheme('{"breakpoints":{"mobile":640,"tablet":1024}}').breakpoints).toBeUndefined();
    expect(parseTheme('{"breakpoints":{"mobile":5,"tablet":700}}').breakpoints).toBeUndefined();
    expect(parseTheme('{"breakpoints":{"mobile":820,"tablet":900}}').breakpoints).toEqual({ mobile: 820 });
  });
});

describe('post lists as filter targets (2.2.0)', () => {
  it('collectionChoices lists post lists only when asked', () => {
    const root = n('root', 'frame', {}, [n('blog', 'postList', { name: 'Blog' }), n('c', 'collection', { props: { source: 'code:tools' } })]);
    expect(collectionChoices(root).map(c => c.id)).toEqual(['c']);
    expect(collectionChoices(root, { codeOnly: true, postLists: true }).map(c => c.id)).toEqual(['blog', 'c']);
    expect(POST_LIST_PARAMS).toEqual(['q', 'cat']);
  });
});

describe('Site Code public/ files (2.2.0)', () => {
  it('allows static types only under public/', () => {
    expect(validatePath('public/base.css')).toBeNull();
    expect(validatePath('public/logo.svg')).toBeNull();
    expect(validatePath('base.css')).toBe('badExtension');
    expect(validatePath('public/page.html')).toBe('badExtension');
    expect(validatePath('lib/x.ts')).toBeNull();
  });
});
