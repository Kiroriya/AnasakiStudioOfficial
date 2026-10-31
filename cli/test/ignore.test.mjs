import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { test } from 'node:test';
import { scanLocal, toLocal, toWire } from '../lib/files.mjs';
import { createIgnore, hasSyncExtension, validRemotePath } from '../lib/ignore.mjs';
import { tmpDir, writeFile } from './fake-server.mjs';

test('built-in ignore rules', () => {
  const ig = createIgnore('');
  for (const p of ['.git/config', '.anasaki/state.json', 'lib/.hidden.ts', 'node_modules/x/index.js', 'a/node_modules/y.js',
    'anasaki.json', '.anasakiignore', 'anasaki.d.ts', 'tsconfig.json', '.vscode/settings.json']) {
    assert.equal(ig.ignores(p), true, p);
  }
  for (const p of ['main.ts', 'lib/anasaki.d.ts', 'lib/tsconfig.json', 'data.json', 'README.md']) {
    assert.equal(ig.ignores(p), false, p);
  }
});

test('.anasakiignore globs, comments, anchoring, folders and negation', () => {
  const ig = createIgnore([
    '# a comment',
    '',
    '*.test.ts',
    '/build',
    'drafts/',
    'docs/**/*.md',
    '!docs/keep/**',
    'tmp?.js',
    '[ab].json',
    '!keep.test.ts',
  ].join('\n'));
  assert.equal(ig.ignores('lib/x.test.ts'), true);
  assert.equal(ig.ignores('keep.test.ts'), false);
  assert.equal(ig.ignores('build/out.js'), true); // anchored folder
  assert.equal(ig.ignores('lib/build/out.js'), false);
  assert.equal(ig.ignores('drafts/a.ts'), true);
  assert.equal(ig.ignores('x/drafts/a.ts'), true);
  assert.equal(ig.ignores('drafts', false), false); // folder-only rule does not match a file
  assert.equal(ig.ignores('docs/a/b/c.md'), true);
  assert.equal(ig.ignores('docs/c.md'), true);
  assert.equal(ig.ignores('docs/keep/c.md'), false);
  assert.equal(ig.ignores('tmp1.js'), true);
  assert.equal(ig.ignores('tmp12.js'), false);
  assert.equal(ig.ignores('a.json'), true);
  assert.equal(ig.ignores('c.json'), false);
  assert.equal(ig.ignores('# a comment'), false);
  // Windows separators are accepted
  assert.equal(ig.ignores('lib\\y.test.ts'), true);
});

test('extensions and server path rules', () => {
  assert.equal(hasSyncExtension('a.TS'), true);
  assert.equal(hasSyncExtension('a.png'), false);
  assert.equal(hasSyncExtension('Makefile'), false);
  assert.equal(validRemotePath('lib/a-b_c.ts'), true);
  assert.equal(validRemotePath('lib/a b.ts'), false);
  assert.equal(validRemotePath('../a.ts'), false);
  assert.equal(validRemotePath('ไทย.ts'), false);
});

test('scanLocal applies ignore + extension + name rules with forward-slash paths', () => {
  const dir = tmpDir();
  writeFile(dir, 'main.ts', 'x');
  writeFile(dir, 'lib/util.ts', 'y');
  writeFile(dir, 'lib/deep/z.json', '{}');
  writeFile(dir, 'logo.png', 'png');
  writeFile(dir, 'bad name.ts', 'z');
  writeFile(dir, 'node_modules/p/index.js', 'n');
  writeFile(dir, '.git/HEAD', 'h');
  writeFile(dir, 'skip/me.ts', 's');
  const { files, skipped } = scanLocal(dir, createIgnore('skip/'));
  assert.deepEqual([...files.keys()].sort(), ['lib/deep/z.json', 'lib/util.ts', 'main.ts']);
  assert.deepEqual(skipped.map((s) => s.path + ':' + s.reason).sort(), ['bad name.ts:name', 'logo.png:ext']);
  assert.equal(toWire(dir, path.join(dir, 'lib', 'util.ts')), 'lib/util.ts');
  assert.equal(toLocal(dir, 'lib/util.ts'), path.join(dir, 'lib', 'util.ts'));
  assert.throws(() => toLocal(dir, '../evil.ts'));
  fs.rmSync(dir, { recursive: true, force: true });
});
