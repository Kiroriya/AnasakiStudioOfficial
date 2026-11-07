// members: visitor accounts for the public site. Own tables (members, sessions, tokens), JSON routes
// under /members, the same flows as site.action for designer forms, the site.visitor resolver (groups
// "member" and "verified"), a "my profile" collection and an admin screen to disable members.
import { register, verify, login, logout, forgot, reset, profile, sessionMember, groupsOf, normEmail, Result, verificationNeeded } from './service';
import { csvOf, csvDisposition } from './csv';
import { accountPage, forgotPage, loginPage, registerMenuBlock, registerPage, resetPage, toLogin, verifyPage } from './pages';

// ---------------------------------------------------------------- who is visiting

// There is at most ONE site.visitor per site (all units together). Installing members next to another
// plugin / Site Code that also calls site.visitor fails activation of the later one.
site.visitor(({ request }) => {
  const m = sessionMember(request);
  return m ? { id: Number(m.id), name: m.name, email: m.email, verified: !!m.verified_at, groups: groupsOf(m) } : null;
});
site.visitorGroups(['member', 'verified']);

// ---------------------------------------------------------------- JSON routes

/** POST routes accept JSON only: a cross-site HTML form cannot send application/json (CSRF). */
function body(request: SiteRequest): Record<string, unknown> | null {
  if (!String(request.headers['content-type'] ?? '').toLowerCase().startsWith('application/json')) return null;
  try {
    const b = request.json();
    return b && typeof b === 'object' ? b : {};
  } catch {
    return {};
  }
}
const answer = (r: Result) => (r.ok ? r : { status: r.errors ? 400 : 401, body: r });
const needJson = { status: 415, body: { ok: false, message: 'Content-Type must be application/json' } };
const PUBLIC = { access: ['*'] };

site.route('POST /members/register', PUBLIC, ({ request, response }) => {
  const b = body(request);
  return b ? answer(register(b, request, response)) : needJson;
});

// The link in the confirmation mail: a page in the site's layout, or a redirect to the "afterVerifyUrl" setting.
site.route('GET /members/verify', PUBLIC, ({ request }) => {
  const token = Array.isArray(request.query.token) ? request.query.token[0] : request.query.token;
  const r = verify(token);
  const next = String(site.settings('afterVerifyUrl') ?? '');
  if (r.ok && next && next.startsWith('/') && !next.startsWith('//')) {
    return { status: 303, headers: { Location: next + (next.includes('?') ? '&' : '?') + 'verified=1' } };
  }
  return verifyPage(r.ok, r.message);
});

// ---------------------------------------------------------------- ready-made pages

site.route('GET /members/login', PUBLIC, ({ request, visitor }) =>
  visitor ? { status: 303, headers: { Location: '/members/account' } } : loginPage(request.query));
site.route('GET /members/register', PUBLIC, ({ visitor }) =>
  visitor ? { status: 303, headers: { Location: '/members/account' } } : registerPage());
site.route('GET /members/forgot', PUBLIC, () => forgotPage());
site.route('GET /members/reset', PUBLIC, ({ request }) => resetPage(request.query));
site.route('GET /members/account', PUBLIC, ({ visitor }) => {
  if (!visitor) return toLogin('/members/account');
  const m = db.table('members').where('id', Number(visitor.id)).first();
  return m ? accountPage(m) : toLogin('/members/account');
});
site.route('POST /members/profile', PUBLIC, ({ request, visitor }) => {
  const b = body(request);
  if (!b) return needJson;
  if (!visitor) return { status: 401, body: { ok: false, message: 'Sign in first' } };
  return answer(profile(visitor.id, b));
});

site.route('POST /members/login', PUBLIC, ({ request, response }) => {
  const b = body(request);
  return b ? answer(login(b, response)) : needJson;
});

site.route('POST /members/logout', PUBLIC, ({ request, response }) => (body(request) ? logout(request, response) : needJson));

site.route('POST /members/forgot', PUBLIC, ({ request }) => {
  const b = body(request);
  return b ? answer(forgot(b, request)) : needJson;
});

site.route('POST /members/reset', PUBLIC, ({ request }) => {
  const b = body(request);
  return b ? answer(reset(b)) : needJson;
});

site.route('GET /members/me', PUBLIC, ({ visitor }) => ({ status: 200, headers: { 'Cache-Control': 'no-store' }, body: { visitor } }));

registerMenuBlock();

// ---------------------------------------------------------------- actions (designer forms / window.anasaki.call)

const act = (r: Result) => (r.ok ? { message: r.message, data: r.data } : { message: r.message, errors: r.errors ?? { form: r.message } });

