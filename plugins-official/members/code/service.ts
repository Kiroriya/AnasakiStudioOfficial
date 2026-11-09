// Member accounts: register, verify email, login / logout (cookie sessions), password reset.
// Every function runs inside a handler (route, action, visitor) of the members unit.

export interface Result {
  ok: boolean;
  message: string;
  errors?: Record<string, string>;
  data?: any;
}

const DAY = 86400_000;

const setting = (k: string, d: string) => String(site.settings(k) ?? d);
/** Visitor texts follow the site's language (Settings → General), unless the "lang" setting says otherwise. */
export const en = () => {
  const own = site.settings('lang');
  if (own === 'th' || own === 'en') return own === 'en';
  try {
    return !String(site.info().language || 'th').startsWith('th');
  } catch {
    return false;
  }
};
export const t = (th: string, e: string) => (en() ? e : th);
const fail = (message: string, errors?: Record<string, string>, data?: any): Result => ({ ok: false, message, errors, ...(data ? { data } : {}) });

export const normEmail = (s: unknown) => String(s ?? '').trim().toLowerCase();
const tokenHash = (token: string) => crypto.hash('sha256', token);

/** Visitor groups of a member row: member, verified (when the email is confirmed) + the row's groups. */
export function groupsOf(m: { verified_at?: string | null; groups?: unknown }): string[] {
  const g = ['member'];
  if (m.verified_at) g.push('verified');
  if (Array.isArray(m.groups)) for (const x of m.groups) if (typeof x === 'string' && /^[a-z][a-z0-9_-]{0,30}$/.test(x) && !g.includes(x)) g.push(x);
  return g;
}

function siteUrl(request: SiteRequest | undefined): string {
  const s = String(site.settings('siteUrl') ?? '');
  if (s) return s.replace(/\/+$/, '');
  try {
    const u = site.info().url;
    if (u) return u.replace(/\/+$/, '');
  } catch {
    /* jobs without SITE_URL */
  }
  const host = request?.headers?.host ?? 'localhost';
  const proto = (request?.headers?.['x-forwarded-proto'] ?? 'https').split(',')[0].trim();
  return `${proto === 'http' ? 'http' : 'https'}://${host}`;
}

function checkEmail(email: string): string | null {
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email) || email.length > 190) return t('อีเมลไม่ถูกต้อง', 'Invalid email');
  const allowed = setting('allowedDomains', '').split(/[\s,]+/).map((d) => d.trim().toLowerCase().replace(/^@/, '')).filter(Boolean);
  if (allowed.length && !allowed.includes(email.split('@')[1])) {
    return t('ไม่รับอีเมลโดเมนนี้', 'This email domain is not accepted');
  }
  return null;
}

function checkPassword(pw: unknown): string | null {
  const s = String(pw ?? '');
  if (s.length < 8) return t('รหัสผ่านอย่างน้อย 8 ตัวอักษร', 'At least 8 characters');
  // bcrypt reads at most 72 bytes
  if (encodeURIComponent(s).replace(/%[0-9A-F]{2}/g, 'x').length > 72) return t('รหัสผ่านยาวเกินไป', 'Password too long');
  return null;
}

/** Creates a one-time token (verify / reset) and returns the raw value (only its hash is stored). */
function issueToken(memberId: number, purpose: 'verify' | 'reset', ttlMs: number): string {
  const token = crypto.random(32);
  db.table('tokens').where({ member_id: memberId, purpose }).whereNull('used_at').delete();
  db.table('tokens').insert({ token_hash: tokenHash(token), member_id: memberId, purpose, expires_at: new Date(Date.now() + ttlMs) });
  return token;
}

/** Consumes a token: the member id, or null when unknown / used / expired. */
/** 1.4.1: whether a link can still be used (not used, not expired) — without using it. */
export function tokenUsable(token: unknown, purpose: 'verify' | 'reset'): boolean {
  const s = String(token ?? '');
  return /^[0-9a-f]{64}$/.test(s) && db.table('tokens').where({ token_hash: tokenHash(s), purpose }).whereNull('used_at').where('expires_at', '>', new Date()).exists();
}

