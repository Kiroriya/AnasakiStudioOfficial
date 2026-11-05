// blog-extras 1.0 — what a blog needs around its posts: reading time and a table of contents above the text,
// share buttons, an author box, related posts and comments below it. Added to every post (type "post") through
// post.content, so the default post template shows them; each part is a setting, and the designer blocks place
// comments / share / related posts anywhere in a custom template instead.
import { NewComment, add, check, get, setStatus, list, counts, remove, Status, STATUSES } from './comments';
import { CSS, authorBox, commentsSection, minutes, relatedList, shareBar, withToc } from './render';
import { esc, lang, t } from './text';

const CAPS = typeof (site as any).can === 'function';
const MODERATE = CAPS ? ['cap:blog-extras.moderate'] : ['editor'];

const SCRIPT = (() => {
  try {
    return site.asset('assets/blog.js').url;
  } catch {
    return '';
  }
})();
const scriptTag = () => (SCRIPT ? `<script defer src="${esc(SCRIPT)}"></script>` : '');

const on = (key: string) => site.settings(key) !== false;

function siteURL(): string {
  try {
    return site.info().url || '';
  } catch {
    return '';
  }
}

interface RelatedPost { title: string; url: string; image: string }

/** Up to n published posts of the same category (newest first), cached ten minutes per post. */
function related(postId: number, categoryUrl: string, n: number): RelatedPost[] {
  const cat = String(categoryUrl ?? '').replace(/\/+$/, '').split('/').pop() ?? '';
  if (!cat || n <= 0) return [];
  const key = `be:related:${postId}:${n}`;
  const hit = cache.get<RelatedPost[]>(key);
  if (hit) return hit;
  const { items } = db.posts.list({ type: 'post', category: decodeURIComponent(cat), perPage: n + 1, status: 'publish' });
  const out = items.filter((p) => p.id !== postId).slice(0, n).map((p) => ({ title: p.title, url: p.url, image: p.image }));
  cache.set(key, out, 3600);
  return out;
}

function author(postId: number): { name: string; bio: string; avatar: string; website: string } | null {
  const key = 'be:author:' + postId;
  const hit = cache.get<any>(key);
  if (hit !== undefined) return hit;
  const p = db.posts.get(postId);
  const u = p && (p as any).authorId ? db.users.get((p as any).authorId) : null;
  const out = u ? { name: u.name, bio: u.bio, avatar: u.avatar, website: u.website } : null;
  cache.set(key, out, 3600);
  return out;
}

/** What the DB-dependent part (author, related posts, comments) depends on — part of its cache key. */
const bottomKey = (id: number) =>
  `be:bottom:${id}:${site.settings('authorBox') === true ? 1 : 0}:${Number(site.settings('related') ?? 3)}:${on('comments') ? 1 : 0}:${lang()}`;

/** Author box, related posts and comments of a post (database reads; cached an hour, cleared on comment changes). */
function bottom(id: number, categoryUrl: string, url: string): string {
  let out = '';
  if (site.settings('authorBox') === true) {
    const a = author(id);
    if (a) out += authorBox(a);
  }
  const n = Number(site.settings('related') ?? 3);
  if (n > 0) out += relatedList(related(id, categoryUrl, n));
  if (on('comments')) out += commentsSection(id, url);
  cache.set(bottomKey(id), out, 3600);
  return out;
}

// post.content runs within 50 ms, so it reads only the cache: on a miss the part below the post is loaded by
// assets/blog.js from GET /blog-extras/post/<id>/more (which fills the cache for the next visitors).
site.filter('post.content', (html: string, ctx: any) => {
  const post = ctx?.post;
  if (!post || post.type !== 'post') return html;
  const id = Number(post.id) || 0;
  const url = siteURL() + String(post.url ?? '');
  let top = '';
  let body = html;
  if (on('readingTime')) top += `<p class="bx-meta">⏱ ${esc(t('minutes', { n: minutes(html) }))}</p>`;
  if (on('toc')) {
    const r = withToc(html);
    body = r.html;
    top += r.toc;
  }
  let below = on('share') ? shareBar(url, String(post.title ?? '')) : '';
  if (id) {
    const hit = cache.get<string>(bottomKey(id));
    below += hit ?? `<div class="bx-more" data-bx-more="/blog-extras/post/${id}/more"></div>`;
  }
  return `<style>${CSS}</style>` + top + body + below + scriptTag();
});

