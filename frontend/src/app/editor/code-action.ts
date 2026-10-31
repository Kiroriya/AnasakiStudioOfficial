import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { TPipe } from '../core/i18n';
import { BindingInput } from './binding-input';
import { CodeData } from './code-data';
import { parseGroups, enclosingCollection, enclosingForm, filterLayers, formInputNames, layerChoices, mappingRows, setMapping } from './code-logic';
import { DNode, NodeAction } from './design-types';
import { EditorStore } from './editor-store';
import './messages-sitecode';
import { groupByUnit } from '../code/units-utils';
import { unitGroupLabel } from './code-panels';

/** A same-site path for success.redirect ("/thanks", not "https://…" or "//host"). */
export const validRedirect = (v: string) => !v || (/^\/(?!\/)/.test(v) && !/[\s\\]/.test(v));

/**
 * "Call code" action (Prototype tab): which site.action, input mapping rows
 * `key = expr`, confirm text, layers re-rendered with the response, success
 * message / redirect. Inside a form frame the inputs are sent too.
 */
@Component({
  selector: 'rea-code-action',
  imports: [TPipe, BindingInput],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let n = node();
    @let a = action();
    <div class="sc-act" id="sc-action">
      <label class="pr-l">{{ 'sc.act.name' | t }}
        <select id="sc-act-name" (change)="patch({ name: val($event) })">
          <option value="" [selected]="!a.name">{{ 'sc.act.pick' | t }}</option>
          @for (g of actionGroups(); track g.unit) {
            @if (actionGroups().length > 1) {
              <optgroup [label]="g.label" [attr.data-unit]="g.unit">
                @for (x of g.items; track x.name) { <option [value]="x.name" [selected]="a.name === x.name">{{ x.name }}</option> }
              </optgroup>
            } @else {
              @for (x of g.items; track x.name) { <option [value]="x.name" [selected]="a.name === x.name">{{ x.name }}</option> }
            }
          }
          @if (a.name && !known()) { <option [value]="a.name" selected>{{ a.name }} ⚠</option> }
        </select>
      </label>
      @if (!actions().length) { <p class="hint warn" id="sc-act-none">{{ 'sc.act.none' | t }}</p> }
      @if (form(); as f) { <p class="hint" id="sc-act-form">📝 {{ 'sc.act.inForm' | t: { name: f.name || 'form', fields: formFields().join(', ') || '—' } }}</p> }

      <h6 class="sc-h6">{{ 'sc.act.input' | t }}</h6>
      @for (r of rows(); track r.name) {
        <div class="sc-map-row sc-act-input-row" [attr.data-key]="r.name">
          <span class="sc-map-key" [title]="r.type ?? ''">{{ r.name }}@if (r.type) {<small>{{ r.type }}</small>}</span>
          <rea-binding-input class="sc-map-val" mode="expr" [nodeId]="n.id" [value]="r.expr" [inputId]="'sc-act-input-' + r.name"
                             [placeholder]="formFields().includes(r.name) ? ('sc.act.fromForm' | t) : 'item.id'" (changed)="setInput(r.name, $event)" />
          @if (fieldSuggestions().length) {
            <select class="sc-act-suggest" [title]="'sc.act.suggest' | t" (change)="pickSuggestion(r.name, $event)">
              <option value="">＋</option>
              @for (s of fieldSuggestions(); track s) { <option [value]="s">{{ s }}</option> }
            </select>
          }
          @if (!r.declared) { <button class="fg-link sc-act-input-del" [title]="'sc.common.remove' | t" (click)="setInput(r.name, '')">✕</button> }
        </div>
      } @empty { <p class="hint">{{ 'sc.act.noInput' | t }}</p> }
      <div class="sc-map-row">
        <input id="sc-act-newkey" class="mono" [value]="newKey()" (input)="newKey.set(val($event))" (keydown.enter)="addKey()" [placeholder]="'sc.act.newKey' | t" />
        <button class="btn btn-sm" id="sc-act-addkey" [disabled]="!newKeyOk()" (click)="addKey()">＋</button>
      </div>

      <label class="pr-l">{{ 'sc.act.confirm' | t }}
        <input id="sc-act-confirm" [value]="a.confirm ?? ''" [placeholder]="'sc.act.confirmPh' | t" (change)="patch({ confirm: val($event) || undefined })" /></label>

      <h6 class="sc-h6">{{ 'sc.act.update' | t }}</h6>
      <input id="sc-act-update-q" type="search" [value]="layerQ()" (input)="layerQ.set(val($event))" [placeholder]="'sc.act.updateSearch' | t" />
      <div class="sc-layer-list" id="sc-act-update">
        @for (l of layers(); track l.id) {
          <label class="sc-layer-opt sc-act-update-opt" [attr.data-id]="l.id" [title]="l.name"><input type="checkbox" [checked]="!!a.update?.includes(l.id)" (change)="toggleUpdate(l.id, chk($event))" /><span>{{ l.label }}</span></label>
        } @empty { <p class="hint">{{ 'sc.act.noLayers' | t }}</p> }
      </div>
      <p class="hint">{{ 'sc.act.updateHint' | t: { n: a.update?.length ?? 0 } }}</p>

      <label class="pr-l">{{ 'sc.act.successMsg' | t }}
        <input id="sc-act-success-msg" [value]="a.success?.message ?? ''" (change)="success({ message: val($event) || undefined })" /></label>
      <label class="pr-l">{{ 'sc.act.redirect' | t }}
        <input id="sc-act-success-redirect" [value]="a.success?.redirect ?? ''" placeholder="/thanks" (change)="success({ redirect: val($event).trim() || undefined })" /></label>
      @if (!redirectOk()) { <p class="hint warn" id="sc-act-redirect-warn">{{ 'sc.act.redirectInvalid' | t }}</p> }
    </div>
  `,
})
export class CodeActionPanel {
  readonly store = inject(EditorStore);
  readonly code = inject(CodeData);
  readonly node = input.required<DNode>();
  readonly newKey = signal('');
  readonly layerQ = signal('');

  readonly action = computed<NodeAction>(() => this.node().props?.action ?? { type: 'code' });
  readonly actions = computed(() => this.code.bindings()?.actions ?? []);
  /** Plugin actions (`<id>:<name>`, posted to /_a/<id>:<name>) grouped by unit after the Site code ones. */
  readonly actionGroups = computed(() => groupByUnit(this.actions()).map(g => ({ ...g, label: unitGroupLabel(g.unit) })));
  readonly known = computed(() => this.actions().some(x => x.name === this.action().name));
  readonly declared = computed(() => {
    const inp = this.actions().find(x => x.name === this.action().name)?.input ?? {};
    return inp;
  });
  readonly rows = computed(() => mappingRows(this.declared(), this.action().input));
  readonly form = computed(() => enclosingForm(this.store.doc(), this.node().id));
  readonly formFields = computed(() => formInputNames(this.form()));
  /** item.* paths of the enclosing collection (quick picks for the mapping). */
  readonly fieldSuggestions = computed(() => {
    const col = enclosingCollection(this.store.doc(), this.node().id);
    return [...this.code.itemFields(col).map(f => 'item.' + f), ...(col ? ['$index'] : [])];
  });
  readonly layers = computed(() => filterLayers(layerChoices(this.store.doc(), this.node().id), this.layerQ()));
  readonly newKeyOk = computed(() => /^[A-Za-z_][\w-]{0,62}$/.test(this.newKey().trim()) && !this.rows().some(r => r.name === this.newKey().trim()));
  readonly redirectOk = computed(() => validRedirect(this.action().success?.redirect ?? ''));

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  chk(e: Event) { return (e.target as HTMLInputElement).checked; }

  patch(p: Partial<NodeAction>) {
    this.store.setProps(this.node().id, { action: { ...this.action(), type: 'code', ...p } });
  }
  setInput(name: string, expr: string) { this.patch({ input: setMapping(this.action().input, name, expr) ?? {} }); }
  pickSuggestion(name: string, e: Event) {
    const el = e.target as HTMLSelectElement;
    if (el.value) this.setInput(name, el.value);
    el.value = '';
  }
  addKey() {
    if (!this.newKeyOk()) return;
    const k = this.newKey().trim();
    this.patch({ input: { ...(this.action().input ?? {}), [k]: this.fieldSuggestions()[0] ?? "''" } });
    this.newKey.set('');
  }
  toggleUpdate(id: string, on: boolean) {
    const cur = new Set(this.action().update ?? []);
    if (on) cur.add(id); else cur.delete(id);
    this.patch({ update: cur.size ? [...cur] : undefined });
  }
  success(p: { message?: string; redirect?: string }) {
    const s = { ...this.action().success, ...p };
    if (!s.message) delete s.message;
    if (!s.redirect) delete s.redirect;
    this.patch({ success: Object.keys(s).length ? s : undefined });
  }
}

/**
 * Canvas toolbar of Site Code previews: view as Visitor / Me, draft code
 * (edit_scripts), show layers hidden by their condition, refresh previews.
 */
@Component({
  selector: 'rea-code-bar',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (visible()) {
      <div class="sc-bar" id="sc-bar" (pointerdown)="$event.stopPropagation()">
        <span class="sc-bar-l">{{ 'sc.bar.viewAs' | t }}</span>
        <div class="sc-seg" role="group" [attr.aria-label]="'sc.bar.viewAs' | t">
          <button id="sc-viewas-anonymous" [class.on]="code.viewAs() === 'anonymous'" (click)="code.viewAs.set('anonymous')" [title]="'sc.bar.visitorTip' | t">{{ 'sc.bar.visitor' | t }}</button>
          <button id="sc-viewas-me" [class.on]="code.viewAs() === 'me'" (click)="code.viewAs.set('me')" [title]="'sc.bar.meTip' | t">{{ 'sc.bar.me' | t }}</button>
          <button id="sc-viewas-group" [class.on]="code.viewAs() === 'group'" (click)="viewAsGroup()" [title]="'sc.bar.groupTip' | t">{{ 'sc.bar.group' | t }}</button>
        </div>
        @if (code.viewAs() === 'group') {
          <span class="sc-viewas-groups" id="sc-viewas-groups" role="group" [attr.aria-label]="'sc.bar.group' | t">
            @for (g of code.visitorGroups(); track g) {
              <label class="sc-bar-chk sc-viewas-grp" [attr.data-group]="g"><input type="checkbox" [checked]="code.viewGroups().includes(g)" (change)="toggleGroup(g, chk($event))" /> {{ g }}</label>
            }
            <input class="sc-viewas-input" id="sc-viewas-groups-input" [value]="extraGroups()" (change)="setExtra($any($event.target).value)"
                   [placeholder]="code.visitorGroups().length ? ('sc.bar.groupMore' | t) : ('sc.bar.groupPh' | t)" [title]="'sc.bar.groupInputTip' | t" />
          </span>
        }
        @if (code.canDraft()) {
          <label class="sc-bar-chk" [title]="'sc.bar.draftTip' | t"><input type="checkbox" id="sc-draft" [checked]="code.draft()" (change)="code.draft.set(chk($event))" /> {{ 'sc.bar.draft' | t }}</label>
        }
        <label class="sc-bar-chk" [title]="'sc.bar.showHiddenTip' | t"><input type="checkbox" id="sc-show-hidden" [checked]="code.showHidden()" (change)="code.showHidden.set(chk($event))" /> {{ 'sc.bar.showHidden' | t }}</label>
        <button class="sc-bar-btn" id="sc-refresh" (click)="code.refresh()" [title]="'sc.bar.refresh' | t">↻</button>
      </div>
    }
  `,
})
export class CodeBar {
  readonly code = inject(CodeData);
  readonly visible = computed(() => {
    const b = this.code.bindings();
    return this.code.uses() || !!(b && (b.collections.length || b.actions.length || b.pages.length || b.tables.length || b.visitorGroups?.length));
  });
  /** Groups typed by hand (not declared with site.visitorGroups). */
  readonly extraGroups = computed(() => this.code.viewGroups().filter(g => !this.code.visitorGroups().includes(g)).join(', '));
  chk(e: Event) { return (e.target as HTMLInputElement).checked; }

  /** "Visitor in group…": starts with the first declared group. */
  viewAsGroup() {
    if (!this.code.viewGroups().length && this.code.visitorGroups().length) this.code.viewGroups.set([this.code.visitorGroups()[0]]);
    this.code.viewAs.set('group');
  }
  toggleGroup(g: string, on: boolean) {
    this.code.viewGroups.update(list => (on ? [...new Set([...list, g])] : list.filter(x => x !== g)));
  }
  setExtra(text: string) {
    const declared = this.code.viewGroups().filter(g => this.code.visitorGroups().includes(g));
    this.code.viewGroups.set([...new Set([...declared, ...parseGroups(text)])]);
  }
}
