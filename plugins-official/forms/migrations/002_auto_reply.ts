// forms 2.1: the auto-reply mail of a form (empty subject = none).
export default migration({
  up(schema) {
    schema.table('forms', (t) => {
      t.string('auto_reply_subject', 200).default('');
      t.text('auto_reply_message').nullable();
    });
  },
  down(schema) {
    schema.table('forms', (t) => {
      t.dropColumn(['auto_reply_subject', 'auto_reply_message']);
    });
  },
});
