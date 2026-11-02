// The SEO admin page (was the Angular page /seo): search-result previews of published pages and posts, the
// per-page editor (post meta "seo.*", also editable in the post editor's SEO panel) and the site-wide defaults
// (saved with site.saveSettings — the same values as Plugins → SEO → Settings). Administrators by default; on
// CMS 2.0.6+ the roles holding seo.manage / seo.edit_meta (Users → Roles).
import { applyTemplate, byteLen, esc, one, safeURL } from './lib';
import { EMPTY, Meta, SETTING_KEYS, SettingKey, metaFrom, metaOfPost, saveMeta, setting, siteInfo } from './meta';

const PER_PAGE = 15;

// Who may use the SEO page. CMS 2.0.6+: the plugin's own capabilities (manifest "capabilities"), ticked per
// role in Users → Roles — seo.manage (the page and the site-wide defaults) and seo.edit_meta (per-page
// overrides); both default to administrators. Older CMS versions keep the former role list.
const CAPS = typeof (site as any).can === 'function';
const PAGE_ACCESS = CAPS ? ['cap:seo.manage', 'cap:seo.edit_meta'] : ['administrator'];
const MANAGE_ACCESS = CAPS ? ['cap:seo.manage'] : ['administrator'];
const EDIT_META_ACCESS = CAPS ? ['cap:seo.edit_meta'] : ['administrator'];
const canManage = (user: any) => !CAPS || (!!user && user.can('seo.manage'));
const canEditMeta = (user: any) => !CAPS || (!!user && user.can('seo.edit_meta'));

/** A page or post by id (db.posts.get looks up one type at a time). */
function postById(id: number): any {
  if (!(id > 0)) return null;
  return db.posts.get(id, { type: 'page' }) ?? db.posts.get(id, { type: 'post' });
}

/** Published pages then posts, the slice [from, from+count) plus whether more exist. */
function published(from: number, count: number): { items: any[]; more: boolean } {
  const out: any[] = [];
  let seen = 0;
  for (const type of ['page', 'post']) {
    for (let p = 1; p <= 200; p++) {
      const res = db.posts.list({ type, perPage: 100, page: p, order: 'newest' });
      for (const it of res.items) {
        if (it.status !== 'publish') continue;
        if (seen >= from && out.length < count) out.push(it);
        seen++;
        if (out.length === count && seen > from + count) return { items: out, more: true };
      }
      if (res.items.length < 100) break;
    }
  }
  return { items: out, more: false };
}

const T = {
  th: {
    desc: 'ช่วยให้ Google เข้าใจเว็บของคุณ — ตั้งชื่อและคำอธิบายที่จะแสดงในผลการค้นหา',
    pages: 'หน้าตาในผลการค้นหา', pagesHint: 'หน้าและบทความที่เผยแพร่แล้ว', preview: 'ตัวอย่างบน Google',
    noDesc: 'ยังไม่มีคำอธิบาย — Google จะเลือกข้อความจากหน้าเอง', defaults: 'ค่าเริ่มต้นทั้งเว็บ',
    title: 'ชื่อในผลการค้นหา', description: 'คำอธิบาย', image: 'รูปตอนแชร์', imageHint: 'แสดงตอนแชร์ลิงก์บน Facebook/LINE',
    noindex: 'ไม่ให้ Google แสดงหน้านี้ (noindex)', edit: 'แก้ไข', save: 'บันทึก', cancel: 'ยกเลิก', saved: 'บันทึกแล้ว',
    none: 'ไม่มีรายการ', prev: 'ก่อนหน้า', next: 'ถัดไป', notFound: 'ไม่พบรายการนี้',
    settingsHint: 'ค่าเดียวกับหน้า ปลั๊กอิน → SEO → ตั้งค่า · ปรับ SEO รายหน้าได้ในหน้าแก้ไขโพสต์ด้วย (แผง SEO)',
    tplHint: 'ใช้ %title% %site% %tagline%',
    titleTemplate: 'รูปแบบชื่อหน้า', defaultDescription: 'คำอธิบายเริ่มต้น', defaultImage: 'รูปแชร์เริ่มต้น (og:image)', twitterHandle: 'บัญชี X/Twitter',
    chooseImage: 'เลือกภาพ', removeImage: 'นำออก',
  },
  en: {
    desc: 'Help search engines understand your site — set the title and description shown in results.',
    pages: 'Search result previews', pagesHint: 'Published pages and posts', preview: 'Google preview',
    noDesc: 'No description — Google will pick text from the page', defaults: 'Site-wide defaults',
    title: 'Search title', description: 'Meta description', image: 'Share image', imageHint: 'Shown when the link is shared on social apps',
    noindex: 'Hide this page from search engines (noindex)', edit: 'Edit', save: 'Save', cancel: 'Cancel', saved: 'Saved',
    none: 'Nothing here', prev: 'Previous', next: 'Next', notFound: 'Not found',
    settingsHint: 'The same values as Plugins → SEO → Settings · per-page SEO is also in the post editor (SEO panel)',
    tplHint: 'Use %title% %site% %tagline%',
    titleTemplate: 'Title template', defaultDescription: 'Default description', defaultImage: 'Default share image', twitterHandle: 'Twitter handle',
    chooseImage: 'Choose image', removeImage: 'Remove',
  },
};
type Lang = keyof typeof T;
const langOf = (ctx: any): Lang => (ctx?.locale === 'en' ? 'en' : 'th');

