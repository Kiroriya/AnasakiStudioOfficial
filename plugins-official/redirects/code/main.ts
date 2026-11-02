// Redirects (official plugin 2.0.0) — the JavaScript port of the built-in Go plugin "redirects":
//  - site.middleware: GET/HEAD requests whose path has a rule are redirected before routing (exact,
//    case-sensitive path; a trailing slash is ignored; the query string is carried over when the target
//    has none; a rule pointing at its own path is ignored)
//  - site.afterResponse: rule hits and the 404 log (top 200 paths, most hit first), written after the
//    response so a visitor never waits for the database
//  - admin page "Redirects": add / edit / delete rules (loop check), the 404 list with "Fix", clear
//  - export legacyImport: the built-in's redirects_rules and redirects_404 rows
import { Rule, normalize, target, findLoop, validate, CODES, EN, clipRunes, esc, one, toDate } from './lib';

/**
 * Who manages the rules. CMS 2.0.6+: the plugin's own capability redirects.manage (manifest "capabilities",
 * default administrators), ticked per role in Users → Roles. Older CMS versions: administrators.
 */
const MANAGE = typeof (site as any).can === 'function' ? ['cap:redirects.manage'] : ['administrator'];

/** How many distinct 404 paths are kept (most hit first). */
const MAX_404 = 200;
const RULES_KEY = 'redirects:rules';
const TRIM_KEY = 'redirects:trimAt';

type Compiled = Record<string, [number, string, number]>; // normalised from → [id, to, code]

const keyOf = (path: string) => crypto.hash('sha256', path);

function allRules(): any[] {
  return db.table('rules').orderBy('id', 'desc').get();
}

/** The rules by normalised path, cached for five seconds (dropped at once when an admin edits them). */
function compiled(): Compiled {
  const c = cache.get<Compiled>(RULES_KEY);
  if (c) return c;
  const m: Compiled = {};
  for (const r of db.table('rules').select('id', 'from_path', 'to_url', 'code').get()) {
    m[normalize(r.from_path)] = [Number(r.id), r.to_url, Number(r.code)];
  }
  cache.set(RULES_KEY, m, 5);
  return m;
}

function lookup(path: string): { id: number; to: string; code: number } | null {
  // keys always start with "/", so no inherited Object property can match
  const hit = compiled()[normalize(path)];
  return Array.isArray(hit) ? { id: hit[0], to: hit[1], code: hit[2] } : null;
}

const invalidate = () => cache.delete(RULES_KEY);

// ---------------------------------------------------------------- public site

site.middleware((ctx) => {
  const req = ctx.request;
  if (req.method !== 'GET' && req.method !== 'HEAD') return;
  const rule = lookup(req.path);
  if (!rule || normalize(rule.to) === normalize(req.path)) return;
  const q = req.url.indexOf('?');
  const raw = q >= 0 ? req.url.slice(q + 1) : '';
  return { status: rule.code, headers: { Location: target(rule.to, raw), 'Cache-Control': 'no-cache' }, body: '' };
}, { priority: 3 });

site.afterResponse((info) => {
  if (info.status === 404 && info.method === 'GET') {
    count404(info.path);
    return;
  }
  if ((info.method === 'GET' || info.method === 'HEAD') && CODES.includes(info.status)) {
    const rule = lookup(info.path);
    if (rule && normalize(rule.to) !== normalize(info.path)) db.table('rules').where('id', rule.id).increment('hits', 1);
  }
});

function count404(path: string) {
  path = clipRunes(path, 191);
  const k = keyOf(path);
  const now = new Date();
  if (db.table('notfound').where('path_key', k).increment('hits', 1, { last_seen: now })) return;
  db.table('notfound').insert({ path, path_key: k, hits: 1, last_seen: now });
  // keep the top MAX_404 paths (checked at most once a minute)
  if (cache.get(TRIM_KEY)) return;
  cache.set(TRIM_KEY, 1, 60);
  if (db.table('notfound').count() <= MAX_404) return;
  const keep = db.table('notfound').orderBy('hits', 'desc').orderBy('last_seen', 'desc').limit(MAX_404).pluck('id');
  db.table('notfound').whereNotIn('id', keep).delete();
}

// ---------------------------------------------------------------- exports

/**
 * Legacy import (phase 7 migration): rows of the built-in tables, upserted by their natural key —
 * redirects_rules {id, from_path, to_url, code, hits, created_at} by the exact from path (the old id is
 * kept when free), redirects_404 {path, hits, last_seen} by the exact path.
 */
