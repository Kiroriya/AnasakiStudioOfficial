import { ChangeDetectionStrategy, Component, DestroyRef, OnInit, computed, inject, signal, viewChild } from '@angular/core';
import { Dialog } from '../core/dialog';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { Api, SITE_ORIGIN, errorMessage } from '../core/api';
import { I18n, TPipe, tr } from '../core/i18n';
import { Media } from '../core/models';
import './messages-editor';
import { MediaPicker } from '../shared/media-picker';
import { CONTEXT_MENU, Command, CommandHost, buildCommands } from './commands';
import { BREAKPOINTS, Breakpoint, newId } from './design-types';
import { EditorCanvas } from './editor-canvas';
import { VariantBar } from './variants';
import { FindReplace } from './find-replace';
import { EditorStore, Tool } from './editor-store';
import { InsertPanel } from './insert-panel';
import { LayersPanel } from './layers-panel';
import { PropsPanel } from './props-panel';
import { TextToolbar } from './text-toolbar';
import { CodeData } from './code-data';
import { CodeBar } from './code-action';
import { CollabStore } from './collab';
import { CommentsLayer } from './comments';
import { CommentTool, PresenceChips, SaveConflictDialog } from './presence';
import { DNode } from './design-types';
import { Revision } from '../core/models';
import { vectorShortcut } from './vector-ops';

