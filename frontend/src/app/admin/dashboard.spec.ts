import { signal } from '@angular/core';
import { DashboardWidget } from '../core/admin-ext';
import { Dashboard, arrange, dropId, moveId, readLayout } from './dashboard';
import { FrameMediaPick } from './plugin-frame';

/** Dashboard.onFrameMessage with a fake component (no TestBed: the handler only uses host, heights and router). */
function setup() {
  const root = document.body.appendChild(document.createElement('div'));
  const frame = (id: string) => {
    const f = root.appendChild(document.createElement('iframe'));
    f.className = 'dash-widget-frame';
    f.dataset['id'] = id;
    return f;
  };
  const a = frame('analytics:visitors');
  const b = frame('other:card');
  const navigated: string[] = [];
  const self = {
    host: { nativeElement: root },
    heights: signal<Record<string, number>>({}),
    tallest: signal<Record<string, number>>({}),
    tallWidget: 420,
    layout: signal(readLayout(null)),
    sizeOf: Dashboard.prototype.sizeOf,
    router: { navigateByUrl: (u: string) => navigated.push(u) },
    mediaPick: new FrameMediaPick(),
    auth: { can: (c: string) => c === 'upload_files' },
  };
  const send = (source: unknown, data: unknown) => Dashboard.prototype.onFrameMessage.call(self as unknown as Dashboard, { source, data } as MessageEvent);
  return { root, a, b, self, navigated, send };
}

describe('Dashboard widget frames', () => {
  it('sizes the sending widget frame to its content (clamped 80..2000)', () => {
    const { root, a, b, self, send } = setup();
    send(a.contentWindow, { type: 'ak:height', height: 412 });
    send(b.contentWindow, { type: 'ak:height', height: 5000 });
    expect(self.heights()).toEqual({ 'analytics:visitors': 412, 'other:card': 2000 });
    send(a.contentWindow, { type: 'ak:height', height: 10 });
    expect(self.heights()['analytics:visitors']).toBe(80);
    const w: DashboardWidget = { id: 'x:y', label: 'X', icon: '', url: '/ext-ui/code/t/widget/y', height: 330 };
    expect(Dashboard.prototype.frameHeight.call(self as unknown as Dashboard, w)).toBe(330); // registered height until it reports
    root.remove();
  });

  it('2.5.3: rows by height (masonry) and a tall widget stays two columns wide', () => {
    const { root, a, self, send } = setup();
    const d = self as unknown as Dashboard;
    const w: DashboardWidget = { id: 'analytics:visitors', label: 'V', icon: '', url: '/ext-ui/code/t/widget/v', height: 340 };
    const card: DashboardWidget = { id: 'other:card', label: 'C', icon: '', url: '/ext-ui/code/t/widget/c', height: 150 };
    (self as any).frameHeight = Dashboard.prototype.frameHeight;
    expect(Dashboard.prototype.rowSpan.call(d, card)).toBe(Math.ceil((150 + 46 + 16) / 4));
    expect(Dashboard.prototype.isWide.call(d, w)).toBe(false); // 340 registered
    send(a.contentWindow, { type: 'ak:height', height: 650 });
    expect(Dashboard.prototype.isWide.call(d, w)).toBe(true);
    send(a.contentWindow, { type: 'ak:height', height: 380 }); // wider frame, shorter chart
    expect(Dashboard.prototype.isWide.call(d, w)).toBe(true);  // still wide: no flip-flopping
    expect(Dashboard.prototype.rowSpan.call(d, w)).toBe(Math.ceil((380 + 46 + 16) / 4));
    root.remove();
  });

  it('2.5.3: the user picks the size — normal stays narrow, wide is wide at any height', () => {
    const { root, self } = setup();
    const d = self as unknown as Dashboard;
    const tall: DashboardWidget = { id: 'analytics:visitors', label: 'V', icon: '', url: '/ext-ui/code/t/widget/v', height: 700 };
    const small: DashboardWidget = { id: 'other:card', label: 'C', icon: '', url: '/ext-ui/code/t/widget/c', height: 150 };
    expect(Dashboard.prototype.isWide.call(d, tall)).toBe(true);
    self.layout.set(readLayout({ size: { 'analytics:visitors': 'normal', 'other:card': 'wide' } }));
    expect(Dashboard.prototype.isWide.call(d, tall)).toBe(false);
    expect(Dashboard.prototype.isWide.call(d, small)).toBe(true);
    root.remove();
  });

  it('ignores messages from other windows and invalid messages', () => {
    const { root, self, navigated, send } = setup();
    const stranger = document.body.appendChild(document.createElement('iframe'));
    send(stranger.contentWindow, { type: 'ak:height', height: 400 });
    send(window, { type: 'ak:navigate', path: '/admin/plugins' });
    send(null, { type: 'ak:height', height: 400 });
    expect(self.heights()).toEqual({});
    expect(navigated).toEqual([]);
    stranger.remove();
    root.remove();
  });

  it('opens admin paths asked by a widget (AK.open) with the router, nothing else', () => {
    const { root, a, navigated, send } = setup();
    send(a.contentWindow, { type: 'ak:navigate', path: '/admin/x/code/analytics:stats' });
    send(a.contentWindow, { type: 'ak:navigate', path: 'https://evil.test/' });
    send(a.contentWindow, { type: 'ak:navigate', path: '//evil.test/admin/' });
    expect(navigated).toEqual(['/x/code/analytics:stats']);
    root.remove();
  });

  it('opens the media library for AK.pickMedia of a widget and answers that widget', () => {
    const { root, a, b, self, send } = setup();
    const toA = vi.spyOn(a.contentWindow!, 'postMessage'), toB = vi.spyOn(b.contentWindow!, 'postMessage');
    send(window, { type: 'ak:pick-media', reqId: 'w0' });
    expect(self.mediaPick.pending()).toBeNull();
    send(b.contentWindow, { type: 'ak:pick-media', reqId: 'w1', accept: 'image' });
    expect(self.mediaPick.pending()?.frame).toBe(b);
    self.mediaPick.done(null);
    expect(toB).toHaveBeenCalledWith({ type: 'ak:pick-media:result', reqId: 'w1', media: null }, '*');
    expect(toA).not.toHaveBeenCalled();
    root.remove();
  });
});