site.export('legacyImport', (payload: any) => {
  if (!payload || payload.kind !== 'rows' || !Array.isArray(payload.rows)) return { imported: 0 };
  let n = 0;
  if (payload.table === 'redirects_rules') {
    const rows = payload.rows.filter((r: any) => r && typeof r.from_path === 'string' && r.from_path !== '' && typeof r.to_url === 'string');
    if (!rows.length) return { imported: 0 };
    const keys = rows.map((r: any) => keyOf(r.from_path));
    const have = new Map<string, number>();
    for (const r of db.table('rules').whereIn('from_key', keys).select('id', 'from_key').get()) have.set(r.from_key, Number(r.id));
    const ids = rows.map((r: any) => Number(r.id)).filter((x: number) => x > 0);
    const taken = new Set<number>(ids.length ? db.table('rules').whereIn('id', ids).pluck('id').map(Number) : []);
    const fresh: any[] = [];
    const seen = new Set<string>();
    for (const r of rows) {
      const k = keyOf(r.from_path);
      if (seen.has(k)) continue;
      seen.add(k);
      const code = CODES.includes(Number(r.code)) ? Number(r.code) : 301;
      const row = { from_path: String(r.from_path).slice(0, 191), from_key: k, to_url: String(r.to_url).slice(0, 1000), code,
        hits: Math.max(0, Math.floor(Number(r.hits) || 0)), created_at: toDate(r.created_at) };
      if (have.has(k)) db.table('rules').where('id', have.get(k) as number).update(row);
      else {
        const id = Number(r.id);
        fresh.push(id > 0 && !taken.has(id) ? { id, ...row } : row);
        if (id > 0) taken.add(id);
      }
      n++;
    }
    for (let i = 0; i < fresh.length; i += 500) {
      // rows with and without an explicit id go in separate statements (one column list per insert)
      const part = fresh.slice(i, i + 500);
      const withId = part.filter((r) => r.id), noId = part.filter((r) => !r.id);
      if (withId.length) db.table('rules').insert(withId);
      if (noId.length) db.table('rules').insert(noId);
    }
    invalidate();
  } else if (payload.table === 'redirects_404') {
    const rows = payload.rows.filter((r: any) => r && typeof r.path === 'string' && r.path !== '');
    if (!rows.length) return { imported: 0 };
    const merged = new Map<string, any>();
    for (const r of rows) {
      const path = clipRunes(r.path, 191);
      const k = keyOf(path);
      const hits = Math.max(0, Math.floor(Number(r.hits) || 0));
      const last = toDate(r.last_seen);
      const prev = merged.get(k);
      if (prev) {
        prev.hits = Math.max(prev.hits, hits);
        if (last > prev.last_seen) prev.last_seen = last;
      } else merged.set(k, { path, path_key: k, hits, last_seen: last });
    }
    const have = new Set<string>(db.table('notfound').whereIn('path_key', [...merged.keys()]).pluck('path_key'));
    const fresh: any[] = [];
    for (const [k, row] of merged) {
      // re-running the import must not double the counts: an existing path takes the larger count
      if (have.has(k)) {
        const cur = db.table('notfound').where('path_key', k).first();
        db.table('notfound').where('path_key', k).update({ hits: Math.max(Number(cur?.hits) || 0, row.hits),
          last_seen: cur && toDate(cur.last_seen) > row.last_seen ? toDate(cur.last_seen) : row.last_seen });
      } else fresh.push(row);
      n++;
    }
    if (fresh.length) db.table('notfound').insert(fresh);
  }
  return { imported: n };
});

// ---------------------------------------------------------------- admin

