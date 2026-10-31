// Aggregate tables (→ p_analytics_*), same data as the built-in plugin's analytics_* tables plus a per-day
// totals table (the query builder has no GROUP BY aggregates in fallback mode, so the daily chart reads
// pre-aggregated rows) and the daily visitor-hash salt (was option analytics_salt).
export default migration({
  up(schema) {
    schema.create('daily', (t) => {
      t.id();
      t.date('day');
      t.string('path', 191);
      t.string('title', 255).default('');
      t.integer('views').default(0);
      t.unique(['day', 'path']);
    });
    schema.create('visitors', (t) => {
      t.id();
      t.date('day');
      t.string('hash', 16);
      t.unique(['day', 'hash']);
    });
    schema.create('page_visitors', (t) => {
      t.id();
      t.date('day');
      t.string('path', 191);
      t.string('hash', 16);
      t.unique(['day', 'path', 'hash']);
    });
    schema.create('referrers', (t) => {
      t.id();
      t.date('day');
      t.string('host', 191);
      t.integer('views').default(0);
      t.unique(['day', 'host']);
    });
    schema.create('days', (t) => {
      t.id();
      t.date('day').unique();
      t.integer('views').default(0);
      t.integer('visitors').default(0);
    });
    schema.create('salts', (t) => {
      t.id();
      t.date('day').unique();
      t.string('salt', 64);
    });
  },
  down(schema) {
    for (const n of ['salts', 'days', 'referrers', 'page_visitors', 'visitors', 'daily']) schema.drop(n);
  },
});
