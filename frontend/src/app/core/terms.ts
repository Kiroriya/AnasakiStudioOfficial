import { Term } from './models';

/** A term in tree order with its depth (CMS 2.1.0 hierarchical taxonomies). */
export interface TermRow { term: Term; depth: number; indent: string }

/** Terms in tree order: each parent followed by its children (indented); orphans at the top level. */
export function termTree(terms: Term[]): TermRow[] {
  const ids = new Set(terms.map(t => t.id));
  const kids = new Map<number, Term[]>();
  for (const t of terms) {
    const p = t.parent && ids.has(t.parent) ? t.parent : 0;
    kids.set(p, [...(kids.get(p) ?? []), t]);
  }
  const out: TermRow[] = [];
  const seen = new Set<number>();
  const walk = (p: number, depth: number) => {
    for (const t of kids.get(p) ?? []) {
      if (seen.has(t.id)) continue; // a cycle never loops forever
      seen.add(t.id);
      out.push({ term: t, depth, indent: '— '.repeat(depth) });
      walk(t.id, depth + 1);
    }
  };
  walk(0, 0);
  return out;
}
