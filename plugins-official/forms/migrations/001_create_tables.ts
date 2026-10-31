// Forms and submissions (→ p_forms_forms, p_forms_submissions): the built-in plugin's forms_forms and
// forms_submissions. Legacy rows keep their ids, so designer blocks (props.formId) still point at them.
export default migration({
  up(schema) {
    schema.create('forms', (t) => {
      t.id();
      t.string('name', 191);
      t.json('fields');
      t.string('submit_label', 100).default('');
      t.string('success_message', 1000).default('');
      t.string('redirect_url', 512).default('');
      t.string('webhook_url', 512).default('');
      t.datetime('created_at').useCurrent();
    });
    schema.create('submissions', (t) => {
      t.id();
      t.bigInteger('form_id');
      t.json('data');
      t.boolean('is_read').default(false);
      t.datetime('created_at').useCurrent();
      t.index(['form_id', 'id']);
    });
  },
  down(schema) {
    schema.drop('submissions');
    schema.drop('forms');
  },
});
