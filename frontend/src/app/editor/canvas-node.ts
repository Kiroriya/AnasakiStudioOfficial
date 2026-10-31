import { ChangeDetectionStrategy, Component, Directive, ElementRef, OnInit, computed, effect, inject, input, output } from '@angular/core';
import { DomSanitizer } from '@angular/platform-browser';
import { TPipe, localized, tr } from '../core/i18n';
import { DNode, LayoutMode, effective, findNode, isContainer } from './design-types';
import { BindCtx, PageLink, bind, bindEscaped, ensureFont, escapeHtml, exprCtx, hasFormatting, hoverCss, instanceChildren, nodeCss, pageItemDecls, pageLinks, sanitizeRich, focal, maskUrl, withTextStyle, PART_CLASS } from './design-css';
import { EditorStore, SampleCtx } from './editor-store';
import { CodeData } from './code-data';
import { buildInputOptions, isCodeSource } from './code-logic';
import { evalExpr, exprPaths, toText, truthy } from './expr';
import { canvasHtml, codeBlockClass, scopeCss } from './codeblock';
import './messages-sitecode';
import { vectorSvg } from './vector';

/** Result of an inline edit: plain text, plus formatted HTML when the text has formatting. */
export interface InlineResult { text: string; rich?: string }

/**
 * contenteditable span used for inline text editing (double-click). Text
 * layers edit as rich text (the floating toolbar adds bold, colors, links);
 * buttons stay plain text.
 */
@Directive({
  selector: '[reaInlineEdit]',
  host: {
    '[attr.contenteditable]': "rich() ? 'true' : 'plaintext-only'",
    spellcheck: 'false',
    '(blur)': 'blur($event)',
    '(keydown)': 'key($event)',
    '(paste)': 'paste($event)',
    '(pointerdown)': '$event.stopPropagation()',
  },
})
export class InlineEdit implements OnInit {
  private el = inject<ElementRef<HTMLElement>>(ElementRef);
  readonly text = input('');
  /** Rich mode: start from this HTML (or the escaped text). */
  readonly rich = input(false);
  readonly html = input<string | undefined>(undefined);
  readonly commit = output<InlineResult>();
  private finished = false;

  ngOnInit() {
    const el = this.el.nativeElement;
    if (this.rich()) el.innerHTML = this.html() || escapeHtml(this.text()).replace(/\n/g, '<br>');
    else el.textContent = this.text();
    queueMicrotask(() => {
      el.focus();
      const r = document.createRange();
      r.selectNodeContents(el);
      const s = getSelection();
      s?.removeAllRanges();
      s?.addRange(r);
    });
  }

  key(e: KeyboardEvent) {
    e.stopPropagation();
    if (e.key === 'Escape' || (e.key === 'Enter' && (e.metaKey || e.ctrlKey))) {
      e.preventDefault();
      this.el.nativeElement.blur();
      return;
    }
    if (this.rich() && e.key === 'Enter') { // a line break, not a new <div>
      e.preventDefault();
      document.execCommand('insertLineBreak');
    }
    if (this.rich() && (e.ctrlKey || e.metaKey) && ['b', 'i', 'u'].includes(e.key.toLowerCase())) {
      e.preventDefault();
      document.execCommand(({ b: 'bold', i: 'italic', u: 'underline' } as Record<string, string>)[e.key.toLowerCase()]);
    }
  }

  /** Pasting keeps only the text (formatting comes from the toolbar). */
  paste(e: ClipboardEvent) {
    if (!this.rich()) return;
    e.preventDefault();
    document.execCommand('insertText', false, e.clipboardData?.getData('text/plain') ?? '');
  }

  /** Clicking the text toolbar must not end the edit. */
  blur(e: FocusEvent) {
    const to = e.relatedTarget as HTMLElement | null;
    if (to?.closest('.rt-bar')) return;
    this.done();
  }

  done() {
    if (this.finished) return;
    this.finished = true;
    const el = this.el.nativeElement;
    const text = el.innerText.replace(/\n$/, '');
    if (!this.rich()) { this.commit.emit({ text }); return; }
    const html = sanitizeRich(el.innerHTML);
    this.commit.emit(hasFormatting(html) ? { text, rich: html } : { text });
  }
}

/**
 * Renders one design node on the canvas, recursively. The host <div> gets
 * exactly the CSS the Go renderer would emit (see design-css.ts).
 */
