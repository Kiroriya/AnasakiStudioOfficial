// The "Docs" admin page: every doc grouped by product and section (status, edit link; drag to reorder docs and
// sections, move a doc to another section, rename a section), the settings form (hub texts, edit link,
// products, redirects) and the JSON bundle import ({products?, hubTitle?, hubIntro?, docs:[{slug,title,excerpt,
// product,section,order,sectionOrder,content}], legacyRedirects?} — upsert by slug). The page is drawn on the
// server; assets/admin.js (admin UI kit) adds the drag and drop, the forms and the import through the actions.
import { DOC_LANGS, docFromPost, editHref, esc, jsonData, Lang, parseProducts, parseRedirects, sectionsOf, SLUG_RE, tr } from './lib';
import { TYPE, docURL, invalidate, loadAll, products, setting } from './model';

// The "Docs hub" page and the bundle import: CMS 2.0.6+ the roles holding docs.manage (default administrators
// and editors; Users → Roles), older CMS versions editors. Saving the settings also needs manage_options.
const ACCESS = typeof (site as any).can === 'function' ? ['cap:docs.manage'] : ['editor'];
const MAX_BATCH = 25;
const MAX_CONTENT = 400_000;

const T = {
  th: {
    title: 'เอกสาร', desc: 'จัดการศูนย์รวมเอกสารที่ /docs — บทความแก้ไขได้ในเมนู เอกสาร ของหลังบ้าน (ตัวแก้ไขเนื้อหาปกติ)',
    view: 'ดูหน้าเอกสาร', newDoc: 'เพิ่มบทความ', docs: 'บทความ', settings: 'ตั้งค่า', importTab: 'นำเข้า',
    colTitle: 'ชื่อ', colSection: 'หมวด', colOrder: 'ลำดับ', colStatus: 'สถานะ', colUpdated: 'แก้ไขล่าสุด',
    edit: 'แก้ไข', open: 'เปิด', draft: 'ฉบับร่าง', published: 'เผยแพร่', none: 'ยังไม่มีบทความ — นำเข้าชุดเอกสาร หรือเพิ่มบทความใหม่',
    orphan: 'ไม่ได้ระบุผลิตภัณฑ์ที่มีอยู่', orphanHint: 'บทความเหล่านี้ไม่แสดงบนเว็บจนกว่าจะตั้งผลิตภัณฑ์ให้ตรงกับรหัสในการตั้งค่า',
    count: (n: number) => `${n} บทความ`,
    dragHint: 'ลาก ⠿ เพื่อเรียงบทความหรือหมวด หรือย้ายบทความไปหมวดอื่น แล้วกด บันทึกลำดับ', moveUp: 'เลื่อนขึ้น', moveDown: 'เลื่อนลง',
  },
  en: {
    title: 'Docs', desc: 'Manage the documentation hub at /docs — articles are edited under Docs in the admin menu (the normal editor).',
    view: 'View the docs', newDoc: 'New article', docs: 'Articles', settings: 'Settings', importTab: 'Import',
    colTitle: 'Title', colSection: 'Section', colOrder: 'Order', colStatus: 'Status', colUpdated: 'Updated',
    edit: 'Edit', open: 'Open', draft: 'Draft', published: 'Published', none: 'No articles yet — import a bundle or add an article',
    orphan: 'Without a known product', orphanHint: 'These are not shown on the site until their product matches an id in the settings',
    count: (n: number) => `${n} articles`,
    dragHint: 'Drag ⠿ to reorder articles or sections, or to move an article to another section, then press Save order', moveUp: 'Move up', moveDown: 'Move down',
  },
};

function assetURL(path: string): string {
  try {
    return site.asset(path).url;
  } catch {
    return '';
  }
}

const langOf = (ctx: any): Lang => (ctx?.locale === 'en' ? 'en' : 'th');
const editURL = (id: number) => `/admin/content/${TYPE}/${id}`;