const STYLE = `<style>
.serp{display:flex;flex-direction:column;gap:2px;max-width:600px}
.serp-url{color:#188038;font-size:13px}.serp-title{color:#1a0dab;font-size:18px;line-height:1.3}
.serp-desc{color:var(--ak-muted,#4d5156);font-size:13px}.serp-big{padding:12px;border:1px solid var(--ak-border,#e5e7eb);border-radius:10px;margin-bottom:16px}
.over{color:#b91c1c}.seo-grid{display:grid;grid-template-columns:minmax(0,1fr) 340px;gap:16px;align-items:start}
@media (max-width:900px){.seo-grid{grid-template-columns:1fr}.seo-defaults{order:-1}}
.seo-img-row{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.seo-thumb{display:grid;place-items:center;width:72px;height:72px;border:1px dashed var(--ak-border-strong,#d1d5db);border-radius:8px;overflow:hidden;font-size:24px;background:var(--ak-surface-2,#f8fafc)}
.seo-thumb img{width:100%;height:100%;object-fit:cover}.seo-block{width:100%}
.seo-img .seo-img-url{flex:1;min-width:180px;width:auto}.seo-img [hidden]{display:none!important}
</style>`;

/**
 * An image field: thumbnail + the URL in a text input called `name` (ids: `<id>`, `<id>-preview`, `<id>-pick`,
 * `<id>-remove`). On CMS with AK.pickMedia in the admin UI kit (the admin's media library) IMG_SCRIPT swaps the URL
 * input for "Choose image" + "Remove"; on older CMS the URL input stays (never a dead button). The buttons are
 * rendered hidden, so the page also works before / without the script.
 */
function imageField(name: string, id: string, label: string, value: string, t: any, attrs = '', hint = ''): string {
  return `<div class="ak-field seo-img" data-seo-image><label class="ak-label" for="${id}">${label}</label>` +
    `<div class="seo-img-row"><span class="seo-thumb" id="${id}-preview">${value ? `<img src="${esc(value)}" alt="">` : '🖼'}</span>` +
    `<input type="text" inputmode="url" class="seo-img-url" name="${name}" id="${id}" value="${esc(value)}" maxlength="512" placeholder="https://…"${attrs}>` +
    `<button type="button" class="ak-btn ak-btn-sm" id="${id}-pick" data-seo-pick hidden>${t.chooseImage}</button>` +
    `<button type="button" class="ak-btn ak-btn-sm ak-btn-danger" id="${id}-remove" data-seo-remove hidden>${t.removeImage}</button></div>` +
    (hint ? `<p class="ak-hint">${hint}</p>` : '') + `</div>`;
}

/**
 * Wires every imageField once the kit has run (it is deferred, this inline script is not): with AK.pickMedia
 * (CMS with the media picker) the URL input is hidden and the buttons pick / remove; without it the URL input stays
 * and typing redraws the thumbnail. Every change fires "input" on the form (the defaults card's save button).
 */
const IMG_SCRIPT = `<script>(function(){function wire(){document.querySelectorAll('[data-seo-image]').forEach(function(box){
if(box.getAttribute('data-wired'))return;box.setAttribute('data-wired','1');
var inp=box.querySelector('.seo-img-url'),pv=box.querySelector('.seo-thumb'),pk=box.querySelector('[data-seo-pick]'),rm=box.querySelector('[data-seo-remove]'),
pick=!!(window.AK&&typeof AK.pickMedia==='function');
function thumb(u){pv.textContent='';if(u){var i=document.createElement('img');i.src=u;i.alt='';pv.appendChild(i)}else pv.textContent='🖼'}
function set(u){inp.value=u;thumb(u);rm.hidden=!u;inp.dispatchEvent(new Event('input',{bubbles:true}))}
box.setAttribute('data-seo-mode',pick?'picker':'url');
if(!pick){inp.addEventListener('input',function(){thumb(inp.value.trim())});return}
inp.hidden=true;pk.hidden=false;rm.hidden=!inp.value;
pk.addEventListener('click',function(){AK.pickMedia({accept:'image'}).then(function(m){if(m&&m.url)set(m.url)})});
rm.addEventListener('click',function(){set('')})})}
document.readyState==='loading'?document.addEventListener('DOMContentLoaded',wire):wire()})()</script>`;

