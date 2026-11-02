import { SchemaField } from './models';
import {
  InvalidJson, editRoute, fieldsToSchema, groupFields, fromDatetimeLocal, humanizeKey, isCustomType, listColumns, listRoute, metaCell, metaErrorsFromApi,
  normalizeAdminPages, normalizeFieldDef, normalizePostTypes, pickMeta, publicPath, routedTypes, supportsOf, templateOption, toDatetimeLocal, typePrefix,
  typeSource, validateMeta,
} from './post-type-utils';

describe('post types from the server', () => {
  it('accepts an array or an envelope; drops bad ids, built-ins and duplicates', () => {
    const raw = [
      { type: 'event', label: { th: 'อีเวนต์', en: 'Events' } }, { type: 'post' }, { type: 'Bad' }, { type: 'x' }, { type: 'event' }, null, { type: 'news_item' },
    ];
    expect(normalizePostTypes(raw).map(p => p.type)).toEqual(['event', 'news_item']);
    expect(normalizePostTypes({ postTypes: [{ type: 'event' }] }).map(p => p.type)).toEqual(['event']);
    expect(normalizePostTypes({ items: [{ type: 'job' }] }).map(p => p.type)).toEqual(['job']);
    expect(normalizePostTypes(null)).toEqual([]);
    expect(isCustomType('template')).toBe(false);
    expect(isCustomType('a_very_long_type_name_x')).toBe(false);
  });
  it('plugin post types (CMS 2.0.5): source and routing', () => {
    const list = normalizePostTypes([
      { type: 'event' }, { type: 'docs_doc', plugin: 'docs', pluginName: 'Docs', public: false, slug: 'docs' }, { type: 'shop_item', plugin: 'shop', public: true },
    ]);
    expect(list.map(p => p.type)).toEqual(['event', 'docs_doc', 'shop_item']);
    expect(list.map(typeSource)).toEqual(['', 'Docs', 'shop']);
    expect(typeSource(undefined)).toBe('');
    expect(routedTypes(list).map(p => p.type)).toEqual(['event', 'shop_item']);
  });
  it('admin pages', () => {
    expect(normalizeAdminPages([{ id: 'reports' }, { id: '../x' }, { id: 'reports' }, { id: 'sales-2026', label: 'Sales' }]).map(p => p.id)).toEqual(['reports', 'sales-2026']);
    expect(normalizeAdminPages({ adminPages: [{ id: 'a' }] })).toEqual([{ id: 'a' }]);
    // plugin pages are namespaced "<plugin id>:<name>"
    expect(normalizeAdminPages([{ id: 'forms:forms' }, { id: 'analytics:stats' }, { id: 'a:b:c' }, { id: ':x' }, { id: 'Bad:x' }]).map(p => p.id))
      .toEqual(['forms:forms', 'analytics:stats']);
  });
});

describe('custom fields → schema form', () => {
  const fields = fieldsToSchema({
    date: { type: 'datetime', required: true, label: { th: 'วันที่', en: 'Date' } },
    place: 'text',
    seats: { type: 'number', min: 0, max: 500 },
    kind: { type: 'select', options: ['talk', { value: 'ws', label: { en: 'Workshop' } }] },
    online: 'bool',
    readingTime: 'number',
    extra: 'json',
    weird: 'colorwheel',
  });
  it('maps types, labels, options and limits', () => {
    expect(fields.map(f => [f.key, f.type])).toEqual([
      ['date', 'datetime'], ['place', 'text'], ['seats', 'number'], ['kind', 'select'], ['online', 'boolean'], ['readingTime', 'number'], ['extra', 'json'], ['weird', 'text'],
    ]);
    expect(fields[0]).toMatchObject({ required: true, label: { th: 'วันที่', en: 'Date' } });
    expect(fields[1].label).toBe('Place');
    expect(fields[5].label).toBe('Reading time');
    expect(fields[2]).toMatchObject({ min: 0, max: 500 });
    expect(fields[3].options).toEqual([{ value: 'talk', label: 'talk' }, { value: 'ws', label: { th: 'Workshop', en: 'Workshop' } }]);
    expect(normalizeFieldDef('url')).toEqual({ type: 'url' });
    expect(humanizeKey('start_date')).toBe('Start date');
    expect(fieldsToSchema(undefined)).toEqual([]);
  });
  it('validates required / min / max / options / formats', () => {
    const e = validateMeta(fields, { seats: -1, kind: 'other', extra: new InvalidJson('{'), online: false });
    expect(e['date']).toEqual({ code: 'required' });
    expect(e['seats']).toEqual({ code: 'min', params: { n: 0 } });
    expect(e['kind']).toEqual({ code: 'option' });
    expect(e['extra']).toEqual({ code: 'json' });
    expect(e['online']).toBeUndefined();
    expect(e['place']).toBeUndefined();
    expect(validateMeta(fields, { date: '2026-10-24T09:00:00', seats: 501 })).toEqual({ seats: { code: 'max', params: { n: 500 } } });
    expect(validateMeta(fields, { date: '2026-10-24T09:00:00', seats: 'abc' })['seats']).toEqual({ code: 'number' });
    const more: SchemaField[] = [
      { key: 'mail', type: 'email', label: 'Mail' }, { key: 'site', type: 'url', label: 'Site' }, { key: 'day', type: 'date', label: 'Day' },
      { key: 'code', type: 'text', label: 'Code', min: 2, max: 4 },
    ];
    expect(validateMeta(more, { mail: 'nope', site: 'ftp://x', day: '24/10/2026', code: 'x' })).toEqual({
      mail: { code: 'email' }, site: { code: 'url' }, day: { code: 'date' }, code: { code: 'minLength', params: { n: 2 } },
    });
    expect(validateMeta(more, { mail: 'a@b.co', site: '/about', day: '2026-10-24', code: 'abcde' })).toEqual({ code: { code: 'maxLength', params: { n: 4 } } });
  });
  it('values to send: declared keys, empty → null, numbers, booleans', () => {
    expect(pickMeta(fields, { date: '2026-10-24T09:00:00', place: '', seats: '12', online: undefined, stray: 1, extra: { a: 1 } })).toEqual({
      date: '2026-10-24T09:00:00', place: null, seats: 12, kind: null, online: false, readingTime: null, extra: { a: 1 }, weird: null,
    });
    expect('extra' in pickMeta(fields, { extra: new InvalidJson('{') })).toBe(false);
  });
  it('reads per-field errors of a refused save', () => {
    expect(metaErrorsFromApi({ error: { error: 'x', fields: { date: 'ต้องระบุวันที่', 'meta.seats': 'ต้องไม่ติดลบ' } } })).toEqual({ date: 'ต้องระบุวันที่', seats: 'ต้องไม่ติดลบ' });
    expect(metaErrorsFromApi({ error: { errors: [{ field: 'place', message: 'ยาวเกินไป' }, { key: 'kind', error: 'ไม่มีตัวเลือกนี้' }] } })).toEqual({ place: 'ยาวเกินไป', kind: 'ไม่มีตัวเลือกนี้' });
    expect(metaErrorsFromApi({ error: '{"errors":{"date":"bad"}}' })).toEqual({ date: 'bad' });
    expect(metaErrorsFromApi({ error: { error: 'only a message' } })).toEqual({});
    expect(metaErrorsFromApi(null)).toEqual({});
  });
});

