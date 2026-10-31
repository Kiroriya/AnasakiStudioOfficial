import { ChangeDetectionStrategy, Component, Injectable, computed, inject, output, signal } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { Api } from '../core/api';
import { Auth } from '../core/auth';
import { TPipe, localized } from '../core/i18n';
import { CodeUnit, SITE_UNIT, normalizeUnits, pickUnit } from './units-utils';
import './messages-code4b';

/**
 * Code units of the Site Code area (phase 4b): 'site' + each active JS plugin.
 * The selected unit is shared by the Logs / Access / Settings / Jobs tabs
 * (and mirrored in `?unit=` so a link opens the same view).
 */
@Injectable({ providedIn: 'root' })
export class CodeUnits {
  private api = inject(Api);
  private auth = inject(Auth);
  readonly units = signal<CodeUnit[]>([{ id: SITE_UNIT, name: SITE_UNIT }]);
  readonly current = signal<string>(SITE_UNIT);
  readonly pluginIds = computed(() => this.units().filter(u => u.id !== SITE_UNIT).map(u => u.id));
  private loading: Promise<void> | null = null;
  private loadedAt = 0;

  /** (Re)loads the list at most every 30 s; resolves when known. */
  load(force = false): Promise<void> {
    if (!force && this.loading && Date.now() - this.loadedAt < 30_000) return this.loading;
    this.loadedAt = Date.now();
    this.loading = (async () => {
      const status = await this.api.codeStatus().catch(() => null);
      let pkgs: { id: string; name?: string; runtime?: string; active?: boolean; version?: string }[] = [];
      if (!status?.units?.length && this.auth.can('manage_options')) pkgs = await this.api.packages().catch(() => []);
      const list = normalizeUnits(status?.units, pkgs);
      this.units.set(list);
      this.current.set(pickUnit(this.current(), list));
    })();
    return this.loading;
  }

  set(unit: string) { this.current.set(pickUnit(unit, this.units())); }

  /** Called by a page before its first load: `?unit=` wins (validated once the list is known). */
  adopt(route: ActivatedRoute) {
    const want = route.snapshot.queryParamMap.get('unit');
    if (want) this.current.set(want);
  }

  /** Query value for API calls: always sent (the server defaults to 'site' too). */
  param() { return this.current(); }
}

/** "Site code / <plugin>…" switcher shown above Logs, Access, Settings and Jobs. Hidden while there is only Site Code. */
@Component({
  selector: 'rea-unit-switcher',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (units.units().length > 1) {
      <div class="unit-switcher" id="code-unit-switcher" role="group" [attr.aria-label]="'cu.label' | t">
        <span class="muted small">{{ 'cu.label' | t }}</span>
        @for (u of units.units(); track u.id) {
          <button type="button" class="unit-btn" [class.on]="units.current() === u.id" [attr.data-unit]="u.id" [attr.aria-pressed]="units.current() === u.id" (click)="pick(u.id)">
            {{ u.id === site ? '⌨️ ' + ('cu.site' | t) : '🧩 ' + name(u) }}
          </button>
        }
        <select class="unit-select" id="code-unit-select" [attr.aria-label]="'cu.label' | t" (change)="pick($any($event.target).value)">
          @for (u of units.units(); track u.id) { <option [value]="u.id" [selected]="units.current() === u.id">{{ u.id === site ? ('cu.site' | t) : name(u) }}</option> }
        </select>
      </div>
    }
  `,
  styles: `
    .unit-switcher { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin: 0 0 12px; }
    .unit-btn { border: 1px solid var(--border); background: #fff; border-radius: 999px; padding: 3px 12px; font-size: 13px; cursor: pointer; color: inherit; }
    .unit-btn.on { background: var(--primary); border-color: var(--primary); color: #fff; }
    .unit-select { display: none; width: auto; }
    @media (max-width: 640px) { .unit-btn { display: none; } .unit-select { display: inline-block; } }
  `,
})
export class UnitSwitcher {
  readonly units = inject(CodeUnits);
  private router = inject(Router);
  readonly site = SITE_UNIT;
  /** Emits the new unit after a change (the page reloads its data). */
  readonly changed = output<string>();

  constructor() {
    const before = this.units.current();
    void this.units.load().then(() => {
      // an unknown ?unit= fell back to 'site': the page must reload
      if (before !== this.units.current()) this.changed.emit(this.units.current());
    });
  }

  name(u: CodeUnit) { return localized(u.label) || (u.name !== u.id ? `${u.name} (${u.id})` : u.id); }

  pick(id: string) {
    if (id === this.units.current()) return;
    this.units.set(id);
    void this.router.navigate([], { queryParams: { unit: id === SITE_UNIT ? null : id }, queryParamsHandling: 'merge', replaceUrl: true });
    this.changed.emit(this.units.current());
  }
}
