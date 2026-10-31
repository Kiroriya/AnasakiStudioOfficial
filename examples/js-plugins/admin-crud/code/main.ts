// admin-crud: a generic list / search / paginate / create / edit / delete screen (site.adminPage) for
// this plugin's own tables — or data owned by another JS plugin, through its exports (plugins.call) —
// described in code/config.json (or the "resources" setting). Writes go through site.action
// (anasaki.call inside the admin page), so the Access screen can change who may save or delete.
// "Export CSV" is the admin page's download handler (anasaki.download → same ticket and role check).
import { resources, resource, validate, label, sourceOf, isUnavailable, Resource, ColumnDef } from './resources';
import { csvOf, csvDisposition } from './csv';

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? '';
const qs = (p: Record<string, string | number | undefined>) =>
  '?' + Object.entries(p).filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => k + '=' + encodeURIComponent(String(v))).join('&');

function shown(c: ColumnDef, v: any, en: boolean): string {
  if (v === null || v === undefined) return '<span class="muted">—</span>';
  if ((c.type ?? 'text') === 'bool') return v ? '✓' : '✗';
  if (c.type === 'datetime' && typeof v === 'string') return esc(v.replace('T', ' ').slice(0, 16));
  const s = String(v);
  return esc(s.length > 80 ? s.slice(0, 80) + '…' : s);
}

function input(c: ColumnDef, v: any, en: boolean): string {
  const name = esc(c.name);
  const req = c.required ? ' required' : '';
  const val = v === null || v === undefined ? '' : v;
  switch (c.type ?? 'text') {
    case 'textarea':
      return `<textarea name="${name}" rows="4"${req}>${esc(val)}</textarea>`;
    case 'bool':
      return `<input type="checkbox" name="${name}"${val ? ' checked' : ''}>`;
    case 'select':
      return `<select name="${name}"${req}>${(c.required ? '' : '<option value=""></option>') +
        (c.options ?? []).map((o) => `<option${o === val ? ' selected' : ''}>${esc(o)}</option>`).join('')}</select>`;
    case 'number':
      return `<input type="number" step="any" name="${name}" value="${esc(val)}"${req}>`;
    case 'email':
      return `<input type="email" name="${name}" value="${esc(val)}"${req}>`;
    case 'date':
      return `<input type="date" name="${name}" value="${esc(String(val).slice(0, 10))}"${req}>`;
    case 'datetime':
      return `<input type="datetime-local" name="${name}" value="${esc(String(val).replace(' ', 'T').slice(0, 16))}"${req}>`;
    default:
      return `<input type="text" name="${name}" value="${esc(val)}"${c.max ? ` maxlength="${c.max}"` : ''}${req}>`;
  }
}

const STYLE = `<style>.tabs a{margin-right:12px}.tabs a.on{font-weight:700;text-decoration:none}.muted{color:#6b7280}
.bar{display:flex;gap:8px;align-items:center;margin:16px 0}.pager{margin-top:12px;display:flex;gap:8px;align-items:center}
form.edit label{display:block;margin:12px 0 4px;font-weight:600}form.edit input:not([type=checkbox]),form.edit select,form.edit textarea{width:100%;max-width:560px;box-sizing:border-box}
.err{color:#b91c1c;font-size:13px}.danger{color:#b91c1c;border-color:#fca5a5}</style>`;

/**
 * English or Thai: ?lang=en|th on the page, else ctx.locale (the signed-in user's admin language,
 * phase 6), else the "lang" setting (older cores without ctx.locale).
 */
function isEn(ctx: { query?: Record<string, string | string[]>; locale?: string }): boolean {
  const q = one(ctx.query?.lang);
  if (q === 'en' || q === 'th') return q === 'en';
  if (ctx.locale) return ctx.locale === 'en';
  return site.settings('lang') === 'en';
}

// Client script of the page: save / delete through anasaki.call, then reload.
const SCRIPT = `<script>
function acSave(f, res, id) {
  var values = {};
  Array.prototype.forEach.call(f.elements, function (el) {
    if (!el.name) return;
    values[el.name] = el.type === 'checkbox' ? el.checked : el.value;
  });
  f.querySelectorAll('.err').forEach(function (e) { e.textContent = ''; });
  function show(errs, msg) {
    Object.keys(errs).forEach(function (k) { var el = f.querySelector('[data-err="' + k + '"]'); if (el) el.textContent = errs[k]; });
    if (!Object.keys(errs).length) alert(msg);
  }
  anasaki.call('admin-crud:save', { resource: res, id: id || null, values: values }).then(function (d) {
    if (d.errors && Object.keys(d.errors).length) return show(d.errors, '');
    location.href = '?r=' + encodeURIComponent(res) + (/[?&]lang=en/.test(location.search) ? '&lang=en' : '');
  }, function (e) { show(e.errors || {}, e.message); });
  return false;
}
function acDelete(res, id, q) {
  if (!confirm(q)) return;
  anasaki.call('admin-crud:delete', { resource: res, id: id }).then(function () { location.reload(); }, function (e) { alert(e.message); });
}
</script>`;

