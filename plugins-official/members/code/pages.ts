// Ready-made member pages in the site's layout: /members/login, /members/register, /members/forgot,
// /members/reset?token=, /members/account. Their forms post JSON to the /members routes through
// assets/members.js (JSON-only POSTs are the CSRF protection), so they need JavaScript.
import { groupsOf, t } from './service';

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);

const SCRIPT = (() => {
  try {
    return site.asset('assets/members.js').url;
  } catch {
    return '';
  }
})();

export const CSS = `.mb-page{max-width:440px;margin:56px auto;padding:0 20px}
.mb-page h1{font-size:28px;font-weight:700;margin:0 0 8px}.mb-page .mb-sub{color:#6b7280;margin:0 0 20px}
.mb-form{display:flex;flex-direction:column;gap:14px}
.mb-form label{display:flex;flex-direction:column;gap:6px;font-weight:600;font-size:.92em}
.mb-form input{font:inherit;font-weight:400;padding:11px 12px;border:1px solid #d1d5db;border-radius:10px;width:100%;box-sizing:border-box}
.mb-form input:focus{outline:2px solid var(--c-primary,#4f46e5);outline-offset:1px}
.mb-form .is-invalid input{border-color:#dc2626}.mb-err{color:#dc2626;font-size:.85em;font-weight:400}
.mb-btn{min-height:46px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer}
.mb-btn[disabled]{opacity:.6}.mb-btn-ghost{background:#fff;color:inherit;border:1px solid #d1d5db}
.mb-links{display:flex;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-top:16px;font-size:.92em}
.mb-links a{color:var(--c-primary,#4f46e5)}
.mb-msg{padding:12px 14px;border-radius:10px;margin:0 0 14px}.mb-ok{background:#dcfce7;color:#166534}.mb-bad{background:#fee2e2;color:#991b1b}
.mb-card{border:1px solid #e5e7eb;border-radius:14px;padding:18px;margin:0 0 16px}
.mb-badge{display:inline-block;padding:2px 10px;border-radius:999px;font-size:.8em;font-weight:600;background:#eef0ff;color:#4338ca;margin-right:4px}
.mb-menu{display:inline-flex;align-items:center;gap:10px}.mb-menu a{color:inherit;text-decoration:none;font-weight:600}
.mb-menu .mb-join{padding:6px 14px;border-radius:999px;background:var(--c-primary,#4f46e5);color:#fff}`;

const page = (title: string, html: string, status = 200): RouteResult => ({
  page: { title, html: `<style>${CSS}</style><div class="mb-page">${html}</div>${SCRIPT ? `<script defer src="${esc(SCRIPT)}"></script>` : ''}` },
  status,
  headers: { 'Cache-Control': 'no-store' },
});

const field = (name: string, label: string, type = 'text', extra = '') =>
  `<div class="mb-field" data-field="${name}"><label>${esc(label)}<input type="${type}" name="${name}"${extra}></label><span class="mb-err" hidden></span></div>`;

const safeNext = (v: unknown) => {
  const s = String(Array.isArray(v) ? v[0] : v ?? '');
  return /^\/(?!\/)[^\s\\]*$/.test(s) ? s : '';
};

function afterLogin(): string {
  return safeNext(site.settings('afterLoginUrl')) || '/members/account';
}

export function loginPage(query: Record<string, unknown>): RouteResult {
  const next = safeNext(query.next) || afterLogin();
  const note = query.verified ? `<p class="mb-msg mb-ok">${esc(t('ยืนยันอีเมลแล้ว เข้าสู่ระบบได้เลย', 'Email confirmed — you can sign in'))}</p>`
    : query.reset ? `<p class="mb-msg mb-ok">${esc(t('ตั้งรหัสผ่านใหม่แล้ว', 'Password changed'))}</p>` : '';
  return page(t('เข้าสู่ระบบ', 'Sign in'), `<h1>${esc(t('เข้าสู่ระบบ', 'Sign in'))}</h1>${note}` +
    `<form class="mb-form" data-members="login" data-next="${esc(next)}" data-resend="${esc(t('ส่งลิงก์ยืนยันอีกครั้ง', 'Send the confirmation link again'))}" novalidate><p class="mb-msg" hidden role="status"></p>` +
    field('email', t('อีเมล', 'Email'), 'email', ' required autocomplete="email"') +
    field('password', t('รหัสผ่าน', 'Password'), 'password', ' required autocomplete="current-password"') +
    `<button class="mb-btn" type="submit">${esc(t('เข้าสู่ระบบ', 'Sign in'))}</button></form>` +
    `<div class="mb-links"><a href="/members/register">${esc(t('สมัครสมาชิก', 'Create an account'))}</a><a href="/members/forgot">${esc(t('ลืมรหัสผ่าน?', 'Forgot password?'))}</a></div>`);
}