/** The Figma-like visual designer (UI3 layout) for pages, templates and components. */
@Component({
  selector: 'rea-editor',
  imports: [EditorCanvas, LayersPanel, PropsPanel, InsertPanel, RouterLink, MediaPicker, TPipe, TextToolbar, VariantBar, CommentsLayer, CommentTool, PresenceChips, SaveConflictDialog, FindReplace, CodeBar],
  providers: [EditorStore, CollabStore, CodeData],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    class: 'editor-root fg',
    '[style]': 'store.themeColorVars()',
    '(document:keydown)': 'keydown($event)',
    '(document:keyup)': 'keyup($event)',
    '(window:blur)': 'store.spaceDown.set(false); store.altDown.set(false)',
    '(window:beforeunload)': 'beforeUnload($event)',
  },
  template: `
    @if (error()) {
      <div class="ed-error">{{ error() }} <a routerLink="/">{{ 'ed.backHome' | t }}</a></div>
    } @else {
      <div class="fg-body" [style.--fg-left-w.px]="leftW()" [style.--fg-right-w.px]="rightW()">
        <!-- ================= left panel ================= -->
        <aside class="fg-left">
          <!-- 2.2.0: drag the edge to widen the panel (double-click = default width), remembered per browser -->
          <div class="fg-resize fg-resize-left" role="separator" aria-orientation="vertical" [attr.aria-label]="'ed.ui.resizeLeft' | t" [title]="'ed.ui.resizeTip' | t"
               (pointerdown)="startResize($event, 'left')" (dblclick)="setPanelW('left', 280)"></div>
          <div class="fg-left-head">
            <button class="fg-back" (click)="back()" [title]="'ed.backTitle' | t">←</button>
            <button class="fg-logo" [class.on]="mainMenu()" (click)="mainMenu.set(!mainMenu())" [title]="'ed.ui.mainMenu' | t">◆ <span>▾</span></button>
            <div class="fg-file">
              <input class="fg-title" [value]="store.title()" (change)="rename($event)" [title]="'ed.pageName' | t" />
              <small>{{ kindLabel() | t }} · {{ (store.kind() === 'post' ? (store.status() === 'publish' ? 'ed.ui.published' : 'ed.ui.draft') : 'ed.ui.mainComponent') | t }}
                @if (store.dirty()) { <b class="dirty" [title]="'ed.unsavedTitle' | t">● {{ 'ed.ui.unsaved' | t }}</b> }</small>
            </div>
          </div>
          <div class="fg-tabs">
            <button [class.on]="leftTab() === 'layers'" (click)="leftTab.set('layers')">{{ 'ed.ui.layers' | t }}</button>
            <button [class.on]="leftTab() === 'assets'" (click)="leftTab.set('assets')">{{ 'ed.ui.assets' | t }}</button>
          </div>
          <div class="panel-scroll">
            @if (leftTab() === 'layers') { <rea-layers-panel /> }
            @else { <rea-insert-panel (editComponent)="openComponent($event)" /> }
          </div>
        </aside>

        <!-- ================= canvas ================= -->
        <main class="fg-canvas">
          @if (templateInfo(); as ti) {
            <div class="tpl-banner">
              <span>🧩 <b>{{ 'ed.tpl.title' | t }}</b> — {{ ti.role | t }}</span>
              @if (ti.unused) {
                <button class="link" (click)="assignTemplate('post_template_id')">{{ 'ed.tpl.useSingle' | t }}</button>
                <button class="link" (click)="assignTemplate('archive_template_id')">{{ 'ed.tpl.useArchive' | t }}</button>
              }
              @if (store.samplePosts().length > 1) {
                <label>{{ 'ed.tpl.sample' | t }}
                  <select [value]="'' + store.sampleIndex()" (change)="store.sampleIndex.set(+$any($event.target).value)">
                    @for (sp of store.samplePosts(); track $index) { <option [value]="'' + $index">{{ sp.ctx['title'] }}</option> }
                  </select>
                </label>
              }
            </div>
          }
          @if (store.kind() === 'component') { <rea-variant-bar /> }
          <rea-canvas />
          <rea-text-toolbar />
          @if (findOpen()) { <rea-find-replace (closed)="findOpen.set(false)" /> }
          <rea-comments-layer />

          <rea-code-bar />
          <div class="fg-bp" [title]="'ed.bpTitle' | t">
            @for (b of breakpoints; track b.id) {
              <button [class.on]="store.bp() === b.id" (click)="setBp(b.id)">{{ b.icon }} {{ b.label }}</button>
            }
          </div>

          <div class="fg-toolbar">
            @for (t of tools; track t.id) {
              <button [class.on]="store.tool() === t.id" [title]="(t.title | t) + (t.keys ? ' (' + t.keys + ')' : '')" (click)="store.tool.set(t.id)">{{ t.icon }}</button>
            }
            <button [title]="('ed.cmd.image' | t) + ' (Ctrl+Shift+K)'" (click)="pickImage()">🖼</button>
            <rea-comment-tool />
            <span class="sep"></span>
            <button class="fg-free" [class.on]="store.freeDrag()" (click)="store.setFreeDrag(!store.freeDrag())"
                    [title]="(store.freeDrag() ? 'ed.freeOn' : 'ed.freeOff') | t">✥</button>
            <span class="sep"></span>
            <button [title]="('ed.cmd.undo' | t) + ' (Ctrl+Z)'" [disabled]="!store.canUndo()" (click)="store.undo()">↶</button>
            <button [title]="('ed.cmd.redo' | t) + ' (Ctrl+Shift+Z)'" [disabled]="!store.canRedo()" (click)="store.redo()">↷</button>
          </div>
        </main>

        <!-- ================= right panel ================= -->
        <aside class="fg-right">
          <div class="fg-resize fg-resize-right" role="separator" aria-orientation="vertical" [attr.aria-label]="'ed.ui.resizeRight' | t" [title]="'ed.ui.resizeTip' | t"
               (pointerdown)="startResize($event, 'right')" (dblclick)="setPanelW('right', 300)"></div>
          <div class="fg-right-head">
            <div class="fg-zoom">
              <button class="fg-zoom-btn" (click)="zoomMenu.set(!zoomMenu())" [title]="'ed.ui.zoom' | t">{{ zoomPct() }}% ▾</button>
              @if (zoomMenu()) {
                <div class="fg-dropdown right" (pointerdown)="$event.stopPropagation()">
                  @for (c of commands['View']; track c.id) {
                    <button [disabled]="c.enabled && !c.enabled()" (click)="run(c)"><span>@if (c.checked) {<b class="chk">{{ c.checked() ? '✓' : '' }}</b>}{{ c.label }}</span><kbd>{{ c.keys }}</kbd></button>
                  }
                </div>
              }
            </div>
            <span class="grow"></span>
            <rea-presence-chips />
            <button class="fg-icon-btn" [title]="'ed.hist.title' | t" (click)="openHistory()">🕘</button>
            @if (store.kind() === 'post') {
              <button class="fg-icon-btn" [title]="('ed.cmd.preview' | t) + ' (Ctrl+Alt+Enter)'" (click)="preview()">▶</button>
              @if (viewUrl()) { <a class="fg-icon-btn" [href]="viewUrl()" target="_blank" [title]="'ed.openLive' | t">↗</a> }
            }
            <button class="fg-btn" [disabled]="store.saving()" (click)="save()" [title]="('ed.cmd.save' | t) + ' (Ctrl+S)'">{{ (store.saving() ? 'ed.ui.saving' : 'ed.cmd.save') | t }}</button>
            @if (store.kind() === 'post') {
              <button class="fg-btn primary" [disabled]="store.saving()" (click)="save('publish')" [title]="('ed.cmd.publish' | t) + ' (Ctrl+Shift+P)'">{{ 'ed.cmd.publish' | t }}</button>
            }
          </div>
          <div class="panel-scroll"><rea-props-panel (editComponent)="openComponent($event)" (showShortcuts)="shortcuts.set(true)" (runPreview)="preview()" /></div>
        </aside>
      </div>

      <!-- main menu (Figma ◆ menu) -->
      @if (mainMenu()) {
        <div class="fg-dropdown main" (pointerdown)="$event.stopPropagation()">
          @for (g of menuGroups; track g) {
            <div class="fg-dd-group">
              <button class="fg-dd-head" (mouseenter)="subMenu.set(g)" (click)="subMenu.set(g)" [class.on]="subMenu() === g">{{ ('ed.menu.' + g) | t }} <span>▸</span></button>
              @if (subMenu() === g) {
                <div class="fg-dropdown sub">
                  @for (c of commands[g]; track c.id) {
                    <button [disabled]="c.enabled && !c.enabled()" (click)="run(c)"><span>@if (c.checked) {<b class="chk">{{ c.checked() ? '✓' : '' }}</b>}{{ c.label }}</span><kbd>{{ c.keys }}</kbd></button>
                  }
                </div>
              }
            </div>
          }
        </div>
      }

      <!-- right-click menu -->
      @if (store.contextMenu(); as cm) {
        <div class="fg-dropdown ctx" [style.left.px]="cm.x" [style.top.px]="cm.y" (pointerdown)="$event.stopPropagation()" (contextmenu)="$event.preventDefault()">
          @for (id of contextItems; track $index) {
            @if (id === '-') { <hr /> }
            @else if (cmd(id); as c) {
              @if (!c.enabled || c.enabled()) { <button (click)="run(c)"><span>@if (c.checked) {<b class="chk">{{ c.checked() ? '✓' : '' }}</b>}{{ c.label }}</span><kbd>{{ c.keys }}</kbd></button> }
            }
          }
        </div>
      }

      <!-- version history -->
      @if (history(); as h) {
        <div class="modal-backdrop" (click)="history.set(null)">
          <div class="modal fg-history" (click)="$event.stopPropagation()">
            <header class="modal-head"><h3>🕘 {{ 'ed.hist.title' | t }}</h3><button class="btn btn-ghost btn-sm" (click)="history.set(null)">✕</button></header>
            <p class="hint">{{ 'ed.hist.hint' | t }}</p>
            <ul class="hist-list">
              @for (r of h; track r.id; let i = $index) {
                <li>
                  <span><b>{{ i18n.date(r.createdAt, true) }}</b><small>{{ r.authorName || '—' }} · {{ r.title }}</small></span>
                  @if (store.kind() === 'post') { <button class="btn btn-sm" (click)="previewVersion(r)">▶ {{ 'ed.hist.preview' | t }}</button> }
                  <button class="btn btn-sm btn-primary" (click)="useVersion(r)">↺ {{ 'ed.hist.use' | t }}</button>
                </li>
              } @empty { <li class="muted">{{ 'ed.hist.empty' | t }}</li> }
            </ul>
          </div>
        </div>
      }

      <!-- keyboard shortcuts sheet -->
      @if (shortcuts()) {
        <div class="modal-backdrop" (click)="shortcuts.set(false)">
          <div class="modal modal-lg fg-shortcuts" (click)="$event.stopPropagation()">
            <header class="modal-head"><h3>{{ 'ed.cmd.shortcuts' | t }}</h3><button class="btn btn-ghost btn-sm" (click)="shortcuts.set(false)">✕</button></header>
            <p class="hint">{{ 'ed.scHint' | t }}</p>
            <div class="fg-sc-grid">
              @for (g of menuGroups; track g) {
                <div>
                  <h4>{{ ('ed.menu.' + g) | t }}</h4>
                  @for (c of commands[g]; track c.id) { @if (c.keys) { <div class="fg-sc"><span>{{ c.label }}</span><kbd>{{ c.keys }}</kbd></div> } }
                </div>
              }
              <div>
                <h4>{{ 'ed.menu.Canvas' | t }}</h4>
                @for (k of canvasKeys; track k[0]) { <div class="fg-sc"><span>{{ k[0] | t }}</span><kbd>{{ k[1] | t }}</kbd></div> }
              </div>
            </div>
          </div>
        </div>
      }
    }

    <rea-media-picker [open]="imagePicker()" (openChange)="imagePicker.set($event)" (picked)="placeImage($event)" />
    <rea-save-conflict />
  `,
})
export class Editor implements OnInit, CommandHost {
  readonly store = inject(EditorStore);
  /** Side panel widths (2.2.0): 280 / 300 by default, 220–480 by dragging their inner edge. */
  readonly leftW = signal(panelWidth('left', 280));
  readonly rightW = signal(panelWidth('right', 300));
  setPanelW(side: 'left' | 'right', w: number) {
    const v = Math.round(Math.min(480, Math.max(220, w)));
    (side === 'left' ? this.leftW : this.rightW).set(v);
    try { localStorage.setItem('anasaki.panel.' + side, String(v)); } catch { /* private mode */ }
  }
  startResize(e: PointerEvent, side: 'left' | 'right') {
    e.preventDefault();
    const x0 = e.clientX, w0 = side === 'left' ? this.leftW() : this.rightW();
    const move = (m: PointerEvent) => this.setPanelW(side, w0 + (side === 'left' ? m.clientX - x0 : x0 - m.clientX));
    const up = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); document.body.classList.remove('fg-resizing'); };
    document.body.classList.add('fg-resizing');
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
  }
  readonly collab = inject(CollabStore);
  private api = inject(Api);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  readonly canvas = viewChild(EditorCanvas);

  readonly breakpoints = BREAKPOINTS;
  readonly leftTab = signal<'layers' | 'assets'>('layers');
  readonly mainMenu = signal(false);
  readonly subMenu = signal<string | null>(null);
  readonly zoomMenu = signal(false);
  readonly shortcuts = signal(false);
  readonly imagePicker = signal(false);
  readonly error = signal('');
  readonly zoomPct = computed(() => Math.round(this.store.zoom() * 100));
  readonly i18n = inject(I18n);
  /** Earlier saved versions (null = dialog closed). */
  readonly history = signal<Revision[] | null>(null);

  /** For templates: what the template is used for, so it is clear how it differs from a page. */
  readonly templateInfo = computed(() => {
    const p = this.store.post();
    if (p?.type !== 'template') return null;
    const s = this.store.settings(), id = String(p.id);
    if (s['post_template_id'] === id) return { role: 'ed.tpl.single', unused: false };
    if (s['archive_template_id'] === id) return { role: 'ed.tpl.archive', unused: false };
    return { role: 'ed.tpl.unused', unused: true };
  });

  /** `title` = i18n key; `keys` is appended as typed. */
  readonly tools: { id: Tool; icon: string; title: string; keys?: string }[] = [
    { id: 'move', icon: '➤', title: 'ed.cmd.move', keys: 'V' },
    { id: 'hand', icon: '✋', title: 'ed.cmd.hand', keys: 'H / Space' },
    { id: 'frame', icon: '#', title: 'ed.tool.frame' },
    { id: 'rect', icon: '▭', title: 'ed.cmd.toolRect', keys: 'R' },
    { id: 'ellipse', icon: '◯', title: 'ed.cmd.toolEllipse', keys: 'O' },
    { id: 'text', icon: 'T', title: 'ed.tool.text' },
    { id: 'pen', icon: '🖋', title: 'ed.tool.pen' },
  ];
  readonly commands = buildCommands(this);
  readonly menuGroups = Object.keys(this.commands);
  readonly contextItems = CONTEXT_MENU;
  /** [what, keys] as i18n keys (sc.k.* keep key names like Ctrl / Shift readable in every language). */
  readonly canvasKeys: [string, string][] = [
    ['sc.click', 'sc.k.click'], ['sc.level', 'sc.k.ctrlClick'], ['sc.boxIn', 'sc.k.dragEmpty'], ['sc.boxDeep', 'sc.k.ctrlDrag'], ['sc.add', 'sc.k.shiftClick'],
    ['sc.inside', 'sc.k.dblclick'], ['sc.children', 'Enter'], ['sc.parent', 'Shift+Enter / Esc'],
    ['sc.sibling', 'Tab / Shift+Tab'], ['sc.nudge', 'sc.k.arrows'],
    ['sc.axis', 'sc.k.shiftDrag'], ['sc.dupDrag', 'sc.k.altDrag'], ['sc.noSnap', 'sc.k.ctrlDrag'],
    ['sc.measure', 'sc.k.holdAlt'], ['sc.ratio', 'sc.k.handle'],
    ['sc.scrub', 'sc.k.scrub'], ['sc.opacity', '1 … 0'], ['sc.zoom', 'sc.k.ctrlScroll'], ['sc.pan', 'sc.k.pan'],
  ];

  /** i18n key of what is being edited. */
  readonly kindLabel = computed(() => {
    if (this.store.kind() === 'component') return 'ed.ui.kind.component';
    return 'ed.ui.kind.' + (this.store.post()?.type ?? 'page');
  });

  /** Live address of a published page/post (the blog prefix follows the blog_base setting). */
  readonly viewUrl = computed(() => {
    const p = this.store.post();
    if (!p || p.status !== 'publish' || p.type === 'template') return '';
    return `${SITE_ORIGIN}${this.store.pathOf(p)}`;
  });

  constructor() {
    // capture phase: the canvas stops propagation, menus must still close
    const close = (e: PointerEvent) => this.closeMenus(e);
    document.addEventListener('pointerdown', close, true);
    inject(DestroyRef).onDestroy(() => document.removeEventListener('pointerdown', close, true));
  }

  async ngOnInit() {
    this.route.paramMap.subscribe(async pm => {
      const id = Number(pm.get('id'));
      const kind = this.route.snapshot.data['kind'] === 'component' ? 'component' : 'post';
      try {
        await this.store.load(kind, id);
        this.store.select(null);
        requestAnimationFrame(() => this.fit());
      } catch (e) {
        this.error.set(tr('ed.loadFail') + errorMessage(e));
      }
    });
  }

  // ---------------------------------------------------------------- CommandHost

  fit() { this.canvas()?.fit(); }
  zoomToSelection() { this.canvas()?.zoomToSelection(); }
  zoomBy(f: number) { this.canvas()?.zoomAt(f); }
  openShortcuts() { this.shortcuts.set(true); }
  pickImage() { this.imagePicker.set(true); }
  openFind() { this.findOpen.set(true); }
  /** Find & replace panel (Ctrl+F). */
  readonly findOpen = signal(false);
  async back() {
    if (this.store.dirty() && !(await Dialog.confirm({ title: tr('ed.leaveConfirm'), danger: true, confirm: tr('ed.leave') }))) return;
    this.store.dirty.set(false);
    this.router.navigateByUrl(this.store.kind() === 'component' ? '/components' : this.store.post()?.type === 'template' ? '/templates' : '/pages');
  }

  async createComponent() {
    const n = this.store.selected()?.node;
    if (!n) return;
    const name = await Dialog.prompt({ title: tr('ed.componentPrompt'), value: n.name || 'Component' });
    if (name) await this.store.createComponent(n.id, name);
  }

  async save(status?: 'draft' | 'publish') {
    try { await this.store.save(status); } catch { /* toast shown */ }
  }

  async preview() {
    const w = window.open('', '_blank');
    if (!w) return;
    try {
      const html = await this.api.preview(this.store.doc(), this.store.post()?.id, this.store.title());
      showSandboxedPreview(w, html, this.store.title());
    } catch (e) {
      w.close();
      this.store.notify(tr('ed.previewFail') + errorMessage(e), true);
    }
  }

  // ---------------------------------------------------------------- history & templates

  async openHistory() {
    try {
      const list = this.store.kind() === 'post'
        ? await this.api.revisions(this.store.post()!.id)
        : await this.api.componentRevisions(this.store.component()!.id);
      this.history.set(list);
    } catch (e) {
      this.store.notify(errorMessage(e), true);
    }
  }

  private async versionDesign(r: Revision): Promise<DNode | null> {
    const full = this.store.kind() === 'post'
      ? await this.api.revision(this.store.post()!.id, r.id)
      : await this.api.componentRevision(this.store.component()!.id, r.id);
    return full.design ?? null;
  }

  async previewVersion(r: Revision) {
    const w = window.open('', '_blank');
    if (!w) return;
    try {
      const design = await this.versionDesign(r);
      if (!design) { w.close(); this.store.notify(tr('ed.hist.noDesign'), true); return; }
      const html = await this.api.preview(design, this.store.post()?.id, r.title);
      showSandboxedPreview(w, html, r.title);
    } catch (e) {
      w.close();
      this.store.notify(errorMessage(e), true);
    }
  }

  /** Loads an earlier version into the canvas; Save keeps it, Ctrl+Z undoes it. */
  async useVersion(r: Revision) {
    try {
      const design = await this.versionDesign(r);
      if (!design) { this.store.notify(tr('ed.hist.noDesign'), true); return; }
      this.store.applyVersion(design);
      this.history.set(null);
      this.store.notify(tr('ed.hist.loaded'));
    } catch (e) {
      this.store.notify(errorMessage(e), true);
    }
  }

  async assignTemplate(key: 'post_template_id' | 'archive_template_id') {
    try {
      this.store.settings.set(await this.api.saveSettings({ [key]: String(this.store.post()!.id) }));
      this.store.notify(tr('ed.tpl.assigned'));
    } catch (e) {
      this.store.notify(errorMessage(e), true);
    }
  }

  // ---------------------------------------------------------------- UI

  cmd(id: string): Command | undefined {
    for (const list of Object.values(this.commands)) {
      const c = list.find(x => x.id === id);
      if (c) return c;
    }
    return undefined;
  }

  run(c: Command) {
    this.mainMenu.set(false);
    this.zoomMenu.set(false);
    this.store.contextMenu.set(null);
    if (!c.enabled || c.enabled()) c.run();
  }

  closeMenus(e: PointerEvent) {
    const t = e.target as HTMLElement;
    if (t.closest('.fg-dropdown, .fg-logo, .fg-zoom-btn')) return;
    this.mainMenu.set(false);
    this.zoomMenu.set(false);
    if (e.button !== 2) this.store.contextMenu.set(null);
  }

  rename(e: Event) {
    this.store.renamePage((e.target as HTMLInputElement).value);
  }

  setBp(bp: Breakpoint) {
    this.store.bp.set(bp);
    requestAnimationFrame(() => this.fit());
  }

  placeImage(m: Media) {
    const w = Math.min(480, m.width || 400);
    this.store.insert({
      id: newId(), type: 'image', name: m.filename.replace(/\.[^.]+$/, ''),
      width: { mode: 'fixed', value: w },
      height: { mode: 'fixed', value: Math.round(w * ((m.height || 300) / (m.width || 400))) },
      props: { src: m.url, alt: m.alt }, style: {},
    });
    this.imagePicker.set(false);
  }

  async openComponent(id: number) {
    if (this.store.dirty()) {
      if (!(await Dialog.confirm({ title: tr('ed.saveBeforeComponent'), confirm: tr('common.save') }))) return;
      await this.save();
    }
    this.router.navigate(['/design/component', id]);
  }

  beforeUnload(e: BeforeUnloadEvent) {
    if (this.store.dirty()) e.preventDefault();
  }

  // ---------------------------------------------------------------- keyboard (Figma map, by physical key)

  keyup(e: KeyboardEvent) {
    if (e.code === 'Space') this.store.spaceDown.set(false);
    if (e.key === 'Alt') { this.store.altDown.set(false); e.preventDefault(); }
  }

  /** Event time of the last key typed in a field / of the last single-key shortcut (keydown). */
  private lastTyped = -Infinity;
  private lastLetter = -Infinity;
  private typingWarned = false;

  keydown(e: KeyboardEvent) {
    const s = this.store;
    const mod = e.ctrlKey || e.metaKey;
    const code = e.code;
    const t = e.target as HTMLElement;
    const typing = t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName);

    if (mod && code === 'KeyS' && !e.altKey) { e.preventDefault(); this.save(); return; }
    if (e.key === 'Alt') { s.altDown.set(true); e.preventDefault(); return; } // measure works even from a field
    if (typing) {
      this.lastTyped = e.timeStamp;
      if (e.key === 'Escape') t.blur();
      return;
    }
    if (code === 'Space') { e.preventDefault(); if (!e.repeat) s.spaceDown.set(true); return; }

    const go = (fn: () => void) => { e.preventDefault(); s.contextMenu.set(null); fn(); };
    const id = s.selectedId();

    if (mod) {
      const vk = vectorShortcut(e); // boolean ops, flatten, export (vector-ops.ts)
      if (vk) return go(() => Object.values(this.commands).flat().find(c => c.id === vk)?.run());
      switch (code) {
        case 'KeyZ': return go(() => (e.shiftKey ? s.redo() : s.undo()));
        case 'KeyY': return go(() => s.redo());
        case 'KeyC': return go(() => (e.altKey && e.shiftKey ? void s.copyCss() : e.altKey ? s.copyProps() : s.copy()));
        case 'KeyX': return go(() => s.cut());
        case 'KeyV': return go(() => (e.altKey ? s.pasteProps() : s.paste(e.shiftKey)));
        case 'KeyD': return go(() => s.duplicateMany());
        case 'KeyA': return go(() => (e.altKey ? s.selectMatching() : s.selectAll()));
        case 'KeyF': return go(() => this.findOpen.set(true));
        case 'KeyG': return go(() => (e.shiftKey ? s.ungroup() : s.group(e.altKey ? 'Frame' : 'Group')));
        case 'KeyK': return go(() => (e.altKey ? this.createComponent() : e.shiftKey ? this.pickImage() : undefined));
        case 'KeyB': return go(() => { if (e.altKey && id) s.detachInstance(id); });
        case 'KeyR': return go(() => s.renamingId.set(id));
        case 'KeyH': return go(() => { if (e.shiftKey) s.toggleVisible(); });
        case 'KeyL': return go(() => { if (e.shiftKey) s.toggleLock(); });
        case 'KeyP': return go(() => { if (e.shiftKey) this.save('publish'); });
        case 'BracketRight': return go(() => s.reorder(e.shiftKey ? 'front' : 'forward'));
        case 'BracketLeft': return go(() => s.reorder(e.shiftKey ? 'back' : 'backward'));
        case 'Slash': return go(() => this.shortcuts.set(true));
        case 'Equal': case 'NumpadAdd': return go(() => this.zoomBy(1.25));
        case 'Minus': case 'NumpadSubtract': return go(() => this.zoomBy(0.8));
        case 'Digit0': return go(() => s.zoom.set(1));
        case 'Digit4': if (e.shiftKey) return go(() => this.commands['View'].find(c => c.id === 'layoutGrids')?.run()); return;
        case 'Enter': return go(() => { if (e.altKey) this.preview(); });
      }
      return;
    }

    if (e.altKey) {
      const map: Record<string, () => void> = {
        KeyA: () => (e.shiftKey ? s.removeAutoLayout() : s.align('left')), KeyD: () => s.align('right'),
        KeyW: () => s.align('top'), KeyS: () => s.align('bottom'),
        KeyH: () => (e.shiftKey ? s.distribute('h') : s.align('hcenter')), KeyV: () => (e.shiftKey ? s.distribute('v') : s.align('vcenter')),
        Digit1: () => this.setBp('desktop'), Digit2: () => this.setBp('tablet'), Digit3: () => this.setBp('mobile'),
      };
      if (map[code]) go(map[code]);
      return;
    }

    if (e.shiftKey) {
      switch (code) {
        case 'Digit0': return go(() => s.zoom.set(1));
        case 'Digit1': return go(() => this.fit());
        case 'Digit2': return go(() => this.zoomToSelection());
        case 'KeyA': return go(() => s.addAutoLayout());
        case 'KeyR': return go(() => this.commands['View'].find(c => c.id === 'rulers')?.run());
        case 'Enter': return go(() => s.selectParent());
        case 'Tab': return go(() => s.selectSibling(-1));
      }
    }

    if (code === 'F2' && id) return go(() => s.renamingId.set(id)); // rename, as in Figma / VS Code (Ctrl+R too)
    // Single-key shortcuts follow the physical key, so Thai letters trigger them too. Text typed just
    // after a field lost focus (or a fast burst of letters) is someone typing, not choosing tools (2.1.1).
    if (/^Key[A-Z]$/.test(code) && !e.shiftKey) { // digits stay free: 5 then 0 = 50% opacity
      const burst = e.timeStamp - this.lastLetter < 250;
      this.lastLetter = e.timeStamp;
      if (e.timeStamp - this.lastTyped < 1000 || burst) {
        this.lastTyped = e.timeStamp; // keep ignoring while the typing goes on
        if (!this.typingWarned) { this.typingWarned = true; s.notify(tr('ed.keys.typingIgnored'), true); }
        e.preventDefault();
        return;
      }
    }
    if (code === 'KeyC' && !e.shiftKey) return go(() => this.collab.toggleMode()); // comment mode (comments.ts)
    const tools: Record<string, Tool> = { KeyV: 'move', KeyH: 'hand', KeyF: 'frame', KeyA: 'frame', KeyR: 'rect', KeyO: 'ellipse', KeyT: 'text', KeyP: 'pen' };
    if (!e.shiftKey && tools[code]) return go(() => s.tool.set(tools[code]));

    if (/^Digit[0-9]$/.test(code) && !e.shiftKey && id) return go(() => s.setOpacityKey(Number(code.slice(5))));

    switch (e.key) {
      case 'Delete':
      case 'Backspace':
        return go(() => s.removeMany());
      case 'Escape':
        return go(() => {
          if (this.collab.escape()) return; // comment draft / thread / comment mode first, then the canvas
          if (this.shortcuts()) this.shortcuts.set(false);
          else if (s.tool() !== 'move') s.tool.set('move');
          else s.selectParent();
        });
      case 'Enter':
        return go(() => s.selectChildren());
      case 'Tab':
        return go(() => s.selectSibling(1));
      case 'ArrowUp': case 'ArrowDown': case 'ArrowLeft': case 'ArrowRight': {
        if (!id) return;
        return go(() => {
          const step = e.shiftKey ? 10 : 1;
          if (s.parentMode(id) === 'none') {
            const dx = e.key === 'ArrowLeft' ? -step : e.key === 'ArrowRight' ? step : 0;
            const dy = e.key === 'ArrowUp' ? -step : e.key === 'ArrowDown' ? step : 0;
            s.beginGesture(); // the whole nudge is one undo step
            for (const o of s.selection()) {
              const ef = s.eff(o.node);
              s.setPos(o.node.id, (ef.x ?? 0) + dx, (ef.y ?? 0) + dy);
            }
            s.endGesture();
          } else {
            s.moveBy(id, e.key === 'ArrowUp' || e.key === 'ArrowLeft' ? -1 : 1);
          }
        });
      }
    }
  }
}