@Component({
  selector: 'div[reaNode]',
  imports: [InlineEdit, TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    '[attr.data-node-id]': 'interactive() ? node().id : null',
    '[style]': 'css()',
    '[class.rea-menu]': "view().type === 'menu'",
    '[class.rea-breadcrumb]': "view().type === 'breadcrumb'",
    '[class.rea-pagination]': "view().type === 'pagination'",
    '[class.rea-field]': "view().type === 'input'",
    '[class.rea-check]': "view().type === 'input' && view().props?.kind === 'checkbox'",
    '[class.is-cond-hidden]': "condHidden() === 'hidden' && code.showHidden()",
    '[attr.data-cond-hidden]': "condHidden() === 'hidden' ? '' : null",
    '[class.is-ghost]': 'ghost()',
    '[class.is-popup]': '!!view().props?.popup',
    '[class.is-collapsed]': '!!view().props?.collapsed',
    '[class.rea-car-edit]': "!!view().props?.carousel && view().type === 'frame'",
    '[attr.data-wfill]': "view().width.mode === 'fill' ? '' : null",
    '(pointerdown)': 'down($event)',
    '(pointerover)': 'over($event)',
    '(dblclick)': 'dbl($event)',
    '(contextmenu)': 'menu($event)',
  },
  template: `
    @if (condHidden() && interactive() && (code.showHidden() || condHidden() !== 'hidden')) {
      <span class="rea-cond-badge" [class.err]="condHidden() === 'error'" [attr.data-reason]="condHidden()">{{ ('sc.canvas.badge.' + condHidden()) | t }}</span>
    }
    @switch (view().type) {
      @case ('text') {
        @if (editing()) {
          <span reaInlineEdit [rich]="true" [html]="node().props?.rich" [text]="node().props?.text ?? ''" (commit)="commitText($event)"></span>
        } @else if (richHtml(); as h) {<span [innerHTML]="h"></span>} @else {<span>{{ text() }}</span>}
        @if (unknownFormatters(); as uf) { <sup class="rea-fmt-marker" [title]="'sc.canvas.serverFormatter' | t: { names: uf }">ƒ</sup> }
      }
      @case ('button') {
        @if (editing()) {
          <span reaInlineEdit [text]="node().props?.text ?? ''" (commit)="commitText($event)"></span>
        } @else {<span>{{ text() }}</span>}
        @if (unknownFormatters(); as uf) { <sup class="rea-fmt-marker" [title]="'sc.canvas.serverFormatter' | t: { names: uf }">ƒ</sup> }
      }
      @case ('image') {
        @if (src()) {
          <img [src]="src()" [alt]="view().props?.alt ?? ''" draggable="false"
               [style.object-fit]="view().style?.objectFit || 'cover'" [style.object-position]="focalOf(view())" class="rea-img" />
        } @else {
          <div class="rea-img-ph"><span>🖼</span></div>
        }
      }
      @case ('menu') {
        @if (view().props?.burger && store.bp() === 'mobile') {
          <span class="rea-burger-ic" [title]="'cs.burger' | t"><svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 7h16M4 12h16M4 17h16"/></svg></span>
        } @else {
          @for (it of store.menuItems(view().props?.menuId); track $index) { <a>{{ it.label }}</a> }
          @empty { <a class="muted">{{ 'ed.ph.chooseMenu' | t }}</a> }
        }
      }
      @case ('postContent') {
        <div class="rea-content" [innerHTML]="post()?.['content'] || placeholderContent"></div>
      }
      @case ('embed') {
        <div class="rea-embed" [innerHTML]="view().props?.html ?? ''"></div>
      }
      @case ('codeblock') {
        @if (codeBlockHtml(); as h) { <div class="rea-codeblock" [innerHTML]="h"></div> }
        @else { <div class="rea-pb-empty rea-codeblock-empty">&#123; &#125; {{ 'cb.empty' | t }}</div> }
        @if (interactive() && !code.codeBlock().edit) { <span class="rea-codeblock-lock codeblock-locked-badge" [title]="'cb.locked' | t">🔒 {{ 'cb.lockedBadge' | t }}</span> }
      }
      @case ('postList') {
        @for (p of posts(); track $index; let i = $index) {
          @for (c of view().children ?? []; track c.id) {
            <div reaNode [node]="c" [parentMode]="childMode()" [post]="p" [item]="item()" [ghost]="ghost() || i > 0" [inInstance]="inInstance()" [chain]="chain()"></div>
          }
        } @empty {
          <p class="rea-empty">{{ view().props?.emptyText || ('ed.ph.noPosts' | t) }}</p>
        }
      }
      @case ('collection') {
        @if (codeState()?.error; as err) { <p class="rea-code-err" data-code-error>⚠ {{ 'sc.canvas.error' | t }} {{ err }}</p> }
        @for (it of items(); track $index; let i = $index) {
          @for (c of itemChildren(); track c.id) {
            <div reaNode [node]="c" [parentMode]="childMode()" [post]="post()" [item]="it" [index]="i" [ghost]="ghost() || i > 0" [inInstance]="inInstance()" [chain]="chain()"></div>
          }
          @if ($last) {
            <!-- empty-state layers once after the items (hidden by their $empty condition, still editable) -->
            @for (c of emptyChildren(); track c.id) {
              <div reaNode [node]="c" [parentMode]="childMode()" [post]="post()" [item]="null" [empty]="false" [ghost]="ghost()" [inInstance]="inInstance()" [chain]="chain()"></div>
            }
          }
        } @empty {
          @if (emptyChildren().length) {
            @for (c of emptyChildren(); track c.id) {
              <div reaNode [node]="c" [parentMode]="childMode()" [post]="post()" [item]="null" [empty]="true" [ghost]="ghost()" [inInstance]="inInstance()" [chain]="chain()"></div>
            }
          } @else if (codeState()?.loading) {
            <p class="rea-empty">{{ 'sc.canvas.loading' | t }}</p>
          } @else {
            <p class="rea-empty">{{ view().props?.emptyText || ('ed.ph.noItems' | t) }}</p>
          }
        }
      }
      @case ('pagination') {
        @for (l of links(); track $index) {
          @if (l.kind === 'gap') { <span class="rea-pg-gap">…</span> }
          @else {
            <a class="rea-pg-item" [class.is-disabled]="l.disabled" [attr.aria-current]="l.active ? 'page' : null" [attr.rel]="l.kind === 'prev' || l.kind === 'next' ? l.kind : null" [style]="pgCss(l)">{{ pgLabel(l) }}</a>
          }
        }
      }
      @case ('input') {
        @switch (view().props?.kind || 'text') {
          @case ('select') {
            <select class="rea-field-el" tabindex="-1">
              @if (view().props?.placeholder) { <option value="">{{ view().props?.placeholder }}</option> }
              @for (o of inputOptions(); track $index) { <option [value]="o.value" [selected]="o.value === inputValue()">{{ o.label }}</option> }
            </select>
          }
          @case ('checkbox') {
            <input type="checkbox" class="rea-field-el" tabindex="-1" [checked]="inputChecked()" /><span>{{ view().props?.label || view().props?.placeholder || view().props?.name || '' }}</span>
          }
          @case ('textarea') { <textarea class="rea-field-el" tabindex="-1" readonly [placeholder]="view().props?.placeholder ?? ''" [value]="inputValue()"></textarea> }
          @case ('hidden') { <span class="rea-field-hidden">{{ 'sc.in.k.hidden' | t }} · {{ view().props?.name || '?' }}</span> }
          @default {
            <input class="rea-field-el" tabindex="-1" readonly [type]="view().props?.kind === 'email' ? 'email' : view().props?.kind === 'search' ? 'search' : 'text'"
                   [placeholder]="view().props?.placeholder ?? ''" [value]="inputValue()" />
          }
        }
      }
      @case ('pluginBlock') {
        @if (blockHtml(); as h) { <div class="rea-pb-preview" [innerHTML]="h"></div> }
        @else { <div class="rea-pb-empty">🧩 {{ blockLabel() }}</div> }
      }
      @case ('icon') { <span class="rea-svg" [innerHTML]="svg()"></span> }
      @case ('shape') { <span class="rea-svg" [innerHTML]="svg()"></span> }
      @case ('vector') { <span class="rea-svg" [innerHTML]="svg()"></span> }
      @case ('download') {<span>{{ text() || fileName() || ('ed.def.download' | t) }}@if (view().props?.showSize && fileMeta()) { <span class="rea-file-meta">{{ fileMeta() }}</span> }</span>}
      @case ('breadcrumb') {
        @for (c of crumbs(); track $index; let i = $index, last = $last) {
          @if (i > 0) { <span class="rea-bc-sep">{{ view().props?.separator || '›' }}</span> }
          @if (last && i > 0) { <span class="rea-bc-current" [style.color]="view().props?.currentColor || null">{{ c }}</span> }
          @else { <a>{{ c }}</a> }
        }
      }
      @case ('rect') {}
      @default {
        @for (c of view().children ?? []; track c.id) {
          <div reaNode [node]="c" [parentMode]="childMode()" [post]="post()" [item]="item()" [ghost]="ghost()"
               [inInstance]="inInstance() || node().type === 'instance'" [chain]="childChain()"></div>
        }
        @if (node().type === 'instance' && !instanceRoot()) {
          <div class="rea-missing">{{ 'ed.ph.missingComponent' | t }}</div>
        }
      }
    }
  `,
})
export class NodeView {
  readonly store = inject(EditorStore);
  private sanitizer = inject(DomSanitizer);
  /** Built-in icon / shape markup (from design.AssetCatalog, not user content). */
  /**
   * Frontend code block on the canvas: the HTML (scripts / handlers removed — JS never runs in
   * the editor) inside `.rea-cb-<id>`, with the CSS scoped exactly like the server does.
   */
  readonly codeBlockHtml = computed(() => {
    const v = this.view();
    if (v.type !== 'codeblock') return null;
    const html = v.props?.html ?? '';
    const css = v.props?.css ?? '';
    if (!html.trim() && !css.trim()) return null;
    const cls = codeBlockClass(v.id);
    const style = css.trim() ? `<style>${scopeCss(css, v.id)}</style>` : '';
    return this.sanitizer.bypassSecurityTrustHtml(`${style}<div class="${cls}" data-rea-code-block="${cls.slice(7)}">${canvasHtml(html, v.id)}</div>`);
  });
  readonly svg = computed(() => {
    const v = this.view(); // vector: validated path + paints (vector.ts)
    return this.sanitizer.bypassSecurityTrustHtml(v.type === 'vector' ? vectorSvg(v) : this.store.svgFor(v));
  });
  readonly node = input.required<DNode>();
  readonly parentMode = input<LayoutMode | ''>('');
  readonly post = input<SampleCtx | null>(null);
  /** Current collection item ({{item.x}} bindings). */
  readonly item = input<Record<string, unknown> | null>(null);
  /** Position of the item in its collection ({{$index}}). */
  readonly index = input<number | undefined>(undefined);
  /** Rendered as the empty state of a collection ($empty). */
  readonly empty = input(false);
  readonly code = inject(CodeData);
  readonly ghost = input(false);
  readonly inInstance = input(false);
  /** Ids of the components being rendered around this node (cycle guard). */
  readonly chain = input<readonly number[]>([]);