export function registerPage(): RouteResult {
  return page(t('สมัครสมาชิก', 'Create an account'), `<h1>${esc(t('สมัครสมาชิก', 'Create an account'))}</h1>` +
    `<form class="mb-form" data-members="register" data-done="${esc(t('เข้าสู่ระบบ', 'Sign in'))}"` +
    ` data-sent-title="${esc(t('ตรวจอีเมลของคุณ', 'Check your e-mail'))}"` +
    ` data-sent-text="${esc(t('เราส่งลิงก์ยืนยันไปที่ {email} แล้ว กดลิงก์ในอีเมลเพื่อเริ่มใช้งาน — ถ้าไม่เห็นภายในไม่กี่นาที ลองดูในกล่องจดหมายขยะ', 'We sent a confirmation link to {email}. Open it to start — if it is not there in a few minutes, check the spam folder.'))}"` +
    ` data-resend="${esc(t('ส่งลิงก์อีกครั้ง', 'Send the link again'))}" data-wait="${esc(t('ส่งอีกครั้งได้ใน {s} วินาที', 'Send again in {s} s'))}" novalidate><p class="mb-msg" hidden role="status"></p>` +
    field('name', t('ชื่อ', 'Name'), 'text', ' maxlength="100" autocomplete="name"') +
    field('email', t('อีเมล', 'Email'), 'email', ' required autocomplete="email"') +
    field('password', t('รหัสผ่าน (อย่างน้อย 8 ตัว)', 'Password (8+ characters)'), 'password', ' required minlength="8" autocomplete="new-password"') +
    `<button class="mb-btn" type="submit">${esc(t('สมัครสมาชิก', 'Create account'))}</button></form>` +
    `<div class="mb-links"><span>${esc(t('มีบัญชีแล้ว?', 'Have an account?'))} <a href="/members/login">${esc(t('เข้าสู่ระบบ', 'Sign in'))}</a></span></div>`);
}

export function forgotPage(): RouteResult {
  return page(t('ลืมรหัสผ่าน', 'Forgot password'), `<h1>${esc(t('ลืมรหัสผ่าน', 'Forgot password'))}</h1>` +
    `<p class="mb-sub">${esc(t('ใส่อีเมลของบัญชี เราจะส่งลิงก์ตั้งรหัสผ่านใหม่ให้', 'Enter your account email and we will send a reset link'))}</p>` +
    `<form class="mb-form" data-members="forgot" novalidate><p class="mb-msg" hidden role="status"></p>` +
    field('email', t('อีเมล', 'Email'), 'email', ' required autocomplete="email"') +
    `<button class="mb-btn" type="submit">${esc(t('ส่งลิงก์', 'Send link'))}</button></form>` +
    `<div class="mb-links"><a href="/members/login">← ${esc(t('เข้าสู่ระบบ', 'Sign in'))}</a></div>`);
}

export function resetPage(query: Record<string, unknown>, usable = true): RouteResult {
  const token = String(Array.isArray(query.token) ? query.token[0] : query.token ?? '');
  // 1.4.1: a used or expired link says so at once (with the way to a new one), instead of after typing a password
  if (!usable) {
    return page(t('ลิงก์ใช้ไม่ได้แล้ว', 'This link no longer works'), `<h1>${esc(t('ลิงก์ใช้ไม่ได้แล้ว', 'This link no longer works'))}</h1>` +
      `<p class="mb-msg mb-bad">${esc(t('ลิงก์ตั้งรหัสผ่านใหม่ใช้ได้ครั้งเดียวภายใน 1 ชั่วโมง — ลิงก์นี้ถูกใช้ไปแล้วหรือหมดอายุ', 'A reset link works once, within an hour — this one was used or has expired'))}</p>` +
      `<p><a class="mb-btn" style="display:inline-flex;align-items:center;padding:0 20px;text-decoration:none" href="/members/forgot">${esc(t('ขอลิงก์ใหม่', 'Get a new link'))}</a></p>`, 400);
  }
  return page(t('ตั้งรหัสผ่านใหม่', 'New password'), `<h1>${esc(t('ตั้งรหัสผ่านใหม่', 'New password'))}</h1>` +
    `<form class="mb-form" data-members="reset" data-next="/members/login?reset=1" novalidate><p class="mb-msg" hidden role="status"></p>` +
    `<input type="hidden" name="token" value="${esc(/^[0-9a-f]{64}$/.test(token) ? token : '')}">` +
    field('password', t('รหัสผ่านใหม่ (อย่างน้อย 8 ตัว)', 'New password (8+ characters)'), 'password', ' required minlength="8" autocomplete="new-password"') +
    field('password2', t('ยืนยันรหัสผ่านใหม่', 'New password again'), 'password', ' required minlength="8" autocomplete="new-password"') +
    `<button class="mb-btn" type="submit">${esc(t('บันทึก', 'Save'))}</button></form>`);
}

