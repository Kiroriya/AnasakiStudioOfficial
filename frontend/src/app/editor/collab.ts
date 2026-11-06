import { HttpClient } from '@angular/common/http';
import { DestroyRef, Injectable, computed, effect, inject, signal, untracked } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { Auth, SESSION_HEADER } from '../core/auth';
import { errorMessage } from '../core/api';
import { tr } from '../core/i18n';
import { EditorStore } from './editor-store';
import './messages-collab';

const BASE = '/api/v1/design';
const HEARTBEAT_MS = 15000;

export interface DesignComment {
  id: number;
  targetKind: 'post' | 'component';
  targetId: number;
  nodeId: string | null;
  x: number;
  y: number;
  body: string;
  authorId: number;
  authorName: string;
  authorAvatar: string;
  parentId: number | null;
  resolved: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Thread { root: DesignComment; replies: DesignComment[]; n: number }

export interface Peer { id: number; name: string; avatar: string; selectedId?: string }

interface Heartbeat {
  users: Peer[];
  updatedAt: string;
  updatedById: number;
  updatedBy: string;
  commentsStamp: string;
}

const PEER_COLORS = ['#f24822', '#9747ff', '#14ae5c', '#ff8c00', '#e91e8c', '#0fa5a5', '#5b5bd6', '#c7a600'];
export const peerColor = (id: number) => PEER_COLORS[Math.abs(id) % PEER_COLORS.length];
export const initials = (name: string) => (name.trim().split(/\s+/).map(w => [...w][0] ?? '').join('').slice(0, 2) || '?').toUpperCase();

/** Same second? (the database stores updated_at with second precision) */
const newer = (a?: string, b?: string) => !!a && !!b && Math.floor(Date.parse(a) / 1000) > Math.floor(Date.parse(b) / 1000);

/**
 * Collaboration state of one designer session: comments (Figma comment
 * mode), presence heartbeat and stale-version detection. Provided next to
 * EditorStore by the editor component.
 */
@Injectable()
export class CollabStore {
  private http = inject(HttpClient);
  private auth = inject(Auth);
  readonly store = inject(EditorStore);
  private session = (crypto.randomUUID?.() ?? String(Math.random()).slice(2)).slice(0, 36);

  /** Comment mode (key C): clicks on the canvas drop comment pins. */
  readonly mode = signal(false);
  readonly panelOpen = signal(false);
  readonly comments = signal<DesignComment[]>([]);
  /** Thread popover open on the canvas (root comment id). */
  readonly openThread = signal<number | null>(null);
  /** New-comment composer: where the pin goes (screen point for the popover). */
  readonly draft = signal<{ nodeId: string | null; x: number; y: number; sx: number; sy: number } | null>(null);
  readonly peers = signal<Peer[]>([]);
  /** Someone else saved a newer version than the one being edited (non-blocking banner). */
  readonly stale = signal<{ by: string; at: string } | null>(null);
  private dismissedAt = '';
  private commentsStamp = '';

  readonly me = computed(() => this.auth.user());
  readonly isAdmin = computed(() => !!this.auth.user()?.caps?.includes('manage_options') || this.auth.user()?.role === 'administrator');

  /** "post:12" / "component:3" once the design is loaded. */
  readonly target = computed(() => {
    const kind = this.store.kind();
    const id = kind === 'post' ? this.store.post()?.id : this.store.component()?.id;
    return id ? { kind, id } : null;
  });
  private readonly targetKey = computed(() => { const t = this.target(); return t ? `${t.kind}:${t.id}` : ''; });

  readonly threads = computed<Thread[]>(() => {
    const all = this.comments();
    const byRoot = new Map<number, DesignComment[]>();
    for (const c of all) if (c.parentId) (byRoot.get(c.parentId) ?? byRoot.set(c.parentId, []).get(c.parentId)!).push(c);
    return all.filter(c => !c.parentId).map((root, i) => ({ root, replies: byRoot.get(root.id) ?? [], n: i + 1 }));
  });
  readonly openCount = computed(() => this.threads().filter(t => !t.root.resolved).length);

  private timer = 0;
  private selTimer = 0;

  constructor() {
    effect(() => {
      const key = this.targetKey();
      untracked(() => this.start(key));
    });
    // tell the others which layer is selected (debounced)
    effect(() => {
      this.store.selectedId();
      untracked(() => {
        clearTimeout(this.selTimer);
        if (this.targetKey()) this.selTimer = window.setTimeout(() => this.beat(), 800);
      });
    });
    // a newer version saved by me (this tab) clears the banner
    effect(() => {
      this.store.post(); this.store.component();
      untracked(() => this.checkStale(this.lastBeat));
    });
    // drawing/moving tools leave comment mode
    effect(() => { if (this.store.tool() !== 'move') untracked(() => this.mode.set(false)); });
    const leave = () => this.leave();
    window.addEventListener('pagehide', leave);
    inject(DestroyRef).onDestroy(() => {
      window.removeEventListener('pagehide', leave);
      clearInterval(this.timer);
      clearTimeout(this.selTimer);
      this.leave();
    });
  }

  private current = '';
  private lastBeat: Heartbeat | null = null;

  private start(key: string) {
    if (key === this.current) return;
    if (this.current) this.leave();
    this.current = key;
    clearInterval(this.timer);
    this.comments.set([]);
    this.peers.set([]);
    this.stale.set(null);
    this.openThread.set(null);
    this.draft.set(null);
    this.commentsStamp = '';
    this.lastBeat = null;
    if (!key) return;
    this.loadComments();
    this.beat();
    this.timer = window.setInterval(() => this.beat(), HEARTBEAT_MS);
  }

