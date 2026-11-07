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
const fail = (message: string, errors?: Record<string, string>): Result => ({ ok: false, message, errors });

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
function useToken(token: unknown, purpose: 'verify' | 'reset'): number | null {
  const s = String(token ?? '');
  if (!/^[0-9a-f]{64}$/.test(s)) return null;
  const row = db.table('tokens').where({ token_hash: tokenHash(s), purpose }).whereNull('used_at').where('expires_at', '>', new Date()).first();
  if (!row) return null;
  const n = db.table('tokens').where('id', row.id).whereNull('used_at').update({ used_at: new Date() });
  return n ? Number(row.member_id) : null;
}

function sendMail(kind: 'verify' | 'reset', member: { email: string; name: string }, link: string): boolean {
  const name = member.name;
  const msg = kind === 'verify'
    ? {
      to: member.email,
      subject: 'ยืนยันอีเมลของคุณ / Confirm your email',
      text: `สวัสดี ${name}\n\nกดลิงก์นี้เพื่อยืนยันอีเมล (ภายใน 48 ชั่วโมง):\n${link}\n\nHi ${name}, confirm your email (within 48 hours):\n${link}\n`,
      html: `<p>สวัสดี ${esc(name)}</p><p>กดลิงก์นี้เพื่อยืนยันอีเมล (ภายใน 48 ชั่วโมง) / Confirm your email (within 48 hours):</p><p><a href="${esc(link)}">${esc(link)}</a></p>`,
    }
    : {
      to: member.email,
      subject: 'ตั้งรหัสผ่านใหม่ / Reset your password',
      text: `สวัสดี ${name}\n\nตั้งรหัสผ่านใหม่ที่ลิงก์นี้ (ภายใน 1 ชั่วโมง):\n${link}\nถ้าคุณไม่ได้ขอ ไม่ต้องทำอะไร\n\nReset your password (within 1 hour):\n${link}\nIgnore this mail if you did not ask for it.\n`,
      html: `<p>สวัสดี ${esc(name)}</p><p>ตั้งรหัสผ่านใหม่ (ภายใน 1 ชั่วโมง) / Reset your password (within 1 hour):</p><p><a href="${esc(link)}">${esc(link)}</a></p><p>ถ้าคุณไม่ได้ขอ ไม่ต้องทำอะไร / Ignore this mail if you did not ask for it.</p>`,
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
  if (!verificationNeeded()) { // no confirmation step: signed in right away
    site.emit('members.registered', { id, email, name });
    if (response) startSession(id, response);
    return { ok: true, message: t('สมัครแล้ว ยินดีต้อนรับ', 'Registered — welcome'), data: { id, mailSent: false, signedIn: !!response } };
  }
  const token = issueToken(id, 'verify', 2 * DAY);
  const sent = sendMail('verify', { email, name }, `${siteUrl(request)}/members/verify?token=${token}`);
  site.emit('members.registered', { id, email, name });
  return {
    ok: true,
    message: t('สมัครแล้ว กรุณายืนยันอีเมลจากลิงก์ที่ส่งไป', 'Registered — please confirm your email from the link we sent'),
    data: { id, mailSent: sent },
  };
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

export function login(input: { email?: unknown; password?: unknown }, response: SiteResponse): Result {
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
  if (verificationNeeded() && !m.verified_at) {
    return fail(t('กรุณายืนยันอีเมลก่อนเข้าสู่ระบบ', 'Please confirm your email first'), { email: t('ยังไม่ได้ยืนยันอีเมล', 'Email not confirmed') });
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

export function forgot(input: { email?: unknown }, request: SiteRequest): Result {
  const email = normEmail(input.email);
  const m = email ? db.table('members').where({ email, status: 'active' }).first() : null;
  if (m) {
    const token = issueToken(Number(m.id), 'reset', 3600_000);
    sendMail('reset', m, `${siteUrl(request)}${setting('resetPath', '/members/reset')}?token=${token}`);
  }
  // the same answer either way (no account enumeration)
  return { ok: true, message: t('ถ้ามีบัญชีนี้ เราได้ส่งลิงก์ตั้งรหัสผ่านใหม่ไปแล้ว', 'If the account exists, we sent a reset link') };
}

export function reset(input: { token?: unknown; password?: unknown }): Result {
  const e = checkPassword(input.password);
  if (e) return fail(e, { password: e });
  const id = useToken(input.token, 'reset');
  if (!id) return fail(t('ลิงก์ไม่ถูกต้องหรือหมดอายุ', 'The link is invalid or expired'));
  // resetting by mail also proves the address
  db.table('members').where('id', id).update({ password_hash: crypto.password.hash(String(input.password)) });
  db.table('members').where('id', id).whereNull('verified_at').update({ verified_at: new Date() });
  db.table('sessions').where('member_id', id).delete(); // sign out everywhere
  return { ok: true, message: t('ตั้งรหัสผ่านใหม่แล้ว', 'Password changed') };
}

export function profile(memberId: number | string, input: { name?: unknown }): Result {
  const name = String(input.name ?? '').trim();
  if (!name || name.length > 100) return fail(t('กรุณากรอกชื่อ', 'Please enter a name'), { name: t('กรุณากรอกชื่อ (ไม่เกิน 100 ตัวอักษร)', 'Required, max 100 characters') });
  db.table('members').where('id', Number(memberId)).update({ name });
  return { ok: true, message: t('บันทึกแล้ว', 'Saved') };
}
