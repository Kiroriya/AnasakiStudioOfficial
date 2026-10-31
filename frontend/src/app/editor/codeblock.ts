import { DNode } from './design-types';

/**
 * "Frontend code" block (node type `codeblock`, props {html, css, js}) — pure helpers shared by
 * the canvas, the inspector and the insert panel. No Angular, no Monaco (unit tested in
 * codeblock.spec.ts; the CSS scoping cases live in testdata/codeblock_scope_vectors.json and
 * are mirrored by the Go renderer).
 */

/** Capability needed to add or change a frontend code block (the server enforces it too). */
export const CODEBLOCK_CAP = 'edit_scripts';

/** What the current user may do with frontend code blocks. */
export function codeBlockAccess(can: (cap: string) => boolean): { insert: boolean; edit: boolean } {
  const ok = can(CODEBLOCK_CAP);
  return { insert: ok, edit: ok };
}

/** Does the tree contain a frontend code block? */
export function hasCodeBlock(n: DNode | null | undefined): boolean {
  if (!n) return false;
  if (n.type === 'codeblock') return true;
  return (n.children ?? []).some(hasCodeBlock);
}

/** Class put on the block root; the block's CSS is prefixed with `.` + this. */
export function codeBlockClass(id: string): string {
  return 'rea-cb-' + id.replace(/[^A-Za-z0-9_-]/g, '');
}

/** At-rules whose body holds style rules (scoped recursively). Other block at-rules are kept as is. */
const GROUP_AT = new Set(['media', 'supports', 'container', 'layer', 'scope', 'document', '-moz-document', 'starting-style']);
/** Statement at-rules that are dropped (they would load or re-encode outside the block). */
const DROP_AT = new Set(['import', 'charset']);

/**
 * Scopes a block's CSS to `.rea-cb-<id>` (mirrors the Go helper exactly — see the vectors):
 * - comments are removed; whitespace in selectors / at-rule preludes collapses to one space;
 * - every selector of a style rule gets the prefix (`a, b` → `.p a, .p b`); `:root` / `:scope`
 *   become the block class itself (`:scope > p` → `.p > p`);
 * - @media / @supports / @container / @layer / @scope / @document / @starting-style blocks are
 *   scoped recursively (empty ones dropped); @keyframes, @font-face, @page, @property… are kept
 *   verbatim; @import and @charset are dropped; other statement at-rules are kept;
 * - declarations are kept as written (trimmed): `sel { body }`, `sel {}` when empty; grouping
 *   at-rules are `prelude {\n…rules…\n}`; rules are joined with "\n";
 * - `</style` is written as `<\/style` so the CSS can't close the style element.
 */
export function scopeCss(css: string, id: string): string {
  const prefix = '.' + codeBlockClass(id);
  const src = stripComments(css ?? '');
  const p = new Parser(src, prefix);
  const out = p.rules(false).join('\n');
  return out.replace(/<\/(style)/gi, '<\\/$1');
}

/** `head { body }` (`head {}` when the body is empty). */
function wrap(head: string, body: string): string {
  const b = body.trim();
  return b ? head + ' { ' + b + ' }' : head + ' {}';
}

function collapse(s: string): string {
  return s.replace(/\s+/g, ' ').trim();
}

/** Removes comments (strings are kept intact). */
function stripComments(s: string): string {
  let out = '';
  let i = 0;
  while (i < s.length) {
    const c = s[i];
    if (c === '"' || c === "'") {
      const end = skipString(s, i);
      out += s.slice(i, end);
      i = end;
    } else if (c === '/' && s[i + 1] === '*') {
      const end = s.indexOf('*/', i + 2);
      i = end < 0 ? s.length : end + 2;
      out += ' ';
    } else {
      out += c;
      i++;
    }
  }
  return out;
}

/** Index just after the string starting at i (unterminated: end of input or line). */
function skipString(s: string, i: number): number {
  const q = s[i];
  let j = i + 1;
  while (j < s.length) {
    const c = s[j];
    if (c === '\\') { j += 2; continue; }
    if (c === q) return j + 1;
    if (c === '\n') return j;
    j++;
  }
  return s.length;
}

/** Splits a selector list at top-level commas (not inside (), [] or strings). */
export function splitSelectors(s: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (c === '"' || c === "'") { i = skipString(s, i) - 1; continue; }
    if (c === '\\') { i++; continue; }
    if (c === '(' || c === '[') depth++;
    else if ((c === ')' || c === ']') && depth > 0) depth--;
    else if (c === ',' && depth === 0) { out.push(s.slice(start, i)); start = i + 1; }
  }
  out.push(s.slice(start));
  return out;
}

const ROOT_RE = /:(?:root|scope)(?![\w-])/gi;

