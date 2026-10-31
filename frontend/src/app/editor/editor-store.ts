import { DestroyRef, Injectable, computed, effect, inject, signal } from '@angular/core';
import { Dialog } from '../core/dialog';
import { Api, DesignAssets, errorMessage } from '../core/api';
import { Plugins } from '../core/plugins';
import { tr } from '../core/i18n';
import './messages-editor';
import { BlockInfo, DesignComponent, Media, Menu, Post, Settings, Term } from '../core/models';
import {
  Breakpoint, DNode, Layout, LayoutMode, Override, Props, Size, Style, cloneWithNewIds, effective, findNode, isAncestor, isContainer, newId, pathTo,
} from './design-types';
import { PART_CLASS, Theme, ensureFont, partCss, partHoverCss, parseTheme, themeStyle } from './design-css';
import { LruCache, normalizeBlocks, replaceInRichHtml, richText, shareTree } from './editor-utils';
import { SiteRole, siteRoleIds } from './site-role';
import './messages-editor4';

export type EditorKind = 'post' | 'component';
export type SampleCtx = Record<string, string>;
export type Tool = 'move' | 'hand' | 'frame' | 'rect' | 'ellipse' | 'text' | 'pen';
export type AlignDir = 'left' | 'hcenter' | 'right' | 'top' | 'vcenter' | 'bottom';
export interface Rect { x: number; y: number; w: number; h: number }

/** Undo steps kept. Steps share every unchanged layer with each other (shareTree), so this costs far less than 200 copies. */
const HISTORY_LIMIT = 200;
/** Plugin block previews kept in memory (least recently used dropped first). */
const BLOCK_CACHE_LIMIT = 120;
const CLIPBOARD_TAG = 'anasaki-studio/nodes';

/**
 * State of one editing session (one page/template/component). Provided by the
 * editor component, so every editor gets a fresh instance.
 */
@Injectable()
export class EditorStore {
  private api = inject(Api);
  readonly plugins = inject(Plugins);

  /** Layer whose path points are being edited (double-click a vector / Enter). */
  readonly vectorEditId = signal<string | null>(null);
  /** Last used Export settings (the Ctrl+Shift+E command repeats them). */
  readonly exportSettings = signal<{ format: 'png' | 'jpg' | 'svg'; scale: number }>({ format: 'png', scale: 2 });

  constructor() {
    // stylesheets this editor added to <head> go away with it
    inject(DestroyRef).onDestroy(() => {
      this.blockStyle?.remove();
      this.partsStyle?.remove();
      this.blockStyle = this.partsStyle = null;
      clearTimeout(this.fxTimer);
    });
  }

  readonly kind = signal<EditorKind>('post');
  readonly post = signal<Post | null>(null);
  readonly component = signal<DesignComponent | null>(null);
  readonly title = signal('');
  /** Address of the page (/slug); saved with the page. */
  readonly slug = signal('');

  /** Renames the page; a still-default address follows the new name. */
  renamePage(title: string) {
    const p = this.post();
    if (p && (this.slug() === p.title || decodeURIComponent(this.slug()) === this.title() || /^(หน้าใหม่|new-page|page)(-\d+)?$/.test(this.slug()))) {
      this.slug.set(slugify(title));
    }
    this.title.set(title);
    this.dirty.set(true);
  }
  readonly status = signal<'draft' | 'publish'>('draft');

  readonly doc = signal<DNode>(emptyRoot());
  /** Primary selection (the last clicked layer) — drives the Design panel. */
  readonly selectedId = signal<string | null>(null);
  /** Full selection, primary included (Shift+click / marquee). */
  readonly selectedIds = signal<string[]>([]);
  readonly hoverId = signal<string | null>(null);
  readonly editingTextId = signal<string | null>(null);
  readonly renamingId = signal<string | null>(null);
  readonly bp = signal<Breakpoint>('desktop');
  readonly zoom = signal(1);
  readonly tool = signal<Tool>('move');
  readonly spaceDown = signal(false);
  readonly altDown = signal(false);
  readonly dirty = signal(false);
  readonly saving = signal(false);
  readonly dropIndicator = signal<Rect | null>(null);
  readonly dropTarget = signal<string | null>(null);
  readonly contextMenu = signal<{ x: number; y: number } | null>(null);
  readonly toast = signal<{ text: string; error?: boolean } | null>(null);

  /**
   * Click selects the deepest layer under the pointer (default, like Framer/Webflow).
   * Off = Figma level rules (first click picks the outer frame). Ctrl/⌘+click does the other one.
   */
  readonly deepSelect = signal(readPref("anasaki.deepSelect", true));
  /** Dragging places layers exactly where dropped (Absolute inside auto layout). Ctrl = the other mode. */
  /** Designer preview of a layer's hover state or scroll-in animation (the canvas never hovers for real). */
  readonly fxPreview = signal<{ id: string; kind: 'hover' | 'anim' } | null>(null);
  private fxTimer = 0;

  previewFx(id: string, kind: 'hover' | 'anim') {
    clearTimeout(this.fxTimer);
    this.fxPreview.set(null);
    // next frame, so an animation that is already showing restarts
    requestAnimationFrame(() => {
      this.fxPreview.set({ id, kind });
      const a = findNode(this.doc(), id)?.node.style?.anim;
      const ms = kind === 'anim' ? (a?.duration ?? 700) + (a?.delay ?? 0) + 300 : 1600;
      this.fxTimer = window.setTimeout(() => this.fxPreview.set(null), ms);
    });
  }

  readonly freeDrag = signal(readPref("anasaki.freeDrag", true));

  /** Factory of the Assets item being dragged onto the canvas (HTML5 drag & drop). */
  readonly assetDrag = signal<{ make: () => DNode; section?: boolean } | null>(null);

  /** Set by the canvas: starts move/marquee gestures from a node pointerdown. */
  nodePointerDown?: (id: string, e: PointerEvent, opts?: { clickSelect?: string; marqueeIn?: string }) => void;
  /** Set by the canvas: a layer's rect in design pixels, relative to the artboard. */
  measure?: (id: string) => Rect | null;

  // CMS data used for live bindings on the canvas
  readonly components = signal<DesignComponent[]>([]);
  readonly menus = signal<Menu[]>([]);
  readonly terms = signal<Term[]>([]);
  readonly settings = signal<Settings>({});
  readonly samplePosts = signal<{ ctx: SampleCtx; cats: string[] }[]>([]);

  private past: DNode[] = [];
  private future: DNode[] = [];
  private gestureStart: DNode | null = null;
  private clipboard: DNode[] = [];
  private propsClipboard: { style?: Style; layout?: Layout } | null = null;
  readonly historyVersion = signal(0);

  readonly canUndo = computed(() => (this.historyVersion(), this.past.length > 0));
  readonly canRedo = computed(() => (this.historyVersion(), this.future.length > 0));

  readonly selected = computed(() => {
    const id = this.selectedId();
    return id ? findNode(this.doc(), id) : null;
  });

  readonly selection = computed(() =>
    this.selectedIds().map(id => findNode(this.doc(), id)).filter((x): x is NonNullable<typeof x> => !!x));

  readonly componentMap = computed(() => new Map(this.components().map(c => [c.id, c])));
  /**
   * Components that may be placed in the open document: never the component
   * being edited, nor one that contains it through nested instances (that
   * would render forever, here and on the site).
   */
  readonly placeableComponents = computed(() => {
    const own = this.component()?.id;
    if (own == null) return this.components();
    const map = this.componentMap();
    const refs = (n: DNode, out: number[]) => {
      if (n.type === 'instance' && n.props?.componentId != null) out.push(n.props.componentId);
      n.children?.forEach(c => refs(c, out));
      return out;
    };
    const reaches = (id: number, seen: Set<number>): boolean => {
      if (id === own) return true;
      if (seen.has(id)) return false;
      seen.add(id);
      const c = map.get(id);
      if (!c) return false;
      const ids = refs(c.node, []);
      c.node.props?.variants?.forEach(v => refs(v.node, ids));
      return ids.some(r => reaches(r, seen));
    };
    return this.components().filter(c => !reaches(c.id, new Set()));
  });