function docsTable(rows: any[], t: (typeof T)['th'], lang: Lang): string {
  const tr_ = rows.map((r) => `<tr data-id="${esc(r.id)}"><td><a href="${esc(editURL(r.id))}" data-ak-open><strong>${esc(r.title || '—')}</strong></a>` +
    `<div class="ak-muted ak-small ak-mono">${esc(r.slug)}</div></td>` +
    `<td>${esc(r.section || '—')}</td><td class="ak-num">${r.order >= 1000 ? '—' : esc(r.order)}</td>` +
    `<td>${r.status === 'publish' ? `<span class="ak-badge ak-badge-ok">${esc(t.published)}</span>` : `<span class="ak-badge">${esc(t.draft)}</span>`}</td>` +
    `<td class="ak-muted">${esc(String(r.updatedAt || '').slice(0, 10))}</td>` +
    `<td style="text-align:right;white-space:nowrap"><a class="ak-btn ak-btn-sm" href="${esc(editURL(r.id))}" data-ak-open>${esc(t.edit)}</a>` +
    (r.status === 'publish' && r.url ? ` <a class="ak-btn ak-btn-sm ak-btn-ghost" href="${esc(r.url)}" target="_blank" rel="noopener">${esc(t.open)}</a>` : '') + `</td></tr>`).join('');
  return `<div class="ak-table-wrap"><table class="ak-table"><thead><tr><th>${esc(t.colTitle)}</th><th>${esc(t.colSection)}</th>` +
    `<th class="ak-num">${esc(t.colOrder)}</th><th>${esc(t.colStatus)}</th><th>${esc(t.colUpdated)}</th><th></th></tr></thead><tbody>${tr_}</tbody></table></div>`;
}

