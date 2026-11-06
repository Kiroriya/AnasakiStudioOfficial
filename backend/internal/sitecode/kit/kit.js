/* Anasaki admin UI kit → window.AK (API: kit-demo.html). th/en from <html lang>. */
(() => {
const d = document, M = {
 th: {pick: 'เลือกไฟล์', nofile: 'ยังไม่ได้เลือกไฟล์', nfiles: 'ไฟล์', ok: 'ตกลง', cancel: 'ยกเลิก', empty: 'ไม่มีข้อมูล', required: 'กรุณากรอกช่องนี้', invalid: 'รูปแบบไม่ถูกต้อง', table: 'ดูเป็นตาราง', chart: 'ดูเป็นกราฟ', date: 'วันที่'},
 en: {pick: 'Choose file', nofile: 'No file chosen', nfiles: 'files', ok: 'OK', cancel: 'Cancel', empty: 'No data', required: 'This field is required', invalid: 'Invalid value', table: 'View as table', chart: 'View as chart', date: 'Date'},
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

// line(el, {labels, series: [{label, color, values}], height, locale, tableLabel, chartLabel, dateLabel}) → {set}
// one y-axis with nice ticks, a soft area under the first series, hover/keyboard crosshair + tooltip, legend, chart ⇄ table
const nice = r => {if (r <= 1) return 1; const p = 10 ** Math.floor(Math.log10(r)), f = r / p; return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * p;};
function line(root, o) {
 root = $(root);
 const W = 1000, box = el('div', 'ak-lc');
 let opt, asTable = false, ro;
 root.replaceChildren(box);
 function draw() {
  const L = (opt.labels || []).map(String), S = (opt.series || []).filter(s => s && Array.isArray(s.values)), H = +opt.height || 240, n = Math.max(1, L.length),
   fmt = v => (+v || 0).toLocaleString(opt.locale || (lang() == 'en' ? 'en-US' : 'th-TH')), m = Math.max(1, ...S.flatMap(s => s.values.map(v => +v || 0))),
   max = Math.ceil(m / nice(m / 4)) * nice(m / 4), step = nice(max / 4),
   x = i => (n == 1 ? W / 2 : i * W / (n - 1)), px = i => x(i) / W * 100, y = v => 8 + (1 - (+v || 0) / max) * (H - 12),
   path = vs => L.map((_, i) => (i ? 'L' : 'M') + x(i).toFixed(1) + ',' + y(vs[i]).toFixed(1)).join('');
  const head = el('div', 'ak-lc-legend'), tg = el('button', 'ak-lc-toggle', asTable ? opt.chartLabel || t('chart') : opt.tableLabel || t('table'), {type: 'button'});
  if (S.length > 1) for (const z of S) head.appendChild(el('span', 0, z.label)).prepend(el('i', 0, null, {style: 'background:' + z.color}));
  tg.onclick = () => {asTable = !asTable; draw();};
  head.append(tg); box.replaceChildren(head);
  if (ro) ro.disconnect();
  if (asTable) {
   const tb = el('table', 'ak-table'), hr = el('tr'), body = el('tbody'), wrap = el('div', 'ak-lc-table');
   hr.append(el('th', 0, opt.dateLabel || t('date')), ...S.map(z => el('th', 'ak-num', z.label)));
   L.forEach((l, i) => body.insertRow().append(el('td', 0, l), ...S.map(z => el('td', 'ak-num', fmt(z.values[i])))));
   tb.append(el('thead'), body); tb.tHead.append(hr); wrap.append(tb); box.append(wrap);
   return;
  }
  const plot = el('div', 'ak-lc-plot', null, {tabindex: 0}), inner = plot.appendChild(el('div', 'ak-lc-inner')),
   s = inner.appendChild(svg('svg', {viewBox: `0 0 ${W} ${H}`, preserveAspectRatio: 'none', role: 'img', 'aria-label': S.map(z => z.label).join(', '), style: 'height:' + H + 'px'})),
   cross = svg('line', {class: 'ak-lc-cross', y1: 8, y2: H - 4, 'vector-effect': 'non-scaling-stroke', visibility: 'hidden'}), xs = el('div', 'ak-lc-x'), hov = el('div');
  for (let v = 0; v <= max + 1e-9; v += step) {
   s.append(svg('line', {class: 'ak-lc-grid', x1: 0, x2: W, y1: y(v), y2: y(v), 'vector-effect': 'non-scaling-stroke'}));
   inner.append(el('span', 'ak-lc-y', fmt(v), {style: 'top:' + y(v) + 'px'}));
  }
  if (S[0] && L.length) s.append(svg('path', {class: 'ak-lc-area', fill: S[0].color, d: path(S[0].values) + `L${x(L.length - 1)},${y(0)}L${x(0)},${y(0)}Z`}));
  if (L.length) for (const z of S) s.append(svg('path', {class: 'ak-lc-line', stroke: z.color, d: path(z.values), 'vector-effect': 'non-scaling-stroke'}));
  s.append(cross); inner.append(xs, hov); box.append(plot);
  let cur = -1;
  const show = i => {
   cur = i; hov.replaceChildren(); at(cross, {visibility: i < 0 ? 'hidden' : 'visible', x1: x(i), x2: x(i)});
   if (i < 0 || !L.length) return;
   const tip = el('div', 'ak-lc-tip' + (px(i) > 60 ? ' left' : ''), null, {style: `left:${px(i)}%`});
   tip.append(el('div', 'ak-lc-date', L[i]));
   for (const z of S) {
    hov.append(el('span', 'ak-lc-dot', null, {style: `left:${px(i)}%;top:${y(z.values[i])}px;border-color:${z.color}`}));
    tip.appendChild(el('div')).append(el('i', 0, null, {style: 'background:' + z.color}), el('b', 0, fmt(z.values[i])), el('span', 0, z.label));
   }
   hov.append(tip);
  };
  const clamp = i => Math.max(0, Math.min(n - 1, i));
  s.onpointermove = e => {const r = s.getBoundingClientRect(); show(clamp(Math.round((e.clientX - r.left) / (r.width || 1) * (n - 1)) || 0));};
  plot.onpointerleave = plot.onblur = () => show(-1);
  plot.onkeydown = e => {const k = {ArrowLeft: -1, ArrowRight: 1}[e.key]; if (k) {e.preventDefault(); show(clamp(cur < 0 ? (k > 0 ? 0 : n - 1) : cur + k));}};
  // x labels: as many as fit without overlapping (first and last always)
  const ticks = () => {
   const w = inner.clientWidth || 600, lw = Math.max(1, ...L.map(l => l.length)) * 7 + 16, want = Math.min(L.length, Math.max(2, Math.floor(w / lw))), ix = new Set();
   for (let k = 0; k < want; k++) ix.add(want < 2 ? 0 : Math.round(k * (L.length - 1) / (want - 1)));
   xs.replaceChildren(...[...ix].map(i => el('span', n > 1 && i == 0 ? 'ak-first' : n > 1 && i == n - 1 ? 'ak-last' : '', L[i], {style: `left:${px(i)}%`})));
  };
  ticks();
  if (window.ResizeObserver) (ro = new ResizeObserver(ticks)).observe(inner);
 }
 const set = v => {opt = v || {}; draw();};
 set(o);
 return {set, el: box};
}

// iframe ↔ admin: the admin sizes the frame to the document ({type: 'ak:height'}), opens admin paths ({type: 'ak:navigate'}) and picks media ({type: 'ak:pick-media'})
const framed = () => window.parent !== window;
let lastH = -1;
function height() {
 const h = Math.ceil(d.documentElement.getBoundingClientRect().height);
 if (framed() && h != lastH) parent.postMessage({type: 'ak:height', height: lastH = h}, '*');
 return h;
}
// open('/admin/x/code/<plugin>:<page>'): an admin page in the top window (the admin validates it; the sandbox never navigates the top)
function open(path) {
 path = String(path || '');
 framed() ? parent.postMessage({type: 'ak:navigate', path}, '*') : (location.href = path);
}

// pickMedia({accept: 'image'|'file', timeout}) → Promise<{url, id, alt, width, height} | null>: the admin's media library dialog
// ({type: 'ak:pick-media', reqId, accept} → {type: 'ak:pick-media:result', reqId, media}); cancelled, not allowed, timed out or unframed → null
const picks = {};
let pickSeq = 0;
function pickMedia(o = {}) {
 if (!framed()) return Promise.resolve(null);
 const reqId = 'ak' + Date.now().toString(36) + '-' + ++pickSeq;
 return new Promise(res => {
  const tm = setTimeout(() => done(null), +o.timeout > 0 ? +o.timeout : 30 * 60e3), done = v => {clearTimeout(tm); delete picks[reqId]; res(v);};
  picks[reqId] = done;
  parent.postMessage({type: 'ak:pick-media', reqId, accept: o.accept == 'file' ? 'file' : 'image'}, '*');
 });
}
window.addEventListener('message', e => {
 const m = e.data;
 if (e.source !== parent || !m || m.type != 'ak:pick-media:result' || !picks[m.reqId]) return;
 const x = m.media && typeof m.media == 'object' && typeof m.media.url == 'string' ? m.media : null;
 picks[m.reqId](x && {url: x.url, id: +x.id || 0, alt: String(x.alt || ''), width: +x.width || 0, height: +x.height || 0});
});

// 2.3.0: <input type=file> gets a button and file name in the admin's language (the browser's own label
// follows the browser, e.g. "Choose File / No file chosen" on a Thai admin); the input stays focusable
function file(i) {
 if (i.dataset.akFile || i.closest('.ak-file')) return;
 i.dataset.akFile = '1';
 const w = el('span', 'ak-file'), b = el('button', 'ak-btn ak-btn-sm', '📄 ' + t('pick'), {type: 'button'}), n = el('span', 'ak-file-name', t('nofile'));
 i.parentNode.insertBefore(w, i);
 w.append(b, n, i);
 i.classList.add('ak-file-input');
 b.addEventListener('click', () => i.click());
 i.addEventListener('change', () => {const f = i.files || []; n.textContent = f.length > 1 ? f.length + ' ' + t('nfiles') : f[0] ? f[0].name : t('nofile');});
}
function init() {
 d.body.classList.add('ak');
 each('input[type=file]', file);
 if (window.MutationObserver) new MutationObserver(ms => ms.forEach(m => m.addedNodes.forEach(x => {if (x.nodeType == 1) {if (x.matches('input[type=file]')) file(x); else each('input[type=file]', file, x);}}))).observe(d.body, {childList: true, subtree: true});
 each('[data-ak-tabs]', tabs);
 each('table[data-ak-sortable]', sortable);
 each('form[data-ak-validate]', f => {f.noValidate = true; f.addEventListener('submit', e => {if (!validate(f)) {e.preventDefault(); e.stopImmediatePropagation();}}, true);});
 each('[data-ak-bar]', e => bar(e, JSON.parse(e.dataset.akBar)));
 each('[data-ak-spark]', e => sparkline(e, JSON.parse(e.dataset.akSpark)));
 each('[data-ak-line]', e => line(e, JSON.parse(e.dataset.akLine)));
 // <a href="/admin/…" data-ak-open>: opens in the admin (a modified click keeps the browser's own new tab)
 d.addEventListener('click', e => {
  const a = e.target.closest && e.target.closest('[data-ak-open]');
  if (a && !e.defaultPrevented && !e.button && !e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey) {e.preventDefault(); open(a.dataset.akOpen || a.getAttribute('href'));}
 });
 if (window.ResizeObserver) new ResizeObserver(() => height()).observe(d.body);
 window.addEventListener('load', () => height());
 height();
}
window.AK = {lang, t, messages: M, toast, confirm, tabs, table, sortable, pager, validate, bar, sparkline, line, open, pickMedia, height, init};
d.readyState == 'loading' ? d.addEventListener('DOMContentLoaded', init) : init();
})();
