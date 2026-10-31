import { CodeBindings } from '../core/code-models';
import {
  GROUP_ASK, groupCondition, parseGroups, previewVisitor, validGroup,
  applySuggestion, buildInputOptions, collectionChoices, collectionKey, collectionPreviewRequest, conditionPresets, defaultKey, docQueryKeys, docUsesCode,
  enclosingCollection, enclosingForm, evalParams, filterLayers, filterQueryName, formInputNames, layerChoices, mappingRows, matchPagePath, parseSource,
  renameMapping, setMapping, sourceFields, sourceParams, suggest, SuggestSource,
} from './code-logic';
import { categoryFilterPreset, createPagination, searchBoxPreset, submitButtonPreset } from './code-presets';
import { pageLinks, paginationCss, pageItemDecls } from './design-css';
import { DNode, cloneWithNewIds } from './design-types';
import { validRedirect } from './code-action';

const B: CodeBindings = {
  collections: [
    { name: 'events', label: { th: 'กิจกรรม', en: 'Events' }, fields: { title: 'text', date: 'date', price: 'number' }, params: { category: 'text', q: 'text' }, perPage: { default: 12, max: 30 } },
    { name: 'categories', fields: { slug: 'text', name: 'text' } },
  ],
  actions: [{ name: 'book', input: { eventId: 'number', note: 'text?' } }],
  pages: [{ path: '/events/{slug}' }],
  formatters: ['baht', 'thaiDate'],
  tables: [{ name: 'orders', columns: [{ name: 'id', type: 'bigint' }, { name: 'code', type: 'varchar' }, { name: 'total', type: 'decimal' }] }],
};

const node = (type: DNode['type'], id: string, props: DNode['props'] = {}, children?: DNode[]): DNode =>
  ({ id, type, width: { mode: 'fill' }, height: { mode: 'hug' }, props, ...(children ? { children } : {}) });

function doc(): DNode {
  return node('frame', 'root', {}, [
    node('collection', 'Col1AB', { source: 'code:events', params: { category: 'query.cat' } }, [
      node('frame', 'card', {}, [node('text', 't1', { text: '{{item.title}}' }), node('button', 'b1', { action: { type: 'code', name: 'book' } })]),
    ]),
    node('collection', 'plug', { source: 'products' }),
    node('collection', 'tbl', { source: 'table:orders', key: 'ord' }),
    node('frame', 'form1', { form: true }, [node('input', 'in1', { name: 'email' }), node('input', 'in2', { name: 'note' }), node('button', 'send')]),
    node('input', 'flt', { kind: 'search', filter: { collection: 'tbl', param: 'q' } }),
  ]);
}

describe('sources and keys', () => {
  it('parses sources', () => {
    expect(parseSource('code:events')).toEqual({ kind: 'code', name: 'events' });
    expect(parseSource('table:orders')).toEqual({ kind: 'table', name: 'orders' });
    expect(parseSource('products')).toEqual({ kind: 'plugin', name: 'products' });
    expect(parseSource(undefined)).toEqual({ kind: '', name: '' });
  });
  it('default key = short id', () => {
    expect(defaultKey('Ab-3_xYz9qwertyU')).toBe('ab3xyz9qwert');
    expect(collectionKey(node('collection', 'Q1W2', { key: 'shop' }))).toBe('shop');
    expect(collectionKey(node('collection', 'Q1W2', { key: 'Bad Key' }))).toBe('q1w2');
  });
  it('fields and params from bindings', () => {
    expect(sourceFields('code:events', B)).toEqual({ title: 'text', date: 'date', price: 'number' });
    expect(sourceFields('table:orders', B)).toEqual({ id: 'bigint', code: 'varchar', total: 'decimal' });
    expect(sourceFields('table:orders', B, { columns: ['code'] })).toEqual({ code: 'varchar' });
    expect(sourceFields('products', B)).toEqual({});
    expect(sourceParams('code:events', B)).toEqual({ category: 'text', q: 'text' });
    expect(sourceParams('table:orders', B)).toEqual({});
  });
});