export function registerAdmin(): void {
  site.adminPage('docs', {
    label: { th: 'เอกสาร (ตั้งค่า)', en: 'Docs hub' },
    icon: '📘',
    access: ACCESS,
    render: (ctx) => {
      const lang = langOf(ctx), t = T[lang];
      const ps = products();
      const all = loadAll(true).map((p) => ({ ...docFromPost(p), status: p.status }));
      const known = new Set(ps.map((p) => p.id));
      const groups = ps.map((p) => {
        const mine = all.filter((d) => d.product === p.id);
        const secs = sectionsOf(mine as any, p.id, lang === 'th' ? 'ทั่วไป' : 'General');
        const noSection = lang === 'th' ? 'ทั่วไป' : 'General';
        const body = secs.map((s) => {
          const rows = s.docs.map((d: any) => ({ ...d, url: d.status === 'publish' ? docURL(d) : '' }));
          // the section's stored name ("" when its docs have none, shown as "General")
          const stored = s.docs.some((d: any) => d.section) ? s.name : '';
          return `<div class="docs-sec" data-section="${esc(stored)}"><h3 class="docs-sec-head"><span class="docs-sec-name">${esc(stored || noSection)}</span></h3>${docsTable(rows, t, lang)}</div>`;
        }).join('');
        return `<section class="ak-card ak-card-flush" data-product="${esc(p.id)}"><div class="ak-card-head" style="padding:14px 16px 0">` +
          `<h2>${esc(p.icon)} ${esc(tr(p.name, lang))} <span class="ak-muted ak-small" style="font-weight:400">· ${esc(p.id)} · ${esc(t.count(mine.length))}</span></h2>` +
          `<a class="ak-btn ak-btn-sm ak-btn-ghost" href="/docs/${esc(p.id)}" target="_blank" rel="noopener">${esc(t.view)} ↗</a></div>` +
          (body || `<p class="ak-muted" style="padding:0 16px 14px">${esc(t.none)}</p>`) + `</section>`;
      }).join('');
      const orphans = all.filter((d) => !known.has(d.product));
      const orphanHTML = orphans.length
        ? `<section class="ak-card ak-card-flush" data-product=""><div class="ak-alert ak-alert-warn" style="margin:12px"><strong>${esc(t.orphan)}</strong> — ${esc(t.orphanHint)}</div>` +
          `${docsTable(orphans.map((d: any) => ({ ...d, url: '' })), t, lang)}</section>`
        : '';
      const counts: Record<string, number> = {};
      for (const d of all) counts[d.product] = (counts[d.product] ?? 0) + 1;
      const data = {
        locale: lang,
        products: ps,
        counts,
        legacyRedirects: parseRedirects(setting('legacyRedirects')).items,
        hubTitle: String(setting('hubTitle') ?? ''),
        hubIntro: String(setting('hubIntro') ?? ''),
        editUrl: String(setting('editUrl') ?? ''),
        canSettings: !!ctx.user?.can?.('manage_options'),
        maxBatch: MAX_BATCH,
        dragHint: t.dragHint, moveUp: t.moveUp, moveDown: t.moveDown,
      };
      return {
        title: t.title,
        html: `<header class="ak-page-head"><span class="ak-page-icon">📘</span><div class="ak-page-head-text"><h1>${esc(t.title)}</h1><p>${esc(t.desc)}</p></div>` +
          `<div class="ak-actions"><a class="ak-btn ak-btn-sm" href="/docs" target="_blank" rel="noopener">${esc(t.view)} ↗</a>` +
          `<a class="ak-btn ak-btn-sm ak-btn-primary" style="color:#fff" href="/admin/content/${TYPE}/new" data-ak-open>＋ ${esc(t.newDoc)}</a></div></header>` +
          `<div class="ak-tabs" data-ak-tabs><button class="ak-tab" data-tab="docs">${esc(t.docs)} (${all.length})</button>` +
          `<button class="ak-tab" data-tab="import">${esc(t.importTab)}</button><button class="ak-tab" data-tab="settings">${esc(t.settings)}</button></div>` +
          `<section data-tab-panel="docs" id="docs-list">${groups}${orphanHTML}</section>` +
          `<section data-tab-panel="import"><div class="ak-card" id="docs-import"></div></section>` +
          `<section data-tab-panel="settings"><div class="ak-card" id="docs-settings"></div></section>` +
          `<script type="application/json" id="docs-data">${jsonData(data)}</script>` +
          `<script src="${assetURL('assets/admin.js')}" defer></script>`,
      };
    },
  });

  const fail = (message: string) => ({ message, errors: { form: message } });

  // one batch of the bundle import (the page splits a bundle into batches of MAX_BATCH)
  site.action('import', {
    input: { docs: 'json', status: 'text?' },
    access: ACCESS,
    run: ({ input }) => {
      const list = input.docs;
      if (!Array.isArray(list) || list.length === 0) return fail('ไม่มีบทความในชุดข้อมูล ({docs: [...]})');
      if (list.length > MAX_BATCH) return fail(`นำเข้าได้ครั้งละไม่เกิน ${MAX_BATCH} บทความ`);
      const status = input.status === 'draft' ? 'draft' : 'publish';
      const known = new Set(products().map((p) => p.id));
      let created = 0, updated = 0;
      const errors: { slug: string; error: string }[] = [];
      for (const d of list) {
        const slug = String(d?.slug ?? '').trim();
        const title = String(d?.title ?? '').trim();
        const product = String(d?.product ?? '').trim();
        const content = String(d?.content ?? '');
        const bad = !SLUG_RE.test(slug) ? 'slug ไม่ถูกต้อง (a-z 0-9 -)'
          : !title ? 'ไม่มีชื่อ (title)'
          : title.length > 255 ? 'ชื่อยาวเกิน 255 ตัวอักษร'
          : !known.has(product) ? `ไม่รู้จักผลิตภัณฑ์ "${product}"`
          : content.length > MAX_CONTENT ? 'เนื้อหายาวเกินไป' : '';
        if (bad) {
          errors.push({ slug: slug || '?', error: bad });
          continue;
        }
        const fields = { title, excerpt: String(d.excerpt ?? '').slice(0, 1000), content };
        const meta: Record<string, any> = { product, section: String(d.section ?? '').trim().slice(0, 80) };
        for (const k of ['order', 'sectionOrder']) {
          const n = Number(d[k]);
          meta[k] = d[k] === '' || d[k] == null || !isFinite(n) ? null : n;
        }
        // 1.2.0: a translation — {lang: "en", base: "<the original's slug>"} (the post slug stays unique, e.g. "intro-en")
        const lng = String(d.lang ?? '').trim().toLowerCase();
        meta.lang = (DOC_LANGS as string[]).includes(lng) ? lng : null;
        const base = String(d.base ?? '').trim();
        meta.base = SLUG_RE.test(base) && base !== slug ? base : null;
        try {
          const cur = db.posts.get(slug, { type: TYPE });
          if (cur) {
            db.posts.update(cur.id, { ...fields, meta, status: d.status === 'draft' || d.status === 'publish' ? d.status : cur.status });
            updated++;
          } else {
            db.posts.create({ type: TYPE, slug, ...fields, meta, status: d.status === 'draft' || d.status === 'publish' ? d.status : status });
            created++;
          }
        } catch (e) {
          errors.push({ slug, error: String((e as Error)?.message ?? e) });
        }
      }
      invalidate();
      return { data: { created, updated, errors } };
    },
  });

  site.action('reorder', {
    input: { items: 'json' },
    access: ACCESS,
    run: ({ input }) => {
      const list = input.items;
      if (!Array.isArray(list) || list.length === 0 || list.length > 1000) return fail('ไม่มีรายการที่จะบันทึก');
      let updated = 0;
      const errors: { id: number; error: string }[] = [];
      for (const it of list) {
        const id = Number(it?.id);
        const section = String(it?.section ?? '').trim().slice(0, 80);
        const order = Number(it?.order), sectionOrder = Number(it?.sectionOrder);
        if (!Number.isInteger(id) || id <= 0 || !isFinite(order) || !isFinite(sectionOrder)) {
          errors.push({ id, error: 'ข้อมูลไม่ถูกต้อง' });
          continue;
        }
        try {
          const cur = db.posts.get(id, { type: TYPE });
          if (!cur || cur.type !== TYPE) {
            errors.push({ id, error: 'ไม่พบบทความ' });
            continue;
          }
          const m = cur.meta ?? {};
          if (String(m.section ?? '').trim() === section && Number(m.order) === order && Number(m.sectionOrder) === sectionOrder) continue;
          db.posts.update(id, { meta: { ...m, section, order, sectionOrder } });
          updated++;
        } catch (e) {
          errors.push({ id, error: String((e as Error)?.message ?? e) });
        }
      }
      invalidate();
      return { data: { updated, errors } };
    },
  });

  site.action('saveSettings', {
    input: { products: 'json?', legacyRedirects: 'json?', hubTitle: 'text?', hubIntro: 'text?', editUrl: 'text?' },
    access: ACCESS,
    run: ({ input, user }) => {
      if (!user?.can?.('manage_options')) return fail('เฉพาะผู้ดูแลระบบ (manage_options) แก้การตั้งค่าได้');
      const values: Record<string, string> = {};
      if (input.products !== undefined && input.products !== null) {
        const r = parseProducts(input.products);
        if (r.error) return fail(r.error);
        values.products = JSON.stringify(r.products, null, 2);
      }
      if (input.legacyRedirects !== undefined && input.legacyRedirects !== null) {
        const r = parseRedirects(input.legacyRedirects);
        if (r.errors.length) return fail('การเปลี่ยนเส้นทางไม่ถูกต้อง: ' + r.errors.slice(0, 5).join(', '));
        values.legacyRedirects = r.items.length ? JSON.stringify(r.items, null, 2) : '';
      }
      if (typeof input.hubTitle === 'string') values.hubTitle = input.hubTitle.trim().slice(0, 120);
      if (typeof input.hubIntro === 'string') values.hubIntro = input.hubIntro.trim().slice(0, 500);
      if (typeof input.editUrl === 'string') {
        const u = input.editUrl.trim().slice(0, 500);
        if (u && !editHref(u, { id: 1, slug: 'x', product: 'x' })) return fail('ลิงก์แก้ไขต้องขึ้นต้นด้วย https:// หรือ / (ใช้ {id} {slug} {product} ได้)');
        values.editUrl = u;
      }
      try {
        site.saveSettings(values);
      } catch (e) {
        return fail(String((e as Error)?.message ?? e));
      }
      invalidate();
      return { data: { saved: Object.keys(values) } };
    },
  });
}
