import {
  ChangeDetectionStrategy, Component, DestroyRef, ElementRef, ViewEncapsulation, afterNextRender, computed, effect, inject, signal, untracked,
} from '@angular/core';
import { Dialog } from '../core/dialog';
import { I18n, TPipe, tr } from '../core/i18n';
import { CollabStore, DesignComment, Thread, initials, peerColor } from './collab';
import { findNode } from './design-types';
import './messages-collab';

interface Pt { x: number; y: number; detached?: boolean }
interface Box { x: number; y: number; w: number; h: number; color: string; label: string }

/**
 * Comment pins, thread popovers, the comments panel and other users'
 * selections, drawn over the canvas (placed inside `.fg-canvas`). Positions
 * follow the layers' `[data-node-id]` elements, so zoom, pan, scroll and
 * layout changes are tracked without hooks in the canvas.
 */
@Component({
  selector: 'rea-comments-layer',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: { class: 'co-layer' }, // Escape is handled by the editor (collab.escape), so it acts once
  template: `
    @for (b of peerBoxes(); track $index) {
      <div class="co-peer-box" [style.left.px]="b.x" [style.top.px]="b.y" [style.width.px]="b.w" [style.height.px]="b.h" [style.border-color]="b.color">
        <span [style.background]="b.color">{{ b.label }}</span>
      </div>
    }

    @if (collab.mode()) { <div class="co-hint" [class.with-panel]="collab.panelOpen()">💬 {{ 'co.modeHint' | t }}</div> }

    @if (collab.stale(); as st) {
      <div class="co-stale" role="status" [class.with-panel]="collab.panelOpen()">
        <span>⚠ {{ 'pr.stale' | t: { name: st.by, time: i18n.date(st.at, true) } }}</span>
        <button class="co-btn primary" (click)="reloadStale()">{{ 'pr.reload' | t }}</button>
        <button class="co-btn" (click)="collab.dismissStale()">{{ 'pr.dismiss' | t }}</button>
      </div>
    }

    @for (t of visibleThreads(); track t.root.id) {
      @if (pins()[t.root.id]; as p) {
        <button class="co-pin" [class.resolved]="t.root.resolved" [class.active]="collab.openThread() === t.root.id" [class.detached]="p.detached"
                [style.left.px]="p.x" [style.top.px]="p.y" [attr.data-comment-id]="t.root.id"
                [title]="t.root.authorName + ': ' + t.root.body" (click)="togglePin(t)" (pointerdown)="$event.stopPropagation()">
          <span>{{ initialsOf(t.root.authorName) }}</span>
        </button>
      }
    }

    @if (collab.draft(); as d) {
      <div class="co-pop" [style.left.px]="popX(d.sx)" [style.top.px]="popY(d.sy)" (pointerdown)="$event.stopPropagation()">
        <textarea #nb class="co-input" rows="3" maxlength="5000" [placeholder]="'co.placeholder' | t" autofocus
                  (keydown)="submitKey($event, () => postDraft(nb))"></textarea>
        <div class="co-actions">
          <button class="co-btn" (click)="collab.draft.set(null)">{{ 'co.cancel' | t }}</button>
          <button class="co-btn primary co-submit" (click)="postDraft(nb)">{{ 'co.post' | t }}</button>
        </div>
      </div>
    }

    @if (openThreadData(); as t) {
      @if (pins()[t.root.id]; as p) {
        <div class="co-pop co-thread" [style.left.px]="popX(p.x + 18)" [style.top.px]="popY(p.y - 8)" (pointerdown)="$event.stopPropagation()">
          <header>
            <b>#{{ t.n }}</b>
            <span class="grow"></span>
            @if (t.root.resolved) { <button class="co-btn co-reopen" (click)="collab.resolve(t.root, false)">↺ {{ 'co.reopen' | t }}</button> }
            @else { <button class="co-btn co-resolve" (click)="collab.resolve(t.root, true)">✓ {{ 'co.resolve' | t }}</button> }
            <button class="co-btn" (click)="collab.openThread.set(null)" [title]="'co.close' | t">✕</button>
          </header>
          @if (p.detached) { <p class="co-muted">{{ 'co.detached' | t }}</p> }
          <div class="co-msgs">
            @for (c of threadMessages(t); track c.id) {
              <div class="co-msg" [attr.data-msg-id]="c.id">
                <div class="co-msg-head">
                  <span class="co-av" [style.background]="color(c.authorId)">
                    @if (c.authorAvatar) { <img [src]="c.authorAvatar" alt="" /> } @else { {{ initialsOf(c.authorName) }} }
                  </span>
                  <b>{{ c.authorName || '—' }}</b>
                  <small>{{ i18n.date(c.createdAt, true) }}</small>
                  <span class="grow"></span>
                  @if (collab.canChange(c)) { <button class="co-link" (click)="editing.set(c.id)">{{ 'co.edit' | t }}</button> }
                  @if (collab.canDelete(c)) { <button class="co-link danger" (click)="remove(c)">{{ 'co.delete' | t }}</button> }
                </div>
                @if (editing() === c.id) {
                  <textarea #eb class="co-input" rows="2" maxlength="5000" [value]="c.body" (keydown)="submitKey($event, () => saveEdit(c, eb))"></textarea>
                  <div class="co-actions">
                    <button class="co-btn" (click)="editing.set(null)">{{ 'co.cancel' | t }}</button>
                    <button class="co-btn primary" (click)="saveEdit(c, eb)">{{ 'co.save' | t }}</button>
                  </div>
                } @else {
                  <p class="co-body">{{ c.body }}</p>
                }
              </div>
            }
          </div>
          <textarea #rb class="co-input co-reply" rows="2" maxlength="5000" [placeholder]="'co.replyPlaceholder' | t"
                    (keydown)="replyKey($event, t.root.id, rb)"></textarea>
          <div class="co-actions"><button class="co-btn primary co-reply-btn" (click)="postReply(t, rb)">{{ 'co.reply' | t }}</button></div>
        </div>
      }
    }

    @if (collab.panelOpen()) {
      <aside class="co-panel" (pointerdown)="$event.stopPropagation()">
        <header>
          <b>💬 {{ 'co.panel' | t }}</b>
          <span class="grow"></span>
          <button class="co-btn" (click)="collab.panelOpen.set(false)" [title]="'co.close' | t">✕</button>
        </header>
        <div class="co-filter">
          <button [class.on]="filter() === 'open'" (click)="filter.set('open')">{{ 'co.open' | t }} ({{ collab.openCount() }})</button>
          <button [class.on]="filter() === 'resolved'" (click)="filter.set('resolved')">{{ 'co.resolvedTab' | t }}</button>
          <button [class.on]="filter() === 'all'" (click)="filter.set('all')">{{ 'co.all' | t }}</button>
        </div>
        <ul>
          @for (t of listThreads(); track t.root.id) {
            <li [class.on]="collab.openThread() === t.root.id" [class.resolved]="t.root.resolved" (click)="focus(t)" [attr.data-thread-id]="t.root.id">
              <div class="co-msg-head">
                <span class="co-av" [style.background]="color(t.root.authorId)">{{ initialsOf(t.root.authorName) }}</span>
                <b>#{{ t.n }} {{ t.root.authorName }}</b>
                <span class="grow"></span>
                <small>{{ i18n.date(t.root.createdAt, true) }}</small>
              </div>
              <p class="co-body clamp">{{ t.root.body }}</p>
              <small class="co-muted">{{ layerName(t.root) }}@if (t.replies.length) { · {{ 'co.replies' | t: { n: t.replies.length } }} }@if (t.root.resolved) { · ✓ }</small>
            </li>
          } @empty { <li class="co-muted co-empty">{{ 'co.empty' | t }}</li> }
        </ul>
      </aside>
    }
  `,
  styles: `
    .co-layer { position: absolute; inset: 0; pointer-events: none; overflow: hidden; z-index: 8; /* above the canvas selection overlays (.ov ≤ 7) */ }
    .co-layer > * { pointer-events: auto; }
    .co-layer .co-peer-box { position: absolute; border: 2px solid; border-radius: 2px; pointer-events: none; box-sizing: border-box; }
    .co-layer .co-peer-box span { position: absolute; top: -18px; left: -2px; color: #fff; font-size: 10.5px; padding: 1px 6px; border-radius: 4px 4px 4px 0; white-space: nowrap; }
    .co-layer .co-hint, .co-layer .co-stale {
      position: absolute; bottom: 64px; left: 50%; transform: translateX(-50%); background: #1e1e1e; color: #fff; font-size: 12px;
      padding: 6px 12px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,.2); white-space: nowrap; pointer-events: none;
    }
    .co-layer .co-stale { pointer-events: auto; bottom: auto; top: 52px; background: #fff7e0; color: #5c4400; border: 1px solid #f0c64b; display: flex; gap: 8px; align-items: center; }
    .co-layer .with-panel { left: calc((100% - 300px) / 2); }
    .co-layer .co-pin {
      position: absolute; width: 28px; height: 28px; margin: -28px 0 0 0; border: 2px solid #fff; padding: 0; cursor: pointer;
      border-radius: 50% 50% 50% 0; background: var(--fg-blue, #0d99ff); color: #fff; font-size: 10.5px; font-weight: 700;
      box-shadow: 0 2px 6px rgba(0,0,0,.3); display: grid; place-items: center;
    }
    .co-layer .co-pin.resolved { background: #9e9e9e; opacity: .75; }
    .co-layer .co-pin.detached { background: #f24822; }
    .co-layer .co-pin.active { outline: 3px solid rgba(13,153,255,.35); transform: scale(1.1); }
    .co-layer .co-pop {
      position: absolute; width: 290px; background: #fff; border-radius: 10px; box-shadow: 0 6px 24px rgba(0,0,0,.2), 0 0 0 1px rgba(0,0,0,.05);
      padding: 10px; font-size: 12px; color: var(--fg-text, #1e1e1e); z-index: 6; box-sizing: border-box;
    }
    .co-layer .co-thread header, .co-layer .co-panel header { display: flex; align-items: center; gap: 6px; margin-bottom: 6px; }
    .co-layer .grow { flex: 1; }
    .co-layer .co-msgs { max-height: 260px; overflow: auto; }
    .co-layer .co-msg { padding: 6px 0; border-bottom: 1px solid #f0f0f0; }
    .co-layer .co-msg-head { display: flex; align-items: center; gap: 6px; }
    .co-layer .co-msg-head small { color: var(--fg-muted, #757575); }
    .co-layer .co-av { width: 20px; height: 20px; border-radius: 50%; color: #fff; display: inline-grid; place-items: center; font-size: 9px; font-weight: 700; overflow: hidden; flex: none; }
    .co-layer .co-av img { width: 100%; height: 100%; object-fit: cover; }
    .co-layer .co-body { margin: 4px 0 0 26px; white-space: pre-wrap; word-break: break-word; }
    .co-layer .co-body.clamp { display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; margin-left: 0; }
    .co-layer .co-input { width: 100%; box-sizing: border-box; border: 1px solid var(--fg-border, #e6e6e6); border-radius: 6px; padding: 6px; font: inherit; resize: vertical; margin-top: 6px; }
    .co-layer .co-actions { display: flex; justify-content: flex-end; gap: 6px; margin-top: 6px; }
    .co-layer .co-btn { border: 1px solid var(--fg-border, #e6e6e6); background: #fff; border-radius: 6px; padding: 3px 8px; cursor: pointer; font-size: 11.5px; color: inherit; }
    .co-layer .co-btn.primary { background: var(--fg-blue, #0d99ff); border-color: var(--fg-blue, #0d99ff); color: #fff; }
    .co-layer .co-link { border: 0; background: none; color: var(--fg-muted, #757575); cursor: pointer; font-size: 11px; padding: 0 2px; }
    .co-layer .co-link.danger:hover { color: #f24822; }
    .co-layer .co-muted { color: var(--fg-muted, #757575); }
    .co-layer .co-panel {
      position: absolute; top: 0; right: 0; bottom: 0; width: 300px; background: #fff; border-left: 1px solid var(--fg-border, #e6e6e6);
      display: flex; flex-direction: column; font-size: 12px; z-index: 5; padding: 10px; box-sizing: border-box;
    }
    .co-layer .co-panel ul { list-style: none; margin: 0; padding: 0; overflow: auto; flex: 1; }
    .co-layer .co-panel li { padding: 8px; border-radius: 8px; cursor: pointer; border: 1px solid transparent; margin-bottom: 4px; }
    .co-layer .co-panel li:hover { background: var(--fg-hover, #f5f5f5); }
    .co-layer .co-panel li.on { border-color: var(--fg-blue, #0d99ff); background: var(--fg-blue-soft, #e5f4ff); }
    .co-layer .co-panel li.resolved { opacity: .65; }
    .co-layer .co-panel li.co-empty { cursor: default; }
    .co-layer .co-filter { display: flex; gap: 4px; margin-bottom: 8px; }
    .co-layer .co-filter button { border: 0; background: var(--fg-hover, #f5f5f5); border-radius: 6px; padding: 4px 8px; cursor: pointer; font-size: 11.5px; }
    .co-layer .co-filter button.on { background: var(--fg-blue, #0d99ff); color: #fff; }
    .fg-canvas.co-mode .viewport, .fg-canvas.co-mode .viewport [data-node-id] { cursor: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='24' height='24'%3E%3Cpath d='M3 21V9a6 6 0 0 1 6-6h6a6 6 0 0 1 0 12H9z' fill='%230d99ff' stroke='white' stroke-width='1.5'/%3E%3C/svg%3E") 2 22, crosshair !important; }
    .fg-toolbar .co-badge { position: absolute; top: 1px; right: 1px; min-width: 14px; height: 14px; border-radius: 7px; background: #f24822; color: #fff; font-size: 9px; line-height: 14px; padding: 0 3px; box-sizing: border-box; pointer-events: none; }
    .fg-toolbar .co-tool { position: relative; }
  `,
})
export class CommentsLayer {
  readonly collab = inject(CollabStore);
  readonly i18n = inject(I18n);
  private host = inject(ElementRef<HTMLElement>).nativeElement as HTMLElement;
  private store = this.collab.store;