  /** The site's design tokens; `themeDraft` holds unsaved edits from the Theme panel. */
  readonly theme = computed(() => parseTheme(this.settings()['theme']));
  readonly themeDraft = signal<Theme | null>(null);
  readonly liveTheme = computed(() => this.themeDraft() ?? this.theme());
  readonly themeVars = computed(() => {
    const t = this.liveTheme();
    ensureFont(t.fontBody || undefined);
    ensureFont(t.fontHeading || undefined);
    return themeStyle(t);
  });
  /** Only the --c-* color variables (for swatches in the panels, outside the artboard). */
  readonly themeColorVars = computed(() => {
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(this.themeVars())) if (k.startsWith('--c-')) out[k] = v;
    return out;
  });

  async saveTheme(quiet = false) {
    const t = this.themeDraft();
    if (!t) return;
    const saved = await this.api.saveSettings({ theme: JSON.stringify(t) });
    this.settings.set(saved);
    // keep edits made while the request was in flight
    if (this.themeDraft() === t) this.themeDraft.set(null);
    if (!quiet) this.notify(tr('ed.theme.saved'));
  }

  /** Which sample post a template previews (index into samplePosts). */
  readonly sampleIndex = signal(0);

  readonly siteCtx = computed<SampleCtx>(() => ({
    title: this.settings()['site_title'] ?? '',
    tagline: this.settings()['tagline'] ?? '',
    url: '/',
    logo: this.settings()['site_logo'] ?? '',
    icon: this.settings()['site_icon'] ?? '',
  }));

  /** Binding context for the page itself ({{post.*}} outside a post list). */
  readonly pageCtx = computed<SampleCtx | null>(() => {
    const p = this.post();
    if (!p) return null;
    if (p.type === 'template') {
      const list = this.samplePosts();
      return list[Math.min(this.sampleIndex(), list.length - 1)]?.ctx ?? null;
    }
    const home = this.settings()['home_page_id'] === String(p.id) || p.slug === 'home';
    return { title: p.title, excerpt: p.excerpt, url: home ? '/' : '/' + p.slug, slug: p.slug, type: p.type, isHome: home ? '1' : '' };
  });

  // ------------------------------------------------------------------ loading

  async load(kind: EditorKind, id: number) {
    this.kind.set(kind);
    this.plugins.load();
    this.loadBlocks();
    this.loadAssets();
    const [components, menus, terms, settings, posts] = await Promise.all([
      this.api.components(), this.api.menus(), this.api.terms(), this.api.settings(),
      this.api.posts({ type: 'post', status: 'publish', perPage: 12 }),
    ]);
    this.loadSitePages();
    this.components.set(components);
    this.menus.set(menus);
    this.terms.set(terms);
    this.settings.set(settings);
    this.samplePosts.set(posts.items.map(p => ({
      cats: p.categories.map(c => c.slug),
      ctx: {
        id: String(p.id), title: p.title, excerpt: p.excerpt, date: new Date(p.publishedAt ?? p.createdAt)
          .toLocaleDateString((settings['language'] || 'th') === 'th' ? 'th-TH' : 'en-GB', { day: 'numeric', month: 'short', year: 'numeric' }),
        author: p.authorName, image: p.featuredImage, category: p.categories[0]?.name ?? '',
        categoryUrl: p.categories[0] ? '/category/' + p.categories[0].slug : '',
        url: '/' + (settings['blog_base'] || 'blog') + '/' + p.slug, slug: p.slug, type: 'post', pinned: p.pinned ? '1' : '', sortOrder: String(p.sortOrder ?? 0),
        content: tr('ed.ph.sampleContent'),
      },
    })));
    if (posts.items[0]) {
      this.api.getPost(posts.items[0].id).then(full => {
        const s = this.samplePosts();
        if (s[0] && full.content) { s[0].ctx = { ...s[0].ctx, content: full.content }; this.samplePosts.set([...s]); }
      }).catch(() => undefined);
    }

    if (kind === 'post') {
      const p = await this.api.getPost(id);
      this.post.set(p);
      this.title.set(p.title);
      this.slug.set(p.slug);
      this.status.set(p.status);
      this.doc.set(p.design ?? emptyRoot());
    } else {
      const c = components.find(x => x.id === id);
      if (!c) throw new Error('component not found');
      this.component.set(c);
      this.title.set(c.name);
      this.doc.set(c.node);
      this.variantIndex.set(-1);
      this.variantBase.set(null);
    }
    this.past = [];
    this.future = [];
    this.variantRenames = [];
    this.historyVersion.update(v => v + 1);
    this.dirty.set(false);
  }

  /** Set when a save was refused because someone else saved a newer version (collab.ts shows the dialog). */
  readonly conflict = signal<{ by: string; byId: number; at: string; status?: 'draft' | 'publish' } | null>(null);

  async save(status?: 'draft' | 'publish', opts?: { force?: boolean }) {
    this.saving.set(true);
    let moved: [Post, Post] | null = null;
    // optimistic concurrency: the server answers 409 when the stored version is newer than the one loaded
    const base = (at?: string) => (opts?.force || !at ? {} : { baseUpdatedAt: at });
    // Text styles (and other theme edits) live in the theme draft until saved; layers using a
    // text style drop their own values, so the page would look wrong on the site without it.
    let themeSaved = false;
    if (this.themeDraft()) {
      try { await this.saveTheme(true); themeSaved = true; } catch (e) { this.notify(tr('ed.theme.saveFail') + errorMessage(e), true); }
    }
    try {
      if (this.kind() === 'post') {
        const p = this.post()!;
        const saved = await this.api.updatePost(p.id, { title: this.title(), slug: this.slug() || undefined, design: this.doc(), status: status ?? this.status(), ...base(p.updatedAt) });
        this.post.set(saved);
        this.slug.set(saved.slug);
        this.status.set(saved.status);
        if (saved.slug !== p.slug && p.type !== 'template') moved = [p, saved];
      } else {
        const c = this.component()!;
        const saved = await this.api.saveComponent({ id: c.id, name: this.title(), node: this.componentTree(), ...base(c.updatedAt) });
        this.component.set(saved);
        // the library (instances, insert panel, variant pickers) must show what was just saved
        this.components.update(list => list.some(x => x.id === saved.id) ? list.map(x => (x.id === saved.id ? saved : x)) : [...list, saved]);
      }
      this.dirty.set(false);
      this.notify(tr(status === 'publish' ? 'ed.toast.published' : 'ed.toast.saved') + (themeSaved ? ' ' + tr('ed.theme.savedWithPage') : ''));
      if (moved) await this.relink(moved[0], moved[1]); // its own message follows the "saved" one
      if (this.kind() === 'component' && this.variantRenames.length) await this.pushVariantRenames(this.component()!.id);
    } catch (e) {
      const cf = (e as { status?: number; error?: { conflict?: { updatedBy?: string; updatedById?: number; updatedAt?: string } } });
      if (cf?.status === 409 && cf.error?.conflict) {
        const c = cf.error.conflict;
        this.conflict.set({ by: c.updatedBy ?? '', byId: c.updatedById ?? 0, at: c.updatedAt ?? '', status });
      } else this.notify(tr('ed.toast.saveFail'), true);
      throw e;
    } finally {
      this.saving.set(false);
    }
  }

  notify(text: string, error = false) {
    Dialog.toast(text, error ? 'error' : 'success');
  }

  // ------------------------------------------------------------------ history

  /**
   * Applies a change to a copy of the document and records it for undo.
   * Unchanged layers keep their objects (shareTree): only what changed
   * re-renders, and undo steps share memory. While a gesture is open (drag,
   * resize, scrub…) the change goes to a working copy kept for the whole
   * gesture — no full clone, no history entry and no anchor/group refit per
   * pointer move; endGesture does those once.
   */
  mutate(fn: (root: DNode) => void) {
    const prev = this.doc();
    if (this.gestureStart !== null) {
      if (!this.work || this.workFor !== prev) this.work = structuredClone(prev); // first move, or the doc was replaced meanwhile
      fn(this.work);
      this.syncAnchors(prev, this.work, true); // only re-parented layers now; the rest at endGesture
      const next = shareTree(prev, this.work, true);
      this.workFor = next;
      if (next !== prev) {
        this.doc.set(next);
        this.dirty.set(true);
      }
      return;
    }
    const work = structuredClone(prev);
    fn(work);
    this.syncAnchors(prev, work);
    if (this.bp() === 'desktop') this.fitGroups(work);
    const next = shareTree(prev, work);
    if (next === prev) return; // nothing changed: no empty undo step
    this.pushHistory(prev);
    this.doc.set(next);
    this.dirty.set(true);
  }

  /** Gesture working copy (see mutate) and the published doc it matches. */
  private work: DNode | null = null;
  private workFor: DNode | null = null;

  private pushHistory(prev: DNode) {
    this.past.push(prev);
    if (this.past.length > HISTORY_LIMIT) this.past.shift();
    this.future = [];
    this.historyVersion.update(v => v + 1);
  }

  /**
   * Freeform groups hug their children like Figma groups: after any change the
   * group's box is recomputed from its children (and shifted when it is itself
   * freely positioned, so nothing moves on screen).
   */
  private fitGroups(root: DNode) {
    const visit = (n: DNode, parentFree: boolean) => {
      const free = (n.layout?.mode ?? 'none') === 'none';
      n.children?.forEach(c => visit(c, free || !!c.absolute));
      if (!n.group || !free || !n.children?.length) return;
      const boxes = n.children.map(c => {
        const r = this.measure?.(c.id);
        return { c, x: c.x ?? 0, y: c.y ?? 0, w: c.width.mode === 'fixed' ? c.width.value ?? 0 : r?.w ?? 0, h: c.height.mode === 'fixed' ? c.height.value ?? 0 : r?.h ?? 0 };
      });
      let x0 = Math.min(...boxes.map(b => b.x)), y0 = Math.min(...boxes.map(b => b.y));
      const x1 = Math.max(...boxes.map(b => b.x + b.w)), y1 = Math.max(...boxes.map(b => b.y + b.h));
      if (!parentFree) { x0 = 0; y0 = 0; } // in a flow parent the group can't move, only grow
      if (x0 || y0) {
        n.x = Math.round((n.x ?? 0) + x0);
        n.y = Math.round((n.y ?? 0) + y0);
        for (const b of boxes) { b.c.x = Math.round(b.x - x0); b.c.y = Math.round(b.y - y0); }
      }
      n.width = { mode: 'fixed', value: Math.max(1, Math.round(x1 - x0)) };
      n.height = { mode: 'fixed', value: Math.max(1, Math.round(y1 - y0)) };
    };
    visit(root, false);
  }

  /**
   * Drag/resize/scrub: many live updates collapse into one undo step. A gesture
   * that was left open (its end event never came) is closed — and recorded —
   * first, so it can't swallow every later change.
   */
  beginGesture() {
    if (this.gestureStart !== null) this.endGesture();
    this.gestureStart = this.doc();
    this.work = this.workFor = null;
  }
  /** True while a gesture is collecting changes. */
  get inGesture() { return this.gestureStart !== null; }
  /** Runs several edits as one undo step (or as part of the gesture already open). */
  transact(fn: () => void) {
    if (this.gestureStart !== null) { fn(); return; }
    this.beginGesture();
    try { fn(); } finally { this.endGesture(); }
  }
  endGesture() {
    const start = this.gestureStart;
    this.gestureStart = null;
    this.work = this.workFor = null;
    if (!start) return;
    const cur = this.doc();
    if (start === cur) return;
    // once per gesture: anchors of moved layers and group bounds
    const fin = structuredClone(cur);
    this.syncAnchors(start, fin);
    if (this.bp() === 'desktop') this.fitGroups(fin);
    const next = shareTree(cur, fin);
    if (next !== cur) this.doc.set(next);
    if (next !== start) this.pushHistory(start);
  }

  undo() {
    if (this.gestureStart !== null) this.endGesture(); // a stuck gesture must not block undo
    const prev = this.past.pop();
    if (!prev) return;
    this.future.push(this.doc());
    this.doc.set(prev);
    this.afterHistory();
  }

  redo() {
    if (this.gestureStart !== null) this.endGesture();
    const next = this.future.pop();
    if (!next) return;
    this.past.push(this.doc());
    this.doc.set(next);
    this.afterHistory();
  }

  private afterHistory() {
    this.dirty.set(true);
    this.historyVersion.update(v => v + 1);
    this.selectMany(this.selectedIds().filter(id => findNode(this.doc(), id)));
  }

  // ------------------------------------------------------------------ selection (Figma rules)

  select(id: string | null) {
    if (this.editingTextId() && this.editingTextId() !== id) this.editingTextId.set(null);
    this.selectedId.set(id);
    this.selectedIds.set(id ? [id] : []);
  }

  selectMany(ids: string[]) {
    const uniq = [...new Set(ids)].filter(id => id !== this.doc().id || ids.length === 1);
    this.selectedIds.set(uniq);
    this.selectedId.set(uniq.length ? (uniq.includes(this.selectedId() ?? '') ? this.selectedId() : uniq[uniq.length - 1]) : null);
  }

  toggleSelect(id: string) {
    const cur = this.selectedIds();
    if (cur.includes(id)) this.selectMany(cur.filter(x => x !== id));
    else {
      this.selectedIds.set([...cur.filter(x => x !== this.doc().id), id]);
      this.selectedId.set(id);
    }
  }

  isSelected(id: string) { return this.selectedIds().includes(id); }

  setFreeDrag(on: boolean) {
    this.freeDrag.set(on);
    try { localStorage.setItem("anasaki.freeDrag", on ? "1" : "0"); } catch { /* ignore */ }
  }

  setDeepSelect(on: boolean) {
    this.deepSelect.set(on);
    try { localStorage.setItem("anasaki.deepSelect", on ? "1" : "0"); } catch { /* ignore */ }
  }

  /** The selected layer (if any) that contains `id` — pressing inside it should drag it. */
  selectedAncestorOf(id: string): string | null {
    const root = this.doc();
    return this.selectedIds().find(s => s !== root.id && (s === id || isAncestor(root, s, id))) ?? null;
  }

  /**
   * Which layer a click on `deepId` selects. Like Figma: first click picks the
   * child of the artboard; clicks inside the current selection's parent pick
   * at that level; Ctrl/Cmd+click picks the deepest layer. Locked layers are skipped.
   */
  pickTarget(deepId: string, deep = false): string {
    const path = pathTo(this.doc(), deepId);
    if (!path.length) return deepId;
    const unlocked = (i: number) => {
      while (i > 0 && path[i].locked) i--;
      return path[i].id;
    };
    if (deep) {
      // a group is picked as one unit until you are working inside it (Figma)
      const cur = this.selectedId();
      for (let i = 1; i < path.length - 1; i++) {
        const g = path[i];
        if (g.group && !(cur && (cur === g.id || isAncestor(this.doc(), g.id, cur)))) return unlocked(i);
      }
      return unlocked(path.length - 1);
    }
    const sel = this.selectedId();
    if (sel && sel !== path[0].id) { // a selected artboard behaves like no selection
      const si = path.findIndex(n => n.id === sel);
      if (si >= 0) return unlocked(si);
      const parentId = findNode(this.doc(), sel)?.parent?.id;
      const pi = path.findIndex(n => n.id === parentId);
      if (pi >= 0 && pi + 1 < path.length) return unlocked(pi + 1);
    }
    return unlocked(Math.min(1, path.length - 1));
  }

  /** Double-click: go one level deeper toward `deepId` (or edit text). */
  drillDown(deepId: string) {
    const path = pathTo(this.doc(), deepId);
    const sel = this.selectedId();
    const si = path.findIndex(n => n.id === sel);
    const node = path[path.length - 1];
    if (si === path.length - 1 || (si < 0 && path.length <= 2)) {
      if (node && (node.type === 'text' || node.type === 'button')) this.editingTextId.set(node.id);
      return;
    }
    const next = path[Math.max(si + 1, 1)];
    if (next) {
      this.select(next.id);
      if (next.id === node.id && (node.type === 'text' || node.type === 'button')) this.editingTextId.set(node.id);
    }
  }

  selectParent() {
    const s = this.selected();
    if (s?.parent) this.select(s.parent.id);
    else this.select(null);
  }

  selectChildren() {
    const n = this.selected()?.node;
    if (n?.children?.length) this.selectMany(n.children.map(c => c.id));
    else if (n && (n.type === 'text' || n.type === 'button')) this.editingTextId.set(n.id);
  }

  selectSibling(delta: number) {
    const s = this.selected();
    if (!s?.parent) return;
    const kids = s.parent.children!;
    this.select(kids[(s.index + delta + kids.length) % kids.length].id);
  }

  /** Ctrl+A: siblings of the selection, or everything on the artboard. */
  selectAll() {
    const parent = this.selected()?.parent ?? this.doc();
    this.selectMany((parent.children ?? []).map(c => c.id));
  }

  /** Selected ids sharing the primary's parent (needed for group/align). */
  siblingSelection(): string[] {
    const p = this.selected()?.parent;
    if (!p) return [];
    return this.selection().filter(s => s.parent?.id === p.id).sort((a, b) => a.index - b.index).map(s => s.node.id);
  }

  // ------------------------------------------------------------------ helpers

  /** The node with breakpoint overrides applied. */
  eff(n: DNode): DNode { return effective(n, this.bp()); }

  parentMode(id: string): LayoutMode | '' {
    const info = findNode(this.doc(), id);
    if (!info?.parent) return '';
    if (info.node.absolute) return 'none';
    return this.eff(info.parent).layout?.mode ?? 'none';
  }

  /** Rect of `id` relative to its parent's box (design px). */
  private relRect(id: string): Rect | null {
    const info = findNode(this.doc(), id);
    const r = this.measure?.(id);
    const pr = info?.parent ? this.measure?.(info.parent.id) : null;
    if (!r || !pr) return r ?? null;
    return { x: r.x - pr.x, y: r.y - pr.y, w: r.w, h: r.h };
  }

  // ------------------------------------------------------------------ property edits (breakpoint-aware, multi-select)

  private override(n: DNode): Override | null {
    const bp = this.bp();
    if (bp === 'desktop') return null;
    n.responsive ??= {};
    return (n.responsive[bp] ??= {});
  }

  private edit(ids: string | string[], fn: (n: DNode, o: Override | null) => void) {
    const list = Array.isArray(ids) ? ids : [ids];
    this.mutate(root => {
      for (const id of list) {
        const f = findNode(root, id);
        if (!f) continue;
        fn(f.node, this.override(f.node));
        cleanupResponsive(f.node);
      }
    });
  }

  setLayout(ids: string | string[], patch: Partial<Layout>) {
    this.edit(ids, (n, o) => {
      if (o) o.layout = assign(o.layout, patch);
      else n.layout = assign(n.layout, patch);
    });
  }

  /** Switching to/from freeform keeps every child where it is on screen. */
  setLayoutMode(id: string, mode: LayoutMode) {
    const info = findNode(this.doc(), id);
    if (!info) return;
    const cur = this.eff(info.node).layout?.mode ?? 'none';
    if (cur === mode) return;
    if (mode !== 'none') {
      this.setLayout(id, { mode, gap: this.eff(info.node).layout?.gap ?? 16 });
      return;
    }
    const kids = (info.node.children ?? []).map(c => ({ c, r: this.relRect(c.id) }));
    const self = this.measure?.(id);
    this.edit(id, (n, o) => {
      if (o) o.layout = assign(o.layout, { mode: 'none' }); else n.layout = assign(n.layout, { mode: 'none' });
      if (self && this.eff(n).height.mode === 'hug') {
        if (o) o.height = { mode: 'fixed', value: Math.round(self.h) }; else n.height = { mode: 'fixed', value: Math.round(self.h) };
      }
      for (const k of kids) {
        const child = n.children!.find(x => x.id === k.c.id);
        if (!child || !k.r) continue;
        const co = this.override(child);
        const x = Math.round(k.r.x), y = Math.round(k.r.y);
        if (co) { co.x = x; co.y = y; } else { child.x = x; child.y = y; }
        if (this.eff(child).width.mode === 'fill') {
          if (co) co.width = { mode: 'fixed', value: Math.round(k.r.w) }; else child.width = { mode: 'fixed', value: Math.round(k.r.w) };
        }
        if (this.eff(child).height.mode === 'fill') {
          if (co) co.height = { mode: 'fixed', value: Math.round(k.r.h) }; else child.height = { mode: 'fixed', value: Math.round(k.r.h) };
        }
        cleanupResponsive(child);
      }
    });
  }

  setStyle(ids: string | string[], patch: Partial<Style>) {
    this.edit(ids, (n, o) => {
      if (o) o.style = assign(o.style, patch);
      else n.style = assign(n.style, patch);
    });
  }

  /**
   * Sets style keys for every breakpoint (the base value, and the same keys
   * removed from tablet/mobile overrides) — used for shared text styles.
   */
  setStyleAll(ids: string | string[], patch: Partial<Style>) {
    this.edit(ids, n => {
      n.style = assign(n.style, patch);
      for (const bp of ['tablet', 'mobile'] as const) {
        const o = n.responsive?.[bp];
        if (o?.style) for (const k of Object.keys(patch)) delete (o.style as Record<string, unknown>)[k];
      }
    });
  }

  // ------------------------------------------------------------------ find & select matching (Figma)

  /** Selects every layer that looks like the selected one (same type, component, styles). */
  selectMatching() {
    const cur = this.selected()?.node;
    if (!cur) return;
    const key = (n: DNode) => JSON.stringify([n.type, n.props?.componentId, n.style?.textStyle, n.style?.fill, n.style?.color,
      n.style?.fontSize, n.style?.fontWeight, n.style?.radius, n.style?.stroke, n.props?.icon, n.props?.shape]);
    const want = key(cur), ids: string[] = [];
    const walk = (n: DNode) => { if (key(n) === want) ids.push(n.id); n.children?.forEach(walk); };
    walk(this.doc());
    this.selectMany(ids);
    this.notify(tr('fr.matched', { n: ids.length }));
  }

  /** Text layers (text, buttons, downloads) whose text — plain or formatted — contains q. */
  findText(q: string, caseSensitive = false): { id: string; name: string; text: string }[] {
    if (!q) return [];
    const norm = (s: string) => (caseSensitive ? s : s.toLowerCase());
    const out: { id: string; name: string; text: string }[] = [];
    const walk = (n: DNode) => {
      const rich = n.props?.rich;
      const t = typeof rich === 'string' && rich ? richText(rich) : n.props?.text;
      if (typeof t === 'string' && norm(t).includes(norm(q))) out.push({ id: n.id, name: n.name || n.type, text: t });
      n.children?.forEach(walk);
    };
    walk(this.doc());
    return out;
  }

  /**
   * Replaces q in the text of the given layers (one undo step). Formatted
   * text keeps its formatting: the replacement happens inside its text runs.
   */
  replaceText(ids: string[], q: string, by: string, caseSensitive = false) {
    if (!q || !ids.length) return 0;
    const re = new RegExp(q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), caseSensitive ? 'g' : 'gi');
    let n = 0;
    this.mutate(root => {
      for (const id of ids) {
        const f = findNode(root, id);
        const p = f?.node.props;
        if (!f || !p) continue;
        const t = typeof p.text === 'string' ? p.text.replace(re, () => by) : p.text;
        const rich = typeof p.rich === 'string' && p.rich ? replaceInRichHtml(p.rich, re, by) : p.rich;
        if (t !== p.text || rich !== p.rich) {
          f.node.props = { ...p, text: t, rich };
          if (!rich) delete f.node.props.rich;
          n++;
        }
      }
    });
    return n;
  }

  // ------------------------------------------------------------------ component variants (Figma)

  /** Variant being designed (-1 = the main design). */
  readonly variantIndex = signal(-1);
  /** The main design (with the variant list) while a variant is open. */
  private readonly variantBase = signal<DNode | null>(null);
  readonly variants = computed(() => (this.variantIndex() < 0 ? this.doc() : this.variantBase())?.props?.variants ?? []);
  readonly mainVariantName = computed(() => (this.variantIndex() < 0 ? this.doc() : this.variantBase())?.props?.variantName || 'Default');

  /** The whole component (main design + variants) as it is saved. */
  componentTree(): DNode {
    const i = this.variantIndex(), base = this.variantBase();
    if (i < 0 || !base) return this.doc();
    const tree = structuredClone(base);
    const node = structuredClone(this.doc());
    if (node.props) { delete node.props.variants; delete node.props.variantName; }
    tree.props!.variants![i] = { ...tree.props!.variants![i], node };
    return tree;
  }

  /** Opens a variant (-1 = main design) on the canvas. */
  openVariant(i: number) {
    const tree = this.componentTree();
    const list = tree.props?.variants ?? [];
    if (i >= list.length) i = -1;
    if (i < 0) { this.variantBase.set(null); this.doc.set(tree); }
    else { this.variantBase.set(tree); this.doc.set(structuredClone(list[i].node)); }
    this.variantIndex.set(i);
    this.past = [];
    this.future = [];
    this.historyVersion.update(v => v + 1);
    this.select(null);
  }

  /** Changes the variant list, then reopens `open`. */
  private setTree(fn: (tree: DNode) => void, open = this.variantIndex()) {
    const tree = this.componentTree();
    tree.props ??= {};
    fn(tree);
    if (!tree.props.variants?.length) delete tree.props.variants;
    this.variantIndex.set(-1);
    this.variantBase.set(null);
    this.doc.set(tree);
    this.dirty.set(true);
    this.openVariant(open);
  }

  /** New variant: a copy of the design on the canvas (same layer ids, so per-page changes carry over). */
  addVariant(name: string) {
    const src = structuredClone(this.doc());
    if (src.props) { delete src.props.variants; delete src.props.variantName; }
    let at = 0;
    this.setTree(t => { t.props!.variants = [...(t.props!.variants ?? []), { name, node: src }]; at = t.props!.variants.length - 1; }, -1);
    this.openVariant(at);
  }

  /**
   * Renames a variant. Placements pick a variant by name, so instances of this
   * component inside the open document follow the new name. (Placements saved in
   * other pages/components are not reachable from here — the variant bar warns.)
   * The main design's name is only a label: its placements store no name.
   */
  renameVariant(i: number, name: string) {
    const own = this.component()?.id;
    this.setTree(t => {
      if (i < 0) { t.props!.variantName = name; return; }
      const old = t.props!.variants![i].name;
      t.props!.variants![i].name = name;
      this.recordVariantRename(old, name);
      const follow = (n: DNode) => {
        if (n.type === 'instance' && own != null && n.props?.componentId === own && n.props.variant === old) n.props.variant = name;
        n.children?.forEach(follow);
      };
      follow(t);
      t.props!.variants!.forEach(v => follow(v.node));
    });
  }

  deleteVariant(i: number) {
    const gone = this.variants()[i]?.name;
    this.variantRenames = this.variantRenames.filter(r => r.to !== gone);
    this.setTree(t => { t.props!.variants!.splice(i, 1); }, -1);
  }

  /**
   * Variant renames since the last save. After the component is saved, the
   * server renames them in placements saved on other pages and components.
   */
  private variantRenames: { from: string; to: string }[] = [];
  /** false once the server said it can't do that (older backend): renaming warns instead. */
  readonly variantRenameApi = signal(true);

  private recordVariantRename(from: string, to: string) {
    const chain = this.variantRenames.find(r => r.to === from); // A→B then B→C = A→C
    if (chain) chain.to = to;
    else this.variantRenames.push({ from, to });
    this.variantRenames = this.variantRenames.filter(r => r.from !== r.to);
  }

  /** Called after a successful component save. */
  private async pushVariantRenames(id: number) {
    const list = this.variantRenames;
    this.variantRenames = [];
    let n = 0, done = 0;
    for (const r of list) {
      try {
        n += (await this.api.renameVariant(id, r.from, r.to)).updated ?? 0;
        done++;
      } catch (e) {
        if ((e as { status?: number })?.status === 404) {
          this.variantRenameApi.set(false);
          this.notify(tr('vr.renameWarn', { old: r.from, name: r.to }), true);
        } else {
          this.variantRenames.push(r); // try again with the next save
          this.notify(tr('vr.renameFail') + errorMessage(e), true);
        }
      }
    }
    if (done) {
      this.notify(tr('vr.renamedPages', { n }));
      try { this.components.set(await this.api.components()); } catch { /* the library refreshes on the next load */ }
    }
  }

  /** Pages of the site (for link pickers and "page not found" warnings). */
  readonly sitePages = signal<Post[]>([]);
  async loadSitePages() {
    try { this.sitePages.set((await this.api.posts({ type: 'page', perPage: 100 })).items); } catch { /* pickers fall back to typing */ }
  }

  /** Site path of a page or post. */
  pathOf(p: Pick<Post, 'type' | 'slug' | 'id'>): string {
    if (p.type === 'page') return this.settings()['home_page_id'] === String(p.id) ? '/' : '/' + p.slug;
    return '/' + (this.settings()['blog_base'] || 'blog') + '/' + p.slug;
  }

  /** The page moved: links to its old address everywhere follow it. */
  private async relink(before: Post, after: Post) {
    const from = this.pathOf(before), to = this.pathOf(after);
    if (from === to || from === '/') return;
    try {
      const r = await this.api.replaceLinks(from, to);
      if (r.changed) {
        // this page may have been one of them: take the server's copy (and its version stamp)
        const fresh = await this.api.getPost(after.id);
        this.post.set(fresh);
        if (fresh.design) this.doc.set(fresh.design);
        this.components.set(await this.api.components());
        this.notify(tr('ln.relinked', { n: r.changed, from: decodeURIComponent(from), to: decodeURIComponent(to) }));
      }
      this.loadSitePages();
    } catch { /* links keep the old address; the link fields warn about it */ }
  }

  /** Text styles by id (theme draft included), for the canvas. */
  readonly textStyleMap = computed(() => new Map(this.liveTheme().textStyles.map(s => [s.id, s])));

  setSize(ids: string | string[], axis: 'width' | 'height', size: Size) {
    this.edit(ids, (n, o) => {
      if (o) o[axis] = size;
      else n[axis] = size;
    });
  }

  setPos(ids: string | string[], x: number | null, y: number | null) {
    this.edit(ids, (n, o) => {
      const t = o ?? n;
      if (x !== null) t.x = x;
      if (y !== null) t.y = y;
    });
  }

  /** Sets size & position together (resize handles), one call per frame. */
  setBox(id: string, box: Partial<Rect>) {
    this.edit(id, (n, o) => {
      const t = o ?? n;
      if (box.x !== undefined) t.x = box.x;
      if (box.y !== undefined) t.y = box.y;
      if (box.w !== undefined) t.width = { mode: 'fixed', value: box.w };
      if (box.h !== undefined) {
        t.height = { mode: 'fixed', value: box.h };
        // a minimum height (often added automatically by keepHeight) would stop it from shrinking
        if ((this.eff(n).style?.minHeight ?? 0) > box.h) t.style = { ...t.style, minHeight: 0 };
      }
    });
  }

  /**
   * Horizontal anchor of a freely placed layer (Figma constraints): 'left' keeps
   * x from the parent's left edge; 'center' / 'right' keep the distance to the
   * parent's centre / right edge and 'scale' the fraction, when the published
   * page is wider or narrower than the designer.
   */
  setAnchor(ids: string | string[], anchor: 'left' | 'center' | 'right' | 'scale') {
    const list = Array.isArray(ids) ? ids : [ids];
    const refs = new Map(list.map(id => [id, this.parentWidth(id)]));
    this.edit(list, (n, o) => {
      // a breakpoint override says 'left' explicitly (an empty key would inherit the desktop anchor)
      const patch: Pick<Style, 'anchorX' | 'anchorRef'> = anchor !== 'left' ? { anchorX: anchor, anchorRef: refs.get(n.id) || undefined }
        : o ? { anchorX: 'left' } : { anchorX: undefined, anchorRef: undefined };
      if (o) o.style = assign(o.style, patch);
      else n.style = assign(n.style, patch);
    });
  }

  /** Width (design px) of a layer's parent at the current breakpoint. */
  private parentWidth(id: string): number {
    const p = findNode(this.doc(), id)?.parent;
    const r = p && this.measure?.(p.id);
    return r ? Math.round(r.w) : 0;
  }

  /**
   * The anchor a free drop gets inside a parent that stretches with the page
   * (width Fill): centre when the parent centres its content or the layer
   * sits in the middle third, otherwise the nearer edge. null = keep 'left'.
   */
  autoAnchor(parent: DNode, parentW: number, x: number, w: number): Pick<Style, 'anchorX' | 'anchorRef'> | null {
    const pe = this.eff(parent);
    if (pe.width.mode !== 'fill' || parentW <= 0) return null;
    const cx = x + w / 2;
    let anchorX: Style['anchorX'] = 'left';
    if (pe.layout?.align === 'center' || (cx > parentW / 3 && cx < (parentW * 2) / 3)) anchorX = 'center';
    else if (cx >= (parentW * 2) / 3) anchorX = 'right';
    return { anchorX, anchorRef: Math.round(parentW) };
  }

  /**
   * After a change: an anchored layer whose x (or parent) changed gets the
   * parent's current width as its new reference, written where its x lives
   * (the base design or this breakpoint's override).
   */
  private syncAnchors(prev: DNode, next: DNode, reparentedOnly = false) {
    if (!this.measure) return;
    const bp = this.bp();
    const before = new Map<string, { x?: number; parent: string }>();
    const walkPrev = (n: DNode) => n.children?.forEach(c => { before.set(c.id, { x: effective(c, bp).x, parent: n.id }); walkPrev(c); });
    walkPrev(prev);
    const walk = (p: DNode) => {
      for (const c of p.children ?? []) {
        walk(c);
        const e = effective(c, bp);
        const a = e.style?.anchorX;
        if (!a || a === 'left') continue;
        const was = before.get(c.id);
        if (was && was.parent === p.id && (reparentedOnly || was.x === e.x)) continue;
        const r = this.measure!(p.id);
        if (!r || r.w <= 0) continue;
        const ref = Math.round(r.w);
        const o = bp !== 'desktop' && c.responsive?.[bp]?.x !== undefined ? c.responsive[bp]! : null;
        if (o) o.style = assign(o.style, { anchorRef: ref });
        else c.style = assign(c.style, { anchorRef: ref });
      }
    };
    walk(next);
  }

  setVisible(ids: string | string[], visible: boolean) {
    this.edit(ids, (n, o) => {
      if (o) o.visible = visible;
      else n.visible = visible ? undefined : false;
    });
  }

  /** Props (content) are shared by all breakpoints. */
  setProps(ids: string | string[], patch: Partial<Props>) {
    const list = Array.isArray(ids) ? ids : [ids];
    this.mutate(root => {
      for (const id of list) {
        const f = findNode(root, id);
        if (f) f.node.props = assign(f.node.props, patch);
      }
    });
  }

  rename(id: string, name: string) {
    this.mutate(root => {
      const f = findNode(root, id);
      if (f) f.node.name = name;
    });
  }

  toggleLock(ids = this.selectedIds()) {
    if (!ids.length) return;
    const lock = !findNode(this.doc(), ids[0])?.node.locked;
    this.mutate(root => ids.forEach(id => {
      const f = findNode(root, id);
      if (f) f.node.locked = lock || undefined;
    }));
  }

  toggleVisible(ids = this.selectedIds()) {
    const first = ids.length ? findNode(this.doc(), ids[0])?.node : null;
    if (!first) return;
    this.setVisible(ids, this.eff(first).visible === false);
  }

  /** Figma "Absolute position" for a child of an auto-layout frame. */
  setAbsolute(id: string, on: boolean) {
    const r = on ? this.relRect(id) : null;
    this.mutate(root => {
      const f = findNode(root, id);
      if (!f) return;
      f.node.absolute = on || undefined;
      if (r) { f.node.x = Math.round(r.x); f.node.y = Math.round(r.y); }
    });
  }

  /**
   * Layers floating (Absolute) inside an auto-layout frame — texts, buttons,
   * pictures… (decorative icons/shapes are fine): they keep their spot while
   * the text around them reflows, so on smaller screens they can cover it.
   */
  readonly floating = computed(() => {
    const out: DNode[] = [];
    const deco = ['icon', 'shape', 'vector', 'rect'];
    const walk = (n: DNode, parentAuto: boolean) => {
      if (parentAuto && n.absolute && !deco.includes(n.type)) out.push(n);
      const auto = (n.layout?.mode ?? 'none') !== 'none';
      n.children?.forEach(c => walk(c, auto));
    };
    walk(this.doc(), false);
    return out;
  });

  /** Puts floating layers back into their frame's flow (one undo step). */
  unfloat(ids: string[]) {
    this.mutate(root => {
      for (const id of ids) {
        const f = findNode(root, id);
        if (f) { delete f.node.absolute; delete f.node.x; delete f.node.y; }
      }
    });
    this.notify(tr('fl.fixed', { n: ids.length }));
  }

  /** Is `key` of `group` overridden at the current breakpoint? */
  isOverridden(n: DNode, group: 'layout' | 'style' | 'width' | 'height' | 'x' | 'visible', key?: string): boolean {
    const bp = this.bp();
    if (bp === 'desktop') return false;
    const o = n.responsive?.[bp];
    if (!o) return false;
    if (group === 'layout' || group === 'style') return !!key && (o[group] as Record<string, unknown> | undefined)?.[key] !== undefined;
    return o[group] !== undefined;
  }

  /** Removes one breakpoint override (e.g. style.fontSize) so the value follows the wider screen again. */
  resetOverride(id: string, group: string, key?: string) {
    const bp = this.bp();
    if (bp === 'desktop') return;
    this.mutate(root => {
      const o = findNode(root, id)?.node.responsive?.[bp] as Record<string, unknown> | undefined;
      if (!o) return;
      if (key && o[group] && typeof o[group] === 'object') delete (o[group] as Record<string, unknown>)[key];
      else delete o[group];
      cleanupResponsive(findNode(root, id)!.node);
    });
  }

  resetOverrides(id: string) {
    const bp = this.bp();
    if (bp === 'desktop') return;
    this.mutate(root => {
      const f = findNode(root, id);
      if (f?.node.responsive) {
        delete f.node.responsive[bp];
        cleanupResponsive(f.node);
      }
    });
  }

  /** 0–9 keys: opacity 100%, 10% … 90% (Figma). */
  setOpacityKey(digit: number) {
    const ids = this.selectedIds();
    if (ids.length) this.setStyle(ids, { opacity: digit === 0 ? undefined : digit / 10 });
  }

  copyProps() {
    const n = this.selected()?.node;
    if (!n) return;
    const e = this.eff(n);
    this.propsClipboard = { style: structuredClone(e.style), layout: structuredClone(e.layout) };
    this.notify(tr('ed.toast.propsCopied'));
  }

  pasteProps() {
    const p = this.propsClipboard;
    const ids = this.selectedIds();
    if (!p || !ids.length) return;
    this.edit(ids, (n, o) => {
      const t = o ?? n;
      t.style = { ...p.style };
      if (p.layout && (n.type === 'frame' || n.type === 'button' || n.type === 'postList')) t.layout = { ...p.layout };
    });
  }

  // ------------------------------------------------------------------ structure

  /** Where a new layer goes: into the selected frame, or after the selection. */
  private insertionPoint(): { parentId: string; index?: number } {
    const sel = this.selected();
    if (sel) {
      if (isContainer(sel.node) && sel.node.type !== 'postList') return { parentId: sel.node.id };
      if (sel.parent) return { parentId: sel.parent.id, index: sel.index + 1 };
    }
    return { parentId: this.doc().id };
  }

  /** The site header/footer components (by their site role; by name on older data). */
  readonly siteRoles = computed(() => siteRoleIds(this.components(), this.settings()));

  /** 'header' / 'footer' when `n` places the site header/footer component. */
  instanceRole(n?: DNode): SiteRole | null {
    if (n?.type !== 'instance' || n.props?.componentId == null) return null;
    const r = this.siteRoles(), id = n.props.componentId;
    return id === r.header ? 'header' : id === r.footer ? 'footer' : null;
  }

  insert(node: DNode) {
    const { parentId, index } = this.insertionPoint();
    this.insertAt(node, parentId, index);
  }

  /**
   * A whole section (Assets → ready-made sections): on a page it goes on the
   * page itself, right after the section holding the selection — or, with
   * nothing selected, above the site footer — never inside another section.
   */
  insertSection(node: DNode) {
    const root = this.doc();
    if (this.kind() !== 'post' || (root.layout?.mode ?? 'none') === 'none') { this.insert(node); return; }
    const kids = root.children ?? [];
    let index = kids.length;
    const sel = this.selectedId();
    const top = sel && sel !== root.id ? kids.findIndex(k => k.id === sel || !!findNode(k, sel)) : -1;
    const isFooter = (k?: DNode) => this.instanceRole(k) === 'footer';
    if (top >= 0) index = isFooter(kids[top]) ? top : top + 1;
    else if (isFooter(kids[kids.length - 1])) index = kids.length - 1;
    this.insertAt(node, root.id, index);
  }

  insertAt(node: DNode, parentId: string, index?: number, keepPos = false) {
    // on a page, new things go between the site header and footer
    if (this.kind() === 'post' && parentId === this.doc().id && (this.doc().layout?.mode ?? 'none') !== 'none') {
      const kids = this.doc().children ?? [];
      const min = this.instanceRole(kids[0]) === 'header' ? 1 : 0;
      const max = this.instanceRole(kids[kids.length - 1]) === 'footer' ? kids.length - 1 : kids.length;
      if (!this.instanceRole(node)) {
        index = Math.min(Math.max(index ?? max, min), max);
      }
    }
    this.mutate(r => {
      const p = findNode(r, parentId)!.node;
      if (!keepPos) prepareForParent(node, effective(p, this.bp()).layout?.mode ?? 'none', (p.children?.length ?? 0) * 16);
      p.children ??= [];
      p.children.splice(index ?? p.children.length, 0, node);
    });
    this.select(node.id);
  }

  remove(id: string) { this.removeMany([id]); }

  removeMany(ids = this.selectedIds()) {
    const root = this.doc();
    // locked layers (or layers inside a locked one) stay, like on the canvas
    const locked = (id: string) => pathTo(root, id).some(n => n.locked);
    const asked = ids.filter(id => id !== root.id);
    const list = asked.filter(id => !locked(id));
    if (asked.length > list.length) this.notify(tr('ed.toast.lockedKept', { n: asked.length - list.length }), true);
    if (!list.length) return;
    const parent = findNode(this.doc(), list[0])?.parent;
    this.mutate(r => {
      for (const id of list) {
        const f = findNode(r, id);
        f?.parent?.children?.splice(f.index, 1);
      }
    });
    this.select(parent && findNode(this.doc(), parent.id) ? parent.id : null);
  }

  duplicate(id: string) {
    this.selectMany([id]);
    this.duplicateMany();
  }

  /** Ctrl+D: copies land right after their originals (offset in freeform). */
  duplicateMany(ids = this.selectedIds()) {
    const infos = ids.map(id => findNode(this.doc(), id)).filter(f => f?.parent) as NonNullable<ReturnType<typeof findNode>>[];
    if (!infos.length) return [];
    const copies: string[] = [];
    this.mutate(r => {
      for (const f of [...infos].sort((a, b) => b.index - a.index)) {
        const copy = cloneWithNewIds(f.node);
        copy.name = f.node.name;
        if (this.parentMode(f.node.id) === 'none') {
          copy.x = (copy.x ?? 0) + 16;
          copy.y = (copy.y ?? 0) + 16;
        }
        findNode(r, f.parent!.id)!.node.children!.splice(f.index + 1, 0, copy);
        copies.push(copy.id);
      }
    });
    this.selectMany(copies.reverse());
    return copies;
  }

  /** Moves a node to a new parent/index (layers drag & drop, canvas reorder). */
  /**
   * Moves a node to a new parent/index. With `pos` it lands at that spot:
   * in a freeform frame as usual, in an auto-layout frame as an Absolute
   * (free-floating) layer — so nothing has to be switched to freeform first.
   */
  move(id: string, parentId: string, index: number, pos?: { x: number; y: number }) {
    const root = this.doc();
    if (id === root.id || id === parentId || isAncestor(root, id, parentId)) return;
    const cur = findNode(root, id);
    if (!pos && cur?.parent?.id === parentId && (index === cur.index || index === cur.index + 1) && !cur.node.absolute) return; // dropped in place
    const size = pos ? this.measure?.(id) : null;
    const targetBox = pos ? this.measure?.(parentId) : null;
    const fromBox = pos && cur?.parent ? this.measure?.(cur.parent.id) : null;
    const wasInFlow = !!cur && !cur.node.absolute;
    this.mutate(r => {
      const f = findNode(r, id)!;
      const target = findNode(r, parentId)!.node;
      const targetFree = (effective(target, this.bp()).layout?.mode ?? 'none') === 'none';
      f.parent!.children!.splice(f.index, 1);
      if (f.parent!.id === parentId && f.index < index) index--;
      if (pos) {
        f.node.x = pos.x;
        f.node.y = pos.y;
        f.node.absolute = targetFree ? undefined : true;
        const anchor = this.autoAnchor(target, targetBox?.w ?? 0, pos.x, size?.w ?? 0);
        f.node.style = assign(f.node.style, anchor ?? { anchorX: undefined, anchorRef: undefined });
        if (!targetFree) keepHeight(target, Math.max(targetBox?.h ?? 0, pos.y + (size?.h ?? 0)));
        if (fromBox && wasInFlow) keepHeight(f.parent!, fromBox.h); // the frame it left keeps its height too
        if (size && f.node.width.mode === 'fill') f.node.width = { mode: 'fixed', value: Math.round(size.w) };
        if (size && f.node.height.mode === 'fill') f.node.height = { mode: 'fixed', value: Math.round(size.h) };
      } else {
        delete f.node.absolute; // back into the flow
        if (f.parent!.id !== parentId) prepareForParent(f.node, effective(target, this.bp()).layout?.mode ?? 'none', 24);
      }
      target.children ??= [];
      target.children.splice(Math.max(0, Math.min(index, target.children.length)), 0, f.node);
    });
  }

  moveBy(id: string, delta: number) {
    const f = findNode(this.doc(), id);
    if (!f?.parent) return;
    const to = f.index + delta;
    if (to < 0 || to >= f.parent.children!.length) return;
    this.move(id, f.parent.id, delta > 0 ? to + 1 : to);
  }

  /** Ctrl+] / Ctrl+[ (one step) and Ctrl+Shift+] / [ (to front/back). */
  reorder(dir: 'forward' | 'backward' | 'front' | 'back') {
    const ids = this.siblingSelection();
    const parent = this.selected()?.parent;
    if (!ids.length || !parent) return;
    this.mutate(r => {
      const p = findNode(r, parent.id)!.node;
      const kids = p.children!;
      const moving = kids.filter(k => ids.includes(k.id));
      const rest = kids.filter(k => !ids.includes(k.id));
      let at = kids.findIndex(k => ids.includes(k.id));
      if (dir === 'front') at = rest.length;
      else if (dir === 'back') at = 0;
      else if (dir === 'forward') at = Math.min(rest.length, at + 1);
      else at = Math.max(0, at - 1);
      rest.splice(at, 0, ...moving);
      p.children = rest;
    });
  }

  /** Ctrl+G / Ctrl+Alt+G: wrap the selection in a new frame. */
  /**
   * Ctrl+G / Ctrl+Alt+G: wrap the selection in a new frame without moving anything.
   * - same parent, auto layout: the wrapper copies the parent's direction/gap/alignment
   *   and fills when a child fills, so the layout looks exactly the same;
   * - freeform parent (or layers from different frames inside a freeform frame):
   *   a freeform wrapper sized to the selection bounds that refits as children move;
   * - layers spread across auto-layout frames can't be grouped without moving them,
   *   so we explain instead of silently grouping only some of them.
   * `asGroup` marks a Figma-style group: it is selected as one unit on the canvas.
   */
  group(name = 'Group', asGroup = true) {
    const root = this.doc();
    const sel = this.selection().filter(s => s.parent).map(s => s.node.id);
    if (!sel.length) return;
    const parents = new Set(sel.map(id => findNode(root, id)!.parent!.id));
    let parentId = [...parents][0];
    if (parents.size > 1) {
      parentId = commonAncestor(root, sel);
      if ((this.eff(findNode(root, parentId)!.node).layout?.mode ?? 'none') !== 'none') {
        this.notify(tr('ed.toast.groupMixed'), true);
        return;
      }
    }
    const parent = findNode(root, parentId)!.node;
    const ids = orderInTree(root, sel);
    const free = parents.size > 1 || ids.every(id => this.parentMode(id) === 'none');
    const prect = this.measure?.(parentId);
    const rects = new Map(ids.map(id => {
      const r = this.measure?.(id);
      return [id, r && prect ? { x: r.x - prect.x, y: r.y - prect.y, w: r.w, h: r.h } : null] as const;
    }));
    const frame: DNode = { id: newId(), type: 'frame', name, width: { mode: 'hug' }, height: { mode: 'hug' }, children: [] };
    if (asGroup) frame.group = true;
    if (free) {
      // layers that can't be measured (not drawn yet, hidden…) count with their stored box
      const rs = ids.map(id => rects.get(id) ?? this.storedRect(id));
      const x0 = Math.min(...rs.map(r => r.x)), y0 = Math.min(...rs.map(r => r.y));
      const x1 = Math.max(...rs.map(r => r.x + r.w)), y1 = Math.max(...rs.map(r => r.y + r.h));
      Object.assign(frame, { x: Math.round(x0), y: Math.round(y0), width: { mode: 'fixed', value: Math.round(x1 - x0) }, height: { mode: 'fixed', value: Math.round(y1 - y0) }, layout: { mode: 'none' } });
    } else {
      const pl = this.eff(parent).layout ?? {};
      const kids = ids.map(id => this.eff(findNode(root, id)!.node));
      frame.layout = { mode: pl.mode, gap: pl.gap ?? 0, align: pl.align };
      if (pl.wrap) frame.layout.wrap = true;
      if (kids.some(k => k.width.mode === 'fill')) frame.width = { mode: 'fill' };
      if (kids.some(k => k.height.mode === 'fill')) frame.height = { mode: 'fill' };
    }
    this.mutate(r => {
      const p = findNode(r, parentId)!.node;
      // insertion point: where the first selected layer (or its branch) sits in the parent
      const firstBranch = pathTo(r, ids[0])[pathTo(r, parentId).length];
      let at = p.children!.findIndex(c => c.id === firstBranch?.id);
      const moving: DNode[] = [];
      for (const id of ids) {
        const f = findNode(r, id)!;
        if (f.parent!.id === parentId && f.index < at) at--;
        f.parent!.children!.splice(f.index, 1);
        if (free) {
          const rr = rects.get(id);
          f.node.x = Math.round((rr?.x ?? f.node.x ?? 0) - (frame.x ?? 0));
          f.node.y = Math.round((rr?.y ?? f.node.y ?? 0) - (frame.y ?? 0));
          delete f.node.absolute;
          if (rr && f.node.width.mode === 'fill') f.node.width = { mode: 'fixed', value: Math.round(rr.w) };
          if (rr && f.node.height.mode === 'fill') f.node.height = { mode: 'fixed', value: Math.round(rr.h) };
        }
        moving.push(f.node);
      }
      frame.children = moving;
      p.children!.splice(Math.max(0, at), 0, frame);
    });
    this.select(frame.id);
  }

  /** A layer's box from its own x / y / fixed size (when it can't be measured). */
  private storedRect(id: string): Rect {
    const n = findNode(this.doc(), id)?.node;
    const e = n ? this.eff(n) : null;
    const size = (s?: Size) => (s?.mode === 'fixed' && s.value ? s.value : 100);
    return { x: e?.x ?? 0, y: e?.y ?? 0, w: size(e?.width), h: size(e?.height) };
  }

  wrapInFrame(id: string) {
    this.selectMany([id]);
    this.group('Frame', false);
  }

  /** Ctrl+Shift+G: replace the selected frame with its children. */
  ungroup() {
    const info = this.selected();
    if (!info?.parent || !info.node.children?.length || info.node.type !== 'frame') return;
    const parentFree = this.parentMode(info.node.id) === 'none';
    const rects = info.node.children.map(c => ({ id: c.id, r: this.relRect(c.id) }));
    const fx = this.eff(info.node).x ?? 0, fy = this.eff(info.node).y ?? 0;
    const ids = info.node.children.map(c => c.id);
    this.mutate(r => {
      const p = findNode(r, info.parent!.id)!.node;
      const f = findNode(r, info.node.id)!;
      const kids = f.node.children!;
      for (const c of kids) {
        if (parentFree) {
          const rr = rects.find(o => o.id === c.id)?.r;
          c.x = Math.round(fx + (rr?.x ?? c.x ?? 0));
          c.y = Math.round(fy + (rr?.y ?? c.y ?? 0));
        }
      }
      p.children!.splice(f.index, 1, ...kids);
    });
    this.selectMany(ids);
  }

  /** Shift+A: add auto layout to a frame, or wrap the selection in one. */
  addAutoLayout() {
    const n = this.selected()?.node;
    if (!n) return;
    if (this.selectedIds().length === 1 && n.type === 'frame' && (this.eff(n).layout?.mode ?? 'none') === 'none') {
      const kids = (n.children ?? []).map(c => this.relRect(c.id)).filter((r): r is Rect => !!r);
      const spreadX = kids.length > 1 ? Math.max(...kids.map(r => r.x)) - Math.min(...kids.map(r => r.x)) : 0;
      const spreadY = kids.length > 1 ? Math.max(...kids.map(r => r.y)) - Math.min(...kids.map(r => r.y)) : 1;
      const mode: LayoutMode = spreadX > spreadY ? 'horizontal' : 'vertical';
      this.mutate(r => {
        const f = findNode(r, n.id)!.node;
        f.layout = { ...f.layout, mode, gap: f.layout?.gap ?? 10 };
        if (f.height.mode === 'fixed') f.height = { mode: 'hug' };
        // children follow their on-screen order
        if (f.children) {
          const pos = new Map(f.children.map(c => [c.id, this.relRect(c.id)]));
          f.children.sort((a, b) => mode === 'horizontal' ? (pos.get(a.id)?.x ?? 0) - (pos.get(b.id)?.x ?? 0) : (pos.get(a.id)?.y ?? 0) - (pos.get(b.id)?.y ?? 0));
        }
      });
      return;
    }
    this.transact(() => { // wrap + turn on auto layout = one undo step
      this.group('Auto layout', false);
      const id = this.selectedId();
      if (id) this.mutate(r => {
        const f = findNode(r, id)!.node;
        const wasFree = f.layout?.mode === 'none';
        f.layout = { mode: 'vertical', gap: 10 };
        if (wasFree) { f.width = { mode: 'hug' }; f.height = { mode: 'hug' }; }
      });
    });
  }

  removeAutoLayout() {
    const id = this.selectedId();
    if (id) this.setLayoutMode(id, 'none');
  }

  // ------------------------------------------------------------------ align & distribute

  align(dir: AlignDir) {
    const ids = this.siblingSelection();
    if (!ids.length) return;
    const parent = this.selected()!.parent!;
    if (ids.length === 1 && this.parentMode(ids[0]) !== 'none') {
      // inside auto layout: align by changing the parent's alignment (like Figma)
      const pm = this.eff(parent).layout?.mode;
      const v = dir === 'left' || dir === 'top' ? 'start' : dir === 'right' || dir === 'bottom' ? 'end' : 'center';
      const crossIsHorizontal = pm === 'vertical';
      const isHorizontalDir = dir === 'left' || dir === 'hcenter' || dir === 'right';
      if (isHorizontalDir === crossIsHorizontal) this.setLayout(parent.id, { align: v as Layout['align'] });
      else this.setLayout(parent.id, { justify: v as Layout['justify'] });
      return;
    }
    const rects = ids.map(id => ({ id, r: this.relRect(id) })).filter(o => o.r) as { id: string; r: Rect }[];
    if (!rects.length) return;
    let box: Rect;
    if (rects.length === 1) {
      const pr = this.measure?.(parent.id);
      box = { x: 0, y: 0, w: pr?.w ?? 0, h: pr?.h ?? 0 };
    } else {
      const x0 = Math.min(...rects.map(o => o.r.x)), y0 = Math.min(...rects.map(o => o.r.y));
      box = { x: x0, y: y0, w: Math.max(...rects.map(o => o.r.x + o.r.w)) - x0, h: Math.max(...rects.map(o => o.r.y + o.r.h)) - y0 };
    }
    this.mutate(root => {
      for (const { id, r } of rects) {
        const f = findNode(root, id)!;
        const t = this.override(f.node) ?? f.node;
        if (dir === 'left') t.x = Math.round(box.x);
        if (dir === 'hcenter') t.x = Math.round(box.x + (box.w - r.w) / 2);
        if (dir === 'right') t.x = Math.round(box.x + box.w - r.w);
        if (dir === 'top') t.y = Math.round(box.y);
        if (dir === 'vcenter') t.y = Math.round(box.y + (box.h - r.h) / 2);
        if (dir === 'bottom') t.y = Math.round(box.y + box.h - r.h);
        cleanupResponsive(f.node);
      }
    });
  }

  distribute(axis: 'h' | 'v') {
    const ids = this.siblingSelection().filter(id => this.parentMode(id) === 'none');
    const rects = ids.map(id => ({ id, r: this.relRect(id)! })).filter(o => o.r);
    if (rects.length < 3) return;
    const k = axis === 'h' ? 'x' : 'y', s = axis === 'h' ? 'w' : 'h';
    rects.sort((a, b) => a.r[k] - b.r[k]);
    const first = rects[0].r, last = rects[rects.length - 1].r;
    const total = rects.reduce((sum, o) => sum + o.r[s], 0);
    const gap = (last[k] + last[s] - first[k] - total) / (rects.length - 1);
    let cur = first[k];
    this.mutate(root => {
      for (const { id, r } of rects) {
        const f = findNode(root, id)!;
        const t = this.override(f.node) ?? f.node;
        t[k] = Math.round(cur);
        cur += r[s] + gap;
        cleanupResponsive(f.node);
      }
    });
  }

  // ------------------------------------------------------------------ clipboard

  async copy(ids = this.selectedIds()) {
    const nodes = ids.map(id => findNode(this.doc(), id)?.node).filter((n): n is DNode => !!n && n.id !== this.doc().id);
    if (!nodes.length) return;
    this.clipboard = structuredClone(nodes);
    try { await navigator.clipboard.writeText(JSON.stringify({ [CLIPBOARD_TAG]: nodes })); } catch { /* clipboard blocked */ }
  }

  cut() {
    this.copy();
    this.removeMany();
  }

  /** Ctrl+V (also from another editor tab via the system clipboard). */
  async paste(inPlace = false) {
    let nodes = this.clipboard;
    try {
      const t = await navigator.clipboard.readText();
      const parsed = JSON.parse(t)?.[CLIPBOARD_TAG];
      if (Array.isArray(parsed) && parsed.length) nodes = parsed;
    } catch { /* not ours or blocked */ }
    if (!nodes.length) return;
    const copies = nodes.map(n => cloneWithNewIds(n));
    const { parentId, index } = this.insertionPoint();
    const mode = this.eff(findNode(this.doc(), parentId)!.node).layout?.mode ?? 'none';
    this.mutate(r => {
      const p = findNode(r, parentId)!.node;
      p.children ??= [];
      copies.forEach((c, i) => {
        if (mode === 'none' && !inPlace) { c.x = (c.x ?? 0) + 16; c.y = (c.y ?? 0) + 16; }
        p.children!.splice((index ?? p.children!.length) + i, 0, c);
      });
    });
    this.selectMany(copies.map(c => c.id));
  }

  /** Swaps a post list's card for another ready-made style; the query and list layout stay. */
  applyCardStyle(listId: string, card: DNode, layout?: Partial<Layout>, responsive?: DNode['responsive']) {
    this.mutate(r => {
      const f = findNode(r, listId);
      if (!f) return;
      f.node.children = [card];
      if (layout) f.node.layout = { ...f.node.layout, ...layout };
      if (responsive !== undefined) f.node.responsive = responsive;
      cleanupResponsive(f.node);
    });
    this.select(listId);
  }

  /** Replaces the whole design with an earlier version (one undo step; saved with Save). */
  applyVersion(design: DNode) {
    this.mutate(r => {
      const next = structuredClone(design);
      Object.keys(r).forEach(k => delete (r as unknown as Record<string, unknown>)[k]);
      Object.assign(r, next);
    });
    this.select(null);
  }

  // ------------------------------------------------------------------ components (Figma) ≈ template parts (WP)

  async createComponent(id: string, name: string) {
    const f = findNode(this.doc(), id);
    if (!f?.parent) return;
    const main = cloneWithNewIds(f.node);
    delete main.x; delete main.y; delete main.responsive; delete main.absolute;
    main.name = name;
    const comp = await this.api.saveComponent({ name, node: main });
    this.components.update(cs => [...cs, comp]);
    const inst: DNode = {
      id: newId(), type: 'instance', name, x: f.node.x, y: f.node.y, absolute: f.node.absolute,
      width: f.node.width, height: f.node.height, responsive: f.node.responsive,
      props: { componentId: comp.id },
    };
    this.mutate(r => findNode(r, f.parent!.id)!.node.children!.splice(f.index, 1, inst));
    this.select(inst.id);
    this.notify(tr('ed.toast.componentMade', { name }));
  }

  /**
   * A component made from the gallery while designing. As the site
   * header/footer the server already swapped it into every saved design, so
   * the open document gets the same change (saving must not undo it);
   * otherwise it is placed like any component.
   */
  async adoptNewComponent(c: DesignComponent, role: 'header' | 'footer' | null, oldId: number | null) {
    this.components.set(await this.api.components());
    if (this.kind() !== 'post') return;
    if (!role) { this.insertInstance(c); return; }
    // the server just saved this page with the swap: take its new version stamp so our save isn't a "conflict"
    const p = this.post();
    if (p) {
      try { const fresh = await this.api.getPost(p.id); this.post.set({ ...p, updatedAt: fresh.updatedAt }); } catch { /* the conflict dialog still protects it */ }
    }
    const refs: DNode[] = [];
    const walk = (n: DNode) => { if (n.type === 'instance' && n.props?.componentId === oldId) refs.push(n); n.children?.forEach(walk); };
    if (oldId) walk(this.doc());
    if (refs.length) {
      this.mutate(r => {
        const swap = (n: DNode) => { if (n.type === 'instance' && n.props?.componentId === oldId) n.props = { ...n.props, componentId: c.id }; n.children?.forEach(swap); };
        swap(r);
      });
    } else if (!oldId && ['page', 'template'].includes(this.post()?.type ?? '')) {
      const name = this.components().find(x => x.id === c.id)?.name ?? c.name; // renamed to ส่วนหัวเว็บ / ส่วนท้ายเว็บ
      const inst: DNode = { id: `site-${role}-${c.id}`, type: 'instance', name, width: { mode: 'fill' }, height: { mode: 'hug' }, props: { componentId: c.id } };
      this.mutate(r => { r.children ??= []; if (role === 'header') r.children.unshift(inst); else r.children.push(inst); });
    }
  }

  insertInstance(c: DesignComponent) {
    if (!this.placeableComponents().some(x => x.id === c.id)) { this.notify(tr('ed.toast.componentCycle')); return; }
    const node: DNode = {
      id: newId(), type: 'instance', name: c.name,
      width: c.node.width.mode === 'fill' ? { mode: 'fill' } : c.node.width, height: { mode: 'hug' },
      props: { componentId: c.id },
    };
    // full-width components are sections (never after the footer or inside another section)
    if (c.node.width.mode === 'fill') this.insertSection(node); else this.insert(node);
  }

  detachInstance(id: string) {
    const f = findNode(this.doc(), id);
    const comp = f && this.componentMap().get(f.node.props?.componentId ?? -1);
    if (!f?.parent || !comp) return;
    const tree = cloneWithNewIds(comp.node);
    Object.assign(tree, { x: f.node.x, y: f.node.y, absolute: f.node.absolute, width: f.node.width, height: f.node.height, responsive: f.node.responsive, name: comp.name });
    this.mutate(r => findNode(r, f.parent!.id)!.node.children!.splice(f.index, 1, tree));
    this.select(tree.id);
  }

  // ------------------------------------------------------------------ dynamic data for the canvas

  postsFor(n: DNode): SampleCtx[] {
    const q = n.props?.query ?? {};
    let list = this.samplePosts();
    if (q.category && q.category !== 'current') list = list.filter(p => p.cats.includes(q.category!));
    const out = list.map(p => p.ctx);
    if (q.order === 'title') out.sort((a, b) => a['title'].localeCompare(b['title']));
    if (q.order === 'oldest') out.reverse();
    if (q.order === 'manual') { // placed posts by position, the rest newest first (like the site)
      const pos = (x: SampleCtx) => Number(x['sortOrder']) || Number.MAX_SAFE_INTEGER;
      out.sort((a, b) => pos(a) - pos(b));
    }
    out.sort((a, b) => Number(!!b['pinned']) - Number(!!a['pinned'])); // pinned first (stable)
    return out.slice(0, q.limit || 6);
  }

  /** Sample items for collection blocks (fetched once per source from the plugin). */
  readonly collectionItems = signal<Record<string, SampleCtx[]>>({});
  private requested = new Set<string>();

  /** Loads the sample items of a collection source once (called from effects, never from computeds). */
  ensureCollection(src: string | undefined) {
    if (!src || this.requested.has(src)) return;
    this.requested.add(src);
    this.api.collection(src, 24)
      .then(items => this.collectionItems.update(c => ({ ...c, [src]: items })))
      .catch(() => undefined);
  }

  /** Sample items of a collection block (see ensureCollection). */
  itemsFor(n: DNode): SampleCtx[] {
    const src = n.props?.source;
    if (!src) return [];
    const q = n.props?.query ?? {};
    const list = [...(this.collectionItems()[src] ?? [])];
    if (q.order === 'title') list.sort((a, b) => (a['title'] ?? '').localeCompare(b['title'] ?? ''));
    if (q.order === 'oldest') list.reverse();
    return list.slice(0, q.limit || 6);
  }

  // ------------------------------------------------------------------ plugin blocks & files

  /** Blocks contributed by active plugins (built-in and packages). */
  readonly blocks = signal<BlockInfo[]>([]);
  /** Rendered previews keyed by plugin/block/values (same HTML the public site gets), LRU-capped. */
  private readonly blockHtml = new LruCache<string, { html: string; css: string } | null>(BLOCK_CACHE_LIMIT);
  /** Bumped when blockHtml changes (the cache itself is not a signal). */
  private readonly blockVersion = signal(0);
  private readonly blockPending = new Set<string>();
  private blockStyle: HTMLStyleElement | null = null;
  /** Latest stylesheet of each plugin block type. */
  private readonly blockCss = new Map<string, string>();
  readonly mediaFiles = signal<Media[]>([]);
  private mediaRequested = false;

  private partsStyle: HTMLStyleElement | null = null;
  /** Keeps the plugin block parts stylesheet in step with the document (edits, undo, loading). */
  private readonly partsSync = effect(() => this.syncPartsCss(this.doc()));
  /** Writes the plugin block parts of the document into one canvas stylesheet. */
  private syncPartsCss(root: DNode) {
    const rules: string[] = [];
    const walk = (n: DNode) => {
      if (n.type === 'pluginBlock' && n.props?.parts) {
        for (const [cls, p] of Object.entries(n.props.parts)) {
          const css = PART_CLASS.test(cls) ? partCss(p) : '';
          const sel = `[data-node-id="${n.id}"]`;
          if (css) rules.push(`${sel}${sel} .${cls}{${css}}`);
          const hov = PART_CLASS.test(cls) ? partHoverCss(p) : '';
          if (hov) rules.push(`${sel}${sel} .${cls}:hover{${hov}}`);
        }
      }
      n.children?.forEach(walk);
    };
    walk(root);
    const text = rules.join('\n');
    if (!text && !this.partsStyle) return;
    if (!this.partsStyle) {
      this.partsStyle = document.createElement('style');
      this.partsStyle.dataset['rea'] = 'block-parts';
      document.head.appendChild(this.partsStyle);
    }
    if (this.partsStyle.textContent !== text) this.partsStyle.textContent = text;
  }

  /** Built-in icons and shapes (one request per designer session). */
  readonly assets = signal<DesignAssets | null>(null);
  /** A plugin block part double-clicked on the canvas (opens it in the panel). */
  readonly partPick = signal<{ id: string; cls: string } | null>(null);
  async loadAssets() {
    if (this.assets()) return;
    try { this.assets.set(await this.api.designAssets()); } catch { /* the canvas shows placeholders */ }
  }

  /** The <svg> of an icon / shape node for the canvas (same markup as design.IconSVG / ShapeSVG). */
  svgFor(n: DNode): string {
    const a = this.assets();
    if (!a) return '';
    if (n.type === 'icon') {
      const ic = a.icons[n.props?.icon ?? ''] ?? a.icons['star'];
      if (!ic) return '';
      const sw = n.props?.strokeWidth && n.props.strokeWidth > 0 && n.props.strokeWidth <= 4 ? n.props.strokeWidth : 2;
      const paint = ic.fill ? 'fill="currentColor" stroke="none"' : `fill="none" stroke="currentColor" stroke-width="${sw}" stroke-linecap="round" stroke-linejoin="round"`;
      return `<svg viewBox="0 0 24 24" width="100%" height="100%" ${paint} style="display:block">${ic.body}</svg>`;
    }
    const sh = a.shapes[n.props?.shape ?? ''] ?? a.shapes['wave'];
    if (!sh) return '';
    return `<svg viewBox="${sh.viewBox}" preserveAspectRatio="${sh.stretch ? 'none' : 'xMidYMid meet'}" width="100%" height="100%" fill="currentColor" style="display:block">${sh.body}</svg>`;
  }

  async loadBlocks() {
    try { this.blocks.set(normalizeBlocks(await this.api.blocks())); } catch { this.blocks.set([]); }
  }

  blockInfo(n: DNode): BlockInfo | undefined {
    return this.blocks().find(b => b.plugin === n.props?.plugin && b.type === n.props?.block);
  }

  private blockKey(n: DNode): string | null {
    const p = n.props ?? {};
    return p.plugin && p.block ? JSON.stringify([p.plugin, p.block, p.values ?? {}]) : null;
  }

  /**
   * Fetches the server-rendered HTML of a plugin block once per distinct prop
   * set (debounced). Called from effects — never from computeds.
   */
  ensureBlockPreview(n: DNode) {
    const p = n.props ?? {};
    const key = this.blockKey(n);
    if (!key || this.blockHtml.has(key) || this.blockPending.has(key)) return;
    this.blockPending.add(key);
    setTimeout(() => this.api.renderBlock(p.plugin!, p.block!, p.values ?? {})
      .then(r => {
        this.injectBlockCss(`${p.plugin}/${p.block}`, r.css);
        this.blockHtml.set(key, r);
      })
      .catch(() => this.blockHtml.set(key, null))
      .finally(() => {
        this.blockPending.delete(key);
        this.blockVersion.update(v => v + 1);
      }), 250);
  }

  /** Server-rendered HTML for a plugin block (see ensureBlockPreview); null while it loads. */
  blockPreview(n: DNode): string | null {
    this.blockVersion();
    const key = this.blockKey(n);
    if (!key) return null;
    if (this.blockHtml.has(key)) return this.blockHtml.get(key)?.html ?? null;
    // While loading, show the last preview of this block so typing doesn't flicker.
    const p = n.props!;
    const prefix = JSON.stringify([p.plugin, p.block]).slice(0, -1);
    const prev = this.blockHtml.entriesNewestFirst().find(([k, v]) => v && k.startsWith(prefix));
    return prev?.[1]?.html ?? null;
  }

  /** Keeps one stylesheet per block type, updated when a plugin's CSS changes. */
  private injectBlockCss(key: string, css: string) {
    if ((this.blockCss.get(key) ?? '') === (css ?? '')) return;
    if (css) this.blockCss.set(key, css); else this.blockCss.delete(key);
    if (!this.blockStyle) {
      this.blockStyle = document.createElement('style');
      this.blockStyle.dataset['rea'] = 'plugin-blocks';
      document.head.appendChild(this.blockStyle);
    }
    this.blockStyle.textContent = [...this.blockCss.values()].join('\n');
  }

  /** Loads the media list once (download blocks show file names); `force` reloads it. */
  ensureMedia(force = false) {
    if (this.mediaRequested && !force) return;
    this.mediaRequested = true;
    this.api.media().then(m => this.mediaFiles.set(m)).catch(() => { this.mediaRequested = false; });
  }

  /** Non-image files for the download block's preview text (see ensureMedia). */
  fileInfo(id: string | undefined): Media | undefined {
    return this.mediaFiles().find(m => String(m.id) === String(id ?? ''));
  }

  refreshFiles() { this.ensureMedia(true); }

  menuItems(menuId?: number) {
    return this.menus().find(m => m.id === menuId)?.items ?? [];
  }
}