const T = {
  th: {
    title: 'เปลี่ยนเส้นทางลิงก์', desc: 'ย้ายหน้าหรือเปลี่ยนลิงก์แล้ว พาคนที่เข้าลิงก์เก่าไปหน้าใหม่ — และดูว่าลิงก์ไหนเสีย (404)',
    from: 'ลิงก์เดิม', to: 'ไปที่', type: 'ชนิด', c301: 'ย้ายถาวร', c302: 'ชั่วคราว', c307: 'ชั่วคราว (คงเมธอด)', c308: 'ถาวร (คงเมธอด)',
    hits: 'ใช้ไป', hint: 'ตรงกับเส้นทางทั้งหมด (ไม่สนใจ / ท้ายลิงก์) และส่งต่อ ?query ไปยังปลายทาง', empty: 'ยังไม่มีการเปลี่ยนเส้นทาง',
    nf: 'ลิงก์ที่หาไม่เจอ', nfHint: 'ลิงก์ที่มีคนเข้าแล้วไม่พบหน้า กด “แก้” เพื่อพาไปหน้าอื่น', fix: 'แก้', clear: 'ล้างรายการ',
    no404: 'ยังไม่พบลิงก์เสีย 🎉', add: 'เพิ่ม', save: 'บันทึก', cancel: 'ยกเลิก', edit: 'แก้ไข', del: 'ลบ', saved: 'บันทึกแล้ว',
    confirmDel: 'ลบ redirect นี้?', confirmClear: 'ล้างรายการลิงก์เสียทั้งหมด?', prev: 'ก่อนหน้า', next: 'ถัดไป',
  },
  en: {
    title: 'Redirects', desc: 'Moved a page? Send visitors from the old link to the new one — and see which links are broken (404).',
    from: 'From', to: 'To', type: 'Type', c301: 'Permanent', c302: 'Temporary', c307: 'Temporary (keep method)', c308: 'Permanent (keep method)',
    hits: 'Hits', hint: 'Matches the whole path (a trailing / is ignored) and carries ?query over to the target.', empty: 'No redirects yet',
    nf: 'Broken links (404)', nfHint: 'Links visitors hit that do not exist. Click “Fix” to redirect them.', fix: 'Fix', clear: 'Clear',
    no404: 'No broken links 🎉', add: 'Add', save: 'Save', cancel: 'Cancel', edit: 'Edit', del: 'Delete', saved: 'Saved',
    confirmDel: 'Delete this redirect?', confirmClear: 'Clear the whole broken-links list?', prev: 'Previous', next: 'Next',
  },
};
type Lang = keyof typeof T;
const langOf = (ctx: any): Lang => (ctx?.locale === 'en' ? 'en' : 'th');
const num = (v: any, d = 1) => Math.max(1, Math.floor(Number(v) || d));

function pager(param: string, page: number, pages: number, query: Record<string, any>, t: any): string {
  if (pages <= 1) return '';
  const link = (p: number) => '?' + Object.entries({ ...query, [param]: p }).filter(([, v]) => v !== '' && v !== undefined)
    .map(([k, v]) => k + '=' + encodeURIComponent(String(v))).join('&');
  return `<div class="ak-row" style="padding:10px 16px;justify-content:space-between">` +
    (page > 1 ? `<a class="ak-btn ak-btn-sm" href="${link(page - 1)}">‹ ${t.prev}</a>` : '<span></span>') +
    `<span class="ak-muted ak-small">${page} / ${pages}</span>` +
    (page < pages ? `<a class="ak-btn ak-btn-sm" href="${link(page + 1)}">${t.next} ›</a>` : '<span></span>') + '</div>';
}

