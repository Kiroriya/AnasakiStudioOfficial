import {
  ancestors, buildTree, entryPoint, extOf, filenameFromDisposition, flattenTree, formatBytes, joinPath, languageFor, movedPath,
  parentDir, parseHeaderLines, parseQuery, prettyBody, problemsByFile, resolveProblemPath, splitRunPath, summarizeUpload,
  toProblems, validatePath, versionRows,
} from './code-utils';

describe('validatePath', () => {
  it('accepts normal project paths', () => {
    for (const p of ['main.ts', 'lib/format.ts', 'a/b/c-d_e.f.js', 'data.json', 'README.md', 'x.mjs']) expect(validatePath(p)).toBeNull();
  });

  it('refuses what the server refuses', () => {
    expect(validatePath('')).toBe('empty');
    expect(validatePath('/main.ts')).toBe('leadingSlash');
    expect(validatePath('lib\\x.ts')).toBe('backslash');
    expect(validatePath('../x.ts')).toBe('dotSegment');
    expect(validatePath('lib/./x.ts')).toBe('dotSegment');
    expect(validatePath('lib//x.ts')).toBe('badSegment');
    expect(validatePath('lib/x y.ts')).toBe('badSegment');
    expect(validatePath('lib/ไทย.ts')).toBe('badSegment');
    expect(validatePath('lib/')).toBe('badSegment');
    expect(validatePath('style.css')).toBe('badExtension');
    expect(validatePath('noext')).toBe('badExtension');
    expect(validatePath('a'.repeat(253) + '.ts')).toBe('tooLong');
    expect(validatePath('a'.repeat(252) + '.ts')).toBeNull();
  });

  it('refuses case-insensitive duplicates and file/folder clashes', () => {
    const existing = ['main.ts', 'lib/format.ts'];
    expect(validatePath('MAIN.ts', existing)).toBe('duplicate');
    expect(validatePath('Lib/Format.TS', existing)).toBe('duplicate');
    expect(validatePath('lib', existing, 'folder')).toBe('duplicate');
    expect(validatePath('LIB', existing, 'folder')).toBe('duplicate');
    expect(validatePath('main.ts/x.ts', existing)).toBe('duplicate');
    expect(validatePath('lib/other.ts', existing)).toBeNull();
    expect(validatePath('tools', existing, 'folder')).toBeNull();
  });

  it('does not require an extension for folders', () => {
    expect(validatePath('lib/utils', [], 'folder')).toBeNull();
    expect(validatePath('lib/../x', [], 'folder')).toBe('dotSegment');
  });
});

describe('path helpers', () => {
  it('splits and joins paths', () => {
    expect(extOf('a/b.TS')).toBe('.ts');
    expect(extOf('a/.hidden')).toBe('');
    expect(parentDir('a/b/c.ts')).toBe('a/b');
    expect(parentDir('c.ts')).toBe('');
    expect(joinPath('', 'x.ts')).toBe('x.ts');
    expect(joinPath('lib/', 'x.ts')).toBe('lib/x.ts');
    expect(ancestors('a/b/c.ts')).toEqual(['a', 'a/b']);
    expect(ancestors('c.ts')).toEqual([]);
  });

  it('maps moved files and folders', () => {
    expect(movedPath('lib/a.ts', 'lib', 'src')).toBe('src/a.ts');
    expect(movedPath('lib', 'lib', 'src')).toBe('src');
    expect(movedPath('library/a.ts', 'lib', 'src')).toBeNull();
    expect(movedPath('main.ts', 'main.ts', 'app.ts')).toBe('app.ts');
  });

  it('picks languages and entry points', () => {
    expect(languageFor('a.ts')).toBe('typescript');
    expect(languageFor('a.mjs')).toBe('javascript');
    expect(languageFor('a.json')).toBe('json');
    expect(languageFor('a.md')).toBe('markdown');
    expect(entryPoint(['index.js', 'main.js'])).toBe('main.js');
    expect(entryPoint(['index.ts', 'index.js'])).toBe('index.ts');
    expect(entryPoint(['lib/main.ts'])).toBeNull();
  });

  it('formats sizes', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(600 * 1024)).toBe('600 KB');
    expect(formatBytes(2 * 1024 * 1024)).toBe('2.0 MB');
  });
});