describe('Dashboard layout (2.5.3)', () => {
  const W = ['a:1', 'b:2', 'c:3', 'd:4'].map(id => ({ id }));
  const ids = (x: { id: string }[]) => x.map(w => w.id);

  it('reads any stored value into a well-formed layout', () => {
    expect(readLayout(null)).toEqual({ order: [], hidden: [], size: {} });
    expect(readLayout('x')).toEqual({ order: [], hidden: [], size: {} });
    expect(readLayout({ order: ['a:1', 3, null], hidden: 'b:2', size: { 'a:1': 'wide', 'b:2': 'huge', 'c:3': 'normal' } }))
      .toEqual({ order: ['a:1'], hidden: [], size: { 'a:1': 'wide', 'c:3': 'normal' } });
  });

  it('puts the widgets in the saved order; new ones after, in their own order; hidden ones apart', () => {
    expect(ids(arrange(W, readLayout(null)).shown)).toEqual(['a:1', 'b:2', 'c:3', 'd:4']);
    const r = arrange(W, readLayout({ order: ['c:3', 'gone:9', 'a:1'], hidden: ['d:4'] }));
    expect(ids(r.shown)).toEqual(['c:3', 'a:1', 'b:2']);
    expect(ids(r.hidden)).toEqual(['d:4']);
  });

  it('moves up/down by one and drops a card in the place of another', () => {
    const o = ['a', 'b', 'c', 'd'];
    expect(moveId(o, 'b', -1)).toEqual(['b', 'a', 'c', 'd']);
    expect(moveId(o, 'd', 1)).toEqual(o);
    expect(moveId(o, 'a', -1)).toEqual(o);
    expect(dropId(o, 'a', 'c')).toEqual(['b', 'c', 'a', 'd']); // down: after
    expect(dropId(o, 'd', 'b')).toEqual(['a', 'd', 'b', 'c']); // up: before
    expect(dropId(o, 'x', 'b')).toEqual(o);
    expect(o).toEqual(['a', 'b', 'c', 'd']); // never changed in place
  });
});
