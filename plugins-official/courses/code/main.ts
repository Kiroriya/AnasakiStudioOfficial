// courses 1.0 — online courses on the site: courses (/course/<slug>) and their lessons (/lesson/<slug>), each
// lesson tied to a course with its place and a video. A course is free (anyone reads its lessons), for members
// (signed in with the Members plugin) or paid (an enrolment, paid by card through the Payments plugin when it is
// installed, else granted by the owner on the Students page). Locked lessons show a sign-in / enrol box instead
// of their content; members mark lessons done, see their progress on the course page and print a certificate.
// The reader comes from site.visitor (Members); the lock is applied in the post.content filter (CMS 2.5.0 gives it
// the visitor), so give lessons their own excerpt — lists show excerpts, never the locked content.

const COURSE = 'courses_course';
const LESSON = 'courses_lesson';
const CAPS = typeof (site as any).can === 'function';
const ACCESS = CAPS ? ['cap:courses.manage'] : ['editor'];

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);
const baht = (n: number) => '฿' + (Math.round(n * 100) / 100).toLocaleString('en-US');

function lang(): 0 | 1 {
  try {
    return String(site.info().language || 'th').startsWith('th') ? 0 : 1;
  } catch {
    return 0;
  }
}
const T = {
  lessons: ['บทเรียน', 'Lessons'], start: ['เริ่มเรียน', 'Start learning'], continue: ['เรียนต่อ', 'Continue'],
  enroll: ['ลงทะเบียนเรียน', 'Enrol'], buy: ['ซื้อคอร์สนี้ {price}', 'Buy this course {price}'], signIn: ['เข้าสู่ระบบเพื่อเรียน', 'Sign in to study'],
  free: ['เรียนฟรี', 'Free'], membersOnly: ['สำหรับสมาชิก', 'For members'], preview: ['ดูตัวอย่างได้', 'Preview'],
  locked: ['บทเรียนนี้สำหรับผู้ลงทะเบียนคอร์ส “{course}”', 'This lesson is for students of “{course}”'],
  lockedMembers: ['บทเรียนนี้สำหรับสมาชิก', 'This lesson is for members'],
  progress: ['เรียนแล้ว {done} จาก {all} บท', '{done} of {all} lessons done'], done: ['✓ เรียนจบบทนี้', '✓ Mark as done'], doneAlready: ['✓ เรียนจบแล้ว', '✓ Done'],
  next: ['บทถัดไป →', 'Next lesson →'], back: ['← กลับไปหน้าคอร์ส', '← Back to the course'],
  pending: ['รอยืนยันการชำระเงิน — เปิดสิทธิ์ให้เมื่อได้รับเงิน', 'Waiting for the payment — access opens once it is received'],
  certificate: ['🎓 ใบประกาศนียบัตร', '🎓 Certificate'], certTitle: ['ใบประกาศนียบัตร', 'Certificate of completion'],
  certBody: ['ขอมอบให้ไว้เพื่อแสดงว่า', 'This certifies that'], certDone: ['ได้เรียนจบหลักสูตร', 'has completed the course'],
  print: ['พิมพ์ / บันทึกเป็น PDF', 'Print / save as PDF'], notFound: ['ไม่พบ', 'Not found'], minutes: ['นาที', 'min'],
} as const;
type K = keyof typeof T;
const t = (k: K, p?: Record<string, string | number>) => {
  let s: string = T[k][lang()];
  for (const [a, b] of Object.entries(p ?? {})) s = s.split(`{${a}}`).join(String(b));
  return s;
};

site.postType(COURSE, {
  label: { th: 'คอร์สเรียน', en: 'Courses' }, singular: { th: 'คอร์ส', en: 'Course' }, icon: '🎓', slug: 'course',
  supports: ['title', 'content', 'excerpt', 'image', 'categories'],
  fields: {
    access: { type: 'select', label: { th: 'ใครเรียนได้', en: 'Who can study' }, required: true, hideOnSite: true,
      options: [{ value: 'free', label: { th: 'ทุกคน (ฟรี)', en: 'Anyone (free)' } }, { value: 'members', label: { th: 'สมาชิกที่เข้าสู่ระบบ', en: 'Signed-in members' } },
        { value: 'paid', label: { th: 'ผู้ซื้อคอร์ส', en: 'Students who paid' } }] },
    price: { type: 'number', label: { th: 'ราคา (บาท)', en: 'Price (THB)' }, min: 0, hideOnSite: 'page' },
    level: { type: 'text', label: { th: 'ระดับ', en: 'Level' } },
    hours: { type: 'number', label: { th: 'ความยาว (ชั่วโมง)', en: 'Length (hours)' }, min: 0 },
  },
  ...(CAPS ? { caps: { edit: 'courses.manage' } } : {}),
} as any);