describe('buildTree', () => {
  const files = [
    { path: 'main.ts', size: 10 },
    { path: 'lib/z.ts', size: 1 },
    { path: 'lib/A.ts', size: 2 },
    { path: 'lib/deep/x/y.ts', size: 3 },
    { path: 'README.md', size: 4 },
    { path: 'events.ts', size: 5 },
    { path: 'file10.ts', size: 6 },
    { path: 'file2.ts', size: 7 },
  ];

  it('derives nested folders and sorts folders first, then names (case-insensitive, natural)', () => {
    const tree = buildTree(files);
    expect(tree.map(n => `${n.kind}:${n.name}`)).toEqual(['folder:lib', 'file:events.ts', 'file:file2.ts', 'file:file10.ts', 'file:main.ts', 'file:README.md']);
    const lib = tree[0];
    expect(lib.path).toBe('lib');
    expect(lib.children.map(n => n.name)).toEqual(['deep', 'A.ts', 'z.ts']);
    const deep = lib.children[0];
    expect(deep.path).toBe('lib/deep');
    expect(deep.children[0].path).toBe('lib/deep/x');
    expect(deep.children[0].children[0]).toMatchObject({ path: 'lib/deep/x/y.ts', name: 'y.ts', kind: 'file', size: 3 });
  });

  it('adds empty UI folders and accepts plain strings', () => {
    const tree = buildTree(['a.ts'], ['tools/new', 'lib/']);
    expect(tree.map(n => n.path)).toEqual(['lib', 'tools', 'a.ts']);
    expect(tree[1].children[0]).toMatchObject({ path: 'tools/new', kind: 'folder', children: [] });
  });

  it('does not duplicate a folder that is both extra and derived', () => {
    const tree = buildTree(['lib/a.ts'], ['lib']);
    expect(tree.length).toBe(1);
    expect(tree[0].children.length).toBe(1);
  });

  it('flattens only expanded folders, with depth', () => {
    const tree = buildTree(files);
    expect(flattenTree(tree, new Set()).length).toBe(6);
    const rows = flattenTree(tree, new Set(['lib', 'lib/deep']));
    expect(rows.map(r => `${r.depth}:${r.node.name}`).slice(0, 6)).toEqual(['0:lib', '1:deep', '2:x', '1:A.ts', '1:z.ts', '0:events.ts']);
  });
});

describe('summarizeUpload', () => {
  const r = { added: ['b.ts', 'a.ts'], changed: ['main.ts'], removed: ['old.ts'], unchanged: 3 };

  it('keeps removals only in replace mode', () => {
    expect(summarizeUpload(r, 'merge')).toEqual({ added: ['a.ts', 'b.ts'], changed: ['main.ts'], removed: [], unchanged: 3, writes: 3, empty: false, destructive: false });
    const rep = summarizeUpload(r, 'replace');
    expect(rep.removed).toEqual(['old.ts']);
    expect(rep.destructive).toBe(true);
  });

  it('detects a no-op upload and tolerates missing lists', () => {
    const s = summarizeUpload({ unchanged: 5 } as never, 'replace');
    expect(s.empty).toBe(true);
    expect(s.writes).toBe(0);
    expect(s.unchanged).toBe(5);
  });
});

