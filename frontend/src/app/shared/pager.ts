import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { TPipe } from '../core/i18n';

/**
 * Page navigation shared by every list: "1–20 of 135", prev/next and page numbers
 * with gaps (1 … 4 5 6 … 12). Hidden when everything fits on one page.
 */
@Component({
  selector: 'rea-pager',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (total() > 0) {
      <nav class="pager" [attr.aria-label]="'pager.label' | t">
        <span class="pager-info">{{ 'pager.range' | t: { from: from(), to: to(), total: total() } }}</span>
        @if (pages() > 1) {
          <span class="pager-pages">
            <button class="pager-btn" [disabled]="page() <= 1" (click)="go(page() - 1)" [attr.aria-label]="'common.prev' | t">‹</button>
            @for (p of numbers(); track $index) {
              @if (p === 0) { <span class="pager-gap">…</span> }
              @else { <button class="pager-btn" [class.on]="p === page()" [attr.aria-current]="p === page() ? 'page' : null" (click)="go(p)">{{ p }}</button> }
            }
            <button class="pager-btn" [disabled]="page() >= pages()" (click)="go(page() + 1)" [attr.aria-label]="'common.next' | t">›</button>
          </span>
        }
      </nav>
    }
  `,
})
export class Pager {
  readonly page = input(1);
  readonly total = input(0);
  readonly perPage = input(20);
  readonly pageChange = output<number>();

  readonly pages = computed(() => Math.max(1, Math.ceil(this.total() / this.perPage())));
  readonly from = computed(() => Math.min(this.total(), (this.page() - 1) * this.perPage() + 1));
  readonly to = computed(() => Math.min(this.total(), this.page() * this.perPage()));
  /** Page numbers to show; 0 marks a gap. */
  readonly numbers = computed(() => {
    const n = this.pages(), c = this.page();
    const want = new Set([1, n, c - 1, c, c + 1].filter(p => p >= 1 && p <= n));
    if (c <= 3) [2, 3, 4].forEach(p => p <= n && want.add(p));
    if (c >= n - 2) [n - 3, n - 2, n - 1].forEach(p => p >= 1 && want.add(p));
    const sorted = [...want].sort((a, b) => a - b);
    const out: number[] = [];
    sorted.forEach((p, i) => { if (i && p - sorted[i - 1] > 1) out.push(0); out.push(p); });
    return out;
  });

  go(p: number) {
    if (p >= 1 && p <= this.pages() && p !== this.page()) this.pageChange.emit(p);
  }
}

/** Client-side slice for lists that are loaded in full. */
export function pageSlice<T>(items: T[], page: number, perPage: number): T[] {
  return items.slice((page - 1) * perPage, page * perPage);
}