  get placeholderContent() { return tr('ed.ph.postContent'); }

  readonly interactive = computed(() => !this.ghost() && !this.inInstance());

  /** For instances: the main component's tree, sized/positioned by the instance. */
  readonly instanceRoot = computed(() => {
    const n = this.node();
    if (n.type !== 'instance') return null;
    const cid = n.props?.componentId ?? -1;
    // a component containing itself (old data) renders as an empty box instead of recursing forever
    if (cid === this.store.component()?.id || this.chain().includes(cid)) return null;
    const comp = this.store.componentMap().get(cid);
    if (!comp) return null;
    const v = n.props?.variant ? comp.node.props?.variants?.find(x => x.name === n.props!.variant)?.node : undefined;
    return effective(withTextStyle(v ?? comp.node, this.store.textStyleMap()), this.store.bp());
  });

  readonly childChain = computed(() => {
    const n = this.node();
    return n.type === 'instance' && n.props?.componentId != null ? [...this.chain(), n.props.componentId] : this.chain();
  });

  readonly view = computed<DNode>(() => {
    const e = effective(withTextStyle(this.node(), this.store.textStyleMap()), this.store.bp());
    const root = this.instanceRoot();
    if (!root) return e;
    return {
      ...root, id: e.id, x: e.x, y: e.y, width: e.width, height: e.height, visible: e.visible, absolute: e.absolute,
      // this placement's own style and inner-layer changes (same as renderInstance on the site)
      style: { ...root.style, ...e.style },
      children: instanceChildren(root.children, e.props?.overrides),
    };
  });

