import { I18n } from '../core/i18n';
import {
  CODE_SCOPES, compareVersions, consentCanConfirm, consentCaps, consentFor, consentNeeded, dependencyStatus, dependentsFromError, dependentsOf,
  destructiveFromError, destructiveText, isJsPlugin, isSensitiveScope, missingDeps, missingFromError, namesOf, normalizeRequires,
  optionalIcon, optionalStatus, satisfies, scopeLabel, sortScopes,
} from './plugin-deps';

const pkg = (id: string, version: string, active = true, extra: Record<string, unknown> = {}) => ({ id, name: id.toUpperCase(), version, active, ...extra });

describe('plugin dependencies: semver', () => {
  it('compares versions incl. prereleases', () => {
    expect(compareVersions('1.2.3', '1.2.3')).toBe(0);
    expect(compareVersions('1.10.0', '1.9.9')).toBe(1);
    expect(compareVersions('1.0.0-beta', '1.0.0')).toBe(-1);
    expect(compareVersions('2.0.0', '10.0.0')).toBe(-1);
  });
  it('caret, tilde, x-ranges, comparators and alternatives', () => {
    const t = (v: string, r: string) => satisfies(v, r);
    expect([t('1.4.0', '^1.0.0'), t('2.0.0', '^1.0.0'), t('0.9.9', '^1.0.0')]).toEqual([true, false, false]);
    expect([t('0.2.5', '^0.2.3'), t('0.3.0', '^0.2.3'), t('0.0.3', '^0.0.3'), t('0.0.4', '^0.0.3')]).toEqual([true, false, true, false]);
    expect([t('1.2.9', '~1.2.3'), t('1.3.0', '~1.2.3'), t('1.9.0', '~1'), t('2.0.0', '~1')]).toEqual([true, false, true, false]);
    expect([t('1.7.1', '1.x'), t('2.0.0', '1.x'), t('1.2.7', '1.2.x'), t('1.3.0', '1.2'), t('5.0.0', '*'), t('5.0.0', '')]).toEqual([true, false, true, false, true, true]);
    expect([t('1.5.0', '>=1.0.0 <2.0.0'), t('2.0.0', '>=1.0.0 <2.0.0'), t('1.0.0', '>= 1.0.0'), t('1.0.0', '>1.0.0')]).toEqual([true, false, true, false]);
    expect([t('3.1.0', '^1.0.0 || ^3.0.0'), t('2.1.0', '^1.0.0 || ^3.0.0')]).toEqual([true, false]);
    expect([t('1.2.3', '1.2.3'), t('1.2.4', '=1.2.3')]).toEqual([true, false]);
    expect([t('1.0.0', 'banana'), t('nope', '^1.0.0')]).toEqual([false, false]);
  });
});

describe('plugin dependencies: status', () => {
  it('normalises map and list forms', () => {
    expect(normalizeRequires({ money: '^1.0.0', auth: '~2.1' })).toEqual([{ id: 'auth', range: '~2.1' }, { id: 'money', range: '^1.0.0' }]);
    expect(normalizeRequires([{ id: 'money', range: '^1.0.0', version: '1.4.0' }, { id: 'x' } as never, null as never])).toEqual([
      { id: 'money', range: '^1.0.0', installed: null, latest: '1.4.0' }, { id: 'x', range: '*', installed: null, latest: null },
    ]);
    expect(normalizeRequires(undefined)).toEqual([]);
  });
  it('ok / missing / incompatible / inactive', () => {
    const installed = [pkg('money', '1.4.0'), pkg('auth', '1.0.0'), pkg('mail', '2.0.0', false)];
    const st = dependencyStatus(normalizeRequires({ money: '^1.0.0', auth: '^2.0.0', mail: '^2.0.0', tax: '*' }), installed);
    expect(st.map(d => [d.id, d.state, d.installedVersion])).toEqual([
      ['auth', 'incompatible', '1.0.0'], ['mail', 'inactive', '2.0.0'], ['money', 'ok', '1.4.0'], ['tax', 'missing', null],
    ]);
    expect(st.find(d => d.id === 'money')!.name).toBe('MONEY');
    expect(missingDeps(normalizeRequires({ money: '^1.0.0', auth: '^2.0.0', mail: '^2.0.0', tax: '*' }), installed).map(d => d.id)).toEqual(['auth', 'tax']);
  });
  it('dependents from the package or from the others\' requires', () => {
    const all = [
      { id: 'money', requires: undefined, dependents: ['shop'] },
      { id: 'shop', requires: { money: '^1' } },
      { id: 'cart', requires: [{ id: 'shop', range: '*' }] },
      { id: 'blog' },
    ];
    expect(dependentsOf('money', all)).toEqual(['shop']);
    expect(dependentsOf('shop', all)).toEqual(['cart']);
    expect(dependentsOf('blog', all)).toEqual([]);
    expect(namesOf(['shop', 'ghost'], [{ id: 'shop', name: 'Shop' }])).toEqual(['Shop', 'ghost']);
  });
});

