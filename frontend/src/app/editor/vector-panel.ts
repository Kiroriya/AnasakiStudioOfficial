import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { TPipe } from '../core/i18n';
import { DNode, Props } from './design-types';
import { EditorStore } from './editor-store';
import { ExportFormat, copyAsPng, exportName, exportNode, svgExportable } from './export';
import { BOOL_OPS, BoolOp, booleanOp, canBoolean, canFlatten, flattenSelection } from './vector-ops';
import './messages-vector';

/**
 * Design-tab sections for vectors: boolean operations (2+ shapes), vector
 * stroke options / edit path / flatten, and the Figma-style Export section.
 */
@Component({
  selector: 'rea-vector-panel',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (bool()) {
      <section class="fg-sec">
        <h5>{{ 'vec.boolean' | t }}</h5>
        <div class="vp-row">
          @for (b of ops; track b.op) {
            <button class="vp-btn" [attr.data-op]="b.op" [title]="('vec.op.' + b.op | t) + ' (' + b.keys + ')'" (click)="run(b.op)">
              <span class="vp-ic">{{ b.icon }}</span>{{ 'vec.op.' + b.op | t }}</button>
          }
        </div>
      </section>
    }
    @if (node(); as n) {
      @if (n.type === 'vector') {
        <section class="fg-sec">
          <h5>{{ 'vec.title' | t }}
            <button class="vp-btn vp-edit" [class.on]="editing()" (click)="toggleEdit(n)">✎ {{ (editing() ? 'vec.done' : 'vec.editPath') | t }}</button>
          </h5>
          @if (n.style?.stroke) {
            <div class="vp-row">
              <label class="vp-lbl">{{ 'vec.cap' | t }}
                <select [value]="n.props?.strokeCap ?? 'butt'" (change)="props({ strokeCap: val($event) })">
                  <option value="butt">{{ 'vec.capButt' | t }}</option><option value="round">{{ 'vec.capRound' | t }}</option><option value="square">{{ 'vec.capSquare' | t }}</option>
                </select></label>
              <label class="vp-lbl">{{ 'vec.join' | t }}
                <select [value]="n.props?.strokeJoin ?? 'miter'" (change)="props({ strokeJoin: val($event) })">
                  <option value="miter">{{ 'vec.joinMiter' | t }}</option><option value="round">{{ 'vec.joinRound' | t }}</option><option value="bevel">{{ 'vec.joinBevel' | t }}</option>
                </select></label>
            </div>
          }
          <label class="vp-check"><input type="checkbox" [checked]="n.props?.fillRule === 'evenodd'"
            (change)="props({ fillRule: checked($event) ? 'evenodd' : undefined })" /> {{ 'vec.evenodd' | t }}</label>
        </section>
      }
    }
    @if (flat()) {
      <section class="fg-sec">
        <button class="vp-btn" [title]="'vec.flattenTip' | t" (click)="flatten()">⬡ {{ 'vec.flatten' | t }} <kbd>Ctrl+E</kbd></button>
      </section>
    }
    @if (node(); as n) {
      <section class="fg-sec vp-export">
        <h5>{{ 'vec.export' | t }}</h5>
        <div class="vp-row">
          <select [value]="scale()" (change)="setScale(val($event))" [disabled]="format() === 'svg'" title="Scale">
            <option value="1">1x</option><option value="2">2x</option><option value="3">3x</option>
          </select>
          <select [value]="format()" (change)="setFormat(val($event))" title="Format">
            <option value="png">PNG</option><option value="jpg">JPG</option>
            @if (svgOk()) { <option value="svg">SVG</option> }
          </select>
        </div>
        <button class="vp-btn vp-wide" data-export (click)="exportNow()" [disabled]="busy()">{{ 'vec.exportBtn' | t: { name: fileName() } }}</button>
        <button class="vp-btn vp-wide vp-sub" (click)="copy()" [disabled]="busy()">⧉ {{ 'vec.copyPng' | t }}</button>
      </section>
    }
  `,
  styles: [`
    .vp-row { display: flex; gap: 6px; flex-wrap: wrap; align-items: center; }
    .vp-row select { flex: 1; min-width: 0; }
    .vp-btn { display: inline-flex; align-items: center; gap: 4px; border: 1px solid var(--fg-border, #e5e5e5); background: #fff; border-radius: 6px;
      padding: 4px 8px; font: inherit; font-size: 11px; cursor: pointer; color: var(--fg-text, #222); }
    .vp-btn:hover:not([disabled]) { border-color: #0d99ff; }
    .vp-btn.on { background: #0d99ff; color: #fff; border-color: #0d99ff; }
    .vp-btn[disabled] { opacity: .5; cursor: default; }
    .vp-btn kbd { font-size: 10px; opacity: .6; margin-left: 4px; }
    .vp-ic { font-size: 14px; }
    .vp-edit { float: right; font-weight: 400; }
    .vp-wide { width: 100%; justify-content: center; margin-top: 6px; }
    .vp-sub { background: transparent; }
    .vp-lbl { display: flex; flex-direction: column; gap: 2px; font-size: 11px; color: var(--fg-muted, #888); flex: 1; }
    .vp-check { display: flex; gap: 6px; align-items: center; font-size: 11px; margin-top: 6px; }
  `],
})
export class VectorPanel {
  readonly store = inject(EditorStore);
  /** The panel's node (the primary selection). */
  readonly sel = input<DNode | null>(null, { alias: 'node' });
  readonly ops = BOOL_OPS;
  readonly busy = signal(false);

  readonly node = computed(() => (this.store.selectedIds().length === 1 ? this.sel() : null));
  readonly bool = computed(() => canBoolean(this.store));
  readonly flat = computed(() => canFlatten(this.store));
  readonly editing = computed(() => !!this.node() && this.store.vectorEditId() === this.node()!.id);
  readonly svgOk = computed(() => !!this.node() && svgExportable(this.node()!));
  readonly format = computed<ExportFormat>(() => {
    const f = this.store.exportSettings().format;
    return f === 'svg' && !this.svgOk() ? 'png' : f;
  });
  readonly scale = computed(() => this.store.exportSettings().scale);
  readonly fileName = computed(() => {
    const n = this.node();
    if (!n) return '';
    const f = this.format();
    return `${exportName(n)}${f !== 'svg' && this.scale() !== 1 ? '@' + this.scale() + 'x' : ''}.${f}`;
  });

  run(op: BoolOp) { booleanOp(this.store, op); }
  flatten() { flattenSelection(this.store); }
  toggleEdit(n: DNode) { this.store.vectorEditId.set(this.editing() ? null : n.id); }
  props(p: Partial<Props>) { this.store.setProps(this.store.selectedIds(), p); }
  setFormat(f: string) { this.store.exportSettings.update(s => ({ ...s, format: f as ExportFormat })); }
  setScale(v: string) { this.store.exportSettings.update(s => ({ ...s, scale: Number(v) || 1 })); }

  async exportNow() {
    this.busy.set(true);
    try { await exportNode(this.store, this.node()?.id ?? null, { format: this.format(), scale: this.scale() }); } finally { this.busy.set(false); }
  }

  async copy() {
    this.busy.set(true);
    try { await copyAsPng(this.store, this.node()?.id ?? null); } finally { this.busy.set(false); }
  }

  val(e: Event) { return (e.target as HTMLSelectElement).value as never; }
  checked(e: Event) { return (e.target as HTMLInputElement).checked; }
}
