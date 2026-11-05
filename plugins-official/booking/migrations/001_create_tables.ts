// Services, weekly hours, closed days and bookings (→ p_booking_*). Times are minutes after midnight in the
// site's time zone; a booking keeps its service's name and minutes as they were.
export default migration({
  up(schema) {
    schema.create('services', (t) => {
      t.id();
      t.string('name', 120);
      t.string('description', 500).default('');
      t.integer('minutes').default(60);
      t.decimal('price', 12, 2).default(0);
      t.integer('capacity').default(1);
      t.boolean('active').default(true);
      t.integer('sort').default(0);
    });
    schema.create('hours', (t) => {
      t.id();
      t.integer('weekday');
      t.integer('open_min').default(540);
      t.integer('close_min').default(1080);
      t.boolean('closed').default(false);
      t.unique(['weekday']);
    });
    schema.create('closed_days', (t) => {
      t.id();
      t.date('day').unique();
      t.string('note', 120).default('');
    });
    schema.create('bookings', (t) => {
      t.id();
      t.string('code', 32).unique();
      t.bigInteger('service_id');
      t.string('service_name', 120);
      t.date('day');
      t.integer('start_min');
      t.integer('end_min');
      t.string('name', 120);
      t.string('phone', 40);
      t.string('email', 191).default('');
      t.text('note');
      t.string('status', 20).default('confirmed');
      t.datetime('created_at').useCurrent();
      t.index(['day', 'status']);
    });
  },
  down(schema) {
    schema.drop('bookings');
    schema.drop('closed_days');
    schema.drop('hours');
    schema.drop('services');
  },
});
