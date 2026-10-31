import { Injectable, inject, signal } from '@angular/core';
import { Api } from './api';
import { tr } from './i18n';
import { DemoStatus } from './models';

/** Product name used in the admin UI, receipts and the browser tab. */
export const PRODUCT = 'Anasaki Studio';

/** Site-wide info shown in the admin (sidebar brand, browser tab, favicon). */
@Injectable({ providedIn: 'root' })
export class SiteInfo {
  private api = inject(Api);
  readonly title = signal('');
  readonly logo = signal('');
  readonly icon = signal('');
  readonly version = signal('');
  /** Demo mode details (null on a normal site). */
  readonly demo = signal<DemoStatus | null>(null);

  private publicLoad: Promise<void> | null = null;

  /**
   * Public info (works before login: login and setup screens). Fetched once per app
   * start (App calls it); later calls reuse the result. A failed fetch (server offline)
   * is forgotten so the next call tries again.
   */
  loadPublic(): Promise<void> {
    this.publicLoad ??= this.api.publicSite().then(
      s => {
        this.setTitle(s.siteTitle ?? '');
        this.setIcon(s.siteIcon ?? '');
        this.logo.set(s.siteLogo ?? '');
        this.version.set(s.version ?? '');
        this.demo.set(s.demo?.enabled ? s.demo : null);
      },
      () => { this.publicLoad = null; },
    );
    return this.publicLoad;
  }

  async load() {
    try {
      const s = await this.api.settings();
      this.setTitle(s['site_title'] ?? '');
      this.setIcon(s['site_icon'] ?? '');
      this.logo.set(s['site_logo'] ?? '');
    } catch { /* not logged in yet */ }
  }

  setTitle(t: string) {
    this.title.set(t);
    document.title = t ? `${t} — ${tr('brand.sub')}` : PRODUCT;
  }

  setIcon(url: string) {
    this.icon.set(url);
    let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (!link) {
      link = document.createElement('link');
      link.rel = 'icon';
      document.head.appendChild(link);
    }
    const href = url || 'favicon.svg';
    link.href = href;
    // The type must match the file, or some browsers ignore the icon.
    const ext = href.split('?')[0].split('.').pop()?.toLowerCase() ?? '';
    const types: Record<string, string> = { svg: 'image/svg+xml', png: 'image/png', ico: 'image/x-icon', jpg: 'image/jpeg', jpeg: 'image/jpeg', webp: 'image/webp', gif: 'image/gif' };
    if (types[ext]) link.type = types[ext]; else link.removeAttribute('type');
  }
}
