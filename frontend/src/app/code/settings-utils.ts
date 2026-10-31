/**
 * Pure helpers of the Site Code "Settings & secrets" tab (/code/settings):
 * editable rows, key / host / mail-limit validation and the PUT body
 * (secret values are write-only: '' keeps the stored one). Unit tested in
 * settings-utils.spec.ts.
 */
import { CodeSettingValue, CodeSettings, CodeSettingsUpdate } from '../core/code-models';

/** Keys usable with site.settings('KEY'). */
export const SETTING_KEY_RE = /^[A-Za-z_][A-Za-z0-9_.-]{0,99}$/;
export const MAIL_LIMIT_MAX = 10000;
export const MASK = '••••••••';

export interface SettingRow {
  key: string;
  secret: boolean;
  /** A value is stored on the server. */
  set: boolean;
  /** Plain settings: the value (edited in place). */
  value: string;
  /** Secrets: the replacement typed in ('' = keep the stored one); null = not replacing. */
  replace: string | null;
  isNew: boolean;
  removed: boolean;
}

export function rowsFromSettings(values: CodeSettingValue[] | null | undefined): SettingRow[] {
  return (values ?? [])
    .filter(v => v && typeof v.key === 'string')
    .map(v => ({ key: v.key, secret: !!v.secret, set: !!v.set, value: v.secret ? '' : v.value ?? '', replace: null, isNew: false, removed: false }))
    .sort((a, b) => a.key.localeCompare(b.key));
}

export type KeyError = 'empty' | 'invalid' | 'duplicate';
export function keyError(key: string, rows: SettingRow[]): KeyError | null {
  const k = key.trim();
  if (!k) return 'empty';
  if (!SETTING_KEY_RE.test(k)) return 'invalid';
  if (rows.some(r => r.key.toLowerCase() === k.toLowerCase())) return 'duplicate';
  return null;
}

/** A pasted URL becomes its host ("https://api.x.com/v1" → "api.x.com"). */
export function normalizeHost(s: string): string {
  let h = s.trim().toLowerCase();
  h = h.replace(/^[a-z][a-z0-9+.-]*:\/\//, '');
  h = h.split(/[/?#]/)[0];
  return h.replace(/\.$/, '');
}

const HOST_RE = /^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$/;
export type HostError = 'empty' | 'invalid' | 'ip' | 'port' | 'duplicate';

/** Allowlist entries: an exact host or `*.example.com`; no IPs, ports or localhost. */
export function hostError(host: string, list: string[]): HostError | null {
  if (!host) return 'empty';
  if (/:\d+$/.test(host)) return 'port';
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(host) || host.includes(':') || host.startsWith('[')) return 'ip';
  if (host === 'localhost' || host.endsWith('.localhost') || !HOST_RE.test(host)) return 'invalid';
  if (list.includes(host)) return 'duplicate';
  return null;
}

export function mailLimitError(n: number | string | null | undefined): 'invalid' | null {
  const v = typeof n === 'number' ? n : Number(n);
  return n === '' || n === null || n === undefined || !Number.isInteger(v) || v < 0 || v > MAIL_LIMIT_MAX ? 'invalid' : null;
}

/** New secrets need a value; plain values may be empty. */
export function rowInvalid(r: SettingRow): boolean {
  return !r.removed && r.secret && r.isNew && !(r.replace ?? '').length;
}

/** PUT /code/settings body. */
export function buildSettingsPut(rows: SettingRow[], hosts: string[], mailPerHour: number): CodeSettingsUpdate {
  return {
    values: rows.filter(r => !r.removed).map(r => ({ key: r.key, secret: r.secret, value: r.secret ? r.replace ?? '' : r.value })),
    fetchHosts: [...hosts],
    limits: { mailPerHour: Number(mailPerHour) },
    remove: rows.filter(r => r.removed && !r.isNew).map(r => r.key),
  };
}

/** Anything to save compared with what the server sent? */
export function settingsDirty(orig: CodeSettings | null, rows: SettingRow[], hosts: string[], mailPerHour: number | string): boolean {
  if (!orig) return false;
  if (Number(mailPerHour) !== Number(orig.limits?.mailPerHour ?? 0)) return true;
  const oh = orig.fetchHosts ?? [];
  if (hosts.length !== oh.length || hosts.some((h, i) => h !== oh[i])) return true;
  const base = rowsFromSettings(orig.values);
  for (const r of rows) {
    if (r.isNew) { if (!r.removed) return true; continue; }
    if (r.removed || (r.secret && r.replace)) return true;
    const b = base.find(x => x.key === r.key);
    if (!b || (!r.secret && r.value !== b.value)) return true;
  }
  return false;
}
