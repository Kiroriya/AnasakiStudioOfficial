import { ChangeDetectionStrategy, Component, inject, output, signal } from '@angular/core';
import { Api, errorMessage } from '../core/api';
import { TPipe } from '../core/i18n';
import { Media } from '../core/models';
import '../admin/messages-admin';

/** "Paste an image link" field: the server downloads the file into the media library. */
@Component({
  selector: 'rea-url-import',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <form class="url-import" (submit)="$event.preventDefault(); run()">
      <span class="url-import-icon">🔗</span>
      <input type="url" [placeholder]="'media.urlPh' | t" [value]="url()" (input)="url.set(val($event))" [disabled]="busy()" />
      <button class="btn btn-sm" [disabled]="busy() || !url().trim()">{{ (busy() ? 'media.importing' : 'media.import') | t }}</button>
    </form>
    @if (error()) { <p class="alert small">{{ error() }}</p> }
  `,
})
export class UrlImport {
  private api = inject(Api);
  readonly imported = output<Media>();
  readonly url = signal('');
  readonly busy = signal(false);
  readonly error = signal('');

  val(e: Event) { return (e.target as HTMLInputElement).value; }

  async run() {
    const u = this.url().trim();
    if (!u) return;
    this.busy.set(true);
    this.error.set('');
    try {
      const m = await this.api.importMedia(u);
      this.url.set('');
      this.imported.emit(m);
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.busy.set(false);
    }
  }
}
