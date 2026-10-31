import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { Readable } from 'node:stream';
import { after, before, test } from 'node:test';
import { parseArgs, VERSION } from '../lib/cli.mjs';
import { normalizeSite } from '../lib/config.mjs';
import { detectLang, messages, translator } from '../lib/i18n.mjs';
import { formatProblem } from '../lib/output.mjs';
import { exists, makeProject, readFile, run, startFakeServer, tmpDir, TOKEN, writeFile } from './fake-server.mjs';

let srv;
before(async () => { srv = await startFakeServer({ 'main.ts': 'export {}\n' }); });
after(async () => { await srv.close(); });

const ttyIn = (text) => { const s = Readable.from([text]); s.isTTY = true; return s; };

test('--help, --version, unknown command / option', async () => {
  const h = await run(['--help'], { cwd: tmpDir() });
  assert.equal(h.code, 0);
  assert.match(h.out, /push \[--dry-run\] \[--delete\]/);
  const v = await run(['--version'], { cwd: tmpDir() });
  assert.equal(v.out.trim(), VERSION);
  const u = await run(['frobnicate'], { cwd: tmpDir() });
  assert.equal(u.code, 1);
  assert.match(u.err, /unknown command/);
  const o = await run(['push', '--nope'], { cwd: tmpDir() });
  assert.equal(o.code, 1);
  assert.match(o.err, /unknown option: --nope/);
});

test('parseArgs handles values, aliases and =', () => {
  const t = translator('en');
  assert.deepEqual(parseArgs(['publish', '-m', 'hi there', '-y'], t), { cmd: 'publish', args: [], opts: { message: 'hi there', yes: true } });
  assert.deepEqual(parseArgs(['logs', '--unit=shop', '-f'], t).opts, { unit: 'shop', follow: true });
  assert.throws(() => parseArgs(['publish', '-m'], t), /needs a value/);
});

test('site normalisation', () => {
  assert.equal(normalizeSite('example.com/'), 'https://example.com');
  assert.equal(normalizeSite('http://localhost:8080/'), 'http://localhost:8080');
  assert.equal(normalizeSite('localhost:8080'), 'http://localhost:8080');
  assert.equal(normalizeSite('ftp://x'), null);
});

test('login verifies with /auth/me and saves the token; logout removes it', async () => {
  const cwd = tmpDir();
  const home = tmpDir();
  const env = { ANASAKI_TOKEN: undefined, ANASAKI_CONFIG_DIR: home };
  const bad = await run(['login', srv.site, '--token', 'ast_wrong'], { cwd, env });
  assert.equal(bad.code, 1);
  assert.match(bad.err, /401/);
  assert.equal(fs.existsSync(path.join(home, 'credentials.json')), false);

  const r = await run(['login', srv.site], { cwd, env, stdin: Readable.from([TOKEN + '\n']) });
  assert.equal(r.code, 0, r.all);
  assert.match(r.out, /Logged in to .* as Admin <admin@example.com>/);
  const creds = JSON.parse(fs.readFileSync(path.join(home, 'credentials.json'), 'utf8'));
  assert.equal(creds[srv.site].token, TOKEN);
  if (process.platform !== 'win32') {
    assert.equal(fs.statSync(path.join(home, 'credentials.json')).mode & 0o777, 0o600);
  }
  // the saved token is used when ANASAKI_TOKEN is absent
  const root = makeProject(srv.site);
  const p = await run(['pull'], { cwd: root, env });
  assert.equal(p.code, 0, p.all);
  // ANASAKI_TOKEN overrides the saved token
  const o = await run(['pull'], { cwd: root, env: { ...env, ANASAKI_TOKEN: 'ast_bogus' } });
  assert.equal(o.code, 1);
  assert.match(o.err, /anasaki-code login/);

  const lo = await run(['logout'], { cwd: root, env });
  assert.equal(lo.code, 0);
  assert.match(lo.out, /Removed the saved token/);
  const none = await run(['pull'], { cwd: root, env });
  assert.equal(none.code, 1);
  assert.match(none.err, /not logged in/);
});

