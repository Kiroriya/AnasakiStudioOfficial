// pull / push / status: three-way comparison of local files, the server
// draft (GET /code/tree) and the last sync (.anasaki/state.json).
import fs from 'node:fs';
import path from 'node:path';
import { ApiError, apiFailure } from './api.mjs';
import { CliError, EXIT_CONFLICT, EXIT_OK } from './errors.mjs';
import { remoteIncluded, removeLocal, scanLocal, sha256, writeLocal } from './files.mjs';
import { errorCount, formatProblem, normalizeProblems } from './output.mjs';
import { loadState, saveState } from './state.mjs';

export const LOCAL_KINDS = ['modified', 'added', 'deleted'];
export const REMOTE_KINDS = ['remote-modified', 'remote-added', 'remote-deleted'];

/**
 * classify compares local (L), remote (R) and last-sync (B) hashes per path.
 * Kinds: same, modified/added/deleted (local change), remote-modified/
 * remote-added/remote-deleted (server change), conflict (both changed).
 */
export function classify(local, remote, base) {
  const paths = new Set([...Object.keys(local), ...Object.keys(remote), ...Object.keys(base)]);
  const out = [];
  for (const p of [...paths].sort()) {
    const L = local[p], R = remote[p], B = base[p];
    let kind;
    if (L === R) kind = L === undefined ? null : 'same';
    else if (L === B) kind = R === undefined ? 'remote-deleted' : B === undefined ? 'remote-added' : 'remote-modified';
    else if (R === B) kind = L === undefined ? 'deleted' : B === undefined ? 'added' : 'modified';
    else kind = 'conflict';
    if (kind) out.push({ path: p, kind, local: L, remote: R, base: B });
  }
  return out;
}

function hashes(map) {
  const o = {};
  for (const [p, f] of map) o[p] = f.sha256;
  return o;
}

async function call(ctx, fn) {
  try {
    return await fn();
  } catch (e) {
    throw apiFailure(e, ctx.t, ctx.project.site);
  }
}

/** snapshot gathers everything needed for a three-way comparison. */
export async function snapshot(ctx) {
  const { project, ig } = ctx;
  const tree = await call(ctx, () => ctx.client.tree());
  const remote = {};
  for (const f of tree.files || []) if (remoteIncluded(ig, f.path)) remote[f.path] = f.sha256;
  const { files, skipped } = scanLocal(project.dir, ig);
  const state = loadState(project.root, () => ctx.err(ctx.t('state.corrupt')));
  const entries = classify(hashes(files), remote, state.files);
  return { tree, remote, files, skipped, state, entries };
}

function printProblems(ctx, problems) {
  for (const p of problems) ctx.out(formatProblem(p, ctx.project.dirRel));
}

function printConflictList(ctx, list) {
  for (const c of list) ctx.err(`  ! ${c.path}`);
  ctx.err(ctx.t('push.conflictNext'));
}

// ---------------------------------------------------------------- status

export async function status(ctx) {
  const { t } = ctx;
  const s = await snapshot(ctx);
  const live = s.tree.liveVersion ? '#' + s.tree.liveVersion : t('status.noLive');
  ctx.out(t('status.server', { site: ctx.project.site, live, draft: s.tree.draftChanged ? t('status.draftChanged') : '' }));
  const group = (kinds) => s.entries.filter((e) => kinds.includes(e.kind));
  const local = group(LOCAL_KINDS), remote = group(REMOTE_KINDS), conflicts = group(['conflict']);
  const line = (e) => `  ${t('status.kind.' + e.kind).padEnd(16)} ${e.path}`;
  if (local.length) { ctx.out(t('status.local')); local.forEach((e) => ctx.out(line(e))); }
  if (remote.length) { ctx.out(t('status.remote')); remote.forEach((e) => ctx.out(line(e))); }
  if (conflicts.length) { ctx.out(t('status.conflicts')); conflicts.forEach((e) => ctx.out(line(e))); }
  if (s.skipped.length) {
    ctx.out(t('status.skipped'));
    for (const k of s.skipped) ctx.out(`  ${k.path} (${t('status.skip.' + k.reason)})`);
  }
  if (!local.length && !remote.length && !conflicts.length) {
    ctx.out(t('status.clean', { n: s.entries.filter((e) => e.kind === 'same').length }));
  }
  return { code: EXIT_OK, entries: s.entries, skipped: s.skipped };
}

// ---------------------------------------------------------------- pull

async function pool(items, n, fn) {
  let i = 0;
  const workers = Array.from({ length: Math.min(n, items.length) }, async () => {
    while (i < items.length) {
      const item = items[i++];
      await fn(item);
    }
  });
  await Promise.all(workers);
}

