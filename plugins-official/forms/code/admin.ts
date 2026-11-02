// The "Forms" admin page (was the Angular page /forms): builder, submissions inbox, CSV export. The page is
// drawn in the browser by assets/admin.js (admin UI kit); every change goes through the actions below,
// which the Access screen can restrict further. CMS 2.0.6+: the plugin's own capabilities, ticked per role in
// Users → Roles — forms.manage (build / delete forms) and forms.view_submissions (inbox, CSV export), both
// defaulting to administrators and editors. Older CMS versions: editors (was capability edit_pages).
import {
  createForm, deleteForm, deleteSubmission, getForm, listForms, listSubmissions, markRead, updateForm, validateForm,
} from './model';
import { exportCSV } from './csv';

const CAPS = typeof (site as any).can === 'function';
const MANAGE = CAPS ? ['cap:forms.manage'] : ['editor'];
const SUBMISSIONS = CAPS ? ['cap:forms.view_submissions'] : ['editor'];
const ACCESS = CAPS ? ['cap:forms.manage', 'cap:forms.view_submissions'] : ['editor'];

/** JSON inside <script type="application/json"> (no "</script>" breakout). */
const jsonData = (v: unknown) => JSON.stringify(v).replace(/</g, '\\u003c').replace(/>/g, '\\u003e').replace(/&/g, '\\u0026');

function assetURL(path: string): string {
  try {
    return site.asset(path).url;
  } catch {
    return '';
  }
}

export function registerAdmin(): void {
  site.adminPage('forms', {
    label: { th: 'แบบฟอร์ม', en: 'Forms' },
    icon: '📝',
    access: ACCESS,
    render: (ctx) => {
      const en = ctx.locale === 'en';
      const sel = Number(Array.isArray(ctx.query?.form) ? ctx.query.form[0] : ctx.query?.form) || 0;
      const data = { locale: en ? 'en' : 'th', forms: listForms(), selected: sel };
      return {
        title: en ? 'Forms' : 'แบบฟอร์ม',
        html: `<div id="forms-app" data-ready="0"><p class="ak-muted">${en ? 'Loading…' : 'กำลังโหลด…'}</p></div>` +
          `<script type="application/json" id="forms-data">${jsonData(data)}</script>` +
          `<script src="${assetURL('assets/admin.js')}" defer></script>`, // after the kit (deferred in <head>)
      };
    },
    // GET …/download?form=<id> → form-<id>.csv
    download: (ctx) => {
      if (CAPS && !ctx.user?.can('forms.view_submissions')) {
        return { status: 403, headers: { 'Content-Type': 'text/plain; charset=utf-8' }, body: 'ไม่มีสิทธิ์ดูข้อความที่ส่งเข้ามา' };
      }
      const id = Number(Array.isArray(ctx.query?.form) ? ctx.query.form[0] : ctx.query?.form) || 0;
      const fm = getForm(id);
      if (!fm) return { status: 404, headers: { 'Content-Type': 'text/plain; charset=utf-8' }, body: 'ไม่พบฟอร์มนี้' };
      return {
        status: 200,
        headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': `attachment; filename="form-${fm.id}.csv"`, 'Cache-Control': 'no-store' },
        body: exportCSV(fm),
      };
    },
  });

  const fail = (message: string) => ({ message, errors: { form: message } });
  const NOT_FOUND = 'ไม่พบฟอร์มนี้';

  site.action('list', { input: {}, access: ACCESS, run: () => ({ data: listForms() }) });

  site.action('save', {
    input: { form: 'json' },
    access: MANAGE,
    run: ({ input }) => {
      const { form, error } = validateForm(input.form);
      if (error) return fail(error);
      if (form.id > 0) {
        const saved = updateForm(form.id, form);
        return saved ? { data: saved } : fail(NOT_FOUND);
      }
      return { data: createForm(form) };
    },
  });

  site.action('delete', {
    input: { id: 'number' },
    access: MANAGE,
    run: ({ input }) => {
      deleteForm(Number(input.id));
      return { data: { deleted: true } };
    },
  });

  site.action('submissions', {
    input: { formId: 'number' },
    access: SUBMISSIONS,
    run: ({ input }) => (getForm(Number(input.formId)) ? { data: listSubmissions(Number(input.formId)) } : fail(NOT_FOUND)),
  });

  site.action('markRead', {
    input: { id: 'number', read: 'bool' },
    access: SUBMISSIONS,
    run: ({ input }) => {
      const s = markRead(Number(input.id), input.read === true);
      return s ? { data: s } : fail('ไม่พบข้อความนี้');
    },
  });

  site.action('deleteSubmission', {
    input: { id: 'number' },
    access: SUBMISSIONS,
    run: ({ input }) => {
      deleteSubmission(Number(input.id));
      return { data: { deleted: true } };
    },
  });
}
