import { noticeTarget, normalizeNotices, normalizeWidgets } from './admin-ext';

describe('admin notices', () => {
  it('keeps valid notices, defaults the level, drops unsafe links', () => {
    const n = normalizeNotices([
      { id: 'maintenance:1', unit: 'maintenance', level: 'warning', text: 'On', link: { label: 'View', href: '/?x=1', external: false } },
      { id: 'b', unit: 'x', level: 'weird', text: 'Hi', link: { label: 'bad', href: 'javascript:alert(1)' } },
      { id: 'c', text: '' },
      null,
    ]);
    expect(n.length).toBe(2);
    expect(n[0].link?.href).toBe('/?x=1');
    expect(n[1].level).toBe('info');
    expect(n[1].link).toBeUndefined();
    expect(normalizeNotices({})).toEqual([]);
  });

  it('routes admin links in the app and opens site paths in a tab', () => {
    expect(noticeTarget({ href: '/admin/plugins', external: false }, '/admin/', '')).toEqual({ kind: 'router', path: '/plugins' });
    expect(noticeTarget({ href: '/admin/x/code/a:b', external: false }, '/', '')).toEqual({ kind: 'router', path: '/x/code/a:b' });
    expect(noticeTarget({ href: '/?bypass=k', external: false }, '/admin/', 'http://s')).toEqual({ kind: 'tab', href: 'http://s/?bypass=k' });
    expect(noticeTarget({ href: 'https://x.test', external: true }, '/admin/', '')).toEqual({ kind: 'tab', href: 'https://x.test' });
  });
});

describe('dashboard widgets', () => {
  it('accepts only /ext-ui/code widget URLs and clamps the height', () => {
    const w = normalizeWidgets([
      { id: 'analytics:visitors', label: { th: 'ผู้เข้าชม', en: 'Visitors' }, icon: '📈', url: '/ext-ui/code/tk/widget/analytics:visitors', height: 260 },
      { id: 'evil', url: 'https://evil.test/widget/x' },
      { id: 'small', url: '/ext-ui/code/tk/widget/small', height: 5 },
    ]);
    expect(w.map(x => x.id)).toEqual(['analytics:visitors', 'small']);
    expect(w[0].height).toBe(260);
    expect(w[1].height).toBe(220);
  });
});
