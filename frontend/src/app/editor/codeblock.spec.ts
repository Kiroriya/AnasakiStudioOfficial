import vectors from './testdata/codeblock_scope_vectors.json';
import { CODEBLOCK_CAP, canvasHtml, codeBlockAccess, codeBlockClass, hasCodeBlock, scopeCss } from './codeblock';
import { DNode } from './design-types';

interface Case { name: string; id: string; css: string; out: string }

describe('frontend code block CSS scoping (vectors shared with Go)', () => {
  for (const c of vectors.cases as Case[]) {
    it(c.name, () => {
      expect(scopeCss(c.css, c.id)).toBe(c.out);
    });
  }

  it('is idempotent on the class name', () => {
    expect(codeBlockClass('abc12345')).toBe('rea-cb-abc12345');
    expect(codeBlockClass('a b"c')).toBe('rea-cb-abc');
  });
});

describe('frontend code block capability gating', () => {
  it('uses edit_scripts', () => {
    expect(CODEBLOCK_CAP).toBe('edit_scripts');
  });

  it('lets users with edit_scripts insert and edit', () => {
    expect(codeBlockAccess(c => c === 'edit_scripts')).toEqual({ insert: true, edit: true });
  });

  it('locks the block for everyone else (even with unfiltered_html)', () => {
    expect(codeBlockAccess(c => c === 'unfiltered_html' || c === 'edit_pages')).toEqual({ insert: false, edit: false });
    expect(codeBlockAccess(() => false)).toEqual({ insert: false, edit: false });
  });

  it('finds code blocks in a tree', () => {
    const leaf: DNode = { id: 'c', type: 'codeblock', width: { mode: 'fill' }, height: { mode: 'hug' }, props: { html: '', css: '', js: '' } };
    const root: DNode = { id: 'r', type: 'frame', width: { mode: 'fill' }, height: { mode: 'hug' }, children: [{ id: 'f', type: 'frame', width: { mode: 'fill' }, height: { mode: 'hug' }, children: [leaf] }] };
    expect(hasCodeBlock(root)).toBe(true);
    expect(hasCodeBlock({ ...root, children: [] })).toBe(false);
    expect(hasCodeBlock(null)).toBe(false);
  });
});

describe('frontend code block canvas HTML (JS never runs in the editor)', () => {
  it('keeps ordinary markup', () => {
    expect(canvasHtml('<p class="x" style="color:red" data-k="1">Hi <b>there</b></p>', 'b1')).toBe('<p class="x" style="color:red" data-k="1">Hi <b>there</b></p>');
  });

  it('removes scripts, frames, handlers and script URLs', () => {
    const out = canvasHtml(
      '<script>alert(1)</script><img src="x" onerror="alert(1)"><a href="javascript:alert(1)">a</a>'
      + '<iframe srcdoc="<script>alert(1)</script>"></iframe><object data="x"></object><svg><script>1</script><a xlink:href="javascript:1">s</a></svg>'
      + '<form action=" javascript:1"><button formaction="data:text/html,x">b</button></form><base href="/evil/">',
      'b1');
    expect(out).not.toMatch(/script|onerror|javascript|iframe|object|data:text|<base/i);
    expect(out).toContain('<img src="x">');
    expect(out).toContain('<a>a</a>');
  });

  it('scopes inline <style> elements like the block CSS', () => {
    expect(canvasHtml('<style>p{color:red}</style><p>x</p>', 'b1')).toBe('<style>.rea-cb-b1 p { color:red }</style><p>x</p>');
  });

  it('returns an empty string for empty HTML', () => {
    expect(canvasHtml('', 'b1')).toBe('');
  });
});
