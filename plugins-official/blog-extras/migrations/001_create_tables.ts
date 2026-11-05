// Comments (→ p_blog_extras_comments). email is never shown on the site; ip_hash only rate-limits.
export default migration({
  up(schema) {
    schema.create('comments', (t) => {
      t.id();
      t.bigInteger('post_id');
      t.bigInteger('parent_id').default(0);
      t.string('name', 100);
      t.string('email', 191).default('');
      t.text('body');
      t.string('status', 20).default('pending');
      t.string('ip_hash', 64).default('');
      t.datetime('created_at').useCurrent();
      t.index(['post_id', 'status', 'id']);
      t.index(['status', 'id']);
    });
  },
  down(schema) {
    schema.drop('comments');
  },
});
