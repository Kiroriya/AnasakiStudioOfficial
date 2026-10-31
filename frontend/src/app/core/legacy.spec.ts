import {
  EMPTY_LEGACY, LegacyItem, LegacySummary, RecommendedItem, defaultSelection, installOutcome, legacyCleanable, legacyErrorKind, legacyName,
  legacyShouldPoll, legacyWaiting, selectionIds,
} from './legacy';
import { importCounts } from '../admin/tools-export';
import { recommendedPerms } from '../admin/setup';
import { I18n, localized } from './i18n';

const item = (id: string, status: LegacyItem['status'], extra: Partial<LegacyItem> = {}): LegacyItem => ({
  id, name: 'ชื่อ ' + id, nameEn: 'Name ' + id, icon: '', status, error: '', attempts: 0, wasActive: true, hadData: true, imported: 0,
  source: '', version: '', approvedBy: '', approvedAt: null, approved: null, nextAttemptAt: null, updatedAt: '', doneAt: null,
  cleanedAt: null, cleanable: false, ...extra,
});
const sum = (items: LegacyItem[], running = false): LegacySummary => ({ waiting: items.filter(i => i.status !== 'done').length, running, items });

describe('legacy migration helpers', () => {
  it('lists the waiting and the cleanable migrations', () => {
    const s = sum([item('forms', 'done', { cleanable: true }), item('seo', 'failed', { attempts: 2 }), item('analytics', 'done', { cleanable: true, cleanedAt: '2026-01-01' }),
      item('redirects', 'importing')]);
    expect(legacyWaiting(s).map(i => i.id)).toEqual(['seo', 'redirects']);
    expect(legacyCleanable(s).map(i => i.id)).toEqual(['forms']);
    expect(legacyWaiting(EMPTY_LEGACY)).toEqual([]);
  });
  it('polls only while the task works', () => {
    expect(legacyShouldPoll(sum([item('a', 'done'), item('b', 'failed', { attempts: 1 })]))).toBe(false);
    expect(legacyShouldPoll(sum([item('a', 'installing')]))).toBe(true);
    expect(legacyShouldPoll(sum([item('a', 'importing')]))).toBe(true);
    expect(legacyShouldPoll(sum([item('a', 'pending')]))).toBe(true); // queued, not tried yet
    expect(legacyShouldPoll(sum([item('a', 'pending', { attempts: 3 })]))).toBe(false);
    expect(legacyShouldPoll(sum([], true))).toBe(true);
    expect(legacyShouldPoll(EMPTY_LEGACY)).toBe(false);
  });
  it('explains failures', () => {
    expect(legacyErrorKind({ status: 'failed', error: 'store unreachable: plugin store unavailable: EOF' })).toBe('store');
    expect(legacyErrorKind({ status: 'failed', error: 'plugin store: status 503' })).toBe('store');
    expect(legacyErrorKind({ status: 'failed', error: 'plugin store: status 404' })).toBe('other');
    expect(legacyErrorKind({ status: 'failed', error: '"x" is not an official replacement' })).toBe('other');
    expect(legacyErrorKind({ status: 'done', error: 'old' })).toBe('');
    expect(legacyErrorKind({ status: 'failed', error: '' })).toBe('');
  });
  it('names in the current language', () => {
    expect(legacyName(item('forms', 'done'), 'en')).toBe('Name forms');
    expect(legacyName(item('forms', 'done'), 'th')).toBe('ชื่อ forms');
    expect(legacyName({ id: 'x', name: '', nameEn: '' }, 'en')).toBe('x');
  });
});

const rec = (id: string, checked: boolean, extra: Partial<RecommendedItem> = {}): RecommendedItem => ({
  id, name: { th: id, en: id }, description: { th: '', en: '' }, icon: '', checked, available: true, version: '2.0.0',
  permissions: ['code'], scopes: ['db'], hosts: [], installed: false, ...extra,
});

describe('setup wizard: recommended plugins', () => {
  const items = [rec('seo', true), rec('redirects', true, { installed: true }), rec('forms', true), rec('analytics', false)];
  it('ticks the defaults that are not installed yet', () => {
    expect([...defaultSelection(items)]).toEqual(['seo', 'forms']);
  });
  it('installs the ticked ones in list order', () => {
    expect(selectionIds(items, new Set(['analytics', 'seo', 'redirects']))).toEqual(['seo', 'analytics']);
    expect(selectionIds(items, new Set())).toEqual([]);
  });
  it('sums up the install', () => {
    expect(installOutcome({ storeAvailable: true, results: [{ id: 'seo', ok: true }] })).toBe('ok');
    expect(installOutcome({ storeAvailable: true, results: [{ id: 'seo', ok: true }, { id: 'forms', ok: false, error: 'x' }] })).toBe('partial');
    expect(installOutcome({ storeAvailable: false, results: [] })).toBe('store');
  });
  it('shows Store names given as a plain string or { th, en }', () => {
    new I18n().set('en');
    expect(localized(rec('shop-x', true, { name: 'Shop X' }).name)).toBe('Shop X');
    expect(localized(rec('seo', true, { name: { th: 'เอสอีโอ', en: 'SEO' } }).name)).toBe('SEO');
  });
  it('describes the permissions in words', () => {
    new I18n().set('en');
    const s = recommendedPerms({ permissions: ['code', 'settings'], scopes: ['routes', 'db'] });
    expect(s).toBe('Has its own settings page · Its own database tables and migrations · Add public routes to the site');
    expect(recommendedPerms({ permissions: ['code'], scopes: [] })).toBe('');
    expect(recommendedPerms({ permissions: [], scopes: ['brand-new'] })).toBe('brand-new');
  });
});

describe('Export / import site', () => {
  it('lists the import counts in order', () => {
    expect(importCounts(null)).toEqual([]);
    expect(importCounts({ imported: { media: 2, posts: 5, terms: 0 }, packages: [], siteCode: { added: [], skipped: 0 } }))
      .toEqual([{ key: 'posts', n: 5 }, { key: 'terms', n: 0 }, { key: 'media', n: 2 }]);
  });
});
