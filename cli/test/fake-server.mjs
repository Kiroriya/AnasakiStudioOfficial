// A fake Anasaki API (node:http) implementing what the CLI talks to:
// auth/me, code/tree, code/files/{path}, code/sync (+409), code/status,
// code/build, code/publish, code/logs, code/types.
import crypto from 'node:crypto';
import fs from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { Writable } from 'node:stream';
import { main } from '../lib/cli.mjs';

export const TOKEN = 'ast_testtoken0000000000000000000000000000';

const sha = (s) => crypto.createHash('sha256').update(Buffer.from(s, 'utf8')).digest('hex');

export async function startFakeServer(initial = {}) {
  const st = {
    files: new Map(Object.entries(initial)), // path -> {content, updatedAt}
    liveVersion: null,
    liveFiles: null,
    problems: [],
    migrations: [],
    logs: [],
    types: 'declare const site: any;\n',
    requests: [],
    nextVersion: 1,
  };
  for (const [p, c] of st.files) st.files.set(p, { content: c, updatedAt: new Date().toISOString() });

  const treeItems = () => [...st.files].sort(([a], [b]) => (a < b ? -1 : 1))
    .map(([p, f]) => ({ path: p, sha256: sha(f.content), size: Buffer.byteLength(f.content), updatedAt: f.updatedAt }));
  const draftChanged = () => {
    if (!st.liveFiles) return st.files.size > 0;
    if (st.liveFiles.size !== st.files.size) return true;
    for (const [p, f] of st.files) if (st.liveFiles.get(p) !== f.content) return true;
    return false;
  };

  const server = http.createServer(async (req, res) => {
    const chunks = [];
    for await (const c of req) chunks.push(c);
    const raw = Buffer.concat(chunks).toString('utf8');
    const body = raw ? JSON.parse(raw) : undefined;
    const url = new URL(req.url, 'http://x');
    st.requests.push({ method: req.method, path: url.pathname, query: Object.fromEntries(url.searchParams), body });
    const send = (code, v, type = 'application/json') => {
      res.writeHead(code, { 'Content-Type': type });
      res.end(type === 'application/json' ? JSON.stringify(v) : v);
    };
    if (req.headers.authorization !== `Bearer ${TOKEN}`) return send(401, { error: 'invalid token' });
    const p = url.pathname;
    const key = `${req.method} ${p}`;
    if (key === 'GET /api/v1/auth/me') return send(200, { id: 1, name: 'Admin', email: 'admin@example.com', caps: ['view_scripts'] });
    if (key === 'GET /api/v1/code/tree') return send(200, { files: treeItems(), liveVersion: st.liveVersion, draftChanged: draftChanged() });
    if (key === 'GET /api/v1/code/status') return send(200, { liveVersion: st.liveVersion, draftChanged: draftChanged(), db: { pendingMigrations: st.migrations.length } });
    if (req.method === 'GET' && p.startsWith('/api/v1/code/files/')) {
      const fp = p.slice('/api/v1/code/files/'.length).split('/').map(decodeURIComponent).join('/');
      const f = st.files.get(fp);
      if (!f) return send(404, { error: 'not found' });
      return send(200, { path: fp, content: f.content, updatedAt: f.updatedAt });
    }
    if (key === 'POST /api/v1/code/sync') {
      const conflicts = [];
      for (const it of [...(body.put || []), ...(body.delete || [])]) {
        if (it.baseSha256 === undefined) continue;
        const cur = st.files.get(it.path);
        const curSha = cur ? sha(cur.content) : '';
        if (curSha !== it.baseSha256) conflicts.push({ path: it.path, serverSha256: curSha });
      }
      if (conflicts.length) return send(409, { error: 'conflict', conflicts });
      for (const it of body.put || []) {
        if (!/\.(ts|js|mjs|json|md)$/.test(it.path)) return send(400, { error: `bad file type: ${it.path}` });
      }
      if (!body.dryRun) {
        for (const it of body.put || []) st.files.set(it.path, { content: it.content, updatedAt: new Date().toISOString() });
        for (const it of body.delete || []) st.files.delete(it.path);
      }
      return send(200, { files: treeItems(), ok: st.problems.length === 0, problems: st.problems, warnings: st.warnings || [] });
    }
    if (key === 'POST /api/v1/code/build') {
      return send(200, { ok: st.problems.length === 0, errors: st.problems, warnings: [], registered: {}, migrations: { pending: st.migrations } });
    }
    if (key === 'POST /api/v1/code/publish') {
      if ((body.baseVersion ?? null) !== st.liveVersion) return send(409, { error: 'conflict', liveVersion: st.liveVersion });
      if (st.problems.length) return send(400, { error: 'build failed', errors: st.problems });
      const destructive = st.migrations.flatMap((m) => m.destructive || []);
      if (destructive.length && !body.confirmDestructive) return send(400, { error: 'destructive', destructive });
      const id = st.nextVersion++;
      st.liveVersion = id;
      st.liveFiles = new Map([...st.files].map(([k, f]) => [k, f.content]));
      st.migrations = [];
      return send(200, { version: { id, message: body.message, live: true, files: st.files.size } });
    }
    if (key === 'GET /api/v1/code/logs') {
      let items = st.logs.filter((l) => !url.searchParams.get('unit') || l.unit === url.searchParams.get('unit'));
      const from = url.searchParams.get('from');
      if (from) items = items.filter((l) => l.at >= from);
      items = [...items].sort((a, b) => b.id - a.id).slice(0, Number(url.searchParams.get('perPage') || 50));
      return send(200, { items, total: items.length });
    }
    if (key === 'GET /api/v1/code/types') return send(200, st.types, 'text/plain; charset=utf-8');
    return send(404, { error: 'no route ' + key });
  });
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  const site = `http://127.0.0.1:${server.address().port}`;
  return {
    st,
    site,
    sha,
    setFile(p, content) { st.files.set(p, { content, updatedAt: new Date().toISOString() }); },
    syncCalls() { return st.requests.filter((r) => r.path === '/api/v1/code/sync'); },
    close: () => new Promise((r) => { server.closeAllConnections?.(); server.close(r); }),
  };
}

