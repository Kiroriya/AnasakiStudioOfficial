// HTML of the extras. The post.content filter has 50 ms, so nothing here reads package files: the CSS is a
// constant and the script URL is looked up once when the plugin loads.
import { CommentRow, approved } from './comments';
import { esc, t } from './text';

export const CSS = `.bx-meta{color:#6b7280;font-size:.9em;margin:0 0 1em}
.bx-toc{border:1px solid #e5e7eb;border-radius:12px;padding:12px 18px;margin:0 0 1.5em;background:#f9fafb}
.bx-toc summary{font-weight:700;cursor:pointer}
.bx-toc ol{margin:.6em 0 0;padding-left:1.3em}.bx-toc li{margin:.25em 0}.bx-toc li.bx-h3{margin-left:1em;list-style:circle}
.bx-toc a{color:inherit;text-decoration:none}.bx-toc a:hover{text-decoration:underline}
.bx-share{display:flex;flex-wrap:wrap;align-items:center;gap:8px;margin:2em 0 1em}
.bx-share b{margin-right:4px}
.bx-share a,.bx-share button{display:inline-flex;align-items:center;gap:6px;min-height:36px;padding:0 14px;border-radius:999px;border:1px solid #e5e7eb;background:#fff;color:inherit;font:inherit;font-size:.9em;text-decoration:none;cursor:pointer}
.bx-share a:hover,.bx-share button:hover{border-color:var(--c-primary,#4f46e5);color:var(--c-primary,#4f46e5)}
.bx-author{display:flex;gap:14px;align-items:flex-start;padding:16px;border-radius:12px;background:#f9fafb;margin:1.5em 0}
.bx-author img{width:56px;height:56px;border-radius:50%;object-fit:cover;flex-shrink:0}
.bx-author p{margin:.2em 0 0;color:#4b5563;font-size:.95em}
.bx-related{margin:2em 0}.bx-related h2,.bx-comments h2{font-size:1.25em;margin:0 0 .8em}
.bx-related ul{list-style:none;margin:0;padding:0;display:grid;gap:14px;grid-template-columns:repeat(auto-fill,minmax(180px,1fr))}
.bx-related a{display:block;color:inherit;text-decoration:none;font-weight:600}
.bx-related img{width:100%;aspect-ratio:16/10;object-fit:cover;border-radius:10px;margin-bottom:6px;background:#f3f4f6}
.bx-comments{margin:2.5em 0 1em;padding-top:1.5em;border-top:1px solid #e5e7eb}
.bx-comment{padding:12px 0;border-bottom:1px solid #f1f2f5}
.bx-comment .bx-c-head{display:flex;gap:8px;align-items:baseline;flex-wrap:wrap}
.bx-comment time{color:#9ca3af;font-size:.85em}
.bx-comment p{margin:.35em 0;white-space:pre-wrap;overflow-wrap:anywhere}
.bx-replies{margin-left:22px;padding-left:14px;border-left:2px solid #eef0f3}
.bx-reply-btn{border:0;background:none;color:var(--c-primary,#4f46e5);font:inherit;font-size:.85em;cursor:pointer;padding:0}
.bx-form{display:flex;flex-direction:column;gap:10px;margin-top:1.2em}
.bx-form .bx-row{display:grid;grid-template-columns:1fr 1fr;gap:10px}
@media (max-width:600px){.bx-form .bx-row{grid-template-columns:1fr}}
.bx-form input,.bx-form textarea{font:inherit;padding:10px 12px;border:1px solid #d1d5db;border-radius:8px;width:100%;box-sizing:border-box}
.bx-form textarea{min-height:110px;resize:vertical}
.bx-form button[type=submit]{align-self:flex-start;min-height:42px;padding:0 18px;border:0;border-radius:10px;background:var(--c-primary,#4f46e5);color:#fff;font:inherit;font-weight:600;cursor:pointer}
.bx-hp{position:absolute;left:-9999px;width:1px;height:1px;overflow:hidden}
.bx-note{padding:10px 14px;border-radius:10px;background:#dcfce7;color:#166534}
.bx-err{padding:10px 14px;border-radius:10px;background:#fee2e2;color:#991b1b}
.bx-replying{font-size:.9em;color:#4b5563}`;

