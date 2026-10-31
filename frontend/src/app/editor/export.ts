import { getFontEmbedCSS, toBlob } from 'html-to-image';
import { tr } from '../core/i18n';
import { DNode, findNode } from './design-types';
import { EditorStore } from './editor-store';
import { vectorSvg, viewBoxOf } from './vector';
import './messages-vector';

/** Figma Export panel: PNG / JPG at 1x–3x, SVG for vector layers. */
export type ExportFormat = 'png' | 'jpg' | 'svg';
/** Last used settings live in the editor (store.exportSettings); the Ctrl+Shift+E command repeats them. */
export interface ExportSettings { format: ExportFormat; scale: number }

export const svgExportable = (n: DNode) => n.type === 'vector' || n.type === 'icon' || n.type === 'shape';

function canvasEl(id: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`.artboard [data-node-id="${CSS.escape(id)}"]`);
}

export function exportName(n: DNode): string {
  const base = (n.name || n.type).replace(/[\\/:*?"<>|\u0000-\u001f]+/g, '_').trim() || 'layer';
  return base.slice(0, 80);
}

function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 2000);
}

/** Rasterizes a layer as drawn on the canvas (at design size × scale). */
export async function rasterize(s: EditorStore, id: string, format: 'png' | 'jpg', scale: number): Promise<Blob | null> {
  const el = canvasEl(id);
  const r = s.measure?.(id);
  if (!el || !r) return null;
  document.body.classList.add('rea-exporting'); // hide editor-only decorations during capture (editor.scss)
  try {
    const w = Math.max(1, Math.round(r.w)), h = Math.max(1, Math.round(r.h));
    // web fonts go into the image; a font stylesheet the browser won't let us read (CORS) falls back to system fonts
    let fontEmbedCSS: string | undefined;
    try { fontEmbedCSS = await getFontEmbedCSS(el, { preferredFontFormat: 'woff2' }); } catch (e) { console.warn('export: fonts not embedded', e); }
    return await toBlob(el, {
      width: w, height: h,
      pixelRatio: scale, // design px × scale, whatever the canvas zoom
      canvasWidth: w, canvasHeight: h,
      backgroundColor: format === 'jpg' ? '#ffffff' : undefined,
      type: format === 'jpg' ? 'image/jpeg' : 'image/png',
      quality: 0.92,
      ...(fontEmbedCSS !== undefined ? { fontEmbedCSS } : { skipFonts: true }),
      // the layer is drawn at the origin of the image, without its position in the parent
      style: { position: 'static', left: 'auto', top: 'auto', margin: '0' },
      filter: (node: Node) => !(node instanceof HTMLElement && node.classList.contains('ov')),
    });
  } finally {
    document.body.classList.remove('rea-exporting');
  }
}

/** Standalone SVG of a vector / icon / shape layer, colours resolved from the canvas. */
export function svgMarkup(s: EditorStore, n: DNode): string | null {
  const el = canvasEl(n.id);
  const r = s.measure?.(n.id);
  if (!el || !r || !svgExportable(n)) return null;
  const w = Math.round(r.w * 100) / 100, h = Math.round(r.h * 100) / 100;
  // theme colours (var(--c-…)) and currentColor resolved to real colours
  const resolve = (markup: string) => {
    const color = getComputedStyle(el).color || '#000000';
    return markup.replace(/var\(--[A-Za-z0-9_-]+\)/g, v => resolveVar(el, v) || color).replace(/currentColor/g, color);
  };
  let svg: string;
  if (n.type === 'vector') {
    const vb = viewBoxOf(n);
    const path = vectorSvg(n).match(/<path [^>]*\/>/)?.[0] ?? '';
    svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="${vb.x} ${vb.y} ${vb.w} ${vb.h}" preserveAspectRatio="none" overflow="visible">${path}</svg>`;
  } else {
    svg = s.svgFor(n).replace('<svg ', `<svg xmlns="http://www.w3.org/2000/svg" `).replace('width="100%" height="100%"', `width="${w}" height="${h}"`)
      .replace(' style="display:block"', '');
  }
  return '<?xml version="1.0" encoding="UTF-8"?>\n' + resolve(svg);
}

function resolveVar(el: HTMLElement, v: string): string {
  const probe = document.createElement('span');
  probe.style.color = v;
  probe.style.display = 'none';
  el.appendChild(probe);
  const c = getComputedStyle(probe).color;
  probe.remove();
  return c;
}

/** Exports the layer `id` (selected layer by default) and downloads it. */
export async function exportNode(s: EditorStore, id = s.selectedId(), settings: ExportSettings = s.exportSettings()): Promise<boolean> {
  const n = id ? findNode(s.doc(), id)?.node : null;
  if (!n) { s.notify(tr('vec.exportNone'), true); return false; }
  s.exportSettings.set(settings);
  try {
    if (settings.format === 'svg') {
      const svg = svgExportable(n) ? svgMarkup(s, n) : null;
      if (!svg) { s.notify(tr('vec.svgOnly'), true); return false; }
      download(new Blob([svg], { type: 'image/svg+xml' }), `${exportName(n)}.svg`);
      return true;
    }
    const blob = await rasterize(s, n.id, settings.format, settings.scale);
    if (!blob) throw new Error('no image');
    const suffix = settings.scale === 1 ? '' : `@${settings.scale}x`;
    download(blob, `${exportName(n)}${suffix}.${settings.format}`);
    return true;
  } catch (e) {
    console.error(e);
    s.notify(tr('vec.exportFail'), true);
    return false;
  }
}

/** Copies the selected layer to the clipboard as a PNG (1x, or 2x on high-DPI screens). */
export async function copyAsPng(s: EditorStore, id = s.selectedId()) {
  if (!id) { s.notify(tr('vec.exportNone'), true); return; }
  try {
    const blobP = rasterize(s, id, 'png', Math.min(2, Math.max(1, Math.round(devicePixelRatio)))).then(b => {
      if (!b) throw new Error('no image');
      return b;
    });
    // pass the promise so Safari keeps the user gesture
    await navigator.clipboard.write([new ClipboardItem({ 'image/png': blobP })]);
    s.notify(tr('vec.copied'));
  } catch (e) {
    console.error(e);
    s.notify(tr('vec.exportFail'), true);
  }
}
