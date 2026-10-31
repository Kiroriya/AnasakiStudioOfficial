import { ChangeDetectionStrategy, Component, ElementRef, OnDestroy, effect, inject, input, output, signal, untracked, viewChild } from '@angular/core';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { Media } from '../core/models';
import { MediaPicker } from './media-picker';
import { safeImageSrc, safeUrl } from './safe-url';

type Block = 'p' | 'h2' | 'h3' | 'h4' | 'blockquote' | 'pre';

const ALLOWED: Record<string, string[]> = {
  p: [], br: [], h1: [], h2: [], h3: [], h4: [], h5: [], h6: [], strong: [], b: [], em: [], i: [], u: [], s: [], strike: [], del: [],
  mark: [], code: [], pre: [], blockquote: [], ul: [], ol: [], li: [], hr: [], sub: [], sup: [],
  a: ['href', 'target', 'rel'], img: ['src', 'alt', 'width', 'height'], figure: [], figcaption: [],
  table: [], thead: [], tbody: [], tr: [], th: [], td: [], span: [], div: [],
};

/** Keeps only safe, semantic markup from pasted content (Word, Google Docs, other sites). */
export function cleanPaste(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const walk = (node: Node): string => {
    if (node.nodeType === Node.TEXT_NODE) return escapeHtml(node.textContent ?? '');
    if (node.nodeType !== Node.ELEMENT_NODE) return '';
    const el = node as HTMLElement;
    const tag = el.tagName.toLowerCase();
    const inner = Array.from(el.childNodes).map(walk).join('');
    if (['script', 'style', 'meta', 'link', 'title', 'iframe', 'object'].includes(tag)) return '';
    if (!(tag in ALLOWED) || tag === 'span' || tag === 'div') return tag === 'div' ? `<p>${inner}</p>` : inner;
    const attrs = ALLOWED[tag]
      .map(a => [a, el.getAttribute(a)] as const)
      .map(([a, v]) => [a, a === 'href' ? safeUrl(v) : a === 'src' ? safeImageSrc(v) : v] as const)
      .filter(([, v]) => v)
      .map(([a, v]) => ` ${a}="${escapeHtml(v!)}"`).join('');
    return ['br', 'hr', 'img'].includes(tag) ? `<${tag}${attrs}>` : `<${tag}${attrs}>${inner}</${tag}>`;
  };
  return Array.from(doc.body.childNodes).map(walk).join('').replace(/<p>\s*<\/p>/g, '');
}

function escapeHtml(s: string) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

/** YouTube / Vimeo / Google Maps link → embeddable player URL, or '' when unsupported. */
export function embedUrl(url: string): string {
  const u = url.trim();
  let m = /(?:youtube\.com\/(?:watch\?v=|shorts\/|embed\/)|youtu\.be\/)([\w-]{6,})/.exec(u);
  if (m) return `https://www.youtube.com/embed/${m[1]}`;
  m = /vimeo\.com\/(?:video\/)?(\d+)/.exec(u);
  if (m) return `https://player.vimeo.com/video/${m[1]}`;
  if (/^https:\/\/(www\.)?google\.com\/maps\/embed/.test(u)) return u;
  return '';
}

const COLORS = ['#111827', '#6b7280', '#dc2626', '#ea580c', '#ca8a04', '#16a34a', '#0891b2', '#2563eb', '#7c3aed', '#db2777'];
const HIGHLIGHTS = ['#fef08a', '#bbf7d0', '#bae6fd', '#fbcfe8', '#fed7aa', '#e9d5ff'];

/**
 * Blog-style rich text editor (like the WordPress / Ghost / Medium editors):
 * block styles, inline formatting, colors, alignment, lists, links, images with
 * size/alignment/caption, video embeds, tables, files, HTML mode, clean paste,
 * word count. The value is HTML.
 */