const json = (v: unknown) => JSON.stringify(v).replace(/</g, '\\u003c');

function serp(host: string, p: any, m: Meta, info: { title: string; tagline: string }, tpl: string, defDesc: string, t: any): string {
  const title = applyTemplate(tpl, m.title || p.title, info.title, info.tagline);
  const desc = m.description || p.excerpt || defDesc;
  return `<span class="serp-url">${esc(host + p.url)}</span><span class="serp-title" data-seo="serp-title">${esc(title)}</span>` +
    `<span class="serp-desc" data-seo="serp-desc">${esc(desc || t.noDesc)}</span>`;
}

/** The site address for previews: site.info().url, else the admin request's host. */
function hostOf(ctx: any): string {
  const u = site.info().url;
  if (u) return u;
  const h = ctx?.request?.headers ?? {};
  const host = String(h['x-forwarded-host'] || h['host'] || '');
  if (!host) return '';
  const proto = String(h['x-forwarded-proto'] || 'https').split(',')[0].trim();
  return proto + '://' + host;
}

function defaultsForm(t: any): string {
  const field = (k: SettingKey, input: string) => `<label class="ak-field"><span>${t[k]}</span>${input}</label>`;
  const v = (k: SettingKey) => esc(String(site.settings(k) ?? ''));
  return `<form id="seo-defaults" data-seo="defaults" onsubmit="return seoDefaults(this)">` +
    field('titleTemplate', `<input name="titleTemplate" data-seo-setting="titleTemplate" maxlength="2000" value="${esc(setting('titleTemplate'))}"><p class="ak-hint">${t.tplHint}</p>`) +
    field('defaultDescription', `<textarea name="defaultDescription" data-seo-setting="defaultDescription" rows="3" maxlength="500">${v('defaultDescription')}</textarea>`) +
    imageField('defaultImage', 'seo-default-image', t.defaultImage, String(site.settings('defaultImage') ?? ''), t, ' data-seo-setting="defaultImage"') +
    field('twitterHandle', `<input name="twitterHandle" data-seo-setting="twitterHandle" maxlength="40" value="${v('twitterHandle')}" placeholder="@yourbrand">`) +
    `<button class="ak-btn ak-btn-primary seo-block" id="seo-defaults-save" type="submit" disabled>💾 ${t.save}</button></form>` +
    `<p class="ak-hint">${t.settingsHint}</p>` + IMG_SCRIPT +
    `<script>(function(){var f=document.getElementById('seo-defaults'),b=document.getElementById('seo-defaults-save'),k=${json(SETTING_KEYS)};` +
    `function cur(){return JSON.stringify(k.map(function(n){return f.elements.namedItem(n).value}))}var start=cur();` +
    `function upd(){b.disabled=cur()===start}f.addEventListener('input',upd);f.addEventListener('change',upd)})();` +
    `function seoDefaults(form){if(document.getElementById('seo-defaults-save').disabled)return false;var f=form.elements,v={};${json(SETTING_KEYS)}.forEach(function(k){v[k]=f.namedItem(k).value});` +
    `anasaki.call('seo:saveDefaults',v).then(function(r){if(r.errors&&Object.keys(r.errors).length){var m=r.message||Object.values(r.errors).join(', ');window.AK?AK.toast(m,'error'):alert(m);return}` +
    `window.AK&&AK.toast(${json(t.saved)},'success');setTimeout(function(){location.reload()},400)},function(e){window.AK?AK.toast(e.message,'error'):alert(e.message)});return false}</script>`;
}

