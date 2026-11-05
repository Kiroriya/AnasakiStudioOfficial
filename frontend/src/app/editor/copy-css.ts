// "Copy CSS" (CMS 2.2.2, like Figma's Dev Mode): the selected layer's CSS as the site renders it — the rule of its
// class (n-<id>, design.ClassName) from nodeCss, which mirrors css.go.
import { nodeCss } from './design-css';
import type { DNode, LayoutMode } from './design-types';

/** design.ClassName: "n-" + the id's letters, digits, - and _. */
export function className(id: string): string { return 'n-' + id.replace(/[^A-Za-z0-9_-]/g, ''); }

export function cssText(n: DNode, parentMode: LayoutMode | ''): string {
  const decls = nodeCss(n, parentMode);
  const body = Object.entries(decls).map(([p, v]) => `  ${p}: ${v};\n`).join('');
  const title = (n.name || n.type).replace(/\*\//g, '* /');
  return `/* ${title} */\n.${className(n.id)} {\n${body}}\n`;
}
