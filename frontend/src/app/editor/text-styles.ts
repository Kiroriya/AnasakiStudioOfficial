import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { Dialog } from '../core/dialog';
import { TPipe, tr } from '../core/i18n';
import { FONTS, STYLE_ID, TEXT_STYLE_KEYS, Theme, TextStyleDef, TextStyleValues } from './design-css';
import { DNode, Style } from './design-types';
import { EditorStore } from './editor-store';
import './messages-styles';

/** A short id from a name ("Hero title" → "hero-title"), unique among `taken`. */
function styleId(name: string, taken: Set<string>): string {
  let base = name.toLowerCase().normalize('NFKD').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 18);
  if (!/^[a-z]/.test(base)) base = 's' + (base ? '-' + base : '');
  let id = base, i = 2;
  while (taken.has(id) || !STYLE_ID.test(id)) id = `${base.slice(0, 18)}-${i++}`;
  return id;
}

/** Edits the theme draft (saved with "Save theme"). */
function editTheme(store: EditorStore, fn: (t: Theme) => void) {
  const t = structuredClone(store.liveTheme());
  fn(t);
  store.themeDraft.set(t);
}

/** Text section: pick a shared text style for the layer, make one from it, or detach. */
@Component({
  selector: 'rea-text-style',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="ts-pick">
      <select [value]="current()?.id ?? ''" (change)="apply($any($event.target).value)" [title]="'st.textStyleTip' | t">
        <option value="">{{ 'st.noStyle' | t }}</option>
        @for (s of store.liveTheme().textStyles; track s.id) { <option [value]="s.id" [selected]="current()?.id === s.id">{{ s.name }} · {{ s.style.fontSize ?? '' }}</option> }
      </select>
      @if (current()) {
        <button class="fg-link" [title]="'st.detachTip' | t" (click)="detach()">{{ 'st.detach' | t }}</button>
      } @else {
        <button class="fg-link" [title]="'st.createTip' | t" (click)="create()">＋ {{ 'st.create' | t }}</button>
      }
    </div>
    @if (current() && modified().length) {
      <p class="hint ts-mod">{{ 'st.modified' | t: { n: modified().length } }}
        <button class="fg-link" (click)="reset()">{{ 'st.reset' | t }}</button>
        <button class="fg-link" (click)="update()">{{ 'st.update' | t }}</button></p>
    }
  `,
  styles: [`
    .ts-pick { display: flex; gap: 6px; align-items: center; margin-bottom: 6px; }
    .ts-pick select { flex: 1; min-width: 0; }
    .ts-mod { display: flex; flex-wrap: wrap; gap: 4px 8px; align-items: center; }
  `],
})
export class TextStylePicker {
  readonly store = inject(EditorStore);
  readonly node = input.required<DNode>();

  readonly current = computed(() => {
    const id = this.node().style?.textStyle;
    return id ? this.store.liveTheme().textStyles.find(s => s.id === id) ?? null : null;
  });
  /** Text keys the layer sets itself on top of its style. */
  readonly modified = computed(() => TEXT_STYLE_KEYS.filter(k => this.node().style?.[k] !== undefined));

  private clear(): Partial<Style> {
    const p: Partial<Style> = {};
    for (const k of TEXT_STYLE_KEYS) (p as Record<string, unknown>)[k] = undefined;
    return p;
  }

  /** A style replaces the layer's own text values (like Figma). */
  apply(id: string) {
    const n = this.node();
    this.store.setStyleAll(n.id, id ? { ...this.clear(), textStyle: id } : { textStyle: undefined });
  }

  reset() { this.store.setStyleAll(this.node().id, this.clear()); }

  /** Writes the layer's own values into the shared style (every layer using it changes). */
  update() {
    const n = this.node(), cur = this.current();
    if (!cur) return;
    const own: Record<string, unknown> = {};
    for (const k of this.modified()) own[k] = n.style![k];
    editTheme(this.store, t => {
      const s = t.textStyles.find(x => x.id === cur.id);
      if (s) s.style = { ...s.style, ...(own as TextStyleValues) };
    });
    this.reset();
    this.store.notify(tr('st.updated', { name: cur.name }));
  }

  /** Copies the style's values into the layer and unlinks it. */
  detach() {
    const cur = this.current();
    if (!cur) return;
    this.store.setStyleAll(this.node().id, { ...cur.style, ...this.ownValues(), textStyle: undefined });
  }

  private ownValues(): Partial<Style> {
    const n = this.node(), out: Record<string, unknown> = {};
    for (const k of TEXT_STYLE_KEYS) if (n.style?.[k] !== undefined) out[k] = n.style[k];
    return out as Partial<Style>;
  }

  async create() {
    const name = await Dialog.prompt({ title: tr('st.createPrompt'), value: this.node().name || 'Text style' });
    if (!name) return;
    const n = this.node();
    const eff = this.store.eff(n);
    const vals: Record<string, unknown> = {};
    for (const k of TEXT_STYLE_KEYS) {
      const v = n.style?.[k] ?? (k === 'fontSize' ? eff.style?.fontSize ?? 16 : undefined);
      if (v !== undefined) vals[k] = v;
    }
    let id = '';
    editTheme(this.store, t => {
      id = styleId(name, new Set(t.textStyles.map(s => s.id)));
      t.textStyles.push({ id, name: name.slice(0, 40), style: vals as TextStyleValues });
    });
    this.apply(id);
    this.store.notify(tr('st.created', { name }));
  }
}

/** Theme panel: the site's color styles and text styles (saved with the theme). */
@Component({
  selector: 'rea-style-library',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <h6 class="sl-h">{{ 'st.colors' | t }} <button class="fg-add" [title]="'st.addColor' | t" (click)="addColor()">+</button></h6>
    @for (c of store.liveTheme().palette; track c.id) {
      <div class="sl-row">
        <input type="color" [value]="hex(c.value)" (input)="setColor(c.id, { value: $any($event.target).value })" />
        <input class="sl-name" [value]="c.name" (change)="setColor(c.id, { name: $any($event.target).value })" />
        <small class="sl-id">--c-{{ c.id }}</small>
        <button class="fg-rm" [title]="'common.delete' | t" (click)="removeColor(c.id)">−</button>
      </div>
    } @empty { <p class="hint">{{ 'st.noColors' | t }}</p> }

    <h6 class="sl-h">{{ 'st.texts' | t }} <button class="fg-add" [title]="'st.addText' | t" (click)="addText()">+</button></h6>
    @for (s of store.liveTheme().textStyles; track s.id) {
      <div class="sl-text" [class.open]="open() === s.id">
        <button class="sl-prev" (click)="open.set(open() === s.id ? null : s.id)"
                [style.font-size.px]="min(s.style.fontSize ?? 16, 26)" [style.font-weight]="s.style.fontWeight ?? 400"
                [style.text-transform]="s.style.textTransform ?? null" [style.font-family]="s.style.fontFamily ? s.style.fontFamily : null">
          {{ s.name }}<small>{{ s.style.fontSize ?? '' }}{{ s.tablet?.fontSize ? ' · ' + s.tablet!.fontSize : '' }}{{ s.mobile?.fontSize ? ' · ' + s.mobile!.fontSize : '' }}</small>
        </button>
        @if (open() === s.id) {
          <div class="sl-edit">
            <label>{{ 'cs.name' | t }}<input [value]="s.name" (change)="setText(s.id, 'name', $any($event.target).value)" /></label>
            <label>{{ 'st.font' | t }}
              <select [value]="s.style.fontFamily ?? ''" (change)="setVal(s.id, 'style', 'fontFamily', $any($event.target).value || undefined)">
                <option value="">{{ 'ed.theme.fontDefault' | t }}</option>
                @for (f of fonts; track f) { <option [value]="f" [selected]="s.style.fontFamily === f">{{ f }}</option> }
              </select></label>
            <div class="sl-grid">
              <label>{{ 'st.size' | t }}<input type="number" min="1" [value]="s.style.fontSize ?? ''" (change)="setNum(s.id, 'style', 'fontSize', $event)" /></label>
              <label>{{ 'st.weight' | t }}
                <select [value]="'' + (s.style.fontWeight ?? '')" (change)="setNum(s.id, 'style', 'fontWeight', $event)">
                  <option value="">—</option>
                  @for (w of weights; track w) { <option [value]="'' + w" [selected]="s.style.fontWeight === w">{{ w }}</option> }
                </select></label>
              <label>{{ 'st.line' | t }}<input type="number" step="0.05" [value]="s.style.lineHeight ?? ''" (change)="setNum(s.id, 'style', 'lineHeight', $event)" /></label>
              <label>{{ 'st.spacing' | t }}<input type="number" step="0.1" [value]="s.style.letterSpacing ?? ''" (change)="setNum(s.id, 'style', 'letterSpacing', $event)" /></label>
              <label>📱↔ {{ 'st.tablet' | t }}<input type="number" min="1" [value]="s.tablet?.fontSize ?? ''" (change)="setNum(s.id, 'tablet', 'fontSize', $event)" /></label>
              <label>📱 {{ 'st.mobile' | t }}<input type="number" min="1" [value]="s.mobile?.fontSize ?? ''" (change)="setNum(s.id, 'mobile', 'fontSize', $event)" /></label>
            </div>
            <label>{{ 'st.case' | t }}
              <select [value]="s.style.textTransform ?? ''" (change)="setVal(s.id, 'style', 'textTransform', $any($event.target).value || undefined)">
                <option value="">—</option><option value="uppercase">ABC</option><option value="capitalize">Abc</option><option value="lowercase">abc</option>
              </select></label>
            <div class="sl-foot"><small>{{ 'st.usage' | t: { n: usage(s.id) } }}</small><button class="fg-link danger" (click)="removeText(s)">{{ 'common.delete' | t }}</button></div>
          </div>
        }
      </div>
    }
  `,
  styles: [`
    .sl-h { display: flex; align-items: center; justify-content: space-between; margin: 12px 0 6px; font-size: 11px; text-transform: uppercase; letter-spacing: .05em; color: var(--fg-muted, #888); }
    .sl-row { display: flex; align-items: center; gap: 6px; margin-bottom: 4px; }
    .sl-row input[type=color] { width: 26px; height: 22px; padding: 0; border: 1px solid #ddd; border-radius: 4px; flex: none; }
    .sl-name { flex: 1; min-width: 0; font-size: 12px; }
    .sl-id { color: var(--fg-muted, #999); font-size: 10px; }
    .sl-text { border-radius: 6px; margin-bottom: 2px; }
    .sl-text.open { background: var(--fg-hover, #f5f5f5); padding-bottom: 6px; }
    .sl-prev { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; width: 100%; border: 0; background: none; padding: 6px; border-radius: 6px; cursor: pointer; color: inherit; text-align: left; line-height: 1.2; }
    .sl-prev:hover { background: var(--fg-hover, #f2f2f2); }
    .sl-prev small { font-size: 10px; font-weight: 400; color: var(--fg-muted, #999); text-transform: none; font-family: Inter, sans-serif; white-space: nowrap; }
    .sl-edit { display: flex; flex-direction: column; gap: 6px; padding: 0 6px; font-size: 11px; color: var(--fg-muted, #777); }
    .sl-edit label { display: flex; flex-direction: column; gap: 2px; }
    .sl-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
    .sl-foot { display: flex; justify-content: space-between; align-items: center; }
    .danger { color: #dc2626; }
  `],
})
export class StyleLibrary {
  readonly store = inject(EditorStore);
  readonly open = signal<string | null>(null);
  readonly fonts = FONTS;
  readonly weights = [300, 400, 500, 600, 700, 800, 900];
  readonly min = Math.min;

