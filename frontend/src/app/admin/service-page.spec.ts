import { signal } from '@angular/core';
import { ServicePage } from './service-page';

/** ServicePage.onFrameMessage with a fake component (the handler only uses host, contentHeight and router). */
function setup() {
  const root = document.body.appendChild(document.createElement('div'));
  const frame = root.appendChild(document.createElement('iframe'));
  frame.className = 'svc-frame';
  frame.id = 'code-admin-frame';
  const navigated: string[] = [];
  const self = { host: { nativeElement: root }, contentHeight: signal<number | null>(null), router: { navigateByUrl: (u: string) => navigated.push(u) } };
  const send = (source: unknown, data: unknown) => ServicePage.prototype.onFrameMessage.call(self as unknown as ServicePage, { source, data } as MessageEvent);
  return { root, frame, self, navigated, send };
}

describe('ServicePage admin frame', () => {
  it('grows the frame to the page height (at least the default frame)', () => {
    const { root, frame, self, send } = setup();
    send(frame.contentWindow, { type: 'ak:height', height: 1900 });
    expect(self.contentHeight()).toBe(1900);
    send(frame.contentWindow, { type: 'ak:height', height: 200 });
    expect(self.contentHeight()).toBe(480);
    root.remove();
  });

  it('only listens to its own iframe and only opens admin paths', () => {
    const { root, frame, self, navigated, send } = setup();
    const other = document.body.appendChild(document.createElement('iframe'));
    send(other.contentWindow, { type: 'ak:height', height: 900 });
    send(other.contentWindow, { type: 'ak:navigate', path: '/admin/plugins' });
    expect(self.contentHeight()).toBeNull();
    send(frame.contentWindow, { type: 'ak:navigate', path: '/admin/x/code/analytics:stats?days=30' });
    send(frame.contentWindow, { type: 'ak:navigate', path: '/about' });
    expect(navigated).toEqual(['/x/code/analytics:stats?days=30']);
    other.remove();
    root.remove();
  });
});
