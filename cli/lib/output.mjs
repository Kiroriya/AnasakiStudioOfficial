// Output helpers: build problems in a VS Code problem-matcher friendly form
// and interactive prompts.
import readline from 'node:readline';

/** normalizeProblems accepts an array of problems or {errors, warnings}. */
export function normalizeProblems(p) {
  if (!p) return [];
  if (Array.isArray(p)) {
    return p.map((x) => ({ ...x, severity: x.severity || x.level || 'error' }));
  }
  const out = [];
  for (const x of p.errors || []) out.push({ ...x, severity: 'error' });
  for (const x of p.warnings || []) out.push({ ...x, severity: 'warning' });
  return out;
}

/**
 * formatProblem renders "path:line:col: error: message" with the path
 * relative to the project root (dirRel prefixes server paths).
 */
export function formatProblem(pr, dirRel = '.') {
  const sev = /^warn/i.test(pr.severity || '') ? 'warning' : 'error';
  const msg = String(pr.message ?? '').replace(/\r?\n/g, ' ');
  if (!pr.file) return `${sev}: ${msg}`;
  const file = dirRel && dirRel !== '.' ? `${dirRel}/${pr.file}` : pr.file;
  const line = pr.line > 0 ? pr.line : 1;
  const col = pr.column > 0 ? pr.column : 1;
  return `${file}:${line}:${col}: ${sev}: ${msg}`;
}

export function errorCount(problems) {
  return problems.filter((p) => !/^warn/i.test(p.severity || '')).length;
}

/** ask reads one line from ctx.stdin (input hidden when asked and on a TTY). */
export function ask(ctx, question, { hidden = false } = {}) {
  return new Promise((resolve) => {
    const output = ctx.stdout;
    const terminal = hidden && !!ctx.stdin.isTTY;
    const rl = readline.createInterface({ input: ctx.stdin, output, terminal });
    let answered = false;
    if (terminal) {
      rl._writeToOutput = (s) => {
        if (s.includes(question)) output.write(s);
        else if (s === '\r\n' || s === '\n') output.write(s);
      };
    }
    rl.question(question, (a) => {
      answered = true;
      rl.close();
      resolve(a.trim());
    });
    rl.on('close', () => { if (!answered) resolve(''); });
  });
}
