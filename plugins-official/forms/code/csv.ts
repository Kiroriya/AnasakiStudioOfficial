// CSV export of a form's submissions — byte for byte what the Go plugin wrote (encoding/csv, "\n" line ends,
// a UTF-8 BOM so Excel reads Thai, spreadsheet formulas neutralised).
import { Form, listSubmissions } from './model';

/** Neutralises spreadsheet formulas (=, +, -, @, tab, CR) in exported cells. */
export function csvSafe(s: string): string {
  return s !== '' && '=+-@\t\r'.includes(s[0]) ? "'" + s : s;
}

/** Go's csv.Writer.fieldNeedsQuotes. */
function needsQuotes(f: string): boolean {
  if (f === '') return false;
  if (f === '\\.') return true;
  if (/[,"\r\n]/.test(f)) return true;
  return /^\s/u.test(f);
}

function field(f: string): string {
  if (!needsQuotes(f)) return f;
  return '"' + f.replace(/"/g, '""') + '"';
}

const line = (cells: string[]) => cells.map(field).join(',') + '\n';

/** The CSV file of form fm: id, createdAt, then one column per field (labels as headers). */
export function exportCSV(fm: Form): string {
  const subs = listSubmissions(fm.id);
  const about = subs.some((s) => s.data._about); // 2.2: a "sentFrom" column when the form sat on a post's page
  let out = '\ufeff' + line(['id', 'createdAt', ...fm.fields.map((f) => csvSafe(f.label)), ...(about ? ['sentFrom'] : [])]);
  for (const s of subs) {
    out += line([String(s.id), s.createdAt, ...fm.fields.map((f) => csvSafe(s.data[f.key] ?? '')), ...(about ? [csvSafe(s.data._about ?? '')] : [])]);
  }
  return out;
}
