import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { after, before, test } from 'node:test';
import { classify } from '../lib/sync.mjs';
import { exists, makeProject, readFile, run, startFakeServer, writeFile } from './fake-server.mjs';

let srv;
before(async () => { srv = await startFakeServer(); });
after(async () => { await srv.close(); });

function reset(files) {
  srv.st.files.clear();
  for (const [p, c] of Object.entries(files)) srv.setFile(p, c);
  srv.st.problems = [];
  srv.st.migrations = [];
  srv.st.liveVersion = null;
  srv.st.liveFiles = null;
  srv.st.requests.length = 0;
}

const state = (root) => JSON.parse(fs.readFileSync(path.join(root, '.anasaki', 'state.json'), 'utf8'));

test('classify: every kind of three-way difference', () => {
  const L = { same: 'a', mod: 'b2', add: 'n', rmod: 'r1', conf: 'x1', bothAdd: 'p', rdelConf: 'q2' };
  const R = { same: 'a', mod: 'b1', rmod: 'r2', radd: 'z', del: 'd', conf: 'x2', bothAdd: 'p2', bothSame: undefined };
  const B = { same: 'a', mod: 'b1', rmod: 'r1', del: 'd', conf: 'x0', rdel: 'k', rdelConf: 'q1' };
  L.rdel = 'k';
  const kinds = Object.fromEntries(classify(L, R, B).map((e) => [e.path, e.kind]));
  assert.deepEqual(kinds, {
    add: 'added', bothAdd: 'conflict', conf: 'conflict', del: 'deleted', mod: 'modified', radd: 'remote-added',
    rdel: 'remote-deleted', rdelConf: 'conflict', rmod: 'remote-modified', same: 'same',
  });
});

test('pull downloads the tree + anasaki.d.ts and records state', async () => {
  reset({ 'main.ts': 'export {}\n', 'lib/a.ts': 'A', 'README.md': '# hi' });
  const root = makeProject(srv.site);
  const r = await run(['pull'], { cwd: root });
  assert.equal(r.code, 0, r.all);
  assert.equal(readFile(root, 'lib/a.ts'), 'A');
  assert.equal(readFile(root, 'anasaki.d.ts'), srv.st.types);
  const s = state(root);
  assert.equal(s.files['lib/a.ts'], srv.sha('A'));
  assert.equal(Object.keys(s.files).length, 3);
  // second pull: nothing to do
  const r2 = await run(['pull'], { cwd: root });
  assert.match(r2.out, /Already up to date/);
});

test('pull never overwrites a locally modified file without --force', async () => {
  reset({ 'main.ts': 'v1', 'other.ts': 'o1' });
  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  writeFile(root, 'main.ts', 'local edit');
  srv.setFile('main.ts', 'server edit');
  srv.setFile('other.ts', 'o2');
  const r = await run(['pull'], { cwd: root });
  assert.equal(r.code, 2);
  assert.match(r.err, /main\.ts — changed locally and on the server/);
  assert.match(r.err, /--force/);
  assert.equal(readFile(root, 'main.ts'), 'local edit');
  assert.equal(readFile(root, 'other.ts'), 'o2'); // non-conflicting file still updated
  // local-only change is kept, not reverted
  writeFile(root, 'other.ts', 'o-local');
  const r2 = await run(['pull'], { cwd: root });
  assert.equal(readFile(root, 'other.ts'), 'o-local');
  assert.match(r2.out, /other\.ts \(local change kept\)/);
  // --force takes the server copy
  const r3 = await run(['pull', '--force'], { cwd: root });
  assert.equal(r3.code, 0, r3.all);
  assert.equal(readFile(root, 'main.ts'), 'server edit');
  assert.equal(readFile(root, 'other.ts'), 'o-local'); // local-only change is still not a conflict
});

test('pull removes files deleted on the server unless changed locally', async () => {
  reset({ 'a.ts': 'a', 'lib/b.ts': 'b' });
  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  srv.st.files.delete('lib/b.ts');
  const r = await run(['pull'], { cwd: root });
  assert.equal(r.code, 0);
  assert.equal(exists(root, 'lib/b.ts'), false);
  assert.equal(exists(root, 'lib'), false); // empty folder pruned
  assert.equal(state(root).files['lib/b.ts'], undefined);
});