function listView(ctx: any, t: any): string {
  const page = Math.max(1, Math.floor(Number(one(ctx.query?.page)) || 1));
  const { items, more } = published((page - 1) * PER_PAGE, PER_PAGE);
  const info = siteInfo();
  const tpl = setting('titleTemplate'), defDesc = setting('defaultDescription');
  const host = hostOf(ctx);
  let rows = '';
  for (const p of items) {
    const m = metaFrom(p.meta, 'seo.');
    rows += `<tr data-id="${p.id}"><td><div class="serp">${serp(host, p, m, info, tpl, defDesc, t)}</div>` +
      (m.noindex ? ' <span class="ak-badge ak-badge-warn">noindex</span>' : '') +
      `</td><td style="text-align:right;white-space:nowrap">` +
      (canEditMeta(ctx.user) ? `<a class="ak-btn ak-btn-sm" href="?edit=${p.id}&page=${page}">✏️ ${t.edit}</a>` : '') + `</td></tr>`;
  }
  if (!items.length) rows = `<tr><td colspan="2" class="ak-muted">${t.none}</td></tr>`;
  const pager = page > 1 || more
    ? `<div class="ak-row" style="padding:12px 16px;justify-content:space-between">` +
      (page > 1 ? `<a class="ak-btn ak-btn-sm" href="?page=${page - 1}">‹ ${t.prev}</a>` : '<span></span>') +
      `<span class="ak-muted ak-small">${page}</span>` +
      (more ? `<a class="ak-btn ak-btn-sm" href="?page=${page + 1}">${t.next} ›</a>` : '<span></span>') + `</div>`
    : '';
  return `<div class="seo-grid"><section class="ak-card ak-card-flush"><div class="ak-card-head" style="padding:12px 16px"><h2>${t.pages}</h2>` +
    `<span class="ak-muted ak-small">${t.pagesHint}</span></div><table class="ak-table" data-seo="list"><thead><tr><th>${t.preview}</th><th></th></tr></thead>` +
    `<tbody>${rows}</tbody></table>${pager}</section>` +
    (canManage(ctx.user) ? `<section class="ak-card seo-defaults"><div class="ak-card-head"><h2>⚙️ ${t.defaults}</h2></div>${defaultsForm(t)}</section>` : '') +
    `</div>`;
}

function editView(ctx: any, t: any, id: number): string {
  const p = postById(id);
  const back = `?page=${Math.max(1, Math.floor(Number(one(ctx.query?.page)) || 1))}`;
  if (!p) return `<div class="ak-alert ak-alert-danger">${t.notFound}</div><p><a class="ak-btn" href="${back}">${t.cancel}</a></p>`;
  const m = metaOfPost(p.id) ?? EMPTY;
  const info = siteInfo();
  const tpl = setting('titleTemplate'), defDesc = setting('defaultDescription');
  const cfg = { id: p.id, tpl, site: info.title, tagline: info.tagline, postTitle: p.title, excerpt: p.excerpt ?? '', defDesc, noDesc: t.noDesc, saved: t.saved, back };
  return `<div class="ak-card"><div class="ak-card-head"><h2>🔍 ${esc(p.title)}</h2></div>` +
    `<div class="serp serp-big">${serp(hostOf(ctx), p, m, info, tpl, defDesc, t)}</div>` +
    `<form id="seo-form" data-seo="form" onsubmit="return seoSave(this)">` +
    `<label class="ak-field"><span>${t.title} <span class="ak-muted ak-small" data-count="title">${[...m.title].length}/60</span></span>` +
    `<input name="title" maxlength="255" value="${esc(m.title)}" placeholder="${esc(p.title)}"></label>` +
    `<label class="ak-field"><span>${t.description} <span class="ak-muted ak-small" data-count="description">${[...m.description].length}/160</span></span>` +
    `<textarea name="description" rows="3">${esc(m.description)}</textarea></label>` +
    imageField('image', 'seo-image', t.image, m.image, t, '', t.imageHint) +
    `<label class="ak-check"><input type="checkbox" name="noindex"${m.noindex ? ' checked' : ''}> ${t.noindex}</label>` +
    `<div class="ak-form-actions"><a class="ak-btn" href="${back}">${t.cancel}</a><button class="ak-btn ak-btn-primary" type="submit">${t.save}</button></div></form></div>` +
    IMG_SCRIPT + `<script>var SEO=${json(cfg)};
function seoTpl(title){var s=SEO.site;if(!title||title===s)return s||title;var tpl=(SEO.tpl||'').trim()?SEO.tpl:'%title% – %site%';
var map={'%title%':title,'%site%':s,'%tagline%':SEO.tagline};var out=tpl.replace(/%title%|%site%|%tagline%/g,function(m){return map[m]});
out=out.trim().replace(/^[–\\-|·]+|[–\\-|·]+$/g,'').trim();return out||title}
function seoUpd(){var f=document.getElementById('seo-form').elements;var ti=f.namedItem('title').value,de=f.namedItem('description').value;
document.querySelector('[data-seo="serp-title"]').textContent=seoTpl(ti||SEO.postTitle);
document.querySelector('[data-seo="serp-desc"]').textContent=de||SEO.excerpt||SEO.defDesc||SEO.noDesc;
var c=document.querySelector('[data-count="title"]');c.textContent=Array.from(ti).length+'/60';
var d=document.querySelector('[data-count="description"]');d.textContent=Array.from(de).length+'/160';d.classList.toggle('over',Array.from(de).length>160)}
document.getElementById('seo-form').addEventListener('input',seoUpd);
function seoSave(form){var f=form.elements;anasaki.call('seo:saveMeta',{postId:SEO.id,title:f.namedItem('title').value,description:f.namedItem('description').value,image:f.namedItem('image').value,noindex:f.namedItem('noindex').checked}).then(function(r){
if(r.errors&&Object.keys(r.errors).length){var m=r.message||Object.values(r.errors).join(', ');window.AK?AK.toast(m,'error'):alert(m);return}
window.AK&&AK.toast(SEO.saved,'success');setTimeout(function(){location.href=SEO.back},400)},function(e){window.AK?AK.toast(e.message,'error'):alert(e.message)});return false}
</script>`;
}

