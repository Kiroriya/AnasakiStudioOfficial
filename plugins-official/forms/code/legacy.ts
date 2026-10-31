// legacyImport: rows of the built-in plugin's forms_forms and forms_submissions → p_forms_forms /
// p_forms_submissions. Rows keep their old ids (designer blocks reference forms by id), so importing a batch
// twice updates the same rows instead of adding new ones.
type Row = Record<string, any>;

const str = (v: unknown, max: number) => {
  const s = v === null || v === undefined ? '' : String(v);
  return Array.from(s).slice(0, max).join('');
};

/** A JSON column of a legacy row: parsed value or JSON text → JSON text (def when invalid). */
function jsonText(v: unknown, def: string): string {
  if (v === null || v === undefined || v === '') return def;
  if (typeof v === 'string') {
    try {
      JSON.parse(v);
      return v;
    } catch {
      return def;
    }
  }
  return JSON.stringify(v);
}

/** DATETIME of a legacy row (RFC 3339, "YYYY-MM-DD HH:MM:SS" UTC, Date) → Date; now when missing. */
function when(v: unknown): Date {
  if (v instanceof Date && !isNaN(v.getTime())) return v;
  let s = String(v ?? '').trim();
  if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}/.test(s)) s = s.replace(' ', 'T');
  if (/^\d{4}-\d{2}-\d{2}T[\d:.]+$/.test(s)) s += 'Z';
  const d = new Date(s);
  return s && !isNaN(d.getTime()) ? d : new Date();
}

const truthy = (v: unknown) => v === true || v === 1 || v === '1' || v === 'true';

function upsert(table: string, rows: Row[]): number {
  const ids = rows.map((r) => r.id);
  const existing = new Set<number>(ids.length ? (db.table(table).whereIn('id', ids).pluck('id') as any[]).map(Number) : []);
  const inserts: Row[] = [];
  for (const r of rows) {
    if (existing.has(r.id)) {
      const { id, ...patch } = r;
      db.table(table).where('id', id).update(patch);
    } else inserts.push(r);
  }
  for (let i = 0; i < inserts.length; i += 500) db.table(table).insert(inserts.slice(i, i + 500));
  return rows.length;
}

function forms(rows: Row[]): number {
  const out: Row[] = [];
  for (const r of rows) {
    const id = Math.floor(Number(r.id));
    if (!(id > 0)) continue;
    out.push({
      id, name: str(r.name, 191) || 'Form ' + id, fields: jsonText(r.fields, '[]'), submit_label: str(r.submit_label, 100),
      success_message: str(r.success_message, 1000), redirect_url: str(r.redirect_url, 512), webhook_url: str(r.webhook_url, 512),
      created_at: when(r.created_at),
    });
  }
  return upsert('forms', out);
}

function submissions(rows: Row[]): number {
  const out: Row[] = [];
  for (const r of rows) {
    const id = Math.floor(Number(r.id));
    const formId = Math.floor(Number(r.form_id));
    if (!(id > 0) || !(formId > 0)) continue;
    out.push({ id, form_id: formId, data: jsonText(r.data, '{}'), is_read: truthy(r.is_read), created_at: when(r.created_at) });
  }
  return upsert('submissions', out);
}

export function legacyImport(p: { kind: string; table: string; rows: Row[]; last?: boolean }): { imported: number } {
  if (!p || p.kind !== 'rows' || !Array.isArray(p.rows)) return { imported: 0 };
  if (p.table === 'forms_forms') return { imported: forms(p.rows) };
  if (p.table === 'forms_submissions') return { imported: submissions(p.rows) };
  return { imported: 0 };
}