test('status classifies local, remote, conflict and skipped files', async () => {
  reset({ 'same.ts': 's', 'mod.ts': 'm', 'del.ts': 'd', 'rmod.ts': 'r', 'conf.ts': 'c' });
  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  writeFile(root, 'mod.ts', 'm2');
  writeFile(root, 'new.ts', 'n');
  fs.unlinkSync(path.join(root, 'del.ts'));
  writeFile(root, 'conf.ts', 'c-local');
  writeFile(root, 'image.png', 'x');
  srv.setFile('rmod.ts', 'r2');
  srv.setFile('radd.ts', 'ra');
  srv.setFile('conf.ts', 'c-server');
  const r = await run(['status'], { cwd: root });
  assert.equal(r.code, 0, r.all);
  const line = (kind, p) => new RegExp(`^\\s+${kind}\\s+${p.replace('.', '\\.')}$`, 'm');
  assert.match(r.out, line('modified', 'mod.ts'));
  assert.match(r.out, line('added', 'new.ts'));
  assert.match(r.out, line('deleted', 'del.ts'));
  assert.match(r.out, line('remote modified', 'rmod.ts'));
  assert.match(r.out, line('remote added', 'radd.ts'));
  assert.match(r.out, line('conflict', 'conf.ts'));
  assert.match(r.out, /image\.png \(unsupported file type\)/);
  assert.doesNotMatch(r.out, /same\.ts/);
});

test('push uploads changes with base hashes, dry-run changes nothing, delete only with --delete', async () => {
  reset({ 'main.ts': 'v1', 'gone.ts': 'g' });
  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  writeFile(root, 'main.ts', 'v2');
  writeFile(root, 'lib/new.ts', 'n');
  fs.unlinkSync(path.join(root, 'gone.ts'));

  const dry = await run(['push', '--dry-run'], { cwd: root });
  assert.equal(dry.code, 0, dry.all);
  assert.match(dry.out, /would upload main\.ts/);
  assert.match(dry.out, /would upload lib\/new\.ts/);
  assert.match(dry.out, /gone\.ts deleted locally; still on the server/);
  const dryCall = srv.syncCalls().at(-1).body;
  assert.equal(dryCall.dryRun, true);
  assert.equal(srv.st.files.get('main.ts').content, 'v1');
  assert.equal(state(root).files['main.ts'], srv.sha('v1'));

  const r = await run(['push'], { cwd: root });
  assert.equal(r.code, 0, r.all);
  const call = srv.syncCalls().at(-1).body;
  assert.deepEqual(call.put.find((p) => p.path === 'main.ts'), { path: 'main.ts', content: 'v2', baseSha256: srv.sha('v1') });
  assert.equal(call.put.find((p) => p.path === 'lib/new.ts').baseSha256, '');
  assert.deepEqual(call.delete, []);
  assert.equal(srv.st.files.get('main.ts').content, 'v2');
  assert.ok(srv.st.files.has('gone.ts'), 'push without --delete never deletes remote files');
  assert.equal(state(root).files['main.ts'], srv.sha('v2'));
  assert.match(r.out, /Build check: OK/);

  const d = await run(['push', '--delete'], { cwd: root });
  assert.equal(d.code, 0, d.all);
  assert.deepEqual(srv.syncCalls().at(-1).body.delete, [{ path: 'gone.ts', baseSha256: srv.sha('g') }]);
  assert.equal(srv.st.files.has('gone.ts'), false);
  assert.equal(state(root).files['gone.ts'], undefined);

  const again = await run(['push'], { cwd: root });
  assert.match(again.out, /Nothing to push/);
});

test('push --delete never deletes server files the CLI has never seen', async () => {
  reset({ 'a.ts': 'a' });
  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  srv.setFile('theirs.ts', 'new on server');
  writeFile(root, 'a.ts', 'a2');
  const r = await run(['push', '--delete'], { cwd: root });
  assert.equal(r.code, 0, r.all);
  assert.ok(srv.st.files.has('theirs.ts'));
  assert.match(r.out, /run "anasaki-code pull"/);
});

test('push refuses when a file changed on both sides (detected before upload)', async () => {
  reset({ 'main.ts': 'v1' });
  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  writeFile(root, 'main.ts', 'mine');
  srv.setFile('main.ts', 'theirs');
  const before = srv.syncCalls().length;
  const r = await run(['push'], { cwd: root });
  assert.equal(r.code, 2);
  assert.equal(srv.syncCalls().length, before, 'nothing sent');
  assert.match(r.err, /Conflict/);
  assert.match(r.err, /! main\.ts/);
  assert.match(r.err, /anasaki-code pull/);
  assert.equal(srv.st.files.get('main.ts').content, 'theirs');
});

test('push explains a 409 from the server (race after the tree was read)', async () => {
  reset({ 'main.ts': 'v1' });
  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  writeFile(root, 'main.ts', 'mine');
  // the server copy changes right after GET /tree, before POST /sync
  const realFetch = globalThis.fetch;
  const fetchImpl = async (url, init) => {
    const res = await realFetch(url, init);
    if (String(url).endsWith('/code/tree')) srv.setFile('main.ts', 'raced');
    return res;
  };
  const { main } = await import('../lib/cli.mjs');
  const out = []; const err = [];
  const w = (arr) => ({ write: (s) => { arr.push(String(s)); return true; } });
  const code = await main(['push'], {
    cwd: root, env: { ANASAKI_TOKEN: (await import('./fake-server.mjs')).TOKEN, ANASAKI_LANG: 'en', ANASAKI_CONFIG_DIR: path.join(root, '.home') },
    stdout: w(out), stderr: w(err), fetch: fetchImpl,
  });
  assert.equal(code, 2);
  assert.match(err.join(''), /server copy changed since your last sync/);
  assert.match(err.join(''), /! main\.ts/);
  assert.match(err.join(''), /Next steps/);
  assert.equal(srv.st.files.get('main.ts').content, 'raced');
});

