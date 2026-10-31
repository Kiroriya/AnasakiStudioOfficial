import vectors from './testdata/expr_vectors.json';
import visitorVectors from './testdata/expr_vectors_visitor.json';
import { evalExpr, exprPaths, formatDate, parseExpr, renderTemplate, templateErrors, toText, truthy } from './expr';
import { escapeHtml } from './design-css';

interface Case { expr: string; out?: string; error?: boolean }

describe('expression vectors (shared with Go)', () => {
  const ctx = vectors.ctx as Record<string, unknown>;
  for (const c of vectors.cases as Case[]) {
    it(c.error ? `rejects ${c.expr}` : `${c.expr} → ${JSON.stringify(c.out)}`, () => {
      const r = evalExpr(c.expr, ctx, { locale: 'en', timeZone: 'UTC' });
      if (c.error) {
        expect(r.error).toBeTruthy();
        // the template keeps the raw text
        expect(renderTemplate(`a {{${c.expr}}} b`, ctx, { locale: 'en' })).toBe(`a {{${c.expr}}} b`);
      } else {
        expect(r.error).toBeUndefined();
        expect(toText(r.value)).toBe(c.out);
        expect(renderTemplate(`{{ ${c.expr} }}`, ctx, { locale: 'en', timeZone: 'UTC' })).toBe(c.out);
      }
    });
  }
});

describe('Thai dates', () => {
  const opts = { locale: 'th' as const, timeZone: 'UTC' };
  it('uses Thai month names and the Buddhist year', () => {
    expect(formatDate('2026-10-24T09:05:00Z', 'D MMM BBBB', opts)).toBe('24 ต.ค. 2569');
    expect(formatDate('2026-01-02', 'D MMMM YYYY', opts)).toBe('2 มกราคม 2026');
    expect(formatDate('2026-12-31', 'DD MMMM BBBB', opts)).toBe('31 ธันวาคม 2569');
    expect(formatDate('2026-02-05', 'D MMM YY', opts)).toBe('5 ก.พ. 26');
  });
  it('shows date-times in the site time zone; plain dates never shift', () => {
    expect(formatDate('2026-10-24T20:30:00Z', 'D/M/YYYY H:mm', { locale: 'th', timeZone: 'Asia/Bangkok' })).toBe('25/10/2026 3:30');
    expect(formatDate('2026-10-24 09:05:00', 'HH:mm', { timeZone: 'Asia/Bangkok' })).toBe('16:05');
    expect(formatDate('2026-10-24', 'D', { timeZone: 'America/New_York' })).toBe('24');
    expect(formatDate('2026-10-24T09:05:00+07:00', 'HH:mm', { timeZone: 'UTC' })).toBe('02:05');
  });
  it('keeps values that are not dates', () => {
    expect(formatDate('soon', 'D MMM', opts)).toBe('soon');
    expect(formatDate(null, 'D MMM', opts)).toBe(null);
  });
});

