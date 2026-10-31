import { Directive, ElementRef, OnDestroy, effect, inject, input } from '@angular/core';

/**
 * `<select [value]="x">` that keeps showing x even when its options render later
 * (e.g. from an @for over data that loads asynchronously). Angular's plain DOM
 * `value` binding runs before the options exist, so the browser falls back to the
 * first option; this re-applies the value whenever the option list changes.
 */
@Directive({ selector: 'select[value]' })
export class SelectValue implements OnDestroy {
  private el = inject<ElementRef<HTMLSelectElement>>(ElementRef).nativeElement;
  readonly value = input<unknown>();
  private observer = new MutationObserver(() => this.apply());

  constructor() {
    this.observer.observe(this.el, { childList: true, subtree: true });
    effect(() => this.apply());
  }

  private apply() {
    const v = this.value();
    const s = v === null || v === undefined ? '' : String(v);
    if (this.el.value !== s) this.el.value = s;
  }

  ngOnDestroy() { this.observer.disconnect(); }
}
