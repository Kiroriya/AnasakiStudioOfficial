import { ChangeDetectionStrategy, Component, ElementRef, afterNextRender, computed, inject, output, signal, viewChild } from '@angular/core';
import { TPipe, tr } from '../core/i18n';
import { EditorStore } from './editor-store';
import './messages-find';

/** Find & replace text on the page (Figma Ctrl+F). */
@Component({
  selector: 'rea-find-replace',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'fr', '(keydown.escape)': 'closed.emit()' },
  template: `
    <div class="fr-head"><b>🔎 {{ 'fr.title' | t }}</b><button class="fr-x" (click)="closed.emit()" aria-label="close">✕</button></div>
    <input #q class="fr-in" [placeholder]="'fr.find' | t" [value]="query()" (input)="query.set($any($event.target).value)" (keydown.enter)="next()" />
    <input class="fr-in" [placeholder]="'fr.replace' | t" [value]="by()" (input)="by.set($any($event.target).value)" />
    <label class="fr-chk"><input type="checkbox" [checked]="caseSensitive()" (change)="caseSensitive.set($any($event.target).checked)" /> Aa {{ 'fr.case' | t }}</label>
    <div class="fr-act">
      <small>{{ query() ? ('fr.count' | t: { n: hits().length }) : '' }}</small>
      <button class="btn btn-sm" [disabled]="!hits().length" (click)="replaceOne()">{{ 'fr.one' | t }}</button>
      <button class="btn btn-sm btn-primary" [disabled]="!hits().length" (click)="replaceAll()">{{ 'fr.all' | t }}</button>
    </div>
    <ul class="fr-list">
      @for (h of hits(); track h.id; let i = $index) {
        <li [class.on]="store.selectedId() === h.id" (click)="go(h.id)"><b>{{ h.name }}</b><span>{{ h.text }}</span></li>
      }
    </ul>
  `,
  styles: [`
    :host { position: absolute; top: 12px; right: 12px; z-index: 40; width: 300px; background: #fff; border: 1px solid #e5e7eb; border-radius: 12px; box-shadow: 0 12px 32px rgba(0,0,0,.15); padding: 10px; display: flex; flex-direction: column; gap: 6px; font-size: 12px; }
    .fr-head { display: flex; justify-content: space-between; align-items: center; }
    .fr-x { border: 0; background: none; cursor: pointer; font-size: 14px; }
    .fr-in { width: 100%; font: inherit; font-size: 13px; padding: 6px 8px; border: 1px solid #e5e7eb; border-radius: 8px; box-sizing: border-box; }
    .fr-chk { display: flex; align-items: center; gap: 6px; color: #6b7280; }
    .fr-act { display: flex; align-items: center; gap: 6px; } .fr-act small { flex: 1; color: #6b7280; }
    .fr-list { list-style: none; margin: 0; padding: 0; max-height: 260px; overflow: auto; }
    .fr-list li { padding: 6px 8px; border-radius: 6px; cursor: pointer; display: flex; flex-direction: column; gap: 2px; }
    .fr-list li:hover, .fr-list li.on { background: #f5f3ff; }
    .fr-list b { font-size: 11px; color: #6b7280; font-weight: 600; }
    .fr-list span { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  `],
})
export class FindReplace {
  readonly store = inject(EditorStore);
  readonly closed = output<void>();
  private input = viewChild<ElementRef<HTMLInputElement>>('q');
  readonly query = signal('');
  readonly by = signal('');
  readonly caseSensitive = signal(false);
  readonly hits = computed(() => this.store.findText(this.query(), this.caseSensitive()));

  constructor() { afterNextRender(() => this.input()?.nativeElement.focus()); }

  go(id: string) { this.store.select(id); }

  next() {
    const h = this.hits();
    if (!h.length) return;
    const i = h.findIndex(x => x.id === this.store.selectedId());
    this.go(h[(i + 1) % h.length].id);
  }

  replaceOne() {
    const h = this.hits();
    const id = h.find(x => x.id === this.store.selectedId())?.id ?? h[0]?.id;
    if (id && this.store.replaceText([id], this.query(), this.by(), this.caseSensitive())) this.next();
  }

  replaceAll() {
    const n = this.store.replaceText(this.hits().map(h => h.id), this.query(), this.by(), this.caseSensitive());
    this.store.notify(tr('fr.replaced', { n }));
  }
}
