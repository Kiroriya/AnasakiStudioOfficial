import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { after, before, test } from 'node:test';
import { createDebouncedRunner } from '../lib/watch.mjs';
import { makeProject, run, startFakeServer, writeFile } from './fake-server.mjs';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

let srv;
before(async () => { srv = await startFakeServer({ 'main.ts': 'v1', 'keep.ts': 'k' }); });
after(async () => { await srv.close(); });

test('debounced runner: a burst of triggers runs once; a trigger during a run runs once more', async () => {
  let calls = 0, release;
  const runner = createDebouncedRunner(async () => {
    calls++;
    if (calls === 2) await new Promise((r) => { release = r; });
  }, 40);
  for (let i = 0; i < 6; i++) { runner.trigger(); await sleep(5); }
  await sleep(20);
  assert.equal(calls, 0, 'still waiting for quiet');
  await runner.idle();
  assert.equal(calls, 1);

  runner.trigger();
  await sleep(70); // second run starts and blocks
  assert.equal(calls, 2);
  runner.trigger(); runner.trigger();
  await sleep(70);
  assert.equal(calls, 2, 'no overlap while running');
  release();
  await runner.idle();
  assert.equal(calls, 3);
});

async function waitFor(fn, ms = 4000) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    if (fn()) return true;
    await sleep(20);
  }
  return false;
}

for (const mode of ['fs.watch', 'poll']) {
  test(`watch (${mode}): initial push, debounced push on change, never deletes remotely`, async () => {
    srv.st.files.clear();
    srv.setFile('main.ts', 'v1');
    srv.setFile('keep.ts', 'k');
    srv.st.problems = [];
    const root = makeProject(srv.site);
    const pulled = await run(['pull'], { cwd: root });
    assert.equal(pulled.code, 0, pulled.all);
    writeFile(root, 'main.ts', 'v2'); // change before watch starts → initial push
    const before = srv.syncCalls().length;

    const ac = new AbortController();
    const args = ['watch', '--debounce', '300'];
    if (mode === 'poll') args.push('--poll', '50');
    const p = run(args, { cwd: root, signal: ac.signal });
    assert.ok(await waitFor(() => srv.syncCalls().length === before + 1), 'initial push');
    assert.equal(srv.st.files.get('main.ts').content, 'v2');
    await sleep(150); // let the watcher settle (fs.watch may report the state.json write)

    srv.st.problems = [{ file: 'main.ts', line: 2, column: 1, message: 'boom' }];
    const n0 = srv.syncCalls().length;
    // a burst of edits → one push
    for (let i = 3; i <= 7; i++) { writeFile(root, 'main.ts', 'v' + i); await sleep(20); }
    fs.unlinkSync(path.join(root, 'keep.ts'));
    assert.ok(await waitFor(() => srv.st.files.get('main.ts').content === 'v7'), 'debounced push');
    await sleep(500);
    const pushes = srv.syncCalls().slice(n0);
    assert.equal(pushes.length, 1, `expected one debounced push, got ${pushes.length}`);
    assert.deepEqual(pushes[0].body.delete, []);
    assert.ok(srv.st.files.has('keep.ts'), 'watch never deletes remote files');

    ac.abort();
    const r = await p;
    assert.equal(r.code, 0, r.all);
    assert.match(r.out, /^main\.ts:2:1: error: boom$/m);
    assert.match(r.out, /keep\.ts was deleted locally; watch never deletes on the server/);
    assert.match(r.out, /\[anasaki\] sync start/);
    assert.match(r.out, /\[anasaki\] sync done/);
    assert.match(r.out, /Stopped watching/);
  });
}

test('watch keeps running after a conflict and explains it', async () => {
  srv.st.files.clear();
  srv.setFile('main.ts', 'v1');
  srv.st.problems = [];
  const root = makeProject(srv.site);
  await run(['pull'], { cwd: root });
  writeFile(root, 'main.ts', 'mine');
  srv.setFile('main.ts', 'theirs');
  const ac = new AbortController();
  const p = run(['watch', '--poll', '50'], { cwd: root, signal: ac.signal });
  await sleep(300);
  ac.abort();
  const r = await p;
  assert.equal(r.code, 0);
  assert.match(r.err, /Conflict/);
  assert.equal(srv.st.files.get('main.ts').content, 'theirs');
});