  /** Is this layer the primary selection? (a boolean: other selection changes don't touch css) */
  private readonly isPrimary = computed(() => this.store.selectedId() === this.node().id);
  /** Hover / animation preview of this layer, or null (same idea). */
  private readonly myFx = computed(() => {
    const fx = this.store.fxPreview();
    return fx && fx.id === this.node().id ? fx.kind : null;
  });

  readonly css = computed(() => {
    const v = this.view();
    ensureFont(v.style?.fontFamily);
    const c = nodeCss(v, this.parentMode());
    if (v.visible === false && this.interactive() && this.isPrimary()) {
      c['display'] = c['display'] === 'none' ? 'block' : c['display'];
      c['opacity'] = '0.35';
    }
    // hidden by its condition: dimmed (badge) or gone like on the site; the selected layer stays visible
    if (this.condHidden() === 'hidden' && !this.code.showHidden() && !(this.interactive() && this.isPrimary())) c['display'] = 'none';
    if (v.type === 'instance' && !this.instanceRoot()) c['min-height'] = '40px';
    const fx = this.myFx();
    if (fx && this.interactive()) {
      if (fx === 'hover') Object.assign(c, hoverCss(v));
      else if (v.style?.anim) {
        const a = v.style.anim;
        c['animation'] = `rea-k-${a.type} ${a.duration ?? 700}ms cubic-bezier(.2,.7,.2,1) ${a.delay ?? 0}ms both`;
      }
    }
    const m = v.style?.mask ? maskUrl(this.store.assets()?.masks?.[v.style.mask]) : '';
    if (m) { c['-webkit-mask'] = `${m} center/100% 100% no-repeat`; c['mask'] = `${m} center/100% 100% no-repeat`; }
    // theme heading font for h1–h6 text (the site does it with an h1…h6 rule)
    if (v.type === 'text' && /^h[1-6]$/.test(v.props?.tag ?? '') && !v.style?.fontFamily && this.store.liveTheme().fontHeading) {
      c['font-family'] = 'var(--font-heading)';
    }
    return c;
  });

