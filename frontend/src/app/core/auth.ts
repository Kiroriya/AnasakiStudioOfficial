import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { Injectable, computed, inject, signal } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { Api } from './api';
import { I18n, Locale } from './i18n';
import { Role, User } from './models';

const TOKEN_KEY = 'anasaki.token';
/** Sessions last 12 hours on the server; the open admin renews its token this often. */
const REFRESH_EVERY_MS = 30 * 60 * 1000;

export type Cap =
  | 'edit_own_posts' | 'edit_others_posts' | 'edit_pages' | 'upload_files'
  | 'manage_categories' | 'manage_options' | 'manage_users' | 'view_stats' | 'publish_posts' | 'read' | 'unfiltered_html'
  | 'view_scripts' | 'edit_scripts' | 'publish_scripts' | 'manage_scripts' | 'view_code_data' | 'edit_code_data'
  | (string & {});

/**
 * Fallback role → capability matrix, mirroring the built-in roles of backend/internal/auth/caps.go
 * (the server sends `user.caps`, which wins; the API enforces it).
 */
const CAPS: Record<string, Cap[]> = {
  administrator: ['read', 'edit_own_posts', 'edit_others_posts', 'publish_posts', 'edit_pages', 'upload_files', 'manage_categories', 'manage_options', 'manage_users', 'unfiltered_html', 'view_stats',
    'view_scripts', 'edit_scripts', 'publish_scripts', 'manage_scripts', 'view_code_data', 'edit_code_data'],
  editor: ['read', 'edit_own_posts', 'edit_others_posts', 'publish_posts', 'edit_pages', 'upload_files', 'manage_categories', 'unfiltered_html', 'view_stats'],
  author: ['read', 'edit_own_posts', 'publish_posts', 'upload_files'],
  contributor: ['read', 'edit_own_posts'],
  subscriber: ['read'],
};

@Injectable({ providedIn: 'root' })
export class Auth {
  private api = inject(Api);
  private router = inject(Router);
  private i18n = inject(I18n);

  readonly user = signal<User | null>(null);
  readonly loggedIn = computed(() => this.user() !== null);
  private refreshTimer: ReturnType<typeof setInterval> | null = null;

  constructor() {
    // Keep every open tab in step: signing in/out in one tab applies to all.
    window.addEventListener('storage', e => {
      if (e.key !== TOKEN_KEY && e.key !== null) return;
      void this.syncFromOtherTab(e.newValue);
    });
  }

  private async syncFromOtherTab(token: string | null) {
    if (!token) {
      if (this.user()) { this.user.set(null); this.router.navigateByUrl('/login'); }
      return;
    }
    const prev = this.user()?.id;
    this.user.set(null);
    if (!(await this.restore())) return;
    const onPublicScreen = ['/login', '/forgot', '/reset', '/setup'].some(p => this.router.url.startsWith(p));
    if (onPublicScreen) this.router.navigateByUrl(this.home());
    else if (prev !== undefined && prev !== this.user()?.id) location.reload(); // a different account signed in
  }

  get token(): string | null {
    try { return localStorage.getItem(TOKEN_KEY); } catch { return null; }
  }

  can(cap: Cap): boolean {
    const u = this.user();
    if (!u) return false;
    return u.caps ? u.caps.includes(cap) : !!CAPS[u.role]?.includes(cap);
  }

  async login(email: string, password: string) {
    this.accept(await this.api.login(email, password));
  }

  /** Stores a session returned by login or the setup wizard. */
  accept(res: { token: string; user: User }) {
    try { localStorage.setItem(TOKEN_KEY, res.token); } catch { /* private mode */ }
    this.setUser(res.user);
    this.startRefresh();
  }

  /** Renews the session token while the admin is open (sliding 12-hour sessions). */
  private startRefresh() {
    if (this.refreshTimer !== null || typeof setInterval !== 'function') return;
    this.refreshTimer = setInterval(() => void this.renew(), REFRESH_EVERY_MS);
  }

  private stopRefresh() {
    if (this.refreshTimer !== null) clearInterval(this.refreshTimer);
    this.refreshTimer = null;
  }

  /** Swaps the stored token for a fresh one (silently keeps the old one on failure). */
  async renew() {
    if (!this.token) return;
    try {
      const res = await this.api.refreshSession();
      if (res?.token) { try { localStorage.setItem(TOKEN_KEY, res.token); } catch { /* private mode */ } }
    } catch { /* 401 → the interceptor signs out */ }
  }