site.adminPage('redirects', {
  label: { th: 'เปลี่ยนเส้นทาง', en: 'Redirects' },
  icon: '↪️',
  access: MANAGE,
  render: (ctx) => {
    const t = T[langOf(ctx)];
    const q = ctx.query ?? {};
    const rp = num(one(q.rp)), mp = num(one(q.mp));
    const keep = { rp, mp };
    const rules = allRules();
    const missing = db.table('notfound').orderBy('hits', 'desc').orderBy('last_seen', 'desc').limit(MAX_404).get();
    const editId = Math.floor(Number(one(q.edit)) || 0);
    const editing = editId ? rules.find((r) => Number(r.id) === editId) : null;
    const from = editing ? editing.from_path : one(q.from);
    const to = editing ? editing.to_url : one(q.from) ? '/' : '';
    const code = editing ? Number(editing.code) : 301;
    const opts = CODES.map((c) => `<option value="${c}"${c === code ? ' selected' : ''}>${c} · ${(t as any)['c' + c]}</option>`).join('');

    const form = `<section class="ak-card"><form id="rd-form" data-rd="form" class="ak-row" style="align-items:flex-end;flex-wrap:wrap;gap:12px" onsubmit="return rdSave(this)">` +
      `<input type="hidden" name="id" value="${editing ? editing.id : ''}">` +
      `<label class="ak-field" style="flex:1;min-width:180px"><span>${t.from}</span><input name="from" value="${esc(from)}" placeholder="/old-page" required></label>` +
      `<span style="padding-bottom:10px">→</span>` +
      `<label class="ak-field" style="flex:1;min-width:180px"><span>${t.to}</span><input name="to" value="${esc(to)}" placeholder="/new-page" required></label>` +
      `<label class="ak-field"><span>${t.type}</span><select name="code">${opts}</select></label>` +
      `<div class="ak-field"><button class="ak-btn ak-btn-primary" type="submit">${editing ? '💾 ' + t.save : '+ ' + t.add}</button>` +
      (editing ? ` <a class="ak-btn" href="?rp=${rp}&mp=${mp}">${t.cancel}</a>` : '') + `</div></form>` +
      `<p class="ak-hint">${t.hint}</p></section>`;

    const per = 20, rpages = Math.max(1, Math.ceil(rules.length / per)), rpage = Math.min(rp, rpages);
    let rrows = rules.slice((rpage - 1) * per, rpage * per).map((r) =>
      `<tr data-id="${r.id}"><td><code>${esc(r.from_path)}</code></td><td><code>${esc(r.to_url)}</code></td><td><span class="ak-badge">${Number(r.code)}</span></td>` +
      `<td class="ak-num">${Number(r.hits)}</td><td style="text-align:right;white-space:nowrap">` +
      `<a class="ak-btn ak-btn-sm" href="?edit=${r.id}&rp=${rpage}&mp=${mp}">✏️</a> ` +
      `<button class="ak-btn ak-btn-sm ak-btn-danger" type="button" onclick="rdDelete(${Number(r.id)}, ${esc(JSON.stringify(r.from_path))})">🗑</button></td></tr>`).join('');
    if (!rules.length) rrows = `<tr><td colspan="5" class="ak-muted">${t.empty}</td></tr>`;

    const mper = 10, mpages = Math.max(1, Math.ceil(missing.length / mper)), mpage = Math.min(mp, mpages);
    let mrows = missing.slice((mpage - 1) * mper, mpage * mper).map((m) =>
      `<li data-path="${esc(m.path)}" class="ak-row" style="justify-content:space-between;gap:8px;padding:6px 0;border-bottom:1px solid var(--ak-border,#e5e7eb)">` +
      `<code title="${esc(m.path)}" style="overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:60%">${esc(m.path)}</code>` +
      `<span class="ak-muted">×${Number(m.hits)}</span><a href="?from=${encodeURIComponent(m.path)}&rp=${rpage}&mp=${mpage}">${t.fix}</a></li>`).join('');
    if (!missing.length) mrows = `<li class="ak-muted">${t.no404}</li>`;

    const cfg = { confirmDel: t.confirmDel, confirmClear: t.confirmClear, saved: t.saved, back: `?rp=${rpage}&mp=${mpage}` };
    const script = `<script>var RD=${JSON.stringify(cfg).replace(/</g, '\\u003c')};
function rdMsg(m,k){window.AK?AK.toast(m,k):alert(m)}
function rdDone(r){if(r.errors&&Object.keys(r.errors).length){rdMsg(r.message||Object.values(r.errors).join(', '),'error');return false}return true}
function rdSave(form){var f=form.elements;var id=f.namedItem('id').value;anasaki.call('redirects:save',{id:id?Number(id):null,from:f.namedItem('from').value,to:f.namedItem('to').value,code:Number(f.namedItem('code').value)})
.then(function(r){if(rdDone(r)){rdMsg(RD.saved,'success');setTimeout(function(){location.href=RD.back},300)}},function(e){rdMsg(e.message,'error')});return false}
function ask(text){return window.AK?AK.confirm(text,{danger:true}):Promise.resolve(confirm(text))}
function rdDelete(id,from){ask(RD.confirmDel+' '+from).then(function(ok){if(!ok)return;anasaki.call('redirects:delete',{id:id}).then(function(r){if(rdDone(r))location.reload()},function(e){rdMsg(e.message,'error')})})}
function rdClear(){ask(RD.confirmClear).then(function(ok){if(!ok)return;anasaki.call('redirects:clear404',{}).then(function(r){if(rdDone(r))location.href=RD.back.replace(/mp=\\d+/,'mp=1')},function(e){rdMsg(e.message,'error')})})}
</script>`;

    const html = `<header class="ak-page-head"><span class="ak-page-icon">↪️</span><div class="ak-page-head-text"><h1>${t.title}</h1><p>${t.desc}</p></div></header>` +
      form +
      `<div style="display:grid;grid-template-columns:minmax(0,1fr) 340px;gap:16px;align-items:start">` +
      `<section class="ak-card ak-card-flush"><table class="ak-table" data-rd="rules"><thead><tr><th>${t.from}</th><th>${t.to}</th><th>${t.type}</th>` +
      `<th class="ak-num">${t.hits}</th><th></th></tr></thead><tbody>${rrows}</tbody></table>${pager('rp', rpage, rpages, keep, t)}</section>` +
      `<section class="ak-card"><div class="ak-card-head"><h2>🚫 ${t.nf}</h2>` +
      (missing.length ? `<button class="ak-btn ak-btn-sm ak-btn-ghost" type="button" data-rd="clear" onclick="rdClear()">${t.clear}</button>` : '') +
      `</div><p class="ak-hint">${t.nfHint}</p><ul data-rd="404" style="list-style:none;margin:0;padding:0">${mrows}</ul>${pager('mp', mpage, mpages, keep, t)}</section></div>` +
      script;
    return { title: t.title, html };
  },
});

