// .anasaki/state.json in the project root: the hashes of the last sync
// (what local and server agreed on), used to tell local from remote changes.
import fs from 'node:fs';
import path from 'node:path';

export function stateFile(root) {
  return path.join(root, '.anasaki', 'state.json');
}

export function loadState(root, warn) {
  const file = stateFile(root);
  let raw;
  try { raw = fs.readFileSync(file, 'utf8'); } catch { return emptyState(); }
  try {
    const v = JSON.parse(raw);
    if (!v || typeof v !== 'object' || typeof v.files !== 'object' || !v.files) throw new Error('shape');
    return { version: 1, site: v.site || null, liveVersion: v.liveVersion ?? null, syncedAt: v.syncedAt || null, files: { ...v.files } };
  } catch {
    if (warn) warn();
    return emptyState();
  }
}

export function emptyState() {
  return { version: 1, site: null, liveVersion: null, syncedAt: null, files: {} };
}

export function saveState(root, state) {
  const file = stateFile(root);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  const sorted = {};
  for (const k of Object.keys(state.files).sort()) sorted[k] = state.files[k];
  const out = { version: 1, site: state.site, liveVersion: state.liveVersion ?? null, syncedAt: new Date().toISOString(), files: sorted };
  const tmp = file + '.' + process.pid + '.tmp';
  fs.writeFileSync(tmp, JSON.stringify(out, null, 2) + '\n');
  fs.renameSync(tmp, file);
}
