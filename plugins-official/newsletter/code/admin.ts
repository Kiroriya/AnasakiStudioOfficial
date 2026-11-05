// The "Newsletter" admin page: subscribers (search, add, remove, CSV export) and campaigns (write, send a test,
// send to everyone, follow the progress). Drawn on the server with the admin UI kit; assets/admin.js runs the
// buttons and forms through anasaki.call('newsletter:…').
import { Campaign, campaigns, counts, esc, getCampaign, listSubscribers, normEmail, upsert, validEmail } from './model';
import { mailFor } from './text';

const CAPS = typeof (site as any).can === 'function';
const ACCESS = CAPS ? ['cap:newsletter.send'] : ['editor'];

const L = {
  title: ['จดหมายข่าว', 'Newsletter'],
  desc: ['ผู้รับข่าวและแคมเปญอีเมล — วางบล็อก “ฟอร์มรับข่าวสาร” บนหน้าเว็บเพื่อรับสมัคร', 'Subscribers and email campaigns — place the “Newsletter form” block on a page to collect sign-ups'],
  subs: ['ผู้รับข่าว', 'Subscribers'], camps: ['แคมเปญ', 'Campaigns'], newCamp: ['＋ แคมเปญใหม่', '＋ New campaign'],
  active: ['รับข่าวอยู่', 'Active'], pending: ['รอยืนยัน', 'Pending'], unsubscribed: ['ยกเลิกแล้ว', 'Unsubscribed'], all: ['ทั้งหมด', 'All'],
  email: ['อีเมล', 'Email'], name: ['ชื่อ', 'Name'], status: ['สถานะ', 'Status'], since: ['สมัครเมื่อ', 'Since'], source: ['ที่มา', 'Source'],
  search: ['ค้นหาอีเมลหรือชื่อ', 'Search email or name'], csv: ['ส่งออก CSV', 'Export CSV'], remove: ['ลบ', 'Remove'],
  add: ['เพิ่มผู้รับข่าว', 'Add subscribers'], addHint: ['วางอีเมล บรรทัดละรายการ (อีเมล,ชื่อ) — เพิ่มเป็น “รับข่าวอยู่” ทันที ใช้เฉพาะคนที่ยินยอมแล้ว', 'Paste one per line (email,name) — added as active right away; only people who agreed'],
  addBtn: ['เพิ่ม', 'Add'], none: ['ยังไม่มี', 'None yet'],
  subject: ['หัวเรื่อง', 'Subject'], body: ['ข้อความ', 'Message'], bodyHint: ['เว้นบรรทัดว่างเพื่อขึ้นย่อหน้าใหม่ ลิงก์ https:// กดได้อัตโนมัติ ลิงก์ยกเลิกรับข่าวถูกใส่ท้ายอีเมลให้', 'A blank line starts a paragraph; https:// links become clickable; the unsubscribe link is added at the end'],
  save: ['บันทึกฉบับร่าง', 'Save draft'], test: ['ส่งทดสอบถึงฉัน', 'Send me a test'], send: ['ส่งถึงผู้รับข่าวทั้งหมด', 'Send to all subscribers'],
  draft: ['ฉบับร่าง', 'Draft'], sending: ['กำลังส่ง', 'Sending'], sent: ['ส่งแล้ว', 'Sent'], progress: ['ส่งแล้ว {n} / {total}', '{n} / {total} sent'],
  failed: ['ไม่สำเร็จ {n}', '{n} failed'], edit: ['แก้ไข', 'Edit'], back: ['← แคมเปญทั้งหมด', '← All campaigns'],
  sendAsk: ['ส่ง “{s}” ถึงผู้รับข่าว {n} คน? ส่งแล้วยกเลิกไม่ได้', 'Send “{s}” to {n} subscribers? This cannot be undone'],
  pace: ['ส่งทยอย {n} ฉบับต่อชั่วโมง (ตั้งค่าได้)', 'Sent {n} an hour (a setting)'], preview: ['ตัวอย่าง', 'Preview'],
} as const;
type LK = keyof typeof L;
const q1 = (v: string | string[] | undefined) => String(Array.isArray(v) ? v[0] : v ?? '');
const jsonData = (v: unknown) => JSON.stringify(v).replace(/</g, '\\u003c').replace(/>/g, '\\u003e').replace(/&/g, '\\u0026');
const when = (s: string | null) => (s ? String(s).replace('T', ' ').slice(0, 16) : '');