  focalOf(v: DNode) { return focal(v.style ?? {}); }

  readonly childMode = computed<LayoutMode>(() => this.view().layout?.mode ?? 'none');
  readonly ctx = computed<BindCtx>(() => ({
    ...this.code.pageCtx(), post: this.post() ?? this.store.pageCtx(), item: this.item(), index: this.index(), empty: this.empty(),
  }));
  /** Preview of a code:/table: collection (loading / error / items). */
  readonly codeState = computed(() => {
    const v = this.view();
    return v.type === 'collection' && isCodeSource(v.props?.source) ? this.code.preview(this.code.requestFor(v)) : undefined;
  });
  readonly items = computed<Record<string, unknown>[]>(() => {
    const v = this.view();
    if (v.type !== 'collection') return [];
    return isCodeSource(v.props?.source) ? this.codeState()?.items ?? [] : this.store.itemsFor(v);
  });
  /** Children shown once when the collection has no items: layers whose condition uses $empty. */
  readonly emptyChildren = computed(() => (this.view().children ?? []).filter(c => !!c.props?.showWhen && exprPaths(c.props.showWhen).includes('$empty')));
  /** The card template: children rendered for every item (empty-state layers are rendered once instead). */
  readonly itemChildren = computed(() => {
    const empty = this.emptyChildren();
    return empty.length ? (this.view().children ?? []).filter(c => !empty.includes(c)) : this.view().children ?? [];
  });
  private readonly textInfo = computed(() => {
    const unknown = new Set<string>();
    const c = this.ctx();
    const text = bind(this.view().props?.text, { ...c, opts: { ...c.opts, unknown } });
    return { text, unknown: [...unknown].join(', ') };
  });
  readonly text = computed(() => this.textInfo().text);
  /** Script formatters (server only) used by the text: raw value shown + marker. */
  readonly unknownFormatters = computed(() => (this.interactive() ? this.textInfo().unknown : ''));

  /** 'hidden' when the layer's condition is false in this preview, 'error' when it does not parse, 'onePage' for a pagination the site hides. */
  readonly condHidden = computed<'hidden' | 'error' | 'onePage' | null>(() => {
    const v = this.view();
    const w = v.props?.showWhen?.trim();
    if (w) {
      const r = evalExpr(w, exprCtx(this.ctx()), this.ctx().opts);
      if (r.error) return 'error';
      if (!truthy(r.value)) return 'hidden';
    }
    if (v.type === 'pagination' && this.pgInfo().onePage) return 'onePage';
    return null;
  });