describe('params mapping', () => {
  it('lists declared params first, then extra ones', () => {
    expect(mappingRows({ category: 'text', q: 'text' }, { q: 'query.q', old: '1' })).toEqual([
      { name: 'category', type: 'text', expr: '', declared: true },
      { name: 'q', type: 'text', expr: 'query.q', declared: true },
      { name: 'old', expr: '1', declared: false },
    ]);
    expect(mappingRows(undefined, undefined)).toEqual([]);
  });
  it('sets, clears and renames entries', () => {
    expect(setMapping(undefined, 'q', ' query.q ')).toEqual({ q: 'query.q' });
    expect(setMapping({ q: 'query.q' }, 'q', '')).toBeUndefined();
    expect(setMapping({ q: 'a', c: 'b' }, 'q', '  ')).toEqual({ c: 'b' });
    expect(renameMapping({ a: '1', b: '2' }, 'a', 'z')).toEqual({ z: '1', b: '2' });
    expect(renameMapping({ a: '1' }, 'a', '9bad')).toEqual({ a: '1' });
  });
  it('evaluates params in the page context, dropping empty values', () => {
    const ctx = { query: { cat: 'music', q: '' }, page: { n: 3 }, site: {}, post: null, item: null };
    expect(evalParams({ category: 'query.cat', q: 'query.q', n: 'page.n * 2', bad: '1 +', fixed: "'x'" }, ctx)).toEqual({ category: 'music', n: 6, fixed: 'x' });
  });
  it('builds the preview request (first page, perPage clamped by the collection)', () => {
    const ctx = { query: { cat: 'music' }, page: {}, site: {}, post: null, item: null };
    const col = doc().children![0];
    expect(collectionPreviewRequest(col, B, ctx, 'anonymous', false)).toEqual({ kind: 'collection', source: 'code:events', page: 1, perPage: 12, params: { category: 'music' }, viewAs: 'anonymous' });
    expect(collectionPreviewRequest({ ...col, props: { ...col.props, perPage: 99 } }, B, ctx, 'me', true)).toMatchObject({ perPage: 30, viewAs: 'me', draft: true });
    const tbl = node('collection', 'x', { source: 'table:orders', table: { where: [{ column: 'code', op: '=', value: 'query.q' }] } });
    expect(collectionPreviewRequest(tbl, B, ctx, 'anonymous', false)).toMatchObject({ source: 'table:orders', perPage: 10, table: { where: [{ column: 'code', op: '=', value: 'query.q' }] } });
    expect(collectionPreviewRequest(node('collection', 'p', { source: 'products' }), B, ctx, 'anonymous', false)).toBeNull();
  });
});

describe('pickers', () => {
  it('collection picker keeps code:/table: collections of the page', () => {
    expect(collectionChoices(doc()).map(c => [c.id, c.key])).toEqual([['Col1AB', 'col1ab'], ['tbl', 'ord']]);
    expect(collectionChoices(doc(), { codeOnly: false }).map(c => c.id)).toEqual(['Col1AB', 'plug', 'tbl']);
  });
  it('layer picker lists every layer but the excluded one, indented', () => {
    const all = layerChoices(doc(), 'b1');
    expect(all.some(l => l.id === 'b1')).toBe(false);
    expect(all.some(l => l.id === 'root')).toBe(false);
    const card = all.find(l => l.id === 'card')!;
    expect(card.depth).toBe(2);
    expect(card.label.startsWith('  ')).toBe(true);
    expect(filterLayers(all, 'INPUT').map(l => l.id)).toEqual(['in1', 'in2', 'flt']);
    expect(filterLayers(all, '').length).toBe(all.length);
  });
  it('finds the enclosing collection and form', () => {
    const d = doc();
    expect(enclosingCollection(d, 't1')?.id).toBe('Col1AB');
    expect(enclosingCollection(d, 'Col1AB')).toBeNull();
    expect(enclosingForm(d, 'send')?.id).toBe('form1');
    expect(enclosingForm(d, 'b1')).toBeNull();
    expect(formInputNames(enclosingForm(d, 'send'))).toEqual(['email', 'note']);
  });
  it('filter query names follow the params mapping or <key>_<param>', () => {
    const d = doc();
    expect(filterQueryName(d.children![0], 'category')).toBe('cat');
    expect(filterQueryName(d.children![0], 'q')).toBe('col1ab_q');
    expect(docQueryKeys(d).sort()).toEqual(['cat', 'ord_q']);
  });
  it('page patterns', () => {
    expect(matchPagePath('/events/{slug}', '/events/jazz')).toBe(true);
    expect(matchPagePath('/events/{slug}', '/events')).toBe(false);
    expect(matchPagePath('/checkout', '/checkout?x=1')).toBe(true);
    expect(matchPagePath('/docs/*', '/docs/a/b')).toBe(true);
    expect(matchPagePath('/', '/')).toBe(true);
    expect(matchPagePath('/a', '/b')).toBe(false);
  });
  it('detects pages using code features', () => {
    expect(docUsesCode(doc())).toBe(true);
    expect(docUsesCode(node('frame', 'r', {}, [node('text', 't', { text: 'hi' })]))).toBe(false);
    expect(docUsesCode(node('frame', 'r', {}, [node('text', 't', { showWhen: 'query.q' })]))).toBe(true);
  });
});

