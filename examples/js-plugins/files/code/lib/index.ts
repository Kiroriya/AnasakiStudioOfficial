// Library of the "files" plugin: `import { toCSV, csvDownload } from 'files'` bundles these files into
// the importer (no scopes needed).
import { toCSV, CsvOptions } from './csv';
import { toXLSX, XlsxOptions } from './xlsx';
import { toPDF, PdfSpec } from './pdf';
import { ColumnSpec } from './columns';
import { base64 } from './bytes';

export { toCSV } from './csv';
export type { CsvOptions } from './csv';
export { toXLSX, xlsxParts, colName } from './xlsx';
export type { XlsxOptions } from './xlsx';
export { toPDF } from './pdf';
export type { PdfSpec, PdfTable } from './pdf';
export { parseTTF } from './ttf';
export type { TrueTypeFont } from './ttf';
export { zip } from './zip';
export { utf8, base64, crc32, ascii85 } from './bytes';
export type { Column, ColumnSpec } from './columns';

export const XLSX_TYPE = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';

/** Content-Disposition for a download: ASCII fallback name + RFC 5987 UTF-8 name (Thai names ok). */
export function contentDisposition(filename: string): string {
  const ascii = filename.replace(/[^\x20-\x7e]/g, '_').replace(/["\\]/g, '_');
  const encoded = encodeURIComponent(filename).replace(/['()*]/g, (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase());
  return `attachment; filename="${ascii}"; filename*=UTF-8''${encoded}`;
}

/**
 * A route result (also usable as a site.adminPage download result) that downloads a file: text
 * (CSV, PDF, JSON…) or binary bytes (Uint8Array / ArrayBuffer, e.g. toXLSX) written verbatim.
 * ผลลัพธ์ route / ดาวน์โหลดจากหน้าแอดมิน สำหรับไฟล์ข้อความหรือไบนารี
 */
export function download(filename: string, content: string | Uint8Array | ArrayBuffer, contentType = 'application/octet-stream') {
  if (typeof content !== 'string' && !(content instanceof Uint8Array) && !(content instanceof ArrayBuffer)) {
    throw new Error('files.download: content must be a string, Uint8Array or ArrayBuffer');
  }
  return {
    status: 200,
    headers: {
      'Content-Type': contentType,
      'Content-Disposition': contentDisposition(filename),
      'Cache-Control': 'no-store',
      'X-Content-Type-Options': 'nosniff',
    },
    body: content,
  };
}

/** download() of toCSV(rows, columns). */
export function csvDownload(filename: string, rows: any[], columns?: ColumnSpec[], opts?: CsvOptions) {
  return download(filename, toCSV(rows, columns, opts), 'text/csv; charset=utf-8');
}

/** download() of toPDF(spec). */
export function pdfDownload(filename: string, spec: PdfSpec) {
  return download(filename, toPDF(spec), 'application/pdf');
}

/** download() of toXLSX(rows, columns) — the .xlsx bytes sent directly (binary route body). */
export function xlsxDownload(filename: string, rows: any[], columns?: ColumnSpec[], opts?: XlsxOptions) {
  return download(filename, toXLSX(rows, columns, opts), XLSX_TYPE);
}

/**
 * The workbook as JSON `{ filename, contentType, base64 }` (kept for compatibility, and still handy
 * from a site.action whose result is JSON): turn it into a file in the browser with atob + Blob.
 * Routes and admin-page downloads can send the bytes directly with xlsxDownload().
 */
export function xlsxPayload(filename: string, rows: any[], columns?: ColumnSpec[], opts?: XlsxOptions) {
  return { filename, contentType: XLSX_TYPE, base64: base64(toXLSX(rows, columns, opts)) };
}

/**
 * A Thai PDF (or any text Sarabun covers): toPDF(spec) rendered by the **files** unit with the Sarabun
 * font shipped in this package (fonts/Sarabun-Regular.woff, SIL OFL 1.1) — through
 * plugins.call('files', 'renderThaiPdf', spec). The spec is copied as JSON: column `format` functions
 * are dropped, so format the rows first. Throws when the files plugin is not active.
 * สร้าง PDF ภาษาไทยด้วยฟอนต์ Sarabun ที่มากับปลั๊กอิน
 */
export function thaiPdf(spec: PdfSpec): string {
  return plugins.call('files', 'renderThaiPdf', spec) as string;
}

/** download() of thaiPdf(spec). */
export function thaiPdfDownload(filename: string, spec: PdfSpec) {
  return download(filename, thaiPdf(spec), 'application/pdf');
}