test('commands outside a project explain how to init', async () => {
  const r = await run(['status'], { cwd: tmpDir() });
  assert.equal(r.code, 1);
  assert.match(r.err, /no anasaki\.json/);
});

test('init writes anasaki.json, tsconfig.json, .anasakiignore and pulls', async () => {
  const cwd = tmpDir();
  const r = await run(['init', srv.site, '--dir', 'src'], { cwd });
  assert.equal(r.code, 0, r.all);
  assert.deepEqual(JSON.parse(readFile(cwd, 'anasaki.json')), { site: srv.site, dir: 'src' });
  const ts = JSON.parse(readFile(cwd, 'tsconfig.json'));
  assert.ok(ts.include.includes('src/**/*.ts'));
  assert.ok(exists(cwd, '.anasakiignore'));
  assert.equal(readFile(cwd, 'src/main.ts'), 'export {}\n');
  assert.ok(exists(cwd, 'src/anasaki.d.ts'));
  assert.ok(exists(cwd, '.anasaki/state.json'));
  // running init again keeps the config
  const again = await run(['init'], { cwd });
  assert.equal(again.code, 0, again.all);
  assert.match(again.out, /already exists/);
});

test('types refreshes anasaki.d.ts', async () => {
  const root = makeProject(srv.site);
  srv.st.types = 'declare const v2: 1;\n';
  const r = await run(['types'], { cwd: root });
  assert.equal(r.code, 0, r.all);
  assert.equal(readFile(root, 'anasaki.d.ts'), 'declare const v2: 1;\n');
});