site.postType(LESSON, {
  label: { th: 'บทเรียน', en: 'Lessons' }, singular: { th: 'บทเรียน', en: 'Lesson' }, icon: '📘', slug: 'lesson',
  supports: ['title', 'content', 'excerpt', 'image'],
  fields: {
    course: { type: 'relation', postType: COURSE, label: { th: 'อยู่ในคอร์ส', en: 'Course' }, required: true, hideOnSite: true },
    order: { type: 'number', label: { th: 'ลำดับบท', en: 'Lesson number' }, min: 0, hideOnSite: true },
    video: { type: 'url', label: { th: 'วิดีโอ (YouTube / Vimeo)', en: 'Video (YouTube / Vimeo)' }, hideOnSite: true },
    minutes: { type: 'number', label: { th: 'ความยาว (นาที)', en: 'Length (minutes)' }, min: 0 },
    preview: { type: 'bool', label: { th: 'ให้ดูตัวอย่างได้โดยไม่ต้องลงทะเบียน', en: 'Free preview' }, hideOnSite: true },
  },
  ...(CAPS ? { caps: { edit: 'courses.manage' } } : {}),
} as any);

// ---------------------------------------------------------------- data

interface Course { id: number; title: string; url: string; access: 'free' | 'members' | 'paid'; price: number }
interface Lesson { id: number; title: string; url: string; course: number; order: number; video: string; minutes: number; preview: boolean }
interface Enrollment { id: number; code: string; course_id: number; member_id: string; member_name: string; status: string; amount: string; created_at: string }

function metaOf(p: any): Record<string, any> {
  let m = p?.meta;
  if (typeof m === 'string') {
    try {
      m = JSON.parse(m);
    } catch {
      m = {};
    }
  }
  return m && typeof m === 'object' ? m : {};
}
const toCourse = (p: any): Course => {
  const m = metaOf(p);
  return { id: Number(p.id), title: String(p.title ?? ''), url: String(p.url ?? ''), access: ['free', 'members', 'paid'].includes(m.access) ? m.access : 'members', price: Math.max(0, Number(m.price) || 0) };
};
const toLesson = (p: any): Lesson => {
  const m = metaOf(p);
  const c = Array.isArray(m.course) ? m.course[0] : m.course;
  return { id: Number(p.id), title: String(p.title ?? ''), url: String(p.url ?? ''), course: Number(c) || 0, order: Number(m.order) || 0,
    video: String(m.video ?? ''), minutes: Number(m.minutes) || 0, preview: m.preview === true };
};
function getCourse(id: number): Course | null {
  const p = id ? db.posts.get(id, { type: COURSE }) : null;
  return p && p.type === COURSE && p.status === 'publish' ? toCourse(p) : null;
}
function lessonsOf(courseId: number): Lesson[] {
  const { items } = db.posts.list({ type: LESSON, status: 'publish', perPage: 100, where: { 'meta.course': courseId }, orderBy: 'meta.order' } as any);
  return items.map(toLesson).filter((l) => l.course === courseId).sort((a, b) => a.order - b.order || a.id - b.id);
}
const memberId = (v: any) => (v && v.id !== undefined && v.id !== null && String(v.id) !== '' ? String(v.id).slice(0, 64) : '');
const enrollment = (courseId: number, member: string) =>
  member ? (db.table('enrollments').where('course_id', courseId).where('member_id', member).first() as Enrollment | null) : null;
const doneIds = (member: string, courseId: number) =>
  new Set((db.table('progress').where('member_id', member).where('course_id', courseId).select('lesson_id').get() as { lesson_id: number }[]).map((r) => Number(r.lesson_id)));

/** May member (the visitor's id, '' = anonymous) read the course's lessons? */
function canStudy(c: Course, visitor: any): boolean {
  if (c.access === 'free') return true;
  const m = memberId(visitor);
  if (!m) return false;
  if (c.access === 'members') return true;
  return enrollment(c.id, m)?.status === 'active';
}

