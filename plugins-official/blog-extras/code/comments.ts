// Comments (p_blog_extras_comments): one level of replies, held for approval unless the setting says "auto".

export type Status = 'pending' | 'approved' | 'spam';
export const STATUSES: Status[] = ['pending', 'approved', 'spam'];

export interface CommentRow {
  id: number; post_id: number; parent_id: number; name: string; email: string; body: string; status: Status; ip_hash: string; created_at: string;
}

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export interface NewComment { postId: number; parentId: number; name: string; email: string; body: string }

/** Field problems ({} = fine). */
export function check(c: NewComment): Record<string, string> {
  const e: Record<string, string> = {};
  const name = c.name.trim();
  if (!name || name.length > 100) e.name = 'e_name';
  if (c.email.trim() && (!EMAIL.test(c.email.trim()) || c.email.length > 191)) e.email = 'e_email';
  const body = c.body.trim();
  if (body.length < 2 || body.length > 3000) e.body = 'e_body';
  else if ((body.match(/https?:\/\//gi) ?? []).length > 2) e.body = 'e_links';
  return e;
}

const cacheKey = (postId: number) => 'be:comments:' + postId;

/** Approved comments of a post, oldest first (cached an hour; cleared on every change). */
export function approved(postId: number): CommentRow[] {
  const k = cacheKey(postId);
  const hit = cache.get<CommentRow[]>(k);
  if (hit) return hit;
  const rows = db.table('comments').where('post_id', postId).where('status', 'approved').orderBy('id').limit(500)
    .select('id', 'post_id', 'parent_id', 'name', 'body', 'status', 'created_at').get() as CommentRow[];
  cache.set(k, rows, 3600);
  return rows;
}

/** Clears a post's cached comments and the cached part below it (every settings combination: by prefix is not
 *  possible, so every combination of the settings in its key is cleared). */
export const forget = (postId: number) => {
  cache.delete(cacheKey(postId));
  for (const a of [0, 1]) for (let n = 0; n <= 6; n++) for (const c of [0, 1]) for (const l of [0, 1]) cache.delete(`be:bottom:${postId}:${a}:${n}:${c}:${l}`);
};

export function add(c: NewComment, status: Status, ipHash: string): number {
  // a reply goes under a top-level approved comment of the same post (one level)
  let parent = 0;
  if (c.parentId > 0) {
    const p = db.table('comments').where('id', c.parentId).where('post_id', c.postId).where('status', 'approved').first() as CommentRow | null;
    if (p) parent = p.parent_id > 0 ? p.parent_id : p.id;
  }
  const id = db.table('comments').insert({ post_id: c.postId, parent_id: parent, name: c.name.trim(), email: c.email.trim(), body: c.body.trim(), status, ip_hash: ipHash });
  forget(c.postId);
  return id;
}

export const get = (id: number) => db.table('comments').where('id', id).first() as CommentRow | null;

export function setStatus(id: number, status: Status): CommentRow | null {
  const c = get(id);
  if (!c || !STATUSES.includes(status)) return null;
  db.table('comments').where('id', id).update({ status });
  forget(c.post_id);
  return get(id);
}

export function remove(id: number): boolean {
  const c = get(id);
  if (!c) return false;
  db.table('comments').where('id', id).orWhere('parent_id', id).delete();
  forget(c.post_id);
  return true;
}

export function list(opts: { status?: string; page?: number; perPage?: number }) {
  const perPage = Math.min(100, Math.max(1, opts.perPage ?? 30));
  const page = Math.max(1, opts.page ?? 1);
  const q = () => {
    const b = db.table('comments');
    if (opts.status && STATUSES.includes(opts.status as Status)) b.where('status', opts.status);
    return b;
  };
  return { items: q().orderBy('id', 'desc').limit(perPage).offset((page - 1) * perPage).get() as CommentRow[], total: q().count(), page, perPage };
}

export function counts(): Record<string, number> {
  const out: Record<string, number> = {};
  for (const r of db.table('comments').select('status').selectCount('n').groupBy('status').get() as { status: string; n: number }[]) out[r.status] = Number(r.n);
  return out;
}
