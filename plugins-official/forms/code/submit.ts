// POST /forms/{id}/submit — the Go plugin's submit handler: urlencoded / multipart / JSON bodies, honeypot,
// 5 submissions per minute per IP, validation, storage, "forms.submitted" event, mail notification, JSON or
// redirect answers.
import { addSubmission, clean, DEFAULT_SUCCESS, Form, getForm, localPath } from './model';
import { esc, failPage } from './render';

// ---------------------------------------------------------------- body parsing

const mediaType = (ct: string) => String(ct ?? '').split(';')[0].trim().toLowerCase();

function param(ct: string, name: string): string {
  const m = new RegExp(';\\s*' + name + '\\s*=\\s*("([^"]*)"|[^;\\s]+)', 'i').exec(String(ct ?? ''));
  return m ? (m[2] !== undefined ? m[2] : m[1]) : '';
}

/** application/x-www-form-urlencoded → first value per key (Go's r.PostForm); null on bad escapes. */
export function parseQuery(s: string): Record<string, string[]> | null {
  const out: Record<string, string[]> = {};
  for (const part of String(s ?? '').split('&')) {
    if (part === '') continue;
    const i = part.indexOf('=');
    const k = i >= 0 ? part.slice(0, i) : part;
    const v = i >= 0 ? part.slice(i + 1) : '';
    try {
      const key = decodeURIComponent(k.replace(/\+/g, ' '));
      (out[key] = out[key] ?? []).push(decodeURIComponent(v.replace(/\+/g, ' ')));
    } catch {
      return null;
    }
  }
  return out;
}

function parseMultipart(body: string, boundary: string): Record<string, string> | null {
  if (!boundary) return null;
  const out: Record<string, string> = {};
  const parts = body.split('--' + boundary);
  if (parts.length < 2) return null;
  for (const raw of parts.slice(1)) {
    if (raw.startsWith('--')) break;
    const p = raw.replace(/^\r?\n/, '');
    const sep = p.indexOf('\r\n\r\n');
    if (sep < 0) continue;
    const head = p.slice(0, sep);
    const cd = /content-disposition:\s*form-data([^\r\n]*)/i.exec(head);
    if (!cd) continue;
    const name = /;\s*name="([^"]*)"/i.exec(cd[1]);
    if (!name || /;\s*filename=/i.test(cd[1])) continue; // files are not form values
    const val = p.slice(sep + 4).replace(/\r\n$/, '');
    if (name[1].endsWith('[]')) {
      const k = name[1].slice(0, -2);
      out[k] = k in out && out[k] !== '' ? out[k] + '\n' + val : val; // 2.1: checkbox groups (key[])
    } else if (!(name[1] in out)) out[name[1]] = val;
  }
  return out;
}

/** Go's plugin.ToString for JSON values. */
function toString(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'string') return v;
  if (typeof v === 'boolean' || typeof v === 'number') return String(v);
  return JSON.stringify(v);
}

/** The submitted values (urlencoded, multipart or JSON), or null when the body cannot be read. */
export function readValues(request: SiteRequest): Record<string, string> | null {
  const ct = request.headers['content-type'] ?? '';
  switch (mediaType(ct)) {
    case 'application/json': {
      let raw: unknown;
      try {
        raw = JSON.parse(request.body || 'null');
      } catch {
        return null;
      }
      if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
      const out: Record<string, string> = {};
      for (const k of Object.keys(raw as object)) out[k] = toString((raw as any)[k]);
      return out;
    }
    case 'multipart/form-data':
      return parseMultipart(request.body ?? '', param(ct, 'boundary'));
    case 'application/x-www-form-urlencoded': {
      const q = parseQuery(request.body ?? '');
      if (!q) return null;
      const out: Record<string, string> = {};
      for (const k of Object.keys(q)) {
        if (k.endsWith('[]')) out[k.slice(0, -2)] = q[k].join('\n'); // 2.1: checkbox groups (key[])
        else out[k] = q[k][0];
      }
      return out;
    }
    default:
      return {}; // like Go's ParseForm: other bodies carry no form values
  }
}

export function wantsJSON(headers: Record<string, string>): boolean {
  return mediaType(headers['content-type'] ?? '') === 'application/json' || String(headers['accept'] ?? '').includes('application/json');
}

// ---------------------------------------------------------------- the page to go back to

