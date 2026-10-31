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
      if (!id) return { html: CHOOSE, css: formCSS() };
      const fm = getForm(id);
      // ?sent=<id>: back from a no-JavaScript submit — show the success message (the script does it with JS)
      const sent = String(ctx.query?.sent ?? '') === String(id);
      return { html: fm ? renderForm(fm, sent) : NOT_FOUND, css: formCSS() };
    },
  });
}