  readonly filter = signal<'open' | 'resolved' | 'all'>('open');
  readonly editing = signal<number | null>(null);
  readonly pins = signal<Record<number, Pt>>({});
  readonly peerBoxes = signal<Box[]>([]);

  readonly visibleThreads = computed(() => {
    const open = this.collab.openThread();
    // resolved pins stay hidden on the canvas unless picked from the panel
    return this.collab.threads().filter(t => !t.root.resolved || t.root.id === open || this.filter() === 'resolved');
  });
  readonly listThreads = computed(() => {
    const f = this.filter();
    return this.collab.threads().filter(t => f === 'all' || (f === 'resolved') === t.root.resolved).reverse();
  });
  readonly openThreadData = computed(() => this.collab.threads().find(t => t.root.id === this.collab.openThread()) ?? null);

  readonly initialsOf = initials;
  readonly color = peerColor;

  constructor() {
    const destroy = inject(DestroyRef);
    // comment mode: a press on the canvas drops a pin instead of selecting (window capture runs before the canvas)
    const down = (e: PointerEvent) => this.canvasDown(e);
    const swallow = (e: MouseEvent) => { if (this.collab.mode() && this.inViewport(e.target)) { e.preventDefault(); e.stopPropagation(); } };
    window.addEventListener('pointerdown', down, true);
    window.addEventListener('click', swallow, true);
    window.addEventListener('dblclick', swallow, true);
    // Pins and peer boxes follow the layers every frame — but only while there is something to
    // place, and not while the tab is hidden.
    let raf = 0;
    const loop = () => { this.measure(); raf = requestAnimationFrame(loop); };
    const ready = signal(false);
    const tabVisible = signal(!document.hidden);
    const onVisibility = () => tabVisible.set(!document.hidden);
    document.addEventListener('visibilitychange', onVisibility);
    afterNextRender(() => ready.set(true));
    const needed = computed(() => this.collab.threads().length > 0 || this.collab.peers().some(p => !!p.selectedId));
    effect(() => {
      const run = ready() && tabVisible() && needed();
      const any = needed();
      untracked(() => {
        if (run && !raf) raf = requestAnimationFrame(loop);
        if (!run && raf) { cancelAnimationFrame(raf); raf = 0; }
        if (!any) {
          if (Object.keys(this.pins()).length) this.pins.set({});
          if (this.peerBoxes().length) this.peerBoxes.set([]);
        }
      });
    });
    destroy.onDestroy(() => {
      cancelAnimationFrame(raf);
      document.removeEventListener('visibilitychange', onVisibility);
      window.removeEventListener('pointerdown', down, true);
      window.removeEventListener('click', swallow, true);
      window.removeEventListener('dblclick', swallow, true);
    });
    effect(() => this.host.parentElement?.classList.toggle('co-mode', this.collab.mode()));
  }

