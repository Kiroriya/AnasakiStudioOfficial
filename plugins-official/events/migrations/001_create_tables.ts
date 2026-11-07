// Registrations for events (→ p_events_registrations): the event is a post of the content type chosen in the
// settings; a registration keeps its title and date as they were.
export default migration({
  up(schema) {
    schema.create('registrations', (t) => {
      t.id();
      t.string('code', 32).unique();
      t.bigInteger('post_id');
      t.string('title', 191).default('');
      t.string('starts', 20).default('');
      t.string('name', 191);
      t.string('email', 191).default('');
      t.string('phone', 40).default('');
      t.integer('seats').default(1);
      t.string('note', 500).default('');
      t.string('status', 20).default('confirmed');
      t.datetime('checked_in_at').nullable();
      t.datetime('created_at').useCurrent();
      t.index(['post_id', 'status']);
    });
  },
  down(schema) {
    schema.drop('registrations');
  },
});
