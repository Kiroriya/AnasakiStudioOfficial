// Page Translate 1.4.0 — admin page "แปลภาษา (ตั้งค่า)" with the plugin's own capabilities:
//  - page-translate.manage  : open the page, change mode / site language / button color
//  - page-translate.api_key : set, change or clear the Google API key (also needs manage to open the page)
// Both are ticked per role in Users → Roles (CMS 2.0.6+). Older CMS versions (no site.can): the page is open to
// editors and administrators, the API key to administrators only.
// The public widget is head assets + templates/body.html ({{settings.*}}); the API key is not in the template —
// site.head below writes <meta name="ptw-api-key"> only in API mode, so a key kept while the free mode is on
// never reaches the page.

const CAPS = typeof (site as any).can === 'function';
const ACCESS_MANAGE = CAPS ? ['cap:page-translate.manage'] : ['editor'];
const ACCESS_KEY = CAPS ? ['cap:page-translate.api_key'] : ['administrator'];

const MODES = ['free', 'api'];
const LANGS = ['th', 'en', 'ja', 'zh'];
const COLOR_RE = /^#[0-9a-fA-F]{6}$/;
const POSITIONS = ['bottom-right', 'bottom-left', 'top-right', 'top-left'];
const STYLES = ['auto', 'text', 'icon'];
const MAX_KEY = 200;

type Lang = 'th' | 'en';

const T = {
  th: {
    title: 'แปลภาษา', desc: 'ตั้งค่าปุ่มแปลภาษาลอยบนทุกหน้าเว็บ',
    mode: 'โหมดการแปล', free: 'ฟรี — Google Translate Widget (ไม่ต้องใช้ API key)', api: 'Google Cloud Translation API (ต้องมี API key)',
    siteLang: 'ภาษาต้นฉบับของเว็บ', color: 'สีปุ่มลอย', save: 'บันทึก',
    position: 'ตำแหน่งปุ่มลอย', style: 'แบบปุ่ม', styleAuto: 'ไอคอน + ข้อความ (จอเล็กเหลือไอคอน)', styleText: 'ไอคอน + ข้อความทุกจอ', styleIcon: 'ไอคอนอย่างเดียว',
    posBR: '↘ ล่างขวา', posBL: '↙ ล่างซ้าย', posTR: '↗ บนขวา', posTL: '↖ บนซ้าย',
    keyTitle: 'Google API Key', keyHint: 'ใช้เฉพาะโหมด API · key จะอยู่ในหน้าเว็บ ต้องจำกัดด้วย HTTP referrer และให้ใช้ได้แค่ Cloud Translation API ใน Google Cloud Console',
    keySet: 'ตั้งค่าแล้ว', keyEmpty: 'ยังไม่ได้ตั้ง', keyNew: 'API key ใหม่', keySave: 'บันทึก key', keyClear: 'ลบ key',
    keyNoPerm: 'ตำแหน่งของคุณไม่มีสิทธิ์แก้ API key (page-translate.api_key)',
    view: 'ดูหน้าเว็บ',
  },
  en: {
    title: 'Page Translate', desc: 'Settings of the floating translate button on every page',
    mode: 'Translation mode', free: 'Free — Google Translate Widget (no API key)', api: 'Google Cloud Translation API (API key required)',
    siteLang: 'Original language of the site', color: 'Floating button color', save: 'Save',
    position: 'Floating button position', style: 'Button style', styleAuto: 'Icon + text (icon only on phones)', styleText: 'Icon + text everywhere', styleIcon: 'Icon only',
    posBR: '↘ Bottom right', posBL: '↙ Bottom left', posTR: '↗ Top right', posTL: '↖ Top left',
    keyTitle: 'Google API key', keyHint: 'API mode only · the key is visible in the page, so restrict it by HTTP referrer and to the Cloud Translation API in Google Cloud Console',
    keySet: 'Set', keyEmpty: 'Not set', keyNew: 'New API key', keySave: 'Save key', keyClear: 'Clear key',
    keyNoPerm: 'Your role cannot change the API key (page-translate.api_key)',
    view: 'View site',
  },
};

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);

