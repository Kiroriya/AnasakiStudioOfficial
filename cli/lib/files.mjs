// Local files: scan the sync folder, hash contents, convert between wire
// paths ("/"-separated, relative to the sync folder) and local paths.
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { createIgnore, hasSyncExtension, validRemotePath } from './ignore.mjs';

export const IGNORE_FILE = '.anasakiignore';

export function sha256(text) {
  return crypto.createHash('sha256').update(Buffer.from(text, 'utf8')).digest('hex');
}

/** toLocal maps a wire path to an absolute local path inside dir. */
export function toLocal(dir, wirePath) {
  const abs = path.resolve(dir, ...wirePath.split('/'));
  const rel = path.relative(dir, abs);
  if (!rel || rel.startsWith('..') || path.isAbsolute(rel)) throw new Error(`unsafe path from server: ${wirePath}`);
  return abs;
}

/** toWire maps a local path (absolute or relative to dir) to a wire path. */
export function toWire(dir, localPath) {
  const rel = path.relative(dir, path.resolve(dir, localPath));
  return rel.split(path.sep).join('/');
}

export function loadIgnore(project) {
  let text = '';
  try { text = fs.readFileSync(path.join(project.root, IGNORE_FILE), 'utf8'); } catch { /* none */ }
  return createIgnore(text);
}

/**
 * scanLocal lists the syncable files of the sync folder.
 * Returns { files: Map<wirePath, {sha256, content}>, skipped: [{path, reason}] }.
 */
export function scanLocal(dir, ig) {
  const files = new Map();
  const skipped = [];
  const walk = (absDir, relDir) => {
    let entries;
    try { entries = fs.readdirSync(absDir, { withFileTypes: true }); } catch { return; }
    entries.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
    for (const e of entries) {
      const rel = relDir ? relDir + '/' + e.name : e.name;
      if (e.isDirectory()) {
        if (!ig.ignores(rel, true)) walk(path.join(absDir, e.name), rel);
        continue;
      }
      if (!e.isFile() || ig.ignores(rel, false)) continue;
      if (!hasSyncExtension(rel)) { skipped.push({ path: rel, reason: 'ext' }); continue; }
      if (!validRemotePath(rel)) { skipped.push({ path: rel, reason: 'name' }); continue; }
      const content = fs.readFileSync(path.join(absDir, e.name)).toString('utf8');
      files.set(rel, { sha256: sha256(content), content });
    }
  };
  walk(dir, '');
  return { files, skipped };
}

/** remoteIncluded tells whether a server path takes part in syncing (same rules as local files). */
export function remoteIncluded(ig, p) {
  return !ig.ignores(p, false);
}

export function writeLocal(dir, wirePath, content) {
  const abs = toLocal(dir, wirePath);
  fs.mkdirSync(path.dirname(abs), { recursive: true });
  fs.writeFileSync(abs, content);
}

export function removeLocal(dir, wirePath) {
  const abs = toLocal(dir, wirePath);
  try { fs.unlinkSync(abs); } catch (e) { if (e.code !== 'ENOENT') throw e; }
  // prune empty parent folders up to the sync folder
  let d = path.dirname(abs);
  while (d !== dir && d.startsWith(dir)) {
    try { fs.rmdirSync(d); } catch { break; }
    d = path.dirname(d);
  }
}
