// members 1.4.0 — a member who joined while the site asked for no confirmation (no e-mail yet) is never asked later,
// when e-mail is switched on.
export default migration({
  up(schema) {
    schema.table('members', (t) => {
      t.boolean('verify_waived').default(false);
    });
  },
  down(schema) {
    schema.table('members', (t) => {
      t.dropColumn(['verify_waived']);
    });
  },
});
