// Maintenance mode (official plugin 2.0.0) — the JavaScript port of the built-in Go plugin "maintenance":
//  - site.middleware: while "enabled", visitors get a standalone 503 page (Retry-After: 3600, no-store);
//    /?bypass=<key> sets an HttpOnly cookie (7 days, bound to the key: changing the key ends old cookies)
//    and redirects to the same URL without "bypass"; the admin, /api, uploads and plugin assets never
//    reach site.middleware
//  - the bypass key: the "bypassKey" setting, or one generated when maintenance is on and it is empty
//    real pages served to bypass-cookie holders get Cache-Control: no-store (middleware {headers})
//  - admin-wide notice "Maintenance mode is on" with the bypass link (site.adminNotice, administrators)
//  - admin page "Maintenance": status, the settings form (site.saveSettings), the bypass link (copy / new
//    key), a preview of the page with the site name and language (site.info)
//  - exports: status() → {enabled, bypassKey, bypassPath}, legacyImport (the built-in had no tables)

/**
 * Who controls maintenance mode. CMS 2.0.6+: the plugin's own capability maintenance.manage (manifest
 * "capabilities", default administrators), ticked per role in Users → Roles. Older CMS versions: administrators.
 */
const MANAGE = typeof (site as any).can === 'function' ? ['cap:maintenance.manage'] : ['administrator'];

const COOKIE = 'bypass'; // → pl_maintenance_bypass
const GEN_KEY = 'maintenance:bypassKey';
const DEFAULTS: Record<string, string> = {
  title: 'ปิดปรับปรุงชั่วคราว',
  message: 'เว็บไซต์กำลังปรับปรุงเพื่อให้บริการได้ดียิ่งขึ้น กรุณากลับมาใหม่อีกครั้งในไม่ช้า',
  bypassKey: '',
};

const truthy = (v: unknown) => v === true || v === 1 || v === '1' || v === 'true';
const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? '';
const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);

function str(key: string): string {
  const v = site.settings(key);
  return v === null || v === undefined || (v === '' && key !== 'bypassKey') ? DEFAULTS[key] : String(v);
}

function enabled(): boolean {
  return truthy(site.settings('enabled'));
}

/** Site title and language (Settings → General). */
function siteInfo(): { title: string; language: string } {
  const i = site.info();
  return { title: String(i.title ?? ''), language: String(i.language ?? '') };
}

/** The cookie value for a key (never the key itself). */
function token(key: string): string {
  return crypto.hash('sha256', 'anasaki-maintenance|' + key).slice(0, 32);
}

function storedKey(): string {
  const c = cache.get<string>(GEN_KEY);
  if (typeof c === 'string') return c;
  const r = db.table('state').where('k', 'bypassKey').first();
  const v = r ? String(r.v) : '';
  cache.set(GEN_KEY, v, 60);
  return v;
}

function storeKey(v: string) {
  if (!db.table('state').where('k', 'bypassKey').update({ v, updated_at: new Date() })) {
    if (!db.table('state').where('k', 'bypassKey').exists()) db.table('state').insert({ k: 'bypassKey', v, updated_at: new Date() });
  }
  cache.set(GEN_KEY, v, 60);
}

/**
 * The bypass key in force: the setting, else the generated one. While maintenance is on and neither
 * exists, a random key is generated and kept (the built-in plugin's EnsureBypassKey).
 */
function bypassKey(create: boolean): string {
  const set = str('bypassKey');
  if (set.trim() !== '') return set;
  let k = storedKey();
  if (!k && create && enabled()) {
    k = crypto.random(12);
    storeKey(k);
  }
  return k;
}

/** Paths that are never blocked (core does not run site.middleware for most of them anyway). */
function exempt(path: string): boolean {
  for (const p of ['/api/', '/uploads/', '/plugin-assets/', '/admin/', '/ext-ui/']) if (path.startsWith(p)) return true;
  return path === '/admin';
}

/** The query string without "bypass" (keys sorted and re-encoded like Go's url.Values.Encode). */
function withoutBypass(url: string): string {
  const i = url.indexOf('?');
  if (i < 0) return '';
  const dec = (s: string) => {
    try {
      return decodeURIComponent(s.replace(/\+/g, ' '));
    } catch {
      return s;
    }
  };
  const enc = (s: string) => encodeURIComponent(s).replace(/%20/g, '+');
  const pairs = url.slice(i + 1).split('&').filter(Boolean).map((p) => {
    const j = p.indexOf('=');
    return j < 0 ? [dec(p), ''] : [dec(p.slice(0, j)), dec(p.slice(j + 1))];
  }).filter(([k]) => k !== 'bypass');
  const keys = [...new Set(pairs.map(([k]) => k))].sort();
  return keys.flatMap((k) => pairs.filter(([x]) => x === k).map(([, v]) => enc(k) + '=' + enc(v))).join('&');
}

