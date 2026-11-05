// Orders (→ p_eshop_orders) and their lines (→ p_eshop_items). Products are the post type eshop_product;
// a line keeps the title and price at the time of the order.
export default migration({
  up(schema) {
    schema.create('orders', (t) => {
      t.id();
      t.string('code', 32).unique();
      t.string('status', 20).default('pending');
      t.string('payment', 20).default('promptpay');
      t.string('name', 191);
      t.string('phone', 40).default('');
      t.string('email', 191).default('');
      t.text('address');
      t.text('note');
      t.decimal('subtotal', 12, 2).default(0);
      t.decimal('shipping', 12, 2).default(0);
      t.decimal('total', 12, 2).default(0);
      t.string('slip', 191).default('');
      t.datetime('slip_at').nullable();
      t.datetime('paid_at').nullable();
      t.datetime('created_at').useCurrent();
      t.datetime('updated_at').useCurrent();
      t.index(['status', 'id']);
    });
    schema.create('items', (t) => {
      t.id();
      t.bigInteger('order_id');
      t.bigInteger('product_id');
      t.string('title', 191);
      t.decimal('price', 12, 2).default(0);
      t.integer('qty').default(1);
      t.index(['order_id']);
    });
  },
  down(schema) {
    schema.drop('items');
    schema.drop('orders');
  },
});
