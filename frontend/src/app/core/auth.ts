import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { Injectable, computed, inject, signal } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { Api } from './api';
import { I18n, Locale, isLocale } from './i18n';
import { Role, User } from './models';

/**
 * Since 2.3.0 the session lives in an HttpOnly cookie that page script can't read (security assessment 45);
 * every API call sends SESSION_HEADER so the server accepts that cookie. TOKEN_KEY is only read: a token put
 * there (older sessions, tests) is still sent as a bearer. SIGNED_IN_KEY holds no secret — it tells other tabs
 * and the next visit that a session cookie should exist.
 */
const TOKEN_KEY = 'anasaki.token';
const SIGNED_IN_KEY = 'anasaki.signedIn';
export const SESSION_HEADER = 'X-Anasaki-Session';
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
      if (e.key !== TOKEN_KEY && e.key !== SIGNED_IN_KEY && e.key !== null) return;
      void this.syncFromOtherTab(this.token ?? signedInMark());
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

  /** A bearer token from storage (older sessions, tests); null with the cookie session. */
  get token(): string | null {
    try { return localStorage.getItem(TOKEN_KEY); } catch { return null; }
  }

  /** Whether a session may exist (cookie mark or stored token), so restore() asks the server. */
  private get maybeSignedIn(): boolean {
    return !!(this.token || signedInMark());
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
    try {
      // the server set the HttpOnly cookie and answered no token; an API-style answer still works
      if (res.token) localStorage.setItem(TOKEN_KEY, res.token);
      else localStorage.removeItem(TOKEN_KEY);
      localStorage.setItem(SIGNED_IN_KEY, String(res.user.id) + ':' + Date.now());
    } catch { /* private mode */ }
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

  /** Renews the session (cookie or stored token); silently keeps the old one on failure. */
  async renew() {
    if (!this.user()) return;
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
    if (isLocale(u.locale)) this.i18n.set(u.locale);
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
    if (!this.maybeSignedIn) return false;
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
    if (server && this.user()) this.api.logout(all).catch(() => undefined);
    this.clear();
    this.router.navigateByUrl('/login');
  }

  clear() {
    try { localStorage.removeItem(TOKEN_KEY); localStorage.removeItem(SIGNED_IN_KEY); } catch { /* ignore */ }
    this.stopRefresh();
    this.user.set(null);
  }
}

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const auth = inject(Auth);
  const token = auth.token;
  let r = req;
  if (req.url.startsWith('/api/')) {
    r = req.clone({ setHeaders: token ? { Authorization: `Bearer ${token}`, [SESSION_HEADER]: 'cookie' } : { [SESSION_HEADER]: 'cookie' } });
  }
  return next(r).pipe(
    catchError((e: HttpErrorResponse) => {
      // 401 = session expired; 403 on /auth/me = account suspended (signed-out callers, e.g. restore(), handle it)
      if ((e.status === 401 || (e.status === 403 && req.url.endsWith('/auth/me'))) && !req.url.endsWith('/auth/login') && auth.loggedIn()) auth.logout(false, false);
      return throwError(() => e);
    }),
  );
};

function signedInMark(): string | null {
  try { return localStorage.getItem(SIGNED_IN_KEY); } catch { return null; }
}

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