// ------------------------------------------------------------------ helpers

/** Hug-height auto-layout frame keeps at least `h` px when a child leaves the flow. */
function keepHeight(n: DNode, h: number) {
  if (n.height.mode !== 'hug' || (n.layout?.mode ?? 'none') === 'none' || !h) return;
  n.style = { ...n.style, minHeight: Math.max(n.style?.minHeight ?? 0, Math.round(h)) };
}

/** Deepest node containing all `ids`. */
function commonAncestor(root: DNode, ids: string[]): string {
  const paths = ids.map(id => pathTo(root, id));
  let i = 0;
  while (paths.every(p => p[i] && p[i].id === paths[0][i].id) && i < paths[0].length - 1) i++;
  return paths[0][Math.max(0, i - 1)].id;
}

/** `ids` sorted in document (layer) order. */
function orderInTree(root: DNode, ids: string[]): string[] {
  const order: string[] = [];
  const walk = (n: DNode) => { order.push(n.id); n.children?.forEach(walk); };
  walk(root);
  return [...ids].sort((a, b) => order.indexOf(a) - order.indexOf(b));
}

function readPref(key: string, def: boolean): boolean {
  try {
    const v = localStorage.getItem(key);
    return v === null ? def : v === "1";
  } catch {
    return def;
  }
}