  async beat() {
    const t = this.target();
    if (!t || document.hidden && this.lastBeat) return;
    try {
      const hb = await firstValueFrom(this.http.post<Heartbeat>(`${BASE}/presence`, {
        kind: t.kind, id: t.id, session: this.session, selectedId: this.store.selectedId() ?? '',
      }));
      if (this.targetKey() !== `${t.kind}:${t.id}`) return;
      this.lastBeat = hb;
      this.peers.set(hb.users ?? []);
      this.checkStale(hb);
      if (hb.commentsStamp !== this.commentsStamp) {
        if (this.commentsStamp) this.loadComments();
        this.commentsStamp = hb.commentsStamp;
      }
    } catch { /* offline: try again on the next beat */ }
  }

  private checkStale(hb: Heartbeat | null) {
    if (!hb) return;
    const base = this.store.kind() === 'post' ? this.store.post()?.updatedAt : this.store.component()?.updatedAt;
    const mine = hb.updatedById && hb.updatedById === this.me()?.id;
    if (!this.store.saving() && !mine && newer(hb.updatedAt, base) && hb.updatedAt !== this.dismissedAt) {
      this.stale.set({ by: hb.updatedBy || tr('pr.someone'), at: hb.updatedAt });
    } else if (!newer(hb.updatedAt, base)) {
      this.stale.set(null);
    }
  }

  dismissStale() {
    this.dismissedAt = this.stale()?.at ?? '';
    this.stale.set(null);
  }

  /** Reloads the stored design (drops local edits). */
  async reload() {
    const t = this.target();
    if (!t) return;
    this.stale.set(null);
    this.store.conflict.set(null);
    await this.store.load(t.kind, t.id);
    this.store.select(null);
    this.beat();
  }

  private leave() {
    const key = this.current;
    if (!key) return;
    const [kind, id] = key.split(':');
    const token = this.auth.token;
    try {
      fetch(`${BASE}/presence?kind=${kind}&id=${id}&session=${encodeURIComponent(this.session)}`, {
        method: 'DELETE', keepalive: true, headers: token ? { Authorization: `Bearer ${token}`, [SESSION_HEADER]: 'cookie' } : { [SESSION_HEADER]: 'cookie' },
      }).catch(() => undefined);
    } catch { /* best effort: the entry expires anyway */ }
  }

  // ---------------------------------------------------------------- comments

  async loadComments() {
    const t = this.target();
    if (!t) return;
    try {
      const list = await firstValueFrom(this.http.get<DesignComment[]>(`${BASE}/comments`, { params: { kind: t.kind, id: t.id } }));
      if (this.targetKey() === `${t.kind}:${t.id}`) this.comments.set(list ?? []);
    } catch (e) {
      this.fail(e);
    }
  }

  private fail(e: unknown) { this.store.notify(tr('co.failed') + errorMessage(e), true); }

  private upsert(c: DesignComment) {
    const list = this.comments();
    const i = list.findIndex(x => x.id === c.id);
    this.comments.set(i < 0 ? [...list, c] : list.map(x => (x.id === c.id ? c : x)));
  }

  async create(body: string, nodeId: string | null, x: number, y: number): Promise<DesignComment | null> {
    const t = this.target();
    if (!t) return null;
    try {
      const c = await firstValueFrom(this.http.post<DesignComment>(`${BASE}/comments`, { kind: t.kind, id: t.id, nodeId, x, y, body }));
      this.upsert(c);
      return c;
    } catch (e) { this.fail(e); return null; }
  }

  async reply(rootId: number, body: string) {
    try { this.upsert(await firstValueFrom(this.http.post<DesignComment>(`${BASE}/comments`, { parentId: rootId, body }))); return true; }
    catch (e) { this.fail(e); return false; }
  }

  async edit(id: number, body: string) {
    try { this.upsert(await firstValueFrom(this.http.put<DesignComment>(`${BASE}/comments/${id}`, { body }))); return true; }
    catch (e) { this.fail(e); return false; }
  }

  async remove(c: DesignComment) {
    try {
      await firstValueFrom(this.http.delete(`${BASE}/comments/${c.id}`));
      this.comments.set(this.comments().filter(x => x.id !== c.id && x.parentId !== c.id));
      if (this.openThread() === c.id) this.openThread.set(null);
    } catch (e) { this.fail(e); }
  }

  async resolve(c: DesignComment, resolved: boolean) {
    try {
      this.upsert(await firstValueFrom(this.http.post<DesignComment>(`${BASE}/comments/${c.id}/resolve`, { resolved })));
      if (resolved && this.openThread() === c.id) this.openThread.set(null);
    } catch (e) { this.fail(e); }
  }

  toggleMode() {
    const on = !this.mode();
    if (on && this.store.tool() !== 'move') this.store.tool.set('move');
    this.mode.set(on);
    if (on) this.panelOpen.set(true);
    else this.draft.set(null);
  }

  /**
   * Escape on the canvas: closes the comment draft, then the open thread, then
   * comment mode. True when it did something (the editor then stops there).
   */
  escape(): boolean {
    if (this.draft()) this.draft.set(null);
    else if (this.openThread()) this.openThread.set(null);
    else if (this.mode()) this.mode.set(false);
    else return false;
    return true;
  }

  canChange(c: DesignComment) { return c.authorId === this.me()?.id; }
  canDelete(c: DesignComment) { return this.canChange(c) || this.isAdmin(); }
}