export function registerAdmin(): void {
  site.adminPage('seo', {
    label: { th: 'SEO', en: 'SEO' },
    icon: '🔍',
    access: PAGE_ACCESS,
    render: (ctx) => {
      const t = T[langOf(ctx)];
      const edit = Math.floor(Number(one(ctx.query?.edit)) || 0);
      const head = `<header class="ak-page-head"><span class="ak-page-icon">🔍</span><div class="ak-page-head-text"><h1>SEO</h1><p>${t.desc}</p></div>` +
        `<div class="ak-actions"><a class="ak-btn ak-btn-sm" href="/sitemap.xml" target="_blank" rel="noopener">🗺 sitemap.xml</a>` +
        `<a class="ak-btn ak-btn-sm" href="/robots.txt" target="_blank" rel="noopener">🤖 robots.txt</a></div></header>`;
      return { title: 'SEO', html: STYLE + head + (edit && canEditMeta(ctx.user) ? editView(ctx, t, edit) : listView(ctx, t)) };
    },
  });

  /** Saves a page's overrides (same validation and messages as the built-in PUT /seo/meta/{postId}). */
  site.action('saveMeta', {
    input: { postId: 'number', title: 'text?', description: 'text?', image: 'text?', noindex: 'bool' },
    access: EDIT_META_ACCESS,
    run: (ctx) => {
      const { input, user } = ctx;
      const en = (ctx as any).locale === 'en';
      const fail = (th: string, e: string) => ({ errors: { form: en ? e : th }, message: en ? e : th });
      const p = postById(Number(input.postId));
      if (!p) return fail('ไม่พบรายการนี้', 'Not found');
      const allowed = p.type === 'post'
        ? !!user && (user.can('edit_others_posts') || user.can('edit_own_posts'))
        : !!user && user.can('edit_pages');
      if (!allowed) return fail('คุณไม่มีสิทธิ์แก้ไขรายการนี้', 'You may not edit this item');
      const m: Meta = {
        title: String(input.title ?? '').trim(),
        description: String(input.description ?? '').trim(),
        image: String(input.image ?? '').trim(),
        noindex: !!input.noindex,
      };
      if (byteLen(m.title) > 255 || byteLen(m.description) > 2000 || byteLen(m.image) > 512) return fail('ข้อความยาวเกินไป', 'Text is too long');
      if (m.image && !safeURL(m.image)) return fail('ลิงก์รูปภาพไม่ถูกต้อง', 'Invalid image link');
      saveMeta(p.id, m);
      return { message: en ? 'Saved' : 'บันทึกแล้ว', data: m };
    },
  });

  /** Saves the site-wide defaults (the plugin's settings; validated by core against the manifest). */
  site.action('saveDefaults', {
    input: { titleTemplate: 'text?', defaultDescription: 'text?', defaultImage: 'text?', twitterHandle: 'text?' },
    access: MANAGE_ACCESS,
    run: (ctx) => {
      const en = (ctx as any).locale === 'en';
      const v: Record<string, string> = {};
      for (const k of SETTING_KEYS) v[k] = String((ctx.input as any)[k] ?? '').trim();
      if (v.defaultImage && !safeURL(v.defaultImage)) {
        const msg = en ? 'Invalid image link' : 'ลิงก์รูปภาพไม่ถูกต้อง';
        return { errors: { form: msg }, message: msg };
      }
      try {
        return { message: en ? 'Saved' : 'บันทึกแล้ว', data: site.saveSettings(v) };
      } catch (e) {
        const msg = String((e as Error)?.message ?? e);
        return { errors: { form: msg }, message: msg };
      }
    },
  });
}
