import { ChangeDetectionStrategy, Component, Directive, computed, inject, input } from '@angular/core';
import { TPipe, localized, tr } from '../core/i18n';
import { Dialog } from '../core/dialog';
import { Plugins } from '../core/plugins';
import { CodeLabel } from '../core/code-models';
import { BindingInput } from './binding-input';
import { CodeData } from './code-data';
import {
  GROUP_ASK, MapRow, buildInputOptions, collectionChoices, collectionKey, conditionPresets, defaultKey, enclosingCollection, filterQueryName, groupCondition,
  mappingRows, parseSource, setMapping, sourceFields, sourceParams, validKey,
} from './code-logic';
import { createPagination } from './code-presets';
import { PAGE_ITEM_DEFAULTS } from './design-css';
import { DNode, InputKind, InputOption, PageItemStyle, Props, TableQuery, WhereOp, findNode } from './design-types';
import { EditorStore } from './editor-store';
import { evalExpr, parseExpr, truthy } from './expr';
import './messages-sitecode';
import '../code/messages-code4b';
import { SITE_UNIT, groupByUnit, shortName } from '../code/units-utils';

/** Optgroup label of a code unit: "Site code" or "JS plugin: <id>". */
export const unitGroupLabel = (unit: string) => (unit === SITE_UNIT ? tr('sc.col.grpCode') : tr('sc.col.grpUnit', { name: unit }));

const val = (e: Event) => (e.target as HTMLInputElement).value;
const checked = (e: Event) => (e.target as HTMLInputElement).checked;
const num = (e: Event): number | undefined => { const v = (e.target as HTMLInputElement).value; return v === '' ? undefined : Number(v); };
export const labelOf = (l: CodeLabel | undefined, name: string) => localized(l) || name;

/** Base of the Site Code panels: the node, its props and the store. */
@Directive()
abstract class CodePanelBase {
  readonly store = inject(EditorStore);
  readonly code = inject(CodeData);
  readonly node = input.required<DNode>();
  readonly val = val;
  readonly checked = checked;
  readonly num = num;
  props(p: Partial<Props>) { this.store.setProps(this.node().id, p); }
}

// ======================================================================== collection

/**
 * Collection block: source picker (plugin sources, site code collections
 * `code:`, developer tables `table:`), items per page, params mapping,
 * URL key, empty text, no-reload, table query builder, preview status.
 */