function useToken(token: unknown, purpose: 'verify' | 'reset'): number | null {
  const s = String(token ?? '');
  if (!/^[0-9a-f]{64}$/.test(s)) return null;
  const row = db.table('tokens').where({ token_hash: tokenHash(s), purpose }).whereNull('used_at').where('expires_at', '>', new Date()).first();
  if (!row) return null;
  const n = db.table('tokens').where('id', row.id).whereNull('used_at').update({ used_at: new Date() });
  return n ? Number(row.member_id) : null;
}

/** 1.4.1: the mails look like a real service's — the site's name, a big button, the plain link as a fallback. */
function mailHtml(site_: string, heading: string, intro: string, button: string, link: string, foot: string): string {
  return `<!doctype html><html><body style="margin:0;padding:0;background:#f3f4f6">` +
    `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f3f4f6;padding:32px 12px"><tr><td align="center">` +
    `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:520px;background:#ffffff;border-radius:14px;font-family:Arial,'Noto Sans Thai',sans-serif;color:#111827">` +
    `<tr><td style="padding:28px 32px 8px;font-size:20px;font-weight:700">${esc(site_)}</td></tr>` +
    `<tr><td style="padding:8px 32px 0;font-size:18px;font-weight:700">${esc(heading)}</td></tr>` +
    `<tr><td style="padding:12px 32px 4px;font-size:15px;line-height:1.7;color:#374151">${intro}</td></tr>` +
    `<tr><td style="padding:20px 32px"><a href="${esc(link)}" style="display:inline-block;background:#3b82f6;color:#ffffff;text-decoration:none;font-weight:700;font-size:16px;padding:13px 28px;border-radius:999px">${esc(button)}</a></td></tr>` +
    `<tr><td style="padding:0 32px 8px;font-size:12px;line-height:1.6;color:#6b7280">ถ้าปุ่มกดไม่ได้ คัดลอกลิงก์นี้ไปเปิดในเบราว์เซอร์ / If the button does not work, open this link:<br><a href="${esc(link)}" style="color:#3b82f6;word-break:break-all">${esc(link)}</a></td></tr>` +
    `<tr><td style="padding:16px 32px 28px;font-size:12px;line-height:1.6;color:#9ca3af;border-top:1px solid #f3f4f6">${foot}</td></tr>` +
    `</table></td></tr></table></body></html>`;
}