  hex(v: string) { return /^#[0-9a-f]{6}$/i.test(v) ? v : /^#[0-9a-f]{3}$/i.test(v) ? '#' + [...v.slice(1)].map(c => c + c).join('') : '#888888'; }

  addColor() {
    editTheme(this.store, t => {
      const id = styleId('brand', new Set([...t.palette.map(c => c.id), 'primary', 'secondary', 'accent', 'text', 'muted', 'surface']));
      t.palette.push({ id, name: tr('st.newColor'), value: '#14b8a6' });
    });
  }
  setColor(id: string, patch: { name?: string; value?: string }) {
    editTheme(this.store, t => { const c = t.palette.find(x => x.id === id); if (c) Object.assign(c, patch); });
  }
  async removeColor(id: string) {
    editTheme(this.store, t => { t.palette = t.palette.filter(c => c.id !== id); });
  }

  addText() {
    let id = '';
    editTheme(this.store, t => {
      id = styleId('text', new Set(t.textStyles.map(s => s.id)));
      t.textStyles.push({ id, name: tr('st.newText'), style: { fontSize: 18, fontWeight: 600, lineHeight: 1.4 } });
    });
    this.open.set(id);
  }
  setText(id: string, key: 'name', v: string) {
    editTheme(this.store, t => { const s = t.textStyles.find(x => x.id === id); if (s) s[key] = v.slice(0, 40); });
  }
  setVal(id: string, where: 'style' | 'tablet' | 'mobile', key: keyof TextStyleValues, v: unknown) {
    editTheme(this.store, t => {
      const s = t.textStyles.find(x => x.id === id) as TextStyleDef | undefined;
      if (!s) return;
      const m = { ...(s[where] ?? {}) } as Record<string, unknown>;
      if (v === undefined || v === '' || (typeof v === 'number' && isNaN(v))) delete m[key]; else m[key] = v;
      if (where === 'style') s.style = m as TextStyleValues;
      else if (Object.keys(m).length) s[where] = m as TextStyleValues; else delete s[where];
    });
  }
  setNum(id: string, where: 'style' | 'tablet' | 'mobile', key: keyof TextStyleValues, e: Event) {
    const raw = (e.target as HTMLInputElement).value;
    this.setVal(id, where, key, raw === '' ? undefined : Number(raw));
  }
  usage(id: string) {
    let n = 0;
    const walk = (x: DNode) => { if (x.style?.textStyle === id) n++; x.children?.forEach(walk); };
    walk(this.store.doc());
    return n;
  }
  async removeText(s: TextStyleDef) {
    if (!(await Dialog.confirm({ title: tr('st.deleteText', { name: s.name }), text: tr('st.deleteTextHint'), danger: true }))) return;
    editTheme(this.store, t => { t.textStyles = t.textStyles.filter(x => x.id !== s.id); });
  }
}
