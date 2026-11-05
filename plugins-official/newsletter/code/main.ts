// newsletter 1.0 — a sign-up form (designer block, also /newsletter/subscribe for any form), double opt-in, a
// one-click unsubscribe link in every mail, and campaigns: plain-text mails sent by a background job in batches,
// paced by the "perHour" setting so the site's shared mail quota is never used up.
import { Subscriber, byToken, confirm, esc, getCampaign, normEmail, unsubscribe, upsert, validEmail } from './model';
import { mailFor, siteTitle, siteURL, t } from './text';
import { registerAdmin } from './admin';

const page = (title: string, html: string, status = 200): RouteResult => ({
  page: { title, html: `<div style="max-width:520px;margin:72px auto;padding:0 20px;text-align:center"><h1 style="font-size:26px">${esc(title)}</h1>${html}` +
    `<p style="margin-top:24px"><a href="/">${esc(t('home'))}</a></p></div>` },
  status,
  headers: { 'Cache-Control': 'no-store', 'X-Robots-Tag': 'noindex' },
});

// ---------------------------------------------------------------- sign-up

function bodyOf(req: SiteRequest): Record<string, string> {
  if (String(req.headers['content-type'] ?? '').includes('application/json')) {
    try {
      const o = req.json<Record<string, unknown>>() ?? {};
      return Object.fromEntries(Object.entries(o).map(([k, v]) => [k, String(v ?? '')]));
    } catch {
      return {};
    }
  }
  const out: Record<string, string> = {};
  for (const part of String(req.body ?? '').split('&')) {
    if (!part) continue;
    const i = part.indexOf('=');
    const dec = (s: string) => {
      try {
        return decodeURIComponent(s.replace(/\+/g, ' '));
      } catch {
        return '';
      }
    };
    out[dec(i < 0 ? part : part.slice(0, i))] = dec(i < 0 ? '' : part.slice(i + 1));
  }
  return out;
}

function sendConfirm(s: Subscriber): void {
  const link = `${siteURL()}/newsletter/confirm?t=${s.token}`;
  mail.send({ to: s.email, subject: t('confirmSubject', { site: siteTitle() }),
    html: `<p>${esc(t('confirmBody'))}</p><p><a href="${esc(link)}">${esc(link)}</a></p><p style="color:#6b7280">${esc(t('ignore'))}</p>`,
    text: `${t('confirmBody')}\n${link}\n\n${t('ignore')}\n` });
}

/** Signs an address up (also for other plugins: plugins.call('newsletter', 'subscribe', email, name)). */
export function subscribe(email: string, name: string, source: string): { ok: boolean; message: string; confirmed: boolean } {
  const e = normEmail(email);
  if (!validEmail(e)) return { ok: false, message: t('bad'), confirmed: false };
  const double = site.settings('doubleOptIn') !== false;
  const r = upsert(e, name, !double, source);
  if (r.needsConfirm) jobs.enqueue('confirmMail', { id: r.sub.id }, { key: 'confirm:' + r.sub.id });
  else if (r.isNew || r.sub.status === 'active') site.emit('newsletter.subscribed', { email: r.sub.email, name: r.sub.name, source });
  return { ok: true, message: double && r.sub.status !== 'active' ? t('checkMail') : t('done'), confirmed: r.sub.status === 'active' };
}

site.route('POST /newsletter/subscribe', { access: ['*'] }, ({ request }) => {
  const b = bodyOf(request);
  const json = String(request.headers.accept ?? '').includes('application/json');
  if (String(b.website ?? '').trim()) return json ? { status: 200, body: { ok: true, message: t('checkMail') } } : page(t('title'), `<p>${esc(t('checkMail'))}</p>`);
  const key = 'nl:rate:' + crypto.hash('sha256', request.ip || '-');
  const n = Number(cache.get(key) ?? 0);
  if (n >= 5) return json ? { status: 429, body: { ok: false, message: t('busy') } } : page(t('title'), `<p>${esc(t('busy'))}</p>`, 429);
  cache.set(key, n + 1, 600);
  const r = subscribe(b.email ?? '', b.name ?? '', 'form');
  if (json) return { status: r.ok ? 200 : 400, body: r };
  return page(t('title'), `<p>${esc(r.message)}</p>`, r.ok ? 200 : 400);
});

site.job('confirmMail', (ctx) => {
  const s = db.table('subscribers').where('id', Number(ctx.data?.id)).first() as Subscriber | null;
  if (s && s.status === 'pending') sendConfirm(s);
});

site.route('GET /newsletter/confirm', { access: ['*'] }, ({ request }) => {
  const s = byToken(String(Array.isArray(request.query.t) ? request.query.t[0] : request.query.t ?? ''));
  if (!s || s.status === 'unsubscribed') return page(t('title'), `<p>${esc(t('invalid'))}</p>`, 400);
  const was = s.status;
  const now = confirm(s);
  if (was !== 'active') site.emit('newsletter.subscribed', { email: now.email, name: now.name, source: now.source });
  return page(t('title'), `<p>✅ ${esc(t('confirmed'))}</p>`);
});

