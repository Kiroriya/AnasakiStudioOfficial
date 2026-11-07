// eshop 1.1: a line's variant (its name at the time of the order and its place in the product's list, for
// the stock), an order's coupon and discount, and the parcel's carrier and tracking number.
export default migration({
  up(schema) {
    schema.table('items', (t) => {
      t.string('variant', 120).default('');
      t.integer('variant_index').default(-1);
    });
    schema.table('orders', (t) => {
      t.decimal('discount', 12, 2).default(0);
      t.string('coupon', 40).default('');
      t.string('carrier', 60).default('');
      t.string('tracking', 120).default('');
    });
  },
  down(schema) {
    schema.table('orders', (t) => {
      t.dropColumn(['discount', 'coupon', 'carrier', 'tracking']);
    });
    schema.table('items', (t) => {
      t.dropColumn(['variant', 'variant_index']);
    });
  },
});