describe('input options', () => {
  it('static options (deduplicated, label falls back to the value)', () => {
    expect(buildInputOptions({ options: [{ value: 'a', label: 'A' }, { value: 'b', label: '' }, { value: 'a', label: 'again' }] }, null))
      .toEqual([{ value: 'a', label: 'A' }, { value: 'b', label: 'b' }]);
  });
  it('options from a source', () => {
    const items = [{ slug: 'music', name: 'ดนตรี' }, { slug: 'art', name: null }, { slug: '', name: 'skip' }, { slug: 'music', name: 'dup' }, { slug: 5, name: 'Five' }];
    expect(buildInputOptions({ optionsFrom: { collection: 'code:categories', value: 'slug', label: 'name' }, options: [{ value: 'x', label: 'X' }] }, items))
      .toEqual([{ value: 'music', label: 'ดนตรี' }, { value: 'art', label: 'art' }, { value: '5', label: 'Five' }]);
    expect(buildInputOptions({ optionsFrom: { collection: 'code:categories', value: 'slug', label: 'name' } }, undefined)).toEqual([]);
  });
});

describe('condition presets', () => {
  it('offers $empty and item fields inside a collection, query keys everywhere', () => {
    const p = conditionPresets(true, ['stock'], ['cat']);
    expect(p.map(x => x.expr)).toEqual(['$empty', '!item.stock', 'item.stock', 'query.cat']);
    expect(conditionPresets(false, ['stock'], []).map(x => x.expr)).toEqual(['query.q']);
  });
});

describe('autocomplete', () => {
  const src: SuggestSource = {
    inCollection: true, itemFields: ['title', 'price'], item: { title: 'A', price: 1, venue: { name: 'Hall', city: 'BKK' }, tags: ['x'] },
    page: { total: 5, cart: { count: 2 } }, queryKeys: ['q', 'cat'], formatters: ['baht'],
  };
  const labels = (r: ReturnType<typeof suggest>) => r?.items.map(i => i.label) ?? null;

  it('suggests roots inside {{ }} only', () => {
    expect(suggest('Hello ', 6, 'template', src)).toBeNull();
    expect(labels(suggest('Hi {{ ', 6, 'template', src))).toEqual(['item', '$index', '$empty', 'page', 'post', 'site', 'query', 'visitor']);
    expect(labels(suggest('{{ p', 4, 'template', src))).toEqual(['page', 'post']);
    expect(suggest('{{ item.title }} ', 17, 'template', src)).toBeNull();
    expect(labels(suggest('', 0, 'expr', { ...src, inCollection: false }))).toEqual(['page', 'post', 'site', 'query', 'visitor']);
  });
  it('suggests item fields (declared + sample keys) and nested keys', () => {
    expect(labels(suggest('{{item.', 7, 'template', src))).toEqual(['title', 'price', 'venue', 'tags']);
    expect(labels(suggest('{{ item.pr', 10, 'template', src))).toEqual(['price']);
    expect(labels(suggest('item.venue.', 11, 'expr', src))).toEqual(['name', 'city']);
    expect(labels(suggest('item.tags.', 10, 'expr', src))).toEqual(['length']);
    expect(suggest('item.', 5, 'expr', { ...src, inCollection: false })).toBeNull();
  });
  it('suggests page keys from the preview, post/site/query keys', () => {
    expect(labels(suggest('page.', 5, 'expr', src))).toEqual(['total', 'cart']);
    expect(labels(suggest('page.cart.', 10, 'expr', src))).toEqual(['count']);
    expect(labels(suggest('query.', 6, 'expr', src))).toEqual(['q', 'cat']);
    expect(labels(suggest('site.t', 6, 'expr', src))).toEqual(['title', 'tagline']);
    expect(labels(suggest('post.ti', 7, 'expr', src))).toEqual(['title']);
  });
  it('suggests formatter names after |', () => {
    expect(labels(suggest('{{ item.price | ', 16, 'template', src))).toEqual(['number', 'date', 'upper', 'lower', 'truncate', 'default', 'json', 'baht']);
    expect(labels(suggest('item.price | b', 14, 'expr', src))).toEqual(['baht']);
    expect(suggest('item.price | zz', 15, 'expr', src)).toBeNull();
  });
  it('suggests plugin formatters (ns:name) and aliases with their origin (phase 6)', () => {
    const ps = { ...src, formatters: ['baht', 'thai:baht', 'thai:thaiDate', 'bahtx'], formatterDetail: { 'thai:baht': 'thai', bahtx: '→ shop:bahtx' } };
    expect(labels(suggest('item.price | th', 15, 'expr', ps))).toEqual(['thai:baht', 'thai:thaiDate']);
    const r = suggest('item.price | thai:b', 19, 'expr', ps)!;
    expect(r.items.map(i => [i.label, i.detail])).toEqual([['thai:baht', 'thai']]);
    expect(applySuggestion('item.price | thai:b', r, r.items[0]).text).toBe('item.price | thai:baht');
    expect(suggest('item.price | b', 14, 'expr', ps)!.items.map(i => i.detail)).toEqual(['site', '→ shop:bahtx']);
    // a core formatter's argument / a site formatter's argument: roots, not formatters
    expect(labels(suggest('item.price | number:', 20, 'expr', ps))?.[0]).toBe('item');
    expect(labels(suggest('item.price | baht:', 18, 'expr', ps))?.[0]).toBe('item');
  });
  it('stays quiet inside strings and numbers', () => {
    expect(suggest("item.x == 'it", 13, 'expr', src)).toBeNull();
    expect(suggest('12', 2, 'expr', src)).toBeNull();
    expect(suggest("'none'", 6, 'expr', src)).toBeNull();
    expect(suggest('(item.a)', 8, 'expr', src)).toBeNull();
    expect(suggest('item.price ', 11, 'expr', src)).toBeNull();
    expect(labels(suggest('item.a || ', 10, 'expr', src))?.[0]).toBe('item');
    expect(suggest('{{ "x" }}', 6, 'template', src)).toBeNull();
  });
  it('after an operator offers roots again', () => {
    expect(labels(suggest('item.price * ', 13, 'expr', src))?.[0]).toBe('item');
    expect(labels(suggest('(q', 2, 'expr', src))).toEqual(['query']);
  });
  it('applies a suggestion at the caret', () => {
    const text = '{{ item.pr }} more';
    const r = suggest(text, 10, 'template', src)!;
    expect(applySuggestion(text, r, r.items[0])).toEqual({ text: '{{ item.price }} more', caret: 13 });
  });
});