/** Go's url.QueryEscape. */
const queryEscape = (s: string) =>
  encodeURIComponent(s).replace(/[!'()*]/g, (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase()).replace(/%20/g, '+');

const urlRe = /^(?:([a-z][a-z0-9+.-]*):)?(?:\/\/([^/?#]*))?([^?#]*)(?:\?([^#]*))?/i;

/**
 * The referring page's path (same site only) with ?sent=<id> (and #rea-form-<id>). The request host is not
 * visible to plugin routes, so "same site" means: the Referer's origin equals the Origin header when the
 * browser sent one. Only the path is used, so the redirect always stays on this site.
 */
export function backURL(headers: Record<string, string>, sent: number): string {
  let p = '/';
  let q: Record<string, string[]> = {};
  const m = urlRe.exec(String(headers['referer'] ?? ''));
  if (m && m[3]) {
    const refHost = (m[2] ?? '').toLowerCase();
    const origin = urlRe.exec(String(headers['origin'] ?? ''));
    const originHost = origin && origin[2] ? origin[2].toLowerCase() : '';
    if ((refHost === '' || originHost === '' || originHost === refHost) && m[3].startsWith('/') && !m[3].startsWith('//')) {
      p = m[3];
      q = parseQuery(m[4] ?? '') ?? {};
    }
  }
  delete q.sent;
  if (sent > 0) q.sent = [String(sent)];
  const enc = Object.keys(q).sort().map((k) => q[k].map((v) => queryEscape(k) + '=' + queryEscape(v)).join('&')).join('&');
  if (enc) p += '?' + enc;
  if (sent > 0) p += '#rea-form-' + sent;
  return p;
}

// ---------------------------------------------------------------- rate limit (real submissions only, like the built-in plugin)

const LIMIT = 5;
const WINDOW_MS = 60_000;

/** Records an attempt by ip; false when it is the 6th within a minute. */
export function allow(ip: string, now = Date.now()): boolean {
  const key = 'rl:' + ip;
  const hits = ((cache.get<number[]>(key) ?? []) as number[]).filter((t) => now - t < WINDOW_MS);
  if (hits.length >= LIMIT) {
    cache.set(key, hits, 60);
    return false;
  }
  hits.push(now);
  cache.set(key, hits, 60);
  return true;
}

const TOO_MANY = 'ส่งบ่อยเกินไป กรุณารอสักครู่แล้วลองใหม่';

// ---------------------------------------------------------------- submit

function done(fm: Form, msg: string, json: boolean, headers: Record<string, string>): RouteResult {
  // forms saved before redirects were limited to this site may still hold another site's address
  const redirect = fm.redirectUrl !== '' && localPath(fm.redirectUrl);
  if (json) {
    const out: Record<string, unknown> = { ok: true, message: msg };
    if (redirect) out.redirect = fm.redirectUrl;
    return { status: 200, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(out) };
  }
  return { status: 303, headers: { Location: redirect ? fm.redirectUrl : backURL(headers, fm.id) } };
}

export function submit(request: SiteRequest): RouteResult {
  const id = Number(/^\d+$/.test(request.params.id) ? request.params.id : 0);
  const json = wantsJSON(request.headers);
  const fail = (status: number, message: string): RouteResult =>
    json ? { status, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ok: false, message }) } : failPage(status, message, backURL(request.headers, 0));
  const fm = getForm(id);
  if (!fm) return fail(404, 'ไม่พบฟอร์มนี้');
  const input = readValues(request);
  if (!input) return fail(400, 'ข้อมูลที่ส่งมาไม่ถูกต้อง');
  const success = fm.successMessage || DEFAULT_SUCCESS;
  if ((input._hp ?? '') !== '') return done(fm, success, json, request.headers); // bot: pretend everything went fine
  if (!allow(request.ip || '?')) return fail(429, TOO_MANY);
  const { data, problem } = clean(fm, input);
  if (problem || !data) return fail(400, problem);
  const now = new Date();
  const submissionId = addSubmission(fm.id, data, now);
  const submittedAt = now.toISOString().replace(/\.\d{3}Z$/, 'Z');
  // the form's own webhook (the built-in plugin's body; any public http(s) URL)
  if (fm.webhookUrl) {
    try {
      http.webhook(fm.webhookUrl, 'form.submitted', { event: 'form.submitted', form: { id: fm.id, name: fm.name }, submissionId, data, submittedAt });
    } catch (e) {
      log.warn('forms: webhook:', String((e as Error)?.message ?? e));
    }
  }
  // → site.on('forms.submitted') and site.on('form.submitted') (code.eventAliases) in every unit — this
  // plugin's mail notification too — and the package webhook setting
  site.emit('forms.submitted', { formId: fm.id, formName: fm.name, submissionId, data, submittedAt });
  return done(fm, success, json, request.headers);
}

// ---------------------------------------------------------------- notification mail

const pad = (n: number) => (n < 10 ? '0' : '') + n;
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/** "2 Jan 2006 15:04" in the server's local time. */
function when(d: Date): string {
  return d.getDate() + ' ' + MONTHS[d.getMonth()] + ' ' + d.getFullYear() + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
}

/** Site name and address for the notification: the plugin's settings, else the site's (site.info, scope mail). */
function siteInfo(): { title: string; adminEmail: string } {
  const info = site.info();
  return {
    title: String(site.settings('siteTitle') || info.title || ''),
    adminEmail: String(site.settings('notifyEmail') || info.adminEmail || '').trim(),
  };
}

/** The notification mail of a submission (the Go plugin's sitemail.Email layout). */
export function notifyMail(fm: Form, data: Record<string, string>, at: Date, siteTitle: string): { subject: string; html: string; replyTo: string } {
  let replyTo = '';
  const rows: [string, string][] = [];
  for (const fd of fm.fields) {
    const v = data[fd.key] ?? '';
    if (fd.type === 'email' && replyTo === '') replyTo = v;
    rows.push([fd.label, v]);
  }
  const p = (s: string) => `<p style="margin:0 0 14px;font-size:15px;line-height:1.6;color:#454a5c">${esc(s)}</p>`;
  const html = `<!doctype html><html><body style="margin:0;background:#f5f6fa;font-family:-apple-system,Segoe UI,Roboto,'Noto Sans Thai',sans-serif;color:#171a26">` +
    `<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">` +
    `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:16px;border:1px solid #e6e8ef">` +
    `<tr><td style="padding:24px 32px;border-bottom:1px solid #eef0f4;font-weight:700;font-size:15px;color:#4f46e5">${esc(siteTitle)}</td></tr>` +
    `<tr><td style="padding:28px 32px"><h1 style="margin:0 0 16px;font-size:20px;line-height:1.3">${esc('มีข้อความใหม่จากฟอร์ม “' + fm.name + '”')}</h1>` +
    p('ส่งเมื่อ ' + when(at) + ' — ดูทั้งหมดได้ที่หน้าจัดการ → แบบฟอร์ม') +
    `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin:8px 0 16px;border:1px solid #eef0f4;border-radius:10px">` +
    rows.map(([k, v], i) => {
      const border = i === 0 ? '' : 'border-top:1px solid #eef0f4;';
      return `<tr><td style="${border}padding:10px 14px;width:35%;font-size:13px;color:#6b7184;vertical-align:top">${esc(k)}</td>` +
        `<td style="${border}padding:10px 14px;font-size:14px;white-space:pre-wrap;word-break:break-word">${esc(v)}</td></tr>`;
    }).join('') + `</table></td></tr>` +
    `<tr><td style="padding:16px 32px;border-top:1px solid #eef0f4;font-size:12px;color:#9aa0b0">${esc('ส่งจาก ' + siteTitle + ' · Anasaki Studio')}</td></tr>` +
    `</table></td></tr></table></body></html>`;
  const subject = (siteTitle ? '[' + siteTitle + '] ' : '') + fm.name + ': ข้อความใหม่';
  return { subject, html, replyTo };
}

/** 2.1: the form's auto-reply to the address in its first email field. */
export function autoReply(fm: Form, data: Record<string, string>): void {
  const subject = fm.autoReply?.subject ?? '';
  const to = fm.fields.filter((f) => f.type === 'email').map((f) => data[f.key] ?? '').find((v) => v !== '') ?? '';
  if (!subject || !to) return;
  const message = fm.autoReply.message ?? '';
  const html = message.split(/\n\s*\n/).map((p) => '<p>' + esc(p).replace(/\n/g, '<br>') + '</p>').join('');
  try {
    mail.send({ to: [to], subject, html, text: message });
  } catch (e) {
    log.warn('forms: auto-reply:', String((e as Error).message ?? e));
  }
}

/** site.on('forms.submitted'): emails the site's admin address (asynchronous, after the answer) and, 2.1, the auto-reply. */
export function notify(ev: { formId: number; data: Record<string, string>; submittedAt: string }): void {
  const form = getForm(Number(ev.formId));
  if (form) autoReply(form, ev.data ?? {});
  const { title, adminEmail } = siteInfo();
  if (!adminEmail) return;
  const fm = getForm(Number(ev?.formId));
  if (!fm) return;
  const m = notifyMail(fm, ev.data ?? {}, new Date(ev.submittedAt || Date.now()), title);
  try {
    mail.send({ to: [adminEmail], subject: m.subject, html: m.html, replyTo: m.replyTo || undefined });
  } catch (e) {
    const msg = String((e as Error)?.message ?? e);
    if (!/ยังไม่ได้ตั้งค่าระบบอีเมล|not configured/i.test(msg)) log.warn('forms: notify:', msg);
  }
}