// GET asks (link checkers in mail apps open links), POST unsubscribes; mail apps' one-click POST works too.
site.route('GET /newsletter/unsubscribe', { access: ['*'] }, ({ request }) => {
  const tok = String(Array.isArray(request.query.t) ? request.query.t[0] : request.query.t ?? '');
  const s = byToken(tok);
  if (!s) return page(t('unsubTitle'), `<p>${esc(t('invalid'))}</p>`, 400);
  if (s.status === 'unsubscribed') return page(t('unsubTitle'), `<p>${esc(t('unsubDone'))}</p>`);
  return page(t('unsubTitle'), `<p>${esc(t('unsubAsk', { email: s.email }))}</p><form method="post" action="/newsletter/unsubscribe?t=${esc(tok)}">` +
    `<button type="submit" style="min-height:44px;padding:0 20px;border:0;border-radius:10px;background:#dc2626;color:#fff;font:inherit;font-weight:600;cursor:pointer">${esc(t('unsubBtn'))}</button></form>`);
});
site.route('POST /newsletter/unsubscribe', { access: ['*'] }, ({ request }) => {
  const s = byToken(String(Array.isArray(request.query.t) ? request.query.t[0] : request.query.t ?? ''));
  if (!s) return page(t('unsubTitle'), `<p>${esc(t('invalid'))}</p>`, 400);
  if (s.status !== 'unsubscribed') {
    unsubscribe(s);
    site.emit('newsletter.unsubscribed', { email: s.email });
  }
  return page(t('unsubTitle'), `<p>${esc(t('unsubDone'))}</p>`);
});

// ---------------------------------------------------------------- the form block

site.block('form', {
  label: { th: 'ฟอร์มรับข่าวสาร', en: 'Newsletter form' },
  icon: '📮',
  script: 'assets/newsletter.js',
  props: [
    { key: 'title', type: 'text', maxLength: 80, label: { th: 'หัวข้อ', en: 'Title' } },
    { key: 'text', type: 'text', maxLength: 200, label: { th: 'คำอธิบาย', en: 'Text' } },
    { key: 'button', type: 'text', maxLength: 30, label: { th: 'ข้อความบนปุ่ม', en: 'Button text' } },
    { key: 'askName', type: 'boolean', default: false, label: { th: 'ถามชื่อด้วย', en: 'Ask for a name' } },
  ],
  render: (_ctx, props) => ({
    html: `<form class="nl-form" method="post" action="/newsletter/subscribe" data-nl-form>` +
      (props.title !== '' ? `<h3>${esc(props.title || t('title'))}</h3>` : '') + (props.text !== '' ? `<p>${esc(props.text || t('sub'))}</p>` : '') +
      `<div class="nl-row">${props.askName ? `<input name="name" maxlength="100" placeholder="${esc(t('name'))}" aria-label="${esc(t('name'))}" autocomplete="name">` : ''}` +
      `<input type="email" name="email" required maxlength="191" placeholder="${esc(t('email'))}" aria-label="${esc(t('email'))}" autocomplete="email">` +
      `<button type="submit">${esc(props.button || t('button'))}</button></div>` +
      '<div class="nl-hp" aria-hidden="true"><input name="website" tabindex="-1" autocomplete="off"></div><p class="nl-msg" role="status" hidden></p></form>',
    css: `.nl-form{display:flex;flex-direction:column;gap:10px;max-width:560px}.nl-form h3{margin:0;font-size:1.3em}.nl-form p{margin:0;color:#4b5563}
.nl-row{display:flex;gap:8px;flex-wrap:wrap}.nl-row input{flex:1 1 200px;min-width:0;font:inherit;padding:11px 12px;border:1px solid #d1d5db;border-radius:10px}
.nl-row button{min-height:44px;padding:0 20px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer}
.nl-hp{position:absolute;left:-9999px;width:1px;height:1px;overflow:hidden}.nl-msg{padding:10px 14px;border-radius:10px;background:#dcfce7;color:#166534}
.nl-msg.nl-bad{background:#fee2e2;color:#991b1b}`,
  }),
});

// ---------------------------------------------------------------- sending

/** One batch of a campaign; queues the next one so that at most perHour mails go out an hour. */
site.job('sendBatch', { timeout: 300, retries: 5 }, (ctx) => {
  const c = getCampaign(Number(ctx.data?.id));
  if (!c || c.status !== 'sending') return;
  const perHour = Math.max(1, Math.min(5000, Number(site.settings('perHour') ?? 60) || 60));
  const batch = Math.min(20, perHour);
  const subs = db.table('subscribers').where('status', 'active').where('id', '>', c.cursor).orderBy('id').limit(batch).get() as Subscriber[];
  let sent = 0, failed = 0, cursor = c.cursor;
  for (const s of subs) {
    let ok = false;
    try {
      ok = mail.send(mailFor(c, s)).ok;
    } catch (e) {
      // the site's mail quota is used up: stop here and go on with this subscriber in the next batch
      if (/quota|limit|โควตา|เกิน/i.test(String((e as Error).message))) break;
    }
    if (ok) sent++;
    else failed++;
    cursor = s.id;
    ctx.progress(`${c.sent + sent}/${c.total}`);
  }
  db.table('campaigns').where('id', c.id).update({ cursor, sent: c.sent + sent, failed: c.failed + failed });
  const more = db.table('subscribers').where('status', 'active').where('id', '>', cursor).exists();
  if (!more) {
    db.table('campaigns').where('id', c.id).update({ status: 'sent', finished_at: new Date() });
    site.emit('newsletter.sent', { id: c.id, subject: c.subject, sent: c.sent + sent, failed: c.failed + failed });
    return;
  }
  // pace: batch mails per (3600 / perHour) seconds each
  jobs.enqueue('sendBatch', { id: c.id }, { delay: Math.max(5, Math.ceil((3600 / perHour) * Math.max(1, sent + failed))), key: 'campaign:' + c.id });
});

site.export('subscribe', (email: string, name?: string) => {
  const r = subscribe(String(email ?? ''), String(name ?? ''), 'plugin');
  return { ok: r.ok, confirmed: r.confirmed };
});

registerAdmin();
