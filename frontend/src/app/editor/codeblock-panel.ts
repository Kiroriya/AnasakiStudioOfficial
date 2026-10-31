import {
  ChangeDetectionStrategy, Component, ElementRef, Injector, OnDestroy, OnInit, afterNextRender, computed, effect, inject, input, output, signal, untracked, viewChild,
} from '@angular/core';
import type * as Monaco from 'monaco-editor';
import { errorMessage } from '../core/api';
import { TPipe } from '../core/i18n';
import { MonacoApi, MonacoLoader } from '../code/monaco-loader';
import { CodeData } from './code-data';
import { DNode } from './design-types';
import { EditorStore } from './editor-store';
import './messages-codeblock';

export type CodeBlockTab = 'html' | 'css' | 'js';
const LANG: Record<CodeBlockTab, string> = { html: 'html', css: 'css', js: 'javascript' };
/** Typing is written to the design after this pause (one undo step per burst). */
const SAVE_DELAY = 300;

/**
 * Inspector of a "Frontend code" block: HTML / CSS / JS in the self-hosted Monaco editor
 * (one model per tab), read-only with a lock hint for users without edit_scripts.
 * JavaScript never runs in the editor — "Run in preview" opens the server-rendered preview.
 */
@Component({
  selector: 'rea-codeblock-panel',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="fg-sec cb-panel" id="codeblock-panel" [class.codeblock-locked]="!canEdit()">
      <h5>{{ 'cb.title' | t }}</h5>
      @if (!canEdit()) { <p class="cb-lock-hint" id="codeblock-locked-hint">🔒 {{ 'cb.locked' | t }}</p> }
      <div class="cb-tabs" role="tablist">
        @for (t of tabs; track t) {
          <button type="button" role="tab" [id]="'codeblock-tab-' + t" [class.on]="tab() === t" [attr.aria-selected]="tab() === t" (click)="pick(t)">
            {{ t.toUpperCase() }}@if (filled()[t]) {<i class="cb-dot"></i>}
          </button>
        }
      </div>
      <div class="cb-editor" #host [attr.data-tab]="tab()" [attr.aria-readonly]="!canEdit()"></div>
      @if (loadError(); as err) { <p class="alert small">{{ 'cb.loadFail' | t }} {{ err }}</p> }
      @else if (!ready()) { <p class="hint">{{ 'cb.loading' | t }}</p> }
      @if (tab() === 'css') { <p class="hint">{{ 'cb.cssHint' | t }}</p> }
      @if (tab() === 'js') { <p class="hint">{{ 'cb.jsHint' | t }}</p> }
      <button type="button" class="btn btn-sm btn-block" id="codeblock-run-preview" (click)="run()">▶ {{ 'cb.run' | t }}</button>
      <p class="hint">{{ 'cb.runHint' | t }}</p>
    </section>
  `,
  styles: `
    .cb-tabs { display: flex; gap: 2px; margin-bottom: 6px; }
    .cb-tabs button { flex: 1; padding: 4px 6px; font: 600 11px/1.4 ui-monospace, monospace; border: 1px solid #e2e8f0; background: #f8fafc; border-radius: 6px; cursor: pointer; color: #475569; }
    .cb-tabs button.on { background: #1e293b; border-color: #1e293b; color: #fff; }
    .cb-dot { display: inline-block; width: 5px; height: 5px; border-radius: 50%; background: currentColor; margin-left: 4px; vertical-align: middle; opacity: .7; }
    .cb-editor { height: 280px; border: 1px solid #e2e8f0; border-radius: 6px; overflow: hidden; margin-bottom: 6px; }
    .codeblock-locked .cb-editor { background: #f8fafc; }
    .cb-lock-hint { margin: 0 0 8px; padding: 6px 8px; border-radius: 6px; background: #f1f5f9; color: #334155; font-size: 12px; }
  `,
})
export class CodeBlockPanel implements OnInit, OnDestroy {
  private store = inject(EditorStore);
  private code = inject(CodeData);
  private loader = inject(MonacoLoader);
  private injector = inject(Injector);
  private host = viewChild<ElementRef<HTMLDivElement>>('host');

  readonly node = input.required<DNode>();
  readonly runPreview = output<void>();

  readonly tabs: CodeBlockTab[] = ['html', 'css', 'js'];
  readonly tab = signal<CodeBlockTab>('html');
  readonly canEdit = computed(() => this.code.codeBlock().edit);
  readonly ready = signal(false);
  readonly loadError = signal('');
  /** Tabs with content (dot on the tab). */
  readonly filled = computed(() => {
    const p = this.node().props ?? {};
    return { html: !!p.html?.trim(), css: !!p.css?.trim(), js: !!p.js?.trim() };
  });

  private m: MonacoApi | null = null;
  private editor: Monaco.editor.IStandaloneCodeEditor | null = null;
  private models = new Map<string, Monaco.editor.ITextModel>();
  private subs: Monaco.IDisposable[] = [];
  private pending = new Map<string, { id: string; tab: CodeBlockTab; timer: ReturnType<typeof setTimeout> }>();
  private destroyed = false;

  constructor() {
    // another block selected, tab switched, or the design changed (undo, collaborator): show it
    effect(() => {
      const n = this.node();
      const tab = this.tab();
      const editable = this.canEdit();
      untracked(() => this.sync(n, tab, editable));
    });
  }

  async ngOnInit() {
    try {
      const m = await this.loader.load();
      if (this.destroyed) return;
      this.m = m;
      afterNextRender(() => this.create(), { injector: this.injector });
    } catch (e) { this.loadError.set(errorMessage(e)); }
  }

  ngOnDestroy() {
    this.destroyed = true;
    this.flush();
    this.subs.forEach(s => s.dispose());
    this.editor?.dispose();
    this.models.forEach(m => m.dispose());
    this.models.clear();
  }

  pick(t: CodeBlockTab) { this.tab.set(t); }

  run() {
    this.flush();
    this.runPreview.emit();
  }

  private create() {
    const host = this.host()?.nativeElement;
    if (!host || !this.m || this.editor || this.destroyed) return;
    this.editor = this.m.editor.create(host, {
      automaticLayout: true, minimap: { enabled: false }, fontSize: 12, tabSize: 2, scrollBeyondLastLine: false, fixedOverflowWidgets: true,
      wordWrap: 'on', lineNumbersMinChars: 3, theme: 'vs', readOnly: !this.canEdit(),
    });
    this.ready.set(true);
    this.sync(this.node(), this.tab(), this.canEdit());
  }

  private key(id: string, tab: CodeBlockTab) { return `${id}.${tab}`; }

  /** The model of a block's tab (created on first use, typing writes back to the design). */
  private model(n: DNode, tab: CodeBlockTab): Monaco.editor.ITextModel {
    const k = this.key(n.id, tab);
    let model = this.models.get(k);
    if (model && !model.isDisposed()) return model;
    const m = this.m!;
    model = m.editor.createModel(n.props?.[tab] ?? '', LANG[tab], m.Uri.parse(`inmemory://codeblock/${encodeURIComponent(n.id)}.${tab === 'js' ? 'js' : tab}`));
    this.models.set(k, model);
    this.subs.push(model.onDidChangeContent(() => this.changed(n.id, tab)));
    return model;
  }

  private sync(n: DNode, tab: CodeBlockTab, editable: boolean) {
    const ed = this.editor;
    if (!ed || !this.m) return;
    ed.updateOptions({ readOnly: !editable });
    const model = this.model(n, tab);
    // outside changes (undo, another tab of the block, collaborators); never while typing in it
    for (const t of this.tabs) {
      const mm = this.models.get(this.key(n.id, t));
      const want = n.props?.[t] ?? '';
      if (mm && !this.pending.has(this.key(n.id, t)) && mm.getValue() !== want) mm.setValue(want);
    }
    if (ed.getModel() !== model) ed.setModel(model);
  }

  private changed(id: string, tab: CodeBlockTab) {
    if (!this.canEdit()) return;
    const k = this.key(id, tab);
    const prev = this.pending.get(k);
    if (prev) clearTimeout(prev.timer);
    this.pending.set(k, { id, tab, timer: setTimeout(() => this.write(k), SAVE_DELAY) });
  }

  private write(k: string) {
    const p = this.pending.get(k);
    if (!p) return;
    clearTimeout(p.timer);
    this.pending.delete(k);
    const model = this.models.get(k);
    if (!model || model.isDisposed() || !this.canEdit()) return;
    const value = model.getValue();
    const n = this.node();
    if (n.id === p.id && (n.props?.[p.tab] ?? '') === value) return;
    this.store.setProps(p.id, p.tab === 'html' ? { html: value } : p.tab === 'css' ? { css: value } : { js: value });
  }

  /** Writes pending typing now (before preview / leaving). */
  private flush() {
    for (const k of [...this.pending.keys()]) this.write(k);
  }
}
