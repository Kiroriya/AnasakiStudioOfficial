import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import { TPipe } from '../core/i18n';
import { DNode } from './design-types';
import { EditorStore } from './editor-store';
import './messages-mask';

/** Design tab: clip a layer to a shape (mask) and pick the focal point of a cropped image. */
@Component({
  selector: 'rea-mask-panel',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @let n = node();
    <section class="fg-sec">
      <h5>✂ {{ 'mk.title' | t }}</h5>
      <div class="mk-grid">
        <button [class.on]="!n.style?.mask" (click)="set({ mask: undefined })" [title]="'mk.none' | t">∅</button>
        @for (m of masks(); track m.name) {
          <button [class.on]="n.style?.mask === m.name" (click)="set({ mask: m.name })" [title]="m.name" [innerHTML]="m.svg"></button>
        }
      </div>
      <p class="hint">{{ 'mk.hint' | t }}</p>

      @if (image(); as src) {
        <h6 class="mk-h">{{ 'mk.focal' | t }}</h6>
        <div class="mk-focal" (pointerdown)="pick($event)">
          <img [src]="src" alt="" draggable="false" />
          <i class="mk-dot" [style.left.%]="n.style?.focalX ?? 50" [style.top.%]="n.style?.focalY ?? 50"></i>
        </div>
        <div class="mk-row">
          <small>{{ round(n.style?.focalX ?? 50) }}% · {{ round(n.style?.focalY ?? 50) }}%</small>
          @if (n.style?.focalX !== undefined || n.style?.focalY !== undefined) { <button class="fg-link" (click)="set({ focalX: undefined, focalY: undefined })">{{ 'pt.reset' | t }}</button> }
        </div>
        <p class="hint">{{ 'mk.focalHint' | t }}</p>
      }
    </section>
  `,
  styles: [`
    .mk-grid { display: grid; grid-template-columns: repeat(6, 1fr); gap: 4px; }
    .mk-grid button { aspect-ratio: 1; border: 1px solid var(--fg-border, #e5e5e5); background: #fff; border-radius: 6px; padding: 5px; cursor: pointer; color: #7c3aed; display: grid; place-items: center; font-size: 14px; }
    .mk-grid button.on { border-color: #7c3aed; background: #f5f3ff; }
    .mk-grid button ::ng-deep svg { width: 100%; height: 100%; display: block; }
    .mk-h { margin: 10px 0 6px; font-size: 11px; color: var(--fg-muted, #888); }
    .mk-focal { position: relative; border-radius: 6px; overflow: hidden; cursor: crosshair; background: #f3f4f6; touch-action: none; }
    .mk-focal img { width: 100%; display: block; max-height: 160px; object-fit: contain; user-select: none; }
    .mk-dot { position: absolute; width: 16px; height: 16px; margin: -8px 0 0 -8px; border-radius: 999px; border: 2px solid #fff; background: #7c3aed; box-shadow: 0 0 0 1px rgba(0,0,0,.3); pointer-events: none; }
    .mk-row { display: flex; justify-content: space-between; align-items: center; margin-top: 4px; font-size: 11px; color: var(--fg-muted, #888); }
  `],
})
export class MaskPanel {
  readonly store = inject(EditorStore);
  private sanitizer = inject(DomSanitizer);
  readonly node = input.required<DNode>();
  readonly round = Math.round;

  readonly masks = computed(() => {
    const a = this.store.assets();
    const out: { name: string; svg: SafeHtml }[] = [];
    for (const name of a?.maskNames ?? []) {
      const m = a!.masks![name];
      out.push({ name, svg: this.sanitizer.bypassSecurityTrustHtml(`<svg viewBox="${m.viewBox}" preserveAspectRatio="none" fill="currentColor">${m.body}</svg>`) });
    }
    return out;
  });

  /** The picture to pick a focal point on (image src or image fill), when it is a plain URL. */
  readonly image = computed(() => {
    const n = this.node();
    const src = n.type === 'image' ? n.props?.src : n.style?.backgroundImage;
    return src && !src.includes('{{') ? src : '';
  });

  set(p: Partial<NonNullable<DNode['style']>>) { this.store.setStyle(this.node().id, p); }

  pick(e: PointerEvent) {
    const box = (e.currentTarget as HTMLElement).querySelector('img')!.getBoundingClientRect();
    const at = (ev: PointerEvent) => ({
      focalX: Math.round(Math.max(0, Math.min(100, ((ev.clientX - box.left) / box.width) * 100))),
      focalY: Math.round(Math.max(0, Math.min(100, ((ev.clientY - box.top) / box.height) * 100))),
    });
    this.store.beginGesture();
    this.set(at(e));
    const move = (ev: PointerEvent) => this.set(at(ev));
    const up = () => {
      window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); window.removeEventListener('pointercancel', up);
      this.store.endGesture();
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  }
}