describe('pagination', () => {
  it('numbers with a window around the current page', () => {
    const show = (l: ReturnType<typeof pageLinks>) => l.map(x => x.kind === 'page' ? (x.active ? `[${x.page}]` : String(x.page)) : x.kind === 'gap' ? '…' : x.kind + (x.disabled ? '!' : '')).join(' ');
    expect(show(pageLinks('numbers', 1, 10, 2))).toBe('prev! [1] 2 3 … 10 next');
    expect(show(pageLinks('numbers', 5, 10, 1))).toBe('prev 1 … 4 [5] 6 … 10 next');
    expect(show(pageLinks('numbers', 10, 10, 0))).toBe('prev 1 … [10] next!');
    expect(show(pageLinks(undefined, 1, 1))).toBe('prev! [1] next!');
    expect(show(pageLinks('prevnext', 2, 3))).toBe('prev next');
    expect(show(pageLinks('loadmore', 1, 3))).toBe('more');
    expect(show(pageLinks('loadmore', 3, 3))).toBe('more!');
  });
  it('state styles merge over defaults and only allow colours / theme tokens', () => {
    const n = createPagination('c1');
    n.props!.itemStyle = { radius: 999, fill: 'red;}body{x:y' };
    n.props!.activeStyle = { fill: 'var(--c-accent)' };
    expect(pageItemDecls(n, 'item')).toMatchObject({ 'border-radius': '999px', background: 'inherit', padding: '8px 14px' });
    expect(pageItemDecls(n, 'active')).toMatchObject({ background: 'var(--c-accent)', color: '#ffffff' });
    expect(pageItemDecls(n, 'disabled')).toMatchObject({ 'pointer-events': 'none' });
    expect(paginationCss(n)).toContain(`.n-${n.id} .rea-pg-item[aria-current=page]{`);
  });
});

