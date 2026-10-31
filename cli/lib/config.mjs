// Project config (anasaki.json), saved credentials (~/.anasaki/credentials.json)
// and the token lookup (ANASAKI_TOKEN wins).
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { CliError } from './errors.mjs';

export const CONFIG_FILE = 'anasaki.json';

/** normalizeSite turns "example.com" / "https://example.com/" into the site key (origin [+ path]). */
export function normalizeSite(site) {
  let s = String(site || '').trim();
  if (!s) return '';
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(s)) {
    s = (/^(localhost|127\.|\[::1\])/i.test(s) ? 'http://' : 'https://') + s;
  }
  let u;
  try { u = new URL(s); } catch { return null; }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return null;
  const p = u.pathname.replace(/\/+$/, '');
  return u.origin + p;
}

/** findProject walks up from cwd to the folder holding anasaki.json. */
export function findProject(cwd) {
  let dir = path.resolve(cwd);
  for (;;) {
    if (fs.existsSync(path.join(dir, CONFIG_FILE))) return dir;
    const up = path.dirname(dir);
    if (up === dir) return null;
    dir = up;
  }
}

export function readProject(root, t) {
  let cfg;
  try {
    cfg = JSON.parse(fs.readFileSync(path.join(root, CONFIG_FILE), 'utf8'));
  } catch (e) {
    throw new CliError(t('err.badConfig', { msg: e.message }), 1);
  }
  const site = normalizeSite(cfg.site);
  if (!site) throw new CliError(t('err.badConfig', { msg: '"site" must be a URL' }), 1);
  const dirRel = String(cfg.dir ?? '.').replace(/\\/g, '/');
  const dir = path.resolve(root, ...dirRel.split('/'));
  const rel = path.relative(root, dir);
  if (rel.startsWith('..') || path.isAbsolute(rel)) {
    throw new CliError(t('err.badConfig', { msg: '"dir" must stay inside the project' }), 1);
  }
  return { root, site, dir, dirRel: rel ? rel.split(path.sep).join('/') : '.', raw: cfg };
}

export function writeProject(root, site, dirRel = '.') {
  const file = path.join(root, CONFIG_FILE);
  fs.writeFileSync(file, JSON.stringify({ site, dir: dirRel }, null, 2) + '\n');
  return file;
}

// ---------------------------------------------------------------- credentials

export function configDir(env) {
  return env.ANASAKI_CONFIG_DIR ? path.resolve(env.ANASAKI_CONFIG_DIR) : path.join(os.homedir(), '.anasaki');
}

export function credentialsFile(env) {
  return path.join(configDir(env), 'credentials.json');
}

export function readCredentials(env) {
  try {
    const v = JSON.parse(fs.readFileSync(credentialsFile(env), 'utf8'));
    return v && typeof v === 'object' ? v : {};
  } catch {
    return {};
  }
}

export function writeCredentials(env, creds) {
  const dir = configDir(env);
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  const file = credentialsFile(env);
  const tmp = file + '.' + process.pid + '.tmp';
  fs.writeFileSync(tmp, JSON.stringify(creds, null, 2) + '\n', { mode: 0o600 });
  fs.renameSync(tmp, file);
  if (process.platform !== 'win32') {
    try { fs.chmodSync(file, 0o600); } catch { /* best effort */ }
  }
  return file;
}

export function tokenFor(env, site) {
  if (env.ANASAKI_TOKEN) return env.ANASAKI_TOKEN.trim();
  const c = readCredentials(env)[site];
  return c && c.token ? c.token : null;
}