function cardReady(): boolean {
  try {
    return plugins.has('payments') && plugins.call<boolean>('payments', 'available') === true;
  } catch {
    return false;
  }
}

// ---------------------------------------------------------------- the course page and the lesson page

const BOX = 'margin:28px 0;padding:20px;border:1px solid #e5e7eb;border-radius:14px';
const BTN = 'display:inline-flex;align-items:center;justify-content:center;min-height:46px;padding:0 22px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer;text-decoration:none';
const signInLink = (back: string) => `<a style="${BTN}" href="/members/login?next=${esc(encodeURIComponent(back))}">${esc(t('signIn'))}</a>`;

function enrolBox(c: Course, visitor: any): string {
  const m = memberId(visitor);
  if (c.access === 'free') return '';
  if (!m) return `<p>${esc(c.access === 'paid' ? t('buy', { price: baht(c.price) }) : t('membersOnly'))}</p>${signInLink(c.url)}`;
  if (c.access === 'members') return '';
  const e = enrollment(c.id, m);
  if (e?.status === 'active') return '';
  if (e?.status === 'pending' && !cardReady()) {
    const note = String(site.settings('paymentNote') ?? '').trim();
    return `<p style="color:#92400e">${esc(t('pending'))}</p>${note ? `<pre style="white-space:pre-wrap;font:inherit;background:#f9fafb;padding:12px;border-radius:8px">${esc(note)}</pre>` : ''}`;
  }
  return `<form method="post" action="/learn/enroll/${c.id}"><button type="submit" style="${BTN}" id="course-buy">${esc(c.price > 0 ? t('buy', { price: baht(c.price) }) : t('enroll'))}</button></form>`;
}

function coursePage(html: string, c: Course, visitor: any): string {
  const lessons = lessonsOf(c.id);
  const m = memberId(visitor);
  const open = canStudy(c, visitor);
  const done = m ? doneIds(m, c.id) : new Set<number>();
  const n = lessons.filter((l) => done.has(l.id)).length;
  const pct = lessons.length ? Math.round((n / lessons.length) * 100) : 0;
  const nextLesson = lessons.find((l) => !done.has(l.id)) ?? lessons[0];
  const badge = c.access === 'free' ? t('free') : c.access === 'members' ? t('membersOnly') : baht(c.price);
  let box = `<div class="course-box" style="${BOX}"><p style="margin:0 0 8px;font-weight:600">${esc(badge)}</p>`;
  if (open && m && lessons.length) {
    box += `<div role="progressbar" aria-valuenow="${pct}" aria-valuemin="0" aria-valuemax="100" style="height:10px;border-radius:999px;background:#e5e7eb;overflow:hidden;margin:8px 0">` +
      `<i style="display:block;height:100%;width:${pct}%;background:var(--c-primary,#4f46e5)"></i></div><p style="color:#6b7280;margin:0 0 12px">${esc(t('progress', { done: n, all: lessons.length }))}</p>`;
  }
  if (open && nextLesson) box += `<a style="${BTN}" href="${esc(nextLesson.url)}">${esc(n ? t('continue') : t('start'))}</a>`;
  else box += enrolBox(c, visitor);
  if (open && m && lessons.length && n === lessons.length && site.settings('certificate') !== false) {
    box += ` <a style="${BTN};background:#059669" href="/learn/certificate/${c.id}">${esc(t('certificate'))}</a>`;
  }
  box += '</div>';
  const list = lessons.length ? `<h2 style="font-size:20px;margin:24px 0 8px">${esc(t('lessons'))}</h2><ol class="course-lessons" style="padding-left:20px;line-height:2">` +
    lessons.map((l) => `<li>${done.has(l.id) ? '✅ ' : ''}${open || l.preview ? `<a href="${esc(l.url)}">${esc(l.title)}</a>` : esc(l.title) + ' 🔒'}` +
      `${l.minutes ? ` <small style="color:#6b7280">${l.minutes} ${esc(t('minutes'))}</small>` : ''}${l.preview && !open ? ` <small style="color:#059669">${esc(t('preview'))}</small>` : ''}</li>`).join('') + '</ol>' : '';
  return html + box + list;
}