describe('presets', () => {
  it('search box filters the first code collection by its search param', () => {
    const s = searchBoxPreset(doc(), B);
    expect(s.type).toBe('input');
    expect(s.props).toMatchObject({ kind: 'search', name: 'q', filter: { collection: 'Col1AB', param: 'q' } });
  });
  it('category filter reads options from a categories collection', () => {
    const c = categoryFilterPreset(doc(), B);
    expect(c.props).toMatchObject({ kind: 'select', name: 'category', optionsFrom: { collection: 'code:categories', value: 'slug', label: 'name' }, filter: { collection: 'Col1AB', param: 'category' } });
    expect(categoryFilterPreset(node('frame', 'r', {}, []), null).props).toMatchObject({ kind: 'select', options: [] });
  });
  it('submit button calls the first action', () => {
    expect(submitButtonPreset(B).props?.action).toMatchObject({ type: 'code', name: 'book', input: {} });
  });
  it('copies keep pagination / filter / update links inside the copy', () => {
    const tree = node('frame', 'r', {}, [
      node('collection', 'c', { source: 'code:events' }),
      node('pagination', 'p', { collection: 'c' }),
      node('input', 'i', { filter: { collection: 'c', param: 'q' } }),
      node('button', 'b', { action: { type: 'code', name: 'book', update: ['c', 'outside'] } }),
    ]);
    const copy = cloneWithNewIds(tree);
    const [c, p, i, b] = copy.children!;
    expect(c.id).not.toBe('c');
    expect(p.props?.collection).toBe(c.id);
    expect(i.props?.filter?.collection).toBe(c.id);
    expect(b.props?.action?.update).toEqual([c.id, 'outside']);
  });
  it('success redirect must be a same-site path', () => {
    expect(validRedirect('/thanks')).toBe(true);
    expect(validRedirect('')).toBe(true);
    expect(validRedirect('//evil.com')).toBe(false);
    expect(validRedirect('https://evil.com')).toBe(false);
    expect(validRedirect('thanks')).toBe(false);
  });
});

describe('site visitor (phase 4a)', () => {
  const src: SuggestSource = { inCollection: false, itemFields: [], queryKeys: [], formatters: [], visitorGroups: ['gold', 'member', 'bad-name'], postMeta: ['date', 'place'] };
  const labels = (r: ReturnType<typeof suggest>) => r?.items.map(i => i.label) ?? null;

  it('autocomplete: visitor root, keys, groups after visitor.is., post.meta fields', () => {
    expect(labels(suggest('{{ vi', 5, 'template', src))).toEqual(['visitor']);
    expect(labels(suggest('visitor.', 8, 'expr', src))).toEqual(['id', 'name', 'groups', 'is']);
    expect(labels(suggest('visitor.is.', 11, 'expr', src))).toEqual(['gold', 'member']);
    expect(labels(suggest('visitor.is.g', 12, 'expr', src))).toEqual(['gold']);
    expect(labels(suggest('post.me', 7, 'expr', src))).toEqual(['meta']);
    expect(labels(suggest('post.meta.', 10, 'expr', src))).toEqual(['date', 'place']);
    expect(suggest('visitor.is.', 11, 'expr', { ...src, visitorGroups: undefined })).toBeNull();
  });
  it('condition presets: signed in / out, one per declared group, "in group…"', () => {
    const p = conditionPresets(false, [], [], ['gold', 'bad-name']);
    expect(p.map(x => x.expr)).toEqual(['query.q', 'visitor', '!visitor', 'visitor.is.gold', '']);
    expect(p[p.length - 1].id).toBe(GROUP_ASK);
    expect(p.find(x => x.id === 'group:gold')?.params).toEqual({ g: 'gold' });
    // without groups argument: unchanged (phase 3)
    expect(conditionPresets(false, [], []).map(x => x.expr)).toEqual(['query.q']);
    expect(conditionPresets(false, [], [], []).map(x => x.expr)).toEqual(['query.q', 'visitor', '!visitor', '']);
  });
  it('group names', () => {
    expect(groupCondition(' gold ')).toBe('visitor.is.gold');
    expect(groupCondition('vip-1')).toBeNull();
    expect(groupCondition('__proto__')).toBeNull();
    expect(validGroup('member_2')).toBe(true);
    expect(parseGroups('gold, member  gold,,bad-x')).toEqual(['gold', 'member']);
    expect(parseGroups('')).toEqual([]);
  });
  it('preview visitor for "view as"', () => {
    expect(previewVisitor('anonymous', 'x')).toBeNull();
    expect(previewVisitor('me', 'x')).toBeNull();
    expect(previewVisitor({ groups: ['gold'] }, 'Sample')).toEqual({ id: 'preview', name: 'Sample', groups: ['gold'] });
  });
  it('collection preview request carries viewAs {groups}', () => {
    const n = { id: 'c1', type: 'collection', props: { source: 'code:events' } } as never;
    expect(collectionPreviewRequest(n, null, {}, { groups: ['gold'] }, false)?.viewAs).toEqual({ groups: ['gold'] });
  });
});
