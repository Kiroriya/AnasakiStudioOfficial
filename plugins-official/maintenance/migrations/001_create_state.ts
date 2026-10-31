// Small key/value state (→ p_maintenance_state): the bypass key generated when maintenance mode is on
// and the "bypassKey" setting is empty (the built-in plugin stored it in its settings).
export default migration({
  up(schema) {
    schema.create('state', (t) => {
      t.id();
      t.string('k', 64).unique();
      t.text('v');
      t.datetime('updated_at').useCurrent();
    });
  },
  down(schema) {
    schema.drop('state');
  },
});