site.action('register', {
  input: { email: 'email', password: 'text', name: 'text?' },
  access: ['*'],
  run: ({ input, request, response }) => act(register(input, request, response)),
});
site.action('login', {
  input: { email: 'email', password: 'text' },
  access: ['*'],
  run: ({ input, response }) => {
    const r = login(input, response);
    const next = String(site.settings('afterLoginUrl') ?? '');
    return r.ok && next && next.startsWith('/') && !next.startsWith('//') ? { ...act(r), redirect: next } : act(r);
  },
});
site.action('logout', { access: ['*'], run: ({ request, response }) => ({ ...act(logout(request, response)), redirect: '/' }) });
site.action('forgot', { input: { email: 'email' }, access: ['*'], run: ({ input, request }) => act(forgot(input, request)) });
site.action('reset', { input: { token: 'text', password: 'text' }, access: ['*'], run: ({ input }) => act(reset(input)) });
site.action('profile', {
  input: { name: 'text' },
  access: ['group:member'],
  run: ({ input, visitor }) => ({ ...act(profile(visitor!.id, input)), refresh: ['me'] }),
});

// ---------------------------------------------------------------- "my profile" for the designer (code:members:me)

site.collection('me', {
  label: { th: 'ข้อมูลของฉัน', en: 'My profile' },
  fields: { name: 'text', email: 'text', verified: 'bool', groups: 'text', createdAt: 'datetime' },
  access: ['group:member'],
  load: ({ visitor }) => {
    if (!visitor) return [];
    const m = db.table('members').where('id', Number(visitor.id)).first();
    return m ? [{ name: m.name, email: m.email, verified: !!m.verified_at, groups: groupsOf(m).join(', '), createdAt: m.created_at }] : [];
  },
});

// ---------------------------------------------------------------- admin

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);

const one = (v: unknown) => String((Array.isArray(v) ? v[0] : v) ?? '');
/** The admin page's language: ?lang=en|th, else ctx.locale (signed-in user's admin language, phase 6). */
const adminEn = (ctx: { query?: Record<string, unknown>; locale?: string }) => {
  const q = one(ctx.query?.lang);
  return q === 'en' || q === 'th' ? q === 'en' : ctx.locale === 'en';
};
const MEMBER_COLS = ['id', 'email', 'name', 'status', 'verified_at', 'last_login_at', 'created_at', 'groups'];

/** Members matching the search box (email or name). */
function memberQuery(q: string) {
  let b = db.table('members').select(...MEMBER_COLS);
  if (q) {
    const like = '%' + q.replace(/[\\%_]/g, (c) => '\\' + c) + '%';
    b = b.where((w) => w.where('email', 'like', like).orWhere('name', 'like', like));
  }
  return b.orderByDesc('id');
}

// 1.1.0: confirmation by email is on but the site cannot send email yet — say so (new members are let in
// without the confirmation until Settings → Email is set up).
if (typeof site.adminNotice === 'function') {
  site.adminNotice(() => {
    if (String(site.settings('requireVerification') ?? 'true') === 'false' || verificationNeeded()) return null;
    return { level: 'warning',
      text: { th: 'สมาชิก: เว็บยังส่งอีเมลไม่ได้ สมาชิกใหม่จึงเข้าสู่ระบบได้โดยไม่ต้องยืนยันอีเมล — ตั้งค่าอีเมลเพื่อเปิดการยืนยัน',
        en: 'Members: the site cannot send email yet, so new members sign in without confirming their email — set up email to turn the confirmation on' },
      link: { href: '/admin/settings?tab=email', label: { th: 'ตั้งค่าอีเมล', en: 'Email settings' } } };
  }, { access: ['administrator'] });
}

