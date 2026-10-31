/**
 * Binding expression language (Site Code phase 3) — `{{ item.price * item.qty | number:2 }}`.
 *
 * Mirrors the Go evaluator in backend/internal/design (shared test vectors:
 * testdata/expr_vectors.json). A small, safe language: literals, dotted paths,
 * unary `- !`, binary `* / % + - < <= > >= == != && || ??`, ternary `?:`,
 * parentheses and pipes `| name:arg`. No calls, indexing or assignment.
 * Phase 4a: root `visitor` (site.visitor) and `visitor.is.<group>`.
 */

export type Ast =
  | { k: 'lit'; v: unknown }
  | { k: 'path'; root: string; segs: string[] }
  | { k: 'un'; op: '-' | '!'; x: Ast }
  | { k: 'bin'; op: string; a: Ast; b: Ast }
  | { k: 'cond'; c: Ast; a: Ast; b: Ast }
  | { k: 'pipe'; x: Ast; name: string; args: Ast[] };

/** Why an expression was rejected (code → i18n key `expr.err.<code>`). */
export interface ExprError { code: 'unexpected' | 'end' | 'assign' | 'call' | 'index' | 'string' | 'char' | 'empty'; pos: number; token?: string }

export type ParseResult = { ok: true; ast: Ast } | { ok: false; error: ExprError };

export interface EvalOptions {
  /** Month names: 'th' (ม.ค., มกราคม) or 'en'. */
  locale?: 'th' | 'en';
  /** IANA time zone dates are shown in (default UTC). */
  timeZone?: string;
  /** Formatter names seen that this evaluator does not know (script formatters run on the server only). */
  unknown?: Set<string>;
}

/** Evaluation context: item, page, post, site, query, visitor, $index, $empty. */
export type ExprCtx = Record<string, unknown>;

/** Root names a path may start with; anything else is null. */
export const ROOTS = ['item', 'page', 'post', 'site', 'query', 'visitor', '$index', '$empty'] as const;
const BLOCKED = new Set(['__proto__', 'constructor', 'prototype']);
export const CORE_FORMATTERS = ['number', 'date', 'upper', 'lower', 'truncate', 'default', 'json'] as const;

// ------------------------------------------------------------------ lexer

interface Tok { t: 'num' | 'str' | 'id' | 'op' | 'end'; v: string; pos: number; n?: number }

const OPS = ['<=', '>=', '==', '!=', '&&', '||', '??', '*', '/', '%', '+', '-', '<', '>', '!', '?', ':', '(', ')', '.', '|'];

function lex(src: string): Tok[] | ExprError {
  const out: Tok[] = [];
  let i = 0;
  while (i < src.length) {
    const c = src[i];
    if (/\s/.test(c)) { i++; continue; }
    if (/[0-9]/.test(c)) {
      const m = /^[0-9]+(\.[0-9]+)?/.exec(src.slice(i))!;
      out.push({ t: 'num', v: m[0], n: Number(m[0]), pos: i });
      i += m[0].length;
      continue;
    }
    if (c === '"' || c === "'") {
      let j = i + 1, s = '';
      for (; j < src.length && src[j] !== c; j++) {
        if (src[j] === '\\' && j + 1 < src.length) { j++; s += ({ n: '\n', t: '\t' } as Record<string, string>)[src[j]] ?? src[j]; }
        else s += src[j];
      }
      if (j >= src.length) return { code: 'string', pos: i };
      out.push({ t: 'str', v: s, pos: i });
      i = j + 1;
      continue;
    }
    if (/[A-Za-z_$]/.test(c)) {
      const m = /^[A-Za-z_$][A-Za-z0-9_$]*/.exec(src.slice(i))!;
      out.push({ t: 'id', v: m[0], pos: i });
      i += m[0].length;
      continue;
    }
    const op = OPS.find(o => src.startsWith(o, i));
    if (op) { out.push({ t: 'op', v: op, pos: i }); i += op.length; continue; }
    if (c === '=') return { code: 'assign', pos: i, token: '=' };
    if (c === '[' || c === ']') return { code: 'index', pos: i, token: c };
    return { code: 'char', pos: i, token: c };
  }
  out.push({ t: 'end', v: '', pos: src.length });
  return out;
}

