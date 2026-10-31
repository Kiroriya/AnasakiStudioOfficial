import { signal } from '@angular/core';
import { Media } from '../core/models';
import { FrameMediaPick } from './plugin-frame';
import { ServicePage, isPluginPageId } from './service-page';

/** ServicePage.onFrameMessage with a fake component (the handler only uses host, contentHeight and router). */
function setup(canMedia = true) {
  const root = document.body.appendChild(document.createElement('div'));
  const frame = root.appendChild(document.createElement('iframe'));
  frame.className = 'svc-frame';
  frame.id = 'code-admin-frame';
  const navigated: string[] = [];
  const self = {
    host: { nativeElement: root }, contentHeight: signal<number | null>(null), router: { navigateByUrl: (u: string) => navigated.push(u) },
    mediaPick: new FrameMediaPick(), auth: { can: (c: string) => canMedia && c === 'upload_files' },
  };
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

  it('opens the media library for AK.pickMedia of its own frame and answers that frame only', () => {
    const { root, frame, self, send } = setup();
    const posted = vi.spyOn(frame.contentWindow!, 'postMessage');
    const other = document.body.appendChild(document.createElement('iframe'));
    send(other.contentWindow, { type: 'ak:pick-media', reqId: 'x1', accept: 'image' });
    send(window, { type: 'ak:pick-media', reqId: 'x2', accept: 'image' });
    expect(self.mediaPick.pending()).toBeNull();
    send(frame.contentWindow, { type: 'ak:pick-media', reqId: 'ak1-1', accept: 'image' });
    expect(self.mediaPick.pending()).toEqual({ frame, reqId: 'ak1-1', accept: 'image' });
    const m: Media = { id: 7, filename: 'a.png', url: '/uploads/a.png', mime: 'image/png', size: 9, width: 640, height: 480, alt: 'A', authorId: 1, createdAt: 'x' };
    self.mediaPick.done(m);
    self.mediaPick.done(null); // the dialog's close after a pick: already answered
    expect(posted).toHaveBeenCalledTimes(1);
    expect(posted).toHaveBeenCalledWith({ type: 'ak:pick-media:result', reqId: 'ak1-1', media: { url: '/uploads/a.png', id: 7, alt: 'A', width: 640, height: 480 } }, '*');
    expect(self.mediaPick.pending()).toBeNull();
    other.remove();
    root.remove();
  });

  it('answers null when the picker is closed, replaced or not allowed', () => {
    const { root, frame, self, send } = setup();
    const posted = vi.spyOn(frame.contentWindow!, 'postMessage');
    send(frame.contentWindow, { type: 'ak:pick-media', reqId: 'r1' });
    send(frame.contentWindow, { type: 'ak:pick-media', reqId: 'r2', accept: 'file' }); // a newer request cancels r1
    expect(self.mediaPick.pending()?.accept).toBe('');
    self.mediaPick.done(null);
    expect(posted.mock.calls).toEqual([
      [{ type: 'ak:pick-media:result', reqId: 'r1', media: null }, '*'],
      [{ type: 'ak:pick-media:result', reqId: 'r2', media: null }, '*'],
    ]);
    send(frame.contentWindow, { type: 'ak:pick-media', reqId: '<bad id>' });
    expect(self.mediaPick.pending()).toBeNull();
    root.remove();

    const denied = setup(false);
    const posted2 = vi.spyOn(denied.frame.contentWindow!, 'postMessage');
    denied.send(denied.frame.contentWindow, { type: 'ak:pick-media', reqId: 'r3', accept: 'image' });
    expect(denied.self.mediaPick.pending()).toBeNull();
    expect(posted2).toHaveBeenCalledWith({ type: 'ak:pick-media:result', reqId: 'r3', media: null }, '*');
    denied.root.remove();
  });

  it('leaves the page header to plugin pages (<plugin>:<page>) but keeps it for the site code and service plugins', () => {
    expect(isPluginPageId('code', 'seo:seo')).toBe(true);
    expect(isPluginPageId('code', 'analytics:stats')).toBe(true);
    expect(isPluginPageId('code', 'reports')).toBe(false);
    expect(isPluginPageId('my-service', 'settings')).toBe(false);
  });
});
