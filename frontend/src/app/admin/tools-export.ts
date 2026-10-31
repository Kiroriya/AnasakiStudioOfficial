import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { saveBlob } from '../shared/download';
import { Dialog } from '../core/dialog';
import { I18n, TPipe } from '../core/i18n';
import { PackageImport, ToolsImportResult } from '../core/legacy';
import './messages-phase7';

/** Counts of an import result in display order. */
export function importCounts(r: ToolsImportResult | null): { key: string; n: number }[] {
  if (!r) return [];
  return ['posts', 'terms', 'menus', 'components', 'media'].filter(k => k in (r.imported ?? {})).map(k => ({ key: k, n: r.imported[k] }));
}

/**
 * Settings → Export / import site (core system tool since phase 7; the former backup plugin):
 * download everything as one .zip and add it to any Anasaki Studio site.
 */
@Component({
  selector: 'rea-tools-export',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">💾</span>
      <div class="page-head-text"><h1>{{ 'tools.title' | t }}</h1><p>{{ 'tools.desc' | t }}</p></div>
    </header>
    <div class="grid-2">
      <section class="card backup-card">
        <div class="backup-icon">⬇</div>
        <h3>{{ 'tools.export' | t }}</h3>
        <p class="muted">{{ 'tools.exportDesc' | t }}</p>
        <button id="tools-export" class="btn btn-primary" [disabled]="busy()" (click)="download()">{{ (busy() === 'export' ? 'tools.working' : 'tools.exportBtn') | t }}</button>
      </section>
      <section class="card backup-card">
        <div class="backup-icon">⬆</div>
        <h3>{{ 'tools.import' | t }}</h3>
        <p class="muted">{{ 'tools.importDesc' | t }}</p>
        <label class="btn" [class.disabled]="busy()">
          {{ (busy() === 'import' ? 'tools.working' : 'tools.importBtn') | t }}
          <input id="tools-import" type="file" accept=".zip,application/zip" hidden (change)="restore($event)" />
        </label>
        @if (result(); as r) {
          <ul class="list small backup-result">
            @for (c of counts(); track c.key) { <li><span>{{ 'tools.k.' + c.key | t }}</span><b>{{ c.n }}</b></li> }
          </ul>
          @if (r.siteCode && (r.siteCode.added.length || r.siteCode.skipped)) {
            <p class="hint">{{ 'tools.siteCode' | t: { n: r.siteCode.added.length, s: r.siteCode.skipped } }}</p>
          }
          @if (r.packages.length) {
            <h4>{{ 'tools.packages' | t }}</h4>
            <ul class="list small tools-packages">
              @for (p of r.packages; track p.id) {
                <li class="tools-pkg" [attr.data-id]="p.id" [attr.data-status]="p.status">
                  <span><b>{{ p.name || p.id }}</b> <span class="muted">v{{ p.version }}</span></span>
                  @if (p.status === 'installed') {
                    <span class="muted">{{ 'tools.pkg.installed' | t: { v: p.installedVersion ?? '' } }}</span>
                  } @else if (installed().has(p.id)) {
                    <span>✓</span>
                  } @else {
                    <button class="btn btn-sm" [disabled]="!!installing()" (click)="install(p)">{{ installing() === p.id ? ('tools.working' | t) : ('tools.pkg.install' | t) }}</button>
                  }
                </li>
              }
            </ul>
          }
        }
      </section>
    </div>
  `,
})
export class ToolsExport {
  private api = inject(Api);
  private i18n = inject(I18n);
  readonly busy = signal<'export' | 'import' | null>(null);
  readonly result = signal<ToolsImportResult | null>(null);
  readonly installing = signal<string | null>(null);
  readonly installed = signal<Set<string>>(new Set());

  counts() { return importCounts(this.result()); }

  async download() {
    this.busy.set('export');
    try {
      const blob = await this.api.toolsExport();
      saveBlob(blob, `anasaki-backup-${new Date().toISOString().slice(0, 10)}.zip`);
    } catch (e) {
      Dialog.error(errorMessage(e));
    } finally {
      this.busy.set(null);
    }
  }

  async restore(e: Event) {
    const input = e.target as HTMLInputElement;
    const f = input.files?.[0];
    input.value = '';
    if (!f) return;
    if (!(await Dialog.confirm({ title: this.i18n.t('tools.confirm'), text: this.i18n.t('tools.confirmText'), icon: 'warning' }))) return;
    this.busy.set('import');
    try {
      this.result.set(await this.api.toolsImport(f));
      this.installed.set(new Set());
      Dialog.toast(this.i18n.t('tools.done'));
    } catch (err) {
      Dialog.error(errorMessage(err));
    } finally {
      this.busy.set(null);
    }
  }

  /** Installs a plugin listed in the backup from the Store (normal consent: it stays off) and applies its settings. */
  async install(p: PackageImport) {
    this.installing.set(p.id);
    try {
      // the backup's version when the Store still has it, else the latest
      await this.api.installPackage(p.id, p.version).catch(() => this.api.installPackage(p.id));
      if (p.settings && Object.keys(p.settings).length) {
        try { await this.api.savePackageSettings(p.id, p.settings); } catch { /* schema changed: keep the defaults */ }
      }
      this.installed.update(s => new Set(s).add(p.id));
      Dialog.toast(this.i18n.t('tools.pkg.installedNow', { name: p.name || p.id }));
    } catch (err) {
      Dialog.error(errorMessage(err));
    } finally {
      this.installing.set(null);
    }
  }
}