// ------------------------------------------------------------------ parser

class Fail { constructor(readonly e: ExprError) {} }

class Parser {
  private i = 0;
  constructor(private toks: Tok[]) {}
  private get cur() { return this.toks[this.i]; }
  private atEnd() { return this.toks[this.i].t === 'end'; }
  private isOp(v: string) { return this.cur.t === 'op' && this.cur.v === v; }
  private eat(v: string) { if (this.isOp(v)) { this.i++; return true; } return false; }
  private fail(tok = this.cur): never {
    if (tok.t === 'end') throw new Fail({ code: 'end', pos: tok.pos });
    if (tok.t === 'op' && tok.v === '(') throw new Fail({ code: 'call', pos: tok.pos, token: '(' });
    throw new Fail({ code: 'unexpected', pos: tok.pos, token: tok.v });
  }

  parseAll(): Ast {
    if (this.cur.t === 'end') throw new Fail({ code: 'empty', pos: 0 });
    const a = this.pipe();
    if (!this.atEnd()) this.fail();
    return a;
  }

  private pipe(): Ast {
    let x = this.ternary();
    while (this.eat('|')) {
      if (this.cur.t !== 'id') this.fail();
      let name = this.cur.v;
      this.i++;
      // phase 6: plugin formatters `ns:name` (`| thai:baht`) — written without spaces, the
      // namespace is not a core formatter and the second identifier is not a root/path (else it is an argument)
      const colon = this.cur, nx = this.toks[this.i + 1], after = this.toks[this.i + 2];
      if (colon.t === 'op' && colon.v === ':' && nx?.t === 'id' && colon.pos === this.toks[this.i - 1].pos + name.length && nx.pos === colon.pos + 1
        && !(CORE_FORMATTERS as readonly string[]).includes(name) && !(ROOTS as readonly string[]).includes(nx.v) && /^[A-Za-z_][A-Za-z0-9_]*$/.test(nx.v)
        && !(after?.t === 'op' && after.v === '.')) {
        name = name + ':' + nx.v;
        this.i += 2;
      }
      const args: Ast[] = [];
      while (this.eat(':')) args.push(this.arg());
      x = { k: 'pipe', x, name, args };
    }
    return x;
  }

  /** Formatter argument: a literal (optionally negative number) or a path. */
  private arg(): Ast {
    if (this.isOp('-') && this.toks[this.i + 1]?.t === 'num') { this.i++; const n = this.cur.n!; this.i++; return { k: 'lit', v: -n }; }
    const t = this.cur;
    if (t.t === 'num') { this.i++; return { k: 'lit', v: t.n }; }
    if (t.t === 'str') { this.i++; return { k: 'lit', v: t.v }; }
    if (t.t === 'id') return this.primary();
    this.fail();
  }

  private ternary(): Ast {
    const c = this.binary(0);
    if (!this.eat('?')) return c;
    const a = this.ternary();
    if (!this.eat(':')) this.fail();
    const b = this.ternary();
    return { k: 'cond', c, a, b };
  }

  private static LEVELS = [['??'], ['||'], ['&&'], ['==', '!='], ['<', '<=', '>', '>='], ['+', '-'], ['*', '/', '%']];

  private binary(level: number): Ast {
    if (level >= Parser.LEVELS.length) return this.unary();
    let a = this.binary(level + 1);
    for (;;) {
      const t = this.cur;
      if (t.t !== 'op' || !Parser.LEVELS[level].includes(t.v)) return a;
      this.i++;
      const b = this.binary(level + 1);
      a = { k: 'bin', op: t.v, a, b };
    }
  }

  private unary(): Ast {
    if (this.isOp('-') || this.isOp('!')) {
      const op = this.cur.v as '-' | '!';
      this.i++;
      return { k: 'un', op, x: this.unary() };
    }
    return this.primary();
  }

