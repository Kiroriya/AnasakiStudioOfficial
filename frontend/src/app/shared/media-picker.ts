import { ChangeDetectionStrategy, Component, inject, input, model, output, signal, effect } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { TPipe } from '../core/i18n';
import { Media } from '../core/models';
import { uploadFiles } from './upload';
import { UrlImport } from './url-import';
import '../admin/messages-admin';

/** Modal media library (WordPress "Add Media" dialog). */
@Component({
  selector: 'rea-media-picker',
  imports: [TPipe, UrlImport],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (open()) {
      <div class="modal-backdrop" (click)="open.set(false)">
        <div class="modal modal-lg" (click)="$event.stopPropagation()">
          <header class="modal-head">
            <h3>{{ kind() === 'image' ? '🖼' : '📁' }} {{ (kind() === 'image' ? 'media.pick' : 'media.pickFile') | t }}</h3>
            <label class="btn btn-primary btn-sm">
              {{ (uploading() ? 'media.uploading' : 'media.upload') | t }}
              <input type="file" [accept]="kind() === 'image' ? 'image/*' : ''" multiple hidden (change)="upload($event)" />
            </label>
            <button class="btn btn-ghost btn-sm" (click)="open.set(false)">✕ {{ 'common.close' | t }}</button>
          </header>
          @if (error()) { <p class="alert">{{ error() }}</p> }
          <rea-url-import (imported)="pick($event)" />
          <p class="hint">{{ 'media.pickHint' | t }}</p><div class="media-grid">
            @for (m of items(); track m.id) {
              <button class="media-tile" (click)="pick(m)" [title]="m.filename">
                @if (m.mime.startsWith('image/')) { <img [src]="m.url" [alt]="m.alt" loading="lazy" /> }
                @else { <span class="file-ext"><i>{{ fileIcon(m) }}</i>{{ m.ext || m.mime.split('/')[1] }}</span> }
                <span class="media-name">{{ m.filename }}</span>
              </button>
            } @empty {
              <p class="muted pad">{{ 'media.empty' | t }}</p>
            }
          </div>
        </div>
      </div>
    }
  `,
})
export class MediaPicker {
  private api = inject(Api);
  readonly open = model(false);
  /** 'image' shows only images; '' shows every file type. */
  readonly kind = input<'image' | ''>('image');
  readonly picked = output<Media>();
  readonly items = signal<Media[]>([]);
  readonly uploading = signal(false);
  readonly error = signal('');

  constructor() {
    effect(() => {
      if (this.open()) this.refresh();
    });
  }

  async refresh() {
    try { this.items.set(await this.api.media({ kind: this.kind() })); } catch (e) { this.error.set(errorMessage(e)); }
  }

  async upload(e: Event) {
    const files = Array.from((e.target as HTMLInputElement).files ?? []);
    this.uploading.set(true);
    this.error.set('');
    try {
      const { error } = await uploadFiles(this.api, files);
      await this.refresh();
      if (error) this.error.set(error);
    } catch (err) {
      this.error.set(errorMessage(err));
    } finally {
      this.uploading.set(false);
    }
  }

  fileIcon(m: Media) { return fileIcon(m); }

  pick(m: Media) {
    this.picked.emit(m);
    this.open.set(false);
  }
}

/** Emoji icon for a media item's kind (used for non-image files). */
export function fileIcon(m: Media): string {
  const k = m.kind ?? (m.mime.startsWith('image/') ? 'image' : m.mime.startsWith('video/') ? 'video' : m.mime.startsWith('audio/') ? 'audio' : 'other');
  return ({ image: '🖼', video: '🎬', audio: '🎵', document: '📄', archive: '🗜', other: '📎' } as Record<string, string>)[k] ?? '📎';
}