  // ---- pagination (page 1 of the bound collection's preview)
  private readonly pgInfo = computed(() => {
    const v = this.view();
    if (v.type !== 'pagination') return { pages: 0, onePage: false };
    const col = this.code.collectionNode(v.props?.collection);
    const req = col ? this.code.requestFor(col) : null;
    const st = this.code.preview(req);
    if (!req || !st || st.loading || st.error) return { pages: 5, onePage: false }; // sample until the preview is in
    const per = req.perPage || 10;
    return { pages: Math.max(1, Math.ceil(st.total / per)), onePage: st.total <= per };
  });
  readonly links = computed(() => {
    const v = this.view();
    return v.type === 'pagination' ? pageLinks(v.props?.style, 1, this.pgInfo().pages, v.props?.window ?? 2) : [];
  });
  private readonly pgDecls = computed(() => {
    const v = this.view();
    return v.type === 'pagination' ? { item: pageItemDecls(v, 'item'), active: pageItemDecls(v, 'active'), disabled: pageItemDecls(v, 'disabled') } : null;
  });
  pgCss(l: PageLink): Record<string, string> {
    const d = this.pgDecls();
    if (!d) return {};
    return { ...d.item, ...(l.active ? d.active : {}), ...(l.disabled ? d.disabled : {}) };
  }
  pgLabel(l: PageLink): string {
    if (l.kind === 'page') return String(l.page);
    const k = l.kind === 'more' ? 'more' : l.kind === 'prev' ? 'prev' : 'next';
    return this.view().props?.labels?.[k] || tr('sc.pg.def.' + (this.code.evalOpts().locale ?? 'th') + '.' + k);
  }

  // ---- input
  readonly inputValue = computed(() => {
    const v = this.view();
    const src = v.type === 'input' ? v.props?.value?.trim() : '';
    if (!src) return '';
    const r = evalExpr(src, exprCtx(this.ctx()), this.ctx().opts);
    return r.error ? '' : toText(r.value);
  });
  readonly inputChecked = computed(() => {
    const src = this.view().props?.value?.trim();
    return !!src && truthy(evalExpr(src, exprCtx(this.ctx()), this.ctx().opts).value);
  });
  private readonly optionsReq = computed(() => {
    const v = this.view();
    const f = v.type === 'input' && v.props?.kind === 'select' ? v.props.optionsFrom : undefined;
    return f?.collection ? this.code.sourceRequest(f.collection) : null;
  });
  readonly inputOptions = computed(() => buildInputOptions(this.view().props, this.code.preview(this.optionsReq())?.items));
  /** Formatted text, already whitelisted (same subset as the site). */
  readonly richHtml = computed(() => {
    const r = this.view().props?.rich;
    return r ? this.sanitizer.bypassSecurityTrustHtml(sanitizeRich(bindEscaped(r, this.ctx()))) : null;
  });
  /** Breadcrumb labels for the page being designed (mirrors renderBreadcrumb). */
  readonly crumbs = computed(() => {
    const v = this.view();
    const p = this.ctx().post;
    const out = [v.props?.homeLabel || tr('ed.def.home')];
    if (p && p['isHome'] !== '1') {
      if (p['type'] === 'post' && p['category'] && !v.props?.hideCategory) out.push(String(p['category']));
      if (p['title']) out.push(String(p['title']));
    }
    return out;
  });
  readonly src = computed(() => bind(this.view().props?.src, this.ctx()));
  readonly posts = computed(() => this.store.postsFor(this.view()));
  readonly blockHtml = computed(() => (this.view().type === 'pluginBlock' ? this.store.blockPreview(this.view()) : null));
  readonly blockLabel = computed(() => {
    const b = this.store.blockInfo(this.view());
    return b ? localized(b.name) : (this.view().props?.block || tr('ed.node.pluginBlock'));
  });
  private readonly file = computed(() => (this.view().type === 'download' ? this.store.fileInfo(this.view().props?.mediaId) : undefined));
  readonly fileName = computed(() => this.file()?.filename ?? '');
  readonly fileMeta = computed(() => {
    const f = this.file();
    if (!f) return '';
    const kb = f.size / 1024;
    const size = kb < 1024 ? `${kb.toFixed(0)} KB` : `${(kb / 1024).toFixed(1)} MB`;
    return `${(f.ext || f.filename.split('.').pop() || '').toUpperCase()} · ${size}`;
  });
  readonly editing = computed(() => this.interactive() && this.store.editingTextId() === this.node().id);