  private get canvasEl() { return this.host.parentElement; }
  private viewport() { return this.canvasEl?.querySelector<HTMLElement>('.viewport') ?? null; }
  private artboard() { return this.canvasEl?.querySelector<HTMLElement>('.artboard') ?? null; }
  private inViewport(t: EventTarget | null) {
    const vp = this.viewport();
    return !!vp && t instanceof Node && vp.contains(t) && !this.host.contains(t);
  }
  private nodeEl(id: string) {
    return this.artboard()?.querySelector<HTMLElement>(`[data-node-id="${CSS.escape(id)}"]`) ?? null;
  }

  private canvasDown(e: PointerEvent) {
    if (!this.collab.mode() || e.button !== 0 || this.store.spaceDown() || !this.inViewport(e.target)) return;
    e.preventDefault();
    e.stopPropagation();
    const art = this.artboard();
    if (!art) return;
    const el = (e.target as HTMLElement).closest<HTMLElement>('[data-node-id]');
    const anchor = el && art.contains(el) ? el : art;
    const r = anchor.getBoundingClientRect(), h = this.host.getBoundingClientRect(), z = this.store.zoom() || 1;
    this.collab.openThread.set(null);
    this.collab.draft.set({
      nodeId: anchor === art ? null : anchor.dataset['nodeId'] ?? null,
      x: Math.round((e.clientX - r.left) / z), y: Math.round((e.clientY - r.top) / z),
      sx: e.clientX - h.left, sy: e.clientY - h.top,
    });
    requestAnimationFrame(() => this.host.querySelector<HTMLTextAreaElement>('.co-pop textarea')?.focus());
  }

