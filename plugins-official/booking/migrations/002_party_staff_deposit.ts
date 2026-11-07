// booking 1.1: party size (a service's seats), staff who serve the bookings, a deposit paid by PromptPay or
// transfer with a slip, answers to the shop's own questions and the reminder mail.
export default migration({
  up(schema) {
    schema.create('staff', (t) => {
      t.id();
      t.string('name', 120);
      // the services this person gives: ids joined by commas ("" = every service)
      t.string('services', 500).default('');
      t.boolean('active').default(true);
      t.integer('sort').default(0);
    });
    schema.table('services', (t) => {
      t.integer('max_party').default(1);
      t.decimal('deposit', 12, 2).default(0);
    });
    schema.table('bookings', (t) => {
      t.integer('party').default(1);
      t.bigInteger('staff_id').default(0);
      t.string('staff_name', 120).default('');
      t.text('answers').nullable();
      t.decimal('deposit', 12, 2).default(0);
      t.string('slip', 191).default('');
      t.datetime('slip_at').nullable();
      t.datetime('reminded_at').nullable();
    });
  },
  down(schema) {
    schema.table('bookings', (t) => {
      t.dropColumn(['party', 'staff_id', 'staff_name', 'answers', 'deposit', 'slip', 'slip_at', 'reminded_at']);
    });
    schema.table('services', (t) => {
      t.dropColumn(['max_party', 'deposit']);
    });
    schema.drop('staff');
  },
});