site.route('GET /blog-extras/post/{id}/more', { access: ['*'] }, ({ request }) => {
  const p = db.posts.get(Number(request.params.id) || 0);
  if (!p || p.type !== 'post' || p.status !== 'publish') return { status: 404, headers: { 'Content-Type': 'text/plain' }, body: '' };
  const cat = p.categories?.[0]?.slug ? '/category/' + p.categories[0].slug : '';
  return { status: 200, headers: { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' }, body: bottom(p.id, cat, p.url) };
});

// ---------------------------------------------------------------- posting a comment

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

const json = (req: SiteRequest) => String(req.headers.accept ?? '').includes('application/json');
const safeBack = (s: string) => (/^\/(?!\/)[^\s\\]*$/.test(s) ? s : '/');

function answer(req: SiteRequest, res: SiteResponse, status: number, key: string, back: string, errors?: Record<string, string>): RouteResult {
  if (json(req)) return { status, body: { message: t(key as any), posted: key === 'posted', errors: errors ? Object.fromEntries(Object.entries(errors).map(([k, v]) => [k, t(v as any)])) : undefined } };
  const ok = status < 300;
  return res.page({ title: t(ok ? 'comments' : 'e_failed'),
    html: `<div style="max-width:640px;margin:64px auto;padding:0 20px"><style>${CSS}</style>` +
      `<p class="${ok ? 'bx-note' : 'bx-err'}">${esc(t(key as any))}</p>` +
      (errors ? `<ul>${Object.values(errors).map((v) => `<li>${esc(t(v as any))}</li>`).join('')}</ul>` : '') +
      `<p><a href="${esc(back)}#comments">← ${esc(t('back'))}</a></p></div>` }, status);
}

site.route('POST /blog-extras/comment', { access: ['*'] }, ({ request, response }) => {
  const b = bodyOf(request);
  const back = safeBack(String(b.back ?? '/'));
  if (!on('comments')) return answer(request, response, 403, 'e_closed', back);
  if (String(b.website ?? '').trim()) return answer(request, response, 200, 'pending', back); // honeypot: pretend it worked
  const c: NewComment = { postId: Number(b.post) || 0, parentId: Number(b.parent) || 0, name: String(b.name ?? ''), email: String(b.email ?? ''), body: String(b.body ?? '') };
  const post = c.postId ? db.posts.get(c.postId) : null;
  if (!post || post.type !== 'post' || post.status !== 'publish') return answer(request, response, 404, 'e_failed', back);
  const errors = check(c);
  if (Object.keys(errors).length) return answer(request, response, 422, 'e_failed', back, errors);
  const ipHash = crypto.hash('sha256', 'bx:' + (request.ip || '-'));
  const k = 'be:rate:' + ipHash;
  const n = Number(cache.get(k) ?? 0);
  if (n >= 3) return answer(request, response, 429, 'e_busy', post.url);
  cache.set(k, n + 1, 60);
  const status: Status = site.settings('moderation') === 'auto' ? 'approved' : 'pending';
  const id = add(c, status, ipHash);
  site.emit('blog-extras.comment.created', { id, postId: post.id, status, name: c.name.trim() });
  if (String(site.settings('notifyEmail') ?? '').trim()) jobs.enqueue('notify', { id });
  return answer(request, response, 201, status === 'approved' ? 'posted' : 'pending', post.url);
});

site.job('notify', (ctx) => {
  const c = get(Number(ctx.data?.id));
  const to = String(site.settings('notifyEmail') ?? '').trim();
  if (!c || !to) return;
  const p = db.posts.get(c.post_id);
  mail.send({ to, subject: `💬 ${c.name} — ${p?.title ?? ''}`,
    html: `<p><b>${esc(c.name)}</b>${c.email ? ' &lt;' + esc(c.email) + '&gt;' : ''}</p><p style="white-space:pre-wrap">${esc(c.body)}</p>` +
      `<p>${esc(c.status)} · <a href="${esc(siteURL() + (p?.url ?? '/'))}#comment-${c.id}">${esc(p?.title ?? '')}</a></p>` });
});

// ---------------------------------------------------------------- designer blocks (custom post templates)

site.block('comments', {
  label: { th: 'ความคิดเห็น', en: 'Comments' },
  icon: '💬',
  script: 'assets/blog.js',
  render: (ctx) => {
    const id = Number(ctx.post?.id) || 0;
    if (!id) return { html: (ctx as any).preview === false ? '' : '<p class="bx-empty">💬 ' + esc(t('comments')) + '</p>', css: CSS };
    return { html: commentsSection(id, String(ctx.post?.url ?? '/')), css: CSS };
  },
});

site.block('share', {
  label: { th: 'ปุ่มแชร์', en: 'Share buttons' },
  icon: '🔗',
  script: 'assets/blog.js',
  render: (ctx) => ({ html: shareBar(siteURL() + String(ctx.post?.url ?? ctx.path ?? '/'), String(ctx.post?.title ?? '')), css: CSS }),
});

site.block('related', {
  label: { th: 'บทความที่เกี่ยวข้อง', en: 'Related posts' },
  icon: '🧭',
  props: [{ key: 'count', type: 'number', min: 1, max: 12, default: 3, label: { th: 'จำนวน', en: 'Count' } }],
  render: (ctx, props) => {
    const id = Number(ctx.post?.id) || 0;
    return { html: id ? relatedList(related(id, String(ctx.post?.categoryUrl ?? ''), Math.min(12, Math.max(1, Number(props.count) || 3)))) : '', css: CSS };
  },
});

// ---------------------------------------------------------------- moderation

const L = {
  title: ['ความคิดเห็น', 'Comments'], desc: ['อนุมัติหรือลบความคิดเห็นจากผู้อ่าน', 'Approve or remove readers’ comments'],
  pending: ['รออนุมัติ', 'Pending'], approved: ['อนุมัติแล้ว', 'Approved'], spam: ['สแปม', 'Spam'], all: ['ทั้งหมด', 'All'],
  approve: ['อนุมัติ', 'Approve'], unapprove: ['ซ่อน', 'Hide'], markSpam: ['สแปม', 'Spam'], del: ['ลบ', 'Delete'],
  none: ['ไม่มีความคิดเห็น', 'No comments'], on: ['ใน', 'on'], confirmDel: ['ลบความคิดเห็นนี้ (และคำตอบ)?', 'Delete this comment (and its replies)?'],
} as const;
type LK = keyof typeof L;
const q1 = (v: string | string[] | undefined) => String(Array.isArray(v) ? v[0] : v ?? '');

site.adminPage('comments', {
  label: { th: 'ความคิดเห็น', en: 'Comments' },
  icon: '💬',
  access: MODERATE,
  render: (ctx) => {
    const en = ctx.locale === 'en' ? 1 : 0;
    const tr = (k: LK) => L[k][en];
    const status = q1(ctx.query.status) || 'pending';
    const page = Math.max(1, Number(q1(ctx.query.page)) || 1);
    const r = list({ status: status === 'all' ? '' : status, page, perPage: 30 });
    const c = counts();
    const titles = new Map<number, { title: string; url: string }>();
    const ids = [...new Set(r.items.map((x) => x.post_id))];
    if (ids.length) for (const p of db.posts.list({ type: 'post', ids, perPage: 100, status: 'any' }).items) titles.set(p.id, { title: p.title, url: p.url });
    const tab = (s: string, label: string, n?: number) => `<a class="ak-tab" href="?status=${s}" aria-selected="${status === s}">${esc(label)}${n === undefined ? '' : ` (${n})`}</a>`;
    const btn = (id: number, action: string, label: string, cls = '') => `<button class="ak-btn ak-btn-sm ${cls}" data-bx-action="${action}" data-id="${id}">${esc(label)}</button>`;
    const rows = r.items.map((x) => {
      const p = titles.get(x.post_id);
      return `<tr id="bx-row-${x.id}"><td><b>${esc(x.name)}</b>${x.email ? `<div class="ak-hint">${esc(x.email)}</div>` : ''}</td>` +
        `<td style="max-width:460px"><div style="white-space:pre-wrap;overflow-wrap:anywhere">${esc(x.body)}</div>` +
        `<div class="ak-hint">${esc(tr('on'))} ${p ? `<a href="${esc(p.url)}#comment-${x.id}" target="_blank" rel="noopener">${esc(p.title)}</a>` : '#' + x.post_id} · ${esc(String(x.created_at).replace('T', ' ').slice(0, 16))}</div></td>` +
        `<td><div class="ak-actions">${x.status !== 'approved' ? btn(x.id, 'approved', tr('approve'), 'ak-btn-primary') : btn(x.id, 'pending', tr('unapprove'))}` +
        `${x.status !== 'spam' ? btn(x.id, 'spam', tr('markSpam')) : ''}${btn(x.id, 'delete', tr('del'), 'ak-btn-danger')}</div></td></tr>`;
    }).join('');
    const pages = Math.ceil(r.total / r.perPage);
    return {
      title: tr('title'),
      html: `<header class="ak-page-head"><span class="ak-page-icon">💬</span><div class="ak-page-head-text"><h1>${esc(tr('title'))}</h1><p>${esc(tr('desc'))}</p></div></header>` +
        `<div class="ak-tabs">${tab('pending', tr('pending'), c.pending ?? 0)}${tab('approved', tr('approved'), c.approved ?? 0)}${tab('spam', tr('spam'), c.spam ?? 0)}${tab('all', tr('all'))}</div>` +
        (rows ? `<div class="ak-card ak-card-flush"><div class="ak-table-wrap"><table class="ak-table" id="bx-comments"><tbody>${rows}</tbody></table></div></div>` : `<div class="ak-card"><p class="ak-muted">${esc(tr('none'))}</p></div>`) +
        (pages > 1 ? `<div class="ak-actions" style="margin-top:12px">${page > 1 ? `<a class="ak-btn ak-btn-sm" href="?status=${status}&page=${page - 1}">‹</a>` : ''}<span class="ak-hint">${page} / ${pages}</span>${page < pages ? `<a class="ak-btn ak-btn-sm" href="?status=${status}&page=${page + 1}">›</a>` : ''}</div>` : '') +
        `<script type="application/json" id="bx-admin-data">${JSON.stringify({ confirmDel: tr('confirmDel') }).replace(/</g, '\\u003c')}</script>` +
        `<script src="${esc((() => { try { return site.asset('assets/admin.js').url; } catch { return ''; } })())}" defer></script>`,
    };
  },
});

site.action('moderate', {
  input: { id: 'number', action: 'text' },
  access: MODERATE,
  run: ({ input }) => {
    const id = Number(input.id);
    if (input.action === 'delete') return remove(id) ? { data: { id, deleted: true } } : { message: 'not found', errors: { id: 'not found' } };
    if (!STATUSES.includes(input.action as Status)) return { message: 'bad action', errors: { action: 'bad action' } };
    const before = get(id);
    const c = setStatus(id, input.action as Status);
    if (!c) return { message: 'not found', errors: { id: 'not found' } };
    if (before && before.status !== 'approved' && c.status === 'approved') site.emit('blog-extras.comment.approved', { id, postId: c.post_id });
    return { data: { id, status: c.status } };
  },
});

site.adminNotice((ctx) => {
  const n = counts().pending ?? 0;
  if (!n) return null;
  return { level: 'info', text: { th: `มีความคิดเห็นรออนุมัติ ${n} รายการ`, en: `${n} comment(s) waiting for approval` },
    link: { label: { th: 'ตรวจความคิดเห็น', en: 'Review comments' }, href: '/admin/x/code/blog-extras:comments' } };
}, { access: MODERATE });
