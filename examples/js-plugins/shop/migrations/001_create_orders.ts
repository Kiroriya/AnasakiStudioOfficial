export default migration({
  up(schema) {
    schema.create('orders', (t) => {   // → p_shop_orders
      t.id();
      t.string('item', 100);
      t.decimal('total', 12, 2);
      t.timestamps();
    });
  },
  down(schema) { schema.drop('orders'); },
});
