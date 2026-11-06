import { ChangeDetectionStrategy, Component, ViewEncapsulation, computed, inject } from '@angular/core';
import { I18n, TPipe, tr } from '../core/i18n';
import { CollabStore, Peer, initials, peerColor } from './collab';
import { findNode } from './design-types';
import './messages-collab';

/** Avatar chips of the other people who have this design open (editor header). */
@Component({
  selector: 'rea-presence-chips',
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  host: { class: 'pr-chips' },
  template: `
    @for (p of shown(); track p.id) {
      <span class="pr-chip" [style.background]="color(p.id)" [style.border-color]="color(p.id)" [title]="tip(p)" [attr.data-user-id]="p.id">
        @if (p.avatar) { <img [src]="p.avatar" alt="" /> } @else { {{ initialsOf(p.name) }} }
      </span>
    }
    @if (more() > 0) { <span class="pr-chip more" [title]="moreNames()">+{{ more() }}</span> }
  `,
  styles: `
    .pr-chips { display: inline-flex; align-items: center; margin-right: 4px; }
    .pr-chips .pr-chip {
      width: 24px; height: 24px; border-radius: 50%; border: 2px solid; color: #fff; font-size: 9.5px; font-weight: 700;
      display: inline-grid; place-items: center; overflow: hidden; margin-left: -6px; box-shadow: 0 0 0 2px #fff; cursor: default;
    }
    .pr-chips .pr-chip:first-child { margin-left: 0; }
    .pr-chips .pr-chip img { width: 100%; height: 100%; object-fit: cover; }
    .pr-chips .pr-chip.more { background: #757575; border-color: #757575; }
  `,
})
export class PresenceChips {
  private collab = inject(CollabStore);
  readonly shown = computed(() => this.collab.peers().slice(0, 4));
  readonly more = computed(() => Math.max(0, this.collab.peers().length - 4));
  readonly moreNames = computed(() => this.collab.peers().slice(4).map(p => p.name).join(', '));
  readonly color = peerColor;
  readonly initialsOf = initials;

  tip(p: Peer) {
    const layer = p.selectedId ? findNode(this.collab.store.doc(), p.selectedId)?.node.name : '';
    return layer ? tr('pr.selecting', { name: p.name, layer }) : tr('pr.here', { name: p.name });
  }
}

/** Toolbar button for comment mode, with the open-thread count. */
@Component({
  selector: 'rea-comment-tool',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <button class="co-tool" [class.on]="collab.mode()" [title]="'co.tool' | t" (click)="collab.toggleMode()">💬
      @if (collab.openCount()) { <span class="co-badge">{{ collab.openCount() }}</span> }
    </button>
  `,
  host: { style: 'display: contents' },
})
export class CommentTool {
  readonly collab = inject(CollabStore);
}

/** Shown when a save was refused because someone else saved a newer version. */
@Component({
  selector: 'rea-save-conflict',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (collab.store.conflict(); as c) {
      <div class="modal-backdrop" (click)="cancel()">
        <div class="modal cf-modal" role="alertdialog" (click)="$event.stopPropagation()">
          <!-- 2.3.0: the same person in another tab is not "someone else" -->
          <header class="modal-head"><h3>⚠ {{ (c.byId && c.byId === collab.me()?.id ? 'cf.titleSelf' : 'cf.title') | t }}</h3><button class="btn btn-ghost btn-sm" (click)="cancel()">✕</button></header>
          <p>{{ 'cf.text' | t: { name: c.by || ('pr.someone' | t), time: time(c.at) } }}</p>
          <div class="cf-choices">
            <button class="btn cf-reload" (click)="reload()"><b>↺ {{ 'cf.reload' | t }}</b><small>{{ 'cf.reloadHint' | t }}</small></button>
            <button class="btn btn-primary cf-overwrite" (click)="overwrite()"><b>⤓ {{ 'cf.overwrite' | t }}</b><small>{{ 'cf.overwriteHint' | t }}</small></button>
            <button class="btn btn-ghost cf-cancel" (click)="cancel()">{{ 'cf.cancel' | t }}</button>
          </div>
        </div>
      </div>
    }
  `,
  styles: `
    .cf-modal { max-width: 440px; }
    .cf-choices { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
    .cf-choices .btn { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; text-align: left; }
    .cf-choices small { font-weight: 400; opacity: .8; }
  `,
})
export class SaveConflictDialog {
  readonly collab = inject(CollabStore);
  private i18n = inject(I18n);

  time(at: string) {
    if (!at) return '';
    const d = new Date(at);
    const sameDay = d.toDateString() === new Date().toDateString();
    return sameDay ? d.toLocaleTimeString(this.i18n.intl(), { hour: '2-digit', minute: '2-digit' }) : this.i18n.date(at, true);
  }

  cancel() { this.collab.store.conflict.set(null); }

  reload() { this.collab.reload(); }

  async overwrite() {
    const c = this.collab.store.conflict();
    this.collab.store.conflict.set(null);
    try { await this.collab.store.save(c?.status, { force: true }); } catch { /* toast shown */ }
  }
}