  private primary(): Ast {
    const t = this.cur;
    let x: Ast;
    if (t.t === 'num') { this.i++; x = { k: 'lit', v: t.n }; }
    else if (t.t === 'str') { this.i++; x = { k: 'lit', v: t.v }; }
    else if (t.t === 'id') {
      this.i++;
      if (t.v === 'true' || t.v === 'false') x = { k: 'lit', v: t.v === 'true' };
      else if (t.v === 'null') x = { k: 'lit', v: null };
      else {
        const segs: string[] = [];
        while (this.isOp('.')) {
          this.i++;
          if (this.cur.t !== 'id') this.fail();
          segs.push(this.cur.v);
          this.i++;
        }
        x = { k: 'path', root: t.v, segs };
      }
    } else if (this.eat('(')) {
      x = this.pipe();
      if (!this.eat(')')) this.fail();
    } else this.fail();
    if (this.isOp('(')) throw new Fail({ code: 'call', pos: this.cur.pos, token: '(' });
    return x;
  }
}

const cache = new Map<string, ParseResult>();

/** Parses an expression (cached). */
export function parseExpr(src: string): ParseResult {
  const hit = cache.get(src);
  if (hit) return hit;
  let r: ParseResult;
  const toks = lex(src);
  if (!Array.isArray(toks)) r = { ok: false, error: toks };
  else {
    try { r = { ok: true, ast: new Parser(toks).parseAll() }; } catch (e) {
      if (!(e instanceof Fail)) throw e;
      r = { ok: false, error: e.e };
    }
  }
  if (cache.size > 2000) cache.clear();
  cache.set(src, r);
  return r;
}

// ------------------------------------------------------------------ values

const NUM_RE = /^-?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$/;

/** A number, or a numeric string (DECIMAL columns arrive as strings). */
export function isNumeric(v: unknown): boolean {
  return (typeof v === 'number' && Number.isFinite(v)) || (typeof v === 'string' && NUM_RE.test(v));
}
function toNum(v: unknown): number { return typeof v === 'number' ? v : Number(v); }

/** false, null, '', 0, '0', NaN and empty arrays are falsy; everything else is truthy. */
export function truthy(v: unknown): boolean {
  if (v === null || v === undefined || v === false || v === '' || v === 0 || v === '0') return false;
  if (typeof v === 'number' && Number.isNaN(v)) return false;
  if (Array.isArray(v)) return v.length > 0;
  return true;
}

/** Up to 10 decimals, no trailing zeros, integers without ".0". */
export function formatNumber(n: number): string {
  if (!Number.isFinite(n)) return '';
  const r = Math.round(n * 1e10) / 1e10;
  if (Object.is(r, -0) || r === 0) return '0';
  if (Math.abs(r) >= 1e21) return String(r);
  let s = r.toFixed(10);
  if (s.includes('.')) s = s.replace(/0+$/, '').replace(/\.$/, '');
  return s === '-0' ? '0' : s;
}

/** How a value prints in a template: null → '', booleans → 'true'/'false', arrays/objects → '' (use | json). */
export function toText(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'string') return v;
  if (typeof v === 'number') return formatNumber(v);
  if (typeof v === 'boolean') return v ? 'true' : 'false';
  return '';
}

/**
 * `visitor.is.<group>` (phase 4a): true when the site visitor (site.visitor)
 * is in that group, false otherwise — also when nobody is signed in. A
 * helper instead of an `in` operator; `is` is reserved on visitor.
 */
function visitorIs(ctx: ExprCtx, group: string): boolean {
  const v = Object.prototype.hasOwnProperty.call(ctx, 'visitor') ? ctx['visitor'] : null;
  if (!v || typeof v !== 'object' || Array.isArray(v)) return false;
  const groups = (v as Record<string, unknown>)['groups'];
  return Array.isArray(groups) && groups.some(g => g === group);
}

function lookup(ctx: ExprCtx, root: string, segs: string[]): unknown {
  if (root === 'visitor' && segs[0] === 'is') return segs.length === 2 && !BLOCKED.has(segs[1]) ? visitorIs(ctx, segs[1]) : null;
  if (!(ROOTS as readonly string[]).includes(root) || !Object.prototype.hasOwnProperty.call(ctx, root)) return null;
  let v: unknown = ctx[root];
  for (const s of segs) {
    if (v === null || v === undefined || BLOCKED.has(s)) return null;
    if (s === 'length' && (Array.isArray(v) || typeof v === 'string')) { v = Array.isArray(v) ? v.length : Array.from(v).length; continue; }
    if (typeof v !== 'object' || Array.isArray(v) || !Object.prototype.hasOwnProperty.call(v, s)) return null;
    v = (v as Record<string, unknown>)[s];
  }
  return v === undefined ? null : v;
}

