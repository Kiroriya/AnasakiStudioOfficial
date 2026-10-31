// Command dispatch. main(argv, io) returns the exit code; io lets tests run
// the CLI in-process (cwd, env, streams, stdin, abort signal).
import fs from 'node:fs';
import path from 'node:path';
import { ApiError, apiFailure, createClient } from './api.mjs';
import {
  CONFIG_FILE, credentialsFile, findProject, normalizeSite, readCredentials, readProject,
  tokenFor, writeCredentials, writeProject,
} from './config.mjs';
import { CliError, EXIT_CONFLICT, EXIT_ERROR, EXIT_OK } from './errors.mjs';
import { IGNORE_FILE, loadIgnore } from './files.mjs';
import { detectLang, HELP, translator } from './i18n.mjs';
import { ask, errorCount, formatProblem, normalizeProblems } from './output.mjs';
import { LOCAL_KINDS, pull, push, refreshTypes, snapshot, status } from './sync.mjs';
import { createDebouncedRunner, startWatcher } from './watch.mjs';

export const VERSION = JSON.parse(fs.readFileSync(new URL('../package.json', import.meta.url), 'utf8')).version;

const SPEC = {
  login: { values: ['token'], flags: [] },
  logout: { values: [], flags: [] },
  init: { values: ['dir', 'token'], flags: [] },
  pull: { values: [], flags: ['force'] },
  push: { values: [], flags: ['dry-run', 'delete'] },
  status: { values: [], flags: [] },
  watch: { values: ['debounce', 'poll'], flags: [] },
  publish: { values: ['message'], flags: ['yes'] },
  logs: { values: ['unit', 'level', 'limit', 'interval'], flags: ['follow'] },
  types: { values: [], flags: [] },
};
const ALIASES = { m: 'message', y: 'yes', f: 'follow', n: 'dry-run' };

export function parseArgs(argv, t) {
  const out = { cmd: null, args: [], opts: {} };
  const rest = [...argv];
  while (rest.length) {
    const a = rest.shift();
    if (a === '--help' || a === '-h') { out.opts.help = true; continue; }
    if (a === '--version' || a === '-v') { out.opts.version = true; continue; }
    if (a === '--') { out.args.push(...rest); break; }
    if (a.startsWith('-') && a !== '-') {
      let name = a.replace(/^--?/, ''), value;
      const eq = name.indexOf('=');
      if (eq >= 0) { value = name.slice(eq + 1); name = name.slice(0, eq); }
      name = ALIASES[name] || name;
      const spec = SPEC[out.cmd] || { values: [], flags: [] };
      if (spec.values.includes(name)) {
        if (value === undefined) {
          if (!rest.length) throw new CliError(t('err.missingValue', { opt: a }), 1);
          value = rest.shift();
        }
        out.opts[name] = value;
      } else if (spec.flags.includes(name) && value === undefined) {
        out.opts[name] = true;
      } else {
        throw new CliError(t('err.unknownOption', { opt: a }), 1);
      }
      continue;
    }
    if (!out.cmd) out.cmd = a;
    else out.args.push(a);
  }
  return out;
}

function makeCtx(io) {
  const env = io.env ?? process.env;
  const lang = detectLang(env);
  const stdout = io.stdout ?? process.stdout;
  const stderr = io.stderr ?? process.stderr;
  return {
    env,
    lang,
    t: translator(lang),
    cwd: path.resolve(io.cwd ?? process.cwd()),
    stdout,
    stderr,
    stdin: io.stdin ?? process.stdin,
    signal: io.signal,
    fetchImpl: io.fetch ?? globalThis.fetch,
    out: (s) => stdout.write(s + '\n'),
    err: (s) => stderr.write(s + '\n'),
  };
}

function attachProject(ctx) {
  const root = findProject(ctx.cwd);
  if (!root) throw new CliError(ctx.t('err.noProject', { cwd: ctx.cwd }), 1);
  ctx.project = readProject(root, ctx.t);
  ctx.ig = loadIgnore(ctx.project);
  const token = tokenFor(ctx.env, ctx.project.site);
  if (!token) throw new CliError(ctx.t('err.noToken', { site: ctx.project.site }), 1);
  ctx.client = createClient({ site: ctx.project.site, token, version: VERSION, fetchImpl: ctx.fetchImpl, t: ctx.t });
}

