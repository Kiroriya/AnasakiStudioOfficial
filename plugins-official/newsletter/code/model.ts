// Subscribers and campaigns.

export type SubStatus = 'pending' | 'active' | 'unsubscribed';
export interface Subscriber { id: number; email: string; name: string; status: SubStatus; token: string; source: string; confirmed_at: string | null; created_at: string }
export interface Campaign {
  id: number; subject: string; body: string; status: 'draft' | 'sending' | 'sent'; cursor: number; sent: number; failed: number; total: number;
  started_at: string | null; finished_at: string | null; created_at: string;
}

export const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);

export const normEmail = (s: unknown) => String(s ?? '').trim().toLowerCase();
export const validEmail = (e: string) => /^[^\s@,;<>]+@[^\s@,;<>]+\.[^\s@,;<>]+$/.test(e) && e.length <= 191;

export const byToken = (token: string): Subscriber | null =>
  /^[0-9a-f]{48}$/.test(token) ? (db.table('subscribers').where('token', token).first() as Subscriber | null) : null;
export const byEmail = (email: string): Subscriber | null => db.table('subscribers').where('email', normEmail(email)).first() as Subscriber | null;

/**
 * Adds (or re-activates) a subscriber. Answers the row and whether a confirmation mail is needed. An active
 * subscriber stays as is (no mail, no hint that the address was already there).
 */
export function upsert(email: string, name: string, confirmed: boolean, source: string): { sub: Subscriber; needsConfirm: boolean; isNew: boolean } {
  const e = normEmail(email);
  const n = String(name ?? '').trim().slice(0, 100);
  const old = byEmail(e);
  if (old) {
    if (old.status === 'active') return { sub: old, needsConfirm: false, isNew: false };
    const status: SubStatus = confirmed ? 'active' : 'pending';
    db.table('subscribers').where('id', old.id).update({ status, name: n || old.name, ...(confirmed ? { confirmed_at: new Date() } : {}) });
    return { sub: byEmail(e) as Subscriber, needsConfirm: !confirmed, isNew: false };
  }
  db.table('subscribers').insert({ email: e, name: n, status: confirmed ? 'active' : 'pending', token: crypto.random(24), source: source.slice(0, 40),
    ...(confirmed ? { confirmed_at: new Date() } : {}) });
  return { sub: byEmail(e) as Subscriber, needsConfirm: !confirmed, isNew: true };
}

export function confirm(s: Subscriber): Subscriber {
  if (s.status !== 'active') db.table('subscribers').where('id', s.id).update({ status: 'active', confirmed_at: new Date() });
  return byEmail(s.email) as Subscriber;
}

export function unsubscribe(s: Subscriber): void {
  db.table('subscribers').where('id', s.id).update({ status: 'unsubscribed' });
}

export function counts(): Record<string, number> {
  const out: Record<string, number> = { pending: 0, active: 0, unsubscribed: 0 };
  for (const r of db.table('subscribers').select('status').selectCount('n').groupBy('status').get() as { status: string; n: number }[]) out[r.status] = Number(r.n);
  return out;
}

export function listSubscribers(opts: { status?: string; search?: string; page?: number; perPage?: number }) {
  const perPage = Math.min(100, Math.max(1, opts.perPage ?? 50));
  const page = Math.max(1, opts.page ?? 1);
  const q = () => {
    const b = db.table('subscribers');
    if (opts.status && ['pending', 'active', 'unsubscribed'].includes(opts.status)) b.where('status', opts.status);
    const s = String(opts.search ?? '').trim().slice(0, 100);
    if (s) b.where((w) => { w.whereLike('email', '%' + s.replace(/[\\%_]/g, (c) => '\\' + c) + '%').orWhereLike('name', '%' + s.replace(/[\\%_]/g, (c) => '\\' + c) + '%'); });
    return b;
  };
  return { items: q().orderBy('id', 'desc').limit(perPage).offset((page - 1) * perPage).get() as Subscriber[], total: q().count(), page, perPage };
}

export const getCampaign = (id: number) => db.table('campaigns').where('id', id).first() as Campaign | null;
export const campaigns = () => db.table('campaigns').orderBy('id', 'desc').limit(100).get() as Campaign[];

/** Plain text (blank line = new paragraph, links made clickable) → safe HTML. */
export function bodyHTML(text: string): string {
  return String(text ?? '').trim().split(/\n\s*\n/).map((para) =>
    '<p>' + esc(para).replace(/\n/g, '<br>').replace(/(https?:\/\/[^\s<&]+)/g, '<a href="$1">$1</a>') + '</p>').join('');
}