describe('build problems', () => {
  const known = ['main.ts', 'lib/format.ts'];

  it('resolves reported files to draft files', () => {
    expect(resolveProblemPath('./main.ts', known)).toBe('main.ts');
    expect(resolveProblemPath('/lib/format.ts', known)).toBe('lib/format.ts');
    expect(resolveProblemPath('lib\\format.ts', known)).toBe('lib/format.ts');
    expect(resolveProblemPath('MAIN.TS', known)).toBe('main.ts');
    expect(resolveProblemPath('site-code/lib/format.ts', known)).toBe('lib/format.ts');
    expect(resolveProblemPath('anasaki.d.ts', known)).toBeNull();
    expect(resolveProblemPath('', known)).toBeNull();
  });

  it('orders errors before warnings by file/line and clamps positions', () => {
    const p = toProblems({
      errors: [
        { file: 'main.ts', line: 9, column: 2, message: 'b' },
        { file: './lib/format.ts', line: 3, column: 0, message: 'a' },
        { file: 'main.ts', line: 2, column: 5, message: 'c' },
      ],
      warnings: [{ file: '', line: 0, column: 0, message: 'no entry' }],
    }, known);
    expect(p.map(x => `${x.severity}:${x.file}:${x.line}:${x.column}`)).toEqual([
      'error:lib/format.ts:3:1', 'error:main.ts:2:5', 'error:main.ts:9:2', 'warning::1:1',
    ]);
    expect(p[0].path).toBe('lib/format.ts');
    expect(p[3].path).toBeNull();
    const byFile = problemsByFile(p);
    expect([...byFile.keys()]).toEqual(['lib/format.ts', 'main.ts']);
    expect(byFile.get('main.ts')!.length).toBe(2);
  });

  it('handles a build without problems', () => {
    expect(toProblems({ errors: [], warnings: [] }, known)).toEqual([]);
    expect(toProblems({ errors: null as never, warnings: undefined as never }, known)).toEqual([]);
  });
});

describe('versionRows', () => {
  it('sorts newest first and formats labels', () => {
    const rows = versionRows([
      { id: 3, message: 'Add route', createdAt: '2026-10-01T10:00:00Z', createdBy: 'Ann', live: false, files: 4 },
      { id: 7, message: '  ', createdAt: '2026-10-02T10:00:00Z', createdBy: '', live: true, files: 5 },
    ], iso => iso.slice(0, 10), '(untitled)');
    expect(rows).toEqual([
      { id: 7, label: '#7', message: '(untitled)', when: '2026-10-02', by: '—', files: 5, live: true },
      { id: 3, label: '#3', message: 'Add route', when: '2026-10-01', by: 'Ann', files: 4, live: false },
    ]);
  });
});

describe('test route helpers', () => {
  it('parses query strings and header lines', () => {
    expect(parseQuery('?a=1&b=x%20y&&c=')).toEqual({ a: '1', b: 'x y', c: '' });
    expect(parseQuery('  ')).toEqual({});
    expect(parseHeaderLines('Accept: application/json\nbad line\nX-Token:  abc:def \n')).toEqual({ Accept: 'application/json', 'X-Token': 'abc:def' });
  });

  it('splits a path with a query', () => {
    expect(splitRunPath('/hello?x=1', 'y=2')).toEqual({ path: '/hello', query: { x: '1', y: '2' } });
    expect(splitRunPath('hello', '')).toEqual({ path: '/hello', query: {} });
    expect(splitRunPath('', '')).toEqual({ path: '/', query: {} });
  });

  it('pretty-prints JSON bodies only', () => {
    expect(prettyBody('{"a":1}', { 'Content-Type': 'application/json' })).toEqual({ text: '{\n  "a": 1\n}', json: true });
    expect(prettyBody('[1,2]')).toEqual({ text: '[\n  1,\n  2\n]', json: true });
    expect(prettyBody('<h1>hi</h1>', { 'content-type': ['text/html'] })).toEqual({ text: '<h1>hi</h1>', json: false });
    expect(prettyBody('{broken', {})).toEqual({ text: '{broken', json: false });
    expect(prettyBody(undefined)).toEqual({ text: '', json: false });
  });

  it('reads file names from Content-Disposition', () => {
    expect(filenameFromDisposition('attachment; filename="site-code.zip"', 'x')).toBe('site-code.zip');
    expect(filenameFromDisposition("attachment; filename*=UTF-8''lib%20a.zip", 'x')).toBe('lib a.zip');
    expect(filenameFromDisposition('attachment; filename=main.ts', 'x')).toBe('main.ts');
    expect(filenameFromDisposition(null, 'fallback.zip')).toBe('fallback.zip');
  });
});