function sendMail(kind: 'verify' | 'reset', member: { email: string; name: string }, link: string): boolean {
  const name = member.name;
  let siteName = '';
  try { siteName = String(site.info().title || ''); } catch { /* older cores */ }
  const tag = siteName ? `[${siteName}] ` : '';
  const msg = kind === 'verify'
    ? {
      to: member.email,
      subject: `${tag}ยืนยันอีเมลของคุณ / Confirm your email`,
      text: `สวัสดี ${name}\n\nขอบคุณที่สมัคร${siteName ? ' ' + siteName : ''} — กดลิงก์นี้เพื่อยืนยันอีเมล (ภายใน 48 ชั่วโมง):\n${link}\n\nถ้าคุณไม่ได้สมัคร ไม่ต้องทำอะไร\n\nHi ${name}, confirm your email (within 48 hours):\n${link}\nIf you did not sign up, ignore this mail.\n`,
      html: mailHtml(siteName || 'Welcome', 'ยืนยันอีเมลของคุณ / Confirm your email',
        `สวัสดี ${esc(name)}<br>ขอบคุณที่สมัครสมาชิก — กดปุ่มด้านล่างเพื่อยืนยันอีเมลและเริ่มใช้งาน (ลิงก์ใช้ได้ 48 ชั่วโมง)<br><span style="color:#6b7280">Thanks for signing up — confirm your email to get started (the link works for 48 hours).</span>`,
        'ยืนยันอีเมล / Confirm email', link, 'ถ้าคุณไม่ได้สมัคร ไม่ต้องทำอะไร บัญชีจะไม่ถูกเปิดใช้ / If you did not sign up, ignore this mail.'),
    }
    : {
      to: member.email,
      subject: `${tag}ตั้งรหัสผ่านใหม่ / Reset your password`,
      text: `สวัสดี ${name}\n\nตั้งรหัสผ่านใหม่ที่ลิงก์นี้ (ภายใน 1 ชั่วโมง):\n${link}\nถ้าคุณไม่ได้ขอ ไม่ต้องทำอะไร\n\nReset your password (within 1 hour):\n${link}\nIgnore this mail if you did not ask for it.\n`,
      html: mailHtml(siteName || 'Account', 'ตั้งรหัสผ่านใหม่ / Reset your password',
        `สวัสดี ${esc(name)}<br>มีคำขอตั้งรหัสผ่านใหม่สำหรับบัญชีนี้ — กดปุ่มด้านล่างภายใน 1 ชั่วโมง<br><span style="color:#6b7280">Someone asked to reset this account's password — use the button within an hour.</span>`,
        'ตั้งรหัสผ่านใหม่ / Reset password', link, 'ถ้าคุณไม่ได้ขอ ไม่ต้องทำอะไร รหัสผ่านเดิมยังใช้ได้ / If you did not ask for this, ignore this mail — your password stays the same.'),
    };
  // other units may restyle the mails: site.filter('members.mail', (msg, { kind, link }) => ({ ...msg, html: … }))
  const final = site.applyFilter('members.mail', msg, { kind, link });
  if (!final) return false;
  try {
    return mail.send(final).ok;
  } catch (e) {
    log.error('members: mail failed', kind, String((e as Error).message));
    return false;
  }
}

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);

// ---------------------------------------------------------------- sessions

function startSession(memberId: number, response: SiteResponse): void {
  const days = Math.min(365, Math.max(1, Number(setting('sessionDays', '30')) || 30));
  const token = crypto.random(32);
  db.table('sessions').where('expires_at', '<', new Date()).delete(); // housekeeping
  db.table('sessions').insert({ token_hash: tokenHash(token), member_id: memberId, expires_at: new Date(Date.now() + days * DAY) });
  response.setCookie('sid', token, { maxAge: days * 86400, httpOnly: true, sameSite: 'Lax' }); // → pl_members_sid
  db.table('members').where('id', memberId).update({ last_login_at: new Date() });
}

/** The member of the session cookie (active and not expired), or null. */
export function sessionMember(request: SiteRequest): any | null {
  const sid = request.cookies?.sid;
  if (!sid || !/^[0-9a-f]{64}$/.test(sid)) return null;
  const row = db.table('sessions as s')
    .join('members as m', 'm.id', 's.member_id')
    .select('m.id', 'm.email', 'm.name', 'm.status', 'm.verified_at', 'm.groups', 'm.created_at')
    .where('s.token_hash', tokenHash(sid))
    .where('s.expires_at', '>', new Date())
    .first();
  return row && row.status === 'active' ? row : null;
}

// ---------------------------------------------------------------- flows

/**
 * Whether a new member must confirm the email first (1.1.0): the setting, and only while the site can send email
 * (mail.ready, CMS 2.5.0) — on a site without SMTP nobody could ever get in. Older cores: the setting alone.
 */
export function verificationNeeded(): boolean {
  if (setting('requireVerification', 'true') === 'false') return false;
  const m = mail as unknown as { ready?: () => boolean };
  return typeof m.ready === 'function' ? !!m.ready() : true;
}