describe('paths, supports, list cells', () => {
  it('routes and public paths', () => {
    expect(listRoute('post')).toBe('/posts');
    expect(listRoute('event')).toBe('/content/event');
    expect(editRoute('post')).toEqual(['/posts', 'new']);
    expect(editRoute('event', 5)).toEqual(['/content', 'event', 5]);
    expect(typePrefix({ type: 'event', slug: '/events/' })).toBe('events');
    expect(publicPath({ type: 'event' }, 'expo')).toBe('/event/expo');
    expect(templateOption('event')).toBe('post_template_event');
    expect(templateOption('post')).toBe('post_template_id');
  });
  it('supports default to title + content + excerpt + image', () => {
    expect([...supportsOf({ type: 'event' })]).toEqual(['title', 'content', 'excerpt', 'image']);
    expect(supportsOf({ type: 'job', supports: ['title', 'categories'] }).has('content')).toBe(false);
    expect(supportsOf(undefined, 'post').has('categories')).toBe(true);
  });
  it('list columns and cells', () => {
    const fs = fieldsToSchema({ about: 'textarea', date: 'datetime', kind: { type: 'select', options: [{ value: 'a', label: 'Alpha' }] }, seats: 'number', on: 'bool' });
    expect(listColumns(fs).map(f => f.key)).toEqual(['date', 'kind']);
    expect(metaCell(fs[1], '2026-10-24T09:30:00')).toBe('2026-10-24 09:30');
    expect(metaCell(fs[2], 'a')).toBe('Alpha');
    expect(metaCell(fs[3], null)).toBe('—');
    expect(metaCell(fs[4], true)).toBe('✓');
    expect(metaCell(fs[0], { x: 1 })).toBe('{"x":1}');
  });
  it('datetime-local conversion', () => {
    expect(toDatetimeLocal('2026-10-24T09:30:00Z')).toBe('2026-10-24T09:30');
    expect(toDatetimeLocal('2026-10-24 09:30:00')).toBe('2026-10-24T09:30');
    expect(toDatetimeLocal(null)).toBe('');
    expect(fromDatetimeLocal('2026-10-24T09:30')).toBe('2026-10-24T09:30:00');
    expect(fromDatetimeLocal('')).toBe('');
  });
});

describe('plugin fields (site.fields in a JS plugin)', () => {
  it('groups plugin fields per plugin and keeps site fields apart', () => {
    const g = groupFields({
      subtitle: 'text',
      'seo.title': { type: 'text', plugin: 'seo', pluginName: 'SEO', label: { th: 'ชื่อ', en: 'Title' } },
      'seo.noindex': { type: 'bool', plugin: 'seo', pluginName: 'SEO' },
      'shop.price': { type: 'number', plugin: 'shop' },
    });
    expect(g.site.map(f => f.key)).toEqual(['subtitle']);
    expect(g.plugins.map(p => [p.plugin, p.name])).toEqual([['seo', 'SEO'], ['shop', 'shop']]);
    expect(g.plugins[0].fields.map(f => [f.key, f.type])).toEqual([['seo.title', 'text'], ['seo.noindex', 'boolean']]);
    expect(g.plugins[0].fields[1].label).toBe('Noindex');
  });

  it('handles empty input', () => {
    expect(groupFields(undefined)).toEqual({ site: [], plugins: [] });
  });
});
