// → p_members_members, p_members_sessions, p_members_tokens
export default migration({
  up(schema) {
    schema.create('members', (t) => {
      t.id();
      t.string('email', 190).unique();
      t.string('name', 100);
      t.string('password_hash', 100);
      t.enum('status', ['active', 'disabled']).default('active');
      t.datetime('verified_at').nullable();
      t.json('groups').nullable();          // extra visitor groups, e.g. ["gold"]
      t.datetime('last_login_at').nullable();
      t.timestamps();
    });
    schema.create('sessions', (t) => {
      t.id();
      t.string('token_hash', 64).unique();  // sha256 of the cookie value; the cookie itself is never stored
      t.foreignId('member_id').references('members').onDelete('cascade');
      t.datetime('expires_at');
      t.timestamps();
    });
    schema.create('tokens', (t) => {
      t.id();
      t.string('token_hash', 64).unique();
      t.foreignId('member_id').references('members').onDelete('cascade');
      t.enum('purpose', ['verify', 'reset']);
      t.datetime('expires_at');
      t.datetime('used_at').nullable();
      t.timestamps();
    });
  },
  down(schema) {
    schema.drop('tokens');
    schema.drop('sessions');
    schema.drop('members');
  },
});