  /** Recomputes pin and peer-selection positions (each frame; only changes trigger a render). */
  private measure() {
    const art = this.artboard();
    if (!art) return;
    const h = this.host.getBoundingClientRect(), a = art.getBoundingClientRect(), z = this.store.zoom() || 1;
    const pins: Record<number, Pt> = {};
    for (const t of this.collab.threads()) {
      const c = t.root;
      const el = c.nodeId ? this.nodeEl(c.nodeId) : null;
      const visible = el && el.getClientRects().length > 0;
      if (visible) {
        const r = el.getBoundingClientRect();
        pins[c.id] = { x: r.left - h.left + Math.min(c.x * z, r.width), y: r.top - h.top + Math.min(c.y * z, r.height) };
      } else if (!c.nodeId) {
        pins[c.id] = { x: a.left - h.left + c.x * z, y: a.top - h.top + c.y * z };
      } else {
        pins[c.id] = { x: a.left - h.left + 8, y: a.top - h.top + 30, detached: true };
      }
    }
    const boxes: Box[] = [];
    for (const p of this.collab.peers()) {
      const el = p.selectedId ? this.nodeEl(p.selectedId) : null;
      if (!el || el === art.firstElementChild) continue;
      const r = el.getBoundingClientRect();
      if (!r.width && !r.height) continue;
      const layer = findNode(this.store.doc(), p.selectedId!)?.node.name ?? '';
      boxes.push({ x: r.left - h.left, y: r.top - h.top, w: r.width, h: r.height, color: peerColor(p.id), label: layer ? `${p.name} · ${layer}` : p.name });
    }
    if (JSON.stringify(pins) !== JSON.stringify(this.pins())) this.pins.set(pins);
    if (JSON.stringify(boxes) !== JSON.stringify(this.peerBoxes())) this.peerBoxes.set(boxes);
  }

