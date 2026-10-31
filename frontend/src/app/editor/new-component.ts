import { ChangeDetectionStrategy, Component, OnInit, computed, inject, input, output, signal } from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { Api, errorMessage } from '../core/api';
import { Dialog } from '../core/dialog';
import { TPipe, tr } from '../core/i18n';
import { DesignComponent } from '../core/models';
import { STARTERS, Starter, fitFrame, freshIds, previewDoc } from './component-starters';
import { DNode } from './design-types';
import { siteRoleIds } from './site-role';

type Group = Starter['group'] | 'copy';
export interface NewComponentResult {
  component: DesignComponent;
  /** Set when it became the site header/footer… */
  siteRole: 'header' | 'footer' | null;
  /** …replacing this component (null: there was none). */
  replacedId: number | null;
}

/**
 * Gallery for a new component: ready-made headers, footers, sections and
 * cards (with a live preview from the site renderer), or a copy of an
 * existing component. A header/footer can replace the site's one right away.
 */
@Component({
  selector: 'rea-new-component',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="modal-backdrop" (click)="closed.emit()">
      <div class="modal modal-lg nc" (click)="$event.stopPropagation()" role="dialog" aria-modal="true">
        <header class="modal-head"><h3>◈ {{ 'cs.title' | t }}</h3><button class="btn btn-ghost btn-sm" (click)="closed.emit()" aria-label="close">✕</button></header>
        <p class="muted small nc-sub">{{ 'cs.pick' | t }}</p>
        <div class="nc-body">
          <aside class="nc-list">
            @for (g of groups; track g) {
              <h5>{{ 'cs.g.' + g | t }}</h5>
              @if (g === 'copy') {
                @for (c of existing(); track c.id) {
                  <button class="nc-item" [class.on]="copyOf()?.id === c.id" (click)="pickCopy(c)"><i>◈</i><span>{{ c.name }}</span></button>
                } @empty { <p class="hint">{{ 'cs.noneToCopy' | t }}</p> }
              } @else {
                @for (s of byGroup(g); track s.id) {
                  <button class="nc-item" [class.on]="starter()?.id === s.id" (click)="pick(s)"><i>{{ s.icon }}</i><span>{{ 'cs.s.' + s.id | t }}</span></button>
                }
              }
            }
          </aside>
          <section class="nc-main">
            <div class="nc-pv-head">
              <b>{{ 'cs.preview' | t }}</b>
              <div class="seg">
                <button [class.on]="!mobile()" (click)="setMobile(false)">🖥 {{ 'cs.desktop' | t }}</button>
                <button [class.on]="mobile()" (click)="setMobile(true)">📱 {{ 'cs.mobile' | t }}</button>
              </div>
            </div>
            <div class="nc-pv" [class.phone]="mobile()">
              @if (html(); as h) {
                <iframe sandbox="allow-same-origin" [srcdoc]="h" (load)="fit($event)" title="preview"></iframe>
              } @else { <div class="nc-loading">…</div> }
            </div>
            <p class="hint">{{ 'cs.previewHint' | t }}</p>
            <label class="nc-field">{{ 'cs.name' | t }}<input [value]="name()" (input)="name.set($any($event.target).value)" maxlength="80" /></label>
            @if (role(); as r) {
              <label class="nc-check"><input type="checkbox" [checked]="useForSite()" (change)="useForSite.set($any($event.target).checked)" />
                {{ (r === 'header' ? 'cs.useHeader' : 'cs.useFooter') | t }}</label>
              <p class="hint">{{ 'cs.useHint' | t }}</p>
            }
            <div class="nc-actions">
              <button class="btn" (click)="closed.emit()">{{ 'common.cancel' | t }}</button>
              <button class="btn btn-primary" [disabled]="busy() || !name().trim()" (click)="create()">{{ (placeHere() ? 'cs.createPlace' : 'cs.create') | t }} →</button>
            </div>
          </section>
        </div>
      </div>
    </div>
  `,
  styles: [`
    .nc { width: min(1080px, 96vw); padding: 20px 22px; }
    .nc-sub { margin: -8px 0 12px; }
    .nc-body { display: grid; grid-template-columns: 230px 1fr; gap: 18px; min-height: 0; }
    .nc-list { max-height: 62vh; overflow: auto; padding-right: 4px; }
    .nc-list h5 { margin: 12px 0 6px; font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--muted, #6b7280); }
    .nc-list h5:first-child { margin-top: 0; }
    .nc-item { display: flex; align-items: center; gap: 10px; width: 100%; text-align: left; border: 1px solid transparent; background: none; padding: 8px 10px; border-radius: 10px; cursor: pointer; font: inherit; font-size: 14px; color: inherit; }
    .nc-item:hover { background: var(--hover, #f3f4f6); }
    .nc-item.on { border-color: var(--primary, #4f46e5); background: color-mix(in srgb, var(--primary, #4f46e5) 8%, transparent); }
    .nc-item i { font-style: normal; width: 28px; height: 28px; display: grid; place-items: center; border-radius: 8px; background: var(--hover, #f3f4f6); flex: none; }
    .nc-item span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .nc-main { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
    .nc-pv-head { display: flex; align-items: center; justify-content: space-between; }
    .seg { display: inline-flex; border: 1px solid var(--border, #e5e7eb); border-radius: 8px; overflow: hidden; }
    .seg button { border: 0; background: none; padding: 5px 10px; font: inherit; font-size: 12px; cursor: pointer; color: inherit; }
    .seg button.on { background: var(--primary, #4f46e5); color: #fff; }
    .nc-pv { height: 330px; border: 1px solid var(--border, #e5e7eb); border-radius: 12px; overflow: hidden; background: #f3f4f6; position: relative; }
    .nc-pv iframe { border: 0; background: #fff; display: block; position: absolute; visibility: hidden; }
    .nc-pv iframe[style*=scale] { visibility: visible; box-shadow: 0 4px 24px rgba(17,24,39,.08); }
    .nc-loading { display: grid; place-items: center; height: 100%; color: #9ca3af; }
    .nc-field { display: flex; flex-direction: column; gap: 4px; font-size: 13px; font-weight: 600; }
    .nc-field input { font: inherit; font-weight: 400; }
    .nc-check { display: flex; gap: 8px; align-items: center; font-size: 14px; }
    .nc-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: auto; }
    @media (max-width: 720px) { .nc-body { grid-template-columns: 1fr; } .nc-list { max-height: 200px; } }
  `],
})
export class NewComponent implements OnInit {
  private api = inject(Api);
  private sanitizer = inject(DomSanitizer);

  /** Group selected first (e.g. 'header' from "New header"). */
  readonly startGroup = input<Group>('header');
  /** Opened from the designer: the new component is placed on the open page. */
  readonly placeHere = input(false);
  /** Fired with the new component (and whether it became the site header/footer). */
  readonly created = output<NewComponentResult>();
  readonly closed = output<void>();

  readonly groups: Group[] = ['header', 'footer', 'section', 'block', 'copy'];
  readonly existing = signal<DesignComponent[]>([]);
  readonly starter = signal<Starter | null>(null);
  readonly copyOf = signal<DesignComponent | null>(null);
  readonly name = signal('');
  readonly useForSite = signal(true);
  readonly mobile = signal(false);
  readonly html = signal<SafeHtml | null>(null);
  readonly busy = signal(false);
  private node: DNode | null = null;
  private menuId?: number;
  private seq = 0;
  private settings: Record<string, string> = {};

  readonly role = computed(() => this.starter()?.role ?? null);

  fit(e: Event) { fitFrame(e.target as HTMLIFrameElement, this.mobile() ? 390 : 1280); }

  byGroup(g: Group) { return STARTERS.filter(s => s.group === g); }

  async ngOnInit() {
    const [menus, comps, settings] = await Promise.all([
      this.api.menus().catch(() => []), this.api.components().catch(() => []), this.api.settings().catch(() => ({})),
    ]);
    this.menuId = menus[0]?.id;
    this.settings = settings;
    this.existing.set(comps);
    const g = this.startGroup();
    const first = g === 'copy' ? null : this.byGroup(g)[0];
    if (first) this.pick(first);
    else if (comps[0]) this.pickCopy(comps[0]);
  }

  pick(s: Starter) {
    this.starter.set(s);
    this.copyOf.set(null);
    this.node = s.build(this.menuId);
    this.name.set(this.node.name ?? tr('cs.blank'));
    this.render();
  }

  pickCopy(c: DesignComponent) {
    this.starter.set(null);
    this.copyOf.set(c);
    this.node = freshIds(c.node);
    this.name.set(tr('cs.copyOf', { name: c.name }));
    this.render();
  }

  setMobile(on: boolean) { this.mobile.set(on); this.render(); }

  private async render() {
    if (!this.node) return;
    const seq = ++this.seq;
    this.html.set(null);
    try {
      const page = await this.api.preview(previewDoc(this.node), undefined, this.name());
      // the preview runs in a sandboxed frame (no scripts, no same-origin access)
      if (seq === this.seq) this.html.set(this.sanitizer.bypassSecurityTrustHtml(page));
    } catch { /* the preview is a nicety; creating still works */ }
  }

  async create() {
    if (!this.node || this.busy()) return;
    this.busy.set(true);
    try {
      const name = this.name().trim();
      const comp = await this.api.saveComponent({ name, node: { ...this.node, name } });
      const r = this.useForSite() ? this.role() : null;
      const replaced = r ? siteRoleIds(this.existing(), this.settings)[r] : null;
      if (r) {
        const res = await this.api.setComponentSiteRole(comp.id, r);
        Dialog.toast(tr('cs.madeSite', { role: tr(r === 'header' ? 'cs.roleHeader' : 'cs.roleFooter'), n: res.replaced + res.added }));
      } else {
        Dialog.toast(tr('cs.made', { name }));
      }
      this.created.emit({ component: comp, siteRole: r, replacedId: replaced });
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
