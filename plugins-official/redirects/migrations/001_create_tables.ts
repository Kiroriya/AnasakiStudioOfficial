// Redirect rules (→ p_redirects_rules) and the 404 log (→ p_redirects_notfound), the JS twins of the
// built-in plugin's redirects_rules / redirects_404 tables. The built-in matched paths case-sensitively
// (utf8mb4_bin); here the unique keys are SHA-256 hashes of the exact path, so "/Old" and "/old" stay
// two different rules / 404 paths whatever the table collation.
export default migration({
  up(schema) {
    schema.create('rules', (t) => {
      t.id();
      t.string('from_path', 191);
      t.string('from_key', 64).unique();
      t.string('to_url', 1000);
      t.integer('code').default(301);
      t.integer('hits').default(0);
      t.datetime('created_at').useCurrent();
    });
    schema.create('notfound', (t) => {
      t.id();
      t.string('path', 191);
      t.string('path_key', 64).unique();
      t.integer('hits').default(0);
      t.datetime('last_seen').useCurrent();
      t.index(['hits', 'last_seen']);
    });
  },
  down(schema) {
    schema.drop('notfound');
    schema.drop('rules');
  },
});