  /** Data the layer shows is loaded here (computeds only read it): collection items, plugin block HTML, file names. */
  private readonly loader = effect(() => {
    const v = this.view();
    if (v.type === 'collection') {
      if (isCodeSource(v.props?.source)) this.code.ensure(this.code.requestFor(v));
      else this.store.ensureCollection(v.props?.source);
    } else if (v.type === 'pagination') {
      const col = this.code.collectionNode(v.props?.collection);
      if (col) this.code.ensure(this.code.requestFor(col));
    } else if (v.type === 'input') this.code.ensure(this.optionsReq());
    else if (v.type === 'pluginBlock') this.store.ensureBlockPreview(v);
    else if (v.type === 'download') this.store.ensureMedia();
  });

  /** Which layer this press targets (deep select by default; Ctrl/⌘ flips to Figma level rules). */
  private target(e: PointerEvent): { id: string; deep: boolean } {
    const deep = this.store.deepSelect() !== (e.ctrlKey || e.metaKey);
    return { id: this.store.pickTarget(this.node().id, deep), deep };
  }

  down(e: PointerEvent) {
    if (!this.interactive() || (e.button !== 0 && e.button !== 2)) return; // ghosts/instance internals bubble up
    if (this.store.tool() !== 'move' || this.store.spaceDown()) return; // canvas tools handle it
    e.stopPropagation();
    if (this.editing()) return;
    const s = this.store;
    const { id: target, deep } = this.target(e);
    if (e.button === 2) {
      if (!s.isSelected(target) && !s.selectedAncestorOf(target)) s.select(target);
      return;
    }
    if (e.shiftKey) {
      s.toggleSelect(target);
      return;
    }
    if (!deep) { // Figma level rules
      if (!s.isSelected(target)) s.select(target);
      else s.selectedId.set(target);
      s.nodePointerDown?.(target, e);
      return;
    }
    // Deep select: pressing inside the current selection drags it; a plain click selects the inner layer.
    const held = s.selectedAncestorOf(target);
    if (held) {
      s.selectedId.set(held);
      s.nodePointerDown?.(held, e, held === target ? undefined : { clickSelect: target });
      return;
    }
    // Empty area of a frame: drag = box-select inside it, click = select the frame.
    const n = findNode(s.doc(), target)?.node;
    if (n && isContainer(n) && n.children?.length && target !== s.doc().id) {
      s.nodePointerDown?.(target, e, { marqueeIn: target });
      return;
    }
    s.select(target);
    s.nodePointerDown?.(target, e);
  }

  over(e: PointerEvent) {
    if (!this.interactive()) return;
    e.stopPropagation();
    this.store.hoverId.set(this.target(e).id);
  }

  dbl(e: MouseEvent) {
    if (!this.interactive() || this.store.tool() !== 'move') return;
    e.stopPropagation();
    if (this.view().type === 'pluginBlock') { // double-click a part of a plugin block → edit that part
      // the canvas captures the pointer, so e.target is not the part: look under the pointer
      const el = (document.elementFromPoint(e.clientX, e.clientY) as HTMLElement | null)?.closest('[class]');
      const cls = el && el !== e.currentTarget && (e.currentTarget as HTMLElement).contains(el) ? [...el.classList].find(c => PART_CLASS.test(c) && !c.startsWith('pb-')) : undefined;
      if (cls) { this.store.select(this.node().id); this.store.partPick.set({ id: this.node().id, cls }); return; }
    }
    this.store.drillDown(this.node().id);
  }

  menu(e: MouseEvent) {
    if (!this.interactive()) return;
    e.preventDefault();
    e.stopPropagation();
    this.store.contextMenu.set({ x: e.clientX, y: e.clientY });
  }

  commitText(r: InlineResult) {
    this.store.editingTextId.set(null);
    const p = this.node().props ?? {};
    if (r.text !== (p.text ?? '') || (r.rich ?? '') !== (p.rich ?? '')) this.store.setProps(this.node().id, { text: r.text, rich: r.rich });
  }
}
