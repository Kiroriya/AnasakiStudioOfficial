// Orders get a status; unpaid ("pending") orders are cancelled by the cron in code/main.ts.
export default migration({
  up(schema) {
    schema.table('orders', (t) => {
      t.enum('status', ['pending', 'paid', 'cancelled']).default('pending');
      t.index(['status', 'created_at']);
    });
  },
  down(schema) {
    schema.table('orders', (t) => {
      t.dropIndex('status_created_at_index');
      t.dropColumn('status');
    });
  },
});