/** The standalone maintenance page (same markup as the built-in plugin). */
function page(siteTitle: string, title: string, message: string, lang: string): string {
  const e = esc;
  if (!lang) lang = 'th';
  return `<!doctype html><html lang="${e(lang)}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">` +
    `<meta name="robots" content="noindex"><title>${e(title)}${siteTitle ? ' – ' + e(siteTitle) : ''}</title>` +
    `<style>html,body{margin:0;height:100%}body{display:flex;align-items:center;justify-content:center;font-family:'Inter','Noto Sans Thai',system-ui,sans-serif;background:#0b0f19;color:#e5e7eb;text-align:center;padding:24px;box-sizing:border-box}` +
    `main{max-width:560px}.i{font-size:56px}h1{font-size:30px;margin:16px 0 12px;color:#fff}p{line-height:1.7;margin:0;color:#9ca3af}small{display:block;margin-top:32px;color:#6b7280}</style></head>` +
    `<body><main><div class="i">🚧</div><h1>${e(title)}</h1><p>${e(message).replace(/\n/g, '<br>')}</p>` +
    `<small>${e(siteTitle)}</small></main></body></html>`;
}

site.middleware((ctx) => {
  if (!enabled()) return;
  const req = ctx.request;
  // a signed-in CMS user (admin bearer token, e.g. previews) and system paths are never blocked
  if (ctx.isAdminUser || exempt(req.path)) return;
  const key = bypassKey(true);
  if (key) {
    const given = one(req.query.bypass as any);
    if (given && crypto.timingSafeEqual(given, key)) {
      ctx.response.setCookie(COOKIE, token(key), { maxAge: 7 * 24 * 3600, httpOnly: true, sameSite: 'Lax', path: '/' });
      const q = withoutBypass(req.url);
      return { status: 302, headers: { Location: req.path + (q ? '?' + q : ''), 'Cache-Control': 'no-store' }, body: '' };
    }
    const c = req.cookies[COOKIE];
    // the real page for a tester: never cached (shared caches must not keep it for other visitors)
    if (c && crypto.timingSafeEqual(c, token(key))) return { headers: { 'Cache-Control': 'no-store' } };
  }
  const info = siteInfo();
  return {
    status: 503,
    headers: { 'Content-Type': 'text/html; charset=utf-8', 'Retry-After': '3600', 'Cache-Control': 'no-store' },
    body: page(info.title, str('title'), str('message'), info.language),
  };
}, { priority: 5 });

// ---------------------------------------------------------------- exports

/** For the admin shell (banner + "View site" bypass link) and other plugins. */
site.export('status', () => {
  const on = enabled();
  const key = bypassKey(true);
  return { enabled: on, bypassKey: key, bypassPath: key ? '/?bypass=' + encodeURIComponent(key) : '/' };
});

/** The admin shell's banner (was core/maintenance.ts): shown to administrators while the mode is on. */
site.adminNotice((ctx) => {
  if (!enabled()) return null;
  const key = bypassKey(true);
  return {
    level: 'warning',
    text: { th: '🚧 โหมดปิดปรับปรุงเปิดอยู่ — ผู้เยี่ยมชมจะเห็นหน้า “ปิดปรับปรุง”', en: '🚧 Maintenance mode is on — visitors see the “under maintenance” page' },
    link: key ? { label: { th: 'ดูเว็บจริง (ลิงก์ bypass)', en: 'View the real site (bypass link)' }, href: '/?bypass=' + encodeURIComponent(key) } : undefined,
  };
}, { access: MANAGE });

/** Legacy import: the built-in plugin had no tables (its settings are mapped by core with the same keys). */
site.export('legacyImport', (_payload: any) => ({ imported: 0 }));

// ---------------------------------------------------------------- admin