describe('plugin dependencies: error bodies', () => {
  it('missing / dependents / destructive', () => {
    const e400 = { status: 400, error: { error: 'x', missing: [{ id: 'money', range: '^1.0.0', installed: '0.9.0' }, { bad: 1 }] } };
    expect(missingFromError(e400)).toEqual([{ id: 'money', range: '^1.0.0', installed: '0.9.0', latest: null }]);
    expect(missingFromError({ error: JSON.stringify({ missing: [{ id: 'a', range: '1.x' }] }) })).toEqual([{ id: 'a', range: '1.x', installed: null, latest: null }]);
    expect(missingFromError({ error: 'plain text' })).toEqual([]);
    expect(dependentsFromError({ status: 409, error: { error: 'x', dependents: ['shop', { id: 'cart' }, 3] } })).toEqual(['shop', 'cart']);
    expect(dependentsFromError(new Error('x'))).toEqual([]);
    const d = destructiveFromError({ error: { destructive: [{ op: 'drop_column', table: 'p_shop_orders', column: 'note', migration: 'shop/002.ts' }, { nope: 1 }] } });
    expect(d.length).toBe(1);
    expect(destructiveText(d[0])).toBe('drop_column p_shop_orders.note (shop/002.ts)');
    expect(destructiveText({ op: 'drop_table', table: 't' })).toBe('drop_table t');
  });
});

