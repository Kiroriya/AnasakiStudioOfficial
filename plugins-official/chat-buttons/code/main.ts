// chat-buttons 1.0 — the ways to reach the business (LINE, Messenger, WhatsApp, Instagram, phone, email) as a
// floating button on every public page (a <details> menu: no JavaScript) and as a row of buttons for the
// designer ("Contact buttons" block). Every address is built from the settings and checked here; the page
// hook's HTML is sanitised by the core as well.

const esc = (v: unknown) =>
  String(v ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&#34;' })[c] as string);
const s = (k: string) => String(site.settings(k) ?? '').trim();

interface Channel { id: string; label: string; url: string }

/** The channels set in the settings, with safe links (empty or malformed ones left out). */
export function channels(): Channel[] {
  const out: Channel[] = [];
  const line = s('line');
  if (/^https:\/\/(lin\.ee|line\.me)\/[\w@%./?=&-]+$/.test(line)) out.push({ id: 'line', label: 'LINE', url: line });
  else if (/^@?[\w.-]{2,40}$/.test(line)) {
    out.push({ id: 'line', label: 'LINE', url: 'https://line.me/R/ti/p/' + encodeURIComponent(line.startsWith('@') ? line : '~' + line) });
  }
  const fb = s('messenger').replace(/^https?:\/\/(www\.)?(m\.me|facebook\.com)\//, '').replace(/\/+$/, '');
  if (/^[\w.-]{2,80}$/.test(fb)) out.push({ id: 'messenger', label: 'Messenger', url: 'https://m.me/' + fb });
  const wa = s('whatsapp').replace(/[^\d]/g, '');
  if (wa.length >= 8 && wa.length <= 15) out.push({ id: 'whatsapp', label: 'WhatsApp', url: 'https://wa.me/' + wa });
  const ig = s('instagram').replace(/^@/, '').replace(/^https?:\/\/(www\.)?instagram\.com\//, '').replace(/\/+$/, '');
  if (/^[\w.]{1,30}$/.test(ig)) out.push({ id: 'instagram', label: 'Instagram', url: 'https://ig.me/m/' + ig });
  const phone = s('phone');
  const tel = phone.replace(/[^\d+]/g, '');
  if (tel.replace('+', '').length >= 6) out.push({ id: 'phone', label: phone, url: 'tel:' + tel });
  const mail = s('email');
  if (/^[^\s@<>"']+@[^\s@<>"']+\.[^\s@<>"']+$/.test(mail)) out.push({ id: 'email', label: mail, url: 'mailto:' + mail });
  return out;
}

const ICONS: Record<string, string> = { line: '💬', messenger: '💭', whatsapp: '🟢', instagram: '📷', phone: '📞', email: '✉️' };

const link = (c: Channel, cls: string) =>
  `<a class="${cls} acb-${c.id}" href="${esc(c.url)}"${/^https:/.test(c.url) ? ' target="_blank" rel="noopener"' : ''}>` +
  `<span aria-hidden="true">${ICONS[c.id]}</span> <span>${esc(c.label)}</span></a>`;

const color = () => (/^#[0-9a-fA-F]{3,8}$/.test(s('color')) ? s('color') : '#06c755');

/** Whether path is under one of the "hide on" paths. */
function hidden(path: string): boolean {
  return s('hidePaths').split('\n').map((p) => p.trim().replace(/\/+$/, '')).filter((p) => p.startsWith('/'))
    .some((p) => path === p || path.startsWith(p + '/') || (p === '' && path === '/'));
}

site.bodyEnd((page) => {
  const pos = s('position') || 'right';
  if (pos === 'off' || hidden(String(page.path ?? '/'))) return '';
  const list = channels();
  if (!list.length) return '';
  const label = s('label');
  return `<details class="acb acb-${pos === 'left' ? 'left' : 'right'}"><summary class="acb-btn" style="background-color:${color()}"` +
    `${label ? '' : ' aria-label="Contact"'}><span aria-hidden="true">💬</span>${label ? `<span>${esc(label)}</span>` : ''}</summary>` +
    `<div class="acb-menu">${list.map((c) => link(c, 'acb-item')).join('')}</div></details>`;
});

const CSS = '.acb-row{display:flex;flex-wrap:wrap;gap:10px}.acb-col{flex-direction:column;align-items:flex-start}' +
  '.acb-chip{display:inline-flex;align-items:center;gap:8px;min-height:44px;padding:0 18px;border-radius:999px;border:1px solid #e5e7eb;background:#fff;color:#111827;text-decoration:none;font-weight:600}' +
  '.acb-chip:hover{border-color:#9ca3af}.acb-line{border-color:#06c755}.acb-whatsapp{border-color:#25d366}.acb-messenger{border-color:#0866ff}' +
  '.acb-hint{color:#6b7280;font-size:13px;padding:8px;border:1px dashed #d1d5db;border-radius:8px}';

site.block('buttons', {
  label: { th: 'ปุ่มติดต่อ (แชท)', en: 'Contact buttons' },
  icon: '💬',
  props: [
    { key: 'layout', type: 'select', label: { th: 'จัดวาง', en: 'Layout' }, default: 'row',
      options: [{ value: 'row', label: { th: 'แถวเดียว', en: 'In a row' } }, { value: 'column', label: { th: 'เรียงลง', en: 'Stacked' } }] },
  ],
  render: (_ctx, props) => {
    const list = channels();
    if (!list.length) return { css: CSS, html: '<p class="acb-hint">ตั้งค่าช่องทางติดต่อที่ ปลั๊กอิน → Chat buttons / Set the channels in Plugins → Chat buttons</p>' };
    return { css: CSS, html: `<div class="acb-row${props.layout === 'column' ? ' acb-col' : ''}">${list.map((c) => link(c, 'acb-chip')).join('')}</div>` };
  },
});