/** Prefixes every selector of a list; returns '' when nothing is left. */
export function scopeSelectors(list: string, prefix: string): string {
  const out: string[] = [];
  for (const raw of splitSelectors(collapse(list))) {
    const sel = raw.trim();
    if (!sel) continue;
    ROOT_RE.lastIndex = 0;
    if (ROOT_RE.test(sel)) {
      ROOT_RE.lastIndex = 0;
      out.push(sel.replace(ROOT_RE, prefix));
    } else {
      out.push(prefix + ' ' + sel);
    }
  }
  return out.join(', ');
}

class Parser {
  i = 0;
  constructor(private s: string, private prefix: string) {}

  private ws() {
    while (this.i < this.s.length && /[\s;]/.test(this.s[this.i])) this.i++;
  }

  /** Reads up to a top-level `{`, `;` or `}` (not consumed); returns the text and the stop char ('' at the end). */
  private prelude(): { text: string; stop: string } {
    const s = this.s;
    const start = this.i;
    let depth = 0;
    while (this.i < s.length) {
      const c = s[this.i];
      if (c === '"' || c === "'") { this.i = skipString(s, this.i); continue; }
      if (c === '\\') { this.i += 2; continue; }
      if (c === '(' || c === '[') depth++;
      else if ((c === ')' || c === ']') && depth > 0) depth--;
      else if (depth === 0 && (c === '{' || c === ';' || c === '}')) return { text: s.slice(start, this.i), stop: c };
      this.i++;
    }
    return { text: s.slice(start), stop: '' };
  }

  /** After a `{`: the raw body up to the matching `}` (consumed; missing = to the end). */
  private block(): string {
    const s = this.s;
    const start = this.i;
    let depth = 1;
    while (this.i < s.length) {
      const c = s[this.i];
      if (c === '"' || c === "'") { this.i = skipString(s, this.i); continue; }
      if (c === '\\') { this.i += 2; continue; }
      if (c === '{') depth++;
      else if (c === '}' && --depth === 0) {
        const body = s.slice(start, this.i);
        this.i++;
        return body;
      }
      this.i++;
    }
    return s.slice(start);
  }

  rules(nested: boolean): string[] {
    const s = this.s;
    const out: string[] = [];
    for (;;) {
      this.ws();
      if (this.i >= s.length) return out;
      if (s[this.i] === '}') {
        if (nested) { this.i++; return out; }
        this.i++; // stray
        continue;
      }
      const { text, stop } = this.prelude();
      const head = collapse(text);
      if (head.startsWith('@')) {
        const name = (/^@([\w-]+)/.exec(head)?.[1] ?? '').toLowerCase();
        if (stop !== '{') {
          if (stop === ';') this.i++;
          if (!DROP_AT.has(name) && name) out.push(head + ';');
          continue;
        }
        this.i++; // {
        if (GROUP_AT.has(name)) {
          const inner = this.rules(true);
          if (inner.length) out.push(head + ' {\n' + inner.join('\n') + '\n}');
        } else {
          const body = this.block();
          if (!DROP_AT.has(name)) out.push(wrap(head, body));
        }
        continue;
      }
      if (stop !== '{') {
        if (stop === ';') this.i++; // garbage declaration outside a rule
        continue;
      }
      this.i++; // {
      const body = this.block();
      const sel = scopeSelectors(head, this.prefix);
      if (sel) out.push(wrap(sel, body));
    }
  }
}

const DROP_TAGS = 'script,iframe,frame,frameset,object,embed,applet,base,meta,link,noscript,template,portal,animate,set';
const URL_ATTRS = new Set(['href', 'src', 'action', 'formaction', 'xlink:href', 'data', 'poster', 'background', 'srcset']);

/**
 * Makes block HTML safe to show on the designer canvas (JS never runs in the editor):
 * scripts, frames, objects, <base>/<meta>/<link>, inline event handlers, `srcdoc` and
 * `javascript:` / `data:text/html` URLs are removed; inline `<style>` elements are scoped
 * like the block CSS. Needs a DOM (the editor; jsdom in tests).
 */
export function canvasHtml(html: string, id: string): string {
  if (!html) return '';
  const tpl = document.createElement('template');
  tpl.innerHTML = html; // inert: nothing loads or runs while parsing a template
  const root = tpl.content;
  root.querySelectorAll(DROP_TAGS).forEach(e => e.remove());
  root.querySelectorAll('style').forEach(e => { e.textContent = scopeCss(e.textContent ?? '', id); });
  root.querySelectorAll('*').forEach(e => {
    for (const a of Array.from(e.attributes)) {
      const name = a.name.toLowerCase();
      if (name.startsWith('on') || name === 'srcdoc') e.removeAttribute(a.name);
      else if (URL_ATTRS.has(name) && /^\s*(javascript|vbscript|data:text\/html)/i.test(a.value.replace(/[\u0000-\u001f]/g, ''))) e.removeAttribute(a.name);
    }
  });
  const box = document.createElement('div');
  box.appendChild(root.cloneNode(true));
  return box.innerHTML;
}