/**
 * Shows server-rendered preview HTML in `w` (a blank tab we opened synchronously so popup
 * blockers allow it). The page may contain user scripts (Embed blocks, plugins), so it never
 * runs in the admin origin: it is rendered inside an `<iframe sandbox>` *without*
 * `allow-same-origin`, which gives it an opaque origin — no access to the admin DOM,
 * cookies-by-script, localStorage (the auth token) or the opener. The outer tab only holds
 * our own static wrapper, and its `opener` link is cut.
 */
function showSandboxedPreview(w: Window, html: string, title: string) {
  try { w.opener = null; } catch { /* ignore */ }
  const base = `<base href="${SITE_ORIGIN || location.origin}/">`;
  const page = /<head[^>]*>/i.test(html) ? html.replace(/<head[^>]*>/i, m => m + base) : base + html;
  const d = w.document;
  d.open();
  d.write('<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">'
    + '<meta name="referrer" content="no-referrer"><title></title>'
    + '<style>html,body{margin:0;height:100%;overflow:hidden;background:#fff}iframe{border:0;width:100%;height:100%;display:block}</style>'
    + '</head><body></body></html>');
  d.close();
  d.title = title || 'Preview';
  const f = d.createElement('iframe');
  // allow-scripts keeps interactions/animations working; no allow-same-origin => opaque origin.
  // allow-downloads: a code block's <a download> (a generated QR / image) works as on the live page.
  f.setAttribute('sandbox', 'allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-modals allow-downloads');
  f.setAttribute('referrerpolicy', 'no-referrer');
  f.srcdoc = page;
  d.body.appendChild(f);
}

/** A side panel's saved width (localStorage), or `def`. */
function panelWidth(side: 'left' | 'right', def: number): number {
  try {
    const v = Number(localStorage.getItem('anasaki.panel.' + side));
    return v >= 220 && v <= 480 ? v : def;
  } catch { return def; }
}
