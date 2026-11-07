// Courses: who may study a course (→ p_courses_enrollments: free / members courses enrol on the first visit,
// paid ones once paid or granted) and the lessons each member finished (→ p_courses_progress).
export default migration({
  up(schema) {
    schema.create('enrollments', (t) => {
      t.id();
      t.string('code', 32).unique();
      t.bigInteger('course_id');
      t.string('member_id', 64);
      t.string('member_name', 191).default('');
      t.string('status', 20).default('active');
      t.decimal('amount', 12, 2).default(0);
      t.datetime('created_at').useCurrent();
      t.unique(['course_id', 'member_id']);
    });
    schema.create('progress', (t) => {
      t.id();
      t.string('member_id', 64);
      t.bigInteger('course_id');
      t.bigInteger('lesson_id');
      t.datetime('done_at').useCurrent();
      t.unique(['member_id', 'lesson_id']);
      t.index(['member_id', 'course_id']);
    });
  },
  down(schema) {
    schema.drop('progress');
    schema.drop('enrollments');
  },
});
