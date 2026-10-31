import { ChangeDetectionStrategy, Component, OnInit, inject, signal } from '@angular/core';
import { Dialog } from '../core/dialog';
import { DecimalPipe } from '@angular/common';
import { Api, SITE_ORIGIN, errorMessage } from '../core/api';
import { I18n, TPipe } from '../core/i18n';
import { Media } from '../core/models';
import { fileIcon } from '../shared/media-picker';
import './messages-platform';
import './messages-admin';

import { Pager } from '../shared/pager';
import { uploadFiles } from '../shared/upload';
import { UrlImport } from '../shared/url-import';

@Component({
  selector: 'rea-media-page',
  imports: [DecimalPipe, TPipe, Pager, UrlImport],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <header class="page-head">
      <span class="page-icon">🖼</span>
      <div class="page-head-text"><h1>{{ 'nav.media' | t }}</h1><p>{{ 'media.desc' | t }}</p></div>
      <label class="btn btn-primary">
        {{ (uploading() ? 'media.uploading' : 'media.upload') | t }}
        <input type="file" multiple hidden (change)="upload($event)" />
      </label>
    </header>
    @if (error()) { <p class="alert">{{ error() }}</p> }

    <div class="dropzone" [class.over]="over()" (dragover)="$event.preventDefault(); over.set(true)" (dragleave)="over.set(false)" (drop)="drop($event)">
      <span class="dropzone-icon">📂</span>
      <b>{{ 'media.drop' | t }}</b>
      <span class="muted small">{{ 'media.dropHint' | t }}</span>
    </div>
    <rea-url-import (imported)="load(); sel.set($event)" />

    <div class="tabs-row">
      @for (k of kinds; track k.id) {
        <button class="tab" [class.on]="kind() === k.id" (click)="setKind(k.id)">{{ k.icon }} {{ 'media.kind.' + (k.id || 'all') | t }}</button>
      }
      <span class="grow"></span>
      <input class="search media-search" [placeholder]="'common.searchPh' | t" [value]="search()" (input)="search.set(val($event))" (keyup.enter)="page.set(1); load()" />
    </div>

    <div class="media-layout" [class.with-detail]="!!sel()">
      <section class="card">
        <div class="media-grid">
          @for (m of items(); track m.id) {
            <button class="media-tile" [class.active]="sel()?.id === m.id" (click)="sel.set(m)">
              @if (m.mime.startsWith('image/')) { <img [src]="m.url" [alt]="m.alt" loading="lazy" /> }
              @else { <span class="file-ext"><i>{{ icon(m) }}</i>{{ m.ext || m.mime.split('/')[1] }}</span><span class="media-name">{{ m.filename }}</span> }
            </button>
          } @empty { <p class="muted pad">{{ 'media.empty' | t }}</p> }
        </div>
        <rea-pager [page]="page()" [total]="total()" [perPage]="perPage" (pageChange)="go($event)" />
      </section>
      @if (sel(); as m) {
        <aside class="card media-detail">
          @if (m.mime.startsWith('image/')) { <img [src]="m.url" alt="" /> }
          <p class="media-file"><b>{{ m.filename }}</b></p>
          <p class="muted small">{{ m.mime }} · {{ m.size / 1024 | number: '1.0-0' }} KB @if (m.width) { · {{ m.width }}×{{ m.height }} }</p>
          <p class="muted small">{{ i18n.date(m.createdAt, true) }}</p>
          @if (!m.mime.startsWith('image/')) {
            <p class="media-dl"><span>⬇ {{ 'media.downloads' | t: { n: m.downloads ?? 0 } }}</span></p>
            <label>{{ 'media.dlLink' | t }} <span class="muted small">{{ 'media.dlLinkHint' | t }}</span><input [value]="dlUrl(m)" readonly (focus)="$any($event.target).select()" /></label>
            <a class="btn btn-sm" [href]="dlUrl(m)" target="_blank">⬇ {{ 'common.download' | t }}</a>
          }
          @if (m.mime.startsWith('image/')) { <label>{{ 'media.alt' | t }} <span class="muted small">{{ 'media.altHint' | t }}</span><input [value]="m.alt" (change)="saveAlt(m, $event)" /></label> }
          <label>{{ 'media.url' | t }} <input [value]="m.url" readonly (focus)="$any($event.target).select()" /></label>
          <div class="row-actions">
            <button class="btn btn-sm" (click)="sel.set(null)">{{ 'common.close' | t }}</button>
            <button class="btn btn-sm btn-danger" (click)="remove(m)">🗑 {{ 'common.delete' | t }}</button>
          </div>
        </aside>
      }
    </div>
  `,
})
export class MediaPage implements OnInit {
  readonly i18n = inject(I18n);
  private api = inject(Api);
  readonly items = signal<Media[]>([]);
  readonly sel = signal<Media | null>(null);
  readonly uploading = signal(false);
  readonly over = signal(false);
  readonly error = signal('');
  readonly kind = signal('');
  readonly perPage = 40;
  readonly page = signal(1);
  readonly total = signal(0);
  readonly search = signal('');
  readonly kinds = [
    { id: '', icon: '🗂' }, { id: 'image', icon: '🖼' }, { id: 'document', icon: '📄' }, { id: 'video', icon: '🎬' },
    { id: 'audio', icon: '🎵' }, { id: 'archive', icon: '🗜' }, { id: 'other', icon: '📎' },
  ];

  ngOnInit() { this.load(); }

  async load() {
    try {
      const r = await this.api.mediaPage({ kind: this.kind(), search: this.search(), page: this.page(), perPage: this.perPage });
      this.items.set(r.items);
      this.total.set(r.total);
    } catch (e) { this.error.set(errorMessage(e)); }
  }

  val(e: Event) { return (e.target as HTMLInputElement).value; }
  icon(m: Media) { return fileIcon(m); }
  setKind(k: string) { this.kind.set(k); this.page.set(1); this.load(); }
  go(p: number) { this.page.set(p); this.load(); window.scrollTo({ top: 0 }); }
  /** Public download link (counts downloads, sends the original file name). */
  dlUrl(m: Media) { return `${SITE_ORIGIN || location.origin}/download/${m.id}`; }

  upload(e: Event) { this.send(Array.from((e.target as HTMLInputElement).files ?? [])); }

  drop(e: DragEvent) {
    e.preventDefault();
    this.over.set(false);
    this.send(Array.from(e.dataTransfer?.files ?? []));
  }

  private async send(files: File[]) {
    if (!files.length) return;
    this.uploading.set(true);
    this.error.set('');
    try {
      const { error } = await uploadFiles(this.api, files);
      await this.load();
      if (error) this.error.set(error);
    } catch (e) {
      this.error.set(errorMessage(e));
    } finally {
      this.uploading.set(false);
    }
  }

  async saveAlt(m: Media, e: Event) {
    try {
      const u = await this.api.updateMedia(m.id, (e.target as HTMLInputElement).value);
      this.items.update(xs => xs.map(x => (x.id === u.id ? u : x)));
      this.sel.set(u);
    } catch (err) { this.error.set(errorMessage(err)); }
  }

  async remove(m: Media) {
    if (!(await Dialog.confirmDelete(m.filename, this.i18n.t('media.deleteText')))) return;
    try {
      await this.api.deleteMedia(m.id);
      this.sel.set(null);
      await this.load();
    } catch (e) { this.error.set(errorMessage(e)); }
  }
}