function fail(ctx: any, msg: string, extra = '') {
  const m = (ctx?.locale === 'en' ? EN[msg] ?? msg : msg) + extra;
  return { errors: { form: m }, message: m };
}

/** Create (no id) or update a rule — the built-in's POST /rules and PUT /rules/{id}. */
site.action('save', {
  input: { id: 'number?', from: 'text', to: 'text', code: 'number?' },
  access: MANAGE,
  run: (ctx) => {
    const { input } = ctx;
    const id = Math.floor(Number(input.id) || 0);
    if (id && !db.table('rules').where('id', id).exists()) return fail(ctx, 'ไม่พบ redirect นี้');
    const rule: Rule = { id: id || undefined, from: input.from, to: input.to, code: Math.floor(Number(input.code) || 0) };
    const err = validate(rule);
    if (err) return fail(ctx, err);
    const list = allRules().map((r) => ({ id: Number(r.id), from: r.from_path, to: r.to_url }));
    const loop = findLoop(list, rule);
    if (loop) return fail(ctx, 'redirect นี้ทำให้เกิดการวนซ้ำไม่รู้จบ: ', loop.join(' → '));
    const k = keyOf(rule.from);
    const dup = db.table('rules').where('from_key', k).first();
    if (dup && Number(dup.id) !== id) return fail(ctx, 'มี redirect สำหรับเส้นทางนี้อยู่แล้ว');
    let rid = id;
    if (id) db.table('rules').where('id', id).update({ from_path: rule.from, from_key: k, to_url: rule.to, code: rule.code });
    else rid = db.table('rules').insert({ from_path: rule.from, from_key: k, to_url: rule.to, code: rule.code, hits: 0, created_at: new Date() });
    invalidate();
    const r = db.table('rules').where('id', rid).first();
    return {
      message: ctx.locale === 'en' ? 'Saved' : 'บันทึกแล้ว',
      data: { id: Number(r.id), from: r.from_path, to: r.to_url, code: Number(r.code), hits: Number(r.hits), createdAt: r.created_at },
    };
  },
});

site.action('delete', {
  input: { id: 'number' },
  access: MANAGE,
  run: (ctx) => {
    const n = db.table('rules').where('id', Math.floor(Number(ctx.input.id) || 0)).delete();
    invalidate();
    return { message: ctx.locale === 'en' ? 'Deleted' : 'ลบแล้ว', data: { deleted: n } };
  },
});

site.action('clear404', {
  access: MANAGE,
  run: (ctx) => {
    db.table('notfound').all().delete();
    return { message: ctx.locale === 'en' ? 'Cleared' : 'ล้างแล้ว', data: { cleared: true } };
  },
});

/** The rules and the 404 log as JSON (the built-in's GET /rules and GET /404s), for scripts and tests. */
site.action('list', {
  access: MANAGE,
  run: () => ({
    data: {
      rules: allRules().map((r) => ({ id: Number(r.id), from: r.from_path, to: r.to_url, code: Number(r.code), hits: Number(r.hits), createdAt: r.created_at })),
      notFound: db.table('notfound').orderBy('hits', 'desc').orderBy('last_seen', 'desc').limit(MAX_404).get()
        .map((m) => ({ path: m.path, hits: Number(m.hits), lastSeen: m.last_seen })),
    },
  }),
});
