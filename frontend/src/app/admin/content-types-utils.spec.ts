import { checkType, emptyField, emptyType, idFrom, moved, parseOptions, rowToForm, specBody } from './content-types-utils';

describe('content types (no code)', () => {
  const ok = () => ({ ...emptyType(), type: 'property', label: { th: 'ประกาศ', en: 'Properties' }, slug: 'homes' });

  it('makes ASCII ids from labels', () => {
    expect(idFrom('Real Estate!', '_')).toBe('real_estate');
    expect(idFrom('Café Menu', '-')).toBe('cafe-menu');
    expect(idFrom('ประกาศขายบ้าน', '_')).toBe('');
  });

  it('checks the type like the server', () => {
    expect(checkType(ok(), [], true)).toBeNull();
    expect(checkType({ ...ok(), label: { th: '', en: '' } }, [], true)?.key).toBe('ct.err.label');
    expect(checkType({ ...ok(), type: 'A' }, [], true)?.key).toBe('ct.err.type');
    expect(checkType({ ...ok(), type: 'page' }, [], true)?.key).toBe('ct.err.builtin');
    expect(checkType(ok(), [{ type: 'property', slug: 'x' }], true)?.key).toBe('ct.err.typeTaken');
    expect(checkType(ok(), [{ type: 'event', slug: 'homes' }], true)?.key).toBe('ct.err.slugTaken');
    expect(checkType({ ...ok(), slug: 'Homes' }, [], true)?.key).toBe('ct.err.slug');
  });

  it('checks fields: keys, duplicates, select options, repeater rows', () => {
    const f = (x: object) => ({ ...ok(), fields: [{ ...emptyField(), key: 'price', ...x }] });
    expect(checkType(f({ key: '1x' }), [], true)).toEqual({ key: 'ct.err.key', x: '1x' });
    expect(checkType({ ...ok(), fields: [{ ...emptyField(), key: 'a' }, { ...emptyField(), key: 'a' }] }, [], true)?.key).toBe('ct.err.keyTwice');
    expect(checkType(f({ type: 'select', options: '' }), [], true)?.key).toBe('ct.err.options');
    expect(checkType(f({ type: 'number', min: 5, max: 1 }), [], true)?.key).toBe('ct.err.minMax');
    expect(checkType(f({ type: 'repeater', fields: [] }), [], true)?.key).toBe('ct.err.repeater');
    expect(checkType(f({ type: 'repeater', fields: [{ ...emptyField(), key: 'room' }] }), [], true)).toBeNull();
  });

  it('builds the request body: title always, options parsed, number limits only when set', () => {
    const body = specBody({ ...ok(), supports: ['content'], fields: [
      { ...emptyField('select'), key: 'kind', options: 'house | บ้านเดี่ยว\ncondo' },
      { ...emptyField('number'), key: 'price', min: 0, max: null },
    ] });
    expect(body['supports']).toEqual(['title', 'content']);
    const fields = body['fields'] as Record<string, unknown>[];
    expect(fields[0]['options']).toEqual([{ value: 'house', label: { th: 'บ้านเดี่ยว', en: 'บ้านเดี่ยว' } }, { value: 'condo', label: { th: 'condo', en: 'condo' } }]);
    expect(fields[1]).toEqual({ key: 'price', type: 'number', label: { th: '', en: '' }, min: 0 });
  });

  it('round-trips a server row into the form', () => {
    const f = rowToForm({ type: 'property', label: { th: 'ประกาศ', en: 'Properties' }, slug: 'homes', public: true, supports: ['title', 'image'],
      fields: [{ key: 'kind', type: 'select', label: { th: 'ประเภท' }, options: [{ value: 'house', label: { th: 'บ้าน' } }] }], postCount: 0, active: true });
    expect(f.supports).toEqual(['image']);
    expect(f.fields[0].options).toBe('house | บ้าน');
    expect(parseOptions('a\n\n b | B ')).toEqual([{ value: 'a', label: { th: 'a', en: 'a' } }, { value: 'b', label: { th: 'B', en: 'B' } }]);
  });

  it('moves rows', () => {
    expect(moved([1, 2, 3], 0, 1)).toEqual([2, 1, 3]);
    expect(moved([1, 2, 3], 0, -1)).toEqual([1, 2, 3]);
  });
});