site.adminPage('list', {
  label: { th: 'สมาชิก', en: 'Members' },
  icon: '👥',
  render: (ctx) => {
    const { query } = ctx;
    const en = adminEn(ctx);
    const t = (th: string, e: string) => (en ? e : th);
    const q = one(query.q).trim().slice(0, 100);
    const page = Math.max(1, Number(one(query.page)) || 1);
    const pg = memberQuery(q).paginate(page, 25);
    const pages = Math.max(1, Math.ceil(pg.total / 25));
    const ql = one(query.lang);
    const keep = ql === 'en' || ql === 'th' ? `&lang=${ql}` : '';
    const rows = pg.items.map((m) => `<tr data-id="${m.id}"><td>${m.id}</td><td>${esc(m.email)}</td><td>${esc(m.name)}</td>` +
      `<td>${m.verified_at ? '<span class="ak-badge ak-badge-ok">✓</span>' : '—'}</td><td class="ak-hint">${esc(String(m.last_login_at ?? '').slice(0, 16).replace('T', ' '))}</td>` +
      `<td>${m.status === 'active' ? `<span class="ak-badge ak-badge-info">${t('ใช้งาน', 'Active')}</span>` : `<span class="ak-badge ak-badge-danger">${t('ระงับ', 'Disabled')}</span>`}</td>` +
      `<td><button class="ak-btn ak-btn-sm${m.status === 'active' ? ' ak-btn-danger' : ''}" data-member-status="${m.status === 'active' ? 'disabled' : 'active'}" data-id="${m.id}">` +
      `${m.status === 'active' ? t('ระงับ', 'Disable') : t('เปิดใช้', 'Enable')}</button></td></tr>`).join('');
    let script = '';
    try {
      script = site.asset('assets/admin.js').url;
    } catch {
      /* none */
    }
    return {
      title: t('สมาชิก', 'Members'),
      html: `<header class="ak-page-head"><span class="ak-page-icon">👥</span><div class="ak-page-head-text"><h1>${t('สมาชิก', 'Members')}</h1>` +
        `<p>${t('บัญชีผู้เยี่ยมชมเว็บ (แยกจากผู้ใช้หลังบ้าน) — หน้า /members/login /members/register /members/account', 'Visitor accounts (separate from admin users) — pages /members/login, /members/register, /members/account')}</p></div></header>` +
        `<form method="get" class="ak-actions" style="margin-bottom:12px">${keep ? `<input type="hidden" name="lang" value="${ql}">` : ''}` +
        `<input type="search" name="q" value="${esc(q)}" placeholder="${t('อีเมลหรือชื่อ', 'email or name')}" style="max-width:320px"> <button class="ak-btn">${t('ค้นหา', 'Search')}</button>` +
        `<span style="flex:1"></span><span class="ak-hint">${t('ทั้งหมด', 'Total')} ${pg.total}</span>` +
        `<button type="button" class="ak-btn" data-members="export" data-q="${esc(q)}">⬇ ${t('ส่งออก CSV', 'Export CSV')}</button></form>` +
        (pg.total ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" data-members="list"><thead><tr><th>#</th><th>${t('อีเมล', 'Email')}</th><th>${t('ชื่อ', 'Name')}</th><th>${t('ยืนยันแล้ว', 'Verified')}</th>` +
        `<th>${t('ล็อกอินล่าสุด', 'Last login')}</th><th>${t('สถานะ', 'Status')}</th><th></th></tr></thead><tbody>${rows}</tbody></table></div></div>`
          : `<div class="ak-card"><p class="ak-muted">${t('ยังไม่มีสมาชิก', 'No members yet')}</p></div>`) +
        (pages > 1 ? `<div class="ak-actions" style="margin-top:12px">${page > 1 ? `<a class="ak-btn ak-btn-sm" href="?q=${encodeURIComponent(q)}&page=${page - 1}${keep}">‹</a>` : ''}<span class="ak-hint">${page} / ${pages}</span>${page < pages ? `<a class="ak-btn ak-btn-sm" href="?q=${encodeURIComponent(q)}&page=${page + 1}${keep}">›</a>` : ''}</div>` : '') +
        (script ? `<script src="${esc(script)}" defer></script>` : ''),
    };
  },
  // "Export CSV": GET …/download?q= (same ticket + role check as the page). Password hashes and
  // tokens are never exported.
  download: (ctx) => {
    const en = adminEn(ctx);
    const t = (th: string, e: string) => (en ? e : th);
    const rows = memberQuery(one(ctx.query.q).trim().slice(0, 100)).limit(10000).get()
      .map((m) => ({ ...m, verified: m.verified_at ? 'yes' : 'no', groups: groupsOf(m).join(' ') }));
    const cols = [
      { key: 'id', label: '#' }, { key: 'email', label: t('อีเมล', 'Email') }, { key: 'name', label: t('ชื่อ', 'Name') },
      { key: 'status', label: t('สถานะ', 'Status') }, { key: 'verified', label: t('ยืนยันแล้ว', 'Verified') },
      { key: 'groups', label: t('กลุ่ม', 'Groups') }, { key: 'last_login_at', label: t('ล็อกอินล่าสุด', 'Last login') },
      { key: 'created_at', label: t('สมัครเมื่อ', 'Registered') },
    ];
    return {
      status: 200,
      headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': csvDisposition('members.csv'), 'Cache-Control': 'no-store' },
      body: csvOf(rows, cols),
    };
  },
});

site.action('setStatus', {
  input: { id: 'number', status: 'text' },
  // default access: administrator (change it on the Access screen)
  run: ({ input }) => {
    if (input.status !== 'active' && input.status !== 'disabled') return { errors: { status: 'active | disabled' } };
    const n = db.table('members').where('id', input.id).update({ status: input.status });
    if (input.status === 'disabled') db.table('sessions').where('member_id', input.id).delete();
    return { message: n ? 'บันทึกแล้ว / Saved' : 'ไม่พบ / Not found', data: { changed: n } };
  },
});

// Other units (with members in "requires") may look members up: import { memberByEmail } from 'members'
site.export('memberByEmail', (email: string) => {
  const m = db.table('members').where('email', normEmail(email)).first();
  return m ? { id: m.id, email: m.email, name: m.name, status: m.status, verified: !!m.verified_at, groups: groupsOf(m) } : null;
});
