// Column definitions shared by toCSV / toXLSX / toPDF.

export interface Column {
  /** Property of each row (dots for nested values: "customer.name"). */
  key: string;
  /** Header text (default: key). */
  label?: string;
  /** Turns the value into what is written (default: the value itself). */
  format?: (value: any, row: any) => any;
  /** XLSX column width in characters / PDF share of the page width (default 15). */
  width?: number;
}

export type ColumnSpec = string | Column;

export function normColumns(rows: any[], columns?: ColumnSpec[]): Column[] {
  if (!columns || columns.length === 0) {
    const keys: string[] = [];
    for (const r of rows.slice(0, 100)) for (const k of Object.keys(r ?? {})) if (!keys.includes(k)) keys.push(k);
    columns = keys;
  }
  return columns.map((c) => (typeof c === 'string' ? { key: c } : c));
}

export function cellValue(row: any, col: Column): any {
  let v: any = row;
  for (const part of col.key.split('.')) v = v == null ? undefined : v[part];
  return col.format ? col.format(v, row) : v;
}

/** A value as text: null/undefined → "", Date → ISO, objects → JSON. */
export function text(v: any): string {
  if (v === null || v === undefined) return '';
  if (v instanceof Date) return v.toISOString();
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}