/** A YouTube / Vimeo address as a player (nothing for other addresses). */
function videoHTML(url: string): string {
  let m = /^https:\/\/(?:www\.|m\.)?(?:youtube\.com\/watch\?v=|youtu\.be\/|youtube\.com\/embed\/)([\w-]{6,20})/.exec(url);
  let src = m ? `https://www.youtube-nocookie.com/embed/${m[1]}` : '';
  if (!src && (m = /^https:\/\/(?:www\.)?(?:player\.)?vimeo\.com\/(?:video\/)?(\d{4,12})/.exec(url))) src = `https://player.vimeo.com/video/${m[1]}`;
  return src ? `<div style="position:relative;padding-top:56.25%;margin:0 0 24px;border-radius:12px;overflow:hidden;background:#000">` +
    `<iframe src="${esc(src)}" style="position:absolute;inset:0;width:100%;height:100%;border:0" allow="autoplay; fullscreen; picture-in-picture" allowfullscreen loading="lazy" title="video"></iframe></div>` : '';
}

function lessonPage(html: string, l: Lesson, visitor: any): string {
  const c = getCourse(l.course);
  if (!c) return html;
  const back = `<p><a href="${esc(c.url)}">${esc(t('back'))}</a></p>`;
  if (!l.preview && !canStudy(c, visitor)) {
    const why = c.access === 'members' ? t('lockedMembers') : t('locked', { course: c.title });
    return back + `<div class="lesson-locked" style="${BOX};text-align:center"><p style="font-size:1.1em">🔒 ${esc(why)}</p>${enrolBox(c, visitor) || signInLink(l.url)}</div>`;
  }
  const m = memberId(visitor);
  let foot = '';
  if (m) {
    const done = doneIds(m, c.id).has(l.id);
    foot += done ? `<span style="color:#059669;font-weight:600">${esc(t('doneAlready'))}</span>`
      : `<form method="post" action="/learn/done/${l.id}" style="display:inline"><button type="submit" style="${BTN};background:#059669" id="lesson-done">${esc(t('done'))}</button></form>`;
  }
  const all = lessonsOf(c.id);
  const i = all.findIndex((x) => x.id === l.id);
  if (i >= 0 && i + 1 < all.length) foot += ` <a style="${BTN};background:#fff;color:inherit;border:1px solid #d1d5db" href="${esc(all[i + 1].url)}">${esc(t('next'))}</a>`;
  return back + videoHTML(l.video) + html + (foot ? `<div class="lesson-foot" style="display:flex;gap:12px;flex-wrap:wrap;align-items:center;margin:28px 0">${foot}</div>` : '');
}

site.filter('post.content', (html: string, ctx: any) => {
  const post = ctx?.post;
  if (!post) return html;
  if (post.type === COURSE) return coursePage(html, toCourse(post), ctx.visitor);
  if (post.type === LESSON) return lessonPage(html, toLesson(post), ctx.visitor);
  return html;
});

// ---------------------------------------------------------------- enrolment, progress, certificate

const go = (to: string): RouteResult => ({ status: 303, headers: { Location: to, 'Cache-Control': 'no-store' } });