export async function main(argv, io = {}) {
  const ctx = makeCtx(io);
  try {
    const a = parseArgs(argv, ctx.t);
    if (a.opts.version) { ctx.out(VERSION); return EXIT_OK; }
    if (a.opts.help || !a.cmd || a.cmd === 'help') { ctx.out(HELP[ctx.lang]); return EXIT_OK; }
    const fn = COMMANDS[a.cmd];
    if (!fn) throw new CliError(ctx.t('err.unknownCommand', { cmd: a.cmd }), 1);
    const r = await fn(ctx, a.args, a.opts);
    return typeof r === 'number' ? r : r && typeof r.code === 'number' ? r.code : EXIT_OK;
  } catch (e) {
    if (e instanceof ApiError) e = apiFailure(e, ctx.t, ctx.project?.site ?? '');
    if (e instanceof CliError) {
      ctx.err(e.message);
      return e.exitCode;
    }
    if (e && e.name === 'AbortError') return EXIT_OK;
    ctx.err(ctx.t('err.generic', { msg: e && e.message ? e.message : String(e) }));
    return EXIT_ERROR;
  }
}

// ---------------------------------------------------------------- commands

function siteArg(ctx, cmd, arg, { allowSingleSaved = false } = {}) {
  let site = arg;
  if (!site) {
    const root = findProject(ctx.cwd);
    if (root) {
      try { site = JSON.parse(fs.readFileSync(path.join(root, CONFIG_FILE), 'utf8')).site; } catch { /* ignore */ }
    }
  }
  if (!site && allowSingleSaved) {
    const keys = Object.keys(readCredentials(ctx.env));
    if (keys.length === 1) site = keys[0];
  }
  if (!site) throw new CliError(ctx.t('err.needSite', { cmd }), 1);
  const norm = normalizeSite(site);
  if (!norm) throw new CliError(ctx.t('err.badSite', { site }), 1);
  return norm;
}

async function verifyAndSave(ctx, site, token) {
  const client = createClient({ site, token, version: VERSION, fetchImpl: ctx.fetchImpl, t: ctx.t });
  let me;
  try { me = await client.me(); } catch (e) { throw apiFailure(e, ctx.t, site); }
  const user = (me && (me.name || me.email)) || '?';
  const creds = readCredentials(ctx.env);
  creds[site] = { token, user, email: me && me.email, savedAt: new Date().toISOString() };
  const file = writeCredentials(ctx.env, creds);
  ctx.out(ctx.t('login.ok', { site, user: me && me.email && me.name ? `${me.name} <${me.email}>` : user }));
  ctx.out(ctx.t('login.saved', { file }));
  if (ctx.env.ANASAKI_TOKEN && ctx.env.ANASAKI_TOKEN !== token) ctx.err(ctx.t('login.envNote'));
}

async function cmdLogin(ctx, args, opts) {
  const site = siteArg(ctx, 'login', args[0]);
  let token = opts.token;
  if (!token) token = await ask(ctx, ctx.t('login.prompt', { site }), { hidden: true });
  token = (token || '').trim();
  if (!token) throw new CliError(ctx.t('err.emptyToken'), 1);
  if (!token.startsWith('ast_')) ctx.err(ctx.t('err.tokenFormat'));
  await verifyAndSave(ctx, site, token);
  return EXIT_OK;
}

async function cmdLogout(ctx, args) {
  const site = siteArg(ctx, 'logout', args[0], { allowSingleSaved: true });
  const creds = readCredentials(ctx.env);
  if (!creds[site]) { ctx.out(ctx.t('logout.none', { site })); return EXIT_OK; }
  delete creds[site];
  writeCredentials(ctx.env, creds);
  ctx.out(ctx.t('logout.ok', { site }));
  return EXIT_OK;
}

const DEFAULT_IGNORE = `# Files that are never synced with the site (gitignore-like globs).
# Built in: dotfiles, node_modules/, anasaki.json, anasaki.d.ts, tsconfig.json
package.json
package-lock.json
`;

function tsconfigFor(dirRel) {
  const inc = dirRel === '.' ? ['**/*.ts', '**/*.js', '**/*.mjs', '**/*.json'] : [`${dirRel}/**/*.ts`, `${dirRel}/**/*.js`, `${dirRel}/**/*.mjs`, `${dirRel}/**/*.json`];
  return {
    compilerOptions: {
      target: 'ES2022',
      module: 'ESNext',
      moduleResolution: 'Bundler',
      lib: ['ES2022'],
      types: [],
      allowJs: true,
      checkJs: true,
      strict: true,
      noEmit: true,
      resolveJsonModule: true,
      skipLibCheck: true,
      allowImportingTsExtensions: true,
    },
    include: inc,
    exclude: ['node_modules'],
  };
}