test('push prints build problems as file:line:col (VS Code friendly) and exits 2', async () => {
  reset({});
  const root = makeProject(srv.site, { dir: 'src' });
  writeFile(root, 'src/main.ts', 'oops(');
  srv.st.problems = [{ file: 'main.ts', line: 1, column: 6, message: 'Unexpected end of file' }, { message: 'no file problem' }];
  const r = await run(['push'], { cwd: root });
  assert.equal(r.code, 2, r.all);
  assert.match(r.out, /^src\/main\.ts:1:6: error: Unexpected end of file$/m);
  assert.match(r.out, /^error: no file problem$/m);
  assert.match(r.out, /2 error\(s\)/);
  assert.equal(srv.st.files.get('main.ts').content, 'oops(');
});

test('server validation errors (400) exit 2', async () => {
  reset({});
  const root = makeProject(srv.site);
  writeFile(root, 'ok.ts', '1');
  const realFetch = globalThis.fetch;
  const { main } = await import('../lib/cli.mjs');
  const { TOKEN } = await import('./fake-server.mjs');
  const fetchImpl = async (url, init) => {
    if (String(url).endsWith('/code/sync')) return new Response(JSON.stringify({ error: 'ไฟล์มากเกินไป' }), { status: 400 });
    return realFetch(url, init);
  };
  let err = '';
  const code = await main(['push'], { cwd: root, env: { ANASAKI_TOKEN: TOKEN, ANASAKI_CONFIG_DIR: path.join(root, '.h') },
    stdout: { write: () => true }, stderr: { write: (s) => { err += s; return true; } }, fetch: fetchImpl });
  assert.equal(code, 2);
  assert.match(err, /rejected the upload: ไฟล์มากเกินไป/);
});

test('sync folder "dir" and .anasakiignore are honoured', async () => {
  reset({ 'main.ts': 'm', 'secret/notes.md': 'server only, ignored' });
  const root = makeProject(srv.site, { dir: 'code' });
  writeFile(root, '.anasakiignore', 'secret/\n*.local.ts\n');
  writeFile(root, 'outside.ts', 'not in dir');
  const r = await run(['pull'], { cwd: path.join(root) });
  assert.equal(r.code, 0, r.all);
  assert.equal(readFile(root, 'code/main.ts'), 'm');
  assert.equal(exists(root, 'code/secret/notes.md'), false);
  assert.ok(exists(root, 'code/anasaki.d.ts'));
  writeFile(root, 'code/x.local.ts', 'ignored');
  writeFile(root, 'code/lib/y.ts', 'y');
  // run from a sub folder: anasaki.json is found by walking up
  const p = await run(['push'], { cwd: path.join(root, 'code', 'lib') });
  assert.equal(p.code, 0, p.all);
  const put = srv.syncCalls().at(-1).body.put.map((x) => x.path);
  assert.deepEqual(put, ['lib/y.ts']);
  assert.ok(srv.st.files.has('secret/notes.md'), 'ignored remote file untouched');
});

test('a new file that appeared on the server meanwhile is a conflict (baseSha256 "")', async () => {
  reset({});
  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  writeFile(root, 'fresh.ts', 'mine');
  const realFetch = globalThis.fetch;
  const fetchImpl = async (url, init) => {
    const res = await realFetch(url, init);
    if (String(url).endsWith('/code/tree')) srv.setFile('fresh.ts', 'theirs');
    return res;
  };
  const { main } = await import('../lib/cli.mjs');
  const { TOKEN } = await import('./fake-server.mjs');
  let err = '';
  const code = await main(['push'], { cwd: root, env: { ANASAKI_TOKEN: TOKEN, ANASAKI_CONFIG_DIR: path.join(root, '.h') },
    stdout: { write: () => true }, stderr: { write: (s) => { err += s; return true; } }, fetch: fetchImpl });
  assert.equal(code, 2);
  assert.match(err, /! fresh\.ts/);
  assert.equal(srv.st.files.get('fresh.ts').content, 'theirs');
});

test('build warnings from sync are printed as warnings and do not fail the push', async () => {
  reset({});
  const root = makeProject(srv.site);
  writeFile(root, 'w.ts', 'x');
  srv.st.warnings = [{ file: 'w.ts', line: 4, column: 2, message: 'unused' }];
  const r = await run(['push'], { cwd: root });
  srv.st.warnings = [];
  assert.equal(r.code, 0, r.all);
  assert.match(r.out, /^w\.ts:4:2: warning: unused$/m);
});
