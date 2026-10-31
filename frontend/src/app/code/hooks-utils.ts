/**
 * Pure helpers of the Site Code "Hooks" tab (GET /code/hooks, phase 4b):
 * normalise the answer, group by kind (filters / events), filter by text and
 * unit. Subscribers keep the server's effective order (priority, then load
 * order). No Angular here — unit tested in hooks-utils.spec.ts.
 */
import { CodeHook, CodeHookSubscriber } from '../core/code-models';

export type HookKind = 'filter' | 'event' | 'other';
export interface HookGroup { kind: HookKind; hooks: CodeHook[] }

/** Accepts `[...]`, `{items}` or `{hooks}`; drops malformed rows; subscribers default to []. */
export function normalizeHooks(raw: unknown): CodeHook[] {
  const list = Array.isArray(raw) ? raw : (raw as { items?: unknown[]; hooks?: unknown[] } | null)?.items ?? (raw as { hooks?: unknown[] } | null)?.hooks ?? [];
  return (list as Partial<CodeHook>[])
    .filter(h => h && typeof h.name === 'string')
    .map(h => ({
      name: h.name!,
      kind: h.kind === 'filter' || h.kind === 'event' ? h.kind : String(h.kind ?? 'other'),
      declaredBy: h.declaredBy || 'core',
      subscribers: (Array.isArray(h.subscribers) ? h.subscribers : []).filter((s): s is CodeHookSubscriber => !!s && typeof s.unit === 'string')
        .map(s => ({ ...s, priority: Number.isFinite(s.priority) ? s.priority : 10 })),
    }));
}

/** Does the hook concern `unit` (declares it or subscribes to it)? '' = any unit. */
export function hookTouchesUnit(h: CodeHook, unit: string): boolean {
  return !unit || h.declaredBy === unit || h.subscribers.some(s => s.unit === unit);
}

/**
 * Filters first, then events, then anything else; inside a kind: core hooks
 * first, then by name. Text matches the hook name or a subscriber's unit/file.
 */
export function groupHooks(hooks: CodeHook[], q = '', unit = ''): HookGroup[] {
  const needle = q.trim().toLowerCase();
  const match = (h: CodeHook) => !needle || h.name.toLowerCase().includes(needle) || h.declaredBy.toLowerCase().includes(needle)
    || h.subscribers.some(s => s.unit.toLowerCase().includes(needle) || (s.file ?? '').toLowerCase().includes(needle));
  const kindOf = (k: string): HookKind => (k === 'filter' || k === 'event' ? k : 'other');
  const groups: HookGroup[] = (['filter', 'event', 'other'] as HookKind[]).map(kind => ({ kind, hooks: [] as CodeHook[] }));
  for (const h of hooks) if (match(h) && hookTouchesUnit(h, unit)) groups.find(g => g.kind === kindOf(h.kind))!.hooks.push(h);
  for (const g of groups) g.hooks.sort((a, b) => (a.declaredBy === 'core' ? 0 : 1) - (b.declaredBy === 'core' ? 0 : 1) || a.name.localeCompare(b.name));
  return groups.filter(g => g.hooks.length);
}

/** Subscribers in effective order: ascending priority; equal priorities keep the server (load) order. */
export function orderedSubscribers(subs: CodeHookSubscriber[]): CodeHookSubscriber[] {
  return subs.map((s, i) => [s, i] as const).sort(([a, i], [b, j]) => a.priority - b.priority || i - j).map(([s]) => s);
}

/** Units appearing anywhere in the list (for the unit filter), 'core' excluded, 'site' first. */
export function hookUnits(hooks: CodeHook[]): string[] {
  const set = new Set<string>();
  for (const h of hooks) {
    if (h.declaredBy !== 'core') set.add(h.declaredBy);
    for (const s of h.subscribers) set.add(s.unit);
  }
  return [...set].sort((a, b) => (a === 'site' ? -1 : b === 'site' ? 1 : a.localeCompare(b)));
}

/** "lib/price.ts:12" */
export const subscriberWhere = (s: CodeHookSubscriber) => (s.file ? s.file + (s.line ? ':' + s.line : '') : '');