async function cmdInit(ctx, args, opts) {
  const { t } = ctx;
  const root = ctx.cwd;
  const cfgFile = path.join(root, CONFIG_FILE);
  let site;
  if (fs.existsSync(cfgFile)) {
    ctx.out(t('init.exists', { dir: root }));
    site = readProject(root, t).site;
  } else {
    site = siteArg(ctx, 'init', args[0], { allowSingleSaved: true });
    const dirRel = String(opts.dir || '.').replace(/\\/g, '/').replace(/\/+$/, '') || '.';
    fs.mkdirSync(path.resolve(root, ...dirRel.split('/')), { recursive: true });
    writeProject(root, site, dirRel);
    ctx.out(t('init.wrote', { file: CONFIG_FILE }));
  }
  if (opts.token) await verifyAndSave(ctx, site, opts.token);
  const project = readProject(root, t);
  const tsFile = path.join(root, 'tsconfig.json');
  if (fs.existsSync(tsFile)) ctx.out(t('init.kept', { file: 'tsconfig.json' }));
  else {
    fs.writeFileSync(tsFile, JSON.stringify(tsconfigFor(project.dirRel), null, 2) + '\n');
    ctx.out(t('init.wrote', { file: 'tsconfig.json' }));
  }
  const igFile = path.join(root, IGNORE_FILE);
  if (!fs.existsSync(igFile)) {
    fs.writeFileSync(igFile, DEFAULT_IGNORE);
    ctx.out(t('init.wrote', { file: IGNORE_FILE }));
  }
  attachProject(ctx);
  const r = await pull(ctx, {});
  if (r.code === EXIT_OK) ctx.out(t('init.done'));
  return r.code;
}

async function cmdPull(ctx, _args, opts) {
  attachProject(ctx);
  return (await pull(ctx, { force: !!opts.force })).code;
}

async function cmdPush(ctx, _args, opts) {
  attachProject(ctx);
  return (await push(ctx, { dryRun: !!opts['dry-run'], deleteRemote: !!opts.delete })).code;
}

async function cmdStatus(ctx) {
  attachProject(ctx);
  return (await status(ctx)).code;
}

async function cmdTypes(ctx) {
  attachProject(ctx);
  await refreshTypes(ctx);
  return EXIT_OK;
}

// Lines starting with "[anasaki]" are stable (English) markers for the VS Code
// background problem matcher.
export async function cmdWatch(ctx, _args, opts) {
  attachProject(ctx);
  const { t, project } = ctx;
  const ms = opts.debounce !== undefined ? Number(opts.debounce) : 300;
  const once = async () => {
    ctx.out('[anasaki] sync start');
    try {
      await push(ctx, { watch: true });
    } catch (e) {
      ctx.err(e instanceof CliError ? e.message : t('err.generic', { msg: e.message }));
    }
    ctx.out('[anasaki] sync done');
  };
  await once();
  const runner = createDebouncedRunner(once, ms);
  ctx.watchRunner = runner;
  const stop = startWatcher(project.dir, ctx.ig, () => runner.trigger(), {
    mode: opts.poll ? 'poll' : 'auto',
    pollMs: opts.poll && Number(opts.poll) > 0 ? Number(opts.poll) : 1000,
    onPollFallback: (p) => ctx.out(t('watch.poll', { ms: p })),
  });
  ctx.out(t('watch.start', { dir: project.dir }));
  ctx.out('[anasaki] watching');
  await new Promise((resolve) => {
    if (!ctx.signal) return; // runs until the process is interrupted
    if (ctx.signal.aborted) return resolve();
    ctx.signal.addEventListener('abort', resolve, { once: true });
  });
  stop();
  runner.cancel();
  await runner.idle();
  ctx.out(t('watch.stopped'));
  return EXIT_OK;
}

function describeTarget(d) {
  return [d.table, d.column].filter(Boolean).join('.');
}