function listView(res: Resource, query: Record<string, string | string[]>, en: boolean, lang?: string): string {
  const t = (th: string, e: string) => (en ? e : th);
  const src = sourceOf(res, en);
  if (!src.available) return `<p class="notice" data-ac="unavailable">⚠ ${esc(src.reason)}</p>`;
  const q = one(query.q).trim().slice(0, 100);
  const page = Math.max(1, Math.floor(Number(one(query.page)) || 1));
  const perPage = Math.min(100, Math.max(1, res.perPage ?? 20));
  const cols = res.columns.filter((c) => c.list);
  const pg = src.list(q, page, perPage);
  const pages = Math.max(1, Math.ceil(pg.total / perPage));
  let h = `<div class="bar"><form method="get"><input type="hidden" name="r" value="${esc(res.id)}">${lang ? `<input type="hidden" name="lang" value="${lang}">` : ''}` +
    `<input type="search" name="q" value="${esc(q)}" placeholder="${t('ค้นหา', 'Search')}"> <button>${t('ค้นหา', 'Search')}</button></form>` +
    (src.canCreate ? `<a href="${qs({ r: res.id, edit: 'new', lang })}"><button class="primary" type="button" data-ac="new">${t('เพิ่ม', 'New')}</button></a>` : '') +
    `<button type="button" data-ac="export" onclick="anasaki.download(${esc(JSON.stringify({ r: res.id, q: q || undefined, lang }))})">` +
    `${t('ส่งออก CSV', 'Export CSV')}</button>` +
    `<span class="muted">${pg.total} ${t('รายการ', 'rows')}</span></div>`;
  h += `<table data-ac="list"><thead><tr><th>#</th>${cols.map((c) => `<th>${esc(label(c.label, c.name, en))}</th>`).join('')}<th></th></tr></thead><tbody>`;
  for (const row of pg.items) {
    h += `<tr data-id="${esc(row.id)}"><td>${esc(row.id)}</td>${cols.map((c) => `<td>${shown(c, row[c.name], en)}</td>`).join('')}` +
      `<td style="white-space:nowrap">${src.canEdit ? `<a href="${qs({ r: res.id, edit: row.id, lang })}">${t('แก้ไข', 'Edit')}</a> ` : ''}` +
      (src.canDelete ? `<button class="danger" onclick="acDelete(${esc(JSON.stringify(res.id))}, ${Number(row.id)}, ${esc(JSON.stringify(t('ลบรายการนี้?', 'Delete this row?')))})">${t('ลบ', 'Delete')}</button>` : '') +
      `</td></tr>`;
  }
  if (!pg.items.length) h += `<tr><td colspan="${cols.length + 2}" class="muted">${t('ไม่มีข้อมูล', 'Nothing here yet')}</td></tr>`;
  h += '</tbody></table>';
  if (pages > 1) {
    h += `<div class="pager">${page > 1 ? `<a href="${qs({ r: res.id, q, page: page - 1, lang })}">‹ ${t('ก่อนหน้า', 'Previous')}</a>` : ''}` +
      `<span>${page} / ${pages}</span>${page < pages ? `<a href="${qs({ r: res.id, q, page: page + 1, lang })}">${t('ถัดไป', 'Next')} ›</a>` : ''}</div>`;
  }
  return h;
}

function editView(res: Resource, id: string, en: boolean, lang?: string): string {
  const t = (th: string, e: string) => (en ? e : th);
  const src = sourceOf(res, en);
  if (!src.available) return `<p class="notice" data-ac="unavailable">⚠ ${esc(src.reason)}</p>`;
  const isNew = id === 'new';
  if (isNew ? !src.canCreate : !src.canEdit) return `<p>${t('รายการนี้อ่านได้อย่างเดียว', 'This resource is read-only')}</p>`;
  const row = isNew ? {} : src.get(id);
  if (!row) return `<p>${t('ไม่พบรายการ', 'Not found')}</p>`;
  return `<form class="edit" data-ac="form" onsubmit="return acSave(this, ${esc(JSON.stringify(res.id))}, ${isNew ? 'null' : Number(id)})">` +
    `<h2>${isNew ? t('เพิ่ม', 'New') : t('แก้ไข', 'Edit') + ' #' + esc(id)}</h2>` +
    res.columns.map((c) => `<label>${esc(label(c.label, c.name, en))}${c.required ? ' *' : ''}</label>${input(c, (row as any)[c.name], en)}<div class="err" data-err="${esc(c.name)}"></div>`).join('') +
    `<p><button class="primary">${t('บันทึก', 'Save')}</button> <a href="${qs({ r: res.id, lang })}">${t('ยกเลิก', 'Cancel')}</a></p></form>`;
}