function equals(a: unknown, b: unknown): boolean {
  const an = a === null || a === undefined, bn = b === null || b === undefined;
  if (an || bn) return an && bn;
  if ((typeof a === 'number' || typeof b === 'number') && isNumeric(a) && isNumeric(b)) return toNum(a) === toNum(b);
  if (typeof a !== typeof b) return false;
  if (typeof a === 'object') return JSON.stringify(a) === JSON.stringify(b);
  return a === b;
}

function compare(op: string, a: unknown, b: unknown): boolean {
  let x: number | string, y: number | string;
  if (isNumeric(a) && isNumeric(b)) { x = toNum(a); y = toNum(b); }
  else if (typeof a === 'string' && typeof b === 'string') { x = a; y = b; }
  else return false;
  switch (op) {
    case '<': return x < y;
    case '<=': return x <= y;
    case '>': return x > y;
    default: return x >= y;
  }
}

function arith(op: string, a: unknown, b: unknown): unknown {
  if (a === null || a === undefined || b === null || b === undefined) return null;
  if (op === '+' && !(isNumeric(a) && isNumeric(b))) {
    if (typeof a === 'string' || typeof b === 'string') return toText(a) + toText(b);
    return null;
  }
  if (!isNumeric(a) || !isNumeric(b)) return null;
  const x = toNum(a), y = toNum(b);
  switch (op) {
    case '+': return x + y;
    case '-': return x - y;
    case '*': return x * y;
    case '/': return y === 0 ? null : x / y;
    default: return y === 0 ? null : x % y;
  }
}

/** Evaluates a parsed expression. */
export function evaluate(ast: Ast, ctx: ExprCtx, opts: EvalOptions = {}): unknown {
  switch (ast.k) {
    case 'lit': return ast.v;
    case 'path': return lookup(ctx, ast.root, ast.segs);
    case 'un': {
      const v = evaluate(ast.x, ctx, opts);
      if (ast.op === '!') return !truthy(v);
      return isNumeric(v) ? -toNum(v) : null;
    }
    case 'cond': return truthy(evaluate(ast.c, ctx, opts)) ? evaluate(ast.a, ctx, opts) : evaluate(ast.b, ctx, opts);
    case 'pipe': {
      const v = evaluate(ast.x, ctx, opts);
      return applyFormatter(ast.name, v, ast.args.map(a => evaluate(a, ctx, opts)), opts);
    }
    case 'bin': {
      const { op } = ast;
      if (op === '&&') return truthy(evaluate(ast.a, ctx, opts)) && truthy(evaluate(ast.b, ctx, opts));
      if (op === '||') return truthy(evaluate(ast.a, ctx, opts)) || truthy(evaluate(ast.b, ctx, opts));
      if (op === '??') { const a = evaluate(ast.a, ctx, opts); return a === null || a === undefined ? evaluate(ast.b, ctx, opts) : a; }
      const a = evaluate(ast.a, ctx, opts), b = evaluate(ast.b, ctx, opts);
      if (op === '==') return equals(a, b);
      if (op === '!=') return !equals(a, b);
      if (op === '<' || op === '<=' || op === '>' || op === '>=') return compare(op, a, b);
      return arith(op, a, b);
    }
  }
}

/** Parses and evaluates; a parse error gives `{ error }`. */
export function evalExpr(src: string, ctx: ExprCtx, opts: EvalOptions = {}): { value: unknown; error?: ExprError } {
  const p = parseExpr(src.trim());
  if (!p.ok) return { value: null, error: p.error };
  return { value: evaluate(p.ast, ctx, opts) };
}

/** The value printed for an expression ('' on parse errors). */
export function exprText(src: string, ctx: ExprCtx, opts: EvalOptions = {}): string {
  const r = evalExpr(src, ctx, opts);
  return r.error ? '' : toText(r.value);
}

