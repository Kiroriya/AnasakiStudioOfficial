import { ChangeDetectionStrategy, Component, ElementRef, Injectable, computed, effect, inject, input, signal, untracked, viewChild } from '@angular/core';
import { DNode, NODE_LABEL, effective, findNode, isAncestor, isContainer, layerIcon, pathTo } from './design-types';
import { TPipe, tr } from '../core/i18n';
import { EditorStore } from './editor-store';
import { selectionRelatives } from './editor-utils';
import './messages-editor4';

type DropPos = 'before' | 'after' | 'inside';

/** State shared by the rows of one layer tree (provided by each layers panel, so editors don't share it). */
@Injectable()
export class LayerTreeState {
  private store = inject(EditorStore);
  /** Layer being dragged in the tree. */
  readonly drag = signal<{ id: string } | null>(null);
  readonly dropTarget = signal<{ id: string; pos: DropPos } | null>(null);
  /** Anchor for Shift+click range selection. */
  rangeAnchor: string | null = null;
  /** "Collapse all / Expand all" requests (version bumps so the same one can repeat). */
  readonly expandCmd = signal<{ open: boolean; v: number }>({ open: true, v: 0 });
  /** Layer search: ids to show (matches + their ancestors) and the matches; null = no search. */
  readonly filter = signal<{ show: Set<string>; hits: Set<string> } | null>(null);
  /** Layers containing a selected layer, and layers inside one — computed once for all rows. */
  readonly relatives = computed(() => selectionRelatives(this.store.doc(), this.store.selectedIds()));
}

