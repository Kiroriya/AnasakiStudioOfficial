// Subscribers (→ p_newsletter_subscribers) and campaigns (→ p_newsletter_campaigns). A subscriber's token is in
// their confirm / unsubscribe links; a campaign remembers the last subscriber it reached (cursor) so sending can
// stop and go on later.
export default migration({
  up(schema) {
    schema.create('subscribers', (t) => {
      t.id();
      t.string('email', 191).unique();
      t.string('name', 100).default('');
      t.string('status', 20).default('pending');
      t.string('token', 64).unique();
      t.string('source', 40).default('form');
      t.datetime('confirmed_at').nullable();
      t.datetime('created_at').useCurrent();
      t.index(['status', 'id']);
    });
    schema.create('campaigns', (t) => {
      t.id();
      t.string('subject', 200);
      t.text('body');
      t.string('status', 20).default('draft');
      t.bigInteger('cursor').default(0);
      t.integer('sent').default(0);
      t.integer('failed').default(0);
      t.integer('total').default(0);
      t.datetime('started_at').nullable();
      t.datetime('finished_at').nullable();
      t.datetime('created_at').useCurrent();
    });
  },
  down(schema) {
    schema.drop('campaigns');
    schema.drop('subscribers');
  },
});
