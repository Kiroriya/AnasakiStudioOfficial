// Public markup of a form — the Go plugin's RenderForm (same HTML, classes, honeypot "_hp"), plus the small
// pages the public routes answer.
import { DEFAULT_SUBMIT, DEFAULT_SUCCESS, Form } from './model';
import { ACCEPT, FILE_HINT } from './files';

/** Go's html.EscapeString (& < > ' " → &amp; &lt; &gt; &#39; &#34;). */
export const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);

/**
 * Accessible HTML of a form (labels, required, honeypot "_hp") posting to /forms/{id}/submit. `sent` = the page was
 * reached through the no-JavaScript redirect after a successful submit (?sent=<id>): the success message is shown.
 */
export function renderForm(fm: Form, sent = false, about = ''): string {
  const e = esc;
  const sid = String(fm.id);
  const success = fm.successMessage || DEFAULT_SUCCESS;
  const submit = fm.submitLabel || DEFAULT_SUBMIT;
  // 2.2: a form with a file field posts multipart/form-data
  const enc = fm.fields.some((f) => f.type === 'file') ? ' enctype="multipart/form-data"' : '';
  let b = `<form class="rea-form" id="rea-form-${sid}" method="post" action="/forms/${sid}/submit"${enc} data-rea-form="${sid}" data-success="${e(success)}">`;
  for (const fd of fm.fields) {
    const id = 'rea-f' + sid + '-' + fd.key;
    const req = fd.required ? ' required' : '';
    const star = fd.required ? '<span class="rea-form-req" aria-hidden="true"> *</span>' : '';
    const ph = fd.placeholder ? ` placeholder="${e(fd.placeholder)}"` : '';
    const name = ` name="${e(fd.key)}" id="${e(id)}"`;
    // 2.1: shown only when an earlier field has a value (assets/form.js; without it every field shows)
    const cond = fd.showIf ? ` data-show-if="${e(fd.showIf.field)}" data-show-value="${e(fd.showIf.value)}"` : '';
    if (fd.type === 'checkbox') {
      b += `<div class="rea-form-field"${cond}><label class="rea-form-field rea-form-check"><input type="checkbox"${name} value="yes"${req}> ${e(fd.label)}${star}</label></div>`;
      continue;
    }
    if (fd.type === 'radio' || fd.type === 'checkboxes') {
      const multi = fd.type === 'checkboxes';
      b += `<fieldset class="rea-form-field rea-form-group"${cond}${multi && fd.required ? ' data-required="1"' : ''}><legend>${e(fd.label)}${star}</legend>`;
      fd.options.forEach((o, i) => {
        b += `<label class="rea-form-check"><input type="${multi ? 'checkbox' : 'radio'}" name="${e(fd.key)}${multi ? '[]' : ''}" id="${e(id)}-${i}" value="${e(o)}"` +
          `${!multi ? req : ''}> ${e(o)}</label>`;
      });
      b += '</fieldset>';
      continue;
    }
    b += `<div class="rea-form-field"${cond}><label for="${e(id)}">${e(fd.label)}${star}</label>`;
    switch (fd.type) {
      case 'textarea':
        b += `<textarea${name}${ph}${req} maxlength="5000"></textarea>`;
        break;
      case 'select':
        b += `<select${name}${req}><option value="">—</option>` + fd.options.map((o) => `<option value="${e(o)}">${e(o)}</option>`).join('') + '</select>';
        break;
      case 'file':
        b += `<input type="file"${name}${req} accept="${ACCEPT}"><small class="rea-form-hint">${e(fd.placeholder || FILE_HINT)}</small>`;
        break;
      default: {
        const auto = fd.type === 'email' ? ' autocomplete="email"' : fd.type === 'tel' ? ' autocomplete="tel"' : '';
        b += `<input type="${fd.type}"${name}${ph}${req}${auto} maxlength="1000">`;
      }
    }
    b += '</div>';
  }
  b += '<div class="rea-form-hp" aria-hidden="true"><label>Leave this empty<input type="text" name="_hp" tabindex="-1" autocomplete="off"></label></div>';
  if (about) b += `<input type="hidden" name="_about" value="${e(about.slice(0, 300))}">`; // 2.2: the page it was sent from
  b += `<button type="submit" class="rea-form-submit">${e(submit)}</button>`;
  b += sent
    ? `<p class="rea-form-msg" role="status" aria-live="polite">${e(success)}</p></form>`
    : '<p class="rea-form-msg" role="status" aria-live="polite" hidden></p></form>';
  return b;
}

export const CHOOSE = '<p class="rea-empty">เลือกฟอร์มในแผงคุณสมบัติ / Choose a form</p>';
export const NOT_FOUND = '<p class="rea-empty">ไม่พบฟอร์ม / Form not found</p>';

/** The form's client script (package asset, also loaded with the designer block). */
export const SCRIPT = 'assets/form.js';

export function scriptTag(): string {
  try {
    const u = site.asset(SCRIPT).url;
    return u ? `<script defer src="${esc(u)}"></script>` : '';
  } catch {
    return '';
  }
}

/** The form CSS as written (unscoped: core scopes the block's CSS under .pb-forms-form). */
export function formCSS(): string {
  try {
    return site.asset('templates/form.css').text();
  } catch {
    return '';
  }
}

/** formCSS scoped under .pb-forms-form by hand (pages outside the designer). */
function scopedCSS(): string {
  return formCSS().replace(/</g, '').replace(/(^|\})\s*([^@{}][^{}]*)\{/g, (_m, end, sel) =>
    end + sel.split(',').map((s: string) => '.pb-forms-form ' + s.trim()).join(',') + '{');
}

/** A page inside the site's header and footer (route result {page}). */
export function page(title: string, body: string, status = 200): RouteResult {
  return {
    page: { title, html: `<style>.rea-page{max-width:640px;margin:64px auto;padding:0 20px}.rea-page h1{font-size:28px;font-weight:700}${scopedCSS()}</style>` +
      `<div class="rea-page">${body}</div>` },
    status,
    headers: { 'Cache-Control': 'no-store' },
  };
}

/** GET /forms/{id}: the form on its own page (a shareable link). */
export function formPage(fm: Form): RouteResult {
  return page(fm.name, `<h1>${esc(fm.name)}</h1><div class="pb-forms-form">${renderForm(fm)}</div>${scriptTag()}`);
}

/** The "could not send" page of a classic (no-JS) post (in the site's layout, like the built-in plugin). */
export function failPage(status: number, msg: string, back: string): RouteResult {
  return page('ส่งฟอร์มไม่สำเร็จ', `<h1>ส่งฟอร์มไม่สำเร็จ</h1><p style="margin:16px 0">${esc(msg)}</p><p><a href="${esc(back)}">← กลับ / Go back</a></p>`, status);
}
