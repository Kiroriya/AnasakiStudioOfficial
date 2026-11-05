// The designer block "forms/form" (was the built-in Block with PropsFunc + Render): rendered on the server
// (works without JavaScript, indexable), the form chosen from a list of the existing forms. The node stays
// {plugin: "forms", block: "form", values: {formId}}, so pages made with the built-in plugin keep working.
import { getForm, listForms } from './model';
import { CHOOSE, NOT_FOUND, SCRIPT, formCSS, renderForm } from './render';

export function registerBlock(): void {
  site.block('form', {
    label: { th: 'ฟอร์ม', en: 'Form' },
    icon: '📝',
    props: [{ key: 'formId', type: 'select', label: { th: 'ฟอร์ม', en: 'Form' } }],
    // the select of the property panel: every form (number — name)
    options: () => ({ formId: listForms().map((f) => ({ value: String(f.id), label: `#${f.id} — ${f.name}` })) }),
    script: SCRIPT,
    render: (ctx, props) => {
      const id = /^\d+$/.test(String(props.formId ?? '').trim()) ? Number(props.formId) : 0;
      // 2.0.3: "choose a form" / "form not found" are hints for the designer canvas; visitors see nothing
      // (ctx.preview since CMS 2.2.2 — older cores do not tell, and keep showing the hint)
      const hint = (h: string) => ({ html: (ctx as any).preview === false ? '' : h, css: formCSS() });
      if (!id) return hint(CHOOSE);
      const fm = getForm(id);
      if (!fm) return hint(NOT_FOUND);
      // ?sent=<id>: back from a no-JavaScript submit — show the success message (the script does it with JS)
      const sent = String(ctx.query?.sent ?? '') === String(id);
      return { html: renderForm(fm, sent), css: formCSS() };
    },
  });
}
