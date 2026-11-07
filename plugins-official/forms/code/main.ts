// forms 2.2 — the built-in form builder as a JS plugin: forms designed in the admin page "Forms", placed on
// pages with the server-rendered designer block "forms/form" (site.block; the same node as before, so existing
// pages keep working; the form is chosen from a list), submissions in an inbox with CSV export, the
// "forms.submitted" event (also delivered as the former core event "form.submitted"), per-form webhooks, the
// package webhook setting and a mail to the site's admin address. 2.2: file fields (CV, photo, document) kept as
// private files and downloaded from the inbox.
import { getForm } from './model';
import { CHOOSE, NOT_FOUND, formPage, page, renderForm } from './render';
import { notify, submit } from './submit';
import { registerBlock } from './block';
import { registerAdmin } from './admin';
import { legacyImport } from './legacy';

const html = (body: string): RouteResult => ({
  status: 200,
  headers: { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' },
  body,
});

const idOf = (s: string) => (/^\d+$/.test(s) ? Number(s) : 0);

// 5 MB: forms with file fields (2.2, a file up to 4 MB)
site.route('POST /forms/{id}/submit', { access: ['*'], maxBody: 5 << 20 }, ({ request }) => submit(request));

// the form markup alone (older pages' block script loaded it from here)
site.route('GET /forms/{id}/embed', { access: ['*'] }, ({ request }) => {
  const id = idOf(request.params.id);
  if (!id) return html(CHOOSE);
  const fm = getForm(id);
  return html(fm ? renderForm(fm) : NOT_FOUND);
});

// the form on its own page (in the site's layout)
site.route('GET /forms/{id}', { access: ['*'] }, ({ request }) => {
  const fm = getForm(idOf(request.params.id));
  return fm ? formPage(fm) : page('ไม่พบฟอร์ม / Form not found', `<p>ไม่พบฟอร์ม / Form not found</p>`, 404);
});

site.on('forms.submitted', (ev: any) => notify(ev));

registerAdmin();
registerBlock();

/** Rows of the built-in plugin's tables (core migration). */
site.export('legacyImport', legacyImport);
