/* analytics: the daily chart of the admin page and the dashboard widget.
 * Draws every <div data-ak-line='{labels, series:[{label, color, values}], height, locale, tableLabel, chartLabel, dateLabel}'>.
 * CMS 2.0.3+ ships AK.line in the admin UI kit (it draws these itself); on older CMS (2.0.1 kit: no AK.line) this file
 * draws them with its own port of AK.line — same look: one y-axis with nice ticks, gridlines, a soft area under the
 * first series, legend, hover/keyboard crosshair + tooltip, chart ⇄ table — and loads chart.css next to it. */
(function () {
  'use strict';
  var d = document, NS = 'http://www.w3.org/2000/svg';
  var me = d.currentScript && d.currentScript.src;
  var M = {
    th: {table: 'ดูเป็นตาราง', chart: 'ดูเป็นกราฟ', date: 'วันที่'},
    en: {table: 'View as table', chart: 'View as chart', date: 'Date'},
  };
  var lang = function () { return /^en/.test(d.documentElement.lang) ? 'en' : 'th'; };
  var t = function (k) { return M[lang()][k] || k; };
  var at = function (e, a) { for (var k in a) e.setAttribute(k, a[k]); return e; };
  var el = function (tag, cls, txt, a) { var e = at(d.createElement(tag), a || {}); if (cls) e.className = cls; if (txt != null) e.textContent = txt; return e; };
  var svg = function (tag, a) { return at(d.createElementNS(NS, tag), a); };
  var nice = function (r) {
    if (r <= 1) return 1;
    var p = Math.pow(10, Math.floor(Math.log(r) / Math.LN10)), f = r / p;
    return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10) * p;
  };

  var cssDone = false;
  function css() {
    if (cssDone || !me) return;
    cssDone = true;
    d.head.appendChild(el('link', null, null, {rel: 'stylesheet', href: me.replace(/chart\.js(\?|$)/, 'chart.css$1'), 'data-an-chart': ''}));
  }

  /** line(el, opts) → {set, el}: the plugin's own AK.line. */
  function line(root, o) {
    css();
    var W = 1000, box = el('div', 'an-lc'), opt, asTable = false, ro;
    root.replaceChildren ? root.replaceChildren(box) : (root.innerHTML = '', root.appendChild(box));
    function draw() {
      var L = (opt.labels || []).map(String),
        S = (opt.series || []).filter(function (s) { return s && Array.isArray(s.values); }),
        H = +opt.height || 240, n = Math.max(1, L.length),
        loc = opt.locale || (lang() == 'en' ? 'en-US' : 'th-TH'),
        fmt = function (v) { return (+v || 0).toLocaleString(loc); },
        all = [1];
      S.forEach(function (s) { s.values.forEach(function (v) { all.push(+v || 0); }); });
      var m = Math.max.apply(null, all), max = Math.ceil(m / nice(m / 4)) * nice(m / 4), step = nice(max / 4),
        x = function (i) { return n == 1 ? W / 2 : i * W / (n - 1); },
        px = function (i) { return x(i) / W * 100; },
        y = function (v) { return 8 + (1 - (+v || 0) / max) * (H - 12); },
        path = function (vs) { return L.map(function (_, i) { return (i ? 'L' : 'M') + x(i).toFixed(1) + ',' + y(vs[i]).toFixed(1); }).join(''); };
      var head = el('div', 'an-lc-legend'),
        tg = el('button', 'an-lc-toggle', asTable ? opt.chartLabel || t('chart') : opt.tableLabel || t('table'), {type: 'button'});
      if (S.length > 1) S.forEach(function (z) {
        var s = head.appendChild(el('span', null, z.label));
        s.insertBefore(el('i', null, null, {style: 'background:' + z.color}), s.firstChild);
      });
      tg.onclick = function () { asTable = !asTable; draw(); };
      head.appendChild(tg);
      box.innerHTML = '';
      box.appendChild(head);
      if (ro) ro.disconnect();
      if (asTable) {
        var tb = el('table', 'ak-table'), hr = el('tr'), body = el('tbody'), wrap = el('div', 'an-lc-table');
        hr.appendChild(el('th', null, opt.dateLabel || t('date')));
        S.forEach(function (z) { hr.appendChild(el('th', 'ak-num', z.label)); });
        L.forEach(function (l, i) {
          var r = body.insertRow();
          r.appendChild(el('td', null, l));
          S.forEach(function (z) { r.appendChild(el('td', 'ak-num', fmt(z.values[i]))); });
        });
        tb.appendChild(el('thead')).appendChild(hr);
        tb.appendChild(body);
        wrap.appendChild(tb);
        box.appendChild(wrap);
        return;
      }
      var plot = el('div', 'an-lc-plot', null, {tabindex: 0}), inner = plot.appendChild(el('div', 'an-lc-inner')),
        s = inner.appendChild(svg('svg', {viewBox: '0 0 ' + W + ' ' + H, preserveAspectRatio: 'none', role: 'img',
          'aria-label': S.map(function (z) { return z.label; }).join(', '), style: 'height:' + H + 'px'})),
        cross = svg('line', {'class': 'an-lc-cross', y1: 8, y2: H - 4, 'vector-effect': 'non-scaling-stroke', visibility: 'hidden'}),
        xs = el('div', 'an-lc-x'), hov = el('div');
      for (var v = 0; v <= max + 1e-9; v += step) {
        s.appendChild(svg('line', {'class': 'an-lc-grid', x1: 0, x2: W, y1: y(v), y2: y(v), 'vector-effect': 'non-scaling-stroke'}));
        inner.appendChild(el('span', 'an-lc-y', fmt(v), {style: 'top:' + y(v) + 'px'}));
      }
      if (S[0] && L.length) s.appendChild(svg('path', {'class': 'an-lc-area', fill: S[0].color,
        d: path(S[0].values) + 'L' + x(L.length - 1) + ',' + y(0) + 'L' + x(0) + ',' + y(0) + 'Z'}));
      if (L.length) S.forEach(function (z) {
        s.appendChild(svg('path', {'class': 'an-lc-line', stroke: z.color, d: path(z.values), 'vector-effect': 'non-scaling-stroke'}));
      });
      s.appendChild(cross);
      inner.appendChild(xs);
      inner.appendChild(hov);
      box.appendChild(plot);
      var cur = -1;
      var show = function (i) {
        cur = i;
        hov.innerHTML = '';
        at(cross, {visibility: i < 0 ? 'hidden' : 'visible', x1: x(i), x2: x(i)});
        if (i < 0 || !L.length) return;
        var tip = el('div', 'an-lc-tip' + (px(i) > 60 ? ' left' : ''), null, {style: 'left:' + px(i) + '%'});
        tip.appendChild(el('div', 'an-lc-date', L[i]));
        S.forEach(function (z) {
          hov.appendChild(el('span', 'an-lc-dot', null, {style: 'left:' + px(i) + '%;top:' + y(z.values[i]) + 'px;border-color:' + z.color}));
          var row = tip.appendChild(el('div'));
          row.appendChild(el('i', null, null, {style: 'background:' + z.color}));
          row.appendChild(el('b', null, fmt(z.values[i])));
          row.appendChild(el('span', null, z.label));
        });
        hov.appendChild(tip);
      };
      var clamp = function (i) { return Math.max(0, Math.min(n - 1, i)); };
      s.onpointermove = s.onmousemove = function (e) {
        var r = s.getBoundingClientRect();
        show(clamp(Math.round((e.clientX - r.left) / (r.width || 1) * (n - 1)) || 0));
      };
      plot.onpointerleave = plot.onmouseleave = plot.onblur = function () { show(-1); };
      plot.onkeydown = function (e) {
        var k = {ArrowLeft: -1, ArrowRight: 1}[e.key];
        if (k) { e.preventDefault(); show(clamp(cur < 0 ? (k > 0 ? 0 : n - 1) : cur + k)); }
      };
      // x labels: as many as fit without overlapping (first and last always)
      var ticks = function () {
        var w = inner.clientWidth || 600, lw = Math.max.apply(null, [1].concat(L.map(function (l) { return l.length; }))) * 7 + 16,
          want = Math.min(L.length, Math.max(2, Math.floor(w / lw))), ix = [];
        for (var k = 0; k < want; k++) {
          var j = want < 2 ? 0 : Math.round(k * (L.length - 1) / (want - 1));
          if (ix.indexOf(j) < 0) ix.push(j);
        }
        xs.innerHTML = '';
        ix.forEach(function (i) {
          xs.appendChild(el('span', n > 1 && i == 0 ? 'an-first' : n > 1 && i == n - 1 ? 'an-last' : '', L[i], {style: 'left:' + px(i) + '%'}));
        });
      };
      ticks();
      if (window.ResizeObserver) (ro = new ResizeObserver(ticks)).observe(inner);
    }
    var set = function (v) { opt = v || {}; draw(); };
    set(o || {});
    return {set: set, el: box};
  }

  /** The kit's AK.line when this CMS has it (2.0.3+), else the plugin's own. */
  function draw(e) {
    var o;
    try { o = JSON.parse(e.getAttribute('data-ak-line') || '{}'); } catch (err) { return; }
    var kit = window.AK && typeof window.AK.line == 'function';
    if (kit) {
      if (!e.querySelector('.ak-lc')) window.AK.line(e, o); // the kit's init normally drew it already
    } else if (!e.querySelector('.an-lc')) {
      line(e, o);
    }
  }
  function init() {
    var list = d.querySelectorAll('[data-ak-line]');
    for (var i = 0; i < list.length; i++) draw(list[i]);
  }
  window.AnalyticsChart = {line: line, init: init};
  // deferred after the kit (both in document order): the kit has run when this runs
  d.readyState == 'loading' ? d.addEventListener('DOMContentLoaded', init) : init();
})();
