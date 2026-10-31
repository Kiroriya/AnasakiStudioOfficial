import { ColumnSpec, normColumns, cellValue, text } from './columns';

export interface CsvOptions {
  /** Field separator (default ","; ";" for Excel in comma-decimal locales). */
  separator?: string;
  /** Prefix a byte-order mark so Excel reads UTF-8 (Thai) correctly (default true). */
  bom?: boolean;
  /** Write the header row (default true). */
  header?: boolean;
  /**
   * Neutralise spreadsheet formulas: text cells starting with = + - @ (tab, CR) get a leading "'"
   * (CSV injection). Numbers and numeric strings are never touched. Default true.
   */
  safe?: boolean;
}

/**
 * Rows as CSV text (RFC 4180: CRLF line ends; fields containing the separator, quotes, CR/LF or edge
 * spaces are quoted, quotes doubled), with a UTF-8 BOM first for Excel. / แปลงเป็น CSV
 *   toCSV(rows, [{ key: 'name', label: 'ชื่อ' }, 'total'])
 */
export function toCSV(rows: any[], columns?: ColumnSpec[], opts: CsvOptions = {}): string {
  const sep = opts.separator ?? ',';
  const cols = normColumns(rows, columns);
  const safe = opts.safe !== false;
  const field = (v: any): string => {
    let s = text(v);
    if (safe && typeof v === 'string' && /^[=+\-@\t\r]/.test(s) && !/^[+-]?\d+(\.\d+)?$/.test(s)) s = "'" + s;
    if (s.includes(sep) || /["\r\n]/.test(s) || /^\s|\s$/.test(s)) s = '"' + s.replace(/"/g, '""') + '"';
    return s;
  };
  const lines: string[] = [];
  if (opts.header !== false) lines.push(cols.map((c) => field(c.label ?? c.key)).join(sep));
  for (const r of rows) lines.push(cols.map((c) => field(cellValue(r, c))).join(sep));
  return (opts.bom === false ? '' : '﻿') + lines.join('\r\n') + '\r\n';
}
