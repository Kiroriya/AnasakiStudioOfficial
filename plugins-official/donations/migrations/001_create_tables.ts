// Donations (→ p_donations_gifts): the amount, the donor (or anonymous), the slip and, when asked for, the details
// of a receipt for tax deduction; a receipt number is given when the gift is confirmed.
export default migration({
  up(schema) {
    schema.create('gifts', (t) => {
      t.id();
      t.string('code', 32).unique();
      t.decimal('amount', 12, 2);
      t.string('campaign', 120).default('');
      t.string('name', 191).default('');
      t.string('email', 191).default('');
      t.string('phone', 40).default('');
      t.string('message', 500).default('');
      t.boolean('anonymous').default(false);
      t.boolean('receipt').default(false);
      t.string('tax_id', 20).default('');
      t.text('address').nullable();
      t.string('status', 20).default('pending');
      t.string('slip', 191).default('');
      t.datetime('slip_at').nullable();
      t.string('receipt_no', 40).default('');
      t.datetime('confirmed_at').nullable();
      t.datetime('created_at').useCurrent();
      t.index(['status', 'id']);
    });
  },
  down(schema) {
    schema.drop('gifts');
  },
});