export function tmpDir(prefix = 'anasaki-cli-') {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

export function makeProject(site, { dir = '.', files = {} } = {}) {
  const root = tmpDir();
  fs.writeFileSync(path.join(root, 'anasaki.json'), JSON.stringify({ site, dir }));
  for (const [p, c] of Object.entries(files)) writeFile(root, p, c);
  return root;
}

export function writeFile(root, rel, content) {
  const abs = path.join(root, ...rel.split('/'));
  fs.mkdirSync(path.dirname(abs), { recursive: true });
  fs.writeFileSync(abs, content);
}

export function readFile(root, rel) {
  return fs.readFileSync(path.join(root, ...rel.split('/')), 'utf8');
}

export function exists(root, rel) {
  return fs.existsSync(path.join(root, ...rel.split('/')));
}

class Capture extends Writable {
  constructor() { super(); this.text = ''; }
  _write(c, _e, cb) { this.text += c.toString(); cb(); }
}

/** run executes the CLI in-process and captures its output. */
export async function run(args, { cwd, env = {}, stdin, signal, lang = 'en' } = {}) {
  const stdout = new Capture(), stderr = new Capture();
  const configDir = env.ANASAKI_CONFIG_DIR || path.join(cwd || os.tmpdir(), '.anasaki-home');
  const fullEnv = { ANASAKI_TOKEN: TOKEN, ANASAKI_LANG: lang, ANASAKI_CONFIG_DIR: configDir, ...env };
  for (const k of Object.keys(fullEnv)) if (fullEnv[k] === undefined) delete fullEnv[k];
  const code = await main(args, { cwd, env: fullEnv, stdout, stderr, stdin, signal });
  return { code, out: stdout.text, err: stderr.text, all: stdout.text + stderr.text };
}