  /** Saves the profile; a new password ends the other sessions and returns a fresh token for this one. */
  async updateMe(data: Partial<User> & { password?: string; currentPassword?: string }) {
    const { token, ...user } = await this.api.updateMe(data);
    if (token) this.accept({ token, user });
    else this.setUser(user);
  }

  /** Where to land after signing in, based on what the account may do. */
  home(): string {
    return '/';
  }

  /** Applies the account (and its UI language). */
  setUser(u: User) {
    this.user.set(u);
    if (u.locale === 'th' || u.locale === 'en') this.i18n.set(u.locale);
  }

  /** Switches language and remembers it on the account. */
  async setLocale(l: Locale) {
    this.i18n.set(l);
    if (this.user()) this.setUser(await this.api.updateMe({ locale: l }));
  }

  /** Reloads the account (e.g. after plugins change capabilities). */
  async refresh() {
    try { this.setUser(await this.api.me()); } catch { /* keep current */ }
  }

  /** Restores the session from the stored token. */
  async restore(): Promise<boolean> {
    if (this.user()) return true;
    if (!this.token) return false;
    try {
      this.setUser(await this.api.me());
      this.startRefresh();
      void this.renew(); // a token restored from storage may be hours old
      return true;
    } catch {
      this.clear();
      return false;
    }
  }

  /** Signs out: the server revokes the token (all = every session of the account). */
  logout(all = false, server = true) {
    if (server && this.token && this.user()) this.api.logout(all).catch(() => undefined);
    this.clear();
    this.router.navigateByUrl('/login');
  }

  clear() {
    try { localStorage.removeItem(TOKEN_KEY); } catch { /* ignore */ }
    this.stopRefresh();
    this.user.set(null);
  }
}

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const auth = inject(Auth);
  const token = auth.token;
  const r = token && req.url.startsWith('/api/') ? req.clone({ setHeaders: { Authorization: `Bearer ${token}` } }) : req;
  return next(r).pipe(
    catchError((e: HttpErrorResponse) => {
      // 401 = session expired; 403 on /auth/me = account suspended
      if ((e.status === 401 || (e.status === 403 && req.url.endsWith('/auth/me'))) && !req.url.endsWith('/auth/login')) auth.logout(false, false);
      return throwError(() => e);
    }),
  );
};

// inject() only works synchronously, so resolve services before any await.
export const authGuard: CanActivateFn = async () => {
  const auth = inject(Auth);
  const router = inject(Router);
  const setup = inject(SetupState);
  if (await setup.needed()) return router.parseUrl('/setup');
  return (await auth.restore()) || router.parseUrl('/login');
};

/** Remembers whether the first-run wizard still has to be completed. */
@Injectable({ providedIn: 'root' })
export class SetupState {
  private api = inject(Api);
  private cached: boolean | null = null;

  /**
   * True while the first-run wizard is pending. A failed check (server
   * starting, network blip) is retried a few times; if it still fails the answer
   * is "unknown": this navigation proceeds as if set up, but nothing is cached,
   * so the next navigation asks the server again.
   */
  async needed(): Promise<boolean> {
    if (this.cached === false) return false;
    for (const wait of [0, 400, 1200]) {
      if (wait) await new Promise(r => setTimeout(r, wait));
      try {
        this.cached = (await this.api.setupStatus()).needsSetup;
        return this.cached;
      } catch { /* retry */ }
    }
    this.cached = null;
    return false;
  }

  done() { this.cached = false; }
}

/** /login is only reachable once the site is set up, and only while signed out. */
export const loginGuard: CanActivateFn = async () => {
  const router = inject(Router);
  const setup = inject(SetupState);
  const auth = inject(Auth);
  if (await setup.needed()) return router.parseUrl('/setup');
  // Already signed in (e.g. in another tab): skip the login screen.
  return (await auth.restore()) ? router.parseUrl(auth.home()) : true;
};

/** /setup is only reachable on a fresh install. */
export const setupGuard: CanActivateFn = async () => {
  const router = inject(Router);
  const setup = inject(SetupState);
  return (await setup.needed()) || router.parseUrl('/login');
};

export const capGuard = (cap: Cap): CanActivateFn => async () => {
  const auth = inject(Auth);
  const router = inject(Router);
  await auth.restore();
  return auth.can(cap) || router.parseUrl('/');
};