@Component({
  selector: 'rea-layer',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (visibleInSearch()) {
    <div class="layer-row"
         [class.selected]="selected()"
         [class.in-selected]="inSelected()"
         [class.hovered]="store.hoverId() === node().id"
         [class.hidden-layer]="hidden()"
         [class.locked]="!!node().locked"
         [class.inst]="node().type === 'instance'"
         [class.drop-before]="dropHere() === 'before'"
         [class.drop-after]="dropHere() === 'after'"
         [class.drop-inside]="dropHere() === 'inside'"
         [class.search-hit]="isHit()"
         [style.padding-left.px]="6 + depth() * 12"
         [attr.draggable]="depth() > 0 && !renaming()"
         (click)="click($event)"
         (dblclick)="startRename()"
         (contextmenu)="menu($event)"
         (mouseenter)="store.hoverId.set(node().id)"
         (dragstart)="dragStart($event)" (dragover)="dragOver($event)" (dragleave)="tree.dropTarget.set(null)"
         (drop)="drop($event)" (dragend)="dragEnd()">
      @if (node().children?.length) {
        <button class="twisty" (click)="$event.stopPropagation(); open.set(!open())">{{ open() ? '▾' : '▸' }}</button>
      } @else { <span class="twisty"></span> }
      <span class="layer-icon">{{ icon() }}</span>
      @if (renaming()) {
        <input #renameBox class="layer-rename" [value]="node().name || label()" (keydown)="renameKey($event)" (blur)="finishRename()" (click)="$event.stopPropagation()" />
      } @else {
        <span class="layer-name" [title]="node().name || label()">{{ node().name || label() }}</span>
      }
      @if (bpNote(); as note) { <span class="bp-dot" [title]="note">●</span> }
      @if (node().absolute) { <span class="abs-badge" [title]="'ed.layer.absolute' | t">⌖</span> }
      @if (node().props?.ref; as ref) { <span class="ref-badge" [title]="'ed.ref.badge' | t: { name: ref }">&lt;/&gt;</span> }
      @if (depth() > 0) {
        <button class="row-btn lock" [class.on]="!!node().locked" [title]="(node().locked ? 'ed.layer.unlock' : 'ed.layer.lock') | t"
                (click)="$event.stopPropagation(); store.toggleLock([node().id])">{{ node().locked ? '🔒' : '🔓' }}</button>
        <button class="row-btn eye" [class.on]="hidden()" [title]="(hidden() ? 'ed.layer.show' : 'ed.layer.hide') | t"
                (click)="$event.stopPropagation(); store.toggleVisible([node().id])">{{ hidden() ? '◌' : '👁' }}</button>
      }
    </div>
    @if (open() || searching()) {
      @for (c of node().children ?? []; track c.id) {
        <rea-layer [node]="c" [depth]="depth() + 1" />
      }
    }
    }
  `,
})
export class LayerItem {
  readonly store = inject(EditorStore);
  readonly tree = inject(LayerTreeState);
  readonly node = input.required<DNode>();
  readonly depth = input(0);
  readonly open = signal(true);
  private renameBox = viewChild<ElementRef<HTMLInputElement>>('renameBox');

  readonly selected = computed(() => this.store.selectedIds().includes(this.node().id));
  readonly inSelected = computed(() => !this.selected() && this.tree.relatives().inside.has(this.node().id));
  /** Contains a selected layer (it opens to reveal it). */
  private readonly holdsSelection = computed(() => this.tree.relatives().ancestors.has(this.node().id));
  readonly hidden = computed(() => effective(this.node(), this.store.bp()).visible === false);
  readonly icon = computed(() => layerIcon(this.node()));
  readonly label = computed(() => NODE_LABEL[this.node().type]);
  readonly renaming = computed(() => this.store.renamingId() === this.node().id);
  readonly searching = computed(() => !!this.tree.filter());
  readonly visibleInSearch = computed(() => { const f = this.tree.filter(); return !f || f.show.has(this.node().id); });
  readonly isHit = computed(() => !!this.tree.filter()?.hits.has(this.node().id));
  /** Dot = this layer is set differently on tablet/mobile (at the current screen when not on desktop). */
  readonly bpNote = computed(() => {
    const r = this.node().responsive;
    if (!r) return '';
    const bp = this.store.bp();
    if (bp !== 'desktop') return r[bp] ? tr('ed.layer.responsiveHere') : '';
    const where = (['tablet', 'mobile'] as const).filter(b => r[b]).map(b => tr('ed.bp.' + b)).join(', ');
    return where ? tr('ed.layer.responsiveOn', { bp: where }) : '';
  });
  readonly dropHere = computed(() => {
    const t = this.tree.dropTarget();
    return t && t.id === this.node().id ? t.pos : null;
  });

  constructor() {
    // auto-expand ancestors of the selection (Figma reveals selected layers)
    effect(() => {
      if (this.holdsSelection()) this.open.set(true);
    });
    effect(() => {
      if (this.renaming()) queueMicrotask(() => { const el = this.renameBox()?.nativeElement; el?.focus(); el?.select(); });
    });
    // Collapse all keeps the page and its sections visible (sections closed); Expand all opens everything.
    let seen = this.tree.expandCmd().v;
    effect(() => {
      const c = this.tree.expandCmd();
      if (c.v === seen) return;
      seen = c.v;
      untracked(() => this.open.set(c.open || this.depth() === 0));
    });
  }

  click(e: MouseEvent) {
    const id = this.node().id, t = this.tree;
    if (e.shiftKey && t.rangeAnchor) {
      this.store.selectMany(rangeBetween(this.store.doc(), t.rangeAnchor, id));
    } else if (e.ctrlKey || e.metaKey) {
      this.store.toggleSelect(id);
      t.rangeAnchor = id;
    } else {
      this.store.select(id);
      t.rangeAnchor = id;
    }
  }

  menu(e: MouseEvent) {
    e.preventDefault();
    if (!this.selected()) this.store.select(this.node().id);
    this.store.contextMenu.set({ x: e.clientX, y: e.clientY });
  }

  startRename() { if (this.depth() > 0) this.store.renamingId.set(this.node().id); }

  renameKey(e: KeyboardEvent) {
    e.stopPropagation();
    if (e.key === 'Enter') (e.target as HTMLInputElement).blur();
    if (e.key === 'Escape') this.store.renamingId.set(null);
  }

  finishRename() {
    const v = this.renameBox()?.nativeElement.value.trim();
    if (this.renaming() && v && v !== this.node().name) this.store.rename(this.node().id, v);
    this.store.renamingId.set(null);
  }

  dragStart(e: DragEvent) {
    e.stopPropagation();
    this.tree.drag.set({ id: this.node().id });
    e.dataTransfer?.setData('text/plain', this.node().id);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
  }

  dragOver(e: DragEvent) {
    const d = this.tree.drag();
    const n = this.node();
    if (!d || d.id === n.id || isAncestor(this.store.doc(), d.id, n.id)) return;
    e.preventDefault();
    e.stopPropagation();
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const y = (e.clientY - r.top) / r.height;
    let pos: DropPos = y < 0.5 ? 'before' : 'after';
    if (isContainer(n) && n.type !== 'postList' && y > 0.25 && y < 0.75) pos = 'inside';
    if (this.depth() === 0) pos = 'inside';
    this.tree.dropTarget.set({ id: n.id, pos });
  }

  drop(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    const d = this.tree.drag(), t = this.tree.dropTarget();
    if (d && t) {
      if (t.pos === 'inside') {
        this.store.move(d.id, t.id, this.node().children?.length ?? 0);
      } else {
        const f = findNode(this.store.doc(), t.id);
        if (f?.parent) this.store.move(d.id, f.parent.id, f.index + (t.pos === 'after' ? 1 : 0));
      }
    }
    this.dragEnd();
  }

  dragEnd() {
    this.tree.drag.set(null);
    this.tree.dropTarget.set(null);
  }
}

/** Visible-order sibling ids between two layers (Shift+click in the panel). */
function rangeBetween(root: DNode, a: string, b: string): string[] {
  const flat: string[] = [];
  const walk = (n: DNode) => { flat.push(n.id); n.children?.forEach(walk); };
  walk(root);
  const i = flat.indexOf(a), j = flat.indexOf(b);
  if (i < 0 || j < 0) return [b];
  const pa = findNode(root, a)?.parent?.id;
  return flat.slice(Math.min(i, j), Math.max(i, j) + 1).filter(id => findNode(root, id)?.parent?.id === pa);
}

@Component({
  selector: 'rea-layers-panel',
  imports: [LayerItem, TPipe],
  providers: [LayerTreeState],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="layers-tools">
      <input class="layers-search" type="search" [placeholder]="'ed.layer.search' | t" [value]="query()" (input)="query.set(val($event))" (keydown.escape)="query.set('')" />
      <button class="icon-btn" [title]="'ed.layer.collapse' | t" (click)="expand(false)">⊟</button>
      <button class="icon-btn" [title]="'ed.layer.expand' | t" (click)="expand(true)">⊞</button>
    </div>
    @if (query() && !hitCount()) { <p class="hint pad">{{ 'ed.layer.noMatch' | t }}</p> }
    <div class="layers" (mouseleave)="store.hoverId.set(null)">
      <rea-layer [node]="store.doc()" [depth]="0" />
    </div>
  `,
})
export class LayersPanel {
  readonly store = inject(EditorStore);
  private tree = inject(LayerTreeState);
  readonly query = signal('');
  readonly hitCount = computed(() => this.tree.filter()?.hits.size ?? 0);

  constructor() {
    // name, type or text content matches; ancestors stay visible so the tree makes sense
    effect(() => {
      const q = this.query().trim().toLowerCase();
      if (!q) { this.tree.filter.set(null); return; }
      const root = this.store.doc();
      const hits = new Set<string>(), show = new Set<string>();
      const walk = (n: DNode) => {
        const hay = [n.name, NODE_LABEL[n.type], n.props?.text].filter(Boolean).join(' ').toLowerCase();
        if (hay.includes(q) && n.id !== root.id) {
          hits.add(n.id);
          for (const a of pathTo(root, n.id)) show.add(a.id);
        }
        n.children?.forEach(walk);
      };
      walk(root);
      show.add(root.id);
      this.tree.filter.set({ show, hits });
    });
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }

  expand(open: boolean) {
    this.query.set('');
    this.tree.expandCmd.update(c => ({ open, v: c.v + 1 }));
  }
}
