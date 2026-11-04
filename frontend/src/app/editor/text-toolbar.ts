import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, signal } from '@angular/core';
import { TPipe } from '../core/i18n';
import { THEME_COLOR_KEYS } from './design-css';
import { findNode } from './design-types';
import { EditorStore } from './editor-store';
import './messages-editor';

/**
 * Floating toolbar over a text layer being edited: bold, italic, underline,
 * strike, theme/custom colors, links and "clear formatting" — so a heading can
 * have one word in another color without splitting it into several layers.
 * Buttons act on the current selection inside the editable text.
 */
@Component({
  selector: 'rea-text-toolbar',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (pos(); as p) {
      <div class="rt-bar" [style.left.px]="p.x" [style.top.px]="p.y" (pointerdown)="keep($event)">
        <button tabindex="-1" [title]="'ed.rt.bold' | t" [attr.aria-label]="'ed.rt.bold' | t" (click)="cmd('bold')"><b>B</b></button>
        <button tabindex="-1" [title]="'ed.rt.italic' | t" [attr.aria-label]="'ed.rt.italic' | t" (click)="cmd('italic')"><i>I</i></button>
        <button tabindex="-1" [title]="'ed.rt.underline' | t" [attr.aria-label]="'ed.rt.underline' | t" (click)="cmd('underline')"><u>U</u></button>
        <button tabindex="-1" [title]="'ed.rt.strike' | t" [attr.aria-label]="'ed.rt.strike' | t" (click)="cmd('strikeThrough')"><s>S</s></button>
        <span class="sep"></span>
        @for (k of colorKeys; track k) {
          <button tabindex="-1" class="rt-sw" [style.background]="store.liveTheme().colors[k]" [title]="('ed.theme.' + k) | t" [attr.aria-label]="('ed.theme.' + k) | t" (click)="color('var(--c-' + k + ')')"></button>
        }
        <label class="rt-custom" [title]="'ed.rt.color' | t">🎨<input type="color" (pointerdown)="save()" (change)="color(val($event))" (blur)="fieldBlur($event)" /></label>
        <span class="sep"></span>
        @if (linking()) {
          <input class="rt-link" placeholder="/about, https://…" [value]="linkUrl()" (input)="linkUrl.set(val($event))" (keydown.enter)="applyLink()" (keydown.escape)="linking.set(false)" (blur)="fieldBlur($event)" />
          <button tabindex="-1" (click)="applyLink()">✓</button>
        } @else {
          <button tabindex="-1" [title]="'ed.rt.link' | t" [attr.aria-label]="'ed.rt.link' | t" (click)="startLink()">🔗</button>
        }
        <button tabindex="-1" [title]="'ed.rt.clear' | t" [attr.aria-label]="'ed.rt.clear' | t" (click)="clear()">⌫</button>
      </div>
    }
  `,
})
export class TextToolbar {
  readonly store = inject(EditorStore);
  readonly colorKeys = THEME_COLOR_KEYS;
  readonly linking = signal(false);
  readonly linkUrl = signal('');
  private range: Range | null = null;
  private tick = signal(0);

  /** Shown above the text layer being edited (text layers only, not buttons). */
  readonly pos = computed(() => {
    this.tick();
    this.store.zoom(); // re-measure after zooming
    this.store.doc(); // …and after the text reflows
    const id = this.store.editingTextId();
    if (!id || findNode(this.store.doc(), id)?.node.type !== 'text') return null;
    const el = document.querySelector(`[data-node-id="${CSS.escape(id)}"]`);
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { x: Math.max(8, Math.min(r.left, innerWidth - 470)), y: Math.max(8, r.top - 46) };
  });

  constructor() {
    // follow the text while the canvas scrolls (any scroll container, captured) or the window resizes
    let frame = 0;
    const bump = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => { frame = 0; this.tick.update(v => v + 1); });
    };
    let listening = false;
    const listen = (on: boolean) => {
      if (on === listening) return;
      listening = on;
      if (on) { document.addEventListener('scroll', bump, true); window.addEventListener('resize', bump); }
      else { document.removeEventListener('scroll', bump, true); window.removeEventListener('resize', bump); }
    };
    effect(() => {
      const editing = !!this.store.editingTextId();
      listen(editing);
      if (editing) {
        this.linking.set(false);
        bump(); // after the editable is in the DOM
      }
    });
    effect(() => { this.store.zoom(); if (this.store.editingTextId()) bump(); }); // zoom animates layout: measure next frame too
    inject(DestroyRef).onDestroy(() => { listen(false); cancelAnimationFrame(frame); });
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }

  /** Toolbar presses keep the text selection (except in the link field). */
  keep(e: PointerEvent) {
    const t = e.target as HTMLElement;
    if (t.closest('input')) return;
    e.preventDefault();
  }

  private editable(): HTMLElement | null {
    return document.querySelector('[reainlineedit]') as HTMLElement | null;
  }

  /** Remembers the selection before a field takes the focus. */
  save() {
    const s = getSelection();
    const ed = this.editable();
    if (s?.rangeCount && ed?.contains(s.getRangeAt(0).commonAncestorContainer)) this.range = s.getRangeAt(0).cloneRange();
  }

  private restore() {
    const ed = this.editable();
    ed?.focus();
    if (this.range) {
      const s = getSelection();
      s?.removeAllRanges();
      s?.addRange(this.range);
    }
  }

  /** Leaving a toolbar field for somewhere else ends the text edit (saving it). */
  fieldBlur(e: FocusEvent) {
    const to = e.relatedTarget as HTMLElement | null;
    if (to?.closest('.rt-bar') || to?.closest('[reainlineedit]')) return;
    this.linking.set(false);
    const ed = this.editable();
    if (!ed) return;
    ed.focus();
    ed.blur();
  }

  cmd(name: string) {
    document.execCommand(name);
  }

  /** Colors the selected words (a theme token or a custom color). */
  color(value: string) {
    if (this.range) this.restore();
    const s = getSelection();
    if (!s?.rangeCount) return;
    const r = s.getRangeAt(0);
    if (r.collapsed || !this.editable()?.contains(r.commonAncestorContainer)) return;
    const span = document.createElement('span');
    span.style.color = value;
    span.appendChild(r.extractContents());
    span.querySelectorAll<HTMLElement>('span').forEach(inner => { // the new color wins over older ones inside
      inner.style.color = '';
      if (!inner.getAttribute('style')) inner.replaceWith(...Array.from(inner.childNodes));
    });
    r.insertNode(span);
    const nr = document.createRange();
    nr.selectNodeContents(span);
    s.removeAllRanges();
    s.addRange(nr);
    this.range = null;
  }

  startLink() {
    this.save();
    const a = (this.range?.commonAncestorContainer as HTMLElement | undefined)?.parentElement?.closest('a');
    this.linkUrl.set(a?.getAttribute('href') ?? '');
    this.linking.set(true);
    queueMicrotask(() => (document.querySelector('.rt-link') as HTMLInputElement | null)?.focus());
  }

  applyLink() {
    const url = this.linkUrl().trim();
    this.linking.set(false);
    this.restore();
    if (url) document.execCommand('createLink', false, url);
    else document.execCommand('unlink');
    this.range = null;
  }

  clear() {
    document.execCommand('removeFormat');
    document.execCommand('unlink');
    const s = getSelection();
    if (!s?.rangeCount) return;
    // removeFormat leaves colored spans: unwrap the ones inside the selection
    const r = s.getRangeAt(0);
    this.editable()?.querySelectorAll('span').forEach(sp => { if (r.intersectsNode(sp)) sp.replaceWith(...Array.from(sp.childNodes)); });
  }
}
