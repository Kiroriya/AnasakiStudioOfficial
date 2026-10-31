// The sample tables managed by code/config.json (→ p_admin_crud_contacts, p_admin_crud_faqs).
// Add your own tables with new migrations and describe them in config.json (or the "resources" setting).
export default migration({
  up(schema) {
    schema.create('contacts', (t) => {
      t.id();
      t.string('name', 100);
      t.string('email', 190).nullable();
      t.string('phone', 30).nullable();
      t.enum('status', ['new', 'contacted', 'closed']).default('new');
      t.text('note').nullable();
      t.timestamps();
      t.index(['status', 'created_at']);
    });
    schema.create('faqs', (t) => {
      t.id();
      t.string('question', 255);
      t.text('answer');
      t.integer('sort').default(0);
      t.boolean('published').default(true);
      t.timestamps();
    });
  },
  down(schema) {
    schema.drop('faqs');
    schema.drop('contacts');
  },
});