  // keep popovers inside the canvas
  popX(x: number) { return Math.max(8, Math.min(x, this.host.clientWidth - 300 - (this.collab.panelOpen() ? 300 : 0))); }
  popY(y: number) { return Math.max(8, Math.min(y, this.host.clientHeight - 240)); }

  threadMessages(t: Thread) { return [t.root, ...t.replies]; }

  layerName(c: DesignComment) {
    if (!c.nodeId) return tr('co.page');
    return findNode(this.store.doc(), c.nodeId)?.node.name || tr('co.detached');
  }

  submitKey(e: KeyboardEvent, fn: () => void) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); fn(); }
    if (e.key === 'Escape') { this.collab.draft.set(null); this.editing.set(null); }
  }

  async postDraft(ta: HTMLTextAreaElement) {
    const d = this.collab.draft();
    const body = ta.value.trim();
    if (!d || !body) return;
    const c = await this.collab.create(body, d.nodeId, d.x, d.y);
    if (c) { this.collab.draft.set(null); this.collab.openThread.set(c.id); }
  }

  replyKey(e: KeyboardEvent, rootId: number, ta: HTMLTextAreaElement) {
    this.submitKey(e, () => this.reply(rootId, ta));
  }

  postReply(t: Thread, ta: HTMLTextAreaElement) { return this.reply(t.root.id, ta); }

  private async reply(rootId: number, ta: HTMLTextAreaElement) {
    const body = ta.value.trim();
    if (body && (await this.collab.reply(rootId, body))) ta.value = '';
  }

  async saveEdit(c: DesignComment, ta: HTMLTextAreaElement) {
    const body = ta.value.trim();
    if (body && (await this.collab.edit(c.id, body))) this.editing.set(null);
  }

  async remove(c: DesignComment) {
    const ok = await Dialog.confirm({ title: tr('co.deleteConfirm'), text: c.parentId ? undefined : tr('co.deleteThread'), danger: true, confirm: tr('co.delete') });
    if (ok) await this.collab.remove(c);
  }

  togglePin(t: Thread) {
    this.collab.draft.set(null);
    this.collab.openThread.set(this.collab.openThread() === t.root.id ? null : t.root.id);
  }

  /** Panel click: select the layer, bring it into view and open the thread. */
  focus(t: Thread) {
    const id = t.root.nodeId;
    if (id && findNode(this.store.doc(), id)) {
      this.store.select(id);
      this.nodeEl(id)?.scrollIntoView({ block: 'center', inline: 'center' });
    }
    if (t.root.resolved && this.filter() === 'open') this.filter.set('all');
    this.collab.draft.set(null);
    this.collab.openThread.set(t.root.id);
  }

  async reloadStale() {
    if (this.store.dirty() && !(await Dialog.confirm({ title: tr('pr.reloadDirty'), danger: true, confirm: tr('pr.reload') }))) return;
    await this.collab.reload();
  }

}
