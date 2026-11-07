// maps 1.0 — a map block for the designer: an address or a "lat, lng" set on the block, read from a field of the
// page's post (a content type's "Address" or "Location" field, so every listing or event shows its own map) or
// the business address from the settings. Google Maps without an API key, or OpenStreetMap for coordinates;
// a "Get directions" link under it. The iframe hosts are the core's trusted embed hosts.

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);

const COORDS = /^\s*(-?\d{1,2}(?:\.\d+)?)\s*,\s*(-?\d{1,3}(?:\.\d+)?)\s*$/;

/** A field of the post (meta may come as JSON text or an object) or of a collection item. */
function fieldOf(ctx: { post: Record<string, any> | null; item: Record<string, any> | null }, key: string): string {
  if (!key) return '';
  if (ctx.item && ctx.item[key] !== undefined) return String(ctx.item[key] ?? '');
  let meta: any = ctx.post?.meta;
  if (typeof meta === 'string') {
    try {
      meta = JSON.parse(meta);
    } catch {
      meta = {};
    }
  }
  const v = meta && typeof meta === 'object' ? meta[key] : undefined;
  return typeof v === 'string' || typeof v === 'number' ? String(v) : '';
}

/** The iframe URL and the directions link of a place ('' when there is none). */
export function mapURLs(place: string, zoom: number, provider: string): { embed: string; directions: string } {
  const p = place.trim().slice(0, 300);
  if (!p) return { embed: '', directions: '' };
  const z = Math.min(20, Math.max(3, Math.round(zoom) || 15));
  const c = COORDS.exec(p);
  const q = encodeURIComponent(c ? `${c[1]},${c[2]}` : p);
  const directions = 'https://www.google.com/maps/dir/?api=1&destination=' + q;
  if (c && provider === 'osm') {
    const lat = Number(c[1]), lng = Number(c[2]);
    const d = 360 / Math.pow(2, z) * 1.2; // about the visible width at that zoom
    const bbox = [lng - d, lat - d / 2, lng + d, lat + d / 2].map((n) => n.toFixed(5)).join(',');
    return { embed: `https://www.openstreetmap.org/export/embed.html?bbox=${bbox}&layer=mapnik&marker=${lat},${lng}`, directions };
  }
  return { embed: `https://www.google.com/maps?q=${q}&z=${z}&output=embed`, directions };
}

const CSS = '.amap{display:flex;flex-direction:column;gap:8px;width:100%}.amap iframe{width:100%;border:0;border-radius:12px;background:#f3f4f6}' +
  '.amap-dir{align-self:flex-start;color:inherit;font-weight:600}.amap-hint{color:#6b7280;font-size:13px;padding:8px;border:1px dashed #d1d5db;border-radius:8px}';

site.block('map', {
  label: { th: 'แผนที่', en: 'Map' },
  icon: '🗺️',
  props: [
    { key: 'address', type: 'text', maxLength: 300, label: { th: 'ที่อยู่ หรือ ละติจูด, ลองจิจูด', en: 'Address or latitude, longitude' },
      placeholder: '13.7466, 100.5393',
      help: { th: 'เว้นว่าง = ใช้ช่องข้อมูลด้านล่าง หรือที่อยู่ร้านในการตั้งค่าปลั๊กอิน', en: 'Empty = the field below, or the business address in the plugin settings' } },
    { key: 'field', type: 'text', maxLength: 40, label: { th: 'อ่านจากช่องข้อมูลของเนื้อหา (คีย์)', en: 'Read from the post’s field (key)' },
      placeholder: 'address',
      help: { th: 'ในแม่แบบของประเภทเนื้อหา เช่น ช่อง address ของประกาศขายบ้าน แต่ละหน้าจะแสดงแผนที่ของตัวเอง', en: 'In a content type’s template, e.g. the address field of a listing: each page shows its own map' } },
    { key: 'zoom', type: 'number', min: 3, max: 20, default: 15, label: { th: 'ซูม', en: 'Zoom' } },
    { key: 'height', type: 'number', min: 160, max: 900, default: 360, label: { th: 'ความสูง (px)', en: 'Height (px)' } },
    { key: 'provider', type: 'select', default: 'google', label: { th: 'ผู้ให้บริการแผนที่', en: 'Map provider' },
      options: [{ value: 'google', label: 'Google Maps' }, { value: 'osm', label: { th: 'OpenStreetMap (ต้องใส่พิกัด)', en: 'OpenStreetMap (coordinates)' } }] },
    { key: 'directions', type: 'boolean', default: true, label: { th: 'ปุ่มนำทาง', en: '“Get directions” link' } },
  ],
  render: (ctx, props) => {
    const place = String(props.address ?? '').trim() || fieldOf(ctx as any, String(props.field ?? '').trim()) || String(site.settings('address') ?? '').trim();
    const { embed, directions } = mapURLs(place, Number(props.zoom ?? 15), String(props.provider ?? 'google'));
    if (!embed) {
      return { css: CSS, html: (ctx as any).preview === false ? '' : '<p class="amap-hint">ใส่ที่อยู่หรือพิกัดในแผงคุณสมบัติ / Enter an address or coordinates</p>' };
    }
    const h = Math.min(900, Math.max(160, Math.round(Number(props.height)) || 360));
    const label = String(site.settings('label') ?? '').trim() || place;
    return {
      css: CSS,
      html: `<div class="amap"><iframe src="${esc(embed)}" height="${h}" loading="lazy" referrerpolicy="no-referrer-when-downgrade" allowfullscreen title="${esc(label)}"></iframe>` +
        (props.directions === false ? '' : `<a class="amap-dir" href="${esc(directions)}" target="_blank" rel="noopener">📍 นำทาง / Get directions</a>`) + '</div>',
    };
  },
});