@Component({
  selector: 'rea-rich-editor',
  imports: [TPipe, MediaPicker],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="rte" [class.rte-full]="full()">
      <div class="rte-bar" role="toolbar" [attr.aria-label]="'rte.toolbar' | t" (mousedown)="keepSelection($event)">
        <button (click)="exec('undo')" [title]="'rte.undo' | t">↶</button>
        <button (click)="exec('redo')" [title]="'rte.redo' | t">↷</button>
        <span class="rte-sep"></span>
        <select class="rte-block" [value]="block()" (change)="setBlock($any($event.target).value)" [title]="'rte.style' | t" [disabled]="htmlMode()">
          <option value="p">{{ 'rte.p' | t }}</option>
          <option value="h2">{{ 'rte.h2' | t }}</option>
          <option value="h3">{{ 'rte.h3' | t }}</option>
          <option value="h4">{{ 'rte.h4' | t }}</option>
          <option value="blockquote">{{ 'rte.quote' | t }}</option>
          <option value="pre">{{ 'rte.code' | t }}</option>
        </select>
        <span class="rte-sep"></span>
        <button [class.on]="state().bold" (click)="exec('bold')" [title]="'rte.bold' | t"><b>B</b></button>
        <button [class.on]="state().italic" (click)="exec('italic')" [title]="'rte.italic' | t"><i>I</i></button>
        <button [class.on]="state().underline" (click)="exec('underline')" [title]="'rte.underline' | t"><u>U</u></button>
        <button [class.on]="state().strike" (click)="exec('strikeThrough')" [title]="'rte.strike' | t"><s>S</s></button>
        <button (click)="wrapInline('code')" [title]="'rte.inlineCode' | t" class="mono">&lt;/&gt;</button>
        <span class="rte-pop">
          <button (click)="toggleMenu('color')" [title]="'rte.color' | t"><span class="rte-a">A</span></button>
          @if (menu() === 'color') {
            <div class="rte-menu rte-swatches">
              <small>{{ 'rte.color' | t }}</small>
              <div>@for (c of colors; track c) { <button class="sw" [style.background]="c" (click)="color('foreColor', c)" [attr.aria-label]="c"></button> }</div>
              <small>{{ 'rte.highlight' | t }}</small>
              <div>@for (c of highlights; track c) { <button class="sw" [style.background]="c" (click)="color('hiliteColor', c)" [attr.aria-label]="c"></button> }
                <button class="sw sw-none" (click)="color('hiliteColor', 'transparent')" [title]="'rte.none' | t">⌀</button></div>
            </div>
          }
        </span>
        <button (click)="exec('removeFormat')" [title]="'rte.clear' | t">T̸</button>
        <span class="rte-sep"></span>
        <button (click)="exec('justifyLeft')" [title]="'rte.left' | t">⇤</button>
        <button (click)="exec('justifyCenter')" [title]="'rte.center' | t">↔</button>
        <button (click)="exec('justifyRight')" [title]="'rte.right' | t">⇥</button>
        <span class="rte-sep"></span>
        <button [class.on]="state().ul" (click)="exec('insertUnorderedList')" [title]="'rte.ul' | t">•≡</button>
        <button [class.on]="state().ol" (click)="exec('insertOrderedList')" [title]="'rte.ol' | t">1≡</button>
        <button (click)="indent(-1)" [title]="'rte.outdent' | t">⇠</button>
        <button (click)="indent(1)" [title]="'rte.indent' | t">⇢</button>
        <span class="rte-sep"></span>
        <button (click)="link()" [title]="('rte.link' | t) + ' (Ctrl+K)'">🔗</button>
        <button (click)="exec('unlink')" [title]="'rte.unlink' | t">⛓̸</button>
        <span class="rte-pop">
          <button class="rte-insert" (click)="toggleMenu('insert')">＋ {{ 'rte.insert' | t }}</button>
          @if (menu() === 'insert') {
            <div class="rte-menu rte-insert-menu">
              <button (click)="pick('image')">🖼 {{ 'rte.image' | t }}</button>
              <button (click)="video()">▶️ {{ 'rte.video' | t }}</button>
              <button (click)="pick('file')">📎 {{ 'rte.file' | t }}</button>
              <button (click)="table()">▦ {{ 'rte.table' | t }}</button>
              <button (click)="insertBlock('<hr>')">― {{ 'rte.hr' | t }}</button>
              <button (click)="callout()">💡 {{ 'rte.callout' | t }}</button>
            </div>
          }
        </span>
        <span class="spacer"></span>
        <button [class.on]="htmlMode()" (click)="toggleHtml()" [title]="'rte.html' | t">HTML</button>
        <button [class.on]="full()" (click)="full.set(!full())" [title]="'rte.full' | t">⛶</button>
      </div>

      @if (htmlMode()) {
        <textarea class="rte-html mono" [value]="html()" (input)="setFromHtml($any($event.target).value)"></textarea>
      } @else {
        <div #body class="rte-body rea-content" [class.is-empty]="!html()" contenteditable="true" [attr.data-ph]="placeholder() || ('rte.placeholder' | t)"
             (input)="normalizeSoon(); sync()" (blur)="sync()" (focus)="normalize()" (keyup)="refresh()" (mouseup)="refresh()" (paste)="paste($event)"
             (keydown)="key($event)" (click)="clickImage($event)"></div>
        @if (img(); as im) {
          <div class="rte-imgbar" [style.top.px]="im.top" [style.left.px]="im.left">
            @for (w of widths; track w) { <button [class.on]="im.width === w" (click)="imgWidth(w)">{{ w }}%</button> }
            <span class="rte-sep"></span>
            <button [class.on]="im.align === 'left'" (click)="imgAlign('left')" [title]="'rte.wrapLeft' | t">◧</button>
            <button [class.on]="im.align === 'center'" (click)="imgAlign('center')" [title]="'rte.center' | t">▣</button>
            <button [class.on]="im.align === 'right'" (click)="imgAlign('right')" [title]="'rte.wrapRight' | t">◨</button>
            <span class="rte-sep"></span>
            <button (click)="imgAlt()">Alt</button>
            <button (click)="imgCaption()">{{ 'rte.caption' | t }}</button>
            <button class="danger" (click)="imgRemove()">🗑</button>
          </div>
        }
      }
      <div class="rte-foot">
        <span>{{ 'rte.words' | t: { n: words() } }}</span>
        <span>·</span>
        <span>{{ 'rte.read' | t: { n: readMinutes() } }}</span>
        <span class="spacer"></span>
        <span class="muted">{{ 'rte.pasteHint' | t }}</span>
      </div>
    </div>
    <rea-media-picker [kind]="picking() === 'file' ? '' : 'image'" [open]="picking() !== null" (openChange)="!$event && picking.set(null)" (picked)="picked($event)" />
  `,
  host: { '(document:click)': 'outside($event)' },
})
export class RichEditor implements OnDestroy {
  private i18n = inject(I18n);
  private host = inject<ElementRef<HTMLElement>>(ElementRef);
  private body = viewChild<ElementRef<HTMLElement>>('body');

  readonly value = input('');
  readonly placeholder = input('');
  readonly valueChange = output<string>();

  readonly colors = COLORS;
  readonly highlights = HIGHLIGHTS;
  readonly widths = [25, 50, 75, 100];
  readonly html = signal('');
  readonly htmlMode = signal(false);
  readonly full = signal(false);
  readonly menu = signal<'color' | 'insert' | null>(null);
  readonly picking = signal<'image' | 'file' | null>(null);
  readonly block = signal<Block>('p');
  readonly state = signal({ bold: false, italic: false, underline: false, strike: false, ul: false, ol: false });
  readonly img = signal<{ el: HTMLImageElement; top: number; left: number; width: number; align: string } | null>(null);
  readonly words = signal(0);
  readonly readMinutes = signal(1);
  /** Last caret/selection inside the editor, tracked live so toolbar actions apply where the user is. */
  private savedRange: Range | null = null;
  private readonly onSelection = () => {
    const sel = getSelection();
    const el = this.body()?.nativeElement;
    if (sel && sel.rangeCount && el && el.contains(sel.anchorNode)) this.savedRange = sel.getRangeAt(0).cloneRange();
  };
  private lastEmitted: string | null = null;

  constructor() {
    document.addEventListener('selectionchange', this.onSelection);
    // External value (load, revision restore) → editor; our own edits are not re-applied.
    effect(() => {
      const v = this.value();
      untracked(() => {
        if (v === this.lastEmitted) return;
        this.html.set(v);
        this.render(v);
        this.count(v);
      });
    });
  }

  ngOnDestroy() {
    document.removeEventListener('selectionchange', this.onSelection);
    document.body.classList.remove('rte-noscroll');
  }

  private render(v: string) {
    queueMicrotask(() => {
      const el = this.body()?.nativeElement;
      if (!el) return;
      if (el.innerHTML !== v) el.innerHTML = v;
      this.normalize();
    });
  }

  /**
   * Keeps the document a clean list of blocks: loose text and <div>s become <p>,
   * blocks never sit inside paragraphs, the editor never ends on a figure/table
   * (so there is always a line to type on), and figures/videos are locked so the
   * caret cannot wander into them (captions stay editable).
   */
  normalize() {
    const el = this.body()?.nativeElement;
    if (!el) return;
    document.execCommand('defaultParagraphSeparator', false, 'p');
    const BLOCK = /^(P|H[1-6]|UL|OL|BLOCKQUOTE|PRE|FIGURE|TABLE|HR)$/;
    for (const n of Array.from(el.childNodes)) {
      if (n.nodeType === Node.TEXT_NODE && !n.textContent?.trim()) { if (!n.textContent) n.remove(); continue; }
      const tag = (n as HTMLElement).tagName;
      if (tag === 'DIV') {
        // unwrap blocks that ended up in a div; plain divs become paragraphs
        const div = n as HTMLElement;
        if (Array.from(div.children).some(c => BLOCK.test(c.tagName))) div.replaceWith(...Array.from(div.childNodes));
        else { const p = document.createElement('p'); p.append(...Array.from(div.childNodes)); div.replaceWith(p); }
      } else if (!tag || tag === 'BR' || !BLOCK.test(tag)) {
        // inline content at the top level → wrap with its inline neighbours in a <p>
        const p = document.createElement('p');
        n.parentNode!.insertBefore(p, n);
        let cur: ChildNode | null = n;
        while (cur && (cur.nodeType === Node.TEXT_NODE || !BLOCK.test((cur as HTMLElement).tagName ?? '') && (cur as HTMLElement).tagName !== 'DIV')) {
          const next: ChildNode | null = cur.nextSibling;
          p.appendChild(cur);
          cur = next;
        }
      }
    }
    for (const list of Array.from(el.querySelectorAll('p > ul, p > ol, h1 > ul, h2 > ul, h3 > ul, h4 > ul, h1 > ol, h2 > ol, h3 > ol, h4 > ol'))) {
      const parent = list.parentElement!;
      parent.replaceWith(...Array.from(parent.childNodes));
    }
    for (const p of Array.from(el.querySelectorAll('p'))) if (!p.textContent && !p.querySelector('img,br,iframe')) p.innerHTML = '<br>';
    const last = el.lastElementChild;
    if (!last || /^(FIGURE|TABLE|HR|PRE|BLOCKQUOTE)$/.test(last.tagName)) el.insertAdjacentHTML('beforeend', '<p><br></p>');
    for (const fig of Array.from(el.querySelectorAll('figure'))) {
      fig.setAttribute('contenteditable', 'false');
      fig.querySelector('figcaption')?.setAttribute('contenteditable', 'true');
    }
  }

  /** The editor HTML without editing-only attributes. */
  private cleanHtml(): string {
    const el = this.body()?.nativeElement;
    if (!el) return '';
    const copy = el.cloneNode(true) as HTMLElement;
    copy.querySelectorAll('[contenteditable]').forEach(x => x.removeAttribute('contenteditable'));
    const h = copy.innerHTML.replace(/<p><br><\/p>$/, '');
    return h === '<br>' || h === '<p><br></p>' ? '' : h;
  }

  /** Current HTML (call before saving). */
  current(): string {
    if (!this.htmlMode()) this.sync();
    return this.html();
  }

  sync() {
    const el = this.body()?.nativeElement;
    if (!el) return;
    const v = this.cleanHtml();
    this.html.set(v);
    this.emit(v);
  }

  private normTimer?: ReturnType<typeof setTimeout>;
  normalizeSoon() {
    clearTimeout(this.normTimer);
    this.normTimer = setTimeout(() => { this.saveRange(); this.normalize(); this.restoreRange(); }, 400);
  }

  setFromHtml(v: string) {
    this.html.set(v);
    this.emit(v);
  }

  private emit(v: string) {
    this.lastEmitted = v;
    this.valueChange.emit(v);
    this.count(v);
  }

  private count(v: string) {
    const text = v.replace(/<[^>]+>/g, ' ').replace(/&nbsp;/g, ' ');
    // Thai has no spaces between words: count ~6 Thai characters as a word.
    const thai = (text.match(/[฀-๿]/g) ?? []).length;
    const latin = text.replace(/[฀-๿]/g, ' ').split(/\s+/).filter(w => /\w/.test(w)).length;
    const n = latin + Math.round(thai / 6);
    this.words.set(n);
    this.readMinutes.set(Math.max(1, Math.round(n / 200)));
  }

  // ---------------------------------------------------------------- selection helpers

  private focus() {
    const el = this.body()?.nativeElement;
    if (!el) return;
    const sel = getSelection();
    const inside = !!sel && sel.rangeCount > 0 && el.contains(sel.anchorNode);
    el.focus();
    if (!inside && this.savedRange && el.contains(this.savedRange.startContainer)) {
      sel?.removeAllRanges();
      sel?.addRange(this.savedRange);
    }
  }

  /** Toolbar buttons must not steal the caret from the text (the style dropdown still opens). */
  keepSelection(e: MouseEvent) {
    const t = e.target as HTMLElement;
    if (!t.closest('select, input')) e.preventDefault();
  }

  private restoreRange() {
    if (!this.savedRange) return;
    const s = getSelection();
    s?.removeAllRanges();
    s?.addRange(this.savedRange);
  }

  private saveRange() {
    const s = getSelection();
    const el = this.body()?.nativeElement;
    if (s && s.rangeCount && el?.contains(s.anchorNode)) this.savedRange = s.getRangeAt(0).cloneRange();
  }

  exec(cmd: string, arg?: string) {
    if (this.htmlMode()) return;
    this.focus();
    document.execCommand('styleWithCSS', false, 'false');
    document.execCommand(cmd, false, arg);
    this.normalize();
    this.sync();
    this.refresh();
  }

  refresh() {
    const q = (c: string) => { try { return document.queryCommandState(c); } catch { return false; } };
    this.state.set({ bold: q('bold'), italic: q('italic'), underline: q('underline'), strike: q('strikeThrough'), ul: q('insertUnorderedList'), ol: q('insertOrderedList') });
    let n: Node | null = getSelection()?.anchorNode ?? null;
    const root = this.body()?.nativeElement;
    let b: Block = 'p';
    while (n && n !== root) {
      const t = (n as HTMLElement).tagName?.toLowerCase();
      if (t && ['h2', 'h3', 'h4', 'blockquote', 'pre'].includes(t)) { b = t as Block; break; }
      n = n.parentNode;
    }
    this.block.set(b);
  }

  setBlock(b: Block) {
    this.focus(); // restores the caret saved when the dropdown was clicked
    document.execCommand('formatBlock', false, `<${b}>`);
    this.sync();
    this.refresh();
  }

  toggleMenu(m: 'color' | 'insert') {
    this.saveRange();
    this.menu.set(this.menu() === m ? null : m);
  }

  outside(e: MouseEvent) {
    const t = e.target as HTMLElement;
    if (!t.closest('.rte-pop')) this.menu.set(null);
    if (!t.closest('.rte-imgbar') && !(t instanceof HTMLImageElement && this.host.nativeElement.contains(t))) this.img.set(null);
  }

  color(cmd: 'foreColor' | 'hiliteColor', c: string) {
    this.menu.set(null);
    this.focus();
    document.execCommand('styleWithCSS', false, 'true');
    document.execCommand(cmd, false, c);
    document.execCommand('styleWithCSS', false, 'false');
    this.sync();
  }

  wrapInline(tag: string) {
    this.focus();
    const s = getSelection();
    if (!s || !s.rangeCount || s.isCollapsed) return;
    const text = s.toString();
    document.execCommand('insertHTML', false, `<${tag}>${escapeHtml(text)}</${tag}>`);
    this.sync();
  }

  insert(html: string) {
    this.menu.set(null);
    if (this.htmlMode()) { this.setFromHtml(this.html() + html); return; }
    this.focus();
    document.execCommand('insertHTML', false, html);
    this.normalize();
    this.sync();
  }

  /** Inserts block content (image, table, video…) after the current paragraph, then a new empty line. */
  insertBlock(html: string) {
    this.menu.set(null);
    if (this.htmlMode()) { this.setFromHtml(this.html() + html); return; }
    const el = this.body()?.nativeElement;
    if (!el) return;
    this.focus();
    let anchor: Node | null = getSelection()?.anchorNode ?? null;
    while (anchor && anchor.parentNode !== el) anchor = anchor.parentNode;
    const tpl = document.createElement('template');
    tpl.innerHTML = html + '<p><br></p>';
    const nodes = Array.from(tpl.content.childNodes);
    const empty = anchor instanceof HTMLElement && anchor.tagName === 'P' && !anchor.textContent?.trim() && !anchor.querySelector('img');
    if (anchor && empty) (anchor as ChildNode).replaceWith(...nodes);
    else if (anchor) (anchor as ChildNode).after(...nodes);
    else el.append(...nodes);
    const line = nodes[nodes.length - 1];
    const r = document.createRange();
    r.setStart(line, 0);
    r.collapse(true);
    const sel = getSelection();
    sel?.removeAllRanges();
    sel?.addRange(r);
    this.normalize();
    this.sync();
  }

  /** Indent / outdent: lists nest; other blocks get a left margin (2em steps). */
  indent(dir: 1 | -1) {
    this.focus();
    const el = this.body()?.nativeElement;
    let n: Node | null = getSelection()?.anchorNode ?? null;
    let inList = false;
    for (let x = n; x && x !== el; x = x.parentNode) if ((x as HTMLElement).tagName === 'LI') inList = true;
    if (inList) { document.execCommand(dir > 0 ? 'indent' : 'outdent'); this.sync(); return; }
    while (n && n.parentNode !== el) n = n.parentNode;
    if (!(n instanceof HTMLElement)) return;
    const cur = parseFloat(n.style.marginLeft) || 0;
    const next = Math.max(0, Math.min(8, cur + dir * 2));
    n.style.marginLeft = next ? `${next}em` : '';
    if (!n.getAttribute('style')) n.removeAttribute('style');
    this.sync();
  }

  key(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.code === 'KeyK') { e.preventDefault(); this.link(); }
  }

  paste(e: ClipboardEvent) {
    const html = e.clipboardData?.getData('text/html');
    const text = e.clipboardData?.getData('text/plain') ?? '';
    e.preventDefault();
    const embed = !html && embedUrl(text);
    if (embed) { this.insertBlock(videoHtml(embed)); return; }
    document.execCommand('insertHTML', false, html ? cleanPaste(html) : escapeHtml(text).replace(/\n{2,}/g, '</p><p>').replace(/\n/g, '<br>'));
    this.sync();
  }

  // ---------------------------------------------------------------- insertions

  async link() {
    this.saveRange();
    let url = await Dialog.prompt({ title: this.i18n.t('rte.linkPrompt'), placeholder: 'https://', required: true });
    if (!url) return;
    const safe = safeUrl(url);
    if (!safe) { Dialog.error(this.i18n.t('url.unsafe')); return; }
    url = safe;
    this.focus();
    const sel = getSelection();
    if (sel && sel.isCollapsed) document.execCommand('insertHTML', false, `<a href="${escapeHtml(url)}">${escapeHtml(url)}</a>`);
    else document.execCommand('createLink', false, url);
    this.sync();
  }

  pick(kind: 'image' | 'file') {
    this.menu.set(null);
    this.picking.set(kind);
  }

  picked(m: Media) {
    const kind = this.picking();
    this.picking.set(null);
    if (kind === 'file') {
      this.insertBlock(`<p><a class="file-download" href="/download/${m.id}">📎 ${escapeHtml(m.filename)}</a></p>`);
    } else {
      this.insertBlock(`<figure class="aligncenter"><img src="${escapeHtml(m.url)}" alt="${escapeHtml(m.alt)}"></figure>`);
    }
  }

  async video() {
    this.menu.set(null);
    const url = await Dialog.prompt({ title: this.i18n.t('rte.videoPrompt'), placeholder: 'https://www.youtube.com/watch?v=…', required: true });
    if (!url) return;
    const src = embedUrl(url);
    if (!src) { Dialog.error(this.i18n.t('rte.videoBad')); return; }
    this.insertBlock(videoHtml(src));
  }

  table() {
    const cell = '<td><br></td>';
    const row = `<tr>${cell.repeat(3)}</tr>`;
    this.insertBlock(`<table><thead><tr><th>${this.i18n.t('rte.col')} 1</th><th>${this.i18n.t('rte.col')} 2</th><th>${this.i18n.t('rte.col')} 3</th></tr></thead><tbody>${row}${row}</tbody></table>`);
  }

  callout() {
    this.insertBlock(`<blockquote class="callout">💡 ${escapeHtml(this.i18n.t('rte.calloutText'))}</blockquote>`);
  }

  // ---------------------------------------------------------------- image toolbar

  clickImage(e: MouseEvent) {
    const t = e.target as HTMLElement;
    if (!(t instanceof HTMLImageElement)) { this.img.set(null); return; }
    const box = this.host.nativeElement.querySelector('.rte')!.getBoundingClientRect();
    const r = t.getBoundingClientRect();
    const fig = t.closest('figure');
    const align = fig?.classList.contains('alignleft') ? 'left' : fig?.classList.contains('alignright') ? 'right' : 'center';
    const width = Number((fig?.style.width || t.style.width || '100').replace('%', '')) || 100;
    this.img.set({ el: t, top: r.top - box.top - 44, left: Math.max(8, r.left - box.left), width, align });
  }

  private figure(img: HTMLImageElement): HTMLElement {
    let fig = img.closest('figure') as HTMLElement | null;
    if (!fig) {
      fig = document.createElement('figure');
      fig.className = 'aligncenter';
      img.replaceWith(fig);
      fig.appendChild(img);
    }
    return fig;
  }

  imgWidth(w: number) {
    const im = this.img();
    if (!im) return;
    const fig = this.figure(im.el);
    fig.style.width = w === 100 ? '' : `${w}%`;
    im.el.style.width = '';
    this.img.set({ ...im, width: w });
    this.sync();
  }

  imgAlign(a: 'left' | 'center' | 'right') {
    const im = this.img();
    if (!im) return;
    const fig = this.figure(im.el);
    fig.classList.remove('alignleft', 'aligncenter', 'alignright');
    fig.classList.add('align' + a);
    if (a !== 'center' && (!fig.style.width || fig.style.width === '100%')) fig.style.width = '50%';
    this.img.set({ ...im, align: a, width: Number(fig.style.width.replace('%', '')) || 100 });
    this.sync();
  }

  async imgAlt() {
    const im = this.img();
    if (!im) return;
    const alt = await Dialog.prompt({ title: this.i18n.t('rte.altPrompt'), value: im.el.alt, required: false });
    if (alt === null) return;
    im.el.alt = alt;
    this.sync();
  }

  imgCaption() {
    const im = this.img();
    if (!im) return;
    const fig = this.figure(im.el);
    let cap = fig.querySelector('figcaption');
    if (cap) cap.remove();
    else {
      cap = document.createElement('figcaption');
      cap.textContent = this.i18n.t('rte.captionText');
      cap.setAttribute('contenteditable', 'true');
      fig.appendChild(cap);
    }
    this.sync();
  }

  imgRemove() {
    const im = this.img();
    if (!im) return;
    (im.el.closest('figure') ?? im.el).remove();
    this.img.set(null);
    this.sync();
  }

  toggleHtml() {
    if (!this.htmlMode()) this.sync();
    this.htmlMode.update(v => !v);
    this.img.set(null);
    if (!this.htmlMode()) this.render(this.html());
  }
}

function videoHtml(src: string) {
  return `<figure class="video"><iframe src="${escapeHtml(src)}" loading="lazy" allowfullscreen allow="autoplay; encrypted-media; picture-in-picture"></iframe></figure>`;
}