function subscribersTab(query: Record<string, string | string[]>, tr: (k: LK, p?: Record<string, string | number>) => string): string {
  const status = q1(query.status);
  const search = q1(query.q);
  const page = Math.max(1, Number(q1(query.page)) || 1);
  const r = listSubscribers({ status, search, page, perPage: 50 });
  const c = counts();
  const tab = (s: string, label: string, n?: number) => `<a class="ak-tab" href="?tab=subs&status=${s}" aria-selected="${status === s}">${esc(label)}${n === undefined ? '' : ` (${n})`}</a>`;
  const badge = (s: string) => `<span class="ak-badge ${s === 'active' ? 'ak-badge-ok' : s === 'pending' ? 'ak-badge-warn' : ''}">${esc(tr(s as LK))}</span>`;
  const rows = r.items.map((s) => `<tr><td>${esc(s.email)}</td><td>${esc(s.name)}</td><td>${badge(s.status)}</td><td class="ak-hint">${esc(s.source)}</td>` +
    `<td class="ak-hint">${esc(when(s.created_at))}</td><td><button class="ak-btn ak-btn-sm ak-btn-danger" data-nl="removeSubscriber" data-id="${s.id}">${esc(tr('remove'))}</button></td></tr>`).join('');
  const pages = Math.ceil(r.total / r.perPage);
  return `<div class="ak-tabs">${tab('', tr('all'))}${tab('active', tr('active'), c.active)}${tab('pending', tr('pending'), c.pending)}${tab('unsubscribed', tr('unsubscribed'), c.unsubscribed)}</div>` +
    `<form method="get" class="ak-actions" style="margin-bottom:12px"><input type="hidden" name="tab" value="subs"><input type="hidden" name="status" value="${esc(status)}">` +
    `<input name="q" value="${esc(search)}" placeholder="${esc(tr('search'))}" style="max-width:320px"><button class="ak-btn">🔍</button><span style="flex:1"></span>` +
    `<button type="button" class="ak-btn" data-nl-download="${esc(status)}">⬇ ${esc(tr('csv'))}</button></form>` +
    (rows ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="nl-subscribers"><thead><tr><th>${esc(tr('email'))}</th><th>${esc(tr('name'))}</th>` +
      `<th>${esc(tr('status'))}</th><th>${esc(tr('source'))}</th><th>${esc(tr('since'))}</th><th></th></tr></thead><tbody>${rows}</tbody></table></div></div>`
      : `<div class="ak-card"><p class="ak-muted">${esc(tr('none'))}</p></div>`) +
    (pages > 1 ? `<div class="ak-actions" style="margin-top:12px">${page > 1 ? `<a class="ak-btn ak-btn-sm" href="?tab=subs&status=${esc(status)}&q=${encodeURIComponent(search)}&page=${page - 1}">‹</a>` : ''}<span class="ak-hint">${page} / ${pages}</span>${page < pages ? `<a class="ak-btn ak-btn-sm" href="?tab=subs&status=${esc(status)}&q=${encodeURIComponent(search)}&page=${page + 1}">›</a>` : ''}</div>` : '') +
    `<div class="ak-card" style="margin-top:16px"><h3>${esc(tr('add'))}</h3><p class="ak-hint">${esc(tr('addHint'))}</p>` +
    `<form data-nl-form="addSubscribers"><textarea name="lines" rows="4" placeholder="name@example.com,Somchai"></textarea><div class="ak-form-actions"><button class="ak-btn ak-btn-primary">${esc(tr('addBtn'))}</button></div></form></div>`;
}

function campaignStatus(c: Campaign, tr: (k: LK, p?: Record<string, string | number>) => string): string {
  if (c.status === 'draft') return `<span class="ak-badge">${esc(tr('draft'))}</span>`;
  const p = esc(tr('progress', { n: c.sent, total: c.total })) + (c.failed ? ' · ' + esc(tr('failed', { n: c.failed })) : '');
  return `<span class="ak-badge ${c.status === 'sent' ? 'ak-badge-ok' : 'ak-badge-info'}">${esc(tr(c.status as LK))}</span> <span class="ak-hint">${p}</span>`;
}

function campaignsTab(query: Record<string, string | string[]>, tr: (k: LK, p?: Record<string, string | number>) => string): string {
  const editId = q1(query.edit);
  if (editId) {
    const c = editId === 'new' ? null : getCampaign(Number(editId));
    const locked = !!c && c.status !== 'draft';
    const active = counts().active;
    const perHour = Math.max(1, Number(site.settings('perHour') ?? 60) || 60);
    return `<p><a class="ak-btn ak-btn-sm ak-btn-ghost" href="?tab=camps">${esc(tr('back'))}</a></p>` +
      `<div class="ak-card"><form data-nl-form="saveCampaign" data-id="${c ? c.id : ''}">` +
      `<label class="ak-field"><span>${esc(tr('subject'))}</span><input name="subject" maxlength="200" required value="${esc(c?.subject ?? '')}"${locked ? ' disabled' : ''}></label>` +
      `<label class="ak-field"><span>${esc(tr('body'))}</span><textarea name="body" rows="14" required${locked ? ' disabled' : ''}>${esc(c?.body ?? '')}</textarea><p class="ak-hint">${esc(tr('bodyHint'))}</p></label>` +
      (locked ? `<p>${campaignStatus(c!, tr)}</p>` : `<div class="ak-form-actions"><button type="submit" class="ak-btn">${esc(tr('save'))}</button>` +
        `<button type="button" class="ak-btn" data-nl="sendTest">${esc(tr('test'))}</button>` +
        `<button type="button" class="ak-btn ak-btn-primary" data-nl="send" data-n="${active}" data-ask="${esc(tr('sendAsk', { s: '{s}', n: active }))}">${esc(tr('send'))} (${active})</button></div>` +
        `<p class="ak-hint">${esc(tr('pace', { n: perHour }))}</p>`) + '</form></div>';
  }
  const rows = campaigns().map((c) => `<tr><td><a href="?tab=camps&edit=${c.id}"><b>${esc(c.subject)}</b></a></td><td>${campaignStatus(c, tr)}</td>` +
    `<td class="ak-hint">${esc(when(c.finished_at ?? c.started_at ?? c.created_at))}</td></tr>`).join('');
  return `<div class="ak-actions" style="margin-bottom:12px"><a class="ak-btn ak-btn-primary" href="?tab=camps&edit=new" id="nl-new">${esc(tr('newCamp'))}</a></div>` +
    (rows ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="nl-campaigns"><tbody>${rows}</tbody></table></div></div>` : `<div class="ak-card"><p class="ak-muted">${esc(tr('none'))}</p></div>`);
}