const jsonData = (v: unknown) =>
  JSON.stringify(v).replace(/</g, '\\u003c').replace(/>/g, '\\u003e').replace(/&/g, '\\u0026');

function setting(key: string): string {
  try {
    return String(site.settings(key) ?? '');
  } catch {
    return '';
  }
}

function assetURL(path: string): string {
  try {
    return site.asset(path).url;
  } catch {
    return '';
  }
}

const langOf = (ctx: any): Lang => (ctx?.locale === 'en' ? 'en' : 'th');
const fail = (message: string) => ({ message, errors: { form: message } });

// whether this user may change the key — only to draw the form; the saveApiKey action enforces it
function canKey(user: any): boolean {
  try {
    if (CAPS && typeof user?.can === 'function') return !!user.can('page-translate.api_key');
    return !!user?.can?.('manage_options');
  } catch {
    return false;
  }
}

function maskKey(k: string): string {
  return k.length > 4 ? '••••••••' + k.slice(-4) : '••••';
}

site.head(() => {
  const key = setting('apiKey').trim();
  return setting('mode') === 'api' && key ? `<meta name="ptw-api-key" content="${esc(key)}">` : '';
});

site.adminPage('settings', {
  label: { th: 'แปลภาษา (ตั้งค่า)', en: 'Page Translate' },
  icon: '🌐',
  access: ACCESS_MANAGE,
  render: (ctx: any) => {
    const lang = langOf(ctx), t = T[lang];
    const mode = MODES.includes(setting('mode')) ? setting('mode') : 'free';
    const siteLang = LANGS.includes(setting('siteLang')) ? setting('siteLang') : 'th';
    const color = COLOR_RE.test(setting('buttonColor')) ? setting('buttonColor') : '#4f46e5';
    const position = POSITIONS.includes(setting('position')) ? setting('position') : 'bottom-right';
    const style = STYLES.includes(setting('buttonStyle')) ? setting('buttonStyle') : 'auto';
    const key = setting('apiKey').trim();
    const mayKey = canKey(ctx?.user);

    const opt = (v: string, label: string, cur: string) => `<option value="${esc(v)}"${v === cur ? ' selected' : ''}>${esc(label)}</option>`;
    const langLabels: Record<string, string> = { th: 'ไทย (th)', en: 'English (en)', ja: '日本語 (ja)', zh: '中文 (zh)' };

    const general =
      `<form class="ak-card" id="ptw-general">` +
      `<label class="ak-field"><span>${esc(t.mode)}</span><select class="ak-input" name="mode">${opt('free', t.free, mode)}${opt('api', t.api, mode)}</select></label>` +
      `<label class="ak-field"><span>${esc(t.siteLang)}</span><select class="ak-input" name="siteLang">${LANGS.map((l) => opt(l, langLabels[l], siteLang)).join('')}</select></label>` +
      `<label class="ak-field"><span>${esc(t.color)}</span><input class="ak-input" type="color" name="buttonColor" value="${esc(color)}"></label>` +
      `<label class="ak-field"><span>${esc(t.position)}</span><select class="ak-input" name="position">${opt('bottom-right', t.posBR, position)}${opt('bottom-left', t.posBL, position)}${opt('top-right', t.posTR, position)}${opt('top-left', t.posTL, position)}</select></label>` +
      `<label class="ak-field"><span>${esc(t.style)}</span><select class="ak-input" name="buttonStyle">${opt('auto', t.styleAuto, style)}${opt('text', t.styleText, style)}${opt('icon', t.styleIcon, style)}</select></label>` +
      `<div class="ak-form-actions"><button type="submit" class="ak-btn ak-btn-primary">${esc(t.save)}</button></div>` +
      `</form>`;

    const keyCard =
      `<section class="ak-card" id="ptw-key"><div class="ak-card-head"><h2>${esc(t.keyTitle)}</h2>` +
      `<span class="ak-badge${key ? ' ak-badge-ok' : ''}">${esc(key ? t.keySet + ' · ' + maskKey(key) : t.keyEmpty)}</span></div>` +
      `<p class="ak-hint">${esc(t.keyHint)}</p>` +
      (mayKey
        ? `<form id="ptw-key-form"><label class="ak-field"><span>${esc(t.keyNew)}</span>` +
          `<input class="ak-input ak-mono" type="password" name="apiKey" maxlength="${MAX_KEY}" autocomplete="off" spellcheck="false"></label>` +
          `<div class="ak-form-actions"><button type="submit" class="ak-btn ak-btn-primary">${esc(t.keySave)}</button>` +
          (key ? `<button type="button" class="ak-btn ak-btn-danger" data-ptw-clear>${esc(t.keyClear)}</button>` : '') +
          `</div></form>`
        : `<div class="ak-alert ak-alert-warn">${esc(t.keyNoPerm)}</div>`) +
      `</section>`;

    return {
      title: t.title,
      html:
        `<header class="ak-page-head"><span class="ak-page-icon">🌐</span><div class="ak-page-head-text"><h1>${esc(t.title)}</h1><p>${esc(t.desc)}</p></div>` +
        `<div class="ak-actions"><a class="ak-btn ak-btn-sm" href="/" target="_blank" rel="noopener">${esc(t.view)} ↗</a></div></header>` +
        general + keyCard +
        `<script type="application/json" id="ptw-data">${jsonData({ locale: lang })}</script>` +
        `<script src="${assetURL('assets/admin.js')}" defer></script>`,
    };
  },
});