const T = {
  th: {
    title: 'โหมดปิดปรับปรุง', desc: 'แสดงหน้า “ปิดปรับปรุงชั่วคราว” (HTTP 503) ให้ผู้เยี่ยมชม ผู้ดูแลยังใช้หลังบ้านได้ตามปกติ',
    on: 'เปิดอยู่ — ผู้เยี่ยมชมจะเห็นหน้า “ปิดปรับปรุง”', off: 'ปิดอยู่ — เว็บแสดงตามปกติ',
    link: 'ลิงก์ดูเว็บจริง (bypass)', linkHint: 'ใครเปิดลิงก์นี้จะดูเว็บจริงได้ 7 วัน — ส่งให้ลูกค้าหรือทีมทดสอบ',
    noKey: 'ยังไม่มีรหัส — จะสร้างให้อัตโนมัติเมื่อเปิดโหมดปิดปรับปรุง', copy: 'คัดลอก', copied: 'คัดลอกแล้ว', open: 'เปิดดูเว็บจริง',
    newKey: 'สร้างรหัสใหม่', newKeyAsk: 'สร้างรหัสใหม่? ลิงก์ bypass เดิมจะใช้ไม่ได้อีก', fixedKey: 'รหัสนี้ตั้งไว้ในหน้าตั้งค่า — เปลี่ยนได้ที่นั่น',
    settings: 'ตั้งค่า', settingsHint: 'ค่าเดียวกับหน้า ปลั๊กอิน → โหมดปิดปรับปรุง → ตั้งค่า',
    preview: 'ตัวอย่างหน้าที่ผู้เยี่ยมชมเห็น', heading: 'หัวข้อ', message: 'ข้อความ', enable: 'เปิดโหมดปิดปรับปรุง',
    keyLabel: 'รหัส bypass', keyHint: 'เว้นว่าง = ระบบสร้างให้เอง', save: 'บันทึก', saved: 'บันทึกแล้ว',
  },
  en: {
    title: 'Maintenance mode', desc: 'Shows a “back soon” page (HTTP 503) to visitors; the admin keeps working as usual.',
    on: 'On — visitors see the “under maintenance” page', off: 'Off — the site is shown as usual',
    link: 'Real-site link (bypass)', linkHint: 'Anyone opening this link sees the real site for 7 days — share it with testers.',
    noKey: 'No key yet — one is generated when maintenance mode is switched on', copy: 'Copy', copied: 'Copied', open: 'Open the real site',
    newKey: 'New key', newKeyAsk: 'Generate a new key? The old bypass link stops working.', fixedKey: 'This key is set in the settings — change it there.',
    settings: 'Settings', settingsHint: 'The same values as Plugins → Maintenance mode → Settings.',
    preview: 'What visitors see', heading: 'Title', message: 'Message', enable: 'Enable maintenance mode',
    keyLabel: 'Bypass key', keyHint: 'Empty = generated for you', save: 'Save', saved: 'Saved',
  },
};