export function register(input: { email?: unknown; password?: unknown; name?: unknown }, request: SiteRequest, response?: SiteResponse): Result {
  const email = normEmail(input.email);
  const name = String(input.name ?? '').trim().slice(0, 100) || email.split('@')[0];
  const errors: Record<string, string> = {};
  const e1 = checkEmail(email);
  if (e1) errors.email = e1;
  const e2 = checkPassword(input.password);
  if (e2) errors.password = e2;
  if (Object.keys(errors).length) return fail(t('ข้อมูลไม่ถูกต้อง', 'Please check the form'), errors);
  if (db.table('members').where('email', email).exists()) {
    return fail(t('อีเมลนี้สมัครไว้แล้ว', 'This email is already registered'), { email: t('อีเมลนี้สมัครไว้แล้ว', 'Already registered') });
  }
  const id = db.table('members').insert({ email, name, password_hash: crypto.password.hash(String(input.password)) });
  if (!verificationNeeded()) { // no confirmation step: signed in right away (and never asked later, 1.4.0)
    db.table('members').where('id', id).update({ verify_waived: true });
    site.emit('members.registered', { id, email, name });
    if (response) startSession(id, response);
    return { ok: true, message: t('สมัครแล้ว ยินดีต้อนรับ', 'Registered — welcome'), data: { id, mailSent: false, signedIn: !!response } };
  }
  queueMail('verify', id, siteUrl(request));
  cache.set('members:resend:' + id, 1, 60);
  site.emit('members.registered', { id, email, name });
  return {
    ok: true,
    message: t('สมัครแล้ว กรุณายืนยันอีเมลจากลิงก์ที่ส่งไป', 'Registered — please confirm your email from the link we sent'),
    data: { id, email, mailQueued: true },
  };
}

/** 1.4.1: after the confirmation link — a session for an active member (the mailbox owner just proved it). */
export function signInVerified(memberId: number, response: SiteResponse): void {
  const m = db.table('members').where('id', memberId).first();
  if (m && m.status === 'active') { startSession(memberId, response); site.emit('members.login', { id: memberId }); }
}

export function verify(token: unknown): Result {
  const id = useToken(token, 'verify');
  if (!id) return fail(t('ลิงก์ไม่ถูกต้องหรือหมดอายุ', 'The link is invalid or expired'));
  db.table('members').where('id', id).whereNull('verified_at').update({ verified_at: new Date() });
  site.emit('members.verified', { id });
  return { ok: true, message: t('ยืนยันอีเมลแล้ว', 'Email confirmed'), data: { id } };
}

function dummyHash(): string {
  let h = cache.get<string>('members:dummy-hash');
  if (!h) {
    h = crypto.password.hash(crypto.random(16));
    cache.set('members:dummy-hash', h);
  }
  return h;
}

export function login(input: { email?: unknown; password?: unknown }, response: SiteResponse, request?: SiteRequest): Result {
  const email = normEmail(input.email);
  const key = 'members:fail:' + crypto.hash('sha256', email);
  const fails = cache.get<number>(key) ?? 0;
  if (fails >= 10) return fail(t('ลองผิดหลายครั้งเกินไป กรุณารอ 15 นาที', 'Too many attempts — try again in 15 minutes'));
  const m = email ? db.table('members').where('email', email).first() : null;
  // same work for unknown emails (no account enumeration by timing)
  const okPw = crypto.password.verify(String(input.password ?? ''), m ? m.password_hash : dummyHash());
  if (!m || !okPw) {
    cache.set(key, fails + 1, 900);
    return fail(t('อีเมลหรือรหัสผ่านไม่ถูกต้อง', 'Wrong email or password'));
  }
  if (m.status !== 'active') return fail(t('บัญชีนี้ถูกระงับ', 'This account is disabled'));
  if (verificationNeeded() && !m.verified_at && !m.verify_waived) {
    const key = 'members:resend:' + m.id;
    if (!cache.get(key)) { cache.set(key, 1, 600); queueMail('verify', Number(m.id), siteUrl(request)); }
    return fail(t(`ยังไม่ได้ยืนยันอีเมล — เราส่งลิงก์ยืนยันไปที่ ${m.email} แล้ว กดลิงก์ในอีเมลแล้วเข้าสู่ระบบอีกครั้ง (ถ้าไม่เห็น ลองดูในกล่องจดหมายขยะ)`,
      `Your email is not confirmed yet — we sent a confirmation link to ${m.email}. Open it, then sign in again (check the spam folder too).`),
      { email: t('ยังไม่ได้ยืนยันอีเมล', 'Email not confirmed') }, { unverified: true, email: m.email });
  }
  cache.delete(key);
  startSession(Number(m.id), response);
  site.emit('members.login', { id: m.id });
  return { ok: true, message: t('เข้าสู่ระบบแล้ว', 'Signed in'), data: { id: m.id, name: m.name, groups: groupsOf(m) } };
}