site.route('POST /learn/enroll/{id}', { access: ['*'] }, ({ request, visitor }) => {
  const c = getCourse(Number(request.params.id) || 0);
  if (!c) return { status: 404, body: { error: t('notFound') } };
  const m = memberId(visitor);
  if (!m) return go('/members/login?next=' + encodeURIComponent(c.url));
  let e = enrollment(c.id, m);
  if (!e) {
    const status = c.access === 'paid' && c.price > 0 ? 'pending' : 'active';
    db.table('enrollments').insert({ code: crypto.random(12), course_id: c.id, member_id: m, member_name: String((visitor as any)?.name ?? '').slice(0, 191), status, amount: c.price });
    e = enrollment(c.id, m) as Enrollment;
    if (status === 'active') site.emit('courses.enrolled', { courseId: c.id, member: m });
  }
  if (e.status === 'pending' && cardReady()) {
    try {
      const r = plugins.call<{ url: string }>('payments', 'checkout', { ref: 'courses:' + e.code, amount: c.price, description: c.title,
        email: String((visitor as any)?.email ?? ''), successPath: c.url, cancelPath: c.url });
      if (r && /^https:\/\//.test(String(r.url))) return go(String(r.url));
    } catch (err) {
      log.warn('courses: card payment:', String((err as Error).message ?? err));
    }
  }
  return go(c.url);
});

site.on('payments.paid', (ev: any) => {
  const m = /^courses:([a-f0-9]{24})$/.exec(String(ev?.ref ?? ''));
  const e = m ? (db.table('enrollments').where('code', m[1]).first() as Enrollment | null) : null;
  if (!e || e.status !== 'pending' || Math.abs(Number(ev.amount) - Number(e.amount)) > 0.01) return;
  db.table('enrollments').where('id', e.id).update({ status: 'active' });
  site.emit('courses.enrolled', { courseId: e.course_id, member: e.member_id });
});

site.route('POST /learn/done/{id}', { access: ['*'] }, ({ request, visitor }) => {
  const p = db.posts.get(Number(request.params.id) || 0, { type: LESSON });
  if (!p || p.type !== LESSON || p.status !== 'publish') return { status: 404, body: { error: t('notFound') } };
  const l = toLesson(p);
  const c = getCourse(l.course);
  const m = memberId(visitor);
  if (!c || !m) return go('/members/login?next=' + encodeURIComponent(l.url));
  if (!canStudy(c, visitor) && !l.preview) return go(c.url);
  if (!db.table('progress').where('member_id', m).where('lesson_id', l.id).exists()) {
    db.table('progress').insert({ member_id: m, course_id: c.id, lesson_id: l.id });
    const all = lessonsOf(c.id);
    if (all.length && all.every((x) => doneIds(m, c.id).has(x.id))) site.emit('courses.completed', { courseId: c.id, member: m });
  }
  const all = lessonsOf(c.id);
  const i = all.findIndex((x) => x.id === l.id);
  return go(i >= 0 && i + 1 < all.length ? all[i + 1].url : c.url);
});

site.route('GET /learn/certificate/{id}', { access: ['*'] }, ({ request, visitor }) => {
  const c = getCourse(Number(request.params.id) || 0);
  const m = memberId(visitor);
  const all = c ? lessonsOf(c.id) : [];
  const done = c && m ? doneIds(m, c.id) : new Set<number>();
  if (!c || !m || !all.length || !all.every((l) => done.has(l.id)) || site.settings('certificate') === false || !canStudy(c, visitor)) {
    return { status: 404, headers: { 'Content-Type': 'text/plain; charset=utf-8' }, body: t('notFound') };
  }
  let issuer = String(site.settings('issuer') ?? '').trim();
  try {
    issuer = issuer || site.info().title || '';
  } catch {
    /* none */
  }
  const last = db.table('progress').where('member_id', m).where('course_id', c.id).orderBy('done_at', 'desc').first() as { done_at: string } | null;
  const d = new Date(last?.done_at ?? Date.now());
  const day = lang() === 0 ? `${d.getDate()}/${d.getMonth() + 1}/${d.getFullYear() + 543}` : d.toISOString().slice(0, 10);
  const name = String((visitor as any)?.name ?? '').trim() || String((visitor as any)?.email ?? '');
  const html = `<!doctype html><html lang="${lang() === 0 ? 'th' : 'en'}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">` +
    `<title>${esc(t('certTitle'))} — ${esc(c.title)}</title><style>body{margin:0;background:#f3f4f6;font-family:'Noto Sans Thai',system-ui,sans-serif;color:#111827}` +
    `.c{max-width:900px;margin:24px auto;background:#fff;padding:64px 48px;text-align:center;border:12px double #c7a548}h1{font-size:36px;margin:0 0 24px;letter-spacing:.04em}` +
    `.n{font-size:32px;font-weight:700;margin:12px 0}.k{font-size:24px;font-weight:600;margin:12px 0 32px}.f{display:flex;justify-content:space-between;margin-top:48px;color:#4b5563}` +
    `button{font:inherit;padding:10px 18px;border-radius:8px;border:1px solid #d1d5db;background:#fff;cursor:pointer;margin-top:24px}@media print{body{background:#fff}.c{margin:0}button{display:none}}</style></head><body>` +
    `<div class="c"><h1>${esc(t('certTitle'))}</h1><p>${esc(t('certBody'))}</p><p class="n">${esc(name)}</p><p>${esc(t('certDone'))}</p><p class="k">${esc(c.title)}</p>` +
    `<div class="f"><span>${esc(day)}</span><span>${esc(issuer)}</span></div><button onclick="print()">🖨 ${esc(t('print'))}</button></div></body></html>`;
  return { status: 200, headers: { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' }, body: html };
});

// ---------------------------------------------------------------- the Students admin page

const q1 = (v: unknown) => String(Array.isArray(v) ? v[0] : v ?? '');

site.adminPage('students', {
  label: { th: 'ผู้เรียน', en: 'Students' },
  icon: '🎓',
  access: ACCESS,
  render: (ctx) => {
    const en = ctx.locale === 'en';
    const L = (th: string, e: string) => (en ? e : th);
    const courseId = Number(q1(ctx.query.course)) || 0;
    let script = '';
    try {
      script = site.asset('assets/admin.js').url;
    } catch {
      /* none */
    }
    const head = `<header class="ak-page-head"><span class="ak-page-icon">🎓</span><div class="ak-page-head-text"><h1>${L('ผู้เรียน', 'Students')}</h1>` +
      `<p>${L('ผู้ลงทะเบียนและความคืบหน้าของแต่ละคอร์ส', 'Enrolments and progress per course')}</p></div></header>`;
    const courses = db.posts.list({ type: COURSE, perPage: 100, status: 'publish' } as any).items.map(toCourse);
    const tabs = `<div class="ak-tabs">${courses.map((c) => `<a class="ak-tab" href="?course=${c.id}" aria-selected="${c.id === courseId}">${esc(c.title)}</a>`).join('')}</div>`;
    const c = courses.find((x) => x.id === courseId);
    if (!c) return { title: L('ผู้เรียน', 'Students'), html: head + tabs + `<div class="ak-card"><p class="ak-muted">${L('เลือกคอร์ส', 'Choose a course')}</p></div>` };
    const all = lessonsOf(c.id).length;
    const rows = (db.table('enrollments').where('course_id', c.id).orderBy('id', 'desc').limit(500).get() as Enrollment[]).map((e) => {
      const n = doneIds(e.member_id, c.id).size;
      return `<tr><td>${esc(e.member_name || e.member_id)}</td><td>${esc(String(e.created_at).slice(0, 16).replace('T', ' '))}</td>` +
        `<td>${e.status === 'active' ? `<span class="ak-badge ak-badge-ok">${L('เรียนได้', 'Active')}</span>` : e.status === 'pending' ? `<span class="ak-badge ak-badge-warn">${L('รอชำระ', 'Unpaid')}</span>` : `<span class="ak-badge ak-badge-danger">${L('ปิดสิทธิ์', 'Removed')}</span>`}</td>` +
        `<td class="ak-num">${all ? Math.round((n / all) * 100) : 0}% (${n}/${all})</td><td><div class="ak-actions">` +
        (e.status !== 'active' ? `<button class="ak-btn ak-btn-sm ak-btn-primary" data-cs="active" data-id="${e.id}">${L('เปิดสิทธิ์', 'Grant')}</button>` : `<button class="ak-btn ak-btn-sm ak-btn-danger" data-cs="removed" data-id="${e.id}">${L('ปิดสิทธิ์', 'Remove')}</button>`) +
        '</div></td></tr>';
    }).join('');
    return {
      title: L('ผู้เรียน', 'Students'),
      html: head + tabs + (rows ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="cs-students"><thead><tr><th>${L('ผู้เรียน', 'Student')}</th><th>${L('ลงทะเบียน', 'Enrolled')}</th>` +
        `<th>${L('สิทธิ์', 'Access')}</th><th class="ak-num">${L('ความคืบหน้า', 'Progress')}</th><th></th></tr></thead><tbody>${rows}</tbody></table></div></div>`
        : `<div class="ak-card"><p class="ak-muted">${L('ยังไม่มีผู้ลงทะเบียน', 'No students yet')}</p></div>`) + (script ? `<script src="${esc(script)}" defer></script>` : ''),
    };
  },
});

site.action('setAccess', {
  input: { id: 'number', status: 'text' },
  access: ACCESS,
  run: ({ input }) => {
    const e = db.table('enrollments').where('id', Number(input.id)).first() as Enrollment | null;
    if (!e || !['active', 'removed'].includes(String(input.status))) return { message: 'not found', errors: { id: 'not found' } };
    db.table('enrollments').where('id', e.id).update({ status: input.status });
    if (input.status === 'active' && e.status !== 'active') site.emit('courses.enrolled', { courseId: e.course_id, member: e.member_id });
    return { data: { id: e.id, status: input.status } };
  },
});
