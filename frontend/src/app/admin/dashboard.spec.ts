import { signal } from '@angular/core';
import { DashboardWidget } from '../core/admin-ext';
import { Dashboard } from './dashboard';

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
    router: { navigateByUrl: (u: string) => navigated.push(u) },
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
});
