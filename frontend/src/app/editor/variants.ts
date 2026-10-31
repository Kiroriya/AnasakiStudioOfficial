import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { Dialog } from '../core/dialog';
import { TPipe, tr } from '../core/i18n';
import { DNode } from './design-types';
import { EditorStore } from './editor-store';
import './messages-variants';

/** Component designer: the list of variants (main design + others), like Figma's variant set. */
@Component({
  selector: 'rea-variant-bar',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="vb">
      <span class="vb-label" [title]="'vr.tip' | t">◈ {{ 'vr.title' | t }}</span>
      <button class="vb-tab" [class.on]="store.variantIndex() < 0" (click)="store.openVariant(-1)" (dblclick)="rename(-1)">{{ store.mainVariantName() }}</button>
      @for (v of store.variants(); track $index; let i = $index) {
        <button class="vb-tab" [class.on]="store.variantIndex() === i" (click)="store.openVariant(i)" (dblclick)="rename(i)">{{ v.name }}</button>
      }
      <button class="vb-add" (click)="add()" [title]="'vr.addTip' | t">＋ {{ 'vr.add' | t }}</button>
      @if (store.variantIndex() >= 0) {
        <button class="vb-act" (click)="rename(store.variantIndex())">{{ 'vr.rename' | t }}</button>
        <button class="vb-act danger" (click)="remove()">{{ 'common.delete' | t }}</button>
      }
    </div>
  `,
  styles: [`
    :host { position: absolute; z-index: 4; top: 10px; left: 12px; max-width: calc(50% - 150px); }
    :host-context(.fg-canvas:has(> rea-canvas .rl-on)) { top: 28px; }
    .vb { display: flex; align-items: center; gap: 4px; flex-wrap: wrap; padding: 6px 10px; background: #f5f3ff; border: 1px solid #e9e5ff; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,.08); font-size: 12px; }
    .vb-label { font-weight: 700; color: #6d28d9; margin-right: 6px; cursor: help; }
    .vb-tab { border: 1px solid #ddd6fe; background: #fff; color: #4c1d95; border-radius: 999px; padding: 3px 12px; cursor: pointer; font: inherit; }
    .vb-tab.on { background: #7c3aed; border-color: #7c3aed; color: #fff; }
    .vb-add, .vb-act { border: 0; background: none; color: #6d28d9; cursor: pointer; font: inherit; padding: 3px 8px; border-radius: 6px; }
    .vb-add:hover, .vb-act:hover { background: #ede9fe; }
    .danger { color: #dc2626; }
  `],
})
export class VariantBar {
  readonly store = inject(EditorStore);

  private names() { return [this.store.mainVariantName(), ...this.store.variants().map(v => v.name)]; }

  private async ask(title: string, value: string): Promise<string | null> {
    const name = (await Dialog.prompt({ title, value }))?.trim().slice(0, 40);
    if (!name) return null;
    if (this.names().includes(name) && name !== value) { this.store.notify(tr('vr.taken'), true); return null; }
    return name;
  }

  async add() {
    const name = await this.ask(tr('vr.addPrompt'), tr('vr.newName', { n: this.store.variants().length + 2 }));
    if (name) this.store.addVariant(name);
  }

  async rename(i: number) {
    const cur = i < 0 ? this.store.mainVariantName() : this.store.variants()[i]?.name;
    const name = await this.ask(tr('vr.renamePrompt'), cur ?? '');
    if (!name || name === cur) return;
    // placements choose a variant by name: the server renames the ones saved on other pages after
    // the component is saved — an older server can't, and those fall back to the main design
    if (i >= 0) {
      const follow = this.store.variantRenameApi();
      const ok = await Dialog.confirm(follow
        ? { title: tr('vr.renameWarnTitle'), text: tr('vr.renameFollow', { old: cur ?? '', name }), confirm: tr('vr.rename'), icon: 'question' }
        : { title: tr('vr.renameWarnTitle'), text: tr('vr.renameWarn', { old: cur ?? '', name }), confirm: tr('vr.rename'), icon: 'warning' });
      if (!ok) return;
    }
    this.store.renameVariant(i, name);
  }

  async remove() {
    const i = this.store.variantIndex();
    const v = this.store.variants()[i];
    if (v && (await Dialog.confirmDelete(v.name, tr('vr.deleteHint')))) this.store.deleteVariant(i);
  }
}

/** Instance: which variant this placement shows. */
@Component({
  selector: 'rea-variant-picker',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (list().length) {
      <section class="fg-sec">
        <h5>{{ 'vr.pick' | t }}</h5>
        <div class="vp">
          <button [class.on]="!node().props?.variant" (click)="set('')">{{ main() }}</button>
          @for (v of list(); track v.name) {
            <button [class.on]="node().props?.variant === v.name" (click)="set(v.name)">{{ v.name }}</button>
          }
        </div>
      </section>
    }
  `,
  styles: [`
    .vp { display: flex; flex-wrap: wrap; gap: 4px; }
    .vp button { border: 1px solid var(--fg-border, #e5e5e5); background: #fff; border-radius: 6px; padding: 4px 10px; cursor: pointer; font: inherit; font-size: 12px; }
    .vp button.on { border-color: #7c3aed; background: #f5f3ff; color: #5b21b6; font-weight: 600; }
  `],
})
export class VariantPicker {
  readonly store = inject(EditorStore);
  readonly node = input.required<DNode>();
  private comp = computed(() => this.store.componentMap().get(this.node().props?.componentId ?? -1));
  readonly list = computed(() => this.comp()?.node.props?.variants ?? []);
  readonly main = computed(() => this.comp()?.node.props?.variantName || 'Default');

  set(name: string) { this.store.setProps(this.node().id, { variant: name || undefined }); }
}
