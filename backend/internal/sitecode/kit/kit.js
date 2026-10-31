/* Anasaki admin UI kit → window.AK (API: kit-demo.html). th/en from <html lang>. */
(() => {
const d = document, M = {
 th: {ok: 'ตกลง', cancel: 'ยกเลิก', empty: 'ไม่มีข้อมูล', required: 'กรุณากรอกช่องนี้', invalid: 'รูปแบบไม่ถูกต้อง'},
 en: {ok: 'OK', cancel: 'Cancel', empty: 'No data', required: 'This field is required', invalid: 'Invalid value'},
};
const lang = () => (/^en/.test(d.documentElement.lang) ? 'en' : 'th');
const t = k => M[lang()][k] || k;
const $ = x => (typeof x == 'string' ? d.querySelector(x) : x);
const each = (s, f, r) => (r || d).querySelectorAll(s).forEach(f);
const at = (e, a) => {for (const k in a) e.setAttribute(k, a[k]); return e;};
const el = (tag, cls, txt, a) => {const e = at(d.createElement(tag), a); if (cls) e.className = cls; if (txt != null) e.textContent = txt; return e;};
const svg = (tag, a) => at(d.createElementNS('http://www.w3.org/2000/svg', tag), a);

// toast(msg, kind, ms)
function toast(msg, kind, ms) {
 let box = d.getElementById('ak-toasts');
 if (!box) d.body.appendChild(box = el('div', 'ak-toasts', null, {id: 'ak-toasts', role: 'status'}));
 const k = /^(success|error|warning|info)$/.test(kind) ? kind : 'success', n = el('div', 'ak-toast ak-toast-' + k, msg, {'data-kind': k});
 box.appendChild(n);
 setTimeout(() => n.remove(), ms || 3000);
 return n;
}

// confirm(text, {ok, cancel, danger}) → Promise<boolean>
function confirm(text, o = {}) {
 return new Promise(res => {
  const bg = el('div', 'ak-modal-bg', null, {id: 'ak-confirm'}), box = el('div', 'ak-modal', null, {role: 'dialog', 'aria-modal': true}),
   row = el('div', 'ak-form-actions'),
   no = el('button', 'ak-btn', o.cancel || t('cancel'), {id: 'ak-confirm-cancel', type: 'button'}),
   yes = el('button', 'ak-btn ' + (o.danger ? 'ak-btn-danger ak-btn-solid' : 'ak-btn-primary'), o.ok || t('ok'), {id: 'ak-confirm-ok', type: 'button'});
  const key = e => e.key == 'Escape' && done(false), done = v => {d.removeEventListener('keydown', key); bg.remove(); res(v);};
  no.onclick = () => done(false); yes.onclick = () => done(true);
  bg.onclick = e => e.target == bg && done(false);
  d.addEventListener('keydown', key);
  row.append(no, yes); box.append(el('p', null, text), row); bg.append(box); d.body.append(bg); yes.focus();
 });
}

// tabs(el): [data-tab=x] shows [data-tab-panel=x]
function tabs(root) {
 root = at($(root), {role: 'tablist'});
 const bs = [...root.querySelectorAll('[data-tab]')], names = bs.map(b => b.dataset.tab);
 const show = n => {
  bs.forEach(b => at(b, {role: 'tab', 'aria-selected': b.dataset.tab == n}));
  each('[data-tab-panel]', p => {if (names.includes(p.dataset.tabPanel)) p.hidden = p.dataset.tabPanel != n;});
  root.dispatchEvent(new CustomEvent('ak-tab', {detail: n}));
 };
 bs.forEach(b => {b.type = 'button'; b.onclick = () => show(b.dataset.tab);});
 const cur = root.querySelector('[aria-selected=true]') || bs[0];
 if (cur) show(cur.dataset.tab);
 return {show};
}

const cmp = (a, b) => {
 a = a == null ? '' : String(a).trim(); b = b == null ? '' : String(b).trim();
 return a !== '' && b !== '' && !isNaN(a) && !isNaN(b) ? a - b : a.localeCompare(b, lang());
};
const sorted = (h, on, dir) => at(h, {'aria-sort': on ? (dir > 0 ? 'ascending' : 'descending') : 'none'});

// table(el, {columns:[{key, label, sort, num, render}], rows, empty}) → {set}
function table(root, o) {
 const tb = el('table', 'ak-table'), tr = el('tr'), body = el('tbody'), wrap = el('div', 'ak-table-wrap');
 for (const c of o.columns) {const h = tr.appendChild(el('th', c.num ? 'ak-num' : '', c.label ?? c.key)); if (c.sort) h.dataset.sort = c.key;}
 tb.append(el('thead'), body); tb.tHead.append(tr); wrap.append(tb); $(root).replaceChildren(wrap);
 const resort = sortable(tb), set = (rows = []) => {
  body.replaceChildren();
  if (!rows.length) body.insertRow().append(el('td', 'ak-empty', o.empty || t('empty'), {colspan: o.columns.length}));
  for (const row of rows) {
   const r = body.insertRow();
   for (const c of o.columns) {
    const td = r.insertCell(), x = row[c.key], v = c.render ? c.render(row) : x;
    if (c.num) td.className = 'ak-num';
    if (x != null && typeof x != 'object') td.dataset.value = x;
    v instanceof Node ? td.append(v) : (td.textContent = v ?? '');
   }
  }
  resort();
 };
 set(o.rows);
 return {set, el: tb};
}

// <table data-ak-sortable>: th[data-sort] sorts rows by cell data-value or text
function sortable(tb) {
 let cur, dir = 1;
 const sort = () => {
  if (!cur) return;
  const i = cur.cellIndex, body = tb.tBodies[0], v = r => (r.cells[i] ? r.cells[i].dataset.value ?? r.cells[i].textContent : '');
  each('th[data-sort]', h => sorted(h, h == cur, dir), tb);
  [...body.rows].filter(r => !r.querySelector('.ak-empty')).sort((a, b) => dir * cmp(v(a), v(b))).forEach(r => body.append(r));
 };
 each('th[data-sort]', h => {
  sorted(h); h.tabIndex = 0;
  h.onclick = h.onkeydown = e => {if (e.key && e.key != 'Enter') return; dir = cur == h ? -dir : 1; cur = h; sort();};
 }, tb);
 return sort;
}

// pager(el, {page, pages, onChange}) → {set}
function pager(root, o) {
 root = at($(root), {role: 'navigation'}); root.classList.add('ak-pager');
 let page = o.page || 1, pages = o.pages || 1;
 const btn = (label, p, off, cur) => {
  const b = el('button', 0, label, {type: 'button', 'data-page': p});
  b.disabled = !!off; if (cur) at(b, {'aria-current': 'page'});
  b.onclick = () => {page = p; draw(); o.onChange && o.onChange(p);};
  root.append(b);
 };
 function draw() {
  root.replaceChildren(); btn('‹', page - 1, page <= 1);
  for (let p = 1, last = 0; p <= pages; p++) if (p == 1 || p == pages || Math.abs(p - page) <= 1) {if (p - last > 1) root.append(el('span', 'ak-gap', '…')); btn(p, p, 0, p == page); last = p;}
  btn('›', page + 1, page >= pages);
 }
 draw();
 return {set: (p, n) => {page = p; if (n) pages = n; draw();}};
}

// validate(form) → boolean; messages in .ak-field .ak-error
function validate(form) {
 let ok = true;
 each('input,select,textarea', i => {
  const f = i.closest('.ak-field'); if (!f) return;
  const err = f.querySelector('.ak-error') || f.appendChild(el('div', 'ak-error')), bad = !i.checkValidity();
  f.classList.toggle('is-invalid', bad); at(i, {'aria-invalid': bad});
  if (bad) {ok = false; err.textContent = i.dataset.error || t(i.validity.valueMissing ? 'required' : 'invalid');}
 }, $(form));
 return ok;
}

// charts: numbers or [{label, value}]
const norm = a => (a || []).map((x, i) => {const o = x && typeof x == 'object'; return {label: String(o && x.label != null ? x.label : i + 1), value: +(o ? x.value : x) || 0};});
const chart = (cls, W, H) => svg('svg', {viewBox: `0 0 ${W} ${H}`, class: 'ak-chart ' + cls, role: 'img', preserveAspectRatio: 'none'});
function bar(root, data, o = {}) {
 const v = norm(data), W = 600, H = o.height || 180, P = 22, ch = H - P * 1.5, bw = W / (v.length || 1),
  max = Math.max(0, ...v.map(x => x.value)) || 1, step = Math.ceil(v.length / 12), s = chart('ak-chart-bar', W, H);
 for (const g of [0, .5, 1]) s.append(svg('line', {x1: 0, x2: W, y1: P / 2 + ch * g, y2: P / 2 + ch * g, class: 'ak-grid-line'}));
 v.forEach((x, i) => {
  const h = ch * Math.max(x.value, 0) / max, r = s.appendChild(svg('rect', {x: i * bw + bw * .15, y: H - P - h, width: bw * .7, height: h, rx: 3, class: 'ak-bar'}));
  r.appendChild(svg('title', {})).textContent = x.label + ': ' + x.value.toLocaleString(lang());
  if (o.labels !== false && i % step == 0) s.appendChild(svg('text', {x: i * bw + bw / 2, y: H - 6, 'text-anchor': 'middle'})).textContent = x.label;
 });
 $(root).replaceChildren(s); return s;
}
function sparkline(root, data, o = {}) {
 const v = norm(data).map(x => x.value), W = o.width || 100, H = o.height || 28, lo = Math.min(...v), span = Math.max(...v) - lo || 1, n = v.length - 1 || 1,
  pts = v.map((x, i) => (i * W / n).toFixed(1) + ',' + (H - 2 - (x - lo) / span * (H - 4)).toFixed(1)).join(' '), s = chart('ak-spark', W, H);
 if (v.length > 1) s.append(svg('polygon', {points: `0,${H} ${pts} ${W},${H}`, class: 'ak-area'}), svg('polyline', {points: pts, class: 'ak-line'}));
 $(root).replaceChildren(s); return s;
}

function init() {
 d.body.classList.add('ak');
 each('[data-ak-tabs]', tabs);
 each('table[data-ak-sortable]', sortable);
 each('form[data-ak-validate]', f => {f.noValidate = true; f.addEventListener('submit', e => {if (!validate(f)) {e.preventDefault(); e.stopImmediatePropagation();}}, true);});
 each('[data-ak-bar]', e => bar(e, JSON.parse(e.dataset.akBar)));
 each('[data-ak-spark]', e => sparkline(e, JSON.parse(e.dataset.akSpark)));
}
window.AK = {lang, t, messages: M, toast, confirm, tabs, table, sortable, pager, validate, bar, sparkline, init};
d.readyState == 'loading' ? d.addEventListener('DOMContentLoaded', init) : init();
})();
