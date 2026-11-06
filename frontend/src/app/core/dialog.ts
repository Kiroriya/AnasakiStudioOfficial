import Swal, { SweetAlertIcon, SweetAlertOptions } from 'sweetalert2';
import { tr } from './i18n';

/**
 * App-wide dialogs and toasts (SweetAlert2), styled to match the admin.
 * Use these instead of window.confirm / prompt / alert.
 */

/** Flat line icons shown in a tinted square (replaces SweetAlert's stock icons). */
const ICON_SVG: Record<SweetAlertIcon, string> = {
  warning: '<path d="M12 9v4m0 4h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z"/>',
  error: '<circle cx="12" cy="12" r="9"/><path d="m15 9-6 6m0-6 6 6"/>',
  success: '<circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 16v-4m0-4h.01"/>',
  question: '<circle cx="12" cy="12" r="9"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3m.1 4h.01"/>',
};
const iconHtml = (i: SweetAlertIcon) =>
  `<svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICON_SVG[i]}</svg>`;

const classes = (icon: SweetAlertIcon | undefined, danger = false) => ({
  container: 'swal-dialog-bg', // only dialogs dim the page; toasts never do
  popup: 'swal-popup',
  title: 'swal-title',
  htmlContainer: 'swal-text',
  icon: `swal-ic swal-ic-${danger ? 'danger' : icon ?? 'info'}`,
  confirmButton: danger ? 'btn swal-btn btn-danger-solid' : 'btn swal-btn btn-primary',
  cancelButton: 'btn swal-btn swal-btn-cancel',
  denyButton: 'btn swal-btn btn-danger',
  input: 'swal-input',
  inputLabel: 'swal-input-label',
  validationMessage: 'swal-validation',
  actions: 'swal-actions',
  closeButton: 'swal-close',
});

/** Fires a centered dialog with our icon + classes. */
function fire(opts: SweetAlertOptions & { danger?: boolean }) {
  const { danger, ...o } = opts;
  const icon = o.icon as SweetAlertIcon | undefined;
  return Swal.fire({
    buttonsStyling: false,
    reverseButtons: true,
    showClass: { popup: 'swal-in' },
    hideClass: { popup: 'swal-out' },
    ...o,
    ...(icon ? { iconHtml: iconHtml(icon) } : {}),
    customClass: classes(icon, danger),
  } as SweetAlertOptions);
}

const toastMixin = Swal.mixin({
  toast: true,
  position: 'bottom',
  showConfirmButton: false,
  timer: 2600,
  timerProgressBar: true,
  showClass: { popup: 'swal-toast-in' },
  hideClass: { popup: 'swal-toast-out' },
});

const toastClasses = (icon: SweetAlertIcon) => ({
  popup: 'swal-toast', title: 'swal-toast-title', icon: `swal-toast-ic swal-ic-${icon}`,
  timerProgressBar: 'swal-toast-bar', confirmButton: 'swal-undo',
});