export function logout(request: SiteRequest, response: SiteResponse): Result {
  const sid = request.cookies?.sid;
  if (sid && /^[0-9a-f]{64}$/.test(sid)) db.table('sessions').where('token_hash', tokenHash(sid)).delete();
  response.setCookie('sid', '', { maxAge: 0 });
  return { ok: true, message: t('ออกจากระบบแล้ว', 'Signed out') };
}

/**
 * 1.4.0: confirmation and reset mails go through the job queue ("members:mail", retried with growing pauses):
 * the visitor's request answers at once instead of waiting for SMTP, and a mail server that is down for a while
 * does not lose the mail. The job makes the link itself, so no raw token is ever stored in the queue.
 */
export function queueMail(kind: 'verify' | 'reset', memberId: number, base: string): void {
  jobs.enqueue('mail', { kind, id: memberId, base });
}

/** The "members:mail" job: one mail (a fresh link each try). Throws when the server refused it, so the queue retries. */
export function deliverMail(data: { kind?: unknown; id?: unknown; base?: unknown }): void {
  const kind = data.kind === 'reset' ? 'reset' : 'verify';
  const m = db.table('members').where('id', Number(data.id)).first();
  if (!m || m.status !== 'active') return;
  if (kind === 'verify' && m.verified_at) return; // confirmed meanwhile
  const base = String(data.base ?? '').replace(/\/+$/, '');
  const link = kind === 'verify'
    ? `${base}/members/verify?token=${issueToken(Number(m.id), 'verify', 2 * DAY)}`
    : `${base}${setting('resetPath', '/members/reset')}?token=${issueToken(Number(m.id), 'reset', 3600_000)}`;
  if (!sendMail(kind, m, link) && mailReady()) throw new Error('mail not sent');
}

/** Whether the site can send mail (CMS 2.5.0 mail.ready; older cores: assume yes). */
function mailReady(): boolean {
  const m = mail as unknown as { ready?: () => boolean };
  return typeof m.ready === 'function' ? !!m.ready() : true;
}

export function forgot(input: { email?: unknown }, request: SiteRequest): Result {
  // 1.3.0: without e-mail nobody would ever get the link — say so instead of "we sent it"
  if (!mailReady()) return fail(t('เว็บนี้ยังส่งอีเมลไม่ได้ กรุณาติดต่อผู้ดูแลเว็บเพื่อตั้งรหัสผ่านใหม่', 'This site cannot send e-mail yet — please contact the site owner to reset your password'));
  const email = normEmail(input.email);
  const m = email ? db.table('members').where({ email, status: 'active' }).first() : null;
  if (m) {
    const key = 'members:reset:' + m.id;
    if (!cache.get(key)) { cache.set(key, 1, 60); queueMail('reset', Number(m.id), siteUrl(request)); }
  }
  // the same answer either way (no account enumeration)
  return { ok: true, message: t('ถ้ามีบัญชีนี้ เราได้ส่งลิงก์ตั้งรหัสผ่านใหม่ไปแล้ว', 'If the account exists, we sent a reset link') };
}

/** 1.4.0: "send the confirmation link again" — at most once a minute per account; the same answer for any address. */
export function resend(input: { email?: unknown }, request: SiteRequest): Result {
  if (!mailReady()) return fail(t('เว็บนี้ยังส่งอีเมลไม่ได้ กรุณาติดต่อผู้ดูแลเว็บ', 'This site cannot send e-mail yet — please contact the site owner'));
  const m = db.table('members').where({ email: normEmail(input.email), status: 'active' }).first();
  if (m && !m.verified_at) {
    const key = 'members:resend:' + m.id;
    if (!cache.get(key)) { cache.set(key, 1, 60); queueMail('verify', Number(m.id), siteUrl(request)); }
  }
  return { ok: true, message: t('ถ้าบัญชีนี้ยังไม่ได้ยืนยัน เราส่งลิงก์ยืนยันไปอีกครั้งแล้ว', 'If this account is not confirmed yet, we sent the link again') };
}