async function cmdPublish(ctx, _args, opts) {
  attachProject(ctx);
  const { t, client } = ctx;
  const site = ctx.project.site;
  const s = await snapshot(ctx);
  const pending = s.entries.filter((e) => LOCAL_KINDS.includes(e.kind) || e.kind === 'conflict');
  if (pending.length) ctx.err(t('publish.localPending', { n: pending.length }));
  const st = await client.status().catch((e) => { throw apiFailure(e, t, site); });
  const live = st.liveVersion ?? null;
  if (live !== null && !st.draftChanged) {
    ctx.out(t('publish.noChanges', { live }));
    return EXIT_OK;
  }
  // dry run: build problems + pending migrations (needs edit_scripts; optional)
  let destructive = [];
  try {
    const b = await client.build();
    const probs = normalizeProblems({ errors: b.errors, warnings: b.warnings });
    if (b.ok === false || errorCount(probs)) {
      for (const p of probs) ctx.out(formatProblem(p, ctx.project.dirRel));
      ctx.err(t('publish.buildFailed'));
      return EXIT_CONFLICT;
    }
    for (const p of probs) ctx.out(formatProblem(p, ctx.project.dirRel));
    const mig = (b.migrations && b.migrations.pending) || [];
    if (mig.length) {
      ctx.out(t('publish.migrations', { n: mig.length }));
      for (const m of mig) {
        ctx.out(`  ${m.name}`);
        for (const d of m.destructive || []) destructive.push({ ...d, migration: d.migration || m.name });
      }
    }
  } catch (e) {
    if (!(e instanceof ApiError && e.status === 403)) throw apiFailure(e, t, site);
  }
  const confirm = async (list) => {
    ctx.out(t('publish.destructiveHead'));
    for (const d of list) ctx.out(t('publish.destructiveItem', { migration: d.migration, op: d.op, target: describeTarget(d) }));
    if (opts.yes) return true;
    if (!ctx.stdin.isTTY) throw new CliError(t('err.notTTY'), EXIT_CONFLICT);
    const a = await ask(ctx, t('publish.confirm'));
    return /^(y|yes|ใช่)$/i.test(a);
  };
  let confirmDestructive = false;
  if (destructive.length) {
    if (!(await confirm(destructive))) { ctx.err(t('publish.aborted')); return EXIT_ERROR; }
    confirmDestructive = true;
  }
  const body = { message: opts.message || '', baseVersion: live, confirmDestructive };
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const r = await client.publish(body);
      const v = (r && r.version) || {};
      ctx.out(t('publish.ok', { id: v.id ?? '?', msg: v.message ? ` — ${v.message}` : '' }));
      return EXIT_OK;
    } catch (e) {
      if (!(e instanceof ApiError)) throw e;
      const b = e.body || {};
      if (e.status === 409) {
        ctx.err(t('publish.conflict', { live: b.liveVersion ?? '?' }));
        return EXIT_CONFLICT;
      }
      if (e.status === 400 && Array.isArray(b.destructive) && b.destructive.length && !body.confirmDestructive) {
        if (!(await confirm(b.destructive))) { ctx.err(t('publish.aborted')); return EXIT_ERROR; }
        body.confirmDestructive = true;
        continue;
      }
      if (e.status === 400) {
        for (const p of normalizeProblems(b.errors)) ctx.out(formatProblem(p, ctx.project.dirRel));
        ctx.err(t('publish.failed', { msg: e.message }));
        return EXIT_CONFLICT;
      }
      throw apiFailure(e, t, site);
    }
  }
  return EXIT_ERROR;
}

function formatLog(e) {
  const at = String(e.at || '').replace('T', ' ').replace(/Z$/, '');
  const unit = e.unit && e.unit !== 'site' ? `${e.unit} ` : '';
  let s = `${at} [${e.level}] ${unit}${e.source}: ${e.message}`;
  if (e.trace) s += '\n' + e.trace;
  return s;
}

const sleep = (ms, signal) => new Promise((resolve) => {
  const tm = setTimeout(resolve, ms);
  signal?.addEventListener('abort', () => { clearTimeout(tm); resolve(); }, { once: true });
});

async function cmdLogs(ctx, _args, opts) {
  attachProject(ctx);
  const { t, client } = ctx;
  const per = Math.min(Math.max(Number(opts.limit) || 50, 1), 200);
  const q = { perPage: per, unit: opts.unit, level: opts.level };
  const get = async (extra) => {
    try { return await client.logs({ ...q, ...extra }, ctx.signal); } catch (e) {
      if (e && e.name === 'AbortError') return { items: [] };
      throw apiFailure(e, t, ctx.project.site);
    }
  };
  const first = await get({});
  const items = [...(first.items || [])].reverse(); // oldest first
  if (!items.length && !opts.follow) ctx.out(t('logs.none'));
  let lastId = 0, lastAt = '';
  for (const e of items) {
    ctx.out(formatLog(e));
    if (e.id > lastId) { lastId = e.id; lastAt = e.at; }
  }
  if (!opts.follow) return EXIT_OK;
  const interval = Number(opts.interval) > 0 ? Number(opts.interval) : 2000;
  while (!ctx.signal?.aborted) {
    await sleep(interval, ctx.signal);
    if (ctx.signal?.aborted) break;
    const r = await get(lastAt ? { from: lastAt, perPage: 200 } : { perPage: 200 });
    const fresh = (r.items || []).filter((e) => e.id > lastId).reverse();
    for (const e of fresh) {
      ctx.out(formatLog(e));
      lastId = e.id;
      lastAt = e.at;
    }
  }
  return EXIT_OK;
}

const COMMANDS = {
  login: cmdLogin,
  logout: cmdLogout,
  init: cmdInit,
  pull: cmdPull,
  push: cmdPush,
  status: cmdStatus,
  watch: cmdWatch,
  publish: cmdPublish,
  logs: cmdLogs,
  types: cmdTypes,
};

export { credentialsFile };