export async function pull(ctx, { force = false } = {}) {
  const { t, project } = ctx;
  const s = await snapshot(ctx);
  const state = s.state;
  state.site = project.site;
  state.liveVersion = s.tree.liveVersion ?? null;
  const download = [], remove = [], conflicts = [], kept = [];
  for (const e of s.entries) {
    switch (e.kind) {
      case 'same':
        state.files[e.path] = e.remote;
        break;
      case 'remote-modified':
      case 'remote-added':
        download.push(e);
        break;
      case 'remote-deleted':
        remove.push(e);
        break;
      case 'conflict':
        if (!force) { conflicts.push(e); break; }
        if (e.remote === undefined) remove.push(e);
        else download.push(e);
        break;
      default: // local changes stay as they are
        kept.push(e);
    }
  }
  await pool(download, 6, async (e) => {
    const f = await call(ctx, () => ctx.client.file(e.path));
    const content = typeof f === 'string' ? f : f.content ?? '';
    writeLocal(project.dir, e.path, content);
    state.files[e.path] = sha256(content);
    ctx.out(t('pull.wrote', { path: e.path }));
  });
  for (const e of remove) {
    removeLocal(project.dir, e.path);
    delete state.files[e.path];
    ctx.out(t('pull.removed', { path: e.path }));
  }
  for (const e of kept) ctx.out(t('pull.kept', { path: e.path }));
  saveState(project.root, state);
  await refreshTypes(ctx, { quiet: true });
  if (conflicts.length) {
    for (const e of conflicts) ctx.err(t('pull.conflict', { path: e.path }));
    ctx.err(t('pull.refused', { n: conflicts.length }));
    return { code: EXIT_CONFLICT, conflicts };
  }
  if (!download.length && !remove.length) ctx.out(t('pull.upToDate'));
  else ctx.out(t('pull.done', { w: download.length, d: remove.length, k: kept.length }));
  return { code: EXIT_OK };
}

export async function refreshTypes(ctx, { quiet = false } = {}) {
  const file = path.join(ctx.project.dir, 'anasaki.d.ts');
  try {
    const text = await ctx.client.types();
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, text);
    if (!quiet) ctx.out(ctx.t('types.wrote', { file: path.relative(ctx.cwd, file) || file }));
    return true;
  } catch (e) {
    if (!quiet) throw apiFailure(e, ctx.t, ctx.project.site);
    ctx.err(ctx.t('types.failed', { msg: e.message }));
    return false;
  }
}

// ---------------------------------------------------------------- push

/**
 * push uploads local changes with their base hashes. Remote files are
 * deleted only with deleteRemote (never in watch mode).
 */
export async function push(ctx, { dryRun = false, deleteRemote = false, watch = false } = {}) {
  const { t, project } = ctx;
  const s = await snapshot(ctx);
  const state = s.state;
  state.site = project.site;
  state.liveVersion = s.tree.liveVersion ?? state.liveVersion;
  for (const e of s.entries) if (e.kind === 'same') state.files[e.path] = e.remote;

  const conflicts = s.entries.filter((e) => e.kind === 'conflict');
  if (conflicts.length) {
    ctx.err(t('push.conflictLocal'));
    printConflictList(ctx, conflicts);
    if (!dryRun) saveState(project.root, state);
    return { code: EXIT_CONFLICT, conflicts };
  }

  const put = [], del = [], keptRemote = [];
  for (const e of s.entries) {
    if (e.kind === 'modified') put.push({ path: e.path, content: s.files.get(e.path).content, baseSha256: e.base });
    else if (e.kind === 'added') put.push({ path: e.path, content: s.files.get(e.path).content, baseSha256: '' }); // "" = must not exist yet
    else if (e.kind === 'deleted') {
      if (deleteRemote && !watch) del.push({ path: e.path, baseSha256: e.base });
      else keptRemote.push(e);
    }
  }
  const remoteNews = s.entries.filter((e) => REMOTE_KINDS.includes(e.kind));

  for (const e of keptRemote) {
    if (watch) {
      ctx.notified ??= new Set();
      if (!ctx.notified.has(e.path)) { ctx.notified.add(e.path); ctx.out(t('watch.deletedLocal', { path: e.path })); }
    } else ctx.out(t('push.keptRemote', { path: e.path }));
  }
  if (remoteNews.length) ctx.out(t('push.remoteNews', { n: remoteNews.length }));

  if (!put.length && !del.length) {
    if (!dryRun) saveState(project.root, state);
    if (!watch) ctx.out(t('push.nothing'));
    return { code: EXIT_OK, put, del, problems: [], noop: true };
  }

  let res;
  try {
    res = await ctx.client.sync({ put, delete: del, ...(dryRun ? { dryRun: true } : {}) });
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      const list = (e.body && Array.isArray(e.body.conflicts) ? e.body.conflicts : []).map((c) => ({ path: c.path }));
      ctx.err(t('push.conflict409'));
      printConflictList(ctx, list);
      return { code: EXIT_CONFLICT, conflicts: list };
    }
    if (e instanceof ApiError && (e.status === 400 || e.status === 413 || e.status === 422)) {
      ctx.err(t('push.rejected', { msg: e.message }));
      printProblems(ctx, normalizeProblems(e.body && (e.body.problems || e.body.errors)));
      return { code: EXIT_CONFLICT };
    }
    throw apiFailure(e, t, project.site);
  }

  for (const p of put) ctx.out(t(dryRun ? 'push.wouldPut' : 'push.put', { path: p.path }));
  for (const d of del) ctx.out(t(dryRun ? 'push.wouldDel' : 'push.del', { path: d.path }));

  const problems = normalizeProblems({ errors: (res && res.problems) || [], warnings: (res && res.warnings) || [] });
  if (!dryRun) {
    const server = {};
    for (const f of (res && res.files) || []) server[f.path] = f.sha256;
    const haveServer = res && Array.isArray(res.files);
    for (const p of put) state.files[p.path] = haveServer && server[p.path] ? server[p.path] : s.files.get(p.path).sha256;
    for (const d of del) delete state.files[d.path];
    saveState(project.root, state);
  }
  printProblems(ctx, problems);
  const errs = errorCount(problems);
  ctx.out(t(dryRun ? 'push.dryDone' : 'push.done', { p: put.length, d: del.length }));
  if (res && Array.isArray(res.problems)) ctx.out(errs ? t('push.buildErrors', { n: errs }) : t('push.buildOk'));
  return { code: errs ? EXIT_CONFLICT : EXIT_OK, put, del, problems };
}

export { CliError };