const csvCell = (v: unknown) => {
  const s = String(v ?? '');
  return /[",\n\r]|^[=+\-@]/.test(s) ? '"' + s.replace(/^([=+\-@])/, "'$1").replace(/"/g, '""') + '"' : s;
};

export function registerAdmin(): void {
  site.adminPage('newsletter', {
    label: { th: 'จดหมายข่าว', en: 'Newsletter' },
    icon: '📮',
    access: ACCESS,
    render: (ctx) => {
      const en = ctx.locale === 'en' ? 1 : 0;
      const tr = (k: LK, p?: Record<string, string | number>) => {
        let s: string = L[k][en];
        if (p) for (const [a, b] of Object.entries(p)) s = s.split(`{${a}}`).join(String(b));
        return s;
      };
      const tab = q1(ctx.query.tab) === 'camps' || q1(ctx.query.edit) ? 'camps' : 'subs';
      let script = '';
      try {
        script = site.asset('assets/admin.js').url;
      } catch {
        /* none */
      }
      return {
        title: tr('title'),
        html: `<header class="ak-page-head"><span class="ak-page-icon">📮</span><div class="ak-page-head-text"><h1>${esc(tr('title'))}</h1><p>${esc(tr('desc'))}</p></div></header>` +
          `<div class="ak-tabs"><a class="ak-tab" href="?tab=subs" aria-selected="${tab === 'subs'}">${esc(tr('subs'))} (${counts().active})</a>` +
          `<a class="ak-tab" href="?tab=camps" aria-selected="${tab === 'camps'}">${esc(tr('camps'))}</a></div>` +
          (tab === 'subs' ? subscribersTab(ctx.query, tr) : campaignsTab(ctx.query, tr)) +
          `<script type="application/json" id="nl-admin-data">${jsonData({ locale: en ? 'en' : 'th' })}</script>` +
          (script ? `<script src="${esc(script)}" defer></script>` : ''),
      };
    },
    download: (ctx) => {
      const status = q1(ctx.query.status);
      const lines = ['email,name,status,source,created_at,confirmed_at'];
      for (let page = 1; page <= 500; page++) {
        const r = listSubscribers({ status, page, perPage: 100 });
        for (const s of r.items) lines.push([s.email, s.name, s.status, s.source, s.created_at, s.confirmed_at ?? ''].map(csvCell).join(','));
        if (page * r.perPage >= r.total) break;
      }
      return { status: 200, headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': 'attachment; filename="subscribers.csv"', 'Cache-Control': 'no-store' },
        body: '\ufeff' + lines.join('\r\n') };
    },
  });

  const fail = (m: string) => ({ message: m, errors: { form: m } });

  site.action('addSubscribers', {
    input: { lines: 'text' },
    access: ACCESS,
    run: ({ input }) => {
      let added = 0, skipped = 0;
      for (const line of String(input.lines).split(/\r?\n/).slice(0, 5000)) {
        const [e, ...rest] = line.split(',');
        const email = normEmail(e);
        if (!email) continue;
        if (!validEmail(email)) {
          skipped++;
          continue;
        }
        const r = upsert(email, rest.join(',').trim(), true, 'import');
        if (r.isNew || r.sub.status === 'active') added++;
      }
      return { message: `+${added}${skipped ? ` (skipped ${skipped})` : ''}`, data: { added, skipped } };
    },
  });

  site.action('removeSubscriber', {
    input: { id: 'number' },
    access: ACCESS,
    run: ({ input }) => ({ data: { removed: db.table('subscribers').where('id', Number(input.id)).delete() } }),
  });

  site.action('saveCampaign', {
    input: { id: 'number?', subject: 'text', body: 'text' },
    access: ACCESS,
    run: ({ input }) => {
      const subject = String(input.subject).trim().slice(0, 200);
      const body = String(input.body).trim().slice(0, 50000);
      if (!subject || !body) return fail('subject / body');
      if (input.id) {
        const c = getCampaign(Number(input.id));
        if (!c) return fail('not found');
        if (c.status !== 'draft') return fail('already sent');
        db.table('campaigns').where('id', c.id).update({ subject, body });
        return { data: { id: c.id } };
      }
      return { data: { id: db.table('campaigns').insert({ subject, body }) } };
    },
  });

  site.action('sendTest', {
    input: { id: 'number' },
    access: ACCESS,
    run: ({ input, user }) => {
      const c = getCampaign(Number(input.id));
      if (!c || !user?.email) return fail('not found');
      const r = mail.send(mailFor(c, { email: user.email, token: '0'.repeat(48) }));
      return r.ok ? { message: '✓ ' + user.email } : fail('mail failed');
    },
  });

  site.action('send', {
    input: { id: 'number' },
    access: ACCESS,
    run: ({ input }) => {
      const c = getCampaign(Number(input.id));
      if (!c) return fail('not found');
      if (c.status !== 'draft') return fail('already sent');
      const total = counts().active;
      if (!total) return fail('no subscribers');
      db.table('campaigns').where('id', c.id).update({ status: 'sending', total, cursor: 0, sent: 0, failed: 0, started_at: new Date() });
      jobs.enqueue('sendBatch', { id: c.id }, { key: 'campaign:' + c.id });
      return { data: { id: c.id, total } };
    },
  });
}
