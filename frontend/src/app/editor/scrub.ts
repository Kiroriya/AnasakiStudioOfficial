import { Directive, inject, input, output } from '@angular/core';
import { EditorStore } from './editor-store';

/**
 * Figma-style scrubbing: drag horizontally on a field's label/icon to change
 * its number (Shift = ×10). The whole drag is one undo step.
 */
@Directive({
  selector: '[reaScrub]',
  host: { class: 'scrub', '(pointerdown)': 'down($event)' },
})
export class Scrub {
  private store = inject(EditorStore);
  readonly reaScrub = input<number | null | undefined>(0);
  readonly step = input(1);
  readonly minValue = input<number | null>(null);
  readonly scrub = output<number>();

  down(e: PointerEvent) {
    if (e.button !== 0) return;
    e.preventDefault();
    const start = Number(this.reaScrub() ?? 0);
    const sx = e.clientX;
    let moved = false;
    const move = (ev: PointerEvent) => {
      const d = Math.round((ev.clientX - sx) / 2);
      if (!moved) {
        if (Math.abs(d) < 1) return;
        moved = true;
        this.store.beginGesture();
        document.body.classList.add('rea-scrubbing');
      }
      let v = start + d * this.step() * (ev.shiftKey ? 10 : 1);
      const min = this.minValue();
      if (min !== null) v = Math.max(min, v);
      this.scrub.emit(Math.round(v * 100) / 100);
    };
    const up = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
      document.body.classList.remove('rea-scrubbing');
      if (moved) this.store.endGesture();
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  }
}
