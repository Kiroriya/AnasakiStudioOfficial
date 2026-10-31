import { ChangeDetectionStrategy, Component, computed, inject, input, output, signal } from '@angular/core';
import { Router } from '@angular/router';
import { Api } from '../core/api';
import { Dialog } from '../core/dialog';
import { TPipe, tr } from '../core/i18n';
import { EditorStore } from './editor-store';
import { starterPage } from './presets';
import { safeUrl } from '../shared/safe-url';
import './messages-links';

/**
 * A link field: type an address or pick one of the site's pages; warns when
 * a site address has no page yet and can create that page right away.
 */
@Component({
  selector: 'rea-link-input',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <span class="li">
      <input [value]="value()" placeholder="/contact, https://…" (change)="typed($event)" />
      <select (change)="pick($event)" [title]="'lk.pick' | t" aria-label="pages">
        <option value="">📄</option>
        @for (o of options(); track o.path) { <option [value]="o.path">{{ o.label }} ({{ o.shown }})</option> }
      </select>
    </span>
    @if (missing(); as m) {
      <span class="li-warn">⚠ {{ 'lk.missing' | t: { path: m } }}
        <button class="fg-link" [disabled]="busy()" (click)="create(m)">＋ {{ 'lk.create' | t }}</button></span>
    }
  `,
  styles: [`
    :host { display: block; }
    .li { display: flex; gap: 4px; }
    .li input { flex: 1; min-width: 0; }
    .li select { width: 38px; flex: none; padding: 0 2px; }
    .li-warn { display: flex; flex-wrap: wrap; gap: 4px 8px; margin-top: 4px; font-size: 11px; color: #b45309; line-height: 1.4; }
  `],
})
export class LinkInput {
  readonly store = inject(EditorStore);
  private api = inject(Api);
  private router = inject(Router);
  readonly value = input('');
  readonly changed = output<string>();
  readonly busy = signal(false);

  readonly options = computed(() => this.store.sitePages().map(p => {
    const path = this.store.pathOf(p);
    let shown = path;
    try { shown = decodeURIComponent(path); } catch { /* keep */ }
    return { path, shown, label: p.title };
  }));

  /** A site address (/x) that no page has. Posts, categories, files and anchors are not checked. */
  readonly missing = computed(() => {
    const v = this.value().trim().split(/[?#]/)[0];
    if (!v.startsWith('/') || v.startsWith('//') || v === '/') return '';
    const seg = v.replace(/^\/+|\/+$/g, '');
    if (!seg || seg.includes('/') || seg === (this.store.settings()['blog_base'] || 'blog') || /\.[a-z0-9]{2,5}$/i.test(seg)) return '';
    if (!this.store.sitePages().length) return '';
    const norm = (s: string) => { try { return decodeURIComponent(s).toLowerCase(); } catch { return s.toLowerCase(); } };
    return this.store.sitePages().some(p => norm(p.slug) === norm(seg)) ? '' : v;
  });

  set(v: string) {
    const safe = safeUrl(v);
    if (safe === null) { this.store.notify(tr('url.unsafe'), true); return false; }
    this.changed.emit(safe);
    return true;
  }

  /** A typed address: a rejected one is put back to the last good value. */
  typed(e: Event) {
    const el = e.target as HTMLInputElement;
    if (!this.set(el.value)) el.value = this.value();
  }

  pick(e: Event) {
    const el = e.target as HTMLSelectElement;
    if (el.value) this.set(el.value);
    el.value = '';
  }

  /** Creates the missing page as a draft with the site header/footer, then offers to open it. */
  async create(path: string) {
    const slug = path.replace(/^\/+|\/+$/g, '');
    const known: Record<string, string> = { contact: tr('lk.t.contact'), about: tr('lk.t.about'), services: tr('lk.t.services'), menu: tr('lk.t.menu'), shop: tr('lk.t.shop') };
    const title = (await Dialog.prompt({ title: tr('lk.createPrompt', { path }), value: known[slug.toLowerCase()] ?? slug }))?.trim();
    if (!title) return;
    this.busy.set(true);
    try {
      const p = await this.api.createPost({ type: 'page', title, slug, status: 'draft', design: starterPage(this.store.components(), 'page', this.store.settings()) });
      await this.store.loadSitePages();
      const go = await Dialog.confirm({ title: tr('lk.created', { title, path: this.store.pathOf(p) }), confirm: tr('lk.open'), cancel: tr('lk.stay') });
      if (go) {
        if (this.store.dirty()) {
          // a failed save (error toast / conflict dialog shown) must not lose the changes by leaving
          try { await this.store.save(); } catch { return; }
        }
        this.router.navigate(['/design', p.id]);
      }
    } catch (e) {
      this.store.notify(String((e as Error)?.message ?? e), true);
    } finally {
      this.busy.set(false);
    }
  }
}