test('publish shows migrations and confirms destructive changes', async () => {
  const root = makeProject(srv.site);
  srv.st.liveVersion = null;
  srv.st.liveFiles = null;
  srv.st.migrations = [
    { name: '001_init', sql: ['CREATE TABLE t'], destructive: [] },
    { name: '002_drop', sql: ['ALTER TABLE t DROP note'], destructive: [{ migration: '002_drop', op: 'drop_column', table: 't', column: 'note' }] },
  ];
  // not interactive and no --yes → refuse
  const nt = await run(['publish', '-m', 'x'], { cwd: root, stdin: Readable.from(['']) });
  assert.equal(nt.code, 2, nt.all);
  assert.match(nt.out, /Pending database migrations \(2\)/);
  assert.match(nt.out, /! 002_drop: drop_column t\.note/);
  assert.match(nt.err, /--yes/);
  assert.equal(srv.st.liveVersion, null);
  // interactive "no"
  const no = await run(['publish'], { cwd: root, stdin: ttyIn('no\n') });
  assert.equal(no.code, 1);
  assert.match(no.err, /cancelled/);
  // interactive "yes"
  const yes = await run(['publish', '-m', 'release'], { cwd: root, stdin: ttyIn('yes\n') });
  assert.equal(yes.code, 0, yes.all);
  assert.match(yes.out, /Published version #1 — release/);
  const pub = srv.st.requests.filter((q) => q.path === '/api/v1/code/publish').at(-1).body;
  assert.deepEqual(pub, { message: 'release', baseVersion: null, confirmDestructive: true });
  // nothing changed since
  const same = await run(['publish', '--yes'], { cwd: root });
  assert.equal(same.code, 0);
  assert.match(same.out, /nothing to publish/);
  // --yes skips the prompt; destructive list coming only from the publish answer
  srv.setFile('main.ts', 'changed');
  srv.st.migrations = [{ name: '003', destructive: [{ op: 'drop_table', table: 'old' }] }];
  const y = await run(['publish', '--yes'], { cwd: root });
  assert.equal(y.code, 0, y.all);
  assert.equal(srv.st.liveVersion, 2);
});

test('publish refuses when the draft has build errors', async () => {
  const root = makeProject(srv.site);
  srv.setFile('main.ts', 'broken(');
  srv.st.problems = [{ file: 'main.ts', line: 3, column: 2, message: 'Expected ")"' }];
  const r = await run(['publish', '--yes'], { cwd: root });
  srv.st.problems = [];
  assert.equal(r.code, 2);
  assert.match(r.out, /^main\.ts:3:2: error: Expected "\)"$/m);
  assert.match(r.err, /build errors/);
});

test('logs prints oldest first and --follow polls for new entries', async () => {
  const root = makeProject(srv.site);
  srv.st.logs = [
    { id: 1, at: '2026-10-30T01:00:00.000Z', level: 'info', source: 'GET /a', message: 'first', unit: 'site', trace: null },
    { id: 2, at: '2026-10-30T01:00:01.000Z', level: 'error', source: 'GET /b', message: 'second', unit: 'shop', trace: 'at x' },
  ];
  const r = await run(['logs'], { cwd: root });
  assert.equal(r.code, 0, r.all);
  const lines = r.out.trim().split('\n');
  assert.match(lines[0], /\[info\] GET \/a: first/);
  assert.match(lines[1], /\[error\] shop GET \/b: second/);
  assert.equal(lines[2], 'at x');

  const ac = new AbortController();
  const p = run(['logs', '--follow', '--unit', 'site', '--interval', '30'], { cwd: root, signal: ac.signal });
  await new Promise((res) => setTimeout(res, 80));
  srv.st.logs.push({ id: 3, at: '2026-10-30T01:00:02.000Z', level: 'warn', source: 'cron', message: 'third', unit: 'site' });
  await new Promise((res) => setTimeout(res, 150));
  ac.abort();
  const f = await p;
  assert.equal(f.code, 0, f.all);
  assert.equal((f.out.match(/third/g) || []).length, 1);
  assert.doesNotMatch(f.out, /second/);
  const q = srv.st.requests.filter((x) => x.path === '/api/v1/code/logs').at(-1).query;
  assert.equal(q.unit, 'site');
});

test('Thai messages when ANASAKI_LANG / LANG start with th', async () => {
  assert.equal(detectLang({ LANG: 'th_TH.UTF-8' }), 'th');
  assert.equal(detectLang({ ANASAKI_LANG: 'th' }), 'th');
  assert.equal(detectLang({ LANG: 'en_US.UTF-8' }), 'en');
  assert.equal(detectLang({}), 'en');
  // every English key has a Thai translation
  assert.deepEqual(Object.keys(messages.th).sort(), Object.keys(messages.en).sort());

  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  writeFile(root, 'new.ts', 'x');
  const r = await run(['status'], { cwd: root, lang: undefined, env: { ANASAKI_LANG: undefined, LANG: 'th_TH.UTF-8' } });
  assert.equal(r.code, 0, r.all);
  assert.match(r.out, /เพิ่มใหม่\s+new\.ts/);
  const h = await run(['--help'], { cwd: root, lang: 'th' });
  assert.match(h.out, /วิธีใช้/);
  const e = await run(['status'], { cwd: tmpDir(), lang: 'th' });
  assert.match(e.err, /ไม่พบ anasaki\.json/);
});

test('formatProblem output', () => {
  assert.equal(formatProblem({ file: 'lib/a.ts', line: 2, column: 5, message: 'x\ny' }), 'lib/a.ts:2:5: error: x y');
  assert.equal(formatProblem({ file: 'a.ts', line: 0, column: 0, message: 'm', severity: 'warning' }, 'src'), 'src/a.ts:1:1: warning: m');
});

test('network errors are reported with exit 1', async () => {
  const root = makeProject('http://127.0.0.1:1');
  const r = await run(['status'], { cwd: root });
  assert.equal(r.code, 1);
  assert.match(r.err, /cannot reach http:\/\/127\.0\.0\.1:1/);
});