describe('expression helpers', () => {
  it('truthiness', () => {
    for (const v of [false, null, undefined, '', 0, '0', []]) expect(truthy(v)).toBe(false);
    for (const v of [true, 1, 'a', ['x'], {}, '0.0']) expect(truthy(v)).toBe(true);
  });
  it('keeps old {{post.x}} bindings identical', () => {
    const ctx = { site: { title: 'S' }, post: { title: 'P', date: '1 ม.ค. 2569' }, item: null };
    expect(renderTemplate('{{site.title}} · {{ post.title }} · {{post.date}} · {{item.title}}', ctx)).toBe('S · P · 1 ม.ค. 2569 · ');
    expect(renderTemplate('no bindings', ctx)).toBe('no bindings');
  });
  it('escapes values in rich templates and decodes entities in the expression', () => {
    const ctx = { item: { t: '<b>', n: 3 } };
    expect(renderTemplate('<b>{{item.t}}</b>', ctx, {}, escapeHtml)).toBe('<b>&lt;b&gt;</b>');
    expect(renderTemplate('{{item.n &gt; 2 ? &#39;many&#39; : &#39;few&#39;}}', ctx, {}, escapeHtml)).toBe('many');
  });
  it('lists template parse errors', () => {
    expect(templateErrors('{{ item.a }} {{ 1 + }}')).toEqual([{ expr: '1 +', error: { code: 'end', pos: 3 } }]);
    expect(templateErrors('plain')).toEqual([]);
  });
  it('reports the paths an expression reads', () => {
    expect(exprPaths('(item.a > 1 && !$empty ? page.x : 1) | default:query.q')).toEqual(['item.a', '$empty', 'page.x', 'query.q']);
    expect(exprPaths('1 +')).toEqual([]);
  });
  it('error codes', () => {
    expect(parseExpr('a = 1')).toMatchObject({ ok: false, error: { code: 'assign' } });
    expect(parseExpr('item.x()')).toMatchObject({ ok: false, error: { code: 'call' } });
    expect(parseExpr('item[0]')).toMatchObject({ ok: false, error: { code: 'index' } });
    expect(parseExpr("'open")).toMatchObject({ ok: false, error: { code: 'string' } });
    expect(parseExpr('')).toMatchObject({ ok: false, error: { code: 'empty' } });
  });
  it('remembers unknown formatters', () => {
    const unknown = new Set<string>();
    expect(toText(evalExpr('page.p | baht', { page: { p: 5 } }, { unknown }).value)).toBe('5');
    expect([...unknown]).toEqual(['baht']);
  });
});

interface VisitorCase { expr: string; out: string; truthy?: boolean }

describe('visitor expressions (phase 4a, shared with Go)', () => {
  for (const g of visitorVectors.groups) {
    describe(g.name, () => {
      const ctx = g.ctx as Record<string, unknown>;
      for (const c of g.cases as VisitorCase[]) {
        it(`${c.expr} → ${JSON.stringify(c.out)}`, () => {
          const r = evalExpr(c.expr, ctx, { locale: 'en', timeZone: 'UTC' });
          expect(r.error).toBeUndefined();
          expect(toText(r.value)).toBe(c.out);
          if (c.truthy !== undefined) expect(truthy(r.value)).toBe(c.truthy);
        });
      }
    });
  }
  it('visitor in templates and paths', () => {
    expect(renderTemplate("{{ visitor.is.gold ? 'Gold' : 'Join' }} {{visitor.name}}", { visitor: { name: 'Ann', groups: ['gold'] } })).toBe('Gold Ann');
    expect(renderTemplate("{{ visitor.is.gold ? 'Gold' : 'Join' }}", { visitor: null })).toBe('Join');
    expect(exprPaths('visitor.is.gold && visitor.name')).toEqual(['visitor.is.gold', 'visitor.name']);
  });
});

describe('plugin formatters ns:name (phase 6)', () => {
  it('parses `| thai:baht` as one formatter name, args after it', () => {
    const p = parseExpr('item.price | thai:baht:2');
    expect(p.ok && p.ast.k === 'pipe' && p.ast.name).toBe('thai:baht');
    expect(p.ok && p.ast.k === 'pipe' && p.ast.args.length).toBe(1);
  });
  it('keeps core formatter arguments, roots and spaced colons as arguments', () => {
    for (const [src, name, args] of [['x | number:two', 'number', 1], ['x | baht:item', 'baht', 1], ['x | baht: two', 'baht', 1], ['x | baht:item.qty', 'baht', 1], ['x | fmt:a.b', 'fmt', 1]] as const) {
      const p = parseExpr(src);
      expect(p.ok && p.ast.k === 'pipe' ? [p.ast.name, p.ast.args.length] : null).toEqual([name, args]);
    }
  });
  it('unknown plugin formatters leave the value and are remembered', () => {
    const unknown = new Set<string>();
    expect(toText(evalExpr('item.p | thai:baht', { item: { p: 5 } }, { unknown }).value)).toBe('5');
    expect([...unknown]).toEqual(['thai:baht']);
  });
});
