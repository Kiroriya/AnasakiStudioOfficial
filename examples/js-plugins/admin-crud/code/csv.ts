// A tiny CSV writer for the "Export CSV" download (the files plugin has a fuller one; this sample stays
// dependency-free): UTF-8 BOM for Excel, RFC 4180 quoting, CRLF, and a guard against formula injection
// (text starting with = + - @ gets a leading apostrophe).

const cell = (v: unknown): string => {
  if (v === null || v === undefined) return '';
  let s = typeof v === 'object' ? JSON.stringify(v) : String(v);
  if (typeof v === 'string' && /^[=+\-@\t\r]/.test(s)) s = "'" + s;
  return /[",\r\n]/.test(s) || /^\s|\s$/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s;
};

export function csvOf(rows: Record<string, any>[], cols: { key: string; label: string }[]): string {
  const lines = [cols.map((c) => cell(c.label)).join(',')];
  for (const r of rows) lines.push(cols.map((c) => cell(r[c.key])).join(','));
  return '\uFEFF' + lines.join('\r\n') + '\r\n';
}

/** Content-Disposition: attachment with an ASCII fallback and the UTF-8 name. */
export function csvDisposition(filename: string): string {
  const ascii = filename.replace(/[^\x20-\x7e]/g, '_').replace(/["\\]/g, '_');
  return `attachment; filename="${ascii}"; filename*=UTF-8''${encodeURIComponent(filename)}`;
}