export function emptyRoot(): DNode {
  return { id: 'root', type: 'frame', name: 'Page', width: { mode: 'fill' }, height: { mode: 'hug' }, layout: { mode: 'vertical' }, style: { fill: '#ffffff' }, children: [] };
}

function assign<T extends object>(target: T | undefined, patch: Partial<T>): T {
  const out = { ...(target ?? {}) } as Record<string, unknown>;
  for (const [k, v] of Object.entries(patch)) {
    if (v === undefined || v === '' || (typeof v === 'number' && Number.isNaN(v))) delete out[k];
    else out[k] = v;
  }
  return out as T;
}

function cleanupResponsive(n?: DNode) {
  if (!n?.responsive) return;
  for (const bp of ['tablet', 'mobile'] as const) {
    const o = n.responsive[bp];
    if (!o) continue;
    if (o.layout && !Object.keys(o.layout).length) delete o.layout;
    if (o.style && !Object.keys(o.style).length) delete o.style;
    if (!Object.keys(o).length) delete n.responsive[bp];
  }
  if (!Object.keys(n.responsive).length) delete n.responsive;
}

/** Adapts sizing when a node lands in a freeform or auto-layout parent. */
function prepareForParent(n: DNode, mode: string, offset: number) {
  if (mode === 'none') {
    n.x ??= 40 + offset;
    n.y ??= 40 + offset;
    if (n.width.mode === 'fill') n.width = { mode: 'fixed', value: 320 };
    if (n.height.mode === 'fill') n.height = { mode: 'fixed', value: 200 };
  }
}

/** A page address from a name: spaces → "-", no slashes or odd characters (Thai stays). */
export function slugify(s: string): string {
  return s.trim().toLowerCase().replace(/[\s_]+/g, '-').replace(/[^\p{L}\p{M}\p{N}-]+/gu, '').replace(/-+/g, '-').replace(/^-|-$/g, '').slice(0, 80);
}
