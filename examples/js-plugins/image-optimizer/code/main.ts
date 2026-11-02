// Image optimizer (example JS plugin, CMS 2.1.0) — proves the media joints:
//  - filter media.beforeSave: refuse images over maxUploadMB before they are stored
//  - event media.uploaded (fired for admin uploads, URL imports and media.upload alike):
//    read the file, scale it down to maxWidth, and replace it when the result is smaller
const num = (k: string, def: number) => {
  const n = Number(site.settings(k));
  return Number.isFinite(n) && n > 0 ? n : def;
};

site.filter('media.beforeSave', (file, ctx) => {
  const limit = Number(site.settings('maxUploadMB')) || 0;
  if (limit > 0 && ctx.kind === 'image' && ctx.size > limit * 1024 * 1024) {
    return { ...file, error: `รูปใหญ่เกิน ${limit} MB / image is over ${limit} MB` };
  }
  return file;
});

site.on('media.uploaded', (m) => {
  if (m.mime !== 'image/jpeg' && m.mime !== 'image/png') return;
  const maxWidth = num('maxWidth', 1920);
  // a small image within the width is left alone
  if (m.width > 0 && m.width <= maxWidth && m.size < 300 * 1024) return;
  db.asSystem(() => {
    const r = media.resize(media.read(m.id), { maxWidth, quality: num('quality', 82) });
    if (r.size >= m.size) return;
    media.replace(m.id, r.bytes);
    log.info('optimized', m.filename, `${m.size} → ${r.size} bytes`, `${m.width}px → ${r.width}px`);
  });
});