site.adminPage('maintenance', {
  label: { th: 'โหมดปิดปรับปรุง', en: 'Maintenance' },
  icon: '🚧',
  access: MANAGE,
  render: (ctx) => {
    const t = T[ctx.locale === 'en' ? 'en' : 'th'];
    const on = enabled();
    const key = bypassKey(true);
    const fixed = str('bypassKey').trim() !== '';
    const path = key ? '/?bypass=' + encodeURIComponent(key) : '';
    const info = siteInfo();
    const preview = page(info.title, str('title'), str('message'), info.language);
    const status = `<div class="ak-alert ${on ? 'ak-alert-warn' : 'ak-alert-ok'}" data-mt="status" data-on="${on}">${on ? '🚧 ' + t.on : '✅ ' + t.off}</div>`;
    const link = `<section class="ak-card"><div class="ak-card-head"><h2>🔑 ${t.link}</h2></div>` +
      (key
        ? `<div class="ak-row" style="gap:8px;flex-wrap:wrap"><input id="mt-link" data-mt="link" readonly value="${esc(path)}" style="flex:1;min-width:220px">` +
          `<button class="ak-btn ak-btn-sm" type="button" onclick="mtCopy()">${t.copy}</button>` +
          `<a class="ak-btn ak-btn-sm" href="${esc(path)}" target="_blank" rel="noopener">${t.open}</a>` +
          (fixed ? '' : `<button class="ak-btn ak-btn-sm ak-btn-ghost" type="button" data-mt="newkey" onclick="mtNew()">${t.newKey}</button>`) + `</div>` +
          `<p class="ak-hint">${t.linkHint}${fixed ? ' ' + t.fixedKey : ''}</p>`
        : `<p class="ak-muted" data-mt="nokey">${t.noKey}</p>`) + `</section>`;
    const settings = `<section class="ak-card"><div class="ak-card-head"><h2>⚙️ ${t.settings}</h2></div>` +
      `<form id="mt-settings" data-mt="settings" onsubmit="return mtSave(this)">` +
      `<label class="ak-check"><input type="checkbox" name="enabled" data-mt="enabled"${on ? ' checked' : ''}> ${t.enable}</label>` +
      `<label class="ak-field"><span>${t.heading}</span><input name="title" data-mt="title" maxlength="200" value="${esc(str('title'))}"></label>` +
      `<label class="ak-field"><span>${t.message}</span><textarea name="message" data-mt="message" rows="3" maxlength="2000">${esc(str('message'))}</textarea></label>` +
      `<label class="ak-field"><span>${t.keyLabel}</span><input name="bypassKey" data-mt="key" maxlength="100" value="${esc(str('bypassKey'))}"><p class="ak-hint">${t.keyHint}</p></label>` +
      `<div class="ak-form-actions"><button class="ak-btn ak-btn-primary" type="submit">${t.save}</button></div></form>` +
      `<p class="ak-hint">${t.settingsHint}</p></section>`;
    const prev = `<section class="ak-card"><div class="ak-card-head"><h2>👀 ${t.preview}</h2></div>` +
      `<iframe title="preview" sandbox="" srcdoc="${esc(preview)}" style="width:100%;height:360px;border:1px solid var(--ak-border,#e5e7eb);border-radius:10px"></iframe></section>`;
    const cfg = { copied: t.copied, ask: t.newKeyAsk, saved: t.saved };
    const script = `<script>var MT=${JSON.stringify(cfg).replace(/</g, '\\u003c')};
function mtCopy(){var i=document.getElementById('mt-link');var u=new URL(i.value,document.referrer||location.href).href;
(navigator.clipboard?navigator.clipboard.writeText(u):Promise.reject()).catch(function(){i.select();document.execCommand('copy')}).then(function(){window.AK&&AK.toast(MT.copied,'success')})}
function mtNew(){(window.AK?AK.confirm(MT.ask,{danger:true}):Promise.resolve(confirm(MT.ask))).then(function(ok){if(!ok)return;
anasaki.call('maintenance:newKey',{}).then(function(){location.reload()},function(e){window.AK?AK.toast(e.message,'error'):alert(e.message)})})}
function mtSave(form){var f=form.elements;anasaki.call('maintenance:saveSettings',{enabled:f.namedItem('enabled').checked,title:f.namedItem('title').value,
message:f.namedItem('message').value,bypassKey:f.namedItem('bypassKey').value}).then(function(r){if(r.errors&&Object.keys(r.errors).length){var m=r.message||Object.values(r.errors).join(', ');
window.AK?AK.toast(m,'error'):alert(m);return}window.AK&&AK.toast(MT.saved,'success');setTimeout(function(){location.reload()},400)},function(e){window.AK?AK.toast(e.message,'error'):alert(e.message)});return false}
</script>`;
    return {
      title: t.title,
      html: `<header class="ak-page-head"><span class="ak-page-icon">🚧</span><div class="ak-page-head-text"><h1>${t.title}</h1><p>${t.desc}</p></div></header>` +
        status + link + settings + prev + script,
    };
  },
});

/** A new generated bypass key (only when the "bypassKey" setting is empty). */
site.action('newKey', {
  access: MANAGE,
  run: (ctx) => {
    const en = ctx.locale === 'en';
    if (str('bypassKey').trim() !== '') {
      const m = en ? 'The bypass key is set in the settings' : 'รหัส bypass ตั้งไว้ในหน้าตั้งค่า';
      return { errors: { form: m }, message: m };
    }
    const k = crypto.random(12);
    storeKey(k);
    return { message: en ? 'New key generated' : 'สร้างรหัสใหม่แล้ว', data: { bypassPath: '/?bypass=' + encodeURIComponent(k) } };
  },
});

/** The settings form of the admin page (the generic settings screen /p/maintenance of the built-in plugin). */
site.action('saveSettings', {
  input: { enabled: 'bool', title: 'text?', message: 'text?', bypassKey: 'text?' },
  access: MANAGE,
  run: (ctx) => {
    const en = ctx.locale === 'en';
    const { input } = ctx;
    try {
      const v = site.saveSettings({
        enabled: !!input.enabled,
        title: String(input.title ?? '').trim(),
        message: String(input.message ?? '').trim(),
        bypassKey: String(input.bypassKey ?? '').trim(),
      });
      return { message: en ? 'Saved' : 'บันทึกแล้ว', data: v };
    } catch (e) {
      const m = String((e as Error)?.message ?? e);
      return { errors: { form: m }, message: m };
    }
  },
});