// ------------------------------------------------------------------ templates

const TPL_RE = /\{\{([\s\S]*?)\}\}/g;

/**
 * Replaces every `{{ expr }}` in a text. Expressions that do not parse stay
 * as written (the editor shows the error). `escape` HTML-escapes the values
 * (rich text templates) — entities inside the expression are decoded first.
 */
export function renderTemplate(text: string | undefined, ctx: ExprCtx, opts: EvalOptions = {}, escape?: (s: string) => string): string {
  if (!text) return '';
  if (!text.includes('{{')) return text;
  return text.replace(TPL_RE, (m, body: string) => {
    const src = escape ? decodeEntities(body) : body;
    const r = evalExpr(src, ctx, opts);
    if (r.error) return m;
    const s = toText(r.value);
    return escape ? escape(s) : s;
  });
}

/** Expressions inside a template with their parse errors (for inline error display). */
export function templateErrors(text: string | undefined): { expr: string; error: ExprError }[] {
  const out: { expr: string; error: ExprError }[] = [];
  if (!text?.includes('{{')) return out;
  for (const m of text.matchAll(TPL_RE)) {
    const p = parseExpr(m[1].trim());
    if (!p.ok) out.push({ expr: m[1].trim(), error: p.error });
  }
  return out;
}

export function decodeEntities(s: string): string {
  return s.replace(/&(amp|lt|gt|quot|#39|apos|nbsp);/g, (_, e: string) => ({ amp: '&', lt: '<', gt: '>', quot: '"', '#39': "'", apos: "'", nbsp: ' ' } as Record<string, string>)[e]);
}

/** Paths an expression reads (`item.title`, `page.total`, …) — used to find `$empty` and item fields. */
export function exprPaths(src: string): string[] {
  const p = parseExpr(src.trim());
  if (!p.ok) return [];
  const out: string[] = [];
  const walk = (a: Ast) => {
    switch (a.k) {
      case 'path': out.push([a.root, ...a.segs].join('.')); break;
      case 'un': walk(a.x); break;
      case 'bin': walk(a.a); walk(a.b); break;
      case 'cond': walk(a.c); walk(a.a); walk(a.b); break;
      case 'pipe': walk(a.x); a.args.forEach(walk); break;
    }
  };
  walk(p.ast);
  return out;
}

// ------------------------------------------------------------------ formatters

const MONTHS: Record<'th' | 'en', { long: string[]; short: string[] }> = {
  en: {
    long: ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'],
    short: ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'],
  },
  th: {
    long: ['มกราคม', 'กุมภาพันธ์', 'มีนาคม', 'เมษายน', 'พฤษภาคม', 'มิถุนายน', 'กรกฎาคม', 'สิงหาคม', 'กันยายน', 'ตุลาคม', 'พฤศจิกายน', 'ธันวาคม'],
    short: ['ม.ค.', 'ก.พ.', 'มี.ค.', 'เม.ย.', 'พ.ค.', 'มิ.ย.', 'ก.ค.', 'ส.ค.', 'ก.ย.', 'ต.ค.', 'พ.ย.', 'ธ.ค.'],
  },
};

function applyFormatter(name: string, v: unknown, args: unknown[], opts: EvalOptions): unknown {
  switch (name) {
    case 'number': {
      if (!isNumeric(v)) return v;
      const n = toNum(v);
      const d = args.length && isNumeric(args[0]) ? Math.max(0, Math.min(20, Math.trunc(toNum(args[0])))) : -1;
      return groupThousands(d >= 0 ? n.toFixed(d) : formatNumber(n));
    }
    case 'date': return formatDate(v, typeof args[0] === 'string' ? args[0] : 'D MMM YYYY', opts);
    case 'upper': return v === null || v === undefined ? v : toText(v).toUpperCase();
    case 'lower': return v === null || v === undefined ? v : toText(v).toLowerCase();
    case 'truncate': {
      if (v === null || v === undefined) return v;
      const n = isNumeric(args[0]) ? Math.max(0, Math.trunc(toNum(args[0]))) : 0;
      const chars = Array.from(toText(v));
      return chars.length > n ? chars.slice(0, n).join('') + '…' : chars.join('');
    }
    case 'default': return v === null || v === undefined || v === '' ? (args[0] ?? '') : v;
    case 'json': return JSON.stringify(v === undefined ? null : v);
    default:
      opts.unknown?.add(name);
      return v;
  }
}

function groupThousands(s: string): string {
  const neg = s.startsWith('-');
  const [int, frac] = (neg ? s.slice(1) : s).split('.');
  const g = int.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  return (neg ? '-' : '') + g + (frac !== undefined ? '.' + frac : '');
}

interface DateParts { y: number; mo: number; d: number; h: number; mi: number }

/** "YYYY-MM-DD" stays that calendar day; date-times are shown in the site time zone (no offset = UTC). */
export function dateParts(v: unknown, timeZone?: string): DateParts | null {
  if (typeof v === 'string') {
    const day = /^(\d{4})-(\d{2})-(\d{2})$/.exec(v);
    if (day) return { y: +day[1], mo: +day[2], d: +day[3], h: 0, mi: 0 };
    const m = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2})(?::(\d{2})(?:\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?$/.exec(v);
    if (!m) return null;
    let ms = Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +(m[6] ?? 0));
    if (m[7] && m[7] !== 'Z') {
      const sign = m[7][0] === '-' ? -1 : 1;
      const hh = +m[7].slice(1, 3), mm = +m[7].slice(-2);
      ms -= sign * (hh * 60 + mm) * 60000;
    }
    return zoned(ms, timeZone);
  }
  if (typeof v === 'number' && Number.isFinite(v)) return zoned(v * 1000, timeZone); // unix seconds
  return null;
}

function zoned(ms: number, timeZone?: string): DateParts | null {
  const dt = new Date(ms);
  if (Number.isNaN(dt.getTime())) return null;
  if (!timeZone || timeZone === 'UTC') return { y: dt.getUTCFullYear(), mo: dt.getUTCMonth() + 1, d: dt.getUTCDate(), h: dt.getUTCHours(), mi: dt.getUTCMinutes() };
  try {
    const f = new Intl.DateTimeFormat('en-US', { timeZone, year: 'numeric', month: 'numeric', day: 'numeric', hour: 'numeric', minute: 'numeric', hourCycle: 'h23' });
    const p: Record<string, number> = {};
    for (const x of f.formatToParts(dt)) if (x.type !== 'literal') p[x.type] = Number(x.value);
    return { y: p['year'], mo: p['month'], d: p['day'], h: p['hour'] % 24, mi: p['minute'] };
  } catch {
    return zoned(ms);
  }
}

const DATE_TOKENS = ['BBBB', 'YYYY', 'MMMM', 'MMM', 'MM', 'M', 'DD', 'D', 'YY', 'HH', 'H', 'mm', 'm'];

/** date:'fmt' — tokens D DD M MM MMM MMMM YY YYYY BBBB H HH m mm; other characters are copied. */
export function formatDate(v: unknown, fmt: string, opts: EvalOptions = {}): unknown {
  const p = dateParts(v, opts.timeZone);
  if (!p) return v;
  const names = MONTHS[opts.locale === 'th' ? 'th' : 'en'];
  const pad = (n: number) => String(n).padStart(2, '0');
  let out = '';
  for (let i = 0; i < fmt.length;) {
    const tok = DATE_TOKENS.find(t => fmt.startsWith(t, i));
    if (!tok) { out += fmt[i++]; continue; }
    i += tok.length;
    switch (tok) {
      case 'BBBB': out += p.y + 543; break;
      case 'YYYY': out += p.y; break;
      case 'YY': out += pad(p.y % 100); break;
      case 'MMMM': out += names.long[p.mo - 1]; break;
      case 'MMM': out += names.short[p.mo - 1]; break;
      case 'MM': out += pad(p.mo); break;
      case 'M': out += p.mo; break;
      case 'DD': out += pad(p.d); break;
      case 'D': out += p.d; break;
      case 'HH': out += pad(p.h); break;
      case 'H': out += p.h; break;
      case 'mm': out += pad(p.mi); break;
      case 'm': out += p.mi; break;
    }
  }
  return out;
}