describe('plugin code scopes and consent', () => {
  it('labels in th and en; unknown scopes show their id', () => {
    const i18n = new I18n();
    const before = i18n.locale();
    try {
      i18n.locale.set('en');
      expect(scopeLabel('db')).toBe('Its own database tables and migrations');
      expect(scopeLabel('content-write')).toContain('Create, edit and delete');
      expect(scopeLabel('teleport')).toBe('teleport');
      for (const s of CODE_SCOPES) expect(scopeLabel(s)).not.toBe(s);
      i18n.locale.set('th');
      expect(scopeLabel('mail')).toBe('ส่งอีเมลจากเว็บ');
      for (const s of CODE_SCOPES) expect(scopeLabel(s)).not.toBe(s);
    } finally { i18n.locale.set(before); }
  });
  it('sensitivity and canonical order', () => {
    expect(CODE_SCOPES.filter(isSensitiveScope)).toEqual(['content-write', 'mail', 'http', 'visitor', 'filters-core', 'content-fields', 'webhooks', 'terms-write', 'media-write']);
    expect(isSensitiveScope('post-types')).toBe(false);
    expect(sortScopes(['routes', 'zzz', 'db', 'db', 'mail'])).toEqual(['db', 'mail', 'routes', 'zzz']);
    expect(isJsPlugin({ runtime: 'JS' })).toBe(true);
    expect(isJsPlugin({ runtime: 'wasm' })).toBe(false);
    expect(isJsPlugin(null)).toBe(false);
  });
  it('consent content for install, re-consent and destructive migrations', () => {
    const src = { name: 'Shop', version: '1.2.0', runtime: 'js', permissions: ['code', 'blocks'], code: { scopes: ['routes', 'db', 'mail'], hosts: ['api.x.com'], routes: ['/shop'] } };
    const c = consentFor('install', src);
    expect(c).toEqual({ mode: 'install', name: 'Shop', version: '1.2.0', runtime: 'js', permissions: ['code', 'blocks'], api: [], scopes: ['db', 'mail', 'routes'], hosts: ['api.x.com'], routes: ['/shop'], destructive: [], capabilities: [] });
    expect(consentNeeded(c)).toBe(true);
    expect(consentNeeded(consentFor('install', { name: 'Plain' }))).toBe(false);
    const rc = consentFor('reconsent', src, { fromVersion: '1.0.0', scopes: ['http', 'db'], hosts: ['b.com'] });
    expect([rc.permissions, rc.scopes, rc.hosts, rc.routes]).toEqual([[], ['db', 'http'], ['b.com'], []]);
    expect(consentNeeded(consentFor('reconsent', { name: 'x' }, { fromVersion: '1' }))).toBe(true);
    const d = consentFor('upgrade', src, null, [{ op: 'drop_table', table: 'p_shop_x' }]);
    expect(consentNeeded(consentFor('upgrade', { name: 'x' }, null, d.destructive))).toBe(true);
    expect([consentCanConfirm(d, false), consentCanConfirm(d, true), consentCanConfirm(c, false)]).toEqual([false, true, true]);
  });
  it('consent lists the capabilities a plugin adds (CMS 2.0.6), in the admin language', () => {
    const src = {
      id: 'analytics', name: 'Analytics', permissions: ['code'],
      capabilities: [
        { id: 'view', label: { th: 'ดูสถิติ', en: 'View analytics' }, description: { th: 'เปิดหน้าสถิติ', en: 'Open the page' }, default: ['administrator', 'editor'] },
        { id: 'export', key: 'analytics.export', label: 'Export' },
      ],
    };
    expect(consentFor('install', src, null, [], 'en').capabilities).toEqual([
      { key: 'analytics.view', label: 'View analytics', description: 'Open the page', default: ['administrator', 'editor'] },
      { key: 'analytics.export', label: 'Export', description: '', default: [] },
    ]);
    expect(consentFor('install', src).capabilities![0].label).toBe('ดูสถิติ');
    expect(consentCaps(undefined, [{ id: 'x', label: { en: 'X' } }], 'th')).toEqual([{ key: 'x', label: 'X', description: '', default: [] }]);
    expect(consentCaps('p', null, 'en')).toEqual([]);
    // re-consent is about permissions only
    expect(consentFor('reconsent', src, { fromVersion: '1.0.0' }).capabilities).toBeUndefined();
  });
});

describe('optionalRequires ("works with")', () => {
  const installed = [
    { id: 'thai', version: '1.2.0', active: true, name: 'Thai' },
    { id: 'shop', version: '2.0.0', active: false, name: 'Shop' },
    { id: 'old', version: '0.9.0', active: true, name: 'Old' },
  ];
  it('states from a manifest map', () => {
    const s = optionalStatus({ thai: '^1.0.0', shop: '^2.0.0', old: '^1.0.0', none: '*' }, installed);
    expect(s.map(d => [d.id, d.state])).toEqual([['none', 'missing'], ['old', 'incompatible'], ['shop', 'inactive'], ['thai', 'ok']]);
    expect(s.find(d => d.id === 'old')!.installedVersion).toBe('0.9.0');
    expect(s.map(d => optionalIcon(d))).toEqual(['○', '⚠', '⏸', '✓']);
  });
  it('states from a resolved list; malformed input is ignored', () => {
    expect(optionalStatus([{ id: 'thai', range: '^1.0.0', installed: '1.2.0' }, { id: 'thai', range: '^1' }, null, { range: 'x' }, 'thai'], installed).map(d => [d.id, d.state]))
      .toEqual([['thai', 'ok']]);
    expect(optionalStatus('thai', installed)).toEqual([]);
    expect(optionalStatus(null, installed)).toEqual([]);
    expect(optionalStatus({ thai: { bad: 1 }, x: '^1' }, installed).map(d => d.id)).toEqual(['x']);
  });
});