/** All rows of a resource for the CSV export (at most 10 000, in pages of 100 — the paginate maximum). */
function allRows(res: Resource, q: string, en: boolean): Record<string, any>[] {
  const src = sourceOf(res, en);
  const out: Record<string, any>[] = [];
  for (let page = 1; out.length < 10000; page++) {
    const pg = src.list(q, page, 100);
    out.push(...pg.items);
    if (pg.items.length < 100 || out.length >= pg.total) break;
  }
  return out.slice(0, 10000);
}

site.adminPage('data', {
  label: { th: 'จัดการข้อมูล', en: 'Data' },
  icon: '🗂️',
  render: (ctx) => {
    const { query } = ctx;
    const en = isEn(ctx);
    // ?lang= is carried in links only when it was given (else ctx.locale decides)
    const ql = one(query.lang);
    const lang = ql === 'en' || ql === 'th' ? ql : undefined;
    const all = resources();
    if (!all.length) return '<p>No resources configured (code/config.json or the "resources" setting).</p>';
    const res = resource(one(query.r)) ?? all[0];
    const tabs = all.map((r) => `<a class="${r.id === res.id ? 'on' : ''}" href="${qs({ r: r.id, lang })}">${esc(label(r.label, r.id, en))}</a>`).join('');
    const edit = one(query.edit);
    return {
      title: label(res.label, res.id, en),
      html: STYLE + SCRIPT + `<nav class="tabs">${tabs}</nav>` + (edit ? editView(res, edit, en, lang) : listView(res, query, en, lang)),
    };
  },
  // GET …/download?r=<resource>&q=<search> → CSV of the (filtered) resource, as the signed-in user
  download: (ctx) => {
    const { query } = ctx;
    const en = isEn(ctx);
    const res = resource(one(query.r)) ?? resources()[0];
    if (!res) return { status: 404, body: 'no resources' };
    try {
      const rows = allRows(res, one(query.q).trim().slice(0, 100), en);
      const cols = [{ name: 'id', label: '#' } as ColumnDef, ...res.columns];
      return {
        status: 200,
        headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': csvDisposition(res.id + '.csv'), 'Cache-Control': 'no-store' },
        body: csvOf(rows, cols.map((c) => ({ key: c.name, label: label(c.label, c.name, en) }))),
      };
    } catch (e) {
      if (isUnavailable(e)) return { status: 409, headers: { 'Content-Type': 'text/plain; charset=utf-8' }, body: String((e as Error).message) };
      throw e;
    }
  },
});

/** Runs a data-source operation; "plugin not installed" becomes a form error instead of a 500. */
function guarded<T>(fn: () => T): T | { errors: Record<string, string>; message: string } {
  try {
    return fn();
  } catch (e) {
    if (!isUnavailable(e)) throw e;
    const message = String((e as Error).message);
    return { errors: { form: message }, message };
  }
}

site.action('save', {
  input: { resource: 'text', id: 'number?', values: 'json' },
  run: (ctx) => {
    const { input } = ctx;
    const res = resource(input.resource);
    if (!res) return { errors: { resource: 'unknown resource' } };
    const en = isEn(ctx);
    const v = validate(res, input.values ?? {}, en);
    if (v.errors) return { errors: v.errors };
    return guarded(() => {
      const out = sourceOf(res, en).save(input.id || null, v.row as Record<string, any>);
      if (out.errors) return { errors: out.errors };
      const created = !input.id;
      site.emit('admin-crud.saved', { resource: res.id, id: out.id, created });
      return { message: created ? (en ? 'Created' : 'เพิ่มแล้ว') : en ? 'Saved' : 'บันทึกแล้ว', data: { id: out.id } };
    });
  },
});

site.action('delete', {
  input: { resource: 'text', id: 'number' },
  run: (ctx) => {
    const { input } = ctx;
    const res = resource(input.resource);
    if (!res) return { errors: { resource: 'unknown resource' } };
    const en = isEn(ctx);
    return guarded(() => {
      const src = sourceOf(res, en);
      if (!src.canDelete) return { errors: { form: en ? 'Read-only' : 'อ่านอย่างเดียว' } };
      const n = src.delete(input.id);
      if (n) site.emit('admin-crud.deleted', { resource: res.id, id: input.id });
      return { message: n ? (en ? 'Deleted' : 'ลบแล้ว') : en ? 'Not found' : 'ไม่พบ', data: { deleted: n } };
    });
  },
});

// Public data for the designer: published FAQs (code:admin-crud:faqs).
site.collection('faqs', {
  label: { th: 'คำถามที่พบบ่อย', en: 'FAQ' },
  fields: { question: 'text', answer: 'text' },
  access: ['*'],
  cache: 60,
  load: () => db.table('faqs').where('published', true).orderBy('sort').select('question', 'answer').get(),
});
