import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { TPipe } from '../core/i18n';
import { DNode, NODE_ICON, NodeAction } from './design-types';
import { EditorStore } from './editor-store';
import { CodeActionPanel } from './code-action';
import { CodeData } from './code-data';
import './messages-prototype';
import './messages-sitecode';

type ActionType = NodeAction['type'];

/**
 * Prototype tab (Figma): what happens on click (open/close popup, show/hide,
 * switch tab, scroll to), popups, layers hidden until shown, sliders and the
 * look of an active trigger. Works on the published site (InteractScript).
 */
@Component({
  selector: 'rea-prototype-panel',
  imports: [TPipe, CodeActionPanel],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let n = node();
    @let a = n.props?.action;
    <section class="fg-sec">
      <h5>🖱 {{ 'pr.onClick' | t }}</h5>
      <select id="pr-action-type" [value]="a?.type ?? ''" (change)="setType($any($event.target).value)">
        <option value="">{{ 'pr.none' | t }}</option>
        @for (t of types; track t) { <option [value]="t" [selected]="a?.type === t">{{ 'pr.t.' + t | t }}</option> }
      </select>
      @if (a?.type === 'code') { <rea-code-action [node]="n" /> }
      @else if (a && a.type !== 'close') {
        <label class="pr-l">{{ (a.type === 'open' ? 'pr.whichPopup' : a.type === 'tab' ? 'pr.whichPanel' : 'pr.whichLayer') | t }}
          <select [value]="a.target ?? ''" (change)="setTarget($any($event.target).value)">
            <option value="">{{ 'pr.pick' | t }}</option>
            @for (o of targets(); track o.id) { <option [value]="o.id" [selected]="a.target === o.id">{{ o.label }}</option> }
          </select>
        </label>
        @if (a.type === 'open' && !popups().length) { <p class="hint warn">{{ 'pr.noPopups' | t }}</p> }
      }
      <p class="hint">{{ (a ? (a.type === 'code' ? 'sc.act.hint' : 'pr.h.' + a.type) : 'pr.hint') | t }}</p>
      @if (n.props?.href && a) { <p class="hint warn">{{ 'pr.hrefIgnored' | t }}</p> }
    </section>

    @if (a?.type === 'toggle' || a?.type === 'tab') {
      <section class="fg-sec">
        <h5>✦ {{ 'pr.active' | t }}</h5>
        <div class="pr-grid">
          <label>{{ 'pr.fill' | t }}<input type="color" [value]="hex(n.style?.active?.fill)" (input)="active({ fill: $any($event.target).value })" /></label>
          <label>{{ 'pr.color' | t }}<input type="color" [value]="hex(n.style?.active?.color)" (input)="active({ color: $any($event.target).value })" /></label>
          <label>{{ 'pr.stroke' | t }}<input type="color" [value]="hex(n.style?.active?.stroke)" (input)="active({ stroke: $any($event.target).value })" /></label>
          @if (n.style?.active) { <button class="fg-link" (click)="store.setStyle(n.id, { active: undefined })">{{ 'pt.reset' | t }}</button> }
        </div>
        <p class="hint">{{ 'pr.activeHint' | t }}</p>
      </section>
    }

    <section class="fg-sec">
      <h5>👁 {{ 'pr.visibility' | t }}</h5>
      <label class="fg-check"><input type="checkbox" [checked]="!!n.props?.collapsed" (change)="props({ collapsed: chk($event) || undefined })" /> {{ 'pr.collapsed' | t }}</label>
      <p class="hint">{{ 'pr.collapsedHint' | t }}</p>
    </section>

    @if (n.type === 'frame') {
      <section class="fg-sec" id="pr-form-sec">
        <h5>📝 {{ 'sc.form.title' | t }}</h5>
        <label class="fg-check"><input type="checkbox" id="pr-form" [checked]="!!n.props?.form" (change)="props({ form: chk($event) || undefined })" /> {{ 'sc.form.is' | t }}</label>
        <p class="hint">{{ 'sc.form.hint' | t }}</p>
      </section>
      <section class="fg-sec">
        <h5>🗔 Popup</h5>
        <label class="fg-check"><input type="checkbox" [checked]="!!n.props?.popup" (change)="props({ popup: chk($event) || undefined })" /> {{ 'pr.isPopup' | t }}</label>
        @if (n.props?.popup) {
          <label class="pr-l">{{ 'pr.delay' | t }}<input type="number" min="0" max="600" [value]="n.props.popupDelay ?? ''" [placeholder]="'pr.delayNone' | t" (change)="props({ popupDelay: numv($event) || undefined })" /></label>
          @if (n.props.popupDelay) { <label class="fg-check"><input type="checkbox" [checked]="!!n.props.popupOnce" (change)="props({ popupOnce: chk($event) || undefined })" /> {{ 'pr.once' | t }}</label> }
          @if (n.width.mode === 'fill') { <p class="hint warn">{{ 'pr.popupWidth' | t }}</p> }
        }
        <p class="hint">{{ 'pr.popupHint' | t }}</p>
      </section>

      <section class="fg-sec">
        <h5>🎞 {{ 'pr.carousel' | t }}</h5>
        <label class="fg-check"><input type="checkbox" [checked]="!!n.props?.carousel" (change)="toggleCarousel(chk($event))" /> {{ 'pr.isCarousel' | t }}</label>
        @if (n.props?.carousel; as c) {
          <label class="fg-check"><input type="checkbox" [checked]="!!c.arrows" (change)="carousel({ arrows: chk($event) || undefined })" /> {{ 'pr.arrows' | t }}</label>
          <label class="fg-check"><input type="checkbox" [checked]="!!c.dots" (change)="carousel({ dots: chk($event) || undefined })" /> {{ 'pr.dots' | t }}</label>
          <label class="pr-l">{{ 'pr.autoplay' | t }}<input type="number" min="0" max="60" [value]="c.autoplay ?? ''" [placeholder]="'pr.off' | t" (change)="carousel({ autoplay: numv($event) || undefined })" /></label>
        }
        <p class="hint">{{ 'pr.carouselHint' | t }}</p>
      </section>
    }
    <p class="hint">▶ {{ 'pr.tryIt' | t }}</p>
  `,
  styles: [`
    .pr-l { display: flex; flex-direction: column; gap: 3px; font-size: 11px; color: var(--fg-muted, #777); margin-top: 6px; }
    .pr-grid { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; font-size: 11px; color: var(--fg-muted, #777); }
    .pr-grid label { display: flex; align-items: center; gap: 4px; }
    .pr-grid input[type=color] { width: 26px; height: 22px; padding: 0; border: 1px solid #ddd; border-radius: 4px; }
    .warn { color: #b45309; }
  `],
})
export class PrototypePanel {
  readonly store = inject(EditorStore);
  readonly node = input.required<DNode>();
  readonly code = inject(CodeData);
  readonly types: ActionType[] = ['open', 'close', 'toggle', 'tab', 'scroll', 'code'];

  /** Every layer of the document (for targets), indented by depth. */
  private readonly all = computed(() => {
    const out: { id: string; label: string; popup: boolean }[] = [];
    const walk = (x: DNode, depth: number) => {
      if (depth > 0) out.push({ id: x.id, label: '  '.repeat(depth - 1) + NODE_ICON[x.type] + ' ' + (x.name || x.type) + (x.props?.popup ? ' (popup)' : ''), popup: !!x.props?.popup });
      x.children?.forEach(c => walk(c, depth + 1));
    };
    walk(this.store.doc(), 0);
    return out;
  });
  readonly popups = computed(() => this.all().filter(o => o.popup));
  readonly targets = computed(() => {
    const a = this.node().props?.action;
    const list = a?.type === 'open' ? this.popups() : this.all();
    return list.filter(o => o.id !== this.node().id);
  });

  props(p: DNode['props']) { this.store.setProps(this.node().id, p!); }
  chk(e: Event) { return (e.target as HTMLInputElement).checked; }
  numv(e: Event) { const v = Number((e.target as HTMLInputElement).value); return Number.isFinite(v) && v > 0 ? v : 0; }
  hex(v?: string) { return v && /^#[0-9a-f]{6}$/i.test(v) ? v : '#ffffff'; }

  setType(t: string) {
    if (!t) { this.props({ action: undefined }); return; }
    const cur = this.node().props?.action;
    if (t === 'code') { // "Call code": the first site.action, inputs mapped in the panel
      this.props({ action: { type: 'code', name: cur?.type === 'code' ? cur.name : this.code.bindings()?.actions[0]?.name ?? '', input: cur?.type === 'code' ? cur.input : {} } });
      return;
    }
    this.props({ action: { type: t as ActionType, target: t === 'close' || cur?.type === 'code' ? undefined : cur?.target } });
  }

  setTarget(id: string) {
    const a = this.node().props?.action;
    if (a) this.props({ action: { ...a, target: id || undefined } });
  }

  active(p: { fill?: string; color?: string; stroke?: string }) {
    this.store.setStyle(this.node().id, { active: { ...this.node().style?.active, ...p } });
  }

  toggleCarousel(on: boolean) {
    const n = this.node();
    if (on && (n.layout?.mode ?? 'none') !== 'horizontal') this.store.setLayout(n.id, { mode: 'horizontal' });
    this.props({ carousel: on ? { arrows: true, dots: true } : undefined });
  }

  carousel(p: { arrows?: boolean; dots?: boolean; autoplay?: number }) {
    const c = { ...this.node().props?.carousel, ...p };
    for (const k of Object.keys(c) as (keyof typeof c)[]) if (c[k] === undefined) delete c[k];
    this.props({ carousel: c });
  }
}
