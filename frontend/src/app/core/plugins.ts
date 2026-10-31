import { Injectable, computed, inject, signal } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { Api } from './api';
import { Auth } from './auth';
import { PluginInfo } from './models';
import { PostTypes } from './post-types';

/** Installed plugins and the admin menu entries of the active ones. */
@Injectable({ providedIn: 'root' })
export class Plugins {
  private api = inject(Api);
  readonly list = signal<PluginInfo[]>([]);
  readonly menu = computed(() => this.list().filter(p => p.active).flatMap(p => p.menu ?? []));
  /** Compiled-in plugins (service plugins are packages and are listed with them). */
  readonly builtins = computed(() => this.list().filter(p => p.kind !== 'service'));
  private loading: Promise<void> | null = null;
  private loaded = false;

  /** (Re)loads the list; calls made while a load is in flight share it. */
  load(): Promise<void> {
    this.loading ??= this.api.plugins()
      .then(xs => this.list.set(xs), () => this.list.set([]))
      .finally(() => { this.loaded = true; this.loading = null; });
    return this.loading;
  }

  /** Loads the list unless it has been loaded already. */
  async ensureLoaded() {
    if (!this.loaded) await this.load();
  }

  isActive(id: string) { return this.list().some(p => p.id === id && p.active); }

  async setActive(id: string, active: boolean) {
    const p = await this.api.setPluginActive(id, active);
    this.list.update(xs => xs.map(x => (x.id === id ? p : x)));
    return p;
  }
}

/**
 * Guards /x/:id/:page (a service plugin's admin screen): the capability the plugin
 * declared for that page (the server fills in manage_options when it declares none).
 * Unknown or inactive pages go back to the dashboard. /x/code/:page = an admin page of the site code.
 */
export const pluginPageGuard: CanActivateFn = async route => {
  const auth = inject(Auth);
  const plugins = inject(Plugins);
  const router = inject(Router);
  const types = inject(PostTypes);
  await auth.restore();
  if (route.paramMap.get('id') === 'code') {
    // admin pages of the site code (site.adminPage): the server lists only those this user may open
    await types.ensureLoaded();
    return types.adminPages().some(p => p.id === route.paramMap.get('page')) || router.parseUrl('/');
  }
  await plugins.ensureLoaded();
  const path = `/x/${route.paramMap.get('id')}/${route.paramMap.get('page')}`;
  const item = plugins.menu().find(m => m.path === path);
  return (!!item && auth.can(item.cap || 'manage_options')) || router.parseUrl('/');
};
