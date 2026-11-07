// Online payments (→ p_payments_payments): one row per reference ("eshop:<order code>", "donations:<code>" …)
// with the provider's checkout session, the amount and whether it is paid.
export default migration({
  up(schema) {
    schema.create('payments', (t) => {
      t.id();
      t.string('ref', 120).unique();
      t.string('provider', 20).default('stripe');
      t.string('session_id', 191).default('');
      t.decimal('amount', 12, 2).default(0);
      t.string('currency', 3).default('thb');
      t.string('description', 191).default('');
      t.string('status', 20).default('open');
      t.datetime('paid_at').nullable();
      t.datetime('created_at').useCurrent();
      t.index(['status', 'id']);
    });
  },
  down(schema) {
    schema.drop('payments');
  },
});
