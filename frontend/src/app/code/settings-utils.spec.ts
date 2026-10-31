import { CodeSettings } from '../core/code-models';
import { SettingRow, buildSettingsPut, hostError, keyError, mailLimitError, normalizeHost, rowInvalid, rowsFromSettings, settingsDirty } from './settings-utils';

const ORIG: CodeSettings = {
  values: [
    { key: 'STRIPE_KEY', secret: true, set: true },
    { key: 'API_BASE', secret: false, set: true, value: 'https://api.example.com' },
    { key: 'WEBHOOK_SECRET', secret: true, set: false },
  ],
  fetchHosts: ['api.example.com', '*.stripe.com'],
  limits: { mailPerHour: 100 },
};

describe('settings rows', () => {
  it('builds rows sorted by key; secrets never carry a value', () => {
    const rows = rowsFromSettings([...ORIG.values, { key: 'X', secret: true, set: true, value: 'leak' }]);
    expect(rows.map(r => r.key)).toEqual(['API_BASE', 'STRIPE_KEY', 'WEBHOOK_SECRET', 'X']);
    expect(rows.find(r => r.key === 'X')?.value).toBe('');
    expect(rows[0]).toEqual({ key: 'API_BASE', secret: false, set: true, value: 'https://api.example.com', replace: null, isNew: false, removed: false });
    expect(rowsFromSettings(undefined)).toEqual([]);
  });
  it('validates new keys', () => {
    const rows = rowsFromSettings(ORIG.values);
    expect(keyError('', rows)).toBe('empty');
    expect(keyError('9LIVES', rows)).toBe('invalid');
    expect(keyError('has space', rows)).toBe('invalid');
    expect(keyError('stripe_key', rows)).toBe('duplicate');
    expect(keyError('MAIL_FROM', rows)).toBeNull();
    expect(keyError('line.channel-token', rows)).toBeNull();
  });
});

describe('fetch host allowlist', () => {
  it('normalises pasted URLs', () => {
    expect(normalizeHost(' https://API.Example.com/v1?x=1 ')).toBe('api.example.com');
    expect(normalizeHost('*.example.com.')).toBe('*.example.com');
  });
  it('accepts exact hosts and *.domain, refuses IPs, ports, localhost, duplicates', () => {
    const list = ['api.example.com'];
    expect(hostError('hooks.slack.com', list)).toBeNull();
    expect(hostError('*.stripe.com', list)).toBeNull();
    expect(hostError('', list)).toBe('empty');
    expect(hostError('api.example.com', list)).toBe('duplicate');
    expect(hostError('10.0.0.1', list)).toBe('ip');
    expect(hostError('[::1]', list)).toBe('ip');
    expect(hostError('example.com:8443', list)).toBe('port');
    expect(hostError('localhost', list)).toBe('invalid');
    expect(hostError('nodot', list)).toBe('invalid');
    expect(hostError('a.*.com', list)).toBe('invalid');
    expect(hostError('-bad.com', list)).toBe('invalid');
  });
  it('mail limit', () => {
    expect(mailLimitError(100)).toBeNull();
    expect(mailLimitError('0')).toBeNull();
    expect(mailLimitError('')).toBe('invalid');
    expect(mailLimitError(-1)).toBe('invalid');
    expect(mailLimitError(2.5)).toBe('invalid');
    expect(mailLimitError(10001)).toBe('invalid');
  });
});

describe('PUT body and dirty state', () => {
  it('nothing changed → not dirty; secrets sent empty (= unchanged)', () => {
    const rows = rowsFromSettings(ORIG.values);
    expect(settingsDirty(ORIG, rows, [...ORIG.fetchHosts], 100)).toBe(false);
    expect(settingsDirty(ORIG, rows, [...ORIG.fetchHosts], '100')).toBe(false);
    expect(buildSettingsPut(rows, ORIG.fetchHosts, 100)).toEqual({
      values: [
        { key: 'API_BASE', secret: false, value: 'https://api.example.com' },
        { key: 'STRIPE_KEY', secret: true, value: '' },
        { key: 'WEBHOOK_SECRET', secret: true, value: '' },
      ],
      fetchHosts: ['api.example.com', '*.stripe.com'],
      limits: { mailPerHour: 100 },
      remove: [],
    });
  });
  it('replace a secret, edit a plain value, remove one, add new ones', () => {
    const rows: SettingRow[] = rowsFromSettings(ORIG.values).map(r =>
      r.key === 'STRIPE_KEY' ? { ...r, replace: 'sk_live_new' } : r.key === 'API_BASE' ? { ...r, value: 'https://v2.example.com' } : r.key === 'WEBHOOK_SECRET' ? { ...r, removed: true } : r);
    rows.push({ key: 'NEW_SECRET', secret: true, set: false, value: '', replace: 'abc', isNew: true, removed: false });
    rows.push({ key: 'GONE', secret: false, set: false, value: 'x', replace: null, isNew: true, removed: true });
    expect(settingsDirty(ORIG, rows, ORIG.fetchHosts, 100)).toBe(true);
    const body = buildSettingsPut(rows, ['api.example.com'], 50);
    expect(body.values).toEqual([
      { key: 'API_BASE', secret: false, value: 'https://v2.example.com' },
      { key: 'STRIPE_KEY', secret: true, value: 'sk_live_new' },
      { key: 'NEW_SECRET', secret: true, value: 'abc' },
    ]);
    expect(body.remove).toEqual(['WEBHOOK_SECRET']);
    expect(body.limits.mailPerHour).toBe(50);
  });
  it('dirty on hosts / mail / a started-but-empty replace is not a change', () => {
    const rows = rowsFromSettings(ORIG.values);
    expect(settingsDirty(ORIG, rows, ['api.example.com'], 100)).toBe(true);
    expect(settingsDirty(ORIG, rows, ORIG.fetchHosts, 99)).toBe(true);
    expect(settingsDirty(ORIG, rows.map(r => (r.secret ? { ...r, replace: '' } : r)), ORIG.fetchHosts, 100)).toBe(false);
    expect(settingsDirty(null, rows, [], 0)).toBe(false);
  });
  it('a new secret needs a value', () => {
    expect(rowInvalid({ key: 'A', secret: true, set: false, value: '', replace: '', isNew: true, removed: false })).toBe(true);
    expect(rowInvalid({ key: 'A', secret: true, set: false, value: '', replace: 'v', isNew: true, removed: false })).toBe(false);
    expect(rowInvalid({ key: 'A', secret: true, set: true, value: '', replace: '', isNew: false, removed: false })).toBe(false);
  });
});