export const Dialog = {
  /** Yes/no question. `danger` styles the confirm button red. Resolves true when confirmed. */
  async confirm(opts: { title: string; text?: string; confirm?: string; cancel?: string; danger?: boolean; icon?: SweetAlertIcon }): Promise<boolean> {
    const r = await fire({
      title: opts.title,
      text: opts.text,
      icon: opts.icon ?? (opts.danger ? 'warning' : 'question'),
      danger: opts.danger,
      showCancelButton: true,
      confirmButtonText: opts.confirm ?? tr('common.yes'),
      cancelButtonText: opts.cancel ?? tr('common.cancel'),
      focusCancel: !!opts.danger,
    });
    return r.isConfirmed;
  },

  /** Shortcut for destructive confirmations ("Delete X?"). */
  confirmDelete(name: string, text?: string): Promise<boolean> {
    return Dialog.confirm({ title: tr('dialog.deleteTitle', { name }), text: text ?? tr('dialog.deleteText'), confirm: tr('common.delete'), danger: true });
  },

  /** Single text input. Resolves the value, or null when cancelled. */
  async prompt(opts: { title: string; value?: string; placeholder?: string; label?: string; required?: boolean }): Promise<string | null> {
    const r = await fire({
      title: opts.title,
      input: 'text',
      inputLabel: opts.label,
      inputValue: opts.value ?? '',
      inputPlaceholder: opts.placeholder,
      showCancelButton: true,
      confirmButtonText: tr('common.ok'),
      cancelButtonText: tr('common.cancel'),
      inputValidator: v => (opts.required !== false && !v.trim() ? tr('dialog.required') : null),
    });
    return r.isConfirmed ? String(r.value).trim() : null;
  },

  /**
   * Name + address of a new page (2.3.0, Qraft feedback 29). The address follows the name until it is edited;
   * a Thai name gives a Thai address, so the dialog suggests a short English one. Resolves null when cancelled;
   * an empty slug lets the server make it from the name.
   */
  async nameAndSlug(opts: { title: string; nameLabel: string; value?: string; slugLabel: string; slugHint: string; slugWarn: string; prefix?: string }): Promise<{ title: string; slug: string } | null> {
    const esc = (s: string) => s.replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!);
    const auto = (s: string) => s.trim().toLowerCase().replace(/[\s_]+/g, '-').replace(/[^\p{L}\p{M}\p{N}-]+/gu, '').replace(/-+/g, '-').replace(/^-|-$/g, '').slice(0, 80);
    const r = await fire({
      title: opts.title,
      html: `<label class="swal-input-label" for="dlg-name">${esc(opts.nameLabel)}</label>
        <input id="dlg-name" class="swal-input swal2-input" value="${esc(opts.value ?? '')}" autocomplete="off">
        <label class="swal-input-label" for="dlg-slug">${esc(opts.slugLabel)}</label>
        <div class="dlg-slug-row">${opts.prefix ? `<span class="dlg-slug-prefix">${esc(opts.prefix)}</span>` : ''}<input id="dlg-slug" class="swal-input swal2-input mono" autocomplete="off" spellcheck="false" placeholder="about-us"></div>
        <p class="dlg-slug-hint" id="dlg-slug-hint">${esc(opts.slugHint)}</p>`,
      showCancelButton: true,
      confirmButtonText: tr('common.ok'),
      cancelButtonText: tr('common.cancel'),
      focusConfirm: false,
      didOpen: (el) => {
        const name = el.querySelector<HTMLInputElement>('#dlg-name')!;
        const slug = el.querySelector<HTMLInputElement>('#dlg-slug')!;
        const hint = el.querySelector<HTMLElement>('#dlg-slug-hint')!;
        let touched = false;
        const check = () => {
          // only ASCII letters, digits and "-" make a short link when shared
          const bad = /[^a-z0-9-]/i.test(slug.value || (touched ? '' : auto(name.value)));
          hint.textContent = bad ? opts.slugWarn : opts.slugHint;
          hint.classList.toggle('warn', bad);
        };
        const follow = () => { if (!touched) slug.value = /^[ -~]*$/.test(name.value) ? auto(name.value) : ''; check(); };
        name.addEventListener('input', follow);
        slug.addEventListener('input', () => { touched = slug.value !== ''; check(); });
        follow();
        name.focus();
        name.select();
      },
      preConfirm: () => {
        const title = (document.getElementById('dlg-name') as HTMLInputElement).value.trim();
        const slug = auto((document.getElementById('dlg-slug') as HTMLInputElement).value);
        if (!title) { Swal.showValidationMessage(tr('dialog.required')); return false; }
        return { title, slug };
      },
    });
    return r.isConfirmed ? (r.value as { title: string; slug: string }) : null;
  },

  /** Choice from a list (options keep their order). Resolves the chosen value, or null when cancelled. */
  async select(opts: { title: string; text?: string; options: [value: string, label: string][]; value?: string; confirm?: string; danger?: boolean }): Promise<string | null> {
    const r = await fire({
      title: opts.title,
      text: opts.text,
      icon: opts.danger ? 'warning' : 'question',
      danger: opts.danger,
      input: 'select',
      inputOptions: new Map(opts.options),
      inputValue: opts.value ?? opts.options[0]?.[0] ?? '',
      showCancelButton: true,
      confirmButtonText: opts.confirm ?? tr('common.ok'),
      cancelButtonText: tr('common.cancel'),
      focusCancel: !!opts.danger,
    });
    return r.isConfirmed ? String(r.value) : null;
  },

  /** Two actions + cancel. Resolves 'confirm', 'deny' (styled as the risky one) or null when cancelled. */
  async choose(opts: { title: string; text?: string; confirm: string; deny: string; cancel?: string; icon?: SweetAlertIcon }): Promise<'confirm' | 'deny' | null> {
    const r = await fire({
      title: opts.title,
      text: opts.text,
      icon: opts.icon ?? 'warning',
      showCancelButton: true,
      showDenyButton: true,
      confirmButtonText: opts.confirm,
      denyButtonText: opts.deny,
      cancelButtonText: opts.cancel ?? tr('common.cancel'),
    });
    return r.isConfirmed ? 'confirm' : r.isDenied ? 'deny' : null;
  },

  alert(title: string, text?: string, icon: SweetAlertIcon = 'info') {
    return fire({ title, text, icon, confirmButtonText: tr('common.ok') });
  },

  error(text: string, title = tr('common.error')) {
    return fire({ title, text, icon: 'error', confirmButtonText: tr('common.ok') });
  },

  /** Small non-blocking notification at the bottom of the screen. */
  toast(title: string, icon: SweetAlertIcon = 'success') {
    return toastMixin.fire({ title, icon, iconHtml: iconHtml(icon), customClass: toastClasses(icon) });
  },

  /** Toast with an action button (e.g. "Undo"); resolves true when it was clicked. */
  async undoToast(title: string, action: string, ms = 5000): Promise<boolean> {
    const r = await toastMixin.fire({
      title, icon: 'info', iconHtml: iconHtml('info'), timer: ms, showConfirmButton: true, confirmButtonText: action,
      buttonsStyling: false, customClass: toastClasses('info'),
    });
    return r.isConfirmed;
  },
};
