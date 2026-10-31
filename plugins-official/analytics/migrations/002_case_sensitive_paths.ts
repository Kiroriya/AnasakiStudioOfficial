// Case-sensitive paths (like the built-in plugin's utf8mb4_bin keys): /About and /about are counted apart.
export default migration({
  up(schema) {
    schema.table('daily', (t) => {
      t.change(t.string('path', 191).binary());
    });
    schema.table('page_visitors', (t) => {
      t.change(t.string('path', 191).binary());
    });
  },
  down(schema) {
    schema.table('daily', (t) => {
      t.change(t.string('path', 191));
    });
    schema.table('page_visitors', (t) => {
      t.change(t.string('path', 191));
    });
  },
});
