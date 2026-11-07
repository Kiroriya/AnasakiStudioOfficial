// eshop 1.1: a receipt / tax invoice per paid order (its number, and the buyer's tax details when asked for at
// checkout) and the member who ordered (order history at /shop/orders with the Members plugin).
export default migration({
  up(schema) {
    schema.table('orders', (t) => {
      t.string('receipt_no', 40).default('');
      t.string('tax_name', 191).default('');
      t.string('tax_id', 20).default('');
      t.text('tax_address').nullable();
      t.string('member_id', 64).default('');
      t.index(['member_id']);
    });
  },
  down(schema) {
    schema.table('orders', (t) => {
      t.dropColumn(['receipt_no', 'tax_name', 'tax_id', 'tax_address', 'member_id']);
    });
  },
});