export function verifyPage(ok: boolean, message: string): RouteResult {
  return page(ok ? t('ยืนยันอีเมลแล้ว', 'Email confirmed') : t('ยืนยันอีเมลไม่สำเร็จ', 'Could not confirm'),
    `<p class="mb-msg ${ok ? 'mb-ok' : 'mb-bad'}">${esc(message)}</p><p><a class="mb-btn" style="display:inline-flex;align-items:center;padding:0 20px;text-decoration:none" href="/members/login${ok ? '?verified=1' : ''}">${esc(t('เข้าสู่ระบบ', 'Sign in'))}</a></p>`,
    ok ? 200 : 400);
}

export function accountPage(m: { name: string; email: string; verified_at?: string | null; groups?: unknown; created_at: string }, query: Record<string, unknown> = {}): RouteResult {
  const groups = groupsOf(m).map((g) => `<span class="mb-badge">${esc(g)}</span>`).join('');
  // 1.3.0: ?next=/somewhere (an app that sent the member here) shows a way back
  const back = safeNext(query.next);
  return page(t('บัญชีของฉัน', 'My account'), (back ? `<p style="margin:0 0 12px"><a class="mb-back" href="${esc(back)}">← ${esc(t('กลับ', 'Back'))}</a></p>` : '') +
    `<h1>${esc(t('บัญชีของฉัน', 'My account'))}</h1>` +
    `<div class="mb-card"><p style="margin:0 0 6px"><b>${esc(m.email)}</b> ${m.verified_at ? '✓' : `<span class="mb-err">${esc(t('ยังไม่ได้ยืนยันอีเมล', 'Email not confirmed'))}</span>`}</p>` +
    `<p style="margin:0">${groups}</p><p class="mb-sub" style="margin:8px 0 0">${esc(t('สมาชิกตั้งแต่', 'Member since'))} ${esc(String(m.created_at).slice(0, 10))}</p></div>` +
    `<form class="mb-form" data-members="profile" novalidate><p class="mb-msg" hidden role="status"></p>` +
    field('name', t('ชื่อที่แสดง', 'Display name'), 'text', ` value="${esc(m.name)}" maxlength="100" required`) +
    `<button class="mb-btn" type="submit">${esc(t('บันทึก', 'Save'))}</button></form>` +
    `<h2 style="margin:28px 0 8px;font-size:1.2rem;font-weight:700">${esc(t('เปลี่ยนรหัสผ่าน', 'Change password'))}</h2>` +
    `<form class="mb-form" data-members="password" novalidate><p class="mb-msg" hidden role="status"></p>` +
    field('current', t('รหัสผ่านปัจจุบัน', 'Current password'), 'password', ' required autocomplete="current-password"') +
    field('password', t('รหัสผ่านใหม่ (อย่างน้อย 8 ตัว)', 'New password (8+ characters)'), 'password', ' required minlength="8" autocomplete="new-password"') +
    field('password2', t('ยืนยันรหัสผ่านใหม่', 'New password again'), 'password', ' required minlength="8" autocomplete="new-password"') +
    `<button class="mb-btn" type="submit">${esc(t('เปลี่ยนรหัสผ่าน', 'Change password'))}</button>` +
    `<p class="mb-sub" style="margin:8px 0 0">${esc(t('จำรหัสผ่านปัจจุบันไม่ได้?', 'Forgot the current one?'))} <a href="/members/forgot">${esc(t('ตั้งรหัสผ่านใหม่ทางอีเมล', 'Reset it by e-mail'))}</a></p></form>` +
    `<form class="mb-form" data-members="logout" data-next="/" style="margin-top:14px"><button class="mb-btn mb-btn-ghost" type="submit">${esc(t('ออกจากระบบ', 'Sign out'))}</button></form>`);
}

export const toLogin = (next: string): RouteResult => ({ status: 303, headers: { Location: '/members/login?next=' + encodeURIComponent(next), 'Cache-Control': 'no-store' } });

/** The "member menu" designer block: filled in the browser from GET /members/me (blocks do not know the visitor). */
export function registerMenuBlock(): void {
  site.block('menu', {
    label: { th: 'เมนูสมาชิก', en: 'Member menu' },
    icon: '👤',
    script: 'assets/members.js',
    cache: 3600,
    render: () => ({
      html: `<span class="mb-menu" data-members-menu data-account="${esc(t('บัญชีของฉัน', 'My account'))}">` +
        `<a href="/members/login">${esc(t('เข้าสู่ระบบ', 'Sign in'))}</a><a class="mb-join" href="/members/register">${esc(t('สมัครสมาชิก', 'Join'))}</a></span>`,
      css: CSS,
    }),
  });
}