/** Plain text of HTML (for reading time). */
const textOf = (html: string) => html.replace(/<(script|style)[^>]*>[\s\S]*?<\/\1>/gi, ' ').replace(/<[^>]+>/g, ' ').replace(/&[a-z#0-9]+;/gi, ' ');

/** Minutes to read: Thai has no spaces, so count characters (~ 900 a minute) and words (~ 230 a minute). */
export function minutes(html: string): number {
  const text = textOf(html);
  const thai = (text.match(/[฀-๿]/g) ?? []).length;
  const words = text.replace(/[฀-๿]+/g, ' ').split(/\s+/).filter(Boolean).length;
  return Math.max(1, Math.round(thai / 900 + words / 230));
}

const slug = (s: string, used: Set<string>) => {
  let base = s.toLowerCase().replace(/<[^>]+>/g, '').replace(/&[a-z#0-9]+;/gi, '').trim()
    .replace(/[^a-z0-9฀-๿぀-ヿ一-鿿가-힯]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 60) || 'section';
  if (/^\d/.test(base)) base = 's-' + base;
  let id = base, i = 2;
  while (used.has(id)) id = base + '-' + i++;
  used.add(id);
  return id;
};

/** Adds ids to h2/h3 (keeping existing ones) and answers the table of contents (empty with fewer than 3). */
export function withToc(html: string): { html: string; toc: string } {
  const used = new Set<string>();
  const items: { level: number; id: string; text: string }[] = [];
  const out = html.replace(/<(h[23])(\s[^>]*)?>([\s\S]*?)<\/\1>/gi, (m, tag: string, attrs = '', inner: string) => {
    const own = /\sid="([^"]+)"/i.exec(attrs);
    const id = own ? own[1] : slug(inner, used);
    if (own) used.add(own[1]);
    const text = inner.replace(/<[^>]+>/g, '').trim();
    if (text) items.push({ level: tag.toLowerCase() === 'h2' ? 2 : 3, id, text });
    return own ? m : `<${tag}${attrs} id="${esc(id)}">${inner}</${tag}>`;
  });
  if (items.length < 3) return { html, toc: '' };
  const lis = items.map((h) => `<li class="bx-h${h.level}"><a href="#${esc(h.id)}">${h.text.replace(/</g, '&lt;')}</a></li>`).join('');
  return { html: out, toc: `<details class="bx-toc" open><summary>${esc(t('toc'))}</summary><ol>${lis}</ol></details>` };
}

export function shareBar(url: string, title: string): string {
  const u = encodeURIComponent(url), tt = encodeURIComponent(title);
  return `<div class="bx-share" data-bx-share><b>${esc(t('share'))}</b>` +
    `<a href="https://www.facebook.com/sharer/sharer.php?u=${u}" target="_blank" rel="noopener nofollow">Facebook</a>` +
    `<a href="https://twitter.com/intent/tweet?url=${u}&amp;text=${tt}" target="_blank" rel="noopener nofollow">X</a>` +
    `<a href="https://social-plugins.line.me/lineit/share?url=${u}" target="_blank" rel="noopener nofollow">LINE</a>` +
    `<button type="button" data-bx-copy="${esc(url)}" data-done="${esc(t('copied'))}">🔗 ${esc(t('copy'))}</button></div>`;
}

export function relatedList(posts: { title: string; url: string; image: string }[]): string {
  if (!posts.length) return '';
  return `<section class="bx-related"><h2>${esc(t('related'))}</h2><ul>` + posts.map((p) =>
    `<li><a href="${esc(p.url)}">${p.image ? `<img src="${esc(p.image)}" alt="" loading="lazy">` : ''}${esc(p.title)}</a></li>`).join('') + '</ul></section>';
}

export function authorBox(u: { name: string; bio: string; avatar: string; website: string }): string {
  return `<aside class="bx-author">${u.avatar ? `<img src="${esc(u.avatar)}" alt="">` : ''}<div><b>${esc(t('author'))} ${esc(u.name)}</b>` +
    (u.bio ? `<p>${esc(u.bio)}</p>` : '') + '</div></aside>';
}

const when = (s: string) => String(s ?? '').replace('T', ' ').slice(0, 16);

function commentHTML(c: CommentRow, replies: CommentRow[]): string {
  const one = (x: CommentRow, canReply: boolean) => `<article class="bx-comment" id="comment-${x.id}"><div class="bx-c-head"><b>${esc(x.name)}</b><time>${esc(when(x.created_at))}</time></div>` +
    `<p>${esc(x.body)}</p>${canReply ? `<button type="button" class="bx-reply-btn" data-bx-reply="${x.id}" data-name="${esc(x.name)}">${esc(t('reply'))}</button>` : ''}</article>`;
  return one(c, true) + (replies.length ? `<div class="bx-replies">${replies.map((r) => one(r, false)).join('')}</div>` : '');
}

/** The comments of a post and the form (notice: 'pending' | 'posted' | an error key, from ?comment=). */
export function commentsSection(postId: number, back: string, notice = ''): string {
  const rows = approved(postId);
  const top = rows.filter((r) => !r.parent_id);
  const list = top.length
    ? top.map((c) => commentHTML(c, rows.filter((r) => r.parent_id === c.id))).join('')
    : `<p class="bx-empty">${esc(t('none'))}</p>`;
  const note = notice === 'pending' || notice === 'posted'
    ? `<p class="bx-note" role="status">${esc(t(notice))}</p>`
    : notice ? `<p class="bx-err" role="alert">${esc(t(notice as any) || t('e_failed'))}</p>` : '';
  return `<section class="bx-comments" id="comments"><h2>${esc(t('count', { n: rows.length }))}</h2>${note}${list}` +
    `<form class="bx-form" method="post" action="/blog-extras/comment" id="bx-comment-form"><h3>${esc(t('leave'))}</h3>` +
    `<input type="hidden" name="post" value="${postId}"><input type="hidden" name="parent" value="0"><input type="hidden" name="back" value="${esc(back)}">` +
    `<p class="bx-replying" hidden><span></span> <button type="button" class="bx-reply-btn" data-bx-cancel>${esc(t('cancelReply'))}</button></p>` +
    `<div class="bx-row"><label><span>${esc(t('name'))}</span><input name="name" required maxlength="100" autocomplete="name"></label>` +
    `<label><span>${esc(t('email'))}</span><input type="email" name="email" maxlength="191" autocomplete="email"></label></div>` +
    `<label><span>${esc(t('body'))}</span><textarea name="body" required minlength="2" maxlength="3000"></textarea></label>` +
    '<div class="bx-hp" aria-hidden="true"><label>Website<input name="website" tabindex="-1" autocomplete="off"></label></div>' +
    `<button type="submit">${esc(t('send'))}</button></form></section>`;
}
