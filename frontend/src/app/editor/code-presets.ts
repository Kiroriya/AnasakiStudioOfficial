import { tr } from '../core/i18n';
import { CodeBindings } from '../core/code-models';
import { DNode, InputKind, newId } from './design-types';
import { collectionChoices, sourceParams } from './code-logic';
import './messages-sitecode';

/** New pagination node, optionally bound to a collection node. */
export function createPagination(collection?: string): DNode {
  return {
    id: newId(), type: 'pagination', name: tr('ed.node.pagination'), width: { mode: 'fill' }, height: { mode: 'hug' },
    layout: { mode: 'horizontal', gap: 6, align: 'center', justify: 'center', wrap: true },
    props: { collection, style: 'numbers', window: 2 },
  };
}

/** New input (form field) node. */
export function createInput(kind: InputKind = 'text', extra: DNode['props'] = {}): DNode {
  const box = kind === 'checkbox' || kind === 'hidden'
    ? { style: { fontSize: 15, color: '#111827' } }
    : { layout: { padding: [10, 14, 10, 14] as [number, number, number, number] }, style: { fill: '#ffffff', stroke: '#d1d5db', strokeWidth: 1, radius: 8, fontSize: 15, color: '#111827' } };
  return {
    id: newId(), type: 'input', name: tr('ed.node.input'), width: kind === 'checkbox' || kind === 'hidden' ? { mode: 'hug' } : { mode: 'fill' }, height: { mode: 'hug' },
    ...box,
    props: { kind, name: kind === 'search' ? 'q' : 'field', placeholder: kind === 'checkbox' || kind === 'hidden' ? undefined : tr('sc.in.defPlaceholder'), ...extra },
  };
}

/** The first code:/table: collection of the page and a param of it whose name matches. */
function targetFor(root: DNode, b: CodeBindings | null, wanted: RegExp, fallback: string): { collection?: string; param: string } {
  const c = collectionChoices(root)[0];
  if (!c) return { param: fallback };
  const params = Object.keys(sourceParams(c.source, b));
  return { collection: c.id, param: params.find(p => wanted.test(p)) ?? params[0] ?? fallback };
}

/** Preset "Search box": a search input filtering the page's first collection. */
export function searchBoxPreset(root: DNode, b: CodeBindings | null): DNode {
  const t = targetFor(root, b, /^(q|query|search|keyword|s)$/i, 'q');
  const n = createInput('search', { name: t.param, placeholder: tr('sc.preset.searchPh') });
  n.name = tr('sc.preset.search');
  if (t.collection) n.props!.filter = { collection: t.collection, param: t.param };
  return n;
}

/** Preset "Category filter": a select filled from a categories collection, filtering the page's first collection. */
export function categoryFilterPreset(root: DNode, b: CodeBindings | null): DNode {
  const t = targetFor(root, b, /categor|type|tag|group/i, 'category');
  const src = b?.collections.find(c => /categor/i.test(c.name));
  const fields = Object.keys(src?.fields ?? {});
  const n = createInput('select', { name: t.param, placeholder: tr('sc.preset.allCategories') });
  n.name = tr('sc.preset.category');
  if (src) {
    n.props!.optionsFrom = {
      collection: 'code:' + src.name,
      value: fields.find(f => /^(slug|id|value|key)$/.test(f)) ?? fields[0] ?? 'id',
      label: fields.find(f => /^(name|title|label)$/.test(f)) ?? fields[0] ?? 'name',
    };
  } else n.props!.options = [];
  if (t.collection) n.props!.filter = { collection: t.collection, param: t.param };
  return n;
}

/** Preset "Submit button wired to action": a button calling the first site.action. */
export function submitButtonPreset(b: CodeBindings | null): DNode {
  const a = b?.actions[0];
  return {
    id: newId(), type: 'button', name: tr('sc.preset.submit'), width: { mode: 'hug' }, height: { mode: 'hug' },
    layout: { padding: [12, 24, 12, 24] }, style: { fill: '#4f46e5', color: '#ffffff', radius: 10, fontWeight: 600, fontSize: 16 },
    props: { text: tr('sc.preset.submitText'), action: { type: 'code', name: a?.name ?? '', input: {}, success: { message: tr('sc.preset.submitDone') } } },
  };
}