@Component({
  selector: 'rea-collection-panel',
  imports: [TPipe, BindingInput],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let n = node();
    @let src = n.props?.source ?? '';
    @let k = kind();
    <section class="fg-sec sc-col" id="sc-collection">
      <h5>▦ {{ 'ed.pp.collection' | t }}</h5>
      <label>{{ 'ed.pp.source' | t }}
        <select id="sc-col-source" (change)="setSource(val($event))">
          <option value="" [selected]="!src">{{ 'sc.col.pick' | t }}</option>
          @if (pluginSources().length) {
            <optgroup [label]="'sc.col.grpPlugins' | t">
              @for (c of pluginSources(); track c.source) { <option [value]="c.source" [selected]="src === c.source">{{ c.label }}</option> }
            </optgroup>
          }
          @for (g of codeGroups(); track g.unit) {
            <optgroup [label]="g.label" [attr.data-unit]="g.unit">
              @for (c of g.items; track c.value) { <option [value]="c.value" [selected]="src === c.value">{{ c.label }}</option> }
            </optgroup>
          }
          @if (tables().length) {
            <optgroup [label]="'sc.col.grpTables' | t">
              @for (c of tables(); track c.value) { <option [value]="c.value" [selected]="src === c.value">{{ c.label }}</option> }
            </optgroup>
          }
          @if (src && !known()) { <option [value]="src" selected>{{ src }} ⚠</option> }
        </select>
      </label>
      @if (!pluginSources().length && !codeCollections().length && !tables().length) { <p class="hint" id="sc-col-nosource">{{ 'sc.col.noSource' | t }}</p> }
      @if (src && !known()) { <p class="hint warn">{{ 'sc.col.unknown' | t }}</p> }

      @if (k === 'code' || k === 'table') {
        <p class="sc-preview" id="sc-col-preview" [attr.data-state]="previewState()">
          @switch (previewState()) {
            @case ('loading') { ⏳ {{ 'sc.col.pvLoading' | t }} }
            @case ('error') { ⚠ {{ 'sc.col.pvError' | t }} <b>{{ preview()?.error }}</b> }
            @case ('empty') { ∅ {{ 'sc.col.pvEmpty' | t }} }
            @case ('ok') { ✓ {{ 'sc.col.pvOk' | t: { n: preview()?.items?.length ?? 0, total: preview()?.total ?? 0 } }} }
            @default { {{ 'sc.col.pvNone' | t }} }
          }
          <button class="fg-link sc-refresh" id="sc-col-refresh" (click)="code.refresh()" [title]="'sc.bar.refresh' | t">↻</button>
        </p>
        @if (code.viewAs() !== 'me' && previewState() === 'error') { <p class="hint warn" id="sc-col-visitor-warn">{{ 'sc.col.visitorWarn' | t }}</p> }
        <div class="fg-row2">
          <label>{{ 'sc.col.perPage' | t }}
            <input id="sc-col-perpage" type="number" min="1" [max]="perPageMax()" [value]="n.props?.perPage ?? ''" [placeholder]="'' + perPageDefault()" (change)="setPerPage(num($event))" /></label>
          <label>{{ 'sc.col.key' | t }}
            <input id="sc-col-key" [value]="n.props?.key ?? ''" [placeholder]="defaultKey()" maxlength="12" (change)="setKey(val($event))" /></label>
        </div>
        @if (n.props?.key && !keyOk()) { <p class="hint warn">{{ 'sc.col.keyInvalid' | t }}</p> }
        <p class="hint">{{ 'sc.col.keyHint' | t: { key: key() } }}</p>

        @if (k === 'code') {
          <h6 class="sc-h6">{{ 'sc.col.params' | t }}</h6>
          @for (r of paramRows(); track r.name) {
            <div class="sc-map-row sc-param-row" [attr.data-param]="r.name">
              <span class="sc-map-key" [title]="r.type ?? ''">{{ r.name }}@if (r.type) {<small>{{ r.type }}</small>}</span>
              <rea-binding-input class="sc-map-val" mode="expr" [pageOnly]="true" [nodeId]="n.id" [value]="r.expr" [inputId]="'sc-col-param-' + r.name"
                                 [placeholder]="'query.' + r.name" (changed)="setParam(r.name, $event)" />
              @if (r.declared) {
                <button class="fg-link sc-from-url" [attr.data-param]="r.name" [title]="'sc.col.fromUrl' | t" (click)="setParam(r.name, 'query.' + r.name)">?{{ r.name }}</button>
              } @else {
                <button class="fg-link sc-param-del" [title]="'sc.common.remove' | t" (click)="setParam(r.name, '')">✕</button>
              }
            </div>
            @if (!r.declared) { <p class="hint warn">{{ 'sc.col.paramUnknown' | t: { name: r.name } }}</p> }
          } @empty { <p class="hint" id="sc-col-noparams">{{ 'sc.col.noParams' | t }}</p> }
          <p class="hint">{{ 'sc.col.paramsHint' | t }}</p>
        }

        @if (k === 'table') {
          @let tq = n.props?.table ?? {};
          <div class="sc-table" id="sc-table">
            <h6 class="sc-h6">{{ 'sc.tbl.columns' | t }}</h6>
            <div class="sc-cols">
              @for (c of tableColumns(); track c.name) {
                <label class="fg-check"><input type="checkbox" [id]="'sc-table-col-' + c.name" [checked]="!!tq.columns?.includes(c.name)" (change)="toggleColumn(c.name, checked($event))" /> {{ c.name }}</label>
              }
            </div>
            <p class="hint">{{ 'sc.tbl.columnsHint' | t }}</p>
            <h6 class="sc-h6">{{ 'sc.tbl.where' | t }}</h6>
            @for (w of tq.where ?? []; track $index; let i = $index) {
              <div class="sc-where-row" [attr.data-index]="i">
                <select class="sc-where-col" (change)="setWhere(i, { column: val($event) })">
                  @for (c of tableColumns(); track c.name) { <option [value]="c.name" [selected]="w.column === c.name">{{ c.name }}</option> }
                </select>
                <select class="sc-where-op" (change)="setWhere(i, { op: $any(val($event)) })">
                  @for (o of ops; track o) { <option [value]="o" [selected]="w.op === o">{{ ('sc.tbl.op.' + o) | t }}</option> }
                </select>
                @if (w.op !== 'null' && w.op !== 'notnull') {
                  <rea-binding-input class="sc-where-val" mode="expr" [pageOnly]="true" [nodeId]="n.id" [value]="w.value ?? ''" placeholder="query.q" (changed)="setWhere(i, { value: $event })" />
                }
                <button class="fg-link sc-where-del" [title]="'sc.common.remove' | t" (click)="removeWhere(i)">✕</button>
              </div>
            }
            <button class="btn btn-sm" id="sc-where-add" [disabled]="!tableColumns().length" (click)="addWhere()">＋ {{ 'sc.tbl.addWhere' | t }}</button>
            <p class="hint">{{ 'sc.tbl.whereHint' | t }}</p>
            <h6 class="sc-h6">{{ 'sc.tbl.order' | t }}</h6>
            @for (o of tq.orderBy ?? []; track $index; let i = $index) {
              <div class="sc-order-row" [attr.data-index]="i">
                <select class="sc-order-col" (change)="setOrder(i, { column: val($event) })">
                  @for (c of tableColumns(); track c.name) { <option [value]="c.name" [selected]="o.column === c.name">{{ c.name }}</option> }
                </select>
                <select class="sc-order-dir" (change)="setOrder(i, { dir: $any(val($event)) })">
                  <option value="asc" [selected]="o.dir !== 'desc'">{{ 'sc.tbl.asc' | t }}</option>
                  <option value="desc" [selected]="o.dir === 'desc'">{{ 'sc.tbl.desc' | t }}</option>
                </select>
                <button class="fg-link sc-order-del" [title]="'sc.common.remove' | t" (click)="removeOrder(i)">✕</button>
              </div>
            }
            <button class="btn btn-sm" id="sc-order-add" [disabled]="!tableColumns().length" (click)="addOrder()">＋ {{ 'sc.tbl.addOrder' | t }}</button>
          </div>
        }

        <label class="fg-check"><input type="checkbox" id="sc-col-noreload" [checked]="!!n.props?.noReload" (change)="props({ noReload: checked($event) || undefined })" /> {{ 'sc.col.noReload' | t }}</label>
        <p class="hint">{{ 'sc.col.noReloadHint' | t }}</p>

        @if (fieldNames().length) {
          <h6 class="sc-h6">{{ 'sc.col.fields' | t }}</h6>
          <div class="sc-chips" id="sc-col-fields">
            @for (f of fieldNames(); track f) { <button class="sc-chip" [attr.data-field]="f" [title]="'sc.col.copyField' | t" (click)="copyField(f)">{{ '{{item.' + f + '}}' }}</button> }
          </div>
        }
        @if (!hasPagination()) {
          <button class="btn btn-sm btn-block" id="sc-col-add-pagination" (click)="addPagination()">⋯ {{ 'sc.col.addPagination' | t }}</button>
        }
      } @else {
        <div class="fg-row2">
          <label>{{ 'ed.pp.limit' | t }} <input type="number" min="1" max="50" [value]="n.props?.query?.limit ?? 6" (change)="query({ limit: num($event) })" /></label>
          <label>{{ 'ed.pp.order' | t }}
            <select [value]="n.props?.query?.order || 'newest'" (change)="query({ order: $any(val($event)) })">
              <option value="newest">{{ 'ed.pp.newest' | t }}</option><option value="oldest">{{ 'ed.pp.oldest' | t }}</option>
              <option value="title">{{ 'ed.pp.byTitle' | t }}</option>
            </select>
          </label>
        </div>
      }
      <label>{{ 'ed.pp.empty' | t }} <input id="sc-col-empty" [value]="n.props?.emptyText ?? ''" (change)="props({ emptyText: val($event) })" /></label>
      @if (k === 'code' || k === 'table') { <p class="hint">{{ 'sc.col.emptyHint' | t }}</p> }
    </section>
  `,
})
export class CollectionPanel extends CodePanelBase {
  private plugins = inject(Plugins);
  readonly ops: WhereOp[] = ['=', '!=', '<', '<=', '>', '>=', 'like', 'in', 'null', 'notnull'];
  readonly kind = computed(() => parseSource(this.node().props?.source).kind);
  readonly pluginSources = computed(() => this.plugins.list().filter(p => p.active).flatMap(p => p.collections ?? []));
  readonly codeCollections = computed(() => (this.code.bindings()?.collections ?? []).map(c => ({ value: 'code:' + c.name, label: labelOf(c.label, c.name) + ' · code:' + c.name })));
  /** Code collections grouped by unit: Site code first, then each JS plugin (`code:<id>:<name>`). */
  readonly codeGroups = computed(() => groupByUnit(this.code.bindings()?.collections ?? []).map(g => ({
    unit: g.unit, label: unitGroupLabel(g.unit),
    items: g.items.map(c => ({ value: 'code:' + c.name, label: labelOf(c.label, shortName(c.name, g.unit)) + ' · code:' + c.name })),
  })));
  readonly tables = computed(() => (this.code.bindings()?.tables ?? []).map(t => ({ value: 'table:' + t.name, label: t.name + ' · table' })));
  readonly known = computed(() => {
    const s = this.node().props?.source;
    return !s || this.pluginSources().some(c => c.source === s) || this.codeCollections().some(c => c.value === s) || this.tables().some(c => c.value === s);
  });
  private readonly binding = computed(() => {
    const s = parseSource(this.node().props?.source);
    return s.kind === 'code' ? this.code.bindings()?.collections.find(c => c.name === s.name) : undefined;
  });
  readonly perPageDefault = computed(() => this.binding()?.perPage?.default ?? 10);
  readonly perPageMax = computed(() => Math.min(this.binding()?.perPage?.max ?? 50, 100));
  readonly defaultKey = computed(() => defaultKey(this.node().id));
  readonly key = computed(() => collectionKey(this.node()));
  readonly keyOk = computed(() => validKey(this.node().props?.key ?? ''));
  readonly paramRows = computed<MapRow[]>(() => mappingRows(sourceParams(this.node().props?.source, this.code.bindings()), this.node().props?.params));
  readonly tableColumns = computed(() => {
    const s = parseSource(this.node().props?.source);
    return s.kind === 'table' ? this.code.bindings()?.tables.find(t => t.name === s.name)?.columns ?? [] : [];
  });
  readonly fieldNames = computed(() => Object.keys(sourceFields(this.node().props?.source, this.code.bindings(), this.node().props?.table)));
  readonly preview = computed(() => this.code.preview(this.code.requestFor(this.node())));
  readonly previewState = computed(() => {
    const p = this.preview();
    if (!p) return 'none';
    if (p.loading) return 'loading';
    if (p.error) return 'error';
    return p.items.length ? 'ok' : 'empty';
  });
  readonly hasPagination = computed(() => {
    const id = this.node().id;
    let found = false;
    const walk = (x: DNode) => { if (x.type === 'pagination' && x.props?.collection === id) found = true; x.children?.forEach(walk); };
    walk(this.store.doc());
    return found;
  });

  setSource(src: string) {
    const was = parseSource(this.node().props?.source).kind;
    const now = parseSource(src).kind;
    const patch: Partial<Props> = { source: src };
    if (was !== now || now === 'table') { patch.params = undefined; patch.table = undefined; } // params / columns belong to the old source
    this.props(patch);
  }
  setPerPage(v: number | undefined) { this.props({ perPage: v && v > 0 ? Math.min(Math.round(v), this.perPageMax()) : undefined }); }
  setKey(v: string) { const k = v.trim().toLowerCase(); this.props({ key: k || undefined }); }
  setParam(name: string, expr: string) { this.props({ params: setMapping(this.node().props?.params, name, expr) }); }
  query(p: Partial<NonNullable<Props['query']>>) { this.props({ query: { ...this.node().props?.query, ...p } }); }

  private table(fn: (t: TableQuery) => TableQuery) {
    const t = fn(structuredClone(this.node().props?.table ?? {}));
    if (!t.columns?.length) delete t.columns;
    if (!t.where?.length) delete t.where;
    if (!t.orderBy?.length) delete t.orderBy;
    this.props({ table: Object.keys(t).length ? t : undefined });
  }
  toggleColumn(c: string, on: boolean) {
    this.table(t => {
      const all = this.tableColumns().map(x => x.name);
      const cur = new Set(t.columns ?? []);
      if (on) cur.add(c); else cur.delete(c);
      return { ...t, columns: all.filter(x => cur.has(x)) };
    });
  }
  addWhere() { this.table(t => ({ ...t, where: [...(t.where ?? []), { column: this.tableColumns()[0]?.name ?? '', op: '=', value: '' }] })); }
  setWhere(i: number, p: Partial<NonNullable<TableQuery['where']>[number]>) {
    this.table(t => {
      const w = [...(t.where ?? [])];
      w[i] = { ...w[i], ...p };
      if (w[i].op === 'null' || w[i].op === 'notnull') delete w[i].value;
      return { ...t, where: w };
    });
  }
  removeWhere(i: number) { this.table(t => ({ ...t, where: (t.where ?? []).filter((_, j) => j !== i) })); }
  addOrder() { this.table(t => ({ ...t, orderBy: [...(t.orderBy ?? []), { column: this.tableColumns()[0]?.name ?? '', dir: 'asc' }] })); }
  setOrder(i: number, p: Partial<NonNullable<TableQuery['orderBy']>[number]>) {
    this.table(t => { const o = [...(t.orderBy ?? [])]; o[i] = { ...o[i], ...p }; return { ...t, orderBy: o }; });
  }
  removeOrder(i: number) { this.table(t => ({ ...t, orderBy: (t.orderBy ?? []).filter((_, j) => j !== i) })); }

  copyField(f: string) {
    const token = `{{item.${f}}}`;
    void navigator.clipboard?.writeText(token).catch(() => undefined);
    this.store.notify(token);
  }

  /** Adds a pagination node right after the collection, bound to it. */
  addPagination() {
    const n = this.node();
    const f = findNode(this.store.doc(), n.id);
    const pg = createPagination(n.id);
    if (f?.parent) this.store.insertAt(pg, f.parent.id, f.index + 1);
    else this.store.insert(pg);
  }
}

// ======================================================================== pagination

type PgKind = 'item' | 'active' | 'disabled';

/** Pagination block: which collection, look (numbers / prev-next / load more), window, labels, state styles. */
@Component({
  selector: 'rea-pagination-panel',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let n = node();
    <section class="fg-sec sc-pg" id="sc-pagination">
      <h5>⋯ {{ 'ed.node.pagination' | t }}</h5>
      <label>{{ 'sc.pg.collection' | t }}
        <select id="sc-pg-collection" (change)="props({ collection: val($event) || undefined })">
          <option value="" [selected]="!n.props?.collection">{{ 'sc.pg.pickCollection' | t }}</option>
          @for (c of choices(); track c.id) { <option [value]="c.id" [selected]="n.props?.collection === c.id">{{ c.label }}</option> }
        </select>
      </label>
      @if (!choices().length) { <p class="hint warn" id="sc-pg-nocollection">{{ 'sc.pg.noCollections' | t }}</p> }
      @else if (n.props?.collection && !bound()) { <p class="hint warn">{{ 'sc.pg.missing' | t }}</p> }
      <label>{{ 'sc.pg.style' | t }}
        <select id="sc-pg-style" (change)="props({ style: $any(val($event)) })">
          @for (s of styles; track s) { <option [value]="s" [selected]="(n.props?.style ?? 'numbers') === s">{{ ('sc.pg.style.' + s) | t }}</option> }
        </select>
      </label>
      @if ((n.props?.style ?? 'numbers') === 'numbers') {
        <label>{{ 'sc.pg.window' | t }} <input id="sc-pg-window" type="number" min="0" max="5" [value]="n.props?.window ?? 2" (change)="props({ window: clampWindow(num($event)) })" /></label>
      }
      <div class="fg-row3 sc-pg-labels">
        @if ((n.props?.style ?? 'numbers') !== 'loadmore') {
          <label>{{ 'sc.pg.prev' | t }} <input id="sc-pg-label-prev" [value]="n.props?.labels?.prev ?? ''" [placeholder]="defLabel('prev')" (change)="label('prev', val($event))" /></label>
          <label>{{ 'sc.pg.next' | t }} <input id="sc-pg-label-next" [value]="n.props?.labels?.next ?? ''" [placeholder]="defLabel('next')" (change)="label('next', val($event))" /></label>
        } @else {
          <label>{{ 'sc.pg.more' | t }} <input id="sc-pg-label-more" [value]="n.props?.labels?.more ?? ''" [placeholder]="defLabel('more')" (change)="label('more', val($event))" /></label>
        }
      </div>
      <p class="hint">{{ 'sc.pg.hint' | t }}</p>
    </section>
    @for (k of kinds; track k) {
      <section class="fg-sec sc-pg-style" [id]="'sc-pg-' + k">
        <h5>{{ ('sc.pg.k.' + k) | t }}
          @if (own(k)) { <button class="fg-link" [attr.id]="'sc-pg-' + k + '-reset'" (click)="resetStyle(k)">{{ 'pt.reset' | t }}</button> }
        </h5>
        <div class="sc-pg-grid">
          <label>{{ 'sc.pg.fill' | t }}<input type="color" class="sc-pg-fill" [value]="hex(eff(k).fill)" (input)="setStyle(k, { fill: val($event) })" /></label>
          <label>{{ 'sc.pg.color' | t }}<input type="color" class="sc-pg-color" [value]="hex(eff(k).color)" (input)="setStyle(k, { color: val($event) })" /></label>
          <label>{{ 'sc.pg.stroke' | t }}<input type="color" class="sc-pg-stroke" [value]="hex(eff(k).stroke)" (input)="setStyle(k, { stroke: val($event) })" /></label>
          @if (k === 'item') {
            <label>{{ 'sc.pg.radius' | t }}<input type="number" class="sc-pg-radius" min="0" [value]="eff(k).radius ?? ''" (change)="setStyle(k, { radius: num($event) })" /></label>
            <label>{{ 'sc.pg.padY' | t }}<input type="number" class="sc-pg-pady" min="0" [value]="eff(k).padY ?? ''" (change)="setStyle(k, { padY: num($event) })" /></label>
            <label>{{ 'sc.pg.padX' | t }}<input type="number" class="sc-pg-padx" min="0" [value]="eff(k).padX ?? ''" (change)="setStyle(k, { padX: num($event) })" /></label>
            <label>{{ 'sc.pg.fontSize' | t }}<input type="number" class="sc-pg-fontsize" min="8" [value]="eff(k).fontSize ?? ''" (change)="setStyle(k, { fontSize: num($event) })" /></label>
            <label>{{ 'sc.pg.fontWeight' | t }}
              <select class="sc-pg-fontweight" (change)="setStyle(k, { fontWeight: num($event) })">
                @for (w of [400, 500, 600, 700]; track w) { <option [value]="w" [selected]="eff(k).fontWeight === w">{{ w }}</option> }
              </select>
            </label>
          }
        </div>
      </section>
    }
  `,
})
export class PaginationPanel extends CodePanelBase {
  readonly styles = ['numbers', 'prevnext', 'loadmore'] as const;
  readonly kinds: PgKind[] = ['item', 'active', 'disabled'];
  /** Only collections of this page with code:/table: sources page. */
  readonly choices = computed(() => collectionChoices(this.store.doc()));
  readonly bound = computed(() => this.choices().some(c => c.id === this.node().props?.collection));
  clampWindow(v?: number) { return v === undefined ? undefined : Math.max(0, Math.min(5, Math.round(v))); }
  defLabel(k: 'prev' | 'next' | 'more') { return this.code.evalOpts().locale === 'en' ? { prev: 'Previous', next: 'Next', more: 'Load more' }[k] : { prev: 'ก่อนหน้า', next: 'ถัดไป', more: 'โหลดเพิ่ม' }[k]; }
  label(k: 'prev' | 'next' | 'more', v: string) {
    const l = { ...this.node().props?.labels, [k]: v.trim() || undefined };
    this.props({ labels: Object.values(l).some(x => x) ? l : undefined });
  }
  private key(k: PgKind) { return k === 'item' ? 'itemStyle' : k === 'active' ? 'activeStyle' : 'disabledStyle'; }
  own(k: PgKind): PageItemStyle | undefined { return this.node().props?.[this.key(k)]; }
  eff(k: PgKind): PageItemStyle { return { ...PAGE_ITEM_DEFAULTS[k], ...this.own(k) }; }
  setStyle(k: PgKind, p: Partial<PageItemStyle>) { this.props({ [this.key(k)]: { ...this.own(k), ...p } }); }
  resetStyle(k: PgKind) { this.props({ [this.key(k)]: undefined }); }
  hex(v?: string) { return v && /^#[0-9a-f]{6}$/i.test(v) ? v : '#ffffff'; }
}

// ======================================================================== input

/** Input block: kind, name, placeholder, value, options (static / from a source), required, filter binding. */
@Component({
  selector: 'rea-input-panel',
  imports: [TPipe, BindingInput],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let n = node();
    @let kind = n.props?.kind ?? 'text';
    <section class="fg-sec sc-in" id="sc-input">
      <h5>⌨ {{ 'ed.node.input' | t }}</h5>
      <div class="fg-row2">
        <label>{{ 'sc.in.kind' | t }}
          <select id="sc-in-kind" (change)="props({ kind: $any(val($event)) })">
            @for (k of kinds; track k) { <option [value]="k" [selected]="kind === k">{{ ('sc.in.k.' + k) | t }}</option> }
          </select>
        </label>
        <label>{{ 'sc.in.name' | t }} <input id="sc-in-name" class="mono" [value]="n.props?.name ?? ''" (change)="setName(val($event))" /></label>
      </div>
      @if (n.props?.name && !nameOk()) { <p class="hint warn">{{ 'sc.in.nameInvalid' | t }}</p> }
      @if (kind !== 'hidden' && kind !== 'checkbox') {
        <label>{{ 'sc.in.placeholder' | t }} <input id="sc-in-placeholder" [value]="n.props?.placeholder ?? ''" (change)="props({ placeholder: val($event) || undefined })" /></label>
      }
      @if (kind === 'checkbox') {
        <label>{{ 'sc.in.label' | t }} <input id="sc-in-label" [value]="n.props?.label ?? ''" (change)="props({ label: val($event) || undefined })" /></label>
      }
      <label>{{ 'sc.in.value' | t }}
        <rea-binding-input mode="expr" [nodeId]="n.id" [value]="n.props?.value ?? ''" inputId="sc-in-value" placeholder="query.q" (changed)="props({ value: $event || undefined })" />
      </label>
      <p class="hint">{{ 'sc.in.valueHint' | t }}</p>
      @if (kind !== 'hidden') {
        <label class="fg-check"><input type="checkbox" id="sc-in-required" [checked]="!!n.props?.required" (change)="props({ required: checked($event) || undefined })" /> {{ 'sc.in.required' | t }}</label>
      }
    </section>

    @if (kind === 'select') {
      <section class="fg-sec sc-in-options" id="sc-in-options">
        <h5>{{ 'sc.in.options' | t }}</h5>
        <div class="fg-seg sc-opt-mode">
          <button id="sc-in-opt-static" [class.on]="!n.props?.optionsFrom" (click)="useStatic()">{{ 'sc.in.optStatic' | t }}</button>
          <button id="sc-in-opt-from" [class.on]="!!n.props?.optionsFrom" (click)="useFrom()">{{ 'sc.in.optFrom' | t }}</button>
        </div>
        @if (n.props?.optionsFrom; as f) {
          <label>{{ 'sc.in.optSource' | t }}
            <select id="sc-in-opt-source" (change)="setFrom({ collection: val($event) })">
              <option value="" [selected]="!f.collection">{{ 'sc.col.pick' | t }}</option>
              @for (s of sources(); track s.value) { <option [value]="s.value" [selected]="f.collection === s.value">{{ s.label }}</option> }
            </select>
          </label>
          <div class="fg-row2">
            <label>{{ 'sc.in.optValue' | t }}
              <select id="sc-in-opt-value" (change)="setFrom({ value: val($event) })">
                @for (k of fromFields(); track k) { <option [value]="k" [selected]="f.value === k">{{ k }}</option> }
              </select>
            </label>
            <label>{{ 'sc.in.optLabel' | t }}
              <select id="sc-in-opt-label" (change)="setFrom({ label: val($event) })">
                @for (k of fromFields(); track k) { <option [value]="k" [selected]="f.label === k">{{ k }}</option> }
              </select>
            </label>
          </div>
          <p class="hint" id="sc-in-opt-preview">{{ 'sc.in.optPreview' | t: { n: builtOptions().length } }}@for (o of builtOptions().slice(0, 4); track $index) { · {{ o.label }} }</p>
        } @else {
          @for (o of n.props?.options ?? []; track $index; let i = $index) {
            <div class="sc-opt-row" [attr.data-index]="i">
              <input class="sc-opt-value mono" [value]="o.value" [placeholder]="'sc.in.optValue' | t" (change)="setOption(i, { value: val($event) })" />
              <input class="sc-opt-label" [value]="o.label" [placeholder]="'sc.in.optLabel' | t" (change)="setOption(i, { label: val($event) })" />
              <button class="fg-link sc-opt-del" [title]="'sc.common.remove' | t" (click)="removeOption(i)">✕</button>
            </div>
          }
          <button class="btn btn-sm" id="sc-in-opt-add" (click)="addOption()">＋ {{ 'sc.in.optAdd' | t }}</button>
        }
      </section>
    }

    @if (kind !== 'hidden') {
      <section class="fg-sec sc-in-filter" id="sc-in-filter">
        <h5>⏷ {{ 'sc.in.filter' | t }}</h5>
        <label>{{ 'sc.in.filterCollection' | t }}
          <select id="sc-in-filter-collection" (change)="setFilterCollection(val($event))">
            <option value="" [selected]="!n.props?.filter">{{ 'sc.in.filterNone' | t }}</option>
            @for (c of collections(); track c.id) { <option [value]="c.id" [selected]="n.props?.filter?.collection === c.id">{{ c.label }}</option> }
          </select>
        </label>
        @if (n.props?.filter; as f) {
          <label>{{ 'sc.in.filterParam' | t }}
            @if (filterParams().length) {
              <select id="sc-in-filter-param" (change)="setFilterParam(val($event))">
                @for (p of filterParams(); track p) { <option [value]="p" [selected]="f.param === p">{{ p }}</option> }
              </select>
            } @else {
              <input id="sc-in-filter-param" class="mono" [value]="f.param" (change)="setFilterParam(val($event))" />
            }
          </label>
          @if (queryName(); as q) {
            <p class="hint" id="sc-in-filter-url">{{ 'sc.in.filterUrl' | t: { q: q } }}
              @if (node().props?.name !== q) { <button class="fg-link" id="sc-in-filter-usename" (click)="props({ name: q })">{{ 'sc.in.useName' | t }}</button> }
            </p>
          }
        }
        @if (!collections().length) { <p class="hint">{{ 'sc.in.filterNoCollections' | t }}</p> }
        <p class="hint">{{ 'sc.in.filterHint' | t }}</p>
      </section>
    }
  `,
})
export class InputPanel extends CodePanelBase {
  readonly kinds: InputKind[] = ['text', 'search', 'number', 'email', 'select', 'checkbox', 'textarea', 'hidden'];
  readonly nameOk = computed(() => /^[A-Za-z_][A-Za-z0-9_.\-[\]]{0,63}$/.test(this.node().props?.name ?? ''));
  readonly collections = computed(() => collectionChoices(this.store.doc()));
  readonly sources = computed(() => [
    ...(this.code.bindings()?.collections ?? []).map(c => ({ value: 'code:' + c.name, label: labelOf(c.label, c.name) + ' · code' })),
    ...(this.code.bindings()?.tables ?? []).map(t => ({ value: 'table:' + t.name, label: t.name + ' · table' })),
  ]);
  readonly fromFields = computed(() => {
    const f = this.node().props?.optionsFrom;
    const declared = Object.keys(sourceFields(f?.collection, this.code.bindings()));
    if (declared.length) return declared;
    const items = this.code.preview(f?.collection ? this.code.sourceRequest(f.collection) : null)?.items ?? [];
    return items.length ? Object.keys(items[0]) : [f?.value, f?.label].filter((x): x is string => !!x);
  });
  readonly builtOptions = computed<InputOption[]>(() => {
    const f = this.node().props?.optionsFrom;
    return buildInputOptions(this.node().props, this.code.preview(f?.collection ? this.code.sourceRequest(f.collection) : null)?.items);
  });
  private readonly filterTarget = computed(() => this.code.collectionNode(this.node().props?.filter?.collection));
  readonly filterParams = computed(() => Object.keys(sourceParams(this.filterTarget()?.props?.source, this.code.bindings())));
  readonly queryName = computed(() => {
    const t = this.filterTarget(), f = this.node().props?.filter;
    return t && f?.param ? filterQueryName(t, f.param) : '';
  });

  setName(v: string) { this.props({ name: v.trim() || undefined }); }
  useStatic() { this.props({ optionsFrom: undefined, options: this.node().props?.options ?? [] }); }
  useFrom() {
    const s = this.sources()[0]?.value ?? '';
    const fields = Object.keys(sourceFields(s, this.code.bindings()));
    this.props({ optionsFrom: { collection: s, value: fields[0] ?? 'id', label: fields.find(f => /name|title|label/.test(f)) ?? fields[0] ?? 'name' } });
  }
  setFrom(p: Partial<NonNullable<Props['optionsFrom']>>) {
    const cur = this.node().props?.optionsFrom ?? { collection: '', value: '', label: '' };
    const next = { ...cur, ...p };
    if (p.collection !== undefined && p.collection !== cur.collection) {
      const fields = Object.keys(sourceFields(p.collection, this.code.bindings()));
      if (fields.length) { next.value = fields.includes(next.value) ? next.value : fields[0]; next.label = fields.includes(next.label) ? next.label : fields.find(f => /name|title|label/.test(f)) ?? fields[0]; }
    }
    this.props({ optionsFrom: next });
  }
  addOption() { this.props({ options: [...(this.node().props?.options ?? []), { value: '', label: '' }] }); }
  setOption(i: number, p: Partial<InputOption>) {
    const list = [...(this.node().props?.options ?? [])];
    list[i] = { ...list[i], ...p };
    this.props({ options: list });
  }
  removeOption(i: number) { this.props({ options: (this.node().props?.options ?? []).filter((_, j) => j !== i) }); }
  setFilterCollection(id: string) {
    if (!id) { this.props({ filter: undefined }); return; }
    const t = this.code.collectionNode(id);
    const params = Object.keys(sourceParams(t?.props?.source, this.code.bindings()));
    const cur = this.node().props?.filter?.param;
    const name = this.node().props?.name;
    const param = cur && (!params.length || params.includes(cur)) ? cur : params.find(p => p === name) ?? params[0] ?? name ?? 'q';
    this.props({ filter: { collection: id, param } });
  }
  setFilterParam(p: string) {
    const f = this.node().props?.filter;
    if (f && p.trim()) this.props({ filter: { ...f, param: p.trim() } });
  }
}

// ======================================================================== conditions

/** "Conditions" section of every layer: show only when an expression is truthy (+ quick presets). */
@Component({
  selector: 'rea-conditions-panel',
  imports: [TPipe, BindingInput],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let n = node();
    <section class="fg-sec sc-cond" id="sc-conditions">
      <h5>👁‍🗨 {{ 'sc.cond.title' | t }}</h5>
      <label>{{ 'sc.cond.showWhen' | t }}
        <rea-binding-input mode="expr" [nodeId]="n.id" [value]="n.props?.showWhen ?? ''" inputId="sc-cond-expr" placeholder="item.stock > 0" (changed)="set($event)" />
      </label>
      <div class="fg-row2">
        <select id="sc-cond-preset" (change)="preset($event)">
          <option value="">{{ 'sc.cond.presets' | t }}</option>
          @for (p of presets(); track p.id) { <option [value]="p.id">{{ p.label | t: p.params }}</option> }
        </select>
        @if (n.props?.showWhen) { <button class="fg-link" id="sc-cond-clear" (click)="set('')">{{ 'sc.cond.clear' | t }}</button> }
      </div>
      @if (state(); as st) {
        <p class="hint sc-cond-state" id="sc-cond-state" [attr.data-state]="st">{{ ('sc.cond.state.' + st) | t }}</p>
      }
      <p class="hint">{{ 'sc.cond.hint' | t }}</p>
    </section>
  `,
})
export class ConditionsPanel extends CodePanelBase {
  private readonly col = computed(() => enclosingCollection(this.store.doc(), this.node().id));
  readonly presets = computed(() => {
    const src = this.code.suggestSource(this.node().id);
    return conditionPresets(!!this.col(), this.code.itemFields(this.col()), src.queryKeys.filter(k => k !== 'page'), this.code.visitorGroups());
  });
  /** Shown / hidden in the current preview (first copy of the layer). */
  readonly state = computed<'shown' | 'hidden' | 'error' | null>(() => {
    const w = this.node().props?.showWhen?.trim();
    if (!w) return null;
    if (!parseExpr(w).ok) return 'error';
    return truthy(evalExpr(w, this.code.nodeExprCtx(this.node().id), this.code.evalOpts()).value) ? 'shown' : 'hidden';
  });
  set(v: string) { this.props({ showWhen: v.trim() || undefined }); }
  async preset(e: Event) {
    const el = e.target as HTMLSelectElement;
    const p = this.presets().find(x => x.id === el.value);
    el.value = '';
    if (!p) return;
    if (p.id !== GROUP_ASK) { this.set(p.expr); return; }
    // "visitor in group…": ask for the group name (declared groups have their own presets)
    const g = await Dialog.prompt({ title: tr('sc.cond.groupAskTitle'), placeholder: 'member', label: tr('sc.cond.groupAskLabel') });
    if (g === null) return;
    const expr = groupCondition(g);
    if (expr) this.set(expr); else Dialog.error(tr('sc.cond.groupInvalid'));
  }
}

