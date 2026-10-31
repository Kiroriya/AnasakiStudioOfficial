// watch: initial push, then a debounced push after file changes. Uses
// fs.watch({recursive}) where available, else polls the folder.
import fs from 'node:fs';
import path from 'node:path';
import { scanLocal } from './files.mjs';

/**
 * createDebouncedRunner calls fn once `ms` after the last trigger(). A
 * trigger while fn runs schedules exactly one more run afterwards.
 */
export function createDebouncedRunner(fn, ms = 300) {
  let timer = null, running = false, again = false, idle = Promise.resolve();
  let resolveIdle = null;
  const run = async () => {
    timer = null;
    if (running) { again = true; return; }
    running = true;
    try {
      do {
        again = false;
        await fn();
      } while (again);
    } finally {
      running = false;
      if (!timer && resolveIdle) { const r = resolveIdle; resolveIdle = null; r(); }
    }
  };
  return {
    trigger() {
      if (timer) clearTimeout(timer);
      if (!resolveIdle) idle = new Promise((r) => { resolveIdle = r; });
      timer = setTimeout(run, ms);
    },
    cancel() {
      if (timer) clearTimeout(timer);
      timer = null;
      if (!running && resolveIdle) { const r = resolveIdle; resolveIdle = null; r(); }
    },
    /** idle resolves when no run is pending or in progress. */
    idle() { return timer || running ? idle : Promise.resolve(); },
    get running() { return running; },
  };
}

function fingerprint(dir, ig) {
  const { files } = scanLocal(dir, ig);
  return [...files].map(([p, f]) => p + ':' + f.sha256).join('\n');
}

/**
 * startWatcher calls onChange for relevant changes under dir. Returns a
 * stop function. mode: 'auto' (fs.watch recursive, else poll) or 'poll'.
 */
export function startWatcher(dir, ig, onChange, { mode = 'auto', pollMs = 1000, onPollFallback } = {}) {
  const relevant = (filename) => {
    if (!filename) return true;
    const rel = String(filename).split(path.sep).join('/');
    return !ig.ignores(rel, false);
  };
  if (mode !== 'poll') {
    try {
      const w = fs.watch(dir, { recursive: true }, (_ev, filename) => {
        if (relevant(filename)) onChange(filename);
      });
      w.on('error', () => {});
      return () => w.close();
    } catch {
      // recursive watching is unavailable on this platform/Node build
    }
  }
  if (onPollFallback) onPollFallback(pollMs);
  let last = fingerprint(dir, ig);
  const iv = setInterval(() => {
    const now = fingerprint(dir, ig);
    if (now !== last) { last = now; onChange(null); }
  }, pollMs);
  return () => clearInterval(iv);
}