site.action('save', {
  input: { mode: 'text', siteLang: 'text', buttonColor: 'text', position: 'text?', buttonStyle: 'text?' },
  access: ACCESS_MANAGE,
  run: ({ input }: any) => {
    const mode = String(input.mode ?? '').trim();
    const siteLang = String(input.siteLang ?? '').trim();
    const buttonColor = String(input.buttonColor ?? '').trim();
    if (!MODES.includes(mode)) return fail('โหมดไม่ถูกต้อง (free / api)');
    if (!LANGS.includes(siteLang)) return fail('ภาษาต้นฉบับไม่ถูกต้อง');
    if (!COLOR_RE.test(buttonColor)) return fail('สีต้องเป็นรูปแบบ #rrggbb');
    const position = String(input.position ?? '').trim() || 'bottom-right';
    const buttonStyle = String(input.buttonStyle ?? '').trim() || 'auto';
    if (!POSITIONS.includes(position)) return fail('ตำแหน่งปุ่มไม่ถูกต้อง');
    if (!STYLES.includes(buttonStyle)) return fail('แบบปุ่มไม่ถูกต้อง');
    try {
      site.saveSettings({ mode, siteLang, buttonColor, position, buttonStyle });
    } catch (e) {
      return fail(String((e as Error)?.message ?? e));
    }
    return { data: { saved: ['mode', 'siteLang', 'buttonColor', 'position', 'buttonStyle'] } };
  },
});

site.action('saveApiKey', {
  input: { apiKey: 'text?' },
  access: ACCESS_KEY,
  run: ({ input }: any) => {
    const apiKey = String(input.apiKey ?? '').trim();
    if (apiKey.length > MAX_KEY) return fail(`API key ยาวเกิน ${MAX_KEY} ตัวอักษร`);
    if (apiKey && !/^[A-Za-z0-9_\-]+$/.test(apiKey)) return fail('API key มีตัวอักษรที่ไม่ถูกต้อง');
    try {
      site.saveSettings({ apiKey });
    } catch (e) {
      return fail(String((e as Error)?.message ?? e));
    }
    return { data: { set: !!apiKey } };
  },
});
