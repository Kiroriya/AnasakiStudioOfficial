// toXLSX: a minimal Office Open XML workbook (one sheet, inline strings, bold frozen header row).
import { ColumnSpec, normColumns, cellValue } from './columns';
import { zip } from './zip';

export interface XlsxOptions {
  /** Sheet name (max 31 characters; []:*?/\ removed). Default "Sheet1". */
  sheetName?: string;
  /** Write the header row (default true). */
  header?: boolean;
}

const esc = (s: string) =>
  s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    // characters XML 1.0 does not allow
    .replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F￾￿]/g, '');

/** 0 → A, 25 → Z, 26 → AA. */
export function colName(i: number): string {
  let s = '';
  for (let n = i + 1; n > 0; n = Math.floor((n - 1) / 26)) s = String.fromCharCode(65 + ((n - 1) % 26)) + s;
  return s;
}

function cell(ref: string, v: any, style = 0): string {
  const s = style ? ` s="${style}"` : '';
  if (v === null || v === undefined || v === '') return '';
  if (typeof v === 'number' && Number.isFinite(v)) return `<c r="${ref}"${s}><v>${v}</v></c>`;
  if (typeof v === 'boolean') return `<c r="${ref}"${s} t="b"><v>${v ? 1 : 0}</v></c>`;
  const str = v instanceof Date ? v.toISOString() : typeof v === 'object' ? JSON.stringify(v) : String(v);
  return `<c r="${ref}"${s} t="inlineStr"><is><t xml:space="preserve">${esc(str)}</t></is></c>`;
}

const DECL = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n';
const NS_MAIN = 'http://schemas.openxmlformats.org/spreadsheetml/2006/main';
const NS_PKG_REL = 'http://schemas.openxmlformats.org/package/2006/relationships';
const NS_DOC_REL = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships';
const CT = 'application/vnd.openxmlformats-officedocument.spreadsheetml';

/** The parts of the workbook (path → XML) before zipping. */
export function xlsxParts(rows: any[], columns?: ColumnSpec[], opts: XlsxOptions = {}): Record<string, string> {
  const cols = normColumns(rows, columns);
  const sheet = (opts.sheetName ?? 'Sheet1').replace(/[\[\]:*?\/\\]/g, '').slice(0, 31) || 'Sheet1';
  const header = opts.header !== false;
  const xmlRows: string[] = [];
  let r = 1;
  if (header) {
    xmlRows.push(`<row r="1">${cols.map((c, i) => cell(colName(i) + 1, c.label ?? c.key, 1)).join('')}</row>`);
    r = 2;
  }
  for (const row of rows) {
    const n = r;
    xmlRows.push(`<row r="${n}">${cols.map((c, i) => cell(colName(i) + n, cellValue(row, c))).join('')}</row>`);
    r++;
  }
  const widths = cols.map((c, i) => `<col min="${i + 1}" max="${i + 1}" width="${Number(c.width ?? 15)}" customWidth="1"/>`).join('');
  const pane = header
    ? '<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>'
    : '';
  return {
    '[Content_Types].xml': DECL +
      '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">' +
      '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>' +
      '<Default Extension="xml" ContentType="application/xml"/>' +
      `<Override PartName="/xl/workbook.xml" ContentType="${CT}.sheet.main+xml"/>` +
      `<Override PartName="/xl/worksheets/sheet1.xml" ContentType="${CT}.worksheet+xml"/>` +
      `<Override PartName="/xl/styles.xml" ContentType="${CT}.styles+xml"/>` +
      '</Types>',
    '_rels/.rels': DECL +
      `<Relationships xmlns="${NS_PKG_REL}">` +
      `<Relationship Id="rId1" Type="${NS_DOC_REL}/officeDocument" Target="xl/workbook.xml"/>` +
      '</Relationships>',
    'xl/workbook.xml': DECL +
      `<workbook xmlns="${NS_MAIN}" xmlns:r="${NS_DOC_REL}">` +
      `<sheets><sheet name="${esc(sheet)}" sheetId="1" r:id="rId1"/></sheets></workbook>`,
    'xl/_rels/workbook.xml.rels': DECL +
      `<Relationships xmlns="${NS_PKG_REL}">` +
      `<Relationship Id="rId1" Type="${NS_DOC_REL}/worksheet" Target="worksheets/sheet1.xml"/>` +
      `<Relationship Id="rId2" Type="${NS_DOC_REL}/styles" Target="styles.xml"/>` +
      '</Relationships>',
    'xl/styles.xml': DECL +
      `<styleSheet xmlns="${NS_MAIN}">` +
      '<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>' +
      '<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>' +
      '<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>' +
      '<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>' +
      '<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>' +
      '<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>' +
      '<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>' +
      '</styleSheet>',
    'xl/worksheets/sheet1.xml': DECL +
      `<worksheet xmlns="${NS_MAIN}">` + pane +
      (widths ? `<cols>${widths}</cols>` : '') + `<sheetData>${xmlRows.join('')}</sheetData></worksheet>`,
  };
}

/** The workbook as .xlsx bytes (a zip). / แปลงเป็นไฟล์ Excel */
export function toXLSX(rows: any[], columns?: ColumnSpec[], opts: XlsxOptions = {}): Uint8Array {
  const parts = xlsxParts(rows, columns, opts);
  return zip(Object.keys(parts).map((name) => ({ name, data: parts[name] })));
}
