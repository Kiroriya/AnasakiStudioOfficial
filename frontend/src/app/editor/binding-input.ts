import { ChangeDetectionStrategy, Component, ElementRef, computed, effect, inject, input, output, signal, untracked, viewChild } from '@angular/core';
import { TPipe, tr } from '../core/i18n';
import { CodeData } from './code-data';
import { Suggestion, applySuggestion, suggest } from './code-logic';
import { ExprError, parseExpr, templateErrors } from './expr';
import './messages-sitecode';

/** Readable message of an expression parse error. */
export function exprErrorText(e: ExprError): string {
  return tr('sc.expr.error', { msg: tr('sc.expr.err.' + e.code, { token: e.token ?? '', pos: e.pos + 1 }) });
}

/**
 * Text field with binding autocomplete: item fields of the enclosing
 * collection, page.* keys from the preview, post / site / query keys, and
 * formatter names after `|`. mode 'template' = text with {{ }} bindings,
 * 'expr' = a bare expression. Parse errors show under the field.
 */
@Component({
  selector: 'rea-binding-input',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="bi" [class.bi-err]="!!error()">
      @if (multiline()) {
        <textarea #el class="bi-el" [attr.id]="inputId() || null" [rows]="rows()" [value]="draft()" [placeholder]="placeholder()" spellcheck="false"
                  (input)="typed($event)" (click)="typed($event)" (keyup)="caretMoved($event)" (keydown)="key($event)" (blur)="blur()" (focus)="focused.set(true)"></textarea>
      } @else {
        <input #el class="bi-el" [attr.id]="inputId() || null" [value]="draft()" [placeholder]="placeholder()" spellcheck="false" autocomplete="off"
               [class.mono]="mode() === 'expr'" (input)="typed($event)" (click)="typed($event)" (keyup)="caretMoved($event)" (keydown)="key($event)" (blur)="blur()" (focus)="focused.set(true)" />
      }
      @if (open() && suggestions(); as r) {
        <ul class="bi-list" role="listbox">
          @for (s of r.items; track s.kind + s.label; let i = $index) {
            <li class="bi-opt" role="option" [class.on]="i === active()" [attr.data-value]="s.insert" (mousedown)="$event.preventDefault(); pick(s)">
              <b>{{ s.label }}</b>
              @if (s.detail) { <small>{{ s.kind === 'root' ? (s.detail | t) : s.detail }}</small> }
            </li>
          }
        </ul>
      }
      @if (error(); as e) { <p class="bi-error">{{ e }}</p> }
    </div>
  `,
  styles: [`
    :host { display: block; position: relative; }
    .bi-el { width: 100%; box-sizing: border-box; }
    .mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 11.5px; }
    .bi-err .bi-el { border-color: #dc2626 !important; }
    .bi-list { position: absolute; left: 0; right: 0; z-index: 40; margin: 2px 0 0; padding: 3px; list-style: none; background: #fff; border-radius: 8px;
      box-shadow: 0 6px 24px rgba(0,0,0,.18), 0 0 0 1px rgba(0,0,0,.06); max-height: 220px; overflow: auto; }
    .bi-opt { display: flex; justify-content: space-between; gap: 8px; padding: 4px 8px; border-radius: 5px; cursor: pointer; font-size: 12px; }
    .bi-opt b { font-weight: 600; font-family: ui-monospace, monospace; font-size: 11.5px; }
    .bi-opt small { color: #6b7280; font-size: 10.5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .bi-opt.on, .bi-opt:hover { background: #e0f2fe; }
    .bi-error { margin: 3px 0 0; color: #b91c1c; font-size: 11px; }
  `],
})
export class BindingInput {
  private code = inject(CodeData);
  readonly value = input<string | undefined>('');
  readonly mode = input<'template' | 'expr'>('template');
  /** Layer the text belongs to (decides whether item.* is offered). */
  readonly nodeId = input<string | null | undefined>(null);
  /** Page context only (collection params are evaluated outside the item loop). */
  readonly pageOnly = input(false);
  readonly multiline = input(false);
  readonly rows = input(3);
  readonly placeholder = input('');
  readonly inputId = input('');
  readonly changed = output<string>();

  private readonly el = viewChild<ElementRef<HTMLInputElement | HTMLTextAreaElement>>('el');
  readonly draft = signal('');
  readonly caret = signal(0);
  readonly focused = signal(false);
  readonly open = signal(false);
  readonly active = signal(0);

  constructor() {
    // follow outside changes (undo, other panels) while the field is not being typed in
    effect(() => {
      const v = this.value() ?? '';
      untracked(() => { if (!this.focused()) this.draft.set(v); });
    });
  }

  readonly suggestions = computed(() => {
    if (!this.focused()) return null;
    return suggest(this.draft(), this.caret(), this.mode(), this.code.suggestSource(this.nodeId(), { pageOnly: this.pageOnly() }));
  });

  readonly error = computed(() => {
    const t = this.draft();
    if (this.mode() === 'expr') {
      if (!t.trim()) return '';
      const p = parseExpr(t.trim());
      return p.ok ? '' : exprErrorText(p.error);
    }
    const e = templateErrors(t)[0];
    return e ? exprErrorText(e.error) : '';
  });

  typed(e: Event) {
    const el = e.target as HTMLInputElement;
    this.draft.set(el.value);
    this.caret.set(el.selectionStart ?? el.value.length);
    this.open.set(true);
    this.active.set(0);
  }

  caretMoved(e: KeyboardEvent) {
    if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) {
      const el = e.target as HTMLInputElement;
      this.caret.set(el.selectionStart ?? 0);
    }
  }

  key(e: KeyboardEvent) {
    e.stopPropagation(); // editor shortcuts stay out of the field
    const r = this.open() ? this.suggestions() : null;
    if (r?.items.length) {
      if (e.key === 'ArrowDown') { e.preventDefault(); this.active.set((this.active() + 1) % r.items.length); return; }
      if (e.key === 'ArrowUp') { e.preventDefault(); this.active.set((this.active() - 1 + r.items.length) % r.items.length); return; }
      if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); this.pick(r.items[Math.min(this.active(), r.items.length - 1)]); return; }
      if (e.key === 'Escape') { e.preventDefault(); this.open.set(false); return; }
    }
    if (e.key === 'Enter' && !this.multiline()) { e.preventDefault(); this.commit(); }
  }

  pick(s: Suggestion) {
    const r = this.suggestions();
    if (!r) return;
    const out = applySuggestion(this.draft(), r, s);
    this.draft.set(out.text);
    this.caret.set(out.caret);
    this.active.set(0);
    const el = this.el()?.nativeElement;
    if (el) {
      el.value = out.text;
      el.focus();
      el.setSelectionRange(out.caret, out.caret);
    }
    // a root or a formatter usually continues (".field", ":arg"); a field completes the path
    this.open.set(s.kind === 'root');
  }

  blur() {
    this.focused.set(false);
    this.open.set(false);
    this.commit();
  }

  private commit() {
    if (this.draft() !== (this.value() ?? '')) this.changed.emit(this.draft());
  }
}