export function reset(input: { token?: unknown; password?: unknown; password2?: unknown }): Result {
  const e = checkPassword(input.password);
  if (e) return fail(e, { password: e });
  if (input.password2 !== undefined && String(input.password2) !== String(input.password)) {
    return fail(t('รหัสผ่านทั้งสองช่องไม่ตรงกัน', 'The two passwords differ'), { password2: t('ไม่ตรงกับช่องด้านบน', 'Does not match') });
  }
  const id = useToken(input.token, 'reset');
  if (!id) return fail(t('ลิงก์ไม่ถูกต้องหรือหมดอายุ', 'The link is invalid or expired'));
  // resetting by mail also proves the address
  db.table('members').where('id', id).update({ password_hash: crypto.password.hash(String(input.password)) });
  db.table('members').where('id', id).whereNull('verified_at').update({ verified_at: new Date() });
  db.table('sessions').where('member_id', id).delete(); // sign out everywhere
  return { ok: true, message: t('ตั้งรหัสผ่านใหม่แล้ว', 'Password changed') };
}

/** 1.3.0: a signed-in member changes the password (the current one first); every other session is signed out. */
export function changePassword(memberId: number | string, input: { current?: unknown; password?: unknown; password2?: unknown }, request: SiteRequest): Result {
  const id = Number(memberId);
  const key = 'members:pwfail:' + id;
  const fails = cache.get<number>(key) ?? 0;
  if (fails >= 10) return fail(t('ลองผิดหลายครั้งเกินไป กรุณารอ 15 นาที', 'Too many attempts — try again in 15 minutes'));
  const m = db.table('members').where('id', id).first();
  if (!m) return fail(t('ไม่พบบัญชี', 'Account not found'));
  if (!crypto.password.verify(String(input.current ?? ''), m.password_hash)) {
    cache.set(key, fails + 1, 900);
    return fail(t('รหัสผ่านปัจจุบันไม่ถูกต้อง', 'The current password is wrong'), { current: t('รหัสผ่านปัจจุบันไม่ถูกต้อง', 'Wrong password') });
  }
  const e = checkPassword(input.password);
  if (e) return fail(e, { password: e });
  if (input.password2 !== undefined && String(input.password2) !== String(input.password)) {
    return fail(t('รหัสผ่านใหม่ทั้งสองช่องไม่ตรงกัน', 'The two new passwords differ'), { password2: t('ไม่ตรงกับช่องด้านบน', 'Does not match') });
  }
  cache.delete(key);
  db.table('members').where('id', id).update({ password_hash: crypto.password.hash(String(input.password)) });
  // stay signed in here, sign out everywhere else
  const sid = request.cookies?.sid;
  const keep = sid && /^[0-9a-f]{64}$/.test(sid) ? tokenHash(sid) : '';
  db.table('sessions').where('member_id', id).where('token_hash', '!=', keep).delete();
  site.emit('members.password', { id });
  return { ok: true, message: t('เปลี่ยนรหัสผ่านแล้ว — อุปกรณ์อื่นถูกออกจากระบบ', 'Password changed — other devices were signed out') };
}

export function profile(memberId: number | string, input: { name?: unknown }): Result {
  const name = String(input.name ?? '').trim();
  if (!name || name.length > 100) return fail(t('กรุณากรอกชื่อ', 'Please enter a name'), { name: t('กรุณากรอกชื่อ (ไม่เกิน 100 ตัวอักษร)', 'Required, max 100 characters') });
  db.table('members').where('id', Number(memberId)).update({ name });
  return { ok: true, message: t('บันทึกแล้ว', 'Saved') };
}
